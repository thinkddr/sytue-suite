/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL MANDO DE LA HOJA: los comandos que la cinta, los paneles y los scripts mandan al
 * lienzo de hoja montado y lo que leen de él, sin el lienzo.
 *
 * Vivía dentro de `SheetCanvas.tsx`, y por eso la cinta, el asistente, los comentarios
 * o las macros de CUALQUIER documento importaban el lienzo de hoja entero (y con él el
 * motor de fórmulas): el editor de Docus bajaba Calcus. Ahora el lienzo se registra
 * aquí al montarse (`lienzoDeHoja`) y lo demás habla con este módulo. `SheetCanvas`
 * reexporta todo, para quien ya lo importaba de allí.
 */
import type { CellValue } from '@sytue/formula';
import type {
  CellStyle,
  CondRule,
  DataValidation,
  DefinedName,
  ExternalTable,
  NumFmt,
  Scenario,
  SheetTable,
  Sparkline,
} from '../types';
import { recordStep } from '../macros';
import type { AnclaCelda } from './anclaCelda.js';
import type { SheetPivot } from './pivot';
import type { SolverEvaluate } from './solver';

/* ---------- Comandos de la pestaña InicioSheets (tarea B11) ---------- */

export type SheetCommand =
  | 'bold'
  | 'italic'
  | 'align'
  | 'numfmt' // value: NumFmtId
  | 'decimals' // value: '+1' | '-1'
  | 'style' // value: JSON.stringify(Partial<CellStyle>) — parche sobre el rango (null = quitar prop)
  | 'border' // value: 'all' | 'outer' | 'bottom' | 'none'
  | 'clearstyle' // quita todo el formato visual del rango
  | 'condadd' // value: JSON.stringify(CondRule)
  | 'conddel' // value: índice de la regla
  | 'condmove' // value: 'i,dir' — reordena la regla i (prioridad); dir -1 sube, +1 baja
  | 'validadd' // value: JSON.stringify(DataValidation) (rango = selección actual)
  | 'validdel' // value: índice de la validación
  | 'merge' // value: 'center' (o ausente) combina y centra; 'merge' combina; 'unmerge' separa; 'content' une el texto en la primera celda (la combinación de contenido de antes)
  | 'copy' // portapapeles: value 'cut' corta (mover); ausente = copiar
  | 'paste' // value 'values' = pegado especial (solo valores)
  /* --- Fase Datos --- */
  | 'freeze' // value: 'cols,rows' ('0,0' descongela)
  | 'sort' // value: 'asc' | 'desc' (por la columna del ancla, con engine.getValue)
  | 'autofilter' // conmuta el autofiltro sobre el rango/tabla del ancla
  | 'filterset' // value: JSON.stringify({ col, value, hidden }) — excluye/incluye un valor
  | 'filtercond' // value: JSON.stringify({ col, cond }) — condición de la columna (cond null la quita)
  | 'filtersort' // value: JSON.stringify({ col, dir }) — ordena el cuerpo del autofiltro
  | 'dedupe' // quita filas duplicadas del rango seleccionado
  | 'text2cols' // value: delimitador — reparte el texto de la columna en varias
  | 'insrow' // value: 'above' | 'below'
  | 'inscol' // value: 'left' | 'right'
  | 'delrow'
  | 'delcol'
  /* --- Fase Visual --- */
  | 'chartadd' // value: JSON.stringify({ type, range }) (id y posición los pone el lienzo)
  | 'sparkadd' // value: JSON.stringify({ range, type, color?, cell? }) — cell por defecto = ancla
  | 'sparkdel' // value: celda anfitriona de la sparkline a quitar
  | 'pivot' // value: JSON de una SheetPivot (sin `name`, una nueva; con él, cambia esa) — la calcula, la escribe y la guarda
  | 'pivotrefresh' // value: nombre de la tabla dinámica, o '' para todas las de la hoja — la recalcula desde su origen
  | 'pivotdel' // value: nombre — vacía lo que ocupaba y la quita
  | 'sliceradd' // value: JSON.stringify({ range, col }) — segmentación sobre una columna (usa/crea el autofiltro)
  | 'aifill' // value: la fórmula del asistente — la escribe en la selección, adaptando referencias
  | 'goalseek' // value: JSON.stringify({ cell, value }) — escribe el valor hallado en la celda variable (Buscar objetivo)
  | 'solverapply' // value: JSON.stringify({ values: { ref: number } }) — escribe los valores hallados en las celdas variable (Solver)
  | 'fillcells' // value: JSON.stringify({ values: { ref: texto } }) — escribe lo que la persona confirmó del relleno con IA
  | 'bloquehoja' // value: JSON de un bloque reutilizable {cells, formats, styles, charts} — lo coloca en la celda activa
  /* --- Escenarios (Administrador de escenarios) --- */
  | 'scenadd' // value: JSON.stringify({ name, range }) — captura los valores actuales de las celdas del rango
  | 'scenupdate' // value: nombre — recaptura los valores actuales de las celdas del escenario
  | 'scenapply' // value: nombre — escribe los valores del escenario en las celdas (la hoja recalcula)
  | 'scendel' // value: nombre — borra el escenario
  | 'scenreport' // value: JSON.stringify({ target, results }) — informe resumen comparando escenarios
  | 'datatable' // value: JSON.stringify({ range, rowInput?, colInput? }) — tabla de datos (sensibilidad)
  /* --- Tablas con formato (ListObject) --- */
  | 'tableadd' // value: JSON.stringify({ range, headers?, style? }) — nombre único automático
  | 'tabledel' // quita la tabla que cubre el ancla
  | 'tablestyle' // value: id de estilo (TABLE_STYLES) de la tabla del ancla
  | 'tabletotals' // conmuta la fila de totales de la tabla del ancla
  | 'tabletotalfn' // value: JSON.stringify({ name, col, fn }) — función de total de una columna
  /* --- Datos externos (conector Power Query): materializar/refrescar --- */
  | 'extdata' // value: JSON { connectionId, spec, anchor, columns, rows, style?, name? } — vuelca el resultado como SheetTable + guarda la definición (name presente = refrescar la existente)
  /* --- Protección de hoja y celda --- */
  /* --- Medidas de columna y fila (sheets/rejilla.ts) --- */
  | 'colwidth' // value: «{px}» o «auto» — ancho de las columnas de la selección
  | 'rowheight' // value: «{px}» o «auto» — alto de las filas de la selección
  | 'protect' // value: JSON.stringify({ password?, selectCells?, formatCells? })
  | 'unprotect' // value: contraseña tecleada (cotejo client-side, no cripto)
  /* --- Protección de estructura del LIBRO --- */
  | 'protectbook' // value: JSON.stringify({ password? }) — bloquea añadir/borrar/renombrar/reordenar hojas
  | 'unprotectbook' // value: contraseña tecleada (cotejo client-side, no cripto)
  /* --- Esquema (agrupar/outline) y subtotales --- */
  | 'outline' // value: JSON { axis:'row'|'col', op:'group'|'ungroup'|'toggle'|'level'|'clear', start?, end?, level? }
  | 'subtotal' // value: JSON { range, groupCol, fn } — inserta subtotales por grupo + total general
  | 'hide' // value: JSON { axis:'row'|'col', op:'hide'|'show' } — oculta o muestra las de la selección
  /* --- Nombres definidos (Administrador de nombres, sheets/nombres.ts) --- */
  | 'nameset' // value: JSON NameRequest — crea o edita (renombrar reescribe las fórmulas que lo usan)
  | 'namedel'; // value: JSON { name, sheet? } — borra el nombre de ese ámbito

