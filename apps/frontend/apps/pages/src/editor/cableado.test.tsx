/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, describe, expect, test } from 'bun:test';
import { render, type VNode } from 'preact';
import type { DatosRender, Sitio } from '@sytue/pages-render';
import { EditorApp } from './EditorApp';
import { BarraSuperior, type BarraSuperiorProps } from './componentes/BarraSuperior';

if (typeof document === 'undefined') GlobalRegistrator.register();

const browser = window as unknown as { happyDOM: { setURL: (url: string) => void } };
let fetchOriginal: typeof fetch;
let urlOriginal: string;
let host: HTMLElement;

beforeEach(() => {
  fetchOriginal = globalThis.fetch;
  urlOriginal = window.location.href;
  browser.happyDOM.setURL('https://web.sytue.eu/web/acme/edit/sitio-1');
  host = document.createElement('div');
  document.body.appendChild(host);
});

afterEach(() => {
  render(null, host);
  host.remove();
  globalThis.fetch = fetchOriginal;
  browser.happyDOM.setURL(urlOriginal);
});

const montar = (nodo: VNode) => render(nodo, host);
const esperar = async (condicion: () => boolean) => {
  const limite = Date.now() + 3000;
  while (!condicion()) {
    if (Date.now() > limite) throw new Error('La condición no llegó a cumplirse');
    await new Promise((r) => setTimeout(r, 10));
  }
};

const sitioEditor: Sitio = {
  version: 1,
  languages: { default: 'es', others: [] },
  theme: {
    colors: {
      background: '#ffffff',
      text: '#111111',
      primary: '#165c9c',
      on_primary: '#ffffff',
      muted: '#596473',
      border: '#dddddd',
    },
    fonts: { heading: 'figtree', body: 'figtree' },
    text_styles: {} as Sitio['theme']['text_styles'],
    radius: { sm: 4, md: 8, lg: 12 },
    container: 960,
  },
  header: null,
  footer: null,
  components: [],
  seo: { site_name: { es: 'Aurora' } },
  pages: [
    {
      id: 'pagina0001',
      slug: { es: '' },
      title: { es: 'Inicio' },
      description: { es: 'Inicio' },
      show_header: false,
      show_footer: false,
      sections: [
        {
          id: 'seccion001',
          type: 'section',
          kind: 'hero',
          full_bleed: false,
          layout: {
            direction: 'column',
            gap: 0,
            padding: [0, 0, 0, 0],
            align: 'start',
            justify: 'start',
            wrap: false,
          },
          children: [
            {
              id: 'imagen0001',
              type: 'image',
              src: { asset: 'imagen-1' },
              alt: { es: 'Producto' },
              fit: 'cover',
            },
          ],
        },
      ],
    },
  ],
};

const renderEditor: DatosRender = {
  site: {
    id: 'sitio-1',
    slug: 'aurora',
    sites_host: 'sites.example.test',
    canonical_host: 'aurora.sites.example.test',
    hosts: [],
    analytics: false,
  },
  images: {
    'a:imagen-1': { file: 'producto.png', ext: 'png', width: 200, height: 200, variants: [] },
  },
  files: {},
  fonts: {},
  forms: {},
  store: null,
  legal: {
    holder_name: '',
    tax_id: '',
    country: 'ES',
    address: { street: '', postal_code: '', city: '', region: '' },
    email: '',
    phone: '',
    registry: '',
    privacy_contact: '',
  },
  origins: { cast: 'https://cast.example.test', pages: 'https://pages.example.test' },
  now: '2026-10-04T00:00:00Z',
};

