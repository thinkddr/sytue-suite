/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LA PRIMERA LECTURA DE CADA RECURSO, UNA SOLA VEZ POR PÁGINA.
 *
 * Cada tarjeta de Mi cuenta carga lo suyo al montarse, y varias necesitan lo
 * mismo: `/api/me` lo pedían Perfil, Seguridad y Recuperación, y la revisión de
 * seguridad necesita además sesiones, dispositivos, passkeys y contraseñas de
 * aplicación. Subir toda la carga a `AccountApp` obligaba a pasar cada lista por
 * props y a que cada tarjeta dejara de saber recargarse sola. Aquí se comparte
 * solo la PRIMERA: la promesa de la respuesta se guarda y cada tarjeta lee su
 * propio `clone()`, así que el cuerpo se puede leer tantas veces como tarjetas.
 *
 * Releer tras una acción (cerrar una sesión, revocar una contraseña) o al pulsar
 * «Reintentar» sigue siendo un `fetch` normal: eso sí tiene que ir a la red.
 */
const lecturas = new Map<string, Promise<Response>>();

export function primeraLectura(url: string): Promise<Response> {
  let p = lecturas.get(url);
  if (!p) {
    p = fetch(url);
    lecturas.set(url, p);
  }
  return p.then((r) => r.clone());
}

/** Olvida lo leído. Para las pruebas: cada una monta su página desde cero. */
export function olvidarLecturas(): void {
  lecturas.clear();
}
