// Checks de agregados condicionales y parser de criterios. `bun test`.
// Se prueban VALORES conocidos (los que daría una hoja real) a través del motor.
import { expect, test } from 'bun:test';
import { Engine } from '../engine.js';

/** Motor con un grid numérico A/B paralelo y una columna de texto C. */
function grid(): Engine {
  const e = new Engine();
  // A1:A5 = 1..5 ; B1:B5 = 10,20,30,40,50 (paralelos)
  const a = [1, 2, 3, 4, 5];
  const b = [10, 20, 30, 40, 50];
  for (let i = 0; i < 5; i++) {
    e.setCell('Hoja1', 'A' + (i + 1), String(a[i]));
    e.setCell('Hoja1', 'B' + (i + 1), String(b[i]));
  }
  // C1:C6 texto (C5 en blanco), con mayúsculas mezcladas
  e.setCell('Hoja1', 'C1', 'apple');
  e.setCell('Hoja1', 'C2', 'apricot');
  e.setCell('Hoja1', 'C3', 'banana');
  e.setCell('Hoja1', 'C4', 'Apple');
  // C5 sin poner → blanco
  e.setCell('Hoja1', 'C6', 'cherry');
  return e;
}

function val(e: Engine, formula: string): unknown {
  e.setCell('Hoja1', 'Z1', formula);
  return e.getValue('Hoja1', 'Z1');
}
function disp(e: Engine, formula: string): string {
  e.setCell('Hoja1', 'Z1', formula);
  return e.getDisplay('Hoja1', 'Z1');
}

test('COUNTIF: comparadores numéricos', () => {
  const e = grid();
  expect(val(e, '=COUNTIF(A1:A5,">2")')).toBe(3);
  expect(val(e, '=COUNTIF(A1:A5,">=3")')).toBe(3);
  expect(val(e, '=COUNTIF(A1:A5,"<3")')).toBe(2);
  expect(val(e, '=COUNTIF(A1:A5,"<=2")')).toBe(2);
  expect(val(e, '=COUNTIF(A1:A5,"<>3")')).toBe(4);
  expect(val(e, '=COUNTIF(A1:A5,3)')).toBe(1);
  expect(val(e, '=COUNTIF(A1:A5,"3")')).toBe(1);
  expect(val(e, '=COUNTIF(A1:A5,"=4")')).toBe(1);
});

test('COUNTIF: negativos y porcentaje en el criterio', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '-3');
  e.setCell('Hoja1', 'A2', '-1');
  e.setCell('Hoja1', 'A3', '0.6');
  e.setCell('Hoja1', 'A4', '0.3');
  expect(val(e, '=COUNTIF(A1:A4,"<-2")')).toBe(1); // solo -3
  expect(val(e, '=COUNTIF(A1:A4,"<=-1")')).toBe(2); // -3, -1
  expect(val(e, '=COUNTIF(A1:A4,">50%")')).toBe(1); // 0.6 > 0.5
});

test('COUNTIF: texto exacto (sin distinguir caja) y comodines', () => {
  const e = grid();
  expect(val(e, '=COUNTIF(C1:C6,"apple")')).toBe(2); // apple + Apple
  expect(val(e, '=COUNTIF(C1:C6,"APPLE")')).toBe(2);
  expect(val(e, '=COUNTIF(C1:C6,"ap*")')).toBe(3); // apple, apricot, Apple
  expect(val(e, '=COUNTIF(C1:C6,"a?ple")')).toBe(2); // apple, Apple
  expect(val(e, '=COUNTIF(C1:C6,"*a*")')).toBe(4); // apple, apricot, banana, Apple
  expect(val(e, '=COUNTIF(C1:C6,"*")')).toBe(5); // solo texto, no el blanco
  expect(val(e, '=COUNTIF(C1:C6,"<>apple")')).toBe(3); // apricot, banana, cherry
});

test('COUNTIF: orden textual (<,>,>=,<=) segrega por tipo', () => {
  // Excel solo compara «<texto»/«>texto» contra celdas de texto: un número no es
  // «menor que texto» ni un booleano «mayor que texto» (pese al orden núm<texto<bool).
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '5'); // número
  e.setCell('Hoja1', 'A2', 'apple'); // texto < banana
  e.setCell('Hoja1', 'A3', 'zebra'); // texto > banana
  e.setCell('Hoja1', 'A4', 'TRUE'); // booleano
  expect(val(e, '=COUNTIF(A1:A4,"<banana")')).toBe(1); // solo "apple" (NO el número 5)
  expect(val(e, '=COUNTIF(A1:A4,">banana")')).toBe(1); // solo "zebra" (NO el booleano TRUE)
  expect(val(e, '=COUNTIF(A1:A4,">=apple")')).toBe(2); // "apple" y "zebra"
  expect(val(e, '=COUNTIF(A1:A4,"<=zebra")')).toBe(2); // "apple" y "zebra"
});

test('COUNTIF: comodín escapado con ~', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'D1', 'a*b');
  e.setCell('Hoja1', 'D2', 'acb');
  e.setCell('Hoja1', 'D3', 'a*b');
  expect(val(e, '=COUNTIF(D1:D3,"a~*b")')).toBe(2); // literal "a*b": D1, D3
  expect(val(e, '=COUNTIF(D1:D3,"a*b")')).toBe(3); // comodín: los tres
  expect(val(e, '=COUNTIF(D1:D3,"a?b")')).toBe(3); // un carácter: los tres
});

