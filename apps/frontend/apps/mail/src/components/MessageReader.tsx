/**
 * MessageReader — panel de lectura de CONVERSACIÓN tipo Gmail. Cabecera con el
 * asunto del hilo y la barra de acciones (Responder/Reenviar sobre el último
 * mensaje, intents del Hub, triaje del hilo completo). Debajo, los mensajes del
 * hilo apilados (antiguo→reciente): todos plegados salvo el más reciente
 * (expandido); cada uno se expande/pliega y carga su cuerpo (extracto/HTML del
 * SDK) bajo demanda. Usa solo componentes de @sytue/ui. docs/CONVENTIONS.md §11.
 */
import type { JSX } from 'preact';
import { imagenesEnLinea, resolverCidsEnLinea } from '../lib/cid.js';
import { neutralizarProxyEnCuerpo, reescribirImagenesRemotas } from '../lib/remoteImage.js';
import { useEffect, useRef, useState } from 'preact/hooks';
import {
  Avatar,
  Button,
  Card,
  Dropdown,
  EmptyState,
  Icon,
  MenuItem,
  Skeleton,
  useToast,
  Illustration,
  cx,
  useTheme,
} from '@sytue/ui';
import type {
  AttachmentMeta,
  EmailSummary,
  Label,
  Mailbox,
  MessageBody,
  UnsubscribeInfo,
} from '../client/mailApi.js';
import {
  attachmentDownloadUrl,
  rawDownloadUrl,
  remoteImageProxyUrl,
  resolveIntent,
  getMessageBody,
  getUnsubscribe,
  listAttachments,
  unsubscribe,
} from '../client/mailApi.js';
import type { Locale } from '@sytue/i18n';
import { useT, type T } from '../i18n/index.js';
import {
  displayName,
  joinAddresses,
  fullDate,
  shortDate,
  formatBytes,
  mailboxLabel,
  conversationLabels,
  hasRemoteContent,
  labelDot,
  type Conversation,
  type ReplyMode,
} from './utils.js';
import { Glyph } from './glyphs.js';

interface Props {
  /** Lectura pura: responder conserva un borrador, el resto exige servidor. */
  readOnly?: boolean;
  /** Idioma ya negociado en el servidor; llega por props desde la isla raíz. */
  locale: Locale;
  conversation: Conversation | null;
  /**
   * Hilo COMPLETO (reciente→antiguo), atravesando carpetas: incluye lo que
   * `conversation` no puede ver por venir de una sola carpeta —tu respuesta en
   * Enviados, lo archivado—. Ausente ⇒ solo los mensajes de la carpeta.
   *
   * Solo afecta a lo que se MUESTRA. Las acciones (archivar, mover, etiquetar)
   * siguen recibiendo `conversation`, la de la vista: archivar un hilo no debe
   * sacar tu copia de Enviados de su carpeta.
   */
  threadMessages?: EmailSummary[] | null;
  /** Abre el compositor prerelleno para Responder / Responder a todos / Reenviar.
   *  `bodyText` es el cuerpo completo si el lector ya lo cargó (cita íntegra). */
  onReply: (email: EmailSummary, mode: ReplyMode, bodyText?: string) => void;
  /** Abre la redaccion (CTA del estado vacio del lector). */
  onCompose: () => void;
  /** Triaje del hilo: estrella, no leído, archivar/papelera (null = sin destino) y «Mover a…». */
  onToggleFlag: (conv: Conversation) => void;
  onMarkUnread: (conv: Conversation) => void;
  /** «Crear una regla para este remitente»: abre el asistente de reglas ya relleno. */
  onCreateRule?: ((conv: Conversation) => void) | undefined;
  onArchive: ((conv: Conversation) => void) | null;
  onTrash: ((conv: Conversation) => void) | null;
  /** «Marcar como spam» (mueve el hilo a Spam); null si no aplica (ya en Spam). */
  onSpam: ((conv: Conversation) => void) | null;
  /** «No es spam» (devuelve el hilo a Recibidos); null salvo en la carpeta Spam. */
  onNotSpam: ((conv: Conversation) => void) | null;
  /** Borrado definitivo del hilo (solo en papelera/spam); la confirmación es del padre. */
  onPurge: ((conv: Conversation) => void) | null;
  onMove: (conv: Conversation, dest: Mailbox) => void;
  /** Destinos del menú «Mover a…» (sin la carpeta actual ni Borradores). */
  moveTargets: Mailbox[];
  /** Registro de etiquetas de la cuenta (para el menú «Etiquetar» y los chips). */
  labels: Label[];
  /** Aplica/quita una etiqueta a TODO el hilo abierto (N mensajes, en lote). */
  onToggleLabel: (conv: Conversation, labelId: string, add: boolean) => void;
}

