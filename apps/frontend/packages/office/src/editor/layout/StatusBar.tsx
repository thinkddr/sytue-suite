/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Barra de estado inferior de 28 px (BLUEPRINT.md §6.7): unidad actual
 * (página/hoja/diapositiva), palabras e idioma (solo kind document) y control
 * de zoom (botón −, deslizador MIN_ZOOM–MAX_ZOOM en pasos de 10, porcentaje, botón +).
 * El porcentaje es un botón: un clic vuelve al 100 %, como Ctrl/⌘ 0 (atajosZoom.ts).
 * Las palabras también: abren el recuento detallado (Ctrl/⌘ Mayús C) y, con texto
 * seleccionado en el folio, dicen «12 de 340», como Word y Docs de Google.
 * El idioma es el DEL DOCUMENTO (los 26 de la suite): se guarda en su capa de página
 * (pagedecor.ts) y el folio lo lleva como `lang`, que es lo que mira el corrector.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import type { DocKind } from '@sytue/api-clients';
import { LOCALE_NAMES, LOCALES, isLocale } from '@sytue/i18n';
import { IconButton, useUltimo } from '@sytue/ui';
import { EditorIcon } from '../icons';
import { MIN_ZOOM, MAX_ZOOM, clampZoom } from '../canvas/metrics';
import { ponerCapaDelFolio } from '../canvas/exec';
import { paginaHtml } from '../pagedecor';
import { RecuentoDialog, type RecuentoDocumento } from '../dialogs/RecuentoDialog';
import {
  cifrasDelTexto,
  esAtajoRecuento,
  textoDeLaSeleccion,
  textoDelDocumento,
  type CifrasTexto,
} from '../recuento';
import type { DocsKey } from '../../i18n';
import type { EditorContext } from '../types';

/* «Página 2 de 7» es UNA frase por tipo de documento, no «unidad» + números
 * concatenados: en alemán y en francés el orden y la preposición cambian. */
const POSITION_KEY: Record<DocKind, DocsKey> = {
  document: 'status.page',
  spreadsheet: 'status.sheet',
  presentation: 'status.slide',
};

/** El de la selección se rehace como el reflejo de formato (EditorApp, §4.4): 100 ms. */
export const RECUENTO_THROTTLE_MS = 100;

/**
 * Las palabras de la selección del folio, o null sin selección. Escucha lo mismo que el
 * reflejo de formato —`selectionchange` y el `input` del folio— con el mismo throttle:
 * contar en cada paso de un arrastre por un documento largo se notaría al escribir.
 */
function usePalabrasSeleccionadas(activo: boolean): number | null {
  const [palabras, setPalabras] = useState<number | null>(null);
  useEffect(() => {
    if (!activo) {
      setPalabras(null);
      return;
    }
    let timer: number | null = null;
    const contar = (): void => {
      const texto = textoDeLaSeleccion();
      setPalabras(texto === null ? null : cifrasDelTexto(texto).palabras);
    };
    const programar = (): void => {
      if (timer !== null) return;
      timer = window.setTimeout(() => {
        timer = null;
        contar();
      }, RECUENTO_THROTTLE_MS);
    };
    contar();
    document.addEventListener('selectionchange', programar);
    document.addEventListener('input', programar, true);
    return () => {
      document.removeEventListener('selectionchange', programar);
      document.removeEventListener('input', programar, true);
      if (timer !== null) window.clearTimeout(timer);
    };
  }, [activo]);
  return palabras;
}

/** La foto que enseña el diálogo: se toma al abrirlo, con la selección de ese momento. */
interface Recuento {
  documento: RecuentoDocumento;
  seleccion: CifrasTexto | null;
}

