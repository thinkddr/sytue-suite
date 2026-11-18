/* SPDX-License-Identifier: Apache-2.0 */
/*
 * Relevo del desbloqueo (ADR 0084 y 0085): la página que Sytue ID pinta al acabar la
 * autorización, en lugar de la redirección, cuando la app lo pide
 * (`sytue_unlock=1` en la web, `sytue_unlock_key=<clave pública>` en las nativas).
 *
 * Abre aquí, en el navegador, el sobre de la clave de desbloqueo con la KEK que
 * acaba de derivar la página de inicio de sesión; si la cuenta no tiene ninguna,
 * la crea; si el sobre de ese factor falta o es viejo (contraseña cambiada, passkey
 * nueva), lo rehace con la clave que ya tiene este navegador. Deja la clave en el
 * almacén de este origen, sin nada legible (comun.js), para el puente de las apps
 * web y, para una app nativa, la añade cifrada a su clave efímera a la URL de
 * vuelta. Al servidor solo sube el sobre, que no puede abrir.
 *
 * Pase lo que pase, la persona vuelve a la app: sin desbloquear, si hace falta.
 */
(function (global) {
  "use strict";
  var E = global.SytueE2E;
  if (!E) return;
  function noop() {}

  function pendingFor(cfg, pending, now) {
    if (!pending || !E.sealedKey(pending.kek) || typeof pending.at !== "number")
      return null;
    var age = now - pending.at;
    // Derivada hace menos de diez minutos y ANTES de abrirse esta sesión (con
    // margen): una que llega después es de un intento que no abrió ninguna.
    if (age < 0 || age > E.PENDING_TTL || age + 30000 < (cfg.auth_age || 0))
      return null;
    if (pending.kind === "password")
      return pending.kdf && pending.kdf.salt === cfg.salt ? pending : null;
    if (pending.kind === "passkey")
      return (cfg.creds || []).indexOf(pending.cred) >= 0 ? pending : null;
    return null;
  }

  /**
   * Decide y guarda. Devuelve `{ unlocked, blob }` (la clave queda guardada en el
   * almacén, nunca en el resultado) o `{ lost: true, startOver }` cuando el sobre de
   * contraseña no abre con la contraseña con la que se acaba de entrar y este
   * navegador no tiene la clave.
   *
   * Los bytes de la clave de desbloqueo existen aquí solo lo que tarda cada paso
   * (abrir el sobre, cerrar uno nuevo, entregarla a una app nativa) y se ponen a
   * cero al acabar; lo que queda es la CryptoKey del puente y la copia cifrada.
   */
  E.relay = function (cfg, opts) {
    opts = opts || {};
    var f = opts.fetch || global.fetch.bind(global);
    var now = (opts.now || Date.now)();
    var envelopes = cfg.envelopes || [];
    function put(env, reset) {
      return f("/e2e/envelopes", {
        method: "PUT",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ envelope: env, reset: !!reset, sub: cfg.sub }),
      }).then(function (r) {
        if (!r.ok) throw new Error("envelope");
      });
    }
    function sealFor(p, key, prfInput) {
      return E.sealEnvelope({
        kek: p.kek,
        kind: p.kind,
        user: cfg.sub,
        cred: p.kind === "passkey" ? p.cred : "",
        kdf:
          p.kind === "password"
            ? p.kdf
            : { alg: "webauthn-prf-hkdf-sha256", prf_salt: E.b64(prfInput) },
        key: key,
      });
    }
    /**
     * Deja la clave en el almacén (si no estaba ya) y, para una app nativa, la
     * cifra para su clave efímera. `key` son bytes que esta función pone a cero.
     */
    var serverPw = null;
    envelopes.forEach(function (e) {
      if (e.kind === "password") serverPw = e;
    });
    function deliver(key, legacy, stored, pwEnv) {
      var keep = [E.store.del("pending").catch(noop)];
      var record = Promise.resolve(stored);
      if (key && !stored) record = E.saveUnlock(cfg.sub, key);
      // La copia del sobre de contraseña vigente: el que acaba de cerrarse, o el
      // del servidor (ADR 0085, cambio de contraseña).
      keep.push(
        record.then(function () {
          return E.keepPassword(cfg.sub, pwEnv || serverPw).catch(noop);
        }),
      );
      var native = !!(key && cfg.app_key);
      var ikms = Promise.all(
        (legacy || []).map(function (l) {
          return E.revealLegacy(l).then(
            function (ikm) {
              return { item: l, ikm: ikm };
            },
            function () {
              return null;
            },
          );
        }),
      ).then(function (list) {
        return list.filter(Boolean);
      });
      keep.push(
        ikms.then(function (list) {
          if (!list.length) return undefined;
          return Promise.all(
            list.map(function (x) {
              return E.legacyKey(x.item, x.ikm);
            }),
          ).then(function (items) {
            return E.store.put("legacy:" + cfg.sub, {
              v: 2,
              items: items,
              at: now,
            });
          });
        }),
      );
      return Promise.all(keep)
        .then(function () {
          return ikms;
        })
        .then(function (list) {
          if (!native) return { unlocked: !!key, blob: "" };
          return E.sealDelivery(cfg.app_key, {
            v: 1,
            sub: cfg.sub,
            key: E.b64(key),
            legacy: list.map(function (x) {
              var out = { kind: x.item.kind };
              if (x.item.salt) out.salt = x.item.salt;
              if (x.item.credential_id)
                out.credential_id = x.item.credential_id;
              out.ikm = E.b64(x.ikm);
              return out;
            }),
          }).then(function (blob) {
            return { unlocked: true, blob: blob };
          });
        })
        .then(
          function (r) {
            E.wipe(key);
            return ikms.then(function (list) {
              list.forEach(function (x) {
                E.wipe(x.ikm);
              });
              return r;
            });
          },
          function (err) {
            E.wipe(key);
            throw err;
          },
        );
    }
    /** La clave guardada en bytes (para rehacer un sobre o una entrega nativa). */
    function storedBytes(stored) {
      return E.revealUnlock(cfg.sub, stored);
    }

    return Promise.all([
      E.load("pending").catch(noop),
      E.load("auk:" + cfg.sub).catch(noop),
      E.passkeyPRFInput(),
    ]).then(function (r) {
      var p = pendingFor(cfg, r[0], now);
      var stored = r[1];
      var prfInput = r[2];
      if (stored && cfg.hint && stored.hint !== cfg.hint) stored = null;
      var legacy = p && p.legacy ? p.legacy : [];
      if (!p) {
        if (!stored) return deliver(null, [], null);
        // Ya desbloqueado en este navegador: a la web no hay nada que darle; a una
        // app nativa, su copia cifrada para ella.
        if (!cfg.app_key) return deliver(null, [], stored).then(unlocked);
        return storedBytes(stored).then(function (key) {
          return deliver(key, [], stored);
        });
      }
      var env = null;
      envelopes.forEach(function (e) {
        if (
          e.kind === p.kind &&
          (p.kind === "password" || e.credential_id === p.cred)
        )
          env = e;
      });
      return (
        env
          ? E.openEnvelope(p.kek, cfg.sub, env).catch(noop)
          : Promise.resolve(null)
      ).then(function (opened) {
        if (opened) {
          if (stored && !cfg.app_key) {
            E.wipe(opened);
            return deliver(null, legacy, stored).then(unlocked);
          }
          return deliver(opened, legacy, stored);
        }
        if (stored || !cfg.hint) {
          // Rehacer el sobre de este factor con la clave que ya tiene este navegador
          // (contraseña cambiada, passkey nueva), o crear la clave de la cuenta.
          var first = !stored;
          return (
            stored ? storedBytes(stored) : Promise.resolve(E.random(32))
          ).then(function (key) {
            var sealed = null;
            return sealFor(p, key, prfInput)
              .then(function (e) {
                sealed = e;
                return put(e, false);
              })
              .then(
                function () {
                  return deliver(key, legacy, stored, sealed);
                },
                function () {
                  // Una clave nueva que el servidor rechaza (otro dispositivo creó
                  // la suya a la vez) no se guarda: la siguiente vez se abre la suya.
                  if (first) {
                    E.wipe(key);
                    return deliver(null, legacy, null);
                  }
                  return deliver(key, legacy, stored);
                },
              );
          });
        }
        if (p.kind === "password" && env) {
          return {
            lost: true,
            startOver: function () {
              var key = E.random(32);
              var sealed = null;
              return sealFor(p, key, prfInput)
                .then(function (e) {
                  sealed = e;
                  return put(e, true);
                })
                .then(
                  function () {
                    return deliver(key, legacy, null, sealed);
                  },
                  function (err) {
                    E.wipe(key);
                    throw err;
                  },
                );
            },
          };
        }
        // Una passkey que aún no tiene sobre y un navegador sin la clave: se dice
        // que entre una vez con la contraseña aquí; desde entonces bastará la passkey.
        return deliver(null, legacy, null).then(function (r) {
          if (p.kind === "passkey" && cfg.hint) r.enroll = true;
          return r;
        });
      });
    });
  };
  function unlocked(r) {
    r.unlocked = true;
    return r;
  }

  /**
   * A dónde vuelve el relevo: `auto` para salir solo y `manual` para el botón «Volver a
   * la aplicación». La entrega nativa va en la URL (`sytue_unlock`).
   *
   * En Android, la vuelta a un esquema propio (`sytue-mail://auth/callback`) sale sola
   * como `intent://…#Intent;scheme=…;end`: Chrome no siempre deja a una página abrir un
   * esquema propio sin un gesto reciente, y el relevo ya ha esperado al cifrado; el
   * intent es la forma que Chrome documenta para abrir una app. El botón lleva la URL
   * del esquema tal cual, que con el gesto de la persona abre cualquier navegador. Sin
   * `package`: si otra app registrara el mismo esquema, el código no le sirve sin el
   * verificador PKCE y la entrega solo la abre la clave efímera de la app que la pidió.
   */
  E.returnTo = function (next, blob, ua, web) {
    var url = next;
    // A una app WEB, en el fragmento: el navegador lo conserva al seguir las
    // redirecciones del callback y no lo manda a ningún servidor ni a sus registros.
    // Solo lo abre el puente de esa app (bridge.js, `handoff`).
    if (blob && web)
      return {
        auto: url.split("#")[0] + "#sytue_unlock=" + blob,
        manual: url.split("#")[0] + "#sytue_unlock=" + blob,
      };
    if (blob)
      url += (url.indexOf("?") < 0 ? "?" : "&") + "sytue_unlock=" + blob;
    var m = /^([a-z][a-z0-9+.-]*):\/\/([^#]*)$/i.exec(url);
    if (!m || /^https?$/i.test(m[1]) || !/\bAndroid\b/.test(ua || ""))
      return { auto: url, manual: url };
    return {
      auto: "intent://" + m[2] + "#Intent;scheme=" + m[1] + ";end",
      manual: url,
    };
  };

  var doc = global.document;
  var el = doc && doc.getElementById("e2e-relay");
  if (!el) return;
  var cfg = JSON.parse(el.textContent);
  var gone = false;
  var link = doc.getElementById("e2e-next");
  function go(blob) {
    if (gone) return;
    gone = true;
    var to = E.returnTo(
      cfg.next,
      blob,
      global.navigator ? global.navigator.userAgent : "",
      cfg.web === true,
    );
    if (link) link.href = to.manual;
    // Por si el navegador no deja salir solo hacia el esquema de una app móvil.
    global.setTimeout(function () {
      var manual = doc.getElementById("e2e-manual");
      if (manual) manual.hidden = false;
    }, 1500);
    global.location.replace(to.auto);
  }
  var timer = global.setTimeout(function () {
    go("");
  }, 20000);
  E.relay(cfg).then(
    function (r) {
      if (r.enroll) {
        global.clearTimeout(timer);
        var working0 = doc.getElementById("e2e-working");
        var first = doc.getElementById("e2e-passkey-first");
        if (working0) working0.hidden = true;
        if (first) {
          first.hidden = false;
          return;
        }
      }
      if (!r.lost) {
        global.clearTimeout(timer);
        go(r.blob);
        return;
      }
      global.clearTimeout(timer);
      var working = doc.getElementById("e2e-working");
      var lost = doc.getElementById("e2e-lost");
      var check = doc.getElementById("e2e-understood");
      var restart = doc.getElementById("e2e-restart");
      if (working) working.hidden = true;
      if (lost) lost.hidden = false;
      if (check && restart) {
        check.addEventListener("change", function () {
          restart.disabled = !check.checked;
        });
        restart.addEventListener("click", function () {
          if (!check.checked) return;
          restart.disabled = true;
          r.startOver().then(
            function (d) {
              go(d.blob);
            },
            function () {
              go("");
            },
          );
        });
      }
    },
    function () {
      go("");
    },
  );
})(typeof window !== "undefined" ? window : globalThis);
