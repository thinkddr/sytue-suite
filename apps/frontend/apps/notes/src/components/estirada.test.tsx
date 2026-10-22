/*
 * LA NOTA ESTIRADA SE PUEDE ESCRIBIR HASTA ABAJO.
 *
 * Una tarjeta se estira por dos caminos: el panel de una nota en árbol y la nota
 * que se ensancha al editarla. El segundo estiraba la tarjeta y no lo de dentro:
 * el campo se quedaba en sus 96px de `min-h-24` y debajo había medio folio de
 * blanco donde pulsar no hacía nada.
 *
 * LO QUE SE COMPRUEBA ES LA CADENA ENTERA, no una clase suelta, y esa lección
 * salió cara: la primera versión de este test miraba si el campo tenía `flex-1`,
 * pasaba en verde, y la geometría seguía rota. Entre el contenedor y el campo hay
 * un envoltorio `relative` —lleva el aviso de «escribe algo» posicionado encima—
 * que era `display:block` y no crecía, así que el `flex-1` del campo no tenía
 * contra qué medirse. Medido en el piloto: tarjeta 630px, campo 84px, 446px
 * muertos, con la clase correcta puesta.
 *
 * `flex-1` en un nieto no sirve de nada si el hijo de en medio no crece. Por eso
 * se recorre desde el campo hasta la raíz: si alguien mete otro div intermedio,
 * aquí se ve.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { RichText } from './RichText.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const t = ((k: string) => k) as never;

function montar(estirado: boolean): { raiz: HTMLElement; campo: HTMLElement } {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(
    <RichText
      t={t}
      initialHtml="<p>tres palabras</p>"
      ariaLabel="nota"
      onChange={() => {}}
      estirado={estirado}
    />,
    host,
  );
  return {
    raiz: host.firstElementChild as HTMLElement,
    campo: host.querySelector('[contenteditable]') as HTMLElement,
  };
}

/** Los elementos entre el campo y la raíz, los dos incluidos. */
function cadena(raiz: HTMLElement, campo: HTMLElement): HTMLElement[] {
  const eslabones: HTMLElement[] = [];
  for (let e: HTMLElement | null = campo; e; e = e.parentElement) {
    eslabones.push(e);
    if (e === raiz) break;
  }
  return eslabones;
}

test('estirado: TODOS los eslabones crecen, no solo el campo', () => {
  const { raiz, campo } = montar(true);
  const rotos = cadena(raiz, campo)
    .filter((e) => !e.className.includes('flex-1'))
    .map((e) => e.className || e.tagName);
  expect(rotos).toEqual([]);
});

test('estirado: el envoltorio del campo es flex, o su hijo no tiene dónde crecer', () => {
  const { campo } = montar(true);
  const envoltorio = campo.parentElement as HTMLElement;
  expect(envoltorio.className).toContain('flex');
  expect(envoltorio.className).toContain('flex-col');
});

test('sin estirar: se queda con su alto mínimo, como estaba', () => {
  const { raiz, campo } = montar(false);
  expect(raiz.className).not.toContain('flex-1');
  expect(campo.className).not.toContain('flex-1');
  // El mínimo se conserva: sin él, un campo vacío no tiene dónde pulsar.
  expect(campo.className).toContain('min-h-24');
});
