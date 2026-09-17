package core_test

import (
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
	"testing"
	"time"
)

func TestForeverMixologyMagnitudeDurationAndReset(t *testing.T) {
	for _, strict := range []bool{false, true} {
		t.Run(map[bool]string{true: "STRICT", false: "BEST_GUESS"}[strict], func(t *testing.T) {
			sim, c := reviewSim(t, func(p *proto.Player) {
				p.Profession1 = proto.Profession_Alchemy
				p.Consumes = &proto.Consumes{Flask: proto.Flask_FlaskOfSupremePower}
				p.Forever.Mechanics = []string{"profession.alchemy.mixology"}
				p.Forever.Parameters = map[string]float64{"scenario.flask_seconds_remaining": 10}
				if strict {
					p.Forever.Mode = proto.ForeverMode_STRICT
				}
			})
			want := 187.5
			duration := 20 * time.Second
			if strict {
				want = 150
				duration = 10 * time.Second
			}
			if c.GetStat(stats.SpellPower) != want {
				t.Fatalf("power %v want %v", c.GetStat(stats.SpellPower), want)
			}
			aura := c.GetAura("Forever flask duration")
			if aura == nil || aura.RemainingDuration(sim) != duration {
				t.Fatal("wrong duration", aura)
			}
			reviewAdvance(t, sim, 11*time.Second)
			if strict && c.GetStat(stats.SpellPower) != 0 {
				t.Fatal("STRICT flask failed to expire")
			}
			if !strict && c.GetStat(stats.SpellPower) != want {
				t.Fatal("predicted longer flask expired early")
			}
			reviewAdvance(t, sim, 21*time.Second)
			if c.GetStat(stats.SpellPower) != 0 {
				t.Fatal("flask stats persisted after expiry")
			}
			sim.Cleanup()
			sim.Reset()
			if c.GetStat(stats.SpellPower) != want {
				t.Fatal("reset lost or doubled consume stats")
			}
		})
	}
}

func TestForeverCampsiteLegacyExpiryAndStrictSetup(t *testing.T) {
	for _, strict := range []bool{false, true} {
		t.Run(map[bool]string{true: "STRICT", false: "BEST_GUESS"}[strict], func(t *testing.T) {
			sim, c := reviewSim(t, func(p *proto.Player) {
				p.Forever.Mechanics = []string{"buffs.first-aid-kit", "legacy.permanence"}
				p.Forever.MechanicRanks = map[string]int32{"legacy.permanence": 2}
				p.Forever.Parameters = map[string]float64{"scenario.campsite_seconds_remaining": 4}
				if strict {
					p.Forever.Mode = proto.ForeverMode_STRICT
				}
			})
			aura := c.GetAura("buffs.first-aid-kit")
			duration := 8 * time.Second
			if strict {
				duration = 6 * time.Second
			}
			if aura.RemainingDuration(sim) != duration {
				t.Fatal("Legacy duration", aura.RemainingDuration(sim))
			}
			before := c.GetStat(stats.Stamina)
			reviewAdvance(t, sim, 9*time.Second)
			if before-c.GetStat(stats.Stamina) != 34 {
				t.Fatal("campsite expiration did not remove exactly34 Stamina")
			}
			setup := false
			for _, s := range c.Spellbook {
				if s.ActionID == c.ForeverAction("buffs.camping") {
					setup = true
				}
			}
			if setup == strict {
				t.Fatal("predicted setup mode isolation")
			}
		})
	}
}
