/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Lo que Calendar recuerda de quien mira: la vista (mes, semana, día o agenda) y los
 * calendarios que ha OCULTADO. Es una preferencia de la persona en este navegador, no
 * un dato que ven los demás: va a `localStorage` y no al servicio, como el orden de
 * Notepads (`sytue.notes.orden`).
 *
 * La clave lleva el `id` de la persona (`sytue.calendar.<sub>.vista`): en un navegador
 * donde entran dos cuentas, lo que una oculta no se lo encuentra oculto la otra. Se
 * guardan los OCULTOS y no los visibles para que un calendario nuevo, o uno que alguien
 * acaba de compartir, aparezca encendido. Sin almacenamiento (modo privado, cuota
 * llena) todo funciona como siempre: mes y todos visibles.
 *
 * Fyne guarda lo mismo con `sesion.PorPersona` (apps/fyne/calendario/preferencias.go).
 */
import type { ViewMode } from '../components/dates.js';
import { miRespuesta } from './ical.js';

const VISTAS: readonly ViewMode[] = ['month', 'week', 'day', 'agenda'];
/** Tope de URLs ocultas guardadas: las de calendarios borrados no deben crecer sin fin. */
const MAX_OCULTOS = 200;

function clave(userId: string, nombre: 'vista' | 'ocultos' | 'ajustes'): string {
  return `sytue.calendar.${userId}.${nombre}`;
}

function leer(k: string): string | null {
  try {
    return typeof localStorage === 'undefined' ? null : localStorage.getItem(k);
  } catch {
    return null;
  }
}

function escribir(k: string, v: string | null): void {
  try {
    if (typeof localStorage === 'undefined') return;
    if (v === null) localStorage.removeItem(k);
    else localStorage.setItem(k, v);
  } catch {
    // Sin almacenamiento la preferencia vale para esta sesión y ya.
  }
}

/** La vista guardada de esta persona, o null si no hay (o lo guardado no es una vista). */
export function leerVista(userId: string): ViewMode | null {
  if (!userId) return null;
  const v = leer(clave(userId, 'vista'));
  return VISTAS.includes(v as ViewMode) ? (v as ViewMode) : null;
}

export function guardarVista(userId: string, vista: ViewMode): void {
  if (userId) escribir(clave(userId, 'vista'), vista);
}

/** Las URLs de los calendarios que esta persona ocultó. Lo ilegible cuenta como nada. */
export function leerOcultos(userId: string): ReadonlySet<string> {
  if (!userId) return new Set();
  try {
    const datos: unknown = JSON.parse(leer(clave(userId, 'ocultos')) ?? '[]');
    return new Set(Array.isArray(datos) ? datos.filter((u) => typeof u === 'string') : []);
  } catch {
    return new Set();
  }
}

/**
 * Guarda los ocultos: los calendarios conocidos que no están visibles, más los ya
 * guardados que esta carga no trajo (un «compartido conmigo» que falló al listarse no
 * debe olvidar que estaba oculto).
 */
export function guardarOcultos(
  userId: string,
  conocidos: Iterable<string>,
  visibles: ReadonlySet<string>,
): void {
  if (!userId) return;
  const lista = [...conocidos];
  const vistos = new Set(lista);
  const ocultos = [
    ...lista.filter((u) => !visibles.has(u)),
    ...[...leerOcultos(userId)].filter((u) => !vistos.has(u)),
  ].slice(0, MAX_OCULTOS);
  escribir(clave(userId, 'ocultos'), ocultos.length ? JSON.stringify(ocultos) : null);
}

/**
 * Los visibles tras cargar la lista. En la primera carga, todos menos los que la
 * persona tenía ocultos; después se respeta la selección y se encienden solo los
 * recién llegados (creados o compartidos), aunque una vez se ocultaran.
 */
export function visiblesTrasCarga(
  todos: readonly string[],
  antes: ReadonlySet<string>,
  conocidos: ReadonlySet<string>,
  ocultos: ReadonlySet<string>,
): Set<string> {
  const primera = conocidos.size === 0;
  const next = new Set<string>();
  for (const u of todos) {
    if (primera ? !ocultos.has(u) : antes.has(u) || !conocidos.has(u)) next.add(u);
  }
  return next;
}

// --- Ajustes de Calendar ------------------------------------------------------

/**
 * Los ajustes de la persona: cómo nace un evento nuevo (duración y recordatorio) y qué
 * enseña el calendario (franja laboral, fines de semana, número de semana). Como la
 * vista, son de quien mira y de este navegador: la suite no tiene preferencias de
 * usuario en el servidor y Calendar ya guarda aquí las suyas.
 *
 * Fyne guarda el MISMO JSON (apps/fyne/calendario/ajustes.go): los nombres de campo
 * son los mismos en las dos plataformas.
 */
