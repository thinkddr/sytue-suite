/**
 * La marca teñida por organización descansa sobre dos cosas frágiles que hay que
 * fijar, porque las dos fallan EN SILENCIO:
 *
 *  1. El tinte se aplica reemplazando un literal del SVG. Si alguien reexporta el
 *     logotipo desde la herramienta de diseño y ese literal cambia, el reemplazo no
 *     encuentra nada, no da error, y todas las organizaciones vuelven al latón. Se
 *     ve perfectamente bien: nadie lo notaría.
 *  2. Los doce colores tienen que contrastar contra el badge blanco y contra el
 *     monograma oscuro. Un color nuevo metido a ojo pasa la revisión visual y deja
 *     la cuña ilegible en la barra a 28 px.
 *
 * Se leen los FICHEROS en vez de importar el componente: `?raw` lo resuelve Vite, y
 * este test corre en Bun. Ejecutar: `bun test`.
 */
import { describe, expect, test } from 'bun:test';
import { ANCLA_CUNA, COLORES_ORGMARK, colorDeOrg } from './OrgMark.js';

const svg = await Bun.file(new URL('../assets/logos/sytue-one-white.svg', import.meta.url)).text();
const tokens = await Bun.file(
  new URL('../../../config/src/tokens/colors.css', import.meta.url),
).text();

/** Contraste WCAG entre dos hex. */
function contraste(a: string, b: string): number {
  const lum = (hex: string): number => {
    const v = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255);
    const c = v.map((x) => (x <= 0.03928 ? x / 12.92 : ((x + 0.055) / 1.055) ** 2.4));
    return 0.2126 * c[0]! + 0.7152 * c[1]! + 0.0722 * c[2]!;
  };
  const [x, y] = [lum(a), lum(b)];
  return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05);
}

describe('OrgMark', () => {
  test('el ancla del tinte sigue en el SVG de marca', () => {
    // Si esto falla, el logotipo se reexportó y hay que actualizar ANCLA_CUNA: sin
    // ella el tinte deja de aplicarse y no lo avisa nadie.
    expect(svg).toContain(ANCLA_CUNA);
    // Y sigue siendo el ÚNICO sitio donde aparece: si hubiera dos, se teñiría solo
    // el primero y la marca saldría a dos colores.
    expect(svg.split(ANCLA_CUNA).length - 1).toBe(1);
  });

  test('los doce colores existen y se leen sobre el badge y junto al monograma', () => {
    for (let i = 1; i <= COLORES_ORGMARK; i += 1) {
      const m = tokens.match(new RegExp(`--sytue-orgmark-${i}:\\s*(#[0-9a-fA-F]{6})`));
      expect(m, `falta --sytue-orgmark-${i} en los tokens`).not.toBeNull();
      const color = m![1]!;
      // El badge es blanco y el monograma #22262B: la cuña va entre los dos.
      expect(contraste(color, '#ffffff'), `orgmark-${i} sobre el badge`).toBeGreaterThanOrEqual(3);
      expect(contraste(color, '#22262b'), `orgmark-${i} junto al monograma`).toBeGreaterThanOrEqual(
        3,
      );
    }
  });

  test('el color de una organización no cambia nunca', () => {
    // Lo importante no es QUÉ color toca, sino que sea el mismo siempre: uno que
    // cambia entre recargas o entre apps se lee como un fallo.
    expect(colorDeOrg('acme')).toBe(colorDeOrg('acme'));
    expect(colorDeOrg('')).toBeGreaterThanOrEqual(1);
    for (const slug of ['acme', 'contoso', 'x', 'una-organizacion-con-slug-largo']) {
      const n = colorDeOrg(slug);
      expect(n, slug).toBeGreaterThanOrEqual(1);
      expect(n, slug).toBeLessThanOrEqual(COLORES_ORGMARK);
    }
  });

  test('reparte: doce slugs distintos no caen todos en el mismo color', () => {
    const slugs = ['acme', 'contoso', 'globex', 'initech', 'umbrella', 'wayne'];
    expect(new Set(slugs.map(colorDeOrg)).size).toBeGreaterThan(1);
  });
});
