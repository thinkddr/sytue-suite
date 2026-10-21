import { expect, test } from 'bun:test';
import { APP_SWITCHER_ORDER, appsPresentables, suiteApps } from '@sytue/ui';
import { recientesVisibles } from './reciente.js';

/**
 * La galería del Hub es la PRIMERA pantalla de la suite, y su contenido salía de
 * una lista escrita a mano en `Launcher.tsx`. Notepads entró en el catálogo y
 * nadie tocó esa copia: el conmutador ofrecía el producto y la galería no,
 * durante versiones, sin que nada fallara.
 *
 * Estos dos tests fijan la regla que sustituyó a la lista: la galería enseña el
 * catálogo ENTERO menos el propio Hub.
 */

/** Lo que el lanzador ofrece: sin filtro de instalación, la suite entera. */
const enLaGaleria = (): string[] =>
  appsPresentables(suiteApps, [])
    .filter((a) => a.key !== 'one')
    .map((a) => a.key);

test('la galería ofrece TODOS los productos del catálogo menos el Hub', () => {
  const esperado = APP_SWITCHER_ORDER.filter((k) => k !== 'one');
  expect(enLaGaleria()).toEqual([...esperado]);
});

test('Notepads está, que es el que faltaba', () => {
  // Explícito y no solo cubierto por el test de arriba: si algún día alguien
  // vuelve a filtrar a mano, que el fallo diga el nombre del producto perdido.
  expect(enLaGaleria()).toContain('notes');
});

test('el Hub NO se ofrece a sí mismo', () => {
  expect(enLaGaleria()).not.toContain('one');
});

/**
 * «Reciente»: en «Marco» se pintan tres y las demás NO SE RENDERIZAN. Taparlas dejaba
 * un filete bajo la tercera, porque el `divide-y` de la lista contaba las tapadas.
 */
test('«Reciente» pinta tres entradas en Marco y todas en Carril', () => {
  const seis = ['a', 'b', 'c', 'd', 'e', 'f'];
  expect(recientesVisibles(seis, 'marco')).toEqual(['a', 'b', 'c']);
  expect(recientesVisibles(seis, 'carril')).toEqual(seis);
  // Con menos de tres no hay nada que quitar.
  expect(recientesVisibles(['a', 'b'], 'marco')).toEqual(['a', 'b']);
});
