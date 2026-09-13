package core_test

import (
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/simsignals"
	"github.com/wowsims/classic/sim/mage"
	"testing"
	"time"
)

func reviewSim(t *testing.T, edits ...func(*proto.Player)) (*core.Simulation, *core.Character) {
	t.Helper()
	p := &proto.Player{Class: proto.Class_ClassMage, Race: proto.Race_RaceGnome, Spec: &proto.Player_Mage{Mage: &proto.Mage{Options: &proto.Mage_Options{}}}, Equipment: &proto.EquipmentSpec{}, Rotation: &proto.APLRotation{Type: proto.APLRotation_TypeAPL}, Forever: &proto.ForeverOptions{RulesetId: foreverdata.RulesetID, Mode: proto.ForeverMode_BEST_GUESS}}
	for _, edit := range edits {
		edit(p)
	}
	s := core.NewSim(&proto.RaidSimRequest{Raid: &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{p}}}}, Encounter: &proto.Encounter{Duration: 60, Targets: []*proto.Target{{Level: 60}}}, SimOptions: &proto.SimOptions{Iterations: 1, Interactive: true, IsTest: true}}, simsignals.Signals{})
	s.Reset()
	return s, s.Raid.Parties[0].Players[0].GetCharacter()
}
func TestForeverReviewControlRefresh(t *testing.T) {
	sim, c := reviewSim(t)
	c.ForeverControlReduction(core.ForeverStun, .2)
	a := c.ForeverControlAura("review-stun", core.ActionID{SpellID: 12809}, core.ForeverStun, 10*time.Second)
	a.Activate(sim)
	if a.RemainingDuration(sim) != 8*time.Second {
		t.Fatal(a.RemainingDuration(sim))
	}
	a.Activate(sim)
	if a.RemainingDuration(sim) != 8*time.Second {
		t.Fatalf("refresh ignored reduction: %s", a.RemainingDuration(sim))
	}
}
func TestForeverReviewControlVariableDuration(t *testing.T) {
	sim, c := reviewSim(t)
	a := c.ForeverControlAura("review-kidney", core.ActionID{SpellID: 8643}, core.ForeverStun, 2*time.Second)
	a.Activate(sim)
	a.Deactivate(sim)
	a = c.ForeverControlAura("review-kidney", core.ActionID{SpellID: 8643}, core.ForeverStun, 6*time.Second)
	a.Activate(sim)
	if a.RemainingDuration(sim) != 6*time.Second {
		t.Fatalf("duration stuck at first combo points: %s", a.RemainingDuration(sim))
	}
}
func TestForeverReviewControlReset(t *testing.T) {
	sim, c := reviewSim(t)
	a := c.ForeverControlAura("review-reset", core.ActionID{SpellID: 12809}, core.ForeverStun, 10*time.Second)
	a.Activate(sim)
	sim.Cleanup()
	sim.Reset()
	if c.ForeverControlled(core.ForeverStun) {
		t.Fatal("stun state survived simulation reset")
	}
}
func TestForeverReviewOverlappingControl(t *testing.T) {
	sim, c := reviewSim(t)
	a := c.ForeverControlAura("review-one", core.ActionID{SpellID: 12809}, core.ForeverStun, 10*time.Second)
	b := c.ForeverControlAura("review-two", core.ActionID{SpellID: 8643}, core.ForeverStun, 6*time.Second)
	a.Activate(sim)
	b.Activate(sim)
	a.Deactivate(sim)
	if !c.ForeverControlled(core.ForeverStun) {
		t.Fatal("first expiration removed second stun")
	}
	b.Deactivate(sim)
	if c.ForeverControlled(core.ForeverStun) {
		t.Fatal("stun stuck")
	}
}
func TestForeverReviewImmunityDispel(t *testing.T) {
	sim, c := reviewSim(t)
	a := c.GetOrRegisterAura(core.Aura{Label: "review-poison", Tag: "forever-debuff-poison", Duration: 20 * time.Second})
	a.Activate(sim)
	i := c.GetAura("review-immunity")
	i.Activate(sim)
	if a.IsActive() {
		t.Fatal("poison not cleansed")
	}
	a.Activate(sim)
	if a.IsActive() {
		t.Fatal("poison bypassed immunity")
	}
	i.Deactivate(sim)
	a.Activate(sim)
	if !a.IsActive() {
		t.Fatal("poison remains immune")
	}
}

