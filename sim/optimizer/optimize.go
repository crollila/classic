package optimizer

// Setup optimizer: "what is the highest-DPS <spec> setup at level L for this Forever build?"
//
// Search space (every point is a legal character, checked by Problem.Validate before any sim):
// gear per slot (items the class can wear at the level), enchants per slot, the talent build
// (the level's point budget and the tree rules), race and the spec's rotation.
//
// Algorithm (deterministic for a seed):
//   - Evaluation: the engine directly, in parallel goroutines, with common random numbers. The
//     evaluator caches per-iteration DPS by configuration, so more iterations only sim the tail.
//   - Racing (successive halving): a candidate set is simmed at N, 3N, 9N iterations; after each
//     level candidates whose paired DPS difference to the leader exceeds 2.5 standard errors are
//     dropped and the field is cut to the best few. The incumbent is replaced only when the winner
//     beats it by more than 2 paired standard errors and 0.05%.
//   - Stage "talents": seeds are the empty build and every popular build truncated to the level;
//     then greedy point allocation while points are unspent, then local search with one-point
//     transfers (the least costly removals are found by sim, then every legal destination for
//     them is raced) and the popular builds as jump moves.
//   - Stage "gear": stat weights come from sims (paired finite differences on the current
//     setup, including weapon DPS). Per slot, items are grouped into equivalence classes (same
//     stats, weapon, set and no special effect), ranked by EP and pruned to the top K, plus the
//     best per weapon type and hand, plus the best items with procs/on-use/set/skill bonuses
//     that EP cannot value. An EP-greedy full set is raced as a jump move, then coordinate
//     descent races each slot's candidates with the rest fixed (main hand moves also carry the
//     best off hand), until a pass changes nothing.
//   - Stage "enchants": per slot, the legal enchants pruned by EP plus enchants with effects.
//   - Stages "rotation" and "race": every option is raced.
//   - Rounds repeat all stages until a round changes nothing (or the budget runs out). The final
//     setup and the baselines (naked, naked + popular build) are re-simmed with a fresh seed.

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/gamedata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
)

// Options tune the search. Zero values take the defaults.
type Options struct {
	Seed            int64
	Parallel        int
	BaseIterations  int32 // first racing level (default 100); then x3, x9
	FinalIterations int32 // final report (default 5000)
	WeightIters     int32 // stat-weight sims (default 1500)
	TopK            int   // EP-ranked items kept per slot (default 5)
	MaxRounds       int   // default 4
	Budget          time.Duration

	SkipGear, SkipEnchants, SkipTalents, SkipRace, SkipRotation bool

	Start    *Config // starting configuration (default: naked, best talent seed)
	Progress func(stage string, sims int64, best float64)
}

func (o *Options) defaults() {
	if o.Seed == 0 {
		o.Seed = 1
	}
	if o.Parallel <= 0 {
		o.Parallel = runtime.NumCPU()
	}
	if o.BaseIterations <= 0 {
		o.BaseIterations = 100
	}
	if o.FinalIterations <= 0 {
		o.FinalIterations = 5000
	}
	if o.WeightIters <= 0 {
		o.WeightIters = 1500
	}
	if o.TopK <= 0 {
		o.TopK = 5
	}
	if o.MaxRounds <= 0 {
		o.MaxRounds = 4
	}
}

// Step is one accepted change.
type Step struct {
	Round      int     `json:"round"`
	Stage      string  `json:"stage"`
	Change     string  `json:"change"`
	DPSBefore  float64 `json:"dps_before"`
	DPSAfter   float64 `json:"dps_after"`
	GainSE     float64 `json:"gain_stderr"`
	Iterations int32   `json:"decided_at_iterations"`
	Candidates int     `json:"candidates"`
}

type optimizer struct {
	p     *Problem
	o     Options
	ev    *Evaluator
	start time.Time
	round int
	steps []Step
	races int
	cands int

	weights     weights
	stageSims   map[string]int64
	stageTime   map[string]time.Duration
	levels      []int32
	talentSeeds []Config
}

const (
	dropZ   = 2.5
	acceptZ = 2.0
	minGain = 0.0005
)

func (op *optimizer) overBudget() bool {
	return op.o.Budget > 0 && time.Since(op.start) > op.o.Budget
}

