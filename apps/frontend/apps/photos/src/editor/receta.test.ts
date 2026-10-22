/**
 * PARIDAD CON GO. Los vectores de referencia los imprimió el propio
 * services/syphotos/internal/domain (huella y forma canónica) el 2026-09-08. Si
 * alguno cambia, cliente y servidor están nombrando derivadas distintas y el
 * editor enseña una foto y descarga otra. Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import { canonica, huella, recetaVacia, validaReceta, type Receta } from './receta.js';

const completa: Receta = {
  version: 1,
  pasos: [
    { tipo: 'recorte', params: { x: 0.1, y: 0.2, ancho: 0.5, alto: 0.25 } },
    { tipo: 'giro', params: { grados: 90 } },
    { tipo: 'espejo', eje: 'h' },
    { tipo: 'temperatura', params: { valor: -0.333333333 } },
  ],
};

test('la forma canónica es byte a byte la de Go', () => {
  expect(canonica(recetaVacia())).toBe('{"pasos":[],"version":1}');
  expect(canonica({ version: 1, pasos: [{ tipo: 'exposicion', params: { valor: 0.5 } }] })).toBe(
    '{"pasos":[{"params":{"valor":"0.500000"},"tipo":"exposicion"}],"version":1}',
  );
  expect(canonica(completa)).toBe(
    '{"pasos":[{"params":{"alto":"0.250000","ancho":"0.500000","x":"0.100000","y":"0.200000"},"tipo":"recorte"},{"params":{"grados":"90.000000"},"tipo":"giro"},{"eje":"h","tipo":"espejo"},{"params":{"valor":"-0.333333"},"tipo":"temperatura"}],"version":1}',
  );
});

test('la huella es la que calcula Go', async () => {
  expect(await huella(recetaVacia())).toBe('ec5adbfaaa1f');
  expect(
    await huella({ version: 1, pasos: [{ tipo: 'exposicion', params: { valor: 0.5 } }] }),
  ).toBe('53600d98c18e');
  expect(await huella(completa)).toBe('1119c7c11660');
});

// El ruido del último bit de un deslizador no puede ser una derivada nueva.
test('el ruido de los flotantes no cambia la huella; un ajuste real sí', async () => {
  const a = await huella({ version: 1, pasos: [{ tipo: 'exposicion', params: { valor: 0.5 } }] });
  const b = await huella({
    version: 1,
    // Un valor que el doble no distingue de 0,5: sumar un cuarto de épsilon
    // se redondea a 0,5 exacto (la separación entre dobles ahí es ≈1,1e-16).
    pasos: [{ tipo: 'exposicion', params: { valor: 0.5 + Number.EPSILON / 4 } }],
  });
  const c = await huella({ version: 1, pasos: [{ tipo: 'exposicion', params: { valor: 0.51 } }] });
  expect(a).toBe(b);
  expect(a).not.toBe(c);
});

test('el orden de los pasos cuenta', async () => {
  const giro = { tipo: 'giro', params: { grados: 90 } } as const;
  const recorte = { tipo: 'recorte', params: { x: 0, y: 0, ancho: 0.5, alto: 0.5 } } as const;
  expect(await huella({ version: 1, pasos: [giro, recorte] })).not.toBe(
    await huella({ version: 1, pasos: [recorte, giro] }),
  );
});

test('rechaza lo que Go rechaza', () => {
  const malas: Receta[] = [
    { version: 1, pasos: [{ tipo: 'psicodelia' as never }] },
    { version: 1, pasos: [{ tipo: 'exposicion', params: { valor: 42 } }] },
    { version: 1, pasos: [{ tipo: 'exposicion', params: { valor: 0, gamma: 1 } }] },
    { version: 1, pasos: [{ tipo: 'exposicion' }] },
    { version: 1, pasos: [{ tipo: 'espejo' }] },
    { version: 1, pasos: [{ tipo: 'recorte', params: { x: 0.8, y: 0, ancho: 0.5, alto: 0.5 } }] },
    { version: 1, pasos: [{ tipo: 'recorte', params: { x: 0, y: 0, ancho: 0, alto: 0.5 } }] },
    { version: 1, pasos: [{ tipo: 'contraste', params: { valor: Number.NaN } }] },
    { version: 1, pasos: [{ tipo: 'contraste', params: { valor: Number.POSITIVE_INFINITY } }] },
    { version: 2, pasos: [] },
    { version: 0, pasos: [] },
  ];
  for (const r of malas) expect(validaReceta(r)).not.toBeNull();
  expect(validaReceta(completa)).toBeNull();
  expect(validaReceta(recetaVacia())).toBeNull();
});
