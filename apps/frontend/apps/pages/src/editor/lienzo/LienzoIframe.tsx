/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Iframe aislado para el lienzo del editor de Sytue Web (PAGES-PLAN §14.1).
 *
 * Características:
 * - sandbox="allow-same-origin" SIN allow-scripts.
 * - tabindex="-1" y body.inert = true tras cada pintado para que Tab no entre en la página.
 * - Primer pintado por srcdoc; posteriores mediante sustitución directa en body.innerHTML
 *   y <style id="sp-theme"> en requestAnimationFrame.
 * - pointer-events: none para delegar toda interacción a la capa superpuesta del padre.
 */

import { useEffect, useRef } from 'preact/hooks';
import type { JSX } from 'preact';
import type { PuntoDeCorte } from '../modelo/tipos';
import { ANCHOS_PUNTO_DE_CORTE } from '../modelo/tipos';

export interface LienzoIframeProps {
  paginaNombre: string;
  puntoDeCorte: PuntoDeCorte;
  html: string;
  css: string;
  onDocReady?: (doc: Document | null) => void;
}

export function LienzoIframe({
  paginaNombre,
  puntoDeCorte,
  html,
  css,
  onDocReady,
}: LienzoIframeProps): JSX.Element {
  const iframeRef = useRef<HTMLIFrameElement>(null);
  const inicializadoRef = useRef(false);
  const anchoPx = ANCHOS_PUNTO_DE_CORTE[puntoDeCorte];

  // Documento inicial completo para srcdoc
  const srcdocInicial = `<!DOCTYPE html>
<html lang="es">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <style id="sp-theme">${css}</style>
</head>
<body inert>${html}</body>
</html>`;

  // Manejo de actualizaciones posteriores mediante requestAnimationFrame
  useEffect(() => {
    if (!inicializadoRef.current) return;
    const iframe = iframeRef.current;
    if (!iframe) return;

    const doc = iframe.contentDocument;
    if (!doc || !doc.body) return;

    const animId = requestAnimationFrame(() => {
      try {
        let styleEl = doc.getElementById('sp-theme');
        if (!styleEl) {
          styleEl = doc.createElement('style');
          styleEl.id = 'sp-theme';
          doc.head.appendChild(styleEl);
        }
        if (styleEl.textContent !== css) {
          styleEl.textContent = css;
        }

        doc.body.innerHTML = html;
        doc.body.inert = true;
        onDocReady?.(doc);
      } catch {
        // En entornos sin acceso directo al documento
      }
    });

    return () => cancelAnimationFrame(animId);
  }, [html, css, onDocReady]);

  const handleLoad = () => {
    inicializadoRef.current = true;
    const iframe = iframeRef.current;
    if (iframe && iframe.contentDocument) {
      try {
        iframe.contentDocument.body.inert = true;
        onDocReady?.(iframe.contentDocument);
      } catch {
        //
      }
    }
  };

  return (
    <iframe
      ref={iframeRef}
      srcdoc={srcdocInicial}
      sandbox="allow-same-origin"
      tabIndex={-1}
      title={`Lienzo: ${paginaNombre} · ${puntoDeCorte}`}
      onLoad={handleLoad}
      style={{
        width: `${anchoPx}px`,
        minHeight: '100%',
        pointerEvents: 'none',
        border: 'none',
        backgroundColor: 'transparent',
      }}
      class="block bg-white shadow-md transition-all duration-150"
    />
  );
}
