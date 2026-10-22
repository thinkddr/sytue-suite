/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

import {
  IDIOMAS_VISITANTE,
  type Estilo,
  type Id,
  type Idioma,
  type Layout,
  type Marco,
  type Medida,
  type Nodo,
  type Override,
  type Pagina,
  type Seccion,
  type SeoSitio,
  type Sitio,
  type Tema,
  type Textos,
  type TextosRicos,
  nuevoId,
} from '@sytue/pages-render';
import type { PuntoDeCorte, ReferenciaNodo } from './tipos.js';

type Contenedor = Seccion | (Nodo & { children: Nodo[] });

function esContenedor(n: Nodo | Seccion): n is Contenedor {
  return 'children' in n && Array.isArray(n.children);
}

/**
 * Busca un nodo por ID en todo el IR (secciones de páginas, header, footer y subárboles).
 */
export function encontrarNodo(ir: Sitio, nodeId: Id): ReferenciaNodo | null {
  // Buscar en header
  if (ir.header) {
    if (ir.header.id === nodeId) {
      return { nodo: ir.header, padre: null, index: -1, pagina: null };
    }
    const r = buscarEnHijos(ir.header, nodeId, null);
    if (r) return r;
  }

  // Buscar en footer
  if (ir.footer) {
    if (ir.footer.id === nodeId) {
      return { nodo: ir.footer, padre: null, index: -1, pagina: null };
    }
    const r = buscarEnHijos(ir.footer, nodeId, null);
    if (r) return r;
  }

  // Buscar en páginas
  for (const pag of ir.pages) {
    for (let sIdx = 0; sIdx < pag.sections.length; sIdx++) {
      const sec = pag.sections[sIdx]!;
      if (sec.id === nodeId) {
        return { nodo: sec, padre: null, index: sIdx, pagina: pag };
      }
      const r = buscarEnHijos(sec, nodeId, pag);
      if (r) return r;
    }
  }

  return null;
}

function buscarEnHijos(
  padre: Contenedor,
  nodeId: Id,
  pagina: Pagina | null,
): ReferenciaNodo | null {
  for (let i = 0; i < padre.children.length; i++) {
    const hijo = padre.children[i]!;
    if (hijo.id === nodeId) {
      return { nodo: hijo, padre, index: i, pagina };
    }
    if (esContenedor(hijo)) {
      const res = buscarEnHijos(hijo, nodeId, pagina);
      if (res) return res;
    }
  }
  return null;
}

/**
 * Mapea inmutablemente los nodos de un Sitio, sustituyendo el nodo objetivo si coincide.
 */
function transformarArbol(
  ir: Sitio,
  transformador: (nodo: Nodo | Seccion) => Nodo | Seccion | null,
): Sitio {
  const mapearNodo = (n: Nodo | Seccion): Nodo | Seccion | null => {
    const transformado = transformador(n);
    if (transformado === null) return null;
    if (transformado.type === 'section') {
      const nuevosHijos: Nodo[] = [];
      for (const h of transformado.children) {
        const mh = mapearNodo(h);
        if (mh !== null && mh.type !== 'section') {
          nuevosHijos.push(mh);
        }
      }
      return { ...transformado, children: nuevosHijos };
    }
    if (transformado.type === 'frame') {
      const nuevosHijos: Nodo[] = [];
      for (const h of transformado.children) {
        const mh = mapearNodo(h);
        if (mh !== null && mh.type !== 'section') {
          nuevosHijos.push(mh);
        }
      }
      return { ...transformado, children: nuevosHijos };
    }
    return transformado;
  };

  const nuevoHeader = ir.header ? (mapearNodo(ir.header) as Seccion | null) : null;
  const nuevoFooter = ir.footer ? (mapearNodo(ir.footer) as Seccion | null) : null;

  const nuevasPaginas = ir.pages.map((p) => {
    const nuevasSecciones: Seccion[] = [];
    for (const sec of p.sections) {
      const ms = mapearNodo(sec);
      if (ms !== null && ms.type === 'section') {
        nuevasSecciones.push(ms);
      }
    }
    return { ...p, sections: nuevasSecciones };
  });

  return {
    ...ir,
    header: nuevoHeader,
    footer: nuevoFooter,
    pages: nuevasPaginas,
  };
}

/**
 * Actualiza propiedades de un nodo de forma pura e inmutable.
 */
export function setPropiedadesNodo<T extends Nodo | Seccion>(
  ir: Sitio,
  nodeId: Id,
  parche: Partial<T>,
): Sitio {
  return transformarArbol(ir, (n) => {
    if (n.id === nodeId) {
      return { ...n, ...parche };
    }
    return n;
  });
}

/**
 * Inserta un nuevo nodo como hijo de un contenedor (sección o marco) en la posición indicada.
 */
