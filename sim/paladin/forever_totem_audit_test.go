package paladin_test

import (
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
	"testing"
	"time"
)

// These enemy attacks/control spells are manufactured test scenarios. Their
// internal action tags are not claims about actual game IDs or enemy abilities.
func hybridSyntheticAttack(enemy *core.Unit) *core.Spell {
	return enemy.RegisterSpell(core.SpellConfig{ActionID: core.ActionID{OtherID: proto.OtherAction_OtherActionForever, Tag: -1901}, SpellSchool: core.SpellSchoolPhysical, DefenseType: core.DefenseTypeMelee, ProcMask: core.ProcMaskMeleeMHAuto, Flags: core.SpellFlagIgnoreResists, DamageMultiplier: 1, ThreatMultiplier: 1})
}

func TestForeverManaTideKillableAndFourPulses(t *testing.T) {
	for _, mode := range []proto.ForeverMode{proto.ForeverMode_STRICT, proto.ForeverMode_BEST_GUESS} {
		for _, destroy := range []bool{false, true} {
			t.Run(mode.String()+core.Ternary(destroy, "/destroyed", "/full-duration"), func(t *testing.T) {
				sim, party := hybridAuditParty(t, "SHAMAN", hybridTalents(t, "shaman.talent.mana-tide-totem", 1), mode)
				c := party[0]
				var pet *core.Pet
				for _, p := range c.Pets {
					if p.Name == "Mana Tide Totem" {
						pet = p
					}
				}
				if pet == nil {
					t.Fatal("Mana Tide entity not constructed")
				}
				attack := hybridSyntheticAttack(sim.Encounter.TargetUnits[0])
				c.DistanceFromTarget = 40
				party[1].DistanceFromTarget = 50
				party[2].DistanceFromTarget = 90
				tide := c.GetSpell(c.ForeverAction("shaman.talent.mana-tide-totem"))
				before := []float64{}
				for _, p := range party {
					p.SpendMana(sim, 1000, p.NewManaMetrics(tide.ActionID))
					before = append(before, p.CurrentMana())
				}
				tide.ApplyEffects(sim, &c.Unit, tide)
				hybridNear(t, pet.MaxHealth(), 5)
				hybridNear(t, pet.CurrentHealth(), 5)
				if !pet.IsEnabled() {
					t.Fatal("Mana Tide not summoned")
				}
				for sim.CurrentTime < 3100*time.Millisecond {
					if sim.Step() {
						break
					}
				}
				hybridNear(t, (party[1].CurrentMana()-before[1])-(party[2].CurrentMana()-before[2]), 88)
				if destroy {
					attack.CalcAndDealDamage(sim, &pet.Unit, 3, attack.OutcomeAlwaysHit)
					hybridNear(t, pet.CurrentHealth(), 2)
					if !pet.IsEnabled() {
						t.Fatal("Mana Tide died before five damage")
					}
					attack.CalcAndDealDamage(sim, &pet.Unit, 3, attack.OutcomeAlwaysHit)
					if pet.IsEnabled() || !pet.Metrics.Died {
						t.Fatal("Incoming damage did not destroy Mana Tide")
					}
				}
				for sim.CurrentTime < 12100*time.Millisecond {
					if sim.Step() {
						break
					}
				}
				expected := 352.0
				if destroy {
					expected = 88
				}
				hybridNear(t, (party[1].CurrentMana()-before[1])-(party[2].CurrentMana()-before[2]), expected)
				if pet.IsEnabled() {
					t.Fatal("Mana Tide remained after twelve seconds")
				}
			})
		}
	}
}

