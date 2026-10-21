/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * SheetCanvas — lienzo de hoja de cálculo (kind `spreadsheet`, BLUEPRINT.md §7.1).
 * Barra de fórmulas, rejilla virtualizada (hasta A..XFD × 1..1.048.576, ver
 * ./rejilla) con cabeceras pegajosas, celdas
 * contenteditable (clic = seleccionar; doble clic, Enter o teclear = editar),
 * pestañas de hoja y persistencia serializando SheetModel por el mismo camino
 * que el folio: `ctx.onContentInput(JSON.stringify(model))`.
 *
 * Decisiones fuera del blueprint (tarea B11; documentadas aquí):
 * - La selección usa el acento de marca: en los tokens el latón de
 *   marca vive en --sytue-primary (--sytue-accent es slate); mismo criterio
 *   que StatusBar/Ribbon: `var(--sytue-primary, var(--sytue-accent))`.
 * - La pestaña «Inicio» propia (InicioSheets) opera a nivel de CELDA (negrita,
 *   cursiva, alineación, formato de número, combinar): esos comandos no caben
 *   en exec (§3), así que este fichero expone un mini-registro de comandos
 *   (dispatchSheetCommand) que el lienzo atiende mientras está montado, y un
 *   canal de LECTURA (readSheetState/subscribeSheetState) con el que el ribbon
 *   refleja el estado de la celda ancla (numfmt, freeze, reglas condicionales).
 * - Valor y formato van SEPARADOS: el texto crudo de la celda (literal o
 *   «=fórmula») es la fuente de verdad y el formato de número (formats) solo
 *   afecta al render; editar una celda muestra siempre su texto crudo.
 * - Para que «combinar» signifique algo, la selección admite rango
 *   (Mayús+clic / Mayús+flechas) además de la celda ancla del §7.1.
 * - §7.1 no menciona el zoom en la hoja: se aplica con CSS `zoom` (mantiene
 *   las cabeceras pegajosas, cosa que `transform` rompería).
 */
import { cloneElement, toChildArray } from 'preact';
import type { JSX, TargetedEvent, VNode, ComponentChildren } from 'preact';
import { celdasARellenar } from './aifill.js';
import { conIdsDeHoja, nuevoIdHoja } from './idhoja.js';
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'preact/hooks';
import {
  MenuItem,
  MenuSeparator,
  useToast,
  Checkbox,
  useUltimo,
  useMobileBack,
  saneaHtmlDeDocumento,
} from '@sytue/ui';
import { confirmarEnEditor, preguntarEnEditor } from '@sytue/ui';
import { FormulaError, nombresDeFunciones, numToCol, type Engine } from '@sytue/formula';
import type { CellValue } from '@sytue/formula';
import type {
  CellBorder,
  CellStyle,
  ChartSpec,
  CondRule,
  DataValidation,
  DefinedName,
  EditorCommand,
  EditorContext,
  ExternalTable,
  NumFmt,
  Scenario,
  FilterCondition,
  SheetAutoFilter,
  SheetData,
  SheetGroup,
  SheetModel,
  SheetTable,
  Slicer,
  Sparkline,
  TableTotalFn,
} from '../types';
import {
  gridToQueryTable,
  literalDeDato,
  parseExecuteResult,
  resultToGrid,
  sanitizeExternalTable,
  sanitizeQuerySpec,
  sanitizeTransform,
} from '../dataquery';
import { applyTransform, type QueryResolver } from './querytransform';
import { writePivot, pivotLabels, pivotSource, legacyPivot } from './pivot-hoja';
import { nextPivotName, pivotItems, sanitizePivot, type SheetPivot } from './pivot';
import { setEditorRoot } from '../canvas/exec';
import type { MatchOptions } from '../canvas/exec';
import { recordStep } from '../macros';
import {
  anclarPosHoja,
  cursorPosKey,
  peerName,
  resolverPosHoja,
  sheetPeerBox,
} from '../state/cursors';
import {
  AMBITO_LIBRO,
  ambitoHoja,
  esPropietario,
  presentarClave,
  presentarNueva,
  yo,
} from '../state/proteccion';
import { CONTENT_PALETTE, RibbonButton } from '../ribbon/primitives';
import { EditorIcon } from '../icons';
import {
  NUMFMT_IDS,
  applyNumFmt,
  buildCondDecor,
  canEditCell,
  canEditCellContent,
  estiloParaMi,
  canEditStructure,
  cellStyleCss,
  classifyConstraintRhs,
  compileValidations,
  dataBarBackground,
  dedupeRowIndices,
  defaultDecimals,
  escapeHtml,
  formatCellValue,
  iconSetSvg,
  isFormula,
  isFormulaHidden,
  isPermissionGranted,
  mergeCellStyle,
  parseCell,
  parseRangeRef,
  parseTsv,
  rawText,
  renderCellHtml,
  sanitizeCellStyle,
  sanitizeChartSpec,
  sanitizeCondRule,
  sanitizeDataValidation,
  sanitizeNumFmt,
  sanitizeProtection,
  sanitizeSparkline,
  sanitizeWorkbookProtection,
  selectionStats,
  serializeTsv,
  shiftCellHtml,
  shiftRefs,
  singleCellRef,
  sortCellCompare,
  sparklineSvg,
  stripHtml,
  validateValue,
  validationAt,
  validationMessage,
} from './format';
import { moverObjetos, reescribirCelda, type Desplazamiento } from './refshift';
import {
  applyNameCommand,
  moverNombres,
  namesVisibleFrom,
  quitarHojaDeNombres,
  renombrarHojaEnNombres,
  sanitizeNames,
  withNames,
} from './nombres';
import {
  anotarCambio,
  cellText,
  construirMotor,
  filteredRows,
  resolvableTablesOf,
  sincronizarMotor,
  type MotorVivo,
} from './motor-vivo';
import {
  activeView,
  sanitizeFilterCond,
  sanitizeFilterConds,
  setCond,
  setExcluded,
  toggleValue,
  viewFromFilter,
  viewWithCriterios,
  withCriterios,
  withView,
} from './filtro';
import { FiltroCondicion } from './FiltroCondicion';
import { FiltroVistasBarra, useFiltroVistas } from './FiltroVistas';
import {
  MAX_COLS,
  MAX_ROWS,
  VISTA_PASO_COLS,
  VISTA_PASO_FILAS,
  ALTO_MAX,
  acumulado,
  cuantiza,
  extension,
  indiceEn,
  indiceVisible,
  medidaDe,
  primeroDesde,
  saltoADatos,
  tramo,
  tramoPorPosiciones,
  type Vista,
} from './rejilla';
import { conEstructura } from './anclas';
import { fusionEn, fusionesEnVentana, moverPorFusiones, parseFusiones } from './fusiones';
import {
  anotar,
  deshacer as deshacerHistorial,
  nuevoHistorial,
  rehacer as rehacerHistorial,
  type Historial,
} from '@sytue/ui';
import type { CellDataBar, CellDecor, SelectionStats, SheetPermission } from './format';
import { formatWithMask } from './numfmt-mask';
import {
  HIT_ACTIVE_BG,
  HIT_BG,
  HIT_INK,
  hitCells,
  nextFrom,
  replaceAllPlan,
  sameMatch,
  setSearchTarget,
  sheetMatches,
  splice,
  withText,
} from './search';
import type { SheetMatch } from './search';
import {
  compileTable,
  autofilterBody,
  compileTables,
  expandRangeForEdit,
  resolveStructuredRef,
  sanitizeTable,
  tableCellInfo,
  tableStyleOf,
  tableTotalValue,
  uniqueHeaderNames,
  uniqueTableName,
} from './table';
import type { CompiledTable } from './table';
import {
  addGroup,
  applySubtotals,
  groupsAtLevel,
  hiddenUnits,
  maxOutlineLevel,
  removeGroup,
  sanitizeGroups,
  toggleGroup,
} from './outline';
import {
  argHintAt,
  cycleRefAt,
  expectsRef,
  extractRefs,
  funcTokenAt,
  refAtCaret,
  tableBracketAt,
} from './formula-edit';
import type { FormulaRef, StructuredResolver } from './formula-edit';
import { lookupSignature, splitSignature } from './formula-catalog';
import { SheetChart } from './Chart';
import { SheetSlicer } from './SlicerPanel';
import {
  excludedFromSelection,
  pivotSlicerRef,
  pivotOnSlicerRemoval,
  pivotWithHidden,
  sanitizeSlicer,
  slicerEntries,
} from './slicer';
import type { SlicerEntry } from './slicer';
import {
  SCENARIO_MAX_CELLS,
  applyScenarioValues,
  buildScenarioReport,
  sanitizeScenario,
} from './scenario';
import { colocarBloque } from './bloque';
import { computeDataTable, planDataTable } from './datatable';
import type { DataTableError, DataTableSubs } from './datatable';
import type { SolverEvaluate } from './solver';
import { useT } from '../context';
import { editorT } from '../../i18n';
import type { DocsKey } from '../../i18n';
import { comandoDelAtajo } from '../comandos';
import '../canvas/superficies.css';
import './superficie-hoja.css';

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
  | 'merge'
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
  | 'protect' // value: JSON.stringify({ password?, selectCells?, formatCells? })
  | 'unprotect' // value: contraseña tecleada (cotejo client-side, no cripto)
  /* --- Protección de estructura del LIBRO --- */
  | 'protectbook' // value: JSON.stringify({ password? }) — bloquea añadir/borrar/renombrar/reordenar hojas
  | 'unprotectbook' // value: contraseña tecleada (cotejo client-side, no cripto)
  /* --- Esquema (agrupar/outline) y subtotales --- */
  | 'outline' // value: JSON { axis:'row'|'col', op:'group'|'ungroup'|'toggle'|'level'|'clear', start?, end?, level? }
  | 'subtotal' // value: JSON { range, groupCol, fn } — inserta subtotales por grupo + total general
  /* --- Nombres definidos (Administrador de nombres, sheets/nombres.ts) --- */
  | 'nameset' // value: JSON NameRequest — crea o edita (renombrar reescribe las fórmulas que lo usan)
  | 'namedel'; // value: JSON { name, sheet? } — borra el nombre de ese ámbito

let sheetCommandHandler: ((cmd: SheetCommand, value?: string) => void) | null = null;

/** Envía un comando de celda al SheetCanvas montado (no-op si no hay ninguno).
 *  Si hay grabación de macro activa, registra el comando antes de despacharlo
 *  (recordStep es no-op fuera de grabación: no altera el despacho normal). */
export function dispatchSheetCommand(cmd: SheetCommand, value?: string): void {
  if (!sheetCommandHandler) return;
  recordStep('spreadsheet', cmd, value);
  sheetCommandHandler(cmd, value);
}

/** Comandos que EDITAN contenido/estructura de celdas: vetados con la hoja
 *  protegida, sin excepción. (Las anotaciones/vistas de hoja —filtros, reglas,
 *  gráficos, sparklines y el ESQUEMA `outline` (agrupar/contraer): reordenan la
 *  VISTA, no editan celdas bloqueadas— quedan permitidas; ponytail: vetarlas también
 *  es un follow-up si se pide protección total al estilo Excel. `subtotal`, en
 *  cambio, ESCRIBE celdas de total, por eso SÍ figura aquí y queda vetado.) `tableadd`
 *  reescribe las celdas de cabecera (nombres únicos), así que SÍ se veta —como
 *  Excel, que deshabilita insertar tabla en hoja protegida—; y el resto de ops de
 *  tabla (estilo, totales, borrar) también: aunque no toquen celdas cambian `tables`,
 *  y el servicio no deja cambiarlo con la hoja protegida (proteccion.go). */
export const PROTECT_CONTENT_CMDS: ReadonlySet<SheetCommand> = new Set<SheetCommand>([
  'paste',
  'dedupe',
  'text2cols',
  'merge',
  'pivot',
  'pivotrefresh',
  'pivotdel',
  'tableadd',
  'tabledel',
  'tablestyle',
  'tabletotals',
  'tabletotalfn',
  'extdata',
  'aifill',
  'goalseek',
  'solverapply',
  'fillcells',
  'bloquehoja',
  'subtotal',
  'scenapply',
  'scenreport',
  'datatable',
]);
/** Comandos de FORMATO de celda: con la hoja protegida solo si `formatCells`. */
export const PROTECT_FORMAT_CMDS: ReadonlySet<SheetCommand> = new Set<SheetCommand>([
  'numfmt',
  'decimals',
  'style',
  'border',
  'clearstyle',
  'bold',
  'italic',
  'align',
]);
/** Comando → permiso granular del cuadro «Proteger hoja» de Excel: con la hoja
 *  protegida solo pasa si ese permiso está CONCEDIDO (ausente = false). Los que
 *  ESCRIBEN celdas sin permiso equivalente en Excel (pegar, quitar duplicados,
 *  pivot, subtotales…) siguen vetados EN BLOQUE por PROTECT_CONTENT_CMDS. */
export const PROTECT_PERMISSION_CMDS: ReadonlyMap<SheetCommand, SheetPermission> = new Map<
  SheetCommand,
  SheetPermission
>([
  ['insrow', 'insertRows'],
  ['delrow', 'deleteRows'],
  ['inscol', 'insertCols'],
  ['delcol', 'deleteCols'],
  ['sort', 'sort'],
  ['autofilter', 'autoFilter'],
  ['filterset', 'autoFilter'],
  ['filtercond', 'autoFilter'],
  ['filtersort', 'sort'],
  ['condadd', 'objects'],
  ['conddel', 'objects'],
  ['condmove', 'objects'],
  ['validadd', 'objects'],
  ['validdel', 'objects'],
  ['chartadd', 'objects'],
  ['sparkadd', 'objects'],
  ['sparkdel', 'objects'],
  ['sliceradd', 'objects'],
  ['scenadd', 'scenarios'],
  ['scenupdate', 'scenarios'],
  ['scendel', 'scenarios'],
]);

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

let sheetStateReader: (() => SheetRibbonState) | null = null;
let sheetGridReader: ((range: string) => string[][] | null) | null = null;
let sheetValueReader: ((range: string, sheet?: string) => CellValue[][] | null) | null = null;
let sheetNameReader: (() => string | null) | null = null;
let sheetProbeReader: ((targetRef: string, varRef: string) => GoalSeekProbeResult) | null = null;
let sheetSolverReader:
  | ((targetRef: string, varRefs: string[], cons: SolverConstraintRef[]) => SolverProbeResult)
  | null = null;
const sheetStateListeners = new Set<() => void>();

/** Estado actual de la hoja para el ribbon; null si no hay SheetCanvas montado. */
export function readSheetState(): SheetRibbonState | null {
  return sheetStateReader ? sheetStateReader() : null;
}

/** Valores mostrados (texto) de un rango de la hoja activa; null si no parsea o
 *  no hay lienzo. Lo usa el asistente de tabla dinámica para leer los encabezados. */
export function readSheetGrid(range: string): string[][] | null {
  return sheetGridReader ? sheetGridReader(range) : null;
}

/** Los VALORES calculados de un rango (de la hoja activa, o de `sheet`), sin formato:
 *  lo que necesita la tabla dinámica para agrupar como el cálculo. null sin hoja. */
export function readSheetValues(range: string, sheet?: string): CellValue[][] | null {
  return sheetValueReader ? sheetValueReader(range, sheet) : null;
}

/** Nombre de la hoja activa; null sin lienzo. Lo usan los scripts para acotar por ámbito. */
export function readSheetName(): string | null {
  return sheetNameReader ? sheetNameReader() : null;
}

/** Prepara la sonda de Buscar objetivo para la celda objetivo y la variable (A1
 *  de la hoja activa). Trabaja sobre un motor DESECHABLE clonado del modelo
 *  actual: el solver puede fijar la variable cuantas veces quiera sin tocar la
 *  hoja real. Devuelve el motivo si las celdas no valen (para avisar en el diálogo). */
export function makeGoalSeekProbe(targetRef: string, varRef: string): GoalSeekProbeResult {
  return sheetProbeReader ? sheetProbeReader(targetRef, varRef) : { ok: false, reason: 'no-sheet' };
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
  return sheetSolverReader
    ? sheetSolverReader(targetRef, varRefs, cons)
    : { ok: false, reason: 'no-sheet' };
}

/** Suscribe al cambio de selección/modelo; devuelve la desuscripción. */
export function subscribeSheetState(fn: () => void): () => void {
  sheetStateListeners.add(fn);
  return () => {
    sheetStateListeners.delete(fn);
  };
}

/* ---------- Constantes ---------- */

/* El tamaño de la hoja ya no es una constante: los límites (los de Excel), la extensión
 * que se recorre y la ventana que se pinta viven en ./rejilla. */

/** Decoración vacía compartida (reglas ausentes): evita crear un Map por render. */
const EMPTY_DECOR: Map<string, CellDecor> = new Map();

/** Ref A1 («B2») de una celda por índices 0-based. */
function A1(c: number, r: number): string {
  return `${numToCol(c)}${r + 1}`;
}

/** ¿El valor de una regla de fórmula dispara? TRUE o número no nulo (criterio Excel). */
function isTruthy(v: CellValue): boolean {
  if (typeof v === 'boolean') return v;
  if (typeof v === 'number') return v !== 0;
  return false;
}

/** Geometría fija de la rejilla, en PÍXELES. La usan los offsets sticky de
 *  congelar paneles, el asa de autorelleno y todas las capas superpuestas.
 *
 *  Y las clases de la rejilla se escriben en px por ESTO: antes eran `w-10`,
 *  `w-24` y `h-6`, que son rem. Con la base del navegador en 14 px —un ajuste de
 *  accesibilidad corriente— `w-10` mide 35 px mientras esta constante sigue
 *  diciendo 40, y las dos capas se separan 5 px por columna. Si se cambia un
 *  número de aquí, hay que cambiar su clase `w-[…px]`/`h-[…px]` a la vez. */
const HEAD_W = 40;
const CELL_W = 96;
const CELL_H = 24;

/** Grosor de un carril del canalón de esquema (un carril por nivel de outline). */
const OUTLINE_STEP = 14;
/** Aire extra del canalón (deja sitio al botón +/− del carril más profundo). */
const OUTLINE_PAD = 5;

/** Azul de selección (canvas/superficies.css): lo que se señala está sobre papel blanco en los dos temas. */
const ACCENT = 'var(--editor-select, var(--sytue-primary-500))';
/** Tinta del contenido sobre papel blanco (negro de marca de CONTENT_PALETTE, §5). */
const INK = CONTENT_PALETTE[0];
/** Rojo de marca (CONTENT_PALETTE, §5) para marcar valores fuera de la validación. */
const DANGER = CONTENT_PALETTE[5];

const SEL_STYLE = { boxShadow: `inset 0 0 0 2px ${ACCENT}` };

/** Estilo de una celda FUSIONANDO el fill condicional, la barra de datos y la
 *  selección: el ancla marca con boxShadow (compatible con fondo), el rango tiñe
 *  el fill con color-mix, y la barra de datos va como `background-image` (bordes
 *  duros, sin degradado visible) por encima del color de relleno. */
function cellStyle(
  isSel: boolean,
  inRange: boolean,
  fill: string | null,
  bar?: CellDataBar,
): Record<string, string> | undefined {
  const out: Record<string, string> = {};
  if (bar) out.backgroundImage = dataBarBackground(bar);
  if (isSel) {
    Object.assign(out, SEL_STYLE);
    if (fill !== null) out.backgroundColor = fill;
  } else if (inRange) {
    out.backgroundColor =
      fill !== null
        ? `color-mix(in srgb, ${ACCENT} 12%, ${fill})`
        : `color-mix(in srgb, ${ACCENT} 12%, var(--editor-page-bg, white))`;
  } else if (fill !== null) {
    out.backgroundColor = fill;
  }
  return Object.keys(out).length > 0 ? out : undefined;
}

// El anillo canónico vive en `@sytue/ui/styles.css` (.sytue-focus-ring): una sola
// definición para toda la suite, en vez de la copia que cada fichero perdía a su manera.
const FOCUS_RING = 'sytue-focus-ring';

/** `id` DOM de una celda, para que la rejilla pueda apuntarla con aria-activedescendant.
 *  Con prefijo porque «A1» a secas es un identificador demasiado goloso para colisionar. */
const cellDomId = (ref: string): string => `sytue-cell-${ref}`;

/* ---------- UX de edición de fórmulas (autocompletado, hints, refs, F4, color) ----------
 * Nombres de función EN VIVO del registro del motor (cobertura completa; el
 * registro es estático, se construye una vez). El resto de la lógica de texto/
 * cursor es pura y vive en ./formula-edit (con tests). */
const FN_NAMES: readonly string[] = nombresDeFunciones();

/** Un color por referencia distinta de una fórmula; se cicla cada seis.
 *
 *  Salen de tokens (§8) y no de la paleta de CONTENIDO: esto es feedback de la interfaz
 *  —qué trozo de la fórmula es qué marco de la rejilla—, no un color que alguien eligió
 *  para su documento. Con los de contenido, tres de los seis quedaban por debajo de AA
 *  sobre el papel (3,19-3,74:1) y cuatro sobre la barra fx en tema oscuro. */
const REF_COUNT = 6;
const refColor = (i: number): string => `var(--editor-ref-${(i % REF_COUNT) + 1})`;
/** La misma referencia, para la barra fx: vive en el cromo, que sí cambia con el tema. */
const refFxColor = (i: number): string => `var(--editor-ref-fx-${(i % REF_COUNT) + 1})`;

/** Sugerencia del autocompletado de fórmula: función, nombre definido, tabla o columna. */
interface AcItem {
  label: string;
  kind: 'fn' | 'name' | 'table' | 'column';
}
/** Estado del autocompletado bajo el cursor: qué ofrecer y qué tramo sustituir. */
interface AcState {
  items: AcItem[];
  /** Inicio del token a sustituir al aceptar. */
  start: number;
  /** Fin del token a sustituir (= cursor). */
  end: number;
}

/** HTML del texto de una fórmula con cada referencia envuelta en un <span> de su
 *  color (el mismo que el marco sobre la rejilla). El resto va escapado.
 *
 *  `color` distingue las dos superficies: la celda se edita sobre el papel y la barra fx
 *  vive en el cromo, que en tema oscuro es casi negro. */
function tintedFormulaHtml(
  text: string,
  refs: readonly FormulaRef[],
  color: (i: number) => string = refColor,
): string {
  let out = '';
  let pos = 0;
  for (const r of refs) {
    if (r.start < pos) continue; // solaparía (defensivo)
    out += escapeHtml(text.slice(pos, r.start));
    out += `<span style="color:${color(r.colorIndex)}">${escapeHtml(r.text)}</span>`;
    pos = r.end;
  }
  return out + escapeHtml(text.slice(pos));
}

/** Offset de texto del cursor dentro de un contenteditable; null si el cursor no
 *  está en él. Cuenta el texto visible (los <span> de tinte no alteran offsets). */
function caretOffsetIn(el: HTMLElement): number | null {
  const s = window.getSelection();
  if (!s || s.rangeCount === 0) return null;
  const range = s.getRangeAt(0);
  if (!el.contains(range.startContainer)) return null;
  const pre = range.cloneRange();
  pre.selectNodeContents(el);
  pre.setEnd(range.startContainer, range.startOffset);
  return pre.toString().length;
}

/** Coloca el cursor en un offset de texto de un contenteditable (recorre los
 *  nodos de texto sumando longitudes; al final si se pasa). */
function setCaretOffsetIn(el: HTMLElement, offset: number): void {
  const s = window.getSelection();
  if (!s) return;
  const range = document.createRange();
  let remaining = offset;
  const walk = (node: Node): boolean => {
    if (node.nodeType === Node.TEXT_NODE) {
      const len = node.textContent?.length ?? 0;
      if (remaining <= len) {
        range.setStart(node, remaining);
        return true;
      }
      remaining -= len;
      return false;
    }
    for (const child of Array.from(node.childNodes)) if (walk(child)) return true;
    return false;
  };
  if (walk(el)) range.collapse(true);
  else {
    range.selectNodeContents(el);
    range.collapse(false);
  }
  s.removeAllRanges();
  s.addRange(range);
}

/* ---------- Modelo y utilidades puras ---------- */

type Pos = { c: number; r: number };
type Rect = { c1: number; c2: number; r1: number; r2: number };
/** Fórmula en edición (celda o barra fx) y posición del cursor. */
type FxEditState = { surface: 'cell' | 'bar'; text: string; caret: number };

function clamp(n: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, n));
}

function colName(c: number): string {
  return numToCol(c);
}

function refOf(pos: Pos): string {
  return `${colName(pos.c)}${pos.r + 1}`;
}

function rectOf(sel: Pos, ext: Pos | null): Rect {
  const e = ext ?? sel;
  return {
    c1: Math.min(sel.c, e.c),
    c2: Math.max(sel.c, e.c),
    r1: Math.min(sel.r, e.r),
    r2: Math.max(sel.r, e.r),
  };
}

function emptyModel(): SheetModel {
  return {
    active: 0,
    sheets: conIdsDeHoja([{ name: editorT()('sheet.defaultName', { n: 1 }), cells: {} }]),
  };
}

/* ---------- Motor de fórmulas (@sytue/formula) ----------
 * El formato de celda (envoltorio HTML canónico alineación→negrita→cursiva) y
 * los helpers puros (escapeHtml/stripHtml/parseCell/renderCellHtml/rawText/
 * isFormula/shiftCellHtml) viven en ./format (compartidos con el pegado).
 */

/* El motor (construirlo entero, ponerlo al día con la diferencia de un modelo al
 * siguiente y las filas que oculta el filtro) vive en ./motor-vivo. */

/** Un motor nuevo y completo para un modelo hipotético (sondas de Buscar objetivo,
 *  ordenar, pivots…): no se conserva, así que no hace falta sincronizarlo. */
function buildEngine(model: SheetModel): Engine {
  return construirMotor(model).engine;
}

/* ---------- Autofiltro persistente (pestaña Datos) ---------- */

/* ---------- Saneado de carga ---------- */

/** Hoja persistida → saneada: conserva los campos opcionales (formats,
 *  condFormats, freeze, charts) descartando basura; los docs viejos
 *  (solo name+cells) cargan exactamente igual que antes. */
