/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El editor entero (cabecera, cinta, lienzo, barra de estado) con un contexto de mentira
 * y contenido de muestra, para medir y capturar el aspecto de las TRES superficies de
 * edición —la rejilla de Calcus, el lienzo de Diapos y la página de Docus— sin sesión ni
 * servicio. Es la hermana de `CintaDemo`: la cinta sola mide maquetación; esta mide
 * acabado. Solo la sirve `pages/dev/superficies.astro` y solo en desarrollo.
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import { DialogosDelEditor, ToastProvider } from '@sytue/ui';
import type { DocKind, DocumentMeta } from '@sytue/api-clients';
import { docsT } from '../../i18n';
import '../editor.css';
import { EMPTY_FORMAT } from '../types';
import type { CommentItem, EditorContext, PeerCursor } from '../types';
import { EditorCtx, EditorI18nCtx } from '../context';
import { EditorLayout } from '../layout/EditorLayout';
import { EditorHeader } from '../layout/EditorHeader';
import { StatusBar } from '../layout/StatusBar';
import { LeftRail } from '../layout/LeftRail';
import { Ribbon } from '../ribbon/Ribbon';
import { PageCanvas } from '../canvas/PageCanvas';
import { SheetCanvas } from '../sheets/SheetCanvas';
import { SlidesCanvas } from '../slides/SlidesCanvas';
import { documentoDeMuestra, hojaDeMuestra, presentacionDeMuestra } from './superficies-fixtures';

const nada = (): void => undefined;
const nadaAsync = (): Promise<void> => Promise.resolve();

const comentario: CommentItem = {
  id: 'c1',
  document_id: 'demo',
  author_sub: 'u2',
  author_name: 'Marta Ruiz',
  anchor: null,
  body: 'Falta citar la fuente de este dato.',
  resolved: false,
  created_at: '2026-10-01T10:00:00Z',
  updated_at: '2026-10-01T10:00:00Z',
};

function contenidoInicial(kind: DocKind, filas: number): string {
  if (kind === 'spreadsheet') return JSON.stringify(hojaDeMuestra(filas));
  if (kind === 'presentation') return JSON.stringify(presentacionDeMuestra());
  return documentoDeMuestra();
}

export function SuperficiesDemo(props: {
  kind: DocKind;
  locale: 'es' | 'en' | 'de' | 'fr';
  soloLectura: boolean;
  remotos: boolean;
  filas: number;
}): JSX.Element {
  const t = docsT(props.locale);
  const [content, setContent] = useState(() => contenidoInicial(props.kind, props.filas));
  const [ui, setUiState] = useState<EditorContext['ui']>({
    panel: null,
    settingsOpen: false,
    ruler: true,
    zoom: 100,
    formFill: false,
    track: false,
  });
  const [pageSetup, setPageSetup] = useState({ margin: 96, landscape: false });
  const [stats, setStats] = useState({ pages: 1, page: 1 });
  const remotos: PeerCursor[] = props.remotos
    ? [
        props.kind === 'spreadsheet'
          ? {
              id: 'u2',
              name: 'Marta Ruiz',
              color: 'var(--sytue-avatar-2)',
              pos: { kind: 'sheet', sheet: 0, c1: 3, r1: 5, c2: 3, r2: 5 },
            }
          : props.kind === 'presentation'
            ? {
                id: 'u2',
                name: 'Marta Ruiz',
                color: 'var(--sytue-avatar-2)',
                pos: { kind: 'slide', slide: 1, boxes: ['a2'] },
              }
            : {
                id: 'u2',
                name: 'Marta Ruiz',
                color: 'var(--sytue-avatar-2)',
                pos: { kind: 'doc', anchor: 40, focus: 60 },
              },
      ]
    : [];
  const ctx: EditorContext = {
    doc: {
      id: 'demo',
      kind: props.kind,
      title: 'Operaciones T3',
    } as unknown as DocumentMeta,
    kind: props.kind,
    locale: props.locale,
    t,
    aiEnabled: false,
    title: 'Operaciones T3',
    setTitle: nada,
    save: 'saved',
    connection: 'online',
    content,
    contentEpoch: 1,
    onContentInput: setContent,
    fmt: EMPTY_FORMAT,
    exec: nada,
    ui,
    setUi: (patch) => setUiState((prev) => ({ ...prev, ...patch })),
    pageSetup,
    setPageSetup: (patch) => setPageSetup((prev) => ({ ...prev, ...patch })),
    stats: { words: 120, pages: stats.pages, page: stats.page },
    reportStats: (patch) =>
      setStats((prev) => {
        const next = { pages: patch.pages ?? prev.pages, page: patch.page ?? prev.page };
        return next.pages === prev.pages && next.page === prev.page ? prev : next;
      }),
    peers: [],
    peerCursors: remotos,
    sendCursor: nada,
    comments: props.kind === 'document' ? [comentario] : [],
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
    esPropietario: false,
    cursorDots: { aPuntos: () => null, aOffsets: () => null },
    soloLectura: props.soloLectura,
    savingToDrive: false,
    saveToDrive: nadaAsync,
  };
  const canvas =
    ctx.kind === 'spreadsheet' ? (
      <SheetCanvas ctx={ctx} />
    ) : ctx.kind === 'presentation' ? (
      <SlidesCanvas ctx={ctx} />
    ) : (
      <PageCanvas ctx={ctx} />
    );
  return (
    <ToastProvider>
      <EditorI18nCtx.Provider value={t}>
        <EditorCtx.Provider value={ctx}>
          <div class="h-dvh">
            <EditorLayout
              header={<EditorHeader ctx={ctx} />}
              ribbon={<Ribbon ctx={ctx} />}
              rail={<LeftRail ctx={ctx} />}
              main={canvas}
              settings={null}
              status={<StatusBar ctx={ctx} />}
            />
          </div>
        </EditorCtx.Provider>
      </EditorI18nCtx.Provider>
      <DialogosDelEditor />
    </ToastProvider>
  );
}
