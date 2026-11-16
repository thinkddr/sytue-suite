/**
 * El asistente «Nuevo proyecto» de punta a punta con el BFF simulado: origen →
 * configurar → variables → revisar → desplegar, y el log del build en el último
 * paso. Y el caso que justifica que el alta sea reanudable: si una variable no se
 * puede guardar, el error sale en el paso de despliegue con su motivo y
 * «Reintentar» sigue desde ahí sin volver a crear el proyecto.
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { I18nProvider } from '@sytue/i18n/preact';

// Idempotente: otro fichero de test puede haber registrado ya happy-dom.
if (typeof document === 'undefined') GlobalRegistrator.register();

const { default: NewProjectWizard } = await import('../src/components/NewProjectWizard.js');

type Call = { url: string; body: Record<string, unknown> | null };

/** BFF falso: responde por ruta y apunta cada llamada. */
function mockBff(opts: { failEnvOnce?: string } = {}) {
  const calls: Call[] = [];
  let envFailed = false;
  const json = (body: unknown, status = 200) =>
    new Response(JSON.stringify(body), {
      status,
      headers: { 'content-type': 'application/json' },
    });
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    const body = init?.body ? (JSON.parse(String(init.body)) as Record<string, unknown>) : null;
    calls.push({ url, body });
    if (url.startsWith('/api/inspect-repo'))
      return json({
        ok: true,
        live: true,
        detected: true,
        detection: { framework: 'astro', confident: true, evidence: 'astro.config.mjs' },
      });
    if (url.startsWith('/api/check-slug')) return json({ ok: true, live: true, available: true });
    if (url.startsWith('/api/projects'))
      return json({
        ok: true,
        live: true,
        project: {
          slug: 'tienda',
          name: 'tienda',
          productionUrl: 'https://tienda.apps.sytue.eu',
          webhookUrl: 'https://cloud.sytue.eu/webhooks/git',
          webhookSecret: 'whsec-123',
        },
      });
    if (url.startsWith('/api/env')) {
      if (opts.failEnvOnce && body?.key === opts.failEnvOnce && !envFailed) {
        envFailed = true;
        return json({ error: 'quota_exceeded' }, 402);
      }
      return json({ ok: true, live: true });
    }
    if (url === '/api/deploy') return json({ ok: true, live: true });
    if (url.startsWith('/api/deployments'))
      return json({
        live: true,
        deployment: { id: 'd1', status: 'ready', url: 'https://tienda.apps.sytue.eu' },
      });
    if (url.startsWith('/api/logs'))
      return json({
        live: true,
        logs: [{ ts: '', level: 'info', source: 'build', message: 'npm run build: done' }],
      });
    return json({ error: 'not_found' }, 404);
  }) as typeof fetch;
  return calls;
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

async function waitFor(check: () => boolean, ms = 3000): Promise<void> {
  const until = Date.now() + ms;
  while (!check()) {
    if (Date.now() > until)
      throw new Error('waitFor: timeout ' + (host?.textContent ?? '').slice(-900));
    await sleep(20);
  }
}

function button(host: HTMLElement, label: string): HTMLButtonElement {
  const b = Array.from(host.querySelectorAll('button')).find(
    (x) => x.textContent?.trim() === label,
  );
  if (!b) throw new Error(`no hay botón «${label}»`);
  return b;
}

function type(el: Element | null, value: string) {
  const input = el as HTMLInputElement;
  input.value = value;
  input.dispatchEvent(new Event('input', { bubbles: true }));
}

function mount(): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(
    <I18nProvider locale="en">
      <NewProjectWizard locale="en" />
    </I18nProvider>,
    host,
  );
  return host;
}

let host: HTMLElement | undefined;
afterEach(() => {
  if (host) render(null, host);
  host?.remove();
});

