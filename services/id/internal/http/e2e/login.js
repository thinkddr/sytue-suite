/* SPDX-License-Identifier: Apache-2.0 */
/*
 * Página de inicio de sesión (ADR 0084 y 0085): deriva, EN ESTE NAVEGADOR, la KEK
 * que abre el sobre de la clave de desbloqueo, y la deja en el almacén de este
 * origen, como CryptoKey no extraíble, para el relevo que viene después de entrar
 * (relay.js).
 *
 * - Contraseña: antes de enviar el formulario se pide la sal pública de la cuenta
 *   (/login/unlock/salt, que no confirma si la cuenta existe), se calcula Argon2id
 *   y HKDF, y SOLO ENTONCES se envía el formulario de siempre. La contraseña viaja
 *   como siempre; lo derivado, nunca.
 * - Passkey: la ceremonia de acceso pide también la extensión PRF (loginScript
 *   llama a withPRF y a rememberPasskey).
 * - Transición desde la ADR 0082: si la app pidió abrir envoltorios viejos
 *   (`sytue_legacy` en la autorización que sigue), se calculan aquí también.
 *
 * Si algo falla (sin WebAssembly, sin IndexedDB, un navegador antiguo), se entra
 * igual: la app dirá que falta desbloquear y ofrecerá volver a entrar.
 */
