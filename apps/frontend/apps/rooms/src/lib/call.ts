/**
 * Tarjetas de sistema de llamada (Sytue Rooms). Una tarjeta `type=call` con CUERPO
 * VACÍO es «inició una llamada» (con acción «Unirse»); con un entero es «Llamada
 * finalizada» y el cuerpo lleva la DURACIÓN en segundos (el backend reusa type=call sin
 * un tipo nuevo, ver services/chat PostCallEnded / meet→chat). Aquí se distingue una de
 * otra y se formatea la duración. Puro y testeable (bun test).
 */
import type { T } from '../i18n/index.js';

/**
 * Segundos de duración si `body` corresponde a una tarjeta de llamada FINALIZADA (cuerpo
 * = entero ≥ 0), o null si es una tarjeta de llamada INICIADA (cuerpo vacío) o cualquier
 * otro contenido. Distingue «Llamada finalizada · N» de «inició una llamada».
 */
export function callEndedSeconds(body: string): number | null {
  const t = body.trim();
  return /^\d+$/.test(t) ? Number(t) : null;
}

/**
 * Formatea una duración en segundos como etiqueta corta del catálogo: «menos de 1
 * min», «N min», «H h» o «H h M min». La `t` la inyecta el llamante (ya atada al
 * idioma), para que la función siga siendo pura y testeable.
 */
export function formatCallDuration(t: T, sec: number): string {
  if (sec < 60) return t('call.duration.underMinute');
  const mins = Math.floor(sec / 60);
  const h = Math.floor(mins / 60);
  const m = mins % 60;
  if (h === 0) return t('call.duration.minutes', { count: m });
  return m === 0
    ? t('call.duration.hours', { count: h })
    : t('call.duration.hoursMinutes', { hours: h, minutes: m });
}
