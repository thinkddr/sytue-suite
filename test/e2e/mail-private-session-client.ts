/* SPDX-License-Identifier: Apache-2.0 */
import {
  SuiteMatrix,
  type MatrixRecoveryUI,
  type MatrixSessionAdapter,
  type MailPrivatePorts,
} from '../../apps/frontend/packages/offline/src/matrix/client.js';
import { PrivateMailClient } from '../../apps/frontend/apps/mail/src/client/privateMail.js';
import { encodeHostJSON } from '../../apps/frontend/packages/offline/src/pqc/host-codec.js';
import type {
  MatrixBootstrap,
  MatrixSession,
  MatrixStorageScope,
} from '../../apps/frontend/packages/offline/src/matrix/types.js';
interface Fixture {
  artificial: true;
  native_migration_completed: true;
  bootstrap: MatrixBootstrap;
  session: MatrixSession;
  scope: MatrixStorageScope;
  recovery: string;
  known_id: string;
  known_mime: string;
  resume_expected_mime?: string;
  baseline_pending?: number;
  resume?: boolean;
}
interface ProofResult {
  message_id?: string;
  mime: string;
  paused: boolean;
  pending_verified: boolean;
  closed_and_retained: boolean;
  lost_ack?: boolean;
  ack_recovered?: boolean;
  baseline_pending: number;
  pending_count: number;
  timings: Record<string, number>;
}
interface Probe {
  ok: boolean;
  stage: string;
  reason?: string;
  failure_stage?: string;
  rpc_failures?: string[];
  host_failures?: string[];
  result?: ProofResult;
}
declare global {
  interface Window {
    mailPrivateProbe: Probe;
    mailPrivateCrashAfterAccepted?: () => void;
  }
}
const probe: Probe = { ok: false, stage: 'fixture' };
window.mailPrivateProbe = probe;
function check(value: unknown, message: string): asserts value {
  if (!value) throw new Error(message);
}
function equal(a: Uint8Array, b: Uint8Array): boolean {
  return a.byteLength === b.byteLength && a.every((value, index) => value === b[index]);
}
function fromBase64(value: string): Uint8Array<ArrayBuffer> {
  const raw = atob(value);
  return Uint8Array.from(raw, (char) => char.charCodeAt(0));
}
function base64(value: Uint8Array): string {
  let raw = '';
  for (let offset = 0; offset < value.length; offset += 8192)
    raw += String.fromCharCode(...value.subarray(offset, offset + 8192));
  return btoa(raw);
}
function tracePrivateFailure(field: 'rpc_failures' | 'host_failures', key: string): void {
  if (!/^[a-z0-9_.:-]{1,160}$/i.test(key)) return;
  const entries = (probe[field] ??= []);
  entries.push(key);
  if (entries.length > 8) entries.shift();
}
/** Test observer only: forwards exact calls/bytes to the real SDK and Go.
 * It never records arguments, replies, key material or exception text. */
