import { expect, test } from 'bun:test';
import { CoEdicion } from './coedicion.js';
import { parseCursorPos } from './cursors.js';

/** El texto visible de un HTML sencillo, que es lo que mediría `textContent` del folio. */
const visible = (html: string): string => html.replace(/<[^>]+>/g, '');

test('un cursor anclado a los caracteres se recoloca tras ediciones ajenas, con etiquetas de por medio', () => {
  const inicial = '<p>Hola <b>mundo</b></p>';
  // Ada abre el documento (la secuencia nace del HTML) y Bea recibe la misma.
  const ada = new CoEdicion('ada', [], '');
  const ops = ada.opsLocales(inicial, 0, true);
  const bea = new CoEdicion('bea', [], '');
  bea.opsLocales('', 0, true);
  bea.recibir(ops);
  bea.fusionar();

  // Bea tiene el cursor entre «Hol» y «a» (offset 3 del texto «Hola mundo»).
  const dots = bea.puntosDeOffsets(visible(inicial).length, { anchor: 3, focus: 3 });
  expect(dots).not.toBeNull();

  // Ada escribe delante «Bien: » y Bea lo recibe: su folio pasa a ser el nuevo.
  const nuevo = '<p>Bien: Hola <b>mundo</b></p>';
  bea.recibir(ada.opsLocales(nuevo, 100, true));
  bea.fusionar();
  // Quien recibe el cursor de Bea (Ada) lo coloca ahora tras «Bien: Hol».
  expect(ada.offsetsDePuntos(visible(nuevo).length, dots!)).toEqual({ anchor: 9, focus: 9 });
  // Si el folio no coincide con la secuencia (acaba de teclear), no se ancla a ciegas.
  expect(ada.offsetsDePuntos(visible(nuevo).length + 1, dots!)).toBeNull();
  expect(ada.puntosDeOffsets(5, { anchor: 1, focus: 1 })).toBeNull();
});

test('parseCursorPos: los puntos son opcionales y van por parejas', () => {
  const base = { kind: 'doc' as const, anchor: 1, focus: 2 };
  const a = { a: 'x', c: 3 };
  expect(parseCursorPos({ ...base, anchorDot: a, focusDot: a })).toEqual({
    ...base,
    anchorDot: a,
    focusDot: a,
  });
  expect(parseCursorPos({ ...base, anchorDot: a })).toEqual(base);
  expect(parseCursorPos({ ...base, anchorDot: { a: 'x' }, focusDot: a })).toEqual(base);
  expect(parseCursorPos(base)).toEqual(base);
});
