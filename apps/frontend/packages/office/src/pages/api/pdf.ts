/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue PDF — LO QUE SE LEE del documento.
 *
 *   POST /api/pdf  { path, op: 'contenido' | 'render' | 'texto' | 'campos' | 'comparar' | 'verificar', … }
 *
 * El fichero vive en Drive y el motor en el servicio Docs, así que este puente
 * descarga el original con el token de la sesión y lo pasa por `/api/v1/pdf/*`.
 * El token NUNCA llega al navegador.
 *
 * POR QUÉ SE DESCARGA EN CADA LLAMADA en vez de subirlo desde el navegador: el
 * PDF ya está en Drive, y hacer que el usuario lo suba otra vez para verlo sería
 * devolverle el trabajo. El coste se acota pidiendo LOTES de páginas —el visor
 * pide la ventana que se está mirando de una vez— en vez de una llamada por
 * página.
 *
 * CSRF: POST con content-type application/json (fuerza preflight) + cookie
 * same-origin, igual que el resto de BFF de la suite.
 */
import type { APIRoute } from 'astro';
import { DriveClient, bearer, cuerpoDeError } from '@sytue/api-clients';
import { bff, toBffContext, docsBaseUrl, driveBaseUrl } from '../../lib/auth';

export const prerender = false;

/** Operaciones de lectura y a qué endpoint del motor van. */
const RUTAS: Record<string, string> = {
  contenido: '/pdf/content',
  render: '/pdf/render',
  texto: '/pdf/text',
  campos: '/pdf/form',
  indice: '/pdf/inspect',
  comparar: '/pdf/compare',
  verificar: '/pdf/verify',
  'analizar-edicion': '/pdf/edit',
  'comprobar-archivo': '/pdf/tools',
  'estado-ocr': '/pdf/ocr',
  adjunto: '/pdf/attachment',
  'detectar-campos': '/pdf/form',
  'exportar-datos': '/pdf/form',
  'buscar-texto': '/pdf/edit',
};

/**
 * Cota de lo que se acepta procesar, y ahora sí alineada con el motor: el
 * servicio corta en `domain.MaxPDFBytes` = 100 MiB, por fichero y también en
 * el cuerpo entero de la petición. El BFF rechaza antes de descargar de Drive y
 * reenviar al motor un fichero que éste nunca podrá abrir. A diferencia del
 * importador ofimático, el PDF tiene una cota propia: su lectura es perezosa y
 * no descomprime el documento entero al abrirlo.
 */
const MAX_BYTES = 100 * 1024 * 1024;

interface Cuerpo {
  /** Qué buscar y por qué cambiarlo (op `buscar-texto`). */
  busqueda?: unknown;
  /** Nombre del fichero incrustado a extraer (op `adjunto`). */
  nombre?: unknown;
  path?: unknown;
  path2?: unknown; // el segundo documento, solo para comparar
  op?: unknown;
  clave?: unknown; // contraseña de apertura, si el PDF la pide
  paginas?: unknown; // «1-4» para el render por lotes
  pagina?: unknown;
  escala?: unknown;
  /** Comparar: `texto` (lo de siempre) o `visual` (por píxeles). */
  modo?: unknown;
  /** Comparar en modo visual: diferencia de tono por canal (1-255) que se ignora. */
  tolerancia?: unknown;
  /**
   * `svg` (lo de siempre, y lo que usa esta web) o `png`. El PNG existe para el
   * cliente que no tiene un motor de SVG con texto detrás —el escritorio Fyne—;
   * pierde la selección y la lectura en voz alta, así que aquí no se usa.
   */
  formato?: unknown;
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

  const op = typeof cuerpo.op === 'string' ? cuerpo.op.trim() : '';
  const ruta = RUTAS[op];
  if (!ruta) return json({ error: 'bad_op' }, 400);

  const formato = texto(cuerpo.formato);
  if (formato !== '' && formato !== 'svg' && formato !== 'png') {
    return json({ error: 'bad_format' }, 400);
  }

  const rutas = [texto(cuerpo.path), texto(cuerpo.path2)].filter((p) => p !== '');
  if (rutas.length === 0) return json({ error: 'missing_path' }, 400);
  if (op === 'comparar' && rutas.length !== 2) return json({ error: 'need_two_files' }, 400);

  const client = new DriveClient({
    baseUrl: driveBaseUrl(),
    auth: bearer(session.tokens.accessToken),
  });
  const form = new FormData();
  let total = 0;
  for (const p of rutas) {
    let original: Blob;
    try {
      original = await client.download(p);
    } catch {
      return json({ error: 'download_failed' }, 404);
    }
    total += original.size;
    if (total > MAX_BYTES) return json({ error: 'too_large' }, 413);
    form.append('file', original, nombreDe(p));
  }

