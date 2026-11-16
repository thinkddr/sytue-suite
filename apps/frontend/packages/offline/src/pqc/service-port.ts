/* SPDX-License-Identifier: Apache-2.0 */
import { encodeStorageScopeHeader, storageScopeHeader } from '@sytue/config/matrix';
import { withBase } from '@sytue/config/base-path';
import { decodeHostObject, encodeHostJSON } from './host-codec.js';
import { PqcResponseStreams } from './response-streams.js';
import { PqcServiceSource, supportsRequestStreams } from './service-source.js';
import type {
  HostCallContext,
  HostHandler,
  HostLease,
  HostMethod,
  HostScope,
} from './host-port.js';
import type { PqcRuntime } from './runtime.js';

const sourceProducts = [
  'calendar',
  'notes',
  'photos',
  'cast',
  'docs',
  'sheets',
  'slides',
  'hub',
  'bases',
  'erp',
] as const;
export type PrivateServiceProduct = 'mail' | 'contacts' | (typeof sourceProducts)[number];
const allowedProducts = new Set<string>(['mail', 'contacts', ...sourceProducts]);

const operations = new Set([
  'contacts.source.state',
  'contacts.source.begin',
  'contacts.source.page',
  'contacts.source.open',
  ...sourceProducts.flatMap((product) => [
    `${product}.source.state`,
    `${product}.source.begin`,
    `${product}.source.page`,
    `${product}.source.open`,
  ]),
  'mail.routing.get',
  'mail.routing.create',
  'mail.routing.delete',
  'mail.routing.plan',
  'drive.cipher.usage',
  'mail.source.state',
  'mail.source.begin',
  'mail.source.page',
  'mail.source.open',
  'mail.ingress.challenge',
  'mail.ingress.apply',
  'mail.ingress.pending',
  'mail.ingress.revalidate',
  'mail.ingress.body',
  'mail.ingress.enrollment',
  'mail.submission.send',
  'mail.submission.register',
  'mail.submission.control',
  'mail.submission.receipt',
  'rooms.authority',
  'rooms.cutover.state',
  'rooms.cutover.begin',
  'rooms.cutover.complete',
]);
const reject = (): never => {
  throw new Error('service_port_rejected');
};
const id = (value: unknown): string => {
  if (typeof value !== 'string' || !/^[0-9a-f]{64}$/u.test(value)) return reject();
  return value;
};
export function privateServiceRoute(
  operation: string,
  args: readonly string[] = [],
  product: PrivateServiceProduct = 'mail',
): readonly [string, string] {
  if (
    !operations.has(operation) ||
    !allowedProducts.has(product) ||
    (!operation.startsWith('rooms.') &&
      (operation === 'drive.cipher.usage'
        ? product !== 'mail'
        : !operation.startsWith(product + '.')))
  )
    return reject();
  const encoded = args.map(encodeURIComponent);
  const epoch = (value: string | undefined) => {
    if (
      !value ||
      value === '00000000-0000-0000-0000-000000000000' ||
      !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/u.test(value)
    )
      return reject();
  };
  if (operation.endsWith('.source.state')) {
    if (args.length) return reject();
    return ['GET', '/private-storage/migration'];
  }
  if (operation.endsWith('.source.begin')) {
    if (args.length) return reject();
    return ['POST', '/private-storage/migration'];
  }
  if (operation.endsWith('.source.page')) {
    epoch(args[0]);
    if (args.length !== 2 || (args[1] !== '' && !/^[A-Za-z0-9_-]{1,8192}$/u.test(args[1] ?? '')))
      return reject();
    return [
      'GET',
      '/private-storage/migration/' +
        encoded[0] +
        '/rows' +
        (args[1] ? '?after=' + encoded[1] : ''),
    ];
  }
  if (operation !== 'contacts.source.open' && operation.endsWith('.source.open')) {
    epoch(args[0]);
    if (
      args.length !== 3 ||
      !/^[A-Za-z0-9._:-]{1,256}$/u.test(args[1] ?? '') ||
      !args[2] ||
      args[2].length > 2048 ||
      /[\0\r\n\t]/u.test(args[2])
    )
      return reject();
    return [
      'GET',
      '/private-storage/migration/' + encoded[0] + '/source/' + encoded[1] + '/' + encoded[2],
    ];
  }
  switch (operation) {
    case 'mail.routing.get':
      if (args.length) return reject();
      return ['GET', '/private-metadata/aliases'];
    case 'mail.routing.create':
      if (args.length) return reject();
      return ['POST', '/private-metadata/aliases'];
    case 'mail.routing.delete':
      if (args.length !== 1) return reject();
      epoch(args[0]);
      return ['DELETE', '/private-metadata/aliases/' + encoded[0]];
    case 'mail.routing.plan':
      if (args.length) return reject();
      return ['GET', '/private-metadata/quota'];
    case 'drive.cipher.usage':
      if (args.length) return reject();
      return ['GET', '/private-metadata/usage'];
    case 'contacts.source.open':
      epoch(args[0]);
      epoch(args[2]);
      if (
        args.length !== 3 ||
        !['book', 'card', 'contact', 'email', 'phone', 'group', 'member'].includes(args[1] ?? '')
      )
        return reject();
      return [
        'GET',
        '/private-storage/migration/' + encoded[0] + '/blobs/' + encoded[1] + '/' + encoded[2],
      ];
    case 'mail.submission.receipt':
      epoch(args[0]);
      if (args.length !== 2 || !/^[A-Za-z0-9._:-]{1,256}$/u.test(args[1] ?? '')) return reject();
      return ['GET', '/private-storage/submissions/' + encoded[0] + '/' + encoded[1]];
    case 'rooms.authority':
      if (args.length) return reject();
      return ['GET', '/matrix/storage/authority/'];
    case 'rooms.cutover.state':
      if (args.length) return reject();
      return ['GET', '/matrix/storage/migration/'];
    case 'rooms.cutover.begin':
      if (args.length) return reject();
      return ['POST', '/matrix/storage/migration/'];
    case 'rooms.cutover.complete':
      if (args.length) return reject();
      return ['POST', '/matrix/storage/migration/complete'];
    default:
      if (args.length) return reject();
      if (operation.startsWith('mail.ingress.'))
        return ['POST', '/private-storage/ingress/' + operation.slice('mail.ingress.'.length)];
      if (operation === 'mail.submission.send') return ['POST', '/private-storage/submissions'];
      if (operation === 'mail.submission.register')
        return ['POST', '/private-storage/submissions/register'];
      if (operation === 'mail.submission.control')
        return ['POST', '/private-storage/submissions/control'];
      return reject();
  }
}
export interface PrivateServiceWitness extends HostLease {
  /** Must reject a replaced Sytue actor/session as well as an untrusted SDK. */
  revalidate: (signal: AbortSignal) => Promise<void>;
  readonly fetch: typeof globalThis.fetch;
}
/** Routes are fixed same-origin BFF endpoints. API response bytes remain raw and
 * the existing Go clients retain every domain parser/control validation. */
