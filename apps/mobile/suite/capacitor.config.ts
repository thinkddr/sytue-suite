import type { CapacitorConfig } from "@capacitor/cli";
import { activeProduct, productURL, suiteApex, trustedSuiteHosts } from "./products";

/**
 * Apps moviles de Sytue (Capacitor 8, MIT). Shell nativo iOS/Android que envuelve la
 * MISMA web de cada producto (codigo unico, no reimplementa editores) — mismo patron que
 * el escritorio de Wails (retirado el 2026-09-23). El código y los datos offline viven en la web compartida:
 * service worker por cuenta + IndexedDB/colas por producto; el binario no duplica UI.
 *
 * UN proyecto, DIEZ apps. Cada producto se publica por separado —su identificador de
 * paquete, su nombre, su icono y su esquema de deep link— pero comparte este proyecto,
 * porque diez carpetas serian diez package.json, diez node_modules y diez sitios donde
 * repetir cada arreglo. Se elige con SYTUE_APP (tabla en products.ts):
 *
 *   SYTUE_APP=mail npx cap sync android    # Sytue Mail
 *   npx cap sync android                   # sin variable: la suite (Sytue One)
 *
 * La URL sigue el mismo convenio que escritorio:
 *   SYTUE_URL=https://one.sytue.localhost npx cap run ios   # dev (Traefik + mkcert)
 */
const SYTUE_URL = process.env.SYTUE_URL ?? "https://one.sytue.eu";
const product = activeProduct();

/** Apex de la suite, derivado del entorno: sytue.eu en produccion, sytue.localhost en dev. */
const apex = suiteApex(SYTUE_URL);

const config: CapacitorConfig = {
  appId: product.appId,
  appName: product.appName,
  // Cascaron local (splash de carga); en runtime el webview navega a server.url.
  webDir: "www",
  server: {
    url: productURL(product, SYTUE_URL),
    // Solo TLS: dev usa https://*.sytue.localhost (mkcert), prod https://*.sytue.eu.
    cleartext: false,
    // Lista cerrada de superficies propias. Nunca usar *.sytue.eu: Cloud aloja
    // publicaciones no confiables en *.apps/preview.sytue.eu.
    allowNavigation: trustedSuiteHosts(apex),
  },
  ios: {
    // La carcasa web reserva env(safe-area-inset-*); UIKit no vuelve a sumarlas.
    contentInset: "never",
    // Pellizco para ampliar. Capacitor lo DESACTIVA por defecto (zoomEnabled:false ->
    // scrollViewWillBeginZooming cancela el gesto), y en un webview eso deja al usuario
    // sin NINGUNA forma de agrandar: WKWebView no atiende Dynamic Type para contenido
    // web. Incumple WCAG 1.4.4, que la casa exige (ADR-0004), y se nota justo donde mas
    // importa: un PDF, un plano o una hoja de calculo en iPad.
    zoomEnabled: true,
  },
  android: {
    // Sin contenido mixto: todo el trafico va por TLS.
    allowMixedContent: false,
    // Igual que en iOS. Los controles de zoom EN PANTALLA siguen ocultos: Capacitor
    // fija setDisplayZoomControls(false) siempre, asi que esto solo habilita el gesto.
    zoomEnabled: true,
  },
  plugins: {
    SystemBars: {
      // WebView <140: inset nativo y env=0; desde140, cover con insets CSS correctos.
      insetsHandling: "native",
      initialViewportFitValueHint: "cover",
    },
  },
};

export default config;
