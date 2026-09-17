package main

// Talent optimizer (optimize-talents, optimize-all).
//
// The search runs under the same normalization as rank (gear, buffs,
// consumables, race, APL, encounter, Forever auto-rotation policy) with its own
// seed, so the reported ranking seed is not the one the builds were tuned on.
//
// Legality: Forever builds use the record-keyed topology from
// ui/forever/data/talents.json (row gates = required_points, prerequisites with
// ranks, 51 points) and every evaluated build passes foreverdata.Validate inside
// toForever. Classic builds use the UI talent trees (presets/classic_talent_trees.json:
// 5 points per row, prerequisites at max rank, 51 points); the Classic engine
// itself does not validate talent strings.
//
// Algorithm (deterministic for a fixed seed unless -budget-seconds is hit):
//  1. starts: the Forever repair of the Classic build (UI preset or optimized
//     Classic build) / the Classic UI preset, and an empty build;
//  2. greedy from empty: repeatedly take the move with the best DPS gain per
//     point, where a move is +1 rank on an available talent or "unlock" (the
//     cheapest gate/prerequisite filler, chosen by measured per-point value, plus
//     the talent); only the spec's focus tree until it has FocusMin points, then
//     all trees; required talents are forced once the focus is reached;
//  3. local swaps on every start: remove 1 point (the 6 least costly removals)
//     and add 1 point anywhere legal, until no swap gains above noise;
//  4. unspent points are filled only with talents that gain DPS (> 0) at the
//     highest iteration level; talents whose effect is not simulated (Forever
//     mode non-sim/blocked, or no effective rank in the mode) are never candidates.
//
// Noise: every comparison uses common random numbers (fixed seed). Candidates
// are screened at iterations N, the close ones (within 2 standard errors of the
// best) re-run at 5N and 15N. A move is accepted at the highest level unless it
// is decisively ahead earlier (gain > 4 SE and 4 SE clear of the runner-up), and
// only if it gains more than max(0.05% DPS, 0.5 x combined SE).

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/game"
)

//go:embed presets/classic_talent_trees.json
var classicTreesJSON []byte

const talentBudget = 51

// ---------------------------------------------------------------- model

type nodePrereq struct {
	idx  int
	rank int32
}

type talentNode struct {
	ID       string
	Name     string
	Tree     int
	Row      int32 // 0-based
	Max      int32
	Req      int32 // points required in lower rows of the tree
	Prereqs  []nodePrereq
	Excluded string // non-empty: never a candidate (effect not simulated)
}

type talentModel struct {
	Game      string
	Class     proto.Class
	Trees     []string
	Nodes     []talentNode
	treeStart []int
	byID      map[string]int
	d         *fvDataset
}

func foreverTalentModel(d *fvDataset, c proto.Class, mode proto.ForeverMode) *talentModel {
	m := &talentModel{Game: "forever", Class: c, Trees: d.treeNames(c), byID: map[string]int{}, d: d}
	recs := d.classRecords(c)
	for i, r := range recs {
		m.byID[r.ID] = i
	}
	for i, r := range recs {
		tree := 0
		for t, name := range m.Trees {
			if name == r.Tree {
				tree = t
			}
		}
		if len(m.treeStart) <= tree {
			m.treeStart = append(m.treeStart, i)
		}
		n := talentNode{ID: r.ID, Name: r.Name, Tree: tree, Row: r.Row - 1, Max: r.MaxRank, Req: r.RequiredPoints}
		for _, p := range r.Prerequisites {
			if j, ok := m.byID[p.ID]; ok {
				n.Prereqs = append(n.Prereqs, nodePrereq{j, p.Rank})
			}
		}
		probe := &proto.ForeverOptions{RulesetId: d.RulesetID, Mode: mode, Talents: map[string]int32{r.ID: r.MaxRank}}
		switch {
		case !isExecutable(r.Mode):
			n.Excluded = "mode " + r.Mode + ": effect not simulated"
		case foreverdata.EffectiveRank(probe, r.ID) == 0:
			n.Excluded = "no effective rank in " + mode.String() + ": effect not simulated in this mode"
		}
		m.Nodes = append(m.Nodes, n)
	}
	return m
}

type uiTalentTree struct {
	Name    string `json:"name"`
	Talents []struct {
		FieldName string `json:"fieldName"`
		Location  struct {
			RowIdx int32 `json:"rowIdx"`
			ColIdx int32 `json:"colIdx"`
		} `json:"location"`
		MaxPoints      int32 `json:"maxPoints"`
		PrereqLocation *struct {
			RowIdx int32 `json:"rowIdx"`
			ColIdx int32 `json:"colIdx"`
		} `json:"prereqLocation"`
	} `json:"talents"`
}

func classicTalentModel(c proto.Class) (*talentModel, error) {
	var file struct {
		Classes map[string][]uiTalentTree `json:"classes"`
	}
	if err := json.Unmarshal(classicTreesJSON, &file); err != nil {
		return nil, fmt.Errorf("classic talent trees: %w", err)
	}
	trees, ok := file.Classes[strings.ToLower(strings.TrimPrefix(c.String(), "Class"))]
	if !ok {
		return nil, fmt.Errorf("no Classic talent trees for %s", c)
	}
	m := &talentModel{Game: "classic", Class: c, byID: map[string]int{}}
	for t, tree := range trees {
		m.Trees = append(m.Trees, tree.Name)
		m.treeStart = append(m.treeStart, len(m.Nodes))
		start := len(m.Nodes)
		for _, tal := range tree.Talents {
			m.byID[tal.FieldName] = len(m.Nodes)
			m.Nodes = append(m.Nodes, talentNode{ID: tal.FieldName, Name: tal.FieldName, Tree: t, Row: tal.Location.RowIdx, Max: tal.MaxPoints, Req: tal.Location.RowIdx * 5})
		}
		for k, tal := range tree.Talents {
			if tal.PrereqLocation == nil {
				continue
			}
			for j, other := range tree.Talents {
				if other.Location.RowIdx == tal.PrereqLocation.RowIdx && other.Location.ColIdx == tal.PrereqLocation.ColIdx {
					m.Nodes[start+k].Prereqs = append(m.Nodes[start+k].Prereqs, nodePrereq{start + j, other.MaxPoints})
				}
			}
		}
	}
	return m, nil
}