// race returns the best of the incumbent and the candidates, and whether it changed.
// fill=true accepts the best candidate even when it does not beat the incumbent (used to spend
// unspent talent points, where the incumbent is not a real alternative).
func (op *optimizer) race(stage string, inc Config, cands []Config, describe func(Config) string, fill bool) (Config, bool, error) {
	keys := map[string]bool{inc.Key(): true}
	field := []Config{inc}
	for _, c := range cands {
		k := c.Key()
		if keys[k] {
			continue
		}
		if err := op.p.Validate(&c); err != nil {
			return inc, false, fmt.Errorf("%s: generated an invalid candidate: %w", stage, err)
		}
		keys[k] = true
		field = append(field, c)
	}
	if len(field) == 1 {
		return inc, false, nil
	}
	op.races++
	op.cands += len(field) - 1
	caps := []int{8, 3, 2}
	var n int32
	for li, lvl := range op.levels {
		n = lvl
		if err := op.ev.Ensure(field, n); err != nil {
			return inc, false, err
		}
		lead := 0
		start := 0
		if fill {
			start = 1
			lead = 1
		}
		for i := start; i < len(field); i++ {
			if op.ev.Mean(&field[i], n).Mean > op.ev.Mean(&field[lead], n).Mean {
				lead = i
			}
		}
		type scored struct {
			c    Config
			mean float64
			keep bool
		}
		rest := []scored{}
		for i := 1; i < len(field); i++ {
			d := op.ev.Diff(&field[lead], &field[i], n)
			keep := i == lead || d.Mean <= dropZ*d.SE
			rest = append(rest, scored{field[i], op.ev.Mean(&field[i], n).Mean, keep})
		}
		sort.SliceStable(rest, func(a, b int) bool { return rest[a].mean > rest[b].mean })
		next := []Config{inc}
		for _, s := range rest {
			if s.keep && len(next)-1 < caps[min(li, len(caps)-1)] {
				next = append(next, s.c)
			}
		}
		field = next
		if len(field) == 1 {
			return inc, false, nil
		}
		if li == len(op.levels)-1 || op.overBudget() {
			break
		}
	}
	best := 1
	for i := 2; i < len(field); i++ {
		if op.ev.Mean(&field[i], n).Mean > op.ev.Mean(&field[best], n).Mean {
			best = i
		}
	}
	w := field[best]
	d := op.ev.Diff(&w, &inc, n)
	base := op.ev.Mean(&inc, n)
	if !fill && (d.Mean <= acceptZ*d.SE || d.Mean <= minGain*math.Abs(base.Mean)) {
		return inc, false, nil
	}
	op.steps = append(op.steps, Step{Round: op.round, Stage: stage, Change: describe(w), DPSBefore: round2(base.Mean),
		DPSAfter: round2(op.ev.Mean(&w, n).Mean), GainSE: round2(d.SE), Iterations: n, Candidates: len(cands)})
	if op.o.Progress != nil {
		op.o.Progress(stage+": "+describe(w), op.ev.sims.Load(), op.ev.Mean(&w, n).Mean)
	}
	return w, true, nil
}

// ---------------------------------------------------------------- weights

type weights struct {
	Stats                  stats.Stats
	MH, OH, Ranged         float64
	reference              string
	referenceDPS, iterDone float64
}

type weightProbe struct {
	stat   int // stats index, or -1 MH, -2 OH, -3 ranged
	delta  float64
	name   string
	shares []int // other stats that take this weight
}

func (op *optimizer) probes() []weightProbe {
	s := op.p.Spec
	ps := []weightProbe{}
	add := func(stat stats.Stat, delta float64, shares ...stats.Stat) {
		sh := []int{}
		for _, x := range shares {
			sh = append(sh, int(x))
		}
		ps = append(ps, weightProbe{stat: int(stat), delta: delta, name: stat.StatName(), shares: sh})
	}
	switch s.Role {
	case "melee", "ranged":
		add(stats.Strength, 20)
		add(stats.Agility, 20)
		add(stats.AttackPower, 40, stats.FeralAttackPower)
		add(stats.MeleeHit, 1)
		add(stats.MeleeCrit, 1)
		ps = append(ps, weightProbe{stat: -1, delta: 5, name: "MainHandDps"})
		if s.DualWield(op.p.Level) {
			ps = append(ps, weightProbe{stat: -2, delta: 5, name: "OffHandDps"})
		}
		if s.Role == "ranged" {
			add(stats.RangedAttackPower, 40)
			add(stats.Intellect, 20)
			ps = append(ps, weightProbe{stat: -3, delta: 5, name: "RangedDps"})
		}
		if s.Key == "retribution_paladin" || s.Key == "enhancement_shaman" {
			add(stats.SpellPower, 20, stats.SpellDamage)
			add(stats.Intellect, 20)
		}
	default:
		add(stats.Intellect, 20)
		add(stats.Spirit, 20)
		add(stats.SpellPower, 20, stats.SpellDamage)
		add(stats.SpellHit, 1)
		add(stats.SpellCrit, 1)
		add(stats.MP5, 10)
		schools := map[string][]stats.Stat{
			"mage": {stats.FirePower, stats.FrostPower, stats.ArcanePower}, "warlock": {stats.ShadowPower, stats.FirePower},
			"shadow_priest": {stats.ShadowPower}, "balance_druid": {stats.NaturePower, stats.ArcanePower},
			"elemental_shaman": {stats.NaturePower, stats.FirePower, stats.FrostPower},
		}
		for _, sc := range schools[s.Key] {
			add(sc, 20)
		}
	}
	return ps
}

