import { canonical } from './optimizer.mjs';
const words=s=>s.replace(/([a-z])([A-Z])/g,'$1 $2').replace(/^./,s=>s.toUpperCase());
export function explainRotation(apl,names={}) {
  const id=x=>names[canonical(x)]|| (x?.spellId?`Spell ${x.spellId}`:x?.itemId?`Item ${x.itemId}`:`Action ${x?.otherId??'?'}${x?.tag?` (${x.tag})`:''}`);
  function value(v) {
    if(!v)return 'whenever usable';
    if(v.const)return v.const.val;
    if(v.and||v.or)return '('+(v.and||v.or).vals.map(value).join(v.and?' and ':' or ')+')';
    if(v.not)return `not (${value(v.not.val)})`;
    if(v.cmp)return `${value(v.cmp.lhs)} ${{OpEq:'=',OpNe:'≠',OpGe:'≥',OpGt:'>',OpLe:'≤',OpLt:'<'}[v.cmp.op]||v.cmp.op} ${value(v.cmp.rhs)}`;
    if(v.math)return `(${value(v.math.lhs)} ${v.math.op} ${value(v.math.rhs)})`;
    const [key,body]=Object.entries(v)[0]||['unknown',{}];
    return `${words(key)}${body.spellId||body.auraId?` of ${id(body.spellId||body.auraId)}`:''}${body.threshold?` ${body.threshold}`:''}${body.sourceUnit?` (${words(body.sourceUnit.type||'Self')})`:''}`;
  }
  function action(a) {
    if(!a)return 'No action';
    const k=Object.keys(a).find(k=>k!=='condition'),b=a[k];
    if(!k)return 'No action';
    let text;
    if(['castSpell','channelSpell','multidot','multishield'].includes(k))text=`${k==='channelSpell'?'Channel ':k==='multidot'?'Maintain on multiple targets: ':''}${id(b.spellId)}${b.target?` on ${words(b.target.type||'CurrentTarget')}${b.target.index?` ${b.target.index+1}`:''}`:''}`;
    else if(k==='sequence'||k==='strictSequence')text=`${words(k)}: ${(b.actions||[]).map(action).join(' → ')}`;
    else if(k==='schedule')text=`At ${b.schedule}: ${action(b.innerAction)}`;
    else if(k==='wait')text=`Wait ${value(b.duration)}`;
    else if(k==='waitUntil')text=`Wait until ${value(b.condition)}`;
    else text=words(k)+(Object.keys(b||{}).length?` (${Object.entries(b).map(([k,v])=>`${words(k)}: ${typeof v==='object'?value(v):v}`).join(', ')})`:'');
    return `${text} — ${value(a.condition)}`;
  }
  const rows=(apl.priorityList||[]).filter(r=>!r.hide&&r.action).map(r=>action(r.action));
  return {opening:(apl.prepullActions||[]).filter(r=>!r.hide).map(r=>`${value(r.doAtValue)}: ${action(r.action)}`),priority:rows,
    cooldowns:rows.filter(r=>/cooldown|current time|remaining time|schedule/i.test(r)),execute:rows.filter(r=>/execute/i.test(r)),
    instructions:'Work down the priority list; press the first usable action whose condition is true. Recheck after each action. Conditions also apply during execute.'};
}

