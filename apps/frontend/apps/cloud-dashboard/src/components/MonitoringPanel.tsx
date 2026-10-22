/**
 * MonitoringPanel — monitorización del proyecto (Wave 2): sondas de uptime y
 * alertas por evento. El uptime lista cada sonda con su estado (up/down/sin
 * datos), latencia y última comprobación, y permite crear/editar/borrar
 * (nombre, URL, intervalo). Las alertas enrutan eventos (despliegue/uptime) a un
 * webhook Slack/Discord/genérico (alcance organización; requiere rol admin).
 * Mutaciones vía POST /api/uptime y /api/alerts. Solo @sytue/ui, tokens
 * --sytue-*. docs/CONVENTIONS.md §14.
 */
import { useState } from 'preact/hooks';
import {
  Card,
  CardBody,
  Button,
  Badge,
  Icon,
  Input,
  Select,
  FieldGroup,
  describedBy,
  Modal,
  useToast,
  ToastProvider,
  SectionHeader,
  EmptyState,
} from '@sytue/ui';
import type { BadgeTone } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { UptimeCheck, UptimeStatus, AlertPolicy, AlertEvent } from '../lib/types.js';
import type { T } from '../i18n/index.js';
import { useT } from '../i18n/index.js';
import { postBff, notifyDemoIfNeeded, demoDesc } from '../lib/bff-client.js';
import { relativeTime } from '../lib/format.js';
import DemoBanner from './DemoBanner.js';

// ---- Constantes de presentación ----

const INTERVAL_OPTIONS = [30, 60, 300, 900, 1800, 3600] as const;

/**
 * Cadencia de la sonda («cada 30 s»). La frase entera sale del catálogo: partirla
 * en «cada » + cifra + unidad dejaría el orden clavado al del español.
 */
function intervalLabel(t: T, sec: number): string {
  if (sec < 60) return t('monitoring.every.seconds', { count: sec });
  if (sec < 3600) return t('monitoring.every.minutes', { count: Math.round(sec / 60) });
  return t('monitoring.every.hours', { count: Math.round(sec / 3600) });
}

/** Presentación del estado de una sonda; la etiqueta la pone el catálogo. */
const STATUS_META: Record<UptimeStatus, { boxBg: string; dot: string; tone: BadgeTone }> = {
  up: { boxBg: 'bg-success-bg', dot: 'bg-[var(--sytue-success)]', tone: 'success' },
  down: { boxBg: 'bg-danger-bg', dot: 'bg-[var(--sytue-danger)]', tone: 'danger' },
  unknown: { boxBg: 'bg-bg-subtle', dot: 'bg-[var(--sytue-text-subtle)]', tone: 'neutral' },
};

function statusLabel(t: T, status: UptimeStatus): string {
  switch (status) {
    case 'up':
      return t('monitoring.status.up');
    case 'down':
      return t('monitoring.status.down');
    case 'unknown':
      return t('monitoring.status.unknown');
  }
}

const EVENTS: readonly AlertEvent[] = [
  'deploy.failed',
  'deploy.ready',
  'uptime.down',
  'uptime.up',
  'billing.spend_threshold',
  'sizing.advice',
  'metric.cpu_high',
  'metric.memory_high',
];

/** Nombre llano de un evento de alerta. El identificador es contrato de API. */
function eventLabel(t: T, event: AlertEvent): string {
  switch (event) {
    case 'deploy.failed':
      return t('monitoring.event.deployFailed');
    case 'deploy.ready':
      return t('monitoring.event.deployReady');
    case 'uptime.down':
      return t('monitoring.event.uptimeDown');
    case 'uptime.up':
      return t('monitoring.event.uptimeUp');
    case 'billing.spend_threshold':
      return t('monitoring.event.spendThreshold');
    case 'sizing.advice':
      return t('monitoring.event.sizingAdvice');
    case 'metric.cpu_high':
      return t('monitoring.event.cpuHigh');
    case 'metric.memory_high':
      return t('monitoring.event.memoryHigh');
  }
}

