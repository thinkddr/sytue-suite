import type { CapacitorConfig } from "@capacitor/cli";

/**
 * Sytue Rooms — app movil (Capacitor 8, MIT). Cascaron nativo iOS/Android que
 * ENVUELVE la web de Rooms (apps/frontend/apps/rooms, servida en rooms.sytue.eu):
 * el webview navega a `server.url` (el PWA remoto) — NO se duplica la UI. Mismo
 * patron «shell que carga Rooms» que tenia la de escritorio en Wails (retirada). Producto distinto
 * del shell Sytue One de la carpeta padre (que carga one.sytue.eu).
 *
 * URL de Rooms configurable via SYTUE_ROOMS_URL (el convenio de la de escritorio en Wails):
 *   SYTUE_ROOMS_URL=https://rooms.sytue.localhost npx cap run ios   # dev (Traefik + mkcert)
 * Sin variable => produccion (https://rooms.sytue.eu).
 *
 * Integracion nativa (push, deep-links, status bar, splash, teclado, badge, back):
 * la wiring corre DENTRO del webview desde `www/native.js` (habla con el puente
 * global window.Capacitor). Como Rooms es un origen REMOTO, ese script se inyecta
 * en la pagina (WKUserScript iOS / evaluateJavascript Android) — ver README §Puente.
 */
const DEFAULT_ROOMS_URL = "https://rooms.sytue.eu";

function trustedRoomsURL(raw: string | undefined): URL {
  try {
    const parsed = new URL(raw ?? DEFAULT_ROOMS_URL);
    const prod = parsed.protocol === "https:" && parsed.hostname === "rooms.sytue.eu";
    const local = (parsed.protocol === "https:" || parsed.protocol === "http:") &&
      parsed.hostname === "rooms.sytue.localhost";
    if ((prod || local) && !parsed.username && !parsed.password) {
      return parsed;
    }
  } catch {
    // La configuración inválida cae a la superficie de producción cerrada.
  }
  return new URL(DEFAULT_ROOMS_URL);
}

const roomsURL = trustedRoomsURL(process.env.SYTUE_ROOMS_URL);
const ROOMS_URL = roomsURL.toString().replace(/\/+$/, "");

// Host del origen de Rooms (en dev, el local de mkcert). Es el que sirve la app.
const roomsHost = roomsURL.hostname;

// Sytue ID, derivado del MISMO apex que Rooms (rooms.sytue.eu -> id.sytue.eu,
// rooms.sytue.localhost -> id.sytue.localhost). trustedRoomsURL ya garantiza que el
// host es uno de esos dos, asi que esto no puede componer un dominio ajeno.
//
// Sin este host el LOGIN ES IMPOSIBLE. Rooms manda al visitante sin sesion a
// /auth/login, que redirige a id.*/oauth/authorize (OIDC + PKCE), y Capacitor aplica
// allowNavigation tambien a las REDIRECCIONES del servidor —iOS en
// WebViewDelegationHandler.decidePolicyFor, Android en shouldOverrideUrlLoading—, no
// solo a los clics. Con id.* fuera de la lista, el webview cancela la navegacion y la
// abre en el navegador del sistema: el usuario se autentica ALLI, la cookie de sesion
// se queda ALLI, y la app vuelve a empezar el mismo bucle en cada arranque.
const idHost = "id." + roomsHost.split(".").slice(1).join(".");

const config: CapacitorConfig = {
  appId: "eu.sytue.rooms",
  appName: "Sytue Rooms",
  // Cascaron local (splash/offline de respaldo); en runtime el webview va a server.url.
  webDir: "www",
  server: {
    url: ROOMS_URL,
    // Solo TLS: dev usa https://rooms.sytue.localhost (mkcert); prod https://rooms.sytue.eu.
    cleartext: false,
    // Lista CERRADA de dos hosts: el origen de Rooms y el de Sytue ID (el login vive
    // alli). Cualquier otro enlace abre en el navegador del sistema y no arrastra la
    // sesion de Rooms.
    allowNavigation: [roomsHost, idHost],
  },
  ios: {
    // La carcasa web reserva env(safe-area-inset-*); UIKit no vuelve a sumarlas.
    contentInset: "never",
    // Pellizco para ampliar. Capacitor lo DESACTIVA por defecto (zoomEnabled:false ->
    // scrollViewWillBeginZooming cancela el gesto), y en un webview eso deja al usuario
    // sin NINGUNA forma de agrandar el texto: WKWebView no atiende Dynamic Type para
    // contenido web. Incumple WCAG 1.4.4, que la casa exige (ADR-0004).
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
    SplashScreen: {
      // launchAutoHide:false => native.js la oculta cuando Rooms ha cargado (evita
      // flash blanco / ocultar antes de tiempo). Colores de marca (Graphite & Brass).
      launchAutoHide: false,
      backgroundColor: "#f0f3f7",
      androidSplashResourceName: "splash",
      showSpinner: false,
    },
    PushNotifications: {
      // Muestra badge/sonido/banner tambien en primer plano (iOS los suprime por defecto).
      presentationOptions: ["badge", "sound", "alert"],
    },
    Keyboard: {
      // Redimensiona el webview al abrir el teclado (chat: el composer sube con el teclado).
      resize: "native",
    },
  },
};

export default config;
