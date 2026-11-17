/* SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { translate } from '@sytue/i18n';
import { I18nProvider } from '@sytue/i18n/preact';
import { abrirPaleta, ProveedorDePaleta } from '@sytue/shell';
import { ComandosDelErp, ESPERA_BUSQUEDA_PALETA, type DestinoVista, type Vista } from './ErpApp.js';
import type { Modelo, Modulo, Registro } from './tipos.js';
import { useT as traducir } from '../i18n/index.js';

if (typeof document === 'undefined') GlobalRegistrator.register();
const t = traducir('es');
// El Hub no encuentra nada: aquí se prueba solo lo que el ERP pone en la paleta.
const hub = { search: async () => ({ results: [] }) } as unknown as Parameters<
  typeof ProveedorDePaleta
>[0]['hub'];
const original = globalThis.fetch;
let host: HTMLDivElement;

/** Lo que se pidió al servicio, con la señal de cada petición. */
let pedidas: { url: string; signal: AbortSignal | null | undefined }[] = [];
/** Lo que contesta `/search`, según la consulta. */
let contestar: (q: string) => Registro[] = () => [];

beforeEach(() => {
  host = document.createElement('div');
  document.body.appendChild(host);
  pedidas = [];
  contestar = () => [];
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    pedidas.push({ url, signal: init?.signal });
    const q = new URL(url, 'http://x').searchParams.get('q') ?? '';
    return new Response(JSON.stringify({ items: contestar(q) }), { status: 200 });
  }) as typeof fetch;
});
afterEach(async () => {
  await act(async () => render(null, host));
  host.remove();
  document.body.innerHTML = '';
  globalThis.fetch = original;
});

const facturas: Modelo = {
  clave: 'factura_venta',
  modulo: 'ventas',
  etiqueta: 'Facturas',
  numerado: true,
  contable: true,
  campos: [],
  lista: [],
};
const clientes: Modelo = {
  clave: 'cliente',
  modulo: 'contactos',
  etiqueta: 'Clientes',
  numerado: false,
  contable: false,
  campos: [],
  lista: [],
};
const modulos: Modulo[] = [
  { clave: 'ventas', etiqueta: 'Ventas', modelos: [facturas] },
  { clave: 'contactos', etiqueta: 'Contactos', modelos: [clientes] },
];
const vistas: DestinoVista[] = [
  { vista: 'modelo', rotulo: t('nav.home') },
  { vista: 'tareas', rotulo: t('nav.tasks') },
  { vista: 'libro', rotulo: t('nav.ledger') },
  { vista: 'balance', rotulo: t('nav.balance') },
];

function registro(modelo: string, id: string, ref: string, etiqueta: string): Registro {
  return {
    id,
    modelo,
    ref,
    etiqueta,
    data: {},
    creado_en: '2026-11-01T00:00:00Z',
    editado_en: '2026-11-01T00:00:00Z',
  };
}

interface Elegidos {
  vistas: Vista[];
  modelos: string[];
  registros: string[];
}

async function pintar(): Promise<Elegidos> {
  const elegidos: Elegidos = { vistas: [], modelos: [], registros: [] };
  // La capa del ERP dentro de la paleta de la suite, abierta como la abre ⌘K.
  await act(async () =>
    render(
      <I18nProvider locale="es">
        <ProveedorDePaleta hub={hub}>
          <ComandosDelErp
            vistas={vistas}
            modelos={[facturas, clientes]}
            modulos={modulos}
            onElegir={(m) => elegidos.modelos.push(m.clave)}
            onVista={(v) => elegidos.vistas.push(v)}
            onRegistro={(r) => elegidos.registros.push(`${r.modelo}/${r.id}`)}
            t={t}
          />
        </ProveedorDePaleta>
      </I18nProvider>,
      host,
    ),
  );
  await act(async () => abrirPaleta());
  return elegidos;
}

function campo(): HTMLInputElement {
  const c = document.querySelector<HTMLInputElement>('input[role="combobox"]');
  if (!c) throw new Error('No está el campo de la paleta');
  return c;
}
async function escribir(texto: string): Promise<void> {
  await act(async () => {
    campo().value = texto;
    campo().dispatchEvent(new Event('input', { bubbles: true }));
  });
}
async function esperarBusqueda(): Promise<void> {
  await act(async () => {
    await new Promise((r) => setTimeout(r, ESPERA_BUSQUEDA_PALETA + 50));
  });
  // La respuesta y su `json()` resuelven en microtareas posteriores.
  await act(async () => {
    await new Promise((r) => setTimeout(r, 0));
  });
}
function opciones(): string[] {
  return [...document.querySelectorAll('[role="option"]')].map(
    (o) => o.firstElementChild?.textContent?.trim() ?? o.textContent?.trim() ?? '',
  );
}
/** Los rótulos de grupo y las opciones, en el orden en que se leen. */
function lectura(): string[] {
  return [...document.querySelectorAll('[role="listbox"] > li')].map(
    (li) => `${li.getAttribute('role') === 'option' ? '' : '# '}${li.textContent?.trim() ?? ''}`,
  );
}
function opcion(texto: string): HTMLElement {
  const o = [...document.querySelectorAll<HTMLElement>('[role="option"]')].find((x) =>
    x.textContent?.includes(texto),
  );
  if (!o) throw new Error(`No está la opción «${texto}»: ${lectura().join(' | ')}`);
  return o;
}

