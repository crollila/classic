import { spawnSync } from 'node:child_process';
import { cpSync, existsSync, mkdirSync, readFileSync,writeFileSync } from 'node:fs';
import { gzipSync } from 'node:zlib';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
process.chdir(root);
process.env.PATH = [path.join(root, 'node_modules/.bin'), process.env.PATH].join(path.delimiter);
const localGo = path.join(root, '.tools/go/bin');
if (existsSync(localGo)) {
  process.env.GOPATH = path.join(root, '.tools/gopath');
  process.env.GOCACHE = path.join(root, '.tools/go-cache');
  process.env.GOTOOLCHAIN = 'local';
  process.env.PATH = [localGo, path.join(process.env.GOPATH, 'bin'), process.env.PATH].join(path.delimiter);
}
function run(command, args, env = {}) {
  const result = spawnSync(command, args, { stdio: 'inherit', env: { ...process.env, ...env } });
  if (result.error) throw result.error;
  if (result.status !== 0) process.exit(result.status ?? 1);
}
const node = (script, ...args) => run(process.execPath, [script, ...args]);
node('scripts/build.mjs', 'generate');
node('node_modules/typescript/bin/tsc', '--noEmit');
mkdirSync('dist/forever-sim', { recursive: true });
run('go', ['build', '--tags=with_db', '-trimpath', '-buildvcs=false', '-o', 'dist/forever-sim/lib.wasm', './sim/wasm/'], { GOOS: 'js', GOARCH: 'wasm' });
writeFileSync('dist/forever-sim/lib.wasm.gz',gzipSync(readFileSync('dist/forever-sim/lib.wasm'),{level:9}));
const { build } = await import('vite');
const goRoot = spawnSync('go', ['env', 'GOROOT'], { encoding: 'utf8' }).stdout.trim();
const wasmRuntime = ['lib/wasm/wasm_exec.js', 'misc/wasm/wasm_exec.js'].map(p => path.join(goRoot, p)).find(existsSync);
if (!wasmRuntime) throw new Error('Go WebAssembly runtime not found');
await build({
  configFile: false,
  define:{'import.meta.env.VITE_FOREVER':'true'},
  plugins: [{ name: 'go-wasm-runtime', transform(code, id) {
    if (id.replaceAll('\\', '/').endsWith('/worker/sim_worker.ts')) return readFileSync(wasmRuntime, 'utf8') + '\n' + code;
  }}],
  build: { outDir: 'dist/forever-sim', emptyOutDir: false, minify: false, target: 'es2020', lib: { entry: 'ui/worker/sim_worker.ts', formats: ['iife'], name: 'ForeverWorker', fileName: () => 'sim_worker.js' } },
});
node('scripts/vite-forever.mjs');
cpSync('assets', 'dist/forever-sim/assets', { recursive: true, filter: p => !p.split(path.sep).includes('db_inputs') });
node('scripts/build-gear-availability.mjs');
cpSync('LICENSE', 'dist/forever-sim/LICENSE.txt');
console.log('Forever Simulator built in dist/forever-sim. Serve dist with npm run preview:forever.');
