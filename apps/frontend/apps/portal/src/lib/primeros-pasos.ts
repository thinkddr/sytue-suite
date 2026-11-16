/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «PRIMEROS PASOS» DE LA PORTADA: CARGA EN EL SERVIDOR Y PREFERENCIAS EN EL NAVEGADOR.
 *
 * Qué pasos quedan lo decide `primerosPasos` de `@sytue/shell` (el mismo criterio
 * que la revisión de seguridad de Mi cuenta). Aquí solo se junta lo que necesita
 * —`/me`, las passkeys y las contraseñas de aplicación de Sytue ID— y se guarda,
 * por persona, lo que solo sabe este navegador: que la tarjeta se ha ocultado y
 * que Sytue One ya está instalada.
 *
 * La carga la usan la portada en SSR (la tarjeta llega pintada y no empuja las
 * apps al hidratar) y `/api/primeros-pasos` (cuando el SSR no pudo).
 */
import { primerosPasos, type PasoPendiente } from '@sytue/shell';

export type { PasoPendiente } from '@sytue/shell';

/**
 * Los pasos que quedan según Sytue ID, sin saber aún si la app está instalada
 * (eso lo dice el navegador). `null` si Sytue ID no ha contestado a NADA: sin
 * datos no hay tarjeta, ni siquiera la de «instalar», que sola sería un anuncio.
 */
export async function cargarPrimerosPasos(
  idBase: string,
  accessToken: string,
  signal?: AbortSignal,
): Promise<PasoPendiente[] | null> {
  const leer = async <T>(ruta: string, campo?: string): Promise<T | null> => {
    try {
      const res = await fetch(`${idBase}${ruta}`, {
        headers: { authorization: `Bearer ${accessToken}` },
        ...(signal ? { signal } : {}),
      });
      if (!res.ok) return null;
      const cuerpo = (await res.json()) as Record<string, unknown>;
      const valor = campo ? cuerpo?.[campo] : cuerpo;
      if (campo && !Array.isArray(valor)) return null;
      return (valor ?? null) as T | null;
    } catch {
      return null;
    }
  };
  const [me, passkeys, contrasenasApp] = await Promise.all([
    leer<{ mfa_enabled?: boolean; recovery_email?: string; phone?: string; has_avatar?: boolean }>(
      '/api/v1/me',
    ),
    leer<unknown[]>('/api/v1/webauthn/credentials', 'credentials'),
    leer<unknown[]>('/api/v1/me/app-passwords', 'app_passwords'),
  ]);
  if (!me && !passkeys && !contrasenasApp) return null;
  return primerosPasos({ me, passkeys, contrasenasApp, instalada: false });
}

/** Lo que este navegador recuerda de la tarjeta, por persona. */
export interface PreferenciasPasos {
  oculto?: boolean;
  instalada?: boolean;
}

/**
 * La clave de `localStorage`: persona (`sub`) y organización. Quien comparte el
 * ordenador, o cambia a otra organización suya, no hereda el «Ocultar» de otro.
 */
export function clavePrimerosPasos(sub: string, tenant: string | null | undefined): string {
  return `sytue.one.primerosPasos:${sub}:${tenant ?? ''}`;
}

export function leerPreferencias(clave: string): PreferenciasPasos {
  try {
    const crudo = JSON.parse(localStorage.getItem(clave) ?? '{}') as unknown;
    if (!crudo || typeof crudo !== 'object') return {};
    const p = crudo as Record<string, unknown>;
    return { oculto: p.oculto === true, instalada: p.instalada === true };
  } catch {
    // Navegación privada, almacenamiento lleno o un valor ajeno: como si no hubiera nada.
    return {};
  }
}

export function guardarPreferencia(clave: string, campo: keyof PreferenciasPasos): void {
  try {
    localStorage.setItem(clave, JSON.stringify({ ...leerPreferencias(clave), [campo]: true }));
  } catch {
    /* sin almacenamiento la preferencia dura lo que la página: no es grave. */
  }
}

/** Abierta como app instalada (PWA en el escritorio o en la pantalla de inicio). */
export function instaladaAqui(): boolean {
  try {
    return window.matchMedia('(display-mode: standalone)').matches;
  } catch {
    return false;
  }
}

/** Los pasos que se enseñan, con lo que sabe el navegador. Vacío ⇒ no hay tarjeta. */
export function pasosVisibles(
  pasos: readonly PasoPendiente[],
  prefs: PreferenciasPasos,
  instalada: boolean,
): PasoPendiente[] {
  if (prefs.oculto) return [];
  return pasos.filter((p) => p.clave !== 'instalar' || !(prefs.instalada || instalada));
}

/** El `id` de la tarjeta: lo busca el guion de antes de pintar. */
export const ID_TARJETA_PASOS = 'primeros-pasos';

/**
 * Guion en línea que va JUSTO DETRÁS de la isla de la portada: aplica las
 * preferencias de este navegador al HTML del SSR antes del primer pintado. Sin
 * él, quien ocultó la tarjeta la vería aparecer y esfumarse en cada visita (el
 * servidor no puede leer `localStorage`). Al hidratar, el componente llega a lo
 * mismo con `pasosVisibles` y Preact quita lo que este guion solo escondió.
 */
export function guionAntesDePintar(clave: string): string {
  // `<` escapado: la clave lleva el `sub` y la organización, y un `</script>` en
  // ellos cerraría el guion en mitad del HTML.
  const k = JSON.stringify(clave).replace(/</g, '\\u003c');
  const id = JSON.stringify(ID_TARJETA_PASOS);
  return (
    `(function(){try{var c=document.getElementById(${id});if(!c)return;` +
    `var p=JSON.parse(localStorage.getItem(${k})||'{}')||{};` +
    `if(p.oculto===true){c.hidden=true;return}` +
    `if(p.instalada===true||matchMedia('(display-mode: standalone)').matches){` +
    `var i=c.querySelector('[data-paso="instalar"]');if(i)i.hidden=true;` +
    `if(!c.querySelector('[data-paso]:not([hidden])'))c.hidden=true}}catch(e){}})();`
  );
}
