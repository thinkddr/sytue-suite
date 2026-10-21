/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Números complejos (COMPLEX + IM*).
 *
 * En una hoja de cálculo un complejo no es un tipo: es TEXTO con forma «a+bi». Así que
 * la función de verdad son el parser y el formateador, y ahí van casi todos los casos.
 * La aritmética en sí solo se prueba donde tiene trampa (el signo de IMCOS, el polo de
 * IMARGUMENT, el denominador nulo de IMDIV).
 */
import { expect, test } from 'bun:test';
import { Engine } from '../engine.js';
import type { CellValue } from '../values.js';

function ev(formula: string): CellValue | string {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', formula);
  const v = e.getValue('Hoja1', 'A1');
  return typeof v === 'object' && v !== null && 'code' in v ? (v as { code: string }).code : v;
}

test('el formateador omite lo que una hoja omite: «3+i», no «3+1i»', () => {
  // Este es el detalle que delata una implementación ingenua. Excel escribe el
  // coeficiente 1 como nada, y quien compare textos lo nota enseguida.
  expect(ev('=COMPLEX(3,1)')).toBe('3+i');
  expect(ev('=COMPLEX(3,-1)')).toBe('3-i');
  expect(ev('=COMPLEX(0,1)')).toBe('i');
  expect(ev('=COMPLEX(0,-1)')).toBe('-i');
  // Parte nula: desaparece entera.
  expect(ev('=COMPLEX(3,0)')).toBe('3');
  expect(ev('=COMPLEX(0,4)')).toBe('4i');
  expect(ev('=COMPLEX(0,0)')).toBe('0');
  expect(ev('=COMPLEX(3,4)')).toBe('3+4i');
  expect(ev('=COMPLEX(3,-4)')).toBe('3-4i');
});

test('el parser NO confunde el signo del exponente con el separador de partes', () => {
  // «1.5E+3+2i» tiene DOS signos «+». El bueno es el segundo: el primero pertenece a la
  // notación científica. Un parser que corte por el primero lee 1.5E de parte real y se
  // queda tan ancho, porque el resultado sigue siendo un número.
  expect(ev('=IMREAL("1.5E+3+2i")')).toBe(1500);
  expect(ev('=IMAGINARY("1.5E+3+2i")')).toBe(2);
  expect(ev('=IMREAL("2E-2-3i")')).toBe(0.02);
  expect(ev('=IMAGINARY("2E-2-3i")')).toBe(-3);

  // El caso que de verdad exige saltar el exponente: cuando está en la parte IMAGINARIA.
  // Recorriendo de derecha a izquierda, el primer signo que aparece es el del exponente,
  // y cortar ahí deja «3+1.5E» de parte real — que no es un número y da #NUM!.
  expect(ev('=IMREAL("3+1.5E+2i")')).toBe(3);
  expect(ev('=IMAGINARY("3+1.5E+2i")')).toBe(150);
});

test('el parser acepta las formas abreviadas y el negativo inicial', () => {
  expect(ev('=IMAGINARY("i")')).toBe(1);
  expect(ev('=IMAGINARY("-i")')).toBe(-1);
  expect(ev('=IMAGINARY("+i")')).toBe(1);
  expect(ev('=IMREAL("-3+4i")')).toBe(-3); // el signo de posición 0 no separa nada
  expect(ev('=IMAGINARY("-3+4i")')).toBe(4);
  expect(ev('=IMREAL("5")')).toBe(5); // sin parte imaginaria
  expect(ev('=IMAGINARY("5")')).toBe(0);
  expect(ev('=IMAGINARY("3+j")')).toBe(1); // sufijo j
});

test('mezclar sufijos i y j es #VALUE!, no elegir uno en silencio', () => {
  // Son la misma unidad imaginaria, pero el resultado no puede llevar los dos; escoger
  // por el llamador sorprendería a quien lee la celda.
  expect(ev('=IMSUM("1+i","1+j")')).toBe('#VALUE!');
  expect(ev('=IMSUB("1+i","1+j")')).toBe('#VALUE!');
  // Con el mismo sufijo, se conserva el que venía.
  expect(ev('=IMSUM("1+2j","3+4j")')).toBe('4+6j');
});

