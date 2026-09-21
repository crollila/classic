package warrior_test

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
	"github.com/wowsims/classic/sim/rogue"
)

// Forever per-rank numbers checked against the beta client's tooltips (talentsforever.com
// export of the client). The characters are unarmed (0-0 weapon damage), so weapon strikes
// deal only their flat bonus plus attack power / 14 x normalized speed (1).

// dealtBase casts once with a forced non-crit hit and returns the damage divided by the
// spell's full multiplier chain (armor, stance, talents), i.e. the base damage the ability
// computed before multipliers.
func dealtBase(t *testing.T, sim *core.Simulation, c *core.Character, s *core.Spell, cast func()) float64 {
	t.Helper()
	target := c.CurrentTarget
	s.BonusHitRating, s.BonusCritRating = 1000, -1000
	c.AddStatDynamic(sim, stats.Expertise, 100)
	before := s.SpellMetrics[target.UnitIndex].TotalDamage
	cast()
	dealt := s.SpellMetrics[target.UnitIndex].TotalDamage - before
	unit := s.CalcDamage(sim, target, 1, s.OutcomeAlwaysHit).Damage
	if unit <= 0 || dealt <= 0 {
		t.Fatalf("no damage: dealt %v, unit %v", dealt, unit)
	}
	return dealt / unit
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestForeverWarriorLearnedRankDamage(t *testing.T) {
	sim, c := physicalSim(t, "Warrior", "warrior", physicalBuild(t, "warrior.talent.mortal-strike"))
	ms := physicalID(t, c, 21553)
	ap := ms.MeleeAttackPower(c.CurrentTarget)
	if got := dealtBase(t, sim, c, ms, func() { ms.ApplyEffects(sim, c.CurrentTarget, ms) }); !near(got, 160+ap/14) {
		t.Fatalf("Mortal Strike rank 4 base %v, want weapon + 160 (%v)", got, 160+ap/14)
	}

	sim, c = physicalSim(t, "Warrior", "warrior", physicalBuild(t, "warrior.talent.bloodthirst"))
	bt := physicalID(t, c, 23894)
	ap = bt.MeleeAttackPower(c.CurrentTarget)
	if got := dealtBase(t, sim, c, bt, func() { bt.ApplyEffects(sim, c.CurrentTarget, bt) }); !near(got, .35*ap+48) {
		t.Fatalf("Bloodthirst rank 4 base %v, want 35%% AP + 48 (%v)", got, .35*ap+48)
	}

	sim, c = physicalSim(t, "Warrior", "warrior", physicalBuild(t, "warrior.talent.shield-slam"))
	ss := physicalID(t, c, 23925)
	bv := c.BlockValue()
	for i := 0; i < 20; i++ {
		if got := dealtBase(t, sim, c, ss, func() { ss.ApplyEffects(sim, c.CurrentTarget, ss) }); got < 640+bv-1e-6 || got > 670+bv+1e-6 {
			t.Fatalf("Shield Slam rank 4 base %v, want 640-670 + Block Value %v", got, bv)
		}
	}

	sim, c = physicalSim(t, "Warrior", "warrior", physicalBuild(t, "warrior.talent.improved-revenge"))
	revenge := physicalID(t, c, 25288)
	for i := 0; i < 20; i++ {
		if got := dealtBase(t, sim, c, revenge, func() { revenge.ApplyEffects(sim, c.CurrentTarget, revenge) }); got < 138-1e-6 || got > 168+1e-6 {
			t.Fatalf("Revenge rank 6 base %v, want 138-168", got)
		}
	}
}

func TestForeverWarriorSlamThunderClapCooldowns(t *testing.T) {
	_, c := physicalSim(t, "Warrior", "warrior", map[string]int32{})
	if slam := physicalID(t, c, 11605); slam.CD.Duration != 15*time.Second {
		t.Fatal("Forever Slam cooldown", slam.CD.Duration)
	}
	if tc := physicalID(t, c, 11581); tc.CD.Duration != 6*time.Second {
		t.Fatal("Forever Thunder Clap cooldown", tc.CD.Duration)
	}
	// Classic keeps Slam without a cooldown and Thunder Clap at 4 sec.
	_, classic := physicalSim(t, "Warrior", "warrior", map[string]int32{}, func(p *proto.Player) { p.Forever = nil })
	if slam := physicalID(t, classic, 11605); slam.CD.Timer != nil {
		t.Fatal("Classic Slam gained a cooldown")
	}
	if tc := physicalID(t, classic, 11581); tc.CD.Duration != 4*time.Second {
		t.Fatal("Classic Thunder Clap cooldown", tc.CD.Duration)
	}
}

func TestForeverSpearingStrikeRequiresTwoHander(t *testing.T) {
	sim, c := physicalSim(t, "Warrior", "warrior", physicalBuild(t, "warrior.talent.spearing-strike"))
	c.AddRage(sim, 100, c.NewRageMetrics(core.ActionID{SpellID: 1}))
	spear := physicalSpell(t, c, "warrior.talent.spearing-strike")
	for _, tc := range []struct {
		hand proto.HandType
		ok   bool
	}{{proto.HandType_HandTypeMainHand, false}, {proto.HandType_HandTypeOneHand, false}, {proto.HandType_HandTypeTwoHand, true}} {
		c.MainHand().HandType = tc.hand
		if got := spear.CanCast(sim, c.CurrentTarget); got != tc.ok {
			t.Fatalf("Spearing Strike castable %v with a %v main hand", got, tc.hand)
		}
	}
}

func TestForeverInterceptDamageAndFocusedRage(t *testing.T) {
	build := physicalBuild(t, "warrior.talent.focused-rage")
	sim, c := physicalSim(t, "Warrior", "warrior", build)
	intercept := physicalID(t, c, 20617)
	if intercept.DefaultCast.Cost != 10-float64(build["warrior.talent.focused-rage"]) {
		t.Fatal("Focused Rage does not reduce Intercept", intercept.DefaultCast.Cost)
	}
	if got := dealtBase(t, sim, c, intercept, func() { intercept.ApplyEffects(sim, c.CurrentTarget, intercept) }); !near(got, 65) {
		t.Fatalf("Intercept rank 3 damage %v, want 65", got)
	}
}

func TestForeverRogueCritAndCostTalents(t *testing.T) {
	_, c := physicalSim(t, "Rogue", "rogue", physicalBuild(t, "rogue.talent.lethality"))
	if bs := physicalID(t, c, 25300); !near(bs.CritDamageBonus, 1.2) {
		t.Fatal("Lethality 5/5 should add 20% crit damage", bs.CritDamageBonus)
	}
	_, c = physicalSim(t, "Rogue", "rogue", map[string]int32{})
	if evis := physicalID(t, c, 31016); evis.DefaultCast.Cost != 35 {
		t.Fatal("Eviscerate base cost", evis.DefaultCast.Cost)
	}
	// Flawless Execution sits in the slot the ruleset still names Restless Blades.
	_, c = physicalSim(t, "Rogue", "rogue", physicalBuild(t, "rogue.talent.flawless-execution"))
	if evis := physicalID(t, c, 31016); evis.DefaultCast.Cost != 25 {
		t.Fatal("Flawless Execution: Eviscerate cost", evis.DefaultCast.Cost)
	}
}

func TestForeverImprovedExposeArmorRefundsTwo(t *testing.T) {
	sim, c := physicalSim(t, "Rogue", "rogue", physicalBuild(t, "rogue.talent.improved-expose-armor"))
	ea := physicalID(t, c, 11198)
	ea.BonusHitRating = 1000
	c.AddStatDynamic(sim, stats.Expertise, 100)
	if ea.DefaultCast.Cost != 15 {
		t.Fatal("Improved Expose Armor 2/2 cost", ea.DefaultCast.Cost)
	}
	c.AddComboPoints(sim, 5, c.CurrentTarget, ea.ComboPointMetrics())
	armor := c.CurrentTarget.GetStat(stats.Armor)
	if !ea.Cast(sim, c.CurrentTarget) {
		t.Fatal("Expose Armor failed")
	}
	if c.ComboPoints() < 2 {
		t.Fatal("rank 2 refunds 2 combo points, have", c.ComboPoints())
	}
	if got := armor - c.CurrentTarget.GetStat(stats.Armor); !near(got, 2250) {
		t.Fatal("Forever Expose Armor rank 5 at 5 points removes 2250 armor, got", got)
	}
}

func TestForeverRogueFinisherAndPoisonValues(t *testing.T) {
	sim, c := physicalSim(t, "Rogue", "rogue", map[string]int32{}, func(p *proto.Player) {
		p.Consumes = &proto.Consumes{MainHandImbue: proto.WeaponImbue_InstantPoison, OffHandImbue: proto.WeaponImbue_DeadlyPoison}
	})
	r := c.Env.Raid.Parties[0].Players[0].(rogue.RogueAgent).GetRogue()
	target := c.CurrentTarget
	ap := r.Rupture.MeleeAttackPower(target)
	if got := r.RuptureDamage(target, 5) - .24/8*ap; !near(got, 35+5*4.73) {
		t.Fatal("Forever Rupture rank 6 5-point tick", got)
	}
	// Instant Poison VI: 76-100 Nature. Spells keep a 1% miss and can partially resist, so
	// only full, unresisted hits are measured.
	ip := physicalID(t, c, 11340)
	ip.BonusCritRating = -1000
	measured := 0
	for i := 0; i < 50; i++ {
		m := ip.SpellMetrics[target.UnitIndex]
		before, resisted := m.TotalDamage, m.ResistedHits
		ip.ApplyEffects(sim, target, ip)
		m = ip.SpellMetrics[target.UnitIndex]
		if m.TotalDamage == before || m.ResistedHits != resisted {
			continue
		}
		measured++
		got := (m.TotalDamage - before) / ip.CalcDamage(sim, target, 1, ip.OutcomeAlwaysHit).Damage
		if got < 76-1e-6 || got > 100+1e-6 {
			t.Fatal("Forever Instant Poison VI", got)
		}
	}
	if measured == 0 {
		t.Fatal("no Instant Poison hit measured")
	}
}

func TestForeverVenomReadsDeadlyPoisonAtTickTime(t *testing.T) {
	sim, c := physicalSim(t, "Rogue", "rogue", physicalBuild(t, "rogue.talent.venom"), func(p *proto.Player) {
		p.Consumes = &proto.Consumes{MainHandImbue: proto.WeaponImbue_DeadlyPoison, OffHandImbue: proto.WeaponImbue_DeadlyPoison}
	})
	target := c.CurrentTarget
	var apply, tick *core.Spell
	for _, s := range c.Spellbook {
		if s.SpellID == 25347 && s.Tag == 100 {
			tick = s
		} else if s.SpellID == 25347 {
			apply = s
		}
	}
	if apply == nil || tick == nil {
		t.Fatal("no Deadly Poison V")
	}
	apply.BonusHitRating = 1000
	venom := c.GetAura("Venom")
	dot := tick.Dot(target)
	tickDamage := func() float64 {
		before := tick.SpellMetrics[target.UnitIndex].TotalDamage
		dot.TickOnce(sim)
		return tick.SpellMetrics[target.UnitIndex].TotalDamage - before
	}

	// Stack applied while Venom is up; Venom then ends: the ticks must lose the 30%.
	venom.Activate(sim)
	apply.ApplyEffects(sim, target, apply)
	if dot.SnapshotBaseDamage != 23 {
		t.Fatal("Forever Deadly Poison V tick", dot.SnapshotBaseDamage)
	}
	during := tickDamage()
	venom.Deactivate(sim)
	after := tickDamage()
	if !near(during/after, 1.3) {
		t.Fatal("Venom stuck to, or missed, the Deadly Poison stack", during, after)
	}
	// And a stack that was already running picks Venom up.
	venom.Activate(sim)
	if again := tickDamage(); !near(again/after, 1.3) {
		t.Fatal("Venom ignored a running Deadly Poison stack", again, after)
	}
}

func TestForeverAmbushRequiresBehind(t *testing.T) {
	sim, c := physicalSim(t, "Rogue", "rogue", map[string]int32{}, func(p *proto.Player) {
		p.Equipment = core.GetGearSet("../../ui/rogue/gear_sets", "combat_backstab_prebis").GearSet
	})
	r := c.Env.Raid.Parties[0].Players[0].(rogue.RogueAgent).GetRogue()
	r.StealthAura.Activate(sim)
	if !r.Ambush.CanCast(sim, c.CurrentTarget) {
		t.Fatal("stealthed Ambush from behind failed")
	}
	c.PseudoStats.InFrontOfTarget = true
	if r.Ambush.CanCast(sim, c.CurrentTarget) {
		t.Fatal("Ambush castable in front of the target")
	}
}


func TestForeverMutilateUsesLearnedRank(t *testing.T) {
	for _, tc := range []struct {
		level int32
		bonus float64
	}{{60, 38}, {50, 27}, {40, 19}} {
		sim, c := physicalSim(t, "Rogue", "rogue", physicalBuild(t, "rogue.talent.mutilate"), func(p *proto.Player) { p.Level = tc.level })
		tag := c.ForeverAction("rogue.talent.mutilate").Tag
		var mh *core.Spell
		for _, s := range c.Spellbook {
			if s.OtherID == proto.OtherAction_OtherActionForever && s.Tag == -tag*10 {
				mh = s
			}
		}
		if mh == nil {
			t.Fatal("no Mutilate main-hand strike at level", tc.level)
		}
		ap := mh.MeleeAttackPower(c.CurrentTarget)
		if got, want := dealtBase(t, sim, c, mh, func() { mh.ApplyEffects(sim, c.CurrentTarget, mh) }), .75*ap/14+tc.bonus; !near(got, want) {
			t.Fatalf("level %d Mutilate main hand %v, want 75%% weapon + %v (%v)", tc.level, got, tc.bonus, want)
		}
	}
}
