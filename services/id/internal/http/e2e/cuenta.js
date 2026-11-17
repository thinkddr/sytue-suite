/* SPDX-License-Identifier: Apache-2.0 */
/*
 * Cambio de contraseña en «Mi cuenta» de Sytue ID (ADR 0085): antes de enviar el
 * formulario, este navegador cierra la clave de desbloqueo con la contraseña NUEVA
 * (solo si la actual abre el sobre vigente; comun.js, passwordEnvelope) y lo manda
 * con el cambio. El servidor lo guarda únicamente si el cambio sale bien: desde ese
 * momento la contraseña nueva desbloquea en cualquier dispositivo, sin esperar al
 * siguiente inicio de sesión. Si algo falla, el cambio se envía igual y el sobre se
 * rehace al entrar, como antes.
 */
(function (global) {
  "use strict";
  var E = global.SytueE2E;
  var doc = global.document;
  if (!E) return;
  function noop() {}

  /** Envía el formulario con el sobre si se pudo cerrar; nunca impide el cambio. */
  E.passwordChangeForm = function (form, opts) {
    opts = opts || {};
    var ready = false;
    form.addEventListener("submit", function (ev) {
      if (ready || ev.defaultPrevented) return;
      var sub = form.getAttribute("data-sub");
      var cur = form.querySelector("input[name=current_password]");
      var next = form.querySelector("input[name=new_password]");
      var out = form.querySelector("input[name=unlock_envelope]");
      if (!sub || !cur || !next || !out || !cur.value || !next.value) return;
      if (!global.hashwasm || !global.indexedDB) return;
      ev.preventDefault();
      var button = form.querySelector("button[type=submit]");
      if (button) button.disabled = true;
      var sent = false;
      function send() {
        if (sent) return;
        sent = true;
        ready = true;
        (opts.submit || global.HTMLFormElement.prototype.submit).call(form);
      }
      var timer = global.setTimeout(send, 20000);
      var job;
      try {
        job = E.passwordEnvelope({
          sub: sub,
          current: cur.value,
          next: next.value,
        });
      } catch (e) {
        job = Promise.reject(e);
      }
      job
        .then(function (r) {
          if (r && r.envelope) out.value = JSON.stringify(r.envelope);
        })
        .catch(noop)
        .then(function () {
          global.clearTimeout(timer);
          send();
        });
    });
  };

  var form =
    doc &&
    doc.querySelector &&
    doc.querySelector('form[action="/account/password"]');
  if (form) E.passwordChangeForm(form);
})(typeof window !== "undefined" ? window : globalThis);
