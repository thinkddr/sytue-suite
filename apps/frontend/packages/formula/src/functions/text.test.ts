// Checks de las funciones de TEXTO contra valores conocidos de una hoja real.
// Se ejercita el motor completo (registro + coacciones + pereza). `bun test`.
import { expect, test } from 'bun:test';
import { Engine } from '../engine.js';
import type { CellValue } from '../values.js';

/** Evalúa una fórmula (con celdas de apoyo opcionales) y devuelve el valor. */
function calc(formula: string, cells: Record<string, string> = {}): CellValue {
  const e = new Engine();
  for (const [k, v] of Object.entries(cells)) e.setCell('Hoja1', k, v);
  e.setCell('Hoja1', 'Z99', formula);
  return e.getValue('Hoja1', 'Z99');
}

/** Igual que calc pero devuelve el texto a mostrar (para códigos de error). */
function show(formula: string, cells: Record<string, string> = {}): string {
  const e = new Engine();
  for (const [k, v] of Object.entries(cells)) e.setCell('Hoja1', k, v);
  e.setCell('Hoja1', 'Z99', formula);
  return e.getDisplay('Hoja1', 'Z99');
}

test('CONCAT / CONCATENATE', () => {
  expect(calc('=CONCAT("a","b","c")')).toBe('abc');
  expect(calc('=CONCATENATE("Hello"," ","World")')).toBe('Hello World');
  expect(calc('=CONCAT(1,2,3)')).toBe('123');
  expect(calc('=CONCAT(TRUE,"!")')).toBe('TRUE!');
  expect(calc('=CONCAT(A1:B1)', { A1: 'x', B1: 'y' })).toBe('xy');
  expect(show('=CONCAT(1/0,"x")')).toBe('#DIV/0!');
});

test('LEFT / RIGHT / MID', () => {
  expect(calc('=LEFT("hello",3)')).toBe('hel');
  expect(calc('=LEFT("hello")')).toBe('h');
  expect(calc('=LEFT("hi",10)')).toBe('hi');
  expect(calc('=LEFT(12345,2)')).toBe('12');
  expect(show('=LEFT("hi",-1)')).toBe('#VALUE!');
  expect(calc('=RIGHT("hello",2)')).toBe('lo');
  expect(calc('=RIGHT("hello")')).toBe('o');
  expect(calc('=RIGHT("hi",10)')).toBe('hi');
  expect(calc('=MID("hello",2,3)')).toBe('ell');
  expect(calc('=MID("hello",3,100)')).toBe('llo');
  expect(calc('=MID("hello",10,2)')).toBe('');
  expect(show('=MID("hello",0,2)')).toBe('#VALUE!');
});

test('LEN / LOWER / UPPER / PROPER', () => {
  expect(calc('=LEN("hello")')).toBe(5);
  expect(calc('=LEN("")')).toBe(0);
  expect(calc('=LEN(123)')).toBe(3);
  expect(calc('=LOWER("HeLLo")')).toBe('hello');
  expect(calc('=UPPER("HeLLo")')).toBe('HELLO');
  expect(calc('=PROPER("hello world")')).toBe('Hello World');
  expect(calc('=PROPER("JOHN SMITH")')).toBe('John Smith');
  expect(calc('=PROPER("abc2def")')).toBe('Abc2Def');
});

test('TRIM', () => {
  expect(calc('=TRIM("  a   b  ")')).toBe('a b');
  expect(calc('=TRIM("  hola  mundo  ")')).toBe('hola mundo');
});

test('REPLACE / SUBSTITUTE', () => {
  expect(calc('=REPLACE("abcde",2,3,"X")')).toBe('aXe');
  expect(calc('=REPLACE("abc",5,2,"X")')).toBe('abcX');
  expect(show('=REPLACE("abc",0,1,"X")')).toBe('#VALUE!');
  expect(calc('=SUBSTITUTE("a-b-c","-","+")')).toBe('a+b+c');
  expect(calc('=SUBSTITUTE("a-b-c","-","+",2)')).toBe('a-b+c');
  expect(calc('=SUBSTITUTE("hello","","X")')).toBe('hello');
  expect(calc('=SUBSTITUTE("aaa","a","b",5)')).toBe('aaa');
});

