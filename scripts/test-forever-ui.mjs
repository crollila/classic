import {build} from 'vite';
import {pathToFileURL} from 'node:url';
import path from 'node:path';
await build({configFile:false,logLevel:'warn',build:{ssr:'scripts/forever-ui.test.ts',outDir:'.artifacts',emptyOutDir:false,minify:false,rollupOptions:{output:{entryFileNames:'forever-ui-tests.mjs'}}}});
await import(pathToFileURL(path.resolve('.artifacts/forever-ui-tests.mjs')).href);
