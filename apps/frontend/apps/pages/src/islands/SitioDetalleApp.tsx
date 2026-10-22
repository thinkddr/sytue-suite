import type { JSX } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';
import {
  Badge,
  Button,
  Checkbox,
  ConfirmDialog,
  EmptyState,
  FieldGroup,
  Input,
  Skeleton,
  Tabs,
  Textarea,
  type BadgeTone,
} from '@sytue/ui';
import { formatDateTime, formatList, formatNumber, type Locale } from '@sytue/i18n';
import type { SytueUser } from '@sytue/auth';
import { Marco } from '../components/Marco.js';
import { Aviso, Seccion } from '../components/FichaUI.js';
import { t as translate, type ClaveMensaje } from '../i18n/index.js';
import { Tienda } from '../tienda/Tienda.js';
import { Pedidos } from '../tienda/Pedidos.js';
import { Bandeja } from '../formularios/Bandeja.js';
import { ruta } from '../lib/rutas.js';
import { administra } from '../lib/roles.js';
import { detalleDominio, progresoDominio, type DominioSitio } from '../lib/dominios.js';
import { direccionDelRender } from '../lib/direccion.js';
import {
  legalFaltante,
  exigeRegistro,
  ejecutarPublicacion,
  leerFallo,
  lineas,
  mensajeDeAjustes,
  mensajeDeDominio,
  mensajeDePublicacion,
  primerEsquemaInvalido,
  primerOrigenInvalido,
  type CampoLegal,
  type DatosLegales,
  type FalloApi,
} from '../lib/ficha.js';

interface Props {
  siteId: string;
  locale: Locale;
  user: SytueUser;
  hubBaseUrl: string;
}

type T = (k: ClaveMensaje, params?: Record<string, string | number>) => string;

const PESTANAS = [
  'publicar',
  'versiones',
  'dominios',
  'tienda',
  'pedidos',
  'formularios',
  'analitica',
  'exportar',
  'legal',
  'ajustes',
] as const;
type Pestana = (typeof PESTANAS)[number];
const esPestana = (v: string): v is Pestana => (PESTANAS as readonly string[]).includes(v);

interface VersionSitio {
  id: string;
  name: string;
  kind: string;
  draft_rev: number;
  created_by?: string;
  created_at: string;
}

interface Sitio {
  id: string;
  slug: string;
  name: string;
  template: string;
  status: string;
  draft_rev: number;
  published_number?: number | null;
  canonical_host?: string | null;
  legal?: DatosLegales;
  analytics_enabled: boolean;
  analytics_site_id?: string;
  export_origins?: string[];
  export_app_schemes?: string[];
}

const comoVersiones = (valor: unknown): VersionSitio[] | null =>
  Array.isArray(valor) ? (valor as VersionSitio[]) : null;

function comoDominios(valor: unknown): DominioSitio[] | null {
  if (!Array.isArray(valor)) return null;
  const validos = valor.every((dominio) => {
    if (!dominio || typeof dominio !== 'object') return false;
    const d = dominio as Record<string, unknown>;
    return (
      typeof d.id === 'string' &&
      typeof d.host === 'string' &&
      typeof d.status === 'string' &&
      typeof d.primary === 'boolean'
    );
  });
  return validos ? (valor as DominioSitio[]) : null;
}

function comoResumen(valor: unknown): { pageviews: number; visitors: number } | null {
  if (!valor || typeof valor !== 'object') return null;
  const totales = (valor as { totals?: { pageviews?: number; visitors?: number } }).totals;
  return { pageviews: totales?.pageviews ?? 0, visitors: totales?.visitors ?? 0 };
}

/** Carga un GET con sus tres estados y el motivo del fallo; `cargar(true)` refresca sin parpadear. */
function useCarga<D>(url: string, leer: (json: unknown) => D | null) {
  const [estado, setEstado] = useState<'cargando' | 'ok' | 'error'>('cargando');
  const [datos, setDatos] = useState<D | null>(null);
  const [fallo, setFallo] = useState<FalloApi | null>(null);

  const cargar = useCallback(
    async (silencioso = false) => {
      if (!silencioso) setEstado('cargando');
      try {
        const res = await fetch(ruta(url));
        if (!res.ok) {
          setFallo(await leerFallo(res));
          setEstado('error');
          return;
        }
        const leido = leer((await res.json()) as unknown);
        if (leido === null) {
          setFallo({ status: res.status, code: '' });
          setEstado('error');
          return;
        }
        setDatos(leido);
        setEstado('ok');
      } catch {
        setFallo({ status: 0, code: '' });
        setEstado('error');
      }
    },
    // Las lectoras del módulo conservan su referencia entre renders.
    [url, leer],
  );

  useEffect(() => {
    void cargar();
  }, [cargar]);

  return { estado, datos, fallo, cargar };
}

/** Un `PUT /sites/{id}` con el cuerpo dado: el sitio actualizado, o el motivo del fallo. */
async function guardarSitio(
  siteId: string,
  cuerpo: Record<string, unknown>,
): Promise<{ sitio: Sitio } | { fallo: FalloApi }> {
  try {
    const res = await fetch(ruta(`/api/sites/${siteId}`), {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(cuerpo),
    });
    if (!res.ok) return { fallo: await leerFallo(res) };
    return { sitio: (await res.json()) as Sitio };
  } catch {
    return { fallo: { status: 0, code: '' } };
  }
}

type Resultado = { tono: 'exito' | 'error' | 'info'; texto: string };

/* ------------------------------------------------------------------------------------------ */
/* Publicar                                                                                    */
/* ------------------------------------------------------------------------------------------ */

const NOMBRE_CAMPO: Record<CampoLegal, ClaveMensaje> = {
  holder_name: 'legal.holder_name',
  tax_id: 'legal.tax_id',
  country: 'legal.country',
  street: 'legal.street',
  postal_code: 'legal.postal_code',
  city: 'legal.city',
  email: 'legal.email',
  registry: 'legal.registry',
};

