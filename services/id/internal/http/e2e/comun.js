/* SPDX-License-Identifier: Apache-2.0 */
/*
 * Desbloqueo del cifrado con el inicio de sesión de Sytue ID (ADR 0084 y 0085): lo
 * común a las páginas que lo usan (inicio de sesión, relevo, puente y cierre).
 *
 * TODO esto corre en el navegador de la persona. Ni la contraseña derivada, ni la
 * salida PRF de la passkey, ni la clave de desbloqueo viajan nunca a un servidor, y
 * desde la ADR 0085 tampoco se guardan legibles: el almacén de este origen
 * (IndexedDB de id.<dominio>) solo tiene CryptoKey de WebCrypto NO EXTRAÍBLES y
 * texto cifrado con una de ellas. La clave de desbloqueo no sale de este origen
 * hacia una app web: el puente hace con ella las operaciones que la app pide y
 * devuelve solo su resultado. A una app nativa le llega cifrada para su clave
 * efímera (relevo). Al servidor solo suben sobres que no puede abrir.
 *
 * El formato lo fijan apps/nucleo/recoverywrap/testdata/desbloqueo.json y la
 * prueba que lo ejecuta (packages/offline/src/matrix/sytue-id-pages.test.ts).
 * Va en línea, con su hash en la CSP, como el resto de guiones del IdP.
 */
