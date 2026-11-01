/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Cinta de Sytue CertiPDF.
 *
 * ERA UNA BARRA ÚNICA de 415 px con catorce herramientas, seis colores, el
 * grosor, la paginación, el zoom y seis diálogos, todo a la vez. Cabía en un
 * portátil y en un teléfono había que arrastrarla de lado para llegar al final.
 * Peor que eso: para encontrar «censurar» había que leerla entera, porque nada
 * decía qué era cada tramo.
 *
 * AHORA SON PESTAÑAS, como en el editor de documentos de esta misma app, y por
 * el mismo motivo: en un programa con sesenta acciones lo que ahorra tiempo no
 * es tenerlas a la vista, es saber dónde mirar. Cada pestaña enseña lo suyo con
 * el rótulo del grupo debajo, así que «Firmar» se busca en Firmar y no
 * recorriendo iconos.
 *
 * SE REUTILIZA `RibbonGroup` del editor —no una copia— para que las dos cintas
 * de la app sean la misma cinta: mismos altos, mismos rótulos y el mismo
 * comportamiento al envolver en pantallas estrechas.
 *
 * Y lo que el documento PROHÍBE no se enseña deshabilitado a secas: se enseña
 * con el motivo. Un botón gris sin explicación hace que la gente pruebe tres
 * veces.
 */
import { useState } from 'preact/hooks';
import { appUrl, driveFolderUrl } from '@sytue/config';
import { Icon, Spinner, Toolbar, cx, useTablist, type IconName } from '@sytue/ui';
import { RibbonGroup } from '../editor/ribbon/primitives';
import { MASCARA_SCROLL_X, useScrollRestante } from '@sytue/ui';
import type { DocsKey, docsT } from '../i18n';
import type { Permisos } from './api';
import type { Dialogo } from './Paneles';

export type Herramienta =
  | 'mano'
  | 'resaltar'
  | 'subrayar'
  | 'tachar'
  | 'nota'
  | 'lapiz'
  | 'borrador'
  | 'rectangulo'
  | 'elipse'
  | 'linea'
  | 'flecha'
  | 'medida'
  | 'texto'
  | 'sello'
  | 'campo'
  | 'recorte'
  | 'censura';

interface Props {
  t: ReturnType<typeof docsT>;
  herramienta: Herramienta;
  onHerramienta: (h: Herramienta) => void;
  color: string;
  onColor: (c: string) => void;
  grosor: number;
  onGrosor: (g: number) => void;
  zoom: number;
  ajuste: 'ancho' | 'pagina' | 'libre';
  onZoom: (delta: number) => void;
  onAjuste: (a: 'ancho' | 'pagina' | 'libre') => void;
  dosPaginas: boolean;
  onDosPaginas: (v: boolean) => void;
  leyendo: boolean;
  onLeer: () => void;
  pagina: number;
  total: number;
  onIrA: (n: number) => void;
  onDialogo: (d: Dialogo) => void;
  permisos: Permisos;
  trabajando: boolean;
  panelAbierto: boolean;
  onPanelAbierto: (abierto: boolean) => void;
  /** Carpeta de Drive desde la que se abrió el PDF, si vino de allí. */
  volverA?: string | undefined;
}

/**
 * Herramienta → clave de traducción e icono.
 *
 * El icono es el NOMBRE de un glifo de `@sytue/ui`, no un carácter: aquí había
 * catorce emoji (✋ 🖍 💬 ✏️ 📐 🏷) y dos apaños con diacrítico (`U̲`, `S̶`), que
 * se pintaban a todo color sobre marfil y grafito y medían distinto en cada
 * sistema —cada emoji trae su propio avance tipográfico—, así que ni el ancho de
 * la fila era el mismo en un iPhone que en un Android.
 */
