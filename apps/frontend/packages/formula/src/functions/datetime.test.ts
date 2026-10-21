// Checks de datetime: sistema de fecha serie 1900 (ECMA-376), con valores
// conocidos de una hoja de cálculo real y casos de error/dominio. `bun test`.
import { expect, test } from 'bun:test';
import { Engine } from '../engine.js';
import type { CellValue } from '../values.js';

/** Reloj fijo inyectable: 2026-07-02 12:00:00 hora LOCAL. */
const FIXED = () => new Date(2026, 6, 2, 12, 0, 0);

/** Evalúa «=…» en A1 y devuelve el valor calculado (reloj inyectable fijo). */
function val(formula: string): CellValue {
  const e = new Engine({ now: FIXED });
  e.setCell('Hoja1', 'A1', formula);
  return e.getValue('Hoja1', 'A1');
}

/** Como val() pero exige un número (falla ruidosamente si sale error/texto). */
function num(formula: string): number {
  const v = val(formula);
  if (typeof v !== 'number') throw new Error(`esperaba número, dio ${String(v)}`);
  return v;
}

/** Texto a mostrar: comprueba códigos de error y el formato «General». */
function disp(formula: string): string {
  const e = new Engine({ now: FIXED });
  e.setCell('Hoja1', 'A1', formula);
  return e.getDisplay('Hoja1', 'A1');
}

test('DATE: seriales conocidos y el bug de 1900 bisiesto', () => {
  expect(num('=DATE(1900,1,1)')).toBe(1); // origen del sistema 1900
  expect(num('=DATE(1900,2,28)')).toBe(59);
  expect(num('=DATE(1900,3,1)')).toBe(61); // salta el día fantasma (60)
  expect(num('=DATE(2008,1,1)')).toBe(39448); // ancla clásica de Excel
  expect(num('=DATE(2000,1,1)')).toBe(36526);
  expect(num('=DATE(2009,1,1)')).toBe(39814); // 2008 bisiesto → +366
});

test('DATE: rollover de mes/día y ajuste de año', () => {
  expect(num('=DATE(2008,13,1)')).toBe(num('=DATE(2009,1,1)')); // mes 13 → enero siguiente
  expect(num('=DATE(2008,1,0)')).toBe(39447); // día 0 → 2007-12-31
  expect(num('=DATE(2008,0,1)')).toBe(num('=DATE(2007,12,1)')); // mes 0 → diciembre anterior
  expect(num('=DATE(0,1,1)')).toBe(num('=DATE(1900,1,1)')); // año 0-1899 → +1900
  expect(num('=DATE(108,1,1)')).toBe(num('=DATE(2008,1,1)'));
});

test('DATE: dominios de error', () => {
  expect(disp('=DATE(10000,1,1)')).toBe('#NUM!'); // año fuera de rango
  expect(disp('=DATE(-1,1,1)')).toBe('#NUM!');
  expect(disp('=DATE(1900,1,-5)')).toBe('#NUM!'); // serial negativo
  expect(disp('=DATE(1/0,1,1)')).toBe('#DIV/0!'); // el error se propaga
  expect(disp('=DATE("x",1,1)')).toBe('#VALUE!');
  expect(disp('=DATE(2008,1)')).toBe('#VALUE!'); // aridad
});

test('TIME: fracción de día', () => {
  expect(num('=TIME(12,0,0)')).toBe(0.5);
  expect(num('=TIME(6,0,0)')).toBe(0.25);
  expect(num('=TIME(18,0,0)')).toBe(0.75);
  expect(num('=TIME(0,0,0)')).toBe(0);
  expect(num('=TIME(24,0,0)')).toBe(0); // envuelve mód 24 h
  expect(num('=TIME(36,0,0)')).toBe(0.5);
  expect(num('=TIME(10,30,0)')).toBeCloseTo(0.4375, 12);
  expect(disp('=TIME(-1,0,0)')).toBe('#NUM!'); // negativo
  expect(disp('=TIME(0,0)')).toBe('#VALUE!');
});

