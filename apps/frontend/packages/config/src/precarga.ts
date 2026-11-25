/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * `<link rel="modulepreload">` de lo que cada isla baja antes de ejecutarse.
 *
 * Sin esto el navegador descubre el JS de una isla por niveles: el trozo de la isla
 * (`component-url`), luego sus importaciones estáticas al analizarlo, luego las de
 * esas, y por último el catálogo de idioma, que espera un `await` de nivel superior
 * (`@sytue/i18n` y el `i18n/index.ts` de cada app). Medido el 2026-11-24: cuatro
 * niveles de ida y vuelta antes de que la app arranque. Con los enlaces delante de la
 * isla, todo baja a la vez en cuanto el analizador llega a ella.
 *
 * Van JUSTO ANTES de cada `<astro-island>` (`modulepreload` vale en el `<body>`) y no
 * en el `<head>`: así la respuesta sigue saliendo por partes; para escribir en el
 * `<head>` habría que esperar a la página entera para saber qué islas lleva. La isla
 * empieza a bajar su trozo en ese mismo punto, así que no se pierde nada.
 *
 * Solo las islas que se hidratan sin esperar a nada (`client:load`, `client:idle`,
 * `client:only`): precargar una `client:visible` desharía su pereza.
 *
 * Aquí está la parte pura —el manifiesto a partir del bundle y la inyección en el
 * HTML— para probarla sin Vite ni Astro; la integración (`astro-precarga.ts`) la
 * engancha al build y el middleware (`astro-precarga-middleware.ts`) al servidor.
 */

/** Lo que el manifiesto necesita de cada trozo del bundle de cliente. */
export interface TrozoDeCliente {
  fileName: string;
  isEntry: boolean;
  imports: readonly string[];
  dynamicImports: readonly string[];
  facadeModuleId: string | null;
}

/** Por trozo de entrada: su cierre estático y, por idioma, el trozo de catálogo que espera. */
export type ManifiestoDePrecarga = Record<
  string,
  { js: string[]; idiomas?: Record<string, string[]> }
>;

/** Un catálogo de idioma: `…/messages/es.ts`, el compartido o el de una app. */
const CATALOGO = /\/messages\/([a-z]{2})\.ts$/;

export function manifiestoDePrecarga(trozos: readonly TrozoDeCliente[]): ManifiestoDePrecarga {
  const porNombre = new Map(trozos.map((t) => [t.fileName, t]));
  const cierre = (inicio: string): string[] => {
    const visto = new Set<string>();
    const pendientes = [inicio];
    while (pendientes.length) {
      const nombre = pendientes.pop()!;
      if (visto.has(nombre) || !porNombre.has(nombre)) continue;
      visto.add(nombre);
      pendientes.push(...porNombre.get(nombre)!.imports);
    }
    return [...visto];
  };
  const manifiesto: ManifiestoDePrecarga = {};
  for (const trozo of trozos) {
    if (!trozo.isEntry) continue;
    const js = cierre(trozo.fileName);
    const idiomas: Record<string, Set<string>> = {};
    for (const nombre of js)
      for (const dinamico of porNombre.get(nombre)!.dynamicImports) {
        const idioma = CATALOGO.exec(porNombre.get(dinamico)?.facadeModuleId ?? '')?.[1];
        if (!idioma) continue;
        const conjunto = (idiomas[idioma] ??= new Set());
        for (const f of cierre(dinamico)) if (!js.includes(f)) conjunto.add(f);
      }
    const conIdiomas = Object.keys(idiomas).length > 0;
    manifiesto[trozo.fileName] = conIdiomas
      ? {
          js,
          idiomas: Object.fromEntries(
            Object.entries(idiomas).map(([idioma, conjunto]) => [idioma, [...conjunto]]),
          ),
        }
      : { js };
  }
  return manifiesto;
}

