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
import { act } from 'preact/test-utils';
import { detectLocale, persistLocale, readLocalePreference } from '@sytue/i18n';
import { I18nProvider } from '@sytue/i18n/preact';

// Idempotente: otro fichero de test puede haber registrado ya happy-dom. El
// idioma se fija ANTES de importar el componente porque el valor sin proveedor
// se resuelve una sola vez (solo cambia recargando la página).
if (typeof document === 'undefined') GlobalRegistrator.register();
document.documentElement.lang = 'fr';

const { LanguageSelect } = await import('./LanguageSelect.js');

test('ofrece Sistema y los 26 idiomas con su propio nombre', () => {
  document.cookie = 'sytue_locale=; path=/; max-age=0';
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(<LanguageSelect />, host);

  const select = host.querySelector('select');
  const options = Array.from(host.querySelectorAll('option'));

  expect(options.map((o) => `${o.value}:${o.textContent}`)).toEqual([
    'system:Système',
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
  expect(select?.value).toBe('system');
  // La etiqueta sí se traduce, y sale en el idioma del documento.
  expect(host.querySelector('label')?.textContent).toBe('Langue');

  render(null, host);
});

test('distingue la preferencia explícita del idioma efectivo del documento', () => {
  // El DOM de componentes puede vivir en about:blank, que no admite cookies.
  // Sólo se proporciona la cabecera de entrada; Chromium comprueba su persistencia.
  const cookie = Object.getOwnPropertyDescriptor(document, 'cookie');
  Object.defineProperty(document, 'cookie', { configurable: true, get: () => 'sytue_locale=es' });
  const host = document.createElement('div');
  document.body.appendChild(host);
  try {
    act(() =>
      render(
        <I18nProvider locale="fr">
          <LanguageSelect />
        </I18nProvider>,
        host,
      ),
    );
    expect(host.querySelector('select')?.value).toBe('es');
    expect(host.querySelector('label')?.textContent).toBe('Langue');
  } finally {
    render(null, host);
    host.remove();
    if (cookie) Object.defineProperty(document, 'cookie', cookie);
    else Reflect.deleteProperty(document, 'cookie');
  }
});

test.each([
  ['elegir el idioma efectivo guarda una preferencia explícita', '', 'fr'],
  ['volver a Sistema retira sólo la preferencia explícita', 'sytue_locale=es', 'system'],
])('%s', (_nombre, entrada, eleccion) => {
  const cookie = Object.getOwnPropertyDescriptor(document, 'cookie');
  const location = Object.getOwnPropertyDescriptor(globalThis, 'location');
  const escritos: string[] = [];
  let recargas = 0;
  Object.defineProperty(document, 'cookie', {
    configurable: true,
    get: () => entrada,
    set: (value: string) => escritos.push(value),
  });
  Object.defineProperty(globalThis, 'location', {
    configurable: true,
    value: {
      protocol: 'https:',
      reload: () => {
        recargas++;
      },
    },
  });
  const host = document.createElement('div');
  document.body.appendChild(host);
  try {
    act(() =>
      render(
        <I18nProvider locale="fr">
          <LanguageSelect />
        </I18nProvider>,
        host,
      ),
    );
    const select = host.querySelector('select')!;
    act(() => {
      select.value = eleccion;
      select.dispatchEvent(new Event('change', { bubbles: true }));
    });
    expect(escritos).toHaveLength(1);
    expect(escritos[0]).toStartWith(
      `sytue_locale=${eleccion === 'system' ? '' : eleccion}; path=/;`,
    );
    expect(escritos[0]).toContain(eleccion === 'system' ? 'max-age=0;' : 'max-age=31536000;');
    expect(recargas).toBe(1);
  } finally {
    act(() => render(null, host));
    host.remove();
    if (cookie) Object.defineProperty(document, 'cookie', cookie);
    else Reflect.deleteProperty(document, 'cookie');
    if (location) Object.defineProperty(globalThis, 'location', location);
    else Reflect.deleteProperty(globalThis, 'location');
  }
});

test.each([true, false])(
  'un getter de cookie bloqueado conserva el idioma (proveedor: %s)',
  (proveedor) => {
    const cookie = Object.getOwnPropertyDescriptor(document, 'cookie');
    const idioma = document.documentElement.lang;
    let lecturas = 0;
    Object.defineProperty(document, 'cookie', {
      configurable: true,
      get: () => {
        lecturas++;
        throw new DOMException('El documento no permite cookies', 'SecurityError');
      },
    });
    document.documentElement.lang = 'fr';
    const host = document.createElement('div');
    document.body.appendChild(host);
    try {
      act(() =>
        render(
          proveedor ? (
            <I18nProvider locale="fr">
              <LanguageSelect />
            </I18nProvider>
          ) : (
            <LanguageSelect />
          ),
          host,
        ),
      );
      expect(host.querySelector('select')?.value).toBe('system');
      expect(host.querySelector('label')?.textContent).toBe('Langue');
      expect(readLocalePreference()).toBeNull();
      expect(detectLocale()).toBe('fr');
      expect(lecturas).toBeGreaterThan(0);
    } finally {
      act(() => render(null, host));
      host.remove();
      document.documentElement.lang = idioma;
      if (cookie) Object.defineProperty(document, 'cookie', cookie);
      else Reflect.deleteProperty(document, 'cookie');
    }
  },
);

test('un setter de cookie bloqueado permite elegir idioma y volver a Sistema sin lanzar', () => {
  const cookie = Object.getOwnPropertyDescriptor(document, 'cookie');
  let escrituras = 0;
  Object.defineProperty(document, 'cookie', {
    configurable: true,
    get: () => '',
    set: () => {
      escrituras++;
      throw new DOMException('El documento no permite cookies', 'SecurityError');
    },
  });
  try {
    expect(() => persistLocale('fr')).not.toThrow();
    expect(() => persistLocale(null)).not.toThrow();
    expect(escrituras).toBe(2);
  } finally {
    if (cookie) Object.defineProperty(document, 'cookie', cookie);
    else Reflect.deleteProperty(document, 'cookie');
  }
});
