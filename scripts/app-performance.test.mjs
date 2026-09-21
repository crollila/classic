import test from 'node:test';
import assert from 'node:assert/strict';
import {runQueue} from '../ui/app/work-queue.mjs';
import {learnedRotation} from '../ui/app/rotation-context.mjs';

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