/** Recorre los cuatro primeros pasos con una variable y pulsa «Create and deploy». */
async function fillAndCreate(h: HTMLElement) {
  // Origen: sin una URL https no se puede seguir.
  expect(button(h, 'Continue').disabled).toBe(true);
  type(h.querySelector('#npw-repo'), 'https://git.sytue.eu/acme/tienda');
  await waitFor(() => h.textContent?.includes('We read your repository') ?? false);
  expect(h.textContent).toContain('Astro, from astro.config.mjs');
  button(h, 'Continue').click();

  // Configurar: el nombre y la dirección salen del repositorio.
  await waitFor(() => h.querySelector('#npw-slug') !== null);
  expect((h.querySelector('#npw-slug') as HTMLInputElement).value).toBe('tienda');
  await waitFor(() => h.textContent?.includes('tienda is available.') ?? false);
  expect((h.querySelector('#npw-fw') as HTMLSelectElement).value).toBe('astro');
  button(h, 'Continue').click();

  // Variables: opcionales; una clave mal escrita impide seguir.
  await waitFor(() => h.textContent?.includes('Continue without variables') ?? false);
  button(h, 'Add variable').click();
  await sleep(0);
  type(h.querySelector('input[aria-label="Key of variable 1"]'), '1BAD');
  type(h.querySelector('input[aria-label="Value of variable 1"]'), 'abc');
  await sleep(0);
  expect(button(h, 'Continue').disabled).toBe(true);
  type(h.querySelector('input[aria-label="Key of variable 1"]'), 'API_KEY');
  await sleep(0);
  expect(button(h, 'Continue').disabled).toBe(false);
  button(h, 'Continue').click();

  // Revisar: todo junto antes de tocar nada.
  await waitFor(() => h.textContent?.includes('This is what is about to happen') ?? false);
  expect(h.textContent).toContain('tienda.apps.sytue.eu');
  expect(h.textContent).toContain('API_KEY');
  button(h, 'Create and deploy').click();
}

test('de punta a punta: crea, guarda las variables, despliega y enseña el log', async () => {
  const calls = mockBff();
  host = mount();
  await fillAndCreate(host);

  const h = host;
  await waitFor(() => h.textContent?.includes('Your app is online') ?? false);
  const posts = calls.filter((c) => c.body !== null).map((c) => c.url);
  expect(posts.filter((u) => !u.includes('inspect-repo') && !u.includes('check-slug'))).toEqual([
    '/api/projects',
    '/api/env',
    '/api/deploy',
  ]);
  const created = calls.find((c) => c.url === '/api/projects')!.body!;
  expect(created).toMatchObject({
    slug: 'tienda',
    repo_url: 'https://git.sytue.eu/acme/tienda',
    framework: 'astro',
  });
  expect(calls.find((c) => c.url === '/api/env')!.body).toMatchObject({
    project: 'tienda',
    key: 'API_KEY',
    value: 'abc',
    scope: 'all',
  });
  // El log del build y el secreto del webhook, que solo se entrega una vez.
  expect(calls.some((c) => c.url === '/api/logs?project=tienda&source=build')).toBe(true);
  expect(h.textContent).toContain('Build log');
  // Sin maquetación (happy-dom) la lista virtual no pinta filas; que el aviso de
  // espera se haya ido es lo que prueba que llegaron líneas REALES del build.
  expect(h.textContent).not.toContain('Waiting for the build to start');
  expect(h.textContent).toContain('whsec-123');
  expect(h.textContent).toContain('Go to the project');
});

test('si una variable falla, el error sale en el despliegue y «Try again» sigue desde ahí', async () => {
  const calls = mockBff({ failEnvOnce: 'API_KEY' });
  host = mount();
  await fillAndCreate(host);

  const h = host;
  await waitFor(() => h.textContent?.includes('Could not save API_KEY') ?? false);
  expect(calls.some((c) => c.url === '/api/deploy')).toBe(false);

  button(h, 'Try again').click();
  await waitFor(() => h.textContent?.includes('Your app is online') ?? false);
  // El proyecto se creó UNA vez: reintentar no choca con su propio nombre.
  expect(calls.filter((c) => c.url === '/api/projects')).toHaveLength(1);
  expect(calls.filter((c) => c.url === '/api/env')).toHaveLength(2);
  expect(calls.filter((c) => c.url === '/api/deploy')).toHaveLength(1);
  expect(h.textContent).not.toContain('Could not save API_KEY');
});
