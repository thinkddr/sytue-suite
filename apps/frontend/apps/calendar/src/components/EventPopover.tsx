/**
 * EventPopover — tarjeta de VISTA RÁPIDA de un evento. Al hacer clic en un evento
 * (en cualquier vista) se abre junto a él en vez de saltar directo al modal:
 * muestra título, hora (o «todo el día»), lugar y descripción con el color del
 * evento, y ofrece «Editar» (abre el EventModal completo) y «Eliminar». Los
 * eventos de solo lectura (suscripciones ICS) no muestran esas acciones.
 *
 * No modal (contextual): se ancla al rectángulo del evento con posición `fixed`
 * (evita el recorte de las celdas `overflow-hidden` del mes, donde el Popover
 * en-flujo de @sytue/ui no cabe). Cierra con Escape o clic fuera; gestiona el
 * foco (entra al abrir, vuelve al disparador al cerrar por teclado), atrapa el
 * Tab y navega las acciones con flechas. role="dialog". Solo Tailwind/tokens
 * @sytue/ui — hairline 1px, sin sombras pesadas ni degradados (anti AI-slop).
 *
 * En el escritorio es un popover de la suite (la superficie, el radio y la sombra del
 * `Popover` de @sytue/ui). En el TELÉFONO no cabe junto al evento —un evento de la vista
 * de mes mide 40 px en una pantalla de 390— y se presenta como la hoja de los diálogos
 * (`Modal`): el título del evento como título, el detalle en el cuerpo y las acciones en
 * el pie, a todo el ancho. Mismo contenido, mismas acciones (barrido de diálogos,
 * 2026-10-16).
 */
import type { JSX } from 'preact';
import type { Locale } from '@sytue/i18n';
import { useLayoutEffect, useRef, useState } from 'preact/hooks';
import {
  Button,
  Icon,
  cx,
  limitesViewport,
  observarViewport,
  useMobileBack,
  useUltimo,
  Modal,
  useEnHoja,
} from '@sytue/ui';
import { EventNotes } from './EventNotes.js';
import type { GridEvent } from './CalendarGrid.js';
import { eventInstant, parseTzid } from './dates.js';
import { formatWhen } from '../lib/search.js';
import type { T } from '../i18n/index.js';

interface Props {
  /** Traductor ya atado al idioma de la petición; baja del padre. */
  t: T;
  locale: Locale;
  event: GridEvent | null;
  /** Rectángulo (viewport) del elemento del evento pulsado; ancla del popover. */
  anchor: DOMRect | null;
  /** Color de reserva si el evento no trae el de su calendario. */
  accentColor: string;
  onClose: () => void;
  onEdit: (event: GridEvent) => void;
  onDelete: (event: GridEvent) => void;
  /** Duplicar: el mismo evento, sin identidad, en el modal de creación. */
  onDuplicate?: ((event: GridEvent) => void) | undefined;
  /**
   * Responder a la invitación desde AQUÍ.
   *
   * El servicio acepta el PARTSTAT desde hace tiempo y solo se podía responder
   * desde la bandeja de invitaciones — pero quien abre una invitación casi nunca
   * viene de la bandeja: viene de ver el hueco en su semana y pulsarlo. Al llegar
   * aquí no había nada que responder, así que había que salir, buscar la bandeja y
   * encontrar el mismo evento otra vez.
   *
   * Opcional: sin él no se pinta nada, y así el popover sigue sirviendo donde no
   * hay invitación que contestar.
   */
  onResponder?: (event: GridEvent, status: 'accepted' | 'tentative' | 'declined') => void;
}

const MARGIN = 8; // margen mínimo al borde del viewport
const GAP = 6; // separación evento↔popover
const WIDTH = 288; // ancho fijo (w-72): base de medida para el volteo horizontal

// Mismos focusables que el Modal/Popover de @sytue/ui: extremos del atrapado de Tab.
const FOCUSABLE =
  'a[href],button:not([disabled]),textarea:not([disabled]),input:not([disabled]),select:not([disabled]),[tabindex]:not([tabindex="-1"])';

