package core

import (
	"github.com/wowsims/classic/sim/core/proto"
	"time"
)

// ForeverDamageMultiplier composes a caster-owned modifier without changing
// shared target debuffs. The predicate is evaluated for each damage event.
func (c *Character) ForeverDamageMultiplier(effect func(*Spell, *AttackTable) float64) {
	c.Env.RegisterPostFinalizeEffect(func() {
		for _, target := range c.Env.Encounter.Targets {
			for _, at := range c.AttackTables[target.UnitIndex] {
				old := at.DamageDoneByCasterMultiplier
				at.DamageDoneByCasterMultiplier = func(s *Spell, a *AttackTable) float64 {
					m := effect(s, a)
					if old != nil {
						m *= old(s, a)
					}
					return m
				}
			}
		}
	})
}

// ForeverSpellRange makes range a real cast eligibility check. Existing Classic
// spells deliberately keep their historical unrestricted eligibility.
func (c *Character) ForeverSpellRange(s *Spell, yards float64) {
	if c.Forever == nil {
		return
	}
	old := s.ExtraCastCondition
	s.ExtraCastCondition = func(sim *Simulation, target *Unit) bool {
		return (!c.IsOpponent(target) || c.DistanceFromTarget <= yards) && (old == nil || old(sim, target))
	}
}

// ForeverAbsorb is an actual damage-absorbing aura, unlike the Classic engine's
// reporting-only Shield. Reapplication replaces or carries the remaining amount
// according to the caller, retaining that choice as an explicit policy.
type ForeverAbsorb struct {
	Aura      *Aura
	Remaining float64
	School    SpellSchool
}

func NewForeverAbsorb(target *Unit, label string, action ActionID, duration time.Duration, school SpellSchool) *ForeverAbsorb {
	shield := &ForeverAbsorb{School: school}
	shield.Aura = target.GetOrRegisterAura(Aura{Label: label, ActionID: action, Duration: duration,
		OnExpire: func(a *Aura, sim *Simulation) { shield.Remaining = 0 }})
	target.AddDynamicDamageTakenModifier(func(sim *Simulation, s *Spell, r *SpellResult) {
		if !shield.Aura.IsActive() || (shield.School != 0 && !s.SpellSchool.Matches(shield.School)) {
			return
		}
		absorbed := min(max(0, r.Damage), shield.Remaining)
		r.Damage -= absorbed
		shield.Remaining -= absorbed
		if shield.Remaining <= 0 {
			shield.Aura.Deactivate(sim)
		}
	})
	return shield
}
func (s *ForeverAbsorb) Apply(sim *Simulation, amount float64, carry bool) {
	if carry && s.Aura.IsActive() {
		amount += s.Remaining
	}
	s.Remaining = max(0, amount)
	s.Aura.Activate(sim)
}

// RegisterForeverWand exposes an actual mana-free ranged action for casters.
// It retains the Classic weapon school, damage range and attack interval.
func (c *Character) RegisterForeverWand() {
	if c.Forever == nil || !c.HasRangedWeapon() {
		return
	}
	weapon := c.WeaponFromRanged()
	c.RegisterSpell(SpellConfig{ActionID: ActionID{OtherID: proto.OtherAction_OtherActionShoot}, SpellSchool: weapon.GetSpellSchool(), DefenseType: DefenseTypeRanged, ProcMask: ProcMaskRangedSpecial, Flags: SpellFlagAPL | SpellFlagMeleeMetrics,
		Cast: CastConfig{DefaultCast: Cast{GCD: time.Duration(weapon.SwingSpeed * float64(time.Second))}, IgnoreHaste: true}, DamageMultiplier: 1, ThreatMultiplier: 1,
		ExtraCastCondition: func(sim *Simulation, t *Unit) bool { return c.IsOpponent(t) && c.DistanceFromTarget <= 30 },
		ApplyEffects: func(sim *Simulation, t *Unit, s *Spell) {
			s.CalcAndDealDamage(sim, t, sim.Roll(weapon.BaseDamageMin, weapon.BaseDamageMax), s.OutcomeRangedHitAndCrit)
		},
	})
}
