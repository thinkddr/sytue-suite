/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL MES COMPACTO DEL TELÉFONO: la vista de mes por debajo de `sm:` (640 de ventana).
 * La pareja en Fyne es `apps/fyne/calendario/vistamescompacta.go`.
 *
 * Por debajo de 640 la rejilla de escritorio encogida eran seis semanas de celdas de
 * ~55 px con píldoras ilegibles. Aquí, como el calendario del teléfono: las semanas que
 * tocan el mes con su número ISO, cada día con su número y hasta tres puntos de color,
 * el día elegido (que es el CURSOR de la app, no otro estado) relleno del primario, un
 * asa que pliega la rejilla a la semana elegida, y debajo la lista de ese día.
 *
 * Tocar un día lo ELIGE (no crea: crear es el «+» de la barra, sobre el día elegido).
 * Teclado: ←→ un día, ↑↓ una semana, Inicio/Fin lunes/domingo, RePág/AvPág un mes
 * (recortando el día). El foco sigue a la selección (tabindex itinerante), también
 * cuando la cambia un atajo global con un día enfocado.
 * Deslizar con el dedo sobre los días cambia de mes (plegado, de semana) con la regla
 * común `decidirDeslizamiento`; la columna de semana no desliza.
 *
 * La geometría no depende de los datos: mientras carga, la hoja es la misma, sin puntos,
 * y la lista enseña su esqueleto.
 */
import type { ComponentChildren, JSX } from 'preact';
import { useEffect, useId, useMemo, useRef } from 'preact/hooks';
import type { Locale } from '@sytue/i18n';
import {
  Button,
  Icon,
  SidebarItem,
  SidebarSection,
  Skeleton,
  cx,
  decidirDeslizamiento,
} from '@sytue/ui';
import { DEFAULT_ACCENT, type GridEvent } from './CalendarGrid.js';
import {
  addDays,
  addMonthsClamped,
  dayMonthLabel,
  fullDateLabel,
  monthYearLabel,
  startOfDay,
  startOfWeek,
  timeLabel,
  weekdayDayMonthLabel,
  weekdayLabels,
  type ViewMode,
} from './dates.js';
import {
  eventosDelDia,
  semanaPlegada,
  semanasCompactas,
  type FilaDia,
} from './modeloMesCompacto.js';
import { SelectorVista } from './SelectorVista.js';
import type { T } from '../i18n/index.js';

/**
 * Columna de semana (44 = canalón 16 + pastilla 22 + 6) y siete columnas iguales. La
 * misma plantilla en la fila de días y en cada semana, para que caigan en su columna.
 */
const COLUMNAS = 'grid grid-cols-[2.75rem_repeat(7,minmax(0,1fr))]';
/** El ancho de la columna de semana en rem: lo que no desliza. */
const COLUMNA_SEMANA_REM = 2.75;
/** Cargando: la misma referencia siempre, para no rehacer las semanas en cada render. */
const SIN_EVENTOS: GridEvent[] = [];

export interface MesCompactoProps {
  t: T;
  locale: Locale;
  /** El día elegido: el cursor de la app. Decide el mes (o la semana, plegado). */
  cursor: Date;
  hoy: Date;
  events: GridEvent[];
  accentColor?: string | undefined;
  /** Solo la semana del cursor. */
  plegado: boolean;
  onPlegar: () => void;
  /** Aún no hay datos que enseñar: hoja sin puntos y esqueleto en la lista. */
  cargando: boolean;
  /** Refrescando con datos previos: la lista atenuada y ocupada. */
  atenuado?: boolean;
  /** Error o sin permiso: va en el sitio de la lista, bajo la hoja. */
  panel?: ComponentChildren;
  onElegir: (dia: Date) => void;
  /** Un deslizamiento: 1 al siguiente, −1 al anterior (mes, o semana si está plegado). */
  onMoverMes: (delta: -1 | 1) => void;
  onSelectEvent: (event: GridEvent, anchor: DOMRect) => void;
}

