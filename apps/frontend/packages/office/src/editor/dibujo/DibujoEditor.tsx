/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Editor del bloque «Dibujo»: un diálogo con un lienzo vectorial sobre la escena
 * común (`@sytue/escena`). Es ACCESIBLE por diseño y no un añadido:
 *
 *  - Todo lo que se hace con el puntero se puede hacer con el teclado: las
 *    herramientas son botones, Intro coloca el objeto en el centro, las flechas
 *    mueven la selección (Mayús: pasos grandes), «[» y «]» recorren los objetos,
 *    Supr borra y los campos numéricos del panel dan posición, tamaño y giro.
 *  - La lista «Objetos» es la vía principal de selección sin puntero: cada objeto
 *    tiene un nombre accesible y un botón.
 *  - Lo que ocurre se anuncia en una región `aria-live` (añadido, movido, borrado).
 *  - El lienzo es un grupo con nombre y descripción; el SVG de la escena lleva
 *    `role="img"` con el título del dibujo.
 *
 * La interacción con el puntero (mover, redimensionar, dibujar arrastrando, marco
 * de selección) funciona con ratón, táctil y lápiz: todo va por Pointer Events con
 * `setPointerCapture`. Cada gesto es UN paso del historial, no uno por movimiento.
 *
 * La lógica pura (historial, nodos nuevos) vive en ./estado y la de la escena en
 * @sytue/escena: aquí solo está lo que toca el DOM y los eventos.
 */
import type { JSX } from 'preact';
import { useLayoutEffect, useMemo, useRef, useState } from 'preact/hooks';
import {
  Button,
  Checkbox,
  FieldGroup,
  Input,
  Modal,
  Select,
  Toolbar,
  useConfirm,
  useUltimo,
} from '@sytue/ui';
import {
  actualizarNodo,
  agregarCapa,
  anadirDiagrama,
  TIPOS_DIAGRAMA,
  agregarNodo,
  agrupar,
  ajustarCapa,
  alinearNodos,
  aSvg,
  buscar,
  cajaMundo,
  desagrupar,
  distribuirNodos,
  eliminarCapa,
  eliminarNodos,
  golpear,
  limpiar,
  moverNodos,
  nodosEnRect,
  nuevoId,
  ordenarCapa,
  ordenarZ,
  redimensionar,
  renombrarCapa,
} from '@sytue/escena';
import type {
  TipoDiagrama,
  AlignEdge,
  DistAxis,
  Escena,
  Nodo,
  Punto,
  Rect,
  ZOrderOp,
} from '@sytue/escena';
import { useT } from '../context';
import type { DocsKey } from '../../i18n';
import { descargar } from '../dibujo';
import { ComentarDibujo } from './ComentarDibujo';
import type { DestinoComentario } from './ComentarDibujo';
import { EditorIcon } from '../icons';
import type { EditorIconName } from '../icons';
import {
  confirmar,
  deshacer,
  nodoNuevo,
  nombreConOrdinal,
  nuevoHistorial,
  rehacer,
} from './estado';
import type { Herramienta, HerramientaObjeto } from './estado';

/* ---------- Catálogos ---------- */

/** Herramientas de la barra, con su icono y la clave del rótulo. */
const HERRAMIENTAS: { id: Herramienta; icono: EditorIconName; clave: DocsKey }[] = [
  { id: 'seleccionar', icono: 'pointer', clave: 'drawing.tool.select' },
  { id: 'rect', icono: 'rect', clave: 'slides.shape.rect' },
  { id: 'ellipse', icono: 'ellipse', clave: 'slides.shape.ellipse' },
  { id: 'triangle', icono: 'triangle', clave: 'slides.shape.triangle' },
  { id: 'diamond', icono: 'diamond', clave: 'slides.shape.diamond' },
  { id: 'pentagon', icono: 'pentagon', clave: 'slides.shape.pentagon' },
  { id: 'hexagon', icono: 'hexagon', clave: 'slides.shape.hexagon' },
  { id: 'star', icono: 'star', clave: 'slides.shape.star' },
  { id: 'cylinder', icono: 'cylinder', clave: 'slides.shape.cylinder' },
  { id: 'callout', icono: 'callout', clave: 'slides.shape.callout' },
  { id: 'linea', icono: 'line', clave: 'slides.shape.line' },
  { id: 'flecha', icono: 'arrow', clave: 'slides.shape.arrow' },
  { id: 'texto', icono: 'textBox', clave: 'drawing.tool.text' },
  { id: 'lapiz', icono: 'draw', clave: 'drawing.tool.pencil' },
  { id: 'borrador', icono: 'eraser', clave: 'drawing.tool.eraser' },
];

const ALINEACIONES: { id: AlignEdge; clave: DocsKey }[] = [
  { id: 'left', clave: 'align.left' },
  { id: 'hcenter', clave: 'align.hcenter' },
  { id: 'right', clave: 'align.right' },
  { id: 'top', clave: 'align.top' },
  { id: 'vmiddle', clave: 'align.vmiddle' },
  { id: 'bottom', clave: 'align.bottom' },
];

const ORDENES: { id: ZOrderOp; clave: DocsKey }[] = [
  { id: 'front', clave: 'slides.order.front' },
  { id: 'forward', clave: 'slides.order.forward' },
  { id: 'backward', clave: 'slides.order.backward' },
  { id: 'back', clave: 'slides.order.back' },
];

const CLAVE_FORMA: Record<string, DocsKey> = {
  rect: 'slides.shape.rect',
  ellipse: 'slides.shape.ellipse',
  triangle: 'slides.shape.triangle',
  diamond: 'slides.shape.diamond',
  pentagon: 'slides.shape.pentagon',
  hexagon: 'slides.shape.hexagon',
  star: 'slides.shape.star',
  cylinder: 'slides.shape.cylinder',
  callout: 'slides.shape.callout',
};

const TRAZADOS: { id: 'straight' | 'elbow' | 'curved'; clave: DocsKey }[] = [
  { id: 'straight', clave: 'slides.connector.straight' },
  { id: 'elbow', clave: 'slides.connector.elbow' },
  { id: 'curved', clave: 'slides.connector.curved' },
];

/** Margen de golpeo del puntero, en píxeles de pantalla. */
const TOLERANCIA_PX = 5;
const LADO_ASA_PX = 9;

type Asa = 'nw' | 'n' | 'ne' | 'e' | 'se' | 's' | 'sw' | 'w';
const ASAS: Asa[] = ['nw', 'n', 'ne', 'e', 'se', 's', 'sw', 'w'];
const CURSOR_ASA: Record<Asa, string> = {
  nw: 'nwse-resize',
  se: 'nwse-resize',
  ne: 'nesw-resize',
  sw: 'nesw-resize',
  n: 'ns-resize',
  s: 'ns-resize',
  e: 'ew-resize',
  w: 'ew-resize',
};