test('sin escribir nada ofrece las pantallas y las áreas, cada una bajo su rótulo', async () => {
  await pintar();
  const l = lectura();
  expect(l[0]).toBe(`# ${t('palette.groupViews')}`);
  expect(l.indexOf(`# ${t('palette.groupAreas')}`)).toBeGreaterThan(vistas.length);
  expect(l.some((x) => x.startsWith(t('nav.ledger')))).toBe(true);
  expect(l.some((x) => x.startsWith('Facturas'))).toBe(true);
  // Con menos de dos letras no se pregunta al servicio.
  await escribir('l');
  await esperarBusqueda();
  expect(pedidas).toEqual([]);
});

test('escribir «libro» ofrece el Libro diario y elegirlo con Intro lleva a la vista', async () => {
  const elegidos = await pintar();
  await escribir('libro');
  expect(opciones()[0]).toContain(t('nav.ledger'));
  expect(lectura()[0]).toBe(`# ${t('palette.groupViews')}`);
  await act(async () => {
    campo().dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
  });
  expect(elegidos.vistas).toEqual(['libro']);
});

test('una referencia trae su registro del servicio y elegirlo abre su ficha', async () => {
  contestar = (q) =>
    q === 'FV-2026-0012'
      ? [
          registro('factura_venta', 'f12', 'FV-2026-0012', 'Acme S.L.'),
          // De un modelo que el catálogo no trae: no hay ficha que abrir, no se ofrece.
          registro('fantasma', 'x1', 'FV-2026-0012-X', 'Oculto'),
        ]
      : [];
  const elegidos = await pintar();
  await escribir('FV-2026-0012');
  // Mientras contesta, no se afirma que no haya nada.
  expect(document.querySelector('[role="listbox"]')?.textContent).toContain(
    translate('es', 'search.busy'),
  );
  await esperarBusqueda();
  expect(pedidas).toHaveLength(1);
  expect(pedidas[0]?.url).toBe('/api/search?q=FV-2026-0012&limit=8');
  const l = lectura();
  expect(l).toContain(`# ${t('palette.groupRecords')}`);
  expect(l.some((x) => x.includes('Oculto'))).toBe(false);
  // El nombre se lee y la referencia y el área van de pista.
  const acme = opcion('Acme S.L.');
  expect(acme.textContent).toContain('FV-2026-0012 · Facturas');
  await act(async () => acme.click());
  expect(elegidos.registros).toEqual(['factura_venta/f12']);
});

test('escribir deprisa hace una sola petición y abandona la anterior', async () => {
  contestar = (q) => (q.includes('acme') ? [registro('cliente', 'c1', '', 'Acme S.L.')] : []);
  await pintar();
  await escribir('ac');
  await escribir('acm');
  await escribir('acme');
  await esperarBusqueda();
  // El antirrebote: solo la última consulta llega al servicio.
  expect(pedidas.map((p) => new URL(p.url, 'http://x').searchParams.get('q'))).toEqual(['acme']);
  expect(opciones().some((o) => o.includes('Acme S.L.'))).toBe(true);

  // Con una petición en vuelo, escribir otra cosa la cancela.
  await escribir('acme s');
  await act(async () => {
    await new Promise((r) => setTimeout(r, ESPERA_BUSQUEDA_PALETA + 20));
  });
  const enVuelo = pedidas.at(-1)?.signal;
  await escribir('globex');
  expect(enVuelo?.aborted).toBe(true);
});

test('al cambiar la búsqueda no se enseñan los registros de la anterior mientras llega la nueva', async () => {
  contestar = (q) => (q === 'acme' ? [registro('cliente', 'c1', '', 'Acme S.L.')] : []);
  await pintar();
  await escribir('acme');
  await esperarBusqueda();
  expect(opciones().some((o) => o.includes('Acme S.L.'))).toBe(true);
  // Otra búsqueda: hasta que contesta, ni Acme (que no casa con «globex») ni «sin nada».
  await escribir('globex');
  expect(opciones().some((o) => o.includes('Acme S.L.'))).toBe(false);
  expect(document.querySelector('[role="listbox"]')?.textContent).toContain(
    translate('es', 'search.busy'),
  );
  await esperarBusqueda();
  expect(opciones().some((o) => o.includes('Acme S.L.'))).toBe(false);
});

test('sin coincidencias en ningún grupo dice el texto de la paleta de la suite', async () => {
  await pintar();
  await escribir('zzzz');
  await esperarBusqueda();
  expect(opciones()).toEqual([]);
  expect(document.querySelector('[role="listbox"]')?.textContent?.trim()).toBe(
    translate('es', 'search.emptyTitle', { query: 'zzzz' }),
  );
});

test('las flechas recorren los grupos de corrido y Escape cierra', async () => {
  await pintar();
  const pulsar = async (key: string): Promise<void> =>
    act(async () => {
      campo().dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true }));
    });
  // Ir hacia atrás desde la primera da la vuelta a la última: la última área.
  await pulsar('ArrowUp');
  const activa = document.querySelector('[role="option"][aria-selected="true"]');
  expect(activa?.textContent).toContain('Clientes');
  await pulsar('Escape');
  expect(document.querySelector('input[role="combobox"]')).toBeNull();
});
