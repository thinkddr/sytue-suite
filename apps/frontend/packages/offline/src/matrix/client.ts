/* SPDX-License-Identifier: Apache-2.0 */
import {
  createClient,
  Method,
  ClientEvent,
  MatrixEvent,
  EventType,
  SyncState,
  Direction,
  type MatrixClient,
  type IContent,
  type IEvent,
  type AuthDict,
} from 'matrix-js-sdk';
import {
  AllDevicesIsolationMode,
  CrossSigningKey,
  EventShieldColour,
  EventShieldReason,
  type CryptoApi,
  type GeneratedSecretStorageKey,
  type VerificationRequest,
} from 'matrix-js-sdk/lib/crypto-api';
import { decodeRecoveryKey, encodeRecoveryKey } from 'matrix-js-sdk/lib/crypto-api/recovery-key';
import { SECRET_STORAGE_ALGORITHM_V1_AES } from 'matrix-js-sdk/lib/secret-storage';
import { secureRandomString } from 'matrix-js-sdk/lib/randomstring';
import {
  Attachment as MatrixAttachment,
  EncryptedAttachment,
  initAsync,
} from '@matrix-org/matrix-sdk-crypto-wasm';
import type { RoomMessageEventContent } from 'matrix-js-sdk/lib/@types/events';
import {
  safeAttachmentType,
  validCallContent,
  type MatrixCallEventType,
  type MatrixCallKeyContent,
} from './guards.js';
import type { ReceivedToDeviceMessage } from 'matrix-js-sdk/lib/sync-accumulator';
import { DeviceVerification } from 'matrix-js-sdk/lib/models/device';
import type { MatrixKeyCustody } from './custody.js';
import { EncryptedOfflineValues } from '../encrypted-storage.js';
import { PqcRuntime, type ClientCoreModule } from '../pqc/runtime.js';
import { StoragePq } from './storage-pq.js';
import { StoragePqKeys } from './storage-pq-keys.js';
import {
  MatrixSessionProvenance,
  MatrixSessionProvenanceMissing,
  type MatrixSessionDescriptor,
} from './session-provenance.js';
import { MatrixOwnerKeyCache } from './owner-key-cache.js';
import { cryptoStoreKeyValues, initCryptoStore } from './crypto-store-key.js';
import { acquireMatrixStore } from './lease.js';
import { logoutMatrixDevice } from './device-logout.js';
import { StorageJobs, storageWork } from './storage-jobs.js';
import { SDKLifetimePair } from './sdk-lifetime.js';
import { openSessionOwner, verifySessionOwner, type OpaqueSessionOwner } from '../session-owner.js';
import { HttpClient, HttpError, DriveClient, createStorageRootService } from '@sytue/api-clients';
import { withBase } from '@sytue/config/base-path';
import { encodeStorageScopeHeader, storageScopeHeader } from '@sytue/config/matrix';
import { PqcPortGroup } from '../pqc/port-lifetime.js';
import { PqcEncryptedJournal } from '../pqc/encrypted-journal-port.js';
import { PqcSourcePort } from '../pqc/source-port.js';
import { PqcWorkPort } from '../pqc/work-port.js';
import { PqcServicePort, privateServiceRoute } from '../pqc/service-port.js';
import { PqcIngressSecretsPort } from '../pqc/ingress-secret-port.js';
import { PqcIngressControlPort } from '../pqc/ingress-control-port.js';
import { encodeHostJSON, decodeHostObject } from '../pqc/host-codec.js';
import type {
  ResolvedHostDelegation,
  HostPortHandle,
  HostHandler,
  HostMethod,
} from '../pqc/host-port.js';
import { PqcStoragePort } from '../pqc/storage-port.js';
import { eraseOwnedBytes } from '../pqc/owned-bytes.js';
import { ensureStorageRoot, checkedStorageAction, publicStorageKey } from './storage-authority.js';
import type {
  StorageRootEnrollment,
  StorageRootAuthority,
  StorageRootService,
  StorageControlIntent,
  StorageControlAction,
  SignedStorageControlAction,
} from '@sytue/config/matrix';
import { recoveryUILifetime } from './recovery-ui.js';
import { privateDeviceFetch, matrixLifetimeFetch } from './private-policy.js';
import type { StorageBlobPort } from './storage-files.js';
import {
  StorageRepository,
  type StorageRecordTransport,
  type StoragePublicationPolicy,
} from './storage-repository.js';
import { StoragePublicationAdmission } from './storage-publication-admission.js';
import { StoragePublicationJobs } from './storage-publication-jobs.js';
import {
  hasSecretStorageCheck,
  decodeOwnerProof,
  ownerKeyID,
  sealStorageOwnerRecord,
  verifyStorageOwnerRecord,
  type StorageOwnerKey,
} from './owner-proof.js';
import {
  decodeStorageRecord,
  sameStorageRecord,
  storageIdentifier,
  storageReference,
  StorageRecordError,
  type StorageRecord,
  type StorageRecordEntry,
  type StoragePublicationTarget,
} from './storage.js';
import {
  decodeAttestedStorageSessions,
  decodeSsssVaultSecret,
  encodeAttestedStorageSessions,
  encodeSsssVaultSecret,
  isValidAttestedStorageSession,
  normalizeAttestedStorageSessions,
  ssssVaultEventPrefix,
  storageSessionsSecretId,
  type AttestedStorageSession,
} from './ssss-vault.js';
import type { MatrixStorageScope } from './types.js';
import {
  MatrixSecurityError,
  matrixStorePrefix,
  isMatrixRoomId,
  validateBootstrap,
  validateCallScope,
  type MatrixCallScope,
  type MatrixCallMember,
  type LocalMatrixAttachment,
  type MatrixBootstrap,
  type MatrixFile,
  type MatrixDraft,
  type MatrixSession,
  type MatrixReference,
  type RoomsEncryptedContent,
} from './types.js';

// Viven en `guards.ts` para que las apps sin Matrix las usen sin el SDK; aquí siguen
// exportándose como antes.
export {
  safeAttachmentType,
  validCallContent,
  type MatrixCallEventType,
  type MatrixCallKeyContent,
};

/**
 * Recovery secrets stay in this client. Since ADR 0081 the shared gate never shows
 * them: it wraps a new key with the person's passkey or password, and unwraps it on
 * a new device.
 */
export interface MatrixRecoveryUI {
  /**
   * A new key, BEFORE it becomes the account's secret storage key `keyID`. Reject to
   * abort: nothing has been published yet.
   */
  saveRecoveryKey: (encodedKey: string, keyID: string) => Promise<void>;
  /** The key for one of `keyIDs` (the secret storage keys the SDK can use), when known. */
  readRecoveryKey: (keyIDs?: readonly string[]) => Promise<string>;
  authenticate: (url: string) => Promise<void>;
  /** Close/reject the host's visible prompts when its device is closed. */
  cancelPending?: () => void;
  confirmStorageRoot?: (request: StorageRootEnrollment) => Promise<void>;
}
export interface MatrixDevice {
  id: string;
  name: string;
  verified: boolean;
  current: boolean;
}

/** Product routing is supplied by the host app; crypto never chooses a BFF or return URL. */
export interface MatrixSessionAdapter {
  readSession: (bootstrap: MatrixBootstrap) => Promise<MatrixSession | null>;
  /** Atomically remove only the captured session when supplied; preserve a newer login. */
  forgetSession: (bootstrap: MatrixBootstrap, expected?: MatrixSession) => Promise<void>;
  refreshBootstrap: () => Promise<MatrixBootstrap>;
  /** The wrapped recovery key of this account (ADR 0081), when the host offers it. */
  readonly custody?: MatrixKeyCustody;
}

/** Only fixed host API callbacks are accepted; consumers never supply phase,
 * identity version, an SDK binding, root key or recovery metadata. */
interface HistoricalStorageHost {
  readCutover: (signal: AbortSignal) => Promise<Uint8Array<ArrayBuffer>>;
}
interface SDKLeaseSnapshot {
  readonly homeserver: string;
  readonly userID: string;
  readonly deviceID: string;
  readonly rootKey: string;
  readonly recoveryKeyID: string;
  readonly backupVersion: string;
}
interface ValidatedIdentityCutover {
  v: 1;
  scope: MatrixStorageScope;
  phase: 'legacy' | 'freezing' | 'frozen' | 'isolated';
  epoch?: string;
  source_rooms: string[];
  receipt?: MatrixReference;
}
const historicalReadMethods: readonly HostMethod[] = [
  'storage.prepare',
  'storage.bindings',
  'storage.history',
  'storage.record.read',
  'storage.object.read',
  'storage.file.info',
  'storage.file.commitment',
  'storage.file.download',
  'storage.file.read',
  'storage.file.close',
];
const verifiedSuiteOwners = new WeakSet<SuiteMatrix>();
export interface MailPrivatePorts {
  readonly runtime: PqcRuntime;
  readonly port: string;
  readonly legacy_port?: string;
  readonly migration_writer_port?: string;
  readonly signal: AbortSignal;
  readonly scope: Readonly<MatrixStorageScope>;
  readonly session: string;
  closeAndDrain: () => Promise<void>;
}
export class SuiteMatrix {
  readonly client: MatrixClient;
  private readonly crypto: CryptoApi;
  private readonly temporaryKeys: Map<string, Uint8Array<ArrayBuffer>>;
  private readonly urls = new Set<string>();
  private readonly prefix: string;
  private readonly release: () => void;
  private readonly onOfflineClear: (event: MessageEvent) => void;
  private closed = false;
  private publicationJobs = new StoragePublicationJobs(() => {
    if (this.closed) throw new MatrixSecurityError('device');
  });
  private deferRelease = false;
  private forgetting: Promise<void> | undefined;
  private storageJobs: StorageJobs | undefined;
  private readonly clientModules = new Map<ClientCoreModule, PqcRuntime>();
  private appSessionOwner: OpaqueSessionOwner | undefined;
  private publicationAdmission: StoragePublicationAdmission | undefined;
  private appSessionBase: string | undefined;
  private openingAppOwner: Promise<OpaqueSessionOwner> | undefined;
  private mailPrivateOpening: Promise<MailPrivatePorts> | undefined;
  private destructionGuard: (() => Promise<void>) | undefined;
  private readonly hostDelegations = new WeakMap<
    object,
    { module: ClientCoreModule; used: boolean; grant: ResolvedHostDelegation }
  >();
  private privateStoragePq: StoragePq | undefined;
  private roomSessionProofs: MatrixSessionProvenance | undefined;
  private roomBackfillCache: EncryptedOfflineValues | undefined;
  private roomBackfill:
    Promise<{ scanned: number; authenticated: number; blocked: number }> | undefined;
  private jobsClosing: Promise<void> | undefined;
  private localJobsClosing: Promise<void> | undefined;
  private lifetimePair: SDKLifetimePair | undefined;
  private lifetimePeer: SuiteMatrix | undefined;
  private erasing: Promise<void> | undefined;
  private readonly policyRooms = new Set<string>();
  private readonly verificationPeers = new Set<string>();
  private attestedStorageSessions?: Map<string, AttestedStorageSession>;
  private liveStorageSessions?: Map<string, AttestedStorageSession>;
  private pendingStorageEventRefs?: MatrixReference[];
  private constructor(
    readonly bootstrap: MatrixBootstrap,
    client: MatrixClient,
    release: () => void,
    prefix: string,
    private readonly ui: MatrixRecoveryUI,
    keys: Map<string, Uint8Array<ArrayBuffer>>,
    private readonly adapter: MatrixSessionAdapter,
    private readonly network: { closed: boolean; lifetime?: AbortController },
    private readonly ownerKeys?: MatrixOwnerKeyCache,
    private readonly session?: MatrixSession,
    private readonly uploadCache?: EncryptedOfflineValues,
    private readonly storagePqCache?: EncryptedOfflineValues,
    /** The sealed key of this device's SDK crypto store (ADR 0085), erased with it. */
    private readonly cryptoStoreKeys?: EncryptedOfflineValues,
  ) {
    this.client = client;
    this.temporaryKeys = keys;
    const crypto = client.getCrypto();
    if (!crypto) throw new MatrixSecurityError('device');
    this.crypto = crypto;
    crypto.setEncryptedEventPublicationGuard(async (event) => {
      if (!bootstrap.storage_product) await this.attestOwnPublication(event);
      await this.publicationJobs.authorize(event);
    });
    client.setEventHttpGuard(async (event) => {
      if (this.closed) throw new MatrixSecurityError('device');
      if (event.getWireType() !== String(EventType.RoomMessageEncrypted))
        return this.network.lifetime?.signal;
      await crypto.revalidateEncryptedEventPublication(event);
      const signal = await this.publicationJobs.authorize(event);
      if (this.closed) throw new MatrixSecurityError('device');
      signal?.throwIfAborted();
      if (this.publicationAdmission) {
        const lease = this.publicationJobs.lease(event);
        await this.publicationAdmission.beforeHTTP(event, lease);
        await crypto.revalidateEncryptedEventPublication(event);
        lease.assertOpen();
        await this.publicationJobs.authorize(event);
      }
      signal?.throwIfAborted();
      return signal ?? this.network.lifetime?.signal;
    });
    this.release = release;
    this.prefix = prefix;
    if (!bootstrap.storage_product)
      this.roomBackfillCache = new EncryptedOfflineValues(
        prefix,
        'rooms-provenance-backfill-v1',
        () => !this.closed,
      );
    this.onOfflineClear = (event) => {
      if ((event.data as { type?: unknown } | null)?.type === 'SYTUE_CLEAR_OFFLINE')
        void this.forget();
    };
    navigator.serviceWorker?.addEventListener('message', this.onOfflineClear);
  }
  static async open(
    bootstrap: MatrixBootstrap,
    ui: MatrixRecoveryUI,
    adapter: MatrixSessionAdapter,
  ): Promise<SuiteMatrix> {
    bootstrap = validateBootstrap(bootstrap);
    if (bootstrap.storage_product) {
      // Pin the verified private scope/rooms before any network or host lifecycle callback.
      const channels: MatrixBootstrap['channels'] = [];
      const rooms = [...(bootstrap.storage_room_ids ?? [])];
      const drafts: string[] = [];
      Object.freeze(channels);
      Object.freeze(rooms);
      Object.freeze(drafts);
      bootstrap = Object.freeze({
        ...bootstrap,
        channels,
        storage_room_ids: rooms,
        draft_room_ids: drafts,
      });
    }
    const storedSession = await adapter.readSession(bootstrap);
    if (!storedSession) throw new MatrixSecurityError('login');
    const session = Object.freeze({
      user_id: storedSession.user_id,
      device_id: storedSession.device_id,
      access_token: storedSession.access_token,
      ...(storedSession.storage_scope
        ? { storage_scope: Object.freeze({ ...storedSession.storage_scope }) }
        : {}),
    });
    if (session.user_id !== bootstrap.matrix_user_id || !session.device_id || !session.access_token)
      throw new MatrixSecurityError('identity');
    const product = bootstrap.storage_product;
    if (
      product &&
      (session.storage_scope?.product !== product ||
        session.storage_scope.subject !== bootstrap.actor ||
        session.storage_scope.tenant !== bootstrap.tenant)
    )
      throw new MatrixSecurityError('identity');
    const prefix = matrixStorePrefix(bootstrap, session.device_id);
    const release = await acquireMatrixStore(prefix);
    const keys = new Map<string, Uint8Array<ArrayBuffer>>();
    const network = { closed: false, lifetime: new AbortController() };
    ui = recoveryUILifetime(ui, network.lifetime.signal);
    let deviceCrypto: CryptoApi | undefined;
    // Explicitly undefined: the crypto callbacks above may run before the cache
    // exists and must then fall back to `keys`, so this cannot be a `const`.
    let ownerKeys: MatrixOwnerKeyCache | undefined = undefined;
    const client = createClient({
      disableVoip: true,
      baseUrl: bootstrap.homeserver_url,
      userId: session.user_id,
      deviceId: session.device_id,
      accessToken: session.access_token,
      ...(product
        ? {
            fetchFn: privateDeviceFetch(
              bootstrap.homeserver_url,
              bootstrap.matrix_user_id,
              async (device) =>
                (await deviceCrypto?.getDeviceVerificationStatus(
                  bootstrap.matrix_user_id,
                  device,
                )) ?? null,
              () => network.closed,
              globalThis.fetch,
              network.lifetime.signal,
            ),
          }
        : { fetchFn: matrixLifetimeFetch(network.lifetime.signal) }),
      cryptoCallbacks: {
        getSecretStorageKey: async ({ keys: available }) => {
          if (network.closed) throw new MatrixSecurityError('device');
          if (ownerKeys) return ownerKeys.get(available);
          for (const [id, description] of Object.entries(available)) {
            const cached = keys.get(id);
            if (
              cached &&
              hasSecretStorageCheck(description) &&
              (await client.secretStorage.checkKey(cached, description))
            ) {
              if (network.closed) throw new MatrixSecurityError('device');
              return [id, cached];
            }
          }
          if (network.closed) throw new MatrixSecurityError('device');
          if (!Object.values(available).some(hasSecretStorageCheck))
            throw new MatrixSecurityError('recovery');
          const encoded = await ui.readRecoveryKey(Object.keys(available));
          const key = decodeRecoveryKey(encoded.trim());
          let retained = false;
          try {
            for (const [id, description] of Object.entries(available)) {
              if (
                hasSecretStorageCheck(description) &&
                (await client.secretStorage.checkKey(key, description))
              ) {
                if (network.closed) throw new MatrixSecurityError('device');
                keys.set(id, key);
                retained = true;
                return [id, key];
              }
            }
            throw new MatrixSecurityError('recovery');
          } finally {
            if (!retained) key.fill(0);
          }
        },
        cacheSecretStorageKey: (id, _description, key) => {
          if (network.closed) throw new MatrixSecurityError('device');
          if (ownerKeys) ownerKeys.remember(id, _description, key);
          else keys.set(id, key);
        },
      },
    });
    const cryptoStoreKeys = cryptoStoreKeyValues(prefix, () => !network.closed);
    ownerKeys = new MatrixOwnerKeyCache(
      new EncryptedOfflineValues(prefix, 'matrix-owner-key-v1', () => !network.closed),
      keys,
      (key, description) => client.secretStorage.checkKey(key, description),
      (keyIDs) => ui.readRecoveryKey(keyIDs),
      () => !network.closed,
    );
    try {
      const who = await client.whoami();
      if (who.user_id !== bootstrap.matrix_user_id || who.device_id !== session.device_id)
        throw new MatrixSecurityError('identity');
      // Encrypted at rest with a key nobody can read (ADR 0085).
      await initCryptoStore(client, prefix, cryptoStoreKeys);
      const crypto = client.getCrypto();
      if (!crypto) throw new MatrixSecurityError('device');
      deviceCrypto = crypto;
      crypto.setRequireKeyBackupBeforeSending(true);
      crypto.setDeviceIsolationMode(new AllDevicesIsolationMode(true));
      crypto.globalBlacklistUnverifiedDevices = true;
      crypto.setTrustCrossSignedDevices(true);
      await assertControllerUntrusted(crypto, bootstrap.controller_user_id);
      await client.startClient({ initialSyncLimit: 20, lazyLoadMembers: false });
      await new Promise<void>((resolve, reject) => {
        if (client.isInitialSyncComplete()) {
          resolve();
          return;
        }
        const timeout = setTimeout(() => {
          client.off(ClientEvent.Sync, sync);
          reject(new MatrixSecurityError('device'));
        }, 30000);
        const sync = (state: SyncState): void => {
          if (state === SyncState.Prepared || state === SyncState.Syncing) {
            clearTimeout(timeout);
            client.off(ClientEvent.Sync, sync);
            resolve();
          } else if (state === SyncState.Error) {
            clearTimeout(timeout);
            client.off(ClientEvent.Sync, sync);
            reject(new MatrixSecurityError('device'));
          }
        };
        client.on(ClientEvent.Sync, sync);
      });
      const suite = new SuiteMatrix(
        bootstrap,
        client,
        release,
        prefix,
        ui,
        keys,
        adapter,
        network,
        ownerKeys,
        session,
        product
          ? new EncryptedOfflineValues(prefix, 'matrix-file-upload-v1', () => !network.closed)
          : undefined,
        product
          ? new EncryptedOfflineValues(prefix, 'matrix-storage-pq-key-v1', () => !network.closed)
          : undefined,
        cryptoStoreKeys,
      );
      verifiedSuiteOwners.add(suite);
      return suite;
    } catch (error) {
      network.closed = true;
      network.lifetime.abort();
      try {
        ui.cancelPending?.();
      } catch {
        /* Continue erasing private credentials. */
      }
      client.stopClient();
      ownerKeys?.close();
      for (const key of keys.values()) key.fill(0);
      release();
      throw error;
    }
  }
  /** Uses only this verified isolated SDK account; public metadata is never a work permit. */
  async ensureStorageAuthority(
    service: StorageRootService,
    signal: AbortSignal,
  ): Promise<StorageRootAuthority> {
    const assertOpen = (current: AbortSignal): void => {
      current.throwIfAborted();
      if (this.closed) throw new MatrixSecurityError('device');
    };
    return ensureStorageRoot(
      {
        assertOpen,
        ownRoot: async (current) => {
          assertOpen(current);
          if (this.bootstrap.storage_identity_version !== 2)
            throw new MatrixSecurityError('identity');
          const repository = this.storage();
          await repository.prepare(current);
          if (!(await this.trusted())) throw new MatrixSecurityError('unverified');
          const root = await this.crypto.getCrossSigningKeyId();
          assertOpen(current);
          if (!publicStorageKey(root)) throw new MatrixSecurityError('recovery');
          return Object.freeze({
            v: 1,
            scope: repository.scope,
            user_id: this.bootstrap.matrix_user_id,
            target: Object.freeze(repository.publicationTarget()),
            root_key: root,
            generation: 1,
          });
        },
        approve: async (request, current) => {
          assertOpen(current);
          if (!this.ui.confirmStorageRoot) throw new MatrixSecurityError('unsupported');
          await this.ui.confirmStorageRoot(request);
          assertOpen(current);
        },
        sign: (intent, action, current) => this.signStorageControl(intent, action, current),
      },
      service,
      signal,
    );
  }

