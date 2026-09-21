import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {scenarioMechanics} from '../ui/app/scenario-mechanics.mjs';
const mechanics=JSON.parse(readFileSync(new URL('../ui/forever/data/talents.json',import.meta.url))).mechanics;
test('all mechanic Off values are omitted, including camping and saved stale values',()=>{
 const state={mechanicRanks:Object.fromEntries(mechanics.map(m=>[m.id,0])),parameters:{}};
 assert.deepEqual(scenarioMechanics(state,mechanics).ranks,{});
 assert.equal(state.mechanicRanks['buffs.camping'],0);
});
test('every executable mechanic has a valid bounded integer rank and only active parameters',()=>{
 for(const m of mechanics.filter(m=>!['blocked','non-sim','baseline'].includes(m.mode))){
  for(const rank of [1,m.max_rank,999,1.5]){
   const out=scenarioMechanics({mechanicRanks:{[m.id]:rank},parameters:{unknown:999}},mechanics);
   assert.ok(Number.isInteger(out.ranks[m.id])&&out.ranks[m.id]>=1&&out.ranks[m.id]<=m.max_rank,m.id);
   assert.deepEqual(out.parameters,{});
  }
 }
 assert.deepEqual(scenarioMechanics({mechanicRanks:{obsolete:1,'buffs.camping':NaN}},mechanics).ranks,{});
});
