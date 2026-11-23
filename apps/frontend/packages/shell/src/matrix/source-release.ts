/* SPDX-License-Identifier: Apache-2.0 */
/**
 * «Soltar la fuente» (ADR 0086) desde la web: leer el modo de la fuente del
 * servidor de una app de registros y, si se congeló al pasar a Matrix desde una
 * app nativa, devolverla a los datos del servidor.
 *
 * El servicio exige la confirmación en el cuerpo y una identificación de hace
 * menos de cinco minutos (con segundo factor si la cuenta lo tiene). Sin ella
 * contesta `insufficient_user_authentication`: la puerta manda a identificarse
 * (`reauth=1` → `max_age=0`) y, a la vuelta, repite la operación ya confirmada.
 */
import { withBase } from '@sytue/config/base-path';
import { encodeStorageScopeHeader, storageScopeHeader } from '@sytue/config/matrix';
import { isSessionOwnerDescriptor, sessionOwnerHeader } from '@sytue/config/session-owner';

export const sourceReleaseProducts = [
  'calendar',
  'contacts',
  'notes',
  'erp',
  'bases',
  'cast',
  'hub',
  'docs',
  'sheets',
  'slides',
] as const;
export type SourceReleaseProduct = (typeof sourceReleaseProducts)[number];
export function isSourceReleaseProduct(value: string): value is SourceReleaseProduct {
  return (sourceReleaseProducts as readonly string[]).includes(value);
}

export interface FrozenSource {
  readonly mode: 'frozen' | 'private';
  readonly epoch: string;
}
export type ReleaseOutcome = 'released' | 'reauth' | 'stale' | 'failed';

export interface SourceReleaseOptions {
  readonly basePath: string;
  readonly actor: string;
  readonly tenant: string;
  readonly product: SourceReleaseProduct;
  readonly fetchImpl?: SourceReleaseFetch;
}
/** Solo lo que este cliente usa de fetch. */
export type SourceReleaseFetch = (input: string, init?: RequestInit) => Promise<Response>;

const epochPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/u;

/** Cliente acotado a una cuenta y un producto: cada petición lleva la generación
 * de la sesión que se comprobó y el ámbito exacto; si la sesión cambió, falla. */
export function sourceReleaseClient(options: SourceReleaseOptions) {
  const send: SourceReleaseFetch =
    options.fetchImpl ?? ((input, init) => globalThis.fetch(input, init));
  const api = (path: string) => withBase(options.basePath, '/api/private-storage/migration' + path);
  const headers = async (signal?: AbortSignal): Promise<Record<string, string>> => {
    const response = await send(withBase(options.basePath, '/auth/session'), {
      credentials: 'same-origin',
      cache: 'no-store',
      redirect: 'error',
      headers: { Accept: 'application/json' },
      ...(signal ? { signal } : {}),
    });
    const body = (await response.json().catch(() => null)) as { owner?: unknown } | null;
    const owner = body?.owner;
    if (
      !response.ok ||
      !isSessionOwnerDescriptor(owner) ||
      owner.actor !== options.actor ||
      owner.tenant !== options.tenant
    )
      throw new Error('session_changed');
    return {
      Accept: 'application/json',
      [sessionOwnerHeader]: owner.generation,
      [storageScopeHeader]: encodeStorageScopeHeader({
        tenant: options.tenant,
        subject: options.actor,
        product: options.product,
      }),
    };
  };
  return {
    /** La fuente si está congelada o ya es privada; null si sigue en legacy. */
    async frozen(signal?: AbortSignal): Promise<FrozenSource | null> {
      const response = await send(api(''), {
        credentials: 'same-origin',
        cache: 'no-store',
        redirect: 'error',
        headers: await headers(signal),
        ...(signal ? { signal } : {}),
      });
      if (!response.ok) throw new Error('source_state_unavailable');
      const state = (await response.json()) as { mode?: unknown; epoch?: unknown };
      if (state.mode === 'legacy') return null;
      if (
        (state.mode === 'frozen' || state.mode === 'private') &&
        typeof state.epoch === 'string' &&
        epochPattern.test(state.epoch)
      )
        return { mode: state.mode, epoch: state.epoch };
      throw new Error('source_state_invalid');
    },
    /** Suelta la fuente de esa época. La confirmación va en el cuerpo. */
    async release(epoch: string, signal?: AbortSignal): Promise<ReleaseOutcome> {
      if (!epochPattern.test(epoch)) return 'failed';
      const body = JSON.stringify({ epoch, discard_private_writes: true });
      let response: Response;
      try {
        response = await send(api('/release'), {
          method: 'POST',
          credentials: 'same-origin',
          cache: 'no-store',
          redirect: 'error',
          headers: {
            ...(await headers(signal)),
            'Content-Type': 'application/json',
          },
          body,
          ...(signal ? { signal } : {}),
        });
      } catch {
        return 'failed';
      }
      if (response.ok) return 'released';
      const failure = (await response.json().catch(() => null)) as {
        error?: { code?: unknown };
      } | null;
      const code = failure?.error?.code;
      if (response.status === 401 && code === 'insufficient_user_authentication') return 'reauth';
      if (response.status === 409 && code === 'private_storage_release_stale') return 'stale';
      return 'failed';
    },
  };
}

/** Lo que sobrevive a la vuelta de Sytue ID: la operación ya confirmada. */
interface PendingRelease {
  readonly actor: string;
  readonly tenant: string;
  readonly product: string;
  readonly epoch: string;
  readonly at: number;
}
const pendingKey = (product: string) => 'sytue.source-release.' + product;
const pendingLifetime = 10 * 60 * 1000;

export function rememberPendingRelease(
  options: Pick<SourceReleaseOptions, 'actor' | 'tenant' | 'product'>,
  epoch: string,
  now = Date.now(),
): void {
  try {
    const value: PendingRelease = { ...options, epoch, at: now };
    sessionStorage.setItem(pendingKey(options.product), JSON.stringify(value));
  } catch {
    // Sin almacenamiento de sesión, a la vuelta se pide confirmar otra vez.
  }
}

/** La época confirmada antes de identificarse, si es de esta misma cuenta y
 * reciente. Se borra al leerla: una vuelta repite la operación una sola vez. */
export function takePendingRelease(
  options: Pick<SourceReleaseOptions, 'actor' | 'tenant' | 'product'>,
  now = Date.now(),
): string | null {
  try {
    const raw = sessionStorage.getItem(pendingKey(options.product));
    sessionStorage.removeItem(pendingKey(options.product));
    if (!raw) return null;
    const value = JSON.parse(raw) as Partial<PendingRelease>;
    if (
      value.actor !== options.actor ||
      value.tenant !== options.tenant ||
      value.product !== options.product ||
      typeof value.epoch !== 'string' ||
      !epochPattern.test(value.epoch) ||
      typeof value.at !== 'number' ||
      now - value.at < 0 ||
      now - value.at > pendingLifetime
    )
      return null;
    return value.epoch;
  } catch {
    return null;
  }
}

/** A Sytue ID con `max_age=0` y de vuelta a esta misma página. */
export function reauthenticationUrl(basePath: string, returnTo: string): string {
  return withBase(basePath, '/auth/login?reauth=1&returnTo=' + encodeURIComponent(returnTo));
}
