package optimizer

import (
	"testing"
	"time"

	"github.com/wowsims/classic/sim"
)

func TestTiming(t *testing.T) {
	sim.RegisterAll()
	d, err := LoadData("")
	if err != nil {
		t.Fatal(err)
	}
	for _, sc := range []struct{ k string; l int32 }{{"rogue", 20}, {"warrior", 60}, {"mage", 60}} {
		p, err := NewProblem(d, sc.k, sc.l)
		if err != nil {
			t.Fatal(err)
		}
		c := p.NakedConfig()
		if pr := p.Presets(); len(pr) > 0 {
			c.Talents = p.PresetTalents(pr[0])
		}
		e := NewEvaluator(p, 1, 1)
		t0 := time.Now()
		if err := e.Ensure([]Config{c}, 100); err != nil {
			t.Fatal(err)
		}
		t1 := time.Since(t0)
		t0 = time.Now()
		c2 := c.Clone(); c2.Race = p.Races()[len(p.Races())-1]
		e.Ensure([]Config{c2}, 1000)
		t.Logf("%s %d: 100 it %v, 1000 it %v, dps %.1f talents %d", sc.k, sc.l, t1, time.Since(t0), e.Mean(&c, 100).Mean, spent(c.Talents))
	}
}
