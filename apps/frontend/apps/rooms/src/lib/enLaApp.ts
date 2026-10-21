/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El enlace de una reunión (`?rooms=<id>`) se abre EN LA APP de Sytue Rooms si está
 * instalada. En el móvil lo hace el sistema sin pasar por aquí (App Links de Android y
 * Universal Links de iOS: `/.well-known/assetlinks.json` y `apple-app-site-association`).
 * En escritorio no hay forma de saber si la app está: se prueba su esquema con
 * `openInApp` (@sytue/shell), que mira si la página pierde el foco.
 *
 * Probar el esquema SIN preguntar solo es inocuo donde un esquema sin dueño no hace nada:
 * Chromium lo ignora en silencio. Safari enseña «no se puede abrir la página» y Firefox su
 * propia página de error, que es justo el «diálogo roto» que no queremos. Por eso ahí solo
 * se prueba si la persona dijo «abrir siempre en la app»; si no, se le ofrece.
 */
import { openInApp, inNativeShell } from '@sytue/shell';

const CLAVE = 'sytue.rooms.abrirEnLaApp';

/** ¿Pidió «abrir siempre en la app»? */
export function siempreEnLaApp(): boolean {
  try {
    return window.localStorage.getItem(CLAVE) === '1';
  } catch {
    return false;
  }
}

/** Recuerda (o olvida) «abrir siempre en la app». */
export function recordarEnLaApp(si: boolean): void {
  try {
    if (si) window.localStorage.setItem(CLAVE, '1');
    else window.localStorage.removeItem(CLAVE);
  } catch {
    /* Sin almacenamiento (privado, bloqueado): se pregunta la próxima vez, y ya. */
  }
}

/** En el móvil el sistema ya habría abierto la app si estuviera; aquí no se insiste. */
export function esMovil(ua: string = navigator.userAgent): boolean {
  return /Android|iPhone|iPad|iPod/i.test(ua);
}

/** Chromium ignora en silencio un esquema sin dueño. */
export function esChromium(): boolean {
  const datos = (navigator as Navigator & { userAgentData?: { brands?: { brand: string }[] } })
    .userAgentData;
  return Boolean(datos?.brands?.some((b) => b.brand === 'Chromium'));
}

/** Qué hacer al llegar con un enlace de reunión: probar la app, ofrecerla o nada. */
export function queHacer(): 'probar' | 'ofrecer' | 'nada' {
  if (typeof window === 'undefined' || inNativeShell() || esMovil()) return 'nada';
  return siempreEnLaApp() || esChromium() ? 'probar' : 'ofrecer';
}

/**
 * Abre la reunión en la app. `true` si la app se llevó el foco (se abrió). La URL que
 * viaja es la canónica de la propia página: la app la traduce con `api.DesdeLaWeb`.
 */
export function abrirReunionEnLaApp(id: string, clave = ''): Promise<boolean> {
  const u = new URL(window.location.pathname, window.location.origin);
  u.searchParams.set('rooms', id);
  if (clave) u.searchParams.set('clave', clave);
  return openInApp(u.toString(), 'rooms');
}
