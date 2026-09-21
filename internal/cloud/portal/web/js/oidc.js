// Login del portal contra Keycloak: código de autorización + PKCE, cliente
// público. Sin secreto de cliente, porque una SPA no puede guardar ninguno.
import { b64u, sha256, utf8 } from './crypto.js';

const KEY = 'ccp.portal.session';
const PKCE = 'ccp.portal.pkce';

function store(k, v) {
  try { v === null ? sessionStorage.removeItem(k) : sessionStorage.setItem(k, JSON.stringify(v)); } catch { /* modo privado */ }
}

function load(k) {
  try { return JSON.parse(sessionStorage.getItem(k) || 'null'); } catch { return null; }
}

// discover lee los endpoints del emisor y exige que sean del MISMO origen: si
// el descubrimiento pudiera mandar el código a otro sitio, el login sería el
// camino para robarlo. Es la misma regla que el cliente de la CLI.
export async function discover(issuer) {
  const r = await fetch(issuer + '/.well-known/openid-configuration');
  if (!r.ok) throw new Error('no se pudo leer el emisor ' + issuer);
  const d = await r.json();
  const origin = new URL(issuer).origin;
  for (const u of [d.authorization_endpoint, d.token_endpoint]) {
    if (!u || new URL(u).origin !== origin) throw new Error('el emisor declara un endpoint de otro origen');
  }
  return d;
}

function rand(n) {
  return b64u(crypto.getRandomValues(new Uint8Array(n)));
}

export function redirectURI() { return location.origin + '/'; }

// begin manda al usuario a Keycloak. El verificador se queda en la pestaña:
// sin él, el código que vuelve no sirve de nada a quien lo intercepte.
export async function begin(issuer, clientID) {
  const d = await discover(issuer);
  const verifier = rand(32);
  const state = rand(16);
  store(PKCE, { verifier, state, token: d.token_endpoint, end: d.end_session_endpoint || '' });
  const q = new URLSearchParams({
    client_id: clientID,
    response_type: 'code',
    scope: 'openid profile email',
    redirect_uri: redirectURI(),
    state,
    code_challenge: b64u(await sha256(utf8(verifier))),
    code_challenge_method: 'S256',
  });
  location.assign(d.authorization_endpoint + '?' + q.toString());
}

// complete canjea el código si la URL trae uno. Devuelve true si hubo canje,
// para que el arranque sepa que acaba de iniciar sesión.
export async function complete(clientID) {
  const url = new URL(location.href);
  const code = url.searchParams.get('code');
  const err = url.searchParams.get('error');
  const pk = load(PKCE);
  if (err) {
    history.replaceState(null, '', '/');
    throw new Error('Keycloak rechazó el login: ' + err);
  }
  if (!code) return false;
  history.replaceState(null, '', '/');
  if (!pk || url.searchParams.get('state') !== pk.state) throw new Error('el estado del login no coincide; vuelve a intentarlo');
  const body = new URLSearchParams({
    grant_type: 'authorization_code', client_id: clientID, code,
    redirect_uri: redirectURI(), code_verifier: pk.verifier,
  });
  const r = await fetch(pk.token, {
    method: 'POST', headers: { 'content-type': 'application/x-www-form-urlencoded' }, body,
  });
  if (!r.ok) throw new Error('no se pudo canjear el código de login (' + r.status + ')');
  save(await r.json(), pk);
  store(PKCE, null);
  return true;
}

function save(tok, pk) {
  store(KEY, {
    access: tok.access_token,
    refresh: tok.refresh_token || '',
    expires: Date.now() + (tok.expires_in || 60) * 1000,
    token: pk.token,
    end: pk.end,
  });
}

export function signedIn() { return !!load(KEY); }

// token devuelve un acceso válido, renovándolo si le quedan menos de 30 s. El
// de Keycloak dura 5 minutos: sin renovar, el portal se caería solo a mitad de
// mirar una línea de tiempo.
export async function token(clientID) {
  const s = load(KEY);
  if (!s) return null;
  if (Date.now() < s.expires - 30000) return s.access;
  if (!s.refresh) { store(KEY, null); return null; }
  const r = await fetch(s.token, {
    method: 'POST', headers: { 'content-type': 'application/x-www-form-urlencoded' },
    body: new URLSearchParams({ grant_type: 'refresh_token', client_id: clientID, refresh_token: s.refresh }),
  });
  if (!r.ok) { store(KEY, null); return null; }
  const tok = await r.json();
  save(tok, { token: s.token, end: s.end });
  return tok.access_token;
}

// signOut borra la sesión de la pestaña y, si el emisor lo ofrece, cierra
// también la de Keycloak: quedarse con la sesión del navegador abierta hace
// que «salir» y volver a entrar no pregunte nada, que no es salir.
export function signOut(clientID) {
  const s = load(KEY);
  store(KEY, null);
  store(PKCE, null);
  if (s && s.end) {
    location.assign(s.end + '?' + new URLSearchParams({ post_logout_redirect_uri: redirectURI(), client_id: clientID }));
    return;
  }
  location.assign('/');
}
