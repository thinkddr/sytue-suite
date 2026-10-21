/**
 * AuditPanel — quién hizo qué en la organización.
 *
 * Los filtros y la paginación los resuelve el CONTROL-PLANE (`/api/audit`), no el
 * navegador: quien audita busca algo de hace semanas y filtrar sobre «los últimos
 * 50» —que es lo que hacía esta pantalla— responde a la pregunta equivocada. Las
 * páginas siguientes se piden por cursor keyset (nunca OFFSET), así que el coste no
 * crece con la profundidad, y el botón de CSV descarga el MISMO conjunto filtrado
 * que se está viendo (lo que pide una auditoría SOC2/ISO: la evidencia, no una
 * captura de pantalla).
 *
 * Las acciones se traducen a lenguaje llano: «token.create» no le dice nada a
 * nadie que no haya escrito el backend.
 */
import { useState } from 'preact/hooks';
import { Badge, Button, EmptyState, Icon, Input, SectionHeader, Table } from '@sytue/ui';
import type { TableColumn } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { formatDateTime } from '@sytue/i18n';
import type { AuditEvent } from '../lib/cloud.js';
import type { T } from '../i18n/index.js';
import { useT } from '../i18n/index.js';
import DemoBanner from './DemoBanner.js';

/**
 * Acción del backend → qué significa, en lenguaje llano y en el idioma de quien
 * mira. El identificador («token.create») es contrato del backend y NO se
 * traduce: sigue viéndose en monoespaciada debajo de la explicación.
 */
function actionLabels(t: T): Record<string, string> {
  return {
    'member.upsert': t('audit.action.member.upsert'),
    'member.remove': t('audit.action.member.remove'),
    'token.create': t('audit.action.token.create'),
    'token.revoke': t('audit.action.token.revoke'),
    'project.create': t('audit.action.project.create'),
    'project.delete': t('audit.action.project.delete'),
    'domain.remove': t('audit.action.domain.remove'),
    'domain.set_primary': t('audit.action.domain.set_primary'),
    'deploy.byo': t('audit.action.deploy.byo'),
    'build.cancel': t('audit.action.build.cancel'),
    'service.restore': t('audit.action.service.restore'),
    'billing.set_plan': t('audit.action.billing.set_plan'),
    'billing.set_spend_limit': t('audit.action.billing.set_spend_limit'),
    'billing.generate_invoice': t('audit.action.billing.generate_invoice'),
    'billing.payment_status': t('audit.action.billing.payment_status'),
    'github.connect': t('audit.action.github.connect'),
    'github.disconnect': t('audit.action.github.disconnect'),
  };
}

/** Acciones sensibles: identidad, credenciales y borrados. Se destacan. */
const SENSITIVE = new Set([
  'member.upsert',
  'member.remove',
  'token.create',
  'token.revoke',
  'project.delete',
  'service.restore',
]);

/** Filtros que el servidor entiende. El backend casa `action` por PREFIJO. */
interface Filters {
  actor: string;
  action: string;
  since: string;
  until: string;
}

const EMPTY: Filters = { actor: '', action: '', since: '', until: '' };

/** Query compartida por el listado y por el export: el CSV es lo que se ve. */
function toParams(f: Filters): URLSearchParams {
  const p = new URLSearchParams();
  if (f.actor.trim()) p.set('actor', f.actor.trim());
  if (f.action.trim()) p.set('action', f.action.trim());
  if (f.since) p.set('since', f.since);
  if (f.until) p.set('until', f.until);
  return p;
}

function detailText(detail: Record<string, unknown>): string {
  return Object.entries(detail)
    .map(([k, v]) => `${k}: ${String(v)}`)
    .join(' · ');
}

interface Props {
  events: AuditEvent[];
  /** Cursor de la primera página (vacío si no hay más). */
  nextCursor: string;
  live: boolean;
  error?: { status: number; code: string } | undefined;
  /** Idioma ya negociado en el servidor (llega por props, nunca se detecta aquí). */
  locale: Locale;
}