// computeWeights measures DPS per point of each stat on the current setup: the setup with a
// bonus of each stat is simmed with the same random numbers as the setup itself.
func (op *optimizer) computeWeights(c Config) error {
	t0 := time.Now()
	s0 := op.ev.sims.Load()
	probes := op.probes()
	n := op.o.WeightIters
	// Bonus sims bypass the cache key space of real setups: they are run directly.
	type res struct {
		vals []float64
		err  error
	}
	out := make([]res, len(probes)+1)
	jobs := make(chan int)
	done := make(chan struct{})
	workers := min(op.o.Parallel, len(out))
	for w := 0; w < workers; w++ {
		go func() {
			for i := range jobs {
				var bonus *proto.UnitStats
				if i > 0 {
					pr := probes[i-1]
					bonus = &proto.UnitStats{Stats: make([]float64, stats.Len), PseudoStats: make([]float64, stats.PseudoStatsLen)}
					switch pr.stat {
					case -1:
						bonus.PseudoStats[proto.PseudoStat_PseudoStatMainHandDps] = pr.delta
					case -2:
						bonus.PseudoStats[proto.PseudoStat_PseudoStatOffHandDps] = pr.delta
					case -3:
						bonus.PseudoStats[proto.PseudoStat_PseudoStatRangedDps] = pr.delta
					default:
						bonus.Stats[pr.stat] = pr.delta
					}
				}
				vals, err := op.bonusSim(c, bonus, n)
				out[i] = res{vals, err}
			}
			done <- struct{}{}
		}()
	}
	for i := range out {
		jobs <- i
	}
	close(jobs)
	for w := 0; w < workers; w++ {
		<-done
	}
	for _, r := range out {
		if r.err != nil {
			return r.err
		}
	}
	w := weights{}
	base := out[0].vals
	for i, pr := range probes {
		v := out[i+1].vals
		d := make([]float64, len(v))
		for j := range v {
			d[j] = v[j] - base[j]
		}
		m, _ := meanSD(d)
		wt := math.Max(0, m/pr.delta)
		switch pr.stat {
		case -1:
			w.MH = wt
		case -2:
			w.OH = wt
		case -3:
			w.Ranged = wt
		default:
			w.Stats[pr.stat] = wt
			for _, sh := range pr.shares {
				w.Stats[sh] = wt
			}
		}
	}
	w.referenceDPS, _ = meanSD(base)
	op.weights = w
	op.stageSims["weights"] += op.ev.sims.Load() - s0 + int64(len(out))
	op.stageTime["weights"] += time.Since(t0)
	return nil
}

func (op *optimizer) bonusSim(c Config, bonus *proto.UnitStats, n int32) ([]float64, error) {
	req, err := op.p.Request(&c, n, op.o.Seed+7919)
	if err != nil {
		return nil, err
	}
	req.Raid.Parties[0].Players[0].BonusStats = bonus
	res := core.RunRaidSim(req)
	if res.Error != nil {
		return nil, fmt.Errorf("stat weight sim: %s", res.Error.Message)
	}
	op.ev.iterations.Add(int64(n))
	return res.RaidMetrics.GetDps().GetAllValues(), nil
}

func weaponDPS(it *proto.UIItem) float64 {
	if it == nil || it.WeaponSpeed <= 0 {
		return 0
	}
	return (it.WeaponDamageMin + it.WeaponDamageMax) / 2 / it.WeaponSpeed
}

func (op *optimizer) itemEP(it *proto.UIItem, slot int) float64 {
	if it == nil {
		return 0
	}
	ep := 0.0
	for i, v := range it.Stats {
		if i < len(op.weights.Stats) {
			ep += v * op.weights.Stats[i]
		}
	}
	switch {
	case slot == slotMainHand:
		ep += weaponDPS(it) * op.weights.MH
	case slot == slotOffHand:
		ep += weaponDPS(it) * op.weights.OH
	case slot == slotRanged:
		ep += weaponDPS(it) * op.weights.Ranged
	}
	return ep
}

func (op *optimizer) enchantEP(e *proto.UIEnchant) float64 {
	ep := 0.0
	for i, v := range e.Stats {
		if i < len(op.weights.Stats) {
			ep += v * op.weights.Stats[i]
		}
	}
	return ep
}

// ---------------------------------------------------------------- gear

func special(it *proto.UIItem) bool {
	if core.HasItemEffect(it.Id) || core.HasWeaponEffect(it.Id) || it.SetId != 0 || it.BonusPhysicalDamage != 0 {
		return true
	}
	for _, s := range it.WeaponSkills {
		if s != 0 {
			return true
		}
	}
	return false
}

// signature is the equivalence class of an item: two items with the same signature are the
// same to the engine (no special effect, identical stats and weapon).
func signature(it *proto.UIItem) string {
	return fmt.Sprintf("%d/%d/%d/%d/%d/%.2f/%.2f/%.2f/%v", it.Type, it.ArmorType, it.WeaponType, it.HandType, it.RangedWeaponType,
		it.WeaponDamageMin, it.WeaponDamageMax, it.WeaponSpeed, it.Stats)
}

type slotCandidates struct {
	items  []*proto.UIItem
	total  int // legal items before pruning
	groups int // equivalence classes
}

// candidates prunes the legal items of a slot (see the header comment).
func (op *optimizer) candidates(slot int, race proto.Race) slotCandidates {
	legal := []*proto.UIItem{}
	for _, it := range op.p.Data.Items {
		if op.p.itemFits(it, slot, race) {
			legal = append(legal, it)
		}
	}
	// Equivalence classes: keep the lowest required level, then the lowest id.
	rep := map[string]*proto.UIItem{}
	reps := []*proto.UIItem{}
	for _, it := range legal {
		if special(it) {
			reps = append(reps, it)
			continue
		}
		sig := signature(it)
		if r, ok := rep[sig]; !ok || it.RequiredLevel < r.RequiredLevel || (it.RequiredLevel == r.RequiredLevel && it.Id < r.Id) {
			rep[sig] = it
		}
	}
	for _, it := range rep {
		reps = append(reps, it)
	}
	sort.Slice(reps, func(a, b int) bool {
		ea, eb := op.itemEP(reps[a], slot), op.itemEP(reps[b], slot)
		if ea != eb {
			return ea > eb
		}
		return reps[a].Id < reps[b].Id
	})
	keep := map[int32]bool{}
	out := []*proto.UIItem{}
	take := func(it *proto.UIItem) {
		if !keep[it.Id] {
			keep[it.Id] = true
			out = append(out, it)
		}
	}
	plain, specials := 0, 0
	perKind := map[string]int{}
	for _, it := range reps {
		kind := ""
		if it.Type == proto.ItemType_ItemTypeWeapon {
			kind = fmt.Sprintf("w%d/h%d", it.WeaponType, it.HandType)
		}
		switch {
		case special(it) && specials < 3:
			specials++
			take(it)
		case !special(it) && plain < op.o.TopK:
			plain++
			take(it)
		case kind != "" && perKind[kind] < 1:
			take(it)
		}
		if kind != "" {
			perKind[kind]++
		}
	}
	return slotCandidates{items: out, total: len(legal), groups: len(reps)}
}

