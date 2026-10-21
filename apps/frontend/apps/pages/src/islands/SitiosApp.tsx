import type { JSX } from 'preact';
import { useState, useEffect, useMemo, useCallback } from 'preact/hooks';
import { Button, EmptyState, Badge, Skeleton } from '@sytue/ui';
import { formatDate, type Locale } from '@sytue/i18n';
import type { SytueUser } from '@sytue/auth';
import { Marco } from '../components/Marco.js';
import { t as translate, type ClaveMensaje } from '../i18n/index.js';
import { ruta } from '../lib/rutas.js';
import { administra } from '../lib/roles.js';

interface Props {
  locale: Locale;
  user: SytueUser;
  hubBaseUrl: string;
}

interface Sitio {
  id: string;
  slug: string;
  name: string;
  template: string;
  status: string;
  published_number?: number | null;
  canonical_host?: string | null;
  updated_at: string;
  deleted_at?: string | null;
}

type Filtro = 'all' | 'published' | 'draft';

const DIA_MS = 24 * 60 * 60 * 1000;

export function SitiosApp({ locale, user, hubBaseUrl }: Props): JSX.Element {
  const t = (k: ClaveMensaje, params?: Record<string, string | number>) =>
    translate(locale, k, params);

  const [sitios, setSitios] = useState<Sitio[]>([]);
  const [cargando, setCargando] = useState(true);
  const [errorCarga, setErrorCarga] = useState(false);
  const [filtro, setFiltro] = useState<Filtro>('all');
  const [restaurando, setRestaurando] = useState<string | null>(null);
  const [errorRestaurar, setErrorRestaurar] = useState(false);

  const cargarSitios = useCallback(async () => {
    setCargando(true);
    setErrorCarga(false);
    try {
      const res = await fetch(ruta('/api/sites?include_deleted=true'));
      if (!res.ok) throw new Error(String(res.status));
      setSitios((await res.json()) as Sitio[]);
    } catch {
      setErrorCarga(true);
    } finally {
      setCargando(false);
    }
  }, []);

  useEffect(() => {
    void cargarSitios();
  }, [cargarSitios]);

  const restaurarSitio = async (id: string) => {
    setRestaurando(id);
    setErrorRestaurar(false);
    try {
      const res = await fetch(ruta(`/api/sites/${id}/restore`), { method: 'POST' });
      if (!res.ok) throw new Error(String(res.status));
      await cargarSitios();
    } catch {
      setErrorRestaurar(true);
    } finally {
      setRestaurando(null);
    }
  };

  const activos = sitios.filter((s) => !s.deleted_at);
  const eliminados = sitios.filter((s) => s.deleted_at);

  const filtrados = activos.filter((s) => {
    if (filtro === 'published') return s.published_number != null;
    if (filtro === 'draft') return s.published_number == null;
    return true;
  });

  const ahora = useMemo(() => Date.now(), []);
  const diasRestantes = (borradoEn: string) =>
    Math.max(0, 30 - Math.floor((ahora - new Date(borradoEn).getTime()) / DIA_MS));

  const filtros: readonly [Filtro, ClaveMensaje][] = [
    ['all', 'sites.filter_all'],
    ['published', 'sites.filter_published'],
    ['draft', 'sites.filter_draft'],
  ];

  const botonCrear = (
    <Button variant="primary" href={ruta('/new')}>
      {t('sites.create')}
    </Button>
  );

  return (
    <Marco user={user} hubBaseUrl={hubBaseUrl} locale={locale} activa="sites">
      <div class="mx-auto max-w-6xl p-4 sm:p-6">
        <div class="mb-6 flex flex-wrap items-center justify-between gap-4">
          <div class="min-w-0">
            <h1 class="sytue-h2 text-text">{t('sites.title')}</h1>
            {!cargando && !errorCarga ? (
              <p class="text-text-muted mt-1 text-sm">
                {t('sites.count', { count: activos.length })}
              </p>
            ) : null}
          </div>
          {botonCrear}
        </div>

        {activos.length > 0 ? (
          <div class="mb-6 flex flex-wrap gap-2" role="group" aria-label={t('sites.filter_label')}>
            {filtros.map(([clave, rotulo]) => (
              <button
                key={clave}
                type="button"
                aria-pressed={filtro === clave}
                onClick={() => setFiltro(clave)}
                class={`sytue-focus-ring rounded-control px-3 py-1.5 text-sm font-medium transition-colors ${
                  filtro === clave
                    ? 'bg-selected text-primary-text font-semibold'
                    : 'text-text-muted hover:bg-bg-subtle'
                }`}
              >
                {t(rotulo)}
              </button>
            ))}
          </div>
        ) : null}

        {cargando ? (
          <div
            class="grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-3"
            aria-busy="true"
            aria-label={t('sites.loading')}
          >
            {[1, 2, 3].map((n) => (
              <Skeleton key={n} height="14rem" class="rounded-xl" />
            ))}
          </div>
        ) : errorCarga ? (
          <EmptyState
            title={t('sites.load_error')}
            description={t('sites.load_error_help')}
            action={
              <Button
                variant="secondary"
                onClick={() => {
                  void cargarSitios();
                }}
              >
                {t('sites.retry')}
              </Button>
            }
          />
        ) : activos.length === 0 ? (
          <EmptyState
            title={t('sites.empty')}
            description={t('sites.empty_help')}
            action={botonCrear}
          />
        ) : filtrados.length === 0 ? (
          <p class="text-text-muted py-8 text-center text-sm">{t('sites.filter_empty')}</p>
        ) : (
          <ul class="grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-3">
            {filtrados.map((sitio) => (
              <li
                key={sitio.id}
                class="pages-site-card border-border flex min-w-0 flex-col overflow-hidden rounded-xl border"
              >
                {/* Sin miniatura todavía: la inicial del sitio sobre el color de su tarjeta. */}
                <div
                  aria-hidden="true"
                  class="pages-site-preview border-border sytue-h1 relative flex h-32 w-full items-center justify-center border-b"
                >
                  {sitio.name.trim().charAt(0).toUpperCase()}
                  <div class="absolute top-3 right-3">
                    {sitio.published_number != null ? (
                      <Badge tone="success">
                        {t('sites.status.published')} (v{sitio.published_number})
                      </Badge>
                    ) : (
                      <Badge tone="neutral">{t('sites.status.draft')}</Badge>
                    )}
                  </div>
                </div>

                <div class="flex flex-1 flex-col justify-between p-4">
                  <div class="min-w-0">
                    <h2 class="sytue-h4 text-text truncate">{sitio.name}</h2>
                    <p class="text-text-muted mt-0.5 truncate text-xs">
                      {sitio.canonical_host || sitio.slug}
                    </p>
                    <p class="text-text-muted mt-2 text-xs">
                      {t('sites.last_updated', { fecha: formatDate(locale, sitio.updated_at) })}
                    </p>
                  </div>

                  <div class="border-border mt-4 flex items-center justify-between gap-2 border-t pt-3">
                    <Button variant="secondary" size="sm" href={ruta(`/site/${sitio.id}`)}>
                      {t('sites.manage')}
                    </Button>
                    <Button variant="primary" size="sm" href={ruta(`/edit/${sitio.id}`)}>
                      {t('sites.open_editor')}
                    </Button>
                  </div>
                </div>
              </li>
            ))}
          </ul>
        )}

        {eliminados.length > 0 ? (
          <section
            aria-labelledby="papelera-titulo"
            class="border-border bg-surface mt-12 rounded-xl border p-6"
          >
            <h2 id="papelera-titulo" class="sytue-h4 text-text">
              {t('sites.trash')}
            </h2>
            <p class="text-text-muted mt-1 text-sm">{t('sites.trash_help')}</p>
            {errorRestaurar ? (
              <p role="alert" class="text-danger-text mt-3 text-sm">
                {t('sites.restore_error')}
              </p>
            ) : null}

            <ul class="divide-border mt-4 divide-y">
              {eliminados.map((sitio) => {
                const dias = diasRestantes(sitio.deleted_at ?? '');
                return (
                  <li key={sitio.id} class="flex flex-wrap items-center justify-between gap-4 py-3">
                    <div class="min-w-0">
                      <p class="text-text font-semibold">{sitio.name}</p>
                      <p class="text-text-muted text-xs">
                        {sitio.slug} · {t('sites.days_left', { count: dias })}
                      </p>
                    </div>
                    {administra(user) ? (
                      <Button
                        variant="secondary"
                        size="sm"
                        loading={restaurando === sitio.id}
                        disabled={restaurando !== null}
                        onClick={() => {
                          void restaurarSitio(sitio.id);
                        }}
                      >
                        {t('sites.restore')}
                      </Button>
                    ) : null}
                  </li>
                );
              })}
            </ul>
          </section>
        ) : null}
      </div>
    </Marco>
  );
}

export default SitiosApp;
