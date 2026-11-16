// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
/**
 * El muestrario de `@sytue/ui` (`muestrario.tsx`): que monta entero en las 16
 * combinaciones del tema, que sus capas se abren, y el guardián de los exports.
 *
 * Vistos en ROJO: cambiando el `<Meter` de la familia «estado» por otra pieza (el
 * guardián: «no salen en el muestrario: Meter») y dejando el diálogo de «dialogos»
 * cerrado aunque se pida `abrir` («el diálogo de «dialogos» no se abre»).
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, beforeEach, expect, spyOn, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { render } from 'preact';
import { I18nProvider } from '@sytue/i18n/preact';

const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register();
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

// Después de registrar happy-dom: varios módulos miran `window` al cargarse.
const ui = await import('../src/index.js');
const { applyThemeColor, applyThemeForma, applyThemeMode } = await import('../src/theme/init.js');
const { CAPAS_DE_DIALOGOS, COLORES, COMBINACIONES, FAMILIAS, FORMAS, MODOS, Muestrario, nombreDe } =
  await import('./muestrario.js');

/**
 * Los componentes exportados que NO salen en el muestrario, con el porqué. Un componente
 * aquí es uno que nadie revisa en oscuro ni en carril: cada fila necesita un motivo de
 * verdad, no «ya lo enseñan las apps».
 */
const FUERA_DEL_MUESTRARIO: Record<string, string> = {
  ImageError: 'es la clase de error de reducirImagen, no un componente que se pinte',
};

/** Lo que exporta `index.ts` con nombre de componente: función y en PascalCase. */
function componentesExportados(): string[] {
  return Object.entries(ui)
    .filter(([nombre, valor]) => /^[A-Z][a-z]/.test(nombre) && typeof valor === 'function')
    .map(([nombre]) => nombre)
    .sort();
}

/** El fuente del muestrario y de su página: un componente sale si aparece como `<Nombre`. */
const fuente = ['muestrario.tsx', 'muestrario-pagina.tsx']
  .map((f) => readFileSync(new URL(f, import.meta.url), 'utf8'))
  .join('\n');
// `<Table<Fila>` y `<Contexto.Provider` también cuentan.
const sale = (nombre: string) => new RegExp(`<${nombre}[\\s/>.<]`).test(fuente);

test('cada componente que exporta index.ts sale en el muestrario (o en su exclusión, con motivo)', () => {
  const componentes = componentesExportados();
  // Si esto baja, el guardián no está leyendo el índice, no es que haya menos piezas.
  expect(componentes.length).toBeGreaterThan(70);
  const faltan = componentes.filter((c) => !sale(c) && !(c in FUERA_DEL_MUESTRARIO));
  expect(faltan, `no salen en el muestrario: ${faltan.join(', ')}`).toEqual([]);
  for (const [nombre, motivo] of Object.entries(FUERA_DEL_MUESTRARIO)) {
    expect(componentes, `la exclusión de ${nombre} nombra algo que ya no se exporta`).toContain(
      nombre,
    );
    expect(sale(nombre), `${nombre} ya sale en el muestrario: quita su exclusión`).toBe(false);
    expect(motivo.trim().length, `${nombre} está fuera sin motivo`).toBeGreaterThan(10);
  }
});

test('el muestrario importa solo la API pública', () => {
  // Una muestra de un módulo interno enseña algo que las apps no pueden usar.
  const muestras = readFileSync(new URL('muestrario.tsx', import.meta.url), 'utf8');
  const origenes = [...muestras.matchAll(/^\s*\} from '([^']+)';$|^import .* from '([^']+)';$/gm)]
    .map((m) => m[1] ?? m[2])
    .filter((o) => o?.startsWith('.'));
  expect(origenes.length).toBeGreaterThan(0);
  expect(new Set(origenes)).toEqual(new Set(['../src/index.js']));
});

test('los ejes del muestrario son los del tema: 2 colores × 4 modos × 2 formas', () => {
  // Un color o una forma nuevos en el tema tienen que entrar en el recorrido; «el de
  // cada app» no es UN color y se revisa en cada app.
  expect([...COLORES]).toEqual(ui.THEME_COLORS.filter((c) => c !== 'app'));
  expect([...FORMAS]).toEqual([...ui.THEME_FORMAS]);
  expect([...MODOS]).toEqual(['light', 'dark', 'translucent', 'system']);
  expect(COMBINACIONES).toHaveLength(16);
  expect(new Set(COMBINACIONES.map(nombreDe)).size).toBe(16);
});

let host: HTMLElement;
let errores: unknown[][];
let espia: ReturnType<typeof spyOn>;
beforeEach(() => {
  host = document.createElement('div');
  document.body.appendChild(host);
  errores = [];
  espia = spyOn(console, 'error').mockImplementation((...args: unknown[]) => {
    errores.push(args);
  });
});
afterEach(() => {
  render(null, host);
  host.remove();
  espia.mockRestore();
  // Lo que las capas dejan en el body (portales de diálogos, avisos, globos).
  document.body.innerHTML = '';
  for (const a of ['data-theme', 'data-color', 'data-forma']) {
    document.documentElement.removeAttribute(a);
  }
});

function montar(props: Parameters<typeof Muestrario>[0] = {}) {
  render(
    <I18nProvider locale="es">
      <Muestrario {...props} />
    </I18nProvider>,
    host,
  );
}

