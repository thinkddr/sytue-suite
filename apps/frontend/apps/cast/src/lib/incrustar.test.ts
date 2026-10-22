import { expect, test } from 'bun:test';
import { codigoDeInsercion, medidas } from './incrustar.js';

test('el iframe lleva la proporción del vídeo y 16:9 si no se sabe', () => {
  expect(medidas(1920, 1080)).toEqual({ w: 640, h: 360 });
  expect(medidas(1080, 1920)).toEqual({ w: 640, h: 1138 });
  expect(medidas(null, null)).toEqual({ w: 640, h: 360 });
});

test('un título con comillas no rompe el atributo (ni cuela HTML)', () => {
  const html = codigoDeInsercion(
    'https://cast.sytue.eu/cast/embed/1',
    'Un "gran" <día>',
    1280,
    720,
  );
  expect(html).toContain('title="Un &quot;gran&quot; &lt;día&gt;"');
  expect(html).toContain('src="https://cast.sytue.eu/cast/embed/1"');
  expect(html).toContain('allowfullscreen');
  expect(html).not.toContain('<día>');
});