test('COUNTIF: blanco, tipos mixtos y booleanos', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'F1', '1');
  e.setCell('Hoja1', 'F2', 'x');
  // F3 en blanco
  e.setCell('Hoja1', 'F4', '3');
  e.setCell('Hoja1', 'F5', 'TRUE');
  expect(val(e, '=COUNTIF(F1:F5,"")')).toBe(1); // solo el blanco
  expect(val(e, '=COUNTIF(F1:F5,"<>")')).toBe(4); // los no-blancos
  expect(val(e, '=COUNTIF(F1:F5,">0")')).toBe(2); // 1 y 3 (no texto, no bool, no blanco)
  expect(val(e, '=COUNTIF(F1:F5,"<>x")')).toBe(3); // 1, 3, TRUE (no blanco)
  expect(val(e, '=COUNTIF(F1:F5,"*")')).toBe(1); // solo el texto "x"
  expect(val(e, '=COUNTIF(F1:F5,TRUE)')).toBe(1); // criterio booleano directo
  expect(val(e, '=COUNTIF(F1:F5,"TRUE")')).toBe(1); // criterio "TRUE" como texto→bool
});

test('COUNTIF: criterio desde referencia de celda', () => {
  const e = grid();
  e.setCell('Hoja1', 'E1', '>2');
  e.setCell('Hoja1', 'E2', '3');
  expect(val(e, '=COUNTIF(A1:A5,E1)')).toBe(3);
  expect(val(e, '=COUNTIF(A1:A5,E2)')).toBe(1);
});

test('SUMIF: con y sin rango de suma, y realineado del rango de suma', () => {
  const e = grid();
  expect(val(e, '=SUMIF(A1:A5,">2")')).toBe(12); // 3+4+5
  expect(val(e, '=SUMIF(A1:A5,">2",B1:B5)')).toBe(120); // 30+40+50
  // realineado: solo B1 → Excel usa B1:B5
  expect(val(e, '=SUMIF(A1:A5,">2",B1)')).toBe(120);
  expect(val(e, '=SUMIF(A1:A5,"<>3",B1:B5)')).toBe(120); // 10+20+40+50
});

test('SUMIFS: criterios múltiples y tamaños distintos → #VALUE!', () => {
  const e = grid();
  expect(val(e, '=SUMIFS(B1:B5,A1:A5,">1",A1:A5,"<5")')).toBe(90); // B2+B3+B4
  expect(disp(e, '=SUMIFS(B1:B5,A1:A4,">1")')).toBe('#VALUE!'); // rangos desiguales
  expect(disp(e, '=SUMIFS(B1:B5,A1:A5)')).toBe('#VALUE!'); // sin par de criterio
});

test('COUNTIFS: dos columnas de criterio', () => {
  const e = grid();
  expect(val(e, '=COUNTIFS(A1:A5,">1",A1:A5,"<5")')).toBe(3); // 2,3,4
  expect(val(e, '=COUNTIFS(A1:A5,">=2",B1:B5,"<50")')).toBe(3); // filas 2,3,4
});

test('AVERAGEIF / AVERAGEIFS y #DIV/0! sin coincidencias', () => {
  const e = grid();
  expect(val(e, '=AVERAGEIF(A1:A5,">2")')).toBe(4); // (3+4+5)/3
  expect(val(e, '=AVERAGEIF(A1:A5,">2",B1:B5)')).toBe(40); // (30+40+50)/3
  expect(disp(e, '=AVERAGEIF(A1:A5,">100")')).toBe('#DIV/0!');
  expect(val(e, '=AVERAGEIFS(B1:B5,A1:A5,">1",A1:A5,"<5")')).toBe(30); // 90/3
  expect(disp(e, '=AVERAGEIFS(B1:B5,A1:A5,">100")')).toBe('#DIV/0!');
});

test('MAXIFS / MINIFS incluido el caso sin coincidencias → 0', () => {
  const e = grid();
  expect(val(e, '=MAXIFS(B1:B5,A1:A5,">2")')).toBe(50);
  expect(val(e, '=MINIFS(B1:B5,A1:A5,">2")')).toBe(30);
  expect(val(e, '=MAXIFS(B1:B5,A1:A5,">100")')).toBe(0); // Excel: 0 sin coincidencias
  expect(val(e, '=MINIFS(B1:B5,A1:A5,">100")')).toBe(0);
});

test('propagación de errores: criterio y rango de valores', () => {
  const e = grid();
  // criterio que es un error → se propaga
  expect(disp(e, '=COUNTIF(A1:A5,1/0)')).toBe('#DIV/0!');
  // error en una celda del rango de valores QUE COINCIDE → se propaga
  e.setCell('Hoja1', 'B3', '=1/0'); // fila A3=3 coincide con ">2"
  expect(disp(e, '=SUMIF(A1:A5,">2",B1:B5)')).toBe('#DIV/0!');
});

test('error en el rango de CRITERIOS no se propaga (celda no cuenta)', () => {
  const e = grid();
  e.setCell('Hoja1', 'A3', '=1/0'); // antes 3; ahora error
  // A3 error no coincide con ">2"; el resto (A4=4,A5=5) sí
  expect(val(e, '=SUMIF(A1:A5,">2")')).toBe(9); // 4+5, sin error
  expect(val(e, '=COUNTIF(A1:A5,">2")')).toBe(2);
});

test('errores de aridad → #VALUE!', () => {
  const e = grid();
  expect(disp(e, '=COUNTIF(A1:A5)')).toBe('#VALUE!');
  expect(disp(e, '=SUMIFS(B1:B5,A1:A5)')).toBe('#VALUE!');
  expect(disp(e, '=MAXIFS(B1:B5)')).toBe('#VALUE!');
});
