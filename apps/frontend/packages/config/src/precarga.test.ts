/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * `modulepreload` delante de cada isla: el manifiesto que sale del bundle y la
 * inyección en un HTML que llega por partes.
 */
import { expect, test } from 'bun:test';
import {
  InyectorDePrecarga,
  inyectarPrecarga,
  manifiestoDePrecarga,
  type TrozoDeCliente,
} from './precarga.js';

const trozo = (
  fileName: string,
  imports: string[] = [],
  extra: Partial<TrozoDeCliente> = {},
): TrozoDeCliente => ({
  fileName,
  isEntry: false,
  imports,
  dynamicImports: [],
  facadeModuleId: null,
  ...extra,
});

const bundle: TrozoDeCliente[] = [
  trozo('_astro/MailApp.a.js', ['_astro/preact.b.js', '_astro/ui.c.js'], {
    isEntry: true,
    dynamicImports: ['_astro/es.d.js', '_astro/es.e.js', '_astro/de.f.js', '_astro/Lazy.g.js'],
  }),
  trozo('_astro/ui.c.js', ['_astro/preact.b.js', '_astro/i18n.h.js']),
  trozo('_astro/preact.b.js'),
  trozo('_astro/i18n.h.js'),
  // El compartido y el de la app, los dos `messages/es.ts`; el de la app importa runtime.
  trozo('_astro/es.d.js', [], { facadeModuleId: '/r/packages/i18n/src/messages/es.ts' }),
  trozo('_astro/es.e.js', ['_astro/runtime.i.js'], {
    facadeModuleId: '/r/apps/mail/src/i18n/messages/es.ts',
  }),
  trozo('_astro/de.f.js', [], { facadeModuleId: '/r/packages/i18n/src/messages/de.ts' }),
  trozo('_astro/runtime.i.js'),
  // Un import() que no es de idioma no se precarga: es perezoso a propósito.
  trozo('_astro/Lazy.g.js', ['_astro/parse5.j.js'], { facadeModuleId: '/r/x/Lazy.tsx' }),
  trozo('_astro/parse5.j.js'),
  trozo('_astro/client.k.js', ['_astro/preact.b.js'], { isEntry: true }),
];

test('each entry gets its static closure and, per locale, the catalog chunks it awaits', () => {
  const m = manifiestoDePrecarga(bundle);
  expect(Object.keys(m).sort()).toEqual(['_astro/MailApp.a.js', '_astro/client.k.js']);
  expect(m['_astro/MailApp.a.js']!.js.sort()).toEqual([
    '_astro/MailApp.a.js',
    '_astro/i18n.h.js',
    '_astro/preact.b.js',
    '_astro/ui.c.js',
  ]);
  expect(m['_astro/MailApp.a.js']!.idiomas!['es']!.sort()).toEqual([
    '_astro/es.d.js',
    '_astro/es.e.js',
    '_astro/runtime.i.js',
  ]);
  expect(m['_astro/MailApp.a.js']!.idiomas!['de']).toEqual(['_astro/de.f.js']);
  expect(JSON.stringify(m)).not.toContain('parse5');
  expect(m['_astro/client.k.js']).toEqual({ js: ['_astro/client.k.js', '_astro/preact.b.js'] });
});

const isla = (client: string) =>
  `<astro-island uid="1" component-url="/mail/_astro/MailApp.a.js" component-export="MailApp" renderer-url="/mail/_astro/client.k.js" props="{&quot;a&quot;:[0,1]}" ssr client="${client}" opts="{}" await-children><main></main></astro-island>`;
const pagina = (client = 'load') =>
  `<!doctype html><html lang="es" dir="ltr"><head><title>x</title></head><body>${isla(client)}</body></html>`;
const enlaces = (html: string) =>
  [...html.matchAll(/<link rel="modulepreload" href="([^"]+)">/g)].map((m) => m[1]);

test('the links go right before the island, with the base, the renderer and the document locale', () => {
  const html = inyectarPrecarga(pagina(), manifiestoDePrecarga(bundle));
  const hrefs = enlaces(html);
  expect(hrefs).toContain('/mail/_astro/MailApp.a.js');
  expect(hrefs).toContain('/mail/_astro/ui.c.js');
  expect(hrefs).toContain('/mail/_astro/client.k.js');
  expect(hrefs).toContain('/mail/_astro/es.d.js');
  expect(hrefs).toContain('/mail/_astro/es.e.js');
  expect(hrefs).not.toContain('/mail/_astro/de.f.js');
  expect(hrefs).not.toContain('/mail/_astro/parse5.j.js');
  // Cada fichero una sola vez, aunque lo importen la isla y el renderer.
  expect(hrefs.filter((h) => h === '/mail/_astro/preact.b.js')).toHaveLength(1);
  expect(html.indexOf('<link rel="modulepreload"')).toBeGreaterThan(html.indexOf('<body>'));
  expect(html.lastIndexOf('<link rel="modulepreload"')).toBeLessThan(html.indexOf('<astro-island'));
  // Lo demás no cambia.
  expect(html.replace(/<link rel="modulepreload" href="[^"]+">/g, '')).toBe(pagina());
});

test('a regional lang tag uses its primary subtag (www writes es-ES)', () => {
  const html = inyectarPrecarga(
    pagina().replace('lang="es"', 'lang="es-ES"'),
    manifiestoDePrecarga(bundle),
  );
  expect(enlaces(html)).toContain('/mail/_astro/es.d.js');
});

test('idle and only islands are preloaded; visible and media keep their laziness', () => {
  const m = manifiestoDePrecarga(bundle);
  expect(enlaces(inyectarPrecarga(pagina('idle'), m))).not.toHaveLength(0);
  expect(enlaces(inyectarPrecarga(pagina('only'), m))).not.toHaveLength(0);
  expect(enlaces(inyectarPrecarga(pagina('visible'), m))).toHaveLength(0);
  expect(enlaces(inyectarPrecarga(pagina('media'), m))).toHaveLength(0);
});

test('streamed in any split, the output is the same as in one piece', () => {
  const m = manifiestoDePrecarga(bundle);
  const html = pagina() + pagina().replace('<!doctype html>', '');
  const entero = inyectarPrecarga(html, m);
  for (const paso of [1, 2, 3, 7, 13, 50]) {
    const inyector = new InyectorDePrecarga(m);
    let salida = '';
    for (let i = 0; i < html.length; i += paso) salida += inyector.push(html.slice(i, i + paso));
    salida += inyector.end();
    expect(salida).toBe(entero);
  }
});

test('an island that is not in the manifest, or HTML without islands, passes untouched', () => {
  const m = manifiestoDePrecarga(bundle);
  const otra = pagina().replaceAll('MailApp.a.js', 'Otra.z.js').replace('client.k.js', 'z.js');
  expect(inyectarPrecarga(otra, m)).toBe(otra);
  const sin = '<html lang="es"><body><p>&lt;astro-island</p></body></html>';
  expect(inyectarPrecarga(sin, m)).toBe(sin);
});
