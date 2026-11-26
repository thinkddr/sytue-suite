/**
 * DeploymentsPanel — historial de despliegues estilo Vercel: producción +
 * previews por PR, con estado, commit, autor y tiempo de build. Acciones:
 * visitar, promover a producción y rollback (volver a un despliegue anterior),
 * desplegar una rama o commit concretos, congelar la producción y abrir el log de
 * la compilación de cada fila. Buscador y filtro por estado, y las cifras de
 * entrega del equipo arriba. Componentes solo de @sytue/ui. §14.
 */
import { useMemo, useState } from 'preact/hooks';
import {
  Card,
  CardBody,
  Button,
  Badge,
  FieldGroup,
  Icon,
  InlineHelp,
  Input,
  Modal,
  Select,
  Stat,
  Tabs,
  Textarea,
  Spinner,
  ToastProvider,
  useToast,
  Dropdown,
  MenuItem,
  EmptyState,
  SectionHeader,
  cx,
} from '@sytue/ui';
import type { BadgeTone } from '@sytue/ui';
import { formatNumber, type Locale } from '@sytue/i18n';
import type { DeployStatus, Deployment } from '../lib/types.js';
import type { DeployFreezeView } from '../lib/cloud.js';
import type { T } from '../i18n/index.js';
import { useT } from '../i18n/index.js';
import { deployStatusLabel, deployStatusTone, formatDate, relativeTime } from '../lib/format.js';
import {
  accionDisponible,
  commitValido,
  filtrarDespliegues,
  metricasDeEntrega,
  refValida,
} from '../lib/despliegues.js';
import { postBff, notifyDemoIfNeeded, BffError } from '../lib/bff-client.js';
import { r } from '../lib/rutas.js';
import DemoBanner from './DemoBanner.js';

// Tinte del icono de estado por tono semántico (mismo vocabulario que Badge):
// comunica running/listo/error también por color de superficie, no solo el badge.
// Solo roles (*-bg / *-text): se reasignan en modo oscuro sin ajuste manual.
const toneTile: Record<BadgeTone, string> = {
  neutral: 'bg-bg-subtle text-text-muted',
  primary: 'bg-selected text-primary-text',
  accent: 'bg-bg-subtle text-accent-text',
  success: 'bg-success-bg text-success-text',
  warning: 'bg-warning-bg text-warning-text',
  danger: 'bg-danger-bg text-danger-text',
  info: 'bg-info-bg text-info-text',
};

/** Estados por los que se puede filtrar, en el orden en que se leen. */
const ESTADOS: DeployStatus[] = ['ready', 'building', 'queued', 'error', 'canceled', 'retired'];

/** Icono de la casilla de estado: el reloj para lo que espera, la cruz para el fallo. */
function statusIcon(status: DeployStatus) {
  if (status === 'error') return 'danger' as const;
  if (status === 'retired' || status === 'canceled') return 'clock' as const;
  return 'check' as const;
}