func init() {
	core.RegisterAgentFactory(proto.Player_Mage{}, proto.Spec_SpecMage, func(c *core.Character, p *proto.Player) core.Agent {
		a := mage.NewMage(c, p)
		u := a.GetCharacter()
		u.GetOrRegisterAura(core.Aura{Label: "review-poison", Tag: "forever-debuff-poison", Duration: 20 * time.Second})
		u.ForeverDebuffImmunity("review-immunity", core.ActionID{SpellID: 20594}, []string{"poison"}, 8*time.Second)
		return a
	}, func(p *proto.Player, s interface{}) { p.Spec = s.(*proto.Player_Mage) })
}
func TestForeverReviewRuntimeControlsStayBounded(t *testing.T) {
	sim, c := reviewSim(t)
	before := len(c.GetAuras())
	for iteration := 0; iteration < 3; iteration++ {
		for i := 0; i < 100; i++ {
			a := c.ForeverControlAura("review-bounded", core.ActionID{SpellID: 12809}, core.ForeverStun, 10*time.Second)
			a.Activate(sim)
			a.Deactivate(sim)
		}
		if len(c.GetAuras()) != before+1 {
			t.Fatal("runtime control registrations grew", len(c.GetAuras()), before)
		}
		sim.Cleanup()
		sim.Reset()
		if c.ForeverControlled(core.ForeverStun) {
			t.Fatal("control survived cleanup/reset")
		}
	}
}
func TestForeverReviewClassicLateAuraGuard(t *testing.T) {
	_, c := reviewSim(t)
	defer func() {
		if recover() == nil {
			t.Fatal("generic late registration must still fail")
		}
	}()
	c.RegisterAura(core.Aura{Label: "forbidden-late-aura", Duration: time.Second})
}
func TestForeverReviewRepeatedMoveCommand(t *testing.T) {
	sim, c := reviewSim(t)
	c.MoveTo(14, sim)
	c.MoveTo(7, sim)
	for sim.CurrentTime < time.Second {
		if sim.Step() {
			t.Fatal("ended early")
		}
	}
	if c.DistanceFromTarget > 7.001 || c.DistanceFromTarget < 6.999 {
		t.Fatal("competing movement actions", c.DistanceFromTarget)
	}
}
func TestForeverReviewRecoveryCompleteAmount(t *testing.T) {
	sim, c := reviewSim(t, func(p *proto.Player) {
		p.Race = proto.Race_RaceTroll
		p.Forever.Mechanics = []string{"racials.troll.rapid-regeneration"}
	})
	c.RemoveHealth(sim, c.MaxHealth()*.9)
	before := c.CurrentHealth()
	id := c.ForeverAction("racials.troll.rapid-regeneration")
	var s *core.Spell
	for _, spell := range c.Spellbook {
		if spell.ActionID == id {
			s = spell
		}
	}
	if s == nil || !s.Cast(sim, c.CurrentTarget) {
		t.Fatal("recovery absent")
	}
	for sim.CurrentTime < 11*time.Second {
		if sim.Step() {
			t.Fatal("ended early")
		}
	}
	actual := (c.CurrentHealth() - before) / c.MaxHealth()
	if actual < .499999 || actual > .500001 {
		t.Fatalf("recovery fraction got %.8f want .5", actual)
	}
}
func TestForeverReviewStrictPredictionGate(t *testing.T) {
	_, base := reviewSim(t)
	_, pred := reviewSim(t, func(p *proto.Player) {
		p.Profession1 = proto.Profession_Mining
		p.Forever.Mechanics = []string{"profession.mining.made-of-metal"}
	})
	_, strict := reviewSim(t, func(p *proto.Player) {
		p.Profession1 = proto.Profession_Mining
		p.Forever.Mode = proto.ForeverMode_STRICT
		p.Forever.Mechanics = []string{"profession.mining.made-of-metal"}
	})
	if pred.MaxHealth() < base.MaxHealth()*1.04999 {
		t.Fatal("prediction did not execute")
	}
	if strict.MaxHealth() != base.MaxHealth() {
		t.Fatal("prediction leaked into STRICT")
	}
}
func TestForeverReviewEurekaChargesAndReversal(t *testing.T) {
	sim, c := reviewSim(t, func(p *proto.Player) { p.Forever.Mechanics = []string{"racials.gnome.eureka"} })
	var e, bolt *core.Spell
	id := c.ForeverAction("racials.gnome.eureka")
	for _, s := range c.Spellbook {
		if s.ActionID == id {
			e = s
		}
		if s.SpellID == 133 {
			bolt = s
		}
	}
	if e == nil || bolt == nil {
		t.Fatal("missing Eureka or Fireball")
	}
	before := bolt.DamageMultiplier
	cost := bolt.Cost.Multiplier
	if !e.Cast(sim, c.CurrentTarget) {
		t.Fatal("Eureka cast failed")
	}
	a := c.GetAura("racials.gnome.eureka")
	for i := int32(3); i > 0; i-- {
		if a.GetStacks() != i || bolt.Cost.Multiplier != cost-20 {
			t.Fatal("Eureka wrong stack/cost")
		}
		if bolt.DamageMultiplier/before < 1.099999 {
			t.Fatal("Eureka damage missing")
		}
		c.OnCastComplete(sim, bolt)
	}
	if a.IsActive() || bolt.Cost.Multiplier != cost || bolt.DamageMultiplier != before {
		t.Fatal("Eureka leaked after third charge")
	}
}
