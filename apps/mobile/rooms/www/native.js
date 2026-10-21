/*
 * Sytue Rooms — puente webview <-> nativo (Capacitor 8).
 *
 * Corre DENTRO del webview, sobre el PWA de Rooms (rooms.sytue.eu). Habla con el
 * puente global `window.Capacitor` (lo inyecta Capacitor incluso en un origen
 * remoto) para cablear la integracion nativa: status bar, splash, push, deep-links,
 * back de Android, teclado y badge de no-leidos. NO expone FS/exec/APIs peligrosas:
 * solo lee no-leidos del titulo y navega DENTRO del origen de Rooms.
 *
 * Como Rooms es un origen REMOTO, este archivo se INYECTA en la pagina desde la capa
 * nativa (WKUserScript en iOS / evaluateJavascript en onPageFinished en Android; ver
 * README §Puente webview<->nativo). Idempotente (guard __sytueRoomsNative). El PWA
 * tambien puede cargarlo el mismo si detecta `window.Capacitor?.isNativePlatform()`.
 *
 * `origin` = location.origin: el script SIEMPRE corre en la pagina de Rooms, asi que
 * su propio origen ES el origen de Rooms (no hay que configurarlo).
 */
(function () {
  "use strict";

  // --- Logica pura (testeable sin webview; ver scripts/selfcheck.cjs) -----------

  // parseUnread: no-leidos desde el titulo. Rooms/Mattermost prefijan "(N)" o
  // "(N+)" cuando hay menciones/DMs sin leer. Sin prefijo => 0.
  function parseUnread(title) {
    var m = /^\s*\((\d+)\+?\)/.exec(String(title || ""));
    return m ? parseInt(m[1], 10) : 0;
  }

  // deepLinkPath: traduce un deep-link a una RUTA dentro del origen de Rooms.
  //   sytue-rooms://open?url=<https>    -> ruta del URL si pertenece a Rooms
  //   rooms://channels/town-square      -> /channels/town-square (compatibilidad)
  //   https://<origen-rooms>/channels/x -> /channels/x   (universal link, mismo origen)
  //   https://otro-host/...             -> ''            (externo: se ignora aqui)
  // Siempre aterriza en el origen de Rooms; nunca salta a otro host.
  function deepLinkPath(raw, origin) {
    try {
      var u = new URL(raw);
      if (u.protocol === "sytue-rooms:" && u.host === "open") {
        var destination = u.searchParams.get("url");
        if (!destination) return "";
        var target = new URL(destination);
        if (target.origin !== origin) return "";
        return target.pathname + (target.search || "") + (target.hash || "");
      }
      if (u.protocol === "rooms:") {
        var p = (u.host + u.pathname).replace(/^\/+/, "");
        return "/" + p + (u.search || "") + (u.hash || "");
      }
      if (
        (u.protocol === "http:" || u.protocol === "https:") &&
        u.origin === origin
      ) {
        return u.pathname + (u.search || "") + (u.hash || "");
      }
    } catch (e) {
      /* deep-link malformado: se ignora */
    }
    return "";
  }

  // Bajo Node (self-check) exporta la logica pura y no toca el DOM/webview.
  if (typeof module !== "undefined" && module.exports) {
    module.exports = { parseUnread: parseUnread, deepLinkPath: deepLinkPath };
    return;
  }

  // --- Wiring nativo (solo en el webview de Capacitor) --------------------------

  if (window.__sytueRoomsNative) return; // idempotente (reinyeccion por navegacion)
  var cap = window.Capacitor;
  if (
    !cap ||
    typeof cap.isNativePlatform !== "function" ||
    !cap.isNativePlatform()
  )
    return;
  window.__sytueRoomsNative = 1;

  var P = cap.Plugins || {};
  var origin = location.origin;
  var platform =
    typeof cap.getPlatform === "function" ? cap.getPlatform() : "web";

  // Navega a un deep-link SI resuelve a una ruta del origen de Rooms.
  function navigateDeep(raw) {
    var path = deepLinkPath(raw, origin);
    if (path) location.assign(origin + path);
  }

  // Badge de la app (no-leidos). @capawesome/capacitor-badge (MIT) => plugin "Badge".
  // Guardado: si el plugin no esta, es no-op (iOS ademas recibe el badge del payload APNs).
  function setBadge(n) {
    try {
      if (!P.Badge) return;
      if (n > 0) P.Badge.set({ count: n });
      else P.Badge.clear();
    } catch (e) {
      /* badge best-effort */
    }
  }

  // POST del token nativo (FCM/APNs) al backend. Cookie de sesion (credentials:include),
  // igual que el BFF de Web Push. Endpoint documentado en README (sender FCM/APNs = PENDIENTE).
  function registerToken(token) {
    try {
      fetch(origin + "/api/push/native", {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ platform: platform, token: token }),
        keepalive: true,
      }).catch(function () {});
    } catch (e) {
      /* red no disponible: se reintenta en el proximo arranque */
    }
  }

  // La barra del sistema sigue el tema del documento, también al cambiarlo.
  if (P.StatusBar) {
    var appearance = window.matchMedia("(prefers-color-scheme: dark)");
    function statusBarTheme() {
      var chosen = document.documentElement.getAttribute("data-theme");
      var dark = chosen === "dark" || (!chosen && appearance.matches);
      var chrome = getComputedStyle(document.documentElement)
        .getPropertyValue("--sytue-chrome").trim();
      P.StatusBar.setStyle({ style: dark ? "DARK" : "LIGHT" }).catch(function () {});
      // La API requiere #RRGGBB; en Android 16 el fondo viene del propio webview.
      if (/^#[a-f0-9]{6}$/i.test(chrome)) {
        P.StatusBar.setBackgroundColor({ color: chrome }).catch(function () {});
      }
    }
    try {
      P.StatusBar.setOverlaysWebView({ overlay: false }).catch(function () {});
      statusBarTheme();
      new MutationObserver(statusBarTheme).observe(document.documentElement, {
        attributes: true, attributeFilter: ["data-theme"],
      });
      appearance.addEventListener("change", statusBarTheme);
    } catch (e) {
      /* status bar best-effort */
    }
  }

  // Splash: se oculta cuando Rooms ha cargado (config launchAutoHide:false).
  function hideSplash() {
    try {
      if (P.SplashScreen) P.SplashScreen.hide();
    } catch (e) {}
  }
  if (document.readyState === "complete") hideSplash();
  else window.addEventListener("load", hideSplash, { once: true });

  // Push: pide permiso, registra el token nativo y cablea la recepcion.
  if (P.PushNotifications) {
    try {
      P.PushNotifications.addListener("registration", function (t) {
        if (t && t.value) registerToken(t.value);
      });
      P.PushNotifications.addListener("registrationError", function () {});
      // Tap en la notificacion (app en background/cerrada) -> deep-link al canal.
      // El sender del backend debe incluir data.url (ruta del canal/mensaje).
      P.PushNotifications.addListener(
        "pushNotificationActionPerformed",
        function (a) {
          var url =
            a &&
            a.notification &&
            a.notification.data &&
            a.notification.data.url;
          if (url) navigateDeep(url);
        },
      );
      P.PushNotifications.checkPermissions()
        .then(function (perm) {
          if (perm && perm.receive === "granted")
            return P.PushNotifications.register();
          return P.PushNotifications.requestPermissions().then(function (req) {
            if (req && req.receive === "granted")
              return P.PushNotifications.register();
          });
        })
        .catch(function () {});
    } catch (e) {
      /* push best-effort */
    }
  }

  // App: deep-links, back de Android y foco.
  if (P.App) {
    try {
      // Deep-links sytue-rooms:// (y rooms:// histórico) + universal links.
      P.App.addListener("appUrlOpen", function (d) {
        if (d && d.url) navigateDeep(d.url);
      });
      // Back de Android: navega el historial del webview; si no puede, sale.
      P.App.addListener("backButton", function () {
        if (window.history.length > 1) window.history.back();
        else if (P.App.exitApp) P.App.exitApp();
      });
      // Al volver a primer plano: limpia el badge (el usuario esta mirando).
      P.App.addListener("appStateChange", function (s) {
        if (s && s.isActive) setBadge(0);
      });
    } catch (e) {
      /* app listeners best-effort */
    }
  }

  // Puente para el PWA: si Rooms quiere empujar no-leidos/foco explicitos, puede.
  //   if (window.SytueNative?.isNative) window.SytueNative.setUnread(count)
  window.SytueNative = {
    isNative: true,
    platform: platform,
    setUnread: setBadge,
    openChannel: navigateDeep,
  };

  // Fallback SIN cambios en el PWA: deriva no-leidos del titulo (Rooms prefija "(N)"),
  // igual que el envoltorio de escritorio. Barato; para el intervalo al ocultarse.
  var lastUnread = -1;
  function syncUnread() {
    var n = parseUnread(document.title);
    if (n !== lastUnread) {
      lastUnread = n;
      setBadge(n);
    }
  }
  setInterval(syncUnread, 2000);
  syncUnread();
})();
