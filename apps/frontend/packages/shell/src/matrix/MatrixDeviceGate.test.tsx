/* SPDX-License-Identifier: Apache-2.0 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { CryptoEvent } from 'matrix-js-sdk/lib/crypto-api';
import {
  MatrixCustodyError,
  MatrixSecurityError,
  type MatrixBootstrap,
  type MatrixKeyCustody,
  type MatrixRecoveryUI,
  type MatrixUnlockMethod,
  type SuiteMatrix,
} from '@sytue/offline/matrix';
import { ToastProvider, useToast } from '@sytue/ui';
import type { StorageRootService } from '@sytue/config/matrix';
import { MatrixDeviceGate, type MatrixGateHost } from './MatrixDeviceGate.js';
import { MatrixSourceGate, type MatrixSourceGateHost } from './MatrixSourceGate.js';

if (typeof document === 'undefined') GlobalRegistrator.register();
const actor = '11111111-1111-4111-8111-111111111111';
const otherActor = '22222222-2222-4222-8222-222222222222';
function bootstrap(subject = actor, tenant = 'synthetic-org'): MatrixBootstrap {
  return {
    actor: subject,
    tenant,
    homeserver_url: 'https://matrix.example',
    matrix_user_id: '@sytue_' + subject + ':matrix.example',
    controller_user_id: '@controller:matrix.example',
    sso_provider_id: 'sytue',
    encryption: 'm.megolm.v1.aes-sha2',
    channels: [],
    storage_product: 'drive',
    storage_room_id: '!private:matrix.example',
    storage_room_ids: ['!private:matrix.example'],
  };
}
function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}
async function until(check: () => boolean) {
  const end = Date.now() + 4000;
  while (!check()) {
    if (Date.now() > end) throw new Error('UI did not reach expected state');
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 10));
    });
  }
}
function findButton(text: string): HTMLButtonElement | undefined {
  return Array.from(document.querySelectorAll('button')).find(
    (node) => node.textContent?.trim() === text,
  );
}
function button(text: string): HTMLButtonElement {
  const element = findButton(text);
  if (!element) throw new Error('missing button ' + text);
  return element;
}
function PrivateNotice() {
  const { notify } = useToast();
  return (
    <button
      data-private-notice
      onClick={() => notify({ title: 'synthetic private notice', duration: 0 })}
    >
      Show private notice
    </button>
  );
}
function GlobalNotice() {
  const { notify } = useToast();
  return (
    <button
      data-global-notice
      onClick={() => notify({ title: 'synthetic global notice', duration: 0 })}
    >
      Show global notice
    </button>
  );
}
/** The person's Matrix account as the homeserver sees it. */
interface Account {
  hasKey: boolean;
  keyID: string | null;
  key: string | null;
}
/** Synthetic service custody: wrappers by kind, the factor kept beside the key. */
function custodyFixture(account: Account) {
  const wrappers = new Map<MatrixUnlockMethod, { keyID: string; key: string; password?: string }>();
  const log: string[] = [];
  const control = { failWraps: 0, legacy: null as string | null };
  const custody: MatrixKeyCustody = {
    state: async (keyIDs) => ({
      methods: [...wrappers.entries()]
        .filter(([, w]) => !keyIDs || keyIDs.includes(w.keyID))
        .map(([kind]) => kind),
      legacy: control.legacy !== null,
    }),
    unwrap: async (factor, target) => {
      const wrapper = wrappers.get(factor.kind);
      if (!wrapper || (target.keyIDs && !target.keyIDs.includes(wrapper.keyID)))
        throw new MatrixCustodyError('wrong');
      if (factor.kind === 'password' && factor.password !== wrapper.password)
        throw new MatrixCustodyError('wrong');
      log.push(`unwrap ${factor.kind}`);
      return wrapper.key;
    },
    wrap: async (key, factor, target) => {
      if (control.failWraps > 0) {
        control.failWraps--;
        log.push('wrap failed');
        throw new MatrixCustodyError('unavailable');
      }
      // The service drops wrappers of any other key ID, and the retired copy.
      for (const [kind, w] of wrappers) if (w.keyID !== target.keyID) wrappers.delete(kind);
      wrappers.set(factor.kind, {
        keyID: target.keyID,
        key,
        ...(factor.kind === 'password' ? { password: factor.password } : {}),
      });
      control.legacy = null;
      log.push(`wrap ${factor.kind} ${target.keyID} hasKey=${account.hasKey}`);
    },
    legacyRecoveryKey: async () => control.legacy,
    passkeySupported: () => true,
  };
  return { custody, wrappers, log, control };
}
function fixture() {
  const root = document.body.appendChild(document.createElement('div'));
  const devices: ReturnType<typeof device>[] = [];
  let data = bootstrap();
  let initialTrust = true;
  // By default an account whose key is wrapped with a password.
  const account: Account = { hasKey: true, keyID: 'KEY1', key: 'synthetic-recovery-key' };
  const vault = custodyFixture(account);
  vault.wrappers.set('password', {
    keyID: 'KEY1',
    key: 'synthetic-recovery-key',
    password: 'synthetic password',
  });
  // Synthetic lifecycle port only: these tests do not claim to verify cryptography.
  function device(value: MatrixBootstrap, ui: MatrixRecoveryUI) {
    const listeners = new Map<string, (...args: unknown[]) => void>();
    const state = {
      verified: initialTrust,
      closed: false,
      peers: 0,
      recovered: false,
      resets: 0,
      closeCalls: 0,
      rootApprovals: 0,
      sdkDrain: undefined as Promise<void> | undefined,
    };
    const instance = {
      bootstrap: value,
      get isClosed() {
        return state.closed;
      },
      client: {
        on: (name: string, fn: (...args: unknown[]) => void) => listeners.set(name, fn),
        secretStorage: {
          hasKey: async () => account.hasKey,
          getDefaultKeyId: async () => account.keyID,
        },
      },
      trusted: async () => state.verified,
      ensureStorageAuthority: async () => {
        if (!ui.confirmStorageRoot) throw new MatrixSecurityError('unsupported');
        await ui.confirmStorageRoot({
          v: 1,
          scope: { tenant: value.tenant, subject: value.actor, product: 'drive' },
          user_id: value.matrix_user_id,
          target: { room_id: value.storage_room_id!, device_id: 'DEVICE' },
          root_key: 'A'.repeat(43),
          generation: 1,
        });
        if (state.closed) throw new MatrixSecurityError('device');
        state.rootApprovals++;
      },
      devices: async () => [
        { id: 'DEVICE', name: 'Synthetic device', current: true, verified: state.verified },
      ],
      unverifiedRecipients: async () => {
        state.peers++;
        return [{ user: '@foreign:matrix.example', device: 'OTHER', name: 'Foreign peer' }];
      },
      canVerifyUser: () => true,
      // Mirrors SuiteMatrix: the new key goes to the host BEFORE it is published.
      setupRecovery: async () => {
        await ui.saveRecoveryKey('synthetic-created-key', 'KEY-NEW');
        Object.assign(account, { hasKey: true, keyID: 'KEY-NEW', key: 'synthetic-created-key' });
        vault.log.push('publish KEY-NEW');
        state.verified = true;
      },
      recover: async () => {
        const key = await ui.readRecoveryKey(account.keyID ? [account.keyID] : undefined);
        if (key !== account.key) throw new MatrixSecurityError('recovery');
        state.recovered = true;
        state.verified = true;
      },
      // Mirrors SuiteMatrix.resetRecovery: the new key is handed over first, then the
      // homeserver's re-authentication replaces the cross-signing keys.
      resetRecovery: async () => {
        if (value.storage_product) throw new MatrixSecurityError('unsupported');
        await ui.saveRecoveryKey('synthetic-reset-key', 'KEY-RESET');
        await ui.authenticate(value.homeserver_url + '/_matrix/client/v3/auth/m.login.sso');
        state.resets++;
        Object.assign(account, { hasKey: true, keyID: 'KEY-RESET', key: 'synthetic-reset-key' });
        vault.log.push('publish KEY-RESET');
        state.verified = true;
      },
      close: () => {
        state.closeCalls++;
        state.closed = true;
        ui.cancelPending?.();
      },
      closeAndDrain: async () => {
        instance.close();
        await state.sdkDrain;
      },
      forget: async () => {
        instance.close();
      },
    };
    return { instance: instance as unknown as SuiteMatrix, state, listeners, ui };
  }
  const host: MatrixGateHost = {
    translate: (key) => key,
    bootstrap: async () => data,
    begin: async () => {},
    custody: vault.custody,
    session: {
      readSession: async () => null,
      forgetSession: async () => {},
      refreshBootstrap: async () => data,
    },
    open: async (value, ui) => {
      const entry = device(value, ui);
      devices.push(entry);
      return entry.instance;
    },
  };
  const mount = async (subject = data.actor, tenant = data.tenant) => {
    await act(() => {
      render(
        <ToastProvider>
          <GlobalNotice />
          <MatrixDeviceGate actor={subject} tenant={tenant} locale="en" host={host}>
            {(client) => (
              <>
                <p data-private="true">{client.bootstrap.actor}</p>
                <PrivateNotice />
              </>
            )}
          </MatrixDeviceGate>
        </ToastProvider>,
        root,
      );
    });
  };
  const unmount = async () => {
    await act(() => {
      render(null, root);
    });
    root.remove();
  };
  return {
    root,
    devices,
    host,
    mount,
    unmount,
    setData: (value: MatrixBootstrap) => {
      data = value;
    },
    untrusted: () => {
      initialTrust = false;
    },
    account,
    vault,
  };
}
/** The recovery key itself never reaches the page, whatever the flow. */
function expectNoKeyRendered() {
  expect(document.body.innerHTML).not.toMatch(/synthetic-(recovery|created|reset)-key/);
  expect(document.querySelector('code')).toBeNull();
}
async function typePassword(value: string, confirmation?: string) {
  const fields = Array.from(
    document.querySelectorAll<HTMLInputElement>('[role=dialog] input[type=password]'),
  );
  const values = confirmation === undefined ? [value] : [value, confirmation];
  expect(fields).toHaveLength(values.length);
  await act(() => {
    fields.forEach((field, index) => {
      field.value = values[index]!;
      field.dispatchEvent(new Event('input', { bubbles: true }));
    });
  });
  await act(() => button('matrix.usePassword').click());
}

