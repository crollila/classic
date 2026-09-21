package warrior_test

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/simsignals"
	"github.com/wowsims/classic/sim/core/stats"
	"github.com/wowsims/classic/sim/mage"
)

func TestForeverUnbridledWrathRequiresDamage(t *testing.T) {
	sim, c := physicalSim(t, "Warrior", "warrior", physicalBuild(t, "warrior.talent.unbridled-wrath"))
	sunder := physicalID(t, c, 11597)
	for i := 0; i < 100; i++ {
		sunder.CalcAndDealOutcome(sim, c.CurrentTarget, sunder.OutcomeAlwaysHit)
	}
	if c.CurrentRage() != 0 {
		t.Fatal("zero-damage Sunder generated rage", c.CurrentRage())
	}
	hamstring := physicalID(t, c, 7373)
	for i := 0; i < 100; i++ {
		hamstring.ApplyEffects(sim, c.CurrentTarget, hamstring)
	}
	if c.CurrentRage() <= 0 {
		t.Fatal("damaging melee attacks never generated rage")
	}
}

func TestForeverColdBloodOnlyListedAbilities(t *testing.T) {
	build := physicalBuild(t, "rogue.talent.cold-blood")
	for k, n := range physicalBuild(t, "rogue.talent.ghostly-strike") {
		build[k] = n
	}
	sim, c := physicalSim(t, "Rogue", "rogue", build)
	ghost := physicalID(t, c, 14278)
	strike := physicalID(t, c, 11294)
	beforeGhost, beforeStrike := ghost.BonusCritRating, strike.BonusCritRating
	if !physicalID(t, c, 14177).Cast(sim, c.CurrentTarget) {
		t.Fatal("Cold Blood failed")
	}
	if ghost.BonusCritRating != beforeGhost || strike.BonusCritRating != beforeStrike+100 {
		t.Fatal("Cold Blood affected an unlisted attack")
	}
	ghost.ApplyEffects(sim, c.CurrentTarget, ghost)
	if !c.GetAura("Cold Blood").IsActive() {
		t.Fatal("Ghostly Strike consumed Cold Blood")
	}
	strike.ApplyEffects(sim, c.CurrentTarget, strike)
	if c.GetAura("Cold Blood").IsActive() {
		t.Fatal("Sinister Strike failed to consume Cold Blood")
	}
}

func TestForeverSetupRejectsZeroDamageLandedSpells(t *testing.T) {
	sim, c := physicalSim(t, "Rogue", "rogue", physicalBuild(t, "rogue.talent.setup"))
	// Manufactured outcomes exercise the same incoming event dispatch as the
	// engine's combined spell-miss/full-binary-resist outcome. No live log data.
	spell := &core.Spell{Unit: c.CurrentTarget, DefenseType: core.DefenseTypeMagic, SpellSchool: core.SpellSchoolShadow}
	result := &core.SpellResult{Target: &c.Unit, Outcome: core.OutcomeHit}
	for i := 0; i < 100; i++ {
		c.OnSpellHitTaken(sim, spell, result)
	}
	if c.ComboPoints() != 0 {
		t.Fatal("landed zero-damage utility triggered Setup")
	}
	result.Outcome = core.OutcomeMiss
	for i := 0; i < 10; i++ {
		c.OnSpellHitTaken(sim, spell, result)
	}
	if c.ComboPoints() == 0 {
		t.Fatal("full magic-resist analogue failed to trigger Setup")
	}
}

