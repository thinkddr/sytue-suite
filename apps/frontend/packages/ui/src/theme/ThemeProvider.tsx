/**
 * ThemeProvider — gestion de modo claro/oscuro de la suite.
 *
 * Aplica `data-theme` en <html>, persiste la eleccion del usuario y respeta
 * `prefers-color-scheme` cuando el modo es "system". El toggle vive en el menu
 * de cuenta de @sytue/shell y consume este contexto. docs/design.md §8.
 */
import { createContext, type ComponentChildren } from 'preact';
import { useCallback, useContext, useEffect, useMemo, useState } from 'preact/hooks';

import {
  COLOR_KEY,
  DEFAULT_COLOR,
  DEFAULT_FORMA,
  FORMA_KEY,
  ORG_COLOR_KEY,
  ORG_FORMA_KEY,
  STORAGE_KEY,
  THEME_COLORS,
  THEME_FORMAS,
  applyThemeColor,
  applyThemeForma,
  applyThemeMode,
  readThemePreference,
  type ThemeColor,
  type ThemeForma,
  type ThemeMode,
} from './init.js';
import { observeThemePreference, saveThemePreference } from './preference.js';
export type { ThemeColor, ThemeForma, ThemeMode } from './init.js';
export {
  DEFAULT_COLOR,
  DEFAULT_FORMA,
  THEME_COLORS,
  THEME_FORMAS,
  themeColorSwatch,
} from './init.js';
export type ResolvedTheme = 'light' | 'dark' | 'translucent';
export { themeInitScript } from './init.js';

interface ThemeContextValue {
  mode: ThemeMode;
  resolved: ResolvedTheme;
  setMode: (mode: ThemeMode) => void;
  toggle: () => void;
  /** El color en vigor: el elegido por la persona o, si no ha elegido, el de su organización. */
  color: ThemeColor;
  /** Lo que la persona ha elegido; `null` si sigue a su organización. */
  colorChoice: ThemeColor | null;
  /** El de su organización, o el de la casa si no ha fijado ninguno. */
  orgColor: ThemeColor;
  /** Con `null` olvida la elección y vuelve a seguir a su organización. */
  setColor: (color: ThemeColor | null) => void;
  /** La forma en vigor, y lo mismo que con el color. */
  forma: ThemeForma;
  formaChoice: ThemeForma | null;
  orgForma: ThemeForma;
  setForma: (forma: ThemeForma | null) => void;
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

function prefersDark(): boolean {
  return typeof window !== 'undefined' && window.matchMedia('(prefers-color-scheme: dark)').matches;
}

function resolve(mode: ThemeMode): ResolvedTheme {
  if (mode === 'system') return prefersDark() ? 'dark' : 'light';
  return mode;
}

/** Lo guardado de un eje: la elección de la persona (o `null`) y lo de su organización. */
function leerEje<T extends string>(
  key: string,
  orgKey: string,
  valores: readonly T[],
  casa: T,
): { elegido: T | null; org: T } {
  return {
    elegido: readThemePreference<T | ''>('', key, valores) || null,
    org: readThemePreference(casa, orgKey, valores),
  };
}

/**
 * Un eje que la organización puede fijar por defecto —el color, la forma—: manda la
 * persona; si no ha elegido, su organización; si no, la casa. Es el mismo orden que
 * aplica el script del primer pintado, y aquí va UNA vez para los dos ejes.
 */
function useEje<T extends string>(
  key: string,
  orgKey: string,
  valores: readonly T[],
  casa: T,
  aplicar: (valor: T) => void,
): { valor: T; elegido: T | null; org: T; elegir: (valor: T | null) => void } {
  const [estado, setEstado] = useState(() => leerEje(key, orgKey, valores, casa));
  const valor = estado.elegido ?? estado.org;
  const elegir = useCallback(
    (elegido: T | null) => {
      setEstado((actual) => ({ ...actual, elegido }));
      saveThemePreference(elegido, key);
    },
    [key],
  );
  useEffect(() => {
    aplicar(valor);
  }, [aplicar, valor]);
  // Otra pestaña u otra app lo cambian por la cookie compartida, y el de la
  // organización llega con la sesión.
  useEffect(
    () => observeThemePreference(() => setEstado(leerEje(key, orgKey, valores, casa)), key),
    [key, orgKey, valores, casa],
  );
  return { valor, elegido: estado.elegido, org: estado.org, elegir };
}

export interface ThemeProviderProps {
  children: ComponentChildren;
  /** Modo inicial si no hay valor persistido. */
  defaultMode?: ThemeMode;
}

export function ThemeProvider({ children, defaultMode = 'light' }: ThemeProviderProps) {
  const [mode, setModeState] = useState<ThemeMode>(() =>
    readThemePreference(defaultMode, STORAGE_KEY),
  );
  const [systemDark, setSystemDark] = useState<boolean>(() => prefersDark());
  const color = useEje(COLOR_KEY, ORG_COLOR_KEY, THEME_COLORS, DEFAULT_COLOR, applyThemeColor);
  const forma = useEje(FORMA_KEY, ORG_FORMA_KEY, THEME_FORMAS, DEFAULT_FORMA, applyThemeForma);

  const setMode = useCallback((next: ThemeMode) => {
    setModeState(next);
    saveThemePreference(next);
    applyThemeMode(next);
  }, []);

  const toggle = useCallback(() => {
    setModeState((current) => {
      const next: ThemeMode = resolve(current) === 'dark' ? 'light' : 'dark';
      saveThemePreference(next);
      applyThemeMode(next);
      return next;
    });
  }, []);

  // Reaccionar a cambios del sistema cuando el modo es "system".
  useEffect(() => {
    if (typeof window === 'undefined') return;
    const mq = window.matchMedia('(prefers-color-scheme: dark)');
    const onChange = (e: MediaQueryListEvent) => setSystemDark(e.matches);
    mq.addEventListener('change', onChange);
    return () => mq.removeEventListener('change', onChange);
  }, []);

  // Sincronizar el atributo al montar / al cambiar de modo.
  useEffect(() => {
    applyThemeMode(mode);
  }, [mode]);

  useEffect(
    () => observeThemePreference(() => setModeState(readThemePreference(defaultMode, STORAGE_KEY))),
    [defaultMode],
  );

  const resolved: ResolvedTheme = mode === 'system' ? (systemDark ? 'dark' : 'light') : mode;

  const value = useMemo<ThemeContextValue>(
    () => ({
      mode,
      resolved,
      setMode,
      toggle,
      color: color.valor,
      colorChoice: color.elegido,
      orgColor: color.org,
      setColor: color.elegir,
      forma: forma.valor,
      formaChoice: forma.elegido,
      orgForma: forma.org,
      setForma: forma.elegir,
    }),
    [
      mode,
      resolved,
      setMode,
      toggle,
      color.valor,
      color.elegido,
      color.org,
      color.elegir,
      forma.valor,
      forma.elegido,
      forma.org,
      forma.elegir,
    ],
  );

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext);
  if (ctx === null) {
    throw new Error('useTheme debe usarse dentro de <ThemeProvider>.');
  }
  return ctx;
}