test('YEAR/MONTH/DAY', () => {
  expect(num('=YEAR(DATE(2008,5,23))')).toBe(2008);
  expect(num('=MONTH(DATE(2008,5,23))')).toBe(5);
  expect(num('=DAY(DATE(2008,5,23))')).toBe(23);
  expect(num('=YEAR(39448)')).toBe(2008); // serial directo
  expect(num('=DAY(DATE(2008,2,29))')).toBe(29); // 2008 bisiesto
  // Coacción de texto de fecha (como Excel):
  expect(num('=YEAR("2008-05-15")')).toBe(2008);
  expect(num('=MONTH("2008-05-15")')).toBe(5);
});

test('YEAR/MONTH/DAY: celda en blanco ≈ serial 0 (quirk 1900-01-00 de Excel)', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'B1', '=YEAR(A1)'); // A1 vacía → serial 0
  e.setCell('Hoja1', 'B2', '=MONTH(A1)');
  e.setCell('Hoja1', 'B3', '=DAY(A1)');
  expect(e.getValue('Hoja1', 'B1')).toBe(1900);
  expect(e.getValue('Hoja1', 'B2')).toBe(1);
  expect(e.getValue('Hoja1', 'B3')).toBe(0);
});

test('HOUR/MINUTE/SECOND', () => {
  expect(num('=HOUR(TIME(15,30,45))')).toBe(15);
  expect(num('=MINUTE(TIME(15,30,45))')).toBe(30);
  expect(num('=SECOND(TIME(15,30,45))')).toBe(45);
  expect(num('=HOUR(0.5)')).toBe(12); // mediodía
  expect(num('=HOUR(DATE(2008,1,1)+TIME(23,59,59))')).toBe(23); // con parte de fecha
  expect(num('=MINUTE(DATE(2008,1,1)+TIME(23,59,59))')).toBe(59);
  expect(num('=SECOND(DATE(2008,1,1)+TIME(23,59,59))')).toBe(59); // sin desliz de f64
  expect(disp('=HOUR("no")')).toBe('#VALUE!');
});

test('WEEKDAY: tipos 1, 2, 3 y 11-17', () => {
  // 2008-01-01 fue martes.
  expect(num('=WEEKDAY(DATE(2008,1,1))')).toBe(3); // tipo 1: dom=1 → mar=3
  expect(num('=WEEKDAY(DATE(2008,1,1),1)')).toBe(3);
  expect(num('=WEEKDAY(DATE(2008,1,1),2)')).toBe(2); // tipo 2: lun=1 → mar=2
  expect(num('=WEEKDAY(DATE(2008,1,1),3)')).toBe(1); // tipo 3: lun=0 → mar=1
  expect(num('=WEEKDAY(DATE(2008,1,6))')).toBe(1); // domingo
  expect(num('=WEEKDAY(DATE(2008,1,1),11)')).toBe(2); // 11 = lun=1
  expect(num('=WEEKDAY(DATE(2008,1,1),13)')).toBe(7); // 13 = mié=1 → mar=7
  expect(num('=WEEKDAY(1)')).toBe(1); // 1900-01-01 = domingo
  expect(disp('=WEEKDAY(DATE(2008,1,1),4)')).toBe('#NUM!'); // tipo inválido
});

test('WEEKNUM: sistema Excel (tipos 1/2) e ISO (21)', () => {
  expect(num('=WEEKNUM(DATE(2008,1,1))')).toBe(1);
  expect(num('=WEEKNUM(DATE(2008,1,6))')).toBe(2); // domingo abre semana 2 (tipo 1)
  expect(num('=WEEKNUM(DATE(2008,1,6),2)')).toBe(1); // lun: sigue en semana 1
  expect(num('=WEEKNUM(DATE(2008,12,31))')).toBe(53);
  expect(num('=WEEKNUM(DATE(2008,12,29),21)')).toBe(1); // ISO: semana 1 de 2009
  expect(num('=WEEKNUM(DATE(2009,1,1),21)')).toBe(1);
  expect(disp('=WEEKNUM(DATE(2008,1,1),5)')).toBe('#NUM!');
});

test('EDATE y EOMONTH con clamp de fin de mes', () => {
  expect(num('=EDATE(DATE(2008,1,31),1)')).toBe(num('=DATE(2008,2,29)')); // clamp a 29
  expect(num('=EDATE(DATE(2008,3,31),-1)')).toBe(num('=DATE(2008,2,29)'));
  expect(num('=EDATE(DATE(2008,1,15),12)')).toBe(num('=DATE(2009,1,15)'));
  expect(num('=EOMONTH(DATE(2008,2,15),0)')).toBe(num('=DATE(2008,2,29)'));
  expect(num('=EOMONTH(DATE(2008,1,15),1)')).toBe(num('=DATE(2008,2,29)'));
  expect(num('=EOMONTH(DATE(2008,3,31),-1)')).toBe(num('=DATE(2008,2,29)'));
  expect(disp('=EDATE("x",1)')).toBe('#VALUE!');
});

