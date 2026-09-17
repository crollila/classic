package main

// Go port of the Forever UI's request preparation (ui/forever/discovery.ts and
// ui/forever/rotation.ts), so CLI results match what the Forever web UI sims:
//   - foreverDefaultOptions: BEST_GUESS, racial + class-ability mechanics
//   - migrateClassicTalents: semantic Classic -> Forever talent migration
//   - decodeTalents: "F1:<CLASS>:a-b-c" Forever talent strings
//   - withForeverRotation: auto-prepends registered new Forever actions (DPS part)

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
	"google.golang.org/protobuf/encoding/protojson"
	googleProto "google.golang.org/protobuf/proto"
)

const foreverDataRelPath = "ui/forever/data/talents.json"

type fvAdapter struct {
	AutoPriority           *float64        `json:"auto_priority"`
	AutoCondition          json.RawMessage `json:"auto_condition"`
	AutoTarget             json.RawMessage `json:"auto_target"`
	Healing                bool            `json:"healing"`
	HybridHealing          bool            `json:"hybrid_healing"`
	AlternateHealingAction bool            `json:"alternate_healing_action"`
}

type fvRecord struct {
	ID             string `json:"id"`
	Class          string `json:"class"`
	Tree           string `json:"tree"`
	Name           string `json:"name"`
	MaxRank        int32  `json:"max_rank"`
	Row            int32  `json:"row"`
	RequiredPoints int32  `json:"required_points"`
	Prerequisites  []struct {
		ID   string `json:"id"`
		Rank int32  `json:"rank"`
	} `json:"prerequisites"`
	ClassicField string     `json:"classic_field"`
	Mode         string     `json:"mode"`
	Confidence   string     `json:"confidence"`
	ActionTag    int32      `json:"action_tag"`
	Adapter      *fvAdapter `json:"adapter"`
}

type fvMechanic struct {
	ID        string     `json:"id"`
	Category  string     `json:"category"`
	Kind      string     `json:"kind"`
	Name      string     `json:"name"`
	Mode      string     `json:"mode"`
	ActionTag int32      `json:"action_tag"`
	Adapter   *fvAdapter `json:"adapter"`
}

type fvDataset struct {
	RulesetID      string                `json:"ruleset_id"`
	ManifestSHA256 string                `json:"manifest_sha256"`
	ClassicLayout  map[string][][]string `json:"classic_layout"`
	RaceClasses    map[string][]string   `json:"race_classes"`
	Records        []fvRecord            `json:"records"`
	Mechanics      []fvMechanic          `json:"mechanics"`
}

var foreverRaceKeys = map[proto.Race]string{
	proto.Race_RaceDwarf: "dwarf", proto.Race_RaceGnome: "gnome", proto.Race_RaceHuman: "human",
	proto.Race_RaceNightElf: "night-elf", proto.Race_RaceOrc: "orc", proto.Race_RaceTauren: "tauren",
	proto.Race_RaceTroll: "troll", proto.Race_RaceUndead: "undead",
	proto.Race_RaceSkyborneWindshaper: "skyborne-windshaper", proto.Race_RaceSkyborneHighOrder: "skyborne-high-order",
}

func loadForeverData(root string) (*fvDataset, error) {
	path := filepath.Join(root, foreverDataRelPath)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("Forever data %s: %w (ship ui/forever/data/talents.json or pass -root)", path, err)
	}
	d := &fvDataset{}
	if err := json.Unmarshal(raw, d); err != nil {
		return nil, fmt.Errorf("Forever data %s: %w", path, err)
	}
	if d.RulesetID != foreverdata.RulesetID || d.ManifestSHA256 != foreverdata.ManifestSHA256() {
		return nil, fmt.Errorf("Forever data %s (ruleset %s, manifest %s) does not match the compiled engine (ruleset %s, manifest %s); rebuild the binary or ship the matching file",
			path, d.RulesetID, d.ManifestSHA256, foreverdata.RulesetID, foreverdata.ManifestSHA256())
	}
	return d, nil
}

func isExecutable(mode string) bool { return mode != "blocked" && mode != "non-sim" }

func (d *fvDataset) classRecords(c proto.Class) []fvRecord {
	name := foreverdata.ClassName(c)
	out := []fvRecord{}
	for _, r := range d.Records {
		if r.Class == name {
			out = append(out, r)
		}
	}
	return out
}

