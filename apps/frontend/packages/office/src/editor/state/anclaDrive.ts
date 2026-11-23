/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL ANCLA DE UN DOCUMENTO A SU FICHERO (`source_ref`). Hay dos, y no se mezclan:
 *
 * - «drive:<uuid>»: un fichero del Drive del SERVIDOR. Lo pone el servicio al crear
 *   (hogar_en_drive.go) o al abrir un fichero de Drive con Sytue Docs.
 * - «vault:<uuid>»: un fichero del Drive de la BÓVEDA (ADR 0086), cifrado de extremo
 *   a extremo. Lo escribe el navegador al «Guardar en Drive» y el servicio solo
 *   guarda el ancla, nunca el contenido.
 *
 * Los ids del Drive del servidor que pasaron a la bóveda conservan su id (la copia
 * de `PrivateDriveRepository.migrate` es por `source_id`), así que un «drive:<uuid>»
 * de antes de pasar a la bóveda puede seguir apuntando al mismo fichero allí. Uno
 * creado en la bóveda no existe en el servidor: por eso lleva otro prefijo y nada
 * del servidor (papelera, «abrir desde Drive») lo trata como un fichero suyo.
 */

export const PREFIJO_DRIVE = 'drive:';
export const PREFIJO_BOVEDA = 'vault:';

/** El uuid canónico (minúsculas) de los ids de Drive, del servidor y de la bóveda. */
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

export type AnclaDrive = { donde: 'servidor' | 'boveda'; id: string };

/** Lee un `source_ref`; `null` si no ancla a ningún fichero de Drive reconocible. */
export function leerAncla(ref: string): AnclaDrive | null {
  if (ref.startsWith(PREFIJO_DRIVE)) {
    const id = ref.slice(PREFIJO_DRIVE.length);
    return UUID.test(id) ? { donde: 'servidor', id } : null;
  }
  if (ref.startsWith(PREFIJO_BOVEDA)) {
    const id = ref.slice(PREFIJO_BOVEDA.length);
    return UUID.test(id) ? { donde: 'boveda', id } : null;
  }
  return null;
}

/** El ancla de un fichero de la bóveda. Lanza si el id no es un uuid canónico. */
export function anclaDeBoveda(id: string): string {
  if (!UUID.test(id)) throw new Error('vault_anchor_invalid');
  return PREFIJO_BOVEDA + id;
}

export function esAnclaDeBoveda(ref: string): boolean {
  return leerAncla(ref)?.donde === 'boveda';
}

/**
 * ¿Se puede cambiar el ancla `antes` por una de la bóveda? Solo si no había (el
 * documento nació sin fichero porque el servidor no podía escribir) o si ya era de
 * la bóveda. Un «drive:» no se sustituye: el servicio lo usa para la papelera de
 * Drive, y si el Drive volviera a estar en el servidor seguiría haciendo falta.
 * El servicio aplica la misma regla.
 */
export function sePuedeAnclarEnBoveda(antes: string): boolean {
  return antes === '' || esAnclaDeBoveda(antes);
}
