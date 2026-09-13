// Reconciles every manifest record; tree import alone is not combat completion.
import {readFileSync,writeFileSync} from 'node:fs';
import assert from 'node:assert/strict';
const manifest=JSON.parse(readFileSync('../forever_changes.json'));
const data=JSON.parse(readFileSync('sim/core/foreverdata/trees.json'));
const byId=new Map(data.records.map(r=>[r.id,r]));
const reachable=new Set();
for(let pass=0;pass<20;pass++)for(const r of data.records){
 if(r.mode==='blocked'||!r.prerequisites.every(p=>reachable.has(p.id)))continue;
 const lower=data.records.filter(t=>t.class===r.class&&t.tree===r.tree&&t.row<r.row&&reachable.has(t.id)).reduce((n,t)=>n+t.max_rank,0);
 if(lower>=r.required_points)reachable.add(r.id);
}
const extras=new Set(data.mechanics.map(m=>m.id));
const coreImplemented=new Set(['core.level-cap','core.raid-sizes','core.talent-point-budget','core.classic-compatibility']);
const statuses=[];
const special={
 'core.raid-sizes':'Discovery target aura registration capacity scales with raid size; regression covers 10, 20 and 40 players. This is an internal allocation guard, not the in-game debuff limit.',
 'buffs.first-aid-kit':'Preapplied +34 Stamina scenario, excluded by Fortitude. Camping, duration and shared cooldown are not simulated.',
 'racials.gnome.expansive-mind':'Replaces Intellect with 5% maximum Mana/Rage/Energy; multiplicative capacity stacking is provisional.',
 'racials.troll.berserking':'All existing tagged variants use 10%; Classic costs, cooldown and class-dependent haste formula retained provisionally.',
 'racials.dwarf.big-game-hunter':'5% Beast damage; retains Classic Beast Slaying crit interaction provisionally.',
 'warlock.talent.demonic-sacrifice':'Observed 2-hour duration and pet/effect mapping; original cancellation and periodic tick scheduling retained. Incubus not selectable in baseline pet catalogue.',
 'hunter.talent.strider-kick':'Observed 61 mana, 8-second cooldown and 100% main-hand weapon damage; Classic melee outcome and threat retained.',
 'shaman.talent.rage-of-the-farseer':'Self-only 30% melee/cast speed, 25 seconds, 3-minute cooldown. No ranged or party haste.',
 'warrior.talent.enrage':'30% on direct damaging hits taken, 12 seconds, no hit charges; periodic-hit eligibility remains provisional.',
};
for(const [category,records] of Object.entries(manifest.sections))for(const record of records){
 let status='BLOCKED',scope='No reviewed executable adapter in this batch.';
 const talent=byId.get(record.id);
 if(talent){
  if(talent.mode!=='blocked'){
   if(reachable.has(record.id)){status='PROVISIONAL';scope=talent.mode==='classic'?'Reviewed Classic effect reached through the Forever tree. Existing engine limitations remain.':'Known numerical effect implemented; Classic-equivalent interactions remain provisional.'}
   else scope='Adapter code exists, but required lower-row points or prerequisite talents need unfinished adapters. Not counted complete.';
  }
 }else if(record.kind==='removed_talent'){
  status='PROVISIONAL';scope='Absent from Forever selection; old typed field stays zero. Does not claim the underlying ability was removed from the game.';
 }else if(coreImplemented.has(record.id)){
  status='IMPLEMENTED';scope={
   'core.level-cap':'Existing level-60 cap retained and tested for discovery characters.',
   'core.raid-sizes':'The announced raid sizes are supported by the discovery request path.',
   'core.talent-point-budget':'51-point budget, lower-row investment and prerequisites validated before simulation.',
   'core.classic-compatibility':'Versioned per-request talent mapping, explicit ID provenance and Classic/discovery isolation implemented.'
  }[record.id];
 }else if(record.kind==='race_class_combinations'&&!record.id.includes('skyborne')){
  status='PROVISIONAL';scope='Race/class combination validated from manifest. Existing Classic base-stat construction retained provisionally.';
 }else if(extras.has(record.id)){
  status='PROVISIONAL';scope='Explicit opt-in racial or preapplied buff adapter. Omitted racials retain Classic behavior; omissions are not presumed removals.';
 }else if(record.kind==='legacy_perk'||record.id==='professions.slot-count'){
  status='BLOCKED';scope='Known source details retained. No reviewed adapter for this perk or profession rule in the current raid model; this is unfinished modeling, not missing source evidence.';
 }else if(record.confidence==='UNVERIFIED'||record.can_implement_immediately===false||record.id.startsWith('core.unresolved.')||['ITEMS','PROFESSIONS','CONSUMES'].includes(category)||record.id==='core.skyborne-base-stats'){
  status='UNKNOWN';scope='Insufficient supported numerical/behavioral evidence for an executable change; no new effect assumed.';
 }
 if(special[record.id])scope+=' '+special[record.id];
 let implementationFiles=[];
 if(talent&&talent.mode!=='blocked')implementationFiles=talent.mode==='replace'?['sim/core/forever.go']:['sim/core/foreverdata/data.go',...record.simulator_files_likely_affected.filter(f=>f.endsWith('.go')).map(f=>f.replace(/^classic\//,''))];
 else if(record.kind==='removed_talent'||record.kind==='race_class_combinations'||record.id==='core.talent-point-budget')implementationFiles=['sim/core/foreverdata/data.go'];
 else if(record.id==='core.level-cap')implementationFiles=['sim/core/constants.go'];
 else if(record.id==='core.raid-sizes')implementationFiles=['sim/core/environment.go','sim/core/aura.go','sim/core/forever_aura_test.go','sim/game/discovery_test.go'];
 else if(record.id==='core.classic-compatibility')implementationFiles=['sim/game/game.go','sim/core/agent.go','sim/core/foreverdata/data.go','proto/api.proto'];
 else if(record.id==='buffs.first-aid-kit')implementationFiles=['sim/core/forever.go'];
 else if(extras.has(record.id))implementationFiles=['sim/core/racials.go','sim/core/forever_racials.go'];
 const entry={id:record.id,category,status,source_confidence:record.confidence,scope,source_url:record.source_url,
  adapter_mode:talent?.mode||null,tree_imported:!!talent,reachable_with_experimental_ranks:talent?reachable.has(record.id):null,
  known_ranks:talent?.ranks.filter(r=>!r.estimated).map(r=>r.rank)||[],experimental_ranks:talent?.ranks.filter(r=>r.estimated).map(r=>r.rank)||[],
  unknowns:record.what_remains_unknown||[],
  implementation_files:[...new Set(implementationFiles)],
  candidate_files:record.simulator_files_likely_affected,
 };
 statuses.push(entry);
}
assert.equal(statuses.length,701);assert.equal(new Set(statuses.map(r=>r.id)).size,701);
const summarize=rows=>{
 const result={TOTAL:rows.length,IMPLEMENTED:0,PROVISIONAL:0,BLOCKED:0,UNKNOWN:0};for(const r of rows)result[r.status]++;
 result.completion_percent=Number((100*(result.IMPLEMENTED+result.PROVISIONAL)/result.TOTAL).toFixed(1));return result;
};
const summary=summarize(statuses);
const categories=Object.fromEntries(Object.keys(manifest.sections).map(k=>[k,summarize(statuses.filter(s=>s.category===k))]));
const report={schema_version:1,ruleset_id:data.ruleset_id,manifest_sha256:data.manifest_sha256,definition:'Completion = (IMPLEMENTED + PROVISIONAL) / all category records. Only reachable combat adapters and explicitly described structural changes count. Experimental ranks require explicit opt-in.',
 summary,categories,records:statuses};
writeFileSync('sim/forever/implementation-status.json',JSON.stringify(report,null,2)+'\n');
const lines=['# Forever implementation status','',`Manifest: 701 records; SHA-256 \`${data.manifest_sha256}\`.`,`Ruleset: \`${data.ruleset_id}\`. Branch: \`forever/mechanics-2026-09-13\`.`,
 '', '**This is a partial discovery ruleset. It is not a complete Forever simulation.**',
 '', 'IMPLEMENTED means the stated scope is complete. PROVISIONAL means usable observed/known behavior with explicit Classic interaction fallbacks. BLOCKED means an adapter, prerequisite, or engine capability remains unfinished. UNKNOWN means supported information is insufficient. Source confidence is separate from implementation status.',
 '', 'Completion uses every record in a category, including removed talents and unresolved abilities. A talent counts only if it is reachable through supported prerequisites and lower rows, including explicitly experimental rank values where necessary. A tree entry alone does not count. Full supported ranks, estimated flags, source URLs and positions are preserved in the versioned data.',
 '', `All 470 talents in 27 trees are imported. ${data.records.filter(r=>r.mode!=='blocked').length} talent adapters/bridges exist; ${reachable.size} are reachable with experimental ranks enabled. All 81 Classic-only talent selections are excluded. The original Classic trees and regression results are untouched.`,
 '', '| Category | Total | Implemented | Provisional | Blocked | Unknown | Completion |','|---|---:|---:|---:|---:|---:|---:|'];
for(const [cat,s]of Object.entries(categories))lines.push(`| ${cat} | ${s.TOTAL} | ${s.IMPLEMENTED} | ${s.PROVISIONAL} | ${s.BLOCKED} | ${s.UNKNOWN} | ${s.completion_percent}% |`);
lines.push(`| TOTAL | ${summary.TOTAL} | ${summary.IMPLEMENTED} | ${summary.PROVISIONAL} | ${summary.BLOCKED} | ${summary.UNKNOWN} | ${summary.completion_percent}% |`,'',
 '## Implementation and verification','',
 'Run instructions, transport compatibility, fallback policies and known limitations: [FOREVER_DISCOVERY.md](classic/docs/FOREVER_DISCOVERY.md). Machine-readable reconciliation: [implementation-status.json](classic/sim/forever/implementation-status.json).',
 '', 'The original browser talent picker still selects Classic builds. The new tree dataset and request builder are available to UI integrations; discovery requests currently run through the CLI or raw API/worker payloads. No deployment was performed.',
 '', 'Test execution evidence is recorded in [discovery-validation.json](classic/sim/forever/discovery-validation.json). The full simulator suite is required after changes; passing the baseline suite does not confirm undocumented interactions.',
 '', '## Record reconciliation','');
for(const cat of Object.keys(categories)){
 lines.push(`### ${cat}`,'','| Record | Status | Source confidence | Scope |','|---|---|---|---|');
 for(const r of statuses.filter(s=>s.category===cat)){
  const ranks=r.known_ranks.length?` Known ranks: ${r.known_ranks.join(', ')}. Experimental: ${r.experimental_ranks.join(', ')||'none'}.`:'';
  lines.push(`| [${r.id}](${r.source_url}) | ${r.status} | ${r.source_confidence} | ${(r.scope+ranks).replaceAll('|','/').replaceAll('\n',' ')} |`);
 }
 lines.push('');
}
writeFileSync('../FOREVER_IMPLEMENTATION_STATUS.md',lines.join('\n')+'\n');
console.log(JSON.stringify({summary,categories},null,2));
