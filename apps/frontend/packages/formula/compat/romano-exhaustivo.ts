/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * ROMAN(n, forma) para TODO n de 1 a 3999 y las cinco formas, contra LibreOffice: 20.000
 * fórmulas, demasiadas para el corpus (que se lee a ojo) pero baratas de comprobar de una
 * vez. No es un test (necesita soffice): se ejecuta a mano al tocar ROMAN.
 *
 *   cd apps/frontend/packages/formula && bun compat/romano-exhaustivo.ts
 */
import { Engine } from '../src/engine';
import { calcular } from './libreoffice';

const formulas: string[] = [];
for (let f = 0; f <= 4; f++) for (let n = 1; n <= 3999; n++) formulas.push(`=ROMAN(${n},${f})`);
const lo = await calcular(formulas);
const e = new Engine();
let mal = 0;
formulas.forEach((f, i) => {
  e.setCell('Hoja1', `A${i + 1}`, f);
  const nuestro = e.getValue('Hoja1', `A${i + 1}`);
  if (nuestro !== lo.casos[i]!.lo) {
    if (mal++ < 20)
      process.stdout.write(
        `${f}: nuestro ${String(nuestro)} · LibreOffice ${JSON.stringify(lo.casos[i]!.lo)}\n`,
      );
  }
});
process.stdout.write(`${formulas.length} comprobadas, ${mal} distintas\n`);
