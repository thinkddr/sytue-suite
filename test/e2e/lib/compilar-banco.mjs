// SPDX-License-Identifier: Apache-2.0
// Compila un banco de componentes reales en UN módulo autocontenido, sin servidor de
// Astro/Vite: el de menús traslúcidos y el del mes del teléfono de Calendar. Los únicos
// contratos de Vite que hacen falta son SVG?url —el SVG completo como data URI con loader
// JS, sin depender del soporte de `dataurl` de Bun— y SVG?raw, su texto (el SVG en línea
// de OrgMark e Illustration, que entra en cuanto el banco importa `@sytue/ui` entero).
import { resolve } from "node:path";

/** Compila `entrada` (ruta absoluta del .tsx) y escribe el módulo en `salida`. */
export async function compilarBanco(entrada, salida) {
  const resultado = await Bun.build({
    entrypoints: [entrada],
    target: "browser",
    format: "esm",
    // El banco no pasa por Astro: también sus paquetes compartidos usan JSX de Preact.
    jsx: { runtime: "automatic", importSource: "preact" },
    // Lo que Vite sustituye y aquí no hay: sin `SSR` los catálogos de i18n toman la rama
    // de los veintiséis (la que usan los tests en Bun), y la app vive en la raíz.
    define: {
      "import.meta.env.SSR": "undefined",
      "import.meta.env.BASE_URL": '"/"',
    },
    plugins: [
      {
        name: "svg-del-banco",
        setup(build) {
          // El tipo va EN la ruta (`x.svg.url` / `x.svg.raw`) y no solo en el espacio de
          // nombres: con los dos `onLoad` filtrando `\.svg$`, Bun daba al `?raw` el
          // cargador del `?url` y OrgMark pintaba el data URI como TEXTO (un cuadrado
          // vacío en la barra del banco, que en la app real no existe).
          build.onResolve(
            { filter: /\.svg\?(url|raw)$/ },
            ({ path, resolveDir }) => ({
              path: `${resolve(resolveDir, path.slice(0, -4))}.${path.slice(-3)}`,
              namespace: "svg-banco",
            }),
          );
          build.onLoad(
            { filter: /\.svg\.url$/, namespace: "svg-banco" },
            async ({ path }) => {
              const svg = Buffer.from(
                await Bun.file(path.slice(0, -4)).arrayBuffer(),
              ).toString("base64");
              return {
                contents: `export default ${JSON.stringify(`data:image/svg+xml;base64,${svg}`)};`,
                loader: "js",
              };
            },
          );
          build.onLoad(
            { filter: /\.svg\.raw$/, namespace: "svg-banco" },
            async ({ path }) => ({
              contents: `export default ${JSON.stringify(await Bun.file(path.slice(0, -4)).text())};`,
              loader: "js",
            }),
          );
        },
      },
    ],
  });
  if (!resultado.success)
    throw new AggregateError(resultado.logs, `no se compiló ${entrada}`);
  if (resultado.outputs.length !== 1) {
    throw new Error(
      `el banco debe ser un único módulo autocontenido; produjo ${resultado.outputs.length} ficheros`,
    );
  }
  await Bun.write(salida, resultado.outputs[0]);
}
