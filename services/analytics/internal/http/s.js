/* Sytue Analytics. Lo prohibido, el porqué y el test que lo verifica sobre
   estos bytes están en script.go; aquí no se nombra ninguna de esas APIs ni en
   un comentario, porque el test no distingue. */
(function () {
  try {
    var n = navigator, w = window, d = document, l = location;
    /* GPC y DNT, sin interruptor que los ignore. */
    if (n.globalPrivacyControl === true || n.doNotTrack === '1' || w.doNotTrack === '1') return;
    if (l.protocol !== 'https:' && l.hostname !== 'localhost') return;

    /* La query string se descarta AQUÍ: un ?email= o un ?reset_token= que llegue
       al servidor ya ha cruzado la red y ha pasado por un log de acceso. */
    var K = ['utm_source', 'utm_medium', 'utm_campaign', 'utm_content', 'utm_term', 'ref'], last = '';

    function send(name, target) {
      try {
        var q = {}, p = new URLSearchParams(l.search), i, v, r = '';
        for (i = 0; i < 6; i++) { v = p.get(K[i]); if (v) q[K[i]] = v.slice(0, 64); }
        try { r = d.referrer ? new URL(d.referrer).hostname : ''; } catch (e) { }
        var b = JSON.stringify({ n: name, p: l.pathname.slice(0, 255), r: r, q: q, x: target || '' });
        if (!(n.sendBeacon && n.sendBeacon('/_sytue/e', b))) {
          fetch('/_sytue/e', { method: 'POST', body: b, keepalive: true, credentials: 'omit', mode: 'same-origin' }).catch(function () { });
        }
      } catch (e) { }
    }

    function view() { if (l.pathname !== last) { last = l.pathname; send('pageview'); } }

    /* SPA: sin esto, una app con enrutado en cliente reporta una sola página. */
    var h = history, ps = h.pushState, rs = h.replaceState;
    h.pushState = function () { ps.apply(h, arguments); view(); };
    h.replaceState = function () { rs.apply(h, arguments); view(); };
    addEventListener('popstate', view);

    /* Un solo listener delegado: eventos declarados, salientes, descargas,
       mailto y tel. Sin API de JavaScript que integrar. */
    addEventListener('click', function (e) {
      try {
        var t = e.target, a = t && t.closest ? t.closest('a,[data-sytue-event]') : null;
        if (!a) return;
        var c = a.getAttribute('data-sytue-event');
        if (c) return send(c.slice(0, 50));
        var href = a.getAttribute('href') || '';
        if (/^mailto:/i.test(href)) return send('mailto');
        if (/^tel:/i.test(href)) return send('tel');
        var u = new URL(href, l.href);
        if (u.hostname !== l.hostname) return send('outbound', u.hostname);
        var m = /\.([a-z0-9]{1,8})$/i.exec(u.pathname);
        if (m && /^(pdf|zip|gz|tgz|rar|7z|docx?|xlsx?|pptx?|odt|ods|odp|csv|rtf|mp3|mp4|mov|wav|dmg|exe|pkg|deb|rpm|apk|iso)$/i.test(m[1])) send('download', m[1].toLowerCase());
      } catch (e) { }
    }, true);

    view();
  } catch (e) { }
})();