type Arrastre =
  | { tipo: 'mover'; desde: Punto; base: Escena; ids: string[] }
  | { tipo: 'crear'; desde: Punto; base: Escena; herramienta: HerramientaObjeto }
  | { tipo: 'lapiz'; base: Escena; id: string; puntos: number[] }
  | { tipo: 'borrar'; base: Escena; actual: Escena; ids: string[] }
  | { tipo: 'asa'; asa: Asa; base: Escena; id: string; caja: Rect }
  | { tipo: 'marco'; desde: Punto; base: Escena; aditivo: boolean; previa: string[] };

/** Caja nueva al arrastrar el asa `asa` hasta `p` (mínimo 4 unidades de lado). */
function cajaConAsa(c: Rect, asa: Asa, p: Punto): Rect {
  let x1 = c.x;
  let y1 = c.y;
  let x2 = c.x + c.w;
  let y2 = c.y + c.h;
  if (asa.includes('w')) x1 = Math.min(p.x, x2 - 4);
  if (asa.includes('e')) x2 = Math.max(p.x, x1 + 4);
  if (asa.includes('n')) y1 = Math.min(p.y, y2 - 4);
  if (asa.includes('s')) y2 = Math.max(p.y, y1 + 4);
  return { x: Math.round(x1), y: Math.round(y1), w: Math.round(x2 - x1), h: Math.round(y2 - y1) };
}

const posAsa = (c: Rect, asa: Asa): Punto => ({
  x: asa.includes('w') ? c.x : asa.includes('e') ? c.x + c.w : c.x + c.w / 2,
  y: asa.includes('n') ? c.y : asa.includes('s') ? c.y + c.h : c.y + c.h / 2,
});

/* ---------- Componente ---------- */

export interface DibujoEditorProps {
  open: boolean;
  /** Escena que se edita (ya saneada). */
  inicial: Escena;
  /** Rótulo del botón de aceptar («Insertar» o «Aplicar»). */
  aceptar: string;
  onGuardar: (escena: Escena) => void;
  onCancelar: () => void;
  /** Comentar la forma o coordenada (solo en un dibujo ya insertado); `undefined` = sin formulario. */
  onComentar?: ((cuerpo: string, destino: DestinoComentario) => Promise<void>) | undefined;
}

