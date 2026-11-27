/**
 * El color del texto y el resaltado tienen que llegar al servicio en `#rrggbb`, que es
 * lo único que su saneador deja pasar; el navegador los escribe en `rgb()`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, beforeAll, expect, test } from 'bun:test';
import { coloresEnHex, normalizarSalida } from './salida.js';

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

/** Lo que tendría el editor en el DOM con ese HTML. */
function editor(html: string): HTMLElement {
  const campo = document.createElement('div');
  campo.innerHTML = html;
  return campo;
}

test('rgb() y rgba() opaco pasan a hexadecimal; con transparencia se quedan', () => {
  expect(coloresEnHex('color: rgb(220, 38, 38);')).toBe('color: #dc2626;');
  expect(coloresEnHex('background-color: rgba(255,243,163,1)')).toBe('background-color: #fff3a3');
  expect(coloresEnHex('color: rgba(0, 0, 0, 0.5)')).toBe('color: rgba(0, 0, 0, 0.5)');
  expect(coloresEnHex('text-align: center')).toBe('text-align: center');
});

test('el HTML entregado lleva los colores en hexadecimal y el texto intacto', () => {
  const html =
    '<p><span style="color: rgb(220, 38, 38);">rojo</span> y rgb(1, 2, 3) escrito</p>' +
    '<p style="text-align: center;"><span style="background-color: rgb(255, 243, 163);">x</span></p>';
  expect(normalizarSalida(editor(html))).toBe(
    '<p><span style="color: #dc2626;">rojo</span> y rgb(1, 2, 3) escrito</p>' +
      '<p style="text-align: center;"><span style="background-color: #fff3a3;">x</span></p>',
  );
});

test('sin colores, el HTML sale tal cual (sin pasar por el analizador)', () => {
  const html = '<p>rgb(1, 2, 3) <b>escrito</b></p>';
  expect(normalizarSalida(editor(html))).toBe(html);
});

test('los espacios de ancho cero del Markdown en la línea no se guardan', () => {
  expect(normalizarSalida(editor('<p><b>sí</b>\u200b y no</p>'))).toBe('<p><b>sí</b> y no</p>');
});

test('se trabaja sobre una copia: el DOM del editor no cambia', () => {
  const html = '<p><span style="color: rgb(220, 38, 38);">rojo</span>\u200b</p>';
  const campo = editor(html);
  const antes = campo.innerHTML;
  expect(normalizarSalida(campo)).toBe('<p><span style="color: #dc2626;">rojo</span></p>');
  expect(campo.innerHTML).toBe(antes);
});
