package optimizer

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/wowsims/classic/sim/core/proto"
)

// Slot indices (proto.ItemSlot order, as in the app).
const (
	slotFinger1  = 10
	slotFinger2  = 11
	slotTrinket1 = 12
	slotTrinket2 = 13
	slotMainHand = 14
	slotOffHand  = 15
	slotRanged   = 16
	numSlots     = 17
)

// slotTypes maps each slot to the item type it holds.
var slotTypes = [numSlots]proto.ItemType{
	proto.ItemType_ItemTypeHead, proto.ItemType_ItemTypeNeck, proto.ItemType_ItemTypeShoulder, proto.ItemType_ItemTypeBack,
	proto.ItemType_ItemTypeChest, proto.ItemType_ItemTypeWrist, proto.ItemType_ItemTypeHands, proto.ItemType_ItemTypeWaist,
	proto.ItemType_ItemTypeLegs, proto.ItemType_ItemTypeFeet, proto.ItemType_ItemTypeFinger, proto.ItemType_ItemTypeFinger,
	proto.ItemType_ItemTypeTrinket, proto.ItemType_ItemTypeTrinket, proto.ItemType_ItemTypeWeapon, proto.ItemType_ItemTypeWeapon,
	proto.ItemType_ItemTypeRanged,
}

var SlotNames = [numSlots]string{"Head", "Neck", "Shoulder", "Back", "Chest", "Wrist", "Hands", "Waist", "Legs", "Feet",
	"Ring 1", "Ring 2", "Trinket 1", "Trinket 2", "Main Hand", "Off Hand", "Ranged"}

// twinSlot is the other slot of a ring/trinket pair (-1 otherwise).
func twinSlot(slot int) int {
	switch slot {
	case slotFinger1:
		return slotFinger2
	case slotFinger2:
		return slotFinger1
	case slotTrinket1:
		return slotTrinket2
	case slotTrinket2:
		return slotTrinket1
	}
	return -1
}

var allianceRaces = []proto.Race{proto.Race_RaceHuman, proto.Race_RaceDwarf, proto.Race_RaceNightElf, proto.Race_RaceGnome}
var hordeRaces = []proto.Race{proto.Race_RaceOrc, proto.Race_RaceUndead, proto.Race_RaceTauren, proto.Race_RaceTroll}

func armorSlot(t proto.ItemType) bool {
	switch t {
	case proto.ItemType_ItemTypeHead, proto.ItemType_ItemTypeShoulder, proto.ItemType_ItemTypeChest, proto.ItemType_ItemTypeWrist,
		proto.ItemType_ItemTypeHands, proto.ItemType_ItemTypeWaist, proto.ItemType_ItemTypeLegs, proto.ItemType_ItemTypeFeet:
		return true
	}
	return false
}

