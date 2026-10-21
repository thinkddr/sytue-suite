/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Comparación VISUAL de dos PDF dentro del diálogo «Comparar»: cada página se compara
 * por píxeles y las regiones que cambian se pintan sobre la página, con navegación
 * entre ellas. El motor (`/api/v1/pdf/compare?modo=visual`) da los números y las
 * cajas; la imagen de una página —con lo quitado en rojo y lo añadido en verde— se pide
 * cuando se mira, no toda de golpe. El modo de texto sigue siendo el de siempre.
 */
import { useEffect, useRef, useState } from 'preact/hooks';
import { cx } from '@sytue/ui';
import { documentLocale } from '@sytue/i18n';
import type { docsT } from '../i18n';
import { api, type ComparacionVisual, type DiferenciaVisual } from './api';
import {
  SENSIBILIDADES,
  SENSIBILIDAD_POR_DEFECTO,
  cajaEnPorcentaje,
  claveDeAviso,
  moverParada,
  paradasDe,
  porcentajeLegible,
  srcDeImagen,
  toleranciaDe,
  type Sensibilidad,
} from './comparacionVisual.js';

type Estado = 'listo' | 'trabajando' | 'error';
type Pagina = DiferenciaVisual | 'cargando' | 'error';

export function CompararVisual({
  t,
  path,
  otro,
  onOtro,
}: {
  t: ReturnType<typeof docsT>;
  path: string;
  otro: string;
  onOtro: (v: string) => void;
}) {
  const [sens, setSens] = useState<Sensibilidad>(SENSIBILIDAD_POR_DEFECTO);
  const [estado, setEstado] = useState<Estado>('listo');
  const [res, setRes] = useState<ComparacionVisual | null>(null);
  const [parada, setParada] = useState(0);
  // La imagen de cada página que se ha mirado, por número. Se vacía al comparar de nuevo.
  const [paginas, setPaginas] = useState<Record<number, Pagina>>({});
  // Quién pidió la última comparación: una respuesta que llega tarde no pisa a la nueva.
  const vuelta = useRef(0);

  const lista = res?.paginas ?? [];
  const paradas = paradasDe(lista);
  const actual = paradas[parada];
  const locale = documentLocale();

  const comparar = () => {
    const mia = ++vuelta.current;
    setEstado('trabajando');
    setRes(null);
    setPaginas({});
    setParada(0);
    void api
      .compararVisual(path, otro, toleranciaDe(sens))
      .then((r) => {
        if (mia !== vuelta.current) return;
        setRes(r);
        setEstado('listo');
      })
      .catch(() => {
        if (mia === vuelta.current) setEstado('error');
      });
  };

  // Al llegar a una página nueva se pide su imagen.
  const paginaActual = actual?.pagina;
  useEffect(() => {
    if (paginaActual === undefined || paginas[paginaActual] !== undefined) return;
    // Una página que solo está en un documento, o que no se pudo comparar, no tiene
    // imagen que pedir: el aviso de arriba ya lo dice todo.
    const resumen = lista.find((p) => p.pagina === paginaActual);
    if (resumen !== undefined && (resumen.solo_en !== undefined || resumen.omitida === true))
      return;
    const mia = vuelta.current;
    setPaginas((p) => ({ ...p, [paginaActual]: 'cargando' }));
    void api
      .compararVisualPagina(path, otro, paginaActual, toleranciaDe(sens))
      .then((r) => {
        if (mia === vuelta.current) setPaginas((p) => ({ ...p, [paginaActual]: r.pagina }));
      })
      .catch(() => {
        if (mia === vuelta.current) setPaginas((p) => ({ ...p, [paginaActual]: 'error' }));
      });
    // `paginas` se lee solo para no repetir la petición: depender de él la repetiría.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [paginaActual, res]);

  const ir = (paso: 1 | -1) => setParada((p) => moverParada(p, paradas.length, paso));
  const irARegion = (pagina: number, region: number) => {
    const i = paradas.findIndex((x) => x.pagina === pagina && x.region === region);
    if (i >= 0) setParada(i);
  };

  const pagina = paginaActual === undefined ? undefined : paginas[paginaActual];
  const regiones = typeof pagina === 'object' ? (pagina.regiones ?? []) : [];

  return (
    <div class="flex flex-col gap-3">
      <p class="text-text-muted text-xs">{t('pdf.dialog.comparar.visual.help')}</p>
      <form
        class="flex flex-wrap items-center gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          comparar();
        }}
      >
        <input
          required
          value={otro}
          onInput={(e) => onOtro((e.target as HTMLInputElement).value)}
          placeholder="/Documentos/contrato-v2.pdf"
          aria-label={t('pdf.dialog.comparar.path')}
          class="border-border bg-surface text-text sytue-focus-ring min-w-48 flex-1 rounded-md border px-2 py-1.5 text-sm"
        />
        <label class="text-text-muted flex items-center gap-1.5 text-xs">
          {t('pdf.dialog.comparar.visual.sens')}
          <select
            value={sens}
            onChange={(e) => setSens((e.target as HTMLSelectElement).value as Sensibilidad)}
            class="border-border bg-surface text-text sytue-focus-ring rounded-md border px-2 py-1.5 text-sm"
          >
            {SENSIBILIDADES.map((s) => (
              <option key={s.id} value={s.id}>
                {t(s.clave)}
              </option>
            ))}
          </select>
        </label>
        <button
          type="submit"
          disabled={estado === 'trabajando'}
          class="bg-primary sytue-focus-ring rounded-control px-3 py-1.5 text-xs font-medium text-[var(--sytue-on-primary)] hover:bg-[var(--sytue-primary-hover)] disabled:opacity-40"
        >
          {t('pdf.dialog.comparar.run')}
        </button>
      </form>

      {estado === 'error' && (
        <p class="text-danger-text text-xs" role="alert">
          {t('pdf.dialog.comparar.error')}
        </p>
      )}

      {res !== null && (
        <div class="flex flex-col gap-3" role="status">
          {res.iguales && (
            <p class="text-success-text text-xs">{t('pdf.dialog.comparar.visual.same')}</p>
          )}
          {!res.iguales && (
            <p class="text-text text-xs font-medium">
              {t('pdf.dialog.comparar.visual.summary', {
                n: lista.length,
                total: Math.max(res.paginas_a, res.paginas_b),
              })}
            </p>
          )}
          {res.truncado && (
            <p class="text-warning-text bg-warning-bg text-2xs rounded p-2">
              {t('pdf.dialog.comparar.visual.truncado', { n: res.comparadas })}
            </p>
          )}
          {lista.map((p) => {
            const aviso = claveDeAviso(p);
            return aviso === null ? null : (
              <p key={p.pagina} class="text-warning-text bg-warning-bg text-2xs rounded p-2">
                {t(aviso, { n: p.pagina })}
              </p>
            );
          })}
        </div>
      )}

      {actual !== undefined && (
        <div class="flex flex-col gap-2">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <p class="text-text text-xs font-medium" aria-live="polite">
              {t('pdf.page.n', { n: actual.pagina })}
              {typeof pagina === 'object' && pagina.solo_en === undefined && !pagina.omitida
                ? ` · ${t('pdf.dialog.comparar.visual.pct', {
                    pct: porcentajeLegible(pagina.porcentaje, locale),
                  })}`
                : ''}
              {regiones.length > 0 && actual.region >= 0
                ? ` · ${t('pdf.dialog.comparar.visual.stop', {
                    i: actual.region + 1,
                    n: regiones.length,
                  })}`
                : ''}
            </p>
            <div class="flex gap-1.5">
              <button
                type="button"
                onClick={() => ir(-1)}
                disabled={paradas.length < 2}
                class="border-border text-text sytue-focus-ring rounded-control border px-2.5 py-1 text-xs disabled:opacity-40"
              >
                {t('nav.previous')}
              </button>
              <button
                type="button"
                onClick={() => ir(1)}
                disabled={paradas.length < 2}
                class="border-border text-text sytue-focus-ring rounded-control border px-2.5 py-1 text-xs disabled:opacity-40"
              >
                {t('nav.nextItem')}
              </button>
            </div>
          </div>

          {pagina === 'cargando' && <p class="text-text-muted text-xs">{t('pdf.page.loading')}</p>}
          {pagina === 'error' && (
            <p class="text-danger-text text-xs" role="alert">
              {t('pdf.dialog.comparar.error')}
            </p>
          )}
          {typeof pagina === 'object' && (
            <VistaDePagina
              t={t}
              pagina={pagina}
              region={actual.pagina === pagina.pagina ? actual.region : -1}
              onRegion={(i) => irARegion(pagina.pagina, i)}
            />
          )}
        </div>
      )}
    </div>
  );
}

