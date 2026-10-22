/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL TEST QUE SE QUEJA CUANDO ALGUIEN VUELVE A COPIAR Y PEGAR.
 *
 * La plomería de una superficie —middleware de idioma, getter de access token,
 * montaje del BFF, proxy del asistente— vive en los paquetes compartidos. Estaba
 * repetida entre cuatro y doce veces, y el fichero de referencia del middleware
 * lo pedía por escrito: «copia este fichero tal cual». Se cumplió, y para cuando
 * se midió solo cinco de las doce copias seguían siendo idénticas byte a byte.
 *
 * Se mira el CÓDIGO FUENTE y no el comportamiento porque el fallo se comete al
 * escribir, no al ejecutar: dos middlewares que negocian el idioma en distinto
 * orden pasan todos los tests de cada app por separado. Es el mismo enfoque que
 * el guardián de `role="toolbar"` en `packages/ui/src/components/Toolbar.test.tsx`.
 *
 * SI ESTE TEST TE PARA: no añadas tu fichero a `excepciones`. Lo normal es que
 * exista ya una función que hace eso. Las excepciones de abajo son sitios donde
 * el comportamiento es DE VERDAD distinto y aplastarlo sería peor que repetirlo.
 */
import { expect, test } from 'bun:test';

/**
 * Dónde se mira: las apps y la ofimática común (`packages/office`), que es código de
 * app aunque viva en un paquete —Docus, Calcus, Diapos y CertiPDF montan sus vistas y
 * su BFF—. Sin ella, todo lo que se movió allí quedaba fuera de estas reglas. Rutas
 * relativas a `apps/frontend/`, como las excepciones.
 */
const RAIZ = new URL('../../../', import.meta.url);
const FUENTES = '{apps/*/src/**/*,packages/office/src/**/*}';

interface Regla {
  /** Trozo de código literal que solo aparece cuando alguien ha copiado el cuerpo. */
  marcador: string;
  /** Qué usar en su lugar. Sale en el mensaje del fallo. */
  usa: string;
  /** Ficheros que pueden llevarlo, con el motivo. Ruta relativa a `apps/frontend/`. */
  excepciones: Record<string, string>;
}

const REGLAS: Regla[] = [
  {
    marcador: 'context.locals.locale = negotiateLocale(',
    usa: "`middlewareDeIdioma()` o `negociarIdioma()` de '@sytue/config/astro-middleware'",
    excepciones: {},
  },
  {
    marcador: "'/auth/token'",
    usa: "`getAccessToken` de '@sytue/auth/client'",
    // Sin excepciones desde el 2026-09-12. Las tres que había —docs, drive y
    // portal, con su propio `lib/token.ts`— decían que sin sesión LANZAN «para
    // que la isla lo muestre» en vez de navegar al login. Al mirarlo, ninguna de
    // las cuatro islas que lo usan lo envuelve: pasan `getAccessToken` COMO
    // REFERENCIA (`accessToken={...}`, `bearer(...)`), así que nadie mostraba ese
    // error a propósito. Lo que sí divergía de verdad era el margen de caducidad
    // —60 s contra los 30 s del resto—, que es justo el fallo que la cabecera del
    // getter compartido dice que teme.
    excepciones: {},
  },
  {
    marcador: 'function getApiSession',
    usa: "`crearBffDeApp(import.meta.env)` de '@sytue/auth/server'",
    excepciones: {
      'apps/portal/src/lib/auth.ts':
        'el Portal monta su BFF con opciones propias; su guard no es el de la suite',
    },
  },
  {
    marcador: 'function pickRequest',
    usa: "`crearProxyDeAsistente` de '@sytue/ai/astro'",
    excepciones: {
      'packages/office/src/pages/api/ai.ts':
        'Sytue Docs manda además una página rasterizada en base64: valida la imagen y su tipo, y aplica un tope distinto DESPUÉS de leer el cuerpo',
    },
  },
  // Tamaños de fichero. El 2026-09-28 quedaban NUEVE copias (Mail, Drive, Rooms ×2,
  // Notes, Console ×2, Cuenta, Descargas y la memoria de Cloud), con tres criterios de
  // decimales, `toFixed` con el punto inglés y unidades traducidas en unas sí y en
  // otras no. Cada marcador es el trozo que deja una tabla de unidades hecha a mano.
  ...["'KB'", "'kB'", "'kilobyte'", '} KB`', '} MB`', '} GB`'].map((marcador) => ({
    marcador,
    usa: "`formatBytes` de '@sytue/ui'",
    excepciones: {},
  })),
  {
    // Sytue Bases nació copiando el `lib/cabeceras.ts` de Sytue Web, con la conexión
    // limitada a `'self'`: en las dos, la shell se quedó sin Hub (campana y presencia
    // mudas) hasta el 2026-10-05. La directiva escrita a mano en una app es la copia.
    marcador: 'connect-src',
    usa: "`cabecerasCerradas(hubBaseUrl())` de '@sytue/config/astro-middleware'",
    excepciones: {},
  },
  {
    // Cinco apps decidían a mano si había un Mac delante para escribir un atajo, y
    // Rooms, Notes y el ERP ni lo miraban: «Ctrl+G» en un Mac y «⌘K» en Windows.
    marcador: 'navigator.platform',
    usa: "`atajoDelSistema`, `useAtajo` o `esMac` de '@sytue/ui'",
    excepciones: {},
  },
];

