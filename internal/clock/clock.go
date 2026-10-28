// Package clock is the wall clock the Python original reads through time.time(), and
// time-machine's frozen replacement for it, which pendulum's travel helpers drive.
package clock

import (
	"math/big"
	"sync"
	"time"
)

var (
	mu     sync.Mutex
	frozen bool
	nowNS  *big.Int
)

// NowNS is time.time_ns(): the frozen destination while travelling, else the real clock.
func NowNS() *big.Int {
	mu.Lock()
	defer mu.Unlock()
	if frozen {
		return new(big.Int).Set(nowNS)
	}
	return big.NewInt(time.Now().UnixNano())
}

// Now is time.time(): NowNS divided by 10**9 with Python's correctly rounded true
// division of two ints.
func Now() float64 {
	f, _ := new(big.Rat).SetFrac(NowNS(), big.NewInt(1_000_000_000)).Float64()
	return f
}

// Freeze stops the clock at ns nanoseconds after the epoch.
func Freeze(ns *big.Int) {
	mu.Lock()
	defer mu.Unlock()
	frozen, nowNS = true, new(big.Int).Set(ns)
}

// Release returns the clock to real time.
func Release() {
	mu.Lock()
	defer mu.Unlock()
	frozen, nowNS = false, nil
}

// Frozen reports whether the clock is stopped.
func Frozen() bool {
	mu.Lock()
	defer mu.Unlock()
	return frozen
}
