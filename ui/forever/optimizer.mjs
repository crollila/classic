// Simulation-only optimization of protobuf-JSON APLs. Shared by browser and God CLI.
export const VERSION = 'rotation-optimizer-1';
export const clone = x => JSON.parse(JSON.stringify(x));
export function canonical(x) {
  if (Array.isArray(x)) return `[${x.map(canonical).join(',')}]`;
  if (x && typeof x === 'object') return `{${Object.keys(x).sort().filter(k=>x[k]!==undefined).map(k=>JSON.stringify(k)+':'+canonical(x[k])).join(',')}}`;
  return JSON.stringify(x);
}
export const defaults = { beam: 3, rounds: 3, candidates: 90, trainIterations: 128, holdoutBlocks: 32, blockIterations: 64, minimumGain: 0 };
const player = r => r.raid.parties[0].players[0];
const cmp = (lhs, val, op='OpGe') => ({cmp:{op,lhs,rhs:{const:{val:String(val)}}}});
const and = (a,b) => a ? {and:{vals:[a,b]}} : b;
const casts = ['castSpell','channelSpell','multidot'];
const actionID = a => casts.map(k=>a?.[k]?.spellId).find(Boolean);
const idKey = id => canonical(id&&Object.fromEntries(Object.entries(id).filter(([k])=>k!=='rank')));
function walk(x, fn, path=[]) {
  if (!x || typeof x!=='object') return;
  fn(x,path);
  for(const [k,v] of Object.entries(x)) walk(v,fn,[...path,k]);
}
const at = (x,path) => path.reduce((a,k)=>a[k],x);
export function safeAPL(apl) {
  let safe = true;
  // These are debugging/prepull state injection operations, not player buttons.
  walk(apl,x=>{if(['activateAura','activateAuraWithStacks','triggerIcd','addComboPoints','customRotation'].some(k=>k in x))safe=false;});
  return safe && !!apl?.priorityList?.some(x=>x.action&&!x.hide);
}
export function* neighbors(apl, metadata, className) {
  const rows=apl.priorityList || [];
  for(let i=0;i<rows.length-1;i++) {
    const n=clone(apl); [n.priorityList[i],n.priorityList[i+1]]=[n.priorityList[i+1],n.priorityList[i]]; yield n;
  }
  const edits=[];
  walk(apl,(x,path)=>{
    if(x.const?.val && /^-?\d+(\.\d+)?(%|ms|s)?$/.test(x.const.val)) {
      const [,v,unit='']=x.const.val.match(/^(-?\d+(?:\.\d+)?)(%|ms|s)?$/);
      const num=Number(v), step=unit==='%'?5:unit==='s'?.5:unit==='ms'?100:Math.max(1,Math.abs(num)*.15);
      for(const value of [num-step,num+step]) {
        if((num>=0&&value<0)||(num<0&&value>=0)||(unit==='%'&&value>100))continue;
        edits.push([path,`${Number(value.toFixed(3))}${unit}`]);
      }
    }
  });
  for(const [path,val] of edits) {const n=clone(apl);at(n,path).const.val=val;yield n;}
  const conditions=rows.map(r=>r.action?.condition).filter(Boolean);
  const spells=metadata?.spells||[];
  for(let i=0;i<rows.length;i++) {
    const a=rows[i].action,id=actionID(a);if(!id||rows[i].hide)continue;
    const spell=spells.find(s=>idKey(s.id)===idKey(id));
    const gates=[cmp({currentTime:{}},'5s'),cmp({numberTargets:{}},2),{isExecutePhase:{threshold:'E20'}},{not:{val:{isExecutePhase:{threshold:'E20'}}}}];
    if(/Warrior|Druid/.test(className))gates.push(cmp({currentRage:{}},40));
    if(/Rogue|Druid/.test(className))gates.push(cmp({currentEnergy:{}},60));
    if(spell?.hasDot)gates.push(cmp({dotRemainingTime:{spellId:id}},'2s','OpLe'));
    if(spell?.isMajorCooldown)gates.push(...conditions.slice(0,8));
    if(spell?.isMajorCooldown)for(const s of spells.filter(s=>s.isMajorCooldown&&idKey(s.id)!==idKey(id)).slice(0,4))gates.push({spellIsReady:{spellId:s.id}});
    for(const aura of (metadata?.auras||[]).slice(0,8))gates.push({auraIsActive:{auraId:aura.id}});
    for(const gate of gates){const n=clone(apl);n.priorityList[i].action.condition=and(a.condition,gate);yield n;}
  }
  // Relax existing DoT refresh guards without discarding unrelated resource/proc gates.
  const dots=[];walk(apl,(x,path)=>{if(x.not?.val?.dotIsActive)dots.push([path,x.not.val.dotIsActive]);});
  for(const [path,dot] of dots)for(const seconds of [.5,1,2]){
    const n=clone(apl),node=at(n,path);delete node.not;
    Object.assign(node,cmp({dotRemainingTime:dot},`${seconds}s`,'OpLe'));yield n;
  }
  // Registered major cooldowns may be explicitly scheduled/aligned; never invent IDs.
  for(const s of spells.filter(s=>s.isCastable&&s.isMajorCooldown&&!s.prepullOnly)){
    if(rows.some(r=>idKey(actionID(r.action))===idKey(s.id)))continue;
    const n=clone(apl);n.priorityList.unshift({action:{castSpell:{spellId:s.id},condition:cmp({currentTime:{}},'5s')}});yield n;
  }
}
export function stats(values) {
  const mean=values.reduce((a,b)=>a+b,0)/values.length;
  const se=values.length>1?Math.sqrt(values.reduce((s,v)=>s+(v-mean)**2,0)/(values.length-1)/values.length):Infinity;
  return {mean,se};
}
export function cacheIdentity(request,pins,templates,policy) {
  const r=clone(request);delete r.simOptions;
  return canonical({version:VERSION,request:r,pins,templates,policy});
}
export async function optimize({request,pins,templates=[],backend,policy={},seed,signal,onProgress=(_progress)=>{},cache}) {
  const cfg={...defaults,...policy};
  for(const k of ['beam','rounds','candidates','trainIterations','holdoutBlocks','blockIterations'])
    if(!Number.isInteger(cfg[k])||cfg[k]<1)throw Error(`Invalid budget: ${k}`);
  if(cfg.candidates>1000||cfg.rounds>20||cfg.beam>10||cfg.trainIterations>4096||cfg.holdoutBlocks<32||cfg.holdoutBlocks>256||cfg.blockIterations>4096||!Number.isFinite(cfg.minimumGain)||cfg.minimumGain<0)throw Error('Unsupported optimization budget');
  if(!Number.isSafeInteger(seed)||seed<1||seed>1e9)throw Error('Explicit positive seed <= 1e9 required');
  if(!pins?.engine||!pins?.mechanics)throw Error('Engine and mechanics content fingerprints required');
  const input=clone(request);
  // Individual Sim exports fixed raid slots, including empty trailing parties.
  while(input.raid?.parties?.length>1&&!input.raid.parties.at(-1).players?.some(p=>p.class&&p.class!=='ClassUnknown'))input.raid.parties.pop();
  const slots=input.raid?.parties?.[0]?.players;
  while(slots?.length>1&&(!slots.at(-1).class||slots.at(-1).class==='ClassUnknown'))slots.pop();
  if(input.raid?.parties?.length!==1||input.raid.parties[0].players?.length!==1||!input.encounter?.targets?.length)throw Error('Optimizer requires one player and an encounter');
  const p=player(input), rawMode=p.forever?.mode;
  if(!['STRICT','BEST_GUESS',1,2].includes(rawMode))throw Error('Explicit STRICT or BEST_GUESS mechanics mode required');
  const mode=rawMode===1?'STRICT':rawMode===2?'BEST_GUESS':rawMode;
  const baseline=clone(p.rotation);if(!safeAPL(baseline))throw Error('A resolved, playable current APL is required');
  baseline.type='TypeAPL';delete baseline.simple;
  const key=cacheIdentity(input,pins,templates,cfg);
  if(cache?.has(key))return clone(cache.get(key));
  const check=()=>{if(signal?.aborted)throw Error('Optimization cancelled');};
  const reqFor=apl=>{const r=clone(input);player(r).rotation=clone(apl);return r;};
  const validate=async apl=>{check();if(!safeAPL(apl))return null;return backend.validate(reqFor(apl));};
  const initial=await validate(baseline);
  if(!initial?.valid)throw Error(`Current APL is invalid: ${(initial?.warnings||[]).join('; ')}`);
  const evaluate=async(apl,start,n)=>{
    check();const r=reqFor(apl);r.simOptions={...(r.simOptions||{}),iterations:n,randomSeed:String(start),debug:false,debugFirstIteration:false,saveAllValues:false};
    const result=await backend.run(r);check();
    const metric=result.raidMetrics?.parties?.[0]?.players?.[0]?.dps;
    // Protobuf JSON omits default scalar zeroes; a present empty DPS message is zero.
    const dps=metric&&metric.avg===undefined?0:metric?.avg;
    if(result.error||result.iterationsDone!==n||!Number.isFinite(dps))throw Error(result.error?.message||'Incomplete or nonfinite simulation');
    return dps;
  };
  const seen=new Set(), ranked=[];let evaluated=0,rejected=0;
  async function score(apl) {
    const k=canonical(apl);if(seen.has(k)||evaluated>=cfg.candidates)return;seen.add(k);evaluated++;
    const validation=await validate(apl);
    if(!validation?.valid){rejected++;return;}
    const mean=await evaluate(apl,seed,cfg.trainIterations);
    ranked.push({apl,mean});ranked.sort((a,b)=>b.mean-a.mean);
    onProgress({phase:'search',evaluated,rejected,bestDps:ranked[0].mean});
  }
  await score(baseline);
  for(const t of templates){const apl=clone(t);apl.type='TypeAPL';delete apl.simple;await score(apl);}
  for(let round=0;round<cfg.rounds&&evaluated<cfg.candidates;round++) {
    const roundLimit=Math.min(cfg.candidates,evaluated+Math.ceil((cfg.candidates-evaluated)/(cfg.rounds-round)));
    const beams=ranked.slice(0,cfg.beam).map(x=>neighbors(x.apl,initial.metadata,String(p.class)));
    let active=true;
    while(active&&evaluated<roundLimit){active=false;for(const gen of beams){if(evaluated>=roundLimit)break;const next=gen.next();if(!next.done){active=true;await score(next.value);}}}
  }
  // Freeze a SINGLE finalist before inspecting any held-out result. No holdout reranking or optional stopping.
  const finalist=ranked[0].apl, current=[],candidate=[],holdoutSeeds=[];
  for(let b=0;b<cfg.holdoutBlocks;b++){
    const start=seed+100000+(b*10000);holdoutSeeds.push(start);
    current.push(await evaluate(baseline,start,cfg.blockIterations));
    candidate.push(canonical(finalist)===canonical(baseline)?current[b]:await evaluate(finalist,start,cfg.blockIterations));
    onProgress({phase:'holdout',completed:b+1,total:cfg.holdoutBlocks});
  }
  const difference=stats(candidate.map((v,i)=>v-current[i])),a=stats(candidate),b=stats(current);
  // Conservative 99% two-sided Student-t interval: t(31)=2.744, decreases with more blocks.
  const error=2.75*difference.se, lower=difference.mean-error,upper=difference.mean+error;
  const improved=lower>cfg.minimumGain;
  const result={version:VERSION,status:improved?'VALIDATED_IMPROVEMENT':'INCONCLUSIVE',mode,pins,
    apl:improved?finalist:baseline,finalistAPL:finalist,optimizedDps:improved?a.mean:b.mean,currentDps:b.mean,
    finalistDps:a.mean,improvement:improved?difference.mean:0,improvementPercent:improved&&b.mean>0?100*difference.mean/b.mean:0,
    confidence:{level:.99,method:'paired independent batch means; single frozen finalist',difference:difference.mean,lower,upper,standardError:difference.se,candidateStandardError:a.se,currentStandardError:b.se},
    search:{evaluated,rejected,policy:cfg,trainSeed:seed,holdoutSeeds,iterationsPerRotation:cfg.holdoutBlocks*cfg.blockIterations},
    mechanics:initial.provenance||pins,limitations:['Best validated APL found within the bounded search, not a global optimum.','Simulation error excludes mechanics uncertainty.','Only modeled actions and seeded strategy families are searched.']};
  if(cache?.size>=16)cache.delete(cache.keys().next().value);
  cache?.set(key,clone(result));return result;
}
