/**
 * GET /api/contacts?q= — autocompletado de contactos para Para/CC/CCO. Proxy BFF
 * (autorizado por cookie de sesión) hacia el servicio Go Sytue Contacts
 * (GET /api/v1/contacts?q=, mismo JWT de Sytue ID). Devuelve una lista compacta
 * { contacts: [{ name, email }] } — una entrada por dirección. Best-effort: si
 * Contacts rechaza o no responde, devuelve lista vacía (la redacción sigue como
 * texto libre; nunca rompe el compositor).
 *
 * GET /api/contacts?email= — ¿está ya esa dirección en la libreta? { id } o { id: null }
 * (MAIL-12: el lector ofrece «Ver en Contactos» en vez de «Añadir»).
 *
 * POST /api/contacts { name, email } — «Añadir a Contactos» desde el remitente: crea la
 * ficha en la libreta por defecto (POST /api/v1/address-books/{id}/contacts). Si la
 * dirección ya estaba, no crea otra: devuelve la que hay con `created: false`, que es
 * lo que hace falta cuando se pulsa dos veces o desde dos pestañas.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { serverEnv } from '@sytue/auth/server';
import {
  contactoConCorreo,
  nombreDelContacto,
  type ContactoDelServicio,
} from '../../lib/contactoRemitente.js';

export const prerender = false;

const env = serverEnv(import.meta.env);
const CONTACTS_BASE_URL = env.SYTUE_CONTACTS_BASE_URL ?? 'https://contacts.sytue.eu';

/** Máximo de sugerencias devueltas (denso, sin abrumar). */
const MAX_SUGGESTIONS = 8;

interface RawContact {
  formatted_name?: string;
  emails?: { email?: string; is_primary?: boolean }[];
}

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ contacts: [] }, 401);

  const email = ctx.url.searchParams.get('email');
  if (email !== null) {
    const correo = email.trim();
    if (!correo.includes('@')) return json({ id: null });
    try {
      return json({ id: await buscarPorCorreo(session.accessToken, correo) });
    } catch {
      return json({ error: 'contacts_unavailable' }, 502);
    }
  }

  const q = (ctx.url.searchParams.get('q') ?? '').trim();
  if (q.length < 2) return json({ contacts: [] });

  try {
    const res = await fetch(
      `${CONTACTS_BASE_URL}/api/v1/contacts?q=${encodeURIComponent(q)}&limit=${MAX_SUGGESTIONS}`,
      { headers: { authorization: `Bearer ${session.accessToken}`, accept: 'application/json' } },
    );
    if (!res.ok) return json({ contacts: [] });
    const data = (await res.json()) as { contacts?: RawContact[] };
    const out: { name: string; email: string }[] = [];
    const seen = new Set<string>();
    for (const c of data.contacts ?? []) {
      // Ordena la dirección principal primero; una entrada por correo.
      const emails = [...(c.emails ?? [])].sort(
        (a, b) => Number(b.is_primary) - Number(a.is_primary),
      );
      for (const e of emails) {
        const email = (e.email ?? '').trim();
        const key = email.toLowerCase();
        if (!email || seen.has(key)) continue;
        seen.add(key);
        out.push({ name: (c.formatted_name ?? '').trim(), email });
        if (out.length >= MAX_SUGGESTIONS) break;
      }
      if (out.length >= MAX_SUGGESTIONS) break;
    }
    return json({ contacts: out });
  } catch {
    return json({ contacts: [] });
  }
};

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let cuerpo: { name?: unknown; email?: unknown };
  try {
    cuerpo = (await ctx.request.json()) as typeof cuerpo;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }
  const correo = typeof cuerpo.email === 'string' ? cuerpo.email.trim() : '';
  const nombre = typeof cuerpo.name === 'string' ? cuerpo.name.slice(0, 256) : '';
  // Un correo de verdad y nada más: el BFF no es un alta de contactos genérica.
  if (!/^[^\s@]+@[^\s@]+$/.test(correo) || correo.length > 320) {
    return json({ error: 'invalid_email' }, 400);
  }

  const token = session.accessToken;
  try {
    const existente = await buscarPorCorreo(token, correo);
    if (existente) return json({ id: existente, created: false });

    const libretas = await servicio<{ address_books?: { id: string; is_default?: boolean }[] }>(
      token,
      '/api/v1/address-books',
    );
    const lista = libretas.address_books ?? [];
    // El servicio crea la libreta por defecto al listar si aún no había ninguna.
    const libreta = lista.find((b) => b.is_default) ?? lista[0];
    if (!libreta) return json({ error: 'contacts_unavailable' }, 502);

    const creado = await servicio<{ id?: string }>(
      token,
      `/api/v1/address-books/${encodeURIComponent(libreta.id)}/contacts`,
      {
        formatted_name: nombreDelContacto(nombre, correo),
        emails: [{ email: correo, is_primary: true }],
      },
    );
    if (!creado.id) return json({ error: 'contacts_unavailable' }, 502);
    return json({ id: creado.id, created: true }, 201);
  } catch {
    return json({ error: 'contacts_unavailable' }, 502);
  }
};

/** El id del contacto con esa dirección exacta, o null. Lanza si Contacts no contesta. */
async function buscarPorCorreo(token: string, correo: string): Promise<string | null> {
  const data = await servicio<{ contacts?: ContactoDelServicio[] }>(
    token,
    `/api/v1/contacts?q=${encodeURIComponent(correo)}&limit=20`,
  );
  return contactoConCorreo(data.contacts ?? [], correo);
}

/** Una llamada a Contacts con el token de la sesión: GET, o POST si lleva cuerpo. */
async function servicio<T>(token: string, ruta: string, cuerpo?: unknown): Promise<T> {
  const res = await fetch(`${CONTACTS_BASE_URL}${ruta}`, {
    method: cuerpo === undefined ? 'GET' : 'POST',
    headers: {
      authorization: `Bearer ${token}`,
      accept: 'application/json',
      ...(cuerpo === undefined ? {} : { 'content-type': 'application/json' }),
    },
    ...(cuerpo === undefined ? {} : { body: JSON.stringify(cuerpo) }),
  });
  if (!res.ok) throw new Error(`contacts ${res.status}`);
  return (await res.json()) as T;
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
