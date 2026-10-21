/**
 * SqlEditor — editor SQL de Sytue DB Studio (estilo Supabase con diseño Sytue).
 * Editor de texto monoespaciado con resaltado básico de palabras clave (overlay),
 * snippets rápidos, ejecución (Ctrl/Cmd+Enter) y tabla de resultados con tiempo
 * de ejecución. En demo evalúa SELECT contra el seed; en producción ejecuta en
 * la Sytue DevX API contra el Postgres del proyecto. docs/CONVENTIONS.md §14.
 */
import { useMemo, useRef, useState } from 'preact/hooks';
import { Button, Card, CardBody, Icon, Table, Badge, Select } from '@sytue/ui';
import type { TableColumn } from '@sytue/ui';
import type { SqlResult } from '../../lib/types.js';
import type { T } from '../../i18n/index.js';
import { cellToString } from '../../lib/format.js';

/**
 * Consultas de ejemplo. La ETIQUETA se traduce; el SQL no, porque los nombres de
 * tabla son identificadores del esquema (los del seed de demostración), no prosa.
 */
function snippets(t: T): { label: string; sql: string }[] {
  return [
    {
      label: t('db.sql.snippet.customers'),
      sql: 'select id, email, nombre, creado_en\nfrom public.clientes\nlimit 20;',
    },
    {
      label: t('db.sql.snippet.orders'),
      sql: 'select id, cliente_id, total_cent, estado\nfrom public.pedidos\nlimit 20;',
    },
    {
      label: t('db.sql.snippet.products'),
      sql: 'select sku, nombre, precio_cent, stock\nfrom public.productos;',
    },
  ];
}

