/* SPDX-License-Identifier: Apache-2.0 */
import {
  deleteOfflineValue,
  listOfflineValues,
  readOfflineValue,
  writeOfflineValue,
} from '@sytue/offline';
import type { ValidatedAuthMetadata } from 'matrix-js-sdk';
import { matrixBootstrap } from './auth.js';
import { MatrixSecurityError } from './types.js';

export interface ExternalMatrixScope {
  actor: string;
  tenant: string;
}
export interface ExternalMatrixAccountSummary {
  id: string;
  homeserver_url: string;
  user_id: string;
  device_id: string;
}
export interface ExternalMatrixAccount extends ExternalMatrixAccountSummary {
  client_id: string;
  metadata: ValidatedAuthMetadata;
  access_token: string;
  refresh_token: string;
  expires_at_ms?: number;
}
interface PendingExternalMatrix {
  v: 1;
  scope: ExternalMatrixScope;
  expected_user: string;
  home: string;
  account_id: string;
  metadata: ValidatedAuthMetadata;
  client_id: string;
  device_id: string;
  verifier: string;
  state: string;
  redirect: string;
  return_to: string;
  created: number;
}
const pendingKey = 'sytue-external-matrix-oauth-v1';
function namespace(scope: ExternalMatrixScope): string {
  if (
    !/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/i.test(scope.actor) ||
    !/^[a-z0-9][a-z0-9-]{0,62}$/.test(scope.tenant)
  )
    throw new MatrixSecurityError('identity');
  return `rooms-external-matrix-v1:${scope.tenant}`;
}
/** Metadata URLs are used only by the Matrix SDK, without Sytue bearer tokens or cookies. */
export function externalMatrixHttps(value: string): URL {
  const url = new URL(value);
  if (url.protocol !== 'https:' || url.username || url.password || url.hash || !url.hostname)
    throw new MatrixSecurityError('identity');
  return url;
}
export function externalMatrixUser(value: string): { user: string; server: string } {
  const user = value.trim();
  const delimiter = user.indexOf(':');
  if (!user.startsWith('@') || delimiter < 2 || user.length > 255 || /[\p{Cc}\p{Cs}\s]/u.test(user))
    throw new MatrixSecurityError('identity');
  const server = user.slice(delimiter + 1);
  const home = externalMatrixHttps(`https://${server}`);
  if (home.pathname !== '/' || home.search || home.host !== server.toLowerCase())
    throw new MatrixSecurityError('identity');
  return { user, server };
}
export function validateExternalMatrixMetadata(
  metadata: ValidatedAuthMetadata,
): ValidatedAuthMetadata {
  const issuer = externalMatrixHttps(metadata.issuer);
  if (issuer.search) throw new MatrixSecurityError('identity');
  for (const field of [
    'authorization_endpoint',
    'token_endpoint',
    'registration_endpoint',
    'revocation_endpoint',
  ] as const)
    externalMatrixHttps(metadata[field]);
  if (
    !metadata.grant_types_supported?.includes('authorization_code') ||
    !metadata.grant_types_supported?.includes('refresh_token') ||
    !metadata.code_challenge_methods_supported?.includes('S256') ||
    !metadata.response_modes_supported?.includes('fragment') ||
    !metadata.response_types_supported?.includes('code')
  )
    throw new MatrixSecurityError('unsupported');
  return metadata;
}
export async function externalMatrixAccounts(
  scope: ExternalMatrixScope,
): Promise<ExternalMatrixAccountSummary[]> {
  const rows = await listOfflineValues<ExternalMatrixAccount>(scope.actor, namespace(scope));
  return rows.map(({ value: account }) => ({
    id: account.id,
    homeserver_url: account.homeserver_url,
    user_id: account.user_id,
    device_id: account.device_id,
  }));
}
export async function readExternalMatrixAccount(
  scope: ExternalMatrixScope,
  id: string,
): Promise<ExternalMatrixAccount> {
  const account = await readOfflineValue<ExternalMatrixAccount>(scope.actor, namespace(scope), id);
  if (
    !account ||
    account.id !== id ||
    !account.access_token ||
    !account.refresh_token ||
    !account.device_id
  )
    throw new MatrixSecurityError('login');
  externalMatrixUser(account.user_id);
  externalMatrixHttps(account.homeserver_url);
  validateExternalMatrixMetadata(account.metadata);
  return account;
}
export async function saveExternalMatrixAccount(
  scope: ExternalMatrixScope,
  account: ExternalMatrixAccount,
): Promise<void> {
  if (
    !account.id ||
    !account.access_token ||
    !account.refresh_token ||
    !account.device_id ||
    !account.client_id
  )
    throw new MatrixSecurityError('identity');
  externalMatrixUser(account.user_id);
  externalMatrixHttps(account.homeserver_url);
  validateExternalMatrixMetadata(account.metadata);
  await writeOfflineValue(scope.actor, namespace(scope), account.id, account);
}
export function externalMatrixReturn(value: string, origin: string): URL {
  if (!value.startsWith('/') || value.startsWith('//')) throw new MatrixSecurityError('identity');
  const target = new URL(value, origin);
  if (
    target.origin !== origin ||
    !(target.pathname === '/rooms' || target.pathname.startsWith('/rooms/'))
  )
    throw new MatrixSecurityError('identity');
  return target;
}

