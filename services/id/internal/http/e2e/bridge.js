/* SPDX-License-Identifier: Apache-2.0 */
/*
 * Puente del desbloqueo (ADR 0084 y 0085): una página de Sytue ID que una app web de
 * la suite carga en un iframe oculto para usar la clave de desbloqueo que dejó aquí
 * el relevo. Así un solo inicio de sesión desbloquea todas las apps de este
 * navegador sin que la clave pase por ningún servidor.
 *
 * Desde la ADR 0085 la clave NO sale de este origen: el puente no la puede leer (es
 * una CryptoKey no extraíble) y no la entrega. Hace él las dos operaciones que una
 * app necesita y le devuelve solo su resultado:
 *
 * - `sytue-e2e-unwrap`: abre el envoltorio de la clave de recuperación de UNA cuenta
 *   de Matrix de la persona y devuelve esa clave de recuperación, que el SDK de la
 *   app necesita igualmente. En la transición, también un envoltorio de la 0082.
 * - `sytue-e2e-wrap`: envuelve la clave de recuperación de una cuenta de la persona
 *   y devuelve el envoltorio (público: es lo que guarda Rooms).
 * - `sytue-e2e-password`: solo para «Mi cuenta», el sobre de la contraseña nueva al
 *   cambiarla, si la actual abre el sobre vigente; lo guarda el servidor con el
 *   cambio. Quien sabe la contraseña actual ya podría abrir el sobre; quien no, no
 *   consigue nada.
 *
 * - LA VUELTA DEL INICIO DE SESIÓN (`handoff`). Safari guarda lo de un iframe
 *   aparte por cada ORIGEN de la página que lo incrusta: este puente, dentro de
 *   `calcus.sytue.eu`, no ve lo que el relevo dejó en `id.sytue.eu` a pantalla
 *   completa, así que tras entrar la app volvía a pedir entrar, sin fin (medido en
 *   producción, v3.21.0). Cuando no tiene clave, el puente contesta `signin` con la
 *   parte PÚBLICA de una clave efímera suya (`handoff_key`, la privada no extraíble
 *   y en SU almacén); la app la lleva al inicio de sesión y el relevo le devuelve la
 *   clave de desbloqueo cerrada para ella, en el FRAGMENTO de la vuelta (no llega a
 *   ningún servidor), como a una app nativa. La app se la pasa al puente sin poder
 *   abrirla (`handoff` en la siguiente petición), y el puente la guarda en su
 *   almacén. Un solo uso; la clave efímera caduca a la media hora.
 *
 * Contesta SOLO a su ventana madre y SOLO si su origen es exactamente el que el
 * servidor validó y puso en la CSP de esta página (`frame-ancestors`): otro origen,
 * otra ventana o un iframe anidado no reciben nada. Solo usa la clave de la persona
 * que la app pide (`sub`) y solo para cuentas de Matrix de ESA persona: la de Rooms
 * desde cualquier app de la suite y la de almacenamiento aislado de un producto solo
 * desde la app de ese producto. Y como mucho unas pocas operaciones por segundo.
 */