const HERRAMIENTAS: { id: Herramienta; clave: DocsKey; icono: IconName; grupo: number }[] = [
  { id: 'mano', clave: 'pdf.tool.hand', icono: 'hand', grupo: 0 },
  { id: 'resaltar', clave: 'pdf.tool.highlight', icono: 'highlighter', grupo: 1 },
  { id: 'subrayar', clave: 'pdf.tool.underline', icono: 'underline', grupo: 1 },
  { id: 'tachar', clave: 'pdf.tool.strike', icono: 'strikethrough', grupo: 1 },
  { id: 'nota', clave: 'pdf.tool.note', icono: 'comment', grupo: 1 },
  { id: 'lapiz', clave: 'pdf.tool.pen', icono: 'pen', grupo: 2 },
  { id: 'rectangulo', clave: 'pdf.tool.rect', icono: 'square', grupo: 2 },
  { id: 'elipse', clave: 'pdf.tool.ellipse', icono: 'circle', grupo: 2 },
  { id: 'linea', clave: 'pdf.tool.line', icono: 'line', grupo: 2 },
  { id: 'flecha', clave: 'pdf.tool.arrow', icono: 'arrowUpRight', grupo: 2 },
  { id: 'medida', clave: 'pdf.tool.measure', icono: 'ruler', grupo: 2 },
  { id: 'texto', clave: 'pdf.tool.text', icono: 'text', grupo: 2 },
  { id: 'sello', clave: 'pdf.tool.stamp', icono: 'stamp', grupo: 2 },
  // Crear un campo NO es anotar: convierte el documento en un formulario que
  // otro va a rellenar. Va en su propio grupo por eso, no por el icono.
  { id: 'campo', clave: 'pdf.tool.field', icono: 'formField', grupo: 3 },
  // Recortar NO toca el documento: se lleva un trozo de la página como imagen.
  // Por eso va con el campo y no con las anotaciones.
  { id: 'recorte', clave: 'pdf.tool.snapshot', icono: 'recorte', grupo: 3 },
  // Censurar va en su propio grupo y el último: es lo único de la barra que
  // DESTRUYE contenido, y ponerlo junto al resaltador es invitar a confundirlos.
  { id: 'censura', clave: 'pdf.tool.redact', icono: 'redact', grupo: 4 },
];

/** Colores de anotación. Los de la casa primero, y todos con nombre. */
const COLORES: { valor: string; clave: DocsKey }[] = [
  { valor: '#f9b923', clave: 'pdf.color.amber' },
  { valor: '#22c55e', clave: 'pdf.color.green' },
  { valor: '#3b82f6', clave: 'pdf.color.blue' },
  { valor: '#ef4444', clave: 'pdf.color.red' },
  { valor: '#a855f7', clave: 'pdf.color.violet' },
  { valor: '#111827', clave: 'pdf.color.ink' },
];

/** Pestañas de la cinta, en el orden en que se usan a lo largo de un documento. */
type Pestana = 'inicio' | 'anotar' | 'formulario' | 'organizar' | 'firmar' | 'ver';

const PESTANAS: { id: Pestana; clave: DocsKey }[] = [
  { id: 'inicio', clave: 'ribbon.tab.home' },
  { id: 'anotar', clave: 'pdf.ribbon.tab.annotate' },
  { id: 'formulario', clave: 'pdf.ribbon.tab.form' },
  { id: 'organizar', clave: 'pdf.menu.organize' },
  // «Firmar» y no «Proteger» (orden del dueño, 2026-10-16): firmar es a lo que se
  // viene a CertiPDF, y la pestaña se llamaba como su botón de contraseñas. El rótulo
  // es el del botón «Firmar», como «Organizar» es el del suyo: `pdf.menu.protect`
  // sigue nombrando ese botón, que sí protege.
  { id: 'firmar', clave: 'pdf.menu.sign' },
  { id: 'ver', clave: 'ribbon.tab.view' },
];

const IDS_PESTANAS = PESTANAS.map((x) => x.id);

/** Qué herramientas viven en cada pestaña. Las que no aparecen aquí no existen. */
const EN_PESTANA: Record<Pestana, Herramienta[]> = {
  inicio: ['mano', 'resaltar', 'subrayar', 'tachar', 'nota'],
  anotar: ['lapiz', 'rectangulo', 'elipse', 'linea', 'flecha', 'medida', 'texto', 'sello'],
  formulario: ['campo', 'recorte'],
  organizar: [],
  // Censurar es lo único de la cinta que DESTRUYE contenido: vive en Firmar, con
  // la contraseña, lejos del resaltador con el que se confundía.
  firmar: ['censura'],
  ver: [],
};

