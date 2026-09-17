import {build} from 'vite';
import {pathToFileURL} from 'node:url';
import path from 'node:path';
await build({configFile:false,logLevel:'warn',build:{ssr:'scripts/talent-window.test.ts',outDir:'.artifacts',emptyOutDir:false,minify:false,rollupOptions:{output:{entryFileNames:'talent-window-tests.mjs'}}}});
await import(pathToFileURL(path.resolve('.artifacts/talent-window-tests.mjs')).href);