/** Envía un comando de celda al SheetCanvas montado (no-op si no hay ninguno).
 *  Si hay grabación de macro activa, registra el comando antes de despacharlo
 *  (recordStep es no-op fuera de grabación: no altera el despacho normal). */
export function dispatchSheetCommand(cmd: SheetCommand, value?: string): void {
  if (!lienzoDeHoja.sheetCommandHandler) return;
  recordStep('spreadsheet', cmd, value);
  lienzoDeHoja.sheetCommandHandler(cmd, value);
}

/**
 * Lo que el lienzo de hoja MONTADO publica: su manejador de comandos y sus lectores.
 * Los rellena `SheetCanvas` al montarse y los vacía al desmontarse; mientras no hay
 * lienzo, todo es `null` y las funciones de aquí no hacen nada (o devuelven `null`).
 * ponytail: un único lienzo montado por documento, como siempre.
 */
export const lienzoDeHoja: {
  sheetCommandHandler: ((cmd: SheetCommand, value?: string) => void) | null;
  sheetStateReader: (() => SheetRibbonState) | null;
  sheetGridReader: ((range: string) => string[][] | null) | null;
  sheetValueReader: ((range: string, sheet?: string) => CellValue[][] | null) | null;
  sheetNameReader: (() => string | null) | null;
  sheetCellAnchorReader: (() => AnclaCelda | null) | null;
  sheetCellRevealer: ((a: AnclaCelda) => boolean) | null;
  sheetCellRefReader: ((a: AnclaCelda) => string | null) | null;
  sheetCellHighlighter: ((a: AnclaCelda | null) => void) | null;
  sheetProbeReader: ((targetRef: string, varRef: string) => GoalSeekProbeResult) | null;
  sheetSolverReader:
    | ((targetRef: string, varRefs: string[], cons: SolverConstraintRef[]) => SolverProbeResult)
    | null;
  readonly sheetStateListeners: Set<() => void>;
} = {
  sheetCommandHandler: null,
  sheetStateReader: null,
  sheetGridReader: null,
  sheetValueReader: null,
  sheetNameReader: null,
  sheetCellAnchorReader: null,
  sheetCellRevealer: null,
  sheetCellRefReader: null,
  sheetCellHighlighter: null,
  sheetProbeReader: null,
  sheetSolverReader: null,
  sheetStateListeners: new Set(),
};

