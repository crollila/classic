package core

import (
	"math"
	"strconv"

	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
)

const foreverEnemyHealthAura = "Forever finite enemy health"

// This is an explicit encounter scenario, not a predicted boss health value.
// Zero keeps the usual immortal duration target. An existing health encounter
// supplies its own target health; parameters optionally override that health.
func (env *Environment) initializeForeverEnemyHealth(encounter *proto.Encounter) {
	hasForever := false
	for _, party := range env.Raid.Parties {
		for _, player := range party.Players {
			hasForever = hasForever || player.GetCharacter().Forever != nil
		}
	}
	if !hasForever {
		return
	}
	for _, target := range env.Encounter.Targets {
		amount := 0.0
		if encounter.GetUseHealth() {
			amount = target.GetStat(stats.Health)
		}
		configuredHealth := 0.0
		for _, party := range env.Raid.Parties {
			for _, player := range party.Players {
				c := player.GetCharacter()
				if c.Forever == nil {
					continue
				}
				configured := c.ForeverParameter("scenario.enemy_health", 0)
				configured = c.ForeverParameter("scenario.enemy_health."+strconv.Itoa(int(target.Index)), configured)
				if configured > 0 && !math.IsInf(configured, 0) && !math.IsNaN(configured) {
					configuredHealth = max(configuredHealth, min(configured, 1e12))
				}
			}
		}
		if configuredHealth > 0 {
			amount = configuredHealth
		}
		if amount <= 0 {
			continue
		}
		target.AddStat(stats.Health, amount-target.GetStat(stats.Health))
		target.EnableHealthBar()
		MakePermanent(target.RegisterAura(Aura{Label: foreverEnemyHealthAura}))
	}
}

func (u *Unit) ForeverEnemyDead() bool {
	return u != nil && u.Type == EnemyUnit && u.HasHealthBar() && u.HasAura(foreverEnemyHealthAura) && u.CurrentHealth() <= 0
}

// Returns false for a corpse or a dead caster, suppressing callbacks as well as
// damage. Removing health before damage callbacks exposes exactly one kill
// event through the ordinary dealt/taken callbacks, including periodic damage.
func (spell *Spell) applyForeverEnemyHealth(sim *Simulation, result *SpellResult) bool {
	target := result.Target
	if spell.Unit.ForeverEnemyDead() || target.ForeverEnemyDead() {
		return false
	}
	if target.Type != EnemyUnit || !target.HasHealthBar() || !target.HasAura(foreverEnemyHealthAura) || result.Damage <= 0 {
		return true
	}
	result.Damage = min(result.Damage, target.CurrentHealth())
	target.RemoveHealth(sim, result.Damage)
	if target.CurrentHealth() > 0 {
		return true
	}
	target.Metrics.Died = true
	target.AutoAttacks.CancelAutoSwing(sim)
	target.foreverInterruptCast(sim)
	var next *Unit
	for _, candidate := range sim.Encounter.TargetUnits {
		if !candidate.ForeverEnemyDead() {
			next = candidate
			break
		}
	}
	if next != nil {
		for _, unit := range sim.Raid.AllUnits {
			if unit.CurrentTarget == target {
				unit.CurrentTarget = next
			}
		}
	} else if sim.Encounter.EndFightAtHealth > 0 {
		// The original loop ends at strictly greater damage. Clamped finite
		// health damage reaches the exact total; lower only its stop threshold.
		sim.endOfCombatDamage = math.Nextafter(sim.Encounter.DamageTaken+result.Damage, math.Inf(-1))
	}
	return true
}
