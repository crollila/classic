import test from 'node:test';
import assert from 'node:assert/strict';
import { optimize, neighbors, canonical, cacheIdentity, safeAPL } from '../ui/forever/optimizer.mjs';
import { explainRotation } from '../ui/forever/optimizer-explain.mjs';
// Manufactured IDs and deterministic DPS below are algorithm tests, not game mechanics.
const apl={type:'TypeAPL',priorityList:[{action:{castSpell:{spellId:{spellId:1}}}},{action:{castSpell:{spellId:{spellId:2}}}}]};
const request={raid:{parties:[{players:[{class:'ClassWarrior',forever:{mode:'STRICT'},rotation:apl}]}]},encounter:{duration:60,targets:[{level:63}]}};
const pins={engine:'synthetic-engine',mechanics:'synthetic-mechanics'};
const make=score=>({validate:async()=>({valid:true,metadata:{spells:[]}}),run:async r=>({iterationsDone:r.simOptions.iterations,raidMetrics:{parties:[{players:[{dps:{avg:score(r)}}]}]}})});
const args=backend=>({request,pins,backend,seed:100,policy:{candidates:8,rounds:2,trainIterations:8,blockIterations:8}});
test('paired common seeds, disjoint holdout and reproducibility',async()=>{
 const seen=[];const backend=make(r=>{seen.push(r);return 100+(r.raid.parties[0].players[0].rotation.priorityList[0].action.castSpell.spellId.spellId===2?20:0)+Number(r.simOptions.randomSeed)%7;});
 const result=await optimize(args(backend));assert.equal(result.status,'VALIDATED_IMPROVEMENT');assert.equal(result.improvement,20);assert.equal(result.confidence.standardError,0);
 assert.equal(result.apl.priorityList[0].action.castSpell.spellId.spellId,2);
 for(const seed of result.search.holdoutSeeds){assert.ok(seed>108);assert.equal(seen.filter(r=>Number(r.simOptions.randomSeed)===seed).length,2);}
 assert.deepEqual(await optimize(args(backend)),result);assert.equal(request.raid.parties[0].players[0].rotation,apl);
});
test('training RNG luck never promotes a losing held-out rotation',async()=>{
 const result=await optimize(args(make(r=>100+(r.raid.parties[0].players[0].rotation.priorityList[0].action.castSpell.spellId.spellId===2?(Number(r.simOptions.randomSeed)===100?30:-5):0))));
 assert.equal(result.status,'INCONCLUSIVE');assert.deepEqual(result.apl,apl);assert.equal(result.improvement,0);assert.equal(result.confidence.difference,-5);
});
test('invalid candidates never reach scoring; invalid baseline fails',async()=>{
 let runs=0;const backend=make(()=>{runs++;return 100;});backend.validate=async r=>({valid:canonical(r.raid.parties[0].players[0].rotation)===canonical(apl),warnings:['synthetic unsupported action']});
 const result=await optimize(args(backend));assert.ok(result.search.rejected>0);assert.equal(runs,33);
 backend.validate=async()=>({valid:false,warnings:['unknown spell']});await assert.rejects(optimize(args(backend)),/unknown spell/);
});
test('partial, nonfinite and cancelled results fail closed',async()=>{
 for(const avg of [NaN,Infinity])await assert.rejects(optimize(args(make(()=>avg))),/nonfinite/);
 const backend=make(()=>100);backend.run=async()=>({iterationsDone:0});await assert.rejects(optimize(args(backend)),/Incomplete/);
 await assert.rejects(optimize({...args(make(()=>100)),signal:{aborted:true}}),/cancelled/);
});
test('protobuf zero DPS is valid and empty trailing raid slots are ignored',async()=>{
 const r=structuredClone(request);r.raid.parties.push({}, {players:[{},{}]});r.raid.parties[0].players.push({});
 const backend=make(()=>0);backend.run=async r=>({iterationsDone:r.simOptions.iterations,raidMetrics:{parties:[{players:[{dps:{}}]}]}});
 const result=await optimize({...args(backend),request:r});assert.equal(result.currentDps,0);assert.equal(result.status,'INCONCLUSIVE');
});
test('cache includes every setup input, templates, mechanics and policy',async()=>{
 const key=cacheIdentity(request,pins,[],{});
 for(const field of ['equipment','talentsString','race','buffs','consumes','itemSwap']){const r=structuredClone(request);r.raid.parties[0].players[0][field]={changed:true};assert.notEqual(cacheIdentity(r,pins,[],{}),key);}
 assert.notEqual(cacheIdentity(request,{...pins,mechanics:'new'},[],{}),key);
 assert.notEqual(cacheIdentity(request,pins,[apl],{}),key);
 assert.notEqual(cacheIdentity(request,pins,[],{blockIterations:100}),key);
 const cache=new Map(),a=args(make(()=>100));await optimize({...a,cache});a.backend.run=async()=>{throw Error('cache miss');};assert.ok(await optimize({...a,cache}));
});
test('mutations preserve IDs and cannot inject developer-only actions',()=>{
 const conditional=structuredClone(apl);conditional.priorityList[0].action.condition={cmp:{op:'OpGe',lhs:{currentRage:{}},rhs:{const:{val:'40'}}}};
 const ns=[...neighbors(conditional,{spells:[]},'ClassWarrior')];assert.ok(ns.some(n=>JSON.stringify(n).includes('46')));
 for(const n of ns){assert.ok(safeAPL(n));assert.deepEqual(n.priorityList.map(r=>r.action.castSpell.spellId.spellId).sort(),[1,2]);}
 assert.equal(safeAPL({priorityList:[{action:{addComboPoints:{}}}]}),false);
 const explanation=explainRotation(conditional,{[canonical({spellId:1})]:'Synthetic Strike'});assert.match(explanation.priority[0],/Synthetic Strike.*Current Rage ≥ 40/);
});
