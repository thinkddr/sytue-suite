/* SPDX-License-Identifier: Apache-2.0 */
import type { HttpClient } from './http.js';
import { HttpError } from './http.js';
import type { ExternalMailAccount } from './externalMail.js';
import type { EventInput } from '../calendar/index.js';
import type { ContactInput } from '../contacts/index.js';

export type LinkedService = 'mail' | 'calendar' | 'contacts' | 'notes';
export interface LinkedServiceCapability {
  service: LinkedService;
  available: boolean;
  reason?: string;
}
export interface RemoteCollection {
  id: string;
  name: string;
  color?: string;
  time_zone?: string;
  read_only: boolean;
}
export interface RemotePage<T> {
  items: T[];
  next_cursor?: string;
}
export interface RemoteResource {
  id: string;
  etag?: string;
  read_only: boolean;
  provider_url?: string;
}
export interface RemoteEvent
  extends RemoteResource, Pick<EventInput, 'summary' | 'description' | 'location' | 'start'> {
  collection_id: string;
  end: string;
  time_zone?: string;
  all_day: boolean;
  recurrence?: string[];
  attendees?: { email: string; name?: string }[];
}
export type RemoteEventPatch = Partial<
  Pick<EventInput, 'summary' | 'description' | 'location' | 'start' | 'end'>
> & { all_day?: boolean; time_zone?: string };
export interface RemoteContact
  extends RemoteResource, Pick<ContactInput, 'emails' | 'phones' | 'organization'> {
  collection_id: string;
  name: string;
  given_name?: string;
  family_name?: string;
  title?: string;
  note?: string;
  addresses?: string[];
}
export type RemoteContactPatch = Partial<Pick<ContactInput, 'emails' | 'phones' | 'organization'>> &
  Partial<Pick<RemoteContact, 'name' | 'given_name' | 'family_name' | 'title' | 'note'>>;
export interface RemoteNote extends RemoteResource {
  collection_id: string;
  title: string;
  text?: string;
  updated_at: string;
  can_append: boolean;
}

/** Live provider data. Changes carry the account identity and version that the
 * editor opened. An ambiguous write is returned to the UI and never replayed. */
export class LinkedServicesClient {
  constructor(private readonly client: HttpClient) {}
  collections(
    account: ExternalMailAccount,
    service: Exclude<LinkedService, 'mail'>,
    signal?: AbortSignal,
  ): Promise<RemotePage<RemoteCollection>> {
    return this.read(account, `${service}/collections`, {}, signal);
  }
  events(
    account: ExternalMailAccount,
    collection: string,
    from: string,
    to: string,
    cursor = '',
    signal?: AbortSignal,
  ): Promise<RemotePage<RemoteEvent>> {
    return this.read(account, 'calendar/events', { collection, from, to, cursor }, signal);
  }
  event(
    account: ExternalMailAccount,
    collection: string,
    id: string,
    signal?: AbortSignal,
  ): Promise<RemoteEvent> {
    return this.read(account, 'calendar/event', { collection, id }, signal);
  }
  saveEvent(
    account: ExternalMailAccount,
    collection: string,
    id: string,
    etag: string,
    patch: RemoteEventPatch,
  ): Promise<RemoteEvent> {
    return this.write(account, 'calendar/save', { collection, id, etag, patch });
  }
  contacts(
    account: ExternalMailAccount,
    collection: string,
    cursor = '',
    signal?: AbortSignal,
  ): Promise<RemotePage<RemoteContact>> {
    return this.read(account, 'contacts/items', { collection, cursor }, signal);
  }
  contact(
    account: ExternalMailAccount,
    collection: string,
    id: string,
    signal?: AbortSignal,
  ): Promise<RemoteContact> {
    return this.read(account, 'contacts/item', { collection, id }, signal);
  }
  saveContact(
    account: ExternalMailAccount,
    collection: string,
    id: string,
    etag: string,
    patch: RemoteContactPatch,
  ): Promise<RemoteContact> {
    return this.write(account, 'contacts/save', { collection, id, etag, patch });
  }
  notes(
    account: ExternalMailAccount,
    collection: string,
    cursor = '',
    signal?: AbortSignal,
  ): Promise<RemotePage<RemoteNote>> {
    return this.read(account, 'notes/items', { collection, cursor }, signal);
  }
  note(
    account: ExternalMailAccount,
    collection: string,
    id: string,
    signal?: AbortSignal,
  ): Promise<RemoteNote> {
    return this.read(account, 'notes/item', { collection, id }, signal);
  }
  createNote(
    account: ExternalMailAccount,
    collection: string,
    title: string,
    text: string,
  ): Promise<RemoteNote> {
    return this.write(account, 'notes/write', { collection, title, text });
  }
  appendNote(
    account: ExternalMailAccount,
    collection: string,
    id: string,
    text: string,
  ): Promise<void> {
    return this.write(account, 'notes/write', { collection, id, text });
  }
  private path(account: ExternalMailAccount, suffix: string): string {
    return `/linked-services/${encodeURIComponent(account.instance_id)}/${suffix}`;
  }
  private read<T>(
    account: ExternalMailAccount,
    suffix: string,
    query: Record<string, string>,
    signal?: AbortSignal,
  ): Promise<T> {
    return this.client.json({
      path: this.path(account, suffix),
      query: { ...query, expected_address: account.address },
      ...(signal ? { signal } : {}),
    });
  }
  private async write<T>(account: ExternalMailAccount, suffix: string, body: object): Promise<T> {
    const path = this.path(account, suffix);
    try {
      return await this.client.postJson<T>(path, { ...body, expected_address: account.address });
    } catch (error) {
      if (error instanceof HttpError && error.status >= 400 && error.status < 500) throw error;
      // The gateway may have completed a mutation before its response was lost.
      // A transport error, unreadable success or intermediary 5xx cannot invite replay.
      throw new HttpError(
        502,
        'Unknown write outcome',
        path,
        JSON.stringify({ error: { code: 'external_write_unknown' } }),
      );
    }
  }
}
