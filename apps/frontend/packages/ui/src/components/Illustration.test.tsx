/**
 * Illustration — su contrato: el SVG entra EN LÍNEA y con sus roles de color
 * intactos, que es lo único que hace que se vea.
 *
 * El defecto que arregla es mudo: por `<img src>` el dibujo es un documento
 * aparte, no hereda las `var(--sytue-*)` de la página y sale con todos los trazos
 * sin color. No hay error, no hay hueco roto — hay un espacio en blanco donde iba
 * la ilustración. Si alguien «optimiza» esto a un `<img>`, este test lo para.
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { Illustration } from './Illustration.js';

// Idempotente: otro fichero de test puede haber registrado ya happy-dom.
if (typeof document === 'undefined') GlobalRegistrator.register();

function mount(name: Parameters<typeof Illustration>[0]['name']): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(<Illustration name={name} />, host);
  return host;
}

test('el SVG se pinta dentro del documento, no se enlaza', () => {
  const host = mount('emptyMail');
  expect(host.querySelector('svg')).not.toBeNull();
  expect(host.querySelector('img')).toBeNull();
});

test('los trazos conservan los roles de color del tema', () => {
  // Sin esto el dibujo se ve, pero transparente: es exactamente el fallo que
  // Calendar arrastraba y que Docs esquivó renunciando a la ilustración.
  expect(mount('errorRetry').innerHTML).toContain('var(--sytue-');
});

test('es decorativa: no la lee nadie', () => {
  // Los SVG traen horneado desde branding/ un <title> y un aria-label EN
  // CASTELLANO. En línea y sin ocultar, un lector de pantalla en alemán los
  // leería tal cual; el estado vacío ya dice en palabras lo que pasa.
  expect(mount('emptyDrive').firstElementChild!.getAttribute('aria-hidden')).toBe('true');
});
