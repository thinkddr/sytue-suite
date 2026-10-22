import type { SytueUser } from '@sytue/auth';

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
    const res = await fetcher(`${driveUrl.replace(/\/+$/, '')}/api/v1/storage/external`, {
      headers: { authorization: `Bearer ${accessToken}` },
      signal: AbortSignal.timeout(1500),
    });
    if (!res.ok) return false;
    const state = (await res.json()) as { available?: unknown };
    return state.available === true;
  } catch {
    return false;
  }
}