export default function AuditPanel({ events, nextCursor, live, error, locale }: Props) {
  const t = useT(locale);
  const actionLabel = actionLabels(t);
  const [items, setItems] = useState<AuditEvent[]>(events);
  const [cursor, setCursor] = useState(nextCursor);
  const [filters, setFilters] = useState<Filters>(EMPTY);
  const [loading, setLoading] = useState(false);
  const [failed, setFailed] = useState(false);

  const params = toParams(filters);
  const filtered = params.toString() !== '';
  const csvHref = `/api/audit?format=csv${filtered ? `&${params.toString()}` : ''}`;

  const set = (k: keyof Filters) => (e: Event) =>
    setFilters({ ...filters, [k]: (e.target as HTMLInputElement).value });

  /** Carga una página. `after` vacío reinicia el listado; con cursor, añade. */
  async function load(after: string, f: Filters = filters) {
    if (loading) return;
    setLoading(true);
    setFailed(false);
    try {
      const p = toParams(f);
      if (after) p.set('cursor', after);
      const res = await fetch(`/api/audit?${p.toString()}`, {
        headers: { accept: 'application/json' },
      });
      if (!res.ok) throw new Error(String(res.status));
      const data = (await res.json()) as { events?: AuditEvent[]; nextCursor?: string };
      setItems(after ? [...items, ...(data.events ?? [])] : (data.events ?? []));
      setCursor(data.nextCursor ?? '');
    } catch {
      // Decirlo y dejar lo ya cargado: vaciar la tabla haría creer que no hay traza.
      setFailed(true);
    } finally {
      setLoading(false);
    }
  }

  function clear() {
    setFilters(EMPTY);
    void load('', EMPTY);
  }

  const columns: TableColumn<AuditEvent>[] = [
    {
      key: 'occurredAt',
      header: t('audit.col.date'),
      width: '11rem',
      cell: (e) => (
        <span class="text-text-muted whitespace-nowrap tabular-nums">
          {e.occurredAt
            ? formatDateTime(locale, e.occurredAt, { dateStyle: 'short', timeStyle: 'short' })
            : '—'}
        </span>
      ),
    },
    {
      key: 'action',
      header: t('audit.col.action'),
      cell: (e) => (
        <span class="block">
          <span class="text-text">
            {/* Una acción nueva del backend se muestra con su identificador tal
                cual: mejor eso que ocultar el evento. */}
            {actionLabel[e.action] ?? e.action}
          </span>
          <span class="sytue-meta block truncate font-mono" title={e.action}>
            {e.action}
            {detailText(e.detail) ? ` · ${detailText(e.detail)}` : ''}
          </span>
        </span>
      ),
    },
    {
      key: 'actorSub',
      header: t('audit.col.actor'),
      width: '16rem',
      cell: (e) => (
        <span class="text-text block truncate font-mono text-xs" title={e.actorSub}>
          {e.actorSub || t('audit.system')}
        </span>
      ),
    },
    {
      key: 'resource',
      header: t('audit.col.resource'),
      width: '14rem',
      cell: (e) => (
        <span class="flex items-center gap-2">
          <span class="text-text-muted min-w-0 truncate font-mono text-xs" title={e.resource}>
            {e.resource || '—'}
          </span>
          {SENSITIVE.has(e.action) ? <Badge tone="warning">{t('audit.sensitive')}</Badge> : null}
        </span>
      ),
    },
  ];

  return (
    <>
      <DemoBanner live={live} locale={locale} error={error} />
      <SectionHeader
        icon={<Icon name="audit" size={20} />}
        title={t('nav.audit')}
        subtitle={t('audit.subtitle')}
        level={1}
      />

      <div class="cloud-content">
        <form
          class="mb-3 flex flex-wrap items-end gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            void load('');
          }}
        >
          <Input
            type="search"
            value={filters.actor}
            placeholder={t('audit.filter.actor')}
            aria-label={t('audit.filter.actorLabel')}
            leadingIcon={<Icon name="search" size={16} />}
            class="w-56"
            onInput={set('actor')}
          />
          {/* Sugerencias NATIVAS (datalist): guía las acciones conocidas sin impedir
              escribir un prefijo cualquiera («billing.» = todo lo de facturación). */}
          <Input
            type="search"
            value={filters.action}
            list="audit-actions"
            placeholder={t('audit.filter.action')}
            aria-label={t('audit.filter.actionLabel')}
            class="w-52"
            onInput={set('action')}
          />
          <datalist id="audit-actions">
            {Object.keys(actionLabel).map((a) => (
              <option key={a} value={a}>
                {actionLabel[a]}
              </option>
            ))}
          </datalist>
          <Input
            type="date"
            value={filters.since}
            aria-label={t('audit.filter.since')}
            class="w-40"
            onInput={set('since')}
          />
          <Input
            type="date"
            value={filters.until}
            aria-label={t('audit.filter.until')}
            class="w-40"
            onInput={set('until')}
          />
          <Button type="submit" variant="secondary" loading={loading}>
            {t('audit.apply')}
          </Button>
          {filtered ? (
            <Button type="button" variant="ghost" onClick={clear}>
              {t('common.clear')}
            </Button>
          ) : null}
          {/* Enlace real (no un botón con JS): se puede abrir en otra pestaña y el
              navegador descarga con el nombre que compone el control-plane. `href` en
              `Button` da eso mismo sin copiar las clases: antes iba a mano y ya se
              había desviado del canónico. */}
          <Button href={csvHref} variant="secondary" class="ml-auto">
            {t('audit.exportCsv')}
          </Button>
        </form>

        {failed ? (
          <p class="bg-danger-bg text-danger-text mb-3 rounded-md px-3 py-2 text-sm" role="alert">
            {t('audit.loadFail')}
          </p>
        ) : null}

        {items.length === 0 ? (
          <EmptyState
            illustration={<Icon name="audit" size={24} />}
            title={filtered ? t('audit.empty.filtered.title') : t('audit.empty.title')}
            description={filtered ? t('audit.empty.filtered.desc') : t('audit.empty.desc')}
            action={
              filtered ? (
                <Button variant="secondary" size="sm" onClick={clear}>
                  {t('audit.clearFilters')}
                </Button>
              ) : undefined
            }
          />
        ) : (
          <>
            <Table
              columns={columns}
              rows={items}
              rowKey={(e, i) => e.id || `${e.occurredAt}-${i}`}
              caption={t('audit.caption')}
            />
            {cursor ? (
              <div class="mt-3 flex justify-center">
                <Button variant="secondary" loading={loading} onClick={() => void load(cursor)}>
                  {t('audit.loadMore')}
                </Button>
              </div>
            ) : null}
          </>
        )}
      </div>
    </>
  );
}
