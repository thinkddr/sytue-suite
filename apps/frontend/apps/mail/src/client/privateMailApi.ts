/* SPDX-License-Identifier: Apache-2.0 */
import { createContext } from 'preact';
import { useContext } from 'preact/hooks';
import { type PrivateMailClient } from './privateMail.js';
import { toEmailSummary, type RawMessage } from './mailPresentation.js';
import * as legacyMailApi from './mailApi.js';
import {
  MailHttpError,
  TEMPLATE_BODY_MAX_BYTES,
  TEMPLATE_NAME_MAX,
  TEMPLATES_MAX,
  remoteImageProxyUrl,
  resolveIntent,
  searchContacts,
  cargarLibreta,
} from './mailApi.js';
import type {
  Alias,
  AttachmentMeta,
  DraftInput,
  DraftReadResult,
  EmailChange,
  FolderResult,
  Label,
  Mailbox,
  MailFilter,
  MailFilterInput,
  MessageBody,
  ReplyTemplate,
  ScheduledItem,
  SendInput,
  StorageQuota,
  Vacation,
} from './mailApi.js';

interface RawMailbox {
  id: string;
  name: string;
  role?: string;
  parent_id?: string;
  total_emails: number;
  unread_emails: number;
}
interface RawLabel {
  id: string;
  name: string;
  color: string;
  total_emails: number;
  unread_emails: number;
}
interface RawFilter {
  id: string;
  name: string;
  enabled: boolean;
  priority: number;
  match_from: string;
  match_subject: string;
  move_to?: string;
  mark_read: boolean;
  mark_flagged: boolean;
  revision: string;
}
interface RawVacation {
  enabled: boolean;
  subject: string;
  body: string;
  starts_at?: string;
  ends_at?: string;
  revision: string;
}
interface View {
  text: string;
  html: string;
  to: string[];
  cc: string[];
  bcc: string[];
  subject: string;
  attachments: AttachmentMeta[];
}
interface RawScheduled {
  id: string;
  from: string;
  recipients: string[];
  subject: string;
  release_at: string;
  created_at: string;
}
interface DraftAttempt {
  scope: Record<string, string>;
  epoch: string;
  editor: string;
  revision: string;
  id: string;
}
interface OutgoingAttempt {
  scope: Record<string, string>;
  epoch: string;
  operation: string;
}
interface OutgoingStatus {
  Attempt: OutgoingAttempt;
  ReleaseAt: string;
  Queued: boolean;
}
export interface DraftIntent {
  readonly editor: string;
  readonly revision: string;
}
export interface SendIntent {
  readonly operation: string;
}
interface RawTemplate {
  id: string;
  name: string;
  body: string;
  updated_at: string;
}
const replyTemplate = (v: RawTemplate): ReplyTemplate => ({
  id: v.id,
  name: v.name,
  body: v.body,
  updatedAt: v.updated_at,
});
type LegacyAPI = typeof legacyMailApi;
const unavailable = (..._args: unknown[]): Promise<never> =>
  Promise.reject(new MailHttpError(409, 'private_mail_operation_unavailable'));
const box = (v: RawMailbox): Mailbox => ({
  id: v.id,
  name: v.name,
  ...(v.role ? { role: v.role } : {}),
  ...(v.parent_id ? { parentId: v.parent_id } : {}),
  totalEmails: v.total_emails,
  unreadEmails: v.unread_emails,
});
const label = (v: RawLabel): Label => ({
  id: v.id,
  name: v.name,
  color: v.color,
  totalEmails: v.total_emails,
  unreadEmails: v.unread_emails,
});
const filter = (v: RawFilter): MailFilter => ({
  id: v.id,
  name: v.name,
  enabled: v.enabled,
  priority: v.priority,
  matchFrom: v.match_from,
  matchSubject: v.match_subject,
  moveTo: v.move_to ?? null,
  markRead: v.mark_read,
  markFlagged: v.mark_flagged,
  revision: v.revision,
});
const change = (v: RawMessage & { mailbox_id: string }): EmailChange => ({
  ...toEmailSummary(v),
  mailboxId: v.mailbox_id,
});
/** El keyword JMAP de una marca en lote → el flag IMAP con que la guarda el núcleo. */
const FLAG_IMAP = {
  $seen: '\\Seen',
  $flagged: '\\Flagged',
  $answered: '\\Answered',
} as const;

