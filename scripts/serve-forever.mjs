import http from 'node:http';
import { createReadStream, existsSync, statSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const directory = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../dist');
const mime = { '.html':'text/html; charset=utf-8', '.js':'text/javascript', '.css':'text/css', '.json':'application/json', '.wasm':'application/wasm', '.bin':'application/octet-stream', '.png':'image/png', '.jpg':'image/jpeg', '.svg':'image/svg+xml', '.woff2':'font/woff2', '.ico':'image/x-icon', '.txt':'text/plain' };
export function serve(req, res) {
  let pathname;
  try { pathname = decodeURIComponent(new URL(req.url, 'http://localhost').pathname); } catch { res.writeHead(400); res.end(); return; }
  if (pathname === '/' || pathname === '/forever-sim') { res.writeHead(302, { Location:'/forever-sim/' }); res.end(); return; }
  if (!pathname.startsWith('/forever-sim/')) { res.writeHead(404); res.end('Not Found'); return; }
  let file = path.resolve(directory, '.' + pathname);
  const appRoot = path.join(directory, 'forever-sim');
  if (file !== appRoot && !file.startsWith(appRoot + path.sep)) { res.writeHead(403); res.end(); return; }
  if (existsSync(file) && statSync(file).isDirectory()) {
    if (!pathname.endsWith('/')) { res.writeHead(302, { Location:pathname + '/' }); res.end(); return; }
    file = path.join(file, 'index.html');
  }
  if (!existsSync(file) || !statSync(file).isFile()) { res.writeHead(404); res.end('Not Found'); return; }
  res.writeHead(200, { 'Content-Type':mime[path.extname(file)] || 'application/octet-stream', 'Cache-Control':'no-cache', 'X-Content-Type-Options':'nosniff' });
  if (req.method === 'HEAD') res.end(); else createReadStream(file).pipe(res);
}
if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const port = Number(process.env.PORT || 4173);
  http.createServer(serve).listen(port, '127.0.0.1', () => console.log(`Forever Simulator: http://127.0.0.1:${port}/forever-sim/`));
}