test('verified device waits for private application import before mounting children', async () => {
  const f = fixture(),
    gate = deferred();
  let imports = 0;
  f.host.prepare = async () => {
    imports++;
    await gate.promise;
  };
  try {
    await f.mount();
    await until(() => imports === 1);
    expect(f.root.querySelector('[data-private]')).toBeNull();
    gate.resolve();
    await until(() => f.root.querySelector('[data-private]') !== null);
    expect(f.root.querySelector('[data-private]')?.textContent).toBe(actor);
  } finally {
    gate.resolve();
    await f.unmount();
  }
});

test('changing account hides old data and waits for its jobs before opening the next device', async () => {
  const f = fixture(),
    firstImport = deferred(),
    drained = deferred();
  let detached = 0,
    closed = 0,
    firstSignal: AbortSignal | undefined;
  f.host.prepare = async (client, signal) => {
    if (client.bootstrap.actor === actor) {
      firstSignal = signal;
      await firstImport.promise;
    }
  };
  f.host.detach = () => {
    detached++;
  };
  f.host.close = async (client) => {
    if (client.bootstrap.actor === actor) {
      closed++;
      await drained.promise;
    }
  };
  try {
    await f.mount();
    await until(() => firstSignal !== undefined);
    f.setData(bootstrap(otherActor));
    await f.mount();
    expect(firstSignal?.aborted).toBe(true);
    expect(detached).toBe(1);
    expect(closed).toBe(1);
    expect(f.root.querySelector('[data-private]')).toBeNull();
    expect(f.devices).toHaveLength(1);
    firstImport.resolve();
    await act(async () => {
      await new Promise((done) => setTimeout(done, 50));
    });
    expect(f.devices).toHaveLength(1);
    drained.resolve();
    await until(() => f.root.querySelector('[data-private]') !== null);
    expect(f.root.querySelector('[data-private]')?.textContent).toBe(otherActor);
    expect(f.devices[0]!.state.closed).toBe(true);
  } finally {
    firstImport.resolve();
    drained.resolve();
    await f.unmount();
  }
});

