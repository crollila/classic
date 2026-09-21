package paladin_test

import (
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/simsignals"
	"github.com/wowsims/classic/sim/core/stats"
	"github.com/wowsims/classic/sim/druid"
	"math"
	"testing"
	"time"
)

func hybridNear(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("got %.9f, want %.9f", got, want)
	}
}

func TestForeverAuditDamageBonusesExcludeHealing(t *testing.T) {
	for _, mode := range []proto.ForeverMode{proto.ForeverMode_STRICT, proto.ForeverMode_BEST_GUESS} {
		t.Run(mode.String(), func(t *testing.T) {
			talents := hybridTalents(t, "druid.talent.moonfury", 1)
			_, c := hybridSim(t, "DRUID", talents, mode)
			heal := c.GetSpell(core.ActionID{SpellID: 25297})
			var wrath *core.Spell
			for _, sp := range c.Spellbook {
				if sp.SpellCode == druid.SpellCode_DruidWrath {
					wrath = sp
				}
			}
			hybridNear(t, heal.DamageMultiplierAdditive, 1)
			if wrath.DamageMultiplierAdditive <= 1 {
				t.Fatal("Moonfury failed to boost damage")
			}
			if c.GetAura("Forever Balance Nature") != nil {
				t.Fatal("Balance of Nature is not a Forever client talent")
			}
		})
	}
}

func TestForeverAuditSealEchoAndHolyShield(t *testing.T) {
	for _, mode := range []proto.ForeverMode{proto.ForeverMode_STRICT, proto.ForeverMode_BEST_GUESS} {
		t.Run(mode.String(), func(t *testing.T) {
			sim, c := hybridSim(t, "PALADIN", hybridTalents(t, "paladin.talent.twist-of-light", 1), mode)
			target := sim.Encounter.TargetUnits[0]
			sor := c.GetSpell(core.ActionID{SpellID: 20293})
			crusader := c.GetSpell(core.ActionID{SpellID: 20308})
			sor.ApplyEffects(sim, target, sor)
			crusader.ApplyEffects(sim, target, crusader)
			echo := c.GetAura("Forever Seal Echo")
			if !echo.IsActive() {
				t.Fatal("seal replacement did not create echo")
			}
			proc := c.GetSpell(core.ActionID{SpellID: 25713})
			before := proc.SpellMetrics[target.UnitIndex].TotalDamage
			special := &core.Spell{Unit: &c.Unit, ProcMask: core.ProcMaskMeleeMHSpecial}
			c.OnSpellHitDealt(sim, special, &core.SpellResult{Target: target, Outcome: core.OutcomeHit})
			if echo.IsActive() || proc.SpellMetrics[target.UnitIndex].TotalDamage <= before {
				t.Fatal("special attack did not deliver Righteousness echo")
			}
			sor.ApplyEffects(sim, target, sor)
			if echo.IsActive() {
				t.Fatal("Crusader incorrectly created Twist of Light payload")
			}
			justice := c.GetSpell(core.ActionID{SpellID: 20164})
			if mode == proto.ForeverMode_STRICT && justice != nil {
				t.Fatal("predicted Justice baseline leaked into STRICT")
			}
			if mode == proto.ForeverMode_BEST_GUESS {
				if justice == nil {
					t.Fatal("Justice not available")
				}
				if c.Forever.Parameters == nil {
					c.Forever.Parameters = map[string]float64{}
				}
				c.Forever.Parameters["seal_of_justice_ppm"] = 60
				c.AutoAttacks.MH().SwingSpeed = 1
				justice.ApplyEffects(sim, target, justice)
				sor.ApplyEffects(sim, target, sor)
				c.OnSpellHitDealt(sim, special, &core.SpellResult{Target: target, Outcome: core.OutcomeHit})
				if !target.ForeverControlled(core.ForeverStun) {
					t.Fatal("Justice echo did not apply stun")
				}
			}
			sim, c = hybridSim(t, "PALADIN", hybridTalents(t, "paladin.talent.holy-shield", 1), mode)
			first := c.GetSpell(core.ActionID{SpellID: 20925})
			last := c.GetSpell(core.ActionID{SpellID: 20928})
			if first.CD.Timer != last.CD.Timer {
				t.Fatal("Holy Shield ranks do not share cooldown")
			}
			block := c.GetStat(stats.Block)
			first.ApplyEffects(sim, &c.Unit, first)
			last.ApplyEffects(sim, &c.Unit, last)
			hybridNear(t, c.GetStat(stats.Block)-block, 20)
		})
	}
}

