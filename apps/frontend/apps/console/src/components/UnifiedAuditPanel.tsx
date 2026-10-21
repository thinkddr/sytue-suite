/**
 * UnifiedAuditPanel — Auditoría UNIFICADA de Sytue Admin: agrega la traza de todos
 * los servicios (Identidad, Correo, Archivos, Integraciones) en una sola tabla,
 * etiquetada por servicio y ordenada por fecha DESC. El fan-out best-effort vive en
 * el BFF (lib/audit-siem.ts): una llamada por servicio; si una fuente cae, se avisa
 * sin romper el resto. Filtros (servicio, acción, actor, rango de fechas) y export
 * (CSV y JSON, esquema SIEM normalizado: ts, service, actor, action, target, detail)
 * se aplican sobre el conjunto agregado. Filas VIRTUALIZADAS con VirtualList de
 * @sytue/ui. El token nunca está aquí (va por el BFF). docs/CONVENTIONS.md §10, §11.
 *
 * FOLLOW-UP: esto es el export *pull*. El push en tiempo real a un SIEM/syslog
 * (forwarder que emita cada evento a un colector configurado) es una ola posterior;
 * ver la nota en lib/audit-siem.ts. El esquema de aquí es el que ese forwarder emitiría.
 */
import { useMemo, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  EmptyState,
  Icon,
  Input,
  SectionHeader,
  Select,
  VirtualList,
  withBase,
} from '@sytue/ui';
import type { BadgeTone } from '@sytue/ui';
import { formatDateTime, formatList, type Locale } from '@sytue/i18n';
import { useT, type ConsoleMessageKey, type T } from '../i18n/index.js';
import type { AuditSource, SiemRecord, SourceStatus } from '../lib/audit-siem.js';

// La CLAVE de cada fuente se declara aquí: lib/audit-siem.ts es módulo SOLO servidor,
// así que solo se importan sus TIPOS (import type se borra en el build del cliente).
const SOURCE_LABEL_KEY: Record<AuditSource, ConsoleMessageKey> = {
  id: 'audit.source.id',
  mail: 'audit.source.mail',
  drive: 'audit.source.drive',
  notes: 'audit.source.notes',
  integrations: 'audit.source.integrations',
};

// Colores para distinguir la fuente de un vistazo (ninguno implica estado/alarma).
const SOURCE_TONE: Record<AuditSource, BadgeTone> = {
  id: 'info',
  mail: 'accent',
  drive: 'success',
  notes: 'primary',
  integrations: 'neutral',
};

/** Texto compacto del detalle (JSON) para tooltip/subtítulo; '' si no hay. */
function detailText(detail: Record<string, unknown>): string {
  return detail && Object.keys(detail).length > 0 ? JSON.stringify(detail) : '';
}

/** Fila normalizada (la pinta y mide VirtualList). Misma retícula que la cabecera. */
function AuditRow({ record: r, locale, t }: { record: SiemRecord; locale: Locale; t: T }) {
  const detail = detailText(r.detail);
  return (
    <div class="border-border hover:bg-bg-subtle flex items-center gap-3 border-b px-3 py-2.5 transition-colors">
      <span class="text-text-muted w-36 shrink-0 text-sm whitespace-nowrap tabular-nums">
        {formatDateTime(locale, r.ts)}
      </span>
      {/* Servicio fuera en <640px: con Fecha (144px) y Servicio (112px) fijos, a
          la Acción —la columna que se lee— no le quedaba ancho en un teléfono. */}
      <span class="w-28 shrink-0 max-sm:hidden">
        <Badge tone={SOURCE_TONE[r.service]}>{t(SOURCE_LABEL_KEY[r.service])}</Badge>
      </span>
      <span class="min-w-0 flex-1">
        <span class="text-text block truncate font-mono text-sm" title={r.action}>
          {r.action}
        </span>
        {detail ? (
          <span class="sytue-meta block truncate" title={detail}>
            {detail}
          </span>
        ) : null}
      </span>
      <span class="text-text hidden w-52 shrink-0 truncate lg:block" title={r.actor}>
        {r.actor || '—'}
      </span>
      <span class="text-text-muted hidden w-44 shrink-0 truncate xl:block" title={r.target}>
        {r.target || '—'}
      </span>
    </div>
  );
}

