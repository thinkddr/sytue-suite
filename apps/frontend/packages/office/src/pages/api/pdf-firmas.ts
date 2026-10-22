/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue PDF — «Pedir firma» a otra persona de la organización
 * (services/docs/internal/http/pdfpedirfirma.go, ADR 0051 §7).
 *
 *   GET  /api/pdf-firmas                                  las recibidas y las pedidas
 *   POST /api/pdf-firmas { op: 'pedir', path, email, mensaje, confirm }
 *   POST /api/pdf-firmas { op: 'entregar', id, path }     el firmado, ya en Drive
 *   POST /api/pdf-firmas { op: 'abrir', id }              de fuera: el PDF, a mi unidad
 *   POST /api/pdf-firmas { op: 'traer', id }              de fuera: el firmado, a mi unidad
 *   POST /api/pdf-firmas { op: 'bloquear', id }           de fuera: bloquea a quien la pidió
 *   POST /api/pdf-firmas { op: 'desbloquear', sub }       quita un bloqueo (ADR 0059)
 *   POST /api/pdf-firmas { op: 'rechazar', id }
 *   POST /api/pdf-firmas { op: 'cancelar', id }
 *
 * La página trabaja con RUTAS de Drive y el servicio con identificadores: la ruta se
 * resuelve aquí, con la sesión de quien llama, así que nadie pide firmar ni entrega
 * un fichero que no puede ver. El destinatario va por su dirección y lo resuelve el
 * servicio contra Sytue ID; el navegador nunca elige un `sub`. Si es de otra
 * organización (ADR 0057), el servicio contesta 409 con su nombre hasta que llega
 * `confirm` (lo que trajo el 409), que es quien pide confirmando que el PDF sale por un
 * enlace de Drive hacia ESA persona.
 */
import type { APIRoute } from 'astro';
import { DriveClient, bearer } from '@sytue/api-clients';
import { bff, toBffContext, driveBaseUrl } from '../../lib/auth';
import { json, proxyDocs } from '../../lib/docsProxy';
import { resolverPersonas } from '../../lib/directorio';

export const prerender = false;

interface Peticion {
  id: string;
  requester: string;
  recipient: string;
  [k: string]: unknown;
}

/** El error del servicio (`{"error":{code,message}}`) aplanado a lo que lee `pedir`. */
async function plano(res: Response): Promise<Response> {
  if (res.ok) return res;
  const cuerpo = (await res.json().catch(() => ({}))) as {
    error?: { code?: string; message?: string; details?: unknown } | string;
  };
  const e: { code?: string | undefined; message?: string | undefined; details?: unknown } =
    typeof cuerpo.error === 'object' ? cuerpo.error : { code: cuerpo.error };
  return json(
    {
      code: e.code ?? 'error',
      message: e.message ?? '',
      ...(e.details ? { details: e.details } : {}),
    },
    res.status,
  );
}

export const GET: APIRoute = async (ctx) => {
  const res = await proxyDocs(ctx, '/pdf/signature-requests');
  if (!res.ok) return plano(res);
  const {
    received = [],
    sent = [],
    blocked = [],
  } = (await res.json()) as {
    received?: Peticion[];
    sent?: Peticion[];
    blocked?: unknown[];
  };
  // Quién pide y a quién se pidió, por su nombre y su correo: el servicio guarda `sub`.
  const personas = await resolverPersonas(ctx, [
    ...new Set([...received.map((p) => p.requester), ...sent.map((p) => p.recipient)]),
  ]);
  const con = (p: Peticion, sub: string) => {
    const x = personas.get(sub);
    return x ? { ...p, persona: { email: x.email, ...(x.name ? { name: x.name } : {}) } } : p;
  };
  return json({
    received: received.map((p) => con(p, p.requester)),
    sent: sent.map((p) => con(p, p.recipient)),
    blocked,
  });
};

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ code: 'unauthenticated' }, 401);
  let c: Record<string, unknown>;
  try {
    c = (await ctx.request.json()) as Record<string, unknown>;
  } catch {
    return json({ code: 'invalid_json' }, 400);
  }
  const texto = (k: string) => {
    const v = c[k];
    return typeof v === 'string' ? v.trim() : '';
  };
  const id = encodeURIComponent(texto('id'));
  const drive = new DriveClient({
    baseUrl: driveBaseUrl(),
    auth: bearer(session.tokens.accessToken),
  });
  const idDe = async (path: string) => {
    try {
      return (await drive.stat(path)).id;
    } catch {
      return '';
    }
  };
  const post = (ruta: string, cuerpo?: unknown) =>
    proxyDocs(ctx, `/pdf/signature-requests${ruta}`, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(cuerpo ?? {}),
    }).then(plano);

  switch (texto('op')) {
    case 'pedir': {
      const path = texto('path');
      const fichero = path ? await idDe(path) : '';
      if (!fichero) return json({ code: 'download_failed' }, 404);
      return post('', {
        file_id: fichero,
        path,
        email: texto('email'),
        message: texto('mensaje'),
        confirm: texto('confirm'),
      });
    }
    case 'entregar': {
      const path = texto('path');
      const fichero = path ? await idDe(path) : '';
      if (!fichero) return json({ code: 'download_failed' }, 404);
      return post(`/${id}/sign`, { signed_file_id: fichero, signed_path: path });
    }
    case 'rechazar':
      return post(`/${id}/decline`);
    case 'abrir':
      return post(`/${id}/open`);
    case 'traer':
      return post(`/${id}/fetch-signed`);
    case 'bloquear':
      return proxyDocs(ctx, `/pdf/signature-requests/${id}/block`, { method: 'POST' }).then((r) =>
        r.status === 204 ? json({ ok: true }) : plano(r),
      );
    case 'desbloquear':
      return proxyDocs(ctx, `/pdf/signature-blocks/${encodeURIComponent(texto('sub'))}`, {
        method: 'DELETE',
      }).then((r) => (r.status === 204 ? json({ ok: true }) : plano(r)));
    case 'cancelar':
      return proxyDocs(ctx, `/pdf/signature-requests/${id}`, { method: 'DELETE' }).then((r) =>
        r.status === 204 ? json({ ok: true }) : plano(r),
      );
  }
  return json({ code: 'bad_op' }, 400);
};