function Publicar({
  siteId,
  sitio,
  locale,
  t,
  irA,
  onCambio,
}: {
  siteId: string;
  sitio: Sitio;
  locale: Locale;
  t: T;
  irA: (p: Pestana) => void;
  onCambio: () => void;
}): JSX.Element {
  const [ocupado, setOcupado] = useState<'preview' | 'live' | null>(null);
  const [resultado, setResultado] = useState<
    (Resultado & { url?: string; enlace?: ClaveMensaje }) | null
  >(null);
  const [revisados, setRevisados] = useState<ReadonlySet<'demo' | 'ir'>>(new Set());

  const faltan = legalFaltante(sitio.legal);
  const legalListo = faltan.length === 0;
  const publicado = sitio.published_number != null;

  const publicar = async (tipo: 'preview' | 'live') => {
    setOcupado(tipo);
    setResultado(null);
    const intento = await ejecutarPublicacion(siteId, tipo);
    setOcupado(null);
    if (intento.ok) {
      setRevisados(new Set());
      const url = intento.url;
      setResultado({
        tono: 'exito',
        texto: t(tipo === 'live' ? 'publish.done_live' : 'publish.done_preview'),
        enlace: tipo === 'live' ? 'ficha.open_site' : 'publish.open_preview',
        ...(url ? { url } : {}),
      });
      onCambio();
      return;
    }
    // Lo que el servidor rechaza y la ficha no puede saber de antemano se suma a la lista.
    if (intento.fallo.code === 'demo_content') setRevisados(new Set(['demo']));
    if (intento.fallo.code === 'ir_invalid') setRevisados(new Set(['ir']));
    setResultado({ tono: 'error', texto: t(mensajeDePublicacion(intento.fallo)) });
  };

  const fila = (id: string, listo: boolean, rotulo: string, extra?: JSX.Element) => (
    <li key={id} class="flex flex-wrap items-center gap-3 p-3">
      <Badge tone={listo ? 'success' : 'warning'}>
        {t(listo ? 'publish.check_ok' : 'publish.check_missing')}
      </Badge>
      <div class="min-w-0 flex-1">
        <p class="text-text text-sm font-medium">{rotulo}</p>
        {extra}
      </div>
    </li>
  );

  return (
    <Seccion titulo={t('publish.title')} descripcion={t('publish.intro')}>
      <div class="flex flex-wrap items-center gap-2">
        <span class="text-text-muted text-sm">{t('publish.status')}</span>
        {publicado ? (
          <Badge tone="success">
            {t('sites.status.published')} (v{sitio.published_number})
          </Badge>
        ) : (
          <Badge tone="neutral">{t('sites.status.draft')}</Badge>
        )}
      </div>

      <h3 class="sytue-h4 text-text mt-6">{t('publish.checklist')}</h3>
      <ul class="border-border divide-border mt-3 divide-y rounded-lg border">
        {fila(
          'legal',
          legalListo,
          t('publish.check_legal'),
          legalListo ? undefined : (
            <p class="text-text-muted text-xs">
              {t('publish.check_legal_missing', {
                campos: formatList(
                  locale,
                  faltan.map((c) => t(NOMBRE_CAMPO[c]).toLowerCase()),
                ),
              })}
            </p>
          ),
        )}
        {revisados.has('demo') ? fila('demo', false, t('publish.check_demo')) : null}
        {revisados.has('ir') ? fila('ir', false, t('publish.check_ir')) : null}
      </ul>
      {!legalListo ? (
        <Button
          class="mt-3"
          size="sm"
          variant="secondary"
          onClick={() => {
            irA('legal');
          }}
        >
          {t('publish.check_legal_fix')}
        </Button>
      ) : null}
      <p class="text-text-muted mt-3 text-xs">{t('publish.check_server')}</p>

      {resultado ? (
        <Aviso tono={resultado.tono} class="mt-5">
          {resultado.texto}
          {resultado.url ? (
            <>
              {' '}
              <a
                class="sytue-focus-ring rounded-sm font-semibold underline"
                href={resultado.url}
                target="_blank"
                rel="noopener noreferrer"
              >
                {t(resultado.enlace ?? 'ficha.open_site')}
              </a>
            </>
          ) : null}
        </Aviso>
      ) : null}

      <div class="mt-5 flex flex-col gap-3 sm:flex-row">
        <Button
          variant="secondary"
          loading={ocupado === 'preview'}
          disabled={ocupado !== null}
          onClick={() => {
            void publicar('preview');
          }}
        >
          {t('publish.preview')}
        </Button>
        <Button
          loading={ocupado === 'live'}
          disabled={ocupado !== null}
          onClick={() => {
            void publicar('live');
          }}
        >
          {t('publish.action')}
        </Button>
      </div>
    </Seccion>
  );
}

/* ------------------------------------------------------------------------------------------ */
/* Versiones                                                                                   */
/* ------------------------------------------------------------------------------------------ */

const TIPOS_VERSION: Record<string, ClaveMensaje> = {
  manual: 'versions.kind.manual',
  publish: 'versions.kind.publish',
  preview: 'versions.kind.preview',
  export: 'versions.kind.export',
  before_restore: 'versions.kind.before_restore',
};

function Versiones({
  siteId,
  locale,
  t,
  onCambio,
}: {
  siteId: string;
  locale: Locale;
  t: T;
  onCambio: () => void;
}): JSX.Element {
  const { estado, datos, cargar } = useCarga(`/api/sites/${siteId}/versions`, comoVersiones);
  const [nombre, setNombre] = useState('');
  const [creando, setCreando] = useState(false);
  const [aviso, setAviso] = useState<Resultado | null>(null);
  const [aRestaurar, setARestaurar] = useState<VersionSitio | null>(null);
  const [restaurando, setRestaurando] = useState(false);

  const nombreDe = (v: VersionSitio) =>
    v.kind === 'before_restore'
      ? t('versions.kind.before_restore')
      : v.name || t('versions.unnamed');

  const crear = async (event: Event) => {
    event.preventDefault();
    setCreando(true);
    setAviso(null);
    try {
      const res = await fetch(ruta(`/api/sites/${siteId}/versions`), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: nombre.trim() || t('versions.manual_name') }),
      });
      if (!res.ok) throw new Error(String(res.status));
      setNombre('');
      setAviso({ tono: 'exito', texto: t('versions.created') });
      await cargar(true);
    } catch {
      setAviso({ tono: 'error', texto: t('versions.create_error') });
    } finally {
      setCreando(false);
    }
  };

  const restaurar = async () => {
    if (!aRestaurar) return;
    setRestaurando(true);
    setAviso(null);
    try {
      const res = await fetch(ruta(`/api/sites/${siteId}/versions/${aRestaurar.id}/restore`), {
        method: 'POST',
      });
      if (!res.ok) throw new Error(String(res.status));
      setAviso({ tono: 'exito', texto: t('versions.restored') });
      await cargar(true);
      onCambio();
    } catch {
      setAviso({ tono: 'error', texto: t('versions.restore_error') });
    } finally {
      setRestaurando(false);
      setARestaurar(null);
    }
  };

  return (
    <Seccion titulo={t('versions.title')} descripcion={t('versions.intro')}>
      <form
        onSubmit={(e) => {
          void crear(e);
        }}
      >
        <FieldGroup label={t('versions.name')} htmlFor="version-nombre">
          <div class="flex flex-col gap-2 sm:flex-row">
            <Input
              id="version-nombre"
              class="min-w-0 flex-1"
              value={nombre}
              maxLength={120}
              placeholder={t('versions.name_placeholder')}
              onInput={(e) => setNombre(e.currentTarget.value)}
            />
            <Button type="submit" variant="secondary" loading={creando}>
              {t('versions.create')}
            </Button>
          </div>
        </FieldGroup>
      </form>

      {aviso ? (
        <Aviso tono={aviso.tono} class="mt-4">
          {aviso.texto}
        </Aviso>
      ) : null}

      <div class="mt-6">
        {estado === 'cargando' ? (
          <div class="space-y-3" aria-busy="true" aria-label={t('versions.loading')}>
            {[1, 2, 3].map((n) => (
              <Skeleton key={n} height="3.5rem" class="rounded-lg" />
            ))}
          </div>
        ) : estado === 'error' ? (
          <EmptyState
            title={t('versions.load_error')}
            description={t('sites.load_error_help')}
            action={
              <Button
                variant="secondary"
                onClick={() => {
                  void cargar();
                }}
              >
                {t('sites.retry')}
              </Button>
            }
          />
        ) : datos && datos.length > 0 ? (
          <ul class="divide-border divide-y">
            {datos.map((v) => {
              const clave = TIPOS_VERSION[v.kind];
              return (
                <li key={v.id} class="flex flex-wrap items-center justify-between gap-3 py-3">
                  <div class="min-w-0">
                    <p class="flex flex-wrap items-center gap-2">
                      <span class="text-text font-semibold break-words">{nombreDe(v)}</span>
                      <Badge tone="neutral">{clave ? t(clave) : v.kind}</Badge>
                    </p>
                    <p class="text-text-muted mt-0.5 text-xs">
                      {t('versions.rev')} {v.draft_rev} · {formatDateTime(locale, v.created_at)}
                    </p>
                  </div>
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() => {
                      setARestaurar(v);
                    }}
                  >
                    {t('sites.restore')}
                  </Button>
                </li>
              );
            })}
          </ul>
        ) : (
          <EmptyState title={t('versions.empty')} description={t('versions.empty_help')} />
        )}
      </div>

      <ConfirmDialog
        open={aRestaurar !== null}
        onClose={() => {
          if (!restaurando) setARestaurar(null);
        }}
        onConfirm={() => {
          void restaurar();
        }}
        title={t('versions.restore')}
        confirmLabel={t('sites.restore')}
        busy={restaurando}
      >
        <p class="text-text text-sm">
          {t('versions.restore_confirm', { nombre: aRestaurar ? nombreDe(aRestaurar) : '' })}
        </p>
      </ConfirmDialog>
    </Seccion>
  );
}

