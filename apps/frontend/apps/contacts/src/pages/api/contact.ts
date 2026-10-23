/**
 * Crear/editar/eliminar un contacto CardDAV (servicio Go Sytue Contacts).
 *
 * POST   /api/contact -> crea o actualiza: { addressBookUrl, contactInput, href?, etag? }
 *                        Si no hay href, se genera <addressBookUrl>/<uid>.vcf.
 * DELETE /api/contact -> elimina: { href }
 *
 * El cuerpo vCard lo genera el servidor con buildVcard (no se confia en el
 * cliente para la vCard). Proxy BFF autorizado por cookie.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { cardDavClient, rutaDav } from '../../lib/carddav.js';
import { buildVcard, newUid, type ContactInput } from '../../lib/vcard.js';

export const prerender = false;

interface PostPayload {
  addressBookUrl?: string;
  contactInput?: Partial<ContactInput>;
  href?: string;
  etag?: string;
}

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const body = (await ctx.request.json().catch(() => ({}))) as PostPayload;
  const addressBookUrl = rutaDav(body.addressBookUrl);
  if (!addressBookUrl || !body.contactInput?.fullName) {
    return json({ error: 'invalid_payload' }, 400);
  }
  // Un `href` propio también tiene que ser del servicio (o no venir).
  if (body.href !== undefined && rutaDav(body.href) === null) {
    return json({ error: 'invalid_payload' }, 400);
  }

  const uid = body.contactInput.uid || newUid();
  const contact: ContactInput = {
    uid,
    fullName: body.contactInput.fullName,
    firstName: body.contactInput.firstName,
    lastName: body.contactInput.lastName,
    emails: body.contactInput.emails ?? [],
    phones: body.contactInput.phones ?? [],
    // Campos tipados del editor (etiqueta + principal), dirección y cumpleaños.
    emailsDetailed: body.contactInput.emailsDetailed,
    phonesDetailed: body.contactInput.phonesDetailed,
    addresses: body.contactInput.addresses,
    birthday: body.contactInput.birthday,
    organization: body.contactInput.organization,
    title: body.contactInput.title,
    note: body.contactInput.note,
    // vCard base para conservar campos no proyectados (PHOTO/X-*…, y ADR/BDAY si
    // el editor no los gestiona).
    baseVcard: body.contactInput.baseVcard,
  };
  const vcard = buildVcard(contact);
  const href = body.href ?? `${addressBookUrl.replace(/\/$/, '')}/${encodeURIComponent(uid)}.vcf`;

  try {
    await cardDavClient(session.accessToken, session.user.id).putContact(href, vcard, body.etag);
    return json({ ok: true, href, uid });
  } catch (err) {
    return json({ error: 'carddav_error', message: 'CardDAV request failed' }, 502);
  }
};

export const DELETE: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const body = (await ctx.request.json().catch(() => ({}))) as { href?: string };
  const href = rutaDav(body.href);
  if (!href) return json({ error: 'missing_href' }, 400);

  try {
    await cardDavClient(session.accessToken, session.user.id).deleteContact(href);
    return json({ ok: true });
  } catch (err) {
    return json({ error: 'carddav_error', message: 'CardDAV request failed' }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
