/**
 * Zonas horarias — utilidades PURAS sobre `Intl` nativo (sin dependencias). La
 * app trabaja con instantes absolutos (Date); estas funciones resuelven el
 * reloj-de-pared de un instante en una zona IANA y a la inversa, para:
 *   - el selector de zona POR EVENTO del modal (lista + etiqueta con offset), y
 *   - situar un evento con TZID en su instante real (no la hora naíf del SO).
 *
 * La ZONA DE VISUALIZACIÓN por defecto es la del navegador: como el layout puro
 * (timeLayout) lee la hora-de-pared local de una Date, alimentar esos cálculos
 * con el instante absoluto correcto ya los posiciona en la zona del navegador.
 */

/** Zona horaria del navegador (zona de visualización por defecto). */
export function browserZone(): string {
  return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
}

/** Lista de zonas IANA para el selector (fallback mínimo si el runtime no la da). */
export function listTimeZones(): string[] {
  const anyIntl = Intl as { supportedValuesOf?: (key: string) => string[] };
  const zones = anyIntl.supportedValuesOf?.('timeZone');
  return zones && zones.length > 0 ? zones : ['UTC', browserZone()];
}

/** Partes del reloj-de-pared de un instante `at` en la zona `tz` (h23). */
interface WallParts {
  year: number;
  month: number;
  day: number;
  hour: number;
  minute: number;
  second: number;
}

function wallParts(at: Date, tz: string): WallParts {
  const dtf = new Intl.DateTimeFormat('en-US', {
    timeZone: tz,
    hourCycle: 'h23',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
  const parts = dtf.formatToParts(at);
  const num = (type: Intl.DateTimeFormatPartTypes): number =>
    Number(parts.find((p) => p.type === type)?.value ?? 0);
  // hour '2-digit' h23 devuelve 00..23, pero algunos motores dan «24» a
  // medianoche: se normaliza con % 24.
  return {
    year: num('year'),
    month: num('month'),
    day: num('day'),
    hour: num('hour') % 24,
    minute: num('minute'),
    second: num('second'),
  };
}

/**
 * Offset de `tz` en el instante `at`, en minutos al ESTE de UTC (Madrid verano
 * = +120; Nueva York invierno = −300). Compara el reloj-de-pared de la zona con
 * el mismo reloj leído como UTC.
 */
export function zoneOffsetMinutes(at: Date, tz: string): number {
  const p = wallParts(at, tz);
  const asUtc = Date.UTC(p.year, p.month - 1, p.day, p.hour, p.minute, p.second);
  const atSec = Math.floor(at.getTime() / 1000) * 1000; // formatToParts trunca a segundos
  return Math.round((asUtc - atSec) / 60_000);
}

/** Etiqueta legible con offset actual: «Europe/Madrid (GMT+02:00)». */
export function zoneLabel(tz: string, at: Date = new Date()): string {
  const off = zoneOffsetMinutes(at, tz);
  const sign = off < 0 ? '-' : '+';
  const abs = Math.abs(off);
  const hh = String(Math.floor(abs / 60)).padStart(2, '0');
  const mm = String(abs % 60).padStart(2, '0');
  return `${tz.replace(/_/g, ' ')} (GMT${sign}${hh}:${mm})`;
}

/**
 * Instante ABSOLUTO de un reloj-de-pared (y,mo,d,h,mi,s) situado en la zona `tz`.
 * Inverso de leer la hora local: corrige el offset de la zona en ese instante.
 * Doble pasada para converger si la corrección cruza un cambio de offset (borde
 * DST); en horas inexistentes/ambiguas devuelve un instante válido y determinista.
 */
export function instantFromZonedWallClock(
  y: number,
  mo: number,
  d: number,
  h: number,
  mi: number,
  s: number,
  tz: string,
): Date {
  const guess = Date.UTC(y, mo - 1, d, h, mi, s);
  const off1 = zoneOffsetMinutes(new Date(guess), tz);
  const off2 = zoneOffsetMinutes(new Date(guess - off1 * 60_000), tz);
  return new Date(guess - off2 * 60_000);
}