func TestForeverAuditVigilAttributionAndIllumination(t *testing.T) {
	for _, mode := range []proto.ForeverMode{proto.ForeverMode_STRICT, proto.ForeverMode_BEST_GUESS} {
		t.Run(mode.String(), func(t *testing.T) {
			talents := hybridTalents(t, "paladin.talent.light-s-vigil", 1)
			talents["paladin.talent.illumination"] = 5
			sim, c := hybridSim(t, "PALADIN", talents, mode)
			c.AddStatDynamic(sim, stats.SpellCrit, 100)
			action := c.ForeverAction("paladin.talent.light-s-vigil")
			action.Tag = -action.Tag
			effect := c.GetSpell(action)
			shock := c.GetSpell(c.ForeverAction("paladin.talent.holy-shock"))
			if effect == nil {
				t.Fatal("missing distinct Vigil effect")
			}
			hybridNear(t, shock.BonusCritRating-effect.BonusCritRating, 2*float64(c.ForeverRank("paladin.talent.holy-power")))
			refunds := 0
			for i := 0; i < 100; i++ {
				c.SpendMana(sim, 1000, c.NewManaMetrics(effect.ActionID))
				before := c.CurrentMana()
				effect.ApplyEffects(sim, &c.Unit, effect)
				delta := c.CurrentMana() - before
				if delta > 0 {
					hybridNear(t, delta, 670) // level-60 rank 3: 1,340 Mana x 50%
					refunds++
				}
			}
			if refunds == 0 {
				t.Fatal("Illumination never refunded Vigil")
			}
			if effect.SpellMetrics[c.UnitIndex].Crits != 100 {
				t.Fatal("Vigil crit not attributed to its own action")
			}
		})
	}
}

func TestForeverAuditFullHealingMultipliers(t *testing.T) {
	for _, mode := range []proto.ForeverMode{proto.ForeverMode_STRICT, proto.ForeverMode_BEST_GUESS} {
		t.Run(mode.String(), func(t *testing.T) {
			talents := hybridTalents(t, "druid.talent.swiftmend", 1)
			talents["druid.talent.improved-rejuvenation"] = 3
			talents["druid.talent.genesis"] = 5
			sim, c := hybridSim(t, "DRUID", talents, mode)
			c.AddStatDynamic(sim, stats.SpellCrit, -100)
			rejuv := c.GetSpell(core.ActionID{SpellID: 25299})
			swift := c.GetSpell(core.ActionID{SpellID: 18562})
			rejuv.ApplyEffects(sim, &c.Unit, rejuv)
			hot := rejuv.Hot(&c.Unit)
			expected := hot.SnapshotBaseDamage * hot.SnapshotAttackerMultiplier * float64(hot.OriginalNumberOfTicks)
			swift.ApplyEffects(sim, &c.Unit, swift)
			hybridNear(t, swift.SpellMetrics[c.UnitIndex].TotalHealing, expected)
			if hot.IsActive() {
				t.Fatal("Swiftmend did not consume HoT")
			}
			tranq := c.GetSpell(core.ActionID{SpellID: 9863})
			tranq.ApplyEffects(sim, &c.Unit, tranq)
			tranq.SelfHot().TickOnce(sim)
			hybridNear(t, tranq.SpellMetrics[c.UnitIndex].TotalHealing, 294*tranq.CasterHealingMultiplier()*(1+.01*float64(c.ForeverRank("druid.talent.genesis"))))
			talents = hybridTalents(t, "shaman.talent.riptide", 1)
			totals := []float64{}
			for _, buffed := range []bool{false, true} {
				sim, c := hybridSim(t, "SHAMAN", talents, mode)
				c.AddStatDynamic(sim, stats.SpellCrit, -100)
				c.AddStatDynamic(sim, stats.HealingPower, 1000)
				riptide := c.GetSpell(c.ForeverAction("shaman.talent.riptide"))
				riptide.ApplyEffects(sim, &c.Unit, riptide)
				if !buffed {
					riptide.Hot(&c.Unit).Deactivate(sim)
				}
				chain := c.GetSpell(core.ActionID{SpellID: 10623})
				chain.ApplyEffects(sim, &c.Unit, chain)
				totals = append(totals, chain.SpellMetrics[c.UnitIndex].TotalHealing)
			}
			hybridNear(t, totals[1], totals[0]*1.25)
			sim, c = hybridSim(t, "SHAMAN", hybridTalents(t, "shaman.talent.restorative-totems", 3), mode)
			stream := c.GetSpell(core.ActionID{SpellID: 10461})
			stream.ApplyEffects(sim, &c.Unit, stream)
			expected = (14 + .022*stream.HealingPower(&c.Unit)) * (1 + .10*float64(c.ForeverRank("shaman.talent.restorative-totems"))) * (1 + .02*float64(c.ForeverRank("shaman.talent.purification")))
			hybridNear(t, stream.SpellMetrics[c.UnitIndex].TotalHealing, expected)
		})
	}
}

