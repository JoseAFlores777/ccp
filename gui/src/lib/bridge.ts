// bridge.ts — cómo llega la interfaz al motor de ccp.
//
// En la app de escritorio (Tauri) las llamadas van a Rust, que mantiene vivo un
// `ccp serve --stdio`. En el navegador (npm run dev) van al puente de Vite, que
// hace lo mismo con el mismo protocolo. La interfaz no sabe cuál de los dos
// tiene delante salvo para las acciones nativas (abrir una terminal, elegir una
// carpeta), que en el navegador se simulan.

import { invoke } from '@tauri-apps/api/core';

export const isTauri = typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window;

export class CcpError extends Error {
  code: string;
  constructor(code: string, message: string) {
    super(message);
    this.code = code;
  }
}

function toCcpError(e: unknown): CcpError {
  if (e instanceof CcpError) return e;
  const raw = typeof e === 'string' ? e : e instanceof Error ? e.message : JSON.stringify(e);
  try {
    const parsed = JSON.parse(raw) as { code?: string; message?: string };
    if (parsed && typeof parsed.message === 'string') return new CcpError(parsed.code ?? 'failed', parsed.message);
  } catch {
    /* no era JSON */
  }
  return new CcpError('failed', raw);
}

export async function ccpCall<T = unknown>(method: string, params: object = {}): Promise<T> {
  if (isTauri) {
    try {
      return await invoke<T>('ccp_call', { method, params });
    } catch (e) {
      throw toCcpError(e);
    }
  }
  let res: Response;
  try {
    res = await fetch('/__ccp', { method: 'POST', body: JSON.stringify({ method, params }) });
  } catch (e) {
    throw new CcpError('bridge_down', e instanceof Error ? e.message : String(e));
  }
  const msg = (await res.json()) as { result?: T; error?: { code: string; message: string } };
  if (msg.error) throw new CcpError(msg.error.code, msg.error.message);
  return msg.result as T;
}

export interface BridgeInfo {
  binary: string;
  source: 'installed' | 'bundled' | 'env' | 'dev';
  fallback_reason?: string;
}

export async function bridgeInfo(): Promise<BridgeInfo> {
  if (isTauri) return invoke<BridgeInfo>('ccp_bridge_info');
  return { binary: 'ccp (puente de Vite)', source: 'dev' };
}

async function native(action: string, payload: object): Promise<{ simulated?: boolean }> {
  if (isTauri) {
    await invoke(action, payload as Record<string, unknown>);
    return {};
  }
  await fetch('/__native', { method: 'POST', body: JSON.stringify({ action, ...payload }) });
  return { simulated: true };
}

/** Abre una terminal nueva en `cwd` con los comandos ya escritos. Cada
 *  argumento viaja separado y Rust los cita: nunca se construye una línea de
 *  shell con datos sin escapar. */
export function openTerminal(cwd: string | null, cmds: string[][]) {
  return native('open_terminal', { cwd, cmds });
}

/** Enseña un archivo o carpeta en Finder. */
export function revealPath(path: string) {
  return native('reveal_path', { path });
}

export async function pickFolder(defaultPath?: string): Promise<string | null> {
  if (isTauri) {
    const { open } = await import('@tauri-apps/plugin-dialog');
    const r = await open({ directory: true, multiple: false, defaultPath });
    return typeof r === 'string' ? r : null;
  }
  return window.prompt('Carpeta (ruta absoluta)', defaultPath ?? '') || null;
}

export async function pickSaveFile(defaultPath: string): Promise<string | null> {
  if (isTauri) {
    const { save } = await import('@tauri-apps/plugin-dialog');
    const r = await save({ defaultPath, filters: [{ name: 'Backup', extensions: ['gz'] }] });
    return r ?? null;
  }
  return window.prompt('Guardar en (ruta absoluta)', defaultPath) || null;
}

export async function pickOpenFile(): Promise<string | null> {
  if (isTauri) {
    const { open } = await import('@tauri-apps/plugin-dialog');
    const r = await open({ multiple: false, directory: false, filters: [{ name: 'Backup', extensions: ['gz'] }] });
    return typeof r === 'string' ? r : null;
  }
  return window.prompt('Archivo de copia (ruta absoluta)') || null;
}

export async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    return false;
  }
}
