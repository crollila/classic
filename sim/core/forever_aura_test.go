package core

import (
	"fmt"
	"testing"
)

func TestDiscoveryTargetRegistrationGuard(t *testing.T) {
	for _, tc := range []struct {
		name      string
		kind      UnitType
		limit     int
		wantPanic bool
	}{
		{"classic enemy", EnemyUnit, 0, true},
		{"discovery enemy", EnemyUnit, 8000, false},
		{"discovery player keeps guard", PlayerUnit, 8000, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			unit := &Unit{Type: tc.kind, Env: &Environment{discoveryTargetAuraRegistrationLimit: tc.limit}, auraTracker: newAuraTracker()}
			panicked := false
			func() {
				defer func() { panicked = recover() != nil }()
				for i := 0; i < 202; i++ {
					unit.RegisterAura(Aura{Label: fmt.Sprintf("synthetic-registration-%d", i)})
				}
			}()
			if panicked != tc.wantPanic {
				t.Fatalf("panic=%v want %v", panicked, tc.wantPanic)
			}
		})
	}
}
