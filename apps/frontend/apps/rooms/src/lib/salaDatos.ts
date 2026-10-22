/**
 * Protocolo del canal de datos de una llamada: mano alzada y reacciones.
 *
 * Va por el canal de datos de LiveKit y no por el servicio Go a propósito. Son
 * señales EFÍMERAS —quién pide turno ahora, quién acaba de aplaudir— que no se
 * consultan más tarde ni tienen por qué sobrevivir a la reunión. Meterlas en la
 * base de datos sería guardar para siempre algo que caduca en segundos, y añadir
 * un viaje al servidor a lo que el SFU ya reparte. El JWT ya concede
 * `CanPublishData` desde que existe; estaba disponible y sin usar.
 *
 * Vive fuera del componente para poder comprobarlo sin navegador: lo que se
 * recibe por este canal viene de OTRO cliente, o sea de fuera, y todo lo que
 * entra por ahí hay que validarlo antes de creérselo.
 */

/** Emojis admitidos. Lista cerrada: lo que llega es de otro cliente. */
export const REACCIONES = ['👏', '👍', '🎉', '❤️', '😂', '🤔'] as const;
export type Reaccion = (typeof REACCIONES)[number];

export interface SenalMano {
  t: 'mano';
  /** true = la levanta, false = la baja. */
  alzada: boolean;
}

export interface SenalReaccion {
  t: 'reaccion';
  emoji: Reaccion;
}

export type Senal = SenalMano | SenalReaccion;

/** Cuánto se ve una reacción antes de desaparecer sola. */
export const REACCION_MS = 4000;

const codificador = new TextEncoder();
const descodificador = new TextDecoder();

/**
 * Serializa una señal para publicarla por el canal de datos.
 *
 * El genérico se declara: `Uint8Array` a secas es `Uint8Array<ArrayBufferLike>`,
 * y `publishData` pide `ArrayBuffer`. Decirlo aquí evita un cast en cada uso.
 */
export function codificar(s: Senal): Uint8Array<ArrayBuffer> {
  return codificador.encode(JSON.stringify(s));
}

/**
 * Interpreta lo que llega del canal, o `null` si no es una señal válida.
 *
 * Estricto a propósito: un emoji fuera de la lista se descarta en vez de
 * pintarse, porque lo que llegue aquí acaba en la pantalla de todos los
 * participantes y esto es una superficie que controla otro cliente. Y un JSON
 * ilegible no puede tumbar la sala: quien manda basura pierde su señal, no la
 * reunión de los demás.
 */
export function descodificar(datos: Uint8Array): Senal | null {
  let crudo: unknown;
  try {
    crudo = JSON.parse(descodificador.decode(datos));
  } catch {
    return null;
  }
  if (typeof crudo !== 'object' || crudo === null) return null;
  const v = crudo as Record<string, unknown>;

  if (v.t === 'mano' && typeof v.alzada === 'boolean') {
    return { t: 'mano', alzada: v.alzada };
  }
  if (v.t === 'reaccion' && typeof v.emoji === 'string') {
    const emoji = REACCIONES.find((e) => e === v.emoji);
    return emoji ? { t: 'reaccion', emoji } : null;
  }
  return null;
}

/**
 * Aplica una señal de mano a la cola de turnos y devuelve la cola resultante.
 *
 * La cola conserva el ORDEN DE LLEGADA, que es lo único que la hace útil: sin él
 * el anfitrión no sabe a quién le toca y la función no resuelve el problema que
 * viene a resolver. Volver a levantar la mano ya alzada NO reordena —si no, con
 * pulsar dos veces uno se colaría el primero.
 */
export function aplicarMano(cola: string[], identidad: string, alzada: boolean): string[] {
  if (!alzada) return cola.filter((id) => id !== identidad);
  return cola.includes(identidad) ? cola : [...cola, identidad];
}
