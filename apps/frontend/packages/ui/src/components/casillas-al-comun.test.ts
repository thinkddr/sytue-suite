/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * NINGUNA CASILLA NI RADIO CRUDOS FUERA DE `Form.tsx`.
 *
 * Chromium deja a 0 el `border-radius` de un `<input type="checkbox|radio">`, así que
 * el aro de foco que se le pinte encima es un cuadrado. `Checkbox`/`Radio` lo arreglan
 * con un `<span class="sytue-focus-ring-choice">` que lleva la esquina; unas 55
 * casillas y 7 radios escritos a mano seguían con el aro cuadrado cuando se midió.
 *
 * SI ESTE TEST TE PARA: usa `Checkbox`/`Radio` de '@sytue/ui'. Sin `label` sale solo
 * el control, para meterlo en tu propio `<label>` o nombrarlo con `aria-label`.
 * Las excepciones se comparan POR IGUALDAD: si una deja de serlo, bórrala de aquí.
 */
import { expect, test } from 'bun:test';

const RAIZ = new URL('../../../../', import.meta.url).pathname; // apps/frontend/

/** `<input` … `type="checkbox|radio"` dentro de la misma etiqueta; `=>` no la cierra. */
const CRUDO = /<input\b(?:=>|[^>])*?\btype=["'`{]*(?:checkbox|radio)\b/g;

/** Fichero → [cuántos crudos lleva, por qué]. Se cuenta para que no se cuele otro al lado. */
const EXCEPCIONES: Record<string, [number, string]> = {
  'packages/ui/src/components/ChoiceCard.tsx': [
    1,
    'radio `sr-only`: el aro lo pinta la TARJETA con `.sytue-focus-ring-inset-within`',
  ],
  'packages/office/src/pdf/Paneles.tsx': [
    1,
    'radio `sr-only` del segmentado «Organizar»: el control visible es la etiqueta',
  ],
  'apps/notes/src/components/RichText.tsx': [
    4,
    'HTML de una lista de tareas dentro de un `contenteditable`, no JSX; el aro lo pinta `.task-row::before` en notes/src/styles/app.css',
  ],
  'packages/pages-render/src/pintar/index.ts': [
    2,
    'HTML de la página publicada (cadena, no JSX): el sitio público no carga @sytue/ui',
  ],
  'apps/notes/src/client/markdown.ts': [1, 'HTML que se inserta en el editor, no JSX'],
  'apps/notes/src/client/plantillas.ts': [1, 'HTML de plantilla para el editor, no JSX'],
};

test('las casillas y los radios salen de Checkbox/Radio', async () => {
  const conCrudos: Record<string, number> = {};
  const glob = new Bun.Glob('{apps,packages}/*/src/**/*.{ts,tsx,astro}');
  for await (const rel of glob.scan({ cwd: RAIZ })) {
    if (rel.includes('.test.') || rel.endsWith('packages/ui/src/components/Form.tsx')) continue;
    // Sin comentarios: la documentación también escribe `<input type="radio">`.
    const codigo = (await Bun.file(RAIZ + rel).text()).replace(
      /\/\*[\s\S]*?\*\/|^\s*\/\/.*$/gm,
      '',
    );
    const n = codigo.match(CRUDO)?.length ?? 0;
    if (n) conCrudos[rel] = n;
  }
  const esperado = Object.fromEntries(Object.entries(EXCEPCIONES).map(([f, [n]]) => [f, n]));
  expect(conCrudos).toEqual(esperado);
});
