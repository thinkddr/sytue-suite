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
/**
 * Whether Mail opens this account: an account connected only for Calendar,
 * Contacts or Notes is listed by the shared endpoint but has no mailbox here.
 * Without `services` (accounts from before linked services) it is a mailbox.
 */
export function servesMail(account: Pick<ExternalMailAccount, 'services'>): boolean {
  return (account.services ?? ['mail']).includes('mail');
}
/**
 * iCloud Mail offers no sign-in for other apps: it is IMAP and SMTP with an
 * app-specific password. Its servers are fixed, so Mail asks only for the
 * address and that password and saves an `imap` connection with these.
 */
export const ICLOUD_MAIL = {
  name: 'iCloud Mail',
  incoming: { host: 'imap.mail.me.com', port: 993, security: 'tls' },
  outgoing: { host: 'smtp.mail.me.com', port: 587, security: 'starttls' },
  /** Where the app-specific password is created: Sign-In and Security. */
  accountPage: 'https://account.apple.com/',
} as const;
/** Whether a listed connection is an iCloud mailbox saved through the preset. */
export function isICloudMail(account: Pick<ExternalMailAccount, 'provider' | 'configuration'>) {
  return (
    account.provider === 'imap' &&
    account.configuration?.incoming.host.toLowerCase() === ICLOUD_MAIL.incoming.host
  );
}
/** The provider's name for a listed connection: «iCloud Mail» for the preset. */
export function mailProviderName(
  account: Pick<ExternalMailAccount, 'provider' | 'configuration'>,
): string {
  return isICloudMail(account) ? ICLOUD_MAIL.name : MAIL_PROVIDERS[account.provider].name;
}
export interface PendingMailConsent {
  instance_id: string;
  provider: MailProvider;
  services: LinkedService[];
}
const PENDING_CONSENT = 'sytue.external-mail.consent';
/**
 * The sign-in this tab sent to a provider. Drafts are not listed, so after a
 * cancelled or expired sign-in this is the only record of which provider it
 * was: without it the wizard came back with Gmail chosen. `instance` is the
 * one in the return URL; without it (an unreadable state) the latest one is used,
 * which is the one still out: every return forgets it (`forgetMailConsent`).
 */
export function pendingMailConsent(instance?: string | null): PendingMailConsent | undefined {
  try {
    const raw = globalThis.sessionStorage?.getItem(PENDING_CONSENT);
    const pending = raw ? (JSON.parse(raw) as PendingMailConsent) : undefined;
    if (
      !pending ||
      !(pending.provider in MAIL_PROVIDERS) ||
      !MAIL_PROVIDERS[pending.provider].oauth
    )
      return undefined;
    return !instance || pending.instance_id === instance ? pending : undefined;
  } catch {
    return undefined;
  }
}
/**
 * Forgets the sign-in sent to the provider once its return is handled, whether
 * it worked or not: a later unreadable return must not name an old provider.
 */
export function forgetMailConsent(): void {
  try {
    globalThis.sessionStorage?.removeItem(PENDING_CONSENT);
  } catch {
    // Nothing stored can be read either.
  }
}
function rememberMailConsent(pending: PendingMailConsent): void {
  try {
    globalThis.sessionStorage?.setItem(PENDING_CONSENT, JSON.stringify(pending));
  } catch {
    // Without storage the return still works; it only cannot name the provider.
  }
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
/**
 * What the summary says of an account: `reconnect` asks the person to sign in
 * again; `unavailable` is the provider's (it did not answer in time or failed).
 */
export type ExternalMailSummaryState = 'ok' | 'reconnect' | 'unavailable';
/** An account as the summary lists it: its folders, empty unless `ok`. */
export interface ExternalMailAccountSummary extends ExternalMailAccount {
  state: ExternalMailSummaryState;
  folders: ExternalMailFolder[];
}
/** `GET /external-mail/summary`: the mailboxes Mail lists, oldest first. */
export interface ExternalMailSummary {
  items: ExternalMailAccountSummary[];
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
  /**
   * The mailboxes Mail's sidebar paints, each with its folders and unread
   * counts, in one request: only the accounts that serve Mail, and an account
   * that fails does not fail the others.
   */
  summary(signal?: AbortSignal): Promise<ExternalMailSummary> {
    return this.client.json({ path: '/external-mail/summary', ...(signal ? { signal } : {}) });
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
    rememberMailConsent({
      instance_id: result.instance_id,
      provider,
      services: services ?? ['mail'],
    });
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
