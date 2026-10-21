/**
 * CalendarApp — isla raiz de Sytue Calendar. Monta la Sytue Shell y, debajo, el
 * AppShell canonico (sidebar de calendarios + barra de control de vista + rejilla
 * mes/semana/dia). Multi-calendario: la sidebar conmuta la visibilidad de cada
 * calendario (checkbox con su color), los eventos de los visibles se cargan en
 * paralelo y se superponen en la rejilla, y cada calendario se crea/renombra/
 * recolorea/borra desde aqui (CalendarModal + kebab). Crea/edita eventos via
 * EventModal -> BFF CalDAV (servicio Go Sytue Calendar). Estetica Sytue, modo
 * claro/oscuro. docs/CONVENTIONS.md §1, §5.
 */
import type { JSX } from 'preact';
import { translate, type Locale } from '@sytue/i18n';
import { useEffect, useMemo, useRef, useState } from 'preact/hooks';
import { SytueShell, ImportData, LinkedServices, FranjaPeticion } from '@sytue/shell';
import { useConnectivity } from '@sytue/offline/preact';
import {
  AppShell,
  Sidebar,
  Badge,
  Button,
  Dropdown,
  FILA_UNICA_EN_MOVIL,
  MenuItem,
  MenuSeparator,
  MASCARA_SCROLL_X,
  Icon,
  Skeleton,
  EmptyState,
  Illustration,
  esAtajoDeApp,
  useToast,
  cx,
  useScrollRestante,
  MobileFab,
  SidebarAction,
  Checkbox,
  Glifo,
} from '@sytue/ui';
import type { SytueUser } from '@sytue/auth';
import {
  calendarIcsUrl,
  listCalendars,
  listEventsOf,
  listInvitations,
  listSharedWithMe,
  refreshSubscription,
  saveEvent,
  deleteEvent,
  type Calendar,
  type CalendarEvent,
  type EventInput,
  type RecurrenceScope,
} from '../client/calendarApi.js';
import { applyRecurringEdit, applyRecurringDelete, type RecurTarget } from '../lib/recurrence.js';
import { getAccessToken } from '@sytue/auth/client';
import {
  type ViewMode,
  AGENDA_DAYS,
  addDays,
  addMonths,
  monthGrid,
  weekGrid,
  startOfDay,
  titleFor,
  toIcalUtc,
  toDatetimeLocal,
  parseIcalDate,
  parseTzid,
  expandOccurrences,
  isToday,
  isSameDay,
  dayMonthYearLabel,
  weekdayNarrowLabels,
} from './dates.js';
import { parseAlarms, parseAttendees, parseColor, parseRecurrence } from '../lib/ical.js';
import { CalendarGrid, DEFAULT_ACCENT, type GridEvent } from './CalendarGrid.js';
import { EventModal, type EditingEvent } from './EventModal.js';
import { EventPopover } from './EventPopover.js';
import { CalendarModal, DeleteCalendarModal } from './CalendarModal.js';
import { ShareCalendarModal } from './ShareCalendarModal.js';
import { ResourcesModal } from './ResourcesModal.js';
import { SubscribeModal } from './SubscribeModal.js';
import { InvitationsModal } from './InvitationsModal.js';
import { ScopeDialog } from './ScopeDialog.js';
import { ReminderCenter } from './ReminderCenter.js';
import { SearchPanel } from './SearchPanel.js';
import { fechaDelEnlace } from '../lib/search.js';
import { useT } from '../i18n/index.js';

interface Props {
  /** Idioma ya negociado en el servidor (middleware): baja a las vistas. */
  locale: Locale;
  user: SytueUser;
  hubBaseUrl: string;
}

/** Rango iCalendar que cubre la vista actual (para la consulta CalDAV). */
function rangeFor(mode: ViewMode, cursor: Date): { start: string; end: string } {
  // Agenda: [cursor, cursor+AGENDA_DAYS) local — ventana rodante de próximos días.
  if (mode === 'agenda') {
    const start = startOfDay(cursor);
    return { start: toIcalUtc(start), end: toIcalUtc(addDays(start, AGENDA_DAYS)) };
  }
  let days: Date[];
  if (mode === 'month') days = monthGrid(cursor);
  else if (mode === 'week') days = weekGrid(cursor);
  // Dia: [00:00, 24:00) local, como mes/semana (sus grids ya van a medianoche).
  else days = [startOfDay(cursor)];
  const start = days[0]!;
  const end = addDays(days[days.length - 1]!, 1);
  return { start: toIcalUtc(start), end: toIcalUtc(end) };
}

// Controles de la cabecera de CONTENIDO, sobre SUPERFICIE clara (bg-surface):
// hover/active como hundido (bg-surface-sunken), texto de tinta (text-text) y
// anillo de foco global (--sytue-focus-ring).
const HEADER_FOCUS = 'sytue-focus-ring';
const HEADER_ICON_BTN =
  'grid h-8 w-8 place-items-center rounded-control text-text transition-colors duration-150 hover:bg-surface-sunken active:bg-surface-sunken';

/**
 * Kebab (tres puntos) — no existe en el set de @sytue/ui; SVG local. Es el
 * gemelo VERTICAL del glifo `ellipsis` del set: misma geometría (rejilla 24,
 * puntos a 6/12/18, trazo 2 con cap redondo, currentColor), para que no pese
 * distinto que el resto de iconos de la misma superficie.
 */
function KebabGlyph(): JSX.Element {
  return <Glifo d="M12 6h.01M12 12h.01M12 18h.01" size={16} />;
}

/** Tecla que conmuta cada vista (la misma letra que en Google Calendar). */
const VIEW_KEY: Record<ViewMode, string> = { month: 'M', week: 'W', day: 'D', agenda: 'A' };

/** Validación mínima de email para sanear los invitados del deep-link. */
const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

/** Fecha redondeada a la siguiente hora en punto (inicio por defecto sin hora dada). */
function roundToNextHour(d: Date): Date {
  const r = new Date(d);
  r.setMinutes(0, 0, 0);
  r.setHours(r.getHours() + 1);
  return r;
}

/**
 * Lee y SANEA el intent «crear evento desde correo» de la query (el Hub la compone;
 * docs/CONVENTIONS.md §11). Devuelve el evento prerellenado o null si no aplica.
 * Todo se trata como dato no confiable: título/descripción se recortan (Preact ya
 * los escapa al pintarlos como texto, sin inyección), `start` se parsea de forma
 * estricta (se ignora si no es una fecha válida) y `attendees` se filtra a emails
 * válidos, deduplicados y acotados. Nunca se interpola HTML.
 */
function readCreateIntent(search: string): EditingEvent | null {
  const p = new URLSearchParams(search);
  if (p.get('action') !== 'create') return null;
  const summary = (p.get('title') ?? '').slice(0, 500);
  const description = (p.get('description') ?? '').slice(0, 5000);
  const startRaw = p.get('start') ?? '';
  const parsed = startRaw ? new Date(startRaw) : null;
  const start = parsed && !Number.isNaN(parsed.getTime()) ? parsed : roundToNextHour(new Date());
  const end = new Date(start.getTime() + 60 * 60 * 1000);
  const invitees = Array.from(
    new Set(
      (p.get('attendees') ?? '')
        .split(',')
        .map((s) => s.trim().toLowerCase())
        .filter((s) => EMAIL_RE.test(s)),
    ),
  ).slice(0, 100);
  return {
    href: null,
    summary,
    start,
    end,
    ...(description ? { description } : {}),
    ...(invitees.length ? { invitees } : {}),
  };
}