export function MessageReader({
  readOnly = false,
  locale,
  conversation,
  threadMessages = null,
  onReply,
  onCompose,
  onToggleFlag,
  onMarkUnread,
  onCreateRule,
  onArchive,
  onTrash,
  onSpam,
  onNotSpam,
  onPurge,
  onMove,
  moveTargets,
  labels,
  onToggleLabel,
}: Props): JSX.Element {
  const t = useT(locale);
  const toast = useToast();
  const [creatingEvent, setCreatingEvent] = useState(false);

  // Mensajes que se PINTAN, reciente→antiguo: el hilo completo si el servidor lo
  // dio (atraviesa carpetas), y si no los de la carpeta. Las acciones siguen
  // yendo contra `conversation` (ver la prop threadMessages).
  const shown = threadMessages ?? conversation?.messages ?? [];

  // Último mensaje del hilo (más reciente): define asunto, acciones y cita de la
  // respuesta. Se carga su cuerpo a nivel de lector para la cita y para el
  // mensaje expandido por defecto (evita doble carga: se le pasa como prefetch).
  // Sale del hilo completo: responder cita lo ÚLTIMO que se dijo, aunque lo
  // último fuera tu propia respuesta desde Enviados.
  const latest = shown[0] ?? conversation?.latest ?? null;
  const latestId = latest?.id ?? null;
  const [latestBody, setLatestBody] = useState<MessageBody | null>(null);
  useEffect(() => {
    setLatestBody(null);
    if (!latestId) return;
    let vigente = true;
    getMessageBody(latestId)
      .then((b) => vigente && setLatestBody(b))
      .catch(() => {
        /* degrada al extracto */
      });
    return () => {
      vigente = false;
    };
  }, [latestId]);

  // ¿Ofrece el correo darse de baja? (List-Unsubscribe / RFC 8058). Solo booleanos:
  // la URL/dirección nunca llega al navegador; el POST la ejecuta por id en el server.
  const [unsub, setUnsub] = useState<UnsubscribeInfo | null>(null);
  const [unsubBusy, setUnsubBusy] = useState(false);
  useEffect(() => {
    setUnsub(null);
    if (!latestId) return;
    let vigente = true;
    getUnsubscribe(latestId)
      .then((u) => vigente && setUnsub(u))
      .catch(() => {
        /* sin método de baja: no se muestra el botón */
      });
    return () => {
      vigente = false;
    };
  }, [latestId]);

  // Darse de baja: el servidor elige el método seguro (one-click https SSRF-safe o
  // enviar el correo al mailto). Muestra el resultado sin romper la vista.
  async function handleUnsubscribe(): Promise<void> {
    if (!latest || unsubBusy) return;
    setUnsubBusy(true);
    try {
      const { status } = await unsubscribe(latest.id);
      if (status === 'failed') {
        toast.notify({ tone: 'danger', title: t('reader.unsubscribeFailed') });
      } else {
        toast.notify({
          tone: 'success',
          title: status === 'sent' ? t('reader.unsubscribeSent') : t('reader.unsubscribeDone'),
        });
        setUnsub(null); // ya no hay acción pendiente: oculta el botón
      }
    } catch {
      toast.notify({ tone: 'danger', title: t('reader.unsubscribeFailed') });
    } finally {
      setUnsubBusy(false);
    }
  }

  if (!conversation || !latest) {
    return (
      <EmptyState
        class="h-full"
        title={t('reader.emptyTitle')}
        description={t('reader.emptyDescription')}
        illustration={<Illustration name="emptyMail" />}
        action={
          <Button
            variant="primary"
            size="sm"
            leadingIcon={<Icon name="plus" />}
            onClick={onCompose}
          >
            {t('nav.compose')}
          </Button>
        }
      />
    );
  }

  // Intent del Hub «crear evento desde correo»: el Hub compone el deep-link al
  // compositor de Sytue Calendar prerellenado (asunto → título, cuerpo → notas,
  // participantes → invitados). Calendar lee la URL, sanea y abre el modal.
  async function handleCreateEvent(): Promise<void> {
    if (!latest) return;
    setCreatingEvent(true);
    try {
      // Invitados = participantes del hilo (remitentes + destinatarios), deduplicados.
      const attendees = Array.from(
        new Set(
          [...latest.from, ...latest.to].map((a) => a.email.trim().toLowerCase()).filter(Boolean),
        ),
      ).join(',');
      const result = await resolveIntent({
        type: 'create-event-from-mail',
        params: {
          message_id: latest.id,
          subject: latest.subject ?? '',
          body: latest.preview ?? '',
          ...(attendees ? { attendees } : {}),
        },
      });
      if (result.url) {
        window.open(result.url, '_blank', 'noopener');
      } else {
        toast.notify({ tone: 'success', title: t('reader.eventCreated') });
      }
    } catch {
      toast.notify({ tone: 'danger', title: t('reader.eventFailed') });
    } finally {
      setCreatingEvent(false);
    }
  }

  // Mensajes en orden de lectura (antiguo → reciente). La lista viene reciente→antiguo.
  const ordered = [...shown].reverse();
  const count = ordered.length;

  // Etiquetas aplicadas al hilo (unión de las de sus mensajes) y su conjunto de
  // ids, para pintar los chips y marcar el estado en el menú «Etiquetar».
  const appliedLabels = conversationLabels(conversation, labels);
  const appliedIds = new Set(appliedLabels.map((l) => l.id));

  return (
    <div class="flex h-full flex-col">
      {/* `bg-surface`: el mismo fondo que la cabecera de la carpeta (SectionHeader de
          MessageList). Sin ella heredaba el papel del fondo de página, y la cabecera
          del asunto se leía como una franja distinta de la de arriba, aunque las dos
          son la cabecera de la misma vista. */}
      <header class="bg-surface border-border shrink-0 border-b px-6 py-5">
        <div class="flex items-baseline gap-2">
          {/* h2: el h1 de la pagina es la carpeta (SectionHeader de MessageList). */}
          <h2 class="sytue-h3 text-text min-w-0 flex-1 [overflow-wrap:anywhere]">
            {latest.subject || t('common.noSubject')}
          </h2>
          {count > 1 ? (
            <span class="text-text-subtle shrink-0 text-sm tabular-nums">
              {t('reader.messageCount', { count })}
            </span>
          ) : null}
        </div>
        {/* En <768px solo «Responder» conserva el rótulo; el resto queda en icono
            con su nombre accesible (`max-md:sr-only`). Con todos los rótulos, la
            barra ocupaba cuatro filas en un iPhone antes de ver el mensaje. */}
        <div class="mt-4 flex flex-wrap gap-2">
          <Button
            size="sm"
            variant="primary"
            onClick={() => onReply(latest, 'reply', latestBody?.text)}
            title={`${t('reader.reply')} (R)`}
            leadingIcon={<Glyph name="reply" />}
          >
            {t('reader.reply')}
          </Button>
          <Button
            size="sm"
            variant="secondary"
            onClick={() => onReply(latest, 'replyAll', latestBody?.text)}
            title={`${t('reader.replyAll')} (A)`}
            leadingIcon={<Glyph name="replyAll" />}
          >
            <span class="max-md:sr-only">{t('reader.replyAll')}</span>
          </Button>
          <Button
            size="sm"
            variant="secondary"
            onClick={() => onReply(latest, 'forward', latestBody?.text)}
            title={`${t('reader.forward')} (F)`}
            leadingIcon={<Glyph name="forward" />}
          >
            <span class="max-md:sr-only">{t('reader.forward')}</span>
          </Button>
          {!readOnly ? (
            <Button
              size="sm"
              variant="ghost"
              loading={creatingEvent}
              onClick={() => void handleCreateEvent()}
              leadingIcon={<Icon name="plus" />}
            >
              {/* Con rótulo también en el móvil: un «+» a secas junto a responder y
                  reenviar no dice qué añade. */}
              <span>{t('reader.createEvent')}</span>
            </Button>
          ) : null}
          {!readOnly && unsub && (unsub.oneClick || unsub.mailto) ? (
            <Button
              size="sm"
              variant="ghost"
              loading={unsubBusy}
              onClick={() => void handleUnsubscribe()}
              leadingIcon={<Glyph name="unsubscribe" />}
            >
              <span class="max-md:sr-only">{t('reader.unsubscribe')}</span>
            </Button>
          ) : null}

          {/* Triaje del hilo abierto, alineado a la derecha en pantallas anchas. */}
          {!readOnly ? (
            <span
              class="ms-auto flex flex-wrap items-center gap-1"
              role="group"
              aria-label={t('reader.triageGroup')}
            >
              <Button
                size="sm"
                variant="ghost"
                iconOnly
                class={conversation.flagged ? 'text-warning-text' : undefined}
                aria-pressed={conversation.flagged}
                aria-label={conversation.flagged ? t('action.unstar') : t('action.star')}
                // El atajo va en el tooltip, no en el nombre accesible: un lector
                // de pantalla leería «(S)» en cada botón sin que sirva de nada.
                title={`${conversation.flagged ? t('action.unstar') : t('action.star')} (S)`}
                leadingIcon={<Glyph name="star" filled={conversation.flagged} />}
                onClick={() => onToggleFlag(conversation)}
              />
              {onArchive ? (
                <Button
                  size="sm"
                  variant="ghost"
                  iconOnly
                  aria-label={t('action.archive')}
                  title={`${t('action.archive')} (E)`}
                  leadingIcon={<Glyph name="archive" />}
                  onClick={() => onArchive(conversation)}
                />
              ) : null}
              {onTrash ? (
                <Button
                  size="sm"
                  variant="ghost"
                  iconOnly
                  aria-label={t('action.trash')}
                  title={`${t('action.trash')} (#)`}
                  leadingIcon={<Glyph name="trash" />}
                  onClick={() => onTrash(conversation)}
                />
              ) : null}
              {onNotSpam ? (
                <Button
                  size="sm"
                  variant="ghost"
                  leadingIcon={<Glyph name="spam" />}
                  onClick={() => onNotSpam(conversation)}
                >
                  {t('action.notSpam')}
                </Button>
              ) : null}
              {/* MENÚ DE DESBORDAMIENTO. Esta cabecera llegó a tener once
                  controles en una sola fila con `flex-wrap`: con el panel del
                  lector entre 600 y 700 px —dos columnas en un portátil— se
                  partía en TRES filas y empujaba el mensaje fuera de la pantalla,
                  que es justo lo que se ha abierto a leer.
                  Se quedan fuera las tres menos frecuentes: marcar como no leído,
                  marcar como spam y borrar definitivamente. Destacar, archivar y
                  papelera siguen a un clic porque son el triaje de todos los días,
                  y las dos primeras tienen además atajo de teclado (E y #). */}
              <Dropdown
                label={t('reader.moreActions')}
                align="end"
                trigger={(props) => (
                  <Button
                    size="sm"
                    variant="ghost"
                    iconOnly
                    aria-label={t('reader.moreActions')}
                    title={t('reader.moreActions')}
                    leadingIcon={<Icon name="ellipsis" size={16} />}
                    {...props}
                  />
                )}
              >
                <MenuItem icon={<Glyph name="unread" />} onClick={() => onMarkUnread(conversation)}>
                  {t('action.markUnread')}
                </MenuItem>
                {onCreateRule ? (
                  <MenuItem
                    icon={<Glyph name="folder" />}
                    onClick={() => onCreateRule(conversation)}
                  >
                    {t('reader.createRule')}
                  </MenuItem>
                ) : null}
                {onSpam ? (
                  <MenuItem icon={<Glyph name="spam" />} onClick={() => onSpam(conversation)}>
                    {t('action.spam')}
                  </MenuItem>
                ) : null}
                {/* En la papelera, «mover a la papelera» ya no dice nada: lo que
                    hace falta es borrarlo de verdad, que además es lo único que
                    devuelve espacio de la cuota. Confirma el padre. */}
                {onPurge ? (
                  <MenuItem
                    danger
                    icon={<Glyph name="trash" />}
                    onClick={() => onPurge(conversation)}
                  >
                    {t('action.purge')}
                  </MenuItem>
                ) : null}
              </Dropdown>
              <Dropdown
                label={t('reader.labelConversation')}
                align="end"
                trigger={(p) => (
                  <Button
                    size="sm"
                    variant="ghost"
                    leadingIcon={<Glyph name="tag" />}
                    trailingIcon={<Icon name="chevronDown" size={16} />}
                    {...p}
                  >
                    <span class="max-md:sr-only">{t('action.label')}</span>
                  </Button>
                )}
              >
                <div class="max-h-72 min-w-[220px] overflow-y-auto">
                  {labels.length === 0 ? (
                    <p class="text-text-subtle px-3 py-2 text-sm">{t('reader.noLabelsYet')}</p>
                  ) : (
                    labels.map((l) => {
                      const on = appliedIds.has(l.id);
                      return (
                        <MenuItem
                          key={l.id}
                          aria-pressed={on}
                          onClick={() => onToggleLabel(conversation, l.id, !on)}
                        >
                          <span class="flex flex-1 items-center gap-2">
                            <span
                              class="h-2.5 w-2.5 shrink-0 rounded-full"
                              style={{ background: labelDot(l.color) }}
                              aria-hidden="true"
                            />
                            <span class="flex-1 truncate">{l.name}</span>
                            {on ? (
                              <>
                                <span class="sr-only">{t('reader.labelApplied')}</span>
                                <Icon name="check" size={16} class="text-primary-text" />
                              </>
                            ) : null}
                          </span>
                        </MenuItem>
                      );
                    })
                  )}
                </div>
              </Dropdown>
              {moveTargets.length > 0 ? (
                <Dropdown
                  label={t('reader.moveToFolder')}
                  align="end"
                  trigger={(p) => (
                    <Button
                      size="sm"
                      variant="ghost"
                      leadingIcon={<Glyph name="folder" />}
                      trailingIcon={<Icon name="chevronDown" size={16} />}
                      {...p}
                    >
                      <span class="max-md:sr-only">{t('action.moveTo')}</span>
                    </Button>
                  )}
                >
                  {/* Acotado: puede haber muchas carpetas; el menú hace scroll. */}
                  <div class="max-h-72 overflow-y-auto">
                    {moveTargets.map((m) => (
                      <MenuItem
                        key={m.id}
                        icon={<Glyph name="folder" />}
                        onClick={() => onMove(conversation, m)}
                      >
                        {mailboxLabel(t, m.name, m.role)}
                      </MenuItem>
                    ))}
                  </div>
                </Dropdown>
              ) : null}
            </span>
          ) : null}
        </div>

        {appliedLabels.length > 0 ? (
          <div
            class="mt-3 flex flex-wrap items-center gap-1.5"
            role="group"
            aria-label={t('reader.conversationLabels')}
          >
            {appliedLabels.map((l) => (
              <span
                key={l.id}
                class="border-border bg-bg-subtle text-text-muted rounded-pill inline-flex items-center gap-1.5 border py-0.5 ps-2 pe-1 text-xs"
              >
                <span
                  class="h-2 w-2 shrink-0 rounded-full"
                  style={{ background: labelDot(l.color) }}
                  aria-hidden="true"
                />
                <span class="max-w-[12rem] truncate">{l.name}</span>
                {!readOnly ? (
                  <button
                    type="button"
                    // 24 px de diana (WCAG 2.5.8), como el resto de la suite: este
                    // botón copió clases en vez de usar el componente y se quedó en
                    // 16. El `-my-1` devuelve la altura exacta del chip, así que la
                    // fila de etiquetas no cambia de densidad.
                    class="hover:text-danger-text sytue-focus-ring rounded-control -my-1 grid h-6 w-6 shrink-0 place-items-center transition-colors duration-150"
                    aria-label={t('reader.removeLabel', { label: l.name })}
                    title={t('reader.removeLabel', { label: l.name })}
                    onClick={() => onToggleLabel(conversation, l.id, false)}
                  >
                    <Icon name="close" size={12} />
                  </button>
                ) : null}
              </span>
            ))}
          </div>
        ) : null}
      </header>

      {/* El hilo crece CON el panel, no con un tope fijo. Antes la columna se
          quedaba en 48rem (768px) centrada: en una pantalla de 27" el correo
          ocupaba un tercio del lector y el resto era margen. Ahora el ancho es
          el 92% del panel (unidad de contenedor, no de ventana: la lista y el
          menú lateral ya se han llevado lo suyo), con un suelo de 44rem para que
          en portátiles no se estreche y un techo de 88rem para que en ultra
          panorámicas la línea de texto siga siendo legible. `min(100%, …)`
          evita el desbordamiento cuando el panel es más estrecho que el suelo. */}
      <div class="bg-bg @container min-h-0 flex-1 overflow-y-auto px-6 py-6 xl:px-8 2xl:px-10">
        <div class="mx-auto flex w-full max-w-[min(100%,clamp(44rem,92cqw,88rem))] flex-col gap-3">
          {ordered.map((msg) => (
            <ThreadMessage
              key={msg.id}
              t={t}
              locale={locale}
              email={msg}
              defaultExpanded={msg.id === latest.id}
              prefetchedBody={msg.id === latest.id ? latestBody : null}
              onReply={onReply}
            />
          ))}
        </div>
      </div>
    </div>
  );
}

