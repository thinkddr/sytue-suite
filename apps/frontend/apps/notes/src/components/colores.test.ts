/** Cada tono de las paletas de Notas tiene nombre propio en el catálogo. */
import { expect, test } from 'bun:test';
import { COLORES_DE_TEXTO, conNombres } from './colores.js';
import { en } from '../i18n/messages/en.js';
import type { T } from '../i18n/index.js';

const t = ((k: string) => (en as Record<string, unknown>)[k] as string) as unknown as T;

test('ningún tono cae al nombre genérico ni se anuncia por su hex', () => {
  for (const paleta of [COLORES_DE_TEXTO]) {
    const nombres = conNombres(t, paleta).map((c) => c.nombre);
    for (const n of nombres) {
      expect(n).not.toBe(en['notebook.color']);
      expect(n).not.toMatch(/^#/);
    }
    // Dentro de una paleta, dos tonos no pueden llamarse igual.
    expect(new Set(nombres).size).toBe(nombres.length);
  }
});
