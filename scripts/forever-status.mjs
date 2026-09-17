// This ledger documents executable scopes; tests provide independent execution evidence.
import {readFileSync,writeFileSync,existsSync} from 'node:fs';
import assert from 'node:assert/strict';
const manifest=JSON.parse(readFileSync('../forever_changes.json'));
const data=JSON.parse(readFileSync('sim/core/foreverdata/trees.json'));
const compiled=new Map([...data.records,...data.mechanics].map(r=>[r.id,r]));
const previous=JSON.parse(readFileSync('sim/forever/blocked-audit-baseline.json'));
const old=new Map(previous.records.map(r=>[r.id,r]));
const records=[];
for(const [category,rows]of Object.entries(manifest.sections))for(const source of rows){
 const r=compiled.get(source.id);assert(r,source.id);
 const a=r.adapter,removed=source.kind==='removed_talent',mode=removed?'removed':r.mode;
 const state=mode==='blocked'?'BLOCKED':mode==='non-sim'?'NON-SIM-RELEVANT':'ACTIVE';
 const files=removed?['sim/core/foreverdata/data.go','ui/forever/talents_picker.ts']:a?.files?.length?a.files:mode==='replace'?['sim/core/forever.go']:['sim/core/foreverdata/data.go',...source.simulator_files_likely_affected.filter(f=>f.endsWith('.go')).map(f=>f.replace(/^classic\//,''))];
 if(state==='ACTIVE')assert(files.some(f=>existsSync(f)),`No executable path for ${r.id}`);
 const predictedRanks=r.ranks?.filter(v=>v.estimated||v.confidence==='PREDICTED').map(v=>v.rank)||[];
 const predictedComponents=a?.predicted_components||[];
 const confidence=state==='ACTIVE'?(r.confidence==='PREDICTED'||predictedRanks.length||predictedComponents.length?'PREDICTED':removed?'PROVISIONAL':r.confidence):r.confidence;
 const scope=removed?'Excluded from Forever selection and semantic Classic bridge; its former talent field remains zero. This removes the talent bonus, not an unsupported assumption that the underlying ability vanished.':a?.scope||a?.reason||(mode==='classic'?'Executable Classic numerical effect selected by its Forever semantic talent ID.':'Executed by the class/core adapter with Classic-equivalent interactions where undocumented.');
 const reason=a?.reason||scope;
 const entry={id:r.id,category,name:r.name,implementation_state:state,confidence,source_confidence:source.confidence,adapter_mode:mode,
  scope,reason,implementation_files:[...new Set(files)],source_url:source.source_url,
  observed_ranks:r.ranks?.filter(v=>!v.estimated).map(v=>v.rank)||[],predicted_ranks:predictedRanks,predicted_components:predictedComponents,
  strict_policy:r.confidence==='PREDICTED'?'Disabled; explicitly predicted adapter.':predictedRanks.length?'Use highest observed rank at or below selection; predicted secondary components are also suppressed.':predictedComponents.length?'Observed effect remains active; predicted components are suppressed (see component-specific scope).':'Observed numerical effect with safe Classic interaction fallback.',
  remaining_unknowns:[...new Set([...(source.what_remains_unknown||[]),...(a?.remaining_unknowns||[])])],previous_state:old.get(r.id)?.status||'UNKNOWN'};
 if(entry.previous_state==='BLOCKED')entry.blocked_audit={previous_reason:old.get(r.id).scope,decision:state,resolution:state==='ACTIVE'?`${mode} implementation: ${reason}`:reason,engine_extension_considered:true,classic_analogue_considered:true,prediction_considered:true};
 records.push(entry);
}
assert.equal(records.length,701);assert.equal(new Set(records.map(r=>r.id)).size,701);assert.equal(records.filter(r=>r.previous_state==='BLOCKED').length,432);
function summarize(rows){const s={TOTAL:rows.length,ACTIVE:0,BLOCKED:0,'NON-SIM-RELEVANT':0,CONFIRMED:0,PROVISIONAL:0,PREDICTED:0};for(const r of rows){s[r.implementation_state]++;if(r.implementation_state==='ACTIVE')s[r.confidence]++;}s.completion_percent=+(100*s.ACTIVE/s.TOTAL).toFixed(1);s.active_sim_relevant_percent=+(100*s.ACTIVE/(s.TOTAL-s['NON-SIM-RELEVANT']||1)).toFixed(1);return s;}
const summary=summarize(records),categories=Object.fromEntries(Object.keys(manifest.sections).map(c=>[c,summarize(records.filter(r=>r.category===c))]));
const validation=JSON.parse(readFileSync('sim/forever/discovery-validation.json'));
const report={schema_version:2,ruleset_id:data.ruleset_id,manifest_sha256:data.manifest_sha256,
 definition:'ACTIVE means an executable effect, constraint, removed bonus, or explicitly retained Classic behavior when applicable. It does not mean enabled in every build. Confidence is separate. Record confidence is PREDICTED if any supported rank/adapter is predicted; observed subsets may still execute in STRICT. Non-combat records do not count as active. Tests are separate execution evidence.',summary,categories,validation,records};
writeFileSync('sim/forever/implementation-status.json',JSON.stringify(report,null,2)+'\n');
const lines=['# Forever implementation status','',`701 records · ruleset \`${data.ruleset_id}\` · manifest SHA-256 \`${data.manifest_sha256}\`.`,
 '',report.definition,'','BEST_GUESS is the player-facing planning default. STRICT excludes predicted adapters and downgrades estimated selections to observed ranks. Unknown interactions retain the nearest supported Classic behavior and remain provisional. Skyborne base stats require BEST_GUESS.',
 '', '| Category | Active / total | Blocked | Non-sim | Confirmed | Provisional | Predicted | Active / all |','|---|---:|---:|---:|---:|---:|---:|---:|'];
for(const [c,s]of Object.entries({...categories,TOTAL:summary}))lines.push(`| ${c} | ${s.ACTIVE} / ${s.TOTAL} | ${s.BLOCKED} | ${s['NON-SIM-RELEVANT']} | ${s.CONFIRMED} | ${s.PROVISIONAL} | ${s.PREDICTED} | ${s.completion_percent}% |`);
lines.push('','## Remaining blocked records','');
for(const r of records.filter(r=>r.implementation_state==='BLOCKED'))lines.push(`- **${r.id}**: ${r.reason} [Source](${r.source_url})`);
lines.push('','## Verification and use','','See [validation evidence](classic/sim/forever/discovery-validation.json), [mode/transport documentation](classic/docs/FOREVER_DISCOVERY.md), and [machine-readable audit](classic/sim/forever/implementation-status.json). Validation reflects actual commands and browser checks; it does not confirm undocumented interactions.',
 '',`All ${records.filter(r=>r.previous_state==='BLOCKED').length} previously blocked records have an individual audit below. ${data.records.length} talents retain their source values plus explicit per-rank predictions and corrections.`,
 '', '## Individual record audit','');
for(const category of Object.keys(categories)){
 lines.push(`### ${category}`,'','| Record | Implementation | Confidence | Scope / audit resolution |','|---|---|---|---|');
 for(const r of records.filter(r=>r.category===category))lines.push(`| [${r.id}](${r.source_url}) | ${r.implementation_state} | ${r.confidence} | ${(r.scope+(r.blocked_audit?' Previous BLOCKED audit: '+r.blocked_audit.resolution:'')+(r.predicted_ranks.length?' Predicted ranks: '+r.predicted_ranks.join(', ')+'.':'')).replaceAll('|','/').replaceAll('\n',' ')} |`);
 lines.push('');
}
writeFileSync('../FOREVER_IMPLEMENTATION_STATUS.md',lines.join('\n')+'\n');
console.log(JSON.stringify({summary,categories},null,2));
