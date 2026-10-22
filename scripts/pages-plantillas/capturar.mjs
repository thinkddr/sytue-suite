// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
// Fotografía la salida de vista-previa.mjs con Playwright (1280 y 390 px).
//   bun capturar.mjs <vista-previa> <salida> [claves|all] [home|all] [light|light,dark]
import { chromium } from 'playwright-core';
import { readdirSync, mkdirSync, existsSync, statSync } from 'node:fs';
import { join } from 'node:path';

const [raiz, salida, claves = 'all', paginas = 'home', esquemas = 'light'] = process.argv.slice(2);
const lista = claves === 'all' ? readdirSync(raiz).filter((d) => d !== 'fuentes' && statSync(join(raiz, d)).isDirectory()).sort() : claves.split(',');
mkdirSync(salida, { recursive: true });
const server = Bun.serve({
  port: 0,
  async fetch(req) {
    const ruta = join(raiz, decodeURIComponent(new URL(req.url).pathname));
    const f = Bun.file(ruta);
    return (await f.exists()) && !ruta.endsWith('/') ? new Response(f) : new Response('no', { status: 404 });
  },
});
const base = `http://localhost:${server.port}`;
const browser = await chromium.launch();
for (const clave of lista) {
  const dir = join(raiz, clave);
  const subs = paginas === 'home' ? [''] : ['', ...readdirSync(dir).filter((d) => existsSync(join(dir, d, 'index.html')))];
  for (const sub of subs) {
    for (const esquema of esquemas.split(',')) {
      for (const [ancho, alto, nombre] of [[1280, 800, 'escritorio'], [390, 844, 'movil']]) {
        const ctx = await browser.newContext({ viewport: { width: ancho, height: alto }, colorScheme: esquema });
        const page = await ctx.newPage();
        await page.goto(`${base}/${clave}/${sub ? sub + '/' : ''}index.html`, { waitUntil: 'load' });
        await page.evaluate(() => document.fonts.ready);
        const horiz = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
        const f = join(salida, `${clave}${sub ? '-' + sub : ''}-${nombre}-${esquema === 'dark' ? 'oscuro' : 'claro'}.png`);
        await page.screenshot({ path: f, fullPage: true });
        if (horiz > 1) console.log(`AVISO desborde horizontal ${horiz}px en ${f}`);
        await ctx.close();
      }
    }
  }
}
await browser.close();
server.stop();
console.log('capturas hechas:', lista.length);
