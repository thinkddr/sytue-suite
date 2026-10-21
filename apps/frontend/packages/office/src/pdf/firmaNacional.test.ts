/**
 * La ayuda para firmar con el certificado del sistema.
 *
 * Lo que se comprueba no es la lista —esa se verifica a mano y con fecha— sino
 * que se elige por REGIÓN y no por idioma: un austriaco y un alemán escriben los
 * dos `de` y sus herramientas no son la misma, así que confundirlas mandaría a
 * media Europa a descargar el programa de otro país. Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import { HERRAMIENTAS_NACIONALES, herramientaDe } from './firmaNacional.js';

test('elige por región, no por idioma', () => {
  expect(herramientaDe('es-ES')?.nombre).toBe('AutoFirma');
  expect(herramientaDe('de-AT')?.nombre).toBe('PDF-Over');
  // Alemania no tiene entrada propia: no se le ofrece la austriaca.
  expect(herramientaDe('de-DE')).toBeUndefined();
});

test('sin región no se adivina', () => {
  expect(herramientaDe('es')).toBeUndefined();
  expect(herramientaDe('')).toBeUndefined();
  expect(herramientaDe(undefined)).toBeUndefined();
});

test('todas llevan país, nombre y una dirección segura', () => {
  for (const h of HERRAMIENTAS_NACIONALES) {
    expect(h.pais).toMatch(/^[A-Z]{2}$/);
    expect(h.nombre.length).toBeGreaterThan(1);
    expect(h.url.startsWith('https://')).toBe(true);
  }
});