export class PqcServicePort {
  private closed = false;
  private closing: Promise<void> | null = null;
  private readonly streams = new PqcResponseStreams(reject);
  private readonly uploads = new Set<PqcServiceSource>();
  private readonly uploadClosures = new WeakMap<PqcServiceSource, Promise<void>>();
  readonly handlers: Partial<Record<Exclude<HostMethod, 'current'>, HostHandler>>;
  constructor(
    scope: Readonly<HostScope>,
    witness: PrivateServiceWitness,
    runtime: Pick<PqcRuntime, 'invoke'>,
    portID: () => string,
    basePath: string,
    maximumMIMEBytes: number,
  ) {
    const bound = Object.freeze({ ...scope });
    if (
      !allowedProducts.has(bound.product) ||
      !Number.isSafeInteger(maximumMIMEBytes) ||
      maximumMIMEBytes <= 0
    )
      throw new Error('service_port_rejected');
    const origin = globalThis.location.origin,
      apiBase = withBase(basePath, '/api');
    const headers = Object.freeze({
      [storageScopeHeader]: encodeStorageScopeHeader({
        ...bound,
        product: bound.product as PrivateServiceProduct,
      }),
    });
    const revalidate = witness.revalidate.bind(witness),
      send = witness.fetch.bind(globalThis);
    const check = (context: HostCallContext) => {
      context.assertOpen();
      witness.assertOpen();
      witness.signal.throwIfAborted();
      if (
        this.closed ||
        context.scope.tenant !== bound.tenant ||
        context.scope.subject !== bound.subject ||
        context.scope.product !== bound.product
      )
        return reject();
    };
    const handle =
      (
        run: (
          raw: Uint8Array<ArrayBuffer>,
          context: HostCallContext,
          discard: (cleanup: () => Promise<void>) => void,
        ) => Promise<Uint8Array<ArrayBuffer>>,
      ): HostHandler =>
      async (raw, ctx) => {
        check(ctx);
        let output: Uint8Array<ArrayBuffer> | undefined;
        let cleanup: (() => Promise<void>) | undefined;
        try {
          output = await run(raw, ctx, (discard) => {
            cleanup = discard;
          });
          check(ctx);
          return output;
        } catch (error) {
          output?.fill(0);
          try {
            await cleanup?.();
          } catch (cleanupError) {
            throw new AggregateError([error, cleanupError], 'service_port_cleanup_failed');
          }
          throw error;
        }
      };
    this.handlers = {
      'service.request': handle(async (raw, ctx, discard) => {
        if (raw.byteLength < 4) return reject();
        const metadataLength = new DataView(raw.buffer, raw.byteOffset, 4).getUint32(0, false);
        if (metadataLength > 64 << 10 || metadataLength > raw.byteLength - 4) return reject();
        const input = decodeHostObject(raw.subarray(4, 4 + metadataLength), reject);
        if (
          typeof input.operation !== 'string' ||
          !operations.has(input.operation) ||
          (input.arguments !== undefined && !Array.isArray(input.arguments))
        )
          return reject();
        if (
          Object.keys(input).some(
            (key) =>
              !['operation', 'arguments', 'content_type', 'submission', 'source', 'size'].includes(
                key,
              ),
          )
        )
          return reject();
        const operation = input.operation,
          args = (input.arguments ?? []) as unknown[];
        if (args.some((value) => typeof value !== 'string')) return reject();
        const [method, path] = privateServiceRoute(
          operation,
          args as string[],
          bound.product as PrivateServiceProduct,
        );
        const controller = new AbortController(),
          abort = () => controller.abort();
        witness.signal.addEventListener('abort', abort, { once: true });
        const detach = () => witness.signal.removeEventListener('abort', abort);
        const signal = AbortSignal.any([ctx.signal, controller.signal]);
        let response: Response | undefined,
          upload: PqcServiceSource | undefined,
          buffered: { readonly bytes: Uint8Array<ArrayBuffer>; release: () => void } | undefined,
          requestStream = false,
          stream: string | undefined,
          output: Uint8Array<ArrayBuffer> | undefined;
        const errors: unknown[] = [];
        try {
          const outgoingHeaders = new Headers(headers);
          outgoingHeaders.set('Accept', 'application/json');
          let body: BodyInit | undefined;
          const rawBody = raw.subarray(4 + metadataLength);
          if (operation === 'mail.submission.send') {
            const source = id(input.source),
              port = id(portID()),
              size = Number(input.size);
            if (
              input.content_type !== 'message/rfc822' ||
              typeof input.submission !== 'string' ||
              !/^[A-Za-z0-9_-]{1,32768}$/u.test(input.submission) ||
              !Number.isSafeInteger(size) ||
              size <= 0 ||
              size > 2 ** 47 ||
              rawBody.length !== 0
            )
              return reject();
            outgoingHeaders.set('Content-Type', 'message/rfc822');
            outgoingHeaders.set('X-Sytue-Mail-Submission', input.submission);
            upload = new PqcServiceSource(
              runtime,
              port,
              bound,
              source,
              size,
              () => check(ctx),
              signal,
            );
            this.uploads.add(upload);
            requestStream = supportsRequestStreams();
            if (requestStream) body = upload.stream();
            else {
              buffered = await upload.buffer(maximumMIMEBytes);
              body = buffered.bytes;
            }
          } else {
            if (
              input.source !== undefined ||
              input.submission !== undefined ||
              !Number.isSafeInteger(input.size) ||
              Number(input.size) !== rawBody.byteLength ||
              rawBody.byteLength > 256 << 10 ||
              (input.content_type !== undefined &&
                input.content_type !== '' &&
                input.content_type !== 'application/json') ||
              (method === 'GET' && rawBody.byteLength !== 0)
            )
              return reject();
            if (input.content_type === 'application/json')
              outgoingHeaders.set('Content-Type', 'application/json');
            if (rawBody.byteLength) body = rawBody;
          }
          await revalidate(signal);
          check(ctx);
          signal.throwIfAborted();
          const url = new URL(apiBase + path, origin);
          if (url.origin !== origin) return reject();
          const init: RequestInit & { duplex?: 'half' } = {
            method,
            headers: outgoingHeaders,
            credentials: 'same-origin',
            redirect: 'error',
            cache: 'no-store',
            signal,
          };
          if (body !== undefined) init.body = body;
          if (requestStream) init.duplex = 'half';
          response = await send(url, init);
          if (upload) {
            const finishedUpload = upload;
            upload = undefined;
            await this.closeUpload(finishedUpload);
          }
          await revalidate(signal);
          check(ctx);
          signal.throwIfAborted();
          if (response.redirected || response.status < 200 || response.status > 599)
            return reject();
          const length = response.headers.get('Content-Length');
          const size = length === null ? -1 : Number(length);
          if (
            length !== null &&
            (!/^(0|[1-9][0-9]*)$/u.test(length) || !Number.isSafeInteger(size) || size > 2 ** 47)
          )
            return reject();
          const maximum = operation.startsWith('contacts.source.')
            ? 8 << 20
            : operation === 'mail.source.open' || operation === 'mail.ingress.body'
              ? 2 ** 47
              : 32 << 20;
          if (size > maximum) return reject();
          stream = this.streams.open(response, maximum, controller, detach);
          output = encodeHostJSON({
            status: response.status,
            stream,
            content_type: response.headers.get('Content-Type') ?? '',
            content_length: size,
            content_encoding: response.headers.get('Content-Encoding') ?? '',
            source_sha256: response.headers.get('X-Sytue-Source-SHA256') ?? '',
          });
        } catch (error) {
          errors.push(error);
        } finally {
          if (upload) {
            try {
              await this.closeUpload(upload);
            } catch (error) {
              errors.push(error);
            }
          }
          try {
            buffered?.release();
          } catch (error) {
            errors.push(error);
          }
        }
        if (errors.length) {
          output?.fill(0);
          if (stream !== undefined) {
            try {
              await this.streams.closeStream(stream);
            } catch (error) {
              errors.push(error);
            }
          } else {
            try {
              controller.abort();
            } catch (error) {
              errors.push(error);
            }
            try {
              detach();
            } catch (error) {
              errors.push(error);
            }
            const body = response?.body;
            if (body?.locked) errors.push(new Error('service_response_body_locked'));
            else {
              try {
                await body?.cancel();
              } catch (error) {
                errors.push(error);
              }
            }
          }
          if (errors.length === 1) throw errors[0];
          throw new AggregateError(errors, 'service_port_cleanup_failed');
        }
        if (!output || stream === undefined) return reject();
        const streamID = stream;
        discard(() => this.streams.closeStream(streamID));
        return output;
      }),
      'service.read': handle(async (raw, ctx) => {
        const input = decodeHostObject(raw, reject);
        if (!Number.isSafeInteger(input.offset) || !Number.isSafeInteger(input.length))
          return reject();
        return this.streams.read(
          id(input.stream),
          Number(input.offset),
          Number(input.length),
          ctx.signal,
        );
      }),
      'service.close': handle(async (raw) => {
        const input = decodeHostObject(raw, reject);
        await this.streams.closeStream(id(input.stream));
        return new Uint8Array();
      }),
    };
  }
  // The source also shares this close with cancellation by its request stream.
  // Keep the upload registered until its close has actually succeeded.
  private closeUpload(upload: PqcServiceSource): Promise<void> {
    let closing = this.uploadClosures.get(upload);
    if (!closing) {
      closing = Promise.resolve()
        .then(() => upload.close())
        .then(() => {
          this.uploads.delete(upload);
        });
      this.uploadClosures.set(upload, closing);
    }
    return closing;
  }

  async close(): Promise<void> {
    this.closed = true;
    if (!this.closing) {
      // This snapshot joins visible uploads and response streams. The host must
      // drain outstanding request handlers separately, including a late fetch.
      this.closing = Promise.resolve().then(async () => {
        const results = await Promise.allSettled([
          ...[...this.uploads].map((upload) => this.closeUpload(upload)),
          this.streams.close(),
        ]);
        const errors = results.filter(
          (result): result is PromiseRejectedResult => result.status === 'rejected',
        );
        if (errors.length)
          throw new AggregateError(
            errors.map((result): unknown => result.reason),
            'service_port_cleanup_failed',
          );
      });
    }
    await this.closing;
  }
}