/* ------------------------------------------------------------------------------------------ */
/* Dominios                                                                                    */
/* ------------------------------------------------------------------------------------------ */

const ESTADOS_DOMINIO: Record<string, { clave: ClaveMensaje; tono: BadgeTone }> = {
  pending: { clave: 'domains.status.pending', tono: 'warning' },
  verified: { clave: 'domains.status.verified', tono: 'info' },
  active: { clave: 'domains.status.active', tono: 'success' },
  failed: { clave: 'domains.status.failed', tono: 'danger' },
};

export function Dominios({
  siteId,
  locale,
  t,
  onPublicar,
  onCambio,
}: {
  siteId: string;
  locale: Locale;
  t: T;
  onPublicar: () => void;
  onCambio: () => void;
}): JSX.Element {
  const { estado, datos, fallo, cargar } = useCarga(`/api/sites/${siteId}/domains`, comoDominios);
  const [nuevo, setNuevo] = useState('');
  const [anadiendo, setAnadiendo] = useState(false);
  const [errorAlta, setErrorAlta] = useState<string | null>(null);
  const [aviso, setAviso] = useState<Resultado | null>(null);
  const [enCurso, setEnCurso] = useState<string | null>(null);
  const [aEliminar, setAEliminar] = useState<DominioSitio | null>(null);
  const [copiado, setCopiado] = useState<string | null>(null);

  const alta = async (event: Event) => {
    event.preventDefault();
    const host = nuevo.trim().toLowerCase();
    if (!host) return;
    setAnadiendo(true);
    setErrorAlta(null);
    setAviso(null);
    try {
      const res = await fetch(ruta(`/api/sites/${siteId}/domains`), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ host }),
      });
      if (!res.ok) {
        setErrorAlta(t(mensajeDeDominio(await leerFallo(res))));
        return;
      }
      setNuevo('');
      setAviso({ tono: 'exito', texto: t('domains.added') });
      await cargar(true);
    } catch {
      setErrorAlta(t('domains.err_generic'));
    } finally {
      setAnadiendo(false);
    }
  };

  const actuar = async (dominio: DominioSitio, accion: 'check' | 'primary' | 'delete') => {
    setEnCurso(`${dominio.id}:${accion}`);
    setAviso(null);
    const sufijo = accion === 'delete' ? '' : `/${accion}`;
    try {
      const res = await fetch(
        ruta(`/api/sites/${siteId}/domains/${encodeURIComponent(dominio.id)}${sufijo}`),
        { method: accion === 'delete' ? 'DELETE' : 'POST' },
      );
      if (!res.ok) {
        setAviso({ tono: 'error', texto: t(mensajeDeDominio(await leerFallo(res))) });
        return;
      }
      if (accion === 'check') {
        const actualizado = (await res.json()) as DominioSitio;
        setAviso({
          tono: actualizado.status === 'active' && !actualizado.detail ? 'exito' : 'info',
          texto: t(detalleDominio(actualizado)),
        });
      } else {
        setAviso({ tono: 'exito', texto: t('domains.updated') });
      }
      await cargar(true);
      onCambio();
    } catch {
      setAviso({ tono: 'error', texto: t('domains.action_error') });
    } finally {
      setEnCurso(null);
      setAEliminar(null);
    }
  };

  const copiar = async (id: string, valor: string) => {
    try {
      await navigator.clipboard.writeText(valor);
      setCopiado(id);
    } catch {
      setAviso({ tono: 'info', texto: t('domains.copy_error') });
    }
  };

  const registro = (id: string, rotulo: string, valor: string | undefined) =>
    valor ? (
      <FieldGroup label={rotulo} htmlFor={id}>
        <div class="flex min-w-0 gap-2">
          <Input
            id={id}
            value={valor}
            readOnly
            class="min-w-0 flex-1 font-mono text-xs"
            onFocus={(e) => e.currentTarget.select()}
          />
          <Button
            type="button"
            size="sm"
            variant="ghost"
            aria-label={t('domains.copy_field', { campo: rotulo })}
            onClick={() => {
              void copiar(id, valor);
            }}
          >
            {t(copiado === id ? 'domains.copied' : 'domains.copy')}
          </Button>
        </div>
      </FieldGroup>
    ) : null;

  const pasos: readonly ClaveMensaje[] = [
    'domains.step.registered',
    'domains.step.dns',
    'domains.step.https',
    'domains.step.primary',
  ];

  return (
    <Seccion titulo={t('domains.title')} descripcion={t('domains.intro')}>
      {estado === 'error' && fallo?.status === 403 ? null : (
        <form
          onSubmit={(e) => {
            void alta(e);
          }}
        >
          <h3 class="text-text mb-1 font-semibold">{t('domains.start_title')}</h3>
          <p class="text-text-muted mb-4 text-sm">{t('domains.start_help')}</p>
          <FieldGroup
            label={t('domains.host_label')}
            htmlFor="nuevo-dominio"
            help={t('domains.host_help')}
            {...(errorAlta ? { error: errorAlta } : {})}
          >
            <div class="flex flex-col gap-2 sm:flex-row">
              <Input
                id="nuevo-dominio"
                class="min-w-0 flex-1"
                value={nuevo}
                name="host"
                required
                invalid={errorAlta !== null}
                placeholder={t('domains.host_placeholder')}
                autocomplete="off"
                autocapitalize="none"
                spellcheck={false}
                onInput={(e) => {
                  setNuevo(e.currentTarget.value);
                  setErrorAlta(null);
                }}
              />
              <Button type="submit" loading={anadiendo} disabled={anadiendo}>
                {t('domains.add')}
              </Button>
            </div>
          </FieldGroup>
        </form>
      )}

      {aviso ? (
        <Aviso tono={aviso.tono} class="mt-4">
          {aviso.texto}
        </Aviso>
      ) : null}

      <div class="mt-6">
        {estado === 'cargando' ? (
          <div class="space-y-3" aria-busy="true" aria-label={t('domains.loading')}>
            {[1, 2].map((n) => (
              <Skeleton key={n} height="7rem" class="rounded-lg" />
            ))}
          </div>
        ) : estado === 'error' ? (
          <EmptyState
            title={t('domains.load_error')}
            description={
              fallo?.status === 403 ? t('domains.err_forbidden') : t('sites.load_error_help')
            }
            action={
              <Button
                variant="secondary"
                onClick={() => {
                  void cargar();
                }}
              >
                {t('sites.retry')}
              </Button>
            }
          />
        ) : datos && datos.length > 0 ? (
          <ul class="space-y-4">
            {datos.map((dominio) => {
              const est = ESTADOS_DOMINIO[dominio.status];
              const progreso = progresoDominio(dominio);
              const pasoActual = progreso.findIndex((completado) => !completado);
              const target = dominio.target;
              const registroExacto =
                (target?.type === 'A' || target?.type === 'CNAME') &&
                !!target.name &&
                !!target.value;
              return (
                <li key={dominio.id} class="border-border rounded-lg border p-4">
                  <div class="flex flex-wrap items-center gap-2">
                    <h3 class="text-text font-semibold break-all">{dominio.host}</h3>
                    {dominio.primary ? <Badge tone="primary">{t('domains.primary')}</Badge> : null}
                    <Badge tone={dominio.detail ? 'warning' : (est?.tono ?? 'neutral')}>
                      {dominio.detail && dominio.status === 'active'
                        ? t('domains.status.review')
                        : est
                          ? t(est.clave)
                          : dominio.status}
                    </Badge>
                  </div>
                  <p class="text-text-muted mt-2 text-sm">{t(detalleDominio(dominio))}</p>
                  {dominio.checked_at ? (
                    <p class="text-text-muted mt-1 text-xs">
                      {t('domains.last_check', {
                        fecha: formatDateTime(locale, dominio.checked_at),
                      })}
                    </p>
                  ) : null}

                  <ol aria-label={t('domains.progress')} class="mt-4 grid gap-2 sm:grid-cols-4">
                    {pasos.map((clave, indice) => (
                      <li
                        key={clave}
                        aria-current={indice === pasoActual ? 'step' : undefined}
                        class={`rounded-md px-3 py-2 text-xs ${progreso[indice] ? 'bg-success-bg text-success-text' : indice === pasoActual ? 'bg-selected text-primary-text' : 'bg-bg-subtle text-text-muted'}`}
                      >
                        <span aria-hidden="true" class="mr-1.5 font-semibold">
                          {progreso[indice] ? '✓' : indice + 1}
                        </span>
                        {t(clave)}
                        <span class="sr-only">
                          {' '}
                          · {t(progreso[indice] ? 'domains.step.done' : 'domains.step.pending')}
                        </span>
                      </li>
                    ))}
                  </ol>

                  <details class="border-border mt-4 border-t pt-3" open={!progreso[1]}>
                    <summary class="text-text cursor-pointer text-sm font-semibold">
                      {t('domains.dns_title')}
                    </summary>
                    <p class="text-text-muted mt-2 text-sm">{t('domains.dns_help')}</p>
                    <div class="mt-4 grid gap-4 lg:grid-cols-2">
                      <fieldset class="border-border min-w-0 space-y-3 rounded-lg border p-3">
                        <legend class="text-text px-1 text-sm font-semibold">
                          {t('domains.dns_txt')}
                        </legend>
                        {registro(
                          `${dominio.id}-txt-name`,
                          t('domains.record_name'),
                          dominio.txt?.name,
                        )}
                        {registro(
                          `${dominio.id}-txt-value`,
                          t('domains.record_value'),
                          dominio.txt?.value,
                        )}
                        {!dominio.txt?.name || !dominio.txt?.value ? (
                          <p class="text-warning-text text-sm">
                            {t('domains.records_unavailable')}
                          </p>
                        ) : null}
                      </fieldset>
                      <fieldset class="border-border min-w-0 space-y-3 rounded-lg border p-3">
                        <legend class="text-text px-1 text-sm font-semibold">
                          {t('domains.routing_title')}
                          {registroExacto ? ` · ${target.type}` : ''}
                        </legend>
                        {registroExacto ? (
                          <>
                            {registro(
                              `${dominio.id}-target-name`,
                              t('domains.record_name'),
                              target.name,
                            )}
                            {registro(
                              `${dominio.id}-target-value`,
                              t('domains.record_value'),
                              target.value,
                            )}
                          </>
                        ) : (
                          <>
                            <p class="text-text-muted text-sm">
                              {t('domains.routing_legacy_help')}
                            </p>
                            {registro(`${dominio.id}-cname`, t('domains.dns_cname'), target?.cname)}
                            {registro(`${dominio.id}-a`, t('domains.dns_a'), target?.a)}
                            {!target?.cname && !target?.a ? (
                              <p class="text-warning-text text-sm">
                                {t('domains.records_unavailable')}
                              </p>
                            ) : null}
                          </>
                        )}
                      </fieldset>
                    </div>
                    <p class="text-text-muted mt-3 text-xs">{t('domains.dns_names_help')}</p>
                    <p class="text-text-muted mt-2 text-xs">{t('domains.dns_propagation')}</p>
                  </details>

                  <div class="mt-4 flex flex-wrap gap-2">
                    <Button
                      size="sm"
                      variant="secondary"
                      loading={enCurso === `${dominio.id}:check`}
                      disabled={enCurso !== null}
                      onClick={() => {
                        void actuar(dominio, 'check');
                      }}
                    >
                      {t('domains.check')}
                    </Button>
                    {!dominio.primary && progreso[2] ? (
                      <Button
                        size="sm"
                        variant="secondary"
                        loading={enCurso === `${dominio.id}:primary`}
                        disabled={enCurso !== null}
                        onClick={() => {
                          void actuar(dominio, 'primary');
                        }}
                      >
                        {t('domains.make_primary')}
                      </Button>
                    ) : null}
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={enCurso !== null}
                      onClick={() => {
                        setAEliminar(dominio);
                      }}
                    >
                      {t('domains.remove')}
                    </Button>
                  </div>
                  {progreso[2] ? (
                    <div class="border-border mt-4 flex flex-wrap items-center justify-between gap-3 border-t pt-4">
                      <p class="text-text-muted max-w-xl text-sm">{t('domains.publish_help')}</p>
                      <Button size="sm" onClick={onPublicar}>
                        {t('domains.publish_action')}
                      </Button>
                    </div>
                  ) : null}
                </li>
              );
            })}
          </ul>
        ) : (
          <p class="text-text-muted text-sm">{t('domains.empty')}</p>
        )}
      </div>

      <ConfirmDialog
        open={aEliminar !== null}
        onClose={() => {
          if (enCurso === null) setAEliminar(null);
        }}
        onConfirm={() => {
          if (aEliminar) void actuar(aEliminar, 'delete');
        }}
        title={t('domains.remove_title')}
        confirmLabel={t('domains.remove')}
        tone="danger"
        busy={enCurso !== null}
      >
        <p class="text-text text-sm">
          {t('domains.remove_confirm', { host: aEliminar?.host ?? '' })}
        </p>
      </ConfirmDialog>
    </Seccion>
  );
}