export function insertarNodo(ir: Sitio, parentId: Id, index: number, nuevo: Nodo | Seccion): Sitio {
  return transformarArbol(ir, (n) => {
    if (n.id === parentId && esContenedor(n)) {
      const hijos = [...n.children];
      const posicion = Math.max(0, Math.min(index, hijos.length));
      hijos.splice(posicion, 0, nuevo as Nodo);
      return { ...n, children: hijos };
    }
    return n;
  });
}

/**
 * Elimina un nodo por su ID en cualquier nivel del IR.
 */
export function eliminarNodo(ir: Sitio, nodeId: Id): Sitio {
  return transformarArbol(ir, (n) => {
    if (n.id === nodeId) {
      return null;
    }
    return n;
  });
}

/**
 * Mueve un nodo existente a un nuevo contenedor en la posición dada.
 */
export function moverNodo(ir: Sitio, nodeId: Id, nuevoPadreId: Id, nuevoIndice: number): Sitio {
  const ref = encontrarNodo(ir, nodeId);
  if (!ref) return ir;

  // 1. Eliminar nodo de su ubicación original
  const sinNodo = eliminarNodo(ir, nodeId);

  // 2. Insertar en el nuevo destino
  return insertarNodo(sinNodo, nuevoPadreId, nuevoIndice, ref.nodo);
}

/**
 * Clona recursivamente un nodo asignando nuevos IDs a todo el subárbol.
 */
export function clonarNodoConNuevosIds(nodo: Nodo | Seccion): Nodo | Seccion {
  const nid = nuevoId();
  if (nodo.type === 'section') {
    return {
      ...nodo,
      id: nid,
      children: nodo.children.map((h) => clonarNodoConNuevosIds(h) as Nodo),
    };
  }
  if (nodo.type === 'frame') {
    return {
      ...nodo,
      id: nid,
      children: nodo.children.map((h) => clonarNodoConNuevosIds(h) as Nodo),
    };
  }
  return { ...nodo, id: nid };
}

/**
 * Duplica un nodo y lo coloca inmediatamente después del original en su contenedor.
 */
export function duplicarNodo(ir: Sitio, nodeId: Id): { ir: Sitio; nuevoId: Id } | null {
  const ref = encontrarNodo(ir, nodeId);
  if (!ref) return null;

  const clon = clonarNodoConNuevosIds(ref.nodo);

  if (ref.padre) {
    const nuevoIR = insertarNodo(ir, ref.padre.id, ref.index + 1, clon);
    return { ir: nuevoIR, nuevoId: clon.id };
  }

  // Si es una sección de una página
  if (ref.pagina) {
    const pagId = ref.pagina.id;
    const nuevasPaginas = ir.pages.map((p) => {
      if (p.id !== pagId) return p;
      const secs = [...p.sections];
      secs.splice(ref.index + 1, 0, clon as Seccion);
      return { ...p, sections: secs };
    });
    return { ir: { ...ir, pages: nuevasPaginas }, nuevoId: clon.id };
  }

  return null;
}

/**
 * Envuelve una lista de nodos hermanos en un nuevo Marco (frame con layout auto-flex).
 */
export function envolverEnMarco(ir: Sitio, nodeIds: Id[]): { ir: Sitio; marcoId: Id } | null {
  if (nodeIds.length === 0) return null;

  const primerRef = encontrarNodo(ir, nodeIds[0]!);
  if (!primerRef || !primerRef.padre) return null;

  const padreId = primerRef.padre.id;
  const nodosAEnvolver: Nodo[] = [];

  for (const nid of nodeIds) {
    const r = encontrarNodo(ir, nid);
    if (!r || !r.padre || r.padre.id !== padreId) {
      return null; // Todos deben pertenecer al mismo padre
    }
    nodosAEnvolver.push(r.nodo as Nodo);
  }

  const marcoId = nuevoId();
  const nuevoMarco: Nodo = {
    id: marcoId,
    type: 'frame',
    tag: 'div',
    layout: {
      direction: 'column',
      gap: 16,
      padding: [0, 0, 0, 0],
      align: 'start',
      justify: 'start',
      wrap: false,
    },
    children: nodosAEnvolver,
  };

  // Reemplazar el primer nodo por el marco y eliminar los demás
  const nuevoIR = transformarArbol(ir, (n) => {
    if (n.id === padreId && esContenedor(n)) {
      const nuevosHijos: Nodo[] = [];
      let insertado = false;
      for (const h of n.children) {
        if (nodeIds.includes(h.id)) {
          if (!insertado) {
            nuevosHijos.push(nuevoMarco);
            insertado = true;
          }
        } else {
          nuevosHijos.push(h);
        }
      }
      return { ...n, children: nuevosHijos };
    }
    return n;
  });

  return { ir: nuevoIR, marcoId };
}

