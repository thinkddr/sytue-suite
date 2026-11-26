/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Coincide con…»: los eventos ya cargados que se pisan con el que se está creando o
 * editando. Es un aviso, no un veto: dos reuniones a la vez pasan, y la persona decide.
 * Lo que se quiere evitar es descubrirlo después, en la rejilla.
 *
 * Se mira solo lo que la vista ya tiene (los calendarios visibles del rango que se ve):
 * pedir el free/busy al servicio en cada tecla del formulario sería una petición por
 * pulsación para un aviso. Fuera de lo cargado no se avisa, que es lo honrado: el aviso
 * dice «coincide», nunca «no coincide con nada».
 *
 * No cuentan, por la misma regla que el free/busy del servicio (`CountsAsBusy`): los de
 * día completo (un cumpleaños no choca con una reunión), los marcados «disponible»
 * (TRANSP:TRANSPARENT), los que he rechazado y el propio evento (su UID, en cualquiera
 * de sus ocurrencias).
 *
 * Fyne aplica la misma regla en `apps/fyne/calendario/solapes.go`.
 */
import { eventInstant, parseTzid } from '../components/dates.js';
import type { GridEvent } from '../components/CalendarGrid.js';
import { miRespuesta, parseTransp } from './ical.js';

export interface Solape {
  evento: GridEvent;
  inicio: Date;
  fin: Date;
}

/**
 * Los eventos de `eventos` que se pisan con [inicio, fin), ordenados por inicio. Un
 * tramo vacío o al revés no se pisa con nada.
 */
export function solapes(
  eventos: readonly GridEvent[],
  inicio: Date,
  fin: Date,
  opciones: { uid?: string | undefined; correo?: string | undefined } = {},
): Solape[] {
  const desde = inicio.getTime();
  const hasta = fin.getTime();
  if (!(hasta > desde)) return [];
  const vistos = new Set<string>();
  const out: Solape[] = [];
  for (const ev of eventos) {
    if (opciones.uid && ev.uid === opciones.uid) continue;
    if (/^\d{8}$/.test(ev.start)) continue;
    if (parseTransp(ev.ical) === 'TRANSPARENT') continue;
    if (miRespuesta(ev.ical, opciones.correo) === 'declined') continue;
    const tz = ev.ical ? parseTzid(ev.ical) : null;
    const ini = eventInstant(ev.start, tz);
    const end = ev.end ? eventInstant(ev.end, tz) : null;
    if (!ini || !end) continue;
    if (!(ini.getTime() < hasta && end.getTime() > desde)) continue;
    // Una ocurrencia por evento y hora: el mismo evento en dos calendarios visibles
    // (el mío y el compartido de quien me invitó) no se cuenta dos veces.
    const clave = `${ev.uid}|${ini.getTime()}`;
    if (vistos.has(clave)) continue;
    vistos.add(clave);
    out.push({ evento: ev, inicio: ini, fin: end });
  }
  return out.sort((a, b) => a.inicio.getTime() - b.inicio.getTime());
}
