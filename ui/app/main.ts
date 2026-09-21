// Forever Sim: one page for every DPS spec, laid out like the classic WarriorSim.
// Character sheet and actions on the left, the paper doll across the top, and for the
// selected slot every Forever item the character can wear, ranked by simulated DPS.
import { ComputeStatsRequest, ForeverMode, ForeverOptions, Player, RaidSimRequest, RaidSimResult, RaidSimRequestSplitRequest, RaidSimResultCombinationRequest, StatWeightsRequest, UnitMetadata } from '../core/proto/api';
import { APLRotation } from '../core/proto/apl';
import { ActionID } from '../core/proto/common';
import { optimize, canonical } from '../forever/optimizer.mjs';
import { explainRotation } from '../forever/optimizer-explain.mjs';
import { learnedRotation } from './rotation-context.mjs';
import { runQueue } from './work-queue.mjs';
import {
	Consumes,
	Cooldowns,
	Debuffs,
	Encounter,
	EquipmentSpec,
	IndividualBuffs,
	ItemSpec,
	MobType,
	PartyBuffs,
	Race,
	RaidBuffs,
	Stat,
	Target,
	UnitReference,
} from '../core/proto/common';
import { Party, Raid } from '../core/proto/api';
import { raceNames } from '../core/proto_utils/names';
import { SimSignalManager, RequestTypes } from '../core/sim_signal_manager';
import { WorkerPool } from '../core/worker_pool';
import { foreverClassName, foreverDiscoveryTalents, foreverRaces, ForeverTalent } from '../forever/discovery';
import mobArmor from './data/mob-armor.json';
import spellNames from './data/spell-names.json';
import talentText from './data/talent-text.json';
import presetData from './data/presets.json';
import { Armor, rotation, SpecDef, SPECS } from './specs';
import './app.css';

// ---------------------------------------------------------------- data

interface Item {
	id: number; name: string; icon: string; type: number; armorType?: number; weaponType?: number; handType?: number;
	rangedWeaponType?: number; stats: number[]; ilvl: number; quality: number; requiredLevel?: number; hidden?: boolean;
	weaponDamageMin?: number; weaponDamageMax?: number; weaponSpeed?: number; classAllowlist?: number[];
	sources?: Array<{ drop?: { otherName?: string } }>; unique?: boolean;
}
type TalentInfo = { icon: string; desc: string[]; tree: string; row: number; col: number; max: number };
const TALENTS = (talentText as unknown as { talents: Record<string, Record<string, TalentInfo> & { __trees: Array<{ name: string; icon: string }> }> }).talents;
const NAMES = (spellNames as unknown as { names: Record<string, string> }).names;
const ARMOR = (mobArmor as unknown as { armor: Record<string, number> }).armor;
const PRESETS = presetData as unknown as Record<string, Array<{ label: string; talents: Record<string, number> }>>;
const ICON = (icon: string) => `https://wow.zamimg.com/images/wow/icons/medium/${icon || 'inv_misc_questionmark'}.jpg`;
const BASE = import.meta.env.BASE_URL;

// proto ItemSlot order; the WarriorSim strip runs armor left to right, then weapons.
const SLOTS = [
	{ key: 0, label: 'Head', type: 1, icon: 'inventoryslot_head' }, { key: 1, label: 'Neck', type: 2, icon: 'inventoryslot_neck' },
	{ key: 2, label: 'Shoulder', type: 3, icon: 'inventoryslot_shoulder' }, { key: 3, label: 'Back', type: 4, icon: 'inventoryslot_chest' },
	{ key: 4, label: 'Chest', type: 5, icon: 'inventoryslot_chest' }, { key: 5, label: 'Wrist', type: 6, icon: 'inventoryslot_wrists' },
	{ key: 6, label: 'Hands', type: 7, icon: 'inventoryslot_hands' }, { key: 7, label: 'Waist', type: 8, icon: 'inventoryslot_waist' },
	{ key: 8, label: 'Legs', type: 9, icon: 'inventoryslot_legs' }, { key: 9, label: 'Feet', type: 10, icon: 'inventoryslot_feet' },
	{ key: 10, label: 'Ring 1', type: 11, icon: 'inventoryslot_finger' }, { key: 11, label: 'Ring 2', type: 11, icon: 'inventoryslot_finger' },
	{ key: 12, label: 'Trinket 1', type: 12, icon: 'inventoryslot_trinket' }, { key: 13, label: 'Trinket 2', type: 12, icon: 'inventoryslot_trinket' },
	{ key: 14, label: 'Main Hand', type: 13, icon: 'inventoryslot_mainhand' }, { key: 15, label: 'Off Hand', type: 13, icon: 'inventoryslot_offhand' },
	{ key: 16, label: 'Ranged', type: 14, icon: 'inventoryslot_ranged' },
];
const QUALITY = ['poor', 'common', 'uncommon', 'rare', 'epic', 'legendary'];
const CLASS_BIT: Record<string, number> = { DRUID: 1, HUNTER: 2, MAGE: 3, PALADIN: 4, PRIEST: 5, ROGUE: 6, SHAMAN: 7, WARLOCK: 8, WARRIOR: 9 };

// ---------------------------------------------------------------- state

interface State {
	autoRotation: boolean;
	spec: string; level: number; race: Race; targetLevel: number; duration: number; iterations: number; itemIterations: number;
	rotation: number; gear: number[]; talents: Record<string, number>; qualities: number[]; aboveLevel: boolean; slot: number;
}
const STORE = 'forever-app-v1';
function defaults(spec: SpecDef): State {
	return {
		spec: spec.key, level: 20, race: foreverRaces(spec.cls)[0], targetLevel: 22, duration: 120, iterations: 1000, itemIterations: 100,
		autoRotation: false, rotation: 0, gear: Array(17).fill(0), talents: {}, qualities: [1, 2, 3, 4, 5], aboveLevel: false, slot: 14,
	};
}
function load(key: string): State | null {
	try { const all = JSON.parse(localStorage.getItem(STORE) || '{}'); return all[key] || null; } catch { return null; }
}
function save() {
	try { const all = JSON.parse(localStorage.getItem(STORE) || '{}'); all[state.spec] = state; all.last = state.spec; localStorage.setItem(STORE, JSON.stringify(all)); } catch { /* private mode */ }
}
function lastSpec(): string | null {
	const hash = location.hash.slice(1);
	if (SPECS.some(s => s.key === hash)) return hash;
	try { return JSON.parse(localStorage.getItem(STORE) || '{}').last || null; } catch { return null; }
}

