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
/** Keys are ephemeral. The server supplies ACL/leases and receives no key material. */
export class CallEncryption {
  private readonly matrix = roomsMatrix();
  private scope: MatrixCallScope | null = null;
  private key: MatrixCallKeyContent | null = null;
  private unsubscribe: (() => void)[] = [];
  private timer: ReturnType<typeof setTimeout> | undefined;
  private waitForKey: { resolve: () => void; reject: (error: Error) => void } | null = null;
  private updating: Promise<void> | null = null;
  private connected = false;
  private stopped = false;
  private acknowledged = false;
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
    if (
      this.stopped ||
      !this.scope ||
      !content.key_b64 ||
      content.generation !== this.scope.generation ||
      content.expires_at !== this.scope.expires_at_ms
    )
      return;
    if (this.key && this.key.key_b64 !== content.key_b64) throw new MatrixSecurityError('identity');
    await this.provider.setKey(content.key_b64);
    if (this.stopped || content.generation !== this.scope?.generation) return;
    this.key = content;
    this.waitForKey?.resolve();
  }
  private async bind(input: MatrixCallScope): Promise<void> {
    const scope = validateCallScope(
      input,
      this.roomId,
      this.matrix.bootstrap.actor,
      this.matrix.deviceId,
    );
    this.acknowledged = false;
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
        void this.install(content).catch(() => this.fail());
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
    if (!this.acknowledged) await this.acknowledge();
    this.schedule();
  }
  private async acknowledge(): Promise<void> {
    if (!this.scope || !this.key || this.stopped) throw new MatrixSecurityError('unverified');
    await readyCallEncryption(this.roomId, this.joinId, this.scope.generation);
    if (this.stopped) return;
    this.acknowledged = true;
    await this.ui.ready();
  }
  refresh(forceReconnect = false): Promise<void> {
    if (forceReconnect) this.reconnectRequested = true;
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
        const changed =
          scope.generation !== this.scope?.generation ||
          scope.matrix_room_id !== this.scope?.matrix_room_id ||
          scope.livekit_room !== this.scope?.livekit_room;
        if (changed) await this.bind(scope);
        const reconnect = this.reconnectRequested;
        this.reconnectRequested = false;
        if ((changed || reconnect) && this.connected) {
          const connection = await reconnectCallEncryption(
            this.roomId,
            this.joinId,
            scope.generation,
          );
          if (
            connection.identity !== this.matrix.bootstrap.actor ||
            connection.joinId !== this.joinId ||
            connection.encryption?.generation !== scope.generation ||
            connection.encryption.matrix_room_id !== scope.matrix_room_id ||
            connection.encryption.livekit_room !== scope.livekit_room
          )
            throw new MatrixSecurityError('identity');
          await this.ui.connect(connection);
        }
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
      if (this.reconnectRequested && !this.stopped) void this.refresh().catch(() => undefined);
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
    this.acknowledged = false;
    clearTimeout(this.timer);
    clearTimeout(this.expiryTimer);
    for (const off of this.unsubscribe) off();
    this.unsubscribe = [];
    this.waitForKey?.reject(new MatrixSecurityError('device'));
    this.waitForKey = null;
    this.key = null;
    this.scope = null;
  }
}
