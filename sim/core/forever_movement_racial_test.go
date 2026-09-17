package core_test

import (
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/druid/balance"
	"math"
	"testing"
)

func TestForeverMovementRacialsInitializeAndReset(t *testing.T) {
	balance.RegisterBalanceDruid()
	for _, tc := range []struct {
		race  proto.Race
		id    string
		speed float64
	}{
		{proto.Race_RaceNightElf, "racials.night-elf.quickness", 7 * 1.02},
		{proto.Race_RaceSkyborneWindshaper, "racials.skyborne-windshaper.skysight", 7 * 1.1},
	} {
		t.Run(tc.id, func(t *testing.T) {
			sim, c := reviewSim(t, func(p *proto.Player) {
				p.Class = proto.Class_ClassDruid
				p.Race = tc.race
				p.Spec = &proto.Player_BalanceDruid{BalanceDruid: &proto.BalanceDruid{Options: &proto.BalanceDruid_Options{}}}
				p.Forever.Mechanics = []string{tc.id}
			})
			for iteration := 0; iteration < 3; iteration++ {
				if math.Abs(c.MovementHandler.MoveSpeed-tc.speed) > 1e-9 {
					t.Fatalf("movement speed %v want %v", c.MovementHandler.MoveSpeed, tc.speed)
				}
				sim.Cleanup()
				sim.Reset()
			}
		})
	}
}
