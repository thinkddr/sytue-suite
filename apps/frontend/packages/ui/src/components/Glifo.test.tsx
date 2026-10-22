/**
 * Glifo — el envoltorio común de los iconos de interfaz: el grosor compensado y
 * la capa de relleno. Lo que se comprueba es el MARCADO, que es donde vive el
 * estilo: un fallo aquí no rompe nada, solo pinta los iconos de otra manera.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render, type ComponentChild } from 'preact';
import { Badge } from './Badge.js';
import { Glifo, grosorDeGlifo, puntos, subtrazosCerrados } from './Glifo.js';
import { Icon } from './Icon.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

function pinta(vnode: ComponentChild): SVGSVGElement {
  const host = document.createElement('div');
  render(vnode, host);
  return host.querySelector('svg') as SVGSVGElement;
}

/** La capa de relleno: el `<path>` sin trazo que va debajo del dibujo. */
const relleno = (svg: SVGSVGElement): string | null =>
  svg.querySelector('path[stroke="none"]')?.getAttribute('d') ?? null;

test('el trazo mide 1,5 px en pantalla a cualquier tamaño', () => {
  // grosor en la rejilla × px por unidad de rejilla = px en pantalla.
  for (const px of [12, 14, 16, 18, 20, 24, 32, 48]) {
    expect((grosorDeGlifo(px) * px) / 24).toBeCloseTo(1.5, 1);
  }
  expect(pinta(<Glifo d="M4 4h16" size={16} />).getAttribute('stroke-width')).toBe('2.25');
  expect(pinta(<Glifo d="M4 4h16" size={24} />).getAttribute('stroke-width')).toBe('1.5');
});

test('solo se rellenan los subtrazos cerrados', () => {
  // La lupa: el aro se cierra, el mango no.
  expect(subtrazosCerrados('M11 4a7 7 0 1 0 0 14 7 7 0 0 0 0-14ZM20 20l-3.8-3.8')).toBe(
    'M11 4a7 7 0 1 0 0 14 7 7 0 0 0 0-14Z',
  );
  // Línea pura: nada que rellenar, y por tanto ninguna capa.
  expect(subtrazosCerrados('M5 8.5l7 7 7-7')).toBe('');
  expect(relleno(pinta(<Icon name="chevronDown" />))).toBeNull();
  // Los separadores con espacio de la cinta de Docs («…Z M…») valen igual.
  expect(subtrazosCerrados('M7 7h10v10H7Z M4 12h1')).toBe('M7 7h10v10H7Z');
  // El escudo: tras la `M` vienen pares sueltos (líneas absolutas), que no se pegan a ella.
  expect(subtrazosCerrados('M12 3.5 4.5 6.6v5.4L12 3.5Z')).toBe('M12 3.5 4.5 6.6v5.4L12 3.5Z');
});

test('una `m` relativa se vuelve absoluta al sacarla de su sitio', () => {
  // La que ABRE el trazado es absoluta, y sus pares siguientes son líneas relativas.
  expect(subtrazosCerrados('m12 3 2.9 5.9-5.8 0Z')).toBe('M12 3l2.9 5.9-5.8 0Z');
  // Tras un `Z` la pluma vuelve al inicio del subtrazo cerrado: (6,15) + (12,7).
  expect(
    subtrazosCerrados('M6 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6Zm12 7a3 3 0 1 0 0-6 3 3 0 0 0 0 6Z'),
  ).toBe('M6 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6Z M18 22a3 3 0 1 0 0-6 3 3 0 0 0 0 6Z');
  // El sobre de Mail: cuerpo cerrado y solapa abierta con `m` relativa. Solo el cuerpo.
  expect(subtrazosCerrados('M4 5h16v14H4Zm-1 2 9 6 9-6')).toBe('M4 5h16v14H4Z');
  // Tras un subtrazo ABIERTO no se sabe dónde está la pluma: se queda sin relleno
  // (el techo documentado; `glifos-al-comun.test.ts` vigila que nadie lo pise).
  expect(subtrazosCerrados('M4 4h6m2 2h4v4h-4Z')).toBe('');
});

test('la capa de relleno va DEBAJO del trazo, en par-impar y con la variable de acento', () => {
  const svg = pinta(<Icon name="ojo" />);
  const [capa, trazo] = [...svg.querySelectorAll('path')];
  expect(capa?.getAttribute('stroke')).toBe('none');
  expect(capa?.getAttribute('fill')).toBe('var(--sytue-glyph-fill, var(--sytue-primary))');
  expect(capa?.getAttribute('fill-opacity')).toBe('0.34');
  // Par-impar: la pupila, que cae dentro del ojo, queda como hueco.
  expect(capa?.getAttribute('fill-rule')).toBe('evenodd');
  // El trazo es el dibujo ENTERO y sin tocar: no se redibuja nada.
  expect(trazo?.getAttribute('d')).toContain('M2.5 12s3.5-6.5 9.5-6.5');
  expect(trazo?.getAttribute('fill')).toBeNull();
});

test('con `fill` propio la forma va llena y no lleva capa de acento', () => {
  // El corazón de «favorita» en Photos y la estrella de Mail.
  const svg = pinta(<Icon name="corazon" fill="currentColor" />);
  expect(svg.getAttribute('fill')).toBe('currentColor');
  expect(relleno(svg)).toBeNull();
  expect(svg.querySelectorAll('path').length).toBe(1);
});