// itemFits is the per-slot item rule of the app (ui/app/main.ts canUse) for the problem's
// character, ignoring what the other slots hold (see Problem.Validate for cross-slot rules):
// not hidden, the slot's item type, class allowlist, required level, quality filter, armor type
// by level, weapon/ranged proficiencies, two-handers only in the main hand, off-hand weapons
// only when the class dual wields at this level, shields and held-in-off-hand items by class.
func (p *Problem) itemFits(it *proto.UIItem, slot int, race proto.Race) bool {
	if it == nil || it.Hidden || it.Type != slotTypes[slot] {
		return false
	}
	// Some upstream tooltip rows assign a wearable slot to a profession recipe (for example
	// "Formula: Enchant Gloves ..."). The game cannot equip these even when the legacy database's
	// type field says Hands, so never admit them to the gear search.
	for _, prefix := range []string{"Formula:", "Pattern:", "Plans:", "Recipe:", "Schematic:", "Manual:"} {
		if strings.HasPrefix(it.Name, prefix) {
			return false
		}
	}
	if len(it.ClassAllowlist) > 0 && !slices.Contains(it.ClassAllowlist, p.Spec.Class) {
		return false
	}
	if !p.AboveLevel && it.RequiredLevel > p.Level {
		return false
	}
	// A zero requirement in the shipped table is often "not stated", especially for endgame
	// rewards. Below 60, do not claim an item above the character's level is legal without an
	// explicit requirement proving it.
	if !p.AboveLevel && p.Level < 60 && it.RequiredLevel == 0 && it.Ilvl > p.Level {
		return false
	}
	if len(p.Qualities) > 0 && !slices.Contains(p.Qualities, it.Quality) {
		return false
	}
	if it.RequiredProfession != proto.Profession_ProfessionUnknown && !p.Professions {
		return false
	}
	switch it.FactionRestriction {
	case proto.UIItem_FACTION_RESTRICTION_ALLIANCE_ONLY:
		if !slices.Contains(allianceRaces, race) {
			return false
		}
	case proto.UIItem_FACTION_RESTRICTION_HORDE_ONLY:
		if !slices.Contains(hordeRaces, race) {
			return false
		}
	}
	if armorSlot(it.Type) && it.ArmorType > p.Spec.Armor(p.Level) {
		return false
	}
	switch it.Type {
	case proto.ItemType_ItemTypeWeapon:
		wt, ht := it.WeaponType, it.HandType
		if slot == slotMainHand {
			if ht == proto.HandType_HandTypeOffHand || wt == proto.WeaponType_WeaponTypeShield || wt == proto.WeaponType_WeaponTypeOffHand {
				return false
			}
			if ht == proto.HandType_HandTypeTwoHand && !p.Spec.TwoHand {
				return false
			}
			return p.Spec.usesWeapon(wt)
		}
		if wt == proto.WeaponType_WeaponTypeShield {
			return p.Spec.Shield
		}
		if wt == proto.WeaponType_WeaponTypeOffHand {
			return p.Spec.usesWeapon(wt)
		}
		if ht == proto.HandType_HandTypeTwoHand {
			return false
		}
		return p.Spec.DualWield(p.Level) && (ht == proto.HandType_HandTypeOneHand || ht == proto.HandType_HandTypeOffHand) && p.Spec.usesWeapon(wt)
	case proto.ItemType_ItemTypeRanged:
		return p.Spec.usesRanged(it.RangedWeaponType)
	}
	return true
}

func isTwoHander(it *proto.UIItem) bool {
	return it != nil && it.HandType == proto.HandType_HandTypeTwoHand
}

// enchantFits ports ui/core/proto_utils/utils.ts enchantAppliesToItem + canEquipEnchant.
func (p *Problem) enchantFits(e *proto.UIEnchant, it *proto.UIItem, slot int) bool {
	if e == nil || it == nil {
		return false
	}
	if len(e.ClassAllowlist) > 0 && !slices.Contains(e.ClassAllowlist, p.Spec.Class) {
		return false
	}
	if e.RequiredProfession != proto.Profession_ProfessionUnknown && !p.Professions {
		return false
	}
	if e.Type != slotTypes[slot] && !slices.Contains(e.ExtraTypes, slotTypes[slot]) {
		return false
	}
	if e.Type != it.Type && !slices.Contains(e.ExtraTypes, it.Type) {
		return false
	}
	if e.EnchantType == proto.EnchantType_EnchantTypeTwoHand && it.HandType != proto.HandType_HandTypeTwoHand {
		return false
	}
	if (e.EnchantType == proto.EnchantType_EnchantTypeShield) != (it.WeaponType == proto.WeaponType_WeaponTypeShield) {
		return false
	}
	if e.EnchantType == proto.EnchantType_EnchantTypeStaff && it.WeaponType != proto.WeaponType_WeaponTypeStaff {
		return false
	}
	if it.WeaponType == proto.WeaponType_WeaponTypeOffHand {
		return false
	}
	if slot == slotRanged && !slices.Contains([]proto.RangedWeaponType{rBow, rCrossbow, rGun}, it.RangedWeaponType) {
		return false
	}
	return true
}

