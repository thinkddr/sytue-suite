/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El diálogo de la TABLA DINÁMICA: el origen y el destino, y las cuatro zonas de
 * campos —Filtros, Columnas, Filas y Valores— con las opciones de cada campo
 * (agrupar fechas o números, orden, elementos que se ven; y en los valores, cómo se
 * resume, cómo se muestra, su formato y su nombre). No calcula nada: manda la
 * definición con `pivot` y la hoja la calcula y la escribe (sheets/pivot.ts).
 *
 * Sin arrastrar y soltar: cada zona tiene su «Añadir campo» y cada campo sus flechas.
 * Es lo mismo que el panel de campos de Excel o de Calc con teclado, y cabe igual en
 * un móvil.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Button, Checkbox, FieldGroup, Input, Modal, Select } from '@sytue/ui';
import { useT } from '../context';
import type { DocsKey } from '../../i18n';
import { EditorIcon } from '../icons';
import { dispatchSheetCommand, readSheetValues } from '../sheets/SheetCanvas';
import { NUMFMT_PRESETS, formatCellValue, parseRangeRef } from '../sheets/format';
import { PIVOT_DATES, PIVOT_FNS, PIVOT_SHOWS, pivotItems } from '../sheets/pivot';
import type { PivotFieldRef, PivotShow, PivotValueRef, SheetPivot } from '../sheets/pivot';
import { PIVOT_FN_LABEL, pivotLabels } from '../sheets/pivot-hoja';
import type { NumFmtId } from '../types';

type Zone = 'filters' | 'cols' | 'rows' | 'values';

/** Un cambio de un campo: `undefined` quita la opción (vuelve a la de por defecto). */
type Patch<T> = { [K in keyof T]?: T[K] | undefined };
const ZONES: { id: Zone; labelKey: DocsKey }[] = [
  { id: 'filters', labelKey: 'pivot.zone.filters' },
  { id: 'cols', labelKey: 'layout.columns' },
  { id: 'rows', labelKey: 'common.rows' },
  { id: 'values', labelKey: 'common.values' },
];

const SHOW_LABEL: Record<PivotShow, DocsKey> = {
  normal: 'pivot.show.normal',
  pctGrand: 'pivot.show.pctGrand',
  pctCol: 'pivot.show.pctCol',
  pctRow: 'pivot.show.pctRow',
  pctParent: 'pivot.show.pctParent',
};

/** La definición que se edita: la de la tabla, sin su rango escrito. */
export type PivotDraft = Omit<SheetPivot, 'range'>;

/** Una definición nueva para el origen dado: destino dos columnas a su derecha. */
export function newPivotDraft(source: string): PivotDraft {
  const rr = parseRangeRef(source);
  const c = rr ? Math.min(rr.c2 + 2, 25) : 7;
  return {
    name: '',
    source,
    anchor: `${String.fromCharCode(65 + c)}${(rr?.r1 ?? 0) + 1}`,
    rows: [],
    cols: [],
    values: [],
  };
}

