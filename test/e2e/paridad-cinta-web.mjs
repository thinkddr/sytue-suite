// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// PARIDAD DE COMANDOS de la cinta web con el registro común (`apps/nucleo/office/comandos.json`):
// qué comandos del registro, en cada tipo de documento, NO tienen control en la cinta que
// pinta el navegador. Es la mitad web de la medida que la app nativa hace en Go
// (`editor.ComandosSinControl`): las dos comparan contra el MISMO registro, así que la
// brecha de Fyne se lee contra la de la web y no contra una intuición.
//
// Mide el banco de pruebas `/dev/cinta` (la cinta sola, con un contexto de mentira), abre
// una a una todas las pestañas del tipo y junta los nombres de sus controles (`aria-label`,
// `title` y texto) con el rótulo en español de cada comando (`labelKey`).
//
// Es una GUARDA POR IGUALDAD: lo medido tiene que ser igual a `paridad-cinta-web.json`.
// Cerrar una brecha obliga a quitarla de ahí y abrir una nueva obliga a anotarla; una
// regresión (un comando que desaparece de la cinta) falla en vez de pasar en silencio.
// `--escribir` regenera el fichero con lo medido.
//
// Los comandos con control dentro de un popover cerrado no se ven desde fuera: la medida
// es, como la nativa, la de los controles que cada pestaña PINTA.
//
// Uso: como `cinta.mjs` (CINTA_URL, el servidor dev de apps/docus en marcha).
import { chromium } from 'playwright';
import { readFileSync, writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { es } from '../../apps/frontend/packages/office/src/i18n/messages/es.ts';

const BASE = process.env.CINTA_URL || 'http://127.0.0.1:4338/docus/prueba';
const REGISTRO = JSON.parse(
  readFileSync(new URL('../../apps/nucleo/office/comandos.json', import.meta.url), 'utf8'),
);
const FICHERO = fileURLToPath(new URL('./paridad-cinta-web.json', import.meta.url));
const KINDS = ['document', 'spreadsheet', 'presentation'];
const norm = (s) => s.replace(/\s+/g, ' ').trim().toLowerCase();

const navegador = await chromium.launch();
const medido = {};
for (const kind of KINDS) {
  const page = await navegador.newPage({ viewport: { width: 1600, height: 900 } });
  // El grupo «Bloques» solo se pinta cuando el servicio entrega su catálogo
  // (`BloquesGroup`: sin lista no hay desplegable). El banco no tiene sesión y el
  // servidor de desarrollo contesta 401, así que se le da un catálogo de un bloque: lo
  // que se mide es si el comando tiene control, no qué bloques hay.
  await page.route(/\/api\/blocks(\?|$)/, (r) =>
    r.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ blocks: [{ key: 'firmas', name: 'Tabla de firmas' }], org_blocks: [] }),
    }),
  );
  await page.goto(`${BASE}/dev/cinta?kind=${kind}&lang=es`, { timeout: 180000 });
  await page.waitForSelector('[role="tablist"]', { timeout: 90000 });
  const tabs = page.locator('[role="tab"]');
  const n = await tabs.count();
  const nombres = new Set();
  for (let i = 0; i < n; i++) {
    await tabs.nth(i).click();
    await page.waitForTimeout(120);
    // «Bloques» llega con una petición: se espera a que la red calle antes de leer la banda.
    await page.waitForLoadState('networkidle', { timeout: 10000 }).catch(() => undefined);
    const textos = await page.evaluate(() => {
      const banda = document.getElementById('ribbon-banda');
      const out = [];
      for (const e of banda?.querySelectorAll('*') ?? []) {
        for (const a of ['aria-label', 'title']) {
          const v = e.getAttribute(a);
          if (v) out.push(v);
        }
        if (e.children.length === 0 && e.textContent) out.push(e.textContent);
      }
      return out;
    });
    for (const t of textos) nombres.add(norm(t));
  }
  const juntos = [...nombres];
  const faltan = Object.entries(REGISTRO)
    .filter(([, c]) => c.kinds.includes(kind))
    .filter(([, c]) => {
      const rotulo = es[c.labelKey];
      return !(typeof rotulo === 'string' && juntos.some((t) => t.includes(norm(rotulo))));
    })
    .map(([id]) => id)
    .sort();
  medido[kind] = faltan;
  await page.close();
}
await navegador.close();

const texto = JSON.stringify(medido, null, 2) + '\n';
if (process.argv.includes('--escribir')) {
  writeFileSync(FICHERO, texto);
  console.log(`escrito ${FICHERO}`);
  process.exit(0);
}
let anotado = {};
try {
  anotado = JSON.parse(readFileSync(FICHERO, 'utf8'));
} catch {
  // sin fichero: todo lo medido cuenta como nuevo
}
let fallos = 0;
for (const kind of KINDS) {
  const m = new Set(medido[kind]);
  const a = new Set(anotado[kind] ?? []);
  const nuevos = [...m].filter((x) => !a.has(x));
  const resueltos = [...a].filter((x) => !m.has(x));
  console.log(`${kind}: ${m.size} comandos sin control (${KINDS.length ? 'anotados ' + a.size : ''})`);
  if (nuevos.length) {
    fallos++;
    console.log(`  ✗ sin control y sin anotar: ${nuevos.join(', ')}`);
  }
  if (resueltos.length) {
    fallos++;
    console.log(`  ✗ anotados pero ya con control (quítalos de paridad-cinta-web.json): ${resueltos.join(', ')}`);
  }
}
console.log(fallos === 0 ? 'TODO EN VERDE' : `${fallos} FALLOS (node/bun paridad-cinta-web.mjs --escribir regenera)`);
process.exit(fallos === 0 ? 0 : 1);
