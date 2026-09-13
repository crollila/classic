package forever_test

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/simsignals"
	"github.com/wowsims/classic/sim/core/stats"
	_ "github.com/wowsims/classic/sim/game" // registers class factories once
	"google.golang.org/protobuf/encoding/protojson"
	googleproto "google.golang.org/protobuf/proto"
)

func discoveryPlayer(t *testing.T, class, race, spec string, talents map[string]int32, mechanics ...string) *proto.Player {
	t.Helper()
	p := &proto.Player{}
	input := fmt.Sprintf(`{"class":"Class%s","race":"Race%s","%s":{"options":{}},"bonusStats":{"stats":[100,100,100,100,100,100,100]}}`, class, race, spec)
	if err := protojson.Unmarshal([]byte(input), p); err != nil {
		t.Fatal(err)
	}
	p.Equipment = &proto.EquipmentSpec{}
	p.Rotation = &proto.APLRotation{Type: proto.APLRotation_TypeAPL}
	p.Forever = &proto.ForeverOptions{RulesetId: foreverdata.RulesetID, Talents: talents, Mechanics: mechanics, ExperimentalEstimatedRanks: true}
	return p
}
func simulation(t *testing.T, p *proto.Player, buffs *proto.RaidBuffs) (*core.Simulation, *core.Character) {
	t.Helper()
	req := &proto.RaidSimRequest{Raid: &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{p}}}, Buffs: buffs}, Encounter: &proto.Encounter{Duration: 60, Targets: []*proto.Target{{Level: 63, MobType: proto.MobType_MobTypeDemon}}}, SimOptions: &proto.SimOptions{Iterations: 1, RandomSeed: 1, IsTest: true, Interactive: true}}
	sim := core.NewSim(req, simsignals.Signals{})
	sim.Reset()
	c := sim.Raid.Parties[0].Players[0].GetCharacter()
	if c.Level != 60 {
		t.Fatal("Forever changed level cap", c.Level)
	}
	return sim, c
}
func near(t *testing.T, actual, want float64) {
	t.Helper()
	if math.Abs(actual-want) > 1e-8 {
		t.Fatalf("got %.12f want %.12f", actual, want)
	}
}

