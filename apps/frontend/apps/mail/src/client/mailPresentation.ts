/* SPDX-License-Identifier: Apache-2.0 */
import { senderAuthDe, type EmailSummary } from '@sytue/api-clients';

/** Forma cruda del sobre (domain.Message del Go): snake_case, flags IMAP. */
export interface RawMessage {
  id: string;
  thread_id?: string;
  subject?: string;
  from?: string;
  from_name?: string;
  to?: string[];
  cc?: string[];
  bcc?: string[];
  preview?: string;
  received_at?: string;
  unsnoozed_at?: string;
  keywords?: string[];
  has_attachment?: boolean;
  thread_count?: number;
  mailbox_id?: string;
  auth_spf?: string;
  auth_dkim?: string;
  auth_dmarc?: string;
}

/** Flag IMAP (\Seen) → keyword JMAP ($seen); el resto en minúsculas. */
const FLAG_TO_KEYWORD: Record<string, string> = {
  '\\Seen': '$seen',
  '\\Flagged': '$flagged',
  '\\Draft': '$draft',
  '\\Answered': '$answered',
  '\\Deleted': '$deleted',
};

function flagsToKeywords(flags?: string[]): Record<string, boolean> {
  const kw: Record<string, boolean> = {};
  for (const f of flags ?? []) kw[FLAG_TO_KEYWORD[f] ?? f.toLowerCase()] = true;
  return kw;
}

/**
 * Normaliza un `domain.Message` crudo del servicio Go al `EmailSummary` del SDK
 * (misma transformación que `mapEmail` del SDK, que no es importable desde aquí).
 */
export function toEmailSummary(m: RawMessage): EmailSummary {
  return {
    id: m.id,
    threadId: m.thread_id ?? '',
    subject: m.subject ?? '(Sin asunto)',
    from: m.from ? [{ ...(m.from_name ? { name: m.from_name } : {}), email: m.from }] : [],
    to: (m.to ?? []).map((email) => ({ email })),
    cc: (m.cc ?? []).map((email) => ({ email })),
    ...(m.bcc?.length ? { bcc: m.bcc.map((email) => ({ email })) } : {}),
    preview: m.preview ?? '',
    receivedAt: m.received_at ?? '',
    ...(m.unsnoozed_at ? { unsnoozedAt: m.unsnoozed_at } : {}),
    keywords: flagsToKeywords(m.keywords),
    hasAttachment: Boolean(m.has_attachment),
    ...(m.thread_count ? { threadCount: m.thread_count } : {}),
    ...(m.mailbox_id ? { mailboxId: m.mailbox_id } : {}),
    ...senderAuthDe(m),
  };
}
