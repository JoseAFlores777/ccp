// El login del portal, con el navegador de mentira. Lo ejecuta oidc_test.go.
let fallos = 0;
const check = (nombre, ok, extra) => {
  if (!ok) { fallos++; console.error('FALLA', nombre, extra ?? ''); } else { console.log('ok  ', nombre); }
};

const almacen = new Map();
globalThis.sessionStorage = {
  getItem: (k) => (almacen.has(k) ? almacen.get(k) : null),
  setItem: (k, v) => almacen.set(k, String(v)),
  removeItem: (k) => almacen.delete(k),
};
globalThis.location = { origin: 'https://ccp.example.com', href: 'https://ccp.example.com/', assign(u) { this.href = u; } };
globalThis.history = { replaceState() {} };

const ISS = 'https://auth.example.com/realms/ccp';
const DISC = {
  authorization_endpoint: ISS + '/protocol/openid-connect/auth',
  token_endpoint: ISS + '/protocol/openid-connect/token',
  end_session_endpoint: ISS + '/protocol/openid-connect/logout',
};
let peticiones = [];
let respuestas = [];
globalThis.fetch = async (url, opts) => {
  peticiones.push({ url, opts });
  const r = respuestas.shift();
  if (!r) throw new Error('fetch inesperado a ' + url);
  return { ok: r.ok !== false, status: r.status || 200, json: async () => r.body, text: async () => JSON.stringify(r.body) };
};

const oidc = await import('./web/js/oidc.js');
const { b64u, sha256, utf8 } = await import('./web/js/crypto.js');

// 1. El descubrimiento no puede desviar el token a otro origen.
respuestas = [{ body: { ...DISC, token_endpoint: 'https://otro.example.com/token' } }];
let err = '';
await oidc.discover(ISS).catch((e) => { err = e.message; });
check('endpoint de otro origen rechazado', err.includes('otro origen'), err);

// 2. begin manda a Keycloak con PKCE S256 y guarda el verificador aquí.
respuestas = [{ body: DISC }];
await oidc.begin(ISS, 'ccp-portal');
const u = new URL(location.href);
check('va al endpoint de autorización', u.origin + u.pathname === DISC.authorization_endpoint);
check('cliente público', u.searchParams.get('client_id') === 'ccp-portal');
check('código de autorización', u.searchParams.get('response_type') === 'code');
check('vuelve a la raíz del portal', u.searchParams.get('redirect_uri') === 'https://ccp.example.com/');
check('pide openid', u.searchParams.get('scope').includes('openid'));
check('PKCE S256', u.searchParams.get('code_challenge_method') === 'S256');
const pk = JSON.parse(almacen.get('ccp.portal.pkce'));
check('el reto es el hash del verificador', u.searchParams.get('code_challenge') === b64u(await sha256(utf8(pk.verifier))));
check('el verificador no viaja', !location.href.includes(pk.verifier));

// 3. Un estado que no coincide no canjea nada.
location.href = 'https://ccp.example.com/?code=abc&state=otro';
peticiones = [];
err = '';
await oidc.complete('ccp-portal').catch((e) => { err = e.message; });
check('estado que no coincide', err.includes('estado') && peticiones.length === 0, err);

// 4. El canje: POST al token endpoint con el verificador.
almacen.set('ccp.portal.pkce', JSON.stringify(pk));
location.href = 'https://ccp.example.com/?code=abc&state=' + pk.state;
peticiones = [];
respuestas = [{ body: { access_token: 'AT', refresh_token: 'RT', expires_in: 300 } }];
check('canjea el código', (await oidc.complete('ccp-portal')) === true);
const body = new URLSearchParams(peticiones[0].opts.body.toString());
check('canje contra el token endpoint', peticiones[0].url === DISC.token_endpoint);
check('grant_type', body.get('grant_type') === 'authorization_code');
check('manda el verificador', body.get('code_verifier') === pk.verifier);
check('sesión iniciada', oidc.signedIn() && almacen.get('ccp.portal.pkce') === undefined);

// 5. El token vivo no pide red; el caducado se renueva.
respuestas = [];
check('token vivo sin red', (await oidc.token('ccp-portal')) === 'AT');
const s = JSON.parse(almacen.get('ccp.portal.session'));
almacen.set('ccp.portal.session', JSON.stringify({ ...s, expires: Date.now() - 1 }));
peticiones = [];
respuestas = [{ body: { access_token: 'AT2', refresh_token: 'RT2', expires_in: 300 } }];
check('renueva con el refresh', (await oidc.token('ccp-portal')) === 'AT2');
check('grant_type refresh_token', new URLSearchParams(peticiones[0].opts.body.toString()).get('grant_type') === 'refresh_token');

// 6. Si la renovación falla, la sesión se borra: un token muerto guardado deja
// al portal intentándolo en cada clic.
almacen.set('ccp.portal.session', JSON.stringify({ ...s, expires: Date.now() - 1 }));
respuestas = [{ ok: false, status: 400, body: {} }];
check('renovación fallida cierra la sesión', (await oidc.token('ccp-portal')) === null && !oidc.signedIn());

process.exit(fallos === 0 ? 0 : 1);
