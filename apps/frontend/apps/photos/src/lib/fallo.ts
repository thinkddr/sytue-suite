/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Qué se le dice a la persona cuando algo falla, en SU idioma. Espejo de
 * `estadoDeFallo` y `textoDeFallo` de Fyne (apps/fyne/apps/photos/app.go).
 *
 * Las ocho pantallas pintaban `String(e)`: «Error: Sytue Fotos respondió 500» en
 * castellano para una persona que usa la suite en alemán, y sin ninguna salida.
 * El error lleva su estado HTTP (`ErrorDeFotos`) y aquí se decide qué significa:
 * la sesión, el permiso, el servicio o la red. El texto sale de las claves comunes
 * de `@sytue/i18n`, las mismas que usa la app nativa.
 */
import { translate, type Locale } from '@sytue/i18n';
import { ErrorDeFotos } from '../client/photosApi.js';

/** Los cuatro estados de fallo de una pantalla; los mismos nombres que en Fyne. */
export type EstadoDeFallo = 'sinSesion' | 'sinPermiso' | 'error' | 'sinConexion';

export function estadoDeFallo(e: unknown): EstadoDeFallo {
  // Un 401 es la sesión, no el permiso: lo que toca es volver a entrar, y decir
  // «no tienes acceso» manda a buscar un permiso que la persona sí tiene.
  if (e instanceof ErrorDeFotos) {
    if (e.status === 401) return 'sinSesion';
    if (e.status === 403) return 'sinPermiso';
    return 'error';
  }
  // `fetch` rechaza con un TypeError cuando no contesta nadie: eso es la red, no el
  // servicio. Lo demás sin estado (un JSON que no lo es, una imagen que no se
  // decodifica) es una avería: decir «sin conexión» mandaría a revisar una red
  // que funciona. En Fyne todo lo que no es `api.Error` es red porque allí el
  // cliente HTTP no lanza otra cosa; aquí el navegador sí.
  if (e instanceof TypeError) return 'sinConexion';
  if (typeof navigator !== 'undefined' && navigator.onLine === false) return 'sinConexion';
  return 'error';
}

/**
 * El texto de un fallo que NO vacía la pantalla (una favorita que no se guardó,
 * una foto que no se fue a la papelera): va en un aviso.
 */
export function textoDeFallo(locale: Locale, e: unknown): string {
  // 507: lo que se añade a un álbum de otra persona se copia a su Drive, y no cabe.
  if (e instanceof ErrorDeFotos && e.status === 507) {
    return translate(locale, 'sharedAlbum.quotaFull');
  }
  switch (estadoDeFallo(e)) {
    case 'sinConexion':
      return translate(locale, 'offline.description');
    case 'sinPermiso':
      return translate(locale, 'forbidden.description');
    default:
      return translate(locale, 'error.connection');
  }
}

/** Qué ofrece la pantalla vacía de un fallo de carga. */
export type SalidaDeFallo = 'reintentar' | 'entrar' | null;

export interface PantallaDeFallo {
  estado: EstadoDeFallo;
  titulo: string;
  descripcion?: string;
  salida: SalidaDeFallo;
  /** El rótulo del botón de la salida, si la hay. */
  rotulo?: string;
}

/**
 * Lo que pinta un fallo de CARGA (la pantalla se queda sin nada que enseñar).
 *
 * Reintentar solo donde reintentar puede servir: el servicio que falló o la red
 * que se fue. Sin permiso, volver a pedir lo mismo da lo mismo; sin sesión, la
 * salida es entrar. `tituloDeApp` es el de la sesión, como en Fyne: la persona
 * no ha hecho nada mal, solo tiene que volver a entrar en Sytue Fotos.
 */
export function pantallaDeFallo(locale: Locale, e: unknown, tituloDeApp: string): PantallaDeFallo {
  const estado = estadoDeFallo(e);
  const reintentar = translate(locale, 'action.retry');
  switch (estado) {
    case 'sinSesion':
      return {
        estado,
        titulo: tituloDeApp,
        salida: 'entrar',
        rotulo: translate(locale, 'account.signIn'),
      };
    case 'sinPermiso':
      return {
        estado,
        titulo: translate(locale, 'forbidden.title'),
        descripcion: translate(locale, 'forbidden.description'),
        salida: null,
      };
    case 'sinConexion':
      return {
        estado,
        titulo: translate(locale, 'offline.status'),
        descripcion: translate(locale, 'offline.description'),
        salida: 'reintentar',
        rotulo: reintentar,
      };
    default:
      return {
        estado,
        titulo: textoDeFallo(locale, e),
        salida: 'reintentar',
        rotulo: reintentar,
      };
  }
}
