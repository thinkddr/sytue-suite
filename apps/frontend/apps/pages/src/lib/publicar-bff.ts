/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Publicar desde el BFF (PAGES-PLAN §7.4), SOLO servidor. El navegador no renderiza ni habla con el
 * servicio Go: pide `POST /web/api/publicar/{id}` con `{kind}` y aquí se hace todo con SU token:
 *
 *   1. `POST /snapshots {purpose}`            → `Snapshot` (IR + datos congelados). 409 si el IR no es el nuestro.
 *   2. `estimarSalida`                        → 413 `publication_too_large {files, bytes}` sin renderizar.
 *   3. vista previa: `POST /previews`         → `base_path` (lleva `X-Sytue-Publicador`).
 *   4. `publicar(ir, data, …)`                → `Salida` determinista.
 *   5. `POST /publications?kind=&token=`      → multipart en STREAMING (manifest primero, luego un `file`
 *                                               por fichero con `filename` = ruta) + `X-Sytue-Publicador`.
 *
 * El secreto del publicador solo vive aquí: no se devuelve, no se registra y no se manda a `/snapshots`
 * (esa ruta no lo exige). Los errores del servicio se reenvían con su estado y su `{error:{code,message}}`.
 */
import { createHash, randomUUID } from 'node:crypto';
import type { APIRoute } from 'astro';
import { publicar, estimarSalida, RENDERER_VERSION, VERSION_IR } from '@sytue/pages-render';
import type { DatosRender, Sitio } from '@sytue/pages-render';
import { ISLAS_HASH } from '@sytue/pages-render/islas';

/** Topes del servicio (§7.4 paso 5, §20.3): mismos que `procesarMultipartPublicacion` en Go. */
export const MAX_FICHEROS = 5000;
export const MAX_BYTES = 64 << 20;

const ID_SITIO = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
/** Lo único que cabe en `filename`: el validador de Go ya exige menos, esto cierra la inyección de cabeceras. */
const RUTA_SEGURA = /^[A-Za-z0-9._-]+(\/[A-Za-z0-9._-]+)*$/;

export type TipoPublicacion = 'preview' | 'live';

export interface DepsPublicar {
  /** La sesión de la persona (`getApiSession`): null = sin sesión. */
  sesion: (ctx: Parameters<APIRoute>[0]) => Promise<{ accessToken: string } | null>;
  /** Dirección interna del servicio Go. */
  destino: string;
  /** Secreto de `X-Sytue-Publicador`; se lee en cada petición (el entorno de runtime manda). */
  secreto: () => string | undefined;
  /** Para los tests. */
  fetch?: typeof fetch;
}

interface Snapshot {
  version_id: string;
  ir_version: number;
  ir: Sitio;
  data: DatosRender;
}

/** El cuerpo de error canónico de la suite: `{error:{code,message,params?}}`. */
export function respuestaError(
  status: number,
  code: string,
  message: string,
  params?: Record<string, unknown>,
): Response {
  return Response.json(
    { error: { code, message, ...(params ? { params } : {}) } },
    { status, headers: { 'cache-control': 'no-store' } },
  );
}

/** Reenvía el error del servicio con su estado; si no es JSON canónico (un proxy, un 502), lo envuelve. */
async function reenviarError(res: Response): Promise<Response> {
  const texto = await res.text().catch(() => '');
  try {
    const cuerpo = JSON.parse(texto) as { error?: { code?: unknown; message?: unknown } };
    if (typeof cuerpo.error?.code === 'string' && typeof cuerpo.error.message === 'string') {
      return new Response(texto, {
        status: res.status,
        headers: { 'content-type': 'application/json', 'cache-control': 'no-store' },
      });
    }
  } catch {
    // no era JSON: se envuelve abajo
  }
  return respuestaError(res.status, 'upstream_error', 'The Sytue Web service returned an error');
}

export const sha256 = (b: Uint8Array): string => createHash('sha256').update(b).digest('hex');

