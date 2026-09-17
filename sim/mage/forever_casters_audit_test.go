package mage

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/simsignals"
)

// These fixtures test the executable secondary effects discovered in the final
// record-by-record audit, rather than merely checking spell registration.
func TestForeverAuditArcticReachBlizzard(t *testing.T) {
	for _, rank := range []int32{0, 2} {
		p := foreverCasterBuild(t, proto.Class_ClassMage, map[string]int32{"mage.talent.arctic-reach": rank})
		sim, c := foreverCasterSim(t, p)
		c.DistanceFromTarget = 35
		can := c.GetSpell(core.ActionID{SpellID: 10187}).CanCast(sim, c.CurrentTarget)
		if can != (rank == 2) {
			t.Fatalf("Arctic Reach rank %d, Blizzard at 35 yards: %v", rank, can)
		}
	}
}

func TestForeverAuditMaledictionChannels(t *testing.T) {
	for _, id := range []int32{11678, 11684} {
		damage := [2]float64{}
		for i, rank := range []int32{0, 1} {
			p := foreverCasterBuild(t, proto.Class_ClassWarlock, map[string]int32{"warlock.talent.malediction": rank})
			sim, c := foreverCasterSim(t, p)
			s := c.GetSpell(core.ActionID{SpellID: id})
			if !s.Cast(sim, c.CurrentTarget) {
				t.Fatal("AoE channel failed to cast")
			}
			foreverAdvance(sim, 4*time.Second)
			damage[i] = s.SpellMetrics[c.CurrentTarget.UnitIndex].TotalDamage
		}
		if damage[0] <= 0 || math.Abs(damage[1]/damage[0]-1.01) > 1e-8 {
			t.Fatalf("Malediction %d: damage %v", id, damage)
		}
	}
}

func TestForeverAuditPenanceRenewedHopeEveryBolt(t *testing.T) {
	p := foreverCasterBuild(t, proto.Class_ClassPriest, map[string]int32{"priest.talent.penance": 1, "priest.talent.renewed-hope": 1})
	sim, c := foreverCasterSim(t, p)
	c.RemoveHealth(sim, 10000)
	weak := c.GetAura("Weakened Soul")
	weak.Activate(sim)
	action := c.ForeverAction("priest.talent.penance")
	s := c.GetSpell(action.WithTag(-action.Tag))
	base := s.BonusCritRating
	seen := 0
	trigger := c.GetAura("Forever Priest healing triggers")
	old := trigger.OnHealDealt
	trigger.OnHealDealt = func(a *core.Aura, sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
		if sp == s {
			seen++
			if math.Abs(sp.BonusCritRating-base-2*core.SpellCritRatingPerCritChance) > 1e-8 {
				t.Error("Penance bolt lost Renewed Hope crit")
			}
		}
		old(a, sim, sp, r)
	}
	if !s.Cast(sim, &c.Unit) {
		t.Fatal("Penance cast failed")
	}
	foreverAdvance(sim, 3*time.Second)
	if seen != 3 || weak.ExpiresAt() != 12*time.Second || s.BonusCritRating != base {
		t.Fatalf("bolts=%d weakened expiration=%v crit=%f", seen, weak.ExpiresAt(), s.BonusCritRating)
	}
}

func TestForeverAuditSoulLinkPetDeathAndResummon(t *testing.T) {
	p := foreverCasterBuild(t, proto.Class_ClassWarlock, map[string]int32{"warlock.talent.soul-link": 1})
	sim, c := foreverCasterSim(t, p)
	var pet *core.Pet
	for _, candidate := range c.Pets {
		if candidate.IsActive() {
			pet = candidate
			break
		}
	}
	if pet == nil || !pet.HasHealthBar() {
		t.Fatal("pet lacks health")
	}
	if !c.GetSpell(core.ActionID{SpellID: 19028}).Cast(sim, &c.Unit) {
		t.Fatal("Soul Link cast")
	}
	incoming := &core.Spell{Unit: c.CurrentTarget, SpellSchool: core.SpellSchoolPhysical}
	r := &core.SpellResult{Target: &c.Unit, Damage: pet.CurrentHealth()/.3 + 100, Outcome: core.OutcomeHit}
	for _, mod := range c.DynamicDamageTakenModifiers {
		mod(sim, incoming, r)
	}
	if pet.IsActive() || c.GetAura("Forever Soul Link").IsActive() {
		t.Fatal("dead linked pet remained active")
	}
	r.Damage = 100
	for _, mod := range c.DynamicDamageTakenModifiers {
		mod(sim, incoming, r)
	}
	if r.Damage != 100 {
		t.Fatal("dead pet continued absorbing damage")
	}
	foreverAdvance(sim, 2*time.Second)
	if !c.GetSpell(core.ActionID{SpellID: 688}).Cast(sim, &c.Unit) {
		t.Fatal("resummon cast")
	}
	foreverAdvance(sim, 13*time.Second)
	if !pet.IsActive() || pet.CurrentHealth() != pet.MaxHealth() {
		t.Fatalf("resummoned pet hp %f/%f", pet.CurrentHealth(), pet.MaxHealth())
	}
}

