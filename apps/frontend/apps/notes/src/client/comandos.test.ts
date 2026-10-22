/**
 * El menú de «/» que se abre donde no debe se come la barra de una URL o de una
 * fecha; uno que no se abre es un comando que no existe. Los negativos importan.
 */
import { expect, test } from 'bun:test';
import { COMANDOS, consultaDeBarra, filtrarComandos } from './comandos.js';
import { en } from '../i18n/messages/en.js';
import { es } from '../i18n/messages/es.js';

test('la barra abre el menú al principio de la línea o detrás de un blanco', () => {
  expect(consultaDeBarra('/')).toBe('');
  expect(consultaDeBarra('/tit')).toBe('tit');
  expect(consultaDeBarra('Hola /h1')).toBe('h1');
  expect(consultaDeBarra('Hola\u00a0/')).toBe('');
});

test('la barra dentro de una palabra, de una URL o tras un blanco escrito no abre nada', () => {
  expect(consultaDeBarra('a/b')).toBeNull();
  expect(consultaDeBarra('https://sytue.eu/')).toBeNull();
  expect(consultaDeBarra('/titulo ')).toBeNull();
  expect(consultaDeBarra('/a/b')).toBeNull();
  expect(consultaDeBarra('/' + 'x'.repeat(21))).toBeNull();
  expect(consultaDeBarra('sin barra')).toBeNull();
});

test('sin consulta salen todos, en el orden de la lista', () => {
  expect(filtrarComandos('', (c) => String(en[c.clave])).map((c) => c.id)).toEqual(
    COMANDOS.map((c) => c.id),
  );
});

test('se filtra por el nombre en el idioma de la app, sin acentos', () => {
  const ids = filtrarComandos('titulo', (c) => String(es[c.clave])).map((c) => c.id);
  expect(ids).toEqual(['h1', 'h2', 'h3']);
  expect(filtrarComandos('cita', (c) => String(es[c.clave])).map((c) => c.id)).toEqual(['cita']);
});

test('los alias valen en cualquier idioma', () => {
  expect(filtrarComandos('h2', (c) => String(es[c.clave])).map((c) => c.id)).toEqual(['h2']);
  expect(filtrarComandos('todo', (c) => String(es[c.clave])).map((c) => c.id)).toEqual(['tarea']);
  expect(filtrarComandos('---', (c) => String(es[c.clave])).map((c) => c.id)).toEqual(['regla']);
  expect(filtrarComandos('zzz', (c) => String(es[c.clave]))).toEqual([]);
});