test('IMARGUMENT usa atan2 y el origen no tiene argumento', () => {
  // `atan(im/re)` daría el cuadrante equivocado en el semiplano izquierdo y dividiría
  // por cero sobre el eje imaginario.
  expect(ev('=IMARGUMENT("-1+0i")') as number).toBeCloseTo(Math.PI, 10);
  expect(ev('=IMARGUMENT("0+2i")') as number).toBeCloseTo(Math.PI / 2, 10);
  expect(ev('=IMARGUMENT("0")')).toBe('#DIV/0!');
});

test('IMCOS no es IMSIN con el signo cambiado a medias', () => {
  // cos(a+bi) = cos(a)cosh(b) − i·sen(a)senh(b): el término imaginario va NEGADO, al
  // revés que en el seno. Copiar la fórmula del seno y olvidar el signo es el fallo
  // habitual, y solo se ve con parte imaginaria distinta de cero.
  expect(ev('=IMCOS("0")')).toBe('1');
  const c = ev('=IMCOS("1+1i")') as string;
  expect(c.startsWith('0.83')).toBe(true); // cos(1)cosh(1) ≈ 0.8337
  expect(c).toContain('-'); // el término imaginario es negativo
  const s = ev('=IMSIN("1+1i")') as string;
  expect(s).toContain('+'); // y el del seno, positivo
});

test('IMDIV con denominador nulo es #NUM!', () => {
  expect(ev('=IMDIV("1+i","0")')).toBe('#NUM!');
  // División normal: (1+i)/(1-i) = i
  expect(ev('=IMDIV("1+i","1-i")')).toBe('i');
});

test('IMSQRT e IMPOWER por forma polar', () => {
  expect(ev('=IMSQRT("-1")')).toContain('i'); // √(−1) = i
  // (1+i)² es 2i EXACTO. Por forma polar saldría «1.2246e-16+2.0000000000000004i»:
  // sen(π/2) no da 0 redondo y el residuo acaba en la celda del usuario. Con exponente
  // entero se multiplica, que además es exacto.
  expect(ev('=IMPOWER("1+i",2)')).toBe('2i');
  expect(ev('=IMPOWER("1+i",4)')).toBe('-4');
  expect(ev('=IMPOWER("2",10)')).toBe('1024');
  // Exponente negativo: (1+i)⁻¹ = 0.5-0.5i
  expect(ev('=IMPOWER("1+i",-1)')).toBe('0.5-0.5i');
  // Fraccionario: ahí la polar es el único camino y el residuo es inevitable.
  expect(ev('=IMPOWER("1+i",0.5)') as string).toContain('i');
  // 0 elevado a 0 no está definido.
  expect(ev('=IMPOWER("0",0)')).toBe('#NUM!');
});

test('IMLN del origen no existe', () => {
  expect(ev('=IMLN("0")')).toBe('#NUM!');
  expect(ev('=IMLOG10("0")')).toBe('#NUM!');
  expect(ev('=IMLN("1")')).toBe('0'); // ln(1) = 0
});

test('IMABS usa hypot y no desborda con componentes grandes', () => {
  expect(ev('=IMABS("3+4i")')).toBe(5);
  // 1e200² desborda un double; hypot lo calcula sin elevar al cuadrado primero.
  const r = ev('=IMABS("1E200+1E200i")') as number;
  expect(Number.isFinite(r)).toBe(true);
});

test('IMCONJUGATE y las operaciones conservan el texto canónico', () => {
  expect(ev('=IMCONJUGATE("3+4i")')).toBe('3-4i');
  expect(ev('=IMCONJUGATE("3-4i")')).toBe('3+4i');
  expect(ev('=IMSUM("1+2i","3+4i")')).toBe('4+6i');
  expect(ev('=IMSUB("5+6i","1+2i")')).toBe('4+4i');
  expect(ev('=IMPRODUCT("1+2i","3+4i")')).toBe('-5+10i');
});

/* ---------- Trigonométricas e hiperbólicas complejas ---------- */

/** Partes real e imaginaria de lo que devuelva una IM*, para comparar números y
 *  no cadenas: el texto canónico arrastra los 17 dígitos del double. */
function partes(formula: string): [number, number] {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', formula);
  e.setCell('Hoja1', 'B1', '=IMREAL(A1)');
  e.setCell('Hoja1', 'C1', '=IMAGINARY(A1)');
  return [e.getValue('Hoja1', 'B1') as number, e.getValue('Hoja1', 'C1') as number];
}