func TestForeverAuditEnemyManaWithoutHunter(t *testing.T) {
	for _, class := range []proto.Class{proto.Class_ClassWarlock, proto.Class_ClassPriest} {
		goals := map[string]int32{"warlock.talent.fel-concentration": 3}
		id := int32(11704)
		want := 700.0
		if class == proto.Class_ClassPriest {
			goals = map[string]int32{"priest.talent.improved-mana-burn": 1}
			id = 10876
			want = 800
		}
		p := foreverCasterBuild(t, class, goals)
		p.Forever.Parameters = map[string]float64{"scenario.enemy_mana": 1400}
		sim, c := foreverCasterSim(t, p)
		s := c.GetSpell(core.ActionID{SpellID: id})
		if !s.Cast(sim, c.CurrentTarget) {
			t.Fatalf("mana spell %d cast failed", id)
		}
		foreverAdvance(sim, 6*time.Second)
		if c.CurrentTarget.CurrentMana() != want {
			t.Fatalf("mana spell %d left %f, want %f", id, c.CurrentTarget.CurrentMana(), want)
		}
		if id == 11704 && s.PushbackReduction < .7 {
			t.Fatal("Drain Mana missing Fel Concentration")
		}
	}
}

func TestForeverAuditAmplifiedWeaknessAndDrainBonus(t *testing.T) {
	p := foreverCasterBuild(t, proto.Class_ClassWarlock, map[string]int32{"warlock.talent.amplify-curse": 1, "warlock.talent.improved-drains": 1})
	sim, c := foreverCasterSim(t, p)
	drain := c.GetSpell(core.ActionID{SpellID: 11699})
	before := drain.TargetDamageMultiplier(c.AttackTables[c.CurrentTarget.UnitIndex][drain.CastType], true)
	if !c.GetSpell(core.ActionID{SpellID: 18288}).Cast(sim, &c.Unit) {
		t.Fatal("Amplify Curse cast")
	}
	physicalBefore := c.CurrentTarget.PseudoStats.BonusPhysicalDamage
	if !c.GetSpell(core.ActionID{SpellID: 11708}).Cast(sim, c.CurrentTarget) {
		t.Fatal("Weakness cast")
	}
	if c.CurrentTarget.PseudoStats.BonusPhysicalDamage != physicalBefore-46.5 {
		t.Fatalf("amplified weakness physical bonus %f", c.CurrentTarget.PseudoStats.BonusPhysicalDamage)
	}
	after := drain.TargetDamageMultiplier(c.AttackTables[c.CurrentTarget.UnitIndex][drain.CastType], true)
	if math.Abs(after/before-1.02) > 1e-8 {
		t.Fatalf("curse did not count as Affliction effect: %f/%f", after, before)
	}
}

func TestForeverAuditDemonArmorRegeneration(t *testing.T) {
	p := foreverCasterBuild(t, proto.Class_ClassWarlock, map[string]int32{"warlock.talent.demonic-aegis": 2})
	p.GetWarlock().Options.Armor = proto.WarlockOptions_DemonArmor
	sim, c := foreverCasterSim(t, p)
	c.RemoveHealth(sim, 1000)
	before := c.CurrentHealth()
	foreverAdvance(sim, 6*time.Second)
	want := 15 * (1 + c.ForeverValue("warlock.talent.demonic-aegis", 0, 0)/100)
	if math.Abs(c.CurrentHealth()-before-want) > 1e-8 {
		t.Fatalf("armor regen %f want %f", c.CurrentHealth()-before, want)
	}
}

func TestForeverAuditIceBlockImmobility(t *testing.T) {
	p := foreverCasterBuild(t, proto.Class_ClassMage, map[string]int32{"mage.talent.ice-block": 1})
	sim, c := foreverCasterSim(t, p)
	if !c.GetSpell(c.ForeverAction("mage.talent.ice-block")).Cast(sim, &c.Unit) {
		t.Fatal("Ice Block cast")
	}
	before := c.DistanceFromTarget
	c.MoveTo(before+10, sim)
	foreverAdvance(sim, 3*time.Second)
	if c.DistanceFromTarget != before {
		t.Fatal("Ice Block allowed movement")
	}
	foreverAdvance(sim, 12*time.Second)
	if c.DistanceFromTarget <= before {
		t.Fatal("movement did not resume after Ice Block")
	}
}