test('FIND (sensible a mayúsculas, sin comodines)', () => {
  expect(calc('=FIND("b","abc")')).toBe(2);
  expect(calc('=FIND("cd","abcd")')).toBe(3);
  expect(calc('=FIND("a","abcabc",2)')).toBe(4);
  expect(calc('=FIND("","abc")')).toBe(1);
  expect(show('=FIND("x","abc")')).toBe('#VALUE!');
  expect(show('=FIND("B","abc")')).toBe('#VALUE!');
});

test('SEARCH (insensible a mayúsculas, con comodines)', () => {
  expect(calc('=SEARCH("B","abc")')).toBe(2);
  expect(calc('=SEARCH("c","abcabc",4)')).toBe(6);
  expect(calc('=SEARCH("a*d","abcd")')).toBe(1);
  expect(calc('=SEARCH("b*","aXbYc")')).toBe(3);
  expect(calc('=SEARCH("~*","a*b")')).toBe(2); // «~*» = asterisco literal
  expect(show('=SEARCH("z","abc")')).toBe('#VALUE!');
});

test('TEXT — formatos numéricos', () => {
  expect(calc('=TEXT(3.14159,"0.00")')).toBe('3.14');
  expect(calc('=TEXT(1234567,"#,##0")')).toBe('1,234,567');
  expect(calc('=TEXT(1234.5,"#,##0.00")')).toBe('1,234.50');
  expect(calc('=TEXT(0.25,"0%")')).toBe('25%');
  expect(calc('=TEXT(0.125,"0.0%")')).toBe('12.5%');
  expect(calc('=TEXT(-5,"0.00")')).toBe('-5.00');
  expect(calc('=TEXT(5,"0000")')).toBe('0005');
  expect(calc('=TEXT("hello","0.00")')).toBe('hello'); // texto → tal cual
});

test('TEXT — formatos de fecha y hora', () => {
  expect(calc('=TEXT(44562,"yyyy-mm-dd")')).toBe('2022-01-01');
  expect(calc('=TEXT(44562,"dd/mm/yyyy")')).toBe('01/01/2022');
  expect(calc('=TEXT(44562,"mmm")')).toBe('Jan');
  expect(calc('=TEXT(0.75,"h:mm")')).toBe('18:00');
  expect(calc('=TEXT(0.75,"hh:mm AM/PM")')).toBe('06:00 PM');
  // «mm» = mes tras «-», = minutos tras «hh:».
  expect(calc('=TEXT(44562.5,"yyyy-mm-dd hh:mm:ss")')).toBe('2022-01-01 12:00:00');
});

test('VALUE / NUMBERVALUE', () => {
  expect(calc('=VALUE("123")')).toBe(123);
  expect(calc('=VALUE("12.5")')).toBe(12.5);
  expect(calc('=VALUE("50%")')).toBe(0.5);
  expect(calc('=VALUE("$1,234.50")')).toBe(1234.5);
  expect(calc('=VALUE(42)')).toBe(42);
  expect(show('=VALUE("abc")')).toBe('#VALUE!');
  expect(calc('=NUMBERVALUE("2.5")')).toBe(2.5);
  expect(calc('=NUMBERVALUE("1.234,56",",",".")')).toBe(1234.56);
  expect(calc('=NUMBERVALUE("")')).toBe(0);
  expect(calc('=NUMBERVALUE("3.5%")')).toBe(0.035);
  expect(show('=NUMBERVALUE("1.2.3")')).toBe('#VALUE!');
});

test('REPT / TEXTJOIN', () => {
  expect(calc('=REPT("ab",3)')).toBe('ababab');
  expect(calc('=REPT("x",0)')).toBe('');
  expect(show('=REPT("x",-1)')).toBe('#VALUE!');
  expect(calc('=TEXTJOIN("-",TRUE,"a","","b")')).toBe('a-b');
  expect(calc('=TEXTJOIN("-",FALSE,"a","","b")')).toBe('a--b');
  expect(calc('=TEXTJOIN(",",TRUE,A1:A3)', { A1: 'x', A3: 'z' })).toBe('x,z');
});

