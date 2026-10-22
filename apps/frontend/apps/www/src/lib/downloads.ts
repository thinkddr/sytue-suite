/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Catálogo de descargas de las apps NATIVAS de Sytue. Cada producto de la suite se
 * instala en escritorio en uno de dos BUNDLES (ver `BUNDLES`); los nombres de fichero son
 * los que publica `.github/workflows/release-desktop.yml` con `empaquetar.sh
 * publicables`: cambiarlos aquí obliga a cambiarlos allí.
 *
 * Escritorio = Fyne (`apps/fyne`, un binario Go por producto; el Wails del ADR-0024
 * se retiró el 2026-09-23): no es una PWA ni Electron. Móvil = Capacitor (proyecto
 * nativo iOS/Android).
 *
 * Aquí no hay texto traducible: los requisitos salen del catálogo
 * (`platform.<id>.req`) y lo único que queda escrito es
 * marca —macOS, Windows, App Store, Google Play—, que no se traduce.
 */
import type { ProductKey } from '@sytue/config';
import { CATALOGO } from '@sytue/ui';

/**
 * Base de descarga. VACÍA por defecto, y es deliberado: sin ella las tarjetas de
 * escritorio dicen «próximamente» en vez de enlazar a un fichero que no existe.
 *
 * Antes apuntaba fija a `github.com/sytue-com/sytueOne/releases/latest/download`, y ahí
 * había DOS problemas encadenados. El primero, que no hay ninguna release publicada —ni
 * un solo tag `v*.*.*`, que es lo único que dispara el job `desktop-apps`—, así que los
 * tres botones daban 404. El segundo, más de fondo: ese repositorio es INTERNO, y un
 * visitante anónimo del sitio público no puede descargar de un repo privado ni aunque la
 * release exista. La base de descarga de un sitio público tiene que ser pública.
 *
 * Cuando haya binarios publicados en un destino accesible, se fija
 * PUBLIC_SYTUE_DOWNLOADS_BASE y los botones se encienden solos. Es el mismo criterio que
 * ya seguían las fichas de tienda y el APK: no enlazar a un asset que no existe.
 */
const DOWNLOADS_BASE = import.meta.env.PUBLIC_SYTUE_DOWNLOADS_BASE ?? '';

/**
 * Escritorio y móvil se separan en el TIPO, no en un booleano, porque su ficha
 * difiere de verdad: en escritorio el formato es un artefacto traducible
 * («Instalador .exe») y en móvil es el nombre de una tienda, que es marca.
 */
export type DesktopId = 'macos' | 'windows' | 'linux';
export type MobileId = 'ios' | 'android';
export type PlatformId = DesktopId | MobileId;

export interface DesktopTarget {
  id: DesktopId;
  /** Nombre de la plataforma: marca, no se traduce. */
  label: string;
  href: string;
}

export interface MobileTarget {
  id: MobileId;
  label: string;
  /** Tienda donde se publica. Marca: tampoco se traduce. */
  store: string;
  href: string;
}

/** `asset` compone la URL solo si hay base; si no, cadena vacía ⇒ «próximamente». */
const asset = (file: string): string => (DOWNLOADS_BASE ? `${DOWNLOADS_BASE}/${file}` : '');

/**
 * Plataformas de escritorio realmente publicadas, separadas por comas
 * (`windows,linux`). Vacía ⇒ las tres, que es el comportamiento de siempre.
 *
 * Existe porque las plataformas no llegan a la vez: macOS necesita compilarse en
 * un Mac —Fyne enlaza contra Cocoa y no cruza-compila; hoy lo hace Codemagic— y
 * mientras no salga solo hay Windows y Linux. Sin esta lista habría que elegir entre
 * dejar las descargas apagadas del todo o encender un botón de macOS que da 404;
 * con ella se publica lo que hay y el resto sigue diciendo «próximamente», que
 * es lo que este fichero defiende desde el principio.
 */
