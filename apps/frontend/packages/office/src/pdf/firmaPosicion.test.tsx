/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * DÓNDE VA LA FIRMA VISIBLE.
 *
 * Lo que se fija: que quien no toca nada firma donde siempre (abajo a la
 * izquierda, [56 56 296 130] en un A4); que cada posición cae dentro del papel
 * en un A4, un A5, una apaisada, un tique y una MediaBox que no empieza en 0,0;
 * que «firmar en todas» se calcula sobre la zona común; y que lo que se ENVÍA al
 * motor —las tres vías que dejan marca— lleva el recuadro elegido.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, describe, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { docsT } from '../i18n';
import type { Pagina } from './api';
import {
  ALTO_FIRMA,
  ANCHO_FIRMA,
  ANCHO_MINIMO_FIRMA,
  POSICIONES_FIRMA,
  aFracciones,
  cajaComun,
  cajaParaFirmar,
  deFracciones,
  encajar,
  escalarRecuadro,
  moverRecuadro,
  recuadroArrastrado,
  rectElegido,
  rectFirma,
  type Rect,
} from './firmaPosicion';
import { Firmar } from './Paneles';

if (typeof document === 'undefined') GlobalRegistrator.register();

const A4: Rect = [0, 0, 595, 842];
const A5: Rect = [0, 0, 420, 595];
const A4_APAISADA: Rect = [0, 0, 842, 595];
const TIQUE: Rect = [0, 0, 160, 400];
const DESPLAZADA: Rect = [100, 200, 712, 992];

function dentro(r: Rect, caja: Rect): boolean {
  return r[0] >= caja[0] && r[1] >= caja[1] && r[2] <= caja[2] && r[3] <= caja[3];
}

describe('rectFirma', () => {
  test('por defecto, abajo a la izquierda y el recuadro de siempre', () => {
    expect(POSICIONES_FIRMA[0]).toBe('abajoIzquierda');
    expect(rectFirma('abajoIzquierda', A4)).toEqual([56, 56, 296, 130]);
    // Sin saber aún cómo es la página, lo mismo que antes.
    expect(rectFirma('abajoIzquierda')).toEqual([56, 56, 296, 130]);
    // En un A5 o una apaisada cabe con el margen entero: no se mueve.
    expect(rectFirma('abajoIzquierda', A5)).toEqual([56, 56, 296, 130]);
    expect(rectFirma('abajoIzquierda', A4_APAISADA)).toEqual([56, 56, 296, 130]);
  });

  test('cada posición cae donde dice, con el margen de siempre', () => {
    expect(rectFirma('abajoCentro', A4)).toEqual([177.5, 56, 417.5, 130]);
    expect(rectFirma('abajoDerecha', A4)).toEqual([299, 56, 539, 130]);
    expect(rectFirma('arribaDerecha', A4)).toEqual([299, 712, 539, 786]);
    expect(rectFirma('abajoDerecha', A4_APAISADA)).toEqual([546, 56, 786, 130]);
    expect(rectFirma('arribaDerecha', A5)).toEqual([124, 465, 364, 539]);
  });

  test('ninguna posición se sale del papel, sea cual sea la hoja', () => {
    for (const caja of [A4, A5, A4_APAISADA, TIQUE, DESPLAZADA]) {
      for (const p of POSICIONES_FIRMA) {
        const r = rectFirma(p, caja);
        expect(dentro(r, caja)).toBe(true);
        expect(r[2] - r[0]).toBeGreaterThan(0);
      }
    }
  });

  test('en una hoja estrecha se escala sin deformar la rúbrica', () => {
    const r = rectFirma('abajoDerecha', TIQUE);
    const proporcion = (r[2] - r[0]) / (r[3] - r[1]);
    expect(proporcion).toBeCloseTo(ANCHO_FIRMA / ALTO_FIRMA, 1);
    expect(r[2] - r[0]).toBeLessThan(ANCHO_FIRMA);
  });

  test('una MediaBox que no empieza en 0,0 se respeta', () => {
    expect(rectFirma('abajoIzquierda', DESPLAZADA)).toEqual([156, 256, 396, 330]);
    expect(rectFirma('arribaDerecha', DESPLAZADA)).toEqual([416, 862, 656, 936]);
  });
});

describe('cajaParaFirmar', () => {
  const paginas = [{ caja: A4 }, { caja: A5 }];
  test('la de la página elegida, o la primera si no existe', () => {
    expect(cajaParaFirmar(paginas, 2, false)).toEqual(A5);
    expect(cajaParaFirmar(paginas, 9, false)).toEqual(A4);
    expect(cajaParaFirmar([], 1, false)).toBeUndefined();
  });
  test('en todas, la zona que comparten: no se sale de la más pequeña', () => {
    expect(cajaComun(paginas)).toEqual([0, 0, 420, 595]);
    const r = rectFirma('arribaDerecha', cajaParaFirmar(paginas, 1, true));
    expect(dentro(r, A5)).toBe(true);
  });
});

