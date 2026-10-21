/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Ningún redirect del servidor puede apuntar a una ruta interna SIN el prefijo
 * de su app.
 *
 * `Astro.redirect(path)` escribe el `Location` TAL CUAL —comprobado en el propio
 * paquete: `core/middleware/index.js` devuelve `headers: { Location: path }`—,
 * así que el `base` del build no se aplica. Y Traefik solo rescata la RAÍZ: su
 * middleware `raiz-al-prefijo` usa el regex `^https?://([^/]+)/?(\?.*)?$`, que no
 * casa en cuanto hay ruta. Resultado: `/edit?id=…` desde Docs o `/bienvenida`
 * desde el portal acaban en el 404 de Astro.
 *
 * Y ese 404 no parece un enlace mal escrito: el host resuelve y el Ingress
 * enruta, así que se lee como «la aplicación está caída». Pasó a la vez en el
 * flujo de abrir un documento desde Drive (sus tres salidas) y en el alta sin
 * completar.
 *
 * Se excluye `/auth/**` y `/api/**`: los sirve el BFF y el Ingress les da regla
 * propia, fuera del prefijo. Y la raíz pelada (`/`), que sí rescata Traefik.
 *
 * Un caso legítimo se marca EN EL CÓDIGO con `prefijo-ok:` y su motivo, no en
 * una lista aquí: quien escriba el redirect es quien sabe por qué no lleva
 * prefijo, y una lista en este fichero envejece sin que nadie la mire.
 */
import { describe, expect, test } from "bun:test";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";

const FRONTEND = join(import.meta.dir, "..", "..", "apps", "frontend");
// Las apps, y la ofimática común: sus endpoints y sus vistas se sirven bajo el
// prefijo de Docus, Calcus, Diapos y CertiPDF, que es lo mismo que estar en una app.
const RAICES = [join(FRONTEND, "apps"), join(FRONTEND, "packages", "office", "src")];

/** Rutas que NO llevan prefijo por diseño. */
const SIN_PREFIJO = /^\/(auth|api)\//;

function ficheros(dir: string, out: string[] = []): string[] {
  for (const e of readdirSync(dir)) {
    if (e === "node_modules" || e === "dist" || e === ".astro") continue;
    const p = join(dir, e);
    if (statSync(p).isDirectory()) ficheros(p, out);
    else if (/\.(astro|ts|tsx)$/.test(e)) out.push(p);
  }
  return out;
}

describe("redirects del servidor", () => {
  test("ninguno apunta a una ruta interna sin el prefijo de su app", () => {
    const malos: string[] = [];
    for (const f of RAICES.flatMap((r) => ficheros(r))) {
      const texto = readFileSync(f, "utf8");
      // `redirect('/algo…')` o `redirect(`/algo…`)`, con la ruta literal pegada.
      for (const m of texto.matchAll(/\.redirect\(\s*['`](\/[^'`]*)['`]/g)) {
        const ruta = m[1] ?? "";
        if (ruta === "/" || SIN_PREFIJO.test(ruta)) continue;
        // La justificación va en la propia línea o en la de encima.
        const antes = texto.slice(0, m.index ?? 0);
        const linea = antes.split("\n").length;
        const contexto = texto
          .split("\n")
          .slice(Math.max(0, linea - 3), linea)
          .join("\n");
        if (contexto.includes("prefijo-ok:")) continue;
        malos.push(`${f.slice(FRONTEND.length + 1)} → ${ruta}`);
      }
    }
    expect(malos).toEqual([]);
  });
});
