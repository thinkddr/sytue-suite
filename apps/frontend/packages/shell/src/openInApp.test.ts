/**
 * `sameSuite` es la verja de un deep link entrante.
 *
 * Un `sytue://open?url=…` lo puede emitir cualquiera —una web, un correo, un PDF— y el
 * sistema operativo no comprueba quién. Si el webview móvil navegara a lo que venga,
 * bastaría un enlace para llevárselo, con la sesión dentro, a un sitio ajeno con aspecto
 * de Sytue. Cada caso rechazado de abajo es un intento real de conseguirlo.
 */
import { describe, expect, it, afterEach } from 'bun:test';
import { readdirSync, readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { sameSuite, deepLinkFor, PRODUCT_SCHEMES } from './openInApp.js';

/** El apex se deriva del host de la página, así que hay que fijarlo por caso. */
function enHost(hostname: string): void {
  Object.defineProperty(globalThis, 'window', {
    value: { location: { hostname } },
    configurable: true,
    writable: true,
  });
}

afterEach(() => {
  Reflect.deleteProperty(globalThis, 'window');
});

describe('sameSuite', () => {
  it('rechaza dominios ajenos y los que imitan el nuestro', () => {
    enHost('one.sytue.eu');
    for (const host of [
      'evil.example',
      'sytue.eu.evil.com', // sufijo que imita
      'notsytue.eu', // prefijo pegado, sin punto
      'xsytue.eu',
      '',
    ]) {
      expect(sameSuite(host)).toBe(false);
    }
  });

  it('acepta el apex y sus subdominios: es lo que hace posible saltar de app a app', () => {
    enHost('one.sytue.eu');
    expect(sameSuite('sytue.eu')).toBe(true);
    expect(sameSuite('docs.sytue.eu')).toBe(true);
    expect(sameSuite('DRIVE.SYTUE.EU')).toBe(true);
  });

  it('sigue al entorno: en dev el apex es sytue.localhost, y prod queda fuera', () => {
    enHost('one.sytue.localhost');
    expect(sameSuite('docs.sytue.localhost')).toBe(true);
    expect(sameSuite('docs.sytue.eu')).toBe(false);
  });
});

describe('deepLinkFor', () => {
  it('codifica el destino para que el lado nativo lo lea entero', () => {
    enHost('one.sytue.eu');
    expect(deepLinkFor('https://docus.sytue.eu/docus/edit?id=a&b=c')).toBe(
      'sytue-docs://open?url=https%3A%2F%2Fdocus.sytue.eu%2Fdocus%2Fedit%3Fid%3Da%26b%3Dc',
    );
  });

  // Cada producto es una app aparte y registra SU esquema. Emitir siempre `sytue://`
  // abriría el lanzador de la suite —o, si varias apps reclamaran ese esquema, la que el
  // sistema operativo eligiera— en vez del producto que toca.
  it('usa el esquema del producto que indica el subdominio', () => {
    enHost('one.sytue.eu');
    expect(deepLinkFor('https://mail.sytue.eu/')).toStartWith('sytue-mail://');
    expect(deepLinkFor('https://drive.sytue.eu/')).toStartWith('sytue-drive://');
    expect(deepLinkFor('https://calendar.sytue.eu/')).toStartWith('sytue-calendar://');
    // La ofimática, con host propio por producto (ADR 0076): el host basta.
    expect(deepLinkFor('https://docus.sytue.eu/docus/edit?id=x')).toStartWith('sytue-docs://');
    expect(deepLinkFor('https://calcus.sytue.eu/calcus/edit?id=x')).toStartWith('sytue-sheets://');
    expect(deepLinkFor('https://diapos.sytue.eu/diapos/edit?id=x')).toStartWith('sytue-slides://');
    expect(deepLinkFor('https://certipdf.sytue.eu/certipdf/pdf?path=x')).toStartWith(
      'sytue-pdf://',
    );
    // Un host que no es de ningún producto cae al Hub, que es `sytue-one` y no `sytue`:
    // `sytue://` lo registraba el escritorio de Wails, retirado el 2026-09-23.
    expect(deepLinkFor('https://sytue.eu/')).toStartWith('sytue-one://');
  });

  // Quien conoce el producto —el editor— puede decirlo y manda sobre el host: en el host
  // de la organización (`corporate.sytue.eu/calcus/…`) el host no dice nada.
  it('acepta el producto explícito', () => {
    enHost('corporate.sytue.eu');
    const url = 'https://corporate.sytue.eu/calcus/acme/edit?id=x';
    expect(deepLinkFor(url, 'sheets')).toStartWith('sytue-sheets://');
    expect(deepLinkFor(url, 'slides')).toStartWith('sytue-slides://');
    expect(deepLinkFor(url, 'pdf')).toStartWith('sytue-pdf://');
    expect(deepLinkFor(url)).toStartWith('sytue-one://');
  });
});

/**
 * El mismo contrato vive en TRES sitios —esta tabla, el `esquema` de cada
 * `apps/fyne/cmd/<app>/FyneApp.toml` y la de `apps/mobile/suite/products.ts`— y los
 * tres lo decían en un comentario que nadie ejecuta. Se desincronizaron: Sytue PDF
 * tenía app de escritorio desde el principio y no estaba ni en el móvil ni aquí, así
 * que «abrir en la aplicación» sobre un PDF abría Sytue Docs y no había app móvil que
 * publicar.
 */
describe('el contrato de productos cuadra en los tres sitios', () => {
  const raiz = fileURLToPath(new URL('../../../../../', import.meta.url));
  const clavesDe = (texto: string, patron: RegExp): Set<string> =>
    new Set(Array.from(texto.matchAll(patron), (m) => m[1] as string));

  it('declara los mismos productos que el escritorio y el móvil', () => {
    // Cada app de Fyne declara `esquema = "sytue-<clave>"`; la demo no es un producto.
    const cmd = `${raiz}apps/fyne/cmd/`;
    const escritorio = clavesDe(
      readdirSync(cmd)
        .filter((app) => app !== 'demo')
        .map((app) => readFileSync(`${cmd}${app}/FyneApp.toml`, 'utf8'))
        .join('\n'),
      /^\s*esquema\s*=\s*"sytue-([a-z]+)"/gm,
    );
    const movil = clavesDe(
      readFileSync(`${raiz}apps/mobile/suite/products.ts`, 'utf8'),
      /key: ["']([a-z]+)["']/g,
    );
    // `rooms` tiene proyecto móvil propio (apps/mobile/rooms, con push nativo).
    movil.add('rooms');
    // DevX y Admin no tienen app de Fyne: las suyas eran del escritorio de Wails, que
    // se retiró el 2026-09-23. El esquema se queda en la web, que así sigue ofreciendo
    // la descarga en vez de abrir la app que no toca.
    for (const sinApp of ['cloud', 'admin']) escritorio.add(sinApp);

    expect(escritorio.size).toBeGreaterThan(5); // el fichero se leyó de verdad
    for (const clave of escritorio) {
      expect(PRODUCT_SCHEMES).toHaveProperty(clave);
      expect(movil).toContain(clave);
    }
    for (const [clave, esquema] of Object.entries(PRODUCT_SCHEMES)) {
      expect(escritorio).toContain(clave);
      // Por VALOR y no solo por clave: `one: 'sytue'` pasaba con las claves iguales y
      // lanzaba un esquema que ninguna app instalada atiende.
      expect(esquema).toBe(`sytue-${clave}`);
    }
  });
});
