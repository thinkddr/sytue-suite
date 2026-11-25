// SPDX-License-Identifier: Apache-2.0
// Shared opt-in, loopback-only browser integration fixture. No product sessions.
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
const repository = fileURLToPath(new URL('../../', import.meta.url));
const sdkRequire = createRequire(
  new URL('../../apps/frontend/packages/offline/package.json', import.meta.url),
);
const coreDirectory = path.join(repository, 'services/rooms/internal/http/client_core_assets');
const digest = (bytes) => new Bun.CryptoHasher('sha256').update(bytes).digest('hex');
const compilers = new Map();
async function compilerAssets(module = 'core') {
  if (!['core', 'vault', 'office', 'mail', 'pdf'].includes(module)) return null;
  let compiler = compilers.get(module);
  if (!compiler) {
    compiler = (async () => {
      const directory = module === 'core' ? coreDirectory : path.join(coreDirectory, module);
      try {
        const wasm = await readFile(path.join(directory, 'core.wasm'));
        const runtime = await readFile(path.join(directory, 'wasm_exec.js'));
        return {
          wasm,
          runtime,
          manifest: {
            version: 1,
            module_id: module,
            wasm_sha256: digest(wasm),
            runtime_sha256: digest(runtime),
          },
        };
      } catch {
        return null;
      }
    })();
    compilers.set(module, compiler);
  }
  return compiler;
}
export async function bundleMatrixFixture(entry, directory) {
  const worker = path.join(repository, 'apps/frontend/packages/offline/src/pqc/worker.ts');
  const result = await Bun.build({
    entrypoints: [entry, worker],
    outdir: directory,
    target: 'browser',
    splitting: true,
    naming: '[name].[ext]',
    define: { 'import.meta.env.BASE_URL': JSON.stringify('/') },
    plugins: [
      {
        name: 'suite-sdk',
        setup(build) {
          build.onResolve({ filter: /^matrix-js-sdk(?:\/|$)/ }, (args) => ({
            path: sdkRequire.resolve(args.path),
          }));
          build.onResolve({ filter: /^@sytue\/ui$/ }, () => ({
            path: path.join(repository, 'apps/frontend/packages/config/src/base-path.ts'),
          }));
        },
      },
    ],
  });
  if (!result.success) throw new Error(result.logs.map(String).join('\n'));
}
/** Only public compiler output and bundled test code is served here. */
export async function matrixFixtureAsset(url, directory) {
  if (url.pathname.includes('/api/client-core/')) {
    const asset = url.pathname.slice(url.pathname.lastIndexOf('/') + 1),
      files = await compilerAssets(url.searchParams.get('module') ?? 'core');
    if (!files) return new Response(null, { status: 503 });
    if (asset === 'manifest.json')
      return Response.json(files.manifest, {
        headers: { 'cache-control': 'no-store' },
      });
    const bytes =
      asset === 'core.wasm' ? files.wasm : asset === 'wasm_exec.js' ? files.runtime : null;
    if (!bytes) return new Response(null, { status: 404 });
    const wanted = url.searchParams.get('sha256');
    if (wanted && wanted !== digest(bytes)) return new Response(null, { status: 412 });
    return new Response(bytes, {
      headers: {
        'content-type': asset === 'core.wasm' ? 'application/wasm' : 'text/javascript',
        'cache-control': 'no-store',
      },
    });
  }
  if (/matrix_sdk_crypto_wasm_bg(?:\.[A-Za-z0-9_-]+)?\.wasm$/.test(url.pathname))
    return new Response(
      Bun.file(
        path.join(
          path.dirname(sdkRequire.resolve('@matrix-org/matrix-sdk-crypto-wasm')),
          'pkg/matrix_sdk_crypto_wasm_bg.wasm',
        ),
      ),
      { headers: { 'content-type': 'application/wasm' } },
    );
  const pathname = url.pathname.endsWith('/worker.ts') ? 'worker.js' : url.pathname.slice(1);
  const filename = path.resolve(directory, pathname);
  if (!filename.startsWith(path.resolve(directory) + path.sep))
    return new Response(null, { status: 404 });
  const file = Bun.file(filename);
  if (!(await file.exists())) return new Response(null, { status: 404 });
  return new Response(file, { headers: { 'content-type': 'text/javascript' } });
}
export function matrixBrowserOptions() {
  const executable = process.env.SYTUE_MATRIX_BROWSER_EXECUTABLE;
  return {
    headless: true,
    ...(executable ? { executablePath: executable } : {}),
    args: [
      '--no-sandbox',
      '--disable-dev-shm-usage',
      '--host-resolver-rules=MAP storage.test 127.0.0.1',
      '--no-proxy-server',
    ],
  };
}
export function requireArtificialMatrixFixture(fixture) {
  assert.equal(fixture.artificial, true);
}
