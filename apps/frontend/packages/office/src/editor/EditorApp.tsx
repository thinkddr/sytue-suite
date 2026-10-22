/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EditorApp — isla raíz del editor Sytue Docs (BLUEPRINT.md §2/§4.4, dueño A1).
 * Compone la capa de datos (useDocument), el estado local de interfaz (ui,
 * pageSetup, fmt, stats) y las zonas: cabecera, ribbon, franja izquierda,
 * lienzo según kind (folio/hoja/diapositivas), panel de ajustes y barra de
 * estado. Construye el EditorContext (contrato §2) y lo pasa por props a cada
 * zona (y por EditorCtx.Provider para piezas profundas).
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useState } from 'preact/hooks';
import { Button, DialogosDelEditor, EmptyState, Icon, ToastProvider } from '@sytue/ui';
import { fetchSession } from '@sytue/auth/client';
import { FranjaPeticion } from '@sytue/shell';
import { driveFolderUrl } from '@sytue/config';
import type { Locale } from '@sytue/i18n';
import { docsT } from '../i18n';
import { r } from '../lib/rutas';
import './editor.css';
import { EMPTY_FORMAT } from './types';
import type { EditorContext, EditorStats, EditorUi, FormatState, PageSetup } from './types';
import { EditorCtx, EditorI18nCtx } from './context';
import { useDocument } from './state/useDocument';
import { exec, getFormatState, setTrackAuthor } from './canvas/exec';
import { EditorLayout } from './layout/EditorLayout';
import { EditorHeader } from './layout/EditorHeader';
import { StatusBar } from './layout/StatusBar';
import { LeftRail } from './layout/LeftRail';
import { SettingsPanel } from './panels/SettingsPanel';
import { Ribbon } from './ribbon/Ribbon';
import { PageCanvas } from './canvas/PageCanvas';
import { SheetCanvas } from './sheets/SheetCanvas';
import { FranjaHojaProtegida } from './sheets/FranjaHojaProtegida';
import { SlidesCanvas } from './slides/SlidesCanvas';

/** Throttle del reflejo de formato bajo el cursor (§4.4: 100 ms). */
const FMT_THROTTLE_MS = 100;

export interface EditorAppProps {
  documentId: string;
  /** Ámbito local: evita mezclar la caché offline entre cuentas del dispositivo. */
  userId: string;
  /** Idioma negociado en el servidor (middleware). La isla NO lo detecta por su
   *  cuenta: así el editor y el `<html lang>` no pueden discrepar (§13.1). */
  locale: Locale;
  /** ¿Hay servicio de IA detrás del BFF (SYTUE_AI_URL)? Lo decide el SERVIDOR: sin
   *  él, `/api/ai` responde 503 y la puerta no se pinta. Un botón que siempre
   *  falla es peor que no tenerlo. */
  aiEnabled: boolean;
  /** Carpeta de Drive de la que vino el documento para volver a ella al salir. */
  volverA?: string | undefined;
}

export function EditorApp({
  documentId,
  userId,
  locale,
  aiEnabled,
  volverA,
}: EditorAppProps): JSX.Element {
  // useDocument usa useToast: el proveedor debe envolver al hook (§4.4).
  return (
    <ToastProvider>
      <EditorShell
        documentId={documentId}
        userId={userId}
        locale={locale}
        aiEnabled={aiEnabled}
        volverA={volverA}
      />
      {/* Una sola vez, aquí: deja los diálogos disponibles para el lienzo de hojas
          y diapositivas y para el ribbon, que hacen su trabajo en funciones
          imperativas y no tienen árbol donde montar uno. */}
      <DialogosDelEditor />
    </ToastProvider>
  );
}