// withItem puts an item in a slot and repairs the dependent slots the way the app does: a
// two-hander empties the off hand, an enchant that no longer fits is removed. It returns false
// when the result would be illegal (off hand under a two-hander, unique item twice).
func (op *optimizer) withItem(c Config, slot int, id int32) (Config, bool) {
	n := c.Clone()
	n.Gear[slot] = id
	it := op.p.Data.ItemByID[id]
	if id != 0 && !op.p.itemFits(it, slot, n.Race) {
		return c, false
	}
	if e := n.Enchants[slot]; e != 0 && (id == 0 || !op.p.enchantFits(op.p.enchantByID[e], it, slot)) {
		n.Enchants[slot] = 0
	}
	if slot == slotMainHand && isTwoHander(it) {
		n.Gear[slotOffHand], n.Enchants[slotOffHand] = 0, 0
	}
	if slot == slotOffHand && id != 0 && isTwoHander(op.p.Data.ItemByID[n.Gear[slotMainHand]]) {
		return c, false
	}
	if twin := twinSlot(slot); twin >= 0 && id != 0 && it.Unique && n.Gear[twin] == id {
		return c, false
	}
	return n, true
}

func (op *optimizer) describeGear(before Config) func(Config) string {
	return func(c Config) string {
		parts := []string{}
		for s := 0; s < numSlots; s++ {
			if c.Gear[s] != before.Gear[s] {
				parts = append(parts, fmt.Sprintf("%s %s -> %s", SlotNames[s], op.itemName(before.Gear[s]), op.itemName(c.Gear[s])))
			}
			if c.Enchants[s] != before.Enchants[s] && c.Gear[s] == before.Gear[s] {
				parts = append(parts, fmt.Sprintf("%s enchant %s -> %s", SlotNames[s], op.enchantName(before.Enchants[s]), op.enchantName(c.Enchants[s])))
			}
		}
		return strings.Join(parts, "; ")
	}
}

func (op *optimizer) itemName(id int32) string {
	if id == 0 {
		return "(empty)"
	}
	if it := op.p.Data.ItemByID[id]; it != nil {
		return it.Name
	}
	return fmt.Sprint(id)
}

func (op *optimizer) enchantName(id int32) string {
	if id == 0 {
		return "(none)"
	}
	if e := op.p.enchantByID[id]; e != nil {
		return e.Name
	}
	return fmt.Sprint(id)
}

// epGreedy is the best-EP full set for the current race (a jump move for the gear stage).
func (op *optimizer) epGreedy(c Config, cands [numSlots]slotCandidates) Config {
	n := c.Clone()
	for s := 0; s < numSlots; s++ {
		if s == slotOffHand || s == slotMainHand {
			continue
		}
		for _, it := range cands[s].items {
			if next, ok := op.withItem(n, s, it.Id); ok {
				n = next
				break
			}
		}
	}
	// Main hand + off hand: best two-hander vs best one-hander with the best off hand.
	best2H, best1H, bestOH := (*proto.UIItem)(nil), (*proto.UIItem)(nil), (*proto.UIItem)(nil)
	for _, it := range cands[slotMainHand].items {
		if isTwoHander(it) {
			if best2H == nil || op.itemEP(it, slotMainHand) > op.itemEP(best2H, slotMainHand) {
				best2H = it
			}
		} else if best1H == nil || op.itemEP(it, slotMainHand) > op.itemEP(best1H, slotMainHand) {
			best1H = it
		}
	}
	for _, it := range cands[slotOffHand].items {
		if bestOH == nil || op.itemEP(it, slotOffHand) > op.itemEP(bestOH, slotOffHand) {
			if !(it.Unique && best1H != nil && it.Id == best1H.Id) {
				bestOH = it
			}
		}
	}
	oneHand := op.itemEP(best1H, slotMainHand) + op.itemEP(bestOH, slotOffHand)
	if best2H != nil && (best1H == nil || op.itemEP(best2H, slotMainHand) > oneHand) {
		n, _ = op.withItem(n, slotMainHand, best2H.Id)
	} else if best1H != nil {
		n, _ = op.withItem(n, slotMainHand, best1H.Id)
		if bestOH != nil {
			if next, ok := op.withItem(n, slotOffHand, bestOH.Id); ok {
				n = next
			}
		}
	}
	return n
}

var gearOrder = []int{slotMainHand, slotOffHand, slotRanged, 4, 8, 0, 2, 6, 9, 7, 5, 3, 1, slotFinger1, slotFinger2, slotTrinket1, slotTrinket2}

