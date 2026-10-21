/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LA PESTAÑA DE FIRMAR SE LLAMA «FIRMAR».
 *
 * Se llamaba «Proteger», como el botón de la contraseña que lleva dentro, y quien
 * venía a firmar —que es a lo que se viene a CertiPDF— no la encontraba (orden del
 * dueño, 2026-10-16). Lo que se fija aquí es lo que se ve: el rótulo de la pestaña,
 * que lo primero que enseña es firmar, que la contraseña sigue a mano con su propio
 * nombre y que la censura, que pinta siempre en negro, no arrastra los seis colores
 * del trazo, que ahí no cambiaban nada.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { docsT } from '../i18n';
import type { Permisos } from './api';
import { BarraHerramientas } from './Barra';
import type { Dialogo } from './Paneles';

if (typeof document === 'undefined') GlobalRegistrator.register();

const tick = (): Promise<void> => new Promise((r) => setTimeout(r, 0));
const t = docsT('es');

const TODO: Permisos = {
  Imprimir: true,
  Modificar: true,
  Copiar: true,
  Anotar: true,
  RellenarCampos: true,
  Accesibilidad: true,
  Ensamblar: true,
  ImprimirEnAlta: true,
  SoloConClaveDeDueno: false,
};

function montar(onDialogo: (d: Dialogo) => void = () => undefined): HTMLElement {
  document.body.innerHTML = '';
  const host = document.createElement('div');
  document.body.appendChild(host);
  const nada = (): void => undefined;
  render(
    <BarraHerramientas
      t={t}
      herramienta="mano"
      onHerramienta={nada}
      color="#f9b923"
      onColor={nada}
      grosor={2}
      onGrosor={nada}
      zoom={1}
      ajuste="ancho"
      onZoom={nada}
      onAjuste={nada}
      dosPaginas={false}
      onDosPaginas={nada}
      leyendo={false}
      onLeer={nada}
      pagina={1}
      total={3}
      onIrA={nada}
      onDialogo={onDialogo}
      permisos={TODO}
      trabajando={false}
      panelAbierto={false}
      onPanelAbierto={nada}
    />,
    host,
  );
  return host;
}

const texto = (e: Element): string => e.textContent?.trim() ?? '';

async function abrirFirmar(host: HTMLElement): Promise<Element[]> {
  const pestana = [...host.querySelectorAll('[role="tab"]')].find(
    (x) => texto(x) === t('pdf.menu.sign'),
  ) as HTMLButtonElement | undefined;
  expect(pestana).toBeDefined();
  pestana!.click();
  await tick();
  return [...host.querySelectorAll('[role="tabpanel"] [role="group"]')];
}

test('la pestaña se llama «Firmar» y ya no hay ninguna «Proteger»', () => {
  const host = montar();
  const pestanas = [...host.querySelectorAll('[role="tab"]')].map(texto);
  expect(t('pdf.menu.sign')).toBe('Firmar');
  expect(pestanas).toContain('Firmar');
  expect(pestanas).not.toContain(t('pdf.menu.protect'));
});

test('lo primero de Firmar es firmar; detrás, la contraseña y la censura', async () => {
  const grupos = await abrirFirmar(montar());
  expect(grupos.map((g) => g.getAttribute('aria-label'))).toEqual([
    t('pdf.panel.signatures'),
    t('pdf.ribbon.group.security'),
    t('pdf.tools'),
  ]);
  expect([...grupos[0]!.querySelectorAll('button')].map(texto)).toEqual(['Firmar', 'Pedir firma']);
  expect([...grupos[1]!.querySelectorAll('button')].map(texto)).toEqual(['Proteger']);
  expect(grupos[2]!.querySelector('button')?.getAttribute('aria-label')).toBe(t('pdf.tool.redact'));
});

test('la censura no ofrece colores: su barra es negra siempre', async () => {
  const grupos = await abrirFirmar(montar());
  expect(grupos.map((g) => g.getAttribute('aria-label'))).not.toContain(
    t('pdf.ribbon.group.stroke'),
  );
});

test('cada botón abre su diálogo, no el de al lado', async () => {
  const abiertos: Dialogo[] = [];
  const grupos = await abrirFirmar(montar((d) => abiertos.push(d)));
  for (const b of [
    ...grupos[0]!.querySelectorAll('button'),
    ...grupos[1]!.querySelectorAll('button'),
  ]) {
    (b as HTMLButtonElement).click();
  }
  expect(abiertos).toEqual(['firmar', 'pedirFirma', 'proteger']);
});
