/**
 * MembersPanel — panel lateral (RHS) con el roster del canal (Ola 2C). Mismo patrón
 * visual que SearchPanel/MessageListPanel: cabecera sólida bg-header con cerrar,
 * buscador local y cuerpo desplazable con filas densas.
 *
 * Dos modos en el mismo panel (sin modal nuevo: no hay que atrapar el foco dos
 * veces): 'list' con el roster y 'add' con los miembros del EQUIPO que aún no están
 * en el canal. Las acciones por fila («Hacer admin»/«Quitar admin» y «Quitar del
 * canal») van en el menú «…» canónico (MessageMenu) y solo si el usuario puede
 * administrar: así la acción destructiva NUNCA queda pegada a otro objetivo de
 * pulsación (WCAG 2.5.8) y hace falta un segundo clic deliberado.
 *
 * ponytail: sin diálogo de confirmación al expulsar — el menú ya son dos pasos y
 * volver a añadir está a un clic en el mismo panel. Añadirlo si aparece un incidente.
 * ponytail: la fila propia no ofrece «Salir del canal»; salir ya existe como
 * «/leave» y evita el estado «me he echado a mí mismo» (el canal desaparecería del
 * sidebar a medio panel).
 */
import type { JSX } from 'preact';
import { useMemo, useState } from 'preact/hooks';
import { Avatar, Badge, Button, Icon, Input, Skeleton } from '@sytue/ui';
import { ChatHttpError, type ChannelMember } from '../client/chatApi.js';
import type { PresenceStatus } from '@sytue/config';
import { toPresenceStatus, type Presence } from '../client/presenceApi.js';
import type { T } from '../i18n/index.js';
import { pareceIdentificador } from '../lib/personas.js';
import { addableMembers, filterMembers } from '../lib/roster.js';
import { BuscarPersona } from './BuscarPersona.js';
import { MessageMenu, type MenuEntry } from './MessageMenu.js';

interface Props {
  /** Nombre del canal (subtítulo de la cabecera). */
  channelName: string;
  /** Roster del canal; el `role` es el rol DE CANAL. */
  members: ChannelMember[];
  /** Miembros del equipo (los mismos del selector de menciones): fuente de «Añadir». */
  teamMembers: ChannelMember[];
  loading: boolean;
  error: boolean;
  /** ¿Puede el usuario administrar el roster? (admin del canal o del team). */
  canManage: boolean;
  /** Sub del usuario de la sesión: marca su fila y le oculta las acciones sobre sí. */
  selfSub: string;
  onAdd: (m: ChannelMember) => Promise<unknown>;
  onRemove: (m: ChannelMember) => Promise<unknown>;
  onSetRole: (m: ChannelMember, role: 'admin' | 'member') => Promise<unknown>;
  /** Subs bloqueados por el usuario (ADR 0073). */
  blockedSubs?: ReadonlySet<string>;
  /** Abre la denuncia de esa persona. Cualquiera puede, no solo quien administra. */
  onReport?: (m: ChannelMember) => void;
  onToggleBlock?: (m: ChannelMember, blocked: boolean) => void;
  onRetry: () => void;
  onClose: () => void;
  /** Presencia conocida por sub (la del Hub): el punto del avatar de cada fila. */
  presences?: Record<string, Presence>;
  /** Traductor ya atado al idioma. */
  t: T;
}

/** Traduce el fallo de una acción: los 4xx que el BFF preserva del servicio Go. */
function actionMessage(t: T, err: unknown): string {
  if (err instanceof ChatHttpError) {
    if (err.status === 403) return t('members.errorForbidden');
    if (err.status === 409) return t('members.errorArchived');
    if (err.status === 404) return t('members.errorGone');
  }
  return t('members.errorGeneric');
}