const EVENT_TONE: Record<AlertEvent, BadgeTone> = {
  'deploy.failed': 'danger',
  'deploy.ready': 'success',
  'uptime.down': 'danger',
  'uptime.up': 'success',
  'billing.spend_threshold': 'warning',
  // Neutro a propósito: el aviso de tamaño tanto puede ser un ahorro como un
  // riesgo, y pintarlo siempre de un color diría lo que no sabemos aquí.
  'sizing.advice': 'neutral',
  // Advertencia y no peligro: el proyecto sigue en pie. Lo que dice es «esto se
  // cae hoy si no haces nada», que es otra cosa que «se ha caído».
  'metric.cpu_high': 'warning',
  'metric.memory_high': 'warning',
};

/** Código HTTP embebido por postBff en el mensaje del error (bff_<status>). */
function statusOf(e: unknown): number {
  const m = /bff_(\d+)/.exec(e instanceof Error ? e.message : String(e));
  return m ? Number(m[1]) : 0;
}

/** Icono de estado bien encuadrado: caja tintada + punto de color por estado. */
function StatusIcon({ status, t }: { status: UptimeStatus; t: T }) {
  const m = STATUS_META[status];
  return (
    <span
      class={`border-border grid h-9 w-9 shrink-0 place-items-center rounded-md border ${m.boxBg}`}
    >
      <span class={`h-2.5 w-2.5 rounded-full ${m.dot}`} aria-hidden="true" />
      <span class="sr-only">{statusLabel(t, status)}</span>
    </span>
  );
}

// ---------------------------------------------------------------------------

export default function MonitoringPanel({
  project,
  checks,
  policies,
  live,
  locale,
  error,
}: {
  project: string;
  checks: UptimeCheck[];
  policies: AlertPolicy[];
  live: boolean;
  /** Idioma ya negociado en el servidor (llega por props, nunca se detecta aquí). */
  locale: Locale;
  /** Error REAL del control-plane, si lo hubo: manda sobre `live` (Sourced.error). */
  error?: { status: number; code: string } | undefined;
}) {
  const t = useT(locale);
  return (
    <ToastProvider>
      <DemoBanner live={live} locale={locale} error={error} />
      <SectionHeader
        level={1}
        icon={<Icon name="bell" size={20} />}
        title={t('nav.monitoring')}
        subtitle={t('monitoring.subtitle')}
      />
      <div class="cloud-content flex flex-col gap-6">
        <UptimeSection project={project} initial={checks} t={t} locale={locale} />
        <AlertsSection initial={policies} t={t} />
      </div>
    </ToastProvider>
  );
}

// ---- Sección: sondas de uptime ----

interface CheckErrors {
  name?: string;
  url?: string;
}

