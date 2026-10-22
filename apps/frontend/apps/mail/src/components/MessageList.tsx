/**
 * MessageList — lista de CONVERSACIONES tipo Gmail/Superhuman con el design
 * system Sytue (List/ListItem de @sytue/ui). Cabecera densa con la carpeta y su
 * conteo; fila de tres lineas (remitente · asunto · extracto) con punto de
 * no-leido, avatar del remitente, contador de mensajes del hilo, fecha y clip de
 * adjunto. Paginacion por scroll infinito (sentinel IntersectionObserver + boton
 * de respaldo accesible). Estados de carga (Skeleton con la geometria real de la
 * fila), error (con reintento) y vacio (EmptyState con accion de redactar).
 * docs/design.md §3.7 / §3.17.
 */
import type { JSX } from 'preact';
import { useEffect, useRef } from 'preact/hooks';
import {
  Avatar,
  Button,
  Input,
  List,
  ListItem,
  Skeleton,
  Spinner,
  EmptyState,
  Icon,
  SectionHeader,
  cx,
  Illustration,
  Checkbox,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { useT } from '../i18n/index.js';
import type { Label } from '../client/mailApi.js';
import type { Conversation } from './utils.js';
import { displayName, fullDate, shortDate, conversationLabels, labelDot } from './utils.js';
import { Glyph } from './glyphs.js';
import { BatchActionBar } from './BatchActionBar.js';

interface Props {
  /** Oculta cualquier acción que necesite confirmar estado en el servidor. */
  readOnly?: boolean;
  /** La franja de solo lectura del buzón compartido, bajo la cabecera. */
  franja?: JSX.Element | null;
  /** Idioma ya negociado en el servidor; llega por props desde la isla raíz. */
  locale: Locale;
  conversations: Conversation[];
  loading: boolean;
  /** Fallo al cargar la carpeta: estado de error con reintento. */
  error: boolean;
  selectedThreadId: string | null;
  mailboxName: string;
  /** «Vaciar carpeta» (papelera y spam): borra definitivamente todo lo que hay. */
  onEmptyFolder?: (() => void) | null | undefined;
  /** Texto de búsqueda (controlado por el padre). */
  query: string;
  /** Se están mostrando resultados de búsqueda, no la carpeta. */
  searchMode: boolean;
  /** Hay más páginas por cargar (scroll infinito). */
  hasMore: boolean;
  /** Se está cargando la página siguiente. */
  loadingMore: boolean;
  onLoadMore: () => void;
  onQueryChange: (value: string) => void;
  onSelect: (conv: Conversation) => void;
  /** Abre la redaccion (CTA del estado vacio). */
  onCompose: () => void;
  /** Reintenta la carga tras un error. */
  onRetry: () => void;
  /** Acciones de triaje de fila (aparecen al hover/foco); operan sobre el hilo. */
  onToggleFlag: (conv: Conversation) => void;
  onToggleSeen: (conv: Conversation) => void;
  /** null oculta la acción (no hay carpeta destino o ya estamos en ella). */
  onArchive: ((conv: Conversation) => void) | null;
  onTrash: ((conv: Conversation) => void) | null;
  /** «Marcar como spam» / «No es spam» (null oculta la acción según la carpeta). */
  onSpam: ((conv: Conversation) => void) | null;
  onNotSpam: ((conv: Conversation) => void) | null;
  /** Registro de etiquetas de la cuenta, para pintar los chips de cada fila. */
  labels: Label[];
  // ---- Multiselección + acciones en lote ----
  /** threadIds seleccionados (para checkbox por fila y barra de lote). */
  selected: Set<string>;
  /** Alterna la selección de un hilo; shift-click selecciona el rango desde el ancla. */
  onToggleSelect: (threadId: string, index: number, shift: boolean) => void;
  /** Alterna «seleccionar todo» (indeterminate cuando hay selección parcial). */
  onToggleSelectAll: () => void;
  /** Limpia la selección. */
  onClearSelection: () => void;
  /** Acciones en lote sobre la selección (null oculta la acción según la vista). */
  onBatchArchive: (() => void) | null;
  onBatchTrash: (() => void) | null;
  onBatchSpam: (() => void) | null;
  /** Borrado definitivo de la selección (solo en papelera/spam). */
  onBatchPurge: (() => void) | null;
  onBatchRead: () => void;
  onBatchUnread: () => void;
  onBatchLabel: (labelId: string) => void;
}

/**
 * Anclas estables para los atajos de teclado de la app (MailApp los busca por
 * id): el campo de búsqueda («/») y el listbox de conversaciones, cuyas filas
 * recorren j/k. Van aquí, junto a lo que las pinta, para que renombrar el
 * marcado obligue a mirar el atajo.
 */
export const SEARCH_INPUT_ID = 'mail-search';
export const CONVERSATION_LIST_ID = 'mail-conversations';

/**
 * Botón de acción de fila: icon-only, para el triaje al hover (no selecciona la fila).
 *
 * `size="xs"` (24 px, el suelo de WCAG 2.5.8 y la talla que Button documenta para el
 * cromo denso) y NO `size="sm"` con un `h-6 w-6` encima. Eso último es lo que había, y
 * no hacía nada: dos utilidades de la misma familia no se resuelven por el orden de la
 * cadena de clases sino por el de la hoja generada, donde `.h-8` va DESPUÉS de `.h-6`.
 * O sea que los botones medían 32 px dentro de una franja de 24 —y el `truncate` del
 * ListItem, que es `overflow:hidden`, les cortaba el borde inferior y el fondo del
 * hover, que salía como un rectángulo partido.
 */
function RowAction({
  label,
  pressed,
  onAct,
  children,
  class: cls,
}: {
  label: string;
  pressed?: boolean | undefined;
  onAct: () => void;
  children: JSX.Element;
  class?: string | undefined;
}): JSX.Element {
  return (
    <Button
      size="xs"
      variant="ghost"
      iconOnly
      class={cls}
      aria-label={label}
      title={label}
      aria-pressed={pressed}
      leadingIcon={children}
      onClick={(e: MouseEvent) => {
        e.stopPropagation(); // no seleccionar la fila al accionar
        onAct();
      }}
    />
  );
}

export function MessageList({
  readOnly = false,
  franja = null,
  locale,
  conversations,
  loading,
  error,
  selectedThreadId,
  mailboxName,
  onEmptyFolder,
  query,
  searchMode,
  hasMore,
  loadingMore,
  onLoadMore,
  onQueryChange,
  onSelect,
  onCompose,
  onRetry,
  onToggleFlag,
  onToggleSeen,
  onArchive,
  onTrash,
  onSpam,
  onNotSpam,
  labels,
  selected,
  onToggleSelect,
  onToggleSelectAll,
  onClearSelection,
  onBatchArchive,
  onBatchTrash,
  onBatchSpam,
  onBatchPurge,
  onBatchRead,
  onBatchUnread,
  onBatchLabel,
}: Props): JSX.Element {
  const t = useT(locale);
  const unreadCount = conversations.reduce((n, c) => (c.unread ? n + 1 : n), 0);
  const name = mailboxName || t('list.fallbackName');
  const heading = searchMode ? t('list.searchResults') : name;
  const count = conversations.length;

  // Selección: cuántas de las conversaciones VISIBLES están seleccionadas (ignora
  // ids ya no visibles, p. ej. tras mover). Alimenta el checkbox «seleccionar
  // todo» (indeterminate) y la barra de lote.
  const selectedCount = conversations.reduce((n, c) => (selected.has(c.threadId) ? n + 1 : n), 0);
  const allSelected = count > 0 && selectedCount === count;
  const someSelected = selectedCount > 0 && !allSelected;
  const selectionActive = !readOnly && selectedCount > 0;

  // Scroll infinito: observa un sentinel al final del scroller y pide más al
  // acercarse (rootMargin 200px). El botón de respaldo cubre teclado/sin-IO.
  const scrollRef = useRef<HTMLDivElement>(null);
  const sentinelRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const root = scrollRef.current;
    const target = sentinelRef.current;
    if (!root || !target || !hasMore || loading) return;
    const obs = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting)) onLoadMore();
      },
      { root, rootMargin: '200px' },
    );
    obs.observe(target);
    return () => obs.disconnect();
  }, [hasMore, loading, loadingMore, count, onLoadMore]);

  return (
    <section
      class="bg-surface flex h-full min-w-0 flex-col"
      aria-label={t('list.region', { folder: name })}
    >
      {/* Cabecera de lista (SectionHeader): título de la carpeta + conteo; debajo,
          la barra de búsqueda. */}
      <div class="shrink-0">
        <SectionHeader
          // Titula la vista entera (bandeja/etiqueta/busqueda): es el h1 de la
          // pagina. El asunto del lector es un h2 por debajo de el.
          level={1}
          title={heading}
          actions={
            !loading && count > 0 ? (
              <span class="flex items-center gap-2">
                {/* `sytue-meta`, que es el rol del dato accesorio (contadores), y NO
                    `text-header-fg`: ese token es el texto de la BANDA de cabecera, y el
                    SectionHeader se pinta sobre `bg-surface`. Cuando la banda era grafito,
                    «1 234 sin leer» salía en blanco sobre blanco —invisible en tema
                    claro—. Resto de la suite: `text-header-fg` solo aparece junto a
                    `bg-header`.

                    CON LLAVES. Escrito con `//` a pelo esto NO era un comentario: dentro
                    de los hijos de JSX es TEXTO, así que la cabecera de la lista de
                    correo pintaba el comentario entero encima de los mensajes. */}
                <span class="sytue-meta tabular-nums">
                  {searchMode
                    ? t('list.resultCount', { count })
                    : unreadCount > 0
                      ? t('list.unreadCount', { count: unreadCount })
                      : t('list.conversationCount', { count })}
                </span>
                {/* Solo donde borrar es definitivo (papelera, spam): el padre decide. */}
                {onEmptyFolder && !searchMode ? (
                  <Button size="sm" variant="ghost" onClick={onEmptyFolder}>
                    <span class="text-danger-text inline-flex items-center gap-1.5">
                      <Glyph name="trash" size={14} /> {t('purge.emptyFolder')}
                    </span>
                  </Button>
                ) : null}
              </span>
            ) : null
          }
        />
        {franja}
        <form
          role="search"
          class="border-border flex items-center gap-2 border-b px-4 py-2.5"
          onSubmit={(e: Event) => e.preventDefault()}
        >
          <Input
            // Id estable: es el destino del atajo «/» (buscar), que se dispara
            // desde cualquier parte de la app.
            id={SEARCH_INPUT_ID}
            type="search"
            class="flex-1"
            value={query}
            onInput={(e) => onQueryChange(e.currentTarget.value)}
            placeholder={t('list.searchPlaceholder')}
            aria-label={t('list.searchLabel')}
            title={t('list.searchHint')}
            leadingIcon={<Icon name="search" size={16} />}
          />
          {query ? (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              aria-label={t('list.clearSearch')}
              onClick={() => onQueryChange('')}
            >
              <Icon name="close" size={16} />
            </Button>
          ) : null}
        </form>
        {/* Barra de acciones en lote: aparece al seleccionar (no ocupa espacio en
            reposo). Trae «seleccionar todo» (indeterminate) + las acciones. */}
        {selectionActive ? (
          <div class="border-border bg-bg-subtle flex items-center gap-2 border-b px-4 py-1.5">
            <Checkbox
              indeterminate={someSelected}
              checked={allSelected}
              onChange={onToggleSelectAll}
              aria-label={t('list.selectAll')}
              class="border-border-strong accent-primary h-4 w-4 shrink-0 rounded-sm pointer-coarse:h-6 pointer-coarse:w-6"
            />
            <BatchActionBar
              locale={locale}
              count={selectedCount}
              onRead={onBatchRead}
              onUnread={onBatchUnread}
              onArchive={onBatchArchive}
              onTrash={onBatchTrash}
              onSpam={onBatchSpam}
              onPurge={onBatchPurge}
              onClear={onClearSelection}
              labels={labels}
              onLabel={onBatchLabel}
            />
          </div>
        ) : null}
      </div>

      {/* `relative`: el panel DESPLAZA, y el «no leído» de cada fila es un `sr-only`
          (`absolute`). Sin bloque contenedor aquí se anclaba fuera, donde le tocaría sin
          desplazar, y alargaba el documento bajo el armazón: 1113 px con el buzón de
          verdad (v3.11.1). El `main` del AppShell no lo cubre: esto va en su panel
          secundario. */}
      <div
        ref={scrollRef}
        class="relative min-h-0 flex-1 overflow-x-hidden overflow-y-auto"
        aria-busy={loading}
      >
        {loading ? (
          <>
            <p class="sr-only" role="status">
              {t('list.loading')}
            </p>
            {/* Skeleton con la MISMA geometria que la fila real, para que no
                salte el layout al cargar. La tercera linea reserva 24px porque
                ahi se relevan el extracto y los botones de triaje.

                La raíz de 16 px comparte geometría con la interfaz nativa.
                ListItem.tsx suma 83 px: py-2 × 2 = 16, título = 22,
                subtítulo = 18, gap-0.5 = 2, triaje = 24 y borde = 1.
                Reservar esa altura evita el salto al llegar los datos. */}
            <ul class="flex flex-col" aria-hidden="true">
              {Array.from({ length: 8 }).map((_, i) => (
                <li
                  key={i}
                  class="border-b-border flex h-[83px] items-center gap-3 border-b border-l-2 border-l-transparent px-3"
                >
                  <span class="flex items-center gap-1">
                    <span class="h-2 w-2 shrink-0" />
                    <Skeleton circle width="40px" height="40px" />
                  </span>
                  <div class="min-w-0 flex-1">
                    <Skeleton height="14px" width="45%" />
                    <Skeleton height="12px" width="80%" class="mt-2" />
                    <Skeleton height="12px" width="65%" class="mt-2" />
                  </div>
                  <Skeleton height="12px" width="36px" />
                </li>
              ))}
            </ul>
          </>
        ) : error ? (
          <EmptyState
            class="h-full"
            title={t('list.errorTitle')}
            description={t('list.errorDescription')}
            illustration={<Illustration name="errorRetry" />}
            action={
              <Button variant="secondary" size="sm" onClick={onRetry}>
                {t('common.retry')}
              </Button>
            }
          />
        ) : count === 0 ? (
          searchMode ? (
            <EmptyState
              class="h-full"
              title={t('list.emptySearchTitle', { query: query.trim() })}
              description={t('list.emptySearchDescription')}
              illustration={<Illustration name="emptyMail" />}
            />
          ) : (
            <EmptyState
              class="h-full"
              title={t('list.emptyTitle', { folder: name })}
              description={t('list.emptyDescription')}
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
          )
        ) : (
          <>
            <List id={CONVERSATION_LIST_ID} selectable label={t('list.region', { folder: name })}>
              {conversations.map((conv, index) => {
                const email = conv.latest;
                const unread = conv.unread;
                const sender = displayName(t, email.from);
                const subject = email.subject || t('common.noSubject');
                const convLabels = labels.length ? conversationLabels(conv, labels) : [];
                return (
                  <ListItem
                    key={conv.threadId}
                    index={index}
                    // Separador 1px entre filas: la vista Mail usa borde (§3.7/§5.1).
                    // `group` habilita las acciones de triaje al hover/foco.
                    class={cx(
                      'group border-b-border border-b',
                      // Una fila MARCADA con la casilla se pinta como elegida, igual
                      // que la conversación abierta. Antes lo único que la señalaba
                      // era la propia casilla, de 16 px: con la selección repartida
                      // por la lista no había forma de ver de un vistazo qué va a
                      // recibir la acción en lote. La abierta se sigue distinguiendo
                      // por su borde izquierdo primario, que esto no toca.
                      selected.has(conv.threadId) && 'bg-selected',
                    )}
                    selected={conv.threadId === selectedThreadId}
                    unread={unread}
                    onClick={() => onSelect(conv)}
                    onKeyDown={(e: KeyboardEvent) => {
                      // Teclas sobre los botones de acción de la fila: no seleccionar.
                      if (e.target !== e.currentTarget) return;
                      if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault();
                        onSelect(conv);
                      }
                    }}
                    leading={
                      // Columna de no-leido (punto naranja, 8px) reservada siempre
                      // para que leidos y no-leidos alineen el avatar; el estado no
                      // se comunica solo por color (texto en negrita + "No leído").
                      <span class="flex items-center gap-1">
                        {/* Checkbox de selección: visible al hover/foco o con la
                            selección activa (patrón Gmail; no ensucia en reposo). */}
                        {!readOnly ? (
                          <Checkbox
                            checked={selected.has(conv.threadId)}
                            aria-label={t('list.selectConversation', { subject })}
                            class={cx(
                              // 24px con el dedo (WCAG 2.5.8, como el resto de la
                              // suite): a 16px la casilla queda pegada a la fila
                              // que abre la conversación, y el fallo de puntería
                              // no marca — abre el correo.
                              'border-border-strong accent-primary h-4 w-4 shrink-0 rounded-sm transition-opacity duration-150 pointer-coarse:h-6 pointer-coarse:w-6',
                              selected.has(conv.threadId) || selectionActive
                                ? 'opacity-100'
                                : // Sin hover (móvil/táctil) la casilla quedaba invisible
                                  // y la selección múltiple era inalcanzable: no había
                                  // forma de marcar la PRIMERA conversación, que es la
                                  // que enciende `selectionActive`.
                                  'opacity-0 group-focus-within:opacity-100 group-hover:opacity-100 focus-visible:opacity-100 pointer-coarse:opacity-100',
                            )}
                            // SIN `preventDefault()`, y esa era justo la avería: la
                            // casilla nunca se veía marcada. El navegador marca el
                            // input ANTES de despachar el click y lo REVIERTE al
                            // terminar si alguien llama a `preventDefault()`; esa
                            // reversión llegaba después del re-render de Preact, así
                            // que el DOM se quedaba en `checked=false` para siempre.
                            // Medido en producción el 2026-09-13: la barra de lote
                            // decía «1 seleccionada» —el estado era correcto— y las
                            // dieciocho casillas seguían con `checked=false`.
                            //
                            // `stopPropagation` sí hace falta: sin él, marcar abre
                            // también la conversación. Y el click es el evento y no
                            // `change` porque `shiftKey` —el rango— solo viaja aquí.
                            onClick={(e: MouseEvent) => {
                              e.stopPropagation(); // no abrir la conversación
                              onToggleSelect(conv.threadId, index, e.shiftKey);
                            }}
                          />
                        ) : null}
                        <span
                          class={`h-2 w-2 shrink-0 rounded-full ${unread ? 'bg-primary-600' : 'bg-transparent'}`}
                          aria-hidden="true"
                        />
                        <Avatar name={sender} size="md" />
                      </span>
                    }
                    title={
                      <span class="flex w-full min-w-0 items-center gap-1">
                        {unread ? <span class="sr-only">{t('list.unreadPrefix')}</span> : null}
                        {/* El PESO es lo que separa leído de no leído de un vistazo, y
                            no lo hacía: ListItem pinta el título en 600 si es no leído
                            y en 500 si no, y un escalón de 100 a 14 px no se percibe —
                            la bandeja se leía como un bloque uniforme. Poniendo el 400
                            aquí (elemento propio, gana al heredado) el salto pasa a ser
                            400 → 600, que es el de cualquier cliente de correo. */}
                        <span
                          class={cx(
                            'truncate',
                            unread ? 'text-primary-text font-bold' : 'font-normal',
                          )}
                          title={sender}
                        >
                          {sender}
                        </span>
                        {/* Cuenta el hilo ENTERO, que es lo que abre el lector: el
                            listado trae `thread_count` agregado en su propia
                            consulta. Antes contaba solo lo plegado de ESTA carpeta y
                            la fila decía «2» para un hilo que se abría con cinco. */}
                        {conv.total > 1 ? (
                          <span
                            class="sytue-meta shrink-0 tabular-nums"
                            title={t('list.messagesInThread', { count: conv.total })}
                          >
                            {conv.total}
                          </span>
                        ) : null}
                        {conv.flagged ? (
                          // El mismo glifo que pinta el botón de destacar de esta fila
                          // (más abajo). Antes era el carácter «★» (U+2605) puesto a
                          // pelo: un dingbat de fuente, que hereda el peso de la
                          // tipografía en vez del grosor de trazo del sistema de
                          // iconos y que algunas plataformas presentan a color.
                          <span
                            class="text-warning-text shrink-0"
                            role="img"
                            aria-label={t('action.starred')}
                          >
                            <Glyph name="star" filled size={16} />
                          </span>
                        ) : null}
                      </span>
                    }
                    subtitle={
                      <span class="flex w-full min-w-0 flex-col gap-0.5">
                        <span
                          class={cx(
                            'truncate',
                            unread ? 'text-primary-text font-bold' : 'text-text-muted font-normal',
                          )}
                          title={subject}
                        >
                          {subject}
                        </span>
                        {/* Extracto y acciones de triaje se RELEVAN en el mismo
                            hueco. Las acciones colgaban de la fecha, se desbordaban
                            por la izquierda y aterrizaraban sobre el asunto: lo tapaban
                            y le robaban el clic. Aquí no tapan nada —el extracto se
                            aparta— y no reservan ancho, así que el asunto conserva
                            todo el suyo.
                            El `max-w-full overflow-x-auto` no es adorno: la barra
                            llega a seis acciones y en filas estrechas sobresalía de
                            la fila +13/+53px (iPhone SE / iPad Mini) — INVISIBLE
                            pero inflando el scrollWidth que recorta la lista. El
                            barrido nocturno lo lleva en rojo desde que aprendió a
                            nombrar culpables ocultos (runs 2138 y siguientes). Con
                            el tope, la barra se queda DENTRO de la fila y el exceso
                            se recorre dentro (foco te lleva al botón, gesto en
                            táctil), sin scrollbar que ensucie el reveal. */}
                        <span class="relative flex h-6 min-h-[24px] w-full min-w-0 items-center">
                          <span class="text-text-subtle min-w-0 flex-1 truncate transition-opacity duration-150 group-focus-within:opacity-0 group-hover:opacity-0">
                            {email.preview || t('list.noPreview')}
                          </span>
                          {!readOnly ? (
                            <span
                              role="group"
                              aria-label={t('list.rowActions', { subject })}
                              class="pointer-events-none invisible absolute inset-y-0 left-0 flex max-w-full [scrollbar-width:none] items-center gap-0.5 overflow-x-auto opacity-0 transition-opacity duration-150 group-focus-within:pointer-events-auto group-focus-within:visible group-focus-within:opacity-100 group-hover:pointer-events-auto group-hover:visible group-hover:opacity-100 [&::-webkit-scrollbar]:hidden"
                            >
                              <RowAction
                                label={conv.flagged ? t('action.unstar') : t('action.star')}
                                pressed={conv.flagged}
                                onAct={() => onToggleFlag(conv)}
                                class={conv.flagged ? 'text-warning-text' : undefined}
                              >
                                <Glyph name="star" filled={conv.flagged} size={16} />
                              </RowAction>
                              <RowAction
                                label={unread ? t('action.markRead') : t('action.markUnread')}
                                onAct={() => onToggleSeen(conv)}
                              >
                                {/* Metáfora coherente con la acción: abrir (leído) / cerrar (no leído). */}
                                <Glyph name={unread ? 'read' : 'unread'} size={16} />
                              </RowAction>
                              {onArchive ? (
                                <RowAction
                                  label={t('action.archive')}
                                  onAct={() => onArchive(conv)}
                                >
                                  <Glyph name="archive" size={16} />
                                </RowAction>
                              ) : null}
                              {onTrash ? (
                                <RowAction label={t('action.trash')} onAct={() => onTrash(conv)}>
                                  <Glyph name="trash" size={16} />
                                </RowAction>
                              ) : null}
                              {onSpam ? (
                                <RowAction label={t('action.spam')} onAct={() => onSpam(conv)}>
                                  <Glyph name="spam" size={16} />
                                </RowAction>
                              ) : null}
                              {onNotSpam ? (
                                <RowAction
                                  label={t('action.notSpam')}
                                  onAct={() => onNotSpam(conv)}
                                >
                                  <Glyph name="spam" size={16} />
                                </RowAction>
                              ) : null}
                            </span>
                          ) : null}
                        </span>
                        {convLabels.length > 0 ? (
                          <span class="mt-0.5 flex flex-wrap items-center gap-1">
                            {convLabels.map((l) => (
                              <span
                                key={l.id}
                                class="border-border bg-bg-subtle text-text-muted rounded-pill text-2xs inline-flex max-w-[10rem] items-center gap-1 border px-1.5 py-px leading-tight"
                                title={l.name}
                              >
                                <span
                                  class="h-1.5 w-1.5 shrink-0 rounded-full"
                                  style={{ background: labelDot(l.color) }}
                                  aria-hidden="true"
                                />
                                <span class="truncate">{l.name}</span>
                              </span>
                            ))}
                          </span>
                        ) : null}
                      </span>
                    }
                    meta={
                      <span class="flex flex-col items-end gap-1">
                        <time
                          class={cx(
                            'whitespace-nowrap tabular-nums',
                            unread ? 'text-primary-text font-bold' : 'text-text-subtle font-normal',
                          )}
                          dateTime={email.receivedAt}
                          title={fullDate(locale, email.receivedAt)}
                        >
                          {shortDate(locale, email.receivedAt)}
                        </time>
                        {/* Un CLIP, no un Badge de texto. El Badge es una píldora
                            rellena en negrita: el elemento de más peso visual de la
                            fila, gastado en el dato menos importante, y repetido en
                            cada correo con adjunto. Además ocupaba ancho de meta
                            —«Pièce jointe» tanto que hubo que ponerle `nowrap` para
                            que no rompiera la altura de la fila—, y ese ancho se lo
                            quitaba al asunto. El nombre accesible no se pierde: sigue
                            siendo el mismo texto, en `sr-only` y en el `title`. */}
                        {conv.hasAttachment ? (
                          <span
                            title={t('list.attachment')}
                            role="img"
                            aria-label={t('list.attachment')}
                          >
                            <Glyph name="paperclip" size={14} />
                          </span>
                        ) : null}
                      </span>
                    }
                  />
                );
              })}
            </List>

            {/* Sentinel + respaldo de paginación. */}
            {hasMore ? (
              <div ref={sentinelRef} class="flex items-center justify-center px-4 py-4">
                {loadingMore ? (
                  <span class="text-text-subtle flex items-center gap-2 text-sm">
                    <Spinner size={16} label={t('list.loadingMore')} />
                    <span aria-hidden="true">{t('list.loadingMoreShort')}</span>
                  </span>
                ) : (
                  <Button variant="ghost" size="sm" onClick={onLoadMore}>
                    {t('common.loadMore')}
                  </Button>
                )}
              </div>
            ) : null}
          </>
        )}
      </div>
    </section>
  );
}