func (op *optimizer) gearStage(c Config) (Config, bool, error) {
	if err := op.computeWeights(c); err != nil {
		return c, false, err
	}
	var cands [numSlots]slotCandidates
	for s := 0; s < numSlots; s++ {
		cands[s] = op.candidates(s, c.Race)
	}
	changed := false
	jump := op.epGreedy(c, cands)
	c2, ok, err := op.race("gear", c, []Config{jump}, op.describeGear(c), false)
	if err != nil {
		return c, false, err
	}
	if ok {
		c, changed = c2, true
	}
	for pass := 0; pass < 4 && !op.overBudget(); pass++ {
		passChanged := false
		for _, s := range gearOrder {
			moves := []Config{}
			for _, it := range cands[s].items {
				if it.Id == c.Gear[s] {
					continue
				}
				n, ok := op.withItem(c, s, it.Id)
				if !ok {
					continue
				}
				moves = append(moves, n)
				// A one-hander into an empty off hand: also try it with the best fitting off hand.
				if s == slotMainHand && !isTwoHander(it) && n.Gear[slotOffHand] == 0 {
					for _, oh := range cands[slotOffHand].items {
						if m, ok := op.withItem(n, slotOffHand, oh.Id); ok {
							moves = append(moves, m)
							break
						}
					}
				}
			}
			if c.Gear[s] != 0 && s != slotMainHand {
				// Emptying a slot is a legal move too (and can matter: two-hander vs dual wield).
				if n, ok := op.withItem(c, s, 0); ok && s == slotOffHand {
					moves = append(moves, n)
				}
			}
			n, ok, err := op.race("gear", c, moves, op.describeGear(c), false)
			if err != nil {
				return c, changed, err
			}
			if ok {
				c, changed, passChanged = n, true, true
			}
		}
		if !passChanged {
			break
		}
	}
	return c, changed, nil
}

// ---------------------------------------------------------------- enchants

func (op *optimizer) enchantStage(c Config) (Config, bool, error) {
	if op.weights.referenceDPS == 0 {
		if err := op.computeWeights(c); err != nil {
			return c, false, err
		}
	}
	changed := false
	for s := 0; s < numSlots && !op.overBudget(); s++ {
		it := op.p.Data.ItemByID[c.Gear[s]]
		if it == nil {
			continue
		}
		legal := []*proto.UIEnchant{}
		seen := map[int32]bool{}
		for _, e := range op.p.Data.Enchants {
			// The runtime resolves duplicate effect IDs to the first database row. Mirror that
			// resolution exactly; a later duplicate may describe a different slot.
			if seen[e.EffectId] {
				continue
			}
			seen[e.EffectId] = true
			if op.p.enchantFits(e, it, s) {
				legal = append(legal, e)
			}
		}
		sort.Slice(legal, func(a, b int) bool {
			ea, eb := op.enchantEP(legal[a]), op.enchantEP(legal[b])
			if ea != eb {
				return ea > eb
			}
			return legal[a].EffectId < legal[b].EffectId
		})
		moves := []Config{}
		plain, specials := 0, 0
		for _, e := range legal {
			isSpecial := core.HasEnchantEffect(e.EffectId)
			if (isSpecial && specials < 4) || (!isSpecial && plain < 3 && op.enchantEP(e) > 0) {
				if isSpecial {
					specials++
				} else {
					plain++
				}
				if e.EffectId != c.Enchants[s] {
					n := c.Clone()
					n.Enchants[s] = e.EffectId
					moves = append(moves, n)
				}
			}
		}
		n, ok, err := op.race("enchants", c, moves, op.describeGear(c), false)
		if err != nil {
			return c, changed, err
		}
		if ok {
			c, changed = n, true
		}
	}
	return c, changed, nil
}

// ---------------------------------------------------------------- talents

func (op *optimizer) describeTalents(before Config) func(Config) string {
	return func(c Config) string {
		parts := []string{}
		ids := map[string]bool{}
		for id := range c.Talents {
			ids[id] = true
		}
		for id := range before.Talents {
			ids[id] = true
		}
		sorted := []string{}
		for id := range ids {
			sorted = append(sorted, id)
		}
		sort.Strings(sorted)
		for _, id := range sorted {
			if a, b := before.Talents[id], c.Talents[id]; a != b {
				parts = append(parts, fmt.Sprintf("%s %d->%d", op.p.recByID[id].Name, a, b))
			}
		}
		return strings.Join(parts, ", ")
	}
}

// addable lists the builds with one more point in some talent (legal ones only).
func (op *optimizer) addable(c Config, exclude string) []Config {
	out := []Config{}
	for _, r := range op.p.records {
		if r.ID == exclude || c.Talents[r.ID] >= r.MaxRank || r.Mode == "non-sim" || r.Mode == "blocked" {
			continue
		}
		n := c.Clone()
		n.Talents[r.ID]++
		if op.p.TalentError(n.Talents) == "" {
			out = append(out, n)
		}
	}
	return out
}

// removable lists the builds with one point fewer in some talent (legal ones only).
func (op *optimizer) removable(c Config) []Config {
	out := []Config{}
	for _, id := range sortedTalentIDs(c.Talents) {
		n := c.Clone()
		n.Talents[id]--
		if n.Talents[id] == 0 {
			delete(n.Talents, id)
		}
		if op.p.TalentError(n.Talents) == "" {
			out = append(out, n)
		}
	}
	return out
}

