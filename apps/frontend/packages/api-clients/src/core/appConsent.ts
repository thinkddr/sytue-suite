/* SPDX-License-Identifier: Apache-2.0 */
import { HttpError, type HttpClient } from './http.js';

export interface ConsentProvider {
  template: string;
  name: string;
  host: string;
}

export const ROOMS_PROVIDERS = {
  'slack-rooms': { template: 'slack-rooms', name: 'Slack', host: 'slack.com' },
  'microsoft-teams-rooms': {
    template: 'microsoft-teams-rooms',
    name: 'Microsoft Teams',
    host: 'login.microsoftonline.com',
  },
  'google-chat': { template: 'google-chat', name: 'Google Chat', host: 'accounts.google.com' },
} as const satisfies Record<string, ConsentProvider>;

export type RoomsProvider = keyof typeof ROOMS_PROVIDERS;

export function isRoomsProvider(value: unknown): value is RoomsProvider {
  return typeof value === 'string' && Object.hasOwn(ROOMS_PROVIDERS, value);
}

function preferredOAuthInstance<T extends { oauth_available?: boolean }>(
  items: T[],
  connected: (item: T) => boolean,
): T | undefined {
  return (
    items.find((item) => item.oauth_available === true && connected(item)) ??
    items.find((item) => item.oauth_available === true) ??
    items.find(connected) ??
    items[0]
  );
}

/** Personal consent must stay on the selected provider's exact HTTPS origin. */
export function providerAuthorizationURL(raw: string, host: string, kind: string): URL {
  const url = new URL(raw);
  if (url.origin !== `https://${host}` || url.username || url.password || url.hash)
    throw new Error(`invalid_${kind}_authorization_url`);
  return url;
}

/** The gateway owns PKCE, sealed state, provider credentials and the token vault. */
export async function startAppConsent(
  client: HttpClient,
  provider: ConsentProvider,
  section: 'storage' | 'rooms',
  instanceId?: string,
  options: { newAccount?: boolean } = {},
): Promise<{ instance_id: string; authorize_url: string }> {
  if (options.newAccount && instanceId) throw new Error(`${section}_instance_mismatch`);
  if (options.newAccount) {
    // A private OAuth client attached to an existing connection cannot be
    // silently copied to a new account. New consent needs platform readiness.
    const { templates } = await client.json<{
      templates: { key: string; oauth_available?: boolean }[];
    }>({ path: '/templates' });
    if (
      !templates.some((item) => item.key === provider.template && item.oauth_available === true)
    ) {
      throw new HttpError(
        409,
        'OAuth provider unavailable',
        '/templates',
        JSON.stringify({ error: { code: 'oauth_provider_unavailable' } }),
      );
    }
  }
  const { instances } = options.newAccount
    ? { instances: [] }
    : await client.json<{
        instances: {
          id: string;
          template_key: string;
          status: string;
          has_secrets?: boolean;
          oauth_available?: boolean;
        }[];
      }>({ path: '/instances' });
  const matches = instances.filter(
    (i) => i.template_key === provider.template && (!instanceId || i.id === instanceId),
  );
  const existing = preferredOAuthInstance(
    matches,
    (instance) => instance.status === 'active' && instance.has_secrets !== false,
  );
  if (instanceId && !existing) throw new Error(`${section}_instance_mismatch`);
  const instance =
    existing ??
    (await client.postJson<{ id: string }>('/instances', {
      template_key: provider.template,
      name: provider.name,
    }));
  const result = await client.postJson<{ authorize_url: string }>(
    `/instances/${encodeURIComponent(instance.id)}/oauth/start`,
    { return_to: section },
  );
  const url = providerAuthorizationURL(result.authorize_url, provider.host, section);
  return { instance_id: instance.id, authorize_url: url.href };
}

export function finishAppConsent(
  client: HttpClient,
  query: URLSearchParams,
): Promise<{ section?: string; instance?: string; error?: string }> {
  return client.postJson('/oauth/finish', {
    state: query.get('state') ?? '',
    code: query.get('code') ?? '',
    error: query.get('error') ?? '',
  });
}

/** A Rooms account can be disconnected here without access to the Admin product. */
export async function disconnectRoomsConnection(
  client: HttpClient,
  instanceId: string,
): Promise<void> {
  const connections = await listRoomsConnections(client);
  if (!connections.items.some((instance) => instance.id === instanceId)) {
    throw new Error('rooms_instance_mismatch');
  }
  await client.postJson(`/instances/${encodeURIComponent(instanceId)}/deactivate`, {});
}

/** Public account inventory: tokens, webhook URLs and provider config stay in the gateway. */
export async function listRoomsConnections(client: HttpClient): Promise<{
  available: boolean;
  providers: {
    key: RoomsProvider;
    name: string;
    scopes: string[];
    connected: boolean;
    available: boolean;
    instance_id?: string;
  }[];
  items: { id: string; template_key: RoomsProvider; name: string; connected: boolean }[];
}> {
  const [catalog, accounts] = await Promise.all([
    client.json<{
      templates: {
        key: string;
        oauth_available?: boolean;
        manifest: { oauth?: { scopes?: string[] } };
      }[];
    }>({ path: '/templates' }),
    client.json<{
      instances: {
        id: string;
        template_key: string;
        name: string;
        status: string;
        has_secrets: boolean;
        oauth_available?: boolean;
      }[];
    }>({ path: '/instances' }),
  ]);
  const items = accounts.instances.flatMap((i) =>
    isRoomsProvider(i.template_key)
      ? [
          {
            id: i.id,
            template_key: i.template_key,
            name: i.name,
            connected: i.status === 'active' && i.has_secrets,
          },
        ]
      : [],
  );
  const providers = catalog.templates.flatMap((template) => {
    if (!isRoomsProvider(template.key) || !template.manifest.oauth) return [];
    const key = template.key;
    const matches = accounts.instances.filter((item) => item.template_key === key);
    const preferred = preferredOAuthInstance(
      matches,
      (item) => item.status === 'active' && item.has_secrets,
    );
    const instance = items.find((item) => item.id === preferred?.id);
    return [
      {
        key,
        name: ROOMS_PROVIDERS[key].name,
        scopes: template.manifest.oauth.scopes ?? [],
        connected: instance?.connected ?? false,
        available: template.oauth_available === true || preferred?.oauth_available === true,
        ...(instance ? { instance_id: instance.id } : {}),
      },
    ];
  });
  return { available: providers.length > 0, providers, items };
}