func TestForeverAuditPounceAndShapeshift(t *testing.T) {
	for _, mode := range []proto.ForeverMode{proto.ForeverMode_STRICT, proto.ForeverMode_BEST_GUESS} {
		t.Run(mode.String(), func(t *testing.T) {
			sim, c := hybridSim(t, "DRUID", hybridTalents(t, "druid.talent.brutal-impact", 2), mode)
			target := sim.Encounter.TargetUnits[0]
			cat := c.GetSpell(core.ActionID{SpellID: 768})
			cat.ApplyEffects(sim, &c.Unit, cat)
			c.AddEnergy(sim, 100, c.NewEnergyMetrics(cat.ActionID))
			c.PseudoStats.InFrontOfTarget = false
			pounce := c.GetSpell(core.ActionID{SpellID: 9827})
			prowl := c.GetSpell(core.ActionID{SpellID: 9913})
			if pounce.CanCast(sim, target) {
				t.Fatal("Pounce allowed outside stealth")
			}
			if !prowl.Cast(sim, &c.Unit) || !pounce.Cast(sim, target) {
				t.Fatal("Prowl/Pounce opener failed")
			}
			if !target.ForeverControlled(core.ForeverStun) || !pounce.Dot(target).IsActive() || c.ComboPoints() != 1 {
				t.Fatal("Pounce stun, bleed or combo point missing")
			}
			hybridNear(t, pounce.Dot(target).SnapshotBaseDamage, 25)
			aura := target.GetAura("Forever Pounce-" + c.Label)
			expected := 2*time.Second + time.Duration(500*c.ForeverRank("druid.talent.brutal-impact"))*time.Millisecond
			if aura.RemainingDuration(sim) != expected {
				t.Fatal("Brutal Impact did not modify Pounce duration")
			}
			root := c.ForeverControlAura("Synthetic test root", core.ActionID{}, core.ForeverRoot, 20*time.Second)
			slow := c.ForeverControlAura("Synthetic test slow", core.ActionID{}, core.ForeverSnare, 20*time.Second)
			root.Activate(sim)
			slow.Activate(sim)
			bear := c.GetSpell(core.ActionID{SpellID: 9634})
			bear.ApplyEffects(sim, &c.Unit, bear)
			if root.IsActive() || slow.IsActive() {
				t.Fatal("Shapeshift left movement restrictions")
			}
		})
	}
}