describe('recuadro dibujado', () => {
  const proporcion = (r: Rect) => (r[2] - r[0]) / (r[3] - r[1]);

  test('arrastrar deja un recuadro con la proporción de la rúbrica, desde donde se empezó', () => {
    const r = recuadroArrastrado(A4, [100, 400], [340, 420]);
    expect(r).toEqual([100, 400, 340, 474]);
    expect(proporcion(r)).toBeCloseTo(ANCHO_FIRMA / ALTO_FIRMA, 1);
    // Hacia atrás y hacia abajo también: el punto de partida es la otra esquina.
    expect(recuadroArrastrado(A4, [340, 474], [100, 470])).toEqual([100, 400, 340, 474]);
  });

  test('un clic suelto deja un recuadro usable, no uno de cero', () => {
    const r = recuadroArrastrado(A4, [200, 200], [200, 200]);
    expect(r[2] - r[0]).toBe(ANCHO_MINIMO_FIRMA);
  });

  test('nunca se sale del papel, ni arrastrando fuera ni en una MediaBox desplazada', () => {
    expect(dentro(recuadroArrastrado(A4, [500, 800], [900, 900]), A4)).toBe(true);
    expect(dentro(recuadroArrastrado(TIQUE, [0, 0], [1000, 0]), TIQUE)).toBe(true);
    expect(dentro(recuadroArrastrado(DESPLAZADA, [110, 210], [0, 0]), DESPLAZADA)).toBe(true);
    expect(dentro(moverRecuadro(A4, [56, 56, 296, 130], -500, 9000), A4)).toBe(true);
    expect(dentro(escalarRecuadro(A4, [300, 700, 540, 774], 10), A4)).toBe(true);
    expect(encajar(A4, [-10, -10, 230, 64])).toEqual([0, 0, 240, 74]);
  });

  test('encoger no baja del mínimo y conserva el centro', () => {
    const r = escalarRecuadro(A4, [100, 100, 340, 174], 0.01);
    expect(r[2] - r[0]).toBe(ANCHO_MINIMO_FIRMA);
    expect((r[0] + r[2]) / 2).toBeCloseTo(220, 0);
  });

  test('en fracciones sigue en el mismo sitio de la hoja aunque cambie la página', () => {
    const r: Rect = [297.5, 421, 417.5, 458];
    const f = aFracciones(A4, r);
    expect(deFracciones(A4, f)).toEqual(r);
    // En un A5 cae en la misma zona relativa y dentro del papel.
    const enA5 = deFracciones(A5, f);
    expect(enA5[0]).toBeCloseTo(210, 0);
    expect(dentro(enA5, A5)).toBe(true);
    expect(proporcion(enA5)).toBeCloseTo(ANCHO_FIRMA / ALTO_FIRMA, 1);
  });

  test('sin dibujar todavía, sale donde siempre', () => {
    expect(rectElegido('dibujada', A4, null)).toEqual([56, 56, 296, 130]);
    expect(rectElegido('abajoDerecha', A4, null)).toEqual(rectFirma('abajoDerecha', A4));
  });
});

/* -------------------------------------------------------------------------- */
/*  Lo que se envía                                                           */
/* -------------------------------------------------------------------------- */

const t = docsT('es');
let host: HTMLElement;
let enviados: Record<string, unknown>[];

const pagina = (numero: number, caja: Rect): Pagina => ({
  numero,
  ancho: caja[2] - caja[0],
  alto: caja[3] - caja[1],
  rotacion: 0,
  caja,
});

beforeEach(() => {
  host = document.createElement('div');
  document.body.appendChild(host);
  enviados = [];
});
afterEach(() => {
  act(() => render(null, host));
  host.remove();
});

function montar(paginas: Pagina[]): void {
  act(() => {
    render(
      <Firmar
        t={t}
        path="/contrato.pdf"
        paginas={paginas}
        svgs={{}}
        onAplicar={(cuerpo) => {
          enviados.push(cuerpo);
          return Promise.resolve(true);
        }}
        onCerrar={() => undefined}
        trabajando={false}
      />,
      host,
    );
  });
}