function VistaDePagina({
  t,
  pagina,
  region,
  onRegion,
}: {
  t: ReturnType<typeof docsT>;
  pagina: DiferenciaVisual;
  region: number;
  onRegion: (i: number) => void;
}) {
  if (pagina.imagen === undefined || pagina.imagen === '') return null;
  const regiones = pagina.regiones ?? [];
  return (
    <div class="flex flex-col gap-2">
      {pagina.tamanos_distintos === true && (
        <p class="text-warning-text bg-warning-bg text-2xs rounded p-2">
          {t('pdf.dialog.comparar.visual.sizes')}
        </p>
      )}
      {pagina.escala < 0.5 && (
        <p class="text-warning-text bg-warning-bg text-2xs rounded p-2">
          {t('pdf.dialog.comparar.visual.baja')}
        </p>
      )}
      <div
        class="border-border relative w-full overflow-hidden border bg-white"
        style={{ aspectRatio: `${pagina.ancho} / ${pagina.alto}` }}
      >
        <img
          src={srcDeImagen(pagina.imagen)}
          alt={t('pdf.dialog.comparar.visual.alt', { n: pagina.pagina })}
          class="absolute inset-0 h-full w-full"
          draggable={false}
        />
        {regiones.map((r, i) => (
          <button
            key={i}
            type="button"
            aria-label={t('pdf.dialog.comparar.visual.region', { i: i + 1 })}
            aria-current={i === region ? 'true' : undefined}
            onClick={() => onRegion(i)}
            style={cajaEnPorcentaje(r, pagina.ancho, pagina.alto)}
            class={cx(
              'sytue-focus-ring absolute rounded-sm border-2',
              i === region
                ? 'border-primary bg-primary/15 z-10'
                : 'border-primary/50 hover:bg-primary/10',
            )}
          />
        ))}
      </div>
      <ul class="text-text-muted text-2xs flex flex-wrap gap-x-4 gap-y-1">
        <li class="flex items-center gap-1.5">
          <span class="bg-danger-bg inline-block h-2.5 w-2.5 rounded-sm" aria-hidden="true" />
          {t('pdf.dialog.comparar.visual.leyenda.a')}
        </li>
        <li class="flex items-center gap-1.5">
          <span class="bg-success-bg inline-block h-2.5 w-2.5 rounded-sm" aria-hidden="true" />
          {t('pdf.dialog.comparar.visual.leyenda.b')}
        </li>
      </ul>
    </div>
  );
}
