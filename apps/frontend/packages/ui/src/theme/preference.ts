import { suiteApex } from '@sytue/config';
import { STORAGE_KEY } from './init.js';

/** Solo compartir preferencias dentro del dominio real de esta instalación. */
export function sharedThemeDomain(hostname: string, apex: string): string | null {
  const host = hostname.toLowerCase();
  const domain = apex.toLowerCase();
  return domain && (host === domain || host.endsWith(`.${domain}`)) ? domain : null;
}

/**
 * La cookie no contiene identidad: comparte únicamente la elección de apariencia.
 * El modo, el color y la forma se guardan igual, cada uno bajo su clave. Con `value`
 * nulo la elección se OLVIDA —cookie y almacenamiento—, que es como una persona
 * vuelve a seguir a su organización.
 */
export function saveThemePreference(value: string | null, key: string = STORAGE_KEY): void {
  if (typeof document !== 'undefined' && typeof location !== 'undefined') {
    try {
      const domain = sharedThemeDomain(location.hostname, suiteApex);
      document.cookie = `${key}=${value ?? ''}; Path=/; Max-Age=${
        value === null ? 0 : 31536000
      }; SameSite=Lax${location.protocol === 'https:' ? '; Secure' : ''}${
        domain ? `; Domain=${domain}` : ''
      }`;
    } catch {
      // Si el navegador rechaza cookies, la elección sigue disponible en esta app.
    }
  }
  try {
    if (typeof localStorage === 'undefined') return;
    if (value === null) localStorage.removeItem(key);
    else localStorage.setItem(key, value);
  } catch {
    // Guardar una preferencia no debe impedir cambiar el tema en memoria.
  }
}

/** Otras apps avisan por cookie; los navegadores sin Cookie Store releen al volver. */
export function observeThemePreference(change: () => void, key: string = STORAGE_KEY): () => void {
  if (typeof window === 'undefined') return () => {};
  const hasSharedPreference = (): boolean => {
    try {
      return document.cookie.split(';').some((entry) => entry.trim().startsWith(`${key}=`));
    } catch {
      return false;
    }
  };
  let hadSharedPreference = hasSharedPreference();
  const refresh = (): void => {
    const hasShared = hasSharedPreference();
    if (hadSharedPreference && !hasShared) {
      try {
        localStorage.removeItem(key);
      } catch {
        // Borrar la cookie sigue siendo válido con almacenamiento bloqueado.
      }
    }
    hadSharedPreference = hasShared;
    change();
  };
  const storage = (event: StorageEvent): void => {
    if (event.key === key || event.key === null) refresh();
  };
  const visibility = (): void => {
    if (document.visibilityState === 'visible') refresh();
  };
  const cookies = (window as Window & { cookieStore?: EventTarget }).cookieStore;
  window.addEventListener('focus', refresh);
  window.addEventListener('pageshow', refresh);
  window.addEventListener('storage', storage);
  document.addEventListener('visibilitychange', visibility);
  cookies?.addEventListener('change', refresh);
  return () => {
    window.removeEventListener('focus', refresh);
    window.removeEventListener('pageshow', refresh);
    window.removeEventListener('storage', storage);
    document.removeEventListener('visibilitychange', visibility);
    cookies?.removeEventListener('change', refresh);
  };
}
