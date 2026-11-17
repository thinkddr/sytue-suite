/* SPDX-License-Identifier: Apache-2.0 */
import { safeAttachmentType, type MailPrivatePorts } from '@sytue/offline/matrix';
import { encodeHostJSON } from '@sytue/offline/pqc/framing';

interface DomainReply {
  data: Uint8Array;
  body?: Uint8Array;
}
export interface ComposedMail {
  from: string;
  recipients: string[];
  body: Uint8Array<ArrayBuffer>;
}
/** A facade belongs to ONE opened application. Captured handlers never switch
 * to the next signed-in account. Domain parsing, MIME and policy remain Go. */
export class PrivateMailClient {
  private readonly scope;
  private readonly runtime;
  private readonly port: string;
  private readonly session: string;
  private readonly bodies = new Set<Uint8Array<ArrayBuffer>>();
  private readonly urls = new Set<string>();
  private closed = false;
  private closing: Promise<void> | undefined;
  private readonly abort = (): void => this.detach();
  constructor(private readonly opened: MailPrivatePorts) {
    this.scope = Object.freeze({ ...opened.scope });
    this.runtime = opened.runtime;
    this.port = opened.port;
    this.session = opened.session;
    if (this.scope.product !== 'mail') throw new Error('mail_private_identity');
    opened.signal.addEventListener('abort', this.abort, { once: true });
    if (opened.signal.aborted) this.detach();
  }
  private check(): void {
    if (this.closed || this.opened.signal.aborted) throw new DOMException('Aborted', 'AbortError');
  }
  private async request(
    operation: 'mail.call' | 'mail.compose',
    input: Record<string, unknown>,
    payload?: Uint8Array,
  ): Promise<DomainReply> {
    this.check();
    const frame = encodeHostJSON({ ...input, session: this.session });
    let reply: DomainReply | undefined;
    try {
      reply = await this.runtime.invoke<DomainReply>(
        {
          operation,
          port: this.port,
          scope: this.scope,
          input: frame,
          ...(payload ? { payload } : {}),
        },
        this.opened.signal,
      );
      this.check();
      if (
        !(reply.data instanceof Uint8Array) ||
        (reply.body !== undefined &&
          (!(reply.body instanceof Uint8Array) || !(reply.body.buffer instanceof ArrayBuffer)))
      )
        throw new Error('mail_private_reply');
      return reply;
    } catch (error) {
      reply?.data?.fill(0);
      reply?.body?.fill(0);
      throw error;
    } finally {
      frame.fill(0);
    }
  }
  private takeBody(reply: DomainReply): Uint8Array<ArrayBuffer> {
    if (!reply.body || !(reply.body.buffer instanceof ArrayBuffer))
      throw new Error('mail_private_reply');
    const body = reply.body as Uint8Array<ArrayBuffer>;
    this.bodies.add(body);
    return body;
  }
  async call<T>(
    action: string,
    params: Record<string, unknown> = {},
    payload?: Uint8Array,
  ): Promise<T> {
    const reply = await this.request('mail.call', { action, params }, payload);
    try {
      // Display DTOs were already produced by the Go repository. Source state,
      // authority, receipts and cryptographic documents never use this parser.
      return JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(reply.data)) as T;
    } finally {
      reply.data.fill(0);
      reply.body?.fill(0);
    }
  }
  async raw(id: string): Promise<Uint8Array<ArrayBuffer>> {
    const reply = await this.request('mail.call', { action: 'raw', params: { id } });
    try {
      return this.takeBody(reply);
    } finally {
      reply.data.fill(0);
    }
  }
  async attachment(id: string, index: number): Promise<Uint8Array<ArrayBuffer>> {
    const reply = await this.request('mail.call', {
      action: 'mime.attachment',
      params: { id, index },
    });
    try {
      return this.takeBody(reply);
    } finally {
      reply.data.fill(0);
    }
  }
  async compose(params: Record<string, unknown>): Promise<ComposedMail> {
    const encoded = encodeHostJSON(params);
    let reply: DomainReply | undefined;
    try {
      reply = await this.request('mail.compose', {}, encoded);
      const value: unknown = JSON.parse(
        new TextDecoder('utf-8', { fatal: true }).decode(reply.data),
      );
      if (
        !value ||
        typeof value !== 'object' ||
        !('from' in value) ||
        typeof value.from !== 'string' ||
        !('recipients' in value) ||
        !Array.isArray(value.recipients) ||
        value.recipients.some((value) => typeof value !== 'string')
      )
        throw new Error('mail_private_reply');
      return {
        from: value.from,
        recipients: value.recipients as string[],
        body: this.takeBody(reply),
      };
    } catch (error) {
      reply?.body?.fill(0);
      throw error;
    } finally {
      encoded.fill(0);
      reply?.data.fill(0);
    }
  }
  releaseBody(body: Uint8Array): void {
    if (this.bodies.delete(body as Uint8Array<ArrayBuffer>)) body.fill(0);
  }
  /** URLs are ephemeral UI capabilities and are revoked with this account. */
  objectURL(body: Uint8Array<ArrayBuffer>, mime: string): string {
    this.check();
    if (!this.bodies.has(body)) throw new Error('mail_private_body');
    const buffer = body.buffer;
    const source =
      body.byteOffset === 0 && body.byteLength === buffer.byteLength
        ? buffer
        : buffer.slice(body.byteOffset, body.byteOffset + body.byteLength);
    const url = URL.createObjectURL(new Blob([source], { type: safeAttachmentType(mime) }));
    this.urls.add(url);
    return url;
  }
  releaseURL(url: string): void {
    if (this.urls.delete(url)) URL.revokeObjectURL(url);
  }
  detach(): void {
    if (this.closed) return;
    this.closed = true;
    this.opened.signal.removeEventListener('abort', this.abort);
    for (const body of this.bodies) body.fill(0);
    this.bodies.clear();
    for (const url of this.urls) URL.revokeObjectURL(url);
    this.urls.clear();
  }
  closeAndDrain(): Promise<void> {
    this.detach();
    return (this.closing ??= this.opened.closeAndDrain());
  }
}
