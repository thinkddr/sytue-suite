/* SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { useEffect } from 'preact/hooks';
import { hayCambios, useAvisoAlSalir, useGuardaDeFicha } from './cambios.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

let host: HTMLDivElement;
beforeEach(() => {
  host = document.createElement('div');
  document.body.appendChild(host);
});
afterEach(async () => {
  await act(async () => render(null, host));
  host.remove();
  document.body.innerHTML = '';
});

test('hayCambios compara campo a campo y trata lo que falta como vacío', () => {
  expect(hayCambios({ a: '1', b: '' }, { a: '1', b: '' })).toBe(false);
  expect(hayCambios({ a: '1' }, { a: '2' })).toBe(true);
  // Escribir y borrar un campo que el registro no traía no es un cambio.
  expect(hayCambios({ a: '1' }, { a: '1', b: '' })).toBe(false);
  expect(hayCambios({ a: '1' }, { a: '1', b: 'x' })).toBe(true);
  expect(hayCambios({ a: '1', b: 'x' }, { a: '1' })).toBe(true);
});

const textos = {
  titulo: '¿Descartar los cambios?',
  mensaje: 'Hay cambios sin guardar.',
  descartar: 'Descartar',
  seguir: 'Seguir editando',
};

type Guarda = ReturnType<typeof useGuardaDeFicha>;
let guarda: Guarda | null = null;
function Banco({ alPintar }: { alPintar: (g: Guarda) => void }): preact.JSX.Element {
  const g = useGuardaDeFicha(textos);
  useEffect(() => alPintar(g));
  return g.dialogo;
}

function boton(rotulo: string): HTMLButtonElement {
  const b = [...document.querySelectorAll<HTMLButtonElement>('button')].find(
    (x) => x.textContent?.trim() === rotulo,
  );
  if (!b) throw new Error(`No está el botón «${rotulo}»`);
  return b;
}

async function montar(): Promise<NonNullable<typeof guarda>> {
  await act(async () =>
    render(
      <Banco
        alPintar={(g) => {
          guarda = g;
        }}
      />,
      host,
    ),
  );
  if (!guarda) throw new Error('sin guarda');
  return guarda;
}

test('sin cambios, la salida va en el acto y no pregunta', async () => {
  const g = await montar();
  let salidas = 0;
  await act(async () => g.dejar(() => salidas++));
  expect(salidas).toBe(1);
  expect(document.body.textContent).not.toContain(textos.titulo);
});

test('con cambios, pregunta; «Seguir editando» se queda y «Descartar» sale', async () => {
  const g = await montar();
  let salidas = 0;
  g.marcar(true);
  await act(async () => g.dejar(() => salidas++));
  expect(salidas).toBe(0);
  expect(document.body.textContent).toContain(textos.titulo);
  expect(document.body.textContent).toContain(textos.mensaje);

  await act(async () => boton(textos.seguir).click());
  expect(salidas).toBe(0);
  expect(document.body.textContent).not.toContain(textos.titulo);

  // Sigue sucia: la próxima salida vuelve a preguntar.
  await act(async () => guarda!.dejar(() => salidas++));
  expect(document.body.textContent).toContain(textos.titulo);
  await act(async () => boton(textos.descartar).click());
  expect(salidas).toBe(1);

  // Descartado: lo siguiente ya no pregunta aunque la ficha no haya avisado aún.
  await act(async () => guarda!.dejar(() => salidas++));
  expect(salidas).toBe(2);
});

test('solo «Descartar» cuenta un descarte, que es lo que vuelve a montar la ficha', async () => {
  const g = await montar();
  expect(g.descartes).toBe(0);
  // Sin nada que perder no se descarta nada: la ficha no tiene por qué rehacerse.
  await act(async () => g.dejar(() => {}));
  expect(guarda!.descartes).toBe(0);

  guarda!.marcar(true);
  await act(async () => guarda!.dejar(() => {}));
  await act(async () => boton(textos.seguir).click());
  expect(guarda!.descartes).toBe(0);

  // «Nuevo» con un alta en blanco abierta sale a la misma ficha: sin el descarte en
  // su clave, lo escrito seguiría ahí y la guarda lo daría por perdido.
  await act(async () => guarda!.dejar(() => {}));
  await act(async () => boton(textos.descartar).click());
  expect(guarda!.descartes).toBe(1);
});

function Aviso({ activo }: { activo: boolean }): null {
  useAvisoAlSalir(activo);
  return null;
}

function salir(): Event {
  const ev = new Event('beforeunload', { cancelable: true });
  window.dispatchEvent(ev);
  return ev;
}

test('el aviso del navegador al salir solo está mientras hay cambios', async () => {
  await act(async () => render(<Aviso activo={false} />, host));
  expect(salir().defaultPrevented).toBe(false);
  await act(async () => render(<Aviso activo />, host));
  expect(salir().defaultPrevented).toBe(true);
  await act(async () => render(<Aviso activo={false} />, host));
  expect(salir().defaultPrevented).toBe(false);
});