export function PivotModal({
  open,
  initial,
  onClose,
}: {
  open: boolean;
  initial: PivotDraft;
  onClose: () => void;
}): JSX.Element {
  const t = useT();
  const [d, setD] = useState<PivotDraft>(initial);
  const [expanded, setExpanded] = useState<string | null>(null);
  useEffect(() => {
    if (open) {
      setD(initial);
      setExpanded(null);
    }
  }, [open, initial]);

  const grid = open ? (readSheetValues(d.source.trim(), d.sourceSheet) ?? []) : [];
  const headers = (grid[0] ?? []).map((h, i) => {
    const s = formatCellValue(h);
    return s.trim() !== '' ? s : t('common.columnN', { n: i + 1 });
  });
  const labels = pivotLabels(t);
  const editing = initial.name !== '';
  const valid =
    parseRangeRef(d.source) !== null &&
    parseRangeRef(d.anchor) !== null &&
    headers.length > 0 &&
    d.rows.length + d.cols.length + d.values.length > 0;

  const zoneList = (z: Zone): (PivotFieldRef | PivotValueRef)[] =>
    z === 'filters' ? (d.filters ?? []) : d[z];
  const setZone = (z: Zone, list: (PivotFieldRef | PivotValueRef)[]): void =>
    setD({ ...d, [z]: list });
  const add = (z: Zone, field: number): void =>
    setZone(z, [...zoneList(z), z === 'values' ? { field, fn: 'sum' } : { field }]);

  const submit = (): void => {
    if (!valid) return;
    const def: Record<string, unknown> = {
      ...d,
      source: d.source.trim().toUpperCase(),
      anchor: d.anchor.trim().toUpperCase(),
    };
    if (def['name'] === '') delete def['name'];
    dispatchSheetCommand('pivot', JSON.stringify(def));
    onClose();
  };

  // Actualizar y eliminar a la izquierda en escritorio (`md:mr-auto` en el último); en
  // el teléfono, una pareja encima de Cancelar | Crear (el pie del Modal).
  const footer = (
    <>
      {editing && (
        <>
          <Button
            variant="secondary"
            size="sm"
            onClick={() => {
              dispatchSheetCommand('pivotrefresh', initial.name);
              onClose();
            }}
          >
            <EditorIcon name="refresh" size="sm" />
            {t('pivot.refresh')}
          </Button>
          <Button
            variant="danger"
            size="sm"
            class="md:mr-auto"
            onClick={() => {
              dispatchSheetCommand('pivotdel', initial.name);
              onClose();
            }}
          >
            {t('pivot.delete')}
          </Button>
        </>
      )}
      <Button variant="secondary" size="sm" onClick={onClose}>
        {t('action.cancel')}
      </Button>
      <Button size="sm" disabled={!valid} onClick={submit}>
        {editing ? t('action.apply') : t('data.pivot.create')}
      </Button>
    </>
  );

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={editing ? t('pivot.edit') : t('data.pivot')}
      size="lg"
      footer={footer}
    >
      <div class="flex flex-col gap-3">
        {/* Una columna por debajo de `sm`, como las zonas de debajo: a 165 los rótulos
            («Origen (con fila de encabezado)») partían en dos líneas. */}
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <FieldGroup label={t('data.pivot.source')} htmlFor="pv-source">
            <Input
              id="pv-source"
              size="sm"
              value={d.source}
              onInput={(e) => setD({ ...d, source: e.currentTarget.value })}
            />
          </FieldGroup>
          <FieldGroup label={t('data.targetTopLeft')} htmlFor="pv-anchor">
            <Input
              id="pv-anchor"
              size="sm"
              value={d.anchor}
              onInput={(e) => setD({ ...d, anchor: e.currentTarget.value })}
            />
          </FieldGroup>
        </div>
        {headers.length === 0 ? (
          <p class="sytue-meta">{t('data.pivot.pickFields')}</p>
        ) : (
          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
            {ZONES.map((z) => (
              <section
                key={z.id}
                aria-label={t(z.labelKey)}
                class="border-border flex min-w-0 flex-col gap-1.5 rounded-md border p-2"
              >
                <h3 class="text-text text-xs font-medium">{t(z.labelKey)}</h3>
                {zoneList(z.id).map((ref, i, list) => {
                  const id = `${z.id}:${i}`;
                  const name = headers[ref.field] ?? t('common.columnN', { n: ref.field + 1 });
                  const move = (dir: -1 | 1): void => {
                    const next = [...list];
                    const [x] = next.splice(i, 1);
                    next.splice(i + dir, 0, x!);
                    setZone(z.id, next);
                  };
                  const change = (patch: Patch<PivotFieldRef & PivotValueRef>): void =>
                    setZone(
                      z.id,
                      list.map((x, j) => (j === i ? ({ ...x, ...patch } as typeof x) : x)),
                    );
                  const isValue = z.id === 'values';
                  const shown = isValue
                    ? ((ref as PivotValueRef).name ??
                      labels.valueName((ref as PivotValueRef).fn, name))
                    : name;
                  return (
                    <div key={id} class="bg-bg-subtle flex flex-col gap-1.5 rounded-sm px-1.5 py-1">
                      <div class="flex items-center gap-1">
                        <button
                          type="button"
                          class="text-text sytue-focus-ring min-w-0 flex-1 truncate rounded-sm text-left text-xs"
                          aria-expanded={expanded === id}
                          aria-label={t('pivot.field.options', { field: shown })}
                          onClick={() => setExpanded(expanded === id ? null : id)}
                        >
                          {shown}
                        </button>
                        <IconBtn
                          icon="chevronUp"
                          label={t('pivot.field.up')}
                          disabled={i === 0}
                          onClick={() => move(-1)}
                        />
                        <IconBtn
                          icon="chevronDown"
                          label={t('pivot.field.down')}
                          disabled={i === list.length - 1}
                          onClick={() => move(1)}
                        />
                        <IconBtn
                          icon="close"
                          label={t('pivot.field.remove', { field: shown })}
                          onClick={() =>
                            setZone(
                              z.id,
                              list.filter((_, j) => j !== i),
                            )
                          }
                        />
                      </div>
                      {expanded === id &&
                        (isValue ? (
                          <ValueOptions value={ref as PivotValueRef} onChange={change} />
                        ) : (
                          <FieldOptions
                            value={ref}
                            filter={z.id === 'filters'}
                            items={pivotItems(grid, ref, {
                              ...labels,
                              show: (v) => formatCellValue(v),
                            })}
                            onChange={change}
                          />
                        ))}
                    </div>
                  );
                })}
                <Select
                  size="sm"
                  aria-label={`${t(z.labelKey)}: ${t('pivot.addField')}`}
                  value=""
                  onChange={(e) => {
                    const v = e.currentTarget.value;
                    if (v !== '') add(z.id, Number(v));
                    e.currentTarget.value = '';
                  }}
                >
                  <option value="">{t('pivot.addField')}</option>
                  {headers.map((h, i) => (
                    <option key={i} value={String(i)}>
                      {h}
                    </option>
                  ))}
                </Select>
              </section>
            ))}
          </div>
        )}
        {headers.length > 0 && d.rows.length + d.cols.length + d.values.length === 0 && (
          <p class="sytue-meta">{t('pivot.empty')}</p>
        )}
        <div class="flex flex-wrap gap-x-4 gap-y-1.5">
          <Checkbox
            class="text-xs"
            label={t('pivot.grandRows')}
            checked={d.grandRows !== false}
            onChange={(e) => setD({ ...d, grandRows: e.currentTarget.checked })}
          />
          <Checkbox
            class="text-xs"
            label={t('pivot.grandCols')}
            checked={d.grandCols !== false}
            onChange={(e) => setD({ ...d, grandCols: e.currentTarget.checked })}
          />
          <Checkbox
            class="text-xs"
            label={t('pivot.subtotals')}
            checked={d.subtotals !== false}
            onChange={(e) => setD({ ...d, subtotals: e.currentTarget.checked })}
          />
        </div>
      </div>
    </Modal>
  );
}

