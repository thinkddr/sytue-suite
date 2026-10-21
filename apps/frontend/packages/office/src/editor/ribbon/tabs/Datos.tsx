/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Pestaña «Datos» del ribbon (solo kind spreadsheet): ordenar por la columna
 * del ancla, AUTOFILTRO persistente (los desplegables por encabezado viven en
 * la rejilla; aquí solo el interruptor), validación de datos, herramientas
 * (quitar duplicados, texto en columnas), insertar/borrar filas y columnas,
 * insertar gráfico y congelar paneles. Todo viaja por dispatchSheetCommand y se
 * refleja vía readSheetState/subscribeSheetState.
 */
import type { JSX } from 'preact';
import { useT } from '../../context';
import { useEffect, useRef, useState } from 'preact/hooks';
import { useToast, Checkbox } from '@sytue/ui';
import type { ChartSpec, DataValidationOp, DataValidationType, EditorContext } from '../../types';
import {
  RibbonButton,
  RibbonGroup,
  RibbonSelect,
  RibbonSeparator,
  SERIE,
  usePopover,
  useBloqueado,
  POPOVER_SHELL,
} from '../primitives';
import { EditorIcon } from '../../icons';
import {
  dispatchSheetCommand,
  makeGoalSeekProbe,
  makeSolverProbe,
  readSheetGrid,
  readSheetState,
  readSheetValues,
  subscribeSheetState,
} from '../../sheets/SheetCanvas';
import type {
  ExternalTableState,
  GoalSeekProbeResult,
  SolverProbeResult,
} from '../../sheets/SheetCanvas';
import { ExternalDataModal } from '../../panels/ExternalDataModal';
import { NameManagerMenu } from '../../sheets/NameManager';
import { PivotModal, newPivotDraft } from '../../panels/PivotModal';
import type { PivotDraft } from '../../panels/PivotModal';
import { QueryStepsModal } from '../../panels/QueryStepsModal';
import {
  buildExecuteBody,
  kindOfSpec,
  parseExecuteResult,
  queryErrorAdvice,
} from '../../dataquery';
import { aiErrorCode, aiErrorKey, parseAiResponse } from '../../ai';
import { buildFillPrompt, parseFillResponse } from '../../sheets/relleno';
import {
  DATA_VALIDATION_OPS,
  DATA_VALIDATION_TYPES,
  formatCellValue,
  parseEsDate,
  parseEsNumber,
  parseEsNumberStrict,
  parseRangeRef,
  parseValidationValues,
  validationLabel,
} from '../../sheets/format';
import type { SheetPermission } from '../../sheets/format';
import { goalSeek } from '../../sheets/goalseek';
import type { GoalSeekResult } from '../../sheets/goalseek';
import { solve } from '../../sheets/solver';
import type {
  SolverConstraint,
  SolverGoal,
  SolverRelation,
  SolverResult,
} from '../../sheets/solver';
import { SCENARIO_MAX_CELLS } from '../../sheets/scenario';
import { BloquesGroup } from '../BloquesGroup';
import { TABLE_STYLES, TABLE_TOTAL_FNS } from '../../sheets/table';
import type { SheetTable, Sparkline, TableTotalFn } from '../../types';
import type { DocsKey } from '../../../i18n';

// El anillo canónico vive en `@sytue/ui/styles.css` (.sytue-focus-ring): una sola
// definición para toda la suite, en vez de la copia que cada fichero perdía a su manera.
const FOCUS_RING = 'sytue-focus-ring';

/**
 * Campo compacto de los diálogos de esta pestaña. Estaba declarado SIETE veces —una por
 * función— y ya había divergido: seis copias sin `placeholder:text-text-subtle`, o sea
 * con el texto de sugerencia al mismo tono que el valor escrito.
 *
 * `@sytue/ui` ya trae esta talla (`<Input size="sm">`, entregado con este mismo cambio);
 * adoptarla en los 20 usos de este fichero es parte de la canonicalización del editor,
 * que se hace por bloques y no en una barrida sobre 2858 líneas de otra ola.
 */
const inputCls =
  'h-6 w-full rounded-sm border border-border bg-surface px-1.5 text-xs text-text placeholder:text-text-subtle ' +
  FOCUS_RING;

export function DatosTab(): JSX.Element {
  const t = useT();
  // Repintado cuando cambian selección/modelo/filtro de la hoja (canal de lectura).
  const [, setTick] = useState(0);
  useEffect(() => subscribeSheetState(() => setTick((n) => n + 1)), []);
  const st = readSheetState();
  const frozen = (st?.freeze.cols ?? 0) > 0 || (st?.freeze.rows ?? 0) > 0;

  /** Congela las filas/columnas ANTERIORES a la celda ancla (como Excel);
   *  en A1 no hay nada antes → fila superior, el caso más pedido. */
  const toggleFreeze = (): void => {
    if (frozen) {
      dispatchSheetCommand('freeze', '0,0');
      return;
    }
    const a = parseRangeRef(readSheetState()?.selection ?? '');
    const c = a?.c1 ?? 0;
    const r = a?.r1 ?? 0;
    dispatchSheetCommand('freeze', c === 0 && r === 0 ? '0,1' : `${c},${r}`);
  };

  /* ----- Datos externos: gestor (modal) + refrescar (re-ejecuta y reescribe) ----- */
  const toast = useToast();
  const [extOpen, setExtOpen] = useState(false);
  const [stepsOpen, setStepsOpen] = useState(false);
  const [stepsInitial, setStepsInitial] = useState('');
  const [refreshing, setRefreshing] = useState(false);
  const externalTables = st?.externalTables ?? [];

  /** Re-ejecuta cada tabla externa (por su conexión + consulta guardadas) y despacha
   *  `extdata` para reescribir la SheetTable. Secuencial (evita el rate-limit por
   *  tenant del backend). Las credenciales viven en el backend: aquí solo el id. */
  const refreshExternal = async (list: readonly ExternalTableState[]): Promise<void> => {
    if (list.length === 0 || refreshing) return;
    setRefreshing(true);
    let ok = 0;
    let firstError: string | null = null;
    for (const ext of list) {
      try {
        const res = await fetch('/api/dataquery', {
          method: 'POST',
          headers: { 'content-type': 'application/json' },
          body: JSON.stringify(buildExecuteBody(ext.connectionId, ext.spec)),
        });
        const data: unknown = await res.json().catch(() => null);
        if (!res.ok) {
          // Se dice qué hacer, no el error crudo del proveedor (que además el
          // backend colapsa a «upstream_error» sin detalle a propósito).
          const advice = queryErrorAdvice(data, res.status, kindOfSpec(ext.spec));
          throw new Error(`${advice.title} ${advice.action}`);
        }
        const result = parseExecuteResult(data);
        if (!result) throw new Error(t('data.refresh.notATable'));
        dispatchSheetCommand(
          'extdata',
          JSON.stringify({
            connectionId: ext.connectionId,
            spec: ext.spec,
            anchor: ext.anchor,
            columns: result.columns,
            rows: result.rows,
            provenance: result.provenance, // Sytue Bases: de dónde vienen (se guarda con la tabla)
            transform: ext.transform, // reaplica los pasos guardados sobre los datos frescos
            name: ext.table, // refrescar la existente (reescribe + reajusta el rango)
          }),
        );
        ok++;
      } catch (e) {
        if (!firstError) firstError = e instanceof Error ? e.message : t('data.refresh.error');
      }
    }
    setRefreshing(false);
    if (firstError && ok === 0) {
      toast.notify({ tone: 'danger', title: t('data.refresh.failed'), description: firstError });
    } else if (firstError) {
      toast.notify({
        tone: 'warning',
        title: t('data.refresh.partial', { ok: String(ok), total: String(list.length) }),
        description: firstError,
      });
    } else {
      toast.notify({
        tone: 'success',
        title: t('data.refresh.done', { count: ok }),
      });
    }
  };

  const pivots = st?.pivots ?? [];
  /** Actualiza la tabla externa que cubre la celda ancla actual (si la hay); y si el
   *  ancla está en una tabla dinámica, esa. */
  const refreshCurrent = (): void => {
    const at = readSheetState()?.pivotAt;
    if (at) {
      dispatchSheetCommand('pivotrefresh', at.name);
      return;
    }
    const sel = parseRangeRef(readSheetState()?.selection ?? '');
    const hit = sel
      ? externalTables.find((e) => {
          const rr = parseRangeRef(e.range);
          return rr && sel.c1 >= rr.c1 && sel.c1 <= rr.c2 && sel.r1 >= rr.r1 && sel.r1 <= rr.r2;
        })
      : undefined;
    if (!hit) {
      toast.notify({
        tone: 'info',
        title: t('data.refresh.noTable'),
      });
      return;
    }
    void refreshExternal([hit]);
  };

  /** Abre el gestor «Pasos aplicados» sobre la tabla externa del ancla (o la 1.ª). */
  const openSteps = (): void => {
    const sel = parseRangeRef(readSheetState()?.selection ?? '');
    const hit = sel
      ? externalTables.find((e) => {
          const rr = parseRangeRef(e.range);
          return rr && sel.c1 >= rr.c1 && sel.c1 <= rr.c2 && sel.r1 >= rr.r1 && sel.r1 <= rr.r2;
        })
      : undefined;
    setStepsInitial(hit?.table ?? '');
    setStepsOpen(true);
  };

  return (
    <>
      <RibbonGroup label={t('group.externalData')}>
        <RibbonButton commandId="extdata" icon="database" onAction={() => setExtOpen(true)} />
        <RibbonButton
          commandId="extTransform"
          icon="filter"
          disabled={externalTables.length === 0}
          onAction={openSteps}
        />
        <RibbonButton
          commandId="extRefresh"
          icon="refresh"
          disabled={refreshing || externalTables.length + pivots.length === 0}
          onAction={refreshCurrent}
        />
        <RibbonButton
          commandId="extRefreshAll"
          icon="refreshAll"
          disabled={refreshing || externalTables.length + pivots.length === 0}
          onAction={() => {
            // Primero los datos externos (que pueden ser el origen de una tabla
            // dinámica) y después las tablas dinámicas, como Actualizar todo en Excel.
            void refreshExternal(externalTables).then(() => {
              if (pivots.length > 0) dispatchSheetCommand('pivotrefresh', '');
            });
          }}
        />
      </RibbonGroup>

      <RibbonSeparator />

      <RibbonGroup label={t('group.sortFilter')}>
        <RibbonButton
          commandId="sort"
          icon="sortAsc"
          onAction={() => dispatchSheetCommand('sort', 'asc')}
        />
        <RibbonButton
          commandId="sort"
          icon="sortDesc"
          label={t('data.sortDesc')}
          onAction={() => dispatchSheetCommand('sort', 'desc')}
        />
        <RibbonButton
          commandId="autofilter"
          icon="filter"
          active={st?.autofilter ?? false}
          onAction={() => dispatchSheetCommand('autofilter')}
        />
        <SlicerMenu />
      </RibbonGroup>

      <RibbonSeparator />

      <RibbonGroup label={t('settings.table')}>
        <TableMenu />
      </RibbonGroup>

      <RibbonSeparator />

      <RibbonGroup label={t('names.group')}>
        <NameManagerMenu />
      </RibbonGroup>

      <RibbonSeparator />

      <RibbonGroup label={t('group.tools')}>
        <RibbonButton
          commandId="dedupe"
          icon="trash"
          onAction={() => dispatchSheetCommand('dedupe')}
        />
        <TextToColumnsMenu />
        <ValidationMenu />
      </RibbonGroup>

      <RibbonSeparator />

      <RibbonGroup label={t('group.cells')}>
        <RibbonButton
          commandId="insrow"
          icon="rowAbove"
          onAction={() => dispatchSheetCommand('insrow', 'above')}
        />
        <RibbonButton
          commandId="insrow"
          icon="rowBelow"
          label={t('table.rowBelow')}
          onAction={() => dispatchSheetCommand('insrow', 'below')}
        />
        <RibbonButton
          commandId="inscol"
          icon="colLeft"
          onAction={() => dispatchSheetCommand('inscol', 'left')}
        />
        <RibbonButton
          commandId="inscol"
          icon="colRight"
          label={t('table.colRight')}
          onAction={() => dispatchSheetCommand('inscol', 'right')}
        />
        <RibbonButton
          commandId="delrow"
          icon="minus"
          onAction={() => dispatchSheetCommand('delrow')}
        />
        <RibbonButton
          commandId="delcol"
          icon="close"
          onAction={() => dispatchSheetCommand('delcol')}
        />
      </RibbonGroup>

      <RibbonSeparator />

      <RibbonGroup label={t('slides.box.chart')}>
        <ChartMenu />
      </RibbonGroup>

      <RibbonSeparator />

      <BloquesGroup kind="spreadsheet" />
      <RibbonSeparator />

      <RibbonGroup label={t('extdata.step.review')}>
        <SparklineMenu />
        <PivotMenu />
      </RibbonGroup>

      <RibbonSeparator />

      <RibbonGroup label={t('rail.outline.title')}>
        <RibbonButton
          commandId="outline"
          icon="groupRows"
          label={t('data.groupRows')}
          onAction={() =>
            dispatchSheetCommand('outline', JSON.stringify({ axis: 'row', op: 'group' }))
          }
        />
        <RibbonButton
          commandId="outline"
          icon="ungroupRows"
          label={t('data.ungroupRows')}
          onAction={() =>
            dispatchSheetCommand('outline', JSON.stringify({ axis: 'row', op: 'ungroup' }))
          }
        />
        <RibbonButton
          commandId="outline"
          icon="groupCols"
          label={t('data.groupCols')}
          onAction={() =>
            dispatchSheetCommand('outline', JSON.stringify({ axis: 'col', op: 'group' }))
          }
        />
        <RibbonButton
          commandId="outline"
          icon="ungroupCols"
          label={t('data.ungroupCols')}
          onAction={() =>
            dispatchSheetCommand('outline', JSON.stringify({ axis: 'col', op: 'ungroup' }))
          }
        />
        <SubtotalMenu />
      </RibbonGroup>

      <RibbonSeparator />

      <RibbonGroup label={t('group.window')}>
        <RibbonButton commandId="freeze" icon="freeze" active={frozen} onAction={toggleFreeze} />
      </RibbonGroup>

      <RibbonSeparator />

      <RibbonGroup label={t('group.protect')}>
        <LockCellButton />
        <ProtectSheetMenu />
        <ProtectWorkbookMenu />
      </RibbonGroup>

      <ExternalDataModal open={extOpen} onClose={() => setExtOpen(false)} />
      <QueryStepsModal
        open={stepsOpen}
        onClose={() => setStepsOpen(false)}
        tables={externalTables}
        initialTable={stepsInitial}
      />
    </>
  );
}