/**
 * Un mensaje dentro del hilo: cabecera plegable (remitente · fecha · extracto) y,
 * al expandir, el cuerpo completo (cargado bajo demanda), sus adjuntos y acciones
 * compactas (Responder/Reenviar a ESE mensaje).
 */
function ThreadMessage({
  t,
  locale,
  email,
  defaultExpanded,
  prefetchedBody,
  onReply,
}: {
  /** `t` y `locale` bajan por props desde el lector: nunca se detecta aquí. */
  t: T;
  locale: Locale;
  email: EmailSummary;
  defaultExpanded: boolean;
  prefetchedBody: MessageBody | null;
  onReply: (email: EmailSummary, mode: ReplyMode, bodyText?: string) => void;
}): JSX.Element {
  const [expanded, setExpanded] = useState(defaultExpanded);
  const [body, setBody] = useState<MessageBody | null>(prefetchedBody);
  const [attachments, setAttachments] = useState<AttachmentMeta[]>([]);
  // Índice del adjunto que se está guardando en Drive (null = ninguno). El trasvase
  // lo hace el servidor y puede tardar con un adjunto grande: sin esto, la persona
  // vuelve a pulsar y lo guarda dos veces.
  const [savingTo, setSavingTo] = useState<number | null>(null);
  const toast = useToast();
  const { resolved } = useTheme();

  async function guardarEnDrive(index: number, name: string): Promise<void> {
    if (savingTo !== null) return;
    setSavingTo(index);
    try {
      const res = await fetch('/api/attachment-to-drive', {
        method: 'POST',
        credentials: 'include',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ emailId: email.id, index }),
      });
      if (!res.ok) throw new Error(String(res.status));
      const saved = (await res.json()) as { name: string };
      toast.notify({
        tone: 'success',
        title: t('reader.savedToDrive'),
        // El nombre GUARDADO, que no siempre es el del adjunto: si ya había otro
        // igual se guarda como «informe (2).pdf» y hay que decirlo, o la persona
        // lo busca por el nombre que vio en el correo y no lo encuentra.
        description: t('reader.savedToDriveIn', { name: saved.name }),
      });
    } catch {
      toast.notify({
        tone: 'danger',
        title: t('reader.saveToDriveFailed'),
        description: t('reader.saveToDriveFailedDescription', { name }),
      });
    } finally {
      setSavingTo(null);
    }
  }

  // Cuerpo completo bajo demanda al expandir (una vez). El extracto se usa mientras
  // carga o si falla: el mensaje nunca queda en blanco.
  useEffect(() => {
    if (prefetchedBody) setBody(prefetchedBody);
  }, [prefetchedBody]);
  useEffect(() => {
    if (!expanded || body) return;
    let vigente = true;
    getMessageBody(email.id)
      .then((b) => vigente && setBody(b))
      .catch(() => {
        /* degrada al extracto */
      });
    return () => {
      vigente = false;
    };
  }, [expanded, body, email.id]);

  // Metadatos de adjuntos al expandir un mensaje que los tenga.
  useEffect(() => {
    if (!expanded || !email.hasAttachment) return;
    let vigente = true;
    listAttachments(email.id)
      .then((res) => vigente && setAttachments(res.attachments))
      .catch(() => {
        /* el bloque de adjuntos degrada a vacío */
      });
    return () => {
      vigente = false;
    };
  }, [expanded, email.hasAttachment, email.id]);

  const unread = !email.keywords.$seen;

  // La firma criptográfica es parte del SOBRE, no un fichero que nadie decidiera
  // mandar: ofrecer «signature.asc · 0,8 kB ⬇» invita a bajar algo que no se abre
  // con nada. Se conserva el índice ORIGINAL porque la descarga va por posición
  // dentro del mensaje, y renumerar aquí bajaría el adjunto equivocado.
  const visibles = attachments.map((a, i) => [a, i] as const).filter(([a]) => !esFirmaDeSobre(a));
  // Firmado y sin nada más: no hay bloque de adjuntos que enseñar. Sin esto se
  // caería al aviso «este mensaje trae adjuntos», que sería mentira.
  const soloFirmas = attachments.length > 0 && visibles.length === 0;

  return (
    <Card
      raised={false}
      class={cx('overflow-hidden p-0', resolved === 'light' && 'sytue-correo-tarjeta-clara')}
    >
      <button
        type="button"
        class="hover:bg-bg-subtle sytue-focus-ring flex w-full items-start gap-3 px-5 py-4 text-left transition-colors duration-150"
        aria-expanded={expanded}
        onClick={() => setExpanded((v) => !v)}
      >
        <Avatar name={displayName(t, email.from)} size={expanded ? 'lg' : 'md'} />
        <span class="min-w-0 flex-1">
          <span class="flex items-baseline gap-3">
            {/* 400 y no 500 para el leído: contra el 600 del no leído, un escalón de
                100 no se distingue, y dentro de un hilo largo saber qué mensaje sigue
                sin abrir es justo lo que se busca. Mismo salto que en la lista. */}
            <span
              class={`min-w-0 flex-1 truncate ${unread ? 'text-primary-text font-bold' : 'text-text font-normal'}`}
              title={displayName(t, email.from)}
            >
              {displayName(t, email.from)}
            </span>
            <time
              class={`shrink-0 text-sm whitespace-nowrap tabular-nums ${unread ? 'text-primary-text font-bold' : 'text-text-subtle font-normal'}`}
              dateTime={email.receivedAt}
              title={fullDate(locale, email.receivedAt)}
            >
              {expanded ? (
                <>
                  {/* La fecha larga solo cabe en escritorio: en el móvil aplastaba el
                      nombre del remitente hasta dejarlo en «D…». */}
                  <span class="max-md:hidden">{fullDate(locale, email.receivedAt)}</span>
                  <span class="md:hidden">{shortDate(locale, email.receivedAt)}</span>
                </>
              ) : (
                shortDate(locale, email.receivedAt)
              )}
            </time>
          </span>
          {expanded ? (
            <>
              <span class="text-text-muted block truncate text-sm" title={email.from[0]?.email}>
                {email.from[0]?.email}
              </span>
              {email.to.length ? (
                <span
                  class="text-text-subtle block truncate text-sm"
                  title={t('reader.to', { addresses: joinAddresses(email.to) })}
                >
                  {t('reader.to', { addresses: joinAddresses(email.to) })}
                </span>
              ) : null}
            </>
          ) : (
            <span class="text-text-subtle block truncate text-sm">
              {email.preview || t('list.noPreview')}
            </span>
          )}
        </span>
        <Icon
          name={expanded ? 'chevronUp' : 'chevronDown'}
          size={18}
          class="text-text-subtle mt-1 shrink-0"
        />
      </button>

      {expanded ? (
        <div class="px-5 pt-1 pb-5">
          <MessageBodyView
            t={t}
            body={body}
            preview={email.preview}
            emailId={email.id}
            adjuntos={attachments}
          />
          {email.hasAttachment && !soloFirmas ? (
            <div class="border-border mt-6 border-t pt-4">
              <p class="sytue-overline text-text-subtle mb-2">
                {visibles.length
                  ? t('reader.attachmentCount', { count: visibles.length })
                  : t('reader.attachments')}
              </p>
              {visibles.length ? (
                <ul class="flex flex-col gap-2">
                  {visibles.map(([a, i]) => (
                    <li key={i} class="flex min-w-0 items-center gap-2">
                      <a
                        href={attachmentDownloadUrl(email.id, i)}
                        download={a.name}
                        title={t('reader.download', { name: a.name })}
                        class="border-border bg-bg-subtle text-text hover:border-border-strong hover:bg-bg sytue-focus-ring inline-flex max-w-full min-w-0 items-center gap-3 rounded-md border px-3 py-2 text-sm transition-colors duration-150"
                      >
                        <span class="truncate">{a.name}</span>
                        <span class="text-text-subtle shrink-0 tabular-nums">
                          {formatBytes(locale, a.size)}
                        </span>
                        <Glyph name="download" size={16} class="text-text-subtle shrink-0" />
                      </a>
                      {/* Archivar un adjunto obligaba a bajarlo y volver a subirlo a
                          mano: el correo y el disco están en la misma suite. */}
                      <Button
                        variant="ghost"
                        size="sm"
                        disabled={savingTo === i}
                        onClick={() => void guardarEnDrive(i, a.name)}
                        title={t('reader.saveToDrive')}
                      >
                        {t('reader.saveToDrive')}
                      </Button>
                    </li>
                  ))}
                </ul>
              ) : (
                <span class="border-border bg-bg-subtle text-text-muted inline-flex items-center gap-2 rounded-md border px-3 py-2 text-sm">
                  <Icon name="info" size={16} class="shrink-0" />
                  {t('reader.hasAttachments')}
                </span>
              )}
            </div>
          ) : null}
          <div class="mt-4 flex flex-wrap gap-2">
            <Button
              size="sm"
              variant="ghost"
              leadingIcon={<Glyph name="reply" />}
              onClick={() => onReply(email, 'reply', body?.text)}
            >
              {t('reader.reply')}
            </Button>
            <Button
              size="sm"
              variant="ghost"
              leadingIcon={<Glyph name="forward" />}
              onClick={() => onReply(email, 'forward', body?.text)}
            >
              {t('reader.forward')}
            </Button>
            {/* El mensaje ENTERO tal como llegó (.eml): cabeceras, firmas y
                adjuntos incluidos. Sirve para guardarlo como prueba, abrirlo en
                otro cliente o mandárselo a quien lo pida. Es un enlace y no un
                botón porque la descarga la lleva el navegador: sin estado que
                mantener aquí, y funciona con «guardar enlace como». */}
            <a
              href={rawDownloadUrl(email.id, email.subject)}
              download
              class="text-text-muted hover:bg-bg-subtle hover:text-text sytue-focus-ring rounded-control inline-flex items-center gap-2 px-3 py-1.5 text-sm transition-colors duration-150"
              title={t('reader.downloadOriginalHint')}
            >
              <Glyph name="download" size={16} />
              {t('reader.downloadOriginal')}
            </a>
          </div>
        </div>
      ) : null}
    </Card>
  );
}