/** Web uses the SDK's authorization-code grant with PKCE. Provider tokens never enter a BFF. */
export async function beginExternalMatrixAuthorization(
  scope: ExternalMatrixScope,
  input: string,
): Promise<void> {
  namespace(scope);
  const { user, server } = externalMatrixUser(input);
  const { AutoDiscovery, OAuth2, createClient } = await import('matrix-js-sdk');
  const discovery = await AutoDiscovery.findClientConfig(server);
  const config = discovery['m.homeserver'];
  if (config.state !== AutoDiscovery.SUCCESS || !config.base_url)
    throw new MatrixSecurityError('unsupported');
  const home = externalMatrixHttps(config.base_url);
  if (home.search) throw new MatrixSecurityError('identity');
  const client = createClient({ baseUrl: home.href });
  const metadata = validateExternalMatrixMetadata(await client.getAuthMetadata());
  const existing = (await externalMatrixAccounts(scope)).find(
    (account) => account.user_id === user && new URL(account.homeserver_url).href === home.href,
  );
  const previous = existing ? await readExternalMatrixAccount(scope, existing.id) : null;
  const redirect = new URL('/rooms/matrix/external/callback', window.location.origin).href;
  const clientId =
    previous?.metadata.issuer === metadata.issuer
      ? previous.client_id
      : await OAuth2.registerClient(metadata, {
          client_name: 'Sytue Rooms',
          application_type: 'web',
          client_uri: `${window.location.origin}/rooms/`,
          redirect_uris: [redirect],
          grant_types: ['authorization_code', 'refresh_token'],
          response_types: ['code'],
          token_endpoint_auth_method: 'none',
          policy_uri: `${window.location.origin}/privacy`,
          tos_uri: `${window.location.origin}/terms`,
        });
  const oauth = new OAuth2(metadata, {
    clientId,
    ...(previous ? { deviceId: previous.device_id } : {}),
  });
  const state = crypto.randomUUID();
  const authorize = await oauth.generateAuthorizationCodeGrantUrl(state, redirect, 'fragment');
  const pending: PendingExternalMatrix = {
    v: 1,
    scope,
    expected_user: user,
    home: home.href,
    metadata,
    client_id: clientId,
    device_id: oauth.context.deviceId,
    verifier: oauth.context.codeVerifier,
    state,
    redirect,
    account_id: previous?.id ?? crypto.randomUUID(),
    return_to: window.location.pathname + window.location.search,
    created: Date.now(),
  };
  externalMatrixReturn(pending.return_to, window.location.origin);
  sessionStorage.setItem(pendingKey, JSON.stringify(pending));
  window.location.assign(authorize);
}

