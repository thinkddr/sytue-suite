// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// Prueba visual y de teclado de la CINTA del editor de Docs: las tres variantes
// (documento, hoja, presentación) a 1280, 390 y 320 px.
//
// Mide el banco de pruebas `/dev/cinta` (editor/dev/CintaDemo.tsx), la cinta sola con un
// contexto de mentira, servida por `astro dev` de apps/docus. NO va contra producción: el
// editor de verdad pide sesión y un servicio, y aquí se mide maquetación y teclado.
//
// La URL lleva una organización cualquiera (`/docus/prueba`): el middleware de Docus toma el
// primer segmento tras `/docs` por la organización y reescribe el resto, así que sin ella
// `/docus/dev/cinta` se leería como la org «dev» y daría 404.
//
// Uso (desde el contenedor del frontal; el servidor dev, antes, en otro proceso):
//   cd apps/frontend/apps/docus && bun --bun astro dev --port 4338 &
//   cd test/e2e && CINTA_URL=http://127.0.0.1:4338/docus/prueba bun run cinta.mjs
// Salida: capturas en CINTA_OUT (por defecto /tmp/cinta) y un informe en consola; el
// código de salida es 1 si algo falla.
//
// Qué comprueba, por variante y ancho, en TODAS las pestañas:
//   1. el documento no desborda en horizontal;
//   2. la banda de grupos es una línea desplazable y, si no cabe, aparece «Más»;
//   3. con «Más» desplegado todos los grupos caben en el ancho de la pantalla y Escape
//      lo pliega devolviendo el foco al botón;
//   4. un popover abierto es `fixed`, queda dentro de la pantalla y NADIE lo recorta;
//   5. la banda es UNA parada de tabulación (roving) y las flechas mueven el foco;
//   6. F6 salta de la cinta al lienzo y del lienzo a la pestaña activa.
//
// ponytail: script de verificación, no forma parte de la suite de specs.
import { chromium } from "playwright";
import { mkdirSync } from "node:fs";

const BASE = process.env.CINTA_URL || "http://127.0.0.1:4338/docus/prueba";
const OUT = process.env.CINTA_OUT || "/tmp/cinta";
const VARIANTES = ["document", "spreadsheet", "presentation"];
const ANCHOS = [
  { w: 1280, h: 800, movil: false },
  { w: 390, h: 844, movil: true },
  { w: 320, h: 640, movil: true },
];
// El idioma más ancho que tenemos (el alemán medía 1304 px en Datos a 1280) y el de base.
const IDIOMAS = ["es", "de"];

mkdirSync(OUT, { recursive: true });
const fallos = [];
const falla = (ctx, msg) => {
  fallos.push(`${ctx}: ${msg}`);
  console.log(`  ✗ ${msg}`);
};
const ok = (msg) => console.log(`  ✓ ${msg}`);

const navegador = await chromium.launch();

// Calentamiento: la primera carga de `astro dev` compila y optimiza las dependencias (en un
// contenedor cargado pasa de 30 s). Sin él, la primera variante falla por tiempo y no por
// la cinta. Las demás cargas ya van con todo compilado.
{
  const p = await navegador.newPage();
  await p.goto(`${BASE}/dev/cinta?kind=document&lang=es`);
  await p.waitForSelector('[role="tablist"]', { timeout: 180000 });
  await p.close();
}

