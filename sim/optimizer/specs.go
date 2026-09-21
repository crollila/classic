package optimizer

import (
	"slices"

	"github.com/wowsims/classic/sim/core/proto"
)

// SpecDef is the Go twin of ui/app/specs.ts: what the Forever app sims for a spec and what
// the class can equip at a level. Keep the two in step (TestSpecsMatchApp checks the keys,
// rotations and proficiency lists against specs.ts).
type SpecDef struct {
	Key, Label string
	Class      proto.Class
	Role       string // melee | ranged | caster
	Rotations  []RotationDef
	Spec       func(level int32) interface{} // a proto.Player_<Spec> for core.WithSpec
	Armor      func(level int32) proto.ArmorType
	Weapons    []proto.WeaponType
	TwoHand    bool
	DualWield  func(level int32) bool
	Ranged     []proto.RangedWeaponType
	Shield     bool
}

// RotationDef is an APL file under ui/ (Path without the .apl.json suffix).
type RotationDef struct {
	Label, Path string
}

const (
	wAxe     = proto.WeaponType_WeaponTypeAxe
	wDagger  = proto.WeaponType_WeaponTypeDagger
	wFist    = proto.WeaponType_WeaponTypeFist
	wMace    = proto.WeaponType_WeaponTypeMace
	wOffHand = proto.WeaponType_WeaponTypeOffHand
	wPolearm = proto.WeaponType_WeaponTypePolearm
	wShield  = proto.WeaponType_WeaponTypeShield
	wStaff   = proto.WeaponType_WeaponTypeStaff
	wSword   = proto.WeaponType_WeaponTypeSword

	rBow      = proto.RangedWeaponType_RangedWeaponTypeBow
	rCrossbow = proto.RangedWeaponType_RangedWeaponTypeCrossbow
	rGun      = proto.RangedWeaponType_RangedWeaponTypeGun
	rIdol     = proto.RangedWeaponType_RangedWeaponTypeIdol
	rLibram   = proto.RangedWeaponType_RangedWeaponTypeLibram
	rThrown   = proto.RangedWeaponType_RangedWeaponTypeThrown
	rTotem    = proto.RangedWeaponType_RangedWeaponTypeTotem
	rWand     = proto.RangedWeaponType_RangedWeaponTypeWand

	cloth   = proto.ArmorType_ArmorTypeCloth
	leather = proto.ArmorType_ArmorTypeLeather
	mail    = proto.ArmorType_ArmorTypeMail
	plate   = proto.ArmorType_ArmorTypePlate
)

func always(v proto.ArmorType) func(int32) proto.ArmorType { return func(int32) proto.ArmorType { return v } }
func from40(low, high proto.ArmorType) func(int32) proto.ArmorType {
	return func(l int32) proto.ArmorType {
		if l >= 40 {
			return high
		}
		return low
	}
}
func never(int32) bool     { return false }
func from20(l int32) bool  { return l >= 20 }
func alwaysDW(int32) bool  { return true }

