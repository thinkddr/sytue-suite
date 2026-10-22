/**
 * BFF Endpoint para iniciar trabajo de exportación asistida por IA.
 * Llama a aumentar() de @sytue/pages-render con páginas legales y catálogo (§9.3, §9.4).
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../../lib/bff.js';
import { DESTINO } from '../../../lib/servicio.js';
import { aumentar } from '@sytue/pages-render';
import type { Sitio, DatosRender } from '@sytue/pages-render';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) {
    return new Response(JSON.stringify({ error: 'unauthorized' }), {
      status: 401,
      headers: { 'Content-Type': 'application/json' },
    });
  }

  const siteId = ctx.params.id;
  if (!siteId) {
    return new Response(JSON.stringify({ error: 'site_id_required' }), {
      status: 400,
      headers: { 'Content-Type': 'application/json' },
    });
  }

  const body = (await ctx.request.json().catch(() => ({}))) as {
    destination?: string;
    ir?: Sitio;
    catalogo?: object;
  };

  const destination = body.destination || 'shopify';

  // Si no viene IR en la petición, lo obtenemos del borrador actual del sitio
  let ir = body.ir;
  if (!ir) {
    const resDraft = await fetch(`${DESTINO}/api/v1/sites/${siteId}/draft`, {
      headers: {
        Authorization: `Bearer ${session.accessToken}`,
      },
    });
    if (!resDraft.ok) {
      return new Response(await resDraft.text(), {
        status: resDraft.status,
        headers: { 'Content-Type': 'application/json' },
      });
    }
    const dataDraft = (await resDraft.json()) as { ir?: Sitio; draft?: Sitio };
    ir = dataDraft.ir || dataDraft.draft;
  }

  if (!ir) {
    return new Response(JSON.stringify({ error: 'draft_not_found' }), {
      status: 404,
      headers: { 'Content-Type': 'application/json' },
    });
  }

  let legal = {
    holder_name: 'Sytue',
    tax_id: '',
    country: 'ES',
    address: { street: '', postal_code: '', city: '', region: '' },
    email: '',
    phone: '',
    registry: '',
    privacy_contact: '',
  };

  try {
    const resSitio = await fetch(`${DESTINO}/api/v1/sites/${siteId}`, {
      headers: {
        Authorization: `Bearer ${session.accessToken}`,
      },
    });
    if (resSitio.ok) {
      const datosSitio = (await resSitio.json()) as { legal?: typeof legal };
      if (datosSitio.legal) {
        legal = { ...legal, ...datosSitio.legal };
      }
    }
  } catch {
    // Valores por defecto
  }

  const datosRender: DatosRender = {
    site: {
      id: siteId,
      slug: siteId,
      sites_host: 'sites.sytue.eu',
      canonical_host: '',
      hosts: [],
      analytics: false,
    },
    images: {},
    files: {},
    fonts: {},
    forms: {},
    origins: { cast: '', pages: '' },
    now: new Date().toISOString(),
    legal,
    store: null,
  };

  const aumentado = aumentar(ir, datosRender);

  const resExport = await fetch(`${DESTINO}/api/v1/sites/${siteId}/ia/export`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${session.accessToken}`,
    },
    body: JSON.stringify({
      destination,
      ir: aumentado.ir,
      catalogo: body.catalogo || aumentado.catalogo,
    }),
  });

  return new Response(await resExport.text(), {
    status: resExport.status,
    headers: { 'Content-Type': 'application/json' },
  });
};