/**
 * Deshace la envoltura de un marco, promoviendo sus hijos a la posición del marco en el padre.
 */
export function desenvolverMarco(ir: Sitio, marcoId: Id): Sitio {
  const ref = encontrarNodo(ir, marcoId);
  if (!ref || !ref.padre || ref.nodo.type !== 'frame') return ir;

  const hijos = ref.nodo.children;
  const padreId = ref.padre.id;

  return transformarArbol(ir, (n) => {
    if (n.id === padreId && esContenedor(n)) {
      const nuevosHijos: Nodo[] = [];
      for (const h of n.children) {
        if (h.id === marcoId) {
          nuevosHijos.push(...hijos);
        } else {
          nuevosHijos.push(h);
        }
      }
      return { ...n, children: nuevosHijos };
    }
    return n;
  });
}

/**
 * Redimensiona un nodo a medidas fijas o relativas, afectando a la base (desktop) o al breakpoint activo.
 */
export function redimensionarNodo(
  ir: Sitio,
  nodeId: Id,
  dims: { width: Medida; height: Medida },
  puntoDeCorte: PuntoDeCorte = 'desktop',
): Sitio {
  return transformarArbol(ir, (n) => {
    if (n.id !== nodeId) return n;

    if (puntoDeCorte === 'desktop') {
      const sizeActual = n.size ?? { width: 'hug', height: 'hug' };
      return {
        ...n,
        size: {
          ...sizeActual,
          width: dims.width,
          height: dims.height,
        },
      };
    }

    // Tablet o Mobile van en breakpoints[puntoDeCorte].size
    const bps = n.breakpoints ?? {};
    const bpActual = bps[puntoDeCorte] ?? {};
    const sizeBpActual = bpActual.size ?? {};

    return {
      ...n,
      breakpoints: {
        ...bps,
        [puntoDeCorte]: {
          ...bpActual,
          size: {
            ...sizeBpActual,
            width: dims.width,
            height: dims.height,
          },
        },
      },
    };
  });
}

// -----------------------------------------------------------------------------
// Operaciones de páginas
// -----------------------------------------------------------------------------

export function crearPagina(
  ir: Sitio,
  titulo: string,
  slugBase?: string,
): { ir: Sitio; paginaId: Id } {
  const pid = nuevoId();
  const defLang = ir.languages.default;
  const slugCalculado =
    slugBase ||
    titulo
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-|-$/g, '') ||
    `pagina-${pid.slice(0, 4)}`;

  const nuevaPagina: Pagina = {
    id: pid,
    slug: { [defLang]: slugCalculado },
    title: { [defLang]: titulo },
    description: { [defLang]: '' },
    sections: [],
    show_header: true,
    show_footer: true,
  };

  return {
    ir: { ...ir, pages: [...ir.pages, nuevaPagina] },
    paginaId: pid,
  };
}

export function duplicarPagina(ir: Sitio, pageId: Id): { ir: Sitio; paginaId: Id } | null {
  const orig = ir.pages.find((p) => p.id === pageId);
  if (!orig) return null;

  const nuevaId = nuevoId();
  const defLang = ir.languages.default;
  const tituloOrig = orig.title[defLang] || 'Página';
  const slugOrig = orig.slug[defLang] || 'pagina';

  const clonSecciones = orig.sections.map((s) => clonarNodoConNuevosIds(s) as Seccion);

  const nuevaPagina: Pagina = {
    ...orig,
    id: nuevaId,
    title: { ...orig.title, [defLang]: `${tituloOrig} (Copia)` },
    slug: { ...orig.slug, [defLang]: `${slugOrig}-copia-${nuevaId.slice(0, 4)}` },
    sections: clonSecciones,
  };

  const idx = ir.pages.findIndex((p) => p.id === pageId);
  const pags = [...ir.pages];
  pags.splice(idx + 1, 0, nuevaPagina);

  return { ir: { ...ir, pages: pags }, paginaId: nuevaId };
}

export function eliminarPagina(ir: Sitio, pageId: Id): Sitio {
  // La portada (pages[0]) nunca se puede eliminar
  if (ir.pages.length <= 1 || ir.pages[0]?.id === pageId) {
    return ir;
  }
  return {
    ...ir,
    pages: ir.pages.filter((p) => p.id !== pageId),
  };
}

export function reordenarPaginas(ir: Sitio, pageIds: Id[]): Sitio {
  if (pageIds.length !== ir.pages.length) return ir;

  const mapa = new Map(ir.pages.map((p) => [p.id, p]));
  const reordenadas: Pagina[] = [];

  for (const pid of pageIds) {
    const p = mapa.get(pid);
    if (!p) return ir;
    reordenadas.push(p);
  }

  return { ...ir, pages: reordenadas };
}