test('a verified device missing its private owner key reopens it with the password, never the key', async () => {
  const f = fixture();
  let attempts = 0;
  // Like the owner key cache: the first time it is unavailable, then it asks for the key.
  f.host.prepare = async () => {
    if (++attempts === 1) throw new MatrixSecurityError('recovery');
    if ((await f.devices[0]!.ui.readRecoveryKey(['KEY1'])) !== 'synthetic-recovery-key')
      throw new MatrixSecurityError('recovery');
  };
  try {
    await f.mount();
    await until(() => findButton('action.retry')?.disabled === false);
    expect(f.root.querySelector('[data-private]')).toBeNull();
    await act(() => button('action.retry').click());
    await until(() => findButton('matrix.usePassword') !== undefined);
    expect(document.querySelector('[role=dialog]')?.textContent).toContain('matrix.unlockBody');
    await typePassword('synthetic password');
    await until(() => f.root.querySelector('[data-private]') !== null);
    expect(attempts).toBe(2);
    expectNoKeyRendered();
  } finally {
    await f.unmount();
  }
});

test('an account without encryption gets a key wrapped BEFORE it is published, and nothing to save', async () => {
  const f = fixture();
  f.untrusted();
  Object.assign(f.account, { hasKey: false, keyID: null, key: null });
  f.vault.wrappers.clear();
  try {
    await f.mount();
    // No button to press: the gate asks how to protect it straight away.
    await until(() => findButton('matrix.usePassword') !== undefined);
    expect(document.querySelector('[role=dialog]')?.textContent).toContain('matrix.protectBody');
    expect(findButton('matrix.usePasskey')).toBeDefined();
    expectNoKeyRendered();
    expect(document.querySelector('input[type=checkbox]')).toBeNull();
    // Two fields that must match: a typo here would lock the data out elsewhere.
    await typePassword('synthetic password', 'synthetic passwort');
    await until(() => document.body.textContent?.includes('matrix.passwordMismatch') === true);
    expect(f.vault.log).toEqual([]);
    await typePassword('synthetic password', 'synthetic password');
    await until(() => f.root.querySelector('[data-private]') !== null);
    expect(f.vault.log).toEqual(['wrap password KEY-NEW hasKey=false', 'publish KEY-NEW']);
    expectNoKeyRendered();
  } finally {
    await f.unmount();
  }
});

