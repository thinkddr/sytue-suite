import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { serverEnv } from '@sytue/auth/server';

export const prerender = false;

const env = serverEnv(import.meta.env);
const CONTACTS_BASE_URL = env.SYTUE_CONTACTS_BASE_URL ?? 'https://contacts.sytue.eu';
/** Tope por página del servicio (`/contacts/all`); más que eso no se recorre a mano. */
const PAGINA = 500;

interface RawContact {
  id?: string;
  formatted_name?: string;
  emails?: { email?: string; is_primary?: boolean }[];
}

export interface EntradaDeLibreta {
  id: string;
  name: string;
  /** Correos del contacto, el principal primero, sin repetidos. */
  emails: string[];
}

/**
 * La libreta entera de la persona, solo los contactos con correo. Es lo que abre
 * el icono de Contactos del redactor; el autocompletado del campo sigue pasando
 * por /api/contacts, que busca por texto. Sin libreta accesible se contesta 502:
 * el redactor lo dice y la redacción sigue como texto libre.
 */
export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  try {
    const res = await fetch(`${CONTACTS_BASE_URL}/api/v1/contacts/all?limit=${PAGINA}`, {
      headers: { authorization: `Bearer ${session.accessToken}`, accept: 'application/json' },
    });
    if (!res.ok) return json({ error: 'contacts_unavailable' }, 502);
    const data = (await res.json()) as { contacts?: RawContact[] };
    const contacts: EntradaDeLibreta[] = [];
    (data.contacts ?? []).forEach((c, i) => {
      const vistos = new Set<string>();
      const emails: string[] = [];
      for (const e of [...(c.emails ?? [])].sort(
        (a, b) => Number(b.is_primary) - Number(a.is_primary),
      )) {
        const email = (e.email ?? '').trim();
        const clave = email.toLowerCase();
        if (!email || vistos.has(clave)) continue;
        vistos.add(clave);
        emails.push(email);
      }
      if (emails.length > 0)
        contacts.push({ id: c.id ?? String(i), name: (c.formatted_name ?? '').trim(), emails });
    });
    return json({ contacts });
  } catch {
    return json({ error: 'contacts_unavailable' }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