export function DibujoEditor({
  open,
  inicial,
  aceptar,
  onGuardar,
  onCancelar,
  onComentar,
}: DibujoEditorProps): JSX.Element | null {
  const t = useT();
  const [hist, setHist] = useState(() => nuevoHistorial(inicial));
  const [borrador, setBorrador] = useState<Escena | null>(null);
  const [sel, setSel] = useState<string[]>([]);
  const [herr, setHerr] = useState<Herramienta>('seleccionar');
  const [capaActiva, setCapaActiva] = useState(inicial.capas[inicial.capas.length - 1]!.id);
  const [aviso, setAviso] = useState('');
  // Tinta y grosor del lápiz (sin color = la tinta del tema).
  const [lapiz, setLapiz] = useState<{ color?: string; grosor: number }>({ grosor: 2.5 });
  const [marco, setMarco] = useState<Rect | null>(null);
  const [escalaPx, setEscalaPx] = useState(1);
  const lienzoRef = useRef<HTMLDivElement>(null);
  const capaRef = useRef<SVGSVGElement>(null);
  const arrRef = useRef<Arrastre | null>(null);

  // El anfitrión monta el editor solo mientras hay una sesión abierta: cada apertura
  // parte de cero sin tener que reiniciar el estado a mano.

  const escena = borrador ?? hist.presente;
  const nombreCapaDefecto = (n: number): string => t('drawing.layer.default', { n });

  // Píxeles de pantalla por unidad de escena: el tamaño de las asas y la
  // tolerancia del puntero se miden en pantalla, no en unidades del dibujo.
  useLayoutEffect(() => {
    const el = lienzoRef.current;
    if (!open || !el) return;
    const medir = (): void => setEscalaPx(el.clientWidth / escena.ancho || 1);
    medir();
    if (typeof ResizeObserver === 'undefined') return;
    const ro = new ResizeObserver(medir);
    ro.observe(el);
    return () => ro.disconnect();
  }, [open, escena.ancho]);

  const svgHtml = useMemo(
    () => aSvg(escena, { responsivo: true, marcas: false, etiqueta: t('drawing.title') }),
    [escena, t],
  );

  /* ----- nombres accesibles ----- */

  const rotuloTipo = (n: Nodo): string => {
    switch (n.tipo) {
      case 'forma':
        return t(CLAVE_FORMA[n.forma] ?? 'slides.shape.rect');
      case 'texto':
        return t('drawing.tool.text');
      case 'linea':
        return n.flechaFin || n.flechaIni ? t('slides.shape.arrow') : t('slides.shape.line');
      case 'conector':
        return t('drawing.node.connector');
      case 'grupo':
        return t('slides.group');
      case 'imagen':
        return t('drawing.node.image');
      case 'trazo':
        return t('drawing.node.stroke');
    }
  };
  const nombreDe = (n: Nodo): string => n.nombre ?? rotuloTipo(n);

  /** Rótulo del tipo de la herramienta («Rectángulo»): la base del nombre de un objeto nuevo. */
  const rotuloHerr = (h: HerramientaObjeto): string => {
    const clave = HERRAMIENTAS.find((x) => x.id === h)?.clave ?? 'drawing.tool.text';
    return t(clave);
  };

  const anunciar = (msg: string): void => setAviso(msg);

  /* ----- aplicar cambios al historial ----- */

  const aplicar = (siguiente: Escena, msg?: string): void => {
    setHist((h) => confirmar(h, siguiente));
    setBorrador(null);
    if (msg) anunciar(msg);
  };

  const idsSeleccion = sel.filter((id) => buscar(escena, id) !== undefined);
  const unico = idsSeleccion.length === 1 ? buscar(escena, idsSeleccion[0]!) : undefined;

  /* ----- puntero ----- */

  const punto = (e: PointerEvent | MouseEvent): Punto => {
    const r = capaRef.current!.getBoundingClientRect();
    return {
      x: ((e.clientX - r.left) * escena.ancho) / (r.width || 1),
      y: ((e.clientY - r.top) * escena.alto) / (r.height || 1),
    };
  };
  const tol = TOLERANCIA_PX / escalaPx;

  const cajaSeleccion = (id: string): Rect | undefined => cajaMundo(escena, id);

  /** La escena con un trazo nuevo (en curso o terminado) en la capa activa. */
  const conTrazo = (base: Escena, puntos: number[], id: string): Escena => {
    const capa = base.capas.find((c) => c.id === capaActiva && c.visible && !c.bloqueada);
    if (!capa) return base;
    const nodo: Nodo = {
      id,
      tipo: 'trazo',
      nombre: nombreConOrdinal(t('drawing.node.stroke'), base),
      puntos,
      estilo: { ...(lapiz.color ? { trazo: lapiz.color } : {}), grosor: lapiz.grosor },
    };
    return agregarNodo(base, capa.id, nodo);
  };

  /** Borra el trazo que haya bajo `p` (solo trazos a mano alzada) dentro del gesto en curso. */
  const borrarBajo = (p: Punto): void => {
    const a = arrRef.current;
    if (a?.tipo !== 'borrar') return;
    const g = golpear(a.actual, p.x, p.y, tol);
    const n = g ? buscar(a.actual, g.id)?.nodo : undefined;
    if (g && n?.tipo === 'trazo') {
      a.ids.push(nombreDe(n));
      a.actual = eliminarNodos(a.actual, [g.id]);
      setBorrador(a.actual);
    }
  };

  const alPulsar = (e: PointerEvent): void => {
    if (e.button !== 0 && e.pointerType === 'mouse') return;
    const p = punto(e);
    lienzoRef.current?.focus({ preventScroll: true });
    (e.currentTarget as Element).setPointerCapture?.(e.pointerId);
    const base = hist.presente;
    // Un solo dedo/lápiz/ratón dibuja: los demás contactos (la palma, un segundo dedo) se ignoran.
    if (!e.isPrimary) return;
    // El extremo borrador de un lápiz (botón 32) borra aunque la herramienta sea otra.
    const borrando = herr === 'borrador' || (e.pointerType === 'pen' && (e.buttons & 32) !== 0);
    if (borrando) {
      arrRef.current = { tipo: 'borrar', base, actual: base, ids: [] };
      borrarBajo(p);
      return;
    }
    if (herr === 'lapiz') {
      arrRef.current = { tipo: 'lapiz', base, id: nuevoId(base), puntos: [p.x, p.y] };
      setBorrador(conTrazo(base, [p.x, p.y], nuevoId(base)));
      return;
    }
    if (herr !== 'seleccionar') {
      arrRef.current = { tipo: 'crear', desde: p, base, herramienta: herr };
      return;
    }
    // Asa de redimensión del objeto único seleccionado.
    const asaPulsada = (e.target as Element).getAttribute?.('data-asa') as Asa | null;
    if (asaPulsada && unico) {
      const caja = cajaSeleccion(unico.nodo.id);
      if (caja) {
        arrRef.current = { tipo: 'asa', asa: asaPulsada, base, id: unico.nodo.id, caja };
        return;
      }
    }
    const golpe = golpear(base, p.x, p.y, tol);
    if (golpe) {
      const aditivo = e.shiftKey || e.ctrlKey || e.metaKey;
      let nueva = sel;
      if (aditivo)
        nueva = sel.includes(golpe.id) ? sel.filter((i) => i !== golpe.id) : [...sel, golpe.id];
      else if (!sel.includes(golpe.id)) nueva = [golpe.id];
      setSel(nueva);
      setCapaActiva(golpe.capaId);
      const nodo = buscar(base, golpe.id)?.nodo;
      if (nodo && nueva.includes(golpe.id))
        anunciar(t('drawing.announce.selected', { name: nombreDe(nodo) }));
      arrRef.current = { tipo: 'mover', desde: p, base, ids: nueva };
      return;
    }
    const aditivo = e.shiftKey || e.ctrlKey || e.metaKey;
    arrRef.current = { tipo: 'marco', desde: p, base, aditivo, previa: aditivo ? sel : [] };
    if (!aditivo) setSel([]);
  };

  const alMover = (e: PointerEvent): void => {
    const a = arrRef.current;
    if (!a) return;
    const p = punto(e);
    switch (a.tipo) {
      case 'lapiz': {
        // Los eventos fusionados dan todos los puntos del fotograma (lápiz y táctil a 240 Hz).
        const evs = e.getCoalescedEvents?.() ?? [];
        for (const ev of evs.length > 0 ? evs : [e]) {
          const q = punto(ev);
          a.puntos.push(q.x, q.y);
        }
        setBorrador(conTrazo(a.base, a.puntos.slice(), a.id));
        return;
      }
      case 'borrar':
        borrarBajo(p);
        return;
      case 'mover': {
        const dx = Math.round(p.x - a.desde.x);
        const dy = Math.round(p.y - a.desde.y);
        setBorrador(moverNodos(a.base, a.ids, dx, dy));
        return;
      }
      case 'crear': {
        const nodo = nodoNuevo(
          a.herramienta,
          a.base,
          rotuloHerr(a.herramienta),
          a.desde.x,
          a.desde.y,
          p.x,
          p.y,
        );
        const capa =
          a.base.capas.find((c) => c.id === capaActiva) ?? a.base.capas[a.base.capas.length - 1]!;
        setBorrador(agregarNodo(a.base, capa.id, nodo));
        return;
      }
      case 'asa':
        setBorrador(redimensionar(a.base, a.id, cajaConAsa(a.caja, a.asa, p)));
        return;
      case 'marco': {
        const r = {
          x: Math.min(a.desde.x, p.x),
          y: Math.min(a.desde.y, p.y),
          w: Math.abs(p.x - a.desde.x),
          h: Math.abs(p.y - a.desde.y),
        };
        setMarco(r);
        const dentro = nodosEnRect(a.base, r, 'toca');
        setSel([...new Set([...a.previa, ...dentro])]);
      }
    }
  };

  const alSoltar = (e: PointerEvent): void => {
    const a = arrRef.current;
    arrRef.current = null;
    (e.currentTarget as Element).releasePointerCapture?.(e.pointerId);
    setMarco(null);
    if (!a) return;
    if (a.tipo === 'crear') {
      const p = punto(e);
      const capa =
        a.base.capas.find((c) => c.id === capaActiva) ?? a.base.capas[a.base.capas.length - 1]!;
      const nodo = nodoNuevo(
        a.herramienta,
        a.base,
        rotuloHerr(a.herramienta),
        a.desde.x,
        a.desde.y,
        p.x,
        p.y,
      );
      aplicar(
        agregarNodo(a.base, capa.id, nodo),
        t('drawing.announce.added', { name: nombreDe(nodo) }),
      );
      setSel([nodo.id]);
      setHerr('seleccionar');
      return;
    }
    if (a.tipo === 'marco') return;
    if (a.tipo === 'lapiz') {
      const puntos = limpiar(a.puntos);
      aplicar(
        conTrazo(a.base, puntos, a.id),
        t('drawing.announce.added', { name: t('drawing.node.stroke') }),
      );
      return;
    }
    if (a.tipo === 'borrar') {
      if (a.ids.length > 0)
        aplicar(a.actual, t('drawing.announce.deleted', { name: a.ids.join(', ') }));
      else setBorrador(null);
      return;
    }
    if (borrador) aplicar(borrador);
  };

  const alCancelarPuntero = (): void => {
    arrRef.current = null;
    setBorrador(null);
    setMarco(null);
  };

  /* ----- acciones ----- */

  const colocarEnCentro = (h: HerramientaObjeto): void => {
    const base = hist.presente;
    const capa = base.capas.find((c) => c.id === capaActiva) ?? base.capas[base.capas.length - 1]!;
    const nodo = nodoNuevo(h, base, rotuloHerr(h), base.ancho / 2, base.alto / 2);
    aplicar(
      agregarNodo(base, capa.id, nodo),
      t('drawing.announce.added', { name: nombreDe(nodo) }),
    );
    setSel([nodo.id]);
    setHerr('seleccionar');
  };

  const borrar = (): void => {
    if (idsSeleccion.length === 0) return;
    const nombres = idsSeleccion
      .map((id) => buscar(hist.presente, id)?.nodo)
      .filter((n): n is Nodo => n !== undefined)
      .map(nombreDe);
    aplicar(
      eliminarNodos(hist.presente, idsSeleccion),
      t('drawing.announce.deleted', { name: nombres.join(', ') }),
    );
    setSel([]);
  };

  const mover = (dx: number, dy: number): void => {
    if (idsSeleccion.length === 0) return;
    const e = moverNodos(hist.presente, idsSeleccion, dx, dy);
    const caja = unico ? cajaMundo(e, unico.nodo.id) : undefined;
    aplicar(
      e,
      unico && caja
        ? t('drawing.announce.moved', {
            name: nombreDe(unico.nodo),
            x: Math.round(caja.x),
            y: Math.round(caja.y),
          })
        : undefined,
    );
  };

  /** Todos los nodos de primer nivel de las capas visibles, de abajo arriba (orden de recorrido con «[» y «]»). */
  const primerNivel = (): string[] =>
    escena.capas.filter((c) => c.visible && !c.bloqueada).flatMap((c) => c.nodos.map((n) => n.id));

  const recorrer = (paso: 1 | -1): void => {
    const ids = primerNivel();
    if (ids.length === 0) return;
    const actual =
      idsSeleccion.length > 0 ? ids.indexOf(idsSeleccion[idsSeleccion.length - 1]!) : -1;
    const i = (actual + paso + ids.length) % ids.length;
    const id = ids[i]!;
    setSel([id]);
    const v = buscar(escena, id);
    if (v) {
      setCapaActiva(v.capa.id);
      anunciar(t('drawing.announce.selected', { name: nombreDe(v.nodo) }));
    }
  };

  /** Diagrama SmartArt de ejemplo (cuatro elementos editables como cualquier forma) en una capa nueva. */
  const insertarDiagrama = (tipo: TipoDiagrama): void => {
    const elementos = Array.from({ length: 4 }, (_, i) => ({
      text: t('drawing.diagram.item', { n: i + 1 }),
    }));
    const { escena: nueva, ids } = anadirDiagrama(
      hist.presente,
      tipo,
      elementos,
      t(`drawing.diagram.${tipo}`),
    );
    if (ids.length === 0) return;
    aplicar(nueva, t('drawing.announce.added', { name: t(`drawing.diagram.${tipo}`) }));
    setCapaActiva(nueva.capas[nueva.capas.length - 1]!.id);
    setSel(ids);
  };

  const conectar = (): void => {
    if (idsSeleccion.length !== 2) return;
    const [a, b] = idsSeleccion as [string, string];
    const base = hist.presente;
    const capa = buscar(base, a)?.capa ?? base.capas[base.capas.length - 1]!;
    const nodo: Nodo = {
      id: nuevoId(base),
      tipo: 'conector',
      nombre: nombreConOrdinal(t('drawing.node.connector'), base),
      trazado: 'straight',
      desde: { nodo: a },
      hasta: { nodo: b },
      flechaFin: true,
    };
    aplicar(
      agregarNodo(base, capa.id, nodo),
      t('drawing.announce.added', { name: nombreDe(nodo) }),
    );
    setSel([nodo.id]);
  };

  const agruparSel = (): void => {
    if (idsSeleccion.length < 2) return;
    const id = nuevoId(hist.presente, 'g');
    const e = agrupar(hist.presente, idsSeleccion, id);
    if (e === hist.presente) return;
    const nombre = nombreConOrdinal(t('slides.group'), e);
    aplicar(
      actualizarNodo(e, id, (n) => ({ ...n, nombre })),
      t('drawing.announce.added', { name: nombre }),
    );
    setSel([id]);
  };

  const desagruparSel = (): void => {
    if (!unico || unico.nodo.tipo !== 'grupo') return;
    const hijos = unico.nodo.hijos.map((h) => h.id);
    aplicar(desagrupar(hist.presente, unico.nodo.id));
    setSel(hijos);
  };

  const deshacerPaso = (): void => {
    setHist(deshacer);
    setBorrador(null);
  };
  const rehacerPaso = (): void => {
    setHist(rehacer);
    setBorrador(null);
  };

  /* Escape. El `Modal` atrapa Escape en `document` (fase de captura) y cierra el
   * diálogo, así que un manejador del lienzo no llegaría nunca. Se atiende ANTES, en
   * `window` (que va primero en la captura): si hay una herramienta armada o una
   * selección, Escape las quita; si no, deja pasar y cierra el diálogo (con la
   * pregunta de descartar si hay cambios). */
  const escRef = useUltimo((e: KeyboardEvent): boolean => {
    if (!lienzoRef.current?.contains(e.target as Node)) return false;
    if (herr !== 'seleccionar') {
      setHerr('seleccionar');
      return true;
    }
    if (idsSeleccion.length > 0) {
      setSel([]);
      anunciar(t('drawing.announce.cleared'));
      return true;
    }
    return false;
  });

  useLayoutEffect(() => {
    if (!open) return;
    const alEscape = (e: KeyboardEvent): void => {
      if (e.key === 'Escape' && escRef.current(e)) {
        e.preventDefault();
        e.stopPropagation();
      }
    };
    window.addEventListener('keydown', alEscape, true);
    return () => window.removeEventListener('keydown', alEscape, true);
  }, [open, escRef]);

  /* Cerrar sin aceptar: con cambios se pregunta (Escape o un clic fuera no deben
   * tirar una sesión de dibujo). */
  const { confirmar: preguntar, dialogo } = useConfirm();
  const cancelar = async (): Promise<void> => {
    if (hist.pasado.length > 0) {
      const descartar = await preguntar({
        titulo: t('drawing.discard.title'),
        mensaje: t('drawing.discard.message'),
        confirmLabel: t('ai.result.discard'),
        tone: 'danger',
      });
      if (!descartar) return;
    }
    onCancelar();
  };

  const alTeclear = (e: KeyboardEvent): void => {
    const objetivo = e.target as HTMLElement;
    // Los campos de texto del panel tienen sus propias teclas.
    if (objetivo !== e.currentTarget) return;
    const paso = e.shiftKey ? 10 : 1;
    const mod = e.ctrlKey || e.metaKey;
    if (mod && e.key.toLowerCase() === 'z') {
      e.preventDefault();
      if (e.shiftKey) rehacerPaso();
      else deshacerPaso();
      return;
    }
    if (mod && e.key.toLowerCase() === 'y') {
      e.preventDefault();
      rehacerPaso();
      return;
    }
    if (mod && e.key.toLowerCase() === 'g') {
      e.preventDefault();
      if (e.shiftKey) desagruparSel();
      else agruparSel();
      return;
    }
    switch (e.key) {
      case 'ArrowLeft':
        e.preventDefault();
        mover(-paso, 0);
        break;
      case 'ArrowRight':
        e.preventDefault();
        mover(paso, 0);
        break;
      case 'ArrowUp':
        e.preventDefault();
        mover(0, -paso);
        break;
      case 'ArrowDown':
        e.preventDefault();
        mover(0, paso);
        break;
      case 'Delete':
      case 'Backspace':
        e.preventDefault();
        borrar();
        break;
      case '[':
        e.preventDefault();
        recorrer(-1);
        break;
      case ']':
        e.preventDefault();
        recorrer(1);
        break;
      case 'Enter':
        if (herr !== 'seleccionar' && herr !== 'lapiz' && herr !== 'borrador') {
          e.preventDefault();
          colocarEnCentro(herr);
        }
        break;
    }
  };

  /* ----- capas ----- */

  const nuevaCapa = (): void => {
    const base = hist.presente;
    const id = nuevoId(base, 'c');
    aplicar(agregarCapa(base, { id, nombre: nombreCapaDefecto(base.capas.length + 1) }));
    setCapaActiva(id);
  };

  /* ----- propiedades ----- */

  const parche = (id: string, fn: (n: Nodo) => Nodo): void =>
    aplicar(actualizarNodo(hist.presente, id, fn));

  const num = (v: string, def: number): number => {
    const n = Number(v);
    return Number.isFinite(n) ? n : def;
  };

  const estiloColor = (id: string, campo: 'relleno' | 'trazo', valor: string | undefined): void =>
    parche(id, (n) => {
      const { [campo]: _quitado, ...resto } = n.estilo ?? {};
      const estilo = valor === undefined ? resto : { ...resto, [campo]: valor };
      const { estilo: _e, ...sin } = n;
      return Object.keys(estilo).length > 0 ? { ...sin, estilo } : sin;
    });

  const panelNodo = (v: NonNullable<typeof unico>): JSX.Element => {
    const n = v.nodo;
    const id = n.id;
    const caja =
      n.tipo === 'forma' || n.tipo === 'texto' || n.tipo === 'imagen' || n.tipo === 'grupo'
        ? n
        : undefined;
    const cerrada = n.tipo === 'forma' || n.tipo === 'grupo' || n.tipo === 'texto';
    return (
      <div class="flex flex-col gap-2">
        <FieldGroup label={t('drawing.prop.name')} htmlFor="dib-nombre">
          <Input
            id="dib-nombre"
            size="sm"
            value={n.nombre ?? ''}
            placeholder={rotuloTipo(n)}
            onChange={(e) => {
              const valor = e.currentTarget.value.trim();
              parche(id, (x) => {
                const { nombre: _n, ...resto } = x;
                return valor === '' ? resto : { ...resto, nombre: valor };
              });
            }}
          />
        </FieldGroup>
        {(n.tipo === 'forma' || n.tipo === 'texto') && (
          <FieldGroup label={t('drawing.prop.text')} htmlFor="dib-texto">
            <Input
              id="dib-texto"
              size="sm"
              value={n.texto ?? ''}
              onChange={(e) => {
                const valor = e.currentTarget.value;
                parche(id, (x) =>
                  x.tipo === 'forma'
                    ? valor === ''
                      ? (({ texto: _t, ...r }) => r)(x)
                      : { ...x, texto: valor }
                    : x.tipo === 'texto'
                      ? { ...x, texto: valor }
                      : x,
                );
              }}
            />
          </FieldGroup>
        )}
        {caja && (
          <div class="grid grid-cols-2 gap-2">
            {(
              [
                ['x', 'drawing.prop.x'],
                ['y', 'drawing.prop.y'],
                ['w', 'drawing.prop.w'],
                ['h', 'drawing.prop.h'],
              ] as const
            ).map(([campo, clave]) => (
              <FieldGroup key={campo} label={t(clave)} htmlFor={`dib-${campo}`}>
                <Input
                  id={`dib-${campo}`}
                  size="sm"
                  type="number"
                  min={campo === 'w' || campo === 'h' ? 1 : undefined}
                  value={String(Math.round(caja[campo]))}
                  disabled={n.tipo === 'grupo' && (campo === 'w' || campo === 'h')}
                  onChange={(e) => {
                    const valor = num(e.currentTarget.value, caja[campo]);
                    aplicar(
                      campo === 'w' || campo === 'h'
                        ? redimensionar(hist.presente, id, {
                            x: caja.x,
                            y: caja.y,
                            w: campo === 'w' ? valor : caja.w,
                            h: campo === 'h' ? valor : caja.h,
                          })
                        : actualizarNodo(hist.presente, id, (x) => ({ ...x, [campo]: valor })),
                    );
                  }}
                />
              </FieldGroup>
            ))}
            <FieldGroup label={t('drawing.prop.rot')} htmlFor="dib-rot">
              <Input
                id="dib-rot"
                size="sm"
                type="number"
                value={String(caja.rot ?? 0)}
                onChange={(e) => {
                  const g = ((num(e.currentTarget.value, 0) % 360) + 360) % 360;
                  parche(id, (x) => {
                    const { rot: _r, ...resto } = x as Nodo & { rot?: number };
                    return (g === 0 ? resto : { ...resto, rot: g }) as Nodo;
                  });
                }}
              />
            </FieldGroup>
          </div>
        )}
        {n.tipo === 'conector' && (
          <>
            <FieldGroup label={t('connector.type')} htmlFor="dib-trazado">
              <Select
                id="dib-trazado"
                size="sm"
                value={n.trazado}
                onChange={(e) => {
                  const valor = e.currentTarget.value as 'straight' | 'elbow' | 'curved';
                  parche(id, (x) => (x.tipo === 'conector' ? { ...x, trazado: valor } : x));
                }}
              >
                {TRAZADOS.map((o) => (
                  <option key={o.id} value={o.id}>
                    {t(o.clave)}
                  </option>
                ))}
              </Select>
            </FieldGroup>
          </>
        )}
        {(n.tipo === 'conector' || n.tipo === 'linea') && (
          <div class="flex flex-col gap-1">
            <Checkbox
              label={t('connector.arrowStart')}
              checked={n.flechaIni === true}
              onChange={(e) => {
                const on = e.currentTarget.checked;
                parche(id, (x) => {
                  if (x.tipo !== 'conector' && x.tipo !== 'linea') return x;
                  const { flechaIni: _f, ...resto } = x;
                  return on ? { ...resto, flechaIni: true } : resto;
                });
              }}
            />
            <Checkbox
              label={t('connector.arrowEnd')}
              checked={n.flechaFin === true}
              onChange={(e) => {
                const on = e.currentTarget.checked;
                parche(id, (x) => {
                  if (x.tipo !== 'conector' && x.tipo !== 'linea') return x;
                  const { flechaFin: _f, ...resto } = x;
                  return on ? { ...resto, flechaFin: true } : resto;
                });
              }}
            />
          </div>
        )}
        {n.tipo !== 'imagen' && n.tipo !== 'grupo' && (
          <div class="grid grid-cols-2 items-end gap-2">
            {cerrada && n.tipo !== 'texto' && (
              <ColorCampo
                etiqueta={t('shape.fill')}
                auto={t('drawing.prop.auto')}
                valor={n.estilo?.relleno}
                onCambio={(v) => estiloColor(id, 'relleno', v)}
              />
            )}
            {n.tipo === 'texto' ? (
              <ColorCampo
                etiqueta={t('drawing.prop.textColor')}
                auto={t('drawing.prop.auto')}
                valor={n.estilo?.relleno}
                onCambio={(v) => estiloColor(id, 'relleno', v)}
              />
            ) : (
              <ColorCampo
                etiqueta={t('shape.outline')}
                auto={t('drawing.prop.auto')}
                valor={n.estilo?.trazo}
                onCambio={(v) => estiloColor(id, 'trazo', v)}
              />
            )}
            {n.tipo !== 'texto' && (
              <FieldGroup label={t('drawing.prop.lineWidth')} htmlFor="dib-grosor">
                <Input
                  id="dib-grosor"
                  size="sm"
                  type="number"
                  min={0}
                  max={64}
                  step={0.5}
                  value={String(n.estilo?.grosor ?? 1.5)}
                  onChange={(e) => {
                    const g = Math.min(64, Math.max(0, num(e.currentTarget.value, 1.5)));
                    parche(id, (x) => ({ ...x, estilo: { ...x.estilo, grosor: g } }));
                  }}
                />
              </FieldGroup>
            )}
          </div>
        )}
      </div>
    );
  };

  const panelEscena = (): JSX.Element => (
    <div class="flex flex-col gap-2">
      <FieldGroup label={t('drawing.prop.canvasTitle')} htmlFor="dib-titulo">
        <Input
          id="dib-titulo"
          size="sm"
          value={hist.presente.titulo ?? ''}
          onChange={(e) => {
            const valor = e.currentTarget.value.trim();
            const { titulo: _t, ...resto } = hist.presente;
            aplicar(valor === '' ? resto : { ...resto, titulo: valor });
          }}
        />
      </FieldGroup>
      <FieldGroup label={t('drawing.prop.canvasDesc')} htmlFor="dib-desc">
        <Input
          id="dib-desc"
          size="sm"
          value={hist.presente.descripcion ?? ''}
          onChange={(e) => {
            const valor = e.currentTarget.value.trim();
            const { descripcion: _d, ...resto } = hist.presente;
            aplicar(valor === '' ? resto : { ...resto, descripcion: valor });
          }}
        />
      </FieldGroup>
      <div class="grid grid-cols-2 gap-2">
        <FieldGroup label={t('drawing.prop.canvasW')} htmlFor="dib-ancho">
          <Input
            id="dib-ancho"
            size="sm"
            type="number"
            min={100}
            max={4000}
            value={String(hist.presente.ancho)}
            onChange={(e) =>
              aplicar({
                ...hist.presente,
                ancho: Math.min(
                  4000,
                  Math.max(100, num(e.currentTarget.value, hist.presente.ancho)),
                ),
              })
            }
          />
        </FieldGroup>
        <FieldGroup label={t('drawing.prop.canvasH')} htmlFor="dib-alto">
          <Input
            id="dib-alto"
            size="sm"
            type="number"
            min={100}
            max={4000}
            value={String(hist.presente.alto)}
            onChange={(e) =>
              aplicar({
                ...hist.presente,
                alto: Math.min(4000, Math.max(100, num(e.currentTarget.value, hist.presente.alto))),
              })
            }
          />
        </FieldGroup>
      </div>
    </div>
  );

  /* ----- vista ----- */

  const BTN =
    'inline-flex h-8 min-w-8 items-center justify-center gap-1 rounded-sm px-1.5 text-xs text-text-muted ' +
    'hover:bg-bg-subtle hover:text-text disabled:pointer-events-none disabled:opacity-40 sytue-focus-ring';

  const caja1 = unico && unico.nodo.tipo !== 'conector' ? cajaSeleccion(unico.nodo.id) : undefined;
  const conAsas =
    unico &&
    caja1 &&
    (unico.nodo.tipo === 'forma' || unico.nodo.tipo === 'texto' || unico.nodo.tipo === 'imagen') &&
    (unico.nodo.rot ?? 0) === 0 &&
    unico.camino.length === 0
      ? caja1
      : undefined;
  const asaLado = LADO_ASA_PX / escalaPx;

  const nodosActiva = escena.capas.find((c) => c.id === capaActiva)?.nodos ?? [];

  return (
    <>
      <Modal
        open={open}
        onClose={() => void cancelar()}
        title={t('drawing.edit')}
        size="xl"
        footer={
          <>
            {/* Las dos exportaciones a la izquierda en escritorio (`md:mr-auto` en la
                segunda); en el teléfono, una pareja encima de Cancelar | Aceptar. */}
            <Button variant="secondary" size="sm" onClick={() => descargar(hist.presente, 'svg')}>
              {t('drawing.export.svg')}
            </Button>
            <Button
              variant="secondary"
              size="sm"
              class="md:mr-auto"
              onClick={() => descargar(hist.presente, 'pdf')}
            >
              {t('drawing.export.pdf')}
            </Button>
            <Button variant="secondary" size="sm" onClick={() => void cancelar()}>
              {t('action.cancel')}
            </Button>
            <Button size="sm" onClick={() => onGuardar(hist.presente)}>
              {aceptar}
            </Button>
          </>
        }
      >
        <div class="flex h-[min(68dvh,720px)] min-h-[360px] flex-col gap-2">
          {/* Barra de herramientas */}
          <Toolbar
            aria-label={t('drawing.tools')}
            class="border-border flex flex-wrap items-center gap-0.5 border-b pb-2"
          >
            <button
              type="button"
              class={BTN}
              aria-label={t('action.undo')}
              title={t('action.undo')}
              disabled={hist.pasado.length === 0}
              onClick={deshacerPaso}
            >
              <EditorIcon name="undo" size="md" />
            </button>
            <button
              type="button"
              class={BTN}
              aria-label={t('action.redo')}
              title={t('action.redo')}
              disabled={hist.futuro.length === 0}
              onClick={rehacerPaso}
            >
              <EditorIcon name="redo" size="md" />
            </button>
            <span class="bg-border mx-1 h-5 w-px" aria-hidden="true" />
            {HERRAMIENTAS.map((h) => (
              <button
                key={h.id}
                type="button"
                class={`${BTN} ${herr === h.id ? 'bg-selected text-text' : ''}`}
                aria-label={t(h.clave)}
                title={t(h.clave)}
                aria-pressed={herr === h.id}
                onClick={() => setHerr(h.id)}
              >
                <EditorIcon name={h.icono} size="md" />
              </button>
            ))}
            <span class="bg-border mx-1 h-5 w-px" aria-hidden="true" />
            <button
              type="button"
              class={BTN}
              aria-label={t('drawing.connect')}
              title={t('drawing.connect')}
              disabled={idsSeleccion.length !== 2}
              onClick={conectar}
            >
              <EditorIcon name="connStraight" size="md" />
            </button>
            <button
              type="button"
              class={BTN}
              disabled={idsSeleccion.length < 2}
              onClick={agruparSel}
            >
              {t('slides.group')}
            </button>
            <button
              type="button"
              class={BTN}
              disabled={unico?.nodo.tipo !== 'grupo'}
              onClick={desagruparSel}
            >
              {t('slides.ungroup')}
            </button>
            <span class="bg-border mx-1 h-5 w-px" aria-hidden="true" />
            <Select
              size="sm"
              class="w-32"
              aria-label={t('group.arrange')}
              value=""
              disabled={idsSeleccion.length === 0}
              onChange={(e) => {
                const op = e.currentTarget.value as ZOrderOp | '';
                e.currentTarget.value = '';
                if (op) aplicar(ordenarZ(hist.presente, idsSeleccion, op));
              }}
            >
              <option value="">{t('group.arrange')}</option>
              {ORDENES.map((o) => (
                <option key={o.id} value={o.id}>
                  {t(o.clave)}
                </option>
              ))}
            </Select>
            <Select
              size="sm"
              class="w-32"
              aria-label={t('slides.arrange.align')}
              value=""
              disabled={idsSeleccion.length === 0}
              onChange={(e) => {
                const v = e.currentTarget.value;
                e.currentTarget.value = '';
                if (!v) return;
                const [ref, borde] = v.split(':') as ['seleccion' | 'lienzo', AlignEdge];
                aplicar(alinearNodos(hist.presente, idsSeleccion, borde, ref));
              }}
            >
              <option value="">{t('slides.arrange.align')}</option>
              {ALINEACIONES.map((o) => (
                <option
                  key={o.id}
                  value={`${idsSeleccion.length > 1 ? 'seleccion' : 'lienzo'}:${o.id}`}
                >
                  {t(o.clave)}
                </option>
              ))}
            </Select>
            <Select
              size="sm"
              class="w-36"
              aria-label={t('slides.arrange.distribute')}
              value=""
              disabled={idsSeleccion.length < 3}
              onChange={(e) => {
                const eje = e.currentTarget.value as DistAxis | '';
                e.currentTarget.value = '';
                if (eje) aplicar(distribuirNodos(hist.presente, idsSeleccion, eje));
              }}
            >
              <option value="">{t('slides.arrange.distribute')}</option>
              <option value="h">{t('slides.distribute.h')}</option>
              <option value="v">{t('slides.distribute.v')}</option>
            </Select>
            <Select
              size="sm"
              class="w-36"
              aria-label={t('drawing.diagram')}
              value=""
              onChange={(e) => {
                const tipo = e.currentTarget.value as TipoDiagrama | '';
                e.currentTarget.value = '';
                if (tipo) insertarDiagrama(tipo);
              }}
            >
              <option value="">{t('drawing.diagram')}</option>
              {TIPOS_DIAGRAMA.map((d) => (
                <option key={d} value={d}>
                  {t(`drawing.diagram.${d}`)}
                </option>
              ))}
            </Select>
            <button
              type="button"
              class={BTN}
              aria-label={t('action.removeItem')}
              title={t('action.removeItem')}
              disabled={idsSeleccion.length === 0}
              onClick={borrar}
            >
              <EditorIcon name="trash" size="md" />
            </button>
          </Toolbar>

          <div class="flex min-h-0 flex-1 gap-3 max-md:flex-col">
            {/* Lienzo */}
            <div class="bg-bg-subtle flex min-h-0 min-w-0 flex-1 flex-col gap-1 overflow-auto rounded-md p-2">
              <p id="dib-pista" class="text-text-muted m-0 text-xs">
                {herr === 'seleccionar'
                  ? t('drawing.canvasHint')
                  : herr === 'lapiz' || herr === 'borrador'
                    ? t('drawing.penHint')
                    : t('drawing.drawHint')}
              </p>
              {/* El lienzo es un widget propio con su teclado (flechas, «[» y «]», Supr…): el rol
                  `application` de WAI-ARIA es el adecuado, y el linter solo lo toma por no interactivo. */}
              {/* oxlint-disable-next-line jsx-a11y/no-noninteractive-element-interactions */}
              <div
                ref={lienzoRef}
                role="application"
                aria-label={t('drawing.canvas')}
                aria-describedby="dib-pista"
                // oxlint-disable-next-line jsx-a11y/no-noninteractive-tabindex
                tabIndex={0}
                onKeyDown={alTeclear}
                class="bg-surface text-text border-border sytue-focus-ring relative mx-auto w-full max-w-[960px] touch-none border select-none"
                style={{ cursor: herr === 'seleccionar' ? 'default' : 'crosshair' }}
              >
                <div dangerouslySetInnerHTML={{ __html: svgHtml }} />
                <svg
                  ref={capaRef}
                  class="absolute inset-0 h-full w-full"
                  viewBox={`0 0 ${escena.ancho} ${escena.alto}`}
                  aria-hidden="true"
                  onPointerDown={alPulsar}
                  onPointerMove={alMover}
                  onPointerUp={alSoltar}
                  onPointerCancel={alCancelarPuntero}
                >
                  {idsSeleccion.map((id) => {
                    const c = cajaSeleccion(id);
                    return c ? (
                      <rect
                        key={id}
                        x={c.x}
                        y={c.y}
                        width={c.w}
                        height={c.h}
                        class="stroke-primary fill-none"
                        stroke-width={1.5 / escalaPx}
                        stroke-dasharray={`${4 / escalaPx} ${3 / escalaPx}`}
                        pointer-events="none"
                      />
                    ) : null;
                  })}
                  {conAsas &&
                    ASAS.map((a) => {
                      const p = posAsa(conAsas, a);
                      return (
                        <rect
                          key={a}
                          data-asa={a}
                          x={p.x - asaLado / 2}
                          y={p.y - asaLado / 2}
                          width={asaLado}
                          height={asaLado}
                          class="fill-surface stroke-primary"
                          stroke-width={1.5 / escalaPx}
                          style={{ cursor: CURSOR_ASA[a] }}
                        />
                      );
                    })}
                  {marco && (
                    <rect
                      x={marco.x}
                      y={marco.y}
                      width={marco.w}
                      height={marco.h}
                      class="stroke-primary fill-primary/10"
                      stroke-width={1 / escalaPx}
                      pointer-events="none"
                    />
                  )}
                </svg>
              </div>
              <div class="sr-only" role="status" aria-live="polite">
                {aviso}
              </div>
            </div>

            {/* Panel lateral */}
            <div class="flex w-72 shrink-0 flex-col gap-3 overflow-y-auto max-md:w-full">
              <section aria-labelledby="dib-capas">
                <div class="mb-1 flex items-center justify-between">
                  <h3 id="dib-capas" class="m-0 text-sm font-semibold">
                    {t('drawing.layers')}
                  </h3>
                  <button type="button" class={BTN} onClick={nuevaCapa}>
                    <EditorIcon name="plus" size="sm" /> {t('drawing.layer.add')}
                  </button>
                </div>
                <ul class="m-0 flex list-none flex-col gap-0.5 p-0">
                  {[...escena.capas].reverse().map((c, ri) => {
                    const i = escena.capas.length - 1 - ri;
                    return (
                      <li key={c.id} class="flex items-center gap-1">
                        <button
                          type="button"
                          class={`${BTN} ${c.id === capaActiva ? 'bg-selected text-text' : ''}`}
                          aria-label={t('drawing.layer.active', { name: c.nombre })}
                          aria-pressed={c.id === capaActiva}
                          onClick={() => setCapaActiva(c.id)}
                        >
                          <EditorIcon name="check" size="sm" />
                        </button>
                        <Input
                          size="sm"
                          class="min-w-0 flex-1"
                          aria-label={t('drawing.layer.name', { n: i + 1 })}
                          value={c.nombre}
                          onChange={(e) => {
                            const v = e.currentTarget.value.trim();
                            aplicar(
                              renombrarCapa(
                                hist.presente,
                                c.id,
                                v === '' ? nombreCapaDefecto(i + 1) : v,
                              ),
                            );
                          }}
                        />
                        <button
                          type="button"
                          class={`${BTN} ${c.visible ? 'text-text' : ''}`}
                          aria-label={t('drawing.layer.show', { name: c.nombre })}
                          aria-pressed={c.visible}
                          onClick={() =>
                            aplicar(ajustarCapa(hist.presente, c.id, { visible: !c.visible }))
                          }
                        >
                          <EditorIcon name="eye" size="sm" />
                        </button>
                        <button
                          type="button"
                          class={`${BTN} ${c.bloqueada ? 'bg-selected text-text' : ''}`}
                          aria-label={t('drawing.layer.lock', { name: c.nombre })}
                          aria-pressed={c.bloqueada}
                          onClick={() =>
                            aplicar(ajustarCapa(hist.presente, c.id, { bloqueada: !c.bloqueada }))
                          }
                        >
                          <EditorIcon name="lock" size="sm" />
                        </button>
                        <button
                          type="button"
                          class={BTN}
                          aria-label={t('drawing.layer.up', { name: c.nombre })}
                          disabled={ri === 0}
                          onClick={() => aplicar(ordenarCapa(hist.presente, c.id, 'forward'))}
                        >
                          <EditorIcon name="chevronUp" size="sm" />
                        </button>
                        <button
                          type="button"
                          class={BTN}
                          aria-label={t('drawing.layer.down', { name: c.nombre })}
                          disabled={ri === escena.capas.length - 1}
                          onClick={() => aplicar(ordenarCapa(hist.presente, c.id, 'backward'))}
                        >
                          <EditorIcon name="chevronDown" size="sm" />
                        </button>
                        <button
                          type="button"
                          class={BTN}
                          aria-label={t('drawing.layer.delete', { name: c.nombre })}
                          disabled={escena.capas.length < 2}
                          onClick={() => {
                            aplicar(eliminarCapa(hist.presente, c.id));
                            setSel([]);
                            if (c.id === capaActiva)
                              setCapaActiva(hist.presente.capas.find((x) => x.id !== c.id)!.id);
                          }}
                        >
                          <EditorIcon name="close" size="sm" />
                        </button>
                      </li>
                    );
                  })}
                </ul>
              </section>

              <section aria-labelledby="dib-objetos">
                <h3 id="dib-objetos" class="m-0 mb-1 text-sm font-semibold">
                  {t('drawing.objects')}
                </h3>
                {nodosActiva.length === 0 ? (
                  <p class="text-text-muted m-0 text-xs">{t('drawing.empty')}</p>
                ) : (
                  <ul class="m-0 flex max-h-40 list-none flex-col gap-0.5 overflow-y-auto p-0">
                    {[...nodosActiva].reverse().map((n) => (
                      <li key={n.id}>
                        <button
                          type="button"
                          class={`${BTN} w-full justify-start ${idsSeleccion.includes(n.id) ? 'bg-selected text-text' : ''}`}
                          aria-pressed={idsSeleccion.includes(n.id)}
                          onClick={(e) => {
                            setSel(
                              e.shiftKey || e.ctrlKey || e.metaKey
                                ? idsSeleccion.includes(n.id)
                                  ? idsSeleccion.filter((i) => i !== n.id)
                                  : [...idsSeleccion, n.id]
                                : [n.id],
                            );
                            anunciar(t('drawing.announce.selected', { name: nombreDe(n) }));
                          }}
                        >
                          {nombreDe(n)}
                        </button>
                      </li>
                    ))}
                  </ul>
                )}
              </section>

              <section aria-labelledby="dib-props">
                <h3 id="dib-props" class="m-0 mb-1 text-sm font-semibold">
                  {t('drawing.properties')}
                </h3>
                {herr === 'lapiz' ? (
                  <div class="grid grid-cols-2 items-end gap-2">
                    <ColorCampo
                      etiqueta={t('drawing.prop.penColor')}
                      auto={t('drawing.prop.auto')}
                      valor={lapiz.color}
                      onCambio={(v) =>
                        setLapiz(({ color: _c, ...r }) =>
                          v === undefined ? r : { ...r, color: v },
                        )
                      }
                    />
                    <FieldGroup label={t('drawing.prop.lineWidth')} htmlFor="dib-lapiz-grosor">
                      <Input
                        id="dib-lapiz-grosor"
                        size="sm"
                        type="number"
                        min={0.5}
                        max={64}
                        step={0.5}
                        value={String(lapiz.grosor)}
                        onChange={(e) =>
                          setLapiz((l) => ({
                            ...l,
                            grosor: Math.min(
                              64,
                              Math.max(0.5, Number(e.currentTarget.value) || l.grosor),
                            ),
                          }))
                        }
                      />
                    </FieldGroup>
                  </div>
                ) : unico ? (
                  panelNodo(unico)
                ) : (
                  panelEscena()
                )}
                {onComentar && herr !== 'lapiz' && (
                  <div class="border-border mt-3 border-t pt-2">
                    <ComentarDibujo
                      destino={unico ? { nodo: unico.nodo.id, nombre: nombreDe(unico.nodo) } : null}
                      nombre={unico ? nombreDe(unico.nodo) : ''}
                      pendienteDeGuardar={
                        unico ? buscar(inicial, unico.nodo.id) === undefined : false
                      }
                      ancho={hist.presente.ancho}
                      alto={hist.presente.alto}
                      onComentar={onComentar}
                    />
                  </div>
                )}
              </section>
            </div>
          </div>
        </div>
      </Modal>
      {dialogo}
    </>
  );
}

/** Un color con su botón «tinta del tema» (quita el color fijo). */
function ColorCampo({
  etiqueta,
  auto,
  valor,
  onCambio,
}: {
  etiqueta: string;
  auto: string;
  valor: string | undefined;
  onCambio: (v: string | undefined) => void;
}): JSX.Element {
  return (
    <div class="flex flex-col gap-1.5">
      <span class="text-text text-sm font-semibold">{etiqueta}</span>
      <div class="flex items-center gap-1">
        <input
          type="color"
          aria-label={etiqueta}
          class="border-border h-6 w-8 cursor-pointer rounded-sm border bg-transparent p-0"
          value={valor ?? '#000000'}
          onInput={(e) => onCambio(e.currentTarget.value)}
        />
        <button
          type="button"
          class="text-text-muted hover:bg-bg-subtle sytue-focus-ring h-6 rounded-sm px-1.5 text-xs disabled:opacity-40"
          aria-label={`${etiqueta}: ${auto}`}
          disabled={valor === undefined}
          onClick={() => onCambio(undefined)}
        >
          {auto}
        </button>
      </div>
    </div>
  );
}