test('a failed upload of the wrapped key publishes nothing and can be retried in place', async () => {
  const f = fixture();
  f.untrusted();
  Object.assign(f.account, { hasKey: false, keyID: null, key: null });
  f.vault.wrappers.clear();
  f.vault.control.failWraps = 1;
  try {
    await f.mount();
    await until(() => findButton('matrix.usePasskey') !== undefined);
    await act(() => button('matrix.usePasskey').click());
    await until(() => document.body.textContent?.includes('matrix.failed') === true);
    // Still locked, the account untouched, and no key to fall back on.
    expect(f.account.hasKey).toBe(false);
    expect(f.root.querySelector('[data-private]')).toBeNull();
    expectNoKeyRendered();
    await act(() => button('matrix.usePasskey').click());
    await until(() => f.root.querySelector('[data-private]') !== null);
    expect(f.vault.log).toEqual([
      'wrap failed',
      'wrap passkey KEY-NEW hasKey=false',
      'publish KEY-NEW',
    ]);
  } finally {
    await f.unmount();
  }
});

test('closing the protection dialog creates nothing and leaves a retry', async () => {
  const f = fixture();
  f.untrusted();
  Object.assign(f.account, { hasKey: false, keyID: null, key: null });
  f.vault.wrappers.clear();
  try {
    await f.mount();
    await until(() => findButton('matrix.usePassword') !== undefined);
    await act(() => {
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
    });
    await until(() => findButton('action.retry')?.disabled === false);
    expect(f.account.hasKey).toBe(false);
    expect(document.body.textContent?.includes('matrix.unknownKey')).toBe(false);
    await act(() => button('action.retry').click());
    await until(() => findButton('matrix.usePassword') !== undefined);
  } finally {
    await f.unmount();
  }
});

test('a new device opens a wrapped key with the password; a wrong one just asks again', async () => {
  const f = fixture();
  f.untrusted();
  try {
    await f.mount();
    await until(() => findButton('matrix.usePassword') !== undefined);
    // Only the methods that have a wrapper are offered.
    expect(findButton('matrix.usePasskey')).toBeUndefined();
    await typePassword('not it');
    await until(() => document.body.textContent?.includes('matrix.unlockWrong') === true);
    expect(f.root.querySelector('[data-private]')).toBeNull();
    await typePassword('not it either');
    await until(() => !document.querySelector('[role=dialog] p[role=status]'));
    expect(f.vault.log).toEqual([]);
    await typePassword('synthetic password');
    await until(() => f.root.querySelector('[data-private]') !== null);
    expect(f.vault.log).toEqual(['unwrap password']);
    expectNoKeyRendered();
  } finally {
    await f.unmount();
  }
});

