// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// Pasa el correo de la cuenta de auditoría a la bóveda y comprueba que abre.
//
// ESCRIBE EN PRODUCCIÓN, autorizado por el dueño el 2026-11-27: es lo que hace el botón
// «Pasar a la bóveda» de Mail (vacía el correo privado de la cuenta en Matrix y abre una
// bóveda nueva). Hace falta porque la cuenta abría su correo por Matrix y la puerta de la
// bóveda no se medía: v4.0.2 salió con ella rota y nada lo vio (ver
// `repro-mail-desbloqueo.mjs` y `almacenCorreo` en tests/puerta-cifrado.ts).
//
// Solo traspasa si producción ya sirve el arreglo de v4.0.3; si no, se para sin tocar
// nada. Con la cuenta ya en la bóveda, solo comprueba que abre. Fichero aparte porque el
// paso `reproductor` de `mobile.yml` solo acepta un nombre:
//
//   reproductor=traspaso-auditoria-boveda.mjs
process.env.SYTUE_TRASPASO_BOVEDA = '1';
await import('./repro-mail-desbloqueo.mjs');
