/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Recordatorios de nota: las repeticiones que ofrece la interfaz y el paso entre el
 * valor del control nativo (`datetime-local` / `date`, hora de pared) y el instante
 * que guarda el servicio. Lo mismo, en Go, en apps/fyne/apps/notes/recordatorios.go.
 */
import type { Reminder, ReminderInput } from '@sytue/api-clients';

/** Las repeticiones del selector. `custom` es una RRULE que no es de estas: se respeta. */
export const REPETICIONES = ['none', 'daily', 'weekdays', 'weekly', 'monthly', 'flexible'] as const;
export type Repeticion = (typeof REPETICIONES)[number] | 'custom';

const RRULE: Partial<Record<Repeticion, string>> = {
  daily: 'FREQ=DAILY',
  weekdays: 'FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR',
  weekly: 'FREQ=WEEKLY',
  monthly: 'FREQ=MONTHLY',
};

/** La hora a la que avisa un recordatorio «todo el día»: las 9 de su día. */
export const HORA_TODO_EL_DIA = 9;

/** Qué repetición es un recordatorio guardado. */
export function repeticionDe(r: Pick<Reminder, 'rrule' | 'after_days'>): Repeticion {
  if ((r.after_days ?? 0) > 0) return 'flexible';
  if (!r.rrule) return 'none';
  const hit = Object.entries(RRULE).find(([, v]) => v === r.rrule);
  return hit ? (hit[0] as Repeticion) : 'custom';
}

const dos = (n: number) => String(n).padStart(2, '0');

/** El valor del control nativo para un instante, en la zona del navegador. */
export function valorLocal(iso: string, todoElDia: boolean): string {
  const d = new Date(iso);
  const dia = `${d.getFullYear()}-${dos(d.getMonth() + 1)}-${dos(d.getDate())}`;
  return todoElDia ? dia : `${dia}T${dos(d.getHours())}:${dos(d.getMinutes())}`;
}

/** El instante de un valor del control nativo; `null` si no es una fecha. */
export function instanteDe(valor: string, todoElDia: boolean): Date | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})(?:T(\d{2}):(\d{2}))?$/.exec(valor);
  if (!m) return null;
  const n = (i: number) => Number(m[i]);
  const out = todoElDia
    ? new Date(n(1), n(2) - 1, n(3), HORA_TODO_EL_DIA, 0)
    : new Date(n(1), n(2) - 1, n(3), n(4), n(5));
  return Number.isNaN(out.getTime()) ? null : out;
}

/** Lo que se manda al servicio. `rruleOriginal` conserva una `custom`. */
export function entradaRecordatorio(
  valor: string,
  todoElDia: boolean,
  rep: Repeticion,
  dias: number,
  rruleOriginal = '',
  tzid = Intl.DateTimeFormat().resolvedOptions().timeZone,
): ReminderInput | null {
  const cuando = instanteDe(valor, todoElDia);
  if (!cuando) return null;
  const flexible = rep === 'flexible';
  if (flexible && !(dias >= 1 && dias <= 365)) return null;
  return {
    due_at: cuando.toISOString(),
    all_day: todoElDia,
    rrule: rep === 'custom' ? rruleOriginal : (RRULE[rep] ?? ''),
    after_days: flexible ? Math.round(dias) : 0,
    tzid: tzid || 'UTC',
  };
}

/** Si ya pasó: una tarea que repite y sonó sin hacerse. */
export function yaPaso(r: Pick<Reminder, 'due_at'>, ahora = Date.now()): boolean {
  return Date.parse(r.due_at) < ahora;
}

/**
 * El rótulo corto del indicador de la fila: «3 oct, 09:30», o solo el día. Día y hora
 * por separado y unidos con coma, como `FormatDayMonth` + `FormatTimeDigits` en Fyne:
 * el formato combinado de Intl cambia el separador según el idioma.
 */
export function rotuloRecordatorio(r: Reminder, locale: string): string {
  const d = new Date(r.due_at);
  const dia = new Intl.DateTimeFormat(locale, { day: 'numeric', month: 'short' }).format(d);
  if (r.all_day) return dia;
  const hora = new Intl.DateTimeFormat(locale, { hour: '2-digit', minute: '2-digit' }).format(d);
  return `${dia}, ${hora}`;
}