test('a new device opens a passkey-wrapped key with one tap', async () => {
  const f = fixture();
  f.untrusted();
  f.vault.wrappers.clear();
  f.vault.wrappers.set('passkey', { keyID: 'KEY1', key: 'synthetic-recovery-key' });
  try {
    await f.mount();
    await until(() => findButton('matrix.usePasskey') !== undefined);
    expect(findButton('matrix.usePassword')).toBeUndefined();
    await act(() => button('matrix.usePasskey').click());
    await until(() => f.root.querySelector('[data-private]') !== null);
    expect(f.vault.log).toEqual(['unwrap passkey']);
  } finally {
    await f.unmount();
  }
});

test('a key the service does not hold offers only verification from another device', async () => {
  const f = fixture();
  f.untrusted();
  f.vault.wrappers.clear();
  try {
    await f.mount();
    await until(() => document.body.textContent?.includes('matrix.unknownKey') === true);
    expect(findButton('matrix.verify')).toBeDefined();
    // Private storage: no reset (it would orphan the vault), no key prompt.
    expect(findButton('matrix.reset')).toBeUndefined();
    expect(document.querySelector('input[type=password]')).toBeNull();
    expect(document.querySelector('[role=dialog]')).toBeNull();
    expect(f.vault.log).toEqual([]);
  } finally {
    await f.unmount();
  }
});

test('the key the service used to hold opens Rooms silently and is wrapped once', async () => {
  const f = fixture();
  f.untrusted();
  f.setData(roomsBootstrap());
  f.vault.wrappers.clear();
  f.vault.control.legacy = 'synthetic-recovery-key';
  try {
    await f.mount();
    // Recovered without asking anything; then protected, which retires the old copy.
    await until(() => findButton('matrix.usePassword') !== undefined);
    expect(document.querySelector('[role=dialog]')?.textContent).toContain('matrix.protectBody');
    expect(f.devices[0]!.state.recovered).toBe(true);
    await typePassword('synthetic password', 'synthetic password');
    await until(() => f.root.querySelector('[data-private]') !== null);
    expect(f.vault.log).toEqual(['wrap password KEY1 hasKey=true']);
    expect(f.vault.control.legacy).toBeNull();
    expectNoKeyRendered();
  } finally {
    await f.unmount();
  }
});

test('private products neither enumerate peers nor accept verification from another person', async () => {
  const f = fixture();
  try {
    await f.mount();
    await until(() => f.root.querySelector('[data-private]') !== null);
    expect(f.devices[0]!.state.peers).toBe(0);
    let cancelled = 0;
    f.devices[0]!.listeners.get(CryptoEvent.VerificationRequestReceived)?.({
      otherUserId: '@foreign:matrix.example',
      cancel: async () => {
        cancelled++;
      },
    });
    await until(() => cancelled === 1);
    expect(document.querySelector('[role=dialog]')).toBeNull();
  } finally {
    await f.unmount();
  }
});

test('changing tenant masks an old recovery prompt while application cleanup is still pending', async () => {
  const f = fixture(),
    drain = deferred();
  f.untrusted();
  f.host.close = async () => {
    await drain.promise;
  };
  try {
    await f.mount();
    await until(() => f.devices.length === 1 && findButton('matrix.usePassword') !== undefined);
    f.setData(bootstrap(actor, 'next-org'));
    await f.mount();
    expect(findButton('matrix.usePassword')).toBeUndefined();
    expect(document.querySelector('[role=dialog]')).toBeNull();
    drain.resolve();
    await until(() => f.devices.length === 2);
  } finally {
    drain.resolve();
    await f.unmount();
  }
});

test('a transient application failure retries import without requesting a new recovery key', async () => {
  const f = fixture();
  let attempts = 0;
  f.host.prepare = async () => {
    if (++attempts === 1) throw new Error('synthetic transient source');
  };
  try {
    await f.mount();
    await until(() => findButton('action.retry')?.disabled === false);
    expect(f.root.querySelector('[data-private]')).toBeNull();
    await act(() => button('action.retry').click());
    await until(() => f.root.querySelector('[data-private]') !== null);
    expect(attempts).toBe(2);
    expect(document.querySelector('input[type=password]')).toBeNull();
  } finally {
    await f.unmount();
  }
});

