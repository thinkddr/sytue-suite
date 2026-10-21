/**
 * useSondeo — repetir una consulta cada N ms SOLO mientras alguien mira.
 *
 * Un `setInterval` que consulta al servidor sigue consultando con la pestaña de
 * fondo, y ahí no hay nadie a quien enseñarle el resultado: gasta batería en el
 * cliente y peticiones en el servidor para nada. Medido en la suite, con las
 * pestañas abiertas y ocultas: el panel de logs de Sytue Admin pedía 720
 * peticiones/hora (dos agregaciones cada 10 s), «Visto» de un DM 240/hora, y
 * versiones otras 240/hora. Con esto, cero mientras nadie mira.
 *
 * Al volver a primer plano consulta INMEDIATAMENTE, que es lo que hay que hacer
 * después de una pausa: lo primero que se ve tiene que estar fresco.
 *
 * La tarea se guarda en una ref a propósito. Estos paneles llaman al sondeo con
 * una closure que captura sus filtros, y meterla en las dependencias reiniciaba
 * el intervalo —y lanzaba una consulta— en CADA pulsación de tecla del buscador.
 * El sondeo depende de su cadencia, no de lo que consulte.
 *
 * Lo escribían igual ReminderCenter (Calendar) y MailApp; el resto se dejó sin
 * pausa. Este es el punto único.
 */
import { useEffect, useState } from 'preact/hooks';
import { useUltimo } from './useUltimo.js';

/**
 * @param tarea  Qué consultar. Se lee siempre la última versión.
 * @param ms     Cadencia en milisegundos.
 * @param activo Con `false` no sondea (filtros vacíos, panel cerrado, sin trabajo
 *               en curso…). Es lo que permite apagarlo sin sacar el hook del render.
 */
export function useSondeo(tarea: () => void, ms: number, activo = true): void {
  const ref = useUltimo(tarea);

  useEffect(() => {
    if (!activo) return undefined;

    let id: ReturnType<typeof setInterval> | null = null;
    const arrancar = (): void => {
      if (id === null) id = setInterval(() => ref.current(), ms);
    };
    const parar = (): void => {
      if (id !== null) clearInterval(id);
      id = null;
    };
    const alCambiarVisibilidad = (): void => {
      if (document.hidden) {
        parar();
        return;
      }
      ref.current(); // al volver, lo que se ve tiene que estar fresco
      arrancar();
    };

    if (!document.hidden) arrancar();
    document.addEventListener('visibilitychange', alCambiarVisibilidad);
    return () => {
      parar();
      document.removeEventListener('visibilitychange', alCambiarVisibilidad);
    };
  }, [ref, ms, activo]);
}

/** Reloj de presentación: avanza mientras se ve y se refresca al volver. */
export function useAhora(ms = 60_000): Date {
  const [ahora, setAhora] = useState(() => new Date());
  useSondeo(() => setAhora(new Date()), ms);
  return ahora;
}
