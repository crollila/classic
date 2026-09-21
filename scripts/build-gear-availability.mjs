import { readFileSync, writeFileSync } from 'node:fs';
import { gunzipSync } from 'node:zlib';
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { classifyGear } from '../ui/app/gear-availability.mjs';

const raw = readFileSync('sim/core/gamedata/current.json.gz');
const snapshot = JSON.parse(gunzipSync(raw));
const database = JSON.parse(readFileSync('assets/database/db.json'));
const manifest = JSON.parse(readFileSync('assets/database/forever-item-database.json'));
// Snapshot revisions append a content suffix; the extraction manifest stores
// the base client build. Preserve the full revision in the output below.
if (manifest.sources?.values?.build !== snapshot.build.split('+')[0]) throw Error('Item evidence and snapshot builds differ');
const baseline = JSON.parse(readFileSync('sim/forever/baseline.json')).upstream_commit;
const legacy = JSON.parse(execFileSync('git', ['-c', 'safe.directory=*', 'show', `${baseline}:assets/database/db.json`], {maxBuffer: 100 * 1024 * 1024}));
const ids = new Set(legacy.items.map(i => i.id));
const items = Object.fromEntries(database.items.map(item => [item.id, classifyGear(item, snapshot, manifest.item_notes || {}, ids)]));
writeFileSync('dist/forever-sim/assets/database/gear-availability.json', JSON.stringify({
  version: 1, build: snapshot.build, snapshotSha256: createHash('sha256').update(raw).digest('hex'),
  databaseSha256: createHash('sha256').update(JSON.stringify(database)).digest('hex'),
  legacyBaseline: baseline, evidenceRetrievedAt: manifest.sources?.presentation?.retrieved_at || null,
  policy: 'Source-listed is not live-confirmed. Legacy means present in the pinned Classic/SoD catalog. New-to-catalog is not proof of Forever exclusivity. Legacy phases are not Forever phases.', items,
}));
console.log(`Generated build-pinned availability labels for ${database.items.length} items`);
