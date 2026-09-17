package core

import (
	"strings"
	"time"
)

// Debuff categories are engine tags, not fabricated game spell IDs. They support
// Forever cleanses/immunities and remain inert for units without these effects.
func foreverDebuffKind(a *Aura) string {
	if a.Tag == "forever-buff-magic" {
		return "magic-buff"
	}
	for _, kind := range []string{"bleed", "poison", "disease", "curse", "bane", "magic"} {
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
	return a.foreverDispelType
}
func (u *Unit) ForeverDispel(sim *Simulation, kind string) { u.foreverDispel(sim, kind, false) }
func (u *Unit) foreverDispel(sim *Simulation, kind string, force bool) {
	for _, a := range u.GetAuras() {
		if a.IsActive() && foreverDebuffKind(a) == kind && (force || !sim.Proc(min(1, max(0, a.ForeverDispelResistance)), "Forever Dispel Resistance")) {
			a.Deactivate(sim)
		}
	}
}
func (u *Unit) ForeverHasDebuff(kind string) bool {
	for _, a := range u.GetAuras() {
		if a.IsActive() && foreverDebuffKind(a) == kind {
			return true
		}
	}
	return false
}
func (u *Unit) ForeverDebuffImmunity(label string, action ActionID, kinds []string, duration time.Duration) *Aura {
	state := u.initForeverControl()
	return u.RegisterAura(Aura{Label: label, ActionID: action, Duration: duration,
		OnGain: func(a *Aura, sim *Simulation) {
			for _, k := range kinds {
				state.immune[ForeverControlKind(k)]++
				u.foreverDispel(sim, k, true)
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

// Single-effect cleanses (for example Improved Mend Pet) remove one eligible
// aura per successful proc, preserving all other debuffs and source tags.
func (u *Unit) ForeverDispelOne(sim *Simulation, kinds ...string) bool {
	for _, a := range u.GetAuras() {
		if !a.IsActive() {
			continue
		}
		for _, kind := range kinds {
			if foreverDebuffKind(a) == kind {
				if sim.Proc(min(1, max(0, a.ForeverDispelResistance)), "Forever Dispel Resistance") {
					return false
				}
				a.Deactivate(sim)
				return true
			}
		}
	}
	return false
}