export function MembersPanel({
  channelName,
  members,
  teamMembers,
  loading,
  error,
  canManage,
  selfSub,
  onAdd,
  onRemove,
  onSetRole,
  blockedSubs,
  onReport,
  onToggleBlock,
  onRetry,
  onClose,
  presences = {},
  t,
}: Props): JSX.Element {
  // El punto de presencia de una persona, si el Hub la conoce.
  const presencia = (sub: string): { presence?: PresenceStatus } => {
    const p = presences[sub];
    return p ? { presence: toPresenceStatus(p.state) } : {};
  };
  const [mode, setMode] = useState<'list' | 'add'>('list');
  const [query, setQuery] = useState('');
  const [busySub, setBusySub] = useState<string | null>(null);
  const [actionError, setActionError] = useState('');
  // Último cambio aplicado, para anunciarlo por aria-live (no solo visual).
  const [notice, setNotice] = useState('');

  const shown = useMemo(() => filterMembers(members, query), [members, query]);
  const candidates = useMemo(
    () => addableMembers(teamMembers, members, query),
    [teamMembers, members, query],
  );

  /** Lanza una acción del roster: un solo vuelo a la vez por fila, con su aviso. */
  function run(m: ChannelMember, action: () => Promise<unknown>, done: string): void {
    setBusySub(m.sub);
    setActionError('');
    setNotice('');
    action()
      .then(() => setNotice(done))
      .catch((e: unknown) => setActionError(actionMessage(t, e)))
      .finally(() => setBusySub(null));
  }

  function switchMode(next: 'list' | 'add'): void {
    setMode(next);
    setQuery('');
    setActionError('');
    setNotice('');
  }

  const statusText = loading
    ? t('members.loading')
    : error
      ? t('members.loadFailed')
      : mode === 'add'
        ? t('members.availableCount', { count: candidates.length })
        : t('members.count', { count: members.length });

  return (
    // onKeyDown a nivel de panel: Escape cierra desde cualquier hijo (burbujeo).
    // eslint-disable-next-line jsx-a11y/no-noninteractive-element-interactions
    <aside
      class="sytue-sheet-corners border-border bg-surface marco:overflow-clip flex h-full w-full shrink-0 flex-col border-l md:w-[var(--rooms-rhs-width)] md:[--sytue-sheet-bl:0px] md:[--sytue-sheet-tl:0px]"
      aria-label={t('members.aria')}
      onKeyDown={(e) => {
        if (e.key === 'Escape') {
          e.stopPropagation();
          if (mode === 'add') switchMode('list');
          else onClose();
        }
      }}
    >
      <header class="bg-header text-header-fg flex h-14 shrink-0 items-center gap-2 px-4">
        {mode === 'add' ? (
          <button
            type="button"
            aria-label={t('members.back')}
            title={t('action.back')}
            onClick={() => switchMode('list')}
            class="text-header-fg hover:bg-chrome-hover sytue-focus-ring rounded-control -ml-1 inline-flex h-8 w-8 shrink-0 items-center justify-center transition-colors duration-150"
          >
            <Icon name="chevronLeft" size={18} />
          </button>
        ) : null}
        <div class="flex min-w-0 flex-1 flex-col justify-center">
          <h2 class="text-md text-header-fg truncate leading-tight font-semibold">
            {mode === 'add' ? t('members.addPeople') : t('members.title')}
          </h2>
          <p class="text-header-fg/80 truncate text-xs leading-tight" title={channelName}>
            #{channelName}
          </p>
        </div>
        <button
          type="button"
          aria-label={t('members.close')}
          title={t('action.close')}
          onClick={onClose}
          class="text-header-fg hover:bg-chrome-hover sytue-focus-ring rounded-control inline-flex h-8 w-8 shrink-0 items-center justify-center transition-colors duration-150"
        >
          <Icon name="close" size={18} />
        </button>
      </header>

      <div class="border-border flex shrink-0 flex-col gap-2 border-b p-3">
        <Input
          type="search"
          value={query}
          placeholder={mode === 'add' ? t('members.searchTeam') : t('members.searchMembers')}
          leadingIcon={<Icon name="search" size={16} />}
          onInput={(e) => setQuery(e.currentTarget.value)}
          aria-label={mode === 'add' ? t('members.searchTeamAria') : t('members.searchMembersAria')}
        />
        {/* Alguien de OTRA organización, por su correo o su @usuario (ADR 0040). El
            nombre con el que entra lo pone Sytue ID en el servidor, no este panel. */}
        {mode === 'add' ? (
          <BuscarPersona
            query={query}
            onFound={(p) => {
              const m: ChannelMember = { ...p, external: true };
              setQuery('');
              // De otra organización: se le invita y entra al aceptar.
              run(m, () => onAdd(m), t('members.invited', { name: p.name }));
            }}
            disabled={busySub !== null}
            t={t}
          />
        ) : null}
        {canManage && mode === 'list' ? (
          <Button
            size="sm"
            variant="secondary"
            leadingIcon={<Icon name="plus" size={16} />}
            onClick={() => switchMode('add')}
            class="w-full"
          >
            {t('members.addPeople')}
          </Button>
        ) : null}
      </div>

      {/* Conteo + resultado de la última acción: nunca solo visual (aria-live). */}
      <div class="text-text-muted shrink-0 px-4 py-1.5 text-xs" role="status" aria-live="polite">
        {actionError ? <span class="text-danger-text">{actionError}</span> : notice || statusText}
      </div>

      <div class="min-h-0 flex-1 overflow-y-auto pb-2">
        {error ? (
          <div class="text-danger-text px-4 py-3 text-sm">
            <p class="flex items-center gap-1.5">
              <Icon name="warning" size={14} />
              <span>{t('members.loadFailed')}</span>
              <button
                type="button"
                class="hover:text-text sytue-focus-ring rounded-sm font-semibold underline underline-offset-2"
                onClick={onRetry}
              >
                {t('action.retry')}
              </button>
            </p>
          </div>
        ) : loading ? (
          <div class="flex flex-col gap-3 px-4 py-2" aria-busy="true">
            {[55, 42, 60].map((w, i) => (
              <div key={i} class="flex items-center gap-3">
                <Skeleton width="2rem" height="2rem" class="shrink-0" />
                <Skeleton width={`${w}%`} height="0.75rem" />
              </div>
            ))}
          </div>
        ) : mode === 'add' ? (
          candidates.length === 0 && pareceIdentificador(query) ? null : candidates.length === 0 ? (
            <p class="text-text-muted px-4 py-6 text-center text-sm">
              {teamMembers.length === 0
                ? t('members.teamLoadFailed')
                : query.trim()
                  ? t('members.teamEmptyQuery')
                  : t('members.teamAllIn')}
            </p>
          ) : (
            candidates.map((m) => (
              <button
                key={m.sub}
                type="button"
                disabled={busySub === m.sub}
                onClick={() => run(m, () => onAdd(m), t('members.added', { name: m.name }))}
                class="hover:bg-bg-subtle sytue-focus-ring-inner flex w-full items-center gap-3 px-4 py-1.5 text-left disabled:opacity-60"
              >
                <Avatar name={m.name} size="sm" {...presencia(m.sub)} />
                <span class="text-text min-w-0 flex-1 truncate text-sm font-semibold">
                  {m.name}
                </span>
                <span class="sytue-meta shrink-0">
                  {busySub === m.sub ? t('members.adding') : t('action.add')}
                </span>
              </button>
            ))
          )
        ) : shown.length === 0 ? (
          <p class="text-text-muted px-4 py-6 text-center text-sm">
            {members.length === 0 ? t('members.empty') : t('members.emptyQuery')}
          </p>
        ) : (
          shown.map((m) => {
            const isSelf = m.sub === selfSub;
            const isAdmin = m.role === 'admin';
            // Sobre uno mismo no se ofrece nada: quitarse el admin dejaría el panel
            // sin acciones a media sesión y salir del canal es «/leave».
            const blocked = blockedSubs?.has(m.sub) ?? false;
            // Denunciar y bloquear son de CUALQUIERA sobre cualquier otra persona;
            // el rol y la expulsión, solo de quien administra.
            const propias: MenuEntry[] = [
              ...(onReport
                ? [
                    {
                      key: 'report',
                      label: t('report.person', { name: m.name }),
                      danger: true,
                      onSelect: () => onReport(m),
                    },
                  ]
                : []),
              ...(onToggleBlock
                ? [
                    {
                      key: 'block',
                      label: t(blocked ? 'block.unblock' : 'block.block', { name: m.name }),
                      danger: !blocked,
                      onSelect: () => onToggleBlock(m, blocked),
                    },
                  ]
                : []),
            ];
            const admin: MenuEntry[] = [
              {
                key: 'role',
                label: isAdmin ? t('members.removeAdmin') : t('members.makeAdmin'),
                onSelect: () =>
                  run(
                    m,
                    () => onSetRole(m, isAdmin ? 'member' : 'admin'),
                    isAdmin
                      ? t('members.adminRemoved', { name: m.name })
                      : t('members.adminGranted', { name: m.name }),
                  ),
              },
              'separator',
              {
                key: 'remove',
                label: t('members.removeFromChannel'),
                danger: true,
                onSelect: () => run(m, () => onRemove(m), t('members.removed', { name: m.name })),
              },
            ];
            const items: MenuEntry[] = canManage
              ? [...admin, ...(propias.length > 0 ? ['separator' as const, ...propias] : [])]
              : propias;
            return (
              <div key={m.sub} class="hover:bg-bg-subtle flex items-center gap-3 px-4 py-1.5">
                <Avatar name={m.name} size="sm" {...presencia(m.sub)} />
                <span class="text-text min-w-0 flex-1 truncate text-sm font-semibold" title={m.sub}>
                  {m.name}
                  {isSelf ? (
                    <span class="text-text-subtle font-normal"> {t('members.you')}</span>
                  ) : null}
                </span>
                {m.external ? (
                  <Badge tone="neutral" class="shrink-0">
                    {t('people.external')}
                  </Badge>
                ) : null}
                {isAdmin ? (
                  <Badge tone="neutral" class="shrink-0">
                    {t('members.admin')}
                  </Badge>
                ) : null}
                {!isSelf && items.length > 0 ? (
                  <MessageMenu label={t('members.actionsOn', { name: m.name })} items={items} />
                ) : null}
              </div>
            );
          })
        )}
      </div>
    </aside>
  );
}
