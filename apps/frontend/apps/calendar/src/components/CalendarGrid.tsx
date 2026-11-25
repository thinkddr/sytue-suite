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
import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'preact/hooks';
import { Button, Icon, Popover, cx, usePointerDrag, useToast } from '@sytue/ui';
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
  isoWeek,
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
import { celdaEnPunto, desplazarDias, eventoDesplazado } from './arrastreMes.js';
import { plegar } from '@sytue/i18n';

// Geometría de las barras multi-día en la vista MES.
// La barra mide 24, como las fichas con hora (`EventChip`): es un control y su objetivo
// táctil es el de WCAG 2.5.8. Medía 18 y era lo único de la celda por debajo. La fila
// suma la misma separación de 2 px que hay entre fichas (`gap-0.5`).
const BAR_H = 26; // alto de una fila de barras, con separación
const BAR_INNER_H = 24; // alto pintado de la barra
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
// Lo que tiene que viajar el puntero para que pulsar una píldora deje de ser un clic
// (vista rápida) y pase a ser un arrastre: el mismo umbral que el desplazamiento por
// los bordes de `usePointerDrag`, para que un temblor de la mano no mueva nada.
const UMBRAL_ARRASTRE_PX = 4;
// Tras soltar un arrastre el navegador aún emite el `click` del gesto; durante este
// margen se traga, o abriría la vista rápida (o «Crear» en la celda de destino).
const MARGEN_CLIC_TRAS_ARRASTRE_MS = 500;

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
  /** Abrir ese día en la vista Día (el número del día en el mes). Sin él, el número
   * es solo un rótulo. */
  onOpenDay?: ((day: Date) => void) | undefined;
  /** Crear con el rango pintado en la rejilla horaria (semana/día). */
  onCreateRange: (start: Date, end: Date) => void;
  /** Persistir un mover/redimensionar de la rejilla horaria (semana/día). */
  onMoveResize: (event: GridEvent, start: Date, end: Date) => Promise<void>;
  /**
   * Los ajustes de la persona que cambian lo que se ve (`lib/preferencias.ts`). Sin
   * ellos, lo de siempre: semana entera, sin número de semana, jornada de 9 a 18 y
   * clic de una hora.
   */
  vista?: VistaAjustada | undefined;
}

/** Lo de los ajustes que llega a la rejilla. */
export interface VistaAjustada {
  finesDeSemana: boolean;
  numeroSemana: boolean;
  inicioLaboral: number;
  finLaboral: number;
  duracion: number;
}

/**
 * Los días de la vista semana: sin fines de semana, de lunes a viernes. La semana
 * empieza en lunes (`weekGrid`), así que basta con quitar sábado y domingo.
 */
