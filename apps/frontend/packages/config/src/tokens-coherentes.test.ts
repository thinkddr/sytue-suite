/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LOS TOKENS DICEN LO MISMO EN LOS TRES SITIOS DONDE SE LEEN: `branding/tokens/spacing.json`
 * (el canon que embeben las nativas), `theme.css` (lo que de verdad pinta la web) y la
 * tabla de forma de `docs/design.md` (de la que implementan las personas y los agentes).
 *
 * Por qué hace falta: el 2026-09-15 `theme.css` subió `--z-dropdown` por encima de la
 * barra lateral y spacing.json se quedó con el orden viejo; `shadows.focus` siguió con un
 * verde que ya no era el del anillo, y design.md seguía diciendo que un botón tiene
 * 999 px de esquina cuando el código y sus tests decían 4. Cada copia, sola, parecía bien.
 */
import { expect, test } from 'bun:test';
import { existsSync, readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

const ruta = (rel: string) => fileURLToPath(new URL(rel, import.meta.url));

const TEMA = readFileSync(ruta('./theme.css'), 'utf8');
const SPACING_RUTA = ruta('./tokens/spacing.json');
const CANON_RUTA = ruta('../../../../../branding/tokens/spacing.json');
const DESIGN_RUTA = ruta('../../../../../docs/design.md');

type Hoja = {
  radii: Record<string, string>;
  zIndex: Record<string, number | string>;
  motion: { duration: Record<string, string>; easing: Record<string, string> };
  shadows: Record<string, string> & { dark: Record<string, string> };
};
const spacing = JSON.parse(readFileSync(SPACING_RUTA, 'utf8')) as Hoja;

/** Sin las notas (`$note`) del catálogo, que no son tokens. */
const sinNotas = <T>(o: Record<string, T>) =>
  Object.fromEntries(Object.entries(o).filter(([k]) => !k.startsWith('$')));

const sinComentarios = (css: string) => css.replace(/\/\*[\s\S]*?\*\//g, '');
const CSS = sinComentarios(TEMA);

/** El cuerpo del primer bloque que abre `apertura`, con las llaves emparejadas. */
function bloque(css: string, apertura: string, desde = 0): string {
  const i = css.indexOf(apertura, desde);
  if (i < 0) throw new Error(`theme.css no tiene el bloque ${apertura}`);
  const abre = css.indexOf('{', i);
  let nivel = 0;
  for (let j = abre; j < css.length; j++) {
    if (css[j] === '{') nivel++;
    else if (css[j] === '}' && --nivel === 0) return css.slice(abre + 1, j);
  }
  throw new Error(`bloque ${apertura} sin cerrar`);
}

/** Las declaraciones de PRIMER nivel de un bloque (las de bloques anidados no cuentan). */
function declaraciones(cuerpo: string): Map<string, string> {
  let plano = '';
  let nivel = 0;
  for (const c of cuerpo) {
    if (c === '{') nivel++;
    else if (c === '}') nivel--;
    else if (nivel === 0) plano += c;
  }
  const m = new Map<string, string>();
  for (const d of plano.matchAll(/(--[\w-]+)\s*:\s*([^;]+);/g)) {
    m.set(d[1]!, d[2]!.replace(/\s+/g, ' ').trim());
  }
  return m;
}

const tema = declaraciones(bloque(CSS, '@theme'));
const prefijo = (p: string) =>
  Object.fromEntries(
    [...tema].filter(([k]) => k.startsWith(p)).map(([k, v]) => [k.slice(p.length), v]),
  );

/** Las esquinas de forma, que reparte «Tokens de forma» y no son escalones de la escala. */
const RADIOS_DE_FORMA = new Set(['control', 'nav', 'card', 'row', 'sheet']);

test('spacing.json (la copia del frontend) es la de branding/', () => {
  if (!existsSync(CANON_RUTA)) return; // Fuera del monorepo (imagen del frontend) no está.
  expect(readFileSync(SPACING_RUTA, 'utf8'), 'ejecutar scripts/sync-branding.sh').toBe(
    readFileSync(CANON_RUTA, 'utf8'),
  );
});

test('el z-index de spacing.json es el orden de theme.css', () => {
  const json = Object.fromEntries(
    Object.entries(sinNotas(spacing.zIndex)).map(([k, v]) => [k, String(v)]),
  );
  expect(json).toEqual(prefijo('--z-'));
});

test('los radios de spacing.json son los de theme.css', () => {
  const json = sinNotas(spacing.radii);
  // `none` y `full` (50 %, relativo a la caja) los trae Tailwind: `rounded-none`/`-full`.
  const { none: _nada, full: _relativo, ...escala } = json;
  const css = Object.fromEntries(
    Object.entries(prefijo('--radius-')).filter(([k]) => !RADIOS_DE_FORMA.has(k)),
  );
  expect(escala).toEqual(css);
});

test('el motion de spacing.json es el de theme.css', () => {
  expect(sinNotas(spacing.motion.easing)).toEqual(prefijo('--ease-'));
  expect(sinNotas(spacing.motion.duration)).toEqual(prefijo('--duration-'));
});

test('las sombras y el anillo de foco de spacing.json son los de theme.css', () => {
  // `none` es `shadow-none` de Tailwind; el foco no es una sombra de elevación.
  const { dark, focus, none: _nada, ...claras } = spacing.shadows;
  expect(sinNotas(claras)).toEqual(prefijo('--shadow-'));
  const oscuro = Object.fromEntries(
    [...declaraciones(bloque(CSS, "[data-theme='dark'] {"))]
      .filter(([k]) => k.startsWith('--shadow-'))
      .map(([k, v]) => [k.slice('--shadow-'.length), v]),
  );
  expect(dark).toEqual(oscuro);
  const anillo = /:where\(:focus-visible\)\s*\{[^}]*box-shadow:\s*([^;]+);/.exec(CSS)?.[1];
  expect(focus).toBe(anillo?.trim());
});

// ---- La tabla de forma ----

/** «999 px», «999px», «0px» y «0»; «1,1» y «1.1»; los rem a px con la raíz de 16. */
function normal(valor: string): string {
  const v = valor
    .trim()
    .replace(/\s+px\b/g, 'px')
    .replace(',', '.');
  const rem = /^([\d.]+)rem$/.exec(v);
  if (rem) return normal(`${Number(rem[1]) * 16}px`);
  return v === '0px' ? '0' : v;
}

/** Filas `| `--token` | Marco | Carril |` de una tabla en Markdown o en un comentario. */
function tabla(texto: string): Map<string, [string, string]> {
  const filas = new Map<string, [string, string]>();
  for (const linea of texto.split('\n')) {
    const celdas = linea.trim().split('|').slice(1);
    if (celdas.length < 3 || !celdas[0]!.includes('`--')) continue;
    // `--a` / `--b` con «800 / 700»: una fila por token. Los que abrevian (`-fill`) no
    // tienen nombre completo y son colores de rol, no medidas: no entran.
    const tokens = [...celdas[0]!.matchAll(/`(--[\w-]+)`/g)].map((m) => m[1]!);
    const marco = celdas[1]!.split('/');
    const carril = celdas[2]!.split('/');
    tokens.forEach((t, i) => {
      if (marco[i] !== undefined && carril[i] !== undefined) {
        filas.set(t, [normal(marco[i]), normal(carril[i])]);
      }
    });
  }
  return filas;
}

const comentarioDeForma = (() => {
  const i = TEMA.indexOf('---- Tokens de FORMA ----');
  expect(i, 'theme.css no tiene el comentario «Tokens de FORMA»').toBeGreaterThan(-1);
  return TEMA.slice(i, TEMA.indexOf('*/', i));
})();
const tablaDelTema = tabla(comentarioDeForma);

test('la tabla de «Tokens de FORMA» de theme.css es lo que declara theme.css', () => {
  expect(tablaDelTema.size).toBeGreaterThan(10);
  const raiz = bloque(CSS, ':root', CSS.indexOf('@custom-variant carril'));
  const marco = new Map([...tema, ...declaraciones(raiz)]);
  const carril = new Map([...marco, ...declaraciones(bloque(CSS, "[data-forma='carril'] {"))]);
  const resuelve = (m: Map<string, string>, v: string): string => {
    const ref = /^var\((--[\w-]+)\)$/.exec(v);
    return ref ? resuelve(m, m.get(ref[1]!) ?? v) : normal(v);
  };
  // Los pesos y la escala de los títulos solo cambian DENTRO de la hoja (appearance.css):
  // la raíz lleva el valor de fuera, y lo que dice la tabla es el de dentro.
  const apariencia = sinComentarios(readFileSync(ruta('./tokens/appearance.css'), 'utf8'));
  const malos: string[] = [];
  for (const [token, [m, c]] of tablaDelTema) {
    if (!marco.has(token) || new RegExp(`${token}\\s*:`).test(apariencia)) continue;
    const real: [string, string] = [
      resuelve(marco, marco.get(token)!),
      resuelve(carril, carril.get(token)!),
    ];
    if (real[0] !== m || real[1] !== c)
      malos.push(`${token}: tabla ${m}/${c}, CSS ${real.join('/')}`);
  }
  expect(malos).toEqual([]);
});

test('la tabla de forma de docs/design.md es la de theme.css', () => {
  if (!existsSync(DESIGN_RUTA)) return;
  const design = readFileSync(DESIGN_RUTA, 'utf8');
  const i = design.indexOf('No hay dos juegos de componentes.');
  expect(i, 'design.md ya no tiene la sección de la forma (§1.0.1)').toBeGreaterThan(-1);
  const tablaDelDoc = tabla(design.slice(i, design.indexOf('**Forma «Marco»**', i)));
  const malos: string[] = [];
  for (const [token, valores] of tablaDelTema) {
    const doc = tablaDelDoc.get(token);
    if (!doc) malos.push(`${token}: falta en design.md`);
    else if (doc.join('/') !== valores.join('/')) {
      malos.push(`${token}: design.md ${doc.join('/')}, theme.css ${valores.join('/')}`);
    }
  }
  expect(malos).toEqual([]);
});
