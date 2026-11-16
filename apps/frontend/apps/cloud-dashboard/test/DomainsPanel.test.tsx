/**
 * «Comprobar ahora» en los dominios propios: el botón vive en las instrucciones
 * DNS del dominio pendiente, pide la comprobación al BFF y pinta el MOTIVO en el
 * propio panel (no en un toast que se va antes de arreglar nada). Si el TXT está
 * bien, la fila pasa a verificada sin recargar. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { I18nProvider } from '@sytue/i18n/preact';
import type { Domain, DomainCheckResult } from '../src/lib/types.js';

// Idempotente: otro fichero de test puede haber registrado ya happy-dom.
if (typeof document === 'undefined') GlobalRegistrator.register();

const { default: DomainsPanel } = await import('../src/components/DomainsPanel.js');

const pendiente: Domain = {
  id: 'd1',
  host: 'www.tienda.es',
  kind: 'custom',
  verified: false,
  tls: 'pending',
  cname: 'ingress.sytue.eu',
  txtRecord: '_sytue-challenge.www.tienda.es',
  txtValue: 'sytue-verify=abc',
};

type Call = Record<string, unknown> | null;

/** BFF falso: contesta a /api/domains con `respuesta` y apunta cada cuerpo. */
function mockBff(respuesta: { status?: number; body: unknown }): Call[] {
  const calls: Call[] = [];
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    calls.push(init?.body ? (JSON.parse(String(init.body)) as Record<string, unknown>) : null);
    if (String(input) !== '/api/domains') return new Response('{}', { status: 404 });
    return new Response(JSON.stringify(respuesta.body), {
      status: respuesta.status ?? 200,
      headers: { 'content-type': 'application/json' },
    });
  }) as typeof fetch;
  return calls;
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

async function waitFor(check: () => boolean, ms = 3000): Promise<void> {
  const until = Date.now() + ms;
  while (!check()) {
    if (Date.now() > until)
      throw new Error('waitFor: timeout ' + (host?.textContent ?? '').slice(-600));
    await sleep(20);
  }
}

function button(h: HTMLElement, label: string): HTMLButtonElement {
  const b = Array.from(h.querySelectorAll('button')).find((x) => x.textContent?.trim() === label);
  if (!b) throw new Error(`no hay botón «${label}»`);
  return b;
}

let host: HTMLElement | undefined;
afterEach(() => {
  if (host) render(null, host);
  host?.remove();
  host = undefined;
});

function mount(): HTMLElement {
  host = document.createElement('div');
  document.body.appendChild(host);
  render(
    <I18nProvider locale="en">
      <DomainsPanel project="tienda" domains={[pendiente]} live locale="en" />
    </I18nProvider>,
    host,
  );
  return host;
}

async function comprobar(check: DomainCheckResult): Promise<{ h: HTMLElement; calls: Call[] }> {
  const calls = mockBff({ body: { ok: true, live: true, check } });
  const h = mount();
  button(h, 'Check now').click();
  return { h, calls };
}

test('sin TXT: dice cuál falta, en el panel, y el dominio sigue sin verificar', async () => {
  const { h, calls } = await comprobar({ verified: false, reason: 'txt_ausente' });
  await waitFor(() => h.textContent?.includes('TXT record not found') ?? false);
  expect(calls[0]).toEqual({ project: 'tienda', domain_id: 'd1', verify: true });
  expect(h.textContent).toContain('We could not find _sytue-challenge.www.tienda.es');
  expect(h.textContent).toContain('unverified');
  // Las instrucciones siguen a la vista: son justo lo que hay que arreglar.
  expect(h.textContent).toContain('Create these records in your DNS');
});

test('TXT con otro valor: lo dice como error, no como «pendiente»', async () => {
  const { h } = await comprobar({ verified: false, reason: 'txt_distinto' });
  await waitFor(() => h.textContent?.includes('The TXT record has a different value') ?? false);
  expect(h.querySelector('[role="alert"]')?.textContent).toContain('different value');
});

test('TXT bien: la fila pasa a verificada sin recargar', async () => {
  const { h } = await comprobar({ verified: true, reason: 'ok' });
  await waitFor(() => h.textContent?.includes('Domain verified') ?? false);
  expect(h.textContent).not.toContain('unverified');
  expect(h.textContent).not.toContain('Create these records in your DNS');
});

test('CNAME a otro sitio: verifica, pero dice a dónde apunta y a dónde debería', async () => {
  const { h } = await comprobar({
    verified: true,
    reason: 'cname_distinto',
    expected: 'ingress.sytue.eu',
    found: 'otro.hosting.example',
  });
  await waitFor(() => h.textContent?.includes('Verified, but traffic goes elsewhere') ?? false);
  expect(h.textContent).toContain(
    'www.tienda.es points to otro.hosting.example. Point its CNAME to ingress.sytue.eu',
  );
});

test('tope: un 429 se explica como «espera», no como fallo', async () => {
  mockBff({ status: 429, body: { error: 'too_many_requests' } });
  const h = mount();
  button(h, 'Check now').click();
  await waitFor(() => h.textContent?.includes('Wait a few seconds') ?? false);
  expect(h.textContent).not.toContain('Could not check the domain');
});

test('error del control-plane: lo dice como fallo y deja reintentar', async () => {
  mockBff({ status: 500, body: { error: 'internal' } });
  const h = mount();
  button(h, 'Check now').click();
  await waitFor(() => h.textContent?.includes('Could not check the domain') ?? false);
  expect(button(h, 'Check now').disabled).toBe(false);
});
