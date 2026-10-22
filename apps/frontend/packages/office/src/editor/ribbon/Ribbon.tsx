/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Ribbon del editor (BLUEPRINT.md §6.2): tira de pestañas de 32 px + fila de
 * grupos de 68 px. La primera posición es «Archivo» (NO es pestaña: abre un
 * menú); después, las pestañas reales filtradas por tipo de documento, con
 * roles tablist/tab/tabpanel, foco roving (←/→/Inicio/Fin) y subrayado de
 * 2 px en latón de marca (Graphite & Brass) en la activa.
 *
 * La fila de grupos es UNA línea que se desplaza en horizontal (la banda). Si no cabe,
 * aparece «Más» al final: despliega la banda en varias líneas sobre el lienzo, con todos
 * los grupos a la vista, y se cierra con Escape, con un clic fuera o al pulsar una acción.
 * La banda es una barra de herramientas (`Toolbar` de @sytue/ui: una parada de
 * tabulación, flechas, Inicio/Fin), F6 salta entre la cinta y el lienzo, y sus popovers
 * son `fixed` (popovers.ts) para que el desplazamiento no los recorte.
 *
 * Nota de integración: los cuerpos de pestaña (B8–B10) se importan aquí por
 * composición, según el reparto del blueprint §10; este fichero es su único
 * punto de montaje. El menú Archivo es propio (no el Dropdown de @sytue/ui)
 * para poder cerrarse al elegir una opción y mantener los ítems compactos.
 */
import { Fragment } from 'preact';
import type { JSX } from 'preact';
import { appUrl, OFFICE_EXPORTS, officeExportAvailable } from '@sytue/config';
import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import type { DocKind } from '@sytue/api-clients';
import {
  MenuItem,
  MenuSeparator,
  Toolbar,
  cx,
  useFitInViewport,
  useMobileBack,
  usePrompt,
} from '@sytue/ui';
import { EditorIcon } from '../icons';
import { MASCARA_SCROLL_X, useScrollRestante } from '@sytue/ui';
import { useDesborda, useSaltoF6 } from './banda';
import { usePopoversFijos } from './popovers';
import { BandaDesplegada } from './primitives';
import { PlantillaOrgItems } from './PlantillaOrgItems';
import { exportExt, fijarPdfAccesible, pdfAccesible } from '../exportName';
import type { DocsKey } from '../../i18n';
import type { EditorContext } from '../types';
import { pestanasDe } from '../pestanas';
import type { RibbonTabId } from '../pestanas';
import { InicioTab } from './tabs/Inicio';
import { InicioSheetsTab } from './tabs/InicioSheets';
import { DisenoSlidesTab, InicioSlidesTab, PresentarTab } from './tabs/InicioSlides';
import { InsertarTab } from './tabs/Insertar';
import { DatosTab, FormulasTab } from './tabs/Datos';
import { DisposicionTab } from './tabs/Disposicion';
import { ReferenciasTab } from './tabs/Referencias';
import { ColaboracionTab } from './tabs/Colaboracion';
import { VistaTab } from './tabs/Vista';

// El anillo canónico vive en `@sytue/ui/styles.css` (.sytue-focus-ring): una sola
// definición para toda la suite, en vez de la copia que cada fichero perdía a su manera.
const FOCUS_RING = 'sytue-focus-ring';

/** El cuerpo de cada pestaña, según el tipo de documento. Cada tipo con SU cuerpo cuando
 *  los comandos actúan sobre otro modelo: en «Inicio» la hoja y la presentación tienen el
 *  suyo (actúan sobre la celda o la diapositiva, no solo sobre el texto bajo el caret), y
 *  «Diseño» del folio es la disposición de página mientras que el de las diapositivas es
 *  su tamaño y su patrón. Qué pestañas hay y dónde se ven lo dice `pestanas.json`; esto
 *  es solo lo que se pinta dentro. `test/taxonomia.test.ts` exige una entrada por id. */
const CUERPOS: Readonly<Record<RibbonTabId, CuerposDe>> = {
  inicio: {
    por: { spreadsheet: InicioSheetsTab, presentation: InicioSlidesTab },
    otro: InicioTab,
  },
  insertar: { por: {}, otro: InsertarTab },
  diseno: { por: { presentation: DisenoSlidesTab }, otro: DisposicionTab },
  referencias: { por: {}, otro: ReferenciasTab },
  datos: { por: {}, otro: DatosTab },
  formulas: { por: {}, otro: FormulasTab },
  presentar: { por: {}, otro: PresentarTab },
  revisar: { por: {}, otro: ColaboracionTab },
  vista: { por: {}, otro: VistaTab },
};

