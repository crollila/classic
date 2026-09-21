package shaman

import (
	"math"
	"time"

	"github.com/wowsims/classic/sim/core"
)

const StrengthOfEarthTotemRanks = 5

var StrengthOfEarthTotemSpellId = [StrengthOfEarthTotemRanks + 1]int32{0, 8075, 8160, 8161, 10442, 25361}
var StrengthOfEarthTotemManaCost = [StrengthOfEarthTotemRanks + 1]float64{0, 25, 65, 125, 225, 275}
var StrengthOfEarthTotemLevel = [StrengthOfEarthTotemRanks + 1]int{0, 10, 24, 38, 52, 60}

func (shaman *Shaman) registerStrengthOfEarthTotemSpell() {
	shaman.StrengthOfEarthTotem = make([]*core.Spell, StrengthOfEarthTotemRanks+1)

	for rank := 1; rank <= StrengthOfEarthTotemRanks; rank++ {
		config := shaman.newStrengthOfEarthTotemSpellConfig(rank)

		if config.RequiredLevel <= int(shaman.Level) {
			shaman.StrengthOfEarthTotem[rank] = shaman.RegisterSpell(config)
		}
	}

	shaman.EarthTotems = append(
		shaman.EarthTotems,
		core.FilterSlice(shaman.StrengthOfEarthTotem, func(spell *core.Spell) bool { return spell != nil })...,
	)
}

func (shaman *Shaman) newStrengthOfEarthTotemSpellConfig(rank int) core.SpellConfig {
	spellId := StrengthOfEarthTotemSpellId[rank]
	manaCost := StrengthOfEarthTotemManaCost[rank]
	level := StrengthOfEarthTotemLevel[rank]

	duration := time.Second * 120
	multiplier := []float64{1, 1.08, 1.15}[shaman.Talents.EnhancingTotems]

	// Core models the top rank. Below its level every registered rank shares one aura
	// holding the highest known rank's value, so the APL sees a single buff.
	var buffAura *core.Aura
	if known := rankAtLevel(StrengthOfEarthTotemLevel[:], shaman.Level); known >= StrengthOfEarthTotemRanks {
		buffAura = core.StrengthOfEarthTotemAura(&shaman.Unit, multiplier)
	} else {
		buffAura = shaman.GetAura("Strength of Earth Totem (Shaman)")
		if buffAura == nil {
			buffAura = shaman.lowRankStatTotemAura("Strength of Earth Totem (Shaman)", StrengthOfEarthTotemSpellId[known], core.StrengthOfEarth,
				StrengthOfEarthTotemStrength[known]/StrengthOfEarthTotemStrength[StrengthOfEarthTotemRanks], multiplier)
		}
	}

	spell := shaman.newTotemSpellConfig(manaCost, spellId)
	spell.RequiredLevel = level
	spell.Rank = rank
	spell.ApplyEffects = func(sim *core.Simulation, _ *core.Unit, spell *core.Spell) {
		shaman.TotemExpirations[EarthTotem] = sim.CurrentTime + duration
		shaman.ActiveTotems[EarthTotem] = spell

		buffAura.Activate(sim)
	}
	return spell
}

const StoneskinTotemRanks = 6

var StoneskinTotemSpellId = [StoneskinTotemRanks + 1]int32{0, 8071, 8154, 8155, 10406, 10407, 10408}
var StoneskinTotemManaCost = [StoneskinTotemRanks + 1]float64{0, 30, 60, 90, 115, 160, 210}
var StoneskinTotemLevel = [StoneskinTotemRanks + 1]int{0, 4, 14, 24, 34, 44, 54}

func (shaman *Shaman) registerStoneskinTotemSpell() {
	shaman.StoneskinTotem = make([]*core.Spell, StoneskinTotemRanks+1)

	for rank := 1; rank <= StoneskinTotemRanks; rank++ {
		config := shaman.newStoneskinTotemSpellConfig(rank)

		if config.RequiredLevel <= int(shaman.Level) {
			shaman.StoneskinTotem[rank] = shaman.RegisterSpell(config)
		}
	}

	shaman.EarthTotems = append(
		shaman.EarthTotems,
		core.FilterSlice(shaman.StoneskinTotem, func(spell *core.Spell) bool { return spell != nil })...,
	)
}

