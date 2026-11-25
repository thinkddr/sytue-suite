/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL EDITOR DE CADA TIPO DE DOCUMENTO, BAJO DEMANDA.
 *
 * Docus, Calcus y Diapos montan la misma isla (`EditorApp`), y la isla traía los tres
 * editores: el lienzo de hoja con su motor de fórmulas, el de diapositivas y el folio,
 * con todas las pestañas de la cinta (el trozo del Ribbon pesaba 1,12 MB). Un documento
 * solo usa el suyo. Lo que depende del tipo —el lienzo, los cuerpos de las pestañas, sus
 * atajos y su franja de aviso— vive en un módulo por tipo (`documento`, `hoja`,
 * `presentacion`) que se baja con `import()` en cuanto se sabe el tipo: la página lo
 * dice (`kind`), así que llega a la vez que el documento; si no lo dice, al llegar este.
 *
 * Lo común (cabecera, barra de estado, paneles, el armazón de la cinta) sigue en la isla,
 * y habla con los lienzos por su mando (`sheets/mando.ts`, `slides/mando.ts`), no con
 * ellos.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import type { DocKind } from '@sytue/api-clients';
import type { Locale } from '@sytue/i18n';
import type { GrupoDeAtajos } from '@sytue/ui';
import type { DocsT } from '../../i18n';
import type { RibbonTabId } from '../pestanas';
import type { EditorContext } from '../types';

/** Lo que se pinta dentro de una pestaña de la cinta. */
export type CuerpoDePestana = (props: { ctx: EditorContext }) => JSX.Element;

export interface EditorDeTipo {
  /** El lienzo: el folio, la hoja o las diapositivas. */
  readonly Lienzo: (props: { ctx: EditorContext }) => JSX.Element;
  /**
   * El cuerpo de cada pestaña que se ve en este tipo (`pestanasDe(kind)`, ni más ni
   * menos: lo comprueba `test/taxonomia.test.ts`).
   */
  readonly cuerpos: Readonly<Partial<Record<RibbonTabId, CuerpoDePestana>>>;
  /** Los atajos propios del tipo, para la hoja de atajos (Ctrl+/). */
  readonly atajos: (t: DocsT) => GrupoDeAtajos[];
  /** La franja de aviso propia del tipo, si la tiene (la hoja protegida). */
  readonly Franja?: ((props: { locale: Locale }) => JSX.Element | null) | undefined;
}

const modulos: Record<DocKind, () => Promise<{ editor: EditorDeTipo }>> = {
  document: () => import('./documento'),
  spreadsheet: () => import('./hoja'),
  presentation: () => import('./presentacion'),
};

const cargados = new Map<DocKind, EditorDeTipo>();
const cargando = new Map<DocKind, Promise<EditorDeTipo>>();

/** Baja el editor del tipo. Idempotente; si el trozo no llega, el siguiente intento repite. */
export function cargarEditorDe(kind: DocKind): Promise<EditorDeTipo> {
  const listo = cargados.get(kind);
  if (listo) return Promise.resolve(listo);
  let promesa = cargando.get(kind);
  if (!promesa) {
    promesa = modulos[kind]().then(
      ({ editor }) => {
        cargados.set(kind, editor);
        return editor;
      },
      (error: unknown) => {
        cargando.delete(kind);
        throw error;
      },
    );
    cargando.set(kind, promesa);
  }
  return promesa;
}

/** El editor del tipo si ya está cargado (sin esperar). */
export function editorCargado(kind: DocKind): EditorDeTipo | undefined {
  return cargados.get(kind);
}

export interface EstadoEditorDeTipo {
  editor: EditorDeTipo | null;
  /** El trozo no llegó: el editor no se puede montar (sin red, un despliegue a medias). */
  fallo: boolean;
}

/**
 * El editor del tipo `kind`, en cuanto está. Con `null` (aún no se sabe el tipo) no
 * pide nada. Si ya estaba cargado, lo da en el primer pintado, sin parpadeo.
 */
export function useEditorDe(kind: DocKind | null): EstadoEditorDeTipo {
  const [estado, setEstado] = useState<EstadoEditorDeTipo & { kind: DocKind | null }>(() => ({
    kind,
    editor: kind ? (editorCargado(kind) ?? null) : null,
    fallo: false,
  }));
  useEffect(() => {
    if (!kind) return;
    const ya = editorCargado(kind);
    if (ya) {
      setEstado((e) =>
        e.kind === kind && e.editor === ya ? e : { kind, editor: ya, fallo: false },
      );
      return;
    }
    let vigente = true;
    setEstado({ kind, editor: null, fallo: false });
    cargarEditorDe(kind).then(
      (editor) => vigente && setEstado({ kind, editor, fallo: false }),
      () => vigente && setEstado({ kind, editor: null, fallo: true }),
    );
    return () => {
      vigente = false;
    };
  }, [kind]);
  // Un cambio de tipo no enseña el editor del anterior ni un instante.
  if (estado.kind !== kind) {
    const ya = kind ? editorCargado(kind) : undefined;
    return { editor: ya ?? null, fallo: false };
  }
  return { editor: estado.editor, fallo: estado.fallo };
}
