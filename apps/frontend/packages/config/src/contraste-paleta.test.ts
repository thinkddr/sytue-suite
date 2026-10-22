/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL CONTRASTE DE LA PALETA SE MIDE SOBRE LA HOJA, no sobre una maqueta. Cada medida es
 * la razón WCAG 2.x de un par que la interfaz pinta de verdad, con los velos y cristales
 * ya aplanados sobre lo que tienen debajo, en las tres apariencias y por las dos puertas
 * del oscuro. El texto pide 4,5:1; los contornos, el foco y los indicadores, 3:1
 * (WCAG 1.4.3 y 1.4.11).
 *
 * En el traslúcido el fondo es un degradado: cada medida se toma en sus tres paradas y
 * bajo la luz cálida, y vale la PEOR.
 *
 * `MEDIDAS=1 bun test contraste-paleta` imprime la tabla (docs/design.md §2.1 la cita).
 *
 * Visto en ROJO: con `--sytue-text-subtle: #7a8a82` en el bloque claro caen «Texto sutil
 * sobre superficie» y «Texto sutil sobre el lienzo».
 *
 * Y EL CARRIL: dentro del menú lateral de color, appearance.css reasigna unos tokens y
 * los devuelve al tema en lo que se abre dentro con superficie propia. Las dos listas
 * tienen que ser LA MISMA: lo que se reasigna y no se devuelve se cuela en el
 * conmutador de aplicaciones (un indicador ámbar, un botón blanco sobre blanco).
 *
 * Se mide lo que se pinta: colors.css y, encima, appearance.css, que es donde vive el
 * carril (las apps nativas leen las dos hojas en ese mismo orden: apps/nucleo/tokens).
 *
 * Y LA FORMA «MARCO», que es la que se ve sin elegir nada: la barra superior y el menú
 * son un marco de color continuo, el ítem activo es una píldora de velo y el contenido, una hoja
 * (un cristal sobre el marco, en el traslúcido). Se mide el texto sobre el marco, la
 * píldora, el buscador de la barra y, otra vez, todo el contenido con la HOJA de lienzo.
 *
 * Visto en ROJO: el buscador nació con el velo claro del ítem activo
 * (`background-color: var(--sytue-sidebar-active-bg)`) y cayó «Marco: marca de agua del
 * buscador» en claro (3,92) y en traslúcido (4,07): por eso el velo es oscuro, el mismo
 * `--sytue-sidebar-field` de la barra-carril del móvil.
 */
import { expect, test } from 'bun:test';

/** Una hoja de tokens, sin comentarios. */
const hoja = async (nombre: string): Promise<string> =>
  (await Bun.file(new URL(`./tokens/${nombre}`, import.meta.url)).text()).replace(
    /\/\*[\s\S]*?\*\//g,
    '',
  );
// En el orden de la cascada: appearance.css va después y gana a igual selector.
const hojas = [await hoja('colors.css'), await hoja('appearance.css')];

type Color = [number, number, number, number];
type Paleta = (nombre: string) => Color;

/** Las custom properties del primer bloque que abre `apertura` en una hoja. */
function declaraciones(css: string, apertura: string): Map<string, string> {
  const out = new Map<string, string>();
  const i = css.indexOf(apertura);
  if (i < 0) return out;
  const cuerpo = css.slice(i + apertura.length, css.indexOf('}', i));
  for (const m of cuerpo.matchAll(/--sytue-([a-z0-9-]+)\s*:\s*([^;]+);/g)) {
    // Una declaración partida en varias líneas (el `color-mix` de la selección) queda
    // con un espacio tras el paréntesis: se quita, como hace el lector de las nativas.
    out.set(m[1]!, m[2]!.replace(/\s+/g, ' ').replace(/\( /g, '(').replace(/ \)/g, ')').trim());
  }
  return out;
}

/** Cuánto pesa un selector de bloque: sus atributos y pseudoclases (`:root`, `:not([…])`). */
const peso = (apertura: string): number => (apertura.match(/:root|\[/g) ?? []).length;

function color(valor: string): Color {
  const hex = /^#([0-9a-f]{6})$/i.exec(valor);
  if (hex) {
    const n = parseInt(hex[1]!, 16);
    return [n >> 16, (n >> 8) & 255, n & 255, 1];
  }
  const rgba = /^rgba?\(([^)]+)\)$/.exec(valor);
  if (rgba) {
    const p = rgba[1]!.split(',').map(Number);
    return [p[0]!, p[1]!, p[2]!, p[3] ?? 1];
  }
  const mezcla = /^color-mix\(in srgb, ?(.+) ([0-9.]+)%, ?transparent ?\)$/.exec(valor);
  if (mezcla) {
    const c = color(mezcla[1]!);
    return [c[0], c[1], c[2], (c[3] * Number(mezcla[2])) / 100];
  }
  if (valor === 'transparent') return [0, 0, 0, 0];
  throw new Error(`valor que no sé leer: ${valor}`);
}

/**
 * La paleta de una apariencia: `:root` y los bloques activos de las dos hojas, aplicados
 * como la cascada: primero por peso del selector y, a igual peso, por orden (colors.css
 * antes que appearance.css, y dentro de cada hoja, el de arriba antes).
 */