export function actualizarAjustesPagina(
  ir: Sitio,
  pageId: Id,
  ajustes: Partial<
    Pick<Pagina, 'slug' | 'title' | 'description' | 'show_header' | 'show_footer' | 'noindex'>
  >,
): Sitio {
  return {
    ...ir,
    pages: ir.pages.map((p) => (p.id === pageId ? { ...p, ...ajustes } : p)),
  };
}

// -----------------------------------------------------------------------------
// Operaciones de capas (bloquear, ocultar, renombrar, reordenar hermanos)
// -----------------------------------------------------------------------------

export function renombrarNodo(ir: Sitio, nodeId: Id, nuevoNombre: string): Sitio {
  return transformarArbol(ir, (n) => {
    if (n.id === nodeId) {
      if (n.type === 'icon') {
        return n;
      }
      return { ...n, name: nuevoNombre.trim().slice(0, 60) };
    }
    return n;
  });
}

export function bloquearNodo(ir: Sitio, nodeId: Id, bloquear?: boolean): Sitio {
  return transformarArbol(ir, (n) => {
    if (n.id === nodeId) {
      const debeBloquear = bloquear !== undefined ? bloquear : !n.locked;
      const copia = { ...n };
      if (debeBloquear) {
        copia.locked = true;
      } else {
        delete copia.locked;
      }
      return copia;
    }
    return n;
  });
}

export function ocultarNodo(
  ir: Sitio,
  nodeId: Id,
  ocultar?: boolean,
  puntoDeCorte: PuntoDeCorte = 'desktop',
): Sitio {
  return transformarArbol(ir, (n) => {
    if (n.id !== nodeId) return n;

    if (puntoDeCorte === 'desktop') {
      const debeOcultar = ocultar !== undefined ? ocultar : !n.hidden;
      const copia = { ...n };
      if (debeOcultar) {
        copia.hidden = true;
      } else {
        delete copia.hidden;
      }
      return copia;
    }

    // Tablet o Mobile: se guarda en breakpoints[puntoDeCorte].hidden
    const bps = n.breakpoints ?? {};
    const bpActual = bps[puntoDeCorte] ?? {};
    const debeOcultar = ocultar !== undefined ? ocultar : bpActual.hidden !== true;

    return {
      ...n,
      breakpoints: {
        ...bps,
        [puntoDeCorte]: {
          ...bpActual,
          hidden: debeOcultar,
        },
      },
    };
  });
}

export function reordenarHermanos(ir: Sitio, nodeId: Id, direccion: 'arriba' | 'abajo'): Sitio {
  const ref = encontrarNodo(ir, nodeId);
  if (!ref) return ir;

  // Si es sección en página
  if (ref.pagina && ref.nodo.type === 'section') {
    const pag = ref.pagina;
    const idx = ref.index;
    const destino = direccion === 'arriba' ? idx - 1 : idx + 1;
    if (destino < 0 || destino >= pag.sections.length) return ir;

    const nuevasSecciones = [...pag.sections];
    const temp = nuevasSecciones[idx]!;
    nuevasSecciones[idx] = nuevasSecciones[destino]!;
    nuevasSecciones[destino] = temp;

    return {
      ...ir,
      pages: ir.pages.map((p) => (p.id === pag.id ? { ...p, sections: nuevasSecciones } : p)),
    };
  }

  // Si tiene un padre contenedor
  if (ref.padre) {
    const hijos = ref.padre.children;
    const idx = ref.index;
    const destino = direccion === 'arriba' ? idx - 1 : idx + 1;
    if (destino < 0 || destino >= hijos.length) return ir;

    return transformarArbol(ir, (n) => {
      if (n.id === ref.padre!.id && esContenedor(n)) {
        const nuevos = [...n.children];
        const temp = nuevos[idx]!;
        nuevos[idx] = nuevos[destino]!;
        nuevos[destino] = temp;
        return { ...n, children: nuevos };
      }
      return n;
    });
  }

  return ir;
}

// -----------------------------------------------------------------------------
// Operaciones de propiedades (auto-layout, estilo, contenido y overrides)
// -----------------------------------------------------------------------------

export function actualizarLayoutNodo(
  ir: Sitio,
  nodeId: Id,
  layoutParcial: Partial<Layout>,
  puntoDeCorte: PuntoDeCorte = 'desktop',
): Sitio {
  return transformarArbol(ir, (n) => {
    if (n.id !== nodeId) return n;

    if (puntoDeCorte === 'desktop') {
      if (n.type === 'frame' || n.type === 'section' || n.type === 'form') {
        const actual = n.layout ?? {
          direction: 'column',
          gap: 16,
          padding: [0, 0, 0, 0],
          align: 'start',
          justify: 'start',
          wrap: false,
        };
        return {
          ...n,
          layout: { ...actual, ...layoutParcial },
        };
      }
      return n;
    }

    // Breakpoint tablet/mobile
    const bps = n.breakpoints ?? {};
    const bpActual = bps[puntoDeCorte] ?? {};
    const layoutActual = bpActual.layout ?? {};

    return {
      ...n,
      breakpoints: {
        ...bps,
        [puntoDeCorte]: {
          ...bpActual,
          layout: { ...layoutActual, ...layoutParcial },
        },
      },
    };
  });
}