const ISLA = '<astro-island';
const ATRIBUTO = (nombre: string) => new RegExp(`\\s${nombre}="([^"]*)"`);
const URL_COMPONENTE = ATRIBUTO('component-url');
const URL_RENDERER = ATRIBUTO('renderer-url');
const DIRECTIVA = ATRIBUTO('client');
const IDIOMA = /<html\b[^>]*?\slang="([^"]*)"/i;
const SIN_ESPERA = new Set(['load', 'idle', 'only']);

/** El valor de un atributo tal como sale en el HTML (Astro escapa `&`). */
const desescapar = (valor: string): string => valor.replaceAll('&amp;', '&');

/**
 * Inyecta los enlaces en un HTML que llega por partes. `push` devuelve lo que ya se
 * puede emitir; `end`, lo que quedaba retenido. Retiene como mucho una etiqueta
 * `<astro-island` a medias: el resto sale tal cual llega.
 */
export class InyectorDePrecarga {
  private pendiente = '';
  private cabeza = '';
  private idioma: string | undefined;
  private readonly emitidos = new Set<string>();

  constructor(
    private readonly manifiesto: ManifiestoDePrecarga,
    private readonly activos = '_astro',
  ) {}

  push(texto: string): string {
    if (this.idioma === undefined && this.cabeza.length < 16384) {
      this.cabeza += texto;
      const lang = IDIOMA.exec(this.cabeza)?.[1];
      if (lang !== undefined) this.idioma = lang.split('-')[0]?.toLowerCase() ?? '';
    }
    let resto = this.pendiente + texto;
    this.pendiente = '';
    let salida = '';
    for (;;) {
      const i = resto.indexOf(ISLA);
      if (i < 0) break;
      const fin = resto.indexOf('>', i);
      if (fin < 0) {
        // La etiqueta sigue en el trozo siguiente.
        salida += resto.slice(0, i);
        this.pendiente = resto.slice(i);
        return salida;
      }
      salida += resto.slice(0, i) + this.enlaces(resto.slice(i, fin + 1)) + resto.slice(i, fin + 1);
      resto = resto.slice(fin + 1);
    }
    // Un `<astro-is` cortado al final del trozo: se guarda hasta ver el resto.
    const corte = resto.lastIndexOf('<');
    if (corte >= 0 && ISLA.startsWith(resto.slice(corte))) {
      this.pendiente = resto.slice(corte);
      resto = resto.slice(0, corte);
    }
    return salida + resto;
  }

  end(): string {
    const resto = this.pendiente;
    this.pendiente = '';
    return resto;
  }

  private enlaces(etiqueta: string): string {
    if (!SIN_ESPERA.has(DIRECTIVA.exec(etiqueta)?.[1] ?? '')) return '';
    let html = '';
    for (const url of [URL_COMPONENTE, URL_RENDERER].map((a) => a.exec(etiqueta)?.[1])) {
      if (!url) continue;
      const ruta = desescapar(url);
      const corte = ruta.lastIndexOf(`/${this.activos}/`);
      if (corte < 0) continue;
      const prefijo = ruta.slice(0, corte + 1);
      const entrada = this.manifiesto[ruta.slice(corte + 1)];
      if (!entrada) continue;
      const idioma = this.idioma ? entrada.idiomas?.[this.idioma] : undefined;
      for (const fichero of [...entrada.js, ...(idioma ?? [])]) {
        if (this.emitidos.has(fichero)) continue;
        this.emitidos.add(fichero);
        html += `<link rel="modulepreload" href="${prefijo}${fichero}">`;
      }
    }
    return html;
  }
}

/** La misma inyección sobre un HTML entero (las páginas prerrenderizadas). */
export function inyectarPrecarga(
  html: string,
  manifiesto: ManifiestoDePrecarga,
  activos = '_astro',
): string {
  const inyector = new InyectorDePrecarga(manifiesto, activos);
  return inyector.push(html) + inyector.end();
}
