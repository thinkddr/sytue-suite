/* SPDX-License-Identifier: Apache-2.0 */
import {
  createClient,
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
  EventShieldColour,
  EventShieldReason,
  type CryptoApi,
  type VerificationRequest,
} from 'matrix-js-sdk/lib/crypto-api';
import { decodeRecoveryKey } from 'matrix-js-sdk/lib/crypto-api/recovery-key';
import {
  Attachment as MatrixAttachment,
  EncryptedAttachment,
  initAsync,
} from '@matrix-org/matrix-sdk-crypto-wasm';
import type { RoomMessageEventContent } from 'matrix-js-sdk/lib/@types/events';
import type { IContent as MatrixCustomContent } from 'matrix-js-sdk';
import type { ReceivedToDeviceMessage } from 'matrix-js-sdk/lib/sync-accumulator';
import { DeviceVerification } from 'matrix-js-sdk/lib/models/device';
import { privateDeviceFetch } from './private-policy.js';
import { StorageRepository } from './storage-repository.js';
import {
  decodeStorageRecord,
  storageReference,
  StorageRecordError,
  type StorageRecord,
  type StorageRecordEntry,
} from './storage.js';
import type { MatrixStorageScope } from './types.js';
import {
  MatrixSecurityError,
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

/** All displayed secrets stay in this client. Implementations must ask for explicit confirmation. */
export interface MatrixRecoveryUI {
  saveRecoveryKey: (encodedKey: string) => Promise<void>;
  readRecoveryKey: () => Promise<string>;
  authenticate: (url: string) => Promise<void>;
}
export interface MatrixDevice {
  id: string;
  name: string;
  verified: boolean;
  current: boolean;
}

async function acquireStore(name: string): Promise<() => void> {
  if (!navigator.locks) throw new MatrixSecurityError('locked');
  return new Promise((resolve, reject) => {
    void navigator.locks
      .request(name, { ifAvailable: true }, async (lock) => {
        if (!lock) throw new MatrixSecurityError('locked');
        await new Promise<void>((release) => resolve(release));
      })
      .catch(reject);
  });
}

/** Product routing is supplied by the host app; crypto never chooses a BFF or return URL. */
export interface MatrixSessionAdapter {
  readSession: (bootstrap: MatrixBootstrap) => Promise<MatrixSession | null>;
  forgetSession: (bootstrap: MatrixBootstrap) => Promise<void>;
  refreshBootstrap: () => Promise<MatrixBootstrap>;
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
  private readonly policyRooms = new Set<string>();
  private readonly verificationPeers = new Set<string>();
  private constructor(
    readonly bootstrap: MatrixBootstrap,
    client: MatrixClient,
    release: () => void,
    prefix: string,
    private readonly ui: MatrixRecoveryUI,
    keys: Map<string, Uint8Array<ArrayBuffer>>,
    private readonly adapter: MatrixSessionAdapter,
    private readonly network: { closed: boolean },
  ) {
    this.client = client;
    this.temporaryKeys = keys;
    const crypto = client.getCrypto();
    if (!crypto) throw new MatrixSecurityError('device');
    this.crypto = crypto;
    this.release = release;
    this.prefix = prefix;
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
    const session = await adapter.readSession(bootstrap);
    if (!session) throw new MatrixSecurityError('login');
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
    // Preserve the deployed Rooms database name. A private product gets a new
    // actor/tenant/product namespace and cannot adopt an unbound Rooms device.
    const account = `${encodeURIComponent(bootstrap.homeserver_url)}-${encodeURIComponent(session.user_id)}-${encodeURIComponent(session.device_id)}`;
    const prefix = product
      ? `sytue-storage-matrix-${encodeURIComponent(JSON.stringify([product, bootstrap.tenant, bootstrap.actor, bootstrap.homeserver_url, session.user_id, session.device_id]))}`
      : `sytue-rooms-matrix-${account}`;
    const release = await acquireStore(prefix);
    const keys = new Map<string, Uint8Array<ArrayBuffer>>();
    const network = { closed: false };
    let deviceCrypto: CryptoApi | undefined;
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
            ),
          }
        : {}),
      cryptoCallbacks: {
        getSecretStorageKey: async ({ keys: available }) => {
          for (const id of Object.keys(available)) {
            const cached = keys.get(id);
            if (cached) return [id, cached];
          }
          const encoded = await ui.readRecoveryKey();
          const key = decodeRecoveryKey(encoded.trim());
          for (const [id, description] of Object.entries(available)) {
            if (await client.secretStorage.checkKey(key, description)) {
              keys.set(id, key);
              return [id, key];
            }
          }
          key.fill(0);
          throw new MatrixSecurityError('recovery');
        },
        cacheSecretStorageKey: (id, _description, key) => {
          keys.set(id, key);
        },
      },
    });
    try {
      const who = await client.whoami();
      if (who.user_id !== bootstrap.matrix_user_id || who.device_id !== session.device_id)
        throw new MatrixSecurityError('identity');
      await client.initRustCrypto({ useIndexedDB: true, cryptoDatabasePrefix: prefix });
      const crypto = client.getCrypto();
      if (!crypto) throw new MatrixSecurityError('device');
      deviceCrypto = crypto;
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
      return new SuiteMatrix(bootstrap, client, release, prefix, ui, keys, adapter, network);
    } catch (error) {
      network.closed = true;
      client.stopClient();
      for (const key of keys.values()) key.fill(0);
      release();
      throw error;
    }
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
    await this.crypto.bootstrapSecretStorage({
      createSecretStorageKey: async () => {
        const generated = await this.crypto.createRecoveryKeyFromPassphrase();
        if (!generated.encodedPrivateKey) throw new MatrixSecurityError('recovery');
        await this.ui.saveRecoveryKey(generated.encodedPrivateKey);
        return generated;
      },
      setupNewKeyBackup: false,
    });
    await this.crypto.bootstrapCrossSigning({
      authUploadDeviceSigningKeys: (request) => this.interactiveAuth(request),
    });
    if ((await this.crypto.checkKeyBackupAndEnable()) === null) await this.crypto.resetKeyBackup();
    if (!(await this.crypto.isSecretStorageReady()) || !(await this.trusted()))
      throw new MatrixSecurityError('device');
  }
  async recover(): Promise<void> {
    if (!(await this.crypto.isSecretStorageReady())) throw new MatrixSecurityError('recovery');
    await this.crypto.bootstrapCrossSigning({
      authUploadDeviceSigningKeys: (request) => this.interactiveAuth(request),
    });
    await this.crypto.loadSessionBackupPrivateKeyFromSecretStorage();
    await this.crypto.restoreKeyBackup();
    if (!(await this.trusted())) throw new MatrixSecurityError('device');
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
  async read(
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
    return {
      ...own,
      ...(attachments ? { attachments } : {}),
      body: content.body,
      authenticityVerified: trust.shieldReason !== EventShieldReason.AUTHENTICITY_NOT_GUARANTEED,
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
  /** Client-only private data adapter. Other products cannot reuse a Rooms device or channel. */
  storage(): StorageRepository {
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
    const prepare = async (room: string): Promise<void> => {
      assertOpen();
      if (!rooms.includes(room)) throw new MatrixSecurityError('reference');
      await this.privateDraftRoom(room);
      assertOpen();
    };
    const decode = async (
      raw: Partial<IEvent>,
      reference: MatrixReference,
    ): Promise<StorageRecord | null> => {
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
      if (
        !trust ||
        trust.shieldColour !== EventShieldColour.NONE ||
        (event.getKeyForwardingUser() &&
          event.getKeyForwardingUser() !== this.bootstrap.matrix_user_id)
      )
        throw new MatrixSecurityError('unverified');
      if (event.getType() !== String(EventType.RoomMessage)) return null;
      return decodeStorageRecord(event.getContent()['org.sytue.storage'], scope);
    };
    return new StorageRepository(scope, {
      rooms,
      assertOpen,
      put: async (transaction, input) => {
        const record = decodeStorageRecord(input, scope);
        await prepare(writeRoom);
        const result = await this.client.sendEvent(
          writeRoom,
          EventType.RoomMessage,
          { msgtype: 'm.text', body: '', 'org.sytue.storage': record } as RoomMessageEventContent,
          `sytue-storage-${transaction}`,
        );
        assertOpen();
        return storageReference({ room_id: writeRoom, event_id: result.event_id });
      },
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
    });
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
    if (this.network) this.network.closed = true;
    this.client.stopClient();
    navigator.serviceWorker?.removeEventListener('message', this.onOfflineClear);
    for (const key of this.temporaryKeys.values()) key.fill(0);
    this.temporaryKeys.clear();
    for (const url of this.urls) URL.revokeObjectURL(url);
    this.urls.clear();
    this.release();
  }
  async forget(): Promise<void> {
    this.close();
    await this.adapter.forgetSession(this.bootstrap);
    await this.client.clearStores({ cryptoDatabasePrefix: this.prefix });
  }
}

/** Active documents must never execute on the Rooms origin after decryption. */
export function safeAttachmentType(type: string): string {
  return /^(image\/(png|jpeg|webp|gif|avif|bmp)|application\/pdf|text\/plain|audio\/[a-z0-9.+-]+|video\/[a-z0-9.+-]+)$/.test(
    type,
  )
    ? type
    : 'application/octet-stream';
}
export type MatrixCallEventType =
  'org.sytue.rooms.call_key.v1' | 'org.sytue.rooms.call_key_request.v1';
export interface MatrixCallKeyContent extends MatrixCustomContent {
  v: 1;
  call_id: string;
  generation: number;
  algorithm: 'AES-GCM';
  expires_at: number;
  key_b64?: string;
}

export function validCallContent(
  type: MatrixCallEventType,
  content: MatrixCallKeyContent,
): boolean {
  return (
    content.v === 1 &&
    typeof content.call_id === 'string' &&
    content.call_id.length > 0 &&
    content.call_id.length <= 256 &&
    Number.isSafeInteger(content.generation) &&
    content.generation >= 0 &&
    content.algorithm === 'AES-GCM' &&
    Number.isSafeInteger(content.expires_at) &&
    content.expires_at > Date.now() &&
    content.expires_at <= Date.now() + 24 * 60 * 60 * 1000 &&
    (type === 'org.sytue.rooms.call_key_request.v1'
      ? content.key_b64 === undefined
      : typeof content.key_b64 === 'string' && /^[A-Za-z0-9+/]{43}=$/.test(content.key_b64))
  );
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
