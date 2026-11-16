/**
 * ThreadPanel — panel lateral de HILO de Sytue Chat (estilo Mattermost): mensaje
 * raíz + respuestas en orden cronológico + composer de respuesta. Se abre desde el
 * indicador «N respuestas» de un mensaje. Carga por REST (BFF /api/thread); las
 * respuestas nuevas llegan por WebSocket (las inserta el padre). docs/design.md §7.
 *
 * Cada mensaje del hilo (raíz y respuestas) se maneja igual que en el canal: barra
 * flotante con reacciones rápidas y el mismo menú «…» (`AccionesDeMensaje`), chips de
 * reacción, edición inline y borrado con confirmación de los propios, y «Reintentar»
 * en una respuesta que no se pudo enviar.
 */
import type { JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import { BlockedFold } from './Moderation.js';
import {
  Avatar,
  Skeleton,
  Icon,
  cx,
  IconButton,
  Checkbox,
  Toolbar,
  ConfirmDialog,
} from '@sytue/ui';
import { formatTime, type Locale } from '@sytue/i18n';
import type { ChatMessage, ChannelMember, Attachment } from '../client/chatApi.js';
import type { T } from '../i18n/index.js';
import { MessageBody } from './MessageBody.js';
import { CardView } from './CardView.js';
import { Attachments } from './Attachments.js';
import { Composer } from './Composer.js';
import { ThreadsGlyph } from './glyphs.js';
import { MessageMenu } from './MessageMenu.js';
import {
  AvisoDeEnvioFallido,
  CajaDeEdicion,
  FilaDeReacciones,
  ReaccionesRapidas,
  entradasDelMenu,
  useCopiarConAviso,
  type ContextoDelMenu,
} from './AccionesDeMensaje.js';
import { revelaPorToque } from '../lib/popover.js';

/** Acciones sobre un mensaje del hilo: las mismas que ofrece el canal, salvo las que
 *  son del canal y no de una respuesta («Marcar como no leído», «Visto por»). */
export interface AccionesDelHilo {
  /** Alterna una reacción del usuario sobre un mensaje del hilo. */
  onReact?: (messageId: string, emoji: string) => void;
  /** Guarda la edición del cuerpo de un mensaje propio. */
  onEdit?: (messageId: string, body: string) => void;
  /** Borra un mensaje propio (tras confirmarlo en el panel). */
  onDelete?: (messageId: string) => void;
  /** Reintenta el envío de una respuesta fallida. */
  onResend?: (message: ChatMessage) => void;
  pinnedIds?: Set<string>;
  savedIds?: Set<string>;
  onTogglePin?: (message: ChatMessage, pinned: boolean) => void;
  onToggleSave?: (message: ChatMessage, saved: boolean) => void;
  onForward?: (message: ChatMessage) => void;
  onReport?: (message: ChatMessage) => void;
  onToggleBlock?: (message: ChatMessage, blocked: boolean) => void;
}

interface Props extends AccionesDelHilo {
  root: ChatMessage;
  replies: ChatMessage[];
  loading: boolean;
  error?: boolean;
  members?: ChannelMember[];
  /** sub -> nombre legible, para resaltar @menciones. */
  memberNames?: Record<string, string>;
  channelName?: string;
  /** ¿El usuario sigue este hilo? Refleja el botón Seguir/Siguiendo (Ola 9). */
  following?: boolean;
  /** Alterna seguir/dejar de seguir el hilo. */
  onToggleFollow?: () => void;
  onClose: () => void;
  onRetry?: () => void;
  /** `alsoToChannel`: la persona marcó «También enviar a #canal». */
  onSendReply: (
    body: string,
    mentions: string[],
    attachments: Attachment[],
    alsoToChannel: boolean,
  ) => void;
  /** Marca o retira un valor de la tarjeta de un mensaje del hilo (una encuesta se
   *  responde igual de bien desde aquí que desde el canal). */
  onCardVote?: (messageId: string, value: string, add: boolean) => void;
  /** La conversación va cifrada: las tarjetas, en solo lectura (`lib/cifrado.ts`). */
  cifrado?: boolean;
  /** Subs bloqueados por el usuario: sus mensajes van plegados (ADR 0073). */
  blockedSubs?: ReadonlySet<string>;
  /** Scope de borrador por cuenta; sin red se redacta localmente, pero no se envía. */
  ownerId: string;
  disabled?: boolean;
  offline?: boolean;
  /** Idioma activo (reloj de 12/24 h) y traductor ya atado a él. */
  locale: Locale;
  t: T;
}

/** Hora corta en el idioma del usuario (el reloj de 12/24 h lo decide el locale). */
function timeLabel(locale: Locale, iso: string): string {
  return formatTime(locale, iso, { hour: '2-digit', minute: '2-digit' });
}

function MessageRow({
  m,
  memberNames,
  blocked = false,
  onCardVote,
  cifrado = false,
  acciones,
  menu,
  tocado,
  onTocar,
  onPedirBorrado,
  locale,
  t,
}: {
  m: ChatMessage;
  memberNames?: Record<string, string>;
  /** El autor está bloqueado: el mensaje se ve plegado hasta que se pide verlo. */
  blocked?: boolean;
  onCardVote?: (messageId: string, value: string, add: boolean) => void;
  cifrado?: boolean;
  acciones: AccionesDelHilo;
  /** Contexto del menú «…» común al panel (copiar, fijar, guardar…). */
  menu: ContextoDelMenu;
  /** La barra de acciones de ESTA fila está desplegada por toque. */
  tocado: boolean;
  /** Alterna la barra por toque (en táctil no hay hover). */
  onTocar: () => void;
  onPedirBorrado: (messageId: string) => void;
  locale: Locale;
  t: T;
}): JSX.Element {
  const mentionNames = (m.mentions ?? []).map((s) => memberNames?.[s] ?? s);
  const [mostrado, setMostrado] = useState(false);
  const [editando, setEditando] = useState(false);
  const [borrador, setBorrador] = useState('');
  const filaRef = useRef<HTMLDivElement>(null);
  const ultimoPunteroRef = useRef('');
  const cajaRef = useRef<HTMLTextAreaElement | null>(null);
  // El `autofocus` solo cuenta al cargar la página: la caja que aparece después se
  // enfoca a mano, con el cursor al final, UNA vez al abrirse (si se enfocara en cada
  // render, una trama que llega mientras se edita robaría el foco a «Guardar»).
  // Al cerrar la edición, la barra (y su «…») vuelve a montarse en ESTE render: el
  // foco se devuelve después de pintarla, no en el manejador que la cierra (allí el
  // «…» aún no existe y el foco caía al <body>).
  const volverAlMenuRef = useRef(false);
  useEffect(() => {
    if (editando) {
      const el = cajaRef.current;
      if (!el) return;
      el.focus();
      el.setSelectionRange(el.value.length, el.value.length);
    } else if (volverAlMenuRef.current) {
      volverAlMenuRef.current = false;
      filaRef.current?.querySelector<HTMLElement>('[aria-haspopup="menu"]')?.focus();
    }
  }, [editando]);
  if (blocked && !mostrado) {
    return (
      <div class="px-4 py-2">
        <BlockedFold t={t} onShow={() => setMostrado(true)} />
      </div>
    );
  }
  const { onReact, onEdit, onDelete, onResend } = acciones;
  const vivo = !m.pending && !m.failed;

  // Al cerrar la edición el textarea se desmonta: el foco vuelve al «…» de la fila
  // (de donde se abrió) en vez de caer al <body> (design.md §2.6).
  function cerrarEdicion(): void {
    volverAlMenuRef.current = true;
    setEditando(false);
  }
  function guardar(): void {
    const body = borrador.trim();
    if (body && body !== m.body) onEdit?.(m.id, body);
    cerrarEdicion();
  }

  const items = entradasDelMenu(m, {
    ...menu,
    pinned: acciones.pinnedIds?.has(m.id) ?? false,
    saved: acciones.savedIds?.has(m.id) ?? false,
    blocked,
    onEdit: onEdit
      ? (msg) => {
          setBorrador(msg.body);
          setEditando(true);
        }
      : undefined,
    onDelete: onDelete ? (msg) => onPedirBorrado(msg.id) : undefined,
  });

  return (
    <div
      ref={filaRef}
      class="group hover:bg-bg-subtle relative flex gap-3 px-4 py-2 transition-colors duration-150"
    >
      {/* Barra flotante como la del canal: al pasar el cursor, al foco por teclado
          (opacity, no display:none, para que siga siendo tabulable) y, con el dedo,
          al tocar el mensaje. */}
      {vivo && !editando ? (
        <Toolbar
          class={cx(
            'sytue-popover-surface border-border absolute -top-2 right-3 z-10 flex items-center gap-0.5 rounded-md border p-0.5 shadow-[var(--shadow-sm)] transition-opacity duration-150 group-hover:pointer-events-auto group-hover:opacity-100 focus-within:pointer-events-auto focus-within:opacity-100',
            tocado ? 'opacity-100' : 'pointer-events-none opacity-0',
          )}
          aria-label={t('composer.messageActions')}
        >
          {onReact ? <ReaccionesRapidas m={m} onReact={onReact} t={t} /> : null}
          <MessageMenu label={t('message.moreActions')} items={items} />
        </Toolbar>
      ) : null}
      <div class="w-9 shrink-0 pt-0.5">
        <Avatar name={m.author.name} size="sm" />
      </div>
      {/* Tocar el mensaje despliega su barra (sustituto TÁCTIL del hover, no un
          control: con teclado la abre `focus-within`). */}
      {/* oxlint-disable-next-line jsx-a11y/no-static-element-interactions, jsx-a11y/click-events-have-key-events */}
      <div
        class="min-w-0 flex-1"
        onPointerDown={(e) => {
          ultimoPunteroRef.current = e.pointerType;
        }}
        onClick={() => {
          if (revelaPorToque(ultimoPunteroRef.current) && !editando) onTocar();
        }}
      >
        <div class="flex min-w-0 items-baseline gap-2">
          <span class="text-text min-w-0 truncate text-sm font-semibold" title={m.author.name}>
            {m.author.name}
          </span>
          <span class="sytue-meta shrink-0">{timeLabel(locale, m.createdAt)}</span>
        </div>
        {editando ? (
          <CajaDeEdicion
            valor={borrador}
            onInput={setBorrador}
            onGuardar={guardar}
            onCancelar={cerrarEdicion}
            cajaRef={(el) => {
              cajaRef.current = el;
            }}
            t={t}
          />
        ) : (
          <>
            {m.body ? (
              <p
                class={cx(
                  'text-base break-words whitespace-pre-wrap',
                  m.pending ? 'text-text-muted' : 'text-text',
                )}
              >
                <MessageBody body={m.body} authorName={m.author.name} mentionNames={mentionNames} />
                {m.editedAt ? (
                  <span class="sytue-meta ml-1" title={t('message.editedTitle')}>
                    {t('message.edited')}
                  </span>
                ) : null}
                {m.pending ? <span class="sytue-meta ml-1">{t('message.sending')}</span> : null}
              </p>
            ) : null}
            <Attachments
              attachments={m.attachments}
              channelId={m.channelId}
              t={t}
              locale={locale}
            />
            {m.card ? (
              <CardView
                card={m.card}
                messageId={m.id}
                {...(onCardVote && vivo ? { onVote: onCardVote } : {})}
                cifrado={cifrado}
                locale={locale}
                t={t}
              />
            ) : null}
            {!m.body && m.pending ? <span class="sytue-meta">{t('message.sending')}</span> : null}
          </>
        )}
        {/* Envío fallido: aviso con icono + reintento (nunca solo color), como en
            el canal. */}
        {m.failed ? <AvisoDeEnvioFallido m={m} onResend={onResend} t={t} /> : null}
        {(m.reactions?.length ?? 0) > 0 ? (
          <div class="mt-1 flex flex-wrap items-center gap-1">
            <FilaDeReacciones
              m={m}
              onReact={vivo ? onReact : undefined}
              memberNames={memberNames}
              locale={locale}
              t={t}
            />
          </div>
        ) : null}
      </div>
    </div>
  );
}

export function ThreadPanel({
  root,
  replies,
  loading,
  error = false,
  members,
  memberNames,
  blockedSubs,
  channelName,
  following = false,
  onToggleFollow,
  onClose,
  onRetry,
  onSendReply,
  onCardVote,
  cifrado = false,
  ownerId,
  disabled = false,
  offline = false,
  locale,
  t,
  ...acciones
}: Props): JSX.Element {
  const endRef = useRef<HTMLDivElement>(null);
  const copiar = useCopiarConAviso(t);
  // Fila con la barra desplegada por toque (null = ninguna) y mensaje cuyo borrado
  // se está confirmando: uno a la vez en todo el panel, como en el canal.
  const [tocado, setTocado] = useState<string | null>(null);
  const [borrando, setBorrando] = useState<string | null>(null);
  const menu: ContextoDelMenu = {
    t,
    copiar,
    onTogglePin: acciones.onTogglePin,
    onToggleSave: acciones.onToggleSave,
    onForward: acciones.onForward,
    onReport: acciones.onReport,
    onToggleBlock: acciones.onToggleBlock,
  };
  const fila = (m: ChatMessage): JSX.Element => (
    <MessageRow
      key={m.id}
      m={m}
      memberNames={memberNames ?? {}}
      blocked={blockedSubs?.has(m.author.id) ?? false}
      {...(onCardVote ? { onCardVote } : {})}
      cifrado={cifrado}
      acciones={acciones}
      menu={menu}
      tocado={tocado === m.id}
      onTocar={() => setTocado((prev) => (prev === m.id ? null : m.id))}
      onPedirBorrado={setBorrando}
      locale={locale}
      t={t}
    />
  );

  // Auto-scroll al llegar respuestas nuevas (el hilo es corto; siempre al final).
  useEffect(() => {
    endRef.current?.scrollIntoView({ block: 'end' });
  }, [replies.length]);

  const count = replies.length;
  // «También enviar a #canal» (Slack): se desmarca tras cada envío, porque difundir
  // es la excepción y dejarlo puesto mandaría al canal la conversación entera.
  const [alCanal, setAlCanal] = useState(false);
  // Otro hilo, casilla limpia: la elección era para la respuesta de aquel.
  useEffect(() => setAlCanal(false), [root.id]);

  return (
    <aside
      class="sytue-sheet-corners border-border bg-surface marco:overflow-clip flex h-full w-full shrink-0 flex-col border-l md:w-[var(--rooms-rhs-width)] md:[--sytue-sheet-bl:0px] md:[--sytue-sheet-tl:0px]"
      aria-label={t('thread.title')}
    >
      {/* Cabecera del hilo con el acento sólido del producto (bg-header), a juego
          con la topbar y la cabecera de canal. docs/design.md §2. */}
      <header class="bg-header text-header-fg flex h-14 shrink-0 items-center justify-between gap-2 px-4">
        <div class="flex min-w-0 flex-col justify-center">
          <h2 class="text-md text-header-fg leading-tight font-semibold">{t('thread.title')}</h2>
          {channelName ? (
            <p class="text-header-fg/80 truncate text-xs leading-tight">#{channelName}</p>
          ) : null}
        </div>
        <div class="flex shrink-0 items-center gap-1">
          {/* Seguir / Siguiendo el hilo (Ola 9): aria-pressed refleja el estado. */}
          {onToggleFollow ? (
            <button
              type="button"
              aria-pressed={following}
              aria-label={following ? t('thread.unfollow') : t('thread.follow')}
              title={following ? t('thread.followingHint') : t('thread.followHint')}
              onClick={onToggleFollow}
              class={cx(
                'text-header-fg hover:bg-chrome-hover sytue-focus-ring rounded-control inline-flex h-8 shrink-0 items-center gap-1.5 px-2.5 text-sm font-medium transition-colors duration-150',
                following && 'bg-chrome-active',
              )}
            >
              <ThreadsGlyph size={16} />
              <span>{following ? t('thread.following') : t('thread.followShort')}</span>
            </button>
          ) : null}
          {/* Botón sobre la cabecera de CROMO grafito: el hover/active son velos
              blancos (bg-chrome-*), no grises claros —sobre grafito no se ven—. */}
          <IconButton size="lg" tone="header" label={t('thread.close')} onClick={onClose}>
            <Icon name="close" size={18} />
          </IconButton>
        </div>
      </header>

      <div class="min-h-0 flex-1 overflow-y-auto py-2">
        {/* Mensaje raíz, destacado. */}
        <div class="border-border border-b pb-1">{fila(root)}</div>

        {/* Separador con el conteo de respuestas. */}
        <div class="my-2 flex items-center gap-3 px-4" role="separator">
          <span class="text-text-muted shrink-0 text-xs font-semibold">
            {count === 0 ? t('thread.noReplies') : t('thread.replyCount', { count })}
          </span>
          <span class="bg-border h-px flex-1" />
        </div>

        {error ? (
          <div role="alert" class="text-danger-text px-4 py-3 text-sm">
            <p class="flex items-center gap-1.5">
              <Icon name="warning" size={14} />
              <span>{t('thread.loadFailed')}</span>
              {onRetry ? (
                <button
                  type="button"
                  class="hover:text-text sytue-focus-ring rounded-sm font-semibold underline underline-offset-2"
                  onClick={onRetry}
                >
                  {t('action.retry')}
                </button>
              ) : null}
            </p>
          </div>
        ) : loading ? (
          <div class="flex flex-col gap-3 px-4 py-2" aria-busy="true">
            <span role="status" class="sr-only">
              {t('thread.loading')}
            </span>
            {[60, 45].map((w, i) => (
              <div key={i} class="flex gap-3">
                <Skeleton width="2rem" height="2rem" class="shrink-0" />
                <div class="flex min-w-0 flex-1 flex-col gap-2 pt-1">
                  <Skeleton width="6rem" height="0.75rem" />
                  <Skeleton width={`${w}%`} height="0.75rem" />
                </div>
              </div>
            ))}
          </div>
        ) : (
          replies.map(fila)
        )}
        <div ref={endRef} />
      </div>

      <Composer
        // key + draftId propios del hilo: borrador aparte del canal y sin arrastrar
        // texto entre hilos al cambiar de raíz (Ola 8).
        key={root.id}
        channelName={channelName ?? ''}
        members={members ?? []}
        draftId={`${ownerId}:${root.channelId}:thread:${root.id}`}
        disabled={disabled}
        offline={offline}
        channelId={root.channelId}
        placeholder={t('thread.replyPlaceholder')}
        ariaLabel={t('thread.replyAria')}
        onSend={(body, mentions, attachments) => {
          onSendReply(body, mentions, attachments, alCanal);
          setAlCanal(false);
        }}
        locale={locale}
        t={t}
      />
      {channelName ? (
        <label class="text-text-muted flex min-h-11 cursor-pointer items-center gap-2 px-4 pb-2 text-sm md:min-h-8">
          <Checkbox
            checked={alCanal}
            disabled={disabled}
            onChange={(e) => setAlCanal(e.currentTarget.checked)}
            class="accent-primary h-4 w-4 shrink-0"
          />
          <span class="min-w-0 truncate">
            {t('thread.alsoToChannel', { channel: channelName })}
          </span>
        </label>
      ) : null}
      {/* Borrar se confirma con el diálogo de la casa, como en el canal. */}
      <ConfirmDialog
        open={borrando !== null}
        onClose={() => setBorrando(null)}
        onConfirm={() => {
          if (borrando) acciones.onDelete?.(borrando);
          setBorrando(null);
        }}
        title={t('action.delete')}
        confirmLabel={t('action.delete')}
        tone="danger"
      >
        <p class="text-text text-sm leading-relaxed">{t('message.confirmDelete')}</p>
      </ConfirmDialog>
    </aside>
  );
}
