import { expect, test } from 'bun:test';
import { mencionaAdjunto, sinCitas } from './adjuntos.js';

test('habla de un adjunto en los cinco idiomas', () => {
  for (const frase of [
    'Te adjunto el informe de septiembre.',
    'Va adjunta la factura.',
    'Please find attached the report.',
    "I'm attaching the invoice.",
    'See the attachment below.',
    'Vous trouverez le rapport en pièce jointe.',
    'Ci-joint le devis.',
    'Im Anhang findest du den Bericht.',
    'Anbei die Rechnung.',
    'In allegato trovi il rapporto.',
    'Allego la fattura.',
  ])
    expect(`${frase} → ${mencionaAdjunto(frase)}`).toBe(`${frase} → true`);
});

test('no salta con texto corriente', () => {
  for (const frase of [
    'Nos vemos el jueves a las diez.',
    'The meeting is at ten.',
    'Merci pour votre message.',
    'Danke für die Nachricht.',
    'Grazie per il messaggio.',
    'La casa está junto al río.',
  ])
    expect(`${frase} → ${mencionaAdjunto(frase)}`).toBe(`${frase} → false`);
});

test('lo citado de la respuesta no cuenta', () => {
  const html = '<p>Gracias.</p><blockquote><p>Te adjunto el informe.</p></blockquote>';
  expect(mencionaAdjunto(sinCitas(html))).toBe(false);
  expect(mencionaAdjunto(sinCitas('<p>Te adjunto el informe.</p>'))).toBe(true);
});
