/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Gestor «Pasos aplicados» (estilo Power Query) de una tabla de datos externos:
 * edita el PIPELINE de transformación (filtrar, elegir/quitar/renombrar/reordenar
 * columnas, cambiar tipo, ordenar, agrupar, añadir columna, reemplazar, quitar
 * duplicados, mantener/quitar N filas) con VISTA PREVIA del resultado tras cada
 * paso. Al aplicar, re-materializa la SheetTable con los datos crudos + los pasos
 * (el comando `extdata` reaplica el pipeline y persiste `transform`).
 *
 * El MOTOR (sheets/querytransform.ts) es puro y testeado; aquí solo hay estado de
 * UI, una ejecución de la consulta al abrir (para previsualizar sin tocar la hoja)
 * y el despacho de `extdata` al aplicar.
 */
import type { JSX } from 'preact';
import { useT } from '../context';
import type { DocsKey } from '../../i18n';
import { useEffect, useMemo, useState } from 'preact/hooks';
import { Button, FieldGroup, MenuItem, Modal, Select, Spinner, Checkbox } from '@sytue/ui';
import { EditorIcon } from '../icons';
import { dispatchSheetCommand, readSheetGrid } from '../sheets/SheetCanvas';
import type { ExternalTableState } from '../sheets/SheetCanvas';
import {
  buildExecuteBody,
  cellText,
  errorMessageFrom,
  gridToQueryTable,
  parseExecuteResult,
} from '../dataquery';
import {
  describeStep,
  runPipeline,
  type QueryColumn,
  type QueryResolver,
  type QueryTable,
} from '../sheets/querytransform';
import type { QueryStep } from '../types';

/** Otra tabla externa de la hoja (para append/merge): nombre + sus columnas. */
interface OtherTable {
  name: string;
  columns: QueryColumn[];
}

const JSON_HEADERS = { 'content-type': 'application/json' } as const;
/** Filas mostradas en la vista previa (la hoja materializa como mucho 100). */
const PREVIEW_ROWS = 60;

/* ---------- Metadatos de los tipos de paso (menú «Añadir») y selects ---------- */

const STEP_KINDS: { kind: QueryStep['kind']; labelKey: DocsKey }[] = [
  { kind: 'filter', labelKey: 'query.step.filter' },
  { kind: 'select', labelKey: 'query.step.selectCols' },
  { kind: 'removeColumns', labelKey: 'query.step.removeCols' },
  { kind: 'rename', labelKey: 'query.step.rename' },
  { kind: 'reorder', labelKey: 'query.step.reorder' },
  { kind: 'changeType', labelKey: 'query.step.changeType' },
  { kind: 'sort', labelKey: 'query.step.sort' },
  { kind: 'groupBy', labelKey: 'common.groupBy' },
  { kind: 'addColumn', labelKey: 'query.step.addCol' },
  { kind: 'replace', labelKey: 'query.step.replace' },
  { kind: 'dedupe', labelKey: 'data.removeDuplicates' },
  { kind: 'keepRows', labelKey: 'query.step.keepRows' },
  { kind: 'removeRows', labelKey: 'query.step.dropRows' },
  { kind: 'unpivot', labelKey: 'query.step.unpivot' },
  { kind: 'pivot', labelKey: 'query.step.pivot' },
  { kind: 'append', labelKey: 'query.step.append' },
  { kind: 'merge', labelKey: 'query.step.merge' },
];

const FILTER_OP_OPTS: { value: string; labelKey: DocsKey }[] = [
  { value: 'eq', labelKey: 'query.op.eq' },
  { value: 'neq', labelKey: 'query.op.ne' },
  { value: 'contains', labelKey: 'query.op.contains' },
  { value: 'gt', labelKey: 'query.op.gt' },
  { value: 'lt', labelKey: 'query.op.lt' },
  { value: 'between', labelKey: 'query.op.between' },
  { value: 'empty', labelKey: 'query.op.empty' },
  { value: 'nonempty', labelKey: 'query.op.notEmpty' },
];
const TYPE_OPTS: { value: string; labelKey: DocsKey }[] = [
  { value: 'text', labelKey: 'query.type.text' },
  { value: 'number', labelKey: 'query.type.number' },
  { value: 'date', labelKey: 'query.type.date' },
  { value: 'boolean', labelKey: 'query.type.boolean' },
];
const AGG_OPTS: { value: string; labelKey: DocsKey }[] = [
  { value: 'sum', labelKey: 'query.agg.sum' },
  { value: 'count', labelKey: 'query.agg.count' },
  { value: 'avg', labelKey: 'query.agg.avg' },
  { value: 'min', labelKey: 'query.agg.min' },
  { value: 'max', labelKey: 'query.agg.max' },
  { value: 'first', labelKey: 'query.agg.first' },
];
const JOIN_OPTS: { value: string; labelKey: DocsKey }[] = [
  { value: 'inner', labelKey: 'query.join.inner' },
  { value: 'left', labelKey: 'query.join.left' },
  { value: 'right', labelKey: 'query.join.right' },
  { value: 'full', labelKey: 'query.join.full' },
  { value: 'antiLeft', labelKey: 'query.join.antiLeft' },
];
const DIR_OPTS: { value: string; labelKey: DocsKey }[] = [
  { value: 'asc', labelKey: 'query.sort.asc' },
  { value: 'desc', labelKey: 'query.sort.desc' },
];
const END_OPTS: { value: string; labelKey: DocsKey }[] = [
  { value: 'first', labelKey: 'query.rows.first' },
  { value: 'last', labelKey: 'query.rows.last' },
];