func hybridAuditParty(t *testing.T, class string, talents map[string]int32, mode proto.ForeverMode) (*core.Simulation, []*core.Character) {
	t.Helper()
	players := []*proto.Player{}
	for i := 0; i < 3; i++ {
		p := &proto.Player{Class: proto.Class_ClassDruid, Race: proto.Race_RaceNightElf, Equipment: &proto.EquipmentSpec{}, Consumes: &proto.Consumes{}, BonusStats: &proto.UnitStats{Stats: stats.Stats{stats.Health: 10000, stats.Mana: 20000, stats.HealingPower: 1000, stats.SpellCrit: -100}.ToFloatArray()}, Rotation: &proto.APLRotation{Type: proto.APLRotation_TypeAPL}, Forever: &proto.ForeverOptions{RulesetId: foreverdata.RulesetID, Mode: mode, Talents: talents, ExperimentalEstimatedRanks: true}, Spec: &proto.Player_RestorationDruid{RestorationDruid: &proto.RestorationDruid{Options: &proto.RestorationDruid_Options{}}}}
		if class == "SHAMAN" {
			p.Class = proto.Class_ClassShaman
			p.Race = proto.Race_RaceTroll
			p.Spec = &proto.Player_RestorationShaman{RestorationShaman: &proto.RestorationShaman{Options: &proto.RestorationShaman_Options{}}}
		}
		players = append(players, p)
	}
	sim := core.NewSim(&proto.RaidSimRequest{Raid: &proto.Raid{Parties: []*proto.Party{{Players: players}}}, Encounter: &proto.Encounter{Duration: 60, Targets: []*proto.Target{{Level: 60, Stats: stats.Stats{stats.Health: 100000}.ToFloatArray()}}}, SimOptions: &proto.SimOptions{Iterations: 1, RandomSeed: 1, Interactive: true, IsTest: true}}, simsignals.Signals{})
	sim.Reset()
	party := []*core.Character{}
	for _, agent := range sim.Raid.Parties[0].Players {
		party = append(party, agent.GetCharacter())
	}
	return sim, party
}

func TestForeverAuditPartyEffects(t *testing.T) {
	for _, mode := range []proto.ForeverMode{proto.ForeverMode_STRICT, proto.ForeverMode_BEST_GUESS} {
		t.Run(mode.String(), func(t *testing.T) {
			sim, party := hybridAuditParty(t, "SHAMAN", hybridTalents(t, "shaman.talent.riptide", 1), mode)
			c := party[0]
			target := &party[2].Unit
			riptide := c.GetSpell(c.ForeverAction("shaman.talent.riptide"))
			riptide.ApplyEffects(sim, target, riptide)
			chain := c.GetSpell(core.ActionID{SpellID: 10623})
			chain.ApplyEffects(sim, target, chain)
			primary := chain.SpellMetrics[target.UnitIndex].TotalHealing
			hybridNear(t, chain.SpellMetrics[party[0].UnitIndex].TotalHealing, primary*.5)
			hybridNear(t, chain.SpellMetrics[party[1].UnitIndex].TotalHealing, primary*.25)

			sim, party = hybridAuditParty(t, "DRUID", hybridTalents(t, "druid.talent.wild-growth", 1), mode)
			party[0].DistanceFromTarget = 0
			party[1].DistanceFromTarget = 40
			party[2].DistanceFromTarget = 90
			c = party[0]
			growth := c.GetSpell(c.ForeverAction("druid.talent.wild-growth"))
			growth.ApplyEffects(sim, &party[1].Unit, growth)
			if !growth.Hot(&party[0].Unit).IsActive() || !growth.Hot(&party[1].Unit).IsActive() || growth.Hot(&party[2].Unit).IsActive() {
				t.Fatal("Wild Growth ignored selected-target radius")
			}

			sim, party = hybridAuditParty(t, "DRUID", hybridTalents(t, "druid.talent.leader-of-the-pack", 1), mode)
			party[0].DistanceFromTarget = 0
			party[1].DistanceFromTarget = 40
			party[2].DistanceFromTarget = 90
			c = party[0]
			before := party[1].GetStat(stats.SpellCrit)
			far := party[2].GetStat(stats.SpellCrit)
			cat := c.GetSpell(core.ActionID{SpellID: 768})
			cat.ApplyEffects(sim, &c.Unit, cat)
			hybridNear(t, party[1].GetStat(stats.SpellCrit)-before, 3)
			hybridNear(t, party[2].GetStat(stats.SpellCrit), far)
			party[1].DistanceFromTarget = 80
			for sim.CurrentTime < 250*time.Millisecond {
				if sim.Step() {
					break
				}
			}
			hybridNear(t, party[1].GetStat(stats.SpellCrit), before)

			sim, party = hybridAuditParty(t, "SHAMAN", hybridTalents(t, "shaman.talent.guardian-totems", 1), mode)
			c = party[0]
			c.DistanceFromTarget = 10
			party[1].DistanceFromTarget = 25
			party[2].DistanceFromTarget = 80
			grounding := c.GetSpell(core.ActionID{SpellID: 8177})
			grounding.ApplyEffects(sim, &c.Unit, grounding)
			incoming := &core.Spell{SpellSchool: core.SpellSchoolFire, DefenseType: core.DefenseTypeMagic}
			hit := func(c *core.Character) float64 {
				r := &core.SpellResult{Target: &c.Unit, Damage: 100, Outcome: core.OutcomeHit}
				for _, f := range c.DynamicDamageTakenModifiers {
					f(sim, incoming, r)
				}
				return r.Damage
			}
			hybridNear(t, hit(party[2]), 100)
			hybridNear(t, hit(party[1]), 0)
			hybridNear(t, hit(c), 100)
		})
	}
}