test('an SDK initiated close hides private content and preserves the application drain barrier', async () => {
  const f = fixture(),
    drained = deferred();
  let detached = 0;
  f.host.detach = () => {
    detached++;
  };
  f.host.close = async (client) => {
    if (client.bootstrap.actor === actor) await drained.promise;
  };
  try {
    await f.mount();
    await until(() => f.root.querySelector('[data-private]') !== null);
    await act(() => f.devices[0]!.instance.close());
    await until(() => f.root.querySelector('[data-private]') === null);
    expect(detached).toBe(1);
    f.setData(bootstrap(otherActor));
    await f.mount();
    await act(async () => {
      await new Promise((done) => setTimeout(done, 50));
    });
    expect(f.devices).toHaveLength(1);
    drained.resolve();
    await until(() => f.root.querySelector('[data-private]') !== null);
    expect(f.root.querySelector('[data-private]')?.textContent).toBe(otherActor);
  } finally {
    drained.resolve();
    await f.unmount();
  }
});

test('private notices disappear with the SDK owned presenter', async () => {
  const f = fixture();
  try {
    await f.mount();
    await until(() => f.root.querySelector('[data-private-notice]') !== null);
    await act(() => f.root.querySelector<HTMLButtonElement>('[data-global-notice]')!.click());
    await act(() => f.root.querySelector<HTMLButtonElement>('[data-private-notice]')!.click());
    await until(() => document.body.textContent?.includes('synthetic private notice') === true);
    await act(() => f.devices[0]!.instance.close());
    await until(() => f.root.querySelector('[data-private]') === null);
    expect(document.body.textContent?.includes('synthetic private notice')).toBe(false);
    expect(document.body.textContent?.includes('synthetic global notice')).toBe(true);
  } finally {
    await f.unmount();
  }
});

test('root consent keeps application data hidden until the person allows the verified device', async () => {
  const f = fixture();
  f.host.authority = {} as StorageRootService;
  try {
    await f.mount();
    await until(() => findButton('matrix.allowStorageAction') !== undefined);
    expect(f.root.querySelector('[data-private]')).toBeNull();
    expect(f.devices[0]!.state.rootApprovals).toBe(0);
    await act(() => button('matrix.allowStorageAction').click());
    await until(() => f.root.querySelector('[data-private]') !== null);
    expect(f.devices[0]!.state.rootApprovals).toBe(1);
  } finally {
    await f.unmount();
  }
});

test('changing account cancels the old root consent before opening the next prompt', async () => {
  const f = fixture();
  f.host.authority = {} as StorageRootService;
  try {
    await f.mount();
    await until(() => findButton('matrix.allowStorageAction') !== undefined);
    const first = f.devices[0]!;
    f.setData(bootstrap(otherActor));
    await f.mount(otherActor);
    await until(
      () => f.devices.length === 2 && findButton('matrix.allowStorageAction') !== undefined,
    );
    expect(first.state.rootApprovals).toBe(0);
    expect(first.state.closed).toBe(true);
    expect(f.root.querySelector('[data-private]')).toBeNull();
    await act(() => button('matrix.allowStorageAction').click());
    await until(() => f.root.querySelector('[data-private]')?.textContent === otherActor);
    expect(f.devices[1]!.state.rootApprovals).toBe(1);
  } finally {
    await f.unmount();
  }
});

function sourceFixture(requireLegacy: boolean) {
  const current = fixture(),
    old = fixture();
  let prepared = 0,
    attempts = 0,
    failFirst = false;
  const data = {
    ...bootstrap(),
    storage_identity_version: 2 as const,
    matrix_user_id: '@sytue_storage_drive_synthetic-org_' + actor + ':matrix.example',
  };
  current.setData(data);
  const host: MatrixSourceGateHost = {
    ...current.host,
    prepare: async (_client, signal, resolveLegacy) => {
      attempts++;
      if (failFirst && attempts === 1) throw new MatrixSecurityError('recovery');
      if (requireLegacy) await resolveLegacy(signal);
      signal.throwIfAborted();
      prepared++;
    },
  };
  const mount = async () => {
    await act(() => {
      render(
        <ToastProvider>
          <MatrixSourceGate
            actor={actor}
            tenant="synthetic-org"
            locale="en"
            host={host}
            legacyHost={old.host}
          >
            {() => <p data-private-source>ready</p>}
          </MatrixSourceGate>
        </ToastProvider>,
        current.root,
      );
    });
  };
  const unmount = async () => {
    await current.unmount();
    await old.unmount();
  };
  return {
    current,
    old,
    host,
    mount,
    unmount,
    prepared: () => prepared,
    failFirst: () => {
      failFirst = true;
    },
  };
}