/* ------------------------------------------------------------------------------------------ */
/* Analítica                                                                                   */
/* ------------------------------------------------------------------------------------------ */

function Resumen({ siteId, locale, t }: { siteId: string; locale: Locale; t: T }): JSX.Element {
  const { estado, datos, fallo, cargar } = useCarga(
    `/api/sites/${siteId}/analytics/summary`,
    comoResumen,
  );
  if (estado === 'cargando') {
    return (
      <div
        class="grid grid-cols-1 gap-4 sm:grid-cols-2"
        aria-busy="true"
        aria-label={t('analytics.loading')}
      >
        <Skeleton height="5.5rem" class="rounded-lg" />
        <Skeleton height="5.5rem" class="rounded-lg" />
      </div>
    );
  }
  if (estado === 'error' || !datos) {
    return (
      <EmptyState
        title={t('analytics.load_error')}
        description={t(fallo?.status === 503 ? 'analytics.unavailable' : 'sites.load_error_help')}
        action={
          <Button
            variant="secondary"
            onClick={() => {
              void cargar();
            }}
          >
            {t('sites.retry')}
          </Button>
        }
      />
    );
  }
  const tarjeta = (rotulo: string, valor: number) => (
    <div class="bg-bg-subtle rounded-lg p-4">
      <dt class="text-text-muted text-sm">{rotulo}</dt>
      <dd class="sytue-h2 text-text mt-1 tabular-nums">{formatNumber(locale, valor)}</dd>
    </div>
  );
  return (
    <>
      <dl class="grid grid-cols-1 gap-4 sm:grid-cols-2">
        {tarjeta(t('analytics.visits'), datos.pageviews)}
        {tarjeta(t('analytics.visitors'), datos.visitors)}
      </dl>
      <Button
        class="mt-4"
        size="sm"
        variant="secondary"
        onClick={() => {
          void cargar(true);
        }}
      >
        {t('analytics.refresh')}
      </Button>
    </>
  );
}