test('CHAR / CODE / UNICHAR / UNICODE', () => {
  expect(calc('=CHAR(65)')).toBe('A');
  expect(calc('=CHAR(97)')).toBe('a');
  expect(show('=CHAR(0)')).toBe('#VALUE!');
  expect(show('=CHAR(256)')).toBe('#VALUE!');
  expect(calc('=CODE("A")')).toBe(65);
  expect(calc('=CODE("abc")')).toBe(97);
  expect(show('=CODE("")')).toBe('#VALUE!');
  expect(calc('=UNICHAR(65)')).toBe('A');
  expect(calc('=UNICHAR(8364)')).toBe('€');
  expect(show('=UNICHAR(0)')).toBe('#VALUE!');
  expect(calc('=UNICODE("A")')).toBe(65);
  expect(calc('=UNICODE("€")')).toBe(8364);
});

test('EXACT / CLEAN / T', () => {
  expect(calc('=EXACT("abc","abc")')).toBe(true);
  expect(calc('=EXACT("abc","ABC")')).toBe(false);
  expect(calc('=EXACT("Word","word")')).toBe(false);
  expect(calc('=CLEAN(CHAR(9)&"hi"&CHAR(10))')).toBe('hi');
  expect(calc('=T("abc")')).toBe('abc');
  expect(calc('=T(123)')).toBe('');
  expect(calc('=T(TRUE)')).toBe('');
  expect(show('=T(1/0)')).toBe('#DIV/0!');
});

test('errores por número de argumentos y rango en contexto escalar', () => {
  expect(show('=LEFT()')).toBe('#VALUE!'); // faltan argumentos → no revienta
  expect(show('=LEN(A1:A3)', { A1: '1', A2: '2', A3: '3' })).toBe('#VALUE!');
});

test('TEXT — redondeo aritmético (mitad se aleja del cero)', () => {
  expect(calc('=TEXT(2.5,"0")')).toBe('3');
  expect(calc('=TEXT(0.5,"0")')).toBe('1');
  expect(calc('=TEXT(1.5,"0")')).toBe('2');
  expect(calc('=TEXT(1234.5,"#,##0")')).toBe('1,235');
  expect(calc('=TEXT(-2.5,"0")')).toBe('-3');
  expect(calc('=TEXT(2.45,"0.0")')).toBe('2.5');
  expect(calc('=TEXT(0.125,"0.00")')).toBe('0.13'); // 0.125 → 0.13
});

test('TEXT — no imprime «-0» cuando la magnitud redondea a cero', () => {
  // Regresión: antes daba "-0" / "-0.0" (Excel omite el signo aquí).
  expect(calc('=TEXT(-0.4,"0")')).toBe('0');
  expect(calc('=TEXT(-0.001,"0.0")')).toBe('0.0');
  expect(calc('=TEXT(-0.004,"0.00")')).toBe('0.00');
  // pero un valor negativo real conserva el signo:
  expect(calc('=TEXT(-5,"0.00")')).toBe('-5.00');
  expect(calc('=TEXT(-0.25,"0%")')).toBe('-25%');
});

test('TEXT — secciones positiva;negativa;cero', () => {
  expect(calc('=TEXT(-5,"0.0;(0.0)")')).toBe('(5.0)');
  expect(calc('=TEXT(0,"0.0;(0.0);0.000")')).toBe('0.000'); // sección de cero
  expect(calc('=TEXT(7,"$0.00;-$0.00")')).toBe('$7.00');
  expect(calc('=TEXT(-7,"$0.00;($0.00)")')).toBe('($7.00)');
});

test('TEXT — «#» omite ceros no significativos', () => {
  expect(calc('=TEXT(0.5,"#.##")')).toBe('.5');
  expect(calc('=TEXT(12,"#,##0.##")')).toBe('12');
  expect(calc('=TEXT(0,"General")')).toBe('0');
});

test('endurecimiento: coacción y solapes', () => {
  expect(calc('=EXACT("1",1)')).toBe(true); // 1 → "1"
  expect(calc('=SUBSTITUTE("aaa","a","aa")')).toBe('aaaaaa'); // sin re-escaneo
  expect(calc('=SEARCH("~?","ab?c")')).toBe(3); // «~?» = interrogante literal
  expect(calc('=SEARCH("~*","a*b")')).toBe(2);
  expect(calc('=SUBSTITUTE("a.b.c",".","-",2)')).toBe('a.b-c');
  expect(show('=FIND("a","")')).toBe('#VALUE!'); // buscar en cadena vacía
});
