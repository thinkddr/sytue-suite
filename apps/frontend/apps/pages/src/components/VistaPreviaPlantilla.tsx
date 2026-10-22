/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Vista previa de una plantilla de la galería: pinta su IR con el renderizador de Pages en un iframe
 * aislado (sin scripts ni origen), en ordenador o en móvil y página a página. Es aproximada: las fotos
 * son marcadores y el mapa se sustituye por un recuadro para no pedir nada a terceros solo por mirar.
 */
import type { JSX } from 'preact';
import { useEffect, useMemo, useRef, useState } from 'preact/hooks';
import { Button, useMobileBack } from '@sytue/ui';
import type { DatosRender, Sitio } from '@sytue/pages-render';
import {
  documentoPlantilla,
  fuentesDePlantilla,
  type FamiliaPlantilla,
} from '../lib/vista-plantilla.js';
import { ruta } from '../lib/rutas.js';
import type { ClaveMensaje } from '../i18n/index.js';

type Traductor = (k: ClaveMensaje, params?: Record<string, string | number>) => string;

export interface VistaPreviaProps {
  plantilla: string;
  nombre: string;
  locale: 'es' | 'en';
  t: Traductor;
  elegida: boolean;
  onElegir: () => void;
  onCerrar: () => void;
}

export function VistaPreviaPlantilla({
  plantilla,
  nombre,
  locale,
  t,
  elegida,
  onElegir,
  onCerrar,
}: VistaPreviaProps): JSX.Element {
  const dialogo = useRef<HTMLDialogElement>(null);
  const [ir, setIr] = useState<Sitio | null>(null);
  const [fuentes, setFuentes] = useState<DatosRender['fonts']>({});
  const [error, setError] = useState(false);
  const [paginaId, setPaginaId] = useState('');
  const [movil, setMovil] = useState(false);

  useMobileBack(true, onCerrar);

  useEffect(() => {
    dialogo.current?.showModal();
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    setIr(null);
    setError(false);
    Promise.all([
      fetch(ruta(`/api/templates/${encodeURIComponent(plantilla)}`), {
        signal: controller.signal,
      }).then((res) =>
        res.ok ? (res.json() as Promise<{ ir: Sitio }>) : Promise.reject(new Error('http')),
      ),
      fetch(ruta('/api/catalog/fonts'), { signal: controller.signal }).then((res) =>
        res.ok ? (res.json() as Promise<FamiliaPlantilla[]>) : Promise.reject(new Error('http')),
      ),
    ])
      .then(async ([d, catalogo]) => {
        const cargadas = await fuentesDePlantilla(
          d.ir,
          catalogo,
          async (_familia, _script, fichero) => {
            const res = await fetch(ruta(`/api/catalog/files/${encodeURIComponent(fichero)}`), {
              signal: controller.signal,
            });
            if (!res.ok) throw new Error('font');
            return new Uint8Array(await res.arrayBuffer());
          },
        );
        if (controller.signal.aborted) return;
        setFuentes(cargadas);
        setIr(d.ir);
        setPaginaId(d.ir.pages[0]?.id ?? '');
      })
      .catch(() => {
        if (!controller.signal.aborted) setError(true);
      });
    return () => controller.abort();
  }, [plantilla]);

  const srcdoc = useMemo(() => {
    if (!ir || !paginaId) return '';
    try {
      return documentoPlantilla(ir, paginaId, locale, fuentes);
    } catch {
      return '';
    }
  }, [ir, paginaId, locale, fuentes]);

  const titulo = (p: Sitio['pages'][number]) => p.title[locale] || p.title.es || p.id;

  return (
    <dialog
      ref={dialogo}
      onClose={onCerrar}
      aria-label={`${t('new.preview')}: ${nombre}`}
      class="sytue-popover-surface text-text m-auto h-[90dvh] w-[min(96vw,1400px)] rounded-xl p-0 shadow-xl backdrop:bg-black/50"
      style={{
        inset:
          'var(--sytue-safe-top, 0px) var(--sytue-safe-right, 0px) var(--sytue-safe-bottom, 0px) var(--sytue-safe-left, 0px)',
        maxWidth: 'calc(96vw - var(--sytue-safe-left, 0px) - var(--sytue-safe-right, 0px))',
        maxHeight: 'calc(90dvh - var(--sytue-safe-top, 0px) - var(--sytue-safe-bottom, 0px))',
      }}
    >
      <div class="flex h-full flex-col">
        <div class="border-border flex flex-wrap items-center gap-3 border-b px-4 py-3">
          <h2 class="text-text mr-auto text-base font-bold">{nombre}</h2>
          <div class="flex gap-1" role="group" aria-label={t('new.preview')}>
            <Button variant={movil ? 'secondary' : 'primary'} onClick={() => setMovil(false)}>
              {t('new.preview_desktop')}
            </Button>
            <Button variant={movil ? 'primary' : 'secondary'} onClick={() => setMovil(true)}>
              {t('new.preview_mobile')}
            </Button>
          </div>
          <Button
            variant="primary"
            onClick={() => {
              dialogo.current?.close();
              onElegir();
            }}
            disabled={elegida}
          >
            {elegida ? t('new.selected') : t('new.select')}
          </Button>
          <Button variant="secondary" onClick={() => dialogo.current?.close()}>
            {t('new.preview_close')}
          </Button>
        </div>
        {ir && ir.pages.length > 1 && (
          <nav
            class="border-border flex flex-wrap gap-2 border-b px-4 py-2"
            aria-label={t('new.preview_pages')}
          >
            {ir.pages.map((p) => (
              <button
                key={p.id}
                type="button"
                aria-pressed={p.id === paginaId}
                onClick={() => setPaginaId(p.id)}
                class={`rounded-control border px-3 py-1 text-xs font-medium ${
                  p.id === paginaId
                    ? 'border-primary-text bg-selected text-primary-text'
                    : 'border-border text-text-muted'
                }`}
              >
                {titulo(p)}
              </button>
            ))}
          </nav>
        )}
        <div class="bg-bg-subtle flex min-h-0 flex-1 justify-center overflow-auto p-4">
          {error && <p class="text-text-muted self-center text-sm">{t('new.preview_error')}</p>}
          {!ir && !error && (
            <p class="text-text-muted self-center text-sm">{t('new.preview_loading')}</p>
          )}
          {ir && (
            <iframe
              title={`${t('new.preview')}: ${nombre}`}
              sandbox=""
              srcdoc={srcdoc}
              class={`h-full rounded-md border-0 bg-white shadow-md ${movil ? 'w-[390px]' : 'w-full'}`}
            />
          )}
        </div>
        <p class="text-text-muted border-border border-t px-4 py-2 text-xs">
          {t('new.preview_note')}
        </p>
      </div>
    </dialog>
  );
}
