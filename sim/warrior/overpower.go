package warrior

import (
	"time"

	"github.com/wowsims/classic/sim/core"
)

func (warrior *Warrior) registerOverpowerSpell(cdTimer *core.Timer) {
	rank, spellID := warrior.trainerRank("Overpower")
	if rank == 0 {
		return
	}
	// Effect 0 (normalized_weapon_damage, type 121): the flat bonus added to the weapon damage.
	bonusDamage := clientRankValue(&warrior.Character, spellID, 0, overpowerBonus[rank-1])
	// The dodge window is not a client spell value (the Overpower spells state no duration).
	window := time.Duration(codeValue(&warrior.Character, "warrior: Overpower", "dodge window_s", 5, "PROVISIONAL",
		"the window the dodge opens is not stored on the Overpower spells; the Classic 5 sec is used") * float64(time.Second))

	warrior.RegisterAura(core.Aura{
		Label:    "Overpower Trigger",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if result.DidDodge() {
				warrior.OverpowerAura.Activate(sim)
			}
		},
	})

	warrior.OverpowerAura = warrior.RegisterAura(core.Aura{
		Label:    "Overpower Aura",
		ActionID: core.ActionID{SpellID: spellID},
		Duration: window,
	})

	warrior.Overpower = warrior.RegisterSpell(BattleStance, core.SpellConfig{
		SpellCode:   SpellCode_WarriorOverpower,
		ActionID:    core.ActionID{SpellID: spellID},
		SpellSchool: core.SpellSchoolPhysical,
		DefenseType: core.DefenseTypeMelee,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagAPL | SpellFlagOffensive,

		RageCost: core.RageCostOptions{
			Cost:   5,
			Refund: 0.8,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
			CD: core.Cooldown{
				Timer:    cdTimer,
				Duration: time.Second * 5,
			},
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return warrior.OverpowerAura.IsActive()
		},

		// Improved Overpower: client effect 0 is the crit chance (25/50%).
		BonusCritRating: core.CritRatingPerCritChance * clientTalent(&warrior.Character, "warrior.talent.improved-overpower", 0, 1, 25*float64(warrior.Talents.ImprovedOverpower)),

		CritDamageBonus: warrior.impale(),

		DamageMultiplier: 1,
		ThreatMultiplier: 0.75,
		BonusCoefficient: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := bonusDamage + spell.Unit.MHNormalizedWeaponDamage(sim, spell.MeleeAttackPower(target))
			result := spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialNoBlockDodgeParry)

			warrior.OverpowerAura.Deactivate(sim)
			if !result.Landed() {
				spell.IssueRefund(sim)
			}
		},
	})
}
