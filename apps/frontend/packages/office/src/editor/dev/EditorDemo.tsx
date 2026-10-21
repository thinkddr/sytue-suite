/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El editor ENTERO (cabecera, cinta, franja, paneles, lienzo y barra de estado) con un
 * documento, una hoja y una presentación de ejemplo, sin sesión ni servicio detrás: el
 * banco de pruebas de las capturas del cromo (`test/e2e/cromo.mjs`). Hermano de
 * `CintaDemo`, que solo monta la cinta.
 *
 * Solo lo sirve `pages/dev/editor.astro` y solo en desarrollo (en producción esa ruta es
 * un 404): nada de esto llega a quien edita. Lo que NO es de verdad: la red (el
 * guardado, la colaboración, el compartir) y los datos (comentarios, versiones y
 * colaboradores son fijos).
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useState } from 'preact/hooks';
import { DialogosDelEditor, ToastProvider } from '@sytue/ui';
import type { DocKind, DocumentMeta } from '@sytue/api-clients';
import type { Locale } from '@sytue/i18n';
import { docsT } from '../../i18n';
import '../editor.css';
import { EMPTY_FORMAT } from '../types';
import type { CommentItem, EditorContext, EditorUi, FormatState, PanelId } from '../types';
import { EditorCtx, EditorI18nCtx } from '../context';
import { exec, getFormatState } from '../canvas/exec';
import { EditorLayout } from '../layout/EditorLayout';
import { EditorHeader } from '../layout/EditorHeader';
import { StatusBar } from '../layout/StatusBar';
import { LeftRail } from '../layout/LeftRail';
import { SettingsPanel } from '../panels/SettingsPanel';
import { Ribbon } from '../ribbon/Ribbon';
import { PageCanvas } from '../canvas/PageCanvas';
import { SheetCanvas } from '../sheets/SheetCanvas';
import { SlidesCanvas } from '../slides/SlidesCanvas';

const nada = (): void => undefined;
const nadaAsync = (): Promise<void> => Promise.resolve();

const DOCUMENTO =
  '<h1>Informe trimestral</h1>' +
  '<p>Resumen de la actividad del tercer trimestre y de las decisiones que quedan abiertas para el comité.</p>' +
  '<h2>Resultados</h2>' +
  '<p>Los ingresos crecieron un <b>12 %</b> respecto al trimestre anterior; el margen se mantuvo en el <i>31 %</i>.</p>' +
  '<ul><li>Altas de clientes: 184</li><li>Bajas: 21</li><li>Incidencias críticas: 2</li></ul>' +
  '<h2>Riesgos</h2>' +
  '<p>El proveedor de almacenamiento ha anunciado una subida de tarifas para enero.</p>' +
  '<table><tbody><tr><td>Área</td><td>Estado</td><td>Responsable</td></tr>' +
  '<tr><td>Facturación</td><td>En curso</td><td>Marta</td></tr>' +
  '<tr><td>Soporte</td><td>Cerrado</td><td>Iván</td></tr></tbody></table>';

const HOJA = JSON.stringify({
  active: 0,
  sheets: [
    {
      name: 'Ventas',
      cells: {
        A1: 'Producto',
        B1: 'T1',
        C1: 'T2',
        D1: 'T3',
        E1: 'Total',
        A2: 'Licencias',
        B2: '1200',
        C2: '1350',
        D2: '1510',
        E2: '=SUM(B2:D2)',
        A3: 'Soporte',
        B3: '640',
        C3: '655',
        D3: '700',
        E3: '=SUM(B3:D3)',
        A4: 'Formación',
        B4: '210',
        C4: '330',
        D4: '280',
        E4: '=SUM(B4:D4)',
        A5: 'Total',
        B5: '=SUM(B2:B4)',
        C5: '=SUM(C2:C4)',
        D5: '=SUM(D2:D4)',
        E5: '=SUM(E2:E4)',
      },
    },
    { name: 'Costes', cells: {} },
  ],
});

const caja = (id: string, x: number, y: number, w: number, h: number, html: string) => ({
  id,
  x,
  y,
  w,
  h,
  html,
});

const PRESENTACION = JSON.stringify({
  slides: [
    {
      id: 's1',
      boxes: [
        caja('b1', 10, 30, 80, 18, '<h1>Plan del cuarto trimestre</h1>'),
        caja('b2', 10, 52, 80, 10, '<p>Comité de dirección</p>'),
      ],
    },
    {
      id: 's2',
      boxes: [
        caja('b3', 8, 8, 84, 14, '<h2>Prioridades</h2>'),
        caja(
          'b4',
          8,
          26,
          84,
          60,
          '<ul><li>Cerrar la migración</li><li>Contratar soporte</li></ul>',
        ),
      ],
    },
    { id: 's3', boxes: [caja('b5', 8, 8, 84, 14, '<h2>Calendario</h2>')] },
  ],
});

