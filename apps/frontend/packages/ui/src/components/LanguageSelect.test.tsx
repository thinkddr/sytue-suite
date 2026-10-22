/**
 * Tests de LanguageSelect — su contrato: ofrece los idiomas de producto con
 * el nombre de cada uno EN SU PROPIO IDIOMA (nunca traducido: es lo que permite
 * encontrar el tuyo cuando no entiendes la interfaz actual), marca el activo y
 * toma el idioma del `<html lang>` cuando no hay proveedor por encima.
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';

// Idempotente: otro fichero de test puede haber registrado ya happy-dom. El
// idioma se fija ANTES de importar el componente porque el valor sin proveedor
// se resuelve una sola vez (solo cambia recargando la página).
if (typeof document === 'undefined') GlobalRegistrator.register();
document.documentElement.lang = 'fr';

const { LanguageSelect } = await import('./LanguageSelect.js');

test('ofrece los 26 idiomas con su propio nombre y marca el activo', () => {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(<LanguageSelect />, host);

  const select = host.querySelector('select');
  const options = Array.from(host.querySelectorAll('option'));

  expect(options.map((o) => `${o.value}:${o.textContent}`)).toEqual([
    'en:English',
    'bg:Български',
    'cs:Čeština',
    'da:Dansk',
    'de:Deutsch',
    'el:Ελληνικά',
    'es:Español',
    'et:Eesti',
    'fi:Suomi',
    'fr:Français',
    'ga:Gaeilge',
    'hr:Hrvatski',
    'hu:Magyar',
    'is:Íslenska',
    'it:Italiano',
    'lt:Lietuvių',
    'lv:Latviešu',
    'mt:Malti',
    'nb:Norsk bokmål',
    'nl:Nederlands',
    'pl:Polski',
    'pt:Português',
    'ro:Română',
    'sk:Slovenčina',
    'sl:Slovenščina',
    'sv:Svenska',
  ]);
  expect(select?.value).toBe('fr');
  // La etiqueta sí se traduce, y sale en el idioma del documento.
  expect(host.querySelector('label')?.textContent).toBe('Langue');

  render(null, host);
});