export function diasDeSemana(cursor: Date, finesDeSemana: boolean): Date[] {
  const dias = weekGrid(cursor);
  return finesDeSemana ? dias : dias.filter((d) => d.getDay() !== 0 && d.getDay() !== 6);
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

/** Un arrastre en curso en el mes: qué evento, de qué celda y hasta qué celda. */
interface ArrastreMes {
  event: GridEvent;
  /** Celda (índice de la rejilla) donde se cogió. */
  origen: number;
  /** Celda bajo el puntero ahora. */
  destino: number;
  /** Ya ha pasado el umbral: deja de ser un clic. */
  movido: boolean;
}

/**
 * Un evento soltado en otro día cuyo guardado aún no ha vuelto. Se reconoce por
 * IDENTIDAD y no por href: las ocurrencias de una serie comparten href, y solo se
 * pinta movida la que se arrastró.
 */
interface PendienteMes {
  original: GridEvent;
  desplazado: GridEvent;
  dias: number;
}

interface MonthChip {
  event: GridEvent;
  /** Instante de inicio resuelto (honra TZID); ordena los chips del día. */
  date: Date;
}

// Lo que mide cada cosa de la celda del mes, para saber cuántas fichas caben.
const FICHA_H = 24; // `EventChip`: `min-h-[max(24px,1.5rem)]`
const FICHA_GAP = 2; // `gap-0.5` entre fichas y `mt-0.5` sobre la primera
// «+N más» mide 24 (el objetivo de WCAG 2.5.8) pero se le reserva lo de siempre, su
// hueco y una línea de `text-xs`: va pegado a la última ficha, sin el `gap-0.5`, y baja
// los 6 px del relleno de la celda (`MasDelDia`). Así no le quita sitio a ninguna ficha.
const MAS_H = 16;
const FICHA2_H = 38; // `EventChip` en dos líneas: dos de `text-xs`, `py-0.5` y el borde
const CELDA_PIE = 6 + 1; // el relleno de abajo (`p-[6px]`) y el borde inferior

/**
 * Cuántas fichas con hora caben en una celda del mes: la semana mide `altoSemana`, las
 * barras se llevan `barras` px y hay `total` fichas. Si no caben todas (o hay barras
 * escondidas, `masForzado`), deja sitio para «+N más». Sin medir (`altoSemana` 0: el
 * primer pintado, o un DOM sin alturas) devuelve null y manda el presupuesto fijo de
 * carriles.
 *
 * Antes solo contaba carriles (cuatro, menos las barras) y no el alto: a 1280×800 la
 * cuarta ficha de un día no se veía y la tercera salía cortada, sin «+N más» que lo
 * dijera, y en una semana con barra la segunda ficha perdía el borde de abajo.
 */
export function fichasQueCaben(
  altoSemana: number,
  barras: number,
  total: number,
  masForzado = false,
): number | null {
  if (!(altoSemana > 0)) return null;
  const hueco = altoSemana - CELDA_PIE - DATE_ROW_H - barras - FICHA_GAP;
  const todas = Math.floor((hueco + FICHA_GAP) / (FICHA_H + FICHA_GAP));
  if (!masForzado && total <= todas) return total;
  return Math.max(0, Math.floor((hueco - MAS_H) / (FICHA_H + FICHA_GAP)));
}

/**
 * Si las `total` fichas de una celda caben en DOS líneas cada una bajo `barras` px de
 * barras, en una semana de `altoSemana`: entonces van en dos (`EventChip dosLineas`) y
 * el título se lee entero en vez de cortarse a la primera línea. Sin medir, solo la
 * ficha sola de una semana sin barras, como antes.
 *
 * Antes solo iba en dos líneas la ficha SOLA de su celda: con dos o más, todas se
 * cortaban a una aunque sobrara sitio, y el título solo se leía en el aviso emergente.
 */
export function fichasEnDosLineas(altoSemana: number, barras: number, total: number): boolean {
  if (total === 0) return false;
  if (!(altoSemana > 0)) return barras === 0 && total === 1;
  const hueco = altoSemana - CELDA_PIE - DATE_ROW_H - barras - FICHA_GAP;
  return total * (FICHA2_H + FICHA_GAP) - FICHA_GAP <= hueco;
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
  onPointerDown,
  atenuado,
}: {
  locale: Locale;
  seg: Segment<GridEvent>;
  accent: string;
  onSelect: (event: GridEvent, anchor: DOMRect) => void;
  /** Empieza el arrastre a otro día (sin él, la barra solo se abre). */
  onPointerDown?: ((e: PointerEvent) => void) | undefined;
  /** Es la que se está arrastrando: su sitio de antes queda en tenue. */
  atenuado?: boolean | undefined;
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
      onPointerDown={onPointerDown}
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
        'pointer-events-auto absolute flex items-center overflow-hidden px-1.5 text-left select-none',
        'transition-[filter,opacity] duration-150 hover:brightness-95',
        onPointerDown ? 'cursor-grab active:cursor-grabbing' : 'cursor-pointer',
        atenuado && 'opacity-50',
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
        // `text-xs`, el cuerpo de las fichas: a 24 de alto el `text-2xs` se quedaba chico.
        <span class="truncate text-xs leading-none font-medium">
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
  onOpenDay,
  onCreateRange,
  onMoveResize,
  vista,
}: Props): JSX.Element {
  const t = useT(locale);
  const toast = useToast();
  const items = position(events);
  const color = accentColor ?? DEFAULT_ACCENT;

  // Un evento soltado en otro día se pinta ya en su sitio nuevo mientras se guarda;
  // la recarga del padre trae el dato guardado y lo sustituye (o lo devuelve a su día
  // si se canceló el alcance de una serie o falló el guardado).
  const [pendienteMes, setPendienteMes] = useState<PendienteMes | null>(null);
  useEffect(() => {
    setPendienteMes(null);
  }, [events]);
  const eventosMes = useMemo(
    () =>
      pendienteMes
        ? events.map((e) => (e === pendienteMes.original ? pendienteMes.desplazado : e))
        : events,
    [events, pendienteMes],
  );

  // Layout de barras multi-día del MES: se recalcula solo al cambiar los eventos
  // o el cursor (memoizado), nunca en cada render — clave para la escala.
  const monthLayout = useMemo(
    () => (mode === 'month' ? buildMonthLayout(eventosMes, monthGrid(cursor)) : null),
    [mode, eventosMes, cursor],
  );

  // --- Arrastrar a otro día en el mes ------------------------------------------
  // Solo la rejilla horaria arrastraba, y el mes es donde se replanifica casi todo.
  // Se arrastra la píldora o la barra; el desplazamiento son los días entre la celda
  // donde se cogió y la celda donde se suelta, así que una barra de varios días
  // cogida por su tercer día cae con ese día bajo el puntero. Guarda por el mismo
  // `onMoveResize` que la rejilla horaria: pregunta el alcance en una serie, no mueve
  // una suscripción y trae «Deshacer».
  const rejillaMesRef = useRef<HTMLDivElement | null>(null);
  // Alto de una semana del mes, medido: de él sale cuántas fichas caben en cada celda.
  const [altoSemana, setAltoSemana] = useState(0);
  const semanasMes = monthLayout?.weeks.length ?? 0;
  // De maquetación: medido antes de pintar, el primer fotograma ya sale con lo que cabe.
  useLayoutEffect(() => {
    const el = rejillaMesRef.current;
    if (!el || semanasMes === 0 || typeof ResizeObserver === 'undefined') return;
    const medir = (): void => setAltoSemana(el.clientHeight / semanasMes);
    medir();
    const observador = new ResizeObserver(medir);
    observador.observe(el);
    return () => observador.disconnect();
  }, [semanasMes]);
  const arrastreMesRef = useRef<ArrastreMes | null>(null);
  const [arrastreMes, setArrastreMes] = useState<ArrastreMes | null>(null);
  const suprimirClicHastaRef = useRef(0);

  function celdaDelPuntero(e: { clientX: number; clientY: number }): number | null {
    const el = rejillaMesRef.current;
    if (!el || !monthLayout) return null;
    return celdaEnPunto(el.getBoundingClientRect(), monthLayout.weeks.length, e.clientX, e.clientY);
  }

  // En `useCallback`: tocan refs y solo se llaman desde manejadores, nunca al pintar.
  const terminarArrastreMes = useCallback((): ArrastreMes | null => {
    const d = arrastreMesRef.current;
    arrastreMesRef.current = null;
    setArrastreMes(null);
    if (d?.movido) suprimirClicHastaRef.current = Date.now() + MARGEN_CLIC_TRAS_ARRASTRE_MS;
    return d;
  }, []);

  /** ¿Es este `click` el que cierra un arrastre? Si lo es, lo gasta. */
  const esClicDeArrastre = useCallback((): boolean => {
    if (Date.now() >= suprimirClicHastaRef.current) return false;
    suprimirClicHastaRef.current = 0;
    return true;
  }, []);

  // La píldora que se suelta en otro día se vuelve a montar allí antes de que llegue
  // su `click`, que entonces ya no pasa por el contenedor: lo filtra ella misma.
  function seleccionarEnMes(event: GridEvent, anchor: DOMRect): void {
    if (!esClicDeArrastre()) onSelectEvent(event, anchor);
  }

  const arrastreDelMes = usePointerDrag<null>({
    // En el documento: la píldora puede desmontarse a mitad del gesto (pasa a «+N más»
    // o cambia de fila al recolocarse) y el arrastre tiene que sobrevivirle.
    documentEvents: true,
    onMove: ({ dxRaw, dyRaw }, e) => {
      const d = arrastreMesRef.current;
      if (!d) return;
      const destino = celdaDelPuntero(e) ?? d.destino;
      const movido = d.movido || Math.hypot(dxRaw, dyRaw) >= UMBRAL_ARRASTRE_PX;
      if (destino === d.destino && movido === d.movido) return;
      const siguiente = { ...d, destino, movido };
      arrastreMesRef.current = siguiente;
      if (movido) setArrastreMes(siguiente);
    },
    // Sin arrastre real no se hace nada aquí: el `click` del botón abre la vista rápida.
    onEnd: () => {
      const d = terminarArrastreMes();
      if (d?.movido && d.destino !== d.origen) void soltarEnMes(d.event, d.destino - d.origen);
    },
    onCancel: () => {
      terminarArrastreMes();
    },
  });

  function empezarArrastreMes(e: PointerEvent, event: GridEvent): void {
    // Una suscripción no se mueve. Con el dedo, pulsar y deslizar es desplazar la
    // página o abrir la píldora; el arrastre es del ratón y del lápiz.
    if (event.readOnly || e.pointerType === 'touch') return;
    const origen = celdaDelPuntero(e);
    if (origen === null) return;
    if (!arrastreDelMes.iniciarArrastre(e, null)) return;
    arrastreMesRef.current = { event, origen, destino: origen, movido: false };
  }

  async function soltarEnMes(event: GridEvent, dias: number): Promise<void> {
    // Se mueve siempre el evento TAL COMO LLEGÓ: si se vuelve a arrastrar uno que aún
    // se está guardando, los días se suman a los que ya llevaba.
    const previo = pendienteMes?.desplazado === event ? pendienteMes : null;
    const original = previo?.original ?? event;
    const total = (previo?.dias ?? 0) + dias;
    const nuevo = desplazarDias(original, total);
    if (!nuevo) return;
    setPendienteMes({ original, desplazado: eventoDesplazado(original, total), dias: total });
    try {
      await onMoveResize(original, nuevo.start, nuevo.end);
    } catch {
      setPendienteMes(null);
      toast.notify({ tone: 'danger', title: t('cal.error.updateEvent') });
    }
  }

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
    // Días que ocuparía el evento si se soltara ahora: se resaltan como destino.
    const destino = (() => {
      if (!arrastreMes?.movido) return null;
      const nuevo = desplazarDias(arrastreMes.event, arrastreMes.destino - arrastreMes.origen);
      return nuevo ? barSpan(days, nuevo.start, nuevo.end) : null;
    })();
    // Por IDENTIDAD y no por href: las ocurrencias de una serie comparten href, y solo
    // queda en tenue la que se coge.
    const arrastrado = arrastreMes?.movido ? arrastreMes.event : null;
    return (
      <div
        class={cx('bg-surface flex h-full flex-col', arrastrado && 'cursor-grabbing select-none')}
        onClickCapture={(e) => {
          if (!esClicDeArrastre()) return;
          e.preventDefault();
          e.stopPropagation();
        }}
      >
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
        <div ref={rejillaMesRef} class="flex min-h-0 flex-1 flex-col">
          {weeks.map((weekSegs, w) => {
            // Filas de barras visibles de la semana (tope MAX_BAR_ROWS); el resto
            // cuenta como «+N más» por día. Los chips comparten el presupuesto.
            const { visible, hiddenByCol } = limitWeekRows(weekSegs, MAX_BAR_ROWS);
            const visibleRows = Math.min(rowCount(weekSegs), MAX_BAR_ROWS);
            // Hasta el pie de la última barra, sin la separación de debajo: esa ya la pone
            // el `mt-0.5` de las fichas, y contarla dos veces le quitaba 2 px a su sitio.
            const barsAreaH = visibleRows > 0 ? visibleRows * BAR_H - (BAR_H - BAR_INNER_H) : 0;
            const chipCap = Math.max(1, MAX_LANES - visibleRows);
            return (
              <div key={w} class="relative grid min-h-0 flex-1 grid-cols-7">
                {Array.from({ length: 7 }, (_, c) => {
                  const gi = w * 7 + c;
                  const day = days[gi]!;
                  const inMonth = day.getMonth() === cursor.getMonth();
                  const dayChips = chipsByDayIdx[gi]!;
                  const caben = fichasQueCaben(
                    altoSemana,
                    barsAreaH,
                    dayChips.length,
                    hiddenByCol[c]! > 0,
                  );
                  const shown = dayChips.slice(
                    0,
                    caben === null ? chipCap : Math.min(chipCap, caben),
                  );
                  const more = hiddenByCol[c]! + (dayChips.length - shown.length);
                  const esDestino = destino !== null && destino.start <= gi && gi <= destino.end;
                  return (
                    // La celda NO es un botón: dentro lleva el número del día, las
                    // píldoras y el «+N más», que son controles propios, y un control
                    // dentro de otro (axe `nested-interactive`) ni se anuncia bien ni
                    // deja pulsar lo de dentro sin disparar lo de fuera —el «+N más»
                    // abría «Crear evento»—. Crear es un botón DE FONDO que ocupa la
                    // celda entera; el contenido va encima sin recibir el puntero salvo
                    // en sus controles, así que pulsar el hueco vacío sigue creando.
                    <div
                      key={day.toISOString()}
                      class={cx(
                        'border-border relative flex min-h-0 flex-col overflow-hidden border-r border-b p-[6px]',
                        !inMonth && 'bg-bg-subtle/40',
                      )}
                      {...(esDestino ? { 'data-destino-arrastre': '' } : {})}
                    >
                      <button
                        type="button"
                        aria-label={t('nav.createEventOn', { date: dayMonthLabel(locale, day) })}
                        onClick={() => onCreateAt(day)}
                        class={cx(
                          'absolute inset-0 transition-colors',
                          'hover:bg-bg-subtle focus-visible:bg-bg-subtle focus-visible:outline-none',
                        )}
                      />
                      {esDestino ? (
                        <span
                          aria-hidden="true"
                          class="bg-primary/10 pointer-events-none absolute inset-0"
                        />
                      ) : null}
                      {esDestino && destino?.start === gi && arrastreMes ? (
                        <span
                          aria-hidden="true"
                          class={cx(
                            'border-primary bg-surface text-text pointer-events-none absolute inset-x-1 bottom-1 z-20',
                            'truncate rounded-sm border px-1.5 py-0.5 text-xs font-medium shadow-[var(--shadow-sm)]',
                          )}
                        >
                          {arrastreMes.event.summary || t('event.untitled')}
                        </span>
                      ) : null}
                      {/* Número de semana ISO en la esquina del lunes, como en la hoja
                          del teléfono (`MesCompacto`): visible el número, leído entero. */}
                      {vista?.numeroSemana && c === 0 ? (
                        <span
                          data-numero-semana=""
                          class="pointer-events-none absolute top-[6px] right-1.5"
                        >
                          <span
                            aria-hidden="true"
                            class="bg-bg-subtle text-text-subtle text-2xs grid h-5.5 min-w-5.5 place-items-center rounded-sm px-1 font-medium tabular-nums"
                          >
                            {isoWeek(day)}
                          </span>
                          <span class="sr-only">
                            {t('grid.weekNumber', { week: isoWeek(day) })}
                          </span>
                        </span>
                      ) : null}
                      <div class="pointer-events-none relative flex min-h-0 flex-col">
                        <NumeroDia locale={locale} day={day} inMonth={inMonth} onOpen={onOpenDay} />
                        {/* Reserva para el carril de barras (overlay), que se pinta encima. */}
                        {barsAreaH > 0 ? (
                          <span aria-hidden="true" style={{ height: `${barsAreaH}px` }} />
                        ) : null}
                        {/* Sin `overflow-hidden`: el «+N más» baja al relleno de la celda, que
                            ya recorta (`overflow-hidden` de la celda). */}
                        <div class="mt-0.5 flex min-w-0 flex-col gap-0.5">
                          {shown.map(({ event, date }) => (
                            <EventChip
                              locale={locale}
                              key={event.href}
                              event={event}
                              date={date}
                              color={color}
                              dosLineas={
                                more === 0 && fichasEnDosLineas(altoSemana, barsAreaH, shown.length)
                              }
                              onClick={seleccionarEnMes}
                              atenuado={event === arrastrado}
                              {...(event.readOnly
                                ? {}
                                : {
                                    onPointerDown: (e: PointerEvent) =>
                                      empezarArrastreMes(e, event),
                                  })}
                            />
                          ))}
                          {more > 0 ? (
                            <MasDelDia
                              locale={locale}
                              day={day}
                              more={more}
                              items={eventosDelDiaMes(weekSegs, c, dayChips)}
                              color={color}
                              onSelectEvent={onSelectEvent}
                              pegado={shown.length > 0}
                            />
                          ) : null}
                        </div>
                      </div>
                    </div>
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
                      onSelect={seleccionarEnMes}
                      atenuado={seg.item === arrastrado}
                      {...(seg.item.readOnly
                        ? {}
                        : { onPointerDown: (e: PointerEvent) => empezarArrastreMes(e, seg.item) })}
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
  const days = mode === 'week' ? diasDeSemana(cursor, vista?.finesDeSemana ?? true) : [cursor];
  return (
    <TimeGrid
      locale={locale}
      days={days}
      {...(vista
        ? {
            jornada: [vista.inicioLaboral, vista.finLaboral] as const,
            duracion: vista.duracion,
            numeroSemana: mode === 'week' && vista.numeroSemana,
          }
        : {})}
      events={events}
      accentColor={color}
      onSelectEvent={onSelectEvent}
      onCreateRange={onCreateRange}
      onMoveResize={onMoveResize}
    />
  );
}

// --- Celda del mes: número del día y «+N más» --------------------------------

/**
 * Todos los eventos de un día del mes, en el orden de la celda: primero las barras
 * (día completo o varios días) que lo cruzan, por su fila, y luego los chips con hora,
 * ya ordenados por inicio. Es la lista que despliega «+N más»: el contador cuenta los
 * ocultos, pero la lista los enseña TODOS para que el día se lea entero y en orden.
 */
export function eventosDelDiaMes(
  weekSegs: Segment<GridEvent>[],
  col: number,
  chips: MonthChip[],
): Positioned[] {
  const barras = weekSegs
    .filter((s) => s.col <= col && col < s.col + s.span)
    .sort((a, z) => a.row - z.row)
    .flatMap((s) => {
      const date = eventInstant(s.item.start, parseTzid(s.item.ical));
      return date ? [{ event: s.item, date }] : [];
    });
  return [...barras, ...chips.map(({ event, date }) => ({ event, date }))];
}

/**
 * El número del día. Con `onOpen` es un botón que abre el día en la vista Día —como
 * en cualquier calendario de escritorio, y la única salida a «ver ese día» sin
 * cambiar de vista a mano—; sin él, un rótulo. Mide 24×24 (`DATE_ROW_H`).
 */
function NumeroDia({
  locale,
  day,
  inMonth,
  onOpen,
}: {
  locale: Locale;
  day: Date;
  inMonth: boolean;
  onOpen?: ((day: Date) => void) | undefined;
}): JSX.Element {
  const t = useT(locale);
  const hoy = isToday(day);
  const clase = cx(
    'inline-flex h-[24px] w-[24px] items-center justify-center rounded-full text-sm tabular-nums',
    hoy
      ? 'bg-primary font-semibold text-[var(--sytue-on-primary)]'
      : inMonth
        ? 'text-text-muted'
        : 'text-text-subtle',
  );
  if (!onOpen) return <span class={clase}>{day.getDate()}</span>;
  return (
    <button
      type="button"
      aria-label={t('grid.openDay', { date: dayMonthLabel(locale, day) })}
      onClick={() => onOpen(day)}
      class={cx(
        clase,
        'sytue-focus-ring pointer-events-auto transition-colors duration-150',
        hoy ? 'hover:brightness-95' : 'hover:bg-surface-sunken hover:text-text',
      )}
    >
      {day.getDate()}
    </button>
  );
}

/**
 * «+N más»: un botón que despliega en un Popover TODOS los eventos del día, con la
 * fila de la agenda (`AgendaRow`). Pulsar una fila cierra la lista y abre su vista
 * rápida. Antes era un `<span title>` dentro del botón de la celda: pulsarlo creaba
 * un evento y los ocultos solo se veían pasando el ratón (en el móvil, nunca).
 */
function MasDelDia({
  locale,
  day,
  more,
  items,
  color,
  onSelectEvent,
  pegado,
}: {
  locale: Locale;
  day: Date;
  more: number;
  items: Positioned[];
  color: string;
  onSelectEvent: (event: GridEvent, anchor: DOMRect) => void;
  /** Hay fichas encima: va pegado a la última, sin el `gap-0.5` (ver MAS_H). */
  pegado: boolean;
}): JSX.Element {
  const t = useT(locale);
  const fecha = dayMonthLabel(locale, day);
  return (
    <div class={cx('pointer-events-auto -mb-[6px] flex min-w-0', pegado && '-mt-0.5')}>
      <Popover
        label={t('grid.dayEvents', { date: fecha, count: items.length })}
        trigger={(props) => (
          <button
            type="button"
            {...props}
            class={cx(
              'text-text-muted hover:text-text hover:bg-surface-sunken sytue-focus-ring',
              'max-w-full truncate rounded-sm px-1.5 text-left text-xs font-medium transition-colors',
              // 24 px de alto, como las fichas (WCAG 2.5.8): con la línea de `text-xs`
              // sola medía 16. Lo que crece no se le quita a las fichas: sube el `gap-0.5`
              // que lo separaba de la última (una ficha y él son objetivos contiguos, no
              // solapados) y baja al relleno de la celda (ver MAS_H).
              'h-6 py-1',
            )}
          >
            {t('grid.more', { count: more })}
          </button>
        )}
      >
        {(close) => (
          <div class="flex w-72 max-w-full flex-col gap-2">
            <div class="text-text text-sm font-semibold">{fecha}</div>
            <ul class="-mx-2 flex flex-col gap-0.5">
              {items.map((p) => (
                <li key={p.event.href} class="min-w-0">
                  <AgendaRow
                    locale={locale}
                    event={p.event}
                    date={p.date}
                    color={color}
                    onClick={(event, anchor) => {
                      close();
                      // La vista rápida se abre DESPUÉS de que la lista se haya ido:
                      // al cerrarse, el Popover devuelve el foco a «+N más», y si la
                      // vista rápida se abriera en el mismo commit ese foco se la
                      // robaría (y su Escape volvería a un botón que ya no está).
                      setTimeout(() => onSelectEvent(event, anchor), 0);
                    }}
                  />
                </li>
              ))}
            </ul>
          </div>
        )}
      </Popover>
    </div>
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
                    <li key={p.event.href} class="min-w-0">
                      <AgendaRow
                        locale={locale}
                        event={p.event}
                        date={p.date}
                        color={color}
                        onClick={onSelectEvent}
                      />
                    </li>
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
    <button
      type="button"
      onClick={(e) => {
        e.stopPropagation();
        onClick(event, e.currentTarget.getBoundingClientRect());
      }}
      class={cx(
        'flex w-full min-w-0 items-center gap-2.5 rounded-md border border-transparent px-2 py-1.5 text-left',
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
    </button>
  );
}

export function EventChip({
  locale,
  event,
  date,
  color,
  expanded,
  dosLineas,
  onClick,
  onPointerDown,
  atenuado,
}: {
  locale: Locale;
  event: GridEvent;
  date: Date;
  /** Color de reserva; prima el del calendario del evento. */
  color: string;
  expanded?: boolean;
  /**
   * La única ficha de su celda, sin barras encima: hay sitio para que el título siga en
   * una segunda línea en vez de cortarse a los diez caracteres («08:00 Reunión s…» a
   * 1280 px). La hora y el título van en el mismo párrafo, así que la segunda línea
   * empieza bajo la hora y no bajo el título.
   */
  dosLineas?: boolean | undefined;
  onClick: (e: GridEvent, anchor: DOMRect) => void;
  /** Empieza el arrastre a otro día (sin él, la píldora solo se abre). */
  onPointerDown?: ((e: PointerEvent) => void) | undefined;
  /** Es la que se está arrastrando: su sitio de antes queda en tenue. */
  atenuado?: boolean | undefined;
}): JSX.Element {
  // Un `<button>` de verdad: la celda del mes ya no es un botón, así que la píldora no
  // queda anidada en otro control y no necesita imitar el rol ni el teclado.
  return (
    <button
      type="button"
      onPointerDown={onPointerDown}
      onClick={(e) => {
        e.stopPropagation();
        onClick(event, e.currentTarget.getBoundingClientRect());
      }}
      class={cx(
        // Pildora sobre superficie con borde: el color del calendario va en el
        // punto (portador de color con texto AA garantizado sobre el borde).
        'border-border bg-bg-subtle text-text flex min-w-0 gap-1.5 rounded-sm border px-1.5 text-left text-xs',
        dosLineas ? 'items-start' : 'items-center',
        // En la celda del mes el contenido no recibe el puntero (cae al botón de
        // crear de fondo); la píldora sí.
        'pointer-events-auto',
        onPointerDown ? 'cursor-grab select-none active:cursor-grabbing' : 'cursor-pointer',
        'transition-[color,background-color,border-color,opacity] duration-150',
        atenuado && 'opacity-50',
        'hover:border-border-strong hover:bg-surface hover:shadow-[var(--shadow-xs)]',
        'sytue-focus-ring',
        expanded ? 'py-1' : 'py-0.5',
        // Objetivo de 24 px (WCAG 2.5.8): con `text-xs`, `py-0.5` y el borde medía 22.
        // En px y rem a la vez, como los botones: con la base de letra en 14 los 1,5rem
        // se quedarían en 21.
        'min-h-[max(24px,1.5rem)]',
      )}
      title={event.summary}
    >
      <span
        // En dos líneas el punto va con la primera (16 px de interlínea: 4 arriba).
        class={cx('h-2 w-2 shrink-0 rounded-full', dosLineas && 'mt-1')}
        style={{ backgroundColor: event.color ?? color }}
        aria-hidden="true"
      />
      {dosLineas ? (
        // Dos líneas solo con la ventana a 640 px de alto o más: por debajo, la fila del
        // mes ya no da para 38 px de ficha y la segunda línea saldría cortada a la mitad.
        <span class="min-w-0 truncate break-words [@media(min-height:40rem)]:line-clamp-2 [@media(min-height:40rem)]:whitespace-normal">
          <span class="text-text-muted me-1.5 hidden font-medium tabular-nums sm:inline">
            {timeLabel(locale, date)}
          </span>
          {event.summary}
        </span>
      ) : (
        <>
          {/* La hora de inicio, delante del título: en el mes es lo que distingue la
              reunión de las 9 de la de las 5 sin abrir ninguna. Solo desde `sm:`: en el
              teléfono la celda mide ~50 px y la hora se comería el título entero. */}
          <span
            class={cx(
              'text-text-muted shrink-0 font-medium tabular-nums',
              expanded ? undefined : 'hidden sm:inline',
            )}
          >
            {timeLabel(locale, date)}
          </span>
          <span class="truncate">{event.summary}</span>
        </>
      )}
    </button>
  );
}
