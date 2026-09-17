import { APLListItem, APLRotation, APLRotation_Type } from '../core/proto/apl';
import { ForeverOptions, SpellStats } from '../core/proto/api';
import { Class, OtherAction, Spec } from '../core/proto/common';
import { foreverDiscoveryTalents as data, foreverClassName, effectiveForeverRank, isExecutable } from './discovery';

// Preserve explicit player APLs; the optional planning overlay prepends registered
// new offensive actions, using metadata to avoid references to disabled spells.
export function withForeverRotation(base:APLRotation, f:ForeverOptions|undefined,c:Class,spells:SpellStats[],spec?:Spec,targetCount=1):APLRotation{
 if(!f||f.parameters['rotation.forever_auto']===0)return base;
 const healer=spec!==undefined&&[Spec.SpecHealingPriest,Spec.SpecHolyPaladin,Spec.SpecRestorationDruid,Spec.SpecRestorationShaman].includes(spec);
 const available=(id:number)=>spells.some(s=>s.isCastable&&s.id?.rawId.oneofKind==='spellId'&&s.id.rawId.spellId===id);
 if(healer){
  const choices=spec===Spec.SpecHealingPriest?[10901,10929,10965]:spec===Spec.SpecHolyPaladin?[25292,19943]:spec===Spec.SpecRestorationShaman?[25357,10468]:[25299,9858,25297];
  const actions=choices.filter(available).map(id=>{const row:any={action:{castSpell:{spellId:{spellId:id},target:{type:'Self'}}}};if([10901,10929,25299,9858].includes(id))row.action.condition={not:{val:{auraIsActive:{auraId:{spellId:id},sourceUnit:{type:'Self'}}}}};return APLListItem.fromJson(row)});
  base=APLRotation.create({type:APLRotation_Type.TypeAPL,priorityList:actions});
 }
 if(spec===Spec.SpecFeralTankDruid){base=APLRotation.create({type:APLRotation_Type.TypeAPL,priorityList:[...([9634,5229,9908,9881].filter(available).map(id=>APLListItem.fromJson({action:{castSpell:{spellId:{spellId:id}},...(id===9634?{condition:{not:{val:{auraIsActive:{auraId:{spellId:9634}}}}}}:{})}})))]});}
 const rows=[...data.records.filter(r=>r.class===foreverClassName(c)&&effectiveForeverRank(f,r)>0),...data.mechanics.filter(m=>f.mechanics.includes(m.id)&&isExecutable(m.mode))]
  .filter(r=>r.adapter?.auto_priority!==undefined).sort((a,b)=>a.adapter!.auto_priority!-b.adapter!.auto_priority!);
 const additions:APLListItem[]=[];
 if(c===Class.ClassMage&&available(10212)){
  const blast=data.records.find(r=>r.id==='mage.talent.arcane-blast')!,barrage=data.records.find(r=>r.id==='mage.talent.missile-barrage')!;
  if(effectiveForeverRank(f,blast)>0){additions.push(APLListItem.fromJson({action:{condition:{or:{vals:[{auraIsActive:{auraId:{otherId:19,tag:barrage.action_tag}}},{cmp:{op:'OpGe',lhs:{auraNumStacks:{auraId:{otherId:19,tag:blast.action_tag}}},rhs:{const:{val:'4'}}}}]}},channelSpell:{spellId:{spellId:10212}}}}))}
 }
 for(const r of rows){
  if((r.adapter as any)?.auto_target?.type==='Target'&&(r.adapter as any).auto_target.index>=targetCount)continue;
  const healing=!!(r.adapter as any)?.healing;
  if(healer&&!healing&&!(r.adapter as any)?.alternate_healing_action)continue;
  if(!healer&&healing&&!(r.adapter as any)?.hybrid_healing)continue;
  const tag=healer&&(r.adapter as any)?.alternate_healing_action?-r.action_tag:r.action_tag;
  const s=spells.find(s=>s.isCastable&&s.id?.rawId.oneofKind==='otherId'&&s.id.rawId.otherId===OtherAction.OtherActionForever&&s.id.tag===tag);
  if(!s||!s.id)continue;
  const id={otherId:OtherAction.OtherActionForever,tag};
  const config:any={action:{castSpell:{spellId:id}}};
  if(s.isChanneled)config.action={channelSpell:{spellId:id}};
  if(healer)config.action[s.isChanneled?'channelSpell':'castSpell'].target={type:'Self'};
  if(s.hasDot&&!s.isChanneled)config.action.condition={not:{val:{dotIsActive:{spellId:id}}}};
  if((r.adapter as any)?.auto_condition)config.action.condition=(r.adapter as any).auto_condition;
  if((r.adapter as any)?.auto_target)config.action[s.isChanneled?'channelSpell':'castSpell'].target=(r.adapter as any).auto_target;
  additions.push(APLListItem.fromJson(config));
 }
 if(spec===Spec.SpecFeralTankDruid){const r=data.records.find(r=>r.id==='druid.talent.shredding-attacks')!;if(spells.some(s=>s.isCastable&&s.id?.rawId.oneofKind==='otherId'&&s.id.tag===-r.action_tag))additions.push(APLListItem.fromJson({action:{castSpell:{spellId:{otherId:19,tag:-r.action_tag}}}}));}
 if(!additions.length)return base;
 const result=APLRotation.clone(base);result.priorityList=[...additions,...result.priorityList];return result;
}
