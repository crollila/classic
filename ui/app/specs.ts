/// <reference types="vite/client" />
// The DPS specs the Forever app offers, with what each one needs to build a request:
// level-appropriate class options, the rotation the site ships, and what the class can equip.
import { Player } from '../core/proto/api';
import { APLRotation } from '../core/proto/apl';
import { Class } from '../core/proto/common';
import { BalanceDruid, BalanceDruid_Options, FeralDruid, FeralDruid_Options } from '../core/proto/druid';
import { Hunter, Hunter_Options, Hunter_Options_PetAttackSpeed, Hunter_Options_PetType } from '../core/proto/hunter';
import { Mage, Mage_Options, Mage_Options_ArmorType } from '../core/proto/mage';
import { PaladinAura, PaladinOptions, PaladinSeal, RetributionPaladin } from '../core/proto/paladin';
import { ShadowPriest, ShadowPriest_Options } from '../core/proto/priest';
import { Rogue, RogueOptions } from '../core/proto/rogue';
import { ElementalShaman, ElementalShaman_Options, EnhancementShaman, EnhancementShaman_Options } from '../core/proto/shaman';
import { Warlock, WarlockOptions, WarlockOptions_Armor, WarlockOptions_Summon } from '../core/proto/warlock';
import { Warrior, Warrior_Options, WarriorShout, WarriorStance } from '../core/proto/warrior';

import balanceApl from '../balance_druid/apls/balance.apl.json?raw';
import elementalApl from '../elemental_shaman/apls/default.apl.json?raw';
import enhancementApl from '../enhancement_shaman/apls/default.apl.json?raw';
import feralLevelApl from '../feral_druid/apls/leveling.apl.json?raw';
import feralApl from '../feral_druid/apls/feral.apl.json?raw';
import hunterLevelApl from '../hunter/apls/leveling.apl.json?raw';
import hunterApl from '../hunter/apls/p1.apl.json?raw';
import mageFireballApl from '../mage/apls/leveling_fireball.apl.json?raw';
import mageFrostboltApl from '../mage/apls/leveling_frostbolt.apl.json?raw';
import mageApl from '../mage/apls/p1.apl.json?raw';
import retApl from '../retribution_paladin/apls/basic_ret.apl.json?raw';
import rogueLevelBsApl from '../rogue/apls/leveling_backstab.apl.json?raw';
import rogueLevelApl from '../rogue/apls/leveling_sinister_strike.apl.json?raw';
import rogueBackstabApl from '../rogue/apls/combat_backstab.apl.json?raw';
import rogueApl from '../rogue/apls/combat_sinister_strike.apl.json?raw';
import shadowApl from '../shadow_priest/apls/p1.apl.json?raw';
import warlockApl from '../warlock/apls/rotation.apl.json?raw';
import warriorLevelApl from '../warrior/apls/leveling.apl.json?raw';
import warriorApl from '../warrior/apls/dps_no_reck.apl.json?raw';

export const enum Armor { Cloth = 1, Leather = 2, Mail = 3, Plate = 4 }
// proto WeaponType / RangedWeaponType values.
const W = { axe: 1, dagger: 2, fist: 3, mace: 4, offhand: 5, polearm: 6, shield: 7, staff: 8, sword: 9 };
const R = { bow: 1, crossbow: 2, gun: 3, idol: 4, libram: 5, thrown: 6, totem: 7, wand: 8 };

export interface SpecDef {
	key: string;
	label: string;
	className: string;
	cls: Class;
	rotations: Array<{ label: string; json: string }>;
	spec: (level: number) => Player['spec'];
	armor: (level: number) => Armor;
	weapons: number[];
	twoHand: boolean;
	dualWield: (level: number) => boolean;
	ranged: number[];
	shield: boolean;
	role: 'melee' | 'ranged' | 'caster';
}

