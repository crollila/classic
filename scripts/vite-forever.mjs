import { readFileSync,writeFileSync,unlinkSync,rmSync } from 'node:fs';
import { fileURLToPath,pathToFileURL } from 'node:url';
import path from 'node:path';
import { transform } from 'esbuild';
import { build,createServer } from 'vite';
// Compile only our config. Avoid ancestor discovery and OS account lookups in
// sandboxed Windows sessions; normal Vite still builds the application.
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
const source=path.join(root,'forever.config.mts');
const compiled=path.join(root,`.forever-vite-config-${process.pid}.mjs`);
const {code}=await transform(readFileSync(source,'utf8'),{loader:'ts',format:'esm',sourcefile:source,define:{'import.meta.url':JSON.stringify(pathToFileURL(source).href)}});
let config;
try{writeFileSync(compiled,code);config=(await import(pathToFileURL(compiled).href)).default}finally{try{unlinkSync(compiled)}catch{}}
if(process.argv.includes('--dev')){const server=await createServer({...config,configFile:false});await server.listen();server.printUrls()}
else {
 // Generated bundles have content hashes. Remove stale bundles while retaining
 // the worker/WASM files produced earlier in the build pipeline.
 const output=path.resolve(root,'dist','forever-sim');
 const bundles=path.resolve(output,'bundle');
 if(path.dirname(bundles)!==output)throw new Error('Unexpected generated bundle directory');
 rmSync(bundles,{recursive:true,force:true});
 await build({...config,configFile:false});
}
