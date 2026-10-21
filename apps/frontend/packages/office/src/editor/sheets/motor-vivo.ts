/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El motor de fórmulas del editor, vivo entre ediciones. Antes se construía otro Engine
 * entero en cada cambio del modelo —parsear y registrar las dependencias de TODAS las
 * celdas, aunque solo se hubiera tocado una—; ahora se conserva y se le aplica solo la
 * diferencia. El motor ya sabe invalidar a los dependientes de una celda (revDeps), pero
 * solo si se le escribe con `setCell` sobre el mismo objeto.
 *
 * Lo que NO se puede aplicar como diferencia vuelve a construirse entero, y es poco:
 * cambiar las hojas (alta, baja, nombre), las tablas o sus encabezados (el resolutor de
 * `Tabla[Columna]` ya no daría lo mismo). Puro, sin DOM: lo prueban los tests.
 */
import { Engine, numToCol } from '@sytue/formula';
import type { SheetAutoFilter, SheetData, SheetModel } from '../types';
import { hiddenByFilter } from './filtro';
import { formatCellValue, rawText } from './format';
import { defineNames } from './nombres';
import { hiddenUnits } from './outline';
import { collectResolvableTables, compileTables, resolveStructuredRef } from './table';
import type { ResolvableTable } from './table';

const A1 = (c: number, r: number): string => `${numToCol(c)}${r + 1}`;

/** Un motor sincronizado con un modelo, y lo necesario para ponerlo al día con el
 *  siguiente sin reconstruirlo. */
export interface MotorVivo {
  engine: Engine;
  /** El modelo con el que está sincronizado. */
  modelo: SheetModel;
  /** Lo que, si cambia, obliga a reconstruir (hojas y tablas resolubles). */
  firma: string;
  /** Tablas resolubles vigentes: el resolutor del motor las lee por aquí, así que se
   *  puede cambiar sin recrearlo. */
  libro: { tablas: ResolvableTable[] };
  /** Lo que ocultan el esquema y el filtro en cada hoja, para saber si cambió. */
  ocultas: Map<string, string>;
}

/** Tablas resolubles del libro para `Tabla[Columna]`: los nombres de columna se leen
 *  del texto CRUDO de la fila de cabecera. Lo comparten el motor (resolutor inyectado)
 *  y el autocompletado/resaltado del editor. */
export function resolvableTablesOf(model: SheetModel): ResolvableTable[] {
  const cellsBySheet = new Map(model.sheets.map((s) => [s.name, s.cells]));
  return collectResolvableTables(model.sheets, (sheet, c, r) =>
    rawText(cellsBySheet.get(sheet)?.[A1(c, r)] ?? ''),
  );
}

/** Texto de agrupación/filtrado del valor calculado de una celda (sin numfmt: mismo
 *  criterio que el render General). */
export function cellText(engine: Engine, sheetName: string, c: number, r: number): string {
  return formatCellValue(engine.getValue(sheetName, A1(c, r)));
}

/** Filas ocultas por el autofiltro: una fila del cuerpo se oculta si en ALGUNA columna
 *  filtrada no pasa (valor excluido o condición; intersección, criterio Excel). Las filas
 *  `protect` (p. ej. las de totales de una tabla) nunca se ocultan. */
export function filteredRows(
  af: SheetAutoFilter | undefined,
  engine: Engine,
  sheetName: string,
  protect?: ReadonlySet<number>,
  styles?: SheetData['styles'],
): Set<number> {
  // Los valores excluidos y las condiciones (sheets/filtro.ts) se evalúan juntos. Hasta la
  // última fila usada: debajo todo está vacío, y la hoja llega al millón.
  return hiddenByFilter(
    af,
    {
      value: (c, r) => engine.getValue(sheetName, A1(c, r)),
      style: (c, r) => styles?.[A1(c, r)],
    },
    engine.usedRange(sheetName).maxRow + 1,
    protect,
  );
}

/**
 * Pistas de qué celdas cambiaron entre dos versiones de `cells`. Comparar dos hojas de
 * 100.000 celdas clave a clave cuesta ~75 ms por edición (medido); quien edita una sola
 * celda sabe cuál y lo anota aquí. Solo vale si `desde` es EXACTAMENTE el `cells` con el
 * que el motor está sincronizado: si entre medias hubo otra versión, no coincide y se
 * compara entera, como antes. La clave es el `cells` nuevo, así que la pista muere con él.
 */
const cambios = new WeakMap<object, { desde: object; refs: readonly string[] }>();

/** Anota que `nuevas` es `previas` con SOLO las celdas `refs` escritas o borradas. */
export function anotarCambio(
  nuevas: Record<string, string>,
  previas: Record<string, string>,
  refs: readonly string[],
): void {
  cambios.set(nuevas, { desde: previas, refs });
}

/** Lo que cambia si hay que reconstruir: nombres de hoja, tablas resolubles y nombres
 *  DEFINIDOS del libro (`model.names`): el motor los registra al construirse (`defineName`
 *  recalcula todas las dependencias), así que un cambio en ellos obliga a reconstruir. */
function firmaDe(model: SheetModel, tablas: ResolvableTable[]): string {
  // Solo lo que usa el resolutor: el estilo de una tabla no obliga a reconstruir.
  const t = tablas.map((x) => [x.sheet, x.table.name, x.rect, x.headers, x.totals, x.columns]);
  return JSON.stringify([model.sheets.map((s) => s.name), t, model.names ?? []]);
}

