/* SPDX-License-Identifier: Apache-2.0 */
/** Resolve names and paths only on the verified device; the server redirects an opaque ID. */
import type { APIRoute } from 'astro';
import { returnToCanonico, rutaConOrg } from '@sytue/config';
import { bff, toBffContext } from '../../lib/auth';

export const prerender = false;
export const GET: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) {
    const returnTo = returnToCanonico(ctx.url, bff.basePath);
    const response = ctx.redirect(
      `${bff.basePath}/auth/login?returnTo=${encodeURIComponent(returnTo)}`,
    );
    response.headers.set('cache-control', 'no-store');
    return response;
  }
  const query = new URLSearchParams();
  if (ctx.params.id) query.set('file', ctx.params.id);
  const response = ctx.redirect(
    rutaConOrg(
      import.meta.env.BASE_URL,
      ctx.locals.org ?? session.user.tenant,
      query.size ? `/?${query}` : '/',
    ),
  );
  response.headers.set('cache-control', 'no-store');
  return response;
};
