/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Grupo «Macros» de la cinta (vive en la pestaña Vista, visible en los 3 tipos
 * de documento). Un toggle de grabar/detener + un gestor emergente con las
 * macros guardadas (reproducir / renombrar / borrar). La lógica (grabación,
 * normalización, persistencia, reproducción) vive en editor/macros.ts; aquí solo
 * el pegamento de interfaz.
 *
 * Reproducción: antes de despachar nada, `reproducir` valida la macro ENTERA contra el
 * registro de comandos y el permiso de ahora (no reproduce en solo lectura; ver
 * macros.ts). Después cada paso se re-despacha por el dispatch de SU editor
 * (exec/dispatchSheetCommand/dispatchSlidesCommand), aplicándose al CONTEXTO
 * ACTUAL (como el grabador de Excel). Al pasar por esos dispatch, una hoja/libro
 * protegido veta los comandos de contenido/estructura sin guardia adicional.
 * Solo se habilita reproducir una macro cuyo editor coincide con el documento
 * abierto (una macro de Hoja no tiene sentido en un folio).
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { useConfirm, usePrompt, useToast } from '@sytue/ui';
import { fetchSession } from '@sytue/auth/client';
import { appDomains } from '@sytue/config';
import type { DocKind } from '@sytue/api-clients';
import type { DocsKey, DocsT } from '../../i18n';
import type { EditorCommand, EditorContext } from '../types';
import { RibbonButton, RibbonGroup, usePopover, POPOVER_SHELL } from './primitives';
import { EditorIcon } from '../icons';
import { exec } from '../canvas/exec';
import { dispatchSheetCommand } from '../sheets/SheetCanvas';
import type { SheetCommand } from '../sheets/SheetCanvas';
import { dispatchSlidesCommand } from '../slides/SlidesCanvas';
import type { SlidesCommand } from '../slides/SlidesCanvas';
import {
  MAX_MACROS,
  MAX_NOMBRE,
  isRecording,
  loadMacros,
  macroEditor,
  newMacroId,
  reproducir,
  saveMacros,
  startRecording,
  stopRecording,
  subscribeMacros,
} from '../macros';
import type { EntornoMacro, Macro, MacroStep, Rechazo } from '../macros';

// El anillo canónico vive en `@sytue/ui/styles.css` (.sytue-focus-ring): una sola
// definición para toda la suite, en vez de la copia que cada fichero perdía a su manera.
const FOCUS_RING = 'sytue-focus-ring';

/** Nombre visible del editor de una macro (para el gestor). */
const EDITOR_LABEL: Record<DocKind, DocsKey> = {
  document: 'macros.editor.document',
  spreadsheet: 'macros.editor.spreadsheet',
  presentation: 'macros.editor.presentation',
};

const editorLabel = (t: DocsT, kind: DocKind): string => t(EDITOR_LABEL[kind]);

/** Enruta un paso al dispatch de su editor. El `cmd` guardado se estrecha al
 *  union del editor (dato propio y saneado al cargar; el dispatch ignora los
 *  desconocidos). */
const runStep = (step: MacroStep): void => {
  if (step.editor === 'spreadsheet') dispatchSheetCommand(step.cmd as SheetCommand, step.value);
  else if (step.editor === 'presentation')
    dispatchSlidesCommand(step.cmd as SlidesCommand, step.value);
  else exec(step.cmd as EditorCommand, step.value);
};

/** Orígenes de los que puede venir una imagen de una macro: esta página y Sytue Drive. */
const origenesDeLaCasa = (): EntornoMacro['origenes'] => {
  const origen = (url: string): string[] => {
    try {
      return [new URL(url).origin];
    } catch {
      return [];
    }
  };
  return { propio: window.location.origin, drive: origen(appDomains.drive) };
};

/** El motivo, dicho a quien pulsó «Reproducir». */
const rechazoTexto = (t: DocsT, motivo: Rechazo, paso: number): string =>
  motivo === 'soloLectura'
    ? t('macros.blocked.readOnly')
    : t('macros.blocked.step', { n: paso + 1 });