/** Rótulo pequeño encima de su control (`sytue-meta-dense`), como `PivotField`. */
function Lbl({ label, children }: { label: string; children: JSX.Element }): JSX.Element {
  return (
    <label class="flex min-w-0 flex-col gap-0.5">
      <span class="sytue-meta-dense">{label}</span>
      {children}
    </label>
  );
}

function IconBtn({
  icon,
  label,
  disabled,
  onClick,
}: {
  icon: 'chevronUp' | 'chevronDown' | 'close';
  label: string;
  disabled?: boolean;
  onClick: () => void;
}): JSX.Element {
  return (
    <button
      type="button"
      class="text-text-muted hover:bg-selected hover:text-text sytue-focus-ring inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-sm disabled:pointer-events-none disabled:opacity-40"
      aria-label={label}
      title={label}
      disabled={disabled}
      onClick={onClick}
    >
      <EditorIcon name={icon} size="sm" />
    </button>
  );
}

/** Opciones de un campo de filas, columnas o filtros. */
function FieldOptions({
  value,
  filter,
  items,
  onChange,
}: {
  value: PivotFieldRef;
  filter: boolean;
  items: { key: string; label: string }[];
  onChange: (p: Patch<PivotFieldRef>) => void;
}): JSX.Element {
  const t = useT();
  const group = value.bin ? 'range' : (value.date ?? 'none');
  const hidden = value.hidden ?? [];
  return (
    <div class="flex flex-col gap-1.5">
      <div class="grid grid-cols-2 gap-1.5">
        <Lbl label={t('pivot.group')}>
          <Select
            size="sm"
            value={group}
            onChange={(e) => {
              const g = e.currentTarget.value;
              onChange({
                date: PIVOT_DATES.includes(g as never) ? (g as PivotFieldRef['date']) : undefined,
                bin: g === 'range' ? { start: 0, end: 100, step: 10 } : undefined,
              });
            }}
          >
            {(['none', ...PIVOT_DATES, 'range'] as const).map((g) => (
              <option key={g} value={g}>
                {t(`pivot.group.${g}`)}
              </option>
            ))}
          </Select>
        </Lbl>
        {!filter && (
          <Lbl label={t('pivot.order')}>
            <Select
              size="sm"
              value={value.order ?? 'asc'}
              onChange={(e) =>
                onChange({ order: e.currentTarget.value === 'desc' ? 'desc' : undefined })
              }
            >
              <option value="asc">{t('pivot.order.asc')}</option>
              <option value="desc">{t('pivot.order.desc')}</option>
            </Select>
          </Lbl>
        )}
      </div>
      {value.bin && (
        <div class="grid grid-cols-3 gap-1.5">
          {(['start', 'end', 'step'] as const).map((k) => (
            <Lbl key={k} label={t(`pivot.bin.${k}`)}>
              <Input
                size="sm"
                type="number"
                value={String(value.bin![k])}
                onInput={(e) => {
                  const n = Number(e.currentTarget.value);
                  if (Number.isFinite(n) && (k !== 'step' || n > 0))
                    onChange({ bin: { ...value.bin!, [k]: n } });
                }}
              />
            </Lbl>
          ))}
        </div>
      )}
      <fieldset class="flex flex-col gap-0.5">
        <legend class="sytue-meta-dense mb-0.5">{t('pivot.items')}</legend>
        <div class="border-border bg-surface max-h-28 overflow-auto rounded-sm border px-1.5 py-1">
          {items.map((it) => (
            <Checkbox
              key={it.key}
              class="text-xs"
              label={it.label}
              checked={!hidden.includes(it.key)}
              onChange={(e) => {
                const next = e.currentTarget.checked
                  ? hidden.filter((k) => k !== it.key)
                  : [...hidden, it.key];
                onChange({ hidden: next.length > 0 ? next : undefined });
              }}
            />
          ))}
        </div>
      </fieldset>
    </div>
  );
}