function sanitizeSheet(s: SheetData, i: number): SheetData {
  const out: SheetData = {
    ...(typeof s.id === 'string' && s.id !== '' ? { id: s.id } : {}),
    name:
      typeof s.name === 'string' && s.name !== ''
        ? s.name
        : editorT()('sheet.defaultName', { n: i + 1 }),
    cells: s.cells && typeof s.cells === 'object' ? s.cells : {},
  };
  if (typeof s.tabColor === 'string' && /^#[0-9a-f]{6}$/i.test(s.tabColor))
    out.tabColor = s.tabColor;
  if (s.formats && typeof s.formats === 'object') {
    const formats: Record<string, NumFmt> = {};
    for (const ref in s.formats) {
      const f = sanitizeNumFmt(s.formats[ref]);
      if (f) formats[ref] = f;
    }
    if (Object.keys(formats).length > 0) out.formats = formats;
  }
  if (s.styles && typeof s.styles === 'object') {
    const styles: Record<string, CellStyle> = {};
    for (const ref in s.styles) {
      const st = sanitizeCellStyle(s.styles[ref]);
      if (st) styles[ref] = st;
    }
    if (Object.keys(styles).length > 0) out.styles = styles;
  }
  if (Array.isArray(s.condFormats)) {
    const rules: CondRule[] = [];
    for (const r of s.condFormats) {
      const rule = sanitizeCondRule(r);
      if (rule) rules.push(rule);
    }
    if (rules.length > 0) out.condFormats = rules;
  }
  if (Array.isArray(s.validations)) {
    const vals: DataValidation[] = [];
    for (const v of s.validations) {
      const val = sanitizeDataValidation(v);
      if (val) vals.push(val);
    }
    if (vals.length > 0) out.validations = vals;
  }
  if (Array.isArray(s.sparklines)) {
    const sparks: Sparkline[] = [];
    for (const sp of s.sparklines) {
      const x = sanitizeSparkline(sp);
      if (x) sparks.push(x);
    }
    if (sparks.length > 0) out.sparklines = sparks;
  }
  if (s.freeze && typeof s.freeze === 'object') {
    out.freeze = {
      cols: clamp(Math.trunc(Number(s.freeze.cols) || 0), 0, MAX_COLS - 1),
      rows: clamp(Math.trunc(Number(s.freeze.rows) || 0), 0, MAX_ROWS - 1),
    };
  }
  if (s.autofilter && typeof s.autofilter === 'object') {
    const af = s.autofilter;
    const c1 = clamp(Math.trunc(Number(af.c1) || 0), 0, MAX_COLS - 1);
    const c2 = clamp(Math.trunc(Number(af.c2) || 0), c1, MAX_COLS - 1);
    const row = clamp(Math.trunc(Number(af.row) || 0), 0, MAX_ROWS - 1);
    const out2: SheetAutoFilter = { row, c1, c2 };
    if (af.excl && typeof af.excl === 'object') {
      const excl: Record<string, string[]> = {};
      for (const key in af.excl) {
        const c = Number(key);
        const vals = af.excl[key];
        if (Number.isInteger(c) && c >= c1 && c <= c2 && Array.isArray(vals)) {
          const list = vals.filter((v): v is string => typeof v === 'string');
          if (list.length > 0) excl[String(c)] = list;
        }
      }
      if (Object.keys(excl).length > 0) out2.excl = excl;
    }
    const cond = sanitizeFilterConds(af.cond, c1, c2);
    if (cond) out2.cond = cond;
    out.autofilter = out2;
  }
  if (Array.isArray(s.tables)) {
    const tables: SheetTable[] = [];
    for (const t of s.tables) {
      const tb = sanitizeTable(t);
      if (tb) tables.push(tb);
    }
    if (tables.length > 0) out.tables = tables;
  }
  if (Array.isArray(s.externalTables)) {
    const ext: ExternalTable[] = [];
    for (const e of s.externalTables) {
      const et = sanitizeExternalTable(e);
      if (et) ext.push(et);
    }
    if (ext.length > 0) out.externalTables = ext;
  }
  if (Array.isArray(s.charts)) {
    const charts: ChartSpec[] = [];
    for (const ch of s.charts) {
      const spec = sanitizeChartSpec(ch);
      if (spec) charts.push(spec);
    }
    if (charts.length > 0) out.charts = charts;
  }
  if (Array.isArray(s.slicers)) {
    const slicers: Slicer[] = [];
    for (const sl of s.slicers) {
      const x = sanitizeSlicer(sl);
      if (x) slicers.push(x);
    }
    if (slicers.length > 0) out.slicers = slicers;
  }
  if (s.protect) {
    const prot = sanitizeProtection(s.protect);
    if (prot) out.protect = prot;
  }
  if (Array.isArray(s.pivots)) {
    const pivots: SheetPivot[] = [];
    for (const raw of s.pivots) {
      const pv = sanitizePivot(raw);
      if (pv && !pivots.some((x) => x.name === pv.name)) pivots.push(pv);
    }
    if (pivots.length > 0) out.pivots = pivots;
  }
  const rowGroups = sanitizeGroups(s.rowGroups, MAX_ROWS);
  if (rowGroups.length > 0) out.rowGroups = rowGroups;
  const colGroups = sanitizeGroups(s.colGroups, MAX_COLS);
  if (colGroups.length > 0) out.colGroups = colGroups;
  if (Array.isArray(s.scenarios)) {
    const scenarios: Scenario[] = [];
    const names = new Set<string>();
    for (const sc of s.scenarios) {
      const x = sanitizeScenario(sc);
      if (x && !names.has(x.name)) {
        names.add(x.name);
        scenarios.push(x);
      }
    }
    if (scenarios.length > 0) out.scenarios = scenarios;
  }
  return out;
}

/* ---------- Portapapeles (Ctrl+C/X/V; TSV + HTML de alta fidelidad) ---------- */

/** Celda de un pegado de alta fidelidad (formato interno). */
interface ClipCell {
  raw: string; // HTML crudo de la celda (fórmula/literal envuelta), como en cells[ref]
  fmt: NumFmt | null;
  style: CellStyle | null;
  display: string; // valor mostrado (para el pegado especial de solo valores)
}
/** Contenido del portapapeles ya interpretado. */
type ClipPayload =
  { kind: 'sytue'; c0: number; r0: number; grid: ClipCell[][] } | { kind: 'tsv'; grid: string[][] };

/** Inner HTML de una celda numérica: una máscara personalizada con [Color]
 *  (p. ej. negativos en rojo) envuelve el texto en un <span> con color de un
 *  conjunto FIJO (sin inyección); el resto pasa por formatCellValue. */
function numFmtInnerHtml(v: CellValue, fmt: NumFmt | undefined): string {
  if (fmt?.id === 'custom' && fmt.mask && typeof v === 'number' && Number.isFinite(v)) {
    const m = formatWithMask(fmt.mask, v);
    const t = escapeHtml(m.text);
    return m.color ? `<span style="color:${m.color}">${t}</span>` : t;
  }
  return escapeHtml(formatCellValue(v, fmt));
}

/** Valor mostrado (texto plano) de una celda, espejo de cellHtml sin DOM. */
function clipDisplay(s: SheetData, engine: Engine, c: number, r: number): string {
  const ref = refOf({ c, r });
  const raw = s.cells[ref] ?? '';
  const fmt = s.formats?.[ref];
  const v = engine.getValue(s.name, ref);
  if (isFormula(raw)) return formatCellValue(v, fmt);
  if (fmt && fmt.id !== 'general' && typeof v === 'number') return applyNumFmt(fmt, v);
  return stripHtml(raw);
}

/** Rango → { tsv, html }: TSV de valores (interop) + tabla de alta fidelidad
 *  con data-raw/data-fmt/data-style para el pegado interno. Puro (sin DOM). */
function buildClipboardPayload(
  s: SheetData,
  rect: Rect,
  engine: Engine,
): { tsv: string; html: string } {
  const displayRows: string[][] = [];
  let html = `<table data-sytue-sheet="1" data-c0="${rect.c1}" data-r0="${rect.r1}"><tbody>`;
  for (let r = rect.r1; r <= rect.r2; r++) {
    const drow: string[] = [];
    html += '<tr>';
    for (let c = rect.c1; c <= rect.c2; c++) {
      const ref = refOf({ c, r });
      const display = clipDisplay(s, engine, c, r);
      drow.push(display);
      // Fórmula OCULTA de una hoja protegida: se copia su VALOR, nunca el «=…».
      // La celda y la barra fx ya la escondían, pero el `data-raw` del
      // portapapeles la sacaba entera con un Ctrl+C.
      const stored = s.cells[ref] ?? '';
      const raw =
        isFormulaHidden(!!s.protect, s.styles?.[ref]) && isFormula(stored)
          ? escapeHtml(display)
          : stored;
      const fmt = s.formats?.[ref];
      const style = s.styles?.[ref];
      const attrs =
        ` data-raw="${escapeHtml(raw)}"` +
        (fmt ? ` data-fmt="${escapeHtml(JSON.stringify(fmt))}"` : '') +
        (style ? ` data-style="${escapeHtml(JSON.stringify(style))}"` : '');
      html += `<td${attrs}>${escapeHtml(display)}</td>`;
    }
    html += '</tr>';
    displayRows.push(drow);
  }
  html += '</tbody></table>';
  return { tsv: serializeTsv(displayRows), html };
}

function safeJson(text: string | null): unknown {
  if (!text) return null;
  try {
    return JSON.parse(text);
  } catch {
    return null;
  }
}

/** HTML del portapapeles → ClipPayload si es nuestra tabla de alta fidelidad; null si no. */
function parseSytueHtml(html: string): ClipPayload | null {
  if (typeof DOMParser === 'undefined' || !html.includes('data-sytue-sheet')) return null;
  const doc = new DOMParser().parseFromString(html, 'text/html');
  const table = doc.querySelector('table[data-sytue-sheet]');
  if (!table) return null;
  const c0 = Math.max(0, Math.trunc(Number(table.getAttribute('data-c0')) || 0));
  const r0 = Math.max(0, Math.trunc(Number(table.getAttribute('data-r0')) || 0));
  const grid: ClipCell[][] = [];
  for (const tr of Array.from(table.querySelectorAll('tr'))) {
    const row: ClipCell[] = [];
    for (const td of Array.from(tr.querySelectorAll('td'))) {
      row.push({
        raw: td.getAttribute('data-raw') ?? '',
        fmt: sanitizeNumFmt(safeJson(td.getAttribute('data-fmt'))),
        style: sanitizeCellStyle(safeJson(td.getAttribute('data-style'))),
        display: td.textContent ?? '',
      });
    }
    if (row.length > 0) grid.push(row);
  }
  return grid.length > 0 ? { kind: 'sytue', c0, r0, grid } : null;
}

/** Escribe TSV + HTML en el portapapeles (con reserva a writeText). */
async function writeClipboard(tsv: string, html: string): Promise<void> {
  const nav = navigator.clipboard;
  if (nav && 'write' in nav && typeof ClipboardItem !== 'undefined') {
    await nav.write([
      new ClipboardItem({
        'text/plain': new Blob([tsv], { type: 'text/plain' }),
        'text/html': new Blob([html], { type: 'text/html' }),
      }),
    ]);
    return;
  }
  if (nav?.writeText) {
    await nav.writeText(tsv);
    return;
  }
  throw new Error('sin portapapeles');
}

/** Lee el portapapeles: prioriza nuestra tabla (fidelidad), si no TSV de texto. */
async function readClipboard(): Promise<ClipPayload | null> {
  const nav = navigator.clipboard;
  if (nav && 'read' in nav) {
    try {
      for (const it of await nav.read()) {
        if (it.types.includes('text/html')) {
          const parsed = parseSytueHtml(await (await it.getType('text/html')).text());
          if (parsed) return parsed;
        }
      }
      for (const it of await nav.read()) {
        if (it.types.includes('text/plain')) {
          return { kind: 'tsv', grid: parseTsv(await (await it.getType('text/plain')).text()) };
        }
      }
    } catch {
      /* algunos navegadores no permiten read(); se prueba readText */
    }
  }
  if (nav?.readText) return { kind: 'tsv', grid: parseTsv(await nav.readText()) };
  return null;
}

/* ---------- Componente ---------- */

/** El reloj, para el antirebote de `notifyProtected`. Aparte porque ese manejador viaja
 *  dentro de `commandActionsRef`, que se arma en el render, y `react(purity)` da por
 *  hecho que lo que se pasa a un hook durante el render puede ejecutarse ahí; nunca
 *  se ejecuta: solo lo llaman los comandos y los manejadores de teclado. */
const ahoraMs = (): number => Date.now();

export function SheetCanvas({ ctx }: { ctx: EditorContext }): JSX.Element {
  const t = useT();
  const { notify } = useToast();
  const gridRef = useRef<HTMLDivElement>(null);
  /** Contenedor con scroll de la rejilla: da la altura visible a Re Pág/Av Pág. */
  const scrollRef = useRef<HTMLDivElement>(null);
  /** Arrastre de SELECCIÓN en curso: por celdas, por cabeceras de columna o de
   *  fila. null = ninguno (el arrastre de point-and-click de fórmulas es otro). */
  const selDragRef = useRef<'cell' | 'col' | 'row' | null>(null);
  const [model, setModel] = useState<SheetModel>(emptyModel);
  const [sel, setSel] = useState<Pos>({ c: 0, r: 0 });
  const [ext, setExt] = useState<Pos | null>(null);
  const [editing, setEditing] = useState(false);
  const [fx, setFx] = useState('');
  /** Rango destino del autorelleno mientras se arrastra el asa (previsualización). */
  const [fillRect, setFillRect] = useState<Rect | null>(null);
  const fillDragRef = useRef<{ src: Rect; rect: Rect | null } | null>(null);
  /** Desplegable de validación de datos (lista) abierto sobre la celda activa. */
  const [validOpen, setValidOpen] = useState(false);
  useMobileBack(validOpen, () => setValidOpen(false));
  /** Columna cuyo desplegable de autofiltro está abierto (encabezado); null = ninguno. */
  const [filterCol, setFilterCol] = useState<number | null>(null);
  useMobileBack(filterCol !== null, () => setFilterCol(null));
  /** El formulario «Filtrar por condición» abierto en el desplegable de `filterCol`. */
  const [condOpen, setCondOpen] = useState(false);
  /** Portapapeles interno pendiente: rango origen y si fue CORTAR (mover). */
  const clipRef = useRef<{ cut: boolean; rect: Rect; sheet: string } | null>(null);

  /* ----- edición de fórmulas: estado del editor activo (celda o barra fx) ----- */
  /** Fórmula en edición y posición del cursor; null si no se edita ninguna. */
  const [fxEdit, setFxEdit] = useState<FxEditState | null>(null);
  /** Índice resaltado del desplegable de autocompletado. */
  const [acIndex, setAcIndex] = useState(0);
  /** Autocompletado cerrado con Esc hasta que cambie el prefijo. */
  const [acDismissed, setAcDismissed] = useState(false);
  /** Espejo síncrono de fxEdit para los manejadores de arrastre (point-and-click). */
  const fxEditRef = useUltimo(fxEdit);
  /** Referencia recién insertada por clic: un nuevo clic la REEMPLAZA (criterio Excel). */
  const refInsertRef = useRef<{ start: number; end: number } | null>(null);
  /** Arrastre de selección de rango para point-and-click (ancla). `kind` marca un
   *  arrastre por CABECERAS: columna(s) entera(s) ('col') o fila(s) enteras ('row'). */
  const pickDragRef = useRef<{ anchor: Pos; kind?: 'col' | 'row' } | null>(null);
  /** Ancla de la última ref insertada por clic (para extender con Mayús+clic). */
  const pickAnchorRef = useRef<Pos | null>(null);
  /** IME en curso: no re-tintar la celda (evita romper la composición). */
  const composingRef = useRef(false);
  /** Cursor pendiente que colocar en la barra fx tras un cambio programático. */
  const pendingBarCaretRef = useRef<number | null>(null);
  const fxInputRef = useRef<HTMLInputElement>(null);
  const fxBackdropRef = useRef<HTMLDivElement>(null);

  const sheet = useMemo<SheetData>(
    () =>
      model.sheets[model.active] ??
      model.sheets[0] ?? { name: t('sheet.defaultName', { n: 1 }), cells: {} },
    [model.active, model.sheets, t],
  );
  const rect = rectOf(sel, ext);

  /* Vistas de filtro PERSONALES (sheets/filtro.ts): la vista activa de cada hoja manda sobre
   * el autofiltro compartido solo en lo que ve esta persona; el modelo no se toca. */
  const vistas = useFiltroVistas(ctx.doc.id);
  const modeloVisto = useMemo<SheetModel>(
    () =>
      model.sheets.some((s) => s.autofilter && activeView(vistas.state, s.name))
        ? {
            ...model,
            sheets: model.sheets.map((s) =>
              s.autofilter
                ? { ...s, autofilter: withView(s.autofilter, activeView(vistas.state, s.name)) }
                : s,
            ),
          }
        : model,
    [model, vistas.state],
  );
  /* Motor de fórmulas: se pone al día cuando cambia el modelo (no en cada selección).
   * Vive entre ediciones y solo se le escribe lo que cambió (ver ./motor-vivo); es la
   * MISMA instancia mientras no cambien las hojas o las tablas, así que lo que se
   * memoriza a partir de él depende también de `model`. getValue da el valor
   * calculado CRUDO (el numfmt lo formatea al pintar); getRaw sigue siendo la fuente de
   * verdad que persiste (§7). */
  const motorRef = useRef<MotorVivo | null>(null);
  const engine = useMemo(() => {
    const { vivo } = sincronizarMotor(motorRef.current, modeloVisto);
    motorRef.current = vivo;
    return vivo.engine;
  }, [modeloVisto]);
  /** El autofiltro que ESTA persona ve: el compartido con su vista activa encima. */
  const vistaActiva = vistas.activeOf(sheet.name);
  const afVisto = useMemo(
    () => (sheet.autofilter ? withView(sheet.autofilter, vistaActiva) : undefined),
    [sheet.autofilter, vistaActiva],
  );

  /* Referencias estructuradas Tabla[Columna]: tablas resolubles del libro (el
   * mismo mapa que usa el resolutor del motor) para el autocompletado de nombres
   * de tabla/columna y el resaltado de la columna en la rejilla. */
  const book = useMemo(() => resolvableTablesOf(model), [model]);

  /** Resuelve una `Tabla[Columna]` a hoja+rectángulo para resaltarla, anclada en
   *  la celda seleccionada (para `@`/tabla implícita). null si es irresoluble. */
  const resolveStructuredRect: StructuredResolver = (s) => {
    const rr = resolveStructuredRef(book, s, { sheet: sheet.name, col: sel.c, row: sel.r });
    if (!rr || rr instanceof FormulaError) return null;
    return { sheet: rr.sheet, rect: { c1: rr.c1, r1: rr.r1, c2: rr.c2, r2: rr.r2 } };
  };

  /* Lo que se ve del contenedor de desplazamiento (px de maqueta, cuantizado a
   * bloques): decide la ventana que se pinta y empuja la extensión al desplazarse. */
  const [vista, setVista] = useState<Vista>({ top: 0, left: 0, w: 1600, h: 900 });
  /* Extensión de la hoja: lo usado, la selección y lo que se ve, con holgura. Es local
   * del render: los manejadores registrados una sola vez la leen de stateRef
   * (cols/rows), y lo que acota una escritura usa los límites MAX_COLS/MAX_ROWS. */
  const usado = engine.usedRange(sheet.name);
  const { cols: nCols, rows: nRows } = extension(
    usado.maxCol,
    usado.maxRow,
    Math.max(sel.c, ext?.c ?? 0) + 1,
    Math.max(sel.r, ext?.r ?? 0) + 1,
    vista,
    CELL_W,
    CELL_H,
  );

  /* Paneles congelados y filas visibles tras el autofiltro. */
  const freeze = sheet.freeze ?? { cols: 0, rows: 0 };
  /* Tablas con formato compiladas (parse de rango una vez): banda/rol de celda,
   * fila de totales y contorno. La fila de totales se PROTEGE del autofiltro
   * (nunca se oculta, aunque su columna filtrada tenga un valor excluido). */
  const tableRects = compileTables(sheet.tables ?? []);

  /* Esquema (agrupar/subtotales): grupos de filas y columnas. Una fila/columna se
   * oculta si está en el detalle de un grupo COLAPSADO; se une a la ocultación del
   * autofiltro (filas). Sin grupos, todo esto queda inerte y la rejilla se pinta
   * idéntica a antes (RGUT/CGUT = 0, visCols = todas). */
  const rowGroups = sheet.rowGroups ?? [];
  const colGroups = sheet.colGroups ?? [];
  // Memorizado: el filtro lee una celda por fila y la hoja puede tener cien mil. Antes,
  // con cien filas fijas, se recalculaba en cada render (cada clic) sin que se notara.
  const hidden = useMemo(() => {
    const totals = new Set<number>();
    for (const ct of compileTables(sheet.tables ?? [])) if (ct.totals) totals.add(ct.rect.r2);
    const h = filteredRows(afVisto, engine, sheet.name, totals, sheet.styles);
    for (const r of hiddenUnits(sheet.rowGroups ?? [])) h.add(r);
    return h;
    // `model` va en las dependencias aunque el cuerpo no lo lea: el motor es la MISMA instancia
    // entre ediciones (ver motor-vivo), así que lo único que avisa de que cambió una celda,
    // incluso de OTRA hoja, es el modelo.
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, [engine, model, sheet, afVisto]);
  const visRows = useMemo(() => {
    const out: number[] = [];
    for (let r = 0; r < nRows; r++) if (!hidden.has(r)) out.push(r);
    return out;
  }, [hidden, nRows]);
  const hiddenCols = useMemo(() => hiddenUnits(sheet.colGroups ?? []), [sheet]);
  const visCols: number[] = [];
  for (let c = 0; c < nCols; c++) if (!hiddenCols.has(c)) visCols.push(c);
  /** Índice visible de una fila (-1 si está oculta o fuera de la extensión). */
  const viOf = (r: number): number => indiceVisible(visRows, r);

  /* Canalón de esquema: un carril por nivel a la izquierda (filas) y arriba
   * (columnas). Desplaza el ORIGEN del contenido (OX/OY) sin cambiar el ancho/alto
   * de celda; toda la geometría de superposición se apoya en OX/OY y en colX().
   * ponytail: los gráficos flotantes usan px absolutos (no siguen a las columnas
   * ocultas ni al canalón, igual que ya no siguen a las filas filtradas); con
   * grupos presentes se desplazan RGUT/CGUT px — reposicionarlos es un follow-up. */
  const rowLevels = maxOutlineLevel(rowGroups);
  const colLevels = maxOutlineLevel(colGroups);
  const RGUT = rowLevels > 0 ? rowLevels * OUTLINE_STEP + OUTLINE_PAD : 0;
  const CGUT = colLevels > 0 ? colLevels * OUTLINE_STEP + OUTLINE_PAD : 0;
  const OX = RGUT + HEAD_W; // x del borde izquierdo de la columna 0
  const OY = CGUT + CELL_H; // y del borde superior de la fila 0
  /* x del borde izquierdo de la columna c (px de maqueta, saltando columnas
   * ocultas); colX[nCols] = borde derecho de la última columna. */
  const colX: number[] = new Array<number>(nCols + 1);
  /* Anchos y altos propios (de un .xlsx o un .ods): solo lo que se aparta de la medida de
   * serie. Sin ellos, `rowTop` es null y la geometría es la de siempre (índice × paso). */
  const hayAnchos = !!sheet.colWidths && Object.keys(sheet.colWidths).length > 0;
  const hayAltos = !!sheet.rowHeights && Object.keys(sheet.rowHeights).length > 0;
  const colW = (c: number): number => medidaDe(sheet.colWidths, c, CELL_W);
  {
    let n = 0;
    for (let c = 0; c <= nCols; c++) {
      colX[c] = OX + n;
      if (c < nCols && !hiddenCols.has(c)) n += colW(c);
    }
  }
  /* Celdas combinadas (sheets/fusiones.ts): válidas y sin solaparse, memorizadas por la lista. */
  const fusiones = useMemo(() => parseFusiones(sheet.merges), [sheet.merges]);
  /** Borde superior de cada fila visible (+ el final de la última); null sin altos propios. */
  const rowTop = useMemo(
    () =>
      hayAltos
        ? acumulado(OY, visRows, (r) => medidaDe(sheet.rowHeights, r, CELL_H, ALTO_MAX))
        : null,
    [hayAltos, sheet.rowHeights, visRows, OY],
  );
  /** y del borde superior de la fila visible `vi`, y lo que mide. */
  const topDe = (vi: number): number => (rowTop ? (rowTop[vi] ?? 0) : OY + vi * CELL_H);
  const altoDe = (vi: number): number =>
    rowTop ? (rowTop[vi + 1] ?? 0) - (rowTop[vi] ?? 0) : CELL_H;

  /* Formato condicional avanzado: mapa ref→decoración (relleno/barra/icono),
   * recalculado SOLO al cambiar el modelo (engine) o la hoja activa, no en cada
   * selección. Las reglas de FÓRMULA las evalúa el motor sin escribirlas en ninguna
   * celda (`evaluate`): no tocan el valor ni el rango usado de la hoja. */
  const decorMap = useMemo(() => {
    const rules = sheet.condFormats;
    if (!rules || rules.length === 0) return EMPTY_DECOR;
    const get = (c: number, r: number): CellValue => engine.getValue(sheet.name, A1(c, r));
    const evalFormula = (formula: string, dc: number, dr: number): boolean => {
      const shifted = shiftRefs(formula.startsWith('=') ? formula : `=${formula}`, dc, dr);
      if (shifted.startsWith('#')) return false; // #REF! fuera de rejilla → no dispara
      return isTruthy(engine.evaluate(sheet.name, shifted));
    };
    return buildCondDecor(rules, get, evalFormula);
    // `model` va en las dependencias aunque el cuerpo no lo lea: el motor es la MISMA instancia
    // entre ediciones (ver motor-vivo), así que lo único que avisa de que cambió una celda,
    // incluso de OTRA hoja, es el modelo.
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, [engine, model, sheet]);

  /* Sparklines por celda anfitriona (barato: pocas; sin useMemo). */
  const sparkByCell = new Map<string, Sparkline>();
  for (const sp of sheet.sparklines ?? []) sparkByCell.set(sp.cell, sp);

  /* Espejo síncrono del estado para los manejadores registrados una sola vez
   * (registro de comandos del ribbon) y para leer el modelo recién aplicado. */
  const stateRef = useUltimo({
    ctx,
    model,
    sel,
    ext,
    editing,
    engine,
    visRows,
    visCols,
    ox: OX,
    oy: OY,
    rowTop,
    colX,
    fusiones,
    /** Extensión de la hoja (lo que se recorre); los límites son MAX_COLS/MAX_ROWS. */
    cols: nCols,
    rows: nRows,
  });

  /** Lee lo que se ve del contenedor y lo guarda cuantizado: solo repinta al cruzar de
   *  bloque. Lee refs, así que sirve igual desde el primer render que desde el último. */
  const medirVista = (): void => {
    const el = scrollRef.current;
    if (!el) return;
    const z = stateRef.current.ctx.ui.zoom / 100;
    const v: Vista = {
      top: cuantiza(el.scrollTop / z, VISTA_PASO_FILAS * CELL_H),
      left: cuantiza(el.scrollLeft / z, VISTA_PASO_COLS * CELL_W),
      w: Math.ceil(el.clientWidth / z),
      h: Math.ceil(el.clientHeight / z),
    };
    setVista((p) => (p.top === v.top && p.left === v.left && p.w === v.w && p.h === v.h ? p : v));
  };
  useEffect(() => {
    const el = scrollRef.current;
    if (!el || typeof ResizeObserver === 'undefined') return;
    const ro = new ResizeObserver(() => medirVista());
    ro.observe(el);
    return () => ro.disconnect();
    // medirVista solo lee refs: registrarlo una vez basta.
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  // El zoom cambia lo que cabe sin que el contenedor cambie de tamaño.
  // oxlint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => medirVista(), [ctx.ui.zoom]);

  /** ¿Hay que traer la celda activa a la vista en el próximo render? Lo marca quien
   *  navega con el teclado. Se resuelve por geometría y DESPUÉS de pintar, porque la
   *  celda de destino puede no existir aún en el DOM (fuera de la ventana) ni caber en
   *  la extensión de antes; scrollIntoView sobre ella no haría nada. */
  const revelarRef = useRef(false);
  useLayoutEffect(() => {
    const el = scrollRef.current;
    if (!revelarRef.current || !el) return;
    revelarRef.current = false;
    const p = ext ?? sel;
    const z = ctx.ui.zoom / 100;
    const top = el.scrollTop / z;
    const left = el.scrollLeft / z;
    // Lo que tapan las cabeceras y lo inmovilizado no cuenta como visible.
    let fzH = 0;
    for (const r of visRows)
      if (r < freeze.rows) fzH += medidaDe(sheet.rowHeights, r, CELL_H, ALTO_MAX);
    const fzW = colX[Math.min(freeze.cols, nCols)]! - OX;
    const vi = viOf(p.r);
    if (vi >= 0 && p.r >= freeze.rows) {
      const y1 = topDe(vi);
      if (y1 < top + OY + fzH) el.scrollTop = (y1 - OY - fzH) * z;
      else if (y1 + altoDe(vi) > top + el.clientHeight / z)
        el.scrollTop = (y1 + altoDe(vi)) * z - el.clientHeight;
    }
    if (p.c >= freeze.cols && p.c < nCols) {
      const x1 = colX[p.c]!;
      const x2 = colX[p.c + 1]!;
      if (x1 < left + OX + fzW) el.scrollLeft = (x1 - OX - fzW) * z;
      else if (x2 > left + el.clientWidth / z) el.scrollLeft = x2 * z - el.clientWidth;
    }
    medirVista(); // la ventana nueva, antes de pintar: sin un fotograma en blanco
  });

  /** Valores numéricos del rango de una sparkline, en orden de lectura. */
  const sparkValues = (range: string): number[] => {
    const rr = parseRangeRef(range);
    if (!rr) return [];
    const out: number[] = [];
    for (let r = rr.r1; r <= Math.min(rr.r2, nRows - 1); r++) {
      for (let c = rr.c1; c <= Math.min(rr.c2, nCols - 1); c++) {
        const v = engine.getValue(sheet.name, A1(c, r));
        out.push(typeof v === 'number' && Number.isFinite(v) ? v : 0);
      }
    }
    return out;
  };

  /** Antepone el icono de conjunto (formato condicional) al contenido, dentro
   *  del envoltorio canónico (negrita/cursiva/alineación) para que quede en línea. */
  const withIcon = (html: string, icon: string | undefined): string => {
    if (!icon) return html;
    const p = parseCell(html);
    p.inner = iconSetSvg(icon) + p.inner;
    return renderCellHtml(p);
  };

  /** Texto (en negrita) de una celda de la fila de totales de una tabla: agrega
   *  la columna con su función sobre las filas VISIBLES (semántica SUBTOTAL: las
   *  filtradas no cuentan). La 1.ª columna sin función muestra «Total» (Excel). */
  const tableTotalHtml = (ct: CompiledTable, col: number): string => {
    const fn = ct.table.totals?.[String(col)] ?? 'none';
    if (fn === 'none') return col === 0 ? `<b>${escapeHtml(t('sheet.total'))}</b>` : '';
    const absCol = ct.rect.c1 + col;
    const vals: CellValue[] = [];
    for (let r = ct.bodyR1; r <= ct.bodyR2; r++) {
      if (!hidden.has(r)) vals.push(engine.getValue(sheet.name, A1(absCol, r)));
    }
    // Formatea con el numfmt de la celda de totales o, si no, el de la última fila de datos.
    const fmt = sheet.formats?.[A1(absCol, ct.rect.r2)] ?? sheet.formats?.[A1(absCol, ct.bodyR2)];
    return `<b>${escapeHtml(formatCellValue(tableTotalValue(fn, vals), fmt))}</b>`;
  };

  /** HTML a pintar en una celda: fila de totales de tabla o sparkline si las hay;
   *  si no, el texto CRUDO (literal o «=fórmula») formateado, con el icono
   *  condicional antepuesto. Se usa engine.getValue (valor crudo), nunca
   *  getDisplay, para poder formatearlo. */
  const cellHtml = (ref: string): string => {
    if (tableRects.length > 0) {
      const rc = parseRangeRef(ref);
      const info = rc ? tableCellInfo(tableRects, rc.c1, rc.r1) : null;
      if (info?.role === 'total') return tableTotalHtml(info.ct, info.colIndex);
    }
    const spark = sparkByCell.get(ref);
    if (spark) return sparklineSvg(sparkValues(spark.range), spark.type, spark.color);
    // El literal de una celda se pinta como HTML (`dangerouslySetInnerHTML`), y lo
    // pudo escribir un coeditor: se sanea en las dos ramas de abajo que lo emiten
    // sin escapar. Las ramas de fórmula/formato ya emiten el valor escapado.
    const raw = sheet.cells[ref] ?? '';
    const fmt = sheet.formats?.[ref];
    const icon = decorMap.get(ref)?.icon;
    if (isFormula(raw)) {
      const p = parseCell(raw); // conserva negrita/cursiva/alineación de la celda
      p.inner = numFmtInnerHtml(engine.getValue(sheet.name, ref), fmt);
      return withIcon(renderCellHtml(p), icon);
    }
    if (!fmt || fmt.id === 'general') return withIcon(saneaHtmlDeDocumento(raw), icon);
    const v = engine.getValue(sheet.name, ref);
    if (typeof v !== 'number') return withIcon(saneaHtmlDeDocumento(raw), icon); // el numfmt solo re-pinta números
    const p = parseCell(raw);
    p.inner = numFmtInnerHtml(v, fmt);
    return withIcon(renderCellHtml(p), icon);
  };

  /* ----- persistencia (contrato §7: mismo camino que el folio) ----- */

  /**
   * Deshacer y rehacer. Vive aquí porque `apply` es el ÚNICO sitio por el que
   * pasan los sesenta y un comandos del lienzo: anotando en él, ninguno se queda
   * fuera y ninguno tiene que acordarse de nada. Ver sheets/historial.ts.
   */
  const historialRef = useRef<Historial<SheetModel>>(nuevoHistorial<SheetModel>());

  /**
   * `anota` es false SOLO cuando el cambio viene de deshacer o rehacer: si no,
   * deshacer se anotaría a sí mismo y Ctrl+Z se quedaría rebotando entre los dos
   * últimos estados sin llegar nunca al tercero.
   */
  const apply = (next: SheetModel, anota = true): void => {
    if (anota) historialRef.current = anotar(historialRef.current, stateRef.current.model);
    stateRef.current.model = next; // coherencia dentro del mismo tick
    setModel(next);
    stateRef.current.ctx.onContentInput(JSON.stringify(next));
  };

  const irAtras = (): void => {
    const r = deshacerHistorial(historialRef.current, stateRef.current.model);
    if (!r) return;
    historialRef.current = r.historial;
    apply(r.modelo, false);
  };

  const irAlante = (): void => {
    const r = rehacerHistorial(historialRef.current, stateRef.current.model);
    if (!r) return;
    historialRef.current = r.historial;
    apply(r.modelo, false);
  };

  /** Parchea la hoja activa (cells, formats, condFormats…) y persiste. */
  const applySheet = (patch: Partial<SheetData>): void => {
    const m = stateRef.current.model;
    apply({ ...m, sheets: m.sheets.map((s, i) => (i === m.active ? { ...s, ...patch } : s)) });
  };

  const applyCells = (cells: Record<string, string>): void => applySheet({ cells });

  /* ----- gráficos flotantes (fase Visual): mover al soltar y borrar ----- */

  /** Con la hoja protegida SIN el permiso «objetos», los gráficos y las segmentaciones
   *  no se mueven, ni se editan, ni se quitan (lo que hace Excel). Antes solo se vetaba
   *  crearlos: se podía borrar un gráfico de una hoja protegida. */
  const objectsLocked = (): boolean => {
    const s = stateRef.current.model.sheets[stateRef.current.model.active];
    if (!s?.protect || s.protect.objects === true) return false;
    notifyProtected(refOf(stateRef.current.sel));
    return true;
  };

  const moveChart = (id: string, cx: number, cy: number): void => {
    if (objectsLocked()) return;
    const m = stateRef.current.model;
    const s = m.sheets[m.active];
    if (!s?.charts) return;
    applySheet({ charts: s.charts.map((ch) => (ch.id === id ? { ...ch, x: cx, y: cy } : ch)) });
  };

  const removeChart = (id: string): void => {
    if (objectsLocked()) return;
    const m = stateRef.current.model;
    const s = m.sheets[m.active];
    if (!s?.charts) return;
    const charts = s.charts.filter((ch) => ch.id !== id);
    const next: SheetData = { ...s, charts };
    if (charts.length === 0) delete next.charts; // no ensuciar el JSON persistido
    apply({ ...m, sheets: m.sheets.map((x, i) => (i === m.active ? next : x)) });
  };

  /** Edita un gráfico existente (tipo/título/rango/series desde su tarjeta). El
   *  parche se sanea; si el rango nuevo no vale, conserva el gráfico anterior. */
  const editChart = (id: string, patch: Partial<ChartSpec>): void => {
    if (objectsLocked()) return;
    const m = stateRef.current.model;
    const s = m.sheets[m.active];
    if (!s?.charts) return;
    applySheet({
      charts: s.charts.map((ch) =>
        ch.id === id ? (sanitizeChartSpec({ ...ch, ...patch }) ?? ch) : ch,
      ),
    });
  };

  /* ----- slicers (segmentación): mover, borrar y aplicar selección ----- *
   * El filtrado en vivo lo hace el autofiltro: la selección de la tarjeta se
   * traduce a `autofilter.excl[col]` (reusa la lógica de filteredRows). */

  const moveSlicer = (id: string, cx: number, cy: number): void => {
    if (objectsLocked()) return;
    const m = stateRef.current.model;
    const s = m.sheets[m.active];
    if (!s?.slicers) return;
    applySheet({ slicers: s.slicers.map((sl) => (sl.id === id ? { ...sl, x: cx, y: cy } : sl)) });
  };

  /** Por qué no se pudo escribir una tabla dinámica. */
  const notifyPivotFailed = (error: 'source' | 'overlap' | 'room'): void => {
    notify({
      tone: 'warning',
      title: t('sheet.msg.pivotFailed'),
      description: t(
        error === 'overlap'
          ? 'sheet.msg.pivotOverlap'
          : error === 'room'
            ? 'sheet.msg.pivotNoRoom'
            : 'sheet.msg.pivotNoSource',
      ),
    });
  };

  const removeSlicer = (id: string): void => {
    if (objectsLocked()) return;
    const m = stateRef.current.model;
    const s = m.sheets[m.active];
    if (!s?.slicers) return;
    const slicers = s.slicers.filter((sl) => sl.id !== id);
    let next: SheetData = { ...s, slicers };
    if (slicers.length === 0) delete next.slicers; // no ensuciar el JSON persistido
    // La de un campo que no está en su tabla dinámica se lleva su filtro oculto (nada
    // más lo enseña), en el mismo paso de deshacer. Si la tabla no se puede reescribir
    // —hoja protegida, o ya no cabe—, la tarjeta se queda y se dice por qué.
    const sin = pivotOnSlicerRemoval(
      s.slicers.find((x) => x.id === id),
      s.pivots,
      !!s.protect,
    );
    if (sin === 'protected') {
      notify({
        tone: 'info',
        title: t('sheet.msg.protectedSheet'),
        description: t('sheet.msg.slicerPivotProtected'),
      });
      return;
    }
    if (sin) {
      const sinTarjeta = { ...m, sheets: m.sheets.map((x, i) => (i === m.active ? next : x)) };
      const res = writePivot(sinTarjeta, stateRef.current.engine, m.active, sin, pivotLabels(t));
      if ('error' in res) {
        notifyPivotFailed(res.error);
        return;
      }
      next = res.sheet;
    }
    apply({ ...m, sheets: m.sheets.map((x, i) => (i === m.active ? next : x)) });
  };

  /** Aplica la selección de un slicer a su columna del autofiltro: excl[col] =
   *  únicos \ seleccionados; vacío ⇒ se quita la exclusión (columna sin filtrar). */
  const applySlicerSelection = (col: number, selected: string[], unique: string[]): void => {
    const m = stateRef.current.model;
    const s = m.sheets[m.active];
    const af = s?.autofilter;
    if (!s || !af) return;
    const excluded = excludedFromSelection(unique, selected);
    // Con una vista activa la segmentación mueve LA VISTA (solo la ve esta persona).
    const vista = vistas.activeOf(s.name);
    if (vista) {
      vistas.update(s.name, viewWithCriterios(vista, setExcluded(vista, col, excluded)));
      return;
    }
    // Segmentar es filtrar: con la hoja protegida pide el mismo permiso que `filterset`.
    if (s.protect && s.protect.autoFilter !== true) {
      notifyProtected(refOf(stateRef.current.sel));
      return;
    }
    applySheet({ autofilter: withCriterios(af, setExcluded(af, col, excluded)) });
  };

  /** Cambia un criterio del filtro que ve esta persona: en su vista activa (nadie más lo ve)
   *  o, sin vista, en el autofiltro compartido por el comando de siempre (con su permiso). */
  const editarFiltro = (
    cmd: 'filterset' | 'filtercond',
    req: { col: number; value?: string; hidden?: boolean; cond?: FilterCondition | null },
  ): void => {
    if (!vistaActiva) {
      dispatchSheetCommand(cmd, JSON.stringify(req));
      return;
    }
    const crit =
      cmd === 'filtercond'
        ? setCond(vistaActiva, req.col, req.cond ?? null)
        : toggleValue(vistaActiva, req.col, req.value ?? '', req.hidden === true);
    vistas.update(sheet.name, viewWithCriterios(vistaActiva, crit));
  };

  /** La selección de la segmentación de una tabla dinámica: el campo oculta lo que no
   *  está elegido y la tabla se vuelve a escribir. Es reescribir celdas, así que con la
   *  hoja protegida queda vetada como `pivot`. */
  const applyPivotSlicer = (sl: Slicer, selected: string[], unique: string[]): void => {
    const cur = stateRef.current;
    const m = cur.model;
    const s = m.sheets[m.active];
    const pv = s?.pivots?.find((p) => p.name === sl.pivot);
    if (!s || !pv) return;
    if (s.protect) {
      notifyProtected(refOf(cur.sel));
      return;
    }
    const hidden = excludedFromSelection(unique, selected);
    const res = writePivot(
      m,
      cur.engine,
      m.active,
      pivotWithHidden(pv, sl.col, hidden),
      pivotLabels(t),
    );
    if ('error' in res) return;
    apply({ ...m, sheets: m.sheets.map((x, i) => (i === m.active ? res.sheet : x)) });
  };

  /** Lo que pinta la segmentación de una tabla dinámica: el nombre del campo (su
   *  encabezado en el origen), sus elementos con lo que la tabla oculta, y si sigue
   *  viva (la tabla existe y el campo, en su origen). */
  const pivotSlicerView = (
    sl: Slicer,
  ): { field: string; entries: SlicerEntry[]; active: boolean } => {
    const pv = sheet.pivots?.find((p) => p.name === sl.pivot);
    const origen = pv ? pivotSource(model, engine, model.active, pv, pivotLabels(t)) : null;
    const head = origen?.grid[0]?.[sl.col];
    const name = head === undefined ? '' : formatCellValue(head);
    const field = name !== '' ? name : t('common.columnN', { n: sl.col + 1 });
    if (!pv || !origen || head === undefined) return { field, entries: [], active: false };
    const ref = pivotSlicerRef(pv, sl.col);
    const hidden = new Set(ref.hidden ?? []);
    return {
      field,
      active: true,
      entries: pivotItems(origen.grid, ref, origen.labels).map((it) => ({
        value: it.key,
        label: it.label,
        hidden: hidden.has(it.key),
      })),
    };
  };

  /* ----- utilidades DOM ----- */

  const cellEl = (pos: Pos): HTMLElement | null =>
    gridRef.current?.querySelector<HTMLElement>(`[data-ref="${refOf(pos)}"]`) ?? null;

  const posOf = (el: HTMLElement): Pos => ({
    c: Number(el.dataset['c'] ?? 0),
    r: Number(el.dataset['r'] ?? 0),
  });

  /** Validación de datos que cubre una celda de la hoja (null si ninguna). */
  const validationOf = (s: SheetData, c: number, r: number): DataValidation | null =>
    s.validations ? validationAt(compileValidations(s.validations), c, r) : null;

  /** Evalúa la fórmula de una validación `formula` para la celda (c,r) con el
   *  valor CANDIDATO puesto en un motor DESECHABLE (como Buscar objetivo/Solver):
   *  no toca la hoja real. Las refs relativas de la fórmula se desplazan desde el
   *  inicio del rango de la validación hasta la celda (criterio Excel). El
   *  candidato entra como texto crudo (literal «42» o fórmula «=…»), así que su
   *  RESULTADO es lo que ve la validación. */
  const dvFormulaOk = (dv: DataValidation, c: number, r: number, candidate: string): boolean => {
    if (!dv.formula) return true;
    const rect = parseRangeRef(dv.range);
    if (!rect) return true;
    const shifted = shiftRefs(
      dv.formula.startsWith('=') ? dv.formula : `=${dv.formula}`,
      c - rect.c1,
      r - rect.r1,
    );
    if (shifted.startsWith('#')) return false; // #REF! fuera de rejilla → no cumple
    const cur = stateRef.current;
    const s = cur.model.sheets[cur.model.active];
    if (!s) return true;
    const eng = buildEngine(cur.model);
    eng.setCell(s.name, refOf({ c, r }), candidate);
    return isTruthy(eng.evaluate(s.name, shifted, c, r));
  };

  /* ----- protección de hoja/celda (paridad Excel; predicados puros en format.ts) ----- */

  /** Antirebote del aviso «hoja protegida» (evita spam al teclear sobre celdas bloqueadas). */
  const protectToastAtRef = useRef(0);
  const notifyProtected = (ref: string): void => {
    const now = ahoraMs();
    if (now - protectToastAtRef.current < 1500) return; // ponytail: throttle simple; sube el umbral si molesta
    protectToastAtRef.current = now;
    notify({
      tone: 'info',
      title: t('sheet.msg.protectedSheet'),
      description: t('sheet.msg.protectedCell', { ref }),
    });
  };
  /** ¿Se puede editar AHORA el contenido de esta celda? (hoja protegida + bloqueada = no;
   *  o fórmula OCULTA aunque esté desbloqueada: editarla la revelaría o borraría). */
  const canEditAt = (pos: Pos): boolean => {
    const s = stateRef.current.model.sheets[stateRef.current.model.active];
    const ref = refOf(pos);
    return canEditCellContent(
      !!s?.protect,
      estiloParaMi(s?.protect, ref, s?.styles?.[ref], yo()),
      s?.cells[ref] ?? '',
    );
  };
  /** ¿Se puede SELECCIONAR esta celda? (protegida sin «seleccionar celdas» ⇒ solo desbloqueadas). */
  const canSelectAt = (pos: Pos): boolean => {
    const s = stateRef.current.model.sheets[stateRef.current.model.active];
    if (!s?.protect || s.protect.selectCells !== false) return true;
    const ref = refOf(pos);
    return canEditCell(true, estiloParaMi(s.protect, ref, s.styles?.[ref], yo()));
  };
  /** ¿Hay alguna celda bloqueada en el rango con la hoja protegida? (veta ediciones masivas). */
  const rangeLocked = (rc: Rect): boolean => {
    const s = stateRef.current.model.sheets[stateRef.current.model.active];
    if (!s?.protect) return false;
    for (let r = rc.r1; r <= rc.r2; r++) {
      for (let c = rc.c1; c <= rc.c2; c++) {
        const ref = refOf({ c, r });
        if (!canEditCell(true, estiloParaMi(s.protect, ref, s.styles?.[ref], yo()))) return true;
      }
    }
    return false;
  };

  /* ----- selección y edición (§7.1) ----- */

  const select = (raw: Pos, extend: boolean): void => {
    // Una celda cubierta por una combinada es su ancla.
    const fu = extend ? null : fusionEn(stateRef.current.fusiones, raw.c, raw.r);
    const pos = fu ? { c: fu.c1, r: fu.r1 } : raw;
    if (extend) {
      setExt(pos);
    } else {
      setSel(pos);
      setExt(null);
    }
  };

  /** Última celda con contenido de la hoja, o de una fila concreta si se pide;
   *  null si ahí no hay nada (así Fin en una fila vacía se queda quieto en vez de
   *  saltar a la columna A, que es lo que hace Inicio). */
  const lastUsedCell = (row?: number): Pos | null => {
    const cur = stateRef.current;
    const cells = cur.model.sheets[cur.model.active]?.cells ?? {};
    let c = -1;
    let r = -1;
    for (const ref in cells) {
      const p = parseRangeRef(ref);
      if (!p || (row !== undefined && p.r1 !== row)) continue;
      c = Math.max(c, p.c1);
      r = Math.max(r, p.r1);
    }
    return c < 0 ? null : { c, r: row ?? r };
  };

  /** Filas que caben en la ventana de la rejilla: Re Pág/Av Pág mueven una
   *  PANTALLA, no un salto fijo, así que el zoom y el alto real mandan. Al menos
   *  una: antes del primer layout el contenedor mide 0 y no movería nada. */
  const pageRows = (): number => {
    const h = (scrollRef.current?.clientHeight ?? 0) / (stateRef.current.ctx.ui.zoom / 100);
    return Math.max(1, Math.floor(h / CELL_H) - 1);
  };

  const move = (dc: number, dr: number, extend: boolean): void => {
    const cur = stateRef.current;
    const base = extend ? (cur.ext ?? cur.sel) : cur.sel;
    let pos = moverPorFusiones(cur.fusiones, base, dc, dr, MAX_COLS - 1, MAX_ROWS - 1);
    if (!canSelectAt(pos)) {
      // «Seleccionar celdas» desactivado y destino bloqueado: salta a la siguiente
      // celda seleccionable en la dirección del avance (criterio Excel); si no hay
      // ninguna antes del borde, no mueve.
      const sc = Math.sign(dc);
      const sr = Math.sign(dr);
      if (!sc && !sr) return;
      let c = pos.c;
      let r = pos.r;
      do {
        c += sc;
        r += sr;
        if (c < 0 || c >= MAX_COLS || r < 0 || r >= MAX_ROWS) return;
      } while (!canSelectAt({ c, r }));
      pos = { c, r };
    }
    select(pos, extend);
    revelarRef.current = true;
  };

  /** Ctrl+flecha: al borde del bloque de datos, a la próxima celda con datos o al borde
   *  de la hoja (criterio Excel). Mayús extiende como con las flechas. */
  const moveToData = (dc: number, dr: number, extend: boolean): void => {
    const cur = stateRef.current;
    const base = extend ? (cur.ext ?? cur.sel) : cur.sel;
    const cells = cur.model.sheets[cur.model.active]?.cells ?? {};
    // Las posiciones con datos de esa fila o columna. Una pasada por las celdas de la
    // hoja: con cien mil son milisegundos, y solo se hace al pulsar.
    const llenas: number[] = [];
    for (const ref in cells) {
      if (stripHtml(cells[ref] ?? '') === '') continue;
      const p = parseRangeRef(ref);
      if (!p) continue;
      if (dr !== 0 && p.c1 === base.c) llenas.push(p.r1);
      else if (dc !== 0 && p.r1 === base.r) llenas.push(p.c1);
    }
    llenas.sort((a, b) => a - b);
    if (dr !== 0) move(0, saltoADatos(llenas, base.r, dr > 0 ? 1 : -1, MAX_ROWS) - base.r, extend);
    else move(saltoADatos(llenas, base.c, dc > 0 ? 1 : -1, MAX_COLS) - base.c, 0, extend);
  };

  /** Al entrar en edición, mostrar el texto CRUDO: el «=…» de una fórmula o el
   *  literal sin numfmt (el DOM mostraba el número formateado, solo de render;
   *  si se editara ese texto, el commit re-guardaría el formateado). */
  const showEditSource = (pos: Pos, el: HTMLElement): void => {
    const m = stateRef.current.model;
    const s = m.sheets[m.active];
    const ref = refOf(pos);
    const raw = s?.cells[ref] ?? '';
    // Invariante: nunca escribir el «=…» de una fórmula OCULTA en el DOM de la celda
    // (los llamadores ya la vetan por canEditAt; el resguardo vive en el punto de
    // revelado para que ningún llamador futuro filtre la fórmula).
    if (isFormulaHidden(!!s?.protect, s?.styles?.[ref]) && isFormula(raw)) return;
    // El literal lo pudo escribir un coeditor: se sanea como en `cellHtml`.
    if (isFormula(raw)) el.innerHTML = escapeHtml(rawText(raw));
    else if (s?.formats?.[ref]) el.innerHTML = saneaHtmlDeDocumento(raw);
    else if (s?.sparklines?.some((sp) => sp.cell === ref))
      el.innerHTML = saneaHtmlDeDocumento(raw); // limpia el SVG de la sparkline
    else if (tableCellInfo(compileTables(s?.tables ?? []), pos.c, pos.r)?.role === 'total')
      el.innerHTML = saneaHtmlDeDocumento(raw); // limpia el total renderizado (<b>…</b>): su valor vive derivado, no en cells
  };

  const startEdit = (pos: Pos, replaceWith?: string): void => {
    if (!canEditAt(pos)) {
      // Hoja protegida + celda bloqueada: solo lectura. Selecciona (para copiar) sin editar.
      setSel(pos);
      setExt(null);
      notifyProtected(refOf(pos));
      return;
    }
    setSel(pos);
    setExt(null);
    setEditing(true);
    const el = cellEl(pos);
    if (!el) return;
    if (replaceWith !== undefined) el.innerHTML = escapeHtml(replaceWith);
    else showEditSource(pos, el);
    el.focus();
    const range = document.createRange();
    range.selectNodeContents(el);
    range.collapse(false); // caret al final
    const s = window.getSelection();
    s?.removeAllRanges();
    s?.addRange(range);
    // Si el contenido es una fórmula, arranca autocompletado/hints/tintado.
    if (readCellText(el).startsWith('=')) syncCellFx(el, true);
    else endFx();
  };

  /** Commit del contenido editado (blur o confirmación, §7.1): vacío → delete. */
  const commitCell = (pos: Pos, el: HTMLElement): void => {
    // Defensa: una celda bloqueada nunca entra en edición (startEdit la veta), pero
    // si llegara aquí (foco por Tab, etc.) no se persiste su cambio.
    if (!canEditAt(pos)) {
      el.innerHTML = cellHtml(refOf(pos)); // restaura el render
      return;
    }
    // Una fórmula en edición lleva los <span> de tinte (UX de color): la fuente
    // de verdad es su TEXTO plano, así que se persiste el «=…» limpio (escapado),
    // no el marcado. Los literales conservan su envoltorio (negrita/cursiva/alineación).
    const text = stripHtml(el.innerHTML);
    const raw = text.trim().startsWith('=') ? escapeHtml(text) : el.innerHTML;
    // ponytail: celda «vacía» = sin texto plano (v1 no contempla imágenes en celdas).
    const html = text.trim() === '' ? '' : raw;
    const m = stateRef.current.model;
    const s = m.sheets[m.active];
    if (!s) return;
    const ref = refOf(pos);
    if ((s.cells[ref] ?? '') === html) {
      // Sin cambios: el DOM quedó mostrando el texto crudo (showEditSource) y no
      // habrá re-render que lo repinte; restaura el render (valor/formato).
      el.innerHTML = cellHtml(ref);
      return;
    }
    // Validación de datos estricta: se rechaza lo que no cumple el criterio (vacío OK).
    const dv = validationOf(s, pos.c, pos.r);
    const candidate = stripHtml(html);
    if (
      dv?.strict &&
      !validateValue(dv, candidate, () => dvFormulaOk(dv, pos.c, pos.r, candidate))
    ) {
      notify({
        tone: 'warning',
        title: t('sheet.msg.badValue'),
        description: validationMessage(dv, ref),
      });
      el.innerHTML = cellHtml(ref); // revierte al valor anterior
      return;
    }
    const cells = { ...s.cells };
    if (html === '') delete cells[ref];
    else cells[ref] = html;
    anotarCambio(cells, s.cells, [ref]); // el motor no compara la hoja entera
    // Auto-expandir una tabla si se escribe en su columna/fila adyacente (Excel).
    const grown = html === '' ? null : grownTables(s.tables, pos.c, pos.r);
    if (grown) applySheet({ cells, tables: grown });
    else applyCells(cells);
  };

  /** Tablas con el rango extendido si (c,r) cae en su borde derecho/inferior;
   *  null si ninguna crece (evita un re-render inútil). */
  const grownTables = (
    list: SheetTable[] | undefined,
    c: number,
    r: number,
  ): SheetTable[] | null => {
    if (!list || list.length === 0) return null;
    let changed = false;
    const next = list.map((t) => {
      const ct = compileTable(t);
      const grown = ct ? expandRangeForEdit(ct, c, r, MAX_COLS - 1, MAX_ROWS - 1) : null;
      if (!grown) return t;
      changed = true;
      return { ...t, range: `${A1(grown.c1, grown.r1)}:${A1(grown.c2, grown.r2)}` };
    });
    return changed ? next : null;
  };

  const finishEdit = (el: HTMLElement, dc: number, dr: number): void => {
    commitCell(posOf(el), el);
    setEditing(false);
    endFx();
    gridRef.current?.focus();
    move(dc, dr, false);
  };

  const cancelEdit = (el: HTMLElement): void => {
    // Restaura lo mostrado: valor calculado si es fórmula, HTML con formato si no.
    // (Escape no toca el modelo, así que no hay re-render que lo repinte por sí solo.)
    el.innerHTML = cellHtml(refOf(posOf(el)));
    setEditing(false);
    endFx();
    gridRef.current?.focus();
  };

  /* ----- edición de fórmulas: sincronización texto/cursor y acciones ----- */

  /** Cierra el estado de edición de fórmula (autocompletado, refs, tinte). */
  const endFx = (): void => {
    setFxEdit(null);
    setAcIndex(0);
    setAcDismissed(false);
    refInsertRef.current = null;
    pickDragRef.current = null;
  };

  /** Texto crudo de un contenteditable, con NBSP normalizado a espacio (misma
   *  longitud, así los offsets del cursor siguen cuadrando). */
  const readCellText = (el: HTMLElement): string => (el.textContent ?? '').replace(/\u00A0/g, ' ');

  /** Re-tinta el contenteditable con los colores de sus referencias, conservando
   *  el cursor. No hace nada durante una composición IME. */
  const retintCell = (el: HTMLElement, text: string, caret: number): void => {
    if (composingRef.current) return;
    const html = tintedFormulaHtml(text, extractRefs(text, resolveStructuredRect));
    if (el.innerHTML !== html) {
      el.innerHTML = html;
      setCaretOffsetIn(el, caret);
    }
  };

  /** Lee texto+cursor de la celda en edición y actualiza el estado de fórmula.
   *  Si dejó de ser fórmula, limpia el estado y (si estaba tintada) la aplana. */
  const syncCellFx = (el: HTMLElement, retint: boolean): void => {
    const text = readCellText(el);
    const caret = caretOffsetIn(el) ?? text.length;
    if (text.startsWith('=')) {
      if (retint) retintCell(el, text, caret);
      setFxEdit({ surface: 'cell', text, caret });
      setFx(text); // la barra fx refleja la fórmula en curso de la celda
    } else {
      setFxEdit(null);
      if (retint && el.querySelector('span')) {
        // Ya no es fórmula pero quedaban tramos tintados: vuelve a texto plano.
        el.innerHTML = escapeHtml(text);
        setCaretOffsetIn(el, caret);
      }
    }
  };

  /** Columnas de la tabla de un corchete `Tabla[…`: por nombre, o la que contiene
   *  la celda editada si es implícita («[@Col]»). [] si no se encuentra. */
  const columnsForBracket = (brk: { table: string }): string[] => {
    const t =
      brk.table === ''
        ? book.find(
            (x) =>
              x.sheet === sheet.name &&
              sel.c >= x.rect.c1 &&
              sel.c <= x.rect.c2 &&
              sel.r >= x.rect.r1 &&
              sel.r <= x.rect.r2,
          )
        : book.find((x) => x.table.name.toLowerCase() === brk.table.toLowerCase());
    return t?.columns ?? [];
  };

  /**
   * Autocompletado bajo el cursor: columnas si el cursor teclea dentro de
   * `Tabla[…`; si no, funciones + nombres de tabla que casan por prefijo (máx. 8
   * + tablas). Null si no hay nada que ofrecer o si la única coincidencia ya está
   * escrita entera (paridad con la UX previa de funciones).
   */
  const acAt = (text: string, caret: number): AcState | null => {
    const brk = tableBracketAt(text, caret);
    if (brk) {
      const pre = brk.prefix.toLowerCase();
      const cols = columnsForBracket(brk).filter((c) => c.toLowerCase().startsWith(pre));
      if (cols.length === 1 && cols[0]!.toLowerCase() === pre) return null;
      return cols.length > 0
        ? {
            items: cols.slice(0, 8).map((label): AcItem => ({ label, kind: 'column' })),
            start: brk.start,
            end: brk.end,
          }
        : null;
    }
    const tok = funcTokenAt(text, caret);
    if (!tok) return null;
    const up = tok.prefix.toUpperCase();
    const items: AcItem[] = [];
    // Los nombres definidos van primero (son del libro, lo más probable) y como mucho 4.
    for (const n of namesVisibleFrom(model.names, sheet.name)) {
      if (items.length < 4 && n.toUpperCase().startsWith(up))
        items.push({ label: n, kind: 'name' });
    }
    for (const n of FN_NAMES) {
      if (n.startsWith(up)) {
        items.push({ label: n, kind: 'fn' });
        if (items.length >= 8) break;
      }
    }
    for (const t of book) {
      if (items.length >= 12) break;
      if (
        t.table.name.toUpperCase().startsWith(up) &&
        !items.some((x) => x.kind === 'table' && x.label === t.table.name)
      )
        items.push({ label: t.table.name, kind: 'table' });
    }
    // Única coincidencia (función o nombre) ya escrita entera → no ofrecer nada.
    const only = items.length === 1 ? items[0]! : null;
    if (only && only.kind !== 'table' && only.label.toUpperCase() === up) return null;
    return items.length > 0 ? { items, start: tok.start, end: tok.end } : null;
  };

  /** Aplica un texto+cursor nuevos a la superficie activa (barra o celda),
   *  re-tintando y conservando el foco/cursor. */
  const applyFxText = (surface: 'cell' | 'bar', text: string, caret: number): void => {
    if (surface === 'bar') {
      setFx(text);
      pendingBarCaretRef.current = caret;
    } else {
      const el = cellEl(sel);
      if (el) {
        el.innerHTML = tintedFormulaHtml(text, extractRefs(text, resolveStructuredRect));
        el.focus();
        setCaretOffsetIn(el, caret);
      }
      setFx(text); // espejo en la barra fx
    }
    setFxEdit({ surface, text, caret });
  };

  /** Ref A1 de un clic/arrastre en la rejilla: «A1» o «A1:B3» (sin anclar). */
  const rangeRefStr = (a: Pos, b: Pos): string => {
    const r = rectOf(a, b);
    const tl = refOf({ c: r.c1, r: r.r1 });
    return r.c1 === r.c2 && r.r1 === r.r2 ? tl : `${tl}:${refOf({ c: r.c2, r: r.r2 })}`;
  };

  /**
   * Inserta (o reemplaza la recién insertada / la que TOCA el cursor) un texto de
   * referencia en la fórmula en edición; la celda/barra SIGUE en edición (Excel
   * point-and-click). Sin ref pendiente, si el cursor toca una ref ya escrita se
   * SUSTITUYE esa (no se añade otra): recolocar el cursor sobre la recién
   * autocompletada y clicar otra la reemplaza (paridad Excel/Sheets). Núcleo común
   * del point-and-click de celdas y del de cabeceras (columna/fila entera).
   */
  const insertRefText = (fe: FxEditState, refStr: string): void => {
    const prev = refInsertRef.current;
    const onRef = prev ? null : refAtCaret(fe.text, fe.caret);
    const start = prev ? prev.start : onRef ? onRef.start : fe.caret;
    const end = prev ? prev.end : onRef ? onRef.end : fe.caret;
    const text = fe.text.slice(0, start) + refStr + fe.text.slice(end);
    refInsertRef.current = { start, end: start + refStr.length };
    applyFxText(fe.surface, text, start + refStr.length);
  };

  /** Inserta la ref del rango ancla→foco (clic/arrastre sobre celdas). */
  const insertRefFromPick = (fe: FxEditState, anchor: Pos, focus: Pos): void => {
    insertRefText(fe, rangeRefStr(anchor, focus));
  };

  /** Inserta una ref de COLUMNA (A:C) o FILA (1:3) entera (clic/arrastre sobre las
   *  cabeceras); una sola da «A:A» / «1:1». Los extremos se normalizan. */
  const insertHeaderRef = (fe: FxEditState, kind: 'col' | 'row', a: number, b: number): void => {
    const lo = Math.min(a, b);
    const hi = Math.max(a, b);
    insertRefText(fe, kind === 'col' ? `${colName(lo)}:${colName(hi)}` : `${lo + 1}:${hi + 1}`);
  };

  /** ¿La fórmula en edición está lista para recibir una ref por point-and-click?
   *  Cierto en posición de operando (espera ref), con una ref recién insertada
   *  (pendiente), o si el cursor TOCA una ref ya escrita (clicar la sustituye). */
  const pickReady = (fe: FxEditState): boolean =>
    expectsRef(fe.text, fe.caret) ||
    refInsertRef.current !== null ||
    refAtCaret(fe.text, fe.caret) !== null;

  /**
   * Acepta una sugerencia del autocompletado sustituyendo el token bajo el cursor:
   * función → «NOMBRE(»; tabla → «Tabla[» (deja el cursor listo para la columna);
   * columna → «Columna]» (cierra el corchete). Reaprovecha el «[»/«]» ya tecleado
   * si lo hay (no lo duplica). Recalcula el contexto desde el ref vivo del editor.
   */
  const acceptAc = (surface: 'cell' | 'bar', item: AcItem): void => {
    const fe = fxEditRef.current;
    if (!fe) return;
    const ac = acAt(fe.text, fe.caret);
    if (!ac) return;
    const { start, end } = ac;
    let body: string;
    let caret: number;
    if (item.kind === 'fn') {
      body = `${item.label}(`;
      caret = start + body.length;
    } else if (item.kind === 'name') {
      body = item.label;
      caret = start + body.length;
    } else if (item.kind === 'table') {
      body = fe.text[end] === '[' ? item.label : `${item.label}[`;
      caret = start + item.label.length + 1; // tras el «[»
    } else {
      body = fe.text[end] === ']' ? item.label : `${item.label}]`;
      caret = start + item.label.length + 1; // tras el «]»
    }
    refInsertRef.current = null;
    setAcDismissed(false);
    setAcIndex(0);
    applyFxText(surface, fe.text.slice(0, start) + body + fe.text.slice(end), caret);
  };

  /** F4: cicla el anclaje de la referencia bajo el cursor de la superficie activa. */
  const doF4 = (surface: 'cell' | 'bar'): void => {
    const fe = fxEditRef.current;
    if (!fe) return;
    const res = cycleRefAt(fe.text, fe.caret);
    if (!res) return;
    refInsertRef.current = null;
    applyFxText(surface, res.text, res.caret);
  };

  const clearRange = (): void => {
    const cur = stateRef.current;
    const s = cur.model.sheets[cur.model.active];
    if (!s) return;
    if (rangeLocked(rect)) {
      notifyProtected(refOf({ c: rect.c1, r: rect.r1 }));
      return;
    }
    const cells = { ...s.cells };
    let changed = false;
    for (let r = rect.r1; r <= rect.r2; r++) {
      for (let c = rect.c1; c <= rect.c2; c++) {
        const ref = refOf({ c, r });
        if (ref in cells) {
          delete cells[ref];
          changed = true;
        }
      }
    }
    if (changed) applyCells(cells);
  };

  /* ----- portapapeles: copiar/cortar/pegar (multi-celda, con ajuste de refs) ----- */

  /** Copia/corta la selección o pega desde el portapapeles del sistema. */
  const doClipboard = async (cmd: 'copy' | 'paste', value?: string): Promise<void> => {
    const cur = stateRef.current;
    const s = cur.model.sheets[cur.model.active];
    if (!s) return;
    if (cmd === 'copy') {
      const cut = value === 'cut';
      const r = rectOf(cur.sel, cur.ext);
      const { tsv, html } = buildClipboardPayload(s, r, cur.engine);
      clipRef.current = { cut, rect: r, sheet: s.name };
      try {
        await writeClipboard(tsv, html);
      } catch {
        clipRef.current = null;
        notify({
          tone: 'warning',
          title: t(cut ? 'sheet.msg.cutFailed' : 'sheet.msg.copyFailed'),
          description: t('sheet.msg.clipboardDenied'),
        });
      }
      return;
    }
    let payload: ClipPayload | null = null;
    try {
      payload = await readClipboard();
    } catch {
      payload = null;
    }
    if (!payload) {
      notify({
        tone: 'warning',
        title: t('sheet.msg.pasteFailed'),
        description: t('sheet.msg.clipboardEmpty'),
      });
      return;
    }
    applyPaste(payload, value === 'values');
  };

  /** Vuelca un ClipPayload anclado en la esquina superior-izquierda de la selección. */
  const applyPaste = (payload: ClipPayload, valuesOnly: boolean): void => {
    const cur = stateRef.current;
    const s = cur.model.sheets[cur.model.active];
    if (!s) return;
    const a = rectOf(cur.sel, cur.ext);
    const anchor = { c: a.c1, r: a.r1 };
    const cells = { ...s.cells };
    const formats = { ...s.formats };
    const styles = { ...s.styles };

    // Cortar = mover: se vacía el origen ANTES de escribir el destino (por si solapan).
    // Solo si el contenido pegado es NUESTRO (kind 'sytue'): si otra app pisó el
    // portapapeles entre el corte y el pegado, no se toca el origen (no perder datos).
    const clip = clipRef.current;
    if (clip?.cut && clip.sheet === s.name && payload.kind === 'sytue') {
      for (let r = clip.rect.r1; r <= clip.rect.r2; r++) {
        for (let c = clip.rect.c1; c <= clip.rect.c2; c++) {
          const ref = refOf({ c, r });
          delete cells[ref];
          delete formats[ref];
          delete styles[ref];
        }
      }
      clipRef.current = null;
    }

    let maxC = anchor.c;
    let maxR = anchor.r;
    const put = (
      dc: number,
      dr: number,
      rawHtml: string,
      fmt: NumFmt | null,
      style: CellStyle | null,
    ): void => {
      const c = anchor.c + dc;
      const r = anchor.r + dr;
      if (c >= MAX_COLS || r >= MAX_ROWS) return; // recorta lo que se sale de la rejilla
      maxC = Math.max(maxC, c);
      maxR = Math.max(maxR, r);
      const ref = refOf({ c, r });
      if (rawHtml === '') delete cells[ref];
      else cells[ref] = rawHtml;
      if (fmt) formats[ref] = fmt;
      else delete formats[ref];
      if (style) styles[ref] = style;
      else delete styles[ref];
    };

    if (payload.kind === 'sytue' && !valuesOnly) {
      const dc = anchor.c - payload.c0;
      const dr = anchor.r - payload.r0;
      payload.grid.forEach((row, ri) =>
        row.forEach((cell, ci) =>
          put(ci, ri, shiftCellHtml(cell.raw, dc, dr), cell.fmt, cell.style),
        ),
      );
    } else {
      // Solo valores: literales de texto (display de nuestra tabla o TSV externo).
      const grid =
        payload.kind === 'sytue'
          ? payload.grid.map((row) => row.map((c) => c.display))
          : payload.grid;
      grid.forEach((row, ri) =>
        row.forEach((text, ci) => put(ci, ri, text ? escapeHtml(text) : '', null, null)),
      );
    }

    applySheet({ cells, formats, styles });
    setSel(anchor);
    setExt(maxC === anchor.c && maxR === anchor.r ? null : { c: maxC, r: maxR });
    gridRef.current?.focus();
  };

  /* ----- insertar/borrar filas y columnas (lo que rebosa del límite de la hoja se pierde) ----- */

  const applyStructural = (cmd: SheetCommand, value?: string): void => {
    const cur = stateRef.current;
    const s = cur.model.sheets[cur.model.active];
    if (!s) return;
    const rc = rectOf(cur.sel, cur.ext);
    // Transforma la posición de una celda; null = se cae fuera de la rejilla.
    let mapPos: (c: number, r: number) => Pos | null;
    if (cmd === 'insrow') {
      const at = value === 'below' ? rc.r2 + 1 : rc.r1;
      mapPos = (c, r) => (r < at ? { c, r } : r + 1 < MAX_ROWS ? { c, r: r + 1 } : null);
    } else if (cmd === 'delrow') {
      const n = rc.r2 - rc.r1 + 1;
      mapPos = (c, r) => (r < rc.r1 ? { c, r } : r <= rc.r2 ? null : { c, r: r - n });
    } else if (cmd === 'inscol') {
      const at = value === 'right' ? rc.c2 + 1 : rc.c1;
      mapPos = (c, r) => (c < at ? { c, r } : c + 1 < MAX_COLS ? { c: c + 1, r } : null);
    } else {
      const n = rc.c2 - rc.c1 + 1;
      mapPos = (c, r) => (c < rc.c1 ? { c, r } : c <= rc.c2 ? null : { c: c - n, r });
    }
    const remap = <T,>(src: Record<string, T> | undefined): Record<string, T> => {
      const out: Record<string, T> = {};
      if (!src) return out;
      for (const ref in src) {
        const rr = parseRangeRef(ref);
        if (!rr) continue;
        const p = mapPos(rr.c1, rr.r1);
        if (p) out[refOf(p)] = src[ref]!;
      }
      return out;
    };
    // Qué le pasa a la rejilla, para que las FÓRMULAS lo sepan. Antes no se
    // reescribían: insertar una fila movía los datos y dejaba `=SUMA(A1:A5)`
    // sumando cinco filas cuando ya eran seis. Ni error ni aviso — solo un
    // resultado que pasaba a estar mal, que es la peor forma de fallar que tiene
    // una hoja de cálculo. Ver sheets/refshift.ts para las reglas.
    const desplazamiento: Desplazamiento =
      cmd === 'insrow'
        ? { eje: 'fila', desde: value === 'below' ? rc.r2 + 1 : rc.r1, cantidad: 1, tope: MAX_ROWS }
        : cmd === 'delrow'
          ? { eje: 'fila', desde: rc.r1, cantidad: -(rc.r2 - rc.r1 + 1), tope: MAX_ROWS }
          : cmd === 'inscol'
            ? {
                eje: 'columna',
                desde: value === 'right' ? rc.c2 + 1 : rc.c1,
                cantidad: 1,
                tope: MAX_COLS,
              }
            : { eje: 'columna', desde: rc.c1, cantidad: -(rc.c2 - rc.c1 + 1), tope: MAX_COLS };
    const celdas = remap(s.cells);
    for (const ref in celdas) celdas[ref] = reescribirCelda(celdas[ref]!, desplazamiento);
    // Los objetos de la hoja —tablas, minigráficos, gráficos, segmentaciones y el
    // autofiltro— siguen a sus celdas con las mismas reglas (refshift.ts).
    // ponytail: los grupos de esquema conservan su rango aunque las filas se desplacen.
    const next: SheetData = {
      ...moverObjetos(s, desplazamiento),
      cells: celdas,
      // La identidad de filas y columnas sigue a la estructura (sheets/anclas.ts).
      ...conEstructura(s, desplazamiento.eje, desplazamiento.desde, desplazamiento.cantidad),
    };
    vistas.mover(s.name, desplazamiento); // las vistas de filtro de cada persona, igual
    const formats = remap(s.formats);
    if (Object.keys(formats).length > 0) next.formats = formats;
    else delete next.formats;
    const styles = remap(s.styles);
    if (Object.keys(styles).length > 0) next.styles = styles;
    else delete next.styles;
    // Y los nombres definidos que apuntan a esta hoja, igual (sheets/nombres.ts).
    const first = cur.model.sheets[0]?.name ?? s.name;
    apply(
      withNames(
        {
          ...cur.model,
          sheets: cur.model.sheets.map((x, i) => (i === cur.model.active ? next : x)),
        },
        moverNombres(cur.model.names, s.name, first, desplazamiento),
      ),
    );
    setExt(null);
  };

  /* ----- Tablas con formato (ListObject): crear/quitar/estilo/totales -----
   * ponytail: al insertar/borrar filas o columnas (applyStructural) el rango de
   * la tabla NO se reajusta, igual que el autofiltro/gráficos; sincronizarlo es
   * un follow-up. Un solo autofiltro por hoja: crear una tabla con cabeceras lo
   * fija sobre ella (da los botones de filtro/orden gratis). */
  const runTableCommand = (cmd: SheetCommand, value?: string): void => {
    const cur = stateRef.current;
    const s = cur.model.sheets[cur.model.active];
    if (!s) return;
    const tablesOf = s.tables ?? [];
    const setSheet = (next: SheetData): void =>
      apply({
        ...cur.model,
        sheets: cur.model.sheets.map((x, i) => (i === cur.model.active ? next : x)),
      });
    /** Índice de la tabla que cubre el ancla (la última definida gana); -1 si ninguna. */
    const anchorIdx = (): number => {
      let idx = -1;
      tablesOf.forEach((t, i) => {
        const rr = parseRangeRef(t.range);
        if (
          rr &&
          cur.sel.c >= rr.c1 &&
          cur.sel.c <= rr.c2 &&
          cur.sel.r >= rr.r1 &&
          cur.sel.r <= rr.r2
        )
          idx = i;
      });
      return idx;
    };

    if (cmd === 'tableadd') {
      let req: Record<string, unknown> = {};
      try {
        req = JSON.parse(value ?? '{}') as Record<string, unknown>;
      } catch {
        return;
      }
      const rect = parseRangeRef(typeof req['range'] === 'string' ? req['range'] : '');
      if (!rect) return;
      if (rect.c1 >= MAX_COLS || rect.r1 >= MAX_ROWS) return; // ancla fuera de rejilla: no crear tabla fantasma
      const c2 = Math.min(rect.c2, MAX_COLS - 1);
      const r2 = Math.min(rect.r2, MAX_ROWS - 1);
      const headers = req['headers'] !== false;
      const range = `${refOf({ c: rect.c1, r: rect.r1 })}:${refOf({ c: c2, r: r2 })}`;
      const table: SheetTable = { name: uniqueTableName(tablesOf.map((t) => t.name)), range };
      if (typeof req['style'] === 'string') table.style = tableStyleOf(req['style']).id;
      if (!headers) table.headers = false;
      const clean = sanitizeTable(table);
      if (!clean) return;
      const cells = { ...s.cells };
      if (headers) {
        // Nombres de columna únicos en la cabecera (rellena vacíos, desambigua duplicados).
        const raw: string[] = [];
        for (let c = rect.c1; c <= c2; c++)
          raw.push(stripHtml(cells[refOf({ c, r: rect.r1 })] ?? ''));
        uniqueHeaderNames(raw).forEach((nm, i) => {
          const ref = refOf({ c: rect.c1 + i, r: rect.r1 });
          const p = parseCell(cells[ref] ?? ''); // conserva negrita/cursiva/alineación
          p.inner = escapeHtml(nm);
          cells[ref] = renderCellHtml(p);
        });
      }
      const next: SheetData = { ...s, cells, tables: [...tablesOf, clean] };
      if (headers) next.autofilter = { row: rect.r1, c1: rect.c1, c2 }; // botones de filtro/orden
      setSheet(next);
      setSel({ c: rect.c1, r: rect.r1 });
      setExt({ c: c2, r: r2 });
      notify({
        tone: 'info',
        title: t('sheet.msg.tableCreated'),
        description: t('sheet.msg.tableCreated.detail', { name: clean.name, range }),
      });
      return;
    }

    const idx = anchorIdx();

    if (cmd === 'tabledel') {
      if (idx < 0) return;
      const gone = tablesOf[idx]!;
      const next: SheetData = { ...s };
      const tables = tablesOf.filter((_, i) => i !== idx);
      if (tables.length > 0) next.tables = tables;
      else delete next.tables;
      // Quita el autofiltro si cubría exactamente la cabecera de esta tabla.
      const rr = parseRangeRef(gone.range);
      if (
        rr &&
        s.autofilter &&
        s.autofilter.row === rr.r1 &&
        s.autofilter.c1 === rr.c1 &&
        s.autofilter.c2 === rr.c2
      ) {
        delete next.autofilter;
      }
      // Si era una tabla de datos externos, quita también su definición de refresco
      // (borrar la tabla desvincula el origen; «Actualizar todo» no la resucita).
      if (s.externalTables?.some((e) => e.table === gone.name)) {
        const ext = s.externalTables.filter((e) => e.table !== gone.name);
        if (ext.length > 0) next.externalTables = ext;
        else delete next.externalTables;
      }
      setSheet(next);
      return;
    }

    if (cmd === 'tablestyle') {
      if (idx < 0) return;
      const style = tableStyleOf(value).id;
      applySheet({ tables: tablesOf.map((t, i) => (i === idx ? { ...t, style } : t)) });
      return;
    }

    if (cmd === 'tabletotals') {
      if (idx < 0) return;
      const t0 = tablesOf[idx]!;
      const rr = parseRangeRef(t0.range);
      if (!rr) return;
      const turnOn = t0.totalsRow !== true;
      if (turnOn && rr.r2 + 1 >= MAX_ROWS) {
        notify({
          tone: 'warning',
          title: t('sheet.msg.noRoom'),
          description: t('sheet.msg.noRoomTotals'),
        });
        return;
      }
      // `tabla`, no `t`: en este componente `t` es el traductor, y la variable
      // local lo sombreaba dentro de todo el bloque.
      const tabla: SheetTable = { ...t0 };
      if (turnOn) {
        tabla.totalsRow = true;
        tabla.range = `${refOf({ c: rr.c1, r: rr.r1 })}:${refOf({ c: rr.c2, r: rr.r2 + 1 })}`;
        tabla.totals = { ...t0.totals, [String(rr.c2 - rr.c1)]: 'sum' }; // última columna: suma (Excel)
      } else {
        delete tabla.totalsRow;
        delete tabla.totals;
        tabla.range = `${refOf({ c: rr.c1, r: rr.r1 })}:${refOf({ c: rr.c2, r: rr.r2 - 1 })}`;
      }
      applySheet({ tables: tablesOf.map((x, i) => (i === idx ? tabla : x)) });
      return;
    }

    if (cmd === 'tabletotalfn') {
      let req: Record<string, unknown> = {};
      try {
        req = JSON.parse(value ?? '{}') as Record<string, unknown>;
      } catch {
        return;
      }
      const name = typeof req['name'] === 'string' ? req['name'] : '';
      const col = Math.trunc(Number(req['col']));
      const fn = req['fn'];
      const ti = tablesOf.findIndex((t) => t.name === name);
      const valid: readonly TableTotalFn[] = [
        'none',
        'sum',
        'average',
        'count',
        'counta',
        'min',
        'max',
      ];
      if (ti < 0 || !Number.isInteger(col) || col < 0) return;
      if (typeof fn !== 'string' || !valid.includes(fn as TableTotalFn)) return;
      const t: SheetTable = { ...tablesOf[ti]! };
      const totals = { ...t.totals };
      if (fn === 'none') delete totals[String(col)];
      else totals[String(col)] = fn as TableTotalFn;
      if (Object.keys(totals).length > 0) t.totals = totals;
      else delete t.totals;
      applySheet({ tables: tablesOf.map((x, i) => (i === ti ? t : x)) });
      return;
    }
  };

  /* ----- validación de datos: desplegable de lista sobre la celda activa ----- */

  const validRef = useRef<HTMLDivElement>(null);
  /** Desplegable de autofiltro abierto (encabezado de columna); patrón del de validación. */
  const filterRef = useRef<HTMLDivElement>(null);

  /** Escribe en la celda activa un valor elegido del desplegable de validación. */
  const pickValue = (val: string): void => {
    const cur = stateRef.current;
    const s = cur.model.sheets[cur.model.active];
    setValidOpen(false);
    if (!s) return;
    if (!canEditAt(cur.sel)) {
      notifyProtected(refOf(cur.sel));
      return;
    }
    const ref = refOf(cur.sel);
    const p = parseCell(s.cells[ref] ?? ''); // conserva negrita/cursiva/alineación
    p.inner = escapeHtml(val);
    const html = renderCellHtml(p);
    const cells = { ...s.cells };
    if (html === '') delete cells[ref];
    else cells[ref] = html;
    applyCells(cells);
    gridRef.current?.focus();
  };

  // Cambiar de celda/hoja cierra el desplegable (el teclado no dispara clic fuera).
  useEffect(() => setValidOpen(false), [sel.c, sel.r, model.active]);

  // Cursor remoto: publica la selección local (hoja + rango) a los peers. El
  // transporte agrupa por tiempo (throttle) y degrada sin WS. sendCursor es estable.
  const sendCursor = ctx.sendCursor;
  const hojaActiva = model.sheets[model.active];
  const sheetsRef = useRef(model.sheets);
  sheetsRef.current = model.sheets;
  useEffect(() => {
    const rc = rectOf(sel, ext);
    sendCursor(anclarPosHoja(sheetsRef.current, model.active, rc));
    // Se reenvía si cambia la selección o la estructura de la hoja (insertar o borrar filas
    // propias mueve el sitio al que apunta la selección); lo ajeno lo siguen las anclas.
  }, [sel, ext, model.active, sendCursor, hojaActiva?.rowIds, hojaActiva?.colIds]);

  // Toque/clic fuera y Escape cierran el desplegable (patrón de los popover del
  // ribbon). Es `pointerdown` y no `mousedown` porque un móvil solo sintetiza
  // eventos de ratón sobre elementos «clicables»: tocando la rejilla, el
  // desplegable se quedaba abierto tapando la celda que se acababa de elegir.
  useEffect(() => {
    if (!validOpen) return;
    const onDown = (e: Event): void => {
      if (!validRef.current?.contains(e.target as Node)) setValidOpen(false);
    };
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') setValidOpen(false);
    };
    document.addEventListener('pointerdown', onDown);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('pointerdown', onDown);
      document.removeEventListener('keydown', onKey);
    };
  }, [validOpen]);

  // Toque/clic fuera y Escape cierran el desplegable de autofiltro abierto.
  useEffect(() => {
    if (filterCol === null) {
      setCondOpen(false);
      return;
    }
    const onDown = (e: Event): void => {
      if (!filterRef.current?.contains(e.target as Node)) setFilterCol(null);
    };
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') setFilterCol(null);
    };
    document.addEventListener('pointerdown', onDown);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('pointerdown', onDown);
      document.removeEventListener('keydown', onKey);
    };
  }, [filterCol]);

  /* ----- autorelleno: asa en la esquina de la selección (Pointer Events) ----- */

  /** Celda bajo un punto de pantalla, por geometría (la rejilla es de paso fijo;
   *  elementFromPoint devolvería el propio asa capturada). Corrige el zoom CSS
   *  y salta las filas ocultas por el filtro. */
  const posFromPoint = (x: number, y: number): Pos | null => {
    const grid = gridRef.current;
    if (!grid) return null;
    const cur = stateRef.current;
    const vis = cur.visRows;
    const visC = cur.visCols;
    if (vis.length === 0 || visC.length === 0) return null;
    const b = grid.getBoundingClientRect(); // ya escalado por el zoom CSS
    const z = cur.ctx.ui.zoom / 100;
    // Los carriles del canalón (OX/OY) desplazan el origen; las filas/columnas
    // ocultas por filtro o esquema no ocupan sitio → se mapea por índice visible.
    const px = (x - b.left) / z;
    const py = (y - b.top) / z;
    const cs = hayAnchos
      ? indiceEn(
          visC.map((c) => cur.colX[c] ?? 0),
          px,
        )
      : clamp(Math.floor((px - cur.ox) / CELL_W), 0, visC.length - 1);
    const i = cur.rowTop
      ? indiceEn(cur.rowTop, py)
      : clamp(Math.floor((py - cur.oy) / CELL_H), 0, vis.length - 1);
    return { c: visC[cs] ?? 0, r: vis[i] ?? 0 };
  };

  const sameRect = (a: Rect | null, b: Rect | null): boolean =>
    a === b || (!!a && !!b && a.c1 === b.c1 && a.c2 === b.c2 && a.r1 === b.r1 && a.r2 === b.r2);

  const onFillDown = (e: PointerEvent): void => {
    e.preventDefault(); // sin foco ni mousedown de la rejilla
    e.stopPropagation();
    const cur = stateRef.current;
    fillDragRef.current = { src: rectOf(cur.sel, cur.ext), rect: null };
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
  };

  const onFillMove = (e: PointerEvent): void => {
    const drag = fillDragRef.current;
    if (!drag) return;
    const pos = posFromPoint(e.clientX, e.clientY);
    if (!pos) return;
    const s = drag.src;
    // Una sola dirección dominante (criterio Excel); vertical tiene prioridad.
    let next: Rect | null = null;
    if (pos.r > s.r2) next = { c1: s.c1, c2: s.c2, r1: s.r2 + 1, r2: pos.r };
    else if (pos.r < s.r1) next = { c1: s.c1, c2: s.c2, r1: pos.r, r2: s.r1 - 1 };
    else if (pos.c > s.c2) next = { r1: s.r1, r2: s.r2, c1: s.c2 + 1, c2: pos.c };
    else if (pos.c < s.c1) next = { r1: s.r1, r2: s.r2, c1: pos.c, c2: s.c1 - 1 };
    if (!sameRect(drag.rect, next)) {
      drag.rect = next;
      setFillRect(next);
    }
  };

  const onFillCancel = (): void => {
    fillDragRef.current = null;
    setFillRect(null);
  };

  const onFillUp = (): void => {
    const drag = fillDragRef.current;
    onFillCancel();
    if (drag?.rect) applyFill(drag.src, drag.rect);
  };

  /** Copia el bloque origen sobre el destino en mosaico ajustando las
   *  referencias RELATIVAS de las fórmulas (shiftRefs); literales, formato de
   *  celda (HTML) y formato de número viajan tal cual. */
  const applyFill = (src: Rect, target: Rect): void => {
    const m = stateRef.current.model;
    const s = m.sheets[m.active];
    if (!s) return;
    if (rangeLocked(target)) {
      notifyProtected(refOf({ c: target.c1, r: target.r1 }));
      return;
    }
    const nC = src.c2 - src.c1 + 1;
    const nR = src.r2 - src.r1 + 1;
    const cells = { ...s.cells };
    const formats = { ...s.formats };
    const styles = { ...s.styles };
    for (let r = target.r1; r <= target.r2; r++) {
      for (let c = target.c1; c <= target.c2; c++) {
        let sc = c;
        let sr = r;
        if (target.r1 > src.r2)
          sr = src.r1 + ((r - src.r1) % nR); // hacia abajo
        else if (target.r2 < src.r1)
          sr = src.r2 - ((src.r2 - r) % nR); // hacia arriba
        else if (target.c1 > src.c2)
          sc = src.c1 + ((c - src.c1) % nC); // derecha
        else sc = src.c2 - ((src.c2 - c) % nC); // izquierda
        const from = refOf({ c: sc, r: sr });
        const to = refOf({ c, r });
        const html = s.cells[from] ?? '';
        // shiftCellHtml ajusta refs si es fórmula (conservando negrita/cursiva/alineación); literal tal cual.
        if (html === '') delete cells[to];
        else cells[to] = shiftCellHtml(html, c - sc, r - sr);
        const f = s.formats?.[from];
        if (f) formats[to] = f;
        else delete formats[to];
        const st = s.styles?.[from];
        if (st) styles[to] = st;
        else delete styles[to];
      }
    }
    applySheet({ cells, formats, styles });
    // La selección pasa a cubrir origen + relleno (criterio Excel).
    setSel({ c: Math.min(src.c1, target.c1), r: Math.min(src.r1, target.r1) });
    setExt({ c: Math.max(src.c2, target.c2), r: Math.max(src.r2, target.r2) });
  };

  /* ----- registro de comandos de la pestaña InicioSheets ----- */

  // El registro global se instala una sola vez, pero cada invocación debe usar
  // las operaciones de la renderización más reciente.
  const commandActionsRef = useUltimo({
    apply,
    applyCells,
    applySheet,
    applyStructural,
    doClipboard,
    endFx,
    notify,
    notifyProtected,
    notifyPivotFailed,
    runTableCommand,
  });

  useEffect(() => {
    const run = (cmd: SheetCommand, value?: string): void => {
      const {
        apply,
        applyCells,
        applySheet,
        applyStructural,
        doClipboard,
        endFx,
        notify,
        notifyProtected,
        notifyPivotFailed,
        runTableCommand,
      } = commandActionsRef.current;
      const cur = stateRef.current;
      // Con el caret dentro de la celda, negrita/cursiva son formato en línea (§3 vía exec).
      if ((cmd === 'bold' || cmd === 'italic') && cur.editing) {
        cur.ctx.exec(cmd);
        return;
      }
      const s = cur.model.sheets[cur.model.active];
      if (!s) return;

      /* --- Protección de hoja: proteger/desproteger + VETO de ediciones de celda --- */
      if (cmd === 'protect') {
        let req: Record<string, unknown> = {};
        try {
          req = JSON.parse(value ?? '{}') as Record<string, unknown>;
        } catch {
          return;
        }
        const m = cur.model;
        // La contraseña NO entra en el modelo: va al servidor aparte (state/proteccion.ts),
        // que guarda solo su hash y es quien decide quién puede quitar la protección.
        const clave = typeof req['password'] === 'string' ? req['password'] : '';
        const prot = sanitizeProtection(req) ?? {};
        if (clave !== '') {
          prot.hasPassword = true;
          presentarNueva(ambitoHoja(s.name), clave);
        }
        const next: SheetData = { ...s, protect: prot };
        apply({ ...m, sheets: m.sheets.map((x, i) => (i === m.active ? next : x)) });
        setEditing(false); // cierra cualquier edición en curso
        endFx();
        return;
      }
      if (cmd === 'unprotect') {
        if (!s.protect) return;
        // Sin contraseña, solo el propietario la quita; con ella, quien la sepa. Lo decide
        // el servidor (esto solo ahorra el viaje): la contraseña viaja con el guardado.
        if (!s.protect.hasPassword && !esPropietario()) {
          notify({
            tone: 'warning',
            title: t('sheet.msg.protectedSheet'),
            description: t('sheet.msg.protectionDenied'),
          });
          return;
        }
        presentarClave(value ?? '');
        const m = cur.model;
        const next: SheetData = { ...s };
        delete next.protect;
        apply({ ...m, sheets: m.sheets.map((x, i) => (i === m.active ? next : x)) });
        return;
      }

      /* --- Protección de ESTRUCTURA del libro: proteger/desproteger (el VETO de
       *     añadir/borrar/renombrar/reordenar hojas vive en los mutadores). --- */
      if (cmd === 'protectbook') {
        let req: Record<string, unknown> = {};
        try {
          req = JSON.parse(value ?? '{}') as Record<string, unknown>;
        } catch {
          return;
        }
        const clave = typeof req['password'] === 'string' ? req['password'] : '';
        const prot = sanitizeWorkbookProtection(req) ?? {};
        if (clave !== '') {
          prot.hasPassword = true;
          presentarNueva(AMBITO_LIBRO, clave);
        }
        apply({ ...cur.model, protect: prot });
        return;
      }
      if (cmd === 'unprotectbook') {
        const m = cur.model;
        if (!m.protect) return;
        if (!m.protect.hasPassword && !esPropietario()) {
          notify({
            tone: 'warning',
            title: t('sheet.msg.protectedBook'),
            description: t('sheet.msg.protectionDenied'),
          });
          return;
        }
        presentarClave(value ?? '');
        const next = { ...m };
        delete next.protect;
        apply(next);
        return;
      }

      /* --- Nombres definidos: del libro, no de la hoja (la protección la mira
       *     applyNameCommand, que no reescribe fórmulas de una hoja protegida). --- */
      if (cmd === 'nameset' || cmd === 'namedel') {
        const res = applyNameCommand(cur.model, cmd, value);
        if (!res) return;
        if ('error' in res) {
          notify({
            tone: 'warning',
            title: t('names.title'),
            description: t(`names.error.${res.error}`),
          });
          return;
        }
        apply(res.model);
        return;
      }

      // Hoja protegida: veta contenido/estructura; el formato solo si `formatCells`;
      // y las acciones con permiso propio (insertar/borrar filas-columnas, ordenar,
      // autofiltro, objetos, escenarios) solo si ese permiso está concedido.
      const perm = PROTECT_PERMISSION_CMDS.get(cmd);
      if (
        s.protect &&
        (PROTECT_CONTENT_CMDS.has(cmd) ||
          (PROTECT_FORMAT_CMDS.has(cmd) && s.protect.formatCells !== true) ||
          (perm !== undefined && !isPermissionGranted(s.protect, perm)))
      ) {
        notifyProtected(refOf(cur.sel));
        return;
      }

      const range = rectOf(cur.sel, cur.ext);
      const eachRef = (fn: (ref: string) => void): void => {
        for (let r = range.r1; r <= range.r2; r++) {
          for (let c = range.c1; c <= range.c2; c++) fn(refOf({ c, r }));
        }
      };

      /* --- Formato de número y reglas condicionales: campos aparte del modelo
       *     (nunca reescriben cells: el texto crudo es la fuente de verdad). --- */
      if (cmd === 'numfmt') {
        // Máscara personalizada: value = «custom:<máscara>»; el resto son presets.
        const mask = value?.startsWith('custom:') ? value.slice(7) : null;
        const formats = { ...s.formats };
        if (mask !== null) {
          eachRef((ref) => {
            if (mask.trim() === '')
              delete formats[ref]; // máscara vacía → General
            else formats[ref] = { id: 'custom', mask };
          });
        } else {
          const id = NUMFMT_IDS.find((n) => n === value && n !== 'custom') ?? 'general';
          eachRef((ref) => {
            if (id === 'general') delete formats[ref];
            else formats[ref] = { id };
          });
        }
        applySheet({ formats });
        return;
      }
      if (cmd === 'decimals') {
        const delta = value === '-1' ? -1 : 1;
        const formats = { ...s.formats };
        eachRef((ref) => {
          const f = formats[ref];
          const base: NumFmt = !f || f.id === 'general' ? { id: 'number' } : f;
          if (base.id === 'date' || base.id === 'custom') return; // decimales sin sentido (fecha/máscara)
          formats[ref] = {
            id: base.id,
            decimals: clamp((base.decimals ?? defaultDecimals(base.id)) + delta, 0, 8),
          };
        });
        applySheet({ formats });
        return;
      }

      /* --- Formato VISUAL de celda (fuente, color, relleno, bordes, subrayado…): mapa `styles` aparte. --- */
      if (cmd === 'style' || cmd === 'clearstyle' || cmd === 'border') {
        const styles = { ...s.styles };
        const setStyle = (ref: string, patch: Partial<Record<keyof CellStyle, unknown>>): void => {
          const merged = mergeCellStyle(styles[ref], patch);
          if (merged) styles[ref] = merged;
          else delete styles[ref];
        };
        if (cmd === 'clearstyle') {
          eachRef((ref) => delete styles[ref]);
        } else if (cmd === 'style') {
          let patch: unknown = {};
          try {
            patch = JSON.parse(value ?? '{}');
          } catch {
            /* parche malformado → no-op */
          }
          if (!patch || typeof patch !== 'object') return;
          eachRef((ref) => setStyle(ref, patch));
        } else {
          // border: caja/contorno/inferior/quitar sobre la posición de cada celda.
          const mode = value === 'all' || value === 'outer' || value === 'bottom' ? value : 'none';
          for (let r = range.r1; r <= range.r2; r++) {
            for (let c = range.c1; c <= range.c2; c++) {
              const ref = refOf({ c, r });
              if (mode === 'all') setStyle(ref, { border: { t: true, r: true, b: true, l: true } });
              else if (mode === 'bottom') setStyle(ref, { border: { b: true } });
              else if (mode === 'none') setStyle(ref, { border: null });
              else {
                const b: CellBorder = {};
                if (r === range.r1) b.t = true;
                if (r === range.r2) b.b = true;
                if (c === range.c1) b.l = true;
                if (c === range.c2) b.r = true;
                if (b.t || b.r || b.b || b.l) setStyle(ref, { border: b }); // solo los bordes de la caja
              }
            }
          }
        }
        applySheet({ styles });
        return;
      }
      if (cmd === 'copy' || cmd === 'paste') {
        void doClipboard(cmd, value);
        return;
      }
      if (cmd === 'condadd' || cmd === 'conddel') {
        const rules = s.condFormats ?? [];
        if (cmd === 'condadd') {
          let parsed: unknown = null;
          try {
            parsed = JSON.parse(value ?? '');
          } catch {
            /* regla malformada → no-op */
          }
          const rule = sanitizeCondRule(parsed);
          if (rule) applySheet({ condFormats: [...rules, rule] });
        } else {
          const i = Number(value);
          if (Number.isInteger(i) && i >= 0 && i < rules.length) {
            applySheet({ condFormats: rules.filter((_, j) => j !== i) });
          }
        }
        return;
      }
      if (cmd === 'condmove') {
        const rules = s.condFormats ?? [];
        const [iRaw, dRaw] = (value ?? '').split(',');
        const i = Number(iRaw);
        const j = i + (Number(dRaw) < 0 ? -1 : 1);
        if (Number.isInteger(i) && i >= 0 && i < rules.length && j >= 0 && j < rules.length) {
          const next = [...rules];
          [next[i], next[j]] = [next[j]!, next[i]!]; // intercambia prioridad
          applySheet({ condFormats: next });
        }
        return;
      }
      if (cmd === 'validadd' || cmd === 'validdel') {
        const list = s.validations ?? [];
        if (cmd === 'validadd') {
          let parsed: unknown = null;
          try {
            parsed = JSON.parse(value ?? '');
          } catch {
            /* validación malformada → no-op */
          }
          const val = sanitizeDataValidation(parsed);
          if (val) applySheet({ validations: [...list, val] });
        } else {
          const i = Number(value);
          if (Number.isInteger(i) && i >= 0 && i < list.length) {
            applySheet({ validations: list.filter((_, j) => j !== i) });
          }
        }
        return;
      }

      /* --- Fase Datos: congelar paneles, ordenar y filtro efímero --- */
      if (cmd === 'freeze') {
        const [c = 0, r = 0] = (value ?? '').split(',').map((n) => Math.trunc(Number(n) || 0));
        const cols = clamp(c, 0, MAX_COLS - 1);
        const rows = clamp(r, 0, MAX_ROWS - 1);
        const m = cur.model;
        const next: SheetData = { ...s, freeze: { cols, rows } };
        if (cols === 0 && rows === 0) delete next.freeze; // no ensuciar el JSON persistido
        apply({ ...m, sheets: m.sheets.map((x, i) => (i === m.active ? next : x)) });
        return;
      }
      if (cmd === 'sort' || cmd === 'filtersort') {
        let dir: 1 | -1 = value === 'desc' ? -1 : 1;
        let rc = rectOf(cur.sel, cur.ext);
        let keyCol = cur.sel.c;
        if (cmd === 'filtersort') {
          // El «Ordenar» del desplegable: el cuerpo del autofiltro (`autofilterBody`) por
          // su columna. Se ordenan también las filas ocultas: el filtro va por valor y
          // siguen ocultas donde caigan.
          const af = s.autofilter;
          let req: { col?: unknown; dir?: unknown } = {};
          try {
            req = JSON.parse(value ?? '{}') as { col?: unknown; dir?: unknown };
          } catch {
            return;
          }
          const col = Number(req.col);
          if (!af || !Number.isInteger(col) || col < af.c1 || col > af.c2) return;
          dir = req.dir === 'desc' ? -1 : 1;
          keyCol = col;
          const body = autofilterBody(compileTables(s.tables ?? []), af, Object.keys(s.cells));
          if (body.r2 <= body.r1) return; // una fila o ninguna
          rc = { c1: af.c1, c2: af.c2, ...body };
        } else if (rc.c1 === rc.c2 && rc.r1 === rc.r2) {
          // Celda suelta: se ordena el rectángulo usado completo de la hoja.
          let c1 = MAX_COLS;
          let r1 = MAX_ROWS;
          let c2 = -1;
          let r2 = -1;
          for (const ref in s.cells) {
            const rr = parseRangeRef(ref);
            if (!rr) continue;
            c1 = Math.min(c1, rr.c1);
            r1 = Math.min(r1, rr.r1);
            c2 = Math.max(c2, rr.c1);
            r2 = Math.max(r2, rr.r1);
          }
          if (c2 < 0 || r1 === r2) return; // hoja vacía o una sola fila
          rc = { c1, c2, r1, r2 };
        }
        keyCol = clamp(keyCol, rc.c1, rc.c2);
        const keyVal = (r: number) => cur.engine.getValue(s.name, refOf({ c: keyCol, r }));
        const order: number[] = [];
        for (let r = rc.r1; r <= rc.r2; r++) order.push(r);
        order.sort((a, b) => sortCellCompare(keyVal(a), keyVal(b), dir)); // sort estable (ES2019)
        if (order.every((r, i) => r === rc.r1 + i)) return; // ya está ordenado
        const cells = { ...s.cells };
        const formats = { ...s.formats };
        const styles = { ...s.styles };
        for (let i = 0; i < order.length; i++) {
          const from = order[i]!;
          const to = rc.r1 + i;
          for (let c = rc.c1; c <= rc.c2; c++) {
            const fromRef = refOf({ c, r: from });
            const toRef = refOf({ c, r: to });
            const html = s.cells[fromRef];
            if (html !== undefined) cells[toRef] = html;
            else delete cells[toRef];
            const f = s.formats?.[fromRef];
            if (f) formats[toRef] = f;
            else delete formats[toRef];
            const st = s.styles?.[fromRef];
            if (st) styles[toRef] = st;
            else delete styles[toRef];
          }
        }
        // ponytail: las fórmulas se mueven con su texto tal cual (sin reescribir refs), como Excel.
        applySheet({ cells, formats, styles });
        return;
      }

      /* --- Autofiltro persistente: activar/desactivar y excluir valores por columna. --- */
      if (cmd === 'autofilter') {
        if (s.autofilter) {
          const next: SheetData = { ...s };
          delete next.autofilter;
          apply({
            ...cur.model,
            sheets: cur.model.sheets.map((x, i) => (i === cur.model.active ? next : x)),
          });
          return;
        }
        // Rango del ancla (o el rectángulo usado de la hoja); su fila superior lleva los desplegables.
        let rc = rectOf(cur.sel, cur.ext);
        if (rc.c1 === rc.c2 && rc.r1 === rc.r2) {
          let c1 = MAX_COLS;
          let r1 = MAX_ROWS;
          let c2 = -1;
          let r2 = -1;
          for (const ref in s.cells) {
            const rr = parseRangeRef(ref);
            if (!rr) continue;
            c1 = Math.min(c1, rr.c1);
            r1 = Math.min(r1, rr.r1);
            c2 = Math.max(c2, rr.c1);
            r2 = Math.max(r2, rr.r1);
          }
          if (c2 < 0) return; // hoja vacía: nada que filtrar
          rc = { c1, c2, r1, r2 };
        }
        applySheet({ autofilter: { row: rc.r1, c1: rc.c1, c2: rc.c2 } });
        return;
      }
      if (cmd === 'filterset' || cmd === 'filtercond') {
        const af = s.autofilter;
        if (!af) return;
        let req: { col?: unknown; value?: unknown; hidden?: unknown; cond?: unknown } = {};
        try {
          req = JSON.parse(value ?? '{}') as typeof req;
        } catch {
          return;
        }
        const col = Number(req.col);
        if (!Number.isInteger(col) || col < af.c1 || col > af.c2) return;
        const cond = req.cond === null ? null : sanitizeFilterCond(req.cond);
        if (cmd === 'filtercond' && req.cond !== null && !cond) return; // condición inválida: no se toca nada
        const crit =
          cmd === 'filtercond'
            ? setCond(af, col, cond)
            : toggleValue(af, col, typeof req.value === 'string' ? req.value : '', !!req.hidden);
        applySheet({ autofilter: withCriterios(af, crit) });
        return;
      }

      /* --- Herramientas de datos: quitar duplicados, texto en columnas, insertar/borrar fila y columna. --- */
      if (cmd === 'dedupe') {
        const rc = rectOf(cur.sel, cur.ext);
        if (rc.r1 === rc.r2) return; // una sola fila: nada que deduplicar
        const keyOf = (r: number): string => {
          const parts: string[] = [];
          for (let c = rc.c1; c <= rc.c2; c++) parts.push(cellText(cur.engine, s.name, c, r));
          return parts.join('\0');
        };
        const keys: string[] = [];
        for (let r = rc.r1; r <= rc.r2; r++) keys.push(keyOf(r));
        const keep = dedupeRowIndices(keys); // índices 0-based dentro del bloque
        if (keep.length === keys.length) return; // sin duplicados
        const cells = { ...s.cells };
        const formats = { ...s.formats };
        const styles = { ...s.styles };
        keep.forEach((srcIdx, i) => {
          const from = rc.r1 + srcIdx;
          const to = rc.r1 + i;
          for (let c = rc.c1; c <= rc.c2; c++) {
            const fr = refOf({ c, r: from });
            const tr = refOf({ c, r: to });
            if (s.cells[fr] !== undefined) cells[tr] = s.cells[fr]!;
            else delete cells[tr];
            if (s.formats?.[fr]) formats[tr] = s.formats[fr]!;
            else delete formats[tr];
            if (s.styles?.[fr]) styles[tr] = s.styles[fr]!;
            else delete styles[tr];
          }
        });
        for (let to = rc.r1 + keep.length; to <= rc.r2; to++) {
          for (let c = rc.c1; c <= rc.c2; c++) {
            const tr = refOf({ c, r: to });
            delete cells[tr];
            delete formats[tr];
            delete styles[tr];
          }
        }
        applySheet({ cells, formats, styles });
        setSel({ c: rc.c1, r: rc.r1 });
        setExt({ c: rc.c2, r: rc.r1 + keep.length - 1 });
        notify({
          tone: 'info',
          title: t('sheet.msg.dupesRemoved'),
          description: t('sheet.msg.dupesRemoved.detail', { count: keys.length - keep.length }),
        });
        return;
      }
      if (cmd === 'text2cols') {
        const delim = value && value.length > 0 ? value : '\t';
        const cells = { ...s.cells };
        let changed = false;
        for (let r = range.r1; r <= range.r2; r++) {
          const srcRef = refOf({ c: range.c1, r });
          const text = stripHtml(cells[srcRef] ?? '');
          if (text === '' || isFormula(cells[srcRef] ?? '')) continue; // no trocea fórmulas
          const parts = text.split(delim);
          if (parts.length <= 1) continue;
          parts.forEach((part, i) => {
            const c = range.c1 + i;
            if (c >= MAX_COLS) return; // recorta más allá de la rejilla
            const ref = refOf({ c, r });
            const t = part.trim();
            if (t === '') delete cells[ref];
            else cells[ref] = escapeHtml(t);
          });
          changed = true;
        }
        if (changed) applyCells(cells);
        return;
      }
      if (cmd === 'insrow' || cmd === 'delrow' || cmd === 'inscol' || cmd === 'delcol') {
        applyStructural(cmd, value);
        return;
      }

      /* --- Fase Visual: insertar gráfico (la pestaña Datos manda {type, range}) --- */
      if (cmd === 'chartadd') {
        let parsed: unknown = null;
        try {
          parsed = JSON.parse(value ?? '');
        } catch {
          /* petición malformada → no-op */
        }
        if (!parsed || typeof parsed !== 'object') return;
        const n = s.charts?.length ?? 0;
        const spec = sanitizeChartSpec({
          ...(parsed as Record<string, unknown>),
          id: crypto.randomUUID(),
          // Cascada para que las tarjetas nuevas no se apilen exactas.
          x: 96 + (n % 8) * 28,
          y: 48 + (n % 8) * 28,
        });
        if (spec) applySheet({ charts: [...(s.charts ?? []), spec] });
        return;
      }

      /* --- Segmentación de datos (slicer): reutiliza/crea el autofiltro de la hoja
       *     y añade una tarjeta flotante para la columna elegida. La selección vive
       *     en el `excl` del autofiltro (una fuente de verdad; sin desincronía con el
       *     desplegable nativo). Un autofiltro por hoja, como las tablas. --- */
      if (cmd === 'sliceradd') {
        let req: Record<string, unknown> = {};
        try {
          req = JSON.parse(value ?? '{}') as Record<string, unknown>;
        } catch {
          return;
        }
        const rect = parseRangeRef(typeof req['range'] === 'string' ? req['range'] : '');
        const col = Math.trunc(Number(req['col']));
        const list = s.slicers ?? [];
        const n = list.length;
        // Cascada para que las tarjetas nuevas no se apilen exactas (como los gráficos).
        const cascada = { x: 96 + (n % 8) * 24, y: 48 + (n % 8) * 24 };
        const pvName = typeof req['pivot'] === 'string' ? req['pivot'] : '';
        if (pvName !== '') {
          // De una tabla dinámica: `col` es un campo de su origen, esté o no en la tabla
          // (como Excel); no toca el autofiltro.
          const pv = s.pivots?.find((p) => p.name === pvName);
          const origen = pv
            ? pivotSource(cur.model, cur.engine, cur.model.active, pv, pivotLabels(t))
            : null;
          const ancho = origen?.grid[0]?.length ?? 0;
          if (!pv || !Number.isInteger(col) || col < 0 || col >= ancho) return;
          if (list.some((sl) => sl.pivot === pvName && sl.col === col)) {
            notify({ tone: 'info', title: t('sheet.msg.slicerExists') });
            return;
          }
          const slicer = sanitizeSlicer({
            id: crypto.randomUUID(),
            col,
            pivot: pvName,
            ...cascada,
          });
          if (slicer) applySheet({ slicers: [...list, slicer] });
          return;
        }
        if (!rect || !Number.isInteger(col) || col < 0 || col >= MAX_COLS) return;
        if (list.some((sl) => !sl.pivot && sl.col === col)) {
          notify({
            tone: 'info',
            title: t('sheet.msg.slicerExists'),
            description: t('sheet.msg.slicerExists.detail', { col: colName(col) }),
          });
          return;
        }
        // Autofiltro que cubra la columna: reutiliza el de la hoja o lo crea sobre
        // el rango; extiende sus columnas si el campo cae fuera (conserva excl y fila).
        const af = s.autofilter;
        const nextAf: SheetAutoFilter = {
          row: af?.row ?? rect.r1,
          c1: clamp(Math.min(af?.c1 ?? rect.c1, col), 0, MAX_COLS - 1),
          c2: clamp(Math.max(af?.c2 ?? rect.c2, col), 0, MAX_COLS - 1),
        };
        if (af?.excl) nextAf.excl = af.excl;
        if (af?.cond) nextAf.cond = af.cond;
        const slicer = sanitizeSlicer({ id: crypto.randomUUID(), col, ...cascada });
        if (!slicer) return;
        applySheet({ autofilter: nextAf, slicers: [...list, slicer] });
        return;
      }

      /* --- Sparklines: añadir/quitar (celda anfitriona = ancla por defecto) --- */
      if (cmd === 'sparkadd') {
        let parsed: Record<string, unknown> = {};
        try {
          parsed = JSON.parse(value ?? '{}') as Record<string, unknown>;
        } catch {
          return;
        }
        const sp = sanitizeSparkline({ cell: refOf(cur.sel), ...parsed });
        if (!sp) return;
        const list = (s.sparklines ?? []).filter((x) => x.cell !== sp.cell); // una por celda
        applySheet({ sparklines: [...list, sp] });
        return;
      }
      if (cmd === 'sparkdel') {
        const list = s.sparklines ?? [];
        const next = list.filter((x) => x.cell !== value);
        if (next.length === list.length) return;
        const patch: SheetData = { ...s, sparklines: next };
        if (next.length === 0) delete patch.sparklines; // no ensuciar el JSON persistido
        apply({
          ...cur.model,
          sheets: cur.model.sheets.map((x, i) => (i === cur.model.active ? patch : x)),
        });
        return;
      }

      /* --- Tabla dinámica: lee el origen del motor, agrega y VUELCA el resumen --- */
      /* --- Tabla dinámica: crear o cambiar (`pivot`), recalcular (`pivotrefresh`) y
       *     quitar (`pivotdel`). El cálculo es `computePivotTable` (pivot.ts), el mismo
       *     que la app nativa; aquí solo se escribe y se guarda la definición. --- */
      if (cmd === 'pivot' || cmd === 'pivotrefresh' || cmd === 'pivotdel') {
        const m = cur.model;
        const list = s.pivots ?? [];
        const labels = pivotLabels(t);
        let targets: SheetPivot[];
        if (cmd === 'pivot') {
          let raw: unknown;
          try {
            raw = JSON.parse(value ?? '{}');
          } catch {
            return;
          }
          const legacy = legacyPivot(raw);
          const named = legacy ?? (raw as Record<string, unknown>);
          const name =
            typeof named['name'] === 'string' && named['name'] !== ''
              ? named['name']
              : nextPivotName(
                  t('pivot.namePrefix'),
                  list.map((x) => x.name),
                );
          const pv = sanitizePivot({
            ...named,
            name,
            range: list.find((x) => x.name === name)?.range,
          });
          if (!pv) return;
          targets = [pv];
        } else if (cmd === 'pivotdel') {
          const pv = list.find((x) => x.name === value);
          if (!pv) return;
          const cells = { ...s.cells };
          const formats = { ...s.formats };
          const rr = parseRangeRef(pv.range ?? '');
          if (rr) {
            for (let r = rr.r1; r <= rr.r2; r++) {
              for (let c = rr.c1; c <= rr.c2; c++) {
                delete cells[refOf({ c, r })];
                delete formats[refOf({ c, r })];
              }
            }
          }
          const next: SheetData = { ...s, cells, formats, pivots: list.filter((x) => x !== pv) };
          if (next.pivots!.length === 0) delete next.pivots;
          if (Object.keys(formats).length === 0) delete next.formats;
          apply({ ...m, sheets: m.sheets.map((x, i) => (i === m.active ? next : x)) });
          return;
        } else {
          targets = value ? list.filter((x) => x.name === value) : list;
          if (targets.length === 0) return;
        }
        let sheet = s;
        let last: SheetPivot | null = null;
        for (const pv of targets) {
          const res = writePivot(
            { ...m, sheets: m.sheets.map((x, i) => (i === m.active ? sheet : x)) },
            cur.engine,
            m.active,
            pv,
            labels,
          );
          if ('error' in res) {
            notifyPivotFailed(res.error);
            return;
          }
          sheet = res.sheet;
          last = res.pivot;
        }
        apply({ ...m, sheets: m.sheets.map((x, i) => (i === m.active ? sheet : x)) });
        const rr = last ? parseRangeRef(last.range ?? '') : null;
        if (rr && cmd === 'pivot') {
          setSel({ c: rr.c1, r: rr.r1 });
          setExt({ c: rr.c2, r: rr.r2 });
        }
        if (cmd === 'pivot' && last) {
          notify({
            tone: 'info',
            title: t('sheet.msg.pivotCreated'),
            description: t('sheet.msg.pivotCreated.detail', {
              count: rr ? rr.r2 - rr.r1 : 0,
              ref: last.anchor,
            }),
          });
        }
        return;
      }

      /* --- Datos externos (conector Power Query): MATERIALIZAR o REFRESCAR. Vuelca
       *     el resultado {columns, rows} (que ya obtuvo la UI vía el BFF) como una
       *     SheetTable y GUARDA la definición (conexión + consulta, SIN credenciales)
       *     en el modelo para poder refrescar. `name` presente = refrescar la tabla
       *     externa existente (reescribe en su ancla y reajusta el rango). Las celdas
       *     son la fuente de verdad; el backend revalida la consulta en cada ejecución. --- */
      if (cmd === 'extdata') {
        let req: {
          connectionId?: unknown;
          spec?: unknown;
          anchor?: unknown;
          columns?: unknown;
          rows?: unknown;
          style?: unknown;
          name?: unknown;
          transform?: unknown;
          provenance?: unknown;
        } = {};
        try {
          req = JSON.parse(value ?? '{}') as typeof req;
        } catch {
          return;
        }
        const connectionId = typeof req.connectionId === 'string' ? req.connectionId : '';
        const anchorRect = parseRangeRef(typeof req.anchor === 'string' ? req.anchor : '');
        if (connectionId === '' || !anchorRect) return;
        const anchor = { c: anchorRect.c1, r: anchorRect.r1 };
        if (anchor.c >= MAX_COLS || anchor.r >= MAX_ROWS) return;
        // Consulta y resultado válidos ANTES de tocar la hoja (early-out sin efectos).
        const spec = sanitizeQuerySpec(req.spec);
        if (!spec) return;
        const rawResult = parseExecuteResult({
          columns: req.columns,
          rows: req.rows,
          provenance: req.provenance,
        });
        if (!rawResult) return;
        // Aplica el pipeline de transformación (pasos aplicados) al resultado crudo
        // ANTES de sembrar la hoja. Si algún paso falla, applyTransform devuelve el
        // crudo (no deja la hoja a medias); el error se ve en el gestor de pasos.
        const transform = sanitizeTransform(req.transform);
        // Resolutor para append/merge: la instantánea YA MATERIALIZADA (rejilla
        // actual) de las OTRAS tablas externas de la hoja, por su nombre. ponytail:
        // usa el snapshot del grid; NO re-ejecuta el pipeline de la otra tabla (la
        // re-evaluación transitiva es un follow-up). Excluye la propia tabla que se
        // (re)materializa → una autorreferencia da error de paso claro, no cuelga.
        const selfName = typeof req.name === 'string' ? req.name : '';
        const resolveOther: QueryResolver = (ref) => {
          if (ref === selfName) return undefined;
          const otRange = (s.tables ?? []).find((tb) => tb.name === ref)?.range;
          const rect = parseRangeRef(otRange ?? '');
          if (!rect || !(s.externalTables ?? []).some((e) => e.table === ref)) return undefined;
          const g: string[][] = [];
          for (let r = rect.r1; r <= Math.min(rect.r2, MAX_ROWS - 1); r++) {
            const row: string[] = [];
            for (let c = rect.c1; c <= Math.min(rect.c2, MAX_COLS - 1); c++)
              row.push(stripHtml(s.cells[refOf({ c, r })] ?? ''));
            g.push(row);
          }
          return gridToQueryTable(g) ?? undefined;
        };
        const result = applyTransform(rawResult, transform, resolveOther);
        const grid = resultToGrid(result);
        if (grid.length === 0 || (grid[0]?.length ?? 0) === 0) return;

        const existingTables = s.tables ?? [];
        const existingExt = s.externalTables ?? [];
        // ¿Refrescar una tabla externa ya materializada? (name = nombre de su SheetTable)
        const refreshName = typeof req.name === 'string' ? req.name : '';
        const priorIdx = refreshName ? existingExt.findIndex((e) => e.table === refreshName) : -1;
        const isRefresh = priorIdx >= 0;
        const tableName = isRefresh
          ? refreshName
          : uniqueTableName(existingTables.map((t) => t.name));

        // Dimensiones acotadas a los límites de la hoja: el backend puede
        // devolver más de lo que cabe; se recorta y se avisa.
        const fullW = grid[0]!.length;
        const fullH = grid.length;
        const width = Math.min(fullW, MAX_COLS - anchor.c);
        const height = Math.min(fullH, MAX_ROWS - anchor.r);
        if (width <= 0 || height <= 0) return;

        const cells = { ...s.cells };
        // Al refrescar, limpia el rectángulo ANTERIOR de la tabla (pudo encoger).
        if (isRefresh) {
          const prevRect = parseRangeRef(
            existingTables.find((t) => t.name === tableName)?.range ?? '',
          );
          if (prevRect) {
            for (let r = prevRect.r1; r <= Math.min(prevRect.r2, MAX_ROWS - 1); r++)
              for (let c = prevRect.c1; c <= Math.min(prevRect.c2, MAX_COLS - 1); c++)
                delete cells[refOf({ c, r })];
          }
        }
        // Vuelca la rejilla (texto escapado; el estilo de cabecera lo pone la SheetTable).
        for (let ri = 0; ri < height; ri++) {
          for (let ci = 0; ci < width; ci++) {
            const ref = refOf({ c: anchor.c + ci, r: anchor.r + ri });
            const txt = grid[ri]![ci] ?? '';
            if (txt === '') delete cells[ref];
            else cells[ref] = escapeHtml(literalDeDato(txt));
          }
        }

        const c2 = anchor.c + width - 1;
        const r2 = anchor.r + height - 1;
        const range = `${refOf(anchor)}:${refOf({ c: c2, r: r2 })}`;
        const tableDef: SheetTable = { name: tableName, range };
        // Estilo: el pedido; si es refresco sin estilo pedido, conserva el anterior.
        if (typeof req.style === 'string') tableDef.style = tableStyleOf(req.style).id;
        else if (isRefresh) {
          const prevStyle = existingTables.find((t) => t.name === tableName)?.style;
          if (prevStyle) tableDef.style = prevStyle;
        }
        const cleanTable = sanitizeTable(tableDef);
        if (!cleanTable) return;
        const tables = existingTables.some((t) => t.name === tableName)
          ? existingTables.map((t) => (t.name === tableName ? cleanTable : t))
          : [...existingTables, cleanTable];

        const cleanExt = sanitizeExternalTable({
          table: tableName,
          connectionId,
          anchor: refOf(anchor),
          spec,
          transform, // pasos aplicados (persiste para refrescar; [] se omite al sanear)
          refreshedAt: new Date().toISOString(),
          ...(rawResult.provenance ? { provenance: rawResult.provenance } : {}),
        } satisfies ExternalTable);
        if (!cleanExt) return;
        const externalTables = isRefresh
          ? existingExt.map((e, i) => (i === priorIdx ? cleanExt : e))
          : [...existingExt, cleanExt];

        applySheet({
          cells,
          tables,
          externalTables,
          autofilter: { row: anchor.r, c1: anchor.c, c2 }, // cabecera con filtros/orden
        });
        setSel(anchor);
        setExt({ c: c2, r: r2 });
        const truncated = width < fullW || height < fullH;
        notify({
          tone: truncated ? 'warning' : 'info',
          title: t(isRefresh ? 'sheet.msg.extRefreshed' : 'sheet.msg.extImported'),
          description:
            t('sheet.msg.extDetail', { name: tableName, count: height - 1, range }) +
            (truncated ? ` ${t('sheet.msg.extTruncated')}` : ''),
        });
        return;
      }

      /* --- Asistente de IA: escribe su fórmula EN LA SELECCIÓN, adaptándola.
       *
       * El asistente devuelve UNA fórmula, escrita para la primera celda del
       * destino. Pegarla tal cual en las veinte celdas de una columna deja
       * veinte veces `=SUMA(A1:B1)`, que es lo que se reportó: «debería insertar
       * la función adaptada en toda la columna, no solo generar funciones de una
       * celda».
       *
       * Se rellena con `shiftRefs`, la MISMA pieza que usa el autorrelleno de la
       * hoja, así que las referencias relativas avanzan y las que llevan `$` se
       * quedan quietas: exactamente lo que espera quien arrastra el tirador.
       *
       * CON UNA SOLA CELDA SELECCIONADA se extiende hacia abajo hasta donde
       * llegan los datos de las columnas que la fórmula MENCIONA, que es lo que
       * hace un doble clic en el tirador de relleno. No es adivinar: es la
       * convención que ya existe en cualquier hoja de cálculo, y sin ella pedir
       * «suma A y B en toda la columna C» seguiría escribiendo una sola celda. */
      if (cmd === 'aifill') {
        // DEL ESPEJO Y NO DE `sel`/`ext`: este manejador se registra UNA vez, así
        // que las variables del cierre se quedan con la selección que hubiera al
        // montar y rellenaría el rango equivocado. Es lo mismo que hace el resto
        // de comandos de este registro.
        const { sel: selAhora, ext: extAhora } = stateRef.current;
        const rect = rectOf(selAhora, extAhora);
        const celdas = celdasARellenar({
          formula: value ?? '',
          rect,
          filas: MAX_ROWS,
          tieneDato: (col, fila) => (s.cells[refOf({ c: col, r: fila })] ?? '') !== '',
          desplazar: shiftRefs,
        });
        if (celdas.length === 0) return;
        const raw: Record<string, string> = {};
        for (const { dc, dr, formula } of celdas) {
          // Escapada, como se guarda una fórmula tecleada. Lo que llega aquí es la
          // respuesta de un modelo, y una celda guarda HTML: sin escapar, un
          // `<style>` en esa respuesta quedaba en la hoja para todo el que la abre
          // (y una fórmula con `<` se leía como una etiqueta).
          raw[refOf({ c: rect.c1 + dc, r: rect.r1 + dr })] = escapeHtml(formula);
        }
        applyCells({ ...s.cells, ...raw });
        return;
      }

      /* --- Buscar objetivo (Análisis Y-si): aplica el valor hallado a la celda
       *     variable. El solver ya corrió en el diálogo (motor desechable); aquí
       *     solo se escribe el número como literal (texto crudo = fuente de verdad). --- */
      if (cmd === 'goalseek') {
        let req: { cell?: unknown; value?: unknown } = {};
        try {
          req = JSON.parse(value ?? '{}') as { cell?: unknown; value?: unknown };
        } catch {
          return;
        }
        const rr = parseRangeRef(typeof req.cell === 'string' ? req.cell : '');
        const num = Number(req.value);
        if (!rr || rr.c1 !== rr.c2 || rr.r1 !== rr.r2 || !Number.isFinite(num)) return;
        const ref = refOf({ c: rr.c1, r: rr.r1 });
        const cells = { ...s.cells };
        const p = parseCell(cells[ref] ?? ''); // conserva negrita/cursiva/alineación de la celda
        p.inner = escapeHtml(String(num));
        cells[ref] = renderCellHtml(p);
        applyCells(cells);
        setSel({ c: rr.c1, r: rr.r1 });
        setExt(null);
        return;
      }

      /* --- Solver (Análisis Y-si, optimización con restricciones): escribe los
       *     valores hallados en las celdas variable. El optimizador ya corrió en el
       *     diálogo (motor desechable); aquí solo se escriben los números como
       *     literales conservando el formato de cada celda (reusa applyScenarioValues). --- */
      if (cmd === 'solverapply') {
        let req: { values?: unknown } = {};
        try {
          req = JSON.parse(value ?? '{}') as typeof req;
        } catch {
          return;
        }
        const src =
          req.values && typeof req.values === 'object'
            ? (req.values as Record<string, unknown>)
            : {};
        const raw: Record<string, string> = {};
        let first: { c: number; r: number } | null = null;
        for (const ref in src) {
          const rr = parseRangeRef(ref);
          const num = Number(src[ref]);
          if (!rr || rr.c1 !== rr.c2 || rr.r1 !== rr.r2 || !Number.isFinite(num)) continue;
          raw[refOf({ c: rr.c1, r: rr.r1 })] = String(num);
          if (!first) first = { c: rr.c1, r: rr.r1 };
        }
        if (!first) return;
        applyCells(applyScenarioValues(s.cells, raw));
        setSel(first);
        setExt(null);
        return;
      }

      /* --- Relleno con IA: escribe los valores que la persona ha visto y confirmado,
       *     como texto crudo y conservando el formato de cada celda. Nunca una fórmula:
       *     parseFillResponse ya las descarta, y aquí otra vez por si llega otra cosa. --- */
      if (cmd === 'fillcells') {
        let req: { values?: unknown } = {};
        try {
          req = JSON.parse(value ?? '{}') as typeof req;
        } catch {
          return;
        }
        const src =
          req.values && typeof req.values === 'object'
            ? (req.values as Record<string, unknown>)
            : {};
        const raw: Record<string, string> = {};
        for (const ref in src) {
          const rr = parseRangeRef(ref);
          const v = src[ref];
          if (!rr || rr.c1 !== rr.c2 || rr.r1 !== rr.r2) continue;
          if (typeof v !== 'string' || v.trim() === '' || v.startsWith('=')) continue;
          raw[refOf({ c: rr.c1, r: rr.r1 })] = v;
        }
        if (Object.keys(raw).length === 0) return;
        applyCells(applyScenarioValues(s.cells, raw));
        return;
      }

      /* --- Bloque reutilizable (sheets/bloque.ts): lo coloca con su esquina en la celda
       *     activa. Del espejo y no de `sel`, como `aifill`. --- */
      if (cmd === 'bloquehoja') {
        const parche = colocarBloque(s, value ?? '', stateRef.current.sel);
        if (parche) applySheet(parche);
        return;
      }

      /* --- Escenarios (Administrador de escenarios): crear/actualizar/aplicar/
       *     borrar y el informe resumen. Los valores de las celdas de entrada se
       *     guardan como TEXTO CRUDO (fuente de verdad); aplicar conserva el
       *     formato de cada celda (applyScenarioValues) y la hoja recalcula. --- */
      if (cmd === 'scenadd') {
        let req: { name?: unknown; range?: unknown } = {};
        try {
          req = JSON.parse(value ?? '{}') as typeof req;
        } catch {
          return;
        }
        const name = typeof req.name === 'string' ? req.name.trim() : '';
        const rr = parseRangeRef(typeof req.range === 'string' ? req.range : '');
        if (name === '' || !rr) return;
        if ((rr.c2 - rr.c1 + 1) * (rr.r2 - rr.r1 + 1) > SCENARIO_MAX_CELLS) {
          notify({
            tone: 'warning',
            title: t('sheet.msg.tooManyCells'),
            description: t('sheet.msg.tooManyCells.detail', { max: SCENARIO_MAX_CELLS }),
          });
          return;
        }
        if ((s.scenarios ?? []).some((x) => x.name === name)) {
          notify({
            tone: 'warning',
            title: t('sheet.msg.nameTaken'),
            description: t('sheet.msg.nameTaken.detail', { name }),
          });
          return;
        }
        const values: Record<string, string> = {};
        for (let r = rr.r1; r <= Math.min(rr.r2, MAX_ROWS - 1); r++) {
          for (let c = rr.c1; c <= Math.min(rr.c2, MAX_COLS - 1); c++) {
            const ref = refOf({ c, r });
            values[ref] = rawText(s.cells[ref] ?? '');
          }
        }
        applySheet({ scenarios: [...(s.scenarios ?? []), { name, values }] });
        notify({
          tone: 'info',
          title: t('sheet.msg.scenarioCreated'),
          description: t('sheet.msg.scenarioCreated.detail', {
            name,
            count: Object.keys(values).length,
          }),
        });
        return;
      }
      if (cmd === 'scenupdate') {
        const name = (value ?? '').trim();
        const list = s.scenarios ?? [];
        const sc = list.find((x) => x.name === name);
        if (!sc) return;
        const values: Record<string, string> = {};
        for (const ref in sc.values) values[ref] = rawText(s.cells[ref] ?? '');
        applySheet({ scenarios: list.map((x) => (x.name === name ? { name, values } : x)) });
        notify({
          tone: 'info',
          title: t('sheet.msg.scenarioUpdated'),
          description: t('sheet.msg.scenarioUpdated.detail', { name }),
        });
        return;
      }
      if (cmd === 'scenapply') {
        const name = (value ?? '').trim();
        const sc = (s.scenarios ?? []).find((x) => x.name === name);
        if (!sc) return;
        applyCells(applyScenarioValues(s.cells, sc.values));
        notify({
          tone: 'info',
          title: t('sheet.msg.scenarioApplied'),
          description: t('sheet.msg.scenarioApplied.detail', { name }),
        });
        return;
      }
      if (cmd === 'scendel') {
        const name = (value ?? '').trim();
        const m = cur.model;
        const list = s.scenarios ?? [];
        const scenarios = list.filter((x) => x.name !== name);
        if (scenarios.length === list.length) return;
        const next: SheetData = { ...s, scenarios };
        if (scenarios.length === 0) delete next.scenarios; // no ensuciar el JSON persistido
        apply({ ...m, sheets: m.sheets.map((x, i) => (i === m.active ? next : x)) });
        return;
      }
      if (cmd === 'scenreport') {
        let req: { target?: unknown; results?: unknown } = {};
        try {
          req = JSON.parse(value ?? '{}') as typeof req;
        } catch {
          return;
        }
        const scenarios = s.scenarios ?? [];
        if (scenarios.length === 0) return;
        const tgt = parseRangeRef(typeof req.target === 'string' ? req.target : '');
        if (!tgt) return;
        const results = Array.isArray(req.results)
          ? req.results.filter((r): r is string => typeof r === 'string')
          : [];
        // Valores crudos actuales de las celdas de entrada (columna «Valores actuales»).
        const current: Record<string, string> = {};
        for (const sc of scenarios) {
          for (const ref in sc.values) current[ref] = rawText(s.cells[ref] ?? '');
        }
        // Motor DESECHABLE por columna (como la sonda de Buscar objetivo): fija los
        // overrides del escenario y lee las celdas de resultado sin tocar la hoja.
        const evaluate = (
          overrides: Record<string, string>,
          refs: readonly string[],
        ): Record<string, string> => {
          const eng = buildEngine(cur.model);
          for (const ref in overrides) eng.setCell(s.name, ref, overrides[ref] ?? '');
          const out: Record<string, string> = {};
          for (const ref of refs) out[ref] = formatCellValue(eng.getValue(s.name, ref));
          return out;
        };
        const grid = buildScenarioReport(scenarios, results, evaluate, {
          title: t('sheet.scenario.report'),
          changingLabel: t('sheet.scenario.changingCells'),
          resultLabel: t('sheet.scenario.resultCellsLabel'),
          currentLabel: t('sheet.scenario.current'),
          current,
        });
        const cells = { ...s.cells };
        grid.forEach((row, ri) => {
          row.forEach((txt, ci) => {
            const c = tgt.c1 + ci;
            const r = tgt.r1 + ri;
            if (c >= MAX_COLS || r >= MAX_ROWS) return;
            const ref = refOf({ c, r });
            if (txt === '') delete cells[ref];
            else cells[ref] = ri === 0 || ci === 0 ? `<b>${escapeHtml(txt)}</b>` : escapeHtml(txt);
          });
        });
        applyCells(cells);
        setSel({ c: tgt.c1, r: tgt.r1 });
        setExt({
          c: Math.min(MAX_COLS - 1, tgt.c1 + (grid[0]?.length ?? 1) - 1),
          r: Math.min(MAX_ROWS - 1, tgt.r1 + grid.length - 1),
        });
        notify({
          tone: 'info',
          title: t('sheet.msg.scenarioReport'),
          description: t('sheet.msg.scenarioReport.detail', {
            count: scenarios.length,
            ref: refOf({ c: tgt.c1, r: tgt.r1 }),
          }),
        });
        return;
      }

      /* --- Tabla de datos (Análisis Y-si): análisis de sensibilidad. Fija la(s)
       *     celda(s) de entrada al valor de cada cabecera (fila superior / columna
       *     izquierda) y evalúa la fórmula en un motor DESECHABLE (como Buscar
       *     objetivo), materializando la columna/fila/matriz de resultados. --- */
      if (cmd === 'datatable') {
        let req: { range?: unknown; rowInput?: unknown; colInput?: unknown } = {};
        try {
          req = JSON.parse(value ?? '{}') as typeof req;
        } catch {
          return;
        }
        const rc = parseRangeRef(typeof req.range === 'string' ? req.range : '');
        if (!rc || rc.c1 >= MAX_COLS || rc.r1 >= MAX_ROWS) return;
        const rect = {
          c1: rc.c1,
          r1: rc.r1,
          c2: Math.min(rc.c2, MAX_COLS - 1),
          r2: Math.min(rc.r2, MAX_ROWS - 1),
        };
        // Celda de entrada = UNA celda; ambas opcionales (una variable) pero al menos una.
        const single = (v: unknown): string | null => {
          const p = parseRangeRef(typeof v === 'string' ? v : '');
          return p && p.c1 === p.c2 && p.r1 === p.r2 ? refOf({ c: p.c1, r: p.r1 }) : null;
        };
        const rowCell = single(req.rowInput);
        const colCell = single(req.colInput);
        const plan = planDataTable(rect, rowCell !== null, colCell !== null);
        if ('error' in plan) {
          const msg: Record<DataTableError, DocsKey> = {
            'no-input': 'sheet.msg.dataTable.noInput',
            'too-small': 'sheet.msg.dataTable.tooSmall',
            'too-many': 'sheet.msg.dataTable.tooMany',
          };
          notify({
            tone: 'warning',
            title: t('sheet.msg.dataTable'),
            description: t(msg[plan.error]),
          });
          return;
        }
        // La celda de la fórmula debe ser una fórmula (si no, sustituir no cambia nada).
        const formulaRef = refOf(plan.formula);
        if (!isFormula(rawText(s.cells[formulaRef] ?? ''))) {
          notify({
            tone: 'warning',
            title: t('sheet.msg.missingFormula'),
            description: t('sheet.msg.missingFormula.detail', { ref: formulaRef }),
          });
          return;
        }
        // Motor DESECHABLE reutilizado (como la sonda de Buscar objetivo): fija las
        // entradas y lee la fórmula por cada resultado, sin tocar la hoja real.
        const eng = buildEngine(cur.model);
        const evaluate = (subs: DataTableSubs): CellValue => {
          if (rowCell && subs.row !== undefined) eng.setCell(s.name, rowCell, subs.row);
          if (colCell && subs.col !== undefined) eng.setCell(s.name, colCell, subs.col);
          return eng.getValue(s.name, formulaRef);
        };
        const read = (c: number, r: number): string => rawText(s.cells[refOf({ c, r })] ?? '');
        const results = computeDataTable(plan, read, evaluate);
        const cells = { ...s.cells };
        for (const { c, r, value } of results) {
          if (c >= MAX_COLS || r >= MAX_ROWS) continue;
          const ref = refOf({ c, r });
          const txt = formatCellValue(value);
          if (txt === '') delete cells[ref];
          else cells[ref] = escapeHtml(txt);
        }
        applyCells(cells);
        setSel({ c: rect.c1, r: rect.r1 });
        setExt({ c: rect.c2, r: rect.r2 });
        notify({
          tone: 'info',
          title: t('sheet.msg.dataTable'),
          description: t('sheet.msg.dataTable.detail', {
            count: results.length,
            ref: formulaRef,
          }),
        });
        return;
      }

      /* --- Tablas con formato (ListObject): crear, quitar, estilo y totales --- */
      if (
        cmd === 'tableadd' ||
        cmd === 'tabledel' ||
        cmd === 'tablestyle' ||
        cmd === 'tabletotals' ||
        cmd === 'tabletotalfn'
      ) {
        runTableCommand(cmd, value);
        return;
      }

      /* --- Esquema (agrupar/outline): grupos de filas o columnas de la selección. --- */
      if (cmd === 'outline') {
        let req: { axis?: unknown; op?: unknown; start?: unknown; end?: unknown; level?: unknown } =
          {};
        try {
          req = JSON.parse(value ?? '{}') as typeof req;
        } catch {
          return;
        }
        const axis = req.axis === 'col' ? 'col' : 'row';
        const groups = (axis === 'col' ? s.colGroups : s.rowGroups) ?? [];
        const max = axis === 'col' ? MAX_COLS : MAX_ROWS;
        const rc = rectOf(cur.sel, cur.ext);
        const a = axis === 'col' ? rc.c1 : rc.r1;
        const b = axis === 'col' ? rc.c2 : rc.r2;
        let next: SheetGroup[];
        if (req.op === 'group') {
          if (a === b) {
            notify({
              tone: 'info',
              title: t('sheet.msg.pickRange'),
              description: t(
                axis === 'col' ? 'sheet.msg.pickRange.cols' : 'sheet.msg.pickRange.rows',
              ),
            });
            return;
          }
          next = addGroup(groups, a, b, max);
        } else if (req.op === 'ungroup') {
          next = removeGroup(groups, a, b);
        } else if (req.op === 'clear') {
          next = [];
        } else if (req.op === 'level') {
          const lvl = Math.trunc(Number(req.level));
          if (!Number.isInteger(lvl) || lvl < 1) return;
          next = groupsAtLevel(groups, lvl);
        } else if (req.op === 'toggle') {
          const start = Math.trunc(Number(req.start));
          const end = Math.trunc(Number(req.end));
          const level = Math.trunc(Number(req.level));
          if (![start, end, level].every((n) => Number.isInteger(n))) return;
          next = toggleGroup(groups, start, end, level);
        } else {
          return;
        }
        const patch: SheetData = { ...s };
        if (axis === 'col') {
          if (next.length > 0) patch.colGroups = next;
          else delete patch.colGroups;
        } else {
          if (next.length > 0) patch.rowGroups = next;
          else delete patch.rowGroups;
        }
        apply({
          ...cur.model,
          sheets: cur.model.sheets.map((x, i) => (i === cur.model.active ? patch : x)),
        });
        return;
      }

      /* --- Subtotales: el asistente entero lo decide `applySubtotals` (outline.ts),
       *     que es el mismo que usa la app nativa con los mismos vectores. Aquí solo
       *     se aplica: mover cada fila a donde dice `from` (con su formato), escribir
       *     las celdas de subtotal y poner el esquema. Con `remove`, «Quitar todos». --- */
      if (cmd === 'subtotal') {
        let req: {
          range?: unknown;
          groupCol?: unknown;
          fn?: unknown;
          cols?: unknown;
          remove?: unknown;
        } = {};
        try {
          req = JSON.parse(value ?? '{}') as typeof req;
        } catch {
          return;
        }
        const rc = parseRangeRef(typeof req.range === 'string' ? req.range : '');
        if (!rc || rc.c1 >= MAX_COLS || rc.r1 >= MAX_ROWS) return;
        const c1 = rc.c1;
        const c2 = Math.min(rc.c2, MAX_COLS - 1);
        const remove = req.remove === true;
        const groupRel = remove ? 0 : Math.trunc(Number(req.groupCol));
        const fn = remove ? 'sum' : req.fn;
        const validFns: readonly TableTotalFn[] = [
          'sum',
          'average',
          'count',
          'counta',
          'min',
          'max',
        ];
        if (!Number.isInteger(groupRel) || groupRel < 0 || groupRel > c2 - c1) return;
        if (typeof fn !== 'string' || !validFns.includes(fn as TableTotalFn)) return;
        // Columnas elegidas en «Añadir subtotal a», relativas al rango; ausente = automático.
        const cols = Array.isArray(req.cols)
          ? req.cols
              .map((x) => c1 + Math.trunc(Number(x)))
              .filter((c) => Number.isInteger(c) && c >= c1 && c <= c2)
          : undefined;
        let maxUsedRow = -1;
        for (const ref in s.cells) {
          const p = parseRangeRef(ref);
          if (p && p.r1 > maxUsedRow) maxUsedRow = p.r1;
        }
        const res = applySubtotals(
          {
            c1,
            r1: rc.r1,
            c2,
            r2: rc.r2,
            groupCol: c1 + groupRel,
            fn: fn as Exclude<TableTotalFn, 'none'>,
            ...(cols ? { cols } : {}),
            remove,
            // Basta con que quepa lo que se desplaza: por debajo de lo usado la hoja está
            // vacía, y un `rows` de un millón haría a applySubtotals recorrerlo entero.
            rows: Math.min(MAX_ROWS, Math.max(maxUsedRow, rc.r2) + 2 * (rc.r2 - rc.r1 + 1) + 3),
            maxUsedRow,
          },
          {
            raw: (c, r) => rawText(s.cells[refOf({ c, r })] ?? ''),
            key: (c, r) => cellText(cur.engine, s.name, c, r),
            isNumber: (c, r) => {
              const v = cur.engine.getValue(s.name, refOf({ c, r }));
              return typeof v === 'number' && Number.isFinite(v);
            },
            hasValue: (c, r) =>
              formatCellValue(cur.engine.getValue(s.name, refOf({ c, r }))) !== '',
          },
          {
            grand: t('sheet.total.grand'),
            total: t('sheet.total'),
            totalOf: (key) => t('sheet.total.of', { key }),
          },
        );
        if ('error' in res) {
          notify({
            tone: 'warning',
            title: t(res.error === 'no-data' ? 'sheet.msg.noData' : 'sheet.msg.noRoom'),
            description: t(
              res.error === 'no-data' ? 'sheet.msg.noDataRows' : 'sheet.msg.noRoomSubtotals',
            ),
          });
          return;
        }
        const cells: Record<string, string> = {};
        const formats: Record<string, NumFmt> = {};
        const styles: Record<string, CellStyle> = {};
        // Se recorre lo que HAY y no columna a columna: así no se queda fuera nada a la
        // derecha de la Z (antes se perdía) ni se visitan miles de columnas vacías.
        const destino = new Map<number, number>(); // fila de origen → fila nueva
        res.from.forEach((fromR, toR) => {
          if (fromR !== null) destino.set(fromR, toR);
        });
        const llevar = <T,>(src: Record<string, T> | undefined, dst: Record<string, T>): void => {
          for (const ref in src) {
            const p = parseRangeRef(ref);
            const toR = p ? destino.get(p.r1) : undefined;
            if (p && toR !== undefined) dst[refOf({ c: p.c1, r: toR })] = src[ref]!;
          }
        };
        llevar(s.cells, cells);
        llevar(s.formats, formats);
        llevar(s.styles, styles);
        for (const w of res.writes)
          cells[refOf({ c: w.c, r: w.r })] = `<b>${escapeHtml(w.text)}</b>`;
        const patch: SheetData = { ...s, cells };
        if (Object.keys(formats).length > 0) patch.formats = formats;
        else delete patch.formats;
        if (Object.keys(styles).length > 0) patch.styles = styles;
        else delete patch.styles;
        if (res.groups.length > 0)
          patch.rowGroups = res.groups; // el subtotal DEFINE el esquema
        else delete patch.rowGroups;
        apply({
          ...cur.model,
          sheets: cur.model.sheets.map((x, i) => (i === cur.model.active ? patch : x)),
        });
        setSel({ c: c1, r: rc.r1 });
        setExt({ c: c2, r: res.lastRow });
        if (!remove) {
          notify({
            tone: 'info',
            title: t('sheet.msg.subtotalsCreated'),
            description: t('sheet.msg.subtotalsCreated.detail', {
              count: res.blocks,
              col: colName(c1 + groupRel),
            }),
          });
        }
        return;
      }

      const cells = { ...s.cells };
      let changed = false;

      if (cmd === 'bold' || cmd === 'italic') {
        const anchor = parseCell(cells[refOf(cur.sel)] ?? '');
        const target = cmd === 'bold' ? !anchor.b : !anchor.i;
        eachRef((ref) => {
          const p = parseCell(cells[ref] ?? '');
          if (p.inner === '') return;
          if (cmd === 'bold') p.b = target;
          else p.i = target;
          cells[ref] = renderCellHtml(p);
          changed = true;
        });
      } else if (cmd === 'align') {
        const align = value === 'center' || value === 'right' ? value : '';
        eachRef((ref) => {
          const p = parseCell(cells[ref] ?? '');
          if (p.inner === '') return;
          p.align = align;
          cells[ref] = renderCellHtml(p);
          changed = true;
        });
      } else {
        // merge: concatena el texto del rango en la celda superior-izquierda.
        // ponytail: combinación de CONTENIDO, sin celda visual extendida
        // (colspan); el modelo del §2 no contempla fusiones y es de A1.
        if (range.c1 === range.c2 && range.r1 === range.r2) return;
        const topRef = refOf({ c: range.c1, r: range.r1 });
        const texts: string[] = [];
        eachRef((ref) => {
          const text = stripHtml(cells[ref] ?? '').trim();
          if (text !== '') texts.push(text);
        });
        const top = parseCell(cells[topRef] ?? '');
        eachRef((ref) => {
          delete cells[ref];
        });
        top.inner = escapeHtml(texts.join(' '));
        const html = renderCellHtml(top);
        if (html !== '') cells[topRef] = html;
        changed = true;
        setSel({ c: range.c1, r: range.r1 });
        setExt(null);
      }
      if (changed) applyCells(cells);
    };
    sheetCommandHandler = run;
    return () => {
      if (sheetCommandHandler === run) sheetCommandHandler = null;
    };
  }, [t, commandActionsRef, stateRef]);

  /* ----- canal de lectura hoja→ribbon: reader + aviso en cada cambio ----- */

  useEffect(() => {
    sheetStateReader = (): SheetRibbonState => {
      const cur = stateRef.current;
      const s = cur.model.sheets[cur.model.active];
      const r = rectOf(cur.sel, cur.ext);
      // Tabla que cubre el ancla (la última definida gana), para el panel del ribbon.
      let table: SheetTable | null = null;
      for (const t of s?.tables ?? []) {
        const rr = parseRangeRef(t.range);
        if (
          rr &&
          cur.sel.c >= rr.c1 &&
          cur.sel.c <= rr.c2 &&
          cur.sel.r >= rr.r1 &&
          cur.sel.r <= rr.r2
        ) {
          table = t;
        }
      }
      return {
        numFmt: s?.formats?.[refOf(cur.sel)] ?? { id: 'general' },
        style: s?.styles?.[refOf(cur.sel)] ?? {},
        freeze: s?.freeze ?? { cols: 0, rows: 0 },
        condRules: s?.condFormats ?? [],
        validations: s?.validations ?? [],
        sparklines: s?.sparklines ?? [],
        selection: cur.ext
          ? `${refOf({ c: r.c1, r: r.r1 })}:${refOf({ c: r.c2, r: r.r2 })}`
          : refOf(cur.sel),
        autofilter: s?.autofilter != null,
        table,
        // Cada tabla externa con el rango actual de su SheetTable enlazada (para
        // el gestor y el refresco). O(externas × tablas): ambas son pocas.
        externalTables: (s?.externalTables ?? []).map((e) => ({
          ...e,
          range: (s?.tables ?? []).find((t) => t.name === e.table)?.range ?? '',
        })),
        protectedSheet: s?.protect != null,
        protectHasPassword: !!s?.protect?.hasPassword,
        protectRanges: (s?.protect?.ranges ?? []).map((g) => g.ref),
        protectedWorkbook: cur.model.protect != null,
        protectWorkbookHasPassword: !!cur.model.protect?.hasPassword,
        scenarios: s?.scenarios ?? [],
        pivots: s?.pivots ?? [],
        pivotAt:
          (s?.pivots ?? []).find((p) => {
            const rr = parseRangeRef(p.range ?? p.anchor);
            return (
              rr &&
              cur.sel.c >= rr.c1 &&
              cur.sel.c <= rr.c2 &&
              cur.sel.r >= rr.r1 &&
              cur.sel.r <= rr.r2
            );
          }) ?? null,
        names: cur.model.names ?? [],
        sheetNames: cur.model.sheets.map((x) => x.name),
        activeSheet: s?.name ?? '',
      };
    };
    /** Última columna y fila que merece la pena leer de una hoja: la extensión, o lo
     *  usado de esa hoja si llega más lejos. Más allá de las dos todo está vacío, y un
     *  rango de columna entera no puede recorrer un millón de filas. */
    const tope = (sheetName: string): { c: number; r: number } => {
      const cur = stateRef.current;
      const u = cur.engine.usedRange(sheetName);
      return { c: Math.max(cur.cols, u.maxCol + 1) - 1, r: Math.max(cur.rows, u.maxRow + 1) - 1 };
    };
    sheetGridReader = (range: string): string[][] | null => {
      const cur = stateRef.current;
      const rr = parseRangeRef(range);
      const s = cur.model.sheets[cur.model.active];
      if (!rr || !s) return null;
      const out: string[][] = [];
      const max = tope(s.name);
      for (let r = rr.r1; r <= Math.min(rr.r2, max.r); r++) {
        const row: string[] = [];
        for (let c = rr.c1; c <= Math.min(rr.c2, max.c); c++) {
          row.push(formatCellValue(cur.engine.getValue(s.name, refOf({ c, r }))));
        }
        out.push(row);
      }
      return out;
    };
    sheetNameReader = (): string | null => {
      const cur = stateRef.current;
      return cur.model.sheets[cur.model.active]?.name ?? null;
    };
    sheetValueReader = (range: string, sheet?: string): CellValue[][] | null => {
      const cur = stateRef.current;
      const rr = parseRangeRef(range);
      const s = sheet
        ? cur.model.sheets.find((x) => x.name === sheet)
        : cur.model.sheets[cur.model.active];
      if (!rr || !s) return null;
      const out: CellValue[][] = [];
      const max = tope(s.name);
      for (let r = rr.r1; r <= Math.min(rr.r2, max.r); r++) {
        const row: CellValue[] = [];
        for (let c = rr.c1; c <= Math.min(rr.c2, max.c); c++) {
          row.push(cur.engine.getValue(s.name, refOf({ c, r })));
        }
        out.push(row);
      }
      return out;
    };
    sheetProbeReader = (targetRef: string, varRef: string): GoalSeekProbeResult => {
      const cur = stateRef.current;
      const s = cur.model.sheets[cur.model.active];
      if (!s) return { ok: false, reason: 'no-sheet' };
      const tRect = parseRangeRef(targetRef);
      const vRect = parseRangeRef(varRef);
      if (!tRect || tRect.c1 !== tRect.c2 || tRect.r1 !== tRect.r2)
        return { ok: false, reason: 'bad-target' };
      if (!vRect || vRect.c1 !== vRect.c2 || vRect.r1 !== vRect.r2)
        return { ok: false, reason: 'bad-var' };
      const targetCell = A1(tRect.c1, tRect.r1);
      const varCell = A1(vRect.c1, vRect.r1);
      // La celda objetivo debe ser fórmula (si no, mover la variable no la cambia);
      // la variable debe ser un valor (una fórmula no se puede tantear). Criterio Excel.
      if (!isFormula(rawText(s.cells[targetCell] ?? '')))
        return { ok: false, reason: 'target-not-formula' };
      if (isFormula(rawText(s.cells[varCell] ?? '')))
        return { ok: false, reason: 'var-is-formula' };
      // Motor DESECHABLE clonado del modelo actual: el solver fija la variable
      // cuantas veces quiera sin tocar el motor vivo ni la hoja. Refs con «.» decimal.
      const probeEngine = buildEngine(cur.model);
      const cv = probeEngine.getValue(s.name, varCell);
      const current = typeof cv === 'number' && Number.isFinite(cv) ? cv : 0;
      const f = (x: number): number | null => {
        probeEngine.setCell(s.name, varCell, String(x));
        const v = probeEngine.getValue(s.name, targetCell);
        return typeof v === 'number' && Number.isFinite(v) ? v : null;
      };
      return { ok: true, probe: { f, current } };
    };
    sheetSolverReader = (
      targetRef: string,
      varRefs: string[],
      cons: SolverConstraintRef[],
    ): SolverProbeResult => {
      const cur = stateRef.current;
      const s = cur.model.sheets[cur.model.active];
      if (!s) return { ok: false, reason: 'no-sheet' };
      // Objetivo = una celda con fórmula (como Buscar objetivo).
      const targetCell = singleCellRef(targetRef);
      if (!targetCell) return { ok: false, reason: 'bad-target' };
      if (!isFormula(rawText(s.cells[targetCell] ?? '')))
        return { ok: false, reason: 'target-not-formula' };
      // Variables = celdas (valor, no fórmula); al menos una.
      if (varRefs.length === 0) return { ok: false, reason: 'no-vars' };
      const varCells: string[] = [];
      for (const v of varRefs) {
        const cell = singleCellRef(v);
        if (!cell) return { ok: false, reason: 'bad-var' };
        if (isFormula(rawText(s.cells[cell] ?? ''))) return { ok: false, reason: 'var-is-formula' };
        varCells.push(cell);
      }
      // Restricciones: LHS = celda; RHS = número (es-ES) o celda, clasificado con la
      // CELDA primero (classifyConstraintRhs) —parseEsNumber('D3') daría 3, la fila—.
      // Se resuelven en cada evaluación (el RHS-celda puede depender de las variables).
      const resolved: { lhsCell: string; rhsNum: number | null; rhsCell: string | null }[] = [];
      for (const c of cons) {
        const lhsCell = singleCellRef(c.lhs);
        if (!lhsCell) return { ok: false, reason: 'bad-constraint' };
        const rhs = classifyConstraintRhs(c.rhs);
        if (!rhs) return { ok: false, reason: 'bad-constraint' };
        resolved.push({ lhsCell, rhsNum: rhs.num, rhsCell: rhs.cell });
      }
      // Motor DESECHABLE clonado (como la sonda de Buscar objetivo / tabla de datos):
      // fija las variables y lee objetivo + restricciones sin tocar la hoja real.
      const probeEngine = buildEngine(cur.model);
      const numAt = (ref: string): number | null => {
        const v = probeEngine.getValue(s.name, ref);
        return typeof v === 'number' && Number.isFinite(v) ? v : null;
      };
      const current = varCells.map((cell) => numAt(cell) ?? 0);
      const evaluate: SolverEvaluate = (x) => {
        for (let i = 0; i < varCells.length; i++)
          probeEngine.setCell(s.name, varCells[i]!, String(x[i] ?? 0));
        return {
          objective: numAt(targetCell),
          constraints: resolved.map((r) => ({
            lhs: numAt(r.lhsCell),
            rhs: r.rhsNum !== null ? r.rhsNum : numAt(r.rhsCell!),
          })),
        };
      };
      return { ok: true, probe: { evaluate, current } };
    };
    return () => {
      sheetStateReader = null;
      sheetGridReader = null;
      sheetValueReader = null;
      sheetNameReader = null;
      sheetProbeReader = null;
      sheetSolverReader = null;
    };
  }, [stateRef]);

  useEffect(() => {
    for (const fn of sheetStateListeners) fn();
  }, [model, sel.c, sel.r, ext]);

  /* ----- carga por época (contrato §7): parse inválido → modelo vacío + toast ----- */

  const loadRef = useUltimo({ content: ctx.content, notify });

  useEffect(() => {
    const latest = loadRef.current;
    let next = emptyModel();
    if (latest.content) {
      try {
        const parsed = JSON.parse(latest.content) as SheetModel | null;
        if (parsed && Array.isArray(parsed.sheets) && parsed.sheets.length > 0) {
          const bookProtect = sanitizeWorkbookProtection(parsed.protect);
          const names = sanitizeNames(parsed.names);
          next = {
            active: clamp(Math.trunc(Number(parsed.active) || 0), 0, parsed.sheets.length - 1),
            sheets: conIdsDeHoja(parsed.sheets.map((s, i) => sanitizeSheet(s, i))),
            ...(bookProtect ? { protect: bookProtect } : {}),
            ...(names ? { names } : {}),
          };
        } else {
          latest.notify({
            tone: 'warning',
            title: t('sheet.msg.readFailed'),
            description: t('sheet.msg.readFailed.detail'),
          });
        }
      } catch {
        latest.notify({
          tone: 'warning',
          title: t('sheet.msg.readFailed'),
          description: t('sheet.msg.readFailed.detail'),
        });
      }
    }
    setModel(next);
    setSel({ c: 0, r: 0 });
    setExt(null);
    setEditing(false);
    setFilterCol(null); // el autofiltro se persiste; solo se cierra su desplegable
  }, [ctx.contentEpoch, t, loadRef]);

  /* ----- estadísticas (§7.1) y limpieza del root de exec ----- */

  const reportStats = ctx.reportStats;
  useEffect(() => {
    reportStats({ pages: model.sheets.length, page: model.active + 1 });
  }, [model.sheets.length, model.active, reportStats]);

  useEffect(() => () => setEditorRoot(null), []);

  /* ----- «Buscar» del carril sobre las celdas de la hoja (sheets/search.ts) ----- */

  const searchRef = useRef<{ matches: SheetMatch[]; q: string; opts: MatchOptions }>({
    matches: [],
    q: '',
    opts: {},
  });
  // Las celdas con coincidencias, que se pintan de amarillo como el folio.
  const [searchHits, setSearchHits] = useState<ReadonlySet<string>>(() => new Set());
  const setMatches = (ms: SheetMatch[]): void => {
    searchRef.current.matches = ms;
    const hits = hitCells(ms);
    setSearchHits((prev) =>
      prev.size === hits.size && [...hits].every((k) => prev.has(k)) ? prev : hits,
    );
  };
  // Sin dependencias a propósito: cada render deja el objetivo con SUS filas ocultas
  // y su protección, y el panel lo pide al pulsar; nunca se queda con uno viejo.
  useEffect(() => {
    const findNow = (): SheetMatch[] => {
      const cur = stateRef.current;
      const s = cur.model.sheets[cur.model.active];
      if (!s) return [];
      const { q, opts } = searchRef.current;
      return sheetMatches(
        s.cells,
        q,
        opts,
        (ref) => rawText(s.cells[ref] ?? ''),
        (c, r, ref) =>
          hidden.has(r) ||
          hiddenCols.has(c) ||
          !canSelectAt({ c, r }) ||
          (isFormulaHidden(!!s.protect, s.styles?.[ref]) && isFormula(s.cells[ref] ?? '')),
      );
    };
    const badValue = (dv: DataValidation, ref: string): void => {
      notify({
        tone: 'warning',
        title: t('sheet.msg.badValue'),
        description: validationMessage(dv, ref),
      });
    };
    const goTo = (i: number): void => {
      const m = searchRef.current.matches[i];
      if (!m) return;
      select({ c: m.c, r: m.r }, false);
      revelarRef.current = true;
    };
    setSearchTarget({
      find: (q, opts) => {
        searchRef.current = { matches: [], q, opts };
        setMatches(findNow());
        return searchRef.current.matches.length;
      },
      count: () => searchRef.current.matches.length,
      goTo,
      editable: () => !ctx.soloLectura,
      replace: (i, by) => {
        const st = searchRef.current;
        const fresh = findNow();
        const m = fresh[i];
        const vieja = !sameMatch(m, st.matches[i]);
        st.matches = fresh;
        if (ctx.soloLectura || vieja || !m) return 0; // no se escribe con una búsqueda vieja
        const cur = stateRef.current;
        const s = cur.model.sheets[cur.model.active];
        if (!s) return 0;
        const ref = refOf(m);
        select({ c: m.c, r: m.r }, false);
        let after = m.end; // si no se escribe, se pasa igual a la siguiente
        const text = splice(rawText(s.cells[ref] ?? ''), m.start, m.end, by);
        const dv = validationOf(s, m.c, m.r);
        if (!canEditAt(m)) notifyProtected(ref);
        else if (dv?.strict && !validateValue(dv, text, () => dvFormulaOk(dv, m.c, m.r, text)))
          badValue(dv, ref);
        else {
          const html = withText(s.cells[ref] ?? '', text);
          if (html !== (s.cells[ref] ?? '')) {
            const cells = { ...s.cells };
            if (html === '') delete cells[ref];
            else cells[ref] = html;
            applyCells(cells);
          }
          after = m.start + by.length;
        }
        setMatches(findNow());
        return nextFrom(st.matches, m.c, m.r, after);
      },
      replaceAll: (by) => {
        const st = searchRef.current;
        st.matches = findNow();
        const cur = stateRef.current;
        const s = cur.model.sheets[cur.model.active];
        if (ctx.soloLectura || !s) return;
        const cells = { ...s.cells };
        let changed = false;
        let warned = false; // lo que no se puede escribir se avisa UNA vez
        for (const p of replaceAllPlan(st.matches, (m) => rawText(s.cells[refOf(m)] ?? ''), by)) {
          const ref = refOf(p);
          const html = withText(s.cells[ref] ?? '', p.text);
          if (html === (s.cells[ref] ?? '')) continue; // nada que escribir
          const dv = validationOf(s, p.c, p.r);
          const locked = !canEditAt(p);
          if (
            locked ||
            (dv?.strict && !validateValue(dv, p.text, () => dvFormulaOk(dv, p.c, p.r, p.text)))
          ) {
            if (!warned && locked) notifyProtected(ref);
            else if (!warned && dv) badValue(dv, ref);
            warned = true;
            continue;
          }
          if (html === '') delete cells[ref];
          else cells[ref] = html;
          changed = true;
        }
        if (changed) applyCells(cells); // UN paso de deshacer
        setMatches(findNow());
      },
    });
  });
  useEffect(() => () => setSearchTarget(null), []);

  /* ----- barra de fórmulas: refleja la celda ancla; Enter escribe y baja ----- */

  useEffect(() => {
    const ref = refOf(sel);
    const raw = sheet.cells[ref] ?? '';
    // Con la hoja protegida, una celda `hidden` no revela su fórmula en la barra.
    if (isFormulaHidden(!!sheet.protect, sheet.styles?.[ref]) && isFormula(raw)) setFx('');
    else setFx(stripHtml(raw));
  }, [sel, sheet]);

  /** Ajusta el fondo tintado al desplazamiento horizontal del input. */
  const syncBackdropScroll = (): void => {
    if (fxBackdropRef.current && fxInputRef.current) {
      fxBackdropRef.current.scrollLeft = fxInputRef.current.scrollLeft;
    }
  };

  /* Coloca el cursor de la barra fx tras un cambio programático (aceptar
   * autocompletado, insertar ref, F4); en el tecleo normal no hay pendiente. */
  useLayoutEffect(() => {
    const c = pendingBarCaretRef.current;
    if (c !== null && fxInputRef.current) {
      fxInputRef.current.focus();
      fxInputRef.current.setSelectionRange(c, c);
      pendingBarCaretRef.current = null;
      syncBackdropScroll();
    }
  });

  const onFxInput = (e: TargetedEvent<HTMLInputElement>): void => {
    const el = e.currentTarget;
    const v = el.value;
    setFx(v);
    setAcDismissed(false);
    setAcIndex(0);
    refInsertRef.current = null;
    if (v.startsWith('='))
      setFxEdit({ surface: 'bar', text: v, caret: el.selectionStart ?? v.length });
    else setFxEdit(null);
    syncBackdropScroll();
  };

  /** Foco o movimiento del cursor en la barra: arranca/actualiza la UX de fórmula.
   *  Solo si el input sigue enfocado (evita reabrirla tras confirmar y saltar). */
  const onFxCaret = (e: TargetedEvent<HTMLInputElement>): void => {
    const el = e.currentTarget;
    if (document.activeElement !== el || !el.value.startsWith('=')) return;
    // Celda bloqueada (hoja protegida): la barra es de solo lectura; no arranca la
    // UX de edición de fórmula (ni point-and-click sobre una barra que no se escribe).
    if (!canEditAt(stateRef.current.sel)) return;
    refInsertRef.current = null;
    setFxEdit({ surface: 'bar', text: el.value, caret: el.selectionStart ?? el.value.length });
  };

  const onFxBlur = (): void => {
    if (fxEditRef.current?.surface === 'bar') endFx();
  };

  const onFxKeyDown = (e: KeyboardEvent): void => {
    const fe = fxEditRef.current;
    if (fe?.surface === 'bar') {
      const ac = acAt(fe.text, fe.caret);
      const acOpen = !!ac && !acDismissed;
      if (acOpen && (e.key === 'ArrowDown' || e.key === 'ArrowUp')) {
        e.preventDefault();
        const n = ac.items.length;
        setAcIndex((i) => (e.key === 'ArrowDown' ? (i + 1) % n : (i - 1 + n) % n));
        return;
      }
      if (acOpen && (e.key === 'Enter' || e.key === 'Tab')) {
        e.preventDefault();
        const it = ac.items[Math.min(acIndex, ac.items.length - 1)];
        if (it) acceptAc('bar', it);
        return;
      }
      if (acOpen && e.key === 'Escape') {
        e.preventDefault();
        setAcDismissed(true);
        return;
      }
      if (e.key === 'F4') {
        e.preventDefault();
        doF4('bar');
        return;
      }
    }
    if (e.key !== 'Enter') return;
    e.preventDefault();
    const cur = stateRef.current;
    const s = cur.model.sheets[cur.model.active];
    if (!s) return;
    const ref = refOf(cur.sel);
    if (!canEditAt(cur.sel)) {
      notifyProtected(ref);
      return;
    }
    const dv = validationOf(s, cur.sel.c, cur.sel.r);
    if (dv?.strict && !validateValue(dv, fx, () => dvFormulaOk(dv, cur.sel.c, cur.sel.r, fx))) {
      notify({
        tone: 'warning',
        title: t('sheet.msg.badValue'),
        description: validationMessage(dv, ref),
      });
      return;
    }
    // Texto escapado (§7.1) conservando negrita/cursiva/alineación de la celda.
    const p = parseCell(s.cells[ref] ?? '');
    p.inner = escapeHtml(fx);
    const html = renderCellHtml(p);
    const cells = { ...s.cells };
    if (html === '') delete cells[ref];
    else cells[ref] = html;
    applyCells(cells);
    endFx();
    move(0, 1, false);
    gridRef.current?.focus();
  };

  /* ----- eventos delegados de la rejilla (2600 celdas sin closures por celda) ----- */

  const onGridMouseDown = (e: MouseEvent): void => {
    const target = e.target as HTMLElement;
    // Los botones que viven DENTRO de la rejilla (esquema, filtro, validación) no
    // son superficie de selección: sin este corte, pulsar el filtro de una columna
    // seleccionaría además la columna entera que hay debajo del desplegable.
    if (target.closest('button')) return;
    const el = target.closest<HTMLElement>('[data-ref]');
    const cur = stateRef.current;
    const fe = fxEditRef.current;
    // Point-and-click sobre CABECERAS: con una fórmula en edición lista para una
    // ref, clicar una cabecera de COLUMNA inserta la columna entera (A:A) y una de
    // FILA la fila entera (1:1); arrastrar por varias extiende (A:C / 1:3).
    if (!el && fe && fe.text.startsWith('=') && pickReady(fe)) {
      const colHead = target.closest<HTMLElement>('[data-colhead]');
      const rowHead = colHead ? null : target.closest<HTMLElement>('[data-rowhead]');
      if (colHead) {
        e.preventDefault(); // no robar el foco del editor ni cambiar la selección
        const c = Number(colHead.dataset['colhead']);
        pickDragRef.current = { anchor: { c, r: cur.sel.r }, kind: 'col' };
        insertHeaderRef(fe, 'col', c, c);
        return;
      }
      if (rowHead) {
        e.preventDefault();
        const r = Number(rowHead.dataset['rowhead']);
        pickDragRef.current = { anchor: { c: cur.sel.c, r }, kind: 'row' };
        insertHeaderRef(fe, 'row', r, r);
        return;
      }
    }
    if (!el) {
      // Cabeceras y esquina: clicar una cabecera selecciona la COLUMNA o la FILA
      // entera, y la esquina la hoja completa (gesto de Excel y de Numbers).
      // Arrastrar por las cabeceras extiende a varias. Sin esto, dar formato a una
      // columna obligaba a arrastrar —o a Mayús+clic— sobre sus 100 celdas.
      const colHead = target.closest<HTMLElement>('[data-colhead]');
      const rowHead = target.closest<HTMLElement>('[data-rowhead]');
      const corner = target.closest<HTMLElement>('[data-selectall]');
      if (!colHead && !rowHead && !corner) return;
      e.preventDefault();
      commitOpenEdit();
      if (corner) {
        setSel({ c: 0, r: 0 });
        setExt({ c: nCols - 1, r: nRows - 1 });
      } else if (colHead) {
        // Mayús+clic extiende desde la columna ancla; el ancla baja a la fila 1
        // para que el rango cubra las columnas ENTERAS y no medio rectángulo.
        const c = Number(colHead.dataset['colhead']);
        setSel({ c: e.shiftKey ? cur.sel.c : c, r: 0 });
        setExt({ c, r: nRows - 1 });
        selDragRef.current = 'col';
      } else if (rowHead) {
        const r = Number(rowHead.dataset['rowhead']);
        setSel({ c: 0, r: e.shiftKey ? cur.sel.r : r });
        setExt({ c: nCols - 1, r });
        selDragRef.current = 'row';
      }
      gridRef.current?.focus();
      return;
    }
    const pos = posOf(el);
    // Point-and-click: editando una fórmula que ESPERA (o acaba de insertar) una
    // referencia, un clic en la rejilla la inserta y la fórmula SIGUE en edición.
    const canPick =
      !!fe &&
      fe.text.startsWith('=') &&
      !(fe.surface === 'cell' && refOf(pos) === refOf(cur.sel)) && // no la propia celda en edición
      pickReady(fe);
    if (canPick && fe) {
      e.preventDefault(); // no robar el foco del editor ni cambiar la selección
      // Mayús+clic extiende la ref recién insertada desde su ancla (rango); si no
      // hay ninguna pendiente (el usuario tecleó), ancla aquí como un clic normal.
      const anchor =
        e.shiftKey && refInsertRef.current && pickAnchorRef.current ? pickAnchorRef.current : pos;
      pickAnchorRef.current = anchor;
      pickDragRef.current = { anchor };
      insertRefFromPick(fe, anchor, pos);
      return;
    }
    if (editing && refOf(pos) === refOf(sel)) return; // caret libre en la celda en edición
    e.preventDefault(); // clic = seleccionar; el foco (y el caret) llegan con doble clic
    commitOpenEdit();
    if (!canSelectAt(pos)) {
      // Hoja protegida sin «seleccionar celdas»: la celda bloqueada no se selecciona.
      gridRef.current?.focus();
      return;
    }
    select(pos, e.shiftKey);
    selDragRef.current = 'cell'; // clic y arrastrar = marcar rango (§ gesto de toda hoja)
    gridRef.current?.focus();
  };

  /** Cierra la edición abierta confirmando la celda (clic fuera de ella). */
  const commitOpenEdit = (): void => {
    if (!stateRef.current.editing) return;
    const prev = cellEl(stateRef.current.sel);
    if (prev) commitCell(stateRef.current.sel, prev);
    setEditing(false);
    endFx();
  };

  /** Arrastre de point-and-click: extiende el rango insertado hasta la celda bajo el
   *  puntero. Con arrastre por cabeceras (`kind`), extiende la columna/fila entera
   *  (la posición geométrica bajo el puntero da el eje; el otro se ignora). */
  const onGridMouseMove = (e: MouseEvent): void => {
    // Arrastre de SELECCIÓN: clic y arrastrar marca el rango, que es como se
    // selecciona en cualquier hoja de cálculo. Si se suelta el botón fuera de la
    // rejilla no llega el mouseup, así que el propio `buttons` cierra el arrastre.
    const sd = selDragRef.current;
    if (sd) {
      if (e.buttons === 0) {
        selDragRef.current = null;
        return;
      }
      const p = posFromPoint(e.clientX, e.clientY);
      if (!p) return;
      const cur = stateRef.current;
      const to =
        sd === 'col' ? { c: p.c, r: nRows - 1 } : sd === 'row' ? { c: nCols - 1, r: p.r } : p;
      if (sd === 'cell' && !canSelectAt(to)) return;
      if (cur.ext && cur.ext.c === to.c && cur.ext.r === to.r) return; // sin cambio: sin re-render
      // Volver sobre la celda ancla deshace el rango: si no, un temblor del ratón
      // dejaba «B2:B2» en la barra de referencia y el resumen de selección abierto.
      setExt(sd === 'cell' && to.c === cur.sel.c && to.r === cur.sel.r ? null : to);
      return;
    }
    const drag = pickDragRef.current;
    if (!drag) return;
    const fe = fxEditRef.current;
    if (!fe) return;
    if (drag.kind) {
      const p = posFromPoint(e.clientX, e.clientY);
      if (p)
        insertHeaderRef(
          fe,
          drag.kind,
          drag.kind === 'col' ? drag.anchor.c : drag.anchor.r,
          drag.kind === 'col' ? p.c : p.r,
        );
      return;
    }
    const el = (e.target as HTMLElement).closest<HTMLElement>('[data-ref]');
    const pos = el ? posOf(el) : posFromPoint(e.clientX, e.clientY);
    if (pos) insertRefFromPick(fe, drag.anchor, pos);
  };

  const onGridMouseUp = (e: MouseEvent): void => {
    pickDragRef.current = null;
    selDragRef.current = null;
    // Clic dentro de la propia celda en edición: recoloca el cursor del hint.
    const el = (e.target as HTMLElement).closest<HTMLElement>('[data-ref]');
    const cur = stateRef.current;
    if (
      el &&
      cur.editing &&
      fxEditRef.current?.surface === 'cell' &&
      refOf(posOf(el)) === refOf(cur.sel)
    ) {
      const text = readCellText(el);
      if (text.startsWith('='))
        setFxEdit({ surface: 'cell', text, caret: caretOffsetIn(el) ?? text.length });
    }
  };

  const onGridDblClick = (e: MouseEvent): void => {
    const el = (e.target as HTMLElement).closest<HTMLElement>('[data-ref]');
    if (el) startEdit(posOf(el));
  };

  /** Teclear en la celda en edición: re-tinta la fórmula y sigue el cursor. */
  const onGridInput = (e: Event): void => {
    const el = e.target as HTMLElement;
    if (!el.dataset['ref'] || !stateRef.current.editing) return;
    const text = readCellText(el);
    // Solo actúa en fórmulas o en una celda YA tintada (no toca literales normales).
    if (!text.startsWith('=') && !el.querySelector('span')) return;
    setAcDismissed(false);
    setAcIndex(0);
    refInsertRef.current = null;
    syncCellFx(el, true);
  };

  /** Movimiento de cursor con flechas dentro de la celda: actualiza el hint. */
  const onGridKeyUp = (e: KeyboardEvent): void => {
    const el = e.target as HTMLElement;
    if (!el.dataset['ref'] || fxEditRef.current?.surface !== 'cell') return;
    if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight' && e.key !== 'Home' && e.key !== 'End')
      return;
    const text = readCellText(el);
    if (!text.startsWith('=')) return;
    refInsertRef.current = null;
    setFxEdit({ surface: 'cell', text, caret: caretOffsetIn(el) ?? text.length });
  };

  const onGridCompositionStart = (): void => {
    composingRef.current = true;
  };
  const onGridCompositionEnd = (e: Event): void => {
    composingRef.current = false;
    const el = e.target as HTMLElement;
    if (el.dataset?.['ref'] && readCellText(el).startsWith('=')) syncCellFx(el, true);
  };

  const onGridFocusIn = (e: FocusEvent): void => {
    const el = e.target as HTMLElement;
    if (!el.dataset['ref']) return;
    setEditorRoot(el); // Inicio/exec formatean texto dentro de la celda (§7)
    if (!stateRef.current.editing) {
      // Foco por Tab del navegador (o startEdit): entrar en la celda es editarla.
      const pos = posOf(el);
      if (!canEditAt(pos)) {
        // Celda bloqueada (hoja protegida): foco sin entrar en edición.
        setSel(pos);
        setExt(null);
        return;
      }
      showEditSource(pos, el); // fórmula → editar el «=…» crudo, no el valor
      setSel(pos);
      setExt(null);
      setEditing(true);
    }
    // Arranca la UX de fórmulas si el contenido es una fórmula.
    if (readCellText(el).startsWith('=')) syncCellFx(el, true);
    else endFx();
  };

  const onGridFocusOut = (e: FocusEvent): void => {
    const el = e.target as HTMLElement;
    if (!el.dataset['ref'] || !stateRef.current.editing) return;
    // Perder el foco confirma SIEMPRE la celda. El point-and-click de rangos no
    // pasa por aquí: onGridMouseDown hace preventDefault(), así que elegir una
    // celda mientras editas no dispara este focusout.
    commitCell(posOf(el), el);
    setEditing(false);
    endFx();
  };

  const onGridKeyDown = (e: KeyboardEvent): void => {
    const target = e.target as HTMLElement;
    if (target.dataset['ref']) {
      // Teclas dentro de la celda en edición.
      const fe = fxEditRef.current;
      if (fe?.surface === 'cell') {
        const ac = acAt(fe.text, fe.caret);
        const acOpen = !!ac && !acDismissed;
        if (acOpen && (e.key === 'ArrowDown' || e.key === 'ArrowUp')) {
          e.preventDefault();
          const n = ac.items.length;
          setAcIndex((i) => (e.key === 'ArrowDown' ? (i + 1) % n : (i - 1 + n) % n));
          return;
        }
        if (acOpen && (e.key === 'Enter' || e.key === 'Tab')) {
          e.preventDefault();
          const it = ac.items[Math.min(acIndex, ac.items.length - 1)];
          if (it) acceptAc('cell', it);
          return;
        }
        if (acOpen && e.key === 'Escape') {
          e.preventDefault();
          setAcDismissed(true);
          return;
        }
        if (e.key === 'F4') {
          e.preventDefault();
          doF4('cell');
          return;
        }
      }
      // FLECHAS SIN FÓRMULA EN CURSO: confirman y mueven, arrastrando el foco a la
      // celda nueva. Es lo que hacen Excel y Sheets, y lo que faltaba: al escribir
      // texto normal, la flecha se quedaba moviendo el caret dentro de la celda —el
      // manejador de navegación de más abajo se corta en `target !== gridRef.current`—
      // así que la selección no avanzaba y había que pulsar Enter para salir.
      //
      // Solo cuando NO hay fórmula: con `=` a medias, las flechas sirven para señalar
      // referencias en la rejilla («=A1+» y flecha abajo apunta a la celda), que es la
      // otra mitad del comportamiento de una hoja de cálculo y se pierde si esto se
      // aplica siempre. `fxEditRef` es quien lo sabe.
      //
      // Solo VERTICALES a propósito. Izquierda/derecha mueven el caret dentro del
      // texto: es el modo «Modificar» de Excel, y es lo que espera quien está
      // corrigiendo una palabra a mitad de celda. Arriba/abajo no tienen ese doble
      // sentido, así que ahí no hay ambigüedad que resolver.
      const enFormula = fxEditRef.current?.surface === 'cell';
      if (!enFormula && (e.key === 'ArrowUp' || e.key === 'ArrowDown')) {
        e.preventDefault();
        finishEdit(target, 0, e.key === 'ArrowDown' ? 1 : -1);
        return;
      }
      if (e.key === 'Enter') {
        e.preventDefault();
        finishEdit(target, 0, 1);
      } else if (e.key === 'Tab') {
        e.preventDefault();
        // Mayús+Tab retrocede: ignorar shiftKey movía a la derecha en ambos casos.
        finishEdit(target, e.shiftKey ? -1 : 1, 0);
      } else if (e.key === 'Escape') {
        e.preventDefault();
        cancelEdit(target);
      } else {
        // Dentro de la celda se edita texto: sus atajos son los de `exec` (registro).
        const deTexto = comandoDelAtajo(e, 'document');
        if (deTexto) {
          e.preventDefault();
          ctx.exec(deTexto.id as EditorCommand, deTexto.value);
        }
      }
      return;
    }
    if (target !== gridRef.current) return;
    // Navegación con la rejilla enfocada (§7.1): flechas mueven; Mayús extiende el rango.
    const extend = e.shiftKey;
    const atajo = comandoDelAtajo(e, 'spreadsheet');
    const arrow =
      e.key === 'ArrowUp'
        ? [0, -1]
        : e.key === 'ArrowDown'
          ? [0, 1]
          : e.key === 'ArrowLeft'
            ? [-1, 0]
            : e.key === 'ArrowRight'
              ? [1, 0]
              : null;
    if (arrow) {
      e.preventDefault();
      // Ctrl (Cmd en Mac) salta al borde de los datos; sin él, una celda.
      if (e.ctrlKey || e.metaKey) moveToData(arrow[0]!, arrow[1]!, extend);
      else move(arrow[0]!, arrow[1]!, extend);
    } else if (e.key === 'Home') {
      // Inicio → columna A de la fila; Ctrl/Cmd+Inicio → A1 (criterio Excel).
      e.preventDefault();
      move(-MAX_COLS, e.ctrlKey || e.metaKey ? -MAX_ROWS : 0, extend);
    } else if (e.key === 'End') {
      // Fin → última celda con contenido de la fila; Ctrl/Cmd+Fin → la de la hoja.
      e.preventDefault();
      const cur = stateRef.current;
      const base = extend ? (cur.ext ?? cur.sel) : cur.sel;
      const to = lastUsedCell(e.ctrlKey || e.metaKey ? undefined : base.r) ?? base;
      move(to.c - base.c, to.r - base.r, extend);
    } else if (e.key === 'PageDown' || e.key === 'PageUp') {
      // Sin esto, recorrer una hoja larga con el teclado iba fila a fila.
      e.preventDefault();
      move(0, e.key === 'PageDown' ? pageRows() : -pageRows(), extend);
    } else if (e.key === 'Enter' || e.key === 'F2') {
      e.preventDefault();
      startEdit(sel);
    } else if (e.key === 'Tab') {
      e.preventDefault();
      // Mayús+Tab retrocede (antes ambos avanzaban a la derecha).
      move(e.shiftKey ? -1 : 1, 0, false);
    } else if (e.key === 'Delete' || e.key === 'Backspace') {
      e.preventDefault();
      clearRange();
    } else if (e.key === 'Escape') {
      // Salida por teclado de la rejilla: Tab está interceptado para navegar entre
      // celdas, así que sin esto no había forma de abandonar el role=grid solo con
      // teclado (trampa de teclado, WCAG 2.1.2). Escape cierra primero la selección
      // extendida y, si no hay ninguna, suelta el foco.
      if (ext) setExt(null);
      else gridRef.current?.blur();
    } else if ((e.ctrlKey || e.metaKey) && !e.altKey && e.key.toLowerCase() === 'a') {
      // Sin esto, Ctrl+A caía en el «seleccionar todo» del navegador y marcaba el
      // texto de la página entera —cinta incluida— en vez de las celdas.
      e.preventDefault();
      setSel({ c: 0, r: 0 });
      setExt({ c: nCols - 1, r: nRows - 1 });
    } else if ((e.ctrlKey || e.metaKey) && !e.altKey && e.key.toLowerCase() === 'z') {
      // Ctrl+Z y Ctrl+Mayús+Z, más Ctrl+Y para quien viene de Windows. Sin esto,
      // ordenar un rango o pegar encima no tenía vuelta atrás: el único remedio
      // era cerrar sin guardar y perder también todo lo bueno de después.
      e.preventDefault();
      if (e.shiftKey) irAlante();
      else irAtras();
    } else if ((e.ctrlKey || e.metaKey) && !e.altKey && e.key.toLowerCase() === 'y') {
      e.preventDefault();
      irAlante();
    } else if (atajo) {
      // Negrita, cursiva, copiar, cortar y pegar: los atajos del registro de comandos.
      e.preventDefault();
      dispatchSheetCommand(atajo.id as SheetCommand, atajo.value);
    } else if (e.key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey) {
      // Teclear = editar; como en toda hoja de cálculo, lo tecleado sustituye al contenido.
      e.preventDefault();
      startEdit(sel, e.key);
    }
  };

  /* ----- pestañas de hoja (§7.1) ----- */

  /** Backstop de la protección de LIBRO: veta las mutaciones de ESTRUCTURA
   *  (añadir/borrar/renombrar/duplicar/reordenar hojas) y avisa. Los botones
   *  también se deshabilitan, pero renombrar (doble clic) y reordenar (arrastrar)
   *  no tienen botón, así que el veto vive aquí. Cambiar de hoja NO pasa por aquí. */
  const guardStructure = (): boolean => {
    if (canEditStructure(model.protect)) return true;
    notify({
      tone: 'warning',
      title: t('sheet.msg.protectedBook'),
      description: t('sheet.msg.unprotectToEdit'),
    });
    return false;
  };

  const switchSheet = (i: number): void => {
    if (i === model.active) return;
    apply({ ...model, active: i }); // el cambio de hoja también se persiste (§7.1)
    setSel({ c: 0, r: 0 });
    setExt(null);
    setEditing(false);
    setFilterCol(null);
  };

  const addSheet = (): void => {
    if (!guardStructure()) return;
    apply({
      ...model,
      active: model.sheets.length,
      sheets: [
        ...model.sheets,
        {
          id: nuevoIdHoja(),
          name: t('sheet.defaultName', { n: model.sheets.length + 1 }),
          cells: {},
        },
      ],
    });
    setSel({ c: 0, r: 0 });
    setExt(null);
    setFilterCol(null);
  };

  const renameSheet = async (i: number): Promise<void> => {
    if (!guardStructure()) return;
    const current = model.sheets[i];
    if (!current) return;
    const name = (
      await preguntarEnEditor({
        mensaje: editorT()('dialog.sheet.rename'),
        valor: current.name,
      })
    )?.trim();
    if (!name || name === current.name) return;
    apply(
      withNames(
        { ...model, sheets: model.sheets.map((s, j) => (j === i ? { ...s, name } : s)) },
        renombrarHojaEnNombres(model.names, current.name, name),
      ),
    );
  };

  /* ----- UX multi-hoja (fase Visual): duplicar, borrar y reordenar ----- */

  const duplicateSheet = (): void => {
    if (!guardStructure()) return;
    const src = model.sheets[model.active];
    if (!src) return;
    // Nombre único: el motor de fórmulas indexa las hojas por nombre.
    const names = new Set(model.sheets.map((s) => s.name));
    let n = 2;
    while (names.has(`${src.name} (${n})`)) n++;
    // structuredClone copia en profundidad cells+formats+condFormats+freeze+charts.
    const copy: SheetData = {
      ...structuredClone(src),
      id: nuevoIdHoja(),
      name: `${src.name} (${n})`,
    };
    if (copy.charts) copy.charts = copy.charts.map((ch) => ({ ...ch, id: crypto.randomUUID() }));
    if (copy.slicers) copy.slicers = copy.slicers.map((sl) => ({ ...sl, id: crypto.randomUUID() }));
    // ponytail: los nombres de hoja del original no se duplican con ella.
    apply({
      ...model,
      active: model.active + 1,
      sheets: [
        ...model.sheets.slice(0, model.active + 1),
        copy,
        ...model.sheets.slice(model.active + 1),
      ],
    });
    setSel({ c: 0, r: 0 });
    setExt(null);
    setEditing(false);
    setFilterCol(null);
  };

  const deleteSheet = async (): Promise<void> => {
    if (!guardStructure()) return;
    if (model.sheets.length <= 1) return; // siempre queda al menos una hoja
    const name = model.sheets[model.active]?.name ?? '';
    if (
      !(await confirmarEnEditor({
        mensaje: editorT()('dialog.sheet.delete', { name }),
        confirmLabel: editorT()('sheet.delete'),
        tone: 'danger',
      }))
    )
      return;
    const sheets = model.sheets.filter((_, i) => i !== model.active);
    apply(
      withNames(
        { ...model, active: Math.min(model.active, sheets.length - 1), sheets },
        quitarHojaDeNombres(model.names, name),
      ),
    );
    setSel({ c: 0, r: 0 });
    setExt(null);
    setEditing(false);
    setFilterCol(null);
  };

  /** Índice de la pestaña en arrastre (reordenar con drag and drop nativo). */
  const sheetDrag = useRef<number | null>(null);

  const moveSheetTo = (from: number, to: number): void => {
    if (from === to) return;
    if (!guardStructure()) return;
    const sheets = [...model.sheets];
    const [moved] = sheets.splice(from, 1);
    if (!moved) return;
    sheets.splice(to, 0, moved);
    const activeSheet = model.sheets[model.active];
    apply({
      ...model,
      active: Math.max(
        0,
        sheets.findIndex((s) => s === activeSheet),
      ),
      sheets,
    });
  };

  /* ----- render ----- */

  const refLabel = ext
    ? `${refOf({ c: rect.c1, r: rect.r1 })}:${refOf({ c: rect.c2, r: rect.r2 })}`
    : refOf(sel);

  /* Resumen de la selección (recuento · suma · promedio), como la barra de estado
   * de Excel y de Numbers: saber cuánto suma lo que acabas de marcar no debería
   * obligar a escribir una fórmula en una celda de más y luego borrarla. Solo con
   * rango marcado —una celda suelta no tiene nada que resumir— y saltando lo que
   * el filtro o el esquema ocultan, que es el criterio de Excel. */
  const hayRango = ext !== null;
  const selStats = useMemo((): SelectionStats | null => {
    if (!hayRango) return null;
    const values: CellValue[] = [];
    // Más allá de lo usado todo está vacío (no cuenta ni suma): con la hoja entera
    // marcada no se recorre la extensión, solo los datos.
    const u = engine.usedRange(sheet.name);
    const r2 = Math.min(rect.r2, u.maxRow);
    const c2 = Math.min(rect.c2, u.maxCol);
    for (let r = rect.r1; r <= r2; r++) {
      if (hidden.has(r)) continue;
      for (let c = rect.c1; c <= c2; c++) {
        if (!hiddenCols.has(c)) values.push(engine.getValue(sheet.name, A1(c, r)));
      }
    }
    return selectionStats(values);
    // `model` va en las dependencias aunque el cuerpo no lo lea: el motor es la MISMA instancia
    // entre ediciones (ver motor-vivo), así que lo único que avisa de que cambió una celda,
    // incluso de OTRA hoja, es el modelo.
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, [hayRango, engine, model, sheet, rect.r1, rect.r2, rect.c1, rect.c2, hidden, hiddenCols]);

  // Protección: ¿la hoja está protegida? ¿se puede editar la celda ancla?
  const protectedSheet = !!sheet.protect;
  // Protección de LIBRO: estructura bloqueada (deshabilita botones y arrastre de pestañas).
  const protectedWorkbook = !!model.protect;
  const selRef = refOf(sel);
  const canEditSel = canEditCellContent(
    protectedSheet,
    estiloParaMi(sheet.protect, selRef, sheet.styles?.[selRef], yo()),
    sheet.cells[selRef] ?? '',
  );

  const headCls =
    // px y NO `h-6`/`leading-6`: las clases de Tailwind son rem y la geometría
    // de la rejilla (HEAD_W/CELL_W/CELL_H) son px absolutos. Con la base del
    // navegador en 14px en vez de 16, `h-6` mide 21px y el JS sigue calculando
    // con 24 — las dos capas se separan. Medido: 5px de desfase entre la
    // esquina (width:40px por estilo) y el encabezado de fila (`w-10` = 35px).
    'h-[24px] shrink-0 select-none border-b border-r border-border text-center text-xs leading-[24px]';

  /* ----- Esquema: canalón (gutter) con botones +/−, brackets y barra de niveles ----- */

  /** Grupo cuyo RESUMEN (end+1) es esta unidad → botón +/− en su carril. Si el grupo
   *  llega a la ÚLTIMA unidad de la rejilla (no existe end+1), el botón cae en la
   *  propia `end`, para no quedar fuera de la rejilla y perder el contraer/expandir. */
  const laneToggle = (
    groups: readonly SheetGroup[],
    unit: number,
    level: number,
    max: number,
  ): SheetGroup | null =>
    groups.find(
      (g) => g.level === level && (g.end + 1 === unit || (g.end === max - 1 && g.end === unit)),
    ) ?? null;
  /** ¿Alguna unidad de un grupo de este nivel cubre `unit`? → línea de bracket. */
  const laneLine = (groups: readonly SheetGroup[], unit: number, level: number): boolean =>
    groups.some((g) => g.level === level && unit >= g.start && unit <= g.end);

  /** Botón +/− que conmuta el colapso de un grupo (eje fila/columna). */
  const toggleBtn = (axis: 'row' | 'col', g: SheetGroup): JSX.Element => (
    <button
      type="button"
      class={`border-border bg-surface text-text-muted hover:bg-selected hover:text-text text-2xs flex h-3 w-3 items-center justify-center rounded-[2px] border leading-none ${FOCUS_RING}`}
      aria-label={t(g.collapsed ? 'sheet.outline.expand' : 'sheet.outline.collapse')}
      aria-expanded={!g.collapsed}
      title={t(g.collapsed ? 'sheet.outline.expand' : 'sheet.outline.collapse')}
      onMouseDown={(e) => e.preventDefault()}
      onClick={() =>
        dispatchSheetCommand(
          'outline',
          JSON.stringify({ axis, op: 'toggle', start: g.start, end: g.end, level: g.level }),
        )
      }
    >
      {g.collapsed ? '+' : '−'}
    </button>
  );

  /** Barra de niveles 1..N+1 (mostrar hasta un nivel de esquema). */
  const levelBar = (axis: 'row' | 'col', levels: number): JSX.Element | null =>
    levels === 0 ? null : (
      <div
        class="flex items-center gap-px"
        role="group"
        aria-label={
          axis === 'col' ? t('sheet.outline.levels.cols') : t('sheet.outline.levels.rows')
        }
      >
        {Array.from({ length: levels + 1 }, (_, i) => i + 1).map((lvl) => (
          <button
            key={lvl}
            type="button"
            // 11 px es el suelo tipográfico del sistema; iba a 9. El número cabe igual en
            // la casilla de 14: el esquema llega a 7 niveles (MAX_OUTLINE_LEVEL), o sea
            // que nunca pasa de una cifra.
            class={`border-border bg-surface text-text-muted hover:bg-selected hover:text-text text-2xs flex h-3.5 w-3.5 items-center justify-center rounded-[2px] border leading-none ${FOCUS_RING}`}
            aria-label={t('sheet.outline.showLevel', { n: lvl })}
            title={t('sheet.outline.showUpTo', { n: lvl })}
            onMouseDown={(e) => e.preventDefault()}
            onClick={() =>
              dispatchSheetCommand('outline', JSON.stringify({ axis, op: 'level', level: lvl }))
            }
          >
            {lvl}
          </button>
        ))}
      </div>
    );

  /** Carriles del canalón de FILAS de una fila (horizontal: carril = nivel). */
  const rowGutterCell = (r: number): JSX.Element | null =>
    RGUT === 0 ? null : (
      <div
        key="esquema"
        role="presentation"
        class="border-border bg-bg-subtle sticky left-0 z-10 flex shrink-0 border-r border-b"
        style={{ left: 0, width: `${RGUT}px`, height: '100%' }}
      >
        {Array.from({ length: rowLevels }, (_, l) => {
          const tg = laneToggle(rowGroups, r, l + 1, nRows);
          return (
            <div
              key={l}
              class="relative flex items-center justify-center"
              style={{ width: `${OUTLINE_STEP}px`, height: '100%' }}
            >
              {tg ? (
                toggleBtn('row', tg)
              ) : laneLine(rowGroups, r, l + 1) ? (
                <span
                  aria-hidden="true"
                  class="bg-border-strong"
                  style={{ width: '1px', height: '100%' }}
                />
              ) : null}
            </div>
          );
        })}
      </div>
    );

  /* ----- Ventana: lo que se pinta de verdad -----
   * Las filas y columnas inmovilizadas van siempre: son pegajosas y van EN FLUJO, al
   * principio de su fila. Del resto solo el tramo que se ve con un margen, colocado en
   * absoluto en su sitio (colX y OY + vi·CELL_H, la misma geometría de las capas). Y
   * siempre la fila y la columna de la celda activa: la edición en curso vive en su
   * DOM y el aria-activedescendant apunta a ella, así que no pueden desaparecer porque
   * alguien se desplace lejos con una fórmula a medias. */
  let nFzR = 0;
  while (nFzR < visRows.length && visRows[nFzR]! < freeze.rows) nFzR++;
  let nFzC = 0;
  while (nFzC < visCols.length && visCols[nFzC]! < freeze.cols) nFzC++;
  const filasV = rowTop
    ? tramoPorPosiciones(rowTop, vista.top, vista.h, nFzR, visRows.length, 'filas')
    : tramo(vista.top, vista.h, OY, CELL_H, nFzR, visRows.length, 'filas');
  const colsV = hayAnchos
    ? tramoPorPosiciones(
        acumulado(OX, visCols, colW),
        vista.left,
        vista.w,
        nFzC,
        visCols.length,
        'cols',
      )
    : tramo(vista.left, vista.w, OX, CELL_W, nFzC, visCols.length, 'cols');
  const conActiva = (fijas: number, t: { desde: number; hasta: number }, i: number): number[] => {
    const out: number[] = [];
    for (let k = 0; k < fijas; k++) out.push(k);
    for (let k = t.desde; k < t.hasta; k++) out.push(k);
    if (i >= fijas && (i < t.desde || i >= t.hasta)) out.push(i);
    return out;
  };
  /** Índices visibles de las filas y columnas que se pintan (las fijas, primero). */
  const pintaFilas = conActiva(nFzR, filasV, viOf(sel.r));
  const pintaCols = conActiva(nFzC, colsV, indiceVisible(visCols, sel.c));
  /* Combinadas que toca lo que se pinta. Su ancla se pinta siempre (con su fila y su columna),
   * aunque quede fuera de la ventana, porque ocupa todo el rango y las cubiertas no se ven. */
  let fusVis: ReturnType<typeof fusionesEnVentana> = [];
  if (fusiones.length > 0 && pintaFilas.length > 0 && pintaCols.length > 0) {
    const rs = pintaFilas.map((vi) => visRows[vi]!);
    const cs = pintaCols.map((ci) => visCols[ci]!);
    fusVis = fusionesEnVentana(
      fusiones,
      Math.min(...cs),
      Math.min(...rs),
      Math.max(...cs),
      Math.max(...rs),
    );
    for (const f of fusVis) {
      const vi = viOf(f.r1);
      if (vi >= 0 && !pintaFilas.includes(vi)) pintaFilas.push(vi);
      const ci = indiceVisible(visCols, f.c1);
      if (ci >= 0 && !pintaCols.includes(ci)) pintaCols.push(ci);
    }
  }
  const gridW = colX[nCols]!;
  const gridH = topDe(visRows.length);
  const multi = rect.c1 !== rect.c2 || rect.r1 !== rect.r2;

  const headerCells: JSX.Element[] = [
    <div
      key="esquina"
      role="columnheader"
      aria-colindex={1}
      data-selectall="1"
      // La esquina selecciona la hoja entera (Excel/Numbers); nada en su aspecto lo
      // insinúa, así que el título es lo único que lo cuenta.
      title={t('sheet.selectAll')}
      class={`${headCls} bg-bg-subtle sticky left-0 z-30 flex cursor-cell items-center justify-end gap-0.5 px-0.5`}
      style={{ left: 0, width: `${OX}px` }}
    >
      {levelBar('row', rowLevels)}
    </div>,
  ];
  for (const ci of pintaCols) {
    const c = visCols[ci]!;
    const hl = c >= rect.c1 && c <= rect.c2;
    const fzCol = c < freeze.cols; // columna congelada: pegajosa también en horizontal
    headerCells.push(
      <div
        key={colName(c)}
        role="columnheader"
        aria-colindex={c + 2}
        data-colhead={c}
        class={`${headCls} w-[96px] cursor-cell ${hl ? 'sytue-grid-head-hl' : 'bg-bg-subtle text-text-muted'} ${
          fzCol ? 'sticky z-10' : 'absolute top-0'
        } ${c === freeze.cols - 1 ? 'border-r-border-strong' : ''}`}
        style={{ left: `${colX[c]}px`, ...(hayAnchos ? { width: `${colW(c)}px` } : {}) }}
      >
        {colName(c)}
      </div>,
    );
  }

  /* Validaciones de datos compiladas (mismo criterio: parse de rango una vez). */
  const validRects = compileValidations(sheet.validations ?? []);
  /* Validación de LISTA sobre la celda activa: habilita el desplegable in-situ
   * (no si la celda está bloqueada por protección: no se puede escribir en ella).
   * Solo la lista ofrece opciones; los demás criterios se comprueban al confirmar. */
  const selDv =
    validRects.length > 0 && !editing && canEditSel ? validationAt(validRects, sel.c, sel.r) : null;
  const selValidation =
    selDv && (selDv.type ?? 'list') === 'list' && (selDv.values?.length ?? 0) > 0 ? selDv : null;

  /* Solo la ventana (ver arriba). Las filas ocultas por el filtro no se renderizan
   * (los rowheader saltan numeración, como en Excel). Congelar = sticky: la FILA
   * entera con offset top acumulado y cada CELDA de columna congelada con offset
   * left; hairline 1px border-*-strong en el borde de la zona congelada. */
  const rows: JSX.Element[] = [];
  for (const vi of pintaFilas) {
    const r = visRows[vi]!;
    const rowHl = r >= rect.r1 && r <= rect.r2;
    const fzRow = r < freeze.rows;
    const rowEdge = r === freeze.rows - 1 ? ' border-b-border-strong' : '';
    const cells: JSX.Element[] = [];
    const rg = rowGutterCell(r);
    if (rg) cells.push(rg);
    cells.push(
      <div
        key="cabecera"
        role="rowheader"
        aria-colindex={1}
        data-rowhead={r}
        class={`${headCls} sticky z-10 w-[40px] cursor-cell ${rowHl ? 'sytue-grid-head-hl' : 'bg-bg-subtle text-text-muted'}${rowEdge}`}
        style={{ left: `${RGUT}px`, ...(hayAltos ? { height: `${altoDe(vi)}px` } : {}) }}
      >
        {r + 1}
      </div>,
    );
    for (const ci of pintaCols) {
      const c = visCols[ci]!;
      const ref = `${colName(c)}${r + 1}`;
      const isSel = sel.c === c && sel.r === r;
      const inRange = !isSel && c >= rect.c1 && c <= rect.c2 && r >= rect.r1 && r <= rect.r2;
      // En un rango el marco lo dibuja `rangeFrame`; el ancla solo lleva su aro si va sola.
      const ringSel = isSel && !multi;
      const cs = sheet.styles?.[ref];
      // Formato de tabla: cabecera de color, banda alterna y fondo de la fila de
      // totales. Es el relleno de MENOR prioridad (el condicional y el manual ganan).
      const tinfo = tableRects.length > 0 ? tableCellInfo(tableRects, c, r) : null;
      const tstyle = tinfo ? tableStyleOf(tinfo.ct.table.style) : null;
      let tableBg: string | null = null;
      if (tinfo && tstyle) {
        if (tinfo.role === 'header') tableBg = tstyle.headerBg;
        else if (tinfo.role === 'total') tableBg = tstyle.band;
        else if (tinfo.banded) tableBg = tstyle.band;
      }
      // El formato condicional (relleno/barra) que aplica gana; si no, el relleno manual; si no, la tabla.
      const decor = decorMap.get(ref);
      const fill = decor?.fill ?? cs?.fill ?? tableBg;
      const fzCol = c < freeze.cols;
      let style = cellStyle(ringSel, inRange, fill, decor?.bar);
      if (cs) style = { ...style, ...cellStyleCss(cs) }; // fuente/color/bordes/subrayado/valign/wrap
      // «Buscar»: amarillo en las celdas que casan y melocotón en la actual (la seleccionada).
      if (searchHits.has(`${c},${r}`)) {
        style = { ...style, backgroundColor: isSel ? HIT_ACTIVE_BG : HIT_BG, color: HIT_INK };
      }
      if (tinfo && tstyle) {
        // Cabecera: tinta del estilo (si el usuario no la fijó) + seminegrita; totales: seminegrita.
        if (tinfo.role === 'header') {
          style = { ...style, fontWeight: '600', ...(cs?.color ? {} : { color: tstyle.headerFg }) };
        } else if (tinfo.role === 'total') {
          style = { ...style, fontWeight: '600' };
        }
      }
      // El color de letra del formato condicional gana al del estilo y al de la tabla, como su
      // relleno; no al de «Buscar», que tiene que leerse sobre el amarillo.
      if (decor?.textColor && !searchHits.has(`${c},${r}`))
        style = { ...style, color: decor.textColor };
      // Validación no estricta: marca en rojo el valor que no cumple (avisar, no bloquear).
      // ponytail: se coteja el TEXTO crudo; las celdas con FÓRMULA no se marcan (evaluar
      // su resultado —o una validación tipo `formula`— pediría el motor en el render).
      if (validRects.length > 0) {
        const dv = validationAt(validRects, c, r);
        const raw = sheet.cells[ref] ?? '';
        if (
          dv &&
          !dv.strict &&
          raw !== '' &&
          !isFormula(raw) &&
          !validateValue(dv, stripHtml(raw))
        ) {
          style = { ...style, outline: `1px solid ${DANGER}`, outlineOffset: '-1px' };
        }
      }
      if (
        fillRect &&
        c >= fillRect.c1 &&
        c <= fillRect.c2 &&
        r >= fillRect.r1 &&
        r <= fillRect.r2
      ) {
        // Previsualización del autorelleno: contorno discontinuo del destino.
        style = { ...style, outline: `1px dashed ${ACCENT}`, outlineOffset: '-2px' };
      }
      style = {
        ...style,
        left: `${colX[c]}px`,
        ...(hayAnchos ? { width: `${colW(c)}px` } : {}),
        ...(hayAltos ? { height: `${altoDe(vi)}px` } : {}),
      };
      // Combinada: el ancla ocupa todo el rango (encima de las cubiertas) y las cubiertas no se ven.
      const fu = fusVis.length > 0 ? fusionEn(fusVis, c, r) : null;
      if (fu && fu.c1 === c && fu.r1 === r) {
        const c2 = Math.min(fu.c2, nCols - 1);
        const ultimoVi = Math.max(vi, primeroDesde(visRows, fu.r2 + 1) - 1);
        style = {
          ...style,
          width: `${colX[c2 + 1]! - colX[c]!}px`,
          height: `${topDe(ultimoVi + 1) - topDe(vi)}px`,
          zIndex: '2',
        };
      } else if (fu) {
        style = { ...style, visibility: 'hidden' };
      }
      // Editable = celda desbloqueada y sin fórmula oculta; contentEditable=false hace
      // que el propio navegador impida teclear (evita revelar/borrar la fórmula oculta).
      const cellEditable = canEditCellContent(
        protectedSheet,
        estiloParaMi(sheet.protect, ref, cs, yo()),
        sheet.cells[ref] ?? '',
      );
      cells.push(
        <div
          key={ref}
          // El `id` es lo que apunta el `aria-activedescendant` de la rejilla: sin él, un
          // lector de pantalla no tiene forma de saber en qué celda está el cursor.
          id={cellDomId(ref)}
          data-ref={ref}
          data-c={c}
          data-r={r}
          role="gridcell"
          aria-colindex={c + 2}
          aria-rowindex={r + 2}
          aria-selected={isSel || inRange}
          // ARIA 1.2 sí admite `aria-readonly` en `gridcell` (es como se anuncia una
          // celda de hoja protegida); la tabla de oxlint aún no lo recoge.
          // oxlint-disable-next-line jsx-a11y/role-supports-aria-props
          aria-readonly={protectedSheet && !cellEditable ? true : undefined}
          contentEditable={cellEditable}
          spellcheck={false}
          // `--editor-grid-line` y NO `border-border`: la celda se pinta sobre
          // --editor-page-bg, que es papel blanco en los DOS temas, mientras que
          // `border-border` es un token de tema que en oscuro se aclara para
          // contrastar contra fondos oscuros. El resultado era una línea casi
          // blanca sobre blanco y la cuadrícula desaparecía en modo noche. Es el
          // mismo motivo por el que la tinta de la regla tampoco sale de los
          // tokens de tema (ver editor.css).
          class={`h-[24px] w-[96px] shrink-0 overflow-hidden border-r border-b border-[var(--editor-grid-line)] bg-[var(--editor-page-bg)] px-1 text-sm leading-[24px] whitespace-nowrap tabular-nums outline-none ${
            fzCol ? 'sticky z-10' : 'absolute top-0'
          } ${c === freeze.cols - 1 ? 'border-r-border-strong' : ''} ${rowEdge}`}
          style={style}
          dangerouslySetInnerHTML={{ __html: cellHtml(ref) }}
        />,
      );
    }
    rows.push(
      <div
        key={r}
        role="row"
        aria-rowindex={r + 2}
        class={`flex ${fzRow ? 'sticky z-20 bg-[var(--editor-page-bg)]' : 'absolute left-0'}`}
        style={{ top: `${topDe(vi)}px`, width: `${gridW}px`, height: `${altoDe(vi)}px` }}
      >
        {cells}
      </div>,
    );
  }

  /* Asa de autorelleno: cuadrado en la esquina inferior-derecha de la selección.
   * ponytail: con z-10 queda tapada si la selección cae DENTRO de filas
   * congeladas (contexto z-20); subirla haría que flotara sobre las cabeceras
   * al hacer scroll. Caso raro; se asume. */
  /* Desplegable de validación de lista sobre la celda activa (afín al asa de
   * relleno: absoluto en el mismo origen que la rejilla). Oculto si la fila del
   * ancla está filtrada. */
  const selVi = editing ? -1 : viOf(sel.r);
  const selRaw = sheet.cells[refOf(sel)] ?? '';
  const validDropdown =
    selValidation && selVi >= 0 ? (
      <div
        ref={validRef}
        class="absolute z-10"
        style={{ left: `${colX[sel.c + 1]! - 18}px`, top: `${topDe(selVi)}px` }}
      >
        <button
          type="button"
          class={`border-border bg-bg-subtle text-text-muted hover:bg-selected hover:text-text text-2xs flex h-[24px] w-[18px] items-center justify-center border-l leading-none ${FOCUS_RING}`}
          aria-label={t('sheet.validation.pick', { ref: refOf(sel) })}
          aria-haspopup="listbox"
          aria-expanded={validOpen}
          title={t('sheet.pickFromList')}
          onMouseDown={(e) => e.preventDefault()}
          onClick={() => setValidOpen((v) => !v)}
        >
          ▾
        </button>
        {validOpen && (
          <ul
            role="listbox"
            aria-label={t('sheet.validation.allowed', { ref: refOf(sel) })}
            class="sytue-menu-surface border-border absolute top-full right-0 z-[var(--z-popover)] mt-0.5 max-h-56 w-max min-w-24 overflow-auto rounded-lg border py-1 shadow-[var(--shadow-md)]"
          >
            {(selValidation.values ?? []).map((val) => (
              <li key={val}>
                <button
                  type="button"
                  role="option"
                  aria-selected={stripHtml(selRaw) === val}
                  class={`text-text hover:bg-bg-subtle aria-selected:bg-selected block w-full px-2 py-0.5 text-left text-sm ${FOCUS_RING}`}
                  onMouseDown={(e) => e.preventDefault()}
                  onClick={() => pickValue(val)}
                >
                  {val}
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>
    ) : null;

  const fillHandleVi = editing ? -1 : viOf(rect.r2);
  const fillHandle =
    fillHandleVi < 0 ? null : (
      <div
        aria-hidden="true"
        class="absolute z-10 h-[7px] w-[7px] cursor-crosshair border border-[var(--editor-page-bg)]"
        style={{
          background: ACCENT,
          left: `${colX[rect.c2 + 1]! - 4}px`,
          top: `${topDe(fillHandleVi + 1) - 4}px`,
          touchAction: 'none',
        }}
        onPointerDown={onFillDown}
        onPointerMove={onFillMove}
        onPointerUp={onFillUp}
        onPointerCancel={onFillCancel}
      />
    );

  /* Autofiltro persistente: flechas en la fila de encabezados; al abrir una,
   * lista de valores distintos con casillas (excluir = ocultar filas). Valores
   * ordenados es-ES, «(Vacías)» al final. Solo se calcula la columna abierta. */
  const af = afVisto;
  const afHeaderVi = af ? viOf(af.row) : -1;
  const filterEntries = (col: number): SlicerEntry[] => {
    const texts: string[] = [];
    for (let r = (af?.row ?? 0) + 1; r < nRows; r++)
      texts.push(cellText(engine, sheet.name, col, r));
    return slicerEntries(texts, af?.excl?.[String(col)] ?? []);
  };
  /** Colores manuales (relleno o texto) que hay en el cuerpo de la columna, para filtrar por color. */
  const coloresDeColumna = (col: number, campo: 'fill' | 'color'): string[] => {
    const out = new Set<string>();
    for (let r = (af?.row ?? 0) + 1; r < nRows; r++) {
      const c = sheet.styles?.[refOf({ c: col, r })]?.[campo];
      if (c) out.add(c.toLowerCase());
    }
    return [...out];
  };
  /* Si la fila de encabezados del filtro está congelada (lo habitual: cabecera fija), sus
   * flechas viven DENTRO de esa fila pegajosa: como capa suelta quedaban bajo ella (z-20 contra
   * z-10) y no se veían ni se podían pulsar. */
  const filtroEnFila = !!af && afHeaderVi >= 0 && af.row < freeze.rows;
  const filterOverlay =
    af && afHeaderVi >= 0
      ? Array.from({ length: af.c2 - af.c1 + 1 }, (_, k) => af.c1 + k).map((c) => {
          const active =
            (af.excl?.[String(c)]?.length ?? 0) > 0 || af.cond?.[String(c)] !== undefined;
          const open = filterCol === c;
          return (
            <div
              key={`filter-${c}`}
              ref={open ? filterRef : null}
              class="absolute z-10"
              style={{
                left: `${colX[c + 1]! - 16}px`,
                top: filtroEnFila ? '0px' : `${topDe(afHeaderVi)}px`,
              }}
            >
              <button
                type="button"
                class={`mx-auto mt-[3px] flex h-[18px] w-[14px] items-center justify-center rounded-[3px] border bg-[var(--editor-page-bg)] ${FOCUS_RING} ${
                  active
                    ? 'border-[var(--editor-select)] text-[var(--editor-select)]'
                    : 'border-[var(--editor-grid-line)] text-[var(--sytue-neutral-600)] hover:border-[var(--editor-select)] hover:text-[var(--editor-select)]'
                }`}
                aria-label={t('sheet.filter.of', { col: colName(c) })}
                aria-haspopup="listbox"
                aria-expanded={open}
                title={t('sheet.filter.action', { col: colName(c) })}
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => {
                  setFilterCol(open ? null : c);
                  setCondOpen(false);
                }}
              >
                <svg width="8" height="8" viewBox="0 0 8 8" aria-hidden="true">
                  <path
                    d="M1 2.5 4 5.5 7 2.5"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="1.5"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                  />
                </svg>
              </button>
              {open && (
                /* Arranca en el borde izquierdo de la columna, como en Excel y Calc: pegado
                 * al «▾» por la derecha, en la columna A se salía de la rejilla y se cortaba. */
                <div
                  class="sytue-menu-surface border-border absolute top-full z-[var(--z-popover)] mt-0.5 max-h-56 w-max min-w-32 overflow-auto rounded-lg border py-1 shadow-[var(--shadow-md)]"
                  style={{ left: `${colX[c]! - (colX[c + 1]! - 16)}px` }}
                >
                  <FiltroVistasBarra
                    views={vistas.of(sheet.name)}
                    activeId={vistaActiva?.id ?? null}
                    onSelect={(id) => vistas.select(sheet.name, id)}
                    onSave={(name) =>
                      vistas.add(sheet.name, viewFromFilter(crypto.randomUUID(), name, af))
                    }
                    onDelete={() => vistaActiva && vistas.remove(sheet.name, vistaActiva.id)}
                    onPromote={
                      sheet.protect && sheet.protect.autoFilter !== true
                        ? null
                        : () => {
                            if (!vistaActiva || !sheet.autofilter) return;
                            applySheet({
                              autofilter: withCriterios(sheet.autofilter, vistaActiva),
                            });
                            vistas.select(sheet.name, null);
                          }
                    }
                  />
                  <div role="menu" class="px-1">
                    {(['asc', 'desc'] as const).map((dir) => (
                      <MenuItem
                        key={dir}
                        size="sm"
                        icon={
                          <EditorIcon name={dir === 'asc' ? 'sortAsc' : 'sortDesc'} size="md" />
                        }
                        onClick={() => {
                          setFilterCol(null);
                          dispatchSheetCommand('filtersort', JSON.stringify({ col: c, dir }));
                        }}
                      >
                        {t(dir === 'asc' ? 'data.sortAsc' : 'data.sortDesc')}
                      </MenuItem>
                    ))}
                    <MenuSeparator />
                    <MenuItem
                      size="sm"
                      icon={<EditorIcon name="filter" size="md" />}
                      onClick={() => setCondOpen(!condOpen)}
                    >
                      {t('filtro.byCondition')}
                    </MenuItem>
                    <MenuSeparator />
                  </div>
                  {condOpen ? (
                    <FiltroCondicion
                      current={af.cond?.[String(c)]}
                      fills={coloresDeColumna(c, 'fill')}
                      fonts={coloresDeColumna(c, 'color')}
                      onApply={(cond) => {
                        editarFiltro('filtercond', { col: c, cond });
                        setCondOpen(false);
                        setFilterCol(null);
                      }}
                      onBack={() => setCondOpen(false)}
                    />
                  ) : (
                    <ul role="listbox" aria-label={t('sheet.filter.values', { col: colName(c) })}>
                      {filterEntries(c).map((en) => (
                        <li key={en.value}>
                          <label class="text-text hover:bg-bg-subtle flex cursor-pointer items-center gap-2 px-2 py-0.5 text-xs">
                            <Checkbox
                              checked={!en.hidden}
                              class="accent-primary"
                              onChange={() =>
                                editarFiltro('filterset', {
                                  col: c,
                                  value: en.value,
                                  hidden: !en.hidden,
                                })
                              }
                            />
                            <span class="min-w-0 flex-1 truncate">
                              {en.value === '' ? t('slicer.blanks') : en.value}
                            </span>
                          </label>
                        </li>
                      ))}
                    </ul>
                  )}
                </div>
              )}
            </div>
          );
        })
      : null;
  if (filtroEnFila && filterOverlay && af) {
    const k = rows.findIndex((fila) => fila.key === af.row);
    const fila = rows[k];
    if (fila)
      rows[k] = cloneElement(
        fila,
        null,
        ...toChildArray((fila as VNode<{ children?: ComponentChildren }>).props.children),
        ...filterOverlay,
      );
  }

  /* Contorno de cada tabla + separador bajo su cabecera (una capa por tabla, sin
   * tocar los bordes de celda). Se usa la banda VISIBLE (mín/máx índice de fila
   * tras el filtro): si el cuerpo se colapsa, el marco se ciñe a lo que se ve. */
  const tableOverlays = tableRects.flatMap((ct, ti) => {
    const top = primeroDesde(visRows, ct.rect.r1);
    const bot = primeroDesde(visRows, ct.rect.r2 + 1) - 1;
    if (bot < top || ct.rect.c1 >= nCols) return [];
    const c2 = Math.min(ct.rect.c2, nCols - 1);
    const tstyle = tableStyleOf(ct.table.style);
    const left = colX[ct.rect.c1]!;
    const width = colX[c2 + 1]! - left;
    const els: JSX.Element[] = [
      <div
        key={`tbl-${ti}`}
        aria-hidden="true"
        class="pointer-events-none absolute z-10"
        style={{
          left: `${left}px`,
          top: `${topDe(top)}px`,
          width: `${width}px`,
          height: `${topDe(bot + 1) - topDe(top)}px`,
          border: `1px solid ${tstyle.border}`,
          boxSizing: 'border-box',
        }}
      />,
    ];
    if (ct.headers) {
      const hv = viOf(ct.rect.r1);
      if (hv >= 0) {
        els.push(
          <div
            key={`tbl-${ti}-h`}
            aria-hidden="true"
            class="pointer-events-none absolute z-10"
            style={{
              left: `${left}px`,
              top: `${topDe(hv + 1)}px`,
              width: `${width}px`,
              borderTop: `1.5px solid ${tstyle.border}`,
            }}
          />,
        );
      }
    }
    return els;
  });

  /* Marco del rango seleccionado: un único recuadro alrededor de todo el rango (el tinte sigue
   * siendo por celda). Una sola capa por render, con la geometría de colX/visRows como las de
   * tabla, y acotada a lo que existe: seleccionar una columna entera no pide una caja de un
   * millón de filas. Sin rango (una celda) no hay marco: la celda lleva su propio aro. */
  const rangeFrame = (() => {
    if (!multi || rect.c1 >= nCols) return null;
    const top = primeroDesde(visRows, rect.r1);
    const bot = primeroDesde(visRows, rect.r2 + 1) - 1;
    if (bot < top) return null;
    const left = colX[rect.c1]!;
    const right = colX[Math.min(rect.c2, nCols - 1) + 1]!;
    return (
      <div
        key="marco-rango"
        aria-hidden="true"
        // z-[5] y no z-10: así queda BAJO las filas y columnas congeladas (sticky z-10) y no
        // se pinta por encima de ellas cuando la selección está desplazada tras una.
        class="pointer-events-none absolute z-[5]"
        style={{
          left: `${left}px`,
          top: `${OY + top * CELL_H}px`,
          width: `${right - left}px`,
          height: `${(bot - top + 1) * CELL_H}px`,
          boxShadow: `inset 0 0 0 2px ${ACCENT}`,
        }}
      />
    );
  })();

  /* ----- estado derivado de la edición de fórmulas (fórmulas cortas: barato) ----- */
  const ac = fxEdit ? acAt(fxEdit.text, fxEdit.caret) : null;
  const acOpen = !!ac && !acDismissed;
  useMobileBack(acOpen, () => setAcDismissed(true));
  const acIdx = Math.min(acIndex, Math.max(0, (ac?.items.length ?? 0) - 1));
  const argHint = fxEdit && !acOpen ? argHintAt(fxEdit.text, fxEdit.caret) : null;
  const argSig = argHint ? lookupSignature(argHint.name) : undefined;
  const fxRefs = fxEdit ? extractRefs(fxEdit.text, resolveStructuredRect) : [];
  const editVi = viOf(sel.r);

  /** Lista sobria de autocompletado: funciones (con su descripción), nombres de
   *  tabla y columnas de tabla (referencias estructuradas Tabla[Columna]). */
  const autoList = (surface: 'cell' | 'bar'): JSX.Element => (
    <ul
      role="listbox"
      aria-label={t('sheet.suggestions')}
      class="sytue-menu-surface border-border max-h-56 w-max max-w-[380px] min-w-[240px] overflow-auto rounded-lg border py-1 shadow-[var(--shadow-md)]"
    >
      {(ac?.items ?? []).map((item, i) => {
        const sig = item.kind === 'fn' ? lookupSignature(item.label) : undefined;
        const hint =
          item.kind === 'table'
            ? t('sheet.ac.table')
            : item.kind === 'name'
              ? t('sheet.ac.name')
              : item.kind === 'column'
                ? t('sheet.ac.column')
                : sig?.summary;
        return (
          <li key={`${item.kind}:${item.label}`}>
            <button
              type="button"
              role="option"
              aria-selected={i === acIdx}
              class={`flex w-full items-baseline gap-2 px-2 py-0.5 text-left text-sm ${
                i === acIdx ? 'bg-selected text-text' : 'text-text hover:bg-bg-subtle'
              } ${FOCUS_RING}`}
              onMouseDown={(e) => {
                e.preventDefault(); // no robar el foco del editor
                setAcIndex(i);
                acceptAc(surface, item);
              }}
            >
              <span class="font-medium tabular-nums">{item.label}</span>
              {hint ? (
                <span class="text-text-muted min-w-0 flex-1 truncate text-xs">{hint}</span>
              ) : null}
            </button>
          </li>
        );
      })}
    </ul>
  );

  /** Tooltip de firma con el argumento actual resaltado (del catálogo). */
  const hintContent =
    argHint && argSig
      ? (() => {
          const sp = splitSignature(argSig.args);
          const active = Math.min(argHint.argIndex, Math.max(0, sp.params.length - 1));
          return (
            <div class="sytue-popover-surface border-border w-max max-w-[400px] rounded-lg border px-2 py-1 text-xs shadow-[var(--shadow-md)]">
              <div class="text-text">
                <span class="font-medium">{sp.name}</span>
                <span>(</span>
                {sp.params.map((p, i) => (
                  <span key={i}>
                    {i > 0 ? '; ' : ''}
                    <span class={i === active ? 'text-text font-semibold' : 'text-text-muted'}>
                      {p}
                    </span>
                  </span>
                ))}
                <span>)</span>
              </div>
              <div class="text-text-muted mt-0.5">{argSig.summary}</div>
            </div>
          );
        })()
      : null;

  /** Marcos de color sobre las celdas referenciadas por la fórmula en edición
   *  (mismo color que el tinte del texto de esa referencia). */
  const rangeOverlays = fxEdit
    ? fxRefs
        .filter((r) => r.sheet === undefined || r.sheet === sheet.name)
        .map((r, i) => {
          if (r.rect.c1 >= nCols) return null;
          const vi1 = viOf(Math.min(r.rect.r1, nRows - 1));
          const vi2 = viOf(Math.min(r.rect.r2, nRows - 1));
          if (vi1 < 0 || vi2 < 0) return null;
          const c2 = Math.min(r.rect.c2, nCols - 1);
          const color = refColor(r.colorIndex);
          return (
            <div
              key={`fxref-${r.start}-${i}`}
              aria-hidden="true"
              class="pointer-events-none absolute z-10"
              style={{
                left: `${colX[r.rect.c1]}px`,
                top: `${topDe(vi1)}px`,
                width: `${colX[c2 + 1]! - colX[r.rect.c1]!}px`,
                height: `${topDe(vi2 + 1) - topDe(vi1)}px`,
                border: `1.5px solid ${color}`,
                background: `color-mix(in srgb, ${color} 8%, transparent)`,
                boxSizing: 'border-box',
              }}
            />
          );
        })
    : null;

  /* Cursores remotos de los peers sobre la hoja activa: marco de color en su
   * celda/rango + etiqueta con el nombre (se desvanece al parar). Efímero; se
   * salta si el peer está en otra hoja o su fila está oculta (sheetPeerBox → null). */
  const peerCursorOverlay = ctx.peerCursors.map((pc) => {
    if (pc.pos.kind !== 'sheet') return null;
    const sitio = resolverPosHoja(model.sheets, pc.pos);
    if (sitio === null || sitio.sheet !== model.active) return null;
    const box = sheetPeerBox(sitio.pos, { colX, visRows, oy: OY, cellH: CELL_H, rowTop });
    if (box === null) return null;
    return (
      <div
        key={pc.id}
        aria-hidden="true"
        class="pointer-events-none absolute z-10"
        style={{
          left: `${box.left}px`,
          top: `${box.top}px`,
          width: `${box.width}px`,
          height: `${box.height}px`,
          boxShadow: `inset 0 0 0 2px ${pc.color}`,
          boxSizing: 'border-box',
        }}
      >
        <span
          key={cursorPosKey(pc.pos)}
          class="sytue-peer-label text-2xs absolute -top-[15px] left-0 max-w-[160px] truncate rounded-sm px-1 py-px leading-tight font-medium text-white"
          style={{ background: pc.color }}
        >
          {peerName(pc.name)}
        </span>
      </div>
    );
  });

  /** Autocompletado o hint bajo la celda en edición (superficie «cell»). */
  const cellFxPopover =
    fxEdit?.surface === 'cell' && editVi >= 0 && (acOpen || hintContent) ? (
      <div
        class="absolute z-[var(--z-popover)]"
        style={{ left: `${colX[sel.c]}px`, top: `${topDe(editVi + 1)}px` }}
      >
        {acOpen ? autoList('cell') : hintContent}
      </div>
    ) : null;

  /* Canalón de esquema de COLUMNAS: fila pegajosa sobre las cabeceras con un
   * carril por nivel (vertical) y su barra de niveles en la esquina. Inerte
   * (null) sin grupos de columna. */
  const colGutterRow =
    CGUT === 0 ? null : (
      <div
        role="presentation"
        class="sticky top-0 z-30 flex"
        style={{ height: `${CGUT}px`, width: `${gridW}px` }}
      >
        <div
          class="border-border bg-bg-subtle sticky left-0 z-40 flex items-center justify-end gap-0.5 border-r border-b px-0.5"
          style={{ left: 0, width: `${OX}px`, height: `${CGUT}px` }}
        >
          {levelBar('col', colLevels)}
        </div>
        {pintaCols.map((ci) => {
          const c = visCols[ci]!;
          const fzCol = c < freeze.cols;
          return (
            <div
              key={c}
              role="presentation"
              class={`border-border bg-bg-subtle flex shrink-0 flex-col border-r border-b ${fzCol ? 'sticky z-10' : 'absolute top-0'}`}
              style={{ width: `${colW(c)}px`, height: `${CGUT}px`, left: `${colX[c]}px` }}
            >
              {Array.from({ length: colLevels }, (_, l) => {
                const tg = laneToggle(colGroups, c, l + 1, nCols);
                return (
                  <div
                    key={l}
                    class="relative flex items-center justify-center"
                    style={{ height: `${OUTLINE_STEP}px`, width: '100%' }}
                  >
                    {tg ? (
                      toggleBtn('col', tg)
                    ) : laneLine(colGroups, c, l + 1) ? (
                      <span
                        aria-hidden="true"
                        class="bg-border-strong"
                        style={{ height: '1px', width: '100%' }}
                      />
                    ) : null}
                  </div>
                );
              })}
            </div>
          );
        })}
      </div>
    );

  const fxIsFormula = fx.startsWith('=');

  return (
    <div class="flex h-full min-h-0 flex-1 flex-col">
      {/* Barra de fórmulas (§7.1): referencia + fx; Enter escribe y baja una fila. */}
      <div class="border-border bg-surface flex h-8 shrink-0 items-center gap-2 border-b px-2 pointer-coarse:h-auto pointer-coarse:min-h-[max(44px,2.75rem)]">
        <div
          class="border-border bg-bg-subtle text-text flex h-6 w-16 shrink-0 items-center justify-center rounded-sm border text-xs font-medium tabular-nums"
          title={t('sheet.activeCell')}
        >
          {refLabel}
        </div>
        <span class="sytue-meta italic" aria-hidden="true">
          fx
        </span>
        {/* Editor fx: input transparente sobre un fondo tintado por referencia
         *  (los <span> no caben dentro de un <input>). El desplegable de
         *  autocompletado y el hint cuelgan bajo la barra.
         *  El texto va con la tinta de TEMA (--sytue-text), no con la de contenido
         *  (INK): esta barra vive en el cromo, así que INK la dejaba invisible en
         *  tema oscuro. INK se reserva para lo que se pinta sobre el papel. */}
        <div class="relative flex min-w-0 flex-1 items-center">
          <div
            ref={fxBackdropRef}
            aria-hidden="true"
            class="pointer-events-none absolute inset-0 overflow-hidden px-1 text-sm leading-5 whitespace-pre"
            style={{ color: 'var(--sytue-text)' }}
          >
            {fxIsFormula ? (
              <span
                dangerouslySetInnerHTML={{
                  __html: tintedFormulaHtml(fx, extractRefs(fx, resolveStructuredRect), refFxColor),
                }}
              />
            ) : null}
          </div>
          <input
            ref={fxInputRef}
            class={`text-text relative z-[1] h-5 w-full min-w-0 rounded-sm bg-transparent px-1 text-sm ${FOCUS_RING}`}
            style={
              fxIsFormula ? { color: 'transparent', caretColor: 'var(--sytue-text)' } : undefined
            }
            aria-label={t('sheet.formulaBar')}
            spellcheck={false}
            readOnly={!canEditSel}
            value={fx}
            onInput={onFxInput}
            onKeyDown={onFxKeyDown}
            onKeyUp={onFxCaret}
            onClick={onFxCaret}
            onFocus={onFxCaret}
            onBlur={onFxBlur}
            onScroll={syncBackdropScroll}
          />
          {fxEdit?.surface === 'bar' && (acOpen || hintContent) ? (
            <div class="absolute top-full left-0 z-[var(--z-popover)] mt-1">
              {acOpen ? autoList('bar') : hintContent}
            </div>
          ) : null}
        </div>
      </div>

      {/* Rejilla con scroll; zoom por CSS zoom para no romper las cabeceras pegajosas. */}
      <div
        ref={scrollRef}
        onScroll={medirVista}
        // `data-sin-tactil`: los botones de dentro de la rejilla miden lo que la celda (editor.css).
        data-sin-tactil
        class="min-h-0 flex-1 overflow-auto"
        style={{ background: 'var(--editor-canvas-bg)' }}
      >
        <div class="relative w-max" style={{ zoom: String(ctx.ui.zoom / 100), color: INK }}>
          <div
            ref={gridRef}
            role="grid"
            aria-label={t('sheet.tab.aria', { name: sheet.name })}
            aria-rowcount={nRows + 1}
            aria-colcount={nCols + 1}
            aria-multiselectable={true}
            /* El foco del navegador se queda en la rejilla y quien se mueve es la celda
               activa; sin esto, moverse con las flechas no anunciaba nada. Se apunta solo
               si la celda está pintada: una fila filtrada o un grupo plegado no existen en
               el DOM, y apuntar a un `id` que no está es peor que no apuntar. */
            aria-activedescendant={
              hidden.has(sel.r) || hiddenCols.has(sel.c)
                ? undefined
                : cellDomId(`${colName(sel.c)}${sel.r + 1}`)
            }
            tabIndex={0}
            class="sytue-focus-ring-inner relative w-max outline-none"
            style={{ width: `${gridW}px`, height: `${gridH}px` }}
            onMouseDown={onGridMouseDown}
            onMouseMove={onGridMouseMove}
            onMouseUp={onGridMouseUp}
            onDblClick={onGridDblClick}
            onKeyDown={onGridKeyDown}
            onKeyUp={onGridKeyUp}
            onInput={onGridInput}
            onCompositionStart={onGridCompositionStart}
            onCompositionEnd={onGridCompositionEnd}
            onFocusIn={onGridFocusIn}
            onFocusOut={onGridFocusOut}
          >
            {colGutterRow}
            <div
              role="row"
              aria-rowindex={1}
              class="sticky z-20 flex"
              style={{ top: `${CGUT}px`, width: `${gridW}px`, height: `${CELL_H}px` }}
            >
              {headerCells}
            </div>
            {rows}
            {tableOverlays}
            {rangeFrame}
            {rangeOverlays}
            {peerCursorOverlay}
            {fillHandle}
            {validDropdown}
            {cellFxPopover}
            {filtroEnFila ? null : filterOverlay}
          </div>
          {/* Gráficos flotantes: HERMANOS del role=grid (sus hijos deben ser rows)
           * y en su mismo origen de coordenadas; se desplazan con el scroll.
           * ponytail: z-10 como el asa de relleno — quedan bajo las cabeceras
           * pegajosas (z-20) pero pueden pisar celdas congeladas; se asume. */}
          {(sheet.charts ?? []).map((ch) => (
            <SheetChart
              key={ch.id}
              spec={ch}
              engine={engine}
              sheetName={sheet.name}
              zoom={ctx.ui.zoom / 100}
              onMove={(cx, cy) => moveChart(ch.id, cx, cy)}
              onRemove={() => removeChart(ch.id)}
              onEdit={(patch) => editChart(ch.id, patch)}
            />
          ))}
          {/* Segmentaciones (slicers): tarjetas flotantes que filtran una columna
           * del autofiltro. Los valores y el estado de exclusión salen de la MISMA
           * fuente que el desplegable de filtro (filterEntries → excl del autofiltro),
           * así que ambos controles quedan siempre sincronizados. */}
          {(sheet.slicers ?? []).map((sl) => {
            if (sl.pivot) {
              const vista = pivotSlicerView(sl);
              return (
                <SheetSlicer
                  key={sl.id}
                  slicer={sl}
                  field={vista.field}
                  entries={vista.entries}
                  active={vista.active}
                  zoom={ctx.ui.zoom / 100}
                  onMove={(cx, cy) => moveSlicer(sl.id, cx, cy)}
                  onApply={(selected, unique) => applyPivotSlicer(sl, selected, unique)}
                  onRemove={() => removeSlicer(sl.id)}
                />
              );
            }
            const afOk = !!af && sl.col >= af.c1 && sl.col <= af.c2;
            const field = af
              ? formatCellValue(engine.getValue(sheet.name, A1(sl.col, af.row))) || colName(sl.col)
              : colName(sl.col);
            return (
              <SheetSlicer
                key={sl.id}
                slicer={sl}
                field={field}
                entries={afOk ? filterEntries(sl.col) : []}
                active={afOk}
                zoom={ctx.ui.zoom / 100}
                onMove={(cx, cy) => moveSlicer(sl.id, cx, cy)}
                onApply={(selected, unique) => applySlicerSelection(sl.col, selected, unique)}
                onRemove={() => removeSlicer(sl.id)}
              />
            );
          })}
        </div>
      </div>

      {/* Pestañas de hoja (§7.1): activa con subrayado del acento del producto;
       * doble clic renombra y arrastrar reordena (drag and drop nativo). */}
      <div class="border-border bg-surface flex h-8 shrink-0 items-center gap-1 border-t px-2 text-xs pointer-coarse:h-auto pointer-coarse:min-h-[max(44px,2.75rem)]">
        {/* La tira de pestañas se DESPLAZA: acotar el ancho de cada nombre retrasaba
            el problema, pero con seis hojas —o con una pantalla de móvil— las
            últimas pestañas seguían saliéndose de la barra sin forma de llegar a
            ellas. Los botones se quedan fuera de la tira: siempre alcanzables.
            `data-scroll-x`: desplazamiento declarado, como la banda de la cinta; sin él el
            barrido lo contaba como desbordamiento en cuanto hay dos hojas en un móvil. */}
        <div data-scroll-x class="flex h-full min-w-0 flex-1 items-center gap-1 overflow-x-auto">
          {model.sheets.map((s, i) => (
            <button
              key={i}
              type="button"
              // Se acota el ancho para que un nombre largo no se coma la tira entera;
              // el nombre completo sigue en el `title`.
              class={`relative h-full max-w-[12rem] shrink-0 truncate px-2 ${FOCUS_RING} ${
                i === model.active ? 'text-text font-medium' : 'text-text-muted hover:text-text'
              }`}
              aria-pressed={i === model.active}
              title={
                protectedWorkbook
                  ? t('sheet.tab.locked', { name: s.name })
                  : t('sheet.tab.hint', { name: s.name })
              }
              draggable={!protectedWorkbook}
              onDragStart={(e) => {
                sheetDrag.current = i;
                // setData: sin él, Firefox no inicia el arrastre.
                e.dataTransfer?.setData('text/plain', s.name);
                if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move';
              }}
              onDragOver={(e) => {
                if (sheetDrag.current !== null) e.preventDefault(); // acepta soltar aquí
              }}
              onDrop={(e) => {
                e.preventDefault();
                const from = sheetDrag.current;
                sheetDrag.current = null;
                if (from !== null && from !== i) moveSheetTo(from, i);
              }}
              onDragEnd={() => {
                sheetDrag.current = null;
              }}
              onClick={() => switchSheet(i)}
              onDblClick={() => void renameSheet(i)}
            >
              {s.name}
              {s.tabColor && (
                // El color de la pestaña (el de Excel): una banda debajo del nombre.
                <span
                  aria-hidden="true"
                  class="absolute inset-x-1 bottom-0 h-1 rounded-full"
                  style={{ background: s.tabColor }}
                />
              )}
              {i === model.active && (
                <span
                  aria-hidden="true"
                  class="bg-indicator absolute inset-x-1 bottom-0 h-0.5 rounded-full"
                />
              )}
            </button>
          ))}
        </div>
        <RibbonButton
          icon="plus"
          label={t('sheet.add')}
          disabled={protectedWorkbook}
          onAction={addSheet}
        />
        <RibbonButton
          icon="copy"
          label={t('sheet.duplicate')}
          disabled={protectedWorkbook}
          onAction={duplicateSheet}
        />
        <RibbonButton
          icon="trash"
          label={t('sheet.delete')}
          disabled={protectedWorkbook || model.sheets.length <= 1}
          onAction={() => void deleteSheet()}
        />
        {selStats && (
          // <output> ya es role=status: el resumen se anuncia solo al cambiar la
          // selección sin añadirle ARIA a mano.
          <output class="text-text-muted shrink-0 truncate tabular-nums">
            {t('sheet.stats.count', { count: selStats.count })} ·{' '}
            {t('sheet.stats.sum', { value: formatCellValue(selStats.sum) })} ·{' '}
            {t('sheet.stats.average', { value: formatCellValue(selStats.avg) })}
          </output>
        )}
      </div>
    </div>
  );
}
