/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

import { describe, expect, it } from 'bun:test';
import type { Marco, Nodo, Sitio } from '@sytue/pages-render';
import { validar } from '@sytue/pages-render';
import {
  actualizarAjustesPagina,
  actualizarContenidoNodo,
  aplicarTraducciones,
  actualizarEstiloNodo,
  actualizarIdiomas,
  actualizarLayoutNodo,
  actualizarSeo,
  actualizarTema,
  bloquearNodo,
  crearNodoPorTipo,
  crearPagina,
  desenvolverMarco,
  duplicarNodo,
  duplicarPagina,
  eliminarNodo,
  eliminarPagina,
  encontrarNodo,
  envolverEnMarco,
  insertarNodo,
  insertarSeccionEnPagina,
  moverNodo,
  ocultarNodo,
  redimensionarNodo,
  renombrarNodo,
  reordenarHermanos,
  reordenarPaginas,
  restablecerOverrideBreakpoint,
  setPropiedadesNodo,
} from './operaciones.js';
import {
  TOPE_PAGES,
  crearHistorialEditor,
  deshacer,
  rehacer,
  registrarCambio,
  sePuedeDeshacer,
  sePuedeRehacer,
} from './historial.js';

function crearSitioPrueba(): Sitio {
  return {
    version: 1,
    languages: { default: 'es', others: ['en'] },
    theme: {
      colors: {
        background: '#ffffff',
        text: '#111827',
        primary: '#ca1682',
        on_primary: '#ffffff',
        muted: '#6b7280',
        border: '#e5e7eb',
      },
      fonts: { heading: 'figtree', body: 'figtree' },
      text_styles: {
        h1: {
          font: 'heading',
          size: [40, 32, 28],
          line_height: 1.2,
          weight: 700,
          letter_spacing: 0,
        },
        h2: {
          font: 'heading',
          size: [32, 28, 24],
          line_height: 1.2,
          weight: 700,
          letter_spacing: 0,
        },
        h3: {
          font: 'heading',
          size: [24, 22, 20],
          line_height: 1.3,
          weight: 600,
          letter_spacing: 0,
        },
        h4: {
          font: 'heading',
          size: [20, 18, 16],
          line_height: 1.3,
          weight: 600,
          letter_spacing: 0,
        },
        h5: {
          font: 'heading',
          size: [16, 16, 14],
          line_height: 1.4,
          weight: 600,
          letter_spacing: 0,
        },
        h6: {
          font: 'heading',
          size: [14, 14, 12],
          line_height: 1.4,
          weight: 600,
          letter_spacing: 0,
        },
        body: {
          font: 'body',
          size: [16, 16, 14],
          line_height: 1.5,
          weight: 400,
          letter_spacing: 0,
        },
        small: {
          font: 'body',
          size: [14, 14, 12],
          line_height: 1.5,
          weight: 400,
          letter_spacing: 0,
        },
        label: {
          font: 'body',
          size: [14, 14, 12],
          line_height: 1.4,
          weight: 500,
          letter_spacing: 0,
        },
        button: {
          font: 'body',
          size: [16, 16, 14],
          line_height: 1.4,
          weight: 600,
          letter_spacing: 0,
        },
      },
      radius: { sm: 4, md: 8, lg: 16 },
      container: 1200,
    },
    header: {
      id: 'hdr0000001',
      type: 'section',
      kind: 'header',
      full_bleed: false,
      layout: {
        direction: 'row',
        gap: 16,
        padding: [16, 24, 16, 24],
        align: 'center',
        justify: 'space-between',
        wrap: false,
      },
      children: [
        {
          id: 'txt0000001',
          type: 'text',
          tag: 'p',
          content: { es: [{ text: 'Mi Logo' }] },
        },
      ],
    },
    footer: null,
    pages: [
      {
        id: 'pag0000001',
        slug: { es: '', en: '' },
        title: { es: 'Inicio' },
        description: { es: 'Página principal' },
        show_header: true,
        show_footer: true,
        sections: [
          {
            id: 'sec0000001',
            type: 'section',
            kind: 'hero',
            full_bleed: false,
            layout: {
              direction: 'column',
              gap: 24,
              padding: [48, 24, 48, 24],
              align: 'center',
              justify: 'center',
              wrap: false,
            },
            children: [
              {
                id: 'txt0000002',
                type: 'text',
                tag: 'h1',
                content: { es: [{ text: 'Título Hero' }] },
              },
              {
                id: 'frm0000001',
                type: 'frame',
                tag: 'div',
                layout: {
                  direction: 'row',
                  gap: 16,
                  padding: [0, 0, 0, 0],
                  align: 'center',
                  justify: 'center',
                  wrap: false,
                },
                children: [
                  {
                    id: 'btn0000001',
                    type: 'button',
                    variant: 'primary',
                    label: { es: 'Empezar' },
                    action: { url: 'https://ejemplo.es' },
                  },
                  {
                    id: 'btn0000002',
                    type: 'button',
                    variant: 'primary',
                    label: { es: 'Saber más' },
                    action: { url: 'https://ejemplo.es/info' },
                  },
                ],
              },
            ],
          },
        ],
      },
    ],
    components: [],
    seo: {
      site_name: { es: 'Sitio Prueba' },
    },
  };
}

