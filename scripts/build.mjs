// Windows-compatible equivalents of the upstream make targets. No combat logic.
import { spawnSync } from 'node:child_process';
import { cpSync, existsSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
process.chdir(root);
const localGo = path.join(root, '.tools', 'go', 'bin');
if (existsSync(localGo)) {
  process.env.GOPATH = path.join(root, '.tools', 'gopath');
  process.env.GOCACHE = path.join(root, '.tools', 'go-cache');
  process.env.GOTOOLCHAIN = 'local';
  process.env.PATH = [localGo, path.join(process.env.GOPATH, 'bin'), process.env.PATH].join(path.delimiter);
}
const node = (script, ...args) => run(process.execPath, [script, ...args]);
function run(command, args, env = {}) {
  const result = spawnSync(command, args, { stdio: 'inherit', env: { ...process.env, ...env } });
  if (result.error) throw result.error;
  if (result.status !== 0) process.exit(result.status ?? 1);
}
function files(dir) {
  return readdirSync(dir, { withFileTypes: true }).flatMap(e =>
    e.isDirectory() ? files(path.join(dir, e.name)) : [path.join(dir, e.name)]);
}
function generate() {
  mkdirSync('sim/core/proto', { recursive: true });
  mkdirSync('ui/core/proto', { recursive: true });
  const protoc = 'node_modules/@protobuf-ts/protoc/protoc.js';
  node(protoc, '-I=proto', '--go_out=sim/core', ...files('proto').filter(f => f.endsWith('.proto')));
  node(protoc, '--ts_opt', 'generate_dependencies', '--ts_out', 'ui/core/proto', '--proto_path', 'proto', 'proto/api.proto');
  for (const file of ['test', 'ui']) node(protoc, '--ts_out', 'ui/core/proto', '--proto_path', 'proto', `proto/${file}.proto`);
  const imports = files('ui/core').filter(f => f.endsWith('.ts') && f !== path.join('ui/core', 'index.ts'));
  writeFileSync('ui/core/index.ts', imports.sort().map(f => `import './${path.relative('ui/core', f).replaceAll('\\', '/').slice(0, -3)}';`).join('\n') + '\n');
  const template = readFileSync('ui/index_template.html', 'utf8');
  for (const entry of readdirSync('ui', { withFileTypes: true })) {
    if (!entry.isDirectory() || !existsSync(`ui/${entry.name}/index.ts`)) continue;
    if (['core', 'worker'].includes(entry.name)) continue;
    const title = entry.name.split('_').map(s => s[0].toUpperCase() + s.slice(1)).join(' ');
    writeFileSync(`ui/${entry.name}/index.html`, template.replaceAll('@@TITLE@@', `Classic ${title} Simulator`).replaceAll('@@SPEC@@', entry.name));
  }
  mkdirSync('binary_dist/classic', { recursive: true });
  writeFileSync('binary_dist/classic/embedded', '');
  cpSync('sim/web/dist.go.tmpl', 'binary_dist/dist.go');
}
function wasm() {
  mkdirSync('dist/classic', { recursive: true });
  run('go', ['build', '-o', 'dist/classic/lib.wasm', './sim/wasm/'], { GOOS: 'js', GOARCH: 'wasm' });
}
switch (process.argv[2]) {
  case 'generate': generate(); break;
  case 'test':
    generate();
    wasm();
    run('go', ['test', '-count=1', '--tags=with_db', './sim/...', './cmd/...']);
    break;
  case 'build':
    generate();
    wasm();
    node('node_modules/typescript/bin/tsc', '--noEmit');
    node('node_modules/tsx/dist/cli.mjs', 'vite.build-workers.ts');
    node('node_modules/vite/bin/vite.js', 'build');
    cpSync('assets', 'dist/classic/assets', { recursive: true, filter: p => !p.split(path.sep).includes('db_inputs') });
    mkdirSync('.artifacts', { recursive: true });
    run('go', ['build', '--tags=with_db', '-o', `.artifacts/wowsimcli${process.platform === 'win32' ? '.exe' : ''}`, './cmd/wowsimcli']);
    run('go', ['build', '--tags=with_db', '-o', `.artifacts/foreversim${process.platform === 'win32' ? '.exe' : ''}`, './cmd/foreversim']);
    break;
  default: throw new Error('Expected generate, test, or build');
}
