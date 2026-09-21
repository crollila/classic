package optimizer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/wowsims/classic/assets/database"
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
	"google.golang.org/protobuf/encoding/protojson"
)

// Data is everything the optimizer reads besides the engine: the Forever talent records and
// mechanics (ui/forever/data/talents.json, checked against the compiled ruleset), the popular
// builds (ui/app/data/presets.json), the item and enchant database the engine embeds, the mob
// armor table and the APL files of the app's rotations.
type Data struct {
	Root       string
	RulesetID  string
	Manifest   string
	Records    []TalentRecord
	Mechanics  []mechanic
	RaceClass  map[string][]string
	Presets    map[string][]Preset
	Items      []*proto.UIItem
	ItemByID   map[int32]*proto.UIItem
	Enchants   []*proto.UIEnchant
	MobArmor   map[string]float64
	DBHash     string // sha256 of the isolated Forever item/enchant database (forever.bin)
	rotations  map[string]*proto.APLRotation
	rotationMu sync.Mutex
}

// TalentRecord is one Forever talent node.
type TalentRecord struct {
	ID             string `json:"id"`
	Class          string `json:"class"`
	Tree           string `json:"tree"`
	Name           string `json:"name"`
	MaxRank        int32  `json:"max_rank"`
	Row            int32  `json:"row"`
	Column         int32  `json:"column"`
	RequiredPoints int32  `json:"required_points"`
	Prerequisites  []struct {
		ID   string `json:"id"`
		Rank int32  `json:"rank"`
	} `json:"prerequisites"`
	Mode string `json:"mode"`
}

type mechanic struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	Kind     string `json:"kind"`
	Mode     string `json:"mode"`
}

// Preset is a popular talent build from ui/app/data/presets.json.
type Preset struct {
	Label   string           `json:"label"`
	Talents map[string]int32 `json:"talents"`
	order   []string
}

