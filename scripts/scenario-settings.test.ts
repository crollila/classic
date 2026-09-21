import test from 'node:test';
import assert from 'node:assert/strict';
import { renderScenarioSettings } from '../ui/app/scenario-settings';
import { defaultScenario, scenarioDebuffs } from '../ui/app/scenario.mjs';
import { RaidBuffs, Debuffs, Consumes } from '../ui/core/proto/common';
import { SETTING_HELP } from '../ui/app/setting-help';

// Small DOM fixture: test schema-generated forms and input bindings without a browser.
class Element {
  children: Element[] = [];
  textContent = ''; value = ''; checked = false;
  id = ''; title = ''; hidden = false; className = '';
  attributes: Record<string, string> = {};
  onchange?: () => void;
  constructor(public tagName: string) {}
  append(...children: Element[]) { this.children.push(...children); }
  replaceChildren(...children: Element[]) { this.children = children; }
  querySelectorAll() { return []; }
  setAttribute(key: string, value: string) { this.attributes[key] = value; }
  addEventListener() {}
}
const walk = (e: Element): Element[] => [e, ...e.children.flatMap(walk)];
function form(warrior = true) {
  const previous = globalThis.document;
  globalThis.document = { createElement: (tag: string) => new Element(tag) } as unknown as Document;
  const root = new Element('section'), scenario = defaultScenario(); let changes = 0;
  try { renderScenarioSettings(root as unknown as HTMLElement, scenario, warrior, () => changes++); }
  finally { globalThis.document = previous; }
  const control = (label: string) => {
    const row = walk(root).find(e => e.tagName === 'label' && e.children[0]?.textContent === label);
    assert.ok(row, `missing ${label}`); return row.children[1];
  };
  return { root, scenario, control, changes: () => changes };
}
test('settings form wires debuffs, buffs and consumables to real protobuf fields', () => {
  const f = form();
  const sunder = f.control('Sunder Armor'); sunder.value = 'external'; sunder.onchange!();
  assert.equal(Debuffs.create(scenarioDebuffs(f.scenario)).sunderArmor, true);
  const shout = f.control('Battle Shout'); shout.value = '1'; shout.onchange!();
  assert.equal(RaidBuffs.create(f.scenario.raidBuffs).battleShout, 1);
  assert.equal(shout.children.some(e => e.value === '2'), false);
  const food = f.control('Food'); food.value = food.children.find(e => e.value !== '0')!.value; food.onchange!();
  assert.ok(Consumes.create(f.scenario.consumes).food > 0);
  assert.equal(f.changes(), 3);
});
test('opening settings does not alter the scenario or expose unsupported no-op buffs', () => {
  const f = form(false);
  assert.deepEqual(f.scenario, defaultScenario());
  const labels = walk(f.root).filter(e => e.tagName === 'label').map(e => e.children[0].textContent);
  assert.ok(!labels.includes('Initial rage'));
  assert.ok(!labels.includes('Blessing Of Sanctuary'));
  assert.ok(!labels.includes('Judgement Of Light'));
  assert.ok(labels.length > 100, `only ${labels.length} settings rendered`);
  assert.equal(labels.filter(label => label === 'Blessing Of Wisdom').length, 1, 'only the functional individual Wisdom control is offered');
});
test('fight input bounds and warrior options are applied on change', () => {
  const f = form();
  const targets = f.control('Total targets (including boss)'); targets.value = '999'; targets.onchange!();
  assert.equal(f.scenario.targets, 20);
  const rage = f.control('Initial rage'); rage.value = '40'; rage.onchange!();
  assert.equal(f.scenario.startingRage, 40);
  const armor = f.control('Base armor (-1 = level default)'); armor.value = '0'; armor.onchange!();
  assert.equal(f.scenario.armor, 0);
});

test('every rendered setting has descriptive help, an accessible association and touch help', () => {
  const f = form();
  for (const row of walk(f.root).filter(e => e.tagName === 'label')) {
    const input = row.children[1], help = row.children[2], tip = row.children[3];
    assert.ok(tip.textContent.length > 25, row.children[0].textContent);
    assert.ok(!tip.textContent.includes('has not yet been documented'), row.children[0].textContent);
    assert.equal(input.attributes['aria-describedby'], tip.id);
    assert.equal(tip.attributes.role, 'tooltip');
    assert.equal(help.tagName, 'button');
    assert.equal(help.attributes['aria-expanded'], 'false');
    assert.equal(tip.hidden, true);
  }
  assert.match(SETTING_HELP.sunder, /removes all self-casts/);
  assert.match(SETTING_HELP.scorpidSting, /no stat reduction/);
  assert.deepEqual(f.scenario, defaultScenario(), 'help must not change the setup');
});

test('pet consume selection codes cannot exceed engine table bounds', () => {
  const f = form();
  for (const [label, key, max] of [['Pet Agility Consumable', 'petAgilityConsumable', 4], ['Pet Strength Consumable', 'petStrengthConsumable', 5], ['Pet Attack Power Consumable', 'petAttackPowerConsumable', 1]] as const) {
    const control = f.control(label); control.value = '20'; control.onchange!();
    assert.equal((f.scenario.consumes as Record<string, unknown>)[key], max);
  }
});
