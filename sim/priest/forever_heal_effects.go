package priest

import (
	"github.com/wowsims/classic/sim/core"
	"time"
)

// Apply Renewed Hope to each actual recipient, including Binding Heal and the
// later Penance bolts. Per-bolt weakening is an explicit provisional rule.
func (p *Priest) foreverRenewedHopeHeal(sim *core.Simulation, target *core.Unit, spell *core.Spell, amount float64) {
	weak := p.WeakenedSouls.Get(target)
	active := weak.IsActive()
	crit := 0.0
	if active {
		crit = p.ForeverValue("priest.talent.renewed-hope", 0, 0) * core.SpellCritRatingPerCritChance
	}
	spell.BonusCritRating += crit
	spell.CalcAndDealHealing(sim, target, amount, spell.OutcomeHealingCrit)
	spell.BonusCritRating -= crit
	if active && p.ForeverRank("priest.talent.renewed-hope") > 0 {
		weak.UpdateExpires(sim, max(sim.CurrentTime, weak.ExpiresAt()-time.Duration(p.ForeverValue("priest.talent.renewed-hope", 1, 0)*float64(time.Second))))
	}
}