export const SPECS: SpecDef[] = [
	{
		key: 'warrior', label: 'Warrior', className: 'Warrior', cls: Class.ClassWarrior, role: 'melee',
		rotations: [{ label: 'Leveling (any level)', json: warriorLevelApl }, { label: 'Level 60 Fury/Arms', json: warriorApl }],
		spec: level => ({ oneofKind: 'warrior', warrior: Warrior.create({ options: Warrior_Options.create({
			startingRage: 0, queueDelay: 250, shout: WarriorShout.WarriorShoutBattle,
			stance: level >= 30 ? WarriorStance.WarriorStanceBerserker : WarriorStance.WarriorStanceBattle }) }) }),
		armor: level => (level >= 40 ? Armor.Plate : Armor.Mail),
		weapons: [W.axe, W.dagger, W.fist, W.mace, W.polearm, W.staff, W.sword], twoHand: true, dualWield: level => level >= 20,
		ranged: [R.bow, R.crossbow, R.gun, R.thrown], shield: true,
	},
	{
		key: 'rogue', label: 'Rogue', className: 'Rogue', cls: Class.ClassRogue, role: 'melee',
		rotations: [{ label: 'Sinister Strike', json: rogueLevelApl }, { label: 'Backstab (dagger)', json: rogueLevelBsApl }, { label: 'Level 60 Sinister Strike', json: rogueApl }, { label: 'Level 60 Backstab', json: rogueBackstabApl }],
		spec: () => ({ oneofKind: 'rogue', rogue: Rogue.create({ options: RogueOptions.create({}) }) }),
		armor: () => Armor.Leather,
		// Forever lets rogues use one-handed axes.
		weapons: [W.axe, W.dagger, W.fist, W.mace, W.sword], twoHand: false, dualWield: () => true,
		ranged: [R.bow, R.crossbow, R.gun, R.thrown], shield: false,
	},
	{
		key: 'hunter', label: 'Hunter', className: 'Hunter', cls: Class.ClassHunter, role: 'ranged',
		rotations: [{ label: 'Leveling (any level)', json: hunterLevelApl }, { label: 'Level 60', json: hunterApl }],
		spec: () => ({ oneofKind: 'hunter', hunter: Hunter.create({ options: Hunter_Options.create({
			petType: Hunter_Options_PetType.Cat, petUptime: 1, petAttackSpeed: Hunter_Options_PetAttackSpeed.OneTwo }) }) }),
		armor: level => (level >= 40 ? Armor.Mail : Armor.Leather),
		weapons: [W.axe, W.dagger, W.fist, W.polearm, W.staff, W.sword], twoHand: true, dualWield: level => level >= 20,
		ranged: [R.bow, R.crossbow, R.gun], shield: false,
	},
	{
		key: 'mage', label: 'Mage', className: 'Mage', cls: Class.ClassMage, role: 'caster',
		rotations: [{ label: 'Fireball', json: mageFireballApl }, { label: 'Frostbolt', json: mageFrostboltApl }, { label: 'Level 60 Fire', json: mageApl }],
		spec: level => ({ oneofKind: 'mage', mage: Mage.create({ options: Mage_Options.create({
			armor: level >= 34 ? Mage_Options_ArmorType.MageArmor : Mage_Options_ArmorType.NoArmor }) }) }),
		armor: () => Armor.Cloth,
		weapons: [W.dagger, W.staff, W.sword, W.offhand], twoHand: true, dualWield: () => false,
		ranged: [R.wand], shield: false,
	},
	{
		key: 'warlock', label: 'Warlock', className: 'Warlock', cls: Class.ClassWarlock, role: 'caster',
		rotations: [{ label: 'Default', json: warlockApl }],
		spec: level => ({ oneofKind: 'warlock', warlock: Warlock.create({ options: WarlockOptions.create({
			armor: WarlockOptions_Armor.DemonArmor,
			summon: level >= 20 ? WarlockOptions_Summon.Succubus : WarlockOptions_Summon.Imp }) }) }),
		armor: () => Armor.Cloth,
		weapons: [W.dagger, W.staff, W.sword, W.offhand], twoHand: true, dualWield: () => false,
		ranged: [R.wand], shield: false,
	},
	{
		key: 'shadow_priest', label: 'Shadow Priest', className: 'Priest', cls: Class.ClassPriest, role: 'caster',
		rotations: [{ label: 'Default', json: shadowApl }],
		spec: () => ({ oneofKind: 'shadowPriest', shadowPriest: ShadowPriest.create({ options: ShadowPriest_Options.create({}) }) }),
		armor: () => Armor.Cloth,
		weapons: [W.dagger, W.mace, W.staff, W.offhand], twoHand: true, dualWield: () => false,
		ranged: [R.wand], shield: false,
	},
	{
		key: 'balance_druid', label: 'Balance Druid', className: 'Druid', cls: Class.ClassDruid, role: 'caster',
		rotations: [{ label: 'Default', json: balanceApl }],
		spec: () => ({ oneofKind: 'balanceDruid', balanceDruid: BalanceDruid.create({ options: BalanceDruid_Options.create({}) }) }),
		armor: () => Armor.Leather,
		weapons: [W.dagger, W.fist, W.mace, W.staff, W.offhand], twoHand: true, dualWield: () => false,
		ranged: [R.idol], shield: false,
	},
	{
		key: 'feral_druid', label: 'Feral Druid', className: 'Druid', cls: Class.ClassDruid, role: 'melee',
		rotations: [{ label: 'Leveling (any level)', json: feralLevelApl }, { label: 'Level 60', json: feralApl }],
		spec: () => ({ oneofKind: 'feralDruid', feralDruid: FeralDruid.create({ options: FeralDruid_Options.create({ latencyMs: 100 }) }) }),
		armor: () => Armor.Leather,
		weapons: [W.dagger, W.fist, W.mace, W.staff, W.offhand], twoHand: true, dualWield: () => false,
		ranged: [R.idol], shield: false,
	},
	{
		key: 'retribution_paladin', label: 'Retribution Paladin', className: 'Paladin', cls: Class.ClassPaladin, role: 'melee',
		rotations: [{ label: 'Default', json: retApl }],
		spec: level => ({ oneofKind: 'retributionPaladin', retributionPaladin: RetributionPaladin.create({ options: PaladinOptions.create({
			aura: level >= 16 ? PaladinAura.RetributionAura : PaladinAura.NoPaladinAura, primarySeal: PaladinSeal.Righteousness }) }) }),
		armor: level => (level >= 40 ? Armor.Plate : Armor.Mail),
		weapons: [W.axe, W.mace, W.polearm, W.sword], twoHand: true, dualWield: () => false,
		ranged: [R.libram], shield: true,
	},
	{
		key: 'elemental_shaman', label: 'Elemental Shaman', className: 'Shaman', cls: Class.ClassShaman, role: 'caster',
		rotations: [{ label: 'Default', json: elementalApl }],
		spec: () => ({ oneofKind: 'elementalShaman', elementalShaman: ElementalShaman.create({ options: ElementalShaman_Options.create({}) }) }),
		armor: level => (level >= 40 ? Armor.Mail : Armor.Leather),
		weapons: [W.axe, W.dagger, W.fist, W.mace, W.staff, W.offhand], twoHand: true, dualWield: () => false,
		ranged: [R.totem], shield: true,
	},
	{
		key: 'enhancement_shaman', label: 'Enhancement Shaman', className: 'Shaman', cls: Class.ClassShaman, role: 'melee',
		rotations: [{ label: 'Default', json: enhancementApl }],
		spec: () => ({ oneofKind: 'enhancementShaman', enhancementShaman: EnhancementShaman.create({ options: EnhancementShaman_Options.create({}) }) }),
		armor: level => (level >= 40 ? Armor.Mail : Armor.Leather),
		weapons: [W.axe, W.dagger, W.fist, W.mace, W.staff], twoHand: true, dualWield: () => false,
		ranged: [R.totem], shield: true,
	},
];

export function rotation(def: SpecDef, index: number): APLRotation {
	return APLRotation.fromJsonString((def.rotations[index] || def.rotations[0]).json, { ignoreUnknownFields: true });
}