/** Cuerpo de una pestaña: el de cada tipo que tiene el suyo y el de los demás. Es un dato y no
 *  una función que devuelve el componente: elegirlo con una llamada en el render lo hacía pasar
 *  por «componente creado al pintar» (react/static-components) aunque siempre sea el mismo. */
interface CuerposDe {
  readonly por: Partial<Record<DocKind, Cuerpo>>;
  readonly otro: Cuerpo;
}

type Cuerpo = (props: { ctx: EditorContext }) => JSX.Element;

export function Ribbon({ ctx }: { ctx: EditorContext }): JSX.Element {
  const [activeId, setActiveId] = useState<RibbonTabId>('inicio');
  // Solo manda en <768px: ahí la fila de grupos empieza plegada (ver abajo).
  const [toolsOpen, setToolsOpen] = useState(false);
  const tabRefs = useRef(new Map<RibbonTabId, HTMLButtonElement>());
  // El desvanecido del borde derecho avisa de que la tira sigue: a 360 px se ven
  // 248 px de los 439 que miden las seis pestañas de un documento.
  const { ref: listaRef, queda: quedaTira } = useScrollRestante<HTMLDivElement>();
  const cintaRef = useRef<HTMLDivElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);
  const bandaRef = useRef<HTMLDivElement>(null);
  const masRef = useRef<HTMLButtonElement>(null);
  // «Más»: la banda desplegada en varias líneas sobre el lienzo (ver el JSX).
  const [expandida, setExpandida] = useState(false);
  useMobileBack(expandida, () => setExpandida(false));
  useFitInViewport(bandaRef, expandida, { vertical: true });
  const [topExpandida, setTopExpandida] = useState(0);

  const visibles = pestanasDe(ctx.kind);
  // 'inicio' existe para todos los kinds, así que visibles nunca está vacío.
  const active = visibles.find((t) => t.id === activeId) ?? visibles[0]!;

  const cuerpos = CUERPOS[active.id];
  const ActiveBody = cuerpos.por[ctx.kind] ?? cuerpos.otro;

  // `expandida` pausa la medida: ver `useDesborda`.
  const desborda = useDesborda(bandaRef, active.id, expandida);
  usePopoversFijos(bandaRef);
  // F6: de la cinta al lienzo, y de cualquier otro sitio a la pestaña activa.
  const alCinta = useCallback(() => tabRefs.current.get(active.id)?.focus(), [active.id]);
  useSaltoF6(cintaRef, alCinta);

  // Otra pestaña u otro documento: la banda vuelve a plegarse.
  useEffect(() => setExpandida(false), [active.id, ctx.kind]);
  // Desplegada, se cierra con un clic fuera o al cambiar el tamaño (su posición es fija).
  useEffect(() => {
    if (!expandida) return;
    const cerrar = (): void => setExpandida(false);
    const fuera = (e: MouseEvent): void => {
      if (!panelRef.current?.contains(e.target as Node)) cerrar();
    };
    document.addEventListener('mousedown', fuera);
    window.addEventListener('resize', cerrar);
    return () => {
      document.removeEventListener('mousedown', fuera);
      window.removeEventListener('resize', cerrar);
    };
  }, [expandida]);

  const alternarMas = (): void => {
    if (!expandida) setTopExpandida(panelRef.current?.getBoundingClientRect().top ?? 0);
    setExpandida((v) => !v);
  };
  const onPanelKeyDown = (e: KeyboardEvent): void => {
    if (e.key !== 'Escape' || !expandida) return;
    // Un popover abierto atiende ÉL el Escape; la banda sigue desplegada.
    if (panelRef.current?.querySelector('.ribbon-popover')) return;
    setExpandida(false);
    masRef.current?.focus();
  };
  /** Pulsar una acción de la banda desplegada la pliega; abrir un popover no. */
  const onPanelClick = (e: MouseEvent): void => {
    if (!expandida) return;
    const boton = (e.target as HTMLElement).closest('button');
    if (!boton || boton === masRef.current) return;
    if (boton.hasAttribute('aria-haspopup') || boton.closest('.ribbon-popover')) return;
    setExpandida(false);
  };

  /** Foco roving del tablist (§0.6): las flechas MUEVEN la pestaña activa. */
  const onTablistKeyDown = (e: KeyboardEvent): void => {
    const idx = visibles.findIndex((t) => t.id === active.id);
    let next: number;
    if (e.key === 'ArrowRight') next = (idx + 1) % visibles.length;
    else if (e.key === 'ArrowLeft') next = (idx - 1 + visibles.length) % visibles.length;
    else if (e.key === 'Home') next = 0;
    else if (e.key === 'End') next = visibles.length - 1;
    else return;
    e.preventDefault();
    const id = visibles[next]!.id;
    setActiveId(id);
    tabRefs.current.get(id)?.focus();
  };

  return (
    <div ref={cintaRef} data-ribbon>
      {/* Tira de pestañas (32 px). */}
      <div class="border-border bg-surface flex h-[var(--editor-tabs-h)] items-end gap-1 border-b px-2">
        <ArchivoMenu ctx={ctx} />
        {/* Roving tabindex (patrón ARIA de tablist): el foco vive en los tabs —uno con
            tabIndex 0, el resto -1—; el tablist solo enruta las flechas y por eso no es
            enfocable en sí. La regla no contempla este patrón. */}
        {/* Deslizable en horizontal: las seis pestañas miden 495px y en un
            teléfono de 320 la última quedaba 258px fuera de la pantalla, sin
            scroll y sin forma de llegar a ella. Aquí SÍ se puede (a diferencia
            de la fila de grupos de abajo): un tab no abre popover, así que el
            `overflow-y:auto` que el navegador computa de rebote no recorta nada.
            `data-scroll-x` marca el desplazamiento como intencionado para la
            auditoría de móvil (test/e2e/mobile-audit.mjs). */}
        {/* eslint-disable-next-line jsx-a11y/interactive-supports-focus */}
        <div
          ref={listaRef}
          role="tablist"
          data-scroll-x
          aria-label={ctx.t('ribbon.label')}
          // min-w-0 flex-1: sin esto la tira mide sus 495px de contenido y empuja
          // fuera de la pantalla al botón de herramientas de la derecha, que en un
          // teléfono de 320px es la única forma de abrir la cinta.
          class={cx(
            'flex h-full min-w-0 flex-1 items-end gap-1 overflow-x-auto',
            // La pestaña que asoma cortada bajo el desvanecido ES el aviso de que
            // hay más: sin él, la tira parece terminar donde termina la pantalla.
            quedaTira && MASCARA_SCROLL_X,
          )}
          onKeyDown={onTablistKeyDown}
        >
          {visibles.map((t) => {
            const selected = t.id === active.id;
            return (
              <button
                key={t.id}
                type="button"
                role="tab"
                id={`ribbon-tab-${t.id}`}
                aria-selected={selected}
                aria-controls="ribbon-tabpanel"
                tabIndex={selected ? 0 : -1}
                ref={(el) => {
                  if (el) tabRefs.current.set(t.id, el);
                  else tabRefs.current.delete(t.id);
                }}
                class={
                  // shrink-0: dentro de un contenedor que ya desplaza, un tab que
                  // encoge parte su rótulo en dos líneas en vez de dejarse deslizar.
                  'relative h-full shrink-0 rounded-t-sm px-3 text-sm whitespace-nowrap transition-colors duration-150 ' +
                  (selected
                    ? 'text-text font-medium '
                    : 'text-text-muted hover:bg-bg-subtle hover:text-text ') +
                  FOCUS_RING
                }
                onClick={() => {
                  setActiveId(t.id);
                  setToolsOpen(true); // no-op en ≥768px (la fila está siempre)
                }}
              >
                {ctx.t(t.labelKey)}
                {selected && (
                  /* Subrayado 2 px pegado al borde inferior, con el color de «estás
                   * aquí» del registro (`--sytue-indicator`), legible en claro y oscuro. */
                  <span
                    aria-hidden="true"
                    class="bg-indicator absolute inset-x-1 bottom-0 h-0.5 rounded-full"
                  />
                )}
              </button>
            );
          })}
        </div>
        {/* Solo en <768px: despliega la fila de grupos. Al elegir pestaña con la
            fila plegada se abre sola — si no, tocar «Insertar» no haría nada
            visible y parecería que la pestaña está rota. */}
        <button
          type="button"
          onClick={() => setToolsOpen((v) => !v)}
          aria-expanded={toolsOpen}
          aria-controls="ribbon-tabpanel"
          aria-label={toolsOpen ? ctx.t('ribbon.hideTools') : ctx.t('ribbon.showTools')}
          title={toolsOpen ? ctx.t('ribbon.hideTools') : ctx.t('ribbon.showTools')}
          class={
            'text-text-muted hover:text-text ml-auto inline-flex h-full shrink-0 items-center px-2 md:hidden ' +
            FOCUS_RING
          }
        >
          <EditorIcon name={toolsOpen ? 'chevronUp' : 'chevronDown'} size="sm" />
        </button>
      </div>

      {/* La banda de grupos (68 px) de la pestaña activa: UNA línea que se desplaza en
       * horizontal cuando no cabe, con «Más» para verla entera.
       *
       * ⚠️ Esta fila era `flex-wrap` y sin `overflow-x`, porque con `overflow-x:auto` el
       * navegador computa `overflow-y:auto` y recorta los popovers que cuelgan de ella.
       * La medición de «0 px de overflow incluso a 1280» que había aquí se hizo SOLO en
       * castellano: medido el 2026-08-06 con la interfaz traducida, en Datos y a 1280 px,
       * el alemán medía 1304 px y el francés 1317, y envolvía dejando la cinta a dos o
       * tres líneas. Ahora los popovers son `fixed` (popovers.ts) y ya no hay nada que
       * recortar, así que la banda SÍ puede desplazarse: lo principal (Portapapeles,
       * Fuente, Párrafo…) queda a la izquierda y lo demás se alcanza deslizando, con el
       * teclado (el foco lleva el desplazamiento) o desde «Más».
       *
       * «Más» aparece solo si algo no cabe. Despliega la MISMA banda —no una copia: así
       * el estado de los controles no se duplica— en varias líneas sobre el lienzo, con
       * todos los grupos a la vista; el hueco que deja lo conserva el contenedor.
       *
       * «Más» OCUPA SU SITIO en la fila, a la derecha de la banda: la banda acaba donde
       * empieza el botón. Flotaba sobre la esquina con un relleno al final de la banda,
       * y un relleno al final solo se ve con la banda desplazada hasta el fondo: en reposo
       * el botón tapaba lo último que cabía (a 1280, la tarjeta «Cita» de la galería de
       * estilos, 64 px por debajo). Con la banda desplegada —fuera de flujo— el botón
       * queda en el mismo sitio, sobre su esquina. */}
      <div
        id="ribbon-tabpanel"
        role="tabpanel"
        aria-labelledby={`ribbon-tab-${active.id}`}
        ref={panelRef}
        class={cx(
          'border-border bg-surface relative flex border-b',
          // El de siempre como mínimo: con la banda desplegada (fuera de flujo) es lo
          // que mantiene el hueco, y con ella plegada es su altura natural.
          'min-h-[var(--editor-groups-h)]',
          !toolsOpen && 'max-md:hidden',
        )}
      >
        {/* Los manejadores van en un envoltorio sin semántica (`contents`: no cambia la
         * maquetación): un `tabpanel` no es interactivo y no debe llevarlos él. */}
        <div role="presentation" class="contents" onKeyDown={onPanelKeyDown} onClick={onPanelClick}>
          <BandaDesplegada.Provider value={expandida}>
            <Toolbar
              id="ribbon-banda"
              innerRef={bandaRef}
              aria-label={ctx.t(active.labelKey)}
              omitir=".ribbon-popover,[role='dialog']"
              data-scroll-x
              style={expandida ? { top: `${topExpandida}px` } : undefined}
              class={cx(
                'flex items-stretch gap-y-1 px-2',
                expandida
                  ? // Fuera de flujo y sobre el lienzo; el alto lo acota la pantalla, no el contenido.
                    'border-border bg-surface fixed inset-x-0 z-[var(--z-dropdown)] max-h-[60dvh] flex-wrap overflow-y-auto border-b py-1 pr-24 shadow-[var(--shadow-md)]'
                  : // Sin relleno vertical: el aire es el del grupo (`pt-1.5 pb-1`), y con el
                    // `py-1` que llevaba la banda medía 75 px y no los 68 del blueprint, que
                    // son los que mide la cinta nativa (medido el 2026-10-16).
                    'min-w-0 flex-1 [scrollbar-width:thin] flex-nowrap overflow-x-auto',
              )}
            >
              <ActiveBody ctx={ctx} />
            </Toolbar>
          </BandaDesplegada.Provider>
          {/* «Más» es un control de la cinta más, en su sitio de la fila: plano como los
              botones (sin el borde de campo que lo hacía leerse como un desplegable), a la
              altura de la fila de controles y no pegado arriba —estaba 14 px por encima de
              los iconos que tiene al lado— y con el separador de grupo delante, que es lo
              que dice que lo que hay a su izquierda sigue y él no. Desplegada, se queda en
              el mismo sitio, sobre su esquina. */}
          {desborda || expandida ? (
            <div class="relative z-[calc(var(--z-dropdown)+1)] ml-auto flex shrink-0 items-start pt-1.5 pr-2 max-md:pt-1">
              <div aria-hidden="true" class="bg-border mx-1.5 mt-1.5 h-8 w-px" />
              <div class="flex min-h-11 items-center">
                <button
                  ref={masRef}
                  type="button"
                  aria-expanded={expandida}
                  aria-controls="ribbon-banda"
                  title={ctx.t('ribbon.moreHint')}
                  onClick={alternarMas}
                  class={cx(
                    'text-text-muted hover:text-text hover:bg-bg-subtle aria-expanded:bg-selected aria-expanded:text-text',
                    'inline-flex h-7 items-center gap-1 rounded-sm px-2 text-xs font-medium transition-colors duration-150',
                    FOCUS_RING,
                  )}
                >
                  {ctx.t('ribbon.more')}
                  <EditorIcon name={expandida ? 'chevronUp' : 'chevronDown'} size="sm" />
                </button>
              </div>
            </div>
          ) : null}
        </div>
      </div>
    </div>
  );
}

