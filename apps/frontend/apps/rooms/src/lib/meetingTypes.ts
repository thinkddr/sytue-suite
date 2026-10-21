/**
 * Formas de la videollamada compartidas por las TRES capas: el cliente de servidor
 * (`lib/meet.ts`), el de navegador (`client/meetApi.ts`) y la vista
 * (`components/MeetingsPanel.tsx`).
 *
 * Viven aparte a propósito. Declararlas en `lib/meet.ts` obligaba al panel a importar
 * de un módulo que trae `@sytue/auth/server`: aunque sea `import type` y desaparezca al
 * compilar, mete código de SERVIDOR en el grafo del cliente y el dev server se queda
 * arrancando para siempre sin llegar a escuchar. Un fichero de tipos puro —sin un solo
 * import— no puede hacer eso.
 */

/** Reunión: una sala de vídeo, cuelgue o no de un canal. */
export interface Meeting {
  id: string;
  /** Identificador técnico en LiveKit (`<tenant>_<opaco>`). */
  name: string;
  title: string;
  status: 'scheduled' | 'active' | 'ended';
  createdBy: string;
  startedAt?: string | undefined;
  endedAt?: string | undefined;
  /** Canal de origen si la reunión nació de uno; ausente en las convocadas sueltas. */
  channelId?: string | undefined;
  /** Clave del enlace de una reunión suelta: con ella entra quien no la convocó. */
  linkKey?: string | undefined;
}

/** Participante de una reunión (presencia derivada de los webhooks del SFU). */
export interface MeetingParticipant {
  id: string;
  /** Identidad en LiveKit = sub de Sytue ID. Es lo que expulsa `removeParticipant`. */
  identity: string;
  name: string;
  role: 'host' | 'speaker' | 'viewer';
  joinedAt: string;
  leftAt?: string | undefined;
}

/** Grabación de una reunión: el fichero vive en Object Storage, esto es su metadato. */
export interface MeetingRecording {
  id: string;
  status: 'started' | 'completed' | 'failed';
  sizeBytes: number;
  durationSec: number;
  startedAt: string;
  endedAt?: string | undefined;
}