(function (global) {
  "use strict";
  var E = global.SytueE2E;
  if (!E) return;
  var enc = new TextEncoder();
  var WRAP_LABEL = "sytue-matrix-recovery-wrap-v1";
  var VAULT_WRAP_LABEL = "sytue-pq-vault-wrap-v1";
  var VAULT_PROFILE = "age-MLKEM768-X25519-v1";
  var VERSION = 2;
  var MAX_WRAPPERS = 32;
  var MAX_RECOVERY = 256;
  var RATE = 10;
  var PRODUCTS = [
    "hub",
    "mail",
    "drive",
    "docs",
    "sheets",
    "slides",
    "pdf",
    "calendar",
    "contacts",
    "notes",
    "photos",
    "rooms",
    "cast",
    "erp",
    "bases",
  ];

  // El host de la app de cada producto con cuenta aislada (`appDomains` de
  // @sytue/config): el suyo propio, si no se llama igual que el producto.
  var HOSTS = {
    docs: ["docus"],
    sheets: ["calcus"],
    slides: ["diapos"],
    pdf: ["certipdf"],
    hub: ["hub", "one"],
  };
  // Los hosts propios de la suite (los de `appDomains` y los que quedaron de antes,
  // como `docs.`, que hoy es solo la API). En uno de ellos vive una sola app, y solo
  // la de ese producto opera su cuenta aislada. Un host de organización
  // (`<organización>.<dominio>/drive`, `/mail`… a la vez) es UN origen para todas sus
  // apps: lo que una puede, lo pueden todas, así que ahí no hay producto que atar.
  var SUITE_HOSTS = [
    "docs",
    "sheets",
    "slides",
    "pdf",
    "web",
    "www",
    "admin",
    "cloud",
    "account",
    "id",
    "hub",
    "one",
    "mail",
    "drive",
    "docus",
    "calcus",
    "diapos",
    "certipdf",
    "calendar",
    "contacts",
    "notes",
    "photos",
    "rooms",
    "cast",
    "erp",
    "bases",
  ];

  /**
   * ¿Puede la app de `origin` operar sobre `account` con la clave de `sub`?
   * Las cuentas de la suite son `@sytue_<sub>:<servidor>` (Rooms) y
   * `@sytue_storage_<producto>_<organización>_<sub>:<servidor>` (aisladas).
   */
  E.accountAllowed = function (origin, sub, account) {
    if (typeof account !== "string" || account.length > 512) return false;
    var m = /^@([^:]+):[^:\s]+$/.exec(account);
    if (!m) return false;
    var local = m[1];
    if (local === "sytue_" + sub) return true;
    var iso = /^sytue_storage_([a-z]+)_(.+)$/.exec(local);
    if (!iso) return false;
    var rest = iso[2];
    if (
      rest.slice(-(sub.length + 1)) !== "_" + sub ||
      rest.length <= sub.length + 1
    )
      return false;
    var host = "";
    try {
      host = new URL(origin).hostname.split(".")[0];
    } catch (e) {
      return false;
    }
    if (SUITE_HOSTS.indexOf(host) < 0) return true;
    return (HOSTS[iso[1]] || [iso[1]]).indexOf(host) >= 0;
  };

  /**
   * ¿Puede la app de `origin` operar la bóveda post-cuántica de `product` (ADR 0086)?
   * En un host propio de la suite solo el de ese producto; en un host de organización
   * (mismo origen para todas sus apps), cualquier producto válido.
   */
  E.productAllowed = function (origin, product) {
    if (typeof product !== "string" || PRODUCTS.indexOf(product) < 0)
      return false;
    var host = "";
    try {
      host = new URL(origin).hostname.split(".")[0];
    } catch (e) {
      return false;
    }
    if (SUITE_HOSTS.indexOf(host) < 0) return true;
    return (HOSTS[product] || [product]).indexOf(host) >= 0;
  };

  function aad(kind, account, keyID, cred) {
    return enc.encode(
      [WRAP_LABEL, kind, account, keyID, cred || ""].join("\0"),
    );
  }
  function wrapperShape(w) {
    return (
      !!w &&
      typeof w === "object" &&
      w.v === 1 &&
      typeof w.kind === "string" &&
      typeof w.key_id === "string" &&
      typeof w.credential_id === "string" &&
      typeof w.nonce === "string" &&
      typeof w.ciphertext === "string" &&
      !!w.kdf &&
      typeof w.kdf === "object"
    );
  }
  function openWith(key, account, w) {
    return global.crypto.subtle
      .decrypt(
        {
          name: "AES-GCM",
          iv: E.unb64(w.nonce),
          additionalData: aad(w.kind, account, w.key_id, w.credential_id),
        },
        key,
        E.unb64(w.ciphertext),
      )
      .then(function (plain) {
        var bytes = new Uint8Array(plain);
        try {
          return new TextDecoder("utf-8", { fatal: true }).decode(bytes);
        } finally {
          E.wipe(bytes);
        }
      });
  }
  /** Prueba en orden; el primero que abre gana. */
  function first(list, fn) {
    var chain = Promise.resolve(null);
    list.forEach(function (x) {
      chain = chain.then(function (done) {
        return done !== null
          ? done
          : fn(x).catch(function () {
              return null;
            });
      });
    });
    return chain;
  }
  function legacyFor(sub, now) {
    return E.load("legacy:" + sub).then(function (old) {
      if (!old || !Array.isArray(old.items) || now - old.at > E.PENDING_TTL)
        return [];
      return old.items.filter(function (l) {
        return E.sealedKey(l.key);
      });
    });
  }

  /** `sytue-e2e-unwrap`: la clave de recuperación de una cuenta, o por qué no. */
  E.bridgeUnwrap = function (m, now) {
    var wrappers = Array.isArray(m.wrappers)
      ? m.wrappers.slice(0, MAX_WRAPPERS).filter(wrapperShape)
      : [];
    return Promise.all([
      E.load("auk:" + m.sub).catch(function () {
        return null;
      }),
      legacyFor(m.sub, now).catch(function () {
        return [];
      }),
    ]).then(function (r) {
      var rec = r[0];
      var legacy = r[1];
      var stale = false;
      var mine = wrappers.filter(function (w) {
        if (
          w.kind !== "sytue-id" ||
          w.kdf.alg !== "sytue-id-unlock-hkdf-sha256"
        )
          return false;
        if (rec && w.kdf.hint === rec.hint) return true;
        // Cerrado con una clave de desbloqueo que la persona ya no tiene.
        stale = true;
        return false;
      });
      return first(mine, function (w) {
        return openWith(rec.wrap, m.account, w).then(function (key) {
          return { recovery_key: key, kind: "sytue-id" };
        });
      }).then(function (got) {
        if (got) return got;
        var pairs = [];
        legacy.forEach(function (l) {
          wrappers.forEach(function (w) {
            if (
              w.kind === l.kind &&
              ((w.kind === "password" &&
                w.kdf.alg === "argon2id" &&
                w.kdf.salt === l.salt) ||
                (w.kind === "passkey" && w.credential_id === l.credential_id))
            )
              pairs.push({ key: l.key, w: w });
          });
        });
        return first(pairs, function (p) {
          return openWith(p.key, m.account, p.w).then(function (key) {
            return { recovery_key: key, kind: p.w.kind };
          });
        }).then(function (old) {
          if (old) return old;
          // Sin clave aquí: que entre en Sytue ID. Con una clave que no es la de
          // sus envoltorios (empezó de cero en otro sitio): se distingue.
          return { error: rec && stale ? "wrong" : "signin" };
        });
      });
    });
  };

  /** `sytue-e2e-wrap`: el envoltorio `sytue-id` de una clave de recuperación. */
  E.bridgeWrap = function (m, now) {
    if (
      typeof m.key_id !== "string" ||
      !m.key_id ||
      m.key_id.length > 255 ||
      typeof m.recovery_key !== "string" ||
      !m.recovery_key ||
      m.recovery_key.length > MAX_RECOVERY
    )
      return Promise.resolve({ error: "invalid" });
    return Promise.all([
      E.load("auk:" + m.sub).catch(function () {
        return null;
      }),
      legacyFor(m.sub, now).catch(function () {
        return [];
      }),
    ]).then(function (r) {
      var rec = r[0];
      if (!rec) return { error: "signin" };
      var nonce = E.random(12);
      var plain = enc.encode(m.recovery_key);
      return global.crypto.subtle
        .encrypt(
          {
            name: "AES-GCM",
            iv: nonce,
            additionalData: aad("sytue-id", m.account, m.key_id, ""),
          },
          rec.wrap,
          plain,
        )
        .then(function (ct) {
          E.wipe(plain);
          return {
            wrapper: {
              v: 1,
              kind: "sytue-id",
              key_id: m.key_id,
              credential_id: "",
              kdf: { alg: "sytue-id-unlock-hkdf-sha256", hint: rec.hint },
              nonce: E.b64(nonce),
              ciphertext: E.b64(new Uint8Array(ct)),
            },
            legacy: r[1].length > 0,
          };
        });
    });
  };

  function vaultScopePart(s, max) {
    return (
      typeof s === "string" &&
      s.length > 0 &&
      s.length <= max &&
      !/[\0\r\n]/.test(s)
    );
  }
  function vaultAAD(tenant, product, sub, keyID) {
    return enc.encode(
      [VAULT_WRAP_LABEL, tenant, product, sub, keyID].join("\0"),
    );
  }
  function vaultWrapperShape(w) {
    return (
      !!w &&
      typeof w === "object" &&
      w.v === 1 &&
      w.kind === "sytue-id" &&
      w.profile === VAULT_PROFILE &&
      vaultScopePart(w.key_id, 128) &&
      typeof w.recipient === "string" &&
      w.recipient.length > 0 &&
      w.recipient.length <= 4096 &&
      typeof w.nonce === "string" &&
      typeof w.ciphertext === "string" &&
      !!w.kdf &&
      typeof w.kdf === "object" &&
      w.kdf.alg === "sytue-id-unlock-hkdf-sha256" &&
      typeof w.kdf.hint === "string"
    );
  }
  function vaultSecretShape(s) {
    return (
      !!s &&
      typeof s === "object" &&
      s.v === 1 &&
      s.profile === VAULT_PROFILE &&
      typeof s.identity === "string" &&
      s.identity.length > 0 &&
      s.identity.length <= 4096 &&
      typeof s.owner_key === "string" &&
      s.owner_key.length > 0 &&
      s.owner_key.length <= 128
    );
  }
  /** Deriva `vaultWrap` bajo demanda para cuentas iniciadas antes de ADR 0086. */
  function ensureVaultWrap(sub, rec) {
    if (!rec) return Promise.resolve(null);
    if (E.sealedKey(rec.vaultWrap)) return Promise.resolve(rec);
    return E.revealUnlock(sub, rec).then(function (key) {
      return E.vaultWrapKey(key, ["encrypt", "decrypt"]).then(
        function (vw) {
          E.wipe(key);
          var next = {};
          Object.keys(rec).forEach(function (k) {
            next[k] = rec[k];
          });
          next.vaultWrap = vw;
          return E.store.put("auk:" + sub, next).then(function () {
            return next;
          });
        },
        function (err) {
          E.wipe(key);
          throw err;
        },
      );
    });
  }
  function openVaultWith(key, tenant, product, sub, w) {
    return global.crypto.subtle
      .decrypt(
        {
          name: "AES-GCM",
          iv: E.unb64(w.nonce),
          additionalData: vaultAAD(tenant, product, sub, w.key_id),
        },
        key,
        E.unb64(w.ciphertext),
      )
      .then(function (plain) {
        var bytes = new Uint8Array(plain);
        try {
          var secret = JSON.parse(
            new TextDecoder("utf-8", { fatal: true }).decode(bytes),
          );
          if (!vaultSecretShape(secret)) throw new Error("secret");
          return secret;
        } finally {
          E.wipe(bytes);
        }
      });
  }

  /** `sytue-e2e-vault-unwrap`: abre el envoltorio PQ de un producto (ADR 0086). */
  E.bridgeVaultUnwrap = function (m) {
    if (
      !vaultScopePart(m.tenant, 128) ||
      typeof m.product !== "string" ||
      PRODUCTS.indexOf(m.product) < 0
    )
      return Promise.resolve({ error: "invalid" });
    var wrappers = Array.isArray(m.wrappers)
      ? m.wrappers.slice(0, MAX_WRAPPERS).filter(vaultWrapperShape)
      : [];
    return E.load("auk:" + m.sub)
      .catch(function () {
        return null;
      })
      .then(function (rec) {
        return ensureVaultWrap(m.sub, rec);
      })
      .then(function (rec) {
        if (!rec) return { error: "signin" };
        var mine = wrappers.filter(function (w) {
          return w.kdf.hint === rec.hint;
        });
        return first(mine, function (w) {
          return openVaultWith(
            rec.vaultWrap,
            m.tenant,
            m.product,
            m.sub,
            w,
          ).then(function (secret) {
            return {
              secret: secret,
              key_id: w.key_id,
              recipient: w.recipient,
              profile: w.profile,
            };
          });
        }).then(function (got) {
          if (got) return got;
          return { error: "wrong" };
        });
      });
  };

  /** `sytue-e2e-vault-wrap`: envuelve una clave de bóveda PQ con la AUK (ADR 0086). */
  E.bridgeVaultWrap = function (m) {
    if (
      !vaultScopePart(m.tenant, 128) ||
      typeof m.product !== "string" ||
      PRODUCTS.indexOf(m.product) < 0 ||
      m.profile !== VAULT_PROFILE ||
      !vaultScopePart(m.key_id, 128) ||
      typeof m.recipient !== "string" ||
      !m.recipient ||
      m.recipient.length > 4096 ||
      !vaultSecretShape(m.secret)
    )
      return Promise.resolve({ error: "invalid" });
    return E.load("auk:" + m.sub)
      .catch(function () {
        return null;
      })
      .then(function (rec) {
        return ensureVaultWrap(m.sub, rec);
      })
      .then(function (rec) {
        if (!rec) return { error: "signin" };
        var nonce = E.random(12);
        var plain = enc.encode(
          JSON.stringify({
            v: 1,
            profile: m.secret.profile,
            identity: m.secret.identity,
            owner_key: m.secret.owner_key,
          }),
        );
        return global.crypto.subtle
          .encrypt(
            {
              name: "AES-GCM",
              iv: nonce,
              additionalData: vaultAAD(m.tenant, m.product, m.sub, m.key_id),
            },
            rec.vaultWrap,
            plain,
          )
          .then(function (ct) {
            E.wipe(plain);
            return {
              wrapper: {
                v: 1,
                kind: "sytue-id",
                profile: m.profile,
                key_id: m.key_id,
                recipient: m.recipient,
                kdf: { alg: "sytue-id-unlock-hkdf-sha256", hint: rec.hint },
                nonce: E.b64(nonce),
                ciphertext: E.b64(new Uint8Array(ct)),
              },
            };
          });
      });
  };

  var HANDOFF_TTL = 30 * 60 * 1000;
  var CURVE = { name: "ECDH", namedCurve: "P-256" };
  function handoffRecord(h, now) {
    return (
      !!h &&
      h.v === 2 &&
      !!h.priv &&
      h.priv.type === "private" &&
      h.priv.extractable === false &&
      typeof h.pub === "string" &&
      typeof h.at === "number" &&
      now >= h.at &&
      now - h.at < HANDOFF_TTL
    );
  }
  /** La parte pública de la clave efímera de la vuelta (la crea si no hay una viva). */
  E.handoffKey = function (now) {
    return E.store.get("handoff").then(function (h) {
      if (handoffRecord(h, now)) return h.pub;
      return global.crypto.subtle
        .generateKey(CURVE, false, ["deriveBits"])
        .then(function (pair) {
          return global.crypto.subtle
            .exportKey("raw", pair.publicKey)
            .then(function (raw) {
              var pub = E.b64u(new Uint8Array(raw));
              return E.store
                .put("handoff", {
                  v: 2,
                  priv: pair.privateKey,
                  pub: pub,
                  at: now,
                })
                .then(function () {
                  return pub;
                });
            });
        });
    });
  };
  /**
   * Abre la vuelta del relevo con la clave efímera y guarda la clave de desbloqueo de
   * `sub` (y lo que abre sus envoltorios de la 0082) en el almacén de ESTE puente.
   * Un solo uso: la clave efímera se borra en cuanto abre una. Nunca rechaza: lo que
   * no abre se ignora (y no gasta la clave) y la operación sigue como si nada.
   */
  E.takeHandoff = function (sub, blob, now) {
    if (
      typeof blob !== "string" ||
      !blob ||
      blob.length > 16384 ||
      !/^[A-Za-z0-9_-]+$/.test(blob)
    )
      return Promise.resolve(false);
    return E.store
      .get("handoff")
      .then(function (h) {
        if (!handoffRecord(h, now)) return false;
        return E.openDelivery(h.priv, h.pub, blob).then(function (d) {
          var key = d && typeof d.key === "string" ? E.unb64(d.key) : null;
          // De otra persona (otra cuenta en Sytue ID): ni se guarda ni gasta la clave.
          if (!d || d.v !== 1 || d.sub !== sub || !key || key.length !== 32) {
            E.wipe(key);
            return false;
          }
          return E.store.del("handoff").then(function () {
            var legacy = Array.isArray(d.legacy) ? d.legacy.slice(0, 4) : [];
            return E.saveUnlock(sub, key)
              .then(function () {
                return Promise.all(
                  legacy
                    .filter(function (l) {
                      return (
                        l &&
                        typeof l.ikm === "string" &&
                        typeof l.kind === "string"
                      );
                    })
                    .map(function (l) {
                      var ikm = E.unb64(l.ikm);
                      return E.legacyKey(
                        {
                          kind: l.kind,
                          salt: l.salt || "",
                          credential_id: l.credential_id || "",
                        },
                        ikm,
                      ).then(function (item) {
                        E.wipe(ikm);
                        return item;
                      });
                    }),
                );
              })
              .then(function (items) {
                if (!items.length) return undefined;
                return E.store.put("legacy:" + sub, {
                  v: 2,
                  items: items,
                  at: now,
                });
              })
              .then(
                function () {
                  E.wipe(key);
                  return true;
                },
                function (err) {
                  E.wipe(key);
                  throw err;
                },
              );
          });
        });
      })
      .catch(function () {
        return false;
      });
  };

  E.bridge = function (cfg, win, now) {
    var origin = cfg && cfg.origin;
    var parent = win.parent;
    var clock = now || Date.now;
    if (!origin || !parent || parent === win) return false;
    var recent = [];
    function reply(id, body) {
      body.type = "sytue-e2e-result";
      body.id = id;
      parent.postMessage(body, origin);
    }
    win.addEventListener("message", function (ev) {
      if (ev.source !== parent || ev.origin !== origin) return;
      var m = ev.data;
      if (
        !m ||
        typeof m !== "object" ||
        typeof m.id !== "string" ||
        typeof m.sub !== "string" ||
        !m.sub ||
        m.sub.length > 255
      )
        return;
      var t = clock();
      recent = recent.filter(function (x) {
        return t - x < 1000;
      });
      if (recent.length >= RATE) {
        reply(m.id, { ok: false, error: "busy" });
        return;
      }
      recent.push(t);
      var op;
      if (m.type === "sytue-e2e-unwrap" || m.type === "sytue-e2e-wrap") {
        if (!E.accountAllowed(origin, m.sub, m.account)) {
          reply(m.id, { ok: false, error: "account" });
          return;
        }
        // La vuelta del inicio de sesión, si la trae, antes de la operación.
        op = (
          m.handoff ? E.takeHandoff(m.sub, m.handoff, t) : Promise.resolve()
        ).then(function () {
          return m.type === "sytue-e2e-unwrap"
            ? E.bridgeUnwrap(m, t)
            : E.bridgeWrap(m, t);
        });
      } else if (
        m.type === "sytue-e2e-vault-unwrap" ||
        m.type === "sytue-e2e-vault-wrap"
      ) {
        if (!E.productAllowed(origin, m.product)) {
          reply(m.id, { ok: false, error: "account" });
          return;
        }
        op = (
          m.handoff ? E.takeHandoff(m.sub, m.handoff, t) : Promise.resolve()
        ).then(function () {
          return m.type === "sytue-e2e-vault-unwrap"
            ? E.bridgeVaultUnwrap(m)
            : E.bridgeVaultWrap(m);
        });
      } else if (m.type === "sytue-e2e-password" && cfg.password === true) {
        // Solo «Mi cuenta» (el servidor lo dice en `password`): el sobre de la
        // contraseña nueva, si la actual abre el vigente (ADR 0085).
        op = E.passwordEnvelope({
          sub: m.sub,
          current: m.current,
          next: m.next,
        });
      } else if (m.type === "sytue-e2e-forget-legacy") {
        op = E.migrate()
          .then(function () {
            return E.store.del("legacy:" + m.sub);
          })
          .then(function () {
            return {};
          });
      } else {
        // `sytue-e2e-get` (la versión anterior, que entregaba la clave) ya no existe.
        return;
      }
      op.then(
        function (out) {
          // Sin clave aquí: la clave efímera con la que el relevo puede devolverla.
          if (out.error === "signin")
            return E.handoffKey(t).then(
              function (pub) {
                reply(m.id, { ok: false, error: "signin", handoff_key: pub });
              },
              function () {
                reply(m.id, { ok: false, error: "signin" });
              },
            );
          if (out.error) reply(m.id, { ok: false, error: out.error });
          else {
            out.ok = true;
            reply(m.id, out);
          }
        },
        function () {
          reply(m.id, { ok: false, error: "unavailable" });
        },
      );
    });
    // La conversión de lo que hubiera en claro, antes de la primera operación.
    E.migrate();
    parent.postMessage({ type: "sytue-e2e-ready", v: VERSION }, origin);
    return true;
  };

  var doc = global.document;
  var el = doc && doc.getElementById("e2e-bridge");
  if (!el) return;
  E.bridge(JSON.parse(el.textContent), global);
})(typeof window !== "undefined" ? window : globalThis);
