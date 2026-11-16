/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Las versiones de un PDF de CertiPDF, por su RUTA en Drive.
 *
 * Cada anotación, relleno, censura o firma reescribe el fichero en Drive, y en el
 * visor no había forma de volver atrás: un sello en la página equivocada o una
 * censura de más se quedaban. Drive ya guarda cada escritura como una versión y
 * restaura sin perder nada —lo que era actual pasa a ser una versión más—, así que
 * aquí solo se le pide eso, con la ruta que conoce el visor en vez del id que pide
 * el servicio.
 *
 * La lógica vive aquí, separada de la ruta de Astro (`pages/api/pdf-versiones.ts`),
 * para poder probarla con un cliente de mentira sin arrastrar la sesión ni Astro.
 */
import { HttpError, type DriveClient } from '@sytue/api-clients';

/** Lo que se usa del cliente de Drive (lo que un test tiene que imitar). */
export type ClienteVersiones = Pick<
  DriveClient,
  'stat' | 'versions' | 'openVersionDownload' | 'restoreVersion'
>;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

/** Lo que dice Drive, traducido a lo que el visor sabe contar. */
function fallo(e: unknown): Response {
  const status = e instanceof HttpError ? e.status : 0;
  // 403: quien solo puede ver el PDF no puede restaurarlo (el servicio lo decide).
  if (status === 403) return json({ error: 'forbidden' }, 403);
  if (status === 404) return json({ error: 'not_found' }, 404);
  if (status === 400) return json({ error: 'invalid_version' }, 400);
  return json({ error: 'drive_unavailable' }, 502);
}

/** Solo PDF: esta ruta es la de CertiPDF y sirve el contenido como PDF. */
function rutaValida(ruta: string): boolean {
  return ruta.startsWith('/') && /\.pdf$/i.test(ruta);
}

/** Un número de versión de la consulta o del cuerpo: entero positivo o nada. */
function numeroDeVersion(v: unknown): number | null {
  const s = typeof v === 'number' ? String(v) : typeof v === 'string' ? v.trim() : '';
  if (!/^\d{1,9}$/.test(s)) return null;
  const n = Number(s);
  return n > 0 ? n : null;
}

/** El id del fichero de esa ruta; `null` si no es un fichero. */
async function idDe(cliente: ClienteVersiones, ruta: string): Promise<string | null> {
  const r = await cliente.stat(ruta);
  return r.isDirectory || !r.id ? null : r.id;
}

/** El nombre con el que se ve una versión: el del PDF y el número detrás. */
function nombreDeVersion(ruta: string, version: number): string {
  const nombre = ruta.split('/').filter(Boolean).pop() ?? 'documento.pdf';
  return `${nombre.replace(/\.pdf$/i, '')} (v${version}).pdf`;
}

/**
 * GET. Sin `version`, una página del historial (`{ versions, nextCursor? }`, la actual
 * primero). Con `version`, el PDF de esa versión para verlo en el navegador.
 */
export async function listarOAbrir(url: URL, cliente: ClienteVersiones): Promise<Response> {
  const ruta = (url.searchParams.get('path') ?? '').trim();
  if (!rutaValida(ruta)) return json({ error: 'missing_path' }, 400);
  const pedida = url.searchParams.get('version');
  const version = pedida === null ? null : numeroDeVersion(pedida);
  if (pedida !== null && version === null) return json({ error: 'invalid_version' }, 400);

  try {
    const id = await idDe(cliente, ruta);
    if (!id) return json({ error: 'not_found' }, 404);

    if (version === null) {
      const cursor = url.searchParams.get('cursor')?.trim() || undefined;
      return json(await cliente.versions(id, { cursor }));
    }

    const original = await cliente.openVersionDownload(id, version);
    // En streaming, sin leerlo: un escaneado son decenas de megas por petición.
    // `inline` porque «Abrir» es para VERLA (en una pestaña, con el visor del
    // navegador); el tipo va FORZADO a PDF con `nosniff`, así que aunque alguien
    // hubiera colado otra cosa con ese nombre, el navegador no la interpreta como
    // una página de este origen.
    return new Response(original.body, {
      status: 200,
      headers: {
        'content-type': 'application/pdf',
        'content-disposition': `inline; filename*=UTF-8''${encodeURIComponent(
          nombreDeVersion(ruta, version),
        )}`,
        'cache-control': 'no-store',
        'x-content-type-options': 'nosniff',
      },
    });
  } catch (e) {
    return fallo(e);
  }
}

/**
 * POST `{ path, version }`: hace actual esa versión. No destruye nada: lo que había
 * pasa a ser una versión más y se puede volver a ella igual.
 */
export async function restaurar(request: Request, cliente: ClienteVersiones): Promise<Response> {
  let cuerpo: { path?: unknown; version?: unknown };
  try {
    cuerpo = (await request.json()) as typeof cuerpo;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }
  const ruta = typeof cuerpo.path === 'string' ? cuerpo.path.trim() : '';
  if (!rutaValida(ruta)) return json({ error: 'missing_path' }, 400);
  const version = numeroDeVersion(cuerpo.version);
  if (version === null) return json({ error: 'invalid_version' }, 400);
  try {
    const id = await idDe(cliente, ruta);
    if (!id) return json({ error: 'not_found' }, 404);
    await cliente.restoreVersion(id, version);
    return json({ ok: true });
  } catch (e) {
    return fallo(e);
  }
}