for (const kind of VARIANTES) {
  for (const lang of IDIOMAS) {
    for (const { w, h, movil } of ANCHOS) {
      const id = `${kind}-${lang}-${w}`;
      console.log(`\n${id}`);
      const ctxB = await navegador.newContext({
        viewport: { width: w, height: h },
        hasTouch: movil,
        isMobile: movil,
      });
      const page = await ctxB.newPage();
      const errores = [];
      page.on("pageerror", (e) => errores.push(e.message));
      await page.goto(`${BASE}/dev/cinta?kind=${kind}&lang=${lang}`);
      await page.waitForSelector('[role="tablist"]', { timeout: 30000 });

      const abrirHerramientas = async () => {
        // En <768 px la fila empieza plegada: el botón de herramientas la abre.
        const b = page.locator('button[aria-controls="ribbon-tabpanel"][aria-expanded]');
        if ((await b.count()) && (await b.first().isVisible())) {
          if ((await b.first().getAttribute("aria-expanded")) === "false") await b.first().click();
        }
      };

      const tabs = page.locator('[role="tab"]');
      const nTabs = await tabs.count();
      for (let i = 0; i < nTabs; i++) {
        const nombre = (await tabs.nth(i).innerText()).trim();
        const c = `${id}/${nombre}`;
        await tabs.nth(i).click();
        await abrirHerramientas();
        await page.waitForTimeout(150);

        // 1. El documento no desborda.
        const doc = await page.evaluate(() => ({
          sw: document.documentElement.scrollWidth,
          iw: window.innerWidth,
        }));
        if (doc.sw > doc.iw) falla(c, `el documento desborda: ${doc.sw} > ${doc.iw}`);

        // 2. Banda de una línea, desplazable; «Más» si no cabe.
        const banda = await page.evaluate(() => {
          const b = document.getElementById("ribbon-banda");
          if (!b) return null;
          const cs = getComputedStyle(b);
          const grupos = [...b.querySelectorAll(':scope > [role="group"]')];
          const tops = new Set(grupos.map((g) => Math.round(g.getBoundingClientRect().top)));
          const mas = document.querySelector('button[aria-controls="ribbon-banda"]');
          return {
            desborda: b.scrollWidth > b.clientWidth + 1,
            overflowX: cs.overflowX,
            lineas: tops.size,
            grupos: grupos.length,
            mas: !!mas && getComputedStyle(mas).display !== "none",
          };
        });
        if (!banda) {
          falla(c, "no hay banda #ribbon-banda");
          continue;
        }
        if (banda.lineas > 1) falla(c, `la banda plegada ocupa ${banda.lineas} líneas`);
        if (banda.desborda && !banda.mas) falla(c, "la banda no cabe y no hay botón «Más»");
        if (!banda.desborda && banda.mas) falla(c, "hay «Más» y la banda cabe");
        if (banda.desborda && banda.overflowX !== "auto") {
          falla(c, `la banda no cabe y no se desplaza (overflow-x: ${banda.overflowX})`);
        }

        // 3. «Más» desplegado: todos los grupos en pantalla; Escape pliega y devuelve el foco.
        if (banda.mas) {
          const mas = page.locator('button[aria-controls="ribbon-banda"]');
          await mas.click();
          await page.waitForTimeout(100);
          const abierta = await page.evaluate(() => {
            const b = document.getElementById("ribbon-banda");
            const iw = window.innerWidth;
            const fuera = [...b.querySelectorAll(':scope > [role="group"]')].filter((g) => {
              const r = g.getBoundingClientRect();
              return r.left < -1 || r.right > iw + 1;
            }).length;
            return {
              fixed: getComputedStyle(b).position === "fixed",
              fuera,
              desbordaX: b.scrollWidth > b.clientWidth + 1,
              bottom: b.getBoundingClientRect().bottom,
              ih: window.innerHeight,
            };
          });
          if (!abierta.fixed) falla(c, "«Más» no despliega la banda");
          if (abierta.fuera > 0) falla(c, `con «Más» quedan ${abierta.fuera} grupos fuera de la pantalla`);
          if (abierta.desbordaX) falla(c, "con «Más» la banda desborda en horizontal");
          if (abierta.bottom > abierta.ih + 1) falla(c, "con «Más» la banda se sale por abajo");
          if (i === 0) await page.screenshot({ path: `${OUT}/${id}-mas.png` });
          await page.keyboard.press("Escape");
          await page.waitForTimeout(100);
          const cierre = await page.evaluate(() => ({
            fixed: getComputedStyle(document.getElementById("ribbon-banda")).position === "fixed",
            foco: document.activeElement?.getAttribute("aria-controls"),
          }));
          if (cierre.fixed) falla(c, "Escape no pliega la banda");
          if (cierre.foco !== "ribbon-banda") falla(c, "Escape no devuelve el foco a «Más»");
        }

        // 4. Un popover abierto: fixed, dentro de la pantalla y a la vista.
        const disparador = page
          .locator('#ribbon-banda button[aria-haspopup="true"]:not([disabled])')
          .first();
        if (await disparador.count()) {
          await disparador.scrollIntoViewIfNeeded();
          await disparador.click();
          await page.waitForTimeout(150);
          const pop = await page.evaluate(() => {
            const p = document.querySelector(".ribbon-popover");
            if (!p) return null;
            const r = p.getBoundingClientRect();
            const el = document.elementFromPoint(
              Math.min(Math.max(r.left + r.width / 2, 0), window.innerWidth - 1),
              Math.min(Math.max(r.top + 8, 0), window.innerHeight - 1),
            );
            return {
              pos: getComputedStyle(p).position,
              l: r.left,
              r: r.right,
              t: r.top,
              b: r.bottom,
              iw: window.innerWidth,
              ih: window.innerHeight,
              visible: !!el && (p === el || p.contains(el)),
            };
          });
          if (pop) {
            if (pop.pos !== "fixed") falla(c, `el popover no es fixed (${pop.pos})`);
            if (pop.l < -1 || pop.r > pop.iw + 1) falla(c, `el popover se sale en horizontal (${pop.l.toFixed(0)}..${pop.r.toFixed(0)} de ${pop.iw})`);
            if (pop.b > pop.ih + 1) falla(c, `el popover se sale por abajo (${pop.b.toFixed(0)} > ${pop.ih})`);
            if (!pop.visible) falla(c, "el popover está tapado o recortado");
            if (i === 0) await page.screenshot({ path: `${OUT}/${id}-popover.png` });
          }
          await page.keyboard.press("Escape");
          await page.waitForTimeout(80);
        }

        // 5. La banda es UNA parada de tabulación y las flechas mueven el foco.
        const paradas = await page.evaluate(() => {
          const b = document.getElementById("ribbon-banda");
          const sel = 'a[href],button:not([disabled]),input:not([disabled]),select:not([disabled])';
          return [...b.querySelectorAll(sel)].filter(
            (e) => !e.closest(".ribbon-popover") && e.tabIndex >= 0,
          ).length;
        });
        if (paradas !== 1) falla(c, `la banda tiene ${paradas} paradas de tabulación (debe ser 1)`);

        // 6. F6.
        await tabs.nth(i).focus();
        await page.keyboard.press("F6");
        const enLienzo = await page.evaluate(() => !!document.activeElement?.closest("[data-editor-main]"));
        if (!enLienzo) falla(c, "F6 desde la cinta no lleva al lienzo");
        await page.keyboard.press("F6");
        const enTab = await page.evaluate(() => {
          const a = document.activeElement;
          return a?.getAttribute("role") === "tab" && a.getAttribute("aria-selected") === "true";
        });
        if (!enTab) falla(c, "F6 desde el lienzo no lleva a la pestaña activa");
      }

      // Flechas dentro de la banda (una vez por combinación, en la última pestaña tocada).
      await tabs.first().click();
      await abrirHerramientas();
      const flechas = await page.evaluate(async () => {
        const b = document.getElementById("ribbon-banda");
        const sel = 'a[href],button:not([disabled]),input:not([disabled]),select:not([disabled])';
        const ctl = [...b.querySelectorAll(sel)].filter((e) => !e.closest(".ribbon-popover"));
        const parada = ctl.find((e) => e.tabIndex === 0) ?? ctl[0];
        parada.focus();
        b.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true, cancelable: true }));
        const tras = document.activeElement;
        return { movio: tras !== parada && b.contains(tras), n: ctl.length };
      });
      if (flechas.n > 1 && !flechas.movio) falla(id, "la flecha derecha no mueve el foco dentro de la banda");
      else ok("flechas dentro de la banda");

      await tabs.first().click();
      await page.screenshot({ path: `${OUT}/${id}.png` });
      if (errores.length) falla(id, `errores de página: ${errores.slice(0, 2).join(" | ")}`);
      await ctxB.close();
    }
  }
}

await navegador.close();
console.log(`\n${fallos.length === 0 ? "TODO EN VERDE" : `${fallos.length} FALLOS`}`);
for (const f of fallos) console.log(` - ${f}`);
process.exit(fallos.length === 0 ? 0 : 1);