func (d *fvDataset) defaultOptions(c proto.Class, race proto.Race, mode proto.ForeverMode) *proto.ForeverOptions {
	raceKey := foreverRaceKeys[race]
	className := foreverdata.ClassName(c)
	mechanics := []string{}
	for _, m := range d.Mechanics {
		if !isExecutable(m.Mode) {
			continue
		}
		if (m.Kind == "racial" && strings.HasPrefix(m.ID, "racials."+raceKey+".")) || (m.Category == className && m.Mode == "ability") {
			mechanics = append(mechanics, m.ID)
		}
	}
	return &proto.ForeverOptions{RulesetId: d.RulesetID, Mode: mode, Mechanics: mechanics, Talents: map[string]int32{}}
}

// migrateClassicTalents keeps talents whose classic_field survives in Forever and
// prunes nodes whose row gate or prerequisites no longer hold (same as the UI).
func (d *fvDataset) migrateClassicTalents(c proto.Class, text string) map[string]int32 {
	className := foreverdata.ClassName(c)
	layout := d.ClassicLayout[className]
	oldFields := map[string]int32{}
	for i, tree := range strings.Split(text, "-") {
		for j, ch := range tree {
			if i < len(layout) && j < len(layout[i]) && ch >= '0' && ch <= '9' {
				oldFields[layout[i][j]] = int32(ch - '0')
			}
		}
	}
	records := d.classRecords(c)
	talents := map[string]int32{}
	for _, r := range records {
		if r.ClassicField != "" && oldFields[r.ClassicField] > 0 {
			talents[r.ID] = min(r.MaxRank, oldFields[r.ClassicField])
		}
	}
	for changed := true; changed; {
		changed = false
		for _, r := range records {
			if talents[r.ID] == 0 {
				continue
			}
			lower := int32(0)
			for _, t := range records {
				if t.Tree == r.Tree && t.Row < r.Row {
					lower += talents[t.ID]
				}
			}
			bad := lower < r.RequiredPoints
			for _, p := range r.Prerequisites {
				if talents[p.ID] < p.Rank {
					bad = true
				}
			}
			if bad {
				delete(talents, r.ID)
				changed = true
			}
		}
	}
	return talents
}

func (d *fvDataset) treeNames(c proto.Class) []string {
	names := []string{}
	for _, r := range d.classRecords(c) {
		found := false
		for _, n := range names {
			found = found || n == r.Tree
		}
		if !found {
			names = append(names, r.Tree)
		}
	}
	return names
}

func (d *fvDataset) decodeTalents(c proto.Class, text string) (map[string]int32, error) {
	prefix := "F1:" + foreverdata.ClassName(c) + ":"
	if !strings.HasPrefix(text, prefix) {
		return nil, fmt.Errorf("use a Forever F1 talent string (%s...) for this class; Classic strings use a different layout", prefix)
	}
	parts := strings.Split(strings.TrimPrefix(text, prefix), "-")
	names := d.treeNames(c)
	if len(parts) != len(names) {
		return nil, fmt.Errorf("a Forever build must contain all %d trees", len(names))
	}
	talents := map[string]int32{}
	records := d.classRecords(c)
	for i, tree := range names {
		inTree := []fvRecord{}
		for _, r := range records {
			if r.Tree == tree {
				inTree = append(inTree, r)
			}
		}
		if len(parts[i]) != len(inTree) {
			return nil, fmt.Errorf("tree %s: expected %d digits, got %d", tree, len(inTree), len(parts[i]))
		}
		for j, ch := range parts[i] {
			if ch < '0' || ch > '5' {
				return nil, fmt.Errorf("tree %s: invalid rank %q", tree, ch)
			}
			if ch != '0' {
				talents[inTree[j].ID] = int32(ch - '0')
			}
		}
	}
	return talents, nil
}

func (d *fvDataset) encodeTalents(c proto.Class, talents map[string]int32) string {
	records := d.classRecords(c)
	trees := []string{}
	for _, tree := range d.treeNames(c) {
		var b strings.Builder
		for _, r := range records {
			if r.Tree == tree {
				b.WriteByte(byte('0' + talents[r.ID]))
			}
		}
		trees = append(trees, b.String())
	}
	return "F1:" + foreverdata.ClassName(c) + ":" + strings.Join(trees, "-")
}

func sumPoints(talents map[string]int32) int32 {
	n := int32(0)
	for _, v := range talents {
		n += v
	}
	return n
}

func classicPoints(text string) int32 {
	n := int32(0)
	for _, ch := range text {
		if ch >= '0' && ch <= '9' {
			n += int32(ch - '0')
		}
	}
	return n
}

type autoAddition struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Tag      int32   `json:"action_tag"`
	Priority float64 `json:"auto_priority"`
}

// rotationCandidate is one APL item the Forever UI would prepend.
type rotationCandidate struct {
	autoAddition
	item *proto.APLListItem
}