/** Paso por defecto al elegir un tipo en el menú «Añadir» (con la 1.ª columna y,
 *  para append/merge, la 1.ª otra tabla de la hoja). */
const defaultStep = (
  kind: QueryStep['kind'],
  cols: readonly QueryColumn[],
  others: readonly string[],
): QueryStep => {
  const c0 = cols[0]?.name ?? '';
  switch (kind) {
    case 'filter':
      return { kind, column: c0, op: 'eq', value: '' };
    case 'select':
    case 'removeColumns':
      return { kind, columns: [] };
    case 'reorder':
      return { kind, columns: cols.map((c) => c.name) };
    case 'rename':
      return { kind, column: c0, to: c0 };
    case 'changeType':
      return { kind, column: c0, type: 'text' };
    case 'sort':
      return { kind, by: [{ column: c0, dir: 'asc' }] };
    case 'groupBy':
      return { kind, columns: c0 ? [c0] : [], aggs: [{ column: '', fn: 'count', as: 'recuento' }] };
    case 'addColumn':
      return { kind, name: 'personalizada', expr: '' };
    case 'replace':
      return { kind, column: c0, find: '', replace: '' };
    case 'dedupe':
      return { kind };
    case 'keepRows':
      return { kind, end: 'first', count: 10 };
    case 'removeRows':
      return { kind, end: 'first', count: 1 };
    case 'unpivot':
      return { kind, columns: [], attribute: 'Atributo', value: 'Valor' };
    case 'pivot':
      return { kind, key: c0, value: cols[1]?.name ?? c0, fn: 'sum' };
    case 'append':
      return { kind, from: others[0] ?? '' };
    case 'merge':
      return {
        kind,
        from: others[0] ?? '',
        join: 'left',
        on: [{ left: c0, right: '' }],
        bring: [],
        prefix: '',
      };
  }
};

/** ¿El borrador tiene lo mínimo para guardarse? (el motor revalida al aplicar). */
const draftValid = (d: QueryStep): boolean => {
  switch (d.kind) {
    case 'select':
    case 'removeColumns':
    case 'reorder':
      return d.columns.length > 0;
    case 'rename':
      return d.to.trim() !== '';
    case 'addColumn':
      return d.name.trim() !== '';
    case 'sort':
      return d.by.length > 0;
    case 'groupBy':
      return d.columns.length > 0 && d.aggs.length > 0 && d.aggs.every((a) => a.as.trim() !== '');
    case 'unpivot':
      return d.columns.length > 0 && d.attribute.trim() !== '' && d.value.trim() !== '';
    case 'pivot':
      return d.key.trim() !== '' && d.value.trim() !== '' && d.key !== d.value;
    case 'append':
      return d.from.trim() !== '';
    case 'merge':
      return (
        d.from.trim() !== '' &&
        d.on.length > 0 &&
        d.on.every((p) => p.left !== '' && p.right !== '')
      );
    default:
      return true;
  }
};

/* ---------- Modal principal ---------- */

