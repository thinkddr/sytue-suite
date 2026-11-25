/* SPDX-License-Identifier: Apache-2.0 */
/**
 * Reanudar una llamada cifrada cuando el SDK de LiveKit empieza a reconectar solo.
 *
 * El SDK puede reanudar el transporte —entero o solo la señal— sin pasar por el
 * servidor: con el token, la sesión y las pistas de antes, y sin un acuse nuevo. En una
 * llamada cifrada eso no vale: la publicación tiene que esperar a que el servidor
 * reconozca la conexión nueva con el ámbito vigente. Hasta ahora la llamada se cortaba
 * y pedía volver a entrar. Ahora, en este orden:
 *
 *   1. AL MOMENTO, sin ceder el hilo: se retira el permiso de publicar (el acuse deja de
 *      valer) y se paran las pistas locales, para que el SDK no pueda volver a mandar
 *      nada con lo de antes.
 *   2. Se corta el transporte del SDK: su reconexión automática no sigue.
 *   3. Se reanuda a mano (CallEncryption.refresh): el ámbito se lee otra vez del
 *      servidor —con su clave nueva si cambió la época—, un token nuevo con la
 *      publicación cerrada, una conexión nueva y un acuse nuevo. Solo entonces se
 *      vuelve a publicar (`ready`).
 *
 * Un segundo aviso del SDK mientras se reanuda no vuelve a empezar. Si la red se cae
 * una y otra vez (más de `maxSeguidas` reanudaciones en `ventanaMs`) o algo falla, la
 * llamada termina como antes, cerrada: fail-closed.
 */

export type EstadoReanudacion = 'quieta' | 'reanudando' | 'fallida';

export interface PasosReanudacion {
  /** Retira el permiso de publicar (CallEncryption.suspendForReconnect). Síncrono. */
  suspender: () => void;
  /** Para las pistas locales (deshabilitar y `stop`). Síncrono. */
  pararPistas: () => void;
  /** La interfaz pasa a «reconectando». */
  avisar: () => void;
  /** Corta el transporte del SDK (room.disconnect): no sigue reconectando solo. */
  cortarTransporte: () => Promise<void>;
  /** Ámbito, clave, conexión y acuse nuevos; publica al final (CallEncryption.refresh). */
  reanudar: () => Promise<void>;
  /** Termina la llamada, cerrada (la de siempre: claves, worker, transporte). */
  fallar: () => void;
}

export interface OpcionesReanudacion {
  /** Cuántas reanudaciones seguidas se admiten dentro de `ventanaMs`. */
  maxSeguidas?: number;
  ventanaMs?: number;
  ahora?: () => number;
}

export class ReanudacionLlamada {
  private estado: EstadoReanudacion = 'quieta';
  private enCurso: Promise<void> | null = null;
  private recientes: number[] = [];
  private readonly maxSeguidas: number;
  private readonly ventanaMs: number;
  private readonly ahora: () => number;

  constructor(
    private readonly pasos: PasosReanudacion,
    opciones: OpcionesReanudacion = {},
  ) {
    this.maxSeguidas = opciones.maxSeguidas ?? 3;
    this.ventanaMs = opciones.ventanaMs ?? 60_000;
    this.ahora = opciones.ahora ?? Date.now;
  }

  /** ¿Hay una reanudación en marcha? Mientras, los avisos del transporte son suyos. */
  get reanudando(): boolean {
    return this.estado === 'reanudando';
  }

  get fallida(): boolean {
    return this.estado === 'fallida';
  }

  /**
   * El SDK ha empezado a reconectar solo (RoomEvent.Reconnecting o
   * SignalReconnecting). Devuelve la reanudación en curso, que no rechaza nunca: un
   * fallo acaba en `fallar`.
   */
  alReconectarSolo(): Promise<void> {
    if (this.estado === 'fallida') return Promise.resolve();
    if (this.enCurso) return this.enCurso;
    this.estado = 'reanudando';
    try {
      this.pasos.suspender();
      this.pasos.pararPistas();
    } catch {
      this.fallar();
      return Promise.resolve();
    }
    const ahora = this.ahora();
    this.recientes = this.recientes.filter((t) => ahora - t < this.ventanaMs);
    this.recientes.push(ahora);
    if (this.recientes.length > this.maxSeguidas) {
      this.fallar();
      return Promise.resolve();
    }
    this.pasos.avisar();
    const job = (async () => {
      try {
        await this.pasos.cortarTransporte();
        if (this.estado !== 'reanudando') return;
        await this.pasos.reanudar();
        if (this.estado === 'reanudando') this.estado = 'quieta';
      } catch {
        this.fallar();
      } finally {
        this.enCurso = null;
      }
    })();
    this.enCurso = job;
    return job;
  }

  /** La llamada terminó por otro lado (colgar, cerrar la vista): no se reanuda más. */
  parar(): void {
    this.estado = 'fallida';
  }

  private fallar(): void {
    if (this.estado === 'fallida') return;
    this.estado = 'fallida';
    try {
      this.pasos.fallar();
    } catch {
      // La llamada ya está terminada para quien la usa.
    }
  }
}