export function BarraHerramientas(p: Props) {
  const { t } = p;
  const [pestana, setPestana] = useState<Pestana>('inicio');
  // Mismo aviso que la tira de pestañas del editor: sin él, una fila que se
  // desliza parece terminar en el borde de la pantalla.
  const { ref, queda } = useScrollRestante<HTMLDivElement>();
  const puedeAnotar = p.permisos.Anotar || !p.permisos.SoloConClaveDeDueno;

  const herramientas = EN_PESTANA[pestana]
    .map((id) => HERRAMIENTAS.find((h) => h.id === id))
    .filter((h): h is (typeof HERRAMIENTAS)[number] => h !== undefined);
  // El color y el grosor solo pintan donde hay algo que dibujar. La censura no:
  // su barra es negra y opaca SIEMPRE (`#111111`), y seis colores que no cambian
  // nada eran la mitad de la pestaña de firmar.
  const conTrazo = herramientas.some(
    (h) => h.id !== 'mano' && h.id !== 'recorte' && h.id !== 'censura',
  );

  // Roving tabindex, flechas y enlace pestaña-panel (@sytue/ui): ver `useTablist`.
  const tl = useTablist(IDS_PESTANAS, pestana, setPestana);

  return (
    <div class="sytue-sheet-corners border-border bg-surface flex flex-col border-b [--sytue-sheet-bl:0px] [--sytue-sheet-br:0px]">
      {/* Fila de pestañas. En el teléfono se desliza: seis rótulos no caben en
          320 px, y plegarlos en un menú esconde justo lo que hay que ver. */}
      <div class="flex items-center gap-1 px-1 pt-1">
        {/* LA SALIDA, lo primero y como en los otros tres editores. CertiPDF no
            tenía ninguna: se salía por el conmutador de nueve puntos, que es una
            puerta escondida al lado del «<» que Docus, Calcus y Diapos llevan en
            su cabecera. Va al Hub por el mismo motivo que ellos: `/docs/` es la
            lista genérica sin producto, la portada conjunta que se decidió no
            tener.

            Es un `<a>` y no un botón porque es navegación: así se abre en otra
            pestaña con el clic central o con Cmd, como cualquier enlace. */}
        {/* Con `volverA`, la salida devuelve A DONDE SE ENTRÓ: quien abrió el
            PDF desde su carpeta de Drive no venía del Hub, y dejarlo allí le
            obliga a rehacer el camino a mano. Sin él, al Hub como siempre. */}
        <a
          href={p.volverA ? driveFolderUrl(p.volverA) : appUrl('one')}
          aria-label={p.volverA ? t('pdf.backToDrive') : t('pdf.back')}
          title={p.volverA ? t('pdf.backToDrive') : t('pdf.back')}
          class="sytue-focus-ring text-text hover:bg-bg-subtle flex h-9 w-9 shrink-0 items-center justify-center rounded-md"
        >
          <Icon name="chevronLeft" size={18} />
        </a>
        <button
          type="button"
          class="sytue-focus-ring text-text hover:bg-bg-subtle rounded-control mr-1 flex h-9 w-9 shrink-0 items-center justify-center lg:hidden"
          aria-expanded={p.panelAbierto}
          aria-controls="pdf-panel"
          aria-label={t('pdf.panel.title')}
          title={t('pdf.panel.title')}
          onClick={() => p.onPanelAbierto(!p.panelAbierto)}
        >
          <Icon name="menu" size={18} />
        </button>
        {/* `data-scroll-x`: la tira de pestañas se desliza de lado A PROPÓSITO en
            pantallas estrechas, igual que la cinta de abajo. Sin declararlo, el
            barrido de desbordamiento lo contaba como defecto (+187px en iPhone
            SE, +117 en iPhone 12, señalando el botón «Ver»). */}
        {/* eslint-disable-next-line jsx-a11y/interactive-supports-focus -- el tablist no se enfoca: lo hace su pestaña activa (roving) */}
        <div
          data-scroll-x
          class="flex min-w-0 gap-0.5 overflow-x-auto"
          role="tablist"
          aria-label={t('pdf.tools')}
          onKeyDown={tl.onKeyDown}
        >
          {PESTANAS.map((x) => (
            <button
              key={x.id}
              type="button"
              {...tl.tab(x.id)}
              class={cx(
                'sytue-focus-ring shrink-0 rounded-t-md px-3 py-1.5 text-xs font-medium whitespace-nowrap transition-colors',
                pestana === x.id
                  ? 'bg-bg-subtle text-text'
                  : 'text-text-muted hover:bg-bg-subtle hover:text-text',
              )}
            >
              {t(x.clave)}
            </button>
          ))}
        </div>
        <div class="flex-1" />
        {p.trabajando && <Spinner size={16} class="mr-2" label={t('pdf.working')} />}
      </div>

      {/* Grupos de la pestaña activa. Se desliza de lado en pantallas estrechas,
          igual que la cinta del editor: envolver de verdad son varias filas, y en
          un visor de documentos el alto ES el producto. */}
      <div {...tl.panel}>
        <Toolbar
          innerRef={ref}
          data-scroll-x
          class={cx(
            'bg-bg-subtle flex items-stretch gap-1 overflow-x-auto px-1 py-0.5',
            queda && MASCARA_SCROLL_X,
          )}
          aria-label={t(PESTANAS.find((x) => x.id === pestana)!.clave)}
        >
          {/* Lo primero de Firmar es firmar. Detrás, lo que protege el documento
              —quién lo abre y qué se le puede hacer— y, la última, la censura:
              son los pasos con que se cierra un documento antes de enviarlo, y
              Foxit, PDF-XChange y Nitro los reúnen también en una sola pestaña. */}
          {pestana === 'firmar' && (
            <>
              <RibbonGroup label={t('pdf.panel.signatures')}>
                <BotonTexto onClick={() => p.onDialogo('firmar')} disabled={p.trabajando}>
                  {t('pdf.menu.sign')}
                </BotonTexto>
                <BotonTexto onClick={() => p.onDialogo('pedirFirma')} disabled={p.trabajando}>
                  {t('pdf.menu.requestSignature')}
                </BotonTexto>
              </RibbonGroup>
              <RibbonGroup label={t('pdf.ribbon.group.security')}>
                <BotonTexto onClick={() => p.onDialogo('proteger')} disabled={p.trabajando}>
                  {t('pdf.menu.protect')}
                </BotonTexto>
              </RibbonGroup>
            </>
          )}

          {herramientas.length > 0 && (
            <RibbonGroup label={t('pdf.tools')}>
              {herramientas.map((h) => (
                <BotonHerramienta
                  key={h.id}
                  activo={p.herramienta === h.id}
                  titulo={t(h.clave)}
                  icono={h.icono}
                  deshabilitado={h.id !== 'mano' && !puedeAnotar}
                  motivo={t('pdf.blocked.annotate')}
                  onClick={() => p.onHerramienta(h.id)}
                />
              ))}
            </RibbonGroup>
          )}

          {conTrazo && (
            <RibbonGroup label={t('pdf.ribbon.group.stroke')}>
              {COLORES.map((c) => (
                <button
                  key={c.valor}
                  type="button"
                  title={t(c.clave)}
                  aria-label={t(c.clave)}
                  aria-pressed={p.color === c.valor}
                  onClick={() => p.onColor(c.valor)}
                  class={cx(
                    // 24 px de caja (WCAG 2.5.8: eran 17,5, 1,25 rem con el rem de 14) y 44 con
                    // el dedo, por el suelo táctil de la zona (`data-suelo-tactil`).
                    'sytue-focus-ring h-[24px] w-[24px] rounded-full border transition',
                    p.color === c.valor ? 'border-text scale-110' : 'border-border',
                  )}
                  style={{ backgroundColor: c.valor }}
                />
              ))}
              <label class="text-text-muted ml-2 flex items-center gap-1 text-xs">
                {t('pdf.thickness')}
                <input
                  type="range"
                  min="1"
                  max="12"
                  step="1"
                  value={p.grosor}
                  onInput={(e) => p.onGrosor(Number((e.target as HTMLInputElement).value))}
                  class="sytue-focus-ring w-16"
                  aria-label={t('pdf.thickness')}
                />
              </label>
            </RibbonGroup>
          )}

          {pestana === 'organizar' && (
            <RibbonGroup label={t('pdf.ribbon.group.document')}>
              <BotonTexto onClick={() => p.onDialogo('organizar')} disabled={p.trabajando}>
                {t('pdf.menu.organize')}
              </BotonTexto>
              <BotonTexto onClick={() => p.onDialogo('herramientas')} disabled={p.trabajando}>
                {t('pdf.menu.tools')}
              </BotonTexto>
              <BotonTexto onClick={() => p.onDialogo('capas')} disabled={p.trabajando}>
                {t('pdf.menu.stamp')}
              </BotonTexto>
              <BotonTexto onClick={() => p.onDialogo('comparar')} disabled={p.trabajando}>
                {t('pdf.menu.compare')}
              </BotonTexto>
            </RibbonGroup>
          )}

          {pestana === 'ver' && (
            <>
              <RibbonGroup label={t('pdf.ribbon.group.page')}>
                <BotonSimple
                  titulo={t('pdf.page.prev')}
                  onClick={() => p.onIrA(p.pagina - 1)}
                  icono="chevronLeft"
                  deshabilitado={p.pagina <= 1}
                />
                <label class="text-text-muted flex items-center gap-1 text-xs">
                  <input
                    type="number"
                    min={1}
                    max={p.total}
                    value={p.pagina}
                    onChange={(e) => p.onIrA(Number((e.target as HTMLInputElement).value))}
                    class="border-border bg-surface text-text sytue-focus-ring w-14 rounded-md border px-1.5 py-1 text-center text-xs tabular-nums"
                    aria-label={t('pdf.page.current')}
                  />
                  <span class="tabular-nums">/ {p.total}</span>
                </label>
                <BotonSimple
                  titulo={t('pdf.page.next')}
                  onClick={() => p.onIrA(p.pagina + 1)}
                  icono="chevronRight"
                  deshabilitado={p.pagina >= p.total}
                />
              </RibbonGroup>
              <RibbonGroup label={t('pdf.ribbon.group.zoom')}>
                <BotonSimple
                  titulo={t('pdf.zoom.out')}
                  onClick={() => p.onZoom(-0.15)}
                  icono="minus"
                />
                <span class="text-text-muted w-12 shrink-0 text-center text-xs tabular-nums">
                  {Math.round(p.zoom * 100)}%
                </span>
                <BotonSimple
                  titulo={t('pdf.zoom.in')}
                  onClick={() => p.onZoom(0.15)}
                  icono="plus"
                />
                <BotonSimple
                  titulo={t('pdf.zoom.width')}
                  activo={p.ajuste === 'ancho'}
                  onClick={() => p.onAjuste('ancho')}
                  icono="fitWidth"
                />
                <BotonSimple
                  titulo={t('pdf.zoom.page')}
                  activo={p.ajuste === 'pagina'}
                  onClick={() => p.onAjuste('pagina')}
                  icono="fitPage"
                />
              </RibbonGroup>
              <RibbonGroup label={t('pdf.ribbon.group.reading')}>
                <BotonSimple
                  titulo={t('pdf.view.twoUp')}
                  activo={p.dosPaginas}
                  onClick={() => p.onDosPaginas(!p.dosPaginas)}
                  icono="dosPaginas"
                />
                <BotonSimple
                  titulo={t(p.leyendo ? 'pdf.speak.stop' : 'pdf.speak.start')}
                  activo={p.leyendo}
                  onClick={p.onLeer}
                  icono="voz"
                />
              </RibbonGroup>
            </>
          )}
        </Toolbar>
      </div>
    </div>
  );
}

