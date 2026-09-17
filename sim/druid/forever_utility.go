package druid

import (
	"github.com/wowsims/classic/sim/core"
	"strings"
	"time"
)

func (d *Druid) registerForeverUtility() {
	d.registerForeverProwl()
	// Rank-6 Classic Roots supplies the provisional base damage; Overgrowth
	// replaces the oldest root when the selected capacity has been reached.
	var rooted []*core.Aura
	roots := d.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		return t.ForeverControlAura("Forever Entangling Roots-"+d.Label, core.ActionID{SpellID: 9853}, core.ForeverRoot, 27*time.Second)
	})
	rootSpell := d.RegisterSpell(Humanoid|Moonkin, core.SpellConfig{ForeverSingleTargetHarmful: true, ActionID: core.ActionID{SpellID: 9853}, SpellSchool: core.SpellSchoolNature, DefenseType: core.DefenseTypeMagic, ProcMask: core.ProcMaskSpellDamage, Flags: core.SpellFlagAPL | core.SpellFlagBinary, ManaCost: core.ManaCostOptions{FlatCost: 160}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault, CastTime: 1500 * time.Millisecond}}, DamageMultiplier: 1 + .25*d.fr("improved-entangling-roots"), ThreatMultiplier: 1, Dot: core.DotConfig{Aura: core.Aura{Label: "Forever Entangling Roots Damage"}, NumberOfTicks: 9, TickLength: 3 * time.Second, BonusCoefficient: .055, DamageMultiplier: 1, OnSnapshot: func(sim *core.Simulation, t *core.Unit, dot *core.Dot, b bool) { dot.Snapshot(t, 270.0/9, b) }, OnTick: func(sim *core.Simulation, t *core.Unit, dot *core.Dot) {
		if roots.Get(t).IsActive() {
			dot.CalcAndDealPeriodicSnapshotDamage(sim, t, dot.OutcomeTick)
		} else {
			dot.Deactivate(sim)
		}
	}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
		if !sp.CalcOutcome(sim, t, sp.OutcomeMagicHit).Landed() {
			return
		}
		active := rooted[:0]
		for _, a := range rooted {
			if a.IsActive() && a != roots.Get(t) {
				active = append(active, a)
			}
		}
		rooted = active
		if len(rooted) >= 1+int(d.fr("overgrowth")) {
			rooted[0].Deactivate(sim)
			rooted = rooted[1:]
		}
		roots.Get(t).Activate(sim)
		rooted = append(rooted, roots.Get(t))
		if roots.Get(t).IsActive() {
			sp.Dot(t).Apply(sim)
		}
	}})
	// PREDICTED Classic-like 20% break chance per non-root direct damage,
	// divided by 1.25 for the documented damage tolerance improvement.
	for _, t := range d.Env.Encounter.Targets {
		target := t
		core.MakePermanent(target.RegisterAura(core.Aura{Label: "Forever Roots Break-" + d.Label, OnSpellHitTaken: func(_ *core.Aura, sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
			if roots.Get(&target.Unit).IsActive() && sp != rootSpell.Spell && r.Damage > 0 && sim.Proc(d.ForeverParameter("roots_break_chance", .2)/(1+.25*core.TernaryFloat64(d.fr("improved-entangling-roots") > 0, 1, 0)), "Forever Roots Break") {
				roots.Get(&target.Unit).Deactivate(sim)
			}
		}}))
	}
	if d.fr("brutal-impact") > 0 || d.fr("feral-charge") > 0 {
		bash := d.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
			return t.ForeverControlAura("Forever Bash-"+d.Label, core.ActionID{SpellID: 8983}, core.ForeverStun, 4*time.Second+time.Duration(500*d.fr("brutal-impact"))*time.Millisecond)
		})
		d.RegisterSpell(Bear, core.SpellConfig{ActionID: core.ActionID{SpellID: 8983}, SpellSchool: core.SpellSchoolPhysical, DefenseType: core.DefenseTypeMelee, Flags: core.SpellFlagAPL, RageCost: core.RageCostOptions{Cost: 10}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: d.NewTimer(), Duration: 60*time.Second - time.Duration(15*d.fr("brutal-impact"))*time.Second}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
			if sp.CalcOutcome(sim, t, sp.OutcomeMeleeSpecialHit).Landed() {
				bash.Get(t).Activate(sim)
			}
		}})
	}
	if d.fr("feral-charge") > 0 {
		catAuras := d.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
			return t.ForeverControlAura("Forever Cat Charge-"+d.Label, d.fa("feral-charge"), core.ForeverSnare, 3*time.Second)
		})
		auras := d.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
			return t.ForeverControlAura("Forever Feral Charge-"+d.Label, d.fa("feral-charge"), core.ForeverRoot, 4*time.Second)
		})
		rageMetrics := d.NewRageMetrics(d.fa("feral-charge"))
		d.RegisterSpell(Bear|Cat, core.SpellConfig{ActionID: d.fa("feral-charge"), Flags: core.SpellFlagAPL, Cast: core.CastConfig{CD: core.Cooldown{Timer: d.NewTimer(), Duration: 15 * time.Second}}, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool {
			return d.DistanceFromTarget >= 8 && d.DistanceFromTarget <= 25 && (!d.InForm(Bear) || d.CurrentRage() >= 5)
		}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
			d.DistanceFromTarget = 0
			if d.InForm(Bear) {
				d.SpendRage(sim, 5, rageMetrics)
				t.ForeverInterruptSchool(sim, 4*time.Second)
				auras.Get(t).Activate(sim)
			} else {
				sp.CD.Timer.Set(sim.CurrentTime + 30*time.Second)
				d.PseudoStats.InFrontOfTarget = false
				catAuras.Get(t).Activate(sim)
			}
		}})
	}
}

// Shapeshifting removes movement restrictions and Polymorph, as on Classic.
// Other incapacitates remain in place; their names must not be generalized.
func (d *Druid) foreverShiftDispel(sim *core.Simulation) {
	for _, aura := range d.GetAuras() {
		if aura.IsActive() && (aura.Tag == "forever-control-root" || aura.Tag == "forever-control-snare" || strings.Contains(strings.ToLower(aura.Label), "polymorph")) {
			aura.Deactivate(sim)
		}
	}
}