test('DAYS', () => {
  expect(num('=DAYS(DATE(2008,3,1),DATE(2008,1,1))')).toBe(60);
  expect(num('=DAYS(DATE(2008,1,1),DATE(2008,3,1))')).toBe(-60);
  expect(num('=DAYS("2008-03-01","2008-01-01")')).toBe(60); // coacción de texto
  expect(disp('=DAYS(1/0,1)')).toBe('#DIV/0!');
});

test('DATEDIF: unidades Y, M, D, YM, MD, YD', () => {
  const a = 'DATE(2001,1,1),DATE(2003,3,15)';
  expect(num(`=DATEDIF(${a},"Y")`)).toBe(2);
  expect(num(`=DATEDIF(${a},"M")`)).toBe(26);
  expect(num(`=DATEDIF(${a},"D")`)).toBe(803);
  expect(num(`=DATEDIF(${a},"YM")`)).toBe(2);
  expect(num(`=DATEDIF(${a},"MD")`)).toBe(14);
  expect(num(`=DATEDIF(${a},"YD")`)).toBe(73);
  expect(num('=DATEDIF(DATE(2020,2,29),DATE(2021,2,28),"Y")')).toBe(0); // aún no cumple el año
  expect(disp('=DATEDIF(DATE(2003,1,1),DATE(2001,1,1),"Y")')).toBe('#NUM!'); // start>end
  expect(disp(`=DATEDIF(${a},"Z")`)).toBe('#NUM!'); // unidad inválida
});

test('NETWORKDAYS', () => {
  expect(num('=NETWORKDAYS(DATE(2008,1,1),DATE(2008,1,7))')).toBe(5);
  expect(num('=NETWORKDAYS(DATE(2008,1,1),DATE(2008,1,31))')).toBe(23); // enero completo
  expect(num('=NETWORKDAYS(DATE(2008,1,7),DATE(2008,1,1))')).toBe(-5); // invertido → negativo
  expect(num('=NETWORKDAYS(DATE(2008,1,5),DATE(2008,1,6))')).toBe(0); // fin de semana
  // Con festivos (un solo día y un rango):
  const e = new Engine();
  e.setCell('Hoja1', 'H1', '=DATE(2008,1,1)'); // martes
  e.setCell('Hoja1', 'H2', '=DATE(2008,1,4)'); // viernes
  e.setCell('Hoja1', 'C1', '=NETWORKDAYS(DATE(2008,1,1),DATE(2008,1,7),H1:H2)');
  expect(e.getValue('Hoja1', 'C1')).toBe(3); // 5 − 2 festivos laborables
  expect(disp('=NETWORKDAYS(1/0,2)')).toBe('#DIV/0!');
});

test('WORKDAY', () => {
  expect(num('=WORKDAY(DATE(2008,1,1),1)')).toBe(num('=DATE(2008,1,2)'));
  expect(num('=WORKDAY(DATE(2008,1,4),1)')).toBe(num('=DATE(2008,1,7)')); // salta el finde
  expect(num('=WORKDAY(DATE(2008,1,1),5)')).toBe(num('=DATE(2008,1,8)'));
  expect(num('=WORKDAY(DATE(2008,1,1),-1)')).toBe(num('=DATE(2007,12,31)')); // hacia atrás
  expect(num('=WORKDAY(DATE(2008,1,1),0)')).toBe(num('=DATE(2008,1,1)')); // 0 → mismo día
  const e = new Engine();
  e.setCell('Hoja1', 'H1', '=DATE(2008,1,7)'); // festivo lunes
  e.setCell('Hoja1', 'C1', '=WORKDAY(DATE(2008,1,4),1,H1)');
  expect(e.getValue('Hoja1', 'C1')).toBe((e.getValue('Hoja1', 'H1') as number) + 1); // salta al martes
});

