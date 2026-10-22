/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Sytue Mail — entrada del SDK. Expone el cliente JMAP (RFC 8620/8621) contra el
 * servidor Go propio (mail.sytue.eu, dev :8086). MailClient deriva el endpoint
 * de sesion JMAP estandar (/.well-known/jmap) a partir de la URL base del
 * servicio. Re-exporta el JmapClient de bajo nivel y sus tipos.
 */
import { JmapClient, type JmapOptions } from './jmap.js';
import type { AuthProvider } from '../core/http.js';

export interface MailOptions {
  /** URL base del servicio, p.ej. https://mail.sytue.eu */
  baseUrl: string;
  auth: AuthProvider;
  fetchImpl?: typeof fetch | undefined;
}

/**
 * Cliente de Sytue Mail. Construye un JmapClient apuntando al endpoint de sesion
 * JMAP canonico del servicio Go (`/.well-known/jmap`).
 */
export class MailClient extends JmapClient {
  constructor(options: MailOptions) {
    const jmap: JmapOptions = {
      sessionUrl: `${options.baseUrl.replace(/\/$/, '')}/.well-known/jmap`,
      auth: options.auth,
      fetchImpl: options.fetchImpl,
    };
    super(jmap);
  }
}

export {
  JmapClient,
  parseAttachments,
  parseDraftContent,
  parseMessageBody,
  parseSearchQuery,
} from './jmap.js';
export type {
  JmapOptions,
  JmapSession,
  Mailbox,
  MailboxSetError,
  EmailSummary,
  EmailAddress,
  OutgoingEmail,
  OutgoingAttachment,
  ParsedAttachment,
  DraftContent,
  MessageBody,
  MailFilter,
  MailFilterInput,
  SearchTokens,
} from './jmap.js';
