import { expect, test } from 'bun:test';
import { mencionaAdjunto, sinCitas } from './adjuntos.js';
import VECTORES from './adjuntos.vectores.json';

// Las frases son las de `adjuntos.vectores.json`, que también recorre Fyne.
test('habla de un adjunto en los 26 idiomas', () => {
  for (const frase of VECTORES.si)
    expect(`${frase} → ${mencionaAdjunto(frase)}`).toBe(`${frase} → true`);
});

test('no salta con texto corriente', () => {
  for (const frase of VECTORES.no)
    expect(`${frase} → ${mencionaAdjunto(frase)}`).toBe(`${frase} → false`);
});

test('lo citado de la respuesta no cuenta', () => {
  const html = '<p>Gracias.</p><blockquote><p>Te adjunto el informe.</p></blockquote>';
  expect(mencionaAdjunto(sinCitas(html))).toBe(false);
  expect(mencionaAdjunto(sinCitas('<p>Te adjunto el informe.</p>'))).toBe(true);
});