describe('cableado del editor', () => {
  test('consulta borrador y render con base y organización, sin sustituir un fallo por localhost', async () => {
    const urls: string[] = [];
    globalThis.fetch = (async (url: string) => {
      urls.push(url);
      return url.endsWith('/draft')
        ? Response.json({ id: 'sitio-1', slug: 'aurora', name: 'Aurora', ir: {}, rev: 1 })
        : Response.json({ error: { code: 'unavailable' } }, { status: 503 });
    }) as typeof fetch;
    montar(<EditorApp siteId="sitio-1" locale="es" administra />);
    await esperar(() => !!host.textContent?.includes('Error al abrir el editor'));
    expect(urls).toEqual([
      '/web/acme/api/sites/sitio-1/draft',
      '/web/acme/api/sites/sitio-1/render-data',
    ]);
    expect(host.textContent).toContain('No se pudo cargar la configuración del sitio (503)');
    expect(host.querySelector('iframe')).toBeNull();
  });

  test('volver lleva a la ficha real y la preview espera al guardado', () => {
    let previews = 0;
    const nada = () => {};
    const props: BarraSuperiorProps = {
      locale: 'es',
      siteId: 'sitio-1',
      siteName: 'Aurora',
      ir: {
        pages: [{ id: 'inicio', title: { es: 'Inicio' } }],
        languages: { default: 'es', others: [] },
      } as unknown as Sitio,
      paginaActivaId: 'inicio',
      puntoDeCorte: 'desktop',
      idiomaActivo: 'es',
      zoom: 1,
      estadoGuardado: 'pendiente',
      puedeDeshacer: false,
      puedeRehacer: false,
      onCambiarPagina: nada,
      onCambiarPuntoDeCorte: nada,
      onCambiarIdioma: nada,
      onAbrirAsistente: nada,
      onDeshacer: nada,
      onRehacer: nada,
      onZoomMas: nada,
      onZoomMenos: nada,
      onZoomAjustar: nada,
      onZoom100: nada,
      previsualizando: false,
      onVistaPrevia: () => {
        previews++;
      },
    };
    montar(<BarraSuperior {...props} />);
    expect(host.querySelector('a[title="Volver"]')?.getAttribute('href')).toBe(
      '/web/acme/site/sitio-1',
    );
    const boton = () =>
      [...host.querySelectorAll('button')].find((b) => b.textContent?.includes('Vista previa'));
    expect(boton()?.disabled).toBe(true);
    expect(host.querySelector('a[href*="/_vista/"]')).toBeNull();
    montar(<BarraSuperior {...props} estadoGuardado="guardado" />);
    expect(boton()?.disabled).toBe(false);
    boton()?.click();
    expect(previews).toBe(1);
  });

  test('guarda las ediciones hechas durante un PUT con su nueva revisión y descarta previews anteriores', async () => {
    const guardados: {
      cuerpo: { rev: number; ir: Sitio };
      terminar: (respuesta: Response) => void;
    }[] = [];
    let terminarPreview: ((respuesta: Response) => void) | undefined;
    globalThis.fetch = (async (url: string, init?: RequestInit) => {
      if (url.endsWith('/draft') && init?.method === 'PUT') {
        return new Promise<Response>((terminar) => {
          guardados.push({
            cuerpo: JSON.parse(String(init.body)) as { rev: number; ir: Sitio },
            terminar,
          });
        });
      }
      if (url.includes('/api/publicar/'))
        return new Promise<Response>((terminar) => {
          terminarPreview = terminar;
        });
      return url.endsWith('/draft')
        ? Response.json({ id: 'sitio-1', slug: 'aurora', name: 'Aurora', ir: sitioEditor, rev: 1 })
        : Response.json(renderEditor);
    }) as unknown as typeof fetch;
    montar(<EditorApp siteId="sitio-1" locale="es" administra />);
    await esperar(() => !!host.querySelector('iframe'));
    expect(host.querySelector('iframe')?.getAttribute('srcdoc')).toContain(
      '/web/acme/api/sites/sitio-1/assets/producto.png',
    );
    const boton = (texto: string) =>
      [...host.querySelectorAll('button')].find((b) => b.textContent?.trim() === texto)!;
    const preview = () => boton('Vista previa');
    boton('Sitio').click();
    await esperar(() => !!host.querySelector('input[type="text"]'));
    const editarFondo = (color: string) => {
      const fila = [...host.querySelectorAll('span')].find(
        (s) => s.textContent === 'Fondo general',
      )?.parentElement;
      const input = fila?.querySelector('input[type="text"]') as HTMLInputElement;
      input.value = color;
      input.dispatchEvent(new Event('input', { bubbles: true }));
    };

    editarFondo('#eeeeee');
    await esperar(() => guardados.length === 1);
    editarFondo('#dddddd');
    await esperar(() => !!host.textContent?.includes('Sin guardar'));
    await new Promise((r) => setTimeout(r, 1600));
    expect(guardados).toHaveLength(1);
    expect(preview().disabled).toBe(true);

    guardados[0]!.terminar(Response.json({ rev: 2, warnings: [] }));
    await esperar(() => guardados.length === 2);
    expect(guardados[1]!.cuerpo.rev).toBe(2);
    expect(guardados[1]!.cuerpo.ir.theme.colors.background).toBe('#dddddd');
    expect(preview().disabled).toBe(true);
    guardados[1]!.terminar(Response.json({ rev: 3, warnings: [] }));
    await esperar(() => !preview().disabled);

    preview().click();
    await esperar(() => !!terminarPreview);
    editarFondo('#cccccc');
    terminarPreview!(Response.json({ url: 'https://preview.example.test/vista-antigua' }));
    await esperar(() => preview()?.textContent?.trim() === 'Vista previa');
    expect(host.querySelector('a[href="https://preview.example.test/vista-antigua"]')).toBeNull();
    expect(preview().disabled).toBe(true);
  }, 12000);

  test('la IA espera al último borrador confirmado y aplicar conserva las ediciones durante el trabajo', async () => {
    const guardados: {
      cuerpo: { rev: number; ir: Sitio };
      terminar: (respuesta: Response) => void;
    }[] = [];
    const borradoresIA: Sitio[] = [];
    let persistido = sitioEditor;
    let terminarTrabajo: ((respuesta: Response) => void) | undefined;
    globalThis.fetch = (async (url: string, init?: RequestInit) => {
      if (url.endsWith('/draft') && init?.method === 'PUT') {
        return new Promise<Response>((terminar) => {
          guardados.push({
            cuerpo: JSON.parse(String(init.body)) as { rev: number; ir: Sitio },
            terminar,
          });
        });
      }
      if (url.endsWith('/ia/translate')) {
        borradoresIA.push(persistido);
        return Response.json({ job: { id: 'traduccion-1' } }, { status: 202 });
      }
      if (url.includes('/api/ia/jobs/')) {
        return new Promise<Response>((terminar) => {
          terminarTrabajo = terminar;
        });
      }
      if (url.endsWith('/api/ia/quota')) {
        return Response.json({ limit: 50, used: 0, reset_at: '2026-11-01T00:00:00Z' });
      }
      if (url.endsWith('/api/ia/key')) return Response.json({ has_key: false });
      return url.endsWith('/draft')
        ? Response.json({ id: 'sitio-1', slug: 'aurora', name: 'Aurora', ir: sitioEditor, rev: 1 })
        : Response.json(renderEditor);
    }) as unknown as typeof fetch;
    montar(<EditorApp siteId="sitio-1" locale="es" administra />);
    await esperar(() => !!host.querySelector('iframe'));
    const boton = (texto: string) =>
      [...host.querySelectorAll('button')].find((b) => b.textContent?.trim() === texto)!;
    boton('Páginas').click();
    const campoTitulo = () =>
      [...host.querySelectorAll('span')]
        .find((s) => s.textContent === 'Título (es)')
        ?.parentElement?.querySelector('input') as HTMLInputElement;
    await esperar(() => !!campoTitulo());
    const editarTitulo = (texto: string) => {
      const input = campoTitulo();
      input.value = texto;
      input.dispatchEvent(new Event('input', { bubbles: true }));
    };
    const confirmarGuardado = (indice: number, rev: number) => {
      persistido = guardados[indice]!.cuerpo.ir;
      guardados[indice]!.terminar(Response.json({ rev, warnings: [] }));
    };

    editarTitulo('Primer cambio sin guardar');
    boton('Traducir').click();
    // El asistente es el `Modal` de la suite, que se pinta en `document.body` (portal) y
    // no dentro del editor.
    await esperar(() => !!document.querySelector('[role="dialog"] form'));
    document
      .querySelector('[role="dialog"] form')!
      .dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
    await esperar(() => guardados.length === 1);
    expect(guardados[0]!.cuerpo.ir.pages[0]!.title.es).toBe('Primer cambio sin guardar');
    expect(borradoresIA).toHaveLength(0);

    editarTitulo('Última edición antes de la IA');
    confirmarGuardado(0, 2);
    await esperar(() => guardados.length === 2);
    expect(guardados[1]!.cuerpo.rev).toBe(2);
    expect(guardados[1]!.cuerpo.ir.pages[0]!.title.es).toBe('Última edición antes de la IA');
    expect(borradoresIA).toHaveLength(0);
    confirmarGuardado(1, 3);
    await esperar(() => !!terminarTrabajo);
    expect(borradoresIA).toHaveLength(1);
    expect(borradoresIA[0]!.pages[0]!.title.es).toBe('Última edición antes de la IA');

    editarTitulo('Edición mientras trabaja la IA');
    terminarTrabajo!(
      Response.json({
        id: 'traduccion-1',
        kind: 'translate',
        status: 'done',
        result: { 'pagina0001/title': { en: 'Updated draft' } },
      }),
    );
    const aplicar = () =>
      [...document.querySelectorAll('[role="dialog"] button')].find(
        (b) => b.textContent?.trim() === 'Aplicar traducciones',
      ) as HTMLButtonElement | undefined;
    await esperar(() => !!aplicar());
    aplicar()!.click();
    await esperar(() => guardados.length === 3);
    expect(guardados[2]!.cuerpo.rev).toBe(3);
    expect(guardados[2]!.cuerpo.ir.pages[0]!.title).toEqual({
      es: 'Edición mientras trabaja la IA',
      en: 'Updated draft',
    });
    expect(guardados[2]!.cuerpo.ir.languages.others).toEqual(['en']);
    confirmarGuardado(2, 4);
  }, 12000);
});
