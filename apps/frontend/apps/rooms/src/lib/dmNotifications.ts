/* SPDX-License-Identifier: Apache-2.0 */
import type { NotificationPref } from '@sytue/api-clients';

export const DM_NOTIFICATION_RULE = { source: 'rooms', kind: 'rooms.message' } as const;
export const LEGACY_DM_NOTIFY_KEY = 'sytue-rooms-dm-notify';

export interface NotificationOwner {
  id: string;
  tenant: string;
}

/** Same specificity as Hub domain.Muted: an exact exception overrides its source. */
export function dmNotificationsEnabled(prefs: readonly NotificationPref[]): boolean {
  const exact = prefs.find(
    (pref) =>
      pref.source === DM_NOTIFICATION_RULE.source && pref.kind === DM_NOTIFICATION_RULE.kind,
  );
  return !(exact ?? prefs.find((pref) => pref.source === 'rooms' && !pref.kind))?.muted;
}

export function notificationOwnerMatches(a: NotificationOwner, b: NotificationOwner): boolean {
  return a.id !== '' && a.tenant !== '' && a.id === b.id && a.tenant === b.tenant;
}

export interface DmNotificationState {
  enabled: boolean;
  loading: boolean;
  saving: boolean;
  error: boolean;
}

export interface DmNotificationAPI {
  read: (signal: AbortSignal) => Promise<boolean>;
  write: (enabled: boolean, signal: AbortSignal) => Promise<void>;
}

const initial = (): DmNotificationState => ({
  enabled: false,
  loading: true,
  saving: false,
  error: false,
});

/** One screen/identity owns this controller. A lost write is reconciled by GET, never replayed. */
export class DmNotificationPreference {
  state = initial();
  private closed = false;
  private revision = 0;
  private request?: AbortController;
  constructor(
    private readonly api: DmNotificationAPI,
    private readonly changed: (state: DmNotificationState) => void,
  ) {}

  private publish(change: Partial<DmNotificationState>): void {
    if (this.closed) return;
    this.state = { ...this.state, ...change };
    this.changed(this.state);
  }

  async refresh(): Promise<void> {
    if (this.closed || this.state.saving) return;
    this.request?.abort();
    const request = new AbortController();
    this.request = request;
    const revision = ++this.revision;
    this.publish({ loading: true, error: false });
    try {
      const enabled = await this.api.read(request.signal);
      if (!this.closed && revision === this.revision) this.publish({ enabled, loading: false });
    } catch {
      if (!this.closed && revision === this.revision) this.publish({ loading: false, error: true });
    }
  }

  async toggle(): Promise<void> {
    if (this.closed || this.state.loading || this.state.saving || this.state.error) return;
    const before = this.state.enabled;
    const request = new AbortController();
    this.request?.abort();
    this.request = request;
    const revision = ++this.revision;
    this.publish({ saving: true, error: false });
    try {
      // Another app may have changed the account since this menu was opened.
      // Adopt that value first: a stale label must not invert the newer choice.
      const current = await this.api.read(request.signal);
      if (this.closed || revision !== this.revision) return;
      if (current !== before) {
        this.publish({ enabled: current, saving: false });
        return;
      }
      await this.api.write(!before, request.signal);
      if (this.closed || revision !== this.revision) return;
      const enabled = await this.api.read(request.signal);
      if (!this.closed && revision === this.revision) this.publish({ enabled, saving: false });
    } catch {
      // An error may follow a committed PUT. Disable toggling until an explicit
      // refresh establishes what the Hub actually retained.
      if (!this.closed && revision === this.revision) this.publish({ saving: false, error: true });
    }
  }

  dispose(): void {
    this.closed = true;
    this.revision++;
    this.request?.abort();
  }
}