function observePrivateTransport(suite: SuiteMatrix): void {
  const modules = new WeakSet<object>(), load = suite.clientModuleRuntime.bind(suite);
  suite.clientModuleRuntime = (module) => {
    const runtime = load(module);
    if (!modules.has(runtime)) {
      modules.add(runtime);
      const invoke = runtime.invoke.bind(runtime), register = runtime.registerHostPort.bind(runtime);
      runtime.invoke = async <T>(request: Record<string, unknown>, signal?: AbortSignal): Promise<T> => {
        try { return await invoke(request, signal) as T; }
        catch (error) {
          if (typeof request.operation === 'string') tracePrivateFailure('rpc_failures', module + ':' + request.operation);
          throw error;
        }
      };
      runtime.registerHostPort = (options) => {
        const handlers = {...options.handlers};
        for (const method of Object.keys(handlers) as Array<keyof typeof handlers>) {
          const handler = handlers[method];
          if (!handler) continue;
          handlers[method] = async (raw, context) => {
            try { return await handler(raw, context); }
            catch (error) { tracePrivateFailure('host_failures', module + ':' + method); throw error; }
          };
        }
        return register({...options, handlers});
      };
    }
    return runtime;
  };
}
async function control<T>(app: MailPrivatePorts, operation: string): Promise<T> {
  const input = encodeHostJSON({ session: app.session });
  try {
    return await app.runtime.invoke<T>(
      { operation, port: app.port, scope: app.scope, input },
      app.signal,
    );
  } finally {
    input.fill(0);
  }
}
export async function verifyMailPrivateSession(
  suite: SuiteMatrix,
  fixture: Fixture,
): Promise<ProofResult> {
  const timings: Record<string, number> = {};
  let started = performance.now();
  probe.stage = 'opening-original-Go-Mail';
  const app = await suite.openMailPrivate({ basePath: '/mail' });
  timings.open_ms = performance.now() - started;
  const client = new PrivateMailClient(app);
  let mime: Uint8Array<ArrayBuffer> | undefined;
  let closed = false;
  try {
    probe.stage = 'known-native-MIME';
    started = performance.now();
    const known = await client.raw(fixture.known_id),
      expected = fromBase64(fixture.known_mime);
    try {
      check(equal(known, expected), 'native-known-MIME-changed');
    } finally {
      client.releaseBody(known);
      expected.fill(0);
    }
    timings.known_raw_ms = performance.now() - started;
    const mailboxes = await client.call<unknown[]>('mailboxes');
    check(Array.isArray(mailboxes) && mailboxes.length > 0, 'mailboxes-not-original-Go');
    const aliases = await client.call<{ primary: string; aliases: string[] }>('aliases');
    check(
      typeof aliases.primary === 'string' && aliases.primary.length > 0,
      'missing-authenticated-alias',
    );
    if (!fixture.resume) {
      const baseline = await control<{ scope: MatrixStorageScope; count: number }>(
        app,
        'mail.pending',
      );
      check(Number.isSafeInteger(baseline.count) && baseline.count >= 0, 'unknown-initial-pending');
      probe.stage = 'compose-original-Go-MIME';
      started = performance.now();
      const composed = await client.compose({
        from: aliases.primary,
        to: ['recipient@example.invalid'],
        subject: 'Artificial browser retained revision',
        text: 'SYTUE_ARTIFICIAL_BROWSER_RETAINED',
        draft: true,
      });
      mime = composed.body;
      timings.compose_ms = performance.now() - started;
      const url = client.objectURL(mime, 'text/html');
      const unsafe = await fetch(url);
      check(
        unsafe.headers.get('content-type') === 'application/octet-stream',
        'active-attachment-mime-accepted',
      );
      await unsafe.body?.cancel();
      client.releaseURL(url);
      let revoked = false;
      try {
        await fetch(url);
      } catch {
        revoked = true;
      }
      check(revoked, 'released-object-URL-still-readable');
      const retainedURL = client.objectURL(mime, 'text/plain');
      probe.stage = 'prepare-WAL-without-publication';
      started = performance.now();
      const attempt = await client.call<Record<string, unknown>>(
        'draft.prepare',
        { editor: crypto.randomUUID(), revision: crypto.randomUUID(), from: composed.from },
        mime,
      );
      timings.prepare_ms = performance.now() - started;
      const original = base64(mime);
      probe.stage = 'pause-and-authenticated-pending';
      await control(app, 'mail.pause');
      const pending = await control<{ scope: MatrixStorageScope; count: number }>(
        app,
        'mail.pending',
      );
      check(pending.count > baseline.count, 'prepared-original-not-retained');
      let paused = false;
      try {
        await client.call('mailboxes');
      } catch {
        paused = true;
      }
      check(paused, 'paused-engine-admitted-operation');
      let eraseDenied = false;
      try {
        await suite.forget();
      } catch {
        eraseDenied = true;
      }
      check(
        eraseDenied && !suite.isClosed && !app.signal.aborted,
        'pending-original-was-destructively-forgotten',
      );
      // The runner will forward one exact encrypted request to the real
      // Synapse/Origin and lose its response before retiring this live Entry.
      let crashed = false,
        draining: Promise<void> | undefined;
      const crash = (): void => {
        if (crashed) return;
        crashed = true;
        probe.stage = 'retire-after-accepted-lost-ACK';
        draining = client.closeAndDrain();
        void draining.catch(() => {});
      };
      window.mailPrivateCrashAfterAccepted = crash;
      try {
        await control(app, 'mail.resume');
        const armed = await fetch('/test/arm-lost-ack', { method: 'POST' });
        check(armed.status === 204, 'lost-ACK-fixture-not-armed');
        probe.stage = 'commit-with-interrupted-ACK';
        let rejected = false;
        try {
          await client.call('draft.commit', { attempt });
        } catch {
          rejected = true;
        }
        check(crashed && rejected && draining, 'accepted-write-did-not-lose-its-ACK');
        await draining;
        closed = true;
      } finally {
        if (window.mailPrivateCrashAfterAccepted === crash)
          delete window.mailPrivateCrashAfterAccepted;
      }
      check(
        mime.every((value) => value === 0),
        'owned-MIME-retained-after-close',
      );
      revoked = false;
      try {
        await fetch(retainedURL);
      } catch {
        revoked = true;
      }
      check(revoked, 'account-object-URL-retained-after-close');
      return {
        mime: original,
        paused: true,
        pending_verified: true,
        closed_and_retained: true,
        baseline_pending: baseline.count,
        pending_count: pending.count,
        lost_ack: true,
        timings,
      };
    }
    probe.stage = 'recover-original-draft-attempt';
    const pending = await client.call<Record<string, unknown>[]>('draft.pending');
    check(Array.isArray(pending) && pending.length === 1, 'reload-did-not-recover-exact-attempt');
    started = performance.now();
    const message = await client.call<{ id: string }>('draft.commit', { attempt: pending[0] });
    timings.commit_ms = performance.now() - started;
    check(typeof message.id === 'string' && message.id.length > 0, 'draft-commit-lost-identity');
    probe.stage = 'readback-recovered-exact-MIME';
    started = performance.now();
    mime = await client.raw(message.id);
    check(typeof fixture.resume_expected_mime === 'string', 'missing-test-only-original');
    const expectedReload = fromBase64(fixture.resume_expected_mime);
    try {
      check(equal(mime, expectedReload), 'reload-committed-replacement-MIME');
    } finally {
      expectedReload.fill(0);
    }
    timings.recovered_raw_ms = performance.now() - started;
    const original = base64(mime);
    await control(app, 'mail.pause');
    const count = await control<{ scope: MatrixStorageScope; count: number }>(app, 'mail.pending');
    check(
      Number.isSafeInteger(count.count) && count.count <= (fixture.baseline_pending ?? 0),
      'committed-draft-retained-its-WAL',
    );
    await client.closeAndDrain();
    closed = true;
    check(
      mime.every((value) => value === 0),
      'read-MIME-not-erased-with-owner',
    );
    return {
      message_id: message.id,
      mime: original,
      paused: true,
      pending_verified: true,
      closed_and_retained: true,
      baseline_pending: fixture.baseline_pending ?? 0,
      pending_count: count.count,
      ack_recovered: true,
      timings,
    };
  } finally {
    if (!closed) await client.closeAndDrain();
  }
}
void (async () => {
  const fixture = (await (await fetch('/fixture')).json()) as Fixture;
  check(
    fixture.artificial === true && fixture.native_migration_completed === true,
    'disposable-native-fixture-required',
  );
  let prompts = 0;
  const ui: MatrixRecoveryUI = {
    confirmStorageRoot: async () => {},
    saveRecoveryKey: async () => {
      throw new Error('existing-key-was-reset');
    },
    readRecoveryKey: async () => {
      prompts++;
      if (fixture.resume) throw new Error('reload-requested-recovery-key');
      return fixture.recovery;
    },
    authenticate: async () => {
      throw new Error('unexpected-existing-identity-reset');
    },
  };
  const adapter: MatrixSessionAdapter = {
    readSession: async () => fixture.session,
    forgetSession: async () => {
      throw new Error('retained-work-cannot-erase-session');
    },
    refreshBootstrap: async () => fixture.bootstrap,
  };
  probe.stage = 'open-existing-SDK';
  const suite = await SuiteMatrix.open(fixture.bootstrap, ui, adapter);
  observePrivateTransport(suite);
  try {
    if (!(await suite.trusted())) {
      probe.stage = 'recover-existing-SSSS';
      await suite.recover();
    }
    check(await suite.trusted(), 'SDK-not-trusted');
    const result = await verifyMailPrivateSession(suite, fixture);
    check(!fixture.resume || prompts === 0, 'update-reload-lost-persistent-session');
    probe.stage = 'complete';
    probe.result = result;
    probe.ok = true;
  } finally {
    await suite.closeAndDrain();
  }
})().catch((error) => {
  probe.failure_stage = probe.stage;
  probe.stage = 'failed';
  probe.reason = error instanceof Error ? error.message : 'private-session-failed';
});