export function actualizarEstiloNodo(
  ir: Sitio,
  nodeId: Id,
  estiloParcial: Partial<Estilo>,
  puntoDeCorte: PuntoDeCorte = 'desktop',
): Sitio {
  return transformarArbol(ir, (n) => {
    if (n.id !== nodeId) return n;

    if (puntoDeCorte === 'desktop') {
      const estiloActual = n.style ?? {};
      return {
        ...n,
        style: { ...estiloActual, ...estiloParcial },
      };
    }

    // Breakpoint tablet/mobile (estilos de texto o generales)
    const bps = n.breakpoints ?? {};
    const bpActual = bps[puntoDeCorte] ?? {};

    return {
      ...n,
      breakpoints: {
        ...bps,
        [puntoDeCorte]: {
          ...bpActual,
        },
      },
    };
  });
}

export function actualizarContenidoNodo(
  ir: Sitio,
  nodeId: Id,
  parche: Record<string, unknown>,
  idioma?: Idioma,
): Sitio {
  return transformarArbol(ir, (n) => {
    if (n.id !== nodeId) return n;

    // Si es un nodo de texto y se actualiza el contenido
    if (n.type === 'text' && typeof parche.text === 'string' && idioma) {
      const textosActuales = n.content ?? {};
      return {
        ...n,
        content: { ...textosActuales, [idioma]: [{ text: parche.text }] },
      };
    }

    // Botones y controles con rótulo mantienen el mapa de idiomas del IR.
    if (
      (n.type === 'button' || n.type === 'cart_button' || n.type === 'icon') &&
      typeof parche.label === 'string' &&
      idioma
    ) {
      const labels = n.label ?? {};
      return {
        ...n,
        label: { ...labels, [idioma]: parche.label },
      };
    }

    // Otros parches directos sobre el nodo
    return { ...n, ...parche };
  });
}