/**
 * Cuerpo del correo. Prioridad: HTML completo (aislado en un iframe sandbox, sin
 * scripts) → texto completo → extracto del sobre mientras carga o si falla.
 *
 * El HTML va PRIMERO porque casi todo el correo que llega de fuera es
 * multipart/alternative: trae las dos partes, y la de texto es el respaldo que
 * su autor no escribió ni miró. Al preferirla, una factura, un boletín o
 * cualquier aviso con tablas y botones se veía como un muro de texto con las
 * URL crudas al final, mientras Apple Mail, Gmail y Outlook enseñaban la
 * maqueta. El aislamiento y el bloqueo de contenido remoto no cambian: los pone
 * HtmlBody.
 */
function MessageBodyView({
  t,
  body,
  preview,
  emailId,
  adjuntos,
}: {
  t: T;
  body: MessageBody | null;
  preview: string;
  emailId: string;
  /** Partes del mensaje: de ahí salen las imágenes en línea que el cuerpo cita. */
  adjuntos: readonly AttachmentMeta[];
}): JSX.Element {
  // `trim()`: una parte HTML que solo trae saltos de línea no es un cuerpo, y
  // pintaría un marco vacío tapando el texto que sí llegó.
  if (body?.html?.trim()) {
    // LAS IMÁGENES EN LÍNEA SE RESUELVEN AQUÍ. Viajan como partes MIME aparte y
    // el cuerpo las cita con `<img src="cid:…">`; el navegador no sabe pedir un
    // `cid:`, así que se cambia por la URL de la parte. Sin esto la imagen sale
    // rota y el correo parece venir vacío.
    const html = resolverCidsEnLinea(body.html, adjuntos, (i) =>
      attachmentDownloadUrl(emailId, i, true),
    );
    return <HtmlBody t={t} html={html} />;
  }
  if (body?.text) {
    return (
      <>
        <p class="text-text text-base leading-relaxed [overflow-wrap:anywhere] whitespace-pre-wrap">
          {body.text}
        </p>
        <ImagenesDelCuerpo t={t} emailId={emailId} adjuntos={adjuntos} />
      </>
    );
  }
  if (preview) {
    return (
      <div>
        <p class="text-text text-base leading-relaxed [overflow-wrap:anywhere] whitespace-pre-wrap">
          {preview}
        </p>
        {body === null ? (
          // Aún cargando el cuerpo completo: el extracto puede estar truncado.
          <div class="mt-3" aria-hidden="true">
            <Skeleton width="60%" height="0.9rem" />
          </div>
        ) : null}
      </div>
    );
  }
  return (
    <p class="text-text-subtle text-base leading-relaxed italic">
      {body === null ? t('reader.loadingBody') : t('reader.noTextContent')}
    </p>
  );
}

