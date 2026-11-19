/* SPDX-License-Identifier: Apache-2.0 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import {
  PqVaultDisabledError,
  PqVaultError,
  pqVaultProfile,
  type PqVault,
} from '@sytue/offline/vault';
import { PqVaultGate } from './PqVaultGate.js';
import { MatrixStorageFixture } from './MatrixStorageFixture.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const actor = '11111111-1111-4111-8111-111111111111';

async function until(check: () => boolean): Promise<void> {
  for (let i = 0; i < 80; i++) {
    if (check()) return;
    await new Promise((r) => setTimeout(r, 10));
  }
  throw new Error('timeout waiting for condition');
}

function fakeVault(overrides: Partial<PqVault> = {}): PqVault & { closedCount: number } {
  let closed = false;
  const stub = {
    scope: Object.freeze({ tenant: 'acme', subject: actor, product: 'notes' as const }),
    profile: pqVaultProfile,
    idOrigin: 'https://id.sytue.eu',
    vaultId: '!pq_testvault',
    keyId: 'abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789',
    recipient: 'age1pq1testrecipient',
    closedCount: 0,
    get isClosed() {
      return closed;
    },
    close() {
      closed = true;
    },
    async closeAndDrain() {
      closed = true;
      stub.closedCount++;
    },
    ...overrides,
  };
  return stub as unknown as PqVault & { closedCount: number };
}

test('PqVaultGate prepares, activates, mounts children, and drains on unmount', async () => {
  const root = document.createElement('div');
  document.body.appendChild(root);
  const vault = fakeVault();
  const events: string[] = [];
  try {
    await act(() => {
      render(
        <PqVaultGate
          actor={actor}
          tenant="acme"
          locale="es"
          product="notes"
          openVault={async () => vault}
          prepare={async (v) => {
            events.push('prepare:' + v.vaultId);
          }}
          activate={(v) => {
            events.push('activate:' + v.vaultId);
          }}
          detach={() => {
            events.push('detach');
          }}
          close={async () => {
            events.push('close');
          }}
        >
          {(v) => <p data-ready="true">{v.profile}</p>}
        </PqVaultGate>,
        root,
      );
    });
    await until(() => root.querySelector('[data-ready]') !== null);
    expect(root.querySelector('[data-ready]')?.textContent).toBe(pqVaultProfile);
    expect(events).toEqual(['prepare:!pq_testvault', 'activate:!pq_testvault']);
  } finally {
    await act(() => {
      render(null, root);
    });
    await until(() => vault.closedCount === 1);
    expect(events).toEqual(['prepare:!pq_testvault', 'activate:!pq_testvault', 'detach', 'close']);
    root.remove();
  }
});

test('PqVaultGate prompts sign-in on signin error and never mounts children', async () => {
  const root = document.createElement('div');
  document.body.appendChild(root);
  try {
    await act(() => {
      render(
        <PqVaultGate
          actor={actor}
          tenant="acme"
          locale="en"
          product="notes"
          openVault={async () => {
            throw new PqVaultError('signin');
          }}
        >
          <p data-secret="true">hidden</p>
        </PqVaultGate>,
        root,
      );
    });
    await until(() => root.querySelector('button') !== null);
    expect(root.querySelector('[data-secret]')).toBeNull();
    expect(root.textContent).toContain(pqVaultProfile);
  } finally {
    await act(() => {
      render(null, root);
    });
    root.remove();
  }
});

test('PqVaultGate mounts static children when encryption is disabled or inside MatrixStorageFixture', async () => {
  const root = document.createElement('div');
  document.body.appendChild(root);
  let openCalls = 0;
  try {
    await act(() => {
      render(
        <PqVaultGate
          actor={actor}
          tenant="acme"
          locale="en"
          product="notes"
          openVault={async () => {
            throw new PqVaultDisabledError();
          }}
        >
          <p data-disabled="true">open</p>
        </PqVaultGate>,
        root,
      );
    });
    await until(() => root.querySelector('[data-disabled]') !== null);
    expect(root.querySelector('[data-disabled]')?.textContent).toBe('open');

    await act(() => {
      render(
        <MatrixStorageFixture>
          <PqVaultGate
            actor={actor}
            tenant="acme"
            locale="en"
            product="notes"
            openVault={async () => {
              openCalls++;
              return fakeVault();
            }}
          >
            <p data-fixture="true">bench</p>
          </PqVaultGate>
        </MatrixStorageFixture>,
        root,
      );
    });
    await until(() => root.querySelector('[data-fixture]') !== null);
    expect(openCalls).toBe(0);
  } finally {
    await act(() => {
      render(null, root);
    });
    root.remove();
  }
});