/** Lo que dice la primera línea de una fila de la lista. */
function cuando(t: T, locale: Locale, f: FilaDia): string {
  if (f.todoElDia) return t('event.allDay');
  if (f.cruza && f.fin) {
    return t('when.rangeAcross', {
      startDate: dayMonthLabel(locale, f.inicio),
      startTime: timeLabel(locale, f.inicio),
      endDate: dayMonthLabel(locale, f.fin),
      endTime: timeLabel(locale, f.fin),
    });
  }
  if (f.fin && f.fin.getTime() > f.inicio.getTime()) {
    return t('when.timeRange', {
      start: timeLabel(locale, f.inicio),
      end: timeLabel(locale, f.fin),
    });
  }
  return timeLabel(locale, f.inicio);
}

/** El esqueleto de la lista: tres filas con la geometría de una de verdad. */
function EsqueletoListaDia(): JSX.Element {
  return (
    <ul aria-hidden="true">
      {[0, 1, 2].map((i) => (
        <li key={i} class="flex min-h-12 items-start gap-3 px-4 py-2">
          <Skeleton width="0.625rem" height="0.625rem" circle class="mt-1.5 shrink-0" />
          <span class="flex flex-col gap-1.5">
            <Skeleton width="5rem" height="0.875rem" />
            <Skeleton width="8rem" height="1rem" />
          </span>
        </li>
      ))}
    </ul>
  );
}

