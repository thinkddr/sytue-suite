/**
 * El arte de marca fijo —la tarjeta social, el hero de la suite y el icono
 * maskable— habla con la identidad ACTUAL, no con la anterior.
 *
 * Estas tres piezas embeben sus colores a propósito (no heredan `var(--sytue-*)`:
 * son superficie de marca independiente del tema), así que nada las arrastra
 * cuando cambia la identidad. Ya pasó una vez: la marca pasó a grafito y latón y
 * estas tres se quedaron en el azul eléctrico de antes. El síntoma era una app
 * instalada como PWA con un icono azul mientras la pestaña enseñaba el badge
 * negro: dos marcas para un mismo producto. Como no rompe nada, solo lo caza un
 * gate.
 *
 * La paleta permitida se LEE del símbolo (`branding/logo/sytue-mark.svg`), más el
 * campo y el badge grafito de las superficies de marca. Si la marca cambia, este
 * test señala qué arte se ha quedado atrás.
 */
import { describe, expect, it } from 'bun:test';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

const RAIZ = fileURLToPath(new URL('../../../../../../', import.meta.url));
const leer = (ruta: string) => readFileSync(`${RAIZ}${ruta}`, 'utf8');

/** Fuente de verdad en `branding/` y las copias que deja `sync-branding.sh`. */
const ARTE = {
  'og-image': [
    'branding/og/og-image.svg',
    'apps/frontend/packages/ui/src/assets/illustrations/og-image.svg',
  ],
  'favicon-maskable': [
    'branding/og/favicon-maskable.svg',
    'apps/frontend/packages/ui/src/assets/illustrations/favicon-maskable.svg',
  ],
  'hero-suite': [
    'branding/illustrations/hero-suite.svg',
    'apps/frontend/packages/ui/src/assets/illustrations/hero-suite.svg',
    'apps/fyne/iconos/ilustraciones/hero-suite.svg',
  ],
} as const;

const SIMBOLO = leer('branding/logo/sytue-mark.svg');

const hexes = (svg: string) =>
  new Set([...svg.matchAll(/#[0-9a-f]{6}\b|#[0-9a-f]{3}\b/gi)].map(([h]) => h.toLowerCase()));

/** Campo grafito y badge grafito de las superficies de marca (og-image, hero). */
const GRAFITO = ['#1a1d21', '#0c0e10'];
const PALETA = new Set([...hexes(SIMBOLO), ...GRAFITO]);

/** El monograma «sy:» del símbolo: un trazado, así que se pinta sin fuentes. */
const MONOGRAMA = SIMBOLO.match(/<path fill="#FFFFFF" d="([^"]+)"/i)?.[1] ?? '';

describe('arte de marca fijo', () => {
  it('el símbolo trae su monograma (si no, el resto del test no prueba nada)', () => {
    expect(MONOGRAMA.length).toBeGreaterThan(100);
    expect(PALETA.has('#b98f4f')).toBe(true);
  });

  for (const [nombre, [fuente, ...copias]] of Object.entries(ARTE)) {
    const svg = leer(fuente);

    it(`${nombre}: solo colores de la marca actual`, () => {
      const fuera = [...hexes(svg)].filter((h) => !PALETA.has(h));
      expect(fuera).toEqual([]);
      // Un color escrito de otra forma (`rgb()`, `hsl()`, `blue`…) se le escaparía al
      // filtro de hexes: aquí solo valen hex, `none` y referencias `url(#…)`.
      const otros = [...svg.matchAll(/\b(?:fill|stroke|stop-color|color)="([^"]+)"/gi)]
        .map(([, v]) => v.trim())
        .filter((v) => !/^(?:#[0-9a-f]{3}|#[0-9a-f]{6}|none|url\(#[\w-]+\))$/i.test(v));
      expect(otros).toEqual([]);
    });

    it(`${nombre}: lleva el símbolo de la marca, no un dibujo propio`, () => {
      expect(svg).toContain(MONOGRAMA);
      // El triángulo de latón del badge: el acento, en su sitio.
      expect(svg.toLowerCase()).toContain('#b98f4f');
    });

    it(`${nombre}: las copias vendorizadas son la fuente, byte a byte`, () => {
      for (const copia of copias) expect(`${copia}\n${leer(copia)}`).toBe(`${copia}\n${svg}`);
    });
  }
});
