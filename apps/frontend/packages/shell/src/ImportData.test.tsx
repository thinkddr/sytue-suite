/* SPDX-License-Identifier: Apache-2.0 */
/**
 * «Trae tus datos» con varias cuentas: un botón por CUENTA y con la cuenta en el rótulo
 * cuando hay más de una del mismo proveedor; la cuenta que ya trae no se ofrece otra vez;
 * la conexión que no puede leer se enseña apagada y dice por qué; el resumen cuenta UNA
 * vez cada cuenta y contenido; y «Reintentar» relanza SOLO lo que falló.
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, beforeAll, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { catalogs } from '@sytue/i18n';
import type { ImportJob, ImportOrigin } from '@sytue/api-clients';

let registradoAqui = false;
beforeAll(() => {
  if (typeof document === 'undefined') {
    GlobalRegistrator.register({ url: 'https://sytue.test/mail' });
    registradoAqui = true;
  }
});
afterAll(async () => {
  if (registradoAqui) await GlobalRegistrator.unregister();
});

const en = catalogs.en;
const fetchOriginal = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = fetchOriginal;
  document.body.innerHTML = '';
});

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
    accountKey: 'google:acme.com',
    accountLabel: 'acme.com',
    createdAt: '2026-11-01T10:00:00Z',
    ...j,
  };
}

/** Monta el bloque con un BFF de mentira; devuelve los cuerpos de los POST. */
async function montar(origins: ImportOrigin[], jobs: ImportJob[]): Promise<unknown[]> {
  const lanzados: unknown[] = [];
  globalThis.fetch = (async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === 'POST') {
      const cuerpo = JSON.parse(String(init.body)) as { domains: string[] };
      lanzados.push(cuerpo);
      return Response.json(
        {
          jobs: cuerpo.domains.map((d) =>
            trabajo({ id: `n-${d}`, domain: d as ImportJob['domain'], status: 'queued' }),
          ),
        },
        { status: 202 },
      );
    }
    return Response.json({ origins, jobs });
  }) as typeof fetch;
  const { ImportData } = await import('./ImportData.js');
  const host = document.createElement('div');
  document.body.appendChild(host);
  await act(async () => {
    render(<ImportData domains={['mail', 'contacts']} />, host);
  });
  for (let i = 0; i < 3; i++) await act(async () => await Promise.resolve());
  return lanzados;
}

function botones(): string[] {
  return [...document.querySelectorAll('button')].map((b) => b.textContent ?? '');
}

test('un botón por cuenta, con la cuenta solo si hay más de una del mismo proveedor', async () => {
  await montar(
    [
      origen({ instanceId: 'a', accountKey: 'google:acme.com', accountLabel: 'acme.com' }),
      origen({ instanceId: 'b', accountKey: 'google:beta.com', accountLabel: 'beta.com' }),
      origen({ instanceId: 'm', source: 'm365', state: 'unavailable' }),
    ],
    [trabajo({ id: 'r', instanceId: 'a', status: 'running' })],
  );
  const texto = document.body.textContent ?? '';
  expect(botones()).toContain(`${en['import.fromGoogle']} · beta.com`);
  // acme ya está trayendo: no se ofrece otra vez.
  expect(botones()).not.toContain(`${en['import.fromGoogle']} · acme.com`);
  // Microsoft no puede leer: apagado y con el motivo.
  const ms = [...document.querySelectorAll('button')].find(
    (b) => b.textContent === en['import.fromMicrosoft'],
  );
  expect(ms?.disabled).toBe(true);
  expect(texto).toContain(en['import.unavailable'].replace('{source}', 'Microsoft 365'));
});

test('el resumen cuenta la importación vigente y «Reintentar» relanza lo que falló', async () => {
  const lanzados = await montar(
    [origen({ instanceId: 'a', accountKey: 'google:acme.com', accountLabel: 'acme.com' })],
    [
      trabajo({ id: 'm', migrated: 1250, skipped: 3, finishedAt: '2026-11-03T10:00:00Z' }),
      trabajo({
        id: 'c',
        domain: 'contacts',
        status: 'error',
        errorCode: 'connection_expired',
      }),
    ],
  );
  const texto = document.body.textContent ?? '';
  expect(texto).toContain(en['dataflow.item.mail']);
  expect(texto).toContain('Email: 1,250 items brought across, 3 left out. Finished on 3 Nov 2026.');
  expect(texto).toContain(en['import.unavailable'].replace('{source}', 'Google'));
  expect(texto).not.toContain(en['import.someFailed']);
  const reintentar = [...document.querySelectorAll('button')].find(
    (b) => b.textContent === en['action.retry'],
  );
  expect(reintentar).toBeDefined();
  await act(async () => {
    reintentar!.click();
    await Promise.resolve();
  });
  expect(lanzados).toEqual([{ instanceId: 'a', source: 'google', domains: ['contacts'] }]);
});
