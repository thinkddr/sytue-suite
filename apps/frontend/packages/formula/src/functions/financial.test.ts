// Checks de la familia «financial» (PMT, PV, FV, NPV, IRR, RATE, NPER, IPMT,
// PPMT, SLN, SYD, DB, DDB, XNPV, XIRR, CUMIPMT, CUMPRINC) contra los valores
// documentados por una hoja de cálculo real (ejemplos de Microsoft/ODF).
// Extremo a extremo por el motor. Ejecuta: `bun test`.
import { expect, test } from 'bun:test';
import { Engine } from '../engine.js';

/** Evalúa `formula` en Z1 con las celdas `cells` fijadas antes. Valor calculado. */
function val(formula: string, cells: Record<string, string> = {}): unknown {
  const e = new Engine();
  for (const [k, v] of Object.entries(cells)) e.setCell('Hoja1', k, v);
  e.setCell('Hoja1', 'Z1', formula);
  return e.getValue('Hoja1', 'Z1');
}

/** Igual que `val` pero devuelve el texto a mostrar (errores como su código). */
function disp(formula: string, cells: Record<string, string> = {}): string {
  const e = new Engine();
  for (const [k, v] of Object.entries(cells)) e.setCell('Hoja1', k, v);
  e.setCell('Hoja1', 'Z1', formula);
  return e.getDisplay('Hoja1', 'Z1');
}

test('PMT: pago de préstamo/anualidad y nper = 0 → #DIV/0!', () => {
  // MS: PMT(0.08/12, 10, 10000) = -1037.03
  expect(val('=PMT(0.08/12, 10, 10000)') as number).toBeCloseTo(-1037.03, 2);
  // Con type=1 (pago al principio) el pago es menor en magnitud.
  expect(val('=PMT(0.08/12, 10, 10000, 0, 1)') as number).toBeCloseTo(-1030.16, 2);
  expect(disp('=PMT(0.1, 0, 1000)')).toBe('#DIV/0!');
});

test('PV / FV: valor actual y futuro (ejemplos MS)', () => {
  // MS: PV(0.08/12, 12*20, 500) = -59777.15
  expect(val('=PV(0.08/12, 12*20, 500)') as number).toBeCloseTo(-59777.15, 2);
  // MS: FV(0.06/12, 10, -200, -500, 1) = 2581.40
  expect(val('=FV(0.06/12, 10, -200, -500, 1)') as number).toBeCloseTo(2581.4, 2);
  // FV sin interés = -(pv + pmt·nper)
  expect(val('=FV(0, 12, -100, -1000)')).toBe(2200);
});

test('NPER: número de periodos (ejemplo MS) y rate = 0', () => {
  // MS: NPER(0.12/12, -100, -1000, 10000, 1) = 59.6738657
  expect(val('=NPER(0.12/12, -100, -1000, 10000, 1)') as number).toBeCloseTo(59.6738657, 5);
  // rate = 0 → -(pv+fv)/pmt
  expect(val('=NPER(0, -100, 1000)')).toBe(10);
});

test('IPMT / PPMT: descomposición del pago; identidad IPMT+PPMT=PMT', () => {
  const loan = { A1: '=0.1/12' }; // 10% anual mensual
  // Primer periodo, type=0: interés = pv·rate = 8000·0.0083333 = 66.67
  expect(val('=IPMT(A1, 1, 36, 8000)', loan) as number).toBeCloseTo(-66.67, 2);
  expect(val('=IPMT(A1, 2, 36, 8000)', loan) as number).toBeCloseTo(-65.07, 2);
  expect(val('=PPMT(A1, 1, 36, 8000)', loan) as number).toBeCloseTo(-191.47, 2);
  // type=1: el primer pago es todo principal (interés 0).
  expect(val('=IPMT(A1, 1, 36, 8000, 0, 1)')).toBe(0);
  expect(val('=IPMT(A1, 2, 36, 8000, 0, 1)', loan) as number).toBeCloseTo(-64.53, 2);
  // IPMT + PPMT = PMT en cualquier periodo.
  const ip = val('=IPMT(A1, 5, 36, 8000)', loan) as number;
  const pp = val('=PPMT(A1, 5, 36, 8000)', loan) as number;
  const pm = val('=PMT(A1, 36, 8000)', loan) as number;
  expect(ip + pp).toBeCloseTo(pm, 8);
  // periodo fuera de rango → #NUM!
  expect(disp('=IPMT(A1, 0, 36, 8000)', loan)).toBe('#NUM!');
  expect(disp('=IPMT(A1, 37, 36, 8000)', loan)).toBe('#NUM!');
});