/**
 * Las imágenes que venían PARA VERSE, cuando no hay HTML que las cite.
 *
 * El iPhone manda las fotos en un `multipart/mixed` junto al texto, marcadas
 * `Content-Disposition: inline`, sin parte HTML y sin `Content-ID` (medido sobre
 * un mensaje real de iPhone Mail 23G83). No hay nada que sustituir: o se pintan
 * aquí, o el correo se ve como un saludo de una línea con tres ficheros al pie,
 * que es como se veía.
 *
 * Van DEBAJO del texto y no intercaladas: `parseMessageBody` devuelve el primer
 * cuerpo de texto, no la secuencia de partes, así que no hay dónde intercalar.
 * Para este mensaje sale igual —las imágenes van después del texto— y para el
 * resto el orden entre ellas se respeta.
 *
 * No se sirven desde el dominio de quien envía: son partes del propio mensaje,
 * ya descargadas, y las pide nuestro BFF. No hay nada que bloquear ni que
 * consentir —el rastreo por imagen remota es otra cosa, y sigue bloqueado en
 * `HtmlBody`—.
 */
function ImagenesDelCuerpo({
  t,
  emailId,
  adjuntos,
}: {
  t: T;
  emailId: string;
  adjuntos: readonly AttachmentMeta[];
}): JSX.Element | null {
  const imagenes = imagenesEnLinea(adjuntos);
  if (imagenes.length === 0) return null;
  return (
    <div class="mt-3 flex flex-col gap-3">
      {imagenes.map(([a, i]) => (
        <img
          key={`${emailId}:${i}`}
          src={attachmentDownloadUrl(emailId, i, true)}
          alt={a.name || t('reader.attachments')}
          loading="lazy"
          class="border-border-subtle h-auto max-w-full rounded-md border"
        />
      ))}
    </div>
  );
}

