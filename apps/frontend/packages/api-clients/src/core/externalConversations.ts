/* SPDX-License-Identifier: Apache-2.0 */
import type { HttpClient } from './http.js';
import { providerAuthorizationURL } from './appConsent.js';

export const EXTERNAL_CHAT_PROVIDERS = {
  slack: { name: 'Slack', host: 'slack.com' },
  'microsoft-teams': { name: 'Microsoft Teams', host: 'login.microsoftonline.com' },
  'google-chat': { name: 'Google Chat', host: 'accounts.google.com' },
} as const;
export type ExternalChatProvider = keyof typeof EXTERNAL_CHAT_PROVIDERS;
export interface ExternalChatCapabilities {
  read: boolean;
  send: boolean;
  threads: boolean;
  edit: boolean;
  delete: boolean;
  attachments: boolean;
  e2ee: boolean;
}
export interface ExternalChatAccount {
  instance_id: string;
  provider: ExternalChatProvider;
  name: string;
  connected: boolean;
  workspace_name?: string;
  account_id?: string;
}
/** Personal video meeting account (Google Meet), connected by consent like a chat. */
export interface ExternalMeetingAccount {
  provider: 'google-meet' | 'microsoft-teams';
  name?: string;
  available: boolean;
  disabled_by_org?: boolean;
  connected?: boolean;
  instance_id?: string;
  account_id?: string;
  e2ee_required?: boolean;
}
export interface ExternalMeetingResult {
  provider: string;
  meeting_id: string;
  url: string;
  e2ee: boolean;
  official_client_required: boolean;
}

export function teamsMeetingAppURL(raw: string): string {
  const url = new URL(raw);
  if (
    url.origin !== 'https://teams.microsoft.com' ||
    url.username ||
    url.password ||
    url.hash ||
    !(
      (url.pathname.startsWith('/l/meetup-join/') &&
        url.pathname.length > '/l/meetup-join/'.length) ||
      (url.pathname.startsWith('/meet/') && url.pathname.length > '/meet/'.length)
    )
  )
    throw new Error('invalid_protected_teams_url');
  return `msteams://${url.host}${url.pathname}${url.search}`;
}
export interface ExternalChatAccounts {
  items: ExternalChatAccount[];
  meetings?: ExternalMeetingAccount[];
  providers: {
    provider: ExternalChatProvider;
    available: boolean;
    capabilities: ExternalChatCapabilities;
  }[];
}
export interface ExternalChatChannel {
  id: string;
  name: string;
  kind: string;
  capabilities: ExternalChatCapabilities;
}
export interface ExternalChatMessage {
  id: string;
  remote_id: string;
  author_id: string;
  author_name?: string;
  text: string;
  created_at: string;
  thread_id?: string;
  reply_count?: number;
  edited?: boolean;
  owned: boolean;
}
export interface ExternalChatPage<T> {
  items: T[];
  next_cursor?: string;
}
export interface ExternalChatMessages extends ExternalChatPage<ExternalChatMessage> {
  capabilities: ExternalChatCapabilities;
}

/** Personal provider conversations. Content never enters a native Rooms channel. */
export class ExternalConversationsClient {
  constructor(private readonly client: HttpClient) {}
  accounts(signal?: AbortSignal): Promise<ExternalChatAccounts> {
    return this.client.json({
      path: '/external-conversations/accounts',
      ...(signal ? { signal } : {}),
    });
  }
  async connect(
    provider: ExternalChatProvider | 'google-meet',
    instance?: string,
    returnTo: 'rooms' | 'native-rooms' = 'rooms',
    purpose?: 'meeting',
  ): Promise<{ instance_id: string; authorize_url: string; scopes: string[] }> {
    const result = await this.client.postJson<{
      instance_id: string;
      authorize_url: string;
      scopes: string[];
    }>('/external-conversations/connect', {
      provider,
      return_to: returnTo,
      ...(instance ? { instance_id: instance } : {}),
      ...(purpose ? { purpose } : {}),
    });
    const url = providerAuthorizationURL(
      result.authorize_url,
      provider === 'google-meet' ? 'accounts.google.com' : EXTERNAL_CHAT_PROVIDERS[provider].host,
      'external',
    );
    return { ...result, authorize_url: url.href };
  }
  connectMeeting(provider: ExternalMeetingAccount['provider'], instance?: string) {
    return this.connect(
      provider,
      instance,
      'rooms',
      provider === 'microsoft-teams' ? 'meeting' : undefined,
    );
  }

  async protectedTeamsMeeting(
    instance: string,
    account: string,
    meeting?: string,
  ): Promise<ExternalMeetingResult> {
    if (!instance || !account) throw new Error('meeting_account_required');
    const result = await this.client.json<ExternalMeetingResult>({
      method: meeting ? 'PATCH' : 'POST',
      path: '/external-conversations/meeting',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({
        provider: 'microsoft-teams',
        instance_id: instance,
        expected_account_id: account,
        e2ee: true,
        ...(meeting ? { meeting_id: meeting } : {}),
      }),
    });
    if (
      result.provider !== 'microsoft-teams' ||
      !result.e2ee ||
      !result.official_client_required ||
      !result.meeting_id ||
      (meeting && result.meeting_id !== meeting)
    )
      throw new Error('invalid_protected_teams_confirmation');
    teamsMeetingAppURL(result.url);
    return result;
  }
  /** A new Google Meet link from the caller's own account. */
  meeting(instance?: string): Promise<{ url: string }> {
    return this.client.postJson('/external-conversations/meeting', {
      provider: 'google-meet',
      ...(instance ? { instance_id: instance } : {}),
    });
  }
  channels(
    instance: string,
    cursor = '',
    signal?: AbortSignal,
  ): Promise<ExternalChatPage<ExternalChatChannel>> {
    return this.client.json({
      path: this.path(instance, 'channels'),
      query: { cursor },
      ...(signal ? { signal } : {}),
    });
  }
  messages(
    instance: string,
    remote: string,
    options: { cursor?: string; thread?: string; signal?: AbortSignal } = {},
  ): Promise<ExternalChatMessages> {
    return this.client.json({
      path: this.path(instance, 'messages'),
      query: { remote_id: remote, cursor: options.cursor, thread_id: options.thread },
      ...(options.signal ? { signal: options.signal } : {}),
    });
  }
  send(
    instance: string,
    remote: string,
    text: string,
    thread?: string,
  ): Promise<ExternalChatMessage> {
    return this.client.postJson(this.path(instance, 'messages'), {
      remote_id: remote,
      text,
      ...(thread ? { thread_id: thread } : {}),
    });
  }
  action(
    instance: string,
    action: 'edit' | 'delete',
    remote: string,
    message: string,
    text?: string,
    thread?: string,
  ): Promise<void> {
    return this.client.postJson(this.path(instance, 'actions'), {
      action,
      remote_id: remote,
      message_id: message,
      ...(text !== undefined ? { text } : {}),
      ...(thread ? { thread_id: thread } : {}),
    });
  }
  async disconnect(instance: string): Promise<void> {
    await this.client.request({ method: 'DELETE', path: this.path(instance) });
  }
  private path(instance: string, suffix?: string): string {
    return `/external-conversations/${encodeURIComponent(instance)}${suffix ? `/${suffix}` : ''}`;
  }
}