/** Igualdad campo a campo de FormatState (13 escalares): evita re-render por tick. */
function sameFmt(a: FormatState, b: FormatState): boolean {
  return (
    a.bold === b.bold &&
    a.italic === b.italic &&
    a.underline === b.underline &&
    a.strike === b.strike &&
    a.font === b.font &&
    a.size === b.size &&
    a.align === b.align &&
    a.bullet === b.bullet &&
    a.number === b.number &&
    a.heading === b.heading &&
    a.inTable === b.inTable &&
    a.imageSelected === b.imageSelected &&
    a.canUndo === b.canUndo &&
    a.canRedo === b.canRedo
  );
}

function EditorShell({
  documentId,
  userId,
  locale,
  aiEnabled,
  volverA,
}: EditorAppProps): JSX.Element {
  const data = useDocument(documentId, userId);
  const t = docsT(locale);

  const [ui, setUiState] = useState<EditorUi>({
    panel: null,
    settingsOpen: false,
    ruler: true,
    zoom: 100,
    formFill: false,
    track: false,
  });
  const [pageSetup, setPageSetupState] = useState<PageSetup>({ margin: 96, landscape: false });
  const [fmt, setFmt] = useState<FormatState>(EMPTY_FORMAT);
  const [canvasStats, setCanvasStats] = useState({ pages: 1, page: 1 });

  const setUi = useCallback((patch: Partial<EditorUi>): void => {
    setUiState((prev) => ({ ...prev, ...patch }));
  }, []);

  const setPageSetup = useCallback((patch: Partial<PageSetup>): void => {
    setPageSetupState((prev) => ({ ...prev, ...patch }));
  }, []);

  /** Los lienzos informan pages/page (words lo pone useDocument, §2). */
  const reportStats = useCallback((patch: Partial<EditorStats>): void => {
    setCanvasStats((prev) => {
      const next = { pages: patch.pages ?? prev.pages, page: patch.page ?? prev.page };
      return next.pages === prev.pages && next.page === prev.page ? prev : next;
    });
  }, []);

  const refreshFmt = useCallback((): void => {
    const next = getFormatState();
    setFmt((prev) => (sameFmt(prev, next) ? prev : next));
  }, []);

  /** exec de canvas/exec.ts + reflejo inmediato en el ribbon (un toggle con el
   *  caret colapsado no dispara selectionchange). */
  // LA BARRERA, y va aquí porque `exec` es el único punto por el que pasan los
  // comandos de las cintas, los atajos y los menús contextuales. Apagar solo el
  // `contentEditable` del lienzo dejaba escribir por esas tres vías.
  const execCommand = useCallback<EditorContext['exec']>(
    (command, value) => {
      if (data.soloLectura) return;
      exec(command, value);
      refreshFmt();
    },
    [refreshFmt, data.soloLectura],
  );

  /* Reflejo de formato: selectionchange (throttle 100 ms) + input del lienzo
   * (canUndo/canRedo cambian al teclear sin mover la selección). */
  useEffect(() => {
    let timer: number | null = null;
    const onChange = (): void => {
      if (timer !== null) return;
      timer = window.setTimeout(() => {
        timer = null;
        refreshFmt();
      }, FMT_THROTTLE_MS);
    };
    document.addEventListener('selectionchange', onChange);
    document.addEventListener('input', onChange, true);
    return () => {
      document.removeEventListener('selectionchange', onChange);
      document.removeEventListener('input', onChange, true);
      if (timer !== null) window.clearTimeout(timer);
    };
  }, [refreshFmt]);

  /* Atajos de suite ofimática que el navegador se quedaba (§6.8 solo cubría los
   * del folio). Ctrl/Cmd+S abría el «Guardar página como…» del navegador —que
   * descarga el HTML de la aplicación— en un editor que ya guarda solo: aquí
   * adelanta el autosave y el chip lo cuenta. Ctrl/Cmd+F abría el buscador del
   * navegador, que no ve más que el folio visible y no sabe reemplazar, teniendo
   * el panel Buscar del editor a un lado. Van en window y no en el folio para que
   * respondan también con el foco en la cinta o en un panel.
   *
   * Se dejan pasar los del navegador que no compiten (Ctrl+P imprime, y aquí no
   * hay nada mejor todavía) y los del folio (B/I/U, deshacer nativo). */
  const { saveNow } = data;
  useEffect(() => {
    const onKey = (event: KeyboardEvent): void => {
      if (!(event.ctrlKey || event.metaKey) || event.altKey) return;
      const key = event.key.toLowerCase();
      if (key === 's') {
        event.preventDefault();
        saveNow();
      } else if (key === 'f' && !event.shiftKey) {
        event.preventDefault();
        setUi({ panel: 'buscar' });
        // Con el panel recién abierto lo enfoca su propio autofocus; con el panel
        // YA abierto (y el cursor de vuelta en el folio) hay que traer el foco a
        // mano, o el segundo Ctrl+F no haría nada visible. El tiempo 0 espera al
        // render, mismo truco que el panel Esquema.
        window.setTimeout(() => {
          const field = document.getElementById('editor-find-input');
          if (field instanceof HTMLInputElement) {
            field.focus();
            field.select(); // teclear sustituye la búsqueda anterior
          }
        }, 0);
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [saveNow, setUi]);

  /* Autoría de las marcas de control de cambios: nombre + id (sub) de la sesión.
   * Best-effort (si falla, exec usa una reserva); no bloquea el arranque. */
  useEffect(() => {
    let alive = true;
    void fetchSession().then((user) => {
      if (alive && user) setTrackAuthor({ name: user.name, id: user.id });
    });
    return () => {
      alive = false;
    };
  }, []);

  if (data.status === 'loading' || (data.status === 'ready' && data.doc === null)) {
    // Esqueleto del cromo (cabecera · pestañas · ribbon · rail · folio · estado):
    // se percibe más rápido que un spinner y no salta al montar la UI real (§4.4).
    // La clase sytue-editor aporta los tokens de métrica y el lienzo oscuro en dark.
    const block = 'rounded-sm bg-bg-subtle';
    return (
      <div
        class="sytue-editor bg-surface text-text flex h-full min-h-0 w-full flex-col"
        role="status"
        aria-busy="true"
      >
        <span class="sr-only">{t('editor.opening')}</span>
        <div class="flex h-full min-h-0 animate-pulse flex-col" aria-hidden="true">
          <div class="border-border flex h-[var(--editor-header-h)] shrink-0 items-center gap-2 border-b px-3">
            <div class={`${block} h-6 w-6`} />
            <div class={`${block} h-4 w-40`} />
            <div class="ml-auto flex gap-2">
              <div class={`${block} h-6 w-16`} />
              <div class={`${block} h-6 w-6`} />
            </div>
          </div>
          <div class="border-border flex h-[var(--editor-tabs-h)] shrink-0 items-center gap-3 border-b px-3">
            {[48, 64, 56, 72, 60].map((w, i) => (
              <div key={i} class={`${block} h-3`} style={{ width: `${w}px` }} />
            ))}
          </div>
          <div class="border-border flex h-[var(--editor-groups-h)] shrink-0 items-center gap-2 border-b px-3">
            {[120, 96, 140, 88].map((w, i) => (
              <div key={i} class={`${block} h-10`} style={{ width: `${w}px` }} />
            ))}
          </div>
          <div class="flex min-h-0 flex-1">
            <div class="border-border flex w-[var(--editor-rail-w)] shrink-0 flex-col items-center gap-1 border-r py-2">
              {[0, 1, 2].map((i) => (
                <div key={i} class={`${block} h-8 w-8`} />
              ))}
            </div>
            <div
              class="min-h-0 flex-1 overflow-hidden"
              style={{ background: 'var(--editor-canvas-bg)' }}
            >
              <div
                class="mx-auto mt-8 w-[min(70%,720px)] rounded-sm p-12"
                style={{
                  background: 'var(--editor-page-bg)',
                  boxShadow: 'var(--editor-page-shadow)',
                }}
              >
                <div class={`${block} mb-6 h-6 w-1/2`} />
                {Array.from({ length: 8 }, (_, i) => (
                  <div
                    key={i}
                    class={`${block} mb-3 h-3`}
                    style={{ width: `${90 - (i % 3) * 15}%` }}
                  />
                ))}
              </div>
            </div>
          </div>
          <div class="border-border h-[var(--editor-status-h)] shrink-0 border-t" />
        </div>
      </div>
    );
  }

  if (data.status === 'error' || data.doc === null) {
    return (
      <div class="bg-surface flex h-full w-full items-center justify-center">
        <EmptyState
          title={t('editor.error.title')}
          description={t('editor.error.description')}
          illustration={<Icon name="warning" size={40} class="text-danger" />}
          action={
            <div class="flex flex-wrap items-center justify-center gap-2">
              <Button size="sm" onClick={() => window.location.reload()}>
                {t('editor.error.retry')}
              </Button>
              <Button
                variant="secondary"
                size="sm"
                onClick={() => (window.location.href = volverA ? driveFolderUrl(volverA) : r('/'))}
              >
                {volverA ? t('pdf.backToDrive') : t('editor.error.back')}
              </Button>
            </div>
          }
        />
      </div>
    );
  }

  const ctx: EditorContext = {
    doc: data.doc,
    kind: data.doc.kind,
    locale,
    t,
    aiEnabled,
    title: data.title,
    setTitle: data.setTitle,
    save: data.save,
    connection: data.connection,
    content: data.content,
    contentEpoch: data.contentEpoch,
    onContentInput: data.onContentInput,
    fmt,
    exec: execCommand,
    ui,
    setUi,
    pageSetup,
    setPageSetup,
    stats: { words: data.words, pages: canvasStats.pages, page: canvasStats.page },
    reportStats,
    peers: data.peers,
    peerCursors: data.peerCursors,
    sendCursor: data.sendCursor,
    comments: data.comments,
    addComment: data.addComment,
    resolveComment: data.resolveComment,
    removeComment: data.removeComment,
    versions: data.versions,
    loadVersions: data.loadVersions,
    createVersion: data.createVersion,
    restoreVersion: data.restoreVersion,
    applyTheme: data.applyTheme,
    previewTheme: data.previewTheme,
    exportAs: data.exportAs,
    sourceRef: data.sourceRef,
    cursorDots: data.cursorDots,
    soloLectura: data.soloLectura,
    esPropietario: data.role === 'owner',
    savingToDrive: data.savingToDrive,
    saveToDrive: data.saveToDrive,
    volverA,
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
    // El proveedor de idioma envuelve al del editor: las piezas profundas de las
    // cintas leen la `t` del contexto y sin esto caerían al inglés por defecto
    // aunque el servidor hubiera negociado otra lengua.
    <EditorI18nCtx.Provider value={t}>
      <EditorCtx.Provider value={ctx}>
        <EditorLayout
          aviso={
            data.status === 'ready' && data.doc ? (
              <>
                <FranjaPeticion
                  endpoint={`/api/edit-requests?id=${encodeURIComponent(data.doc.id)}`}
                  soloLectura={data.soloLectura}
                  puedeConceder={data.role === 'owner'}
                  pedida={data.edicionPedida}
                  mensaje={t('editor.readOnly')}
                  locale={locale}
                />
                {/* Con permiso, una hoja protegida es un bloqueo propio: «Editar». */}
                {!data.soloLectura && ctx.kind === 'spreadsheet' ? (
                  <FranjaHojaProtegida locale={locale} />
                ) : null}
              </>
            ) : undefined
          }
          header={<EditorHeader ctx={ctx} />}
          ribbon={<Ribbon ctx={ctx} />}
          rail={<LeftRail ctx={ctx} />}
          main={canvas}
          settings={<SettingsPanel ctx={ctx} />}
          status={<StatusBar ctx={ctx} />}
          panelOpen={ui.panel !== null || ui.settingsOpen}
        />
      </EditorCtx.Provider>
    </EditorI18nCtx.Provider>
  );
}