/**
 * Pestaña «Fórmulas» de la hoja: lo que escribe o calcula con fórmulas —el asistente que
 * propone la fórmula de una celda y el análisis de hipótesis (Buscar objetivo, Solver,
 * tabla de datos, escenarios)—. Salió de «Datos» al fijarse la taxonomía de pestañas
 * (`pestanas.json`); sus comandos son los de siempre, en `comandos.json` con
 * `tab: "formulas"`.
 */
export function FormulasTab({ ctx }: { ctx: EditorContext }): JSX.Element {
  const t = ctx.t;
  return (
    <>
      {/* Solo con servicio detrás, como el asistente de Inicio: sin SYTUE_AI_URL
          sería un botón que siempre falla. */}
      {ctx.aiEnabled ? (
        <>
          <RibbonGroup label={t('group.assistant')}>
            <AiFillMenu />
          </RibbonGroup>
          <RibbonSeparator />
        </>
      ) : null}

      <RibbonGroup label={t('group.whatIf')}>
        <GoalSeekMenu />
        <SolverMenu />
        <DataTableMenu />
        <ScenarioMenu />
      </RibbonGroup>
    </>
  );
}

/* ---------- Cierre de popover compartido (Escape + clic fuera) ---------- */

/* ---------- Texto en columnas (popover: elegir delimitador) ---------- */

const DELIMS: { value: string; labelKey: DocsKey }[] = [
  { value: ',', labelKey: 'data.delim.comma' },
  { value: ';', labelKey: 'data.delim.semicolon' },
  { value: '\t', labelKey: 'data.delim.tab' },
  { value: ' ', labelKey: 'data.delim.space' },
];

