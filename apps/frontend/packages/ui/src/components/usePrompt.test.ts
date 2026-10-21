/**
 * usePrompt — las preguntas heredadas de `window.prompt` traen sus dos puntos
 * («Nombre de la macro:») y hacen de título y de etiqueta del campo, donde no llevan.
 * Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import { sinDosPuntos } from './usePrompt.js';

test('los dos puntos del final se van, con el espacio fino del francés', () => {
  expect(sinDosPuntos('Nombre de la macro:')).toBe('Nombre de la macro');
  expect(sinDosPuntos('Etiqueta de la versión (opcional): ')).toBe(
    'Etiqueta de la versión (opcional)',
  );
  expect(sinDosPuntos('Nom de la macro :')).toBe('Nom de la macro');
  expect(sinDosPuntos('マクロ名：')).toBe('マクロ名');
});

test('lo que no acaba en dos puntos se queda igual', () => {
  expect(sinDosPuntos('Renombrar la hoja')).toBe('Renombrar la hoja');
  expect(sinDosPuntos('Hora 10:30 del lunes')).toBe('Hora 10:30 del lunes');
});