let items: Item[] = [];
let byId = new Map<number, Item>();
let def = SPECS.find(s => s.key === lastSpec()) || SPECS[1];
let state: State = { ...defaults(def), ...(load(def.key) || {}) };
const pool = new WorkerPool(Math.max(1, Math.min(3, (navigator.hardwareConcurrency || 4) - 1)));
const itemSignals = new SimSignalManager();
const foregroundSignals = new SimSignalManager();
let foregroundBusy = false;
let setupRevision = 0;
let optimizerController: AbortController | undefined;
let optimized: { key: string; apl: APLRotation; result: Awaited<ReturnType<typeof optimize>> } | undefined;
const optimizerCache = new Map();
const setupKey = () => canonical([state.spec, state.level, state.race, state.targetLevel, state.duration, state.rotation, state.talents, state.gear]);
const activeRotation = () => optimized && optimized.key === setupKey() ? APLRotation.clone(optimized.apl) : rotation(def, state.rotation);
function stopItemWork() { ++runToken; void itemSignals.abortType(RequestTypes.All); }
let runToken = 0;

// ---------------------------------------------------------------- request building

function records(): ForeverTalent[] { return foreverDiscoveryTalents.records.filter(r => r.class === foreverClassName(def.cls)); }
function pointsAllowed() { return Math.max(0, Math.min(51, state.level - 9)); }
function pointsSpent(t = state.talents) { return Object.values(t).reduce((a, b) => a + b, 0); }

function foreverOptions(): ForeverOptions {
	const raceKey: Record<number, string> = { 1: 'dwarf', 2: 'gnome', 3: 'human', 4: 'night-elf', 5: 'orc', 6: 'tauren', 7: 'troll', 8: 'undead', 9: 'skyborne-windshaper', 10: 'skyborne-high-order' };
	const cls = foreverClassName(def.cls);
	return ForeverOptions.create({
		rulesetId: foreverDiscoveryTalents.ruleset_id, mode: ForeverMode.BEST_GUESS, talents: { ...state.talents },
		mechanics: foreverDiscoveryTalents.mechanics
			.filter(m => m.mode !== 'blocked' && m.mode !== 'non-sim' && ((m.kind === 'racial' && m.id.startsWith(`racials.${raceKey[state.race]}.`)) || (m.category === cls && m.mode === 'ability')))
			.map(m => m.id),
	});
}

function player(gear = state.gear): Player {
	const forever = foreverOptions();
	return Player.create({
		name: 'Forever', class: def.cls, race: state.race, level: state.level,
		equipment: EquipmentSpec.create({ items: gear.map(id => ItemSpec.create({ id })) }),
		consumes: Consumes.create(), buffs: IndividualBuffs.create(), cooldowns: Cooldowns.create(),
		talentsString: "", forever,
		rotation: activeRotation(), spec: def.spec(state.level),
		reactionTimeMs: 100, distanceFromTarget: def.role === 'melee' ? 5 : 25,
	});
}
function encounter(): Encounter {
	const stats = Array(Object.keys(Stat).length / 2).fill(0);
	stats[Stat.StatArmor] = state.targetLevel > 60 ? 3731 : ARMOR[String(state.targetLevel)] || 0;
	return Encounter.create({ duration: state.duration, durationVariation: 10, executeProportion20: 0.2, executeProportion35: 0.35,
		targets: [Target.create({ level: state.targetLevel, mobType: MobType.MobTypeHumanoid, stats })] });
}
function request(iterations: number, gear = state.gear): RaidSimRequest {
	return RaidSimRequest.create({
		raid: Raid.create({ parties: [Party.create({ players: [player(gear)] })], buffs: RaidBuffs.create(), debuffs: Debuffs.create() }),
		encounter: encounter(),
		simOptions: { iterations, randomSeed: BigInt(Math.floor(Math.random() * 2 ** 31)), debug: false, debugFirstIteration: false, isTest: false, saveAllValues: false, interactive: false, useLabeledRands: false },
	});
}
const dps = (r: RaidSimResult) => r.raidMetrics?.dps?.avg || 0;

// ---------------------------------------------------------------- item rules

function canUse(item: Item, slot: number): boolean {
	const slotDef = SLOTS[slot];
	if (item.hidden || item.type !== slotDef.type) return false;
	if (item.classAllowlist?.length && !item.classAllowlist.includes(CLASS_BIT[foreverClassName(def.cls)])) return false;
	if (!state.aboveLevel && (item.requiredLevel || 0) > state.level) return false;
	if (!state.qualities.includes(item.quality)) return false;
	if (item.type <= 10 && item.armorType && ![1, 3, 4, 11, 12].includes(slotDef.type) || item.type === 5 || item.type === 7 || item.type === 9 || item.type === 10 || item.type === 6 || item.type === 8) {
		if (item.armorType && item.armorType > def.armor(state.level) && item.type !== 4) return false;
	}
	if (item.type === 1 || item.type === 3) { if (item.armorType && item.armorType > def.armor(state.level)) return false; }
	if (item.type === 13) {
		const wt = item.weaponType || 0, ht = item.handType || 0;
		if (slot === 14) {
			if (ht === 3) return false;
			if (ht === 4 && !def.twoHand) return false;
			return def.weapons.includes(wt);
		}
		// Off hand: shields, held-in-off-hand, or a one-hander when the class dual wields.
		if (wt === 7) return def.shield && !twoHanded();
		if (wt === 5) return def.weapons.includes(5) && !twoHanded();
		if (ht === 4 || twoHanded()) return false;
		return def.dualWield(state.level) && (ht === 2 || ht === 3) && def.weapons.includes(wt);
	}
	if (item.type === 14) return def.ranged.includes(item.rangedWeaponType || 0);
	return true;
}
const twoHanded = () => byId.get(state.gear[14])?.handType === 4;
function candidates(slot: number) { return items.filter(i => canUse(i, slot)); }

// ---------------------------------------------------------------- rendering helpers

function el<K extends keyof HTMLElementTagNameMap>(tag: K, cls = '', text = ''): HTMLElementTagNameMap[K] {
	const e = document.createElement(tag); if (cls) e.className = cls; if (text) e.textContent = text; return e;
}
const fmt = (n: number, d = 1) => n.toLocaleString(undefined, { minimumFractionDigits: d, maximumFractionDigits: d });
function wowhead(a: HTMLAnchorElement, kind: 'item' | 'spell', id: number) {
	a.href = `https://www.wowhead.com/classic/${kind}=${id}`; a.target = '_blank'; a.rel = 'noopener';
	a.dataset.wowhead = `${kind}=${id}&domain=classic`;
}