function paleta(bloques: string[]): Paleta {
  const activos = [':root {', ...bloques];
  const capas = hojas.flatMap((css, hoja) =>
    activos.map((apertura) => ({
      peso: peso(apertura),
      orden: hoja * 1e9 + css.indexOf(apertura),
      decl: declaraciones(css, apertura),
    })),
  );
  capas.sort((a, b) => a.peso - b.peso || a.orden - b.orden);
  const decl = new Map<string, string>();
  for (const capa of capas) for (const [k, v] of capa.decl) decl.set(k, v);
  const expandir = (v: string): string =>
    v.replace(/var\(--sytue-([a-z0-9-]+)\)/g, (_, n: string) => {
      const crudo = decl.get(n);
      if (crudo === undefined) throw new Error(`var(--sytue-${n}) sin definir`);
      return expandir(crudo);
    });
  return (nombre) => {
    const crudo = decl.get(nombre);
    if (crudo === undefined) throw new Error(`no existe --sytue-${nombre}`);
    return color(expandir(crudo));
  };
}

/** `encima` compuesto sobre `debajo` como lo hace el navegador (sRGB, sin luz lineal). */
function sobre(encima: Color, debajo: Color): Color {
  const a = encima[3];
  return [0, 1, 2].map((i) => encima[i]! * a + debajo[i]! * (1 - a)).concat(1) as Color;
}

function luminancia(c: Color): number {
  const f = (v: number) => {
    const s = v / 255;
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
  };
  return 0.2126 * f(c[0]) + 0.7152 * f(c[1]) + 0.0722 * f(c[2]);
}

function razon(a: Color, b: Color): number {
  const [x, y] = [luminancia(a), luminancia(b)];
  return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05);
}

const TARJETAS = ['blue', 'violet', 'mint', 'rose', 'amber', 'cyan', 'coral', 'lime', 'slate'];

/** Rótulo, mínimo exigido y razón, con las capas YA aplanadas sobre `lienzo`. */
function medidas(t: Paleta, lienzo: Color): [string, number, number][] {
  const sup = sobre(t('surface'), lienzo);
  const lado = sobre(t('sidebar-bg'), lienzo);
  const barra = sobre(t('topbar-bg'), lienzo);
  const activo = sobre(t('sidebar-active-bg'), lado);
  const elegido = sobre(t('selected'), sup);
  // El buscador de la barra superior. Pinta SUPERFICIE y marca de agua `text-subtle`, no
  // el `bg-chrome-hover` que pide GlobalSearch.tsx: gana el `bg-surface` de `controlBase`
  // (medido en producción; lo documenta `colorCampoCromo` en apps/fyne/ui/campo.go).
  const campo = sobre(t('surface'), barra);
  const campoCarril = sobre(t('sidebar-field'), lado);
  const velo = t('sidebar-active-bg');
  const pastillaTenue = sobre([velo[0], velo[1], velo[2], velo[3] / 2], lado);
  const tarjeta = Math.min(
    ...TARJETAS.map((n) => razon(t(`on-card-solid-${n}`), sobre(t(`card-solid-${n}`), sup))),
  );
  return [
    ['Texto sobre superficie', 4.5, razon(t('text'), sup)],
    ['Texto atenuado sobre superficie', 4.5, razon(t('text-muted'), sup)],
    ['Texto sutil sobre superficie', 4.5, razon(t('text-subtle'), sup)],
    ['Texto sutil sobre el lienzo', 4.5, razon(t('text-subtle'), lienzo)],
    ['Texto sobre fila seleccionada', 4.5, razon(t('text'), elegido)],
    ['Enlace sobre superficie', 4.5, razon(t('primary-text'), sup)],
    ['Rótulo del botón principal', 4.5, razon(t('on-primary'), t('primary'))],
    ['Botón principal contra la superficie', 3, razon(t('primary'), sup)],
    // El CARRIL: lo que vive en el menú lateral usa sus propios valores (appearance.css).
    ['Carril: ítem activo', 4.5, razon(sobre(t('sidebar-active-fg'), activo), activo)],
    ['Carril: texto', 4.5, razon(sobre(t('sidebar-fg'), lado), lado)],
    ['Carril: ítem en reposo', 4.5, razon(sobre(t('sidebar-fg-muted'), lado), lado)],
    ['Carril: botón principal contra el carril', 3, razon(t('sidebar-primary'), lado)],
    [
      'Carril: rótulo del botón principal',
      4.5,
      razon(t('sidebar-on-primary'), t('sidebar-primary')),
    ],
    ['Carril: anillo de foco', 3, razon(t('sidebar-focus'), lado)],
    ['Carril: indicador «estás aquí»', 3, razon(t('sidebar-indicator'), activo)],
    // Lo que el repaso apoyó en el carril: el buscador de la barra en <768px (velo
    // neutro con el texto del carril) y el aro de las muestras de color —el punto de una
    // etiqueta de Mail, la casilla de un calendario—, que es lo único que las separa de
    // un carril de su mismo tono. Visto en ROJO: con el velo en blanco al 50 % caen las
    // dos del buscador en todas las apariencias, y con el texto del carril en un tono
    // del propio carril, el aro y el texto del buscador.
    [
      'Carril: texto del buscador en móvil',
      4.5,
      razon(sobre(t('sidebar-fg'), campoCarril), campoCarril),
    ],
    [
      'Carril: marca de agua del buscador en móvil',
      4.5,
      razon(sobre(t('sidebar-fg-muted'), campoCarril), campoCarril),
    ],
    ['Carril: aro de las muestras de color', 3, razon(sobre(t('sidebar-fg'), lado), lado)],
    // En <768px la barra superior es el carril, y lo que lleva color propio encima —el
    // punto rojo de la campana, el disco del avatar— se distingue por su aro. El punto
    // SIN aro no llega: `danger` contra el carril se queda por debajo de 3:1 (y con el
    // color de Mail es rojo sobre rojo), que es lo que se vio en ROJO.
    ['Carril: aro del punto de aviso y del avatar', 3, razon(sobre(t('sidebar-fg'), lado), lado)],
    // El canal SILENCIADO de Rooms: su nombre es la medida «ítem en reposo» de arriba, y
    // su contador va en una pastilla del velo activo a la mitad, con el texto del carril
    // sin atenuar. Visto en ROJO con el tono atenuado, que es el que llevaba: 4,49 en
    // cobalto claro y 4,46 en cobalto traslúcido.
    [
      'Carril: contador de un canal silenciado',
      4.5,
      razon(sobre(t('sidebar-fg'), pastillaTenue), pastillaTenue),
    ],
    ['Barra superior: texto', 4.5, razon(sobre(t('topbar-fg'), barra), barra)],
    ['Barra superior: marca de agua del buscador', 4.5, razon(t('text-subtle'), campo)],
    ['Tarjeta: rótulo sobre su color', 4.5, tarjeta],
    ['Contorno de control', 3, razon(sobre(t('border-strong'), sup), sup)],
    ['Anillo de foco', 3, razon(t('focus-ring'), sup)],
    ['Indicador «estás aquí» fuera del carril', 3, razon(t('indicator'), sup)],
  ];
}