function TextToColumnsMenu(): JSX.Element {
  const bloqueado = useBloqueado('text2cols');
  const t = useT();
  const { open, setOpen, rootRef } = usePopover();
  return (
    <div ref={rootRef} class="relative">
      <button
        type="button"
        class={
          'disabled:pointer-events-none disabled:opacity-40 ' +
          'text-text-muted inline-flex h-7 w-7 items-center justify-center rounded-sm ' +
          'hover:bg-bg-subtle hover:text-text aria-expanded:bg-selected aria-expanded:text-text ' +
          FOCUS_RING
        }
        aria-label={t('data.textToColumns')}
        title={t('data.textToColumns.hint')}
        aria-haspopup="true"
        aria-expanded={open}
        disabled={bloqueado}
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => setOpen(!open)}
      >
        <EditorIcon name="colRight" />
      </button>
      {open && (
        <div
          role="group"
          aria-label={t('data.textToColumns')}
          class={`${POPOVER_SHELL} left-0 w-48 p-2`}
        >
          <div class="sytue-meta-dense pb-1.5">{t('data.separateBy')}</div>
          <div class="grid grid-cols-2 gap-1">
            {DELIMS.map((d) => (
              <button
                key={d.value}
                type="button"
                class={
                  'border-border bg-surface text-text hover:bg-bg-subtle h-6 rounded-sm border px-2 text-xs ' +
                  FOCUS_RING
                }
                onClick={() => {
                  dispatchSheetCommand('text2cols', d.value);
                  setOpen(false);
                }}
              >
                {t(d.labelKey)}
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

/* ---------- Insertar gráfico (popover: rango A1 + tipo; fase Visual) ---------- */

const CHART_TYPE_OPTIONS: { value: ChartSpec['type']; labelKey: DocsKey }[] = [
  { value: 'bar', labelKey: 'chart.kind.bar' },
  { value: 'hbar', labelKey: 'chart.kind.hbar' },
  { value: 'line', labelKey: 'chart.kind.line' },
  { value: 'area', labelKey: 'chart.kind.area' },
  { value: 'combo', labelKey: 'chart.kind.combo' },
  { value: 'pie', labelKey: 'chart.kind.pie' },
  { value: 'donut', labelKey: 'chart.kind.doughnut' },
  { value: 'scatter', labelKey: 'chart.kind.scatter' },
  { value: 'radar', labelKey: 'chart.kind.radar' },
];

function ChartMenu(): JSX.Element {
  const bloqueado = useBloqueado('chartadd');
  const t = useT();
  const { open, setOpen, rootRef } = usePopover();
  const [range, setRange] = useState('A1:B4');
  const [type, setType] = useState<ChartSpec['type']>('bar');

  const valid = parseRangeRef(range) !== null;
  const insert = (): void => {
    if (!valid) return;
    dispatchSheetCommand('chartadd', JSON.stringify({ type, range: range.trim() }));
    setOpen(false);
  };

  return (
    <div ref={rootRef} class="relative">
      <button
        type="button"
        class={
          'disabled:pointer-events-none disabled:opacity-40 ' +
          'text-text-muted inline-flex h-7 w-7 items-center justify-center rounded-sm ' +
          'hover:bg-bg-subtle hover:text-text ' +
          FOCUS_RING
        }
        aria-label={t('data.chart.insert')}
        title={t('data.chart.insert')}
        aria-haspopup="true"
        aria-expanded={open}
        disabled={bloqueado}
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => {
          if (!open) setRange(readSheetState()?.selection ?? 'A1:B4'); // prellena con la selección
          setOpen(!open);
        }}
      >
        <EditorIcon name="chart" />
      </button>
      {open && (
        <div
          role="group"
          aria-label={t('data.chart.insert')}
          class={`${POPOVER_SHELL} left-0 w-56 p-2`}
        >
          <div class="text-text pb-1 text-xs font-medium">{t('data.chart.insert')}</div>
          <label class="block pb-1.5">
            <span class="sytue-meta-dense mb-0.5 block">{t('data.chart.range')}</span>
            <input
              class={
                'border-border bg-surface text-text h-6 w-full rounded-sm border px-1.5 text-xs ' +
                FOCUS_RING
              }
              value={range}
              onInput={(e) => setRange(e.currentTarget.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  insert();
                }
              }}
            />
          </label>
          <div class="pb-2">
            <RibbonSelect
              label={t('chart.type')}
              value={type}
              width={208}
              options={CHART_TYPE_OPTIONS.map((o) => ({ value: o.value, label: t(o.labelKey) }))}
              onSelect={(v) => setType(v as ChartSpec['type'])}
            />
          </div>
          <button
            type="button"
            class={
              'border-border bg-surface text-text hover:bg-bg-subtle h-6 w-full rounded-sm border px-2 text-xs ' +
              'disabled:pointer-events-none disabled:opacity-40 ' +
              FOCUS_RING
            }
            disabled={!valid}
            onClick={insert}
          >
            {t('action.insert')}
          </button>
        </div>
      )}
    </div>
  );
}

/* ---------- Segmentación de datos (slicer): elegir el campo de una tabla/rango ---------- */

function SlicerMenu(): JSX.Element {
  const bloqueado = useBloqueado('sliceradd');
  const t = useT();
  const { open, setOpen, rootRef } = usePopover();
  const [range, setRange] = useState('A1:D10');
  const [field, setField] = useState('0');
  // '' = un rango (el autofiltro); si no, el nombre de la tabla dinámica que segmenta.
  const [pivot, setPivot] = useState('');

  const pivots = open ? (readSheetState()?.pivots ?? []) : [];
  const pv = pivots.find((p) => p.name === pivot) ?? null;
  // Encabezados (1.ª fila) del rango o del origen de la tabla dinámica → opciones de
  // campo; recalculados en vivo. De una tabla dinámica vale cualquier campo del origen,
  // esté o no en la tabla (como Excel).
  const headers = !open
    ? []
    : pv
      ? (readSheetValues(pv.source, pv.sourceSheet)?.[0] ?? []).map((v) => formatCellValue(v))
      : (readSheetGrid(range.trim())?.[0] ?? []);
  const fieldOpts = headers.map((h, i) => ({
    value: String(i),
    label: h.trim() !== '' ? h : t('common.columnN', { n: i + 1 }),
  }));
  const rr = parseRangeRef(range);
  const fi = Number(field);
  const canCreate = (pv !== null || rr !== null) && fi >= 0 && fi < fieldOpts.length;

  const create = (): void => {
    if (!canCreate) return;
    if (pv) {
      dispatchSheetCommand('sliceradd', JSON.stringify({ pivot: pv.name, col: fi }));
      setOpen(false);
      return;
    }
    if (!rr) return;
    // col ABSOLUTA = 1.ª columna del rango + índice del campo elegido.
    dispatchSheetCommand(
      'sliceradd',
      JSON.stringify({ range: range.trim().toUpperCase(), col: rr.c1 + fi }),
    );
    setOpen(false);
  };

  return (
    <div ref={rootRef} class="relative">
      <button
        type="button"
        class={
          'disabled:pointer-events-none disabled:opacity-40 ' +
          'text-text-muted inline-flex h-7 w-7 items-center justify-center rounded-sm ' +
          'hover:bg-bg-subtle hover:text-text ' +
          FOCUS_RING
        }
        aria-label={t('data.slicer.insertFull')}
        title={t('data.slicer.insertHint')}
        aria-haspopup="true"
        aria-expanded={open}
        disabled={bloqueado}
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => {
          if (!open) setRange(readSheetState()?.selection ?? 'A1:D10'); // prellena con la selección
          setOpen(!open);
        }}
      >
        <EditorIcon name="slicer" />
      </button>
      {open && (
        <div
          role="group"
          aria-label={t('data.slicer.insert')}
          class={`${POPOVER_SHELL} left-0 w-64 p-2`}
        >
          <div class="text-text pb-1 text-xs font-medium">{t('data.slicer')}</div>
          {pivots.length > 0 && (
            <label class="block pb-1.5">
              <span class="sytue-meta-dense mb-0.5 block">{t('query.source')}</span>
              <RibbonSelect
                label={t('query.source')}
                value={pivot}
                width={240}
                options={[
                  { value: '', label: t('data.rangeWithHeaders') },
                  ...pivots.map((p) => ({ value: p.name, label: p.name })),
                ]}
                onSelect={(v) => {
                  setPivot(v);
                  const p = pivots.find((x) => x.name === v);
                  const first = p ? [...(p.filters ?? []), ...p.rows, ...p.cols][0] : undefined;
                  setField(String(first?.field ?? 0));
                }}
              />
            </label>
          )}
          {!pv && (
            <label class="block pb-1.5">
              <span class="sytue-meta-dense mb-0.5 block">{t('data.rangeWithHeaders')}</span>
              <input
                class={
                  'border-border bg-surface text-text h-6 w-full rounded-sm border px-1.5 text-xs ' +
                  FOCUS_RING
                }
                value={range}
                onInput={(e) => setRange(e.currentTarget.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault();
                    create();
                  }
                }}
              />
            </label>
          )}
          {fieldOpts.length === 0 ? (
            <div class="sytue-meta-dense pb-2">{t('data.slicer.pickField')}</div>
          ) : (
            <label class="block pb-2">
              <span class="sytue-meta-dense mb-0.5 block">{t('common.field')}</span>
              <RibbonSelect
                label={t('common.field')}
                value={field}
                width={240}
                options={fieldOpts}
                onSelect={setField}
              />
            </label>
          )}
          <div class="sytue-meta-dense pb-2">{t('data.slicer.explain')}</div>
          <button
            type="button"
            class={
              'border-border bg-surface text-text hover:bg-bg-subtle h-6 w-full rounded-sm border px-2 text-xs ' +
              'disabled:pointer-events-none disabled:opacity-40 ' +
              FOCUS_RING
            }
            disabled={!canCreate}
            onClick={create}
          >
            {t('action.insert')}
          </button>
        </div>
      )}
    </div>
  );
}

/* ---------- Validación de datos: lista/número/fecha/longitud/fórmula (popover) ---------- */

function ValidationMenu(): JSX.Element {
  const bloqueado = useBloqueado('validadd');
  const t = useT();
  const { open, setOpen, rootRef } = usePopover();
  const [type, setType] = useState<DataValidationType>('list');
  const [text, setText] = useState(''); // lista
  const [op, setOp] = useState<DataValidationOp>('between');
  const [a, setA] = useState('');
  const [b, setB] = useState('');
  const [integer, setInteger] = useState(false); // solo número
  const [formula, setFormula] = useState('');
  const [errorMsg, setErrorMsg] = useState('');
  const [strict, setStrict] = useState(false);

  const st = readSheetState();
  const selection = st?.selection ?? 'A1';
  const validations = st?.validations ?? [];
  const twoOperands = op === 'between' || op === 'notBetween';
  const cmp = type === 'number' || type === 'date' || type === 'textLength';
  const operandOk = (s: string): boolean =>
    type === 'date' ? parseEsDate(s) !== null : parseEsNumberStrict(s) !== null;

  const criterionOk =
    type === 'list'
      ? parseValidationValues(text).length > 0
      : type === 'formula'
        ? formula.trim() !== ''
        : operandOk(a) && (!twoOperands || operandOk(b));
  const canApply = criterionOk && parseRangeRef(selection) !== null;

  const apply = (): void => {
    if (!canApply) return;
    const payload: Record<string, unknown> = { range: selection, strict };
    if (errorMsg.trim() !== '') payload['error'] = errorMsg.trim();
    if (type === 'list') {
      payload['values'] = parseValidationValues(text);
    } else if (type === 'formula') {
      payload['type'] = 'formula';
      payload['formula'] = formula.trim();
    } else {
      payload['type'] = type;
      payload['op'] = op;
      payload['a'] = a.trim();
      if (twoOperands) payload['b'] = b.trim();
      if (type === 'number' && integer) payload['integer'] = true;
    }
    dispatchSheetCommand('validadd', JSON.stringify(payload));
    setText('');
    setA('');
    setB('');
    setFormula('');
    setErrorMsg('');
    setInteger(false);
    setStrict(false);
    setOpen(false);
  };

  const operandPh = type === 'date' ? t('data.validation.datePlaceholder') : '0';
  const operandLabel =
    type === 'date'
      ? t('valid.kind.date')
      : type === 'textLength'
        ? t('valid.kind.textLength')
        : t('common.value');
  const onOperandKey = (e: KeyboardEvent): void => {
    if (e.key === 'Enter') {
      e.preventDefault();
      apply();
    }
  };

  return (
    <div ref={rootRef} class="relative">
      <button
        type="button"
        class={
          'disabled:pointer-events-none disabled:opacity-40 ' +
          'text-text-muted inline-flex h-7 w-7 items-center justify-center rounded-sm ' +
          'hover:bg-bg-subtle hover:text-text aria-pressed:bg-selected aria-pressed:text-text ' +
          FOCUS_RING
        }
        aria-label={t('data.validation')}
        title={t('data.validation.hint')}
        aria-haspopup="true"
        aria-expanded={open}
        disabled={bloqueado}
        aria-pressed={validations.length > 0}
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => setOpen(!open)}
      >
        <EditorIcon name="listBullet" />
      </button>
      {open && (
        <div
          role="group"
          aria-label={t('data.validation')}
          class={`${POPOVER_SHELL} left-0 w-64 p-2`}
        >
          <div class="text-text pb-1 text-xs font-medium">{t('data.validation')}</div>
          {validations.length > 0 && (
            <ul class="border-border mb-2 max-h-28 overflow-auto border-b pb-2">
              {validations.map((v, i) => (
                <li key={i} class="text-text flex items-center gap-1 py-0.5 text-xs">
                  <span class="min-w-0 flex-1 truncate" title={validationLabel(v, t)}>
                    <span class="font-medium">{v.range}</span>: {validationLabel(v, t)}
                  </span>
                  <button
                    type="button"
                    class={
                      'text-text-muted inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-sm ' +
                      'hover:bg-bg-subtle hover:text-danger-text ' +
                      FOCUS_RING
                    }
                    aria-label={t('sheet.validation.remove', { range: v.range })}
                    title={t('data.validation.remove')}
                    onClick={() => dispatchSheetCommand('validdel', String(i))}
                  >
                    <EditorIcon name="trash" size="sm" />
                  </button>
                </li>
              ))}
            </ul>
          )}
          <label class="block pb-1.5">
            <span class="sytue-meta-dense mb-0.5 block">{t('common.criterion')}</span>
            <RibbonSelect
              label={t('common.criterion')}
              value={type}
              width={232}
              options={DATA_VALIDATION_TYPES.map((o) => ({ value: o.value, label: t(o.labelKey) }))}
              onSelect={(v) => setType(v as DataValidationType)}
            />
          </label>

          {type === 'list' && (
            <label class="block pb-1.5">
              <span class="sytue-meta-dense mb-0.5 block">
                {t('data.validation.allowedValues')}
              </span>
              <textarea
                class={
                  'border-border bg-surface text-text h-20 w-full resize-none rounded-sm border px-1.5 py-1 text-xs ' +
                  'placeholder:text-text-subtle ' +
                  FOCUS_RING
                }
                value={text}
                placeholder={t('data.validation.listPlaceholder')}
                onInput={(e) => setText(e.currentTarget.value)}
              />
            </label>
          )}

          {cmp && (
            <>
              <label class="block pb-1.5">
                <span class="sytue-meta-dense mb-0.5 block">{t('common.condition')}</span>
                <RibbonSelect
                  label={t('common.condition')}
                  value={op}
                  width={232}
                  options={DATA_VALIDATION_OPS.map((o) => ({
                    value: o.value,
                    label: t(o.labelKey),
                  }))}
                  onSelect={(v) => setOp(v as DataValidationOp)}
                />
              </label>
              <div class="flex gap-1.5 pb-1.5">
                <label class="block min-w-0 flex-1">
                  <span class="sytue-meta-dense mb-0.5 block">{operandLabel}</span>
                  <input
                    class={inputCls}
                    value={a}
                    placeholder={operandPh}
                    onInput={(e) => setA(e.currentTarget.value)}
                    onKeyDown={onOperandKey}
                  />
                </label>
                {twoOperands && (
                  <label class="block min-w-0 flex-1">
                    <span class="sytue-meta-dense mb-0.5 block">{t('common.and')}</span>
                    <input
                      class={inputCls}
                      value={b}
                      placeholder={operandPh}
                      onInput={(e) => setB(e.currentTarget.value)}
                      onKeyDown={onOperandKey}
                    />
                  </label>
                )}
              </div>
              {type === 'number' && (
                <label class="text-text flex cursor-pointer items-center gap-1.5 pb-1.5 text-xs">
                  <Checkbox
                    checked={integer}
                    class="accent-primary"
                    onChange={(e) => setInteger(e.currentTarget.checked)}
                  />
                  {t('data.validation.integerOnly')}
                </label>
              )}
            </>
          )}

          {type === 'formula' && (
            <label class="block pb-1.5">
              <span class="sytue-meta-dense mb-0.5 block">{t('data.validation.formula')}</span>
              <input
                class={inputCls}
                value={formula}
                placeholder="=A1>0"
                onInput={(e) => setFormula(e.currentTarget.value)}
                onKeyDown={onOperandKey}
              />
            </label>
          )}

          <label class="block pb-1.5">
            <span class="sytue-meta-dense mb-0.5 block">{t('data.validation.errorMsg')}</span>
            <input
              class={inputCls}
              value={errorMsg}
              placeholder={t('data.validation.invalid')}
              onInput={(e) => setErrorMsg(e.currentTarget.value)}
            />
          </label>

          <label class="text-text flex cursor-pointer items-center gap-1.5 pb-2 text-xs">
            <Checkbox
              checked={strict}
              class="accent-primary"
              onChange={(e) => setStrict(e.currentTarget.checked)}
            />
            {t('data.validation.reject')}
          </label>
          <button
            type="button"
            class={
              'border-border bg-surface text-text hover:bg-bg-subtle h-6 w-full rounded-sm border px-2 text-xs ' +
              'disabled:pointer-events-none disabled:opacity-40 ' +
              FOCUS_RING
            }
            disabled={!canApply}
            onClick={apply}
          >
            {t('data.validation.applyTo', { range: selection })}
          </button>
        </div>
      )}
    </div>
  );
}

/* ---------- Sparklines (popover: rango de datos + tipo + color) ---------- */

const SPARK_TYPE_OPTIONS: { value: Sparkline['type']; labelKey: DocsKey }[] = [
  { value: 'line', labelKey: 'spark.kind.line' },
  { value: 'column', labelKey: 'spark.kind.column' },
  { value: 'winloss', labelKey: 'spark.kind.winloss' },
];
const SPARK_COLORS = [SERIE.azul, SERIE.verde, SERIE.rojo, SERIE.ambar] as const;

/** Primera celda («B2») de una selección A1 («B2:D5» → «B2»); '' si no parsea. */
function anchorCell(selection: string): string {
  const rr = parseRangeRef(selection);
  if (!rr) return '';
  return `${String.fromCharCode(65 + Math.min(rr.c1, 25))}${rr.r1 + 1}`;
}

function SparklineMenu(): JSX.Element {
  const bloqueado = useBloqueado('sparkadd');
  const t = useT();
  const { open, setOpen, rootRef } = usePopover();
  const [range, setRange] = useState('A1:A8');
  const [type, setType] = useState<Sparkline['type']>('line');
  const [color, setColor] = useState<string>(SPARK_COLORS[0]);

  const st = readSheetState();
  const host = anchorCell(st?.selection ?? '');
  const existing = st?.sparklines ?? [];
  const valid = parseRangeRef(range) !== null && host !== '';

  const insert = (): void => {
    if (!valid) return;
    dispatchSheetCommand(
      'sparkadd',
      JSON.stringify({ range: range.trim().toUpperCase(), type, color, cell: host }),
    );
    setOpen(false);
  };

  return (
    <div ref={rootRef} class="relative">
      <button
        type="button"
        class={
          'disabled:pointer-events-none disabled:opacity-40 ' +
          'text-text-muted inline-flex h-7 w-7 items-center justify-center rounded-sm ' +
          'hover:bg-bg-subtle hover:text-text ' +
          FOCUS_RING
        }
        aria-label={t('data.sparkline.insert')}
        title={t('data.sparkline.insertHint')}
        aria-haspopup="true"
        aria-expanded={open}
        disabled={bloqueado}
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => {
          if (!open) setRange(readSheetState()?.selection ?? 'A1:A8');
          setOpen(!open);
        }}
      >
        <EditorIcon name="chart" />
      </button>
      {open && (
        <div
          role="group"
          aria-label={t('data.sparkline.insert')}
          class={`${POPOVER_SHELL} left-0 w-56 p-2`}
        >
          <div class="text-text pb-1 text-xs font-medium">{t('data.sparkline.title')}</div>
          {existing.length > 0 && (
            <ul class="border-border mb-2 max-h-24 overflow-auto border-b pb-2">
              {existing.map((sp) => (
                <li key={sp.cell} class="text-text flex items-center gap-1 py-0.5 text-xs">
                  <span class="min-w-0 flex-1 truncate">
                    <span class="font-medium">{sp.cell}</span> · {sp.range}
                  </span>
                  <button
                    type="button"
                    class={
                      'text-text-muted inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-sm ' +
                      'hover:bg-bg-subtle hover:text-danger-text ' +
                      FOCUS_RING
                    }
                    aria-label={t('sheet.sparkline.remove', { cell: sp.cell })}
                    title={t('data.sparkline.remove')}
                    onClick={() => dispatchSheetCommand('sparkdel', sp.cell)}
                  >
                    <EditorIcon name="trash" size="sm" />
                  </button>
                </li>
              ))}
            </ul>
          )}
          <label class="block pb-1.5">
            <span class="sytue-meta-dense mb-0.5 block">{t('data.sparkline.range')}</span>
            <input
              class={
                'border-border bg-surface text-text h-6 w-full rounded-sm border px-1.5 text-xs ' +
                FOCUS_RING
              }
              value={range}
              onInput={(e) => setRange(e.currentTarget.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  insert();
                }
              }}
            />
          </label>
          <div class="pb-1.5">
            <RibbonSelect
              label={t('chart.field.type')}
              value={type}
              width={208}
              options={SPARK_TYPE_OPTIONS.map((o) => ({ value: o.value, label: t(o.labelKey) }))}
              onSelect={(v) => setType(v as Sparkline['type'])}
            />
          </div>
          <div
            class="flex items-center gap-1 pb-2"
            role="group"
            aria-label={t('palette.colorLabel')}
          >
            {SPARK_COLORS.map((c) => (
              <button
                key={c}
                type="button"
                class={`border-border relative h-5 w-5 rounded-[3px] border ${FOCUS_RING}`}
                style={{ backgroundColor: c }}
                aria-label={t('common.colorN', { c })}
                title={c}
                aria-pressed={c === color}
                onClick={() => setColor(c)}
              >
                {c === color && (
                  <span class="absolute inset-0 grid place-items-center text-white mix-blend-difference">
                    <EditorIcon name="check" size="sm" />
                  </span>
                )}
              </button>
            ))}
          </div>
          <div class="sytue-meta-dense pb-2">
            {host ? t('data.sparkline.willInsert', { cell: host }) : t('data.sparkline.pickHost')}
          </div>
          <button
            type="button"
            class={
              'border-border bg-surface text-text hover:bg-bg-subtle h-6 w-full rounded-sm border px-2 text-xs ' +
              'disabled:pointer-events-none disabled:opacity-40 ' +
              FOCUS_RING
            }
            disabled={!valid}
            onClick={insert}
          >
            {t('action.insert')}
          </button>
        </div>
      )}
    </div>
  );
}

/* ---------- Tabla dinámica (popover: origen + campos + destino) ---------- */

/** «Tabla dinámica»: con el ancla dentro de una, la edita; si no, crea una nueva con la
 *  selección como origen. El diálogo entero es `PivotModal`. */
