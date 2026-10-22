// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// Vuelca los catálogos comunes de @sytue/i18n a JSON para que Go los embeba, y saca
// del motor Intl REAL los vectores contra los que se compara el puerto.
//
//   bun apps/nucleo/scripts/gen-i18n.ts     (o `go generate ./i18n`)
//
// Los catálogos siguen naciendo en TypeScript: mientras la web exista, esa es la
// fuente y esto una copia generada. No editar los .json a mano.
//
// # Lo COMÚN, y solo lo común
//
// Esto escribe en `apps/nucleo/i18n` y en ningún otro sitio. El catálogo POR APP es de
// `apps/fyne/scripts/gen-i18n.ts`, que lo deja en `apps/fyne/catalogos/<app-de-la-web>`.
// Hasta el 2026-09-22 este guion intentaba hacer también aquella mitad, y escribía en
// `<repo>/apps/apps/<app>/locales` —un `../..` de más— o sea en ningún sitio: los
// catálogos por app «generados» no llegaban nunca, y cuatro agentes se apañaron cada
// uno por su lado. Vivía además en la suite en Gio, congelada desde el 2026-09-21 y
// retirada el 2026-09-23, así que su `go:generate` apuntaba a un guion intocable.
import { mkdirSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { catalogs } from '../../frontend/packages/i18n/src/messages/index.ts';
import { intlTag, LOCALES } from '../../frontend/packages/i18n/src/locale.ts';

// SOLO EN LINUX. En macOS, bun no usa su ICU sino el de Apple, que lleva otro CLDR:
// «20 Sep 2026» donde Linux —y Chromium en todas partes— da «20 Sept 2026», «πμ» por
// «π.μ.», y el croata en cifras («05.01.2026.») en vez de «5. sij 2026.». Los datos de
// fechas se generaron una vez en un Mac y `ci-gio` (Linux) estuvo en rojo tres commits
// diciendo que estaban desfasados, con razón. La referencia es la del CI.
if (process.platform !== 'linux') {
  console.error(
    `gen-i18n: esto es ${process.platform} (ICU ${process.versions.icu}); los vectores de Intl se generan en Linux, ` +
      'que es donde los comprueba el CI. Ejecútalo allí (el piloto vale).',
  );
  process.exit(1);
}

const destino = join(dirname(fileURLToPath(import.meta.url)), '../i18n');
mkdirSync(join(destino, 'locales'), { recursive: true });
mkdirSync(join(destino, 'testdata'), { recursive: true });

const ordenado = (o: Record<string, unknown>): Record<string, unknown> =>
  Object.fromEntries(Object.keys(o).sort().map((k) => [k, o[k]]));

for (const locale of LOCALES) {
  writeFileSync(
    join(destino, 'locales', `${locale}.json`),
    `${JSON.stringify(ordenado(catalogs[locale]), null, 2)}\n`,
  );
}

// Cantidades que separan las categorías de los 26 idiomas: los teens eslavos y
// bálticos, el 2 de esloveno, irlandés y maltés, el 0 de letón, y decimales.
const cantidades = [
  0, 1, 2, 3, 4, 5, 6, 7, 10, 11, 12, 19, 20, 21, 22, 25, 100, 101, 102, 111, 1000, 1000000, 0.5,
  1.5, 2.5,
];
const numeros = [
  0, 1, 12, 999, 1234, 12345, 1234567, 1234.5, 0.125, -42, -1234.5, 1234.5678, 0.0625, 0.9996, 1e9,
];

const intl: Record<string, { tag: string; plural: string[]; numero: string[] }> = {};
for (const locale of LOCALES) {
  const tag = intlTag(locale);
  const reglas = new Intl.PluralRules(tag);
  intl[locale] = {
    tag,
    plural: cantidades.map((n) => reglas.select(n)),
    numero: numeros.map((n) => n.toLocaleString(tag)),
  };
}
// Los separadores de cada idioma, sacados de Intl y no de una tabla escrita a mano:
// x/text los trae de CLDR 32 y ya no coinciden (el francés agrupa con U+202F, y nueve
// idiomas no agrupan por debajo de 10 000).
const formato: Record<
  string,
  {
    grupo: string;
    decimal: string;
    menos: string;
    min_grupo: number;
    porcentaje: string;
    moneda: string;
    moneda_menos: string;
    lista_par: string;
    lista_entre: string;
    lista_final: string;
    monedas: Record<string, { moneda: string; moneda_menos: string; decimales: number }>;
  }
> = {};
// Las monedas del EEE y Suiza que NO son el euro: las que trae el servicio del ERP en
// `organizacion/paises` y en las obligaciones de cada país
// (`services/syerp/internal/obligaciones/paises`). Cada una con su patrón en cada idioma
// —en inglés el código va DELANTE («SEK 250.00») y en español detrás («250,00 SEK»)— y
// sus decimales, que no siempre son dos: la corona islandesa no lleva ninguno.
const MONEDAS = ['BGN', 'CHF', 'CZK', 'DKK', 'HUF', 'ISK', 'NOK', 'PLN', 'RON', 'SEK'];
/**
 * El PATRÓN de un número con estilo: se sustituye por `{n}` todo el tramo numérico
 * —cifras, separadores de millar y decimal— y se conserva el resto tal cual. Sale de
 * `formatToParts` y no de pegar un sufijo: dónde va el signo, y qué hay entre él y el
 * número, es del idioma.
 */
const patron = (partes: Intl.NumberFormatPart[]): string => {
  const numero = new Set(['integer', 'group', 'decimal', 'fraction']);
  let puesto = false;
  return partes
    .map((p) => {
      if (!numero.has(p.type)) return p.value;
      if (puesto) return '';
      puesto = true;
      return '{n}';
    })
    .join('');
};
for (const locale of LOCALES) {
  const nf = new Intl.NumberFormat(intlTag(locale));
  const parte = (n: number, tipo: string): string =>
    nf.formatToParts(n).find((p) => p.type === tipo)?.value ?? '';
  // El porcentaje: DÓNDE va el signo y qué hay entre el número y él. No es «pegar un
  // %»: en francés y en español va detrás con espacio fino (U+00A0), en inglés pegado,
  // y en turco delante —que no es de los 26, pero la forma del dato lo admite—. Se
  // guarda el PATRÓN con {n} en el hueco del número, sacado de formatToParts.
  const pc = new Intl.NumberFormat(intlTag(locale), { style: 'percent', maximumFractionDigits: 1 });
  // La moneda, en EUR, con DOS patrones: el positivo y el negativo. No basta con uno y
  // pegarle el menos delante del número —que es lo que vale para el porcentaje—: en
  // inglés el símbolo va delante («€1,234.56») y el menos va delante DEL SÍMBOLO
  // («-€1,234.56»), así que un solo patrón daría «€-1,234.56». Un abono es un importe
  // negativo y en un ERP los hay todos los días.
  const mo = new Intl.NumberFormat(intlTag(locale), { style: 'currency', currency: 'EUR' });
  formato[locale] = {
    grupo: parte(1234567, 'group'),
    decimal: parte(0.5, 'decimal'),
    menos: parte(-1, 'minusSign'),
    min_grupo: parte(1234, 'group') === '' ? 2 : 1,
    porcentaje: patron(pc.formatToParts(0.42)),
    moneda: patron(mo.formatToParts(1234.56)),
    moneda_menos: patron(mo.formatToParts(-1234.56)),
    // La enumeración de `formatList` (`Intl.ListFormat`, conjunción larga): lo que va
    // entre dos, y con tres o más, lo que va entre los primeros y antes del último. No
    // es «pegar una y»: la palabra es del idioma, y en algunos (el inglés de EE. UU.)
    // el último lleva además coma y con dos no.
    ...(() => {
      const lf = new Intl.ListFormat(intlTag(locale), { type: 'conjunction', style: 'long' });
      const literales = (xs: string[]): string[] =>
        lf.formatToParts(xs).filter((p) => p.type === 'literal').map((p) => p.value);
      const [par = ''] = literales(['a', 'b']);
      const [entre = '', final = ''] = literales(['a', 'b', 'c']);
      return { lista_par: par, lista_entre: entre, lista_final: final };
    })(),
    monedas: Object.fromEntries(
      MONEDAS.map((codigo) => {
        const f = new Intl.NumberFormat(intlTag(locale), { style: 'currency', currency: codigo });
        return [
          codigo,
          {
            moneda: patron(f.formatToParts(1234.56)),
            moneda_menos: patron(f.formatToParts(-1234.56)),
            decimales: f.resolvedOptions().maximumFractionDigits ?? 2,
          },
        ];
      }),
    ),
  };
}

// Los nombres de los 31 países del ERP (la UE, el EEE y Suiza) como los escribe
// `Intl.DisplayNames(…, { type: 'region' })`, que es lo que pinta la web en el
// asistente y en Obligaciones. No salen de golang.org/x/text: su CLDR es otro y no
// coincide letra a letra.
const PAISES = [
  'AT', 'BE', 'BG', 'CH', 'CY', 'CZ', 'DE', 'DK', 'EE', 'ES', 'FI', 'FR', 'GR', 'HR', 'HU', 'IE',
  'IS', 'IT', 'LI', 'LT', 'LU', 'LV', 'MT', 'NL', 'NO', 'PL', 'PT', 'RO', 'SE', 'SI', 'SK',
];
const paises: Record<string, Record<string, string>> = {};
for (const locale of LOCALES) {
  const dn = new Intl.DisplayNames([intlTag(locale)], { type: 'region' });
  paises[locale] = Object.fromEntries(PAISES.map((c) => [c, dn.of(c) ?? c]));
}
writeFileSync(join(destino, 'paises.json'), `${JSON.stringify(paises, null, 2)}\n`);
// Y las fechas, por lo mismo: x/text no trae formato de fechas, y el de la web es
// `dateStyle: 'medium'` y `timeStyle: 'short'` de Intl. De cada idioma se saca el PATRÓN
// (en qué orden van día, mes y año, con qué separadores y con cuántas cifras) y los doce
// meses tal y como salen en ese patrón, que no es lo mismo que «el nombre del mes».
/**
 * Las tablas de `Intl.RelativeTimeFormat(tag, { numeric: 'auto', style: 'short' })`
 * para las cuatro unidades que usa `formatTimeAgo`. Solo el PASADO: esa función nunca
 * pasa un valor positivo, y generar el futuro sería doblar la tabla por nada.
 *
 * De cada unidad salen dos cosas, y hacen falta las dos:
 *
 *   · `pasado`, el patrón por CATEGORÍA DE PLURAL con `{n}` en el hueco del número. No
 *     vale un patrón por unidad: en polaco «5 min temu» y «22 min temu» no comparten
 *     forma, y las categorías son las de CLDR, que Go ya sabe calcular.
 *   · `auto`, lo que `numeric: 'auto'` da SIN número —«ayer», «hoy», «ahora»—, por
 *     valor. Es lo que hace que no salga «hace 1 día», y sale de CLDR en los 26
 *     idiomas sin que haya que inventar una clave de catálogo.
 *
 * El número se saca con `formatToParts`, no recortando la cadena: en los idiomas donde
 * la unidad lleva cifras («2 h») un `replace` del número se llevaría la unidad por
 * delante.
 */
const relativoDe = (
  tag: string,
): Record<string, { pasado: Record<string, string>; auto: Record<string, string> }> => {
  const reglas = new Intl.PluralRules(tag);
  const auto = new Intl.RelativeTimeFormat(tag, { numeric: 'auto', style: 'short' });
  const siempre = new Intl.RelativeTimeFormat(tag, { numeric: 'always', style: 'short' });
  const conHueco = (f: Intl.RelativeTimeFormat, n: number, u: Intl.RelativeTimeFormatUnit): string =>
    f
      .formatToParts(n, u)
      .map((p) => (p.type === 'integer' ? '{n}' : p.value))
      .join('');
  // Cantidades que separan las seis categorías de CLDR en los 26 idiomas.
  const muestras = [1, 2, 3, 4, 5, 6, 11, 21, 22, 25, 101, 102, 111];
  const out: Record<string, { pasado: Record<string, string>; auto: Record<string, string> }> = {};
  for (const unidad of ['second', 'minute', 'hour', 'day'] as const) {
    const pasado: Record<string, string> = {};
    for (const n of muestras) {
      const cat = reglas.select(n);
      if (!(cat in pasado)) pasado[cat] = conHueco(siempre, -n, unidad);
    }
    const formasAuto: Record<string, string> = {};
    for (const n of [0, -1, -2]) {
      // Sin parte `integer` es una forma literal («ayer»); con ella, `auto` no aporta
      // nada sobre `always` y no hace falta guardarla.
      if (!auto.formatToParts(n, unidad).some((p) => p.type === 'integer')) {
        formasAuto[String(n)] = auto.format(n, unidad);
      }
    }
    out[unidad] = { pasado, auto: formasAuto };
  }
  return out;
};

type Parte = { t: string; v?: string; dos?: boolean };
// La parte de un RANGO lleva además de qué extremo sale: `startRange`, `endRange` o
// `shared` (la que los dos comparten, como el año de «6–12 de julio de 2026»).
type ParteRango = Parte & { src: string };
const fechas: Record<
  string,
  {
    fecha: Parte[];
    hora: Parte[];
    hora_dos: Parte[];
    meses: string[];
    periodos: string[];
    mes_ano: Parte[];
    meses_largos: string[];
    mes_corto_ano: Parte[];
    meses_corto_ano: string[];
    dias: string[];
    dias_estrechos: string[];
    dia_mes: Parte[];
    meses_dia_mes: string[];
    dias_largos: string[];
    fecha_dia2: Parte[];
    meses_dia2: string[];
    fecha_larga: Parte[];
    meses_fecha_larga: string[];
    dia_mes_largo: Parte[];
    meses_dia_mes_largo: string[];
    fecha_larga_hora: Parte[];
    meses_fecha_larga_hora: string[];
    fecha_completa: Parte[];
    meses_fecha_completa: string[];
    dias_fecha_completa: string[];
    meses_cortos: string[];
    dia_corto: Parte[];
    meses_dia_corto: string[];
    dias_dia_corto: string[];
    rango_semana: Record<'mismo_mes' | 'mismo_ano' | 'distinto', { partes: ParteRango[]; meses: string[] }>;
    relativo: Record<string, { pasado: Record<string, string>; auto: Record<string, string> }>;
  }
> = {};
for (const locale of LOCALES) {
  const tag = intlTag(locale);
  const deFecha = new Intl.DateTimeFormat(tag, { dateStyle: 'medium', timeZone: 'UTC' });
  const deHora = new Intl.DateTimeFormat(tag, { timeStyle: 'short', timeZone: 'UTC' });
  // `timeLabel` de Sytue Calendar: `{ hour: '2-digit', minute: '2-digit' }`. NO es
  // `timeStyle: 'short'`: en español la corta da «9:07» y esta «09:07».
  const deHoraDos = new Intl.DateTimeFormat(tag, { hour: '2-digit', minute: '2-digit', timeZone: 'UTC' });
  const deMesAno = new Intl.DateTimeFormat(tag, { month: 'long', year: 'numeric', timeZone: 'UTC' });
  // Un mes agrupado de un informe de Sytue AI («sept 2026»): `valor` de SytueIA.tsx,
  // `{ month: 'short', year: 'numeric' }`. No es `mes_ano` con el mes corto: en español
  // el largo lleva «de» y el corto no.
  const deMesCortoAno = new Intl.DateTimeFormat(tag, { month: 'short', year: 'numeric', timeZone: 'UTC' });
  const deDia = new Intl.DateTimeFormat(tag, { weekday: 'short', timeZone: 'UTC' });
  // Las INICIALES del mini-calendario (`weekdayNarrowLabels` de la web). No se pueden
  // sacar de las cortas cortando por la primera letra: en español el miércoles es «X».
  const deInicial = new Intl.DateTimeFormat(tag, { weekday: 'narrow', timeZone: 'UTC' });
  // El sobretítulo de Sytue Hub («LUNES, 21 DE SEPTIEMBRE»): `todayLabel` de
  // Launcher.tsx, que es `{ weekday: 'long', day: 'numeric', month: 'long' }`. Sin año,
  // y con el día de la semana delante o detrás según el idioma.
  const deDiaMes = new Intl.DateTimeFormat(tag, {
    weekday: 'long',
    day: 'numeric',
    month: 'long',
    timeZone: 'UTC',
  });
  // El 5 a las 09:07 delata si día, hora y minuto van con cero delante.
  // La fecha con el día SIEMPRE a dos cifras y el mes SIEMPRE por su nombre corto:
  // `{ day: '2-digit', month: 'short', year: 'numeric' }`, que es lo que pinta la
  // columna «modificado» del explorador de Sytue Drive (FileBrowser.tsx:429) y lo que
  // alinea una lista de ficheros. NO es `dateStyle: 'medium'` con el día rellenado:
  // medido el 2026-09-22 en los 26 idiomas, en alemán y en letón el patrón entero es
  // otro (en alemán el mes pasa de «09» a «Sept.») y en los otros 24 coincide. Por eso
  // es su propio vector y no un `if` en Go.
  const deDia2 = new Intl.DateTimeFormat(tag, {
    day: '2-digit',
    month: 'short',
    year: 'numeric',
    timeZone: 'UTC',
  });
  // La fecha LARGA sin día de la semana, `{ day: 'numeric', month: 'long', year:
  // 'numeric' }` («4 de mayo de 1990»): el cumpleaños de la ficha de Sytue Contacts
  // (`formatBirthday` de ContactDetail.tsx). Y sin año, `{ day: 'numeric', month:
  // 'long' }` («4 de mayo»), la fecha de la lista de próximos cumpleaños
  // (BirthdaysModal.tsx) y `dayMonthLabel` de Sytue Calendar. Vectores propios por lo
  // mismo que `fecha_dia2`: el mes largo de cada patrón puede ir en otro caso gramatical.
  const deFechaLarga = new Intl.DateTimeFormat(tag, {
    day: 'numeric',
    month: 'long',
    year: 'numeric',
    timeZone: 'UTC',
  });
  const deDiaMesLargo = new Intl.DateTimeFormat(tag, { day: 'numeric', month: 'long', timeZone: 'UTC' });
  // La fecha larga CON HORA, `{ dateStyle: 'long', timeStyle: 'short' }`: la fecha de
  // la ficha del visor de Sytue Fotos. No es fecha larga + «, » + hora: el conector es
  // del idioma («14 de septiembre de 2026, 12:00» en español, «… at 12:00» en inglés).
  const deFechaLargaHora = new Intl.DateTimeFormat(tag, {
    dateStyle: 'long',
    timeStyle: 'short',
    timeZone: 'UTC',
  });
  // Los otros dos de Sytue Calendar (apps/frontend/apps/calendar/src/components/dates.ts):
  // el título de la vista de DÍA, la fecha larga CON el día de la semana («martes, 22
  // de septiembre de 2026»), y `monthShortLabel`, el mes SUELTO de la columna de fecha
  // de la agenda. El suelto no es el de la fecha media: en alemán la media lleva «09» y
  // el suelto «Sept.».
  const deFechaCompleta = new Intl.DateTimeFormat(tag, {
    weekday: 'long',
    day: 'numeric',
    month: 'long',
    year: 'numeric',
    timeZone: 'UTC',
  });
  const deMesCorto = new Intl.DateTimeFormat(tag, { month: 'short', timeZone: 'UTC' });
  // El día de un grupo de «buscar hueco» (`dayFmt` de FindTimePanel.tsx): «lun, 21 sept».
  const deDiaCorto = new Intl.DateTimeFormat(tag, {
    weekday: 'short',
    day: 'numeric',
    month: 'short',
    timeZone: 'UTC',
  });
  // El título de la vista de SEMANA: `formatRange` de `{ day, month: 'long', year }`.
  // Cada lengua une los extremos a su manera y además cambia de forma según lo que
  // compartan (mismo mes, mismo año, o nada), así que se guardan los TRES patrones.
  // Los días de muestra son de una cifra para que se vea si el idioma los rellena.
  const deRango = new Intl.DateTimeFormat(tag, { day: 'numeric', month: 'long', year: 'numeric', timeZone: 'UTC' });
  const patronRango = (a: Date, b: Date): ParteRango[] =>
    deRango.formatRangeToParts(a, b).map((p) =>
      p.type === 'literal'
        ? { t: 'literal', v: p.value, src: p.source }
        : { t: p.type, dos: p.value.length === 2 && p.value.startsWith('0'), src: p.source },
    );
  // El patrón sale del par de enero y los doce meses, del extremo que diga el par (el
  // inicial si no dice nada) de doce rangos de la misma forma.
  type Par = [Date, Date] | [Date, Date, 'endRange'];
  const rangoDe = (par: (m: number) => Par): { partes: ParteRango[]; meses: string[] } => ({
    partes: patronRango(par(0)[0], par(0)[1]),
    meses: Array.from({ length: 12 }, (_, m) => {
      const [a, b, extremo] = par(m);
      const vale = (src: string): boolean => (extremo === 'endRange' ? src === 'endRange' : src !== 'endRange');
      return deRango.formatRangeToParts(a, b).find((p) => p.type === 'month' && vale(p.source))?.value ?? '';
    }),
  });
  const muestra = new Date(Date.UTC(2026, 0, 5, 9, 7));
  const patron = (f: Intl.DateTimeFormat): Parte[] =>
    f.formatToParts(muestra).map((p) =>
      p.type === 'literal'
        ? { t: 'literal', v: p.value }
        : { t: p.type, dos: p.value.length === 2 && p.value.startsWith('0') },
    );
  const parte = (f: Intl.DateTimeFormat, d: Date, tipo: string): string =>
    f.formatToParts(d).find((p) => p.type === tipo)?.value ?? '';
  fechas[locale] = {
    fecha: patron(deFecha),
    hora: patron(deHora),
    hora_dos: patron(deHoraDos),
    meses: Array.from({ length: 12 }, (_, m) => parte(deFecha, new Date(Date.UTC(2026, m, 15)), 'month')),
    periodos: [9, 21].map((h) => parte(deHora, new Date(Date.UTC(2026, 0, 5, h)), 'dayPeriod')),
    // El título del calendario («septiembre de 2026») y las cabeceras de la semana, de
    // lunes a domingo como `weekdayLabels` de la web. El mes va tal y como sale EN ese
    // patrón: en los idiomas con casos no es el mismo que el de la fecha corta.
    mes_ano: patron(deMesAno),
    meses_largos: Array.from({ length: 12 }, (_, m) => parte(deMesAno, new Date(Date.UTC(2026, m, 15)), 'month')),
    mes_corto_ano: patron(deMesCortoAno),
    meses_corto_ano: Array.from({ length: 12 }, (_, m) =>
      parte(deMesCortoAno, new Date(Date.UTC(2026, m, 15)), 'month'),
    ),
    dias: Array.from({ length: 7 }, (_, i) => deDia.format(new Date(Date.UTC(2026, 0, 5 + i)))),
    dias_estrechos: Array.from({ length: 7 }, (_, i) =>
      deInicial.format(new Date(Date.UTC(2026, 0, 5 + i))),
    ),
    // El mes y el día de la semana TAL Y COMO SALEN en el patrón de la fecha larga: en
    // los idiomas con casos no coinciden con los de la fecha corta ni con los de
    // `mes_ano` (en checo, «září» frente a «září»; en polaco, «września» frente a
    // «wrzesień»). Por eso se sacan de este formateador y no se reutilizan.
    dia_mes: patron(deDiaMes),
    meses_dia_mes: Array.from({ length: 12 }, (_, m) =>
      parte(deDiaMes, new Date(Date.UTC(2026, m, 15)), 'month'),
    ),
    dias_largos: Array.from({ length: 7 }, (_, i) =>
      parte(deDiaMes, new Date(Date.UTC(2026, 0, 5 + i)), 'weekday'),
    ),
    fecha_dia2: patron(deDia2),
    meses_dia2: Array.from({ length: 12 }, (_, m) =>
      parte(deDia2, new Date(Date.UTC(2026, m, 15)), 'month'),
    ),
    fecha_larga: patron(deFechaLarga),
    meses_fecha_larga: Array.from({ length: 12 }, (_, m) =>
      parte(deFechaLarga, new Date(Date.UTC(2026, m, 15)), 'month'),
    ),
    dia_mes_largo: patron(deDiaMesLargo),
    meses_dia_mes_largo: Array.from({ length: 12 }, (_, m) =>
      parte(deDiaMesLargo, new Date(Date.UTC(2026, m, 15)), 'month'),
    ),
    fecha_larga_hora: patron(deFechaLargaHora),
    meses_fecha_larga_hora: Array.from({ length: 12 }, (_, m) =>
      parte(deFechaLargaHora, new Date(Date.UTC(2026, m, 15)), 'month'),
    ),
    fecha_completa: patron(deFechaCompleta),
    meses_fecha_completa: Array.from({ length: 12 }, (_, m) =>
      parte(deFechaCompleta, new Date(Date.UTC(2026, m, 15)), 'month'),
    ),
    dias_fecha_completa: Array.from({ length: 7 }, (_, i) =>
      parte(deFechaCompleta, new Date(Date.UTC(2026, 0, 5 + i)), 'weekday'),
    ),
    meses_cortos: Array.from({ length: 12 }, (_, m) => deMesCorto.format(new Date(Date.UTC(2026, m, 15)))),
    dia_corto: patron(deDiaCorto),
    meses_dia_corto: Array.from({ length: 12 }, (_, m) => parte(deDiaCorto, new Date(Date.UTC(2026, m, 15)), 'month')),
    dias_dia_corto: Array.from({ length: 7 }, (_, i) => parte(deDiaCorto, new Date(Date.UTC(2026, 0, 5 + i)), 'weekday')),
    // Los meses se sacan de CADA patrón y no del formato suelto: en checo y en eslovaco
    // el rango escribe el mes en cifras («5. – 11. 1. 2026») y el suelto con su nombre.
    rango_semana: {
      mismo_mes: rangoDe((m) => [new Date(Date.UTC(2026, m, 1)), new Date(Date.UTC(2026, m, 7))]),
      // Diciembre no puede EMPEZAR un rango del mismo año que cruce de mes: ese se saca
      // del extremo FINAL de enero–diciembre. Los días son de UNA cifra en los dos
      // extremos: en croata y en italiano el rango rellena el día del final («06») y el
      // del principio no, y un 28 no lo habría delatado.
      mismo_ano: rangoDe((m) =>
        m < 11
          ? [new Date(Date.UTC(2026, m, 5)), new Date(Date.UTC(2026, 11, 3))]
          : [new Date(Date.UTC(2026, 0, 5)), new Date(Date.UTC(2026, 11, 3)), 'endRange'],
      ),
      distinto: rangoDe((m) => [new Date(Date.UTC(2025, m, 5)), new Date(Date.UTC(2026, 0, 3))]),
    },
    relativo: relativoDe(tag),
  };
}
writeFileSync(join(destino, 'fechas.json'), `${JSON.stringify(fechas, null, 2)}\n`);

// Las zonas horarias del selector del evento de Sytue Calendar: `listTimeZones()` de
// components/tz.ts, que es `Intl.supportedValuesOf('timeZone')`. Son NOMBRES IANA, no
// texto de interfaz: la lista es la misma en los 26 idiomas. Go no sabe listarlas.
writeFileSync(join(destino, 'zonas.json'), `${JSON.stringify(Intl.supportedValuesOf('timeZone'), null, 2)}\n`);

writeFileSync(join(destino, 'formato.json'), `${JSON.stringify(formato, null, 2)}\n`);

// Instantes que separan lo que un formateador de fechas suele hacer mal: día y mes de
// una cifra, diciembre, medianoche, mediodía, y un año de cinco siglos atrás.
const instantes = [
  '2026-01-05T09:07:00Z', '2026-03-31T00:00:00Z', '2026-09-20T12:00:00Z', '2026-12-25T23:59:00Z',
  '2024-02-29T13:05:00Z', '1999-05-01T07:30:00Z', '2026-07-04T18:45:00Z', '1492-10-12T04:00:00Z',
];
const intlFechas: Record<
  string,
  {
    fecha: string[];
    hora: string[];
    hora_dos: string[];
    mes_ano: string[];
    mes_corto_ano: string[];
    dia_mes: string[];
    fecha_dia2: string[];
    fecha_larga: string[];
    dia_mes_largo: string[];
    fecha_larga_hora: string[];
    fecha_completa: string[];
    mes_corto: string[];
    dia_corto: string[];
    rango_semana: string[];
    rango_largo: string[];
  }
> = {};
for (const locale of LOCALES) {
  const tag = intlTag(locale);
  intlFechas[locale] = {
    fecha_dia2: instantes.map((i) =>
      new Date(i).toLocaleDateString(tag, {
        day: '2-digit',
        month: 'short',
        year: 'numeric',
        timeZone: 'UTC',
      }),
    ),
    fecha_larga_hora: instantes.map((i) =>
      new Date(i).toLocaleString(tag, { dateStyle: 'long', timeStyle: 'short', timeZone: 'UTC' }),
    ),
    fecha: instantes.map((i) => new Date(i).toLocaleDateString(tag, { dateStyle: 'medium', timeZone: 'UTC' })),
    hora: instantes.map((i) => new Date(i).toLocaleTimeString(tag, { timeStyle: 'short', timeZone: 'UTC' })),
    hora_dos: instantes.map((i) =>
      new Date(i).toLocaleTimeString(tag, { hour: '2-digit', minute: '2-digit', timeZone: 'UTC' }),
    ),
    mes_ano: instantes.map((i) =>
      new Date(i).toLocaleDateString(tag, { month: 'long', year: 'numeric', timeZone: 'UTC' }),
    ),
    mes_corto_ano: instantes.map((i) =>
      new Date(i).toLocaleDateString(tag, { month: 'short', year: 'numeric', timeZone: 'UTC' }),
    ),
    dia_mes: instantes.map((i) =>
      new Date(i).toLocaleDateString(tag, {
        weekday: 'long',
        day: 'numeric',
        month: 'long',
        timeZone: 'UTC',
      }),
    ),
    fecha_larga: instantes.map((i) =>
      new Date(i).toLocaleDateString(tag, { day: 'numeric', month: 'long', year: 'numeric', timeZone: 'UTC' }),
    ),
    dia_mes_largo: instantes.map((i) =>
      new Date(i).toLocaleDateString(tag, { day: 'numeric', month: 'long', timeZone: 'UTC' }),
    ),
    fecha_completa: instantes.map((i) =>
      new Date(i).toLocaleDateString(tag, {
        weekday: 'long',
        day: 'numeric',
        month: 'long',
        year: 'numeric',
        timeZone: 'UTC',
      }),
    ),
    mes_corto: instantes.map((i) => new Date(i).toLocaleDateString(tag, { month: 'short', timeZone: 'UTC' })),
    dia_corto: instantes.map((i) =>
      new Date(i).toLocaleDateString(tag, { weekday: 'short', day: 'numeric', month: 'short', timeZone: 'UTC' }),
    ),
    // Cada instante contra él mismo más seis días: salen semanas del mismo mes, que
    // cruzan de mes y que cruzan de año (el del 25 de diciembre).
    rango_semana: instantes.map((i) =>
      new Intl.DateTimeFormat(tag, { day: 'numeric', month: 'long', year: 'numeric', timeZone: 'UTC' }).formatRange(
        new Date(i),
        new Date(new Date(i).getTime() + 6 * 86_400_000),
      ),
    ),
    // Y a diez días, para que el del 25 de diciembre cruce de AÑO.
    rango_largo: instantes.map((i) =>
      new Intl.DateTimeFormat(tag, { day: 'numeric', month: 'long', year: 'numeric', timeZone: 'UTC' }).formatRange(
        new Date(i),
        new Date(new Date(i).getTime() + 10 * 86_400_000),
      ),
    ),
  };
}

// Tamaños de fichero: el MISMO cálculo que `formatBytes` de @sytue/ui (un test de Go
// vigila que aquel no cambie), con Intl poniendo el separador decimal de cada idioma.
const tamanos = [0, 512, 1023, 1024, 1536, 10240, 1572864, 5e9, 2.5e12, 1e15, 9.96 * 1024 ** 2, -1];
const comoBytes = (tag: string, n: number): string => {
  if (!Number.isFinite(n) || n < 0) return '—';
  if (n < 1024) return `${n.toLocaleString(tag)} B`;
  const unidades = ['KB', 'MB', 'GB', 'TB'];
  let v = n / 1024;
  let i = 0;
  while (v >= 1024 && i < unidades.length - 1) {
    v /= 1024;
    i += 1;
  }
  const d = v < 10 ? 1 : 0;
  return `${v.toLocaleString(tag, { minimumFractionDigits: d, maximumFractionDigits: d })} ${unidades[i]}`;
};
const intlBytes: Record<string, string[]> = {};
for (const locale of LOCALES) intlBytes[locale] = tamanos.map((n) => comoBytes(intlTag(locale), n));

// Porcentajes: el 0 y el 100 (donde el espacio duro se ve), un decimal que se redondea
// y uno que se corta, y un negativo.
const porcentajes = [0, 0.42, 1, 0.005, 0.1234, 0.9996, -0.075];
const intlPorcentaje: Record<string, string[]> = {};
for (const locale of LOCALES) {
  const nf = new Intl.NumberFormat(intlTag(locale), { style: 'percent', maximumFractionDigits: 1 });
  intlPorcentaje[locale] = porcentajes.map((v) => nf.format(v));
}

// Importes, en CÉNTIMOS, que es como viajan: el cero, menos de un euro (donde se ve el
// cero de relleno de los decimales), el corte del separador de millar, un negativo —un
// abono— y una cifra grande.
const importes = [0, 5, 99, 100, 123456, 999999, -123456, -5, 123456789];

// Enumeraciones de uno a cuatro elementos, para `formatList`. Iker, Hilario e hierro
// están por el español, donde la «y» pasa a «e» delante de «i» e «hi» (no de «hie»).
const listas = [
  ['Ana'], ['Ana', 'Luis'], ['Ana', 'Luis', 'Eva'], ['Ana', 'Luis', 'Eva', 'Iker'],
  ['Eva', 'Hilario'], ['Ana', 'hierro'],
];
const intlLista: Record<string, string[]> = {};
for (const locale of LOCALES) {
  const lf = new Intl.ListFormat(intlTag(locale), { type: 'conjunction', style: 'long' });
  intlLista[locale] = listas.map((xs) => lf.format(xs));
}
const intlMoneda: Record<string, string[]> = {};
for (const locale of LOCALES) {
  const nf = new Intl.NumberFormat(intlTag(locale), { style: 'currency', currency: 'EUR' });
  intlMoneda[locale] = importes.map((c) => nf.format(c / 100));
}
// Los mismos importes en cada moneda que no es el euro: `formatearDinero(c, locale,
// moneda)` del ERP. Con la corona islandesa se ve el redondeo a cero decimales.
const intlMonedas: Record<string, Record<string, string[]>> = {};
for (const locale of LOCALES) {
  intlMonedas[locale] = Object.fromEntries(
    MONEDAS.map((codigo) => {
      const nf = new Intl.NumberFormat(intlTag(locale), { style: 'currency', currency: codigo });
      return [codigo, importes.map((c) => nf.format(c / 100))];
    }),
  );
}

// Tiempo relativo: las formas literales de `auto` (0 y −1), el singular, y las
// cantidades que separan las categorías de plural eslavas y bálticas.
const relativos: [number, Intl.RelativeTimeFormatUnit][] = [
  [0, 'second'], [-1, 'minute'], [-2, 'minute'], [-5, 'minute'], [-22, 'minute'],
  [-1, 'hour'], [-3, 'hour'], [-11, 'hour'], [-1, 'day'], [-2, 'day'], [-101, 'day'],
];
const intlRelativo: Record<string, string[]> = {};
for (const locale of LOCALES) {
  const rtf = new Intl.RelativeTimeFormat(intlTag(locale), { numeric: 'auto', style: 'short' });
  intlRelativo[locale] = relativos.map(([n, u]) => rtf.format(n, u));
}

writeFileSync(
  join(destino, 'testdata', 'intl.json'),
  `${JSON.stringify({ cantidades, numeros, intl, instantes, intl_fechas: intlFechas, tamanos, intl_bytes: intlBytes, porcentajes, intl_porcentaje: intlPorcentaje, importes, intl_moneda: intlMoneda, intl_monedas: intlMonedas, relativos, intl_relativo: intlRelativo, listas, intl_lista: intlLista }, null, 2)}\n`,
);