func (op *optimizer) fillTalents(c Config) (Config, bool, error) {
	changed := false
	for spent(c.Talents) < op.p.PointsAllowed() && !op.overBudget() {
		moves := op.addable(c, "")
		if len(moves) == 0 {
			break
		}
		n, ok, err := op.race("talents", c, moves, op.describeTalents(c), true)
		if err != nil || !ok {
			return c, changed, err
		}
		c, changed = n, true
	}
	return c, changed, nil
}

func (op *optimizer) talentStage(c Config) (Config, bool, error) {
	changed := false
	// Jump moves: the popular builds truncated to the level (and filled when short).
	jumps := []Config{}
	for _, seed := range op.talentSeeds {
		n := c.Clone()
		n.Talents = copyTalents(seed.Talents)
		jumps = append(jumps, n)
	}
	n, ok, err := op.race("talents", c, jumps, func(x Config) string { return "build " + op.seedLabel(x) }, false)
	if err != nil {
		return c, false, err
	}
	if ok {
		c, changed = n, true
	}
	if n, ok, err := op.fillTalents(c); err != nil {
		return c, changed, err
	} else if ok {
		c, changed = n, true
	}
	// One-point transfers.
	for iter := 0; iter < 25 && !op.overBudget(); iter++ {
		rem := op.removable(c)
		if len(rem) == 0 {
			break
		}
		lvl := op.levels[min(1, len(op.levels)-1)]
		if err := op.ev.Ensure(append([]Config{c}, rem...), lvl); err != nil {
			return c, changed, err
		}
		sort.SliceStable(rem, func(a, b int) bool { return op.ev.Mean(&rem[a], lvl).Mean > op.ev.Mean(&rem[b], lvl).Mean })
		moves := []Config{}
		for _, r := range rem[:min(3, len(rem))] {
			removed := ""
			for id, v := range c.Talents {
				if r.Talents[id] < v {
					removed = id
				}
			}
			moves = append(moves, op.addable(r, removed)...)
		}
		n, ok, err := op.race("talents", c, moves, op.describeTalents(c), false)
		if err != nil {
			return c, changed, err
		}
		if !ok {
			break
		}
		c, changed = n, true
	}
	return c, changed, nil
}

func (op *optimizer) seedLabel(c Config) string {
	for i, s := range op.talentSeeds {
		if talentKey(s.Talents) == talentKey(c.Talents) {
			if i == 0 {
				return "(no talents)"
			}
			return op.p.Presets()[i-1].Label
		}
	}
	return "custom"
}

func talentKey(t map[string]int32) string {
	c := Config{Talents: t}
	return c.Key()
}

// ---------------------------------------------------------------- rotation / race

func (op *optimizer) rotationStage(c Config) (Config, bool, error) {
	moves := []Config{}
	for i := range op.p.Spec.Rotations {
		if i != c.Rotation {
			n := c.Clone()
			n.Rotation = i
			moves = append(moves, n)
		}
	}
	return op.race("rotation", c, moves, func(x Config) string { return "rotation " + op.p.Spec.Rotations[x.Rotation].Label }, false)
}

func (op *optimizer) raceStage(c Config) (Config, bool, error) {
	moves := []Config{}
	for _, r := range op.p.races {
		if r == c.Race {
			continue
		}
		n := c.Clone()
		n.Race = r
		// Faction-restricted items would become illegal: drop them.
		for s := 0; s < numSlots; s++ {
			if n.Gear[s] != 0 && !op.p.itemFits(op.p.Data.ItemByID[n.Gear[s]], s, r) {
				n.Gear[s], n.Enchants[s] = 0, 0
			}
		}
		moves = append(moves, n)
	}
	return op.race("race", c, moves, func(x Config) string { return "race " + x.Race.String() }, false)
}

// ---------------------------------------------------------------- driver

// Optimize searches the problem's space and returns the best configuration found.
func Optimize(p *Problem, o Options) (*Result, error) {
	o.defaults()
	op := &optimizer{p: p, o: o, ev: NewEvaluator(p, o.Seed, o.Parallel), start: time.Now(),
		stageSims: map[string]int64{}, stageTime: map[string]time.Duration{}}
	op.levels = []int32{o.BaseIterations, o.BaseIterations * 3, o.BaseIterations * 9}

	naked := p.NakedConfig()
	op.talentSeeds = []Config{naked}
	for _, pr := range p.Presets() {
		n := naked.Clone()
		n.Talents = p.PresetTalents(pr)
		op.talentSeeds = append(op.talentSeeds, n)
	}
	c := naked
	if o.Start != nil {
		c = o.Start.Clone()
		if err := p.Validate(&c); err != nil {
			return nil, fmt.Errorf("start configuration: %w", err)
		}
	} else if !o.SkipTalents {
		// Best talent seed on the naked character.
		var err error
		if c, _, err = op.race("talents", naked, op.talentSeeds[1:], func(x Config) string { return "build " + op.seedLabel(x) }, len(op.talentSeeds) > 1); err != nil {
			return nil, err
		}
	}
	type stage struct {
		name string
		skip bool
		run  func(Config) (Config, bool, error)
	}
	stages := []stage{
		{"rotation", o.SkipRotation, op.rotationStage},
		{"gear", o.SkipGear, op.gearStage},
		{"talents", o.SkipTalents, op.talentStage},
		{"enchants", o.SkipEnchants, op.enchantStage},
		{"race", o.SkipRace, op.raceStage},
	}
	rounds := 0
	for op.round = 1; op.round <= o.MaxRounds && !op.overBudget(); op.round++ {
		rounds = op.round
		any := false
		for _, st := range stages {
			if st.skip || op.overBudget() {
				continue
			}
			t0, s0 := time.Now(), op.ev.sims.Load()
			n, ok, err := st.run(c)
			op.stageSims[st.name] += op.ev.sims.Load() - s0
			op.stageTime[st.name] += time.Since(t0)
			if err != nil {
				return nil, err
			}
			if ok {
				c, any = n, true
			}
		}
		if !any {
			break
		}
	}
	if err := p.Validate(&c); err != nil {
		return nil, fmt.Errorf("optimizer produced an invalid configuration: %w", err)
	}
	return op.finish(c, rounds)
}

