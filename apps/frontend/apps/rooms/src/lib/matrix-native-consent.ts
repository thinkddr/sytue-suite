/* SPDX-License-Identifier: Apache-2.0 */
import type { ChatClient } from '@sytue/api-clients/chat';
import { appForKey, type SuiteApp } from '@sytue/ui';

type Preview = Awaited<ReturnType<ChatClient['previewMatrixNative']>>;
type Home = Pick<
  Awaited<ReturnType<ChatClient['matrixBootstrap']>>,
  'actor' | 'tenant' | 'homeserver_url' | 'sso_provider_id'
>;
export interface NativeConsentHandoff {
  target: URL;
  tenant: string;
  app?: SuiteApp;
  requiresApproval: boolean;
}

/** Public preview metadata is routing, never authorization. The backend still
 * consumes the authenticated actor/tenant/PKCE purpose and its durable approval. */
export function matrixNativeConsent(
  preview: Preview,
  home: Home,
  current: { actor: string; state: string; origin: string; basePath: string },
): NativeConsentHandoff {
  const target = new URL(preview.authorize_url);
  const homeserver = new URL(home.homeserver_url);
  const callback = new URL(target.searchParams.get('redirectUrl') ?? '');
  if (
    preview.actor !== current.actor ||
    preview.actor !== home.actor ||
    preview.tenant !== home.tenant ||
    preview.state !== current.state ||
    target.protocol !== 'https:' ||
    homeserver.protocol !== 'https:' ||
    target.origin !== homeserver.origin ||
    target.pathname !== `/_matrix/client/v3/login/sso/redirect/${home.sso_provider_id}` ||
    target.username ||
    target.password ||
    target.hash ||
    callback.origin !== current.origin ||
    callback.pathname !== `${current.basePath}/matrix/native/callback` ||
    callback.username ||
    callback.password ||
    callback.hash ||
    callback.searchParams.get('state') !== current.state ||
    [...callback.searchParams.keys()].join(',') !== 'state'
  )
    throw new Error('matrix_identity');
  if (preview.purpose === undefined || preview.purpose === '')
    return { target, tenant: preview.tenant, requiresApproval: false };
  const product = preview.product;
  const app =
    typeof product === 'string' ? appForKey(product === 'hub' ? 'one' : product) : undefined;
  if (
    !app ||
    preview.purpose !== `resources:${product}` ||
    typeof preview.resource_approved !== 'boolean'
  )
    throw new Error('matrix_purpose');
  return { target, tenant: preview.tenant, app, requiresApproval: !preview.resource_approved };
}