const app = document.getElementById('app')!;
const side = el('aside', 'fa-side');
const main = el('main', 'fa-main');
app.append(side, main);

// ---------------------------------------------------------------- left: character + actions

const specSelect = el('select', 'fa-spec');
for (const s of SPECS) { const o = el('option', '', s.label); o.value = s.key; specSelect.append(o); }
specSelect.addEventListener('change', () => switchSpec(specSelect.value));
const heading = el('div', 'fa-heading');
const statsTable = el('table', 'fa-stats');
const dpsBox = el('div', 'fa-dps');
const buttons = el('div', 'fa-buttons');
const dpsButton = el('button', 'fa-btn', 'Sim equipped DPS');
const optimizeButton = el('button', 'fa-btn', 'Find best rotation');
const cancelButton = el('button', 'fa-btn', 'Cancel');
cancelButton.hidden = true;
cancelButton.onclick = () => { optimizerController?.abort(); void foregroundSignals.abortType(RequestTypes.All); };
const weightsButton = el('button', 'fa-btn', 'Stat Weights');
buttons.append(dpsButton, optimizeButton, weightsButton, cancelButton);
const settings = el('div', 'fa-settings');
side.append(specSelect, heading, statsTable, dpsBox, buttons, settings);

function field(label: string, input: HTMLElement) {
	const row = el('label', 'fa-field'); row.append(el('span', '', label), input); return row;
}
function numberInput(value: number, min: number, max: number, onChange: (v: number) => void) {
	const i = el('input'); i.type = 'number'; i.min = String(min); i.max = String(max); i.value = String(value);
	i.addEventListener('change', () => { const v = Math.max(min, Math.min(max, Math.round(Number(i.value) || min))); i.value = String(v); onChange(v); });
	return i;
}
function renderSettings() {
	settings.replaceChildren();
	const auto = el('input'); auto.type = 'checkbox'; auto.checked = state.autoRotation;
	auto.onchange = () => { state.autoRotation = auto.checked; changed(); };
	settings.append(field('Deep rotation search before DPS (slower)', auto));
	const level = el('input'); level.type = 'range'; level.min = '10'; level.max = '60'; level.value = String(state.level);
	const levelOut = el('output', '', String(state.level));
	level.addEventListener('input', () => { levelOut.textContent = level.value; });
	level.addEventListener('change', () => {
		state.level = Number(level.value); state.targetLevel = Math.min(63, state.level + 2);
		trimTalents(); changed();
	});
	const levelRow = el('div', 'fa-level'); levelRow.append(level, levelOut);
	settings.append(field('Level', levelRow));
	settings.append(field('Target level', numberInput(state.targetLevel, 1, 63, v => { state.targetLevel = v; changed(); })));
	const race = el('select');
	for (const r of foreverRaces(def.cls)) { const o = el('option', '', raceNames.get(r) || String(r)); o.value = String(r); o.selected = r === state.race; race.append(o); }
	race.addEventListener('change', () => { state.race = Number(race.value) as Race; changed(); });
	settings.append(field('Race', race));
	if (def.rotations.length > 1) {
		const rot = el('select');
		def.rotations.forEach((r, i) => { const o = el('option', '', r.label); o.value = String(i); o.selected = i === state.rotation; rot.append(o); });
		rot.addEventListener('change', () => { state.rotation = Number(rot.value); changed(); });
		settings.append(field('Rotation', rot));
	}
	settings.append(field('Fight length (s)', numberInput(state.duration, 20, 600, v => { state.duration = v; changed(); })));
	settings.append(field('Iterations', numberInput(state.iterations, 100, 50000, v => { state.iterations = v; save(); })));
	settings.append(field('Item sim iterations', numberInput(state.itemIterations, 100, 10000, v => { state.itemIterations = v; itemCache.clear(); save(); renderItems(); })));
	const armorNote = el('p', 'fa-note', `Target armor ${state.targetLevel > 60 ? 3731 : ARMOR[String(state.targetLevel)] || 0} (level ${state.targetLevel} mob)`);
	const reset = el('button', 'fa-link', 'Reset this spec'); reset.addEventListener('click', () => { state = defaults(def); changed(); });
	const advanced = el('a', 'fa-link', 'Advanced simulator ↗'); advanced.href = `${BASE}${def.key}/`;
	settings.append(armorNote, reset, advanced);
}

const STAT_ROWS: Array<[string, (s: number[]) => string, SpecDef['role'][]]> = [
	['Health', s => fmt(s[Stat.StatHealth], 0), ['melee', 'ranged', 'caster']],
	['Mana', s => fmt(s[Stat.StatMana], 0), ['ranged', 'caster']],
	['Strength', s => fmt(s[Stat.StatStrength], 0), ['melee']],
	['Agility', s => fmt(s[Stat.StatAgility], 0), ['melee', 'ranged']],
	['Stamina', s => fmt(s[Stat.StatStamina], 0), ['melee', 'ranged', 'caster']],
	['Intellect', s => fmt(s[Stat.StatIntellect], 0), ['ranged', 'caster']],
	['Spirit', s => fmt(s[Stat.StatSpirit], 0), ['caster']],
	['Attack Power', s => fmt(s[Stat.StatAttackPower], 0), ['melee']],
	['Ranged AP', s => fmt(s[Stat.StatRangedAttackPower], 0), ['ranged']],
	['Hit', s => `${fmt(s[Stat.StatMeleeHit], 2)}%`, ['melee', 'ranged']],
	['Crit', s => `${fmt(s[Stat.StatMeleeCrit], 2)}%`, ['melee', 'ranged']],
	['Spell Power', s => fmt(s[Stat.StatSpellPower] + s[Stat.StatSpellDamage], 0), ['caster']],
	['Spell Hit', s => `${fmt(s[Stat.StatSpellHit], 2)}%`, ['caster']],
	['Spell Crit', s => `${fmt(s[Stat.StatSpellCrit], 2)}%`, ['caster']],
	['Armor', s => fmt(s[Stat.StatArmor], 0), ['melee', 'ranged', 'caster']],
];
async function renderStats() {
	const revision = setupRevision;
	heading.replaceChildren(el('strong', '', `${raceNames.get(state.race)} ${def.label}`), el('span', '', `Level ${state.level} · ${pointsSpent()}/${pointsAllowed()} talent points`));
	try {
		const result = await pool.computeStats(ComputeStatsRequest.create({ raid: request(1).raid, encounter: encounter() }));
		if (revision !== setupRevision) return;
		const s = result.raidStats?.parties[0]?.players[0]?.finalStats?.stats || [];
		statsTable.replaceChildren(...STAT_ROWS.filter(r => r[2].includes(def.role)).map(([label, value]) => {
			const tr = el('tr'); tr.append(el('th', '', label), el('td', '', s.length ? value(s) : '—')); return tr;
		}));
		if (result.errorResult) statsTable.append(Object.assign(el('tr'), { textContent: result.errorResult }));
	} catch (e) { if (revision === setupRevision) statsTable.replaceChildren(el('tr', '', String(e))); }
}