func foreverCasterPair(t *testing.T, first, second *proto.Player) (*core.Simulation, *core.Character, *core.Character) {
	t.Helper()
	request := &proto.RaidSimRequest{Raid: &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{first, second}}}}, Encounter: &proto.Encounter{Duration: 90, Targets: []*proto.Target{{Level: 60, MobType: proto.MobType_MobTypeHumanoid}, {Level: 60, MobType: proto.MobType_MobTypeHumanoid}}}, SimOptions: &proto.SimOptions{Iterations: 1, RandomSeed: 123, IsTest: true, Interactive: true}}
	sim := core.NewSim(request, simsignals.Signals{})
	sim.Reset()
	return sim, sim.Raid.Parties[0].Players[0].GetCharacter(), sim.Raid.Parties[0].Players[1].GetCharacter()
}

func TestForeverAuditEmbraceAllyKillSpiritTap(t *testing.T) {
	p := foreverCasterBuild(t, proto.Class_ClassPriest, map[string]int32{"priest.talent.vampiric-embrace": 1, "priest.talent.spirit-tap": 5})
	p.Forever.Parameters = map[string]float64{"scenario.enemy_health.0": 100, "scenario.enemy_health.1": 100000}
	m := foreverCasterBuild(t, proto.Class_ClassMage, map[string]int32{})
	sim, c, ally := foreverCasterPair(t, p, m)
	if !c.GetSpell(core.ActionID{SpellID: 15286}).Cast(sim, c.CurrentTarget) {
		t.Fatal("Embrace cast")
	}
	spell := ally.GetSpell(core.ActionID{SpellID: 10149})
	spell.CalcAndDealDamage(sim, c.CurrentTarget, 10000, spell.OutcomeAlwaysHit)
	if !c.GetAura("Spirit Tap").IsActive() {
		t.Fatal("ally kill of embraced target did not grant Spirit Tap")
	}
}

func TestForeverAuditBindingHealRecipientsAndDivineFury(t *testing.T) {
	p := foreverCasterBuild(t, proto.Class_ClassPriest, map[string]int32{"priest.talent.binding-heal": 1, "priest.talent.renewed-hope": 1})
	allyP := foreverCasterBuild(t, proto.Class_ClassMage, map[string]int32{})
	sim, c, ally := foreverCasterPair(t, p, allyP)
	c.RemoveHealth(sim, 10000)
	ally.RemoveHealth(sim, 10000)
	weak := c.GetAura("Weakened Soul")
	weak.Activate(sim)
	if !c.GetSpell(c.ForeverAction("priest.talent.binding-heal")).Cast(sim, &ally.Unit) {
		t.Fatal("Binding Heal cast")
	}
	foreverAdvance(sim, 2*time.Second)
	if weak.ExpiresAt() != 14*time.Second {
		t.Fatal("Binding Heal ignored caster's own Weakened Soul")
	}
	p = foreverCasterBuild(t, proto.Class_ClassPriest, map[string]int32{"priest.talent.divine-fury": 5})
	_, c = foreverCasterSim(t, p)
	if c.GetSpell(core.ActionID{SpellID: 2053}).DefaultCast.CastTime != 2500*time.Millisecond || c.GetSpell(core.ActionID{SpellID: 10965}).DefaultCast.CastTime != 2500*time.Millisecond {
		t.Fatal("Divine Fury changed wrong healing cast time")
	}
}

func TestForeverAuditClearcastingConsumesNextDamageSpell(t *testing.T) {
	p := foreverCasterBuild(t, proto.Class_ClassMage, map[string]int32{"mage.talent.arcane-concentration": 1})
	sim, c := foreverCasterSim(t, p)
	// Isolate consumption from an independent random reproc on the consuming hit.
	c.GetAura("Arcane Concentration").OnSpellHitDealt = func(*core.Aura, *core.Simulation, *core.Spell, *core.SpellResult) {}
	a := c.GetAura("Clearcasting")
	a.Activate(sim)
	foreverAdvance(sim, time.Second)
	s := c.GetSpell(core.ActionID{SpellID: 10149})
	if s.Cost.GetCurrentCost() != 0 || !s.Cast(sim, c.CurrentTarget) {
		t.Fatal("Clearcasting did not grant free Fireball")
	}
	foreverAdvance(sim, 6*time.Second)
	if a.IsActive() || s.Cost.GetCurrentCost() <= 0 {
		t.Fatal("Clearcasting persisted after its free damage spell")
	}
}

