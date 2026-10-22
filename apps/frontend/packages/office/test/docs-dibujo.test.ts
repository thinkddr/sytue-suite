/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test del bloque «Dibujo» de Docus (src/editor/dibujo.ts y dibujo/estado.ts):
 * horneado del <figure> con la escena serializada, el SVG regenerado y la imagen de
 * respaldo; lectura saneada (nunca se confía en el SVG ni en el JSON del HTML);
 * historial de deshacer y nodos de cada herramienta.
 * Ejecutar: cd apps/frontend/packages/office && bun test test/docs-dibujo.test.ts
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { agregarNodo, eliminarNodos, escenaVacia, serializar } from '@sytue/escena';
import type { Escena } from '@sytue/escena';
import {
  DIBUJO_ANCHO_EXPORT,
  anclaDibujo,
  asegurarIdDibujo,
  dibujoFigureHtml,
  estadoAnclaDibujo,
  leerAnclaDibujo,
  exportar,
  hornearFigura,
  leerFigura,
  respaldoDe,
  tamanoExportado,
} from '../src/editor/dibujo';
import {
  confirmar,
  deshacer,
  nodoNuevo,
  nombreConOrdinal,
  nuevoHistorial,
  rehacer,
} from '../src/editor/dibujo/estado';

if (typeof document === 'undefined') GlobalRegistrator.register();

const PNG =
  'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==';

function escena(): Escena {
  let e: Escena = {
    ...escenaVacia({ ancho: 800, alto: 400 }),
    titulo: 'Flujo de alta',
    descripcion: 'Tres pasos',
  };
  e = agregarNodo(e, 'capa1', {
    id: 'a',
    tipo: 'forma',
    forma: 'rect',
    x: 10,
    y: 10,
    w: 100,
    h: 50,
    texto: '<b>"Inicio"</b> & fin',
  });
  return e;
}

function figura(html: string): HTMLElement {
  const host = document.createElement('div');
  host.innerHTML = html;
  return host.querySelector('figure') as HTMLElement;
}

test('la figura lleva la escena serializada, el SVG y el respaldo; es un bloque atómico enfocable', () => {
  const fig = figura(dibujoFigureHtml(escena(), { etiqueta: 'Dibujo', respaldo: PNG }));
  expect(fig.hasAttribute('data-dibujo')).toBe(true);
  expect(fig.getAttribute('contenteditable')).toBe('false');
  expect(fig.getAttribute('tabindex')).toBe('0');
  expect(fig.getAttribute('data-escena')).toBe(serializar(escena()));
  expect(fig.querySelector('svg')?.getAttribute('aria-label')).toBe('Flujo de alta');
  const img = fig.querySelector('img[data-dibujo-respaldo]');
  expect(img?.hasAttribute('hidden')).toBe(true);
  expect(img?.getAttribute('alt')).toBe('Tres pasos');
  // 800 de ancho se reduce a 600 en el fichero exportado, con la misma proporción.
  expect(img?.getAttribute('data-w')).toBe(String(DIBUJO_ANCHO_EXPORT));
  expect(img?.getAttribute('data-h')).toBe('300');
});

test('el texto del usuario va escapado en atributos y SVG (anti-XSS)', () => {
  const html = dibujoFigureHtml(escena(), { etiqueta: 'Dibujo', tmpId: 'x"><script>' });
  expect(html).not.toContain('<script>');
  expect(html).not.toContain('<b>"Inicio"');
  const fig = figura(html);
  expect(fig.querySelector('script')).toBeNull();
  expect(fig.textContent).toContain('<b>"Inicio"</b> & fin');
});

test('leer la figura devuelve la escena saneada; un JSON hostil o ajeno no pasa', () => {
  const ok = leerFigura(figura(dibujoFigureHtml(escena(), { etiqueta: 'Dibujo' })));
  expect(ok?.ok).toBe(true);
  expect(ok?.ok && ok.escena.capas[0]!.nodos).toHaveLength(1);

  const roto = figura('<figure data-dibujo data-escena="{no"></figure>');
  expect(leerFigura(roto)).toEqual({ ok: false, motivo: 'json' });
  const futuro = figura('<figure data-dibujo data-escena=\'{"v":99,"capas":[]}\'></figure>');
  expect(leerFigura(futuro)).toEqual({ ok: false, motivo: 'version-futura' });
  expect(leerFigura(figura('<figure></figure>'))).toBeUndefined();

  const hostil = figura(
    `<figure data-dibujo data-escena='${JSON.stringify({
      v: 1,
      capas: [
        {
          id: 'c',
          nodos: [
            {
              id: 'i',
              tipo: 'imagen',
              x: 0,
              y: 0,
              w: 9,
              h: 9,
              href: 'javascript:alert(1)',
              alt: '',
            },
          ],
        },
      ],
    })}'></figure>`,
  );
  const l = leerFigura(hostil);
  expect(l?.ok && l.escena.capas[0]!.nodos).toHaveLength(0);
});