export function StatusBar({ ctx }: { ctx: EditorContext }): JSX.Element {
  const t = ctx.t;
  const { page, pages, words } = ctx.stats;
  const esDocumento = ctx.kind === 'document';
  const seleccionadas = usePalabrasSeleccionadas(esDocumento);
  const [recuento, setRecuento] = useState<Recuento | null>(null);

  const abrirRecuento = (): void => {
    const texto = textoDeLaSeleccion();
    setRecuento({
      // Las palabras del documento son las de la barra (countWords), no un segundo
      // recuento: el diálogo no puede contradecir a la cifra que se acaba de pulsar.
      documento: {
        ...cifrasDelTexto(textoDelDocumento(ctx.content)),
        palabras: words,
        paginas: pages,
      },
      seleccion: texto === null ? null : cifrasDelTexto(texto),
    });
  };
  const abrirRef = useUltimo(abrirRecuento);

  /* Ctrl/⌘ Mayús C en la ventana, para que responda también con el foco en la cinta. */
  useEffect(() => {
    if (!esDocumento) return;
    const onKey = (e: KeyboardEvent): void => {
      if (!esAtajoRecuento(e)) return;
      e.preventDefault();
      abrirRef.current();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [esDocumento, abrirRef]);

  const rotuloPalabras =
    seleccionadas === null
      ? t('status.words', { count: words })
      : t('status.words.selection', { selected: seleccionadas, count: words });
  /* Sin idioma propio, el documento va en el de la interfaz; elegir ese mismo lo fija. */
  const idioma = ctx.pageSetup.lang ?? ctx.locale;
  const cambiarIdioma = (valor: string): void => {
    if (!isLocale(valor) || ctx.soloLectura) return;
    const next = { ...ctx.pageSetup, lang: valor };
    ctx.setPageSetup(next);
    ponerCapaDelFolio('[data-sytue-page]', paginaHtml(next));
  };
  const zoom = ctx.ui.zoom;
  const setZoom = (next: number): void => {
    ctx.setUi({ zoom: clampZoom(next) });
  };

  return (
    <div class="sytue-sheet-corners border-border bg-surface text-text-muted flex h-[var(--editor-status-h)] shrink-0 items-center gap-4 border-t px-3 text-xs [--sytue-sheet-tl:0px] [--sytue-sheet-tr:0px]">
      <span class="shrink-0 whitespace-nowrap">
        {t(POSITION_KEY[ctx.kind] ?? 'status.page', { page, pages })}
      </span>
      {esDocumento && (
        <>
          {/* Fuera en <640px: la barra mide 28px de alto y no puede envolver, y
              con los dos informativos sumaba 465px contra los 390 de un móvil.
              Se van los que solo informan; el zoom —la única acción, y la que más
              falta hace en un teléfono— se queda entero. */}
          {/* Un botón con pinta de rótulo, como el «100 %»: lo que se lee es su nombre
              accesible, y la pista dice qué abre y con qué atajo. */}
          <button
            type="button"
            onClick={abrirRecuento}
            aria-haspopup="dialog"
            title={t('status.words.details')}
            class="hover:text-text sytue-focus-ring shrink-0 cursor-pointer rounded-sm whitespace-nowrap tabular-nums max-sm:hidden"
          >
            {rotuloPalabras}
          </button>
          {/* 24 px de alto, no `h-5`: eran 17,5 (1,25 rem cuando 1 rem eran 14 px), medido
              en producción; la barra mide 28 y cabe. Los nombres van en su propio idioma
              (LOCALE_NAMES), como en el selector de la cuenta. */}
          <select
            aria-label={t('status.language')}
            title={t('status.language')}
            value={idioma}
            disabled={ctx.soloLectura}
            onChange={(e) => cambiarIdioma(e.currentTarget.value)}
            class="text-text-muted hover:text-text sytue-focus-ring h-[24px] cursor-pointer rounded-sm bg-transparent text-xs disabled:cursor-default max-sm:hidden"
          >
            {LOCALES.map((l) => (
              <option key={l} value={l} lang={l}>
                {LOCALE_NAMES[l]}
              </option>
            ))}
          </select>
        </>
      )}

      {/* EL DESLIZADOR CEDE. Con el dedo los dos botones miden 44 px, y con el rótulo más
          largo —«Diapositiva 1 de 3»— el grupo se salía 4 px de la barra a 320. De todo
          lo que hay aquí, el deslizador es lo único que puede medir menos sin dejar de
          servir (los botones de al lado hacen lo mismo paso a paso), y nunca menos de
          44 px, que sigue siendo una diana para el dedo; ni el rótulo ni el porcentaje
          parten en dos líneas, que la barra mide 28 px. */}
      <div class="ml-auto flex min-w-0 items-center gap-1">
        <IconButton
          size="sm"
          label={t('status.zoom.out')}
          disabled={zoom <= MIN_ZOOM}
          onClick={() => setZoom(zoom - 10)}
        >
          <EditorIcon name="minus" size="sm" />
        </IconButton>
        <input
          type="range"
          min={MIN_ZOOM}
          max={MAX_ZOOM}
          step={10}
          value={zoom}
          aria-label={t('status.zoom')}
          aria-valuetext={`${zoom} %`}
          title={t('status.zoom')}
          onInput={(e) => setZoom(Number(e.currentTarget.value))}
          class="sytue-zoom sytue-focus-ring w-24 min-w-[44px] shrink cursor-pointer rounded-full pointer-coarse:min-h-[max(44px,2.75rem)]"
        />
        {/* «Volver al 100 %» de un clic: con el deslizador en pasos de 10, desde el 130
            eran tres pulsaciones de «−» o atinar con el dedo. El nombre accesible empieza
            por lo que se lee («130 %»): quien manda por voz dice lo que ve. */}
        <button
          type="button"
          onClick={() => setZoom(100)}
          aria-label={t('status.zoom.reset', { zoom })}
          title={t('status.zoom.reset', { zoom })}
          class="hover:text-text sytue-focus-ring w-10 shrink-0 cursor-pointer rounded-sm text-center whitespace-nowrap tabular-nums pointer-coarse:min-h-[max(44px,2.75rem)]"
        >
          {zoom} %
        </button>
        <IconButton
          size="sm"
          label={t('status.zoom.in')}
          disabled={zoom >= MAX_ZOOM}
          onClick={() => setZoom(zoom + 10)}
        >
          <EditorIcon name="plus" size="sm" />
        </IconButton>
      </div>
      {recuento && (
        <RecuentoDialog
          open
          onClose={() => setRecuento(null)}
          t={t}
          locale={ctx.locale}
          documento={recuento.documento}
          seleccion={recuento.seleccion}
        />
      )}
    </div>
  );
}