/** Aplica el mapa estable de IA; conserva también las traducciones editadas mientras se generaba. */
export function aplicarTraducciones(
  ir: Sitio,
  resultado: unknown,
  sobrescribir = false,
  idiomasDestino?: readonly string[],
): Sitio {
  if (!resultado || typeof resultado !== 'object' || Array.isArray(resultado)) return ir;
  // Solo se admiten los destinos del trabajo, nunca idiomas arbitrarios de su respuesta.
  const destinos = new Set(
    (idiomasDestino ?? [ir.languages.default, ...ir.languages.others]).filter((idioma) =>
      IDIOMAS_VISITANTE.some((permitido) => permitido === idioma),
    ),
  );
  const parches = resultado as Record<string, unknown>;
  const para = (clave: string): Textos | undefined => {
    const valor = parches[clave];
    if (!valor || typeof valor !== 'object' || Array.isArray(valor)) return undefined;
    const salida: Textos = {};
    for (const [idioma, texto] of Object.entries(valor)) {
      if (typeof texto === 'string' && destinos.has(idioma)) salida[idioma as Idioma] = texto;
    }
    return Object.keys(salida).length > 0 ? salida : undefined;
  };
  const combinar = (actual: Textos, parche?: Textos): Textos => {
    if (!parche) return actual;
    const salida = { ...actual };
    for (const [idioma, texto] of Object.entries(parche)) {
      const lengua = idioma as Idioma;
      if (sobrescribir || !actual[lengua]?.trim()) salida[lengua] = texto;
    }
    return salida;
  };
  const rico = (clave: string): TextosRicos | undefined => {
    const parche = para(clave);
    return parche
      ? Object.fromEntries(
          Object.entries(parche).map(([idioma, texto]) => [idioma, [{ text: texto }]]),
        )
      : undefined;
  };
  const combinarRico = (actual: TextosRicos, parche?: TextosRicos): TextosRicos => {
    if (!parche) return actual;
    const salida = { ...actual };
    for (const [idioma, tramos] of Object.entries(parche)) {
      const lengua = idioma as Idioma;
      const tieneTexto = actual[lengua]?.some((tramo) => tramo.text.trim().length > 0);
      if (sobrescribir || !tieneTexto) salida[lengua] = tramos;
    }
    return salida;
  };

  const transformar = (n: Nodo | Seccion): Nodo | Seccion => {
    switch (n.type) {
      case 'text':
        return { ...n, content: combinarRico(n.content, rico(`${n.id}/content`)) };
      case 'image':
        return { ...n, alt: combinar(n.alt, para(`${n.id}/alt`)) };
      case 'button':
      case 'cart_button':
        return { ...n, label: combinar(n.label, para(`${n.id}/label`)) };
      case 'icon':
        return { ...n, label: combinar(n.label ?? {}, para(`${n.id}/label`)) };
      case 'embed':
        return { ...n, title: combinar(n.title, para(`${n.id}/title`)) };
      case 'form':
        return {
          ...n,
          submit: combinar(n.submit, para(`${n.id}/submit`)),
          success: combinar(n.success, para(`${n.id}/success`)),
          consent: combinar(n.consent, para(`${n.id}/consent`)),
          fields: n.fields.map((campo) => ({
            ...campo,
            label: combinar(campo.label, para(`${campo.id}/label`)),
            ...(campo.options
              ? {
                  options: campo.options.map((opcion, i) =>
                    combinar(opcion, para(`${campo.id}/options/${i}`)),
                  ),
                }
              : {}),
          })),
        };
      case 'table':
        return {
          ...n,
          caption: combinar(n.caption, para(`${n.id}/caption`)),
          header: n.header.map((celda, i) => combinar(celda, para(`${n.id}/header/${i}`))),
          rows: n.rows.map((fila, f) =>
            fila.map((celda, c) => combinar(celda, para(`${n.id}/rows/${f}/${c}`))),
          ),
        };
      case 'menu':
        return {
          ...n,
          label: combinar(n.label ?? {}, para(`${n.id}/label`)),
          items: aplicarMenu(n.items, para, combinar),
        };
      case 'instance': {
        const overrides: Record<string, Override> = {};
        for (const [id, override] of Object.entries(n.overrides ?? {})) {
          const texto = rico(`${n.id}/${id}/text`);
          const etiqueta = para(`${n.id}/${id}/label`);
          const alt = para(`${n.id}/${id}/alt`);
          overrides[id] = {
            ...override,
            ...(texto ? { text: combinarRico(override.text ?? {}, texto) } : {}),
            ...(etiqueta ? { label: combinar(override.label ?? {}, etiqueta) } : {}),
            ...(alt ? { alt: combinar(override.alt ?? {}, alt) } : {}),
          };
        }
        return {
          ...n,
          overrides,
        };
      }
      default:
        return n;
    }
  };

  const sitio = transformarArbol(ir, transformar);
  const traducido: Sitio = {
    ...sitio,
    seo: { ...sitio.seo, site_name: combinar(sitio.seo.site_name, para('seo/site_name')) },
    pages: sitio.pages.map((pagina) => ({
      ...pagina,
      title: combinar(pagina.title, para(`${pagina.id}/title`)),
      description: combinar(pagina.description, para(`${pagina.id}/description`)),
    })),
  };
  if (!idiomasDestino) return traducido;
  return actualizarIdiomas(traducido, {
    default: ir.languages.default,
    others: [...new Set([...ir.languages.others, ...destinos])].filter(
      (idioma) => idioma !== ir.languages.default,
    ) as Idioma[],
  });
}

function aplicarMenu(
  items: Extract<Nodo, { type: 'menu' }>['items'],
  para: (clave: string) => Textos | undefined,
  combinar: (actual: Textos, parche?: Textos) => Textos,
): Extract<Nodo, { type: 'menu' }>['items'] {
  return items.map((item) => ({
    ...item,
    label: combinar(item.label, para(`${item.id}/label`)),
    ...(item.children ? { children: aplicarMenu(item.children, para, combinar) } : {}),
  }));
}

export function restablecerOverrideBreakpoint(
  ir: Sitio,
  nodeId: Id,
  puntoDeCorte: 'tablet' | 'mobile',
): Sitio {
  return transformarArbol(ir, (n) => {
    if (n.id !== nodeId || !n.breakpoints) return n;

    const copiaBps = { ...n.breakpoints };
    delete copiaBps[puntoDeCorte];

    if (Object.keys(copiaBps).length === 0) {
      const { breakpoints: _omitted, ...sinBreakpoints } = n as { breakpoints?: unknown };
      return sinBreakpoints as typeof n;
    }
    return { ...n, breakpoints: copiaBps };
  });
}

// -----------------------------------------------------------------------------
// Operaciones de Sitio (Tema, Idiomas, SEO)
// -----------------------------------------------------------------------------

export function actualizarTema(ir: Sitio, temaParcial: Partial<Tema>): Sitio {
  return {
    ...ir,
    theme: {
      ...ir.theme,
      ...temaParcial,
      colors: temaParcial.colors ? { ...ir.theme.colors, ...temaParcial.colors } : ir.theme.colors,
      fonts: temaParcial.fonts ? { ...ir.theme.fonts, ...temaParcial.fonts } : ir.theme.fonts,
      radius: temaParcial.radius ? { ...ir.theme.radius, ...temaParcial.radius } : ir.theme.radius,
      text_styles: temaParcial.text_styles
        ? { ...ir.theme.text_styles, ...temaParcial.text_styles }
        : ir.theme.text_styles,
    },
  };
}

