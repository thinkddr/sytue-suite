/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Contenedor del lienzo de edición de Sytue Web (PAGES-PLAN §14.1).
 *
 * Integra LienzoIframe y LienzoOverlay en un escenario escalado por zoom,
 * con soporte para rueda con Ctrl para acercar/alejar y centrado visual.
 */

import { useCallback, useMemo, useState } from 'preact/hooks';
import type { JSX } from 'preact';
import type { DatosRender, Idioma, Sitio, Tamano } from '@sytue/pages-render';
import { pintarPagina } from '@sytue/pages-render';
import type { PuntoDeCorte } from '../modelo/tipos';
import { ANCHOS_PUNTO_DE_CORTE } from '../modelo/tipos';
import { LienzoIframe } from './LienzoIframe';
import { LienzoOverlay } from './LienzoOverlay';
import type { IndicadorInsercion } from './LienzoOverlay';
import { ruta } from '../../lib/rutas.js';

export interface LienzoProps {
  siteId: string;
  ir: Sitio;
  datosRender: DatosRender;
  paginaActivaId: string;
  idioma: Idioma;
  puntoDeCorte: PuntoDeCorte;
  zoom: number;
  nodoSeleccionadoId: string | null;
  onSeleccionar: (id: string | null, acumulativo: boolean) => void;
  onRedimensionar: (id: string, nuevoTamano: Tamano) => void;
  onZoomChange?: (nuevoZoom: number) => void;
  indicadorInsercion?: IndicadorInsercion | null;
}

export function Lienzo({
  siteId,
  ir,
  datosRender,
  paginaActivaId,
  idioma,
  puntoDeCorte,
  zoom,
  nodoSeleccionadoId,
  onSeleccionar,
  onRedimensionar,
  onZoomChange,
  indicadorInsercion,
}: LienzoProps): JSX.Element {
  const [docIframe, setDocIframe] = useState<Document | null>(null);

  const anchoPx = ANCHOS_PUNTO_DE_CORTE[puntoDeCorte];
  const pagina = ir.pages.find((p) => p.id === paginaActivaId) ?? ir.pages[0];
  const paginaNombre = pagina?.title?.es || pagina?.title?.en || pagina?.slug?.es || paginaActivaId;

  // Pintado del IR a HTML y CSS mediante el renderizador oficial en modo 'lienzo'
  const { html, css } = useMemo(() => {
    try {
      return pintarPagina(ir, datosRender, paginaActivaId, idioma, {
        modo: 'lienzo',
        puntoDeCorte,
        urlRecurso: (nombre: string) =>
          ruta(`/api/sites/${encodeURIComponent(siteId)}/assets/${encodeURIComponent(nombre)}`),
      });
    } catch {
      return {
        html: '<div style="padding:2rem;text-align:center;color:#666">Página vacía o sin contenido</div>',
        css: '',
      };
    }
  }, [ir, datosRender, paginaActivaId, idioma, puntoDeCorte, siteId]);

  const handleDocReady = useCallback((doc: Document | null) => {
    setDocIframe(doc);
  }, []);

  // Zoom con rueda + Ctrl/Cmd
  const handleWheel = (e: WheelEvent) => {
    if (e.ctrlKey || e.metaKey) {
      e.preventDefault();
      if (!onZoomChange) return;
      const factor = e.deltaY < 0 ? 1.05 : 0.95;
      const nuevoZoom = Math.min(Math.max(zoom * factor, 0.25), 2.0);
      onZoomChange(nuevoZoom);
    }
  };

  return (
    <div
      onWheel={handleWheel}
      class="bg-bg-subtle relative flex flex-1 items-start justify-center overflow-auto p-8"
      style={{
        backgroundImage:
          'radial-gradient(circle, var(--sytue-border, #cbd5e1) 1px, transparent 1px)',
        backgroundSize: '24px 24px',
      }}
    >
      {/* Contenedor escalado */}
      <div
        class="relative origin-top overflow-hidden rounded-sm shadow-2xl transition-transform duration-75"
        style={{
          width: `${anchoPx}px`,
          minHeight: '800px',
          transform: `scale(${zoom})`,
          backgroundColor: '#ffffff',
        }}
      >
        {/* Iframe con srcdoc e inert */}
        <LienzoIframe
          paginaNombre={paginaNombre}
          puntoDeCorte={puntoDeCorte}
          html={html}
          css={css}
          onDocReady={handleDocReady}
        />

        {/* Capa superpuesta del padre para hit-testing, tiradores y selección */}
        <LienzoOverlay
          documentoIframe={docIframe}
          nodoSeleccionadoId={nodoSeleccionadoId}
          zoom={zoom}
          onSeleccionar={onSeleccionar}
          onRedimensionar={onRedimensionar}
          indicadorInsercion={indicadorInsercion}
        />
      </div>
    </div>
  );
}