// ---------------------------------------------------------------- result

// Result is the optimizer's answer with its evidence.
type Result struct {
	Spec       string       `json:"spec"`
	Level      int32        `json:"level"`
	Config     ConfigReport `json:"config"`
	DPS        float64      `json:"dps"`
	StdErr     float64      `json:"dps_stderr"`
	CI95       float64      `json:"dps_ci95"`
	Iterations int32        `json:"iterations"`
	Baselines  []Baseline   `json:"baselines"`
	Search     SearchStats  `json:"search"`
	Provenance Provenance   `json:"provenance"`
	Best       Config       `json:"-"`
}

type Baseline struct {
	Label     string  `json:"label"`
	DPS       float64 `json:"dps"`
	CI95      float64 `json:"dps_ci95"`
	GainOver  float64 `json:"optimized_minus_baseline"`
	GainCI95  float64 `json:"difference_ci95"`
	Talents   int32   `json:"talent_points"`
	RaceLabel string  `json:"race"`
}

type ConfigReport struct {
	Race        string            `json:"race"`
	RaceID      int32             `json:"race_id"`
	Rotation    string            `json:"rotation"`
	RotationAPL string            `json:"rotation_apl"`
	Gear        []GearReport      `json:"gear"`
	Talents     map[string]int32  `json:"talents"`
	TalentTrees map[string]int32  `json:"talent_points_by_tree"`
	TalentNames map[string]string `json:"talent_names"`
}

type GearReport struct {
	Slot        string `json:"slot"`
	ItemID      int32  `json:"item_id"`
	Item        string `json:"item"`
	EnchantID   int32  `json:"enchant_id,omitempty"`
	Enchant     string `json:"enchant,omitempty"`
	ReqLevel    int32  `json:"required_level,omitempty"`
	ItemQuality string `json:"quality,omitempty"`
}

type SearchStats struct {
	ElapsedSec      float64            `json:"elapsed_seconds"`
	Rounds          int                `json:"rounds"`
	Sims            int64              `json:"sims"`
	SimIterations   int64              `json:"sim_iterations"`
	CacheHits       int64              `json:"cache_hits"`
	Configs         int                `json:"distinct_configs"`
	InvalidRejected int64              `json:"invalid_configs_rejected"`
	Races           int                `json:"races"`
	Candidates      int                `json:"candidates_raced"`
	StageSims       map[string]int64   `json:"sims_by_stage"`
	StageSeconds    map[string]float64 `json:"seconds_by_stage"`
	RacingLevels    []int32            `json:"racing_iterations"`
	GearPool        map[string]string  `json:"gear_pool_per_slot"`
	Weights         map[string]float64 `json:"stat_weights_dps_per_point"`
	Steps           []Step             `json:"accepted_changes"`
}

type Provenance struct {
	Build          string `json:"gamedata_build"`
	SnapshotSHA256 string `json:"gamedata_snapshot_sha256"`
	ClassicBuild   string `json:"classic_reference_build"`
	ItemDBSHA256   string `json:"item_db_sha256_prefix"`
	Ruleset        string `json:"forever_ruleset"`
	Manifest       string `json:"forever_manifest_sha256"`
	Engine         string `json:"engine_version"`
	GoVersion      string `json:"go_version"`
	Seed           int64  `json:"seed"`
	FinalSeed      int64  `json:"final_seed"`
	Encounter      string `json:"encounter"`
	Parallel       int    `json:"parallel"`
}

func round2(x float64) float64 { return math.Round(x*100) / 100 }