/**
 * HTML del correo en un iframe sandbox: sin scripts, formularios ni popups
 * (atributo sandbox sin allow-scripts). allow-same-origin SOLO para poder medir
 * la altura desde fuera — sin scripts dentro, el contenido no puede tocar al
 * padre. Los enlaces salen con target=_blank via <base>.
 *
 * El contenido REMOTO (imágenes servidas desde el dominio de quien envía) se
 * bloquea por defecto con una CSP dentro del propio documento. No es cosmético:
 * una imagen de 1×1 descargada al abrir el mensaje le confirma al remitente la
 * hora de lectura y la IP desde la que se lee, y es la técnica estándar de
 * rastreo del correo comercial. Se muestra solo si la persona lo pide.
 */

/**
 * ¿El correo trae maqueta propia?
 *
 * Pregunta por COLORES y no por complejidad: quien declara un color de letra o
 * un fondo lo eligió contra un lienzo concreto —casi siempre blanco—. De noche
 * se invierte esa maqueta para que siga siendo legible sobre grafito. El HTML
 * desnudo usa la tinta del tema; en claro ambos dejan ver la superficie neutra
 * de la tarjeta, salvo los fondos que el propio mensaje haya declarado.
 *
 * ANTE LA DUDA, MAQUETADO. Equivocarse de menos pinta la letra negra del
 * remitente sobre grafito, que no se lee.
 */
