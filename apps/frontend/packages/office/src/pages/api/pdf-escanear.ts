/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * POST /api/pdf-escanear   { imagenes: [dataURL…], nombre?: string }
 *
 * Escanear con la cámara: varias fotos → UN PDF de varias páginas.
 *
 * Es la última herramienta de iLovePDF que faltaba, y en la web no hace falta
 * más que esto: `<input capture>` abre la cámara del teléfono y devuelve las
 * fotos como ficheros normales. El motor ya sabía montar el PDF
 * (`ImagenesAPDF`, una página por imagen, sin recomprimir el JPEG); lo que no
 * había era por dónde entrar.
 *
 * UNA petición con todas las fotos y no una por foto: un escaneo de diez hojas
 * es UN documento, y subirlas de una en una dejaría diez PDF sueltos que luego
 * habría que combinar a mano —además de diez ficheros en Drive si algo falla a
 * la mitad—.
 *
 * Las imágenes viajan como data URL dentro de un JSON, que es el patrón del
 * resto del estudio: multipart desde el navegador dispara el `checkOrigin`
 * (CSRF) de Astro.
 */
import type { APIRoute } from 'astro';
import { DriveClient, bearer } from '@sytue/api-clients';
import { bff, toBffContext, docsBaseUrl, driveBaseUrl } from '../../lib/auth';

export const prerender = false;

/** El tope del motor (`domain.MaxImportBytes`), que vale para el cuerpo entero:
 *  las cincuenta hojas van en UNA petición. Ver `pdf.ts`. */
const MAX_BYTES = 25 * 1024 * 1024;

/** Carpeta donde aterrizan los escaneos. La misma que la de abrir. */
const CARPETA = '/PDF';

/**
 * Tope de hojas por escaneo. Un documento de oficina no pasa de aquí, y sin
 * tope una petición con mil fotos ata el servicio con un cuerpo de pocos megas.
 */
const MAX_HOJAS = 50;

/** Lo que el motor sabe convertir en página. Se mira el tipo declarado en el
 *  data URL; quien decide de verdad es el motor al leer los bytes. */
const TIPOS = /^image\/(jpeg|png|gif|webp|bmp|tiff)$/;

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let cuerpo: { imagenes?: unknown; nombre?: unknown };
  try {
    cuerpo = (await ctx.request.json()) as typeof cuerpo;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }

  const crudas = Array.isArray(cuerpo.imagenes) ? cuerpo.imagenes : [];
  if (crudas.length === 0) return json({ error: 'sin_imagenes' }, 400);
  if (crudas.length > MAX_HOJAS) return json({ error: 'demasiadas' }, 413);

  const form = new FormData();
  let total = 0;
  for (const [i, cruda] of crudas.entries()) {
    const img = deDataURL(typeof cruda === 'string' ? cruda : '');
    if (!img) return json({ error: 'imagen_invalida', pagina: i + 1 }, 415);
    total += img.datos.byteLength;
    // El tope se mira SOBRE LA MARCHA y no al final: con cincuenta fotos de
    // móvil se pasa de sobra, y decodificarlas todas antes de comprobarlo es
    // justo el gasto que el tope existe para evitar.
    if (total > MAX_BYTES) return json({ error: 'too_large' }, 413);
    form.append('file', new Blob([img.datos], { type: img.tipo }), `hoja-${i + 1}`);
  }
  form.append('op', 'imagenes-a-pdf');

  let hecho: Response;
  try {
    hecho = await fetch(`${docsBaseUrl()}/api/v1/pdf/tools`, {
      method: 'POST',
      headers: { authorization: `Bearer ${session.tokens.accessToken}` },
      body: form,
    });
  } catch {
    return json({ error: 'docs_unreachable' }, 502);
  }
  if (!hecho.ok) return json({ error: 'scan_failed' }, hecho.status === 413 ? 413 : 400);
  const pdf = await hecho.blob();

  const client = new DriveClient({
    baseUrl: driveBaseUrl(),
    auth: bearer(session.tokens.accessToken),
  });
  try {
    await client.createFolder(CARPETA).catch(() => undefined);
    const destino = await rutaLibre(client, nombreSeguro(cuerpo.nombre));
    const recurso = await client.upload(destino, pdf, 'application/pdf');
    return json({ path: destino, resource: recurso, paginas: crudas.length }, 201);
  } catch {
    return json({ error: 'upload_failed' }, 502);
  }
};

/**
 * Saca los bytes de un `data:image/jpeg;base64,…`.
 *
 * Se comprueba el tipo declarado ANTES de decodificar: un data URL de 60 MB con
 * `text/html` dentro no tiene por qué llegar siquiera a base64.
 */
// `Uint8Array<ArrayBuffer>` y no `Uint8Array` a secas: sin el parámetro, TS lo
// ensancha a `ArrayBufferLike` —que incluye SharedArrayBuffer— y `BlobPart` lo
// rechaza, así que el `new Blob([img.datos])` de arriba no compila. El valor que
// se devuelve ya es exactamente eso; lo único que faltaba era decirlo.
function deDataURL(url: string): { datos: Uint8Array<ArrayBuffer>; tipo: string } | null {
  const m = /^data:([^;,]+);base64,(.+)$/s.exec(url);
  if (!m?.[1] || !m[2] || !TIPOS.test(m[1])) return null;
  try {
    const bin = atob(m[2]);
    const datos = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; i++) datos[i] = bin.charCodeAt(i);
    return datos.byteLength > 0 ? { datos, tipo: m[1] } : null;
  } catch {
    // base64 mal formado: es un error de quien sube, no del servicio.
    return null;
  }
}

/** Nombre utilizable. Lo escribe quien sube, así que de aquí saldría un «../». */
function nombreSeguro(crudo: unknown): string {
  let n = (typeof crudo === 'string' ? crudo : '').trim();
  const corte = Math.max(n.lastIndexOf('/'), n.lastIndexOf('\\'));
  if (corte >= 0) n = n.slice(corte + 1);
  n = n.replace(/[ -"*:<>?|]/g, '_').trim();
  if (!/\.pdf$/i.test(n)) n += '.pdf';
  if (n.length > 120) n = n.slice(-120);
  return n === '.pdf' ? 'escaneo.pdf' : n;
}

/**
 * Ruta libre dentro de la carpeta, numerando si hace falta.
 *
 * Solo un 404 confirma que un nombre está libre: cualquier otro fallo —red,
 * permiso— NO lo confirma, y darlo por libre sobrescribiría un fichero ajeno.
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
