import {createServer} from 'vite';
const server=await createServer({configFile:false,root:'ui',optimizeDeps:{noDiscovery:true,include:[]},server:{host:'127.0.0.1',port:5178,strictPort:true},define:{'import.meta.env.VITE_FOREVER':'"true"'}});
await server.listen();server.printUrls();
