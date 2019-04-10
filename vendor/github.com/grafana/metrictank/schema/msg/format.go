package msg

//go:generate stringer -type=Format

type Format uint8

// identifier of message format
const (
	FormatMetricDataArrayJson Format = iota
	FormatMetricDataArrayMsgp
<<<<<<< HEAD:vendor/github.com/grafana/metrictank/schema/msg/format.go
	FormatMetricPoint
	FormatMetricPointWithoutOrg
=======
>>>>>>> c49d21cc... Replace old metrics with Prometheus (#7):vendor/gopkg.in/raintank/schema.v1/msg/format.go
)
