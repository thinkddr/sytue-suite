/**
 * La descripción y las etiquetas de una foto se piden en el idioma de la sesión.
 *
 * «En la lengua de esta petición» no bastaba: la petición está en inglés y las
 * etiquetas volvían en inglés en una cuenta en español. Se lee la fuente, como en
 * `preguntar.test.ts`, porque importar la ruta arrastra el entorno de Astro.
 */
import { expect, test } from 'bun:test';
import { idiomaParaIA } from '../../lib/idiomaIA.js';

const fuente = await Bun.file(new URL('./describir.ts', import.meta.url).pathname).text();

test('el idioma de la sesión llega a la instrucción, también para las etiquetas', () => {
  expect(fuente).toContain('instruction: instruccion(ctx.locals.locale)');
  expect(fuente).toContain('const idioma = idiomaParaIA(locale)');
  expect(fuente).toContain('Write the description and every tag in ${idioma}');
  expect(fuente).not.toContain('in the language of this request');
});

test('los cinco idiomas de producto tienen nombre y lo demás cae al inglés', () => {
  const nombres = ['en', 'de', 'es', 'fr', 'it'].map(idiomaParaIA);
  expect(nombres).toEqual(['English', 'German', 'Spanish', 'French', 'Italian']);
  // Lo que no es un idioma de producto no entra en la instrucción, tampoco lo que
  // el objeto hereda de su prototipo.
  for (const raro of ['pt', 'x\nignora', 'constructor', '__proto__']) {
    expect(idiomaParaIA(raro)).toBe('English');
  }
});