func TestDiscoveryClassStatChanges(t *testing.T) {
	tests := []struct {
		class, race, spec, id string
		stat                  stats.Stat
		value                 float64
		ratio                 bool
	}{
		{"Warrior", "Human", "warrior", "warrior.talent.anticipation", stats.Defense, 4, false},
		{"Paladin", "Human", "retributionPaladin", "paladin.talent.divine-strength", stats.Strength, 1.02, true},
		{"Hunter", "Troll", "hunter", "hunter.talent.lethal-attacks", stats.MeleeCrit, 1, false},
		{"Rogue", "Human", "rogue", "rogue.talent.malice", stats.SpellCrit, 1, false},
		{"Priest", "Human", "shadowPriest", "priest.talent.holy-specialization", stats.Strength, 1, true},
		{"Shaman", "Troll", "elementalShaman", "shaman.talent.ancestral-knowledge", stats.Intellect, 1.02, true},
		{"Mage", "Gnome", "mage", "mage.talent.arcane-focus", stats.Strength, 1, true},
		{"Warlock", "Gnome", "warlock", "warlock.talent.demonic-embrace", stats.Stamina, 1.03, true},
		{"Druid", "NightElf", "balanceDruid", "druid.talent.improved-wrath", stats.Strength, 1, true},
	}
	// Some deep talents cannot yet be selected legally. Test available early rows;
	// the report independently marks unreachable adapters as BLOCKED.
	for _, tc := range tests {
		t.Run(tc.class, func(t *testing.T) {
			base := discoveryPlayer(t, tc.class, tc.race, tc.spec, map[string]int32{})
			_, b := simulation(t, base, nil)
			changed := googleproto.Clone(base).(*proto.Player)
			changed.Forever.Talents = map[string]int32{tc.id: 1}
			_, c := simulation(t, changed, nil)
			want := b.GetStat(tc.stat) + tc.value
			if tc.ratio {
				want = b.GetStat(tc.stat) * tc.value
			}
			near(t, c.GetStat(tc.stat), want)
			checked := 0
			if tc.class == "Mage" {
				for _, s := range c.Spellbook {
					if s.SpellID == 5143 {
						near(t, s.BonusHitRating, 1)
						checked++
					}
				}
			}
			if tc.class == "Priest" {
				for _, s := range c.Spellbook {
					if s.SpellID == 10934 {
						near(t, s.BonusCritRating, 1)
						checked++
					}
				}
			}
			if tc.class == "Druid" {
				for _, s := range c.Spellbook {
					if s.SpellID == 9912 {
						near(t, float64(s.DefaultCast.CastTime), float64(1900*time.Millisecond))
						near(t, float64(s.Cost.Multiplier), 90)
						checked++
					}
				}
			}
			if (tc.class == "Mage" || tc.class == "Priest" || tc.class == "Druid") && checked == 0 {
				t.Fatal("expected registered spell was not tested")
			}
		})
	}
}
func TestGnomeResourceCapsDoNotChangeIntellect(t *testing.T) {
	for _, tc := range []struct{ class, spec string }{{"Mage", "mage"}, {"Warrior", "warrior"}, {"Rogue", "rogue"}} {
		t.Run(tc.class, func(t *testing.T) {
			p := discoveryPlayer(t, tc.class, "Gnome", tc.spec, map[string]int32{}, "racials.gnome.expansive-mind")
			_, c := simulation(t, p, nil)
			old := googleproto.Clone(p).(*proto.Player)
			old.Forever = nil
			_, b := simulation(t, old, nil)
			near(t, c.GetStat(stats.Intellect), b.GetStat(stats.Intellect)/1.05)
			if tc.class == "Rogue" {
				near(t, c.MaxEnergy(), 105)
			}
			if tc.class == "Warrior" {
				near(t, c.MaxRage(), 105)
			}
			if tc.class == "Mage" && c.MaxMana() <= 0 {
				t.Fatal("missing mana")
			}
		})
	}
}
func TestWarriorCapsCostsAndBloodrage(t *testing.T) {
	p := discoveryPlayer(t, "Warrior", "Human", "warrior", map[string]int32{
		"warrior.talent.cruelty": 5, "warrior.talent.unbridled-wrath": 5, "warrior.talent.boundless-rage": 3,
		"warrior.talent.anticipation": 5, "warrior.talent.improved-bloodrage": 2, "warrior.talent.improved-thunder-clap": 1,
	})
	sim, c := simulation(t, p, nil)
	near(t, c.MaxRage(), 130)
	c.AddRage(sim, 200, c.NewRageMetrics(core.ActionID{OtherID: proto.OtherAction_OtherActionRageGain}))
	near(t, c.CurrentRage(), 130)
	c.SpendRage(sim, 130, c.NewRageMetrics(core.ActionID{OtherID: proto.OtherAction_OtherActionRageGain}))
	bloodrage := c.GetSpell(core.ActionID{SpellID: 2687})
	if bloodrage == nil || !bloodrage.Cast(sim, &c.Unit) {
		t.Fatal("Bloodrage unavailable")
	}
	near(t, c.CurrentRage(), 15)
	// Two cooldown families retain their Classic identity for existing APLs.
	for _, s := range c.Spellbook {
		if s.SpellID == 11581 {
			near(t, s.DefaultCast.Cost, 18)
		}
	}
}
func TestFirstAidKitExcludesFortitude(t *testing.T) {
	p := discoveryPlayer(t, "Mage", "Gnome", "mage", map[string]int32{})
	_, base := simulation(t, p, nil)
	p.Forever.Mechanics = []string{"buffs.first-aid-kit"}
	_, camp := simulation(t, p, nil)
	near(t, camp.GetStat(stats.Stamina)-base.GetStat(stats.Stamina), 34)
	fort := &proto.RaidBuffs{PowerWordFortitude: proto.TristateEffect_TristateEffectRegular}
	_, both := simulation(t, p, fort)
	p.Forever.Mechanics = nil
	_, only := simulation(t, p, fort)
	near(t, both.GetStat(stats.Stamina), only.GetStat(stats.Stamina))
}
func TestDemonicSacrificeSchoolsAndDuration(t *testing.T) {
	for _, tc := range []struct {
		summon proto.WarlockOptions_Summon
		label  string
		school stats.SchoolIndex
	}{{proto.WarlockOptions_Imp, "Burning Wish", stats.SchoolIndexShadow}, {proto.WarlockOptions_Succubus, "Touch of Shadow", stats.SchoolIndexFire}} {
		p := discoveryPlayer(t, "Warlock", "Gnome", "warlock", map[string]int32{"warlock.talent.demonic-embrace": 5, "warlock.talent.unholy-power": 5, "warlock.talent.demonic-sacrifice": 1})
		p.GetWarlock().Options.Summon = tc.summon
		sim, c := simulation(t, p, nil)
		s := c.GetSpell(core.ActionID{SpellID: 18788})
		if s == nil || !s.Cast(sim, &c.Unit) {
			t.Fatal("sacrifice unavailable")
		}
		a := c.GetAura(tc.label)
		if a.Duration != 2*time.Hour || !a.IsActive() {
			t.Fatal(a)
		}
		near(t, c.PseudoStats.SchoolDamageDealtMultiplier[tc.school], 1.15)
		a.Deactivate(sim)
		near(t, c.PseudoStats.SchoolDamageDealtMultiplier[tc.school], 1)
	}
}

