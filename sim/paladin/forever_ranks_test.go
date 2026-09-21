package paladin_test

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
)

// Per-rank Forever values from the beta client tooltips (tf/data.json spell_desc).

func mergeTalents(maps ...map[string]int32) map[string]int32 {
	out := map[string]int32{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = max(out[k], v)
		}
	}
	return out
}

func TestForeverHolyStrikeRanks(t *testing.T) {
	for level, cost := range map[int32]float64{6: 5, 20: 12, 44: 17, 60: 20} {
		c := hybridSimAtLevel(t, "PALADIN", map[string]int32{}, level)
		hs := c.GetSpell(c.ForeverAction("paladin.baseline.holy-strike"))
		if hs == nil {
			t.Fatalf("level %d: Holy Strike missing", level)
		}
		if hs.DefaultCast.Cost != cost || hs.CD.Duration != 12*time.Second || hs.BonusCoefficient != .429 {
			t.Errorf("level %d: Holy Strike cost %v cd %v coef %v, want %v / 12s / 0.429", level, hs.DefaultCast.Cost, hs.CD.Duration, hs.BonusCoefficient, cost)
		}
	}
	if c := hybridSimAtLevel(t, "PALADIN", map[string]int32{}, 5); c.GetSpell(c.ForeverAction("paladin.baseline.holy-strike")) != nil {
		t.Error("Holy Strike is learned at level 6")
	}
	_, c := hybridSim(t, "PALADIN", hybridTalents(t, "paladin.talent.improved-holy-strike", 2), proto.ForeverMode_BEST_GUESS)
	if hs := c.GetSpell(c.ForeverAction("paladin.baseline.holy-strike")); hs == nil || hs.CD.Duration != 10*time.Second {
		t.Error("Improved Holy Strike 2/2 should give a 10 sec cooldown")
	}
}

func TestForeverSealOfCommandRanksAndJudgement(t *testing.T) {
	talents := mergeTalents(hybridTalents(t, "paladin.talent.seal-of-command", 1), hybridTalents(t, "paladin.talent.sanctified-judgement", 3))
	for level, cost := range map[int32]float64{20: 65, 30: 110, 40: 140, 50: 180, 60: 210} {
		c := hybridSimAtLevel(t, "PALADIN", talents, level)
		var top *core.Spell
		for _, id := range []int32{20375, 20915, 20918, 20919, 20920} {
			if sp := c.GetSpell(core.ActionID{SpellID: id}); sp != nil {
				top = sp
			}
		}
		// The Retribution prerequisites include Benediction, so compare the pre-discount cost.
		if top == nil || math.Abs(top.DefaultCast.Cost*100/float64(top.Cost.Multiplier)-cost) > .001 {
			t.Errorf("level %d: Seal of Command top rank %v, want base cost %v", level, top.ActionID, cost)
		}
	}

	sim, c := hybridSim(t, "PALADIN", talents, proto.ForeverMode_BEST_GUESS)
	target := sim.Encounter.TargetUnits[0]
	seal := c.GetSpell(core.ActionID{SpellID: 20920})
	judgement := c.GetSpell(core.ActionID{SpellID: 20271})
	seal.ApplyEffects(sim, &c.Unit, seal)
	aura := c.GetAuraByID(core.ActionID{SpellID: 20920})
	c.SpendMana(sim, 5000, c.NewManaMetrics(judgement.ActionID))
	before := c.CurrentMana()
	judgement.ApplyEffects(sim, target, judgement)
	if !aura.IsActive() {
		t.Error("Forever Judgement must not consume the Seal")
	}
	// Sanctified Judgement 3/3: 100% chance to return 60% of the judged seal's Mana cost.
	if got, want := c.CurrentMana()-before, .6*seal.DefaultCast.Cost; math.Abs(got-want) > .001 {
		t.Errorf("Sanctified Judgement 3/3 returned %v Mana, want %v", got, want)
	}
}

