/**
 * CalendarGrid — render de las vistas mes/semana/dia con el lenguaje visual
 * Sytue: rejilla densa sobre superficie blanca, esquinas suaves y "hoy" resaltado
 * con el primario. Los eventos se muestran como pildoras con el color de SU
 * calendario de origen (superposicion multi-calendario); al hacer click se
 * editan, y el click en una celda/columna vacia crea. Solo Tailwind/tokens de
 * @sytue/ui (sin CSS ad-hoc).
 */
import type { JSX } from 'preact';
import { useT } from '../i18n/index.js';
import { useMemo, useState } from 'preact/hooks';
import { Button, Icon, cx } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { CalendarEvent } from '../client/calendarApi.js';
import {
  type ViewMode,
  weekdayLabels,
  monthGrid,
  weekGrid,
  eventInstant,
  parseTzid,
  startOfDay,
  isToday,
  timeLabel,
  dayMonthLabel,
  monthShortLabel,
} from './dates.js';
import { DiaDeSemana } from './DiaDeSemana.js';
import {
  barSpan,
  assignRows,
  sliceWeeks,
  limitWeekRows,
  rowCount,
  barTextColor,
  type Bar,
  type Segment,
} from './multiDayLayout.js';
import { TimeGrid } from './TimeGrid.js';
import { plegar } from '@sytue/i18n';

// Geometría de las barras multi-día en la vista MES.
const BAR_H = 22; // alto de una fila de barras, con separación
const BAR_INNER_H = 18; // alto pintado de la barra
// Alto reservado por el número de día: el relleno de la celda (6) más el círculo
// del día (24). Va en PÍXELES y las clases que lo producen también —`p-[6px]` y
// `h-[24px]`— porque este número se usa en un `style` absoluto: las barras de
// evento se colocan a `top: 30px`.
//
// Con `p-1.5` y `h-6`, que son REM, las dos capas solo coinciden si la raíz del
// navegador está en 16. Con la base en 14 el contenido real mide 26,25 y quedaba
// un hueco; con la base en 18 mide 33,75 y la primera barra de evento se pintaba
// ENCIMA del número del día. Es el mismo error de unidad que la rejilla de Sheets.
//
// Si se cambia este número, hay que cambiar sus dos clases a la vez.
const DATE_ROW_H = 30;
const MAX_BAR_ROWS = 3; // filas de barras visibles por semana; el resto va a «+N más»
const MAX_LANES = 4; // presupuesto de filas por celda (barras + chips)

// Color de reserva cuando un calendario no trae el suyo. HEX RESUELTO (no var())
// para que barTextColor mida el contraste REAL sobre el relleno: primary-600
// (#1a6fbb) da blanco 5,2:1 AA en las barras/franjas CON TEXTO; un var() no es
// parseable y forzaba blanco a ciegas aunque el relleno no llegara a AA.
export const DEFAULT_ACCENT = '#1a6fbb';

/** Evento a pintar, etiquetado con su calendario de origen (color/URL). */
export interface GridEvent extends CalendarEvent {
  /** Color del calendario al que pertenece el evento. */
  color?: string | undefined;
  /** URL de la coleccion CalDAV de origen (para editar/mover). */
  calendarUrl?: string | undefined;
  /** El evento viene de un calendario de SOLO LECTURA (suscripción ICS): no se
   * arrastra/redimensiona ni se edita, solo se ve. */
  readOnly?: boolean | undefined;
}

interface Props {
  /** Idioma ya negociado en el servidor: fechas y horas salen en su convención. */
  locale: Locale;
  mode: ViewMode;
  cursor: Date;
  events: GridEvent[];
  /** Color de reserva si un evento no trae el de su calendario. */
  accentColor?: string | undefined;
  /** Abre la vista rápida del evento anclada a su rectángulo (viewport). */
  onSelectEvent: (event: GridEvent, anchor: DOMRect) => void;
  onCreateAt: (day: Date) => void;
  /** Crear con el rango pintado en la rejilla horaria (semana/día). */
  onCreateRange: (start: Date, end: Date) => void;
  /** Persistir un mover/redimensionar de la rejilla horaria (semana/día). */
  onMoveResize: (event: GridEvent, start: Date, end: Date) => Promise<void>;
}

interface Positioned {
  event: GridEvent;
  date: Date;
}

function position(events: GridEvent[]): Positioned[] {
  return events
    .map((event) => {
      // Resuelve a la zona de visualización (honra el TZID del evento), no a la
      // hora naíf del SO: un evento con TZID se sitúa en su instante real.
      const date = eventInstant(event.start, parseTzid(event.ical));
      return date ? { event, date } : null;
    })
    .filter((p): p is Positioned => p !== null);
}