describe('Modelo de operaciones de Pages IR', () => {
  it('encuentra nodos en cualquier nivel del árbol', () => {
    const ir = crearSitioPrueba();

    const h = encontrarNodo(ir, 'hdr0000001');
    expect(h).not.toBeNull();
    expect(h?.nodo.type).toBe('section');

    const t = encontrarNodo(ir, 'txt0000001');
    expect(t).not.toBeNull();
    expect(t?.padre?.id).toBe('hdr0000001');

    const btn = encontrarNodo(ir, 'btn0000002');
    expect(btn).not.toBeNull();
    expect(btn?.nodo.type).toBe('button');
    expect(btn?.padre?.id).toBe('frm0000001');
    expect(btn?.pagina?.id).toBe('pag0000001');
    expect(btn?.index).toBe(1);

    expect(encontrarNodo(ir, 'no-existe')).toBeNull();
  });

  it('modifica propiedades de nodo de forma inmutable', () => {
    const ir = crearSitioPrueba();
    const modificado = setPropiedadesNodo(ir, 'txt0000002', {
      name: 'Cabecera Principal',
    });

    expect(modificado).not.toBe(ir);
    const refOriginal = encontrarNodo(ir, 'txt0000002');
    expect(refOriginal?.nodo.name).toBeUndefined();

    const refModificado = encontrarNodo(modificado, 'txt0000002');
    expect(refModificado?.nodo.name).toBe('Cabecera Principal');
  });

  it('inserta un nodo en un marco o sección', () => {
    const ir = crearSitioPrueba();
    const nuevoBoton: Nodo = {
      id: 'btn0000003',
      type: 'button',
      variant: 'primary',
      label: { es: 'Nuevo' },
      action: { url: '#' },
    };

    const conBoton = insertarNodo(ir, 'frm0000001', 1, nuevoBoton);
    const frame = encontrarNodo(conBoton, 'frm0000001')?.nodo as Marco;
    expect(frame.children).toHaveLength(3);
    expect(frame.children[1]!.id).toBe('btn0000003');
  });

  it('elimina un nodo correctamente', () => {
    const ir = crearSitioPrueba();
    const sinBoton = eliminarNodo(ir, 'btn0000001');
    expect(encontrarNodo(sinBoton, 'btn0000001')).toBeNull();
    const frame = encontrarNodo(sinBoton, 'frm0000001')?.nodo as Marco;
    expect(frame.children).toHaveLength(1);
    expect(frame.children[0]!.id).toBe('btn0000002');
  });

  it('mueve un nodo entre contenedores', () => {
    const ir = crearSitioPrueba();
    // Mover btn0000002 de frm0000001 directamente a sec0000001
    const movido = moverNodo(ir, 'btn0000002', 'sec0000001', 0);

    const ref = encontrarNodo(movido, 'btn0000002');
    expect(ref?.padre?.id).toBe('sec0000001');
    expect(ref?.index).toBe(0);

    const frame = encontrarNodo(movido, 'frm0000001')?.nodo as Marco;
    expect(frame.children).toHaveLength(1);
  });

  it('duplica un nodo generando IDs únicos recursivos', () => {
    const ir = crearSitioPrueba();
    const res = duplicarNodo(ir, 'frm0000001');
    expect(res).not.toBeNull();
    expect(res?.nuevoId).not.toBe('frm0000001');

    const nuevoFrame = encontrarNodo(res!.ir, res!.nuevoId)?.nodo as Marco;
    expect(nuevoFrame).not.toBeNull();
    expect(nuevoFrame.children).toHaveLength(2);
    expect(nuevoFrame.children[0]!.id).not.toBe('btn0000001');
    expect(nuevoFrame.children[1]!.id).not.toBe('btn0000002');
  });

  it('envuelve y desenvuelve nodos en un marco', () => {
    const ir = crearSitioPrueba();
    // Envolver btn0000001 y btn0000002
    const envuelto = envolverEnMarco(ir, ['btn0000001', 'btn0000002']);
    expect(envuelto).not.toBeNull();
    const marcoId = envuelto!.marcoId;

    const marcoNode = encontrarNodo(envuelto!.ir, marcoId)?.nodo as Marco;
    expect(marcoNode.type).toBe('frame');
    expect(marcoNode.children).toHaveLength(2);
    expect(marcoNode.children[0]!.id).toBe('btn0000001');

    // Ahora desenvolverlo
    const desenvuelto = desenvolverMarco(envuelto!.ir, marcoId);
    const padre = encontrarNodo(desenvuelto, 'frm0000001')?.nodo as Marco;
    expect(padre.children).toHaveLength(2);
    expect(padre.children[0]!.id).toBe('btn0000001');
    expect(padre.children[1]!.id).toBe('btn0000002');
    expect(encontrarNodo(desenvuelto, marcoId)).toBeNull();
  });

  it('redimensiona nodos en escritorio y en breakpoints móviles', () => {
    const ir = crearSitioPrueba();

    // Escritorio
    const redimDesktop = redimensionarNodo(ir, 'btn0000001', { width: 200, height: 48 }, 'desktop');
    const nDesktop = encontrarNodo(redimDesktop, 'btn0000001')?.nodo;
    expect(nDesktop?.size?.width).toBe(200);
    expect(nDesktop?.size?.height).toBe(48);

    // Móvil
    const redimMobile = redimensionarNodo(
      redimDesktop,
      'btn0000001',
      { width: 'fill', height: 'hug' },
      'mobile',
    );
    const nMobile = encontrarNodo(redimMobile, 'btn0000001')?.nodo;
    expect(nMobile?.size?.width).toBe(200); // Desktop no cambia
    expect(nMobile?.breakpoints?.mobile?.size?.width).toBe('fill');
    expect(nMobile?.breakpoints?.mobile?.size?.height).toBe('hug');
  });

  it('gestiona páginas: crear, duplicar, reordenar y eliminar', () => {
    const ir = crearSitioPrueba();

    // Crear página
    const { ir: conPag, paginaId } = crearPagina(ir, 'Contacto');
    expect(conPag.pages).toHaveLength(2);
    expect(conPag.pages[1]!.id).toBe(paginaId);
    expect(conPag.pages[1]!.slug.es).toBe('contacto');

    // Actualizar ajustes
    const ajustado = actualizarAjustesPagina(conPag, paginaId, { noindex: true });
    expect(ajustado.pages[1]!.noindex).toBe(true);

    // Duplicar página
    const dupRes = duplicarPagina(ajustado, paginaId);
    expect(dupRes).not.toBeNull();
    expect(dupRes!.ir.pages).toHaveLength(3);
    expect(dupRes!.ir.pages[2]!.id).toBe(dupRes!.paginaId);

    // Reordenar páginas
    const reordenado = reordenarPaginas(dupRes!.ir, [dupRes!.paginaId, 'pag0000001', paginaId]);
    expect(reordenado.pages[0]!.id).toBe(dupRes!.paginaId);

    // Eliminar página
    const sinPagina = eliminarPagina(reordenado, paginaId);
    expect(sinPagina.pages).toHaveLength(2);

    // No permite eliminar la portada si solo queda una
    const unico = crearSitioPrueba();
    expect(eliminarPagina(unico, 'pag0000001').pages).toHaveLength(1);
  });
});

