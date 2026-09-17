import type { SimUI } from '../core/sim_ui';
import './optimizer.css';
import type { IndividualSimUI } from '../core/individual_sim_ui';
import { Spec, ActionID } from '../core/proto/common';
import { APLRotation } from '../core/proto/apl';
import { ComputeStatsRequest, RaidSimRequest, RaidSimResult, ForeverMode } from '../core/proto/api';
import { WorkerPool } from '../core/worker_pool';
import { SimRequest } from '../worker/types';
import { TypedEvent } from '../core/typed_event';
import { ActionId } from '../core/proto_utils/action_id';
import { optimize, canonical, cacheIdentity, defaults } from './optimizer.mjs';
import { explainRotation } from './optimizer-explain.mjs';
import { el, Release } from './metadata';
import { foreverDiscoveryTalents as data, effectiveForeverRank, isExecutable } from './discovery';

const cache=new Map();
export function mountOptimizer(ui:SimUI,release:Release) {
  const individual=ui as IndividualSimUI<Spec>,player=individual.player;
  if(!player)return;
  const panel=el('section','forever-optimizer'), button=el('button','btn btn-primary','FIND IDEAL ROTATION');
  const cancel=el('button','btn btn-secondary','Cancel'),output=el('div'),status=el('p');
  status.setAttribute('role','status');output.setAttribute('aria-live','polite');cancel.hidden=true;
  panel.append(button,cancel,el('p','','Uses the STRICT / BEST_GUESS mode selected in Talents. Searches up to 90 candidates, then validates on 2,048 unseen-seed iterations per rotation.'),status,output);ui.simContentContainer.prepend(panel);
  let controller:AbortController|undefined,pool:WorkerPool|undefined;
  cancel.onclick=()=>controller?.abort();
  ui.addOnDisposeCallback(()=>controller?.abort());
  button.onclick=async()=>{
    button.disabled=true;cancel.hidden=false;output.replaceChildren();controller=new AbortController();
    try {
      await ui.sim.waitForInit();pool ||=new WorkerPool(1);
      const request=ui.sim.makeRaidSimRequest(false),json=RaidSimRequest.toJson(request) as any;
      if(!release.engineSha256||!release.databaseSha256)throw Error('Version fingerprints unavailable; rebuild the simulator before optimization.');
      const pins={engine:release.engineSha256,database:release.databaseSha256,mechanics:data.manifest_sha256,version:release.simulatorVersion};
      const templates=individual.individualConfig.presets.rotations.map((r:any)=>r.rotation?.rotation).filter(Boolean).map((r:any)=>APLRotation.toJson(r));
      const identity=cacheIdentity(json,pins,templates,defaults);
      const options=request.raid!.parties[0].players[0].forever;
      const mechanics=options?[
        ...data.records.filter(r=>effectiveForeverRank(options,r)>0).map(r=>({id:r.id,name:r.name,confidence:r.confidence,rank:effectiveForeverRank(options,r),rankEvidence:r.ranks[effectiveForeverRank(options,r)-1],predictedComponents:options.mode===ForeverMode.STRICT?[]:r.adapter?.predicted_components||[]})),
        ...data.mechanics.filter(m=>options.mechanics.includes(m.id)&&isExecutable(m.mode)&&(options.mode!==ForeverMode.STRICT||m.confidence!=='PREDICTED')).map(m=>({id:m.id,name:m.name,confidence:m.confidence,rankEvidence:undefined,predictedComponents:options.mode===ForeverMode.STRICT||(m.id.startsWith('legacy.')&&!(options.mechanicRanks[m.id]>1))?[]:m.adapter?.predicted_components||[]}))]:[];
      const backend={
        validate:async(r:any)=>{
          const result=await pool!.computeStats(ComputeStatsRequest.fromJson({raid:r.raid,encounter:r.encounter}));
          const p=result.raidStats?.parties[0]?.players[0];
          const warnings=[result.errorResult,...(p?.rotationStats?.prepullActions||[]).flatMap(x=>x.warnings),...(p?.rotationStats?.priorityList||[]).flatMap(x=>x.warnings)].filter(Boolean);
          return {valid:!!p&&!warnings.length,warnings,metadata:p?.metadata?JSON.parse(JSON.stringify((await import('../core/proto/api')).UnitMetadata.toJson(p.metadata))):{},provenance:mechanics};
        },
        run:async(r:any)=>RaidSimResult.toJson(RaidSimResult.fromBinary(await pool!.makeApiCall(SimRequest.raidSim,RaidSimRequest.toBinary(RaidSimRequest.fromJson(r))))) as any,
      };
      const seed=crypto.getRandomValues(new Uint32Array(1))[0]%1000000000+1;
      const result=await optimize({request:json,pins,templates,backend,seed,signal:controller.signal,cache,onProgress:(p:any)=>{status.textContent=p.phase==='search'?`Searching: ${p.evaluated} candidates (${p.rejected} rejected).`:`Validating unseen seeds: ${p.completed}/${p.total}.`;}});
      const names:Record<string,string>={};
      const ids:any[]=[];
      const collect=(x:any)=>{if(!x||typeof x!=='object')return;for(const [k,v] of Object.entries(x)){if((k==='spellId'||k==='auraId')&&typeof v==='object')ids.push(v);else collect(v);}};collect(result.apl);
      await Promise.all(ids.map(async id=>{try{const a=await ActionId.fromProto(ActionID.fromJson(id)).fill();names[canonical(id)]=a.name;}catch{/* Keep honest ID fallback. */}}));
      const explanation=explainRotation(result.apl,names);
      const predictions=mechanics.filter(m=>m.confidence==='PREDICTED'||m.predictedComponents.length||m.rankEvidence?.estimated||m.rankEvidence?.confidence==='PREDICTED');
      output.append(el('h3','','IDEAL ROTATION'),el('p','',`${result.mode} · ${result.status==='VALIDATED_IMPROVEMENT'?'Validated improvement':'No meaningful improvement established; current rotation retained'}`),
        el('p','',`Optimized DPS ${result.optimizedDps.toFixed(2)} · Current DPS ${result.currentDps.toFixed(2)} · Improvement ${result.improvement.toFixed(2)} (${result.improvementPercent.toFixed(2)}%)`),
        el('p','',`Finalist difference: ${result.confidence.difference.toFixed(2)} DPS; 99% confidence interval ${result.confidence.lower.toFixed(2)} to ${result.confidence.upper.toFixed(2)}. ${result.search.iterationsPerRotation} holdout iterations per rotation.`),el('p','',explanation.instructions));
      if(predictions.length)output.append(el('p','forever-disclosure',`PREDICTED mechanics contribute to this model: ${predictions.map(m=>m.name).join(', ')}. The DPS confidence interval does not validate these interactions.`));
      for(const section of ['opening','priority','cooldowns','execute'] as const){output.append(el('h4','',section.toUpperCase()));const list=el('ol');for(const line of explanation[section])list.append(el('li','',line));if(!list.children.length)list.append(el('li','','Follow the main priority; no separate action specified.'));output.append(list);}
      const details=el('details');details.append(el('summary','','Mechanics and APL evidence'),el('pre','',JSON.stringify({mechanics:result.mechanics,apl:result.apl,search:result.search,pins},null,2)));output.append(details,el('p','',result.limitations.join(' ')));
      const use=el('button','btn btn-primary','USE THIS ROTATION');use.disabled=result.status!=='VALIDATED_IMPROVEMENT';
      use.onclick=()=>{
        const now=RaidSimRequest.toJson(ui.sim.makeRaidSimRequest(false));
        if(cacheIdentity(now,pins,templates,defaults)!==identity){status.textContent='Setup changed. Run the optimizer again.';use.disabled=true;return;}
        TypedEvent.freezeAllAndDo(()=>{const f=player.getForever();if(f){f.parameters['rotation.forever_auto']=0;player.setForever(TypedEvent.nextEventID(),f);}player.setAplRotation(TypedEvent.nextEventID(),APLRotation.fromJson(result.apl));});
        use.disabled=true;status.textContent='Validated rotation applied.';
      };output.append(use);status.textContent='Optimization complete.';
    }catch(e){status.textContent=e instanceof Error?e.message:String(e);}
    finally{button.disabled=false;cancel.hidden=true;}
  };
}