/** Los fondos sobre los que se mide una apariencia: uno, salvo el degradado traslúcido. */
function lienzos(t: Paleta, traslucido: boolean): Color[] {
  if (!traslucido) return [t('bg')];
  const paradas = ['workspace-blue', 'workspace-middle', 'workspace-mint'].map((n) => t(n));
  return [...paradas, ...paradas.map((p) => sobre(t('workspace-warm'), p))];
}

const MODOS: [string, string | undefined][] = [
  // nombre y bloque del modo en petróleo
  ['claro', undefined],
  ['traslúcido', "[data-theme='translucent'] {"],
  ['oscuro elegido', "[data-theme='dark'] {"],
  ['oscuro del sistema', ':root:not([data-theme]) {'],
];
/** El bloque de un color elegible: el claro, o el de un modo. */
const claroDe = (color: string): string => `[data-color='${color}'] {`;
const modoDe = (color: string, modo: string): string =>
  modo.startsWith(':root')
    ? `:root:not([data-theme])[data-color='${color}'] {`
    : `[data-color='${color}']${modo}`;
const COBALTO = claroDe('cobalto');
/** En la cascada: `:root`, el modo, el claro del color, lo propio de la app y el modo del color. */
const capas = (modo: string | undefined, color: string, app?: string): string[] => [
  ...(modo ? [modo] : []),
  claroDe(color),
  ...(app ? [`[data-color='app'][data-app='${app}'] {`] : []),
  ...(modo ? [modoDe(color, modo)] : []),
];
/** Las apps con rampa propia en el tema «color de cada app», por su clave de producto. */
const APPS = [...hojas[0]!.matchAll(/^\[data-color='app'\]\[data-app='([a-z]+)'\] \{/gm)].map(
  (m) => m[1]!,
);
// El color es un eje aparte del modo: cada medida, en cada color por los cuatro modos. Y
// en «color de cada app», en cada app y en la superficie sin rampa propia (la casa).
const APARIENCIAS: [string, string[]][] = MODOS.flatMap(([nombre, modo]) => [
  [`petróleo · ${nombre}`, modo ? [modo] : []] as [string, string[]],
  [`cobalto · ${nombre}`, capas(modo, 'cobalto')] as [string, string[]],
  [`app (la casa) · ${nombre}`, capas(modo, 'app')] as [string, string[]],
  ...APPS.map((app) => [`app ${app} · ${nombre}`, capas(modo, 'app', app)] as [string, string[]]),
]);

/**
 * Lo que «color de cada app» promete por encima del mínimo común: el carril de cada
 * app aguanta texto blanco a 6,6:1 en claro, y en claro y en oscuro el rótulo del botón
 * llega a 4,7:1 y el enlace a 6:1. En traslúcido la superficie cambia con el fondo y
 * rige el mínimo común.
 */
function minimoDeApp(nombre: string, rotulo: string, minimo: number): number {
  if (!nombre.startsWith('app ') || nombre.includes('traslúcido')) return minimo;
  if (rotulo === 'Carril: texto' && nombre.endsWith('claro')) return 6.6;
  if (rotulo === 'Rótulo del botón principal') return 4.7;
  if (rotulo === 'Enlace sobre superficie') return 6;
  return minimo;
}

/** La peor razón de cada medida entre los fondos de la apariencia. */
function peores(nombre: string, bloques: string[]): [string, number, number][] {
  const t = paleta(bloques);
  const porLienzo = lienzos(t, nombre.includes('traslúcido')).map((l) => medidas(t, l));
  return porLienzo[0]!.map(([rotulo, minimo], i) => [
    rotulo,
    minimo,
    Math.min(...porLienzo.map((m) => m[i]![2])),
  ]);
}

for (const [nombre, bloques] of APARIENCIAS) {
  test(`la paleta cumple el contraste en ${nombre}`, () => {
    const filas = peores(nombre, bloques).map(
      ([rotulo, minimo, valor]) =>
        [rotulo, minimoDeApp(nombre, rotulo, minimo), valor] as [string, number, number],
    );
    if (process.env.MEDIDAS) {
      // `warn` y no `log`: es salida pedida a mano, y el linter solo deja esos dos.
      console.warn(`\n${nombre}`);
      for (const [rotulo, minimo, valor] of filas) {
        console.warn(`  ${valor.toFixed(2).padStart(6)}  (mín. ${minimo})  ${rotulo}`);
      }
    }
    const cortas = filas
      .filter(([, minimo, valor]) => valor < minimo)
      .map(([rotulo, minimo, valor]) => `${rotulo}: ${valor.toFixed(2)} < ${minimo}`);
    expect(cortas).toEqual([]);
  });
}

// ---- La forma «Marco» ----
const MARCO = ":root:not([data-forma='carril']) {";
const MARCO_TRASLUCIDO = "[data-theme='translucent']:not([data-forma='carril']) {";

/** El cromo de Marco sobre un punto del marco (en el traslúcido, un degradado). */
function medidasMarco(t: Paleta, marco: Color): [string, number, number][] {
  const pildora = sobre(t('nav-active-bg'), marco);
  const velo = sobre(t('rail-active-bg'), marco);
  const encima = sobre(t('sidebar-hover-bg'), marco);
  // El buscador de la barra: el velo neutro del carril con el texto del cromo.
  const campo = sobre(t('sidebar-field'), marco);
  return [
    ['Marco: texto de la barra y del menú', 4.5, razon(sobre(t('sidebar-fg'), marco), marco)],
    ['Marco: ítem en reposo', 4.5, razon(sobre(t('sidebar-fg-muted'), marco), marco)],
    ['Marco: ítem bajo el cursor', 4.5, razon(sobre(t('sidebar-fg'), encima), encima)],
    // El ítem activo es una píldora de VELO con el texto claro; la maciza es del botón.
    ['Marco: ítem activo', 4.5, razon(sobre(t('nav-active-fg'), pildora), pildora)],
    ['Marco: botón del menú contra el marco', 3, razon(t('sidebar-primary'), marco)],
    ['Marco: rótulo del botón del menú', 4.5, razon(t('sidebar-on-primary'), t('sidebar-primary'))],
    ['Marco: texto del buscador', 4.5, razon(sobre(t('sidebar-fg'), campo), campo)],
    ['Marco: marca de agua del buscador', 4.5, razon(sobre(t('sidebar-fg-muted'), campo), campo)],
    ['Marco: anillo de foco sobre el marco', 3, razon(t('sidebar-focus'), marco)],
    // El CARRIL DE ICONOS (ERP): no hay rótulo, así que cada icono pide el 3:1 de un
    // componente gráfico y «estás aquí» lo dice el relleno del icono activo sobre su velo.
    ['Marco: carril de iconos, icono en reposo', 3, razon(sobre(t('sidebar-fg'), marco), marco)],
    ['Marco: carril de iconos, icono activo', 3, razon(sobre(t('rail-active-fg'), velo), velo)],
    // «Estás aquí» es la BARRA del indicador, a plena opacidad, sobre el marco. Aquí se
    // medía el ámbar entero contra el velo (3,73) cuando lo que se pintaba era el relleno
    // del glifo al 34 %: 1,59 en claro. Una medida compone la opacidad con que se pinta
    // su color (`sobre`), o no mide lo que se ve.
    ['Marco: carril de iconos, barra de «estás aquí»', 3, razon(t('sidebar-indicator'), marco)],
    // El punto de la campana y el avatar, sobre el marco a cualquier ancho (ver «Carril:
    // aro del punto de aviso y del avatar»).
    ['Marco: aro del punto de aviso y del avatar', 3, razon(sobre(t('sidebar-fg'), marco), marco)],
  ];
}

/** Lo que `medidas` mide del CONTENIDO: en Marco se repite con la hoja de lienzo. */
const esContenido = (rotulo: string) => !/^(Carril|Barra superior)/.test(rotulo);

for (const [nombre, bloques] of APARIENCIAS) {
  test(`la forma Marco cumple el contraste en ${nombre}`, () => {
    const traslucido = nombre.includes('traslúcido');
    const t = paleta([...bloques, MARCO, ...(traslucido ? [MARCO_TRASLUCIDO] : [])]);
    for (const token of ['frame-from', 'frame-to', 'sheet-bg', 'nav-active-bg', 'sidebar-field']) {
      expect(() => t(token), `appearance.css no declara --sytue-${token} en Marco`).not.toThrow();
    }
    // Los dos extremos del marco: entre ellos el degradado es monótono.
    const puntos = [t('frame-from'), t('frame-to')].map((c) => sobre(c, t('bg')));
    const porPunto = puntos.map((marco) => [
      ...medidasMarco(t, marco),
      ...medidas(t, sobre(t('sheet-bg'), marco))
        .filter(([rotulo]) => esContenido(rotulo))
        .map(([rotulo, minimo, valor]): [string, number, number] => [
          `Marco, sobre la hoja: ${rotulo}`,
          minimo,
          valor,
        ]),
    ]);
    const filas = porPunto[0]!.map(([rotulo, minimo], i): [string, number, number] => [
      rotulo,
      minimo,
      Math.min(...porPunto.map((m) => m[i]![2])),
    ]);
    if (process.env.MEDIDAS) {
      console.warn(`\nMarco, ${nombre}`);
      for (const [rotulo, minimo, valor] of filas) {
        console.warn(`  ${valor.toFixed(2).padStart(6)}  (mín. ${minimo})  ${rotulo}`);
      }
    }
    const cortas = filas
      .filter(([, minimo, valor]) => valor < minimo)
      .map(([rotulo, minimo, valor]) => `${rotulo}: ${valor.toFixed(2)} < ${minimo}`);
    expect(cortas).toEqual([]);
  });
}

// «ESTÁS AQUÍ» NO PUEDE DEPENDER SOLO DEL VELO. El ítem activo de Marco es una píldora de
// velo que contra el marco no llega al 3:1 de un indicador (WCAG 1.4.11); en Carril lo
// suple la barra ámbar y en Marco, que no la lleva, la NEGRITA. Una forma sin barra y
// sin negrita deja el ítem activo marcado solo por un paso de tono.
test('la forma que no lleva barra de «estás aquí» lleva el ítem activo en negrita', async () => {
  const tema = (await Bun.file(new URL('./theme.css', import.meta.url)).text()).replace(
    /\/\*[\s\S]*?\*\//g,
    '',
  );
  const bloque = (apertura: string): Map<string, string> => {
    const i = tema.indexOf(apertura);
    expect(i, `theme.css no tiene los tokens de forma en ${apertura}`).toBeGreaterThan(-1);
    const cuerpo = tema.slice(i, tema.indexOf('}', i));
    return new Map(
      [...cuerpo.matchAll(/--sytue-([a-z-]+)\s*:\s*([^;]+);/g)].map((m) => [m[1]!, m[2]!.trim()]),
    );
  };
  for (const [forma, tokens] of [
    // El bloque de Marco es el `:root` que abre con la separación de la hoja.
    ['Marco', bloque(':root {\n  --sytue-sheet-gap')],
    [
      'Carril',
      new Map([...bloque(':root {\n  --sytue-sheet-gap'), ...bloque("[data-forma='carril'] {")]),
    ],
  ] as const) {
    const barra = parseFloat(tokens.get('nav-indicator') ?? '');
    const peso = Number(tokens.get('nav-active-weight'));
    expect(Number.isFinite(barra) && Number.isFinite(peso), `${forma}: faltan tokens`).toBe(true);
    expect(barra > 0 || peso >= 600, `${forma}: barra ${barra}px y peso ${peso}`).toBe(true);
    // Y el carril de solo iconos, donde no hay rótulo: o el activo es un cuadro macizo
    // (Carril) o lleva su barra (Marco).
    if (forma === 'Marco') {
      expect(
        parseFloat(tokens.get('rail-indicator') ?? '0'),
        'Marco: barra del carril',
      ).toBeGreaterThanOrEqual(2);
    }
  }
});

/**
 * LA HOJA DE «MARCO» NO RECORTA. Llevó `overflow: clip` para redondear sus esquinas, y una
 * caja que recorta deja sin alcance lo que no contiene: en la v3.11.1 la portada de Docs
 * perdía la lista de documentos en un móvil —«Recientes» empezaba en y=950 con el documento
 * acabado en 664— y no había forma de llegar a ella. Las esquinas las redondea lo de dentro
 * (`.sytue-sheet-corners`); esto impide que el recorte vuelva por la puerta de atrás, con
 * `overflow` o con cualquier otra propiedad que recorte la pintura o la contenga.
 */
test('la hoja de la forma Marco no recorta lo que lleva dentro', () => {
  const RECORTA =
    /(^|[;{\s])(overflow(-[xy]|-block|-inline)?|clip-path|clip|contain|mask(-image)?)\s*:/;
  const reglas = [...hojas[1]!.matchAll(/([^{}]+)\{([^{}]*)\}/g)]
    .map((m) => ({ selector: m[1]!.trim(), cuerpo: m[2]! }))
    // Las reglas cuyo SUJETO es la hoja o una variante suya (a sangre, de página): ni
    // `.sytue-sheet-corners`, que es lo de dentro, ni lo que solo cuelga de una hoja.
    .filter((r) =>
      r.selector.split(',').some((parte) =>
        /\.sytue-sheet(?!-corners)/.test(
          parte
            .trim()
            .split(/[\s>+~]+/)
            .pop() ?? '',
        ),
      ),
    );
  expect(reglas.length, 'appearance.css ya no tiene reglas de .sytue-sheet').toBeGreaterThan(3);
  for (const r of reglas)
    expect(RECORTA.test(r.cuerpo), `${r.selector.replace(/\s+/g, ' ')} recorta`).toBe(false);
});

test('Marco mantiene el fondo continuo en la barra móvil y en el borde del viewport', () => {
  const reglas = [...hojas[1]!.matchAll(/([^{}]+)\{([^{}]*)\}/g)].map((m) => ({
    selector: m[1]!.trim(),
    cuerpo: m[2]!,
  }));
  const barra = reglas.find(
    (r) => r.selector === ":root:not([data-forma='carril']) .sytue-mobilebar",
  );
  expect(barra, 'falta el fondo de la barra móvil de Marco').toBeDefined();
  expect(barra!.cuerpo).toMatch(/background:\s*transparent\s*;/);
  expect(barra!.cuerpo).toMatch(/color:\s*var\(--sytue-sidebar-fg\)\s*;/);
  expect(barra!.cuerpo).toMatch(/border-bottom-color:\s*transparent\s*;/);
  // Los controles de la barra leen los mismos tokens que el resto del cromo.
  const cromo = reglas.find((r) =>
    r.cuerpo.includes('--color-primary-text: var(--sytue-sidebar-fg)'),
  )!;
  expect(cromo.selector).toContain('.sytue-mobilebar');
  // Html/body usan EL MISMO fondo que la carcasa y solo si hay barra de la suite.
  const fondo = reglas.find((r) => r.selector.includes(':is(.sytue-frame, .sytue-topbar-carril)'))!;
  expect(fondo.selector).toContain(":root:not([data-forma='carril']):has(.sytue-topbar-carril),");
  expect(fondo.selector).toContain(
    ":root:not([data-forma='carril']):has(.sytue-topbar-carril) body",
  );
  expect(fondo.cuerpo).toContain('var(--sytue-frame-from), var(--sytue-frame-to)');
});

test('el carril devuelve al tema todo lo que reasigna, y nada más', () => {
  const apariencia = hojas[1]!;
  /** Las propiedades que declara la regla cuyo selector acaba en `fin`. */
  const declara = (fin: string): string[] => {
    const i = apariencia.indexOf(fin + ' {');
    expect(i, `appearance.css no tiene la regla ${fin}`).toBeGreaterThan(-1);
    const cuerpo = apariencia.slice(i, apariencia.indexOf('}', i));
    return [...cuerpo.matchAll(/(--[a-z0-9-]+)\s*:/g)].map((m) => m[1]!).sort();
  };
  const carril = declara('.bg-rail');
  const vuelta = declara("[role='tooltip']\n  )");
  expect(carril.length).toBeGreaterThan(20);
  expect(vuelta).toEqual(carril);
  // La barra superior en móvil es el mismo carril: la misma lista, más su propio fondo.
  const movil = declara('.sytue-topbar-carril').filter((p) => p !== '--sytue-topbar-bg');
  expect(movil).toEqual(carril);
  // Y cada valor «de fuera» existe: una copia `--sytue-root-*` por token reasignado.
  const copias = new Set(declara('\nhtml'));
  const sinCopia = carril
    .filter((p) => p.startsWith('--sytue-'))
    .filter((p) => !copias.has(p.replace('--sytue-', '--sytue-root-')));
  expect(sinCopia).toEqual([]);
});

test('el carril reasigna cada token en los cuatro bloques, o el de `:root` pisaría al del tema', () => {
  // appearance.css va después de colors.css: un token del menú lateral que solo estuviera
  // en su `:root` ganaría, por orden, al valor que colors.css le da en oscuro y traslúcido.
  const nombres = (apertura: string): Set<string> => {
    const css = hojas[1]!;
    const i = css.indexOf(apertura);
    expect(i, `appearance.css no tiene el bloque ${apertura}`).toBeGreaterThan(-1);
    const cuerpo = css.slice(i, css.indexOf('}', i));
    return new Set([...cuerpo.matchAll(/--sytue-([a-z0-9-]+)\s*:/g)].map((m) => m[1]!));
  };
  const enColores = (apertura: string): Set<string> => {
    const css = hojas[0]!;
    const i = css.indexOf(apertura);
    const cuerpo = css.slice(i, css.indexOf('}', i));
    return new Set([...cuerpo.matchAll(/--sytue-([a-z0-9-]+)\s*:/g)].map((m) => m[1]!));
  };
  const raiz = nombres(':root {');
  const sinGemelo: string[] = [];
  for (const bloque of [
    "[data-theme='dark'] {",
    ':root:not([data-theme]) {',
    "[data-theme='translucent'] {",
  ]) {
    const aqui = nombres(bloque);
    for (const n of enColores(bloque))
      if (raiz.has(n) && !aqui.has(n)) sinGemelo.push(`${bloque} ${n}`);
  }
  expect(sinGemelo).toEqual([]);
});

test('la muestra de cada color elegible es su primario claro', () => {
  // El selector enseña también el color que NO está pintado, así que su muestra no
  // puede salir de `--sytue-primary`: es un token fijo de `:root`, y si el primario
  // de un color cambia, su muestra cambia con él.
  expect(paleta([])('swatch-petroleo')).toEqual(paleta([])('primary'));
  expect(paleta([COBALTO])('swatch-cobalto')).toEqual(paleta([COBALTO])('primary'));
});

test('cada modo de un color repite lo que su bloque claro declara, o ganaría el claro', () => {
  // `[data-color='cobalto']` y `[data-theme='dark']` pesan lo mismo y el del color va
  // después: un token que el claro del color y el modo del petróleo declaran los dos
  // tiene que estar también en el modo del color, o en oscuro saldría el valor claro.
  const nombres = (css: string, apertura: string): Set<string> => {
    const i = css.indexOf(apertura);
    if (i < 0) return new Set();
    const cuerpo = css.slice(i, css.indexOf('}', i));
    return new Set([...cuerpo.matchAll(/--sytue-([a-z0-9-]+)\s*:/g)].map((m) => m[1]!));
  };
  const faltan: string[] = [];
  for (const color of ['cobalto', 'app']) {
    for (const css of hojas) {
      const claro = nombres(css, claroDe(color));
      expect(claro.size).toBeGreaterThan(5);
      for (const [, modo] of MODOS) {
        if (!modo) continue;
        const suyo = nombres(css, modoDe(color, modo));
        for (const n of nombres(css, modo)) {
          if (claro.has(n) && !suyo.has(n)) faltan.push(`${modoDe(color, modo)} ${n}`);
        }
      }
    }
  }
  expect(faltan).toEqual([]);
});

/** El tono OKLCH de un color, en grados. */
function tono([r, g, b]: Color): number {
  const [lr, lg, lb] = [r, g, b].map((v) => {
    const c = v / 255;
    return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  }) as [number, number, number];
  const l = Math.cbrt(0.4122214708 * lr + 0.5363325363 * lg + 0.0514459929 * lb);
  const m = Math.cbrt(0.2119034982 * lr + 0.6806995451 * lg + 0.1073969566 * lb);
  const s = Math.cbrt(0.0883024619 * lr + 0.2817188376 * lg + 0.6299787005 * lb);
  return (
    (Math.atan2(
      0.0259040371 * l + 0.7827717662 * m - 0.808675766 * s,
      1.9779984951 * l - 2.428592205 * m + 0.4505937099 * s,
    ) *
      180) /
    Math.PI
  );
}

// LA FRANJA DE TONO DE `danger`. Un color de producto cuyo tono (OKLCH) cae a menos de
// 26° del de `--sytue-danger` puede confundirse con el rojo de peligro en lo pequeño:
// insignias, enlaces y anillo de foco. Dentro de la franja NO decide una fórmula. Lo
// decide el dueño, app por app, mirando la muestra del arnés de capturas —un diálogo de
// borrado con el botón principal y el de peligro juntos, un campo enfocado, otro con
// error e insignias—, y la decisión se apunta en una de las dos listas de abajo.
//
// Es un corte a la medida del catálogo y se dice tal cual: ni el tono ni la distancia de
// color separan a las que se confunden de la que no. Por tono, Bases (+17,8°) queda
// ENTRE Diapos (+16,4°) y ERP (+19,8°); por ΔE2000, Diapos (11,0), Bases (13,9) y ERP
// (14,2) quedan más cerca de `danger` que Cast (15,5).
const FRANJA_DE_PELIGRO = 26;
/** Solo tiñen el CARRIL: botón, enlace, selección y foco van en el primario de la casa. */
const SOLO_CARRIL = ['cast', 'erp', 'mail', 'pdf', 'slides'];
/** En la franja, vistas y sin confusión: Bases es marrón chocolate, más oscuro y apagado. */
const CON_ACCION_EN_LA_FRANJA = ['bases'];

function enLaFranjaDePeligro(app: string): boolean {
  const t = paleta([]);
  const desvio = ((tono(t(`product-${app}`)) - tono(t('danger')) + 540) % 360) - 180;
  return Math.abs(desvio) <= FRANJA_DE_PELIGRO;
}

test('una app en la franja de tono de peligro está decidida a propósito', () => {
  // Una app nueva cuyo color cae en la franja NO se decide sola: esto falla hasta que
  // alguien mire su muestra (`COLOR=app … capturar.sh <salida> muestra`) y la apunte.
  const decididas = [...SOLO_CARRIL, ...CON_ACCION_EN_LA_FRANJA];
  const sinDecidir = APPS.filter((app) => enLaFranjaDePeligro(app) && !decididas.includes(app));
  expect(
    sinDecidir.map(
      (app) =>
        `${app}: su color cae en la franja de tono de peligro. Mira su muestra y apúntala en ` +
        'SOLO_CARRIL (se confunde en insignias, enlaces o foco) o en CON_ACCION_EN_LA_FRANJA',
    ),
  ).toEqual([]);
  // Las listas se comparan por igualdad: una app que ya no existe, o que salió de la
  // franja al cambiar su color, tampoco se queda apuntada.
  expect(decididas.filter((app) => !APPS.includes(app) || !enLaFranjaDePeligro(app))).toEqual([]);
  expect(SOLO_CARRIL.filter((app) => CON_ACCION_EN_LA_FRANJA.includes(app))).toEqual([]);

  // Y la hoja dice lo mismo que la lista: la rampa del carril sola, o la del primario.
  const css = hojas[0]!;
  const malDeclaradas = APPS.filter((app) => {
    const nombres = [...declaraciones(css, `[data-color='app'][data-app='${app}'] {`).keys()];
    return !nombres.every((n) => n.startsWith(SOLO_CARRIL.includes(app) ? 'rail-' : 'primary-'));
  });
  expect(malDeclaradas).toEqual([]);

  // Lo que eso significa en pantalla, en los cuatro modos: en las de solo carril la
  // acción es la de la casa y el carril no lo es.
  for (const [nombre, modo] of MODOS) {
    const casa = paleta(capas(modo, 'app'));
    for (const app of SOLO_CARRIL) {
      const t = paleta(capas(modo, 'app', app));
      for (const token of ['primary', 'primary-hover', 'primary-text', 'focus-ring', 'selected']) {
        expect([app, nombre, token, t(token)]).toEqual([app, nombre, token, casa(token)]);
      }
      expect([app, nombre, t('sidebar-bg')]).not.toEqual([app, nombre, casa('sidebar-bg')]);
    }
  }
});

test('cada app de la web encuentra su color por el prefijo bajo el que se sirve', async () => {
  // La web dice qué app es con `data-app`, y lo que pone es su prefijo (`/mail`,
  // `/web`). Si una app cambia de prefijo —Pages pasó a `/web`— y la hoja no lo sabe,
  // esa app se queda en petróleo sin que nada falle. Las de la casa no llevan rampa.
  const CASA = ['one', 'account'];
  const frontend = new URL('../../../apps/', import.meta.url);
  const sinColor: string[] = [];
  for await (const fichero of new Bun.Glob('*/astro.config.*').scan(frontend.pathname)) {
    const base = /base:\s*'\/([a-z]+)'/.exec(await Bun.file(frontend.pathname + fichero).text());
    if (!base || CASA.includes(base[1]!)) continue;
    if (!hojas[0]!.includes(`[data-color='app'][data-app='${base[1]}']`)) sinColor.push(fichero);
  }
  // Y la que se sirve en la raíz —Sytue Admin en `platform.<dominio>`— publica el prefijo
  // vacío y lo dice con la primera etiqueta de su host.
  const host = /startsWith\('([a-z]+)\.'\)/.exec(
    await Bun.file(frontend.pathname + 'console/src/lib/superficies.ts').text(),
  );
  if (!host || !hojas[0]!.includes(`[data-color='app'][data-app='${host[1]}']`)) {
    sinColor.push('console, por su host');
  }
  // La ofimática (`packages/office`) sirve cuatro productos y cada uno pinta el suyo: su
  // layout pone `data-app` con la clave del producto, que tiene que tener bloque en la hoja.
  const productos = /export type ProductoDocs = ([^;]+);/.exec(
    await Bun.file(new URL('../../office/src/lib/producto.ts', import.meta.url)).text(),
  );
  for (const [, clave] of (productos?.[1] ?? '').matchAll(/'([a-z]+)'/g)) {
    if (!hojas[0]!.includes(`[data-color='app'][data-app='${clave}']`)) {
      sinColor.push(`docs, producto ${clave}`);
    }
  }
  expect(productos).not.toBeNull();
  expect(sinColor).toEqual([]);
});

// LA COPIA QUE EMBEBEN LAS APPS NATIVAS. `apps/nucleo/tokens/theme.css` es esta misma hoja
// (go:embed no sale de su módulo, y la copia la hace scripts/sync-branding.sh). Quien la
// compara es un test de Go, que quien toca theme.css desde la web no corre: el token del
// ítem activo en negrita entró sin sincronizarla y `main` se quedó en rojo. Aquí se ve.
test('la copia de theme.css que embeben las apps nativas está al día', async () => {
  const aqui = await Bun.file(new URL('./theme.css', import.meta.url)).text();
  const nativa = await Bun.file(
    new URL('../../../../nucleo/tokens/theme.css', import.meta.url),
  ).text();
  expect(
    nativa === aqui,
    'apps/nucleo/tokens/theme.css difiere de theme.css: ejecutar scripts/sync-branding.sh',
  ).toBe(true);
});