function UptimeSection({
  project,
  initial,
  t,
  locale,
}: {
  project: string;
  initial: UptimeCheck[];
  t: T;
  locale: Locale;
}) {
  const toast = useToast();
  const [items, setItems] = useState<UptimeCheck[]>(initial);
  const [editing, setEditing] = useState<string | null>(null);
  const [name, setName] = useState('');
  const [url, setUrl] = useState('');
  const [intervalSec, setIntervalSec] = useState<number>(60);
  const [errors, setErrors] = useState<CheckErrors>({});
  const [busy, setBusy] = useState(false);
  const [pendingDelete, setPendingDelete] = useState<UptimeCheck | null>(null);
  const [deleting, setDeleting] = useState(false);
  /** ID de la sonda que se está pausando/reanudando (bloquea SOLO su botón). */
  const [toggling, setToggling] = useState<string | null>(null);

  function resetForm() {
    setEditing(null);
    setName('');
    setUrl('');
    setIntervalSec(60);
    setErrors({});
  }

  function startEdit(c: UptimeCheck) {
    setEditing(c.id);
    setName(c.name);
    setUrl(c.targetUrl);
    setIntervalSec(c.intervalSeconds);
    setErrors({});
    requestAnimationFrame(() => document.getElementById('check-name')?.focus());
  }

  function validate(): CheckErrors {
    const errs: CheckErrors = {};
    if (!name.trim()) errs.name = t('monitoring.error.nameRequired');
    const u = url.trim();
    if (!u) errs.url = t('monitoring.error.urlRequired');
    else {
      try {
        if (new URL(u).protocol !== 'https:') errs.url = t('monitoring.error.urlHttps');
      } catch {
        errs.url = t('monitoring.error.urlInvalid');
      }
    }
    return errs;
  }

  async function submit(e: Event) {
    e.preventDefault();
    const errs = validate();
    setErrors(errs);
    if (errs.name || errs.url) return;

    const cleanName = name.trim();
    const cleanUrl = url.trim();
    setBusy(true);
    try {
      if (editing) {
        const res = await postBff<{ ok: boolean; live: boolean }>('/api/uptime', {
          check_id: editing,
          name: cleanName,
          target_url: cleanUrl,
          interval_seconds: intervalSec,
        });
        setItems((prev) =>
          prev.map((c) =>
            c.id === editing
              ? { ...c, name: cleanName, targetUrl: cleanUrl, intervalSeconds: intervalSec }
              : c,
          ),
        );
        notifyDemoIfNeeded(t, toast, res.live);
        toast.notify({
          tone: 'success',
          title: t('monitoring.updated'),
          ...demoDesc(t, res.live),
        });
      } else {
        const res = await postBff<{ ok: boolean; live: boolean; check: UptimeCheck | null }>(
          '/api/uptime',
          { project, name: cleanName, target_url: cleanUrl, interval_seconds: intervalSec },
        );
        const created: UptimeCheck = res.check ?? {
          id: crypto.randomUUID(),
          name: cleanName,
          targetUrl: cleanUrl,
          intervalSeconds: intervalSec,
          enabled: true,
          status: 'unknown',
          latencyMs: 0,
        };
        setItems((prev) => [created, ...prev]);
        notifyDemoIfNeeded(t, toast, res.live);
        toast.notify({
          tone: 'success',
          title: t('monitoring.created'),
          ...demoDesc(t, res.live, t('monitoring.created.desc')),
        });
      }
      resetForm();
    } catch (err) {
      toast.notify({ tone: 'danger', ...checkErrorToast(t, err) });
    } finally {
      setBusy(false);
    }
  }

  /**
   * Pausa o reanuda una sonda. El sondeador solo recoge las activas, así que
   * pausarla es lo que calla sus alertas durante un mantenimiento programado sin
   * perder la sonda ni su configuración —hasta ahora la única forma de no recibir
   * el aviso era borrarla y volver a crearla—.
   *
   * Sin confirmación: es reversible con el mismo botón, y un diálogo aquí sería
   * una interrupción gratis.
   */
  async function toggleEnabled(c: UptimeCheck) {
    if (toggling) return;
    const next = !c.enabled;
    setToggling(c.id);
    setItems((prev) => prev.map((i) => (i.id === c.id ? { ...i, enabled: next } : i)));
    try {
      const res = await postBff<{ ok: boolean; live: boolean }>('/api/uptime', {
        check_id: c.id,
        enabled: next,
      });
      notifyDemoIfNeeded(t, toast, res.live);
      toast.notify({
        tone: 'success',
        title: next ? t('monitoring.check.resumed') : t('monitoring.check.paused'),
        ...demoDesc(t, res.live),
      });
    } catch (err) {
      setItems((prev) => prev.map((i) => (i.id === c.id ? { ...i, enabled: c.enabled } : i)));
      toast.notify({ tone: 'danger', ...checkErrorToast(t, err) });
    } finally {
      setToggling(null);
    }
  }

  async function confirmDelete() {
    if (!pendingDelete) return;
    const c = pendingDelete;
    setDeleting(true);
    try {
      const res = await postBff<{ ok: boolean; live: boolean }>('/api/uptime', {
        check_id: c.id,
        delete: true,
      });
      setItems((prev) => prev.filter((i) => i.id !== c.id));
      if (editing === c.id) resetForm();
      setPendingDelete(null);
      notifyDemoIfNeeded(t, toast, res.live);
      toast.notify({ tone: 'success', title: t('monitoring.deleted'), ...demoDesc(t, res.live) });
    } catch (err) {
      toast.notify({ tone: 'danger', ...checkErrorToast(t, err) });
    } finally {
      setDeleting(false);
    }
  }

  // Si el intervalo en edición no es un preset, lo añadimos como opción para que
  // el Select refleje el valor real (no lo pisa silenciosamente).
  const intervalChoices = INTERVAL_OPTIONS.includes(
    intervalSec as (typeof INTERVAL_OPTIONS)[number],
  )
    ? INTERVAL_OPTIONS
    : [intervalSec, ...INTERVAL_OPTIONS];

  return (
    <section aria-labelledby="uptime-heading">
      <div class="mb-3 flex items-baseline justify-between gap-3">
        <h2 id="uptime-heading" class="sytue-h4 text-text">
          {t('monitoring.heading')}
        </h2>
        <span class="text-text-muted text-sm tabular-nums">
          {t('monitoring.count', { count: items.length })}
        </span>
      </div>

      <Card class="mb-4">
        <CardBody>
          <form class="flex flex-wrap items-end gap-3" onSubmit={(e) => void submit(e)}>
            <FieldGroup
              label={t('common.name')}
              htmlFor="check-name"
              class="min-w-[10rem] flex-1"
              {...(errors.name ? { error: errors.name } : {})}
            >
              <Input
                id="check-name"
                value={name}
                placeholder={t('monitoring.field.name.placeholder')}
                invalid={Boolean(errors.name)}
                aria-describedby={describedBy('check-name', undefined, errors.name)}
                onInput={(e) => setName(e.currentTarget.value)}
              />
            </FieldGroup>
            <FieldGroup
              label={t('monitoring.field.url')}
              htmlFor="check-url"
              class="min-w-[14rem] flex-[2]"
              {...(errors.url ? { error: errors.url } : {})}
            >
              <Input
                id="check-url"
                type="url"
                value={url}
                placeholder={t('monitoring.field.url.placeholder')}
                class="font-mono"
                invalid={Boolean(errors.url)}
                aria-describedby={describedBy('check-url', undefined, errors.url)}
                onInput={(e) => setUrl(e.currentTarget.value)}
              />
            </FieldGroup>
            {/* El alemán («alle 30 Min.») es la etiqueta más larga: 9rem la aguanta. */}
            <FieldGroup
              label={t('monitoring.field.interval')}
              htmlFor="check-interval"
              class="w-36"
            >
              <Select
                id="check-interval"
                value={String(intervalSec)}
                onChange={(e) => setIntervalSec(Number(e.currentTarget.value))}
              >
                {intervalChoices.map((s) => (
                  <option key={s} value={String(s)}>
                    {intervalLabel(t, s)}
                  </option>
                ))}
              </Select>
            </FieldGroup>
            <span class="flex shrink-0 gap-2">
              {editing ? (
                <Button type="button" variant="ghost" disabled={busy} onClick={resetForm}>
                  {t('common.cancel')}
                </Button>
              ) : null}
              <Button
                type="submit"
                variant="primary"
                loading={busy}
                {...(editing ? {} : { leadingIcon: <Icon name="plus" size={16} /> })}
              >
                {editing ? t('common.saveChanges') : t('monitoring.add')}
              </Button>
            </span>
          </form>
        </CardBody>
      </Card>

      {items.length === 0 ? (
        <Card>
          <CardBody class="p-0">
            <EmptyState
              illustration={<Icon name="globe" size={24} />}
              title={t('monitoring.empty.title')}
              description={t('monitoring.empty.desc')}
            />
          </CardBody>
        </Card>
      ) : (
        <Card>
          <CardBody class="p-0">
            <ul>
              {items.map((c) => (
                <li
                  key={c.id}
                  class="border-border hover:bg-bg-subtle flex flex-wrap items-center gap-3 border-b px-4 py-3 transition-colors last:border-0"
                >
                  <StatusIcon status={c.status} t={t} />
                  <div class="min-w-0 flex-1">
                    <p class="text-text flex flex-wrap items-center gap-2 font-medium">
                      <span class="truncate" title={c.name}>
                        {c.name}
                      </span>
                      <Badge tone={STATUS_META[c.status].tone}>{statusLabel(t, c.status)}</Badge>
                      {!c.enabled ? <Badge tone="neutral">{t('monitoring.paused')}</Badge> : null}
                    </p>
                    <p class="text-text-muted mt-0.5 flex flex-wrap items-center gap-x-1.5 text-sm">
                      <a
                        href={c.targetUrl}
                        target="_blank"
                        rel="noopener noreferrer"
                        class="hover:text-text max-w-[22rem] truncate font-mono underline-offset-2 hover:underline"
                        title={c.targetUrl}
                      >
                        {c.targetUrl}
                      </a>
                      <span aria-hidden="true">·</span>
                      <span class="whitespace-nowrap">{intervalLabel(t, c.intervalSeconds)}</span>
                      {c.status !== 'unknown' ? (
                        <>
                          <span aria-hidden="true">·</span>
                          <span class="whitespace-nowrap tabular-nums">
                            {t('common.ms', { count: c.latencyMs })}
                          </span>
                        </>
                      ) : null}
                      {c.lastCheckedAt ? (
                        <>
                          <span aria-hidden="true">·</span>
                          <span class="whitespace-nowrap">
                            {t('monitoring.checkedAt', {
                              time: relativeTime(locale, c.lastCheckedAt),
                            })}
                          </span>
                        </>
                      ) : null}
                    </p>
                  </div>
                  <span class="ml-auto flex shrink-0 gap-1">
                    {/* Sin `aria-pressed`: la etiqueta ya CAMBIA con el estado, y
                        poner las dos cosas hace que un lector anuncie «Reanudar,
                        pulsado» —que se lee justo al revés de lo que pasa—. El
                        estado de la sonda lo dice su distintivo, arriba. */}
                    <Button
                      variant="ghost"
                      size="sm"
                      loading={toggling === c.id}
                      disabled={toggling !== null}
                      onClick={() => void toggleEnabled(c)}
                      aria-label={
                        c.enabled
                          ? t('monitoring.pauseCheck', { name: c.name })
                          : t('monitoring.resumeCheck', { name: c.name })
                      }
                    >
                      {c.enabled ? t('monitoring.pause') : t('monitoring.resume')}
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => startEdit(c)}
                      aria-label={t('monitoring.editCheck', { name: c.name })}
                    >
                      {t('common.edit')}
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      class="text-danger-text"
                      onClick={() => setPendingDelete(c)}
                      aria-label={t('monitoring.deleteCheck', { name: c.name })}
                    >
                      {t('common.delete')}
                    </Button>
                  </span>
                </li>
              ))}
            </ul>
          </CardBody>
        </Card>
      )}

      <Modal
        open={pendingDelete !== null}
        onClose={() => setPendingDelete(null)}
        title={t('monitoring.confirm.title')}
        size="sm"
        footer={
          <>
            <Button variant="ghost" onClick={() => setPendingDelete(null)} disabled={deleting}>
              {t('common.cancel')}
            </Button>
            <Button variant="danger" loading={deleting} onClick={() => void confirmDelete()}>
              {t('common.delete')}
            </Button>
          </>
        }
      >
        <p class="text-text-muted text-base">
          {t('monitoring.confirm.body', { name: pendingDelete?.name ?? '' })}
        </p>
      </Modal>
    </section>
  );
}