/**
 * ¿Es este «adjunto» la firma criptográfica del mensaje?
 *
 * PGP y S/MIME firman adjuntando una parte más (`application/pgp-signature`,
 * `application/pkcs7-signature`). Es un detalle del transporte: quien escribe no
 * eligió mandarla y no puede hacer nada con ella. Los clientes serios la usan
 * para pintar «firmado» y no la listan.
 *
 * Se mira el TIPO primero y el nombre solo como red: el tipo puede venir con
 * parámetros (`; name=...`) y hay pasarelas que lo reescriben a
 * `application/octet-stream`, en cuyo caso el nombre canónico es lo único que
 * queda. Al revés no: un fichero que alguien llamó `signature.asc` a mano y
 * mandó con su tipo real se sigue viendo.
 */
export function esFirmaDeSobre(a: AttachmentMeta): boolean {
  const tipo = a.type.split(';')[0]!.trim().toLowerCase();
  return (
    /^application\/(pgp-signature|(x-)?pkcs7-signature)$/.test(tipo) ||
    /^(signature\.asc|smime\.p7s)$/i.test(a.name)
  );
}

export const MAQUETA_PROPIA = /<(?:table|body)\b|bgcolor\s*=|(?:background|color)\s*:/i;

/**
 * Un token del tema, ya resuelto.
 *
 * `srcdoc` no hereda las custom properties de la página —la variable se
 * quedaría sin resolver y el texto caería al negro del navegador—, así que hay
 * que leerlas aquí e inyectar el literal. Se lee en cada pintado: en Mail el
 * tema lo fija el script del `<head>` antes de hidratar y no hay conmutador en
 * caliente, así que con eso basta.
 */
function tokenResuelto(nombre: string, respaldo: string): string {
  if (typeof document === 'undefined') return respaldo;
  return getComputedStyle(document.documentElement).getPropertyValue(nombre).trim() || respaldo;
}

