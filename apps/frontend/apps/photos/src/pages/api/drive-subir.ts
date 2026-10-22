/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * POST /api/drive-subir?nombre=<fichero> — el original entra en la carpeta
 * «Fotos» del Drive de la persona y se devuelve su id, que es lo que el servicio
 * de Fotos necesita para indexarlo.
 *
 * POR QUÉ PASA POR AQUÍ. Antes el navegador hablaba con `drive.<dominio>`
 * directamente, y eso no funcionaba ni podía funcionar: la página vive en el
 * host de la organización, así que la petición es cross-origin y el navegador
 * manda antes un preflight, que va SIN credenciales. Drive lo contestaba con 401
 * —no tiene CORS configurado— y el navegador ni llegaba a mandar la subida.
 * Medido en producción el 2026-09-10: «Preflight response is not successful.
 * Status code: 401».
 *
 * Y aunque Drive tuviera CORS, el token no tiene por qué salir del servidor para
 * esto: es el mismo motivo por el que existe el proxy de al lado.
 *
 * EL BINARIO VA EN STREAMING, sin leerlo a memoria: una foto son megabytes y
 * bufferizar multiplicaría la memoria de la réplica por cada subida en curso.
 * Solo las llamadas de metadatos —resolver o crear la carpeta— pasan por el SDK.
 */
import type { APIRoute } from 'astro';
import { DriveClient, bearer } from '@sytue/api-clients';
import { serverEnv } from '@sytue/auth/server';
import { bff, toBffContext } from '../../lib/bff.js';

export const prerender = false;

const env = serverEnv(import.meta.env);
// POR EL SERVICE DEL CLÚSTER, NO POR EL NOMBRE PÚBLICO. Salir al ingress y
// volver a entrar no funciona desde dentro: el pod no resuelve el nombre
// público —`getaddrinfo` se queda sin respuesta— y el fetch ni llega a ser
// HTTP, que aquí sale como 502. Es el mismo fallo que tuvieron el panel de
// cumplimiento (2026-08-14) y la consola (2026-09-10), y el mismo arreglo.
// Con respaldo a la pública para no romper un despliegue sin la variable.
const drive = (
  env.SYTUE_DRIVE_INTERNAL_URL ??
  env.SYTUE_DRIVE_API_URL ??
  'http://drive:8082'
).replace(/\/$/, '');

/** Carpeta del Drive donde van los originales que se suben desde la galería. */
const CARPETA = '/Fotos';

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const POST: APIRoute = async (ctx) => {
  const sesion = await bff.getSession(toBffContext(ctx));
  if (!sesion) return json({ error: 'unauthenticated' }, 401);

  const nombre = (new URL(ctx.request.url).searchParams.get('nombre') ?? '').trim();
  // El nombre viaja en el query y no en una cabecera, por lo mismo que en el
  // SDK: una cabecera HTTP es una ByteString y un nombre de fichero es Unicode.
  if (!nombre || nombre.includes('/')) {
    return json({ error: 'bad_request', message: 'Falta el nombre del fichero.' }, 400);
  }
  if (!ctx.request.body) {
    return json({ error: 'bad_request', message: 'Falta el contenido.' }, 400);
  }

  const token = sesion.tokens.accessToken;
  // `bearer` acepta el token a secas: envolverlo en una función asíncrona que
  // no espera nada no aportaba nada y rompía el lint.
  const cliente = new DriveClient({ baseUrl: drive, auth: bearer(token) });

  let carpeta;
  try {
    carpeta = await cliente.stat(CARPETA);
  } catch {
    try {
      carpeta = await cliente.createFolder(CARPETA);
    } catch {
      return json({ error: 'drive', message: 'No se pudo preparar la carpeta Fotos.' }, 502);
    }
  }

  // El id es opcional en el tipo del SDK, así que se comprueba: sin él la URL
  // saldría con «undefined» dentro y el 404 de Drive no diría por qué.
  if (!carpeta.id) {
    return json({ error: 'drive', message: 'La carpeta Fotos no trae id.' }, 502);
  }
  const destino =
    `${drive}/api/v1/folders/${encodeURIComponent(carpeta.id)}/files` +
    `?${new URLSearchParams({ name: nombre })}`;
  const res = await fetch(destino, {
    method: 'POST',
    headers: {
      authorization: `Bearer ${token}`,
      'content-type': ctx.request.headers.get('content-type') ?? 'application/octet-stream',
    },
    body: ctx.request.body,
    // @ts-expect-error duplex es obligatorio al enviar un cuerpo en streaming
    duplex: 'half',
  });
  if (!res.ok) {
    // 413 NO ES 502. Traducir «el fichero es demasiado grande» a «Bad Gateway»
    // manda a buscar una avería del servidor cuando lo que hay que hacer es subir
    // algo más pequeño; en la galería salía como «no se pudo subir» a secas.
    // Medido en el piloto con una foto de 1,4 MB: Drive contestaba 413 y aquí
    // salía 502.
    if (res.status === 413) {
      return json(
        { error: 'too_large', message: 'La foto es más grande de lo que admite este Drive.' },
        413,
      );
    }
    return json({ error: 'drive', status: res.status }, res.status === 401 ? 401 : 502);
  }
  const fichero = (await res.json()) as { id?: string };
  if (!fichero.id) return json({ error: 'drive', message: 'Drive no devolvió el id.' }, 502);
  return json({ id: fichero.id }, 201);
};
