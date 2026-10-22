/**
 * Timbre de llamada entrante. Sin fichero de audio y sin dependencia: dos senos
 * cortos por la Web Audio API, que es la que ya trae el navegador.
 *
 * Un .mp3 habría sido un binario que empaquetar, versionar y servir —y que
 * revisar en cada auditoría de licencias— para 400 ms de sonido. Aquí el tono se
 * describe en diez líneas y no hay nada que descargar antes de que suene, que en
 * una llamada entrante es justo lo que importa.
 *
 * Dos avisos honestos sobre lo que un navegador permite:
 *
 *  - El audio SIN gesto previo del usuario está bloqueado por política (autoplay).
 *    Si la persona no ha tocado la pestaña, `AudioContext` nace suspendido y esto
 *    no suena. No es un fallo que haya que arreglar aquí: es la razón por la que
 *    el aviso VISUAL nunca depende del timbre.
 *  - Se para solo a los 30 segundos. Un timbre que suena para siempre porque
 *    nadie cerró la pestaña es peor que no tenerlo.
 */

/** Cada cuánto se repite el bitono, en milisegundos. */
const CADENCIA_MS = 3000;
/** Tope de duración: pasado esto se calla aunque nadie conteste. */
const MAXIMO_MS = 30_000;

type Ctor = typeof AudioContext;

/** Constructor de AudioContext del navegador, o null donde no exista (SSR, tests). */
function contextoDisponible(): Ctor | null {
  if (typeof window === 'undefined') return null;
  const w = window as unknown as { AudioContext?: Ctor; webkitAudioContext?: Ctor };
  return w.AudioContext ?? w.webkitAudioContext ?? null;
}

/** Un pitido con ataque y caída suaves: un seno a pelo chasquea al empezar y al parar. */
function pitido(ctx: AudioContext, hz: number, empieza: number, dura: number): void {
  const osc = ctx.createOscillator();
  const gan = ctx.createGain();
  osc.type = 'sine';
  osc.frequency.value = hz;
  gan.gain.setValueAtTime(0, empieza);
  gan.gain.linearRampToValueAtTime(0.14, empieza + 0.02);
  gan.gain.setValueAtTime(0.14, empieza + dura - 0.04);
  gan.gain.linearRampToValueAtTime(0, empieza + dura);
  osc.connect(gan).connect(ctx.destination);
  osc.start(empieza);
  osc.stop(empieza + dura + 0.02);
}

/**
 * Empieza a sonar. Devuelve la función que lo para; llamarla dos veces no hace
 * daño. Si el navegador no deja sonar, devuelve un no-op en vez de reventar: el
 * aviso en pantalla ya está puesto y una llamada entrante no es el momento de
 * tirar una excepción.
 */
export function sonarTimbre(): () => void {
  const Ctx = contextoDisponible();
  if (!Ctx) return () => undefined;

  let ctx: AudioContext;
  try {
    ctx = new Ctx();
  } catch {
    return () => undefined;
  }
  // Suspendido = la pestaña no ha tenido gesto del usuario. `resume()` es una
  // promesa que puede rechazarse, y su rechazo no debe llegar a la consola como
  // un error sin manejar.
  void ctx.resume?.().catch(() => undefined);

  let parado = false;
  const tanda = (): void => {
    if (parado || ctx.state === 'closed') return;
    const t = ctx.currentTime;
    // Bitono ascendente, como el de un teléfono: dos notas cortas y silencio.
    pitido(ctx, 587.33, t, 0.18); // re5
    pitido(ctx, 880, t + 0.22, 0.22); // la5
  };
  tanda();
  const repeticion = window.setInterval(tanda, CADENCIA_MS);
  const corte = window.setTimeout(() => parar(), MAXIMO_MS);

  function parar(): void {
    if (parado) return;
    parado = true;
    window.clearInterval(repeticion);
    window.clearTimeout(corte);
    void ctx.close?.().catch(() => undefined);
  }
  return parar;
}
