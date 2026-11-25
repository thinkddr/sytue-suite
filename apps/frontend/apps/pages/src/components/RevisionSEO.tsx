/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * RevisionSEO — lo que verá un buscador del borrador, antes de publicarlo.
 *
 * La revisión la hace el servicio (`GET /sites/{id}/seo`, paquete Go `seo`) sobre el
 * mismo IR que se publica: títulos y descripciones vacíos, demasiado largos o
 * repetidos entre páginas, imágenes sin texto alternativo (que es también lo que oye
 * quien usa un lector de pantalla) y páginas ocultas a los buscadores. Son avisos, no
 * un bloqueo: publicar con ellos es legítimo.
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useState } from 'preact/hooks';
import { Badge, Button, Skeleton } from '@sytue/ui';
import type { ClaveMensaje } from '../i18n/index.js';
import { ruta } from '../lib/rutas.js';

type T = (k: ClaveMensaje, params?: Record<string, string | number>) => string;

interface AvisoSEO {
  code: string;
  /** `warning` empeora cómo se ve en un buscador; `info` puede ser intencionado. */
  severity: string;
  page_id?: string;
  page?: string;
  lang?: string;
  count?: number;
}

/** Texto de cada código estable del servicio (services/pages/internal/seo). */
const TEXTO: Record<string, ClaveMensaje> = {
  title_missing: 'seo.title_missing',
  title_long: 'seo.title_long',
  title_duplicate: 'seo.title_duplicate',
  description_missing: 'seo.description_missing',
  description_long: 'seo.description_long',
  image_alt_missing: 'seo.image_alt_missing',
  page_noindex: 'seo.page_noindex',
  site_noindex: 'seo.site_noindex',
  share_image_missing: 'seo.share_image_missing',
};

export function RevisionSEO({ siteId, t }: { siteId: string; t: T }): JSX.Element {
  const [estado, setEstado] = useState<'cargando' | 'ok' | 'error'>('cargando');
  const [avisos, setAvisos] = useState<AvisoSEO[]>([]);

  const cargar = useCallback(async () => {
    setEstado('cargando');
    try {
      const res = await fetch(ruta(`/api/sites/${siteId}/seo`));
      if (!res.ok) throw new Error(String(res.status));
      const d = (await res.json()) as { issues?: AvisoSEO[] };
      setAvisos(d.issues ?? []);
      setEstado('ok');
    } catch {
      setEstado('error');
    }
  }, [siteId]);

  useEffect(() => {
    void cargar();
  }, [cargar]);

  const texto = (a: AvisoSEO) => {
    const clave = TEXTO[a.code];
    return clave ? t(clave, { count: a.count ?? 0 }) : a.code;
  };
  const avisosReales = avisos.filter((a) => a.severity === 'warning').length;

  return (
    <section aria-labelledby="seo-titulo" class="mt-6">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <h3 id="seo-titulo" class="sytue-h4 text-text">
          {t('seo.title')}
        </h3>
        <div class="flex flex-wrap gap-2">
          <Button size="sm" variant="ghost" href={ruta(`/edit/${siteId}`)}>
            {t('seo.open_editor')}
          </Button>
          <Button
            size="sm"
            variant="secondary"
            disabled={estado === 'cargando'}
            onClick={() => {
              void cargar();
            }}
          >
            {t('seo.recheck')}
          </Button>
        </div>
      </div>
      <p class="text-text-muted mt-1 text-sm">{t('seo.intro')}</p>
      {estado === 'cargando' ? (
        <Skeleton height="4rem" class="mt-3 rounded-lg" />
      ) : estado === 'error' ? (
        <p class="text-danger-text mt-3 text-sm" role="alert">
          {t('seo.error')}
        </p>
      ) : avisos.length === 0 ? (
        <p class="text-success-text mt-3 text-sm font-medium">{t('seo.all_good')}</p>
      ) : (
        <>
          <p class="text-text-muted mt-3 text-sm" aria-live="polite">
            {t('seo.summary', { count: avisosReales })}
          </p>
          <ul class="border-border divide-border mt-2 divide-y rounded-lg border">
            {avisos.map((a, i) => (
              <li
                key={`${a.code}-${a.page_id ?? ''}-${a.lang ?? ''}-${i}`}
                class="flex flex-wrap items-center gap-3 p-3"
              >
                <Badge tone={a.severity === 'warning' ? 'warning' : 'info'}>
                  {t(a.severity === 'warning' ? 'seo.badge_warning' : 'seo.badge_info')}
                </Badge>
                <div class="min-w-0 flex-1">
                  <p class="text-text text-sm">{texto(a)}</p>
                  {a.page || a.lang ? (
                    <p class="text-text-muted text-xs">
                      {a.page ? t('seo.on_page', { page: a.page }) : ''}
                      {a.page && a.lang ? ' · ' : ''}
                      {a.lang ? a.lang.toUpperCase() : ''}
                    </p>
                  ) : null}
                </div>
              </li>
            ))}
          </ul>
        </>
      )}
    </section>
  );
}

export default RevisionSEO;