export function QueryStepsModal({
  open,
  onClose,
  tables,
  initialTable,
}: {
  open: boolean;
  onClose: () => void;
  tables: readonly ExternalTableState[];
  initialTable: string;
}): JSX.Element | null {
  const t = useT();
  const [tableName, setTableName] = useState('');
  const [raw, setRaw] = useState<QueryTable | null>(null);
  const [loading, setLoading] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [steps, setSteps] = useState<QueryStep[]>([]);
  /** Índice del paso cuyo resultado se previsualiza (-1 = datos de origen). */
  const [selected, setSelected] = useState(-1);
  /** Editor abierto: index=null añade al final; si no, edita ese paso. */
  const [editor, setEditor] = useState<{ index: number | null; draft: QueryStep } | null>(null);

  // Al abrir: elige la tabla (la del ancla si es válida) y carga sus datos.
  useEffect(() => {
    if (!open) return;
    const first = tables.find((t) => t.table === initialTable) ?? tables[0];
    if (first) void load(first.table);
    else {
      setRaw(null);
      setTableName('');
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const load = async (name: string): Promise<void> => {
    const tabla = tables.find((x) => x.table === name);
    if (!tabla) return;
    setTableName(name);
    setLoading(true);
    setLoadError(null);
    setRaw(null);
    setEditor(null);
    try {
      const res = await fetch('/api/dataquery', {
        method: 'POST',
        headers: JSON_HEADERS,
        body: JSON.stringify(buildExecuteBody(tabla.connectionId, tabla.spec)),
      });
      const data: unknown = await res.json().catch(() => null);
      if (!res.ok) throw new Error(errorMessageFrom(data, String(res.status)));
      const result = parseExecuteResult(data);
      if (!result) throw new Error(t('query.error.badResult'));
      setRaw(result);
      const saved = tabla.transform ?? [];
      setSteps(saved);
      setSelected(saved.length - 1);
    } catch (e) {
      setLoadError(e instanceof Error ? e.message : t('query.error.load'));
    } finally {
      setLoading(false);
    }
  };

  // OTRAS tablas externas de la hoja (para append/merge): nombre + columnas de su
  // instantánea YA MATERIALIZADA (rejilla actual). Excluye la tabla en edición.
  const otherTables = useMemo<OtherTable[]>(() => {
    const out: OtherTable[] = [];
    for (const t of tables) {
      if (t.table === tableName || t.range === '') continue;
      const qt = gridToQueryTable(readSheetGrid(t.range));
      if (qt) out.push({ name: t.table, columns: qt.columns });
    }
    return out;
  }, [tables, tableName]);

  // Resolutor para la vista previa: instantánea de la otra tabla desde la rejilla
  // (mismo criterio que el materializado). ponytail: usa el snapshot, no re-ejecuta
  // el pipeline de la otra tabla (re-evaluación transitiva = follow-up).
  const resolve = useMemo<QueryResolver>(
    () => (ref) => {
      const other = tables.find((t) => t.table === ref);
      if (!other || other.table === tableName || other.range === '') return undefined;
      return gridToQueryTable(readSheetGrid(other.range)) ?? undefined;
    },
    [tables, tableName],
  );

  // Vista previa: pipeline hasta el paso seleccionado (o el origen). Se recalcula
  // solo si cambian datos/pasos/selección. ponytail: reejecuta el pipeline entero
  // en cada cambio; para tablas grandes conviene cachear por prefijo de pasos.
  const preview = useMemo(() => {
    if (!raw) return null;
    return runPipeline(raw, steps.slice(0, selected + 1), resolve);
  }, [raw, steps, selected, resolve]);

  // Primer error del pipeline COMPLETO (marca el paso rojo en la lista).
  const failIndex = useMemo(
    () => (raw ? (runPipeline(raw, steps, resolve).error?.index ?? -1) : -1),
    [raw, steps, resolve],
  );

  // Columnas de ENTRADA al paso que se está editando (para elegir columnas).
  const editorCols = useMemo<QueryColumn[]>(() => {
    if (!raw || !editor) return [];
    const upto = editor.index ?? steps.length;
    return runPipeline(raw, steps.slice(0, upto), resolve).table.columns;
  }, [raw, editor, steps, resolve]);

  const openAdd = (kind: QueryStep['kind']): void => {
    const cols = raw ? runPipeline(raw, steps, resolve).table.columns : [];
    setEditor({
      index: null,
      draft: defaultStep(
        kind,
        cols,
        otherTables.map((o) => o.name),
      ),
    });
  };

  const commit = (): void => {
    if (!editor) return;
    const at = editor.index;
    setSteps((prev) =>
      at === null ? [...prev, editor.draft] : prev.map((s, i) => (i === at ? editor.draft : s)),
    );
    setSelected(at ?? steps.length);
    setEditor(null);
  };

  const move = (i: number, dir: -1 | 1): void => {
    const j = i + dir;
    if (j < 0 || j >= steps.length) return;
    setSteps((prev) => {
      const n = [...prev];
      const t = n[i]!;
      n[i] = n[j]!;
      n[j] = t;
      return n;
    });
    setSelected(j);
  };

  const remove = (i: number): void => {
    setSteps((prev) => prev.filter((_, k) => k !== i));
    setSelected((s) => (s >= i ? s - 1 : s));
    if (editor?.index === i) setEditor(null);
  };

  const apply = (): void => {
    const t = tables.find((x) => x.table === tableName);
    if (!raw || !t) return;
    dispatchSheetCommand(
      'extdata',
      JSON.stringify({
        connectionId: t.connectionId,
        spec: t.spec,
        anchor: t.anchor,
        columns: raw.columns,
        rows: raw.rows,
        transform: steps,
        name: t.table,
      }),
    );
    onClose();
  };

  const previewTable = preview?.table ?? null;

  return (
    <Modal open={open} onClose={onClose} title={t('query.steps')} size="lg">
      {tables.length === 0 ? (
        <p class="text-text-muted text-sm">{t('query.noTables')}</p>
      ) : (
        <div class="flex flex-col gap-3">
          {/* Selector de tabla + aplicar */}
          <div class="flex items-end gap-2">
            <FieldGroup label={t('query.table')} htmlFor="qs-table" class="flex-1">
              <Select
                id="qs-table"
                value={tableName}
                disabled={loading}
                onChange={(e) => void load(e.currentTarget.value)}
              >
                {tables.map((t) => (
                  <option key={t.table} value={t.table}>
                    {t.table}
                  </option>
                ))}
              </Select>
            </FieldGroup>
            <Button disabled={!raw || loading} onClick={apply}>
              <EditorIcon name="check" size="sm" />
              {t('query.applyToSheet')}
            </Button>
          </div>

          {loadError ? (
            <p class="text-danger-text text-sm" role="alert">
              {loadError}
            </p>
          ) : null}

          {loading ? (
            <div class="text-text-muted flex items-center gap-2 py-6 text-sm">
              <Spinner size={16} /> {t('query.loading')}
            </div>
          ) : raw ? (
            <div class="flex min-h-0 gap-3">
              {/* Panel izquierdo: lista de pasos + menú añadir */}
              <div class="flex w-56 shrink-0 flex-col gap-2">
                <StepList
                  steps={steps}
                  selected={selected}
                  failIndex={failIndex}
                  editingIndex={editor?.index ?? null}
                  onSelect={setSelected}
                  onEdit={(i) => setEditor({ index: i, draft: steps[i]! })}
                  onMove={move}
                  onRemove={remove}
                />
                <AddStepMenu onAdd={openAdd} />
              </div>

              {/* Panel derecho: editor del paso + vista previa */}
              <div class="flex min-w-0 flex-1 flex-col gap-3">
                {editor ? (
                  <StepEditor
                    draft={editor.draft}
                    cols={editorCols}
                    isNew={editor.index === null}
                    others={otherTables}
                    onChange={(draft) => setEditor({ index: editor.index, draft })}
                    onCommit={commit}
                    onCancel={() => setEditor(null)}
                  />
                ) : null}

                {preview?.error ? (
                  <p class="text-danger-text text-xs" role="alert">
                    {t('query.step.error', {
                      n: preview.error.index + 1,
                      message: preview.error.message,
                    })}
                  </p>
                ) : null}

                {previewTable ? <PreviewGrid table={previewTable} /> : null}
              </div>
            </div>
          ) : null}
        </div>
      )}
    </Modal>
  );
}

/* ---------- Lista de pasos aplicados ---------- */

function StepList({
  steps,
  selected,
  failIndex,
  editingIndex,
  onSelect,
  onEdit,
  onMove,
  onRemove,
}: {
  steps: readonly QueryStep[];
  selected: number;
  failIndex: number;
  editingIndex: number | null;
  onSelect: (i: number) => void;
  onEdit: (i: number) => void;
  onMove: (i: number, dir: -1 | 1) => void;
  onRemove: (i: number) => void;
}): JSX.Element {
  const t = useT();
  return (
    <div class="border-border flex flex-col overflow-hidden rounded-md border">
      {/* `aria-current="step"` y no `aria-pressed`: esto no es un conmutador, es cuál de
          los pasos de la consulta se está viendo. Sin él, el relleno era la ÚNICA señal
          de dónde estás —WCAG 1.4.1, el color no puede ser el único indicador— y quien
          usa lector de pantalla recorría la lista sin saber cuál está seleccionado. */}
      <button
        type="button"
        aria-current={selected < 0 ? 'step' : undefined}
        class={
          'border-border border-b px-2 py-1.5 text-left text-xs ' +
          (selected < 0 ? 'bg-selected text-text' : 'text-text-muted hover:bg-bg-subtle')
        }
        onClick={() => onSelect(-1)}
      >
        {t('query.source')}
      </button>
      <ol class="max-h-72 overflow-auto">
        {steps.map((s, i) => {
          const active = i === selected;
          const failed = i === failIndex;
          return (
            <li
              key={i}
              class={
                'group border-border flex items-center gap-1 border-b px-1.5 py-1 text-xs last:border-b-0 ' +
                (active ? 'bg-selected' : 'hover:bg-bg-subtle')
              }
            >
              <button
                type="button"
                class={
                  'min-w-0 flex-1 truncate text-left ' +
                  (failed ? 'text-danger-text' : active ? 'text-text' : 'text-text-muted') +
                  (i === editingIndex ? ' font-medium' : '')
                }
                title={describeStep(s)}
                aria-current={active ? 'step' : undefined}
                onClick={() => onSelect(i)}
                onDblClick={() => onEdit(i)}
              >
                {i + 1}. {describeStep(s)}
              </button>
              {/* Ocultas con opacidad hasta hover/foco: siguen en el orden de
                  tabulación, así que sin focus-within el teclado tabulaba a botones
                  invisibles; en punteros gruesos, siempre visibles (mismo patrón
                  que DocList). */}
              <div class="flex shrink-0 items-center gap-1 opacity-0 transition-opacity duration-150 group-hover:opacity-100 focus-within:opacity-100 pointer-coarse:opacity-100">
                <MiniButton
                  icon="chevronUp"
                  title={t('common.moveUp')}
                  onClick={() => onMove(i, -1)}
                />
                <MiniButton
                  icon="chevronDown"
                  title={t('common.moveDown')}
                  onClick={() => onMove(i, 1)}
                />
                <MiniButton icon="pencil" title={t('action.edit')} onClick={() => onEdit(i)} />
                <MiniButton
                  icon="trash"
                  title={t('action.removeItem')}
                  danger
                  onClick={() => onRemove(i)}
                />
              </div>
            </li>
          );
        })}
      </ol>
      {steps.length === 0 ? <p class="sytue-meta px-2 py-2">{t('query.noSteps')}</p> : null}
    </div>
  );
}

function MiniButton({
  icon,
  title,
  onClick,
  danger,
}: {
  icon: 'chevronDown' | 'chevronUp' | 'pencil' | 'trash';
  title: string;
  onClick: () => void;
  danger?: boolean;
}): JSX.Element {
  return (
    // 24 px de área activa (WCAG 2.5.8): estos botones van adyacentes y uno es
    // destructivo, así que un fallo de puntería borraba el paso.
    <button
      type="button"
      class={
        'text-text-muted hover:bg-surface inline-flex h-6 w-6 items-center justify-center rounded-sm ' +
        (danger ? 'hover:text-danger-text' : 'hover:text-text')
      }
      aria-label={title}
      title={title}
      onClick={onClick}
    >
      <EditorIcon name={icon} size="sm" />
    </button>
  );
}

/* ---------- Menú «Añadir paso» ---------- */

function AddStepMenu({ onAdd }: { onAdd: (kind: QueryStep['kind']) => void }): JSX.Element {
  const t = useT();
  const [open, setOpen] = useState(false);
  return (
    <div class="relative">
      <Button variant="secondary" size="sm" onClick={() => setOpen((o) => !o)}>
        <EditorIcon name="plus" size="sm" />
        {t('query.addStep')}
      </Button>
      {open ? (
        <div
          class="sytue-menu-surface border-border absolute bottom-full left-0 z-[var(--z-popover)] mb-1 max-h-72 w-52 overflow-auto rounded-lg border p-1 shadow-[var(--shadow-md)]"
          role="menu"
        >
          {STEP_KINDS.map((k) => (
            <MenuItem
              key={k.kind}
              size="sm"
              onClick={() => {
                onAdd(k.kind);
                setOpen(false);
              }}
            >
              {t(k.labelKey)}
            </MenuItem>
          ))}
        </div>
      ) : null}
    </div>
  );
}

/* ---------- Editor de un paso (formulario por tipo) ---------- */

function Choice({
  value,
  options,
  onChange,
  disabled,
}: {
  value: string;
  options: { value: string; label: string }[];
  onChange: (v: string) => void;
  disabled?: boolean;
}): JSX.Element {
  return (
    <Select value={value} disabled={disabled} onChange={(e) => onChange(e.currentTarget.value)}>
      {options.map((o) => (
        <option key={o.value} value={o.value}>
          {o.label}
        </option>
      ))}
    </Select>
  );
}

/** Lista de columnas con casillas; el orden = orden de marcado (importa en elegir/
 *  reordenar/agrupar). Vacío = todas (para quitar duplicados). */
function ColumnPicker({
  cols,
  value,
  onChange,
  allNote,
}: {
  cols: readonly QueryColumn[];
  value: string[];
  onChange: (v: string[]) => void;
  allNote?: string;
}): JSX.Element {
  const toggle = (name: string): void =>
    onChange(value.includes(name) ? value.filter((c) => c !== name) : [...value, name]);
  return (
    <div class="border-border max-h-36 overflow-auto rounded-sm border p-1">
      {allNote && value.length === 0 ? (
        <div class="sytue-meta-dense px-1 pb-1">{allNote}</div>
      ) : null}
      {cols.map((c) => {
        const order = value.indexOf(c.name);
        return (
          <label
            key={c.name}
            class="text-text flex cursor-pointer items-center gap-1.5 px-1 py-0.5 text-xs"
          >
            <Checkbox checked={order >= 0} class="accent-primary" onChange={() => toggle(c.name)} />
            <span class="min-w-0 flex-1 truncate">{c.name}</span>
            {order >= 0 ? <span class="sytue-meta-dense">{order + 1}</span> : null}
          </label>
        );
      })}
    </div>
  );
}

function StepEditor({
  draft,
  cols,
  isNew,
  others,
  onChange,
  onCommit,
  onCancel,
}: {
  draft: QueryStep;
  cols: readonly QueryColumn[];
  isNew: boolean;
  others: readonly OtherTable[];
  onChange: (d: QueryStep) => void;
  onCommit: () => void;
  onCancel: () => void;
}): JSX.Element {
  const t = useT();
  const colOpts = cols.map((c) => ({ value: c.name, label: c.name }));
  const noCols = colOpts.length === 0;

  return (
    <div class="border-border bg-bg-subtle flex flex-col gap-2 rounded-md border p-2.5">
      <div class="text-text text-xs font-semibold">
        {isNew ? t('query.stepNew') : t('query.stepEdit')}:{' '}
        {(() => {
          const k = STEP_KINDS.find((x) => x.kind === draft.kind);
          return k ? t(k.labelKey) : '';
        })()}
      </div>

      <StepFields
        draft={draft}
        cols={cols}
        colOpts={colOpts}
        noCols={noCols}
        others={others}
        onChange={onChange}
      />

      <div class="flex justify-end gap-2 pt-0.5">
        <Button variant="ghost" size="sm" onClick={onCancel}>
          {t('action.cancel')}
        </Button>
        <Button size="sm" disabled={!draftValid(draft)} onClick={onCommit}>
          {isNew ? t('query.step.add') : t('action.save')}
        </Button>
      </div>
    </div>
  );
}

/** Campos del formulario según el tipo de paso. `others` = otras tablas externas
 *  de la hoja (nombre + columnas), necesarias para anexar/combinar. */
function StepFields({
  draft,
  cols,
  colOpts,
  noCols,
  others,
  onChange,
}: {
  draft: QueryStep;
  cols: readonly QueryColumn[];
  colOpts: { value: string; label: string }[];
  noCols: boolean;
  others: readonly OtherTable[];
  onChange: (d: QueryStep) => void;
}): JSX.Element {
  const t = useT();
  const inputCls =
    'h-9 w-full rounded-sm border border-border bg-surface px-1.5 text-sm text-text placeholder:text-text-subtle';

  switch (draft.kind) {
    case 'filter': {
      const needsValue = draft.op !== 'empty' && draft.op !== 'nonempty';
      return (
        <>
          <Row label={t('common.column')}>
            <Choice
              value={draft.column}
              options={colOpts}
              disabled={noCols}
              onChange={(column) => onChange({ ...draft, column })}
            />
          </Row>
          <Row label={t('common.condition')}>
            <Choice
              value={draft.op}
              options={FILTER_OP_OPTS.map((o) => ({ value: o.value, label: t(o.labelKey) }))}
              onChange={(op) => onChange({ ...draft, op: op as typeof draft.op })}
            />
          </Row>
          {needsValue ? (
            <div class="flex gap-1.5">
              <input
                class={inputCls}
                placeholder={t('extdata.param.value.placeholder')}
                value={draft.value ?? ''}
                onInput={(e) => onChange({ ...draft, value: e.currentTarget.value })}
              />
              {draft.op === 'between' ? (
                <input
                  class={inputCls}
                  placeholder="y"
                  value={draft.value2 ?? ''}
                  onInput={(e) => onChange({ ...draft, value2: e.currentTarget.value })}
                />
              ) : null}
            </div>
          ) : null}
        </>
      );
    }
    case 'select':
    case 'removeColumns':
    case 'reorder':
      return (
        <ColumnPicker
          cols={cols}
          value={draft.columns}
          onChange={(columns) => onChange({ ...draft, columns })}
        />
      );
    case 'rename':
      return (
        <>
          <Row label={t('common.column')}>
            <Choice
              value={draft.column}
              options={colOpts}
              disabled={noCols}
              onChange={(column) => onChange({ ...draft, column })}
            />
          </Row>
          <Row label={t('common.newName')}>
            <input
              class={inputCls}
              value={draft.to}
              onInput={(e) => onChange({ ...draft, to: e.currentTarget.value })}
            />
          </Row>
        </>
      );
    case 'changeType':
      return (
        <>
          <Row label={t('common.column')}>
            <Choice
              value={draft.column}
              options={colOpts}
              disabled={noCols}
              onChange={(column) => onChange({ ...draft, column })}
            />
          </Row>
          <Row label={t('chart.field.type')}>
            <Choice
              value={draft.type}
              options={TYPE_OPTS.map((o) => ({ value: o.value, label: t(o.labelKey) }))}
              onChange={(type) => onChange({ ...draft, type: type as typeof draft.type })}
            />
          </Row>
        </>
      );
    case 'sort':
      return (
        <div class="flex flex-col gap-1.5">
          {draft.by.map((k, i) => (
            <div key={i} class="flex items-center gap-1.5">
              <div class="flex-1">
                <Choice
                  value={k.column}
                  options={colOpts}
                  disabled={noCols}
                  onChange={(column) =>
                    onChange({
                      ...draft,
                      by: draft.by.map((x, j) => (j === i ? { ...x, column } : x)),
                    })
                  }
                />
              </div>
              <div class="w-32">
                <Choice
                  value={k.dir}
                  options={DIR_OPTS.map((o) => ({ value: o.value, label: t(o.labelKey) }))}
                  onChange={(dir) =>
                    onChange({
                      ...draft,
                      by: draft.by.map((x, j) =>
                        j === i ? { ...x, dir: dir as typeof x.dir } : x,
                      ),
                    })
                  }
                />
              </div>
              <Button
                variant="ghost"
                size="md"
                iconOnly
                aria-label={t('smartart.levelRemove')}
                onClick={() => onChange({ ...draft, by: draft.by.filter((_, j) => j !== i) })}
              >
                <EditorIcon name="close" size="sm" />
              </Button>
            </div>
          ))}
          <div>
            <Button
              variant="ghost"
              size="sm"
              disabled={noCols}
              onClick={() =>
                onChange({
                  ...draft,
                  by: [...draft.by, { column: cols[0]?.name ?? '', dir: 'asc' }],
                })
              }
            >
              <EditorIcon name="plus" size="sm" />
              {t('query.addLevel')}
            </Button>
          </div>
        </div>
      );
    case 'groupBy':
      return (
        <div class="flex flex-col gap-2">
          <div>
            <div class="sytue-meta-dense mb-0.5">{t('query.groupBy')}</div>
            <ColumnPicker
              cols={cols}
              value={draft.columns}
              onChange={(columns) => onChange({ ...draft, columns })}
            />
          </div>
          <div class="flex flex-col gap-1.5">
            <div class="sytue-meta-dense">{t('query.aggregations')}</div>
            {draft.aggs.map((a, i) => (
              <div key={i} class="flex items-center gap-1.5">
                <div class="w-24">
                  <Choice
                    value={a.fn}
                    options={AGG_OPTS.map((o) => ({ value: o.value, label: t(o.labelKey) }))}
                    onChange={(fn) =>
                      onChange({
                        ...draft,
                        aggs: draft.aggs.map((x, j) =>
                          j === i ? { ...x, fn: fn as typeof x.fn } : x,
                        ),
                      })
                    }
                  />
                </div>
                <div class="min-w-0 flex-1">
                  <Choice
                    value={a.column}
                    options={
                      a.fn === 'count' ? [{ value: '', label: '(filas)' }, ...colOpts] : colOpts
                    }
                    disabled={noCols}
                    onChange={(column) =>
                      onChange({
                        ...draft,
                        aggs: draft.aggs.map((x, j) => (j === i ? { ...x, column } : x)),
                      })
                    }
                  />
                </div>
                <input
                  class={inputCls + ' w-24'}
                  placeholder={t('extdata.param.name.placeholder')}
                  value={a.as}
                  onInput={(e) =>
                    onChange({
                      ...draft,
                      aggs: draft.aggs.map((x, j) =>
                        j === i ? { ...x, as: e.currentTarget.value } : x,
                      ),
                    })
                  }
                />
                <Button
                  variant="ghost"
                  size="md"
                  iconOnly
                  aria-label={t('query.aggregation.remove')}
                  onClick={() => onChange({ ...draft, aggs: draft.aggs.filter((_, j) => j !== i) })}
                >
                  <EditorIcon name="close" size="sm" />
                </Button>
              </div>
            ))}
            <div>
              <Button
                variant="ghost"
                size="sm"
                onClick={() =>
                  onChange({
                    ...draft,
                    aggs: [...draft.aggs, { column: '', fn: 'count', as: '' }],
                  })
                }
              >
                <EditorIcon name="plus" size="sm" />
                {t('query.addAgg')}
              </Button>
            </div>
          </div>
        </div>
      );
    case 'addColumn':
      return (
        <>
          <Row label={t('extdata.field.name')}>
            <input
              class={inputCls}
              value={draft.name}
              onInput={(e) => onChange({ ...draft, name: e.currentTarget.value })}
            />
          </Row>
          <Row label={t('common.expression')}>
            <input
              class={inputCls}
              placeholder="{precio} * 1,21"
              value={draft.expr}
              onInput={(e) => onChange({ ...draft, expr: e.currentTarget.value })}
            />
          </Row>
          <p class="sytue-meta-dense">
            Constante (texto o número), referencias entre llaves como {'{columna}'} o aritmética
            simple (+ - * / y paréntesis).
          </p>
        </>
      );
    case 'replace':
      return (
        <>
          <Row label={t('common.column')}>
            <Choice
              value={draft.column}
              options={colOpts}
              disabled={noCols}
              onChange={(column) => onChange({ ...draft, column })}
            />
          </Row>
          <div class="flex gap-1.5">
            <input
              class={inputCls}
              placeholder={t('query.find')}
              value={draft.find}
              onInput={(e) => onChange({ ...draft, find: e.currentTarget.value })}
            />
            <input
              class={inputCls}
              placeholder={t('query.replaceWith')}
              value={draft.replace}
              onInput={(e) => onChange({ ...draft, replace: e.currentTarget.value })}
            />
          </div>
        </>
      );
    case 'dedupe':
      return (
        <ColumnPicker
          cols={cols}
          value={draft.columns ?? []}
          allNote={t('query.dedupe.allNote')}
          onChange={(columns) =>
            onChange(columns.length > 0 ? { kind: 'dedupe', columns } : { kind: 'dedupe' })
          }
        />
      );
    case 'keepRows':
    case 'removeRows':
      return (
        <div class="flex items-end gap-1.5">
          <div class="w-32">
            <Choice
              value={draft.end}
              options={END_OPTS.map((o) => ({ value: o.value, label: t(o.labelKey) }))}
              onChange={(end) => onChange({ ...draft, end: end as typeof draft.end })}
            />
          </div>
          <input
            // El único campo de este panel que no va dentro de un `Row`, y `Row` es
            // quien envuelve en `<label>`: sin esto, un lector de pantalla lo anuncia
            // como «campo de edición, número» y no dice de qué. El «filas» que hay al
            // lado es un `<span>` suelto, que no nombra a nada.
            aria-label={t('query.rowsCountAria')}
            class={inputCls + ' w-20'}
            type="number"
            min={0}
            value={String(draft.count)}
            onInput={(e) =>
              onChange({
                ...draft,
                count: Math.max(0, Math.trunc(Number(e.currentTarget.value) || 0)),
              })
            }
          />
          <span class="text-text-muted pb-1.5 text-xs">{t('query.rowsUnit')}</span>
        </div>
      );
    case 'unpivot':
      return (
        <div class="flex flex-col gap-2">
          <div>
            <div class="sytue-meta-dense mb-0.5">{t('query.unpivot.columns')}</div>
            <ColumnPicker
              cols={cols}
              value={draft.columns}
              onChange={(columns) => onChange({ ...draft, columns })}
            />
          </div>
          <Row label={t('query.attrColumn')}>
            <input
              class={inputCls}
              value={draft.attribute}
              onInput={(e) => onChange({ ...draft, attribute: e.currentTarget.value })}
            />
          </Row>
          <Row label={t('query.valueColumn')}>
            <input
              class={inputCls}
              value={draft.value}
              onInput={(e) => onChange({ ...draft, value: e.currentTarget.value })}
            />
          </Row>
        </div>
      );
    case 'pivot':
      return (
        <>
          <Row label={t('query.keyColumn')}>
            <Choice
              value={draft.key}
              options={colOpts}
              disabled={noCols}
              onChange={(key) => onChange({ ...draft, key })}
            />
          </Row>
          <Row label={t('query.valuesColumn')}>
            <Choice
              value={draft.value}
              options={colOpts}
              disabled={noCols}
              onChange={(value) => onChange({ ...draft, value })}
            />
          </Row>
          <Row label={t('query.aggregation')}>
            <Choice
              value={draft.fn}
              options={AGG_OPTS.map((o) => ({ value: o.value, label: t(o.labelKey) }))}
              onChange={(fn) => onChange({ ...draft, fn: fn as typeof draft.fn })}
            />
          </Row>
        </>
      );
    case 'append': {
      if (others.length === 0)
        return <p class="sytue-meta-dense">{t('query.noOtherTable.append')}</p>;
      return (
        <Row label={t('query.appendFrom')}>
          <Choice
            value={draft.from}
            options={others.map((o) => ({ value: o.name, label: o.name }))}
            onChange={(from) => onChange({ ...draft, from })}
          />
        </Row>
      );
    }
    case 'merge': {
      if (others.length === 0)
        return <p class="sytue-meta-dense">{t('query.noOtherTable.merge')}</p>;
      const rightCols = others.find((o) => o.name === draft.from)?.columns ?? [];
      const rightOpts = rightCols.map((c) => ({ value: c.name, label: c.name }));
      const isAnti = draft.join === 'antiLeft';
      return (
        <div class="flex flex-col gap-2">
          <Row label={t('query.mergeWith')}>
            <Choice
              value={draft.from}
              options={others.map((o) => ({ value: o.name, label: o.name }))}
              onChange={(from) => onChange({ ...draft, from })}
            />
          </Row>
          <Row label={t('query.mergeType')}>
            <Choice
              value={draft.join}
              options={JOIN_OPTS.map((o) => ({ value: o.value, label: t(o.labelKey) }))}
              onChange={(join) => onChange({ ...draft, join: join as typeof draft.join })}
            />
          </Row>
          <div class="flex flex-col gap-1.5">
            <div class="sytue-meta-dense">{t('query.keyColumns')}</div>
            {draft.on.map((p, i) => (
              <div key={i} class="flex items-center gap-1.5">
                <div class="min-w-0 flex-1">
                  <Choice
                    value={p.left}
                    options={colOpts}
                    disabled={noCols}
                    onChange={(left) =>
                      onChange({
                        ...draft,
                        on: draft.on.map((x, j) => (j === i ? { ...x, left } : x)),
                      })
                    }
                  />
                </div>
                <span class="sytue-meta-dense shrink-0">=</span>
                <div class="min-w-0 flex-1">
                  <Choice
                    value={p.right}
                    options={rightOpts}
                    disabled={rightOpts.length === 0}
                    onChange={(right) =>
                      onChange({
                        ...draft,
                        on: draft.on.map((x, j) => (j === i ? { ...x, right } : x)),
                      })
                    }
                  />
                </div>
                <Button
                  variant="ghost"
                  size="md"
                  iconOnly
                  aria-label={t('query.keyPair.remove')}
                  onClick={() => onChange({ ...draft, on: draft.on.filter((_, j) => j !== i) })}
                >
                  <EditorIcon name="close" size="sm" />
                </Button>
              </div>
            ))}
            <div>
              <Button
                variant="ghost"
                size="sm"
                disabled={noCols}
                onClick={() =>
                  onChange({
                    ...draft,
                    on: [...draft.on, { left: cols[0]?.name ?? '', right: '' }],
                  })
                }
              >
                <EditorIcon name="plus" size="sm" />
                {t('query.addKeyPair')}
              </Button>
            </div>
          </div>
          {isAnti ? (
            <p class="sytue-meta-dense">{t('query.merge.antiNote')}</p>
          ) : (
            <>
              <div>
                <div class="sytue-meta-dense mb-0.5">{t('query.merge.bring')}</div>
                <ColumnPicker
                  cols={rightCols}
                  value={draft.bring}
                  onChange={(bring) => onChange({ ...draft, bring })}
                />
              </div>
              <Row label={t('query.collisionPrefix')}>
                <input
                  class={inputCls}
                  placeholder="p. ej. «otra_»"
                  value={draft.prefix ?? ''}
                  onInput={(e) => onChange({ ...draft, prefix: e.currentTarget.value })}
                />
              </Row>
            </>
          )}
        </div>
      );
    }
  }
}

function Row({ label, children }: { label: string; children: JSX.Element }): JSX.Element {
  return (
    <label class="flex flex-col gap-0.5">
      {/* `text-subtle` sobre `bg-subtle` da 4,20:1 y AA pide 4,5:1 para texto de este
          tamaño: la etiqueta que dice QUÉ es un campo se leía peor que su contenido.
          `text-muted` sube a 6,01:1 sin cambiar ni el tamaño ni el peso. */}
      <span class="text-text-muted text-2xs">{label}</span>
      {children}
    </label>
  );
}

/* ---------- Vista previa del resultado ---------- */

function PreviewGrid({ table }: { table: QueryTable }): JSX.Element {
  const t = useT();
  const rows = table.rows.slice(0, PREVIEW_ROWS);
  return (
    <div class="flex min-h-0 flex-col gap-1.5">
      <div class="border-border max-h-72 overflow-auto rounded-md border">
        <table class="w-full border-collapse text-xs">
          <thead class="bg-bg-subtle sticky top-0">
            <tr>
              {table.columns.map((c, i) => (
                <th
                  key={i}
                  class="border-border text-text truncate border-b px-2 py-1 text-left font-semibold"
                  title={`${c.name} · ${c.type}`}
                >
                  {c.name}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row, r) => (
              <tr key={r} class="odd:bg-bg-subtle/40">
                {table.columns.map((_, c) => (
                  <td
                    key={c}
                    class="border-border/60 text-text-muted max-w-52 truncate border-b px-2 py-1"
                  >
                    {cellText(row[c])}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <p class="sytue-meta">
        {t('query.preview.rows', { count: table.rows.length })} ·{' '}
        {t('query.preview.cols', { count: table.columns.length })}
        {table.rows.length > PREVIEW_ROWS
          ? ` · ${t('query.preview.firstN', { n: String(PREVIEW_ROWS) })}`
          : ''}
      </p>
    </div>
  );
}
