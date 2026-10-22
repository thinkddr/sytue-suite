import { ruta } from '../lib/rutas.js';
/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Lo que comparten diseñar, traducir y exportar con IA: el idioma de la interfaz, el
 * lanzamiento de un trabajo con sus errores traducidos, y el panel de cupo mensual y aviso
 * RGPD. Estaba pensado tres veces y salía distinto en cada pantalla (CONVENTIONS §13.2).
 */
import { useCallback, useEffect, useState } from 'preact/hooks';
import type { ComponentChildren, JSX } from 'preact';
import { Button, Meter, Skeleton } from '@sytue/ui';
import { formatDate, isLocale, type Locale } from '@sytue/i18n';
import { t as translate, type ClaveMensaje } from '../i18n/index.js';
import { leerFallo } from '../lib/ficha.js';

export type Traducir = (clave: ClaveMensaje, params?: Record<string, string | number>) => string;

/**
 * Idioma de la interfaz. El editor monta estas pantallas sin pasarle `locale`, así que, si
 * falta, se lee el del documento (la página `.astro` lo fija en `<html lang>`).
 */
export function useTraduccion(locale?: Locale): { locale: Locale; t: Traducir } {
  const doc = typeof document === 'undefined' ? '' : document.documentElement.lang.slice(0, 2);
  const l: Locale = locale ?? (isLocale(doc) ? doc : 'es');
  return { locale: l, t: (clave, params) => translate(l, clave, params) };
}

/** Nombre del idioma en el idioma de la interfaz. Los nombres no se guardan en el catálogo. */
export function nombreIdioma(locale: Locale, codigo: string): string {
  try {
    const nombre = new Intl.DisplayNames([locale], { type: 'language' }).of(codigo) ?? codigo;
    return nombre.charAt(0).toLocaleUpperCase(locale) + nombre.slice(1);
  } catch {
    return codigo;
  }
}

// ---- Errores de la API ----

export interface ErrorIA {
  status: number;
  code?: string | undefined;
  resetAt?: string | undefined;
}

/** Los errores de la API traen un `code` estable (contrato); el `message` es solo respaldo. */
export async function leerError(res: Response): Promise<ErrorIA> {
  const fallo = await leerFallo(res);
  const resetAt = fallo.details?.reset_at;
  return {
    status: fallo.status,
    code: fallo.code,
    resetAt: typeof resetAt === 'string' ? resetAt : undefined,
  };
}

export function textoError(t: Traducir, locale: Locale, e: ErrorIA | null): string {
  switch (e?.code) {
    case 'ai_quota_exceeded':
      return e.resetAt
        ? t('ai.error.quota', { date: formatDate(locale, e.resetAt) })
        : t('ai.error.quota_nodate');
    case 'ai_busy':
      return t('ai.error.busy');
    case 'admin_required':
      return t('ai.error.admin');
    case 'prompt_required':
      return t('ai.error.prompt');
    case 'invalid_languages':
      return t('ai.error.languages');
    case 'invalid_translation_scope':
      return t('ai.translate.scope_invalid');
    case 'translation_scope_unavailable':
      return t('ai.translate.store_unavailable');
    case 'draft_not_saved':
      return t('ai.error.draft_not_saved');
    default:
      if (e?.status === 401) return t('ai.error.session');
      return e?.status === 403 ? t('ai.error.admin') : t('ai.error.generic');
  }
}

/** Lanza un trabajo (la API responde 202 con `{job}`) y devuelve su id, o el error tipado. */
export async function lanzarTrabajoIA(
  url: string,
  cuerpo: unknown,
  prepararBorrador?: () => Promise<boolean>,
): Promise<{ jobId: string } | { error: ErrorIA }> {
  try {
    if (prepararBorrador && !(await prepararBorrador())) {
      return { error: { status: 409, code: 'draft_not_saved' } };
    }
    const res = await fetch(ruta(url), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(cuerpo),
    });
    if (!res.ok) return { error: await leerError(res) };
    const data = (await res.json()) as { job?: { id?: string } };
    if (!data.job?.id) return { error: { status: 502 } };
    return { jobId: data.job.id };
  } catch {
    return { error: { status: 0 } };
  }
}

/** El resultado del trabajo llega como objeto o como cadena JSON según el camino. */
export function resultadoDe(valor: unknown): unknown {
  if (typeof valor !== 'string') return valor;
  try {
    return JSON.parse(valor);
  } catch {
    return null;
  }
}

// ---- Proveedores y región ----

const PROVEEDOR_FUERA_DEL_EEE = new Set(['openai', 'anthropic', 'zai']);
export const fueraDelEEE = (proveedor: string | undefined): boolean =>
  !!proveedor && PROVEEDOR_FUERA_DEL_EEE.has(proveedor);

const ETIQUETA_PROVEEDOR: Record<string, ClaveMensaje> = {
  openai: 'ai.settings.provider_openai',
  anthropic: 'ai.settings.provider_anthropic',
  zai: 'ai.settings.provider_zai',
};
export const etiquetaProveedor = (t: Traducir, proveedor: string): string =>
  ETIQUETA_PROVEEDOR[proveedor] ? t(ETIQUETA_PROVEEDOR[proveedor]) : proveedor;

// ---- Cupo mensual y aviso RGPD ----

interface Cupo {
  limit: number;
  used: number;
  reset_at: string;
}
interface ClaveVista {
  has_key: boolean;
  provider?: string;
}

export interface ContextoIA {
  cargando: boolean;
  error: boolean;
  cupo: Cupo | null;
  /** null = no se sabe: la clave la ve solo un administrador (403 para el resto). */
  clave: ClaveVista | null;
  /** Solo es cierto cuando se sabe que no hay clave propia: con clave propia no hay cupo. */
  bloqueado: boolean;
  recargar: () => void;
}

