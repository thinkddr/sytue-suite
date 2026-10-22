/**
 * OverviewPanel — resumen del proyecto (estilo Vercel): producción actual,
 * métricas de uso en lenguaje claro, servicios gestionados conectados y últimos
 * despliegues. Solo @sytue/ui. docs/CONVENTIONS.md §14.
 */
import { useState } from 'preact/hooks';
import {
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  Badge,
  Button,
  Icon,
  SectionHeader,
  useToast,
  useConfirm,
  ToastProvider,
  InlineHelp,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { formatList } from '@sytue/i18n';
import type { Deployment, ManagedService, Project, UsageMetric } from '../lib/types.js';
import type { T } from '../i18n/index.js';
import { useT } from '../i18n/index.js';
import { r } from '../lib/rutas.js';
import { deployStatusLabel, deployStatusTone, relativeTime } from '../lib/format.js';
import { postBff, notifyDemoIfNeeded } from '../lib/bff-client.js';
import { servicesNotYetLive } from '../lib/servicios.js';
import DemoBanner from './DemoBanner.js';

/** Duración del build en texto compacto («1 min 20 s»), en el idioma de la página. */
function formatDuration(t: T, sec: number): string {
  if (sec < 60) return t('duration.seconds', { count: sec });
  const minutes = Math.floor(sec / 60);
  const seconds = sec % 60;
  return seconds
    ? t('duration.minutesSeconds', { minutes, seconds })
    : t('duration.minutes', { count: minutes });
}

function serviceLabel(t: T, kind: ManagedService['kind']): string {
  switch (kind) {
    case 'postgres':
      return t('service.managed.postgres');
    case 'storage':
      return t('service.managed.storage');
    case 'cache':
      return t('service.managed.cache');
    case 'functions':
      return t('service.managed.functions');
    case 'cron':
      return t('service.managed.cron');
    case 'job':
      return t('service.managed.job');
    case 'worker':
      return t('service.managed.worker');
    case 'auth':
      return t('service.managed.auth');
    case 'app':
      return t('service.managed.app');
  }
}

const serviceStatusTone: Record<
  ManagedService['status'],
  { tone: 'success' | 'info' | 'warning'; dot: string }
> = {
  running: { tone: 'success', dot: 'bg-success' },
  provisioning: { tone: 'info', dot: 'bg-info' },
  paused: { tone: 'warning', dot: 'bg-warning' },
};

function serviceStatusLabel(t: T, status: ManagedService['status']): string {
  switch (status) {
    case 'running':
      return t('service.status.running');
    case 'provisioning':
      return t('service.status.provisioning');
    case 'paused':
      return t('service.status.paused');
  }
}

interface OverviewProps {
  project: Project;
  current?: Deployment | undefined;
  usage: UsageMetric[];
  services: ManagedService[];
  recent: Deployment[];
  /** Hay un dominio propio del usuario (para el checklist de setup). */
  hasCustomDomain: boolean;
  live: boolean;
  /** Idioma ya negociado en el servidor (llega por props, nunca se detecta aquí). */
  locale: Locale;
  /** Error REAL del control-plane, si lo hubo: manda sobre `live` (Sourced.error). */
  error?: { status: number; code: string } | undefined;
}

export default function OverviewPanel(props: OverviewProps) {
  return (
    <ToastProvider>
      <OverviewInner {...props} />
    </ToastProvider>
  );
}

function OverviewInner({
  project,
  current,
  usage,
  services,
  recent,
  hasCustomDomain,
  live,
  locale,
  error,
}: OverviewProps) {
  const t = useT(locale);
  const toast = useToast();
  const { confirmar, dialogo } = useConfirm();
  const [redeployBusy, setRedeployBusy] = useState(false);
  const [restartBusy, setRestartBusy] = useState(false);
  // Despliegues recientes como estado local: al encolar insertamos de forma
  // optimista la build (la acción primaria produce un cambio visible en el acto).
  const [recentItems, setRecentItems] = useState(recent);
  // Servicios que la app EN MARCHA todavía no puede usar (ver servicesNotYetLive).
  const pendingServices = servicesNotYetLive(services, current);

  async function redeploy() {
    setRedeployBusy(true);
    const src = current ?? recentItems[0];
    const optimistic: Deployment = {
      id: crypto.randomUUID(),
      project: project.slug,
      status: 'building',
      target: 'production',
      branch: src?.branch ?? 'main',
      commitSha: src?.commitSha ?? '',
      // `||`, no `??`: sin push el mensaje es la cadena VACÍA (ver DeploymentsPanel).
      commitMessage: src?.commitMessage || t('deploy.newDeployment'),
      author: src?.author ?? t('deploy.you'),
      url: src?.url ?? project.productionUrl,
      createdAt: new Date().toISOString(),
    };
    setRecentItems((prev) => [optimistic, ...prev]);
    try {
      const res = await postBff<{ ok: boolean; live: boolean }>('/api/deploy', {
        project: project.slug,
      });
      notifyDemoIfNeeded(t, toast, res.live);
      toast.notify({
        tone: 'success',
        title: t('toast.deployQueued'),
        description: res.live ? t('overview.deployQueued.desc') : t('toast.demoNotDeployed'),
      });
    } catch {
      // El encolado falló de verdad: retira la fila optimista.
      setRecentItems((prev) => prev.filter((d) => d.id !== optimistic.id));
      toast.notify({
        tone: 'danger',
        title: t('toast.deployFailed'),
        description: t('common.retry'),
      });
    } finally {
      setRedeployBusy(false);
    }
  }

  // Reiniciar sí pregunta, y redesplegar no: este corta las peticiones que estén
  // en curso AHORA en producción, y está a un centímetro de «Visitar». El
  // redespliegue, en cambio, no toca lo que hay servido hasta que la versión
  // nueva está lista.
  async function restart() {
    const ok = await confirmar({
      titulo: t('restart.confirm.title'),
      mensaje: t('restart.confirm.body'),
      confirmLabel: t('restart.action'),
    });
    if (!ok) return;
    setRestartBusy(true);
    try {
      const res = await postBff<{ ok: boolean; live: boolean; restarted: number }>('/api/restart', {
        project: project.slug,
      });
      notifyDemoIfNeeded(t, toast, res.live);
      toast.notify({
        tone: res.live && res.restarted === 0 ? 'warning' : 'success',
        // Cero instancias reiniciadas no es un fallo ni un éxito: es que no había
        // nada corriendo. Decirlo evita que alguien se quede esperando un efecto.
        title:
          res.live && res.restarted === 0
            ? t('restart.none')
            : t('restart.done', { count: res.restarted }),
        ...(res.live ? { description: t('restart.done.desc') } : {}),
      });
    } catch (err) {
      const paused = /bff_409/.test(err instanceof Error ? err.message : '');
      toast.notify({
        tone: 'danger',
        title: paused ? t('restart.paused') : t('restart.fail'),
      });
    } finally {
      setRestartBusy(false);
    }
  }

  // Con `r()`, como en la barra lateral: sin el prefijo de `/cloud` cada paso
  // del checklist mandaba fuera de la aplicación.
  const base = r(`/projects/${project.slug}`);
  // Checklist de setup derivado del estado REAL (nada hardcodeado). Cada paso
  // pendiente enlaza a donde se completa; se oculta entero cuando ya está todo.
  const setup: Array<{ done: boolean; label: string; help: string; href: string }> = [
    {
      done: Boolean(project.repo),
      label: t('overview.setup.repo'),
      help: project.repo || t('overview.setup.repo.help'),
      href: `${base}/deployments`,
    },
    {
      done: Boolean(current) || recent.length > 0,
      label: t('overview.setup.deploy'),
      help: t('overview.setup.deploy.help'),
      href: `${base}/deployments`,
    },
    {
      done: hasCustomDomain,
      label: t('overview.setup.domain'),
      help: t('overview.setup.domain.help'),
      href: `${base}/domains`,
    },
    {
      done: services.length > 0,
      label: t('overview.setup.service'),
      help: t('overview.setup.service.help'),
      href: `${base}/database`,
    },
  ];
  const setupDone = setup.filter((s) => s.done).length;

  return (
    <>
      <DemoBanner live={live} locale={locale} error={error} />
      <SectionHeader
        level={1}
        icon={<Icon name="grid" size={20} />}
        title={project.name}
        subtitle={t('overview.subtitle', { repo: project.repo, region: project.region })}
        actions={
          <>
            <Button
              variant="secondary"
              // Sin despliegue de producción listo, la URL aún no sirve (404):
              // se deshabilita con la misma condición que gobierna la card de Producción.
              disabled={!current}
              onClick={() => window.open(project.productionUrl, '_blank', 'noopener')}
              trailingIcon={<Icon name="chevronRight" size={16} />}
            >
              {t('overview.visit')}
            </Button>
            {/* Sin producción activa no hay proceso que reiniciar: el botón se
                apaga con la misma condición que «Visitar». */}
            <Button
              variant="secondary"
              loading={restartBusy}
              disabled={!current || restartBusy}
              onClick={() => void restart()}
            >
              {t('restart.action')}
            </Button>
            <Button variant="primary" loading={redeployBusy} onClick={() => void redeploy()}>
              {t('overview.redeploy')}
            </Button>
          </>
        }
      />
      {dialogo}
      <div class="cloud-content">
        {/* Checklist de setup (U1): solo mientras quede algún paso pendiente. */}
        {setupDone < setup.length ? (
          <Card class="mb-4">
            <CardHeader>
              <CardTitle as="h2">{t('overview.setup.title', { name: project.name })}</CardTitle>
              <span class="text-text-muted text-sm tabular-nums">
                {setupDone}/{setup.length}
              </span>
            </CardHeader>
            <CardBody class="p-0">
              <ul>
                {setup.map((s) => (
                  <li
                    key={s.label}
                    class="border-border flex items-center gap-3 border-b px-4 py-3 last:border-0"
                  >
                    <span
                      class={
                        'grid h-6 w-6 shrink-0 place-items-center rounded-full ' +
                        (s.done
                          ? 'bg-success text-white'
                          : 'border-border-strong text-text-subtle border')
                      }
                      aria-hidden="true"
                    >
                      {s.done ? <Icon name="check" size={16} /> : null}
                    </span>
                    <span class="min-w-0 flex-1">
                      <span
                        class={
                          'block truncate ' + (s.done ? 'text-text-muted' : 'text-text font-medium')
                        }
                      >
                        {s.label}
                      </span>
                      <span class="text-text-muted block truncate text-sm">{s.help}</span>
                    </span>
                    {s.done ? (
                      <span class="text-success-text shrink-0 text-sm font-medium">
                        {t('overview.setup.done')}
                      </span>
                    ) : (
                      <a
                        href={s.href}
                        class="text-primary-text sytue-focus-ring-inset shrink-0 rounded-sm text-sm font-semibold hover:underline"
                      >
                        {t('overview.setup.configure')}
                      </a>
                    )}
                  </li>
                ))}
              </ul>
            </CardBody>
          </Card>
        ) : null}

        <div class="grid gap-4 lg:grid-cols-3">
          {/* Producción */}
          <Card class="lg:col-span-2">
            <CardHeader>
              <CardTitle as="h2">{t('overview.production')}</CardTitle>
              {current ? (
                <Badge tone={deployStatusTone[current.status]}>
                  {deployStatusLabel(t, current.status)}
                </Badge>
              ) : null}
            </CardHeader>
            <CardBody class="flex flex-col gap-3">
              {current ? (
                <>
                  <a
                    href={project.productionUrl}
                    target="_blank"
                    rel="noopener"
                    class="text-primary-text sytue-focus-ring-inset min-w-0 self-start truncate rounded-sm font-mono text-base hover:underline"
                    title={project.productionUrl}
                  >
                    {project.productionUrl.replace('https://', '')}
                  </a>
                  <p
                    class="text-text truncate text-base"
                    title={current.commitMessage || t('deployments.noMessage')}
                  >
                    {current.commitMessage || t('deployments.noMessage')}
                  </p>
                  <p class="text-text-muted truncate text-sm">
                    {current.branch}
                    {current.commitSha ? (
                      <>
                        {' · '}
                        <span class="font-mono">{current.commitSha}</span>
                      </>
                    ) : null}
                    {' · '}
                    {relativeTime(locale, current.createdAt)}
                    {current.durationSec !== undefined
                      ? ` · ${t('overview.builtIn', { duration: formatDuration(t, current.durationSec) })}`
                      : ''}
                  </p>
                </>
              ) : (
                <p class="text-text-muted text-base">{t('overview.noProduction')}</p>
              )}
            </CardBody>
          </Card>

          {/* Servicios */}
          <Card>
            <CardHeader>
              <CardTitle as="h2">{t('overview.services')}</CardTitle>
            </CardHeader>
            <CardBody class="flex flex-col gap-2">
              {services.length === 0 ? (
                <p class="text-text-muted text-sm">{t('overview.noServices')}</p>
              ) : null}
              {/* Un servicio creado DESPUÉS del despliegue vivo está aprovisionado
                  y a la vez es inalcanzable para la app: sus credenciales se
                  inyectan al desplegar. Sin este aviso, todo parece correcto y la
                  conexión falla — de los fallos más desconcertantes que hay. */}
              {pendingServices.length > 0 ? (
                <InlineHelp tone="warning">
                  {t('overview.servicesNotLive', {
                    services: formatList(
                      locale,
                      pendingServices.map((s) => serviceLabel(t, s.kind)),
                    ),
                    count: pendingServices.length,
                  })}{' '}
                  <Button
                    size="sm"
                    variant="secondary"
                    class="mt-2"
                    disabled={redeployBusy || !live}
                    onClick={() => void redeploy()}
                  >
                    {t('overview.servicesNotLive.action')}
                  </Button>
                </InlineHelp>
              ) : null}
              {services.map((s) => {
                const st = serviceStatusTone[s.status];
                return (
                  <div key={s.kind} class="flex items-center justify-between gap-2">
                    <span class="text-text flex min-w-0 items-center gap-2 truncate text-base">
                      <span class={'h-2 w-2 shrink-0 rounded-full ' + st.dot} aria-hidden="true" />
                      {serviceLabel(t, s.kind)}
                    </span>
                    <Badge tone={st.tone}>{serviceStatusLabel(t, s.status)}</Badge>
                  </div>
                );
              })}
            </CardBody>
          </Card>
        </div>

        {/* Uso */}
        <div class="mt-4 grid grid-cols-2 gap-4 lg:grid-cols-4">
          {usage.map((m) => (
            <Card key={m.label} raised={false}>
              <CardBody>
                <p class="text-text-muted truncate text-sm">{m.label}</p>
                <p class="sytue-h3 text-text mt-1 tabular-nums">{m.value}</p>
                {m.delta ? (
                  <p
                    class={
                      'mt-0.5 text-sm font-medium tabular-nums ' +
                      (m.trend === 'down' ? 'text-success-text' : 'text-text-muted')
                    }
                  >
                    {m.delta}
                  </p>
                ) : null}
              </CardBody>
            </Card>
          ))}
        </div>

        {/* Despliegues recientes */}
        <Card class="mt-4">
          <CardHeader>
            <CardTitle as="h2">{t('overview.recent')}</CardTitle>
            <a
              href={r(`/projects/${project.slug}/deployments`)}
              class="text-primary-text sytue-focus-ring-inset rounded-sm text-sm font-semibold hover:underline"
            >
              {t('overview.viewAll')}
            </a>
          </CardHeader>
          <CardBody class="p-0">
            {recentItems.length === 0 ? (
              <p class="text-text-muted px-4 py-8 text-center text-sm">
                {t('overview.noDeployments')}
              </p>
            ) : (
              <ul>
                {recentItems.map((d) => (
                  <li
                    key={d.id}
                    class="border-border hover:bg-bg-subtle flex items-center gap-3 border-b px-4 py-2.5 transition-colors last:border-0"
                  >
                    <Badge tone={deployStatusTone[d.status]}>
                      {deployStatusLabel(t, d.status)}
                    </Badge>
                    <span
                      class="text-text min-w-0 flex-1 truncate text-base"
                      title={d.commitMessage || t('deployments.noMessage')}
                    >
                      {d.commitMessage || t('deployments.noMessage')}
                    </span>
                    {d.pr ? (
                      <Badge tone="primary">PR #{d.pr}</Badge>
                    ) : (
                      <Badge tone="neutral">{t('overview.prod')}</Badge>
                    )}
                    <span class="text-text-muted shrink-0 text-sm tabular-nums">
                      {relativeTime(locale, d.createdAt)}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </CardBody>
        </Card>
      </div>
    </>
  );
}
