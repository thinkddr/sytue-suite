/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Pestaña «Vista» del ribbon (BLUEPRINT.md §6.3 + tarea B6): zoom con presets
 * 50–200 % y «Ajustar al ancho», conmutadores de regla y panel de ajustes
 * (icon-buttons con aria-pressed, §6.3), pantalla completa (Fullscreen API) y
 * tema del lienzo (claro/sepia sutil — SOLO el fondo del área de trabajo; el
 * folio sigue siendo papel blanco).
 *
 * Decisiones fuera del blueprint (la tarea manda; documentadas aquí):
 * - EditorUi (§2) no contempla «tema del lienzo» y types.ts/editor.css
 *   pertenecen a A1: el ajuste vive como estado de módulo (sobrevive al cambio
 *   de pestaña) aplicado por DOM directo.
 * - El hex sepia debería ser una var de editor.css (§0.5); al no poder tocar
 *   ese fichero, la constante local se declara aquí y migrará a editor.css
 *   cuando A1 amplíe el contrato.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import type { EditorContext } from '../../types';
import { RibbonButton, RibbonGroup, RibbonSelect, RibbonSeparator } from '../primitives';
import { MacrosGroup } from '../MacrosGroup';
import { ScriptsGroup } from '../ScriptsGroup';
import { MIN_ZOOM, MAX_ZOOM } from '../../canvas/metrics';

/* ---------- Zoom (§6.3: presets; el rango lo fija canvas/metrics) ---------- */

/* El suelo y el techo son los del deslizador de la barra de estado y los del
 * ajuste inicial al ancho (canvas/metrics). Aquí estaban escritos a mano con un
 * suelo del 50 %, y con él «Ajustar al ancho» NO ajustaba nada en un teléfono: un
 * A4 pide ~37 % para caber en 360 px, así que el cálculo bueno se recortaba al 50
 * y el folio seguía saliéndose. Y el «−» se apagaba al 50 mientras el deslizador
 * de abajo seguía bajando: dos controles del mismo zoom con distinto tope. */
const ZOOM_PRESETS = [50, 75, 100, 125, 150, 200] as const;
const ZOOM_OPTIONS = ZOOM_PRESETS.map((v) => ({ value: String(v), label: `${v} %` }));
/** Aire visual (px) que se deja a cada lado del folio al ajustar al ancho. */
const FIT_GAP = 48;

function clampZoom(next: number): number {
  return Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, Math.round(next)));
}

/** El preset más cercano al zoom actual (§6.3: si no es un preset, se muestra el más próximo). */
function nearestPreset(zoom: number): number {
  return ZOOM_PRESETS.reduce((best, p) => (Math.abs(p - zoom) < Math.abs(best - zoom) ? p : best));
}

/* ---------- Tema del lienzo (estado de módulo) ---------- */

type CanvasTheme = 'claro' | 'sepia';

/**
 * Sepia sutil para el fondo del área de trabajo (tarea B6). Único hex local:
 * tono papel-cálido cercano al greige cálido de --editor-canvas-bg (#e9e6dd).
 * ponytail: debería vivir en editor.css (A1); migrar cuando se amplíe §8.
 */
const SEPIA_CANVAS_BG = 'var(--editor-canvas-bg-sepia)';

/* ponytail: EditorUi (§2) no da hueco al tema del lienzo; estado de módulo
 * (persiste al desmontar la pestaña) + DOM directo. Camino de mejora: campo
 * canvasTheme en EditorUi cuando A1 amplíe el contrato. */
let canvasTheme: CanvasTheme = 'claro';

/** Raíz del editor (EditorLayout): ahí viven las vars --editor-* de editor.css. */
function editorRoot(): HTMLElement | null {
  return document.querySelector<HTMLElement>('.sytue-editor');
}

/** Tiñe SOLO el fondo del área de trabajo vía --editor-canvas-bg. El override
 *  inline manda también en modo oscuro (elección explícita del usuario);
 *  «Claro» lo retira y restaura el tema por defecto, incluido el oscuro. */
function applyCanvasTheme(theme: CanvasTheme): void {
  canvasTheme = theme;
  const root = editorRoot();
  if (!root) return;
  if (theme === 'sepia') root.style.setProperty('--editor-canvas-bg', SEPIA_CANVAS_BG);
  else root.style.removeProperty('--editor-canvas-bg');
}

/* ---------- Pestaña Vista ---------- */