/* ---------- Canal de LECTURA hoja→ribbon (dispatch es solo escritura) ---------- */

/** Tabla de datos externos + el rango ACTUAL de su SheetTable enlazada (para
 *  localizar la que cubre el ancla y para el gestor/refresco del ribbon). */
export interface ExternalTableState extends ExternalTable {
  /** Rango A1 actual de la SheetTable enlazada; '' si ya no existe. */
  range: string;
}

export interface SheetRibbonState {
  /** Formato de número de la celda ancla ({ id: 'general' } si no tiene). */
  numFmt: NumFmt;
  /** Formato visual de la celda ancla ({} si ninguno) — refleja botones del ribbon. */
  style: CellStyle;
  /** Paneles congelados de la hoja activa (0,0 = sin congelar). */
  freeze: { cols: number; rows: number };
  /** Ancho de la columna y alto de la fila del ancla (px de maqueta): prellenan los cuadros
   *  «Ancho de columna…» y «Alto de fila…». */
  colWidth: number;
  rowHeight: number;
  /** Reglas de formato condicional de la hoja activa. */
  condRules: readonly CondRule[];
  /** Validaciones de datos de la hoja activa (la pestaña Datos las lista y elimina). */
  validations: readonly DataValidation[];
  /** Sparklines de la hoja activa (la pestaña Datos las lista y elimina). */
  sparklines: readonly Sparkline[];
  /** Selección actual en A1 («B2» o «A1:C3»), para prellenar el rango de una regla. */
  selection: string;
  /** Autofiltro persistente activo (los desplegables por columna viven en la rejilla). */
  autofilter: boolean;
  /** Tabla con formato que cubre la celda ancla (null si el ancla no está en una). */
  table: SheetTable | null;
  /** Tablas de datos externos de la hoja activa (con el rango actual de su SheetTable
   *  enlazada) para el gestor/refresco del ribbon. */
  externalTables: readonly ExternalTableState[];
  /** La hoja activa está protegida (celdas bloqueadas en solo lectura). */
  protectedSheet: boolean;
  /** La protección de la hoja tiene contraseña (para pedirla al desproteger). */
  protectHasPassword: boolean;
  /** Referencias de los rangos editables por persona o grupo de la hoja protegida. */
  protectRanges: readonly string[];
  /** El LIBRO está protegido: estructura bloqueada (añadir/borrar/renombrar/reordenar hojas). */
  protectedWorkbook: boolean;
  /** La protección del libro tiene contraseña (para pedirla al desproteger). */
  protectWorkbookHasPassword: boolean;
  /** Escenarios del Análisis Y-si de la hoja activa (el Administrador los lista). */
  scenarios: readonly Scenario[];
  /** Tablas dinámicas de la hoja activa. */
  pivots: readonly SheetPivot[];
  /** La tabla dinámica cuya salida cubre la celda ancla, o null. */
  pivotAt: SheetPivot | null;
  /** Nombres definidos del libro, las hojas (su ámbito) y la activa: Administrador de nombres. */
  names: readonly DefinedName[];
  sheetNames: readonly string[];
  activeSheet: string;
}