test('hornear regenera el SVG desde la escena: el SVG que venía en el HTML no se conserva', () => {
  const fig = figura(
    `<figure data-dibujo data-escena="{}"><svg onload="alert(1)"><script>x</script></svg>` +
      `<img data-dibujo-respaldo src="${PNG}"></figure>`,
  );
  hornearFigura(fig, escena(), { etiqueta: 'Dibujo', respaldo: undefined });
  expect(fig.querySelector('script')).toBeNull();
  expect(fig.innerHTML).not.toContain('onload');
  expect(respaldoDe(fig)).toBeUndefined(); // sin respaldo nuevo, el viejo NO se conserva
  expect(fig.getAttribute('data-escena')).toBe(serializar(escena()));
  hornearFigura(fig, escena(), { etiqueta: 'Dibujo', respaldo: PNG });
  expect(respaldoDe(fig)).toBe(PNG);
});

test('el respaldo solo acepta un PNG en data URL', () => {
  const html = dibujoFigureHtml(escena(), {
    etiqueta: 'Dibujo',
    respaldo: 'https://evil.example/x.png',
  });
  expect(html).not.toContain('data-dibujo-respaldo');
  const js = dibujoFigureHtml(escena(), {
    etiqueta: 'Dibujo',
    respaldo: 'data:image/svg+xml;base64,PHN2Zz4=',
  });
  expect(js).not.toContain('data-dibujo-respaldo');
  expect(
    respaldoDe(
      figura('<figure data-dibujo><img data-dibujo-respaldo src="javascript:1"></figure>'),
    ),
  ).toBeUndefined();
});

test('tamaño exportado: no crece y respeta la proporción', () => {
  expect(tamanoExportado(escenaVacia({ ancho: 300, alto: 150 }))).toEqual({ w: 300, h: 150 });
  expect(tamanoExportado(escenaVacia({ ancho: 1200, alto: 900 }))).toEqual({ w: 600, h: 450 });
});

test('historial: confirmar, deshacer y rehacer; una operación sin efecto no abre paso', () => {
  const a = escenaVacia();
  const b = agregarNodo(a, 'capa1', nodoNuevo('rect', a, 'Rectángulo', 50, 50));
  let h = nuevoHistorial(a);
  h = confirmar(h, b);
  expect(h.pasado).toEqual([a]);
  expect(confirmar(h, b)).toBe(h);
  h = deshacer(h);
  expect(h.presente).toBe(a);
  expect(deshacer(h)).toBe(h);
  h = rehacer(h);
  expect(h.presente).toBe(b);
  expect(rehacer(h)).toBe(h);
  // Una edición nueva tras deshacer descarta lo rehacible.
  h = deshacer(h);
  h = confirmar(h, escenaVacia({ ancho: 100 }));
  expect(h.futuro).toEqual([]);
});

test('historial: tope de pasos', () => {
  let h = nuevoHistorial(escenaVacia());
  for (let i = 0; i < 150; i++) h = confirmar(h, escenaVacia({ ancho: 200 + i }));
  expect(h.pasado).toHaveLength(100);
});

test('nodoNuevo: un clic da tamaño por defecto centrado; arrastrar da la caja arrastrada', () => {
  const e = escenaVacia();
  const clic = nodoNuevo('ellipse', e, 'Elipse', 200, 100);
  expect(clic).toMatchObject({
    tipo: 'forma',
    forma: 'ellipse',
    nombre: 'Elipse 1',
    x: 130,
    y: 55,
    w: 140,
    h: 90,
  });
  const arr = nodoNuevo('rect', e, 'Rectángulo', 300, 200, 100, 50);
  expect(arr).toMatchObject({ x: 100, y: 50, w: 200, h: 150 }); // de cualquier esquina a cualquier otra
  const flecha = nodoNuevo('flecha', e, 'Flecha', 10, 10, 90, 10);
  expect(flecha).toMatchObject({ tipo: 'linea', x1: 10, x2: 90, flechaFin: true });
  const texto = nodoNuevo('texto', e, 'Texto', 50, 50);
  expect(texto).toMatchObject({ tipo: 'texto', texto: 'Texto', h: 32 });
});

test('nombreConOrdinal salta los nombres ya usados', () => {
  let e = escenaVacia();
  expect(nombreConOrdinal('Elipse', e)).toBe('Elipse 1');
  e = agregarNodo(e, 'capa1', { ...nodoNuevo('ellipse', e, 'Elipse', 0, 0) });
  expect(nombreConOrdinal('Elipse', e)).toBe('Elipse 2');
});

