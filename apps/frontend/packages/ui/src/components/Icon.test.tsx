/**
 * Icon — los glifos del reproductor y de la navegación de Sytue Cast existen y
 * caben en la rejilla de 24 (ninguna coordenada absoluta fuera de 0–24). Un
 * trazado que se sale no da error: se recorta en silencio a 16 px.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { Icon, type IconName } from './Icon.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const NUEVOS: IconName[] = [
  'play',
  'pause',
  'vozOff',
  'maximizar',
  'minimizar',
  'pip',
  'home',
  'explorar',
  'emitir',
  'video',
  'ojo',
  'ojoOff',
];

function trazado(name: IconName): string {
  const host = document.createElement('div');
  render(<Icon name={name} />, host);
  // El último `<path>`: el primero puede ser la capa de relleno de `Glifo`.
  return [...host.querySelectorAll('path')].at(-1)?.getAttribute('d') ?? '';
}

test.each(NUEVOS)('%s se pinta con un trazado propio', (name) => {
  const d = trazado(name);
  expect(d.length).toBeGreaterThan(10);
  // Ninguno repite el de otro: dos iconos iguales con nombres distintos dicen
  // dos cosas con el mismo dibujo.
  for (const otro of NUEVOS) if (otro !== name) expect(trazado(otro)).not.toBe(d);
});

test.each(NUEVOS)('%s cabe en la rejilla de 24', (name) => {
  // Solo los comandos ABSOLUTOS en mayúscula llevan coordenadas de la rejilla;
  // los relativos (minúscula) son desplazamientos y pueden ser negativos.
  for (const [, comando, args] of trazado(name).matchAll(/([MLHVZ])([^A-Za-z]*)/g)) {
    if (comando === 'Z') continue;
    for (const n of (args ?? '').match(/-?\d*\.?\d+/g) ?? []) {
      const v = Number(n);
      expect(v).toBeGreaterThanOrEqual(0);
      expect(v).toBeLessThanOrEqual(24);
    }
  }
});
