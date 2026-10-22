/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Anfitrión del editor de dibujo dentro del folio de Docus. Es la pieza que une el
 * bloque `<figure data-dibujo>` (./../dibujo.ts) con el diálogo (./DibujoEditor):
 *
 *  - Abre el editor al pedirlo la cinta, con doble clic sobre un dibujo del folio
 *    o con Intro/Espacio sobre el dibujo enfocado (teclado).
 *  - Al aceptar, hornea la escena (data-escena + SVG regenerado) y la imagen de
 *    respaldo en la figura —o inserta un bloque nuevo en el caret capturado— y emite
 *    `input` para que el autosave y la colaboración lo recojan como cualquier edición.
 *
 * Vive montado junto al folio (y no dentro de la pestaña Insertar de la cinta, que
 * solo existe mientras esa pestaña está a la vista) para que el doble clic funcione
 * siempre.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { useUltimo } from '@sytue/ui';
import { escenaVacia } from '@sytue/escena';
import type { Escena } from '@sytue/escena';
import type { EditorContext } from '../types';
import { getEditorRoot } from '../canvas/exec';
import { captureRange, figureFromSelection, insertBlock, tmpId } from '../ribbon/tabs/BloquesDoc';
import {
  DIBUJO_FIGURE,
  anclaDibujo,
  asegurarIdDibujo,
  nuevoIdDibujo,
  EVENTO_ABRIR_DIBUJO,
  dibujoFigureHtml,
  hornearFigura,
  leerFigura,
  rasterizar,
} from '../dibujo';
import { DibujoEditor } from './DibujoEditor';

interface Sesion {
  escena: Escena;
  /** Figura que se edita; `null` = bloque nuevo. */
  fig: HTMLElement | null;
  /** Caret donde se insertará un bloque nuevo. */
  rango: Range | null;
  /** Id estable del dibujo (de la figura, o el que llevará al insertarse). */
  id: string;
}

/** Tamaño de un dibujo nuevo: 16:10, cabe a lo ancho del folio. */
const NUEVO = { ancho: 640, alto: 400 };

export function DibujoHost({ ctx }: { ctx: EditorContext }): JSX.Element | null {
  const t = ctx.t;
  const [sesion, setSesion] = useState<Sesion | null>(null);
  const abrir = (fig: HTMLElement | null): void => {
    if (ctx.soloLectura) return;
    const root = getEditorRoot();
    if (!root) return;
    const nueva = (): Escena =>
      escenaVacia({ ...NUEVO, nombreCapa: t('drawing.layer.default', { n: 1 }) });
    if (!fig) {
      setSesion({ escena: nueva(), fig: null, rango: captureRange(root), id: nuevoIdDibujo() });
      return;
    }
    const lectura = leerFigura(fig);
    // Un dibujo de una versión más nueva no se abre: guardarlo borraría lo que
    // este editor no entiende. Se sigue viendo (su SVG) pero no se edita.
    if (lectura && !lectura.ok) return;
    // Los dibujos anteriores a los comentarios no tienen id: se lo damos y se persiste.
    const { id, nuevo } = asegurarIdDibujo(fig);
    if (nuevo) root.dispatchEvent(new Event('input', { bubbles: true }));
    setSesion({
      escena: lectura?.ok === true ? lectura.escena : nueva(),
      fig,
      rango: null,
      id,
    });
  };
  // Los oyentes se registran una vez: leen siempre la última versión de `abrir`.
  const abrirRef = useUltimo(abrir);

  useEffect(() => {
    const dentro = (el: EventTarget | null): HTMLElement | null => {
      const fig = el instanceof Element ? el.closest<HTMLElement>(DIBUJO_FIGURE) : null;
      const root = getEditorRoot();
      return fig && root?.contains(fig) ? fig : null;
    };
    const pedido = (): void => {
      const root = getEditorRoot();
      abrirRef.current(root ? figureFromSelection(root, DIBUJO_FIGURE) : null);
    };
    const doble = (e: MouseEvent): void => {
      const fig = dentro(e.target);
      if (fig) {
        e.preventDefault();
        abrirRef.current(fig);
      }
    };
    const tecla = (e: KeyboardEvent): void => {
      if (e.key !== 'Enter' && e.key !== ' ') return;
      const fig =
        e.target instanceof HTMLElement && e.target.matches(DIBUJO_FIGURE)
          ? dentro(e.target)
          : null;
      if (fig) {
        e.preventDefault();
        abrirRef.current(fig);
      }
    };
    window.addEventListener(EVENTO_ABRIR_DIBUJO, pedido);
    document.addEventListener('dblclick', doble);
    document.addEventListener('keydown', tecla);
    return () => {
      window.removeEventListener(EVENTO_ABRIR_DIBUJO, pedido);
      document.removeEventListener('dblclick', doble);
      document.removeEventListener('keydown', tecla);
    };
  }, [abrirRef]);

  const guardar = async (escena: Escena): Promise<void> => {
    const s = sesion;
    setSesion(null);
    const root = getEditorRoot();
    if (!s || !root) return;
    const etiqueta = t('drawing.title');
    // Sin imagen de respaldo (el navegador no pudo pintar el SVG) el bloque se guarda
    // igual; los ficheros exportados lo dejan fuera. Un respaldo viejo NO se conserva:
    // enseñaría un dibujo que ya no es el actual.
    const respaldo = await rasterizar(escena);
    if (s.fig && root.contains(s.fig)) {
      hornearFigura(s.fig, escena, { etiqueta, respaldo });
      root.dispatchEvent(new Event('input', { bubbles: true }));
      s.fig.focus({ preventScroll: true });
      return;
    }
    const id = tmpId();
    const fig = insertBlock(
      s.rango,
      dibujoFigureHtml(escena, { etiqueta, respaldo, tmpId: id, id: s.id }),
      id,
    );
    fig?.focus({ preventScroll: true });
  };

  if (!sesion) return null;
  return (
    <DibujoEditor
      open
      inicial={sesion.escena}
      aceptar={sesion.fig ? t('action.apply') : t('action.insert')}
      onComentar={
        sesion.fig
          ? async (cuerpo, destino) => ctx.addComment(cuerpo, anclaDibujo(sesion.id, destino))
          : undefined
      }
      onGuardar={(e) => void guardar(e)}
      onCancelar={() => setSesion(null)}
    />
  );
}