test('rango de fechas válido [0, 2958465]: negativo y desbordamiento → #NUM!', () => {
  // Serial de inicio negativo (antes de 1900-01-00): las funciones que PRODUCEN
  // una fecha lo rechazan, coherente con YEAR/WEEKDAY/DATEDIF.
  expect(disp('=EDATE(-5,1)')).toBe('#NUM!');
  expect(disp('=EOMONTH(-5,0)')).toBe('#NUM!'); // antes: devolvía 0 (serial válido de basura)
  expect(disp('=WORKDAY(-5,1)')).toBe('#NUM!');
  expect(disp('=WORKDAY(DATE(1900,1,10),-20)')).toBe('#NUM!'); // retrocede por debajo de 1900
  // Desbordamiento por encima de 9999-12-31 (serial 2958465, el máximo válido).
  expect(num('=DATE(9999,12,31)')).toBe(2958465); // el límite superior es válido
  expect(disp('=DATE(9999,13,1)')).toBe('#NUM!'); // rollover de mes → año 10000
  expect(disp('=EDATE(DATE(9999,12,1),2)')).toBe('#NUM!');
  expect(disp('=EOMONTH(DATE(9999,12,1),1)')).toBe('#NUM!');
});

test('DATEVALUE', () => {
  expect(num('=DATEVALUE("2008-01-01")')).toBe(39448);
  expect(num('=DATEVALUE("2008/1/1")')).toBe(39448);
  expect(num('=DATEVALUE("1/1/2008")')).toBe(39448); // US M/D/AAAA
  expect(num('=DATEVALUE("12/31/2007")')).toBe(39447);
  expect(num('=DATEVALUE("2008-01-01 13:30")')).toBe(39448); // ignora la hora
  expect(disp('=DATEVALUE("2008-13-01")')).toBe('#VALUE!'); // mes inválido
  expect(disp('=DATEVALUE("13:30")')).toBe('#VALUE!'); // solo hora
  expect(disp('=DATEVALUE("basura")')).toBe('#VALUE!');
});

test('TIMEVALUE', () => {
  expect(num('=TIMEVALUE("12:00")')).toBe(0.5);
  expect(num('=TIMEVALUE("06:00:00")')).toBe(0.25);
  expect(num('=TIMEVALUE("1:30 PM")')).toBeCloseTo(0.5625, 12);
  expect(num('=TIMEVALUE("12:00 AM")')).toBe(0); // medianoche
  expect(num('=TIMEVALUE("12:00 PM")')).toBe(0.5); // mediodía
  expect(num('=TIMEVALUE("2008-01-01 06:00")')).toBe(0.25); // ignora la fecha
  expect(disp('=TIMEVALUE("25:00")')).toBe('#VALUE!');
  expect(disp('=TIMEVALUE("nope")')).toBe('#VALUE!');
});

test('TODAY y NOW usan el reloj inyectable', () => {
  // Reloj fijo = 2026-07-02 12:00 local.
  expect(num('=TODAY()')).toBe(num('=DATE(2026,7,2)'));
  expect(num('=NOW()')).toBeCloseTo(num('=DATE(2026,7,2)') + 0.5, 9); // +mediodía
  expect(num('=NOW()-TODAY()')).toBeCloseTo(0.5, 9);
  expect(disp('=TODAY(1)')).toBe('#VALUE!'); // no admite argumentos
  expect(disp('=NOW(1)')).toBe('#VALUE!');
});

test('round-trip DATE ↔ YEAR/MONTH/DAY sobre un rango de fechas', () => {
  const e = new Engine();
  const cases: [number, number, number][] = [
    [1900, 1, 1],
    [1900, 3, 1],
    [1999, 12, 31],
    [2000, 2, 29],
    [2008, 5, 23],
    [2026, 7, 2],
    [2400, 2, 29],
  ];
  cases.forEach(([y, m, d], i) => {
    const row = i + 1;
    e.setCell('Hoja1', `A${row}`, `=DATE(${y},${m},${d})`);
    e.setCell('Hoja1', `B${row}`, `=YEAR(A${row})`);
    e.setCell('Hoja1', `C${row}`, `=MONTH(A${row})`);
    e.setCell('Hoja1', `D${row}`, `=DAY(A${row})`);
    expect(e.getValue('Hoja1', `B${row}`)).toBe(y);
    expect(e.getValue('Hoja1', `C${row}`)).toBe(m);
    expect(e.getValue('Hoja1', `D${row}`)).toBe(d);
  });
});
