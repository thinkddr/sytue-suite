/* SPDX-License-Identifier: Apache-2.0 */
import type { ExternalE2EEKeyProvider } from 'livekit-client';
import { roomsMatrix, type MatrixCallKeyContent } from './matrix/client.js';
import { MatrixSecurityError, validateCallScope, type MatrixCallScope } from './matrix/types.js';
import {
  callEncryption,
  readyCallEncryption,
  reconnectCallEncryption,
  LlamadaError,
  type LiveKitConnection,
} from './meetApi.js';

interface CallEncryptionUI {
  pause: () => Promise<void>;
  connect: (connection: LiveKitConnection) => Promise<void>;
  ready: () => Promise<void>;
  failed: () => void;
}

// An epoch is immutable: the server rotates it when any member or lease changes.
function sameCallEpoch(left: MatrixCallScope, right: MatrixCallScope): boolean {
  const members = new Map(right.members.map((member) => [member.subject, member.device_id]));
  return (
    left.call_id === right.call_id &&
    left.generation === right.generation &&
    left.matrix_room_id === right.matrix_room_id &&
    left.livekit_room === right.livekit_room &&
    left.expires_at_ms === right.expires_at_ms &&
    left.leader_subject === right.leader_subject &&
    left.leader_device_id === right.leader_device_id &&
    left.members.length === right.members.length &&
    left.members.every((member) => members.get(member.subject) === member.device_id)
  );
}
/** Keys are ephemeral. The server supplies ACL/leases and receives no key material. */
export class CallEncryption {
  private readonly matrix = roomsMatrix();
  private scope: MatrixCallScope | null = null;
  private key: MatrixCallKeyContent | null = null;
  private installing: {
    scope: MatrixCallScope;
    content: MatrixCallKeyContent;
    done: Promise<void>;
  } | null = null;
  private installQueue: Promise<void> = Promise.resolve();
  private unsubscribe: (() => void)[] = [];
  private timer: ReturnType<typeof setTimeout> | undefined;
  private waitForKey: { resolve: () => void; reject: (error: Error) => void } | null = null;
  private updating: Promise<void> | null = null;
  private connected = false;
  private stopped = false;
  private acknowledged = false;
  private authorizationVersion = 0;
  private expiryTimer: ReturnType<typeof setTimeout> | undefined;
  private reconnectRequested = false;
  private constructor(
    private readonly roomId: string,
    private readonly joinId: string,
    private readonly provider: ExternalE2EEKeyProvider,
    private readonly ui: CallEncryptionUI,
  ) {}
  static async prepare(
    roomId: string,
    connection: LiveKitConnection,
    provider: ExternalE2EEKeyProvider,
    ui: CallEncryptionUI,
  ): Promise<CallEncryption> {
    const matrix = roomsMatrix();
    if (
      !connection.encryption ||
      !connection.joinId ||
      connection.identity !== matrix.bootstrap.actor ||
      connection.joinId.length > 255
    )
      throw new MatrixSecurityError('identity');
    const instance = new CallEncryption(roomId, connection.joinId, provider, ui);
    try {
      await instance.bind(connection.encryption);
      return instance;
    } catch (error) {
      instance.close();
      throw error;
    }
  }
  get ready(): boolean {
    return !this.stopped && this.acknowledged && (this.scope?.expires_at_ms ?? 0) > Date.now();
  }
  /** A pending media operation must still belong to the same authorization. */
  publicationGuard(): () => boolean {
    const version = this.authorizationVersion;
    const scope = this.scope;
    return () => this.ready && this.scope === scope && this.authorizationVersion === version;
  }
  private invalidateAuthorization(): void {
    this.acknowledged = false;
    this.authorizationVersion++;
  }
  /** Invalidate immediately, before the SDK can resume a disconnected transport. */
  suspendForReconnect(): void {
    if (this.stopped) return;
    this.reconnectRequested = true;
    this.invalidateAuthorization();
  }
  private content(): MatrixCallKeyContent {
    if (!this.scope) throw new MatrixSecurityError('reference');
    return {
      v: 1,
      call_id: this.scope.call_id,
      generation: this.scope.generation,
      algorithm: 'AES-GCM',
      expires_at: this.scope.expires_at_ms,
    };
  }
  private async install(content: MatrixCallKeyContent): Promise<void> {
    const scope = this.scope;
    const key = content.key_b64;
    if (
      this.stopped ||
      !scope ||
      !key ||
      content.generation !== scope.generation ||
      content.expires_at !== scope.expires_at_ms
    )
      return;
    if (this.key && this.key.key_b64 !== key) throw new MatrixSecurityError('identity');
    if (this.key) return;
    if (this.installing?.scope === scope) {
      if (this.installing.content.key_b64 !== key) throw new MatrixSecurityError('identity');
      await this.installing.done;
      return;
    }
    // Reserve a private copy before yielding; serialize SDK writes across epochs.
    const candidate = { ...content, key_b64: key };
    const done = this.installQueue.then(async () => {
      if (this.stopped || this.scope !== scope) throw new MatrixSecurityError('device');
      await this.provider.setKey(key);
      if (this.stopped || this.scope !== scope) throw new MatrixSecurityError('device');
      this.key = candidate;
      this.waitForKey?.resolve();
    });
    this.installing = { scope, content: candidate, done };
    this.installQueue = done.catch(() => undefined);
    try {
      await done;
    } finally {
      if (this.installing?.done === done) this.installing = null;
    }
  }
  private async bind(input: MatrixCallScope): Promise<void> {
    const checked = validateCallScope(
      input,
      this.roomId,
      this.matrix.bootstrap.actor,
      this.matrix.deviceId,
    );
    // Keep the validated roster independent of callers retaining the response object.
    const scope = { ...checked, members: checked.members.map((member) => ({ ...member })) };
    this.invalidateAuthorization();
    await this.ui.pause();
    if (this.stopped) throw new MatrixSecurityError('device');
    for (const off of this.unsubscribe) off();
    this.unsubscribe = [];
    this.key = null;
    this.scope = scope;
    clearTimeout(this.expiryTimer);
    this.expiryTimer = setTimeout(() => this.fail(), Math.max(0, scope.expires_at_ms - Date.now()));
    await this.matrix.prepareCallScope(scope);
    this.unsubscribe.push(
      this.matrix.onEncryptedCustom(scope, 'org.sytue.rooms.call_key.v1', (content) => {
        void this.install(content).catch(() => {
          if (this.scope === scope) this.fail();
        });
      }),
    );
    const leader =
      scope.leader_subject === this.matrix.bootstrap.actor &&
      scope.leader_device_id === this.matrix.deviceId;
    if (leader) {
      this.unsubscribe.push(
        this.matrix.onEncryptedCustom(
          scope,
          'org.sytue.rooms.call_key_request.v1',
          (_content, sender) => {
            const member = scope.members.find(
              (item) =>
                item.device_id === sender.senderDevice &&
                `@sytue_${item.subject}:${new URL(this.matrix.bootstrap.homeserver_url).host}` ===
                  sender.sender,
            );
            if (this.key && member && !this.stopped)
              void this.matrix
                .sendEncryptedCustom(scope, 'org.sytue.rooms.call_key.v1', this.key, member)
                .catch(() => this.fail());
          },
        ),
      );
      const bytes = crypto.getRandomValues(new Uint8Array(32));
      const key = btoa(String.fromCharCode(...bytes));
      bytes.fill(0);
      const content = { ...this.content(), key_b64: key };
      await this.install(content);
      await this.matrix.sendEncryptedCustom(scope, 'org.sytue.rooms.call_key.v1', content);
      return;
    }
    await new Promise<void>((resolve, reject) => {
      let attempts = 0;
      let settled = false;
      let retry: ReturnType<typeof setTimeout> | undefined;
      const timeout = setTimeout(() => finish(new MatrixSecurityError('unverified')), 15000);
      const finish = (error?: Error): void => {
        if (settled) return;
        settled = true;
        clearTimeout(timeout);
        clearTimeout(retry);
        if (this.waitForKey === pending) this.waitForKey = null;
        if (error) reject(error);
        else resolve();
      };
      const pending = { resolve: () => finish(), reject: (error: Error) => finish(error) };
      this.waitForKey = pending;
      const request = (): void => {
        if (settled) return;
        if (this.stopped || this.scope?.generation !== scope.generation) {
          finish(new MatrixSecurityError('device'));
          return;
        }
        if (this.key) {
          finish();
          return;
        }
        if (attempts++ >= 12) return;
        void this.matrix
          .sendEncryptedCustom(scope, 'org.sytue.rooms.call_key_request.v1', this.content())
          .catch((error: unknown) =>
            finish(error instanceof Error ? error : new MatrixSecurityError('unverified')),
          );
        retry = setTimeout(request, 1000);
      };
      request();
    });
  }
  async started(): Promise<void> {
    this.connected = true;
    await this.refresh();
    if (!this.scope || !this.key) throw new MatrixSecurityError('unverified');
    this.schedule();
  }
  private async acknowledge(): Promise<void> {
    if (!this.scope || !this.key || this.stopped) throw new MatrixSecurityError('unverified');
    const scope = this.scope;
    const version = this.authorizationVersion;
    await readyCallEncryption(this.roomId, this.joinId, scope.generation);
    if (this.stopped || this.scope !== scope || this.authorizationVersion !== version) return;
    this.acknowledged = true;
    await this.ui.ready();
  }
  refresh(forceReconnect = false): Promise<void> {
    if (forceReconnect) {
      this.suspendForReconnect();
    }
    if (this.updating) return this.updating;
    const run = async (): Promise<void> => {
      if (this.stopped) return;
      try {
        const scope = validateCallScope(
          await callEncryption(this.roomId, this.joinId),
          this.roomId,
          this.matrix.bootstrap.actor,
          this.matrix.deviceId,
        );
        const previous = this.scope;
        if (!previous) throw new MatrixSecurityError('reference');
        const changed = scope.generation !== previous.generation;
        if (
          (!changed && !sameCallEpoch(previous, scope)) ||
          (changed &&
            (scope.generation <= previous.generation ||
              scope.matrix_room_id === previous.matrix_room_id ||
              scope.livekit_room === previous.livekit_room))
        )
          throw new MatrixSecurityError('identity');
        if (changed) {
          // Preserve the obligation to reconnect even if a transient 5xx occurs
          // after binding the new epoch but before establishing its transport.
          this.reconnectRequested = true;
          await this.bind(scope);
        }
        const reconnect = this.reconnectRequested;
        this.reconnectRequested = false;
        if (reconnect && this.connected) {
          try {
            const connection = await reconnectCallEncryption(
              this.roomId,
              this.joinId,
              scope.generation,
            );
            if (
              connection.identity !== this.matrix.bootstrap.actor ||
              connection.joinId !== this.joinId ||
              !connection.encryption ||
              !sameCallEpoch(
                scope,
                validateCallScope(
                  connection.encryption,
                  this.roomId,
                  this.matrix.bootstrap.actor,
                  this.matrix.deviceId,
                ),
              )
            )
              throw new MatrixSecurityError('identity');
            await this.ui.connect(connection);
          } catch (error) {
            this.reconnectRequested = true;
            throw error;
          }
        }
        if (this.reconnectRequested) return;
        if (this.connected && (!this.acknowledged || reconnect)) await this.acknowledge();
      } catch (error) {
        if (this.stopped) return;
        const retryable =
          error instanceof LlamadaError &&
          error.status >= 500 &&
          (this.scope?.expires_at_ms ?? 0) > Date.now() + 5000;
        if (!retryable) {
          this.fail();
          throw error;
        }
      } finally {
        if (!this.stopped && this.connected) this.schedule();
      }
    };
    const job = run().finally(() => {
      if (this.updating === job) this.updating = null;
    });
    this.updating = job;
    return job;
  }
  private schedule(): void {
    clearTimeout(this.timer);
    this.timer = setTimeout(() => {
      void this.refresh().catch(() => undefined);
    }, 5000);
  }
  private fail(): void {
    if (this.stopped) return;
    this.close();
    this.ui.failed();
  }
  close(): void {
    if (this.stopped) return;
    this.stopped = true;
    this.invalidateAuthorization();
    clearTimeout(this.timer);
    clearTimeout(this.expiryTimer);
    const listeners = this.unsubscribe;
    this.unsubscribe = [];
    this.waitForKey?.reject(new MatrixSecurityError('device'));
    this.waitForKey = null;
    this.key = null;
    this.scope = null;
    for (const off of listeners) {
      try {
        off();
      } catch {
        // The call is already closed; still remove the other listeners.
      }
    }
  }
}