const PUBLISHED: readonly string[] = (import.meta.env.PUBLIC_SYTUE_DOWNLOADS_PLATFORMS ?? '')
  .split(',')
  .map((p: string) => p.trim())
  .filter(Boolean);

/** URL del instalador de una plataforma, o '' si esa plataforma no se publica. */
const platformAsset = (id: DesktopId, file: string): string =>
  PUBLISHED.length === 0 || PUBLISHED.includes(id) ? asset(file) : '';

/**
 * Nombre del fichero de un enlace ya compuesto, para que quien lo sirva pueda comprobar
 * que existe. Lo usa la página de descargas: con una base LOCAL (el `/descargas` del
 * entorno de desarrollo) solo está lo que se haya compilado en este equipo —macOS no
 * cruza-compila—, así que enlazar las tres plataformas de las once apps daría enlaces
 * rotos para las otras dos plataformas.
 */
export function assetFilename(href: string): string {
  return href.slice(href.lastIndexOf('/') + 1);
}

/** ¿La base es una ruta servida por esta misma web (y por tanto comprobable en disco)? */
export const DOWNLOADS_ARE_LOCAL: boolean = DOWNLOADS_BASE.startsWith('/');

/**
 * Directorio de app de Fyne (`apps/fyne/cmd/<dir>`) → clave del CATÁLOGO. Casi todos se
 * llaman igual; los que no, por su nombre de siempre en el código nativo: el Hub es la
 * clave `one` del catálogo, y Docus, Calcus, Diapos y CertiPDF son docs, sheets, slides y
 * pdf.
 */
const CLAVE_DE_DIRECTORIO: Record<string, ProductKey> = {
  hub: 'one',
  docus: 'docs',
  calcus: 'sheets',
  diapos: 'slides',
  certipdf: 'pdf',
};

/**
 * Nombre de marca de una app de escritorio, por su directorio de app en `BUNDLES`.
 *
 * SALE DEL CATÁLOGO, no de una copia. Aquí había una tercera tabla de nombres de
 * producto —tras la del catálogo y la de `packages/office/src/lib/producto.ts`, que ya se
 * arregló— y había divergido exactamente como se esperaba: decía «Sytue Fotos» donde el
 * catálogo dice «Sytue Photos», así que la página de descargas prometía un programa con
 * un nombre que no existe en ninguna otra superficie.
 */
function nombreDeApp(dir: string): string {
  return CATALOGO.get(CLAVE_DE_DIRECTORIO[dir] ?? (dir as ProductKey))?.name ?? dir;
}

/**
 * Los instaladores de escritorio se reparten en BUNDLES, no en una app por fichero.
 *
 * Quien quiere escribir un documento no debería instalar tres programas para poder abrir
 * además una hoja de cálculo y una presentación. Se reparten como los reparte el resto de
 * la industria —Office es un instalador, no seis—: la suite de trabajo y el disco a solas
 * para quien solo quiere eso. En el móvil no hay bundles: una app por producto.
 *
 * Cada app SIGUE siendo un binario independiente con su icono, su identificador de
 * paquete y su esquema de deep link. Un bundle es una caja, no un programa.
 *
 * ES UN ESPEJO de lo que declara cada `apps/fyne/cmd/<app>/FyneApp.toml` (`bundle`), que
 * es de donde `apps/fyne/scripts/empaquetar.sh` arma los instaladores; el test de este
 * fichero los compara. `slug` es el prefijo del fichero que publica
 * (`sytue-<bundle>-<plataforma>…`): cambiarlo aquí obliga a cambiarlo allí.
 */
export interface Bundle {
  key: string;
  /** Nombre del bundle: marca, no se traduce. */
  name: string;
  /** Prefijo de los instaladores de escritorio. */
  slug: string;
  /** Directorios de app de Fyne que van dentro. */
  apps: readonly string[];
}

