/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL DOCUMENTO NO DESPLAZA POR DEBAJO DEL ARMAZÓN. Las carcasas de alto fijo ocupan la
 * ventana y lo que desplaza es su `main`. Para que eso sea verdad, el `main` tiene que
 * ser el bloque contenedor de lo que lleva dentro: un `sr-only` es `position: absolute`,
 * y sin un ancestro posicionado se ancla al DOCUMENTO en el sitio que ocuparía sin
 * desplazar. Medido en la v3.11.0 a 1280×800: el documento crecía 315 px en Sytue Admin
 * y 674 en Mi cuenta, y en la forma «Marco» se veía el fondo de la página bajo el marco,
 * con las tarjetas cortadas por el borde de la hoja.
 *
 * happy-dom no maqueta, así que aquí se vigila la condición —que cada panel que desplaza
 * esté posicionado— y la medida en píxeles la da el barrido de capturas.
 *
 * Visto en ROJO quitando `relative` del `main` de AdminShell.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render, type ComponentChild } from 'preact';
import { I18nProvider } from '@sytue/i18n/preact';
import { AdminShell } from './AdminShell.js';
import { AppShell } from './AppShell.js';
import { Sidebar } from './Sidebar.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

/** Los paneles que desplazan dentro de `raiz` y no son bloque contenedor de lo suyo. */
function sinPosicionar(raiz: Element): string[] {
  return [...raiz.querySelectorAll('*')]
    .filter((e) => /(^|\s)overflow(-[xy])?-auto(\s|$)/.test(e.getAttribute('class') ?? ''))
    .filter(
      (e) => !/(^|\s)(relative|absolute|fixed|sticky)(\s|$)/.test(e.getAttribute('class') ?? ''),
    )
    .map((e) => `<${e.tagName.toLowerCase()} class="${e.getAttribute('class')}">`);
}

function monta(arbol: ComponentChild): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(<I18nProvider locale="es">{arbol}</I18nProvider>, host);
  return host;
}

/** Fuera del documento al acabar: otros tests buscan su armazón en `document`. */
function desmonta(host: HTMLElement): void {
  render(null, host);
  host.remove();
}

const menu = (
  <Sidebar>
    <a href="/uno">Uno</a>
  </Sidebar>
);

test('AdminShell: el main y el menú que desplazan son bloque contenedor de lo suyo', () => {
  const host = monta(<AdminShell sidebar={menu}>contenido</AdminShell>);
  expect(host.querySelector('main')?.getAttribute('class')).toContain('overflow-y-auto');
  const sueltos = sinPosicionar(host);
  desmonta(host);
  expect(sueltos).toEqual([]);
});

test('AppShell: el main y el menú que desplazan son bloque contenedor de lo suyo', () => {
  const host = monta(
    <AppShell topbar={null} sidebar={menu}>
      contenido
    </AppShell>,
  );
  expect(host.querySelector('main')?.getAttribute('class')).toContain('overflow-y-auto');
  const sueltos = sinPosicionar(host);
  desmonta(host);
  expect(sueltos).toEqual([]);
});