export function CalendarApp({ user, hubBaseUrl, locale }: Props): JSX.Element {
  const t = useT(locale);
  const offline = useConnectivity() === 'offline';
  const [calendars, setCalendars] = useState<Calendar[]>([]);
  // Calendarios compartidos CON el usuario («Compartido conmigo»): se pintan por
  // el mismo camino CalDAV que los propios (su URL es la del dueño). Un reader es
  // readOnly (arrastrar/editar bloqueado, como una suscripción ICS); un writer es
  // editable.
  const [sharedCalendars, setSharedCalendars] = useState<Calendar[]>([]);
  // Calendario propio cuyo diálogo «Compartir» está abierto (solo el dueño), o null.
  const [sharingCal, setSharingCal] = useState<Calendar | null>(null);
  // Conjunto de calendarios visibles (por URL): sus eventos se superponen.
  const [visible, setVisible] = useState<ReadonlySet<string>>(new Set());
  // URLs ya vistas: al recargar la lista, solo los calendarios NUEVOS se marcan
  // visibles; los desmarcados por el usuario conservan su estado.
  const knownUrls = useRef<Set<string>>(new Set());
  // La fila de la cabecera, para saber si queda contenido a la derecha (máscara).
  const cabecera = useScrollRestante<HTMLElement>();
  // Secuencia de cargas de calendarios: descarta respuestas fuera de orden. En un
  // arranque en frío el primer GET /api/calendars (compilación de la ruta) puede
  // tardar y resolver DESPUÉS del GET disparado tras crear/renombrar/borrar; sin
  // este guardia esa respuesta rancia pisaría la lista fresca y la sidebar
  // mostraría el nombre viejo indefinidamente (bug de renombrar). Igual que el
  // efecto de eventos, solo la última carga aplica su resultado.
  const calLoadSeq = useRef(0);
  // Cache de eventos por calendario, valida solo para su rango (clave `range`).
  const [eventsByCal, setEventsByCal] = useState<{
    range: string;
    byCal: Record<string, CalendarEvent[]>;
  }>({
    range: '',
    byCal: {},
  });
  const [seq, setSeq] = useState(0); // fuerza la recarga de eventos (guardar/reintentar)
  const [mode, setMode] = useState<ViewMode>('month');
  const [cursor, setCursor] = useState(new Date());
  const [loadingCals, setLoadingCals] = useState(true);
  const [loadingEvents, setLoadingEvents] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [modal, setModal] = useState<{ open: boolean; editing: EditingEvent | null }>({
    open: false,
    editing: null,
  });
  const [calModal, setCalModal] = useState<{ open: boolean; editing: Calendar | null }>({
    open: false,
    editing: null,
  });
  const [subscribeOpen, setSubscribeOpen] = useState(false);
  const [resourcesOpen, setResourcesOpen] = useState(false);
  // Solo los administradores del tenant dan de alta/gestionan salas y recursos.
  const canManageResources = user.roles.includes('admin') || user.roles.includes('owner');
  const [searchOpen, setSearchOpen] = useState(false);
  const [deleting, setDeleting] = useState<Calendar | null>(null);
  const [invitesOpen, setInvitesOpen] = useState(false);
  const [pendingInvites, setPendingInvites] = useState(0);
  // Arrastre de una instancia recurrente pendiente de elegir alcance.
  const [dragScope, setDragScope] = useState<{
    event: GridEvent;
    start: Date;
    end: Date;
  } | null>(null);
  // Vista rápida (popover) del evento pulsado, anclada a su rectángulo.
  const [quickView, setQuickView] = useState<{ event: GridEvent; anchor: DOMRect } | null>(null);
  // Borrado de una serie recurrente desde la vista rápida, pendiente de alcance.
  const [deleteScope, setDeleteScope] = useState<GridEvent | null>(null);
  const toast = useToast();

  // Nº de invitaciones pendientes para el badge de la cabecera (carga inicial).
  useEffect(() => {
    void listInvitations(true)
      .then(({ invitations }) => setPendingInvites(invitations.length))
      .catch(() => {});
  }, []);

  // Deep-link «crear evento desde correo» (intent del Hub): abre el modal de
  // creación prerellenado con los datos saneados de la query. Se lee UNA vez al
  // montar y se limpia la URL (un refresco/volver no debe reabrirlo con datos
  // rancios). Los calendarios pueden llegar después: EventModal fija el destino
  // por defecto en cuanto cargan.
  useEffect(() => {
    const editing = readCreateIntent(window.location.search);
    if (!editing) return;
    window.history.replaceState(null, '', window.location.pathname);
    if (!offline) setModal({ open: true, editing });
  }, [offline]);

  // Enlace a un evento (el que compone el buscador global de la suite): la vista se
  // abre en su día, igual que un resultado de la búsqueda propia. Una vez al montar,
  // y la fecha sale de la URL para que volver o refrescar no la reaplique.
  useEffect(() => {
    const dia = fechaDelEnlace(window.location.search);
    if (!dia) return;
    window.history.replaceState(null, '', window.location.pathname);
    setCursor(dia);
    setMode('day');
  }, []);

  // El aviso de una petición de edición (`hubnotify.EnlaceCalendario`) trae el
  // calendario: si es mío, su franja me enseña quién lo pide y me deja concederlo.
  const [calendarioDelAviso] = useState(() =>
    typeof window === 'undefined'
      ? null
      : new URLSearchParams(window.location.search).get('calendar'),
  );

  // El aviso de una invitación (`hubnotify.EnlaceInvitaciones`) abre la bandeja: el
  // evento aún está en el calendario de quien invita y aquí no se puede leer.
  useEffect(() => {
    const q = new URLSearchParams(window.location.search);
    if (!q.get('invitations')) return;
    q.delete('invitations');
    const resto = q.toString();
    window.history.replaceState(null, '', window.location.pathname + (resto ? `?${resto}` : ''));
    setInvitesOpen(true);
  }, []);

  // Una pérdida de red convierte Calendar en consulta. Cierra cualquier flujo
  // de escritura que estuviera abierto, pero conserva búsqueda y vista rápida.
  useEffect(() => {
    if (!offline) return;
    setModal({ open: false, editing: null });
    setCalModal({ open: false, editing: null });
    setSubscribeOpen(false);
    setSharingCal(null);
    setResourcesOpen(false);
    setDeleting(null);
    setInvitesOpen(false);
    setDragScope(null);
    setDeleteScope(null);
  }, [offline]);

  // Carga de calendarios (tambien desde «Reintentar» y tras crear/editar/borrar).
  // Propios y «compartidos conmigo» en paralelo; que fallen los compartidos no debe
  // tumbar la vista (degradan a lista vacía).
  function loadCalendars(): void {
    const seq = ++calLoadSeq.current;
    setLoadingCals(true);
    void Promise.all([
      listCalendars(),
      listSharedWithMe().catch(() => ({ calendars: [] as Calendar[] })),
    ])
      .then(([{ calendars: cals }, { calendars: shared }]) => {
        if (seq !== calLoadSeq.current) return; // respuesta obsoleta: llegó otra carga después
        setCalendars(cals);
        setSharedCalendars(shared);
        const all = [...cals, ...shared];
        setVisible((prev) => {
          const next = new Set<string>();
          for (const c of all) {
            // Primera carga: todos visibles. Después: conserva la selección y
            // enciende solo los recién creados/compartidos.
            if (knownUrls.current.size === 0 || prev.has(c.url) || !knownUrls.current.has(c.url)) {
              next.add(c.url);
            }
          }
          return next;
        });
        knownUrls.current = new Set(all.map((c) => c.url));
        setError(null);
      })
      .catch(() => {
        if (seq === calLoadSeq.current) setError(t('cal.error.loadCalendars'));
      })
      .finally(() => {
        if (seq === calLoadSeq.current) setLoadingCals(false);
      });
  }

  // Solo al montar; el reintento manual reutiliza la misma funcion. `loadCalendars`
  // se declara en el cuerpo del componente y cambia de identidad en cada render:
  // meterla en las dependencias recargaría los calendarios sin parar.
  useEffect(() => {
    loadCalendars();
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const range = useMemo(() => rangeFor(mode, cursor), [mode, cursor]);
  const rangeKey = `${range.start}..${range.end}`;
  const visibleKey = [...visible].sort().join(',');

  // Propios + compartidos conmigo: ambos se cargan/pintan por el mismo camino.
  const allCalendars = useMemo(
    () => [...calendars, ...sharedCalendars],
    [calendars, sharedCalendars],
  );
  // Destinos válidos para crear/editar eventos: propios + compartidos con permiso de
  // ESCRITURA (writer). Los readers quedan fuera (no editables), como una suscripción.
  const eventTargets = useMemo(
    () => [...calendars, ...sharedCalendars.filter((c) => !c.readOnly)],
    [calendars, sharedCalendars],
  );

  // Carga de eventos: un listEvents por calendario visible aun no cacheado para
  // el rango actual, en paralelo. Conmutar un checkbox ya cargado es instantaneo.
  useEffect(() => {
    const byCal = eventsByCal.range === rangeKey ? eventsByCal.byCal : {};
    const missing = allCalendars.filter((c) => visible.has(c.url) && !(c.url in byCal));
    if (missing.length === 0) {
      // Nada que pedir (p. ej. rango nuevo sin visibles, o toggle a apagado):
      // fija la clave de rango y apaga cualquier loading de una carga cancelada.
      if (eventsByCal.range !== rangeKey) setEventsByCal({ range: rangeKey, byCal });
      setLoadingEvents(false);
      return;
    }
    let stale = false;
    setLoadingEvents(true);
    void listEventsOf(missing, range)
      .then(({ byCal: loaded, sinAcceso }) => {
        if (stale) return;
        setEventsByCal((prev) => ({
          range: rangeKey,
          byCal: { ...(prev.range === rangeKey ? prev.byCal : {}), ...loaded },
        }));
        setError(null);
        if (sinAcceso.length === 0) return;
        // Se quita de la vista: si siguiera en la lista, cada cambio de rango lo
        // volvería a pedir para recibir el mismo 403.
        const fuera = new Set(sinAcceso.map((c) => c.url));
        setCalendars((prev) => prev.filter((c) => !fuera.has(c.url)));
        setSharedCalendars((prev) => prev.filter((c) => !fuera.has(c.url)));
        for (const c of sinAcceso)
          toast.notify({
            tone: 'warning',
            title: t('cal.error.noAccess', { name: c.displayName }),
          });
      })
      .catch(() => {
        if (!stale) setError(t('cal.error.loadEvents'));
      })
      .finally(() => {
        if (!stale) setLoadingEvents(false);
      });
    return () => {
      stale = true;
    };
    // La cache (eventsByCal) se lee pero no dispara: solo rango/visibles/lista/seq.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [rangeKey, visibleKey, allCalendars, seq]);

  // Fusion de los eventos visibles, cada uno etiquetado con el color y la URL de
  // su calendario; el backend devuelve el VEVENT maestro con su RRULE (sin
  // expandir) y aqui se expanden las ocurrencias del rango visible.
  const shownEvents = useMemo<GridEvent[]>(() => {
    const byCal = eventsByCal.range === rangeKey ? eventsByCal.byCal : {};
    const merged: GridEvent[] = [];
    for (const c of allCalendars) {
      if (!visible.has(c.url)) continue;
      for (const ev of byCal[c.url] ?? [])
        // Color POR evento (X-APPLE-CALENDAR-COLOR/COLOR del VEVENT) si lo tiene;
        // si no, el color del calendario de origen. readOnly marca los eventos de
        // una suscripción ICS (no arrastrables/editables).
        merged.push({
          ...ev,
          color: (ev.ical ? parseColor(ev.ical) : undefined) ?? c.color,
          calendarUrl: c.url,
          readOnly: c.readOnly || offline,
        });
    }
    const from = parseIcalDate(range.start);
    const to = parseIcalDate(range.end);
    // expandOccurrences copia cada ocurrencia por spread: conserva color/calendarUrl.
    return from && to ? expandOccurrences(merged, from, to) : merged;
    // `visible` se relee vía `visibleKey` (recomputa al cambiar la selección).
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [eventsByCal, visibleKey, allCalendars, rangeKey, range.start, range.end, offline]);

  /** Invalida la cache y relanza la carga (tras guardar/borrar un evento). */
  function reload(): void {
    setEventsByCal({ range: '', byCal: {} });
    setSeq((s) => s + 1);
  }

  function toggleVisible(url: string): void {
    setVisible((prev) => {
      const next = new Set(prev);
      if (next.has(url)) next.delete(url);
      else next.add(url);
      return next;
    });
  }

  function navigate(delta: number): void {
    setCursor((c) =>
      mode === 'month'
        ? addMonths(c, delta)
        : mode === 'agenda'
          ? addDays(c, delta * AGENDA_DAYS)
          : addDays(c, mode === 'week' ? delta * 7 : delta),
    );
  }

  // Atajos de teclado, los mismos que Apple Calendar y Google Calendar: cambiar de
  // vista o de semana sin soltar el teclado es la diferencia entre una agenda y una
  // web con eventos. Se ignoran mientras se escribe, con modificador (⌘D es un
  // marcador del navegador) y con cualquier diálogo abierto, que tiene sus propias
  // teclas. Se enlaza al documento porque la rejilla no siempre tiene el foco.
  useEffect(() => {
    const onKey = (e: KeyboardEvent): void => {
      // `esAtajoDeApp` de @sytue/ui: la misma comprobación que hacían por su
      // cuenta Sytue Mail, Sytue Calendar y el visor de PDF, con tres criterios
      // distintos. Esta además descarta los menús abiertos (`role="menu"`), que
      // aquí no se miraban: con un desplegable desplegado, pulsar «d» cambiaba de
      // vista por detrás del menú.
      if (!esAtajoDeApp(e)) return;
      const k = e.key.length === 1 ? e.key.toLowerCase() : e.key;
      if (k === 't') setCursor(new Date());
      else if (k === 'm') setMode('month');
      else if (k === 'w') setMode('week');
      else if (k === 'd') setMode('day');
      else if (k === 'a') setMode('agenda');
      else if (k === 'n' || k === 'j') navigate(1);
      else if (k === 'p' || k === 'k') navigate(-1);
      else if (k === 'c') openCreate(cursor);
      else if (k === '/') setSearchOpen(true);
      else return;
      e.preventDefault();
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
    // `navigate` y `openCreate` se redeclaran en cada render: el oyente se vuelve a
    // enlazar cuando cambia lo que leen del cierre (vista, cursor y conectividad).
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, [mode, cursor, offline]);

  function openCreate(day: Date): void {
    if (offline) return;
    const start = new Date(day);
    // Mes/agenda no fijan hora al elegir día: por defecto a las 9:00.
    if (mode === 'month' || mode === 'agenda') start.setHours(9, 0, 0, 0);
    const end = new Date(start);
    end.setHours(start.getHours() + 1);
    setModal({ open: true, editing: { href: null, summary: '', start, end } });
  }

  /** Crear desde la rejilla horaria: el modal se abre con el rango pintado. */
  function openCreateRange(start: Date, end: Date): void {
    if (offline) return;
    setModal({ open: true, editing: { href: null, summary: '', start, end } });
  }

  /**
   * Persistir un mover/redimensionar de la rejilla horaria (semana/día): reescribe
   * el evento con su nuevo inicio/fin por el mismo PUT CalDAV que el modal (href +
   * etag), conservando resumen/lugar/descripción y los ATTENDEE/VALARM del maestro
   * que la UI no edita. La rejilla no arrastra eventos recurrentes, así que aquí no
   * hay RRULE que reescribir. Al terminar, `reload()` trae la posición guardada.
   */
  async function moveResizeEvent(event: GridEvent, start: Date, end: Date): Promise<void> {
    if (offline) return;
    if (!event.calendarUrl || !event.href) return;
    if (event.readOnly) return; // suscripción ICS: no se mueve (la rejilla ya lo impide)
    // Recurrente: preguntar el alcance (este / este y siguientes / todos) antes de
    // persistir; el split lo aplica el ScopeDialog. La rejilla mantiene el bloque
    // en su nueva posición (optimista) hasta que la recarga concilie.
    if (event.ical && parseRecurrence(event.ical)) {
      setDragScope({ event, start, end });
      return;
    }
    const attendees = event.ical ? parseAttendees(event.ical) : [];
    const alarms = event.ical ? parseAlarms(event.ical) : [];
    const color = event.ical ? parseColor(event.ical) : undefined;
    const eventInput: EventInput = {
      summary: event.summary,
      start: toDatetimeLocal(start),
      end: toDatetimeLocal(end),
      ...(event.uid ? { uid: event.uid } : {}),
      ...(event.location ? { location: event.location } : {}),
      ...(event.description ? { description: event.description } : {}),
      ...(color ? { color } : {}),
      ...(attendees.length ? { attendees } : {}),
      ...(alarms.length ? { alarms } : {}),
    };
    await saveEvent({
      calendarUrl: event.calendarUrl,
      eventInput,
      href: event.href,
      ...(event.etag ? { etag: event.etag } : {}),
    });
    reload();
  }

  /** Aplica el alcance elegido tras arrastrar una instancia recurrente. */
  function confirmDragScope(scope: RecurrenceScope): void {
    const d = dragScope;
    if (!d || !d.event.calendarUrl || !d.event.href) return;
    const cal = allCalendars.find((c) => c.url === d.event.calendarUrl);
    const attendees = d.event.ical ? parseAttendees(d.event.ical) : [];
    const alarms = d.event.ical ? parseAlarms(d.event.ical) : [];
    const color = d.event.ical ? parseColor(d.event.ical) : undefined;
    const edited: EventInput = {
      summary: d.event.summary,
      start: toDatetimeLocal(d.start),
      end: toDatetimeLocal(d.end),
      ...(d.event.location ? { location: d.event.location } : {}),
      ...(d.event.description ? { description: d.event.description } : {}),
      ...(color ? { color } : {}),
      ...(attendees.length ? { attendees } : {}),
      ...(alarms.length ? { alarms } : {}),
    };
    const target: RecurTarget = {
      calendarId: cal?.id ?? '',
      calendarUrl: d.event.calendarUrl,
      uid: d.event.uid,
      href: d.event.href,
      etag: d.event.etag,
      masterIcal: d.event.ical ?? '',
      recurrenceId: parseIcalDate(d.event.start) ?? d.start,
      allDay: false,
    };
    const rule = d.event.ical ? (parseRecurrence(d.event.ical) ?? undefined) : undefined;
    setDragScope(null);
    void applyRecurringEdit(target, scope, edited, rule)
      .then(reload)
      .catch(() => {
        reload(); // deshace el bloque optimista de la rejilla
        toast.notify({ tone: 'danger', title: t('cal.error.updateEvent') });
      });
  }

  /** Cancela el arrastre recurrente: recarga para devolver el bloque a su sitio. */
  function cancelDragScope(): void {
    setDragScope(null);
    reload();
  }

  function openEdit(event: GridEvent): void {
    setModal({ open: true, editing: editingDe(event) });
  }

  /** El evento de la rejilla, como lo entiende el modal de edición. */
  function editingDe(event: GridEvent): EditingEvent {
    const start = parseIcalDate(event.start) ?? new Date();
    const end = event.end ? (parseIcalDate(event.end) ?? undefined) : undefined;
    // Recupera la RRULE, los invitados y los VALARM del maestro para no
    // perderlos al guardar (los recordatorios pueden venir de un cliente
    // CalDAV externo; la UI no los edita, solo los preserva).
    const recurrence = event.ical ? (parseRecurrence(event.ical) ?? undefined) : undefined;
    const invitees = event.ical ? parseAttendees(event.ical) : [];
    const alarms = event.ical ? parseAlarms(event.ical) : [];
    return {
      href: event.href,
      etag: event.etag,
      uid: event.uid,
      summary: event.summary,
      start,
      end,
      allDay: /^\d{8}$/.test(event.start),
      ...(parseTzid(event.ical) ? { tzid: parseTzid(event.ical)! } : {}),
      location: event.location,
      description: event.description,
      color: event.ical ? parseColor(event.ical) : undefined,
      conferenceUrl: event.conferenceUrl,
      recurrence,
      invitees,
      alarms,
      calendarUrl: event.calendarUrl,
      ical: event.ical,
      readOnly: event.readOnly || offline,
    };
  }

  /**
   * «Duplicar» de la vista rápida: el mismo evento SIN identidad —href, etag y
   * uid nuevos— en el modal de creación, para retocarlo y guardarlo. Vale
   * también para copiarte un evento de una suscripción de solo lectura: la
   * copia nace en tu calendario por defecto, y esa sí es tuya.
   */
  function duplicateFromQuickView(event: GridEvent): void {
    setQuickView(null);
    if (offline) return;
    setModal({
      open: true,
      editing: {
        ...editingDe(event),
        href: null,
        etag: undefined,
        uid: undefined,
        ical: undefined,
        readOnly: false,
        ...(event.readOnly ? { calendarUrl: undefined } : {}),
      },
    });
  }

  /** Clic en un evento (cualquier vista): abre la vista rápida junto a él. */
  function openQuickView(event: GridEvent, anchor: DOMRect): void {
    setQuickView({ event, anchor });
  }

  /** «Editar» de la vista rápida: cierra el popover y abre el modal completo. */
  /**
   * Responder a la invitación desde la vista rápida.
   *
   * El servicio acepta el PARTSTAT desde hace tiempo y hasta ahora solo se podía
   * contestar desde la bandeja de invitaciones — y quien abre una invitación casi
   * nunca viene de la bandeja: viene de ver el hueco en su semana. Aquí no había
   * nada que responder, así que había que salir y buscar el mismo evento otra vez.
   *
   * Se recarga después: el estado de la respuesta se pinta en el evento, y dejarlo
   * sin refrescar haría dudar de si se guardó.
   */
  function responderInvitacion(
    event: GridEvent,
    status: 'accepted' | 'tentative' | 'declined',
  ): void {
    setQuickView(null);
    if (offline || !event.uid) return;
    void fetch('/api/partstat', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ id: event.uid, status }),
    })
      .then((r) => {
        if (!r.ok) throw new Error(String(r.status));
        toast.notify({ tone: 'success', title: t('popover.responded') });
        reload();
      })
      .catch(() => toast.notify({ tone: 'danger', title: t('popover.respondFailed') }));
  }

  /**
   * ¿Estoy yo entre los asistentes de este evento?
   *
   * Se mira en el iCalendar crudo porque es lo único que trae el evento —no hay
   * lista de asistentes en el modelo—, y se comprueba MI dirección y no solo que
   * haya asistentes: al organizador no se le pregunta «¿vas a ir?» a su propia
   * reunión, y ofrecerle un botón que el servicio va a rechazar con un 403 es
   * exactamente lo que no se hace.
   */
  function meHanInvitado(event: GridEvent | null | undefined): boolean {
    if (!event?.ical || !user.email) return false;
    const mio = user.email.toLowerCase();
    return event.ical
      .split(/\r?\n/)
      .some((linea) => linea.startsWith('ATTENDEE') && linea.toLowerCase().includes(mio));
  }

  function editFromQuickView(event: GridEvent): void {
    setQuickView(null);
    openEdit(event);
  }

  /**
   * «Eliminar» de la vista rápida. Los eventos de solo lectura no llegan aquí (el
   * popover no muestra la acción). Una serie recurrente pregunta el alcance
   * (este / este y siguientes / todos) antes de borrar, para no perder la serie.
   */
  function deleteFromQuickView(event: GridEvent): void {
    setQuickView(null);
    if (offline || event.readOnly || !event.href) return;
    if (event.ical && parseRecurrence(event.ical)) {
      setDeleteScope(event);
      return;
    }
    void deleteEvent(event.href)
      .then(() => {
        toast.notify({ tone: 'success', title: t('toast.eventDeleted') });
        reload();
      })
      .catch(() => toast.notify({ tone: 'danger', title: t('cal.error.deleteEvent') }));
  }

  /** Aplica el alcance elegido para borrar una instancia recurrente. */
  function confirmDeleteScope(scope: RecurrenceScope): void {
    if (offline) return;
    const event = deleteScope;
    setDeleteScope(null);
    if (!event || !event.href) return;
    const cal = allCalendars.find((c) => c.url === event.calendarUrl);
    const target: RecurTarget = {
      calendarId: cal?.id ?? '',
      calendarUrl: event.calendarUrl ?? '',
      uid: event.uid ?? '',
      href: event.href,
      etag: event.etag,
      masterIcal: event.ical ?? '',
      recurrenceId: parseIcalDate(event.start) ?? new Date(),
      allDay: /^\d{8}$/.test(event.start),
    };
    void applyRecurringDelete(target, scope)
      .then(() => {
        toast.notify({ tone: 'success', title: t('toast.eventDeleted') });
        reload();
      })
      .catch(() => toast.notify({ tone: 'danger', title: t('cal.error.deleteEvent') }));
  }

  /** Refresca ahora una suscripción ICS y recarga sus eventos. */
  function refreshNow(c: Calendar): void {
    if (offline || !c.id) return;
    void refreshSubscription(c.id)
      .then(() => {
        toast.notify({ tone: 'success', title: t('cal.sub.updated') });
        loadCalendars();
        reload();
      })
      .catch(() => toast.notify({ tone: 'danger', title: t('cal.error.updateSub') }));
  }

  const sidebar = (
    <Sidebar
      header={
        <Button
          fullWidth
          variant="primary"
          leadingIcon={<Icon name="plus" />}
          disabled={offline}
          title={t('header.shortcut', { label: t('nav.createEvent'), key: 'C' })}
          onClick={() => openCreate(cursor)}
        >
          {t('nav.createEvent')}
        </Button>
      }
      // «Trae tus calendarios»: la migración solo tenía puerta en la consola de
      // administración, y quien estrena Sytue con la agenda vacía no va a buscarla ahí.
      // Se pinta solo si la organización tiene conectado un origen.
      footer={
        <div class="space-y-2">
          <LinkedServices
            service="calendar"
            basePath={import.meta.env.BASE_URL}
            labels={{
              create: t('nav.createEvent'),
              title: t('event.title'),
              text: t('event.description'),
              start: t('event.start'),
              end: t('event.end'),
              allDay: t('event.allDay'),
              location: t('event.location'),
              previousMonth: t('nav.prevMonth'),
              nextMonth: t('nav.nextMonth'),
            }}
          />
          <ImportData domains={['calendar']} onFinished={loadCalendars} />
        </div>
      }
    >
      <MiniMonth
        locale={locale}
        cursor={cursor}
        onPick={(day) => setCursor(day)}
        onShift={(delta) => setCursor((c) => addMonths(c, delta))}
      />
      <div class="mb-4">
        <div class="flex items-center justify-between pt-1 pr-1 pb-1.5 pl-2.5">
          <p class="sytue-overline text-sidebar-fg-muted">{t('nav.myCalendars')}</p>
          {/* Los mismos botones de cabecero que Mail y Notepads (`SidebarAction`);
              antes eran tres <button> a mano con estas mismas clases. */}
          <div class="flex items-center gap-0.5">
            {canManageResources && !offline ? (
              <SidebarAction
                label={t('resources.manage')}
                icon={<Icon name="building" size={16} />}
                onClick={() => setResourcesOpen(true)}
              />
            ) : null}
            <SidebarAction
              label={t('subscribe.byUrl')}
              disabled={offline}
              icon={<Icon name="globe" size={16} />}
              onClick={() => setSubscribeOpen(true)}
            />
            <SidebarAction
              label={t('cal.create')}
              disabled={offline}
              icon={<Icon name="plus" size={16} />}
              onClick={() => setCalModal({ open: true, editing: null })}
            />
          </div>
        </div>
        {loadingCals ? (
          // Esqueleto con la geometría de una fila, tintado para el fondo de
          // marca oscuro (bg-bg-subtle sería un destello claro sobre negro).
          <ul aria-hidden="true" class="flex flex-col gap-0.5">
            <li class="list-none">
              <Skeleton
                height="36px"
                style={{ backgroundColor: 'var(--sytue-sidebar-hover-bg)' }}
              />
            </li>
            <li class="list-none">
              <Skeleton
                height="36px"
                style={{ backgroundColor: 'var(--sytue-sidebar-hover-bg)' }}
              />
            </li>
          </ul>
        ) : calendars.length === 0 ? (
          <p class="text-sidebar-fg-muted px-2.5 py-2 text-sm">{t('nav.noCalendars')}</p>
        ) : (
          <ul class="flex flex-col gap-0.5">
            {calendars.map((c) => (
              <li
                key={c.url}
                class="group hover:bg-sidebar-hover flex items-center gap-1 rounded-md pr-1 transition-colors duration-150"
              >
                <label class="text-sidebar-fg flex min-w-0 flex-1 cursor-pointer items-center gap-2.5 py-2 pl-2.5 text-base font-medium">
                  <Checkbox
                    ringClass="sytue-focus-ring-chrome"
                    checked={visible.has(c.url)}
                    onChange={() => toggleVisible(c.url)}
                    aria-label={t('nav.showCalendar', { name: c.displayName })}
                    // Aro del color del texto del menú: la casilla lleva el color del
                    // calendario, y uno del tono del carril desaparecía sobre él.
                    class="ring-sidebar-fg h-3.5 w-3.5 shrink-0 cursor-pointer rounded-[2px] ring-[1.5px]"
                    // El checkbox lleva el color del calendario (accent-color
                    // nativo); esquema claro para que el estado sin marcar se
                    // vea sobre el fondo de marca oscuro.
                    style={{
                      accentColor: c.color ?? 'var(--sytue-primary-500)',
                      colorScheme: 'light',
                    }}
                  />
                  <span class="min-w-0 flex-1 truncate" title={c.displayName}>
                    {c.displayName}
                  </span>
                  {c.readOnly ? (
                    <Icon
                      name="globe"
                      size={14}
                      class="text-sidebar-fg-muted shrink-0"
                      label={t('cal.subscribedReadOnly')}
                    />
                  ) : null}
                </label>
                {!offline ? (
                  <Dropdown
                    label={t('nav.calendarOptions', { name: c.displayName })}
                    trigger={(props) => (
                      <button
                        type="button"
                        {...props}
                        // Abre un menú DENTRO de la barra, así que en móvil no
                        // debe cerrar el cajón: se llevaría por delante el menú
                        // que se acaba de abrir. Sus opciones sí lo cierran, y
                        // eso se conserva: el `Dropdown` las monta como hermanas
                        // del disparador, no dentro, así que no heredan la marca.
                        data-mantiene-nav
                        aria-label={t('nav.calendarOptions', { name: c.displayName })}
                        title={t('cal.options')}
                        class={cx(
                          'text-sidebar-fg-muted rounded-control grid h-6 w-6 shrink-0 place-items-center',
                          'transition-[color,background-color,opacity] duration-150',
                          'hover:bg-sidebar-hover hover:text-sidebar-fg',
                          'sytue-focus-ring-chrome',
                          // Visible al pasar/enfocar la fila o con el menu abierto; en
                          // puntero grueso, siempre (sin hover era inalcanzable).
                          'opacity-0 group-focus-within:opacity-100 group-hover:opacity-100 pointer-coarse:opacity-100',
                          props['aria-expanded'] && 'opacity-100',
                        )}
                      >
                        <KebabGlyph />
                      </button>
                    )}
                  >
                    {c.readOnly ? (
                      <>
                        <MenuItem onClick={() => refreshNow(c)}>{t('cal.refresh')}</MenuItem>
                        <MenuSeparator />
                      </>
                    ) : null}
                    <MenuItem onClick={() => setCalModal({ open: true, editing: c })}>
                      {t('nav.rename')}
                    </MenuItem>
                    <MenuItem onClick={() => setCalModal({ open: true, editing: c })}>
                      {t('nav.changeColour')}
                    </MenuItem>
                    {/* Compartir con ACL: solo calendarios propios editables (no las
                      suscripciones ICS de solo lectura). */}
                    {!c.readOnly ? (
                      <MenuItem onClick={() => setSharingCal(c)}>{t('cal.share')}</MenuItem>
                    ) : null}
                    {/* El calendario entero como .ics: para llevárselo a otra app o dárselo
                      a quien no está en la organización. Vale también para una suscripción
                      de solo lectura — es SU copia de esos eventos, no la del origen. Como
                      enlace (`href`) y no `onClick`: es una descarga de verdad (el servicio
                      manda Content-Disposition: attachment) y el navegador ya sabe hacerla;
                      no navega fuera, así que no hace falta `target="_blank"`. */}
                    {c.id ? (
                      <MenuItem href={calendarIcsUrl(c.id)}>{t('cal.downloadIcs')}</MenuItem>
                    ) : null}
                    {!c.isDefault ? (
                      <>
                        <MenuSeparator />
                        <MenuItem danger onClick={() => setDeleting(c)}>
                          {c.readOnly ? t('cal.sub.delete') : t('common.delete')}
                        </MenuItem>
                      </>
                    ) : null}
                  </Dropdown>
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </div>

      {/* «Compartido conmigo»: calendarios que otros han compartido con el usuario
          (color del dueño + badge de permiso con TEXTO). Los reader no son
          editables (readOnly bloquea arrastrar/editar, como una suscripción); los
          writer sí. Se pintan por el mismo camino CalDAV que los propios. */}
      {sharedCalendars.length > 0 ? (
        <div class="mb-4">
          <div class="flex items-center pt-1 pr-1 pb-1.5 pl-2.5">
            <p class="sytue-overline text-sidebar-fg-muted">{t('cal.sharedWithMe')}</p>
          </div>
          <ul class="flex flex-col gap-0.5">
            {sharedCalendars.map((c) => (
              <li
                key={c.url}
                class="hover:bg-sidebar-hover flex items-center gap-1 rounded-md pr-2 transition-colors duration-150"
              >
                <label class="text-sidebar-fg flex min-w-0 flex-1 cursor-pointer items-center gap-2.5 py-2 pl-2.5 text-base font-medium">
                  <Checkbox
                    ringClass="sytue-focus-ring-chrome"
                    checked={visible.has(c.url)}
                    onChange={() => toggleVisible(c.url)}
                    aria-label={t('nav.showCalendar', { name: c.displayName })}
                    // El mismo aro que en «Mis calendarios».
                    class="ring-sidebar-fg h-3.5 w-3.5 shrink-0 cursor-pointer rounded-[2px] ring-[1.5px]"
                    style={{
                      accentColor: c.color ?? 'var(--sytue-primary-500)',
                      colorScheme: 'light',
                    }}
                  />
                  <span
                    class="min-w-0 flex-1 truncate"
                    title={c.sharedBy ? t('nav.sharedBy', { name: c.sharedBy }) : c.displayName}
                  >
                    {c.displayName}
                  </span>
                </label>
                <span
                  class={cx(
                    'text-2xs shrink-0 rounded-full px-1.5 py-0.5 font-semibold',
                    c.permission === 'writer'
                      ? 'bg-sidebar-hover text-sidebar-fg'
                      : 'border-sidebar-fg-muted/40 text-sidebar-fg-muted border',
                  )}
                  title={t(
                    c.permission === 'writer' ? 'nav.permWriterTitle' : 'nav.permReaderTitle',
                  )}
                >
                  {t(c.permission === 'writer' ? 'nav.permWriter' : 'nav.permReader')}
                </span>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </Sidebar>
  );

  return (
    <SytueShell
      currentApp="calendar"
      user={user}
      hubBaseUrl={hubBaseUrl}
      accessToken={getAccessToken}
      // Sin esto la topbar (buscador global, cuenta, notificaciones, aviso de
      // «sin conexión») se queda en el idioma por defecto mientras el cuerpo va
      // en el del usuario: media pantalla en inglés y media en castellano.
      locale={locale}
    >
      {/* La topbar global ya la pinta la Sytue Shell: el AppShell ocupa el resto del viewport. */}
      <AppShell topbar={null} class="h-[calc(100dvh-var(--spacing-topbar))]!" sidebar={sidebar}>
        <div class="flex h-full flex-col">
          {/* En móvil la cabecera NO envuelve en dos filas: es UNA fila que se
              desplaza de lado (ver scrollX.ts en @sytue/ui). El salto de línea
              robaba media pantalla al calendario, que es lo caro en un teléfono. */}
          <header
            data-scroll-x
            ref={cabecera.ref}
            class={cx(
              'bg-surface text-text border-border flex flex-wrap items-center gap-x-3 gap-y-2 border-b px-4 py-2.5',
              FILA_UNICA_EN_MOVIL,
              cabecera.queda && MASCARA_SCROLL_X,
            )}
          >
            <div class="flex items-center gap-1">
              {/* El atajo va en el `title`, no en el nombre accesible: un lector de
                  pantalla no navega por teclas de una sola letra y leer «(T)» tras
                  cada botón sería ruido. */}
              <button
                type="button"
                title={t('header.shortcut', { label: t('header.today'), key: 'T' })}
                onClick={() => setCursor(new Date())}
                class={cx(
                  'border-border-strong text-text rounded-control inline-flex h-8 items-center border px-3 text-sm font-semibold',
                  'hover:bg-surface-sunken active:bg-surface-sunken transition-colors duration-150',
                  HEADER_FOCUS,
                )}
              >
                {t('header.today')}
              </button>
              <button
                type="button"
                aria-label={t('nav.prev')}
                title={t('header.shortcut', { label: t('nav.prev'), key: 'P' })}
                onClick={() => navigate(-1)}
                class={cx(HEADER_ICON_BTN, HEADER_FOCUS)}
              >
                <Icon name="chevronLeft" />
              </button>
              <button
                type="button"
                aria-label={t('nav.next')}
                title={t('header.shortcut', { label: t('nav.next'), key: 'N' })}
                onClick={() => navigate(1)}
                class={cx(HEADER_ICON_BTN, HEADER_FOCUS)}
              >
                <Icon name="chevronRight" />
              </button>
            </div>
            <h1 class="sytue-h3 text-text min-w-0 truncate" title={titleFor(mode, cursor, locale)}>
              {titleFor(mode, cursor, locale)}
            </h1>
            <button
              type="button"
              aria-label={t('search.events')}
              title={t('header.shortcut', { label: t('search.events'), key: '/' })}
              onClick={() => setSearchOpen(true)}
              class={cx('ml-auto', HEADER_ICON_BTN, HEADER_FOCUS)}
            >
              <Icon name="search" size={18} />
            </button>
            <ReminderCenter
              t={t}
              locale={locale}
              onDeepLink={(d) => {
                setCursor(d);
                setMode('day');
              }}
            />
            <button
              type="button"
              onClick={() => setInvitesOpen(true)}
              class={cx(
                'border-border-strong text-text rounded-control inline-flex h-8 items-center gap-1.5 border px-3 text-sm font-semibold',
                'hover:bg-surface-sunken active:bg-surface-sunken transition-colors duration-150',
                HEADER_FOCUS,
              )}
            >
              {/* `users`, no `bell`: la campana ya es el centro de recordatorios,
                  que vive justo a la izquierda — dos campanas seguidas con
                  significados distintos no distinguen nada. */}
              <Icon name="users" size={16} />
              {t('header.invitations')}
              {pendingInvites > 0 ? (
                <Badge
                  count
                  class="ml-1"
                  aria-label={t('header.pending', { count: pendingInvites })}
                >
                  {pendingInvites}
                </Badge>
              ) : null}
            </button>
            <div
              class="bg-bg-subtle inline-flex items-center gap-0.5 rounded-md p-0.5"
              role="group"
              aria-label={t('view.label')}
            >
              {(['month', 'week', 'day', 'agenda'] as ViewMode[]).map((m) => (
                <button
                  key={m}
                  type="button"
                  aria-pressed={mode === m}
                  title={t('header.shortcut', {
                    label: t(`view.${m}` as const),
                    key: VIEW_KEY[m],
                  })}
                  onClick={() => setMode(m)}
                  class={cx(
                    'h-7 rounded-sm px-2.5 text-sm font-semibold transition-colors duration-150',
                    HEADER_FOCUS,
                    mode === m
                      ? // `primary-text` y no `primary`: el relleno no se reasigna en
                        // oscuro, así que sobre `surface` oscuro daba 2,96:1. El rol de
                        // texto sí invierte (7,83:1 en claro, 8,03:1 en oscuro).
                        'bg-surface text-primary-text shadow-[var(--shadow-xs)]'
                      : // El grupo es una píldora CLARA sobre el cromo grafito: al pasar
                        // por encima el texto se oscurece; `text-text` (blanco) aquí
                        // dejaba la etiqueta invisible sobre `bg-surface`.
                        'text-text-muted hover:bg-surface hover:text-text',
                  )}
                >
                  {m === 'month'
                    ? t('view.month')
                    : m === 'week'
                      ? t('view.week')
                      : m === 'day'
                        ? t('view.day')
                        : t('view.agenda')}
                </button>
              ))}
            </div>
          </header>
          {/* La franja de solo lectura de cada calendario compartido de lector que está a
              la vista, con su «Pedir acceso de edición»; y, para el dueño que llega desde
              el aviso, la de quién se lo pide. Al conceder, el aviso del Hub la quita. */}
          {sharedCalendars
            .filter(
              (c) => c.id && c.permission === 'reader' && !c.subscription && visible.has(c.url),
            )
            .map((c) => (
              <FranjaPeticion
                key={c.id}
                endpoint={`/api/edit-requests?calendar=${encodeURIComponent(c.id ?? '')}`}
                soloLectura={!offline}
                puedeConceder={false}
                pedida={c.editRequested ?? false}
                mensaje={translate(locale, 'readOnly.sharedWithYou', { name: c.displayName })}
                locale={locale}
                concedido="calendar.access.granted"
                recurso={c.id ?? ''}
                alConceder={loadCalendars}
              />
            ))}
          {calendarioDelAviso && calendars.some((c) => c.id === calendarioDelAviso) ? (
            <FranjaPeticion
              endpoint={`/api/edit-requests?calendar=${encodeURIComponent(calendarioDelAviso)}`}
              soloLectura={false}
              puedeConceder
              mensaje=""
              locale={locale}
            />
          ) : null}

          <div class="min-h-0 flex-1 overflow-auto">
            {loadingCals || (loadingEvents && shownEvents.length === 0) ? (
              <GridSkeleton locale={locale} mode={mode} />
            ) : error ? (
              <div class="flex h-full items-center justify-center p-6">
                <EmptyState
                  title={t('load.error')}
                  description={error}
                  illustration={<Illustration name="errorRetry" />}
                  action={
                    <Button
                      variant="secondary"
                      size="sm"
                      onClick={() => (calendars.length === 0 ? loadCalendars() : reload())}
                    >
                      {t('common.retry')}
                    </Button>
                  }
                />
              </div>
            ) : (
              // Refresco con datos previos (cambiar de mes/vista): rejilla atenuada
              // en vez de esqueleto — sin parpadeo de layout en cada navegación.
              <div
                class={cx(
                  'h-full transition-opacity duration-150',
                  loadingEvents && 'pointer-events-none opacity-60',
                )}
                aria-busy={loadingEvents || undefined}
              >
                <CalendarGrid
                  locale={locale}
                  mode={mode}
                  cursor={cursor}
                  events={shownEvents}
                  onSelectEvent={openQuickView}
                  onCreateAt={openCreate}
                  onCreateRange={openCreateRange}
                  onMoveResize={moveResizeEvent}
                />
              </div>
            )}
          </div>
        </div>
      </AppShell>
      {/* Crear evento al alcance del pulgar: en móvil el menú lateral es un cajón cerrado. */}
      {offline ? null : (
        <MobileFab
          label={t('nav.createEvent')}
          icon={<Icon name="plus" size={24} />}
          onClick={() => openCreate(cursor)}
        />
      )}

      <EventModal
        t={t}
        locale={locale}
        open={modal.open}
        calendars={eventTargets}
        editing={modal.editing}
        onClose={() => setModal({ open: false, editing: null })}
        onSaved={reload}
      />

      <CalendarModal
        t={t}
        open={calModal.open}
        editing={calModal.editing}
        onClose={() => setCalModal({ open: false, editing: null })}
        onSaved={loadCalendars}
      />

      <SubscribeModal
        t={t}
        open={subscribeOpen}
        onClose={() => setSubscribeOpen(false)}
        onSaved={loadCalendars}
      />

      <ShareCalendarModal t={t} calendar={sharingCal} onClose={() => setSharingCal(null)} />

      <ResourcesModal t={t} open={resourcesOpen} onClose={() => setResourcesOpen(false)} />

      <SearchPanel
        t={t}
        locale={locale}
        open={searchOpen}
        onClose={() => setSearchOpen(false)}
        calendars={calendars}
        onOpenResult={(start) => {
          // Navega al día del evento: la rejilla lo muestra en contexto (su
          // popover permite ver detalles/editar/eliminar).
          setCursor(start);
          setMode('day');
        }}
      />

      <DeleteCalendarModal
        t={t}
        calendar={deleting}
        onClose={() => setDeleting(null)}
        onDeleted={loadCalendars}
      />

      <InvitationsModal
        t={t}
        locale={locale}
        open={invitesOpen}
        onClose={() => setInvitesOpen(false)}
        onCountChange={setPendingInvites}
      />

      <ScopeDialog
        t={t}
        open={dragScope !== null}
        op="edit"
        onConfirm={confirmDragScope}
        onClose={cancelDragScope}
      />

      <ScopeDialog
        t={t}
        open={deleteScope !== null}
        op="delete"
        onConfirm={confirmDeleteScope}
        onClose={() => setDeleteScope(null)}
      />

      <EventPopover
        t={t}
        locale={locale}
        event={quickView?.event ?? null}
        anchor={quickView?.anchor ?? null}
        accentColor={DEFAULT_ACCENT}
        onClose={() => setQuickView(null)}
        onEdit={editFromQuickView}
        onDelete={deleteFromQuickView}
        onDuplicate={duplicateFromQuickView}
        // Solo donde HAY invitación que contestar: si el evento no tiene
        // asistentes, ofrecer «¿vas a ir?» sobre una cita propia no significa nada.
        {...(meHanInvitado(quickView?.event) ? { onResponder: responderInvitacion } : {})}
      />
    </SytueShell>
  );
}

/**
 * MiniMonth — mini-calendario mensual de la barra lateral (sobre el fondo de
 * marca oscuro). Navega el cursor: elegir un dia lo fija, las flechas cambian de
 * mes. "Hoy" en primario, el dia seleccionado con realce sutil. Reutiliza las
 * utilidades de `dates.js`; no mantiene estado propio.
 */
function MiniMonth({
  locale,
  cursor,
  onPick,
  onShift,
}: {
  locale: Locale;
  cursor: Date;
  onPick: (day: Date) => void;
  onShift: (delta: number) => void;
}): JSX.Element {
  const t = useT(locale);
  const days = monthGrid(cursor);
  // El SSR calcula «hoy» con el reloj del servidor (UTC): si el día local del
  // navegador difiere (00:00–02:00 CEST), la hidratación de Preact no repara
  // clases/atributos y el resaltado quedaría pegado al día equivocado. El
  // cambio de `key` tras montar remonta la rejilla ya con la fecha local.
  const [hydrated, setHydrated] = useState(false);
  useEffect(() => setHydrated(true), []);
  return (
    <div class="mb-4 px-1">
      <div class="mb-1.5 flex items-center justify-between">
        <span class="text-sidebar-fg pl-1.5 text-sm font-semibold">
          {titleFor('month', cursor, locale)}
        </span>
        <div class="flex items-center gap-0.5">
          {/* `SidebarAction` y no dos copias de sus clases: las copias se quedaron en
              `h-6`, que con la raíz a 14 son 21 px, cuando la pieza subió a 24. */}
          <SidebarAction
            label={t('nav.prevMonth')}
            icon={<Icon name="chevronLeft" size={16} />}
            onClick={() => onShift(-1)}
          />
          <SidebarAction
            label={t('nav.nextMonth')}
            icon={<Icon name="chevronRight" size={16} />}
            onClick={() => onShift(1)}
          />
        </div>
      </div>
      <div class="grid grid-cols-7" aria-hidden="true">
        {weekdayNarrowLabels(locale).map((w, i) => (
          <span
            key={i}
            class="text-sidebar-fg-muted text-2xs grid h-6 place-items-center font-semibold uppercase"
          >
            {w}
          </span>
        ))}
      </div>
      <div class="grid grid-cols-7" key={hydrated ? 'local' : 'ssr'}>
        {days.map((day) => {
          const inMonth = day.getMonth() === cursor.getMonth();
          const today = isToday(day);
          const selected = isSameDay(day, cursor);
          return (
            <button
              key={day.toISOString()}
              type="button"
              aria-label={dayMonthYearLabel(locale, day)}
              aria-current={today ? 'date' : undefined}
              onClick={() => onPick(day)}
              class={cx(
                'rounded-control grid h-7 place-items-center text-xs tabular-nums transition-colors',
                'sytue-focus-ring-chrome focus-visible:outline-none',
                today
                  ? 'bg-primary font-semibold text-[var(--sytue-on-primary)]'
                  : selected
                    ? 'bg-sidebar-hover text-sidebar-fg font-medium'
                    : inMonth
                      ? 'text-sidebar-fg hover:bg-sidebar-hover'
                      : 'text-sidebar-fg-muted hover:bg-sidebar-hover',
              )}
            >
              {day.getDate()}
            </button>
          );
        })}
      </div>
    </div>
  );
}

/**
 * GridSkeleton — carga de la rejilla replicando su geometría (cabecera de días +
 * celdas/columnas), sin salto de layout. Un solo role="status" anuncia la carga;
 * los bloques Skeleton son decorativos (aria-hidden).
 */
function GridSkeleton({ mode, locale }: { mode: ViewMode; locale: Locale }): JSX.Element {
  const t = useT(locale);
  if (mode === 'month') {
    return (
      <div
        role="status"
        aria-busy="true"
        aria-label={t('loading.calendar')}
        class="bg-surface flex h-full flex-col"
      >
        <div class="border-border grid grid-cols-7 border-b">
          {Array.from({ length: 7 }, (_, i) => (
            <div key={i} class="px-2.5 py-2.5">
              <Skeleton width="2rem" height="0.75rem" />
            </div>
          ))}
        </div>
        <div class="grid flex-1 grid-cols-7 grid-rows-6">
          {Array.from({ length: 42 }, (_, i) => (
            <div key={i} class="border-border border-r border-b p-1.5">
              <Skeleton width="1.5rem" height="1.5rem" circle />
            </div>
          ))}
        </div>
      </div>
    );
  }
  const cols = mode === 'week' ? 7 : 1;
  return (
    <div
      role="status"
      aria-busy="true"
      aria-label={t('loading.calendar')}
      class={cx('bg-surface grid h-full', cols === 7 ? 'grid-cols-7' : 'grid-cols-1')}
    >
      {Array.from({ length: cols }, (_, i) => (
        <div key={i} class="border-border flex flex-col border-r last:border-r-0">
          <div class="border-border border-b px-3 py-2.5">
            <Skeleton width="3rem" height="0.875rem" />
          </div>
          <div class="flex flex-col gap-1.5 p-2">
            <Skeleton height="1.75rem" />
            <Skeleton height="1.75rem" width="80%" />
          </div>
        </div>
      ))}
    </div>
  );
}
