/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * POST /api/pdf-pages — organiza las páginas de un PDF de Drive (Sytue PDF, ola P4:
 * extraer, borrar, rotar). El motor vive en el servicio Docs y el fichero en Drive,
 * así que este BFF hace de puente: descarga el original con el token de la sesión,
 * lo pasa por `POST /api/v1/pdf/pages` de Docs y SUBE el resultado a Drive junto al
 * original. El token nunca llega al cliente.
 *
 * Se guarda en Drive en vez de descargarse porque el fichero ya estaba ahí: hacer
 * que el usuario baje el resultado y lo vuelva a subir sería devolverle el trabajo.
 * Y se guarda como fichero NUEVO, nunca sobreescribiendo el original: una
 * operación de páginas es destructiva y aquí no hay deshacer.
 *
 * CSRF: POST con content-type application/json (fuerza preflight) + cookie
 * same-origin, igual que el resto de BFF de la app.
 */
import type { APIRoute } from 'astro';
import { HttpError, type DriveClient } from '@sytue/api-clients';
import { bff, toBffContext, driveClientFor, docsBaseUrl } from '../../lib/auth';

export const prerender = false;

/** Operaciones que este endpoint acepta; Docs revalida y aplica la suya. */
const OPERACIONES = new Set([
  'extraer',
  'borrar',
  'rotar',
  'combinar',
  'marca-agua',
  'numerar',
  'bates',
  'encabezado',
  'pie',
]);

/** Las que SUPERPONEN texto: llevan `texto` en vez de `paginas`. */
const CAPAS = new Set(['marca-agua', 'numerar', 'bates', 'encabezado', 'pie']);

/** Cota del PDF que se acepta procesar, alineada con el límite del servicio Docs. */
const MAX_BYTES = 64 * 1024 * 1024;

/** Tope de ficheros a combinar de una vez, alineado con el del servicio. */
const MAX_FICHEROS = 64;

interface Cuerpo {
  path?: unknown;
  paths?: unknown; // solo para combinar: varios ficheros, en el orden dado
  op?: unknown;
  paginas?: unknown;
  grados?: unknown;
  sufijo?: unknown; // palabra para el nombre del resultado, en el idioma del usuario
  // Campos de las operaciones de capa.
  texto?: unknown;
  posicion?: unknown;
  desde?: unknown;
  digitos?: unknown;
}

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let cuerpo: Cuerpo;
  try {
    cuerpo = (await ctx.request.json()) as Cuerpo;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }

  const path = typeof cuerpo.path === 'string' ? cuerpo.path.trim() : '';
  const op = typeof cuerpo.op === 'string' ? cuerpo.op.trim() : '';
  const paginas = typeof cuerpo.paginas === 'string' ? cuerpo.paginas.trim() : '';
  // Solo número o texto: cualquier otra cosa se ignora en vez de acabar como
  // «[object Object]» en el formulario que se le manda al servicio.
  const grados =
    typeof cuerpo.grados === 'number'
      ? String(cuerpo.grados)
      : typeof cuerpo.grados === 'string'
        ? cuerpo.grados.trim()
        : '';

  if (!OPERACIONES.has(op)) return json({ error: 'bad_op' }, 400);

  // `combinar` trabaja sobre VARIOS ficheros y sin lista de páginas; el resto,
  // sobre uno y con ella. Se normalizan aquí a una sola lista de rutas.
  const combinar = op === 'combinar';
  const rutas = combinar
    ? (Array.isArray(cuerpo.paths) ? cuerpo.paths : []).filter(
        (p): p is string => typeof p === 'string' && p.trim() !== '',
      )
    : [path];

  const esCapa = CAPAS.has(op);
  if (combinar) {
    if (rutas.length < 2) return json({ error: 'need_two_files' }, 400);
    if (rutas.length > MAX_FICHEROS) return json({ error: 'too_many_files' }, 400);
  } else {
    if (!path) return json({ error: 'missing_path' }, 400);
    // Las capas se aplican a TODAS las páginas: no llevan lista.
    if (!esCapa && !paginas) return json({ error: 'missing_pages' }, 400);
  }

  const client = await driveClientFor(ctx);
  if (!client) return json({ error: 'unauthenticated' }, 401);

  // 1. Los originales. La verja la aplica Drive: sin acceso a uno de ellos, esto
  // ya falla y no se procesa nada — no hay resultado a medias.
  const form = new FormData();
  let total = 0;
  for (const ruta of rutas) {
    let original: Blob;
    try {
      original = await client.download(ruta);
    } catch {
      return json({ error: 'download_failed' }, 404);
    }
    total += original.size;
    if (original.size > MAX_BYTES || total > MAX_BYTES) {
      return json({ error: 'too_large' }, 413);
    }
    // El orden en que se añaden al formulario es el orden en que se combinan.
    form.append('file', original, nombreDe(ruta));
  }

  // 2. El motor, en el servicio Docs.
  form.append('op', op);
  if (!combinar && !esCapa) form.append('paginas', paginas);
  if (grados) form.append('grados', grados);
  if (esCapa) {
    // El servicio valida longitud y rangos; aquí solo se normaliza el tipo.
    form.append('texto', texto(cuerpo.texto));
    if (typeof cuerpo.posicion === 'string') form.append('posicion', cuerpo.posicion);
    const desde = entero(cuerpo.desde);
    const digitos = entero(cuerpo.digitos);
    if (desde !== '') form.append('desde', desde);
    if (digitos !== '') form.append('digitos', digitos);
  }

  let res: Response;
  try {
    res = await fetch(`${docsBaseUrl()}/api/v1/pdf/pages`, {
      method: 'POST',
      headers: { authorization: `Bearer ${session.tokens.accessToken}` },
      body: form,
    });
  } catch {
    return json({ error: 'docs_unreachable' }, 502);
  }
  if (!res.ok) {
    // Los 4xx de Docs son accionables por el usuario (PDF con contraseña,
    // páginas fuera de rango): su mensaje se propaga tal cual en vez de
    // aplanarlo a un «error» que no dice nada.
    const detalle = await res.text().catch(() => '');
    return json(
      { error: 'pdf_failed', message: mensajeDe(detalle) },
      res.status >= 500 ? 502 : res.status,
    );
  }
  const salida = await res.blob();

  // 3. De vuelta a Drive, como fichero nuevo junto al primero de los originales.
  try {
    const destino = await destinoLibre(client, rutas[0]!, op, cuerpo.sufijo);
    const recurso = await client.upload(destino, salida, 'application/pdf');
    return json({ resource: recurso }, 201);
  } catch {
    return json({ error: 'upload_failed' }, 502);
  }
};

