import { build, createServer } from 'vite';
import config from '../forever.config.mjs';

// Load via tsx so Windows builds do not need Vite's config bundler to scan
// inaccessible ancestor directories. The application still uses normal Vite.
if (process.argv.includes('--dev')) {
  const server = await createServer({ ...config, configFile: false });
  await server.listen();
  server.printUrls();
} else {
  await build({ ...config, configFile: false });
}