export const BUNDLES: readonly Bundle[] = [
  {
    key: 'one',
    name: 'Sytue One',
    slug: 'sytue-one',
    apps: [
      'hub',
      'mail',
      'calendar',
      'contacts',
      'notes',
      'photos',
      'erp',
      'docus',
      'calcus',
      'diapos',
      'certipdf',
      'rooms',
      'cast',
      'bases',
    ],
  },
  { key: 'drive', name: 'Sytue Drive', slug: 'sytue-drive', apps: ['drive'] },
];

/**
 * ¿Se puede ofrecer el instalador de este bundle? Lo dice el CATÁLOGO: la clave del
 * bundle es la del producto que le da nombre. Un bundle de un producto que todavía no
 * existe no se anuncia (encargo v4 §14). Clave que no está en el catálogo ⇒ sí, porque
 * entonces no es un producto y nadie ha declarado lo contrario.
 */
export function bundleDisponible(key: string): boolean {
  const e = CATALOGO.get(key as ProductKey);
  return !e || (e.disponibilidad !== 'desarrollo' && e.disponibilidad !== 'no-disponible');
}

/** Nombres de marca de lo que trae un bundle, para enseñarlos en la ficha. */
export function appsDe(bundle: Bundle): readonly string[] {
  return bundle.apps.map(nombreDeApp);
}

/** Descargas de escritorio de UN producto. Vacías mientras no haya base pública. */
export function desktopFor(slug: string): readonly DesktopTarget[] {
  return [
    { id: 'macos', label: 'macOS', href: platformAsset('macos', `${slug}-macos-universal.dmg`) },
    {
      id: 'windows',
      label: 'Windows',
      href: platformAsset('windows', `${slug}-windows-amd64-installer.exe`),
    },
    { id: 'linux', label: 'Linux', href: platformAsset('linux', `${slug}-linux-amd64.tar.xz`) },
  ];
}

/**
 * Descargas de la SUITE. La página pública ofrece hoy solo esta: con `DOWNLOADS_BASE`
 * vacío —el repositorio es interno y no hay destino público— las once dirían
 * «próximamente», así que un selector de producto no añadiría nada todavía. El catálogo de
 * arriba ya está listo para cuando lo haya.
 */
export const DESKTOP: readonly DesktopTarget[] = desktopFor('sytue-one');

// Fichas de tienda: hasta que existan, `href` va vacío y la tarjeta se muestra
// como «próximamente» en vez de enlazar a un listado que aún no está publicado.
export const MOBILE: readonly MobileTarget[] = [
  {
    id: 'ios',
    label: 'iOS',
    store: 'App Store',
    href: import.meta.env.PUBLIC_SYTUE_STORE_IOS ?? '',
  },
  {
    id: 'android',
    label: 'Android',
    store: 'Google Play',
    href: import.meta.env.PUBLIC_SYTUE_STORE_ANDROID ?? '',
  },
];

/**
 * Plataforma probable del visitante a partir del user agent. Solo decide qué
 * botón se destaca — el resto siguen visibles y accesibles, así que un fallo de
 * detección nunca deja a nadie sin su descarga.
 */
export function detectPlatform(ua: string, maxTouchPoints = 0): PlatformId | null {
  if (/android/i.test(ua)) return 'android';
  if (/iphone|ipad|ipod/i.test(ua)) return 'ios';
  if (/macintosh|mac os x/i.test(ua)) {
    // iPadOS 13+ se anuncia como Macintosh y su user agent es INDISTINGUIBLE del
    // de un Mac. Lo desempata el táctil: ningún Mac reporta puntos de contacto y
    // un iPad reporta cinco. El comentario decía esto desde el principio y el
    // código no lo comprobaba, así que a un iPad se le ofrecía el DMG de macOS —
    // un fichero que no puede ni abrir.
    return maxTouchPoints > 1 ? 'ios' : 'macos';
  }
  if (/windows/i.test(ua)) return 'windows';
  if (/linux|x11/i.test(ua)) return 'linux';
  return null;
}
