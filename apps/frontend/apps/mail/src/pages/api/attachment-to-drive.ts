/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * POST /api/attachment-to-drive  { emailId, index }  → { path, name, link }
 *
 * GUARDAR UN ADJUNTO EN SYTUE DRIVE sin pasar por el disco de nadie. Hasta ahora el
 * único destino de un adjunto era la carpeta de descargas: para archivarlo, hacía
 * falta bajarlo y volver a subirlo a mano. Es el sentido inverso de «Adjuntar desde
 * Drive» (drive-files.ts) y comparte con él el mismo puente: el access token de la
 * sesión de Mail contra la API de Drive, server→server, sin salir al navegador.
 *
 * DÓNDE LO GUARDA: en /Attachments, no en la raíz. Drive SUSTITUYE por nombre
 * creando una versión nueva, así que guardar «factura.pdf» en la raíz colgaría el
 * adjunto de un tercero del historial de versiones del «factura.pdf» propio. En su
 * carpeta y con el nombre libre (`factura (2).pdf`) no se pisa nunca nada.
 *
 * El nombre de la carpeta NO se traduce: si dependiera del idioma de quien guarda,
 * la misma persona acabaría con cinco carpetas distintas al cambiar de idioma.
 */
import type { APIRoute } from 'astro';
import { DriveClient, bearer, HttpError } from '@sytue/api-clients';
import { driveFileUrl } from '@sytue/config';
import { getApiSession } from '../../lib/session.js';
import { jmapClient } from '../../lib/jmap.js';
import { serverEnv } from '@sytue/auth/server';

export const prerender = false;

const env = serverEnv(import.meta.env);

/** Carpeta destino, fija y en inglés base (ver cabecera). */
const FOLDER = '/Attachments';

function driveApiUrl(): string {
  // El Service del clúster primero: desde un pod, el nombre público no resuelve
  // y la llamada ni llega a ser HTTP (sale como 502).
  return (
    env.SYTUE_DRIVE_INTERNAL_URL ??
    env.SYTUE_DRIVE_API_URL ??
    env.PUBLIC_SYTUE_DRIVE_BASE_URL ??
    'https://drive.sytue.eu'
  );
}

/**
 * Nombre libre dentro de la carpeta: `informe.pdf`, `informe (2).pdf`, …
 *
 * ponytail: dos guardados simultáneos del mismo nombre pueden elegir el mismo hueco
 * y el segundo quedaría como versión del primero (Drive versiona, no pierde). Si
 * llegara a importar, el sitio de arreglarlo es el servicio, con un upload que
 * rechace sustituir.
 */
function freeName(taken: ReadonlySet<string>, name: string): string {
  if (!taken.has(name.toLowerCase())) return name;
  const dot = name.lastIndexOf('.');
  const stem = dot > 0 ? name.slice(0, dot) : name;
  const ext = dot > 0 ? name.slice(dot) : '';
  for (let n = 2; n < 1000; n++) {
    const candidate = `${stem} (${n})${ext}`;
    if (!taken.has(candidate.toLowerCase())) return candidate;
  }
  return `${stem} (${Date.now()})${ext}`;
}

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let body: { emailId?: unknown; index?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }
  const emailId = typeof body.emailId === 'string' ? body.emailId : '';
  const index = Number(body.index);
  if (!emailId || !Number.isInteger(index) || index < 0) {
    return json({ error: 'invalid_body' }, 400);
  }

  let attachment;
  try {
    const all = await jmapClient(session.accessToken).getAttachments(emailId);
    attachment = all[index];
  } catch {
    return json({ error: 'jmap_error' }, 502);
  }
  if (!attachment) return json({ error: 'not_found' }, 404);

  const drive = new DriveClient({
    baseUrl: driveApiUrl(),
    auth: bearer(session.accessToken),
  });

  try {
    // La carpeta se crea la primera vez. Si ya está, Drive contesta 409 y eso ES el
    // estado que se quería: seguir adelante.
    let existing: Awaited<ReturnType<typeof drive.list>> = [];
    try {
      existing = await drive.list(FOLDER);
    } catch (err) {
      if (err instanceof HttpError && err.status === 404) {
        await drive.createFolder(FOLDER);
      } else {
        throw err;
      }
    }

    const taken = new Set(existing.map((e) => e.name.toLowerCase()));
    const name = freeName(taken, attachment.name || 'adjunto');
    const buf = new ArrayBuffer(attachment.content.byteLength);
    new Uint8Array(buf).set(attachment.content);
    const saved = await drive.upload(
      `${FOLDER}/${name}`,
      buf,
      attachment.type || 'application/octet-stream',
    );
    return json({
      name,
      path: saved.path,
      // Enlace directo al fichero para que el aviso pueda llevar hasta él.
      link: saved.id ? driveFileUrl(saved.id) : undefined,
    });
  } catch (err) {
    if (err instanceof HttpError && err.status === 507) return json({ error: 'quota' }, 507);
    return json({ error: 'drive_error' }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
