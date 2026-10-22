/* SPDX-License-Identifier: Apache-2.0 */
import type { HttpClient } from './http.js';
import { startAppConsent } from './appConsent.js';

/** Cloud storage uses the gateway's sealed OAuth state, PKCE and token vault. */
export const STORAGE_PROVIDERS = {
  gdrive: { template: 'google-drive', name: 'Google Drive', host: 'accounts.google.com' },
  onedrive: { template: 'onedrive', name: 'OneDrive', host: 'login.microsoftonline.com' },
  dropbox: { template: 'dropbox-storage', name: 'Dropbox', host: 'www.dropbox.com' },
} as const;

export type StorageOAuthKind = keyof typeof STORAGE_PROVIDERS;

export function isStorageOAuthKind(kind: unknown): kind is StorageOAuthKind {
  return typeof kind === 'string' && Object.hasOwn(STORAGE_PROVIDERS, kind);
}

export async function startStorageConsent(
  client: HttpClient,
  kind: StorageOAuthKind,
  instanceId?: string,
  options: { newAccount?: boolean } = {},
): Promise<{ instance_id: string; authorize_url: string }> {
  return startAppConsent(client, STORAGE_PROVIDERS[kind], 'storage', instanceId, options);
}
