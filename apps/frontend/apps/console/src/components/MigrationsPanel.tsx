/**
 * MigrationsPanel — «Traer contenido de Microsoft 365 o Google» como página de ajustes.
 *
 * Tres estados: el asistente (MigrateWizard), la lista de traslados con sus
 * contadores EN VIVO y el informe final de cada uno (qué se omitió y por qué).
 *
 * HONESTIDAD DE LA BARRA DE PROGRESO: el servicio NO sabe cuántas entidades hay en
 * el origen hasta que termina de recorrerlo, así que aquí no hay porcentajes ni
 * barras que se inventen un total. Se enseñan los contadores reales, que suben
 * mientras el trabajo corre. Una barra que miente es peor que no tener barra.
 */
import { useEffect, useState } from 'preact/hooks';
import {
  Badge,
  Button,
  Card,
  CardBody,
  EmptyState,
  formatBytes,
  Icon,
  InlineHelp,
  Modal,
  SettingsSection,
  SettingsShell,
  Spinner,
  ToastProvider,
  useToast,
} from '@sytue/ui';
import { formatNumber, type Locale } from '@sytue/i18n';
import MigrateWizard from './MigrateWizard.js';
import type { MigrationSourceOption } from './MigrateWizard.js';
import AutoMigrateWizard from './AutoMigrateWizard.js';
import type { AutoSource } from './AutoMigrateWizard.js';
import { useT, type T } from '../i18n/index.js';
import type { MigrationJob, MigrationSkipped } from '../lib/gateway.js';

interface MigrationsPanelProps {
  /** Idioma negociado en el servidor; llega por props desde el `.astro`. */
  locale: Locale;
  sources: MigrationSourceOption[];
  /** Orígenes del asistente AUTOMÁTICO (los que se conectan con un login). */
  autoSources: AutoSource[];
  /** Instancia que acaba de volver del consentimiento (?instance=…&oauth=connected). */
  justConnected?: string;
  /** Ese consentimiento era el de escritura: se vuelve a la salida, no a traer. */
  justAuthorizedOut?: boolean;
  jobs: MigrationJob[];
  /** false = gateway no accesible; el panel avisa y no edita. */
  live: boolean;
}

/** Cada cuánto se refresca el progreso mientras haya algo corriendo. */
const POLL_MS = 5000;

const DOMAIN_KEY = {
  drive: 'migrations.domain.drive',
  contacts: 'migrations.domain.contacts',
  repos: 'migrations.domain.repos',
  gists: 'migrations.domain.gists',
  teams: 'migrations.domain.teams',
  team_files: 'migrations.domain.team_files',
  mail: 'migrations.domain.mail',
  calendar: 'migrations.domain.calendar',
  cloud: 'migrations.domain.cloud',
} as const satisfies Record<MigrationJob['domain'], string>;

function statusBadge(status: MigrationJob['status'], t: T) {
  switch (status) {
    case 'done':
      return <Badge tone="success">{t('migrations.status.done')}</Badge>;
    case 'running':
      return <Badge tone="accent">{t('migrations.status.running')}</Badge>;
    case 'queued':
      return <Badge tone="neutral">{t('migrations.status.queued')}</Badge>;
    case 'canceled':
      return <Badge tone="neutral">{t('migrations.status.canceled')}</Badge>;
    default:
      return <Badge tone="danger">{t('migrations.status.error')}</Badge>;
  }
}

/** Una línea de contadores REALES (sin porcentaje: no hay total que conocer). */
function counters(job: MigrationJob, t: T, locale: Locale): string {
  const parts = [t('migrations.counts.migrated', { count: job.counts.migrated })];
  if (job.counts.skipped > 0)
    parts.push(t('migrations.counts.skipped', { count: job.counts.skipped }));
  if (job.counts.errors > 0)
    parts.push(t('migrations.counts.errors', { count: job.counts.errors }));
  if (job.counts.bytesCopied > 0) parts.push(formatBytes(locale, job.counts.bytesCopied));
  return parts.join(' · ');
}

