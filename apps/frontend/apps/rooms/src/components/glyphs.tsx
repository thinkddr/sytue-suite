/**
 * Glifos de Sytue Rooms: iconos de acción del mensaje, el composer y el roster.
 *
 * Aquí vivió EmptyChatGlyph, una ilustración de 200x160 dibujada a mano para los
 * estados vacíos. Se borró: enmarcada en la tesela de EmptyState leía como
 * decoración genérica, y la mitad de los ~96 estados vacíos de la suite no lleva
 * ilustración ninguna. El título y la descripción bastan.
 */
import type { JSX } from 'preact';
import { Glifo } from '@sytue/ui';

/* Los glyphs de acción de Chat viven aquí porque `Icon` de @sytue/ui no expone
 * estos nombres (pin, guardado, copiar, enlace, no-leído, «…»). El DIBUJO es de
 * Rooms; el envoltorio —lienzo, grosor y relleno— es `Glifo`, el de toda la suite. */

/** «…» horizontal: disparador del menú de acciones por mensaje. */
export function MoreGlyph({ size = 16 }: { size?: number }): JSX.Element {
  return (
    <Glifo size={size}>
      <circle cx="5" cy="12" r="1" />
      <circle cx="12" cy="12" r="1" />
      <circle cx="19" cy="12" r="1" />
    </Glifo>
  );
}

/** Chincheta (fijar). */
export function PinGlyph({ size = 16 }: { size?: number }): JSX.Element {
  return (
    <Glifo size={size}>
      <path d="M9 4h6l-1 6 3 3v2H7v-2l3-3-1-6Z" />
      <path d="M12 15v5" />
    </Glifo>
  );
}

/** Marcador (guardar). */
export function BookmarkGlyph({ size = 16 }: { size?: number }): JSX.Element {
  return (
    <Glifo size={size}>
      <path d="M6 4h12a1 1 0 0 1 1 1v15l-7-4-7 4V5a1 1 0 0 1 1-1Z" />
    </Glifo>
  );
}

/** Nota con la esquina doblada (guardar en Sytue Notepads). Mismo dibujo que el
 *  icono de producto, podado al grid 24 de este juego. */
export function NoteGlyph({ size = 16 }: { size?: number }): JSX.Element {
  return (
    <Glifo size={size}>
      <path d="M7 4h9a1.5 1.5 0 0 1 1.5 1.5v7L12 18H7a1.5 1.5 0 0 1-1.5-1.5v-11A1.5 1.5 0 0 1 7 4Z" />
      <path d="M17.5 12.5h-3.9A1.6 1.6 0 0 0 12 14.1V18" />
    </Glifo>
  );
}

/** Dos hojas (copiar texto). */
export function CopyGlyph({ size = 16 }: { size?: number }): JSX.Element {
  return (
    <Glifo size={size}>
      <rect x="9" y="9" width="11" height="11" rx="2" />
      <path d="M5 15a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2" />
    </Glifo>
  );
}

/** Cámara (reunión de vídeo). */
export function VideoGlyph({ size = 16 }: { size?: number }): JSX.Element {
  return (
    <Glifo size={size}>
      <path d="M23 7 16 12l7 5V7Z" />
      <rect x="1" y="5" width="15" height="14" rx="2" />
    </Glifo>
  );
}

/** Eslabón (copiar enlace). */
export function LinkGlyph({ size = 16 }: { size?: number }): JSX.Element {
  return (
    <Glifo size={size}>
      <path d="M10 13a5 5 0 0 0 7 0l2-2a5 5 0 0 0-7-7l-1 1" />
      <path d="M14 11a5 5 0 0 0-7 0l-2 2a5 5 0 0 0 7 7l1-1" />
    </Glifo>
  );
}