func TestForeverTotemReplacementAndStoneclawDamage(t *testing.T) {
	for _, mode := range []proto.ForeverMode{proto.ForeverMode_STRICT, proto.ForeverMode_BEST_GUESS} {
		t.Run(mode.String(), func(t *testing.T) {
			sim, c := hybridSim(t, "SHAMAN", hybridTalents(t, "shaman.talent.mana-tide-totem", 1), mode)
			tide := c.GetSpell(c.ForeverAction("shaman.talent.mana-tide-totem"))
			tide.ApplyEffects(sim, &c.Unit, tide)
			var pet *core.Pet
			for _, p := range c.Pets {
				if p.Name == "Mana Tide Totem" {
					pet = p
				}
			}
			stream := c.GetSpell(core.ActionID{SpellID: 10463})
			if !stream.Cast(sim, &c.Unit) {
				t.Fatal("Replacement totem cast failed")
			}
			if pet.IsEnabled() {
				t.Fatal("Water totem replacement did not remove Mana Tide")
			}
			sim, c = hybridSim(t, "SHAMAN", hybridTalents(t, "shaman.talent.earth-s-grasp", 2), mode)
			enemy := sim.Encounter.TargetUnits[0]
			attack := hybridSyntheticAttack(enemy)
			previous := enemy.CurrentTarget
			stone := c.GetSpell(core.ActionID{SpellID: 10428})
			stone.ApplyEffects(sim, enemy, stone)
			for _, p := range c.Pets {
				if p.Name == "Stoneclaw Totem" {
					pet = p
				}
			}
			if enemy.CurrentTarget != &pet.Unit {
				t.Fatal("Stoneclaw failed to taunt")
			}
			attack.CalcAndDealDamage(sim, &pet.Unit, 1000, attack.OutcomeAlwaysHit)
			if pet.IsEnabled() || enemy.CurrentTarget != previous {
				t.Fatal("Actual Stoneclaw destruction did not release taunt")
			}
		})
	}
}

func TestForeverGroundingInterceptsSingleTargetControl(t *testing.T) {
	for _, mode := range []proto.ForeverMode{proto.ForeverMode_STRICT, proto.ForeverMode_BEST_GUESS} {
		t.Run(mode.String(), func(t *testing.T) {
			sim, party := hybridAuditParty(t, "SHAMAN", hybridTalents(t, "shaman.talent.guardian-totems", 1), mode)
			enemy := sim.Encounter.TargetUnits[0]
			roots := map[*core.Unit]*core.Aura{}
			for _, c := range party {
				roots[&c.Unit] = c.ForeverControlAura("Synthetic Grounding root", core.ActionID{}, core.ForeverRoot, 10*time.Second)
			}
			spell := enemy.RegisterSpell(core.SpellConfig{ActionID: core.ActionID{OtherID: proto.OtherAction_OtherActionForever, Tag: -1902}, ForeverSingleTargetHarmful: true, SpellSchool: core.SpellSchoolNature, DefenseType: core.DefenseTypeMagic, ApplyEffects: func(sim *core.Simulation, target *core.Unit, sp *core.Spell) { roots[target].Activate(sim) }})
			c := party[0]
			c.DistanceFromTarget = 10
			party[1].DistanceFromTarget = 25
			party[2].DistanceFromTarget = 80
			grounding := c.GetSpell(core.ActionID{SpellID: 8177})
			grounding.ApplyEffects(sim, &c.Unit, grounding)
			if !spell.Cast(sim, &party[2].Unit) || !roots[&party[2].Unit].IsActive() {
				t.Fatal("Out-of-range control was incorrectly intercepted")
			}
			if !c.GetAura("Forever Grounding Totem").IsActive() {
				t.Fatal("Out-of-range spell consumed charge")
			}
			if !spell.Cast(sim, &party[1].Unit) {
				t.Fatal("Incoming control cast failed")
			}
			if roots[&party[1].Unit].IsActive() || c.GetAura("Forever Grounding Totem").IsActive() {
				t.Fatal("Grounding failed to consume control before root applied")
			}
			if spell.SpellMetrics[party[1].UnitIndex].Casts != 1 {
				t.Fatal("Interception erased cast accounting")
			}
			spell.Cast(sim, &c.Unit)
			if !roots[&c.Unit].IsActive() {
				t.Fatal("Grounding charge was duplicated between party members")
			}
			roots[&c.Unit].Deactivate(sim)
			grounding.ApplyEffects(sim, &c.Unit, grounding)
			spell.ForeverSingleTargetHarmful = false
			spell.Cast(sim, &c.Unit)
			if !roots[&c.Unit].IsActive() || !c.GetAura("Forever Grounding Totem").IsActive() {
				t.Fatal("Unclassified non-damaging spell was incorrectly intercepted")
			}
			roots[&c.Unit].Deactivate(sim)
			spell.ForeverSingleTargetHarmful = true
			spell.Flags |= core.SpellFlagHelpful
			spell.Cast(sim, &c.Unit)
			if !roots[&c.Unit].IsActive() || !c.GetAura("Forever Grounding Totem").IsActive() {
				t.Fatal("Helpful spell was intercepted")
			}
		})
	}
}
