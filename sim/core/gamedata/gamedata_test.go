package gamedata

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestBuildRegistryConcurrentReadersAndWriters(t *testing.T) {
	Current()
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			for j := 0; j < 30; j++ {
				Register(&Snapshot{Build: fmt.Sprintf("synthetic-concurrent-%d-%d", i, j)})
				_ = Builds()
				if _, err := ForBuild("synthetic-missing-build"); err == nil {
					t.Error("unknown build silently accepted")
				}
			}
		}(i)
	}
	workers.Wait()
}

func TestVariantReindexesAddedAndReplacedSpells(t *testing.T) {
	v := RegisterVariant("synthetic-reindex", func(s *Snapshot) {
		s.RawSpells["2147483000"] = &Spell{Name: "Synthetic added spell"}
		s.RawSpells["21553"] = &Spell{Name: "Synthetic replacement spell"}
	})
	if v.Spell(2147483000) == nil || v.Spell(21553).Name != "Synthetic replacement spell" {
		t.Fatal("variant indexes ignored added/replaced data")
	}
	if Current().Spell(2147483000) != nil || Current().Spell(21553).Name != "Mortal Strike" {
		t.Fatal("variant changed the original snapshot")
	}
}

func TestEmbeddedSnapshot(t *testing.T) {
	start := time.Now()
	s := Current()
	t.Logf("build %s, %d spells, decoded in %s", s.Build, len(s.RawSpells), time.Since(start))
	if s.Build == "" || len(s.RawSpells) < 10000 {
		t.Fatalf("unexpected snapshot: %q with %d spells", s.Build, len(s.RawSpells))
	}
	ms := s.Spell(21553) // Mortal Strike rank 4
	if ms == nil || ms.Name != "Mortal Strike" {
		t.Fatalf("Mortal Strike missing: %+v", ms)
	}
	if lo, hi := ms.Effect(1).Range(); lo != 160 || hi != 160 {
		t.Fatalf("Mortal Strike weapon bonus = %v..%v, want 160", lo, hi)
	}
	if c := ms.Cost("rage"); c == nil || c.Cost != 300 { // client stores rage x10
		t.Fatalf("Mortal Strike rage cost = %+v", c)
	}
	lethality := s.TalentForRecord("rogue.talent.lethality")
	for rank, want := range []float64{4, 8, 12, 16, 20} {
		if got, ok := lethality.Points(rank+1, 0); !ok || got != want {
			t.Fatalf("Lethality rank %d = %v, want %v", rank+1, got, want)
		}
	}
	if v, ok := s.Value("combatratings", "60", "Crit - Melee"); !ok || v != 14 {
		t.Fatalf("crit rating per 1%% = %v", v)
	}
}

func TestCloneIsIndependent(t *testing.T) {
	s := Current()
	c := s.Clone("test-build")
	c.Spell(21553).Effect(1).Base = 999
	if s.Spell(21553).Effect(1).Base == 999 {
		t.Fatal("clone shares effect data with the original")
	}
}
