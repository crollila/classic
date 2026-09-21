import test from 'node:test';
import assert from 'node:assert/strict';
import {runQueue} from '../ui/app/work-queue.mjs';
import {learnedRotation} from '../ui/app/rotation-context.mjs';
import {parallelism} from '../ui/app/parallelism.mjs';
import {defaultScenario, scenarioRotation, scenarioDebuffs} from '../ui/app/scenario.mjs';
import {classifyGear, gearAllowed, defaultGearFilter} from '../ui/app/gear-availability.mjs';
import {readFileSync} from 'node:fs';
import {normalizeWeapons, replaceWeapon, weaponViewId} from '../ui/app/weapon-slots.mjs';

test('two-hand and dual-wield choices are mutually exclusive in every candidate', () => {
  const items = new Map([[1,{handType:4}],[2,{handType:2}],[3,{handType:2}]]); // Synthetic.
  const itemFor = id => items.get(id);
  const dual = Array(17).fill(0); dual[14] = 2; dual[15] = 3;
  const two = replaceWeapon(dual,17,1,itemFor);
  assert.equal(two.length,17); assert.deepEqual(two.slice(14,16),[1,0]);
  assert.deepEqual(dual.slice(14,16),[2,3]); // Comparison cannot mutate equipped gear.
  assert.equal(weaponViewId(two,17,itemFor,true),1);
  assert.equal(weaponViewId(two,14,itemFor,true),0);
  assert.deepEqual(replaceWeapon(two,15,3,itemFor).slice(14,16),[0,3]);
  assert.deepEqual(replaceWeapon(two,14,2,itemFor).slice(14,16),[2,0]);
  assert.deepEqual(replaceWeapon(dual,17,0,itemFor),dual);
  assert.deepEqual(replaceWeapon(two,17,0,itemFor).slice(14,16),[0,0]);
  assert.deepEqual(normalizeWeapons([...two.slice(0,15),3,0,1],itemFor),two);
  assert.throws(() => replaceWeapon(dual,17,2,itemFor));
});

test('client presence and legacy phases never imply Forever obtainability', () => {
  const legacy = new Set([1]); // Synthetic IDs, not game evidence.
  const snapshot = {items:{1:{},2:{}}};
  const old = classifyGear({id:1,phase:6}, snapshot, {}, legacy);
  assert.equal(old.evidence, 'client'); assert.equal(old.origin, 'legacy');
  assert.equal(gearAllowed(old, defaultGearFilter()), false);
  const listed = classifyGear({id:2}, snapshot, {'2':{carried:{sources:'foreverchanges.pro loot table (names only)'}}}, legacy);
  assert.equal(gearAllowed(listed, defaultGearFilter()), true);
  assert.equal(listed.origin, 'unclassified');
  assert.equal(gearAllowed(old, {...defaultGearFilter(),evidence:'client',maxPhase:5}), false);
  assert.equal(gearAllowed(old, {...defaultGearFilter(),evidence:'client',maxPhase:6}), true);
  assert.equal(gearAllowed(listed, {...defaultGearFilter(),unclassified:false}), false);
  assert.equal(gearAllowed(undefined, defaultGearFilter()), false);
  assert.equal(gearAllowed(undefined, {...defaultGearFilter(),evidence:'all'}), true);
});

test('all matching gear renders continuously without page slicing', () => {
  const app = readFileSync(new URL('../ui/app/main.ts',import.meta.url),'utf8');
  assert.match(app, /for \(const item of sorted\)/);
  assert.doesNotMatch(app, /PAGE_SIZE|itemPage|sorted\.slice/);
});

