package core

import (
	"strings"
	"time"
)

// Debuff categories are engine tags, not fabricated game spell IDs. They support
// Forever cleanses/immunities and remain inert for units without these effects.
func foreverDebuffKind(a *Aura) string {
	for _, kind := range []string{"bleed", "poison", "disease", "curse", "bane"} {
		if a.Tag == "forever-debuff-"+kind {
			return kind
		}
	}
	name := strings.ToLower(a.Label)
	if strings.Contains(name, "curse of") {
		return "curse"
	}
	if strings.Contains(name, "bane of") {
		return "bane"
	}
	return ""
}
func (u *Unit) ForeverDispel(sim *Simulation, kind string) {
	for _, a := range u.GetAuras() {
		if a.IsActive() && foreverDebuffKind(a) == kind {
			a.Deactivate(sim)
		}
	}
}
func (u *Unit) ForeverDebuffImmunity(label string, action ActionID, kinds []string, duration time.Duration) *Aura {
	state := u.initForeverControl()
	return u.RegisterAura(Aura{Label: label, ActionID: action, Duration: duration,
		OnGain: func(a *Aura, sim *Simulation) {
			for _, k := range kinds {
				state.immune[ForeverControlKind(k)]++
				u.ForeverDispel(sim, k)
			}
		},
		OnExpire: func(a *Aura, sim *Simulation) {
			for _, k := range kinds {
				state.immune[ForeverControlKind(k)]--
			}
		},
	})
}
func (u *Unit) foreverAuraImmune(a *Aura) bool {
	return u.foreverControls != nil && u.foreverControls.immune[ForeverControlKind(foreverDebuffKind(a))] > 0
}
