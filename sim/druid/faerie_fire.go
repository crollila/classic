package druid

import (
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/stats"
)

// Faerie Fire ranks: spell id, Faerie Fire (Feral) id, learned level, mana, armor.
var faerieFireRanks = []struct {
	id, feralID, level int32
	mana, armor        float64
}{
	{770, 16857, 18, 55, 175},
	{778, 17390, 30, 75, 285},
	{9749, 17391, 42, 95, 395},
	{9907, 17392, 54, 115, 505},
}

// faerieFireRankAura mirrors core.FaerieFireAura for a rank below the top one.
func faerieFireRankAura(target *core.Unit, label string, spellID int32, armor float64) *core.Aura {
	aura := target.GetOrRegisterAura(core.Aura{
		Label:    label,
		ActionID: core.ActionID{SpellID: spellID},
		Duration: time.Second * 40,
	})
	aura.NewExclusiveEffect("MinorArmorReduction", true, core.ExclusiveEffect{
		Priority: armor,
		OnGain: func(ee *core.ExclusiveEffect, sim *core.Simulation) {
			ee.Aura.Unit.AddStatDynamic(sim, stats.Armor, -armor)
		},
		OnExpire: func(ee *core.ExclusiveEffect, sim *core.Simulation) {
			ee.Aura.Unit.AddStatDynamic(sim, stats.Armor, armor)
		},
	})
	return aura
}

func (druid *Druid) registerFaerieFireSpell() {
	rank := -1
	for i, r := range faerieFireRanks {
		if r.level <= druid.Level {
			rank = i
		}
	}
	if rank < 0 {
		return // learned at level 18
	}
	ffRank := faerieFireRanks[rank]
	topRank := rank == len(faerieFireRanks)-1

	spellCode := SpellCode_DruidFaerieFire
	actionID := core.ActionID{SpellID: ffRank.id}
	manaCostOptions := core.ManaCostOptions{
		FlatCost: ffRank.mana,
	}
	gcd := core.GCDDefault
	ignoreHaste := false
	cd := core.Cooldown{}
	flatThreatBonus := 2. * float64(ffRank.level)
	flags := core.SpellFlagNone
	formMask := Humanoid | Moonkin

	druid.FaerieFireAuras = druid.NewEnemyAuraArray(func(target *core.Unit) *core.Aura {
		if !topRank {
			return faerieFireRankAura(target, "Faerie Fire", ffRank.id, ffRank.armor)
		}
		return core.FaerieFireAura(target)
	})

	if druid.InForm(Cat|Bear) && druid.Talents.FaerieFireFeral {
		spellCode = SpellCode_DruidFaerieFireFeral
		actionID = core.ActionID{SpellID: ffRank.feralID}
		manaCostOptions = core.ManaCostOptions{}
		gcd = time.Second
		ignoreHaste = true
		formMask = Cat | Bear
		cd = core.Cooldown{
			Timer:    druid.NewTimer(),
			Duration: time.Second * 6,
		}
		druid.FaerieFireAuras = druid.NewEnemyAuraArray(func(target *core.Unit) *core.Aura {
			if !topRank {
				return faerieFireRankAura(target, "Faerie Fire (Feral)", ffRank.feralID, ffRank.armor)
			}
			return core.FaerieFireFeralAura(target)
		})
	}
	flags |= core.SpellFlagAPL | core.SpellFlagResetAttackSwing

	druid.FaerieFire = druid.RegisterSpell(formMask, core.SpellConfig{
		SpellCode:   spellCode,
		ActionID:    actionID,
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskSpellDamage,
		Flags:       flags,

		ManaCost: manaCostOptions,
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: gcd,
			},
			IgnoreHaste: ignoreHaste,
			CD:          cd,
		},

		ThreatMultiplier: 1,
		FlatThreatBonus:  flatThreatBonus,
		DamageMultiplier: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcAndDealOutcome(sim, target, spell.OutcomeMagicHit)
			if result.Landed() {
				druid.FaerieFireAuras.Get(target).Activate(sim)
			}

			if druid.InForm(Humanoid | Moonkin) {
				druid.AutoAttacks.StopMeleeUntil(sim, sim.CurrentTime, false)
			}
		},

		RelatedAuras: []core.AuraArray{druid.FaerieFireAuras},
	})
}

func (druid *Druid) ShouldFaerieFire(sim *core.Simulation, target *core.Unit) bool {
	if druid.FaerieFire == nil {
		return false
	}

	if !druid.FaerieFire.IsReady(sim) {
		return false
	}

	debuff := druid.FaerieFireAuras.Get(target)
	return !debuff.IsActive() || debuff.RemainingDuration(sim) < time.Second*4
}