  signIngressAction(
    expected: StorageControlIntent,
    input: StorageControlAction,
    signal: AbortSignal,
  ): Promise<SignedStorageControlAction> {
    if (
      (this.bootstrap.storage_product !== 'mail' &&
        this.bootstrap.storage_product !== 'calendar') ||
      !['key.enroll', 'task.claim', 'task.ack', 'task.handoff'].includes(expected.kind)
    )
      return Promise.reject(new MatrixSecurityError('unsupported'));
    return this.signStorageControl(expected, input, signal);
  }

  private signStorageControl(
    expected: StorageControlIntent,
    input: StorageControlAction,
    signal: AbortSignal,
  ): Promise<SignedStorageControlAction> {
    const action = checkedStorageAction(input, expected, input.root_key);
    const jobs = (this.storageJobs ??= new StorageJobs());
    return jobs.run(
      crypto.randomUUID(),
      async (current) => {
        current.throwIfAborted();
        if (this.closed || this.bootstrap.storage_identity_version !== 2)
          throw new MatrixSecurityError('device');
        const repository = this.storage();
        await repository.prepare(current);
        const target = repository.publicationTarget(action.target);
        const root = await this.crypto.getCrossSigningKeyId();
        if (
          action.scope.tenant !== repository.scope.tenant ||
          action.scope.subject !== repository.scope.subject ||
          action.scope.product !== repository.scope.product ||
          target.device_id !== this.client.getDeviceId() ||
          root !== action.root_key ||
          !(await this.trusted())
        )
          throw new MatrixSecurityError('unverified');
        current.throwIfAborted();
        if (this.closed) throw new MatrixSecurityError('device');
        const signed = await this.crypto.signJsonObject({ ...action });
        current.throwIfAborted();
        if (this.closed) throw new MatrixSecurityError('device');
        return signed as unknown as SignedStorageControlAction;
      },
      signal,
    );
  }

