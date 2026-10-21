/**
 * TimeGrid — rejilla horaria REAL de las vistas semana/día (sustituye a la lista
 * anterior). Eje de horas 0–24 a la izquierda; cada evento se POSICIONA por su
 * hora y se DIMENSIONA por su duración; los que se solapan se reparten en
 * columnas lado a lado (timeLayout). Franja aparte para «todo el día», línea de
 * «hora actual» y arrastre para CREAR (pintar el rango), MOVER (incl. cambio de
 * día en semana) y REDIMENSIONAR (borde inferior). Imantado a 15 min. Los
 * eventos de una serie recurrente no se arrastran (mover una instancia suelta es
 * otra ola): se editan con clic/Intro como hasta ahora. Solo Tailwind/tokens
 * @sytue/ui — hairlines 1px, sin degradados ni sombras pesadas (anti AI-slop).
 */
import type { CSSProperties, JSX, TargetedPointerEvent } from 'preact';
import type { Locale } from '@sytue/i18n';
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'preact/hooks';
import { cx, useToast } from '@sytue/ui';
import type { GridEvent } from './CalendarGrid.js';
import { eventInstant, parseTzid, isToday, timeLabel } from './dates.js';
import { DiaDeSemana } from './DiaDeSemana.js';
import { parseRecurrence } from '../lib/ical.js';
import {
  dayMinutes,
  layoutTimed,
  snapMin,
  DAY_MINUTES,
  MIN_EVENT_MIN,
  SNAP_MIN,
} from './timeLayout.js';
import {
  barSpan,
  assignRows,
  sliceWeeks,
  rowCount,
  barTextColor,
  type Bar,
  type Segment,
} from './multiDayLayout.js';
import { useT } from '../i18n/index.js';

/** Alto de una hora en px: fija la escala minuto→píxel de toda la rejilla. */
const HOUR_PX = 48;
const PX_PER_MIN = HOUR_PX / 60;
const TOTAL_PX = DAY_MINUTES * PX_PER_MIN;

// Geometría de las barras de la franja «todo el día».
const STRIP_BAR_H = 22; // alto de una fila de barras, con separación
const STRIP_BAR_INNER_H = 18; // alto pintado de la barra
const STRIP_PAD = 4; // relleno vertical de la franja

type PE = TargetedPointerEvent<HTMLDivElement>;

interface Props {
  /** Idioma ya negociado en el servidor: fechas y horas salen en su convención. */
  locale: Locale;
  /** Días de la vista: 7 (semana) o 1 (día). */
  days: Date[];
  events: GridEvent[];
  /** Color de reserva si un evento no trae el de su calendario. */
  accentColor: string;
  /** Abre la vista rápida del evento anclada a su rectángulo (viewport). */
  onSelectEvent: (event: GridEvent, anchor: DOMRect) => void;
  /** Crear con el rango pintado (o 1 h por clic simple). */
  onCreateRange: (start: Date, end: Date) => void;
  /** Persistir un mover/redimensionar; el llamador recarga al terminar. */
  onMoveResize: (event: GridEvent, start: Date, end: Date) => Promise<void>;
}

interface TimedBlock {
  event: GridEvent;
  recurring: boolean;
  startMin: number;
  endMin: number;
  col: number;
  cols: number;
}

interface DayLayout {
  day: Date;
  timed: TimedBlock[];
}

type Drag =
  | { kind: 'create'; dayIdx: number; anchorMin: number; curMin: number }
  | {
      kind: 'move';
      event: GridEvent;
      dayIdx: number;
      startMin: number;
      durMin: number;
      grabOffMin: number;
      col: number;
      cols: number;
      moved: boolean;
      /** Rectángulo del bloque al agarrar; ancla la vista rápida si fue un clic. */
      rect: DOMRect;
    }
  | {
      kind: 'resize';
      event: GridEvent;
      dayIdx: number;
      startMin: number;
      endMin: number;
      col: number;
      cols: number;
    };

interface Pending {
  event: GridEvent;
  dayIdx: number;
  startMin: number;
  endMin: number;
  col: number;
  cols: number;
}

