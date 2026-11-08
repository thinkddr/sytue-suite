/* SPDX-License-Identifier: Apache-2.0 */
// Clave de recuperación custodiada por el servicio para la persona de la sesión
// OIDC (ADR 0081): quien entra en un dispositivo nuevo no tiene que guardarla ni
// teclearla. La misma forma que valida el servicio (base58 en grupos de cuatro).
import { withBase } from '@sytue/ui';

export const recoveryKeyShape = /^[1-9A-HJ-NP-Za-km-z]{4}(?: [1-9A-HJ-NP-Za-km-z]{1,4}){8,24}$/;

const url = (): string =>
  withBase(import.meta.env.BASE_URL ?? '/rooms', '/api/matrix/recovery-key');

/** La clave custodiada, o null si todavía no hay ninguna (o no se puede leer). */
export async function readEscrowedRecoveryKey(signal?: AbortSignal): Promise<string | null> {
  try {
    const response = await fetch(url(), {
      credentials: 'include',
      cache: 'no-store',
      ...(signal ? { signal } : {}),
    });
    if (!response.ok) return null;
    const { recovery_key } = (await response.json()) as { recovery_key?: unknown };
    return typeof recovery_key === 'string' && recoveryKeyShape.test(recovery_key)
      ? recovery_key
      : null;
  } catch {
    return null;
  }
}

/** Custodia la clave; false si el servicio no la ha guardado. */
export async function escrowRecoveryKey(key: string): Promise<boolean> {
  try {
    const response = await fetch(url(), {
      method: 'POST',
      credentials: 'include',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ recovery_key: key }),
    });
    return response.ok;
  } catch {
    return false;
  }
}
