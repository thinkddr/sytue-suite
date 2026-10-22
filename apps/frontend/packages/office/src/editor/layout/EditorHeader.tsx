/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Cabecera del editor de 48 px (BLUEPRINT.md §6.1): volver a Documentos,
 * marca mini, título editable inline, chip de guardado (aria-live), avatares
 * de colaboradores, «Abrir en la aplicación» (deep link a la app instalada, al
 * estilo Office Online), Compartir (popover con copiar enlace) y Descargar (menú
 * de exportación PDF/HTML/Markdown/ODF/OOXML — los formatos que el backend sabe
 * serializar).
 */
import type { JSX } from 'preact';
import { appUrl, driveFolderUrl, OFFICE_EXPORTS, officeExportAvailable } from '@sytue/config';
import { useEffect, useRef, useState } from 'preact/hooks';
import type { DocKind } from '@sytue/api-clients';
import { Button, Dropdown, MenuItem, ProductIcon, useToast } from '@sytue/ui';
import { openInApp, inNativeShell } from '@sytue/shell';
import { ShareDialog } from '../dialogs/ShareDialog';
import { EditorIcon } from '../icons';
import { cambioDePresencia, peerColor, peerName, quienCambia } from '../state/cursors';
import { SIN_LLAMADA, enLlamada, nombreDe, parseLlamada } from '../state/llamada';
import type { DocsKey } from '../../i18n';
import { EXPORT_FORMATS } from '../types';
import type { EditorContext, ExportFormat, SaveState } from '../types';

const ICON_BTN =
  'inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-sm text-text-muted ' +
  'hover:bg-bg-subtle hover:text-text ' +
  'sytue-focus-ring';

const SAVE_CHIP: Record<SaveState, { dot: string; labelKey: DocsKey }> = {
  saved: { dot: 'bg-success', labelKey: 'header.save.saved' },
  saving: { dot: 'bg-warning', labelKey: 'header.save.saving' },
  dirty: { dot: 'bg-warning', labelKey: 'header.save.dirty' },
  error: { dot: 'bg-danger', labelKey: 'header.save.error' },
};

/** Tesela de producto por tipo de documento: la misma que ve el usuario en el conmutador. */
const TESELA_POR_KIND: Record<DocKind, 'docs' | 'sheets' | 'slides'> = {
  document: 'docs',
  spreadsheet: 'sheets',
  presentation: 'slides',
};

/** Extensiones por kind para las entradas ODF/OOXML del menú Descargar. */
const EXPORT_EXT: Record<DocKind, { odf: string; ooxml: string }> = {
  document: { odf: '.odt', ooxml: '.docx' },
  spreadsheet: { odf: '.ods', ooxml: '.xlsx' },
  presentation: { odf: '.odp', ooxml: '.pptx' },
};

/** «Guardado» / «Guardado hace Xs»; granularidad del tic: 5 s. La frase entera
 *  vive en el catálogo (con su hueco): en alemán el complemento va delante. */
function savedLabel(t: EditorContext['t'], savedAt: number | null, now: number): string {
  if (savedAt === null) return t('header.save.saved');
  const s = Math.round((now - savedAt) / 1000);
  if (s < 5) return t('header.save.saved');
  if (s < 60) return t('header.save.agoSeconds', { seconds: s });
  return t('header.save.agoMinutes', { minutes: Math.round(s / 60) });
}