// withForeverRotation mirrors ui/forever/rotation.ts for non-healer specs: every
// candidate is prepended (the web UI behaviour).
func (d *fvDataset) withForeverRotation(base *proto.APLRotation, f *proto.ForeverOptions, c proto.Class, spells []*proto.SpellStats, targetCount int) (*proto.APLRotation, []autoAddition, error) {
	cands, err := d.foreverRotationCandidates(f, c, spells, targetCount)
	if err != nil {
		return nil, nil, err
	}
	rot, added := applyRotationCandidates(base, cands)
	return rot, added, nil
}

// applyRotationCandidates prepends the candidates (in priority order) to base.
func applyRotationCandidates(base *proto.APLRotation, cands []rotationCandidate) (*proto.APLRotation, []autoAddition) {
	added := []autoAddition{}
	if len(cands) == 0 || base == nil {
		return base, added
	}
	out := googleProto.Clone(base).(*proto.APLRotation)
	items := []*proto.APLListItem{}
	for _, cand := range cands {
		items = append(items, googleProto.Clone(cand.item).(*proto.APLListItem))
		added = append(added, cand.autoAddition)
	}
	out.PriorityList = append(items, out.PriorityList...)
	return out, added
}

// foreverRotationCandidates lists, in UI priority order, the APL items the Forever
// UI auto-rotation would prepend for this player.
func (d *fvDataset) foreverRotationCandidates(f *proto.ForeverOptions, c proto.Class, spells []*proto.SpellStats, targetCount int) ([]rotationCandidate, error) {
	if f == nil {
		return nil, nil
	}
	if v, ok := f.Parameters["rotation.forever_auto"]; ok && v == 0 {
		return nil, nil
	}
	available := func(id int32) bool {
		for _, s := range spells {
			if s.IsCastable && s.GetId().GetSpellId() == id {
				return true
			}
		}
		return false
	}
	className := foreverdata.ClassName(c)
	type row struct {
		id, name string
		tag      int32
		adapter  *fvAdapter
	}
	rows := []row{}
	for _, r := range d.Records {
		if r.Class == className && foreverdata.EffectiveRank(f, r.ID) > 0 && r.Adapter != nil && r.Adapter.AutoPriority != nil {
			rows = append(rows, row{r.ID, r.Name, r.ActionTag, r.Adapter})
		}
	}
	for _, m := range d.Mechanics {
		if isExecutable(m.Mode) && m.Adapter != nil && m.Adapter.AutoPriority != nil {
			for _, id := range f.Mechanics {
				if id == m.ID {
					rows = append(rows, row{m.ID, m.Name, m.ActionTag, m.Adapter})
					break
				}
			}
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return *rows[i].adapter.AutoPriority < *rows[j].adapter.AutoPriority })

	out := []rotationCandidate{}
	parse := func(js string) (*proto.APLListItem, error) {
		item := &proto.APLListItem{}
		if err := protojson.Unmarshal([]byte(js), item); err != nil {
			return nil, fmt.Errorf("auto rotation item %s: %w", js, err)
		}
		return item, nil
	}
	if c == proto.Class_ClassMage && available(10212) {
		var blast, barrage *fvRecord
		for i := range d.Records {
			switch d.Records[i].ID {
			case "mage.talent.arcane-blast":
				blast = &d.Records[i]
			case "mage.talent.missile-barrage":
				barrage = &d.Records[i]
			}
		}
		if blast != nil && barrage != nil && foreverdata.EffectiveRank(f, blast.ID) > 0 {
			js := fmt.Sprintf(`{"action":{"condition":{"or":{"vals":[{"auraIsActive":{"auraId":{"otherId":19,"tag":%d}}},{"cmp":{"op":"OpGe","lhs":{"auraNumStacks":{"auraId":{"otherId":19,"tag":%d}}},"rhs":{"const":{"val":"4"}}}}]}},"channelSpell":{"spellId":{"spellId":10212}}}}`, barrage.ActionTag, blast.ActionTag)
			item, err := parse(js)
			if err != nil {
				return nil, err
			}
			out = append(out, rotationCandidate{autoAddition{ID: "spell:10212", Name: "Arcane Missiles (Arcane Blast / Missile Barrage condition)"}, item})
		}
	}
	for _, r := range rows {
		var target map[string]interface{}
		if len(r.adapter.AutoTarget) > 0 {
			if err := json.Unmarshal(r.adapter.AutoTarget, &target); err != nil {
				return nil, err
			}
			if t, _ := target["type"].(string); t == "Target" {
				if idx, _ := target["index"].(float64); int(idx) >= targetCount {
					continue
				}
			}
		}
		if r.adapter.Healing && !r.adapter.HybridHealing {
			continue
		}
		var spell *proto.SpellStats
		for _, s := range spells {
			if s.IsCastable && s.GetId().GetOtherId() == proto.OtherAction_OtherActionForever && s.GetId().GetTag() == r.tag {
				spell = s
				break
			}
		}
		if spell == nil {
			continue
		}
		kind := "castSpell"
		if spell.IsChanneled {
			kind = "channelSpell"
		}
		inner := map[string]interface{}{"spellId": map[string]interface{}{"otherId": 19, "tag": r.tag}}
		action := map[string]interface{}{kind: inner}
		if spell.HasDot && !spell.IsChanneled {
			action["condition"] = map[string]interface{}{"not": map[string]interface{}{"val": map[string]interface{}{"dotIsActive": map[string]interface{}{"spellId": map[string]interface{}{"otherId": 19, "tag": r.tag}}}}}
		}
		if len(r.adapter.AutoCondition) > 0 && string(r.adapter.AutoCondition) != "null" {
			action["condition"] = r.adapter.AutoCondition
		}
		if target != nil {
			inner["target"] = target
		}
		js, err := json.Marshal(map[string]interface{}{"action": action})
		if err != nil {
			return nil, err
		}
		item, err := parse(string(js))
		if err != nil {
			return nil, err
		}
		out = append(out, rotationCandidate{autoAddition{ID: r.id, Name: r.name, Tag: r.tag, Priority: *r.adapter.AutoPriority}, item})
	}
	return out, nil
}