test('RATE: tasa por periodo (iterativa) y no convergencia → #NUM!', () => {
  // MS: RATE(4*12, -200, 8000) ≈ 0.0077 mensual (9.24% anual)
  expect(val('=RATE(4*12, -200, 8000)') as number).toBeCloseTo(0.0077, 4);
  // Sin cambio de signo posible (todo positivo) → no hay tasa → #NUM!
  expect(disp('=RATE(10, 100, 100)')).toBe('#NUM!');
});

test('NPV: valor actual neto (primer flujo a un periodo vista)', () => {
  // MS: NPV(0.1, -10000, 3000, 4200, 6800) = 1188.44
  expect(val('=NPV(0.1, -10000, 3000, 4200, 6800)') as number).toBeCloseTo(1188.44, 2);
  // Con rango; texto/blanco en el rango se ignoran.
  const cells = { A1: '-10000', A2: '3000', A3: '4200', A4: '6800' };
  expect(val('=NPV(0.1, A1:A4)', cells) as number).toBeCloseTo(1188.44, 2);
});

test('IRR: tasa interna de retorno (ejemplos MS) y estimación', () => {
  const c: Record<string, string> = {
    A1: '-70000',
    A2: '12000',
    A3: '15000',
    A4: '18000',
    A5: '21000',
    A6: '26000',
  };
  // MS: IRR de los 6 valores = 0.086631; de los 5 primeros = -0.021245.
  expect(val('=IRR(A1:A6)', c) as number).toBeCloseTo(0.086631, 5);
  expect(val('=IRR(A1:A5)', c) as number).toBeCloseTo(-0.021245, 5);
  // MS: IRR(A1:A3, -10%) = -0.443507 (necesita la estimación para converger aquí).
  expect(val('=IRR(A1:A3, -0.1)', c) as number).toBeCloseTo(-0.443507, 4);
  // Sin flujos de signo mixto → #NUM!
  expect(disp('=IRR(A1:A3)', { A1: '100', A2: '200', A3: '300' })).toBe('#NUM!');
});

test('SLN / SYD: depreciación lineal y por suma de dígitos', () => {
  // MS: SLN(30000, 7500, 10) = 2250
  expect(val('=SLN(30000, 7500, 10)')).toBe(2250);
  expect(disp('=SLN(1000, 100, 0)')).toBe('#DIV/0!');
  // MS: SYD(30000, 7500, 10, 1) = 4090.91 ; SYD(...,10) = 409.09
  expect(val('=SYD(30000, 7500, 10, 1)') as number).toBeCloseTo(4090.91, 2);
  expect(val('=SYD(30000, 7500, 10, 10)') as number).toBeCloseTo(409.09, 2);
  expect(disp('=SYD(30000, 7500, 10, 11)')).toBe('#NUM!'); // per > life
});

test('DB: saldo fijo decreciente con prorrateo por meses (ejemplo MS)', () => {
  // MS DB(1000000, 100000, 6, período, 7):
  expect(val('=DB(1000000, 100000, 6, 1, 7)') as number).toBeCloseTo(186083.33, 2);
  expect(val('=DB(1000000, 100000, 6, 2, 7)') as number).toBeCloseTo(259639.42, 2);
  expect(val('=DB(1000000, 100000, 6, 7, 7)') as number).toBeCloseTo(15845.1, 2);
  expect(disp('=DB(-1, 100, 6, 1)')).toBe('#NUM!'); // cost <= 0
});

