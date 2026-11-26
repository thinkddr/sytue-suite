/**
 * EL NOMBRE DEL SALUDO NO SE PARTE CON GUION.
 *
 * El título de la portada lleva `hyphens: auto` (`.sytue-h1`), y en el móvil partía el
 * nombre de la persona («Da-niel»). Se vigila que el nombre va en su propio trozo con
 * `hyphens-none`, después de la coma y un espacio por donde sí puede saltar de línea,
 * y que sin nombre queda el saludo solo.
 *
 * Visto en ROJO con el saludo de antes: una sola cadena «Buenas tardes, Daniel» dentro
 * del título, sin nada que proteja el nombre.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, expect, test } from 'bun:test';
import { render } from 'preact';

const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register({ url: 'https://one.sytue.test/' });
const { Saludo } = await import('./Saludo.js');

afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

test('el nombre va entero, sin guion, tras la coma', () => {
  const raiz = document.createElement('h1');
  render(<Saludo texto="Buenas tardes" nombre="Maximiliano" />, raiz);
  const nombre = raiz.querySelector('[data-nombre-saludo]');
  expect(nombre?.textContent).toBe('Maximiliano');
  expect(nombre?.className.split(' ')).toContain('hyphens-none');
  expect(raiz.textContent).toBe('Buenas tardes, Maximiliano');
  render(null, raiz);
});

test('sin nombre, el saludo solo', () => {
  const raiz = document.createElement('h1');
  render(<Saludo texto="Buenos días" />, raiz);
  expect(raiz.querySelector('[data-nombre-saludo]')).toBeNull();
  expect(raiz.textContent).toBe('Buenos días');
  render(null, raiz);
});
