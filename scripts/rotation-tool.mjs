// God-callable local tool. The caller supplies an imported/resolved WoWSims setup;
// no language model authors the APL, changes mechanics, or supplies DPS scores.
import { spawn } from 'node:child_process';
import { createInterface } from 'node:readline';
import { readFile, writeFile } from 'node:fs/promises';
import { createHash, randomInt } from 'node:crypto';
import { pathToFileURL } from 'node:url';
import { optimize, canonical } from '../ui/forever/optimizer.mjs';
import { explainRotation } from '../ui/forever/optimizer-explain.mjs';
export const tool={name:'find_ideal_rotation',description:'Optimize an imported Forever RaidSimRequest using WoWSims; return APL, paired holdout confidence and mechanics provenance.',inputSchema:{type:'object',required:['request'],properties:{request:{type:'object'},templates:{type:'array',items:{type:'object'}},policy:{type:'object'},seed:{type:'integer',minimum:1,maximum:1000000000}},additionalProperties:false}};
const sha=x=>createHash('sha256').update(x).digest('hex');
const sessionCache=new Map();
export async function findIdealRotation(input,{executable,cache=sessionCache,signal,onProgress=()=>{}}) {
  if(!input||Object.keys(input).some(k=>!['request','templates','policy','seed'].includes(k)))throw Error('Unsupported tool arguments');
  const child=spawn(executable,[],{stdio:['pipe','pipe','pipe'],windowsHide:true});
  const lines=createInterface({input:child.stdout});let pending,stderr='';
  const fail=e=>{pending?.reject(e);pending=undefined;};
  child.on('error',fail);child.on('exit',code=>fail(Error(`Simulator exited (${code}): ${stderr.slice(-1000)}`)));
  child.stderr.on('data',x=>{stderr=(stderr+x).slice(-4000);});
  lines.on('line',line=>{try{const v=JSON.parse(line);if(v.error)fail(Error(v.error));else{pending?.resolve(v);pending=undefined;}}catch(e){fail(e);}});
  const abort=()=>{fail(Error('Optimization cancelled'));child.kill();};signal?.addEventListener('abort',abort,{once:true});
  const call=(op,request)=>new Promise((resolve,reject)=>{
    if(signal?.aborted){reject(Error('Optimization cancelled'));return;}
    const timer=setTimeout(()=>{fail(Error('Simulation timeout'));child.kill();},120000);
    pending={resolve:v=>{clearTimeout(timer);resolve(v);},reject:e=>{clearTimeout(timer);reject(e);}};
    child.stdin.write(JSON.stringify({op,request})+'\n');
  });
  try {
    const pins={engine:sha(await readFile(executable)),mechanics:sha(await readFile(new URL('../ui/forever/data/talents.json',import.meta.url)))};
    const result=await optimize({...input,pins,backend:{validate:r=>call('validate',r),run:async r=>(await call('run',r)).result},seed:input.seed??randomInt(1,1000000001),signal,cache,onProgress});
    const db=JSON.parse(await readFile(new URL('../assets/database/db.json',import.meta.url),'utf8'));
    const discovery=JSON.parse(await readFile(new URL('../ui/forever/data/talents.json',import.meta.url),'utf8'));
    const names={};
    function collect(x){if(!x||typeof x!=='object')return;for(const [key,id] of Object.entries(x)){
      if((key==='spellId'||key==='auraId')&&id&&typeof id==='object'){
        const row=id.spellId?db.spellIcons.find(s=>s.id===id.spellId):id.itemId?db.itemIcons.find(s=>s.id===id.itemId):[...discovery.records,...discovery.mechanics].find(s=>s.action_tag===Math.abs(id.tag));
        if(row?.name)names[canonical(id)]=row.name;
      }else collect(id);
    }}collect(result.apl);
    result.explanation=explainRotation(result.apl,names);
    return result;
  }finally{signal?.removeEventListener('abort',abort);child.kill();lines.close();}
}
if(process.argv[1]&&import.meta.url===pathToFileURL(process.argv[1]).href){
  if(process.argv[2]==='--describe'){process.stdout.write(JSON.stringify(tool)+'\n');process.exit(0);}
  const [inputPath,executable,outputPath]=process.argv.slice(2);
  if(!inputPath||!executable)throw Error('Usage: node scripts/rotation-tool.mjs INPUT.json ROTATIONWORKER.exe [OUTPUT.json]');
  const input=JSON.parse(await readFile(inputPath,'utf8'));
  const result=await findIdealRotation(input,{executable,onProgress:p=>process.stderr.write(JSON.stringify(p)+'\n')});
  const json=JSON.stringify(result,null,2)+'\n';if(outputPath)await writeFile(outputPath,json);else process.stdout.write(json);
}