function Analitica({
  siteId,
  sitio,
  locale,
  t,
  irA,
}: {
  siteId: string;
  sitio: Sitio;
  locale: Locale;
  t: T;
  irA: (p: Pestana) => void;
}): JSX.Element {
  return (
    <Seccion titulo={t('analytics.title')} descripcion={t('analytics.intro')}>
      {sitio.analytics_enabled ? (
        <Resumen siteId={siteId} locale={locale} t={t} />
      ) : (
        <EmptyState
          title={t('analytics.disabled')}
          description={t('analytics.disabled_help')}
          action={
            <Button
              variant="secondary"
              onClick={() => {
                irA('ajustes');
              }}
            >
              {t('analytics.go_settings')}
            </Button>
          }
        />
      )}
    </Seccion>
  );
}

/* ------------------------------------------------------------------------------------------ */
/* Exportar                                                                                    */
/* ------------------------------------------------------------------------------------------ */

function Exportar({
  siteId,
  sitio,
  t,
  onSitio,
}: {
  siteId: string;
  sitio: Sitio;
  t: T;
  onSitio: (s: Sitio) => void;
}): JSX.Element {
  const [origenes, setOrigenes] = useState((sitio.export_origins ?? []).join('\n'));
  const [esquemas, setEsquemas] = useState((sitio.export_app_schemes ?? []).join('\n'));
  const [errorOrigenes, setErrorOrigenes] = useState<string | null>(null);
  const [errorEsquemas, setErrorEsquemas] = useState<string | null>(null);
  const [guardando, setGuardando] = useState(false);
  const [aviso, setAviso] = useState<Resultado | null>(null);

  const guardar = async (event: Event) => {
    event.preventDefault();
    setAviso(null);
    const export_origins = lineas(origenes);
    const export_app_schemes = lineas(esquemas);
    const malOrigen = primerOrigenInvalido(export_origins);
    const malEsquema = primerEsquemaInvalido(export_app_schemes);
    setErrorOrigenes(malOrigen ? t('export.origins_invalid', { n: malOrigen }) : null);
    setErrorEsquemas(malEsquema ? t('export.schemes_invalid', { n: malEsquema }) : null);
    if (malOrigen || malEsquema) return;

    setGuardando(true);
    const res = await guardarSitio(siteId, { export_origins, export_app_schemes });
    setGuardando(false);
    if ('sitio' in res) {
      onSitio(res.sitio);
      setAviso({ tono: 'exito', texto: t('export.saved') });
    } else {
      setAviso({ tono: 'error', texto: t('export.err_generic') });
    }
  };

  return (
    <Seccion titulo={t('export.title')} descripcion={t('export.intro')} estrecha>
      <form
        class="space-y-5"
        onSubmit={(e) => {
          void guardar(e);
        }}
      >
        <FieldGroup
          label={t('export.origins_label')}
          htmlFor="export-origenes"
          help={t('export.origins_help')}
          {...(errorOrigenes ? { error: errorOrigenes } : {})}
        >
          <Textarea
            id="export-origenes"
            rows={3}
            value={origenes}
            invalid={errorOrigenes !== null}
            placeholder={t('export.origins_placeholder')}
            spellcheck={false}
            onInput={(e) => setOrigenes(e.currentTarget.value)}
          />
        </FieldGroup>
        <FieldGroup
          label={t('export.schemes_label')}
          htmlFor="export-esquemas"
          help={t('export.schemes_help')}
          {...(errorEsquemas ? { error: errorEsquemas } : {})}
        >
          <Textarea
            id="export-esquemas"
            rows={2}
            value={esquemas}
            invalid={errorEsquemas !== null}
            placeholder={t('export.schemes_placeholder')}
            spellcheck={false}
            onInput={(e) => setEsquemas(e.currentTarget.value)}
          />
        </FieldGroup>
        {aviso ? <Aviso tono={aviso.tono}>{aviso.texto}</Aviso> : null}
        <Button type="submit" loading={guardando}>
          {t('export.save')}
        </Button>
      </form>
    </Seccion>
  );
}