function DeploymentRow({
  d,
  project,
  frozen,
  onPromote,
  onCancel,
  onRollback,
  busyId,
  t,
  locale,
}: {
  d: Deployment;
  project: string;
  frozen: boolean;
  onPromote: (d: Deployment) => void;
  onCancel: (d: Deployment) => void;
  onRollback: (d: Deployment) => void;
  busyId: string | null;
  t: T;
  locale: Locale;
}) {
  const busy = busyId === d.id;
  const accion = accionDisponible(d);
  // Con producción congelada, publicar una preview es un despliegue nuevo y se
  // para; el rollback sigue: es la salida de emergencia.
  const verbo = accion === 'promote' && frozen ? null : accion;
  // Sin push no hay mensaje de commit (redeploy, rollback, plantilla, BYO).
  const titulo = d.commitMessage || t('deployments.noMessage');
  const tone = deployStatusTone[d.status];
  return (
    <li class="border-border hover:bg-bg-subtle flex flex-wrap items-center gap-3 border-b px-4 py-3 transition-colors last:border-0">
      <span
        class={cx('grid h-8 w-8 shrink-0 place-items-center rounded-md', toneTile[tone])}
        aria-hidden="true"
      >
        {d.status === 'building' ? (
          <Spinner size={16} />
        ) : (
          <Icon name={statusIcon(d.status)} size={16} />
        )}
      </span>
      <div class="min-w-0 flex-1">
        <div class="flex items-center gap-2">
          <span class="text-text truncate font-medium" title={titulo}>
            {titulo}
          </span>
          {d.current ? <Badge tone="success">{t('deployments.production')}</Badge> : null}
          {d.target === 'preview' && d.pr ? <Badge tone="primary">PR #{d.pr}</Badge> : null}
        </div>
        <p class="text-text-muted truncate text-sm tabular-nums">
          {d.branch} · <span class="text-text-subtle font-mono">{d.commitSha}</span> · {d.author} ·{' '}
          {relativeTime(locale, d.createdAt)}
          {d.durationSec ? ` · ${d.durationSec} s` : ''}
        </p>
        {/* El porqué del fallo, en la misma fila: antes la fila roja decía QUE
            falló y había que ir a los registros a adivinar de qué compilación. */}
        {d.status === 'error' && d.errorMessage ? (
          <p class="text-danger-text mt-1 text-sm break-words">
            <span class="font-medium">{t('deployments.errorReason')}</span> {d.errorMessage}
          </p>
        ) : null}
      </div>
      <Badge tone={tone}>{deployStatusLabel(t, d.status)}</Badge>
      <div class="flex items-center gap-1.5">
        {/* El log de LA compilación de esta fila, no el de la última: es la que
            hay que leer para saber por qué esta en concreto no llegó. */}
        {d.buildId ? (
          <Button
            variant="ghost"
            size="sm"
            href={r(
              `/projects/${encodeURIComponent(project)}/logs?build=${encodeURIComponent(d.buildId)}`,
            )}
            aria-label={t('deployments.buildLog.aria', { sha: d.commitSha || d.branch })}
          >
            {t('deployments.buildLog')}
          </Button>
        ) : null}
        <Button
          variant="secondary"
          size="sm"
          onClick={() => window.open(d.url, '_blank', 'noopener')}
          disabled={d.status !== 'ready'}
        >
          {t('deployments.visit')}
        </Button>
        {/* Un despliegue atascado se quedaba girando para siempre: el control-plane
            sabía cancelar desde el principio y no había ningún botón. Solo aparece
            mientras hay algo que parar. */}
        {d.status === 'building' ? (
          <Button variant="ghost" size="sm" loading={busy} onClick={() => onCancel(d)}>
            {t('deployments.cancel')}
          </Button>
        ) : null}
        {/* El menú solo existe cuando hay algo accionable (accionDisponible): una
            preview lista que publicar o una producción anterior a la que volver
            —también las ya reemplazadas, que son justo a las que se vuelve—. */}
        {verbo ? (
          <Dropdown
            label={t('deployments.menu')}
            trigger={(p) => (
              <Button
                variant="ghost"
                size="sm"
                iconOnly
                aria-label={t('deployments.moreActions')}
                loading={busy}
                onClick={p.onClick}
                aria-expanded={p['aria-expanded']}
                aria-haspopup={p['aria-haspopup']}
                // iconOnly descarta children: el icono va como leadingIcon.
                leadingIcon={<Icon name="ellipsis" size={16} />}
              />
            )}
          >
            {/* Cada fila ofrece SU verbo, no los dos: a una preview no se
                «vuelve» (nunca fue producción), y una producción anterior no se
                «publica» —se recupera—. Ofrecer ambos hacía que uno de los dos
                fallara siempre. */}
            {verbo === 'promote' ? (
              <MenuItem onClick={() => onPromote(d)}>{t('deployments.publish')}</MenuItem>
            ) : (
              <MenuItem onClick={() => onRollback(d)}>{t('deployments.rollback')}</MenuItem>
            )}
          </Dropdown>
        ) : null}
      </div>
    </li>
  );
}

/** Las cuatro cifras de entrega, sobre los despliegues cargados. */
function DeliveryStats({ items, t, locale }: { items: Deployment[]; t: T; locale: Locale }) {
  // `Date.now()` una vez por render de la lista: las cifras no necesitan reloj vivo.
  const m = useMemo(() => metricasDeEntrega(items, Date.now()), [items]);
  if (m.muestra === 0) return null;
  const nf = (n: number, digits: number) =>
    new Intl.NumberFormat(locale, { maximumFractionDigits: digits }).format(n);
  const exito = m.tasaExito;
  return (
    <section aria-labelledby="deploy-stats-title" class="mb-4">
      <div class="mb-2 flex flex-wrap items-baseline justify-between gap-2">
        <h2 id="deploy-stats-title" class="sytue-h4 text-text">
          {t('deployments.stats.title')}
        </h2>
        <span class="text-text-muted text-sm">
          {t('deployments.stats.sample', { count: m.muestra })}
        </span>
      </div>
      <div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Stat
          label={t('deployments.stats.frequency')}
          value={m.produccionPorSemana === null ? '—' : nf(m.produccionPorSemana, 1)}
        />
        <Stat
          label={t('deployments.stats.success')}
          value={exito === null ? '—' : `${formatNumber(locale, Math.round(exito * 100))} %`}
          tone={
            exito === null
              ? 'neutral'
              : exito >= 0.9
                ? 'success'
                : exito >= 0.7
                  ? 'warning'
                  : 'danger'
          }
          hint={t('deployments.stats.success.hint')}
        />
        <Stat
          label={t('deployments.stats.duration')}
          value={m.duracionMediana === null ? '—' : `${formatNumber(locale, m.duracionMediana)} s`}
          hint={t('deployments.stats.duration.hint')}
        />
        <Stat
          label={t('deployments.stats.last')}
          value={m.ultimaProduccion === null ? '—' : relativeTime(locale, m.ultimaProduccion)}
          {...(m.ultimaProduccion ? { hint: formatDate(locale, m.ultimaProduccion) } : {})}
        />
      </div>
    </section>
  );
}

interface Props {
  project: string;
  deployments: Deployment[];
  live: boolean;
  /** Si la producción está congelada (y por qué). */
  freeze: DeployFreezeView;
  /** Idioma ya negociado en el servidor (llega por props, nunca se detecta aquí). */
  locale: Locale;
  /** Error REAL del control-plane, si lo hubo: manda sobre `live` (Sourced.error). */
  error?: { status: number; code: string } | undefined;
}

/**
 * El ToastProvider es propio y no heredado: este panel es una isla aparte de la
 * shell, y el contexto de Preact no cruza de una isla a otra. Sin él `useToast`
 * cae al no-op del design-system y se pierde TODO lo que dice esta pantalla —
 * incluido «no se pudo promover», que sin toast no deja ni rastro: la fila se
 * queda como estaba y el fallo parece que no ha ocurrido.
 */
export default function DeploymentsPanel(props: Props) {
  return (
    <ToastProvider>
      <DeploymentsInner {...props} />
    </ToastProvider>
  );
}

/** El código real del control-plane, si el fallo lo trae. */
function codeOf(err: unknown): string {
  return err instanceof BffError ? err.code : '';
}

function statusOf(err: unknown): number {
  return err instanceof BffError ? err.status : 0;
}

function DeploymentsInner({
  project,
  deployments,
  live: initialLive,
  freeze: initialFreeze,
  locale,
  error,
}: Props) {
  const t = useT(locale);
  const toast = useToast();
  const [items, setItems] = useState<Deployment[]>(deployments);
  const [busyId, setBusyId] = useState<string | null>(null);
  const [redeployBusy, setRedeployBusy] = useState(false);
  const [q, setQ] = useState('');
  const [estado, setEstado] = useState<DeployStatus | ''>('');
  const [freeze, setFreeze] = useState<DeployFreezeView>(initialFreeze);
  const [freezeOpen, setFreezeOpen] = useState(false);
  const [freezeReason, setFreezeReason] = useState('');
  const [freezeBusy, setFreezeBusy] = useState(false);
  const [refOpen, setRefOpen] = useState(false);
  const [refValue, setRefValue] = useState('');
  const [commitValue, setCommitValue] = useState('');
  const [refErrors, setRefErrors] = useState<{ ref?: string; commit?: string }>({});

  const visibles = useMemo(() => filtrarDespliegues(items, { q, estado }), [items, q, estado]);
  const production = useMemo(() => visibles.filter((d) => d.target === 'production'), [visibles]);
  const previews = useMemo(() => visibles.filter((d) => d.target === 'preview'), [visibles]);
  const filtrando = q.trim() !== '' || estado !== '';

  /** Toast de un fallo de producción: dice «congelada» si es eso lo que pasó. */
  function failToast(err: unknown, title: string) {
    if (codeOf(err) === 'deploy_frozen') {
      toast.notify({
        tone: 'danger',
        title: t('deployments.frozen.blocked'),
        description: t('deployments.frozen.blocked.desc'),
      });
      return;
    }
    toast.notify({ tone: 'danger', title, description: t('common.retry') });
  }

  // Promover y revertir son la misma operación del control-plane: publicar un
  // despliegue 'ready' como producción. El BFF (/api/deploy) llama a
  // POST /deployments/{id}/rollback; si no hay backend vivo responde live=false y
  // el cambio queda en demostración (misma degradación que el resto del dashboard).
  /**
   * Para un despliegue en curso. El estado pasa a 'error' en la lista porque es lo que
   * devolverá el control-plane: un despliegue cancelado no llegó a producción, y
   * pintarlo de otro modo diría que sí.
   */
  async function cancelInFlight(target: Deployment) {
    setBusyId(target.id);
    try {
      const res = await postBff<{ ok: boolean; live: boolean }>('/api/deploy', {
        deployment: target.id,
        cancel: true,
      });
      setItems((prev) =>
        prev.map((d) => (d.id === target.id ? { ...d, status: 'error' as const } : d)),
      );
      notifyDemoIfNeeded(t, toast, res.live);
      toast.notify({ tone: 'success', title: t('deployments.canceled') });
    } catch {
      toast.notify({ tone: 'danger', title: t('deployments.updateFail') });
    } finally {
      setBusyId(null);
    }
  }

  async function applyToProduction(target: Deployment, kind: 'promote' | 'rollback') {
    setBusyId(target.id);
    try {
      // Una preview va por otro camino del control-plane: crea una producción
      // nueva reusando su imagen. El de al lado (rollback) EXIGE que el
      // despliegue ya sea de producción, así que este menú prometía sobre
      // previews algo que devolvía 409 sin excepción.
      const res = await postBff<{ ok: boolean; live: boolean }>('/api/deploy', {
        deployment: target.id,
        ...(target.target === 'preview' ? { preview: true } : {}),
      });
      // La producción que deja de servir pasa a «reemplazada», que es lo que dirá
      // el control-plane al recargar: dejarla en «lista» ofrecería volver a ella
      // como si fuera la viva.
      setItems((prev) =>
        prev.map((d) => {
          if (d.id === target.id) return { ...d, current: true, status: 'ready' as const };
          if (d.current) return { ...d, current: false, status: 'retired' as const };
          return d;
        }),
      );
      notifyDemoIfNeeded(t, toast, res.live);
      // En demostración no afirmamos un cambio de producción que no ocurrió: la
      // descripción se sincera (el toast info de notifyDemoIfNeeded ya avisa).
      toast.notify(
        kind === 'promote'
          ? {
              tone: 'success',
              title: t('deployments.promoted'),
              description: !res.live
                ? t('deployments.demoNoChange')
                : target.target === 'preview'
                  ? t('deployments.promoted.preview.desc')
                  : t('deployments.promoted.desc', { sha: target.commitSha }),
            }
          : {
              tone: 'success',
              title: t('deployments.rolledBack'),
              description: res.live
                ? t('deployments.rolledBack.desc', { sha: target.commitSha })
                : t('deployments.demoNoChange'),
            },
      );
    } catch (err) {
      failToast(err, t('deployments.updateFail'));
    } finally {
      setBusyId(null);
    }
  }

  /**
   * Encola un despliegue de producción: el de siempre (rama de producción) o, con
   * `ref`, el de una rama/etiqueta o commit concretos.
   */
  async function redeploy(ref?: { ref?: string; commit?: string }): Promise<boolean> {
    setRedeployBusy(true);
    // Inserción optimista: la acción primaria produce un cambio visible en el
    // acto (una fila 'building' arriba). Con backend vivo el control-plane la
    // resolverá; en demostración queda como simulación local (el toast lo dice).
    const src = items.find((d) => d.current) ?? items[0];
    const optimistic: Deployment = {
      id: crypto.randomUUID(),
      project,
      status: 'building',
      target: 'production',
      branch: ref?.ref || src?.branch || 'main',
      commitSha: ref?.commit ? ref.commit.slice(0, 7) : ref?.ref ? '' : (src?.commitSha ?? ''),
      // `||`, no `??`: un deployment que no viene de un push trae la cadena
      // VACÍA, y `??` solo cubre null/undefined —la fila saldría sin título—.
      commitMessage: ref
        ? t('deploy.newDeployment')
        : src?.commitMessage || t('deploy.newDeployment'),
      author: src?.author ?? t('deploy.you'),
      url: src?.url ?? '',
      createdAt: new Date().toISOString(),
    };
    setItems((prev) => [optimistic, ...prev]);
    try {
      const res = await postBff<{ ok: boolean; live: boolean }>('/api/deploy', {
        project,
        ...(ref?.ref ? { ref: ref.ref } : {}),
        ...(ref?.commit ? { commit: ref.commit } : {}),
      });
      notifyDemoIfNeeded(t, toast, res.live);
      toast.notify({
        tone: 'success',
        title: t('toast.deployQueued'),
        description: res.live ? t('deployments.deployQueued.desc') : t('toast.demoNotDeployed'),
      });
      return true;
    } catch (err) {
      // El encolado falló de verdad: retira la fila optimista.
      setItems((prev) => prev.filter((d) => d.id !== optimistic.id));
      failToast(err, t('toast.deployFailed'));
      return false;
    } finally {
      setRedeployBusy(false);
    }
  }

  async function submitRef(e: Event) {
    e.preventDefault();
    const ref = refValue.trim();
    const commit = commitValue.trim();
    const errs: { ref?: string; commit?: string } = {};
    if (!ref && !commit) errs.ref = t('deployments.ref.required');
    else if (ref && !refValida(ref)) errs.ref = t('deployments.ref.invalid');
    if (commit && !commitValido(commit)) errs.commit = t('deployments.ref.commitInvalid');
    setRefErrors(errs);
    if (errs.ref || errs.commit) return;
    const ok = await redeploy({ ...(ref ? { ref } : {}), ...(commit ? { commit } : {}) });
    if (ok) {
      setRefOpen(false);
      setRefValue('');
      setCommitValue('');
    }
  }

  async function toggleFreeze(next: boolean) {
    setFreezeBusy(true);
    try {
      const res = await postBff<{ ok: boolean; live: boolean } & DeployFreezeView>(
        '/api/deploy-freeze',
        { project, frozen: next, ...(next ? { reason: freezeReason } : {}) },
      );
      setFreeze(
        next
          ? {
              frozen: true,
              ...(res.reason || freezeReason.trim()
                ? { reason: res.reason ?? freezeReason.trim() }
                : {}),
              frozenAt: res.frozenAt ?? new Date().toISOString(),
            }
          : { frozen: false },
      );
      setFreezeOpen(false);
      setFreezeReason('');
      notifyDemoIfNeeded(t, toast, res.live);
      toast.notify({
        tone: 'success',
        title: next ? t('deployments.frozen.done') : t('deployments.unfrozen.done'),
      });
    } catch (err) {
      toast.notify({
        tone: 'danger',
        title:
          statusOf(err) === 403 ? t('deployments.freeze.forbidden') : t('deployments.freeze.fail'),
      });
    } finally {
      setFreezeBusy(false);
    }
  }

  const list = (rows: Deployment[]) => (
    <Card>
      <CardBody class="p-0">
        {rows.length === 0 ? (
          filtrando ? (
            <EmptyState
              icon="search"
              title={t('deployments.filter.empty')}
              action={
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => {
                    setQ('');
                    setEstado('');
                  }}
                >
                  {t('deployments.filter.clear')}
                </Button>
              }
            />
          ) : (
            <EmptyState
              icon="rocket"
              title={t('deployments.empty.title')}
              description={t('deployments.empty.desc')}
              action={
                <Button
                  variant="secondary"
                  size="sm"
                  loading={redeployBusy}
                  disabled={freeze.frozen}
                  onClick={() => void redeploy()}
                >
                  {t('deployments.empty.cta')}
                </Button>
              }
            />
          )
        ) : (
          <ul>
            {rows.map((d) => (
              <DeploymentRow
                key={d.id}
                d={d}
                project={project}
                frozen={freeze.frozen}
                onPromote={(x) => void applyToProduction(x, 'promote')}
                onRollback={(x) => void applyToProduction(x, 'rollback')}
                onCancel={(x) => void cancelInFlight(x)}
                busyId={busyId}
                t={t}
                locale={locale}
              />
            ))}
          </ul>
        )}
      </CardBody>
    </Card>
  );

  return (
    <>
      <DemoBanner live={initialLive} locale={locale} error={error} />
      <SectionHeader
        level={1}
        icon={<Icon name="rocket" size={20} />}
        title={t('nav.deployments')}
        subtitle={t('deployments.subtitle', { project })}
        actions={
          <>
            <Button
              variant="ghost"
              class="shrink-0"
              leadingIcon={<Icon name="lock" size={16} />}
              onClick={() => (freeze.frozen ? void toggleFreeze(false) : setFreezeOpen(true))}
              loading={freezeBusy && freeze.frozen}
            >
              {freeze.frozen ? t('deployments.unfreeze') : t('deployments.freeze')}
            </Button>
            <Button
              variant="ghost"
              class="shrink-0"
              leadingIcon={<Icon name="code" size={16} />}
              disabled={freeze.frozen}
              onClick={() => {
                setRefErrors({});
                setRefOpen(true);
              }}
            >
              {t('deployments.ref.open')}
            </Button>
            <Button
              variant="secondary"
              class="shrink-0"
              leadingIcon={<Icon name="plus" size={16} />}
              loading={redeployBusy}
              disabled={freeze.frozen}
              onClick={() => void redeploy()}
            >
              {t('deployments.redeploy')}
            </Button>
          </>
        }
      />
      <div class="cloud-content">
        {/* La congelación se ve donde se despliega, no en una pantalla de ajustes:
            quien va a pulsar «Volver a desplegar» es quien necesita saber que no
            va a llegar, y por qué. */}
        {freeze.frozen ? (
          <InlineHelp tone="warning" title={t('deployments.frozen.title')} class="mb-4">
            <p>
              {freeze.reason
                ? t('deployments.frozen.reason', { reason: freeze.reason })
                : t('deployments.frozen.noReason')}
              {freeze.frozenAt
                ? ` · ${t('deployments.frozen.since', { when: relativeTime(locale, freeze.frozenAt) })}`
                : ''}
            </p>
            <p>{t('deployments.frozen.help')}</p>
          </InlineHelp>
        ) : null}

        <DeliveryStats items={items} t={t} locale={locale} />

        <div class="mb-3 flex flex-wrap items-center gap-2">
          <Input
            type="search"
            value={q}
            class="w-72 min-w-0"
            leadingIcon={<Icon name="search" size={16} />}
            placeholder={t('deployments.filter.search')}
            aria-label={t('deployments.filter.search')}
            onInput={(e) => setQ(e.currentTarget.value)}
          />
          <Select
            value={estado}
            class="w-48"
            aria-label={t('deployments.filter.status')}
            onChange={(e) => setEstado(e.currentTarget.value as DeployStatus | '')}
          >
            <option value="">{t('deployments.filter.allStatuses')}</option>
            {ESTADOS.map((s) => (
              <option key={s} value={s}>
                {deployStatusLabel(t, s)}
              </option>
            ))}
          </Select>
          {filtrando ? (
            <span class="text-text-muted text-sm tabular-nums" aria-live="polite">
              {t('deployments.filter.count', { count: visibles.length, total: items.length })}
            </span>
          ) : null}
        </div>

        <Tabs
          items={[
            {
              id: 'all',
              label: t('deployments.tab.all', { count: visibles.length }),
              content: list(visibles),
            },
            {
              id: 'prod',
              label: t('deployments.tab.production', { count: production.length }),
              content: list(production),
            },
            {
              id: 'preview',
              label: t('deployments.tab.previews', { count: previews.length }),
              content: list(previews),
            },
          ]}
        />
      </div>

      {/* Congelar: el motivo es opcional, pero es lo que leerá quien se encuentre
          el aviso, así que se pide aquí y no se esconde en otro sitio. */}
      <Modal
        open={freezeOpen}
        onClose={() => (freezeBusy ? undefined : setFreezeOpen(false))}
        title={t('deployments.freeze.title')}
        size="sm"
        footer={
          <>
            <Button variant="ghost" disabled={freezeBusy} onClick={() => setFreezeOpen(false)}>
              {t('common.cancel')}
            </Button>
            <Button variant="primary" loading={freezeBusy} onClick={() => void toggleFreeze(true)}>
              {t('deployments.freeze')}
            </Button>
          </>
        }
      >
        <p class="text-text-muted mb-3 text-sm">{t('deployments.freeze.desc')}</p>
        <FieldGroup
          label={t('deployments.freeze.reason')}
          htmlFor="freeze-reason"
          help={t('deployments.freeze.reason.help')}
        >
          <Textarea
            id="freeze-reason"
            rows={2}
            maxLength={280}
            value={freezeReason}
            onInput={(e) => setFreezeReason(e.currentTarget.value)}
          />
        </FieldGroup>
      </Modal>

      {/* Desplegar una rama o un commit: el hotfix que hoy obligaba a cambiar la
          rama de producción en ajustes, desplegar y acordarse de volver a ponerla. */}
      <Modal
        open={refOpen}
        onClose={() => (redeployBusy ? undefined : setRefOpen(false))}
        title={t('deployments.ref.title')}
        size="sm"
        footer={
          <>
            <Button variant="ghost" disabled={redeployBusy} onClick={() => setRefOpen(false)}>
              {t('common.cancel')}
            </Button>
            <Button type="submit" form="deploy-ref-form" variant="primary" loading={redeployBusy}>
              {t('deployments.ref.submit')}
            </Button>
          </>
        }
      >
        <form id="deploy-ref-form" class="flex flex-col gap-3" onSubmit={(e) => void submitRef(e)}>
          <p class="text-text-muted text-sm">{t('deployments.ref.desc')}</p>
          <FieldGroup
            label={t('deployments.ref.ref')}
            htmlFor="deploy-ref"
            {...(refErrors.ref ? { error: refErrors.ref } : {})}
          >
            <Input
              id="deploy-ref"
              value={refValue}
              class="font-mono"
              placeholder="hotfix/pago"
              invalid={Boolean(refErrors.ref)}
              onInput={(e) => setRefValue(e.currentTarget.value)}
            />
          </FieldGroup>
          <FieldGroup
            label={t('deployments.ref.commit')}
            htmlFor="deploy-commit"
            help={t('deployments.ref.commit.help')}
            {...(refErrors.commit ? { error: refErrors.commit } : {})}
          >
            <Input
              id="deploy-commit"
              value={commitValue}
              class="font-mono"
              placeholder="a1b2c3d"
              invalid={Boolean(refErrors.commit)}
              onInput={(e) => setCommitValue(e.currentTarget.value)}
            />
          </FieldGroup>
        </form>
      </Modal>
    </>
  );
}