test('source gate never opens a historical SDK when the domain plan needs only the canonical identity', async () => {
  const f = sourceFixture(false);
  try {
    await f.mount();
    await until(() => f.prepared() === 1);
    expect(f.current.root.querySelector('[data-private-source]')).not.toBeNull();
    expect(f.current.devices).toHaveLength(1);
    expect(f.old.devices).toHaveLength(0);
  } finally {
    await f.unmount();
  }
});

test('source gate reuses the same recovery UI and retires both actual lifecycle slots together', async () => {
  const f = sourceFixture(true);
  try {
    await f.mount();
    await until(() => f.prepared() === 1);
    expect(f.current.devices).toHaveLength(1);
    expect(f.old.devices).toHaveLength(1);
    expect(f.current.root.querySelector('[data-private-source]')).not.toBeNull();
    await act(() => f.old.devices[0]!.instance.close());
    await until(() => f.current.devices[0]!.state.closed);
    expect(f.current.root.querySelector('[data-private-source]')).toBeNull();
  } finally {
    await f.unmount();
  }
});

test('source gate refuses a foreign historical owner before opening that SDK or mounting application data', async () => {
  const f = sourceFixture(true);
  f.old.setData(bootstrap(otherActor));
  try {
    await f.mount();
    await until(() => f.current.root.textContent?.includes('matrix.failed') === true);
    expect(f.prepared()).toBe(0);
    expect(f.old.devices).toHaveLength(0);
    expect(f.current.root.querySelector('[data-private-source]')).toBeNull();
  } finally {
    await f.unmount();
  }
});

test('source gate retries preparation on its existing verified SDK before opening the retained source', async () => {
  const f = sourceFixture(true);
  f.failFirst();
  try {
    await f.mount();
    await until(() => findButton('action.retry') !== undefined);
    await act(() => button('action.retry').click());
    await until(() => f.prepared() === 1);
    expect(f.current.devices).toHaveLength(1);
    expect(f.current.devices[0]!.state.closed).toBe(false);
    expect(f.old.devices).toHaveLength(1);
    expect(f.current.root.querySelector('[data-private-source]')).not.toBeNull();
  } finally {
    await f.unmount();
  }
});

test('source gate retries a rejected SDK open only after its prior generation has settled', async () => {
  const f = sourceFixture(true);
  const original = f.host.open!;
  let opens = 0;
  f.host.open = async (...args) => {
    if (++opens === 1) throw new MatrixSecurityError('device');
    return original(...args);
  };
  try {
    await f.mount();
    await until(() => findButton('action.retry') !== undefined);
    expect(f.current.devices).toHaveLength(0);
    await act(() => button('action.retry').click());
    await until(() => f.prepared() === 1);
    expect(opens).toBe(2);
    expect(f.current.devices).toHaveLength(1);
    expect(f.old.devices).toHaveLength(1);
  } finally {
    await f.unmount();
  }
});

test('source gate cancels an unverified source SDK when the canonical device retires during recovery', async () => {
  const f = sourceFixture(true);
  f.old.untrusted();
  try {
    await f.mount();
    await until(() => f.old.devices.length === 1);
    expect(f.prepared()).toBe(0);
    await act(() => f.current.devices[0]!.instance.close());
    await until(() => f.old.devices[0]!.state.closed);
    expect(f.current.root.querySelector('[data-private-source]')).toBeNull();
  } finally {
    await f.unmount();
  }
});

test('switching the visible account waits for the previous SDK drain after application cleanup', async () => {
  const f = fixture(),
    sdk = deferred();
  try {
    await f.mount();
    await until(() => f.root.querySelector('[data-private]') !== null);
    f.devices[0]!.state.sdkDrain = sdk.promise;
    f.setData(bootstrap(otherActor));
    await f.mount();
    await act(async () => {
      await new Promise((done) => setTimeout(done, 30));
    });
    expect(f.devices).toHaveLength(1);
    expect(f.devices[0]!.state.closed).toBe(true);
    expect(f.root.querySelector('[data-private]')).toBeNull();
    sdk.resolve();
    await until(() => f.devices.length === 2);
    await until(() => f.root.querySelector('[data-private]')?.textContent === otherActor);
  } finally {
    sdk.resolve();
    await f.unmount();
  }
});

