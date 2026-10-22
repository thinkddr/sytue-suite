/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Lo que comparten las rutas del BFF que le enseñan una foto a Sytue AI.
 *
 * Son dos —describir y preguntar— y las dos hacen exactamente lo mismo antes y
 * después de la instrucción: traer la miniatura en nombre de quien pregunta y
 * llamar a `/assist` con su sesión. Lo único que cambia entre ellas es qué se
 * le pide al modelo, así que eso es lo único que se queda en cada ruta.
 */
import { serverEnv } from '@sytue/auth/server';

const env = serverEnv(import.meta.env);
const fotos = (env.SYTUE_PHOTOS_API_URL ?? 'http://syphotos:8097').replace(/\/$/, '');

export function json(cuerpo: unknown, status = 200): Response {
  return new Response(JSON.stringify(cuerpo), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

/** Sin `SYTUE_AI_URL` la función no existe, y se dice con un mensaje claro. */
export const urlDeIA = (): string | undefined => env.SYTUE_AI_URL;

/**
 * La variable es la BASE del servicio, no el endpoint: Sytue AI tiene dos
 * (`/assist` y `/transcribe`). Se tolera que ya venga con la ruta puesta porque
 * hay instalaciones así y romperlas al actualizar sería cambiar un fallo por
 * otro.
 */
function endpoint(base: string): string {
  const limpia = base.replace(/\/$/, '');
  return limpia.endsWith('/assist') ? limpia : `${limpia}/api/v1/assist`;
}

/**
 * La miniatura de la foto en base64, o el error ya listo para devolver.
 *
 * SE PIDE EN NOMBRE de quien pregunta: si la foto no es suya, el servicio
 * contesta 404 y aquí no hay nada que mirar. Es la misma comprobación que
 * protege la galería, sin repetirla.
 *
 * Y SE MANDA LA MINIATURA, no el original ni la previa. Para decir qué sale en
 * una foto sobran 512 píxeles, y el tope de la petición de IA es de 256 KiB
 * contando el base64, que abulta un tercio más: una previa de 2048 no cabría.
 */
export async function miniaturaEnBase64(
  id: string,
  token: string,
): Promise<{ base64: string } | { error: Response }> {
  const imagen = await fetch(
    `${fotos}/api/v1/assets/${encodeURIComponent(id)}/variants/miniatura`,
    { headers: { authorization: `Bearer ${token}`, accept: 'image/jpeg' } },
  );
  if (!imagen.ok) {
    return { error: json({ error: 'photo_not_found' }, imagen.status === 404 ? 404 : 502) };
  }
  const bytes = new Uint8Array(await imagen.arrayBuffer());
  return { base64: Buffer.from(bytes).toString('base64') };
}

/** Le enseña la foto al modelo con la instrucción dada y devuelve su texto. */
export async function verLaFoto(opts: {
  base: string;
  token: string;
  locale: string;
  action: string;
  instruction: string;
  /**
   * Lo que haya escrito la persona va SIEMPRE aquí y nunca en `instruction`:
   * el contexto es dato y la instrucción es orden, y mezclarlos deja que una
   * pregunta escrita a mano reescriba las reglas de la de arriba.
   */
  context: string;
  esfuerzo: 'none' | 'medium' | 'high';
  base64: string;
}): Promise<{ text: string } | { error: Response }> {
  const res = await fetch(endpoint(opts.base), {
    method: 'POST',
    headers: {
      authorization: `Bearer ${opts.token}`,
      'content-type': 'application/json',
      // La lengua de la sesión viaja en la cabecera además de en la
      // instrucción: es lo que el servicio ya usa para elegir idioma.
      'accept-language': opts.locale,
    },
    body: JSON.stringify({
      action: opts.action,
      kind: 'photo',
      instruction: opts.instruction,
      context: opts.context,
      esfuerzo: opts.esfuerzo,
      imagen: opts.base64,
      // `imagen_tipo` y no `imagenTipo`: el servicio lo renombró al alinear los
      // tags JSON con la convención de la casa. Con el nombre viejo, el campo se
      // ignora y la foto viaja etiquetada como PNG siendo JPEG.
      imagen_tipo: 'image/jpeg',
    }),
    signal: AbortSignal.timeout(70_000),
  }).catch(() => null);

  if (!res) return { error: json({ error: 'ai_timeout' }, 504) };
  if (!res.ok) return { error: json({ error: 'ai_failed', status: res.status }, 502) };
  const { text } = (await res.json().catch(() => ({}))) as { text?: string };
  return { text: text ?? '' };
}
