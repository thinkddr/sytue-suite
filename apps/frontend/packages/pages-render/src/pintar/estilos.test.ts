/* SPDX-License-Identifier: Apache-2.0 */

import { describe, expect, test } from 'bun:test';
import type { DatosRender } from '../contexto';
import type { Layout, Marco, Nodo, Seccion, Sitio } from '../ir/tipos';
import { cssNodos, cssTema, color } from './estilos';
import { cssComun, pintarPagina } from './index';

const lay = (parcial: Partial<Layout> = {}): Layout => ({
  direction: 'column',
  gap: 16,
  padding: [0, 0, 0, 0],
  align: 'start',
  justify: 'start',
  wrap: false,
  ...parcial,
});
const texto = (id: string, t: string): Nodo => ({
  id,
  type: 'text',
  tag: 'p',
  content: { es: [{ text: t }] },
});
const marco = (id: string, hijos: Nodo[], extra: Partial<Marco> = {}): Marco => ({
  id,
  type: 'frame',
  tag: 'div',
  layout: lay(),
  children: hijos,
  ...extra,
});
const seccion = (id: string, hijos: Nodo[], extra: Partial<Seccion> = {}): Seccion => ({
  id,
  type: 'section',
  kind: 'custom',
  full_bleed: false,
  layout: lay({ padding: [48, 32, 48, 32] }),
  children: hijos,
  ...extra,
});
const sitio = (secciones: Seccion[]): Sitio => ({
  version: 1,
  languages: { default: 'es', others: [] },
  header: null,
  footer: null,
  components: [],
  seo: { site_name: { es: 'X' } },
  theme: {
    colors: {
      background: '#ffffff',
      text: '#111111',
      primary: '#9a3412',
      on_primary: '#ffffff',
      muted: '#555555',
      border: '#dddddd',
      surface: '#f4f4f4',
    },
    fonts: { heading: 'inter', body: 'inter' },
    text_styles: {
      h1: { font: 'heading', size: [48, 40, 32], line_height: 1.1, weight: 700, letter_spacing: 0 },
    } as Sitio['theme']['text_styles'],
    radius: { sm: 4, md: 8, lg: 12 },
    container: 1120,
  },
  pages: [
    {
      id: 'page000001',
      slug: { es: '' },
      title: { es: 'Inicio' },
      description: { es: 'd' },
      show_header: false,
      show_footer: false,
      sections: secciones,
    },
  ],
});

/** Cuerpo de la primera regla `.n<id>{…}` (o, con `punto`, la de ese @media). */
const regla = (css: string, id: string, punto?: 'tablet' | 'mobile'): string => {
  const media =
    punto === 'tablet'
      ? '@media (max-width:1023.98px){'
      : punto === 'mobile'
        ? '@media (max-width:639.98px){'
        : '';
  const desde = media === '' ? 0 : css.indexOf(media);
  if (desde < 0) return '';
  const i = css.indexOf(`.n${id}{`, desde);
  if (i < 0) return '';
  return css.slice(i + id.length + 3, css.indexOf('}', i));
};