export function VistaTab({ ctx }: { ctx: EditorContext }): JSX.Element {
  const t = ctx.t;
  const zoom = ctx.ui.zoom;
  const [theme, setTheme] = useState<CanvasTheme>(canvasTheme);
  const [fullscreen, setFullscreen] = useState(
    typeof document !== 'undefined' && document.fullscreenElement !== null,
  );

  // El estado de pantalla completa cambia también por Escape o F11: se escucha el evento.
  useEffect(() => {
    const onChange = (): void => setFullscreen(document.fullscreenElement !== null);
    document.addEventListener('fullscreenchange', onChange);
    return () => document.removeEventListener('fullscreenchange', onChange);
  }, []);

  const setZoom = (next: number): void => {
    ctx.setUi({ zoom: clampZoom(next) });
  };

  /** Zoom que hace caber el ancho del folio en la zona de scroll del lienzo.
   *  Solo kind document: la hoja es rejilla fluida y el escenario de slides ya
   *  se auto-encaja (§7.2). El folio mide en px de maqueta (offsetWidth no se
   *  escala con transform), así que el cociente da el porcentaje directamente. */
  const ajustarAlAncho = (): void => {
    const page = document.querySelector<HTMLElement>('.sytue-editor .sytue-page');
    if (!page || page.offsetWidth === 0) return;
    let scroller: HTMLElement | null = page.parentElement;
    while (scroller && !/(auto|scroll)/.test(getComputedStyle(scroller).overflowY)) {
      scroller = scroller.parentElement;
    }
    const avail = (scroller ?? editorRoot())?.clientWidth ?? 0;
    if (avail - FIT_GAP <= 0) return;
    // Redondeo a múltiplo de 5 (misma familia que los presets 75/125).
    setZoom(Math.round(((avail - FIT_GAP) / page.offsetWidth) * 20) * 5);
  };

  const pickTheme = (next: CanvasTheme): void => {
    applyCanvasTheme(next);
    setTheme(next);
  };

  const toggleFullscreen = (): void => {
    // Fullscreen API estándar; la promesa puede rechazarse (p. ej. permisos) y se ignora.
    if (document.fullscreenElement) {
      void document.exitFullscreen().catch(() => undefined);
    } else {
      void document.documentElement.requestFullscreen().catch(() => undefined);
    }
  };

  return (
    <>
      <RibbonGroup label={t('group.zoom')}>
        <RibbonButton
          commandId="zoomOut"
          icon="minus"
          disabled={zoom <= MIN_ZOOM}
          onAction={() => setZoom(zoom - 10)}
        />
        <RibbonSelect
          commandId="zoomSet"
          width={72}
          value={String(nearestPreset(zoom))}
          options={ZOOM_OPTIONS}
          onSelect={(v) => setZoom(Number(v))}
        />
        <RibbonButton
          commandId="zoomIn"
          icon="plus"
          disabled={zoom >= MAX_ZOOM}
          onAction={() => setZoom(zoom + 10)}
        />
        <RibbonButton
          commandId="fitWidth"
          icon="pageSize"
          disabled={ctx.kind !== 'document'}
          onAction={ajustarAlAncho}
        />
      </RibbonGroup>

      <RibbonSeparator />

      {/* «Mostrar» (§6.3): toggles icon-button con aria-pressed, mismo lenguaje
       * 28×28 que el resto de la cinta (iconos ruler/panelRight de §9). */}
      <RibbonGroup label={t('group.show')}>
        {ctx.kind === 'document' && (
          <RibbonButton
            commandId="ruler"
            icon="ruler"
            active={ctx.ui.ruler}
            onAction={() => ctx.setUi({ ruler: !ctx.ui.ruler })}
          />
        )}
        <RibbonButton
          commandId="settingsPanel"
          icon="panelRight"
          active={ctx.ui.settingsOpen}
          onAction={() => ctx.setUi({ settingsOpen: !ctx.ui.settingsOpen })}
        />
      </RibbonGroup>

      <RibbonSeparator />

      <RibbonGroup label={t('group.screen')}>
        <RibbonButton
          commandId="fullscreen"
          icon="fullscreen"
          active={fullscreen}
          onAction={toggleFullscreen}
        />
      </RibbonGroup>

      <RibbonSeparator />

      <RibbonGroup label={t('group.canvasTheme')}>
        <RibbonSelect
          commandId="canvasTheme"
          width={92}
          value={theme}
          options={[
            { value: 'claro', label: t('canvasTheme.light') },
            { value: 'sepia', label: t('canvasTheme.sepia') },
          ]}
          onSelect={(v) => pickTheme(v === 'sepia' ? 'sepia' : 'claro')}
        />
      </RibbonGroup>

      <RibbonSeparator />

      {/* Grabador de macros: grabar/reproducir secuencias de comandos (los 3 kinds). */}
      <MacrosGroup ctx={ctx} />

      <RibbonSeparator />

      {/* Scripts de usuario: código aislado, con manifiesto de permisos y registro. */}
      <ScriptsGroup ctx={ctx} />
    </>
  );
}
