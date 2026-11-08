/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL MES COMPACTO DEL TELÉFONO, como datos. La pareja en Fyne es
 * `apps/fyne/calendario/mescompacto.go`, con los mismos vectores de test (V1–V14).
 *
 * - La semana empieza en lunes (`startOfWeek`) y su número es el ISO 8601 de ese lunes.
 * - Solo las filas que tocan el mes (4, 5 o 6); los días de otro mes son HUECO
 *   (`fecha: null`). Plegado, la semana del cursor va COMPLETA: los días del mes vecino
 *   llevan fecha, puntos y se pueden elegir (`otroMes`).
 * - Un evento cubre un día si `[inicio, fin)` lo solapa: fin exclusivo; sin fin, o con
 *   fin ≤ inicio, solo el día de inicio. Es `barSpan`, una pasada por evento.
 * - Orden de un día: primero los de día completo, luego por instante de inicio; si
 *   empatan, el de entrada. Los puntos son los colores de los `MAX_PUNTOS` primeros.
 *
 * Sin DOM ni Preact: lo pinta `MesCompacto.tsx`.
 */
import { barSpan } from './multiDayLayout.js';
import type { GridEvent } from './CalendarGrid.js';
import {
  addDays,
  eventInstant,
  isSameDay,
  isoWeek,
  parseTzid,
  startOfDay,
  startOfWeek,
  weekGrid,
} from './dates.js';

/** Puntos por día como mucho: el resto lo dice el nombre accesible (`eventos`). */
export const MAX_PUNTOS = 3;

export interface DiaCompacto {
  /** null = hueco (un día de otro mes, solo desplegado). */
  fecha: Date | null;
  hoy: boolean;
  elegido: boolean;
  /** Día del mes vecino en la semana plegada. */
  otroMes: boolean;
  /** Colores de los primeros `MAX_PUNTOS` eventos del día, en su orden. */
  puntos: string[];
  /** Todos los eventos que cubren el día. */
  eventos: number;
}

export interface SemanaCompacta {
  /** Número de semana ISO 8601 de su lunes. */
  numero: number;
  dias: DiaCompacto[];
}

/** Un evento del día, como lo pinta la lista bajo la rejilla. */
export interface FilaDia {
  event: GridEvent;
  inicio: Date;
  fin: Date | null;
  todoElDia: boolean;
  /** Con hora y acaba otro día que el de su inicio: la fila enseña las dos fechas. */
  cruza: boolean;
}

interface Resuelto {
  event: GridEvent;
  inicio: Date;
  fin: Date | null;
  todoElDia: boolean;
}

/** Los instantes del evento en la zona de quien mira (honra el TZID); null si no se leen. */
function resolver(event: GridEvent): Resuelto | null {
  const tz = parseTzid(event.ical);
  const inicio = eventInstant(event.start, tz);
  if (!inicio) return null;
  const fin = event.end ? eventInstant(event.end, tz) : null;
  return { event, inicio, fin, todoElDia: /^\d{8}$/.test(event.start) };
}

/** Día completo primero, luego por inicio; `sort` es estable, así que empata el de entrada. */
function ordenar(a: Resuelto, b: Resuelto): number {
  if (a.todoElDia !== b.todoElDia) return a.todoElDia ? -1 : 1;
  return a.inicio.getTime() - b.inicio.getTime();
}

/** Los eventos que cubre cada día de `dias`, ya ordenados. Una pasada por evento. */
function porDia(dias: Date[], events: GridEvent[]): Resuelto[][] {
  const cubos: Resuelto[][] = dias.map(() => []);
  for (const event of events) {
    const r = resolver(event);
    if (!r) continue;
    const sp = barSpan(dias, r.inicio, r.fin);
    if (!sp) continue;
    for (let i = sp.start; i <= sp.end; i++) cubos[i]!.push(r);
  }
  for (const c of cubos) c.sort(ordenar);
  return cubos;
}

function dia(
  fecha: Date,
  hoy: Date,
  cursor: Date,
  eventos: Resuelto[],
  accent: string,
  otroMes: boolean,
): DiaCompacto {
  return {
    fecha,
    hoy: isSameDay(fecha, hoy),
    elegido: isSameDay(fecha, cursor),
    otroMes,
    puntos: eventos.slice(0, MAX_PUNTOS).map((r) => r.event.color ?? accent),
    eventos: eventos.length,
  };
}

const HUECO: DiaCompacto = {
  fecha: null,
  hoy: false,
  elegido: false,
  otroMes: false,
  puntos: [],
  eventos: 0,
};

/** Los días (lun→dom) de las semanas que tocan el mes de `cursor`: 28, 35 o 42. */
export function diasDelMes(cursor: Date): Date[] {
  const inicio = startOfWeek(new Date(cursor.getFullYear(), cursor.getMonth(), 1));
  const fin = startOfWeek(new Date(cursor.getFullYear(), cursor.getMonth() + 1, 0));
  const semanas = Math.round((fin.getTime() - inicio.getTime()) / (7 * 86_400_000)) + 1;
  return Array.from({ length: semanas * 7 }, (_, i) => addDays(inicio, i));
}

/** El mes de `cursor` desplegado: sus semanas con número ISO, huecos y puntos. */
export function semanasCompactas(
  cursor: Date,
  hoy: Date,
  events: GridEvent[],
  accent: string,
): SemanaCompacta[] {
  const dias = diasDelMes(cursor);
  const cubos = porDia(dias, events);
  const semanas: SemanaCompacta[] = [];
  for (let w = 0; w < dias.length / 7; w++) {
    semanas.push({
      numero: isoWeek(dias[w * 7]!),
      dias: Array.from({ length: 7 }, (_, c) => {
        const i = w * 7 + c;
        const fecha = dias[i]!;
        return fecha.getMonth() === cursor.getMonth()
          ? dia(fecha, hoy, cursor, cubos[i]!, accent, false)
          : HUECO;
      }),
    });
  }
  return semanas;
}

/** Plegado: la semana del cursor completa, con los días del mes vecino elegibles. */
export function semanaPlegada(
  cursor: Date,
  hoy: Date,
  events: GridEvent[],
  accent: string,
): SemanaCompacta {
  const dias = weekGrid(cursor);
  const cubos = porDia(dias, events);
  return {
    numero: isoWeek(dias[0]!),
    dias: dias.map((fecha, i) =>
      dia(fecha, hoy, cursor, cubos[i]!, accent, fecha.getMonth() !== cursor.getMonth()),
    ),
  };
}

/** Los eventos que cubren `dia`, en el orden de la lista: día completo primero. */
export function eventosDelDia(dia: Date, events: GridEvent[]): FilaDia[] {
  const [cubo] = porDia([startOfDay(dia)], events);
  return cubo!.map((r) => {
    const ultimo =
      r.fin && r.fin.getTime() > r.inicio.getTime() ? new Date(r.fin.getTime() - 1) : r.inicio;
    return { ...r, cruza: !r.todoElDia && !isSameDay(r.inicio, ultimo) };
  });
}