  async trusted(): Promise<boolean> {
    const device = this.client.getDeviceId();
    if (!device || this.closed) return false;
    const status = await this.crypto.getDeviceVerificationStatus(
      this.bootstrap.matrix_user_id,
      device,
    );
    const backup = await this.crypto.checkKeyBackupAndEnable();
    return (
      status?.crossSigningVerified === true &&
      (await this.crypto.isCrossSigningReady()) &&
      (await this.crypto.isSecretStorageReady()) &&
      backup?.trustInfo.trusted === true
    );
  }
  private async interactiveAuth<T>(makeRequest: (auth: AuthDict | null) => Promise<T>): Promise<T> {
    try {
      return await makeRequest(null);
    } catch (error) {
      const auth = error as {
        httpStatus?: number;
        data?: { session?: string; flows?: { stages: string[] }[] };
      };
      if (
        auth.httpStatus !== 401 ||
        !auth.data?.session ||
        !auth.data.flows?.some(
          (flow) => flow.stages.length === 1 && flow.stages[0] === 'm.login.sso',
        )
      )
        throw error;
      await this.ui.authenticate(this.client.getFallbackAuthUrl('m.login.sso', auth.data.session));
      return makeRequest({ session: auth.data.session });
    }
  }
  async setupRecovery(): Promise<void> {
    await this.crypto.getUserDeviceInfo([this.bootstrap.matrix_user_id], true);
    const previous = await this.crypto.getCrossSigningStatus();
    const cached = previous.privateKeysCachedLocally;
    if (
      previous.publicKeysOnDevice &&
      !previous.privateKeysInSecretStorage &&
      !(cached.masterKey && cached.selfSigningKey && cached.userSigningKey)
    )
      throw new MatrixSecurityError('recovery');
    await this.publishRecoveryKey(await this.newRecoveryKey());
    await this.finishRecoverySetup(false);
  }
  /**
   * What `setupRecovery` does once the new key is the default one: the cross-signing keys
   * and the backup key go into secret storage under it, and the device signs itself.
   * `newIdentity`: an account that never published one gets a fresh identity even if
   * this device kept private keys of an upload that never happened.
   */
  private async finishRecoverySetup(newIdentity: boolean): Promise<void> {
    await this.crypto.bootstrapSecretStorage({ setupNewKeyBackup: false });
    await this.ownerKeys?.flush();
    await this.crypto.bootstrapCrossSigning({
      authUploadDeviceSigningKeys: (request) => this.interactiveAuth(request),
      ...(newIdentity ? { setupNewCrossSigning: true } : {}),
    });
    await this.crypto.getUserDeviceInfo([this.bootstrap.matrix_user_id], true);
    if ((await this.crypto.checkKeyBackupAndEnable()) === null) await this.crypto.resetKeyBackup();
    await this.ownerKeys?.flush();
    if (!(await this.settledTrust())) throw new MatrixSecurityError('device');
  }
  /**
   * The account has a secret storage key that nothing here opens: no wrapper, no copy
   * the service held, not in this device's owner-key cache. Whether this device can
   * still protect the account again WITHOUT losing anything that someone could open:
   *
   * - `new`: the encryption was never set up. The homeserver has no cross-signing
   *   identity for the account and no key backup, and secret storage holds no vault
   *   secret: no device was ever trusted, so the key protected nothing (an attempt
   *   interrupted under v3.20, ADR 0082). It is set up as for a new account.
   * - `cached`: this device keeps the account's own cross-signing private keys (the
   *   published ones) and the decryption key of its current backup, if any: everything
   *   secret storage protects for the account, except vault secrets, and there are none.
   *   A storage account also keeps no other owner key here. Those keys are stored again
   *   under a new secret storage key, wrapped with the Sytue ID unlock key.
   * - null: anything else. Nothing is replaced.
   *
   * Everything is read from the homeserver: a key query that fails is never «no identity».
   */
  async orphanedRecovery(): Promise<'new' | 'cached' | null> {
    if (this.closed) return null;
    const me = this.bootstrap.matrix_user_id;
    const keys = await this.client.downloadKeysForUsers([me]);
    if (this.closed || Object.keys(keys.failures ?? {}).length > 0) return null;
    if (this.vaultInUse()) return null;
    const published = !!(
      keys.master_keys?.[me] ||
      keys.self_signing_keys?.[me] ||
      keys.user_signing_keys?.[me]
    );
    const backup = await this.crypto.getKeyBackupInfo();
    if (this.closed) return null;
    if (!published) return backup ? null : 'new';
    await this.crypto.getUserDeviceInfo([me], true);
    // Non-null only with the three private keys here AND matching the published identity.
    if (!(await this.crypto.getCrossSigningKeyId(CrossSigningKey.Master))) return null;
    if (backup && !(await this.crypto.isKeyBackupTrusted(backup)).matchesDecryptionKey) return null;
    if (this.bootstrap.storage_product && (await this.ownerKeys?.cachedID()) != null) return null;
    return this.closed ? null : 'cached';
  }
  /**
   * Vault secrets (`org.sytue.vault.*`) are sealed with the secret storage key and only
   * that key opens them: while there are any, the key is never replaced. An account data
   * store that cannot be listed counts as «in use».
   */
  private vaultInUse(): boolean {
    const data = (this.client.store as { accountData?: unknown }).accountData;
    if (!(data instanceof Map)) return true;
    for (const [type, event] of data as Map<string, MatrixEvent>) {
      if (!type.startsWith(ssssVaultEventPrefix)) continue;
      const content = event.getContent();
      if (content && Object.keys(content).length > 0) return true;
    }
    return false;
  }
  /**
   * Whether any `org.sytue.vault.*` secret in account data is NOT encrypted under `keyID`
   * (or account data cannot be listed).
   */
  private vaultHasSecretsOutside(keyID: string): boolean {
    const data = (this.client.store as { accountData?: unknown }).accountData;
    if (!(data instanceof Map)) return true;
    for (const [type, event] of data as Map<string, MatrixEvent>) {
      if (!type.startsWith(ssssVaultEventPrefix)) continue;
      const content = event.getContent();
      if (!content || Object.keys(content).length === 0) continue;
      const encrypted = content.encrypted as Record<string, unknown> | undefined;
      if (!encrypted || typeof encrypted !== 'object' || !(keyID in encrypted)) return true;
    }
    return false;
  }
  /**
   * When secret storage's default key cannot be opened, but this device already holds a
   * valid owner key (`cached !== current`) that still exists on the homeserver and covers
   * every `org.sytue.vault.*` secret, restores `m.secret_storage.default_key` to `cached`
   * and completes recovery under it — preserving historical owner proofs and vault secrets
   * without rotating the key.
   */
  async restoreHeldRecovery(): Promise<boolean> {
    if (this.closed || !this.ownerKeys) return false;
    const cached = await this.ownerKeys.cachedID().catch(() => null);
    if (!cached || this.closed) return false;
    const current = await this.client.secretStorage.getDefaultKeyId();
    if (!current || cached === current || this.closed) return false;
    const held = await this.heldRecoveryKey(cached);
    if (!held || this.closed) return false;
    if (this.vaultHasSecretsOutside(cached)) return false;
    await this.client.secretStorage.setDefaultKeyId(cached);
    try {
      await this.recover();
      return true;
    } catch {
      await this.client.secretStorage.setDefaultKeyId(current).catch(() => undefined);
      return false;
    }
  }
  /**
   * Protects again an account `orphanedRecovery` found safe to protect, checked again
   * here: a new secret storage key, wrapped with the Sytue ID unlock key BEFORE it is
   * published (`saveRecoveryKey`), the default from then on. Nothing on the homeserver is
   * deleted: the old key description stays, and only the secrets this device holds in
   * the clear are stored again under the new key. This device's Olm account and Megolm
   * sessions are not touched, so whatever it can read now it still reads.
   */
  async restartRecovery(): Promise<void> {
    if (this.closed) throw new MatrixSecurityError('device');
    const plan = await this.orphanedRecovery();
    if (!plan) throw new MatrixSecurityError('recovery');
    // The key of a setup that never published an identity protected nothing; outside
    // private storage a cached key is no owner proof either (a storage account with one
    // is never `cached`, see above).
    if (plan === 'new' || !this.bootstrap.storage_product) await this.ownerKeys?.discard();
    await this.publishRecoveryKey(await this.newRecoveryKey());
    await this.finishRecoverySetup(plan === 'new');
  }
  /**
   * Lost access: a new cross-signing identity, secret storage and key backup for
   * the account, with a new recovery key handed to the host before the reset. The old backup is
   * deleted, so older messages stay readable only on devices that already hold
   * their keys, and the person must verify their other devices and contacts again.
   * Replacing cross-signing keys always goes through the homeserver's SSO re-auth.
   */
  async resetRecovery(): Promise<void> {
    if (this.closed) throw new MatrixSecurityError('device');
    // Private storage keeps its vault inside secret storage: a reset would orphan it.
    if (this.bootstrap.storage_product) throw new MatrixSecurityError('unsupported');
    // The new key is handed to the host (wrapped and stored for the person, ADR 0081)
    // BEFORE anything is reset: if that fails, the account is left as it was.
    const generated = await this.newRecoveryKey();
    await this.crypto.resetEncryption((request) => this.interactiveAuth(request));
    for (const key of this.temporaryKeys.values()) key.fill(0);
    this.temporaryKeys.clear();
    await this.publishRecoveryKey(generated);
    // The new default key is in place: this stores the new cross-signing keys and the
    // backup that resetEncryption just made (`setupNewKeyBackup: false`).
    await this.crypto.bootstrapSecretStorage({ setupNewKeyBackup: false });
    if (!(await this.settledTrust())) throw new MatrixSecurityError('device');
  }
  /**
   * Right after new cross-signing keys or a new backup, the device's own signature
   * can take a moment to show up (seen once in six resets against Synapse 1.163):
   * a single check reported a correct setup as failed. Waits up to ~5 s.
   */
  private async settledTrust(): Promise<boolean> {
    for (let attempt = 0; ; attempt++) {
      if ((await this.crypto.isSecretStorageReady()) && (await this.trusted())) return true;
      if (attempt >= 10 || this.closed) return false;
      await new Promise((resolve) => setTimeout(resolve, 500));
      if (this.closed) return false;
      await this.crypto
        .getUserDeviceInfo([this.bootstrap.matrix_user_id], true)
        .catch(() => undefined);
    }
  }
  /**
   * A new recovery key and the secret storage key ID it will have, handed to the host
   * (wrapped and stored for the person, ADR 0081) BEFORE anything is published: the
   * SDK would pick the ID itself only when publishing, too late to bind it.
   */
  private async newRecoveryKey(): Promise<{ generated: GeneratedSecretStorageKey; keyID: string }> {
    const generated = await this.crypto.createRecoveryKeyFromPassphrase();
    if (!generated.encodedPrivateKey) throw new MatrixSecurityError('recovery');
    const keyID = secureRandomString(32);
    await this.ui.saveRecoveryKey(generated.encodedPrivateKey, keyID);
    return { generated, keyID };
  }
  /** What `bootstrapSecretStorage` does with a new key, with the ID chosen above. */
  private async publishRecoveryKey({
    generated,
    keyID,
  }: {
    generated: GeneratedSecretStorageKey;
    keyID: string;
  }): Promise<void> {
    if (this.closed) throw new MatrixSecurityError('device');
    const added = await this.client.secretStorage.addKey(
      SECRET_STORAGE_ALGORITHM_V1_AES,
      {
        key: generated.privateKey,
        ...(generated.keyInfo?.passphrase ? { passphrase: generated.keyInfo.passphrase } : {}),
        ...(generated.keyInfo?.name ? { name: generated.keyInfo.name } : {}),
      },
      keyID,
    );
    await this.client.secretStorage.setDefaultKeyId(added.keyId);
    this.client.cryptoCallbacks.cacheSecretStorageKey?.(
      added.keyId,
      added.keyInfo,
      generated.privateKey,
    );
  }
  /**
   * The recovery key of secret storage key `keyID` if this device already holds it (its
   * owner-key cache, or the key it just used), without asking anyone; null otherwise. A
   * device verified before ADR 0084 moves its account to the `sytue-id` wrapper with it
   * (ADR 0085), so the person does not sign in again for it.
   */
  async heldRecoveryKey(keyID: string): Promise<string | null> {
    if (this.closed) return null;
    const found = await this.client.secretStorage.getKey(keyID).catch(() => null);
    if (!found || !hasSecretStorageCheck(found[1])) return null;
    const description = found[1];
    let key: Uint8Array<ArrayBuffer> | null = this.temporaryKeys.get(keyID)?.slice() ?? null;
    if (key && !(await this.client.secretStorage.checkKey(key, description))) {
      key.fill(0);
      key = null;
    }
    if (!key && this.ownerKeys) key = await this.ownerKeys.held(keyID, description);
    if (!key || this.closed) {
      key?.fill(0);
      return null;
    }
    try {
      return encodeRecoveryKey(key) ?? null;
    } finally {
      key.fill(0);
    }
  }
  async recover(): Promise<void> {
    // A prepared sync does not guarantee that Rust has fetched our public
    // cross-signing identity. Import requires it; never replace the identity
    // merely because that asynchronous query has not finished yet.
    await this.crypto.getUserDeviceInfo([this.bootstrap.matrix_user_id], true);
    if (!(await this.crypto.isSecretStorageReady())) await this.completeSecretStorage();
    await this.crypto.bootstrapCrossSigning({
      authUploadDeviceSigningKeys: (request) => this.interactiveAuth(request),
    });
    await this.crypto.loadSessionBackupPrivateKeyFromSecretStorage();
    await this.crypto.restoreKeyBackup();
    // Cross-signing uploaded this device's signature. Refresh the SDK cache
    // before checking the device instead of racing the next background sync.
    await this.crypto.getUserDeviceInfo([this.bootstrap.matrix_user_id], true);
    await this.ownerKeys?.flush();
    if (!(await this.settledTrust())) throw new MatrixSecurityError('device');
    if (this.bootstrap.storage_product) {
      await this.loadAttestedStorageSessions(this.vaultScope());
    }
  }
  /**
   * Secret storage without everything it should hold under its current key: a setup cut
   * short (under v3.20, the backup made but its key never stored). If this device keeps
   * the missing secrets in the clear — cross-signing keys only when they ARE the
   * published identity, the backup key only when it opens the current backup — they are
   * stored under the current key, which this device holds (the caller opened it). Nothing
   * new is created; otherwise the account stays as it is (`recovery`).
   */
  private async completeSecretStorage(): Promise<void> {
    const status = await this.crypto.getCrossSigningStatus();
    const cached = status.privateKeysCachedLocally;
    const identity =
      cached.masterKey &&
      cached.selfSigningKey &&
      cached.userSigningKey &&
      !!(await this.crypto.getCrossSigningKeyId(CrossSigningKey.Master));
    const anyCached =
      cached.masterKey || cached.selfSigningKey || cached.userSigningKey ? identity : true;
    if (
      !anyCached ||
      !(identity || (await this.crypto.getSessionBackupPrivateKey())) ||
      !(await this.client.secretStorage.hasKey())
    )
      throw new MatrixSecurityError('recovery');
    await this.crypto.bootstrapSecretStorage({ setupNewKeyBackup: false });
    if (!(await this.crypto.isSecretStorageReady())) throw new MatrixSecurityError('recovery');
  }
  async backup(): Promise<void> {
    if (this.closed || !(await this.trusted())) throw new MatrixSecurityError('device');
    const backup = await this.crypto.checkKeyBackupAndEnable();
    if (!backup?.trustInfo.trusted) throw new MatrixSecurityError('device');
    await this.crypto.flushKeyBackup();
    if (this.closed) throw new MatrixSecurityError('device');
    if (this.bootstrap.storage_product) {
      await this.attestLiveStorageSessions(this.vaultScope());
    }
  }
  async putSecret(id: string, payload: Uint8Array, revision = 1): Promise<void> {
    if (this.closed || !(await this.trusted())) throw new MatrixSecurityError('device');
    await this.putSecretInternal(this.vaultScope(), id, payload, revision);
  }
  async readSecret(
    id: string,
    minRevision = 1,
  ): Promise<{ payload: Uint8Array<ArrayBuffer>; revision: number } | null> {
    if (this.closed) throw new MatrixSecurityError('device');
    return this.readSecretInternal(this.vaultScope(), id, minRevision);
  }
  private vaultScope(): MatrixStorageScope {
    return {
      tenant: this.bootstrap.tenant,
      subject: this.bootstrap.actor,
      product: this.bootstrap.storage_product ?? 'rooms',
    };
  }
  private async putSecretInternal(
    scope: MatrixStorageScope,
    id: string,
    payload: Uint8Array,
    revision: number,
  ): Promise<void> {
    if (!this.client?.secretStorage) throw new MatrixSecurityError('recovery');
    const encoded = await encodeSsssVaultSecret(id, scope, revision, payload);
    await this.client.secretStorage.store(`${ssssVaultEventPrefix}${id}`, encoded);
    if (this.closed) throw new MatrixSecurityError('device');
  }
  private async readSecretInternal(
    scope: MatrixStorageScope,
    id: string,
    minRevision = 1,
  ): Promise<{ payload: Uint8Array<ArrayBuffer>; revision: number } | null> {
    if (!storageIdentifier(id) || !Number.isSafeInteger(minRevision) || minRevision < 1) {
      throw new StorageRecordError();
    }
    if (!this.client?.secretStorage) throw new MatrixSecurityError('recovery');
    const eventType = `${ssssVaultEventPrefix}${id}`;
    let raw: string | undefined;
    try {
      raw = await this.client.secretStorage.get(eventType);
    } catch (error) {
      if (error instanceof MatrixSecurityError) throw error;
      throw new MatrixSecurityError('identity');
    }
    if (this.closed) throw new MatrixSecurityError('device');
    if (raw === undefined) return null;
    return decodeSsssVaultSecret(raw, id, scope, minRevision);
  }
  private sessionTupleKey(entry: AttestedStorageSession): string {
    return `${entry.room_id}\u0000${entry.session_id}\u0000${entry.sender_key}`;
  }
  private extractEncryptedSession(
    roomId: string,
    raw: Partial<IEvent>,
  ): AttestedStorageSession | null {
    const content = raw.content;
    if (!content || typeof content !== 'object' || Array.isArray(content)) return null;
    const record = content as Record<string, unknown>;
    if (record.algorithm !== 'm.megolm.v1.aes-sha2') return null;
    const candidate = {
      room_id: roomId,
      session_id: record.session_id,
      sender_key: record.sender_key,
    };
    return isValidAttestedStorageSession(candidate) ? candidate : null;
  }
  private recordLiveStorageSession(
    entry: AttestedStorageSession,
    resolvedRef?: MatrixReference,
  ): void {
    this.liveStorageSessions ??= new Map();
    this.liveStorageSessions.set(this.sessionTupleKey(entry), entry);
    if (resolvedRef && this.pendingStorageEventRefs) {
      this.pendingStorageEventRefs = this.pendingStorageEventRefs.filter(
        (ref) => !(ref.room_id === resolvedRef.room_id && ref.event_id === resolvedRef.event_id),
      );
    }
  }
  private queueStorageEventAttestation(ref: MatrixReference): void {
    this.pendingStorageEventRefs ??= [];
    this.pendingStorageEventRefs.push(ref);
    if (this.pendingStorageEventRefs.length > 128) {
      this.pendingStorageEventRefs.splice(0, this.pendingStorageEventRefs.length - 128);
    }
  }
  private cacheAttestedStorageSessions(entries: readonly AttestedStorageSession[]): void {
    this.attestedStorageSessions ??= new Map();
    for (const entry of entries) {
      this.attestedStorageSessions.set(this.sessionTupleKey(entry), entry);
    }
  }
  private async loadAttestedStorageSessions(scope: MatrixStorageScope): Promise<boolean> {
    if (!this.client?.secretStorage) return false;
    const secretId = await storageSessionsSecretId(scope);
    let secret: { payload: Uint8Array<ArrayBuffer>; revision: number } | null;
    try {
      secret = await this.readSecretInternal(scope, secretId, 1);
    } catch (error) {
      if (
        error instanceof MatrixSecurityError &&
        (error.reason === 'recovery' || error.reason === 'identity')
      ) {
        return false;
      }
      throw error;
    }
    if (!secret) return false;
    try {
      const entries = decodeAttestedStorageSessions(secret.payload);
      this.cacheAttestedStorageSessions(entries);
      return true;
    } catch (error) {
      if (error instanceof MatrixSecurityError && error.reason === 'identity') {
        return false;
      }
      throw error;
    } finally {
      secret.payload.fill(0);
    }
  }
  private async isAttestedRecoveredStorageEvent(
    scope: MatrixStorageScope,
    roomId: string,
    raw: Partial<IEvent>,
  ): Promise<boolean> {
    const candidate = this.extractEncryptedSession(roomId, raw);
    if (!candidate) return false;
    const key = this.sessionTupleKey(candidate);
    if (this.attestedStorageSessions?.has(key)) return true;
    if (!(await this.loadAttestedStorageSessions(scope))) return false;
    return this.attestedStorageSessions?.has(key) === true;
  }
  private async attestLiveStorageSessions(scope: MatrixStorageScope): Promise<void> {
    const pending = [...(this.pendingStorageEventRefs ?? [])];
    if (this.pendingStorageEventRefs) this.pendingStorageEventRefs.length = 0;
    for (const ref of pending) {
      const raw = await this.client.fetchRoomEvent(ref.room_id, ref.event_id);
      if (this.closed) throw new MatrixSecurityError('device');
      if (
        raw.type !== String(EventType.RoomMessageEncrypted) ||
        raw.sender !== this.bootstrap.matrix_user_id ||
        raw.event_id !== ref.event_id
      ) {
        continue;
      }
      const event = new MatrixEvent({ ...raw, room_id: ref.room_id });
      await this.client.decryptEventIfNeeded(event);
      if (this.closed) throw new MatrixSecurityError('device');
      if (event.isDecryptionFailure()) continue;
      const trust = await this.crypto.getEncryptionInfoForEvent(event);
      if (this.closed) throw new MatrixSecurityError('device');
      if (
        trust?.shieldColour === EventShieldColour.NONE &&
        (!event.getKeyForwardingUser() ||
          event.getKeyForwardingUser() === this.bootstrap.matrix_user_id)
      ) {
        const live = this.extractEncryptedSession(ref.room_id, raw);
        if (live) this.recordLiveStorageSession(live);
      }
    }
    if (!this.liveStorageSessions || this.liveStorageSessions.size === 0) return;
    const secretId = await storageSessionsSecretId(scope);
    const existing = await this.readSecretInternal(scope, secretId, 1);
    let combined: AttestedStorageSession[];
    try {
      const prior = existing ? decodeAttestedStorageSessions(existing.payload) : [];
      combined = normalizeAttestedStorageSessions([...prior, ...this.liveStorageSessions.values()]);
    } finally {
      existing?.payload.fill(0);
    }
    const payload = encodeAttestedStorageSessions(combined);
    try {
      await this.putSecretInternal(scope, secretId, payload, (existing?.revision ?? 0) + 1);
      this.cacheAttestedStorageSessions(combined);
    } finally {
      payload.fill(0);
    }
  }
  requestVerification(): Promise<VerificationRequest> {
    return this.crypto.requestOwnUserVerification();
  }
  async devices(): Promise<MatrixDevice[]> {
    const { devices } = await this.client.getDevices();
    return Promise.all(
      devices.map(async (device) => ({
        id: device.device_id,
        name: device.display_name ?? device.device_id,
        current: device.device_id === this.client.getDeviceId(),
        verified:
          (
            await this.crypto.getDeviceVerificationStatus(
              this.bootstrap.matrix_user_id,
              device.device_id,
            )
          )?.crossSigningVerified === true,
      })),
    );
  }
  async revokeDevice(id: string): Promise<void> {
    if (id === this.client.getDeviceId()) throw new MatrixSecurityError('device');
    await this.interactiveAuth((auth) => this.client.deleteDevice(id, auth ?? undefined));
  }
  /** Only Sytue human identities can be verified; the provisioning controller is never a recipient. */
  canVerifyUser(user: string): boolean {
    if (this.bootstrap.storage_product) return user === this.bootstrap.matrix_user_id;
    const suffix = `:${new URL(this.bootstrap.homeserver_url).host}`;
    if (!user.endsWith(suffix)) return false;
    return /^@sytue_[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(
      user.slice(0, -suffix.length),
    );
  }
  private isController(user: string): boolean {
    return user === this.bootstrap.controller_user_id;
  }
  private async guardRecipients(): Promise<void> {
    await assertControllerUntrusted(this.crypto, this.bootstrap.controller_user_id);
  }
  private requireVerification(): never {
    window.dispatchEvent(new Event('sytue-matrix-verification-required'));
    throw new MatrixSecurityError('unverified');
  }
  private async applyRecipientPolicy(room: string): Promise<void> {
    await this.guardRecipients();
    if (this.policyRooms.has(room)) return;
    await this.crypto.forceDiscardSession(room);
    this.policyRooms.add(room);
  }
  async unverifiedRecipients(): Promise<{ user: string; device: string; name: string }[]> {
    const users = new Set(this.verificationPeers);
    for (const binding of this.bootstrap.channels) {
      const room = this.client.getRoom(binding.matrix_room_id);
      for (const member of room ? await room.getEncryptionTargetMembers() : [])
        if (member.userId !== this.bootstrap.matrix_user_id && this.canVerifyUser(member.userId))
          users.add(member.userId);
    }
    const pending = [];
    for (const user of users) {
      if ((await this.crypto.getUserVerificationStatus(user)).isCrossSigningVerified()) continue;
      const info = await this.crypto.getUserDeviceInfo([user], true);
      for (const [device, entry] of info.get(user) ?? [])
        pending.push({ user, device, name: entry.displayName ?? device });
    }
    return pending;
  }
  verifyRecipient(user: string, device: string): Promise<VerificationRequest> {
    if (!this.canVerifyUser(user)) return Promise.reject(new MatrixSecurityError('unverified'));
    return this.crypto.requestDeviceVerification(user, device);
  }
  private async joinEncryptedRoom(id: string): Promise<void> {
    if (this.client.getRoom(id)?.getMyMembership() !== 'join') await this.client.joinRoom(id);
    const check = async (): Promise<boolean> =>
      this.client.getRoom(id)?.getMyMembership() === 'join' &&
      (await this.crypto.isEncryptionEnabledInRoom(id)) &&
      this.client
        .getRoom(id)
        ?.currentState.getStateEvents(EventType.RoomEncryption, '')
        ?.getContent().algorithm === 'm.megolm.v1.aes-sha2';
    if (await check()) return;
    await new Promise<void>((resolve, reject) => {
      let done = false;
      const finish = (error?: Error): void => {
        if (done) return;
        done = true;
        clearTimeout(timer);
        this.client.off(ClientEvent.Sync, sync);
        if (error) reject(error);
        else resolve();
      };
      const sync = (): void => {
        void check()
          .then((ready) => {
            if (ready) finish();
          })
          .catch(() => finish(new MatrixSecurityError('room')));
      };
      const timer = setTimeout(() => finish(new MatrixSecurityError('room')), 15000);
      this.client.on(ClientEvent.Sync, sync);
      sync();
    });
  }
  private async room(channel: string, sending = false): Promise<string> {
    if (this.bootstrap.storage_product) throw new MatrixSecurityError('unsupported');
    if (this.closed || !(await this.trusted())) throw new MatrixSecurityError('device');
    let id = this.bootstrap.channels.find((item) => item.channel_id === channel)?.matrix_room_id;
    if (!id) {
      const next = await this.adapter.refreshBootstrap();
      if (
        next.actor !== this.bootstrap.actor ||
        next.tenant !== this.bootstrap.tenant ||
        next.matrix_user_id !== this.bootstrap.matrix_user_id ||
        next.homeserver_url !== this.bootstrap.homeserver_url
      )
        throw new MatrixSecurityError('identity');
      this.bootstrap.channels = next.channels;
      id = next.channels.find((item) => item.channel_id === channel)?.matrix_room_id;
    }
    if (!id) throw new MatrixSecurityError('room');
    await this.joinEncryptedRoom(id);
    if (!(await this.crypto.isEncryptionEnabledInRoom(id))) throw new MatrixSecurityError('room');
    const state = this.client
      .getRoom(id)
      ?.currentState.getStateEvents(EventType.RoomEncryption, '')
      ?.getContent();
    if (state?.algorithm !== 'm.megolm.v1.aes-sha2') throw new MatrixSecurityError('room');
    if (sending) {
      await this.applyRecipientPolicy(id);
      const room = this.client.getRoom(id);
      if (!room) throw new MatrixSecurityError('room');
      for (const member of await room.getEncryptionTargetMembers()) {
        if (member.userId === this.bootstrap.matrix_user_id || this.isController(member.userId))
          continue;
        if (!this.canVerifyUser(member.userId)) this.requireVerification();
        const trust = await this.crypto.getUserVerificationStatus(member.userId);
        if (!trust.isCrossSigningVerified() || trust.needsUserApproval) this.requireVerification();
      }
    }
    return id;
  }
  async send(
    channel: string,
    content: RoomsEncryptedContent,
    txnId: string,
    relation?: { thread?: MatrixReference; replaces?: MatrixReference },
    attachmentRoom?: string,
  ): Promise<MatrixReference> {
    const room = await this.room(channel, true);
    if (this.closed || (content.attachments?.length && attachmentRoom !== room))
      throw new MatrixSecurityError('reference');
    const payload: IContent = {
      msgtype: 'm.text',
      body: content.body,
      'org.sytue.rooms': {
        version: 1,
        ...content,
        attachments: content.attachments?.map(({ objectUrl: _url, ...file }) => file),
      },
    };
    if (relation?.thread) {
      if (relation.thread.room_id !== room) throw new MatrixSecurityError('reference');
      payload['m.relates_to'] = {
        rel_type: 'm.thread',
        event_id: relation.thread.event_id,
        is_falling_back: false,
      };
    }
    if (relation?.replaces) {
      if (relation.replaces.room_id !== room) throw new MatrixSecurityError('reference');
      payload['m.new_content'] = { ...payload };
      payload['m.relates_to'] = { rel_type: 'm.replace', event_id: relation.replaces.event_id };
    }
    const result = await this.client.sendEvent(
      room,
      EventType.RoomMessage,
      payload as RoomMessageEventContent,
      txnId,
    );
    return { room_id: room, event_id: result.event_id };
  }
  private deviceJob<T>(action: (signal: AbortSignal) => Promise<T>): Promise<T> {
    const jobs = (this.storageJobs ??= new StorageJobs());
    return storageWork(
      (name, task, parent) => jobs.run(name, task, parent),
      () => {
        if (this.closed) throw new MatrixSecurityError('device');
      },
      this.network?.lifetime?.signal,
      action,
    );
  }
  async read(
    channel: string,
    reference: MatrixReference,
    authorSub: string,
  ): Promise<RoomsEncryptedContent> {
    return this.deviceJob(() => this.readRoomEvent(channel, reference, authorSub));
  }
  private async readRoomEvent(
    channel: string,
    reference: MatrixReference,
    authorSub: string,
  ): Promise<RoomsEncryptedContent> {
    const room = await this.room(channel);
    if (reference.room_id !== room || !reference.event_id.startsWith('$'))
      throw new MatrixSecurityError('reference');
    const raw = await this.client.fetchRoomEvent(room, reference.event_id);
    if (
      raw.type !== String(EventType.RoomMessageEncrypted) ||
      raw.event_id !== reference.event_id ||
      raw.sender !== `@sytue_${authorSub}:${new URL(this.bootstrap.homeserver_url).host}` ||
      (raw.room_id && raw.room_id !== room)
    )
      throw new MatrixSecurityError('reference');
    const event = new MatrixEvent({ ...raw, room_id: room });
    await this.client.decryptEventIfNeeded(event);
    if (event.isDecryptionFailure() || event.getType() !== String(EventType.RoomMessage))
      throw new MatrixSecurityError('unverified');
    const trust = await this.crypto.getEncryptionInfoForEvent(event);
    if (
      !trust ||
      (trust.shieldColour === EventShieldColour.RED &&
        trust.shieldReason !== EventShieldReason.AUTHENTICITY_NOT_GUARANTEED) ||
      (trust.shieldColour !== EventShieldColour.NONE &&
        trust.shieldReason !== EventShieldReason.UNVERIFIED_IDENTITY &&
        trust.shieldReason !== EventShieldReason.AUTHENTICITY_NOT_GUARANTEED)
    )
      throw new MatrixSecurityError('unverified');
    await this.authorizeRoomEvent(event, trust);
    const identity = await this.crypto.getUserVerificationStatus(event.getSender() ?? '');
    if (identity.needsUserApproval) throw new MatrixSecurityError('unverified');
    if (!identity.isCrossSigningVerified())
      await this.crypto.pinCurrentUserIdentity(event.getSender() ?? '');
    const content = event.getContent();
    const own = content['org.sytue.rooms'] as RoomsEncryptedContent | undefined;
    // Standard Matrix m.file/m.image events from the native SDK use the same encrypted v2 descriptor.
    const file = content.file as MatrixFile | undefined;
    const info = content.info as { mimetype?: unknown; size?: unknown } | undefined;
    const nativeAttachment =
      !own?.attachments &&
      file?.v === 'v2' &&
      typeof file.url === 'string' &&
      file.url.startsWith('mxc://')
        ? [
            {
              fileId: file.url,
              name:
                typeof content.filename === 'string'
                  ? content.filename
                  : String(content.body ?? ''),
              contentType:
                typeof info?.mimetype === 'string' ? info.mimetype : 'application/octet-stream',
              sizeBytes: typeof info?.size === 'number' ? info.size : 0,
              matrixFile: file,
            },
          ]
        : undefined;
    const attachments = own?.attachments ?? nativeAttachment;
    if (typeof content.body !== 'string') throw new MatrixSecurityError('reference');
    if (
      attachments &&
      (!Array.isArray(attachments) ||
        attachments.length > 10 ||
        attachments.some(
          (file) =>
            !file ||
            'objectUrl' in file ||
            typeof file.name !== 'string' ||
            typeof file.contentType !== 'string' ||
            !file.matrixFile ||
            !Number.isSafeInteger(file.sizeBytes) ||
            file.sizeBytes < 0 ||
            file.sizeBytes > 50 * 1024 * 1024,
        ) ||
        attachments.reduce((sum, file) => sum + file.sizeBytes, 0) > 50 * 1024 * 1024)
    )
      throw new MatrixSecurityError('reference');
    if (this.closed) throw new MatrixSecurityError('device');
    return {
      ...own,
      ...(attachments ? { attachments } : {}),
      body: content.body,
      authenticityVerified: true,
      identityVerified: trust.shieldColour === EventShieldColour.NONE,
    };
  }
  private async privateDraftRoom(id: string): Promise<void> {
    if (this.closed || !(await this.trusted()) || !isMatrixRoomId(id))
      throw new MatrixSecurityError('device');
    await this.joinEncryptedRoom(id);
    if (!(await this.crypto.isEncryptionEnabledInRoom(id))) throw new MatrixSecurityError('room');
    const state = this.client
      .getRoom(id)
      ?.currentState.getStateEvents(EventType.RoomEncryption, '')
      ?.getContent();
    if (state?.algorithm !== 'm.megolm.v1.aes-sha2') throw new MatrixSecurityError('room');
    const room = this.client.getRoom(id);
    if (!room) throw new MatrixSecurityError('room');
    await this.applyRecipientPolicy(id);
    if (
      (await room.getEncryptionTargetMembers()).some(
        (member) =>
          member.userId !== this.bootstrap.matrix_user_id && !this.isController(member.userId),
      )
    )
      throw new MatrixSecurityError('unverified');
  }
  /** The owner's symmetric secret stays in this authorized client. */
  private async storageAuthKey(requested?: string): Promise<StorageOwnerKey> {
    if (this.closed) throw new MatrixSecurityError('device');
    if (requested !== undefined && !ownerKeyID(requested))
      throw new MatrixSecurityError('unverified');
    const entry = await this.client.secretStorage.getKey(requested);
    if (this.closed) throw new MatrixSecurityError('device');
    if (
      !entry ||
      !ownerKeyID(entry[0]) ||
      !hasSecretStorageCheck(entry[1]) ||
      (requested !== undefined && entry[0] !== requested)
    )
      throw new MatrixSecurityError('recovery');
    const [id, description] = entry;
    if (this.ownerKeys) {
      const [savedID, key] = await this.ownerKeys.get({ [id]: description });
      if (this.closed) throw new MatrixSecurityError('device');
      return { id: savedID, key };
    }
    const cached = this.temporaryKeys.get(id);
    if (cached && (await this.client.secretStorage.checkKey(cached, description))) {
      if (this.closed) throw new MatrixSecurityError('device');
      return { id, key: cached };
    }
    const encoded = await this.ui.readRecoveryKey([id]);
    let key: Uint8Array<ArrayBuffer>;
    try {
      key = decodeRecoveryKey(encoded.trim());
    } catch {
      throw new MatrixSecurityError('recovery');
    }
    try {
      if (this.closed) throw new MatrixSecurityError('device');
      if (!(await this.client.secretStorage.checkKey(key, description)))
        throw new MatrixSecurityError('recovery');
      if (this.closed) throw new MatrixSecurityError('device');
      this.temporaryKeys.set(id, key);
      return { id, key };
    } catch (error) {
      key.fill(0);
      throw error;
    }
  }
  private async appOwner(basePath: string): Promise<OpaqueSessionOwner> {
    this.assertSDKOpen();
    if (!verifiedSuiteOwners.has(this)) throw new MatrixSecurityError('identity');
    const base = withBase(basePath, '/');
    if (this.appSessionBase !== undefined && this.appSessionBase !== base)
      throw new MatrixSecurityError('identity');
    if (this.appSessionOwner) {
      verifySessionOwner(this.appSessionOwner, {
        actor: this.bootstrap.actor,
        tenant: this.bootstrap.tenant,
      });
      await this.appSessionOwner.revalidate();
      return this.appSessionOwner;
    }
    if (this.openingAppOwner) return this.openingAppOwner;
    this.appSessionBase = base;
    const pending = openSessionOwner(
      { actor: this.bootstrap.actor, tenant: this.bootstrap.tenant },
      base,
      this.network.lifetime?.signal,
    ).then(async (owner) => {
      try {
        this.assertSDKOpen();
        verifySessionOwner(owner, { actor: this.bootstrap.actor, tenant: this.bootstrap.tenant });
        this.appSessionOwner = owner;
        owner.signal.addEventListener('abort', () => this.close(), { once: true });
        if (owner.signal.aborted) this.close();
        this.assertSDKOpen();
        return owner;
      } catch (error) {
        await owner.closeAndDrain();
        throw error;
      }
    });
    this.openingAppOwner = pending;
    try {
      return await pending;
    } finally {
      if (this.openingAppOwner === pending) this.openingAppOwner = undefined;
    }
  }
  private scopedServiceHttp(owner: OpaqueSessionOwner, basePath: string): HttpClient {
    const scope = this.vaultScope();
    verifySessionOwner(owner, { actor: scope.subject, tenant: scope.tenant });
    return new HttpClient({
      baseUrl: new URL(withBase(basePath, '/api'), globalThis.location.origin).toString(),
      fetchImpl: owner.fetch,
      defaultHeaders: { [storageScopeHeader]: encodeStorageScopeHeader(scope) },
    });
  }
  private immutableBlobPort(owner: OpaqueSessionOwner, basePath: string): StorageBlobPort {
    const scope = Object.freeze({ ...this.vaultScope() });
    verifySessionOwner(owner, { actor: scope.subject, tenant: scope.tenant });
    const client = new DriveClient({
      baseUrl: new URL(withBase(basePath, '/'), globalThis.location.origin)
        .toString()
        .replace(/\/$/, ''),
      fetchImpl: owner.fetch,
    });
    return Object.freeze({
      read: (id: string, signal?: AbortSignal) => client.readCipherBlob(scope, id, signal),
      write: (id: string, bytes: Uint8Array<ArrayBuffer>, signal?: AbortSignal) =>
        client.putCipherBlob(scope, id, bytes, signal),
    });
  }
  private assertSDKOpen(): void {
    if (
      this.closed ||
      this.network.lifetime?.signal.aborted ||
      this.client.getUserId() !== this.bootstrap.matrix_user_id ||
      this.client.getDeviceId() !== this.session?.device_id
    )
      throw new MatrixSecurityError('device');
  }
  private async snapshotSDK(): Promise<Readonly<SDKLeaseSnapshot>> {
    this.assertSDKOpen();
    if (!(await this.trusted())) throw new MatrixSecurityError('unverified');
    const rootKey = await this.crypto.getCrossSigningKeyId(),
      entry = await this.client.secretStorage.getKey(),
      backupVersion = await this.crypto.getActiveSessionBackupVersion();
    this.assertSDKOpen();
    if (
      !publicStorageKey(rootKey) ||
      !entry ||
      !ownerKeyID(entry[0]) ||
      !hasSecretStorageCheck(entry[1]) ||
      !backupVersion
    )
      throw new MatrixSecurityError('recovery');
    return Object.freeze({
      homeserver: this.bootstrap.homeserver_url,
      userID: this.bootstrap.matrix_user_id,
      deviceID: this.client.getDeviceId()!,
      rootKey,
      recoveryKeyID: entry[0],
      backupVersion,
    });
  }
  private async revalidateSDK(expected: Readonly<SDKLeaseSnapshot>): Promise<void> {
    const actual = await this.snapshotSDK();
    for (const key of Object.keys(expected) as Array<keyof SDKLeaseSnapshot>)
      if (actual[key] !== expected[key]) throw new MatrixSecurityError('identity');
  }
  private coupleSDKLifetime(legacy: SuiteMatrix): void {
    this.assertSDKOpen();
    legacy.assertSDKOpen();
    if (this.lifetimePeer === legacy && legacy.lifetimePeer === this) return;
    if (this.lifetimePair || legacy.lifetimePair) throw new MatrixSecurityError('locked');
    const member = (owner: SuiteMatrix) => ({
      retire: () => owner.close(),
      drainIO: () => owner.localJobsClosing ?? Promise.reject(new MatrixSecurityError('device')),
      erase: () => owner.eraseAfterDrain(),
    });
    const pair = new SDKLifetimePair([member(this), member(legacy)]);
    this.lifetimePair = legacy.lifetimePair = pair;
    this.lifetimePeer = legacy;
    legacy.lifetimePeer = this;
  }
  private bothSDKJobs<T>(
    legacy: SuiteMatrix,
    action: (signal: AbortSignal) => Promise<T>,
    signal?: AbortSignal,
  ): Promise<T> {
    this.assertSDKOpen();
    legacy.assertSDKOpen();
    const own = (this.storageJobs ??= new StorageJobs()),
      previous = (legacy.storageJobs ??= new StorageJobs());
    return own.run(
      crypto.randomUUID(),
      (current) => previous.run(crypto.randomUUID(), action, current),
      signal,
    );
  }
  private async identityCutover(
    host: HistoricalStorageHost,
    signal: AbortSignal,
  ): Promise<ValidatedIdentityCutover> {
    this.assertSDKOpen();
    signal.throwIfAborted();
    const raw = await host.readCutover(signal);
    let validated: Uint8Array | undefined;
    try {
      validated = await this.clientCoreRuntime().invoke<Uint8Array>(
        { operation: 'storage.cutover.decode', scope: this.vaultScope(), state: raw },
        signal,
      );
      this.assertSDKOpen();
      signal.throwIfAborted();
      return JSON.parse(
        new TextDecoder('utf-8', { fatal: true }).decode(validated),
      ) as ValidatedIdentityCutover;
    } finally {
      raw.fill(0);
      validated?.fill(0);
    }
  }
  /** Compatibility getter for the single maintained crypto kernel. */
  clientCoreRuntime(): PqcRuntime {
    return this.clientModuleRuntime('core');
  }
  /** Lazy processors share this exact immutable SDK/account/product lease.
   * Consumers close handles, while SuiteMatrix drains every module together. */
  clientModuleRuntime(module: ClientCoreModule): PqcRuntime {
    if (this.closed || !this.network.lifetime) throw new MatrixSecurityError('device');
    const previous = this.clientModules.get(module);
    if (previous) return previous;
    const deviceID = this.client.getDeviceId();
    if (!deviceID) throw new MatrixSecurityError('device');
    const matrixUserID = this.bootstrap.matrix_user_id;
    const assertOpen = (): void => {
      if (
        this.closed ||
        this.network.lifetime?.signal.aborted ||
        this.client.getDeviceId() !== deviceID ||
        this.client.getUserId() !== matrixUserID
      )
        throw new MatrixSecurityError('device');
    };
    assertOpen();
    const binding = Object.freeze({
      scope: Object.freeze(this.vaultScope()),
      matrixUserID,
      deviceID,
    });
    const runtime = new PqcRuntime(
      { signal: this.network.lifetime.signal, assertOpen, binding },
      module,
      (capability) => {
        assertOpen();
        const entry = this.hostDelegations.get(capability);
        if (!entry || entry.module !== module || entry.used)
          throw new MatrixSecurityError('device');
        entry.grant.options.lease.assertOpen();
        entry.used = true;
        return entry.grant;
      },
    );
    this.clientModules.set(module, runtime);
    return runtime;
  }
  /** Canonical SDK1 remains a real independent SDK. Its read port is minted
   * here, never assembled from a caller-provided binding or identity version. */
  private async registerHistoricalStorageReader(
    legacy: SuiteMatrix,
    host: HistoricalStorageHost,
    group: PqcPortGroup,
    runtime: PqcRuntime,
    module: ClientCoreModule,
    blobs?: StorageBlobPort,
    signal?: AbortSignal,
  ): Promise<HostPortHandle> {
    this.assertSDKOpen();
    legacy.assertSDKOpen();
    validateBootstrap(this.bootstrap);
    validateBootstrap(legacy.bootstrap);
    const scope = this.vaultScope(),
      oldScope = legacy.vaultScope();
    if (
      legacy === this ||
      this.bootstrap.homeserver_url !== legacy.bootstrap.homeserver_url ||
      this.bootstrap.storage_identity_version !== 2 ||
      legacy.bootstrap.storage_identity_version === 2 ||
      scope.product === 'rooms' ||
      scope.tenant !== oldScope.tenant ||
      scope.subject !== oldScope.subject ||
      scope.product !== oldScope.product ||
      typeof host.readCutover !== 'function'
    )
      throw new MatrixSecurityError('identity');
    const source = Object.freeze({ readCutover: host.readCutover.bind(host) });
    const initial = await (this.storageJobs ??= new StorageJobs()).run(
      crypto.randomUUID(),
      (current) => this.identityCutover(source, current),
      signal,
    );
    if (initial.phase === 'isolated') throw new MatrixSecurityError('unsupported');
    // Coupling retirement grants no data access. The opaque port is issued only
    // after both trusted SDK snapshots and actual product routing are verified.
    this.coupleSDKLifetime(legacy);
    return this.bothSDKJobs(
      legacy,
      async (current) => {
        const [ownSnapshot, oldSnapshot] = await Promise.all([
          this.snapshotSDK(),
          legacy.snapshotSDK(),
        ]);
        const repository = legacy.storage(blobs);
        await repository.prepare(current);
        const kernel = runtime;
        let portID = '';
        const adapter = new PqcStoragePort(repository, kernel, () => portID);
        const check = async (signal: AbortSignal): Promise<ValidatedIdentityCutover> => {
          signal.throwIfAborted();
          await Promise.all([this.revalidateSDK(ownSnapshot), legacy.revalidateSDK(oldSnapshot)]);
          const state = await this.identityCutover(source, signal);
          signal.throwIfAborted();
          return state;
        };
        await check(current);
        const handlers: Partial<Record<Exclude<HostMethod, 'current'>, HostHandler>> = {};
        for (const method of historicalReadMethods) {
          if (method === 'current') continue;
          const handler = adapter.handlers[method];
          if (handler) handlers[method] = handler;
        }
        const fences = new Map<string, string>();
        handlers['storage.frozen'] = async (_raw, ctx) => {
          const state = await check(ctx.signal),
            binding = repository.binding();
          if (state.phase !== 'frozen' && state.phase !== 'isolated')
            throw new MatrixSecurityError('locked');
          if (
            !Array.isArray(state.source_rooms) ||
            state.source_rooms.length !== binding.rooms.length ||
            binding.rooms.some((room) => !state.source_rooms.includes(room))
          )
            throw new MatrixSecurityError('identity');
          const found: MatrixReference[] = [];
          for (const room of binding.rooms) {
            ctx.assertOpen();
            const response = await legacy.client.http.authedRequest<Blob>(
              Method.Get,
              '/rooms/' + encodeURIComponent(room) + '/state',
              {},
              undefined,
              {
                rawResponseBody: true,
                headers: { Accept: 'application/json' },
                abortSignal: ctx.signal,
              },
            );
            if (response.size > 2 << 20) throw new MatrixSecurityError('reference');
            const raw = new Uint8Array(await response.arrayBuffer());
            try {
              const reference = await this.clientCoreRuntime().invoke<MatrixReference>(
                {
                  operation: 'storage.frozen.validate',
                  scope: oldScope,
                  room,
                  own: oldSnapshot.userID,
                  controller: legacy.bootstrap.controller_user_id,
                  state: raw,
                },
                ctx.signal,
              );
              if (
                reference.room_id !== room ||
                (fences.has(room) && fences.get(room) !== reference.event_id)
              )
                throw new MatrixSecurityError('identity');
              fences.set(room, reference.event_id);
              found.push(reference);
            } finally {
              raw.fill(0);
            }
          }
          const after = await check(ctx.signal);
          ctx.assertOpen();
          if (
            (after.phase !== 'frozen' && after.phase !== 'isolated') ||
            after.epoch !== state.epoch ||
            !Array.isArray(after.source_rooms) ||
            after.source_rooms.length !== binding.rooms.length ||
            binding.rooms.some((room) => !after.source_rooms.includes(room))
          )
            throw new MatrixSecurityError('identity');
          return new TextEncoder().encode(JSON.stringify({ binding, fences: found }));
        };
        const lease = {
          signal: group.signal,
          assertOpen: () => {
            group.assertOpen();
            this.assertSDKOpen();
            legacy.assertSDKOpen();
            if (this.lifetimePair?.retired) throw new MatrixSecurityError('device');
          },
        };
        const token = Object.freeze(Object.create(null) as object);
        const grant: ResolvedHostDelegation = {
          binding: Object.freeze({
            scope: Object.freeze({ ...oldScope }),
            matrixUserID: oldSnapshot.userID,
            deviceID: oldSnapshot.deviceID,
          }),
          options: {
            scope: oldScope,
            lease,
            run: <T>(
              action: (signal: AbortSignal) => Promise<T>,
              parent: AbortSignal,
            ): Promise<T> =>
              this.bothSDKJobs(
                legacy,
                async (active) => {
                  await check(active);
                  const value = await action(active);
                  try {
                    await check(active);
                    return value;
                  } catch (error) {
                    eraseOwnedBytes(value);
                    throw error;
                  }
                },
                parent,
              ),
            handlers,
            cleanup: () => adapter.close(),
          },
        };
        this.hostDelegations.set(token, { module, used: false, grant });
        try {
          const port = kernel.registerDelegatedHostPort(token);
          portID = port.id;
          return port;
        } catch (error) {
          await adapter.close();
          throw error;
        }
      },
      signal,
    );
  }
  private async registerHistoricalStorageWriter(
    legacy: SuiteMatrix,
    host: HistoricalStorageHost,
    group: PqcPortGroup,
    runtime: PqcRuntime,
    module: ClientCoreModule,
    blobs: StorageBlobPort,
    signal?: AbortSignal,
  ): Promise<HostPortHandle> {
    group.assertOpen();
    this.assertSDKOpen();
    legacy.assertSDKOpen();
    const scope = this.vaultScope(),
      oldScope = legacy.vaultScope();
    if (
      !verifiedSuiteOwners.has(this) ||
      !verifiedSuiteOwners.has(legacy) ||
      this.lifetimePeer !== legacy ||
      this.bootstrap.storage_identity_version !== 2 ||
      legacy.bootstrap.storage_identity_version === 2 ||
      this.bootstrap.homeserver_url !== legacy.bootstrap.homeserver_url ||
      scope.tenant !== oldScope.tenant ||
      scope.subject !== oldScope.subject ||
      scope.product !== oldScope.product
    )
      throw new MatrixSecurityError('identity');
    return this.bothSDKJobs(
      legacy,
      async (current) => {
        const [ownSnapshot, oldSnapshot] = await Promise.all([
          this.snapshotSDK(),
          legacy.snapshotSDK(),
        ]);
        const check = async (active: AbortSignal): Promise<ValidatedIdentityCutover> => {
          group.assertOpen();
          active.throwIfAborted();
          await Promise.all([this.revalidateSDK(ownSnapshot), legacy.revalidateSDK(oldSnapshot)]);
          const state = await this.identityCutover(host, active);
          group.assertOpen();
          return state;
        };
        if ((await check(current)).phase !== 'legacy') throw new MatrixSecurityError('locked');
        const records = legacy.storage(blobs);
        await records.prepare(current);
        const kernel = runtime;
        let portID = '';
        const adapter = new PqcStoragePort(records, kernel, () => portID);
        const methods: readonly HostMethod[] = [
          'storage.prepare',
          'storage.bindings',
          'storage.recoverable',
          'storage.target',
          'storage.record.put',
          'storage.object.put',
          'storage.file.upload',
          'storage.file.ack',
        ];
        const publications = new Set<HostMethod>([
          'storage.record.put',
          'storage.object.put',
          'storage.file.upload',
        ]);
        const handlers: Partial<Record<Exclude<HostMethod, 'current'>, HostHandler>> = {};
        for (const method of methods) {
          if (method === 'current') continue;
          const original = adapter.handlers[method];
          if (!original) throw new MatrixSecurityError('unsupported');
          handlers[method] = async (raw, ctx) => {
            const before = await check(ctx.signal);
            ctx.assertOpen();
            if (method !== 'storage.file.ack' && before.phase !== 'legacy')
              throw new MatrixSecurityError('locked');
            if (publications.has(method)) {
              const value = decodeHostObject(raw, () => {
                throw new MatrixSecurityError('reference');
              });
              if (
                typeof value.authorization !== 'string' ||
                !/^[0-9a-f]{64}$/.test(value.authorization)
              )
                throw new MatrixSecurityError('unverified');
            }
            const output = await original(raw, ctx);
            try {
              const after = await check(ctx.signal);
              ctx.assertOpen();
              if (method !== 'storage.file.ack' && after.phase !== 'legacy')
                throw new MatrixSecurityError('locked');
              return output;
            } catch (error) {
              output.fill(0);
              throw error;
            }
          };
        }
        const lease = {
          signal: group.signal,
          assertOpen: () => {
            group.assertOpen();
            this.assertSDKOpen();
            legacy.assertSDKOpen();
          },
        };
        const token = Object.freeze(Object.create(null) as object);
        const grant: ResolvedHostDelegation = {
          binding: Object.freeze({
            scope: Object.freeze({ ...oldScope }),
            matrixUserID: oldSnapshot.userID,
            deviceID: oldSnapshot.deviceID,
          }),
          options: {
            scope: oldScope,
            lease,
            run: <T>(action: (active: AbortSignal) => Promise<T>, parent: AbortSignal) =>
              this.bothSDKJobs(legacy, action, parent),
            handlers,
            cleanup: () => adapter.close(),
          },
        };
        this.hostDelegations.set(token, { module, used: false, grant });
        try {
          const port = kernel.registerDelegatedHostPort(token);
          portID = port.id;
          return port;
        } catch (error) {
          await adapter.close();
          throw error;
        }
      },
      signal,
    );
  }
  /** Open one account-bound Mail engine. All routing, SDK bindings and phase
   * decisions come from the authenticated host and the same Go core. */
  private validateHistoricalIdentity(legacy: SuiteMatrix): void {
    if (
      !verifiedSuiteOwners.has(legacy) ||
      legacy === this ||
      legacy.bootstrap.storage_identity_version === 2 ||
      !this.bootstrap.storage_product ||
      legacy.bootstrap.storage_product !== this.bootstrap.storage_product ||
      legacy.bootstrap.tenant !== this.bootstrap.tenant ||
      legacy.bootstrap.actor !== this.bootstrap.actor ||
      legacy.bootstrap.homeserver_url !== this.bootstrap.homeserver_url
    )
      throw new MatrixSecurityError('identity');
  }
  async prepareDeviceStorage(
    options: { basePath?: string } = {},
    signal?: AbortSignal,
  ): Promise<{
    readonly storage: StorageRepository;
    readonly scope: MatrixStorageScope;
  }> {
    this.assertSDKOpen();
    if (
      !verifiedSuiteOwners.has(this) ||
      !this.bootstrap.storage_product ||
      this.bootstrap.storage_identity_version !== 2
    )
      throw new MatrixSecurityError('identity');
    const basePath = options.basePath ?? '/',
      activeSignal = signal ?? this.network.lifetime!.signal;
    activeSignal.throwIfAborted();
    const owner = await this.appOwner(basePath),
      scope = Object.freeze({ ...this.vaultScope() }),
      kernel = this.clientCoreRuntime(),
      ownSnapshot = await this.snapshotSDK(),
      http = this.scopedServiceHttp(owner, basePath),
      blobs = this.immutableBlobPort(owner, basePath);
    const revalidate = async (active: AbortSignal): Promise<void> => {
      this.assertSDKOpen();
      active.throwIfAborted();
      verifySessionOwner(owner, { actor: scope.subject, tenant: scope.tenant });
      await owner.revalidate(active);
      await this.revalidateSDK(ownSnapshot);
      this.assertSDKOpen();
      active.throwIfAborted();
    };
    const [authorityMethod, authorityPath] = privateServiceRoute('rooms.authority');
    if (authorityMethod !== 'GET') throw new MatrixSecurityError('unsupported');
    this.publicationAdmission = new StoragePublicationAdmission({
      scope,
      userID: ownSnapshot.userID,
      deviceID: ownSnapshot.deviceID,
      rootKey: ownSnapshot.rootKey,
      runtime: kernel,
      revalidate,
      authority: async (active) =>
        http.bytes({
          path: authorityPath,
          method: 'GET',
          maxResponseBytes: 16384,
          signal: active,
        }),
      reserve: async (body, active) =>
        http.bytes({
          path: '/matrix/storage/publications/reserve',
          method: 'POST',
          body,
          headers: { 'Content-Type': 'application/json' },
          maxResponseBytes: 16384,
          signal: active,
        }),
      recovery: async (roomID, transaction, active) => {
        await revalidate(active);
        if (!this.bootstrap.storage_room_ids?.includes(roomID))
          throw new MatrixSecurityError('reference');
        const query = new URLSearchParams({ room_id: roomID, transaction_id: transaction });
        try {
          return await http.bytes({
            path: '/matrix/storage/publications/recovery?' + query.toString(),
            method: 'GET',
            maxResponseBytes: 32768,
            signal: active,
          });
        } catch (error) {
          if (error instanceof HttpError && error.status === 404) return undefined;
          throw error;
        }
      },
      accepted: async (reference, active) => {
        await revalidate(active);
        if (!this.bootstrap.storage_room_ids?.includes(reference.room_id))
          throw new MatrixSecurityError('reference');
        const raw = await this.client.http.authedRequest<IEvent>(
          Method.Get,
          '/rooms/' +
            encodeURIComponent(reference.room_id) +
            '/event/' +
            encodeURIComponent(reference.event_id),
          undefined,
          undefined,
          { abortSignal: active },
        );
        await revalidate(active);
        return new MatrixEvent({ ...raw, room_id: raw.room_id ?? reference.room_id });
      },
      sign: async (payload) => await this.crypto.signJsonObject(payload),
    });
    await this.ensureStorageAuthority(
      createStorageRootService(http, scope, authorityPath.replace(/\/$/, '')),
      activeSignal,
    );
    await revalidate(activeSignal);
    const storage = this.storage(blobs);
    await storage.prepare(activeSignal);
    await revalidate(activeSignal);
    return Object.freeze({ storage, scope });
  }
  async preparePrivateStorage(
    options: {
      basePath?: string;
      legacy?: SuiteMatrix;
      resolveLegacy?: (signal: AbortSignal) => Promise<SuiteMatrix>;
    } = {},
    signal?: AbortSignal,
  ): Promise<{
    readonly storage: StorageRepository;
    readonly scope: MatrixStorageScope;
    readonly phase: ValidatedIdentityCutover['phase'];
    readonly legacy?: SuiteMatrix;
  }> {
    let legacy = options.legacy;
    const resolveLegacy = options.resolveLegacy;
    if (legacy) {
      this.validateHistoricalIdentity(legacy);
      legacy.assertSDKOpen();
    }
    if (resolveLegacy !== undefined && typeof resolveLegacy !== 'function')
      throw new MatrixSecurityError('identity');
    const basePath = options.basePath ?? '/',
      activeSignal = signal ?? this.network.lifetime!.signal;
    const { storage, scope } = await this.prepareDeviceStorage({ basePath }, activeSignal);
    const owner = await this.appOwner(basePath),
      http = this.scopedServiceHttp(owner, basePath),
      blobs = this.immutableBlobPort(owner, basePath),
      ownSnapshot = await this.snapshotSDK();
    const revalidate = async (active: AbortSignal): Promise<void> => {
      this.assertSDKOpen();
      active.throwIfAborted();
      verifySessionOwner(owner, { actor: scope.subject, tenant: scope.tenant });
      await owner.revalidate(active);
      await this.revalidateSDK(ownSnapshot);
      this.assertSDKOpen();
      active.throwIfAborted();
    };
    const source: HistoricalStorageHost = Object.freeze({
      readCutover: async (active: AbortSignal) => {
        await revalidate(active);
        const [method, path] = privateServiceRoute('rooms.cutover.state');
        const raw = await http.bytes({
          method,
          path,
          maxResponseBytes: 64 << 10,
          signal: active,
        });
        try {
          await revalidate(active);
          return raw;
        } catch (error) {
          raw.fill(0);
          throw error;
        }
      },
    });
    const initial = await this.identityPlan(source, activeSignal);
    if (initial.require_legacy && !legacy) {
      if (!resolveLegacy) throw new MatrixSecurityError('recovery');
      const candidate = await resolveLegacy(activeSignal);
      this.validateHistoricalIdentity(candidate);
      legacy = candidate;
      candidate.assertSDKOpen();
      this.coupleSDKLifetime(candidate);
      await revalidate(activeSignal);
    }
    if (initial.require_legacy && legacy) {
      this.coupleSDKLifetime(legacy);
      const legacyRecords = legacy.storage(blobs);
      await legacyRecords.prepare(activeSignal);
      await revalidate(activeSignal);
    }
    return Object.freeze({
      storage,
      scope,
      phase: initial.phase,
      ...(initial.require_legacy && legacy ? { legacy } : {}),
    });
  }
  private async identityPlan(
    host: HistoricalStorageHost,
    signal: AbortSignal,
  ): Promise<{
    phase: ValidatedIdentityCutover['phase'];
    require_legacy: boolean;
  }> {
    this.assertSDKOpen();
    signal.throwIfAborted();
    const raw = await host.readCutover(signal);
    try {
      const plan = await this.clientCoreRuntime().invoke<{
        phase: ValidatedIdentityCutover['phase'];
        require_legacy: boolean;
      }>({ operation: 'storage.identity.plan', scope: this.vaultScope(), state: raw }, signal);
      this.assertSDKOpen();
      signal.throwIfAborted();
      if (!plan || typeof plan.require_legacy !== 'boolean' || typeof plan.phase !== 'string')
        throw new MatrixSecurityError('identity');
      return plan;
    } finally {
      raw.fill(0);
    }
  }
  openMailPrivate(
    options: {
      basePath?: string;
      legacy?: SuiteMatrix;
      resolveLegacy?: (signal: AbortSignal) => Promise<SuiteMatrix>;
    } = {},
    signal?: AbortSignal,
  ): Promise<MailPrivatePorts> {
    this.assertSDKOpen();
    if (
      !verifiedSuiteOwners.has(this) ||
      this.bootstrap.storage_product !== 'mail' ||
      this.bootstrap.storage_identity_version !== 2
    )
      throw new MatrixSecurityError('identity');
    const legacy = options.legacy,
      resolveLegacy = options.resolveLegacy;
    if (legacy) {
      this.validateHistoricalIdentity(legacy);
      legacy.assertSDKOpen();
    }
    if (resolveLegacy !== undefined && typeof resolveLegacy !== 'function')
      throw new MatrixSecurityError('identity');
    if (this.mailPrivateOpening) return this.mailPrivateOpening;
    // Unknown or in-progress work must never be represented as pending=0.
    this.destructionGuard = () => Promise.reject(new MatrixSecurityError('locked'));
    // Cancellation also owns the initial BFF owner request, before any port
    // group exists. The SDK lifetime aborts that request and joins its cleanup.
    const retire = () => this.close();
    signal?.addEventListener('abort', retire, { once: true });
    if (signal?.aborted) retire();
    const pending = this.createMailPrivate(options.basePath ?? '/', legacy, resolveLegacy, signal);
    this.mailPrivateOpening = pending;
    void pending.finally(() => signal?.removeEventListener('abort', retire)).catch(() => {});
    return pending;
  }
  private async createMailPrivate(
    basePath: string,
    providedLegacy: SuiteMatrix | undefined,
    resolveLegacy: ((signal: AbortSignal) => Promise<SuiteMatrix>) | undefined,
    signal?: AbortSignal,
  ): Promise<MailPrivatePorts> {
    let legacy = providedLegacy;
    const owner = await this.appOwner(basePath),
      scope = Object.freeze({ ...this.vaultScope() }),
      kernel = this.clientCoreRuntime(),
      domain = this.clientModuleRuntime('mail');
    const group = new PqcPortGroup([
      owner.signal,
      this.network.lifetime!.signal,
      ...(legacy ? [legacy.network.lifetime!.signal] : []),
      ...(signal ? [signal] : []),
    ]);
    const retire = () => this.close();
    group.signal.addEventListener('abort', retire, { once: true });
    let closing: Promise<void> | undefined;
    const closeAndDrain = (): Promise<void> => {
      if (closing) return closing;
      closing = (async () => {
        const errors: unknown[] = [];
        try {
          await group.closeAndDrain();
        } catch (error) {
          errors.push(error);
        }
        try {
          await owner.closeAndDrain();
        } catch (error) {
          errors.push(error);
        }
        this.close();
        if (legacy) legacy.close();
        const joined = await Promise.allSettled([
          this.closeAndDrain(),
          ...(legacy ? [legacy.closeAndDrain()] : []),
        ]);
        for (const result of joined) if (result.status === 'rejected') errors.push(result.reason);
        if (errors.length) throw new AggregateError(errors, 'matrix_private_mail_close');
      })();
      void closing.catch(() => {});
      return closing;
    };
    try {
      group.assertOpen();
      const ownSnapshot = await this.snapshotSDK();
      const http = this.scopedServiceHttp(owner, basePath),
        blobs = this.immutableBlobPort(owner, basePath);
      const revalidate = async (active: AbortSignal): Promise<void> => {
        group.assertOpen();
        active.throwIfAborted();
        verifySessionOwner(owner, { actor: scope.subject, tenant: scope.tenant });
        await owner.revalidate(active);
        await this.revalidateSDK(ownSnapshot);
        group.assertOpen();
        active.throwIfAborted();
      };
      const run = <T>(
        action: (active: AbortSignal) => Promise<T>,
        parent: AbortSignal,
        validate: (active: AbortSignal) => Promise<void> = revalidate,
      ): Promise<T> => {
        const jobs = (this.storageJobs ??= new StorageJobs());
        return jobs.run(
          crypto.randomUUID(),
          async (active) => {
            await validate(active);
            const value = await action(active);
            try {
              await validate(active);
              return value;
            } catch (error) {
              eraseOwnedBytes(value);
              throw error;
            }
          },
          parent,
        );
      };
      const [authorityMethod, authorityPath] = privateServiceRoute('rooms.authority');
      if (authorityMethod !== 'GET') throw new MatrixSecurityError('unsupported');
      this.publicationAdmission = new StoragePublicationAdmission({
        scope,
        userID: ownSnapshot.userID,
        deviceID: ownSnapshot.deviceID,
        rootKey: ownSnapshot.rootKey,
        runtime: kernel,
        revalidate,
        authority: async (active) =>
          http.bytes({
            path: authorityPath,
            method: 'GET',
            maxResponseBytes: 16384,
            signal: active,
          }),
        reserve: async (body, active) =>
          http.bytes({
            path: '/matrix/storage/publications/reserve',
            method: 'POST',
            body,
            headers: { 'Content-Type': 'application/json' },
            maxResponseBytes: 16384,
            signal: active,
          }),
        recovery: async (roomID, transaction, active) => {
          await revalidate(active);
          if (!this.bootstrap.storage_room_ids?.includes(roomID))
            throw new MatrixSecurityError('reference');
          const query = new URLSearchParams({ room_id: roomID, transaction_id: transaction });
          try {
            return await http.bytes({
              path: '/matrix/storage/publications/recovery?' + query.toString(),
              method: 'GET',
              maxResponseBytes: 32768,
              signal: active,
            });
          } catch (error) {
            if (error instanceof HttpError && error.status === 404) return undefined;
            throw error;
          }
        },
        accepted: async (reference, active) => {
          await revalidate(active);
          if (!this.bootstrap.storage_room_ids?.includes(reference.room_id))
            throw new MatrixSecurityError('reference');
          const raw = await this.client.http.authedRequest<IEvent>(
            Method.Get,
            '/rooms/' +
              encodeURIComponent(reference.room_id) +
              '/event/' +
              encodeURIComponent(reference.event_id),
            undefined,
            undefined,
            { abortSignal: active },
          );
          await revalidate(active);
          return new MatrixEvent({ ...raw, room_id: raw.room_id ?? reference.room_id });
        },
        sign: async (payload) => await this.crypto.signJsonObject(payload),
      });
      await this.ensureStorageAuthority(
        createStorageRootService(http, scope, authorityPath.replace(/\/$/, '')),
        group.signal,
      );
      await revalidate(group.signal);
      const source: HistoricalStorageHost = Object.freeze({
        readCutover: async (active: AbortSignal) => {
          await revalidate(active);
          const [method, path] = privateServiceRoute('rooms.cutover.state');
          const raw = await http.bytes({
            method,
            path,
            maxResponseBytes: 64 << 10,
            signal: active,
          });
          try {
            await revalidate(active);
            return raw;
          } catch (error) {
            raw.fill(0);
            throw error;
          }
        },
      });
      const initial = await this.identityPlan(source, group.signal);
      if (initial.require_legacy && !legacy) {
        if (!resolveLegacy) throw new MatrixSecurityError('recovery');
        const candidate = await resolveLegacy(group.signal);
        this.validateHistoricalIdentity(candidate);
        // Own a verified same-scope result before cancellation checks so a late
        // resolver is still drained. A foreign SDK is never adopted or closed.
        legacy = candidate;
        candidate.assertSDKOpen();
        group.assertOpen();
        this.coupleSDKLifetime(candidate);
        await revalidate(group.signal);
      }
      // The temporary source port cannot mutate SQL, read originals or publish.
      const unavailable = (): Promise<never> =>
        Promise.reject(new MatrixSecurityError('unsupported'));
      const sourceAdapter = new PqcSourcePort(scope, group, {
        state: async (active) => {
          await revalidate(active);
          const [method, path] = privateServiceRoute('mail.source.state');
          return http.bytes({ method, path, maxResponseBytes: 64 << 10, signal: active });
        },
        begin: unavailable,
        page: unavailable,
        open: unavailable,
      });
      const temporary = group.add(
        kernel.registerHostPort({
          scope,
          lease: group,
          run,
          handlers: sourceAdapter.capabilitiesHandlers(revalidate),
          cleanup: () => sourceAdapter.close(),
        }),
      );
      let maximum: number;
      try {
        const capabilities = await kernel.invoke<{
          v: number;
          scope: MatrixStorageScope;
          max_body_bytes: number;
        }>({ operation: 'mail.capabilities', port: temporary.id, scope }, group.signal);
        if (
          capabilities.v !== 1 ||
          capabilities.scope.tenant !== scope.tenant ||
          capabilities.scope.subject !== scope.subject ||
          capabilities.scope.product !== scope.product ||
          !Number.isSafeInteger(capabilities.max_body_bytes) ||
          capabilities.max_body_bytes <= 0 ||
          capabilities.max_body_bytes > 2 ** 47
        )
          throw new MatrixSecurityError('reference');
        maximum = capabilities.max_body_bytes;
      } finally {
        temporary.revoke();
        await temporary.drain();
      }
      group.assertOpen();
      const historical = initial.require_legacy ? legacy : undefined;
      if (historical) this.coupleSDKLifetime(historical);
      let oldSnapshot: Readonly<SDKLeaseSnapshot> | undefined;
      if (historical) oldSnapshot = await historical.snapshotSDK();
      const check = async (active: AbortSignal) => {
        await revalidate(active);
        if (historical && oldSnapshot) await historical.revalidateSDK(oldSnapshot);
        group.assertOpen();
      };
      const workWitness = {
        revalidate: check,
        acquire: async (active: AbortSignal) => {
          await check(active);
          const releases: Array<() => void | Promise<void>> = [];
          try {
            releases.push(await owner.acquire(active));
            releases.push(await (this.storageJobs ??= new StorageJobs()).retain(active));
            if (historical)
              releases.push(await (historical.storageJobs ??= new StorageJobs()).retain(active));
            await check(active);
            let release: Promise<void> | undefined;
            return () => {
              if (!release)
                release = Promise.allSettled(
                  releases.map((done) => Promise.resolve().then(done)),
                ).then((results) => {
                  if (results.some((result) => result.status === 'rejected'))
                    throw new MatrixSecurityError('device');
                });
              return release;
            };
          } catch (error) {
            await Promise.allSettled(releases.map((done) => Promise.resolve().then(done)));
            throw error;
          }
        },
      };
      const records = this.storage(blobs);
      await records.prepare(group.signal);
      await check(group.signal);
      let canonicalID = '';
      const storageAdapter = new PqcStoragePort(records, domain, () => canonicalID);
      const journal = new PqcEncryptedJournal(scope, () => !group.signal.aborted),
        work = new PqcWorkPort(scope, group, workWitness),
        secrets = new PqcIngressSecretsPort(records, this),
        control = new PqcIngressControlPort(records, this);
      const service = new PqcServicePort(
        scope,
        {
          signal: group.signal,
          assertOpen: () => group.assertOpen(),
          revalidate: check,
          fetch: owner.fetch,
        },
        domain,
        () => canonicalID,
        basePath,
        maximum,
      );
      const canonical = group.add(
        domain.registerHostPort({
          scope,
          lease: group,
          run: <T>(action: (active: AbortSignal) => Promise<T>, parent: AbortSignal) =>
            run(action, parent, check),
          handlers: {
            ...storageAdapter.handlers,
            ...journal.handlers,
            ...work.handlers,
            ...secrets.handlers,
            ...control.handlers,
            ...service.handlers,
            'identity.root': async (_raw, ctx) => {
              await check(ctx.signal);
              return encodeHostJSON({ root_key: ownSnapshot.rootKey });
            },
          },
          cleanup: async () => {
            const cleaned = await Promise.allSettled([
              Promise.resolve(storageAdapter.close()),
              journal.close(),
              work.close(),
              Promise.resolve(secrets.close()),
              Promise.resolve(control.close()),
              service.close(),
            ]);
            if (cleaned.some((result) => result.status === 'rejected'))
              throw new MatrixSecurityError('device');
          },
        }),
      );
      canonicalID = canonical.id;
      let reader: HostPortHandle | undefined, writer: HostPortHandle | undefined;
      if (historical) {
        reader = group.add(
          await this.registerHistoricalStorageReader(
            historical,
            source,
            group,
            domain,
            'mail',
            blobs,
            group.signal,
          ),
        );
        if (initial.phase === 'legacy')
          writer = group.add(
            await this.registerHistoricalStorageWriter(
              historical,
              source,
              group,
              domain,
              'mail',
              blobs,
              group.signal,
            ),
          );
      }
      await check(group.signal);
      const input = encodeHostJSON({
        ...(reader ? { legacy_port: reader.id } : {}),
        ...(writer ? { migration_writer_port: writer.id } : {}),
      });
      let opened: { session: string; scope: MatrixStorageScope; ready: boolean };
      try {
        opened = await domain.invoke(
          { operation: 'mail.open', port: canonicalID, scope, input },
          group.signal,
        );
      } finally {
        input.fill(0);
      }
      if (
        !opened ||
        typeof opened.session !== 'string' ||
        !/^[0-9a-f]{64}$/.test(opened.session) ||
        opened.ready !== true ||
        opened.scope.tenant !== scope.tenant ||
        opened.scope.subject !== scope.subject ||
        opened.scope.product !== scope.product
      )
        throw new MatrixSecurityError('identity');
      await check(group.signal);
      const engine = opened.session;
      this.destructionGuard = async () => {
        group.assertOpen();
        await check(group.signal);
        const input = encodeHostJSON({ session: engine });
        try {
          await domain.invoke(
            { operation: 'mail.pause', port: canonicalID, scope, input },
            group.signal,
          );
          const pending = await domain.invoke<{ scope: MatrixStorageScope; count: number }>(
            { operation: 'mail.pending', port: canonicalID, scope, input },
            group.signal,
          );
          if (
            !pending ||
            pending.scope.tenant !== scope.tenant ||
            pending.scope.subject !== scope.subject ||
            pending.scope.product !== scope.product ||
            !Number.isSafeInteger(pending.count) ||
            pending.count !== 0
          )
            throw new MatrixSecurityError('locked');
          await check(group.signal);
        } finally {
          input.fill(0);
        }
      };
      return Object.freeze({
        runtime: domain,
        port: canonicalID,
        ...(reader ? { legacy_port: reader.id } : {}),
        ...(writer ? { migration_writer_port: writer.id } : {}),
        scope,
        signal: group.signal,
        session: engine,
        closeAndDrain,
      });
    } catch (error) {
      try {
        await closeAndDrain();
      } catch {
        /* Preserve the original actionable failure; every resource attempted drain. */
      }
      throw error;
    }
  }
  private roomProvenance(): MatrixSessionProvenance {
    if (this.closed || this.bootstrap.storage_product) throw new MatrixSecurityError('device');
    const scope = this.vaultScope();
    return (this.roomSessionProofs ??= new MatrixSessionProvenance(
      this.clientCoreRuntime(),
      scope,
      {
        assertOpen: () => {
          if (this.closed) throw new MatrixSecurityError('device');
        },
        read: (id) => this.readSecretInternal(scope, id, 1),
        write: (id, payload) => this.putSecretInternal(scope, id, payload, 1),
      },
    ));
  }
  private sdkSession(event: MatrixEvent): MatrixSessionDescriptor {
    const room = event.getRoomId(),
      sender = event.getSender(),
      senderKey = event.getSenderKey(),
      signingKey = event.getClaimedEd25519Key();
    const wire = event.getWireContent();
    if (
      event.getWireType() !== String(EventType.RoomMessageEncrypted) ||
      wire.algorithm !== 'm.megolm.v1.aes-sha2' ||
      typeof wire.session_id !== 'string' ||
      !room ||
      !sender ||
      !senderKey ||
      !signingKey
    )
      throw new MatrixSecurityError('unverified');
    return {
      room_id: room,
      session_id: wire.session_id,
      sender_key: senderKey,
      sender_user: sender,
      signing_key: signingKey,
    };
  }
  private async attestOwnPublication(event: MatrixEvent): Promise<void> {
    if (this.closed || !(await this.trusted())) throw new MatrixSecurityError('device');
    const session = this.sdkSession(event);
    const own = await this.crypto.getOwnDeviceKeys();
    if (
      this.closed ||
      session.sender_user !== this.bootstrap.matrix_user_id ||
      session.sender_key !== own.curve25519 ||
      session.signing_key !== own.ed25519
    )
      throw new MatrixSecurityError('unverified');
    await this.roomProvenance().attest(session);
    if (this.closed) throw new MatrixSecurityError('device');
  }
  private async authorizeRoomEvent(
    event: MatrixEvent,
    trust: NonNullable<Awaited<ReturnType<CryptoApi['getEncryptionInfoForEvent']>>>,
    prompt = true,
  ): Promise<void> {
    if (this.closed) throw new MatrixSecurityError('device');
    const deny = (): never => {
      if (prompt) this.requireVerification();
      throw new MatrixSecurityError('unverified');
    };
    const session = this.sdkSession(event);
    const provenance = this.roomProvenance();
    if (
      trust.shieldReason === EventShieldReason.AUTHENTICITY_NOT_GUARANTEED ||
      event.getKeyForwardingUser()
    ) {
      if (!(await provenance.contains(session))) throw new MatrixSessionProvenanceMissing();
      return;
    }
    if (trust.shieldColour !== EventShieldColour.NONE || !this.canVerifyUser(session.sender_user))
      deny();
    const identity = await this.crypto.getUserVerificationStatus(session.sender_user);
    if (!identity.isCrossSigningVerified() || identity.needsUserApproval) deny();
    if (await provenance.contains(session)) return;
    const devices = (await this.crypto.getUserDeviceInfo([session.sender_user], true)).get(
      session.sender_user,
    );
    let verified = false;
    for (const [id, device] of devices ?? []) {
      if (
        device.verified === DeviceVerification.Blocked ||
        device.getIdentityKey() !== session.sender_key ||
        device.getFingerprint() !== session.signing_key
      )
        continue;
      const status = await this.crypto.getDeviceVerificationStatus(session.sender_user, id);
      if (status?.signedByOwner && status.crossSigningVerified) {
        verified = true;
        break;
      }
    }
    if (this.closed) throw new MatrixSecurityError('device');
    if (!verified) deny();
    await provenance.attest(session);
    if (this.closed) throw new MatrixSecurityError('device');
  }
  /** Migrate only sessions whose origin this SDK already authenticates. The
   * checkpoint contains opaque pagination metadata in the existing local cipher;
   * interrupted scans resume, and unresolved sessions are never self-attested. */
  backfillRoomSessionProvenance(): Promise<{
    scanned: number;
    authenticated: number;
    blocked: number;
  }> {
    if (this.roomBackfill) return this.roomBackfill;
    const pending = this.deviceJob(async (signal) => {
      if (this.bootstrap.storage_product || !(await this.trusted()))
        throw new MatrixSecurityError('device');
      const key = await this.client.secretStorage.getDefaultKeyId();
      if (!key) throw new MatrixSecurityError('recovery');
      const check = async (): Promise<void> => {
        if (this.closed || signal.aborted) throw new MatrixSecurityError('device');
        if (key !== (await this.client.secretStorage.getDefaultKeyId()))
          throw new MatrixSecurityError('recovery');
        if (this.closed || signal.aborted) throw new MatrixSecurityError('device');
      };
      const cache = (this.roomBackfillCache ??= new EncryptedOfflineValues(
        this.prefix,
        'rooms-provenance-backfill-v1',
        () => !this.closed,
      ));
      const rooms = new Set(
        [
          ...this.bootstrap.channels.map((entry) => entry.matrix_room_id),
          this.bootstrap.draft_room_id,
          ...(this.bootstrap.draft_room_ids ?? []),
        ].filter((id): id is string => !!id),
      );
      const report = { scanned: 0, authenticated: 0, blocked: 0 };
      for (const room of rooms) {
        await check();
        let progress: {
          v: 1;
          key: string;
          cursor: string | null;
          done: boolean;
          retry: { cursor: string | null } | null;
        } = { v: 1, key, cursor: null, done: false, retry: null };
        const stored = await cache.read(room);
        try {
          if (stored) {
            const value: unknown = JSON.parse(new TextDecoder().decode(stored));
            const p = value as typeof progress;
            if (
              !p ||
              p.v !== 1 ||
              typeof p.key !== 'string' ||
              (p.cursor !== null && typeof p.cursor !== 'string') ||
              typeof p.done !== 'boolean' ||
              (p.retry !== null &&
                (!p.retry || (p.retry.cursor !== null && typeof p.retry.cursor !== 'string')))
            )
              throw new MatrixSecurityError('recovery');
            if (p.key === key) progress = p;
          }
        } finally {
          stored?.fill(0);
        }
        if (progress.done) continue;
        for (;;) {
          await check();
          const cursor = progress.cursor;
          const page = await this.client.createMessagesRequest(
            room,
            cursor,
            100,
            Direction.Backward,
          );
          await check();
          for (const raw of page.chunk) {
            report.scanned++;
            if (raw.type !== String(EventType.RoomMessageEncrypted)) continue;
            const event = new MatrixEvent({ ...raw, room_id: room });
            await this.client.decryptEventIfNeeded(event);
            await check();
            if (!event.isDecryptionFailure() && event.getType() !== String(EventType.RoomMessage))
              continue;
            let authenticated = false;
            if (!event.isDecryptionFailure()) {
              const trust = await this.crypto.getEncryptionInfoForEvent(event);
              if (trust) {
                try {
                  await this.authorizeRoomEvent(event, trust, false);
                  authenticated = true;
                } catch (error) {
                  if (!(error instanceof MatrixSecurityError && error.reason === 'unverified'))
                    throw error;
                }
              }
            }
            await check();
            if (authenticated) report.authenticated++;
            else {
              report.blocked++;
              progress.retry ??= { cursor };
            }
          }
          const done = !page.end || page.end === cursor || page.chunk.length === 0;
          progress = {
            v: 1,
            key,
            cursor: done ? (progress.retry?.cursor ?? null) : page.end!,
            done: done && !progress.retry,
            retry: done ? null : progress.retry,
          };
          const bytes = new TextEncoder().encode(JSON.stringify(progress));
          try {
            await check();
            await cache.write(room, bytes);
            await check();
          } finally {
            bytes.fill(0);
          }
          if (done) break;
        }
      }
      await check();
      return report;
    });
    this.roomBackfill = pending;
    void pending
      .finally(() => {
        if (this.roomBackfill === pending) this.roomBackfill = undefined;
      })
      .catch(() => undefined);
    return pending;
  }
  private storagePq(): StoragePq {
    if (this.closed || !this.bootstrap.storage_product || !this.network.lifetime)
      throw new MatrixSecurityError('device');
    if (this.privateStoragePq) return this.privateStoragePq;
    const assertOpen = (): void => {
      if (this.closed) throw new MatrixSecurityError('device');
    };
    const core = this.clientCoreRuntime();
    const scope = this.vaultScope();
    const cache = this.storagePqCache;
    if (!cache) throw new MatrixSecurityError('device');
    const keys = new StoragePqKeys(
      core,
      {
        assertOpen,
        binding: async () => ({
          scope,
          identity_version: this.bootstrap.storage_identity_version === 2 ? 2 : 1,
          user_id: this.bootstrap.matrix_user_id,
          recovery_key_id: (await this.storageAuthKey()).id,
        }),
        read: (id) => this.readSecretInternal(scope, id, 1),
        write: (id, payload) => this.putSecretInternal(scope, id, payload, 1),
      },
      cache,
    );
    return (this.privateStoragePq = new StoragePq(core, keys, scope, assertOpen));
  }
  /** Client-only private data adapter. Other products cannot reuse a Rooms device or channel. */
  storage(blobs?: StorageBlobPort): StorageRepository {
    const product = this.bootstrap.storage_product;
    if (!product) throw new MatrixSecurityError('unsupported');
    const scope: MatrixStorageScope = {
      tenant: this.bootstrap.tenant,
      subject: this.bootstrap.actor,
      product,
    };
    const rooms = [...(this.bootstrap.storage_room_ids ?? [])];
    const writeRoom = this.bootstrap.storage_room_id;
    const assertOpen = (): void => {
      if (this.closed) throw new MatrixSecurityError('device');
    };
    assertOpen();
    if (!writeRoom || !rooms.includes(writeRoom)) throw new MatrixSecurityError('room');
    const jobs = (this.storageJobs ??= new StorageJobs());
    const prepare = async (room: string): Promise<void> => {
      assertOpen();
      if (!rooms.includes(room)) throw new MatrixSecurityError('reference');
      await this.privateDraftRoom(room);
      assertOpen();
    };
    const decodeEvent = async (
      raw: Partial<IEvent>,
      reference: MatrixReference,
    ): Promise<{
      event: MatrixEvent;
      trust: NonNullable<Awaited<ReturnType<CryptoApi['getEncryptionInfoForEvent']>>>;
    }> => {
      assertOpen();
      if (
        raw.type !== String(EventType.RoomMessageEncrypted) ||
        raw.sender !== this.bootstrap.matrix_user_id ||
        raw.event_id !== reference.event_id ||
        (raw.room_id && raw.room_id !== reference.room_id)
      )
        throw new MatrixSecurityError('reference');
      const event = new MatrixEvent({ ...raw, room_id: reference.room_id });
      await this.client.decryptEventIfNeeded(event);
      assertOpen();
      if (event.isDecryptionFailure()) throw new MatrixSecurityError('unverified');
      const trust = await this.crypto.getEncryptionInfoForEvent(event);
      assertOpen();
      // A decrypted byte string is not sufficient. Unknown, unsigned, changed
      // or unverifiably imported sender keys cannot publish private app records.
      // Backup-restored history is accepted when the record carries an owner
      // proof or the exact Megolm session is attested inside the owner's
      // authenticated SSSS vault.
      if (
        !trust ||
        (event.getKeyForwardingUser() &&
          event.getKeyForwardingUser() !== this.bootstrap.matrix_user_id)
      )
        throw new MatrixSecurityError('unverified');
      if (
        trust.shieldColour !== EventShieldColour.NONE &&
        trust.shieldReason !== EventShieldReason.AUTHENTICITY_NOT_GUARANTEED
      )
        throw new MatrixSecurityError('unverified');
      return { event, trust };
    };
    const readContent = async (reference: MatrixReference): Promise<IContent> => {
      await prepare(reference.room_id);
      const raw = await this.client.fetchRoomEvent(reference.room_id, reference.event_id);
      const { event } = await decodeEvent(raw, reference);
      if (event.getType() !== String(EventType.RoomMessage))
        throw new MatrixSecurityError('reference');
      return event.getContent();
    };
    const decode = async (
      raw: Partial<IEvent>,
      reference: MatrixReference,
    ): Promise<StorageRecord | null> => {
      const { event, trust } = await decodeEvent(raw, reference);
      let content = event.getContent();
      if (Object.prototype.hasOwnProperty.call(content, 'org.sytue.storage_pq')) {
        const opened = await this.storagePq().open(content, reference, readContent);
        if (!opened) throw new StorageRecordError();
        if ('fragment' in opened) return null;
        content = { 'org.sytue.storage': opened.record, 'org.sytue.storage_auth': opened.proof };
      }
      // Two attestations compose for recovered history: a record written with
      // an owner proof carries its own —the sealed proof below is always the
      // authority, whatever the sender's shield— while proofless events
      // publish only when the exact Megolm session is attested inside the
      // owner's authenticated SSSS vault.
      let attestedSession = false;
      if (trust.shieldColour === EventShieldColour.NONE) {
        const live = this.extractEncryptedSession(reference.room_id, raw);
        if (live) this.recordLiveStorageSession(live, reference);
      } else if (trust.shieldReason !== EventShieldReason.AUTHENTICITY_NOT_GUARANTEED) {
        throw new MatrixSecurityError('unverified');
      } else if (content['org.sytue.storage_auth'] === undefined) {
        attestedSession = await this.isAttestedRecoveredStorageEvent(scope, reference.room_id, raw);
        if (!attestedSession) throw new MatrixSecurityError('unverified');
      }
      assertOpen();
      if (event.getType() !== String(EventType.RoomMessage)) return null;
      const record = decodeStorageRecord(content['org.sytue.storage'], scope);
      const proof: unknown = content['org.sytue.storage_auth'];
      if (proof === undefined) {
        // Unsealed legacy imports remain blocked. Decryption or an authenticated
        // backup version alone cannot attest a private application record; the
        // sole exception is the SSSS-attested recovered session accepted above.
        if (!attestedSession && (!trust || trust.shieldColour !== EventShieldColour.NONE))
          throw new MatrixSecurityError('unverified');
      } else {
        // Owner attestation uses standard SSSS under a symmetric secret unavailable
        // to the homeserver or anonymous backup writers. A present proof is always
        // checked, even when the outer SDK already trusts the live sender.
        const checkedProof = decodeOwnerProof(proof);
        const key = await this.storageAuthKey(checkedProof.key_id);
        await verifyStorageOwnerRecord(reference.room_id, record, scope, checkedProof, key);
      }
      assertOpen();
      return record;
    };
    const putRecord = async (
      transaction: string,
      input: StorageRecord,
      target?: StoragePublicationTarget,
      policy?: StoragePublicationPolicy,
      parent?: AbortSignal,
    ): Promise<MatrixReference> => {
      parent?.throwIfAborted();
      policy?.assertOpen();
      const publicationSignal = policy
        ? parent
          ? AbortSignal.any([parent, policy.signal])
          : policy.signal
        : parent;
      if (
        policy &&
        (policy.target.room_id !== (target?.room_id ?? writeRoom) ||
          policy.target.device_id !== this.client.getDeviceId())
      )
        throw new MatrixSecurityError('reference');
      if (target && target.device_id !== this.session?.device_id)
        throw new MatrixSecurityError('device');
      const destination = target?.room_id ?? writeRoom;
      const record = decodeStorageRecord(input, scope);
      await prepare(destination);
      const key = await this.storageAuthKey();
      const proof = await sealStorageOwnerRecord(destination, record, scope, key);
      assertOpen();
      const proofBytes = new TextEncoder().encode(JSON.stringify(proof));
      let ref: MatrixReference;
      try {
        ref = await this.storagePq().write(
          destination,
          transaction,
          record,
          proofBytes,
          async (txn, content) => {
            await prepare(destination);
            const signal = publicationSignal ?? this.network?.lifetime?.signal;
            const device =
              typeof this.client.getDeviceId === 'function'
                ? this.client.getDeviceId()
                : this.session?.device_id;
            if (!signal || !device) throw new MatrixSecurityError('device');
            const actualTransaction = 'sytue-storage-' + txn;
            const recovered = await this.publicationAdmission?.recoverBeforeSend(
              destination,
              actualTransaction,
              signal,
            );
            if (recovered) return recovered;
            const jobs = (this.publicationJobs ??= new StoragePublicationJobs(() => {
              if (this.closed) throw new MatrixSecurityError('device');
            }));
            const result = await jobs.run(
              { room_id: destination, device_id: device },
              actualTransaction,
              signal,
              policy,
              () =>
                this.client.sendEvent(
                  destination,
                  EventType.RoomMessage,
                  content as RoomMessageEventContent,
                  actualTransaction,
                ),
            );
            assertOpen();
            return storageReference({ room_id: destination, event_id: result.event_id });
          },
          readContent,
        );
      } finally {
        proofBytes.fill(0);
      }
      const accepted = await decode(
        await this.client.fetchRoomEvent(ref.room_id, ref.event_id),
        ref,
      );
      if (!accepted || !sameStorageRecord(record, accepted))
        throw new MatrixSecurityError('reference');
      assertOpen();
      this.queueStorageEventAttestation(ref);
      return ref;
    };
    const transport: StorageRecordTransport = {
      recoverable: async (target, signal) => {
        signal.throwIfAborted();
        assertOpen();
        if (target.device_id !== this.client.getDeviceId() || !rooms.includes(target.room_id))
          throw new MatrixSecurityError('reference');
        if (!(await this.trusted())) throw new MatrixSecurityError('recovery');
        await this.ownerKeys?.flush();
        assertOpen();
        signal.throwIfAborted();
      },
      prepare: async (signal) => {
        signal.throwIfAborted();
        await prepare(writeRoom);
        await this.storageAuthKey();
        await this.storagePq().keys.prepare();
        await this.ownerKeys?.flush();
        assertOpen();
        signal.throwIfAborted();
      },
      run: (name, action, parent) => jobs.run(name, action, parent),
      exclusive: (action, parent) => jobs.exclusive('catalog', action, parent),
      rooms,
      ...(this.session
        ? { publication: { room_id: writeRoom, device_id: this.session.device_id } }
        : {}),
      assertOpen,
      put: (transaction, input, target, signal) =>
        putRecord(transaction, input, target, undefined, signal),
      putAuthorized: (transaction, input, target, policy, signal) =>
        putRecord(transaction, input, target, policy, signal),
      read: async (input) => {
        const ref = storageReference(input);
        await prepare(ref.room_id);
        const raw = await this.client.fetchRoomEvent(ref.room_id, ref.event_id);
        const record = await decode(raw, ref);
        if (!record) throw new StorageRecordError();
        return record;
      },
      history: async (room, cursor, limit) => {
        await prepare(room);
        const page = await this.client.createMessagesRequest(
          room,
          cursor,
          limit,
          Direction.Backward,
        );
        assertOpen();
        if (
          !Array.isArray(page.chunk) ||
          page.chunk.length > limit ||
          (page.end !== undefined &&
            page.end !== null &&
            (typeof page.end !== 'string' || page.end.length > 8192))
        )
          throw new StorageRecordError('incomplete');
        const entries: StorageRecordEntry[] = [];
        for (const raw of page.chunk) {
          if (raw.type === String(EventType.RoomMessage))
            throw new MatrixSecurityError('unverified');
          if (raw.type !== String(EventType.RoomMessageEncrypted)) continue;
          const reference = storageReference({ room_id: room, event_id: raw.event_id });
          const record = await decode(raw, reference);
          if (record) entries.push({ reference, record });
        }
        assertOpen();
        return { entries, scanned: page.chunk.length, next: page.end ?? null };
      },
    };
    return new StorageRepository(scope, transport, blobs, this.uploadCache);
  }
  async saveDraft(channelId: string, parentId: string | undefined, body: string): Promise<void> {
    if (
      !this.bootstrap.channels.some((entry) => entry.channel_id === channelId) ||
      body.length > 65536
    )
      throw new MatrixSecurityError('reference');
    const id = this.bootstrap.draft_room_id;
    if (!id) throw new MatrixSecurityError('room');
    await this.privateDraftRoom(id);
    await this.client.sendEvent(
      id,
      EventType.RoomMessage,
      {
        msgtype: 'm.text',
        body,
        'org.sytue.rooms.draft': {
          v: 1,
          tenant: this.bootstrap.tenant,
          channel_id: channelId,
          ...(parentId ? { parent_id: parentId } : {}),
          updated_at: new Date().toISOString(),
        },
      } as RoomMessageEventContent,
      crypto.randomUUID(),
    );
  }
  async drafts(): Promise<MatrixDraft[]> {
    return this.deviceJob(() => this.readDrafts());
  }
  private async readDrafts(): Promise<MatrixDraft[]> {
    const rooms = new Set(
      [this.bootstrap.draft_room_id, ...(this.bootstrap.draft_room_ids ?? [])].filter(
        (id): id is string => Boolean(id),
      ),
    );
    const latest = new Map<string, MatrixDraft & { timestamp: number }>();
    for (const id of rooms) {
      await this.privateDraftRoom(id);
      let cursor: string | null = null;
      let count = 0;
      for (;;) {
        const page = await this.client.createMessagesRequest(id, cursor, 100, Direction.Backward);
        for (const raw of page.chunk) {
          if (
            raw.type !== String(EventType.RoomMessageEncrypted) ||
            raw.sender !== this.bootstrap.matrix_user_id
          )
            continue;
          const event = new MatrixEvent({ ...raw, room_id: id });
          await this.client.decryptEventIfNeeded(event);
          if (event.isDecryptionFailure() || event.getType() !== String(EventType.RoomMessage))
            continue;
          const trust = await this.crypto.getEncryptionInfoForEvent(event);
          if (!trust) throw new MatrixSecurityError('unverified');
          await this.authorizeRoomEvent(event, trust);
          const content = event.getContent();
          const draft = content['org.sytue.rooms.draft'] as
            | { v?: unknown; tenant?: unknown; channel_id?: unknown; parent_id?: unknown }
            | undefined;
          if (
            draft?.v !== 1 ||
            draft.tenant !== this.bootstrap.tenant ||
            typeof draft.channel_id !== 'string' ||
            typeof content.body !== 'string' ||
            content.body.length > 65536 ||
            (draft.parent_id !== undefined && typeof draft.parent_id !== 'string') ||
            !this.bootstrap.channels.some((entry) => entry.channel_id === draft.channel_id)
          )
            continue;
          const key = draft.channel_id + ':' + (draft.parent_id ?? '');
          const timestamp = event.getTs();
          if ((latest.get(key)?.timestamp ?? -1) >= timestamp) continue;
          latest.set(key, {
            channelId: draft.channel_id,
            ...(typeof draft.parent_id === 'string' ? { parentId: draft.parent_id } : {}),
            body: content.body,
            updatedAt: new Date(timestamp).toISOString(),
            timestamp,
          });
        }
        count += page.chunk.length;
        if (!page.end || page.end === cursor || page.chunk.length === 0) break;
        if (count >= 20000) throw new MatrixSecurityError('reference');
        cursor = page.end;
      }
    }
    if (this.closed) throw new MatrixSecurityError('device');
    return [...latest.values()]
      .filter((draft) => draft.body.trim())
      .map(({ timestamp: _timestamp, ...draft }) => draft);
  }
  get deviceId(): string {
    return this.client.getDeviceId() ?? '';
  }
  private mxid(subject: string): string {
    return `@sytue_${subject}:${new URL(this.bootstrap.homeserver_url).host}`;
  }
  private async callDevice(member: MatrixCallMember): Promise<void> {
    const user = this.mxid(member.subject);
    const devices = await this.crypto.getUserDeviceInfo([user], true);
    const device = devices.get(user)?.get(member.device_id);
    const trust = await this.crypto.getDeviceVerificationStatus(user, member.device_id);
    const identity = await this.crypto.getUserVerificationStatus(user);
    if (
      !this.canVerifyUser(user) ||
      !device ||
      device.verified === DeviceVerification.Blocked ||
      !trust?.signedByOwner ||
      !identity.isCrossSigningVerified() ||
      identity.needsUserApproval
    )
      this.requireVerification();
  }
  async prepareCallScope(scope: MatrixCallScope): Promise<void> {
    if (this.bootstrap.storage_product) throw new MatrixSecurityError('unsupported');
    validateCallScope(scope, scope.call_id, this.bootstrap.actor, this.deviceId);
    if (this.closed || !(await this.trusted())) throw new MatrixSecurityError('device');
    await this.guardRecipients();
    for (const member of scope.members) {
      const user = this.mxid(member.subject);
      if (user !== this.bootstrap.matrix_user_id && this.canVerifyUser(user))
        this.verificationPeers.add(user);
    }
    await this.joinEncryptedRoom(scope.matrix_room_id);
    const room = this.client.getRoom(scope.matrix_room_id);
    const allowed = new Set(scope.members.map((member) => this.mxid(member.subject)));
    if (
      !room ||
      (await room.getEncryptionTargetMembers()).some(
        (member) => !allowed.has(member.userId) && !this.isController(member.userId),
      )
    )
      throw new MatrixSecurityError('room');
  }
  /** Call secrets use Olm to-device; Megolm would disclose them to other devices of each user. */
  async sendEncryptedCustom(
    scope: MatrixCallScope,
    type: MatrixCallEventType,
    content: MatrixCallKeyContent,
    recipient?: MatrixCallMember,
  ): Promise<void> {
    await this.prepareCallScope(scope);
    if (
      !validCallContent(type, content) ||
      content.call_id !== scope.call_id ||
      content.generation !== scope.generation ||
      content.expires_at > scope.expires_at_ms
    )
      throw new MatrixSecurityError('reference');
    if (
      type === 'org.sytue.rooms.call_key.v1' &&
      (scope.leader_subject !== this.bootstrap.actor || scope.leader_device_id !== this.deviceId)
    )
      throw new MatrixSecurityError('identity');
    const targets = recipient
      ? [recipient]
      : type === 'org.sytue.rooms.call_key_request.v1'
        ? [{ subject: scope.leader_subject, device_id: scope.leader_device_id }]
        : scope.members;
    const recipients = targets.filter(
      (member) => !(member.subject === this.bootstrap.actor && member.device_id === this.deviceId),
    );
    for (const member of recipients) {
      if (
        !scope.members.some(
          (known) => known.subject === member.subject && known.device_id === member.device_id,
        )
      )
        throw new MatrixSecurityError('identity');
      await this.callDevice(member);
    }
    if (recipients.length === 0) return;
    const batch = await this.crypto.encryptToDeviceMessages(
      type,
      recipients.map((member) => ({
        userId: this.mxid(member.subject),
        deviceId: member.device_id,
      })),
      content,
    );
    if (batch.batch.length !== recipients.length) throw new MatrixSecurityError('device');
    await this.client.queueToDevice(batch);
  }
  onEncryptedCustom(
    scope: MatrixCallScope,
    type: MatrixCallEventType,
    handler: (
      content: MatrixCallKeyContent,
      metadata: { sender: string; senderDevice: string },
    ) => void,
  ): () => void {
    let active = true;
    const receive = (payload: ReceivedToDeviceMessage): void => {
      const proof = payload.encryptionInfo;
      if (!active || !proof?.senderDevice || payload.message.type !== type) return;
      const sender = scope.members.find(
        (member) =>
          this.mxid(member.subject) === proof.sender && member.device_id === proof.senderDevice,
      );
      if (!sender || payload.message.sender !== proof.sender) return;
      if (
        type === 'org.sytue.rooms.call_key.v1' &&
        (sender.subject !== scope.leader_subject || sender.device_id !== scope.leader_device_id)
      )
        return;
      const content = payload.message.content as MatrixCallKeyContent;
      if (
        !validCallContent(type, content) ||
        content.call_id !== scope.call_id ||
        content.generation !== scope.generation ||
        content.expires_at > scope.expires_at_ms
      )
        return;
      void this.callDevice(sender)
        .then(async () => {
          const device = (await this.crypto.getUserDeviceInfo([proof.sender], false))
            .get(proof.sender)
            ?.get(sender.device_id);
          if (!active || !device || device.getIdentityKey() !== proof.senderCurve25519KeyBase64)
            return;
          validateCallScope(scope, scope.call_id, this.bootstrap.actor, this.deviceId);
          handler(content, { sender: proof.sender, senderDevice: sender.device_id });
        })
        .catch(() => undefined);
    };
    this.client.on(ClientEvent.ReceivedToDeviceMessage, receive);
    return () => {
      active = false;
      this.client.off(ClientEvent.ReceivedToDeviceMessage, receive);
    };
  }
  async upload(
    channel: string,
    file: File,
    signal?: AbortSignal,
  ): Promise<{ room: string; file: LocalMatrixAttachment }> {
    const room = await this.room(channel);
    if (file.size > 50 * 1024 * 1024) throw new Error('too_large');
    if (signal?.aborted) throw new DOMException('Aborted', 'AbortError');
    await initAsync();
    const plain = new Uint8Array(await file.arrayBuffer());
    if (this.closed || signal?.aborted) {
      plain.fill(0);
      throw new MatrixSecurityError('device');
    }
    const encrypted = MatrixAttachment.encrypt(plain);
    plain.fill(0);
    try {
      const info = encrypted.mediaEncryptionInfo;
      if (!info) throw new MatrixSecurityError('reference');
      const bytes = encrypted.encryptedData;
      const upload = await this.client.uploadContent(
        new Blob([bytes as Uint8Array<ArrayBuffer>], { type: 'application/octet-stream' }),
        {
          type: 'application/octet-stream',
          includeFilename: false,
          ...(signal ? { abortController: this.abortController(signal) } : {}),
        },
      );
      if (this.closed || signal?.aborted) throw new MatrixSecurityError('device');
      const matrixFile = {
        ...(JSON.parse(info) as Omit<MatrixFile, 'url'>),
        url: upload.content_uri,
      };
      const objectUrl = URL.createObjectURL(
        new Blob([file], { type: safeAttachmentType(file.type) }),
      );
      this.urls.add(objectUrl);
      return {
        room,
        file: {
          fileId: upload.content_uri,
          name: file.name,
          contentType: file.type || 'application/octet-stream',
          sizeBytes: file.size,
          matrixFile,
          objectUrl,
        },
      };
    } finally {
      encrypted.free();
    }
  }
  private abortController(signal: AbortSignal): AbortController {
    const controller = new AbortController();
    if (signal.aborted) controller.abort();
    else signal.addEventListener('abort', () => controller.abort(), { once: true });
    return controller;
  }
  async download(file: LocalMatrixAttachment): Promise<string> {
    if (this.closed) throw new MatrixSecurityError('device');
    if (!file.matrixFile) throw new MatrixSecurityError('reference');
    const url = this.client.mxcUrlToHttp(
      file.matrixFile.url,
      undefined,
      undefined,
      undefined,
      false,
      true,
      true,
    );
    if (!url || new URL(url).origin !== this.bootstrap.homeserver_url)
      throw new MatrixSecurityError('reference');
    const token = this.client.getAccessToken();
    if (!token) throw new MatrixSecurityError('login');
    const response = await fetch(url, {
      headers: { authorization: `Bearer ${token}` },
      credentials: 'omit',
      redirect: 'error',
      cache: 'no-store',
    });
    if (!response.ok) throw new MatrixSecurityError('reference');
    if (this.closed) throw new MatrixSecurityError('device');
    await initAsync();
    const bytes = await boundedMedia(response, 50 * 1024 * 1024);
    if (this.closed) throw new MatrixSecurityError('device');
    const attachment = new EncryptedAttachment(bytes, JSON.stringify(file.matrixFile));
    try {
      const plain = MatrixAttachment.decrypt(attachment);
      const objectUrl = URL.createObjectURL(
        new Blob([plain as Uint8Array<ArrayBuffer>], {
          type: safeAttachmentType(file.contentType),
        }),
      );
      plain.fill(0);
      this.urls.add(objectUrl);
      return objectUrl;
    } finally {
      attachment.free();
    }
  }
  releaseAttachment(url: string): void {
    this.urls.delete(url);
    URL.revokeObjectURL(url);
  }
  get isClosed(): boolean {
    return this.closed;
  }
  close(): void {
    if (this.closed) return;
    this.closed = true;
    if (this.network) {
      this.network.closed = true;
      this.network.lifetime?.abort();
    }
    try {
      this.ui?.cancelPending?.();
    } catch {
      /* Remaining resources still retire. */
    }
    navigator.serviceWorker?.removeEventListener('message', this.onOfflineClear);
    if (this.urls) {
      for (const url of this.urls) URL.revokeObjectURL(url);
      this.urls.clear();
    }
    const modules = this.clientModules ? [...this.clientModules.values()] : [];
    this.clientModules?.clear();
    this.localJobsClosing = Promise.allSettled([
      this.storageJobs?.close(),
      ...modules.map((runtime) => runtime.closeAndDrain()),
      this.appSessionOwner?.closeAndDrain(),
      this.openingAppOwner?.then(
        (owner) => owner.closeAndDrain(),
        () => undefined,
      ),
    ]).then((results) => {
      const failures = results.filter((result) => result.status === 'rejected');
      if (failures.length)
        throw new AggregateError(
          failures.map((result): unknown => result.reason),
          'matrix_device',
        );
    });
    void this.localJobsClosing.catch(() => {});
    // The pair installs its common barrier before reentering the other owner.
    const coupled = this.lifetimePair?.retire();
    this.jobsClosing =
      coupled ??
      this.localJobsClosing.then(
        () => this.eraseAfterDrain(),
        async (error) => {
          await this.eraseAfterDrain();
          throw error;
        },
      );
    void this.jobsClosing.catch(() => {});
    if (!this.deferRelease)
      void this.jobsClosing.then(
        () => this.release(),
        () => this.release(),
      );
  }
  async closeAndDrain(): Promise<void> {
    this.close();
    await this.jobsClosing;
  }
  private eraseAfterDrain(): Promise<void> {
    if (this.erasing) return this.erasing;
    this.erasing = Promise.resolve().then(async () => {
      const cleared = await Promise.allSettled(
        [
          () => this.client.stopClient(),
          () => this.ownerKeys?.close(),
          () => this.uploadCache?.close(),
          () => this.privateStoragePq?.close(),
          () => this.storagePqCache?.close(),
          () => this.roomSessionProofs?.close(),
          () => this.roomBackfillCache?.close(),
          () => {
            for (const key of this.temporaryKeys.values()) key.fill(0);
            this.temporaryKeys.clear();
          },
          () => {
            this.attestedStorageSessions?.clear();
            this.liveStorageSessions?.clear();
            if (this.pendingStorageEventRefs) this.pendingStorageEventRefs.length = 0;
          },
        ].map((cleanup) => Promise.resolve().then(cleanup)),
      );
      const failures = cleared.filter((result) => result.status === 'rejected');
      if (failures.length)
        throw new AggregateError(
          failures.map((result): unknown => result.reason),
          'matrix_device',
        );
    });
    return this.erasing;
  }
  async forget(): Promise<void> {
    if (this.forgetting) return this.forgetting;
    // An old closed instance must never erase a device another instance now owns.
    if (this.closed) throw new MatrixSecurityError('device');
    // Publish one destruction attempt before the authenticated pending guard
    // yields. Concurrent callers share both the decision and credential cleanup.
    const pending = Promise.resolve().then(async () => {
      if (this.destructionGuard) await this.destructionGuard();
      if (this.closed) throw new MatrixSecurityError('device');
      this.deferRelease = true;
      this.close();
      try {
        await this.jobsClosing;
        // Close this device on the homeserver with its own token BEFORE its session and
        // keys are erased: afterwards nobody can, and a forgotten device that stays alive
        // keeps receiving every room key. Bounded and never fatal; erasing continues.
        if (this.session)
          await logoutMatrixDevice(this.bootstrap.homeserver_url, this.session.access_token);
        const results = await Promise.allSettled([
          Promise.resolve().then(() => this.ownerKeys?.destroy()),
          Promise.resolve().then(() => this.uploadCache?.destroy()),
          Promise.resolve().then(() => this.storagePqCache?.destroy()),
          Promise.resolve().then(() => this.roomBackfillCache?.destroy()),
          Promise.resolve().then(() => this.adapter.forgetSession(this.bootstrap, this.session)),
          Promise.resolve()
            .then(() => this.client.clearStores({ cryptoDatabasePrefix: this.prefix }))
            .then(() => this.cryptoStoreKeys?.destroy()),
        ]);
        const failures = results.filter((result) => result.status === 'rejected');
        if (failures.length)
          throw new AggregateError(
            failures.map((result) => result.reason as unknown),
            'matrix_device',
          );
      } finally {
        this.deferRelease = false;
        this.release();
      }
    });
    this.forgetting = pending;
    void pending.catch(() => {
      // A denied guard erased nothing. It may be retried after pending work is
      // committed; a retired SDK keeps its original terminal result.
      if (!this.closed && this.forgetting === pending) this.forgetting = undefined;
    });
    return pending;
  }
}

async function boundedMedia(response: Response, maximum: number): Promise<Uint8Array<ArrayBuffer>> {
  const length = Number(response.headers.get('content-length'));
  if (length > maximum || !response.body) throw new MatrixSecurityError('reference');
  const reader = response.body.getReader();
  const chunks: Uint8Array[] = [];
  let total = 0;
  for (;;) {
    const chunk = await reader.read();
    if (chunk.done) break;
    total += chunk.value.byteLength;
    if (total > maximum) {
      await reader.cancel();
      throw new MatrixSecurityError('reference');
    }
    chunks.push(chunk.value);
  }
  const result = new Uint8Array(total);
  let offset = 0;
  for (const chunk of chunks) {
    result.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return result;
}

/** The SDK's trusted-only policy must never find a previously approved controller device. */
async function assertControllerUntrusted(crypto: CryptoApi, user: string): Promise<void> {
  if ((await crypto.getUserVerificationStatus(user)).isCrossSigningVerified())
    throw new MatrixSecurityError('identity');
  const devices = (await crypto.getUserDeviceInfo([user], true)).get(user);
  for (const id of devices?.keys() ?? []) {
    if ((await crypto.getDeviceVerificationStatus(user, id))?.isVerified())
      throw new MatrixSecurityError('identity');
  }
}
