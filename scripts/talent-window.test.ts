import test from 'node:test';
import assert from 'node:assert/strict';
import { Class } from '../ui/core/proto/common';
import { ForeverMode,ForeverOptions } from '../ui/core/proto/api';
import {foreverDiscoveryTalents as data,foreverBuildErrors,sampleForeverBuild,effectiveForeverRank} from '../ui/forever/discovery';
import {classRecords,treeNames,rankChange,encodeTalents,decodeTalents,talentURL,readTalentURL} from '../ui/forever/talent-build';
import art from '../ui/forever/data/talent-presentation.json';

for(const cls of Object.values(Class).filter((v):v is Class=>typeof v==='number'&&v!==Class.ClassUnknown)){
 for(const tree of treeNames(cls))for(const mode of [ForeverMode.STRICT,ForeverMode.BEST_GUESS]){
  test(`${Class[cls]} / ${tree} / ${ForeverMode[mode]}: allocation, gates, artwork and share roundtrip`,()=>{
   const records=classRecords(cls),build=sampleForeverBuild(cls,tree),f=ForeverOptions.create({talents:build,mode});
   assert.deepEqual(foreverBuildErrors(cls,build),[]);assert.equal(Object.values(build).reduce((a,b)=>a+b,0),51);
   const url=talentURL('https://example.test/forever-sim/druid/?keep=yes#simulator-data',cls,build,mode),decoded=readTalentURL(url,cls)!;
   assert.deepEqual(decoded.talents,build);assert.equal(decoded.mode,mode);assert.equal(new URL(url).hash,'');assert.equal(new URL(url).searchParams.get('keep'),'yes');
   assert.deepEqual(decodeTalents(cls,encodeTalents(cls,build)),build);
   const positions=new Set<string>();
   for(const r of records.filter(r=>r.tree===tree)){
    const pos=`${r.row}:${r.column}`;assert(!positions.has(pos));positions.add(pos);assert(r.column>=1&&r.column<=4);
    assert(art.talents[r.id as keyof typeof art.talents]?.icon);assert.equal(r.ranks.length,r.max_rank);
    const active=effectiveForeverRank(f,r);assert(active<=(build[r.id]||0));
    if(mode===ForeverMode.STRICT&&active){assert(!r.ranks[active-1].estimated);assert.notEqual(r.ranks[active-1].confidence,'PREDICTED');assert.notEqual(r.confidence,'PREDICTED');}
    if(mode===ForeverMode.BEST_GUESS&&!['blocked','non-sim'].includes(r.mode))assert.equal(active,build[r.id]||0);
    if(r.required_points||r.prerequisites.length)assert(rankChange(cls,{},r,1).errors.length>0);
    if(build[r.id]){const removed=rankChange(cls,build,r,-1);assert.equal(removed.errors.length>0,foreverBuildErrors(cls,{...build,[r.id]:build[r.id]-1}).length>0);}
    assert(rankChange(cls,{[r.id]:r.max_rank},r,1).errors.length);assert(rankChange(cls,{},r,-1).errors.length);
    for(const p of r.prerequisites){assert(records.some(parent=>parent.id===p.id));const broken={...build,[r.id]:1,[p.id]:0};assert(foreverBuildErrors(cls,broken).some(e=>e.includes('prerequisite')));}
   }
   assert(art.trees[records[0].class as keyof typeof art.trees].some(t=>t.name===tree&&t.background.startsWith('https://wow.zamimg.com/')));
  });
 }
}
test('all 470 talents / 27 trees covered; malformed imports leave caller state intact',()=>{
 assert.equal(data.records.length,470);assert.equal(new Set(data.records.map(r=>r.class+':'+r.tree)).size,27);
 const cls=Class.ClassWarrior,build=sampleForeverBuild(cls,'Arms'),original=structuredClone(build),valid=encodeTalents(cls,build);
 for(const bad of ['',valid.replace('F1:','F9:'),valid.replace('WARRIOR','DRUID'),valid+'0',valid.replace(/:[0-5]/,':9'),'000-000-000'])assert.throws(()=>decodeTalents(cls,bad));
 assert.deepEqual(build,original);assert.throws(()=>readTalentURL('https://example.test/?ft='+encodeURIComponent(valid)+'&fm=INVALID',cls));
});
test('row-support removal and dependent prerequisite removal are rejected atomically',()=>{
 const cls=Class.ClassWarrior,records=classRecords(cls);let foundRow=false,foundPrereq=false;
 for(const tree of treeNames(cls)){
  let build=sampleForeverBuild(cls,tree);
  for(let pass=0;pass<51;pass++)for(const r of records){if(!build[r.id])continue;const result=rankChange(cls,build,r,-1);if(!result.errors.length)build=result.talents;else{foundRow ||= result.errors.some(e=>e.includes('earlier rows'));foundPrereq ||= result.errors.some(e=>e.includes('prerequisite'));}}
 }
 assert(foundRow);assert(foundPrereq);
});