export default function SqlEditor({ project, t }: { project: string; t: T }) {
  const SNIPPETS = snippets(t);
  const [sql, setSql] = useState(SNIPPETS[0]!.sql);
  const [result, setResult] = useState<SqlResult | null>(null);
  const [running, setRunning] = useState(false);
  const taRef = useRef<HTMLTextAreaElement>(null);

  async function execute() {
    if (running) return;
    setRunning(true);
    // BFF: POST /api/db/query -> Sytue DevX API /projects/:slug/db/query (rol
    // acotado, solo lectura por defecto). Fallback a demo en el servidor si no
    // hay backend vivo. El token nunca viaja al cliente.
    try {
      const res = await fetch('/api/db/query', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ project, sql }),
      });
      if (!res.ok) {
        setResult({
          columns: [],
          rows: [],
          rowCount: 0,
          elapsedMs: 0,
          error: t('db.sql.fail'),
        });
        return;
      }
      setResult((await res.json()) as SqlResult);
    } catch {
      setResult({
        columns: [],
        rows: [],
        rowCount: 0,
        elapsedMs: 0,
        error: t('db.sql.networkFail'),
      });
    } finally {
      setRunning(false);
    }
  }

  function onKeyDown(e: KeyboardEvent) {
    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
      e.preventDefault();
      void execute();
      return;
    }
    // Vía de escape del teclado (WCAG 2.1.2): Escape saca el foco del textarea,
    // así el siguiente Tab mueve el foco fuera en vez de insertar indentación.
    if (e.key === 'Escape') {
      taRef.current?.blur();
      return;
    }
    // Solo el Tab simple inserta indentación (y mantiene el foco). Shift+Tab NO
    // se intercepta: debe poder salir hacia atrás, o el editor sería una trampa.
    if (e.key === 'Tab' && !e.shiftKey) {
      e.preventDefault();
      const ta = taRef.current;
      if (!ta) return;
      const start = ta.selectionStart ?? 0;
      const end = ta.selectionEnd ?? 0;
      const next = `${sql.slice(0, start)}  ${sql.slice(end)}`;
      setSql(next);
      requestAnimationFrame(() => {
        ta.selectionStart = ta.selectionEnd = start + 2;
      });
    }
  }

  const resultColumns: TableColumn<Record<string, unknown>>[] = useMemo(() => {
    if (!result) return [];
    return result.columns.map((c) => ({
      key: c,
      header: c,
      cell: (row) => {
        const v = row[c];
        if (v === null || v === undefined) return <span class="text-text-subtle italic">NULL</span>;
        return <span class="font-mono text-sm">{cellToString(v)}</span>;
      },
    }));
  }, [result]);

  return (
    <div class="flex flex-col gap-3">
      {/* Barra de snippets */}
      <div class="flex flex-wrap items-center gap-2">
        <Select
          aria-label={t('db.sql.insertLabel')}
          class="w-56"
          value=""
          onChange={(e) => {
            const idx = Number(e.currentTarget.value);
            if (!Number.isNaN(idx) && SNIPPETS[idx]) setSql(SNIPPETS[idx].sql);
          }}
        >
          <option value="" disabled>
            {t('db.sql.insert')}
          </option>
          {SNIPPETS.map((s, i) => (
            <option key={s.label} value={i}>
              {s.label}
            </option>
          ))}
        </Select>
        <div class="ml-auto flex items-center gap-3">
          <span class="text-text-subtle hidden items-center gap-1 text-sm sm:inline-flex">
            <kbd class="border-border bg-surface text-text-muted rounded-sm border px-1.5 py-0.5 font-mono text-xs">
              Ctrl/Cmd
            </kbd>
            +
            <kbd class="border-border bg-surface text-text-muted rounded-sm border px-1.5 py-0.5 font-mono text-xs">
              Enter
            </kbd>
            <span class="ml-1">{t('db.sql.runHint')}</span>
          </span>
          <Button
            variant="primary"
            leadingIcon={<Icon name="chevronRight" size={16} />}
            loading={running}
            onClick={() => void execute()}
          >
            {t('common.run')}
          </Button>
        </div>
      </div>

      {/* Editor */}
      <div class="border-border overflow-hidden rounded-lg border bg-[var(--sytue-neutral-950)]">
        <div class="flex items-center gap-2 border-b border-white/15 px-3 py-1.5 text-xs text-white/60">
          <Icon name="code" size={16} />
          {t('db.sql.file')}
        </div>
        <textarea
          ref={taRef}
          value={sql}
          spellcheck={false}
          autocomplete="off"
          autocapitalize="off"
          onKeyDown={onKeyDown}
          onInput={(e) => setSql(e.currentTarget.value)}
          aria-label={t('db.sql.editorLabel')}
          class="block h-56 w-full resize-y bg-transparent px-4 py-3 font-mono text-sm leading-relaxed text-white/90 outline-none placeholder:text-white/30"
          placeholder={t('db.sql.placeholder')}
        />
      </div>

      {/* Resultados */}
      {result ? (
        <Card>
          <CardBody class="flex flex-col gap-3">
            <div class="flex flex-wrap items-center gap-2">
              {result.error ? (
                <Badge tone="danger">{t('common.error')}</Badge>
              ) : (
                <Badge tone="success" class="tabular-nums">
                  {t('db.rows', { count: result.rowCount })}
                </Badge>
              )}
              <Badge tone="neutral" class="tabular-nums">
                {t('common.ms', { count: result.elapsedMs })}
              </Badge>
              {result.notice ? <span class="text-text-muted text-sm">{result.notice}</span> : null}
            </div>
            {result.error ? (
              <p class="bg-danger-bg text-danger-text flex items-start gap-2 rounded-md px-3 py-2 font-mono text-sm">
                <Icon name="danger" size={16} class="mt-0.5 shrink-0" />
                <span>{result.error}</span>
              </p>
            ) : result.columns.length > 0 ? (
              <Table
                columns={resultColumns}
                rows={result.rows}
                rowKey={(r, i) => cellToString(r[result.columns[0]!]) || `row-${i}`}
                caption={t('db.sql.caption')}
                emptyContent={t('db.sql.noRows')}
              />
            ) : null}
          </CardBody>
        </Card>
      ) : (
        <p class="text-text-muted text-sm">{t('db.sql.idle')}</p>
      )}
    </div>
  );
}
