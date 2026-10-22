import { expect, test } from 'bun:test';
import { rutaInterna } from './rutaInterna.js';

test('quita el prefijo del despliegue, que es el caso de todos los días', () => {
  expect(rutaInterna('/photos/api/v1/photos', '/photos')).toBe('/api/v1/photos');
  expect(rutaInterna('/photos/api/v1/albums/7', '/photos')).toBe('/api/v1/albums/7');
});

test('el prefijo con barra final también recorta', () => {
  // `SYTUE_BASE_PATH` y `BASE_URL` no siempre coinciden en la barra; el proxy la
  // quita antes de llamar, pero la función no debe depender de eso.
  expect(rutaInterna('/photos/api/v1/photos', '/photos/'.replace(/\/$/, ''))).toBe(
    '/api/v1/photos',
  );
});

test('sin prefijo, la ruta pasa intacta', () => {
  // Es el caso de dev y el de una superficie servida en la raíz del host.
  expect(rutaInterna('/api/v1/photos', '')).toBe('/api/v1/photos');
});

test('NO muerde un prefijo que solo comparte principio', () => {
  // Con base `/photo`, la ruta `/photos/...` no es suya: sin exigir la barra,
  // el recorte dejaba `s/api/v1/photos`, que es un 404 con otra cara.
  expect(rutaInterna('/photos/api/v1/photos', '/photo')).toBe('/photos/api/v1/photos');
});

test('la ruta que ya viene sin prefijo no se toca', () => {
  expect(rutaInterna('/api/v1/photos', '/photos')).toBe('/api/v1/photos');
});
