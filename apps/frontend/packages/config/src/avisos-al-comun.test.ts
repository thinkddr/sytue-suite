/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LA CAJA DE AVISO LA PONE `InlineHelp` (`@sytue/ui`). Las apps tenían unas cuarenta
 * cajas hechas a mano con `bg-warning-bg rounded-md p-3` y compañía, y cada una
 * decidía su radio (`rounded`, `-sm`, `-md`, `-lg`, `-xl`), su relleno, su icono y si
 * se anunciaba al lector de pantalla o no (la mitad de los errores no lo hacían). La
 * común trae el anuncio por tono (`alert`/`status`, o `live` para cambiarlo), el
 * icono, el titular (también como encabezado) y la acción a la derecha.
 *
 * Busca en las cadenas de clase de las apps (y de la ofimática, que es código de app
 * aunque viva en `packages/office`) un fondo de estado `bg-(warning|danger|info|
 * success)-bg` junto a una esquina `rounded*` y un relleno `p-*`/`px-*`/`py-*`: esa
 * combinación es una caja de aviso. Las variantes `hover:`/`focus:` no cuentan (son
 * el fondo de un botón al pasar). Los banners de borde a borde no llevan esquina y
 * no saltan (`DemoBanner`, la franja de «sin conexión» de una lista).
 *
 * Es un TRINQUETE: `PERMITIDAS` cuenta lo que queda por fichero con su porqué. Una
 * caja nueva lo rompe; quitar una de la lista sin bajar su cuenta, también, para que
 * la excepción no se quede abierta para la siguiente.
 */
import { expect, test } from 'bun:test';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

const FRONTEND = fileURLToPath(new URL('../../../', import.meta.url));
const RAICES = ['apps', 'packages/office/src'].map((r) => join(FRONTEND, r));

const NO_ES_UNA_CAJA = 'no es una caja de aviso';
const PDF_DENSO = 'panel lateral del PDF a text-2xs; pendiente de un tamaño compacto de InlineHelp';
// Fuera del alcance de la pieza (las apps de la suite): la web pública las pinta en Astro
// como nota legal `role="note"` en prosa, sin icono; pasarlas cambia la cara de su legal.
const WWW = 'web pública: nota legal role="note" en prosa, fuera del alcance de DS-23';

/** Fichero (relativo a `apps/frontend`) → cuántas quedan y por qué. */
const PERMITIDAS: Record<string, { n: number; porque: string }> = {
  'apps/www/src/pages/[...locale]/legal.astro': { n: 1, porque: WWW },
  'apps/www/src/pages/[...locale]/privacy.astro': { n: 1, porque: WWW },
  'apps/www/src/pages/[...locale]/terms.astro': { n: 1, porque: WWW },
  'apps/www/src/pages/[...locale]/product/[key].astro': { n: 1, porque: WWW },
  'apps/rooms/src/components/MessageBody.tsx': {
    n: 1,
    porque: `${NO_ES_UNA_CAJA}: el resaltado en línea de una mención`,
  },
  'apps/rooms/src/components/MessageView.tsx': {
    n: 1,
    porque: `${NO_ES_UNA_CAJA}: la píldora de una reacción`,
  },
  'apps/pages/src/ia/AjustesIA.tsx': {
    n: 1,
    porque: `${NO_ES_UNA_CAJA}: la píldora de estado de la clave`,
  },
  'apps/pages/src/islands/SitioDetalleApp.tsx': {
    n: 1,
    porque: `${NO_ES_UNA_CAJA}: el paso hecho de la lista de progreso`,
  },
  'packages/office/src/pdf/PdfApp.tsx': {
    n: 1,
    porque: `${NO_ES_UNA_CAJA}: un botón destructivo de la barra`,
  },
  'packages/office/src/pdf/CompararVisual.tsx': { n: 4, porque: PDF_DENSO },
  // 1 de las 9 es la etiqueta de estado de una anotación (no es una caja).
  'packages/office/src/pdf/Paneles.tsx': { n: 9, porque: PDF_DENSO },
};