func TestFarseerIsSelfHasteWithObservedDurationAndCooldown(t *testing.T) {
	p := discoveryPlayer(t, "Shaman", "Troll", "enhancementShaman", map[string]int32{
		"shaman.talent.thundering-strikes": 5, "shaman.talent.ancestral-knowledge": 5,
		"shaman.talent.mental-dexterity": 3, "shaman.talent.improved-lightning-shield": 3,
		"shaman.talent.anticipation": 3, "shaman.talent.toughness": 5, "shaman.talent.flurry": 5,
		"shaman.talent.mental-quickness": 2, "shaman.talent.rage-of-the-farseer": 1,
	})
	sim, c := simulation(t, p, nil)
	beforeMelee := c.PseudoStats.MeleeSpeedMultiplier
	beforeCast := c.PseudoStats.CastSpeedMultiplier
	beforeRanged := c.PseudoStats.RangedSpeedMultiplier
	spell := c.GetSpell(c.ForeverAction("shaman.talent.rage-of-the-farseer"))
	if spell == nil || spell.CD.Duration != 3*time.Minute || !spell.Cast(sim, &c.Unit) {
		t.Fatal("missing self haste cooldown")
	}
	near(t, c.PseudoStats.MeleeSpeedMultiplier, beforeMelee*1.3)
	near(t, c.PseudoStats.CastSpeedMultiplier, beforeCast*1.3)
	near(t, c.PseudoStats.RangedSpeedMultiplier, beforeRanged)
	aura := c.GetAura("shaman.talent.rage-of-the-farseer")
	if aura.Duration != 25*time.Second {
		t.Fatal(aura.Duration)
	}
	aura.Deactivate(sim)
	near(t, c.PseudoStats.MeleeSpeedMultiplier, beforeMelee)
	near(t, c.PseudoStats.CastSpeedMultiplier, beforeCast)
}