/** Sonda de Buscar objetivo: función objetivo lista para el solver + valor actual. */
export interface GoalSeekProbe {
  /** f(x) = valor calculado de la celda objetivo al fijar la variable en x;
   *  null si la celda objetivo no da un número (error/texto). */
  f: (x: number) => number | null;
  /** Valor numérico actual de la celda variable (punto de partida del solver). */
  current: number;
}

/** Resultado de preparar la sonda: lista, o el motivo por el que no se puede. */
export type GoalSeekProbeResult =
  | { ok: true; probe: GoalSeekProbe }
  | {
      ok: false;
      reason: 'no-sheet' | 'bad-target' | 'bad-var' | 'target-not-formula' | 'var-is-formula';
    };

/** Una restricción del Solver sin resolver: refs de texto (LHS = celda; RHS =
 *  número en es-ES o celda). El lienzo las resuelve al construir la sonda. */
export interface SolverConstraintRef {
  lhs: string;
  rhs: string;
}

/** Sonda del Solver: evaluador multivariable listo para el optimizador (solver.ts)
 *  + valores actuales de las celdas variable (punto de partida). */
export interface SolverProbe {
  /** evaluate(x) fija las celdas variable en x y lee objetivo + LHS/RHS de cada
   *  restricción (una recalculación del motor desechable). */
  evaluate: SolverEvaluate;
  /** Valores numéricos actuales de las celdas variable, en su orden. */
  current: number[];
}

/** Resultado de preparar la sonda del Solver: lista, o el motivo por el que no. */
export type SolverProbeResult =
  | { ok: true; probe: SolverProbe }
  | {
      ok: false;
      reason:
        | 'no-sheet'
        | 'bad-target'
        | 'target-not-formula'
        | 'no-vars'
        | 'bad-var'
        | 'var-is-formula'
        | 'bad-constraint';
    };

/** Estado actual de la hoja para el ribbon; null si no hay SheetCanvas montado. */
export function readSheetState(): SheetRibbonState | null {
  return lienzoDeHoja.sheetStateReader ? lienzoDeHoja.sheetStateReader() : null;
}

/** Valores mostrados (texto) de un rango de la hoja activa; null si no parsea o
 *  no hay lienzo. Lo usa el asistente de tabla dinámica para leer los encabezados. */
export function readSheetGrid(range: string): string[][] | null {
  return lienzoDeHoja.sheetGridReader ? lienzoDeHoja.sheetGridReader(range) : null;
}