/* ------------------------------------------------------------------------------------------ */
/* Datos legales                                                                               */
/* ------------------------------------------------------------------------------------------ */

function Legal({
  siteId,
  sitio,
  locale,
  t,
  onSitio,
  administra: puedeCambiar,
}: {
  siteId: string;
  sitio: Sitio;
  locale: Locale;
  t: T;
  onSitio: (s: Sitio) => void;
  /** Los datos legales los cambia quien administra la organización: el servicio niega al resto. */
  administra: boolean;
}): JSX.Element {
  // El país se enseña como «ES» cuando está vacío: se guarda lo que se ve, no un vacío.
  const [legal, setLegal] = useState<DatosLegales>(() => ({
    ...sitio.legal,
    country: sitio.legal?.country || 'ES',
  }));
  const [guardando, setGuardando] = useState(false);
  const [aviso, setAviso] = useState<Resultado | null>(null);

  const campo = (k: Exclude<keyof DatosLegales, 'address'>) => ({
    value: legal[k] ?? '',
    onInput: (e: JSX.TargetedInputEvent<HTMLInputElement>) =>
      setLegal({ ...legal, [k]: e.currentTarget.value }),
  });
  const direccion = (k: 'street' | 'postal_code' | 'city' | 'region') => ({
    value: legal.address?.[k] ?? '',
    onInput: (e: JSX.TargetedInputEvent<HTMLInputElement>) =>
      setLegal({ ...legal, address: { ...legal.address, [k]: e.currentTarget.value } }),
  });

  const guardar = async (event: Event) => {
    event.preventDefault();
    setGuardando(true);
    setAviso(null);
    const res = await guardarSitio(siteId, { legal });
    setGuardando(false);
    if (!('sitio' in res)) {
      setAviso({ tono: 'error', texto: t('legal.err_generic') });
      return;
    }
    onSitio(res.sitio);
    const faltan = legalFaltante(res.sitio.legal);
    setAviso(
      faltan.length === 0
        ? { tono: 'exito', texto: t('legal.saved') }
        : {
            tono: 'info',
            texto: t('legal.saved_incomplete', {
              campos: formatList(
                locale,
                faltan.map((c) => t(NOMBRE_CAMPO[c]).toLowerCase()),
              ),
            }),
          },
    );
  };

  return (
    <Seccion titulo={t('legal.title')} descripcion={t('legal.intro')} estrecha>
      <form
        onSubmit={(e) => {
          void guardar(e);
        }}
      >
        <p class="text-text-muted mb-4 text-xs">{t('legal.required_note')}</p>
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <FieldGroup label={t('legal.holder_name')} htmlFor="legal-titular" required>
            <Input
              id="legal-titular"
              autocomplete="organization"
              placeholder={t('legal.ph_holder')}
              {...campo('holder_name')}
            />
          </FieldGroup>
          <FieldGroup label={t('legal.tax_id')} htmlFor="legal-nif" required>
            <Input id="legal-nif" placeholder="B12345678" {...campo('tax_id')} />
          </FieldGroup>
          <FieldGroup
            label={t('legal.country')}
            htmlFor="legal-pais"
            help={t('legal.country_help')}
            required
          >
            <Input
              id="legal-pais"
              maxLength={2}
              autocomplete="country"
              placeholder="ES"
              {...campo('country')}
            />
          </FieldGroup>
          <FieldGroup label={t('legal.email')} htmlFor="legal-correo" required>
            <Input
              id="legal-correo"
              type="email"
              autocomplete="email"
              placeholder={t('legal.ph_email')}
              {...campo('email')}
            />
          </FieldGroup>
          <FieldGroup
            label={t('legal.street')}
            htmlFor="legal-calle"
            required
            class="min-w-0 sm:col-span-2"
          >
            <Input
              id="legal-calle"
              autocomplete="street-address"
              placeholder={t('legal.ph_street')}
              {...direccion('street')}
            />
          </FieldGroup>
          <FieldGroup label={t('legal.postal_code')} htmlFor="legal-cp" required>
            <Input
              id="legal-cp"
              autocomplete="postal-code"
              placeholder="28001"
              {...direccion('postal_code')}
            />
          </FieldGroup>
          <FieldGroup label={t('legal.city')} htmlFor="legal-ciudad" required>
            <Input
              id="legal-ciudad"
              autocomplete="address-level2"
              placeholder={t('legal.ph_city')}
              {...direccion('city')}
            />
          </FieldGroup>
          <FieldGroup label={t('legal.region')} htmlFor="legal-region">
            <Input id="legal-region" autocomplete="address-level1" {...direccion('region')} />
          </FieldGroup>
          <FieldGroup label={t('legal.phone')} htmlFor="legal-telefono">
            <Input
              id="legal-telefono"
              type="tel"
              autocomplete="tel"
              placeholder="+34 912 345 678"
              {...campo('phone')}
            />
          </FieldGroup>
          <FieldGroup label={t('legal.privacy_contact')} htmlFor="legal-privacidad">
            <Input
              id="legal-privacidad"
              type="email"
              placeholder={t('legal.ph_privacy')}
              {...campo('privacy_contact')}
            />
          </FieldGroup>
          <FieldGroup
            label={t('legal.registry')}
            htmlFor="legal-registro"
            help={t('legal.registry_help')}
            required={exigeRegistro(legal)}
            class="min-w-0 sm:col-span-2"
          >
            <Input
              id="legal-registro"
              placeholder={t('legal.ph_registry')}
              {...campo('registry')}
            />
          </FieldGroup>
        </div>
        {aviso ? (
          <Aviso tono={aviso.tono} class="mt-5">
            {aviso.texto}
          </Aviso>
        ) : null}
        {puedeCambiar ? null : (
          <Aviso tono="info" class="mt-5">
            {t('settings.admin_only')}
          </Aviso>
        )}
        <Button type="submit" class="mt-5" loading={guardando} disabled={!puedeCambiar}>
          {t('legal.save')}
        </Button>
      </form>
    </Seccion>
  );
}

