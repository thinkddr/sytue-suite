/**
 * Panel «Detalles» (`DetailsDialog.tsx`). Ejecutar: `bun test`.
 *
 * Se monta el panel de verdad, con `fetch` sustituido: mientras llega la respuesta hay
 * esqueleto; al llegar, la lista con cada dato; si falla, «Reintentar» vuelve a pedir.
 * Y se vigila el fuente del explorador para que «Detalles» salga en el menú «⋯».
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { render } from 'preact';
import type { DriveResource } from '@sytue/api-clients';
import { I18nProvider } from '@sytue/i18n/preact';
import { DetailsDialog } from './DetailsDialog';
import type { Detalles } from '../lib/detalles';

if (typeof document === 'undefined') GlobalRegistrator.register();

const tick = (ms = 150): Promise<void> => new Promise((r) => setTimeout(r, ms));
const original = globalThis.fetch;

let host: HTMLElement | null = null;
afterEach(() => {
  if (host) render(null, host);
  host?.remove();
  host = null;
  globalThis.fetch = original;
});

const ITEM: DriveResource = {
  id: 'f1',
  name: 'informe.pdf',
  path: '/Proyectos/2026/informe.pdf',
  isDirectory: false,
  size: 2048,
};

const DETALLES: Detalles = {
  kind: 'file',
  name: 'informe.pdf',
  path: '/Proyectos/2026/informe.pdf',
  contentType: 'application/pdf',
  size: 2048,
  owner: { name: 'Ana Ruiz', email: 'ana@org.eu' },
  createdAt: '2026-01-02T10:00:00Z',
  modifiedAt: '2026-11-01T09:30:00Z',
  modifiedBy: {},
  version: 4,
  label: { name: 'Confidencial', blockExternalShare: true },
  access: { links: 1, people: 2, groups: 3 },
};

function montar(props: Partial<Parameters<typeof DetailsDialog>[0]> = {}): void {
  host = document.body.appendChild(document.createElement('div'));
  render(
    <I18nProvider locale="es">
      <DetailsDialog item={ITEM} onClose={() => {}} {...props} />
    </I18nProvider>,
    host,
  );
}

/** Pares «término → definición» de la lista del panel. */
function lista(): Record<string, string> {
  const out: Record<string, string> = {};
  for (const dt of document.querySelectorAll('dl dt')) {
    out[dt.textContent ?? ''] = dt.nextElementSibling?.textContent ?? '';
  }
  return out;
}

test('esqueleto mientras carga; luego, todos los datos del fichero', async () => {
  let soltar: (r: Response) => void = () => {};
  const pedidas: string[] = [];
  globalThis.fetch = ((input: RequestInfo | URL) => {
    pedidas.push(String(input));
    return new Promise<Response>((r) => (soltar = r));
  }) as unknown as typeof fetch;
  montar();
  await tick();
  expect(document.querySelector('[data-detalles-esqueleto]')).not.toBeNull();
  expect(document.querySelector('dl')).toBeNull();
  expect(pedidas[0]).toContain('/api/details?id=f1&kind=file');

  soltar(new Response(JSON.stringify({ details: DETALLES }), { status: 200 }));
  await tick();
  expect(document.querySelector('[data-detalles-esqueleto]')).toBeNull();
  const d = lista();
  expect(d['Tipo']).toBe('Archivo PDF');
  expect(d['Tamaño']).toMatch(/2(,0)?\s?KB/);
  expect(d['Ubicación']).toBe('Mi unidad › Proyectos › 2026');
  expect(d['Propietario']).toBe('Ana Ruiz');
  expect(d['Creado']).toContain('2026');
  // Quien modificó no está en el directorio: «Alguien», nunca su identificador.
  expect(d['Modificado']).toContain('Alguien');
  expect(d['Versión']).toBe('4');
  expect(d['Clasificación']).toBe('Confidencial (sin enlaces públicos)');
  expect(d['Enlaces públicos']).toBe('1');
  expect(d['Personas con acceso']).toBe('2');
  expect(d['Grupos con acceso']).toBe('3');
  expect(document.querySelector('dl')?.className).toContain('tabular-nums');
});

test('si falla, «Reintentar» vuelve a pedir y pinta la respuesta', async () => {
  let llamadas = 0;
  globalThis.fetch = (async () => {
    llamadas += 1;
    if (llamadas === 1) return new Response('{}', { status: 502 });
    return new Response(JSON.stringify({ details: { ...DETALLES, access: null } }), {
      status: 200,
    });
  }) as unknown as typeof fetch;
  montar();
  await tick();
  const alerta = document.querySelector('[role="alert"]');
  expect(alerta?.textContent).toContain('No se pudieron cargar los detalles.');
  const boton = [...document.querySelectorAll('button')].find(
    (b) => b.textContent === 'Reintentar',
  );
  boton?.click();
  await tick();
  expect(llamadas).toBe(2);
  // Sin el listado de comparticiones no se afirma «0»: se dice que no hay dato.
  expect(lista()['Enlaces públicos']).toBe('No disponible');
});

test('los atajos abren el historial y «Compartir» con el mismo elemento', async () => {
  globalThis.fetch = (async () =>
    new Response(JSON.stringify({ details: DETALLES }), {
      status: 200,
    })) as unknown as typeof fetch;
  const abiertos: string[] = [];
  montar({
    onVersions: (it) => abiertos.push(`versiones:${it.id}`),
    onShare: (it) => abiertos.push(`compartir:${it.id}`),
  });
  await tick();
  const botones = [...document.querySelectorAll('button')];
  botones.find((b) => b.textContent?.includes('Historial de versiones'))?.click();
  botones.find((b) => b.textContent?.includes('Compartir'))?.click();
  expect(abiertos).toEqual(['versiones:f1', 'compartir:f1']);
});

test('el menú «⋯» del explorador ofrece «Detalles» y monta el panel', () => {
  const fuente = readFileSync(new URL('./FileBrowser.tsx', import.meta.url), 'utf8');
  expect(fuente).toMatch(/onClick=\{\(\) => onDetails\(item\)\}>\s*\{t\('details\.action'\)\}/);
  expect(fuente).toMatch(/<DetailsDialog[^]*?item=\{detailsItem\}/);
  // Las dos vistas de «Mi unidad» (lista y cuadrícula) le pasan el manejador.
  expect(fuente.match(/onDetails=\{offline \? undefined : setDetailsItem\}/g)?.length).toBe(2);
});