describe('Historial del editor (tope 100)', () => {
  it('guarda hasta 100 pasos y gestiona deshacer/rehacer', () => {
    let h = crearHistorialEditor();
    const irBase = crearSitioPrueba();
    let actual = irBase;

    expect(sePuedeDeshacer(h)).toBe(false);
    expect(sePuedeRehacer(h)).toBe(false);

    // Anotar 105 cambios
    for (let i = 1; i <= 105; i++) {
      h = registrarCambio(h, actual);
      actual = {
        ...actual,
        seo: { site_name: { es: `Paso ${i}` } },
      };
    }

    expect(h.pasado.length).toBe(TOPE_PAGES); // Exactamente 100
    expect(sePuedeDeshacer(h)).toBe(true);

    // Deshacer 1 paso
    const paso1 = deshacer(h, actual);
    expect(paso1).not.toBeNull();
    expect(paso1!.modelo.seo.site_name.es).toBe('Paso 104');
    expect(sePuedeRehacer(paso1!.historial)).toBe(true);

    // Rehacer 1 paso
    const reh = rehacer(paso1!.historial, paso1!.modelo);
    expect(reh).not.toBeNull();
    expect(reh!.modelo.seo.site_name.es).toBe('Paso 105');
  });

  it('permite renombrar, bloquear y ocultar nodos', () => {
    let ir = crearSitioPrueba();
    ir = renombrarNodo(ir, 'btn0000001', 'Botón Principal');
    const nodo = encontrarNodo(ir, 'btn0000001');
    expect(nodo?.nodo.name).toBe('Botón Principal');

    ir = bloquearNodo(ir, 'btn0000001', true);
    expect(encontrarNodo(ir, 'btn0000001')?.nodo.locked).toBe(true);

    ir = bloquearNodo(ir, 'btn0000001', false);
    expect(encontrarNodo(ir, 'btn0000001')?.nodo.locked).toBeUndefined();

    // Ocultar en escritorio
    ir = ocultarNodo(ir, 'btn0000001', true, 'desktop');
    expect(encontrarNodo(ir, 'btn0000001')?.nodo.hidden).toBe(true);

    // Ocultar en móvil (override de breakpoint)
    ir = ocultarNodo(ir, 'btn0000001', true, 'mobile');
    expect(encontrarNodo(ir, 'btn0000001')?.nodo.breakpoints?.mobile?.hidden).toBe(true);
  });

  it('permite reordenar hermanos arriba y abajo', () => {
    let ir = crearSitioPrueba();
    // 'btn0000001' (index 0) y 'btn0000002' (index 1) están en 'frm0000001'
    ir = reordenarHermanos(ir, 'btn0000002', 'arriba');
    const fr = encontrarNodo(ir, 'frm0000001')?.nodo as Marco;
    expect(fr.children[0]?.id).toBe('btn0000002');
    expect(fr.children[1]?.id).toBe('btn0000001');

    ir = reordenarHermanos(ir, 'btn0000002', 'abajo');
    const fr2 = encontrarNodo(ir, 'frm0000001')?.nodo as Marco;
    expect(fr2.children[0]?.id).toBe('btn0000001');
    expect(fr2.children[1]?.id).toBe('btn0000002');
  });

  it('actualiza layout, estilos y overrides de breakpoint', () => {
    let ir = crearSitioPrueba();
    ir = actualizarLayoutNodo(
      ir,
      'frm0000001',
      { direction: 'row', gap: 24, justify: 'center' },
      'desktop',
    );
    const fr = encontrarNodo(ir, 'frm0000001')?.nodo as Marco;
    expect(fr.layout?.direction).toBe('row');
    expect(fr.layout?.gap).toBe(24);
    expect(fr.layout?.justify).toBe('center');

    // Override en tablet
    ir = actualizarLayoutNodo(ir, 'frm0000001', { direction: 'column' }, 'tablet');
    expect(encontrarNodo(ir, 'frm0000001')?.nodo.breakpoints?.tablet?.layout?.direction).toBe(
      'column',
    );

    // Restablecer override
    ir = restablecerOverrideBreakpoint(ir, 'frm0000001', 'tablet');
    expect(encontrarNodo(ir, 'frm0000001')?.nodo.breakpoints?.tablet).toBeUndefined();

    // Actualizar estilo
    ir = actualizarEstiloNodo(ir, 'btn0000001', { radius: '$lg', shadow: 'md' });
    const btn = encontrarNodo(ir, 'btn0000001')?.nodo;
    expect(btn?.style?.radius).toBe('$lg');
    expect(btn?.style?.shadow).toBe('md');
  });

  it('actualiza tema, idiomas y seo del sitio', () => {
    let ir = crearSitioPrueba();
    ir = actualizarTema(ir, {
      container: 1400,
      radius: { sm: 4, md: 8, lg: 16 },
    });
    expect(ir.theme.container).toBe(1400);
    expect(ir.theme.radius.lg).toBe(16);

    ir = actualizarIdiomas(ir, { default: 'en', others: ['es'] });
    expect(ir.languages.default).toBe('en');

    ir = actualizarSeo(ir, {
      site_name: { es: 'Mi Sitio Web', en: 'My Website' },
    });
    expect(ir.seo.site_name.es).toBe('Mi Sitio Web');
    expect(ir.seo.site_name.en).toBe('My Website');
  });

  it('crea e inserta bloques predefinidos de varios tipos', () => {
    const tipos = [
      'texto',
      'imagen',
      'boton',
      'icono',
      'marco',
      'tabla',
      'formulario',
      'mapa',
      'video',
      'rejilla_productos',
      'menu',
      'pdf',
      'whatsapp',
      'redes',
    ] as const;

    for (const t of tipos) {
      const nodo = crearNodoPorTipo(t, 'es');
      expect(nodo.id).toBeDefined();
      expect(nodo.name).toBeDefined();
    }

    let ir = crearSitioPrueba();
    const sec = ir.pages[0]!.sections[0]!;
    ir = insertarSeccionEnPagina(ir, 'pag0000001', sec);
    expect(ir.pages[0]!.sections.length).toBe(2);
  });

  it('actualiza contenido de nodos texto y boton', () => {
    let ir = crearSitioPrueba();
    ir = actualizarContenidoNodo(ir, 'txt0000001', { text: 'Nuevo Texto' }, 'es');
    const refTexto = encontrarNodo(ir, 'txt0000001');
    expect(refTexto?.nodo.type).toBe('text');
    if (refTexto?.nodo.type === 'text') {
      expect(refTexto.nodo.content?.es?.[0]?.text).toBe('Nuevo Texto');
    }

    ir = actualizarContenidoNodo(ir, 'btn0000001', { label: 'Comprar Ya' }, 'es');
    const refBoton = encontrarNodo(ir, 'btn0000001');
    expect(refBoton?.nodo.type).toBe('button');
    if (refBoton?.nodo.type === 'button') {
      expect(refBoton.nodo.label?.es).toBe('Comprar Ya');
    }
  });

  it('aplica al borrador las traducciones por id y campo de la IA', () => {
    const resultado = aplicarTraducciones(crearSitioPrueba(), {
      'seo/site_name': { en: 'My site' },
      'pag0000001/title': { en: 'Home' },
      'txt0000002/content': { en: 'Hero heading' },
      'btn0000001/label': { en: 'Start' },
    });

    expect(resultado.seo.site_name.en).toBe('My site');
    expect(resultado.pages[0]?.title.en).toBe('Home');
    const texto = encontrarNodo(resultado, 'txt0000002')?.nodo;
    const boton = encontrarNodo(resultado, 'btn0000001')?.nodo;
    expect(texto?.type === 'text' && texto.content.en?.[0]?.text).toBe('Hero heading');
    expect(boton?.type === 'button' && boton.label.en).toBe('Start');
  });

  it('conserva traducciones actuales, formato y menús salvo que se pida sobrescribir', () => {
    const ir = crearSitioPrueba();
    ir.pages[0]!.title.en = 'Título manual';
    ir.seo.site_name.en = 'Nombre manual';
    const texto = encontrarNodo(ir, 'txt0000002')?.nodo;
    if (!texto || texto.type !== 'text') throw new Error('falta el texto de la prueba');
    texto.content.en = [{ text: 'Editado durante el trabajo', bold: true }];
    const boton = encontrarNodo(ir, 'btn0000001')?.nodo;
    if (!boton || boton.type !== 'button') throw new Error('falta el botón de la prueba');
    boton.label.en = '   ';
    ir.header!.children.push({
      id: 'menu000001',
      type: 'menu',
      orientation: 'horizontal',
      collapse_below: 'mobile',
      items: [
        {
          id: 'item000001',
          label: { es: 'Productos', en: 'Manual menu' },
          link: { url: '/products' },
          children: [
            {
              id: 'item000002',
              label: { es: 'Ofertas', en: 'Manual child' },
              link: { url: '/offers' },
            },
          ],
        },
      ],
    });
    const parche = {
      'seo/site_name': { en: 'Site' },
      'pag0000001/title': { en: 'Home', fr: 'Accueil' },
      'txt0000002/content': { en: 'Heading' },
      'btn0000001/label': { en: 'Start' },
      'item000001/label': { en: 'Products' },
      'item000002/label': { en: 'Offers' },
    };
    const conservado = aplicarTraducciones(ir, parche, false);
    const reemplazado = aplicarTraducciones(ir, parche, true);
    expect(conservado.pages[0]?.title).toEqual({ es: 'Inicio', en: 'Título manual' });
    expect(conservado.seo.site_name.en).toBe('Nombre manual');
    const textoConservado = encontrarNodo(conservado, texto.id)?.nodo;
    const textoReemplazado = encontrarNodo(reemplazado, texto.id)?.nodo;
    expect(textoConservado?.type === 'text' && textoConservado.content.en).toEqual([
      { text: 'Editado durante el trabajo', bold: true },
    ]);
    expect(textoReemplazado?.type === 'text' && textoReemplazado.content.en).toEqual([
      { text: 'Heading' },
    ]);
    const botonConservado = encontrarNodo(conservado, boton.id)?.nodo;
    expect(botonConservado?.type === 'button' && botonConservado.label.en).toBe('Start');
    const menuConservado = encontrarNodo(conservado, 'menu000001')?.nodo;
    const menuReemplazado = encontrarNodo(reemplazado, 'menu000001')?.nodo;
    expect(menuConservado?.type === 'menu' && menuConservado.items[0]?.label.en).toBe(
      'Manual menu',
    );
    expect(
      menuConservado?.type === 'menu' && menuConservado.items[0]?.children?.[0]?.label.en,
    ).toBe('Manual child');
    expect(menuReemplazado?.type === 'menu' && menuReemplazado.items[0]?.label.en).toBe('Products');
    expect(
      menuReemplazado?.type === 'menu' && menuReemplazado.items[0]?.children?.[0]?.label.en,
    ).toBe('Offers');
    expect(reemplazado.pages[0]?.title.en).toBe('Home');
    expect(reemplazado.seo.site_name.en).toBe('Site');
    expect(ir.pages[0]?.title.en).toBe('Título manual');
  });

  it('activa el destino validado de un sitio monolingüe y deja un borrador guardable', () => {
    const ir = crearSitioPrueba();
    ir.languages.others = [];
    delete ir.pages[0]!.slug.en;
    ir.pages.push({
      id: 'pag0000002',
      slug: { es: 'ofertas' },
      title: { es: 'Ofertas' },
      description: { es: 'Nuestras ofertas' },
      sections: [],
      show_header: true,
      show_footer: true,
    });
    expect(validar(ir, { modo: 'borrador' }).filter((p) => p.severity === 'error')).toEqual([]);
    const traducido = aplicarTraducciones(
      ir,
      {
        'pag0000001/title': { en: 'Home', es: 'Unrequested source overwrite', fr: 'Accueil' },
        'txt0000002/content': { en: 'Heading' },
      },
      false,
      ['en'],
    );
    expect(traducido.languages).toEqual({ default: 'es', others: ['en'] });
    expect(traducido.pages[0]?.title).toEqual({ es: 'Inicio', en: 'Home' });
    expect(traducido.pages[0]?.slug).toEqual({ es: '', en: '' });
    expect(traducido.pages[1]?.slug).toEqual({ es: 'ofertas', en: 'ofertas' });
    expect(traducido.pages[1]?.title).toEqual({ es: 'Ofertas' });
    expect(validar(traducido, { modo: 'borrador' }).filter((p) => p.severity === 'error')).toEqual(
      [],
    );
    expect(ir.languages.others).toEqual([]);
    expect(ir.pages[0]?.slug.en).toBeUndefined();
  });
});