test('exportar: SVG autónomo a tamaño real y PDF con nombre seguro', () => {
  const e = { ...escena(), titulo: 'Flujo de alta/ñandú' };
  const svg = exportar(e, 'svg');
  expect(svg.nombre).toBe('Flujo-de-alta-nandu.svg');
  expect(svg.tipo).toBe('image/svg+xml');
  expect(String(svg.datos)).toContain('width="800" height="400"');
  expect(String(svg.datos)).not.toContain('data-id');
  const pdf = exportar(escena(), 'pdf');
  expect(pdf.nombre).toBe('Flujo-de-alta.pdf');
  expect(String.fromCharCode(...(pdf.datos as Uint8Array).slice(0, 8))).toBe('%PDF-1.4');
});

test('comentarios: el dibujo lleva un id estable que se conserva al re-hornear', () => {
  const fig = figura(dibujoFigureHtml(escena(), { etiqueta: 'Dibujo', id: 'dib-abc' }));
  expect(fig.getAttribute('data-dibujo-id')).toBe('dib-abc');
  hornearFigura(fig, escena(), { etiqueta: 'Dibujo' });
  expect(fig.getAttribute('data-dibujo-id')).toBe('dib-abc');
  expect(asegurarIdDibujo(fig)).toEqual({ id: 'dib-abc', nuevo: false });
  const vieja = figura(dibujoFigureHtml(escena(), { etiqueta: 'Dibujo' }));
  const dado = asegurarIdDibujo(vieja);
  expect(dado.nuevo).toBe(true);
  expect(vieja.getAttribute('data-dibujo-id')).toBe(dado.id);
});

test('comentarios: el ancla guarda el id de la forma o la coordenada, y se lee con rigor', () => {
  expect(anclaDibujo('dib-1', { nodo: 'a', nombre: 'Rectángulo 1' })).toEqual({
    dibujo: 'dib-1',
    nodo: 'a',
    text: 'Rectángulo 1',
  });
  expect(anclaDibujo('dib-1', { x: 10.04, y: 20.06 })).toEqual({
    dibujo: 'dib-1',
    x: 10,
    y: 20.1,
    text: '10, 20.1',
  });
  expect(leerAnclaDibujo({ dibujo: 'dib-1', nodo: 'a', text: 'x' })).toEqual({
    dibujo: 'dib-1',
    nodo: 'a',
    text: 'x',
  });
  expect(leerAnclaDibujo({ mark: 'm1', text: 'hola' })).toBeNull(); // un comentario de texto
  expect(leerAnclaDibujo({ dibujo: '<x>' })).toBeNull();
  expect(leerAnclaDibujo({ dibujo: 'd', x: 'a', y: 1 })).toEqual({ dibujo: 'd' });
  expect(leerAnclaDibujo(null)).toBeNull();
  expect(leerAnclaDibujo('dib')).toBeNull();
});

test('comentarios: si se borra la forma o el dibujo, el comentario queda huérfano (y se conserva)', () => {
  const raiz = document.createElement('div');
  raiz.innerHTML = dibujoFigureHtml(escena(), { etiqueta: 'Dibujo', id: 'dib-1' });
  const ancla = { dibujo: 'dib-1', nodo: 'a' };
  expect(estadoAnclaDibujo(raiz, ancla).huerfano).toBe(false);
  expect(estadoAnclaDibujo(raiz, { dibujo: 'dib-1', x: 5, y: 5 }).huerfano).toBe(false);
  // Mover la forma no rompe el ancla: va por id.
  const f = raiz.querySelector('figure') as HTMLElement;
  const movida = escena();
  hornearFigura(f, movida, { etiqueta: 'Dibujo' });
  expect(estadoAnclaDibujo(raiz, ancla).huerfano).toBe(false);
  // Borrar la forma: huérfano, pero el dibujo sigue (hay a dónde ir).
  hornearFigura(f, eliminarNodos(escena(), ['a']), { etiqueta: 'Dibujo' });
  const st = estadoAnclaDibujo(raiz, ancla);
  expect(st.huerfano).toBe(true);
  expect(st.figura).toBe(f);
  // La coordenada sigue valiendo aunque la forma no exista.
  expect(estadoAnclaDibujo(raiz, { dibujo: 'dib-1', x: 5, y: 5 }).huerfano).toBe(false);
  // Borrar el dibujo entero.
  f.remove();
  expect(estadoAnclaDibujo(raiz, ancla)).toEqual({ figura: null, huerfano: true });
  expect(estadoAnclaDibujo(raiz, { dibujo: 'dib-1', x: 5, y: 5 }).huerfano).toBe(true);
});