function PivotMenu(): JSX.Element {
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState<PivotDraft>(() => newPivotDraft('A1:D10'));
  return (
    <>
      <RibbonButton
        commandId="pivot"
        icon="table"
        onAction={() => {
          const st = readSheetState();
          const at = st?.pivotAt;
          if (at) {
            const { range: _range, ...def } = at;
            setDraft(def);
          } else setDraft(newPivotDraft(st?.selection ?? 'A1:D10'));
          setOpen(true);
        }}
      />
      <PivotModal open={open} initial={draft} onClose={() => setOpen(false)} />
    </>
  );
}

/** Campo etiquetado del asistente de tabla dinámica (label encima del select). */
function PivotField({
  label,
  value,
  options,
  onSelect,
}: {
  label: string;
  value: string;
  options: { value: string; label: string }[];
  onSelect: (v: string) => void;
}): JSX.Element {
  return (
    <label class="flex flex-col gap-0.5">
      <span class="sytue-meta-dense">{label}</span>
      <RibbonSelect label={label} value={value} width={116} options={options} onSelect={onSelect} />
    </label>
  );
}

/* ---------- Subtotales (popover: agrupar por una columna + función de agregación) ---------- */

/** Funciones de subtotal: las de la fila de totales de tabla salvo «Ninguno». */
const SUBTOTAL_FNS = TABLE_TOTAL_FNS.filter((f) => f.value !== 'none');

