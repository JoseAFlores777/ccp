// mcp.ts — de los campos del formulario a la entrada `mcpServers` que lee
// Claude Code. Cubre las tres formas de conectar un servidor MCP:
//
//   stdio  un programa local que Claude Code arranca (command + args + env)
//   http   un servidor remoto por HTTP (url + headers)
//   sse    un servidor remoto por SSE, la forma antigua (url + headers)
//
// y una cuarta, «JSON», para lo que no quepa en los campos: se pega la entrada
// tal cual. Todo es puro: la pantalla solo enseña el resultado y los errores.

import { t } from './i18n';

export type McpTransport = 'stdio' | 'http' | 'sse' | 'json';

export interface McpBuild {
  config?: Record<string, unknown>;
  errors: string[];
  warnings: string[];
}

const NAME_RE = /^[A-Za-z0-9._-]+$/;
const ENV_KEY_RE = /^[A-Za-z_][A-Za-z0-9_]*$/;
// Nombre de cabecera: los caracteres «token» de HTTP.
const HEADER_RE = /^([A-Za-z0-9!#$%&'*+.^_`|~-]+)\s*:\s*(.*)$/;
const URL_RE = /^https?:\/\/\S+$/i;

/** Las líneas con contenido; las que empiezan por # son comentarios. */
function lines(text: string | undefined): string[] {
  return (text ?? '')
    .split('\n')
    .map((l) => l.replace(/\r$/, '').trim())
    .filter((l) => l !== '' && !l.startsWith('#'));
}

/** Un valor que no lee nada del entorno (sin ${…}) queda escrito en claro. */
function literal(v: string): boolean {
  return v.trim() !== '' && !/\$\{[A-Za-z_][A-Za-z0-9_]*(:-[^}]*)?\}/.test(v);
}

const SECRET_NAME = /(key|token|secret|passw|pass$|auth|credential|private|cookie|session)/i;

/** ¿Un secreto escrito en claro? Por el nombre (API_KEY, Authorization…) o
 *  porque el valor tiene pinta de token: largo, sin espacios, letras y números. */
function secretInClear(name: string, v: string): boolean {
  if (!literal(v)) return false;
  if (SECRET_NAME.test(name)) return true;
  const w = v.trim();
  return w.length >= 24 && /^[A-Za-z0-9_\-.~+/=]+$/.test(w) && /[0-9]/.test(w) && /[A-Za-z]/.test(w);
}

export function parseEnv(text: string | undefined): { env: Record<string, string>; errors: string[]; literal: boolean } {
  const env: Record<string, string> = {};
  const errors: string[] = [];
  let lit = false;
  for (const l of lines(text)) {
    const i = l.indexOf('=');
    const k = i < 0 ? l : l.slice(0, i).trim();
    if (i < 0 || !ENV_KEY_RE.test(k)) {
      errors.push(t('Variable mal escrita: «{l}». Va como NOMBRE=valor.', { l }));
      continue;
    }
    const v = l.slice(i + 1).trim();
    env[k] = v;
    if (secretInClear(k, v)) lit = true;
  }
  return { env, errors, literal: lit };
}

export function parseHeaders(text: string | undefined): { headers: Record<string, string>; errors: string[]; literal: boolean } {
  const headers: Record<string, string> = {};
  const errors: string[] = [];
  let lit = false;
  for (const l of lines(text)) {
    const m = HEADER_RE.exec(l);
    if (!m) {
      errors.push(t('Cabecera mal escrita: «{l}». Va como Nombre: valor.', { l }));
      continue;
    }
    headers[m[1]] = m[2].trim();
    if (secretInClear(m[1], m[2])) lit = true;
  }
  return { headers, errors, literal: lit };
}

export function buildMcp(f: Record<string, string>): McpBuild {
  const errors: string[] = [];
  const warnings: string[] = [];
  const name = (f.name ?? '').trim();
  if (!name) errors.push(t('Falta el nombre del servidor.'));
  else if (!NAME_RE.test(name)) errors.push(t('El nombre solo admite letras, números, punto, guion y guion bajo.'));

  const transport = (f.transport || 'stdio') as McpTransport;
  let config: Record<string, unknown> | undefined;

  if (transport === 'stdio') {
    const command = (f.command ?? '').trim();
    if (!command) errors.push(t('Falta el programa que arranca el servidor (por ejemplo npx, uvx o docker).'));
    else if (/\s/.test(command) && !command.startsWith('/')) {
      warnings.push(t('El programa lleva espacios: si son argumentos, van en su campo, uno por línea.'));
    }
    const args = lines(f.args);
    const env = parseEnv(f.env);
    errors.push(...env.errors);
    if (env.literal) warnings.push(t('Hay una variable que parece un secreto escrita en claro. Escribe ${VARIABLE} en su lugar y Claude Code la leerá de tu entorno al arrancar.'));
    config = { type: 'stdio', command, ...(args.length ? { args } : {}), ...(Object.keys(env.env).length ? { env: env.env } : {}) };
  } else if (transport === 'http' || transport === 'sse') {
    const url = (f.url ?? '').trim();
    if (!url) errors.push(t('Falta la URL del servidor.'));
    else if (!URL_RE.test(url)) errors.push(t('La URL tiene que empezar por http:// o https://.'));
    else if (/^http:\/\//i.test(url) && !/^http:\/\/(localhost|127\.0\.0\.1|\[::1\])([:/]|$)/i.test(url)) {
      warnings.push(t('La URL no va cifrada (http://): las cabeceras, token incluido, viajan en claro.'));
    }
    const h = parseHeaders(f.headers);
    errors.push(...h.errors);
    if (h.literal) warnings.push(t('Hay una cabecera que parece un token escrito en claro. Escribe ${VARIABLE} en su lugar y Claude Code lo leerá de tu entorno al arrancar.'));
    config = { type: transport, url, ...(Object.keys(h.headers).length ? { headers: h.headers } : {}) };
  } else {
    const raw = (f.json ?? '').trim();
    if (!raw) {
      errors.push(t('Falta el JSON de la entrada.'));
    } else {
      try {
        const v: unknown = JSON.parse(raw);
        if (!v || typeof v !== 'object' || Array.isArray(v)) {
          errors.push(t('El JSON tiene que ser un objeto: la entrada de un servidor, sin el nombre.'));
        } else {
          const o = v as Record<string, unknown>;
          if ('mcpServers' in o) errors.push(t('Pega solo la entrada del servidor, sin «mcpServers» ni el nombre: el nombre va en su campo.'));
          else if (!('command' in o) && !('url' in o)) errors.push(t('La entrada necesita «command» (servidor local) o «url» (servidor remoto).'));
          config = o;
        }
      } catch (e) {
        errors.push(t('El JSON no es válido: {e}', { e: e instanceof Error ? e.message : String(e) }));
      }
    }
  }
  return { config: errors.length ? undefined : config, errors, warnings };
}