interface MonthChip {
  event: GridEvent;
  /** Instante de inicio resuelto (honra TZID); ordena los chips del día. */
  date: Date;
}

interface MonthLayout {
  days: Date[];
  /** Segmentos de barra por semana (6): todo-el-día y multi-día, apilados y partidos. */
  weeks: Segment<GridEvent>[][];
  /** Chips (eventos de un solo día con hora) por índice de día de la rejilla. */
  chipsByDayIdx: MonthChip[][];
}

/**
 * Reparte los eventos del mes en BARRAS (todo-el-día o multi-día, apiladas en
 * filas sin solape y partidas por semana) y CHIPS (eventos de un solo día con
 * hora, en su celda). Resuelve el instante de cada evento (honra TZID) una vez;
 * el resto es geometría pura de `multiDayLayout`. El resultado se memoiza.
 */
function buildMonthLayout(events: GridEvent[], days: Date[]): MonthLayout {
  const bars: Bar<GridEvent>[] = [];
  const buckets: MonthChip[][] = Array.from({ length: days.length }, () => []);
  for (const event of events) {
    const tz = parseTzid(event.ical);
    const start = eventInstant(event.start, tz);
    if (!start) continue;
    const end = event.end ? eventInstant(event.end, tz) : null;
    const sp = barSpan(days, start, end);
    if (!sp) continue; // no solapa el mes visible
    const allDay = /^\d{8}$/.test(event.start);
    // Barra si es de todo el día o abarca >1 jornada; si no, chip en su día.
    if (allDay || sp.end > sp.start) bars.push({ item: event, start: sp.start, end: sp.end });
    else buckets[sp.start]!.push({ event, date: start });
  }
  for (const b of buckets) b.sort((a, z) => a.date.getTime() - z.date.getTime());
  const weeks = sliceWeeks(assignRows(bars), Math.ceil(days.length / 7), 7);
  return { days, weeks, chipsByDayIdx: buckets };
}

/**
 * Barra multi-día del mes: rectángulo con el color del evento que cruza las
 * celdas. Texto (título) solo en el primer segmento visible, con color de
 * contraste AA calculado sobre el relleno. Bordes rectos en los cortes de
 * continuación (salto de semana). Clic/Intro abre la vista rápida.
 */
function MonthBar({
  locale,
  seg,
  accent,
  onSelect,
}: {
  locale: Locale;
  seg: Segment<GridEvent>;
  accent: string;
  onSelect: (event: GridEvent, anchor: DOMRect) => void;
}): JSX.Element {
  const t = useT(locale);
  const event = seg.item;
  const fill = event.color ?? accent;
  const inL = seg.continuesLeft ? 0 : 3; // sin sangría en el corte: se pega al borde
  const inR = seg.continuesRight ? 0 : 3;
  return (
    <button
      type="button"
      title={event.summary}
      aria-label={`${event.summary || t('event.untitled')}${seg.continuesLeft ? t('cal.fromEarlier') : ''}`}
      onClick={(e) => {
        e.stopPropagation();
        onSelect(event, e.currentTarget.getBoundingClientRect());
      }}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          e.stopPropagation();
          onSelect(event, (e.currentTarget as HTMLElement).getBoundingClientRect());
        }
      }}
      class={cx(
        'pointer-events-auto absolute flex items-center overflow-hidden px-1.5 text-left',
        'transition-[filter] duration-150 hover:brightness-95',
        'sytue-focus-ring focus-visible:z-10',
        seg.continuesLeft ? 'rounded-l-none' : 'rounded-l-[3px]',
        seg.continuesRight ? 'rounded-r-none' : 'rounded-r-[3px]',
      )}
      style={{
        top: `${seg.row * BAR_H}px`,
        left: `calc(${(seg.col / 7) * 100}% + ${inL}px)`,
        width: `calc(${(seg.span / 7) * 100}% - ${inL + inR}px)`,
        height: `${BAR_INNER_H}px`,
        backgroundColor: fill,
        color: barTextColor(fill),
      }}
    >
      {seg.continuesLeft ? (
        <span class="sr-only">{event.summary}</span>
      ) : (
        <span class="text-2xs truncate leading-none font-medium">
          {event.summary || t('event.untitled')}
        </span>
      )}
    </button>
  );
}