/** La tinta del token en tema CLARO, para el correo que viene maquetado para blanco. */
const TINTA_SOBRE_BLANCO = '#1a3541';

/**
 * Modo noche para el correo maquetado sobre blanco: se invierte el documento
 * entero y se vuelven a invertir las imágenes, que así conservan sus colores.
 * Es lo que hacen Apple Mail y Proton con el correo que no trae tema oscuro
 * propio, y lo que se esperaba aquí: el lienzo blanco de siempre era, de noche,
 * un rectángulo deslumbrante en mitad del grafito (informado el 2026-09-09).
 *
 * `hue-rotate(180deg)` deshace el giro de tono que la inversión aplica a los
 * colores: un enlace azul sigue azul, un aviso rojo sigue rojo; solo cambia la
 * luminosidad. El documento permanece transparente: el filtro transforma la
 * tinta y los fondos internos del remitente, sin añadir otro lienzo sobre la
 * tarjeta. Así también se integra cuando su superficie es translúcida.
 *
 * ponytail: también invierte un correo que YA venía oscuro (raro fuera del
 * marketing); si molesta, medir la luminancia del fondo declarado y saltárselo.
 */
function inversionNocturna(): string {
  return (
    `body{filter:invert(1) hue-rotate(180deg)}` +
    `a,img,video,picture,svg,[style*="background-image"],[background]{filter:invert(1) hue-rotate(180deg)}`
  );
}

function HtmlBody({ t, html }: { t: T; html: string }): JSX.Element {
  const frame = useRef<HTMLIFrameElement>(null);
  const [height, setHeight] = useState(240);
  const [remoteAllowed, setRemoteAllowed] = useState(false);
  const remote = hasRemoteContent(html);

  // Cambiar de mensaje NO arrastra el permiso del anterior: cada correo se abre
  // bloqueado (el componente se reutiliza entre mensajes del hilo).
  useEffect(() => setRemoteAllowed(false), [html]);

  // `img-src data: 'self'` deja pasar las imágenes incrustadas en el propio
  // mensaje (`/api/attachments`) y, cuando la persona pulsa «Mostrar imágenes»,
  // las remotas reescritas hacia nuestro proxy (`/api/remote-image`), sin abrir
  // nunca `https:` en la CSP ni exponer la IP del navegador al remitente.
  const csp = "default-src 'none'; img-src data: 'self'; style-src 'unsafe-inline'; font-src data:";
  const htmlSeguro = neutralizarProxyEnCuerpo(html);
  const htmlCuerpo = remoteAllowed
    ? reescribirImagenesRemotas(htmlSeguro, remoteImageProxyUrl)
    : htmlSeguro;
  // Tipografía base del sistema dentro del documento aislado + enlaces en
  // pestaña nueva (el iframe no debe navegar dentro del lector).
  //
  // La tinta va en LITERAL y no como `var(--sytue-text)`: ver `tokenResuelto`.
  // Cuál de las dos, lo decide `MAQUETA_PROPIA`.
  const maquetado = MAQUETA_PROPIA.test(html);
  // El tema se lee del proveedor y no del DOM: así el conmutador de apariencia
  // del menú de cuenta vuelve a pintar el correo abierto sin cambiar de mensaje.
  const { resolved } = useTheme();
  const invertido = maquetado && resolved === 'dark';
  const esquema = resolved === 'dark' ? 'dark' : 'light';
  const tinta = maquetado ? TINTA_SOBRE_BLANCO : tokenResuelto('--sytue-text', TINTA_SOBRE_BLANCO);
  const enlace = tokenResuelto('--sytue-primary-text', '#1460a1');
  const doc = `<!doctype html><html><head><meta charset="utf-8">
<meta http-equiv="Content-Security-Policy" content="${csp}">
<base target="_blank">
<style>html{color-scheme:${esquema}}html,body{background:transparent!important}body{margin:0;font:15px/1.6 system-ui,-apple-system,sans-serif;color:${tinta};overflow-wrap:anywhere}a{color:${enlace}}${invertido ? inversionNocturna() : ''}</style>
</head><body>${htmlCuerpo}</body></html>`;

  const measure = () => {
    const d = frame.current?.contentDocument;
    if (d?.body) setHeight(Math.min(Math.max(d.body.scrollHeight + 16, 80), 4000));
  };

  return (
    <div>
      {remote && !remoteAllowed ? (
        <div class="border-border bg-surface-sunken mb-3 flex flex-wrap items-center gap-3 rounded-md border px-3 py-2">
          <Icon name="shield" size={16} class="text-text-subtle shrink-0" aria-hidden="true" />
          <p class="text-text-subtle min-w-0 flex-1 text-sm">{t('reader.remoteContentBlocked')}</p>
          <Button variant="secondary" size="sm" onClick={() => setRemoteAllowed(true)}>
            {t('reader.remoteContentShow')}
          </Button>
        </div>
      ) : null}
      <iframe
        ref={frame}
        srcdoc={doc}
        sandbox="allow-same-origin allow-popups allow-popups-to-escape-sandbox"
        referrerpolicy="no-referrer"
        title={t('reader.frameTitle')}
        // El iframe deja ver la tarjeta. Los fondos declarados por el remitente
        // siguen dentro del documento y se conservan; un simple `color:` no
        // añade un rectángulo blanco alrededor de todo el mensaje.
        // La inversión nocturna cambia la tinta, manteniendo transparente el lienzo.
        class="sytue-correo-cuerpo w-full"
        // El documento y su marco comparten esquema. Si difieren, Chromium
        // pinta un lienzo blanco aunque ambos declaren background transparente.
        style={{ height: `${height}px`, colorScheme: esquema }}
        onLoad={measure}
      />
    </div>
  );
}
