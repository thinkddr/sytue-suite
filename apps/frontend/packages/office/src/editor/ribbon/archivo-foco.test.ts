import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { OPCION_ACTIVA } from './Ribbon';

if (typeof document === 'undefined') GlobalRegistrator.register();

// VISTO EN ROJO (2026-11-06): con `[role^="menuitem"]` a secas, abrir «Archivo» con la
// primera descarga inerte enfocaba ese botón `disabled`, el foco se quedaba en el
// disparador y accesibilidad-office fallaba en Docus, Calcus y Diapos.
test('al abrir «Archivo» el foco va a la primera opción que lo admite', () => {
  const menu = document.createElement('div');
  menu.innerHTML =
    '<button role="menuitem" disabled>PDF</button>' +
    '<button role="menuitem" aria-disabled="true">DOCX</button>' +
    '<button role="menuitemcheckbox">PDF accesible</button>' +
    '<button role="menuitem">Cambiar nombre</button>';
  document.body.appendChild(menu);
  menu.querySelector<HTMLElement>(OPCION_ACTIVA)?.focus();
  expect(document.activeElement?.textContent).toBe('PDF accesible');
  expect(Array.from(menu.querySelectorAll(OPCION_ACTIVA)).map((b) => b.textContent)).toEqual([
    'PDF accesible',
    'Cambiar nombre',
  ]);
  menu.remove();
});