function SubtotalMenu(): JSX.Element {
  const bloqueado = useBloqueado('subtotal');
  const t = useT();
  const { open, setOpen, rootRef } = usePopover();
  const [range, setRange] = useState('A1:D10');
  const [groupCol, setGroupCol] = useState('0');
  const [fn, setFn] = useState<TableTotalFn>('sum');
  /** «Añadir subtotal a»: columnas relativas al rango. null = las que tengan números. */
  const [addTo, setAddTo] = useState<number[] | null>(null);

  // Encabezados del rango (1.ª fila) → opciones de columna de grupo; en vivo.
  const grid = open ? (readSheetGrid(range.trim()) ?? []) : [];
  const headers = grid[0] ?? [];
  const colOpts = headers.map((h, i) => ({
    value: String(i),
    label: h.trim() !== '' ? h : t('common.columnN', { n: i + 1 }),
  }));
  const gc = Number(groupCol);
  const canApply =
    parseRangeRef(range) !== null && colOpts.length > 0 && gc >= 0 && gc < colOpts.length;
  // Sin elegir nada, se marcan las columnas con algún número bajo la cabecera, que es lo
  // que el asistente haría solo: así lo que se ve marcado es lo que va a pasar.
  const auto = headers
    .map((_, i) => i)
    .filter((i) => i !== gc && grid.slice(1).some((row) => parseEsNumber(row[i] ?? '') !== null));
  const checked = addTo ?? auto;

  const apply = (): void => {
    if (!canApply) return;
    dispatchSheetCommand(
      'subtotal',
      JSON.stringify({
        range: range.trim().toUpperCase(),
        groupCol: gc,
        fn,
        // Sin tocar las casillas decide la hoja, que ve los valores y no su texto.
        ...(addTo ? { cols: addTo.filter((i) => i !== gc) } : {}),
      }),
    );
    setOpen(false);
  };
  const removeAll = (): void => {
    if (parseRangeRef(range) === null) return;
    dispatchSheetCommand(
      'subtotal',
      JSON.stringify({ range: range.trim().toUpperCase(), remove: true }),
    );
    setOpen(false);
  };

  return (
    <div ref={rootRef} class="relative">
      <button
        type="button"
        class={
          'disabled:pointer-events-none disabled:opacity-40 ' +
          'text-text-muted inline-flex h-7 w-7 items-center justify-center rounded-sm ' +
          'hover:bg-bg-subtle hover:text-text ' +
          FOCUS_RING
        }
        aria-label={t('data.subtotals')}
        title={t('data.subtotals.hint')}
        aria-haspopup="true"
        aria-expanded={open}
        disabled={bloqueado}
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => {
          if (!open) {
            setRange(readSheetState()?.selection ?? 'A1:D10'); // prellena con la selección
            setAddTo(null);
          }
          setOpen(!open);
        }}
      >
        <EditorIcon name="subtotal" />
      </button>
      {open && (
        <div
          role="group"
          aria-label={t('data.subtotals')}
          class={`${POPOVER_SHELL} left-0 w-64 p-2`}
        >
          <div class="text-text pb-1 text-xs font-medium">{t('data.subtotals')}</div>
          <label class="block pb-1.5">
            <span class="sytue-meta-dense mb-0.5 block">{t('data.rangeWithHeaders')}</span>
            <input
              class={
                'border-border bg-surface text-text h-6 w-full rounded-sm border px-1.5 text-xs ' +
                FOCUS_RING
              }
              value={range}
              onInput={(e) => setRange(e.currentTarget.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  apply();
                }
              }}
            />
          </label>
          {colOpts.length === 0 ? (
            <div class="sytue-meta-dense pb-2">{t('data.subtotals.pickGroup')}</div>
          ) : (
            <div class="grid grid-cols-2 gap-1.5 pb-2">
              <PivotField
                label={t('common.groupBy')}
                value={groupCol}
                options={colOpts}
                onSelect={setGroupCol}
              />
              <PivotField
                label={t('common.function')}
                value={fn}
                options={SUBTOTAL_FNS.map((o) => ({ value: o.value, label: t(o.labelKey) }))}
                onSelect={(v) => setFn(v as TableTotalFn)}
              />
            </div>
          )}
          {colOpts.length > 0 && (
            <fieldset class="pb-1.5">
              <legend class="sytue-meta-dense mb-0.5">{t('data.subtotals.addTo')}</legend>
              <div class="border-border max-h-28 overflow-auto rounded-sm border px-1.5 py-1">
                {colOpts.map((o, i) =>
                  i === gc ? null : (
                    <label
                      key={o.value}
                      class="text-text flex cursor-pointer items-center gap-1.5 py-px text-xs"
                    >
                      <Checkbox
                        checked={checked.includes(i)}
                        class="accent-primary"
                        onChange={(e) =>
                          setAddTo(
                            e.currentTarget.checked
                              ? [...checked, i].sort((a, b) => a - b)
                              : checked.filter((x) => x !== i),
                          )
                        }
                      />
                      <span class="truncate">{o.label}</span>
                    </label>
                  ),
                )}
              </div>
            </fieldset>
          )}
          <div class="sytue-meta-dense pb-2">{t('data.subtotals.explain')}</div>
          <div class="grid grid-cols-2 gap-1">
            <button
              type="button"
              class={
                'border-border bg-surface text-text hover:bg-bg-subtle h-6 rounded-sm border px-2 text-xs ' +
                'disabled:pointer-events-none disabled:opacity-40 ' +
                FOCUS_RING
              }
              disabled={parseRangeRef(range) === null}
              onClick={removeAll}
            >
              {t('data.subtotals.removeAll')}
            </button>
            <button
              type="button"
              class={
                'border-border bg-surface text-text hover:bg-bg-subtle h-6 rounded-sm border px-2 text-xs ' +
                'disabled:pointer-events-none disabled:opacity-40 ' +
                FOCUS_RING
              }
              disabled={!canApply}
              onClick={apply}
            >
              {t('action.apply')}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

/* ---------- Rellenar con IA: Sytue AI propone lo que falta en una columna ---------- */

/**
 * La persona elige el rango (con encabezados) y la columna; Sytue AI ve las filas que
 * ya tienen valor como ejemplos y propone el resto. Lo propuesto se enseña fila a fila
 * y NO se escribe hasta pulsar «Escribir». Qué se manda y cómo se lee es
 * `sheets/relleno.ts`, compartido con la app nativa por vectores.
 */
function AiFillMenu(): JSX.Element {
  const bloqueado = useBloqueado('fillcells');
  const t = useT();
  const { open, setOpen, rootRef } = usePopover();
  const [range, setRange] = useState('');
  const [target, setTarget] = useState('0');
  const [hint, setHint] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  /** Lo propuesto: celda → valor, en el orden de las filas. */
  const [proposal, setProposal] = useState<[string, string][] | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  // Cerrar el panel corta la petición en vuelo: no se escribe nada de ella.
  useEffect(() => {
    if (!open) abortRef.current?.abort();
  }, [open]);

  const rr = parseRangeRef(range);
  const grid = open && rr ? (readSheetGrid(range.trim()) ?? []) : [];
  const headers = grid[0] ?? [];
  const colOpts = headers.map((h, i) => ({
    value: String(i),
    label: h.trim() !== '' ? h : t('common.columnN', { n: i + 1 }),
  }));
  const col = Number(target);
  const prompt =
    rr && col < headers.length
      ? buildFillPrompt({
          headers,
          rows: grid.slice(1),
          firstRow: rr.r1 + 2,
          target: col,
          hint,
        })
      : null;

  const reset = (): void => {
    abortRef.current?.abort();
    setProposal(null);
    setError('');
  };

  const propose = async (): Promise<void> => {
    if (!prompt || !rr) return;
    reset();
    const ctrl = new AbortController();
    abortRef.current = ctrl;
    setBusy(true);
    try {
      const res = await fetch('/api/ai', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({
          action: 'rellenar',
          instruction: prompt.instruction,
          context: prompt.context,
          kind: 'spreadsheet',
        }),
        signal: ctrl.signal,
      });
      const data: unknown = await res.json().catch(() => null);
      if (!res.ok) {
        setError(t(aiErrorKey(aiErrorCode(data))));
        return;
      }
      const text = parseAiResponse(data);
      if (text === null) {
        setError(t('ai.error.badResponse'));
        return;
      }
      const letter = colLetter(rr.c1 + col);
      const got = parseFillResponse(text, prompt.pending);
      const rows = prompt.pending.flatMap((n): [string, string][] =>
        got.has(n) ? [[`${letter}${n}`, got.get(n)!]] : [],
      );
      if (rows.length === 0) setError(t('data.aiFill.none'));
      else setProposal(rows);
    } catch {
      if (!ctrl.signal.aborted) setError(t(aiErrorKey()));
    } finally {
      if (abortRef.current === ctrl) {
        abortRef.current = null;
        setBusy(false);
      }
    }
  };

  const write = (): void => {
    if (!proposal) return;
    dispatchSheetCommand('fillcells', JSON.stringify({ values: Object.fromEntries(proposal) }));
    setProposal(null);
    setOpen(false);
  };

  const btn =
    'border-border bg-surface text-text hover:bg-bg-subtle h-6 rounded-sm border px-2 text-xs ' +
    'disabled:pointer-events-none disabled:opacity-40 ' +
    FOCUS_RING;

  return (
    <div ref={rootRef} class="relative">
      <button
        type="button"
        class={
          'disabled:pointer-events-none disabled:opacity-40 ' +
          'text-text-muted inline-flex h-7 w-7 items-center justify-center rounded-sm ' +
          'hover:bg-bg-subtle hover:text-text aria-expanded:bg-selected aria-expanded:text-text ' +
          FOCUS_RING
        }
        aria-label={t('data.aiFill')}
        title={t('data.aiFill.hint')}
        aria-haspopup="true"
        aria-expanded={open}
        disabled={bloqueado}
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => {
          if (!open) {
            const sel = readSheetState()?.selection ?? '';
            const s = parseRangeRef(sel);
            setRange(sel);
            // Por defecto, la última columna: la que suele quedar por rellenar.
            setTarget(String(s ? s.c2 - s.c1 : 0));
            setHint('');
            reset();
          }
          setOpen(!open);
        }}
      >
        <EditorIcon name="sparkles" />
      </button>
      {open && (
        <div role="group" aria-label={t('data.aiFill')} class={`${POPOVER_SHELL} left-0 w-72 p-2`}>
          <div class="text-text pb-1 text-xs font-medium">{t('data.aiFill')}</div>
          <label class="block pb-1.5">
            <span class="sytue-meta-dense mb-0.5 block">{t('data.rangeWithHeaders')}</span>
            <input
              class={inputCls}
              value={range}
              placeholder="A1:C20"
              onInput={(e) => {
                setRange(e.currentTarget.value);
                reset();
              }}
            />
          </label>
          {colOpts.length > 0 && (
            <div class="pb-1.5">
              <PivotField
                label={t('data.aiFill.column')}
                value={target}
                options={colOpts}
                onSelect={(v) => {
                  setTarget(v);
                  reset();
                }}
              />
            </div>
          )}
          <label class="block pb-1.5">
            <span class="sytue-meta-dense mb-0.5 block">{t('data.aiFill.instruction')}</span>
            <input
              class={inputCls}
              value={hint}
              placeholder={t('data.aiFill.instructionPlaceholder')}
              onInput={(e) => {
                setHint(e.currentTarget.value);
                reset();
              }}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  if (prompt && !busy) void propose();
                }
              }}
            />
          </label>
          <div class="sytue-meta-dense pb-2">
            {prompt || colOpts.length === 0 ? t('data.aiFill.privacy') : t('data.aiFill.nothing')}
          </div>
          <button
            type="button"
            class={`${btn} w-full`}
            disabled={!prompt || busy}
            onClick={() => void propose()}
          >
            {busy ? t('data.aiFill.busy') : t('data.aiFill.propose')}
          </button>

          {error !== '' && <div class="text-danger-text text-2xs pt-2">{error}</div>}

          {proposal && prompt && (
            <div class="border-border mt-2 border-t pt-2">
              <div class="text-text text-2xs pb-1">
                {t('data.aiFill.count', {
                  count: proposal.length,
                  total: prompt.pending.length,
                })}
              </div>
              <div class="border-border max-h-40 overflow-auto rounded-sm border px-1.5 py-1">
                {proposal.map(([ref, v]) => (
                  <div key={ref} class="flex gap-2 py-px text-xs">
                    <span class="text-text-muted w-10 shrink-0 tabular-nums">{ref}</span>
                    <span class="text-text truncate">{v}</span>
                  </div>
                ))}
              </div>
              <div class="flex gap-1 pt-2">
                <button type="button" class={`${btn} flex-1 font-medium`} onClick={write}>
                  {t('data.aiFill.write')}
                </button>
                <button
                  type="button"
                  class={`${btn} flex-1`}
                  onClick={() => {
                    setProposal(null);
                    setOpen(false);
                  }}
                >
                  {t('action.cancel')}
                </button>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

/* ---------- Buscar objetivo (Análisis Y-si): popover con solver + aplicar ---------- */

/** ¿El texto es UNA sola celda (no un rango)? */
function isSingleCell(text: string): boolean {
  const rr = parseRangeRef(text.trim());
  return rr !== null && rr.c1 === rr.c2 && rr.r1 === rr.r2;
}

/** Por qué la sonda no se puede preparar. La clave, no el texto: lo traduce quien lo pinta. */
const PROBE_REASONS: Record<Exclude<GoalSeekProbeResult, { ok: true }>['reason'], DocsKey> = {
  'no-sheet': 'data.err.noSheet',
  'bad-target': 'data.goalSeek.err.badTarget',
  'bad-var': 'data.goalSeek.err.badVar',
  'target-not-formula': 'data.goalSeek.err.targetNotFormula',
  'var-is-formula': 'data.goalSeek.err.varIsFormula',
};

function GoalSeekMenu(): JSX.Element {
  const bloqueado = useBloqueado('goalseek');
  const t = useT();
  const { open, setOpen, rootRef } = usePopover();
  const [setCell, setSetCell] = useState('');
  const [toValue, setToValue] = useState('0');
  const [byCell, setByCell] = useState('');
  const [result, setResult] = useState<GoalSeekResult | null>(null);
  const [error, setError] = useState('');

  const canSolve = isSingleCell(setCell) && isSingleCell(byCell) && parseEsNumber(toValue) !== null;

  const solve = (): void => {
    setError('');
    setResult(null);
    const tv = parseEsNumber(toValue);
    if (tv === null) {
      setError(t('data.goalSeek.err.badValue'));
      return;
    }
    const probe = makeGoalSeekProbe(setCell.trim(), byCell.trim());
    if (!probe.ok) {
      setError(t(PROBE_REASONS[probe.reason]));
      return;
    }
    // Iteraciones acotadas por el solver (no bloquea la UI); corre en un motor desechable.
    setResult(goalSeek(probe.probe.f, tv, { x0: probe.probe.current }));
  };

  const applyResult = (): void => {
    if (!result) return;
    dispatchSheetCommand('goalseek', JSON.stringify({ cell: byCell.trim(), value: result.value }));
    setResult(null);
    setOpen(false);
  };

  return (
    <div ref={rootRef} class="relative">
      <button
        type="button"
        class={
          'disabled:pointer-events-none disabled:opacity-40 ' +
          'text-text-muted inline-flex h-7 w-7 items-center justify-center rounded-sm ' +
          'hover:bg-bg-subtle hover:text-text aria-expanded:bg-selected aria-expanded:text-text ' +
          FOCUS_RING
        }
        aria-label={t('data.goalSeek')}
        title={t('data.goalSeek.hint')}
        aria-haspopup="true"
        aria-expanded={open}
        disabled={bloqueado}
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => {
          if (!open) {
            // Prellena «Definir la celda» con el ancla de la selección; el resto en blanco.
            setSetCell(anchorCell(readSheetState()?.selection ?? ''));
            setByCell('');
            setToValue('0');
            setResult(null);
            setError('');
          }
          setOpen(!open);
        }}
      >
        <EditorIcon name="target" />
      </button>
      {open && (
        <div
          role="group"
          aria-label={t('data.goalSeek')}
          class={`${POPOVER_SHELL} left-0 w-64 p-2`}
        >
          <div class="text-text pb-1 text-xs font-medium">{t('data.goalSeek')}</div>
          <label class="block pb-1.5">
            <span class="sytue-meta-dense mb-0.5 block">{t('data.goalSeek.setCell')}</span>
            <input
              class={inputCls}
              value={setCell}
              placeholder="B5"
              onInput={(e) => setSetCell(e.currentTarget.value)}
            />
          </label>
          <label class="block pb-1.5">
            <span class="sytue-meta-dense mb-0.5 block">{t('data.withValue')}</span>
            <input
              class={inputCls}
              value={toValue}
              onInput={(e) => setToValue(e.currentTarget.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  if (canSolve) solve();
                }
              }}
            />
          </label>
          <label class="block pb-2">
            <span class="sytue-meta-dense mb-0.5 block">{t('data.goalSeek.byCell')}</span>
            <input
              class={inputCls}
              value={byCell}
              placeholder="B3"
              onInput={(e) => setByCell(e.currentTarget.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  if (canSolve) solve();
                }
              }}
            />
          </label>
          <button
            type="button"
            class={
              'border-border bg-surface text-text hover:bg-bg-subtle h-6 w-full rounded-sm border px-2 text-xs ' +
              'disabled:pointer-events-none disabled:opacity-40 ' +
              FOCUS_RING
            }
            disabled={!canSolve}
            onClick={solve}
          >
            {t('action.ok')}
          </button>

          {error !== '' && <div class="text-danger-text text-2xs pt-2">{error}</div>}

          {result && (
            <div class="border-border mt-2 border-t pt-2">
              <div class="text-text text-2xs">
                {result.solved
                  ? t('data.goalSeek.found', { cell: byCell.trim().toUpperCase() })
                  : t('data.goalSeek.approx')}
              </div>
              <div class="text-text pt-0.5 text-xs font-medium">
                {byCell.trim().toUpperCase()} = {formatCellValue(result.value)}
              </div>
              {!result.solved && (
                <div class="sytue-meta-dense pt-0.5">
                  {t('data.goalSeek.residual', {
                    error: formatCellValue(result.finalError),
                    iterations: result.iterations,
                  })}
                </div>
              )}
              <div class="flex gap-1 pt-2">
                <button
                  type="button"
                  class={
                    'border-border bg-surface text-text hover:bg-bg-subtle h-6 flex-1 rounded-sm border px-2 text-xs font-medium ' +
                    FOCUS_RING
                  }
                  onClick={applyResult}
                >
                  {t('action.apply')}
                </button>
                <button
                  type="button"
                  class={
                    'border-border bg-surface text-text hover:bg-bg-subtle h-6 flex-1 rounded-sm border px-2 text-xs ' +
                    FOCUS_RING
                  }
                  onClick={() => {
                    setResult(null);
                    setOpen(false);
                  }}
                >
                  {t('action.cancel')}
                </button>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

/* ---------- Solver (Análisis Y-si): optimización multivariable con restricciones ---------- */

/** Tope de celdas variable del Solver (Nelder-Mead acotado → UI responsiva).
 *  ponytail: subir si se pide; con muchas variables conviene un multi-arranque. */
const SOLVER_MAX_VARS = 12;

/** Expande el texto de celdas variable (refs sueltas y/o rangos «B3:B5») a una
 *  lista de celdas únicas (una por celda), acotada a SOLVER_MAX_VARS. `splitRefs`
 *  y `colLetter` son declaraciones de función (hoisted), definidas más abajo. */
function expandVarRefs(text: string): string[] {
  const out: string[] = [];
  const seen = new Set<string>();
  for (const tok of splitRefs(text)) {
    const rr = parseRangeRef(tok);
    if (!rr) continue;
    for (let r = rr.r1; r <= rr.r2; r++) {
      for (let c = rr.c1; c <= rr.c2; c++) {
        const ref = `${colLetter(c)}${r + 1}`;
        if (!seen.has(ref)) {
          seen.add(ref);
          out.push(ref);
          if (out.length >= SOLVER_MAX_VARS) return out;
        }
      }
    }
  }
  return out;
}

/** Fila de restricción del diálogo: celda LHS, relación y RHS (número o celda). */
interface SolverRow {
  lhs: string;
  rel: SolverRelation;
  rhs: string;
}

/** Por qué la sonda del Solver no se puede preparar. La clave, no el texto. */
const SOLVER_REASONS: Record<Exclude<SolverProbeResult, { ok: true }>['reason'], DocsKey> = {
  'no-sheet': 'data.err.noSheet',
  'bad-target': 'data.solver.err.badTarget',
  'target-not-formula': 'data.solver.err.targetNotFormula',
  'no-vars': 'data.solver.err.noVars',
  'bad-var': 'data.solver.err.badVar',
  'var-is-formula': 'data.solver.err.varIsFormula',
  'bad-constraint': 'data.solver.err.badConstraint',
};

const SOLVER_KINDS: { value: SolverGoal['kind']; labelKey: DocsKey }[] = [
  { value: 'max', labelKey: 'solver.goal.max' },
  { value: 'min', labelKey: 'solver.goal.min' },
  { value: 'value', labelKey: 'solver.goal.value' },
];
const SOLVER_RELS: { value: SolverRelation; label: string }[] = [
  { value: 'lte', label: '≤' },
  { value: 'gte', label: '≥' },
  { value: 'eq', label: '=' },
];

function SolverMenu(): JSX.Element {
  const bloqueado = useBloqueado('solverapply');
  const t = useT();
  const { open, setOpen, rootRef } = usePopover();
  const [target, setTarget] = useState('');
  const [kind, setKind] = useState<SolverGoal['kind']>('max');
  const [targetValue, setTargetValue] = useState('0');
  const [vars, setVars] = useState('');
  const [rows, setRows] = useState<SolverRow[]>([]);
  const [result, setResult] = useState<{ res: SolverResult; cells: string[] } | null>(null);
  const [error, setError] = useState('');

  const varCells = expandVarRefs(vars);
  const valueOk = kind !== 'value' || parseEsNumber(targetValue) !== null;
  const canSolve = isSingleCell(target) && varCells.length > 0 && valueOk;

  const setRow = (i: number, patch: Partial<SolverRow>): void =>
    setRows((rs) => rs.map((r, j) => (j === i ? { ...r, ...patch } : r)));

  const solveNow = (): void => {
    setError('');
    setResult(null);
    if (!canSolve) return;
    const goal: SolverGoal =
      kind === 'value' ? { kind, target: parseEsNumber(targetValue) ?? 0 } : { kind };
    // Solo restricciones con LHS y RHS no vacíos (las incompletas se ignoran).
    const active = rows.filter((r) => r.lhs.trim() !== '' && r.rhs.trim() !== '');
    const probe = makeSolverProbe(
      target.trim(),
      varCells,
      active.map((r) => ({ lhs: r.lhs.trim(), rhs: r.rhs.trim() })),
    );
    if (!probe.ok) {
      setError(t(SOLVER_REASONS[probe.reason]));
      return;
    }
    const constraints: SolverConstraint[] = active.map((r) => ({ relation: r.rel }));
    // Iteraciones acotadas por el solver (no bloquea la UI); motor desechable.
    setResult({
      res: solve(probe.probe.evaluate, probe.probe.current, goal, constraints),
      cells: varCells,
    });
  };

  const applyResult = (): void => {
    if (!result) return;
    const values: Record<string, number> = {};
    result.cells.forEach((ref, i) => {
      values[ref] = result.res.values[i] ?? 0;
    });
    dispatchSheetCommand('solverapply', JSON.stringify({ values }));
    setResult(null);
    setOpen(false);
  };

  const segCls = (on: boolean): string =>
    'h-6 flex-1 rounded-sm border px-1.5 text-2xs ' +
    (on
      ? 'border-text bg-selected text-text'
      : 'border-border bg-surface text-text hover:bg-bg-subtle') +
    ' ' +
    FOCUS_RING;

  return (
    <div ref={rootRef} class="relative">
      <button
        type="button"
        class={
          'disabled:pointer-events-none disabled:opacity-40 ' +
          'text-text-muted inline-flex h-7 w-7 items-center justify-center rounded-sm ' +
          'hover:bg-bg-subtle hover:text-text aria-expanded:bg-selected aria-expanded:text-text ' +
          FOCUS_RING
        }
        aria-label={t('data.solver')}
        title={t('data.solver.hint')}
        aria-haspopup="true"
        aria-expanded={open}
        disabled={bloqueado}
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => {
          if (!open) {
            // Prellena el objetivo con el ancla de la selección; el resto en blanco.
            setTarget(anchorCell(readSheetState()?.selection ?? ''));
            setKind('max');
            setTargetValue('0');
            setVars('');
            setRows([]);
            setResult(null);
            setError('');
          }
          setOpen(!open);
        }}
      >
        <EditorIcon name="solver" />
      </button>
      {open && (
        <div role="group" aria-label={t('data.solver')} class={`${POPOVER_SHELL} left-0 w-72 p-2`}>
          <div class="text-text pb-1 text-xs font-medium">{t('data.solver')}</div>

          <label class="block pb-1.5">
            <span class="sytue-meta-dense mb-0.5 block">{t('data.targetCell')}</span>
            <input
              class={inputCls}
              value={target}
              placeholder="B5"
              onInput={(e) => setTarget(e.currentTarget.value)}
            />
          </label>

          <div class="pb-1.5">
            <span class="sytue-meta-dense mb-0.5 block">{t('data.solver.goal')}</span>
            <div class="flex items-center gap-1">
              {SOLVER_KINDS.map((k) => (
                <button
                  key={k.value}
                  type="button"
                  class={segCls(kind === k.value)}
                  aria-pressed={kind === k.value}
                  onClick={() => setKind(k.value)}
                >
                  {t(k.labelKey)}
                </button>
              ))}
              {kind === 'value' && (
                <input
                  class={
                    'border-border bg-surface text-text h-6 w-16 rounded-sm border px-1.5 text-xs ' +
                    FOCUS_RING
                  }
                  aria-label={t('data.targetValue')}
                  value={targetValue}
                  onInput={(e) => setTargetValue(e.currentTarget.value)}
                />
              )}
            </div>
          </div>

          <label class="block pb-1.5">
            <span class="sytue-meta-dense mb-0.5 block">{t('data.solver.vars')}</span>
            <input
              class={inputCls}
              value={vars}
              placeholder={t('data.solver.varsPlaceholder')}
              onInput={(e) => setVars(e.currentTarget.value)}
            />
          </label>

          <div class="pb-1.5">
            <div class="mb-0.5 flex items-center justify-between">
              <span class="sytue-meta-dense">{t('data.constraints')}</span>
              <button
                type="button"
                class={
                  'text-text-muted text-2xs inline-flex h-5 items-center gap-0.5 rounded-sm px-1 ' +
                  'hover:bg-bg-subtle hover:text-text ' +
                  FOCUS_RING
                }
                onClick={() => setRows((rs) => [...rs, { lhs: '', rel: 'lte', rhs: '' }])}
              >
                <EditorIcon name="plus" size="sm" /> {t('action.add')}
              </button>
            </div>
            {rows.length === 0 ? (
              <div class="sytue-meta-dense">{t('data.solver.noConstraints')}</div>
            ) : (
              <ul class="flex flex-col gap-1">
                {rows.map((r, i) => (
                  <li key={i} class="flex items-center gap-1">
                    <input
                      class={
                        'border-border bg-surface text-text h-6 min-w-0 flex-1 rounded-sm border px-1.5 text-xs ' +
                        FOCUS_RING
                      }
                      aria-label={t('data.constraint.cell')}
                      value={r.lhs}
                      placeholder="C3"
                      onInput={(e) => setRow(i, { lhs: e.currentTarget.value })}
                    />
                    <select
                      class={
                        'border-border bg-surface text-text h-6 shrink-0 cursor-pointer rounded-sm border px-0.5 text-xs ' +
                        FOCUS_RING
                      }
                      aria-label={t('data.relation')}
                      value={r.rel}
                      onChange={(e) => setRow(i, { rel: e.currentTarget.value as SolverRelation })}
                    >
                      {SOLVER_RELS.map((o) => (
                        <option key={o.value} value={o.value}>
                          {o.label}
                        </option>
                      ))}
                    </select>
                    <input
                      class={
                        'border-border bg-surface text-text h-6 min-w-0 flex-1 rounded-sm border px-1.5 text-xs ' +
                        FOCUS_RING
                      }
                      aria-label={t('data.constraint.limit')}
                      value={r.rhs}
                      placeholder={t('data.constraint.limitPlaceholder')}
                      onInput={(e) => setRow(i, { rhs: e.currentTarget.value })}
                    />
                    <button
                      type="button"
                      class={
                        'text-text-muted inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-sm ' +
                        'hover:bg-bg-subtle hover:text-danger-text ' +
                        FOCUS_RING
                      }
                      aria-label={t('data.constraint.remove')}
                      title={t('data.constraint.remove')}
                      onClick={() => setRows((rs) => rs.filter((_, j) => j !== i))}
                    >
                      <EditorIcon name="trash" size="sm" />
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>

          <button
            type="button"
            class={
              'border-border bg-surface text-text hover:bg-bg-subtle h-6 w-full rounded-sm border px-2 text-xs ' +
              'disabled:pointer-events-none disabled:opacity-40 ' +
              FOCUS_RING
            }
            disabled={!canSolve}
            onClick={solveNow}
          >
            {t('data.solver.solve')}
          </button>

          {error !== '' && <div class="text-danger-text text-2xs pt-2">{error}</div>}

          {result && (
            <div class="border-border mt-2 border-t pt-2">
              <div class="text-text text-2xs">
                {result.res.feasible ? t('data.solver.feasible') : t('data.solver.infeasible')}
              </div>
              <div class="text-text pt-0.5 text-xs font-medium">
                {target.trim().toUpperCase()} ={' '}
                {result.res.objective !== null ? formatCellValue(result.res.objective) : '—'}
              </div>
              <ul class="mt-1 max-h-24 overflow-auto">
                {result.cells.map((ref, i) => (
                  <li key={ref} class="sytue-meta-dense flex justify-between">
                    <span>{ref}</span>
                    <span class="text-text tabular-nums">
                      {formatCellValue(result.res.values[i] ?? 0)}
                    </span>
                  </li>
                ))}
              </ul>
              {!result.res.feasible &&
                result.res.maxViolation > 0 &&
                Number.isFinite(result.res.maxViolation) && (
                  <div class="sytue-meta-dense pt-0.5">
                    {t('data.solver.violation', {
                      max: formatCellValue(result.res.maxViolation),
                      evaluations: result.res.iterations,
                    })}
                  </div>
                )}
              <div class="flex gap-1 pt-2">
                <button
                  type="button"
                  class={
                    'border-border bg-surface text-text hover:bg-bg-subtle h-6 flex-1 rounded-sm border px-2 text-xs font-medium ' +
                    FOCUS_RING
                  }
                  onClick={applyResult}
                >
                  {t('action.apply')}
                </button>
                <button
                  type="button"
                  class={
                    'border-border bg-surface text-text hover:bg-bg-subtle h-6 flex-1 rounded-sm border px-2 text-xs ' +
                    FOCUS_RING
                  }
                  onClick={() => {
                    setResult(null);
                    setOpen(false);
                  }}
                >
                  {t('action.cancel')}
                </button>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

/* ---------- Tabla de datos (Análisis Y-si): análisis de sensibilidad ---------- */

function DataTableMenu(): JSX.Element {
  const bloqueado = useBloqueado('datatable');
  const t = useT();
  const { open, setOpen, rootRef } = usePopover();
  const [range, setRange] = useState('');
  const [rowInput, setRowInput] = useState('');
  const [colInput, setColInput] = useState('');

  const rr = parseRangeRef(range);
  const bigEnough = rr !== null && rr.c2 > rr.c1 && rr.r2 > rr.r1;
  const rowOk = rowInput.trim() === '' || isSingleCell(rowInput);
  const colOk = colInput.trim() === '' || isSingleCell(colInput);
  const hasInput = isSingleCell(rowInput) || isSingleCell(colInput);
  const canApply = bigEnough && rowOk && colOk && hasInput;

  const apply = (): void => {
    if (!canApply) return;
    dispatchSheetCommand(
      'datatable',
      JSON.stringify({
        range: range.trim().toUpperCase(),
        rowInput: rowInput.trim() !== '' ? rowInput.trim().toUpperCase() : undefined,
        colInput: colInput.trim() !== '' ? colInput.trim().toUpperCase() : undefined,
      }),
    );
    setOpen(false);
  };

  return (
    <div ref={rootRef} class="relative">
      <button
        type="button"
        class={
          'disabled:pointer-events-none disabled:opacity-40 ' +
          'text-text-muted inline-flex h-7 w-7 items-center justify-center rounded-sm ' +
          'hover:bg-bg-subtle hover:text-text aria-expanded:bg-selected aria-expanded:text-text ' +
          FOCUS_RING
        }
        aria-label={t('data.dataTable')}
        title={t('data.dataTable.hint')}
        aria-haspopup="true"
        aria-expanded={open}
        disabled={bloqueado}
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => {
          if (!open) {
            setRange(readSheetState()?.selection ?? '');
            setRowInput('');
            setColInput('');
          }
          setOpen(!open);
        }}
      >
        <EditorIcon name="datatable" />
      </button>
      {open && (
        <div
          role="group"
          aria-label={t('data.dataTable')}
          class={`${POPOVER_SHELL} left-0 w-72 p-2`}
        >
          <div class="text-text pb-1 text-xs font-medium">{t('data.dataTable')}</div>
          <div class="sytue-meta-dense pb-1.5">{t('data.dataTable.explain')}</div>
          <label class="block pb-1.5">
            <span class="sytue-meta-dense mb-0.5 block">{t('data.dataTable.range')}</span>
            <input
              class={inputCls}
              value={range}
              placeholder="A1:D5"
              onInput={(e) => setRange(e.currentTarget.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  apply();
                }
              }}
            />
          </label>
          <label class="block pb-1.5">
            <span class="sytue-meta-dense mb-0.5 block">{t('data.dataTable.rowInput')}</span>
            <input
              class={inputCls}
              value={rowInput}
              placeholder={t('data.dataTable.optionalCell', { example: 'B1' })}
              onInput={(e) => setRowInput(e.currentTarget.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  apply();
                }
              }}
            />
          </label>
          <label class="block pb-2">
            <span class="sytue-meta-dense mb-0.5 block">{t('data.dataTable.colInput')}</span>
            <input
              class={inputCls}
              value={colInput}
              placeholder={t('data.dataTable.optionalCell', { example: 'B2' })}
              onInput={(e) => setColInput(e.currentTarget.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  apply();
                }
              }}
            />
          </label>
          {!rowOk || !colOk ? (
            <div class="text-danger-text text-2xs pb-1.5">{t('data.dataTable.singleCell')}</div>
          ) : null}
          <button
            type="button"
            class={
              'border-border bg-surface text-text hover:bg-bg-subtle h-6 w-full rounded-sm border px-2 text-xs ' +
              'disabled:pointer-events-none disabled:opacity-40 ' +
              FOCUS_RING
            }
            disabled={!canApply}
            onClick={apply}
          >
            {t('data.dataTable.fill')}
          </button>
        </div>
      )}
    </div>
  );
}

/* ---------- Administrador de escenarios (Análisis Y-si): crear/aplicar/borrar + informe ---------- */

/** Divide un texto de refs («B6, B7  B8») en celdas individuales en MAYÚSCULAS. */
function splitRefs(text: string): string[] {
  return text
    .split(/[\s,;]+/)
    .map((r) => r.trim().toUpperCase())
    .filter((r) => r !== '');
}

function ScenarioMenu(): JSX.Element {
  const bloqueado = useBloqueado('scenadd');
  const t = useT();
  const { open, setOpen, rootRef } = usePopover();
  const st = readSheetState();
  const scenarios = st?.scenarios ?? [];

  const [name, setName] = useState('');
  const [range, setRange] = useState('');
  const [results, setResults] = useState('');
  const [target, setTarget] = useState('');

  const rr = parseRangeRef(range);
  const cellCount = rr ? (rr.c2 - rr.c1 + 1) * (rr.r2 - rr.r1 + 1) : 0;
  const nameTaken = scenarios.some((x) => x.name === name.trim());
  const tooMany = rr !== null && cellCount > SCENARIO_MAX_CELLS;
  const canCreate = name.trim() !== '' && rr !== null && !tooMany && !nameTaken;
  const canReport = scenarios.length > 0 && parseRangeRef(target) !== null;

  const create = (): void => {
    if (!canCreate) return;
    dispatchSheetCommand(
      'scenadd',
      JSON.stringify({ name: name.trim(), range: range.trim().toUpperCase() }),
    );
    setName('');
  };
  const makeReport = (): void => {
    if (!canReport) return;
    dispatchSheetCommand(
      'scenreport',
      JSON.stringify({ target: target.trim().toUpperCase(), results: splitRefs(results) }),
    );
    setOpen(false);
  };

  const btnCls =
    'border-border bg-surface text-text hover:bg-bg-subtle h-6 w-full rounded-sm border px-2 text-xs ' +
    'disabled:pointer-events-none disabled:opacity-40 ' +
    FOCUS_RING;

  return (
    <div ref={rootRef} class="relative">
      <button
        type="button"
        class={
          'disabled:pointer-events-none disabled:opacity-40 ' +
          'text-text-muted inline-flex h-7 w-7 items-center justify-center rounded-sm ' +
          'hover:bg-bg-subtle hover:text-text aria-expanded:bg-selected aria-expanded:text-text ' +
          FOCUS_RING
        }
        aria-label={t('data.scenarios')}
        title={t('data.scenarios.hint')}
        aria-haspopup="true"
        aria-expanded={open}
        disabled={bloqueado}
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => {
          if (!open) {
            // Prellena las celdas cambiantes con la selección y el destino del informe a su derecha.
            const sel = readSheetState()?.selection ?? '';
            setRange(sel);
            const r = parseRangeRef(sel);
            setTarget(r ? `${colLetter(Math.min(r.c2 + 2, 25))}${r.r1 + 1}` : '');
            setName('');
            setResults('');
          }
          setOpen(!open);
        }}
      >
        <EditorIcon name="scenarios" />
      </button>
      {open && (
        <div
          role="group"
          aria-label={t('data.scenarios')}
          class={`${POPOVER_SHELL} left-0 w-72 p-2`}
        >
          <div class="text-text pb-1 text-xs font-medium">{t('data.scenarios')}</div>

          {scenarios.length > 0 ? (
            <ul class="border-border mb-2 max-h-40 overflow-auto border-b pb-2">
              {scenarios.map((sc) => (
                <li key={sc.name} class="text-text flex items-center gap-1 py-0.5 text-xs">
                  <span class="min-w-0 flex-1 truncate" title={sc.name}>
                    <span class="font-medium">{sc.name}</span>
                    <span class="text-text-subtle"> · {Object.keys(sc.values).length}</span>
                  </span>
                  <button
                    type="button"
                    class={
                      'border-border bg-surface text-text hover:bg-bg-subtle text-2xs h-5 shrink-0 rounded-sm border px-1.5 ' +
                      FOCUS_RING
                    }
                    title={t('data.scenario.apply')}
                    onClick={() => dispatchSheetCommand('scenapply', sc.name)}
                  >
                    {t('action.apply')}
                  </button>
                  <button
                    type="button"
                    class={
                      'text-text-muted inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-sm ' +
                      'hover:bg-bg-subtle hover:text-text ' +
                      FOCUS_RING
                    }
                    aria-label={t('data.scenario.update', { name: sc.name })}
                    title={t('data.scenario.refresh')}
                    onClick={() => dispatchSheetCommand('scenupdate', sc.name)}
                  >
                    <EditorIcon name="history" size="sm" />
                  </button>
                  <button
                    type="button"
                    class={
                      'text-text-muted inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-sm ' +
                      'hover:bg-bg-subtle hover:text-danger-text ' +
                      FOCUS_RING
                    }
                    aria-label={t('data.scenario.deleteNamed', { name: sc.name })}
                    title={t('data.scenario.delete')}
                    onClick={() => dispatchSheetCommand('scendel', sc.name)}
                  >
                    <EditorIcon name="trash" size="sm" />
                  </button>
                </li>
              ))}
            </ul>
          ) : (
            <div class="sytue-meta-dense pb-2">{t('data.scenario.empty')}</div>
          )}

          <div class="sytue-overline text-text-subtle pb-1">{t('data.scenario.new')}</div>
          <label class="block pb-1.5">
            <span class="sytue-meta-dense mb-0.5 block">{t('common.name')}</span>
            <input
              class={inputCls}
              value={name}
              placeholder={t('data.scenario.optimistic')}
              onInput={(e) => setName(e.currentTarget.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  create();
                }
              }}
            />
          </label>
          <label class="block pb-1">
            <span class="sytue-meta-dense mb-0.5 block">{t('data.scenario.changing')}</span>
            <input
              class={inputCls}
              value={range}
              placeholder="B3:B5"
              onInput={(e) => setRange(e.currentTarget.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  create();
                }
              }}
            />
          </label>
          {tooMany && (
            <div class="text-danger-text text-2xs pb-1">
              {t('data.scenario.tooMany', { max: SCENARIO_MAX_CELLS })}
            </div>
          )}
          {nameTaken && name.trim() !== '' && (
            <div class="text-danger-text text-2xs pb-1">{t('data.scenario.duplicate')}</div>
          )}
          <button type="button" class={btnCls + ' mt-1'} disabled={!canCreate} onClick={create}>
            {t('data.scenario.capture')}
          </button>

          <div class="border-border my-2 border-t" />

          <div class="sytue-overline text-text-subtle pb-1">{t('data.summaryReport')}</div>
          <label class="block pb-1.5">
            <span class="sytue-meta-dense mb-0.5 block">{t('data.scenario.resultCells')}</span>
            <input
              class={inputCls}
              value={results}
              placeholder="B6, B7"
              onInput={(e) => setResults(e.currentTarget.value)}
            />
          </label>
          <label class="block pb-1.5">
            <span class="sytue-meta-dense mb-0.5 block">{t('data.targetTopLeft')}</span>
            <input
              class={inputCls}
              value={target}
              placeholder="F1"
              onInput={(e) => setTarget(e.currentTarget.value)}
            />
          </label>
          <button type="button" class={btnCls} disabled={!canReport} onClick={makeReport}>
            {t('data.scenario.createReport')}
          </button>
        </div>
      )}
    </div>
  );
}

/* ---------- Tablas con formato (popover: crear o gestionar la tabla del ancla) ---------- */

/** Letra de columna A..Z de un índice 0-based (la rejilla es 26 columnas). */
function colLetter(c: number): string {
  return String.fromCharCode(65 + Math.min(Math.max(c, 0), 25));
}

/** Fila de muestras de estilo (franja de cabecera sobre banda; sin degradados). */
function StylePicker({
  value,
  onPick,
}: {
  value: string | undefined;
  onPick: (id: string) => void;
}): JSX.Element {
  const t = useT();
  return (
    <div class="pb-2">
      <div class="sytue-meta-dense mb-1">{t('data.table.style')}</div>
      <div class="flex items-center gap-1" role="group" aria-label={t('data.table.style')}>
        {TABLE_STYLES.map((s) => (
          <button
            key={s.id}
            type="button"
            class={`relative h-6 w-6 overflow-hidden rounded-[3px] border ${
              value === s.id ? 'border-text' : 'border-border'
            } ${FOCUS_RING}`}
            aria-label={t('common.styleN', { label: t(s.labelKey) })}
            title={t(s.labelKey)}
            aria-pressed={value === s.id}
            onClick={() => onPick(s.id)}
          >
            <span class="absolute inset-x-0 top-0 h-2" style={{ backgroundColor: s.headerBg }} />
            <span class="absolute inset-x-0 top-2 bottom-0" style={{ backgroundColor: s.band }} />
          </button>
        ))}
      </div>
    </div>
  );
}

/** Panel de gestión de la tabla que cubre el ancla: estilo, fila de totales
 *  (con la función de cada columna) y quitar. */
function TableManage({ table }: { table: SheetTable }): JSX.Element {
  const t = useT();
  const rr = parseRangeRef(table.range);
  const width = rr ? rr.c2 - rr.c1 + 1 : 0;
  const totalsOn = table.totalsRow === true;
  // Nombres de columna (valores mostrados de la cabecera); «Columna N» si no hay cabecera.
  const headerRow =
    rr && table.headers !== false
      ? (readSheetGrid(`${colLetter(rr.c1)}${rr.r1 + 1}:${colLetter(rr.c2)}${rr.r1 + 1}`)?.[0] ??
        [])
      : [];
  const colName = (i: number): string => {
    const nm = headerRow[i]?.trim();
    return nm && nm !== '' ? nm : t('common.columnN', { n: i + 1 });
  };

  return (
    <>
      <div class="flex items-baseline justify-between gap-2 pb-1.5">
        <span class="text-text min-w-0 flex-1 truncate text-xs font-medium" title={table.name}>
          {table.name}
        </span>
        <span class="sytue-meta-dense shrink-0 tabular-nums">{table.range}</span>
      </div>
      <StylePicker value={table.style} onPick={(id) => dispatchSheetCommand('tablestyle', id)} />
      <label class="text-text flex cursor-pointer items-center gap-1.5 pb-1.5 text-xs">
        <Checkbox
          checked={totalsOn}
          class="accent-primary"
          onChange={() => dispatchSheetCommand('tabletotals')}
        />
        {t('data.table.totalsRow')}
      </label>
      {totalsOn && width > 0 && (
        <div class="border-border mb-2 max-h-44 overflow-auto border-t pt-1.5">
          <div class="sytue-meta-dense mb-1">{t('data.columnRole')}</div>
          <div class="flex flex-col gap-1">
            {Array.from({ length: width }, (_, i) => (
              <label key={i} class="flex items-center gap-1.5">
                <span class="text-text min-w-0 flex-1 truncate text-xs" title={colName(i)}>
                  {colName(i)}
                </span>
                <RibbonSelect
                  label={t('data.totalOf', { col: colName(i) })}
                  value={table.totals?.[String(i)] ?? 'none'}
                  width={128}
                  options={TABLE_TOTAL_FNS.map((o) => ({ value: o.value, label: t(o.labelKey) }))}
                  onSelect={(fn) =>
                    dispatchSheetCommand(
                      'tabletotalfn',
                      JSON.stringify({ name: table.name, col: i, fn }),
                    )
                  }
                />
              </label>
            ))}
          </div>
        </div>
      )}
      <button
        type="button"
        class={
          'border-border bg-surface text-text h-6 w-full rounded-sm border px-2 text-xs ' +
          'hover:bg-bg-subtle hover:text-danger-text ' +
          FOCUS_RING
        }
        onClick={() => dispatchSheetCommand('tabledel')}
      >
        {t('data.table.remove')}
      </button>
    </>
  );
}

function TableMenu(): JSX.Element {
  const bloqueado = useBloqueado('tableadd');
  const t = useT();
  const { open, setOpen, rootRef } = usePopover();
  const table = readSheetState()?.table ?? null;

  // Formulario de CREACIÓN (solo cuando el ancla no está dentro de una tabla).
  const [range, setRange] = useState('A1:D10');
  const [headers, setHeaders] = useState(true);
  const [styleId, setStyleId] = useState<string>(TABLE_STYLES[0]!.id);
  const validRange = parseRangeRef(range) !== null;

  const create = (): void => {
    if (!validRange) return;
    dispatchSheetCommand(
      'tableadd',
      JSON.stringify({ range: range.trim().toUpperCase(), headers, style: styleId }),
    );
    setOpen(false);
  };

  return (
    <div ref={rootRef} class="relative">
      <button
        type="button"
        class={
          'disabled:pointer-events-none disabled:opacity-40 ' +
          'text-text-muted inline-flex h-7 w-7 items-center justify-center rounded-sm ' +
          'hover:bg-bg-subtle hover:text-text aria-pressed:bg-selected aria-pressed:text-text ' +
          FOCUS_RING
        }
        aria-label={t('data.table.format')}
        title={t('data.table.format')}
        aria-haspopup="true"
        aria-expanded={open}
        disabled={bloqueado}
        aria-pressed={table !== null}
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => {
          if (!open && !table) setRange(readSheetState()?.selection ?? 'A1:D10');
          setOpen(!open);
        }}
      >
        <EditorIcon name="table" />
      </button>
      {open && (
        <div
          role="group"
          aria-label={t('data.table.formatted')}
          class={`${POPOVER_SHELL} left-0 w-64 p-2`}
        >
          {table ? (
            <TableManage table={table} />
          ) : (
            <>
              <div class="text-text pb-1 text-xs font-medium">{t('data.table.format')}</div>
              <label class="block pb-1.5">
                <span class="sytue-meta-dense mb-0.5 block">{t('data.range')}</span>
                <input
                  class={
                    'border-border bg-surface text-text h-6 w-full rounded-sm border px-1.5 text-xs ' +
                    FOCUS_RING
                  }
                  value={range}
                  onInput={(e) => setRange(e.currentTarget.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') {
                      e.preventDefault();
                      create();
                    }
                  }}
                />
              </label>
              <StylePicker value={styleId} onPick={setStyleId} />
              <label class="text-text flex cursor-pointer items-center gap-1.5 pb-2 text-xs">
                <Checkbox
                  checked={headers}
                  class="accent-primary"
                  onChange={(e) => setHeaders(e.currentTarget.checked)}
                />
                {t('data.table.hasHeaders')}
              </label>
              <button
                type="button"
                class={
                  'border-border bg-surface text-text hover:bg-bg-subtle h-6 w-full rounded-sm border px-2 text-xs ' +
                  'disabled:pointer-events-none disabled:opacity-40 ' +
                  FOCUS_RING
                }
                disabled={!validRange}
                onClick={create}
              >
                {t('data.table.create')}
              </button>
            </>
          )}
        </div>
      )}
    </div>
  );
}

/* ---------- Protección de hoja y celda (paridad Excel) ---------- */

/** Conmuta el bloqueo de la celda ancla. El bloqueo solo surte efecto con la hoja
 *  protegida; se deshabilita cuando ya lo está (Excel tampoco deja cambiar el
 *  bloqueo de una celda con la hoja protegida). Reusa el comando `style`. */
function LockCellButton(): JSX.Element {
  const t = useT();
  const st = readSheetState();
  const style = st?.style ?? {};
  const locked = style.locked !== false; // ausente/true = bloqueada (criterio Excel)
  return (
    <RibbonButton
      commandId="style"
      icon="lock"
      label={locked ? t('data.cell.unlock') : t('data.cell.lock')}
      active={locked}
      disabled={st?.protectedSheet ?? false}
      onAction={() => dispatchSheetCommand('style', JSON.stringify({ locked: !locked }))}
    />
  );
}

/** Permisos granulares del cuadro «Proteger hoja» (mismos que Excel, en su orden).
 *  Desmarcados por defecto: con la hoja protegida la acción queda vetada. */
const PROTECT_PERMS: { key: SheetPermission; labelKey: DocsKey; hintKey?: DocsKey }[] = [
  { key: 'insertRows', labelKey: 'table.rowAbove' },
  { key: 'deleteRows', labelKey: 'table.deleteRows' },
  { key: 'insertCols', labelKey: 'data.perm.insertCols' },
  { key: 'deleteCols', labelKey: 'data.perm.deleteCols' },
  { key: 'sort', labelKey: 'data.perm.sort' },
  { key: 'autoFilter', labelKey: 'data.perm.autoFilter' },
  {
    key: 'objects',
    labelKey: 'data.perm.objects',
    hintKey: 'data.perm.objects.hint',
  },
  { key: 'scenarios', labelKey: 'data.perm.scenarios' },
];

/** Popover de protección de HOJA: ocultar la fórmula de la celda + proteger/
 *  desproteger la hoja con contraseña opcional y permisos (seleccionar/formatear
 *  + los granulares de PROTECT_PERMS). */
/** Una concesión pendiente del cuadro «Proteger hoja»: este rango, esta persona. */
interface ConcesionRango {
  ref: string;
  sub: string;
  email: string;
}

/**
 * Rangos editables por persona dentro de una hoja protegida. La persona se pone por su
 * correo y la traduce el servidor a su `sub` (GET /api/people?resolve=): el cliente no
 * elige identificadores. Se definen al proteger; cambiarlos es desproteger y volver a
 * proteger (propietario o contraseña), como en Excel.
 */
function RangosEditablesCampo({
  value,
  onChange,
}: {
  value: readonly ConcesionRango[];
  onChange: (v: ConcesionRango[]) => void;
}): JSX.Element {
  const t = useT();
  const [ref, setRef] = useState('');
  const [email, setEmail] = useState('');
  const [error, setError] = useState('');
  const anadir = async (): Promise<void> => {
    const rc = parseRangeRef(ref);
    if (rc === null) {
      setError(t('data.protect.rangeBad'));
      return;
    }
    try {
      const res = await fetch(`/api/people?resolve=${encodeURIComponent(email.trim())}`);
      const persona = res.ok ? ((await res.json()) as { sub?: string; email?: string }) : null;
      if (!persona?.sub) {
        setError(t('data.protect.rangeUnknown'));
        return;
      }
      onChange([
        ...value,
        {
          ref: ref.trim().toUpperCase().replace(/\$/g, ''),
          sub: persona.sub,
          email: persona.email ?? email,
        },
      ]);
      setRef('');
      setEmail('');
      setError('');
    } catch {
      setError(t('data.protect.rangeUnknown'));
    }
  };
  return (
    <>
      <div class="text-text pb-1 text-xs font-medium">{t('data.protect.ranges')}</div>
      {value.map((g, i) => (
        <div key={`${g.ref}-${g.sub}`} class="text-text flex items-center gap-1 pb-0.5 text-xs">
          <span class="min-w-0 flex-1 truncate">
            {g.ref} · {g.email}
          </span>
          <button
            type="button"
            class={'text-text-muted hover:text-text px-1 ' + FOCUS_RING}
            aria-label={t('action.removeItem')}
            onClick={() => onChange(value.filter((_, j) => j !== i))}
          >
            ×
          </button>
        </div>
      ))}
      <div class="flex gap-1 pb-1">
        <input
          class={inputCls + ' w-16 shrink-0'}
          placeholder="B2:D10"
          aria-label={t('data.protect.rangeRef')}
          value={ref}
          onInput={(e) => setRef(e.currentTarget.value)}
        />
        <input
          type="email"
          class={inputCls}
          placeholder={t('data.protect.rangePerson')}
          aria-label={t('data.protect.rangePerson')}
          value={email}
          onInput={(e) => setEmail(e.currentTarget.value)}
        />
      </div>
      <button
        type="button"
        class={
          'border-border bg-surface text-text hover:bg-bg-subtle mb-1 h-6 w-full rounded-sm border px-2 text-xs ' +
          FOCUS_RING
        }
        disabled={ref.trim() === '' || email.trim() === ''}
        onClick={() => void anadir()}
      >
        {t('data.protect.rangeAdd')}
      </button>
      {error !== '' && (
        <div role="alert" class="text-danger-text pb-1 text-xs">
          {error}
        </div>
      )}
      <div class="sytue-meta-dense pb-2">{t('data.protect.rangesHint')}</div>
    </>
  );
}

function ProtectSheetMenu(): JSX.Element {
  const bloqueado = useBloqueado('protect');
  const t = useT();
  const { open, setOpen, rootRef } = usePopover();
  const st = readSheetState();
  const protectedSheet = st?.protectedSheet ?? false;
  const hasPassword = st?.protectHasPassword ?? false;
  const hidden = (st?.style ?? {}).hidden === true;

  const [pw, setPw] = useState('');
  const [selectCells, setSelectCells] = useState(true);
  const [formatCells, setFormatCells] = useState(false);
  const [perms, setPerms] = useState<Partial<Record<SheetPermission, boolean>>>({});
  const [rangos, setRangos] = useState<ConcesionRango[]>([]);

  const doProtect = (): void => {
    const ranges = [...new Set(rangos.map((g) => g.ref))].map((ref) => ({
      ref,
      users: rangos.filter((g) => g.ref === ref).map((g) => g.sub),
    }));
    dispatchSheetCommand(
      'protect',
      JSON.stringify({ password: pw, selectCells, formatCells, ...perms, ranges }),
    );
    setPw('');
    setRangos([]);
    setOpen(false);
  };
  // No cerramos al desproteger: si la contraseña falla, un toast avisa y se reintenta;
  // si acierta, la hoja pasa a no-protegida y el panel muestra el formulario de proteger.
  const doUnprotect = (): void => {
    dispatchSheetCommand('unprotect', pw);
    setPw('');
  };

  return (
    <div ref={rootRef} class="relative">
      <button
        type="button"
        class={
          'disabled:pointer-events-none disabled:opacity-40 ' +
          'text-text-muted inline-flex h-7 w-7 items-center justify-center rounded-sm ' +
          'hover:bg-bg-subtle hover:text-text aria-pressed:bg-selected aria-pressed:text-text ' +
          FOCUS_RING
        }
        aria-label={t('data.protectSheet')}
        title={t('data.protectSheet')}
        aria-haspopup="true"
        aria-expanded={open}
        disabled={bloqueado}
        aria-pressed={protectedSheet}
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => setOpen(!open)}
      >
        <EditorIcon name="shield" />
      </button>
      {open && (
        <div
          role="group"
          aria-label={t('data.protectSheet')}
          class={`${POPOVER_SHELL} left-0 w-64 p-2`}
        >
          {/* Celda ancla: ocultar su fórmula (solo efectivo con la hoja protegida). */}
          <label class="text-text flex cursor-pointer items-center gap-1.5 pb-2 text-xs">
            <Checkbox
              checked={hidden}
              disabled={protectedSheet}
              class="accent-primary"
              onChange={(e) =>
                dispatchSheetCommand('style', JSON.stringify({ hidden: e.currentTarget.checked }))
              }
            />
            {t('data.protect.hideFormula')}
          </label>

          <div class="border-border mb-2 border-t" />

          {protectedSheet ? (
            <>
              <div class="text-text pb-1.5 text-xs font-medium">
                {t('data.protect.sheetProtected')}
              </div>
              {(st?.protectRanges.length ?? 0) > 0 && (
                <div class="sytue-meta-dense pb-1.5">
                  {t('data.protect.ranges')}: {st?.protectRanges.join(', ')}
                </div>
              )}
              {hasPassword && (
                <label class="block pb-1.5">
                  <span class="sytue-meta-dense mb-0.5 block">{t('data.password')}</span>
                  <input
                    type="password"
                    class={inputCls}
                    value={pw}
                    onInput={(e) => setPw(e.currentTarget.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        e.preventDefault();
                        doUnprotect();
                      }
                    }}
                  />
                </label>
              )}
              <button
                type="button"
                class={
                  'border-border bg-surface text-text hover:bg-bg-subtle h-6 w-full rounded-sm border px-2 text-xs ' +
                  FOCUS_RING
                }
                onClick={doUnprotect}
              >
                {t('data.protect.unprotectSheet')}
              </button>
            </>
          ) : (
            <>
              <div class="text-text pb-1.5 text-xs font-medium">{t('data.protectSheet')}</div>
              <label class="block pb-1.5">
                <span class="sytue-meta-dense mb-0.5 block">{t('data.passwordOptional')}</span>
                <input
                  type="password"
                  class={inputCls}
                  value={pw}
                  onInput={(e) => setPw(e.currentTarget.value)}
                />
              </label>
              <label class="text-text flex cursor-pointer items-center gap-1.5 pb-1 text-xs">
                <Checkbox
                  checked={selectCells}
                  class="accent-primary"
                  onChange={(e) => setSelectCells(e.currentTarget.checked)}
                />
                {t('data.perm.selectCells')}
              </label>
              <label class="text-text flex cursor-pointer items-center gap-1.5 pb-2 text-xs">
                <Checkbox
                  checked={formatCells}
                  class="accent-primary"
                  onChange={(e) => setFormatCells(e.currentTarget.checked)}
                />
                {t('data.perm.formatCells')}
              </label>
              {/* Permisos granulares (Excel): desmarcados = la acción queda vetada.
                  Lista acotada en alto para no desbordar el popover. */}
              {/* Cabecera de sección del popover: la misma receta que las otras diez del
                  fichero. Iba en `text-2xs subtle`, o sea, más pequeña y más floja que las
                  opciones que encabeza. */}
              <div class="text-text pb-1 text-xs font-medium">{t('data.validation.allowAlso')}</div>
              <div class="border-border mb-2 max-h-32 overflow-auto rounded-sm border p-1">
                {PROTECT_PERMS.map((p) => (
                  <label
                    key={p.key}
                    title={p.hintKey ? t(p.hintKey) : undefined}
                    class="text-text flex cursor-pointer items-center gap-1.5 py-0.5 text-xs"
                  >
                    <Checkbox
                      checked={perms[p.key] === true}
                      class="accent-primary"
                      onChange={(e) => setPerms({ ...perms, [p.key]: e.currentTarget.checked })}
                    />
                    {t(p.labelKey)}
                  </label>
                ))}
              </div>
              <RangosEditablesCampo value={rangos} onChange={setRangos} />
              {/* La contraseña viaja aparte y el servidor guarda solo su hash (state/proteccion.ts). */}
              <button
                type="button"
                class={
                  'border-border bg-surface text-text hover:bg-bg-subtle h-6 w-full rounded-sm border px-2 text-xs ' +
                  FOCUS_RING
                }
                onClick={doProtect}
              >
                {t('data.protectSheet')}
              </button>
            </>
          )}
        </div>
      )}
    </div>
  );
}

/** Popover de protección de ESTRUCTURA del libro: proteger/desproteger con
 *  contraseña opcional. Con el libro protegido no se pueden añadir/borrar/
 *  renombrar/duplicar/reordenar hojas (el veto vive en el lienzo). */
function ProtectWorkbookMenu(): JSX.Element {
  const bloqueado = useBloqueado('protectbook');
  const t = useT();
  const { open, setOpen, rootRef } = usePopover();
  const st = readSheetState();
  const protectedWorkbook = st?.protectedWorkbook ?? false;
  const hasPassword = st?.protectWorkbookHasPassword ?? false;

  const [pw, setPw] = useState('');

  const doProtect = (): void => {
    dispatchSheetCommand('protectbook', JSON.stringify({ password: pw }));
    setPw('');
    setOpen(false);
  };
  // No cerramos al desproteger: si la contraseña falla, un toast avisa y se reintenta.
  const doUnprotect = (): void => {
    dispatchSheetCommand('unprotectbook', pw);
    setPw('');
  };

  return (
    <div ref={rootRef} class="relative">
      <button
        type="button"
        class={
          'disabled:pointer-events-none disabled:opacity-40 ' +
          'text-text-muted inline-flex h-7 w-7 items-center justify-center rounded-sm ' +
          'hover:bg-bg-subtle hover:text-text aria-pressed:bg-selected aria-pressed:text-text ' +
          FOCUS_RING
        }
        aria-label={t('data.protectBook')}
        title={t('data.protectBook')}
        aria-haspopup="true"
        aria-expanded={open}
        disabled={bloqueado}
        aria-pressed={protectedWorkbook}
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => setOpen(!open)}
      >
        <EditorIcon name="book" />
      </button>
      {open && (
        <div
          role="group"
          aria-label={t('data.protectBook')}
          class={`${POPOVER_SHELL} left-0 w-64 p-2`}
        >
          {protectedWorkbook ? (
            <>
              <div class="text-text pb-1.5 text-xs font-medium">
                {t('data.protect.bookProtected')}
              </div>
              <div class="sytue-meta-dense pb-1.5">{t('data.protect.bookLocked')}</div>
              {hasPassword && (
                <label class="block pb-1.5">
                  <span class="sytue-meta-dense mb-0.5 block">{t('data.password')}</span>
                  <input
                    type="password"
                    class={inputCls}
                    value={pw}
                    onInput={(e) => setPw(e.currentTarget.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        e.preventDefault();
                        doUnprotect();
                      }
                    }}
                  />
                </label>
              )}
              <button
                type="button"
                class={
                  'border-border bg-surface text-text hover:bg-bg-subtle h-6 w-full rounded-sm border px-2 text-xs ' +
                  FOCUS_RING
                }
                onClick={doUnprotect}
              >
                {t('data.protect.unprotectBook')}
              </button>
            </>
          ) : (
            <>
              <div class="text-text pb-1.5 text-xs font-medium">{t('data.protectBook')}</div>
              <div class="sytue-meta-dense pb-1.5">{t('data.protect.lockStructure')}</div>
              <label class="block pb-2">
                <span class="sytue-meta-dense mb-0.5 block">{t('data.passwordOptional')}</span>
                <input
                  type="password"
                  class={inputCls}
                  value={pw}
                  onInput={(e) => setPw(e.currentTarget.value)}
                />
              </label>
              {/* ponytail: la contraseña es un cotejo EN CLARO en el cliente, NO es cifrado
                  (como Proteger libro de Excel); solo evita cambios accidentales de estructura. */}
              <button
                type="button"
                class={
                  'border-border bg-surface text-text hover:bg-bg-subtle h-6 w-full rounded-sm border px-2 text-xs ' +
                  FOCUS_RING
                }
                onClick={doProtect}
              >
                {t('data.protectBook')}
              </button>
            </>
          )}
        </div>
      )}
    </div>
  );
}