test('external Sunder removes every rank and nested cast without mutating presets', () => {
  const cast = id => ({castSpell:{spellId:{spellId:id}}});
  const apl = {prepullActions:[{action:cast(7386)}], priorityList:[
    ...[7386,7405,8380,11596,11597].map(id => ({action:cast(id)})),
    {action:{sequence:{actions:[cast(11597),cast(78)]}}}, {action:cast(23894)},
  ]};
  const s = defaultScenario(); s.sunder = 'external';
  const out = scenarioRotation(apl, s, true);
  assert.equal(out.prepullActions.length, 0);
  assert.equal(out.priorityList.length, 2);
  assert.deepEqual(out.priorityList[0].action.sequence.actions, [cast(78)]);
  assert.equal(apl.priorityList.length, 7);
  assert.equal(scenarioDebuffs(s).sunderArmor, true);
  assert.deepEqual(scenarioRotation(apl, defaultScenario(), true), apl);
  assert.deepEqual(scenarioRotation(apl, s, false), apl);
});
test('external Battle Shout avoids self-casts and Expose takes precedence over Sunder', () => {
  const s = defaultScenario(); s.sunder = 'external'; s.debuffs.exposeArmor = 1; s.raidBuffs.battleShout = 1;
  const apl = {priorityList:[25289,11597,78].map(spellId => ({action:{castSpell:{spellId:{spellId}}}}))};
  assert.equal(scenarioDebuffs(s).sunderArmor, false);
  assert.equal(scenarioRotation(apl,s,true).priorityList.length, 1);
  assert.equal(scenarioRotation(apl,s,true).priorityList[0].action.castSpell.spellId.spellId, 78);
});
test('scenario settings have independent defaults and preserve explicit zero values', () => {
  const a = defaultScenario(), b = defaultScenario();
  a.debuffs.faerieFire = true; a.armor = 0; a.execute20 = 0;
  assert.equal(b.debuffs.faerieFire, undefined);
  assert.equal(a.armor, 0); assert.equal(a.execute20, 0);
});

test('auto reserves CPU capacity and bounds missing or extreme hardware hints', () => {
  assert.equal(parallelism('auto', 24), 21);
  assert.equal(parallelism('auto', 8), 7);
  assert.equal(parallelism('auto', 1), 1);
  assert.equal(parallelism('auto', undefined), 3);
  assert.equal(parallelism('auto', NaN), 3);
  assert.equal(parallelism('auto', 256), 64);
  assert.equal(parallelism('invalid', 8), 7);
});
test('manual modes permit 20 and 64 even when CPU reports fewer threads', () => {
  assert.equal(parallelism('20', 8), 20);
  assert.equal(parallelism('64', 8), 64);
  assert.equal(parallelism('0', 8), 7);
});
test('high concurrency runs each item exactly once and stops pending work on cancellation', async () => {
  for (const concurrency of [1, 20, 64]) {
    const seen = new Set(); let active = 0, peak = 0;
    await runQueue(Array.from({length: 100}, (_, i) => i), concurrency, () => true, async id => {
      assert.equal(seen.has(id), false); seen.add(id); peak = Math.max(peak, ++active);
      await new Promise(resolve => setTimeout(resolve, 0)); active--;
    });
    assert.equal(seen.size, 100); assert.equal(peak, concurrency);
    let current = true, started = 0;
    await runQueue(Array(100).fill(0), concurrency, () => current, async () => {
      started++; await new Promise(resolve => setTimeout(resolve, 0)); current = false;
    });
    assert.equal(started, concurrency);
  }
});

test('large slot queues keep at most two simulations in flight', async () => {
  let active = 0, peak = 0, completed = 0;
  await runQueue(Array.from({length: 500}, (_, i) => i), 2, () => true, async () => {
    peak = Math.max(peak, ++active);
    await new Promise(resolve => setTimeout(resolve, 0));
    active--; completed++;
  });
  assert.equal(peak, 2); assert.equal(completed, 500);
});
test('switching context stops queued work after the active pair', async () => {
  let current = true, started = 0;
  await runQueue(Array(500).fill(0), 2, () => current, async () => {
    started++; await new Promise(resolve => setTimeout(resolve, 0)); current = false;
  });
  assert.equal(started, 2);
});
test('resolve only engine-confirmed unlearned actions and preserve other validation errors', () => {
  const apl = {priorityList: [{action:{name:'learned'}},{action:{name:'unlearned'}},{action:{name:'invalid'}}], prepullActions: []};
  const resolved = learnedRotation(apl, {priorityList:[{warnings:[]},{warnings:['Player does not know spell {SpellID: 1}']},{warnings:['Invalid condition']} ]});
  assert.deepEqual(resolved.priorityList.map(x=>x.action.name), ['learned','invalid']);
  assert.equal(apl.priorityList.length, 3);
});
test('unselected talent auras are unavailable; unrelated errors still fail validation', () => {
  const apl = {priorityList: [{action:{}},{action:{}}]};
  const resolved = learnedRotation(apl, {priorityList:[
    {warnings:['No aura found on Synthetic player for: {SpellID: 1}', 'Synthetic player does not know spell {SpellID: 1}']},
    {warnings:['No aura found on Synthetic player for: {SpellID: 2}', 'Invalid condition']},
  ]});
  assert.equal(resolved.priorityList.length, 1);
});
