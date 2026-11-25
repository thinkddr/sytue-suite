/* SPDX-License-Identifier: Apache-2.0 */
import { HttpClient, upstreamStatus } from './http.js';
import {
  isClientCoreModule,
  clientCoreModuleBounds,
  vaultCoreModule,
} from '@sytue/config/client-core';
export function roomsServiceBase(env: {
  SYTUE_ROOMS_INTERNAL_URL?: string;
  SYTUE_ROOMS_API_URL?: string;
  SYTUE_CHAT_API_URL?: string;
}): string {
  return (
    env.SYTUE_ROOMS_INTERNAL_URL ??
    env.SYTUE_ROOMS_API_URL ??
    env.SYTUE_CHAT_API_URL ??
    'https://rooms.sytue.eu'
  );
}
interface CoreRequest {
  params: { asset?: string | undefined };
  url: URL;
  request: Request;
}
/**
 * Bun puede pedir la respuesta SIN descomprimir (`decompress: false`). Rooms sirve
 * `core.wasm` y `wasm_exec.js` ya comprimidos en brotli 11 (docs/decisions.md#0086), y
 * así llegan tal cual al navegador, que los descomprime y comprueba la SHA-256 de los
 * bytes originales. En otro runtime `fetch` descomprimiría y dejaría la cabecera
 * `content-encoding`: ahí se piden sin comprimir y Traefik comprime como antes.
 */
const passesEncodedBodies = typeof (globalThis as { Bun?: unknown }).Bun !== 'undefined';
// `decompress` es de Bun y no está en el RequestInit de la DOM.
const encodedFetch = ((input: RequestInfo | URL, init?: RequestInit) =>
  fetch(input, Object.assign({}, init, { decompress: false }))) as typeof fetch;
const precompressed = new Set(['core.wasm', 'wasm_exec.js']);
const passedEncodings = new Set(['br', 'gzip']);

/** Same read-only compiler artifacts for every app; never reads an app session. */
export function clientCoreProxy(baseUrl: string): (context: CoreRequest) => Promise<Response> {
  const base = baseUrl.replace(/\/$/, '') + '/api/v1';
  const http = new HttpClient({ baseUrl: base });
  const encodedHttp = new HttpClient({ baseUrl: base, fetchImpl: encodedFetch });
  return async (ctx: CoreRequest): Promise<Response> => {
    const asset = ctx.params.asset ?? '',
      hash = ctx.url.searchParams.get('sha256'),
      module = ctx.url.searchParams.get('module') ?? 'core';
    if (
      !['manifest.json', 'core.wasm', 'wasm_exec.js', 'GO-LICENSE'].includes(asset) ||
      !isClientCoreModule(module) ||
      (hash !== null && !/^[a-f0-9]{64}$/.test(hash))
    )
      return new Response(null, { status: 400 });
    for (const key of ctx.url.searchParams.keys())
      if ((key !== 'sha256' && key !== 'module') || ctx.url.searchParams.getAll(key).length !== 1)
        return new Response(null, { status: 400 });
    // Solo la preferencia del navegador, acotada; nunca otra cabecera suya.
    const passthrough = passesEncodedBodies && precompressed.has(asset);
    const accepted = passthrough
      ? (ctx.request.headers.get('accept-encoding') ?? '').slice(0, 256)
      : 'identity';
    try {
      const upstream = await (passthrough ? encodedHttp : http).request({
        path: '/client-core/' + asset,
        query: { ...(hash ? { sha256: hash } : {}), ...(module === 'core' ? {} : { module }) },
        headers: { 'accept-encoding': accepted || 'identity' },
        signal: ctx.request.signal,
        maxResponseBytes: clientCoreModuleBounds[module],
      });
      const encoding = upstream.headers.get('content-encoding');
      if (encoding && (!passthrough || !passedEncodings.has(encoding))) {
        await upstream.body?.cancel().catch(() => undefined);
        return new Response(null, { status: 502, headers: { 'cache-control': 'no-store' } });
      }
      const headers = new Headers({
        'cache-control':
          asset === 'manifest.json'
            ? 'no-store'
            : hash
              ? 'public, max-age=31536000, immutable'
              : 'no-cache',
        'x-content-type-options': 'nosniff',
      });
      for (const key of ['content-type', 'etag', 'content-encoding', 'vary']) {
        const value = upstream.headers.get(key);
        if (value) headers.set(key, value);
      }
      return new Response(upstream.body, { status: upstream.status, headers });
    } catch (error) {
      return new Response(null, {
        status: upstreamStatus(error),
        headers: { 'cache-control': 'no-store' },
      });
    }
  };
}

