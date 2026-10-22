/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los VECTORES COMPARTIDOS de Calcus con la app nativa (`vectores/calcus.ts`): lo que
 * la web calcula hoy tiene que ser lo que está guardado en `vectores/calcus.json`, que
 * es lo mismo que exige `apps/fyne/calcus/vectores_test.go` al motor de Go. Si esto se
 * pone rojo tras un cambio DELIBERADO, se regenera el JSON y el lado de Go se ajusta.
 */
import { expect, test } from 'bun:test';
import { calcularVectores } from './vectores/calcus';

test('los vectores compartidos con la app nativa siguen dando lo mismo en la web', async () => {
  const guardados: unknown = await Bun.file(
    new URL('./vectores/calcus.json', import.meta.url),
  ).json();
  expect(JSON.parse(JSON.stringify(calcularVectores()))).toEqual(guardados);
});
