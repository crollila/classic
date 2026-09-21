package enhancement

import (
	"testing"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/simsignals"
	"github.com/wowsims/classic/sim/core/stats"
	"github.com/wowsims/classic/sim/shaman"
)

// A shaman of the given level with a two-hand mace (Black Malice) and the given imbue.
func levelShaman(t *testing.T, level int32, imbue proto.WeaponImbue) (*core.Simulation, *shaman.Shaman) {
	t.Helper()
	equipment := &proto.EquipmentSpec{Items: make([]*proto.ItemSpec, 17)}
	for i := range equipment.Items {
		equipment.Items[i] = &proto.ItemSpec{}
	}
	equipment.Items[proto.ItemSlot_ItemSlotMainHand] = &proto.ItemSpec{Id: 3194}
	player := &proto.Player{
		Name: "Shaman", Class: proto.Class_ClassShaman, Race: proto.Race_RaceOrc, Level: level,
		Equipment: equipment,
		Consumes:  &proto.Consumes{MainHandImbue: imbue},
		Buffs:     &proto.IndividualBuffs{},
		Rotation:  &proto.APLRotation{Type: proto.APLRotation_TypeAPL},
		Spec:      &proto.Player_EnhancementShaman{EnhancementShaman: &proto.EnhancementShaman{Options: &proto.EnhancementShaman_Options{}}},
	}
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid:       &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{player}}}, Buffs: &proto.RaidBuffs{}, Debuffs: &proto.Debuffs{}},
		Encounter:  &proto.Encounter{Duration: 60, Targets: []*proto.Target{{Level: level + 2, MobType: proto.MobType_MobTypeHumanoid, Stats: stats.Stats{stats.Armor: 850}.ToFloatArray()}}},
		SimOptions: &proto.SimOptions{Iterations: 1, RandomSeed: 1, IsTest: true},
	}, simsignals.Signals{})
	sim.Reset()
	return sim, sim.Raid.Parties[0].Players[0].(shaman.ShamanAgent).GetShaman()
}

// highestRank is the spell id of the highest registered rank in a rank-indexed spell slice.
func highestRank(spells []*core.Spell) int32 {
	id := int32(0)
	for _, spell := range spells {
		if spell != nil {
			id = spell.SpellID
		}
	}
	return id
}

func TestShamanLevel20Ranks(t *testing.T) {
	sim, s := levelShaman(t, 20, proto.WeaponImbue_RockbiterWeapon)
	for name, c := range map[string]struct {
		spells []*core.Spell
		want   int32
	}{
		"Lightning Bolt":          {s.LightningBolt, 915},
		"Chain Lightning":         {s.ChainLightning, 0},
		"Earth Shock":             {s.EarthShock, 8045},
		"Flame Shock":             {s.FlameShock, 8052},
		"Frost Shock":             {s.FrostShock, 8056},
		"Lightning Shield":        {s.LightningShield, 325},
		"Searing Totem":           {s.SearingTotem, 6363},
		"Magma Totem":             {s.MagmaTotem, 0},
		"Fire Nova Totem":         {s.FireNovaTotem, 1535},
		"Strength of Earth Totem": {s.StrengthOfEarthTotem, 8075},
		"Stoneskin Totem":         {s.StoneskinTotem, 8154},
		"Healing Stream Totem":    {s.HealingStreamTotem, 5394},
		"Mana Spring Totem":       {s.ManaSpringTotem, 0},
		"Windfury Totem":          {s.WindfuryTotem, 0},
		"Grace of Air Totem":      {s.GraceOfAirTotem, 0},
	} {
		if got := highestRank(c.spells); got != c.want {
			t.Errorf("%s: highest registered rank %d, want %d", name, got, c.want)
		}
	}
	if s.TremorTotem == nil || s.TremorTotem.SpellID != 8143 {
		t.Errorf("Tremor Totem (learned at 18) not registered")
	}

	// Strength of Earth rank 1 is a class-local buff of 10 Strength, not core's rank 5.
	if s.GetAuraByID(core.ActionID{SpellID: 25361}) != nil || s.GetAuraByID(core.ActionID{SpellID: 8075}) == nil {
		t.Errorf("Strength of Earth self buff is not the rank 1 aura")
	} else {
		before := s.GetStat(stats.Strength)
		s.GetAuraByID(core.ActionID{SpellID: 8075}).Activate(sim)
		if got := s.GetStat(stats.Strength) - before; got < 9.99 || got > 10.01 {
			t.Errorf("Strength of Earth rank 1 gave %.2f Strength, want 10", got)
		}
	}
	// Rockbiter rank 3 (level 16) enchant.
	if got := s.MainHand().TempEnchant; got != shaman.RockbiterWeaponEnchantId[3] {
		t.Errorf("Rockbiter enchant %d, want rank 3 (%d)", got, shaman.RockbiterWeaponEnchantId[3])
	}
}

func TestShamanLevel20Imbues(t *testing.T) {
	_, s := levelShaman(t, 20, proto.WeaponImbue_FlametongueWeapon)
	if s.GetSpell(core.ActionID{SpellID: 8027}) == nil || s.GetSpell(core.ActionID{SpellID: 16342}) != nil {
		t.Errorf("Flametongue Weapon should be rank 2 (8027) at level 20")
	}
	_, s = levelShaman(t, 20, proto.WeaponImbue_FrostbrandWeapon)
	if s.GetSpell(core.ActionID{SpellID: 8033}) == nil || s.GetSpell(core.ActionID{SpellID: 16356}) != nil {
		t.Errorf("Frostbrand Weapon should be rank 1 (8033) at level 20")
	}
	// Windfury Weapon is learned at 30: nothing registers and nothing panics.
	_, s = levelShaman(t, 20, proto.WeaponImbue_WindfuryWeapon)
	if s.WindfuryWeaponMH != nil {
		t.Errorf("Windfury Weapon registered at level 20")
	}
	// Level 9: no Flametongue rank yet.
	_, s = levelShaman(t, 9, proto.WeaponImbue_FlametongueWeapon)
	if s.MainHand().TempEnchant != 0 {
		t.Errorf("Flametongue Weapon applied below level 10")
	}
}

func TestShamanLevel60Ranks(t *testing.T) {
	_, s := levelShaman(t, 60, proto.WeaponImbue_WindfuryWeapon)
	for name, c := range map[string]struct {
		spells []*core.Spell
		want   int32
	}{
		"Lightning Bolt":          {s.LightningBolt, 15208},
		"Chain Lightning":         {s.ChainLightning, 10605},
		"Earth Shock":             {s.EarthShock, 10414},
		"Searing Totem":           {s.SearingTotem, 10438},
		"Strength of Earth Totem": {s.StrengthOfEarthTotem, 25361},
		"Grace of Air Totem":      {s.GraceOfAirTotem, 25359},
	} {
		if got := highestRank(c.spells); got != c.want {
			t.Errorf("%s: highest registered rank %d, want %d", name, got, c.want)
		}
	}
	if s.WindfuryWeaponMH == nil || s.WindfuryWeaponMH.SpellID != 16362 {
		t.Errorf("Windfury Weapon should be rank 4 (16362) at level 60")
	}
	if s.GetAuraByID(core.ActionID{SpellID: 25361}) == nil {
		t.Errorf("Strength of Earth should use core's rank 5 aura at level 60")
	}
}
