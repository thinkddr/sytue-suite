/* SPDX-License-Identifier: Apache-2.0 */
import { decodeHostObject } from './host-codec.js';
import { hasExpectedResponseLengthHeaders, ResponseLengthError } from '@sytue/api-clients';
import { storageIdentifier } from '../matrix/storage.js';
import type {
  HostCallContext,
  HostHandler,
  HostMethod,
  HostScope,
  HostLease,
} from './host-port.js';
import { PqcResponseStreams } from './response-streams.js';

export interface SourceDescriptor {
  readonly kind: string;
  readonly id: string;
  readonly size: number;
  readonly sha256: string;
}
/** Product adapters own only fixed authenticated routes. No URL or credential
 * crosses this interface or the worker boundary. State/page bytes stay raw. */
export interface PrivateSourceTransport {
  state: (signal: AbortSignal) => Promise<Uint8Array<ArrayBuffer>>;
  begin: (
    epoch: string,
    provision: boolean,
    signal: AbortSignal,
  ) => Promise<Uint8Array<ArrayBuffer>>;
  page: (epoch: string, after: string, signal: AbortSignal) => Promise<Uint8Array<ArrayBuffer>>;
  open: (epoch: string, row: SourceDescriptor, signal: AbortSignal) => Promise<Response>;
}
const encoder = new TextEncoder();
const empty = () => new Uint8Array();
function reject(): never {
  throw new Error('source_port_rejected');
}

function epoch(value: unknown): string {
  if (
    typeof value !== 'string' ||
    !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/u.test(value)
  )
    return reject();
  return value;
}
function row(value: unknown): Readonly<SourceDescriptor> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return reject();
  const input = value as Record<string, unknown>;
  if (
    !storageIdentifier(input.kind) ||
    typeof input.id !== 'string' ||
    !input.id ||
    encoder.encode(input.id).length > 2048 ||
    /[\0\r\n\t]/u.test(input.id) ||
    !Number.isSafeInteger(input.size) ||
    Number(input.size) < 0 ||
    Number(input.size) > 2 ** 47 ||
    typeof input.sha256 !== 'string' ||
    !/^[0-9a-f]{64}$/u.test(input.sha256)
  )
    return reject();
  return Object.freeze({
    kind: input.kind,
    id: input.id,
    size: Number(input.size),
    sha256: input.sha256,
  });
}
/** One reader per immutable original. Reads are sequential and at most256KiB;
 * closing cancels and joins actual IO before dropping retained bytes. */
export class PqcSourcePort {
  private closed = false;
  private readonly streams = new PqcResponseStreams(reject);
  readonly handlers: Partial<Record<Exclude<HostMethod, 'current'>, HostHandler>>;
  constructor(scope: Readonly<HostScope>, lease: HostLease, transport: PrivateSourceTransport) {
    const bound = Object.freeze({ ...scope });
    const checked = (context: HostCallContext): void => {
      context.assertOpen();
      lease.assertOpen();
      lease.signal.throwIfAborted();
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
      async (raw, context) => {
        checked(context);
        let output: Uint8Array<ArrayBuffer> | undefined;
        let cleanup: (() => Promise<void>) | undefined;
        try {
          output = await run(raw, context, (discard) => {
            cleanup = discard;
          });
          checked(context);
          return output;
        } catch (error) {
          output?.fill(0);
          try {
            await cleanup?.();
          } catch (cleanupError) {
            throw new AggregateError([error, cleanupError], 'source_port_cleanup_failed');
          }
          throw error;
        }
      };
    // Bind once. A later mutation cannot swap the account/source under a job.
    const state = transport.state.bind(transport),
      begin = transport.begin.bind(transport),
      page = transport.page.bind(transport),
      open = transport.open.bind(transport);
    this.handlers = {
      'source.capabilities': handle(async (_raw, ctx) => state(ctx.signal)),
      'source.begin': handle(async (raw, ctx) => {
        const value = decodeHostObject(raw, reject);
        if (typeof value.provision !== 'boolean' || (value.provision && bound.product !== 'mail'))
          return reject();
        return begin(epoch(value.epoch), value.provision, ctx.signal);
      }),
      'source.page': handle(async (raw, ctx) => {
        const value = decodeHostObject(raw, reject);
        if (
          typeof value.after !== 'string' ||
          value.after.length > 8192 ||
          (value.after !== '' && !/^[A-Za-z0-9_-]+$/u.test(value.after))
        )
          return reject();
        return page(epoch(value.epoch), value.after, ctx.signal);
      }),
      'source.open': handle(async (raw, ctx, discard) => {
        const value = decodeHostObject(raw, reject),
          descriptor = row(value.row),
          controller = new AbortController();
        const abort = () => controller.abort();
        lease.signal.addEventListener('abort', abort, { once: true });
        const detach = () => lease.signal.removeEventListener('abort', abort);
        // Explicitly undefined: `discard` may run before `open` settles and reads both.
        let response: Response | undefined = undefined;
        let stream: string | undefined = undefined;
        discard(async () => {
          if (stream !== undefined) {
            await this.streams.closeStream(stream);
            return;
          }
          const errors: unknown[] = [];
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
          if (body) {
            if (body.locked) errors.push(new Error('source_response_body_locked'));
            else {
              try {
                await body.cancel();
              } catch (error) {
                errors.push(error);
              }
            }
          }
          if (errors.length) throw new AggregateError(errors, 'source_port_cleanup_failed');
        });
        if (lease.signal.aborted) abort();
        response = await open(
          epoch(value.epoch),
          descriptor,
          AbortSignal.any([ctx.signal, controller.signal]),
        );
        checked(ctx);
        if (
          !response.ok ||
          response.status !== 200 ||
          response.headers.get('content-type') !== 'application/octet-stream' ||
          response.redirected
        )
          return reject();
        if (!hasExpectedResponseLengthHeaders(response, descriptor.size))
          throw new ResponseLengthError();
        stream = this.streams.open(response, descriptor.size, controller, detach);
        return encoder.encode(JSON.stringify({ stream, row: descriptor }));
      }),
      'source.read': handle(async (raw, ctx) => {
        const value = decodeHostObject(raw, reject);
        if (
          typeof value.stream !== 'string' ||
          !/^[0-9a-f]{64}$/u.test(value.stream) ||
          !Number.isSafeInteger(value.offset) ||
          !Number.isSafeInteger(value.length)
        )
          return reject();
        const output = await this.streams.read(
          value.stream,
          Number(value.offset),
          Number(value.length),
          ctx.signal,
        );
        if (output.byteLength !== Number(value.length)) {
          output.fill(0);
          return reject();
        }
        return output;
      }),
      'source.close': handle(async (raw) => {
        const value = decodeHostObject(raw, reject);
        if (typeof value.stream !== 'string' || !/^[0-9a-f]{64}$/u.test(value.stream))
          return reject();
        await this.streams.closeStream(value.stream);
        return empty();
      }),
    };
  }
  async close(): Promise<void> {
    this.closed = true;
    await this.streams.close();
  }
}