const COMENTARIOS: CommentItem[] = [
  {
    id: 'c1',
    document_id: 'demo',
    author_sub: 'u-marta',
    author_name: 'Marta Ruiz',
    anchor: { text: 'subida de tarifas' },
    body: '¿Tenemos ya la cifra exacta de la subida?',
    resolved: false,
    created_at: '2026-10-02T09:12:00Z',
    updated_at: '2026-10-02T09:12:00Z',
  },
  {
    id: 'c2',
    document_id: 'demo',
    author_sub: 'u-ivan',
    author_name: 'Iván Soto',
    anchor: null,
    body: 'Revisado, pasa a comité.',
    resolved: true,
    created_at: '2026-10-01T16:40:00Z',
    updated_at: '2026-10-01T16:40:00Z',
  },
];

const CONTENIDO: Record<DocKind, string> = {
  document: DOCUMENTO,
  spreadsheet: HOJA,
  presentation: PRESENTACION,
};

/** Un modelo de fuera en vez del de ejemplo: lo pone `test/e2e/importados.mjs` antes de cargar
 *  la página, para pintar lo que devuelve el importador. */
const CONTENIDO_DE_FUERA = (globalThis as { __SYTUE_DEMO_CONTENIDO__?: string })
  .__SYTUE_DEMO_CONTENIDO__;

export function EditorDemo(props: {
  kind: DocKind;
  locale: Locale;
  panel: PanelId | null;
  settings: boolean;
  soloLectura: boolean;
}): JSX.Element {
  const t = docsT(props.locale);
  const [ui, setUiState] = useState<EditorUi>({
    panel: props.panel,
    settingsOpen: props.settings,
    ruler: true,
    zoom: 100,
    formFill: false,
    track: false,
  });
  const [ultimoVisto] = useState(() => Date.now());
  const [fmt, setFmt] = useState<FormatState>(EMPTY_FORMAT);
  const [pageSetup, setPageSetup] = useState({ margin: 96, landscape: false });
  const [stats, setStats] = useState({ words: 64, pages: 1, page: 1 });
  const setUi = useCallback((patch: Partial<EditorUi>): void => {
    setUiState((prev) => ({ ...prev, ...patch }));
  }, []);
  useEffect(() => {
    const onChange = (): void => setFmt(getFormatState());
    document.addEventListener('selectionchange', onChange);
    return () => document.removeEventListener('selectionchange', onChange);
  }, []);

  const ctx: EditorContext = {
    doc: { id: 'demo', kind: props.kind, title: 'Informe trimestral' } as unknown as DocumentMeta,
    kind: props.kind,
    locale: props.locale,
    t,
    aiEnabled: true,
    applyTheme: nadaAsync,
    cursorDots: { aPuntos: () => null, aOffsets: () => null },
    esPropietario: true,
    title: 'Informe trimestral',
    setTitle: nada,
    save: 'saved',
    connection: 'online',
    content: CONTENIDO_DE_FUERA ?? CONTENIDO[props.kind],
    contentEpoch: 1,
    onContentInput: nada,
    fmt,
    exec: (command, value) => {
      if (props.soloLectura) return;
      exec(command, value);
      setFmt(getFormatState());
    },
    ui,
    setUi,
    pageSetup,
    setPageSetup: (patch) => setPageSetup((prev) => ({ ...prev, ...patch })),
    stats,
    reportStats: (patch) =>
      setStats((prev) => {
        const next = { ...prev, ...patch };
        return next.pages === prev.pages && next.page === prev.page ? prev : next;
      }),
    peers: [
      { sub: 'mr-marta', lastSeen: ultimoVisto },
      { sub: 'is-ivan', lastSeen: ultimoVisto },
    ],
    peerCursors: [],
    sendCursor: nada,
    comments: COMENTARIOS,
    addComment: nadaAsync,
    resolveComment: nadaAsync,
    removeComment: nadaAsync,
    versions: [],
    loadVersions: nadaAsync,
    createVersion: nadaAsync,
    restoreVersion: nadaAsync,
    exportAs: nadaAsync,
    sourceRef: '',
    soloLectura: props.soloLectura,
    savingToDrive: false,
    saveToDrive: nadaAsync,
  };
  const canvas =
    props.kind === 'spreadsheet' ? (
      <SheetCanvas ctx={ctx} />
    ) : props.kind === 'presentation' ? (
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
              settings={<SettingsPanel ctx={ctx} />}
              status={<StatusBar ctx={ctx} />}
              panelOpen={ui.panel !== null || ui.settingsOpen}
            />
          </div>
        </EditorCtx.Provider>
      </EditorI18nCtx.Provider>
      <DialogosDelEditor />
    </ToastProvider>
  );
}
