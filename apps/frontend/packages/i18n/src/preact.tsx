/**
 * Binding de Preact: el idioma activo como contexto, para que un componente pida
 * `t` sin que nadie le encadene props. Vive aparte del core (`@sytue/i18n`) para
 * que el catálogo y la negociación se puedan usar donde no hay Preact: middleware
 * de Astro, BFF, correos transaccionales.
 */
import { createContext, type ComponentChildren, type JSX } from 'preact';
import { useContext, useEffect, useMemo, useState } from 'preact/hooks';
import {
  applyDocumentLocale,
  detectLocale,
  persistLocale,
  readLocaleCookie,
  type Locale,
} from './locale.js';
import { createTranslate, type Translate } from './translate.js';

export interface I18nValue {
  /** Idioma activo. */
  locale: Locale;
  /** Elección explícita del navegador; null sigue la negociación del servidor. */
  preference: Locale | null;
  /** Traduce una clave del catálogo. */
  t: Translate;
  /** Cambia el idioma del usuario (persiste la preferencia y recarga). */
  setLocale: (next: Locale | null) => void;
}

const I18nContext = createContext<I18nValue | null>(null);

export interface I18nProviderProps {
  /**
   * Idioma ya negociado en el servidor (`negotiateLocale` con las cabeceras de la
   * petición). Si se omite, la isla lo deduce del `<html lang>` al hidratar; en
   * SSR eso significa renderizar en inglés y corregir en el cliente, así que el
   * layout DEBE pasarlo en cuanto la app tenga negociación de idioma.
   */
  locale?: Locale | undefined;
  children?: ComponentChildren;
}

function buildValue(
  locale: Locale,
  preference: Locale | null,
  onChange?: (next: Locale) => void,
): I18nValue {
  return {
    locale,
    preference,
    t: createTranslate(locale),
    setLocale: (next) => {
      if (next === (typeof document === 'undefined' ? null : readLocaleCookie(document.cookie)))
        return;
      persistLocale(next);
      if (next !== null) {
        applyDocumentLocale(next);
        onChange?.(next);
      }
      // «Sistema» sólo borra la preferencia. El SSR vuelve a negociar con
      // Accept-Language; no se adivina desde navigator ni se cambia la ruta.
      // El HTML que pintó el servidor sigue en el idioma anterior: solo las islas
      // se re-renderizarían. Recargar es lo único honesto mientras haya contenido
      // estático fuera del catálogo.
      globalThis.location?.reload();
    },
  };
}

export function I18nProvider({ locale, children }: I18nProviderProps): JSX.Element {
  // El idioma del servidor no cambia en vida de la página: se lee una vez y a
  // partir de ahí manda lo que el usuario elija en el selector.
  const [current, setCurrent] = useState<Locale>(() => locale ?? detectLocale());
  // La cookie no existe en SSR. Se adopta después de hidratar para que el control
  // no conserve el «Sistema» del HTML inicial al haber una preferencia explícita.
  const [preference, setPreference] = useState<Locale | null>(null);
  useEffect(() => {
    setPreference(readLocaleCookie(document.cookie));
  }, []);
  const value = useMemo<I18nValue>(
    () => buildValue(current, preference, setCurrent),
    [current, preference],
  );

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

// Sin proveedor NO se lanza un error: un componente de @sytue/ui puede montarse
// en una página que aún no tiene shell (login, error 500, sitio público) y ahí lo
// correcto es hablar el idioma que ya declara el `<html lang>`, no reventar.
//
// Se cachea, porque en una página real ese idioma solo cambia recargando y el
// valor debe mantener su identidad entre renders (memos aguas abajo). Pero el
// caché se ata al `lang` y a la preferencia que lo generaron: era un singleton de por vida y en la
// batería de tests —un único proceso— el primer fichero que montaba un
// componente del DS le fijaba el idioma a todos los demás.
let orphan: I18nValue | null = null;
let orphanLang: string | null = null;

/** El valor huérfano para `lang`, creado una vez por idioma. Idempotente: dos llamadas
 *  con el mismo `lang` devuelven el MISMO objeto, así que el hook que lo pide sigue
 *  siendo puro aunque el caché viva a nivel de módulo. */
function valorHuerfano(lang: string): I18nValue {
  if (orphan === null || lang !== orphanLang) {
    orphanLang = lang;
    orphan = buildValue(
      detectLocale(),
      typeof document === 'undefined' ? null : readLocaleCookie(document.cookie),
    );
  }
  return orphan;
}

export function useI18n(): I18nValue {
  const ctx = useContext(I18nContext);
  if (ctx !== null) return ctx;
  return valorHuerfano(
    typeof document === 'undefined'
      ? ''
      : `${document.documentElement.lang}\0${readLocaleCookie(document.cookie) ?? ''}`,
  );
}

/** Atajo para el caso normal: solo traducir. */
export function useT(): Translate {
  return useI18n().t;
}