/** Texto de lo que ocultan el esquema y el filtro de una hoja ('' si nada). */
function ocultasDe(
  model: SheetModel,
  engine: Engine,
  i: number,
): { manual: Set<number>; filtrado: Set<number>; firma: string } {
  const s = model.sheets[i]!;
  const manual = hiddenUnits(s.rowGroups ?? []);
  const totals = new Set<number>();
  for (const ct of compileTables(s.tables ?? [])) if (ct.totals) totals.add(ct.rect.r2);
  const filtrado = s.autofilter
    ? filteredRows(s.autofilter, engine, s.name, totals, s.styles)
    : new Set<number>();
  const firma =
    manual.size === 0 && filtrado.size === 0
      ? ''
      : `${[...manual].sort((a, b) => a - b).join(',')}|${[...filtrado].sort((a, b) => a - b).join(',')}`;
  return { manual, filtrado, firma };
}

/**
 * SUBTOTAL y AGGREGATE necesitan saber qué filas no se ven: las de un grupo contraído
 * (ocultas a mano) y las que esconde el autofiltro. El filtro se evalúa con el motor ya
 * cargado, igual que al pintar. Solo se le dice al motor lo que cambió: `setHiddenRows`
 * vacía su caché entera, y no hay por qué pagarlo en cada edición si nada se ocultó ni
 * se mostró.
 */
function aplicarOcultas(vivo: MotorVivo, model: SheetModel): void {
  model.sheets.forEach((s, i) => {
    const previa = vivo.ocultas.get(s.name) ?? '';
    // Sin esquema ni filtro y sin nada que limpiar: ni se evalúa.
    if (previa === '' && !s.autofilter && (s.rowGroups ?? []).length === 0) return;
    const { manual, filtrado, firma } = ocultasDe(model, vivo.engine, i);
    if (firma === previa) return;
    vivo.engine.setHiddenRows(s.name, filtrado, manual);
    vivo.ocultas.set(s.name, firma);
  });
}

/** Construye un motor nuevo y completo desde el modelo. */
export function construirMotor(model: SheetModel): MotorVivo {
  // Referencias estructuradas Tabla[Columna]: el resolutor mapea (tabla, columna)
  // → rango leyendo las tablas del modelo (@sytue/formula, carril motor).
  const libro = { tablas: resolvableTablesOf(model) };
  const engine = new Engine({
    sheets: model.sheets.map((s) => s.name),
    resolveTable: (q, here) => resolveStructuredRef(libro.tablas, q, here),
  });
  defineNames(engine, model.names);
  for (const s of model.sheets) {
    for (const ref in s.cells) engine.setCell(s.name, ref, rawText(s.cells[ref] ?? ''));
  }
  const vivo: MotorVivo = {
    engine,
    modelo: model,
    firma: firmaDe(model, libro.tablas),
    libro,
    ocultas: new Map(),
  };
  aplicarOcultas(vivo, model);
  return vivo;
}

/** Escribe en el motor el estado de una celda (`html` undefined = borrada) si su texto
 *  crudo difiere del que ya tiene. Devuelve 1 si escribió y 0 si no. Cambiar solo el
 *  formato no cambia el texto crudo: no se escribe y no se invalida nada. */
function aplicarCelda(engine: Engine, hoja: string, ref: string, html: string | undefined): 0 | 1 {
  const raw = rawText(html ?? '');
  if (engine.getRaw(hoja, ref) === raw) return 0;
  engine.setCell(hoja, ref, raw);
  return 1;
}

/**
 * Pone el motor al día con `model`: escribe solo las celdas cuyo texto crudo cambió. Si
 * cambió la estructura (hojas o tablas), lo reconstruye. Devuelve el motor vivo —el mismo
 * objeto si pudo aplicar la diferencia— y cuántas celdas escribió (-1 si reconstruyó).
 */
export function sincronizarMotor(
  previo: MotorVivo | null,
  model: SheetModel,
): { vivo: MotorVivo; escritas: number } {
  if (previo?.modelo === model) return { vivo: previo, escritas: 0 };
  if (!previo) return { vivo: construirMotor(model), escritas: -1 };
  const tablas = resolvableTablesOf(model);
  if (firmaDe(model, tablas) !== previo.firma) return { vivo: construirMotor(model), escritas: -1 };

  previo.libro.tablas = tablas;
  const antes = new Map(previo.modelo.sheets.map((s) => [s.name, s.cells]));
  let escritas = 0;
  for (const s of model.sheets) {
    const vieja = antes.get(s.name);
    if (vieja === s.cells) continue; // misma hoja, mismas celdas: ni se mira
    const previas = vieja ?? {};
    const pista = cambios.get(s.cells);
    if (pista && vieja && pista.desde === vieja) {
      for (const ref of pista.refs)
        escritas += aplicarCelda(previo.engine, s.name, ref, s.cells[ref]);
      continue;
    }
    // Lo escrito o cambiado…
    for (const ref in s.cells) {
      if (previas[ref] === s.cells[ref]) continue;
      escritas += aplicarCelda(previo.engine, s.name, ref, s.cells[ref]);
    }
    // …y lo borrado.
    for (const ref in previas) {
      if (!(ref in s.cells)) escritas += aplicarCelda(previo.engine, s.name, ref, undefined);
    }
  }
  previo.modelo = model;
  aplicarOcultas(previo, model);
  return { vivo: previo, escritas };
}