/** Un trazo en el lienzo: basta para que «Aplicar» tenga algo que firmar. */
function dibujar(): void {
  const lienzo = host.querySelector('canvas')!;
  lienzo.setPointerCapture = () => undefined;
  // happy-dom no maqueta: el lienzo mide lo que mide su resolución interna.
  lienzo.getBoundingClientRect = () => ({ left: 0, top: 0, width: 480, height: 148 }) as DOMRect;
  act(() => {
    lienzo.dispatchEvent(new PointerEvent('pointerdown', { clientX: 10, clientY: 10 }));
  });
  act(() => {
    lienzo.dispatchEvent(new PointerEvent('pointermove', { clientX: 40, clientY: 30 }));
  });
}

function elegir(posicion: string): void {
  const radio = host.querySelector<HTMLInputElement>(
    `input[name="posicion-firma"][value="${posicion}"]`,
  )!;
  act(() => {
    radio.click();
  });
}

function enviar(): Record<string, unknown> {
  act(() => {
    host.querySelector('form')!.dispatchEvent(new Event('submit', { cancelable: true }));
  });
  expect(enviados).toHaveLength(1);
  return enviados[0]!;
}

test('quien no toca nada firma donde siempre', () => {
  montar([pagina(1, A4)]);
  dibujar();
  const cuerpo = enviar() as { op: string; datos: { rect: Rect; trazos: [number, number][][] } };
  expect(cuerpo.op).toBe('firmar');
  expect(cuerpo.datos.rect).toEqual([56, 56, 296, 130]);
  // Los trazos caen dentro de ESE recuadro.
  for (const [x, y] of cuerpo.datos.trazos.flat()) {
    expect(x).toBeGreaterThanOrEqual(56);
    expect(x).toBeLessThanOrEqual(296);
    expect(y).toBeGreaterThanOrEqual(56);
    expect(y).toBeLessThanOrEqual(130);
  }
});

test('la posición elegida viaja en el cuerpo, con el tamaño de la página', () => {
  montar([pagina(1, A4_APAISADA)]);
  dibujar();
  elegir('arribaDerecha');
  const cuerpo = enviar() as { datos: { rect: Rect } };
  expect(cuerpo.datos.rect).toEqual(rectFirma('arribaDerecha', A4_APAISADA));
  expect(cuerpo.datos.rect).toEqual([546, 465, 786, 539]);
});

test('con certificado, el mismo recuadro va en `Rect`', () => {
  montar([pagina(1, A4)]);
  const pestana = [...host.querySelectorAll<HTMLButtonElement>('[role="tab"]')].find((b) =>
    b.textContent?.includes(t('pdf.dialog.firmar.modo.certificado')),
  )!;
  act(() => {
    pestana.click();
  });
  elegir('abajoDerecha');
  // Sin certificado cargado «Aplicar» está desactivado; el formulario se envía
  // igualmente para mirar el cuerpo, que es lo que importa aquí.
  const cuerpo = enviar() as { op: string; datos: { Rect: Rect } };
  expect(cuerpo.op).toBe('firmar-certificado');
  expect(cuerpo.datos.Rect).toEqual([299, 56, 539, 130]);
});

test('la miniatura pone el recuadro donde va a caer', () => {
  montar([pagina(1, A4)]);
  elegir('arribaDerecha');
  const recuadro = host.querySelector<HTMLElement>('[data-recuadro-firma]')!;
  // Arriba a la derecha en coordenadas de pantalla: poco «top», mucho «left».
  expect(parseFloat(recuadro.style.left)).toBeGreaterThan(50);
  expect(parseFloat(recuadro.style.top)).toBeLessThan(10);
  expect(host.querySelector('[data-miniatura-firma]')!.getAttribute('aria-hidden')).toBe('true');
});

test('sin firma visible no se ofrece dónde ponerla y el recuadro va a cero', () => {
  montar([pagina(1, A4)]);
  dibujar();
  const visible = host.querySelector<HTMLInputElement>('input[data-firma-visible]')!;
  act(() => {
    visible.click();
  });
  expect(host.querySelector('input[name="posicion-firma"]')).toBeNull();
  const cuerpo = enviar() as { datos: { rect: Rect } };
  expect(cuerpo.datos.rect).toEqual([0, 0, 0, 0]);
});

test('cada opción mide al menos 44 px en el móvil, con cualquier base de letra', () => {
  montar([pagina(1, A4)]);
  const opciones = [...host.querySelectorAll('[data-posiciones-firma] > label')];
  // Las cuatro esquinas y «Dibujar en la página».
  expect(opciones).toHaveLength(5);
  for (const o of opciones) {
    // `min-h-11` son 2.75rem, 38,5 px con la letra a 14: el suelo tiene que ir en px.
    expect(o.className).toContain('max-sm:min-h-[max(44px,2.75rem)]');
    expect(o.className).not.toContain('min-h-11');
  }
});