func TestForeverHolyShockAndVigilRanks(t *testing.T) {
	talents := hybridTalents(t, "paladin.talent.light-s-vigil", 1)
	for level, want := range map[int32][2]float64{30: {160, 0}, 40: {225, 730}, 48: {275, 730}, 50: {275, 1000}, 60: {325, 1340}} {
		c := hybridSimAtLevel(t, "PALADIN", talents, level)
		shock := c.GetSpell(c.ForeverAction("paladin.talent.holy-shock"))
		if shock == nil || shock.DefaultCast.Cost != want[0] {
			t.Errorf("level %d: Holy Shock cost, want %v", level, want[0])
		}
		if want[1] == 0 {
			continue
		}
		if vigil := c.GetSpell(c.ForeverAction("paladin.talent.light-s-vigil")); vigil == nil || vigil.DefaultCast.Cost != want[1] {
			t.Errorf("level %d: Light's Vigil cost, want %v", level, want[1])
		}
	}
}

func TestForeverHammerOfWrathTalents(t *testing.T) {
	talents := mergeTalents(hybridTalents(t, "paladin.talent.holy-power", 5), hybridTalents(t, "paladin.talent.divine-precision", 3))
	talents["paladin.talent.purifying-power"] = 1
	_, c := hybridSim(t, "PALADIN", talents, proto.ForeverMode_BEST_GUESS)
	how := c.GetSpell(core.ActionID{SpellID: 24239})
	if how == nil {
		t.Fatal("Hammer of Wrath missing")
	}
	hybridNear(t, how.BonusCritRating, 5*core.CritRatingPerCritChance)
	hybridNear(t, how.BonusHitRating, 18*core.MeleeHitRatingPerHitChance)
	// Purifying Power 1/2: 17% off Exorcism's 15 sec.
	if ex := c.GetSpell(core.ActionID{SpellID: 10314}); ex == nil || math.Abs(ex.CD.Duration.Seconds()-15*.83) > .001 {
		t.Errorf("Purifying Power 1/2 Exorcism cooldown %v", ex.CD.Duration)
	}

	talents = mergeTalents(hybridTalents(t, "paladin.talent.instrument-of-law", 2))
	talents["paladin.talent.benediction"] = 5
	_, c = hybridSim(t, "PALADIN", talents, proto.ForeverMode_BEST_GUESS)
	how = c.GetSpell(core.ActionID{SpellID: 24239})
	want := 100 - 2*c.ForeverRank("paladin.talent.benediction") - 20*c.ForeverRank("paladin.talent.holy-conduit")
	if how.DefaultCast.CastTime != 0 || how.Cost.Multiplier != want {
		t.Errorf("Instrument of Law 2/2 makes Hammer of Wrath instant, so Benediction 5/5 applies: cast %v, cost multiplier %v", how.DefaultCast.CastTime, how.Cost.Multiplier)
	}
}

func TestForeverLavaBurstRanksAndFireNova(t *testing.T) {
	talents := hybridTalents(t, "shaman.talent.lava-burst", 1)
	for level, cost := range map[int32]float64{40: 165, 50: 230, 60: 265} {
		c := hybridSimAtLevel(t, "SHAMAN", talents, level)
		lava := c.GetSpell(c.ForeverAction("shaman.talent.lava-burst"))
		convection := float64(c.ForeverRank("shaman.talent.convection"))
		if lava == nil || math.Abs(lava.DefaultCast.Cost-cost*(1-.02*convection)) > .001 {
			t.Errorf("level %d: Lava Burst cost %v, want %v before Convection", level, lava.DefaultCast.Cost, cost)
		}
	}
	for _, mode := range []proto.ForeverMode{proto.ForeverMode_STRICT, proto.ForeverMode_BEST_GUESS} {
		_, c := hybridSim(t, "SHAMAN", map[string]int32{}, mode)
		if nova := c.GetSpell(c.ForeverAction("shaman.baseline.fire-nova")); nova == nil || nova.CD.Duration != 10*time.Second || nova.DefaultCast.Cost != 520 {
			t.Errorf("%v: Fire Nova should be rank 5 with a 10 sec cooldown", mode)
		}
	}
	_, c := hybridSim(t, "SHAMAN", hybridTalents(t, "shaman.talent.improved-fire-nova", 2), proto.ForeverMode_BEST_GUESS)
	if nova := c.GetSpell(c.ForeverAction("shaman.baseline.fire-nova")); nova == nil || nova.CD.Duration != 6*time.Second {
		t.Error("Improved Fire Nova 2/2 should give a 6 sec cooldown")
	}
}