(function (global) {
  "use strict";
  var E = global.SytueE2E;
  var doc = global.document;
  if (!E || !doc) return;
  function noop() {}
  var DEFAULTS = { alg: "argon2id", m: 65536, t: 3, p: 1 };

  // Un intento fallido vuelve a pintar el formulario con el aviso: lo derivado para
  // él no es de ninguna sesión.
  if (doc.querySelector(".alert")) E.store.del("pending").catch(noop);

  function returnTo() {
    var el = doc.querySelector("input[name=return_to]");
    return el ? el.value : "";
  }
  /** Los envoltorios de la 0082 que la app pidió abrir: `p.<sal>` y `k.<credencial>.<entrada PRF>`. */
  function legacyRequest() {
    var q = null;
    try {
      q = new URL(returnTo(), global.location.origin).searchParams.get(
        "sytue_legacy",
      );
    } catch (e) {
      q = null;
    }
    if (!q) return [];
    return q
      .split("~")
      .slice(0, 4)
      .map(function (item) {
        var p = item.split(".");
        if (p[0] === "p" && p.length === 2 && E.unb64(p[1]).length === 16)
          return { kind: "password", salt: p[1] };
        if (p[0] === "k" && p.length === 3 && E.unb64(p[2]).length === 32)
          return { kind: "passkey", cred: p[1], salt: p[2] };
        return null;
      })
      .filter(Boolean);
  }
  E.legacyRequest = legacyRequest;

  /**
   * Lo que guarda el formulario de contraseña: la KEK (una CryptoKey no extraíble),
   * la sal con la que salió y, en la transición, lo que abre los envoltorios de la
   * 0082 cifrado con la clave del almacén. Ningún byte secreto queda en una variable
   * al acabar: Argon2id y HKDF van directos a CryptoKey o a texto cifrado.
   */
  E.derivePassword = function (email, password, fetchImpl) {
    var f = fetchImpl || global.fetch.bind(global);
    return f("/login/unlock/salt", {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email: email }),
    })
      .then(function (r) {
        if (!r.ok) throw new Error("salt");
        return r.json();
      })
      .then(function (body) {
        var kdf = body && body.kdf;
        if (!kdf || kdf.alg !== "argon2id" || E.unb64(kdf.salt).length !== 16)
          throw new Error("salt");
        var legacy = legacyRequest().filter(function (l) {
          return l.kind === "password";
        });
        return E.passwordKEK(password, kdf).then(function (kek) {
          var items = [];
          var chain = Promise.resolve();
          legacy.forEach(function (l) {
            chain = chain.then(function () {
              var old = {
                alg: DEFAULTS.alg,
                m: DEFAULTS.m,
                t: DEFAULTS.t,
                p: DEFAULTS.p,
                salt: l.salt,
              };
              return E.passwordIKM(password, old).then(function (ikm) {
                var item = { kind: "password", salt: E.b64(E.unb64(l.salt)) };
                return E.legacyBox(item, ikm).then(
                  function (boxed) {
                    E.wipe(ikm);
                    items.push(boxed);
                  },
                  function (err) {
                    E.wipe(ikm);
                    throw err;
                  },
                );
              });
            });
          });
          return chain.then(function () {
            return {
              v: 2,
              kind: "password",
              kek: kek,
              kdf: kdf,
              legacy: items,
              at: Date.now(),
            };
          });
        });
      });
  };

  /** Añade la PRF a la petición de la passkey (y la entrada vieja, en la transición). */
  E.withPRF = function (publicKey) {
    return E.passkeyPRFInput().then(function (first) {
      var legacy = legacyRequest().filter(function (l) {
        return l.kind === "passkey";
      });
      var prf = { eval: { first: first } };
      if (legacy.length) {
        // evalByCredential exige allowCredentials: se ofrecen las passkeys de los
        // envoltorios viejos, que son de esta misma persona.
        prf.evalByCredential = {};
        publicKey.allowCredentials = legacy.map(function (l) {
          prf.evalByCredential[l.cred] = {
            first: first,
            second: E.unb64(l.salt),
          };
          return { type: "public-key", id: E.unb64(l.cred) };
        });
      }
      publicKey.extensions = Object.assign({}, publicKey.extensions, {
        prf: prf,
      });
      return publicKey;
    });
  };

  /**
   * Guarda la KEK de la passkey que acaba de usarse, si su autenticador dio la PRF.
   * La salida PRF se convierte en una CryptoKey no extraíble y se pone a cero.
   */
  E.rememberPasskey = function (credential) {
    var ext =
      credential && credential.getClientExtensionResults
        ? credential.getClientExtensionResults()
        : {};
    var results = ext && ext.prf && ext.prf.results;
    if (!results || !results.first) return E.store.del("pending");
    var cred = E.b64u(new Uint8Array(credential.rawId));
    var first = new Uint8Array(results.first.buffer || results.first).slice();
    var second = results.second
      ? new Uint8Array(results.second.buffer || results.second).slice()
      : null;
    // El navegador devuelve sus propios ArrayBuffer: también se borran.
    E.wipe(new Uint8Array(results.first.buffer || results.first));
    if (results.second)
      E.wipe(new Uint8Array(results.second.buffer || results.second));
    var legacy = Promise.resolve([]);
    if (second) {
      var match = legacyRequest().filter(function (l) {
        return l.kind === "passkey" && l.cred === cred;
      });
      if (match.length) {
        legacy = E.legacyBox(
          { kind: "passkey", credential_id: cred },
          second,
        ).then(function (boxed) {
          return [boxed];
        });
      }
    }
    if (first.length !== 32) {
      E.wipe(first, second);
      return E.store.del("pending");
    }
    return Promise.all([E.passkeyKEK(first), legacy]).then(
      function (r) {
        E.wipe(first, second);
        return E.store.put("pending", {
          v: 2,
          kind: "passkey",
          kek: r[0],
          cred: cred,
          legacy: r[1],
          at: Date.now(),
        });
      },
      function (err) {
        E.wipe(first, second);
        throw err;
      },
    );
  };

  var form = doc.querySelector('form[action="/login/"]');
  if (!form) return;
  var ready = false;
  var busy = false;
  form.addEventListener("submit", function (ev) {
    // El captcha retiene el envío hasta resolverse y lo repite él: entonces se deriva.
    if (ready || ev.defaultPrevented) return;
    var pw = form.querySelector("input[name=password]");
    var em = form.querySelector("input[name=email]");
    if (!pw || !pw.value || !global.hashwasm || !global.indexedDB) return;
    ev.preventDefault();
    if (busy) return;
    busy = true;
    var button = form.querySelector("button[type=submit]");
    if (button) button.disabled = true;
    var sent = false;
    function send() {
      if (sent) return;
      sent = true;
      ready = true;
      global.HTMLFormElement.prototype.submit.call(form);
    }
    // Nunca más de quince segundos: un móvil lento entra sin desbloquear antes que no entrar.
    var timer = global.setTimeout(send, 15000);
    // Nada de lo que falle aquí impide entrar: se envía el formulario igual.
    var derived;
    try {
      derived = E.derivePassword(em ? em.value : "", pw.value).then(
        function (pending) {
          return E.store.put("pending", pending);
        },
      );
    } catch (e) {
      derived = Promise.reject(e);
    }
    derived
      .catch(function () {
        try {
          return E.store.del("pending").catch(noop);
        } catch (e) {
          return undefined;
        }
      })
      .then(function () {
        global.clearTimeout(timer);
        send();
      });
  });
})(typeof window !== "undefined" ? window : globalThis);