func (d *fvDataset) actionName(tag int32) string {
	if tag < 0 {
		tag = -tag
	}
	for _, r := range d.Records {
		if r.ActionTag == tag && tag != 0 {
			return r.Name
		}
	}
	for _, m := range d.Mechanics {
		if m.ActionTag == tag && tag != 0 {
			return m.Name
		}
	}
	return ""
}

// ---------------------------------------------------------------- talent policies

// talentBuilder places points while respecting Forever row gates, prerequisites
// and the 51-point budget (same rules as foreverdata.Validate).
type talentBuilder struct {
	records []fvRecord
	byID    map[string]fvRecord
	chosen  map[string]int32
	filler  map[string]int32
}

func (d *fvDataset) newBuilder(c proto.Class) *talentBuilder {
	b := &talentBuilder{records: d.classRecords(c), byID: map[string]fvRecord{}, chosen: map[string]int32{}, filler: map[string]int32{}}
	for _, r := range b.records {
		b.byID[r.ID] = r
	}
	return b
}

func (b *talentBuilder) total() int32 { return sumPoints(b.chosen) }

func (b *talentBuilder) lower(r fvRecord) int32 {
	n := int32(0)
	for _, t := range b.records {
		if t.Tree == r.Tree && t.Row < r.Row {
			n += b.chosen[t.ID]
		}
	}
	return n
}

func (b *talentBuilder) available(r fvRecord) bool {
	for _, p := range r.Prerequisites {
		if b.chosen[p.ID] < p.Rank {
			return false
		}
	}
	return b.lower(r) >= r.RequiredPoints
}

// ensure mirrors sampleForeverBuild's ensure(): satisfy prerequisites, then fill
// the row gate one point at a time (executable nodes first, lowest row first).
func (b *talentBuilder) ensure(r fvRecord, n int32, depth int) {
	if depth > 10 {
		return
	}
	for _, p := range r.Prerequisites {
		if parent, ok := b.byID[p.ID]; ok && b.chosen[p.ID] < p.Rank {
			b.ensure(parent, p.Rank, depth+1)
		}
	}
	for guard := 0; guard < 51 && !b.available(r) && b.total() < 51; guard++ {
		cands := []fvRecord{}
		for _, x := range b.records {
			if x.Tree == r.Tree && x.Row < r.Row && b.chosen[x.ID] < x.MaxRank && b.available(x) {
				cands = append(cands, x)
			}
		}
		if len(cands) == 0 {
			break
		}
		sort.SliceStable(cands, func(i, j int) bool {
			ei, ej := isExecutable(cands[i].Mode), isExecutable(cands[j].Mode)
			if ei != ej {
				return ei
			}
			return cands[i].Row < cands[j].Row
		})
		b.chosen[cands[0].ID]++
		b.filler[cands[0].ID]++
	}
	if b.available(r) && b.chosen[r.ID] < n {
		b.chosen[r.ID] = min(n, b.chosen[r.ID]+51-b.total())
	}
}

