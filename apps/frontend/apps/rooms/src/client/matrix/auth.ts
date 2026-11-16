/* SPDX-License-Identifier: Apache-2.0 */
import { MatrixBrowserAuth } from '@sytue/offline/matrix/auth';
import type { MatrixBootstrap } from './types.js';

/** Shared SSO engine; deployed Rooms session, device store and callback stay intact. */
const auth = new MatrixBrowserAuth({ deviceName: 'Sytue Rooms · Web' });
export const matrixBootstrap = auth.refreshBootstrap;
export const readMatrixSession = auth.readSession;
export const forgetMatrixSession = auth.forgetSession;
export const beginMatrixSso = (bootstrap: MatrixBootstrap): Promise<void> => auth.begin(bootstrap);
export const completeMatrixSso = (): Promise<string> => auth.complete();
export const roomsMatrixSessionAdapter = auth;
