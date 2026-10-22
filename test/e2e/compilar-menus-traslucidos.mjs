// SPDX-License-Identifier: Apache-2.0
// Solo CI: compila el banco con componentes reales y sin servidor de Astro/Vite.
// El único contrato de Vite que necesita es SVG?url. Se conserva el SVG completo
// como data URI con loader JS, sin depender del soporte de `dataurl` de Bun.
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const resultado = await Bun.build({
  entrypoints: [
    fileURLToPath(
      new URL('../../apps/frontend/packages/ui/test/menus-traslucidos.tsx', import.meta.url),
    ),
  ],
  target: 'browser',
  format: 'esm',
  // El banco no pasa por Astro: también sus paquetes compartidos usan JSX de Preact.
  jsx: { runtime: 'automatic', importSource: 'preact' },
  plugins: [
    {
      name: 'svg-url-del-banco-de-menus',
      setup(build) {
        build.onResolve({ filter: /\.svg\?url$/ }, ({ path, resolveDir }) => ({
          path: resolve(resolveDir, path.slice(0, -4)),
          namespace: 'svg-url',
        }));
        build.onLoad({ filter: /\.svg$/, namespace: 'svg-url' }, async ({ path }) => {
          const svg = Buffer.from(await Bun.file(path).arrayBuffer()).toString('base64');
          return {
            contents: `export default ${JSON.stringify(`data:image/svg+xml;base64,${svg}`)};`,
            loader: 'js',
          };
        });
      },
    },
  ],
});
if (!resultado.success) throw new AggregateError(resultado.logs, 'no se compiló el banco de menús');
if (resultado.outputs.length !== 1) {
  throw new Error(
    `el banco debe ser un único módulo autocontenido; produjo ${resultado.outputs.length} ficheros`,
  );
}
await Bun.write(process.env.MENUS_JS || '/tmp/menus-traslucidos.js', resultado.outputs[0]);