/** Los VALORES calculados de un rango (de la hoja activa, o de `sheet`), sin formato:
 *  lo que necesita la tabla dinámica para agrupar como el cálculo. null sin hoja. */
export function readSheetValues(range: string, sheet?: string): CellValue[][] | null {
  return lienzoDeHoja.sheetValueReader ? lienzoDeHoja.sheetValueReader(range, sheet) : null;
}

/** Nombre de la hoja activa; null sin lienzo. Lo usan los scripts para acotar por ámbito. */
export function readSheetName(): string | null {
  return lienzoDeHoja.sheetNameReader ? lienzoDeHoja.sheetNameReader() : null;
}

/** El ancla de la celda activa (anclaCelda.ts), para colgarle un comentario: «Nuevo
 *  comentario» de la cinta. null sin lienzo de hoja. */
export function readSheetCellAnchor(): AnclaCelda | null {
  return lienzoDeHoja.sheetCellAnchorReader ? lienzoDeHoja.sheetCellAnchorReader() : null;
}

/** Lleva a la celda de un comentario: cambia de hoja si hace falta, la selecciona y la trae
 *  a la vista. false si no hay lienzo de hoja o la celda ya no existe. */
export function revealCommentCell(a: AnclaCelda): boolean {
  return lienzoDeHoja.sheetCellRevealer ? lienzoDeHoja.sheetCellRevealer(a) : false;
}

/** Dónde está HOY la celda de un comentario («B5», o «Hoja2!B5» si es de otra hoja): tras
 *  insertar una fila encima ya no es la que se citó al comentar. null sin lienzo o si ya no
 *  existe. */
export function readCommentCellRef(a: AnclaCelda): string | null {
  return lienzoDeHoja.sheetCellRefReader ? lienzoDeHoja.sheetCellRefReader(a) : null;
}

/** Enmarca en violeta la celda de un comentario mientras el puntero (o el foco) está en su
 *  hilo del panel; null lo apaga. Solo se ve en la hoja activa: no cambia de hoja ni mueve
 *  la selección, que es lo que hace el botón «Celda B5». */
export function highlightCommentCell(a: AnclaCelda | null): void {
  lienzoDeHoja.sheetCellHighlighter?.(a);
}

/** Prepara la sonda de Buscar objetivo para la celda objetivo y la variable (A1
 *  de la hoja activa). Trabaja sobre un motor DESECHABLE clonado del modelo
 *  actual: el solver puede fijar la variable cuantas veces quiera sin tocar la
 *  hoja real. Devuelve el motivo si las celdas no valen (para avisar en el diálogo). */
export function makeGoalSeekProbe(targetRef: string, varRef: string): GoalSeekProbeResult {
  return lienzoDeHoja.sheetProbeReader
    ? lienzoDeHoja.sheetProbeReader(targetRef, varRef)
    : { ok: false, reason: 'no-sheet' };
}

/** Prepara la sonda del Solver (celda objetivo, celdas variable y restricciones,
 *  A1 de la hoja activa) sobre un motor DESECHABLE clonado del modelo actual: el
 *  optimizador fija las variables cuantas veces quiera sin tocar la hoja real.
 *  Devuelve el motivo si algo no vale (para avisar en el diálogo). */
export function makeSolverProbe(
  targetRef: string,
  varRefs: string[],
  cons: SolverConstraintRef[],
): SolverProbeResult {
  return lienzoDeHoja.sheetSolverReader
    ? lienzoDeHoja.sheetSolverReader(targetRef, varRefs, cons)
    : { ok: false, reason: 'no-sheet' };
}

/** Suscribe al cambio de selección/modelo; devuelve la desuscripción. */
export function subscribeSheetState(fn: () => void): () => void {
  lienzoDeHoja.sheetStateListeners.add(fn);
  return () => {
    lienzoDeHoja.sheetStateListeners.delete(fn);
  };
}
