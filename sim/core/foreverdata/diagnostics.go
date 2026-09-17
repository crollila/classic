package foreverdata

import (
	"fmt"
	"slices"
	"sync"
)

// Override event scopes.
const (
	ScopeSpell      = "spell"       // spells.<id> applied at spell/aura registration
	ScopeSpellValue = "spell_value" // spells.<id>.effects.<i>.base used by hand-written sim code (Unit.ForeverSpellValue)
	ScopeItem       = "item"        // items.<id>.stats / weapon
	ScopeNewItem    = "new_item"    // items.<id>.new_item
	ScopeTalent     = "talent"      // talents.<id>.values
	ScopeParameter  = "parameter"   // parameters.<key>.value
)

// OverrideEvent records one applied or rejected override.
type OverrideEvent struct {
	Scope string `json:"scope"`
	ID    string `json:"id"`
	// Field is the override path below the id, e.g. "effects.0.coefficient", "cast_ms", "stats.strength".
	Field string `json:"field"`
	// Unit is the sim unit label the override was evaluated for (empty for load-time events).
	Unit string `json:"unit,omitempty"`
	// Detail disambiguates repeated evaluations (e.g. action tag, slot, caller value).
	Detail     string   `json:"detail,omitempty"`
	Classic    *float64 `json:"classic,omitempty"`
	Forever    *float64 `json:"forever,omitempty"`
	Registered *float64 `json:"registered,omitempty"` // value observed in the simulator before the override
	Result     *float64 `json:"result,omitempty"`     // value used by the simulator after the override
	Applied    bool     `json:"applied"`
	Reason     string   `json:"reason,omitempty"`
	Beliefs    []string `json:"beliefs,omitempty"`
	Status     string   `json:"status,omitempty"`
}

func (e OverrideEvent) key() string {
	return fmt.Sprintf("%s|%s|%s|%s|%s", e.Scope, e.ID, e.Field, e.Unit, e.Detail)
}

// Diagnostics is a concurrency-safe, de-duplicated override event log.
// One instance exists per Forever character (its pets share it).
type Diagnostics struct {
	mu     sync.Mutex
	seen   map[string]struct{}
	events []OverrideEvent
}

func NewDiagnostics() *Diagnostics { return &Diagnostics{seen: map[string]struct{}{}} }

// Record adds an event unless an event with the same scope/id/field/unit/detail exists.
func (d *Diagnostics) Record(e OverrideEvent) {
	d.mu.Lock()
	defer d.mu.Unlock()
	k := e.key()
	if _, ok := d.seen[k]; ok {
		return
	}
	d.seen[k] = struct{}{}
	e.Beliefs = slices.Clone(e.Beliefs)
	d.events = append(d.events, e)
}

// Seen reports whether an event with this identity was already recorded.
func (d *Diagnostics) Seen(scope, id, field, unit, detail string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.seen[OverrideEvent{Scope: scope, ID: id, Field: field, Unit: unit, Detail: detail}.key()]
	return ok
}

// DiagnosticsReport is an immutable snapshot of applied and rejected overrides.
type DiagnosticsReport struct {
	Applied  []OverrideEvent `json:"applied"`
	Rejected []OverrideEvent `json:"rejected"`
}

func (d *Diagnostics) Report() DiagnosticsReport {
	r := DiagnosticsReport{Applied: []OverrideEvent{}, Rejected: []OverrideEvent{}}
	if d == nil {
		return r
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, e := range d.events {
		e.Beliefs = slices.Clone(e.Beliefs)
		if e.Applied {
			r.Applied = append(r.Applied, e)
		} else {
			r.Rejected = append(r.Rejected, e)
		}
	}
	return r
}

// Ptr is a small helper for building events.
func Ptr(v float64) *float64 { return &v }
