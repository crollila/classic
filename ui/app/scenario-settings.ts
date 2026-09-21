import { Consumes, Debuffs, IndividualBuffs, RaidBuffs, PartyBuffs, Profession, MobType } from '../core/proto/common';
import { foreverDiscoveryTalents } from '../forever/discovery';

const readable = (name: string) => name.replace(/([a-z0-9])([A-Z])/g, '$1 $2').replace(/_/g, ' ').replace(/^./, c => c.toUpperCase());

export function renderScenarioSettings(root: HTMLElement, s: any, warrior: boolean, change: () => void) {
  const opened = new Set(Array.from(root.querySelectorAll('details[open] > summary')).map(e => e.textContent));
  root.replaceChildren();
  const note = (parent: HTMLElement, text: string) => { const p = document.createElement('p'); p.className = 'fa-note'; p.textContent = text; parent.append(p); };
  const section = (title: string, open = false) => {
    const d = document.createElement('details'); d.className = 'fa-scenario-section'; d.open = opened.size ? opened.has(title) : open;
    const h = document.createElement('summary'); h.textContent = title; d.append(h); root.append(d); return d;
  };
  const field = (parent: HTMLElement, label: string, input: HTMLElement) => {
    const row = document.createElement('label'); row.className = 'fa-field'; const span = document.createElement('span'); span.textContent = label; row.append(span, input); parent.append(row);
  };
  const number = (parent: HTMLElement, label: string, obj: any, key: string, min: number, max: number, step = 1, fallback = min) => {
    const i = document.createElement('input'); i.type = 'number'; i.min = String(min); i.max = String(max); i.step = String(step); i.value = String(obj[key] ?? fallback);
    i.onchange = () => { const v = Number(i.value); if (!Number.isFinite(v)) return; obj[key] = Math.max(min, Math.min(max, Math.round(v / step) * step)); change(); }; field(parent, label, i);
  };
  const choice = (parent: HTMLElement, label: string, obj: any, key: string, values: Array<[string, string]>, numeric = false) => {
    const select = document.createElement('select');
    for (const [value, text] of values) { const o = document.createElement('option'); o.value = value; o.textContent = text; o.selected = String(obj[key] ?? 0) === value; select.append(o); }
    select.onchange = () => { obj[key] = numeric ? Number(select.value) : select.value; change(); }; field(parent, label, select);
  };
  const check = (parent: HTMLElement, label: string, obj: any, key: string) => { const i = document.createElement('input'); i.type = 'checkbox'; i.checked = !!obj[key]; i.onchange = () => { obj[key] = i.checked; change(); }; field(parent, label, i); };
  const enumValues = (values: any): Array<[string, string]> => Object.entries(values).filter(([, v]) => typeof v === 'number').map(([k, v]) => [String(v), Number(v) === 0 ? 'None' : readable(k)]);
  note(root, 'All enabled controls are sent to equipped DPS, slot comparisons, stat weights and rotation search. Changes discard old results and invalidate the best-rotation selection. Settings are saved per spec.');
  const fight = section('Fight and target', true);
  number(fight, 'Fight variation (± seconds)', s, 'durationVariation', 0, 300);
  number(fight, 'Below 20% health (% of fight)', s, 'execute20', 0, 100);
  number(fight, 'Below 35% health (% of fight)', s, 'execute35', 0, 100);
  number(fight, 'Base armor (-1 = level default)', s, 'armor', -1, 100000);
  number(fight, 'Magic resistance (all schools)', s, 'resistance', 0, 1000);
  number(fight, 'Total targets (including boss)', s, 'targets', 1, 20);
  choice(fight, 'Target type', s, 'mobType', enumValues(MobType).map(([v, n]) => [v, n.replace('Mob Type ', '')]), true);
  check(fight, 'Stand in front of target', s, 'inFront');
  check(fight, 'Target attacks this character', s, 'tanking');
  number(fight, 'Target swing interval (ms)', s, 'targetSwingMs', 100, 30000);
  number(fight, 'Target minimum swing damage', s, 'targetMinDamage', 0, 100000);
  number(fight, 'Target maximum swing damage', s, 'targetMaxDamage', 0, 100000);
  note(fight, 'Other targets share these stats. Incoming swings require “Target attacks this character”; zero damage adds no incoming-damage rage. Fight duration is the sidebar value ± variation, clamped to stay positive.');
  const character = section('Character and execution');
  number(character, 'Reaction delay (ms)', s, 'reactionMs', 0, 2000);
  number(character, 'Distance (yards; -1 = spec default)', s, 'distance', -1, 100);
  number(character, 'Reproducible random seed', s, 'seed', 1, 2147483647);
  choice(character, 'Profession 1', s, 'profession1', enumValues(Profession), true);
  choice(character, 'Profession 2', s, 'profession2', enumValues(Profession), true);
  if (warrior) {
    number(character, 'Initial rage', s, 'startingRage', 0, 100);
    number(character, 'Heroic Strike / Cleave queue delay (ms)', s, 'queueDelay', 0, 2000);
  }
  const debuffs = section('Target debuffs and responsibilities', true);
  choice(debuffs, 'Sunder Armor', s, 'sunder', [['preset', 'Use preset responsibility'], ['none', 'Do not cast / none supplied'], ['external', '5 stacks supplied externally — never cast']]);
  note(debuffs, 'External Sunder uses the engine’s max-rank, five-stack debuff (0.8-second initial ramp). Expose Armor takes precedence. External debuffs are max-rank raid assumptions, not automatically level-matched providers.');
  // Schema-backed forms avoid inventing unsupported settings or losing enum choices.
  const message = (parent: HTMLElement, type: any, obj: any, excluded: string[] = []) => {
    for (const f of type.fields) {
      const key = f.localName;
      if (excluded.includes(key) || f.options?.deprecated || f.repeat || f.oneof) continue;
      const label = readable(key);
      if (f.kind === 'enum') {
        let values = enumValues(f.T()[1]);
        if (f.T()[0].endsWith('TristateEffect')) values = [['0', 'None'], ['1', 'Regular'], ['2', 'Improved']];
        // These talents were removed in Forever. Do not offer their Classic improved ranks.
        if (['battleShout', 'demoralizingShout'].includes(key)) values = values.filter(([v]) => v !== '2');
        choice(parent, label, obj, key, values, true);
      } else if (f.kind === 'scalar' && f.T === 8) check(parent, label, obj, key);
      else if (f.kind === 'scalar') number(parent, label, obj, key, 0, 20);
      else if (f.kind === 'message') { obj[key] ??= {}; const nested = document.createElement('details'); const title = document.createElement('summary'); title.textContent = label; nested.append(title); parent.append(nested); message(nested, f.T(), obj[key]); }
    }
  };
  message(debuffs, Debuffs, s.debuffs, ['sunderArmor', 'judgementOfLight']);
  const buffs = section('Raid, party and world buffs');
  note(buffs, 'These are externally supplied buffs. External Battle Shout removes self-casts from warrior rotations. Faction restrictions and exclusive-effect rules are enforced by the engine. Improved Battle Shout and Improved Demoralizing Shout are unavailable in Forever.');
  message(buffs, RaidBuffs, s.raidBuffs);
  message(buffs, PartyBuffs, s.partyBuffs);
  message(buffs, IndividualBuffs, s.buffs, ['blessingOfSanctuary']);
  const consumes = section('Consumables and weapon imbues');
  note(consumes, 'This list exposes the currently implemented engine catalog, not every new Forever consumable. Availability, level restrictions and changed effects still require coverage review.');
  message(consumes, Consumes, s.consumes, ['boglingRoot']);
  const forever = section('Forever Legacy, camps and profession effects');
  note(forever, 'Options come from the versioned Forever manifest. Predicted behavior is identified below; unavailable/non-combat entries cannot be selected. Profession effects may also require the matching profession and gear.');
  for (const m of foreverDiscoveryTalents.mechanics.filter(m => ['legacy_perk', 'profession_claim'].includes(m.kind) || m.category === 'BUFFS' || m.id.startsWith('buffs.'))) {
    if (m.mode === 'blocked' || m.mode === 'non-sim' || m.mode === 'baseline') { note(forever, `${m.name}: ${m.mode === 'blocked' ? 'not implemented' : m.mode === 'baseline' ? 'baseline behavior; no adjustable combat effect' : 'not modeled in combat'}`); continue; }
    number(forever, `${m.name} (${m.confidence})`, s.mechanicRanks, m.id, 0, m.max_rank);
    note(forever, `${m.effect}${m.adapter?.remaining_unknowns?.length ? ' Unknown: ' + m.adapter.remaining_unknowns.join('; ') : ''}`);
    if (s.mechanicRanks[m.id]) for (const p of m.adapter?.parameters || []) {
      number(forever, `${p.label} (${p.confidence})`, s.parameters, p.key, p.min, p.max, 0.01, p.default);
    }
  }
  const gaps = section('Coverage and differences from Classic');
  note(gaps, 'Uses Forever race/talent data and the existing versioned mechanics layer. This page does not claim complete validation of every buff, consumable or beta mechanic. Classic AQ-book toggles are replaced by the engine’s learned-rank selection.');
  note(gaps, 'Not adjustable yet: engine spell batching (fixed 10 ms), randomized reaction-delay ranges, arbitrary bleed reduction, or a separate spell-queueing on/off switch. Reaction delay is the engine’s fixed player setting, not a universal delay applied to every action. Enchant comparison and item-source/phase filters remain separate coverage gaps.');
  for (const [label, href] of [['Forever changes and evidence', 'https://foreverchanges.pro/'], ['Reference settings', 'https://guybrushgit.github.io/WarriorSim/classic.html']]) {
    const a = document.createElement('a'); a.textContent = label; a.href = href; a.target = '_blank'; a.rel = 'noopener'; gaps.append(a, document.createElement('br'));
  }
}