for (const combinacion of COMBINACIONES) {
  test(`monta entero en ${nombreDe(combinacion)}`, () => {
    applyThemeMode(combinacion.modo);
    applyThemeColor(combinacion.color);
    applyThemeForma(combinacion.forma);
    const raiz = document.documentElement;
    // Los ejes de la casa son la hoja SIN atributo: así los ve la persona.
    expect(raiz.getAttribute('data-theme')).toBe(
      combinacion.modo === 'system' ? null : combinacion.modo,
    );
    expect(raiz.getAttribute('data-color')).toBe(
      combinacion.color === 'petroleo' ? null : combinacion.color,
    );
    expect(raiz.getAttribute('data-forma')).toBe(
      combinacion.forma === 'marco' ? null : combinacion.forma,
    );

    montar();
    const secciones = [...host.querySelectorAll<HTMLElement>('section[data-familia]')];
    expect(secciones.map((s) => s.dataset.familia)).toEqual(FAMILIAS.map((f) => f.clave));
    for (const s of secciones) {
      // Bajo el título, la familia tiene que pintar algo: una que se queda en blanco
      // porque un componente pide algo que no le llega no avisa sola.
      const cuerpo = [...s.children].slice(1);
      expect(cuerpo.length, `«${s.dataset.familia}» no pinta nada`).toBeGreaterThan(0);
      expect(s.querySelectorAll('*').length, `«${s.dataset.familia}» casi vacía`).toBeGreaterThan(
        4,
      );
    }
    expect(errores, `errores al montar: ${errores.map((e) => String(e[0])).join(' | ')}`).toEqual(
      [],
    );
  });
}

test('las familias de capa salen cerradas y se abren con `abrir`', async () => {
  const capas = FAMILIAS.filter((f) => f.capa).map((f) => f.clave);
  expect(capas).toEqual(['menus', 'dialogos']);

  montar();
  expect(document.querySelector('[role="dialog"]')).toBeNull();
  render(null, host);

  montar({ familias: ['menus'], abrir: 'menus' });
  const panel = document.querySelector('[role="dialog"][aria-label="Opciones de vista"]');
  expect(panel, 'el panel de «menus» no se abre').not.toBeNull();
  render(null, host);

  montar({ familias: ['dialogos'], abrir: 'dialogos' });
  const dialogo = document.querySelector('[role="dialog"][aria-modal="true"]');
  expect(dialogo, 'el diálogo de «dialogos» no se abre').not.toBeNull();
  expect(dialogo?.textContent).toContain('Diálogo de muestra');
  // Los avisos se lanzan en un efecto y la pila se pinta en el render siguiente: se
  // espera a que salga el primero, con tope, porque 20 ms fijos no llegaban con la
  // máquina cargada.
  for (let i = 0; i < 50 && !(document.body.textContent ?? '').includes('Guardado'); i++) {
    await new Promise((r) => setTimeout(r, 20));
  }
  // Los avisos, ya lanzados: los tres primeros a la vista y el cuarto en cola, que es
  // el tope de la pila (MAX_AVISOS_VISIBLES) y también se revisa.
  const avisos = document.body.textContent ?? '';
  for (const titulo of ['Guardado', 'Sincronizando', 'Queda poco espacio']) {
    expect(avisos, `falta el aviso «${titulo}»`).toContain(titulo);
  }
  expect(avisos).not.toContain('No se pudo enviar');
  expect(errores).toEqual([]);
});

/** Lo que tiene que enseñar cada capa de «dialogos» una vez abierta. */
const TEXTO_DE_CAPA: Record<(typeof CAPAS_DE_DIALOGOS)[number], string> = {
  modal: 'Diálogo de muestra',
  confirmar: 'Presupuesto 2027',
  atajos: 'Atajos de muestra',
  paleta: 'Nuevo mensaje',
  editor: '¿Seguir con la acción?',
};

for (const capa of CAPAS_DE_DIALOGOS) {
  test(`la capa «${capa}» de «dialogos» se abre sola con abrir=dialogos:${capa}`, async () => {
    // Montar el botón no prueba nada: la capa es lo que se rompe en oscuro o en carril.
    montar({ familias: ['dialogos'], abrir: `dialogos:${capa}` });
    // La capa sale tras los efectos (la del editor, además, tras una promesa): se espera a
    // que aparezca, con tope, en vez de un plazo fijo que en una máquina cargada no llega.
    const capas = () => [...document.querySelectorAll('[role="dialog"], [role="alertdialog"]')];
    for (let i = 0; i < 50 && capas().length === 0; i++) {
      await new Promise((r) => setTimeout(r, 20));
    }
    const dialogos = capas();
    expect(dialogos, `la capa «${capa}» no se abre`).toHaveLength(1);
    expect(dialogos[0]?.textContent).toContain(TEXTO_DE_CAPA[capa]);
    // Sin `dialogos` a secas no se lanzan los avisos: la capa sale sola.
    expect(document.body.textContent).not.toContain('Sincronizando');
    expect(errores).toEqual([]);
  });
}

test('el menú de «menus» despliega sus opciones', async () => {
  montar({ familias: ['menus'] });
  const disparador = [...host.querySelectorAll('button')].find(
    (b) => b.textContent?.trim() === 'Acciones',
  );
  expect(disparador, 'no está el botón del menú').toBeDefined();
  disparador?.click();
  await new Promise((r) => setTimeout(r, 0));
  const menu = document.querySelector('[role="menu"]');
  expect(menu, 'el menú no se despliega').not.toBeNull();
  expect(menu?.textContent).toContain('Duplicar');
  expect(errores).toEqual([]);
});

test('`familias` acota lo que se monta', () => {
  montar({ familias: ['botones', 'campos'] });
  const claves = [...host.querySelectorAll<HTMLElement>('section[data-familia]')].map(
    (s) => s.dataset.familia,
  );
  expect(claves).toEqual(['botones', 'campos']);
});