export function CalendarGrid({
  locale,
  mode,
  cursor,
  events,
  accentColor,
  onSelectEvent,
  onCreateAt,
  onCreateRange,
  onMoveResize,
}: Props): JSX.Element {
  const t = useT(locale);
  const items = position(events);
  const color = accentColor ?? DEFAULT_ACCENT;

  // Layout de barras multi-día del MES: se recalcula solo al cambiar los eventos
  // o el cursor (memoizado), nunca en cada render — clave para la escala.
  const monthLayout = useMemo(
    () => (mode === 'month' ? buildMonthLayout(events, monthGrid(cursor)) : null),
    [mode, events, cursor],
  );

  if (mode === 'agenda') {
    return (
      <AgendaView
        locale={locale}
        items={items}
        cursor={cursor}
        color={color}
        onSelectEvent={onSelectEvent}
        onCreateAt={onCreateAt}
      />
    );
  }

  if (mode === 'month' && monthLayout) {
    const { days, weeks, chipsByDayIdx } = monthLayout;
    return (
      <div class="bg-surface flex h-full flex-col">
        <div class="border-border grid grid-cols-7 border-b">
          {/* Cada celda es `@container`: en una columna estrecha —un mes en un móvil mide
              46 px por día— va la inicial en vez de la abreviatura recortada. */}
          {weekdayLabels(locale).map((w, i) => (
            <div key={w} class="@container min-w-0">
              <div class="text-text-subtle sytue-overline truncate px-2.5 py-2">
                <DiaDeSemana locale={locale} dia={i} />
              </div>
            </div>
          ))}
        </div>
        <div class="flex min-h-0 flex-1 flex-col">
          {weeks.map((weekSegs, w) => {
            // Filas de barras visibles de la semana (tope MAX_BAR_ROWS); el resto
            // cuenta como «+N más» por día. Los chips comparten el presupuesto.
            const { visible, hiddenByCol } = limitWeekRows(weekSegs, MAX_BAR_ROWS);
            const visibleRows = Math.min(rowCount(weekSegs), MAX_BAR_ROWS);
            const barsAreaH = visibleRows * BAR_H;
            const chipCap = Math.max(1, MAX_LANES - visibleRows);
            return (
              <div key={w} class="relative grid min-h-0 flex-1 grid-cols-7">
                {Array.from({ length: 7 }, (_, c) => {
                  const gi = w * 7 + c;
                  const day = days[gi]!;
                  const inMonth = day.getMonth() === cursor.getMonth();
                  const dayChips = chipsByDayIdx[gi]!;
                  const shown = dayChips.slice(0, chipCap);
                  const more = hiddenByCol[c]! + (dayChips.length - shown.length);
                  return (
                    <button
                      key={day.toISOString()}
                      type="button"
                      aria-label={t('nav.createEventOn', { date: dayMonthLabel(locale, day) })}
                      onClick={() => onCreateAt(day)}
                      class={cx(
                        'border-border flex min-h-0 flex-col overflow-hidden border-r border-b p-[6px] text-left',
                        'hover:bg-bg-subtle focus-visible:bg-bg-subtle transition-colors focus-visible:z-10 focus-visible:outline-none',
                        !inMonth && 'bg-bg-subtle/40',
                      )}
                    >
                      <span
                        class={cx(
                          'inline-flex h-[24px] w-[24px] items-center justify-center rounded-full text-sm tabular-nums',
                          isToday(day)
                            ? 'bg-primary font-semibold text-[var(--sytue-on-primary)]'
                            : inMonth
                              ? 'text-text-muted'
                              : 'text-text-subtle',
                        )}
                      >
                        {day.getDate()}
                      </span>
                      {/* Reserva para el carril de barras (overlay), que se pinta encima. */}
                      {barsAreaH > 0 ? (
                        <span aria-hidden="true" style={{ height: `${barsAreaH}px` }} />
                      ) : null}
                      <span class="mt-0.5 flex min-w-0 flex-col gap-0.5 overflow-hidden">
                        {shown.map(({ event, date }) => (
                          <EventChip
                            locale={locale}
                            key={event.href}
                            event={event}
                            date={date}
                            color={color}
                            onClick={onSelectEvent}
                          />
                        ))}
                        {more > 0 ? (
                          <span
                            class="text-text-muted px-1.5 text-xs font-medium"
                            title={dayChips
                              .slice(chipCap)
                              .map(({ event }) => event.summary)
                              .join('\n')}
                          >
                            {t('grid.more', { count: more })}
                          </span>
                        ) : null}
                      </span>
                    </button>
                  );
                })}
                {/* Overlay de barras de la semana: cruzan las celdas sin recorte y
                    reciben el clic (pointer-events-auto); el resto pasa a la celda. */}
                <div
                  class="pointer-events-none absolute inset-x-0"
                  style={{ top: `${DATE_ROW_H}px`, height: `${barsAreaH}px` }}
                >
                  {visible.map((seg) => (
                    <MonthBar
                      key={seg.item.href}
                      locale={locale}
                      seg={seg}
                      accent={color}
                      onSelect={onSelectEvent}
                    />
                  ))}
                </div>
              </div>
            );
          })}
        </div>
      </div>
    );
  }

  // Vistas semana/día: rejilla horaria real (eje 0–24 h, bloques posicionados por
  // hora y dimensionados por duración, columnas lado a lado en los solapes, franja
  // de todo-el-día, línea de hora actual y arrastre para crear/mover/redimensionar).
  const days = mode === 'week' ? weekGrid(cursor) : [cursor];
  return (
    <TimeGrid
      locale={locale}
      days={days}
      events={events}
      accentColor={color}
      onSelectEvent={onSelectEvent}
      onCreateRange={onCreateRange}
      onMoveResize={onMoveResize}
    />
  );
}

