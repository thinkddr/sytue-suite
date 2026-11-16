/* SPDX-License-Identifier: Apache-2.0 */
/** SSO/session plumbing shared by private products and the deployed Rooms client.
 * Import this subpath on callbacks: the maintained crypto SDK loads only AFTER URL cleanup. */
import { acquireMatrixStore } from './lease.js';
import { prefijoDeApp, suiteApex } from '@sytue/config/tokens';
import { BrowserKeyCustody } from './custody-browser.js';
import type { MatrixClient, ICreateClientOpts } from 'matrix-js-sdk';
import { readOfflineValue, mutateOfflineValue, deleteOfflineValue } from '../storage.js';
import {
  MatrixSecurityError,
  matrixStorePrefix,
  isMatrixDeviceId,
  validateBootstrap,
  type MatrixBootstrap,
  type MatrixSession,
  type MatrixStorageProduct,
} from './types.js';
import type { MatrixSessionAdapter } from './client.js';

type LoginClient = Pick<MatrixClient, 'loginFlows' | 'getSsoLoginUrl' | 'loginRequest' | 'logout'>;
type ClientFactory = (options: Pick<ICreateClientOpts, 'baseUrl' | 'accessToken'>) => LoginClient;
export interface MatrixSessionStore {
  read: typeof readOfflineValue;
  mutate: typeof mutateOfflineValue;
  delete: typeof deleteOfflineValue;
}
export interface MatrixSSOBrowser {
  location: Pick<Location, 'href' | 'origin' | 'pathname' | 'search' | 'assign'>;
  history: Pick<History, 'replaceState'>;
  sessionStorage: Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>;
  lifecycle?: EventTarget;
}
export interface MatrixBrowserAuthOptions {
  /** Omit only for the existing Rooms conversations session, which keeps its deployed namespace. */
  storageProduct?: MatrixStorageProduct;
  /** Chosen by the host application; legacy remains the default. */
  identityVersion?: 1 | 2;
  bootstrap?: (signal?: AbortSignal) => Promise<MatrixBootstrap>;
  deviceName: string;
  /** Ports for the host/test runtime; production uses the existing shared IndexedDB store and SDK. */
  store?: MatrixSessionStore;
  browser?: MatrixSSOBrowser;
  loadClient?: () => Promise<ClientFactory>;
  acquireDeviceLease?: (prefix: string) => Promise<() => void>;
}
interface Pending {
  nonce: string;
  actor: string;
  tenant: string;
  home: string;
  user: string;
  returnTo: string;
  created: number;
  product?: MatrixStorageProduct;
  identityVersion?: 1 | 2;
}
function unsafeReturnCharacter(value: string): boolean {
  return Array.from(value).some(
    (character) =>
      character === '\\' || character.charCodeAt(0) < 32 || character.charCodeAt(0) === 127,
  );
}

const standardStore: MatrixSessionStore = {
  read: readOfflineValue,
  mutate: mutateOfflineValue,
  delete: deleteOfflineValue,
};
async function loadStandardClient(): Promise<ClientFactory> {
  const { createClient } = await import('matrix-js-sdk');
  return (options) => createClient({ ...options, disableVoip: true });
}

