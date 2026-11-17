/**
 * Las ocurrencias que pinta la web, sacadas del motor de recurrencias del servicio
 * Go (`GET /calendars/{id}/instances`), el mismo que usa Sytue Calendar en Fyne.
 *
 * Antes la web expandía las RRULE por su cuenta (`expandOccurrences`) con un motor
 * parcial: sin BYDAY, BYMONTHDAY, YEARLY ni RDATE, y un «cada mes el 31» que caía en
 * el 1–3 del mes siguiente. Eran dos calendarios distintos para la misma persona.
 *
 * El servicio da las fechas; el `REPORT` CalDAV da lo que la web necesita para
 * editar (href, etag e iCal del recurso). Aquí se casan por UID: cada instancia sale
 * como un clon de su maestro con SUS fechas, que es lo que ya esperaban la rejilla y
 * los flujos «solo este / este y siguientes / todos».
 */
import type { CalendarEvent } from '@sytue/api-clients';
import { parseTzid, toIcalUtc } from '../components/dates.js';
import { wallClockIcal } from '../components/tz.js';

/** Lo que devuelve el servicio por instancia (domain.Instance), solo lo que se lee. */
export interface InstanciaServicio {
  event: {
    uid: string;
    summary?: string;
    description?: string;
    location?: string;
    all_day?: boolean;
  };
  start: string;
  end?: string;
  /** Viene de un VEVENT con RECURRENCE-ID (una repetición cambiada en otro cliente). */
  is_override?: boolean;
}

/**
 * Una fecha del servicio en la MISMA forma que el DTSTART del maestro, para que el
 * resto de la web la lea como siempre:
 *   - día completo → `yyyymmdd`, con la fecha UTC del instante (el servicio guarda
 *     los días completos a medianoche UTC; Fyne los lee igual, `ocurrenciaInstantes`);
 *   - con TZID → la hora de pared en esa zona, sin Z: `eventInstant` la sitúa con el
 *     TZID y el formulario la enseña tal cual (en UTC, editar desde otra zona movía
 *     la hora);
 *   - UTC → UTC con Z, el instante que ha calculado el servicio;
 *   - flotante (sin Z ni TZID) → sin Z, que la web lee en la hora local como siempre.
 *     El servicio guarda la hora flotante como si fuera UTC, así que su reloj UTC es
 *     la hora escrita en el .ics; con Z, un «10:00» flotante se pintaba a las 11:00
 *     en Madrid, también en los eventos que no se repiten.
 */
type Forma = 'dia' | 'utc' | 'flotante' | { tz: string };

function comoElMaestro(iso: string, forma: Forma): string | null {
  const at = new Date(iso);
  if (Number.isNaN(at.getTime())) return null;
  if (forma === 'dia') return toIcalUtc(at).slice(0, 8);
  if (forma === 'utc') return toIcalUtc(at);
  if (forma === 'flotante') return toIcalUtc(at).slice(0, 15);
  try {
    return wallClockIcal(at, forma.tz);
  } catch {
    return toIcalUtc(at); // TZID que este motor no conoce: el instante, que no miente
  }
}

function formaDe(maestro: CalendarEvent): Forma {
  if (/^\d{8}$/.test(maestro.start)) return 'dia';
  if (maestro.start.endsWith('Z')) return 'utc';
  const tz = parseTzid(maestro.ical);
  return tz ? { tz } : 'flotante';
}

/**
 * Una ocurrencia por instancia del servicio, con el href/etag/iCal de su maestro.
 *
 * Una instancia sin maestro en el `REPORT` no se pinta: sin href no se puede abrir ni
 * editar, y guardarla crearía un evento nuevo. No debería pasar (los dos leen la misma
 * tabla con el mismo rango); si pasa, es mejor que falte a que se duplique al editar.
 */
export function ocurrenciasDeInstancias(
  maestros: readonly CalendarEvent[],
  instancias: readonly InstanciaServicio[],
): CalendarEvent[] {
  const porUid = new Map<string, CalendarEvent>();
  for (const m of maestros) if (m.uid && !porUid.has(m.uid)) porUid.set(m.uid, m);
  const out: CalendarEvent[] = [];
  for (const inst of instancias) {
    const maestro = porUid.get(inst.event.uid);
    if (!maestro) continue;
    const forma = formaDe(maestro);
    const start = comoElMaestro(inst.start, forma);
    if (!start) continue;
    const end = inst.end ? comoElMaestro(inst.end, forma) : null;
    out.push({
      ...maestro,
      start,
      // Sin fin en la instancia, tampoco el del maestro: caería en el primer día.
      end: end ?? undefined,
      // Una repetición cambiada en otro cliente (RECURRENCE-ID) lleva su propio
      // título y lugar; el maestro del recurso no los sabe.
      ...(inst.is_override
        ? {
            summary: inst.event.summary || maestro.summary,
            location: inst.event.location || undefined,
            description: inst.event.description || undefined,
          }
        : {}),
    });
  }
  return out;
}

/** `yyyymmddThhmmssZ` (la consulta CalDAV) → RFC 3339, que es lo que pide el REST. */
export function rfc3339DeIcal(valor: string): string | null {
  const m = valor.match(/^(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})(\d{2})Z$/);
  return m ? `${m[1]}-${m[2]}-${m[3]}T${m[4]}:${m[5]}:${m[6]}Z` : null;
}