func TestForeverLastStandLosesTemporaryHealth(t *testing.T) {
	sim, c := physicalSim(t, "Warrior", "warrior", physicalBuild(t, "warrior.talent.last-stand"))
	before := c.MaxHealth()
	if !physicalID(t, c, 12975).Cast(sim, c.CurrentTarget) {
		t.Fatal("Last Stand failed")
	}
	c.RemoveHealth(sim, before*.1)
	c.GetAura("Last Stand").Deactivate(sim)
	if math.Abs(c.MaxHealth()-before) > 1e-8 || math.Abs(c.CurrentHealth()-before*.9) > 1e-8 {
		t.Fatal("temporary health persisted", c.CurrentHealth(), c.MaxHealth(), before)
	}
	c.GetAura("Last Stand").Activate(sim)
	c.RemoveHealth(sim, c.CurrentHealth()-10)
	c.GetAura("Last Stand").Deactivate(sim)
	if c.CurrentHealth() != 1 {
		t.Fatal("Last Stand expiration failed to preserve 1 HP", c.CurrentHealth())
	}
}

func TestForeverHawkEyeConstrainsScheduledAutoShot(t *testing.T) {
	sim, c := physicalSim(t, "Hunter", "hunter", physicalBuild(t, "hunter.talent.hawk-eye"), func(p *proto.Player) {
		p.Equipment = core.GetGearSet("../../ui/hunter/gear_sets", "p0.bis").GearSet
		p.DistanceFromTarget = 42
	})
	auto := c.AutoAttacks.RangedAuto()
	if auto.CanCast(sim, c.CurrentTarget) {
		t.Fatal("ranged auto cast beyond 41 yards")
	}
	sim.PrePull()
	physicalAdvance(t, sim, 2*time.Second)
	if auto.SpellMetrics[c.CurrentTarget.UnitIndex].Casts != 0 {
		t.Fatal("scheduled auto ignored maximum range")
	}
	c.DistanceFromTarget = 40
	physicalAdvance(t, sim, 5*time.Second)
	if auto.SpellMetrics[c.CurrentTarget.UnitIndex].Casts == 0 {
		t.Fatal("auto failed to resume inside extended range")
	}
}

func TestForeverPathfindingPackAffectsPartyAndMovement(t *testing.T) {
	build := physicalBuild(t, "hunter.talent.pathfinding")
	makePlayer := func(name string, distance float64) *proto.Player {
		return &proto.Player{Name: name, Class: proto.Class_ClassHunter, Race: proto.Race_RaceTroll, DistanceFromTarget: distance, Spec: &proto.Player_Hunter{Hunter: &proto.Hunter{Options: &proto.Hunter_Options{}}}, Equipment: &proto.EquipmentSpec{}, Rotation: &proto.APLRotation{Type: proto.APLRotation_TypeAPL}, Forever: &proto.ForeverOptions{RulesetId: foreverdata.RulesetID, Talents: build, Mode: proto.ForeverMode_BEST_GUESS}}
	}
	sim := core.NewSim(&proto.RaidSimRequest{Raid: &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{makePlayer("source", 0), makePlayer("near", 20), makePlayer("far", 50)}}}}, Encounter: &proto.Encounter{Duration: 60, Targets: []*proto.Target{{Level: 60}}}, SimOptions: &proto.SimOptions{Iterations: 1, Interactive: true, IsTest: true}}, simsignals.Signals{})
	sim.Reset()
	players := sim.Raid.Parties[0].Players
	source, near, far := players[0].GetCharacter(), players[1].GetCharacter(), players[2].GetCharacter()
	if !physicalID(t, source, 13159).Cast(sim, source.CurrentTarget) {
		t.Fatal("Pack failed")
	}
	if math.Abs(near.MovementHandler.MoveSpeed-7*1.36) > 1e-8 || far.MovementHandler.MoveSpeed != 7 {
		t.Fatal("Pack failed party range", near.MovementHandler.MoveSpeed, far.MovementHandler.MoveSpeed)
	}
	near.MoveTo(30, sim)
	physicalAdvance(t, sim, time.Second)
	if near.DistanceFromTarget < 29 {
		t.Fatal("party movement did not use Pathfinding", near.DistanceFromTarget)
	}
	far.DistanceFromTarget = 20
	physicalAdvance(t, sim, 1500*time.Millisecond)
	if math.Abs(far.MovementHandler.MoveSpeed-7*1.36) > 1e-8 {
		t.Fatal("moving into Pack range did not gain aura")
	}
	source.GetAura("Aspect of the Pack").Deactivate(sim)
	if near.MovementHandler.MoveSpeed != 7 || far.MovementHandler.MoveSpeed != 7 {
		t.Fatal("Pack expiration leaked party speed")
	}
	sim.Cleanup()
	sim.Reset()
	if near.MovementHandler.MoveSpeed != 7 || far.MovementHandler.MoveSpeed != 7 {
		t.Fatal("Pack leaked across iterations")
	}
}

