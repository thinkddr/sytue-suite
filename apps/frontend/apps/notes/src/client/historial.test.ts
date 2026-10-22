/**
 * Las dos pilas del deshacer. Lo que se prueba es la disciplina, que es lo que
 * se rompe sin que nadie lo vea: rehacer algo que ya no toca, o perder una
 * acción porque su deshacer falló una vez.
 */
import { expect, test } from 'bun:test';
import { Historial } from '../client/historial.js';

function accion(registro: string[], nombre: string, falla = false) {
  return {
    etiqueta: nombre,
    deshacer: () => {
      if (falla) throw new Error('no');
      registro.push(`undo ${nombre}`);
    },
    rehacer: () => {
      registro.push(`redo ${nombre}`);
    },
  };
}

test('deshacer y rehacer recorren las pilas en orden', async () => {
  const r: string[] = [];
  const h = new Historial();
  h.registrar(accion(r, 'a'));
  h.registrar(accion(r, 'b'));
  expect(h.siguienteDeshacer?.etiqueta).toBe('b');
  expect((await h.deshacer())?.etiqueta).toBe('b');
  expect((await h.deshacer())?.etiqueta).toBe('a');
  expect(await h.deshacer()).toBeUndefined();
  expect((await h.rehacer())?.etiqueta).toBe('a');
  expect(r).toEqual(['undo b', 'undo a', 'redo a']);
});

test('una acción nueva vacía lo que se podía rehacer', async () => {
  const r: string[] = [];
  const h = new Historial();
  h.registrar(accion(r, 'a'));
  await h.deshacer();
  expect(h.siguienteRehacer?.etiqueta).toBe('a');
  h.registrar(accion(r, 'b'));
  expect(h.siguienteRehacer).toBeUndefined();
});

test('si deshacer falla, la acción sigue siendo la siguiente y el error sube', async () => {
  const r: string[] = [];
  const h = new Historial();
  h.registrar(accion(r, 'rota', true));
  await expect(h.deshacer()).rejects.toThrow();
  expect(h.siguienteDeshacer?.etiqueta).toBe('rota');
  expect(h.siguienteRehacer).toBeUndefined();
});

test('el historial tiene tope y olvida lo más viejo', () => {
  const r: string[] = [];
  const h = new Historial(2);
  h.registrar(accion(r, 'a'));
  h.registrar(accion(r, 'b'));
  h.registrar(accion(r, 'c'));
  expect(h.siguienteDeshacer?.etiqueta).toBe('c');
  void h.deshacer();
  void h.deshacer();
  expect(h.siguienteDeshacer).toBeUndefined();
});

test('los guardados de una misma edición son una sola entrada de deshacer', async () => {
  const r: string[] = [];
  const h = new Historial();
  h.registrar(accion(r, 'crear'));
  // Tres autoguardados de la misma sesión: el texto pasó por «ho», «hol», «hola».
  h.registrar(accion(r, 'ho'), 's1');
  h.registrar(accion(r, 'hol'), 's1');
  h.registrar(accion(r, 'hola'), 's1');

  // Un solo deshacer devuelve la nota a antes de escribir, y rehacer trae el
  // texto tal como quedó, no un paso intermedio.
  expect((await h.deshacer())?.etiqueta).toBe('ho');
  expect((await h.rehacer())?.etiqueta).toBe('ho');
  expect(r).toEqual(['undo ho', 'redo hola']);

  // Y debajo sigue estando lo de antes: la edición no se comió la pila.
  expect((await h.deshacer())?.etiqueta).toBe('ho');
  expect((await h.deshacer())?.etiqueta).toBe('crear');
});

test('otra sesión de edición abre una entrada nueva', async () => {
  const r: string[] = [];
  const h = new Historial();
  h.registrar(accion(r, 'a'), 's1');
  h.registrar(accion(r, 'b'), 's2');
  expect((await h.deshacer())?.etiqueta).toBe('b');
  expect((await h.deshacer())?.etiqueta).toBe('a');
});

test('sin sesión no se fusiona nada', async () => {
  const r: string[] = [];
  const h = new Historial();
  h.registrar(accion(r, 'a'));
  h.registrar(accion(r, 'b'));
  expect((await h.deshacer())?.etiqueta).toBe('b');
  expect((await h.deshacer())?.etiqueta).toBe('a');
});
