import test from 'node:test';
import assert from 'node:assert/strict';
import {foreverDiscoveryTalents as data,sampleForeverBuild,foreverBuildErrors,effectiveForeverRank,foreverDefaultOptions,foreverRaces} from '../ui/forever/discovery';
import {withForeverRotation} from '../ui/forever/rotation';
import {Class,Spec} from '../ui/core/proto/common';
import {ForeverOptions,ForeverMode,SpellStats} from '../ui/core/proto/api';
import {APLRotation,APLRotation_Type} from '../ui/core/proto/apl';
test('all 27 browser sample trees form legal 51-point builds',()=>{
 let trees=0;
 for(const name of [...new Set(data.records.map(r=>r.class))]){
  const cls=Object.values(Class).find(v=>typeof v==='number'&&Class[v].toUpperCase()==='CLASS'+name) as Class;
  for(const tree of [...new Set(data.records.filter(r=>r.class===name).map(r=>r.tree))]){
   const talents=sampleForeverBuild(cls,tree);assert.deepEqual(foreverBuildErrors(cls,talents),[],name+tree);assert.equal(Object.values(talents).reduce((a,b)=>a+b,0),51,name+tree);trees++;
   const f=ForeverOptions.create({talents,mode:ForeverMode.STRICT});
   for(const r of data.records.filter(r=>r.class===name)){const n=effectiveForeverRank(f,r);assert(n<=(talents[r.id]||0));if(n){assert.equal(r.ranks[n-1].estimated,false);assert.notEqual(r.confidence,'PREDICTED')}}
  }
  for(const race of foreverRaces(cls)){const f=foreverDefaultOptions(cls,race);assert.equal(f.mode,ForeverMode.BEST_GUESS);assert(f.mechanics.every(id=>data.mechanics.some(m=>m.id===id&&m.mode!=='blocked')))}
 }
 assert.equal(trees,27);
});
test('healer browser rotations contain available Classic heals with friendly targets',()=>{
 for(const [spec,cls,id]of [[Spec.SpecHealingPriest,Class.ClassPriest,10965],[Spec.SpecHolyPaladin,Class.ClassPaladin,25292],[Spec.SpecRestorationShaman,Class.ClassShaman,25357],[Spec.SpecRestorationDruid,Class.ClassDruid,25297]]){
  const f=ForeverOptions.create({mode:ForeverMode.BEST_GUESS}),spells=[SpellStats.fromJson({id:{spellId:id},isCastable:true})];
  const result=withForeverRotation(APLRotation.create(),f,cls,spells,spec);
  assert.equal(result.type,APLRotation_Type.TypeAPL);const json=APLRotation.toJson(result) as any;assert.equal(json.priorityList[0].action.castSpell.spellId.spellId,id);assert.equal(json.priorityList[0].action.castSpell.target.type,'Self');
 }
});
test('saved selections survive mode switching while estimated effects are suppressed',()=>{
 const r=data.records.find(r=>r.id==='warrior.talent.improved-heroic-strike')!;
 const f=ForeverOptions.create({talents:{[r.id]:2},mode:ForeverMode.BEST_GUESS});assert.equal(effectiveForeverRank(f,r),2);f.mode=ForeverMode.STRICT;assert.equal(effectiveForeverRank(f,r),1);assert.equal(f.talents[r.id],2);
});
