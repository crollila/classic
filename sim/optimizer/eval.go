package optimizer

import (
	"fmt"
	"math"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/wowsims/classic/sim/core"
)

// Evaluator runs the engine for configurations and caches every per-iteration DPS value.
//
// The engine reseeds its random source with seed+i before iteration i, so iteration i of two
// configurations sees the same random stream (common random numbers). The evaluator keeps the
// values per configuration in iteration order: asking for more iterations only simulates the
// missing tail (RandomSeed = seed+have), comparisons use paired differences over the common
// prefix, and a configuration is never simulated twice for the same iteration.
type Evaluator struct {
	P        *Problem
	Seed     int64
	Parallel int

	mu      sync.Mutex
	entries map[string]*entry

	sims, iterations, hits, invalid atomic.Int64
	OnSim                           func(sims int64) // progress hook (called from worker goroutines)
}

type entry struct {
	cfg  Config
	vals []float64
	err  error
}

// NewEvaluator creates an evaluator with its own cache.
func NewEvaluator(p *Problem, seed int64, parallel int) *Evaluator {
	if parallel <= 0 {
		parallel = runtime.NumCPU()
	}
	return &Evaluator{P: p, Seed: seed, Parallel: parallel, entries: map[string]*entry{}}
}

func (e *Evaluator) entryFor(c *Config) *entry {
	k := c.Key()
	e.mu.Lock()
	defer e.mu.Unlock()
	en, ok := e.entries[k]
	if !ok {
		en = &entry{cfg: c.Clone()}
		e.entries[k] = en
	}
	return en
}

type job struct {
	en           *entry
	start, count int32
	out          []float64
	err          error
}

// Ensure simulates each configuration up to n iterations. Configurations that fail validation
// are never simulated; their error is returned.
func (e *Evaluator) Ensure(cfgs []Config, n int32) error {
	seen := map[*entry]bool{}
	pending := []*entry{}
	for i := range cfgs {
		en := e.entryFor(&cfgs[i])
		if seen[en] {
			continue
		}
		seen[en] = true
		if en.err != nil {
			return en.err
		}
		if err := e.P.Validate(&en.cfg); err != nil {
			e.invalid.Add(1)
			en.err = fmt.Errorf("invalid configuration reached the evaluator: %w", err)
			return en.err
		}
		if int32(len(en.vals)) >= n {
			e.hits.Add(1)
			continue
		}
		pending = append(pending, en)
	}
	if len(pending) == 0 {
		return nil
	}
	// Split the missing iterations so every worker has something to do.
	chunksPer := max(1, (e.Parallel+len(pending)-1)/len(pending))
	jobs := []*job{}
	for _, en := range pending {
		have := int32(len(en.vals))
		need := n - have
		chunk := max(int32(100), (need+int32(chunksPer)-1)/int32(chunksPer))
		for s := have; s < n; s += chunk {
			jobs = append(jobs, &job{en: en, start: s, count: min(chunk, n-s)})
		}
	}
	work := make(chan *job)
	var wg sync.WaitGroup
	for w := 0; w < min(e.Parallel, len(jobs)); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range work {
				j.out, j.err = e.run(&j.en.cfg, j.start, j.count)
			}
		}()
	}
	for _, j := range jobs {
		work <- j
	}
	close(work)
	wg.Wait()
	sort.SliceStable(jobs, func(a, b int) bool { return jobs[a].start < jobs[b].start })
	for _, j := range jobs {
		if j.err != nil {
			j.en.err = j.err
			return j.err
		}
		if int32(len(j.en.vals)) != j.start {
			return fmt.Errorf("evaluator: iteration gap (have %d, chunk starts at %d)", len(j.en.vals), j.start)
		}
		j.en.vals = append(j.en.vals, j.out...)
	}
	return nil
}

func (e *Evaluator) run(c *Config, start, count int32) ([]float64, error) {
	req, err := e.P.Request(c, count, e.Seed+int64(start))
	if err != nil {
		return nil, err
	}
	res := core.RunRaidSim(req)
	if res.Error != nil {
		return nil, fmt.Errorf("sim error: %s", res.Error.Message)
	}
	vals := res.RaidMetrics.GetDps().GetAllValues()
	if int32(len(vals)) != count {
		return nil, fmt.Errorf("sim returned %d per-iteration values for %d iterations", len(vals), count)
	}
	e.iterations.Add(int64(count))
	s := e.sims.Add(1)
	if e.OnSim != nil {
		e.OnSim(s)
	}
	return vals, nil
}

// Estimate is a mean over n iterations with its standard error.
type Estimate struct {
	Mean, SE float64
	N        int32
}

// Mean is the configuration's mean DPS over its first n iterations (which must be simulated).
func (e *Evaluator) Mean(c *Config, n int32) Estimate {
	v := e.entryFor(c).vals
	n = min(n, int32(len(v)))
	m, sd := meanSD(v[:n])
	return Estimate{Mean: m, SE: sd / math.Sqrt(float64(max(1, n))), N: n}
}

// Diff is the paired difference a - b over their common first n iterations.
func (e *Evaluator) Diff(a, b *Config, n int32) Estimate {
	va, vb := e.entryFor(a).vals, e.entryFor(b).vals
	n = min(n, int32(len(va)), int32(len(vb)))
	d := make([]float64, n)
	for i := range d {
		d[i] = va[i] - vb[i]
	}
	m, sd := meanSD(d)
	return Estimate{Mean: m, SE: sd / math.Sqrt(float64(max(1, n))), N: n}
}

func meanSD(v []float64) (float64, float64) {
	if len(v) == 0 {
		return 0, 0
	}
	s := 0.0
	for _, x := range v {
		s += x
	}
	m := s / float64(len(v))
	if len(v) < 2 {
		return m, 0
	}
	ss := 0.0
	for _, x := range v {
		ss += (x - m) * (x - m)
	}
	return m, math.Sqrt(ss / float64(len(v)-1))
}

// Counters reports the evaluator's work.
func (e *Evaluator) Counters() (sims, iterations, cacheHits, invalid int64, configs int) {
	e.mu.Lock()
	configs = len(e.entries)
	e.mu.Unlock()
	return e.sims.Load(), e.iterations.Load(), e.hits.Load(), e.invalid.Load(), configs
}
