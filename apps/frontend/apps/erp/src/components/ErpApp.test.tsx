/* SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { ToastProvider } from '@sytue/ui';
import { Bienvenida, Ficha, FichaCargando, Lista } from './ErpApp.js';
import { CAMPOS_ESQUELETO_FICHA, FILAS_ESQUELETO_LISTA } from './Esqueletos.js';
import type { Modelo, Modulo, Pagina, Registro } from './tipos.js';
import { useT as traducir } from '../i18n/index.js';

if (typeof document === 'undefined') GlobalRegistrator.register();
const t = traducir('es');
const original = globalThis.fetch;
let host: HTMLDivElement;
beforeEach(() => {
  host = document.createElement('div');
  document.body.appendChild(host);
  // Lo que pidan las vistas (recuentos, resumen) no contesta: aquí no importa, y las
  // dos lo llevan como «sin conexión».
  globalThis.fetch = (async (_input: RequestInfo | URL) =>
    new Response(JSON.stringify({ error: { code: 'unavailable', message: 'Down' } }), {
      status: 503,
    })) as typeof fetch;
});
afterEach(async () => {
  await act(async () => render(null, host));
  host.remove();
  document.body.innerHTML = '';
  globalThis.fetch = original;
});

const modelo: Modelo = {
  clave: 'factura_venta',
  modulo: 'ventas',
  etiqueta: 'Facturas',
  numerado: true,
  contable: true,
  campos: [{ clave: 'cliente', tipo: 'texto', etiqueta: 'Cliente' }],
  lista: ['ref', 'cliente'],
};
function registro(id: string, etiqueta: string): Registro {
  return {
    id,
    modelo: modelo.clave,
    ref: id.toUpperCase(),
    etiqueta,
    data: { cliente: etiqueta },
    creado_en: '2026-11-01T00:00:00Z',
    editado_en: '2026-11-01T00:00:00Z',
  };
}
const pagina: Pagina = { registros: [registro('f1', 'Acme'), registro('f2', 'Globex')] };

async function pintarLista(
  onAbrir: (r: Registro) => void,
  { cargando = false, conPagina = pagina }: { cargando?: boolean; conPagina?: Pagina | null } = {},
): Promise<void> {
  await act(async () =>
    render(
      <ToastProvider>
        <Lista
          modelo={modelo}
          pagina={conPagina}
          cargando={cargando}
          masCargando={false}
          busqueda=""
          estado=""
          orden={null}
          papelera={false}
          locale="es"
          onBuscar={() => {}}
          onEstado={() => {}}
          onOrden={() => {}}
          onPapelera={() => {}}
          onAbrir={onAbrir}
          onNueva={() => {}}
          onMas={() => {}}
          onInvalidar={() => {}}
          puedeAccion={() => true}
          soloLectura={false}
          revision={0}
          t={t}
        />
      </ToastProvider>,
      host,
    ),
  );
}

function filaMovil(texto: string): HTMLButtonElement {
  const boton = [...host.querySelectorAll<HTMLButtonElement>('ul.md\\:hidden li button')].find(
    (b) => b.textContent?.includes(texto),
  );
  if (!boton) throw new Error(`No está la fila móvil «${texto}»`);
  return boton;
}

function puntero(tipo: string, objetivo: Element): void {
  objetivo.dispatchEvent(
    new PointerEvent(tipo, {
      bubbles: true,
      cancelable: true,
      pointerId: 1,
      pointerType: 'touch',
      isPrimary: true,
      button: 0,
      clientX: 10,
      clientY: 10,
    }),
  );
}

function barraDeLote(): string | null {
  const recuento = t('list.selectedCount', { n: '1' });
  return host.textContent?.includes(recuento) ? recuento : null;
}

test('mantener pulsada una fila móvil la marca, saca la barra del lote y no abre la ficha', async () => {
  const abiertas: string[] = [];
  await pintarLista((r) => abiertas.push(r.id));
  const fila = filaMovil('Acme');
  expect(barraDeLote()).toBeNull();
  // Sin nada marcado, la lista del teléfono no lleva casillas.
  expect(host.querySelectorAll('ul.md\\:hidden input[type="checkbox"]').length).toBe(0);

  await act(async () => {
    puntero('pointerdown', fila);
    await new Promise((r) => setTimeout(r, 500));
  });
  await act(async () => {
    puntero('pointerup', fila);
    // El clic que sigue al soltar es el de la pulsación larga: no abre.
    fila.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, detail: 1 }));
  });

  expect(barraDeLote()).not.toBeNull();
  expect(abiertas).toEqual([]);
  expect(fila.closest('li')?.className).toContain('bg-surface-raised');
  // Ya en modo lote, cada fila móvil enseña su casilla para sumar con un toque.
  const casillas = host.querySelectorAll<HTMLInputElement>('ul.md\\:hidden input[type="checkbox"]');
  expect(casillas.length).toBe(2);
  expect(casillas[0]?.checked).toBe(true);
  expect(casillas[1]?.checked).toBe(false);
});

test('una pulsación normal en la fila móvil sigue abriendo la ficha', async () => {
  const abiertas: string[] = [];
  await pintarLista((r) => abiertas.push(r.id));
  const fila = filaMovil('Globex');
  await act(async () => {
    puntero('pointerdown', fila);
    puntero('pointerup', fila);
    fila.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, detail: 1 }));
  });
  expect(abiertas).toEqual(['f2']);
  expect(barraDeLote()).toBeNull();
});

test('la lista que carga pinta la forma de la tabla: seis filas con una barra por columna', async () => {
  await pintarLista(() => {}, { cargando: true, conPagina: null });
  const esqueleto = host.querySelector('[data-esqueleto="lista"]');
  expect(esqueleto).not.toBeNull();
  expect(esqueleto?.getAttribute('aria-hidden')).toBe('true');
  const filas = esqueleto?.children ?? [];
  expect(filas.length).toBe(FILAS_ESQUELETO_LISTA);
  for (const fila of filas) {
    expect(fila.querySelectorAll('.sytue-skeleton').length).toBe(modelo.lista.length);
  }
  // Ni una rueda: es la forma de lo que llega lo que evita el salto.
  expect(host.querySelector('[role="status"] svg, .animate-spin')).toBeNull();
  expect(host.querySelector('[aria-busy="true"]')).not.toBeNull();
});

test('con filas ya pintadas, recargar no las cambia por el esqueleto', async () => {
  await pintarLista(() => {}, { cargando: true });
  expect(host.querySelector('[data-esqueleto="lista"]')).toBeNull();
  expect(filaMovil('Acme')).toBeDefined();
});

test('el panel de la ficha que espera su registro esboza sus campos y dice que está ocupado', async () => {
  await act(async () => render(<FichaCargando etiqueta="Facturas" />, host));
  const panel = host.querySelector('aside');
  expect(panel?.getAttribute('aria-busy')).toBe('true');
  expect(panel?.getAttribute('aria-label')).toBe('Facturas');
  const ficha = host.querySelector('[data-esqueleto="ficha"]');
  // El título y la referencia, y un rótulo y su caja por campo.
  expect(ficha?.querySelectorAll('.sytue-skeleton').length).toBe(2 + 2 * CAMPOS_ESQUELETO_FICHA);
});

async function pintarBienvenida(
  modulos: Modulo[],
  onAbrirModelo: (m: Modelo) => void,
  onVista: (v: string) => void,
  {
    activos = modulos.map((m) => m.clave),
    onCambiar = async () => {},
  }: { activos?: string[]; onCambiar?: (m: Modulo, activo: boolean) => Promise<void> } = {},
): Promise<void> {
  await act(async () =>
    render(
      <ToastProvider>
        <Bienvenida
          modulos={modulos}
          activos={activos}
          recientes={[]}
          onCambiar={onCambiar}
          onAbrirModelo={onAbrirModelo}
          onNuevaFicha={() => {}}
          onVista={onVista}
          onPaleta={() => {}}
          userName="Ana"
          organizacion={null}
          onConfigurar={() => {}}
          tareas={0}
          verObligaciones={false}
          t={t}
        />
      </ToastProvider>,
      host,
    ),
  );
}

function tarjetaKpi(etiqueta: string): HTMLElement {
  const seccion = host.querySelector('section[aria-labelledby="heading-resumen"]');
  const tarjeta = [...(seccion?.querySelectorAll<HTMLElement>('.rounded-card') ?? [])].find((c) =>
    c.textContent?.includes(etiqueta),
  );
  if (!tarjeta) throw new Error(`No está la tarjeta «${etiqueta}»`);
  return tarjeta;
}

function tecla(objetivo: Element, key: string): void {
  objetivo.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }));
}

test('las tarjetas KPI del panel se enfocan y se abren con Intro y con Espacio', async () => {
  const abiertos: string[] = [];
  const vistas: string[] = [];
  await pintarBienvenida(
    [{ clave: 'ventas', etiqueta: 'Ventas', modelos: [modelo] }],
    (m) => abiertos.push(m.clave),
    (v) => vistas.push(v),
  );
  const ventas = tarjetaKpi(t('dashboard.kpiSales'));
  const libro = tarjetaKpi(t('nav.ledger'));
  const cobro = tarjetaKpi(t('dashboard.kpiReceivable'));
  for (const tarjeta of [ventas, libro, cobro]) {
    expect(tarjeta.getAttribute('role')).toBe('button');
    expect(tarjeta.tabIndex).toBe(0);
    expect(tarjeta.className).toContain('sytue-focus-ring-inset');
  }
  await act(async () => {
    tecla(ventas, 'Enter');
    tecla(libro, ' ');
    tecla(cobro, 'Enter');
    // Otra tecla no hace nada.
    tecla(libro, 'a');
  });
  expect(abiertos).toEqual(['factura_venta', 'factura_venta']);
  expect(vistas).toEqual(['libro']);
});

test('sin modelo de facturas, sus KPI no fingen ser botones ni entran en el Tab', async () => {
  await pintarBienvenida(
    [{ clave: 'contabilidad', etiqueta: 'Contabilidad', modelos: [] }],
    () => {},
    () => {},
  );
  const ventas = tarjetaKpi(t('dashboard.kpiSales'));
  expect(ventas.getAttribute('role')).toBeNull();
  expect(ventas.hasAttribute('tabindex')).toBe(false);
  // El libro sí: no depende del catálogo.
  expect(tarjetaKpi(t('nav.ledger')).getAttribute('role')).toBe('button');
});

test('con las facturas de venta encendidas el resumen pide y pinta la facturación por mes', async () => {
  const rutas: string[] = [];
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    rutas.push(String(input));
    return String(input).endsWith('informes/datos')
      ? new Response(JSON.stringify({ datos: [{ columnas: [], filas: [['2026-11', 50]] }] }))
      : new Response('{}', { status: 503 });
  }) as typeof fetch;
  await pintarBienvenida(
    [{ clave: 'ventas', etiqueta: 'Ventas', modelos: [modelo] }],
    () => {},
    () => {},
  );
  await act(async () => {
    for (let i = 0; i < 3; i++) await new Promise((r) => setTimeout(r, 0));
  });
  expect(rutas).toContain('/api/informes/datos');
  const grafico = host.querySelector(
    'section[aria-labelledby="heading-resumen"] [data-grafico="facturacion-mes"]',
  );
  expect(grafico?.querySelectorAll('rect')).toHaveLength(12);
});

test('sin facturas de venta el resumen no pide la facturación por mes', async () => {
  const rutas: string[] = [];
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    rutas.push(String(input));
    return new Response('{}', { status: 503 });
  }) as typeof fetch;
  await pintarBienvenida(
    [{ clave: 'contabilidad', etiqueta: 'Contabilidad', modelos: [] }],
    () => {},
    () => {},
  );
  expect(rutas).not.toContain('/api/informes/datos');
  expect(host.querySelector('[data-grafico="facturacion-mes"]')).toBeNull();
});

test('las cifras del resumen son barras mientras llega y lo neutro si no llega', async () => {
  let soltar: (r: Response) => void = () => {};
  globalThis.fetch = ((input: RequestInfo | URL) =>
    String(input).includes('resumen')
      ? new Promise<Response>((r) => {
          soltar = r;
        })
      : Promise.resolve(new Response('{}', { status: 503 }))) as typeof fetch;
  await pintarBienvenida(
    [{ clave: 'ventas', etiqueta: 'Ventas', modelos: [modelo] }],
    () => {},
    () => {},
  );
  const seccion = host.querySelector('section[aria-labelledby="heading-resumen"]');
  expect(seccion?.getAttribute('aria-busy')).toBe('true');
  expect(seccion?.querySelectorAll('[data-esqueleto="cifra"]').length).toBe(3);
  // Mientras carga no hay «0 €» que parezca un dato.
  expect(tarjetaKpi(t('dashboard.kpiSales')).textContent).not.toMatch(/0[,.]00/);
  await act(async () => {
    soltar(new Response(JSON.stringify({ error: { code: 'unavailable' } }), { status: 503 }));
    await new Promise((r) => setTimeout(r, 0));
  });
  expect(seccion?.getAttribute('aria-busy')).toBe('false');
  expect(seccion?.querySelectorAll('[data-esqueleto="cifra"]').length).toBe(0);
  expect(tarjetaKpi(t('nav.ledger')).textContent).toContain(t('dashboard.doubleEntry'));
});

// Ventas apagada, y con ella Contactos, que necesita (y que a su vez necesita Etiquetas):
// abrir sus facturas enciende las tres.
const etiquetas: Modulo = { clave: 'etiquetas', etiqueta: 'Etiquetas', modelos: [] };
const contactos: Modulo = {
  clave: 'contactos',
  etiqueta: 'Contactos',
  modelos: [],
  dependencias: ['etiquetas'],
};
const ventasApagada: Modulo = {
  clave: 'ventas',
  etiqueta: 'Ventas',
  modelos: [modelo],
  dependencias: ['contactos'],
};
async function pintarAreaApagada(
  cambios: string[],
  abiertos: string[],
  activos: string[] = [],
): Promise<void> {
  await pintarBienvenida(
    [etiquetas, contactos, ventasApagada],
    (m) => abiertos.push(m.clave),
    () => {},
    {
      activos,
      onCambiar: async (m, activo) => {
        cambios.push(`${m.clave}:${activo}`);
      },
    },
  );
}
function chipDeModelo(etiqueta: string): HTMLButtonElement {
  const chip = [...host.querySelectorAll<HTMLButtonElement>('button')].find(
    (b) => b.textContent === etiqueta && !b.closest('section[aria-labelledby="heading-resumen"]'),
  );
  if (!chip) throw new Error(`No está el modelo «${etiqueta}»`);
  return chip;
}
function dialogo(): HTMLElement | null {
  return document.querySelector<HTMLElement>('[role="dialog"]');
}
function botonDelDialogo(rotulo: string): HTMLButtonElement {
  const b = [...(dialogo()?.querySelectorAll<HTMLButtonElement>('button') ?? [])].find(
    (x) => x.textContent?.trim() === rotulo,
  );
  if (!b) throw new Error(`No está el botón «${rotulo}» en el diálogo`);
  return b;
}

test('abrir un modelo de un área apagada pregunta antes de encenderla para toda la organización', async () => {
  const cambios: string[] = [];
  const abiertos: string[] = [];
  await pintarAreaApagada(cambios, abiertos);
  await act(async () => chipDeModelo('Facturas').click());
  // Nada se enciende ni se abre hasta contestar.
  expect(cambios).toEqual([]);
  expect(abiertos).toEqual([]);
  const texto = dialogo()?.textContent ?? '';
  expect(texto).toContain(t('welcome.enableConfirmTitle', { module: 'Ventas' }));
  expect(texto).toContain(t('welcome.enableConfirmBody', { module: 'Ventas' }));
  // Las dependencias apagadas, también las transitivas, se nombran antes, no en un
  // aviso cuando ya está hecho.
  expect(texto).toContain(t('welcome.enableConfirmDeps', { deps: 'Contactos, Etiquetas' }));

  await act(async () => botonDelDialogo(t('welcome.enable')).click());
  expect(cambios).toEqual(['ventas:true']);
  expect(abiertos).toEqual(['factura_venta']);
});

test('cancelar la pregunta no enciende el área ni abre el modelo', async () => {
  const cambios: string[] = [];
  const abiertos: string[] = [];
  await pintarAreaApagada(cambios, abiertos, ['contactos', 'etiquetas']);
  await act(async () => chipDeModelo('Facturas').click());
  // Con sus dependencias ya encendidas no hay ninguna que nombrar.
  expect(dialogo()?.querySelector('p')?.textContent).toBe(
    t('welcome.enableConfirmBody', { module: 'Ventas' }),
  );
  // El pie del diálogo es «Cancelar» y, el último, la acción.
  const pie = [...(dialogo()?.querySelectorAll<HTMLButtonElement>('button') ?? [])];
  expect(pie.at(-1)?.textContent?.trim()).toBe(t('welcome.enable'));
  await act(async () => pie.at(-2)?.click());
  expect(cambios).toEqual([]);
  expect(abiertos).toEqual([]);
});

test('un modelo de un área encendida se abre sin preguntar', async () => {
  const cambios: string[] = [];
  const abiertos: string[] = [];
  await pintarAreaApagada(cambios, abiertos, ['etiquetas', 'contactos', 'ventas']);
  await act(async () => chipDeModelo('Facturas').click());
  expect(dialogo()).toBeNull();
  expect(cambios).toEqual([]);
  expect(abiertos).toEqual(['factura_venta']);
});

test('la ficha avisa de lo escrito sin guardar, y deja de avisar al deshacerlo o cerrarse', async () => {
  const avisos: boolean[] = [];
  const pintar = (): Promise<void> =>
    act(async () =>
      render(
        <ToastProvider>
          <Ficha
            modelo={{ ...modelo, contable: false }}
            registro={registro('f1', 'Acme')}
            semilla={null}
            etiquetas={{}}
            locale="es"
            enPapelera={false}
            facturae={false}
            requisitos={null}
            soloLectura={false}
            onTareas={() => {}}
            onCambios={(sucia) => avisos.push(sucia)}
            onCerrar={() => {}}
            onDuplicar={() => {}}
            onConvertido={() => {}}
            onGuardado={() => {}}
            onInvalidar={() => {}}
            t={t}
          />
        </ToastProvider>,
        host,
      ),
    );
  await pintar();
  expect(avisos.at(-1)).toBe(false);

  const cliente = host.querySelector<HTMLInputElement>('#campo-cliente');
  if (!cliente) throw new Error('No está el campo «Cliente»');
  const escribir = (valor: string): Promise<void> =>
    act(async () => {
      cliente.value = valor;
      cliente.dispatchEvent(new Event('input', { bubbles: true }));
    });

  await escribir('Acme Ibérica');
  expect(avisos.at(-1)).toBe(true);
  await escribir('Acme');
  expect(avisos.at(-1)).toBe(false);

  await escribir('Otra');
  expect(avisos.at(-1)).toBe(true);
  // Cerrada (desmontada), ya no hay nada que perder.
  await act(async () => render(null, host));
  expect(avisos.at(-1)).toBe(false);
});

test('los totales de la barra y la exportación siguen la búsqueda, el estado y el orden de la lista', async () => {
  const pedidas: string[] = [];
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    const url = new URL(String(input), 'https://erp.example.test');
    pedidas.push(url.pathname + url.search);
    return new Response(
      JSON.stringify({ modelo: 'factura_venta', total: 9, filtrados: 2, por_estado: {} }),
      { status: 200, headers: { 'content-type': 'application/json' } },
    );
  }) as typeof fetch;
  await act(async () =>
    render(
      <ToastProvider>
        <Lista
          modelo={modelo}
          pagina={pagina}
          cargando={false}
          masCargando={false}
          busqueda="acme"
          estado="pagado"
          orden={{ campo: 'cliente', asc: true }}
          papelera={false}
          locale="es"
          onBuscar={() => {}}
          onEstado={() => {}}
          onOrden={() => {}}
          onPapelera={() => {}}
          onAbrir={() => {}}
          onNueva={() => {}}
          onMas={() => {}}
          onInvalidar={() => {}}
          puedeAccion={() => true}
          soloLectura={false}
          revision={0}
          t={t}
        />
      </ToastProvider>,
      host,
    ),
  );
  await act(async () => {
    await new Promise((r) => setTimeout(r, 0));
  });
  expect(pedidas).toContain('/api/factura_venta/estadisticas?q=acme&estado=pagado');
  // El recuento es lo que la lista enseña, no el modelo entero.
  expect(host.textContent).toContain(t('list.recordsLoaded', { count: 2 }));
  const exportar = host.querySelector<HTMLAnchorElement>('a[href*="/export"]');
  expect(exportar?.getAttribute('href')).toContain('orden=cliente');
  expect(exportar?.getAttribute('href')).toContain('dir=asc');
});

test('una factura abierta y vencida lleva su insignia en la lista; una pagada, no', async () => {
  const vencida = {
    ...registro('f1', 'Acme'),
    estado: 'confirmado',
    data: { cliente: 'Acme', vencimiento: '2020-01-01' },
  };
  const pagada = {
    ...registro('f2', 'Globex'),
    estado: 'pagado',
    data: { cliente: 'Globex', vencimiento: '2020-01-01' },
  };
  await pintarLista(() => {}, {
    conPagina: { registros: [vencida, pagada] },
  });
  const insignias = [...host.querySelectorAll('[data-vencida]')].map((e) =>
    e.closest('tr, li')?.textContent?.includes('Acme'),
  );
  expect(insignias.length).toBeGreaterThan(0);
  expect(insignias.every(Boolean)).toBe(true);
});
