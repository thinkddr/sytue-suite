/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El grupo «Bloques» de la cinta, común a Docus, Calcus y Diapos: inserta un bloque
 * reutilizable —portada, tabla de firmas, pie legal, agenda, bloque de totales…— del
 * catálogo de Sytue o de la biblioteca de la organización. La lista y el contenido los da
 * el servicio (GET /api/blocks), que lo entrega en el idioma de la sesión y vestido con
 * la marca vigente de la organización; aquí solo se coloca:
 *
 *   · documento: el fragmento, donde estaba el caret (insertHTML, con su deshacer);
 *   · hoja: en la celda activa (comando `bloquehoja`, sheets/bloque.ts);
 *   · presentación: una diapositiva detrás de la actual (comando `bloque`, slides/bloque.ts).
 *
 * El HTML de un bloque de la organización es el documento de otra persona: el servicio lo
 * sanea al subconjunto del editor antes de entregarlo (domain.ExtraerBloque).
 */
import type { JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import { useToast } from '@sytue/ui';
import { executeEditingCommand } from '@sytue/ui/browser/editing';
import type { DocKind } from '@sytue/api-clients';
import { getEditorRoot } from '../canvas/exec';
import { useT } from '../context';
import { dispatchSheetCommand } from '../sheets/SheetCanvas';
import { dispatchSlidesCommand } from '../slides/SlidesCanvas';
import { RibbonGroup, RibbonSelect } from './primitives';

interface Bloque {
  key: string;
  name: string;
  /** Solo los de la organización: sus versiones publicadas, la vigente primero. */
  versions?: { no: number }[];
}

interface Lista {
  blocks: Bloque[];
  org_blocks: Bloque[];
}

/** Coloca el contenido de un bloque según el tipo de documento. */
function colocar(kind: DocKind, contenido: string, caret: Range | null): void {
  if (kind === 'spreadsheet') {
    dispatchSheetCommand('bloquehoja', contenido);
    return;
  }
  if (kind === 'presentation') {
    dispatchSlidesCommand('bloque', contenido);
    return;
  }
  const root = getEditorRoot();
  if (!root) return;
  root.focus({ preventScroll: true });
  const sel = document.getSelection();
  if (sel && caret && root.contains(caret.commonAncestorContainer)) {
    sel.removeAllRanges();
    sel.addRange(caret);
  }
  // insertHTML emite `input`: el autoguardado se programa solo y entra en el deshacer.
  executeEditingCommand('insertHTML', `${contenido}<p><br></p>`);
}

export function BloquesGroup({ kind }: { kind: DocKind }): JSX.Element | null {
  const t = useT();
  const toast = useToast();
  const [lista, setLista] = useState<Lista | null>(null);
  const caretRef = useRef<Range | null>(null);

  useEffect(() => {
    let vigente = true;
    fetch(`/api/blocks?kind=${kind}`, { credentials: 'include' })
      .then((res) => (res.ok ? (res.json() as Promise<Lista>) : null))
      .then((d) => vigente && setLista(d))
      .catch(() => undefined);
    return () => {
      vigente = false;
    };
  }, [kind]);

  if (!lista) return null;

  /** El desplegable se lleva el foco: el caret del folio se guarda antes de abrirlo. */
  const guardarCaret = (): void => {
    const root = getEditorRoot();
    const sel = document.getSelection();
    if (!root || !sel || sel.rangeCount === 0) return;
    const r = sel.getRangeAt(0);
    if (root.contains(r.commonAncestorContainer)) caretRef.current = r.cloneRange();
  };

  const insertar = async (key: string): Promise<void> => {
    try {
      const res = await fetch(`/api/blocks?key=${encodeURIComponent(key)}`, {
        credentials: 'include',
      });
      if (!res.ok) throw new Error(String(res.status));
      const { content } = (await res.json()) as { content?: string };
      if (!content) throw new Error('vacío');
      colocar(kind, content, caretRef.current);
    } catch {
      toast.notify({ tone: 'danger', title: t('blocks.msg.failed') });
    }
  };

  const opciones = [
    { value: '', label: t('blocks.placeholder') },
    ...lista.blocks.map((b) => ({ value: b.key, label: b.name })),
    ...lista.org_blocks.map((b) => ({
      value: b.key,
      label: t('blocks.org', { name: b.name, version: b.versions?.[0]?.no ?? 1 }),
    })),
  ];
  return (
    <RibbonGroup label={t('blocks.group')}>
      <div onPointerDownCapture={guardarCaret} onFocusCapture={guardarCaret}>
        <RibbonSelect
          commandId={kind === 'spreadsheet' ? 'bloquehoja' : 'bloque'}
          width={150}
          value=""
          options={opciones}
          onSelect={(v) => {
            if (v) void insertar(v);
          }}
        />
      </div>
    </RibbonGroup>
  );
}