test('DDB: doble saldo decreciente con tope en salvamento (ejemplo MS)', () => {
  expect(val('=DDB(2400, 300, 10, 1)')).toBe(480);
  expect(val('=DDB(2400, 300, 10, 2)') as number).toBeCloseTo(384, 6);
  expect(val('=DDB(2400, 300, 10, 10)') as number).toBeCloseTo(22.12, 2);
  // MS: DDB(2400, 300, 3650, 1) = 1.32 (vida en días)
  expect(val('=DDB(2400, 300, 3650, 1)') as number).toBeCloseTo(1.32, 2);
  expect(disp('=DDB(2400, 300, 0, 1)')).toBe('#NUM!'); // life <= 0
});

test('XNPV / XIRR: flujos con fechas irregulares (números de serie)', () => {
  // Serie Excel: 1/1/2008=39448, 3/1/2008=39508, 10/30/2008=39751,
  // 2/15/2009=39859, 4/1/2009=39904.
  const c: Record<string, string> = {
    A1: '-10000',
    A2: '2750',
    A3: '4250',
    A4: '3250',
    A5: '2750',
    B1: '39448',
    B2: '39508',
    B3: '39751',
    B4: '39859',
    B5: '39904',
  };
  // MS: XNPV(0.09, ...) = 2086.65 ; XIRR(...) = 0.373363
  expect(val('=XNPV(0.09, A1:A5, B1:B5)', c) as number).toBeCloseTo(2086.65, 2);
  expect(val('=XIRR(A1:A5, B1:B5)', c) as number).toBeCloseTo(0.373363, 5);
  // Longitudes distintas → #NUM!
  expect(disp('=XNPV(0.09, A1:A5, B1:B4)', c)).toBe('#NUM!');
  // Una fecha anterior a la inicial → #NUM!
  expect(
    disp('=XNPV(0.09, A1:A2, B1:B2)', { A1: '-100', A2: '200', B1: '39508', B2: '39448' }),
  ).toBe('#NUM!');
});

test('CUMIPMT / CUMPRINC: interés y principal acumulados (ejemplo MS)', () => {
  // Hipoteca 125000 a 30 años al 9%. Segundo año = periodos 13..24.
  // MS: CUMIPMT = -11135.23 ; CUMPRINC = -934.11
  expect(val('=CUMIPMT(0.09/12, 30*12, 125000, 13, 24, 0)') as number).toBeCloseTo(-11135.23, 2);
  expect(val('=CUMPRINC(0.09/12, 30*12, 125000, 13, 24, 0)') as number).toBeCloseTo(-934.11, 2);
  // La suma de CUM sobre todos los periodos ≈ PMT·nper (identidad de amortización).
  const cum = val('=CUMIPMT(0.09/12, 30*12, 125000, 1, 360, 0)') as number;
  const cump = val('=CUMPRINC(0.09/12, 30*12, 125000, 1, 360, 0)') as number;
  const total = (val('=PMT(0.09/12, 30*12, 125000)') as number) * 360;
  expect(cum + cump).toBeCloseTo(total, 4);
  // Todo el principal amortizado ≈ -pv.
  expect(cump).toBeCloseTo(-125000, 4);
  // Rango de periodos inválido → #NUM!
  expect(disp('=CUMIPMT(0.09/12, 360, 125000, 24, 13, 0)')).toBe('#NUM!');
});

test('propagación de #errores y rango en contexto escalar', () => {
  // Un #error en un argumento se propaga.
  expect(disp('=SLN(1/0, 100, 5)')).toBe('#DIV/0!');
  // Rango multi-celda donde se espera un escalar → #VALUE!.
  expect(disp('=PMT(A1:A2, 10, 1000)', { A1: '0.1', A2: '0.2' })).toBe('#VALUE!');
  // Argumento obligatorio ausente → #VALUE!.
  expect(disp('=PMT(0.1)')).toBe('#VALUE!');
});