/** Date en el minuto `min` (desde 00:00) del día `day`. min puede pasar de 1440. */
function dateAtMinute(day: Date, min: number): Date {
  const d = new Date(day);
  d.setHours(0, 0, 0, 0);
  d.setMinutes(min);
  return d;
}

function currentMinutes(): number {
  const n = new Date();
  return n.getHours() * 60 + n.getMinutes();
}

export function TimeGrid({
  locale,
  days,
  events,
  accentColor,
  onSelectEvent,
  onCreateRange,
  onMoveResize,
}: Props): JSX.Element {
  const t = useT(locale);
  const toast = useToast();
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const regionRef = useRef<HTMLDivElement | null>(null);
  const [drag, setDrag] = useState<Drag | null>(null);
  const [pending, setPending] = useState<Pending | null>(null);
  const [nowMin, setNowMin] = useState(currentMinutes);
  const [mounted, setMounted] = useState(false);
  // Paso de imantado del arrastre (crear/mover/redimensionar): conmutable 15↔30.
  const [snap, setSnap] = useState<number>(SNAP_MIN);

  const daysKey = days.map((d) => d.getTime()).join(',');

  // Layout por día: separa «todo el día» (franja) de los cronometrados (rejilla)
  // y reparte estos en columnas de solape. No depende del arrastre → no recalcula
  // en cada pointermove.
  const layout = useMemo<DayLayout[]>(() => {
    // Resuelve cada evento a la ZONA DE VISUALIZACIÓN (instante real según su
    // TZID) UNA vez; timeLayout recibe ya esos instantes y reparte por columnas.
    const resolved = events.map((ev) => {
      const tz = parseTzid(ev.ical);
      return {
        ev,
        start: eventInstant(ev.start, tz),
        end: ev.end ? eventInstant(ev.end, tz) : null,
        allDay: /^\d{8}$/.test(ev.start),
        recurring: Boolean(ev.ical && parseRecurrence(ev.ical)),
      };
    });
    return days.map((day) => {
      const timedInput: Array<{
        startMin: number;
        endMin: number;
        event: GridEvent;
        recurring: boolean;
      }> = [];
      for (const r of resolved) {
        if (!r.start) continue;
        if (r.allDay) continue; // todo el día → barra en la franja, no en la rejilla
        const iv = dayMinutes(r.start, r.end, day);
        if (!iv) continue;
        timedInput.push({ ...iv, event: r.ev, recurring: r.recurring });
      }
      return { day, timed: layoutTimed(timedInput) };
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [events, daysKey]);

  // Barras de la franja «todo el día»: los eventos de todo el día como barras
  // continuas que abarcan sus días del rango (apiladas sin solape). Memoizado.
  const allDayBars = useMemo<Segment<GridEvent>[]>(() => {
    const bars: Bar<GridEvent>[] = [];
    for (const ev of events) {
      if (!/^\d{8}$/.test(ev.start)) continue;
      const tz = parseTzid(ev.ical);
      const start = eventInstant(ev.start, tz);
      if (!start) continue;
      const sp = barSpan(days, start, ev.end ? eventInstant(ev.end, tz) : null);
      if (!sp) continue;
      bars.push({ item: ev, start: sp.start, end: sp.end });
    }
    return sliceWeeks(assignRows(bars), 1, days.length)[0] ?? [];
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [events, daysKey]);

  const stripRows = rowCount(allDayBars);
  const hasAllDay = allDayBars.length > 0;

  // La hora actual avanza mientras la vista está abierta. `mounted` evita el
  // desajuste de hidratación (reloj del servidor ≠ del navegador).
  useEffect(() => {
    setMounted(true);
    const id = setInterval(() => setNowMin(currentMinutes()), 60_000);
    return () => clearInterval(id);
  }, []);

  // Un mover/redimensionar en vuelo se descarta cuando llegan datos frescos del
  // padre (la recarga trae la posición ya guardada).
  useEffect(() => {
    setPending(null);
  }, [events]);

  // Al abrir, encuadra la franja laboral: hoy → hora actual; si no, 08:00.
  useLayoutEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const focusMin = days.some(isToday) ? currentMinutes() : 8 * 60;
    el.scrollTop = Math.max(0, (focusMin - 60) * PX_PER_MIN);
    // Solo al montar; navegar de semana conserva el desplazamiento.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  /** Coordenadas del puntero → (columna de día, minuto crudo del día). */
  function locate(e: PE): { dayIdx: number; min: number } {
    const el = regionRef.current;
    if (!el) return { dayIdx: 0, min: 0 };
    const r = el.getBoundingClientRect();
    const colW = r.width / days.length;
    const dayIdx = Math.max(0, Math.min(days.length - 1, Math.floor((e.clientX - r.left) / colW)));
    const min = (e.clientY - r.top) / PX_PER_MIN;
    return { dayIdx, min };
  }

  // --- Inicio de gestos --------------------------------------------------------

  function startCreate(e: PE, dayIdx: number): void {
    if (e.button !== 0) return;
    e.currentTarget.setPointerCapture(e.pointerId);
    const anchor = snapMin(locate(e).min, snap);
    setDrag({ kind: 'create', dayIdx, anchorMin: anchor, curMin: anchor });
  }

  function startMove(e: PE, block: TimedBlock, dayIdx: number): void {
    // Botón no primario: sin arrastre. Recurrentes SÍ se arrastran; al soltar, el
    // padre pregunta el alcance (este / este y siguientes / todos). Los eventos de
    // un calendario de SOLO LECTURA (suscripción ICS) no se mueven.
    if (block.event.readOnly) return;
    e.stopPropagation();
    if (e.button !== 0) return;
    const rect = e.currentTarget.getBoundingClientRect();
    e.currentTarget.setPointerCapture(e.pointerId);
    setDrag({
      kind: 'move',
      event: block.event,
      dayIdx,
      startMin: block.startMin,
      durMin: block.endMin - block.startMin,
      grabOffMin: locate(e).min - block.startMin,
      col: block.col,
      cols: block.cols,
      moved: false,
      rect,
    });
  }

  function startResize(e: PE, block: TimedBlock, dayIdx: number): void {
    if (block.event.readOnly) return; // suscripción ICS: no redimensionable
    e.stopPropagation();
    if (e.button !== 0) return;
    e.currentTarget.setPointerCapture(e.pointerId);
    setDrag({
      kind: 'resize',
      event: block.event,
      dayIdx,
      startMin: block.startMin,
      endMin: block.endMin,
      col: block.col,
      cols: block.cols,
    });
  }

  // --- Seguimiento y cierre ----------------------------------------------------

  function onRegionMove(e: PE): void {
    if (!drag) return;
    const { dayIdx, min } = locate(e);
    setDrag((d) => {
      if (!d) return d;
      if (d.kind === 'create') return { ...d, curMin: snapMin(min, snap) };
      if (d.kind === 'move') {
        const startMin = Math.max(
          0,
          Math.min(DAY_MINUTES - MIN_EVENT_MIN, snapMin(min - d.grabOffMin, snap)),
        );
        return { ...d, dayIdx, startMin, moved: true };
      }
      return {
        ...d,
        endMin: Math.min(DAY_MINUTES, Math.max(d.startMin + MIN_EVENT_MIN, snapMin(min, snap))),
      };
    });
  }

  async function commit(
    event: GridEvent,
    dayIdx: number,
    startMin: number,
    endMin: number,
    col: number,
    cols: number,
  ): Promise<void> {
    const day = days[dayIdx]!;
    // Mantén el bloque en el sitio soltado hasta que la recarga traiga el dato
    // guardado (evita el parpadeo de vuelta a la posición anterior).
    setPending({ event, dayIdx, startMin, endMin, col, cols });
    try {
      await onMoveResize(event, dateAtMinute(day, startMin), dateAtMinute(day, endMin));
    } catch {
      setPending(null);
      toast.notify({ tone: 'danger', title: t('cal.error.updateEvent') });
    }
  }

  function onRegionUp(): void {
    const d = drag;
    if (!d) return;
    setDrag(null);
    if (d.kind === 'create') {
      const lo = Math.min(d.anchorMin, d.curMin);
      const hi = Math.max(d.anchorMin, d.curMin);
      const day = days[d.dayIdx]!;
      // Clic sin arrastre → evento de 1 h; arrastre → el rango pintado.
      const endMin = hi - lo < snap ? Math.min(DAY_MINUTES, lo + 60) : hi;
      onCreateRange(dateAtMinute(day, lo), dateAtMinute(day, endMin));
      return;
    }
    if (d.kind === 'move') {
      if (!d.moved) {
        onSelectEvent(d.event, d.rect); // clic sin mover = vista rápida
        return;
      }
      void commit(d.event, d.dayIdx, d.startMin, d.startMin + d.durMin, d.col, d.cols);
      return;
    }
    void commit(d.event, d.dayIdx, d.startMin, d.endMin, d.col, d.cols);
  }

  // href del evento que se está arrastrando/guardando: su bloque «normal» se
  // omite y se pinta el de arrastre en su lugar.
  const activeHref = drag && 'event' in drag ? drag.event.href : (pending?.event.href ?? null);

  return (
    <div class="bg-surface flex h-full flex-col">
      <div ref={scrollRef} class="min-h-0 flex-1 overflow-y-auto">
        {/* Cabecera de días + franja de todo-el-día: fijas arriba al desplazar. Cristal,
            como la barra superior: en el traslúcido `bg-surface` es un velo, y las horas
            que pasan por debajo («07:00») se leían a través, encima de «15 min». */}
        <div class="sytue-glass-surface bg-surface sticky top-0 z-30">
          <div class="border-border flex border-b">
            <div class="flex w-14 shrink-0 items-center justify-center">
              <button
                type="button"
                onClick={() => setSnap((s) => (s === SNAP_MIN ? 30 : SNAP_MIN))}
                aria-label={t('grid.snapAria', { count: snap })}
                title={t('grid.snapTitle', { count: snap })}
                class={cx(
                  'sytue-meta-dense hover:text-text hover:bg-bg-subtle rounded px-1.5 py-0.5 font-medium tabular-nums transition-colors',
                  'sytue-focus-ring',
                )}
              >
                {t('grid.snapValue', { count: snap })}
              </button>
            </div>
            <div
              class="grid flex-1"
              style={{ gridTemplateColumns: `repeat(${days.length}, minmax(0, 1fr))` }}
            >
              {days.map((day) => (
                // LA CABECERA SIGUE AL ANCHO DE SU COLUMNA (consulta de contenedor), no al
                // de la ventana: una semana mide 38 px por día en un móvil, 63 en una
                // tableta con el menú puesto y 137 en un portátil; un día suelto, todo el
                // ancho. Con sitio (≥110 px), «LUN 5» en una línea. Con menos, apilada: el
                // día encima del número —en una línea, el número del último día se salía
                // de la rejilla y toda la semana desplazaba de lado—. Y si ni apilada cabe
                // la abreviatura, la inicial (ver `DiaDeSemana`).
                <div
                  key={day.getTime()}
                  class={cx(
                    'border-border @container min-w-0 border-l first:border-l-0',
                    isToday(day) && 'bg-selected',
                  )}
                >
                  <div class="flex items-baseline gap-1.5 px-3 py-2 @max-[110px]:flex-col @max-[110px]:items-center @max-[110px]:gap-0 @max-[110px]:px-0.5">
                    <span class="text-text-subtle sytue-overline max-w-full truncate">
                      <DiaDeSemana locale={locale} dia={(day.getDay() + 6) % 7} />
                    </span>
                    <span
                      class={cx(
                        'text-sm font-semibold tabular-nums',
                        isToday(day) ? 'text-primary-text' : 'text-text',
                      )}
                    >
                      {day.getDate()}
                    </span>
                  </div>
                </div>
              ))}
            </div>
          </div>
          {hasAllDay ? (
            <div class="border-border flex border-b">
              <div class="sytue-meta-dense flex w-14 shrink-0 items-start justify-end px-1.5 py-1.5 text-right leading-tight">
                {t('event.allDay')}
              </div>
              {/* Barras continuas: cada evento de todo el día abarca sus días del
                  rango en una fila propia (apilado sin solape). */}
              <div
                class="relative flex-1"
                style={{ height: `${stripRows * STRIP_BAR_H + 2 * STRIP_PAD}px` }}
              >
                {allDayBars.map((seg) => (
                  <AllDayBar
                    key={seg.item.href}
                    locale={locale}
                    seg={seg}
                    cols={days.length}
                    color={accentColor}
                    onSelect={onSelectEvent}
                  />
                ))}
              </div>
            </div>
          ) : null}
        </div>

        {/* Cuerpo horario: eje de horas + región de columnas de día. */}
        <div class="flex" style={{ height: `${TOTAL_PX}px` }}>
          <div class="relative w-14 shrink-0">
            {Array.from({ length: 24 }, (_, h) =>
              h === 0 ? null : (
                <span
                  key={h}
                  class="sytue-meta-dense absolute right-1.5 -translate-y-1/2 tabular-nums"
                  style={{ top: `${h * HOUR_PX}px` }}
                >
                  {String(h).padStart(2, '0')}:00
                </span>
              ),
            )}
          </div>
          <div
            ref={regionRef}
            class="relative grid flex-1"
            style={{ gridTemplateColumns: `repeat(${days.length}, minmax(0, 1fr))` }}
            onPointerMove={onRegionMove}
            onPointerUp={onRegionUp}
            // En táctil, deslizar para DESPLAZAR la rejilla empieza siempre por un
            // pointerdown en una columna, o sea por un arrastre de «crear»; cuando
            // el navegador se queda el gesto para el scroll manda `pointercancel`.
            // Sin recogerlo, el arrastre quedaba vivo: el rectángulo azul se quedaba
            // pintado y el SIGUIENTE toque —el que iba a abrir un evento— caía en
            // este `pointerup` y abría «Nuevo evento» con un rango inventado.
            onPointerCancel={() => setDrag(null)}
          >
            {layout.map(({ day, timed }, di) => (
              <div
                key={day.getTime()}
                class="border-border relative border-l first:border-l-0"
                style={{
                  // Hairline por hora sin DOM extra (crisp, 1px).
                  backgroundImage: `repeating-linear-gradient(to bottom, var(--sytue-border) 0, var(--sytue-border) 1px, transparent 1px, transparent ${HOUR_PX}px)`,
                }}
                onPointerDown={(e) => startCreate(e, di)}
              >
                {timed.map((block) =>
                  block.event.href === activeHref ? null : (
                    <EventBlock
                      locale={locale}
                      key={block.event.href}
                      block={block}
                      day={day}
                      color={accentColor}
                      onSelect={(rect) => onSelectEvent(block.event, rect)}
                      onPointerDownBlock={(e) => startMove(e, block, di)}
                      onPointerDownResize={(e) => startResize(e, block, di)}
                    />
                  ),
                )}

                {/* Bloque en arrastre (mover/redimensionar) en su columna activa. */}
                {drag && drag.kind === 'move' && drag.dayIdx === di ? (
                  <DragBlock
                    locale={locale}
                    startMin={drag.startMin}
                    endMin={drag.startMin + drag.durMin}
                    col={drag.col}
                    cols={drag.cols}
                    color={accentColor}
                    label={drag.event.summary}
                    day={day}
                  />
                ) : null}
                {drag && drag.kind === 'resize' && drag.dayIdx === di ? (
                  <DragBlock
                    locale={locale}
                    startMin={drag.startMin}
                    endMin={drag.endMin}
                    col={drag.col}
                    cols={drag.cols}
                    color={accentColor}
                    label={drag.event.summary}
                    day={day}
                  />
                ) : null}

                {/* Persistencia optimista tras soltar (atenuado mientras guarda). */}
                {pending && pending.dayIdx === di ? (
                  <DragBlock
                    locale={locale}
                    startMin={pending.startMin}
                    endMin={pending.endMin}
                    col={pending.col}
                    cols={pending.cols}
                    color={accentColor}
                    label={pending.event.summary}
                    day={day}
                    saving
                  />
                ) : null}

                {/* Rango pintado al crear. */}
                {drag && drag.kind === 'create' && drag.dayIdx === di ? (
                  <CreateDraft
                    locale={locale}
                    anchorMin={drag.anchorMin}
                    curMin={drag.curMin}
                    day={day}
                  />
                ) : null}

                {/* Línea de hora actual. */}
                {mounted && isToday(day) ? (
                  <div
                    class="pointer-events-none absolute inset-x-0 z-10"
                    style={{ top: `${nowMin * PX_PER_MIN}px` }}
                    aria-hidden="true"
                  >
                    <div class="bg-danger absolute top-0 -left-[3px] h-2 w-2 -translate-y-1/2 rounded-full" />
                    <div class="bg-danger h-[2px] w-full" />
                  </div>
                ) : null}
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}

/** Estilo de posición/tamaño de un bloque a partir de sus minutos y su columna. */
function blockStyle(startMin: number, endMin: number, col: number, cols: number): CSSProperties {
  const top = startMin * PX_PER_MIN;
  const height = Math.max(MIN_EVENT_MIN, endMin - startMin) * PX_PER_MIN;
  return {
    top: `${top}px`,
    height: `${height}px`,
    left: `${(col / cols) * 100}%`,
    width: `calc(${100 / cols}% - 3px)`,
  };
}

function rangeLabel(locale: Locale, day: Date, startMin: number, endMin: number): string {
  return `${timeLabel(locale, dateAtMinute(day, startMin))} – ${timeLabel(locale, dateAtMinute(day, endMin))}`;
}

function EventBlock({
  locale,
  block,
  day,
  color,
  onSelect,
  onPointerDownBlock,
  onPointerDownResize,
}: {
  locale: Locale;
  block: TimedBlock;
  day: Date;
  color: string;
  /** Abre la vista rápida anclada al rectángulo del bloque. */
  onSelect: (anchor: DOMRect) => void;
  onPointerDownBlock: (e: PE) => void;
  onPointerDownResize: (e: PE) => void;
}): JSX.Element {
  const t = useT(locale);
  const { event, startMin, endMin, col, cols } = block;
  const height = Math.max(MIN_EVENT_MIN, endMin - startMin) * PX_PER_MIN;
  // Instante resuelto a la zona de visualización (honra TZID) para las etiquetas.
  const startAt = eventInstant(event.start, parseTzid(event.ical)) ?? dateAtMinute(day, startMin);
  return (
    <div
      role="button"
      tabIndex={0}
      aria-label={`${event.summary || t('event.untitled')}, ${timeLabel(locale, startAt)}`}
      title={event.summary}
      onPointerDown={onPointerDownBlock}
      // Solo lectura (suscripción ICS): no arrastra, así que el clic nativo abre la
      // vista rápida. Los editables la abren por el gesto (clic sin mover).
      onClick={(e) => {
        if (event.readOnly) onSelect((e.currentTarget as HTMLElement).getBoundingClientRect());
      }}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          onSelect((e.currentTarget as HTMLElement).getBoundingClientRect());
        }
      }}
      class={cx(
        'border-border bg-bg-subtle text-text absolute overflow-hidden rounded-sm border select-none',
        'transition-shadow duration-150 hover:z-10 hover:border-[color:var(--sytue-border-strong)] hover:shadow-[var(--shadow-xs)]',
        'sytue-focus-ring focus-visible:z-10',
        // Suscripción ICS (solo lectura): no se arrastra; cursor de solo apertura.
        event.readOnly ? 'cursor-pointer' : 'cursor-grab active:cursor-grabbing',
      )}
      style={blockStyle(startMin, endMin, col, cols)}
    >
      <span
        class="absolute inset-y-0 left-0 w-[3px]"
        style={{ backgroundColor: event.color ?? color }}
        aria-hidden="true"
      />
      <div class="min-w-0 py-0.5 pr-1 pl-2">
        <div class="truncate text-xs leading-tight font-medium">
          {event.summary || t('event.untitled')}
        </div>
        {height >= 34 ? (
          <div class="text-text-muted text-2xs truncate leading-tight tabular-nums">
            {rangeLabel(locale, startAt, startMin, endMin)}
          </div>
        ) : null}
      </div>
      {height >= 24 && !event.readOnly ? (
        <div
          onPointerDown={onPointerDownResize}
          class="absolute inset-x-0 bottom-0 h-2 cursor-ns-resize"
          aria-hidden="true"
        />
      ) : null}
    </div>
  );
}

/** Bloque «fantasma» durante el arrastre o el guardado optimista. */
function DragBlock({
  locale,
  startMin,
  endMin,
  col,
  cols,
  color,
  label,
  day,
  saving,
}: {
  locale: Locale;
  startMin: number;
  endMin: number;
  col: number;
  cols: number;
  color: string;
  label: string;
  day: Date;
  saving?: boolean;
}): JSX.Element {
  const t = useT(locale);
  return (
    <div
      class={cx(
        'border-border bg-surface text-text pointer-events-none absolute z-20 overflow-hidden rounded-sm border shadow-[var(--shadow-sm)]',
        saving && 'opacity-70',
      )}
      style={blockStyle(startMin, endMin, col, cols)}
      aria-hidden="true"
    >
      <span class="absolute inset-y-0 left-0 w-[3px]" style={{ backgroundColor: color }} />
      <div class="min-w-0 py-0.5 pr-1 pl-2">
        <div class="truncate text-xs leading-tight font-medium">{label || t('event.untitled')}</div>
        <div class="text-text-muted text-2xs truncate leading-tight tabular-nums">
          {rangeLabel(locale, day, startMin, endMin)}
        </div>
      </div>
    </div>
  );
}

/** Rectángulo del rango que se está pintando al crear. */
function CreateDraft({
  locale,
  anchorMin,
  curMin,
  day,
}: {
  locale: Locale;
  anchorMin: number;
  curMin: number;
  day: Date;
}): JSX.Element {
  const lo = Math.min(anchorMin, curMin);
  const hi = Math.max(anchorMin, curMin);
  const top = lo * PX_PER_MIN;
  const height = Math.max(MIN_EVENT_MIN, hi - lo) * PX_PER_MIN;
  return (
    <div
      class="border-primary bg-primary/10 pointer-events-none absolute inset-x-0.5 z-20 rounded-sm border"
      style={{ top: `${top}px`, height: `${height}px` }}
      aria-hidden="true"
    >
      <div class="text-primary-text text-2xs px-2 py-0.5 font-medium tabular-nums">
        {rangeLabel(locale, day, lo, hi === lo ? lo + 60 : hi)}
      </div>
    </div>
  );
}

/**
 * Barra de un evento de todo el día en la franja superior: rectángulo con el
 * color del evento que abarca sus días del rango, con texto de contraste AA y
 * bordes rectos en los cortes de continuación (evento que empieza/termina fuera
 * del rango). Clic/Intro abre la vista rápida.
 */
function AllDayBar({
  locale,
  seg,
  cols,
  color,
  onSelect,
}: {
  locale: Locale;
  seg: Segment<GridEvent>;
  /** Nº de columnas de la franja (7 en semana, 1 en día). */
  cols: number;
  color: string;
  onSelect: (event: GridEvent, anchor: DOMRect) => void;
}): JSX.Element {
  const t = useT(locale);
  const event = seg.item;
  const fill = event.color ?? color;
  const inL = seg.continuesLeft ? 0 : 3;
  const inR = seg.continuesRight ? 0 : 3;
  return (
    <button
      type="button"
      title={event.summary}
      aria-label={event.summary || t('event.untitled')}
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
        'absolute flex items-center overflow-hidden px-1.5 text-left',
        'transition-[filter] duration-150 hover:brightness-95',
        'sytue-focus-ring focus-visible:z-10',
        seg.continuesLeft ? 'rounded-l-none' : 'rounded-l-[3px]',
        seg.continuesRight ? 'rounded-r-none' : 'rounded-r-[3px]',
      )}
      style={{
        top: `${STRIP_PAD + seg.row * STRIP_BAR_H}px`,
        left: `calc(${(seg.col / cols) * 100}% + ${inL}px)`,
        width: `calc(${(seg.span / cols) * 100}% - ${inL + inR}px)`,
        height: `${STRIP_BAR_INNER_H}px`,
        backgroundColor: fill,
        color: barTextColor(fill),
      }}
    >
      <span class="text-2xs truncate leading-none font-medium">
        {event.summary || t('event.untitled')}
      </span>
    </button>
  );
}