describe('estilos (§4.2)', () => {
  test('hijos fill, hug y fijos en el eje principal', () => {
    const fila = marco(
      'fila000001',
      [
        { ...marco('hijo000001', []), size: { width: 'fill', height: 'hug' } },
        { ...marco('hijo000002', []), size: { width: 'hug', height: 'hug' } },
        { ...marco('hijo000003', []), size: { width: 240, height: 'hug' } },
      ],
      { layout: lay({ direction: 'row' }) },
    );
    const css = cssNodos(sitio([seccion('sec0000001', [fila])]));
    expect(regla(css, 'hijo000001')).toStartWith('flex:1 1 0%;min-width:0');
    expect(regla(css, 'hijo000002')).toStartWith('flex:0 0 auto');
    expect(regla(css, 'hijo000003')).toStartWith('flex:0 0 240px;width:240px;max-width:100%');
    expect(regla(css, 'fila000001')).toContain('display:flex;flex-direction:row;gap:16px;');
  });

  test('columnas de una fila con wrap y su cambio en tableta y móvil', () => {
    const rejilla = marco('rej0000001', [marco('car0000001', []), marco('car0000002', [])], {
      layout: lay({ direction: 'row', wrap: true, columns: 3, gap: 20 }),
      breakpoints: { tablet: { columns: 2 }, mobile: { columns: 1 } },
    });
    const css = cssNodos(sitio([seccion('sec0000001', [rejilla])]));
    expect(regla(css, 'car0000001')).toStartWith('flex:0 0 calc((100% - 40px)/3);min-width:0');
    expect(regla(css, 'car0000001', 'tablet')).toStartWith('flex:0 0 calc((100% - 20px)/2)');
    expect(regla(css, 'car0000001', 'mobile')).toStartWith('flex:0 0 calc((100% - 0px)/1)');
  });

  test('la sección centra su contenido a la medida del tema y lleva su fondo fuera', () => {
    const css = cssNodos(sitio([seccion('sec0000001', [], { style: { background: '$surface' } })]));
    expect(css).toContain('.nsec0000001{background:var(--c-surface);padding:48px 32px 48px 32px}');
    expect(css).toContain('.nsec0000001>.sp-c{display:flex;flex-direction:column;gap:16px;');
    expect(css).toContain('max-width:var(--sp-container)');
  });

  test('una sección full_bleed lleva el flex en sí misma', () => {
    const css = cssNodos(sitio([seccion('sec0000001', [], { full_bleed: true })]));
    expect(regla(css, 'sec0000001')).toContain('display:flex;');
    expect(regla(css, 'sec0000001')).toContain('padding:48px 32px 48px 32px');
    expect(css).not.toContain('>.sp-c');
  });

  test('un delta cambia la dirección en el móvil y hereda la tableta', () => {
    const f = marco('fila000001', [texto('txt0000001', 'a')], {
      layout: lay({ direction: 'row' }),
      breakpoints: { tablet: { layout: { gap: 8 } }, mobile: { layout: { direction: 'column' } } },
    });
    const css = cssNodos(sitio([seccion('sec0000001', [f])]));
    expect(regla(css, 'fila000001', 'tablet')).toContain('flex-direction:row;gap:8px');
    expect(regla(css, 'fila000001', 'mobile')).toContain('flex-direction:column;gap:8px');
  });

  test('ocultar en un punto de corte', () => {
    const t = { ...texto('txt0000001', 'a'), breakpoints: { mobile: { hidden: true } } } as Nodo;
    const css = cssNodos(sitio([seccion('sec0000001', [t])]));
    expect(regla(css, 'txt0000001', 'mobile')).toContain('display:none');
    expect(regla(css, 'txt0000001')).not.toContain('display:none');
  });

  test('los tokens se traducen y lo que no es color ni token no llega al CSS', () => {
    expect(color('$primary')).toBe('var(--c-primary)');
    expect(color('#ABCDEF')).toBeUndefined();
    expect(color('red;}body{background:url(x)')).toBeUndefined();
    const f = marco('fila000001', [], {
      style: {
        background: 'red;}body{x:y',
        color: '$text',
        radius: '$md',
        border: { width: 1, color: '$border', style: 'solid' },
      },
    });
    const css = cssNodos(sitio([seccion('sec0000001', [f])]));
    expect(css).not.toContain('body{');
    expect(css).toContain('color:var(--c-text)');
    expect(css).toContain('border-radius:var(--r-md)');
    expect(css).toContain('border:1px solid var(--c-border)');
  });

  test('el tema emite tokens ordenados, estilos de texto y @font-face de los datos', () => {
    const datos = {
      fonts: {
        inter: {
          name: 'Inter',
          fallback: 'sans-serif',
          scripts: ['latin'],
          faces: [
            {
              file: 'inter-latin.woff2',
              style: 'normal',
              weight: '100 900',
              unicode_range: 'U+0000-00FF',
            },
          ],
        },
      },
    } as unknown as DatosRender;
    const css = cssTema(sitio([]), datos);
    expect(css).toContain('@font-face{font-family:"Inter";src:url(/_astro/inter-latin.woff2)');
    expect(css.indexOf('--c-background')).toBeLessThan(css.indexOf('--c-text'));
    expect(css).toContain('--f-heading:"Inter",sans-serif');
    expect(css).toContain(
      '.t-h1{font-family:var(--f-heading);line-height:1.1;font-weight:700;letter-spacing:0em;font-size:48px}',
    );
    expect(css).toContain('@media (max-width:639.98px){.t-h1{font-size:32px}}');
  });

  test('es determinista y solo usa @media y @font-face', () => {
    const s = sitio([seccion('sec0000001', [marco('fila000001', [texto('txt0000001', 'a')])])]);
    expect(cssComun(s)).toBe(cssComun(s));
    const reglasAt = [...cssComun(s).matchAll(/@([a-z-]+)/g)].map((m) => m[1]);
    for (const r of reglasAt) expect(['media', 'font-face']).toContain(r);
    expect(cssComun(s)).not.toContain('url(');
  });

  test('en el lienzo un relleno de imagen se pinta en línea sin pedir un recurso externo', () => {
    const foto: Nodo = {
      id: 'img0000001',
      type: 'image',
      src: { placeholder: 'portrait' },
      alt: { es: 'Retrato' },
      fit: 'cover',
    };
    const s = sitio([seccion('sec0000001', [foto])]);
    const datos = {
      fonts: {},
      images: {},
      origins: { cast: '', pages: '' },
    } as unknown as DatosRender;
    const { html } = pintarPagina(s, datos, 'page000001', 'es', {
      modo: 'lienzo',
      puntoDeCorte: 'desktop',
      urlRecurso: (n) => `/api/${n}`,
    });
    expect(html).toContain('src="data:image/svg+xml;utf8,');
    expect(html).toContain('width="960" height="1200"');
    expect(html).not.toContain('/api/placeholder-portrait.svg');
  });

  test('la salida del lienzo trae el CSS y el HTML con las clases de los nodos', () => {
    const s = sitio([seccion('sec0000001', [texto('txt0000001', 'Hola')])]);
    const datos = {
      fonts: {},
      images: {},
      origins: { cast: '', pages: '' },
    } as unknown as DatosRender;
    const { html, css } = pintarPagina(s, datos, 'page000001', 'es', {
      modo: 'lienzo',
      puntoDeCorte: 'desktop',
      urlRecurso: (n) => n,
    });
    expect(html).toContain('class="sp-section nsec0000001"');
    expect(html).toContain('<div class="sp-c">');
    expect(html).toContain('class="sp-text ntxt0000001 t-body"');
    expect(css).toContain('.ntxt0000001');
  });
});
