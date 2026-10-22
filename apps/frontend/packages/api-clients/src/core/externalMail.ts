/* SPDX-License-Identifier: Apache-2.0 */
import type { HttpClient } from './http.js';
import { providerAuthorizationURL } from './appConsent.js';
import type { LinkedService, LinkedServiceCapability } from './linkedServices.js';

export const MAIL_PROVIDERS = {
  gmail: { name: 'Gmail', host: 'accounts.google.com', oauth: true },
  microsoft: { name: 'Microsoft Outlook', host: 'login.microsoftonline.com', oauth: true },
  yahoo: { name: 'Yahoo Mail', host: 'api.login.yahoo.com', oauth: true },
  imap: { name: 'IMAP', host: '', oauth: false },
  pop3: { name: 'POP3', host: '', oauth: false },
} as const;
export type MailProvider = keyof typeof MAIL_PROVIDERS;
export interface ExternalMailAddress {
  name?: string;
  email: string;
}
export interface ExternalMailServer {
  host: string;
  port: number;
  username: string;
  security: 'tls' | 'starttls';
}
export interface ExternalMailConfiguration {
  provider: 'imap' | 'pop3';
  name: string;
  address: string;
  incoming: ExternalMailServer;
  outgoing: ExternalMailServer;
  password: string;
  smtp_password?: string;
  instance_id?: string;
}
export interface ExternalMailAccount {
  services?: LinkedService[];
  instance_id: string;
  provider: MailProvider;
  name: string;
  address: string;
  connected: boolean;
  configuration?: Pick<ExternalMailConfiguration, 'address' | 'incoming' | 'outgoing'>;
  capabilities: {
    read: boolean;
    send: boolean;
    actions: boolean;
    attachments: boolean;
    continuous_sync: boolean;
  };
}
export interface ExternalMailAccounts {
  items: ExternalMailAccount[];
  providers: {
    provider: MailProvider;
    available: boolean;
    authorization: 'oauth2' | 'password';
    services?: LinkedServiceCapability[];
  }[];
}
export interface ExternalMailFolder {
  id: string;
  name: string;
  role?: string;
  total?: number;
  unread?: number;
}
export interface ExternalMailAttachment {
  id?: string;
  name: string;
  type: string;
  size?: number;
  content_base64?: string;
}
export interface ExternalMailMessage {
  id: string;
  folder_id?: string;
  subject: string;
  from: ExternalMailAddress[];
  to: ExternalMailAddress[];
  cc?: ExternalMailAddress[];
  date: string;
  snippet?: string;
  text?: string;
  html?: string;
  seen: boolean;
  flagged: boolean;
  attachments?: ExternalMailAttachment[];
}
export interface ExternalMailOutgoing {
  expected_address: string;
  to: ExternalMailAddress[];
  cc?: ExternalMailAddress[];
  bcc?: ExternalMailAddress[];
  subject: string;
  text: string;
  html?: string;
  attachments?: ExternalMailAttachment[];
  in_reply_to?: string;
}
export interface ExternalMailPage {
  items: ExternalMailMessage[];
  next_cursor?: string;
}

/** Personal live mailbox access. Sending is never automatically retried. */
export class ExternalMailClient {
  constructor(private readonly client: HttpClient) {}
  accounts(signal?: AbortSignal): Promise<ExternalMailAccounts> {
    return this.client.json({ path: '/external-mail/accounts', ...(signal ? { signal } : {}) });
  }
  async connect(
    provider: MailProvider,
    returnTo:
      | 'mail'
      | 'native-mail'
      | 'external-calendar'
      | 'external-contacts'
      | 'external-notes' = 'mail',
    instance?: string,
    services?: LinkedService[],
  ): Promise<{ instance_id: string; authorize_url: string; scopes: string[] }> {
    if (!MAIL_PROVIDERS[provider].oauth) throw new Error('invalid_mail_provider');
    const result = await this.client.postJson<{
      instance_id: string;
      authorize_url: string;
      scopes: string[];
    }>('/external-mail/connect', {
      provider,
      return_to: returnTo,
      ...(instance ? { instance_id: instance } : {}),
      ...(services ? { services } : {}),
    });
    const url = providerAuthorizationURL(
      result.authorize_url,
      MAIL_PROVIDERS[provider].host,
      'mail',
    );
    return { ...result, authorize_url: url.href };
  }
  configure(configuration: ExternalMailConfiguration): Promise<{ instance_id: string }> {
    return this.client.postJson('/external-mail/accounts', configuration);
  }
  folders(
    instance: string,
    signal?: AbortSignal,
    expectedAddress?: string,
  ): Promise<{ items: ExternalMailFolder[] }> {
    return this.client.json({
      path: this.path(instance, 'folders'),
      query: { expected_address: expectedAddress },
      ...(signal ? { signal } : {}),
    });
  }
  messages(
    instance: string,
    folder: string,
    cursor = '',
    signal?: AbortSignal,
    expectedAddress?: string,
  ): Promise<ExternalMailPage> {
    return this.client.json({
      path: this.path(instance, 'messages'),
      query: { folder, cursor, expected_address: expectedAddress },
      ...(signal ? { signal } : {}),
    });
  }
  message(
    instance: string,
    folder: string,
    id: string,
    signal?: AbortSignal,
    expectedAddress?: string,
  ): Promise<ExternalMailMessage> {
    return this.client.json({
      path: this.path(instance, 'message'),
      query: { folder, id, expected_address: expectedAddress },
      ...(signal ? { signal } : {}),
    });
  }
  send(
    instance: string,
    outgoing: ExternalMailOutgoing,
  ): Promise<{ id?: string; sent_copy_saved?: boolean }> {
    return this.client.postJson(this.path(instance, 'send'), outgoing);
  }
  action(
    instance: string,
    folder: string,
    id: string,
    action: 'read' | 'unread' | 'flag' | 'unflag' | 'trash' | 'move',
    destination: string | undefined,
    expectedAddress: string,
  ): Promise<void> {
    return this.client.postJson(this.path(instance, 'actions'), {
      expected_address: expectedAddress,
      folder_id: folder,
      message_id: id,
      action,
      ...(destination ? { destination } : {}),
    });
  }
  async disconnect(instance: string): Promise<void> {
    await this.client.request({ method: 'DELETE', path: this.path(instance) });
  }
  private path(instance: string, part?: string): string {
    return `/external-mail/${encodeURIComponent(instance)}${part ? `/${part}` : ''}`;
  }
}