const folderReason = (v: string): FolderResult =>
  v === ''
    ? 'ok'
    : v === 'forbidden'
      ? 'system'
      : v === 'mailboxHasEmail'
        ? 'notEmpty'
        : v === 'alreadyExists'
          ? 'exists'
          : 'invalid';

/** Captures one application. Display DTO adaptation is the only policy here;
 * MIME, immutable attempts, CAS revisions and content authority stay in Go. */
export function createPrivateMailApi(client: PrivateMailClient) {
  const call = <T>(action: string, params: Record<string, unknown> = {}, body?: Uint8Array) =>
    client.call<T>(action, params, body);
  const checkTemplate = async (id: string | undefined, name: string, body: string) => {
    const trimmed = name.trim();
    // Code points, as Go's utf8.RuneCountInString counts the service's limit.
    const runes = Array.from(trimmed);
    if (
      !trimmed ||
      runes.length > TEMPLATE_NAME_MAX ||
      runes.some((c) => c.charCodeAt(0) < 0x20 || c.charCodeAt(0) === 0x7f) ||
      body.includes('\u0000') ||
      new TextEncoder().encode(body).byteLength > TEMPLATE_BODY_MAX_BYTES
    )
      throw new MailHttpError(400, 'invalid_template');
    const current = (await call<RawTemplate[] | null>('templates')) ?? [];
    if (id !== undefined && !current.some((v) => v.id === id))
      throw new MailHttpError(404, 'not_found');
    const lower = trimmed.toLowerCase();
    if (current.some((v) => v.id !== id && v.name.toLowerCase() === lower))
      throw new MailHttpError(409, 'template_exists');
    if (id === undefined && current.length >= TEMPLATES_MAX)
      throw new MailHttpError(409, 'template_limit');
  };
  const composeInput = (input: DraftInput | SendInput, draft: boolean) => ({
    from: 'from' in input ? input.from : undefined,
    to: input.to.map((v) => (v.name ? JSON.stringify(v.name) + ' <' + v.email + '>' : v.email)),
    cc: (input.cc ?? []).map((v) =>
      v.name ? JSON.stringify(v.name) + ' <' + v.email + '>' : v.email,
    ),
    bcc: ('bcc' in input ? (input.bcc ?? []) : []).map((v) => v.email),
    subject: input.subject,
    text: input.textBody ?? '',
    html: input.htmlBody ?? '',
    draft,
    attachments: (input.attachments ?? []).map((v) => ({
      name: v.name,
      type: v.type,
      body: v.contentBase64,
      cid: v.contentId ?? '',
    })),
  });
  const download = (body: Uint8Array<ArrayBuffer>, name: string, type: string): void => {
    let url: string | undefined;
    try {
      url = client.objectURL(body, type);
      const link = document.createElement('a');
      link.href = url;
      link.download = name;
      link.click();
    } finally {
      client.releaseBody(body);
      if (url) {
        const held = url;
        setTimeout(() => client.releaseURL(held), 0);
      }
    }
  };
  const readView = async (id: string): Promise<MessageBody> => {
    const value = await call<View>('mime.view', { id });
    const urls: string[] = [];
    try {
      const attachments = await Promise.all(
        (value.attachments ?? []).map(async (meta, index) => {
          if (!meta.cid && !meta.inline) return meta;
          if (!/^image\/(png|jpeg|gif|webp|avif|bmp|x-icon)$/i.test(meta.type)) return meta;
          const body = await client.attachment(id, index);
          try {
            const url = client.objectURL(body, meta.type);
            urls.push(url);
            return { ...meta, url };
          } finally {
            client.releaseBody(body);
          }
        }),
      );
      return {
        text: value.text,
        html: value.html,
        attachments,
        release: () => {
          for (const url of urls) client.releaseURL(url);
          urls.length = 0;
        },
      };
    } catch (error) {
      for (const url of urls) client.releaseURL(url);
      throw error;
    }
  };
  const draftEntries = new Map<
    string,
    {
      input: DraftInput;
      composed?: Awaited<ReturnType<PrivateMailClient['compose']>>;
      attempt?: DraftAttempt;
    }
  >();
  const outboundEntries = new Map<
    string,
    {
      input: SendInput;
      composed?: Awaited<ReturnType<PrivateMailClient['compose']>>;
      attempt?: OutgoingAttempt;
      release: string;
    }
  >();
  const api = {
    listMailboxes: async () => {
      return { mailboxes: ((await call<RawMailbox[] | null>('mailboxes')) ?? []).map(box) };
    },
    listEmails: async (id: string, opts?: { limit?: number; cursor?: string }) => {
      const v = await call<{ messages: RawMessage[] | null; cursor: string }>('messages', {
        id,
        cursor: opts?.cursor ?? '',
        limit: opts?.limit ?? 50,
      });
      return { emails: (v.messages ?? []).map(toEmailSummary), nextCursor: v.cursor };
    },
    listLabelEmails: async (id: string, opts?: { limit?: number; cursor?: string }) => {
      const v = await call<{ messages: RawMessage[] | null; cursor: string }>('label.messages', {
        id,
        cursor: opts?.cursor ?? '',
        limit: opts?.limit ?? 50,
      });
      return { emails: (v.messages ?? []).map(toEmailSummary), nextCursor: v.cursor };
    },
    searchEmails: async (
      query: string,
      opts?: { mailboxId?: string; limit?: number; position?: number },
    ) => {
      if (opts?.mailboxId) throw new MailHttpError(409, 'private_mail_scoped_search_unavailable');
      return {
        emails: (
          (await call<RawMessage[] | null>('search', {
            query,
            offset: opts?.position ?? 0,
            limit: opts?.limit ?? 50,
          })) ?? []
        ).map(toEmailSummary),
      };
    },
    // El modo privado no tiene posponer ni silenciar (son del servicio): el hilo llega
    // sin ese estado, con el mismo tipo que el de mailApi para que MailApp lo lea igual.
    listThreadEmails: async (id: string): ReturnType<LegacyAPI['listThreadEmails']> => {
      return {
        emails: (
          (await call<Array<RawMessage & { mailbox_id: string }> | null>('thread', { id })) ?? []
        ).map(change),
      };
    },
    // Posponer y silenciar son del servicio, que mueve los hilos al vencer: el modo
    // privado no los tiene. Nada pospuesto, y las órdenes se rechazan sin tocar la red:
    // por `mailApi` irían al servicio en claro, con hilos que no son los de esta vista.
    admitePosponer: false as boolean,
    listSnoozes: (): ReturnType<LegacyAPI['listSnoozes']> => Promise.resolve({ snoozes: [] }),
    snoozeThread: unavailable as LegacyAPI['snoozeThread'],
    unsnoozeThread: unavailable as LegacyAPI['unsnoozeThread'],
    muteThread: unavailable as LegacyAPI['muteThread'],
    fetchChanges: async (since: string) => {
      const v = await call<{
        changed: Array<RawMessage & { mailbox_id: string }> | null;
        destroyed: string[] | null;
        state: string;
        has_more: boolean;
      }>('changes', { since });
      return {
        changed: (v.changed ?? []).map(change),
        destroyed: v.destroyed ?? [],
        state: v.state,
        hasMore: v.has_more,
      };
    },
    createMailbox: async (name: string): Promise<FolderResult> => {
      await call('mailbox.create', { name });
      return 'ok';
    },
    renameMailbox: async (id: string, name: string) => {
      return folderReason((await call<{ reason: string }>('mailbox.rename', { id, name })).reason);
    },
    deleteMailbox: async (id: string) => {
      return folderReason((await call<{ reason: string }>('mailbox.delete', { id })).reason);
    },
    batchMove: async (ids: string[], mailbox: string) => {
      await call('messages.move', { ids, mailbox });
      return { moved: ids.length };
    },
    batchSetFlag: async (
      ids: string[],
      keyword: '$seen' | '$flagged' | '$answered',
      add: boolean,
    ) => {
      await call('messages.flag', { ids, flag: FLAG_IMAP[keyword], add });
      return { updated: ids.length };
    },
    purgeEmails: async (ids: string[]) => {
      if (ids.length !== 1) return unavailable();
      await call('message.delete', { id: ids[0] });
      return { deleted: 1 };
    },
    listLabels: async () => {
      return { labels: ((await call<RawLabel[] | null>('labels')) ?? []).map(label) };
    },
    createLabel: async (name: string, color: string) => {
      return { label: label(await call<RawLabel>('label.create', { name, color })) };
    },
    updateLabel: async (id: string, name: string, color: string) => {
      await call('label.update', { id, name, color });
      return { ok: true };
    },
    deleteLabel: async (id: string) => {
      await call('label.delete', { id });
      return { ok: true };
    },
    applyLabel: async (id: string, ids: string[], add: boolean) => {
      await call('label.apply', { id, ids, add });
      return { ok: true };
    },
    listAliases: async () => {
      return await call<{ primary: string; aliases: Alias[] }>('aliases.refresh');
    },
    createAlias: async (address: string) => {
      return { alias: await call<Alias>('alias.create', { address }) };
    },
    deleteAlias: async (id: string) => {
      await call('alias.delete', { id });
      return { ok: true };
    },
    getStorageQuota: async (): Promise<StorageQuota> => {
      const v = await call<{
        used_bytes: number;
        quota_bytes: number;
        external_used_bytes?: number;
        pending_delete_bytes?: number;
        ciphertext_bytes?: number;
      }>('quota');
      return {
        usedBytes: v.used_bytes,
        quotaBytes: v.quota_bytes,
        externalUsedBytes: v.external_used_bytes ?? 0,
        pendingDeleteBytes: v.pending_delete_bytes ?? 0,
        ciphertextBytes: v.ciphertext_bytes ?? 0,
      };
    },
    getSignature: async () => {
      return { signature: await call<string>('signature') };
    },
    saveSignature: async (value: string) => {
      await call('signature.set', { value });
      return { ok: true };
    },
    listFilters: async () => {
      return { filters: ((await call<RawFilter[] | null>('filters')) ?? []).map(filter) };
    },
    saveFilter: async (input: MailFilterInput & { id?: string; revision?: string }) => {
      const params = {
        id: input.id ?? '',
        name: input.name ?? '',
        enabled: input.enabled ?? true,
        priority: input.priority ?? 0,
        match_from: input.matchFrom ?? '',
        match_subject: input.matchSubject ?? '',
        move_to: input.moveTo ?? '',
        mark_read: input.markRead ?? false,
        mark_flagged: input.markFlagged ?? false,
        revision: input.revision ?? '',
      };
      if (input.id) {
        await call('filter.update', params);
        return { ok: true };
      }
      return { filter: filter(await call<RawFilter>('filter.create', params)) };
    },
    deleteFilter: async (id: string) => {
      await call('filter.delete', { id });
      return { ok: true };
    },
    // Reply templates are private text, like the signature. Go enforces the
    // limits again; checking here first gives the dialog the service's reasons.
    listTemplates: async (): Promise<{ templates: ReplyTemplate[] }> => {
      return {
        templates: ((await call<RawTemplate[] | null>('templates')) ?? []).map(replyTemplate),
      };
    },
    createTemplate: async (name: string, body: string): Promise<{ template: ReplyTemplate }> => {
      await checkTemplate(undefined, name, body);
      return {
        template: replyTemplate(await call<RawTemplate>('template.save', { id: '', name, body })),
      };
    },
    updateTemplate: async (
      id: string,
      name: string,
      body: string,
    ): Promise<{ template: ReplyTemplate }> => {
      await checkTemplate(id, name, body);
      return {
        template: replyTemplate(await call<RawTemplate>('template.save', { id, name, body })),
      };
    },
    deleteTemplate: async (id: string): Promise<{ ok: boolean }> => {
      const current = (await call<RawTemplate[] | null>('templates')) ?? [];
      if (!current.some((v) => v.id === id)) throw new MailHttpError(404, 'not_found');
      await call('template.delete', { id });
      return { ok: true };
    },
    getVacation: async (): Promise<Vacation> => {
      const v = await call<RawVacation>('vacation');
      return {
        enabled: v.enabled,
        subject: v.subject,
        body: v.body,
        ...(v.starts_at ? { startsAt: v.starts_at } : {}),
        ...(v.ends_at ? { endsAt: v.ends_at } : {}),
        revision: v.revision,
      };
    },
    saveVacation: async (v: Vacation) => {
      await call('vacation.set', {
        enabled: v.enabled,
        subject: v.subject,
        body: v.body,
        starts_at: v.startsAt || null,
        ends_at: v.endsAt || null,
        revision: v.revision ?? '',
      });
      return { ok: true };
    },
    getMessageBody: readView,
    listAttachments: async (id: string) => {
      return { attachments: (await call<View>('mime.view', { id })).attachments ?? [] };
    },
    downloadAttachment: async (id: string, index: number, name: string, type: string) => {
      download(await client.attachment(id, index), name, type);
    },
    downloadOriginal: async (id: string, subject: string) => {
      download(await client.raw(id), subject + '.eml', 'message/rfc822');
    },
    readDraft: async (id: string): Promise<DraftReadResult> => {
      const v = await call<View>('mime.view', { id });
      const attachments = await Promise.all(
        (v.attachments ?? []).map(async (meta, index) => {
          const body = await client.attachment(id, index);
          try {
            let raw = '';
            for (let offset = 0; offset < body.length; offset += 8192)
              raw += String.fromCharCode(...body.subarray(offset, offset + 8192));
            return {
              name: meta.name,
              type: meta.type,
              contentBase64: btoa(raw),
              ...(meta.cid ? { contentId: meta.cid } : {}),
            };
          } finally {
            client.releaseBody(body);
          }
        }),
      );
      return {
        to: (v.to ?? []).join(', '),
        cc: (v.cc ?? []).join(', '),
        bcc: (v.bcc ?? []).join(', '),
        subject: v.subject,
        textBody: v.text,
        htmlBody: v.html,
        attachments,
      };
    },
    deleteDraft: async (id: string) => {
      await call('message.delete', { id });
    },
    saveDraft: async (
      input: DraftInput,
      intent: DraftIntent = { editor: crypto.randomUUID(), revision: crypto.randomUUID() },
    ) => {
      let entry = draftEntries.get(intent.revision);
      if (!entry) {
        entry = { input: structuredClone(input) };
        draftEntries.set(intent.revision, entry);
      }
      if (!entry.composed) entry.composed = await client.compose(composeInput(entry.input, true));
      try {
        if (!entry.attempt)
          entry.attempt = await call<DraftAttempt>(
            'draft.prepare',
            {
              editor: intent.editor,
              revision: intent.revision,
              id: entry.input.id ?? '',
              from: entry.composed.from,
            },
            entry.composed.body,
          );
        const message = await call<RawMessage>('draft.commit', { attempt: entry.attempt });
        draftEntries.delete(intent.revision);
        return { id: message.id };
      } finally {
        if (entry.attempt && entry.composed) {
          client.releaseBody(entry.composed.body);
          delete entry.composed;
        }
      }
    },
    sendEmail: async (
      input: SendInput,
      intent: SendIntent = { operation: crypto.randomUUID() },
    ) => {
      let entry = outboundEntries.get(intent.operation);
      if (!entry) {
        entry = {
          input: structuredClone(input),
          release:
            input.sendAt ??
            (input.undoSeconds
              ? new Date(Date.now() + input.undoSeconds * 1000).toISOString()
              : '0001-01-01T00:00:00Z'),
        };
        outboundEntries.set(intent.operation, entry);
      }
      if (!entry.composed && !entry.attempt)
        entry.composed = await client.compose(composeInput(entry.input, false));
      try {
        if (!entry.attempt) {
          const composed = entry.composed;
          if (!composed) throw new Error('mail_private_intent');
          entry.attempt = await call<OutgoingAttempt>(
            'outbound.prepare',
            {
              operation: intent.operation,
              draft_id: entry.input.draftId ?? '',
              from: composed.from,
              recipients: composed.recipients,
              consent: 'smtp-transit-v1',
              release_at: entry.release,
            },
            composed.body,
          );
        }
        const status = await call<OutgoingStatus>('outbound.commit', { attempt: entry.attempt });
        outboundEntries.delete(intent.operation);
        return {
          ok: true,
          id: status.Attempt.operation,
          releaseAt: status.ReleaseAt,
          status: status.Queued ? 'queued' : 'accepted',
        };
      } finally {
        if (entry.attempt && entry.composed) {
          client.releaseBody(entry.composed.body);
          delete entry.composed;
        }
      }
    },
    listScheduled: async (_cursor?: string) => {
      return {
        scheduled: ((await call<RawScheduled[] | null>('scheduled')) ?? []).map(
          (v): ScheduledItem => ({
            id: v.id,
            from: v.from,
            recipients: v.recipients,
            subject: v.subject,
            releaseAt: v.release_at,
            createdAt: v.created_at,
          }),
        ),
        nextCursor: '',
      };
    },
    cancelScheduled: async (id: string) => {
      await call('scheduled.cancel', { id });
      return { canceled: true };
    },
    rescheduleScheduled: async (id: string, when: string) => {
      await call('scheduled.reschedule', { id, when });
      return { id, releaseAt: when };
    },
    getUnsubscribe: unavailable as LegacyAPI['getUnsubscribe'],
    unsubscribe: unavailable as LegacyAPI['unsubscribe'],
    listSharedMailboxes: unavailable as LegacyAPI['listSharedMailboxes'],
    listSharedFolders: unavailable as LegacyAPI['listSharedFolders'],
    listSharedEmails: unavailable as LegacyAPI['listSharedEmails'],
    createSharedMailbox: unavailable as LegacyAPI['createSharedMailbox'],
    listGrants: unavailable as LegacyAPI['listGrants'],
    grantAccess: unavailable as LegacyAPI['grantAccess'],
    revokeAccess: unavailable as LegacyAPI['revokeAccess'],
    listDelegation: unavailable as LegacyAPI['listDelegation'],
    delegateAccess: unavailable as LegacyAPI['delegateAccess'],
    revokeDelegation: unavailable as LegacyAPI['revokeDelegation'],
    resolveIntent,
    remoteImageProxyUrl,
    searchContacts,
    cargarLibreta,
  };
  return api;
}
export type PrivateMailApi = ReturnType<typeof createPrivateMailApi>;
export const PrivateMailApiContext = createContext<PrivateMailApi | null>(null);
export function usePrivateMailApi(): PrivateMailApi {
  const value = useContext(PrivateMailApiContext);
  return value ?? (legacyMailApi as unknown as PrivateMailApi);
}
