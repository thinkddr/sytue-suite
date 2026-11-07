/* SPDX-License-Identifier: Apache-2.0 */
/**
 * Las rutas de `/api/v1/external-mail/` que el BFF de Mail deja pasar al gateway:
 * la lista, el alta, el resumen del lateral y, por cuenta, sus carpetas, mensajes,
 * envío, acciones y la baja. Cualquier otra se corta en el proxy
 * (`pages/api/external-mail/[...ruta].ts`) antes de llegar al gateway.
 */
const PERMITIDAS =
  /^\/api\/v1\/external-mail\/(?:accounts|connect|summary|[0-9a-f-]{36}(?:\/(?:folders|messages|message|send|actions))?)$/;

export function rutaCorreoExternoDenegada(path: string): boolean {
  return !PERMITIDAS.test(path);
}