func TestForeverAngerManagementPrecombatDecayScenario(t *testing.T) {
	for _, selected := range []bool{false, true} {
		build := map[string]int32{}
		if selected {
			build = physicalBuild(t, "warrior.talent.anger-management")
		}
		sim, c := physicalSim(t, "Warrior", "warrior", build, func(p *proto.Player) {
			p.GetWarrior().Options.StartingRage = 30
			p.Forever.Parameters = map[string]float64{"scenario.precombat_rage_wait_seconds": 10, "scenario.precombat_rage_loss_per_second": 1}
		})
		sim.PrePull()
		physicalAdvance(t, sim, 0)
		expected := 20.0
		if selected {
			expected = 23
		}
		if math.Abs(c.CurrentRage()-expected) > 1e-8 {
			t.Fatal("precombat rage reduction", selected, c.CurrentRage(), expected)
		}
	}
}

func TestForeverPhysicalInterruptLocksOnlyInterruptedSchool(t *testing.T) {
	for _, tc := range []struct {
		name     string
		class    proto.Class
		action   int32
		talent   string
		duration time.Duration
	}{
		{"Kick", proto.Class_ClassRogue, 1769, "rogue.talent.improved-kick", 5 * time.Second},
		{"Pummel", proto.Class_ClassWarrior, 6554, "warrior.talent.improved-shield-bash", 4 * time.Second},
		{"Shield Bash", proto.Class_ClassWarrior, 1672, "warrior.talent.improved-shield-bash", 6 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &proto.Player{Class: tc.class, Race: proto.Race_RaceHuman, Equipment: &proto.EquipmentSpec{}, Rotation: &proto.APLRotation{Type: proto.APLRotation_TypeAPL}, Forever: &proto.ForeverOptions{RulesetId: foreverdata.RulesetID, Talents: physicalBuild(t, tc.talent), Mode: proto.ForeverMode_BEST_GUESS}}
			if tc.class == proto.Class_ClassRogue {
				p.Spec = &proto.Player_Rogue{Rogue: &proto.Rogue{Options: &proto.RogueOptions{}}}
			} else {
				p.Spec = &proto.Player_Warrior{Warrior: &proto.Warrior{Options: &proto.Warrior_Options{}}}
			}
			caster := &proto.Player{Class: proto.Class_ClassMage, Race: proto.Race_RaceHuman, Equipment: &proto.EquipmentSpec{}, Rotation: &proto.APLRotation{Type: proto.APLRotation_TypeAPL}, Spec: &proto.Player_Mage{Mage: &proto.Mage{Options: &proto.Mage_Options{}}}}
			sim := core.NewSim(&proto.RaidSimRequest{Raid: &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{p, caster}}}}, Encounter: &proto.Encounter{Duration: 60, Targets: []*proto.Target{{Level: 60}}}, SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, Interactive: true}}, simsignals.Signals{})
			sim.Reset()
			actor, victim := sim.Raid.Parties[0].Players[0].GetCharacter(), sim.Raid.Parties[0].Players[1].GetCharacter()
			var fire, frost *core.Spell
			for _, s := range victim.Spellbook {
				if s.SpellCode == mage.SpellCode_MageFireball {
					fire = s
				}
				if s.SpellCode == mage.SpellCode_MageFrostbolt {
					frost = s
				}
			}
			if fire == nil || frost == nil {
				t.Fatal("missing mage cast probes")
			}
			if !fire.Cast(sim, victim.CurrentTarget) {
				t.Fatal("Fireball did not start")
			}
			interrupt := physicalID(t, actor, tc.action)
			interrupt.BonusHitRating = 100
			// Pummel's damage must land; the deterministic seed lands immediately,
			// while retries make the test independent of player dodge RNG.
			for i := 0; i < 100 && victim.IsCasting(sim); i++ {
				interrupt.ApplyEffects(sim, &victim.Unit, interrupt)
			}
			if victim.IsCasting(sim) {
				t.Fatal("physical interrupt did not end Fireball")
			}
			victim.GCD.Set(sim.CurrentTime)
			physicalAdvance(t, sim, 3100*time.Millisecond) // Beyond Improved Kick/Bash's blanket silence.
			if fire.CanCast(sim, victim.CurrentTarget) || !frost.CanCast(sim, victim.CurrentTarget) {
				t.Fatal("school lockout leaked to Frost or omitted Fire")
			}
			physicalAdvance(t, sim, tc.duration+time.Millisecond)
			if !fire.CanCast(sim, victim.CurrentTarget) {
				t.Fatal("school lockout failed to expire")
			}
		})
	}
}

