/**
 * Programar envío («Enviar más tarde» de Slack): opciones RELATIVAS calculadas en
 * cliente, en hora local, para no mostrar nunca un calendario del sistema antes de
 * que el usuario lo pida. El compositor guarda el instante elegido como valor de
 * `<input type="datetime-local">` (hora local sin zona, `YYYY-MM-DDTHH:mm`), así
 * los atajos y la fecha personalizada comparten un único estado; al enviar se
 * convierte a ISO/UTC para `ComposeOptions.sendAt`.
 *
 * Todo es PURO (`now` inyectable) para testear el cálculo sin reloj real, igual que
 * `expiryAt` en lib/status.ts.
 */
import { formatDateTime, type Locale } from '@sytue/i18n';

/** Hora de los atajos relativos: inicio de jornada. */
const WORK_START_HOUR = 9;

/** Una opción del menú «Enviar más tarde»: clave de catálogo + valor para el input. */
export interface SchedulePreset {
  /** Clave del catálogo: `schedule.tomorrow` o `schedule.monday`, con `{time}`. */
  key: 'schedule.tomorrow' | 'schedule.monday';
  /** Instante en formato `datetime-local` (hora LOCAL sin zona). */
  value: string;
  /** Instante (epoch ms), para formatear la hora en el idioma del usuario. */
  at: number;
}

/**
 * Valor de un `<input type="datetime-local">` para un instante (hora LOCAL sin
 * zona). También sirve de `min` del control para impedir elegir un instante pasado.
 */
export function toLocalValue(ms: number): string {
  const d = new Date(ms - new Date(ms).getTimezoneOffset() * 60000);
  return d.toISOString().slice(0, 16);
}

/**
 * Atajos relativos ofrecidos en el menú, en orden: mañana y el próximo lunes, ambos
 * a las 9:00 locales. `setDate`/`setHours` respetan el huso y los cambios de hora.
 * Ambos instantes son siempre FUTUROS (el lunes se busca estrictamente hacia
 * delante: si hoy es lunes, cae en el de la semana que viene).
 */
export function schedulePresets(now: number = Date.now()): SchedulePreset[] {
  const tomorrow = new Date(now);
  tomorrow.setDate(tomorrow.getDate() + 1);
  tomorrow.setHours(WORK_START_HOUR, 0, 0, 0);

  const monday = new Date(now);
  monday.setHours(WORK_START_HOUR, 0, 0, 0);
  // Avanza al menos un día y para en el primer lunes (getDay: 0=dom … 1=lun).
  do {
    monday.setDate(monday.getDate() + 1);
  } while (monday.getDay() !== 1);

  return [
    { key: 'schedule.tomorrow', value: toLocalValue(tomorrow.getTime()), at: tomorrow.getTime() },
    { key: 'schedule.monday', value: toLocalValue(monday.getTime()), at: monday.getTime() },
  ];
}

/**
 * Etiqueta del chip de programación a partir del valor del control, en el idioma
 * del usuario (el reloj de 12/24 h y el orden día/mes los decide el locale).
 * ponytail: fecha absoluta corta («lun 3 ago, 09:00») en vez de lenguaje relativo
 * («mañana»), que se vuelve falso al cruzar la medianoche con el chip en pantalla.
 */
export function formatSchedule(locale: Locale, localValue: string): string {
  const d = new Date(localValue);
  if (Number.isNaN(d.getTime())) return '';
  return formatDateTime(locale, d, {
    weekday: 'short',
    day: 'numeric',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
  });
}