test('IMTAN, IMCOT, IMSEC e IMCSC contra valores conocidos', () => {
  // Referencias calculadas aparte con aritmética compleja de doble precisión.
  const casos: [string, number, number][] = [
    ['=IMTAN("1+i")', 0.2717525853195118, 1.0839233273386943],
    ['=IMTAN("3+4i")', -0.00018734620462947839, 0.99935598738147302],
    ['=IMTAN("0.5-2i")', 0.030215987322877582, -0.97994084996173803],
    ['=IMCOT("1+i")', 0.21762156185440279, -0.86801414289592493],
    ['=IMCOT("3+4i")', -0.0001875877379836592, -1.0006443924715591],
    ['=IMSEC("1+i")', 0.4983370305551868, 0.591083841721045],
    ['=IMSEC("0.5-2i")', 0.2371138960159079, -0.1248761915565979],
    ['=IMCSC("1+i")', 0.62151801717042843, -0.30393100162842646],
    ['=IMCSC("3+4i")', 0.0051744731840193967, 0.036275889628626011],
  ];
  for (const [f, re, im] of casos) {
    const [gr, gi] = partes(f);
    expect(gr).toBeCloseTo(re, 12);
    expect(gi).toBeCloseTo(im, 12);
  }
});

test('IMSINH, IMCOSH, IMSECH e IMCSCH no son las circulares con otro nombre', () => {
  // senh(a+bi) pone el argumento REAL en las hiperbólicas y el imaginario en las
  // circulares, justo al revés que sen(a+bi). Y en cosh el término imaginario es
  // POSITIVO, mientras que en cos va negado. Copiar una de la otra da un número
  // con la pinta correcta y equivocado, así que se comparan las cuatro.
  const casos: [string, number, number][] = [
    ['=IMSINH("1+i")', 0.63496391478473613, 1.2984575814159773],
    ['=IMSINH("3+4i")', -6.5481200409110025, -7.6192317203214106],
    ['=IMCOSH("1+i")', 0.83373002513114913, 0.98889770576286506],
    ['=IMCOSH("0.5-2i")', -0.46925797822905341, -0.473830620416407],
    ['=IMSECH("1+i")', 0.4983370305551868, -0.591083841721045],
    ['=IMSECH("0.5-2i")', -1.0551797842192612, 1.0654618887767002],
    ['=IMCSCH("1+i")', 0.30393100162842646, -0.62151801717042843],
    ['=IMCSCH("3+4i")', -0.064877471370635487, 0.075489832915863697],
  ];
  for (const [f, re, im] of casos) {
    const [gr, gi] = partes(f);
    expect(gr).toBeCloseTo(re, 12);
    expect(gi).toBeCloseTo(im, 12);
  }
  // Sobre el eje real se reducen a las hiperbólicas de toda la vida.
  expect(partes('=IMSINH("2")')[0]).toBeCloseTo(Math.sinh(2), 12);
  expect(partes('=IMCOSH("2")')[0]).toBeCloseTo(Math.cosh(2), 12);
  // Y sobre el imaginario, senh(bi) = i·sen(b) y cosh(bi) = cos(b).
  expect(partes('=IMSINH("2i")')[1]).toBeCloseTo(Math.sin(2), 12);
  expect(partes('=IMCOSH("2i")')[0]).toBeCloseTo(Math.cos(2), 12);
});

test('los polos son #NUM!, no un número enorme', () => {
  // cot y csc tienen polo donde el seno se anula (z real múltiplo de π), sech y
  // csch donde se anula su hiperbólica. Con el cero EXACTO no hay número que
  // devolver, y dar 1e17 sería peor que el error.
  expect(ev('=IMCOT("0")')).toBe('#NUM!');
  expect(ev('=IMCSC("0")')).toBe('#NUM!');
  expect(ev('=IMCSCH("0")')).toBe('#NUM!');
  // La tangente y la secante no tienen polo en 0 (el coseno vale 1 ahí).
  expect(ev('=IMTAN("0")')).toBe('0');
  expect(ev('=IMSEC("0")')).toBe('1');
  expect(ev('=IMSECH("0")')).toBe('1');
  expect(ev('=IMCOSH("0")')).toBe('1');
  expect(ev('=IMSINH("0")')).toBe('0');
  // El error del argumento se propaga tal cual, no se traduce a #NUM!.
  expect(ev('=IMTAN(1/0)')).toBe('#DIV/0!');
});
