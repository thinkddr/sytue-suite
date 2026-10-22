/* SPDX-License-Identifier: Apache-2.0 */
import { readOfflineValue, writeOfflineValue, deleteOfflineValue } from '@sytue/offline';
import { withBase } from '@sytue/ui';
import {
  MatrixSecurityError,
  validateBootstrap,
  type MatrixBootstrap,
  type MatrixSession,
} from './types.js';
const namespace = 'rooms-matrix-session-v1';
const pendingKey = 'sytue-rooms-matrix-sso';
export async function matrixBootstrap(signal?: AbortSignal): Promise<MatrixBootstrap> {
  const response = await fetch(
    withBase(import.meta.env.BASE_URL ?? '/rooms', '/api/matrix/bootstrap'),
    { credentials: 'include', cache: 'no-store', ...(signal ? { signal } : {}) },
  );
  if (!response.ok) throw new MatrixSecurityError('login');
  return validateBootstrap((await response.json()) as MatrixBootstrap);
}
export function readMatrixSession(bootstrap: MatrixBootstrap): Promise<MatrixSession | null> {
  return readOfflineValue(bootstrap.matrix_user_id, namespace, bootstrap.homeserver_url);
}
export function forgetMatrixSession(bootstrap: MatrixBootstrap): Promise<void> {
  return deleteOfflineValue(bootstrap.matrix_user_id, namespace, bootstrap.homeserver_url);
}
/** The one-use SSO token is never written to browser storage or a Rooms request. */
export async function beginMatrixSso(bootstrap: MatrixBootstrap): Promise<void> {
  const { createClient } = await import('matrix-js-sdk');
  const client = createClient({ baseUrl: bootstrap.homeserver_url });
  const flows = await client.loginFlows();
  const supported = flows.flows.some(
    (flow) =>
      flow.type === 'm.login.sso' &&
      'identity_providers' in flow &&
      flow.identity_providers?.some((provider) => provider.id === bootstrap.sso_provider_id),
  );
  if (!supported) throw new MatrixSecurityError('identity');
  const nonce = crypto.randomUUID();
  const callback = new URL(
    withBase(import.meta.env.BASE_URL ?? '/rooms', '/matrix/callback'),
    window.location.origin,
  );
  callback.searchParams.set('state', nonce);
  const returnTo = window.location.pathname + window.location.search;
  sessionStorage.setItem(
    pendingKey,
    JSON.stringify({
      nonce,
      actor: bootstrap.actor,
      tenant: bootstrap.tenant,
      home: bootstrap.homeserver_url,
      user: bootstrap.matrix_user_id,
      returnTo,
      created: Date.now(),
    }),
  );
  window.location.assign(client.getSsoLoginUrl(callback.href, 'sso', bootstrap.sso_provider_id));
}
export async function completeMatrixSso(): Promise<string> {
  const url = new URL(window.location.href);
  let token = url.searchParams.get('loginToken') ?? '';
  const nonce = url.searchParams.get('state');
  // Clear the token before loading the Matrix SDK or any Rooms application code.
  window.history.replaceState(null, '', window.location.pathname);
  const raw = sessionStorage.getItem(pendingKey);
  sessionStorage.removeItem(pendingKey);
  if (
    !token ||
    token.length > 8192 ||
    !raw ||
    url.searchParams.getAll('loginToken').length !== 1 ||
    url.searchParams.getAll('state').length !== 1
  )
    throw new MatrixSecurityError('login');
  const pending = JSON.parse(raw) as {
    nonce: string;
    actor: string;
    tenant: string;
    home: string;
    user: string;
    returnTo: string;
    created: number;
  };
  if (
    pending.nonce !== nonce ||
    Date.now() - pending.created > 10 * 60 * 1000 ||
    pending.created > Date.now() ||
    !pending.returnTo.startsWith('/rooms/') ||
    pending.returnTo.startsWith('//')
  )
    throw new MatrixSecurityError('identity');
  const bootstrap = await matrixBootstrap();
  if (
    bootstrap.actor !== pending.actor ||
    bootstrap.tenant !== pending.tenant ||
    bootstrap.homeserver_url !== pending.home ||
    bootstrap.matrix_user_id !== pending.user
  )
    throw new MatrixSecurityError('identity');
  const { createClient } = await import('matrix-js-sdk');
  const client = createClient({ baseUrl: bootstrap.homeserver_url });
  const existing = await readMatrixSession(bootstrap);
  const session = await client.loginRequest({
    ...(existing?.user_id === bootstrap.matrix_user_id ? { device_id: existing.device_id } : {}),
    type: 'm.login.token',
    token,
    initial_device_display_name: 'Sytue Rooms · Web',
  });
  token = '';
  if (session.user_id !== bootstrap.matrix_user_id || !session.device_id || !session.access_token) {
    const foreign = createClient({
      baseUrl: bootstrap.homeserver_url,
      accessToken: session.access_token,
    });
    await foreign.logout().catch(() => undefined);
    throw new MatrixSecurityError('identity');
  }
  await writeOfflineValue(bootstrap.matrix_user_id, namespace, bootstrap.homeserver_url, {
    user_id: session.user_id,
    device_id: session.device_id,
    access_token: session.access_token,
  } satisfies MatrixSession);
  return pending.returnTo;
}