/** Sobre con punto (marcar como no leído). */
export function UnreadGlyph({ size = 16 }: { size?: number }): JSX.Element {
  return (
    <Glifo size={size}>
      <path d="M4 8a2 2 0 0 1 2-2h9M4 8v9a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-6" />
      <path d="M4 8l8 5 3-2" />
      <circle cx="19" cy="6" r="2.4" fill="currentColor" stroke="none" />
    </Glifo>
  );
}

/** Doble marca de visto: quién ha leído el mensaje. */
export function SeenGlyph({ size = 16 }: { size?: number }): JSX.Element {
  return (
    <Glifo size={size}>
      <path d="m2 12 4 4 8-9" />
      <path d="m10 16 1.5 1.5L22 7" />
    </Glifo>
  );
}

/** Flecha a la derecha sobre una base (reenviar a otro canal). */
export function ForwardGlyph({ size = 16 }: { size?: number }): JSX.Element {
  return (
    <Glifo size={size}>
      <polyline points="15 17 20 12 15 7" />
      <path d="M4 18v-2a4 4 0 0 1 4-4h13" />
    </Glifo>
  );
}

/** Lápiz (editar). */
export function EditGlyph({ size = 16 }: { size?: number }): JSX.Element {
  return (
    <Glifo size={size}>
      <path d="M12 20h9" />
      <path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4Z" />
    </Glifo>
  );
}

/** Papelera (borrar). */
export function TrashGlyph({ size = 16 }: { size?: number }): JSX.Element {
  return (
    <Glifo size={size}>
      <path d="M3 6h18" />
      <path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2m3 0v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6" />
      <path d="M10 11v6M14 11v6" />
    </Glifo>
  );
}

/** Estrella (favorito). `filled` la rellena (canal ya favorito). */
export function StarGlyph({
  size = 16,
  filled = false,
}: {
  size?: number;
  filled?: boolean;
}): JSX.Element {
  return (
    <Glifo size={size} fill={filled ? 'currentColor' : undefined}>
      <path d="M12 3.5l2.6 5.27 5.82.85-4.21 4.1.99 5.79L12 17.77 6.8 19.5l.99-5.79-4.21-4.1 5.82-.85L12 3.5Z" />
    </Glifo>
  );
}

/** Campana tachada (silenciar). */
export function MuteGlyph({ size = 16 }: { size?: number }): JSX.Element {
  return (
    <Glifo size={size}>
      <path d="M8.7 6.7A5 5 0 0 1 17 10c0 2.6.6 4.3 1.3 5.3M6.9 6.9A5 5 0 0 0 6 10c0 4-2 5-2 5h12" />
      <path d="M10.3 20a2 2 0 0 0 3.4 0" />
      <path d="M3 3l18 18" />
    </Glifo>
  );
}

/** Caja de archivo (archivar canal). */
export function ArchiveGlyph({ size = 16 }: { size?: number }): JSX.Element {
  return (
    <Glifo size={size}>
      <path d="M3 5h18v4H3z" />
      <path d="M5 9v10a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V9" />
      <path d="M10 13h4" />
    </Glifo>
  );
}

/** Burbuja de conversación con líneas de texto (vista global de «Hilos»). */
export function ThreadsGlyph({ size = 16 }: { size?: number }): JSX.Element {
  return (
    <Glifo size={size}>
      <path d="M21 11.5a8.38 8.38 0 0 1-8.5 8.5 9.5 9.5 0 0 1-4-.9L3 21l1.9-5.5A8.38 8.38 0 0 1 4 11.5 8.5 8.5 0 0 1 12.5 3 8.38 8.38 0 0 1 21 11.5Z" />
      <path d="M8.5 10h7M8.5 13.5h4.5" />
    </Glifo>
  );
}

/** Reloj (mensajes programados). */
export function ClockGlyph({ size = 16 }: { size?: number }): JSX.Element {
  return (
    <Glifo size={size}>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 7v5l3.5 2" />
    </Glifo>
  );
}

