/* SPDX-License-Identifier: Apache-2.0 */
/**
 * Sonido de aviso de mensajes (pieza D-ROOM-07). Un toque corto cuando llega un
 * mensaje que no estás mirando, con la misma receta que el timbre de llamada
 * (`timbre.ts`): dos senos por la Web Audio API, sin fichero de audio.
 *
 * La política por defecto es la decidida para la suite: SUENA para los mensajes
 * directos y las menciones (incluidas las palabras clave, que avisan como una
 * mención), y CALLA para el resto. Las dos cosas se ajustan en «Notificaciones».
 *
 * El ajuste se guarda en ESTE navegador (localStorage), no en el servicio: el
 * sonido es cosa del dispositivo —quien quiere el portátil callado en la oficina
 * no quiere por eso el móvil mudo—, igual que la suscripción push del mismo modal.
 */
import { contextoDisponible, pitido } from './timbre.js';

/** Qué mensajes suenan. */
export interface PreferenciasSonido {
  /** Mensajes directos, @menciones y palabras clave. */
  directos: boolean;
  /** Todo lo demás (canales en «Todas las notificaciones» o sin fijar). */
  resto: boolean;
}

export const SONIDO_POR_DEFECTO: Readonly<PreferenciasSonido> = Object.freeze({
  directos: true,
  resto: false,
});

/** Clave de localStorage del ajuste de sonido de este navegador. */
export const CLAVE_SONIDO = 'sytue-rooms:sonido-aviso';

/** Separación mínima entre dos sonidos: una ráfaga de diez mensajes es UN aviso. */
export const SEPARACION_MINIMA_MS = 2000;

/** Lee el ajuste de este navegador; sin almacenamiento o con basura, el defecto. */
export function leerPreferenciasSonido(): PreferenciasSonido {
  try {
    const raw = localStorage.getItem(CLAVE_SONIDO);
    if (!raw) return { ...SONIDO_POR_DEFECTO };
    const p = JSON.parse(raw) as Partial<PreferenciasSonido> | null;
    return {
      directos: typeof p?.directos === 'boolean' ? p.directos : SONIDO_POR_DEFECTO.directos,
      resto: typeof p?.resto === 'boolean' ? p.resto : SONIDO_POR_DEFECTO.resto,
    };
  } catch {
    return { ...SONIDO_POR_DEFECTO };
  }
}

/** Guarda el ajuste de este navegador; sin almacenamiento, se pierde en silencio. */
export function guardarPreferenciasSonido(p: PreferenciasSonido): void {
  try {
    localStorage.setItem(CLAVE_SONIDO, JSON.stringify({ directos: p.directos, resto: p.resto }));
  } catch {
    // Almacenamiento no disponible (modo privado): vale el defecto.
  }
}

/** Contexto en el que llega el mensaje; lo que la política no puede saber sola. */
export interface ContextoSonido {
  /** Me menciona (@ o @canal) o contiene una de mis palabras clave. */
  personal: boolean;
  /** El mensaje es del canal que tengo abierto. */
  canalActivo: boolean;
  /** La pestaña tiene el foco. */
  ventanaConFoco: boolean;
  /** El autor está bloqueado. */
  bloqueado: boolean;
  /** Estoy en «No molestar» o con los avisos en pausa. */
  silencio: boolean;
  prefs: PreferenciasSonido;
}

/**
 * ¿Suena este mensaje? Pura, para probar la política sin audio ni reloj.
 *
 * Calla siempre: lo propio, lo de alguien bloqueado, en «No molestar»/pausa, en un
 * canal silenciado, lo que ya estás viendo (canal abierto con la pestaña al frente) y
 * la tarjeta de «llamada iniciada», que ya tiene su timbre: sumarle el aviso haría
 * sonar dos cosas a la vez en cada llamada por un directo.
 * Un canal en «Solo menciones» —también un directo así ajustado— solo suena por lo
 * personal. Lo demás lo decide el ajuste: `directos` para directos y menciones,
 * `resto` para el resto.
 */
export function debeSonar(
  msg: { mine?: boolean; type?: string },
  channel: { kind: string; muted?: boolean; notifyLevel?: string } | undefined,
  ctx: ContextoSonido,
): boolean {
  if (msg.mine || msg.type === 'call' || ctx.bloqueado || ctx.silencio) return false;
  if (channel === undefined || channel.muted || channel.notifyLevel === 'none') return false;
  if (ctx.ventanaConFoco && ctx.canalActivo) return false;
  if (channel.notifyLevel === 'mentions' && !ctx.personal) return false;
  if (ctx.personal || channel.kind === 'direct') return ctx.prefs.directos;
  return ctx.prefs.resto;
}

/**
 * Envuelve un reproductor con la separación mínima: devuelve la función que pide
 * sonar y dice si sonó. El reloj se inyecta para probarlo sin esperar.
 */
export function conSeparacion(
  reproducir: () => void,
  ahora: () => number = Date.now,
  separacion = SEPARACION_MINIMA_MS,
): () => boolean {
  let ultimo = -Infinity;
  return () => {
    const t = ahora();
    if (t - ultimo < separacion) return false;
    ultimo = t;
    reproducir();
    return true;
  };
}

// Un único AudioContext para toda la página: los navegadores limitan cuántos hay
// vivos, y crear uno por mensaje acabaría dejando de sonar.
let compartido: AudioContext | null = null;

/**
 * Toca el aviso: dos notas cortas descendentes, más bajas y breves que el timbre de
 * llamada para que no se confundan. Sin gesto previo del usuario el navegador puede
 * no dejar sonar (autoplay): se ignora, el aviso visual no depende de esto.
 */
export function sonarAviso(): void {
  const Ctx = contextoDisponible();
  if (!Ctx) return;
  try {
    if (!compartido || compartido.state === 'closed') compartido = new Ctx();
    const ctx = compartido;
    void ctx.resume?.().catch(() => undefined);
    const t = ctx.currentTime;
    pitido(ctx, 1318.51, t, 0.11, 0.08); // mi6
    pitido(ctx, 987.77, t + 0.12, 0.16, 0.08); // si5
  } catch {
    // Sin audio disponible: el aviso se queda en el badge y la notificación.
  }
}
