// El cliente del API desde el navegador. Mismo origen que el portal, así que
// no hay CORS que negociar ni cookies que mandar: el token va en la cabecera.
import * as oidc from './oidc.js';

let clientID = '';
let device = '';

export function configure(id) { clientID = id; }
export function deviceID() { return device; }

export class ApiError extends Error {
  constructor(status, code, message) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

export async function req(method, path, body, opts = {}) {
  const t = await oidc.token(clientID);
  if (!t) throw new ApiError(401, 'unauthorized', 'la sesión ha caducado');
  const headers = { authorization: 'Bearer ' + t };
  if (device && !opts.noDevice) headers['x-ccp-device'] = device;
  if (body !== undefined) headers['content-type'] = 'application/json';
  const r = await fetch(path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) });
  if (r.status === 204) return null;
  const text = await r.text();
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch { /* el error no era JSON */ }
  if (!r.ok) {
    throw new ApiError(r.status, (data && data.code) || 'http_' + r.status,
      (data && data.message) || 'el servidor respondió ' + r.status);
  }
  return data;
}

// info es lo único que se lee sin sesión: dice a dónde ir a iniciarla.
export async function info() {
  const r = await fetch('/v1/info');
  if (!r.ok) throw new ApiError(r.status, 'info', 'el servidor no contesta a /v1/info');
  return r.json();
}

const DEV = 'ccp.portal.device.';

// ensureDevice da de alta ESTE navegador como un equipo más de la cuenta. El
// API exige cabecera de dispositivo en todo lo que no sea el alta, y el
// portal, que también publica revisiones, tiene que ser identificable en la
// auditoría como cualquier otra máquina.
export async function ensureDevice(userID, name) {
  const key = DEV + userID;
  let id = '';
  try { id = localStorage.getItem(key) || ''; } catch { /* modo privado */ }
  if (id) {
    device = id;
    try {
      await req('GET', '/v1/devices');
      return id;
    } catch (e) {
      // 403 es «desconocido o revocado»: este navegador vuelve a darse de alta.
      // Cualquier otro error no se tapa dando de alta un equipo más.
      if (!(e instanceof ApiError) || e.status !== 403) throw e;
      device = '';
    }
  }
  const d = await req('POST', '/v1/devices', { name, platform: 'portal', ccp_version: '' }, { noDevice: true });
  device = d.id;
  try { localStorage.setItem(key, d.id); } catch { /* modo privado */ }
  return d.id;
}

// blob baja un blob sellado por el MISMO origen que el resto del API. El
// portal no puede ir al bucket: su CSP solo deja salir hacia aquí y hacia
// Keycloak, y el bucket tampoco responde CORS a una pestaña.
export async function blob(id) {
  const t = await oidc.token(clientID);
  if (!t) throw new ApiError(401, 'unauthorized', 'la sesión ha caducado');
  const r = await fetch('/v1/blobs/' + encodeURIComponent(id),
    { headers: { authorization: 'Bearer ' + t, 'x-ccp-device': device } });
  if (!r.ok) throw new ApiError(r.status, 'blob', 'no se pudo bajar un contenido (' + r.status + ')');
  return new Uint8Array(await r.arrayBuffer());
}

// putBlob sube uno. 201 es «lo subí yo» y 200 «ya estaba»; ninguno es error.
export async function putBlob(id, sealed) {
  const t = await oidc.token(clientID);
  if (!t) throw new ApiError(401, 'unauthorized', 'la sesión ha caducado');
  const r = await fetch('/v1/blobs/' + encodeURIComponent(id), {
    method: 'PUT',
    headers: { authorization: 'Bearer ' + t, 'x-ccp-device': device, 'content-type': 'application/octet-stream' },
    body: sealed,
  });
  if (!r.ok) throw new ApiError(r.status, 'blob', 'no se pudo subir un contenido (' + r.status + ')');
}

export const presign = (op, ids) => req('POST', '/v1/blobs/presign', { op, ids });
export const commitSnapshot = (in_) => req('POST', '/v1/snapshots', in_);
export const publishRevision = (in_) => req('POST', '/v1/revisions', in_);

export const me = () => req('GET', '/v1/me');
export const devices = () => req('GET', '/v1/devices');
export const vault = () => req('GET', '/v1/vault');
export const snapshots = (dev, limit = 200) =>
  req('GET', '/v1/snapshots?limit=' + limit + (dev ? '&device=' + encodeURIComponent(dev) : ''));
export const snapshot = (id) => req('GET', '/v1/snapshots/' + encodeURIComponent(id));
export const revisions = (dev, limit = 20) =>
  req('GET', '/v1/revisions?limit=' + limit + (dev ? '&device=' + encodeURIComponent(dev) : ''));

// Grupos de dispositivos («todas mis Macs», §10.3). Un grupo es una etiqueta
// con miembros y no autoriza nada: aplicar a un grupo sigue siendo publicar
// una revisión FIRMADA por miembro.
export const groups = () => req('GET', '/v1/groups');
export const createGroup = (in_) => req('POST', '/v1/groups', in_);
export const updateGroup = (id, in_) => req('PUT', '/v1/groups/' + encodeURIComponent(id), in_);
export const deleteGroup = (id) => req('DELETE', '/v1/groups/' + encodeURIComponent(id));
export const groupStatus = (id) => req('GET', '/v1/groups/' + encodeURIComponent(id) + '/status');
