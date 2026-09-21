package gamedata

import (
	"sort"
	"sync"
)

// A Fallback is a place where a Forever simulation used a value that is not the client's: the
// client has no such spell/effect, or the mechanic needs a number the client does not state
// (a proc rate, a hidden internal cooldown). Every one is recorded so none is silent; the
// registry is what tests and the release report read.
type Fallback struct {
	Owner      string  // e.g. "rogue: Instant Poison proc"
	What       string  // e.g. "proc chance"
	Value      float64 // the value used
	Reason     string  // why the client value was not used
	Confidence string  // PROVISIONAL, PREDICTED or UNKNOWN
}

var (
	fallbackMu sync.Mutex
	fallbacks  = map[string]Fallback{}
)

// Use records a non-client value and returns it.
func Use(owner, what string, value float64, confidence, reason string) float64 {
	fallbackMu.Lock()
	fallbacks[owner+"|"+what] = Fallback{Owner: owner, What: what, Value: value, Reason: reason, Confidence: confidence}
	fallbackMu.Unlock()
	return value
}

// Fallbacks lists every recorded non-client value, sorted.
func Fallbacks() []Fallback {
	fallbackMu.Lock()
	defer fallbackMu.Unlock()
	out := make([]Fallback, 0, len(fallbacks))
	for _, f := range fallbacks {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Owner+out[i].What < out[j].Owner+out[j].What })
	return out
}