/**
 * Verdad independiente: reconstruye el cuadro de amortización con la recurrencia
 * estándar (interés sobre el saldo pendiente; en type=1 el primer pago es todo
 * principal) y comprueba que IPMT/PPMT del motor coinciden periodo a periodo.
 * Esto valida sobre todo la forma cerrada de type=1 (anualidad anticipada).
 */
function amortize(rate: number, nper: number, pv: number, type: 0 | 1, pmt: number) {
  const ip: number[] = [];
  const pp: number[] = [];
  let bal = pv;
  for (let k = 1; k <= nper; k++) {
    const interest = type === 1 && k === 1 ? 0 : -bal * rate;
    const principal = pmt - interest; // pmt = interés + principal (signos Excel)
    ip.push(interest);
    pp.push(principal);
    bal += principal; // el saldo baja hacia 0
  }
  return { ip, pp, bal };
}

test('IPMT/PPMT: coinciden periodo a periodo con una simulación (type 0 y 1)', () => {
  const rate = 0.0075; // 9% anual / 12, exacto en binario
  const nper = 24;
  const pv = 15000;
  for (const type of [0, 1] as const) {
    const pmt = val(`=PMT(${rate}, ${nper}, ${pv}, 0, ${type})`) as number;
    const { ip, pp } = amortize(rate, nper, pv, type, pmt);
    let sumPp = 0;
    for (let per = 1; per <= nper; per++) {
      const gotI = val(`=IPMT(${rate}, ${per}, ${nper}, ${pv}, 0, ${type})`) as number;
      const gotP = val(`=PPMT(${rate}, ${per}, ${nper}, ${pv}, 0, ${type})`) as number;
      expect(gotI).toBeCloseTo(ip[per - 1]!, 6);
      expect(gotP).toBeCloseTo(pp[per - 1]!, 6);
      sumPp += gotP;
    }
    // El principal total amortizado es −pv (préstamo saldado, fv=0).
    expect(sumPp).toBeCloseTo(-pv, 4);
  }
});

test('consistencia iterativa: NPV(IRR) ≈ 0 y XNPV(XIRR) ≈ 0', () => {
  const c: Record<string, string> = {
    A1: '-70000',
    A2: '12000',
    A3: '15000',
    A4: '18000',
    A5: '21000',
    A6: '26000',
  };
  const irr = val('=IRR(A1:A6)', c) as number;
  // NPV descuenta desde t=1; el flujo en t=0 (A1) va aparte.
  expect(val(`=A1 + NPV(${irr}, A2:A6)`, c) as number).toBeCloseTo(0, 4);

  const x: Record<string, string> = {
    A1: '-10000',
    A2: '2750',
    A3: '4250',
    A4: '3250',
    A5: '2750',
    B1: '39448',
    B2: '39508',
    B3: '39751',
    B4: '39859',
    B5: '39904',
  };
  const xirr = val('=XIRR(A1:A5, B1:B5)', x) as number;
  expect(val(`=XNPV(${xirr}, A1:A5, B1:B5)`, x) as number).toBeCloseTo(0, 2);
});

test('RATE con fv (meta de ahorro): verificada por FV; anualidad anticipada', () => {
  // Ahorrar 1000/mes durante 60 meses hasta 100000 (pv=0): la tasa despejada,
  // reinyectada en FV, debe devolver 100000.
  const r = val('=RATE(60, -1000, 0, 100000)') as number;
  expect(r).toBeGreaterThan(0);
  expect(val(`=FV(${r}, 60, -1000, 0)`) as number).toBeCloseTo(100000, 2);
  // PV/FV de anualidad anticipada = vencida · (1+rate).
  const pv0 = val('=PV(0.05, 10, -1000, 0, 0)') as number;
  expect(val('=PV(0.05, 10, -1000, 0, 1)') as number).toBeCloseTo(pv0 * 1.05, 6);
  const fv0 = val('=FV(0.05, 10, -1000, 0, 0)') as number;
  expect(val('=FV(0.05, 10, -1000, 0, 1)') as number).toBeCloseTo(fv0 * 1.05, 6);
});

