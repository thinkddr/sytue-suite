/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Cuándo una factura está VENCIDA: emitida, sin cobrar ni pagar, y con su vencimiento ya
 * pasado. La lista enseñaba «Confirmado» igual para la que vence mañana que para la que
 * lleva tres meses sin cobrarse, y lo segundo es lo que se mira primero.
 *
 * Los estados abiertos son los del servicio (`store.EstadosAbiertos`): los mismos que
 * «Pendiente de cobro» de la portada. Fyne aplica la misma regla
 * (`apps/fyne/apps/erp/vencimiento.go`, mismos nombres).
 */
import type { Registro } from './tipos.js';

/** Los estados de una factura emitida y aún por cobrar o pagar. */
export const ESTADOS_ABIERTOS: readonly string[] = ['confirmado', 'emitida', 'pendiente'];

/** «2026-11-25» → la medianoche local de ese día; null si no es una fecha. */
function dia(iso: string): Date | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(iso);
  return m ? new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3])) : null;
}

/**
 * Cuántos días lleva vencido un documento, o 0 si no lo está (sin vencimiento, no
 * abierto, o vence hoy o más tarde). Se cuentan días de calendario en el huso de quien
 * mira: el día del vencimiento aún no es «vencido».
 */
export function diasVencida(r: Registro, hoy: Date = new Date()): number {
  if (!r.estado || !ESTADOS_ABIERTOS.includes(r.estado)) return 0;
  const v = r.data['vencimiento'];
  const fecha = typeof v === 'string' ? dia(v) : null;
  if (!fecha) return 0;
  const hoyDia = new Date(hoy.getFullYear(), hoy.getMonth(), hoy.getDate());
  const dias = Math.round((hoyDia.getTime() - fecha.getTime()) / 86_400_000);
  return Math.max(dias, 0);
}
