import { spawnSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { createHash } from 'node:crypto';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

process.chdir(path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..'));
const { upstream_commit: upstream } = JSON.parse(readFileSync('sim/forever/baseline.json', 'utf8'));
// Explicit extension files are pinned below; everything else stays at upstream.
const protectedPaths = [
  'sim/core', 'sim/common', 'sim/encounters', 'sim/register_all.go', 'sim/sim_test.go',
  'sim/druid', 'sim/hunter', 'sim/mage', 'sim/paladin', 'sim/priest',
  'sim/rogue', 'sim/shaman', 'sim/warlock', 'sim/warrior', 'proto', 'assets/database',
  ':(glob)ui/*/apls/**', ':(glob)ui/*/gear_sets/**', 'ui/core/talents',
];
const reviewed=JSON.parse(readFileSync('sim/forever/reviewed-boundary.json','utf8'));
if(reviewed.upstream_commit!==upstream)throw new Error('Reviewed boundary uses the wrong upstream');
const issues = [];
const binaryFiles=reviewed.binary_files || {};
if(Object.keys(binaryFiles).some(file=>Object.hasOwn(reviewed.files,file)))throw new Error('A reviewed file cannot have both text and binary hashes');
const allFiles={...reviewed.files,...binaryFiles};
for(const [file,expected]of Object.entries(allFiles)){
 try {
  const normalized=Object.hasOwn(binaryFiles,file) ? readFileSync(file) : readFileSync(file,'utf8').replaceAll('\r\n','\n');
  const actual=createHash('sha256').update(normalized).digest('hex');
  if(actual!==expected)issues.push({file, kind:'changed-review-pin', expected, actual});
 } catch (error) {
  if(error.code!=='ENOENT')throw error;
  issues.push({file, kind:'missing-reviewed-file'});
 }
}
const exclusions=Object.keys(allFiles).map(file=>`:(exclude)${file}`);
function gitFiles(args) {
 const result=spawnSync('git',args,{encoding:'utf8'});
 if(result.error)throw result.error;
 if(result.status!==0)throw new Error(result.stderr || `git failed (${result.status})`);
 return result.stdout.split('\0').filter(Boolean);
}
for(const file of gitFiles(['diff','--name-only','-z',upstream,'--',...protectedPaths,...exclusions])){
 issues.push({file,kind:'unreviewed-protected-change'});
}
// git diff alone omits newly created, untracked engine files.
for(const file of gitFiles(['ls-files','--others','--exclude-standard','-z','--',...protectedPaths,...exclusions])){
 issues.push({file,kind:'untracked-protected-file'});
}
issues.sort((a,b)=>a.file.localeCompare(b.file));
if(process.argv.includes('--json')){
 console.log(JSON.stringify({upstream,acceptedBaseline:reviewed.accepted_baseline_commit || null,reviewedFiles:Object.keys(allFiles).length,passed:issues.length===0,issues},null,2));
}else if(issues.length){
 console.error(`${issues.length} Classic boundary review issues (no pins were changed):`);
 for(const issue of issues)console.error(`  ${issue.kind}: ${issue.file}`);
 console.error('Review behavior and regression evidence before updating pins. Use --json for the full inventory.');
}else{
 console.log(`Protected files outside the accepted review match ${upstream}; ${Object.keys(allFiles).length} reviewed files match their pinned hashes. Retained-baseline regressions are checked by npm test.`);
}
process.exitCode=issues.length ? 1 : 0;
