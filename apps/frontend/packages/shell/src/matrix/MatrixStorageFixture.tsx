/* SPDX-License-Identifier: Apache-2.0 */
/**
 * Development benches only (`/dev/*` routes, a 404 in production): mounts the REAL
 * `MatrixStorageGate` of a product screen with a synthetic, already verified device and
 * no homeserver. The benches audit the screen behind the gate (layout, keyboard, axe,
 * scroll) without a BFF session; the gate's own flow has its tests in
 * MatrixDeviceGate.test.tsx and the browser fixtures of test/e2e.
 *
 * Only the homeserver half is replaced (bootstrap and SDK open). The gate keeps its state
 * machine, its identity checks against the bootstrap and the product's `prepare`, and
 * renders the same ready-state tree as in production: the children inside its isolated
 * toast provider, plus the encryption button and its dialog.
 */
import type { ComponentChildren, JSX } from 'preact';
import type { MatrixBootstrap, SuiteMatrix } from '@sytue/offline/matrix';
import {
  MatrixStorageFixtureContext,
  type MatrixStorageFixtureFactory,
} from './storage-fixture-context.js';

const homeserver = 'https://matrix.fixture.invalid';

const fixture: MatrixStorageFixtureFactory = ({ actor, tenant, product }) => {
  const bootstrap: MatrixBootstrap = {
    actor,
    tenant: tenant ?? 'fixture',
    homeserver_url: homeserver,
    matrix_user_id: `@fixture-${product}:matrix.fixture.invalid`,
    controller_user_id: '@controller:matrix.fixture.invalid',
    sso_provider_id: 'sytue',
    channels: [],
    encryption: 'm.megolm.v1.aes-sha2',
    storage_product: product,
    storage_identity_version: 2,
    storage_room_id: '!private:matrix.fixture.invalid',
    storage_room_ids: ['!private:matrix.fixture.invalid'],
  };
  return {
    bootstrap: () => Promise.resolve(bootstrap),
    open: (value, ui) => {
      let closed = false;
      const device = { id: 'FIXTURE', name: 'Fixture web device', verified: true, current: true };
      const instance = {
        bootstrap: value,
        get isClosed() {
          return closed;
        },
        client: { on: () => undefined },
        trusted: () => Promise.resolve(true),
        devices: () => Promise.resolve([device]),
        unverifiedRecipients: () => Promise.resolve([]),
        canVerifyUser: () => false,
        // No private record exists to migrate or open: the bench answers the product's
        // own BFF routes with `page.route`.
        preparePrivateStorage: () => Promise.resolve(),
        prepareDeviceStorage: () => Promise.resolve(),
        close: () => {
          closed = true;
          ui.cancelPending?.();
        },
        closeAndDrain: () => {
          closed = true;
          return Promise.resolve();
        },
        forget: () => {
          closed = true;
          return Promise.resolve();
        },
      };
      return Promise.resolve(instance as unknown as SuiteMatrix);
    },
  };
};

export function MatrixStorageFixture({ children }: { children: ComponentChildren }): JSX.Element {
  return (
    <MatrixStorageFixtureContext.Provider value={fixture}>
      {children}
    </MatrixStorageFixtureContext.Provider>
  );
}
