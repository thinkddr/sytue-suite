/* SPDX-License-Identifier: Apache-2.0 */
import { HttpClient, upstreamStatus } from './http.js';
import { isClientCoreModule, clientCoreModuleBounds } from '@sytue/config/client-core';
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
/** Same read-only compiler artifacts for every app; never reads an app session. */
export function clientCoreProxy(baseUrl: string): (context: CoreRequest) => Promise<Response> {
  const http = new HttpClient({ baseUrl: baseUrl.replace(/\/$/, '') + '/api/v1' });
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
    try {
      const upstream = await http.request({
        path: '/client-core/' + asset,
        query: { ...(hash ? { sha256: hash } : {}), ...(module === 'core' ? {} : { module }) },
        signal: ctx.request.signal,
        maxResponseBytes: clientCoreModuleBounds[module],
      });
      const headers = new Headers({
        'cache-control':
          asset === 'manifest.json'
            ? 'no-store'
            : hash
              ? 'public, max-age=31536000, immutable'
              : 'no-cache',
        'x-content-type-options': 'nosniff',
      });
      for (const key of ['content-type', 'etag']) {
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
