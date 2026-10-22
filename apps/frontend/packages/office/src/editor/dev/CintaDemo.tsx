/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La cinta del editor SOLA, con un contexto de mentira: el banco de pruebas de la prueba
 * visual de Playwright (`test/e2e/cinta.mjs`), que comprueba las tres variantes
 * (documento, hoja, presentación) a 1280, 390 y 320 px.
 *
 * Existe porque el editor de verdad pide sesión y un servicio detrás, y la prueba mide
 * maquetación y teclado, no datos. Solo la sirve `pages/dev/cinta.astro` y solo en
 * desarrollo (en producción esa ruta es un 404): nada de esto llega a quien edita.
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { DialogosDelEditor, ToastProvider } from '@sytue/ui';
import type { DocKind, DocumentMeta } from '@sytue/api-clients';
import { docsT } from '../../i18n';
import '../editor.css';
import { EMPTY_FORMAT } from '../types';
import type { EditorContext } from '../types';
import { EditorCtx, EditorI18nCtx } from '../context';
import { EditorLayout } from '../layout/EditorLayout';
import { Ribbon } from '../ribbon/Ribbon';

const nada = (): void => undefined;
const nadaAsync = (): Promise<void> => Promise.resolve();

export function CintaDemo(props: {
  kind: DocKind;
  locale: 'es' | 'en' | 'de' | 'fr';
  soloLectura: boolean;
}): JSX.Element {
  const t = docsT(props.locale);
  const [ui, setUiState] = useState<EditorContext['ui']>({
    panel: null,
    settingsOpen: false,
    ruler: true,
    zoom: 100,
    formFill: false,
    track: false,
  });
  const ctx: EditorContext = {
    doc: { id: 'demo', kind: props.kind, title: 'Demo' } as unknown as DocumentMeta,
    kind: props.kind,
    locale: props.locale,
    t,
    aiEnabled: true,
    title: 'Demo',
    setTitle: nada,
    save: 'saved',
    connection: 'online',
    content: '',
    contentEpoch: 0,
    onContentInput: nada,
    fmt: EMPTY_FORMAT,
    exec: nada,
    ui,
    setUi: (patch) => setUiState((prev) => ({ ...prev, ...patch })),
    pageSetup: { margin: 96, landscape: false },
    setPageSetup: nada,
    stats: { words: 0, pages: 1, page: 1 },
    reportStats: nada,
    peers: [],
    peerCursors: [],
    sendCursor: nada,
    comments: [],
    addComment: nadaAsync,
    resolveComment: nadaAsync,
    removeComment: nadaAsync,
    versions: [],
    loadVersions: nadaAsync,
    createVersion: nadaAsync,
    restoreVersion: nadaAsync,
    applyTheme: nadaAsync,
    exportAs: nadaAsync,
    sourceRef: '',
    cursorDots: { aPuntos: () => null, aOffsets: () => null },
    soloLectura: props.soloLectura,
    esPropietario: false,
    savingToDrive: false,
    saveToDrive: nadaAsync,
  };
  return (
    <ToastProvider>
      <EditorI18nCtx.Provider value={t}>
        <EditorCtx.Provider value={ctx}>
          <div class="h-dvh">
            <EditorLayout
              header={<div class="h-10 border-b" />}
              ribbon={<Ribbon ctx={ctx} />}
              rail={null}
              settings={null}
              status={<div class="h-6 border-t" />}
              main={
                <div
                  id="demo-lienzo"
                  contentEditable
                  role="textbox"
                  aria-label="Lienzo"
                  class="m-4 min-h-40 border p-4"
                >
                  Lienzo de prueba
                </div>
              }
            />
          </div>
        </EditorCtx.Provider>
      </EditorI18nCtx.Provider>
      <DialogosDelEditor />
    </ToastProvider>
  );
}
