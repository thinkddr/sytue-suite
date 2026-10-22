/**
 * Helpers del BFF (/api/*) para islas Preact. Centraliza POST JSON y la señal
 * live=false (modo demostración sin backend vivo).
 */

export interface BffOk {
  ok: boolean;
  live: boolean;
}

/**
 * Fallo de una llamada al BFF, con el status y el CÓDIGO reales que devolvió el
 * control-plane. Sin esto el error llegaba como `Error('bff_409')` y la interfaz
 * solo podía decir «ha fallado, inténtalo otra vez» cuando el backend ya había
 * explicado exactamente qué pasaba (nombre cogido, cuota, sin permiso…).
 * Sigue siendo un `Error` con el mismo mensaje, así que los `catch {}` de siempre
 * no cambian de comportamiento.
 */
export class BffError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
  ) {
    super(`bff_${status}`);
    this.name = 'BffError';
  }
}

/** POST JSON al BFF; lanza BffError si !res.ok. */
export async function postBff<T extends BffOk>(
  url: string,
  body: Record<string, unknown>,
): Promise<T> {
  const res = await fetch(url, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    // El BFF responde {"error":"<código>"} con el código REAL del control-plane; si
    // el cuerpo no es eso (proxy caído, HTML de error), queda el status.
    const code = await res
      .json()
      .then((b) => (b as { error?: unknown }).error)
      .catch(() => undefined);
    throw new BffError(res.status, typeof code === 'string' && code ? code : 'backend_error');
  }
  return (await res.json()) as T;
}

import type { T } from '../i18n/index.js';

export interface ToastLike {
  notify: (opts: {
    tone: 'info' | 'success' | 'danger';
    title: string;
    description?: string;
  }) => void;
}

/** Avisa cuando la mutación solo afectó al estado local (backend no conectado). */
export function notifyDemoIfNeeded(t: T, toast: ToastLike, live: boolean): void {
  if (live) return;
  toast.notify({
    tone: 'info',
    title: t('toast.demoMode.title'),
    description: t('toast.demoMode.desc'),
  });
}

/**
 * Prop `description` honesta para el toast de éxito de una mutación, lista para
 * esparcir en `toast.notify({...})`. Con backend vivo usa la descripción real (o
 * la omite si no hay); en modo demostración NO afirma el cambio (mismo patrón
 * que DeploymentsPanel): devuelve el aviso neutral. Devuelve un objeto para no
 * asignar `description: undefined` (rechazado por exactOptionalPropertyTypes).
 */
export function demoDesc(t: T, live: boolean, liveDescription?: string): { description?: string } {
  const description = live ? liveDescription : t('toast.demoSimulated');
  return description ? { description } : {};
}
