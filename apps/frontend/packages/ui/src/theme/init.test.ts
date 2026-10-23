import { expect, test } from 'bun:test';
import { ORG_COLOR_COOKIE, ORG_FORMA_COOKIE } from '@sytue/config';
import {
  DEFAULT_FORMA,
  ORG_COLOR_KEY,
  ORG_FORMA_KEY,
  THEME_COLORS,
  THEME_FORMAS,
  readThemePreference,
  themeInitScript,
} from './init.js';

/** Un documento que recuerda cada atributo por su nombre: modo, color y forma no se pisan. */
function arrancar(
  cookie: string | undefined,
  stored: Record<string, string | null> | 'blocked',
  /** Lo que el servidor ya puso en `<html>` antes de que corra el script. */
  delServidor: Record<string, string> = {},
) {
  const attrs = new Map<string, string>(Object.entries(delServidor));
  const document = {
    ...(cookie === undefined ? {} : { cookie }),
    documentElement: {
      setAttribute: (name: string, value: string) => void attrs.set(name, value),
      removeAttribute: (name: string) => void attrs.delete(name),
      hasAttribute: (name: string) => attrs.has(name),
    },
  };
  new Function('document', 'localStorage', themeInitScript)(document, {
    getItem: (key: string) => {
      if (stored === 'blocked') throw new Error('blocked');
      return stored[key] ?? null;
    },
  });
  return {
    theme: attrs.get('data-theme') ?? null,
    color: attrs.get('data-color') ?? null,
    app: attrs.get('data-app') ?? null,
    forma: attrs.get('data-forma') ?? null,
  };
}

for (const [stored, expected] of [
  [null, 'light'],
  ['invalid', 'light'],
  ['light', 'light'],
  ['dark', 'dark'],
  ['system', null],
  ['translucent', 'translucent'],
] as const) {
  test(`el primer pintado respeta ${stored ?? 'la ausencia de preferencia'}`, () => {
    expect(arrancar(undefined, { 'sytue-theme': stored }).theme).toBe(expected);
  });
}

test('el almacenamiento bloqueado también abre en claro con el color de cada app', () => {
  expect(arrancar(undefined, 'blocked')).toEqual({
    theme: 'light',
    color: 'app',
    app: null,
    forma: null,
  });
});

for (const [cookie, stored, expected] of [
  ['sytue-theme=dark', 'light', 'dark'],
  ['other=value; sytue-theme=translucent', 'dark', 'translucent'],
  ['sytue-theme=system', 'dark', null],
  ['sytue-theme=light', 'dark', 'light'],
  ['sytue-theme=invalid', 'dark', 'dark'],
  ['other=value', 'translucent', 'translucent'],
  ['', null, 'light'],
] as const) {
  test(`la preferencia compartida ${cookie || 'ausente'} se aplica antes de hidratar`, () => {
    expect(arrancar(cookie, { 'sytue-theme': stored }).theme).toBe(expected);
  });
}

test('la cookie compartida funciona con localStorage bloqueado', () => {
  expect(arrancar('sytue-theme=dark', 'blocked').theme).toBe('dark');
});

// El color: manda la persona; si no ha elegido, su organización; si no, el de cada
// app. Petróleo sigue siendo la hoja SIN atributo cuando se elige explícitamente.
for (const [cookie, stored, expected] of [
  ['', null, 'app'],
  ['', 'petroleo', null],
  ['sytue-color-org=petroleo', null, null],
  ['sytue-color=cobalto', null, 'cobalto'],
  ['', 'cobalto', 'cobalto'],
  ['sytue-color-org=cobalto', null, 'cobalto'],
  ['sytue-color=petroleo; sytue-color-org=cobalto', null, null],
  ['sytue-color=cobalto; sytue-color-org=petroleo', null, 'cobalto'],
  ['sytue-color=fucsia; sytue-color-org=cobalto', null, 'cobalto'],
  ['sytue-color-org=fucsia', null, 'app'],
  ['sytue-theme=dark; sytue-color=cobalto', null, 'cobalto'],
  ['sytue-color=app', null, 'app'],
  ['sytue-color-org=app', null, 'app'],
] as const) {
  test(`el color del primer pintado con ${cookie || 'nada'} y ${stored ?? 'nada'} guardado`, () => {
    expect(arrancar(cookie, { 'sytue-color': stored }).color).toBe(expected);
  });
}

test('el color no toca el modo, ni el modo el color', () => {
  expect(arrancar('sytue-theme=dark; sytue-color=cobalto', {})).toEqual({
    theme: 'dark',
    color: 'cobalto',
    app: null,
    forma: null,
  });
});