/** Toast de error de sonda: distingue permisos (403) del resto. */
function checkErrorToast(t: T, err: unknown): { title: string; description: string } {
  if (statusOf(err) === 403) {
    return { title: t('monitoring.denied'), description: t('monitoring.denied.checks') };
  }
  return { title: t('monitoring.failed'), description: t('monitoring.failed.desc') };
}

// ---- Sección: alertas por evento ----

function AlertsSection({ initial, t }: { initial: AlertPolicy[]; t: T }) {
  const toast = useToast();
  const [items, setItems] = useState<AlertPolicy[]>(initial);
  const [event, setEvent] = useState<AlertEvent>('deploy.failed');
  const [channel, setChannel] = useState('');
  const [urlError, setUrlError] = useState<string | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  const [removing, setRemoving] = useState<string | null>(null);

  async function add(e: Event) {
    e.preventDefault();
    const clean = channel.trim();
    let err: string | undefined;
    try {
      const u = new URL(clean);
      if (u.protocol === 'mailto:') {
        // Un `mailto:` es un canal legítimo: la alerta llega por correo en vez
        // de por webhook. La dirección la revalida el control-plane.
        const addr = u.pathname.split('?')[0]?.trim() ?? '';
        if (!/^[^@\s]+@[^@\s.]+(\.[^@\s.]+)+$/.test(addr)) {
          err = t('monitoring.alerts.error.mail');
        }
      } else if (u.protocol !== 'https:' && u.protocol !== 'http:') {
        err = t('monitoring.alerts.error.urlProtocol');
      }
    } catch {
      err = t('monitoring.alerts.error.urlInvalid');
    }
    setUrlError(err);
    if (err) return;

    setBusy(true);
    try {
      const res = await postBff<{ ok: boolean; live: boolean; policy: AlertPolicy | null }>(
        '/api/alerts',
        { event, channel_url: clean },
      );
      const created: AlertPolicy = res.policy ?? {
        id: crypto.randomUUID(),
        event,
        channelUrl: clean,
        enabled: true,
        orgScope: true,
      };
      setItems((prev) => [created, ...prev]);
      setChannel('');
      notifyDemoIfNeeded(t, toast, res.live);
      toast.notify({
        tone: 'success',
        title: t('monitoring.alerts.created'),
        ...demoDesc(t, res.live),
      });
    } catch (err) {
      toast.notify({ tone: 'danger', ...alertErrorToast(t, err) });
    } finally {
      setBusy(false);
    }
  }

  async function remove(id: string) {
    setRemoving(id);
    try {
      const res = await postBff<{ ok: boolean; live: boolean }>('/api/alerts', {
        policy_id: id,
        delete: true,
      });
      setItems((prev) => prev.filter((p) => p.id !== id));
      notifyDemoIfNeeded(t, toast, res.live);
      toast.notify({
        tone: 'success',
        title: t('monitoring.alerts.deleted'),
        ...demoDesc(t, res.live),
      });
    } catch (err) {
      toast.notify({ tone: 'danger', ...alertErrorToast(t, err) });
    } finally {
      setRemoving(null);
    }
  }

  return (
    <section aria-labelledby="alerts-heading">
      <div class="mb-1 flex items-baseline justify-between gap-3">
        <h2 id="alerts-heading" class="sytue-h4 text-text flex items-center gap-2">
          {t('monitoring.alerts.heading')}
          <Badge tone="neutral">{t('monitoring.alerts.orgBadge')}</Badge>
        </h2>
        <span class="text-text-muted text-sm tabular-nums">
          {t('monitoring.alerts.count', { count: items.length })}
        </span>
      </div>
      <p class="text-text-muted mb-3 text-sm">{t('monitoring.alerts.note')}</p>

      <Card class="mb-4">
        <CardBody>
          <form class="flex flex-wrap items-end gap-3" onSubmit={(e) => void add(e)}>
            {/* w-60: «Wenn dies passiert» y «Quando succede» piden más que w-52. */}
            <FieldGroup
              label={t('monitoring.alerts.field.event')}
              htmlFor="alert-event"
              class="w-60"
            >
              <Select
                id="alert-event"
                value={event}
                onChange={(e) => setEvent(e.currentTarget.value as AlertEvent)}
              >
                {EVENTS.map((ev) => (
                  <option key={ev} value={ev}>
                    {eventLabel(t, ev)}
                  </option>
                ))}
              </Select>
            </FieldGroup>
            <FieldGroup
              label={t('monitoring.alerts.field.channel')}
              htmlFor="alert-channel"
              class="min-w-[16rem] flex-1"
              {...(urlError ? { error: urlError } : {})}
            >
              <Input
                id="alert-channel"
                type="url"
                value={channel}
                placeholder="https://hooks.slack.com/… o mailto:ops@empresa.com"
                class="font-mono"
                invalid={Boolean(urlError)}
                aria-describedby={describedBy('alert-channel', undefined, urlError)}
                onInput={(e) => setChannel(e.currentTarget.value)}
              />
            </FieldGroup>
            <Button
              type="submit"
              variant="primary"
              class="shrink-0"
              loading={busy}
              leadingIcon={<Icon name="plus" size={16} />}
            >
              {t('monitoring.alerts.add')}
            </Button>
          </form>
        </CardBody>
      </Card>

      {items.length === 0 ? (
        <Card>
          <CardBody class="p-0">
            <EmptyState
              illustration={<Icon name="bell" size={24} />}
              title={t('monitoring.alerts.empty.title')}
              description={t('monitoring.alerts.empty.desc')}
            />
          </CardBody>
        </Card>
      ) : (
        <Card>
          <CardBody class="p-0">
            <ul>
              {items.map((p) => (
                <li
                  key={p.id}
                  class="border-border hover:bg-bg-subtle flex flex-wrap items-center gap-3 border-b px-4 py-3 transition-colors last:border-0"
                >
                  <span
                    class="border-border bg-bg-subtle text-text-subtle grid h-9 w-9 shrink-0 place-items-center rounded-md border"
                    aria-hidden="true"
                  >
                    <Icon name="bell" size={16} />
                  </span>
                  <div class="min-w-0 flex-1">
                    <p class="text-text flex flex-wrap items-center gap-2 font-medium">
                      <Badge tone={EVENT_TONE[p.event]}>{eventLabel(t, p.event)}</Badge>
                      <Badge tone="neutral">
                        {p.orgScope
                          ? t('monitoring.alerts.scope.org')
                          : t('monitoring.alerts.scope.project')}
                      </Badge>
                      {!p.enabled ? (
                        <Badge tone="warning">{t('monitoring.alerts.disabled')}</Badge>
                      ) : null}
                    </p>
                    <p
                      class="text-text-muted mt-0.5 max-w-full truncate font-mono text-sm"
                      title={p.channelUrl}
                    >
                      {p.channelUrl}
                    </p>
                  </div>
                  <Button
                    variant="ghost"
                    size="sm"
                    class="text-danger-text ml-auto shrink-0"
                    loading={removing === p.id}
                    disabled={removing !== null}
                    onClick={() => void remove(p.id)}
                    aria-label={t('monitoring.alerts.delete', { event: eventLabel(t, p.event) })}
                  >
                    {t('common.delete')}
                  </Button>
                </li>
              ))}
            </ul>
          </CardBody>
        </Card>
      )}
    </section>
  );
}

/** Toast de error de alerta: distingue permisos (403) del resto. */
function alertErrorToast(t: T, err: unknown): { title: string; description: string } {
  if (statusOf(err) === 403) {
    return {
      title: t('monitoring.alerts.error.forbidden.title'),
      description: t('monitoring.alerts.error.forbidden.body'),
    };
  }
  return {
    title: t('monitoring.alerts.error.generic.title'),
    description: t('monitoring.alerts.error.generic.body'),
  };
}