// ---------------------------------------------------------------- top: paper doll + tabs

const doll = el('nav', 'fa-doll');
const tabs = el('div', 'fa-tabs');
const panel = el('section', 'fa-panel');
main.append(doll, tabs, panel);
const TAB_NAMES = ['Gear', 'Talents', 'Results', 'Rotation'] as const;
let tab: (typeof TAB_NAMES)[number] = 'Gear';
for (const name of TAB_NAMES) {
	const b = el('button', 'fa-tab', name); b.addEventListener('click', () => { if (tab === name) return; stopItemWork(); tab = name; renderPanel(); }); tabs.append(b);
}

function renderDoll() {
	doll.replaceChildren();
	for (const slot of SLOTS) {
		const item = byId.get(state.gear[slot.key]);
		const b = el('button', `fa-slot${state.slot === slot.key && tab === 'Gear' ? ' active' : ''}${item ? ` q-${QUALITY[item.quality]}` : ''}`);
		b.title = item ? `${slot.label}: ${item.name}` : `${slot.label}: empty`;
		const img = el('img'); img.alt = ''; img.src = item ? ICON(item.icon) : `https://wow.zamimg.com/images/wow/icons/medium/${slot.icon}.jpg`;
		b.append(img);
		b.addEventListener('click', () => { state.slot = slot.key; itemPage = 0; tab = 'Gear'; save(); renderPanel(); });
		doll.append(b);
	}
}

// ---------------------------------------------------------------- gear tab: item table with simulated DPS

const itemCache = new Map<string, number>();
let sortKey: 'dps' | 'ilvl' | 'name' = 'dps';
let search = '';
let itemPage = 0;
const PAGE_SIZE = 75;
function gearKey(gear: number[]) {
	return canonical([state.spec, state.level, state.race, state.targetLevel, state.duration, APLRotation.toJson(activeRotation()), state.talents, state.itemIterations, gear]);
}
function withItem(slot: number, id: number): number[] {
	const gear = [...state.gear]; gear[slot] = id;
	const item = byId.get(id);
	if (slot === 14 && item?.handType === 4) gear[15] = 0;
	// A unique item cannot be worn twice.
	if (id && item?.unique) { const twin = slot === 10 ? 11 : slot === 11 ? 10 : slot === 12 ? 13 : slot === 13 ? 12 : -1; if (twin >= 0 && gear[twin] === id) gear[twin] = 0; }
	return gear;
}
function equip(slot: number, id: number) {
	state.gear = withItem(slot, id); changed();
}
function renderItems() {
	if (tab !== 'Gear') return;
	stopItemWork();
	const slot = state.slot;
	const eligible = candidates(slot);
	const list = eligible.filter(i => !search || i.name.toLowerCase().includes(search));
	const head = el('div', 'fa-itemhead');
	const find = el('input', 'fa-search'); find.placeholder = 'Search'; find.value = search;
	let searchTimer: ReturnType<typeof setTimeout>;
	find.addEventListener('input', () => { clearTimeout(searchTimer); searchTimer = setTimeout(() => { if (!find.isConnected) return; search = find.value.toLowerCase(); itemPage = 0; renderItems(); (panel.querySelector('.fa-search') as HTMLInputElement)?.focus(); }, 150); });
	const filters = el('div', 'fa-filters');
	[['Commons', 1], ['Greens', 2], ['Blues', 3], ['Epics', 4]].forEach(([label, q]) => {
		const l = el('label'), c = el('input'); c.type = 'checkbox'; c.checked = state.qualities.includes(q as number);
		c.addEventListener('change', () => { state.qualities = c.checked ? [...state.qualities, q as number] : state.qualities.filter(x => x !== q); save(); renderItems(); });
		l.append(c, ` ${label}`); filters.append(l);
	});
	const above = el('label'), ac = el('input'); ac.type = 'checkbox'; ac.checked = state.aboveLevel;
	ac.addEventListener('change', () => { state.aboveLevel = ac.checked; save(); renderItems(); });
	above.append(ac, ' Above my level'); filters.append(above);
	head.append(el('h2', '', SLOTS[slot].label), find, filters);

	const table = el('table', 'fa-items');
	const thead = el('thead'), hr = el('tr');
	for (const [key, label] of [['ilvl', 'ilvl'], ['name', 'Name'], ['', 'Req'], ['', 'Source'], ['dps', 'DPS']] as const) {
		const th = el('th', key ? 'sortable' : '', label);
		if (key) th.addEventListener('click', () => { sortKey = key; itemPage = 0; renderItems(); });
		if (key === sortKey) th.classList.add('sorted');
		hr.append(th);
	}
	thead.append(hr); table.append(thead);
	const tbody = el('tbody');
	const rows: Array<{ id: number; item?: Item; row: HTMLTableRowElement; cell: HTMLTableCellElement }> = [];
	const context = gearKey([]);
	const keys = new Map([0, ...eligible.map(i => i.id)].map(id => [id, context + JSON.stringify(withItem(slot, id))]));
	const score = (id: number) => itemCache.get(keys.get(id)!);
	const sorted: Array<Item | undefined> = [undefined, ...list];
	sorted.sort((a, b) => sortKey === 'name' ? (a?.name || '').localeCompare(b?.name || '') : sortKey === 'ilvl'
		? (b?.ilvl || 0) - (a?.ilvl || 0) : (score(b?.id || 0) ?? -1) - (score(a?.id || 0) ?? -1));
	itemPage = Math.min(itemPage, Math.max(0, Math.ceil(sorted.length / PAGE_SIZE) - 1));
	for (const item of sorted.slice(itemPage * PAGE_SIZE, (itemPage + 1) * PAGE_SIZE)) {
		const id = item?.id || 0;
		const tr = el('tr', `${state.gear[slot] === id ? 'equipped' : ''}`);
		const name = el('td', 'name');
		if (item) {
			const a = el('a', `q-${QUALITY[item.quality]}`); wowhead(a, 'item', item.id);
			const img = el('img'); img.loading = 'lazy'; img.src = ICON(item.icon); img.alt = '';
			a.append(img, item.name); a.addEventListener('click', e => e.preventDefault()); name.append(a);
		} else name.append(el('span', 'none', '— None —'));
		const cell = el('td', 'dps', '');
		tr.append(el('td', 'ilvl', item ? String(item.ilvl) : ''), name, el('td', 'req', item?.requiredLevel ? String(item.requiredLevel) : ''),
			el('td', 'src', item?.sources?.[0]?.drop?.otherName || ''), cell);
		tr.addEventListener('click', () => equip(slot, id));
		rows.push({ id, item, row: tr, cell });
	}
	const paint = () => {
		const base = score(state.gear[slot]);
		for (const r of rows) {
			const v = score(r.id);
			r.cell.textContent = v === undefined ? '—' : fmt(v, 2);
			r.cell.title = v !== undefined && base !== undefined ? `${v - base >= 0 ? '+' : ''}${fmt(v - base, 2)} vs equipped` : '';
		}
	};
	tbody.append(...rows.map(r => r.row)); table.append(tbody);
	paint();
	const simSlot = el('button', 'fa-btn', 'Sim this slot'); simSlot.disabled = foregroundBusy;
	const stop = el('button', 'fa-btn', 'Stop'); stop.hidden = true;
	const progress = el('span', 'fa-note', `${eligible.length} eligible items. Search only filters the display.`);
	progress.setAttribute('role', 'status');
	stop.onclick = () => { stopItemWork(); stop.hidden = true; simSlot.disabled = false; progress.textContent = 'Stopped. Completed scores are kept.'; };
	const navigation = el('div', 'fa-itemhead');
	const prev = el('button', 'fa-btn', 'Previous'), next = el('button', 'fa-btn', 'Next');
	prev.disabled = itemPage === 0; next.disabled = (itemPage + 1) * PAGE_SIZE >= sorted.length;
	prev.onclick = () => { --itemPage; renderItems(); }; next.onclick = () => { ++itemPage; renderItems(); };
	navigation.append(prev, el('span', '', `Page ${itemPage + 1} of ${Math.max(1, Math.ceil(sorted.length / PAGE_SIZE))}`), next);
	head.append(simSlot, stop, progress);
	panel.replaceChildren(head, table, navigation, el('p', 'fa-note', `DPS uses ${state.itemIterations} iterations per item and the displayed rotation, with other slots as equipped. Switching tabs stops scoring; completed results are cached. Click DPS to sort cached results instantly.`));
	simSlot.onclick = async () => {
		const token = ++runToken;
		const missing = [...new Set([state.gear[slot], 0, ...eligible.map(i => i.id)])].filter(id => keys.has(id) && score(id) === undefined);
		const baseRequest = request(state.itemIterations);
		const jobs = missing.map(id => ({ key: keys.get(id)!, gear: withItem(slot, id) }));
		baseRequest.simOptions!.randomSeed = 20260921n;
		let done = 0, failed = 0, lastPaint = 0;
		simSlot.disabled = true; stop.hidden = false;
		const worker = async (job: {key: string; gear: number[]}) => {
				const signal = itemSignals.registerRunning(RequestTypes.RaidSim);
				const req = RaidSimRequest.clone(baseRequest);
				req.raid!.parties[0].players[0].equipment = EquipmentSpec.create({ items: job.gear.map(id => ItemSpec.create({ id })) });
				try {
					const result = await pool.raidSimAsync(req, () => {}, signal);
					if (!signal.abort.isTriggered() && !result.error && result.iterationsDone === req.simOptions!.iterations) {
						if (itemCache.size >= 4000) itemCache.delete(itemCache.keys().next().value!);
						itemCache.set(job.key, dps(result));
					} else if (!signal.abort.isTriggered()) failed++;
				} catch { failed++; } finally { itemSignals.unregisterRunning(signal); }
				done++;
				if (token !== runToken) return;
				progress.textContent = `${done}/${jobs.length} scored${failed ? ` · ${failed} failed` : ''}`;
				if (performance.now() - lastPaint > 150) { paint(); lastPaint = performance.now(); }
		};
		await runQueue(jobs, Math.min(2, pool.getNumWorkers()), () => token === runToken, worker);
		if (token !== runToken) return;
		sortKey = 'dps'; itemPage = 0; renderItems();
		const status = panel.querySelector('[role="status"]');
		if (status) status.textContent = `Complete: ${jobs.length - failed} new scores${failed ? `; ${failed} failed, click again to retry` : ''}. Sorted by DPS.`;
	};
}

