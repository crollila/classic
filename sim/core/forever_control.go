package core

import (
	"fmt"
	"math"
	"time"
)

// PROVISIONAL encounter control model: Classic boss immunity and strongest
// duration reduction; separate sources overlap without prematurely resuming attacks.
type ForeverControlKind string

const (
	ForeverStun         ForeverControlKind = "stun"
	ForeverRoot         ForeverControlKind = "root"
	ForeverSilence      ForeverControlKind = "silence"
	ForeverFear         ForeverControlKind = "fear"
	ForeverDisarm       ForeverControlKind = "disarm"
	ForeverSnare        ForeverControlKind = "snare"
	ForeverCharm        ForeverControlKind = "charm"
	ForeverSleep        ForeverControlKind = "sleep"
	ForeverIncapacitate ForeverControlKind = "incapacitate"
)

type foreverControlState struct {
	spellInterceptors []func(*Simulation, *Spell) bool
	active            map[ForeverControlKind]int
	reduction         map[ForeverControlKind]float64
	immune            map[ForeverControlKind]int
	moveGeneration    uint64
	snares            map[*Aura]float64
	interruptResists  map[*Aura]float64
	immunityCharges   map[ForeverControlKind][]*Aura
	schoolLocks       map[*Aura]SpellSchool
}

// AddForeverSpellInterceptor registers a conditional, single-charge interceptor.
// Callers own its range and active aura. Returning true consumes the incoming
// effect, including control/debuff applications that deal no direct damage.
func (u *Unit) AddForeverSpellInterceptor(intercept func(*Simulation, *Spell) bool) {
	state := u.initForeverControl()
	state.spellInterceptors = append(state.spellInterceptors, intercept)
}

func (u *Unit) foreverInterceptSpell(sim *Simulation, spell *Spell) bool {
	if u.foreverControls == nil || !spell.ForeverSingleTargetHarmful || spell.Flags.Matches(SpellFlagHelpful) || spell.SpellSchool.Matches(SpellSchoolPhysical) {
		return false
	}
	for _, intercept := range u.foreverControls.spellInterceptors {
		if intercept(sim, spell) {
			return true
		}
	}
	return false
}

func (u *Unit) initForeverControl() *foreverControlState {
	if u.foreverControls == nil {
		u.foreverControls = &foreverControlState{active: map[ForeverControlKind]int{}, reduction: map[ForeverControlKind]float64{}, immune: map[ForeverControlKind]int{}, snares: map[*Aura]float64{}, interruptResists: map[*Aura]float64{}, immunityCharges: map[ForeverControlKind][]*Aura{}}
	}
	return u.foreverControls
}
func (u *Unit) ForeverControlReduction(kind ForeverControlKind, amount float64) {
	state := u.initForeverControl()
	state.reduction[kind] = max(state.reduction[kind], min(1, amount))
}
func (u *Unit) ForeverControlled(kind ForeverControlKind) bool {
	return u.foreverControls != nil && u.foreverControls.active[kind] > 0
}

// Movement is integrated in short steps so a root, stun or snare applied mid-run
// changes the remaining travel time. Classic movement uses its existing path.
func (u *Unit) foreverMoveTo(destination float64, sim *Simulation) {
	if u.MovementHandler == nil {
		u.initMovement()
	}
	state := u.initForeverControl()
	state.moveGeneration++
	generation := state.moveGeneration
	if destination == u.DistanceFromTarget {
		u.MovementHandler.moveAura.Deactivate(sim)
		return
	}
	u.MovementHandler.moveAura.Activate(sim)
	step := 100 * time.Millisecond
	var next func(*Simulation)
	next = func(sim *Simulation) {
		if generation != state.moveGeneration || !u.MovementHandler.moveAura.IsActive() {
			return
		}
		if !u.ForeverControlled(ForeverRoot) && !u.foreverIncapacitated() {
			speed := u.MovementHandler.MoveSpeed
			slow := 0.
			for aura, fraction := range state.snares {
				if aura.IsActive() {
					slow = math.Max(slow, fraction)
				}
			}
			speed *= 1 - slow
			remaining := destination - u.DistanceFromTarget
			travel := math.Min(math.Abs(remaining), speed*step.Seconds())
			u.DistanceFromTarget += math.Copysign(travel, remaining)
		}
		if math.Abs(destination-u.DistanceFromTarget) < 0.001 {
			u.DistanceFromTarget = destination
			u.MovementHandler.moveAura.Deactivate(sim)
			return
		}
		sim.AddPendingAction(&PendingAction{NextActionAt: sim.CurrentTime + step, OnAction: next})
	}
	sim.AddPendingAction(&PendingAction{NextActionAt: sim.CurrentTime + step, OnAction: next})
}
func (u *Unit) foreverCanCast(s *Spell) bool {
	if u.foreverControls == nil || s.ForeverIgnoreControl {
		return true
	}
	// Triggered effects continue while the caster is controlled.
	if !s.Flags.Matches(SpellFlagAPL) && s.DefaultCast.GCD == 0 && s.DefaultCast.CastTime == 0 {
		return true
	}
	if u.foreverIncapacitated() {
		return false
	}
	for aura, school := range u.foreverControls.schoolLocks {
		if aura.IsActive() && s.SpellSchool.Matches(school) {
			return false
		}
	}
	if u.ForeverControlled(ForeverSilence) && !s.SpellSchool.Matches(SpellSchoolPhysical) {
		return false
	}
	if u.ForeverControlled(ForeverDisarm) && s.ProcMask.Matches(ProcMaskMeleeOrRanged) {
		return false
	}
	return true
}
func (u *Unit) ForeverInterrupt(sim *Simulation) {
	if u.foreverResistsInterrupt(sim) {
		return
	}
	u.foreverInterruptCast(sim)
}