test('DDB con factor personalizado y CUM type=1', () => {
  // factor 1.5 → f = 1.5/10 = 0.15. Periodo 1 = 2400·0.15 = 360; periodo 2 = 306.
  expect(val('=DDB(2400, 300, 10, 1, 1.5)') as number).toBeCloseTo(360, 6);
  expect(val('=DDB(2400, 300, 10, 2, 1.5)') as number).toBeCloseTo(306, 6);
  expect(disp('=DDB(2400, 300, 10, 1, 0)')).toBe('#NUM!'); // factor <= 0

  // CUMIPMT/CUMPRINC con type=1: identidades de amortización sobre toda la vida.
  const cum = val('=CUMIPMT(0.08/12, 120, 20000, 1, 120, 1)') as number;
  const cump = val('=CUMPRINC(0.08/12, 120, 20000, 1, 120, 1)') as number;
  const pmt = val('=PMT(0.08/12, 120, 20000, 0, 1)') as number;
  expect(cum + cump).toBeCloseTo(pmt * 120, 4);
  expect(cump).toBeCloseTo(-20000, 4);
});

test('propagación de #errores en agregados y validación de rango de periodos', () => {
  // Un #error dentro del rango de flujos se propaga (NPV, IRR).
  expect(disp('=NPV(0.1, A1:A3)', { A1: '1', A2: '=1/0', A3: '3' })).toBe('#DIV/0!');
  expect(disp('=IRR(A1:A3)', { A1: '-1', A2: '=1/0', A3: '3' })).toBe('#DIV/0!');
  // XIRR con menos de dos flujos → #NUM!.
  expect(disp('=XIRR(A1:A1, B1:B1)', { A1: '-100', B1: '39448' })).toBe('#NUM!');
  // CUMPRINC con end > nper → #NUM!.
  expect(disp('=CUMPRINC(0.1/12, 12, 1000, 1, 13, 0)')).toBe('#NUM!');
});

// VDB: decreciente con cambio a lineal. Todos los números son los PUBLICADOS por
// Microsoft para la misma función; si alguno se mueve, es que el algoritmo dejó de
// ser el de Excel —y una amortización que no cuadra con la hoja del asesor no
// sirve de nada—.
test('VDB: los ejemplos publicados por Microsoft', () => {
  // Vida en días, meses y años sobre el mismo activo (2400 → 300 en 10 años).
  expect(val('=VDB(2400, 300, 3650, 0, 1)') as number).toBeCloseTo(1.32, 2);
  expect(val('=VDB(2400, 300, 120, 0, 1)') as number).toBeCloseTo(40, 2);
  expect(val('=VDB(2400, 300, 10, 0, 1)') as number).toBeCloseTo(480, 2);
  // Periodo FRACCIONARIO: 0,875 de año.
  expect(val('=VDB(2400, 300, 10, 0, 0.875)') as number).toBeCloseTo(420, 2);
  // Tramo intermedio: del mes 6 al 18. No es restar dos acumuladas —el cambio a
  // lineal se decide con la vida QUE QUEDA—, y por eso no sale 329,15.
  expect(val('=VDB(2400, 300, 120, 6, 18)') as number).toBeCloseTo(396.31, 2);
  expect(val('=VDB(2400, 300, 120, 6, 18, 1.5)') as number).toBeCloseTo(311.81, 2);
  // Sin cambio a lineal: decreciente pura durante toda la vida.
  expect(val('=VDB(2400, 300, 10, 0, 10, 2, TRUE)') as number).toBeCloseTo(2100, 2);
});

test('VDB: argumentos imposibles dan #NUM!', () => {
  expect(disp('=VDB(2400, 300, 0, 0, 1)')).toBe('#NUM!'); // vida 0
  expect(disp('=VDB(2400, 300, 10, 5, 2)')).toBe('#NUM!'); // fin antes que inicio
  expect(disp('=VDB(2400, 300, 10, 0, 11)')).toBe('#NUM!'); // fin más allá de la vida
  expect(disp('=VDB(-1, 300, 10, 0, 1)')).toBe('#NUM!'); // coste negativo
});
