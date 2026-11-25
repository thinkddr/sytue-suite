/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La auditoría unificada pide la traza de acceso de cada servicio que la tiene
 * (docs/security.md §1.7) y no solo la de cumplimiento: un servicio que escribe su
 * `audit_log` y que la consola no lee es una traza que nadie mira. Contra un fetch
 * falso que contesta por host. Ejecutar: `bun test`.
 */
import { afterEach, expect, test } from 'bun:test';
import type { SytueSession } from '@sytue/auth';

process.env.SYTUE_OIDC_ISSUER = 'https://id.ejemplo';
process.env.SYTUE_OIDC_CLIENT_ID = 'sytue-console';
process.env.SYTUE_OIDC_CLIENT_SECRET = 'secreto-de-prueba';
process.env.SYTUE_OIDC_REDIRECT_URI = 'https://admin.ejemplo/auth/callback';
process.env.SYTUE_OIDC_POST_LOGOUT_URI = 'https://www.ejemplo/';
process.env.SYTUE_SESSION_SECRET = 'x'.repeat(32);
process.env.SYTUE_ID_INTERNAL_URL = 'http://id.interno';
process.env.SYTUE_INTEGRATIONS_BASE_URL = 'http://integrations.interno';
for (const s of ['MAIL', 'DRIVE', 'NOTES', 'DOCS', 'ROOMS', 'CALENDAR', 'CONTACTS', 'HUB']) {
  process.env[`SYTUE_${s}_INTERNAL_URL`] = `http://${s.toLowerCase()}.interno`;
}

const { ALL_SOURCES, loadUnifiedAudit, parseAuditFilters } = await import('./audit-siem.js');

const sesion = { tokens: { accessToken: 'tok' } } as SytueSession;
const original = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = original;
});

/** Contesta a cada servicio con una línea suya; los `caidos`, 502. */
function servicios(caidos: string[] = []): string[] {
  const pedidas: string[] = [];
  globalThis.fetch = ((input: RequestInfo | URL) => {
    const url = new URL(input instanceof Request ? input.url : String(input));
    // El servicio es la primera etiqueta del host: vale con las URL internas de
    // aquí y con las públicas por defecto si otro test cargó antes `auth.ts`.
    const svc = url.hostname.split('.')[0]!;
    pedidas.push(`${svc}${url.pathname}`);
    if (caidos.includes(svc)) return Promise.resolve(new Response('', { status: 502 }));
    const linea = {
      occurred_at: '2026-11-25T10:00:00Z',
      actor: 'ana',
      action: `acceso.${svc}.algo`,
      target: `obj-${svc}`,
      detail: { ip: '192.0.2.1' },
    };
    return Promise.resolve(
      new Response(JSON.stringify({ items: svc === 'integrations' ? [] : [linea] }), {
        status: 200,
        headers: { 'content-type': 'application/json' },
      }),
    );
  }) as typeof fetch;
  return pedidas;
}

test('pide la traza de acceso de Docs, Rooms, Calendar, Contactos y el Hub', async () => {
  const pedidas = servicios();
  const { records, sources } = await loadUnifiedAudit(sesion, {});
  for (const s of ['mail', 'drive', 'notes', 'docs', 'rooms', 'calendar', 'contacts', 'hub']) {
    expect(pedidas).toContain(`${s}/api/v1/admin/audit-log`);
    const r = records.find((x) => x.service === s);
    expect(r?.action).toBe(`acceso.${s}.algo`);
    expect(r?.target).toBe(`obj-${s}`);
  }
  expect(sources.map((s) => s.source).sort()).toEqual([...ALL_SOURCES].sort());
  expect(sources.every((s) => s.ok)).toBe(true);
});

test('una fuente caída se dice sin tapar a las demás', async () => {
  servicios(['rooms']);
  const { records, sources } = await loadUnifiedAudit(sesion, {});
  expect(sources.find((s) => s.source === 'rooms')?.ok).toBe(false);
  expect(sources.find((s) => s.source === 'hub')?.ok).toBe(true);
  expect(records.some((r) => r.service === 'rooms')).toBe(false);
  expect(records.some((r) => r.service === 'calendar')).toBe(true);
});

test('el filtro por servicio acepta las fuentes nuevas', () => {
  const f = parseAuditFilters(new URLSearchParams('service=contacts&service=hub&service=otra'));
  expect(f.services).toEqual(['contacts', 'hub']);
});
