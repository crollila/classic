package druid

import (
	"github.com/wowsims/classic/sim/core"
	"time"
)

// Classic rank-3 Prowl/Pounce supply the baseline referenced by the Forever
// Feral Instinct and Brutal Impact tooltips. IDs and values are retained from
// assets/db_inputs/wowhead_spell_tooltips.csv (9913 and 9827).
func (d *Druid) registerForeverProwl() {
	action := core.ActionID{SpellID: 9913}
	prowl := d.RegisterAura(core.Aura{Label: "Forever Prowl", ActionID: action, Duration: core.NeverExpires, OnGain: func(a *core.Aura, sim *core.Simulation) {
		d.AutoAttacks.CancelAutoSwing(sim)
		d.AddMoveSpeedModifier(&a.ActionID, .7)
	}, OnExpire: func(a *core.Aura, sim *core.Simulation) {
		d.RemoveMoveSpeedModifier(&a.ActionID)
		d.AutoAttacks.EnableAutoSwing(sim)
	}, OnSpellHitTaken: func(a *core.Aura, sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
		if r.Damage > 0 {
			a.Deactivate(sim)
		}
	}, OnSpellHitDealt: func(a *core.Aura, sim *core.Simulation, sp *core.Spell, r *core.SpellResult) { a.Deactivate(sim) }, OnCastComplete: func(a *core.Aura, sim *core.Simulation, sp *core.Spell) {
		if !d.InForm(Cat) {
			a.Deactivate(sim)
		}
	}})
	d.RegisterSpell(Cat, core.SpellConfig{ActionID: action, Flags: core.SpellFlagAPL, Cast: core.CastConfig{CD: core.Cooldown{Timer: d.NewTimer(), Duration: 10 * time.Second}}, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool { return sim.CurrentTime <= 0 && !prowl.IsActive() }, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) { prowl.Activate(sim) }})
	stun := d.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		return t.ForeverControlAura("Forever Pounce-"+d.Label, core.ActionID{SpellID: 9827}, core.ForeverStun, 2*time.Second+time.Duration(500*d.fr("brutal-impact"))*time.Millisecond)
	})
	d.RegisterSpell(Cat, core.SpellConfig{ActionID: core.ActionID{SpellID: 9827}, SpellSchool: core.SpellSchoolPhysical, DefenseType: core.DefenseTypeMelee, ProcMask: core.ProcMaskMeleeMHSpecial, Flags: core.SpellFlagAPL | core.SpellFlagMeleeMetrics | core.SpellFlagIgnoreResists | SpellFlagBuilder, EnergyCost: core.EnergyCostOptions{Cost: 50, Refund: .8}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: time.Second}, IgnoreHaste: true}, DamageMultiplier: 1, ThreatMultiplier: 1, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool {
		return prowl.IsActive() && !d.PseudoStats.InFrontOfTarget && d.DistanceFromTarget <= 5
	}, Dot: core.DotConfig{Aura: core.Aura{Label: "Forever Pounce Bleed"}, NumberOfTicks: 6, TickLength: 3 * time.Second, DamageMultiplier: 1, OnSnapshot: func(sim *core.Simulation, t *core.Unit, dot *core.Dot, b bool) { dot.Snapshot(t, 25, b) }, OnTick: func(sim *core.Simulation, t *core.Unit, dot *core.Dot) {
		dot.CalcAndDealPeriodicSnapshotDamage(sim, t, dot.OutcomeTick)
	}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
		prowl.Deactivate(sim)
		result := sp.CalcAndDealOutcome(sim, t, sp.OutcomeMeleeSpecialHit)
		if result.Landed() {
			stun.Get(t).Activate(sim)
			sp.Dot(t).Apply(sim)
			d.AddComboPoints(sim, 1, t, sp.ComboPointMetrics())
		} else {
			sp.IssueRefund(sim)
		}
	}})
}
