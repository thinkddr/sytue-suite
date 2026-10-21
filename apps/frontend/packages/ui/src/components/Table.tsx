/**
 * Table — tabla densa con cabeceras, orden y seleccion de filas (sin paginacion:
 * el consumidor pagina/virtualiza; ver VirtualList para listas grandes).
 * Semantica `table`, scope en cabeceras, orden anunciado (aria-sort).
 * docs/design.md §9.
 */
import type { ComponentChildren, JSX } from 'preact';
import { useT } from '@sytue/i18n/preact';
import { cx } from '../lib/cx.js';
import { Icon } from './Icon.js';
import { Checkbox } from './Form.js';

export type SortDirection = 'asc' | 'desc';

export interface TableColumn<Row> {
  /** Clave estable de la columna. */
  key: string;
  header: ComponentChildren;
  /** Render de celda. */
  cell: (row: Row) => ComponentChildren;
  sortable?: boolean;
  align?: 'start' | 'center' | 'end';
  width?: string;
}

export interface TableProps<Row> {
  columns: TableColumn<Row>[];
  rows: Row[];
  /** Identificador estable de fila (para keys y seleccion). El índice sirve de
   *  respaldo estable cuando la fila no tiene id natural (resultados SQL). */
  rowKey: (row: Row, index: number) => string;
  caption?: string;
  sortKey?: string;
  sortDirection?: SortDirection;
  onSort?: (key: string) => void;
  selectedKeys?: ReadonlySet<string>;
  onToggleRow?: (key: string) => void;
  emptyContent?: ComponentChildren;
  /** Fila de resumen/totales opcional (tfoot). Mapea clave de columna → contenido
   *  y reutiliza la alineación + tabular-nums de cada columna. Sin ella no se
   *  renderiza tfoot (comportamiento actual). */
  summaryRow?: Partial<Record<string, ComponentChildren>>;
  class?: string;
}

const alignClass: Record<NonNullable<TableColumn<unknown>['align']>, string> = {
  start: 'text-left',
  center: 'text-center',
  end: 'text-right',
};

export function Table<Row>({
  columns,
  rows,
  rowKey,
  caption,
  sortKey,
  sortDirection = 'asc',
  onSort,
  selectedKeys,
  onToggleRow,
  emptyContent,
  summaryRow,
  class: cls,
}: TableProps<Row>): JSX.Element {
  const t = useT();
  const selectable = Boolean(onToggleRow);
  const colCount = columns.length + (selectable ? 1 : 0);

  return (
    <div
      class={cx(
        'border-border bg-surface overflow-x-auto rounded-lg border shadow-[var(--shadow-sm)]',
        cls,
      )}
    >
      <table class="w-full border-collapse text-base">
        {caption ? <caption class="sr-only">{caption}</caption> : null}
        <thead>
          {/* Cabecera BLANCA con rótulos en tinta y negrita, la de un listado de
              oficina. */}
          <tr class="border-border bg-surface border-b">
            {selectable ? <th scope="col" class="w-10 px-3 py-2.5" /> : null}
            {columns.map((col) => {
              const isSorted = sortKey === col.key;
              return (
                <th
                  key={col.key}
                  scope="col"
                  style={col.width ? { width: col.width } : undefined}
                  aria-sort={
                    col.sortable
                      ? isSorted
                        ? sortDirection === 'asc'
                          ? 'ascending'
                          : 'descending'
                        : 'none'
                      : undefined
                  }
                  class={cx(
                    'sytue-overline text-text px-3 py-2.5 font-bold',
                    alignClass[col.align ?? 'start'],
                  )}
                >
                  {col.sortable && onSort ? (
                    <button
                      type="button"
                      onClick={() => onSort(col.key)}
                      class="group/sort hover:text-text sytue-focus-ring inline-flex items-center gap-1 rounded-sm"
                    >
                      {col.header}
                      {/* Columna ordenada: flecha según dirección; sin ordenar:
                          la flecha aparece al hover/foco como affordance. */}
                      <Icon
                        name={isSorted && sortDirection === 'desc' ? 'chevronDown' : 'chevronUp'}
                        size={14}
                        class={
                          isSorted
                            ? // La flecha de la columna ordenada lleva el color de «estás
                              // aquí»: de un vistazo se ve POR QUÉ columna está ordenado.
                              'text-indicator'
                            : 'opacity-0 transition-opacity duration-150 group-hover/sort:opacity-60 group-focus-visible/sort:opacity-60'
                        }
                      />
                    </button>
                  ) : (
                    col.header
                  )}
                </th>
              );
            })}
          </tr>
        </thead>
        <tbody>
          {rows.length === 0 ? (
            <tr>
              <td colSpan={colCount} class="text-text-muted px-3 py-10 text-center">
                {emptyContent ?? t('ui.noResults')}
              </td>
            </tr>
          ) : (
            rows.map((row, rowIndex) => {
              const key = rowKey(row, rowIndex);
              const selected = selectedKeys?.has(key) ?? false;
              return (
                <tr
                  key={key}
                  class={cx(
                    // Separación por borde hairline al 8% (spec §Table: sin zebra
                    // por defecto); además la zebra anulaba el hover en tema oscuro.
                    'border-border-subtle border-b transition-colors last:border-b-0',
                    selected ? 'bg-selected' : 'hover:bg-bg-subtle',
                  )}
                >
                  {selectable ? (
                    <td class="px-3 py-[calc(0.625rem+var(--sytue-row-grow))]">
                      <Checkbox
                        checked={selected}
                        onChange={() => onToggleRow?.(key)}
                        aria-label={t('ui.selectRow')}
                        class="border-border-strong accent-primary h-4 w-4 rounded-sm"
                      />
                    </td>
                  ) : null}
                  {columns.map((col) => (
                    <td
                      key={col.key}
                      class={cx(
                        // El alto de la fila es de FORMA: `--sytue-row-grow` suma 1 px
                        // arriba y abajo en «Marco» y nada en «Carril».
                        'text-text px-3 py-[calc(0.625rem+var(--sytue-row-grow))]',
                        alignClass[col.align ?? 'start'],
                        col.align === 'end' && 'tabular-nums',
                      )}
                    >
                      {col.cell(row)}
                    </td>
                  ))}
                </tr>
              );
            })
          )}
        </tbody>
        {summaryRow ? (
          <tfoot>
            <tr class="border-border border-t">
              {selectable ? <td class="px-3 py-2.5" /> : null}
              {columns.map((col) => (
                <td
                  key={col.key}
                  class={cx(
                    'text-text px-3 py-2.5 font-semibold',
                    alignClass[col.align ?? 'start'],
                    col.align === 'end' && 'tabular-nums',
                  )}
                >
                  {summaryRow[col.key]}
                </td>
              ))}
            </tr>
          </tfoot>
        ) : null}
      </table>
    </div>
  );
}