/* ---------- Menú Archivo (§6.2) ---------- */

/**
 * Rótulo de la descarga en formato de Office y de OpenDocument, POR TIPO de
 * documento. Se nombra el formato que le toca a este documento —«Word», «Excel»,
 * «PowerPoint»— y no el estándar: nadie busca su hoja de cálculo por «OOXML».
 * Las extensiones las calcula `exportExt` con la misma correspondencia.
 */

function ArchivoMenu({ ctx }: { ctx: EditorContext }): JSX.Element {
  // Guardar versión pedía la etiqueta con el diálogo del navegador.
  const { preguntar, dialogo: dialogoPreguntar } = usePrompt();
  const [open, setOpen] = useState(false);
  useMobileBack(open, () => setOpen(false));
  const [exporting, setExporting] = useState(false);
  const [accesible, setAccesible] = useState(pdfAccesible);
  const rootRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent): void => {
      if (!rootRef.current?.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') {
        setOpen(false);
        triggerRef.current?.focus();
        return;
      }
      if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return;
      const items = Array.from(
        rootRef.current?.querySelectorAll<HTMLElement>('[role^="menuitem"]') ?? [],
      );
      if (items.length === 0) return;
      e.preventDefault();
      const idx = items.indexOf(document.activeElement as HTMLElement);
      const next =
        e.key === 'ArrowDown' ? (idx + 1) % items.length : (idx - 1 + items.length) % items.length;
      items[next]?.focus();
    };
    document.addEventListener('mousedown', onDown);
    document.addEventListener('keydown', onKey);
    // Foco al primer elemento al abrir (mismo patrón que el Dropdown de @sytue/ui).
    rootRef.current?.querySelector<HTMLElement>('[role^="menuitem"]')?.focus();
    return () => {
      document.removeEventListener('mousedown', onDown);
      document.removeEventListener('keydown', onKey);
    };
  }, [open]);

  /** Cierra el menú y ejecuta la acción del ítem. */
  const run = (action: () => void) => () => {
    setOpen(false);
    action();
  };

  const guardarVersion = async (): Promise<void> => {
    // Mismo flujo que Colaboración (§6.3): etiqueta OPCIONAL —de ahí
    // `permitirVacio`—; cancelar aborta.
    const label = await preguntar({
      mensaje: ctx.t('versions.labelPrompt'),
      permitirVacio: true,
    });
    if (label === null) return;
    void ctx.createVersion(label || undefined);
  };

  const exportar = async (
    format: (typeof OFFICE_EXPORTS)[number]['id'],
    plantilla = false,
  ): Promise<void> => {
    if (exporting || ctx.connection === 'offline') return;
    setExporting(true);
    try {
      await ctx.exportAs(format, plantilla);
    } finally {
      setExporting(false);
    }
  };

  return (
    <div ref={rootRef} class="relative flex h-full items-center">
      <button
        ref={triggerRef}
        type="button"
        aria-haspopup="menu"
        aria-expanded={open}
        // La única entrada de la tira que NO es una pestaña (abre un menú de documento, no
        // cambia la cinta): en tinta, como la banda de cabecera de los paneles, para que no
        // se lea como una pestaña más. Con el menú abierto (`aria-expanded`) o al apuntarlo se aclara.
        class={
          'bg-header text-header-fg hover:brightness-125 aria-expanded:brightness-125 ' +
          'ring-chrome-border h-6 rounded-sm px-3 text-sm font-medium ring-1 ring-inset pointer-coarse:h-8 ' +
          'transition-colors duration-150 ' +
          'sytue-focus-ring-chrome focus-visible:outline-none'
        }
        onClick={() => setOpen((v) => !v)}
      >
        {ctx.t('file.menu')}
      </button>
      {open && (
        <div
          role="menu"
          aria-label={ctx.t('file.menu')}
          class="sytue-menu-surface border-border absolute top-full left-0 z-[var(--z-popover)] mt-1 w-max max-w-[min(360px,90vw)] min-w-[220px] rounded-lg border p-1 shadow-[var(--shadow-md)]"
        >
          {/* Devolver a Drive: solo si el documento SALIÓ de un fichero de allí. Es
              lo que cierra el viaje de «Abrir con Sytue Docs»; sin esta entrada,
              editar un .docx de Drive deja el original intacto y los cambios en un
              documento aparte, que no es lo que nadie espera al abrir un fichero. */}
          {ctx.sourceRef ? (
            <MenuItem
              size="sm"
              disabled={ctx.connection === 'offline' || ctx.savingToDrive}
              onClick={run(() => void ctx.saveToDrive())}
            >
              {ctx.t('file.saveToDrive')}
            </MenuItem>
          ) : null}
          {OFFICE_EXPORTS.map((command) => (
            <Fragment key={command.id}>
              <MenuItem
                size="sm"
                disabled={!officeExportAvailable(command, ctx.connection === 'offline', exporting)}
                onClick={run(() => void exportar(command.id))}
              >
                {ctx.t(command.labels[ctx.kind] as DocsKey)}
              </MenuItem>
              {/* Interruptor de la descarga («PDF accesible»): cambia la preferencia y deja
                  el menú abierto, que es donde se ve el tic. */}
              {command.option ? (
                <MenuItem
                  size="sm"
                  role="menuitemcheckbox"
                  aria-checked={accesible}
                  class="pl-6"
                  onClick={() => {
                    fijarPdfAccesible(!accesible);
                    setAccesible(!accesible);
                  }}
                >
                  <span class="flex-1">{ctx.t(command.option.label as DocsKey)}</span>
                  {accesible ? <EditorIcon name="check" size="sm" /> : null}
                </MenuItem>
              ) : null}
            </Fragment>
          ))}
          {/* Como PLANTILLA de Office u OpenDocument: el mismo paquete que Word, Excel,
              PowerPoint o LibreOffice abren como documento nuevo (plantillas-1). */}
          {(['ooxml', 'odf'] as const).map((formato) => (
            <MenuItem
              key={`plantilla-${formato}`}
              size="sm"
              disabled={ctx.connection === 'offline' || exporting}
              onClick={run(() => void exportar(formato, true))}
            >
              {ctx.t(
                formato === 'ooxml' ? 'file.downloadTemplateOoxml' : 'file.downloadTemplateOdf',
                { ext: exportExt(ctx.kind, formato, true) },
              )}
            </MenuItem>
          ))}
          <MenuSeparator />
          <MenuItem
            size="sm"
            disabled={ctx.connection === 'offline'}
            onClick={run(() => void guardarVersion())}
          >
            {ctx.t('file.saveVersion')}
          </MenuItem>
          <MenuItem
            size="sm"
            onClick={run(() => document.getElementById('editor-title-input')?.focus())}
          >
            {ctx.t('file.rename')}
          </MenuItem>
          {/* La biblioteca de plantillas de la organización: marcar este documento,
              publicar una versión o quitarlo (plantillas-1). */}
          <PlantillaOrgItems ctx={ctx} cerrar={() => setOpen(false)} />
          <MenuSeparator />
          {/* Al Hub, igual que el «<» de la cabecera: ver el comentario de
              EditorHeader. Eran dos salidas al mismo sitio equivocado. */}
          <MenuItem size="sm" href={appUrl('one')} onClick={() => setOpen(false)}>
            {ctx.t('file.back')}
          </MenuItem>
        </div>
      )}
      {dialogoPreguntar}
    </div>
  );
}