// UnmarshalJSON keeps the key order of the build: the app fills a truncated build in its
// own order, and so does the optimizer.
func (p *Preset) UnmarshalJSON(raw []byte) error {
	var head struct {
		Label   string          `json:"label"`
		Talents json.RawMessage `json:"talents"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return err
	}
	p.Label = head.Label
	p.Talents = map[string]int32{}
	dec := json.NewDecoder(strings.NewReader(string(head.Talents)))
	if _, err := dec.Token(); err != nil {
		return err
	}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return err
		}
		var n int32
		if err := dec.Decode(&n); err != nil {
			return err
		}
		id := t.(string)
		p.Talents[id] = n
		p.order = append(p.order, id)
	}
	return nil
}

var raceKeys = map[proto.Race]string{
	proto.Race_RaceDwarf: "dwarf", proto.Race_RaceGnome: "gnome", proto.Race_RaceHuman: "human",
	proto.Race_RaceNightElf: "night-elf", proto.Race_RaceOrc: "orc", proto.Race_RaceTauren: "tauren",
	proto.Race_RaceTroll: "troll", proto.Race_RaceUndead: "undead",
	proto.Race_RaceSkyborneWindshaper: "skyborne-windshaper", proto.Race_RaceSkyborneHighOrder: "skyborne-high-order",
}

// FindRoot walks up from dir to the sim repository root (the directory holding go.mod and ui/).
func FindRoot(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for d := abs; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(d, "ui", "forever", "data", "talents.json")); err == nil {
				return d, nil
			}
		}
		if filepath.Dir(d) == d {
			return "", fmt.Errorf("no sim repository root (go.mod + ui/forever/data/talents.json) above %s", abs)
		}
	}
}

var (
	dataOnce  sync.Once
	dataCache *Data
	dataErr   error
)

// LoadData reads the optimizer's data once per process (root "" = found from the working directory).
func LoadData(root string) (*Data, error) {
	dataOnce.Do(func() { dataCache, dataErr = loadData(root) })
	if dataErr == nil && root != "" {
		want, _ := filepath.Abs(root)
		if want != dataCache.Root {
			return loadData(root)
		}
	}
	return dataCache, dataErr
}

func loadData(root string) (*Data, error) {
	if root == "" {
		wd, _ := os.Getwd()
		r, err := FindRoot(wd)
		if err != nil {
			return nil, err
		}
		root = r
	}
	root, _ = filepath.Abs(root)
	d := &Data{Root: root, rotations: map[string]*proto.APLRotation{}}

	var tf struct {
		RulesetID      string              `json:"ruleset_id"`
		ManifestSHA256 string              `json:"manifest_sha256"`
		RaceClasses    map[string][]string `json:"race_classes"`
		Records        []TalentRecord      `json:"records"`
		Mechanics      []mechanic          `json:"mechanics"`
	}
	if err := readJSON(filepath.Join(root, "ui", "forever", "data", "talents.json"), &tf); err != nil {
		return nil, err
	}
	if tf.RulesetID != foreverdata.RulesetID || tf.ManifestSHA256 != foreverdata.ManifestSHA256() {
		return nil, fmt.Errorf("ui/forever/data/talents.json (ruleset %s) does not match the compiled engine (ruleset %s)", tf.RulesetID, foreverdata.RulesetID)
	}
	d.RulesetID, d.Manifest, d.RaceClass, d.Records, d.Mechanics = tf.RulesetID, tf.ManifestSHA256, tf.RaceClasses, tf.Records, tf.Mechanics

	if err := readJSON(filepath.Join(root, "ui", "app", "data", "presets.json"), &d.Presets); err != nil {
		return nil, err
	}
	var armor struct {
		Armor map[string]float64 `json:"armor"`
	}
	if err := readJSON(filepath.Join(root, "ui", "app", "data", "mob-armor.json"), &armor); err != nil {
		return nil, err
	}
	d.MobArmor = armor.Armor

	db := database.LoadForever()
	d.Items, d.Enchants = db.Items, db.Enchants
	d.ItemByID = make(map[int32]*proto.UIItem, len(db.Items))
	for _, it := range db.Items {
		d.ItemByID[it.Id] = it
	}
	if raw, err := os.ReadFile(filepath.Join(root, "assets", "database", "forever.bin")); err == nil {
		sum := sha256.Sum256(raw)
		d.DBHash = hex.EncodeToString(sum[:8])
	}
	if !core.WITH_DB {
		return nil, fmt.Errorf("the engine was built without its item database: build and test with -tags=with_db")
	}
	return d, nil
}

func readJSON(path string, v interface{}) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// ClassRecords are the talent nodes of a class, in data order.
func (d *Data) ClassRecords(c proto.Class) []TalentRecord {
	name := foreverdata.ClassName(c)
	out := []TalentRecord{}
	for _, r := range d.Records {
		if r.Class == name {
			out = append(out, r)
		}
	}
	return out
}

// Races are the races the Forever data allows for a class, in proto order.
func (d *Data) Races(c proto.Class) []proto.Race {
	name := foreverdata.ClassName(c)
	out := []proto.Race{}
	for race, key := range raceKeys {
		for _, cls := range d.RaceClass[key] {
			if cls == name {
				out = append(out, race)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Mechanics are the Forever mechanics the app enables: the race's racials and the class's abilities.
func (d *Data) mechanicsFor(c proto.Class, race proto.Race) []string {
	cls, key := foreverdata.ClassName(c), raceKeys[race]
	out := []string{}
	for _, m := range d.Mechanics {
		if m.Mode == "blocked" || m.Mode == "non-sim" {
			continue
		}
		if (m.Kind == "racial" && strings.HasPrefix(m.ID, "racials."+key+".")) || (m.Category == cls && m.Mode == "ability") {
			out = append(out, m.ID)
		}
	}
	return out
}

// Rotation loads (once) an APL file of the app.
func (d *Data) Rotation(path string) (*proto.APLRotation, error) {
	d.rotationMu.Lock()
	defer d.rotationMu.Unlock()
	if r, ok := d.rotations[path]; ok {
		return r, nil
	}
	raw, err := os.ReadFile(filepath.Join(d.Root, "ui", filepath.FromSlash(path)+".apl.json"))
	if err != nil {
		return nil, err
	}
	r := &proto.APLRotation{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(raw, r); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	d.rotations[path] = r
	return r, nil
}

func (d *Data) targetArmor(targetLevel int32) float64 {
	if targetLevel > 60 {
		return 3731
	}
	return d.MobArmor[fmt.Sprint(targetLevel)]
}