test('un `fill` indefinido NO borra el `fill="none"`', () => {
  // Es como lo pasan Mail, Rooms y Contacts: `fill={marcada ? 'currentColor' : undefined}`.
  // Si ese `undefined` llega al `<svg>` se queda sin atributo y SVG lo rellena de negro:
  // todas las estrellas sin marcar salían macizas.
  const sinMarcar = pinta(
    <Glifo d="M12 3l3 6 6 1-4.5 4 1 6-5.5-3-5.5 3 1-6L3 10l6-1Z" fill={undefined} />,
  );
  expect(sinMarcar.getAttribute('fill')).toBe('none');
  expect(relleno(sinMarcar)).not.toBeNull();
});

test('las figuras hijas se rellenan como un `d`, en un solo trazado', () => {
  // El ojo de Pages: contorno + pupila en dos figuras. Un trazado, para que el
  // par-impar abra el hueco igual que en el `ojo` del catálogo compartido.
  const svg = pinta(
    <Glifo>
      <path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z" />
      <circle cx="12" cy="12" r="3" />
    </Glifo>,
  );
  expect(relleno(svg)).toBe(
    'M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z M9 12a3 3 0 1 0 6 0a3 3 0 1 0 -6 0Z',
  );
  // El dibujo sigue siendo el suyo: las figuras, tal cual.
  expect(svg.querySelector('circle')?.getAttribute('r')).toBe('3');
});

test('un rect con esquinas, un polígono y lo que no se rellena', () => {
  const svg = pinta(
    <Glifo>
      <rect x="9" y="9" width="11" height="11" rx="2" />
      <polygon points="12 2 2 7 12 12 22 7 12 2" />
      <polyline points="2 17 12 22 22 17" />
      <line x1="4" y1="4" x2="8" y2="8" />
      {/* Con relleno propio ya dice cómo se pinta: el punto de «no leído». */}
      <circle cx="19" cy="6" r="2.4" fill="currentColor" stroke="none" />
    </Glifo>,
  );
  expect(relleno(svg)).toBe(
    'M11 9h7a2 2 0 0 1 2 2v7a2 2 0 0 1 -2 2h-7a2 2 0 0 1 -2 -2v-7a2 2 0 0 1 2 -2Z M12 2 2 7 12 12 22 7 12 2Z',
  );
});

test('decorativo por defecto; con etiqueta es una imagen con nombre', () => {
  expect(pinta(<Glifo d="M4 4h16" />).getAttribute('aria-hidden')).toBe('true');
  const con = pinta(<Glifo d="M4 4h16" label="Cerrar" />);
  expect(con.getAttribute('role')).toBe('img');
  expect(con.getAttribute('aria-label')).toBe('Cerrar');
  expect(con.getAttribute('aria-hidden')).toBeNull();
});

test('los puntos del dibujo van macizos y con tamaño propio', () => {
  // Los tres de «…»: un círculo de radio 1,25 en cada sitio, 2,1 px a 20 y 2,5 a 24.
  expect(puntos('M6 12h.01M12 12h.01')).toBe(
    'M4.75 12a1.25 1.25 0 1 0 2.5 0a1.25 1.25 0 1 0 -2.5 0Z M10.75 12a1.25 1.25 0 1 0 2.5 0a1.25 1.25 0 1 0 -2.5 0Z',
  );
  // Un tramo de verdad no es un punto.
  expect(puntos('M4 6h16M4 12h.5')).toBe('');
  const svg = pinta(<Icon name="ellipsis" />);
  const mota = svg.querySelector('path[fill="currentColor"]');
  expect(mota?.getAttribute('stroke')).toBe('none');
  expect(mota?.getAttribute('d')?.split('Z').filter(Boolean).length).toBe(3);
  // También en un glifo escrito con figuras (la cara de Rooms, la etiqueta de Contacts).
  const conHijos = pinta(
    <Glifo>
      <path d="M20.6 13.4 12 22l-9-9V3h10l7.6 7.6a2 2 0 0 1 0 2.8Z" />
      <path d="M7.5 7.5h.01" />
    </Glifo>,
  );
  expect(conHijos.querySelector('path[fill="currentColor"]')?.getAttribute('d')).toBe(
    'M6.25 7.5a1.25 1.25 0 1 0 2.5 0a1.25 1.25 0 1 0 -2.5 0Z',
  );
  // Un glifo sin puntos no lleva la capa.
  expect(pinta(<Icon name="check" />).querySelector('path[fill="currentColor"]')).toBeNull();
});

test('con tinta de color, el relleno sigue a la tinta', async () => {
  // Decisión del dueño (2026-10-04): un icono rojo de peligro o verde de éxito sobre
  // fondo neutro lleva el relleno de SU color, no el de marca. Lo hace la regla de
  // `styles.css`, que reconoce la tinta por su utilidad; aquí se ata a las clases que
  // de verdad escriben los componentes, que es por donde se rompería.
  const css = await Bun.file(new URL('../styles.css', import.meta.url)).text();
  const hasta = css.indexOf('--sytue-glyph-fill: currentColor');
  const selector = css.slice(css.lastIndexOf(':where(', hasta), hasta);
  for (const tinta of ['danger', 'success', 'warning', 'info']) {
    expect(selector).toContain(`[class*='text-${tinta}']`);
  }
  expect(selector).toContain('.sytue-glifo-tinta');
  const clases = (tone: 'danger' | 'success' | 'warning' | 'info' | 'neutral'): string => {
    const host = document.createElement('div');
    render(<Badge tone={tone}>x</Badge>, host);
    return host.querySelector('span')?.className ?? '';
  };
  for (const tone of ['danger', 'success', 'warning', 'info'] as const) {
    expect(clases(tone)).toContain(`text-${tone}`);
  }
  // La tinta neutra NO está en la regla: conserva el relleno de marca.
  for (const neutra of ['text-text', 'text-muted', 'text-subtle', 'text-primary']) {
    expect(selector).not.toContain(neutra);
  }
  expect(clases('neutral')).not.toMatch(/text-(danger|success|warning|info)/);
});
