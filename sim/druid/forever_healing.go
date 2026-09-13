package druid

import (
	"github.com/wowsims/classic/sim/core"
	"math"
	"time"
)

// Healing rank numbers/costs are Classic counterparts. The new talent effects
// apply independently of coefficients so beta client ranks can replace this table.
func (d *Druid) registerForeverHealing() {
	var rejuv, regrowth *DruidSpell
	for _, v := range []struct {
		id                   int32
		code                 int32
		mana, low, high, hot float64
		cast                 time.Duration
		ticks                int32
	}{{25297, foreverHealingTouch, 800, 2267, 2677, 0, 3500 * time.Millisecond, 0}, {25299, foreverRejuvenation, 360, 0, 0, 888, 0, 4}, {9858, foreverRegrowth, 880, 1003, 1119, 1064, 2 * time.Second, 7}} {
		v := v
		mana := v.mana
		cast := v.cast
		crit := 0.0
		multi := 1.0
		if v.code == foreverHealingTouch {
			mana *= 1 - .02*d.fr("tranquil-spirit")
			cast -= time.Duration(100*d.fr("naturalist")) * time.Millisecond
		}
		if v.code == foreverRejuvenation {
			multi *= 1 + .05*d.fr("improved-rejuvenation")
		}
		if v.code == foreverRegrowth {
			crit = 10 * d.fr("improved-regrowth") * core.SpellCritRatingPerCritChance
		}
		config := core.SpellConfig{ActionID: core.ActionID{SpellID: v.id}, SpellCode: v.code, SpellSchool: core.SpellSchoolNature, DefenseType: core.DefenseTypeMagic, ProcMask: core.ProcMaskSpellHealing, Flags: core.SpellFlagAPL | core.SpellFlagHelpful, ManaCost: core.ManaCostOptions{FlatCost: mana}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault, CastTime: cast}}, DamageMultiplier: multi, ThreatMultiplier: .5, BonusCoefficient: v.cast.Seconds() / 3.5, BonusCritRating: crit, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
			if d.IsOpponent(t) {
				t = &d.Unit
			}
			if v.low > 0 {
				sp.CalcAndDealHealing(sim, t, sim.Roll(v.low, v.high), sp.OutcomeHealingCrit)
			}
			if v.hot > 0 {
				sp.Hot(t).Apply(sim)
			}
		}}
		if v.code == foreverRejuvenation {
			config.Cast.DefaultCast.GCD -= time.Duration(500*d.fr("gift-of-the-earthmother")) * time.Millisecond
		}
		if v.hot > 0 {
			config.Hot = core.DotConfig{Aura: core.Aura{Label: "Forever " + core.Ternary(v.code == foreverRejuvenation, "Rejuvenation", "Regrowth")}, NumberOfTicks: v.ticks, TickLength: 3 * time.Second, DamageMultiplier: 1, OnSnapshot: func(sim *core.Simulation, t *core.Unit, dot *core.Dot, b bool) {
				dot.SnapshotBaseDamage = v.hot/float64(v.ticks) + .2*dot.Spell.HealingPower(t)
				dot.SnapshotAttackerMultiplier = dot.Spell.CasterHealingMultiplier() * dot.DamageMultiplier
			}, OnTick: func(sim *core.Simulation, t *core.Unit, dot *core.Dot) {
				dot.CalcAndDealPeriodicSnapshotHealing(sim, t, dot.OutcomeTick)
			}}
		}
		sp := d.RegisterSpell(Humanoid, config)
		if v.code == foreverRejuvenation {
			rejuv = sp
		}
		if v.code == foreverRegrowth {
			regrowth = sp
		}
	}
	if d.fr("swiftmend") > 0 {
		d.RegisterSpell(Humanoid, core.SpellConfig{ActionID: core.ActionID{SpellID: 18562}, SpellCode: foreverSwiftmend, SpellSchool: core.SpellSchoolNature, DefenseType: core.DefenseTypeMagic, ProcMask: core.ProcMaskSpellHealing, Flags: core.SpellFlagAPL | core.SpellFlagHelpful, ManaCost: core.ManaCostOptions{FlatCost: 162}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault - time.Duration(500*d.fr("gift-of-the-earthmother"))*time.Millisecond}, CD: core.Cooldown{Timer: d.NewTimer(), Duration: 15 * time.Second}}, DamageMultiplier: 1, ThreatMultiplier: .5, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool {
			if d.IsOpponent(t) {
				t = &d.Unit
			}
			return rejuv.Hot(t).IsActive() || regrowth.Hot(t).IsActive()
		}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
			if d.IsOpponent(t) {
				t = &d.Unit
			}
			hot := rejuv.Hot(t)
			if !hot.IsActive() {
				hot = regrowth.Hot(t)
			}
			// Consume the complete snapshotted periodic effect, including HoT-only talents.
			heal := hot.SnapshotBaseDamage * hot.SnapshotAttackerMultiplier * float64(hot.OriginalNumberOfTicks)
			oldFlags := sp.Flags
			sp.Flags |= core.SpellFlagIgnoreAttackerModifiers
			sp.CalcAndDealHealing(sim, t, heal, sp.OutcomeHealingCrit)
			sp.Flags = oldFlags
			hot.Deactivate(sim)
		}})
	}
	if d.fr("wild-growth") > 0 {
		d.RegisterSpell(Humanoid, core.SpellConfig{ActionID: d.fa("wild-growth"), SpellCode: foreverWildGrowth, SpellSchool: core.SpellSchoolNature, DefenseType: core.DefenseTypeMagic, ProcMask: core.ProcMaskSpellHealing, Flags: core.SpellFlagAPL | core.SpellFlagHelpful, ManaCost: core.ManaCostOptions{FlatCost: 550}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault - time.Duration(500*d.fr("gift-of-the-earthmother"))*time.Millisecond}, CD: core.Cooldown{Timer: d.NewTimer(), Duration: 6 * time.Second}}, DamageMultiplier: 1, ThreatMultiplier: .5, Hot: core.DotConfig{Aura: core.Aura{Label: "Forever Wild Growth"}, NumberOfTicks: 7, TickLength: time.Second, DamageMultiplier: 1, OnSnapshot: func(sim *core.Simulation, t *core.Unit, dot *core.Dot, b bool) {
			dot.SnapshotBaseDamage = 285 + .7*dot.Spell.HealingPower(t)
			dot.SnapshotAttackerMultiplier = dot.Spell.CasterHealingMultiplier() * dot.DamageMultiplier
		}, OnTick: func(sim *core.Simulation, t *core.Unit, dot *core.Dot) {
			base := dot.SnapshotBaseDamage
			dot.SnapshotBaseDamage = base * float64(8-dot.TickCount) / 28
			dot.CalcAndDealPeriodicSnapshotHealing(sim, t, dot.OutcomeTick)
			dot.SnapshotBaseDamage = base
		}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
			party := d.Env.Raid.GetPlayerParty(t)
			if party == nil || len(party.Players) == 0 {
				party = d.Party
			}
			for _, a := range party.Players {
				if math.Abs(a.GetCharacter().DistanceFromTarget-t.DistanceFromTarget) <= 43 {
					sp.Hot(&a.GetCharacter().Unit).Apply(sim)
				}
			}
		}})
	}
	// Tranquility's Classic 10-second channel is represented by five 2-second
	// party ticks and a real channel, retaining Classic threat/cooldown analogs.
	d.RegisterSpell(Humanoid, core.SpellConfig{ActionID: core.ActionID{SpellID: 9863}, SpellCode: foreverTranquility, SpellSchool: core.SpellSchoolNature, DefenseType: core.DefenseTypeMagic, ProcMask: core.ProcMaskSpellHealing, Flags: core.SpellFlagAPL | core.SpellFlagHelpful | core.SpellFlagChanneled, ManaCost: core.ManaCostOptions{FlatCost: 925 * (1 - .02*d.fr("tranquil-spirit"))}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: d.NewTimer(), Duration: time.Duration(float64(5*time.Minute) * (1 - .3*d.fr("improved-tranquility")))}}, DamageMultiplier: 1, ThreatMultiplier: .5 * (1 - .5*d.fr("improved-tranquility")), Hot: core.DotConfig{SelfOnly: true, Aura: core.Aura{Label: "Forever Tranquility"}, NumberOfTicks: 5, TickLength: 2 * time.Second, DamageMultiplier: 1, OnTick: func(sim *core.Simulation, t *core.Unit, dot *core.Dot) {
		for _, a := range d.Party.Players {
			dot.Spell.CalcAndDealHealing(sim, &a.GetCharacter().Unit, 294*dot.DamageMultiplier, dot.Spell.OutcomeHealing)
		}
	}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) { sp.SelfHot().Apply(sim) }})
	if d.fr("nature-s-swiftness") > 0 {
		var spells []*core.Spell
		d.OnSpellRegistered(func(sp *core.Spell) {
			if sp.SpellSchool.Matches(core.SpellSchoolNature) && sp.DefaultCast.CastTime > 0 {
				spells = append(spells, sp)
			}
		})
		a := d.RegisterAura(core.Aura{Label: "Forever Nature's Swiftness", ActionID: core.ActionID{SpellID: 17116}, Duration: core.NeverExpires, OnGain: func(_ *core.Aura, sim *core.Simulation) {
			for _, sp := range spells {
				sp.CastTimeMultiplier -= 1
			}
		}, OnExpire: func(_ *core.Aura, sim *core.Simulation) {
			for _, sp := range spells {
				sp.CastTimeMultiplier += 1
			}
		}, OnCastComplete: func(a *core.Aura, sim *core.Simulation, sp *core.Spell) {
			if sp.SpellSchool.Matches(core.SpellSchoolNature) && sp.DefaultCast.CastTime > 0 {
				a.Deactivate(sim)
			}
		}})
		d.RegisterSpell(Any, core.SpellConfig{ActionID: a.ActionID, Flags: core.SpellFlagAPL | core.SpellFlagHelpful, Cast: core.CastConfig{CD: core.Cooldown{Timer: d.NewTimer(), Duration: 3 * time.Minute}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) { a.Activate(sim) }})
	}
}