interface UnifiedAuditPanelProps {
  records: SiemRecord[];
  sources: SourceStatus[];
  /** false = ninguna fuente respondió (sin backend vivo). */
  live?: boolean;
  /** Idioma ya negociado en el servidor; llega por props. */
  locale: Locale;
}

const SOURCE_OPTIONS: { value: '' | AuditSource; labelKey: ConsoleMessageKey }[] = [
  { value: '', labelKey: 'audit.source.all' },
  { value: 'id', labelKey: 'audit.source.id' },
  { value: 'mail', labelKey: 'audit.source.mail' },
  { value: 'drive', labelKey: 'audit.source.drive' },
  { value: 'notes', labelKey: 'audit.source.notes' },
  { value: 'integrations', labelKey: 'audit.source.integrations' },
];

export default function UnifiedAuditPanel({
  records,
  sources,
  live = true,
  locale,
}: UnifiedAuditPanelProps) {
  const t = useT(locale);
  const [items, setItems] = useState<SiemRecord[]>(records);
  const [status, setStatus] = useState<SourceStatus[]>(sources);
  const [service, setService] = useState<'' | AuditSource>('');
  const [action, setAction] = useState('');
  const [actor, setActor] = useState('');
  const [since, setSince] = useState('');
  const [until, setUntil] = useState('');
  const [loading, setLoading] = useState(false);
  const [failed, setFailed] = useState(false);

  const filterParams = useMemo(() => {
    const p = new URLSearchParams();
    if (service) p.set('service', service);
    if (action.trim()) p.set('action', action.trim());
    if (actor.trim()) p.set('actor', actor.trim());
    if (since) p.set('since', since);
    if (until) p.set('until', until);
    return p;
  }, [service, action, actor, since, until]);

  const exportCsvHref = useMemo(() => {
    const qs = filterParams.toString();
    return withBase(
      import.meta.env.BASE_URL,
      `/api/admin/siem-audit/export.csv${qs ? `?${qs}` : ''}`,
    );
  }, [filterParams]);
  const exportJsonHref = useMemo(() => {
    const qs = filterParams.toString();
    return withBase(
      import.meta.env.BASE_URL,
      `/api/admin/siem-audit/export.json${qs ? `?${qs}` : ''}`,
    );
  }, [filterParams]);

  // Fuentes que NO respondieron (para avisar de una laguna sin romper la vista).
  const downSources = useMemo(() => status.filter((s) => !s.ok).map((s) => s.source), [status]);

  async function apply(params: URLSearchParams = filterParams) {
    if (loading) return;
    setLoading(true);
    setFailed(false);
    try {
      const res = await fetch(`/api/admin/siem-audit?${params.toString()}`, {
        headers: { accept: 'application/json' },
      });
      if (!res.ok) throw new Error(String(res.status));
      const data = (await res.json()) as { records?: SiemRecord[]; sources?: SourceStatus[] };
      setItems(data.records ?? []);
      if (Array.isArray(data.sources)) setStatus(data.sources);
    } catch {
      setFailed(true);
    } finally {
      setLoading(false);
    }
  }

  function clear() {
    setService('');
    setAction('');
    setActor('');
    setSince('');
    setUntil('');
    void apply(new URLSearchParams());
  }

  const getKey = useMemo(
    () => (i: number) => `${items[i]?.service ?? ''}:${items[i]?.ts ?? ''}:${i}`,
    [items],
  );

  return (
    // h-full + min-h-0: la lista virtualizada necesita altura acotada; el scroll vive
    // DENTRO del VirtualList, no en el main.
    <div class="flex h-full min-h-0 flex-col">
      <SectionHeader
        level={1}
        class="shrink-0"
        title={t('audit.title')}
        subtitle={t('audit.subtitle')}
      />

      <form
        class="border-border bg-surface flex shrink-0 flex-wrap items-end gap-2 border-b px-4 py-2.5"
        onSubmit={(e) => {
          e.preventDefault();
          void apply();
        }}
      >
        <Select
          value={service}
          aria-label={t('audit.filterService')}
          class="w-48"
          onChange={(e) => setService(e.currentTarget.value as '' | AuditSource)}
        >
          {SOURCE_OPTIONS.map((o) => (
            <option key={o.value} value={o.value}>
              {t(o.labelKey)}
            </option>
          ))}
        </Select>
        <Input
          type="search"
          value={action}
          placeholder={t('audit.actionPlaceholder')}
          aria-label={t('audit.filterAction')}
          leadingIcon={<Icon name="search" size={16} />}
          class="w-56"
          onInput={(e) => setAction(e.currentTarget.value)}
        />
        <Input
          type="search"
          value={actor}
          placeholder={t('audit.actorPlaceholder')}
          aria-label={t('audit.filterActor')}
          class="w-48"
          onInput={(e) => setActor(e.currentTarget.value)}
        />
        <Input
          type="date"
          value={since}
          aria-label={t('audit.since')}
          class="w-40"
          onInput={(e) => setSince(e.currentTarget.value)}
        />
        <Input
          type="date"
          value={until}
          aria-label={t('audit.until')}
          class="w-40"
          onInput={(e) => setUntil(e.currentTarget.value)}
        />
        <Button type="submit" variant="secondary" disabled={loading}>
          {loading ? t('audit.applying') : t('audit.apply')}
        </Button>
        <div class="ml-auto flex items-center gap-2">
          <a
            href={exportCsvHref}
            class="border-border bg-surface text-text hover:bg-bg-subtle sytue-focus-ring rounded-control inline-flex items-center gap-1.5 border px-3 py-1.5 text-sm font-semibold"
          >
            <Icon name="audit" size={16} aria-hidden="true" /> CSV
          </a>
          <a
            href={exportJsonHref}
            class="border-border bg-surface text-text hover:bg-bg-subtle sytue-focus-ring rounded-control inline-flex items-center gap-1.5 border px-3 py-1.5 text-sm font-semibold"
          >
            <Icon name="audit" size={16} aria-hidden="true" /> JSON
          </a>
        </div>
      </form>

      {!live ? (
        <p class="bg-warning-bg text-warning-text shrink-0 px-4 py-1.5 text-xs">
          {t('audit.allSourcesDown')}
        </p>
      ) : downSources.length > 0 ? (
        <p class="bg-warning-bg text-warning-text shrink-0 px-4 py-1.5 text-xs">
          {t('audit.someSourcesDown', {
            sources: formatList(
              locale,
              downSources.map((s) => t(SOURCE_LABEL_KEY[s])),
            ),
          })}
        </p>
      ) : null}
      {failed ? (
        <p class="bg-danger-bg text-danger-text shrink-0 px-4 py-1.5 text-xs">
          {t('audit.loadFailed')}
        </p>
      ) : null}

      <div class="border-border bg-surface m-4 flex min-h-0 flex-col overflow-hidden rounded-lg border">
        <div
          aria-hidden="true"
          class="border-border bg-surface-sunken text-text-muted sytue-overline flex shrink-0 items-center gap-3 border-b px-3 py-2.5"
        >
          <span class="w-36 shrink-0">{t('audit.col.date')}</span>
          <span class="w-28 shrink-0 max-sm:hidden">{t('audit.col.service')}</span>
          <span class="min-w-0 flex-1">{t('audit.col.action')}</span>
          <span class="hidden w-52 shrink-0 lg:block">{t('audit.col.actor')}</span>
          <span class="hidden w-44 shrink-0 xl:block">{t('audit.col.target')}</span>
        </div>
        {items.length === 0 ? (
          <EmptyState
            title={t('audit.empty.title')}
            description={t('audit.empty.description')}
            illustration={<Icon name="audit" size={28} />}
            action={
              <Button variant="secondary" size="sm" onClick={clear}>
                {t('users.empty.clearFilters')}
              </Button>
            }
          />
        ) : (
          <VirtualList
            items={items}
            estimateSize={52}
            getItemKey={getKey}
            aria-label={t('audit.listLabel')}
            class="min-h-0 text-base"
            renderItem={(r) => <AuditRow record={r} locale={locale} t={t} />}
          />
        )}
      </div>
    </div>
  );
}