export function MesCompacto({
  t,
  locale,
  cursor,
  hoy,
  events,
  accentColor,
  plegado,
  onPlegar,
  cargando,
  atenuado = false,
  panel,
  onElegir,
  onMoverMes,
  onSelectEvent,
}: MesCompactoProps): JSX.Element {
  const accent = accentColor ?? DEFAULT_ACCENT;
  const base = useId();
  const idRejilla = `sytue-mes-${base}`;
  const idTitulo = `sytue-mes-dia-${base}`;
  const rejillaRef = useRef<HTMLDivElement>(null);
  const enfocarRef = useRef(false);
  const gestoRef = useRef<{ x: number; y: number; t: number } | null>(null);
  const anularClicRef = useRef(false);

  // Cargando no se pintan puntos: la geometría es la misma, sin datos que esperar.
  const visibles = cargando ? SIN_EVENTOS : events;
  const diaHoy = startOfDay(hoy).getTime();
  const semanas = useMemo(
    () =>
      plegado
        ? [semanaPlegada(cursor, new Date(diaHoy), visibles, accent)]
        : semanasCompactas(cursor, new Date(diaHoy), visibles, accent),
    [cursor, diaHoy, visibles, accent, plegado],
  );
  const filas = useMemo(() => eventosDelDia(cursor, events), [cursor, events]);

  // ¿Está el foco en un día? Lo anotan `focusin`/`focusout` de la rejilla. Hace falta
  // anotarlo porque cada día lleva `key={fecha}`: un atajo global (`n`, `p`, `t`) que
  // cambia de mes DESMONTA el botón enfocado, el foco cae en <body> y en el efecto ya no
  // se sabe de dónde venía. Un `focusout` sin `relatedTarget` puede ser ese desmontaje
  // (unos navegadores lo lanzan al quitar el nodo y otros no), así que solo se deja de
  // estar dentro cuando el foco se va a OTRO sitio con nombre.
  const focoDentroRef = useRef(false);
  function onFocusOut(e: FocusEvent): void {
    const a = e.relatedTarget as Node | null;
    if (a && !rejillaRef.current?.contains(a)) focoDentroRef.current = false;
  }

  // El foco sigue a la selección cuando el cambio vino del teclado, y también cuando
  // vino de fuera (un atajo, «Hoy», plegar) con el foco en un día: si no, se quedaba en
  // <body> o en el día viejo, ya fuera del recorrido, y las flechas dejaban de ir. Solo si
  // el foco sigue en la rejilla o se ha perdido (<body>): nunca se le quita a otro control.
  useEffect(() => {
    const porTeclado = enfocarRef.current;
    enfocarRef.current = false;
    const rejilla = rejillaRef.current;
    if (!rejilla) return;
    const activo = document.activeElement;
    const perdido = !activo || activo === document.body || rejilla.contains(activo);
    if (!porTeclado && !(focoDentroRef.current && perdido)) return;
    rejilla.querySelector<HTMLElement>('[aria-pressed="true"]')?.focus();
  }, [cursor, plegado]);

  function onKeyDown(e: KeyboardEvent): void {
    // Con modificador la tecla es de otro: Alt+← es «atrás» del navegador y Ctrl/⌘+Inicio
    // va al principio de la página. Sin esto se anulaban y movían la selección.
    if (e.altKey || e.ctrlKey || e.metaKey) return;
    let destino: Date | null = null;
    if (e.key === 'ArrowLeft') destino = addDays(cursor, -1);
    else if (e.key === 'ArrowRight') destino = addDays(cursor, 1);
    else if (e.key === 'ArrowUp') destino = addDays(cursor, -7);
    else if (e.key === 'ArrowDown') destino = addDays(cursor, 7);
    else if (e.key === 'Home') destino = startOfWeek(cursor);
    else if (e.key === 'End') destino = addDays(startOfWeek(cursor), 6);
    else if (e.key === 'PageUp') destino = addMonthsClamped(cursor, -1);
    else if (e.key === 'PageDown') destino = addMonthsClamped(cursor, 1);
    if (!destino) return;
    e.preventDefault();
    enfocarRef.current = true;
    onElegir(destino);
  }

  // Deslizar: solo dedo o lápiz, y solo si empieza sobre los días (la columna de semana
  // no desliza). El `click` que llega al soltar sobre un día no lo elige.
  function anchoColumnaSemana(): number {
    const raiz =
      typeof getComputedStyle === 'function'
        ? parseFloat(getComputedStyle(document.documentElement).fontSize)
        : NaN;
    return COLUMNA_SEMANA_REM * (Number.isFinite(raiz) && raiz > 0 ? raiz : 16);
  }
  function onPointerDown(e: PointerEvent): void {
    anularClicRef.current = false;
    gestoRef.current = null;
    if (e.pointerType === 'mouse') return;
    const caja = (e.currentTarget as HTMLElement).getBoundingClientRect();
    if (e.clientX - caja.left < anchoColumnaSemana()) return;
    gestoRef.current = { x: e.clientX, y: e.clientY, t: e.timeStamp };
  }
  function onPointerUp(e: PointerEvent): void {
    const g = gestoRef.current;
    gestoRef.current = null;
    if (!g) return;
    const caja = (e.currentTarget as HTMLElement).getBoundingClientRect();
    const ancho = Math.max(0, caja.width - anchoColumnaSemana());
    const r = decidirDeslizamiento(e.clientX - g.x, e.clientY - g.y, ancho, e.timeStamp - g.t);
    if (r === 0) return;
    anularClicRef.current = true;
    onMoverMes(r);
  }
  function onClickCapture(e: MouseEvent): void {
    if (!anularClicRef.current) return;
    anularClicRef.current = false;
    e.preventDefault();
    e.stopPropagation();
  }

  const titulo = fullDateLabel(locale, cursor);
  return (
    <div class="bg-surface flex min-h-full flex-col">
      {/* La hoja de la rejilla, sobre la lista con una sombra que las separa. */}
      <div class="bg-surface relative z-10 shadow-[var(--shadow-sm)]">
        <div aria-hidden="true" class={cx(COLUMNAS, 'h-6 items-center')}>
          <span />
          {weekdayLabels(locale).map((w) => (
            <span key={w} class="text-text-subtle truncate text-center text-sm">
              {w}
            </span>
          ))}
        </div>
        {/* El grupo DELEGA: las teclas burbujean desde el día enfocado (un <button>) y el
            deslizar es un gesto sobre toda la rejilla, no un control más. Cada día sigue
            siendo un botón accesible por sí mismo. */}
        {/* oxlint-disable-next-line jsx-a11y/no-noninteractive-element-interactions */}
        <div
          id={idRejilla}
          ref={rejillaRef}
          role="group"
          aria-label={monthYearLabel(locale, cursor)}
          class="touch-pan-y"
          onKeyDown={onKeyDown}
          onFocusIn={() => (focoDentroRef.current = true)}
          onFocusOut={onFocusOut}
          onPointerDown={onPointerDown}
          onPointerUp={onPointerUp}
          onPointerCancel={() => (gestoRef.current = null)}
          onClickCapture={onClickCapture}
        >
          {semanas.map((w) => (
            <div
              key={`${w.numero}-${w.dias.find((d) => d.fecha)?.fecha?.getTime()}`}
              class={cx(COLUMNAS, 'h-10')}
            >
              <div class="flex justify-end pt-[5px] pr-1.5">
                <span
                  aria-hidden="true"
                  class="bg-bg-subtle text-text-subtle text-2xs grid h-5.5 w-5.5 place-items-center rounded-sm font-medium tabular-nums"
                >
                  {w.numero}
                </span>
                <span class="sr-only">{t('grid.weekNumber', { week: w.numero })}</span>
              </div>
              {w.dias.map((d, c) => {
                if (!d.fecha) return <span key={`h${c}`} aria-hidden="true" />;
                const fecha = d.fecha;
                const nombre = weekdayDayMonthLabel(locale, fecha);
                return (
                  <button
                    key={fecha.getTime()}
                    type="button"
                    tabIndex={d.elegido ? 0 : -1}
                    aria-pressed={d.elegido}
                    aria-current={d.hoy ? 'date' : undefined}
                    aria-label={
                      d.eventos > 0
                        ? t('grid.dayEvents', { date: nombre, count: d.eventos })
                        : nombre
                    }
                    onClick={() => onElegir(fecha)}
                    class="sytue-focus-ring flex min-w-0 flex-col items-center rounded-md pt-0.5 focus-visible:outline-none"
                  >
                    <span
                      class={cx(
                        'text-md grid h-7 w-7 place-items-center rounded-md tabular-nums',
                        d.elegido
                          ? cx(
                              'bg-primary text-[var(--sytue-on-primary)]',
                              d.hoy && 'font-semibold',
                            )
                          : d.hoy
                            ? 'text-primary-text font-semibold'
                            : d.otroMes
                              ? 'text-text-subtle'
                              : 'text-text',
                      )}
                    >
                      {fecha.getDate()}
                    </span>
                    <span aria-hidden="true" class="mt-0.5 flex h-1.25 gap-[3px]">
                      {d.puntos.map((color, i) => (
                        <span
                          key={i}
                          data-punto
                          class="h-1.25 w-1.25 rounded-full"
                          style={{ backgroundColor: color }}
                        />
                      ))}
                    </span>
                  </button>
                );
              })}
            </div>
          ))}
        </div>
        <button
          type="button"
          aria-expanded={!plegado}
          aria-controls={idRejilla}
          aria-label={plegado ? t('grid.showMonth') : t('grid.showWeek')}
          title={plegado ? t('grid.showMonth') : t('grid.showWeek')}
          onClick={onPlegar}
          class="sytue-focus-ring flex h-8 w-full items-center justify-center focus-visible:outline-none"
        >
          <span aria-hidden="true" class="bg-border-strong rounded-pill h-1 w-9" />
        </button>
      </div>

      {panel ?? (
        <section
          aria-labelledby={idTitulo}
          aria-busy={cargando || atenuado || undefined}
          class={cx('flex-1 pb-4 transition-opacity duration-150', atenuado && 'opacity-60')}
        >
          <h2 id={idTitulo} class="text-text text-md px-4 pt-4 pb-2 font-semibold">
            {titulo}
          </h2>
          {cargando ? (
            <>
              <span class="sr-only">{t('grid.loading')}</span>
              <EsqueletoListaDia />
            </>
          ) : filas.length === 0 ? (
            <p class="text-text-muted px-4 py-2 text-sm">{t('day.noEvents')}</p>
          ) : (
            <ul>
              {filas.map((f) => (
                <li key={`${f.event.href}|${f.event.start}`}>
                  <button
                    type="button"
                    onClick={(e) => onSelectEvent(f.event, e.currentTarget.getBoundingClientRect())}
                    class="hover:bg-bg-subtle sytue-focus-ring flex min-h-12 w-full items-start gap-3 px-4 py-2 text-left transition-colors duration-150 focus-visible:outline-none"
                  >
                    <span
                      aria-hidden="true"
                      class="mt-[5px] h-2.5 w-2.5 shrink-0 rounded-full"
                      style={{ backgroundColor: f.event.color ?? accent }}
                    />
                    <span class="min-w-0 flex-1">
                      <span class="text-text-muted block text-sm tabular-nums">
                        {cuando(t, locale, f)}
                      </span>
                      <span class="text-text text-md block truncate">
                        {f.event.summary || t('event.untitled')}
                      </span>
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </section>
      )}
    </div>
  );
}

/**
 * Las acciones de la barra móvil en el mes del teléfono: «Hoy» con forma de hoja de
 * calendario y el número de HOY dentro, y «+» para crear en el día elegido.
 */
export function AccionesBarraCompacta({
  t,
  locale,
  hoy,
  cursor,
  offline,
  onHoy,
  onCrear,
}: {
  t: T;
  locale: Locale;
  hoy: Date;
  cursor: Date;
  offline: boolean;
  onHoy: () => void;
  onCrear: () => void;
}): JSX.Element {
  return (
    <>
      {/* La etiqueta visible («6») va dentro del nombre accesible (WCAG 2.5.3). */}
      <Button
        variant="ghost"
        size="sm"
        iconOnly
        aria-label={t('grid.todayLabel', { date: dayMonthLabel(locale, hoy) })}
        title={t('header.shortcut', { label: t('header.today'), key: 'T' })}
        onClick={onHoy}
      >
        <span
          aria-hidden="true"
          class="text-2xs grid h-5 w-5 place-items-center rounded-sm border-[1.5px] border-current leading-none font-semibold tabular-nums"
        >
          {hoy.getDate()}
        </span>
      </Button>
      <Button
        variant="ghost"
        size="sm"
        iconOnly
        aria-label={t('nav.createEventOn', { date: dayMonthLabel(locale, cursor) })}
        title={t('header.shortcut', { label: t('nav.createEvent'), key: 'C' })}
        disabled={offline}
        onClick={onCrear}
      >
        <Icon name="plus" size={20} />
      </Button>
    </>
  );
}

/**
 * El primer bloque del cajón en el mes del teléfono: lo que la cabecera de la vista
 * tenía y en la barra no cabe. Vista, buscar, recordatorios (la rama con su panel) e
 * invitaciones con su contador.
 */
export function BloqueVistaMovil({
  t,
  mode,
  onMode,
  onBuscar,
  recordatorios,
  invitaciones,
  onInvitaciones,
}: {
  t: T;
  mode: ViewMode;
  onMode: (m: ViewMode) => void;
  onBuscar: () => void;
  /** `ReminderCenter disparador="fila"`: una rama del menú, ya montada por la app. */
  recordatorios: ComponentChildren;
  invitaciones: number;
  onInvitaciones: () => void;
}): JSX.Element {
  return (
    <SidebarSection title={t('view.label')}>
      <li class="list-none px-1 pb-1.5">
        <SelectorVista t={t} mode={mode} onChange={onMode} rejilla />
      </li>
      <SidebarItem
        icon={<Icon name="search" size={18} />}
        label={t('search.events')}
        onClick={onBuscar}
      />
      {recordatorios}
      <SidebarItem
        icon={<Icon name="users" size={18} />}
        label={t('header.invitations')}
        count={invitaciones}
        onClick={onInvitaciones}
      />
    </SidebarSection>
  );
}
