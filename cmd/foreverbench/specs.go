package main

import (
	"github.com/wowsims/classic/sim/core/proto"
)

// SpecConfig mirrors one core.CharacterSuiteConfig from the spec Go tests.
// Values are copied from the test files named in Source; buffs are always
// core.FullBuffs (every test uses it). Gear/APL files are resolved at runtime
// relative to -root so they stay in sync with the repository.
type SpecConfig struct {
	ID          string
	Class       proto.Class
	Spec        string
	Role        string
	Race        proto.Race
	Talents     string
	GearDir     string // ui/<dir>/gear_sets
	GearSet     string
	AplDir      string // ui/<dir>/apls
	Apl         string
	Phase       int32
	Consumes    *proto.Consumes
	SpecOptions interface{} // *proto.Player_<Spec> oneof wrapper, for core.WithSpec
	Source      string
	Notes       []string
}

var warriorDpsOptions = &proto.Warrior_Options{
	StartingRage: 50,
	Shout:        proto.WarriorShout_WarriorShoutBattle,
}

var specTable = []SpecConfig{
	{
		ID: "warrior-fury", Class: proto.Class_ClassWarrior, Spec: "Fury", Role: "dps",
		Race: proto.Race_RaceOrc, Talents: "30305001302-05050005525010051",
		GearDir: "warrior", GearSet: "phase_1", AplDir: "warrior", Apl: "dps_reck", Phase: 1,
		Consumes: &proto.Consumes{
			AgilityElixir:     proto.AgilityElixir_ElixirOfTheMongoose,
			AttackPowerBuff:   proto.AttackPowerBuff_JujuMight,
			DefaultPotion:     proto.Potions_MightyRagePotion,
			DragonBreathChili: true,
			Food:              proto.Food_FoodSmokedDesertDumpling,
			MainHandImbue:     proto.WeaponImbue_Windfury,
			OffHandImbue:      proto.WeaponImbue_ElementalSharpeningStone,
			StrengthBuff:      proto.StrengthBuff_JujuPower,
		},
		SpecOptions: &proto.Player_Warrior{Warrior: &proto.Warrior{Options: warriorDpsOptions}},
		Source:      "sim/warrior/dps_warrior/dps_warrior_test.go TestP1DPSWarrior",
		Notes:       []string{"The repo has no separate Arms test; PlayerOptionsArms is defined but unused."},
	},
	{
		ID: "rogue-combat-swords", Class: proto.Class_ClassRogue, Spec: "Combat (Swords)", Role: "dps",
		Race: proto.Race_RaceHuman, Talents: "005323105-0240052020050150231",
		GearDir: "rogue", GearSet: "combat_sinister_strike_prebis", AplDir: "rogue", Apl: "combat_sinister_strike", Phase: 5,
		Consumes: &proto.Consumes{
			AgilityElixir:   proto.AgilityElixir_ElixirOfTheMongoose,
			MainHandImbue:   proto.WeaponImbue_Windfury,
			OffHandImbue:    proto.WeaponImbue_InstantPoison,
			StrengthBuff:    proto.StrengthBuff_JujuPower,
			AttackPowerBuff: proto.AttackPowerBuff_JujuMight,
		},
		SpecOptions: &proto.Player_Rogue{Rogue: &proto.Rogue{Options: &proto.RogueOptions{}}},
		Source:      "sim/rogue/dps_rogue/dps_rogue_test.go TestCombatSinisterStrike",
		Notes:       []string{"Pre-BiS gear set."},
	},
	{
		ID: "rogue-combat-daggers", Class: proto.Class_ClassRogue, Spec: "Combat (Daggers)", Role: "dps",
		Race: proto.Race_RaceHuman, Talents: "005023104-0233050020550100221-05",
		GearDir: "rogue", GearSet: "combat_backstab_prebis", AplDir: "rogue", Apl: "combat_backstab", Phase: 5,
		Consumes: &proto.Consumes{
			AgilityElixir:   proto.AgilityElixir_ElixirOfTheMongoose,
			MainHandImbue:   proto.WeaponImbue_Windfury,
			OffHandImbue:    proto.WeaponImbue_InstantPoison,
			StrengthBuff:    proto.StrengthBuff_JujuPower,
			AttackPowerBuff: proto.AttackPowerBuff_JujuMight,
		},
		SpecOptions: &proto.Player_Rogue{Rogue: &proto.Rogue{Options: &proto.RogueOptions{}}},
		Source:      "sim/rogue/dps_rogue/dps_rogue_test.go TestCombatDaggers",
		Notes:       []string{"Pre-BiS gear set."},
	},
	{
		ID: "hunter-marksmanship", Class: proto.Class_ClassHunter, Spec: "Marksmanship/Survival", Role: "dps",
		Race: proto.Race_RaceOrc, Talents: "-05451002503051-33400023023",
		GearDir: "hunter", GearSet: "p0.bis", AplDir: "hunter", Apl: "p1", Phase: 1,
		Consumes: &proto.Consumes{
			AgilityElixir:     proto.AgilityElixir_ElixirOfTheMongoose,
			AttackPowerBuff:   proto.AttackPowerBuff_JujuMight,
			DefaultPotion:     proto.Potions_ManaPotion,
			DragonBreathChili: true,
			Flask:             proto.Flask_FlaskOfSupremePower,
			Food:              proto.Food_FoodSagefishDelight,
			MainHandImbue:     proto.WeaponImbue_Windfury,
			OffHandImbue:      proto.WeaponImbue_ElementalSharpeningStone,
			SpellPowerBuff:    proto.SpellPowerBuff_GreaterArcaneElixir,
			StrengthBuff:      proto.StrengthBuff_JujuPower,
		},
		SpecOptions: &proto.Player_Hunter{Hunter: &proto.Hunter{Options: &proto.Hunter_Options{
			Ammo:           proto.Hunter_Options_RazorArrow,
			PetType:        proto.Hunter_Options_Cat,
			PetUptime:      1,
			PetAttackSpeed: 2.0,
		}}},
		Source: "sim/hunter/hunter_test.go TestP1Hunter",
	},
	{
		ID: "mage-frost", Class: proto.Class_ClassMage, Spec: "Frost (21 Fire / 30 Frost)", Role: "dps",
		Race: proto.Race_RaceTroll, Talents: "-0550320003021-2035020310035105",
		GearDir: "mage", GearSet: "p0.bis", AplDir: "mage", Apl: "p1", Phase: 1,
		Consumes: &proto.Consumes{
			DefaultPotion:  proto.Potions_MajorManaPotion,
			Flask:          proto.Flask_FlaskOfSupremePower,
			FirePowerBuff:  proto.FirePowerBuff_ElixirOfGreaterFirepower,
			FrostPowerBuff: proto.FrostPowerBuff_ElixirOfFrostPower,
			Food:           proto.Food_FoodRunnTumTuberSurprise,
			MainHandImbue:  proto.WeaponImbue_BrilliantWizardOil,
			SpellPowerBuff: proto.SpellPowerBuff_GreaterArcaneElixir,
		},
		SpecOptions: &proto.Player_Mage{Mage: &proto.Mage{Options: &proto.Mage_Options{Armor: proto.Mage_Options_MoltenArmor}}},
		Source:      "sim/mage/mage_test.go TestP1Mage",
		Notes:       []string{"Only one mage test exists (Frost-heavy hybrid); no separate Fire/Arcane configs in the repo."},
	},
	{
		ID: "warlock-sm-ruin", Class: proto.Class_ClassWarlock, Spec: "Affliction (SM/Ruin)", Role: "dps",
		Race: proto.Race_RaceOrc, Talents: "5502203112201105--52500051020001",
		GearDir: "warlock", GearSet: "mc", AplDir: "warlock", Apl: "rotation", Phase: 1,
		Consumes:    warlockConsumes,
		SpecOptions: warlockOptions,
		Source:      "sim/warlock/dps/dps_warlock_test.go TestWarlockSMRuin",
	},
	{
		ID: "warlock-ds-ruin", Class: proto.Class_ClassWarlock, Spec: "Demonology (DS/Ruin)", Role: "dps",
		Race: proto.Race_RaceOrc, Talents: "25002-2050300152201-52500051020001",
		GearDir: "warlock", GearSet: "mc", AplDir: "warlock", Apl: "rotation", Phase: 1,
		Consumes:    warlockConsumes,
		SpecOptions: warlockOptions,
		Source:      "sim/warlock/dps/dps_warlock_test.go TestWarlockDSRuin",
		Notes:       []string{"Test keeps Summon=Succubus with DS talents, copied as-is."},
	},
	{
		ID: "priest-shadow", Class: proto.Class_ClassPriest, Spec: "Shadow", Role: "dps",
		Race: proto.Race_RaceUndead, Talents: "0512301302--5002504103501251",
		GearDir: "shadow_priest", GearSet: "p0.bis", AplDir: "shadow_priest", Apl: "p1", Phase: 1,
		Consumes: &proto.Consumes{
			DefaultPotion:   proto.Potions_MajorManaPotion,
			Flask:           proto.Flask_FlaskOfSupremePower,
			Food:            proto.Food_FoodRunnTumTuberSurprise,
			MainHandImbue:   proto.WeaponImbue_WizardOil,
			SpellPowerBuff:  proto.SpellPowerBuff_GreaterArcaneElixir,
			ShadowPowerBuff: proto.ShadowPowerBuff_ElixirOfShadowPower,
		},
		SpecOptions: &proto.Player_ShadowPriest{ShadowPriest: &proto.ShadowPriest{Options: &proto.ShadowPriest_Options{Armor: proto.ShadowPriest_Options_InnerFire}}},
		Source:      "sim/priest/shadow/shadow_priest_test.go TestP1Shadow",
	},
	{
		ID: "druid-balance", Class: proto.Class_ClassDruid, Spec: "Balance", Role: "dps",
		Race: proto.Race_RaceTauren, Talents: "5000550012551251--5005031",
		GearDir: "balance_druid", GearSet: "p0.bis", AplDir: "balance_druid", Apl: "p1", Phase: 1,
		Consumes: &proto.Consumes{
			DefaultPotion:  proto.Potions_MajorManaPotion,
			Flask:          proto.Flask_FlaskOfSupremePower,
			Food:           proto.Food_FoodNightfinSoup,
			MainHandImbue:  proto.WeaponImbue_BrilliantWizardOil,
			SpellPowerBuff: proto.SpellPowerBuff_GreaterArcaneElixir,
		},
		SpecOptions: &proto.Player_BalanceDruid{BalanceDruid: &proto.BalanceDruid{Options: &proto.BalanceDruid_Options{OkfUptime: 0.2}}},
		Source:      "sim/druid/balance/balance_test.go TestP1Balance",
	},
	{
		ID: "druid-feral-cat", Class: proto.Class_ClassDruid, Spec: "Feral (Cat)", Role: "dps",
		Race: proto.Race_RaceTauren, Talents: "500005301-5500020323202151-15",
		GearDir: "feral_druid", GearSet: "p0.bis", AplDir: "feral_druid", Apl: "p1", Phase: 1,
		Consumes: &proto.Consumes{
			AgilityElixir:     proto.AgilityElixir_ElixirOfTheMongoose,
			AttackPowerBuff:   proto.AttackPowerBuff_JujuMight,
			DefaultConjured:   proto.Conjured_ConjuredDemonicRune,
			DefaultPotion:     proto.Potions_MajorManaPotion,
			DragonBreathChili: true,
			Flask:             proto.Flask_FlaskOfDistilledWisdom,
			Food:              proto.Food_FoodSmokedDesertDumpling,
			MainHandImbue:     proto.WeaponImbue_ElementalSharpeningStone,
			MiscConsumes:      &proto.MiscConsumes{},
			StrengthBuff:      proto.StrengthBuff_JujuPower,
		},
		SpecOptions: &proto.Player_FeralDruid{FeralDruid: &proto.FeralDruid{Options: &proto.FeralDruid_Options{
			InnervateTarget:   &proto.UnitReference{},
			LatencyMs:         100,
			AssumeBleedActive: true,
		}}},
		Source: "sim/druid/feral/feral_test.go TestP1Feral (PlayerOptionsMonoCat)",
	},
	{
		ID: "shaman-elemental", Class: proto.Class_ClassShaman, Spec: "Elemental", Role: "dps",
		Race: proto.Race_RaceTroll, Talents: "550331050002151--50105301005",
		GearDir: "elemental_shaman", GearSet: "phase_1", AplDir: "elemental_shaman", Apl: "default", Phase: 1,
		Consumes: &proto.Consumes{
			DefaultConjured: proto.Conjured_ConjuredDemonicRune,
			DefaultPotion:   proto.Potions_MajorManaPotion,
			Flask:           proto.Flask_FlaskOfSupremePower,
			FirePowerBuff:   proto.FirePowerBuff_ElixirOfGreaterFirepower,
			Food:            proto.Food_FoodNightfinSoup,
			MainHandImbue:   proto.WeaponImbue_LesserWizardOil,
			SpellPowerBuff:  proto.SpellPowerBuff_GreaterArcaneElixir,
		},
		SpecOptions: &proto.Player_ElementalShaman{ElementalShaman: &proto.ElementalShaman{Options: &proto.ElementalShaman_Options{}}},
		Source:      "sim/shaman/elemental/elemental_test.go TestElemental (Phase 1 entry)",
	},
	{
		ID: "shaman-enhancement", Class: proto.Class_ClassShaman, Spec: "Enhancement", Role: "dps",
		Race: proto.Race_RaceTroll, Talents: "05-5025002105023051-05105301",
		GearDir: "enhancement_shaman", GearSet: "phase_1", AplDir: "enhancement_shaman", Apl: "default", Phase: 1,
		Consumes: &proto.Consumes{
			AttackPowerBuff:   proto.AttackPowerBuff_JujuMight,
			AgilityElixir:     proto.AgilityElixir_ElixirOfTheMongoose,
			DefaultConjured:   proto.Conjured_ConjuredDemonicRune,
			DefaultPotion:     proto.Potions_MajorManaPotion,
			DragonBreathChili: true,
			FirePowerBuff:     proto.FirePowerBuff_ElixirOfGreaterFirepower,
			Flask:             proto.Flask_FlaskOfSupremePower,
			Food:              proto.Food_FoodBlessSunfruit,
			MainHandImbue:     proto.WeaponImbue_WindfuryWeapon,
			SpellPowerBuff:    proto.SpellPowerBuff_GreaterArcaneElixir,
			StrengthBuff:      proto.StrengthBuff_JujuPower,
		},
		SpecOptions: &proto.Player_EnhancementShaman{EnhancementShaman: &proto.EnhancementShaman{Options: &proto.EnhancementShaman_Options{SyncType: proto.ShamanSyncType_Auto}}},
		Source:      "sim/shaman/enhancement/enhancement_test.go TestEnhancement (Phase 1 entry, Sync Auto)",
	},
	{
		ID: "paladin-retribution", Class: proto.Class_ClassPaladin, Spec: "Retribution", Role: "dps",
		Race: proto.Race_RaceHuman, Talents: "500501-503-52230351200315",
		GearDir: "retribution_paladin", GearSet: "blank", AplDir: "retribution_paladin", Apl: "basic_ret", Phase: 5,
		Consumes: &proto.Consumes{
			AgilityElixir:     proto.AgilityElixir_ElixirOfTheMongoose,
			AttackPowerBuff:   proto.AttackPowerBuff_JujuMight,
			DefaultPotion:     proto.Potions_MajorManaPotion,
			DragonBreathChili: true,
			Flask:             proto.Flask_FlaskOfSupremePower,
			FirePowerBuff:     proto.FirePowerBuff_ElixirOfFirepower,
			Food:              proto.Food_FoodSmokedDesertDumpling,
			SpellPowerBuff:    proto.SpellPowerBuff_GreaterArcaneElixir,
			StrengthBuff:      proto.StrengthBuff_JujuPower,
		},
		SpecOptions: &proto.Player_RetributionPaladin{RetributionPaladin: &proto.RetributionPaladin{Options: &proto.PaladinOptions{PrimarySeal: proto.PaladinSeal_Righteousness}}},
		Source:      "sim/paladin/retribution/retribution_test.go TestRetribution",
		Notes:       []string{"UNRELIABLE FOR RANKING: the test (and the repo) only has the 'blank' gear set, so this spec sims with no gear."},
	},
	{
		ID: "warrior-protection", Class: proto.Class_ClassWarrior, Spec: "Protection", Role: "tank",
		Race: proto.Race_RaceOrc, Talents: "20304300302-03-55200110530201051",
		GearDir: "tank_warrior", GearSet: "p0.bis", AplDir: "warrior", Apl: "dps_reck", Phase: 1,
		Consumes: &proto.Consumes{
			AgilityElixir:     proto.AgilityElixir_ElixirOfTheMongoose,
			AttackPowerBuff:   proto.AttackPowerBuff_JujuMight,
			DefaultPotion:     proto.Potions_MightyRagePotion,
			DragonBreathChili: true,
			Flask:             proto.Flask_FlaskOfTheTitans,
			Food:              proto.Food_FoodSmokedDesertDumpling,
			MainHandImbue:     proto.WeaponImbue_Windfury,
			StrengthBuff:      proto.StrengthBuff_JujuPower,
		},
		SpecOptions: &proto.Player_TankWarrior{TankWarrior: &proto.TankWarrior{Options: &proto.TankWarrior_Options{
			Shout:        proto.WarriorShout_WarriorShoutCommanding,
			StartingRage: 0,
		}}},
		Source: "sim/warrior/tank_warrior/tank_warrior_test.go TestP1TankWarrior",
		Notes:  []string{"Test uses the warrior dps_reck APL (from ui/warrior/apls), not a tank rotation; not registered as a raid tank (IsTank unset)."},
	},
	{
		ID: "paladin-protection", Class: proto.Class_ClassPaladin, Spec: "Protection", Role: "tank",
		Race: proto.Race_RaceHuman, Talents: "-053020335001551-0500535",
		GearDir: "protection_paladin", GearSet: "blank", AplDir: "protection_paladin", Apl: "basic_prot", Phase: 4,
		Consumes: &proto.Consumes{
			DefaultPotion:     proto.Potions_MajorManaPotion,
			AgilityElixir:     proto.AgilityElixir_ElixirOfTheMongoose,
			AttackPowerBuff:   proto.AttackPowerBuff_JujuMight,
			Flask:             proto.Flask_FlaskOfSupremePower,
			SpellPowerBuff:    proto.SpellPowerBuff_GreaterArcaneElixir,
			DragonBreathChili: true,
			Food:              proto.Food_FoodSmokedDesertDumpling,
			StrengthBuff:      proto.StrengthBuff_JujuPower,
		},
		SpecOptions: &proto.Player_ProtectionPaladin{ProtectionPaladin: &proto.ProtectionPaladin{Options: &proto.PaladinOptions{
			PrimarySeal:   proto.PaladinSeal_Righteousness,
			RighteousFury: true,
		}}},
		Source: "sim/paladin/protection/protection_test.go TestProtection",
		Notes:  []string{"UNRELIABLE: only the 'blank' gear set exists."},
	},
	{
		ID: "shaman-warden", Class: proto.Class_ClassShaman, Spec: "Warden (tank)", Role: "tank",
		Race: proto.Race_RaceTroll, Talents: "5203015-0505000145503151",
		GearDir: "warden_shaman", GearSet: "blank", AplDir: "warden_shaman", Apl: "default", Phase: 1,
		Consumes: &proto.Consumes{
			AttackPowerBuff:   proto.AttackPowerBuff_JujuMight,
			AgilityElixir:     proto.AgilityElixir_ElixirOfTheMongoose,
			DefaultConjured:   proto.Conjured_ConjuredDemonicRune,
			DefaultPotion:     proto.Potions_MajorManaPotion,
			DragonBreathChili: true,
			FirePowerBuff:     proto.FirePowerBuff_ElixirOfGreaterFirepower,
			Flask:             proto.Flask_FlaskOfTheTitans,
			Food:              proto.Food_FoodBlessSunfruit,
			MainHandImbue:     proto.WeaponImbue_RockbiterWeapon,
			SpellPowerBuff:    proto.SpellPowerBuff_GreaterArcaneElixir,
			StrengthBuff:      proto.StrengthBuff_JujuPower,
		},
		SpecOptions: &proto.Player_WardenShaman{WardenShaman: &proto.WardenShaman{Options: &proto.WardenShaman_Options{}}},
		Source:      "sim/shaman/warden/warden_test.go TestWardenShaman",
		Notes:       []string{"UNRELIABLE: only the 'blank' gear set exists."},
	},
}

var warlockConsumes = &proto.Consumes{
	DefaultPotion:   proto.Potions_MajorManaPotion,
	Flask:           proto.Flask_FlaskOfSupremePower,
	FirePowerBuff:   proto.FirePowerBuff_ElixirOfGreaterFirepower,
	ShadowPowerBuff: proto.ShadowPowerBuff_ElixirOfShadowPower,
	Food:            proto.Food_FoodTenderWolfSteak,
	MainHandImbue:   proto.WeaponImbue_WizardOil,
	SpellPowerBuff:  proto.SpellPowerBuff_GreaterArcaneElixir,
}

var warlockOptions = &proto.Player_Warlock{Warlock: &proto.Warlock{Options: &proto.WarlockOptions{
	Armor:       proto.WarlockOptions_DemonArmor,
	Summon:      proto.WarlockOptions_Succubus,
	WeaponImbue: proto.WarlockOptions_NoWeaponImbue,
}}}

func findSpec(id string) (SpecConfig, bool) {
	for _, s := range specTable {
		if s.ID == id {
			return s, true
		}
	}
	return SpecConfig{}, false
}
