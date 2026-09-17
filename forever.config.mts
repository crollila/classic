import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { defineConfig, Plugin } from 'vite';
import { validateRelease, rewriteLocalPaths } from './scripts/forever-contract.mjs';

const root = path.dirname(fileURLToPath(import.meta.url));
const ui = path.resolve(root, 'ui');
const output = path.resolve(root, 'dist/forever-sim');
const releasePath = path.resolve(root, '../shared/simulator/release.json');
const base = '/forever-sim/';

function release() {
  const data = JSON.parse(fs.readFileSync(releasePath, 'utf8'));
  validateRelease(data);
	const discovery=JSON.parse(fs.readFileSync(path.join(root,'sim/core/foreverdata/trees.json'),'utf8'));
  let commit = 'unavailable';
  try { commit = execFileSync('git', ['-c', `safe.directory=${root.replaceAll('\\', '/')}`, 'rev-parse', '--short=12', 'HEAD'], { cwd: root, encoding: 'utf8' }).trim(); } catch {}
  const hash = (file: string) => fs.existsSync(file) ? createHash('sha256').update(fs.readFileSync(file)).digest('hex') : null;
  return {
    ...data,
		schemaVersion:2,engineMode:'forever-discovery',foreverBuild:null,rulesetId:discovery.ruleset_id,
		manifestSha256:discovery.manifest_sha256,mechanicsUpdatedAt:'2026-09-13T07:59:08.480Z',
		notice:'Pre-beta discovery simulator. BEST_GUESS includes provisional mechanics and labeled predictions; STRICT skips predictions. This is not a validated beta-client ruleset.',
		mechanics:[...discovery.records,...discovery.mechanics].filter(r=>!['blocked','non-sim'].includes(r.mode)).map(r=>({id:r.id,label:r.name||r.id,category:r.tree?'talents':r.kind==='racial'?'race':'encounter',confidence:r.ranks?.some((v: { estimated: boolean })=>v.estimated)||r.adapter?.predicted_components?.length?'PREDICTED':r.confidence||'PROVISIONAL',note:(r.adapter?.scope||'Known effect uses Classic interaction behavior provisionally.')+' See Talents for rank-specific evidence and mode behavior.',evidence:[r.source_url]})),
    simulatorVersion: `${JSON.parse(fs.readFileSync(path.join(root, 'package.json'), 'utf8')).version}+forever-v2.${hash(path.join(output,'lib.wasm'))?.slice(0,12)||commit}`,
    upstreamCommit: commit,
    builtAt: new Date().toISOString(),
    databaseSha256: hash(path.join(root, 'assets/database/db.json')),
    engineSha256: hash(path.join(output, 'lib.wasm')),
  };
}

function foreverAdapter(): Plugin {
  return {
    name: 'forever-presentation-adapter',
    enforce: 'pre',
    transform(code, id) {
      // Only UI URLs are adapted. External Wowhead /classic/ links stay intact.
      if (!id.replaceAll('\\', '/').includes('/ui/')) return;
      let transformed = rewriteLocalPaths(code);
      if (id.replaceAll('\\', '/').endsWith('/core/constants/other.ts')) {
        transformed = transformed.replace("REPO_NAME = 'classic'", "REPO_NAME = 'forever-sim'");
      }
      return transformed === code ? undefined : { code: transformed, map: null };
    },
    transformIndexHtml: {
      order: 'pre',
      handler(html, context) {
        if (path.resolve(context.filename) === path.join(ui, 'index.html')) {
          html = fs.readFileSync(path.join(ui, 'forever/landing.html'), 'utf8');
        }
        html = rewriteLocalPaths(html)
          .replace(/<html>/, '<html lang="en">')
          .replace(/<title>Classic (.*?) Simulator<\/title>/, '<title>$1 · Forever Simulator | Exalted Capital</title>')
          .replace('Simulations for World of Warcraft® Classic.', 'World of Warcraft: Forever simulation tools by Exalted Capital, built on WoWSims.')
          .replace(/<!-- Global site tag[\s\S]*?<\/script>\s*<script>[\s\S]*?<\/script>/g, '')
          .replace('</head>', '<link rel="stylesheet" href="/forever/theme.css" /></head>');
        return html;
      },
    },
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        const url = new URL(req.url || '/', 'http://localhost').pathname;
        if (url === '/forever-sim' || url === '/') {
          res.writeHead(302, { Location: base }); res.end(); return;
        }
        if (url === `${base}release.json`) {
          res.setHeader('Content-Type', 'application/json');
          res.setHeader('Cache-Control', 'no-store');
          res.end(JSON.stringify(release())); return;
        }
        const relative = url.startsWith(`${base}assets/`) ? url.slice(`${base}assets/`.length) : null;
        const assetRoot = path.join(root, 'assets');
        const file = relative !== null ? path.resolve(assetRoot, relative) :
          ['sim_worker.js', 'lib.wasm'].includes(url.slice(base.length)) ? path.join(output, url.slice(base.length)) : null;
        if (!file) { next(); return; }
        if (relative !== null && !file.startsWith(assetRoot + path.sep)) { res.writeHead(403); res.end(); return; }
        if (!fs.existsSync(file)) { res.writeHead(404); res.end('Run npm run build:forever first.'); return; }
        const types: Record<string, string> = { '.wasm': 'application/wasm', '.js': 'text/javascript', '.json': 'application/json', '.png': 'image/png', '.jpg': 'image/jpeg', '.woff2': 'font/woff2', '.svg': 'image/svg+xml' };
        res.setHeader('Content-Type', types[path.extname(file)] || 'application/octet-stream');
        fs.createReadStream(file).pipe(res);
      });
    },
    generateBundle() {
      this.emitFile({ type: 'asset', fileName: 'release.json', source: JSON.stringify(release(), null, 2) });
    },
  };
}

export default defineConfig({
  root: ui,
  base,
  define: { 'import.meta.env.VITE_FOREVER': '"true"' },
  plugins: [foreverAdapter()],
  esbuild: { jsxInject: "import { element, fragment } from 'tsx-vanilla';" },
  css: { postcss: { plugins: [{ postcssPlugin: 'forever-local-assets', Declaration(decl) { decl.value = rewriteLocalPaths(decl.value); } }] } },
  server: { host: '127.0.0.1', port: 5173, strictPort: true },
  build: {
    outDir: output,
    emptyOutDir: false,
    target: 'es2020',
    rollupOptions: {
      input: [path.join(ui, 'index.html'), ...fs.readdirSync(ui, { withFileTypes: true })
        .filter(e => e.isDirectory() && e.name !== 'forever' && fs.existsSync(path.join(ui, e.name, 'index.html')))
        .map(e => path.join(ui, e.name, 'index.html'))],
      output: { assetFileNames: 'bundle/[name]-[hash][extname]', entryFileNames: 'bundle/[name]-[hash].js', chunkFileNames: 'bundle/[name]-[hash].js' },
    },
  },
});
