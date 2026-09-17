package core

import (
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
	googleproto "google.golang.org/protobuf/proto"
	"time"
)

// Buff lifetime inputs describe time remaining at pull. Longer Legacy duration
// is applied to that scenario. The Classic path keeps its original permanent buffs.
func (c *Character) foreverTimedStats(id string, delta stats.Stats, duration time.Duration) *Aura {
	return c.foreverTimedStatsNamed(id, id, delta, duration)
}
func (c *Character) foreverTimedStatsNamed(id, label string, delta stats.Stats, duration time.Duration) *Aura {
	applied := true
	a := c.RegisterAura(Aura{Label: label, ActionID: c.ForeverAction(id), Duration: duration,
		OnReset: func(a *Aura, sim *Simulation) { applied = true; a.Activate(sim) },
		OnGain: func(a *Aura, sim *Simulation) {
			if !applied {
				c.AddStatsDynamic(sim, delta)
				applied = true
			}
		},
		OnExpire: func(a *Aura, sim *Simulation) {
			if applied {
				c.AddStatsDynamic(sim, delta.Multiply(-1))
				applied = false
			}
		},
	})
	return a
}
func (c *Character) applyForeverFoodDuration(before stats.Stats) {
	if c.Forever == nil || c.Consumes.Food == proto.Food_FoodUnknown {
		return
	}
	delta := c.stats.Subtract(before)
	duration := c.ForeverParameter("scenario.food_seconds_remaining", 900) * (1 + .33*float64(c.ForeverMechanicRank("legacy.gourmand")))
	c.foreverTimedStats("legacy.gourmand", delta, time.Duration(duration*float64(time.Second)))
}
func (c *Character) applyForeverProfessions() {
	if c.Forever == nil {
		return
	}
	if c.HasForeverMechanic("profession.mining.made-of-metal") && c.HasProfession(proto.Profession_Mining) {
		c.MultiplyStat(stats.Health, 1.05)
	}
	if c.HasForeverMechanic("profession.skinning.beasts-of-the-wild") && c.HasProfession(proto.Profession_Skinning) {
		c.foreverMobDamage(proto.MobType_MobTypeBeast, 1.05)
		c.foreverMobDamage(proto.MobType_MobTypeDragonkin, 1.05)
	}
	if c.HasForeverMechanic("profession.herbalism.natural-talent") && c.HasProfession(proto.Profession_Herbalism) {
		c.AddResistances(c.ForeverParameter("profession.herbalism.resistance", 10))
	}
	if c.HasForeverMechanic("profession.enchanting.ring-enchants") && c.HasProfession(proto.Profession_Enchanting) {
		count := 0.
		for _, slot := range []proto.ItemSlot{proto.ItemSlot_ItemSlotFinger1, proto.ItemSlot_ItemSlotFinger2} {
			if c.Equipment[slot].ID != 0 {
				count++
			}
		}
		c.AddStat(stats.SpellPower, count*c.ForeverParameter("profession.enchanting.spell_power_per_ring", 12))
	}
	if c.HasForeverMechanic("profession.blacksmithing.belt-buckles") && c.HasProfession(proto.Profession_Blacksmithing) && c.Equipment[proto.ItemSlot_ItemSlotWaist].ID != 0 {
		attribute := stats.Strength
		if c.Class == proto.Class_ClassHunter || c.Class == proto.Class_ClassRogue || c.Class == proto.Class_ClassDruid {
			attribute = stats.Agility
		} else if c.HasManaBar() {
			attribute = stats.Intellect
		}
		c.AddStat(attribute, c.ForeverParameter("profession.blacksmithing.socket_stat", 8))
	}
	if c.HasForeverMechanic("profession.tailoring.embroider-your-work") && c.HasProfession(proto.Profession_Tailoring) && c.Equipment[proto.ItemSlot_ItemSlotBack].ID != 0 {
		c.AddStat(stats.SpellPower, c.ForeverParameter("profession.tailoring.embroidery_spell_power", 15))
	}
	if c.HasForeverMechanic("profession.leatherworking.armor-kits") && c.HasProfession(proto.Profession_Leatherworking) {
		for _, slot := range []proto.ItemSlot{proto.ItemSlot_ItemSlotChest, proto.ItemSlot_ItemSlotLegs, proto.ItemSlot_ItemSlotHands, proto.ItemSlot_ItemSlotFeet} {
			item := c.Equipment[slot]
			if item.ID != 0 && (item.ArmorType == proto.ArmorType_ArmorTypeLeather || item.ArmorType == proto.ArmorType_ArmorTypeMail) {
				c.AddStat(stats.Armor, 40)
			}
		}
	}
	for _, cleanse := range []struct{ id, kind string }{{"consumes.woolen-tourniquet", "bleed"}, {"consumes.simple-poultice", "disease"}, {"consumes.anti-venom", "poison"}} {
		if !c.HasForeverMechanic(cleanse.id) {
			continue
		}
		cooldown := time.Minute // PREDICTED repeat-use timing; known cleanse is usable once in STRICT.
		if foreverdata.IsStrict(c.Forever) {
			cooldown = NeverExpires
		}
		s := c.foreverCooldown(cleanse.id, cooldown, CooldownTypeSurvival, func(sim *Simulation, t *Unit, s *Spell) { c.ForeverDispel(sim, cleanse.kind) })
		s.ExtraCastCondition = func(sim *Simulation, t *Unit) bool {
			for _, a := range c.GetAuras() {
				if a.IsActive() && foreverDebuffKind(a) == cleanse.kind {
					return true
				}
			}
			return false
		}
	}
}
func (c *Character) applyForeverMixology(before stats.Stats) {
	if c.Forever == nil {
		return
	}
	if c.HasForeverMechanic("profession.alchemy.mixology") && c.HasProfession(proto.Profession_Alchemy) {
		c.AddStats(c.stats.Subtract(before).Multiply(c.ForeverParameter("profession.alchemy.effect_bonus", .25)))
	}
	c.foreverFlaskStats = c.stats.Subtract(before)
}
func (c *Character) foreverStrictBuffs(input *proto.IndividualBuffs) *proto.IndividualBuffs {
	if !foreverdata.IsStrict(c.Forever) {
		return input
	}
	out := googleproto.Clone(input).(*proto.IndividualBuffs)
	out.RallyingCryOfTheDragonslayer = false
	out.SpiritOfZandalar = false
	out.SongflowerSerenade = false
	out.WarchiefsBlessing = false
	out.SaygesFortune = proto.SaygesFortune_SaygesUnknown
	out.FengusFerocity = false
	out.MoldarsMoxie = false
	out.SlipkiksSavvy = false
	return out
}

