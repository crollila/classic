package core

import (
	"slices"
	"sort"
	"time"

	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
)

// All mechanics in this file are PROVISIONAL. Numbers come from the manifest;
// Classic stat dependencies, additive spell modifiers and attack tables are
// retained where interactions are not documented. Selection validates ranks.
func (c *Character) ForeverRank(id string) int32 {
	if c.Forever == nil {
		return 0
	}
	return c.Forever.Talents[id]
}
func (c *Character) ForeverParameter(name string, fallback float64) float64 {
	if c.Forever != nil {
		if value, ok := c.Forever.Parameters[name]; ok {
			return value
		}
		// Best-known default from overrides.json (request values always win).
		if value, ok := c.foreverParameterDefault(name); ok {
			return value
		}
	}
	return fallback
}
func (c *Character) HasForeverMechanic(id string) bool {
	return c.Forever != nil && slices.Contains(c.Forever.Mechanics, id)
}
func (c *Character) ForeverValue(id string, index int, classic float64) float64 {
	rank := c.ForeverRank(id)
	if rank == 0 {
		return classic
	}
	r, ok := foreverdata.Lookup(id)
	if !ok || index >= len(r.Ranks[rank-1].Values) {
		panic("missing Forever numerical value: " + id)
	}
	return r.Ranks[rank-1].Values[index]
}
func (c *Character) ForeverAction(id string) ActionID {
	tag := foreverdata.ActionTag(id)
	if tag == 0 {
		panic(id)
	}
	return ActionID{OtherID: proto.OtherAction_OtherActionForever, Tag: tag}
}
func (c *Character) ForeverMechanicRank(id string) int32 {
	if !c.HasForeverMechanic(id) {
		return 0
	}
	return max(1, c.Forever.MechanicRanks[id])
}
func (c *Character) ForeverMaxRage() float64 {
	capacity := 100 + c.ForeverValue("warrior.talent.boundless-rage", 0, 0)
	if c.HasForeverMechanic("racials.gnome.expansive-mind") {
		capacity *= 1.05
	}
	return capacity
}
func (c *Character) foreverSchoolHit(school SpellSchool, amount float64) {
	c.OnSpellRegistered(func(s *Spell) {
		if s.SpellSchool.Matches(school) && s.DefenseType == DefenseTypeMagic {
			s.BonusHitRating += amount * SpellHitRatingPerHitChance
		}
	})
}
func (c *Character) foreverAllHit(amount float64) {
	c.AddStat(stats.MeleeHit, amount*MeleeHitRatingPerHitChance)
	c.AddStat(stats.SpellHit, amount*SpellHitRatingPerHitChance)
}
func (c *Character) foreverAllCrit(amount float64) {
	c.AddStat(stats.MeleeCrit, amount*CritRatingPerCritChance)
	c.AddStat(stats.SpellCrit, amount*SpellCritRatingPerCritChance)
}