/** Rooms: the canonical identity, without a private storage product. */
function roomsBootstrap(): MatrixBootstrap {
  const { storage_product: _p, storage_room_id: _r, storage_room_ids: _s, ...rest } = bootstrap();
  return rest;
}
function checkbox(): HTMLInputElement {
  const element = document.querySelector<HTMLInputElement>('[role=dialog] input[type=checkbox]');
  if (!element) throw new Error('missing checkbox');
  return element;
}

// «He olvidado la clave de recuperación» (D-ROOM-02, 2026-11-08): sin la clave no había salida.
test('Rooms without another device resets only after confirmation, and the new key is wrapped first', async () => {
  const f = fixture();
  f.untrusted();
  f.setData(roomsBootstrap());
  f.vault.wrappers.clear();
  const open = window.open;
  const opened: string[] = [];
  window.open = ((url: string) => {
    opened.push(url);
    return null;
  }) as typeof window.open;
  try {
    await f.mount();
    await until(() => f.devices.length === 1 && findButton('matrix.reset')?.disabled === false);
    await act(() => button('matrix.reset').click());
    await until(() => findButton('matrix.resetAction') !== undefined);
    expect(document.querySelector('[role=dialog]')?.textContent).toContain('matrix.resetBody');
    // Nothing is discarded without the acknowledgement.
    expect(button('matrix.resetAction').disabled).toBe(true);
    await act(() => button('matrix.resetAction').click());
    expect(f.devices[0]!.state.resets).toBe(0);
    await act(() => checkbox().click());
    expect(button('matrix.resetAction').disabled).toBe(false);
    await act(() => button('matrix.resetAction').click());
    // The new key is protected (and the old wrappers replaced) before anything is reset.
    await until(() => findButton('matrix.usePassword') !== undefined);
    expect(f.devices[0]!.state.resets).toBe(0);
    await typePassword('synthetic password', 'synthetic password');
    // The homeserver's re-authentication comes before any key is replaced.
    await until(() => findButton('matrix.authContinue') !== undefined);
    expect(f.devices[0]!.state.resets).toBe(0);
    expect(button('matrix.authContinue').disabled).toBe(true);
    await act(() => button('matrix.auth').click());
    expect(opened).toEqual(['https://matrix.example/_matrix/client/v3/auth/m.login.sso']);
    await act(() => button('matrix.authContinue').click());
    await until(() => f.root.querySelector('[data-private]') !== null);
    expect(f.devices[0]!.state.resets).toBe(1);
    expect(f.vault.log).toEqual(['wrap password KEY-RESET hasKey=true', 'publish KEY-RESET']);
    expect([...f.vault.wrappers.values()].map((w) => w.keyID)).toEqual(['KEY-RESET']);
    expectNoKeyRendered();
  } finally {
    window.open = open;
    await f.unmount();
  }
});

test('closing the reset confirmation discards nothing and forgets the acknowledgement', async () => {
  const f = fixture();
  f.untrusted();
  f.setData(roomsBootstrap());
  f.vault.wrappers.clear();
  try {
    await f.mount();
    await until(() => f.devices.length === 1 && findButton('matrix.reset')?.disabled === false);
    await act(() => button('matrix.reset').click());
    await until(() => findButton('matrix.resetAction') !== undefined);
    await act(() => checkbox().click());
    await act(() => {
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
    });
    await until(() => findButton('matrix.resetAction') === undefined);
    expect(f.devices[0]!.state.resets).toBe(0);
    await act(() => button('matrix.reset').click());
    await until(() => findButton('matrix.resetAction') !== undefined);
    expect(checkbox().checked).toBe(false);
    expect(button('matrix.resetAction').disabled).toBe(true);
  } finally {
    await f.unmount();
  }
});

test('private storage products do not offer a reset that would orphan their vault', async () => {
  const f = fixture();
  f.untrusted();
  f.vault.wrappers.clear();
  try {
    await f.mount();
    await until(() => f.devices.length === 1 && findButton('matrix.verify')?.disabled === false);
    expect(findButton('matrix.reset')).toBeUndefined();
  } finally {
    await f.unmount();
  }
});