  if (texto(cuerpo.clave) !== '') form.append('clave', texto(cuerpo.clave));
  if (op === 'render') {
    if (texto(cuerpo.paginas) !== '') form.append('paginas', texto(cuerpo.paginas));
    if (entero(cuerpo.pagina) !== '') form.append('pagina', entero(cuerpo.pagina));
    if (typeof cuerpo.escala === 'number' && cuerpo.escala > 0) {
      form.append('escala', String(cuerpo.escala));
    }
    if (formato !== '') form.append('formato', formato);
  }
  if (op === 'comparar' && texto(cuerpo.modo) !== '') {
    if (texto(cuerpo.modo) !== 'texto' && texto(cuerpo.modo) !== 'visual') {
      return json({ error: 'bad_mode' }, 400);
    }
    form.append('modo', texto(cuerpo.modo));
    // Los límites de verdad los pone el motor, que rechaza lo que se pase; aquí solo
    // se deja pasar lo que tiene forma de número.
    if (entero(cuerpo.pagina) !== '') form.append('pagina', entero(cuerpo.pagina));
    if (entero(cuerpo.tolerancia) !== '') form.append('tolerancia', entero(cuerpo.tolerancia));
    if (typeof cuerpo.escala === 'number' && cuerpo.escala > 0) {
      form.append('escala', String(cuerpo.escala));
    }
  }
  if (op === 'campos') form.append('op', 'leer');
  if (op === 'exportar-datos') form.append('op', 'exportar-datos');
  if (op === 'buscar-texto') {
    form.append('op', 'buscar-reemplazar');
    form.append('busqueda', JSON.stringify(cuerpo.busqueda ?? {}));
  }
  if (op === 'detectar-campos') {
    form.append('op', 'detectar');
    if (entero(cuerpo.pagina) !== '') form.append('pagina', entero(cuerpo.pagina));
  }
  if (op === 'analizar-edicion') {
    form.append('op', 'analizar');
    if (entero(cuerpo.pagina) !== '') form.append('pagina', entero(cuerpo.pagina));
  }
  if (op === 'comprobar-archivo') form.append('op', 'comprobar-archivo');
  if (op === 'estado-ocr') form.append('op', 'estado');
  if (op === 'adjunto') {
    const nombre = texto(cuerpo.nombre);
    if (nombre === '') return json({ error: 'missing_name' }, 400);
    form.append('nombre', nombre);
  }

  let res: Response;
  try {
    res = await fetch(`${docsBaseUrl()}/api/v1${ruta}`, {
      method: 'POST',
      headers: { authorization: `Bearer ${session.tokens.accessToken}` },
      body: form,
    });
  } catch {
    return json({ error: 'docs_unreachable' }, 502);
  }
  // Una página en PNG también es binaria, pero es una IMAGEN que el cliente
  // pinta, no una descarga: se reenvía con su tipo y sin `content-disposition`.
  // Un lote de PNG viene en JSON con base64 y sigue el camino normal.
  if (op === 'render' && formato === 'png' && res.ok && texto(cuerpo.paginas) === '') {
    return new Response(await res.arrayBuffer(), {
      status: 200,
      headers: {
        'content-type': 'image/png',
        'x-content-type-options': 'nosniff',
        'cache-control': 'no-store',
      },
    });
  }
  // El adjunto es BINARIO: `reenviar` lee la respuesta como texto y eso
  // destrozaría cualquier fichero que no sea UTF-8 —un ZIP, una imagen—.
  if ((op === 'adjunto' || op === 'exportar-datos') && res.ok) {
    return new Response(await res.arrayBuffer(), {
      status: 200,
      headers: {
        // Se REESCRIBEN las dos cabeceras que deciden qué hace el navegador, sin
        // mirar lo que venga: el fichero lo eligió quien hizo el PDF, y honrar un
        // `text/html` suyo sería servir HTML ajeno desde el origen de la sesión.
        'content-type': 'application/octet-stream',
        'content-disposition':
          res.headers.get('content-disposition') ?? 'attachment; filename="descarga"',
        'x-content-type-options': 'nosniff',
        'cache-control': 'no-store',
      },
    });
  }
  return await reenviar(res);
};

/**
 * Reenvía la respuesta del motor. Un 4xx suyo es ACCIONABLE por el usuario —el
 * PDF pide contraseña, la página no existe— así que su mensaje y su código se
 * propagan tal cual en vez de aplanarse a un «error» que no dice nada: el código
 * `pdf_password_required` es lo que hace que la interfaz abra el diálogo de
 * contraseña en lugar de enseñar un aviso rojo y quedarse ahí.
 */
async function reenviar(res: Response): Promise<Response> {
  const bruto = await res.text().catch(() => '');
  if (res.ok) {
    return new Response(bruto, {
      status: res.status,
      headers: {
        'content-type': res.headers.get('content-type') ?? 'application/json; charset=utf-8',
        'cache-control': 'no-store',
      },
    });
  }
  const detalle = cuerpoDeError(bruto);
  return json(
    { error: 'pdf_failed', code: detalle.code, message: detalle.message },
    res.status >= 500 ? 502 : res.status,
  );
}

function texto(v: unknown): string {
  return typeof v === 'string' ? v.trim() : '';
}

function entero(v: unknown): string {
  if (typeof v === 'number' && Number.isFinite(v)) return String(Math.trunc(v));
  if (typeof v === 'string' && /^\d+$/.test(v.trim())) return v.trim();
  return '';
}

function nombreDe(path: string): string {
  const partes = path.split('/').filter(Boolean);
  return partes[partes.length - 1] ?? 'documento.pdf';
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