func cloneState(s []int32) []int32 { return append([]int32(nil), s...) }

func (m *talentModel) total(s []int32) int32 {
	n := int32(0)
	for _, v := range s {
		n += v
	}
	return n
}

func (m *talentModel) treePoints(s []int32, tree int) int32 {
	n := int32(0)
	for i, node := range m.Nodes {
		if node.Tree == tree {
			n += s[i]
		}
	}
	return n
}

func (m *talentModel) lower(s []int32, i int) int32 {
	n := int32(0)
	for j, node := range m.Nodes {
		if node.Tree == m.Nodes[i].Tree && node.Row < m.Nodes[i].Row {
			n += s[j]
		}
	}
	return n
}

func (m *talentModel) available(s []int32, i int) bool {
	for _, p := range m.Nodes[i].Prereqs {
		if s[p.idx] < p.rank {
			return false
		}
	}
	return m.lower(s, i) >= m.Nodes[i].Req
}

func (m *talentModel) legal(s []int32) error {
	if len(s) != len(m.Nodes) {
		return fmt.Errorf("build has %d nodes, want %d", len(s), len(m.Nodes))
	}
	if t := m.total(s); t > talentBudget {
		return fmt.Errorf("talent point budget exceeded: %d > %d", t, talentBudget)
	}
	for i, n := range m.Nodes {
		if s[i] < 0 || s[i] > n.Max {
			return fmt.Errorf("invalid rank %d for %s", s[i], n.Name)
		}
		if s[i] > 0 && !m.available(s, i) {
			return fmt.Errorf("%s: row gate (%d points below) or prerequisite not met", n.Name, n.Req)
		}
	}
	return nil
}

func (m *talentModel) key(s []int32) string {
	b := make([]byte, len(s))
	for i, v := range s {
		b[i] = byte('0' + v)
	}
	return string(b)
}

func (m *talentModel) toMap(s []int32) map[string]int32 {
	out := map[string]int32{}
	for i, v := range s {
		if v > 0 {
			out[m.Nodes[i].ID] = v
		}
	}
	return out
}

func (m *talentModel) fromMap(t map[string]int32) ([]int32, error) {
	s := make([]int32, len(m.Nodes))
	for id, v := range t {
		i, ok := m.byID[id]
		if !ok {
			return nil, fmt.Errorf("unknown talent %s", id)
		}
		s[i] = v
	}
	return s, nil
}

func (m *talentModel) encode(s []int32) string {
	if m.Game == "forever" {
		return m.d.encodeTalents(m.Class, m.toMap(s))
	}
	trees := []string{}
	for t := range m.Trees {
		var b strings.Builder
		for i, n := range m.Nodes {
			if n.Tree == t {
				b.WriteByte(byte('0' + s[i]))
			}
		}
		trees = append(trees, strings.TrimRight(b.String(), "0"))
	}
	return strings.Join(trees, "-")
}

func (m *talentModel) decode(text string) ([]int32, error) {
	if m.Game == "forever" {
		t, err := m.d.decodeTalents(m.Class, text)
		if err != nil {
			return nil, err
		}
		return m.fromMap(t)
	}
	s := make([]int32, len(m.Nodes))
	parts := strings.Split(text, "-")
	if len(parts) > len(m.Trees) {
		return nil, fmt.Errorf("Classic talent string %q has %d trees, want %d", text, len(parts), len(m.Trees))
	}
	for t, part := range parts {
		size := len(m.Nodes) - m.treeStart[t]
		if t+1 < len(m.treeStart) {
			size = m.treeStart[t+1] - m.treeStart[t]
		}
		if len(part) > size {
			return nil, fmt.Errorf("Classic talent string tree %d has %d digits, max %d", t, len(part), size)
		}
		for j, ch := range part {
			if ch < '0' || ch > '5' {
				return nil, fmt.Errorf("Classic talent string: invalid rank %q", ch)
			}
			s[m.treeStart[t]+j] = int32(ch - '0')
		}
	}
	return s, nil
}

func (m *talentModel) treeSummary(s []int32) string {
	parts := []string{}
	for t, name := range m.Trees {
		parts = append(parts, fmt.Sprintf("%s %d", name, m.treePoints(s, t)))
	}
	return strings.Join(parts, " / ")
}

// validateEngine checks a build with the engine's own validation (Forever) or
// the UI tree rules (Classic).
func (m *talentModel) validateEngine(s []int32, v Variant, race proto.Race, mode proto.ForeverMode) error {
	if err := m.legal(s); err != nil {
		return err
	}
	if m.Game != "forever" {
		return nil
	}
	p := &proto.Player{Class: v.Class, Race: race}
	if err := setSpecOptions(p, v.OneofField, nil); err != nil {
		return err
	}
	f := m.d.defaultOptions(v.Class, race, mode)
	f.Talents = m.toMap(s)
	p.Forever = f
	return foreverdata.Validate(p)
}

// ---------------------------------------------------------------- constraints

type buildConstraints struct {
	FocusTree int      `json:"focus_tree_index"`
	Focus     string   `json:"focus_tree"`
	FocusMin  int32    `json:"focus_min_points"`
	Require   []string `json:"required,omitempty"`
	Forbid    []string `json:"forbidden,omitempty"`
	require   []int
	forbid    map[int]bool
}

func constraintsFor(m *talentModel, v Variant) (buildConstraints, error) {
	c := buildConstraints{FocusTree: v.FocusTree, FocusMin: v.FocusMin, forbid: map[int]bool{}}
	if c.FocusTree < 0 || c.FocusTree >= len(m.Trees) {
		return c, fmt.Errorf("%s: focus tree %d out of range", v.ID, c.FocusTree)
	}
	c.Focus = m.Trees[c.FocusTree]
	req, forbid := v.RequireForever, v.ForbidForever
	if m.Game == "classic" {
		req, forbid = v.RequireClassic, v.ForbidClassic
	}
	for _, id := range req {
		i, ok := m.byID[id]
		if !ok {
			return c, fmt.Errorf("%s: required talent %s not in the %s tree", v.ID, id, m.Game)
		}
		c.require = append(c.require, i)
		c.Require = append(c.Require, m.Nodes[i].Name)
	}
	for _, id := range forbid {
		i, ok := m.byID[id]
		if !ok {
			return c, fmt.Errorf("%s: forbidden talent %s not in the %s tree", v.ID, id, m.Game)
		}
		c.forbid[i] = true
		c.Forbid = append(c.Forbid, m.Nodes[i].Name)
	}
	return c, nil
}