interface CoreDigests {
  wasm_sha256: string;
  runtime_sha256: string;
}
const hex64 = /^[0-9a-f]{64}$/;
const manifestCache = new Map<
  string,
  { value: CoreDigests | null; at: number; pending?: Promise<CoreDigests | null> }
>();
/** El manifiesto solo cambia al desplegar Rooms: un minuto de caché basta. */
const manifestFresh = 60_000;

async function fetchCoreDigests(baseUrl: string, timeoutMs: number): Promise<CoreDigests | null> {
  const http = new HttpClient({ baseUrl: baseUrl.replace(/\/$/, '') + '/api/v1' });
  try {
    const upstream = await http.request({
      path: '/client-core/manifest.json',
      // El de la bóveda: es el que arranca PqVaultGate (`prewarmClientCore`). Un Rooms
      // que aún no lo conozca contesta 400 y la página sale sin resúmenes.
      query: { module: vaultCoreModule },
      signal: AbortSignal.timeout(timeoutMs),
      maxResponseBytes: 4096,
    });
    if (!upstream.ok) return null;
    const value = (await upstream.json()) as Record<string, unknown>;
    if (
      value.version !== 1 ||
      value.module_id !== vaultCoreModule ||
      typeof value.wasm_sha256 !== 'string' ||
      !hex64.test(value.wasm_sha256) ||
      typeof value.runtime_sha256 !== 'string' ||
      !hex64.test(value.runtime_sha256)
    )
      return null;
    return { wasm_sha256: value.wasm_sha256, runtime_sha256: value.runtime_sha256 };
  } catch {
    return null;
  }
}

/** Los resúmenes del núcleo, de caché; pasado un minuto se refrescan sin esperar. */
function coreDigests(baseUrl: string, timeoutMs: number): Promise<CoreDigests | null> {
  const cached = manifestCache.get(baseUrl);
  if (cached?.pending && !cached.value) return cached.pending;
  if (cached && (Date.now() - cached.at <= manifestFresh || cached.pending))
    return Promise.resolve(cached.value);
  const pending = fetchCoreDigests(baseUrl, timeoutMs).then((value) => {
    // Un fallo no borra lo que había: un resumen viejo solo cuesta un 412 y el
    // manifiesto en el navegador.
    const kept = value ?? manifestCache.get(baseUrl)?.value ?? null;
    manifestCache.set(baseUrl, { value: kept, at: Date.now() });
    return kept;
  });
  manifestCache.set(baseUrl, { value: cached?.value ?? null, at: cached?.at ?? 0, pending });
  // Con algo en caché no se espera al refresco.
  return cached?.value ? Promise.resolve(cached.value) : pending;
}

function escapeAttribute(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/"/g, '&quot;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
}

/**
 * El `<head>` con el que el núcleo de la bóveda empieza a bajar en el instante cero
 * (ADR 0086): los resúmenes del manifiesto en la página (`<meta
 * name="sytue-client-core">`, que el worker usa para no pedir el manifiesto) y la
 * precarga de `wasm_exec.js` y `core.wasm?module=vault&sha256=…` con las MISMAS URL que
 * pedirá el worker, para que las sirva la caché. Solo para las pantallas que esperan a la
 * bóveda (Mail, Drive): en las demás el núcleo competiría con la primera carga.
 *
 * `basePath` es el prefijo real de la superficie (`SYTUE_BASE_PATH ?? BASE_URL`). Si
 * Rooms no contesta en `timeoutMs`, no se pinta nada y el navegador pide el
 * manifiesto como antes. Los resúmenes no dan ninguna confianza nueva: el servidor
 * solo sirve un sha256 que sea el suyo (412 si no) y el worker comprueba el del wasm.
 */
export async function clientCoreHead(
  baseUrl: string,
  basePath: string,
  timeoutMs = 300,
): Promise<string> {
  const digests = await coreDigests(baseUrl, timeoutMs);
  if (!digests) return '';
  let end = basePath.length;
  while (end > 0 && basePath.charCodeAt(end - 1) === 47) end--;
  const prefix = basePath.slice(0, end);
  // En el MISMO orden que las construye el worker (`artifact`): module y luego sha256.
  const asset = (name: string, digest: string): string =>
    escapeAttribute(`${prefix}/api/client-core/${name}?module=${vaultCoreModule}&sha256=${digest}`);
  return (
    `<meta name="sytue-client-core" content="${digests.wasm_sha256} ${digests.runtime_sha256}">` +
    `<link rel="modulepreload" href="${asset('wasm_exec.js', digests.runtime_sha256)}">` +
    `<link rel="preload" as="fetch" crossorigin="anonymous" type="application/wasm" href="${asset('core.wasm', digests.wasm_sha256)}">`
  );
}