test('con la aplicación oficial no se ofrece dónde poner la firma', () => {
  montar([pagina(1, A4)]);
  const pestana = [...host.querySelectorAll<HTMLButtonElement>('[role="tab"]')].find((b) =>
    b.textContent?.includes(t('pdf.dialog.firmar.modo.oficial')),
  )!;
  act(() => {
    pestana.click();
  });
  expect(host.querySelector('input[name="posicion-firma"]')).toBeNull();
});

/** El área donde se dibuja, con el tamaño de la página en píxeles (1 px = 1 pt). */
function areaDeDibujo(p: Pagina): HTMLElement {
  const area = host.querySelector<HTMLElement>('[data-dibujar-firma]')!;
  area.setPointerCapture = () => undefined;
  area.getBoundingClientRect = () =>
    ({ left: 0, top: 0, width: p.ancho, height: p.alto }) as DOMRect;
  return area;
}

test('dibujar el recuadro sobre la página lo manda al motor y no borra la rúbrica', () => {
  const p = pagina(1, A4);
  montar([p]);
  dibujar();
  elegir('dibujada');
  const area = areaDeDibujo(p);
  expect(area.getAttribute('aria-describedby')).toBe('ayuda-dibujar-firma');
  // De (100, 300) a (340, 320) en pantalla: en el PDF, con el eje vertical al revés.
  act(() => {
    area.dispatchEvent(new PointerEvent('pointerdown', { clientX: 100, clientY: 300 }));
  });
  act(() => {
    area.dispatchEvent(new PointerEvent('pointermove', { clientX: 340, clientY: 320 }));
  });
  act(() => {
    area.dispatchEvent(new PointerEvent('pointerup', { clientX: 340, clientY: 320 }));
  });
  const esperado = recuadroArrastrado(A4, [100, 542], [340, 522]);
  // Se ve donde se ha dibujado.
  const recuadro = host.querySelector<HTMLElement>('[data-recuadro-dibujado]')!;
  expect(parseFloat(recuadro.style.left)).toBeCloseTo((100 / 595) * 100, 1);
  const cuerpo = enviar() as { datos: { rect: Rect; trazos: [number, number][][] } };
  expect(cuerpo.datos.rect).toEqual(esperado);
  // El trazo del lienzo sigue ahí y cae dentro del recuadro dibujado.
  expect(cuerpo.datos.trazos.flat().length).toBeGreaterThan(0);
  for (const [x, y] of cuerpo.datos.trazos.flat()) {
    expect(x).toBeGreaterThanOrEqual(esperado[0]);
    expect(x).toBeLessThanOrEqual(esperado[2]);
    expect(y).toBeGreaterThanOrEqual(esperado[1]);
    expect(y).toBeLessThanOrEqual(esperado[3]);
  }
});

test('con el teclado, las flechas mueven el recuadro y + lo agranda', () => {
  const p = pagina(1, A4);
  montar([p]);
  dibujar();
  elegir('dibujada');
  const area = areaDeDibujo(p);
  const tecla = (key: string, shiftKey = false) =>
    act(() => {
      area.dispatchEvent(new KeyboardEvent('keydown', { key, shiftKey, bubbles: true }));
    });
  tecla('ArrowRight', true); // un 10 % de 842 a la derecha
  tecla('ArrowUp'); // y un 1 % hacia ARRIBA en pantalla, que en el PDF es sumar
  tecla('+');
  const cuerpo = enviar() as { datos: { rect: Rect } };
  const movido = moverRecuadro(A4, moverRecuadro(A4, [56, 56, 296, 130], 84.2, 0), 0, 8.42);
  // Medio punto de holgura: cada paso se guarda redondeado.
  escalarRecuadro(A4, movido, 1.1).forEach((v, i) =>
    expect(Math.abs(cuerpo.datos.rect[i]! - v)).toBeLessThanOrEqual(0.5),
  );
});

test('el recuadro dibujado se queda al cambiar de pestaña y el certificado lo usa', () => {
  const p = pagina(1, A4);
  montar([p]);
  elegir('dibujada');
  const area = areaDeDibujo(p);
  act(() => {
    area.dispatchEvent(new PointerEvent('pointerdown', { clientX: 300, clientY: 100 }));
  });
  act(() => {
    area.dispatchEvent(new PointerEvent('pointerup', { clientX: 300, clientY: 100 }));
  });
  const pestana = [...host.querySelectorAll<HTMLButtonElement>('[role="tab"]')].find((b) =>
    b.textContent?.includes(t('pdf.dialog.firmar.modo.certificado')),
  )!;
  act(() => {
    pestana.click();
  });
  const cuerpo = enviar() as { datos: { Rect: Rect } };
  expect(cuerpo.datos.Rect).toEqual(recuadroArrastrado(A4, [300, 742], [300, 742]));
});