func (c *Character) applyForeverClassBuffDuration(raid *proto.RaidBuffs) {
	if c.Forever == nil {
		return
	}
	delta := stats.Stats{}
	if raid.ArcaneBrilliance {
		delta = delta.Add(BuffSpellValues[ArcaneIntellect])
	}
	if raid.DivineSpirit {
		delta = delta.Add(BuffSpellValues[DivineSpirit])
	}
	if raid.PowerWordFortitude > 0 {
		amount := BuffSpellValues[PowerWordFortitude]
		if raid.PowerWordFortitude == proto.TristateEffect_TristateEffectImproved {
			amount = amount.Multiply(1.3).Floor()
		}
		delta = delta.Add(amount)
	}
	if raid.GiftOfTheWild > 0 {
		amount := BuffSpellValues[MarkOfTheWild]
		if raid.GiftOfTheWild == proto.TristateEffect_TristateEffectImproved {
			amount = amount.Multiply(1.35).Floor()
		}
		if raid.NatureResistanceTotem || raid.AspectOfTheWild {
			amount[stats.NatureResistance] = 0
		}
		if raid.FireResistanceAura || raid.FireResistanceTotem {
			amount[stats.FireResistance] = 0
		}
		if raid.FrostResistanceAura || raid.FrostResistanceTotem {
			amount[stats.FrostResistance] = 0
		}
		if raid.ShadowResistanceAura || raid.ShadowProtection {
			amount[stats.ShadowResistance] = 0
		}
		delta = delta.Add(amount)
	}
	duration := c.ForeverParameter("scenario.class_buff_seconds_remaining", 1800) * (1 + .5*float64(c.ForeverMechanicRank("legacy.permanence")))
	c.foreverTimedStats("legacy.permanence", delta, time.Duration(duration*float64(time.Second)))
}

func (c *Character) addForeverElixirStats(value stats.Stats) {
	if c.HasForeverMechanic("profession.alchemy.mixology") && c.HasProfession(proto.Profession_Alchemy) {
		value = value.Multiply(1 + c.ForeverParameter("profession.alchemy.effect_bonus", .25))
	}
	c.AddStats(value)
	if c.Forever != nil {
		c.foreverElixirStats = c.foreverElixirStats.Add(value)
	}
}

func (c *Character) finishForeverAlchemyDuration() {
	if c.Forever == nil {
		return
	}
	multiplier := 1.0
	if c.HasForeverMechanic("profession.alchemy.mixology") && c.HasProfession(proto.Profession_Alchemy) {
		multiplier = 2
	} // PREDICTED: expansion Mixology duration analogue.
	if c.foreverFlaskStats != (stats.Stats{}) {
		c.foreverTimedStatsNamed("profession.alchemy.mixology", "Forever flask duration", c.foreverFlaskStats, time.Duration(c.ForeverParameter("scenario.flask_seconds_remaining", 7200)*multiplier*float64(time.Second)))
	}
	if c.foreverElixirStats != (stats.Stats{}) {
		c.foreverTimedStatsNamed("profession.alchemy.mixology", "Forever elixir duration", c.foreverElixirStats, time.Duration(c.ForeverParameter("scenario.elixir_seconds_remaining", 3600)*multiplier*float64(time.Second)))
	}
}
