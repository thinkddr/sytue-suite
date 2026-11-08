import { schedulePresets } from './schedule.js';

/**
 * «Pausar avisos» (Slack «Pause notifications»): hasta cuándo no avisa Rooms de nada.
 * El instante se guarda en el servicio (`paused_until`, mig. 0061) para que valga en
 * todos los dispositivos, y CADA cliente lo compara con su reloj, igual que la ventana
 * «No molestar». Gemelo en Fyne: apps/fyne/apps/rooms/pausa.go.
 */

/** Las cinco duraciones del menú, en orden. */
export type DuracionPausa = '30m' | '1h' | '2h' | 'tomorrow' | 'monday';

export const DURACIONES_PAUSA: readonly DuracionPausa[] = ['30m', '1h', '2h', 'tomorrow', 'monday'];

const MINUTOS: Partial<Record<DuracionPausa, number>> = { '30m': 30, '1h': 60, '2h': 120 };

/**
 * Hasta cuándo (epoch ms) pausa cada opción: las tres primeras cuentan desde ahora;
 * «hasta mañana» y «hasta el lunes» son a las 9:00 locales, los mismos instantes que
 * «Enviar más tarde» (schedulePresets): un solo criterio de «mañana» en Rooms.
 */
export function finDePausa(duracion: DuracionPausa, now: number = Date.now()): number {
  const minutos = MINUTOS[duracion];
  if (minutos !== undefined) return now + minutos * 60_000;
  const [manana, lunes] = schedulePresets(now);
  return duracion === 'tomorrow' ? manana!.at : lunes!.at;
}

/** ¿Hay una pausa en curso? Sin fecha, o con una ya pasada, no. */
export function pausaActiva(pausedUntil: string | undefined, now: number = Date.now()): boolean {
  if (!pausedUntil) return false;
  const hasta = Date.parse(pausedUntil);
  return Number.isFinite(hasta) && hasta > now;
}

/**
 * Turnos de las preferencias de aviso: una lectura lanzada ANTES de la última
 * escritura (pausar, reanudar) puede responder después con el estado de antes, y
 * pisaba la pausa recién confirmada. `escribir()` se llama al pedir la escritura y al
 * confirmarla; `leer()` devuelve si la lectura sigue valiendo cuando llega.
 * Gemelo en Fyne: `pausaTurno` de la Sala.
 */
export function turnosDePausa(): { escribir: () => void; leer: () => () => boolean } {
  let turno = 0;
  return {
    escribir: () => {
      turno++;
    },
    leer: () => {
      const mio = turno;
      return () => mio === turno;
    },
  };
}