export function MacrosGroup({ ctx }: { ctx: EditorContext }): JSX.Element {
  // Grabar, renombrar y borrar una macro pedían su dato con el diálogo del
  // navegador; ahora usan el de la casa (ver useConfirm/usePrompt en @sytue/ui).
  const { confirmar, dialogo: dialogoConfirmar } = useConfirm();
  const { preguntar, dialogo: dialogoPreguntar } = usePrompt();
  const { notify } = useToast();
  const t = ctx.t;
  const { open, setOpen, rootRef } = usePopover();

  // El estado de grabación vive en el módulo (cambia fuera de este componente).
  const [, force] = useState(0);
  useEffect(() => subscribeMacros(() => force((n) => n + 1)), []);
  const recording = isRecording();

  // sub del usuario = clave de almacenamiento por cuenta (best-effort; sin él, «anon»).
  const [userSub, setUserSub] = useState<string | undefined>(undefined);
  useEffect(() => {
    let alive = true;
    void fetchSession().then((u) => {
      if (alive && u) setUserSub(u.id);
    });
    return () => {
      alive = false;
    };
  }, []);

  const [macros, setMacros] = useState<Macro[]>([]);
  useEffect(() => {
    setMacros(loadMacros(userSub));
  }, [userSub]);

  const persist = (next: Macro[]): void => {
    setMacros(next);
    saveMacros(next, userSub);
  };

  /** Toggle grabar/detener. Al detener, pide nombre y guarda (cancelar = descartar). */
  const toggleRecord = async (): Promise<void> => {
    if (!recording) {
      startRecording();
      return;
    }
    const { steps, descartados } = stopRecording();
    if (steps.length === 0) {
      notify({
        tone: 'info',
        title: t('macros.empty.title'),
        description: t('macros.empty.description'),
      });
      return;
    }
    if (macros.length >= MAX_MACROS) {
      notify({ tone: 'warning', title: t('macros.limit', { n: MAX_MACROS }) });
      return;
    }
    const name = await preguntar({
      mensaje: t('macros.namePrompt'),
      valor: t('macros.defaultName', { n: macros.length + 1 }),
    });
    if (name === null) return; // cancelar = descartar la grabación
    const trimmed = name.trim().slice(0, MAX_NOMBRE);
    if (trimmed === '') return;
    persist([...macros, { id: newMacroId(), name: trimmed, steps }]);
    notify({
      tone: 'success',
      title: t('macros.savedToast'),
      description: t('macros.savedToast.description', { name: trimmed, count: steps.length }),
    });
    if (descartados > 0) {
      notify({ tone: 'info', title: t('macros.discarded', { count: descartados }) });
    }
  };

  const play = (m: Macro): void => {
    setOpen(false);
    const veredicto = reproducir(
      m.steps,
      { kind: ctx.kind, soloLectura: ctx.soloLectura, origenes: origenesDeLaCasa() },
      runStep,
    );
    if (!veredicto.ok) {
      notify({
        tone: 'warning',
        title: t('macros.blocked.title'),
        description: rechazoTexto(t, veredicto.motivo, veredicto.paso),
      });
    }
  };

  const rename = async (m: Macro): Promise<void> => {
    const name = await preguntar({
      titulo: t('macros.renameTitle'),
      mensaje: t('macros.renamePrompt'),
      valor: m.name,
    });
    if (name === null) return;
    const trimmed = name.trim().slice(0, MAX_NOMBRE);
    if (trimmed === '') return;
    persist(macros.map((x) => (x.id === m.id ? { ...x, name: trimmed } : x)));
  };

  const remove = async (m: Macro): Promise<void> => {
    const ok = await confirmar({
      titulo: t('macros.deleteTitle'),
      mensaje: t('macros.deleteConfirm', { name: m.name }),
      confirmLabel: t('macros.delete'),
      tone: 'danger',
    });
    if (!ok) return;
    persist(macros.filter((x) => x.id !== m.id));
  };

  return (
    <RibbonGroup label={t('group.macros')}>
      <RibbonButton
        commandId="macroRecord"
        icon={recording ? 'stopRec' : 'record'}
        label={t(recording ? 'macros.stop' : 'macros.record')}
        active={recording}
        onAction={() => void toggleRecord()}
      />
      <div ref={rootRef} class="relative">
        <button
          type="button"
          class={
            'text-text-muted inline-flex h-7 w-7 items-center justify-center rounded-sm ' +
            'hover:bg-bg-subtle hover:text-text aria-expanded:bg-selected aria-expanded:text-text ' +
            FOCUS_RING
          }
          aria-label={t('macros.manage')}
          title={t('macros.manage')}
          aria-haspopup="true"
          aria-expanded={open}
          onMouseDown={(e) => e.preventDefault()}
          onClick={() => setOpen((v) => !v)}
        >
          <EditorIcon name="macro" />
        </button>
        {open && (
          <div
            role="menu"
            aria-label={t('macros.saved.label')}
            class={`${POPOVER_SHELL} left-0 w-[280px] p-1`}
          >
            {userSub === undefined && (
              <p class="text-text-muted border-border border-b px-2.5 py-2 text-xs">
                {t('macros.noPersist')}
              </p>
            )}
            {macros.length === 0 ? (
              <p class="text-text-muted px-2.5 py-3 text-xs">{t('macros.none')}</p>
            ) : (
              macros.map((m) => {
                const kind = macroEditor(m);
                const playable = kind === ctx.kind && !recording && !ctx.soloLectura;
                return (
                  <div
                    key={m.id}
                    class="hover:bg-bg-subtle flex items-center gap-1 rounded-sm px-1.5 py-1"
                  >
                    <div class="min-w-0 flex-1">
                      <div class="text-text truncate text-sm" title={m.name}>
                        {m.name}
                      </div>
                      <div class="sytue-meta-dense">
                        {t('macros.steps', { count: m.steps.length })} · {editorLabel(t, kind)}
                      </div>
                    </div>
                    <RibbonButton
                      icon="play"
                      label={
                        playable
                          ? t('macros.play')
                          : ctx.soloLectura
                            ? t('macros.blocked.readOnly')
                            : t('macros.notPlayable', { editor: editorLabel(t, kind) })
                      }
                      disabled={!playable}
                      onAction={() => play(m)}
                    />
                    <RibbonButton
                      icon="pencil"
                      label={t('macros.rename')}
                      onAction={() => void rename(m)}
                    />
                    <RibbonButton
                      icon="trash"
                      label={t('macros.delete')}
                      onAction={() => void remove(m)}
                    />
                  </div>
                );
              })
            )}
          </div>
        )}
      </div>
      {dialogoConfirmar}
      {dialogoPreguntar}
    </RibbonGroup>
  );
}