func TestForeverAuditRedemptionTargetingAndMovement(t *testing.T) {
	p := foreverCasterBuild(t, proto.Class_ClassPriest, map[string]int32{"priest.talent.spirit-of-redemption": 1})
	other := foreverCasterBuild(t, proto.Class_ClassPriest, map[string]int32{})
	sim, c, ally := foreverCasterPair(t, p, other)
	incoming := &core.Spell{Unit: c.CurrentTarget, SpellSchool: core.SpellSchoolPhysical}
	r := &core.SpellResult{Target: &c.Unit, Damage: c.CurrentHealth() + 100, Outcome: core.OutcomeHit}
	for _, mod := range c.DynamicDamageTakenModifiers {
		mod(sim, incoming, r)
	}
	c.RemoveHealth(sim, r.Damage)
	if !c.GetAura("Forever Spirit of Redemption").IsActive() {
		t.Fatal("lethal damage did not trigger Redemption")
	}
	heal := c.GetSpell(core.ActionID{SpellID: 10965})
	if heal.Cost.GetCurrentCost() != 0 || !heal.CanCast(sim, &ally.Unit) {
		t.Fatal("Spirit cannot heal an ally for free")
	}
	if ally.GetSpell(core.ActionID{SpellID: 10965}).CanCast(sim, &c.Unit) {
		t.Fatal("Spirit remained spell-targetable")
	}
	before := c.DistanceFromTarget
	c.MoveTo(before+10, sim)
	foreverAdvance(sim, 3*time.Second)
	if c.DistanceFromTarget != before {
		t.Fatal("Spirit moved")
	}
	foreverAdvance(sim, 16*time.Second)
	if c.CurrentHealth() != 0 || heal.CanCast(sim, &ally.Unit) {
		t.Fatal("Spirit expiry did not end the priest's life/actions")
	}
}

func TestForeverAuditShadowburnRefundOnAllyPeriodicKill(t *testing.T) {
	p := foreverCasterBuild(t, proto.Class_ClassWarlock, map[string]int32{"warlock.talent.shadowburn": 1})
	p.Forever.Parameters = map[string]float64{"scenario.enemy_health.0": 100000, "scenario.enemy_health.1": 100000, "warlock.soul_shards": 1}
	allyP := foreverCasterBuild(t, proto.Class_ClassPriest, map[string]int32{})
	sim, c, ally := foreverCasterPair(t, p, allyP)
	if !c.GetSpell(core.ActionID{SpellID: 18871}).Cast(sim, c.CurrentTarget) {
		t.Fatal("Shadowburn cast")
	}
	shards := c.GetAura("Forever Soul Shards")
	if shards.GetStacks() != 0 {
		t.Fatal("Shadowburn did not spend the prepared shard")
	}
	target := c.CurrentTarget
	target.RemoveHealth(sim, target.CurrentHealth()-1)
	dot := ally.GetSpell(core.ActionID{SpellID: 10894}).Dot(target)
	dot.Apply(sim)
	dot.SnapshotBaseDamage = 10000
	dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTick)
	if shards.GetStacks() != 1 {
		t.Fatal("ally periodic kill did not refund Shadowburn shard")
	}
}

func TestForeverAuditPrayerMendingPeriodicDamageAndFiveJumps(t *testing.T) {
	p := foreverCasterBuild(t, proto.Class_ClassPriest, map[string]int32{"priest.talent.prayer-of-mending": 1})
	other := foreverCasterBuild(t, proto.Class_ClassMage, map[string]int32{})
	sim, c, ally := foreverCasterPair(t, p, other)
	c.RemoveHealth(sim, 20000)
	ally.RemoveHealth(sim, 20000)
	spell := c.GetSpell(c.ForeverAction("priest.talent.prayer-of-mending"))
	if !spell.Cast(sim, &c.Unit) {
		t.Fatal("Prayer of Mending cast")
	}
	label := "Forever Prayer of Mending-" + c.Label
	incoming := &core.Spell{Unit: c.CurrentTarget, SpellSchool: core.SpellSchoolShadow}
	for i := 0; i < 6; i++ {
		target := c
		if i%2 == 1 {
			target = ally
		}
		a := target.GetAura(label)
		if !a.IsActive() {
			t.Fatalf("missing Mending transfer %d", i)
		}
		before := target.CurrentHealth()
		// This invokes the same target event as an actual periodic-damage tick.
		target.OnPeriodicDamageTaken(sim, incoming, &core.SpellResult{Target: &target.Unit, Damage: 1, Outcome: core.OutcomeHit})
		if target.CurrentHealth() <= before {
			t.Fatalf("periodic damage failed to trigger Mending heal %d", i)
		}
	}
	if c.GetAura(label).IsActive() || ally.GetAura(label).IsActive() {
		t.Fatal("Mending exceeded five jumps")
	}
}
