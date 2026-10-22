/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El núcleo, puro, de GROUPBY y PIVOTBY: agrupar las filas de una matriz por uno o varios
 * campos y resumir cada grupo con una función. El motor (engine.ts) los despacha aparte
 * del registro porque su argumento `función` es un LAMBDA (o el nombre de una función),
 * que no cabe en un valor; aquí llega ya hecho un `Agregador`.
 *
 * Decisiones, para quien compare con otras hojas de cálculo:
 * - Las etiquetas de un grupo se repiten en TODAS sus filas (tabla plana): no se
 *   «ahueca» la jerarquía, así que el resultado se puede volver a usar como datos.
 * - Subtotales (total_depth ±2) solo cuando hay más de un campo; con uno es igual a ±1.
 *   Se ordenan por la primera aparición del grupo exterior tras ordenar las hojas.
 * - PIVOTBY resume UNA columna de valores (con varias, #VALUE!) y no calcula subtotales
 *   de campos múltiples (|total| ≥ 2 con más de un campo, #VALUE!).
 * - El total se llama «Total»; las cabeceras generadas (modo 2), «Campo n» y «Valor n».
 */
import type { CellValue } from '../values.js';
import { ERR, FormulaError, compareValues } from '../values.js';

/** Resume los valores de UN grupo (una columna) de una columna de valores. `todos` es la
 *  misma columna con TODAS las filas filtradas: PERCENTOF la necesita como denominador. */
export type Agregador = (grupo: CellValue[][], todos: CellValue[][]) => CellValue;

export interface OpcionesAgrupar {
  /** Campos por los que se agrupa: n filas × k columnas. */
  filas: CellValue[][];
  /** Valores a resumir: n filas × m columnas. */
  valores: CellValue[][];
  agregar: Agregador;
  /** 0 sin cabeceras, 1 las hay pero no se muestran, 2 no las hay y se generan, 3 las hay
   *  y se muestran. Sin indicar: se detectan (texto sobre número). */
  cabeceras?: number | undefined;
  /** 0 sin totales, 1 total general, 2 total general y subtotales; negativo = arriba. */
  totales?: number | undefined;
  /** Columnas de la salida por las que ordenar (1-based; negativo = descendente). */
  orden?: number[] | undefined;
  /** Una columna de n filas: solo cuentan las filas verdaderas. */
  filtro?: CellValue[][] | undefined;
}

export interface OpcionesPivot {
  filas: CellValue[][];
  columnas: CellValue[][];
  valores: CellValue[][];
  agregar: Agregador;
  cabeceras?: number | undefined;
  /** Total de filas (una fila «Total»): 0, ±1; ±2 solo con un campo. */
  totalFilas?: number | undefined;
  ordenFilas?: number[] | undefined;
  /** Total de columnas (una columna «Total»). */
  totalColumnas?: number | undefined;
  ordenColumnas?: number[] | undefined;
  filtro?: CellValue[][] | undefined;
}

const TOTAL = 'Total';

/** Clave estable de una tupla de valores: «1» (número) y «1» (texto) no son lo mismo. */
function claveDe(partes: readonly CellValue[]): string {
  return JSON.stringify(
    partes.map((v) => (v instanceof FormulaError ? `#${v.code}` : [typeof v, v])),
  );
}

function comparar(a: CellValue, b: CellValue): number {
  // Un error en la clave de orden no ordena: empata (era NaN en el comparador, que sort trata como 0).
  const r = compareValues(a, b);
  return typeof r === 'number' ? r : 0;
}

/** Texto de la cabecera detectada: ¿la primera fila de los valores es texto y la
 *  siguiente número? Es el criterio de las hojas de cálculo para «hay cabeceras». */
function hayCabeceras(valores: CellValue[][]): boolean {
  const a = valores[0]?.[0];
  const b = valores[1]?.[0];
  return typeof a === 'string' && a !== '' && typeof b === 'number';
}

interface Preparado {
  hayCab: boolean;
  muestraCab: boolean;
  cabFilas: CellValue[];
  cabValores: CellValue[];
  filas: CellValue[][];
  columnas: CellValue[][];
  valores: CellValue[][];
}

/** Separa las cabeceras de los datos, aplica el filtro y comprueba las dimensiones. */
function preparar(
  filas: CellValue[][],
  columnas: CellValue[][] | undefined,
  valores: CellValue[][],
  cabeceras: number | undefined,
  filtro: CellValue[][] | undefined,
): Preparado | FormulaError {
  if (filas.length === 0 || valores.length === 0) return ERR.VALUE;
  if (filas.length !== valores.length) return ERR.VALUE;
  if (columnas && columnas.length !== valores.length) return ERR.VALUE;
  if (filtro && filtro.length !== valores.length) return ERR.VALUE;
  const modo = cabeceras ?? (hayCabeceras(valores) ? 3 : 0);
  if (!Number.isInteger(modo) || modo < 0 || modo > 3) return ERR.VALUE;
  const hayCab = modo === 1 || modo === 3;
  const muestraCab = modo === 2 || modo === 3;
  const k = filas[0]!.length;
  const m = valores[0]!.length;
  const nombresDe = (fila: CellValue[] | undefined, n: number, prefijo: string): CellValue[] =>
    hayCab && fila ? fila.slice(0, n) : Array.from({ length: n }, (_, i) => `${prefijo} ${i + 1}`);
  const cabFilas = nombresDe(filas[0], k, 'Campo');
  const cabValores = nombresDe(valores[0], m, 'Valor');
  const resto = <T>(x: T[]): T[] => (hayCab ? x.slice(1) : x);
  let f = resto(filas);
  let c = columnas ? resto(columnas) : [];
  let v = resto(valores);
  if (filtro) {
    const bueno = resto(filtro).map((r) => {
      const x = r[0];
      return x === true || (typeof x === 'number' && x !== 0);
    });
    f = f.filter((_, i) => bueno[i]);
    c = c.filter((_, i) => bueno[i]);
    v = v.filter((_, i) => bueno[i]);
  }
  return { hayCab, muestraCab, cabFilas, cabValores, filas: f, columnas: c, valores: v };
}

/** Los valores de la columna `j` de las filas `idx`, como una columna n×1. */
function columna(valores: CellValue[][], idx: readonly number[], j: number): CellValue[][] {
  return idx.map((i) => [valores[i]![j] ?? '']);
}

/** Lo que devuelve un agregador, reducido a un escalar. */
function escalar(v: CellValue | CellValue[][]): CellValue {
  if (Array.isArray(v)) return v[0]?.[0] ?? ERR.VALUE;
  return v;
}

/** Un grupo de filas con la misma clave. */
interface Grupo {
  clave: CellValue[];
  idx: number[];
}

function agrupar(campos: CellValue[][]): Grupo[] {
  const mapa = new Map<string, Grupo>();
  campos.forEach((fila, i) => {
    const clave = fila;
    const id = claveDe(clave);
    const g = mapa.get(id);
    if (g) g.idx.push(i);
    else mapa.set(id, { clave, idx: [i] });
  });
  return [...mapa.values()];
}

/** Ordena `grupos` por las columnas pedidas de la salida: las ≤ k son partes de la clave;
 *  las demás, un valor ya resumido (`resumen(g)[j]`). */
function ordenar(
  grupos: Grupo[],
  k: number,
  orden: readonly number[] | undefined,
  resumen: (g: Grupo) => CellValue[],
): void {
  const crit = orden && orden.length > 0 ? orden : Array.from({ length: k }, (_, i) => i + 1);
  const cache = new Map<Grupo, CellValue[]>();
  const valoresDe = (g: Grupo): CellValue[] => {
    let r = cache.get(g);
    if (!r) {
      r = resumen(g);
      cache.set(g, r);
    }
    return r;
  };
  grupos.sort((a, b) => {
    for (const c of crit) {
      const j = Math.abs(c) - 1;
      const x = j < k ? a.clave[j]! : valoresDe(a)[j - k]!;
      const y = j < k ? b.clave[j]! : valoresDe(b)[j - k]!;
      const d = comparar(x, y);
      if (d !== 0) return c < 0 ? -d : d;
    }
    return 0;
  });
}

/** ¿Son válidas las columnas de orden (enteros no nulos dentro de 1..max)? */
function ordenValido(orden: readonly number[] | undefined, max: number): boolean {
  return (orden ?? []).every((c) => Number.isInteger(c) && c !== 0 && Math.abs(c) <= max);
}

export function groupBy(o: OpcionesAgrupar): CellValue[][] | FormulaError {
  const p = preparar(o.filas, undefined, o.valores, o.cabeceras, o.filtro);
  if (p instanceof FormulaError) return p;
  const k = p.cabFilas.length;
  const m = p.cabValores.length;
  const total = o.totales ?? 1;
  if (!Number.isInteger(total) || Math.abs(total) > 2) return ERR.VALUE;
  if (!ordenValido(o.orden, k + m)) return ERR.VALUE;
  if (p.filas.length === 0) return ERR.CALC; // el filtro lo dejó todo fuera

  const todas = p.filas.map((_, i) => i);
  const todos = (j: number): CellValue[][] => columna(p.valores, todas, j);
  const resumirIdx = (idx: readonly number[]): CellValue[] => {
    const out: CellValue[] = [];
    for (let j = 0; j < m; j++) out.push(escalar(o.agregar(columna(p.valores, idx, j), todos(j))));
    return out;
  };

  const grupos = agrupar(p.filas);
  ordenar(grupos, k, o.orden, (g) => resumirIdx(g.idx));

  const cuerpo: CellValue[][] = [];
  const conSub = Math.abs(total) === 2 && k > 1;
  if (!conSub) {
    for (const g of grupos) cuerpo.push([...g.clave, ...resumirIdx(g.idx)]);
  } else {
    // Subtotales de cada prefijo de la clave (de fuera adentro), tras sus hojas.
    const subarbol = (gs: Grupo[], nivel: number): CellValue[][] => {
      if (nivel === k - 1) return gs.map((g) => [...g.clave, ...resumirIdx(g.idx)]);
      const porPrefijo = new Map<string, Grupo[]>();
      for (const g of gs) {
        const id = claveDe(g.clave.slice(0, nivel + 1));
        const lista = porPrefijo.get(id);
        if (lista) lista.push(g);
        else porPrefijo.set(id, [g]);
      }
      const out: CellValue[][] = [];
      for (const lista of porPrefijo.values()) {
        const hijos = subarbol(lista, nivel + 1);
        const fila: CellValue[] = [
          ...lista[0]!.clave.slice(0, nivel + 1),
          TOTAL,
          ...Array.from({ length: k - nivel - 2 }, () => ''),
          ...resumirIdx(lista.flatMap((g) => g.idx)),
        ];
        // Subtotal ARRIBA si el total es negativo, abajo si no.
        out.push(...(total < 0 ? [fila, ...hijos] : [...hijos, fila]));
      }
      return out;
    };
    cuerpo.push(...subarbol(grupos, 0));
  }

  if (total !== 0) {
    const fila: CellValue[] = [
      TOTAL,
      ...Array.from({ length: k - 1 }, () => ''),
      ...resumirIdx(todas),
    ];
    if (total < 0) cuerpo.unshift(fila);
    else cuerpo.push(fila);
  }
  if (!p.muestraCab) return cuerpo;
  return [[...p.cabFilas, ...p.cabValores], ...cuerpo];
}

export function pivotBy(o: OpcionesPivot): CellValue[][] | FormulaError {
  const p = preparar(o.filas, o.columnas, o.valores, o.cabeceras, o.filtro);
  if (p instanceof FormulaError) return p;
  const kr = p.cabFilas.length;
  const kc = p.columnas[0]?.length ?? o.columnas[0]?.length ?? 0;
  if (kc === 0 || p.cabValores.length !== 1) return ERR.VALUE; // una columna de valores
  const totF = o.totalFilas ?? 1;
  const totC = o.totalColumnas ?? 1;
  const bien = (t: number, k: number): boolean =>
    Number.isInteger(t) && (Math.abs(t) <= 1 || (Math.abs(t) === 2 && k === 1));
  if (!bien(totF, kr) || !bien(totC, kc)) return ERR.VALUE;
  if (!ordenValido(o.ordenFilas, kr + 1) || !ordenValido(o.ordenColumnas, kc + 1)) return ERR.VALUE;
  if (p.filas.length === 0) return ERR.CALC;

  const todas = p.filas.map((_, i) => i);
  const todosV = columna(p.valores, todas, 0);
  const resumir = (idx: readonly number[]): CellValue =>
    escalar(o.agregar(columna(p.valores, idx, 0), todosV));

  const gFilas = agrupar(p.filas);
  const gCols = agrupar(p.columnas);
  // El criterio kr+1 / kc+1 ordena por el total de esa fila / columna.
  ordenar(gFilas, kr, o.ordenFilas, (g) => [resumir(g.idx)]);
  ordenar(gCols, kc, o.ordenColumnas, (g) => [resumir(g.idx)]);

  // Celda (fila, columna): las filas de datos que caen en las dos.
  const enCol = new Map<string, Set<number>>();
  for (const g of gCols) enCol.set(claveDe(g.clave), new Set(g.idx));
  const celda = (gf: Grupo, gc: Grupo): CellValue => {
    const set = enCol.get(claveDe(gc.clave))!;
    const idx = gf.idx.filter((i) => set.has(i));
    return idx.length === 0 ? '' : resumir(idx);
  };

  const conTotCol = totC !== 0;
  const colsSal: (Grupo | null)[] = gCols.map((g) => g);
  if (conTotCol) {
    if (totC < 0) colsSal.unshift(null);
    else colsSal.push(null);
  }
  const filaDe = (gf: Grupo | null): CellValue[] => {
    const out: CellValue[] = gf
      ? [...gf.clave]
      : [TOTAL, ...Array.from({ length: kr - 1 }, () => '')];
    for (const gc of colsSal) {
      if (gf && gc) out.push(celda(gf, gc));
      else if (gf)
        out.push(resumir(gf.idx)); // total de la fila
      else if (gc)
        out.push(resumir(gc.idx)); // total de la columna
      else out.push(resumir(todas)); // total general
    }
    return out;
  };
  const cuerpo = gFilas.map((g) => filaDe(g));
  if (totF !== 0) {
    if (totF < 0) cuerpo.unshift(filaDe(null));
    else cuerpo.push(filaDe(null));
  }

  // Cabeceras de columna: una fila por campo de columna; en la última, los nombres de los
  // campos de fila sobre las etiquetas de fila.
  const cab: CellValue[][] = [];
  for (let i = 0; i < kc; i++) {
    const esUltima = i === kc - 1;
    const izq: CellValue[] =
      esUltima && p.muestraCab ? [...p.cabFilas] : Array.from({ length: kr }, () => '');
    const claves: CellValue[] = colsSal.map((gc) => (gc ? gc.clave[i]! : i === 0 ? TOTAL : ''));
    cab.push([...izq, ...claves]);
  }
  return [...cab, ...cuerpo];
}
