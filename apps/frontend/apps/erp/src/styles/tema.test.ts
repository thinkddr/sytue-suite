/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La hoja del ERP solo pinta con tokens. Un color fijo no conmuta a oscuro: la
 * etiqueta con el nombre del producto sobre su foto en el TPV llevaba un gris
 * claro y una tinta oscura escritos a mano, y en grafito salía como una mancha
 * clara en mitad de una pantalla oscura. Se comprueba sobre el fuente porque es
 * una regla de escritura: cualquier regla nueva tiene que cumplirla.
 */
import { describe, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

const hoja = readFileSync(join(import.meta.dir, 'app.css'), 'utf8');
// Los comentarios pueden citar un color para explicar por qué ya no está.
const sinComentarios = hoja.replace(/\/\*[\s\S]*?\*\//g, '');

/** Cuerpo de la regla cuyo selector es exactamente `selector`. */
function regla(selector: string): string {
  const i = sinComentarios.indexOf(`${selector} {`);
  expect(i).toBeGreaterThanOrEqual(0);
  return sinComentarios.slice(i, sinComentarios.indexOf('}', i));
}

describe('tema de la hoja del ERP', () => {
  test('no hay colores fijos (hex, rgb, hsl): todo sale de tokens que conmutan', () => {
    const fijos = sinComentarios
      .split('\n')
      .map((linea, i) => ({ linea, n: i + 1 }))
      .filter(({ linea }) => /#[0-9a-f]{3,8}\b|\b(rgba?|hsla?)\(/i.test(linea))
      .map(({ n, linea }) => `app.css:${n}: ${linea.trim()}`);
    expect(fijos).toEqual([]);
  });

  test('la etiqueta sobre la foto del TPV usa superficie opaca y tinta del tema', () => {
    const etiqueta = regla('.erp-tpv-product-with-image .erp-tpv-product-info');
    expect(etiqueta).toContain('var(--sytue-surface-raised)');
    expect(etiqueta).toContain('color: var(--sytue-text)');
    const codigo = regla('.erp-tpv-product-with-image .erp-tpv-product-code');
    // El código no va dentro de la etiqueta sino sobre la foto: también opaco.
    expect(codigo).toContain('var(--sytue-surface-raised)');
    expect(codigo).not.toContain('bg-subtle');
    expect(codigo).toContain('var(--sytue-text-muted)');
  });
});