func (c *Character) applyForeverTalents() {
	if c.Forever == nil {
		return
	}
	ids := []string{}
	for id, n := range c.Forever.Talents {
		if n > 0 {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		r, _ := foreverdata.Lookup(id)
		if r.Mode != "replace" {
			continue
		}
		values := r.Ranks[c.Forever.Talents[id]-1].Values
		v := values[0]
		p := v / 100
		switch id {
		case "warrior.talent.boundless-rage": // Capacity is set when constructing the rage bar.
		case "warrior.talent.anticipation", "paladin.talent.anticipation":
			c.AddStat(stats.Defense, v)
		case "warrior.talent.precision", "paladin.talent.precision", "rogue.talent.precision":
			c.foreverAllHit(v)
		case "warrior.talent.vitality":
			c.MultiplyStat(stats.Strength, 1+p)
			c.MultiplyStat(stats.Stamina, 1+p)
		case "warrior.talent.bastion":
			c.foreverTargetMultiplier(func(s *Spell) float64 {
				if c.OffHand().WeaponType == proto.WeaponType_WeaponTypeShield {
					return 1 + p
				}
				return 1
			})
		case "warrior.talent.master-of-defense":
			metrics := c.NewRageMetrics(c.ForeverAction(id))
			MakePermanent(c.RegisterAura(Aura{Label: id, OnSpellHitTaken: func(a *Aura, sim *Simulation, s *Spell, result *SpellResult) {
				if c.OffHand().WeaponType == proto.WeaponType_WeaponTypeShield && result.Outcome.Matches(OutcomeDodge|OutcomeParry) && sim.Proc(p, id) {
					c.AddRage(sim, values[1], metrics)
				}
			}}))
		case "paladin.talent.reverence", "priest.talent.meditation", "shaman.talent.mindfulness", "mage.talent.arcane-meditation", "druid.talent.reflection":
			c.PseudoStats.SpiritRegenRateCasting += p
		case "paladin.talent.divine-precision", "priest.talent.holy-precision":
			c.foreverSchoolHit(SpellSchoolHoly, v)
		case "paladin.talent.champion-of-the-light", "shaman.talent.mental-quickness":
			c.AddStatDependency(stats.Intellect, stats.SpellPower, p)
		case "paladin.talent.crusade":
			c.PseudoStats.DamageDealtMultiplier *= 1 + p
			c.Env.RegisterPostFinalizeEffect(func() {
				for _, target := range c.Env.Encounter.Targets {
					if target.MobType == proto.MobType_MobTypeDemon || target.MobType == proto.MobType_MobTypeUndead {
						for _, at := range c.AttackTables[target.UnitIndex] {
							at.DamageDealtMultiplier *= (1 + p + values[1]/100) / (1 + p)
						}
					}
				}
			})
		case "paladin.talent.one-handed-weapon-specialization", "paladin.talent.two-handed-weapon-specialization":
			c.foreverTargetMultiplier(func(s *Spell) float64 {
				two := c.MainHand().HandType == proto.HandType_HandTypeTwoHand
				if two == (id == "paladin.talent.two-handed-weapon-specialization") && s.ProcMask.Matches(ProcMaskMelee) && s.BonusCoefficient > 0 {
					return 1 + p
				}
				return 1
			})
		case "hunter.talent.careful-aim", "shaman.talent.mental-dexterity":
			c.AddStatDependency(stats.Intellect, stats.AttackPower, p)
			if c.Class == proto.Class_ClassHunter {
				c.AddStatDependency(stats.Intellect, stats.RangedAttackPower, p)
			}
		case "hunter.talent.lightning-reflexes":
			c.MultiplyStat(stats.Agility, 1+p)
		case "hunter.talent.deflection", "rogue.talent.deflection":
			c.AddStat(stats.Parry, v)
		case "hunter.talent.lethal-attacks":
			c.AddStat(stats.MeleeCrit, v*CritRatingPerCritChance)
		case "hunter.talent.strider-kick":
			c.RegisterSpell(SpellConfig{ActionID: c.ForeverAction(id), SpellSchool: SpellSchoolPhysical, DefenseType: DefenseTypeMelee,
				ProcMask: ProcMaskMeleeMHSpecial, Flags: SpellFlagMeleeMetrics | SpellFlagAPL,
				ManaCost:         ManaCostOptions{FlatCost: 61},
				Cast:             CastConfig{DefaultCast: Cast{GCD: GCDDefault}, IgnoreHaste: true, CD: Cooldown{Timer: c.NewTimer(), Duration: 8 * time.Second}},
				DamageMultiplier: 1, ThreatMultiplier: 1, BonusCoefficient: 1,
				ExtraCastCondition: func(sim *Simulation, t *Unit) bool { return c.DistanceFromTarget <= MaxMeleeAttackDistance },
				ApplyEffects: func(sim *Simulation, t *Unit, s *Spell) {
					s.CalcAndDealDamage(sim, t, c.MHWeaponDamage(sim, s.MeleeAttackPower(t)), s.OutcomeMeleeSpecialHitAndCrit)
				}})
		case "hunter.talent.endurance-training", "hunter.talent.ferocity", "hunter.talent.unleashed-fury":
			for _, pet := range c.Pets {
				if pet.IsGuardian() {
					continue
				}
				switch id {
				case "hunter.talent.endurance-training":
					pet.MultiplyStat(stats.Health, 1+p)
					pet.MultiplyStat(stats.Armor, 1+p)
				case "hunter.talent.ferocity":
					pet.AddStat(stats.MeleeCrit, v*CritRatingPerCritChance)
					pet.AddStat(stats.SpellCrit, v*SpellCritRatingPerCritChance)
				case "hunter.talent.unleashed-fury":
					pet.PseudoStats.DamageDealtMultiplier *= 1 + p
				}
			}
		case "hunter.talent.bestial-discipline":
			c.PseudoStats.SpiritRegenRateCasting += values[1] / 100
			for _, pet := range c.Pets {
				if !pet.IsGuardian() {
					MakePermanent(pet.RegisterAura(Aura{Label: id, OnInit: func(a *Aura, s *Simulation) { pet.AddFocusRegenMultiplier(p) }}))
				}
			}
		case "hunter.talent.focused-fire", "hunter.talent.lone-wolf":
			// Predicate is checked at damage calculation, so despawning and resummoning
			// pets cannot leave a stale multiplier behind.
			c.foreverTargetMultiplier(func(s *Spell) float64 {
				active := false
				for _, pet := range c.Pets {
					if !pet.IsGuardian() && pet.IsActive() {
						active = true
					}
				}
				applies := active
				if id == "hunter.talent.lone-wolf" {
					applies = !active && s.ProcMask.Matches(ProcMaskMeleeOrRanged)
				}
				if applies {
					return 1 + p
				}
				return 1
			})
		case "rogue.talent.malice", "shaman.talent.thundering-strikes", "druid.talent.nature-s-majesty":
			c.foreverAllCrit(v)
		case "priest.talent.mental-strength", "shaman.talent.ancestral-knowledge":
			c.MultiplyStat(stats.Intellect, 1+p)
		case "priest.talent.spiritual-guidance":
			c.AddStatDependency(stats.Spirit, stats.HealingPower, p)
			c.AddStatDependency(stats.Spirit, stats.SpellDamage, values[1]/100)
		case "priest.talent.shadow-focus":
			c.foreverSchoolHit(SpellSchoolShadow, v)
		case "priest.talent.shadow-affinity":
			c.OnSpellRegistered(func(s *Spell) {
				if s.SpellSchool.Matches(SpellSchoolShadow) {
					s.ThreatMultiplier *= 1 - p
				}
			})
		case "priest.talent.improved-mind-flay": // Spell family IDs in the Classic engine; coefficient unchanged provisionally.
			c.OnSpellRegistered(func(s *Spell) {
				if slices.Contains([]int32{15407, 17311, 17312, 17313, 17314, 18807}, s.SpellID) {
					s.DamageMultiplier *= 1 + p
				}
			})
		case "shaman.talent.anticipation":
			c.AddStat(stats.Dodge, v)
		case "shaman.talent.toughness":
			c.MultiplyStat(stats.Stamina, 1+p)
		case "shaman.talent.natural-grace":
			c.OnSpellRegistered(func(s *Spell) {
				if s.DefenseType == DefenseTypeMagic || s.ProcMask.Matches(ProcMaskSpellHealing) {
					s.ThreatMultiplier *= 1 - p
				}
			})
		case "shaman.talent.rage-of-the-farseer":
			action := c.ForeverAction(id)
			aura := c.RegisterAura(Aura{Label: id, ActionID: action, Duration: 25 * time.Second,
				OnGain:   func(a *Aura, sim *Simulation) { c.MultiplyMeleeSpeed(sim, 1.3); c.MultiplyCastSpeed(1.3) },
				OnExpire: func(a *Aura, sim *Simulation) { c.MultiplyMeleeSpeed(sim, 1/1.3); c.MultiplyCastSpeed(1 / 1.3) }})
			spell := c.RegisterSpell(SpellConfig{ActionID: action, Flags: SpellFlagAPL | SpellFlagHelpful,
				Cast:         CastConfig{CD: Cooldown{Timer: c.NewTimer(), Duration: 3 * time.Minute}},
				ApplyEffects: func(sim *Simulation, t *Unit, s *Spell) { aura.Activate(sim) }})
			c.AddMajorCooldown(MajorCooldown{Spell: spell, Type: CooldownTypeDPS})
		case "mage.talent.arcane-focus":
			c.foreverSchoolHit(SpellSchoolArcane, v)
		case "mage.talent.elemental-precision":
			c.foreverSchoolHit(SpellSchoolFire|SpellSchoolFrost, v)
		case "mage.talent.arcane-resilience":
			c.AddStatDependency(stats.Intellect, stats.Armor, p)
		case "mage.talent.arcane-impact":
			c.OnSpellRegistered(func(s *Spell) {
				if s.SpellSchool.Matches(SpellSchoolArcane) && s.DefenseType == DefenseTypeMagic {
					s.BonusCritRating += v * SpellCritRatingPerCritChance
				}
			})
		case "mage.talent.arcane-mind":
			c.MultiplyStat(stats.Intellect, 1+p)
			c.OnSpellRegistered(func(s *Spell) {
				if s.SpellSchool.Matches(SpellSchoolArcane) {
					s.CritDamageBonus += values[1] / 100
				}
			})
		case "warlock.talent.suppression":
			c.foreverAllHit(v)
			c.PseudoStats.ThreatMultiplier *= 1 - values[1]/100
		case "warlock.talent.demonic-embrace":
			c.MultiplyStat(stats.Stamina, 1+p)
		case "warlock.talent.unholy-power":
			for _, pet := range c.Pets {
				if !pet.IsGuardian() {
					pet.PseudoStats.DamageDealtMultiplier *= 1 + p
				}
			}
		case "warlock.talent.fel-vitality":
			c.MultiplyStat(stats.Mana, 1+values[1]/100)
			for _, pet := range c.Pets {
				if !pet.IsGuardian() {
					pet.MultiplyStat(stats.Health, 1+p)
					pet.MultiplyStat(stats.Mana, 1+p)
				}
			}
		case "warlock.talent.demonic-knowledge":
			aura := c.NewTemporaryStatsAura(id, c.ForeverAction(id), stats.Stats{stats.SpellPower: p * float64(c.Level)}, NeverExpires)
			for _, pet := range c.Pets {
				if !pet.IsGuardian() {
					pet.ApplyOnPetEnable(func(sim *Simulation) { aura.Activate(sim) })
					old := pet.OnPetDisable
					pet.OnPetDisable = func(sim *Simulation) {
						if old != nil {
							old(sim)
						}
						aura.Deactivate(sim)
					}
				}
			}
		case "warlock.talent.shadow-mastery":
			c.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexShadow] *= 1 + p
		case "warlock.talent.molten-skin":
			c.PseudoStats.DamageTakenMultiplier *= 1 - p
		case "warlock.talent.malevolence":
			c.OnSpellRegistered(func(s *Spell) {
				if s.SpellSchool.Matches(SpellSchoolShadow) {
					s.BonusCritRating += v * SpellCritRatingPerCritChance
				}
			})
		case "druid.talent.living-spirit":
			c.MultiplyStat(stats.Spirit, 1+p)
		case "druid.talent.moonglow":
			c.OnSpellRegistered(func(s *Spell) {
				if s.Cost != nil && s.Cost.CostType() == CostTypeMana {
					s.Cost.Multiplier -= int32(v)
				}
			})
		case "druid.talent.moonfury":
			c.OnSpellRegistered(func(s *Spell) {
				if s.ProcMask.Matches(ProcMaskSpellDamage) && s.SpellSchool.Matches(SpellSchoolArcane|SpellSchoolNature) {
					s.DamageMultiplierAdditive += p
				}
			})
		default:
			panic("missing Forever adapter: " + id)
		}
	}
}

