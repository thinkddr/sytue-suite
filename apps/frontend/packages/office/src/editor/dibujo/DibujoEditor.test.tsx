/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El editor de dibujo, por su lado accesible: se maneja entero con el teclado, tiene
 * roles y nombres, y anuncia lo que hace en una región aria-live. El puntero (arrastrar,
 * asas) no se prueba aquí: happy-dom no mide cajas; su geometría la cubre
 * @sytue/escena (golpear, redimensionar) y la verán los barridos en navegador real.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { escenaVacia } from '@sytue/escena';
import type { Escena } from '@sytue/escena';
import { DibujoEditor } from './DibujoEditor';

if (typeof document === 'undefined') GlobalRegistrator.register();

const tick = (): Promise<void> => new Promise((r) => setTimeout(r, 0));

function abrir(
  guardar: (e: Escena) => void = () => undefined,
  cancelar: () => void = () => undefined,
): HTMLElement {
  document.body.innerHTML = '';
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(
    <DibujoEditor
      open
      inicial={escenaVacia({ ancho: 640, alto: 400, nombreCapa: 'Capa 1' })}
      aceptar="Insertar"
      onGuardar={guardar}
      onCancelar={cancelar}
    />,
    host,
  );
  return document.body;
}

const boton = (raiz: ParentNode, nombre: string): HTMLButtonElement => {
  const b = [...raiz.querySelectorAll('button')].find(
    (x) => x.getAttribute('aria-label') === nombre,
  );
  if (!b) throw new Error(`sin botón «${nombre}»`);
  return b;
};

const lienzo = (raiz: ParentNode): HTMLElement =>
  raiz.querySelector('[role="application"][aria-label="Drawing canvas"]')!;

function tecla(el: Element, key: string, init: KeyboardEventInit = {}): void {
  el.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true, ...init }));
}

test('la barra de herramientas tiene rol, nombre y herramientas con aria-pressed', async () => {
  const raiz = abrir();
  await tick();
  const barra = raiz.querySelector('[role=toolbar]');
  expect(barra?.getAttribute('aria-label')).toBe('Drawing tools');
  const seleccionar = boton(raiz, 'Select');
  expect(seleccionar.getAttribute('aria-pressed')).toBe('true');
  expect(boton(raiz, 'Rectangle').getAttribute('aria-pressed')).toBe('false');
  expect(seleccionar.tabIndex).toBe(0);
  seleccionar.focus();
  tecla(seleccionar, 'ArrowRight');
  expect(document.activeElement).toBe(boton(raiz, 'Rectangle'));
  expect(seleccionar.tabIndex).toBe(-1);
  expect(raiz.querySelector('[role="dialog"]')).not.toBeNull();
});

test('con el teclado: armar una forma, Intro la coloca, las flechas la mueven y Supr la borra', async () => {
  const raiz = abrir();
  await tick();
  boton(raiz, 'Rectangle').click();
  await tick();
  expect(boton(raiz, 'Rectangle').getAttribute('aria-pressed')).toBe('true');
  const cv = lienzo(raiz);
  tecla(cv, 'Enter');
  await tick();
  const aviso = raiz.querySelector('[role="status"]')!;
  expect(aviso.textContent).toBe('Rectangle 1 added');
  // Aparece en la lista de objetos, seleccionado, y la herramienta vuelve a Seleccionar.
  const objeto = [...raiz.querySelectorAll('section[aria-labelledby="dib-objetos"] button')][0]!;
  expect(objeto.textContent).toBe('Rectangle 1');
  expect(objeto.getAttribute('aria-pressed')).toBe('true');
  expect(boton(raiz, 'Select').getAttribute('aria-pressed')).toBe('true');
  // El SVG de la escena lleva la forma.
  expect(raiz.querySelectorAll('svg rect').length).toBeGreaterThan(0);

  tecla(cv, 'ArrowRight', { shiftKey: true });
  await tick();
  expect(aviso.textContent).toBe('Rectangle 1 moved to 260, 155');
  tecla(cv, 'ArrowDown');
  await tick();
  expect(aviso.textContent).toBe('Rectangle 1 moved to 260, 156');

  tecla(cv, 'Delete');
  await tick();
  expect(aviso.textContent).toBe('Rectangle 1 deleted');
  expect(raiz.querySelector('section[aria-labelledby="dib-objetos"] ul')).toBeNull();
});

