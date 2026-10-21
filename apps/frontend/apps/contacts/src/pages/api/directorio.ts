/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * POST /api/directorio — pone al día la libreta «Organización».
 *
 * QUÉ RESUELVE. Las personas con las que se trabaja tenían que teclearse una a
 * una, y quien entraba nuevo en la organización no aparecía en la agenda de
 * nadie. Aquí se trae el directorio de Sytue ID y se crea en Contactos lo que
 * falte, en una libreta propia y separada de los contactos de siempre.
 *
 * LO QUE HAY YA NO SE TOCA, y es la decisión que hace esto útil: si has añadido
 * un móvil, una nota o un cumpleaños a una persona de la empresa, eso es TUYO y
 * vive solo en tu Contactos. Sincronizar volviendo a escribir los campos lo
 * borraría en la siguiente pasada. El precio es que un cambio de puesto o de
 * apellido en el directorio no refresca la ficha ya creada; cuando eso importe,
 * el sitio de arreglarlo es el servicio, guardando qué campos vinieron de fuera
 * para distinguir lo que tocó la persona de lo que no.
 *
 * A UNO MISMO NO SE AÑADE: la agenda es con quién trabajas, no un espejo.
 */
import type { APIRoute } from 'astro';
import { serverEnv } from '@sytue/auth/server';
import { getApiSession } from '../../lib/session.js';
import { CONTACTS_BASE_URL } from '../../lib/carddav.js';

export const prerender = false;

const env = serverEnv(import.meta.env);
const ID_BASE = (env.SYTUE_ID_BASE_URL ?? 'https://id.sytue.eu').replace(/\/$/, '');
const CONTACTS = CONTACTS_BASE_URL.replace(/\/$/, '');

/** Nombre de la colección CardDAV. Estable: es parte de la URL del recurso. */
const LIBRETA = 'organizacion';

interface MiembroDelDirectorio {
  user_id: string;
  /** La dirección principal. Se conserva por los clientes que ya la leían. */
  email: string;
  /**
   * TODAS las direcciones en las que esa persona recibe DENTRO de Sytue: su
   * buzón principal y sus alias, en el subdominio de la organización o en su
   * dominio propio verificado.
   *
   * No es lo mismo que `email` a secas y por eso existe: a quien se invita desde
   * fuera, el correo con el que entra es el de la invitación —su Gmail, el de su
   * empresa anterior— y escribir ahí sale de Sytue. Sytue ID ya no lo manda
   * salvo que su dominio esté verificado por la organización.
   */
  emails?: string[];
  name?: string;
  department?: string;
  title?: string;
  location?: string;
}

interface LibretaCruda {
  id: string;
  name?: string;
  display_name?: string;
}

/**
 * Cómo se llama la libreta, en el idioma de quien la estrena.
 *
 * El nombre se guarda UNA vez, al crearla, y luego es suyo: si la renombra, se
 * respeta. Por eso se coge el idioma del navegador en ese momento y no se
 * reescribe después. El identificador de la colección (`organizacion`) no
 * cambia nunca: es parte de la URL del recurso CardDAV.
 */
function nombreVisible(acceptLanguage: string | null): string {
  const idioma = (acceptLanguage ?? '').slice(0, 2).toLowerCase();
  const nombres: Record<string, string> = {
    es: 'Organización',
    en: 'Organisation',
    fr: 'Organisation',
    de: 'Organisation',
    it: 'Organizzazione',
  };
  return nombres[idioma] ?? nombres['en']!;
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const cabeceras = { authorization: `Bearer ${session.accessToken}`, accept: 'application/json' };

  // 1. Quién trabaja aquí. Una sola página: el tope del servicio es 500 y una
  //    organización mayor pide paginar, que es cuando toca hacerlo en el
  //    servicio y no aquí.
  const resDir = await fetch(`${ID_BASE}/api/v1/directory?limit=500`, { headers: cabeceras });
  if (!resDir.ok) return json({ error: 'directorio', status: resDir.status }, 502);
  const { items = [] } = (await resDir.json()) as { items?: MiembroDelDirectorio[] };
  const miembros = items.filter((m) => m.user_id !== session.user.id && m.email);
  /** Las direcciones de una persona, con la principal delante y sin repetidos. */
  const correosDe = (m: MiembroDelDirectorio): string[] => {
    const vistos = new Set<string>();
    return [m.email, ...(m.emails ?? [])].filter((c) => {
      const l = (c ?? '').toLowerCase();
      if (!l || vistos.has(l)) return false;
      vistos.add(l);
      return true;
    });
  };

  // 2. La libreta «Organización», creándola la primera vez.
  const resLibretas = await fetch(`${CONTACTS}/api/v1/address-books`, { headers: cabeceras });
  if (!resLibretas.ok) return json({ error: 'libretas', status: resLibretas.status }, 502);
  const { address_books: libretas = [] } = (await resLibretas.json()) as {
    address_books?: LibretaCruda[];
  };
  let libreta = libretas.find((l) => l.name === LIBRETA);
  if (!libreta) {
    const creada = await fetch(`${CONTACTS}/api/v1/address-books`, {
      method: 'POST',
      headers: { ...cabeceras, 'content-type': 'application/json' },
      body: JSON.stringify({
        name: LIBRETA,
        display_name: nombreVisible(ctx.request.headers.get('accept-language')),
        color: '#8A6A2F',
      }),
    });
    if (!creada.ok) return json({ error: 'crear_libreta', status: creada.status }, 502);
    libreta = (await creada.json()) as LibretaCruda;
  }

  // 3. Quién está ya, por correo: es lo que identifica a una persona entre el
  //    directorio y una ficha ya creada.
  const resFichas = await fetch(
    `${CONTACTS}/api/v1/address-books/${encodeURIComponent(libreta.id)}/contacts?limit=500`,
    { headers: cabeceras },
  );
  if (!resFichas.ok) return json({ error: 'contactos', status: resFichas.status }, 502);
  const { contacts = [] } = (await resFichas.json()) as {
    contacts?: { emails?: { email?: string }[] }[];
  };
  const yaEstan = new Set(
    contacts.flatMap((c) => (c.emails ?? []).map((e) => (e.email ?? '').toLowerCase())),
  );

  // 4. Los que faltan. Uno a uno y sin abortar: que una ficha falle no puede
  //    dejar fuera a las demás.
  let creados = 0;
  const fallos: string[] = [];
  for (const m of miembros) {
    const correos = correosDe(m);
    // Basta con que YA esté una cualquiera de sus direcciones: la persona es la
    // misma y volver a crearla la duplicaría con otro alias delante.
    if (correos.some((c) => yaEstan.has(c.toLowerCase()))) continue;
    const cuerpo = {
      formatted_name: m.name?.trim() || correos[0],
      organization: m.department ?? '',
      title: m.title ?? '',
      // TODAS sus direcciones, no solo la principal: al buscar en la agenda por
      // un alias —que es como mucha gente conoce a un compañero— la ficha tiene
      // que aparecer igual.
      emails: correos.map((email, i) => ({ email, type: 'work', is_primary: i === 0 })),
    };
    const res = await fetch(
      `${CONTACTS}/api/v1/address-books/${encodeURIComponent(libreta.id)}/contacts`,
      {
        method: 'POST',
        headers: { ...cabeceras, 'content-type': 'application/json' },
        body: JSON.stringify(cuerpo),
      },
    );
    if (res.ok) creados++;
    else fallos.push(m.email);
  }

  return json({ addressBookId: libreta.id, miembros: miembros.length, creados, fallos });
};
