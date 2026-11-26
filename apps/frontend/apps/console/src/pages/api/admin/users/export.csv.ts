/*
 * BFF de Sytue Admin — el directorio en CSV.
 *
 *   GET /api/admin/users/export.csv[?q=…&filter=without_mfa|suspended|unverified_email]
 *
 * Recorre las páginas del directorio de Sytue ID EN EL SERVIDOR (keyset, 200 por
 * página, hasta 10 000 personas) con el mismo término y filtro que la pantalla, y
 * entrega un fichero. Es lo que se pide para una auditoría de accesos o para
 * cuadrar plantilla con Recursos Humanos, y hasta ahora solo se podía hacer
 * copiando filas a mano de una lista paginada.
 *
 * Mismo listón que leer el directorio (users.read o users.manage): el fichero no
 * contiene nada que la pantalla no enseñe ya. Las celdas pasan por `csvCell` (la
 * regla del export de auditoría) contra la inyección de fórmulas. Sin backend
 * vivo, 503: no se entrega la demostración como si fuera el directorio real.
 */
import type { APIRoute } from 'astro';
import { requireSession } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../../../server/bff.js';
import { exportDirectoryCsv, proxyStatus } from '../../../../lib/admin.js';
import { filtroDeParametro } from '../../../../lib/filtros-usuarios.js';

export const prerender = false;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const GET: APIRoute = async (ctx) => {
  const guard = await requireSession(getBff(), toBffContext(ctx), {
    roles: ['admin', 'owner'],
    perms: ['users.read', 'users.manage'],
  });
  if (guard instanceof Response) {
    return json(
      { error: guard.status === 403 ? 'forbidden' : 'unauthenticated' },
      guard.status === 403 ? 403 : 401,
    );
  }
  const url = new URL(ctx.request.url);
  const q = url.searchParams.get('q')?.trim() ?? '';
  const filter = filtroDeParametro(url.searchParams.get('filter') ?? '');
  try {
    const { csv, truncated } = await exportDirectoryCsv(guard.session, { q, filter });
    const fecha = new Date().toISOString().slice(0, 10);
    return new Response(csv, {
      status: 200,
      headers: {
        'content-type': 'text/csv; charset=utf-8',
        'content-disposition': `attachment; filename="directorio-${fecha}.csv"`,
        'cache-control': 'no-store',
        // Más de 10 000 personas: el fichero sale con las primeras y lo dice.
        ...(truncated ? { 'x-sytue-truncated': '1' } : {}),
      },
    });
  } catch (e) {
    return json(
      { error: 'upstream' },
      e instanceof Error && e.message === 'bad_shape' ? 502 : proxyStatus(e),
    );
  }
};