/**
 * `administra` (de lib/roles): solo quien administra la organización puede leer la vista
 * de la clave. A quien no, NO SE LE PIDE —el servicio contestaría 403— y se queda en
 * `clave: null`, que es lo que ya valía ese 403: su pantalla no sabe si hay clave propia.
 */
export function useContextoIA(administra = true): ContextoIA {
  const [estado, setEstado] = useState<Omit<ContextoIA, 'bloqueado' | 'recargar'>>({
    cargando: true,
    error: false,
    cupo: null,
    clave: null,
  });

  const recargar = useCallback(() => {
    setEstado((e) => ({ ...e, cargando: true, error: false }));
    void (async () => {
      try {
        const [rc, rk] = await Promise.all([
          fetch(ruta('/api/ia/quota')),
          administra ? fetch(ruta('/api/ia/key')) : null,
        ]);
        if (!rc.ok) throw new Error(String(rc.status));
        setEstado({
          cargando: false,
          error: false,
          cupo: (await rc.json()) as Cupo,
          clave: rk?.ok ? ((await rk.json()) as ClaveVista) : null,
        });
      } catch {
        setEstado({ cargando: false, error: true, cupo: null, clave: null });
      }
    })();
  }, [administra]);

  useEffect(recargar, [recargar]);

  const agotado = !!estado.cupo && estado.cupo.used >= estado.cupo.limit;
  // ponytail: un miembro no puede leer la clave (403), así que su pantalla no sabe si hay clave
  // propia y no se bloquea el envío por cupo: lo decide el servidor y su error ya sale traducido.
  return {
    ...estado,
    bloqueado: agotado && estado.clave !== null && !estado.clave.has_key,
    recargar,
  };
}

/** Cupo del mes y dónde se procesan los datos, a la vista ANTES de lanzar un trabajo. */
export function PanelContextoIA({
  ctx,
  t,
  locale,
}: {
  ctx: ContextoIA;
  t: Traducir;
  locale: Locale;
}): JSX.Element {
  if (ctx.cargando) {
    return (
      <div role="status" aria-busy="true" aria-label={t('ai.context.loading')} class="space-y-2">
        <Skeleton height="0.75rem" width="40%" />
        <Skeleton height="0.375rem" />
      </div>
    );
  }
  if (ctx.error || !ctx.cupo) {
    return (
      <div class="border-border bg-bg-subtle flex flex-wrap items-center justify-between gap-2 rounded-md border p-3">
        <p class="text-text-muted text-sm" role="status">
          {t('ai.context.load_error')}
        </p>
        <Button type="button" size="sm" variant="secondary" onClick={ctx.recargar}>
          {t('ai.context.retry')}
        </Button>
      </div>
    );
  }

  const { cupo, clave } = ctx;
  const propia = !!clave?.has_key;
  const fuera = propia && fueraDelEEE(clave?.provider);
  const reinicio = formatDate(locale, cupo.reset_at);

  return (
    <div class="space-y-3">
      {propia ? (
        <p class="text-text-muted text-sm">{t('ai.context.byok')}</p>
      ) : (
        <div>
          <Meter
            value={cupo.used}
            max={cupo.limit}
            label={t('ai.context.quota_label')}
            valueText={t('ai.context.quota_used', { used: cupo.used, limit: cupo.limit })}
          />
          <p class="text-text-muted mt-1 text-xs">
            {t('ai.context.quota_reset', { date: reinicio })}
          </p>
        </div>
      )}

      {ctx.bloqueado ? (
        <div role="alert" class="border-border bg-danger-bg rounded-md border p-3">
          <p class="text-danger-text text-sm font-semibold">
            {t('ai.context.quota_exhausted_title')}
          </p>
          <p class="text-text mt-1 text-sm">
            {t('ai.context.quota_exhausted_desc', { date: reinicio })}
          </p>
        </div>
      ) : null}

      {fuera ? (
        <div class="border-border bg-warning-bg rounded-md border p-3">
          <p class="text-warning-text text-sm font-semibold">{t('ai.notice.non_eu_title')}</p>
          <p class="text-text mt-1 text-sm">
            {t('ai.notice.non_eu_desc', { provider: etiquetaProveedor(t, clave?.provider ?? '') })}
          </p>
        </div>
      ) : clave ? (
        <p class="text-text-muted text-xs">{t('ai.notice.eu')}</p>
      ) : null}
    </div>
  );
}

// ---- Piezas de maquetación comunes ----

/** Pie de acciones: apilado en móvil (la principal arriba) y a la derecha en pantallas anchas. */
export function PieAcciones({ children }: { children: ComponentChildren }): JSX.Element {
  return (
    <div class="border-border flex flex-col-reverse gap-2 border-t pt-4 sm:flex-row sm:justify-end">
      {children}
    </div>
  );
}

/** Lo que devolvió el trabajo, a revisar antes de aplicarlo. */
export function TarjetaResultadoIA({
  titulo,
  acciones,
  children,
}: {
  titulo: string;
  acciones: ComponentChildren;
  children: ComponentChildren;
}): JSX.Element {
  return (
    <section class="border-border bg-surface space-y-3 rounded-lg border p-4">
      <h4 class="sytue-h4 text-text">{titulo}</h4>
      {children}
      <PieAcciones>{acciones}</PieAcciones>
    </section>
  );
}

/** Mensaje de error de un envío o de un resultado vacío. */
export function AvisoErrorIA({ texto }: { texto: string }): JSX.Element {
  return (
    <p
      role="alert"
      class="border-border bg-danger-bg text-danger-text rounded-md border p-3 text-sm"
    >
      {texto}
    </p>
  );
}