// Specs lists the DPS specs of the Forever app, in the app's order.
var Specs = []*SpecDef{
	{
		Key: "warrior", Label: "Warrior", Class: proto.Class_ClassWarrior, Role: "melee",
		Rotations: []RotationDef{{"Leveling (any level)", "warrior/apls/leveling"}, {"Level 60 Fury/Arms", "warrior/apls/dps_no_reck"}},
		Spec: func(l int32) interface{} {
			stance := proto.WarriorStance_WarriorStanceBattle
			if l >= 30 {
				stance = proto.WarriorStance_WarriorStanceBerserker
			}
			return &proto.Player_Warrior{Warrior: &proto.Warrior{Options: &proto.Warrior_Options{
				StartingRage: 0, QueueDelay: 250, Shout: proto.WarriorShout_WarriorShoutBattle, Stance: stance}}}
		},
		Armor:   from40(mail, plate),
		Weapons: []proto.WeaponType{wAxe, wDagger, wFist, wMace, wPolearm, wStaff, wSword}, TwoHand: true, DualWield: from20,
		Ranged: []proto.RangedWeaponType{rBow, rCrossbow, rGun, rThrown}, Shield: true,
	},
	{
		Key: "rogue", Label: "Rogue", Class: proto.Class_ClassRogue, Role: "melee",
		Rotations: []RotationDef{{"Sinister Strike", "rogue/apls/leveling_sinister_strike"}, {"Backstab (dagger)", "rogue/apls/leveling_backstab"},
			{"Level 60 Sinister Strike", "rogue/apls/combat_sinister_strike"}, {"Level 60 Backstab", "rogue/apls/combat_backstab"}},
		Spec: func(int32) interface{} {
			return &proto.Player_Rogue{Rogue: &proto.Rogue{Options: &proto.RogueOptions{}}}
		},
		Armor:   always(leather),
		Weapons: []proto.WeaponType{wAxe, wDagger, wFist, wMace, wSword}, TwoHand: false, DualWield: alwaysDW,
		Ranged: []proto.RangedWeaponType{rBow, rCrossbow, rGun, rThrown}, Shield: false,
	},
	{
		Key: "hunter", Label: "Hunter", Class: proto.Class_ClassHunter, Role: "ranged",
		Rotations: []RotationDef{{"Leveling (any level)", "hunter/apls/leveling"}, {"Level 60", "hunter/apls/p1"}},
		Spec: func(int32) interface{} {
			return &proto.Player_Hunter{Hunter: &proto.Hunter{Options: &proto.Hunter_Options{
				PetType: proto.Hunter_Options_Cat, PetUptime: 1, PetAttackSpeed: proto.Hunter_Options_OneTwo}}}
		},
		Armor:   from40(leather, mail),
		Weapons: []proto.WeaponType{wAxe, wDagger, wFist, wPolearm, wStaff, wSword}, TwoHand: true, DualWield: from20,
		Ranged: []proto.RangedWeaponType{rBow, rCrossbow, rGun}, Shield: false,
	},
	{
		Key: "mage", Label: "Mage", Class: proto.Class_ClassMage, Role: "caster",
		Rotations: []RotationDef{{"Fireball", "mage/apls/leveling_fireball"}, {"Frostbolt", "mage/apls/leveling_frostbolt"}, {"Level 60 Fire", "mage/apls/p1"}},
		Spec: func(l int32) interface{} {
			armor := proto.Mage_Options_NoArmor
			if l >= 34 {
				armor = proto.Mage_Options_MageArmor
			}
			return &proto.Player_Mage{Mage: &proto.Mage{Options: &proto.Mage_Options{Armor: armor}}}
		},
		Armor:   always(cloth),
		Weapons: []proto.WeaponType{wDagger, wStaff, wSword, wOffHand}, TwoHand: true, DualWield: never,
		Ranged: []proto.RangedWeaponType{rWand}, Shield: false,
	},
	{
		Key: "warlock", Label: "Warlock", Class: proto.Class_ClassWarlock, Role: "caster",
		Rotations: []RotationDef{{"Default", "warlock/apls/rotation"}},
		Spec: func(l int32) interface{} {
			summon := proto.WarlockOptions_Imp
			if l >= 20 {
				summon = proto.WarlockOptions_Succubus
			}
			return &proto.Player_Warlock{Warlock: &proto.Warlock{Options: &proto.WarlockOptions{Armor: proto.WarlockOptions_DemonArmor, Summon: summon}}}
		},
		Armor:   always(cloth),
		Weapons: []proto.WeaponType{wDagger, wStaff, wSword, wOffHand}, TwoHand: true, DualWield: never,
		Ranged: []proto.RangedWeaponType{rWand}, Shield: false,
	},
	{
		Key: "shadow_priest", Label: "Shadow Priest", Class: proto.Class_ClassPriest, Role: "caster",
		Rotations: []RotationDef{{"Default", "shadow_priest/apls/p1"}},
		Spec: func(int32) interface{} {
			return &proto.Player_ShadowPriest{ShadowPriest: &proto.ShadowPriest{Options: &proto.ShadowPriest_Options{}}}
		},
		Armor:   always(cloth),
		Weapons: []proto.WeaponType{wDagger, wMace, wStaff, wOffHand}, TwoHand: true, DualWield: never,
		Ranged: []proto.RangedWeaponType{rWand}, Shield: false,
	},
	{
		Key: "balance_druid", Label: "Balance Druid", Class: proto.Class_ClassDruid, Role: "caster",
		Rotations: []RotationDef{{"Default", "balance_druid/apls/balance"}},
		Spec: func(int32) interface{} {
			return &proto.Player_BalanceDruid{BalanceDruid: &proto.BalanceDruid{Options: &proto.BalanceDruid_Options{}}}
		},
		Armor:   always(leather),
		Weapons: []proto.WeaponType{wDagger, wFist, wMace, wStaff, wOffHand}, TwoHand: true, DualWield: never,
		Ranged: []proto.RangedWeaponType{rIdol}, Shield: false,
	},
	{
		Key: "feral_druid", Label: "Feral Druid", Class: proto.Class_ClassDruid, Role: "melee",
		Rotations: []RotationDef{{"Leveling (any level)", "feral_druid/apls/leveling"}, {"Level 60", "feral_druid/apls/feral"}},
		Spec: func(int32) interface{} {
			return &proto.Player_FeralDruid{FeralDruid: &proto.FeralDruid{Options: &proto.FeralDruid_Options{LatencyMs: 100}}}
		},
		Armor:   always(leather),
		Weapons: []proto.WeaponType{wDagger, wFist, wMace, wStaff, wOffHand}, TwoHand: true, DualWield: never,
		Ranged: []proto.RangedWeaponType{rIdol}, Shield: false,
	},
	{
		Key: "retribution_paladin", Label: "Retribution Paladin", Class: proto.Class_ClassPaladin, Role: "melee",
		Rotations: []RotationDef{{"Default", "retribution_paladin/apls/basic_ret"}},
		Spec: func(l int32) interface{} {
			aura := proto.PaladinAura_NoPaladinAura
			if l >= 16 {
				aura = proto.PaladinAura_RetributionAura
			}
			return &proto.Player_RetributionPaladin{RetributionPaladin: &proto.RetributionPaladin{Options: &proto.PaladinOptions{
				Aura: aura, PrimarySeal: proto.PaladinSeal_Righteousness}}}
		},
		Armor:   from40(mail, plate),
		Weapons: []proto.WeaponType{wAxe, wMace, wPolearm, wSword}, TwoHand: true, DualWield: never,
		Ranged: []proto.RangedWeaponType{rLibram}, Shield: true,
	},
	{
		Key: "elemental_shaman", Label: "Elemental Shaman", Class: proto.Class_ClassShaman, Role: "caster",
		Rotations: []RotationDef{{"Default", "elemental_shaman/apls/default"}},
		Spec: func(int32) interface{} {
			return &proto.Player_ElementalShaman{ElementalShaman: &proto.ElementalShaman{Options: &proto.ElementalShaman_Options{}}}
		},
		Armor:   from40(leather, mail),
		Weapons: []proto.WeaponType{wAxe, wDagger, wFist, wMace, wStaff, wOffHand}, TwoHand: true, DualWield: never,
		Ranged: []proto.RangedWeaponType{rTotem}, Shield: true,
	},
	{
		Key: "enhancement_shaman", Label: "Enhancement Shaman", Class: proto.Class_ClassShaman, Role: "melee",
		Rotations: []RotationDef{{"Default", "enhancement_shaman/apls/default"}},
		Spec: func(int32) interface{} {
			return &proto.Player_EnhancementShaman{EnhancementShaman: &proto.EnhancementShaman{Options: &proto.EnhancementShaman_Options{}}}
		},
		Armor:   from40(leather, mail),
		Weapons: []proto.WeaponType{wAxe, wDagger, wFist, wMace, wStaff}, TwoHand: true, DualWield: never,
		Ranged: []proto.RangedWeaponType{rTotem}, Shield: true,
	},
}

// SpecByKey returns the spec definition for an app key (e.g. "warrior", "rogue").
func SpecByKey(key string) *SpecDef {
	for _, s := range Specs {
		if s.Key == key {
			return s
		}
	}
	return nil
}

func (s *SpecDef) usesWeapon(t proto.WeaponType) bool       { return slices.Contains(s.Weapons, t) }
func (s *SpecDef) usesRanged(t proto.RangedWeaponType) bool { return slices.Contains(s.Ranged, t) }