/** Lo que no se revisa: otro agente toca `pages/auth/`, y los mensajes son datos. */
function seRevisa(rel: string): boolean {
  if (rel.includes('/pages/auth/') || rel.includes('/i18n/messages/')) return false;
  return /\.(ts|tsx|astro)$/.test(rel) && !rel.includes('.test.');
}

test('la plomería compartida no se copia, se importa', async () => {
  const infractores: string[] = [];
  for await (const rel of new Bun.Glob(FUENTES).scan({ cwd: RAIZ.pathname })) {
    if (!seRevisa(rel)) continue;
    const fuente = await Bun.file(new URL(rel, RAIZ)).text();
    for (const regla of REGLAS) {
      if (!fuente.includes(regla.marcador)) continue;
      if (rel in regla.excepciones) continue;
      infractores.push(`${rel}: lleva «${regla.marcador}» — usa ${regla.usa}`);
    }
  }
  expect(infractores).toEqual([]);
});

/*
 * Que las excepciones no se queden de adorno: si alguien arregla una de esas
 * diferencias y deduplica el fichero, la excepción sobra y hay que quitarla. Sin
 * esto la lista solo crece, que es como se pierde la cuenta de la deuda.
 */
test('no quedan excepciones que ya no hagan falta', async () => {
  const sobran: string[] = [];
  for (const regla of REGLAS) {
    for (const rel of Object.keys(regla.excepciones)) {
      const fichero = Bun.file(new URL(rel, RAIZ));
      const sigue = (await fichero.exists()) && (await fichero.text()).includes(regla.marcador);
      if (!sigue) sobran.push(`${rel} ya no lleva «${regla.marcador}»: quita la excepción`);
    }
  }
  expect(sobran).toEqual([]);
});

/**
 * EL ICONO QUE SAFARI NO VUELVE A PEDIR.
 *
 * Safari guarda el favicon POR PÁGINA y no lo pide otra vez aunque el `<link>`
 * haya cambiado: quien abrió una página cuando el icono fallaba se queda sin él
 * indefinidamente. Cambiar la URL es lo único que le hace mirar de nuevo, y para
 * eso está `SUFIJO_ICONOS` (packages/ui/src/basePath.ts).
 *
 * Se comprueba aquí porque el fallo es de OMISIÓN: cuando se arreglaron las once
 * apps, el sitio público se quedó fuera y no lo notó nadie hasta que alguien
 * abrió sytue.eu en un Mac. Un `<link>` sin sufijo no rompe ningún test ni
 * ninguna página; simplemente no se ve el icono, y solo en un navegador.
 */
test('todo <link> de icono lleva el sufijo de caché', async () => {
  const sinSufijo: string[] = [];
  for await (const rel of new Bun.Glob(`${FUENTES}.astro`).scan({ cwd: RAIZ.pathname })) {
    const src = await Bun.file(new URL(rel, RAIZ)).text();
    src.split('\n').forEach((linea, i) => {
      if (!/href=/.test(linea)) return;
      if (!/favicon|apple-touch-icon/.test(linea)) return;
      if (linea.includes('SUFIJO_ICONOS')) return;
      sinSufijo.push(`${rel}:${i + 1}  ${linea.trim()}`);
    });
  }
  expect(
    sinSufijo,
    `Estos iconos no llevan SUFIJO_ICONOS, así que Safari no volverá a pedirlos:\n  ${sinSufijo.join('\n  ')}`,
  ).toEqual([]);
});