/** Clear the authorization code before loading the SDK, then bind the return to the current person. */
export async function completeExternalMatrixAuthorization(): Promise<string> {
  const here = new URL(window.location.href);
  const params = new URLSearchParams(here.hash.slice(1));
  let code = params.get('code') ?? '';
  const state = params.get('state');
  window.history.replaceState(null, '', here.pathname);
  const raw = sessionStorage.getItem(pendingKey);
  sessionStorage.removeItem(pendingKey);
  if (
    !raw ||
    !code ||
    code.length > 8192 ||
    params.getAll('code').length !== 1 ||
    params.getAll('state').length !== 1 ||
    here.search
  )
    throw new MatrixSecurityError('login');
  const pending = JSON.parse(raw) as PendingExternalMatrix;
  if (
    pending.v !== 1 ||
    pending.state !== state ||
    !pending.verifier ||
    !pending.client_id ||
    !pending.device_id ||
    Date.now() - pending.created > 10 * 60 * 1000 ||
    pending.created > Date.now() ||
    pending.redirect !== `${here.origin}/rooms/matrix/external/callback` ||
    pending.expected_user !== externalMatrixUser(pending.expected_user).user ||
    (params.has('iss') &&
      (params.getAll('iss').length !== 1 || params.get('iss') !== pending.metadata.issuer))
  )
    throw new MatrixSecurityError('identity');
  validateExternalMatrixMetadata(pending.metadata);
  externalMatrixHttps(pending.home);
  const returnTo = externalMatrixReturn(pending.return_to, here.origin);
  const current = await matrixBootstrap();
  if (current.actor !== pending.scope.actor || current.tenant !== pending.scope.tenant)
    throw new MatrixSecurityError('identity');
  const { OAuth2, createClient } = await import('matrix-js-sdk');
  const oauth = new OAuth2(pending.metadata, {
    clientId: pending.client_id,
    deviceId: pending.device_id,
    codeVerifier: pending.verifier,
  });
  const tokens = await oauth.completeAuthorizationCodeGrant(code, pending.redirect);
  code = '';
  try {
    const granted = new Set(tokens.scope?.split(' ') ?? []);
    if (
      !tokens.refresh_token ||
      !granted.has('urn:matrix:client:api:*') ||
      !granted.has(`urn:matrix:client:device:${pending.device_id}`)
    )
      throw new MatrixSecurityError('identity');
    const client = createClient({ baseUrl: pending.home, accessToken: tokens.access_token });
    const who = await client.whoami();
    if (who.user_id !== pending.expected_user || who.device_id !== pending.device_id)
      throw new MatrixSecurityError('identity');
    await saveExternalMatrixAccount(pending.scope, {
      id: pending.account_id,
      user_id: who.user_id,
      device_id: pending.device_id,
      homeserver_url: pending.home,
      client_id: pending.client_id,
      metadata: pending.metadata,
      access_token: tokens.access_token,
      refresh_token: tokens.refresh_token,
      ...(tokens.expires_in ? { expires_at_ms: Date.now() + tokens.expires_in * 1000 } : {}),
    });
  } catch (error) {
    await oauth
      .revokeToken(
        tokens.refresh_token || tokens.access_token,
        tokens.refresh_token ? 'refresh_token' : 'access_token',
      )
      .catch(() => undefined);
    throw error;
  }
  returnTo.searchParams.set('connect-external-matrix', '1');
  returnTo.searchParams.set('matrix-account', pending.account_id);
  return returnTo.pathname + returnTo.search;
}
export async function disconnectExternalMatrixAccount(
  scope: ExternalMatrixScope,
  id: string,
): Promise<void> {
  const account = await readExternalMatrixAccount(scope, id);
  const { OAuth2 } = await import('matrix-js-sdk');
  await new OAuth2(account.metadata, {
    clientId: account.client_id,
    deviceId: account.device_id,
  }).revokeToken(account.refresh_token, 'refresh_token');
  await deleteOfflineValue(scope.actor, namespace(scope), id);
}
