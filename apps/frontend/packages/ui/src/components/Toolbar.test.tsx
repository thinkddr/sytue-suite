/**
 * Tests de Toolbar — su contrato: UNA sola parada de tabulación, las flechas mueven el
 * foco y dan la vuelta, y dentro de un campo las flechas siguen siendo del campo.
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import type { VNode } from 'preact';
import { Toolbar } from './Toolbar.js';

// Idempotente: otro fichero de test puede haber registrado ya happy-dom.
if (typeof document === 'undefined') GlobalRegistrator.register();

function mount(children: VNode): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(children, host);
  return host;
}

function botones(host: HTMLElement): HTMLButtonElement[] {
  return Array.from(host.querySelectorAll('button'));
}

function pulsar(host: HTMLElement, key: string): void {
  host
    .querySelector('[role="toolbar"]')!
    .dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }));
}

const TRES = (
  <Toolbar aria-label="Acciones">
    <button type="button">Uno</button>
    <button type="button">Dos</button>
    <button type="button">Tres</button>
  </Toolbar>
);

test('la barra entera es una sola parada de tabulación', () => {
  const host = mount(TRES);
  expect(botones(host).map((b) => b.tabIndex)).toEqual([0, -1, -1]);
});

test('las flechas mueven el foco y dan la vuelta', () => {
  const host = mount(TRES);
  const [uno, dos, tres] = botones(host);
  uno!.focus();

  pulsar(host, 'ArrowRight');
  expect(document.activeElement).toBe(dos!);
  // La parada de tabulación viaja con el foco: al volver con Tab se entra por donde se
  // salió, no por el principio.
  expect(botones(host).map((b) => b.tabIndex)).toEqual([-1, 0, -1]);

  pulsar(host, 'ArrowLeft');
  pulsar(host, 'ArrowLeft');
  expect(document.activeElement).toBe(tres!); // desde el primero, atrás = al último

  pulsar(host, 'Home');
  expect(document.activeElement).toBe(uno!);
  pulsar(host, 'End');
  expect(document.activeElement).toBe(tres!);
});

test('dentro de un campo, las flechas son del campo', () => {
  const host = mount(
    <Toolbar aria-label="Acciones">
      <select>
        <option>a</option>
      </select>
      <button type="button">Uno</button>
    </Toolbar>,
  );
  const campo = host.querySelector('select')!;
  campo.focus();
  campo.dispatchEvent(
    new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true, cancelable: true }),
  );
  expect(document.activeElement).toBe(campo); // el foco no se ha ido a ninguna parte
});

/*
 * El componente existe para que el ROL y el COMPORTAMIENTO no vuelvan a separarse, y eso
 * no lo garantiza ningún test de este fichero: lo rompe quien escriba `role="toolbar"` a
 * mano en otra app. Pasó dos veces —la cinta de Sytue PDF y la barra de selección de
 * Fotos—, y desde fuera no se ve: el lector de pantalla anuncia «barra de herramientas,
 * usa las flechas», las flechas no hacen nada y salir cuesta un tab por botón.
 *
 * Se mira el CÓDIGO FUENTE porque es donde se comete: renderizar las nueve apps para
 * comprobarlo costaría minutos y no cubriría las barras que solo salen con algo marcado.
 */
test('nadie promete role="toolbar" sin el componente que lo cumple', async () => {
  // Las apps y la ofimática común (`packages/office`), que es código de app.
  const raiz = new URL('../../../../', import.meta.url);
  const infractores: string[] = [];
  const fuentes = new Bun.Glob('{apps/*/src/**/*.tsx,packages/office/src/**/*.tsx}');
  for await (const rel of fuentes.scan({ cwd: raiz.pathname })) {
    const fuente = await Bun.file(new URL(rel, raiz)).text();
    for (const [i, linea] of fuente.split('\n').entries()) {
      if (linea.includes('role="toolbar"')) infractores.push(`${rel}:${i + 1}`);
    }
  }
  expect(infractores, 'usa <Toolbar> de @sytue/ui en vez de escribir el rol').toEqual([]);
});

test('`omitir` deja fuera los controles de un popover: conservan su tabulación', () => {
  const host = mount(
    <Toolbar aria-label="Acciones" omitir=".popover">
      <button type="button">Uno</button>
      <button type="button">Dos</button>
      <div class="popover">
        <input type="text" />
        <button type="button">Dentro</button>
      </div>
    </Toolbar>,
  );
  const [uno, dos, dentro] = botones(host);
  // Los de la barra, con una sola parada; los del popover, intactos.
  expect([uno!.tabIndex, dos!.tabIndex]).toEqual([0, -1]);
  expect(dentro!.tabIndex).toBe(0);
  expect(host.querySelector('input')!.tabIndex).toBe(0);

  uno!.focus();
  pulsar(host, 'End');
  expect(document.activeElement).toBe(dos!); // el último de la BARRA, no el del popover
});

test('`omitir`: las flechas dentro de un popover no las toma la barra', () => {
  const host = mount(
    <Toolbar aria-label="Acciones" omitir=".popover">
      <button type="button">Uno</button>
      <button type="button">Dos</button>
      <div class="popover">
        <button type="button">Dentro</button>
      </div>
    </Toolbar>,
  );
  const dentro = botones(host)[2]!;
  dentro.focus();
  dentro.dispatchEvent(
    new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true, cancelable: true }),
  );
  expect(document.activeElement).toBe(dentro); // el foco no se ha ido a la barra
});

test('`data-toolbar-skip`: una región con scroll enfocable no entra en el roving', () => {
  const host = mount(
    <Toolbar aria-label="Acciones">
      <button type="button">Uno</button>
      {/* eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex */}
      <div data-toolbar-skip tabIndex={0} role="group" aria-label="Galería">
        <button type="button">Ficha</button>
      </div>
      <button type="button">Dos</button>
    </Toolbar>,
  );
  const region = host.querySelector<HTMLElement>('[data-toolbar-skip]')!;
  // La región conserva su propia parada (que es lo que pide WCAG 2.1.1 para lo que se
  // desplaza) y la barra no se la quita.
  expect(region.tabIndex).toBe(0);
  const [uno, ficha, dos] = botones(host);
  expect([uno!.tabIndex, ficha!.tabIndex, dos!.tabIndex]).toEqual([0, -1, -1]);
});
