/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Ir a» (Ctrl+G / F5) y el cuadro de nombres de la barra de fórmulas: lo que se
 * escribe ahí —«B12», «Hoja2!A1:C5», «'Mi hoja'!D4», «A:C», «3:5» o un nombre
 * definido— se convierte en la hoja y el rango a los que saltar. Es la regla PURA,
 * sin DOM: la app nativa tiene el mismo puerto en `apps/fyne/calcus/ir_a.go`, y los
 * dos dan lo mismo con los vectores compartidos (`test/vectores/calcus.ts`).
 *
 * Con hojas de un millón de filas no hay otra forma razonable de llegar a la fila
 * 800 000, y quien viene de Excel o de Sheets escribe la celda en ese cuadro sin
 * pensarlo.
 */
import type { DefinedName } from '../types';
import { extractRefs } from './formula-edit';
import { MAX_COLS, MAX_ROWS } from './rejilla';

/** Lo que hay que saber del libro para resolver un destino. */
export interface LibroIrA {
  hojas: readonly string[];
  /** Índice de la hoja activa: la de una referencia sin hoja. */
  activa: number;
  nombres?: readonly DefinedName[] | undefined;
}

/** Por qué no se puede ir; la clave de su texto es `sheet.goTo.<código>`. «vacio» no
 *  avisa: Intro en el cuadro sin nada escrito solo devuelve el foco a la hoja. */
export type ErrorIrA = 'vacio' | 'invalido' | 'hoja' | 'nombre' | 'fuera' | 'noRango';

/** Adónde se va: la hoja (índice) y el rango, 0-based y ordenado. */
export interface DestinoIrA {
  hoja: number;
  c1: number;
  r1: number;
  c2: number;
  r2: number;
}

export type ResultadoIrA = DestinoIrA | { error: ErrorIrA };

/** La fila o columna «entera» de un eje libre en `extractRefs` (2^20 − 1). */
const EJE_ENTERO = 1_048_575;

/** Una hoja por su nombre, sin distinguir mayúsculas (como Excel); -1 si no está. */
const indiceHoja = (hojas: readonly string[], nombre: string): number => {
  const i = hojas.indexOf(nombre);
  if (i >= 0) return i;
  const up = nombre.toUpperCase();
  return hojas.findIndex((h) => h.toUpperCase() === up);
};

/**
 * Si `texto` es UNA referencia entera (celda, rango o eje, con o sin hoja), su destino;
 * null si no lo es. `hojaPorDefecto` es la de una referencia sin prefijo.
 */
function referencia(texto: string, libro: LibroIrA, hojaPorDefecto: number): ResultadoIrA | null {
  const refs = extractRefs(`=${texto}`);
  const r = refs[0];
  if (refs.length !== 1 || !r || r.start !== 1 || r.end !== texto.length + 1) return null;
  const hoja = r.sheet === undefined ? hojaPorDefecto : indiceHoja(libro.hojas, r.sheet);
  if (hoja < 0) return { error: 'hoja' };
  // Un eje libre llega hasta 2^20 − 1, que son las filas de la hoja pero no sus columnas.
  const c2 = r.rect.c2 === EJE_ENTERO ? MAX_COLS - 1 : r.rect.c2;
  const r2 = r.rect.r2 === EJE_ENTERO ? MAX_ROWS - 1 : r.rect.r2;
  if (c2 > MAX_COLS - 1 || r2 > MAX_ROWS - 1) return { error: 'fuera' };
  return { hoja, c1: r.rect.c1, r1: r.rect.r1, c2, r2 };
}

/** «Hoja!Nombre» o «'Mi hoja'!Nombre»: el nombre con ámbito de esa hoja. */
const CON_HOJA_RE = /^(?:'((?:[^']|'')+)'|([^!']+))!(.+)$/;
/** Lo que puede ser un nombre definido (la regla completa está en `nameError`). */
const FORMA_NOMBRE_RE = /^[\p{L}_\\][\p{L}\p{N}_.\\?]*$/u;

/**
 * El destino de lo escrito en el cuadro de nombres, o por qué no lo hay. Primero una
 * referencia; si no, un nombre definido: el de la hoja activa tapa al del libro, como
 * en las fórmulas, y «Hoja2!Nombre» busca el de esa hoja. La definición del nombre
 * tiene que ser un rango («Hoja1!$B$2:$B$9»); una constante o una fórmula no lleva a
 * ninguna parte.
 *
 * ponytail: ni varias áreas («A1,C3») ni tablas por su nombre; Excel las admite, aquí
 * avisan como cualquier texto que no se entiende. Añadirlas si alguien las echa de menos.
 */
export function irA(texto: string, libro: LibroIrA): ResultadoIrA {
  const t = texto.trim().replace(/^=/, '').trim();
  if (t === '') return { error: 'vacio' };
  const ref = referencia(t, libro, libro.activa);
  if (ref) return ref;

  let ambito = libro.hojas[libro.activa];
  let nombre = t;
  const m = CON_HOJA_RE.exec(t);
  if (m) {
    const i = indiceHoja(libro.hojas, m[1] !== undefined ? m[1].replace(/''/g, "'") : m[2]!);
    if (i < 0) return { error: 'hoja' };
    // «Año!B2»: en una fórmula esa hoja iría entre comillas, pero en el cuadro se
    // escribe tal cual, así que lo de detrás del «!» se mira también como referencia.
    const enHoja = referencia(m[3]!, libro, i);
    if (enHoja) return enHoja;
    ambito = libro.hojas[i];
    nombre = m[3]!;
  }
  if (!FORMA_NOMBRE_RE.test(nombre)) return { error: 'invalido' };
  const up = nombre.toUpperCase();
  const nombres = libro.nombres ?? [];
  const deHoja = nombres.find((n) => n.sheet === ambito && n.name.toUpperCase() === up);
  // Con «Hoja!Nombre» solo vale el de esa hoja; sin hoja, también el del libro.
  const def =
    deHoja ??
    (m ? undefined : nombres.find((n) => n.sheet === undefined && n.name.toUpperCase() === up));
  if (!def) return { error: 'nombre' };
  // Una referencia sin hoja dentro de un nombre es de su hoja, o de la activa si es del libro.
  const hojaDef = def.sheet !== undefined ? indiceHoja(libro.hojas, def.sheet) : libro.activa;
  const destino = referencia(def.ref.trim().replace(/^=/, ''), libro, Math.max(hojaDef, 0));
  return destino ?? { error: 'noRango' };
}

/** Los nombres que ofrece el desplegable del cuadro: los que ve la hoja activa (el de la
 *  hoja tapa al del libro) y que llevan a un rango, en orden alfabético. */
export function nombresParaIrA(libro: LibroIrA): string[] {
  const vistos = new Map<string, string>();
  const hoja = libro.hojas[libro.activa];
  for (const n of libro.nombres ?? []) {
    const k = n.name.toUpperCase();
    if (n.sheet !== undefined && n.sheet !== hoja) continue;
    if (n.sheet === undefined && vistos.has(k)) continue;
    vistos.set(k, n.name);
  }
  return [...vistos.values()]
    .filter((n) => !('error' in irA(n, libro)))
    .sort((a, b) => {
      // Por el texto en mayúsculas y sin `localeCompare`: el orden tiene que ser el mismo
      // en la app nativa, que no ordena con las reglas de un idioma.
      const x = a.toUpperCase();
      const y = b.toUpperCase();
      return x < y ? -1 : x > y ? 1 : 0;
    });
}