/** Clip (adjuntar un fichero). */
export function ClipGlyph({ size = 16 }: { size?: number }): JSX.Element {
  return (
    <Glifo size={size}>
      <path d="M21.44 11.05 12.25 20.24a5 5 0 0 1-7.07-7.07l9.19-9.19a3 3 0 0 1 4.24 4.24l-9.2 9.19a1 1 0 0 1-1.41-1.41l8.48-8.49" />
    </Glifo>
  );
}

/** «!» en cuadro redondeado: prioridad del mensaje (Importante / Urgente). */
export function PriorityGlyph({ size = 16 }: { size?: number }): JSX.Element {
  return (
    <Glifo size={size}>
      <rect x="3.5" y="3.5" width="17" height="17" rx="4" />
      <path d="M12 8v4.5" />
      <path d="M12 16h.01" />
    </Glifo>
  );
}

/** Cara sonriente (abrir el selector de emoji). */
export function EmojiGlyph({ size = 16 }: { size?: number }): JSX.Element {
  return (
    <Glifo size={size}>
      <circle cx="12" cy="12" r="9" />
      <path d="M8.5 14a4 4 0 0 0 7 0" />
      <path d="M9 9h.01M15 9h.01" />
    </Glifo>
  );
}

/** Avión de papel (enviar mensaje). Encuadrado con ~3u de margen: ningún trazo
 *  toca el borde. El chevron queda reservado para navegación, no para enviar. */
export function SendGlyph({ size = 16 }: { size?: number }): JSX.Element {
  return (
    <Glifo size={size}>
      <path d="M21 3 14.5 21 11 13 3 9.5Z" />
      <path d="M21 3 11 13" />
    </Glifo>
  );
}

/** Flecha a bandeja (descargar un adjunto). */
export function DownloadGlyph({ size = 16 }: { size?: number }): JSX.Element {
  return (
    <Glifo size={size}>
      <path d="M12 3v12" />
      <path d="M7 10l5 5 5-5" />
      <path d="M5 21h14" />
    </Glifo>
  );
}

/** Flecha desde bandeja (subir un fichero del dispositivo). Espejo de DownloadGlyph. */
export function UploadGlyph({ size = 16 }: { size?: number }): JSX.Element {
  return (
    <Glifo size={size}>
      <path d="M12 15V3" />
      <path d="M7 8l5-5 5 5" />
      <path d="M5 21h14" />
    </Glifo>
  );
}

/** Carpeta (navegar el árbol de Sytue Drive en el selector de ficheros). */
export function FolderGlyph({ size = 16 }: { size?: number }): JSX.Element {
  return (
    <Glifo size={size}>
      <path d="M3 7a2 2 0 0 1 2-2h3.5l2 2.5H19a2 2 0 0 1 2 2V18a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7Z" />
    </Glifo>
  );
}

/** Hoja de fichero (tarjeta de adjunto no-imagen). */
export function FileGlyph({ size = 16 }: { size?: number }): JSX.Element {
  return (
    <Glifo size={size}>
      <path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8Z" />
      <path d="M14 3v5h5" />
    </Glifo>
  );
}

/** Imagen (montaña dentro de un marco): la entrada «GIF» del menú «+». */
export function ImageGlyph({ size = 16 }: { size?: number }): JSX.Element {
  return (
    <Glifo size={size}>
      <path d="M4 5h16a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1Z" />
      <path d="M3 16l5-5 4 4 3-3 6 6" />
      <path d="M9 9.5h.01" />
    </Glifo>
  );
}

/** Insignia «#» de canal sobre superficie clara: mismo cuadro 32 que el avatar de DM
 *  en las listas de canales de los modales (Explorar / Reenviar). Decorativa. */
export function ChannelIcon(): JSX.Element {
  return (
    <span
      class="bg-bg-subtle text-md text-text-muted grid h-8 w-8 shrink-0 place-items-center rounded-md font-semibold"
      aria-hidden="true"
    >
      #
    </span>
  );
}
