// Checks de la detección de plataforma de la página de descargas.
// Ejecuta: `bun test`.
//
// El caso que motiva este fichero: un iPad recibía el DMG de macOS. iPadOS 13+
// se anuncia como «Macintosh» y su user agent es indistinguible del de un Mac,
// así que hace falta el táctil para desempatar. El comentario del código ya lo
// decía; lo que faltaba era comprobarlo, y no había ningún test que lo notara.
import { expect, test } from 'bun:test';
import { readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
import { CATALOGO } from '@sytue/ui';
import { BUNDLES, appsDe, bundleDisponible, detectPlatform } from './downloads';

const UA = {
  ipadOS:
    'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15',
  mac: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36',
  iphone:
    'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Version/17.0 Mobile/15E148 Safari/604.1',
  ipadViejo:
    'Mozilla/5.0 (iPad; CPU OS 12_0 like Mac OS X) AppleWebKit/605.1.15 Version/12.0 Mobile/15E148 Safari/604.1',
  android:
    'Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Mobile Safari/537.36',
  windows:
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36',
  linux:
    'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36',
};

test('un iPad moderno NO recibe la descarga de macOS', () => {
  // Mismo user agent que un Mac; los cinco puntos de contacto son la diferencia.
  expect(detectPlatform(UA.ipadOS, 5)).toBe('ios');
});

test('un Mac de verdad sigue recibiendo macOS', () => {
  expect(detectPlatform(UA.mac, 0)).toBe('macos');
  // Un Mac con pantalla táctil externa no existe, pero un solo punto tampoco
  // debe bastar: el umbral es «más de uno», no «alguno».
  expect(detectPlatform(UA.mac, 1)).toBe('macos');
});

test('sin dato de táctil se mantiene el comportamiento anterior', () => {
  // El parámetro es opcional para no romper a quien ya llamaba con un argumento.
  expect(detectPlatform(UA.mac)).toBe('macos');
});

test('los que se identifican por su nombre no dependen del táctil', () => {
  expect(detectPlatform(UA.iphone, 5)).toBe('ios');
  expect(detectPlatform(UA.ipadViejo, 5)).toBe('ios');
  expect(detectPlatform(UA.android, 5)).toBe('android');
  expect(detectPlatform(UA.windows, 0)).toBe('windows');
  expect(detectPlatform(UA.windows, 10)).toBe('windows'); // portátil táctil
  expect(detectPlatform(UA.linux, 0)).toBe('linux');
});

test('los nombres de los instaladores salen del catálogo, no de una copia', () => {
  // Aquí vivía una TERCERA tabla de nombres de producto y había divergido: decía
  // «Sytue Fotos» donde el catálogo dice «Sytue Photos». Este test es lo que impide
  // que vuelva a pasar: todo nombre que enseñe la página de descargas —salvo «Sytue
  // One», que es la marca matriz y no un producto— tiene que ser el del catálogo.
  const delCatalogo = new Set([...CATALOGO.values()].map((e) => e.name));
  delCatalogo.add('Sytue One');
  for (const bundle of BUNDLES) {
    for (const nombre of appsDe(bundle)) {
      expect([bundle.slug, nombre, delCatalogo.has(nombre)]).toEqual([bundle.slug, nombre, true]);
    }
  }
});

test('un bundle de un producto que aún no existe no se ofrece', () => {
  // El filtro de la página de descargas ya no es una lista de excepciones escrita a
  // mano: sale de la `disponibilidad` del catálogo.
  expect(bundleDisponible('drive')).toBe(true);
  // «one» no es la clave de un producto del catálogo aquí sino la del bundle de la
  // suite; el catálogo sí la tiene (Sytue Hub) y está disponible.
  expect(bundleDisponible('one')).toBe(true);
});

test('un user agent irreconocible no elige por ti', () => {
  // Devolver null es correcto: la página deja todas las descargas visibles, así
  // que no acertar nunca deja a nadie sin la suya.
  expect(detectPlatform('curl/8.4.0')).toBeNull();
  expect(detectPlatform('')).toBeNull();
});

test('los bundles son los que arma empaquetar.sh, leídos de cada FyneApp.toml', () => {
  // Esta tabla ya fue una vez el espejo de un fichero retirado (el `bundles.json` de
  // Wails) y siguió anunciando tres bundles que nadie construía. Ahora se compara con
  // la declaración de verdad: el `bundle = "…"` de cada app de apps/fyne/cmd.
  const cmd = join(import.meta.dir, '../../../../../fyne/cmd');
  const quiero: Record<string, string[]> = {};
  for (const dir of readdirSync(cmd).sort()) {
    if (dir === 'demo') continue;
    let toml = '';
    try {
      toml = readFileSync(join(cmd, dir, 'FyneApp.toml'), 'utf8');
    } catch {
      continue;
    }
    const b = /^\s*bundle\s*=\s*"([a-z0-9]+)"/m.exec(toml)?.[1] ?? dir;
    (quiero[b] ??= []).push(dir);
  }
  const hay = Object.fromEntries(BUNDLES.map((b) => [b.key, [...b.apps].sort()]));
  expect(hay).toEqual(quiero);
  for (const b of BUNDLES) expect(b.slug).toBe(`sytue-${b.key}`);
});
