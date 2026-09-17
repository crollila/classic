// Extracts the per-spec UI presets (ui/<spec>/presets.ts) into a JSON file the
// foreverbench CLI embeds (cmd/foreverbench/presets/ui_presets.json).
//
//   node cmd/foreverbench/extract_presets.mjs [--root <repo>] [--proto-dir <ui/core/proto>] [--out <file>]
//
// Each presets.ts is bundled with esbuild and imported. UI-only modules are
// replaced by small shims so no DOM is needed:
//   - core/preset_utils: makePreset* return {kind, name, file|talentsString}
//   - core/constants/other: only the Phase enum
//   - core/player, individual_sim_ui, proto_utils, <spec>/sim: empty
//   - gear_sets/*.json, apls/*.json: {__file: "<path relative to ui/>"} (the CLI
//     reads the real files at runtime so presets stay in sync with the repo)
// The generated protobuf-ts modules (ui/core/proto, produced by `make`) are used
// for real, so enum values and message defaults match the UI. If the checkout
// has no generated protos, pass --proto-dir pointing at one that does.
import { createRequire } from 'node:module';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const require = createRequire(import.meta.url);

function arg(name, def) {
	const i = process.argv.indexOf(name);
	return i >= 0 ? process.argv[i + 1] : def;
}

function findUp(start, rel) {
	let dir = start;
	for (let i = 0; i < 10; i++) {
		if (fs.existsSync(path.join(dir, rel))) return dir;
		const parent = path.dirname(dir);
		if (parent === dir) break;
		dir = parent;
	}
	return null;
}

const root = path.resolve(arg('--root', path.join(here, '..', '..')));
const uiDir = path.join(root, 'ui');
const protoDir = path.resolve(arg('--proto-dir', path.join(findUp(root, 'ui/core/proto/common.ts') ?? root, 'ui', 'core', 'proto')));
const outFile = path.resolve(arg('--out', path.join(here, 'presets', 'ui_presets.json')));
if (!fs.existsSync(path.join(protoDir, 'common.ts'))) {
	console.error(`generated protos not found in ${protoDir}; run make (or pass --proto-dir)`);
	process.exit(1);
}
let esbuild;
try {
	esbuild = require('esbuild');
} catch {
	esbuild = createRequire(path.join(findUp(root, 'node_modules/esbuild') ?? root, 'x.js'))('esbuild');
}

const shims = {
	preset_utils: `
export const makePresetGear = (name, gear, options) => ({ kind: 'gear', name, file: gear && gear.__file, tooltip: options && options.tooltip });
export const makePresetTalents = (name, data) => ({ kind: 'talents', name, talentsString: data && data.talentsString });
export const makePresetAPLRotation = (name, apl) => ({ kind: 'apl', name, file: apl && apl.__file });
export const makePresetSimpleRotation = (name) => ({ kind: 'simple_rotation', name });
export const makePresetEpWeights = (name) => ({ kind: 'ep', name });
export const makePresetEncounter = (name) => ({ kind: 'encounter', name });
export const makePresetBuild = (name, b) => ({ kind: 'build', name, gear: b.gear && b.gear.name, talents: b.talents && b.talents.name, rotation: b.rotation && b.rotation.name, race: b.race });
`,
	other: `export var Phase; (function (P) { P[P.Phase1 = 1] = 'Phase1'; P[P.Phase2 = 2] = 'Phase2'; P[P.Phase3 = 3] = 'Phase3'; P[P.Phase4 = 4] = 'Phase4'; P[P.Phase5 = 5] = 'Phase5'; P[P.Phase6 = 6] = 'Phase6'; })(Phase || (Phase = {}));
export const CURRENT_PHASE = 6;`,
	empty: `export class Player {}; export class IndividualSimUI {}; export const getSpecConfig = () => ({}); export const naturalSpecOrder = [];`,
};