func autoPriority(r fvRecord) float64 {
	if r.Adapter != nil && r.Adapter.AutoPriority != nil {
		return *r.Adapter.AutoPriority
	}
	return 999
}

// spendRemaining mirrors the final loop of sampleForeverBuild. With passiveFirst,
// nodes that the auto-rotation overlay would cast (adapter.auto_priority) are only
// used once no other node is available, so filler points do not add new actions.
func (b *talentBuilder) spendRemaining(tree string, passiveFirst bool) {
	for guard := 0; guard < 100 && b.total() < 51; guard++ {
		cands := []fvRecord{}
		for _, r := range b.records {
			if b.chosen[r.ID] < r.MaxRank && b.available(r) && isExecutable(r.Mode) {
				cands = append(cands, r)
			}
		}
		if len(cands) == 0 {
			return
		}
		sort.SliceStable(cands, func(i, j int) bool {
			if passiveFirst {
				ai, aj := autoPriority(cands[i]) != 999, autoPriority(cands[j]) != 999
				if ai != aj {
					return aj
				}
			}
			ti, tj := cands[i].Tree == tree, cands[j].Tree == tree
			if ti != tj {
				return ti
			}
			if pi, pj := autoPriority(cands[i]), autoPriority(cands[j]); pi != pj {
				return pi < pj
			}
			return cands[i].Row > cands[j].Row
		})
		b.chosen[cands[0].ID]++
		b.filler[cands[0].ID]++
	}
}

func (b *talentBuilder) primaryTree(talents map[string]int32) string {
	points := map[string]int32{}
	order := []string{}
	for _, r := range b.records {
		if _, ok := points[r.Tree]; !ok {
			order = append(order, r.Tree)
			points[r.Tree] = 0
		}
		points[r.Tree] += talents[r.ID]
	}
	best := order[0]
	for _, t := range order {
		if points[t] > points[best] {
			best = t
		}
	}
	return best
}

// repairClassicTalents keeps every Classic talent that still has a Forever node,
// placing them in row order and filling row gates, then spends leftover points
// with the UI sample-build ordering in the primary tree. Returns talents and
// the filler points that were not part of the Classic build.
func (d *fvDataset) repairClassicTalents(c proto.Class, text string, spendLeftover bool) (map[string]int32, map[string]int32) {
	b := d.newBuilder(c)
	layout := d.ClassicLayout[foreverdata.ClassName(c)]
	oldFields := map[string]int32{}
	classicTreePoints := map[int]int32{}
	for i, tree := range strings.Split(text, "-") {
		for j, ch := range tree {
			if i < len(layout) && j < len(layout[i]) && ch >= '0' && ch <= '9' {
				oldFields[layout[i][j]] = int32(ch - '0')
				classicTreePoints[i] += int32(ch - '0')
			}
		}
	}
	wanted := map[string]int32{}
	targets := []fvRecord{}
	for _, r := range b.records {
		if r.ClassicField != "" && oldFields[r.ClassicField] > 0 && isExecutable(r.Mode) {
			wanted[r.ID] = min(r.MaxRank, oldFields[r.ClassicField])
			targets = append(targets, r)
		}
	}
	sort.SliceStable(targets, func(i, j int) bool { return targets[i].Row < targets[j].Row })
	// Two passes: a later node may have raised a gate for an earlier one's tree.
	for pass := 0; pass < 2; pass++ {
		for _, r := range targets {
			if b.chosen[r.ID] < wanted[r.ID] && b.total() < 51 {
				b.ensure(r, wanted[r.ID], 0)
			}
		}
	}
	if spendLeftover {
		b.spendRemaining(b.primaryTree(b.chosen), true)
	}
	filler := map[string]int32{}
	for id, n := range b.chosen {
		if n == 0 {
			delete(b.chosen, id)
		} else if n > wanted[id] {
			filler[id] = n - wanted[id]
		}
	}
	return b.chosen, filler
}

// sampleBuild is the UI's sampleForeverBuild for a tree ("legal exploration build").
func (d *fvDataset) sampleBuild(c proto.Class, tree string) map[string]int32 {
	b := d.newBuilder(c)
	cands := []fvRecord{}
	for _, r := range b.records {
		if r.Tree == tree {
			cands = append(cands, r)
		}
	}
	if len(cands) > 0 {
		sort.SliceStable(cands, func(i, j int) bool {
			if cands[i].Row != cands[j].Row {
				return cands[i].Row > cands[j].Row
			}
			return autoPriority(cands[i]) < autoPriority(cands[j])
		})
		b.ensure(cands[0], cands[0].MaxRank, 0)
	}
	b.spendRemaining(tree, false)
	return b.chosen
}
