/**
 * Decodifica un adjunto recibido del navegador (bytes en base64, viajan en el
 * JSON de envío o de borrador) al formato que espera el SDK JMAP. Compartido
 * por /api/send y /api/drafts: los dos reciben la MISMA forma de adjunto y
 * antes solo lo sabía decodificar el primero.
 */
import type { OutgoingAttachment } from '@sytue/api-clients';

/** Adjunto recibido del navegador: bytes en base64 (viajan en el JSON). */
export interface AttachmentPayload {
  name?: string;
  type?: string;
  contentBase64?: string;
  /**
   * Identificador de la imagen INCRUSTADA en el cuerpo (`<img src="cid:…">`).
   *
   * Con él el mensaje se compone como `multipart/related` y la imagen se ve dentro
   * del texto; sin él, el mismo fichero va como adjunto. Lo elige el compositor al
   * insertar, no el tipo MIME.
   */
  contentId?: string;
}

export function decodeAttachment(a: AttachmentPayload): OutgoingAttachment {
  const b64 = (a.contentBase64 ?? '').replace(/\s/g, '');
  if (!b64) throw new Error('empty');
  const bin = atob(b64); // valida el base64 (lanza si es inválido)
  const content = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) content[i] = bin.charCodeAt(i) & 0xff;
  return {
    name: a.name || 'adjunto',
    type: a.type || 'application/octet-stream',
    content,
    // Se acota a lo que puede ser un identificador: lo compone el navegador y
    // acaba en una cabecera del mensaje. Un carácter raro ahí es una cabecera
    // inventada, y `buildRfc5322` limpia además los ángulos y los saltos.
    ...(a.contentId && /^[A-Za-z0-9._-]{1,128}$/.test(a.contentId)
      ? { contentId: a.contentId }
      : {}),
  };
}
