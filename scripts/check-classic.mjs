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
for(const [file,expected]of Object.entries(reviewed.files)){
 const normalized=readFileSync(file,'utf8').replaceAll('\r\n','\n');
 const actual=createHash('sha256').update(normalized).digest('hex');
 if(actual!==expected)throw new Error(`Unreviewed changes in ${file}; review behavior and rerun regression checks before updating its pinned hash.`);
}
const exclusions=Object.keys(reviewed.files).map(file=>`:(exclude)${file}`);
const check = spawnSync('git', ['diff', '--exit-code', upstream, '--', ...protectedPaths,...exclusions], { stdio: 'inherit' });
if (check.error) throw check.error;
if (check.status !== 0) process.exit(check.status ?? 1);
console.log(`Classic protected files match ${upstream}; ${Object.keys(reviewed.files).length} explicit discovery extension files match their reviewed hashes. Behavioral parity is checked by npm test.`);
