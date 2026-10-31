import { expect, test } from 'bun:test';
import { APP_VIEWPORT } from './viewport.js';

test('las apps de la suite usan el contrato común de viewport, sin extenderlo al sitio público', async () => {
  expect(APP_VIEWPORT).toContain('viewport-fit=cover');
  expect(APP_VIEWPORT).toContain('interactive-widget=resizes-content');
  expect(APP_VIEWPORT).not.toContain('user-scalable=no');
  const frontend = new URL('../../../', import.meta.url).pathname;
  for await (const ruta of new Bun.Glob('apps/*/src/layouts/*.astro').scan(frontend)) {
    const fuente = await Bun.file(`${frontend}${ruta}`).text();
    if (ruta.includes('/www/') || !fuente.includes('name="viewport"')) continue;
    expect(fuente, ruta).toContain('content={APP_VIEWPORT}');
  }
  const office = await Bun.file(
    new URL('../../office/src/layouts/AppLayout.astro', import.meta.url),
  ).text();
  expect(office).toContain('content={APP_VIEWPORT}');
});

test('Hub y Office reservan las zonas seguras también en Carril sin crear cajas en papel', async () => {
  const tema = await Bun.file(new URL('./theme.css', import.meta.url)).text();
  const pantalla = tema.slice(tema.indexOf('@media screen {'));
  expect(pantalla).toMatch(/\.sytue-safe-workspace\.sytue-frame-page\s*\{\s*display:\s*block;/);
  const apariencia = await Bun.file(new URL('./tokens/appearance.css', import.meta.url)).text();
  expect(apariencia).toMatch(
    /:is\(\.sytue-frame-page,\s*\.sytue-sheet-page\)\s*\{\s*display:\s*contents;/,
  );
  for (const ruta of [
    '../../office/src/components/Shell.tsx',
    '../../../apps/portal/src/components/Shell.tsx',
  ]) {
    const shell = await Bun.file(new URL(ruta, import.meta.url)).text();
    expect(shell, ruta).toMatch(/class="[^"]*sytue-frame-page[^"]*sytue-safe-workspace/);
    // Office y Hub comparten exactamente los mismos márgenes de hoja en Marco (sin bleed).
    expect(shell, ruta).toContain('class="sytue-sheet sytue-sheet-page"');
    expect(shell, ruta).not.toContain('sytue-sheet-bleed');
  }
});

test('los editores Office dejan el inset inferior y el margen de la hoja al main que los contiene', async () => {
  const office = new URL('../../office/src/', import.meta.url);
  const shell = await Bun.file(new URL('components/Shell.tsx', office)).text();
  expect(shell).toContain('sytue-safe-workspace');
  for (const ruta of ['vistas/Edicion.astro', 'vistas/Pdf.astro']) {
    const fuente = await Bun.file(new URL(ruta, office)).text();
    const alturas = [...fuente.matchAll(/h-\[calc\((100dvh[^\]]+)\)\]/g)];
    expect(alturas.length, ruta).toBeGreaterThan(0);
    for (const [, altura] of alturas) {
      // Sin esta resta, el editor acaba bajo el indicador o añade los 10 px de margen
      // inferior de la hoja fuera del viewport creando un segundo scroll.
      expect(altura, ruta).toBe(
        '100dvh-var(--spacing-topbar)-var(--sytue-sheet-gap,0px)-var(--sytue-safe-bottom,0px)',
      );
    }
    expect(fuente, ruta).not.toMatch(/sytue-safe-(?:workspace|fullscreen)/);
    if (ruta.includes('Edicion')) expect(fuente).toContain('min-h-[32rem]');
  }
});