// ---------------------------------------------------------------- talents tab

function talentInfo(name: string): TalentInfo | undefined { return TALENTS[foreverClassName(def.cls)]?.[name]; }
function trimTalents() {
	// Dropping level removes points from the deepest rows first.
	const recs = records().sort((a, b) => b.row - a.row);
	while (pointsSpent() > pointsAllowed()) {
		const r = recs.find(x => (state.talents[x.id] || 0) > 0)!;
		state.talents[r.id]--; if (!state.talents[r.id]) delete state.talents[r.id];
	}
}
function legal(next: Record<string, number>): string | null {
	if (pointsSpent(next) > pointsAllowed()) return `Level ${state.level} has ${pointsAllowed()} talent points.`;
	for (const r of records()) {
		const n = next[r.id] || 0; if (!n) continue;
		if (r.prerequisites.some(p => (next[p.id] || 0) < p.rank)) return `${r.name} needs its prerequisite maxed first.`;
		const lower = records().filter(t => t.tree === r.tree && t.row < r.row).reduce((a, t) => a + (next[t.id] || 0), 0);
		if (lower < r.required_points) return `${r.name} needs ${r.required_points} points in earlier rows.`;
	}
	return null;
}
function renderTalents() {
	const wrap = el('div', 'fa-talents');
	const top = el('div', 'fa-talenttop');
	top.append(el('strong', '', `${pointsSpent()} / ${pointsAllowed()} points`), el('span', 'fa-note', 'Left click adds a point, right click removes one. Talent text is the Forever beta client\'s.'));
	const presets = PRESETS[def.key] || [];
	if (presets.length) {
		const p = el('select'); p.append(Object.assign(el('option', '', 'Load a build…'), { value: '' }));
		presets.forEach((b, i) => { const o = el('option', '', b.label); o.value = String(i); p.append(o); });
		p.addEventListener('change', () => {
			const b = presets[Number(p.value)]; if (!b) return;
			state.talents = {}; // Fill the preset in its own order until the level's points run out.
			for (const [id, rank] of Object.entries(b.talents)) {
				for (let n = 1; n <= rank; n++) { const next = { ...state.talents, [id]: n }; if (legal(next)) break; state.talents = next; }
			}
			changed();
		});
		top.append(p);
	}
	const clear = el('button', 'fa-link', 'Reset talents'); clear.addEventListener('click', () => { state.talents = {}; changed(); });
	top.append(clear); wrap.append(top);
	const msg = el('p', 'fa-talentmsg');
	const trees = el('div', 'fa-trees');
	const treeNames = [...new Set(records().map(r => r.tree))];
	for (const tree of treeNames) {
		const t = el('div', 'fa-tree');
		const spent = records().filter(r => r.tree === tree).reduce((a, r) => a + (state.talents[r.id] || 0), 0);
		t.append(el('h3', '', `${tree} (${spent})`));
		const grid = el('div', 'fa-grid');
		for (const r of records().filter(x => x.tree === tree)) {
			const info = talentInfo(r.name);
			const rank = state.talents[r.id] || 0;
			const cell = el('button', `fa-talent${rank ? ' has' : ''}${rank === r.max_rank ? ' max' : ''}`);
			cell.style.gridRow = String(r.row); cell.style.gridColumn = String(r.column);
			const img = el('img'); img.src = ICON(info?.icon || ''); img.alt = r.name; cell.append(img, el('span', 'rank', `${rank}/${r.max_rank}`));
			const text = info?.desc?.[Math.max(0, rank - 1)] || r.ranks?.[Math.max(0, rank - 1)]?.effect || '';
			const next = rank && rank < r.max_rank ? `\nNext rank: ${info?.desc?.[rank] || ''}` : '';
			cell.title = `${r.name} (${rank}/${r.max_rank})\n${text}${next}`;
			cell.addEventListener('click', () => { const n = { ...state.talents, [r.id]: rank + 1 }; if (rank >= r.max_rank) return; const e = legal(n); if (e) { msg.textContent = e; return; } state.talents = n; changed(); });
			cell.addEventListener('contextmenu', ev => {
				ev.preventDefault(); if (!rank) return; const n = { ...state.talents, [r.id]: rank - 1 }; if (!n[r.id]) delete n[r.id];
				const e = legal(n); if (e) { msg.textContent = e; return; } state.talents = n; changed();
			});
			grid.append(cell);
		}
		t.append(grid); trees.append(t);
	}
	wrap.append(msg, trees);
	panel.replaceChildren(wrap);
}

