import type { SytueUser } from '@sytue/auth';
import { isStorageOAuthKind } from '@sytue/api-clients';

type StorageFetch = (url: string, init?: RequestInit) => Promise<Response>;

/**
 * La puerta de Drive exige owner/admin; la cuenta individual es owner de su
 * organización personal. Su plan no cierra esta función, aunque sí cierre Admin.
 * Drive confirma además que el despliegue permite conectar almacenamiento.
 */
export async function almacenamientoExternoDisponible(
  user: Pick<SytueUser, 'roles'>,
  accessToken: string,
  driveUrl: string,
  fetcher: StorageFetch = fetch,
): Promise<boolean> {
  if (!user.roles.some((role) => role === 'owner' || role === 'admin')) return false;
  try {
    const base = `${driveUrl.replace(/\/+$/, '')}/api/v1/storage/external`;
    const request = {
      headers: { authorization: `Bearer ${accessToken}` },
      signal: AbortSignal.timeout(1500),
    };
    const res = await fetcher(base, request);
    if (!res.ok) return false;
    const state = (await res.json()) as { available?: unknown };
    if (state.available === true) return true;
    if (state.available !== false) return false;
    // Drive seals WebDAV/S3 credentials. OAuth accounts use the gateway vault,
    // and its capability flags already include the organisation opt-out policy.
    const accounts = await fetcher(`${base}/instances`, request);
    if (!accounts.ok) return false;
    const { providers } = (await accounts.json()) as { providers?: Record<string, unknown> };
    return Object.entries(providers ?? {}).some(
      ([provider, available]) => isStorageOAuthKind(provider) && available === true,
    );
  } catch {
    return false;
  }
}
