import { expect, test } from 'bun:test';
import { MAX_BYTES, cambiar, esVideo, nuevo, progresoTotal, siguiente, tituloDe } from './cola.js';

test('se rechaza ANTES de subir lo que no es vídeo o pasa de 8 GiB', () => {
  expect(nuevo('1', { name: 'a.pdf', size: 10, type: 'application/pdf' }, 'org')).toMatchObject({
    estado: 'fallo',
    motivo: 'tipo',
  });
  expect(
    nuevo('2', { name: 'a.mp4', size: MAX_BYTES + 1, type: 'video/mp4' }, 'org'),
  ).toMatchObject({
    estado: 'fallo',
    motivo: 'grande',
  });
  expect(nuevo('3', { name: 'a.mp4', size: MAX_BYTES, type: 'video/mp4' }, 'org').estado).toBe(
    'esperando',
  );
});

test('un .mkv sin tipo MIME (Linux) también es vídeo', () => {
  expect(esVideo('clase.mkv', '')).toBe(true);
  expect(esVideo('notas.txt', '')).toBe(false);
});

test('el título sale del nombre del fichero', () => {
  expect(tituloDe('entrevista_final.mp4')).toBe('entrevista final');
  expect(tituloDe('.mp4')).toBe('.mp4');
});

test('de uno en uno: mientras uno sube no sale el siguiente', () => {
  let cola = [
    nuevo('a', { name: 'a.mp4', size: 100, type: 'video/mp4' }, 'org'),
    nuevo('b', { name: 'b.mp4', size: 300, type: 'video/mp4' }, 'org'),
  ];
  expect(siguiente(cola)?.clave).toBe('a');
  cola = cambiar(cola, 'a', { estado: 'subiendo', progreso: 0.5 });
  expect(siguiente(cola)).toBeUndefined();
  expect(progresoTotal(cola)).toBeCloseTo(50 / 400);
  cola = cambiar(cola, 'a', { estado: 'procesando', progreso: 1 });
  expect(siguiente(cola)?.clave).toBe('b');
  expect(progresoTotal(cola)).toBeCloseTo(100 / 400);
});