/** Opciones de un campo de valores. */
function ValueOptions({
  value,
  onChange,
}: {
  value: PivotValueRef;
  onChange: (p: Patch<PivotValueRef>) => void;
}): JSX.Element {
  const t = useT();
  return (
    <div class="grid grid-cols-2 gap-1.5">
      <Lbl label={t('pivot.fn')}>
        <Select
          size="sm"
          value={value.fn}
          onChange={(e) => onChange({ fn: e.currentTarget.value as PivotValueRef['fn'] })}
        >
          {PIVOT_FNS.map((f) => (
            <option key={f} value={f}>
              {t(PIVOT_FN_LABEL[f])}
            </option>
          ))}
        </Select>
      </Lbl>
      <Lbl label={t('pivot.show')}>
        <Select
          size="sm"
          value={value.show ?? 'normal'}
          onChange={(e) => {
            const s = e.currentTarget.value as PivotShow;
            onChange({ show: s === 'normal' ? undefined : s });
          }}
        >
          {PIVOT_SHOWS.map((s) => (
            <option key={s} value={s}>
              {t(SHOW_LABEL[s])}
            </option>
          ))}
        </Select>
      </Lbl>
      <Lbl label={t('pivot.format')}>
        <Select
          size="sm"
          value={value.fmt?.id ?? 'general'}
          onChange={(e) => {
            const id = e.currentTarget.value as NumFmtId;
            onChange({ fmt: id === 'general' ? undefined : { id } });
          }}
        >
          {NUMFMT_PRESETS.map((p) => (
            <option key={p.value} value={p.value}>
              {t(p.labelKey)}
            </option>
          ))}
        </Select>
      </Lbl>
      <Lbl label={t('common.name')}>
        <Input
          size="sm"
          value={value.name ?? ''}
          onInput={(e) => {
            const n = e.currentTarget.value;
            onChange({ name: n.trim() === '' ? undefined : n });
          }}
        />
      </Lbl>
    </div>
  );
}