// ---------------------------------------------------------------- results + rotation tabs

let lastResult: RaidSimResult | null = null;
let lastRequest: RaidSimRequest | null = null;
let lastIterations = 1;
// proto OtherAction, in enum order.
const OTHER = ['', 'Wait', 'Mana regen', 'Energy regen', 'Focus regen', 'Mana gain', 'Rage gain', 'Melee', 'Auto shot', 'Pet', 'Refund',
	'Damage taken', 'Healing model', 'Potion', 'Move', 'Combo points', 'Explosives', 'On-use trinket', 'Defensive trinket', 'Forever ability'];
function actionName(id: { rawId?: { oneofKind?: string; spellId?: number; itemId?: number; otherId?: number }; tag?: number } | undefined) {
	const raw = id?.rawId;
	if (raw?.oneofKind === 'spellId') return NAMES[String(raw.spellId)] || `Spell ${raw.spellId}`;
	if (raw?.oneofKind === 'itemId') return byId.get(raw.itemId || 0)?.name || `Item ${raw.itemId}`;
	if (raw?.oneofKind === 'otherId') {
		if (raw.otherId === 19) return foreverDiscoveryTalents.records.find(r => r.action_tag === id?.tag)?.name || foreverDiscoveryTalents.mechanics.find(m => m.action_tag === id?.tag)?.name || 'Forever ability';
		return OTHER[raw.otherId || 0] || 'Other';
	}
	return 'Unknown';
}
function renderResults() {
	if (!lastResult) { panel.replaceChildren(el('p', 'fa-note', 'Press DPS to simulate the current character.')); return; }
	const unit = lastResult.raidMetrics?.parties[0]?.players[0];
	const total = (unit?.actions || []).reduce((a, x) => a + x.targets.reduce((b, t) => b + t.damage, 0), 0) + (unit?.pets || []).reduce((a, p) => a + p.actions.reduce((b, x) => b + x.targets.reduce((c, t) => c + t.damage, 0), 0), 0);
	const table = el('table', 'fa-items fa-breakdown');
	const hr = el('tr'); ['Ability', 'Damage %', 'DPS', 'Casts / fight', 'Hit %', 'Crit %'].forEach(h => hr.append(el('th', '', h)));
	table.append(el('thead')); table.tHead!.append(hr);
	const tbody = el('tbody');
	const add = (label: string, actions: NonNullable<typeof unit>['actions']) => {
		for (const a of actions) {
			const dmg = a.targets.reduce((b, t) => b + t.damage, 0); if (!dmg) continue;
			const casts = a.targets.reduce((b, t) => b + t.casts, 0), hits = a.targets.reduce((b, t) => b + t.hits, 0), crits = a.targets.reduce((b, t) => b + t.crits, 0);
			const landed = a.targets.reduce((b, t) => b + t.hits + t.crits, 0), attempts = landed + a.targets.reduce((b, t) => b + t.misses + t.dodges + t.parries, 0);
			const tr = el('tr');
			tr.append(el('td', 'name', `${label}${actionName(a.id)}`), el('td', '', `${fmt((100 * dmg) / Math.max(1, total), 1)}%`),
				el('td', '', fmt(((unit?.dps?.avg || 0) * dmg) / Math.max(1, total), 1)),
				el('td', '', fmt(casts / Math.max(1, lastIterations), 1)), el('td', '', attempts ? `${fmt((100 * landed) / attempts, 1)}` : ''), el('td', '', landed ? `${fmt((100 * crits) / Math.max(1, hits + crits), 1)}` : ''));
			tbody.append(tr);
		}
	};
	add('', unit?.actions || []);
	for (const p of unit?.pets || []) add(`${p.name}: `, p.actions);
	table.append(tbody);
	const summary = el('div', 'fa-summary');
	summary.append(el('strong', '', `${fmt(unit?.dps?.avg || 0, 2)} DPS`), el('span', '', ` ± ${fmt(unit?.dps?.stdev || 0, 2)} · ${lastResult.raidMetrics?.dps ? '' : ''}${state.iterations} iterations · ${state.duration}s vs level ${state.targetLevel}`));
	panel.replaceChildren(summary, table, el('p', 'fa-note', 'Each ability\'s share of the average DPS; casts are per fight.'), provenanceBlock());
}
// What the result was computed from, and a copy button that captures everything needed to rerun it.
function provenanceBlock() {
	const p = lastResult?.provenance;
	const box = el('div', 'fa-note');
	const data = p?.gameDataBuild ? `Forever client data ${p.gameDataBuild} (${(p.gameDataSha256 || '').slice(0, 12)})` : 'Classic data';
	box.append(el('span', '', `${data} · engine ${String(import.meta.env.VITE_ENGINE_VERSION || 'dev')} · seed ${p?.randomSeed ?? '?'}`));
	if (p?.nonClientValues?.length) box.append(el('span', '', ` · ${p.nonClientValues.length} non-client values used`));
	const copy = el('button', 'fa-link', ' · Copy reproducible run');
	copy.addEventListener('click', async () => {
		if (!lastRequest || !lastResult) return;
		const replay = RaidSimRequest.clone(lastRequest);
		if (replay.simOptions && p) replay.simOptions.randomSeed = BigInt(p.randomSeed);
		for (const party of replay.raid?.parties || []) for (const player of party.players) if (player.forever && p?.gameDataBuild) player.forever.gameDataBuild = p.gameDataBuild;
		const text = JSON.stringify({ engine: import.meta.env.VITE_ENGINE_VERSION, provenance: lastResult.provenance, request: RaidSimRequest.toJson(replay) }, null, 1);
		try { await navigator.clipboard.writeText(text); copy.textContent = ' · Copied'; } catch { copy.textContent = ' · Copy failed'; }
	});
	box.append(copy);
	if (p?.nonClientValues?.length) {
		const details = el('details');
		details.append(el('summary', '', 'Values not from the client (assumptions)'), ...p.nonClientValues.map(v => el('div', '', v)));
		box.append(details);
	}
	return box;
}

