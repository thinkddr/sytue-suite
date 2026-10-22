/**
 * Borradores de Sytue Mail (proxy BFF autorizado por cookie de sesión).
 *
 *   POST   /api/drafts            → guarda un borrador (o lo actualiza si trae id;
 *                                   o lo descarta si trae {id, delete:true}).
 *   GET    /api/drafts?id=…        → lee el contenido editable de un borrador.
 *   DELETE /api/drafts?id=…        → descarta (uso servidor; desde el navegador se
 *                                   usa POST {delete:true} porque el checkOrigin
 *                                   CSRF de Astro bloquea el DELETE con 403).
 *
 * El remitente se fija al usuario autenticado (no se confía en el cliente), igual
 * que en /api/send. El SDK compone el RFC 5322 y el servicio Go lo materializa en
 * la carpeta Drafts (\Draft) sin encolar el envío. Listar borradores = /api/emails
 * de la carpeta Drafts.
 */
import type { APIRoute } from 'astro';
import type { EmailAddress, OutgoingAttachment, OutgoingEmail } from '@sytue/api-clients';
import { getApiSession } from '../../lib/session.js';
import { jmapClient } from '../../lib/jmap.js';
import { direccionDeEnvio } from '../../lib/identidad.js';
import { decodeAttachment, type AttachmentPayload } from '../../lib/attachments.js';

export const prerender = false;

interface Payload {
  /** Presente al actualizar un borrador ya guardado (reemplazo). */
  id?: string;
  /** true = descartar el borrador `id` (el navegador no puede usar DELETE). */
  delete?: boolean;
  to?: EmailAddress[];
  cc?: EmailAddress[];
  subject?: string;
  textBody?: string;
  // Los dos siguientes: el borrador guardaba SOLO texto plano, y el editor
  // enriquecido (negrita, listas, imágenes incrustadas) se perdía al cerrar el
  // compositor sin enviar. `saveDraft`/`updateDraft` del SDK JMAP ya sabían
  // componer un MIME completo con los dos — los usaba /api/send y nunca este
  // endpoint — así que no hacía falta nada nuevo del lado del servicio, solo
  // dejar de recortar lo que el compositor ya tenía listo para mandar.
  htmlBody?: string;
  attachments?: AttachmentPayload[];
}

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const body = (await ctx.request.json().catch(() => ({}))) as Payload;

  // Descartar: el navegador no puede usar DELETE (el checkOrigin CSRF de Astro lo
  // rechaza con 403), así que el descarte llega como POST con {id, delete:true}.
  if (body.delete && body.id) {
    try {
      await jmapClient(session.accessToken).deleteDraft(body.id);
      return json({ ok: true });
    } catch (err) {
      return json({ error: 'jmap_error', message: String(err) }, 502);
    }
  }

  let attachments: OutgoingAttachment[];
  try {
    attachments = (body.attachments ?? []).map(decodeAttachment);
  } catch {
    return json({ error: 'invalid_attachment' }, 400);
  }

  // Misma regla que en el envío: el remitente es la dirección del BUZÓN. Un
  // borrador con el `From` equivocado se guarda igual y luego no se puede enviar,
  // que es la peor forma de fallar — parece que estaba bien hasta el final.
  const propia = await direccionDeEnvio(session.accessToken, session.user.id, session.user.email);
  const email: OutgoingEmail = {
    from: { name: session.user.name, email: propia },
    to: Array.isArray(body.to) ? body.to : [],
    subject: body.subject ?? '(Sin asunto)',
    ...(body.cc ? { cc: body.cc } : {}),
    textBody: body.textBody ?? '',
    ...(body.htmlBody != null ? { htmlBody: body.htmlBody } : {}),
    ...(attachments.length ? { attachments } : {}),
  };

  try {
    const client = jmapClient(session.accessToken);
    const res = body.id ? await client.updateDraft(body.id, email) : await client.saveDraft(email);
    return json({ id: res.id });
  } catch (err) {
    return json({ error: 'jmap_error', message: String(err) }, 502);
  }
};

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const id = ctx.url.searchParams.get('id');
  if (!id) return json({ error: 'missing_id' }, 400);

  try {
    const draft = await jmapClient(session.accessToken).readDraft(id);
    return json({
      to: draft.to,
      cc: draft.cc,
      subject: draft.subject,
      textBody: draft.textBody,
      htmlBody: draft.htmlBody,
      // Base64 y no una URL de descarga aparte (como /api/attachments para un
      // correo YA enviado/recibido): esto vuelve a un formulario que sigue
      // editándose, y el compositor necesita los bytes en memoria para poder
      // reenviarlos en el siguiente guardado, no solo mostrarlos.
      attachments: draft.attachments.map((a) => ({
        name: a.name,
        type: a.type,
        contentBase64: Buffer.from(a.content).toString('base64'),
        ...(a.cid ? { contentId: a.cid } : {}),
      })),
    });
  } catch (err) {
    return json({ error: 'jmap_error', message: String(err) }, 502);
  }
};

export const DELETE: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const id = ctx.url.searchParams.get('id');
  if (!id) return json({ error: 'missing_id' }, 400);

  try {
    await jmapClient(session.accessToken).deleteDraft(id);
    return json({ ok: true });
  } catch (err) {
    return json({ error: 'jmap_error', message: String(err) }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
