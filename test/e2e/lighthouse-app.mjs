// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
/*
 * LA SUPERFICIE DE APP DE LIGHTHOUSE-CI (`.lighthouserc.apps.json`).
 *
 * El presupuesto de las apps (`sytue:app-ready` junto al LCP) no regía: Lighthouse-CI
 * solo recogía www, que es estático y no pone la marca. Una app de verdad pide sesión
 * de Sytue ID y sus servicios Go, y en CI no hay ni lo uno ni lo otro (ni secretos que
 * meter). Esto es lo que hizo el arnés de medida del ADR 0086, dentro del repositorio:
 *
 * - el SERVIDOR DE PRODUCCIÓN de la app, el que sale de `bun run build`
 *   (`dist/server/entry.mjs`), sin tocar nada de su código;
 * - una sesión sellada con un secreto efímero (`sealSession`, la misma que usan las
 *   pruebas de `account-capacity.mjs`): sin IdP, sin tokens reales;
 * - sus servicios simulados, con un cuaderno con notas de verdad (las de
 *   `notepads-dinamico.mjs`): lo que la app pinta antes de poner la marca;
 * - y delante, un proxy que comprime como Traefik y pone la cookie de sesión en
 *   cada petición, porque Lighthouse arranca cada pasada con un perfil vacío.
 *
 * La bóveda se abre después de la marca y aquí no existe: falla contra los servicios
 * simulados y se queda en el candado, que es lo que pasa sin cifrado configurado. No
 * cuenta para el presupuesto.
 *
 * Uso (desde la raíz del repositorio, tras `bun run --filter '@sytue/notes' build`):
 *   bun test/e2e/lighthouse-app.mjs        # escribe «lighthouse-app: listo …»
 *   LHCI_APP_PORT=4790 (por defecto)
 */