func TestRogueMultiRankVigorAndWeaponDamage(t *testing.T) {
	p := discoveryPlayer(t, "Rogue", "Human", "rogue", map[string]int32{
		"rogue.talent.malice": 5, "rogue.talent.ruthlessness": 3, "rogue.talent.murder": 2,
		"rogue.talent.lethality": 5, "rogue.talent.relentless-strikes": 1, "rogue.talent.vile-poisons": 5, "rogue.talent.vigor": 2,
	})
	_, c := simulation(t, p, nil)
	near(t, c.MaxEnergy(), 110)
	p.Forever.Talents["rogue.talent.vigor"] = 1
	_, c = simulation(t, p, nil)
	near(t, c.MaxEnergy(), 105)
}

func TestDemonicSacrificeRegenerationAndResummoning(t *testing.T) {
	for _, summon := range []proto.WarlockOptions_Summon{proto.WarlockOptions_Voidwalker, proto.WarlockOptions_Felhunter} {
		p := discoveryPlayer(t, "Warlock", "Gnome", "warlock", map[string]int32{"warlock.talent.demonic-embrace": 5, "warlock.talent.unholy-power": 5, "warlock.talent.demonic-sacrifice": 1})
		p.GetWarlock().Options.Summon = summon
		sim, c := simulation(t, p, nil)
		s := c.GetSpell(core.ActionID{SpellID: 18788})
		if !s.Cast(sim, &c.Unit) {
			t.Fatal("cannot sacrifice")
		}
		label := "Fel Stamina"
		if summon == proto.WarlockOptions_Felhunter {
			label = "Fel Energy"
		}
		aura := c.GetAura(label)
		if aura.Duration != 2*time.Hour || !aura.IsActive() {
			t.Fatal("sacrifice not active")
		}
		// Summoning any pet must cancel the effect without a documented Pact adapter.
		for _, pet := range c.Pets {
			if !pet.IsGuardian() {
				pet.OnPetEnable(sim)
				break
			}
		}
		if aura.IsActive() {
			t.Fatal("Demonic Sacrifice survived a new demon without Demonic Pact")
		}
	}
}

func TestDemonicSacrificeFourSecondResourceTicks(t *testing.T) {
	for _, summon := range []proto.WarlockOptions_Summon{proto.WarlockOptions_Voidwalker, proto.WarlockOptions_Felhunter} {
		run := func(active bool) (float64, float64, float64, float64) {
			p := discoveryPlayer(t, "Warlock", "Gnome", "warlock", map[string]int32{"warlock.talent.demonic-embrace": 5, "warlock.talent.unholy-power": 5, "warlock.talent.demonic-sacrifice": 1})
			p.GetWarlock().Options.Summon = summon
			sim, c := simulation(t, p, nil)
			c.SpendMana(sim, c.MaxMana()/2, c.NewManaMetrics(core.ActionID{OtherID: proto.OtherAction_OtherActionManaGain}))
			c.RemoveHealth(sim, c.MaxHealth()/2)
			if !c.GetSpell(core.ActionID{SpellID: 18788}).Cast(sim, &c.Unit) {
				t.Fatal("cannot sacrifice")
			}
			if !active {
				if summon == proto.WarlockOptions_Voidwalker {
					c.GetAura("Fel Stamina").Deactivate(sim)
				} else {
					c.GetAura("Fel Energy").Deactivate(sim)
				}
			}
			for i := 0; i < 10000 && sim.CurrentTime < 4100*time.Millisecond; i++ {
				if sim.Step() {
					break
				}
			}
			if sim.CurrentTime < 4*time.Second || sim.CurrentTime >= 8*time.Second {
				t.Fatal("failed to isolate first periodic tick", sim.CurrentTime)
			}
			return c.CurrentMana(), c.CurrentHealth(), c.MaxMana(), c.MaxHealth()
		}
		mana, health, maxMana, maxHealth := run(true)
		baseMana, baseHealth, _, _ := run(false)
		if summon == proto.WarlockOptions_Voidwalker {
			near(t, mana-baseMana, maxMana*.02)
			near(t, health-baseHealth, 0)
		} else {
			near(t, health-baseHealth, maxHealth*.03)
			near(t, mana-baseMana, 0)
		}
	}
}