func TestForeverAuditUnqualifiedDodgeAndExactArmor(t *testing.T) {
	for _, mode := range []proto.ForeverMode{proto.ForeverMode_STRICT, proto.ForeverMode_BEST_GUESS} {
		t.Run(mode.String(), func(t *testing.T) {
			_, base := hybridSim(t, "DRUID", map[string]int32{}, mode)
			sim, c := hybridSim(t, "DRUID", hybridTalents(t, "druid.talent.feral-swiftness", 2), mode)
			dodge := c.GetStat(stats.Dodge)
			hybridNear(t, dodge-base.GetStat(stats.Dodge), 2*float64(c.ForeverRank("druid.talent.feral-swiftness")))
			cat := c.GetSpell(core.ActionID{SpellID: 768})
			cat.ApplyEffects(sim, &c.Unit, cat)
			hybridNear(t, c.GetStat(stats.Dodge), dodge)
			bear := c.GetSpell(core.ActionID{SpellID: 9634})
			bear.ApplyEffects(sim, &c.Unit, bear)
			hybridNear(t, c.GetStat(stats.Dodge), dodge)
			sim, c = hybridSim(t, "DRUID", hybridTalents(t, "druid.talent.thick-hide", 3), mode)
			c.AddStatDynamic(sim, stats.Defense, 100)
			before := c.GetStat(stats.Armor)
			cat = c.GetSpell(core.ActionID{SpellID: 768})
			cat.ApplyEffects(sim, &c.Unit, cat)
			hybridNear(t, c.GetStat(stats.Armor)-before, float64(c.Level)+100*c.ForeverValue("druid.talent.thick-hide", 1, 0))
			sim, c = hybridSim(t, "DRUID", hybridTalents(t, "druid.talent.natural-reaction", 5), mode)
			total := 0.0
			for i := 0; i < 100; i++ {
				c.SpendRage(sim, c.CurrentRage(), c.NewRageMetrics(core.ActionID{}))
				c.OnSpellHitTaken(sim, &core.Spell{Unit: sim.Encounter.TargetUnits[0]}, &core.SpellResult{Target: &c.Unit, Outcome: core.OutcomeDodge})
				if c.CurrentRage() > 0 {
					hybridNear(t, c.CurrentRage(), 5)
					total += c.CurrentRage()
				}
			}
			if total == 0 {
				t.Fatal("Natural Reaction failed outside Bear form")
			}
		})
	}
}

func TestForeverAuditManaTideCastPosition(t *testing.T) {
	for _, mode := range []proto.ForeverMode{proto.ForeverMode_STRICT, proto.ForeverMode_BEST_GUESS} {
		t.Run(mode.String(), func(t *testing.T) {
			sim, party := hybridAuditParty(t, "SHAMAN", hybridTalents(t, "shaman.talent.mana-tide-totem", 1), mode)
			party[0].DistanceFromTarget = 40
			party[1].DistanceFromTarget = 50
			party[2].DistanceFromTarget = 90
			c := party[0]
			tide := c.GetSpell(c.ForeverAction("shaman.talent.mana-tide-totem"))
			before := []float64{}
			for _, c := range party {
				c.SpendMana(sim, 1000, c.NewManaMetrics(tide.ActionID))
				before = append(before, c.CurrentMana())
			}
			tide.ApplyEffects(sim, &c.Unit, tide)
			for sim.CurrentTime < 3100*time.Millisecond {
				if sim.Step() {
					break
				}
			}
			near := party[1].CurrentMana() - before[1]
			far := party[2].CurrentMana() - before[2]
			hybridNear(t, near-far, 88)
		})
	}
}