func (c *Character) foreverTargetMultiplier(effect func(*Spell) float64) {
	c.Env.RegisterPostFinalizeEffect(func() {
		for _, target := range c.Env.Encounter.Targets {
			for _, at := range c.AttackTables[target.UnitIndex] {
				old := at.DamageDoneByCasterMultiplier
				at.DamageDoneByCasterMultiplier = func(s *Spell, a *AttackTable) float64 {
					m := effect(s)
					if old != nil {
						m *= old(s, a)
					}
					return m
				}
			}
		}
	})
}

func (c *Character) applyForeverBuffs(raid *proto.RaidBuffs) {
	if !c.HasForeverMechanic("buffs.first-aid-kit") {
		return
	}
	// Preapplied scenario with explicit remaining duration; Fortitude wins the
	// Classic Stamina-buff exclusivity rule. Predicted setup is BEST_GUESS only.
	if raid.GetPowerWordFortitude() != proto.TristateEffect_TristateEffectMissing {
		return
	}
	c.AddStat(stats.Stamina, 34)
	duration := c.ForeverParameter("scenario.campsite_seconds_remaining", 1800) * (1 + .5*float64(c.ForeverMechanicRank("legacy.permanence")))
	auraDuration := time.Duration(duration * float64(time.Second))
	if _, supplied := c.Forever.Parameters["scenario.campsite_seconds_remaining"]; foreverdata.IsStrict(c.Forever) && !supplied {
		// STRICT uses the Classic preapplied-buff snapshot. An explicitly supplied
		// remaining lifetime is an encounter input, not an inferred Forever duration.
		auraDuration = NeverExpires
	}
	aura := c.foreverTimedStats("buffs.first-aid-kit", stats.Stats{stats.Stamina: 34}, auraDuration)
	if foreverdata.IsStrict(c.Forever) {
		return
	}
	c.RegisterSpell(SpellConfig{ActionID: c.ForeverAction("buffs.camping"), Flags: SpellFlagAPL | SpellFlagHelpful | SpellFlagPrepullOnly,
		Cast:               CastConfig{DefaultCast: Cast{CastTime: 5 * time.Second}, CD: Cooldown{Timer: c.NewTimer(), Duration: time.Duration(float64(time.Hour) * (1 - .08*float64(c.ForeverMechanicRank("legacy.field-guide"))))}},
		ExtraCastCondition: func(sim *Simulation, t *Unit) bool { return sim.CurrentTime < 0 && !c.IsMoving() }, ApplyEffects: func(sim *Simulation, t *Unit, s *Spell) { aura.Activate(sim) }})
}