function renderRotation() {
	const apl = APLRotation.toJson(activeRotation());
	const names: Record<string, string> = {};
	const collect = (value: any) => {
		if (!value || typeof value !== 'object') return;
		for (const [key, child] of Object.entries(value)) {
			if ((key === 'spellId' || key === 'auraId') && typeof child === 'object') names[canonical(child)] = actionName(ActionID.fromJson(child as any)).replace(/\s*\(Rank \d+\)/, '');
			else collect(child);
		}
	};
	collect(apl);
	const explanation = explainRotation(apl, names);
	const selected = optimized && optimized.key === setupKey() ? optimized.result : undefined;
	panel.replaceChildren(el('p', 'fa-note', selected
		? `Using the best validated rotation found for this gear, talents and ${state.duration}s encounter. ${selected.search.evaluated} candidates tested; ${selected.search.iterationsPerRotation} independent validation iterations per finalist. ${fmt(selected.improvement, 2)} DPS improvement over the selected preset.`
		: `Using the ${def.rotations[state.rotation]?.label || 'default'} preset. Choose Find best rotation to search for this gear and encounter.`), el('p', 'fa-note', explanation.instructions));
	for (const section of ['opening', 'priority', 'cooldowns', 'execute'] as const) {
		const list = el('ol', 'fa-rotation');
		for (const line of explanation[section]) list.append(el('li', '', line));
		if (list.children.length) panel.append(el('h3', '', section[0].toUpperCase() + section.slice(1)), list);
	}
	panel.append(el('p', 'fa-note', 'The engine uses your highest learned spell rank. Search compares presets, priorities, resource thresholds and cooldown timing using your fight and gear. It finds the best supported strategy within a bounded search; a global maximum is not guaranteed.'));
}
function renderPanel() {
	for (const b of tabs.querySelectorAll('button')) b.classList.toggle('active', b.textContent === tab);
	renderDoll();
	if (tab === 'Gear') renderItems();
	else if (tab === 'Talents') renderTalents();
	else if (tab === 'Results') renderResults();
	else renderRotation();
}

// ---------------------------------------------------------------- actions

function busy(value: boolean) {
	foregroundBusy = value;
	dpsButton.disabled = optimizeButton.disabled = weightsButton.disabled = value;
	cancelButton.hidden = !value;
	if (value) stopItemWork();
	if (tab === 'Gear') renderItems();
}

async function findRotation() {
	const key = setupKey();
	if (optimized?.key === key) return;
	optimizerController = new AbortController();
	const controller = optimizerController;
	dpsBox.textContent = 'Preparing rotation search…';
	const release = await fetch(`${BASE}release.json?v=${import.meta.env.VITE_ENGINE_VERSION || ''}`).then(r => { if (!r.ok) throw Error('Unable to load engine version'); return r.json(); });
	const req = request(1);
	// Search starts from the user-selected preset and holds gear/talents/encounter fixed.
	req.raid!.parties[0].players[0].rotation = rotation(def, state.rotation);
	const templates = [];
	for (let i = 0; i < def.rotations.length; i++) {
		if (controller.signal.aborted) throw Error('Optimization cancelled');
		const candidate = RaidSimRequest.clone(req);
		candidate.raid!.parties[0].players[0].rotation = rotation(def, i);
		const stats = await pool.computeStats(ComputeStatsRequest.create({ raid: candidate.raid, encounter: candidate.encounter }));
		if (stats.errorResult) throw Error(stats.errorResult);
		templates.push(learnedRotation(APLRotation.toJson(candidate.raid!.parties[0].players[0].rotation!), stats.raidStats?.parties[0]?.players[0]?.rotationStats));
	}
	req.raid!.parties[0].players[0].rotation = APLRotation.fromJson(templates[state.rotation] || templates[0]);
	const result = await optimize({ request: RaidSimRequest.toJson(req),
		pins: { engine: release.engineSha256, database: release.databaseSha256, mechanics: foreverDiscoveryTalents.manifest_sha256 },
		templates, seed: 20260921,
		signal: controller.signal, cache: optimizerCache,
		backend: {
			validate: async (r: any) => {
				const stats = await pool.computeStats(ComputeStatsRequest.fromJson({ raid: r.raid, encounter: r.encounter }));
				const unit = stats.raidStats?.parties[0]?.players[0];
				const warnings = [stats.errorResult, ...(unit?.rotationStats?.prepullActions || []).flatMap(x => x.warnings), ...(unit?.rotationStats?.priorityList || []).flatMap(x => x.warnings)].filter(Boolean);
				return { valid: !!unit && !warnings.length, warnings, metadata: unit?.metadata ? UnitMetadata.toJson(unit.metadata) : {} };
			},
			run: async (r: any) => {
				const signal = foregroundSignals.registerRunning(RequestTypes.RaidSim);
				const abort = () => { void signal.abort.trigger(); };
				controller.signal.addEventListener('abort', abort, { once: true });
				if (controller.signal.aborted) abort();
				try { return RaidSimResult.toJson(await pool.raidSimAsync(RaidSimRequest.fromJson(r), () => {}, signal)); }
				finally { foregroundSignals.unregisterRunning(signal); controller.signal.removeEventListener('abort', abort); }
			},
		},
		onProgress: (p: any) => { if (key !== setupKey()) return; dpsBox.textContent = p.phase === 'search' ? `Searching rotations: ${p.evaluated}/90 (${p.rejected} rejected)` : `Validating rotation: ${p.completed}/${p.total}`; },
	});
	if (controller.signal.aborted || key !== setupKey()) throw Error('Setup changed or search cancelled. Run again for the current setup.');
	optimized = { key, apl: APLRotation.fromJson(result.apl), result };
	lastResult = null; lastRequest = null;
}

