/* SPDX-License-Identifier: Apache-2.0 */
/**
 * Vuelta del proveedor con `oauth_error=exists`: la cuenta elegida en Google ya estaba
 * conectada para este servicio. El servidor descartó el borrador y `instance` es la
 * cuenta que ya hay. Antes se veía «permiso no concedido» sobre el asistente.
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { catalogs } from '@sytue/i18n';

// Hace falta un origen: el componente construye la URL de su API con él.
const ORIGEN = 'https://sytue.test/calendar';
if (typeof document === 'undefined') GlobalRegistrator.register({ url: ORIGEN });
else
  (window as unknown as { happyDOM?: { setURL: (url: string) => void } }).happyDOM?.setURL(ORIGEN);
const en = catalogs.en;

const account = {
  instance_id: 'acc-1',
  provider: 'gmail',
  name: 'Gmail',
  address: 'ana@example.org',
  connected: true,
  services: ['mail', 'calendar'],
  capabilities: {},
};
const pedidas: string[] = [];
const conexiones: unknown[] = [];
globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
  const url = new URL(String(input));
  pedidas.push(url.pathname);
  if (url.pathname.endsWith('/external-mail/connect')) {
    conexiones.push(JSON.parse(String(init?.body)));
    // Sin URL que seguir: la prueba mira qué se pidió, no navega.
    return new Response('{}', { status: 503, headers: { 'content-type': 'application/json' } });
  }
  const body = url.pathname.endsWith('/external-mail/accounts')
    ? {
        items: [account],
        providers: ['gmail', 'microsoft'].map((provider) => ({
          provider,
          available: true,
          services: [{ service: 'calendar', available: true }],
        })),
      }
    : { items: [] };
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'content-type': 'application/json' },
  });
}) as unknown as typeof fetch;

const { LinkedServices } = await import('./LinkedServices.js');

afterEach(() => {
  document.body.innerHTML = '';
  pedidas.length = 0;
  conexiones.length = 0;
  sessionStorage.clear();
});

async function volverCon(query: string): Promise<void> {
  window.history.replaceState(null, '', `/calendar?${query}`);
  const host = document.createElement('div');
  document.body.appendChild(host);
  await act(async () => {
    render(
      <LinkedServices
        service="calendar"
        basePath="/calendar"
        labels={{ create: 'New', title: 'Title', text: 'Text' }}
      />,
      host,
    );
  });
  for (let i = 0; i < 3; i++)
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
    });
}

test('una cuenta ya conectada se abre con un aviso, no con «permiso no concedido»', async () => {
  await volverCon('connect-linked=1&instance=acc-1&oauth_error=exists');
  const texto = document.body.textContent ?? '';
  expect(texto).toContain(en['linkedServices.exists']);
  expect(texto).not.toContain(en['linkedServices.notGranted']);
  expect(document.querySelector('[role="status"]')?.textContent).toContain(
    en['linkedServices.exists'],
  );
  // Abre la cuenta: pide sus colecciones en vez de quedarse en el asistente.
  expect(pedidas.some((p) => p.endsWith('/calendar/collections'))).toBe(true);
  // Y limpia la vuelta para que recargar no repita el aviso.
  expect(window.location.search).toBe('');
});

test('cualquier otro fallo de la vuelta sigue siendo «permiso no concedido»', async () => {
  await volverCon('connect-linked=1&instance=acc-1&oauth_error=scope');
  const texto = document.body.textContent ?? '';
  expect(texto).toContain(en['linkedServices.notGranted']);
  expect(texto).not.toContain(en['linkedServices.exists']);
});

// Los borradores no se listan: al volver de un Outlook cancelado el asistente
// enseñaba Gmail y «Iniciar sesión» llevaba a Google.
test('al volver de un inicio cancelado se sigue con el proveedor que se eligió', async () => {
  sessionStorage.setItem(
    'sytue.external-mail.consent',
    JSON.stringify({ instance_id: 'draft-ms', provider: 'microsoft', services: ['calendar'] }),
  );
  await volverCon('connect-linked=1&instance=draft-ms&oauth_error=denied');
  const elegida = document.querySelector<HTMLInputElement>('input[name="linked-provider"]:checked');
  expect(elegida?.closest('label')?.textContent).toContain('Microsoft');
  // Atendida la vuelta, se olvida: otra vuelta ilegible no la nombraría.
  expect(sessionStorage.getItem('sytue.external-mail.consent')).toBeNull();
  // El del asistente, junto a las tarjetas de proveedor (la cabecera tiene otro).
  const entrar = [...document.querySelectorAll('button')].find(
    (b) =>
      b.textContent?.trim() === en['account.signIn'] &&
      b.parentElement?.querySelector('input[name="linked-provider"]'),
  );
  expect(entrar).toBeDefined();
  await act(async () => entrar?.click());
  await act(async () => {
    await new Promise((r) => setTimeout(r, 0));
  });
  expect(conexiones).toEqual([
    {
      provider: 'microsoft',
      return_to: 'external-calendar',
      instance_id: 'draft-ms',
      services: ['calendar'],
    },
  ]);
});
