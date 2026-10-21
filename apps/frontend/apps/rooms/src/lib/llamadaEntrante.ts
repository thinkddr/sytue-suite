/**
 * Cuándo una llamada que empieza se convierte en un aviso en pantalla.
 *
 * Vive fuera del componente porque son cuatro reglas que se rompen en silencio:
 * quitar una no da error de tipos ni de render, solo hace que suene un timbre
 * cuando no debía —o, peor, que no suene cuando sí—. Aquí se pueden ejercitar.
 */
import type { Channel } from './chat.js';

/** Lo que hay que pintar cuando alguien llama. */
export interface AvisoEntrante {
  channelId: string;
  label: string;
  /** Un directo además SUENA; una llamada de canal solo se ve. */
  direct: boolean;
}

export interface Situacion {
  /** Canal donde ha empezado la llamada. */
  channelId: string;
  /** El canal que la persona está mirando ahora (null = ninguno). */
  activeId: string | null;
  /** ¿Ya está dentro de una llamada? */
  enLlamada: boolean;
  /** Los canales del usuario, para resolver nombre, tipo y silencio. */
  canales: Channel[];
}

/**
 * Devuelve el aviso, o null si esta llamada no debe interrumpir.
 *
 * No se avisa cuando:
 *   - es el canal que ya está abierto (el banner de la cabecera lo cuenta);
 *   - la persona ya está en otra llamada (dos timbres a la vez no ayudan);
 *   - el canal está silenciado (ya dijo que no quiere que le interrumpa);
 *   - el canal no está en su lista (no es suyo, o aún no ha cargado).
 */
export function avisoDeLlamada(s: Situacion): AvisoEntrante | null {
  if (!s.channelId || s.channelId === s.activeId || s.enLlamada) return null;
  const canal = s.canales.find((c) => c.id === s.channelId);
  if (!canal || canal.muted) return null;
  return {
    channelId: canal.id,
    label: canal.name,
    direct: canal.kind === 'direct',
  };
}
