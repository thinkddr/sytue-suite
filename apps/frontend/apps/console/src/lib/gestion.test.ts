/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La lógica pura de las herramientas de gestión: el CSV del directorio (que se abre
 * en una hoja de cálculo), el tope de «seleccionar las visibles», los fallos de un
 * lote, el filtro de dispositivos y los rangos rápidos de la auditoría.
 */
import { expect, test } from 'bun:test';
import {
  conSegundoFactor,
  csvDirectorio,
  diasParaRestaurar,
  fallosDelLote,
  filtrarDispositivos,
  MAX_LOTE,
  rangoRapido,
  seleccionarVisibles,
} from './gestion.js';
import type { Device } from './types.js';

// `csvCell` vive en audit-siem.ts, módulo de servidor que lee su configuración al
// importarse: el entorno mínimo antes, como en audit-siem.test.ts.
process.env.SYTUE_OIDC_ISSUER ??= 'https://id.ejemplo';
process.env.SYTUE_OIDC_CLIENT_ID ??= 'sytue-console';
process.env.SYTUE_OIDC_CLIENT_SECRET ??= 'secreto-de-prueba';
process.env.SYTUE_OIDC_REDIRECT_URI ??= 'https://admin.ejemplo/auth/callback';
process.env.SYTUE_OIDC_POST_LOGOUT_URI ??= 'https://www.ejemplo/';
process.env.SYTUE_SESSION_SECRET ??= 'x'.repeat(32);
const { csvCell } = await import('./audit-siem.js');

test('el CSV del directorio neutraliza fórmulas y entrecomilla lo que lo necesita', () => {
  const csv = csvDirectorio(
    [
      {
        email: 'ana@acme.test',
        name: '=HYPERLINK("http://malo")',
        role: 'admin',
        status: 'active',
        mfa_enabled: true,
        email_verified: false,
        department: 'Ventas, Norte',
        created_at: '2026-01-01T00:00:00Z',
      },
    ],
    csvCell,
  );
  const [cabecera, fila] = csv.trimEnd().split('\r\n');
  expect(cabecera).toBe(
    'email,name,role,status,mfa,email_verified,department,title,location,created_at',
  );
  // La celda que empieza por «=» sale como texto, nunca como fórmula.
  expect(fila).toContain(`"'=HYPERLINK(""http://malo"")"`);
  expect(fila).toContain('"Ventas, Norte"');
  expect(fila).toContain(',yes,no,');
});

test('seleccionar las visibles se queda en el tope del servicio y lo dice', () => {
  const ids = Array.from({ length: MAX_LOTE + 5 }, (_, i) => `u${i}`);
  const r = seleccionarVisibles(ids);
  expect(r.ids.size).toBe(MAX_LOTE);
  expect(r.recortado).toBe(true);
  expect(seleccionarVisibles(['a', 'b']).recortado).toBe(false);
});

test('los fallos de un lote son los que no salieron bien', () => {
  expect(
    fallosDelLote([
      { id: 'a', ok: true },
      { id: 'b', ok: false, code: 'last_owner' },
    ]),
  ).toEqual([{ id: 'b', ok: false, code: 'last_owner' }]);
  expect(fallosDelLote(undefined)).toEqual([]);
});

test('dispositivos: busca sin acentos en persona, equipo e IP, y filtra por confianza', () => {
  const d = (p: Partial<Device>): Device =>
    ({ id: p.id ?? 'x', name: '', trusted: false, userAgent: '', lastSeen: '', ...p }) as Device;
  const lista = [
    d({ id: '1', userName: 'Martín', name: 'Portátil', trusted: true }),
    d({ id: '2', userEmail: 'eva@acme.test', lastIp: '10.0.0.7' }),
  ];
  expect(filtrarDispositivos(lista, 'martin', '').map((x) => x.id)).toEqual(['1']);
  expect(filtrarDispositivos(lista, '10.0.0', '').map((x) => x.id)).toEqual(['2']);
  expect(filtrarDispositivos(lista, '', 'untrusted').map((x) => x.id)).toEqual(['2']);
  expect(filtrarDispositivos(lista, '', '')).toBe(lista);
});

test('rangos rápidos en hora local y días para restaurar', () => {
  const ahora = new Date(2026, 10, 25, 9, 30);
  expect(rangoRapido(0, ahora)).toEqual({ since: '2026-11-25', until: '2026-11-25' });
  expect(rangoRapido(7, ahora)).toEqual({ since: '2026-11-18', until: '2026-11-25' });
  // Cruza el cambio de mes.
  expect(rangoRapido(30, ahora).since).toBe('2026-10-26');

  const t = Date.parse('2026-11-25T00:00:00Z');
  expect(diasParaRestaurar('2026-11-27T00:00:00Z', t)).toBe(2);
  expect(diasParaRestaurar('2026-11-24T00:00:00Z', t)).toBe(0);
  expect(conSegundoFactor(['pwd', 'otp'])).toBe(true);
  expect(conSegundoFactor(['pwd'])).toBe(false);
});
