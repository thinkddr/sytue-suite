/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Galería de plantillas de «Crear sitio». Con más de setenta plantillas se encuentra por el tipo de
 * negocio: buscador que entiende «pelu» o «barbero», grupos por sector y vista previa de cada una.
 */
import type { JSX } from 'preact';
import { useMemo, useState } from 'preact/hooks';
import { Badge, Button, Input } from '@sytue/ui';
import type { ClaveMensaje } from '../i18n/index.js';
import {
  agrupar,
  buscar,
  CATEGORIAS,
  descripcionDe,
  nombreDe,
  type PlantillaInfo,
} from '../lib/galeria.js';
import { VistaPreviaPlantilla } from './VistaPreviaPlantilla.js';
import { rutaPublica } from '../lib/rutas.js';

type Traductor = (k: ClaveMensaje, params?: Record<string, string | number>) => string;

export interface GaleriaProps {
  plantillas: PlantillaInfo[];
  seleccionada: string;
  onSeleccionar: (key: string) => void;
  locale: 'es' | 'en';
  t: Traductor;
}

export function GaleriaPlantillas({
  plantillas: todas,
  seleccionada,
  onSeleccionar,
  locale,
  t,
}: GaleriaProps): JSX.Element {
  const [consulta, setConsulta] = useState('');
  const [categoria, setCategoria] = useState<string | null>(null);
  const [enVista, setEnVista] = useState<PlantillaInfo | null>(null);
  // «En blanco» tiene su propia tarjeta: la plantilla blanco del catálogo no se repite en su grupo.
  const plantillas = useMemo(() => todas.filter((p) => p.key !== 'blanco'), [todas]);

  const etiquetaCategoria = (c: string) => t(`new.cat.${c}` as ClaveMensaje);
  const filtrando = consulta.trim() !== '' || categoria !== null;
  const resultados = buscar(plantillas, consulta, categoria, locale, etiquetaCategoria);
  const grupos = useMemo(() => agrupar(plantillas, locale), [plantillas, locale]);
  const presentes = new Set(plantillas.map((p) => p.category || 'otras'));
  const plantillaBlanca = todas.find((p) => p.key === 'blanco');

  const tarjeta = (p: PlantillaInfo) => {
    const elegida = seleccionada === p.key;
    return (
      <article
        key={p.key}
        class={`flex min-w-0 flex-col justify-between overflow-hidden rounded-lg border transition-colors ${
          elegida ? 'border-primary-text bg-selected shadow-sm' : 'border-border bg-surface'
        }`}
      >
        <button
          type="button"
          class="sytue-focus-ring block w-full overflow-hidden"
          onClick={() => setEnVista(p)}
          aria-label={`${t('new.preview')}: ${nombreDe(p, locale)}`}
        >
          <img
            src={rutaPublica(`/templates/${encodeURIComponent(p.key)}-${locale}.jpg`)}
            width={960}
            height={640}
            loading="lazy"
            alt=""
            class="aspect-[3/2] w-full object-cover object-top"
          />
        </button>
        <div class="p-4 pb-0">
          <div class="flex items-center justify-between gap-2">
            <h4 class="text-text font-bold">{nombreDe(p, locale)}</h4>
            {p.store && <Badge tone="success">{t('new.badge_store')}</Badge>}
          </div>
          <p class="text-text-muted mt-2 text-xs">{descripcionDe(p, locale)}</p>
        </div>
        <div class="flex flex-wrap gap-2 p-4">
          <Button
            variant={elegida ? 'primary' : 'secondary'}
            size="sm"
            aria-pressed={elegida}
            onClick={() => onSeleccionar(p.key)}
          >
            {elegida ? t('new.selected') : t('new.select')}
          </Button>
          <Button variant="secondary" size="sm" onClick={() => setEnVista(p)}>
            {t('new.preview')}
          </Button>
        </div>
      </article>
    );
  };

  const rejilla = (items: PlantillaInfo[]) => (
    <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">{items.map(tarjeta)}</div>
  );

  const enBlanco = (
    <article
      class={`flex min-w-0 flex-col justify-between overflow-hidden rounded-lg border ${
        seleccionada === 'blanco'
          ? 'border-primary-text bg-selected shadow-sm'
          : 'border-border bg-surface'
      }`}
    >
      <button
        type="button"
        class="sytue-focus-ring block w-full overflow-hidden"
        onClick={() => plantillaBlanca && setEnVista(plantillaBlanca)}
        disabled={!plantillaBlanca}
        aria-label={`${t('new.preview')}: ${t('new.template_blank')}`}
      >
        <img
          src={rutaPublica(`/templates/blanco-${locale}.jpg`)}
          width={960}
          height={640}
          loading="lazy"
          alt=""
          class="aspect-[3/2] w-full object-cover object-top"
        />
      </button>
      <div class="p-4 pb-0">
        <h4 class="text-text font-bold">{t('new.template_blank')}</h4>
        <p class="text-text-muted mt-2 text-xs">{t('new.template_blank_desc')}</p>
      </div>
      <div class="flex items-center gap-2 p-4">
        <Button
          variant={seleccionada === 'blanco' ? 'primary' : 'secondary'}
          size="sm"
          aria-pressed={seleccionada === 'blanco'}
          onClick={() => onSeleccionar('blanco')}
        >
          {seleccionada === 'blanco' ? t('new.selected') : t('new.select')}
        </Button>
        <Badge tone="neutral">{t('new.badge_custom')}</Badge>
      </div>
    </article>
  );

  return (
    <div>
      <section
        class="border-border bg-surface mb-6 rounded-xl border p-5"
        aria-labelledby="galeria-titulo"
      >
        <h2 id="galeria-titulo" class="text-text text-lg font-bold">
          {t('new.search_title')}
        </h2>
        <p class="text-text-muted mt-1 text-sm">{t('new.search_help')}</p>
        <label for="galeria-buscar" class="text-text mt-4 mb-1 block text-sm font-medium">
          {t('new.search_label')}
        </label>
        <div class="flex gap-2">
          <Input
            id="galeria-buscar"
            type="search"
            value={consulta}
            autocomplete="off"
            placeholder={t('new.search_placeholder')}
            onInput={(e) => setConsulta((e.target as HTMLInputElement).value)}
          />
          {consulta !== '' && (
            <Button variant="secondary" onClick={() => setConsulta('')}>
              {t('new.clear_search')}
            </Button>
          )}
        </div>
        <div class="mt-4 flex flex-wrap gap-2" role="group" aria-label={t('new.search_title')}>
          {[null, ...CATEGORIAS.filter((c) => presentes.has(c))].map((c) => (
            <button
              key={c ?? 'todos'}
              type="button"
              aria-pressed={categoria === c}
              onClick={() => setCategoria(c)}
              class={`rounded-control border px-3 py-1 text-xs font-medium ${
                categoria === c
                  ? 'border-primary-text bg-selected text-primary-text'
                  : 'border-border text-text-muted'
              }`}
            >
              {c === null ? t('new.all_groups') : etiquetaCategoria(c)}
            </button>
          ))}
        </div>
      </section>

      {filtrando ? (
        <div>
          <p class="text-text-muted mb-3 text-sm" role="status" aria-live="polite">
            {t('new.results', { count: resultados.length })}
          </p>
          {resultados.length === 0 ? (
            <div class="border-border rounded-xl border p-5">
              <p class="text-text font-medium">{t('new.no_results', { q: consulta.trim() })}</p>
              <p class="text-text-muted mt-1 mb-4 text-sm">{t('new.no_results_help')}</p>
              <div class="max-w-xs">{enBlanco}</div>
            </div>
          ) : (
            rejilla(resultados)
          )}
        </div>
      ) : (
        <div class="space-y-8">
          <div class="max-w-xs">{enBlanco}</div>
          {grupos.map((g) => (
            <section key={g.categoria} aria-labelledby={`grupo-${g.categoria}`}>
              <h3 id={`grupo-${g.categoria}`} class="text-text mb-3 text-base font-bold">
                {etiquetaCategoria(g.categoria)}{' '}
                <span class="text-text-muted text-xs font-normal">({g.plantillas.length})</span>
              </h3>
              {rejilla(g.plantillas)}
            </section>
          ))}
        </div>
      )}

      {enVista && (
        <VistaPreviaPlantilla
          plantilla={enVista.key}
          nombre={nombreDe(enVista, locale)}
          locale={locale}
          t={t}
          elegida={seleccionada === enVista.key}
          onElegir={() => {
            onSeleccionar(enVista.key);
            setEnVista(null);
          }}
          onCerrar={() => setEnVista(null)}
        />
      )}
    </div>
  );
}
