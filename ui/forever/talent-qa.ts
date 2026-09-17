// Browser fixture only; it is not an entry in the production build.
import { ForeverTalentsPicker } from './talents_picker';
import {Class,Spec} from '../core/proto/common';
import {ForeverOptions} from '../core/proto/api';
import {TypedEvent} from '../core/typed_event';
import type {Player} from '../core/player';
import {foreverDefaultOptions,foreverRaces} from './discovery';
import {runBrowserChecks} from './talent-qa-checks';
const select=document.querySelector<HTMLSelectElement>('#class')!;
for(const cls of Object.values(Class).filter((v):v is Class=>typeof v==='number'&&v!==Class.ClassUnknown)){const o=document.createElement('option');o.value=String(cls);o.textContent=Class[cls].replace('Class','');select.append(o);}
const cls=Number(new URL(location.href).searchParams.get('qaClass')||Class.ClassDruid) as Class;select.value=String(cls);
select.addEventListener('change',()=>{const url=new URL(location.href);url.search='?qaClass='+select.value;location.href=url.href;});
let race=foreverRaces(cls)[0],f=foreverDefaultOptions(cls,race);
const talentsChangeEmitter=new TypedEvent<void>(),raceChangeEmitter=new TypedEvent<void>();
const player={getClass:()=>cls,getRace:()=>race,getForever:()=>ForeverOptions.clone(f),setForever:(id:number,next:ForeverOptions)=>{f=ForeverOptions.clone(next);talentsChangeEmitter.emit(id);},setRace:(id:number,next:number)=>{race=next;raceChangeEmitter.emit(id);},talentsChangeEmitter,raceChangeEmitter,sim:{waitForInit:()=>Promise.resolve()}} as unknown as Player<Spec>;
new ForeverTalentsPicker(document.querySelector('#app')!,player);
const run=document.createElement('button');run.textContent='Run all UI checks';document.querySelector('#qa-status')!.after(run);
run.addEventListener('click',async()=>{run.disabled=true;const host=document.createElement('div');document.body.append(host);try{document.querySelector('#qa-status')!.textContent=await runBrowserChecks(host);}catch(error){document.querySelector('#qa-status')!.textContent='FAILED: '+String(error);}finally{host.remove();run.disabled=false;}});