/* ------------------------------------------------------------------------------------------ */
/* Ajustes del sitio y zona de peligro                                                         */
/* ------------------------------------------------------------------------------------------ */

function Ajustes({
  siteId,
  sitio,
  t,
  onSitio,
  administra: puedeCambiar,
}: {
  siteId: string;
  sitio: Sitio;
  t: T;
  onSitio: (s: Sitio) => void;
  /** La dirección del sitio y su borrado son de quien administra: el servicio niega al resto. */
  administra: boolean;
}): JSX.Element {
  const [nombre, setNombre] = useState(sitio.name);
  const [slug, setSlug] = useState(sitio.slug);
  const [analitica, setAnalitica] = useState(sitio.analytics_enabled);
  const [errorNombre, setErrorNombre] = useState<string | null>(null);
  const [guardando, setGuardando] = useState(false);
  const [aviso, setAviso] = useState<Resultado | null>(null);

  const [borrando, setBorrando] = useState(false);
  const [confirmando, setConfirmando] = useState(false);
  const [escrito, setEscrito] = useState('');
  const [errorBorrado, setErrorBorrado] = useState(false);

  const guardar = async (event: Event) => {
    event.preventDefault();
    setAviso(null);
    if (!nombre.trim()) {
      setErrorNombre(t('settings.name_required'));
      return;
    }
    setErrorNombre(null);
    setGuardando(true);
    const res = await guardarSitio(siteId, {
      name: nombre.trim(),
      ...(puedeCambiar ? { slug: slug.trim() } : {}),
      analytics_enabled: analitica,
    });
    setGuardando(false);
    if ('sitio' in res) {
      onSitio(res.sitio);
      setAviso({ tono: 'exito', texto: t('settings.saved') });
    } else {
      setAviso({ tono: 'error', texto: t(mensajeDeAjustes(res.fallo)) });
    }
  };

  const borrar = async () => {
    setBorrando(true);
    setErrorBorrado(false);
    try {
      const res = await fetch(ruta(`/api/sites/${siteId}`), { method: 'DELETE' });
      if (!res.ok) throw new Error(String(res.status));
      window.location.assign(ruta('/'));
    } catch {
      setErrorBorrado(true);
      setBorrando(false);
    }
  };

  return (
    <div class="max-w-2xl space-y-6">
      <Seccion titulo={t('settings.title')} descripcion={t('settings.intro')}>
        <form
          class="space-y-5"
          onSubmit={(e) => {
            void guardar(e);
          }}
        >
          <FieldGroup
            label={t('settings.name')}
            htmlFor="ajuste-nombre"
            required
            {...(errorNombre ? { error: errorNombre } : {})}
          >
            <Input
              id="ajuste-nombre"
              value={nombre}
              invalid={errorNombre !== null}
              autocomplete="off"
              onInput={(e) => setNombre(e.currentTarget.value)}
            />
          </FieldGroup>
          <FieldGroup
            label={t('settings.slug')}
            htmlFor="ajuste-slug"
            help={t(puedeCambiar ? 'settings.slug_help' : 'settings.admin_only')}
          >
            <Input
              id="ajuste-slug"
              value={slug}
              disabled={!puedeCambiar}
              autocomplete="off"
              autocapitalize="none"
              spellcheck={false}
              onInput={(e) => setSlug(e.currentTarget.value)}
            />
          </FieldGroup>
          <div>
            <Checkbox
              id="ajuste-analitica"
              label={t('settings.analytics')}
              checked={analitica}
              onChange={(e) => setAnalitica(e.currentTarget.checked)}
            />
            <p class="text-text-muted mt-1 ml-6 text-xs">{t('settings.analytics_help')}</p>
          </div>
          {aviso ? <Aviso tono={aviso.tono}>{aviso.texto}</Aviso> : null}
          <Button type="submit" loading={guardando}>
            {t('settings.save')}
          </Button>
        </form>
      </Seccion>

      {puedeCambiar ? (
        <section
          aria-labelledby="zona-peligro"
          class="border-danger bg-surface rounded-xl border p-4 sm:p-6"
        >
          <h2 id="zona-peligro" class="sytue-h4 text-danger-text">
            {t('settings.danger_zone')}
          </h2>
          <p class="text-text-muted mt-1 text-sm">{t('settings.delete_desc')}</p>
          <Button
            class="mt-4"
            variant="danger"
            onClick={() => {
              setEscrito('');
              setErrorBorrado(false);
              setConfirmando(true);
            }}
          >
            {t('settings.delete_button')}
          </Button>
        </section>
      ) : null}

      <ConfirmDialog
        open={confirmando}
        onClose={() => {
          if (!borrando) setConfirmando(false);
        }}
        onConfirm={() => {
          void borrar();
        }}
        title={t('settings.delete_title')}
        confirmLabel={t('settings.delete_button')}
        tone="danger"
        confirmDisabled={escrito !== sitio.name}
        busy={borrando}
      >
        <div class="space-y-4">
          <p class="text-text text-sm">{t('settings.delete_desc')}</p>
          <FieldGroup
            label={t('settings.delete_confirm_prompt', { name: sitio.name })}
            htmlFor="borrar-nombre"
          >
            <Input
              id="borrar-nombre"
              value={escrito}
              autocomplete="off"
              spellcheck={false}
              onInput={(e) => setEscrito(e.currentTarget.value)}
            />
          </FieldGroup>
          {errorBorrado ? <Aviso tono="error">{t('settings.delete_error')}</Aviso> : null}
        </div>
      </ConfirmDialog>
    </div>
  );
}

/* ------------------------------------------------------------------------------------------ */
/* La ficha                                                                                    */
/* ------------------------------------------------------------------------------------------ */

