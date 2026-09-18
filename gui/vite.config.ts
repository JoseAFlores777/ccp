import { defineConfig, type Plugin } from 'vite';
import react from '@vitejs/plugin-react';
import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process';
import { createInterface } from 'node:readline';

// Puente de desarrollo: en `npm run dev` (navegador, sin Tauri) la interfaz
// habla con `ccp serve --stdio` a través de POST /__ccp. Es el mismo protocolo
// que usa la app de escritorio, así que lo que funciona aquí funciona allí.
//
// CCP_GUI_BIN elige el binario (por defecto `ccp` del PATH). Para no tocar tu
// configuración real mientras se desarrolla, `npm run sandbox` prepara un HOME y
// un CCP_HOME de pruebas en .sandbox y `npm run dev:sandbox` arranca con
// CCP_SANDBOX apuntando ahí: solo el proceso de ccp ve ese HOME, Vite no.
function ccpDevBridge(): Plugin {
  let child: ChildProcessWithoutNullStreams | null = null;
  let nextId = 1;
  const pending = new Map<number, (msg: unknown) => void>();

  const childEnv = () => {
    const env: NodeJS.ProcessEnv = { ...process.env };
    let bin = process.env.CCP_GUI_BIN || 'ccp';
    const sandbox = process.env.CCP_SANDBOX;
    if (sandbox) {
      env.HOME = `${sandbox}/home`;
      env.CCP_HOME = `${sandbox}/ccp`;
      if (!process.env.CCP_GUI_BIN) bin = `${sandbox}/bin/ccp`;
      for (const k of ['CCP_PROFILE', 'CLAUDE_CONFIG_DIR', 'ANTHROPIC_BASE_URL', 'ANTHROPIC_AUTH_TOKEN']) delete env[k];
    }
    return { env, bin };
  };

  const start = () => {
    const { env, bin } = childEnv();
    child = spawn(bin, ['serve', '--stdio'], { env });
    const rl = createInterface({ input: child.stdout });
    rl.on('line', (line) => {
      let msg: { id?: number; event?: string };
      try {
        msg = JSON.parse(line);
      } catch {
        return;
      }
      if (typeof msg.id === 'number' && pending.has(msg.id)) {
        pending.get(msg.id)!(msg);
        pending.delete(msg.id);
      }
    });
    child.stderr.on('data', (d) => process.stderr.write(`[ccp serve] ${d}`));
    child.on('exit', (code) => {
      process.stderr.write(`[ccp serve] salió con ${code}\n`);
      child = null;
      for (const [, resolve] of pending) resolve({ error: { code: 'bridge_down', message: 'ccp serve terminó' } });
      pending.clear();
    });
  };

  return {
    name: 'ccp-dev-bridge',
    configureServer(server) {
      server.middlewares.use('/__ccp', (req, res) => {
        let body = '';
        req.on('data', (c) => (body += c));
        req.on('end', () => {
          if (!child) start();
          const { method, params } = JSON.parse(body || '{}');
          const id = nextId++;
          pending.set(id, (msg) => {
            res.setHeader('content-type', 'application/json');
            res.end(JSON.stringify(msg));
          });
          child!.stdin.write(JSON.stringify({ id, method, params: params ?? {} }) + '\n');
        });
      });
      server.middlewares.use('/__native', (req, res) => {
        // Las acciones nativas (abrir una terminal, revelar en Finder) solo
        // existen en la app; en el navegador se devuelve lo que se haría.
        let body = '';
        req.on('data', (c) => (body += c));
        req.on('end', () => {
          res.setHeader('content-type', 'application/json');
          res.end(JSON.stringify({ simulated: true, request: JSON.parse(body || '{}') }));
        });
      });
    },
  };
}

export default defineConfig({
  plugins: [react(), ccpDevBridge()],
  clearScreen: false,
  server: { port: 1420, strictPort: true },
  envPrefix: ['VITE_', 'TAURI_ENV_'],
  // La app carga el paquete del disco, no de la red: un solo archivo grande no
  // cuesta nada, y partirlo solo añadiría peticiones.
  build: { target: 'safari15', outDir: 'dist', sourcemap: false, chunkSizeWarningLimit: 1500 },
});