/**
 * Cuerpo `multipart/form-data` como `ReadableStream`, sin pasar por `FormData` (no copia los ficheros a
 * un segundo búfer). El manifiesto va PRIMERO: el servicio lo exige así.
 */
export function multipartEnStreaming(
  manifiesto: unknown,
  ficheros: { ruta: string; bytes: Uint8Array }[],
): { cuerpo: ReadableStream<Uint8Array>; tipo: string } {
  const frontera = `sytue-${randomUUID()}`;
  const enc = new TextEncoder();
  const partes: Uint8Array[] = [
    enc.encode(
      `--${frontera}\r\nContent-Disposition: form-data; name="manifest"\r\nContent-Type: application/json\r\n\r\n${JSON.stringify(manifiesto)}\r\n`,
    ),
  ];
  for (const f of ficheros) {
    partes.push(
      enc.encode(
        `--${frontera}\r\nContent-Disposition: form-data; name="file"; filename="${f.ruta}"\r\nContent-Type: application/octet-stream\r\n\r\n`,
      ),
      f.bytes,
      enc.encode('\r\n'),
    );
  }
  partes.push(enc.encode(`--${frontera}--\r\n`));
  let i = 0;
  return {
    tipo: `multipart/form-data; boundary=${frontera}`,
    cuerpo: new ReadableStream<Uint8Array>({
      pull(c) {
        const parte = partes[i++];
        if (parte === undefined) c.close();
        else c.enqueue(parte);
      },
    }),
  };
}