export function SitioDetalleApp({ siteId, locale, user, hubBaseUrl }: Props): JSX.Element {
  const t: T = (k, params) => translate(locale, k, params);

  const [pestana, setPestana] = useState<Pestana>('publicar');
  const [sitio, setSitio] = useState<Sitio | null>(null);
  const [direccion, setDireccion] = useState<string | null>(null);
  const [estado, setEstado] = useState<'cargando' | 'ok' | 'error'>('cargando');
  const [fallo, setFallo] = useState<FalloApi | null>(null);

  // La pestaña vive en el `#hash` para poder enlazarla y para sobrevivir a una recarga. Se lee tras
  // hidratar (el servidor no ve el hash) y se escribe sin añadir pasos al historial.
  useEffect(() => {
    const hash = window.location.hash.slice(1);
    if (esPestana(hash)) setPestana(hash);
  }, []);
  const elegir = useCallback((id: string) => {
    if (!esPestana(id)) return;
    setPestana(id);
    window.history.replaceState(null, '', `#${id}`);
  }, []);

  const cargarSitio = useCallback(
    async (silencioso = false) => {
      if (!silencioso) setEstado('cargando');
      try {
        const [res, resRender] = await Promise.all([
          fetch(ruta(`/api/sites/${siteId}`)),
          fetch(ruta(`/api/sites/${siteId}/render-data`)).catch(() => null),
        ]);
        if (!res.ok) {
          setDireccion(null);
          setFallo(await leerFallo(res));
          setEstado('error');
          return;
        }
        setSitio((await res.json()) as Sitio);
        setDireccion(
          resRender?.ok ? direccionDelRender(await resRender.json().catch(() => null)) : null,
        );
        setEstado('ok');
      } catch {
        setDireccion(null);
        setFallo({ status: 0, code: '' });
        setEstado('error');
      }
    },
    [siteId],
  );

  useEffect(() => {
    void cargarSitio();
  }, [cargarSitio]);

  useEffect(() => {
    if (sitio) document.title = `${sitio.name} · ${translate(locale, 'nav.title')}`;
  }, [sitio, locale]);

  const refrescar = () => {
    void cargarSitio(true);
  };

  const volver = (
    <Button variant="secondary" href={ruta('/')}>
      {t('ficha.back')}
    </Button>
  );

  const contenido = (s: Sitio): JSX.Element => {
    const paneles: Record<Pestana, JSX.Element> = {
      publicar: (
        <Publicar
          siteId={siteId}
          sitio={s}
          locale={locale}
          t={t}
          irA={elegir}
          onCambio={refrescar}
        />
      ),
      versiones: <Versiones siteId={siteId} locale={locale} t={t} onCambio={refrescar} />,
      dominios: administra(user) ? (
        <Dominios
          siteId={siteId}
          locale={locale}
          t={t}
          onPublicar={() => elegir('publicar')}
          onCambio={refrescar}
        />
      ) : (
        <Seccion titulo={t('domains.title')}>
          <Aviso tono="info">{t('domains.err_forbidden')}</Aviso>
        </Seccion>
      ),
      tienda: (
        <Tienda siteId={siteId} t={(k) => t(k as ClaveMensaje)} administra={administra(user)} />
      ),
      pedidos: <Pedidos siteId={siteId} locale={locale} />,
      formularios: <Bandeja siteId={siteId} locale={locale} />,
      analitica: <Analitica siteId={siteId} sitio={s} locale={locale} t={t} irA={elegir} />,
      exportar: <Exportar siteId={siteId} sitio={s} t={t} onSitio={setSitio} />,
      legal: (
        <Legal
          siteId={siteId}
          sitio={s}
          locale={locale}
          t={t}
          onSitio={setSitio}
          administra={administra(user)}
        />
      ),
      ajustes: (
        <Ajustes siteId={siteId} sitio={s} t={t} onSitio={setSitio} administra={administra(user)} />
      ),
    };
    const rotulos: Record<Pestana, ClaveMensaje> = {
      publicar: 'publish.tab',
      versiones: 'versions.title',
      dominios: 'domains.title',
      tienda: 'store.title',
      pedidos: 'orders.title',
      formularios: 'forms.title',
      analitica: 'analytics.title',
      exportar: 'export.title',
      legal: 'legal.title',
      ajustes: 'settings.tab',
    };
    return (
      <Tabs
        label={t('ficha.sections')}
        value={pestana}
        onChange={elegir}
        items={PESTANAS.map((id) => ({ id, label: t(rotulos[id]), content: paneles[id] }))}
      />
    );
  };

  return (
    <Marco user={user} hubBaseUrl={hubBaseUrl} locale={locale} activa="sites">
      <div class="mx-auto max-w-6xl p-4 sm:p-6">
        <div class="mb-6 flex flex-wrap items-start justify-between gap-4">
          <div class="min-w-0 flex-1">
            {sitio ? (
              <>
                <div class="flex flex-wrap items-center gap-x-3 gap-y-1">
                  <h1 class="sytue-h2 text-text min-w-0 break-words">{sitio.name}</h1>
                  {sitio.published_number != null ? (
                    <Badge tone="success">
                      {t('sites.status.published')} (v{sitio.published_number})
                    </Badge>
                  ) : (
                    <Badge tone="neutral">{t('sites.status.draft')}</Badge>
                  )}
                </div>
                <div class="text-text-muted mt-1 flex flex-wrap items-center gap-2 text-sm break-all">
                  {direccion ? (
                    sitio.published_number != null ? (
                      <a
                        href={direccion}
                        target="_blank"
                        rel="noopener noreferrer"
                        class="sytue-focus-ring rounded-sm hover:underline"
                      >
                        {new URL(direccion).host}
                      </a>
                    ) : (
                      new URL(direccion).host
                    )
                  ) : (
                    <>
                      <span>{t('ficha.address_unavailable')}</span>
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => {
                          void cargarSitio(true);
                        }}
                      >
                        {t('sites.retry')}
                      </Button>
                    </>
                  )}
                </div>
              </>
            ) : estado === 'cargando' ? (
              <div aria-busy="true" aria-label={t('ficha.loading')}>
                <Skeleton height="2rem" width="16rem" class="max-w-full" />
                <Skeleton height="1rem" width="10rem" class="mt-2 max-w-full" />
              </div>
            ) : (
              <h1 class="sytue-h2 text-text">{t('ficha.page_title')}</h1>
            )}
          </div>
          <div class="flex flex-col gap-2 sm:flex-row">
            {volver}
            {sitio ? (
              <Button variant="primary" href={ruta(`/edit/${siteId}`)}>
                {t('sites.open_editor')}
              </Button>
            ) : null}
          </div>
        </div>

        {sitio ? (
          contenido(sitio)
        ) : estado === 'cargando' ? (
          <div class="space-y-4" aria-hidden="true">
            <Skeleton height="2.5rem" class="rounded-md" />
            <Skeleton height="14rem" class="rounded-xl" />
          </div>
        ) : fallo?.status === 404 || fallo?.status === 403 ? (
          <EmptyState
            title={t(fallo.status === 404 ? 'errors.not_found' : 'errors.unauthorized')}
            description={t('ficha.not_found_help')}
          />
        ) : (
          <EmptyState
            title={t('ficha.load_error')}
            description={t('sites.load_error_help')}
            action={
              <Button
                variant="secondary"
                onClick={() => {
                  void cargarSitio();
                }}
              >
                {t('sites.retry')}
              </Button>
            }
          />
        )}
      </div>
    </Marco>
  );
}

export default SitioDetalleApp;
