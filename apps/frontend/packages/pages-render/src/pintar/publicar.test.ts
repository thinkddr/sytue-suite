/* SPDX-License-Identifier: Apache-2.0 */

import { describe, expect, test } from 'bun:test';
import type { DatosRender } from '../contexto';
import type { Sitio } from '../ir/tipos';
import { publicar } from '../exportar/index';

const datos: DatosRender = {
  site: {
    id: 'site',
    slug: 'demo',
    sites_host: 'sites.example.test',
    canonical_host: 'demo.example.test',
    hosts: ['demo.example.test'],
    analytics: false,
  },
  images: {},
  files: {},
  fonts: {},
  forms: {},
  origins: { cast: 'https://cast.example.test', pages: 'https://pages.example.test' },
  now: '2026-09-29T00:00:00Z',
  legal: {
    holder_name: 'Sytue',
    tax_id: 'X',
    country: 'ES',
    address: { street: 'Calle Uno', postal_code: '28001', city: 'Madrid', region: '' },
    email: 'legal@example.test',
    phone: '',
    registry: '',
    privacy_contact: '',
  },
  store: null,
};

const sitio: Sitio = {
  version: 1,
  languages: { default: 'es', others: ['en'] },
  header: null,
  footer: null,
  components: [],
  seo: { site_name: { es: 'Sytue', en: 'Sytue' } },
  theme: {
    colors: {
      background: '#ffffff',
      text: '#111111',
      primary: '#000000',
      on_primary: '#ffffff',
      muted: '#eeeeee',
      border: '#dddddd',
    },
    fonts: { heading: 'inter', body: 'inter' },
    text_styles: {} as Sitio['theme']['text_styles'],
    radius: { sm: 4, md: 8, lg: 12 },
    container: 960,
  },
  pages: [
    {
      id: 'page000001',
      slug: { es: '', en: '' },
      title: { es: 'Inicio', en: 'Home' },
      description: { es: 'Descripción', en: 'Description' },
      show_header: false,
      show_footer: false,
      sections: [
        {
          id: 'section001',
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
              id: 'heading001',
              type: 'text',
              tag: 'h1',
              content: { es: [{ text: 'Hola' }], en: [{ text: 'Hello' }] },
            },
          ],
        },
      ],
    },
  ],
};

describe('publicar', () => {
  test('es determinista y emite cabeceras, legales y sitemap', () => {
    const hash = (bytes: Uint8Array) => new Bun.CryptoHasher('sha256').update(bytes).digest('hex');
    const a = publicar(sitio, datos, { base: '', vista: false, hash });
    const b = publicar(sitio, datos, { base: '', vista: false, hash });
    expect(a).toEqual(b);
    expect(a.archivos.map((f) => f.ruta)).toContain('es/legal/privacy/index.html');
    expect(a.archivos.map((f) => f.ruta)).toContain('sitemap.xml');
    expect(String(a.archivos.find((f) => f.ruta === 'es/index.html')?.contenido)).toContain(
      '<html lang="es">',
    );
  });

  test('toda página HTML preview empieza con la franja, incluidas legales y 404', () => {
    const hash = (bytes: Uint8Array) => new Bun.CryptoHasher('sha256').update(bytes).digest('hex');
    for (const base of ['', '/_vista/0123456789abcdef0123456789abcdef'] as const) {
      const vista = base !== '';
      const salida = publicar(sitio, datos, { base, vista, hash });
      const htmls = salida.archivos.filter((f) => f.ruta.endsWith('.html'));
      expect(htmls.some((f) => f.ruta.endsWith('404.html'))).toBe(true);
      expect(htmls.some((f) => f.ruta.includes('/legal/'))).toBe(true);
      for (const archivo of htmls) {
        const html = String(archivo.contenido);
        if (vista) {
          expect(html).toMatch(/<body><div class="sp-vista" role="status">/);
          expect(html.match(/class="sp-vista"/g)).toHaveLength(1);
          expect(html).toContain(
            archivo.ruta.startsWith('es/') ? '>Vista previa</div>' : '>Preview</div>',
          );
        } else expect(html).not.toContain('class="sp-vista"');
      }
    }
  });
});