optimizeButton.onclick = async () => {
	busy(true);
	try { await findRotation(); dpsBox.textContent = 'Rotation search complete.'; tab = 'Rotation'; renderPanel(); }
	catch (e) { dpsBox.textContent = String(e); }
	finally { busy(false); }
};

dpsButton.addEventListener('click', async () => {
	busy(true); dpsBox.textContent = 'Simulating…';
	const revision = setupRevision;
	try {
		if (state.autoRotation) await findRotation();
		if (revision !== setupRevision) return;
		const req = request(state.iterations);
		const signal = foregroundSignals.registerRunning(RequestTypes.RaidSim);
		let result: RaidSimResult;
		const started = performance.now();
		try {
			const split = await pool.raidSimRequestSplit(RaidSimRequestSplitRequest.create({ splitCount: pool.getNumWorkers(), request: req }));
			if (split.errorResult || !split.requests.length) throw Error(split.errorResult || 'Unable to split simulation');
			if (signal.abort.isTriggered()) return;
			const counts = split.requests.map(() => 0), estimates = split.requests.map(() => 0);
			const results = await Promise.all(split.requests.map((part, i) => pool.raidSimAsync(part, p => {
				counts[i] = p.completedIterations; estimates[i] = p.dps;
				const completed = counts.reduce((a, b) => a + b, 0);
				if (revision === setupRevision) dpsBox.textContent = `${fmt(completed ? estimates.reduce((sum, v, j) => sum + v * counts[j], 0) / completed : 0, 1)} DPS · ${completed}/${req.simOptions!.iterations} · ${fmt((performance.now() - started) / 1000, 1)}s`;
			}, signal)));
			const error = results.find(r => r.error);
			result = error || await pool.raidSimResultCombination(RaidSimResultCombinationRequest.create({ results }));
		}
		finally { foregroundSignals.unregisterRunning(signal); }
		if (revision !== setupRevision || signal.abort.isTriggered()) { dpsBox.textContent = 'Simulation cancelled.'; return; }
		if (result.error) { dpsBox.textContent = result.error.message; return; }
		lastRequest = req; lastResult = result; lastIterations = req.simOptions!.iterations;
		const unit = result.raidMetrics?.parties[0]?.players[0];
		dpsBox.replaceChildren(el('strong', '', fmt(unit?.dps?.avg || 0, 2)), el('span', '', ` DPS · ${fmt((performance.now() - started) / 1000, 1)}s`));
		tab = 'Results'; renderPanel();
	} catch (e) { dpsBox.textContent = String(e); }
	finally { busy(false); }
});
weightsButton.addEventListener('click', async () => {
	busy(true); dpsBox.textContent = 'Computing stat weights…';
	const revision = setupRevision;
	const signal = foregroundSignals.registerRunning(RequestTypes.StatWeights);
	const stats = def.role === 'caster' ? [Stat.StatIntellect, Stat.StatSpirit, Stat.StatSpellPower, Stat.StatSpellHit, Stat.StatSpellCrit]
		: [Stat.StatStrength, Stat.StatAgility, Stat.StatAttackPower, Stat.StatMeleeHit, Stat.StatMeleeCrit];
	if (def.role === 'ranged') stats.push(Stat.StatRangedAttackPower);
	try {
		const req = request(Math.max(1000, state.iterations));
		const result = await pool.statWeightsAsync(StatWeightsRequest.create({
			player: req.raid!.parties[0].players[0], raidBuffs: RaidBuffs.create(), partyBuffs: PartyBuffs.create(), debuffs: Debuffs.create(),
			encounter: req.encounter, simOptions: req.simOptions, statsToWeigh: stats, epReferenceStat: stats[def.role === 'caster' ? 2 : 2], tanks: [] as UnitReference[],
		}), () => {}, signal);
		if (revision !== setupRevision || signal.abort.isTriggered()) return;
		const w = result.dps?.weights?.stats || [];
		const tbl = el('table', 'fa-stats');
		for (const s of stats) { const tr = el('tr'); tr.append(el('th', '', Stat[s].replace('Stat', '').replace(/([a-z])([A-Z])/g, '$1 $2')), el('td', '', fmt(w[s] || 0, 3))); tbl.append(tr); }
		dpsBox.replaceChildren(el('span', '', 'DPS per point'), tbl);
	} catch (e) { dpsBox.textContent = String(e); }
	finally { foregroundSignals.unregisterRunning(signal); busy(false); }
});

function changed() {
	++setupRevision; stopItemWork(); optimizerController?.abort(); void foregroundSignals.abortType(RequestTypes.All);
	lastResult = null; lastRequest = null; dpsBox.textContent = '';
	save(); renderSettings(); void renderStats(); renderPanel();
}
function switchSpec(key: string) {
	def = SPECS.find(s => s.key === key) || def;
	state = { ...defaults(def), ...(load(def.key) || {}) };
	location.hash = def.key; specSelect.value = def.key; lastResult = null; changed();
}

// ---------------------------------------------------------------- boot

(async () => {
	specSelect.value = def.key;
	const db = await fetch(`${BASE}assets/database/db.json`).then(r => r.json());
	items = (db.items as Item[]).filter(i => !i.hidden);
	byId = new Map(items.map(i => [i.id, i]));
	// Drop anything saved from an older pool.
	state.gear = state.gear.map(id => (byId.has(id) ? id : 0));
	changed();
	const wh = document.createElement('script'); wh.src = 'https://wow.zamimg.com/js/tooltips.js'; document.head.append(wh);
	(window as unknown as { whTooltips: object }).whTooltips = { colorLinks: false, iconizeLinks: false, renameLinks: false };
})();
