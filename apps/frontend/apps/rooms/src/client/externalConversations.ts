/* SPDX-License-Identifier: Apache-2.0 */
import { ExternalConversationsClient, HttpClient } from '@sytue/api-clients';
import { withBase } from '@sytue/ui';

export const externalConversations = new ExternalConversationsClient(
  new HttpClient({
    baseUrl:
      typeof window === 'undefined'
        ? ''
        : new URL(withBase(import.meta.env.BASE_URL, '/api'), window.location.origin).href,
  }),
);