// ForeverInterruptSchool interrupts an active cast/channel and locks its school.
// The duration comes from the calling ability's evidence or disclosed Classic
// analogue. An idle target gains no lockout. Triggered effects are unaffected.
func (u *Unit) ForeverInterruptSchool(sim *Simulation, duration time.Duration) bool {
	var interrupted *Spell
	if u.ChanneledDot != nil && u.ChanneledDot.IsActive() {
		interrupted = u.ChanneledDot.Spell
	} else if u.Hardcast.Expires > sim.CurrentTime {
		interrupted = u.GetSpell(u.Hardcast.ActionID)
	}
	if interrupted == nil || u.foreverResistsInterrupt(sim) {
		return false
	}
	u.foreverInterruptCast(sim)
	if duration <= 0 {
		return true
	}
	state := u.initForeverControl()
	if state.schoolLocks == nil {
		state.schoolLocks = map[*Aura]SpellSchool{}
	}
	label := fmt.Sprintf("Forever interrupted school %d", interrupted.SpellSchool)
	a := u.GetAura(label)
	if a == nil {
		a = u.RegisterAura(Aura{Label: label, Duration: duration, foreverControlRuntime: true})
		state.schoolLocks[a] = interrupted.SpellSchool
	}
	if a.IsActive() {
		duration = max(duration, a.RemainingDuration(sim))
	}
	a.Duration = duration
	a.Activate(sim)
	return true
}