export interface AjustesCalendario {
  /** Minutos que dura un evento nuevo creado con un clic. */
  duracion: number;
  /** Minutos antes del evento del recordatorio por defecto; null, ninguno. */
  recordatorio: number | null;
  /** Hora (0–23) en la que empieza la jornada: encuadre de la rejilla y hora por defecto. */
  inicioLaboral: number;
  /** Hora (1–24) en la que acaba la jornada; siempre mayor que el inicio. */
  finLaboral: number;
  /** Con false, la semana enseña solo de lunes a viernes. */
  finesDeSemana: boolean;
  /** Número de semana ISO en el mes y la semana. */
  numeroSemana: boolean;
  /**
   * Con false, los eventos a los que he dicho que no salen de la rejilla y de la
   * agenda (siguen guardados, y en la búsqueda). Por defecto se ven, como hasta ahora.
   */
  rechazados: boolean;
}

/** Lo de siempre: una hora, sin recordatorio, de 9 a 18, la semana entera, sin número y
 * con los rechazados a la vista. */
export const AJUSTES_POR_DEFECTO: Readonly<AjustesCalendario> = {
  duracion: 60,
  recordatorio: null,
  inicioLaboral: 9,
  finLaboral: 18,
  finesDeSemana: true,
  numeroSemana: false,
  rechazados: true,
};

/** Las duraciones que se ofrecen; una guardada que no esté aquí vuelve a la de siempre. */
export const DURACIONES: readonly number[] = [15, 30, 45, 60, 90, 120];
/** Los recordatorios que se ofrecen (minutos antes), los mismos atajos que el editor. */
export const RECORDATORIOS: readonly number[] = [0, 5, 10, 15, 30, 60, 1440];

function horaValida(v: unknown, min: number, max: number): v is number {
  return typeof v === 'number' && Number.isInteger(v) && v >= min && v <= max;
}

/**
 * Normaliza lo leído campo a campo: un campo roto vuelve al valor de siempre sin
 * arrastrar a los demás, y una franja al revés (fin antes que inicio) se descarta
 * entera, porque no hay forma buena de adivinar cuál de las dos horas sobra.
 */
export function normalizarAjustes(datos: unknown): AjustesCalendario {
  const d = (datos && typeof datos === 'object' ? datos : {}) as Record<string, unknown>;
  const base = AJUSTES_POR_DEFECTO;
  let inicio = horaValida(d['inicioLaboral'], 0, 23) ? d['inicioLaboral'] : base.inicioLaboral;
  let fin = horaValida(d['finLaboral'], 1, 24) ? d['finLaboral'] : base.finLaboral;
  if (fin <= inicio) {
    inicio = base.inicioLaboral;
    fin = base.finLaboral;
  }
  const rec = d['recordatorio'];
  return {
    duracion: DURACIONES.includes(d['duracion'] as number)
      ? (d['duracion'] as number)
      : base.duracion,
    recordatorio: RECORDATORIOS.includes(rec as number) ? (rec as number) : null,
    inicioLaboral: inicio,
    finLaboral: fin,
    finesDeSemana:
      typeof d['finesDeSemana'] === 'boolean' ? d['finesDeSemana'] : base.finesDeSemana,
    numeroSemana: typeof d['numeroSemana'] === 'boolean' ? d['numeroSemana'] : base.numeroSemana,
    rechazados: typeof d['rechazados'] === 'boolean' ? d['rechazados'] : base.rechazados,
  };
}

/** Los ajustes de esta persona; sin nada guardado (o ilegible), los de siempre. */
export function leerAjustes(userId: string): AjustesCalendario {
  if (!userId) return { ...AJUSTES_POR_DEFECTO };
  try {
    return normalizarAjustes(JSON.parse(leer(clave(userId, 'ajustes')) ?? 'null'));
  } catch {
    return { ...AJUSTES_POR_DEFECTO };
  }
}

export function guardarAjustes(userId: string, ajustes: AjustesCalendario): void {
  if (userId) escribir(clave(userId, 'ajustes'), JSON.stringify(normalizarAjustes(ajustes)));
}

/**
 * Si un evento se queda fuera de la vista por «Mostrar los eventos que he rechazado»:
 * solo con el ajuste apagado y solo si MI respuesta es «no» (la del organizador o la
 * de otros invitados no cuenta). Fyne aplica lo mismo en `calendario.OcultoPorRechazo`.
 */
export function ocultoPorRechazo(
  ajustes: Pick<AjustesCalendario, 'rechazados'>,
  ical: string | undefined,
  correo: string | undefined,
): boolean {
  return !ajustes.rechazados && miRespuesta(ical, correo) === 'declined';
}
