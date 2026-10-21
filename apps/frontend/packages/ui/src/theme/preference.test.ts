import { expect, test } from 'bun:test';
import { suiteApex } from '@sytue/config';
import {
  COLOR_KEY,
  DEFAULT_COLOR,
  ORG_COLOR_KEY,
  readThemePreference,
  STORAGE_KEY,
  THEME_COLORS,
  type ThemeColor,
} from './init.js';

/** El color en vigor, en el orden del script del primer pintado: persona, organización, casa. */
const readThemeColor = (): ThemeColor =>
  readThemePreference(
    readThemePreference<ThemeColor>(DEFAULT_COLOR, ORG_COLOR_KEY, THEME_COLORS),
    COLOR_KEY,
    THEME_COLORS,
  );
import { observeThemePreference, saveThemePreference, sharedThemeDomain } from './preference.js';

test('la cookie de apariencia comparte el apex y sus productos', () => {
  for (const host of ['sytue.eu', 'mail.sytue.eu', 'corporate.sytue.eu', 'id.sytue.eu']) {
    expect(sharedThemeDomain(host, 'sytue.eu')).toBe('sytue.eu');
  }
  expect(sharedThemeDomain('drive.sytue.localhost', 'sytue.localhost')).toBe('sytue.localhost');
  expect(sharedThemeDomain('cast.sytue.com', 'sytue.com')).toBe('sytue.com');
});

test('un sufijo engañoso o un dominio ajeno no recibe la cookie de suite', () => {
  for (const host of [
    'other.eu',
    'evilsytue.eu',
    'sytue.eu.example.org',
    'localhost',
    '127.0.0.1',
  ]) {
    expect(sharedThemeDomain(host, 'sytue.eu')).toBeNull();
  }
  expect(sharedThemeDomain('mail.sytue.eu', '')).toBeNull();
});

function withBrowser(
  run: (browser: {
    document: EventTarget & { cookie: string; visibilityState: string };
    window: EventTarget & { cookieStore: EventTarget };
    local: Map<string, string>;
  }) => void,
): void {
  const document = Object.assign(new EventTarget(), { cookie: '', visibilityState: 'visible' });
  const window = Object.assign(new EventTarget(), { cookieStore: new EventTarget() });
  const local = new Map<string, string>();
  const replacements = {
    document,
    window,
    location: { hostname: `erp.${suiteApex}`, protocol: 'https:' },
    localStorage: {
      getItem: (key: string) => local.get(key) ?? null,
      setItem: (key: string, value: string) => {
        local.set(key, value);
      },
      removeItem: (key: string) => {
        local.delete(key);
      },
    },
  };
  const descriptors = Object.keys(replacements).map(
    (key) => [key, Object.getOwnPropertyDescriptor(globalThis, key)] as const,
  );
  try {
    for (const [key, value] of Object.entries(replacements)) {
      Object.defineProperty(globalThis, key, { configurable: true, writable: true, value });
    }
    run({ document, window, local });
  } finally {
    for (const [key, descriptor] of descriptors) {
      if (descriptor) Object.defineProperty(globalThis, key, descriptor);
      else Reflect.deleteProperty(globalThis, key);
    }
  }
}

test('guardar Sistema comparte una cookie cosmética con caducidad y conserva su valor', () => {
  withBrowser(({ document, local }) => {
    saveThemePreference('system');
    expect(document.cookie).toBe(
      `sytue-theme=system; Path=/; Max-Age=31536000; SameSite=Lax; Secure; Domain=${suiteApex}`,
    );
    expect(local.get(STORAGE_KEY)).toBe('system');
  });
});

test('el color se guarda donde el modo, bajo su propia clave', () => {
  withBrowser(({ document, local }) => {
    saveThemePreference('cobalto', COLOR_KEY);
    expect(document.cookie).toBe(
      `sytue-color=cobalto; Path=/; Max-Age=31536000; SameSite=Lax; Secure; Domain=${suiteApex}`,
    );
    expect(local.get(COLOR_KEY)).toBe('cobalto');
    expect(local.has(STORAGE_KEY)).toBe(false);
  });
});

test('olvidar la elección borra la cookie y el almacenamiento: vuelve a mandar la organización', () => {
  withBrowser(({ document, local }) => {
    saveThemePreference('cobalto', COLOR_KEY);
    saveThemePreference(null, COLOR_KEY);
    // El navegador quita una cookie al recibirla con Max-Age=0, con el mismo dominio y ruta.
    expect(document.cookie).toBe(
      `sytue-color=; Path=/; Max-Age=0; SameSite=Lax; Secure; Domain=${suiteApex}`,
    );
    expect(local.has(COLOR_KEY)).toBe(false);
    document.cookie = 'sytue-color-org=cobalto';
    expect(readThemeColor()).toBe('cobalto');
  });
});

test('quien no ha elegido color sigue al de su organización cuando cambia', () => {
  withBrowser(({ document, window }) => {
    let color = readThemeColor();
    const stop = observeThemePreference(() => {
      color = readThemeColor();
    }, COLOR_KEY);
    expect(color).toBe('app');
    document.cookie = 'sytue-color-org=cobalto';
    window.cookieStore.dispatchEvent(new Event('change'));
    expect(color).toBe('cobalto');
    document.cookie = 'sytue-color=petroleo; sytue-color-org=cobalto';
    window.cookieStore.dispatchEvent(new Event('change'));
    expect(color).toBe('petroleo');
    stop();
  });
});

test('otras apps se sincronizan por cookie y su borrado elimina la elección local antigua', () => {
  withBrowser(({ document, window, local }) => {
    local.set(STORAGE_KEY, 'light');
    document.cookie = 'sytue-theme=dark';
    let mode = readThemePreference('light', STORAGE_KEY);
    const stop = observeThemePreference(() => {
      mode = readThemePreference('light', STORAGE_KEY);
    });
    document.cookie = 'sytue-theme=translucent';
    window.cookieStore.dispatchEvent(new Event('change'));
    expect(mode).toBe('translucent');
    document.cookie = '';
    window.cookieStore.dispatchEvent(new Event('change'));
    expect(mode).toBe('light');
    expect(local.has(STORAGE_KEY)).toBe(false);
    stop();
    document.cookie = 'sytue-theme=dark';
    window.cookieStore.dispatchEvent(new Event('change'));
    expect(mode).toBe('light');
  });
});

test('sin Cookie Store, volver a una pestaña detecta la caducidad sin sondeo', () => {
  withBrowser(({ document, window, local }) => {
    Reflect.deleteProperty(window, 'cookieStore');
    document.cookie = 'sytue-theme=dark';
    local.set(STORAGE_KEY, 'dark');
    let mode = readThemePreference('light', STORAGE_KEY);
    const stop = observeThemePreference(() => {
      mode = readThemePreference('light', STORAGE_KEY);
    });
    document.cookie = '';
    window.dispatchEvent(new Event('focus'));
    expect(mode).toBe('light');
    expect(local.has(STORAGE_KEY)).toBe(false);
    stop();
  });
});