export function actualizarIdiomas(ir: Sitio, languages: Sitio['languages']): Sitio {
  const habilitados = [languages.default, ...languages.others];
  return {
    ...ir,
    languages,
    // Todas las páginas necesitan una ruta en cada idioma, incluso si solo se traduce una.
    pages: ir.pages.map((pagina, indice) => {
      const slug = { ...pagina.slug };
      for (const idioma of habilitados) {
        if (slug[idioma] === undefined)
          slug[idioma] =
            indice === 0 ? '' : (pagina.slug[ir.languages.default] ?? `pagina-${pagina.id}`);
      }
      return { ...pagina, slug };
    }),
  };
}

export function actualizarSeo(ir: Sitio, seoParcial: Partial<SeoSitio>): Sitio {
  return {
    ...ir,
    seo: {
      ...ir.seo,
      ...seoParcial,
    },
  };
}

// -----------------------------------------------------------------------------
// Generación e inserción de nuevos bloques predefinidos
// -----------------------------------------------------------------------------

export type BloqueInsertable =
  | 'texto'
  | 'imagen'
  | 'boton'
  | 'icono'
  | 'marco'
  | 'tabla'
  | 'formulario'
  | 'mapa'
  | 'video'
  | 'rejilla_productos'
  | 'menu'
  | 'pdf'
  | 'whatsapp'
  | 'redes';

/** Una vista de Sytue Bases tal como la devuelve `POST /api/sites/{id}/bases/table`. */
export interface TablaDeBases {
  caption: string;
  header: string[];
  rows: string[][];
  provenance: { base_name: string; table_name: string; view_name: string; version: number };
}

/**
 * Un nodo `table` corriente con los datos de una vista de Sytue Bases (instantánea). La
 * procedencia queda en el NOMBRE del nodo, que solo se ve en el editor (capas): el sitio
 * publicado enseña la tabla, no de dónde salió.
 */
export function crearTablaDeBases(datos: TablaDeBases, idioma: Idioma = 'es'): Nodo {
  const p = datos.provenance;
  return {
    id: nuevoId(),
    type: 'table',
    name: `Sytue Bases · ${p.base_name} › ${p.table_name} › ${p.view_name} · v${p.version}`.slice(
      0,
      120,
    ),
    caption: { [idioma]: datos.caption },
    header: datos.header.map((h) => ({ [idioma]: h })),
    rows: datos.rows.map((fila) => fila.map((c) => ({ [idioma]: c }))),
  };
}