// PointsAllowed is the talent budget at the problem's level (level - 9, at most 51).
func (p *Problem) PointsAllowed() int32 { return max(0, min(51, p.Level-9)) }

// TalentError explains why a build is not legal at the problem's level ("" when it is):
// class records only, ranks within 0..max, the level's point budget, prerequisites at their
// rank, and each node's required points in the lower rows of its tree.
func (p *Problem) TalentError(t map[string]int32) string {
	total := int32(0)
	for id, n := range t {
		r, ok := p.recByID[id]
		if !ok {
			return fmt.Sprintf("%s is not a %s talent", id, p.Spec.Class)
		}
		if n < 0 || n > r.MaxRank {
			return fmt.Sprintf("%s rank %d outside 0..%d", id, n, r.MaxRank)
		}
		total += n
	}
	if total > p.PointsAllowed() {
		return fmt.Sprintf("%d talent points spent, level %d has %d", total, p.Level, p.PointsAllowed())
	}
	for id, n := range t {
		if n == 0 {
			continue
		}
		r := p.recByID[id]
		for _, pre := range r.Prerequisites {
			if t[pre.ID] < pre.Rank {
				return fmt.Sprintf("%s needs %s rank %d", id, pre.ID, pre.Rank)
			}
		}
		if r.RequiredPoints > 0 && p.pointsBelow(t, r) < r.RequiredPoints {
			return fmt.Sprintf("%s needs %d points in lower rows of %s", id, r.RequiredPoints, r.Tree)
		}
	}
	return ""
}

func (p *Problem) pointsBelow(t map[string]int32, r TalentRecord) int32 {
	below := int32(0)
	for other, pts := range t {
		o := p.recByID[other]
		if o.Tree == r.Tree && o.Row < r.Row {
			below += pts
		}
	}
	return below
}

// Validate returns why a configuration is not one the character can have (nil when it is legal).
// Every configuration the optimizer simulates or returns passes it.
func (p *Problem) Validate(c *Config) error {
	if !slices.Contains(p.races, c.Race) {
		return fmt.Errorf("race %s is not available to %s", c.Race, p.Spec.Label)
	}
	if c.Rotation < 0 || c.Rotation >= len(p.Spec.Rotations) {
		return fmt.Errorf("rotation %d out of range", c.Rotation)
	}
	for slot := 0; slot < numSlots; slot++ {
		id := c.Gear[slot]
		if id == 0 {
			if c.Enchants[slot] != 0 {
				return fmt.Errorf("%s: enchant on an empty slot", SlotNames[slot])
			}
			continue
		}
		it := p.Data.ItemByID[id]
		if !p.itemFits(it, slot, c.Race) {
			return fmt.Errorf("%s: item %d cannot be worn there by a level %d %s", SlotNames[slot], id, p.Level, p.Spec.Label)
		}
		if twin := twinSlot(slot); twin >= 0 && it.Unique && c.Gear[twin] == id {
			return fmt.Errorf("%s: unique item %d worn twice", SlotNames[slot], id)
		}
		if e := c.Enchants[slot]; e != 0 {
			if !p.enchantFits(p.enchantByID[e], it, slot) {
				return fmt.Errorf("%s: enchant %d does not apply to item %d", SlotNames[slot], e, id)
			}
		}
	}
	if isTwoHander(p.Data.ItemByID[c.Gear[slotMainHand]]) && c.Gear[slotOffHand] != 0 {
		return fmt.Errorf("two-handed main hand with an off hand")
	}
	if msg := p.TalentError(c.Talents); msg != "" {
		return fmt.Errorf("talents: %s", msg)
	}
	return nil
}

// sortedTalentIDs returns the build's ids in a stable order.
func sortedTalentIDs(t map[string]int32) []string {
	ids := make([]string, 0, len(t))
	for id, n := range t {
		if n > 0 {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}
