/**
 * DeploymentsPanel — historial de despliegues estilo Vercel: producción +
 * previews por PR, con estado, commit, autor y tiempo de build. Acciones:
 * visitar, promover a producción y rollback (volver a un despliegue anterior).
 * Filtro producción/previews. Componentes solo de @sytue/ui. §14.
 */
import { useMemo, useState } from 'preact/hooks';
import {
  Card,
  CardBody,
  Button,
  Badge,
  Icon,
  Tabs,
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
import type { Locale } from '@sytue/i18n';
import type { Deployment } from '../lib/types.js';
import type { T } from '../i18n/index.js';
import { useT } from '../i18n/index.js';
import { deployStatusLabel, deployStatusTone, relativeTime } from '../lib/format.js';
import { postBff, notifyDemoIfNeeded } from '../lib/bff-client.js';
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

function DeploymentRow({
  d,
  onPromote,
  onCancel,
  onRollback,
  busyId,
  t,
  locale,
}: {
  d: Deployment;
  onPromote: (d: Deployment) => void;
  onCancel: (d: Deployment) => void;
  onRollback: (d: Deployment) => void;
  busyId: string | null;
  t: T;
  locale: Locale;
}) {
  const busy = busyId === d.id;
  const locked = d.status !== 'ready' || Boolean(d.current);
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
          <Icon name={d.status === 'error' ? 'danger' : 'check'} size={16} />
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
      </div>
      <Badge tone={tone}>{deployStatusLabel(t, d.status)}</Badge>
      <div class="flex items-center gap-1.5">
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
        {/* El menú solo existe cuando hay algo accionable: un despliegue 'ready'
            que no sea ya producción. En la producción actual o en builds en
            curso/con error no se renderiza (evita un menú de ítems muertos). */}
        {!locked ? (
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
            {d.target === 'preview' ? (
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

interface Props {
  project: string;
  deployments: Deployment[];
  live: boolean;
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

function DeploymentsInner({ project, deployments, live: initialLive, locale, error }: Props) {
  const t = useT(locale);
  const toast = useToast();
  const [items, setItems] = useState<Deployment[]>(deployments);
  const [busyId, setBusyId] = useState<string | null>(null);
  const [redeployBusy, setRedeployBusy] = useState(false);

  const production = useMemo(() => items.filter((d) => d.target === 'production'), [items]);
  const previews = useMemo(() => items.filter((d) => d.target === 'preview'), [items]);

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
      setItems((prev) =>
        prev.map((d) => ({
          ...d,
          current:
            d.id === target.id ? true : d.target === 'production' ? false : Boolean(d.current),
        })),
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
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('deployments.updateFail'),
        description: t('common.retry'),
      });
    } finally {
      setBusyId(null);
    }
  }

  async function redeploy() {
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
      branch: src?.branch ?? 'main',
      commitSha: src?.commitSha ?? '',
      // `||`, no `??`: un deployment que no viene de un push trae la cadena
      // VACÍA, y `??` solo cubre null/undefined —la fila saldría sin título—.
      commitMessage: src?.commitMessage || t('deploy.newDeployment'),
      author: src?.author ?? t('deploy.you'),
      url: src?.url ?? '',
      createdAt: new Date().toISOString(),
    };
    setItems((prev) => [optimistic, ...prev]);
    try {
      const res = await postBff<{ ok: boolean; live: boolean }>('/api/deploy', { project });
      notifyDemoIfNeeded(t, toast, res.live);
      toast.notify({
        tone: 'success',
        title: t('toast.deployQueued'),
        description: res.live ? t('deployments.deployQueued.desc') : t('toast.demoNotDeployed'),
      });
    } catch {
      // El encolado falló de verdad: retira la fila optimista.
      setItems((prev) => prev.filter((d) => d.id !== optimistic.id));
      toast.notify({
        tone: 'danger',
        title: t('toast.deployFailed'),
        description: t('common.retry'),
      });
    } finally {
      setRedeployBusy(false);
    }
  }

  const list = (rows: Deployment[]) => (
    <Card>
      <CardBody class="p-0">
        {rows.length === 0 ? (
          <EmptyState
            illustration={<Icon name="rocket" size={24} />}
            title={t('deployments.empty.title')}
            description={t('deployments.empty.desc')}
            action={
              <Button
                variant="secondary"
                size="sm"
                loading={redeployBusy}
                onClick={() => void redeploy()}
              >
                {t('deployments.empty.cta')}
              </Button>
            }
          />
        ) : (
          <ul>
            {rows.map((d) => (
              <DeploymentRow
                key={d.id}
                d={d}
                onPromote={(t) => void applyToProduction(t, 'promote')}
                onRollback={(t) => void applyToProduction(t, 'rollback')}
                onCancel={(t) => void cancelInFlight(t)}
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
          <Button
            variant="secondary"
            class="shrink-0"
            leadingIcon={<Icon name="plus" size={16} />}
            loading={redeployBusy}
            onClick={() => void redeploy()}
          >
            {t('deployments.redeploy')}
          </Button>
        }
      />
      <div class="cloud-content">
        <Tabs
          items={[
            {
              id: 'all',
              label: t('deployments.tab.all', { count: items.length }),
              content: list(items),
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
    </>
  );
}