export function crearNodoPorTipo(tipo: BloqueInsertable, idioma: Idioma = 'es'): Nodo {
  const nid = nuevoId();

  switch (tipo) {
    case 'texto':
      return {
        id: nid,
        type: 'text',
        name: 'Texto',
        tag: 'p',
        content: {
          [idioma]: [{ text: 'Escribe tu contenido aquí...' }],
        },
        style: {
          text: { preset: 'body' },
        },
      };

    case 'imagen':
      return {
        id: nid,
        type: 'image',
        name: 'Imagen',
        src: { placeholder: 'photo' },
        alt: { [idioma]: 'Fotografía descriptiva' },
        size: { width: 'fill', height: 320 },
        fit: 'cover',
      };

    case 'boton':
      return {
        id: nid,
        type: 'button',
        name: 'Botón',
        label: { [idioma]: 'Hacer clic' },
        action: { url: '#' },
        variant: 'primary',
        size: { width: 'hug', height: 'hug' },
      };

    case 'icono':
      return {
        id: nid,
        type: 'icon',
        name: 'star',
      };

    case 'marco':
      return {
        id: nid,
        type: 'frame',
        name: 'Contenedor',
        tag: 'div',
        layout: {
          direction: 'column',
          gap: 16,
          padding: [16, 16, 16, 16],
          align: 'start',
          justify: 'start',
          wrap: false,
        },
        children: [],
      };

    case 'tabla':
      return {
        id: nid,
        type: 'table',
        name: 'Tabla',
        caption: { [idioma]: 'Tabla de datos' },
        header: [{ [idioma]: 'Columna 1' }, { [idioma]: 'Columna 2' }, { [idioma]: 'Columna 3' }],
        rows: [
          [{ [idioma]: 'Dato 1' }, { [idioma]: 'Dato 2' }, { [idioma]: 'Dato 3' }],
          [{ [idioma]: 'Dato 4' }, { [idioma]: 'Dato 5' }, { [idioma]: 'Dato 6' }],
        ],
      };

    case 'formulario':
      return {
        id: nid,
        type: 'form',
        name: 'Formulario',
        form: 'contacto',
        layout: {
          direction: 'column',
          gap: 16,
          padding: [0, 0, 0, 0],
          align: 'start',
          justify: 'start',
          wrap: false,
        },
        fields: [
          {
            id: nuevoId(),
            kind: 'text',
            label: { [idioma]: 'Nombre completo' },
            required: true,
          },
          {
            id: nuevoId(),
            kind: 'email',
            label: { [idioma]: 'Correo electrónico' },
            required: true,
          },
          {
            id: nuevoId(),
            kind: 'textarea',
            label: { [idioma]: 'Mensaje o consulta' },
            required: true,
          },
        ],
        submit: { [idioma]: 'Enviar mensaje' },
        success: { [idioma]: '¡Mensaje recibido con éxito!' },
        consent: { [idioma]: 'He leído y acepto la política de privacidad.' },
      };

    case 'mapa':
      return {
        id: nid,
        type: 'embed',
        name: 'Mapa',
        provider: 'osm',
        ref: '40.415,-3.705,15',
        title: { [idioma]: 'Ubicación en el mapa' },
        size: { width: 'fill', height: 350 },
      };

    case 'video':
      return {
        id: nid,
        type: 'embed',
        name: 'Vídeo',
        provider: 'youtube',
        ref: 'dQw4w9WgXcQ',
        title: { [idioma]: 'Vídeo de presentación' },
        size: { width: 'fill', height: 400 },
      };

    case 'rejilla_productos':
      return {
        id: nid,
        type: 'product_grid',
        name: 'Catálogo de productos',
        source: { all: true },
        limit: 8,
        columns: 4,
        show_price: true,
        show_add_to_cart: true,
      };

    case 'menu':
      return {
        id: nid,
        type: 'menu',
        name: 'Menú de navegación',
        orientation: 'horizontal',
        collapse_below: 'mobile',
        items: [
          { id: nuevoId(), label: { [idioma]: 'Inicio' }, link: { page: 'inicio' } },
          { id: nuevoId(), label: { [idioma]: 'Servicios' }, link: { url: '#servicios' } },
          { id: nuevoId(), label: { [idioma]: 'Contacto' }, link: { url: '#contacto' } },
        ],
      };

    case 'pdf':
      return {
        id: nid,
        type: 'button',
        name: 'Descarga PDF',
        label: { [idioma]: 'Descargar documento (PDF)' },
        action: { url: '#' },
        variant: 'secondary',
        icon: 'download',
      };

    case 'whatsapp':
      return {
        id: nid,
        type: 'button',
        name: 'WhatsApp',
        label: { [idioma]: 'Contactar por WhatsApp' },
        action: { url: 'https://wa.me/34600000000' },
        variant: 'primary',
      };

    case 'redes':
      return {
        id: nid,
        type: 'frame',
        name: 'Redes sociales',
        tag: 'div',
        layout: {
          direction: 'row',
          gap: 12,
          padding: [0, 0, 0, 0],
          align: 'center',
          justify: 'start',
          wrap: false,
        },
        children: [
          {
            id: nuevoId(),
            type: 'icon',
            name: 'instagram',
          },
          {
            id: nuevoId(),
            type: 'icon',
            name: 'linkedin',
          },
          {
            id: nuevoId(),
            type: 'icon',
            name: 'whatsapp',
          },
        ],
      };
  }
}

export function insertarSeccionEnPagina(
  ir: Sitio,
  paginaId: Id,
  seccion: Seccion,
  indice?: number,
): Sitio {
  const nuevaSeccion = clonarNodoConNuevosIds(seccion) as Seccion;
  return {
    ...ir,
    pages: ir.pages.map((p) => {
      if (p.id !== paginaId) return p;
      const secs = [...p.sections];
      const pos = indice !== undefined ? Math.max(0, Math.min(indice, secs.length)) : secs.length;
      secs.splice(pos, 0, nuevaSeccion);
      return { ...p, sections: secs };
    }),
  };
}

/** Guarda una copia independiente de un marco como componente reutilizable. */
export function crearComponente(ir: Sitio, nodeId: Id, nombre: string): Sitio {
  const referencia = encontrarNodo(ir, nodeId);
  if (!referencia || referencia.nodo.type !== 'frame') return ir;
  return {
    ...ir,
    components: [
      ...ir.components,
      {
        id: nuevoId(),
        name: nombre.trim() || 'Componente',
        root: clonarNodoConNuevosIds(referencia.nodo) as Marco,
      },
    ],
  };
}

export function crearInstanciaComponente(component: Id): Nodo {
  return { id: nuevoId(), type: 'instance', name: 'Componente', component, overrides: {} };
}
