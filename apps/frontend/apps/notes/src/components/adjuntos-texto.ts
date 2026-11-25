/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * IMÁGENES Y FICHEROS DENTRO DEL TEXTO DE UNA NOTA.
 *
 * Se guardan como los demás adjuntos —en `data.attachments`, como URL de datos (NoteCard)—
 * y el cuerpo los nombra por su id con el esquema `adjunto:`, que es lo único que el
 * saneador del servicio deja en un `src` (`apps/nucleo/notes/model/html.go`):
 *
 *   imagen   <img src="adjunto:1732540000000-ab12cd" alt="plano.png">
 *   fichero  <a href="adjunto:1732540000000-ab12cd">informe.pdf</a>
 *
 * Es el mismo HTML que escribe Fyne (`adjuntos_cuerpo.go`). Al pintarlo, la imagen toma
 * los datos de su adjunto (`data-adjunto` recuerda cuál era) y, al LEER, el enlace a un
 * fichero lo descarga. Lo que se entrega al guardar vuelve a nombrar el adjunto
 * (`normalizarSalida`): la imagen nunca viaja dos veces.
 *
 * Los datos se ponen como una URL `blob:` y no como la URL de datos: el editor lee su
 * `innerHTML` en cada pulsación, y con una imagen de megas dentro cada letra copiaba y
 * volvía a analizar los megas. La de cada adjunto se hace una vez (su id no cambia de
 * contenido) y dura lo que la página.
 */
import { ESQUEMA_ADJUNTO, idDeAdjunto } from '@sytue/ui/browser/bloques-de-notas';
import { escapar } from '../client/captura.js';
import type { NoteAttachment } from './NoteCard.js';

/** Lo que cabe en `data` de una nota (el tope del servicio): un fichero mayor no entra. */
export const MAX_ADJUNTO = 5 * 1024 * 1024;

/** Los tipos de imagen que se pintan dentro del texto; lo demás va como fichero. */
export const TIPOS_DE_IMAGEN: readonly string[] = [
  'image/png',
  'image/jpeg',
  'image/gif',
  'image/webp',
];

/** Una URL de datos en base64, que es como se guardan los adjuntos. */
const DATOS = /^data:[^,;]*;base64,[A-Za-z0-9+/\r\n]*={0,2}$/;

/** La URL `blob:` de cada adjunto ya pintado, por su id. */
const urls = new Map<string, string>();

/** Lo que se pone en un `src` o un `href` para un adjunto: su URL `blob:` (o, donde no
 *  las hay, la de datos). */
function urlDe(a: NoteAttachment): string {
  const hecha = urls.get(a.id);
  if (hecha) return hecha;
  if (typeof URL.createObjectURL !== 'function') return a.dataUrl;
  const [cabeza = '', cuerpo = ''] = a.dataUrl.split(',', 2);
  const tipo = cabeza.slice('data:'.length, -';base64'.length);
  let url = a.dataUrl;
  try {
    const bytes = Uint8Array.from(atob(cuerpo.replace(/\s/g, '')), (c) => c.charCodeAt(0));
    url = URL.createObjectURL(new Blob([bytes], { type: tipo }));
  } catch {
    // Unos datos que no se descodifican: la URL de datos tal cual.
  }
  urls.set(a.id, url);
  return url;
}

export function esImagenEnLinea(a: Pick<NoteAttachment, 'type'>): boolean {
  return TIPOS_DE_IMAGEN.includes(a.type);
}

/** Lee ficheros como adjuntos (`readAsDataURL`), con el id de siempre: hora y azar. */
export function leerAdjuntos(ficheros: readonly File[]): Promise<NoteAttachment[]> {
  return Promise.all(
    ficheros.map(
      (f) =>
        new Promise<NoteAttachment>((resolve) => {
          const reader = new FileReader();
          reader.onload = () =>
            resolve({
              id: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
              name: f.name,
              size: f.size,
              type: f.type,
              dataUrl: typeof reader.result === 'string' ? reader.result : '',
            });
          reader.readAsDataURL(f);
        }),
    ),
  );
}

/**
 * El HTML con que entra un adjunto ya guardado en el texto: la imagen o el enlace al
 * fichero con su nombre y un espacio duro para seguir escribiendo fuera de él. Vacío si
 * su id no tiene la forma de un adjunto.
 */
export function htmlDeAdjunto(a: NoteAttachment): string {
  const id = idDeAdjunto(ESQUEMA_ADJUNTO + a.id);
  if (!id) return '';
  return esImagenEnLinea(a)
    ? `<img src="${ESQUEMA_ADJUNTO}${id}" alt="${escapar(a.name)}">`
    : `<a href="${ESQUEMA_ADJUNTO}${id}">${escapar(a.name)}</a>&nbsp;`;
}

/**
 * Pinta los adjuntos de un cuerpo ya puesto en el DOM: cada `<img src="adjunto:ID">`
 * toma los datos de su adjunto y apunta cuál era en `data-adjunto`; al LEER (`lectura`),
 * cada enlace a un fichero lo descarga con su nombre. Lo que no es un adjunto de la nota,
 * o no es del tipo que dice, se queda como está (la imagen, rota, con su `alt`).
 */
export function pintarAdjuntos(
  raiz: HTMLElement | null,
  adjuntos: readonly NoteAttachment[],
  lectura: boolean,
): void {
  if (!raiz) return;
  const de = (ref: string | null) => {
    const id = idDeAdjunto(ref);
    const a = id ? adjuntos.find((x) => x.id === id) : undefined;
    return a && typeof a.dataUrl === 'string' && DATOS.test(a.dataUrl) ? a : undefined;
  };
  for (const img of raiz.querySelectorAll<HTMLImageElement>('img[src^="adjunto:"]')) {
    const a = de(img.getAttribute('src'));
    if (!a || !esImagenEnLinea(a) || !a.dataUrl.startsWith(`data:${a.type};`)) continue;
    img.setAttribute('data-adjunto', a.id);
    img.setAttribute('src', urlDe(a));
  }
  if (!lectura) return;
  for (const enlace of raiz.querySelectorAll<HTMLAnchorElement>('a[href^="adjunto:"]')) {
    const a = de(enlace.getAttribute('href'));
    if (!a) continue;
    enlace.setAttribute('href', urlDe(a));
    enlace.setAttribute('download', a.name);
    enlace.removeAttribute('target');
  }
}
