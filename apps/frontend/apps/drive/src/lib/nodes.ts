/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El mapeo de la vista plana del servicio Go («nodeView») a DriveResource vive en
 * el SDK (`nodeResource`): lo necesita también el selector de Notepads, que busca
 * en Drive sin pasar por este BFF. Aquí se reexporta con el nombre de siempre.
 */
export { nodeResource as toResource, type RawNode } from '@sytue/api-clients';

/**
 * ¿Es un fichero cuyo contenido se puede leer como texto?
 *
 * Vive AQUÍ y no en el handler que lo usa para poder probarse: importar
 * `pages/api/ai.ts` arrastra la configuración OIDC y esta pregunta no depende
 * de ninguna sesión.
 *
 * La lista es de MEDIA TYPES y no de extensiones porque el servicio guarda el
 * tipo al subir y es lo que de verdad describe el contenido: un `.txt`
 * renombrado a `.dat` sigue siendo texto.
 */
export function esTexto(tipo: string): boolean {
  const t = tipo.split(';')[0]?.trim().toLowerCase() ?? '';
  if (t.startsWith('text/')) return true;
  return [
    'application/json',
    'application/xml',
    'application/x-yaml',
    'application/yaml',
    'application/javascript',
    'application/sql',
  ].includes(t);
}