/** Normaliza a texto lo que llegue, sin caer en «[object Object]». */
function texto(v: unknown): string {
  return typeof v === 'string' ? v : '';
}

/** Normaliza a número escrito lo que llegue; vacío si no es un número. */
function entero(v: unknown): string {
  if (typeof v === 'number' && Number.isFinite(v)) return String(Math.trunc(v));
  if (typeof v === 'string' && /^\d+$/.test(v.trim())) return v.trim();
  return '';
}

/** Último segmento de la ruta. */
function nombreDe(path: string): string {
  const partes = path.split('/').filter(Boolean);
  return partes[partes.length - 1] ?? 'documento.pdf';
}

/**
 * Ruta del resultado: junto al original, con una palabra que dice qué se hizo y
 * —si ya existe— un número, como cualquier gestor de archivos.
 *
 *   Contrato.pdf → «Contrato (extraído).pdf» → «Contrato (extraído) 2.pdf»
 *
 * Antes llevaba la marca de tiempo entera («Contrato (extraer 2026-08-08-14-33-02).pdf»)
 * porque `upload` NO avisa de que el nombre ya exista: sin nada que lo hiciera
 * único, repetir la operación pisaba el resultado anterior. El sello resolvía eso
 * al precio de un nombre que nadie quiere leer en una lista de ficheros, así que
 * ahora se pregunta: si el nombre bonito está libre, se usa.
 *
 * Solo un 404 dice que un nombre está libre. Cualquier otro fallo —red, permiso—
 * NO lo dice, y darlo por libre sobreescribiría un fichero ajeno, así que en ese
 * caso se corta y se cae al sello de tiempo, que no choca con nada.
 */
async function destinoLibre(
  client: DriveClient,
  path: string,
  op: string,
  sufijo: unknown,
): Promise<string> {
  const partes = path.split('/').filter(Boolean);
  const nombre = partes.pop() ?? 'documento.pdf';
  const base = nombre.replace(/\.pdf$/i, '');
  const etiqueta = limpiarSufijo(sufijo) || op;
  const ruta = (cola: string): string =>
    '/' + [...partes, `${base} (${etiqueta}${cola}).pdf`].join('/');

  // Diez intentos: más allá, el problema no es el nombre.
  for (let n = 1; n <= 10; n++) {
    const candidato = ruta(n === 1 ? '' : ` ${n}`);
    try {
      await client.stat(candidato);
    } catch (e) {
      if (e instanceof HttpError && e.status === 404) return candidato;
      break;
    }
  }
  const sello = new Date().toISOString().slice(0, 19).replace(/[:T]/g, '-');
  return ruta(` ${sello}`);
}

/**
 * La palabra del nombre la escribe el CLIENTE (va traducida a su idioma), así
 * que se acota a letras, números, espacios y guiones: ni «/», ni «..», ni saltos
 * de línea pueden llegar a formar parte de una ruta. Vacío = el que llame decide
 * el respaldo.
 */
function limpiarSufijo(v: unknown): string {
  if (typeof v !== 'string') return '';
  return v
    .normalize('NFC')
    .replace(/[^\p{L}\p{N} -]/gu, '')
    .replace(/\s+/g, ' ')
    .trim()
    .slice(0, 24)
    .trim();
}

/** Extrae el mensaje legible del error de Docs, si lo trae. */
function mensajeDe(cuerpo: string): string | undefined {
  try {
    const j = JSON.parse(cuerpo) as { message?: unknown; error?: unknown };
    if (typeof j.message === 'string' && j.message !== '') return j.message;
    if (typeof j.error === 'string' && j.error !== '') return j.error;
  } catch {
    // cuerpo no-JSON: no hay nada que propagar
  }
  return undefined;
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