// --- Vista agenda / lista ----------------------------------------------------

/** Normaliza texto para buscar: sin acentos y en minúsculas (es_ES). */
function normalizeText(s: string): string {
  return plegar(s);
}

/** Clave de agrupación por día local (independiente de la hora). */
function dayKey(d: Date): string {
  return `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`;
}

/**
 * AgendaView — vista lista: los eventos de la ventana (cursor..+N días) agrupados
 * por día en orden cronológico, solo los días con eventos (densidad real, sin
 * rejilla vacía). Buscador por texto (resumen/lugar/descripción, insensible a
 * mayúsculas y acentos) sobre los eventos ya cargados; el índice trigram en el
 * backend (Go) queda como follow-up de escala. Click en un evento lo edita.
 */
function AgendaView({
  locale,
  items,
  cursor,
  color,
  onSelectEvent,
  onCreateAt,
}: {
  locale: Locale;
  items: Positioned[];
  cursor: Date;
  color: string;
  onSelectEvent: (e: GridEvent, anchor: DOMRect) => void;
  onCreateAt: (day: Date) => void;
}): JSX.Element {
  const t = useT(locale);
  const [query, setQuery] = useState('');
  const q = normalizeText(query.trim());
  const matched = q
    ? items.filter((p) =>
        normalizeText(
          `${p.event.summary} ${p.event.location ?? ''} ${p.event.description ?? ''}`,
        ).includes(q),
      )
    : items;

  // Agrupa por día (asc). Un evento en curso desde antes de la ventana se ancla
  // al primer día (cursor) para no encabezar con una fecha pasada.
  const windowStart = startOfDay(cursor);
  const groups = new Map<string, Positioned[]>();
  for (const p of [...matched].sort((a, b) => a.date.getTime() - b.date.getTime())) {
    const day = p.date.getTime() < windowStart.getTime() ? windowStart : p.date;
    const bucket = groups.get(dayKey(day));
    if (bucket) bucket.push(p);
    else groups.set(dayKey(day), [p]);
  }
  const days = [...groups.values()];

  return (
    <div class="bg-surface flex h-full flex-col">
      <div class="border-border border-b p-2">
        <input
          type="search"
          value={query}
          onInput={(e) => setQuery(e.currentTarget.value)}
          placeholder={t('search.events')}
          aria-label={t('search.events')}
          class={cx(
            'border-border bg-surface text-text h-9 w-full rounded-md border px-3 text-sm',
            'placeholder:text-text-subtle focus-visible:outline-none',
            'sytue-focus-ring',
          )}
        />
      </div>
      {days.length === 0 ? (
        // Un vacío que solo dice «no hay nada» deja al usuario sin salida: sin
        // búsqueda en curso, la agenda vacía ofrece la acción que la llena.
        <div class="flex flex-1 flex-col items-center justify-center gap-3 p-8">
          <span class="text-text-subtle text-sm">
            {q ? t('agenda.noMatches') : t('cal.noUpcoming')}
          </span>
          {q ? null : (
            <Button
              variant="secondary"
              size="sm"
              leadingIcon={<Icon name="plus" size={16} />}
              onClick={() => onCreateAt(cursor)}
            >
              {t('nav.createEvent')}
            </Button>
          )}
        </div>
      ) : (
        <ul class="divide-border min-h-0 flex-1 divide-y overflow-y-auto">
          {days.map((evs) => {
            const first = evs[0]!.date;
            const day = first.getTime() < windowStart.getTime() ? windowStart : first;
            return (
              <li key={dayKey(day)} class="flex gap-3 px-3 py-2.5">
                <div class="w-14 shrink-0 pt-0.5 text-center">
                  <div class="text-text-subtle sytue-overline">
                    {weekdayLabels(locale)[(day.getDay() + 6) % 7]}
                  </div>
                  <div
                    class={cx(
                      'text-lg font-semibold tabular-nums',
                      isToday(day) ? 'text-primary-text' : 'text-text',
                    )}
                  >
                    {day.getDate()}
                  </div>
                  <div class="sytue-meta">{monthShortLabel(locale, day)}</div>
                </div>
                <ul class="flex min-w-0 flex-1 flex-col gap-1">
                  {evs.map((p) => (
                    <AgendaRow
                      locale={locale}
                      key={p.event.href}
                      event={p.event}
                      date={p.date}
                      color={color}
                      onClick={onSelectEvent}
                    />
                  ))}
                </ul>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}

function AgendaRow({
  locale,
  event,
  date,
  color,
  onClick,
}: {
  locale: Locale;
  event: GridEvent;
  date: Date;
  color: string;
  onClick: (e: GridEvent, anchor: DOMRect) => void;
}): JSX.Element {
  const t = useT(locale);
  const allDay = /^\d{8}$/.test(event.start);
  return (
    <span
      role="button"
      tabIndex={0}
      onClick={(e: MouseEvent) => {
        e.stopPropagation();
        onClick(event, (e.currentTarget as HTMLElement).getBoundingClientRect());
      }}
      onKeyDown={(e: KeyboardEvent) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          e.stopPropagation();
          onClick(event, (e.currentTarget as HTMLElement).getBoundingClientRect());
        }
      }}
      class={cx(
        'flex min-w-0 items-center gap-2.5 rounded-md border border-transparent px-2 py-1.5 text-left',
        'hover:border-border hover:bg-bg-subtle cursor-pointer transition-colors duration-150',
        'sytue-focus-ring',
      )}
      title={event.summary}
    >
      <span
        class="h-2 w-2 shrink-0 rounded-full"
        style={{ backgroundColor: event.color ?? color }}
        aria-hidden="true"
      />
      <span class="text-text-muted w-20 shrink-0 text-xs font-medium tabular-nums">
        {allDay ? t('event.allDay') : timeLabel(locale, date)}
      </span>
      <span class="text-text min-w-0 flex-1 truncate text-sm">
        {event.summary || t('event.untitled')}
      </span>
      {event.location ? (
        <span class="sytue-meta hidden max-w-[38%] shrink truncate sm:block">{event.location}</span>
      ) : null}
    </span>
  );
}

function EventChip({
  locale,
  event,
  date,
  color,
  expanded,
  onClick,
}: {
  locale: Locale;
  event: GridEvent;
  date: Date;
  /** Color de reserva; prima el del calendario del evento. */
  color: string;
  expanded?: boolean;
  onClick: (e: GridEvent, anchor: DOMRect) => void;
}): JSX.Element {
  return (
    <span
      role="button"
      tabIndex={0}
      onClick={(e: MouseEvent) => {
        e.stopPropagation();
        onClick(event, (e.currentTarget as HTMLElement).getBoundingClientRect());
      }}
      onKeyDown={(e: KeyboardEvent) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          e.stopPropagation();
          onClick(event, (e.currentTarget as HTMLElement).getBoundingClientRect());
        }
      }}
      class={cx(
        // Pildora sobre superficie con borde: el color del calendario va en el
        // punto (portador de color con texto AA garantizado sobre el borde).
        'border-border bg-bg-subtle text-text flex min-w-0 items-center gap-1.5 rounded-sm border px-1.5 text-xs',
        'cursor-pointer transition-colors duration-150',
        'hover:border-border-strong hover:bg-surface hover:shadow-[var(--shadow-xs)]',
        'sytue-focus-ring',
        expanded ? 'py-1' : 'py-0.5',
      )}
      title={event.summary}
    >
      <span
        class="h-2 w-2 shrink-0 rounded-full"
        style={{ backgroundColor: event.color ?? color }}
        aria-hidden="true"
      />
      {expanded ? (
        <span class="text-text-muted shrink-0 font-medium tabular-nums">
          {timeLabel(locale, date)}
        </span>
      ) : null}
      <span class="truncate">{event.summary}</span>
    </span>
  );
}