import { spawn } from 'node:child_process';
import { existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import {
  hostCookieName,
  sealSession,
  SESSION_COOKIE,
} from '../../apps/frontend/packages/auth/src/server/cookies.ts';

const RAIZ = fileURLToPath(new URL('../../', import.meta.url));
const PUERTO = Number(process.env.LHCI_APP_PORT ?? 4790);
const PUERTO_APP = PUERTO + 1;
const PUERTO_SERVICIOS = PUERTO + 2;
const SERVICIOS = `http://127.0.0.1:${PUERTO_SERVICIOS}`;
const ORG = 'prueba';
const CLIENTE = 'sytue-notes';
// Efímero: muere con el proceso. Nada fuera de aquí puede sellar ni leer la sesión.
const SECRETO = crypto.getRandomValues(new Uint8Array(32)).toHex();

const entrada = `${RAIZ}apps/frontend/apps/notes/dist/server/entry.mjs`;
if (!existsSync(entrada)) {
  console.error(`lighthouse-app: falta ${entrada}; antes, bun run --filter '@sytue/notes' build`);
  process.exit(1);
}

// --- Los servicios simulados ------------------------------------------------

const fecha = '2026-11-02T10:00:00Z';
const cuaderno = (id, title, position) => ({
  id,
  title,
  position,
  owner_sub: 'lighthouse',
  color: '#146a47',
  layout: 'list',
  visibility: 'private',
  indexable: false,
  encrypted: false,
  note_count: id === 'cuaderno-proyecto' ? 24 : 0,
  created_at: fecha,
  updated_at: fecha,
});
const cuadernos = [
  cuaderno('cuaderno-proyecto', 'Proyecto de noviembre', 0),
  cuaderno('cuaderno-ideas', 'Ideas por desarrollar', 1),
];
const notas = Array.from({ length: 24 }, (_, i) => ({
  id: `nota-${i}`,
  notebook_id: 'cuaderno-proyecto',
  owner_sub: 'lighthouse',
  kind: 'text',
  title: ['Preparar la reunión', 'Recoger las propuestas del equipo', 'Revisar el borrador'][i % 3],
  content: '<p>Una idea por línea, con sitio para escribir y <b>ordenar</b>.</p>',
  checked: false,
  collapsed: false,
  pinned: false,
  x: 0,
  y: 0,
  col: 0,
  position: i,
  group_id: null,
  width: 320,
  states: [],
  links: [],
  created_at: fecha,
  updated_at: fecha,
}));

Bun.serve({
  port: PUERTO_SERVICIOS,
  hostname: '127.0.0.1',
  async fetch(req) {
    const { pathname } = new URL(req.url);
    // La bóveda contesta tarde a propósito. Tras la marca, la puerta arranca el worker
    // del núcleo a la vez que pide la bóveda; si la bóveda falla antes de que el
    // worker reciba su manifiesto (otro 404), la puerta lo termina con esa petición a
    // medias, Lighthouse la da por abierta para siempre y espera a la red hasta su
    // tope en cada pasada.
    if (pathname.includes('/pq-vault/')) await Bun.sleep(500);
    if (req.method === 'GET' && pathname === '/api/v1/notebooks')
      return Response.json({ notebooks: cuadernos });
    const pagina = /^\/api\/v1\/notebooks\/([^/]+)\/notes\/page$/.exec(pathname);
    if (req.method === 'GET' && pagina)
      return Response.json({
        notes: notas.filter((n) => n.notebook_id === pagina[1]),
      });
    if (req.method === 'GET' && pathname === '/api/v1/tags') return Response.json({ tags: [] });
    // El resto (Hub, Sytue ID, Drive y la bóveda): no hay servicio. Responde YA, para
    // que nada espere a un DNS que en CI no existe.
    return Response.json({ error: 'not_found' }, { status: 404 });
  },
});

// --- El servidor de producción de la app ---------------------------------------

const entorno = {
  ...process.env,
  NODE_ENV: 'production',
  HOST: '127.0.0.1',
  PORT: String(PUERTO_APP),
  SYTUE_SESSION_SECRET: SECRETO,
  // El emisor solo sirve para el login, que aquí no ocurre (y tiene que ser https).
  SYTUE_OIDC_ISSUER: 'https://id.prueba.invalid',
  SYTUE_OIDC_INTERNAL_BASE_URL: SERVICIOS,
  SYTUE_OIDC_CLIENT_ID: CLIENTE,
  SYTUE_OIDC_CLIENT_SECRET: 'lighthouse',
  SYTUE_OIDC_REDIRECT_URI: `http://localhost:${PUERTO}/notes/auth/callback`,
  SYTUE_HUB_BASE_URL: `http://localhost:${PUERTO}/hub`,
};
for (const servicio of [
  'NOTES',
  'HUB',
  'DRIVE',
  'ROOMS',
  'DOCS',
  'MAIL',
  'CALENDAR',
  'CONTACTS',
  'PHOTOS',
  'BASES',
  'CAST',
  'ERP',
  'SYERP',
]) {
  entorno[`SYTUE_${servicio}_INTERNAL_URL`] = SERVICIOS;
  entorno[`SYTUE_${servicio}_API_URL`] ??= SERVICIOS;
}
const app = spawn('bun', [entrada], {
  env: entorno,
  stdio: ['ignore', 'inherit', 'inherit'],
});
const parar = () => {
  app.kill('SIGTERM');
  process.exit(0);
};
process.on('SIGINT', parar);
process.on('SIGTERM', parar);
app.on('exit', (code) => {
  console.error(`lighthouse-app: el servidor de la app terminó (${code})`);
  process.exit(1);
});

// --- La sesión y el proxy ----------------------------------------------------------

const sesion = await sealSession(
  {
    clientId: CLIENTE,
    user: {
      id: 'lighthouse',
      tenant: ORG,
      email: 'lighthouse@example.invalid',
      emailVerified: true,
      name: 'Lighthouse',
      preferredUsername: 'lighthouse',
      roles: [],
      groups: [],
    },
    tokens: {
      accessToken: 'lighthouse',
      expiresAt: Date.now() + 6 * 3600_000,
      tokenType: 'Bearer',
    },
  },
  SECRETO,
);
const cookie = `${hostCookieName(SESSION_COOKIE, true)}=${sesion}`;
const COMPRIMIBLE = /^(text\/|application\/(javascript|json|xml|manifest\+json)|image\/svg)/;

Bun.serve({
  port: PUERTO,
  hostname: '127.0.0.1',
  async fetch(req) {
    const url = new URL(req.url);
    // El Hub (avisos, búsqueda) va a otro origen en producción: aquí, a ninguno.
    if (url.pathname.startsWith('/hub/'))
      return Response.json({ error: 'not_found' }, { status: 404 });
    const cabeceras = new Headers(req.headers);
    cabeceras.set('cookie', [req.headers.get('cookie'), cookie].filter(Boolean).join('; '));
    cabeceras.set('x-forwarded-proto', 'https');
    cabeceras.set('x-forwarded-host', url.host);
    const res = await fetch(`http://127.0.0.1:${PUERTO_APP}${url.pathname}${url.search}`, {
      method: req.method,
      headers: cabeceras,
      body: req.method === 'GET' || req.method === 'HEAD' ? undefined : req.body,
      redirect: 'manual',
      decompress: false,
    });
    const tipo = res.headers.get('content-type') ?? '';
    // Como el middleware `compress` de Traefik: gzip en lo que es texto y no viene ya
    // comprimido (el wasm precomprimido de la app pasa tal cual).
    if (
      !res.body ||
      res.headers.has('content-encoding') ||
      !COMPRIMIBLE.test(tipo) ||
      !/\bgzip\b/.test(req.headers.get('accept-encoding') ?? '')
    )
      return res;
    const salida = new Headers(res.headers);
    salida.set('content-encoding', 'gzip');
    salida.delete('content-length');
    salida.append('vary', 'accept-encoding');
    return new Response(res.body.pipeThrough(new CompressionStream('gzip')), {
      status: res.status,
      headers: salida,
    });
  },
});

for (let i = 0; i < 120; i++) {
  try {
    const r = await fetch(`http://127.0.0.1:${PUERTO_APP}/notes/healthz`);
    if (r.status < 500) break;
  } catch {
    /* todavía arrancando */
  }
  await Bun.sleep(500);
}
console.log(`lighthouse-app: listo en http://localhost:${PUERTO}/notes/${ORG}/`);