function Inner({
  locale,
  sources,
  autoSources,
  justConnected,
  justAuthorizedOut,
  jobs,
  live,
}: MigrationsPanelProps) {
  const t = useT(locale);
  const toast = useToast();
  const [items, setItems] = useState<MigrationJob[]>(jobs);
  // Volver del consentimiento abre directamente el asistente automático en el paso
  // del plan: la persona ya dijo que sí, no tiene que volver a empezar.
  const [wizard, setWizard] = useState<'auto' | 'manual' | 'out' | 'self' | null>(
    justAuthorizedOut ? 'self' : justConnected ? 'auto' : null,
  );
  const [busyId, setBusyId] = useState<string | null>(null);
  const [report, setReport] = useState<{ job: MigrationJob; skipped: MigrationSkipped[] } | null>(
    null,
  );
  const [loadingReport, setLoadingReport] = useState(false);

  const active = items.filter((j) => j.status === 'queued' || j.status === 'running');
  const finished = items.filter((j) => j.status !== 'queued' && j.status !== 'running');

  // Refresco en vivo mientras haya trabajos sin terminar. Se para solo: sin nada
  // corriendo no se consulta al servicio (ni se calienta el móvil de nadie).
  useEffect(() => {
    if (!live || active.length === 0) return undefined;
    const id = setInterval(() => {
      void fetch('/api/admin/migrations')
        .then((r) => (r.ok ? r.json() : null))
        .then((out: { migrations?: MigrationJob[] } | null) => {
          if (out?.migrations) setItems(out.migrations);
        })
        .catch(() => undefined);
    }, POLL_MS);
    return () => clearInterval(id);
  }, [live, active.length]);

  async function act(job: MigrationJob, action: 'cancel' | 'resume') {
    setBusyId(job.id);
    try {
      const res = await fetch(`/api/admin/migrations/${encodeURIComponent(job.id)}?do=${action}`, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: '{}',
      });
      if (!res.ok) throw new Error(String(res.status));
      setItems((prev) =>
        prev.map((j) =>
          j.id === job.id ? { ...j, status: action === 'cancel' ? 'canceled' : 'queued' } : j,
        ),
      );
      toast.notify({
        tone: 'success',
        title: t(action === 'cancel' ? 'migrations.toast.stopped' : 'migrations.toast.resumed'),
        description: t(
          action === 'cancel' ? 'migrations.toast.stoppedHint' : 'migrations.toast.resumedHint',
        ),
      });
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('migrations.toast.actionFailed'),
        description: t('common.tryAgain'),
      });
    } finally {
      setBusyId(null);
    }
  }

  async function openReport(job: MigrationJob) {
    setLoadingReport(true);
    setReport({ job, skipped: [] });
    try {
      const out = (await fetch(`/api/admin/migrations/${encodeURIComponent(job.id)}`).then((r) =>
        r.ok ? r.json() : Promise.reject(new Error(String(r.status))),
      )) as { migration: MigrationJob; skipped: MigrationSkipped[] };
      setReport({ job: out.migration, skipped: out.skipped ?? [] });
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('migrations.toast.reportFailed'),
        description: t('common.tryAgain'),
      });
      setReport(null);
    } finally {
      setLoadingReport(false);
    }
  }

  // Función de render, NO un componente: declarada dentro de `Inner`, su
  // identidad cambia en cada render y Preact compara vnodes por `type`, así que
  // como componente desmontaba y remontaba TODAS las filas cada vez — y con el
  // sondeo vivo de POLL_MS eso es cada 5 segundos mientras haya un trabajo en
  // curso. Invocada, el vnode que sale es el `<li>`, cuyo tipo sí es estable.
  // (Por eso tampoco puede tener hooks; si algún día los necesita, hay que
  // sacarla del cuerpo de `Inner` y pasarle lo que use por props.)
  function renderRow(job: MigrationJob) {
    const busy = busyId === job.id;
    const running = job.status === 'running' || job.status === 'queued';
    return (
      <li key={job.id}>
        <Card>
          <CardBody class="flex flex-wrap items-center gap-3">
            <span class="min-w-0 flex-1">
              <span class="flex flex-wrap items-center gap-2">
                <span class="text-text truncate font-semibold">{job.targetUser}</span>
                {statusBadge(job.status, t)}
                <Badge tone="neutral">{t(DOMAIN_KEY[job.domain])}</Badge>
              </span>
              <span class="text-text-muted mt-0.5 block text-sm leading-snug">
                {job.sourceUser ? `${t('migrations.from', { user: job.sourceUser })} · ` : ''}
                {counters(job, t, locale)}
              </span>
              {job.status === 'error' && job.lastError ? (
                <span class="text-danger-text mt-1 block text-xs leading-snug">
                  {job.lastError}
                </span>
              ) : null}
            </span>
            {job.status === 'running' ? <Spinner size={16} /> : null}
            <span class="flex items-center gap-1">
              {job.counts.skipped > 0 || !running ? (
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={!live || loadingReport}
                  onClick={() => void openReport(job)}
                >
                  {t('migrations.viewReport')}
                </Button>
              ) : null}
              {running ? (
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={!live || busy}
                  onClick={() => void act(job, 'cancel')}
                >
                  {t('migrations.stop')}
                </Button>
              ) : job.status !== 'done' ? (
                <Button
                  variant="secondary"
                  size="sm"
                  disabled={!live || busy}
                  onClick={() => void act(job, 'resume')}
                >
                  {t('migrations.resume')}
                </Button>
              ) : null}
            </span>
          </CardBody>
        </Card>
      </li>
    );
  }

  function launched(jobsCreated: MigrationJob[]) {
    setItems((prev) => [...jobsCreated, ...prev]);
    setWizard(null);
    toast.notify({
      tone: 'success',
      title: t('migrations.toast.launched'),
      description: t('migrations.toast.launchedHint', { count: jobsCreated.length }),
    });
  }

  if (wizard === 'auto') {
    return (
      <SettingsShell
        title={t('migrations.auto.title')}
        description={t('migrations.auto.description')}
      >
        <AutoMigrateWizard
          t={t}
          sources={autoSources}
          {...(justConnected ? { justConnected } : {})}
          onCancel={() => setWizard(null)}
          onLaunched={launched}
        />
      </SettingsShell>
    );
  }

  // Llevarse el contenido: el MISMO asistente, en sentido contrario. Está aquí, a
  // la vista y en dos clics, porque una plataforma de la que no puedes salir no
  // es soberana; lo que retiene a alguien es que el producto sea mejor.
  // «Llevarme mi contenido» es lo PROPIO (con correo y agenda); desde ahí se pasa a
  // llevarse lo de otras personas, que exige la delegación de la organización.
  if (wizard === 'out' || wizard === 'self') {
    return (
      <SettingsShell
        title={t('migrations.out.title')}
        description={t(
          wizard === 'self' ? 'migrations.self.description' : 'migrations.out.description',
        )}
      >
        <MigrateWizard
          key={wizard}
          t={t}
          sources={sources}
          direction={wizard}
          onOthers={() => setWizard('out')}
          onCancel={() => setWizard(null)}
          onLaunched={launched}
        />
      </SettingsShell>
    );
  }

  if (wizard === 'manual') {
    return (
      <SettingsShell
        title={t('migrations.wizard.title')}
        description={t('migrations.wizard.description')}
      >
        <MigrateWizard
          t={t}
          sources={sources}
          onCancel={() => setWizard(null)}
          onLaunched={launched}
        />
      </SettingsShell>
    );
  }

  return (
    <SettingsShell
      title={t('page.migrations')}
      description={t('migrations.subtitle')}
      actions={
        <span class="flex flex-wrap items-center gap-2">
          <Button
            variant="primary"
            disabled={!live || autoSources.length === 0}
            leadingIcon={<Icon name="database" size={16} />}
            onClick={() => setWizard('auto')}
          >
            {t('migrations.autoStart')}
          </Button>
          <Button variant="secondary" disabled={!live} onClick={() => setWizard('manual')}>
            {t('migrations.manualStart')}
          </Button>
          <Button
            variant="ghost"
            disabled={!live || sources.length === 0}
            leadingIcon={<Icon name="globe" size={16} />}
            onClick={() => setWizard('self')}
          >
            {t('migrations.outStart')}
          </Button>
        </span>
      }
      banner={
        live ? undefined : (
          <InlineHelp tone="warning" title={t('common.offlineGatewayTitle')}>
            {t('migrations.offlineBody')}
          </InlineHelp>
        )
      }
    >
      {active.length > 0 ? (
        <SettingsSection
          plain
          title={t('migrations.active.title')}
          description={t('migrations.active.description')}
        >
          <ul class="flex flex-col gap-2">{active.map((j) => renderRow(j))}</ul>
        </SettingsSection>
      ) : null}

      <SettingsSection
        plain
        title={t(active.length > 0 ? 'migrations.finished.title' : 'migrations.list.title')}
        description={t('migrations.list.description')}
      >
        {finished.length === 0 ? (
          <Card>
            <EmptyState
              title={t(
                active.length > 0 ? 'migrations.empty.titleActive' : 'migrations.empty.title',
              )}
              description={t('migrations.empty.description')}
              illustration={<Icon name="database" size={28} />}
              action={
                active.length > 0 ? undefined : (
                  <Button variant="primary" disabled={!live} onClick={() => setWizard('auto')}>
                    {t('migrations.empty.action')}
                  </Button>
                )
              }
            />
          </Card>
        ) : (
          <ul class="flex flex-col gap-2">{finished.map((j) => renderRow(j))}</ul>
        )}
      </SettingsSection>

      <Modal
        open={Boolean(report)}
        onClose={() => setReport(null)}
        title={t('migrations.report.title')}
        size="lg"
      >
        {report ? (
          <div class="flex flex-col gap-4">
            <p class="text-text-muted text-sm leading-relaxed">
              <span class="text-text font-medium">{report.job.targetUser}</span> ·{' '}
              {t(DOMAIN_KEY[report.job.domain])}
              {report.job.sourceUser
                ? ` · ${t('migrations.from', { user: report.job.sourceUser })}`
                : ''}
            </p>
            <ul class="grid gap-2 sm:grid-cols-3">
              <li class="border-border bg-surface-raised rounded-lg border p-3">
                <span class="text-text block text-lg font-semibold">
                  {formatNumber(locale, report.job.counts.migrated)}
                </span>
                <span class="text-text-muted text-xs">{t('migrations.report.migrated')}</span>
              </li>
              <li class="border-border bg-surface-raised rounded-lg border p-3">
                <span class="text-text block text-lg font-semibold">
                  {formatNumber(locale, report.job.counts.skipped)}
                </span>
                <span class="text-text-muted text-xs">{t('migrations.report.skipped')}</span>
              </li>
              <li class="border-border bg-surface-raised rounded-lg border p-3">
                <span class="text-text block text-lg font-semibold">
                  {formatBytes(locale, report.job.counts.bytesCopied)}
                </span>
                <span class="text-text-muted text-xs">{t('migrations.report.content')}</span>
              </li>
            </ul>

            {report.job.lastError ? (
              <InlineHelp tone="warning" title={t('migrations.report.whatFailed')}>
                {report.job.lastError}
              </InlineHelp>
            ) : null}

            {loadingReport ? (
              <p class="text-text-muted text-sm">{t('common.loading')}</p>
            ) : report.skipped.length === 0 ? (
              <InlineHelp tone="success" title={t('migrations.report.nothingLostTitle')}>
                {t('migrations.report.nothingLostBody')}
              </InlineHelp>
            ) : (
              <section>
                <h3 class="text-text text-sm font-semibold">
                  {t('migrations.report.skippedTitle')}
                </h3>
                <ul class="border-border divide-border-subtle mt-2 max-h-72 divide-y overflow-y-auto rounded-lg border">
                  {report.skipped.map((s) => (
                    <li key={s.sourceId} class="px-3 py-2">
                      <span class="text-text block truncate text-sm font-medium">
                        {s.path || s.sourceId}
                      </span>
                      <span class="text-text-muted block text-xs leading-snug">{s.reason}</span>
                    </li>
                  ))}
                </ul>
              </section>
            )}

            <div class="flex justify-end">
              <Button variant="secondary" onClick={() => setReport(null)}>
                {t('common.close')}
              </Button>
            </div>
          </div>
        ) : null}
      </Modal>
    </SettingsShell>
  );
}

export default function MigrationsPanel(props: MigrationsPanelProps) {
  return (
    <ToastProvider>
      <Inner {...props} />
    </ToastProvider>
  );
}