func (u *Unit) foreverResistsInterrupt(sim *Simulation) bool {
	if u.foreverControls != nil {
		chance := 0.
		for aura, n := range u.foreverControls.interruptResists {
			if aura.IsActive() {
				chance = math.Max(chance, n)
			}
		}
		if chance > 0 && sim.Proc(chance, "Forever Interrupt Resistance") {
			return true
		}
	}
	return false
}
func (u *Unit) foreverInterruptCast(sim *Simulation) {
	if u.ChanneledDot != nil && u.ChanneledDot.IsActive() {
		u.ChanneledDot.Cancel(sim)
	}
	u.Hardcast = Hardcast{Expires: startingCDTime}
	if u.hardcastAction != nil {
		u.hardcastAction.Cancel(sim)
		u.hardcastAction = nil
	}
}
func (u *Unit) ForeverControlAura(label string, action ActionID, kind ForeverControlKind, duration time.Duration) *Aura {
	state := u.initForeverControl()
	effectiveDuration := max(time.Nanosecond, time.Duration(float64(duration)*(1-state.reduction[kind])))
	if existing := u.GetAura(label); existing != nil {
		if existing.ActionID == action && existing.Tag == "forever-control-"+string(kind) {
			existing.Duration = effectiveDuration
			if kind == ForeverSnare {
				state.snares[existing] = .5
			}
			return existing
		}
		label += fmt.Sprintf("-%s-%s", action.String(), kind)
		if existing = u.GetAura(label); existing != nil {
			existing.Duration = effectiveDuration
			if kind == ForeverSnare {
				state.snares[existing] = .5
			}
			return existing
		}
	}
	applied := false
	aura := u.GetOrRegisterAura(Aura{Label: label, ActionID: action, Tag: "forever-control-" + string(kind), Duration: effectiveDuration, foreverControlRuntime: true,
		OnReset: func(a *Aura, sim *Simulation) { applied = false },
		OnGain: func(a *Aura, sim *Simulation) {
			if (u.Type == EnemyUnit && u.Level >= 63) || state.immune[kind] > 0 || state.reduction[kind] >= 1 {
				if state.immune[kind] > 0 {
					charged := 0
					for _, ward := range state.immunityCharges[kind] {
						if ward.IsActive() {
							charged++
						}
					}
					// A non-consuming immunity takes priority over charge-based wards.
					if charged == state.immune[kind] {
						for _, ward := range state.immunityCharges[kind] {
							if ward.IsActive() {
								ward.RemoveStack(sim)
								break
							}
						}
					}
				}
				a.Deactivate(sim)
				return
			}
			applied = true
			state.active[kind]++
			if foreverControlStopsActions(kind) || kind == ForeverSilence {
				u.foreverInterruptCast(sim)
			}
			if kind == ForeverStun {
				u.PseudoStats.Stunned = true
			}
			if foreverControlStopsActions(kind) || kind == ForeverDisarm {
				u.AutoAttacks.CancelAutoSwing(sim)
			}
		}, OnExpire: func(a *Aura, sim *Simulation) {
			if !applied {
				return
			}
			applied = false
			state.active[kind]--
			if kind == ForeverStun {
				u.PseudoStats.Stunned = state.active[ForeverStun] > 0
			}
			if !u.foreverAutosSuppressed() && u.IsEnabled() && (u.MovementHandler == nil || !u.IsMoving()) {
				u.AutoAttacks.EnableAutoSwing(sim)
			}
		}})
	if kind == ForeverSnare {
		state.snares[aura] = .5
	}
	if kind == ForeverIncapacitate || kind == ForeverSleep {
		breakOnDamage := func(a *Aura, sim *Simulation, s *Spell, r *SpellResult) {
			if r.Damage > 0 {
				a.Deactivate(sim)
			}
		}
		aura.OnSpellHitTaken = breakOnDamage
		aura.OnPeriodicDamageTaken = breakOnDamage
	}
	return aura
}
func (u *Unit) ForeverControlImmunityAura(label string, action ActionID, kinds []ForeverControlKind, duration time.Duration) *Aura {
	state := u.initForeverControl()
	return u.GetOrRegisterAura(Aura{Label: label, ActionID: action, Duration: duration,
		OnGain: func(a *Aura, sim *Simulation) {
			for _, k := range kinds {
				state.immune[k]++
				for _, other := range u.GetAurasWithTag("forever-control-" + string(k)) {
					other.Deactivate(sim)
				}
			}
		},
		OnExpire: func(a *Aura, sim *Simulation) {
			for _, k := range kinds {
				state.immune[k]--
			}
		},
	})
}

// Explicit source-backed slows use the strongest active value, as with the
// existing movement penalty heap. A generic Snare remains a 50% Classic fallback.
func (u *Unit) ForeverSnareAura(label string, action ActionID, duration time.Duration, slowFraction float64) *Aura {
	a := u.ForeverControlAura(label, action, ForeverSnare, duration)
	if math.IsNaN(slowFraction) {
		slowFraction = .5
	}
	u.initForeverControl().snares[a] = math.Max(0, math.Min(1, slowFraction))
	return a
}
func foreverControlStopsActions(kind ForeverControlKind) bool {
	switch kind {
	case ForeverStun, ForeverFear, ForeverCharm, ForeverSleep, ForeverIncapacitate:
		return true
	}
	return false
}
func (u *Unit) foreverIncapacitated() bool {
	for _, k := range []ForeverControlKind{ForeverStun, ForeverFear, ForeverCharm, ForeverSleep, ForeverIncapacitate} {
		if u.ForeverControlled(k) {
			return true
		}
	}
	return false
}
func (u *Unit) foreverAutosSuppressed() bool {
	return u.foreverIncapacitated() || u.ForeverControlled(ForeverDisarm)
}

func (u *Unit) ForeverInterruptResistanceAura(label string, action ActionID, duration time.Duration, chance float64) *Aura {
	a := u.GetOrRegisterAura(Aura{Label: label, ActionID: action, Duration: duration})
	u.initForeverControl().interruptResists[a] = math.Max(0, math.Min(1, chance))
	return a
}
func (u *Unit) ForeverControlImmunityChargesAura(label string, action ActionID, kinds []ForeverControlKind, duration time.Duration, charges int32) *Aura {
	state := u.initForeverControl()
	if existing := u.GetAura(label); existing != nil {
		return existing
	}
	a := u.ForeverControlImmunityAura(label, action, kinds, duration)
	a.MaxStacks = charges
	a.ApplyOnGain(func(a *Aura, sim *Simulation) { a.SetStacks(sim, charges) })
	for _, kind := range kinds {
		state.immunityCharges[kind] = append(state.immunityCharges[kind], a)
	}
	return a
}
