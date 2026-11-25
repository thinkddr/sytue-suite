/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El contexto derivado no LEE el original: un getter como `csp` solo se evalúa si
 * alguien lo pide. Copiarlo con `...` lo leía en cada petición y es lo que escribía
 * «context.csp was used … but CSP was not configured» en el log de producción.
 * Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import { readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
import type { APIContext } from 'astro';
import { contextoDerivado } from './contexto-derivado.js';

function contextoConGetter() {
  let lecturas = 0;
  const ctx = {
    params: { ruta: 'a' },
    request: new Request('https://drive.ejemplo/x'),
    locals: { locale: 'es' },
    get csp() {
      lecturas++;
      return undefined;
    },
  } as unknown as APIContext;
  return { ctx, lecturas: () => lecturas };
}

test('cambia params (y request si se pide) sin evaluar los getters del original', () => {
  const { ctx, lecturas } = contextoConGetter();
  const otra = new Request('https://drive.ejemplo/y');
  const sub = contextoDerivado(ctx, { ruta: 'migration/a' }, { request: otra });
  expect(lecturas()).toBe(0);
  expect(sub.params).toEqual({ ruta: 'migration/a' });
  expect(sub.request).toBe(otra);
  // Lo demás se hereda tal cual, y el getter sigue funcionando si alguien lo pide.
  expect(sub.locals).toBe(ctx.locals);
  expect(sub.csp).toBeUndefined();
  expect(lecturas()).toBe(1);
  // El original no cambia.
  expect(ctx.params).toEqual({ ruta: 'a' });
});

test('copiar con `...` sí evalúa el getter: es exactamente lo que se evita', () => {
  const { ctx, lecturas } = contextoConGetter();
  void { ...ctx, params: {} };
  expect(lecturas()).toBe(1);
});

test('ninguna ruta del paquete vuelve a copiar el contexto con `...ctx`', () => {
  const dir = import.meta.dir;
  const culpables = readdirSync(dir)
    // El propio helper lo nombra en su comentario: es el único que puede.
    .filter((f) => f.endsWith('.ts') && !f.endsWith('.test.ts') && f !== 'contexto-derivado.ts')
    .filter((f) => /\.\.\.ctx\b(?!\.)/.test(readFileSync(join(dir, f), 'utf8')));
  expect(culpables).toEqual([]);
});
