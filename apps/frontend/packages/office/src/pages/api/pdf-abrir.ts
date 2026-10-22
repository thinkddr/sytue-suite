/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * POST /api/pdf-abrir?name=…  (cuerpo CRUDO, application/octet-stream)
 *
 * Abre un PDF que está en el ORDENADOR, no en Drive.
 *
 * Hasta ahora el visor solo sabía trabajar con un documento ya guardado
 * (`?path=`), así que para mirar un fichero recién descargado había que subirlo
 * a mano primero. En iLovePDF arrastras el fichero y ya está, y es lo que espera
 * cualquiera; también es lo que hace falta para que la app de escritorio pueda
 * abrir lo que hay en el disco.
 *
 * A DIFERENCIA de `/api/import`, aquí NO se convierte nada: un PDF se guarda como
 * PDF. Se sube a Drive porque es donde viven los documentos de la suite —de ahí
 * salen los permisos, las versiones y el compartir—, y porque todas las
 * operaciones del estudio (firmar, censurar, reconocer) trabajan sobre una ruta.
 *
 * Cuerpo crudo y nombre en `?name=` y no multipart: así no dispara el
 * `checkOrigin` (CSRF) de Astro. Mismo patrón que el resto de subidas.
 */
import type { APIRoute } from 'astro';
import { DriveClient, bearer } from '@sytue/api-clients';
import { bff, toBffContext, driveBaseUrl } from '../../lib/auth';

export const prerender = false;

/**
 * El tope del motor (`domain.MaxPDFBytes`). Aquí solo se sube a Drive y
 * cabría más, pero abrir es para MIRARLO: guardar un PDF que el visor no va a
 * poder dibujar nunca —cada operación pasa por el motor— es dejarle al usuario
 * un fichero muerto en su carpeta en vez de decírselo en la puerta.
 */
const MAX_BYTES = 100 * 1024 * 1024;

/** Carpeta donde aterrizan los ficheros que se abren desde el ordenador. */
const CARPETA = '/PDF';

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const len = Number(ctx.request.headers.get('content-length') ?? '0');
  if (Number.isFinite(len) && len > MAX_BYTES) return json({ error: 'too_large' }, 413);

  const nombre = nombreSeguro(ctx.url.searchParams.get('name'));
  const datos = await ctx.request.arrayBuffer();
  if (datos.byteLength === 0) return json({ error: 'empty' }, 400);
  if (datos.byteLength > MAX_BYTES) return json({ error: 'too_large' }, 413);

  // Se comprueba que ES un PDF antes de guardarlo. El nombre lo pone quien sube y
  // no dice nada: un `.pdf` puede traer cualquier cosa, y guardarlo igual llenaría
  // el Drive de ficheros que el visor no puede abrir.
  const cabecera = new Uint8Array(datos.slice(0, 5));
  if (String.fromCharCode(...cabecera) !== '%PDF-') {
    return json({ error: 'not_a_pdf' }, 415);
  }

  const client = new DriveClient({
    baseUrl: driveBaseUrl(),
    auth: bearer(session.tokens.accessToken),
  });

  try {
    // La carpeta puede no existir la primera vez; crearla es parte de abrir.
    await client.createFolder(CARPETA).catch(() => undefined);
    const destino = await rutaLibre(client, nombre);
    const recurso = await client.upload(
      destino,
      new Blob([datos], { type: 'application/pdf' }),
      'application/pdf',
    );
    return json({ path: destino, resource: recurso }, 201);
  } catch {
    return json({ error: 'upload_failed' }, 502);
  }
};

/**
 * Nombre de fichero utilizable. Lo escribe quien sube, así que de aquí saldría
 * un «../» que escribiera fuera de su sitio.
 */
function nombreSeguro(crudo: string | null): string {
  let n = (crudo ?? '').trim();
  const corte = Math.max(n.lastIndexOf('/'), n.lastIndexOf('\\'));
  if (corte >= 0) n = n.slice(corte + 1);
  // Los caracteres de control SÍ hay que quitarlos, que es justo lo que la regla
  // desaconseja buscar: un salto de línea dentro de un nombre de fichero acaba
  // partiendo la cabecera que lo transporta, y un NUL lo rechaza la base de datos.
  // Iban aquí escritos como BYTES CRUDOS, así que el fichero entero contaba como
  // binario para git y ningún cambio suyo se podía leer en una revisión.
  // eslint-disable-next-line no-control-regex
  n = n.replace(/[\x00-\x1f\x7f"*:<>?|]/g, '_').trim();
  if (!/\.pdf$/i.test(n)) n += '.pdf';
  if (n.length > 120) n = n.slice(-120);
  return n === '.pdf' ? 'documento.pdf' : n;
}

/**
 * Ruta libre dentro de la carpeta, numerando si hace falta.
 *
 * Solo un 404 confirma que un nombre está libre: cualquier otro fallo —red,
 * permiso— NO lo confirma, y darlo por libre sobrescribiría un fichero ajeno.
 * Ahí se cae a un sufijo de tiempo, que no choca con nada.
 */
async function rutaLibre(client: DriveClient, nombre: string): Promise<string> {
  const base = nombre.replace(/\.pdf$/i, '');
  for (let i = 0; i < 20; i++) {
    const ruta = `${CARPETA}/${base}${i === 0 ? '' : ` (${i})`}.pdf`;
    try {
      await client.stat(ruta);
    } catch (e) {
      if (e instanceof Error && /404|not.?found/i.test(e.message)) return ruta;
      break;
    }
  }
  return `${CARPETA}/${base} (${Date.now()}).pdf`;
}

function json(cuerpo: unknown, status: number): Response {
  return new Response(JSON.stringify(cuerpo), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