const plugin = {
	name: 'foreverbench-shims',
	setup(build) {
		build.onResolve({ filter: /core\/preset_utils(\.js)?$/ }, () => ({ path: 'preset_utils', namespace: 'shim' }));
		build.onResolve({ filter: /core\/constants\/other(\.js)?$/ }, () => ({ path: 'other', namespace: 'shim' }));
		build.onResolve({ filter: /(core\/player|core\/individual_sim_ui|core\/proto_utils\/utils|\/sim)(\.js)?$/ }, () => ({ path: 'empty', namespace: 'shim' }));
		build.onResolve({ filter: /core\/proto\/[a-z_]+(\.js)?$/ }, args => ({ path: path.join(protoDir, path.basename(args.path).replace(/\.js$/, '') + '.ts') }));
		build.onResolve({ filter: /\.(gear|apl)\.json$/ }, args => ({ path: path.resolve(args.resolveDir, args.path), namespace: 'preset-file' }));
		build.onLoad({ filter: /.*/, namespace: 'shim' }, args => ({ contents: shims[args.path], loader: 'js' }));
		build.onLoad({ filter: /.*/, namespace: 'preset-file' }, args => ({
			contents: `export default ${JSON.stringify({ __file: path.relative(uiDir, args.path).split(path.sep).join('/') })};`,
			loader: 'js',
		}));
	},
};

const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'foreverbench-presets-'));
const specs = {};
const dirs = fs
	.readdirSync(uiDir, { withFileTypes: true })
	.filter(d => d.isDirectory() && d.name !== 'raid' && fs.existsSync(path.join(uiDir, d.name, 'presets.ts')))
	.map(d => d.name)
	.sort();
const errors = {};
for (const dir of dirs) {
	const outfile = path.join(tmp, dir + '.mjs');
	try {
		await esbuild.build({
			entryPoints: [path.join(uiDir, dir, 'presets.ts')],
			bundle: true,
			format: 'esm',
			platform: 'node',
			outfile,
			logLevel: 'silent',
			plugins: [plugin],
			nodePaths: [path.join(findUp(protoDir, 'node_modules/@protobuf-ts') ?? root, 'node_modules')],
		});
		const mod = await import(pathToFileURL(outfile).href);
		const exports = {};
		for (const key of Object.keys(mod).sort()) {
			if (typeof mod[key] === 'function') continue;
			exports[key] = mod[key];
		}
		const files = dir => {
			const p = path.join(uiDir, dir);
			return fs.existsSync(p) ? fs.readdirSync(p).sort() : [];
		};
		specs[dir] = {
			exports,
			gear_set_files: files(path.join(dir, 'gear_sets')),
			apl_files: files(path.join(dir, 'apls')),
		};
	} catch (e) {
		errors[dir] = String(e && e.message ? e.message : e).split('\n')[0];
	}
}
fs.rmSync(tmp, { recursive: true, force: true });

// Snapshot every gear set and APL file (registered or not) so the CLI binary is
// self-contained; the Go test suite checks the snapshot against the repo files.
const files = {};
for (const [dir, spec] of Object.entries(specs)) {
	for (const [sub, list] of [
		['gear_sets', spec.gear_set_files],
		['apls', spec.apl_files],
	]) {
		for (const name of list) {
			if (!name.endsWith('.json')) continue;
			const rel = `${dir}/${sub}/${name}`;
			files[rel] = JSON.parse(fs.readFileSync(path.join(uiDir, rel), 'utf8'));
		}
	}
}

const out = {
	schema: 1,
	generator: 'cmd/foreverbench/extract_presets.mjs',
	note: 'Generated from ui/<spec>/presets.ts; enums are protobuf-ts numeric values, messages are JSON-compatible with protojson (lowerCamel names). files holds the parsed gear_sets/apls JSON (path relative to ui/).',
	specs,
	errors,
	files,
};
fs.mkdirSync(path.dirname(outFile), { recursive: true });
fs.writeFileSync(outFile, JSON.stringify(out, (k, v) => (typeof v === 'bigint' ? v.toString() : v), 1) + '\n');
console.error(`wrote ${outFile}: ${Object.keys(specs).length} specs, ${Object.keys(errors).length} errors`);
for (const [k, v] of Object.entries(errors)) console.error(`  ${k}: ${v}`);
if (Object.keys(errors).length) process.exit(1);
