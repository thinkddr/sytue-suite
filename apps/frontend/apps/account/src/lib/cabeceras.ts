/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Las cabeceras que el BFF pone al hablar con Sytue ID.
 *
 * EN SU PROPIO FICHERO Y NO EN `bff.ts` porque aquel construye el cliente OIDC
 * AL IMPORTARSE —`createBff(resolveOidcConfig(env))`—, así que traerse de allí
 * una función pura exige `SYTUE_OIDC_ISSUER` en el entorno y no se puede probar
 * sin montar media configuración.
 */

/**
 * Las cabeceras de quien llama, más las que pone el proxy.
 *
 * `Headers` Y NO UN OBJETO LITERAL: `HeadersInit` admite también `string[][]`, y
 * esparcir un array dentro de un objeto da `{0: [...], 1: [...]}` —las cabeceras
 * de quien llama se perderían sin un solo aviso—. `new Headers()` normaliza las
 * tres formas que admite el tipo.
 *
 * El token del proxy se pone con `set` y no con `append`: una petición no puede
 * colar su propia autorización y hablar con Sytue ID en nombre de otro.
 */
export function cabecerasConToken(init: HeadersInit | undefined, token: string): Headers {
  const headers = new Headers(init);
  // NINGUNA COOKIE HACIA SYTUE ID. Esta app se sirve en `id.<dominio>/account`,
  // así que el navegador le manda las cookies del IdP (la de sesión y la de
  // dispositivo): riesgo aceptado, y lo que depende de nosotros es no
  // leerlas ni reenviarlas (docs/security.md#riesgo-aceptado-mi-cuenta-en-el-origen-del-idp).
  // Aquí pasan TODAS las llamadas del BFF al IdP; con el token basta.
  headers.delete('cookie');
  headers.set('authorization', `Bearer ${token}`);
  headers.set('accept', 'application/json');
  return headers;
}
