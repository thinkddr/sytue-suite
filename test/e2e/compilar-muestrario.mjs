// SPDX-License-Identifier: Apache-2.0
// Compila el muestrario de @sytue/ui en un directorio que se abre en el navegador sin
// servidor: `index.html` + el módulo de la página + la hoja de Tailwind.
//
//   bun run test/e2e/compilar-muestrario.mjs [destino]   (por defecto /tmp/muestrario)
//   → abrir <destino>/index.html?modo=dark&color=cobalto&forma=carril
//
// No es un gate: el que monta todo en las 16 combinaciones del tema y vigila que no
// falte ningún componente es `packages/ui/test/muestrario.test.tsx`. Esto es para
// MIRARLO, y para ponerlo al lado del de Fyne (`SYTUE_MUESTRARIO=1` en `cmd/demo`).
import { execFileSync } from "node:child_process";
import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { compilarBanco } from "./lib/compilar-banco.mjs";

const ui = fileURLToPath(
  new URL("../../apps/frontend/packages/ui/", import.meta.url),
);
const tailwind = fileURLToPath(
  new URL(
    "../../apps/frontend/node_modules/.bin/tailwindcss",
    import.meta.url,
  ),
);
const destino = process.argv[2] || "/tmp/muestrario";
mkdirSync(destino, { recursive: true });

await compilarBanco(
  join(ui, "test/muestrario-pagina.tsx"),
  join(destino, "muestrario.js"),
);

// La hoja de la suite más el propio muestrario como fuente de clases: sus marcos usan
// alguna que ninguna app usa todavía y Tailwind solo genera lo que encuentra.
const entrada = join(destino, "entrada.css");
writeFileSync(
  entrada,
  `@import '${join(ui, "src/styles.css")}';\n@source '${join(ui, "test")}/muestrario*.tsx';\n`,
);
execFileSync(tailwind, ["-i", entrada, "-o", join(destino, "muestrario.css")], {
  cwd: ui,
  stdio: "inherit",
});

writeFileSync(
  join(destino, "index.html"),
  `<!doctype html>
<html lang="es">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Muestrario de @sytue/ui</title>
    <link rel="stylesheet" href="muestrario.css" />
  </head>
  <body class="bg-bg">
    <script type="module" src="muestrario.js"></script>
  </body>
</html>
`,
);
console.log(`✓ muestrario en ${join(destino, "index.html")}`);