export function EditorHeader({ ctx }: { ctx: EditorContext }): JSX.Element {
  const t = ctx.t;
  const toast = useToast();
  // Formato en curso de exportación (null = ninguno) para el estado «Generando…».
  const [exporting, setExporting] = useState<ExportFormat | null>(null);

  // Salta a la app instalada con ESTE documento (la URL actual ya lleva su id). Si el
  // sistema no se lleva el foco se OFRECE la descarga: ningún navegador dice si una app
  // está instalada —sería huella digital—, así que la detección es un indicio, no un dato.
  // El producto va explícito: hojas y presentaciones comparten host con Docs, y por la
  // URL sola una hoja se abriría en Docus y no en Calcus.
  async function abrirEnApp(): Promise<void> {
    if (await openInApp(window.location.href, TESELA_POR_KIND[ctx.kind])) return;
    toast.notify({
      tone: 'info',
      title: t('header.openInApp'),
      description: t('header.openInApp.notOpened'),
      action: {
        label: t('header.openInApp.downloads'),
        onClick: () => window.open('https://sytue.eu/descargas', '_blank', 'noopener'),
      },
    });
  }

  async function download(format: ExportFormat): Promise<void> {
    setExporting(format);
    try {
      await ctx.exportAs(format); // nunca lanza: resuelve el error con toast internamente
    } finally {
      setExporting(null);
    }
  }

  // Marca local del último tránsito a 'saved' para el texto relativo del chip.
  const [savedAt, setSavedAt] = useState<number | null>(null);
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (ctx.save === 'saved') {
      // OJO: NO llamar `t` a esta marca de tiempo — ensombrecería la `t` de
      // traducción del componente y el fallo no se vería hasta producción.
      const at = Date.now();
      setSavedAt(at);
      setNow(at);
    }
  }, [ctx.save]);
  useEffect(() => {
    if (ctx.save !== 'saved') return;
    // Ídem: `timer`, no `t` (la trampa clásica de esta base de código).
    const timer = window.setInterval(() => setNow(Date.now()), 5000);
    return () => window.clearInterval(timer);
  }, [ctx.save]);

  /* Presencia para lectores de pantalla: la fila de avatares es visual. Una región
     `aria-live` anuncia cuando alguien entra o sale, con su nombre si se conoce (el que
     trae su cursor en la presencia; la lista de la cabecera solo trae el sub), y si no con
     la frase genérica («Otra persona se ha conectado», o el recuento nuevo). Los dos
     primeros segundos no se anuncian: es la lista de quien ya estaba, no una novedad. */
  const subsKey = ctx.peers.map((p) => p.sub).join('|');
  const subsPreviosRef = useRef<string[] | null>(null);
  /** Último nombre conocido de cada sub: al salir, su cursor ya no está. */
  const nombresRef = useRef<Map<string, string>>(new Map());
  /** Copia para pintar: una ref no se lee durante el render, y cambiarla no repinta. */
  const [nombres, setNombres] = useState<Map<string, string>>(new Map());
  useEffect(() => {
    let cambia = false;
    for (const pc of ctx.peerCursors) {
      if (!pc.name.trim()) continue;
      const nombre = peerName(pc.name);
      if (nombresRef.current.get(pc.id) === nombre) continue;
      nombresRef.current.set(pc.id, nombre);
      cambia = true;
    }
    if (cambia) setNombres(new Map(nombresRef.current));
  }, [ctx.peerCursors]);
  const [abiertoEn] = useState(() => Date.now());
  const [anuncio, setAnuncio] = useState({ texto: '', n: 0 });
  useEffect(() => {
    const ahora = subsKey === '' ? [] : subsKey.split('|');
    const antes = subsPreviosRef.current;
    subsPreviosRef.current = ahora;
    if (antes === null || Date.now() - abiertoEn < 2000) return;
    const cambio = cambioDePresencia(antes, ahora);
    if (cambio === null) return;
    const { entran, salen } = quienCambia(antes, ahora);
    const nombres = (cambio === 'entra' ? entran : salen).map((s) => nombresRef.current.get(s));
    const conocidos = nombres.every((n) => n !== undefined);
    const texto = conocidos
      ? t(cambio === 'entra' ? 'header.peerJoined' : 'header.peerLeft', {
          name: nombres.join(', '),
        })
      : cambio === 'entra'
        ? t('header.peer')
        : t('header.peers', { count: ahora.length });
    setAnuncio((a) => ({ texto, n: a.n + 1 }));
  }, [subsKey, t, abiertoEn]);

  /* «Llamar desde el documento»: la sala de Rooms ligada a este documento y quién está en
     ella. Se sondea cada 15 s con la pestaña visible; si Rooms no está desplegado la
     respuesta es `available: false` y no se pinta nada. */
  const [llamada, setLlamada] = useState(SIN_LLAMADA);
  const docId = ctx.doc.id;
  const offline = ctx.connection === 'offline';
  useEffect(() => {
    if (offline) return;
    let vivo = true;
    const sondear = (): void => {
      if (document.visibilityState === 'hidden') return;
      fetch(`/api/llamada?id=${encodeURIComponent(docId)}`)
        .then((r) => (r.ok ? r.json() : null))
        .then((b: unknown) => {
          if (vivo) setLlamada(parseLlamada(b));
        })
        .catch(() => {
          if (vivo) setLlamada(SIN_LLAMADA);
        });
    };
    sondear();
    const timer = window.setInterval(sondear, 15000);
    return () => {
      vivo = false;
      window.clearInterval(timer);
    };
  }, [docId, offline]);
  const llamar = (): void => {
    // La ventana se abre desde el clic (un bloqueador la corta si no) y se rellena después.
    const v = window.open('about:blank', 'sytue-llamada-doc', 'popup=yes,width=420,height=340');
    fetch(`/api/llamada?id=${encodeURIComponent(docId)}`, { method: 'POST' })
      .then((r) => (r.ok ? (r.json() as Promise<{ url?: string }>) : Promise.reject(new Error())))
      .then((b) => {
        if (!b.url || !v) throw new Error();
        v.location.href = b.url;
      })
      .catch(() => {
        v?.close();
        toast.notify({ tone: 'danger', title: t('call.error') });
      });
  };

  const chip = SAVE_CHIP[ctx.save];
  const chipLabel =
    ctx.connection === 'offline'
      ? t('header.save.offline')
      : ctx.save === 'saved'
        ? savedLabel(t, savedAt, now)
        : t(chip.labelKey);
  const chipDot = ctx.connection === 'offline' ? 'bg-warning' : chip.dot;
  const ext = EXPORT_EXT[ctx.kind];
  // Etiqueta de cada formato; el ORDEN y el conjunto los fija EXPORT_FORMATS, que es
  // también lo que acepta el BFF. Antes eran dos listas y PDF estaba en esta y no en la
  // otra: el menú lo ofrecía y la descarga contestaba `bad_format`.
  const EXPORT_LABEL: Record<ExportFormat, string> = {
    pdf: 'PDF',
    html: 'HTML',
    markdown: 'Markdown',
    odf: `ODF (${ext.odf})`,
    ooxml: `OOXML (${ext.ooxml})`,
  };
  const exportItems = EXPORT_FORMATS.map((format) => ({ format, label: EXPORT_LABEL[format] }));

  const peers = ctx.peers.slice(0, 4);

  return (
    <header class="sytue-sheet-corners border-border bg-surface flex h-[var(--editor-header-h)] shrink-0 items-center gap-2 border-b px-3">
      {/* SALE AL HUB, no a la raíz de esta app. `/docs/` es la lista GENÉRICA sin
          producto: quien cerraba Calcus aterrizaba en un cajón con documentos,
          hojas, presentaciones y PDF mezclados, que es justo la portada conjunta
          que se decidió no tener —«no hay entrada genérica Docs: la portada
          conjunta vive en el Hub», dice el catálogo de apps—.

          Con `appUrl('one')` y no con una ruta relativa: es EXACTAMENTE a donde
          lleva la tesela «Sytue Hub» del conmutador, así que las dos salidas de
          la aplicación acaban en el mismo sitio. Una ruta `/one` funcionaría en
          el host de una organización y rompería en el canónico, donde el Hub
          vive en otro subdominio. */}
      <a
        href={ctx.volverA ? driveFolderUrl(ctx.volverA) : appUrl('one')}
        aria-label={ctx.volverA ? t('pdf.backToDrive') : t('header.back')}
        title={ctx.volverA ? t('pdf.backToDrive') : t('header.back')}
        class={ICON_BTN}
      >
        <EditorIcon name="chevronLeft" />
      </a>

      {/* La tesela del producto, la misma del conmutador y del portal. Aquí había
          un isotipo antiguo (cuadrado grafito con esquina de latón) que a 24 px
          se leía como un cuadrado negro sin significado. */}
      <span class="inline-flex shrink-0 max-md:hidden" aria-hidden="true">
        <ProductIcon appKey={TESELA_POR_KIND[ctx.kind]} size={24} />
      </span>

      <div class="bg-border h-6 w-px shrink-0 max-md:hidden" aria-hidden="true" />

      {/* `max-md:max-w-[40vw]`: el `size` del campo (hasta 44 caracteres) lo hacía
          más ancho que lo que le deja la cabecera de un móvil y el documento
          desbordaba 14-34px (barrido móvil del 2026-09-08, Pixel 5/7 y 14 Pro Max). */}
      <input
        id="editor-title-input"
        aria-label={t('header.title.label')}
        class="text-text hover:bg-bg-subtle focus:bg-bg-subtle sytue-focus-ring field-sizing-content h-7 max-w-[360px] min-w-24 shrink rounded-sm bg-transparent px-1.5 text-sm font-semibold transition-colors duration-150 max-md:max-w-[40vw] max-md:flex-1"
        value={ctx.title}
        size={Math.min(Math.max(ctx.title.length + 2, 10), 44)}
        onInput={(e) => ctx.setTitle(e.currentTarget.value)}
        onKeyDown={(e) => {
          // Enter confirma (setTitle ya guardó de forma optimista; el blur cierra la edición).
          if (e.key === 'Enter') e.currentTarget.blur();
        }}
      />

      {/* En móvil cede ancho antes que el título: con «Guardado hace 10 s»
          entero, al nombre del documento le quedaban 72px y se leía «Prue». El
          anuncio a lectores de pantalla (aria-live) va con el texto completo. */}
      <span
        class="text-text-muted inline-flex shrink-0 items-center gap-1.5 text-xs max-md:min-w-0 max-md:shrink max-md:truncate"
        aria-live="polite"
      >
        <span class={`h-1.5 w-1.5 rounded-full ${chipDot}`} aria-hidden="true" />
        {chipLabel}
      </span>

      <div class="flex-1 max-md:hidden" />

      {/* El espacio de no separación alterna para que un mismo texto repetido se vuelva a leer. */}
      <span class="sr-only" role="status" aria-live="polite">
        {anuncio.texto}
        {anuncio.n % 2 === 1 ? '\u00a0' : ''}
      </span>

      {peers.length > 0 && (
        <div
          class="flex items-center"
          role="group"
          aria-label={t('header.peers', { count: ctx.peers.length })}
        >
          {peers.map((p) => {
            const nombre = nombreDe(llamada, p.sub, nombres);
            const dentro = enLlamada(llamada, p.sub);
            const etiqueta = nombre ?? t('header.peer');
            return (
              <span
                key={p.sub}
                role="img"
                aria-label={dentro ? t('call.peer', { name: etiqueta }) : etiqueta}
                title={dentro ? t('call.peer', { name: etiqueta }) : etiqueta}
                class={`border-surface text-2xs -ml-1.5 inline-flex h-7 w-7 items-center justify-center rounded-full border-2 font-semibold text-white first:ml-0 ${
                  dentro ? 'ring-success ring-2' : ''
                }`}
                style={{ background: peerColor(p.sub) }}
              >
                {(nombre ?? p.sub).slice(0, 2).toUpperCase()}
              </span>
            );
          })}
        </div>
      )}

      {llamada.available && (
        <Button variant="ghost" size="sm" class="max-md:hidden" disabled={offline} onClick={llamar}>
          {llamada.active ? `${t('call.join')} (${llamada.participants.length})` : t('call.start')}
        </Button>
      )}

      {/* «Abrir en la aplicación»: el mismo documento en la app instalada, como el
          «Open in Desktop App» de Office Online. Oculto dentro de la app nativa —ya
          estás en ella— y con la descarga como salida si no se abre nada. */}
      {!inNativeShell() ? (
        <button
          type="button"
          class={`${ICON_BTN} max-md:hidden`}
          title={t('header.openInApp')}
          aria-label={t('header.openInApp')}
          onClick={() => void abrirEnApp()}
        >
          <EditorIcon name="openInApp" />
        </button>
      ) : null}

      <ShareDialog
        ctx={ctx}
        align="end"
        trigger={(trigger) => (
          <Button {...trigger} variant="primary" size="sm" disabled={ctx.connection === 'offline'}>
            {t('header.share')}
          </Button>
        )}
      />

      {/* En <768px se retira: el mismo menú (PDF/HTML/Markdown) vive en «Archivo»
          de la cinta, y aquí eran 117px de los 360 de un Android corriente. */}
      <Dropdown
        label={t('header.download')}
        align="end"
        class="max-md:hidden"
        trigger={(trigger) => (
          <Button
            {...trigger}
            variant="ghost"
            size="sm"
            leadingIcon={<EditorIcon name="download" />}
            loading={exporting !== null}
            disabled={ctx.connection === 'offline'}
          >
            {t('header.download')}
          </Button>
        )}
      >
        {exportItems.map((it) => (
          <MenuItem
            key={it.format}
            size="sm"
            disabled={
              !officeExportAvailable(
                OFFICE_EXPORTS.find((command) => command.id === it.format)!,
                ctx.connection === 'offline',
                exporting !== null,
              )
            }
            aria-busy={exporting === it.format}
            onClick={() => void download(it.format)}
          >
            {exporting === it.format ? t('header.download.generating') : it.label}
          </MenuItem>
        ))}
      </Dropdown>
    </header>
  );
}
