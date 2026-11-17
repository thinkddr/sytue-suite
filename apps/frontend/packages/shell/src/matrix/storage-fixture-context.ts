/* SPDX-License-Identifier: Apache-2.0 */
import { createContext } from 'preact';
import type { MatrixStorageProduct } from '@sytue/config/matrix';
import type { MatrixGateHost } from './MatrixDeviceGate.js';

export interface MatrixStorageFixtureScope {
  actor: string;
  tenant: string | undefined;
  product: MatrixStorageProduct;
}

/** Replaces the gate's homeserver half (bootstrap and SDK open) for one storage scope. */
export type MatrixStorageFixtureFactory = (
  scope: MatrixStorageFixtureScope,
) => Pick<MatrixGateHost, 'bootstrap' | 'open'>;

/**
 * Empty in every product tree. Only the development benches (`/dev/*` routes, a 404 in
 * production) provide it, through `@sytue/shell/matrix/fixture`.
 */
export const MatrixStorageFixtureContext = createContext<MatrixStorageFixtureFactory | null>(null);