function fuentes(dir: string, out: string[] = []): string[] {
  for (const e of readdirSync(dir)) {
    if (e === 'node_modules' || e === 'dist' || e === '.astro') continue;
    const p = join(dir, e);
    if (statSync(p).isDirectory()) fuentes(p, out);
    else if (/\.(tsx|astro)$/.test(e) && !e.includes('.test.')) out.push(p);
  }
  return out;
}

// Una clase cuenta solo si va suelta: ni `hover:bg-danger-bg` ni `border-danger-bg/30`.
const FONDO = /(?<![\w:/-])bg-(?:warning|danger|info|success)-bg\b/;
const ESQUINA = /(?<![\w:/-])rounded(?:-[\w]+)?(?![\w/-])/;
const RELLENO = /(?<![\w:/-])p[xy]?-[\d.]+(?![\w/-])/;

/**
 * Cuántas cadenas de clase de `src` son una caja de aviso hecha a mano. También la que
 * esconde el fondo en una tabla de tonos (`TONOS = { error: 'bg-danger-bg …' }`) y lo
 * interpola en `\`${TONOS[tono]} rounded-md p-3\``: así eran los dos `Aviso` propios de
 * Pages, que se le escapaban al detector de cadena a cadena.
 */
function cajasDeAviso(src: string): number {
  let n = 0;
  const cadenas = [...src.matchAll(/(["'`])((?:(?!\1)[^\\]|\\.)*?)\1/g)].map((m) => m[2] ?? '');
  const tablaDeTonos = cadenas.some((s) => FONDO.test(s) && !ESQUINA.test(s));
  for (const s of cadenas) {
    if (!ESQUINA.test(s) || !RELLENO.test(s)) continue;
    if (FONDO.test(s) || (tablaDeTonos && /\$\{[^}]*\b(?:TONOS?|tonos?|caja)\b/.test(s))) n++;
  }
  return n;
}

test('el detector ve la caja hecha a mano y deja pasar el botón y el banner', () => {
  expect(cajasDeAviso(`<p class="bg-warning-bg text-warning-text rounded-md p-3">x</p>`)).toBe(1);
  expect(
    cajasDeAviso(`class={\`rounded-md border px-3 py-2 \${ok ? 'a' : 'b'} bg-danger-bg\`}`),
  ).toBe(1);
  // El fondo al pasar de un botón de borrar no es un aviso.
  expect(cajasDeAviso(`class="hover:bg-danger-bg rounded-control p-1.5"`)).toBe(0);
  // El banner de borde a borde no lleva esquina.
  expect(cajasDeAviso(`class="bg-warning-bg border-b px-4 py-2"`)).toBe(0);
  // La tabla de tonos interpolada en la caja también es una caja hecha a mano.
  expect(
    cajasDeAviso(
      `const TONOS = { error: 'bg-danger-bg text-danger-text' };\n` +
        `<p class={\`\${TONOS[tono]} rounded-md p-3 text-sm\`} />`,
    ),
  ).toBe(1);
  // Una casilla de color sin relleno tampoco.
  expect(cajasDeAviso(`class="bg-info-bg grid h-9 w-9 rounded-md"`)).toBe(0);
});

test('las cajas de aviso de las apps son InlineHelp (trinquete)', () => {
  const vistas: Record<string, number> = {};
  for (const raiz of RAICES) {
    for (const f of fuentes(raiz)) {
      const n = cajasDeAviso(readFileSync(f, 'utf8'));
      if (n) vistas[relative(FRONTEND, f)] = n;
    }
  }
  const esperadas = Object.fromEntries(Object.entries(PERMITIDAS).map(([f, { n }]) => [f, n]));
  // Si sale un fichero nuevo: pásalo a `<InlineHelp tone=…>`. Si baja una cuenta:
  // bájala aquí también (o borra la fila).
  expect(vistas).toEqual(esperadas);
});
