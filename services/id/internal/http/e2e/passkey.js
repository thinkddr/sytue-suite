/* SPDX-License-Identifier: Apache-2.0 */
/*
 * La passkey recién dada de alta desbloquea desde ya (ADR 0085): la página
 * /e2e/passkey, en el origen de Sytue ID, cierra para ESA passkey la clave de
 * desbloqueo que ya tiene este navegador y vuelve a donde se dio de alta.
 *
 * La salida PRF tiene que salir de una ceremonia real en ESTE origen: si la diera
 * otra página (la app de la cuenta), un guion inyectado en ella podría inventarse
 * una passkey y una PRF y quedarse con un sobre que sabe abrir. Por eso:
 *
 * - Si el alta fue aquí mismo («Mi cuenta» de Sytue ID) y el autenticador dio la PRF
 *   al crearla, su KEK ya está en el almacén (`enroll`, una CryptoKey no
 *   extraíble, cinco minutos) y no se pide nada más.
 * - Si no, se pide la passkey una vez (navigator.credentials.get con esa credencial
 *   y la PRF). Si el navegador exige un gesto, un botón; «Seguir sin desbloquear»
 *   vuelve sin más: la passkey desbloqueará en su primer uso, como antes.
 */
(function (global) {
  "use strict";
  var E = global.SytueE2E;
  if (!E) return;
  var ENROLL_TTL = 5 * 60 * 1000;

  /** Una KEK de la PRF pendiente para esta passkey, si la dejó el alta en este origen. */
  E.takeEnrollKEK = function (cred, now) {
    return E.load("enroll").then(function (p) {
      return E.store
        .del("enroll")
        .catch(function () {})
        .then(function () {
          if (
            !p ||
            p.cred !== cred ||
            !E.sealedKey(p.kek) ||
            now - p.at < 0 ||
            now - p.at > ENROLL_TTL
          )
            return null;
          return p.kek;
        });
    });
  };

  /** Pide la PRF de `cred` en una ceremonia y la convierte en KEK sin guardarla en bytes. */
  E.prfKEK = function (cfg, credentials) {
    return E.passkeyPRFInput().then(function (first) {
      return credentials
        .get({
          publicKey: {
            challenge: E.random(32),
            rpId: cfg.rp_id,
            timeout: 120000,
            userVerification: "preferred",
            allowCredentials: [{ type: "public-key", id: E.unb64(cfg.cred) }],
            extensions: { prf: { eval: { first: first } } },
          },
        })
        .then(function (c) {
          var ext =
            c && c.getClientExtensionResults
              ? c.getClientExtensionResults()
              : {};
          var out = ext && ext.prf && ext.prf.results && ext.prf.results.first;
          if (!out) return null;
          var prf = new Uint8Array(out.buffer || out).slice();
          E.wipe(new Uint8Array(out.buffer || out));
          if (prf.length !== 32) {
            E.wipe(prf);
            return null;
          }
          return E.passkeyKEK(prf);
        });
    });
  };

  /** Todo el paso: devuelve true si la passkey ya desbloquea. */
  E.passkeyEnroll = function (cfg, opts) {
    opts = opts || {};
    var now = (opts.now || Date.now)();
    return E.load("auk:" + cfg.sub).then(function (rec) {
      if (!rec) return false;
      return E.takeEnrollKEK(cfg.cred, now)
        .then(function (kek) {
          if (kek) return kek;
          if (!opts.credentials) return null;
          return E.prfKEK(cfg, opts.credentials);
        })
        .then(function (kek) {
          if (!kek) return false;
          return E.enrollPasskey({
            sub: cfg.sub,
            cred: cfg.cred,
            kek: kek,
            fetch: opts.fetch,
          });
        });
    });
  };

  var doc = global.document;
  var el = doc && doc.getElementById("e2e-passkey");
  if (!el) return;
  var cfg = JSON.parse(el.textContent);
  var gone = false;
  function go() {
    if (gone) return;
    gone = true;
    // Solo una vuelta navegable (comun.js, `safeReturn`); si no, «Mi cuenta».
    global.location.replace(E.safeReturn(cfg.next) || "/account");
  }
  var creds =
    global.navigator &&
    global.navigator.credentials &&
    global.PublicKeyCredential
      ? global.navigator.credentials
      : null;
  function attempt(withCeremony) {
    return E.passkeyEnroll(cfg, {
      credentials: withCeremony ? creds : null,
    });
  }
  // Primero sin ceremonia (la KEK del alta, si la hay); luego, con ella.
  attempt(false)
    .then(function (done) {
      if (done || !creds) return done;
      return attempt(true);
    })
    .then(go, function (err) {
      // Sin gesto de la persona algunos navegadores no abren la passkey: un botón.
      var ask = doc.getElementById("e2e-passkey-ask");
      var button = doc.getElementById("e2e-passkey-go");
      var working = doc.getElementById("e2e-working");
      if (
        !ask ||
        !button ||
        !creds ||
        (err && err.name !== "NotAllowedError")
      ) {
        go();
        return;
      }
      if (working) working.hidden = true;
      ask.hidden = false;
      button.addEventListener("click", function () {
        button.disabled = true;
        attempt(true).then(go, go);
      });
    });
})(typeof window !== "undefined" ? window : globalThis);
