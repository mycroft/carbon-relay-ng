package destination

import (
	"sync"
	"time"

	"github.com/graphite-ng/carbon-relay-ng/encoding"
)

// keepSafe is a buffer which retains
// at least the last periodKeep's worth of data
// typically you get between periodKeep and 2*periodKeep
// but don't rely on that
type keepSafe struct {
	initialCap int
	safeOld    []encoding.Datapoint
	safeRecent []encoding.Datapoint
	periodKeep time.Duration
	closed     chan struct{}
	wg         sync.WaitGroup
	sync.Mutex
}

func NewKeepSafe(initialCap int, periodKeep time.Duration) *keepSafe {
	k := &keepSafe{
		initialCap: initialCap,
		safeOld:    make([]encoding.Datapoint, 0, initialCap),
		safeRecent: make([]encoding.Datapoint, 0, initialCap),
		periodKeep: periodKeep,
		closed:     make(chan struct{}),
	}
	k.wg.Add(1)
	go k.keepClean()
	return k
}

func (k *keepSafe) keepClean() {
	tick := time.NewTicker(k.periodKeep)
	defer k.wg.Done()
	for {
		select {
		case <-k.closed:
			return
		case <-tick.C:
			k.Lock()
			k.safeOld = k.safeRecent
			k.safeRecent = make([]encoding.Datapoint, 0, k.initialCap)
			k.Unlock()
		}
	}
}

func (k *keepSafe) Add(dp encoding.Datapoint) {
	k.Lock()
	k.safeRecent = append(k.safeRecent, dp)
	k.Unlock()
}

func (k *keepSafe) GetAll() []encoding.Datapoint {
	k.Lock()
	ret := append(k.safeOld, k.safeRecent...)
	k.safeOld = make([]encoding.Datapoint, 0, k.initialCap)
	k.safeRecent = make([]encoding.Datapoint, 0, k.initialCap)
	k.Unlock()
	return ret
}

func (k *keepSafe) Stop() {
	close(k.closed)
	k.wg.Wait()
}
