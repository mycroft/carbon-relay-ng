// carbon-relay-ng
// route traffic to anything that speaks the Graphite Carbon protocol (text or pickle)
// such as Graphite's carbon-cache.py, influxdb, ...
package main

import (
	"flag"
	"fmt"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/BurntSushi/toml"
	"github.com/graphite-ng/carbon-relay-ng/badmetrics"
	"github.com/graphite-ng/carbon-relay-ng/cfg"
	"github.com/graphite-ng/carbon-relay-ng/logger"
	"github.com/graphite-ng/carbon-relay-ng/stats"
	"github.com/graphite-ng/carbon-relay-ng/statsmt"
	tbl "github.com/graphite-ng/carbon-relay-ng/table"
	"github.com/graphite-ng/carbon-relay-ng/ui/telnet"
	"github.com/graphite-ng/carbon-relay-ng/ui/web"
	log "github.com/sirupsen/logrus"

	"strconv"
	"strings"
	"time"
)

var (
	config_file     string
	config          = cfg.NewConfig()
	to_dispatch     = make(chan []byte)
	shutdownTimeout = time.Second * 30 // how long to wait for shutdown
	table           *tbl.Table
	enablePprof     = flag.Bool("enable-pprof", false, "Will enable debug endpoints on /debug/pprof/")
	badMetrics      *badmetrics.BadMetrics
	Version         = "unknown"
)

func usage() {
	header := `Usage:
        carbon-relay-ng version
        carbon-relay-ng <path-to-config>
	`
	fmt.Fprintln(os.Stderr, header)
	flag.PrintDefaults()
}

func main() {

	flag.Usage = usage
	flag.Parse()

	config_file = "/etc/carbon-relay-ng.ini"
	if 1 == flag.NArg() {
		val := flag.Arg(0)
		if val == "version" {
			fmt.Printf("carbon-relay-ng %s (built with %s)\n", Version, runtime.Version())
			return
		}
		config_file = val
	}

	meta, err := toml.DecodeFile(config_file, &config)
	if err != nil {
		log.Fatalf("Invalid config file %q: %s", config_file, err.Error())
	}

	if len(meta.Undecoded()) > 0 {
		log.Fatalf("Unknown configuration keys in %s: %q", config_file, meta.Undecoded())
	}

	//runtime.SetBlockProfileRate(1) // to enable block profiling. in my experience, adds 35% overhead.
	formatter := &logger.TextFormatter{}
	formatter.TimestampFormat = "2006-01-02 15:04:05.000"
	log.SetFormatter(formatter)
	lvl, err := log.ParseLevel(config.Log_level)
	if err != nil {
		log.Fatalf("failed to parse log-level %q: %s", config.Log_level, err.Error())
	}
	log.SetLevel(lvl)

	config.Instance = os.Expand(config.Instance, expandVars)
	if len(config.Instance) == 0 {
		log.Error("instance identifier cannot be empty")
		os.Exit(1)
	}

	log.Infof("===== carbon-relay-ng instance '%s' starting. (version %s) =====", config.Instance, Version)

	if os.Getenv("GOMAXPROCS") == "" && config.Max_procs >= 1 {
		log.Debugf("setting GOMAXPROCS to %d", config.Max_procs)
		runtime.GOMAXPROCS(config.Max_procs)
	}

	if config.Pid_file != "" {
		f, err := os.Create(config.Pid_file)
		if err != nil {
			log.Fatalf("error creating pidfile: %s", err.Error())
		}
		_, err = f.Write([]byte(strconv.Itoa(os.Getpid())))
		if err != nil {
			log.Fatalf("error writing to pidfile: %s", err.Error())
		}
		f.Close()

	go func() {
		sys := stats.Gauge("what=virtual_memory.unit=Byte")
		alloc := stats.Gauge("what=memory_allocated.unit=Byte")
		ticker := time.NewTicker(time.Second)
		var memstats runtime.MemStats
		for range ticker.C {
			runtime.ReadMemStats(&memstats)
			sys.Update(int64(memstats.Sys))
			alloc.Update(int64(memstats.Alloc))

		}
	}()

	if config.Instrumentation.Graphite_addr != "" {
		addr, err := net.ResolveTCPAddr("tcp", config.Instrumentation.Graphite_addr)
		if err != nil {
			log.Fatal(err)
		}
		go metrics.Graphite(metrics.DefaultRegistry, time.Duration(config.Instrumentation.Graphite_interval)*time.Millisecond, "", addr)

		// we use a copy of metrictank's stats library for some extra process/memory related stats
		// note: they follow a different naming scheme, and have their own reporter.

		statsmt.NewMemoryReporter()
		_, err = statsmt.NewProcessReporter()
		if err != nil {
			log.Fatalf("stats: could not initialize process reporter: %v", err)
		}
		statsmt.NewGraphite("carbon-relay-ng.stats."+config.Instance, config.Instrumentation.Graphite_addr, config.Instrumentation.Graphite_interval/1000, 1000, time.Second*10)
	}

	err = config.ProcessInputConfig()
	if err != nil {
		log.Fatalf("can't initialize inputs config: %s", err)
	}

	log.Info("initializing routing table...")

	table, err := tbl.InitFromConfig(config, meta)
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}

	tablePrinted := table.Print()
	log.Info("===========================")
	log.Info("========== TABLE ==========")
	log.Info("===========================")
	for _, line := range strings.Split(tablePrinted, "\n") {
		log.Info(line)
	}

	for _, in := range config.Inputs {
		err := in.Start(table)
		if err != nil {
			log.Fatal(err.Error())
		}
	}

	if config.Admin_addr != "" {
		go func() {
			err := telnet.Start(config.Admin_addr, table)
			if err != nil {
				log.Fatalf("Error listening: %s", err.Error())
			}
		}()
	}

	if config.Http_addr != "" {
		go web.Start(config.Http_addr, config, table, *enablePprof)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig, ok := <-sigChan
	if ok {
		log.Infof("Received signal %q. Shutting down", sig)
	}
	for _, i := range config.Inputs {
		err = i.Stop()
		if err != nil {
			log.Warnf("failed to stop input %s: %s", i.Name(), err)
		}
	}
}

func expandVars(in string) (out string) {
	switch in {
	case "HOST":
		hostname, _ := os.Hostname()
		// in case hostname is an fqdn or has dots, only take first part
		parts := strings.SplitN(hostname, ".", 2)
		return parts[0]
	default:
		return ""
	}
}