/**
 * Línea de «cuándo» del evento pulsado. Resuelve sus instantes a la zona de
 * visualización (honra el TZID) y delega el TEXTO en `formatWhen`, la misma
 * plantilla del catálogo que usa el resultado de búsqueda.
 */
function whenLabel(t: T, locale: Locale, event: GridEvent): string {
  const tz = parseTzid(event.ical);
  const start = eventInstant(event.start, tz);
  if (!start) return '';
  const end = event.end ? eventInstant(event.end, tz) : null;
  return formatWhen(t, locale, start, end, /^\d{8}$/.test(event.start));
}

export function EventPopover({
  t,
  locale,
  event,
  anchor,
  accentColor,
  onClose,
  onEdit,
  onDelete,
  onDuplicate,
  onResponder,
}: Props): JSX.Element | null {
  const panelRef = useRef<HTMLDivElement>(null);
  // Elemento a devolver el foco al cerrar por teclado; sí por Esc/X, no por clic
  // fuera (robaría el foco de donde el usuario acaba de pulsar) ni al ir al modal.
  const restoreRef = useRef<HTMLElement | null>(null);
  const shouldRestoreRef = useRef(true);
  // onClose vía ref: el efecto se enlaza solo a la identidad del evento y siempre
  // invoca el callback más reciente aunque el padre re-renderice.
  const onCloseRef = useUltimo(onClose);

  const [pos, setPos] = useState<{ top: number; left: number } | null>(null);
  // En el teléfono, la hoja de los diálogos: el foco, Escape y el cierre los lleva `Modal`.
  const enHoja = useEnHoja();
  // «Atrás» cierra la vista rápida; en la hoja lo hace el `Modal`.
  useMobileBack(Boolean(event && anchor) && !enHoja, onClose);

  // Coloca el panel: preferentemente bajo el evento y alineado a su izquierda;
  // voltea arriba si no cabe y se acota al viewport. Mide el panel ya montado.
  useLayoutEffect(() => {
    if (!event || !anchor) {
      setPos(null);
      return;
    }
    const colocar = (): void => {
      const panel = panelRef.current;
      const v = limitesViewport(MARGIN);
      if (panel) {
        panel.style.maxWidth = `${v.width}px`;
        panel.style.maxHeight = `${v.height}px`;
      }
      const h = panel?.offsetHeight ?? 0;
      const w = panel?.offsetWidth ?? WIDTH;
      const left = Math.max(v.left, Math.min(anchor.left, v.right - w));

      let top = Math.max(v.top, anchor.bottom + GAP);
      if (top + h > v.bottom) {
        const above = anchor.top - GAP - h;
        top = above >= v.top ? above : Math.max(v.top, v.bottom - h);
      }
      setPos({ top, left });
    };
    colocar();
    // Las notas vinculadas llegan después de abrir: su carga puede aumentar la
    // ficha y obliga a volver a encajarla, igual que un cambio de orientación.
    const observer = new ResizeObserver(colocar);
    if (panelRef.current) observer.observe(panelRef.current);
    const dejarDeObservar = observarViewport(colocar);
    return () => {
      observer.disconnect();
      dejarDeObservar();
    };
  }, [event, anchor]);

  // Foco de entrada, cierre por Esc/clic fuera/scroll, atrapado de Tab, flechas y
  // retorno de foco. Se re-arma con cada evento distinto (identidad de `event`).
  useLayoutEffect(() => {
    if (!event || enHoja) return;
    restoreRef.current = (document.activeElement as HTMLElement | null) ?? null;
    shouldRestoreRef.current = true;

    const close = (restore: boolean): void => {
      shouldRestoreRef.current = restore;
      onCloseRef.current();
    };
    const onKeyDown = (e: KeyboardEvent): void => {
      const panel = panelRef.current;
      if (!panel) return;
      if (e.key === 'Escape') {
        e.stopPropagation();
        close(true);
        return;
      }
      const nodes = Array.from(panel.querySelectorAll<HTMLElement>(FOCUSABLE));
      if (nodes.length === 0) return;
      const first = nodes[0]!;
      const last = nodes[nodes.length - 1]!;
      if (e.key === 'Tab') {
        if (e.shiftKey && document.activeElement === first) {
          e.preventDefault();
          last.focus();
        } else if (!e.shiftKey && document.activeElement === last) {
          e.preventDefault();
          first.focus();
        }
        return;
      }
      // Flechas: navegación circular entre las acciones (accesibilidad de diálogo).
      if (
        e.key === 'ArrowDown' ||
        e.key === 'ArrowRight' ||
        e.key === 'ArrowUp' ||
        e.key === 'ArrowLeft'
      ) {
        const i = nodes.indexOf(document.activeElement as HTMLElement);
        if (i === -1) return;
        e.preventDefault();
        const fwd = e.key === 'ArrowDown' || e.key === 'ArrowRight';
        nodes[(i + (fwd ? 1 : nodes.length - 1)) % nodes.length]!.focus();
      }
    };
    const onPointerDown = (e: PointerEvent): void => {
      if (!panelRef.current?.contains(e.target as Node)) close(false);
    };
    const onScroll = (e: Event): void => {
      // Leer una ficha alta no la desancla: solo un scroll de fuera la cierra.
      if (e.target instanceof Node && panelRef.current?.contains(e.target)) return;
      close(false);
    };

    document.addEventListener('keydown', onKeyDown, true);
    // `pointerdown` y NO `mousedown`: en semana/día la vista rápida se abre desde
    // el `pointerup` de la rejilla, y en táctil el navegador aún emite después el
    // `mousedown` de compatibilidad de ESE MISMO toque. Con `mousedown` el
    // popover se cerraba a los milisegundos de abrirse en cualquier móvil, sin
    // que se llegara a leer nada. El `pointerdown` que lo abrió ya pasó cuando se
    // engancha este oyente, así que solo cierra el toque SIGUIENTE.
    document.addEventListener('pointerdown', onPointerDown, true);
    // Un scroll de la rejilla desancla el popover: ciérralo (sin robar foco).
    window.addEventListener('scroll', onScroll, true);

    // Foco de entrada en el propio diálogo (lee su aria-label); `preventScroll`
    // evita que el foco dispare el listener de scroll y lo cierre al instante.
    panelRef.current?.focus({ preventScroll: true });

    return () => {
      document.removeEventListener('keydown', onKeyDown, true);
      document.removeEventListener('pointerdown', onPointerDown, true);
      window.removeEventListener('scroll', onScroll, true);
      if (shouldRestoreRef.current) restoreRef.current?.focus?.({ preventScroll: true });
    };
    // Solo la identidad del evento (y la presentación) re-arma el ciclo; el resto son
    // refs estables.
  }, [event, enHoja, onCloseRef]);

  if (!event || !anchor) return null;

  const color = event.color ?? accentColor;
  const when = whenLabel(t, locale, event);
  const readOnly = Boolean(event.readOnly);
  const titulo = event.summary || t('event.untitled');

  // Las acciones de la hoja, en el pie del diálogo: «Eliminar» arriba y la pareja
  // Duplicar | Editar debajo, con el principal abajo a la derecha.
  const acciones = (): JSX.Element =>
    readOnly ? (
      <>
        {onDuplicate ? (
          <Button
            variant="secondary"
            onClick={() => {
              shouldRestoreRef.current = false;
              onDuplicate(event);
            }}
          >
            {t('popover.copyToMine')}
          </Button>
        ) : null}
      </>
    ) : (
      <>
        <Button
          variant="danger"
          class="md:mr-auto"
          onClick={() => {
            shouldRestoreRef.current = false;
            onDelete(event);
          }}
        >
          {t('common.delete')}
        </Button>
        {onDuplicate ? (
          <Button
            variant="secondary"
            onClick={() => {
              shouldRestoreRef.current = false;
              onDuplicate(event);
            }}
          >
            {t('popover.duplicate')}
          </Button>
        ) : null}
        <Button
          variant="primary"
          onClick={() => {
            shouldRestoreRef.current = false; // el foco pasa al modal
            onEdit(event);
          }}
        >
          {t('common.edit')}
        </Button>
      </>
    );

  if (enHoja) {
    return (
      <Modal
        open
        onClose={onClose}
        title={titulo}
        size="sm"
        footer={readOnly && !onDuplicate ? undefined : acciones()}
      >
        <div class="flex flex-col gap-3">
          <dl class="flex flex-col gap-1.5">
            {when ? (
              <div class="text-text-muted flex items-center gap-2 text-sm tabular-nums">
                <span
                  class="h-2.5 w-2.5 shrink-0 rounded-full"
                  style={{ backgroundColor: color }}
                  aria-hidden="true"
                />
                <dt class="sr-only">{t('popover.when')}</dt>
                <dd>{when}</dd>
              </div>
            ) : null}
            {event.location ? (
              <div class="text-text-muted text-sm break-words">
                <dt class="sr-only">{t('popover.location')}</dt>
                <dd>{event.location}</dd>
              </div>
            ) : null}
            {event.description ? (
              <div class="text-text text-sm break-words">
                <dt class="sr-only">{t('popover.description')}</dt>
                <dd class="whitespace-pre-line">{event.description}</dd>
              </div>
            ) : null}
          </dl>
          {event.conferenceUrl ? (
            <Button
              href={event.conferenceUrl}
              target="_blank"
              rel="noreferrer"
              variant="primary"
              fullWidth
            >
              {t('popover.join')}
            </Button>
          ) : null}
          <EventNotes t={t} eventUid={event.uid} summary={event.summary} />
          {onResponder ? (
            <div class="border-border-subtle flex flex-col gap-2 border-t pt-3">
              <span class="sytue-meta">{t('popover.respond')}</span>
              <div class="grid grid-cols-3 gap-2">
                {(['accepted', 'tentative', 'declined'] as const).map((estado) => (
                  <Button
                    key={estado}
                    variant={estado === 'accepted' ? 'primary' : 'secondary'}
                    onClick={() => onResponder(event, estado)}
                  >
                    {t(
                      estado === 'accepted'
                        ? 'popover.yes'
                        : estado === 'tentative'
                          ? 'popover.maybe'
                          : 'popover.no',
                    )}
                  </Button>
                ))}
              </div>
            </div>
          ) : null}
          {readOnly ? <p class="sytue-meta">{t('popover.readOnly')}</p> : null}
        </div>
      </Modal>
    );
  }

  return (
    <div
      ref={panelRef}
      role="dialog"
      aria-modal="false"
      aria-label={t('popover.details', { title: event.summary || t('event.untitledLong') })}
      tabIndex={-1}
      class={cx(
        'sytue-popover-surface border-border fixed w-72 max-w-[calc(100vw-16px)]',
        'max-h-[calc(100dvh-16px)] overflow-y-auto overscroll-contain',
        'rounded-lg border p-3.5 shadow-[var(--shadow-md)] focus-visible:outline-none',
      )}
      style={{
        top: `${pos?.top ?? anchor.bottom + GAP}px`,
        left: `${pos?.left ?? anchor.left}px`,
        zIndex: 'var(--z-popover)',
        // Invisible hasta medir/colocar (evita el salto), pero ya en el DOM para medir.
        visibility: pos ? 'visible' : 'hidden',
      }}
    >
      <div class="flex items-start gap-2">
        <span
          class="mt-1 h-2.5 w-2.5 shrink-0 rounded-full"
          style={{ backgroundColor: color }}
          aria-hidden="true"
        />
        <h2 class="text-text min-w-0 flex-1 text-sm leading-snug font-semibold break-words">
          {event.summary || t('event.untitled')}
        </h2>
        <Button
          variant="ghost"
          size="sm"
          iconOnly
          aria-label={t('common.close')}
          onClick={onClose}
          class="-mt-0.5 -mr-1 shrink-0"
          leadingIcon={<Icon name="close" size={16} />}
        />
      </div>

      <dl class="mt-2 flex flex-col gap-1.5 pl-[18px]">
        {when ? (
          <div class="text-text-muted text-xs tabular-nums">
            <dt class="sr-only">{t('popover.when')}</dt>
            <dd>{when}</dd>
          </div>
        ) : null}
        {event.location ? (
          <div class="text-text-muted min-w-0 text-xs">
            <dt class="sr-only">{t('popover.location')}</dt>
            <dd class="truncate" title={event.location}>
              {event.location}
            </dd>
          </div>
        ) : null}
        {event.conferenceUrl ? (
          <div class="min-w-0 text-xs">
            <dt class="sr-only">{t('popover.conference')}</dt>
            <dd>
              {/* «Unirse» ANTES que el resto del detalle: cuando llega la hora, entrar
                  es lo único que se quiere hacer con el evento. */}
              <a
                href={event.conferenceUrl}
                target="_blank"
                rel="noreferrer"
                class="bg-primary sytue-focus-ring rounded-control inline-flex items-center gap-1.5 px-2.5 py-1 text-xs font-semibold text-[var(--sytue-on-primary)] transition-colors duration-150 hover:bg-[var(--sytue-primary-hover)]"
              >
                {t('popover.join')}
              </a>
            </dd>
          </div>
        ) : null}
        {event.description ? (
          <div class="sytue-meta min-w-0">
            <dt class="sr-only">{t('popover.description')}</dt>
            <dd class="line-clamp-3 break-words">{event.description}</dd>
          </div>
        ) : null}
      </dl>

      {/* Las notas vinculadas (Sytue Notepads) van pegadas al detalle y ANTES de las
          acciones: son contexto de la reunión —el acta, el guion—, no una acción
          que se ejecute sobre el evento. Se pinta sola si no hay Notes detrás. */}
      <EventNotes t={t} eventUid={event.uid} summary={event.summary} />

      {/* Responder va ANTES de editar y borrar, y también en los eventos de solo
          lectura: la invitación de otro no se edita, pero sí se contesta — y
          contestarla es justo lo único que se puede hacer con ella. */}
      {onResponder ? (
        <div class="border-border mt-3 flex flex-wrap items-center gap-2 border-t pt-2.5">
          <span class="sytue-meta w-full">{t('popover.respond')}</span>
          {(['accepted', 'tentative', 'declined'] as const).map((estado) => (
            <Button
              key={estado}
              variant={estado === 'accepted' ? 'primary' : 'secondary'}
              size="sm"
              onClick={() => {
                shouldRestoreRef.current = false;
                onResponder(event, estado);
              }}
            >
              {t(
                estado === 'accepted'
                  ? 'popover.yes'
                  : estado === 'tentative'
                    ? 'popover.maybe'
                    : 'popover.no',
              )}
            </Button>
          ))}
        </div>
      ) : null}

      {readOnly ? (
        <div class="border-border mt-3 flex flex-wrap items-center gap-2 border-t pt-2.5">
          <p class="sytue-meta w-full">{t('popover.readOnly')}</p>
          {/* Lo único que se puede hacer con el evento de otro: copiártelo. La
              exportación a .ics del calendario entero vive en el menú «⋮» del
              calendario, en la barra lateral. */}
          {onDuplicate ? (
            <Button
              variant="secondary"
              size="sm"
              onClick={() => {
                shouldRestoreRef.current = false;
                onDuplicate(event);
              }}
            >
              {t('popover.copyToMine')}
            </Button>
          ) : null}
        </div>
      ) : (
        // `flex-wrap`: tres botones con la etiqueta larga de algunos idiomas
        // («Elimina» catalán, «Löschen» alemán) no caben en los 288px fijos del
        // popover en una sola fila, y sin envolver se salían por el borde.
        <div class="border-border mt-3 flex flex-wrap items-center gap-2 border-t pt-2.5">
          <Button
            variant="primary"
            size="sm"
            onClick={() => {
              shouldRestoreRef.current = false; // el foco pasa al modal
              onEdit(event);
            }}
          >
            {t('common.edit')}
          </Button>
          {onDuplicate ? (
            <Button
              variant="secondary"
              size="sm"
              onClick={() => {
                shouldRestoreRef.current = false;
                onDuplicate(event);
              }}
            >
              {t('popover.duplicate')}
            </Button>
          ) : null}
          <Button
            variant="danger"
            size="sm"
            onClick={() => {
              shouldRestoreRef.current = false;
              onDelete(event);
            }}
          >
            {t('common.delete')}
          </Button>
        </div>
      )}
    </div>
  );
}
