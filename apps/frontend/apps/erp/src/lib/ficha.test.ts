/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * UN ALTA NO HEREDA LO QUE HABÍA EN LA FICHA ABIERTA. El formulario de la ficha toma sus
 * valores al montarse; sin una `key` que cambie con el registro, «Nuevo» con una ficha
 * abierta salía relleno con los datos de ese registro —y guardaba una copia—, y abrir
 * otra fila dejaba en ella los valores de la anterior, que al guardar la sobrescribían.
 *
 * Visto en ROJO: sin `key` en `<Ficha>` cae el último test; y con una clave que no
 * distinguiera el alta del registro abierto, el primero.
 */
import { expect, test } from 'bun:test';
import { claveDeFicha } from './ficha';

const a = { id: 'contacto-1' };
const b = { id: 'contacto-2' };

test('un alta en blanco no es la ficha del registro que estaba abierto', () => {
  expect(claveDeFicha('contacto', 'nueva', null)).not.toBe(claveDeFicha('contacto', a, null));
});

test('cada registro es una ficha distinta', () => {
  expect(claveDeFicha('contacto', a, null)).not.toBe(claveDeFicha('contacto', b, null));
  expect(claveDeFicha('contacto', a, null)).toBe(
    claveDeFicha('contacto', { id: 'contacto-1' }, null),
  );
});

test('duplicar es otra ficha: ni la del original ni la de un alta en blanco', () => {
  const copia = claveDeFicha('contacto', 'nueva', a);
  expect(copia).not.toBe(claveDeFicha('contacto', a, null));
  expect(copia).not.toBe(claveDeFicha('contacto', 'nueva', null));
  expect(copia).not.toBe(claveDeFicha('contacto', 'nueva', b));
});

test('el mismo identificador en otro modelo es otra ficha', () => {
  expect(claveDeFicha('contacto', a, null)).not.toBe(claveDeFicha('empresa', a, null));
});

test('la ficha se monta con esa clave: sin ella su formulario sobrevive al cambio de registro', async () => {
  const fuente = await Bun.file(new URL('../components/ErpApp.tsx', import.meta.url)).text();
  const fichas = [...fuente.matchAll(/<Ficha\s([^>]*?)modelo=\{modelo\}/g)];
  expect(fichas.length).toBe(1);
  expect(fichas[0]![1]).toContain('key={claveDeFicha(modelo.clave, ficha, semilla)}');
});

test('la ficha móvil encaja entre ambos insets y Atrás usa su mismo cierre', async () => {
  const fuente = await Bun.file(new URL('../components/ErpApp.tsx', import.meta.url)).text();
  const ficha = fuente.slice(fuente.indexOf('function Ficha('), fuente.indexOf('function Paleta('));
  expect(ficha).toContain('useMobileBack(true, onCerrar, { nivel: 2 })');
  expect(ficha).toContain('sytue-side-panel');
  // Con left/right seguros, w-full desborda tanto como la muesca del lado izquierdo.
  expect(ficha).toContain('max-md:w-auto');
  const paleta = fuente.slice(fuente.indexOf('function Paleta('));
  // La paleta delega en PaletaDeComandos (@sytue/ui), que gestiona useMobileBack(open, onClose, { nivel: 2 }).
  expect(paleta).toContain('<PaletaDeComandos');
  expect(paleta).toContain('onClose={onCerrar}');
});