func (c buildConstraints) satisfied(m *talentModel, s []int32) bool {
	if m.treePoints(s, c.FocusTree) < c.FocusMin {
		return false
	}
	for _, i := range c.require {
		if s[i] == 0 {
			return false
		}
	}
	for i := range c.forbid {
		if s[i] > 0 {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------- evaluation

type evalResult struct {
	DPS    float64
	StdErr float64
	Err    string
	Added  []string
}

type OptStep struct {
	Phase       string  `json:"phase"`
	Move        string  `json:"move"`
	PointsSpent int32   `json:"points_after"`
	DPSBefore   float64 `json:"dps_before"`
	DPSAfter    float64 `json:"dps_after"`
	GainPct     float64 `json:"gain_pct"`
	Iterations  int32   `json:"decided_at_iterations"`
	Candidates  int     `json:"candidates"`
}

type optimizer struct {
	b        *bench
	game     string
	e        PresetEntry
	race     proto.Race
	enc      EncounterSpec
	m        *talentModel
	cons     buildConstraints
	levels   []int32
	parallel int
	deadline time.Time

	mu        sync.Mutex
	cache     map[string]evalResult
	sims      int64
	values    map[int]float64
	budgetHit bool
}

func (o *optimizer) overBudget() bool {
	if !o.deadline.IsZero() && time.Now().After(o.deadline) {
		o.budgetHit = true
	}
	return o.budgetHit
}

func (o *optimizer) evalOne(s []int32, iterations int32) (res evalResult, sims int64) {
	req, err := o.b.classicRequest(o.e, o.race, o.enc, iterations)
	if err != nil {
		return evalResult{Err: err.Error()}, 0
	}
	var r RunResult
	if o.game == "classic" {
		req.Raid.Parties[0].Players[0].TalentsString = o.m.encode(s)
		r, sims = runGame(game.Classic, req), 1
	} else {
		freq, _, err := toForever(o.b.d, req, o.b.mode, TalentPolicy{Name: "override", Override: o.m.toMap(s)})
		if err != nil {
			return evalResult{Err: err.Error()}, 0
		}
		freq, rep, cur, err := o.b.autoRotationN(freq, iterations)
		if err != nil {
			return evalResult{Err: err.Error()}, 1
		}
		res.Added = rep.Added
		if cur != nil {
			r, sims = *cur, int64(1+len(rep.Candidates))
		} else {
			r, sims = runGame(game.Forever, freq), 1
		}
	}
	res.DPS, res.Err = r.DPS, r.Error
	if r.Iterations > 0 {
		res.StdErr = r.Stdev / math.Sqrt(float64(r.Iterations))
	}
	return res, sims
}

// evaluate runs the states at the iteration count (cached, parallel).
func (o *optimizer) evaluate(states [][]int32, iterations int32) []evalResult {
	keys := make([]string, len(states))
	todo := map[string][]int32{}
	order := []string{}
	o.mu.Lock()
	for i, s := range states {
		keys[i] = fmt.Sprintf("%s@%d", o.m.key(s), iterations)
		if _, ok := o.cache[keys[i]]; !ok {
			if _, queued := todo[keys[i]]; !queued {
				todo[keys[i]] = s
				order = append(order, keys[i])
			}
		}
	}
	o.mu.Unlock()
	jobs := []func(){}
	for _, k := range order {
		k, s := k, todo[k]
		jobs = append(jobs, func() {
			r, n := o.evalOne(s, iterations)
			o.mu.Lock()
			o.cache[k] = r
			o.sims += n
			o.mu.Unlock()
		})
	}
	runJobs(jobs, o.parallel)
	out := make([]evalResult, len(states))
	o.mu.Lock()
	for i, k := range keys {
		out[i] = o.cache[k]
	}
	o.mu.Unlock()
	return out
}

// ---------------------------------------------------------------- search

type optMove struct {
	state []int32
	desc  string
	cost  int32
	node  int // single-point move on this node, else -1
}

type selectMode int

const (
	selectNormal selectMode = iota
	selectFill              // accept any positive gain at the top level
	selectForce             // accept the best candidate even without a gain
)

type decision struct {
	move       optMove
	base, best evalResult
	iterations int32
	candidates int
}

func (o *optimizer) threshold(base, comb float64, mode selectMode) float64 {
	switch mode {
	case selectFill:
		return 0
	case selectForce:
		return math.Inf(-1)
	}
	return math.Max(0.0005*base, 0.5*comb)
}

func (o *optimizer) selectBest(base []int32, moves []optMove, mode selectMode) (decision, bool) {
	if len(moves) == 0 {
		return decision{}, false
	}
	cands := make([]int, len(moves))
	for i := range cands {
		cands[i] = i
	}
	keep := []int{8, 4}
	var last decision
	for li, it := range o.levels {
		states := [][]int32{base}
		for _, i := range cands {
			states = append(states, moves[i].state)
		}
		res := o.evaluate(states, it)
		br := res[0]
		if br.Err != "" {
			return decision{}, false
		}
		type scored struct {
			i         int
			r         evalResult
			score, se float64
		}
		list := []scored{}
		for k, i := range cands {
			r := res[k+1]
			if r.Err != "" {
				continue
			}
			cost := float64(max(1, moves[i].cost))
			list = append(list, scored{i, r, (r.DPS - br.DPS) / cost, math.Sqrt(r.StdErr*r.StdErr+br.StdErr*br.StdErr) / cost})
		}
		if li == 0 {
			for _, x := range list {
				if moves[x.i].node >= 0 && moves[x.i].cost == 1 {
					o.values[moves[x.i].node] = x.score
				}
			}
		}
		if len(list) == 0 {
			return decision{}, false
		}
		sort.SliceStable(list, func(a, b int) bool { return list[a].score > list[b].score })
		top := list[0]
		gain := top.r.DPS - br.DPS
		comb := math.Sqrt(top.r.StdErr*top.r.StdErr + br.StdErr*br.StdErr)
		last = decision{move: moves[top.i], base: br, best: top.r, iterations: it, candidates: len(moves)}
		final := li == len(o.levels)-1
		if mode != selectForce && (gain+2*comb <= 0 || (final && gain <= 0)) {
			return last, false
		}
		thr := o.threshold(br.DPS, comb, mode)
		decisive := len(list) == 1 || top.score-list[1].score > 4*math.Max(top.se, list[1].se)
		if final || (mode != selectForce && decisive && gain > thr && gain > 4*comb) {
			return last, gain > thr
		}
		next := []int{}
		for _, x := range list {
			if len(next) >= keep[min(li, len(keep)-1)] {
				break
			}
			if x.score >= top.score-2*math.Sqrt(top.se*top.se+x.se*x.se) {
				next = append(next, x.i)
			}
		}
		cands = next
	}
	return last, false
}

func (o *optimizer) step(phase string, d decision) OptStep {
	pct := 0.0
	if d.base.DPS > 0 {
		pct = round2((d.best.DPS - d.base.DPS) / d.base.DPS * 100)
	}
	return OptStep{Phase: phase, Move: d.move.desc, PointsSpent: o.m.total(d.move.state), DPSBefore: round2(d.base.DPS), DPSAfter: round2(d.best.DPS),
		GainPct: pct, Iterations: d.iterations, Candidates: d.candidates}
}

func (o *optimizer) fillerBetter(j, k int) bool {
	vj, vk := o.values[j], o.values[k]
	// A measured value of exactly zero means the talent changed nothing in the
	// sim (identical random stream): use such talents as filler last.
	if (vj == 0) != (vk == 0) {
		return vk == 0
	}
	if vj != vk {
		return vj > vk
	}
	if o.m.Nodes[j].Row != o.m.Nodes[k].Row {
		return o.m.Nodes[j].Row < o.m.Nodes[k].Row
	}
	return j < k
}

// ensure makes node i available: prerequisites first, then row-gate filler
// (best measured per-point value, then lowest row).
func (o *optimizer) ensure(c []int32, i, depth int, filler map[int]int32) bool {
	if depth > 8 {
		return false
	}
	n := o.m.Nodes[i]
	for _, p := range n.Prereqs {
		for c[p.idx] < p.rank {
			if !o.m.available(c, p.idx) && !o.ensure(c, p.idx, depth+1, filler) {
				return false
			}
			c[p.idx]++
			filler[p.idx]++
			if o.m.total(c) > talentBudget {
				return false
			}
		}
	}
	for o.m.lower(c, i) < n.Req {
		best := -1
		for j, x := range o.m.Nodes {
			if x.Tree != n.Tree || x.Row >= n.Row || c[j] >= x.Max || x.Excluded != "" || o.cons.forbid[j] || !o.m.available(c, j) {
				continue
			}
			if best < 0 || o.fillerBetter(j, best) {
				best = j
			}
		}
		if best < 0 {
			return false
		}
		c[best]++
		filler[best]++
		if o.m.total(c) > talentBudget {
			return false
		}
	}
	return o.m.available(c, i)
}

func (o *optimizer) unlock(s []int32, i int) ([]int32, string) {
	c := cloneState(s)
	filler := map[int]int32{}
	if !o.ensure(c, i, 0, filler) {
		return nil, ""
	}
	c[i]++
	if o.m.legal(c) != nil {
		return nil, ""
	}
	names := []string{}
	for j := range o.m.Nodes {
		if filler[j] > 0 {
			names = append(names, fmt.Sprintf("%s +%d", o.m.Nodes[j].Name, filler[j]))
		}
	}
	return c, fmt.Sprintf("+%s (1/%d) with filler: %s", o.m.Nodes[i].Name, o.m.Nodes[i].Max, strings.Join(names, ", "))
}

func (o *optimizer) feasible(s []int32) bool {
	need := max(0, o.cons.FocusMin-o.m.treePoints(s, o.cons.FocusTree))
	for _, i := range o.cons.require {
		if s[i] == 0 {
			need++
		}
	}
	return talentBudget-o.m.total(s) >= need
}

// addMoves lists +1 rank moves on available talents and, with unlocks, the
// composite moves that fill a gate/prerequisite and take a locked talent.
func (o *optimizer) addMoves(s []int32, focusOnly, unlocks bool) []optMove {
	moves := []optMove{}
	for i, n := range o.m.Nodes {
		if n.Excluded != "" || o.cons.forbid[i] || s[i] >= n.Max || (focusOnly && n.Tree != o.cons.FocusTree) {
			continue
		}
		var ns []int32
		desc, node := "", i
		switch {
		case o.m.available(s, i):
			ns = cloneState(s)
			ns[i]++
			desc = fmt.Sprintf("+%s (%d/%d)", n.Name, ns[i], n.Max)
		case s[i] == 0 && unlocks:
			ns, desc = o.unlock(s, i)
			node = -1
		}
		if ns == nil || o.m.total(ns) > talentBudget || !o.feasible(ns) || o.m.legal(ns) != nil {
			continue
		}
		moves = append(moves, optMove{ns, desc, o.m.total(ns) - o.m.total(s), node})
	}
	return moves
}

func (o *optimizer) forceRequired(s []int32) ([]int32, []OptStep) {
	steps := []OptStep{}
	for _, i := range o.cons.require {
		if s[i] > 0 {
			continue
		}
		var ns []int32
		desc := ""
		if o.m.available(s, i) {
			ns = cloneState(s)
			ns[i]++
			desc = "+" + o.m.Nodes[i].Name + " (required)"
		} else if ns, desc = o.unlock(s, i); ns != nil {
			desc += " (required)"
		}
		if ns == nil || o.m.total(ns) > talentBudget {
			continue
		}
		steps = append(steps, OptStep{Phase: "require", Move: desc, PointsSpent: o.m.total(ns)})
		s = ns
	}
	return s, steps
}

func (o *optimizer) greedy() ([]int32, []OptStep) {
	s := make([]int32, len(o.m.Nodes))
	steps := []OptStep{}
	for guard := 0; guard < 2*talentBudget && o.m.total(s) < talentBudget && !o.overBudget(); guard++ {
		focusPhase := o.m.treePoints(s, o.cons.FocusTree) < o.cons.FocusMin
		if !focusPhase {
			var forced []OptStep
			s, forced = o.forceRequired(s)
			steps = append(steps, forced...)
		}
		// Measure single-point values first so unlock filler uses them.
		singles := o.addMoves(s, focusPhase, false)
		states := [][]int32{s}
		for _, mv := range singles {
			states = append(states, mv.state)
		}
		res := o.evaluate(states, o.levels[0])
		for k, mv := range singles {
			if res[0].Err == "" && res[k+1].Err == "" {
				o.values[mv.node] = res[k+1].DPS - res[0].DPS
			}
		}
		moves := o.addMoves(s, focusPhase, true)
		phase := "greedy"
		if focusPhase {
			phase = "greedy-focus"
		}
		d, ok := o.selectBest(s, moves, selectNormal)
		if !ok && focusPhase {
			d, ok = o.selectBest(s, moves, selectForce)
			phase = "greedy-focus-forced"
		}
		if !ok {
			break
		}
		steps = append(steps, o.step(phase, d))
		s = d.move.state
	}
	var forced []OptStep
	s, forced = o.forceRequired(s)
	steps = append(steps, forced...)
	return s, steps
}

func (o *optimizer) fill(s []int32) ([]int32, []OptStep) {
	steps := []OptStep{}
	for o.m.total(s) < talentBudget && !o.overBudget() {
		d, ok := o.selectBest(s, o.addMoves(s, false, true), selectFill)
		if !ok {
			break
		}
		steps = append(steps, o.step("fill", d))
		s = d.move.state
	}
	return s, steps
}

func (o *optimizer) swaps(s []int32) ([]int32, []OptStep) {
	steps := []OptStep{}
	required := map[int]bool{}
	for _, i := range o.cons.require {
		required[i] = true
	}
	for round := 0; round < 60 && !o.overBudget(); round++ {
		removals := []optMove{}
		for i, n := range o.m.Nodes {
			if s[i] == 0 || (required[i] && s[i] == 1) {
				continue
			}
			c := cloneState(s)
			c[i]--
			if o.m.legal(c) == nil && o.cons.satisfied(o.m, c) {
				removals = append(removals, optMove{c, "-" + n.Name, 1, i})
			}
		}
		moves := []optMove{}
		seen := map[string]bool{o.m.key(s): true}
		if len(removals) > 0 {
			states := [][]int32{}
			for _, r := range removals {
				states = append(states, r.state)
			}
			res := o.evaluate(states, o.levels[0])
			idx := make([]int, len(removals))
			for i := range idx {
				idx[i] = i
			}
			sort.SliceStable(idx, func(a, b int) bool {
				ra, rb := res[idx[a]], res[idx[b]]
				if (ra.Err == "") != (rb.Err == "") {
					return ra.Err == ""
				}
				return ra.DPS > rb.DPS
			})
			for _, k := range idx[:min(6, len(idx))] {
				r := removals[k]
				for j, n := range o.m.Nodes {
					if j == r.node || n.Excluded != "" || o.cons.forbid[j] || r.state[j] >= n.Max || !o.m.available(r.state, j) {
						continue
					}
					c := cloneState(r.state)
					c[j]++
					key := o.m.key(c)
					if seen[key] || o.m.legal(c) != nil || !o.cons.satisfied(o.m, c) {
						continue
					}
					seen[key] = true
					moves = append(moves, optMove{c, fmt.Sprintf("%s +%s", r.desc, n.Name), 1, -1})
				}
			}
		}
		if o.m.total(s) < talentBudget {
			for _, mv := range o.addMoves(s, false, false) {
				if o.cons.satisfied(o.m, mv.state) && !seen[o.m.key(mv.state)] {
					seen[o.m.key(mv.state)] = true
					moves = append(moves, mv)
				}
			}
		}
		d, ok := o.selectBest(s, moves, selectNormal)
		if !ok {
			break
		}
		steps = append(steps, o.step("swap", d))
		s = d.move.state
	}
	return s, steps
}

// ---------------------------------------------------------------- driver

type OptStart struct {
	Name          string    `json:"name"`
	Talents       string    `json:"talents"`
	TreePoints    string    `json:"tree_points"`
	DPS           float64   `json:"dps"`
	Note          string    `json:"note,omitempty"`
	Refined       string    `json:"refined_talents,omitempty"`
	RefinedPoints string    `json:"refined_tree_points,omitempty"`
	RefinedDPS    float64   `json:"refined_dps,omitempty"`
	Path          []OptStep `json:"path"`
}

type OptimizeResult struct {
	Spec              string           `json:"spec"`
	Game              string           `json:"game"`
	Phase             string           `json:"phase"`
	Race              string           `json:"race"`
	GearStatus        string           `json:"gear_status"`
	GearLabel         string           `json:"gear_label"`
	APL               string           `json:"apl"`
	Targets           int              `json:"targets"`
	DurationS         float64          `json:"duration_s"`
	Seed              int64            `json:"seed"`
	Iterations        []int32          `json:"iterations"`
	Talents           string           `json:"talents"`
	TreePoints        string           `json:"tree_points"`
	Points            int32            `json:"points"`
	DPS               float64          `json:"dps"`
	StdErr            float64          `json:"stderr"`
	CI95              [2]float64       `json:"ci95"`
	AutoRotationAdded []string         `json:"forever_auto_rotation_added,omitempty"`
	Baseline          *BuildBaseline   `json:"baseline"`
	GainVsBaselinePct *float64         `json:"gain_vs_baseline_pct,omitempty"`
	Chosen            string           `json:"chosen_start"`
	Starts            []OptStart       `json:"starts"`
	Constraints       buildConstraints `json:"constraints"`
	NotSimulated      []string         `json:"excluded_not_simulated,omitempty"`
	NormalizationHash string           `json:"normalization_hash"`
	Sims              int64            `json:"sims"`
	WallSeconds       float64          `json:"wall_seconds"`
	BudgetExhausted   bool             `json:"budget_exhausted"`
	Error             string           `json:"error,omitempty"`
}

type optimizeOptions struct {
	game     string
	phase    Phase
	enc      EncounterSpec
	base     int32
	budget   time.Duration
	parallel int
}

func (b *bench) optimize(specID string, opts optimizeOptions) (OptimizeResult, error) {
	start := time.Now()
	e, err := matrixEntry(specID, opts.phase)
	if err != nil {
		return OptimizeResult{}, err
	}
	e = e.withGearFill(b.norm.GearFill)
	if e.Status != "ok" {
		return OptimizeResult{}, fmt.Errorf("%s has no simmable gear for %s: %s", specID, opts.phase, e.Reason)
	}
	v := e.variant
	var m *talentModel
	if opts.game == "forever" {
		m = foreverTalentModel(b.d, v.Class, b.mode)
	} else if m, err = classicTalentModel(v.Class); err != nil {
		return OptimizeResult{}, err
	}
	cons, err := constraintsFor(m, v)
	if err != nil {
		return OptimizeResult{}, err
	}
	race := b.chooseRace(e, true, game.Version(opts.game), opts.enc)
	o := &optimizer{b: b, game: opts.game, e: e, race: race.race, enc: opts.enc, m: m, cons: cons,
		levels: []int32{opts.base, opts.base * 5, opts.base * 15}, parallel: max(1, opts.parallel),
		cache: map[string]evalResult{}, values: map[int]float64{}}
	if opts.budget > 0 {
		o.deadline = start.Add(opts.budget)
	}
	top := o.levels[len(o.levels)-1]
	res := OptimizeResult{Spec: specID, Game: opts.game, Phase: opts.phase.String(), Race: race.Race, GearStatus: e.GearStatus, GearLabel: e.GearLabel,
		APL: e.APLPreset, Targets: opts.enc.Targets, DurationS: opts.enc.Duration, Seed: b.norm.Seed, Iterations: o.levels, Constraints: cons,
		NormalizationHash: normalizationHash(b.norm), Starts: []OptStart{}}
	for _, n := range m.Nodes {
		if n.Excluded != "" {
			res.NotSimulated = append(res.NotSimulated, n.Name+" ("+n.Excluded+")")
		}
	}

	// Baseline start: Forever repair of the Classic build / Classic UI preset.
	type startState struct {
		name  string
		state []int32
		note  string
		path  []OptStep
	}
	starts := []startState{}
	resolved := b.resolveTalents(e)
	res.Baseline = &BuildBaseline{Kind: "none"}
	switch {
	case opts.game == "forever" && resolved.TalentsString != "":
		repaired, _ := b.d.repairClassicTalents(v.Class, resolved.TalentsString, true)
		s, err := m.fromMap(repaired)
		if err == nil {
			res.Baseline = &BuildBaseline{Kind: "repair", Talents: m.encode(s)}
			starts = append(starts, startState{name: "repair of Classic " + resolved.TalentSource, state: s})
		}
	case opts.game == "classic" && v.Talents != "":
		s, err := m.decode(e.TalentsString)
		if err == nil && m.legal(s) == nil {
			res.Baseline = &BuildBaseline{Kind: "ui", Talents: m.encode(s)}
			starts = append(starts, startState{name: "UI preset " + e.TalentPreset, state: s})
		} else {
			note := "UI preset is not legal under the UI tree rules"
			if err != nil {
				note = err.Error()
			}
			res.Baseline = &BuildBaseline{Kind: "ui", Talents: e.TalentsString, Error: note}
		}
	}
	greedyState, greedyPath := o.greedy()
	starts = append(starts, startState{name: "greedy from empty", state: greedyState, path: greedyPath})

	finals := [][]int32{}
	for i := range starts {
		st := &starts[i]
		refined := st.state
		if !cons.satisfied(m, st.state) {
			st.note = "violates the spec identity constraints; not refined"
		} else {
			var p []OptStep
			refined, p = o.swaps(st.state)
			st.path = append(st.path, p...)
			refined, p = o.fill(refined)
			st.path = append(st.path, p...)
		}
		finals = append(finals, refined)
	}
	all := [][]int32{}
	for i := range starts {
		all = append(all, starts[i].state, finals[i])
	}
	evals := o.evaluate(all, top)
	best := -1
	for i := range starts {
		st, fin := evals[2*i], evals[2*i+1]
		rep := OptStart{Name: starts[i].name, Talents: m.encode(starts[i].state), TreePoints: m.treeSummary(starts[i].state), DPS: round2(st.DPS), Note: starts[i].note, Path: starts[i].path}
		if st.Err != "" {
			rep.Note = strings.TrimSpace(rep.Note + " error: " + st.Err)
		}
		if rep.Path == nil {
			rep.Path = []OptStep{}
		}
		if cons.satisfied(m, finals[i]) {
			rep.Refined, rep.RefinedPoints, rep.RefinedDPS = m.encode(finals[i]), m.treeSummary(finals[i]), round2(fin.DPS)
			if fin.Err == "" && m.validateEngine(finals[i], v, race.race, b.mode) == nil && (best < 0 || fin.DPS > evals[2*best+1].DPS) {
				best = i
			}
		}
		if i == 0 && res.Baseline.Kind != "none" && res.Baseline.Error == "" {
			res.Baseline.DPS, res.Baseline.StdErr = round2(st.DPS), round2(st.StdErr)
			if st.Err != "" {
				res.Baseline.Error = st.Err
			}
		}
		res.Starts = append(res.Starts, rep)
	}
	res.Sims, res.BudgetExhausted = o.sims, o.budgetHit
	res.WallSeconds = math.Round(time.Since(start).Seconds()*10) / 10
	if best < 0 {
		res.Error = "no start produced a legal build satisfying the constraints"
		return res, nil
	}
	fin := evals[2*best+1]
	res.Chosen = starts[best].name
	res.Talents, res.TreePoints, res.Points = m.encode(finals[best]), m.treeSummary(finals[best]), m.total(finals[best])
	res.DPS, res.StdErr = round2(fin.DPS), round2(fin.StdErr)
	res.CI95 = [2]float64{round2(fin.DPS - 1.96*fin.StdErr), round2(fin.DPS + 1.96*fin.StdErr)}
	res.AutoRotationAdded = fin.Added
	if res.Baseline.DPS > 0 {
		g := round2((fin.DPS - res.Baseline.DPS) / res.Baseline.DPS * 100)
		res.GainVsBaselinePct = &g
	}
	res.Sims = o.sims
	res.WallSeconds = math.Round(time.Since(start).Seconds()*10) / 10
	return res, nil
}

func (r OptimizeResult) build(b *bench, date string) TalentBuild {
	ci := r.CI95
	normJSON, _ := json.Marshal(b.norm)
	return TalentBuild{Spec: r.Spec, Name: fmt.Sprintf("optimized %s %s (%s)", r.Game, r.Phase, r.TreePoints), Talents: r.Talents,
		Source: "optimize-talents", Game: r.Game, Phase: r.Phase, Date: date,
		RulesetID: map[string]string{"forever": foreverdata.RulesetID, "classic": game.ClassicRulesetID}[r.Game], ManifestSHA256: foreverdata.ManifestSHA256(),
		PresetSnapshotSHA256: presetSnapshotSHA(), NormalizationHash: r.NormalizationHash, Normalization: normJSON,
		Targets: r.Targets, DurationS: r.DurationS, Seed: r.Seed, Iterations: r.Iterations, DPS: r.DPS, StdErr: r.StdErr, CI95: &ci,
		Baseline: r.Baseline, Points: r.Points, Sims: r.Sims, WallSeconds: r.WallSeconds, BudgetExhausted: r.BudgetExhausted}
}

// ---------------------------------------------------------------- commands

const optimizerSeed = 7

func optimizerFlags(name string, stderr io.Writer) (*flag.FlagSet, *commonFlags, *float64) {
	fs, c := newFlagSet(name, stderr)
	budget := fs.Float64("budget-seconds", 900, "wall-clock budget per spec/game search (0 = none); a search cut short is reported as budget_exhausted and is no longer deterministic")
	return fs, c, budget
}

// applyOptimizerDefaults: -iterations is the screening level (default 200,
// escalated to 5x and 15x), -seed defaults to 7 (not the ranking seed), and
// -parallel defaults to all CPUs.
func applyOptimizerDefaults(c *commonFlags, parallelDefault int) {
	if !c.set["iterations"] {
		c.iterations = 200
	}
	if !c.set["seed"] {
		c.seed = optimizerSeed
	}
	if !c.set["parallel"] {
		c.parallel = parallelDefault
	}
}

func cmdOptimizeTalents(args []string, stderr io.Writer) (interface{}, error) {
	fs, c, budget := optimizerFlags("optimize-talents", stderr)
	specID := fs.String("spec", "", "spec id from `list`")
	gameName := fs.String("game", "forever", "forever or classic")
	outFile := fs.String("out", "", `write the build as a builds file ({"builds":[...]}, usable with -forever-builds / -classic-builds)`)
	if err := c.parse(fs, args); err != nil {
		return nil, err
	}
	applyOptimizerDefaults(c, runtime.NumCPU())
	if *specID == "" {
		return nil, fmt.Errorf("-spec is required")
	}
	if *gameName != "forever" && *gameName != "classic" {
		return nil, fmt.Errorf("-game must be forever or classic")
	}
	b, root, err := c.newBench()
	if err != nil {
		return nil, err
	}
	res, err := b.optimize(*specID, optimizeOptions{game: *gameName, phase: c.phaseValue, base: int32(c.iterations),
		enc: EncounterSpec{Name: "custom", Weight: 1, Targets: c.targets, Duration: c.duration}, budget: time.Duration(*budget * float64(time.Second)), parallel: c.parallel})
	if err != nil {
		return nil, err
	}
	if *outFile != "" && res.Error == "" {
		data, _ := json.MarshalIndent(buildsFile{Note: "written by foreverbench optimize-talents", Builds: []TalentBuild{res.build(b, time.Now().Format("2006-01-02"))}}, "", " ")
		if err := os.WriteFile(*outFile, append(data, '\n'), 0o644); err != nil {
			return nil, err
		}
	}
	return map[string]interface{}{
		"command":       "optimize-talents",
		"result":        res,
		"normalization": b.norm,
		"provenance":    provenanceBlock(root, nil),
		"notes": []string{
			"Evaluations use the rank normalization with seed " + fmt.Sprint(b.norm.Seed) + " (common random numbers); the ranking seed differs so the build is not tuned to the ranking's random stream.",
			"iterations: candidates are screened at the first level; close candidates are re-simmed at the higher levels before a move is accepted.",
			"Forever builds pass foreverdata.Validate; Classic builds follow the UI talent tree rules (row gates of 5 points, prerequisites at max rank, 51 points).",
		},
	}, nil
}

type optimizeAllEntry struct {
	Spec              string     `json:"spec"`
	Game              string     `json:"game"`
	Talents           string     `json:"talents,omitempty"`
	TreePoints        string     `json:"tree_points,omitempty"`
	DPS               float64    `json:"dps,omitempty"`
	CI95              [2]float64 `json:"ci95,omitempty"`
	Baseline          string     `json:"baseline_kind,omitempty"`
	BaselineDPS       float64    `json:"baseline_dps,omitempty"`
	GainVsBaselinePct *float64   `json:"gain_vs_baseline_pct,omitempty"`
	WallSeconds       float64    `json:"wall_seconds"`
	Sims              int64      `json:"sims"`
	BudgetExhausted   bool       `json:"budget_exhausted,omitempty"`
	Written           bool       `json:"written"`
	Error             string     `json:"error,omitempty"`
	result            OptimizeResult
}

func cmdOptimizeAll(args []string, stderr io.Writer) (interface{}, error) {
	start := time.Now()
	fs, c, budget := optimizerFlags("optimize-all", stderr)
	games := fs.String("game", "forever,classic", "comma-separated: forever, classic")
	classicScope := fs.String("classic", "missing", "Classic specs to optimize: missing (no UI talent preset; written to classic_builds.json) or all (UI-preset specs are optimized for comparison but not written)")
	outDir := fs.String("out-dir", "", "directory for forever_builds.json / classic_builds.json (default <root>/cmd/foreverbench/presets)")
	innerParallel := fs.Int("inner-parallel", 0, "sims per spec search (default NumCPU / -parallel)")
	details := fs.Bool("details", false, "include the full search report (paths) per spec")
	if err := c.parse(fs, args); err != nil {
		return nil, err
	}
	if !c.set["parallel"] {
		c.parallel = 2
	}
	specParallel := c.parallel
	applyOptimizerDefaults(c, specParallel)
	inner := *innerParallel
	if inner <= 0 {
		inner = max(1, runtime.NumCPU()/specParallel)
	}
	wantForever, wantClassic := false, false
	for _, g := range strings.Split(*games, ",") {
		switch strings.TrimSpace(g) {
		case "forever":
			wantForever = true
		case "classic":
			wantClassic = true
		case "":
		default:
			return nil, fmt.Errorf("-game: unknown game %q", g)
		}
	}
	if *classicScope != "missing" && *classicScope != "all" {
		return nil, fmt.Errorf("-classic must be missing or all")
	}
	b, root, err := c.newBench()
	if err != nil {
		return nil, err
	}
	dir := *outDir
	if dir == "" {
		dir = filepath.Join(root, "cmd", "foreverbench", "presets")
	}
	entries, excluded, err := selectEntries(c.phaseValue, c.role, c.specs, c.gearFill)
	if err != nil {
		return nil, err
	}
	enc := EncounterSpec{Name: "custom", Weight: 1, Targets: c.targets, Duration: c.duration}
	date := time.Now().Format("2006-01-02")
	run := func(gameName string, list []PresetEntry) []optimizeAllEntry {
		out := make([]optimizeAllEntry, len(list))
		jobs := []func(){}
		for i, e := range list {
			i, e := i, e
			jobs = append(jobs, func() {
				fmt.Fprintf(stderr, "optimize %s %s ...\n", gameName, e.Spec)
				r, err := b.optimize(e.Spec, optimizeOptions{game: gameName, phase: c.phaseValue, base: int32(c.iterations), enc: enc,
					budget: time.Duration(*budget * float64(time.Second)), parallel: inner})
				o := optimizeAllEntry{Spec: e.Spec, Game: gameName, result: r}
				if err != nil {
					o.Error = err.Error()
				} else {
					o.Talents, o.TreePoints, o.DPS, o.CI95, o.WallSeconds, o.Sims, o.BudgetExhausted, o.Error = r.Talents, r.TreePoints, r.DPS, r.CI95, r.WallSeconds, r.Sims, r.BudgetExhausted, r.Error
					if r.Baseline != nil {
						o.Baseline, o.BaselineDPS, o.GainVsBaselinePct = r.Baseline.Kind, r.Baseline.DPS, r.GainVsBaselinePct
					}
				}
				fmt.Fprintf(stderr, "optimize %s %s: %.1f dps in %.0fs %s\n", gameName, e.Spec, o.DPS, o.WallSeconds, o.Error)
				out[i] = o
			})
		}
		runJobs(jobs, specParallel)
		return out
	}
	summary := []optimizeAllEntry{}
	written := map[string]int{}
	if wantClassic {
		list := []PresetEntry{}
		for _, e := range entries {
			if *classicScope == "all" || e.variant.Talents == "" {
				list = append(list, e)
			}
		}
		results := run("classic", list)
		newBuilds := []TalentBuild{}
		for i := range results {
			if results[i].Error == "" && list[i].variant.Talents == "" {
				nb := results[i].result.build(b, date)
				newBuilds = append(newBuilds, nb)
				results[i].Written = true
			}
		}
		if len(newBuilds) > 0 {
			if err := mergeBuildsFile(filepath.Join(dir, "classic_builds.json"), classicBuildsNote, newBuilds); err != nil {
				return nil, err
			}
			b.classic = append(b.classic, newBuilds...)
			written["classic_builds.json"] = len(newBuilds)
		}
		summary = append(summary, results...)
	}
	if wantForever {
		results := run("forever", entries)
		newBuilds := []TalentBuild{}
		for i := range results {
			if results[i].Error == "" {
				newBuilds = append(newBuilds, results[i].result.build(b, date))
				results[i].Written = true
			}
		}
		if len(newBuilds) > 0 {
			if err := mergeBuildsFile(filepath.Join(dir, "forever_builds.json"), foreverBuildsNote, newBuilds); err != nil {
				return nil, err
			}
			written["forever_builds.json"] = len(newBuilds)
		}
		summary = append(summary, results...)
	}
	out := map[string]interface{}{
		"command":       "optimize-all",
		"phase":         c.phaseValue.String(),
		"summary":       summary,
		"written":       written,
		"out_dir":       dir,
		"excluded":      excludedList(excluded),
		"normalization": b.norm,
		"provenance":    provenanceBlock(root, nil),
		"notes": []string{
			"The binary embeds presets/*_builds.json: rebuild it (go build -tags=with_db ./cmd/foreverbench) or pass -forever-builds / -classic-builds for rank, delta and meta to use new builds.",
		},
		"elapsed_ms": time.Since(start).Milliseconds(),
	}
	if *details {
		full := []OptimizeResult{}
		for _, s := range summary {
			full = append(full, s.result)
		}
		out["details"] = full
	}
	return out, nil
}

const foreverBuildsNote = "Forever talent builds (F1:<CLASS>:tree-tree-tree strings) per spec and phase. Entries with source optimize-talents are written by `foreverbench optimize-all` (deterministic search under the rank normalization; provenance: ruleset/manifest, preset snapshot, normalization hash, seed, iterations, DPS, baseline repair DPS, date). rank/delta/meta use the build for the ranked phase (else the nearest phase) and report talent_source optimized:<date>; the Classic-preset repair is only the fallback. Hand-curated builds need only spec, name, talents, source (optionally phase)."

const classicBuildsNote = "Classic talent builds for specs without a UI talent preset, written by `foreverbench optimize-all -game classic` (same search and provenance as forever_builds.json). Specs with a UI talent preset always use the UI preset on the Classic side."

// mergeBuildsFile replaces entries with the same spec, phase and source in path
// (creating it if needed) and keeps every other entry, sorted by spec order and phase.
func mergeBuildsFile(path, note string, add []TalentBuild) error {
	file := buildsFile{Note: note}
	if raw, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(raw, &file); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		file.Note = note
	}
	keep := []TalentBuild{}
	for _, old := range file.Builds {
		replaced := false
		for _, nb := range add {
			replaced = replaced || (old.Spec == nb.Spec && old.Phase == nb.Phase && old.optimized() && nb.optimized())
		}
		if !replaced {
			keep = append(keep, old)
		}
	}
	file.Builds = append(keep, add...)
	order := map[string]int{}
	for i, v := range variants {
		order[v.ID] = i
	}
	sort.SliceStable(file.Builds, func(i, j int) bool {
		a, b := file.Builds[i], file.Builds[j]
		if order[a.Spec] != order[b.Spec] {
			return order[a.Spec] < order[b.Spec]
		}
		return a.Phase < b.Phase
	})
	data, err := json.MarshalIndent(file, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
