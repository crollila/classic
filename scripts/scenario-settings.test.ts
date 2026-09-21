import test from 'node:test';
import assert from 'node:assert/strict';
import { renderScenarioSettings } from '../ui/app/scenario-settings';
import { defaultScenario, scenarioDebuffs } from '../ui/app/scenario.mjs';
import { RaidBuffs, Debuffs, Consumes } from '../ui/core/proto/common';

// Small DOM fixture: test schema-generated forms and input bindings without a browser.
class Element {
  children: Element[] = [];
  textContent = ''; value = ''; checked = false;
  onchange?: () => void;
  constructor(public tagName: string) {}
  append(...children: Element[]) { this.children.push(...children); }
  replaceChildren(...children: Element[]) { this.children = children; }
  querySelectorAll() { return []; }
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
