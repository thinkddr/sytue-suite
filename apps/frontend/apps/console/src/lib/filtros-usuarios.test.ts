// Los filtros de estado de Usuarios: los avisos de la portada enlazan a la lista
// con el filtro puesto, y lo que no puede pasar es que el filtro se pierda por el
// camino —que el servicio reciba la consulta sin él y devuelva a todo el mundo—
// ni que sin Sytue ID el seed lo ignore y enseñe gente que no está en ese estado.
// Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import type { SytueSession } from '@sytue/auth';
import type { DirectoryUser } from './types.js';

// `admin.ts` arrastra el wiring del BFF, que exige su configuración OIDC al
// cargarse. Se da aquí y se importa después, como en myorgs.test.ts.
process.env.SYTUE_OIDC_ISSUER = 'https://id.ejemplo';
process.env.SYTUE_OIDC_CLIENT_ID = 'sytue-console';
process.env.SYTUE_OIDC_CLIENT_SECRET = 'secreto-de-prueba';
process.env.SYTUE_OIDC_REDIRECT_URI = 'https://admin.ejemplo/auth/callback';
process.env.SYTUE_OIDC_POST_LOGOUT_URI = 'https://www.ejemplo/';
process.env.SYTUE_SESSION_SECRET = 'x'.repeat(32);

const { loadUsers } = await import('./admin.js');
const { cumpleFiltro, esFiltroUsuarios, filtroDeParametro, FILTROS_USUARIOS } =
  await import('./filtros-usuarios.js');

const sesion = {
  tokens: { accessToken: 'tok' },
  user: { tenant: 'acme' },
} as unknown as SytueSession;

test('los nombres son los de /admin/insights y lo demás no es un filtro', () => {
  expect([...FILTROS_USUARIOS]).toEqual(['without_mfa', 'suspended', 'unverified_email']);
  expect(esFiltroUsuarios('suspended')).toBe(true);
  expect(esFiltroUsuarios('todos')).toBe(false);
  expect(esFiltroUsuarios('')).toBe(false);
  expect(filtroDeParametro(' without_mfa ')).toBe('without_mfa');
  expect(filtroDeParametro('SUSPENDED')).toBe('');
  expect(filtroDeParametro(null)).toBe('');
});

test('el criterio es el del servicio: las suspendidas no cuentan como «sin segundo factor»', () => {
  const u = (enabled: boolean, mfaEnabled: boolean, emailVerified: boolean) =>
    ({ enabled, mfaEnabled, emailVerified }) as DirectoryUser;
  expect(cumpleFiltro(u(true, false, true), 'without_mfa')).toBe(true);
  expect(cumpleFiltro(u(false, false, true), 'without_mfa')).toBe(false);
  expect(cumpleFiltro(u(false, true, true), 'suspended')).toBe(true);
  expect(cumpleFiltro(u(true, true, false), 'unverified_email')).toBe(true);
  expect(cumpleFiltro(u(false, true, false), 'unverified_email')).toBe(false);
  expect(cumpleFiltro(u(false, false, false), '')).toBe(true);
});

test('loadUsers manda el filtro al servicio', async () => {
  const real = globalThis.fetch;
  const pedidos: string[] = [];
  globalThis.fetch = (async (url: string | URL | Request, init?: RequestInit) => {
    pedidos.push(`${String(url)} ${String(init?.body ?? '')}`);
    return new Response(JSON.stringify({ items: [], nextCursor: '' }), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    });
  }) as unknown as typeof fetch;
  try {
    const { live } = await loadUsers(sesion, undefined, { filter: 'unverified_email', q: 'ana' });
    expect(live).toBe(true);
  } finally {
    globalThis.fetch = real;
  }
  expect(pedidos).toHaveLength(1);
  expect(pedidos[0]).toContain('/users');
  expect(pedidos[0]).toContain('filter=unverified_email');
  expect(pedidos[0]).toContain('q=ana');
});

test('sin Sytue ID, el seed también se filtra', async () => {
  const real = globalThis.fetch;
  globalThis.fetch = (() => Promise.reject(new Error('sin red'))) as unknown as typeof fetch;
  try {
    for (const f of FILTROS_USUARIOS) {
      const { live, data } = await loadUsers(sesion, undefined, { filter: f });
      expect(live).toBe(false);
      expect(data.items.every((u) => cumpleFiltro(u, f))).toBe(true);
    }
    const todos = await loadUsers(sesion);
    const suspendidos = await loadUsers(sesion, undefined, { filter: 'suspended' });
    // El seed tiene de todo: si el filtro no hiciera nada, las dos listas medirían igual.
    expect(suspendidos.data.items.length).toBeLessThan(todos.data.items.length);
  } finally {
    globalThis.fetch = real;
  }
});
