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
  // Drive la suelta al vaciar su bóveda (PqVaultGate): sin fuente congelada, la
  // bóveda nueva no tendría de dónde importar el Drive del servidor.
  'drive',
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
  // Drive sirve la fuente en la ruta de su BFF de siempre; el resto, en la común.
  const root =
    options.product === 'drive'
      ? '/api/matrix/storage/migration'
      : '/api/private-storage/migration';
  const api = (path: string) => withBase(options.basePath, root + path);
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

/**
 * «Vaciar» la bóveda de una app con fuente en el servidor (PqVaultGate): antes de
 * vaciar se suelta la fuente congelada, para que la bóveda nueva vuelva a importar
 * los datos del servidor. Si para soltarla hay que identificarse otra vez, esta
 * marca (de esta pestaña, de esta cuenta y de esa época) dice a la vuelta que la
 * persona ya confirmó vaciar: se suelta y se vacía sin volver a preguntar.
 */
interface PendingEmpty {
  readonly actor: string;
  readonly tenant: string;
  readonly product: string;
  readonly epoch: string;
  readonly at: number;
}
const pendingEmptyKey = (product: string) => 'sytue.vault-empty.' + product;

export function rememberPendingEmpty(
  options: Pick<SourceReleaseOptions, 'actor' | 'tenant' | 'product'>,
  epoch: string,
  now = Date.now(),
): void {
  try {
    const value: PendingEmpty = { ...options, epoch, at: now };
    sessionStorage.setItem(pendingEmptyKey(options.product), JSON.stringify(value));
  } catch {
    // Sin almacenamiento de sesión, a la vuelta se pide vaciar otra vez.
  }
}

/** La época con la que se confirmó vaciar, si es de esta cuenta y reciente. Se
 * borra al leerla: la vuelta repite la operación una sola vez. */
export function takePendingEmpty(
  options: Pick<SourceReleaseOptions, 'actor' | 'tenant' | 'product'>,
  now = Date.now(),
): string | null {
  try {
    const raw = sessionStorage.getItem(pendingEmptyKey(options.product));
    sessionStorage.removeItem(pendingEmptyKey(options.product));
    if (!raw) return null;
    const value = JSON.parse(raw) as Partial<PendingEmpty>;
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

/** Lo que la puerta necesita de la fuente para vaciar (sustituible en pruebas). */
export interface SourceReleaser {
  frozen: (signal?: AbortSignal) => Promise<FrozenSource | null>;
  release: (epoch: string, signal?: AbortSignal) => Promise<ReleaseOutcome>;
}

/**
 * Suelta la fuente congelada antes de vaciar. `none`: no hay nada congelado (o ya
 * se soltó), se puede vaciar; `reauth`: hay que identificarse otra vez con esa
 * época; `failed`: no se pudo, y vaciar ahora dejaría la bóveda sin de dónde importar.
 */
export async function releaseBeforeEmpty(
  client: SourceReleaser,
  signal?: AbortSignal,
  expected?: string,
): Promise<{ outcome: 'none' | 'released' | 'failed' } | { outcome: 'reauth'; epoch: string }> {
  let frozen: FrozenSource | null;
  try {
    frozen = await client.frozen(signal);
  } catch {
    return { outcome: 'failed' };
  }
  if (!frozen) return { outcome: 'none' };
  // De vuelta de Sytue ID solo se suelta la época que la persona confirmó.
  if (expected !== undefined && frozen.epoch !== expected) return { outcome: 'failed' };
  const outcome = await client.release(frozen.epoch, signal);
  if (outcome === 'released') return { outcome };
  if (outcome === 'reauth') return { outcome, epoch: frozen.epoch };
  if (outcome === 'stale') {
    const now = await client.frozen(signal).catch(() => undefined);
    return { outcome: now === null ? 'none' : 'failed' };
  }
  return { outcome: 'failed' };
}
