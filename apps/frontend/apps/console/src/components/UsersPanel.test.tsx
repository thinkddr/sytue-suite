/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El filtro de estado de Usuarios y los avisos de la portada que enlazan a él.
 * Lo que se prueba es el camino entero: el aviso lleva a la lista con su filtro,
 * la lista pinta ese filtro elegido, cambiarlo pide al SERVIDOR (no recorta lo
 * cargado), «Cargar más» lo conserva y la URL lo recuerda. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { DirectoryUser } from '../lib/types.js';
import type { OrgInsights } from '../lib/admin.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

// La portada arrastra, a través de la guía de primeros pasos, el wiring del BFF,
// que exige su configuración OIDC al cargarse. Se da antes de importar.
process.env.SYTUE_OIDC_ISSUER ??= 'https://id.ejemplo';
process.env.SYTUE_OIDC_CLIENT_ID ??= 'sytue-console';
process.env.SYTUE_OIDC_CLIENT_SECRET ??= 'secreto-de-prueba';
process.env.SYTUE_OIDC_REDIRECT_URI ??= 'https://admin.ejemplo/auth/callback';
process.env.SYTUE_OIDC_POST_LOGOUT_URI ??= 'https://www.ejemplo/';
process.env.SYTUE_SESSION_SECRET ??= 'x'.repeat(32);
// En bun `import.meta.env` es `process.env`: el prefijo de la consola para `r()`.
process.env.BASE_URL ??= '/admin/';

const { default: UsersPanel } = await import('./UsersPanel.js');
const { default: DashboardPanel } = await import('./DashboardPanel.js');

const persona = (id: string, extra: Partial<DirectoryUser> = {}): DirectoryUser => ({
  id,
  username: id,
  email: `${id}@acme.test`,
  fullName: id,
  enabled: true,
  emailVerified: true,
  roles: ['member'],
  groups: ['org/acme'],
  tenant: 'acme',
  mfaEnabled: false,
  createdAt: '2026-11-01T10:00:00Z',
  provisioning: { drive: 'synced', chat: 'synced', mail: 'synced' },
  department: '',
  jobTitle: '',
  location: '',
  ...extra,
});

let pedidos: string[] = [];
function servidor(paginas: Record<string, { items: DirectoryUser[]; nextCursor?: string }>) {
  pedidos = [];
  globalThis.fetch = (async (url: string) => {
    pedidos.push(String(url));
    const u = new URL(String(url), 'https://admin.ejemplo');
    if (u.pathname !== '/api/admin/users') return new Response(null, { status: 404 });
    const clave = `${u.searchParams.get('filter') ?? ''}|${u.searchParams.get('cursor') ?? ''}`;
    return new Response(JSON.stringify(paginas[clave] ?? { items: [], nextCursor: '' }), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    });
  }) as unknown as typeof fetch;
}

/** Pone la URL de la página. happy-dom arranca en about:blank, y ahí
 *  `replaceState` con una ruta no cambia nada. */
function irA(ruta: string) {
  (window as unknown as { happyDOM: { setURL: (u: string) => void } }).happyDOM.setURL(
    `https://admin.ejemplo${ruta}`,
  );
}

let host: HTMLElement | null = null;
afterEach(async () => {
  if (host) await act(async () => render(null, host!));
  host = null;
  document.body.innerHTML = '';
  irA('/admin/acme/users');
});

async function esperar(ms = 20) {
  await act(async () => {
    await new Promise((r) => setTimeout(r, ms));
  });
}

async function montar(vnode: preact.VNode) {
  host = document.createElement('div');
  document.body.appendChild(host);
  await act(async () => render(vnode, host!));
  await esperar();
}

const selectorDeEstado = () =>
  document.querySelector<HTMLSelectElement>('select[aria-label="Filtrar por estado"]');

test('la lista llega filtrada y el desplegable lo dice', async () => {
  irA('/admin/acme/users?filter=without_mfa');
  servidor({});
  await montar(
    <UsersPanel
      locale="es"
      users={[persona('sin-mfa')]}
      tenants={[]}
      nextCursor=""
      filter="without_mfa"
    />,
  );
  const sel = selectorDeEstado();
  expect(sel).not.toBeNull();
  expect(sel!.value).toBe('without_mfa');
  // La primera página ya vino filtrada del servidor: no se vuelve a pedir.
  expect(pedidos.filter((p) => p.startsWith('/api/admin/users?'))).toHaveLength(0);
});

test('cambiar el filtro pide al servidor, conserva el filtro al cargar más y lo apunta en la URL', async () => {
  irA('/admin/acme/users?__org=acme');
  servidor({
    'suspended|': { items: [persona('fuera', { enabled: false })], nextCursor: 'c2' },
    'suspended|c2': { items: [persona('fuera2', { enabled: false })], nextCursor: '' },
  });
  await montar(
    <UsersPanel
      locale="es"
      users={[persona('ana', { mfaEnabled: true }), persona('luis', { mfaEnabled: true })]}
      tenants={[]}
      nextCursor="c1"
    />,
  );
  const sel = selectorDeEstado()!;
  expect(sel.value).toBe('');

  await act(async () => {
    sel.value = 'suspended';
    sel.dispatchEvent(new Event('change', { bubbles: true }));
  });
  await esperar();

  expect(pedidos).toContain('/api/admin/users?filter=suspended');
  // La respuesta sustituye a lo cargado: con una suspendida, la lista no está vacía.
  expect(document.body.textContent).not.toContain('Nadie coincide');
  // Lo que ya estaba en la URL se conserva; el filtro se añade.
  expect(window.location.search).toContain('filter=suspended');
  expect(window.location.search).toContain('__org=acme');

  const mas = [...document.querySelectorAll('button')].find(
    (b) => b.textContent?.trim() === 'Cargar más',
  );
  expect(mas).toBeDefined();
  await act(async () => {
    mas!.click();
  });
  await esperar();
  expect(pedidos).toContain('/api/admin/users?filter=suspended&cursor=c2');

  // Volver a «todas» quita el filtro de la URL y de la consulta.
  await act(async () => {
    sel.value = '';
    sel.dispatchEvent(new Event('change', { bubbles: true }));
  });
  await esperar();
  expect(pedidos.at(-1)).toBe('/api/admin/users?');
  expect(window.location.search).not.toContain('filter=');
});

test('con el filtro puesto y nadie en ese estado, la lista vacía lo cuenta como buena noticia', async () => {
  servidor({});
  await montar(
    <UsersPanel locale="es" users={[]} tenants={[]} nextCursor="" filter="without_mfa" />,
  );
  expect(document.body.textContent).toContain('Todas las personas activas tienen segundo factor.');
});

test('los avisos de la portada llevan a Usuarios con su filtro puesto', async () => {
  const insights: OrgInsights = {
    without_mfa: 3,
    suspended: 1,
    unverified_email: 2,
    invitations_expired: 0,
    invitations_pending: 0,
  };
  await montar(
    <DashboardPanel metrics={[]} tenants={[]} events={[]} insights={insights} locale="es" />,
  );
  const enlaces = [...document.querySelectorAll('a')].map((a) => a.getAttribute('href') ?? '');
  for (const f of ['without_mfa', 'suspended', 'unverified_email']) {
    expect(enlaces.some((h) => h.endsWith(`/users?filter=${f}`))).toBe(true);
  }
});