// La preview de un sitio nuevo no depende de recursos públicos de una versión live.
test('preview conserva CSS, fuentes, imágenes y PDF dentro del token', () => {
  const ir = structuredClone(sitio);
  ir.pages[0]!.sections[0]!.children.push(
    {
      id: 'image00001',
      type: 'image',
      src: { asset: 'foto' },
      alt: { es: 'Foto' },
      fit: 'contain',
    },
    {
      id: 'button0001',
      type: 'button',
      label: { es: 'Documento' },
      variant: 'primary',
      action: { file: 'documento' },
    },
  );
  const conRecursos: DatosRender = {
    ...datos,
    images: { 'a:foto': { file: 'foto.png', ext: 'png', width: 100, height: 100, variants: [] } },
    files: {
      'a:documento': { file: '0123456789abcdef0123456789abcdef', name: 'Guía', bytes: 100 },
    },
    fonts: {
      inter: {
        name: 'Inter',
        fallback: 'sans-serif',
        scripts: ['latin'],
        faces: [
          { file: 'inter.woff2', style: 'normal', weight: '400', unicode_range: 'U+0000-00FF' },
        ],
      },
    },
  };
  const hash = (bytes: Uint8Array) => new Bun.CryptoHasher('sha256').update(bytes).digest('hex');
  for (const base of ['', '/_vista/0123456789abcdef0123456789abcdef'] as const) {
    const salida = publicar(ir, conRecursos, { base, vista: base !== '', hash });
    const css = salida.archivos.find((f) => f.ruta.endsWith('.css'))!;
    const html = String(salida.archivos.find((f) => f.ruta === 'es/index.html')!.contenido);
    expect(html).toContain(`href="${base}/${css.ruta}"`);
    expect(html).toContain(`src="${base}/_astro/foto.png"`);
    expect(html).toContain(`href="${base}/_astro/0123456789abcdef0123456789abcdef.pdf"`);
    expect(new TextDecoder().decode(css.contenido as Uint8Array)).toContain(
      `url(${base}/_astro/inter.woff2)`,
    );
    expect(html).toContain(`href="${base}/es/legal/privacy/"`);
    if (base !== '') expect(html).not.toContain('href="/_astro/');
    expect(salida.recursos).toEqual([
      '0123456789abcdef0123456789abcdef.pdf',
      'foto.png',
      'inter.woff2',
    ]);
  }
});

test('publica rellenos SVG accesibles sin pedir ficheros inexistentes ni usar data URLs', () => {
  const ir = structuredClone(sitio);
  ir.pages[0]!.sections[0]!.children.push(
    {
      id: 'portrait01',
      type: 'image',
      src: { placeholder: 'portrait' },
      alt: { es: 'Retrato' },
      fit: 'cover',
    },
    {
      id: 'missing001',
      type: 'image',
      src: { asset: 'ausente' },
      alt: { es: 'Sin foto' },
      decorative: true,
      fit: 'contain',
    },
  );
  const hash = (bytes: Uint8Array) => new Bun.CryptoHasher('sha256').update(bytes).digest('hex');
  for (const base of ['', '/_vista/0123456789abcdef0123456789abcdef'] as const) {
    const salida = publicar(ir, datos, { base, vista: base !== '', hash });
    const html = String(salida.archivos.find((f) => f.ruta === 'es/index.html')!.contenido);
    expect(html).toContain('viewBox="0 0 960 1200"');
    expect(html).toContain('role="img" aria-label="Retrato"');
    expect(html).toContain('aria-hidden="true"');
    expect(html).not.toContain('placeholder-');
    expect(html).not.toContain('data:image/');
    expect(salida.recursos).toEqual([]);
  }
});

test('los enlaces a un PDF ausente no inventan una URL pública', () => {
  const ir = structuredClone(sitio);
  ir.pages[0]!.sections[0]!.children.push({
    id: 'button0001',
    type: 'button',
    label: { es: 'Documento' },
    variant: 'primary',
    action: { file: 'ausente' },
  });
  const hash = (bytes: Uint8Array) => new Bun.CryptoHasher('sha256').update(bytes).digest('hex');
  const salida = publicar(ir, datos, { base: '', vista: false, hash });
  const html = String(salida.archivos.find((f) => f.ruta === 'es/index.html')!.contenido);
  expect(html).not.toContain('/_astro/ausente.pdf');
  expect(salida.recursos).toEqual([]);
});