func (shaman *Shaman) newStoneskinTotemSpellConfig(rank int) core.SpellConfig {
	spellId := StoneskinTotemSpellId[rank]
	manaCost := StoneskinTotemManaCost[rank]
	level := StoneskinTotemLevel[rank]

	duration := time.Second * 120

	var stoneskinAura *core.Aura
	spell := shaman.newTotemSpellConfig(manaCost, spellId)
	spell.RequiredLevel = level
	spell.Rank = rank
	if shaman.Forever == nil {
		// Core's Stoneskin aura is the top rank; below it use the highest known rank's value.
		if known := rankAtLevel(StoneskinTotemLevel[:], shaman.Level); known < StoneskinTotemRanks {
			stoneskinAura = shaman.GetAura("Stoneskin (Shaman)")
			if stoneskinAura == nil {
				reduction := -math.Floor(StoneskinTotemReduction[known] * (1 + .1*float64(shaman.Talents.GuardianTotems)))
				stoneskinAura = shaman.RegisterAura(core.Aura{
					Label:    "Stoneskin (Shaman)",
					ActionID: core.ActionID{SpellID: StoneskinTotemSpellId[known]},
					Duration: core.NeverExpires,
					OnGain: func(aura *core.Aura, sim *core.Simulation) {
						aura.Unit.PseudoStats.BonusDamageTakenAfterModifiers[core.DefenseTypeMelee] += reduction
					},
					OnExpire: func(aura *core.Aura, sim *core.Simulation) {
						aura.Unit.PseudoStats.BonusDamageTakenAfterModifiers[core.DefenseTypeMelee] -= reduction
					},
				})
			}
		}
	}
	if shaman.Forever != nil {
		aura := shaman.RegisterAura(core.Aura{Label: "Forever Stoneskin-" + core.ActionID{SpellID: spellId}.String(), ActionID: core.ActionID{SpellID: spellId}, Duration: duration})
		amount := []float64{0, 4, 7, 11, 16, 22, 30}[rank] * (1 + .1*shaman.fr("guardian-totems"))
		for _, agent := range shaman.Party.Players {
			agent.GetCharacter().AddDynamicDamageTakenModifier(func(sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
				active := shaman.ActiveTotems[EarthTotem]
				if aura.IsActive() && active != nil && active.SpellID == spellId && sp.DefenseType == core.DefenseTypeMelee {
					r.Damage = max(0, r.Damage-amount)
				}
			})
		}
		spell.ApplyEffects = func(sim *core.Simulation, _ *core.Unit, sp *core.Spell) {
			shaman.TotemExpirations[EarthTotem] = sim.CurrentTime + duration
			shaman.ActiveTotems[EarthTotem] = sp
			aura.Activate(sim)
		}
		return spell
	}
	spell.ApplyEffects = func(sim *core.Simulation, _ *core.Unit, spell *core.Spell) {
		shaman.TotemExpirations[EarthTotem] = sim.CurrentTime + duration
		shaman.ActiveTotems[EarthTotem] = spell

		if stoneskinAura == nil {
			stoneskinAura = core.StoneskinTotemAura(&shaman.Unit, shaman.Talents.GuardianTotems)
		}
		stoneskinAura.Activate(sim)
	}
	return spell
}

func (shaman *Shaman) registerTremorTotemSpell() {
	spellId := int32(8143)
	manaCost := float64(60)
	duration := time.Second * 120
	level := 18
	if int(shaman.Level) < level {
		return
	}

	spell := shaman.newTotemSpellConfig(manaCost, spellId)
	spell.RequiredLevel = level
	spell.ApplyEffects = func(sim *core.Simulation, _ *core.Unit, spell *core.Spell) {
		shaman.TotemExpirations[EarthTotem] = sim.CurrentTime + duration
		shaman.ActiveTotems[EarthTotem] = spell
	}
	shaman.TremorTotem = shaman.RegisterSpell(spell)
	shaman.EarthTotems = append(shaman.EarthTotems, shaman.TremorTotem)
}