(function (global) {
  "use strict";
  var LABEL = "sytue-e2e-unlock-v1";
  var WRAP_LABEL = "sytue-matrix-recovery-wrap-v1";
  var VAULT_WRAP_LABEL = "sytue-pq-vault-wrap-v1";
  var enc = new TextEncoder();
  var subtle = global.crypto && global.crypto.subtle;
  var AES = { name: "AES-GCM", length: 256 };

  function b64(bytes) {
    var s = "";
    for (var i = 0; i < bytes.length; i++) s += String.fromCharCode(bytes[i]);
    return btoa(s);
  }
  function b64u(bytes) {
    return b64(bytes)
      .replace(/\+/g, "-")
      .replace(/\//g, "_")
      .replace(/=+$/, "");
  }
  function unb64(s) {
    s = String(s).replace(/-/g, "+").replace(/_/g, "/");
    while (s.length % 4) s += "=";
    var t = atob(s);
    var out = new Uint8Array(t.length);
    for (var i = 0; i < t.length; i++) out[i] = t.charCodeAt(i);
    return out;
  }
  function concat(a, b) {
    var out = new Uint8Array(a.length + b.length);
    out.set(a, 0);
    out.set(b, a.length);
    return out;
  }
  function same(a, b) {
    if (!a || !b || a.length !== b.length) return false;
    var d = 0;
    for (var i = 0; i < a.length; i++) d |= a[i] ^ b[i];
    return d === 0;
  }
  function random(n) {
    return global.crypto.getRandomValues(new Uint8Array(n));
  }
  /** Pone a cero los bytes que ya no hacen falta: lo único que JS deja borrar. */
  function wipe() {
    for (var i = 0; i < arguments.length; i++) {
      var a = arguments[i];
      if (a && typeof a.fill === "function") a.fill(0);
    }
  }
  /** Una CryptoKey que nadie puede leer: la única forma en que algo secreto se guarda. */
  function sealedKey(k) {
    return (
      !!k &&
      typeof k === "object" &&
      k.type === "secret" &&
      k.extractable === false &&
      typeof k.algorithm === "object"
    );
  }

  /** HKDF-SHA256 a bytes: solo para lo que es público (la huella) o efímero (la entrega). */
  function hkdf(ikm, info, length, salt) {
    return subtle
      .importKey("raw", ikm, "HKDF", false, ["deriveBits"])
      .then(function (base) {
        return subtle
          .deriveBits(
            {
              name: "HKDF",
              hash: "SHA-256",
              salt: salt || new Uint8Array(0),
              info: enc.encode(info),
            },
            base,
            length * 8,
          )
          .then(function (bits) {
            return new Uint8Array(bits);
          });
      });
  }
  /**
   * HKDF-SHA256 directamente a una clave AES-256-GCM NO extraíble: los bytes de la
   * clave derivada no existen nunca en JavaScript.
   */
  function hkdfAES(ikm, info, usages, salt) {
    return subtle
      .importKey("raw", ikm, "HKDF", false, ["deriveKey"])
      .then(function (base) {
        return subtle.deriveKey(
          {
            name: "HKDF",
            hash: "SHA-256",
            salt: salt || new Uint8Array(0),
            info: enc.encode(info),
          },
          base,
          AES,
          false,
          usages,
        );
      });
  }
  function aesKey(raw, usages) {
    return subtle.importKey("raw", raw, "AES-GCM", false, usages);
  }
  function encrypt(key, nonce, plain, aad) {
    return subtle
      .encrypt({ name: "AES-GCM", iv: nonce, additionalData: aad }, key, plain)
      .then(function (ct) {
        return new Uint8Array(ct);
      });
  }
  function decrypt(key, nonce, ct, aad) {
    return subtle
      .decrypt({ name: "AES-GCM", iv: nonce, additionalData: aad }, key, ct)
      .then(function (plain) {
        return new Uint8Array(plain);
      });
  }

  /** La huella pública de una clave de desbloqueo (recoverywrap.Hint). */
  function hint(key) {
    return hkdf(key, LABEL + "/hint", 16);
  }
  function envelopeAAD(kind, user, cred) {
    return enc.encode([LABEL, kind, user, cred || ""].join("\0"));
  }
  /**
   * La salida de Argon2id con los parámetros públicos del sobre (o del envoltorio de
   * la 0082). Quien la recibe la pone a cero en cuanto la usa. La contraseña es una
   * cadena de JavaScript y no se puede borrar: solo se codifica aquí, y esa copia sí
   * se borra. hash-wasm crea su memoria de WebAssembly en cada llamada y no la
   * guarda: al volver, nadie tiene una referencia a ella (ADR 0085).
   */
  function passwordIKM(password, kdf) {
    if (!global.hashwasm || kdf.alg !== "argon2id")
      return Promise.reject(new Error("argon2"));
    var pw = enc.encode(password);
    return global.hashwasm
      .argon2id({
        password: pw,
        salt: unb64(kdf.salt),
        iterations: kdf.t,
        memorySize: kdf.m,
        parallelism: kdf.p,
        hashLength: 32,
        outputType: "binary",
      })
      .then(
        function (out) {
          wipe(pw);
          var ikm = new Uint8Array(out);
          if (out !== ikm) wipe(out);
          return ikm;
        },
        function (err) {
          wipe(pw);
          throw err;
        },
      );
  }
  var KEK_USES = ["encrypt", "decrypt"];
  /** La KEK de un sobre: una CryptoKey AES-GCM no extraíble, derivada sin pasar por bytes. */
  function kekFrom(ikm, kind) {
    return hkdfAES(ikm, LABEL + "/" + kind, KEK_USES).then(
      function (kek) {
        wipe(ikm);
        return kek;
      },
      function (err) {
        wipe(ikm);
        throw err;
      },
    );
  }
  function passwordKEK(password, kdf) {
    return passwordIKM(password, kdf).then(function (ikm) {
      return kekFrom(ikm, "password");
    });
  }
  /** La KEK de la passkey. Pone a cero `prf`: es la salida del autenticador. */
  function passkeyKEK(prf) {
    return kekFrom(prf, "passkey");
  }
  /** La clave de un envoltorio de la clave de recuperación (recoverywrap.KeyFromIKM). */
  function wrapKey(ikm, kind, usages) {
    return hkdfAES(ikm, WRAP_LABEL + "/" + kind, usages);
  }
  /** La clave de un envoltorio de bóveda post-cuántica (ADR 0086, recoverywrap.PQVaultKeyFromAUK). */
  function vaultWrapKey(ikm, usages) {
    return hkdfAES(ikm, VAULT_WRAP_LABEL + "/sytue-id", usages);
  }
  var prfInput = null;
  /** La entrada `first` de la PRF con la que se evalúa cualquier passkey. */
  function passkeyPRFInput() {
    if (!prfInput) {
      prfInput = subtle
        .digest("SHA-256", enc.encode(LABEL + "/passkey-prf"))
        .then(function (d) {
          return new Uint8Array(d);
        });
    }
    return prfInput.then(function (p) {
      return p.slice();
    });
  }

  /** Cierra la clave de desbloqueo con una KEK (CryptoKey): el sobre que guarda Sytue ID. */
  function sealEnvelope(o) {
    var nonce = o.nonce || random(12);
    return Promise.all([
      hint(o.key),
      encrypt(o.kek, nonce, o.key, envelopeAAD(o.kind, o.user, o.cred)),
    ]).then(function (r) {
      return {
        v: 1,
        kind: o.kind,
        credential_id: o.cred || "",
        kdf: o.kdf,
        hint: b64(r[0]),
        nonce: b64(nonce),
        ciphertext: b64(r[1]),
      };
    });
  }
  /**
   * Abre un sobre; rechaza si la KEK no es la suya o lo que sale no es su clave.
   * Devuelve los bytes de la clave: quien los recibe los pone a cero al acabar.
   */
  function openEnvelope(kek, user, env) {
    return decrypt(
      kek,
      unb64(env.nonce),
      unb64(env.ciphertext),
      envelopeAAD(env.kind, user, env.credential_id),
    ).then(function (key) {
      return hint(key).then(function (h) {
        if (key.length !== 32 || !same(h, unb64(env.hint))) {
          wipe(key);
          throw new Error("hint");
        }
        return key;
      });
    });
  }

  /**
   * La entrega a una app nativa: ECDH P-256 con su clave efímera, HKDF-SHA256 y
   * AES-256-GCM (recoverywrap.SealDelivery). `eph` y `nonce` solo los fijan las
   * pruebas, para repetir los vectores. El JSON con la clave es una cadena que JS
   * no deja borrar: vive lo que tarda en cifrarse y nadie guarda una referencia.
   */
  function sealDelivery(appPublic, payload, eph, nonce) {
    var curve = { name: "ECDH", namedCurve: "P-256" };
    var appRaw = unb64(appPublic);
    if (appRaw.length !== 65) return Promise.reject(new Error("delivery key"));
    var pair = eph
      ? subtle
          .importKey(
            "jwk",
            { kty: "EC", crv: "P-256", d: eph.d, x: eph.x, y: eph.y },
            curve,
            false,
            ["deriveBits"],
          )
          .then(function (priv) {
            return subtle
              .importKey(
                "jwk",
                { kty: "EC", crv: "P-256", x: eph.x, y: eph.y },
                curve,
                true,
                [],
              )
              .then(function (pub) {
                return { privateKey: priv, publicKey: pub };
              });
          })
      : // Solo la parte pública se exporta; la privada efímera muere aquí.
        subtle.generateKey(curve, false, ["deriveBits"]);
    return Promise.all([
      pair,
      subtle.importKey("raw", appRaw, curve, false, []),
    ]).then(function (r) {
      var keys = r[0];
      return Promise.all([
        subtle.deriveBits({ name: "ECDH", public: r[1] }, keys.privateKey, 256),
        subtle.exportKey("raw", keys.publicKey),
      ]).then(function (d) {
        var shared = new Uint8Array(d[0]);
        var ephRaw = new Uint8Array(d[1]);
        var iv = nonce || random(12);
        return hkdfAES(
          shared,
          LABEL + "/native",
          ["encrypt"],
          concat(ephRaw, appRaw),
        ).then(function (key) {
          wipe(shared);
          var plain = enc.encode(JSON.stringify(payload));
          return encrypt(key, iv, plain, enc.encode(LABEL + "/native")).then(
            function (ct) {
              wipe(plain);
              return b64u(concat(concat(ephRaw, iv), ct));
            },
          );
        });
      });
    });
  }

  /**
   * Abre lo que cerró sealDelivery con la clave privada (CryptoKey no extraíble) de
   * `appPublic`. Lo usa el puente para la vuelta de una app web (bridge.js,
   * `handoff`); devuelve el JSON de la entrega, con la clave en base64, y quien lo
   * recibe pone a cero sus bytes.
   */
  function openDelivery(priv, appPublic, blob) {
    var curve = { name: "ECDH", namedCurve: "P-256" };
    var appRaw = unb64(appPublic);
    var raw = unb64(blob);
    if (appRaw.length !== 65 || raw.length < 65 + 12 + 16)
      return Promise.reject(new Error("delivery"));
    var ephRaw = raw.slice(0, 65);
    return subtle
      .importKey("raw", ephRaw, curve, false, [])
      .then(function (eph) {
        return subtle
          .deriveBits({ name: "ECDH", public: eph }, priv, 256)
          .then(function (bits) {
            var shared = new Uint8Array(bits);
            return hkdfAES(
              shared,
              LABEL + "/native",
              ["decrypt"],
              concat(ephRaw, appRaw),
            ).then(function (key) {
              wipe(shared);
              return decrypt(
                key,
                raw.slice(65, 77),
                raw.slice(77),
                enc.encode(LABEL + "/native"),
              );
            });
          })
          .then(function (plain) {
            try {
              return JSON.parse(new TextDecoder().decode(plain));
            } finally {
              wipe(plain);
            }
          });
      });
  }

  /*
   * El almacén de este origen. Desde la ADR 0085 todo lo secreto es una CryptoKey
   * no extraíble o va cifrado con una:
   *
   * - `vault`: la clave del almacén, al azar, AES-GCM, nacida aquí y no extraíble.
   *   Cifra las dos cosas que hay que poder recuperar en bytes: la clave de
   *   desbloqueo (para rehacer un sobre o entregarla a una app nativa) y lo que abre
   *   los envoltorios de la 0082 (para entregarlo a una app nativa).
   * - `pending`: la KEK que acaba de derivar la página de inicio de sesión (CryptoKey),
   *   hasta que el relevo la usa; caduca a los diez minutos.
   * - `auk:<sub>`: la clave de desbloqueo de esa persona en este navegador: su huella
   *   (pública), la clave de sus envoltorios `sytue-id` (CryptoKey, la que usa el
   *   puente) y la propia clave cifrada con `vault` (solo la abre el relevo).
   * - `legacy:<sub>`: las claves (CryptoKey) que abren sus envoltorios de la 0082,
   *   solo hasta pasarlos al tipo nuevo.
   *
   * Lo que escribieron las versiones anteriores en claro se convierte en el primer
   * uso (migrate) y la copia en claro se sobrescribe. Cerrar sesión lo borra todo.
   */
  var DB = "sytue-e2e";
  function idb(mode, fn) {
    return new Promise(function (resolve, reject) {
      var req = global.indexedDB.open(DB, 1);
      req.onupgradeneeded = function () {
        req.result.createObjectStore("k");
      };
      req.onerror = function () {
        reject(req.error);
      };
      req.onsuccess = function () {
        var db = req.result;
        var tx = db.transaction("k", mode);
        var out = fn(tx.objectStore("k"));
        tx.oncomplete = function () {
          db.close();
          resolve(out && "result" in out ? out.result : out && out.value);
        };
        tx.onerror = tx.onabort = function () {
          db.close();
          reject(tx.error);
        };
      };
    });
  }
  var store = {
    get: function (key) {
      return idb("readonly", function (s) {
        return s.get(key);
      });
    },
    put: function (key, value) {
      return idb("readwrite", function (s) {
        s.put(value, key);
      });
    },
    /** Guarda `value` solo si no hay nada; devuelve lo que queda guardado. Atómico. */
    putIfAbsent: function (key, value) {
      return idb("readwrite", function (s) {
        var out = { value: undefined };
        var req = s.get(key);
        req.onsuccess = function () {
          if (req.result === undefined) {
            s.put(value, key);
            out.value = value;
          } else {
            out.value = req.result;
          }
        };
        return out;
      });
    },
    del: function (key) {
      return idb("readwrite", function (s) {
        s.delete(key);
      });
    },
    keys: function () {
      return idb("readonly", function (s) {
        return s.getAllKeys();
      });
    },
    clear: function () {
      return idb("readwrite", function (s) {
        s.clear();
      });
    },
  };

  var E = {
    LABEL: LABEL,
    PENDING_TTL: 10 * 60 * 1000,
    b64: b64,
    b64u: b64u,
    unb64: unb64,
    same: same,
    random: random,
    wipe: wipe,
    sealedKey: sealedKey,
    hint: hint,
    passwordIKM: passwordIKM,
    passwordKEK: passwordKEK,
    passkeyKEK: passkeyKEK,
    kekFrom: kekFrom,
    wrapKey: wrapKey,
    vaultWrapKey: vaultWrapKey,
    passkeyPRFInput: passkeyPRFInput,
    sealEnvelope: sealEnvelope,
    openEnvelope: openEnvelope,
    sealDelivery: sealDelivery,
    openDelivery: openDelivery,
    store: store,
  };

  /** La clave del almacén: se crea una vez, sin carreras entre pestañas. */
  E.vault = function () {
    return E.store.get("vault").then(function (k) {
      if (sealedKey(k)) return k;
      return subtle.generateKey(AES, false, KEK_USES).then(function (fresh) {
        var put = E.store.putIfAbsent
          ? E.store.putIfAbsent("vault", fresh)
          : E.store.put("vault", fresh).then(function () {
              return fresh;
            });
        return put.then(function (kept) {
          if (!sealedKey(kept)) throw new Error("vault");
          return kept;
        });
      });
    });
  };
  function boxAAD(what) {
    return enc.encode(LABEL + "/box\0" + what);
  }
  /** Cifra bytes con la clave del almacén para `what` (que va en el AAD). */
  E.hide = function (bytes, what) {
    return E.vault().then(function (v) {
      var iv = random(12);
      return encrypt(v, iv, bytes, boxAAD(what)).then(function (ct) {
        return { iv: b64(iv), ct: b64(ct) };
      });
    });
  };
  /** Descifra lo que guardó hide. Quien recibe los bytes los pone a cero. */
  E.reveal = function (box, what) {
    if (!box || typeof box.iv !== "string" || typeof box.ct !== "string")
      return Promise.reject(new Error("box"));
    return E.vault().then(function (v) {
      return decrypt(v, unb64(box.iv), unb64(box.ct), boxAAD(what));
    });
  };

  /** Lo que el almacén guarda de la clave de desbloqueo de `sub`. No toca `key`. */
  E.unlockRecord = function (sub, key) {
    return Promise.all([
      hint(key),
      wrapKey(key, "sytue-id", ["encrypt", "decrypt"]),
      vaultWrapKey(key, ["encrypt", "decrypt"]),
      E.hide(key, "auk\0" + sub),
    ]).then(function (r) {
      return {
        v: 2,
        hint: b64(r[0]),
        wrap: r[1],
        vaultWrap: r[2],
        box: r[3],
      };
    });
  };
  E.saveUnlock = function (sub, key) {
    return E.unlockRecord(sub, key).then(function (rec) {
      return E.store.put("auk:" + sub, rec).then(function () {
        return rec;
      });
    });
  };
  /** Los bytes de la clave de desbloqueo de `sub`: solo el relevo los pide. */
  E.revealUnlock = function (sub, rec) {
    return E.reveal(rec.box, "auk\0" + sub);
  };
  function validUnlock(rec) {
    return (
      !!rec &&
      rec.v === 2 &&
      typeof rec.hint === "string" &&
      sealedKey(rec.wrap) &&
      !!rec.box
    );
  }
  function legacyAAD(item) {
    return (
      "legacy\0" + item.kind + "\0" + (item.salt || item.credential_id || "")
    );
  }
  /** Lo que abre un envoltorio de la 0082: su clave no extraíble (para el puente). */
  E.legacyKey = function (item, ikm) {
    return wrapKey(ikm, item.kind, ["decrypt"]).then(function (key) {
      return {
        kind: item.kind,
        salt: item.salt || "",
        credential_id: item.credential_id || "",
        key: key,
      };
    });
  };
  /** Lo mismo cifrado con la clave del almacén (para entregarlo a una app nativa). */
  E.legacyBox = function (item, ikm) {
    return E.hide(ikm, legacyAAD(item)).then(function (box) {
      return {
        kind: item.kind,
        salt: item.salt || "",
        credential_id: item.credential_id || "",
        box: box,
      };
    });
  };
  E.revealLegacy = function (item) {
    return E.reveal(item.box, legacyAAD(item));
  };

  /*
   * La conversión de lo que guardaron en claro las páginas de la ADR 0084: una vez
   * por página, antes de leer nada. Lo que no se reconoce se borra.
   */
  function bytes32(a) {
    return a instanceof Uint8Array && a.length === 32;
  }
  function convert(name, value) {
    if (name === "vault")
      return Promise.resolve(sealedKey(value) ? value : null);
    if (!value || typeof value !== "object") return Promise.resolve(null);
    if (value.v === 2) return Promise.resolve(value);
    if (name === "pending" && bytes32(value.kek)) {
      var legacy = Array.isArray(value.legacy) ? value.legacy : [];
      return Promise.all([
        aesKey(value.kek, KEK_USES),
        Promise.all(
          legacy
            .filter(function (l) {
              return bytes32(l.ikm);
            })
            .map(function (l) {
              return E.legacyBox(l, l.ikm);
            }),
        ),
      ]).then(function (r) {
        var out = {
          v: 2,
          kind: value.kind,
          kek: r[0],
          legacy: r[1],
          at: value.at,
        };
        if (value.kdf) out.kdf = value.kdf;
        if (value.cred) out.cred = value.cred;
        return out;
      });
    }
    if (name.indexOf("auk:") === 0 && bytes32(value.key))
      return E.unlockRecord(name.slice(4), value.key);
    if (name.indexOf("legacy:") === 0 && Array.isArray(value.items)) {
      return Promise.all(
        value.items
          .filter(function (l) {
            return bytes32(l.ikm);
          })
          .map(function (l) {
            return E.legacyKey(l, l.ikm);
          }),
      ).then(function (items) {
        return { v: 2, at: value.at, items: items };
      });
    }
    return Promise.resolve(null);
  }
  function clearBytes(value) {
    if (!value || typeof value !== "object") return;
    wipe(value.kek, value.key, value.hint);
    [value.legacy, value.items].forEach(function (list) {
      if (Array.isArray(list))
        list.forEach(function (l) {
          if (l) wipe(l.ikm);
        });
    });
  }
  var migrated = null;
  E.migrate = function () {
    if (!migrated) {
      migrated = E.store
        .keys()
        .then(function (names) {
          var chain = Promise.resolve();
          (names || []).forEach(function (name) {
            name = String(name);
            chain = chain.then(function () {
              return E.store.get(name).then(function (value) {
                return convert(name, value).then(function (next) {
                  if (next === value) return undefined;
                  clearBytes(value);
                  return next === null
                    ? E.store.del(name)
                    : E.store.put(name, next);
                });
              });
            });
          });
          return chain;
        })
        .catch(function () {
          // Un almacén que no se puede convertir no se lee: se vacía. La persona
          // vuelve a entrar en Sytue ID y todo se rehace sin nada en claro.
          return E.store.clear().catch(function () {});
        });
    }
    return migrated;
  };
  /** Lee un registro ya convertido; null si no está o no tiene la forma de la 0085. */
  E.load = function (name) {
    return E.migrate().then(function () {
      return E.store.get(name).then(function (v) {
        if (name.indexOf("auk:") === 0) return validUnlock(v) ? v : null;
        return v && v.v === 2 ? v : null;
      });
    });
  };

  /**
   * La copia del sobre de contraseña vigente (público: es lo que guarda Sytue ID)
   * junto a la clave de `sub`, para poder rehacerlo en el momento de cambiar la
   * contraseña comprobando antes la actual. Solo si es de la misma clave (huella).
   */
  E.keepPassword = function (sub, env) {
    if (!env || env.kind !== "password") return Promise.resolve();
    return E.load("auk:" + sub).then(function (rec) {
      if (!rec || env.hint !== rec.hint) return undefined;
      if (rec.pw && rec.pw.ciphertext === env.ciphertext) return undefined;
      var next = {};
      Object.keys(rec).forEach(function (k) {
        next[k] = rec[k];
      });
      next.pw = {
        v: env.v,
        kind: env.kind,
        credential_id: "",
        kdf: env.kdf,
        hint: env.hint,
        nonce: env.nonce,
        ciphertext: env.ciphertext,
      };
      return E.store.put("auk:" + sub, next);
    });
  };

  /**
   * El sobre de contraseña para la contraseña NUEVA, en el momento de cambiarla
   * (ADR 0085): solo si la ACTUAL abre el sobre vigente de este navegador (quien la
   * sabe ya podría abrirlo; quien no, no consigue nada), con la misma sal. La clave
   * existe en bytes solo entre abrir uno y cerrar el otro. El servidor lo guarda
   * únicamente si el cambio de contraseña sale bien.
   */
  E.passwordEnvelope = function (o) {
    return E.load("auk:" + o.sub).then(function (rec) {
      if (
        !rec ||
        !rec.pw ||
        typeof o.current !== "string" ||
        typeof o.next !== "string"
      )
        return { error: "signin" };
      var kdf = rec.pw.kdf;
      return passwordKEK(o.current, kdf)
        .then(function (kek) {
          return openEnvelope(kek, o.sub, rec.pw);
        })
        .then(
          function (key) {
            return passwordKEK(o.next, kdf)
              .then(function (kek) {
                return sealEnvelope({
                  kek: kek,
                  kind: "password",
                  user: o.sub,
                  cred: "",
                  kdf: kdf,
                  key: key,
                });
              })
              .then(
                function (env) {
                  wipe(key);
                  return { envelope: env };
                },
                function (err) {
                  wipe(key);
                  throw err;
                },
              );
          },
          function () {
            return { error: "wrong" };
          },
        );
    });
  };

  /**
   * El sobre de una passkey recién dada de alta (ADR 0085): la clave de desbloqueo
   * que ya tiene este navegador, cerrada con la KEK de la PRF de ESA passkey, y
   * subida con la sesión. Así la passkey desbloquea desde ya en cualquier
   * dispositivo, sin esperar a su primer uso. `kek` es la CryptoKey no extraíble de
   * la PRF; devuelve false si este navegador no tiene la clave de `sub`.
   */
  E.enrollPasskey = function (o) {
    var f = o.fetch || global.fetch.bind(global);
    return Promise.all([E.load("auk:" + o.sub), passkeyPRFInput()]).then(
      function (r) {
        var rec = r[0];
        if (!rec) return false;
        return E.revealUnlock(o.sub, rec).then(function (key) {
          return sealEnvelope({
            kek: o.kek,
            kind: "passkey",
            user: o.sub,
            cred: o.cred,
            kdf: { alg: "webauthn-prf-hkdf-sha256", prf_salt: b64(r[1]) },
            key: key,
          })
            .then(
              function (env) {
                wipe(key);
                if (env.hint !== rec.hint) throw new Error("hint");
                return f("/e2e/envelopes", {
                  method: "PUT",
                  credentials: "same-origin",
                  headers: { "Content-Type": "application/json" },
                  body: JSON.stringify({
                    envelope: env,
                    reset: false,
                    sub: o.sub,
                  }),
                });
              },
              function (err) {
                wipe(key);
                throw err;
              },
            )
            .then(function (res) {
              if (!res.ok) throw new Error("envelope");
              return true;
            });
        });
      },
    );
  };

  global.SytueE2E = E;
})(typeof window !== "undefined" ? window : globalThis);
