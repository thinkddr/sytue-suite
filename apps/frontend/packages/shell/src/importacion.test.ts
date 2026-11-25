/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import type { ImportJob, ImportOrigin } from '@sytue/api-clients';
import { claveDeFallo, esDeOrigen, proveedoresRepetidos, repartir } from './importacion.js';

function origen(o: Partial<ImportOrigin> & Pick<ImportOrigin, 'instanceId'>): ImportOrigin {
  return {
    instanceIds: [o.instanceId],
    name: 'Google',
    source: 'google',
    accountKey: '',
    accountLabel: '',
    state: 'ready',
    ...o,
  };
}

function trabajo(j: Partial<ImportJob> & Pick<ImportJob, 'id'>): ImportJob {
  return {
    instanceId: 'a',
    domain: 'mail',
    source: 'google',
    status: 'done',
    migrated: 0,
    skipped: 0,
    errors: 0,
    lastError: '',
    errorCode: '',
    accountKey: '',
    accountLabel: '',
    createdAt: '2026-11-01T10:00:00Z',
    ...j,
  };
}

const acme = origen({ instanceId: 'a', instanceIds: ['a', 'a2'], accountKey: 'google:acme.com' });
const beta = origen({ instanceId: 'b', accountKey: 'google:beta.com' });
const m365 = origen({ instanceId: 'm', source: 'm365', state: 'unavailable' });

test('un trabajo es del origen por su cuenta, o por cualquiera de sus conexiones', () => {
  expect(
    esDeOrigen(trabajo({ id: '1', accountKey: 'google:acme.com', instanceId: 'x' }), acme),
  ).toBe(true);
  expect(esDeOrigen(trabajo({ id: '2', instanceId: 'a2' }), acme)).toBe(true);
  expect(
    esDeOrigen(trabajo({ id: '3', accountKey: 'google:beta.com', instanceId: 'a' }), acme),
  ).toBe(false);
  expect(esDeOrigen(trabajo({ id: '4', source: 'm365', instanceId: 'a' }), acme)).toBe(false);
});

test('solo el proveedor con varias cuentas lleva la cuenta en el botón', () => {
  expect([...proveedoresRepetidos([acme, beta, m365])]).toEqual(['google']);
  expect([...proveedoresRepetidos([acme, m365])]).toEqual([]);
});

test('una cuenta que ya está trayendo no se ofrece otra vez', () => {
  const r = repartir(
    [acme, beta, m365],
    [
      trabajo({ id: 'c', instanceId: 'a2', status: 'running' }),
      trabajo({ id: 'd', domain: 'drive', accountKey: 'google:beta.com', status: 'running' }),
    ],
    ['mail', 'contacts'],
  );
  expect(r.libres.map((o) => o.instanceId)).toEqual(['b', 'm']);
  // El trabajo de Drive no es de esta app: ni ocupa a beta ni sale en el resumen.
  expect(r.trabajando.map((j) => j.id)).toEqual(['c']);
});

test('el resumen separa terminados y fallidos, y sabe si mezcla cuentas', () => {
  const una = repartir(
    [acme],
    [
      trabajo({ id: 'm', accountKey: 'google:acme.com' }),
      trabajo({ id: 'c', domain: 'contacts', accountKey: 'google:acme.com', status: 'error' }),
    ],
    ['mail', 'contacts'],
  );
  expect(una.hechos.map((j) => j.id)).toEqual(['m']);
  expect(una.fallidos.map((j) => j.id)).toEqual(['c']);
  expect(una.variasCuentas).toBe(false);
  const dos = repartir(
    [acme, beta],
    [
      trabajo({ id: 'm', accountKey: 'google:acme.com' }),
      trabajo({ id: 'n', accountKey: 'google:beta.com' }),
    ],
    ['mail'],
  );
  expect(dos.variasCuentas).toBe(true);
});

test('un fallo con arreglo se dice por su motivo, no como «inténtalo luego»', () => {
  expect(claveDeFallo('connection_expired')).toBe('import.unavailable');
  expect(claveDeFallo('import_connection_unavailable')).toBe('import.unavailable');
  expect(claveDeFallo('needs_admin')).toBe('import.needsAdmin');
  expect(claveDeFallo('')).toBe('import.someFailed');
});