func TestForeverLightningOverloadChanceAndMaelstrom(t *testing.T) {
	// Lightning Overload (row 5) with Maelstrom Weapon (row 6) must construct: the overload
	// copies carry no mana cost.
	talents := mergeTalents(hybridTalents(t, "shaman.talent.lightning-overload", 1), hybridTalents(t, "shaman.talent.maelstrom-weapon", 5))
	sim, c := hybridSim(t, "SHAMAN", talents, proto.ForeverMode_BEST_GUESS)
	mw := c.GetAura("Forever Maelstrom Weapon")
	mw.Activate(sim)
	mw.SetStacks(sim, 5)
	mw.Deactivate(sim)

	sim, c = hybridSim(t, "SHAMAN", hybridTalents(t, "shaman.talent.lightning-overload", 3), proto.ForeverMode_BEST_GUESS)
	c.AddStatDynamic(sim, stats.SpellHit, 100)

	target := sim.Encounter.TargetUnits[0]
	lb := c.GetSpell(core.ActionID{SpellID: 15208})
	overload := c.GetSpell(core.ActionID{SpellID: 15208, Tag: 1})
	if lb == nil || overload == nil {
		t.Fatal("missing Lightning Bolt or its overload")
	}
	const casts = 20000
	for i := 0; i < casts; i++ {
		lb.ApplyEffects(sim, target, lb)
	}
	for sim.CurrentTime < 30*time.Second {
		if sim.Step() {
			break
		}
	}
	rate := float64(overload.SpellMetrics[target.UnitIndex].Casts) / casts
	if math.Abs(rate-.10) > .01 {
		t.Errorf("Lightning Overload 3/3 proc rate %.4f, want 10%%", rate)
	}
}

func TestForeverStormstrikeEmpowersOnlyNextSpell(t *testing.T) {
	sim, c := hybridSim(t, "SHAMAN", hybridTalents(t, "shaman.talent.stormstrike", 1), proto.ForeverMode_BEST_GUESS)
	c.AddStatDynamic(sim, stats.SpellHit, 100)
	c.AddStatDynamic(sim, stats.MeleeHit, 100)
	target := sim.Encounter.TargetUnits[0]
	ss := c.GetSpell(core.ActionID{SpellID: 17364})
	debuff := target.GetAura("Forever Stormstrike-" + c.Label)
	for i := 0; i < 20 && !debuff.IsActive(); i++ {
		ss.ApplyEffects(sim, target, ss)
	}
	if !debuff.IsActive() {
		t.Fatal("Stormstrike never applied its debuff")
	}
	var es *core.Spell
	for _, id := range []int32{10414, 10413, 10412} {
		if es = c.GetSpell(core.ActionID{SpellID: id}); es != nil {
			break
		}
	}
	at := c.AttackTables[target.UnitIndex][proto.CastType_CastTypeMainHand]
	hybridNear(t, at.DamageDoneByCasterMultiplier(es, at), 1.2)
	es.ApplyEffects(sim, target, es)
	if debuff.IsActive() {
		t.Error("Earth Shock should use up Stormstrike")
	}
	hybridNear(t, at.DamageDoneByCasterMultiplier(es, at), 1)
}
