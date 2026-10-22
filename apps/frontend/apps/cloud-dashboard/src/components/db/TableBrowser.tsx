/**
 * TableBrowser — explorador de tablas de Sytue DB Studio (estilo Supabase con
 * diseño Sytue). Panel izquierdo: lista de tablas por esquema con conteo de
 * filas y si están expuestas en la API REST (PostgREST). Panel derecho: tabla
 * de datos (primeras filas) + definición de columnas (tipos, PK, FK). Todo con
 * componentes de @sytue/ui. docs/CONVENTIONS.md §14.
 */
import { useEffect, useMemo, useState } from 'preact/hooks';
import { Table, Badge, Button, Icon, Tabs, Input, EmptyState, Spinner } from '@sytue/ui';
import type { TableColumn } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { contiene, formatNumber } from '@sytue/i18n';
import type { DbRows, DbTable } from '../../lib/types.js';
import type { T } from '../../i18n/index.js';
import { cellToString } from '../../lib/format.js';

interface TableBrowserProps {
  /** Slug del proyecto (para pedir filas al BFF bajo demanda). */
  project: string;
  tables: DbTable[];
  /** Filas precargadas por tabla (SSR); el resto se piden al abrir la tabla. */
  rowsByTable: Record<string, DbRows>;
  /** Traductor del idioma ya negociado; baja desde DbStudio. */
  t: T;
  locale: Locale;
}

const EMPTY_ROWS: DbRows = { columns: [], rows: [], totalRows: 0 };

/**
 * OJO con el nombre: las tablas se llaman `tbl` en todo este fichero, nunca `t`.
 * `t` es la función de traducción y ensombrecerla en un `map` dejaría los textos
 * de este panel sin traducir sin que el compilador dijera nada.
 */
function tableId(tbl: DbTable): string {
  return `${tbl.schema}.${tbl.name}`;
}

