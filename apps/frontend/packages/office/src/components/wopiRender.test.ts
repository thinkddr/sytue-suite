// Checks del render de la previsualización WOPI. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import type { SheetModel, SlidesModel } from '../editor/types';
import { hojaAHtml, indiceAColumna, modeloAHtml, rangoUsado, refACelda } from './wopiRender';

test('las referencias de más de una letra no se leen mal', () => {
  // 'AA' es 27, no 1+1: es donde se rompe una conversión escrita a ojo, y una
  // hoja con columnas más allá de la Z saldría con los datos en el sitio de otro.
  expect(refACelda('AA10')).toEqual({ col: 27, fila: 10 });
  expect(refACelda('B12')).toEqual({ col: 2, fila: 12 });
  expect(indiceAColumna(27)).toBe('AA');
  expect(indiceAColumna(52)).toBe('AZ');
  expect(indiceAColumna(53)).toBe('BA');
  // Lo que no es una referencia no cuenta como celda en la esquina inferior.
  expect(refACelda('total')).toBeNull();
  expect(refACelda('A')).toBeNull();
});

test('el rango sale de lo escrito, y se corta antes de congelar la pestaña', () => {
  // El modelo solo guarda celdas NO vacías: si el rango no se dedujera de ellas,
  // habría que elegir entre cortar los datos o pintar una cuadrícula infinita.
  expect(rangoUsado({ A1: 'x', C5: 'y' })).toEqual({ cols: 3, filas: 5 });
  expect(rangoUsado({})).toEqual({ cols: 0, filas: 0 });
  // Una hoja enorme se corta: 100.000 filas no las mira nadie y sí atragantan.
  expect(rangoUsado({ ZZ99999: 'x' })).toEqual({ cols: 40, filas: 200 });
});

test('una hoja cortada lo DICE', () => {
  // Enseñar 200 filas de 100.000 sin avisar es peor que no enseñarlas: quien mira
  // se cree que ese es el fichero entero.
  const grande: SheetModel = { active: 0, sheets: [{ name: 'Datos', cells: { A99999: '1' } }] };
  expect(hojaAHtml(grande, 'CORTADA')).toContain('CORTADA');
  const chica: SheetModel = { active: 0, sheets: [{ name: 'Datos', cells: { A1: '1' } }] };
  expect(hojaAHtml(chica, 'CORTADA')).not.toContain('CORTADA');
});

test('el nombre de la hoja se escapa, el contenido de la celda no', () => {
  const m: SheetModel = {
    active: 0,
    sheets: [{ name: '<script>x</script>', cells: { A1: '<b>hola</b>' } }],
  };
  const html = hojaAHtml(m, '');
  // El nombre lo compone esta función y va escapado.
  expect(html).toContain('&lt;script&gt;');
  // El HTML de la celda ES el modelo (negritas, colores) y va tal cual: se pinta
  // en un iframe con `sandbox` vacío, que le da origen opaco y ningún script.
  expect(html).toContain('<b>hola</b>');
});

test('las diapositivas se colocan en porcentaje del lienzo', () => {
  const m: SlidesModel = {
    slides: [{ id: 's1', boxes: [{ id: 'b1', x: 10, y: 20, w: 30, h: 40, html: 'Título' }] }],
  };
  const html = modeloAHtml('presentation', JSON.stringify(m), '') ?? '';
  expect(html).toContain('left:10%;top:20%;width:30%;height:40%');
  expect(html).toContain('Título');
});

test('un modelo ilegible devuelve null y no una pantalla en blanco', () => {
  // null hace que el visor enseñe «esto todavía no», que es información; un marco
  // vacío parece que ha fallado la red.
  expect(modeloAHtml('spreadsheet', 'no soy json', '')).toBeNull();
  expect(modeloAHtml('vector', '{}', '')).toBeNull();
  // El documento de texto ES html: pasa tal cual.
  expect(modeloAHtml('document', '<p>hola</p>', '')).toBe('<p>hola</p>');
});