func TestForeverGougeDamageUsesLethalityBeforeIncapacitate(t *testing.T) {
	build := physicalBuild(t, "rogue.talent.lethality")
	build["rogue.talent.improved-gouge"] = 3
	sim, c := physicalSim(t, "Rogue", "rogue", build)
	c.PseudoStats.InFrontOfTarget = true
	c.AddStatDynamic(sim, stats.Expertise, 100)
	gouge := physicalID(t, c, 11286)
	gouge.BonusHitRating, gouge.BonusCritRating = 100, 100
	if math.Abs(gouge.CritDamageBonus-1.2) > 1e-8 { // Forever Lethality 5/5: +20%
		t.Fatal("Lethality omitted Gouge", gouge.CritDamageBonus)
	}
	if !gouge.Cast(sim, c.CurrentTarget) {
		t.Fatal("Gouge failed")
	}
	m := gouge.SpellMetrics[c.CurrentTarget.UnitIndex]
	if m.TotalDamage <= 0 || m.Crits != 1 || c.ComboPoints() < 1 {
		t.Fatal("Gouge damage/crit/CP event missing", m, c.ComboPoints())
	}
	a := c.CurrentTarget.GetAura("Gouge")
	if a == nil || !a.IsActive() || a.Duration != 5500*time.Millisecond {
		t.Fatal("Gouge damage broke its own improved incapacitate")
	}
}

func TestForeverBestialSwiftnessChangesPetArrivalAndAttacks(t *testing.T) {
	for _, swift := range []bool{false, true} {
		build := map[string]int32{}
		if swift {
			build = physicalBuild(t, "hunter.talent.bestial-swiftness")
		}
		sim, c := physicalSim(t, "Hunter", "hunter", build, func(p *proto.Player) {
			p.GetHunter().Options.PetType = proto.Hunter_Options_Cat
			p.GetHunter().Options.PetUptime = 1
			p.Forever.Parameters = map[string]float64{"scenario.pet_starting_distance": 21}
		})
		pet := c.PetAgents[0].GetPet()
		sim.PrePull()
		physicalAdvance(t, sim, 2250*time.Millisecond)
		casts := pet.AutoAttacks.MHAuto().SpellMetrics[pet.CurrentTarget.UnitIndex].Casts
		if swift && (pet.IsMoving() || casts == 0) {
			t.Fatal("faster pet did not arrive and attack", pet.DistanceFromTarget, casts)
		}
		if !swift && (!pet.IsMoving() || casts != 0) {
			t.Fatal("ordinary pet attacked before traveling", pet.DistanceFromTarget, casts)
		}
	}
}
