package pendulum

import (
	"math/big"
	"sync"
	"time"

	"github.com/python-pendulum/pendulum/internal/clocal"
	"github.com/python-pendulum/pendulum/internal/clock"
)

// The travel helpers drive time-machine as pendulum's Traveller does: while travelling
// the clock reads the destination (frozen, or running on from it), and a destination in
// a named zone also makes that zone the C library's local zone until the next move.
var trav struct {
	mu      sync.Mutex
	started bool
	tick    bool
	destNS  *big.Int
	tzname  *string
	origTZ  *string
	realRef time.Time
}

// destination is time-machine's extract_timestamp_tzname for a DateTime.
func destination(dt DateTime) (*big.Int, *string, error) {
	var tzname *string
	if tz, ok := dt.Timezone().(*Timezone); ok {
		n := tz.Name()
		tzname = &n
	}
	d := dt
	if dt.Timezone() == nil {
		var err error
		if d, err = dt.replace(replaceArgs{tz: UTC, setTz: true}); err != nil {
			return nil, nil, err
		}
	}
	whole, err := d.replace(replaceArgs{f: [7]*int{6: ip(0)}})
	if err != nil {
		return nil, nil, err
	}
	ts, err := whole.d.Timestamp()
	if err != nil {
		return nil, nil, err
	}
	ns := new(big.Int).Mul(big.NewInt(int64(ts)), big.NewInt(1_000_000_000))
	return ns.Add(ns, big.NewInt(int64(dt.Microsecond())*1000)), tzname, nil
}

func travStart() {
	if trav.tzname != nil {
		if v, ok := clocal.TZ(); ok {
			trav.origTZ = &v
		} else {
			trav.origTZ = nil
		}
		clocal.SetTZ(trav.tzname)
	}
	trav.realRef = time.Now()
	applyClock()
}

func travStop() {
	if trav.tzname != nil {
		clocal.SetTZ(trav.origTZ)
	}
}

func applyClock() {
	if trav.tick {
		// A running clock: the destination plus the real time elapsed since the move.
		ns := new(big.Int).Add(trav.destNS, big.NewInt(time.Since(trav.realRef).Nanoseconds()))
		clock.Freeze(ns)
		return
	}
	clock.Freeze(trav.destNS)
}

func moveTo(dt DateTime, tick *bool) error {
	ns, tzname, err := destination(dt)
	if err != nil {
		return err
	}
	travStop()
	trav.destNS, trav.tzname = ns, tzname
	travStart()
	if tick != nil {
		trav.tick = *tick
		applyClock()
	}
	return nil
}

func startTravel(freeze bool) error {
	if trav.started {
		return nil
	}
	now, err := Now(nil)
	if err != nil {
		return err
	}
	ns, tzname, err := destination(now)
	if err != nil {
		return err
	}
	trav.destNS, trav.tzname, trav.tick = ns, tzname, !freeze
	travStart()
	trav.started = true
	return nil
}

// TravelTo is pendulum.travel_to(dt, freeze=freeze): the clock jumps to dt.
func TravelTo(dt DateTime, freeze bool) error {
	trav.mu.Lock()
	defer trav.mu.Unlock()
	if err := startTravel(freeze); err != nil {
		return err
	}
	return moveTo(dt, nil)
}

// Travel is pendulum.travel(..., freeze=freeze): the clock moves by the given amount
// from now (as seen in the local zone).
func Travel(a AddArgs, freeze bool) error {
	trav.mu.Lock()
	defer trav.mu.Unlock()
	if err := startTravel(freeze); err != nil {
		return err
	}
	now, err := Now(nil)
	if err != nil {
		return err
	}
	dest, err := now.Add(a)
	if err != nil {
		return err
	}
	return moveTo(dest, nil)
}

// Freeze is pendulum.freeze(): the clock stops at now.
func Freeze() error {
	trav.mu.Lock()
	defer trav.mu.Unlock()
	if !trav.started {
		return startTravel(true)
	}
	now, err := Now(nil)
	if err != nil {
		return err
	}
	f := false
	return moveTo(now, &f)
}

// TravelBack is pendulum.travel_back(): the real clock again.
func TravelBack() {
	trav.mu.Lock()
	defer trav.mu.Unlock()
	if !trav.started {
		return
	}
	travStop()
	clock.Release()
	trav.started = false
	trav.destNS, trav.tzname = nil, nil
}
