/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los dos ficheros con los que Android e iOS se creen que la app de Sytue Rooms es
 * dueña de `https://rooms.<dominio>/`: con ellos, pulsar un enlace de reunión
 * (`?rooms=<id>`) abre la APP sin pasar por el navegador (App Links / Universal Links).
 * Sin ellos el sistema no verifica el dominio y el enlace se abre en la web, que es lo
 * que tiene que pasar si la app no está.
 *
 * Los datos de firma NO son del código: la huella del certificado con el que se firma
 * el APK y el Team ID de Apple los pone quien despliega
 * (`SYTUE_ANDROID_CERT_SHA256`, varias separadas por comas; `SYTUE_IOS_TEAM_ID`). Sin
 * ellos, Android recibe una lista vacía (no verifica) e iOS un 404.
 */

import { serverEnv } from '@sytue/auth/server';

/** El identificador de la app en los dos sistemas: el `ID` de su FyneApp.toml. */
export const ID_APP = 'eu.sytue.rooms';

export function assetLinks(huellas: string | undefined): unknown[] {
  const lista = (huellas ?? '')
    .split(',')
    .map((h) => h.trim().toUpperCase())
    .filter((h) => /^([0-9A-F]{2}:){31}[0-9A-F]{2}$/.test(h));
  if (lista.length === 0) return [];
  return [
    {
      relation: ['delegate_permission/common.handle_all_urls'],
      target: { namespace: 'android_app', package_name: ID_APP, sha256_cert_fingerprints: lista },
    },
  ];
}

/**
 * `apple-app-site-association`: solo la raíz con `?rooms=`, que es el enlace de una
 * reunión. El resto de rooms.<dominio> sigue siendo la web.
 */
export function asociacionApple(equipo: string | undefined): Record<string, unknown> | null {
  const t = (equipo ?? '').trim();
  if (!/^[A-Z0-9]{10}$/.test(t)) return null;
  return {
    applinks: {
      details: [
        {
          appIDs: [`${t}.${ID_APP}`],
          // La raíz, que es la que se reparte, y el prefijo al que la redirige Traefik.
          components: ['/', '/rooms/'].map((ruta) => ({ '/': ruta, '?': { rooms: '?*' } })),
        },
      ],
    },
  };
}

/** Los dos ficheros con los datos de firma del entorno del despliegue. */
export function asociacionDelEntorno(): {
  android: unknown[];
  apple: Record<string, unknown> | null;
} {
  const env = serverEnv(import.meta.env);
  return {
    android: assetLinks(env.SYTUE_ANDROID_CERT_SHA256),
    apple: asociacionApple(env.SYTUE_IOS_TEAM_ID),
  };
}
