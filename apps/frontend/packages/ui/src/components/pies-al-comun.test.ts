/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LAS ACCIONES DE UN DIÁLOGO VAN SUELTAS EN SU PIE, SIN ENVOLTORIO.
 *
 * El pie del `Modal` reparte sus acciones: a la derecha en escritorio y, en el teléfono,
 * a todo el ancho, de dos en dos y con el principal abajo a la derecha. Lo hace con sus
 * HIJOS. Un pie que mete sus botones en `<div class="flex justify-end">` le da un solo
 * hijo, y en el teléfono se quedaba con las acciones apiñadas a la derecha, a su ancho,
 * mientras el resto de la suite las ponía a todo el ancho: así estaban 25 de los 99 pies
 * de la suite cuando se midió (2026-10-16), casi todos para conseguir lo que el pie ya
 * hace solo. La acción que va a la izquierda (`Eliminar`) lleva `class="md:mr-auto"`.
 *
 * SI ESTE TEST TE PARA: quita el envoltorio y deja los `<Button>` en un fragmento. Las
 * excepciones son pies que no son una botonera, y se comparan POR IGUALDAD: si una deja
 * de serlo, bórrala de aquí.
 */
import { expect, test } from 'bun:test';

const RAIZ = new URL('../../../../', import.meta.url).pathname; // apps/frontend/

/** Fichero → por qué su pie trae su propio reparto. Ruta relativa a `apps/frontend/`. */
const EXCEPCIONES: Record<string, string> = {
  'apps/mail/src/components/ComposeModal.tsx':
    'el pie del redactor es una barra (formato, adjuntar, programar, descartar) y el envío: ocupa la fila entera y se reparte por dentro, con «Enviar» creciendo en el teléfono',
  'packages/office/src/editor/dev/SuperposicionesDemo.tsx':
    'página de desarrollo que prueba a propósito un pie propio que no desborda (test/e2e/superposiciones.mjs)',
  'apps/mail/src/components/ExternalMail.tsx':
    'el pie aloja el contenedor de destino con ref para los botones del Wizard de cuentas de correo externas',
  'apps/rooms/src/components/ExternalConversations.tsx':
    'el pie aloja el contenedor de destino con ref para las acciones del Wizard de conversaciones externas',
  'apps/rooms/src/components/IntegrationsModal.tsx':
    'el pie aloja el contenedor de destino con ref para las acciones del Wizard de conexiones de WhatsApp',
};

/** Los pies de `<Modal>`/`<ConfirmDialog>` del código (no los `footer` de una barra lateral). */
function piesEnvueltos(codigo: string): number {
  let n = 0;
  for (const m of codigo.matchAll(/\bfooter=\{/g)) {
    // De quién es la prop: la última etiqueta de componente abierta antes de ella.
    const antes = codigo.slice(0, m.index);
    const etiqueta = [...antes.matchAll(/<([A-Z]\w*)\b/g)].at(-1)?.[1];
    if (etiqueta !== 'Modal' && etiqueta !== 'ConfirmDialog') continue;
    let prof = 1;
    let j = m.index + m[0].length;
    while (prof > 0 && j < codigo.length) {
      if (codigo[j] === '{') prof++;
      else if (codigo[j] === '}') prof--;
      j++;
    }
    const expr = codigo.slice(m.index + m[0].length, j - 1);
    // Un `<div` como raíz del pie o de una rama del ternario.
    if (/(?:^|[?:(])\s*<div\b/.test(expr)) n++;
  }
  return n;
}

test('los pies de los diálogos dejan sus acciones sueltas', async () => {
  const envueltos: Record<string, string> = {};
  const glob = new Bun.Glob('{apps,packages}/*/src/**/*.tsx');
  for await (const rel of glob.scan({ cwd: RAIZ })) {
    if (rel.includes('.test.')) continue;
    // Sin comentarios: la documentación también escribe pies de ejemplo.
    const codigo = (await Bun.file(RAIZ + rel).text()).replace(
      /\/\*[\s\S]*?\*\/|^\s*\/\/.*$/gm,
      '',
    );
    if (piesEnvueltos(codigo) > 0)
      envueltos[rel] = EXCEPCIONES[rel] ?? 'SIN MOTIVO: quita el envoltorio';
  }
  expect(envueltos).toEqual(EXCEPCIONES);
});