test('deshacer y rehacer, con los botones y con Ctrl+Z', async () => {
  const raiz = abrir();
  await tick();
  expect(boton(raiz, 'Undo').disabled).toBe(true);
  boton(raiz, 'Ellipse').click();
  await tick();
  tecla(lienzo(raiz), 'Enter');
  await tick();
  expect(boton(raiz, 'Undo').disabled).toBe(false);
  boton(raiz, 'Undo').click();
  await tick();
  expect(raiz.querySelector('section[aria-labelledby="dib-objetos"] ul')).toBeNull();
  expect(boton(raiz, 'Redo').disabled).toBe(false);
  tecla(lienzo(raiz), 'z', { ctrlKey: true, shiftKey: true });
  await tick();
  expect(raiz.querySelector('section[aria-labelledby="dib-objetos"] ul')).not.toBeNull();
});

test('«[» y «]» recorren los objetos y la lista de objetos los selecciona', async () => {
  const raiz = abrir();
  await tick();
  for (const f of ['Rectangle', 'Ellipse']) {
    boton(raiz, f).click();
    await tick();
    tecla(lienzo(raiz), 'Enter');
    await tick();
  }
  const aviso = raiz.querySelector('[role="status"]')!;
  tecla(lienzo(raiz), '[');
  await tick();
  expect(aviso.textContent).toBe('Rectangle 1 selected');
  tecla(lienzo(raiz), ']');
  await tick();
  expect(aviso.textContent).toBe('Ellipse 1 selected');
  tecla(lienzo(raiz), 'Escape');
  await tick();
  expect(aviso.textContent).toBe('Selection cleared');
});

test('conectar exige exactamente dos objetos seleccionados y crea un conector con nombre', async () => {
  const raiz = abrir();
  await tick();
  expect(boton(raiz, 'Connect the two selected objects').disabled).toBe(true);
  for (const f of ['Rectangle', 'Ellipse']) {
    boton(raiz, f).click();
    await tick();
    tecla(lienzo(raiz), 'Enter');
    await tick();
  }
  const filas = [...raiz.querySelectorAll('section[aria-labelledby="dib-objetos"] button')];
  filas[0]!.dispatchEvent(new MouseEvent('click', { bubbles: true })); // Ellipse (la lista va de arriba abajo)
  await tick();
  filas[1]!.dispatchEvent(new MouseEvent('click', { bubbles: true, shiftKey: true }));
  await tick();
  const conectar = boton(raiz, 'Connect the two selected objects');
  expect(conectar.disabled).toBe(false);
  conectar.click();
  await tick();
  expect(raiz.querySelector('[role="status"]')?.textContent).toBe('Connector 1 added');
});

test('las capas se nombran, se ocultan y se bloquean; no se puede borrar la última', async () => {
  const raiz = abrir();
  await tick();
  expect(boton(raiz, 'Delete layer Capa 1').disabled).toBe(true);
  [...raiz.querySelectorAll('button')].find((b) => b.textContent?.includes('Add layer'))!.click();
  await tick();
  expect(raiz.querySelectorAll('section[aria-labelledby="dib-capas"] li')).toHaveLength(2);
  const mostrar = boton(raiz, 'Show layer Layer 2');
  expect(mostrar.getAttribute('aria-pressed')).toBe('true');
  mostrar.click();
  await tick();
  expect(boton(raiz, 'Show layer Layer 2').getAttribute('aria-pressed')).toBe('false');
  const bloquear = boton(raiz, 'Lock layer Layer 2');
  expect(bloquear.getAttribute('aria-pressed')).toBe('false');
  bloquear.click();
  await tick();
  expect(boton(raiz, 'Lock layer Layer 2').getAttribute('aria-pressed')).toBe('true');
  expect(boton(raiz, 'Delete layer Layer 2').disabled).toBe(false);
});

test('aceptar entrega la escena editada', async () => {
  let entregada: Escena | undefined;
  const raiz = abrir((e) => (entregada = e));
  await tick();
  boton(raiz, 'Star').click();
  await tick();
  tecla(lienzo(raiz), 'Enter');
  await tick();
  const aceptar = [...raiz.querySelectorAll('button')].find((b) => b.textContent === 'Insertar')!;
  aceptar.click();
  expect(entregada?.capas[0]!.nodos).toHaveLength(1);
  expect(entregada?.capas[0]!.nodos[0]).toMatchObject({
    tipo: 'forma',
    forma: 'star',
    nombre: 'Star 1',
  });
});