export default function TableBrowser({
  project,
  tables,
  rowsByTable,
  t,
  locale,
}: TableBrowserProps) {
  const [activeId, setActiveId] = useState(tables[0] ? tableId(tables[0]) : '');
  const [filter, setFilter] = useState('');
  // Cache de filas por tabla (arranca con las precargadas por SSR).
  const [cache, setCache] = useState<Record<string, DbRows>>(rowsByTable);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  // Contador de reintentos: relanza el efecto de carga tras un error.
  const [attempt, setAttempt] = useState(0);

  const grouped = useMemo(() => {
    const map = new Map<string, DbTable[]>();
    for (const tbl of tables) {
      if (filter && !contiene(tbl.name, filter)) continue;
      const list = map.get(tbl.schema) ?? [];
      list.push(tbl);
      map.set(tbl.schema, list);
    }
    return map;
  }, [tables, filter]);

  const active = tables.find((tbl) => tableId(tbl) === activeId);

  // Carga bajo demanda de filas cuando la tabla activa no está en caché.
  useEffect(() => {
    if (!active) return;
    if (cache[activeId]) return;
    let cancelled = false;
    setLoading(true);
    setError(false);
    const params = new URLSearchParams({
      project,
      schema: active.schema,
      table: active.name,
      limit: '50',
    });
    fetch(`/api/db/rows?${params.toString()}`)
      .then((res) => (res.ok ? res.json() : Promise.reject(new Error('http'))))
      .then((body: { rows: DbRows }) => {
        if (cancelled) return;
        setCache((prev) => ({ ...prev, [activeId]: body.rows }));
      })
      .catch(() => {
        if (!cancelled) setError(true);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [active, activeId, cache, project, attempt]);

  const rows: DbRows = (active && cache[activeId]) || EMPTY_ROWS;

  // Columnas dinámicas de la tabla de datos.
  const dataColumns: TableColumn<Record<string, unknown>>[] = rows.columns.map((c) => ({
    key: c,
    header: c,
    cell: (row) => {
      const v = row[c];
      if (v === null || v === undefined) return <span class="text-text-subtle italic">NULL</span>;
      return <span class="font-mono text-sm">{cellToString(v)}</span>;
    },
  }));

  // Columnas de la vista "Definición".
  const defColumns: TableColumn<DbTable['columns'][number]>[] = [
    {
      key: 'name',
      header: t('db.col.column'),
      cell: (col) => (
        <span class="flex items-center gap-1.5 font-mono text-sm">
          {col.name}
          {col.isPrimaryKey ? <Badge tone="primary">PK</Badge> : null}
          {col.references ? <Badge tone="accent">FK</Badge> : null}
        </span>
      ),
    },
    {
      key: 'type',
      header: t('db.col.type'),
      cell: (col) => <span class="text-text-muted font-mono text-sm">{col.type}</span>,
    },
    {
      key: 'nullable',
      header: t('db.col.nullable'),
      cell: (col) => (col.nullable ? t('common.yes') : t('common.no')),
    },
    {
      key: 'default',
      header: t('db.col.default'),
      cell: (col) =>
        col.default ? <span class="text-text-muted font-mono text-sm">{col.default}</span> : '—',
    },
    { key: 'references', header: t('db.col.references'), cell: (col) => col.references ?? '—' },
  ];

  return (
    <div class="border-border bg-surface flex min-h-[520px] overflow-hidden rounded-lg border">
      {/* Lista de tablas */}
      <div class="border-border bg-surface-sunken flex w-64 shrink-0 flex-col border-r">
        <div class="border-border border-b p-2">
          <Input
            type="search"
            value={filter}
            placeholder={t('db.filterTables')}
            aria-label={t('db.filterTables')}
            leadingIcon={<Icon name="search" size={16} />}
            onInput={(e) => setFilter(e.currentTarget.value)}
          />
        </div>
        <div class="flex-1 overflow-y-auto p-2">
          {grouped.size === 0 ? (
            <p class="text-text-muted px-2 py-6 text-center text-sm">
              {t('db.noTableMatches', { query: filter })}
            </p>
          ) : null}
          {[...grouped.entries()].map(([schema, list]) => (
            <div key={schema} class="mb-3">
              <p class="sytue-overline text-text-subtle px-2 pb-1">{schema}</p>
              <ul class="flex flex-col gap-0.5">
                {list.map((tbl) => {
                  const id = tableId(tbl);
                  const isActive = id === activeId;
                  return (
                    <li key={id}>
                      <button
                        type="button"
                        onClick={() => setActiveId(id)}
                        aria-current={isActive ? 'true' : undefined}
                        title={id}
                        class={
                          'flex w-full items-center justify-between gap-2 rounded-md border-l-2 px-2 py-1.5 text-left text-base transition-colors ' +
                          // Rol de marca (primary): la fila seleccionada se tiñe con el
                          // latón, no con el rol de estado `info`. Barra izquierda 2px +
                          // relleno `bg-selected` (tinte de marca adaptable a oscuro,
                          // List §3.7); borde transparente si inactiva para no desplazar
                          // el texto.
                          (isActive
                            ? 'border-primary-500 bg-selected text-primary-text font-medium'
                            : 'text-text hover:bg-bg-subtle active:bg-surface-sunken border-transparent')
                        }
                      >
                        <span class="flex min-w-0 items-center gap-1.5">
                          <Icon
                            name="table"
                            size={16}
                            class={isActive ? 'text-primary-text' : 'text-text-subtle'}
                          />
                          <span class="truncate">{tbl.name}</span>
                        </span>
                        <span class="sytue-meta shrink-0 tabular-nums">
                          {formatNumber(locale, tbl.rowCount)}
                        </span>
                      </button>
                    </li>
                  );
                })}
              </ul>
            </div>
          ))}
        </div>
      </div>

      {/* Detalle de la tabla */}
      <div class="flex min-w-0 flex-1 flex-col">
        {active ? (
          <>
            <div class="border-border flex flex-wrap items-center justify-between gap-2 border-b px-4 py-3">
              <div class="flex items-center gap-2">
                <h3 class="sytue-h4 text-text">
                  <span class="text-text-subtle">{active.schema}.</span>
                  {active.name}
                </h3>
                <Badge tone="neutral" class="tabular-nums">
                  {t('db.rows', { count: active.rowCount })}
                </Badge>
                {active.apiExposed ? (
                  <Badge tone="accent">REST</Badge>
                ) : (
                  <Badge tone="warning">{t('db.notExposed')}</Badge>
                )}
              </div>
            </div>
            <div class="flex-1 overflow-auto p-4">
              <Tabs
                items={[
                  {
                    id: 'data',
                    label: t('db.tab.data'),
                    content: loading ? (
                      <div
                        class="text-text-muted flex items-center justify-center gap-2 py-12 text-sm"
                        role="status"
                        aria-live="polite"
                      >
                        <Spinner size={16} /> {t('db.loadingRows', { table: active.name })}
                      </div>
                    ) : error ? (
                      <EmptyState
                        illustration={<Icon name="warning" size={24} />}
                        title={t('db.rowsFail.title')}
                        description={t('db.rowsFail.desc')}
                        action={
                          <Button
                            variant="secondary"
                            size="sm"
                            onClick={() => {
                              setError(false);
                              setAttempt((n) => n + 1);
                            }}
                          >
                            {t('common.tryAgain')}
                          </Button>
                        }
                      />
                    ) : (
                      <>
                        <Table
                          columns={dataColumns}
                          rows={rows.rows}
                          rowKey={(r, i) => cellToString(r[rows.columns[0] ?? 'id']) || `row-${i}`}
                          caption={t('db.rows.caption', { table: active.name })}
                          emptyContent={t('db.rows.empty')}
                        />
                        <p class="text-text-muted mt-2 text-sm tabular-nums">
                          {t('db.rows.showing', {
                            shown: rows.rows.length,
                            total: rows.totalRows,
                          })}
                        </p>
                      </>
                    ),
                  },
                  {
                    id: 'def',
                    label: t('db.tab.definition'),
                    content: (
                      <Table
                        columns={defColumns}
                        rows={active.columns}
                        rowKey={(c) => c.name}
                        caption={t('db.columns.caption', { table: active.name })}
                      />
                    ),
                  },
                ]}
              />
            </div>
          </>
        ) : (
          <div class="grid flex-1 place-items-center">
            <EmptyState
              illustration={<Icon name="table" size={24} />}
              title={t('db.pickTable.title')}
              description={t('db.pickTable.desc')}
            />
          </div>
        )}
      </div>
    </div>
  );
}
