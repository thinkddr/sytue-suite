/**
 * GET /api/contacts?q= — autocompletado de contactos para Para/CC/CCO. Proxy BFF
 * (autorizado por cookie de sesión) hacia el servicio Go Sytue Contacts
 * (GET /api/v1/contacts?q=, mismo JWT de Sytue ID). Devuelve una lista compacta
 * { contacts: [{ name, email }] } — una entrada por dirección. Best-effort: si
 * Contacts rechaza o no responde, devuelve lista vacía (la redacción sigue como
 * texto libre; nunca rompe el compositor).
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { serverEnv } from '@sytue/auth/server';

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

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
