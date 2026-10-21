/* SPDX-License-Identifier: Apache-2.0 */
import { withBase } from '@sytue/ui';
import { MatrixSecurityError } from './types.js';
export function opaqueMatrixNative(value: unknown): value is string {
  return typeof value === 'string' && /^[A-Za-z0-9_-]{43}$/.test(value);
}
export function matrixNativeReturn(input: unknown, state: string): string {
  if (!input || typeof input !== 'object') throw new MatrixSecurityError('identity');
  const result = input as { ticket?: unknown; redirect_url?: unknown };
  if (!opaqueMatrixNative(result.ticket) || typeof result.redirect_url !== 'string')
    throw new MatrixSecurityError('identity');
  const url = new URL(result.redirect_url);
  if (
    url.protocol !== 'sytue-rooms:' ||
    url.host !== 'matrix' ||
    url.pathname !== '/callback' ||
    url.username ||
    url.password ||
    url.hash ||
    url.searchParams.get('state') !== state ||
    url.searchParams.get('ticket') !== result.ticket ||
    [...url.searchParams.keys()].sort().join(',') !== 'state,ticket'
  )
    throw new MatrixSecurityError('identity');
  return url.href;
}
/** Browser session is established before Synapse issues this one-use token. */
export async function completeMatrixNative(): Promise<string> {
  const url = new URL(window.location.href);
  let loginToken = url.searchParams.get('loginToken') ?? '';
  const state = url.searchParams.get('state');
  window.history.replaceState(null, '', window.location.pathname);
  if (
    !opaqueMatrixNative(state) ||
    url.searchParams.getAll('state').length !== 1 ||
    url.searchParams.getAll('loginToken').length !== 1 ||
    loginToken.length < 16 ||
    loginToken.length > 8192 ||
    /[\r\n\0]/.test(loginToken)
  )
    throw new MatrixSecurityError('login');
  const response = await fetch(
    withBase(import.meta.env.BASE_URL ?? '/rooms', '/api/matrix/native/accept'),
    {
      method: 'POST',
      credentials: 'include',
      cache: 'no-store',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ state, login_token: loginToken }),
    },
  );
  loginToken = '';
  if (!response.ok) throw new MatrixSecurityError('identity');
  return matrixNativeReturn(await response.json(), state);
}
