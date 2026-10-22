/**
 * La pregunta que escribe una persona es DATO, no orden.
 *
 * Si acabara pegada dentro de `instruction`, una pregunta escrita a mano podría
 * reescribir las reglas de arriba —empezando por la de no identificar a nadie—,
 * y esa regla no puede depender de lo que se teclee en un campo de texto. Es un
 * fallo que no da error: el endpoint seguiría contestando, solo que obedeciendo
 * a otro. Por eso se vigila aquí y no en una revisión.
 */
import { expect, test } from 'bun:test';

const ruta = new URL('./preguntar.ts', import.meta.url).pathname;
const rutaComun = new URL('../../lib/visionIA.ts', import.meta.url).pathname;
const fuente = await Bun.file(ruta).text();
const comun = await Bun.file(rutaComun).text();

test('la pregunta viaja como contexto y la instrucción es una constante', () => {
  expect(fuente).toContain('context: texto');
  // La instrucción se arma con un array de literales y un `join`: sin
  // interpolación no hay forma de que entre nada de fuera.
  const bloque = fuente.slice(fuente.indexOf('const INSTRUCCION'), fuente.indexOf('].join'));
  expect(bloque).not.toContain('${');
  expect(fuente).toContain('instruction: INSTRUCCION');
});

test('la instrucción prohíbe identificar personas y adivinar', () => {
  expect(fuente).toContain('Do not name or identify people');
  expect(fuente).toContain('instead of guessing');
});

test('una pregunta vacía o kilométrica no llega a gastar una llamada', () => {
  // El orden importa: las dos comprobaciones van ANTES de pedir la miniatura,
  // que es lo que cuesta dinero y tiempo.
  const vacia = fuente.indexOf("'missing_question'");
  const larga = fuente.indexOf("'question_too_long'");
  // La LLAMADA, no el import: el import está arriba del todo y haría pasar
  // este test siempre... al revés.
  const imagen = fuente.indexOf('await miniaturaEnBase64(');
  expect(vacia).toBeGreaterThan(0);
  expect(larga).toBeGreaterThan(0);
  expect(imagen).toBeGreaterThan(larga);
  expect(imagen).toBeGreaterThan(vacia);
});

test('la respuesta no se guarda en ninguna parte', () => {
  // Describir sí escribe en la foto; preguntar, no. Que no aparezca ninguna
  // llamada de guardado es la comprobación entera.
  expect(fuente).not.toContain('guardarDescripcion');
  expect(fuente).not.toContain('PUT');
});

test('el tipo de imagen va con el nombre del contrato, en snake', () => {
  // `imagenTipo` en camel se ignora SIN dar error y la foto viaja etiquetada
  // como PNG siendo JPEG. Ya pasó una vez en describir; desde que las dos rutas
  // comparten el helper, volvería a pasar en las dos a la vez.
  expect(comun).toContain("imagen_tipo: 'image/jpeg'");
  // Como CLAVE, no como palabra: el comentario de al lado nombra el error que
  // se está evitando, y buscar la palabra suelta daría un fallo por citarlo.
  expect(comun).not.toMatch(/imagenTipo\s*:/);
});
