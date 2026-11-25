/**
 * La tabla de una nota: cabeceras, filas numeradas, fondo, alineación y Tab. El HTML es
 * el compartido con Fyne y el que deja pasar el saneador (`<thead>`, `th scope`,
 * `data-numerada`, `data-fondo`, `text-align`).
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, beforeAll, expect, test } from 'bun:test';
import {
  alinearColumna,
  alternarCabeceraDeColumna,
  alternarCabeceraDeFila,
  alternarFilasNumeradas,
  celdaVecina,
  combinarAbajo,
  combinarDerecha,
  esCombinada,
  fondoDeCelda,
  separarCelda,
  vecinaAbajo,
  vecinaDerecha,
  tieneCabeceraDeColumna,
  tieneCabeceraDeFila,
} from './tabla-notas.js';

let registrado = false;
beforeAll(() => {
  if (typeof document === 'undefined') {
    GlobalRegistrator.register();
    registrado = true;
  }
});
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

function tabla(): HTMLTableElement {
  const caja = document.createElement('div');
  caja.innerHTML =
    '<table><tbody><tr><td>a</td><td>b</td></tr><tr><td>c</td><td>d</td></tr></tbody></table>';
  return caja.querySelector('table')!;
}

test('la fila de cabecera es un <thead> de <th>, y se quita igual', () => {
  const t = tabla();
  alternarCabeceraDeFila(t);
  expect(tieneCabeceraDeFila(t)).toBe(true);
  expect(t.outerHTML).toBe(
    '<table><thead><tr><th>a</th><th>b</th></tr></thead>' +
      '<tbody><tr><td>c</td><td>d</td></tr></tbody></table>',
  );
  alternarCabeceraDeFila(t);
  expect(tieneCabeceraDeFila(t)).toBe(false);
  expect(t.outerHTML).toBe(
    '<table><tbody><tr><td>a</td><td>b</td></tr><tr><td>c</td><td>d</td></tr></tbody></table>',
  );
});

test('la columna de cabecera son <th scope="row"> en el cuerpo, no en la cabecera', () => {
  const t = tabla();
  alternarCabeceraDeFila(t);
  alternarCabeceraDeColumna(t);
  expect(tieneCabeceraDeColumna(t)).toBe(true);
  expect(t.querySelector('tbody')!.innerHTML).toBe('<tr><th scope="row">c</th><td>d</td></tr>');
  expect(t.querySelector('thead')!.innerHTML).toContain('<th>a</th>');
  alternarCabeceraDeColumna(t);
  expect(t.querySelector('tbody')!.innerHTML).toBe('<tr><td>c</td><td>d</td></tr>');
});

test('filas numeradas, fondo con nombre de token y alineación de la columna', () => {
  const t = tabla();
  alternarFilasNumeradas(t);
  expect(t.outerHTML.startsWith('<table data-numerada="">')).toBe(true);
  alternarFilasNumeradas(t);
  expect(t.hasAttribute('data-numerada')).toBe(false);
  const celdas = [...t.querySelectorAll('td')];
  const b = celdas[1]!;
  fondoDeCelda(b, 'azul');
  expect(b.getAttribute('data-fondo')).toBe('azul');
  fondoDeCelda(b, null);
  expect(b.hasAttribute('data-fondo')).toBe(false);
  alinearColumna(b, 'center');
  expect(celdas[3]!.getAttribute('style')).toBe('text-align: center;');
  alinearColumna(b, 'left');
  expect(celdas[3]!.hasAttribute('style')).toBe(false);
});

test('Tab va de celda en celda en orden de lectura; en los extremos, nada', () => {
  const t = tabla();
  const [a, b, c, d] = [...t.querySelectorAll('td')];
  expect(celdaVecina(a!, 1)).toBe(b!);
  expect(celdaVecina(b!, 1)).toBe(c!);
  expect(celdaVecina(c!, -1)).toBe(b!);
  expect(celdaVecina(d!, 1)).toBeNull();
  expect(celdaVecina(a!, -1)).toBeNull();
});

function rejilla(html: string): HTMLTableElement {
  const caja = document.createElement('div');
  caja.innerHTML = html;
  return caja.querySelector('table')!;
}
const celdas = (t: HTMLTableElement) => [...t.querySelectorAll<HTMLTableCellElement>('td, th')];

test('combinar a la derecha y abajo, y separar, como Fyne (combinar.go)', () => {
  const t = rejilla(
    '<table><tbody><tr><td>a</td><td>b</td></tr><tr><td>c</td><td>d</td></tr></tbody></table>',
  );
  const [a] = celdas(t);
  expect(vecinaDerecha(a!)).toBe(celdas(t)[1]!);
  expect(vecinaDerecha(celdas(t)[1]!)).toBeNull();
  combinarDerecha(a!);
  expect(t.innerHTML).toBe(
    '<tbody><tr><td colspan="2">ab</td></tr><tr><td>c</td><td>d</td></tr></tbody>',
  );
  expect(vecinaAbajo(a!)).toBeNull(); // una de dos columnas no se junta con una de una
  separarCelda(a!);
  expect(t.innerHTML).toBe(
    '<tbody><tr><td>ab</td><td><br></td></tr><tr><td>c</td><td>d</td></tr></tbody>',
  );
  combinarAbajo(a!);
  expect(t.innerHTML).toBe(
    '<tbody><tr><td rowspan="2">abc</td><td><br></td></tr><tr><td>d</td></tr></tbody>',
  );
  expect(esCombinada(a!)).toBe(true);
  separarCelda(a!);
  expect(t.innerHTML).toBe(
    '<tbody><tr><td>abc</td><td><br></td></tr><tr><td><br></td><td>d</td></tr></tbody>',
  );
});

test('no se combina entre secciones ni pasando el tope del saneador', () => {
  const t = rejilla(
    '<table><thead><tr><th>A</th></tr></thead><tbody><tr><td>1</td></tr></tbody></table>',
  );
  expect(vecinaAbajo(celdas(t)[0]!)).toBeNull();
  const ancha = rejilla(
    '<table><tbody><tr><td colspan="19">a</td><td colspan="2">b</td></tr></tbody></table>',
  );
  expect(vecinaDerecha(celdas(ancha)[0]!)).toBeNull();
});
