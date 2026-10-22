/* SPDX-License-Identifier: Apache-2.0 */
import type { APIRoute } from 'astro';
import { canonicaConOrg } from '@sytue/config';
import type { Bff } from './bff.js';
import { requireSession } from './guard.js';
import { toBffContext } from './app-bff.js';

/** Direct OAuth return to the originating app, including free personal accounts. */
export function rutaDeRetornoDeApp(options: {
  bff: Bff;
  section:
    | 'storage'
    | 'rooms'
    | 'external-chat'
    | 'personal-storage'
    | 'external-mail'
    | 'external-calendar'
    | 'external-contacts'
    | 'external-notes';
  finish: (
    accessToken: string,
    query: URLSearchParams,
  ) => Promise<{ section?: string; instance?: string; error?: string }>;
}): APIRoute {
  return async (ctx) => {
    // The gateway checks actor + tenant against sealed state when exchanging the code.
    const guard = await requireSession(options.bff, toBffContext(ctx));
    if (guard instanceof Response) return guard;
    const key =
      options.section === 'external-calendar' ||
      options.section === 'external-contacts' ||
      options.section === 'external-notes'
        ? 'connect-linked'
        : options.section === 'external-mail'
          ? 'connect-mail'
          : options.section === 'storage'
            ? 'connect-storage'
            : options.section === 'personal-storage'
              ? 'connect-cloud'
              : options.section === 'external-chat'
                ? 'connect-external'
                : 'connect-app';
    const query = new URLSearchParams({ [key]: '1' });
    try {
      const result = await options.finish(guard.session.tokens.accessToken, ctx.url.searchParams);
      const meeting =
        options.section === 'external-chat' && result.section === 'external-chat-meeting';
      if (result.section !== options.section && !meeting) query.set('oauth_error', 'state');
      else {
        if (result.instance) query.set('instance', result.instance);
        if (meeting) query.set('meeting', '1');
        if (result.error) query.set('oauth_error', result.error);
      }
    } catch {
      query.set('oauth_error', 'exchange');
    }
    const path = `${options.bff.basePath}/?${query.toString()}`;
    return new Response(null, {
      status: 302,
      headers: {
        location:
          canonicaConOrg(new URL(path, ctx.url), options.bff.basePath, guard.session.user.tenant) ??
          path,
        'cache-control': 'no-store',
        'referrer-policy': 'no-referrer',
      },
    });
  };
}