export function crearPublicar(deps: DepsPublicar): APIRoute {
  const destino = deps.destino.replace(/\/$/, '');
  const llamar = deps.fetch ?? fetch;

  return async (ctx) => {
    if (ctx.request.method !== 'POST') return respuestaError(405, 'method_not_allowed', 'Use POST');
    const origen = ctx.request.headers.get('origin');
    if (origen && origen !== ctx.url.origin)
      return respuestaError(403, 'forbidden', 'Invalid request origin');
    const sesion = await deps.sesion(ctx);
    if (!sesion) return respuestaError(401, 'unauthorized', 'Sign in to continue');

    // Sin secreto no se hace nada: ni siquiera el snapshot. El mensaje no dice qué falta ni dónde.
    const secreto = deps.secreto();
    if (!secreto) return respuestaError(500, 'publisher_unavailable', 'Publishing is unavailable');

    const siteId = ctx.params.id ?? '';
    if (!ID_SITIO.test(siteId)) return respuestaError(400, 'invalid_site_id', 'Invalid site id');

    // Forzar `application/json` obliga a un preflight a cualquier origen ajeno.
    if (
      !(ctx.request.headers.get('content-type') ?? '').toLowerCase().startsWith('application/json')
    ) {
      return respuestaError(415, 'unsupported_media_type', 'Content-Type must be application/json');
    }
    const cuerpo = (await ctx.request.json().catch(() => null)) as { kind?: unknown } | null;
    const kind = cuerpo?.kind;
    if (kind !== 'preview' && kind !== 'live') {
      return respuestaError(400, 'invalid_kind', 'kind must be preview or live');
    }

    const api = `${destino}/api/v1/sites/${siteId}`;
    const auth = { authorization: `Bearer ${sesion.accessToken}` };
    const pub = { ...auth, 'x-sytue-publicador': secreto };
    const pedir = async (url: string, init: RequestInit, ms: number): Promise<Response> => {
      try {
        return await llamar(url, { ...init, signal: AbortSignal.timeout(ms) });
      } catch {
        return respuestaError(503, 'service_unavailable', 'The Sytue Web service is unavailable');
      }
    };

    // 1. Instantánea.
    const rSnap = await pedir(
      `${api}/snapshots`,
      {
        method: 'POST',
        headers: { ...auth, 'content-type': 'application/json' },
        body: JSON.stringify({ purpose: kind === 'live' ? 'publish' : 'preview' }),
      },
      30_000,
    );
    if (!rSnap.ok) return reenviarError(rSnap);
    const snap = (await rSnap.json().catch(() => null)) as Snapshot | null;
    if (!snap?.ir || !snap.data || !ID_SITIO.test(snap.version_id)) {
      return respuestaError(502, 'bad_gateway', 'The Sytue Web service sent an invalid snapshot');
    }

    // 2. ¿El IR es el que este renderizador entiende?
    if (snap.ir_version !== VERSION_IR) {
      return respuestaError(
        409,
        'ir_version_mismatch',
        'The site was saved with another IR version',
        {
          expected: VERSION_IR,
          got: snap.ir_version,
        },
      );
    }

    // 3. Tope, sin renderizar.
    const est = estimarSalida(snap.ir, snap.data);
    if (est.archivos > MAX_FICHEROS || est.bytes > MAX_BYTES) {
      return respuestaError(413, 'publication_too_large', 'The site is too large to publish', {
        files: est.archivos,
        bytes: est.bytes,
      });
    }

    // 4. Vista previa: el token y la base los fija el servicio.
    let base: '' | `/_vista/${string}` = '';
    let token = '';
    let expiraEn: string | undefined;
    if (kind === 'preview') {
      const rPrev = await pedir(`${api}/previews`, { method: 'POST', headers: pub }, 30_000);
      if (!rPrev.ok) return reenviarError(rPrev);
      const prev = (await rPrev.json().catch(() => null)) as {
        token?: string;
        base_path?: string;
        expires_at?: string;
      } | null;
      if (
        !prev?.token ||
        !/^[0-9a-f]{32}$/.test(prev.token) ||
        prev.base_path !== `/_vista/${prev.token}`
      ) {
        return respuestaError(502, 'bad_gateway', 'The Sytue Web service sent an invalid preview');
      }
      token = prev.token;
      base = prev.base_path as `/_vista/${string}`;
      expiraEn = prev.expires_at;
    }

    // 5. Renderizar.
    const salida = publicar(snap.ir, snap.data, { base, vista: kind === 'preview', hash: sha256 });
    const enc = new TextEncoder();
    const ficheros = salida.archivos.map((a) => ({
      ruta: a.ruta,
      bytes: typeof a.contenido === 'string' ? enc.encode(a.contenido) : a.contenido,
    }));
    const bytes = ficheros.reduce((n, f) => n + f.bytes.length, 0);
    if (ficheros.length > MAX_FICHEROS || bytes > MAX_BYTES) {
      return respuestaError(413, 'publication_too_large', 'The site is too large to publish', {
        files: ficheros.length,
        bytes,
      });
    }
    if (ficheros.some((f) => !RUTA_SEGURA.test(f.ruta))) {
      return respuestaError(500, 'render_invalid_output', 'The renderer produced an invalid path');
    }

    // 6. Subir en streaming.
    const { cuerpo: stream, tipo } = multipartEnStreaming(
      {
        version_id: snap.version_id,
        kind,
        ...(token ? { preview_token: token } : {}),
        renderer_version: RENDERER_VERSION,
        islas_hash: ISLAS_HASH,
        files: ficheros.map((f) => ({
          path: f.ruta,
          sha256: sha256(f.bytes),
          bytes: f.bytes.length,
        })),
        resources: salida.recursos,
      },
      ficheros,
    );
    const query =
      kind === 'preview' ? `?kind=preview&token=${encodeURIComponent(token)}` : '?kind=live';
    const rPub = await pedir(
      `${api}/publications${query}`,
      {
        method: 'POST',
        headers: { ...pub, 'content-type': tipo },
        body: stream,
        // @ts-expect-error duplex es obligatorio al enviar un cuerpo en streaming
        duplex: 'half',
      },
      120_000,
    );
    if (!rPub.ok) return reenviarError(rPub);
    const res = (await rPub.json().catch(() => null)) as { number?: number; url?: string } | null;
    if (kind === 'live') {
      return Response.json(
        { kind, number: res?.number, url: `https://${snap.data.site.canonical_host}/` },
        { headers: { 'cache-control': 'no-store' } },
      );
    }
    return Response.json(
      { kind, url: res?.url, expires_at: expiraEn },
      { headers: { 'cache-control': 'no-store' } },
    );
  };
}
