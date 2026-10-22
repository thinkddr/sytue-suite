/** Inicialización sin dependencias para Astro y las páginas autocontenidas. */
export const STORAGE_KEY = 'sytue-theme';
export type ThemeMode = 'light' | 'dark' | 'translucent' | 'system';

/**
 * El color es un eje aparte del modo (ADR 0071): cada valor es un bloque
 * `[data-color='…']` de la hoja de tokens, salvo el de la casa, que es la hoja sin
 * atributo. `init.test.ts` falla si esta lista y la hoja dejan de decir lo mismo.
 */
export const THEME_COLORS = ['petroleo', 'cobalto', 'app'] as const;
export type ThemeColor = (typeof THEME_COLORS)[number];
/** Sin elección de la persona ni de su organización, cada app usa su color. */
export const DEFAULT_COLOR: ThemeColor = 'app';
/** La elección de la persona: misma cookie de apex y mismo almacenamiento que el modo. */
export const COLOR_KEY = 'sytue-color';
/** El color de su organización, que escribe el BFF con la sesión. */
export const ORG_COLOR_KEY = 'sytue-color-org';

/**
 * La FORMA es el tercer eje, aparte del modo y del color: `data-forma` en <html>.
 * «Marco» es la forma de la casa y se dice con la hoja SIN atributo; cada alternativa
 * es un bloque `[data-forma='…']` de appearance.css. `init.test.ts` falla si esta lista
 * y la hoja dejan de decir lo mismo.
 */
export const THEME_FORMAS = ['marco', 'carril'] as const;
export type ThemeForma = (typeof THEME_FORMAS)[number];
export const DEFAULT_FORMA: ThemeForma = 'marco';
/** La elección de la persona: misma cookie de apex y mismo almacenamiento que el modo. */
export const FORMA_KEY = 'sytue-forma';
/** La forma de su organización, que escribe el BFF con la sesión. */
export const ORG_FORMA_KEY = 'sytue-forma-org';

/**
 * La preferencia compartida manda sobre el antiguo almacenamiento por origen.
 *
 * Un solo lector para el modo y para el color: `allowed` es lo único que cambia,
 * y va como parámetro con valor por defecto —no como constante del módulo— porque
 * la función viaja por `toString()` al script del primer pintado.
 */
export function readThemePreference<T extends string = ThemeMode>(
  fallback: T,
  key: string,
  allowed: readonly string[] = ['light', 'dark', 'translucent', 'system'],
): T {
  const valid = (value: string | null | undefined): value is T =>
    typeof value === 'string' && allowed.includes(value);
  try {
    const entry =
      typeof document === 'undefined'
        ? undefined
        : document.cookie
            ?.split(';')
            .map((value) => value.trim())
            .find((value) => value.startsWith(`${key}=`));
    const shared = entry?.slice(key.length + 1);
    if (valid(shared)) return shared;
  } catch {
    // Un documento sin cookies todavía puede conservar la elección local.
  }
  try {
    const stored = typeof localStorage === 'undefined' ? null : localStorage.getItem(key);
    if (valid(stored)) return stored;
  } catch {
    // El modo predeterminado también funciona con almacenamiento bloqueado.
  }
  return fallback;
}

/** Aplica la elección sin resolver «sistema», que continúa siguiendo la media query. */
export function applyThemeMode(mode: ThemeMode): void {
  if (typeof document === 'undefined') return;
  if (mode === 'system') document.documentElement.removeAttribute('data-theme');
  else document.documentElement.setAttribute('data-theme', mode);
}

/** El color de la casa es la hoja sin atributo: elegirlo es quitarlo. */
export function applyThemeColor(color: ThemeColor): void {
  if (typeof document === 'undefined') return;
  if (color === 'petroleo') document.documentElement.removeAttribute('data-color');
  else document.documentElement.setAttribute('data-color', color);
}

/**
 * Qué app es esta, para «color de cada app» (ADR 0072). Quien ya lo dice en su `<html>`
 * manda: Docus, Calcus, Diapos y CertiPDF comparten `/docs`, y su servidor pone ahí el
 * producto que se está viendo. Si no, el prefijo bajo el que se sirve (`/mail`), que
 * `basePathScript` publica justo antes de este script. Una superficie servida en la
 * RAÍZ publica el prefijo vacío —Sytue Admin en `platform.<dominio>`— y lo dice la
 * primera etiqueta de su host. Sin prefijo publicado —el IdP, una página
 * autocontenida— no hay atributo y la hoja pinta el color de la casa.
 */
export function applyThemeApp(): void {
  if (typeof document === 'undefined' || document.documentElement.hasAttribute('data-app')) return;
  const g = globalThis as { __sytueBase?: unknown; location?: { hostname?: string } };
  if (typeof g.__sytueBase !== 'string') return;
  const app = g.__sytueBase.replace(/^\/+/, '') || (g.location?.hostname ?? '').split('.')[0] || '';
  if (/^[a-z]+$/.test(app)) document.documentElement.setAttribute('data-app', app);
}

/**
 * El color CSS de la muestra de cada opción del selector. Petróleo y cobalto tienen
 * un token fijo en la hoja; «el de cada app» no es UN color, así que su muestra son
 * cuatro colores de producto en cuartos, sin fundido.
 */
export function themeColorSwatch(color: ThemeColor): string {
  return color === 'app'
    ? 'conic-gradient(var(--sytue-product-mail) 0 25%, var(--sytue-product-drive) 0 50%, var(--sytue-product-docs) 0 75%, var(--sytue-product-calendar) 0)'
    : `var(--sytue-swatch-${color})`;
}

/** La forma de la casa es la hoja sin atributo: elegirla es quitarlo. */
export function applyThemeForma(forma: ThemeForma): void {
  if (typeof document === 'undefined') return;
  if (forma === 'marco') document.documentElement.removeAttribute('data-forma');
  else document.documentElement.setAttribute('data-forma', forma);
}

// Las cinco funciones no capturan dependencias: el MISMO lector funciona antes del
// primer pintado en Astro, el IdP y los cascarones móviles sin cargar un bundle.
const colors = JSON.stringify(THEME_COLORS);
const formas = JSON.stringify(THEME_FORMAS);
export const themeInitScript = `(function(read,mode,color,forma,app){mode(read('light','${STORAGE_KEY}'));color(read(read('${DEFAULT_COLOR}','${ORG_COLOR_KEY}',${colors}),'${COLOR_KEY}',${colors}));forma(read(read('${DEFAULT_FORMA}','${ORG_FORMA_KEY}',${formas}),'${FORMA_KEY}',${formas}));app();})(${readThemePreference.toString()},${applyThemeMode.toString()},${applyThemeColor.toString()},${applyThemeForma.toString()},${applyThemeApp.toString()});`;