test('Escape quita la herramienta armada y después la selección, sin cerrar el diálogo', async () => {
  let cerrado = 0;
  const raiz = abrir(undefined, () => cerrado++);
  await tick();
  boton(raiz, 'Rectangle').click();
  await tick();
  tecla(lienzo(raiz), 'Escape');
  await tick();
  expect(boton(raiz, 'Rectangle').getAttribute('aria-pressed')).toBe('false');
  expect(boton(raiz, 'Select').getAttribute('aria-pressed')).toBe('true');
  expect(cerrado).toBe(0);

  boton(raiz, 'Rectangle').click();
  await tick();
  tecla(lienzo(raiz), 'Enter');
  await tick();
  tecla(lienzo(raiz), 'Escape');
  await tick();
  expect(raiz.querySelector('[role="status"]')?.textContent).toBe('Selection cleared');
  expect(cerrado).toBe(0);
});

test('cerrar con cambios pregunta si se descartan; sin cambios cierra directamente', async () => {
  let cerrado = 0;
  let raiz = abrir(undefined, () => cerrado++);
  await tick();
  boton(raiz, 'Rectangle').click();
  await tick();
  tecla(lienzo(raiz), 'Enter');
  await tick();
  const cancelar = [...raiz.querySelectorAll('button')].find((b) => b.textContent === 'Cancel')!;
  cancelar.click();
  await tick();
  expect(raiz.textContent).toContain('Discard the changes?');
  expect(cerrado).toBe(0);
  [...raiz.querySelectorAll('button')].find((b) => b.textContent === 'Discard')!.click();
  await tick();
  expect(cerrado).toBe(1);

  raiz = abrir(undefined, () => cerrado++);
  await tick();
  [...raiz.querySelectorAll('button')].find((b) => b.textContent === 'Cancel')!.click();
  await tick();
  expect(cerrado).toBe(2);
  expect(raiz.textContent).not.toContain('Discard the changes?');
});

test('comentar una forma: solo en un dibujo insertado y con la forma ya guardada', async () => {
  const comentarios: [string, unknown][] = [];
  document.body.innerHTML = '';
  const host = document.createElement('div');
  document.body.appendChild(host);
  let e = escenaVacia({ ancho: 640, alto: 400, nombreCapa: 'Capa 1' });
  e = {
    ...e,
    capas: [
      {
        ...e.capas[0]!,
        nodos: [
          { id: 'a', tipo: 'forma', forma: 'rect', nombre: 'Caja', x: 10, y: 10, w: 50, h: 40 },
        ],
      },
    ],
  };
  render(
    <DibujoEditor
      open
      inicial={e}
      aceptar="Aplicar"
      onGuardar={() => undefined}
      onCancelar={() => undefined}
      onComentar={async (c, d) => void comentarios.push([c, d])}
    />,
    host,
  );
  await tick();
  // Sin selección: comentario en una coordenada (centro por defecto).
  expect(document.body.textContent).toContain('Comment on a point of the canvas');
  const area = document.body.querySelector('textarea[aria-label="Comment"]') as HTMLTextAreaElement;
  area.value = 'Revisar esto';
  area.dispatchEvent(new Event('input', { bubbles: true }));
  await tick();
  const enviar = () =>
    [...document.body.querySelectorAll('form button')].find(
      (b) => b.textContent === 'Comment',
    ) as HTMLButtonElement;
  enviar().click();
  await tick();
  expect(comentarios).toEqual([['Revisar esto', { x: 320, y: 200 }]]);
  expect(
    document.body.querySelector('[role="status"][aria-live="polite"]:not(.sr-only)')?.textContent,
  ).toBe('Comment added');
  // Con la forma seleccionada: anclado a su id.
  const fila = [
    ...document.body.querySelectorAll('section[aria-labelledby="dib-objetos"] button'),
  ][0] as HTMLButtonElement;
  fila.click();
  await tick();
  expect(document.body.textContent).toContain('Comment on Caja');
  const area2 = document.body.querySelector(
    'textarea[aria-label="Comment"]',
  ) as HTMLTextAreaElement;
  area2.value = 'Cambiar color';
  area2.dispatchEvent(new Event('input', { bubbles: true }));
  await tick();
  enviar().click();
  await tick();
  expect(comentarios[1]).toEqual(['Cambiar color', { nodo: 'a', nombre: 'Caja' }]);
});

test('sin onComentar (dibujo aún sin insertar) no hay formulario de comentarios', async () => {
  const raiz = abrir();
  await tick();
  expect(raiz.querySelector('textarea[aria-label="Comment"]')).toBeNull();
});
