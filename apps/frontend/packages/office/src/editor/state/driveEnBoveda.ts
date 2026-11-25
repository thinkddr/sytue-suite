/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * ¿EL DRIVE DE ESTA PERSONA ESTÁ EN LA BÓVEDA? Lo que el editor pregunta al abrir un
 * documento, sin abrir nada: el estado de la fuente por el BFF.
 *
 * Va aparte de `driveCifrado.ts` porque aquel trae el Drive de la bóveda entero
 * (`@sytue/offline/matrix` y su repositorio, ~183 KB sin comprimir) y el editor lo
 * importaba para hacer solo esta pregunta: ahora lo baja con `import()` al guardar en un
 * Drive que está en la bóveda (`useDocument`, `guardarCifrado`).
 */
import { DriveClient } from '@sytue/api-clients';
import type { MatrixStorageScope } from '@sytue/config/matrix';
import type { PrivateDriveSource } from '@sytue/offline/vault/drive';
import { withBase } from '@sytue/ui';

/** El prefijo de las rutas del BFF de la ofimática para la bóveda de Drive. */
export const RUTA_DRIVE_CIFRADO = '/api/drive-cifrado';

/** La ruta del BFF de la bóveda de Drive en esta app. */
export const baseDelBff = (): string =>
  withBase(import.meta.env.BASE_URL ?? '/', RUTA_DRIVE_CIFRADO);

/** El cliente de Drive del navegador, por el BFF de la bóveda. */
export function fuenteDelNavegador(): PrivateDriveSource {
  return new DriveClient({ baseUrl: window.location.origin + baseDelBff() });
}

/**
 * ¿El Drive del servidor de esta persona ya no admite escrituras (está, o está
 * pasando, a la bóveda)? Solo lee el estado de la fuente, sin claves. Ante la duda
 * (sin organización, error de red) contesta que no: el editor sigue como siempre.
 */
export async function driveEnBoveda(
  actor: string,
  tenant: string | undefined,
  fuente: () => Pick<PrivateDriveSource, 'privateStorageState'> = fuenteDelNavegador,
  signal?: AbortSignal,
): Promise<boolean> {
  if (!tenant || !actor) return false;
  const scope: MatrixStorageScope = { tenant, subject: actor, product: 'drive' };
  try {
    return (await fuente().privateStorageState(scope, signal)).mode !== 'legacy';
  } catch {
    return false;
  }
}