export class MatrixBrowserAuth implements MatrixSessionAdapter {
  private generation = 0;
  private readonly base: string;
  private readonly namespace: string;
  private readonly pendingKey: string;
  private readonly store: MatrixSessionStore;
  /** The wrapped recovery key of this account (ADR 0081), through the same app's BFF. */
  readonly custody: BrowserKeyCustody;
  constructor(private readonly options: MatrixBrowserAuthOptions) {
    if (
      options.identityVersion !== undefined &&
      (options.storageProduct === undefined ||
        (options.identityVersion !== 1 && options.identityVersion !== 2))
    )
      throw new MatrixSecurityError('identity');
    this.options = Object.freeze({ ...options });
    this.base =
      prefijoDeApp(
        options.storageProduct === 'hub' ? 'one' : (options.storageProduct ?? 'rooms'),
      ) ?? '';
    if (!this.base) throw new MatrixSecurityError('unsupported');
    this.namespace = options.storageProduct ? 'suite-matrix-session-v1' : 'rooms-matrix-session-v1';
    this.pendingKey = options.storageProduct
      ? `sytue-storage-matrix-sso-${options.storageProduct}${options.identityVersion === 2 ? '-v2' : ''}`
      : 'sytue-rooms-matrix-sso';
    this.store = options.store ?? standardStore;
    this.custody = new BrowserKeyCustody({
      base: this.base,
      isolated: options.identityVersion === 2,
      rpId: suiteApex,
    });
  }
  private browser(): MatrixSSOBrowser {
    return (
      this.options.browser ?? {
        location: window.location,
        history: window.history,
        sessionStorage: window.sessionStorage,
        ...(navigator.serviceWorker ? { lifecycle: navigator.serviceWorker } : {}),
      }
    );
  }
  private binding(input: MatrixBootstrap): MatrixBootstrap {
    const bootstrap = validateBootstrap(input);
    if (
      bootstrap.storage_product !== this.options.storageProduct ||
      (this.options.storageProduct !== undefined &&
        (bootstrap.storage_identity_version === undefined ||
        bootstrap.storage_identity_version === 0
          ? 1
          : bootstrap.storage_identity_version) !== (this.options.identityVersion ?? 1))
    )
      throw new MatrixSecurityError('identity');
    return bootstrap;
  }
  refreshBootstrap = async (signal?: AbortSignal): Promise<MatrixBootstrap> => {
    if (this.options.bootstrap) return this.binding(await this.options.bootstrap(signal));
    const path = this.options.storageProduct
      ? '/api/matrix/storage/bootstrap' +
        (this.options.identityVersion === 2 ? '?identity_version=2' : '')
      : '/api/matrix/bootstrap';
    const response = await fetch(this.base + path, {
      credentials: 'same-origin',
      cache: 'no-store',
      ...(signal ? { signal } : {}),
    });
    if (!response.ok) throw new MatrixSecurityError('login');
    return this.binding((await response.json()) as MatrixBootstrap);
  };
  private key(bootstrap: MatrixBootstrap): [string, string, string] {
    return [
      this.options.storageProduct
        ? JSON.stringify([
            bootstrap.homeserver_url,
            bootstrap.matrix_user_id,
            bootstrap.tenant,
            bootstrap.actor,
            this.options.storageProduct,
          ])
        : bootstrap.matrix_user_id,
      this.namespace,
      bootstrap.homeserver_url,
    ];
  }
  private session(value: MatrixSession, bootstrap: MatrixBootstrap): MatrixSession {
    if (
      !value ||
      typeof value !== 'object' ||
      Array.isArray(value) ||
      value.user_id !== bootstrap.matrix_user_id ||
      !isMatrixDeviceId(value.device_id) ||
      typeof value.access_token !== 'string' ||
      !/^[\x21-\x7e]{1,16384}$/.test(value.access_token)
    )
      throw new MatrixSecurityError('identity');
    if (this.options.storageProduct) {
      if (
        value.storage_scope?.product !== this.options.storageProduct ||
        value.storage_scope.subject !== bootstrap.actor ||
        value.storage_scope.tenant !== bootstrap.tenant
      )
        throw new MatrixSecurityError('identity');
    } else if (value.storage_scope !== undefined) throw new MatrixSecurityError('identity');
    return {
      user_id: value.user_id,
      device_id: value.device_id,
      access_token: value.access_token,
      ...(this.options.storageProduct
        ? {
            storage_scope: {
              product: this.options.storageProduct,
              tenant: bootstrap.tenant,
              subject: bootstrap.actor,
            },
          }
        : {}),
    };
  }
  readSession = async (input: MatrixBootstrap): Promise<MatrixSession | null> => {
    const bootstrap = this.binding(input),
      value = await this.store.read<MatrixSession>(...this.key(bootstrap));
    return value === null ? null : this.session(value, bootstrap);
  };
  forgetSession = async (input: MatrixBootstrap, expected?: MatrixSession): Promise<void> => {
    ++this.generation;
    const key = this.key(this.binding(input));
    if (!expected) {
      await this.store.delete(...key);
      return;
    }
    const snapshot = { ...expected };
    await this.store.mutate<MatrixSession>(...key, (previous) =>
      previous?.user_id === snapshot.user_id &&
      previous.device_id === snapshot.device_id &&
      previous.access_token === snapshot.access_token
        ? null
        : previous,
    );
  };
  private flow(signal?: AbortSignal) {
    const browser = this.browser(),
      generation = ++this.generation;
    let cleared = false;
    const onClear = (event: Event): void => {
      if ((event as MessageEvent<{ type?: unknown }>).data?.type === 'SYTUE_CLEAR_OFFLINE')
        cleared = true;
    };
    browser.lifecycle?.addEventListener('message', onClear);
    const live = (): boolean => !cleared && !signal?.aborted && generation === this.generation;
    return {
      browser,
      live,
      check: (): void => {
        if (!live()) throw new MatrixSecurityError('device');
      },
      finish: (): void => browser.lifecycle?.removeEventListener('message', onClear),
    };
  }
  private callbackPath(): string {
    return this.base + (this.options.storageProduct ? '/auth/matrix/callback' : '/matrix/callback');
  }
  private returnPath(value: unknown, origin: string): string {
    if (
      typeof value !== 'string' ||
      value.length > 8192 ||
      !value.startsWith('/') ||
      value.startsWith('//') ||
      unsafeReturnCharacter(value)
    )
      throw new MatrixSecurityError('identity');
    const url = new URL(value, origin);
    let decoded: string;
    try {
      decoded = decodeURIComponent(url.pathname);
    } catch {
      throw new MatrixSecurityError('identity');
    }
    if (
      url.origin !== origin ||
      (url.pathname !== this.base && !url.pathname.startsWith(this.base + '/')) ||
      unsafeReturnCharacter(decoded) ||
      decoded.split('/').some((part) => part === '.' || part === '..')
    )
      throw new MatrixSecurityError('identity');
    for (const name of ['loginToken', 'access_token', 'id_token', 'refresh_token', 'code'])
      url.searchParams.delete(name);
    return url.pathname + url.search;
  }
  async begin(input: MatrixBootstrap, signal?: AbortSignal): Promise<void> {
    const bootstrap = this.binding(input),
      flow = this.flow(signal);
    try {
      flow.check();
      const factory = await (this.options.loadClient ?? loadStandardClient)();
      flow.check();
      const client = factory({ baseUrl: bootstrap.homeserver_url });
      const flows = await client.loginFlows();
      flow.check();
      if (
        !flows.flows.some(
          (entry) =>
            entry.type === 'm.login.sso' &&
            'identity_providers' in entry &&
            entry.identity_providers?.some((provider) => provider.id === bootstrap.sso_provider_id),
        )
      )
        throw new MatrixSecurityError('identity');
      const nonce = crypto.randomUUID(),
        callback = new URL(this.callbackPath(), flow.browser.location.origin);
      callback.searchParams.set('state', nonce);
      if (this.options.storageProduct && this.options.identityVersion === 2)
        callback.searchParams.set('identity_version', '2');
      const pending: Pending = {
        nonce,
        actor: bootstrap.actor,
        tenant: bootstrap.tenant,
        home: bootstrap.homeserver_url,
        user: bootstrap.matrix_user_id,
        returnTo: this.returnPath(
          flow.browser.location.pathname + flow.browser.location.search,
          flow.browser.location.origin,
        ),
        created: Date.now(),
        ...(this.options.storageProduct
          ? {
              product: this.options.storageProduct,
              identityVersion: this.options.identityVersion ?? 1,
            }
          : {}),
      };
      const destination = new URL(
        client.getSsoLoginUrl(callback.href, 'sso', bootstrap.sso_provider_id),
      );
      if (bootstrap.storage_product) {
        // Only the private Synapse providers forward these parameters. The
        // historical provider keeps its existing behavior during migration.
        destination.searchParams.set('sytue_tenant', bootstrap.tenant);
        destination.searchParams.set('sytue_subject', bootstrap.actor);
      }
      flow.check();
      flow.browser.sessionStorage.setItem(this.pendingKey, JSON.stringify(pending));
      flow.browser.location.assign(destination.href);
    } finally {
      flow.finish();
    }
  }
  async complete(signal?: AbortSignal): Promise<string> {
    const flow = this.flow(signal);
    let token = '';
    let issued: LoginClient | undefined;
    let releaseDevice: (() => void) | undefined;
    try {
      const url = new URL(flow.browser.location.href);
      token = url.searchParams.get('loginToken') ?? '';
      const nonce = url.searchParams.get('state');
      // Synchronous, before bootstrap, SDK loading, storage reads or any callback awaits.
      flow.browser.history.replaceState(null, '', url.pathname);
      const raw = flow.browser.sessionStorage.getItem(this.pendingKey);
      flow.browser.sessionStorage.removeItem(this.pendingKey);
      flow.check();
      if (
        !token ||
        token.length > 8192 ||
        !raw ||
        raw.length > 32768 ||
        url.searchParams.getAll('loginToken').length !== 1 ||
        url.searchParams.getAll('state').length !== 1 ||
        url.pathname !== this.callbackPath() ||
        url.searchParams.getAll('identity_version').length > 1 ||
        (url.searchParams.has('identity_version') &&
          url.searchParams.get('identity_version') !== String(this.options.identityVersion ?? 1)) ||
        (this.options.identityVersion === 2 && !url.searchParams.has('identity_version'))
      )
        throw new MatrixSecurityError('login');
      let pending: Pending;
      try {
        pending = JSON.parse(raw) as Pending;
      } catch {
        throw new MatrixSecurityError('identity');
      }
      if (
        !pending ||
        typeof pending.nonce !== 'string' ||
        !/^[0-9a-f-]{36}$/i.test(pending.nonce) ||
        pending.nonce !== nonce ||
        !Number.isSafeInteger(pending.created) ||
        Date.now() - pending.created > 10 * 60 * 1000 ||
        pending.created > Date.now() ||
        pending.product !== this.options.storageProduct ||
        (pending.identityVersion ?? 1) !== (this.options.identityVersion ?? 1)
      )
        throw new MatrixSecurityError('identity');
      const returnTo = this.returnPath(pending.returnTo, flow.browser.location.origin);
      const bootstrap = await this.refreshBootstrap(signal);
      flow.check();
      if (
        bootstrap.actor !== pending.actor ||
        bootstrap.tenant !== pending.tenant ||
        bootstrap.homeserver_url !== pending.home ||
        bootstrap.matrix_user_id !== pending.user
      )
        throw new MatrixSecurityError('identity');
      const factory = await (this.options.loadClient ?? loadStandardClient)();
      flow.check();
      const client = factory({ baseUrl: bootstrap.homeserver_url });
      const existing = await this.readSession(bootstrap);
      flow.check();
      if (existing) {
        releaseDevice = await (this.options.acquireDeviceLease ?? acquireMatrixStore)(
          matrixStorePrefix(bootstrap, existing.device_id),
        );
        flow.check();
      }
      const result = await client.loginRequest({
        type: 'm.login.token',
        token,
        initial_device_display_name: this.options.deviceName,
        ...(existing ? { device_id: existing.device_id } : {}),
      });
      token = '';
      if (typeof result.access_token === 'string' && result.access_token)
        issued = factory({ baseUrl: bootstrap.homeserver_url, accessToken: result.access_token });
      flow.check();
      if (existing && result.device_id !== existing.device_id)
        throw new MatrixSecurityError('device');
      const session = this.session(
        {
          user_id: result.user_id,
          device_id: result.device_id,
          access_token: result.access_token,
          ...(this.options.storageProduct
            ? {
                storage_scope: {
                  product: this.options.storageProduct,
                  tenant: bootstrap.tenant,
                  subject: bootstrap.actor,
                },
              }
            : {}),
        },
        bootstrap,
      );
      let committed = false;
      await this.store.mutate<MatrixSession>(...this.key(bootstrap), (previous) => {
        flow.check();
        if (previous !== null) this.session(previous, bootstrap);
        if ((previous?.access_token ?? null) !== (existing?.access_token ?? null))
          throw new MatrixSecurityError('locked');
        committed = true;
        return session;
      });
      if (!committed) throw new MatrixSecurityError('device');
      if (!flow.live()) {
        await this.store.mutate<MatrixSession>(...this.key(bootstrap), (previous) =>
          previous?.access_token === session.access_token ? null : previous,
        );
        throw new MatrixSecurityError('device');
      }
      issued = undefined;
      return returnTo;
    } catch (error) {
      // Revoke a late/foreign/failed login, while leaving another tab's newer session intact.
      if (issued) await issued.logout().catch(() => undefined);
      throw error;
    } finally {
      token = '';
      releaseDevice?.();
      flow.finish();
    }
  }
}