func (op *optimizer) finish(best Config, rounds int) (*Result, error) {
	p := op.p
	// Fresh random numbers for the report: the search's winner is re-simmed, not reused.
	finalSeed := op.o.Seed + 1_000_003
	fe := NewEvaluator(p, finalSeed, op.o.Parallel)
	type bl struct {
		label string
		c     Config
	}
	bls := []bl{{"naked, no talents", p.NakedConfig()}}
	for i, pr := range p.Presets() {
		bls = append(bls, bl{"naked + " + pr.Label, op.talentSeeds[i+1]})
	}
	if op.o.Start != nil {
		bls = append(bls, bl{"start configuration", *op.o.Start})
	}
	all := []Config{best}
	for _, b := range bls {
		all = append(all, b.c)
	}
	n := op.o.FinalIterations
	if err := fe.Ensure(all, n); err != nil {
		return nil, err
	}
	m := fe.Mean(&best, n)
	res := &Result{Spec: p.Spec.Key, Level: p.Level, DPS: round2(m.Mean), StdErr: round2(m.SE), CI95: round2(1.96 * m.SE), Iterations: n, Best: best}
	for _, b := range bls {
		bm := fe.Mean(&b.c, n)
		d := fe.Diff(&best, &b.c, n)
		res.Baselines = append(res.Baselines, Baseline{Label: b.label, DPS: round2(bm.Mean), CI95: round2(1.96 * bm.SE),
			GainOver: round2(d.Mean), GainCI95: round2(1.96 * d.SE), Talents: spent(b.c.Talents), RaceLabel: b.c.Race.String()})
	}
	res.Config = op.report(best)

	sims, iters, hits, invalid, configs := op.ev.Counters()
	fs, fi, _, _, _ := fe.Counters()
	st := SearchStats{ElapsedSec: round2(time.Since(op.start).Seconds()), Rounds: rounds, Sims: sims + fs, SimIterations: iters + fi,
		CacheHits: hits, Configs: configs, InvalidRejected: invalid, Races: op.races, Candidates: op.cands,
		StageSims: op.stageSims, StageSeconds: map[string]float64{}, RacingLevels: op.levels, Steps: op.steps,
		GearPool: map[string]string{}, Weights: map[string]float64{}}
	st.StageSims["final"] = fs
	for k, v := range op.stageTime {
		st.StageSeconds[k] = round2(v.Seconds())
	}
	if op.weights.referenceDPS > 0 {
		for i, w := range op.weights.Stats {
			if w > 0 {
				st.Weights[stats.Stat(i).StatName()] = math.Round(w*1e4) / 1e4
			}
		}
		st.Weights["MainHandDps"] = math.Round(op.weights.MH*1e4) / 1e4
		st.Weights["OffHandDps"] = math.Round(op.weights.OH*1e4) / 1e4
		st.Weights["RangedDps"] = math.Round(op.weights.Ranged*1e4) / 1e4
		for s := 0; s < numSlots; s++ {
			sc := op.candidates(s, best.Race)
			st.GearPool[SlotNames[s]] = fmt.Sprintf("%d legal, %d classes, %d raced", sc.total, sc.groups, len(sc.items))
		}
	}
	res.Search = st
	res.Provenance = op.provenance(finalSeed)
	return res, nil
}

func (op *optimizer) report(c Config) ConfigReport {
	p := op.p
	r := ConfigReport{Race: c.Race.String(), RaceID: int32(c.Race), Rotation: p.Spec.Rotations[c.Rotation].Label,
		RotationAPL: p.Spec.Rotations[c.Rotation].Path, Talents: copyTalents(c.Talents), TalentTrees: map[string]int32{}, TalentNames: map[string]string{}}
	for id, v := range c.Talents {
		rec := p.recByID[id]
		r.TalentTrees[rec.Tree] += v
		r.TalentNames[id] = rec.Name
	}
	for s := 0; s < numSlots; s++ {
		g := GearReport{Slot: SlotNames[s], ItemID: c.Gear[s], Item: op.itemName(c.Gear[s]), EnchantID: c.Enchants[s]}
		if c.Enchants[s] != 0 {
			g.Enchant = op.enchantName(c.Enchants[s])
		}
		if it := p.Data.ItemByID[c.Gear[s]]; it != nil {
			g.ReqLevel = it.RequiredLevel
			g.ItemQuality = strings.TrimPrefix(it.Quality.String(), "ItemQuality")
		}
		r.Gear = append(r.Gear, g)
	}
	return r
}

func (op *optimizer) provenance(finalSeed int64) Provenance {
	p := op.p
	pv := Provenance{Ruleset: p.Data.RulesetID, Manifest: p.Data.Manifest, ItemDBSHA256: p.Data.DBHash, GoVersion: runtime.Version(),
		Seed: op.o.Seed, FinalSeed: finalSeed, Parallel: op.o.Parallel,
		Encounter: fmt.Sprintf("%.0fs +-10s vs level %d humanoid, armor %.0f, no buffs/consumables", p.duration(), p.targetLevel(), p.Data.targetArmor(p.targetLevel()))}
	if snap, err := gamedata.ForBuild(p.Build); err == nil && snap != nil {
		pv.Build, pv.SnapshotSHA256, pv.ClassicBuild = snap.Build, snap.SnapshotSHA256, snap.ClassicBuild
	}
	pv.Engine = engineVersion(p.Data.Root)
	return pv
}

// engineVersion reads the sim repository's checked-out commit without running git.
func engineVersion(root string) string {
	head, err := os.ReadFile(filepath.Join(root, ".git", "HEAD"))
	if err != nil {
		return "unknown"
	}
	ref := strings.TrimSpace(string(head))
	if !strings.HasPrefix(ref, "ref: ") {
		return ref
	}
	name := strings.TrimPrefix(ref, "ref: ")
	if sha, err := os.ReadFile(filepath.Join(root, ".git", filepath.FromSlash(name))); err == nil {
		return strings.TrimPrefix(name, "refs/heads/") + "@" + strings.TrimSpace(string(sha))[:12] + " (+ working tree)"
	}
	if packed, err := os.ReadFile(filepath.Join(root, ".git", "packed-refs")); err == nil {
		for _, line := range strings.Split(string(packed), "\n") {
			if f := strings.Fields(line); len(f) == 2 && f[1] == name && len(f[0]) >= 12 {
				return strings.TrimPrefix(name, "refs/heads/") + "@" + f[0][:12] + " (+ working tree)"
			}
		}
	}
	return strings.TrimPrefix(name, "refs/heads/")
}

var _ = slices.Contains[[]int]