function BotonHerramienta({
  activo,
  titulo,
  icono,
  onClick,
  deshabilitado,
  motivo,
}: {
  activo: boolean;
  titulo: string;
  icono: IconName;
  onClick: () => void;
  deshabilitado?: boolean;
  motivo?: string;
}) {
  return (
    <button
      type="button"
      title={deshabilitado ? motivo : titulo}
      aria-label={titulo}
      aria-pressed={activo}
      disabled={deshabilitado}
      onClick={onClick}
      class={`sytue-focus-ring rounded-control flex h-8 w-8 shrink-0 items-center justify-center transition ${
        // `bg-selected`, no `bg-accent`: en tema claro accent y accent-text son el
        // MISMO gris 700, y la herramienta activa salía como un cuadrado negro sin icono.
        activo ? 'bg-selected text-text' : 'text-text hover:bg-bg-subtle'
      } disabled:cursor-not-allowed disabled:opacity-40`}
    >
      <Icon name={icono} size={16} />
    </button>
  );
}

function BotonSimple({
  titulo,
  onClick,
  activo,
  icono,
  deshabilitado,
}: {
  titulo: string;
  onClick: () => void;
  activo?: boolean;
  icono: IconName;
  deshabilitado?: boolean;
}) {
  return (
    <button
      type="button"
      title={titulo}
      aria-label={titulo}
      // `aria-pressed` solo donde el botón ES un interruptor (ajustar al ancho,
      // dos páginas). En «página anterior» no lo es, y anunciarlo como «no
      // pulsado» confunde a quien usa un lector de pantalla.
      {...(activo === undefined ? {} : { 'aria-pressed': activo })}
      disabled={deshabilitado}
      onClick={onClick}
      class={`sytue-focus-ring rounded-control flex h-7 w-7 shrink-0 items-center justify-center disabled:cursor-not-allowed disabled:opacity-40 ${
        activo ? 'bg-selected text-text' : 'text-text hover:bg-bg-subtle'
      }`}
    >
      <Icon name={icono} size={15} />
    </button>
  );
}

function BotonTexto({
  onClick,
  disabled,
  children,
}: {
  onClick: () => void;
  disabled?: boolean;
  children: preact.ComponentChildren;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      // shrink-0 + nowrap: dentro de una barra que se desliza, un rótulo que
      // encoge se parte en dos líneas en vez de dejarse arrastrar.
      class="text-text hover:bg-bg-subtle sytue-focus-ring rounded-control shrink-0 px-2 py-1.5 text-xs font-medium whitespace-nowrap disabled:cursor-not-allowed disabled:opacity-40"
    >
      {children}
    </button>
  );
}