// «Color de cada app» necesita saber qué app es ANTES del primer pintado, y lo único
// que hay entonces es el prefijo que `basePathScript` acaba de publicar.
test('el script dice qué app es con el prefijo publicado, y nada si no lo hay', () => {
  const g = globalThis as { __sytueBase?: string };
  // `location` se sustituye entera y se devuelve como estaba: otro fichero de la misma
  // tanda puede haber registrado happy-dom, que trae la suya y no se deja asignar.
  const original = Object.getOwnPropertyDescriptor(globalThis, 'location');
  const enHost = (hostname: string): void => {
    Object.defineProperty(globalThis, 'location', {
      configurable: true,
      writable: true,
      value: { hostname },
    });
  };
  try {
    enHost('corporate.sytue.eu');
    g.__sytueBase = '/mail';
    expect(arrancar('', {}).app).toBe('mail');
    // Quien ya lo dice en su `<html>` manda sobre el prefijo: bajo `/docs` viven cuatro
    // productos, y el servidor pone el que se está viendo.
    g.__sytueBase = '/docs';
    expect(arrancar('', {}, { 'data-app': 'slides' }).app).toBe('slides');
    expect(arrancar('', {}).app).toBe('docs');
    g.__sytueBase = '/mail';
    // Acaba en un selector de atributo: solo letras, o nada.
    g.__sytueBase = '/x"]{}';
    expect(arrancar('', {}).app).toBeNull();
    // Servida en la raíz, la app la dice el host: Sytue Admin en `platform.<dominio>`.
    g.__sytueBase = '';
    enHost('platform.sytue.eu');
    expect(arrancar('', {}).app).toBe('platform');
    enHost('127.0.0.1');
    expect(arrancar('', {}).app).toBeNull();
    // Y sin prefijo publicado —el IdP— no hay app, tenga el host que tenga.
    delete g.__sytueBase;
    enHost('platform.sytue.eu');
    expect(arrancar('', {}).app).toBeNull();
  } finally {
    delete g.__sytueBase;
    if (original) Object.defineProperty(globalThis, 'location', original);
    else Reflect.deleteProperty(globalThis, 'location');
  }
});

// La forma: «Marco» es la hoja SIN atributo; solo una alternativa de la lista lo escribe.
for (const [cookie, stored, expected] of [
  ['', null, null],
  ['', 'carril', 'carril'],
  ['', 'marco', null],
  ['sytue-forma=carril', null, 'carril'],
  ['sytue-forma=marco', 'carril', null],
  ['sytue-forma=barra', 'carril', 'carril'],
  ['sytue-forma=barra', null, null],
  // Quien no ha elegido ve la de su organización; la suya manda sobre ella.
  ['sytue-forma-org=carril', null, 'carril'],
  ['sytue-forma=marco; sytue-forma-org=carril', null, null],
  ['sytue-forma=barra; sytue-forma-org=carril', null, 'carril'],
  ['sytue-forma-org=barra', null, null],
  ['sytue-theme=dark; sytue-color=cobalto; sytue-forma=carril', null, 'carril'],
] as const) {
  test(`la forma del primer pintado con ${cookie || 'nada'} y ${stored ?? 'nada'} guardado`, () => {
    expect(arrancar(cookie, { 'sytue-forma': stored }).forma).toBe(expected);
  });
}

test('la forma no toca el modo ni el color', () => {
  expect(arrancar('sytue-theme=dark; sytue-color=cobalto; sytue-forma=carril', {})).toEqual({
    theme: 'dark',
    color: 'cobalto',
    app: null,
    forma: 'carril',
  });
});

test('la lista de formas es la de la hoja de apariencia', async () => {
  const hoja = await Bun.file(
    new URL('../../../config/src/tokens/appearance.css', import.meta.url),
  ).text();
  const enLaHoja = [...hoja.matchAll(/^\[data-forma='([a-z]+)'\] \{/gm)].map((m) => m[1]);
  expect([DEFAULT_FORMA, ...enLaHoja]).toEqual([...THEME_FORMAS]);
});

// `init.ts` no puede importar nada —viaja incrustado en páginas sin bundle—, así que
// repite dos cosas que viven en otro sitio. Si se separan, el selector guardaría un
// color que la hoja no pinta, o el BFF escribiría una cookie que nadie lee.
test('la lista de colores es la de la hoja de tokens', async () => {
  const hoja = await Bun.file(
    new URL('../../../config/src/tokens/colors.css', import.meta.url),
  ).text();
  const enLaHoja = [...hoja.matchAll(/^\[data-color='([a-z]+)'\] \{/gm)].map((m) => m[1]);
  expect(['petroleo', ...enLaHoja]).toEqual([...THEME_COLORS]);
});

test('la cookie del color de organización se llama igual donde se escribe y donde se lee', () => {
  expect(ORG_COLOR_KEY).toBe(ORG_COLOR_COOKIE);
  expect(ORG_FORMA_KEY).toBe(ORG_FORMA_COOKIE);
});

test('los valores del primer pintado se escapan sin prohibir operadores del código', () => {
  // Bun puede emitir `typeof document > "u"` al serializar las funciones. El
  // operador es código válido; los valores interpolados van antes de sus cuerpos.
  const end = themeInitScript.indexOf(readThemePreference.toString());
  expect(end).toBeGreaterThan(0);
  expect(themeInitScript.slice(0, end)).not.toMatch(/[<>&\u2028\u2029]/);
  expect(themeInitScript).not.toMatch(/<\/script[\s/>]/i);
});
