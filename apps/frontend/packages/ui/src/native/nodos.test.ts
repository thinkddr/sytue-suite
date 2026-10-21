/**
 * `ui.t` marca un rótulo con su clave de catálogo SIN perder el literal, que es el
 * respaldo y lo que sigue leyendo un renderer que no conozca el campo. Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import { ui } from './nodos';
import { todasLasPantallas } from './pantallas';

test('ui.t añade la clave y conserva el literal', () => {
  expect(ui.t('note.new', ui.boton('Nueva nota', 'nueva_nota', 'principal'))).toEqual({
    tipo: 'boton',
    texto: 'Nueva nota',
    accion: 'nueva_nota',
    variante: 'principal',
    clave: 'note.new',
  });
});

test('ui.t rechaza un nodo que no lleva rótulo', () => {
  // Una clave en una lista o en un divisor no la traduciría nadie: que se vea al construir.
  expect(() => ui.t('x', ui.divisor())).toThrow();
  expect(() => ui.t('x', ui.lista('s', []))).toThrow();
});

test('las pantallas declaradas ya no llevan botones ni campos sin clave', () => {
  // «Sincronizar» es la excepción conocida: es un concepto del escritorio y la web no
  // tiene esa cadena en ningún catálogo. Si aparece otra, o se marca con `ui.t` o se
  // añade aquí diciendo por qué.
  const sinClave: string[] = [];
  const recorrer = (n: unknown): void => {
    if (Array.isArray(n)) return n.forEach(recorrer);
    if (typeof n !== 'object' || n === null) return;
    const nodo = n as Record<string, unknown>;
    if ((nodo.tipo === 'boton' || nodo.tipo === 'entrada') && nodo.clave === undefined) {
      sinClave.push(String(nodo.texto ?? nodo.placeholder));
    }
    Object.values(nodo).forEach(recorrer);
  };
  recorrer(todasLasPantallas);
  expect(sinClave).toEqual(['Sincronizar']);
});
