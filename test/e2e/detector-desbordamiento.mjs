// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// Detector de desbordamiento horizontal, en un módulo aparte porque lo usan DOS
// puertas y una segunda copia divergiría de la primera:
//   – `mobile-audit.mjs`: barrido nocturno de las diez apps contra producción.
//   – `audit-www-estatico.mjs`: puerta de PR sobre el `dist` de www, que es la
//     única app con build estático y justo donde estuvo la regresión de agosto
//     de 2026 (23 rutas localizadas desbordando el documento entero).
//
// `DETECTOR` se serializa y corre EN LA PÁGINA (`page.evaluate`), así que no
// puede referirse a nada de fuera: todo lo que usa lo declara dentro.
//
// ponytail: dos consumidores, un módulo. No hay tercero previsto ni interfaz.

/**
 * Corre EN LA PÁGINA. Mide el desbordamiento horizontal de CADA contenedor, no
 * solo del documento: el shell de la suite envuelve el contenido en un `main`
 * con overflow propio, así que un desbordamiento real deja `document.scrollWidth`
 * intacto y solo se ve como scroll lateral dentro del panel.
 * Por cada contenedor desbordado señala el descendiente que lo provoca.
 */
export const DETECTOR = () => {
  const ruta = (el) => {
    const partes = [];
    for (
      let n = el;
      n && n.nodeType === 1 && partes.length < 4;
      n = n.parentElement
    ) {
      let s = n.tagName.toLowerCase();
      if (n.id) s += `#${n.id}`;
      else if (typeof n.className === "string" && n.className.trim())
        s += `.${n.className.trim().split(/\s+/).slice(0, 3).join(".")}`;
      partes.unshift(s);
    }
    return partes.join(" > ");
  };
  const visible = (el) => {
    const cs = getComputedStyle(el);
    return cs.visibility !== "hidden" && +cs.opacity !== 0;
  };
  /**
   * Recorte DELIBERADO: no es un defecto de layout aunque el contenido no quepa.
   *
   * `culpable` es el descendiente que MÁS rebasa el borde, y la excusa de la
   * tabla cuelga de él y no del contenedor. Antes se preguntaba
   * `el.querySelector("table, …")`, que busca en TODO el subárbol: bastaba una
   * tabla cualquiera en otra parte del panel —sin relación con lo que se
   * desbordaba— para que un desbordamiento real desapareciera del informe.
   * Medido con página sintética: un `main` de 320px con un hijo de 900px se
   * reporta +580px; añadiendo un `<table>` como hermano del culpable pasaba a
   * cero hallazgos. Siete líneas más abajo, para `pre`/`code`, ya se acotaba.
   */
  const deliberado = (el, cs, culpable) =>
    // Truncado con elipsis (Tailwind `truncate`) o a N líneas.
    cs.textOverflow === "ellipsis" ||
    cs.webkitLineClamp !== "none" ||
    // UN CAMPO DE TEXTO CON MÁS TEXTO DEL QUE CABE se desplaza por dentro: es lo
    // que hace cualquier <input> desde que existen, no un defecto de maquetación.
    // Sin esto el barrido señalaba el título del editor de Docs —«Documento sin
    // título (14)», 25 caracteres en 157px: scrollWidth 213, clientWidth 157,
    // +56px en Pixel 5 y +48 en Pixel 7— mientras su cabecera medía 393px de 393
    // y no se salía nada. El informe mandaba a tocar una cabecera que estaba bien.
    el.tagName === "INPUT" ||
    el.tagName === "TEXTAREA" ||
    // Cajas de 1px: `sr-only` (skip-link) y medidores ocultos.
    el.clientWidth < 40 ||
    el.clientHeight < 16 ||
    // Scroll horizontal legítimo: tabla o rejilla ancha (una hoja de cálculo se
    // desplaza de lado por definición) o carrusel declarado. La rejilla no
    // siempre es hija directa del contenedor que desplaza, de ahí el `closest`
    // desde el culpable (que también acierta si el culpable ES la tabla).
    !!el.closest("[data-scroll-x]") ||
    (!!culpable &&
      !!culpable.closest(
        "table, [role=table], [role=grid], [role=treegrid]",
      )) ||
    // Y hacia ABAJO, con una condición. El culpable puede ser un ENVOLTORIO de la
    // rejilla y no la rejilla: en el editor de hojas hay un div entre el scroller y
    // el `role="grid"` que aplica el zoom, así que `closest` —que solo sube— no la
    // encontraba y una hoja de cálculo salía como defecto de maquetación en las tres
    // ventanas de escritorio (+1736px a 880, +1336 a 1280, +1080 a 1536: siempre el
    // mismo lienzo de 2616px, que es justo lo que delata que era ancho fijo).
    //
    // La condición evita el fallo que costó el comentario de arriba: se exige que la
    // rejilla sea LA QUE LO ENSANCHA —mismo ancho que el culpable, con 4px de
    // margen—. Una tabla pequeña dentro de algo ancho por otro motivo no excusa nada.
    (!!culpable &&
      (() => {
        const r = culpable.querySelector(
          "table, [role=table], [role=grid], [role=treegrid]",
        );
        return (
          !!r &&
          r.getBoundingClientRect().width >=
            culpable.getBoundingClientRect().width - 4
        );
      })()) ||
    el.tagName === "TABLE" ||
    // Un bloque de código no se parte: `$ git push sytue main` con salto de
    // línea deja de ser el comando que se copia. Un `<pre>` que desplaza lo hace
    // a propósito, igual que una tabla ancha — 33 combinaciones de www salían
    // como defecto por esto y no lo eran.
    el.tagName === "PRE" ||
    !!el.querySelector(":scope > pre, :scope > code") ||
    ["table", "grid", "treegrid"].includes(el.getAttribute("role") ?? "");

  /**
   * Segunda pasada: lo que se sale por la IZQUIERDA. `scrollWidth` no lo ve —una
   * coordenada X negativa no genera scroll en LTR, se recorta— y es el caso
   * PEOR, porque a lo recortado por la izquierda no se llega ni desplazando.
   * Así se colaba el centro de notificaciones (−86px a 320px) en 18 perfiles.
   * Solo el elemento MÁS EXTERNO de cada panel: si no, un panel de 330px se
   * reporta 20 veces (una por hijo) y el informe deja de leerse.
   */
  const fueraIzq = () => {
    const izq = [];
    for (const d of document.querySelectorAll("body *")) {
      const r = d.getBoundingClientRect();
      // Adornos absolutos y medidores ocultos: por debajo de este tamaño no hay
      // contenido que perder. Y lo que vive dentro de un scroller lateral SÍ se
      // alcanza (marcado o no), así que no es este defecto.
      if (r.left >= -1 || r.width < 24 || r.height < 12 || !visible(d))
        continue;
      if (d.closest("[aria-hidden=true],[inert],[data-scroll-x]")) continue;
      if (izq.some((x) => x.el.contains(d))) continue;
      izq.push({
        el: d,
        sel: ruta(d),
        fuera: Math.round(-r.left),
        w: Math.round(r.width),
        txt: (d.textContent || "").trim().slice(0, 40),
      });
    }
    return izq.map(({ el: _el, ...resto }) => resto);
  };

  const hallazgos = [];
  const raiz = document.scrollingElement || document.documentElement;
  for (const el of [raiz, ...document.querySelectorAll("body *")]) {
    const exceso = el.scrollWidth - el.clientWidth;
    if (exceso <= 1 || !visible(el)) continue;
    const cs = getComputedStyle(el);
    // `visible` no recorta ni scrollea: el desbordamiento lo hereda el ancestro,
    // que ya se reporta por su cuenta. Evita duplicar la misma causa N veces.
    if (el !== raiz && cs.overflowX === "visible") continue;

    // Quién se sale: el descendiente que más rebasa el borde derecho del contenedor.
    //
    // Lo que cuelga de un scroller INTERMEDIO no cuenta: eso lo recorta el
    // scroller y no llega hasta este borde, aunque su rectángulo diga que sí. La
    // página de Sytue PDF es un SVG de 4964px dentro de su visor y se llevaba la
    // culpa de TODO lo que desbordara por encima —medido en producción: el
    // informe señalaba el SVG cuando quien sacaba la página era la barra de
    // herramientas—, así que la línea del informe mandaba al fichero equivocado.
    const caja = el.getBoundingClientRect();
    const borde = caja.left + el.clientWidth + el.scrollLeft;
    const tapadoAntesDeLlegar = (d) => {
      for (let n = d.parentElement; n && n !== el; n = n.parentElement)
        if (getComputedStyle(n).overflowX !== "visible") return true;
      return false;
    };
    let peor = null;
    let peorEl = null;
    // Respaldo para el caso «culpable fantasma»: un contenedor con
    // scrollWidth > clientWidth y NINGÚN descendiente visible que rebase. Lo que
    // infla el scrollWidth ahí es una caja INVISIBLE —visibility/opacity 0, como
    // las barras de acciones al hover— que el escaneo normal se salta. Nombrarla
    // es la única forma de que el informe mande a un fichero y no a un «?»: así
    // se descubrió que la lista de Mail desbordaba +13/+53px sin señalarse a
    // nadie (runs 2120 y 2138), y sin nombre no hubo forma de arreglarlo.
    let peorOculto = null;
    for (const d of el.querySelectorAll("*")) {
      const r = d.getBoundingClientRect();
      if (r.width < 1 || r.height < 1) continue;
      const fuera = Math.round(r.right - borde);
      if (fuera > 1 && !tapadoAntesDeLlegar(d)) {
        const cand = {
          sel: ruta(d),
          fuera,
          w: Math.round(r.width),
          txt: (d.textContent || "").trim().slice(0, 40),
        };
        if (visible(d)) {
          if (!peor || fuera > peor.fuera) {
            peorEl = d;
            peor = cand;
          }
        } else if (!peorOculto || fuera > peorOculto.fuera) {
          peorOculto = { ...cand, oculto: true };
        }
      }
    }
    if (!peor && peorOculto) peor = peorOculto;
    hallazgos.push({
      sel: el === raiz ? ":root" : ruta(el),
      exceso,
      // overflow-x:hidden no da scroll: recorta. El usuario NO puede llegar a eso.
      // `clip` tampoco: la hoja de la forma «Marco» recorta así, y lo que se salía de
      // ella constaba como «scroll-lateral» cuando no había por dónde desplazar.
      modo: /hidden|clip/.test(cs.overflowX) ? "RECORTADO" : "scroll-lateral",
      legitimo: el !== raiz && deliberado(el, cs, peorEl),
      culpable: peor,
    });
  }
  hallazgos.sort((a, b) => b.exceso - a.exceso);
  return {
    vw: document.documentElement.clientWidth,
    docScroll: raiz.scrollWidth - raiz.clientWidth,
    hallazgos: hallazgos.filter((h) => !h.legitimo).slice(0, 10),
    izq: fueraIzq().slice(0, 10),
    legitimos: hallazgos.filter((h) => h.legitimo).length,
    titulo: document.title,
    error: /error|no se pudo|failed/i.test(
      document.body.innerText.slice(0, 200),
    )
      ? document.body.innerText.slice(0, 120).replace(/\s+/g, " ")
      : null,
  };
};

/**
 * Detector VERTICAL. Corre EN LA PÁGINA, igual que `DETECTOR`.
 *
 * POR QUÉ NO LO CUBRE EL DE ARRIBA. En móvil la altura no es un límite: la
 * página se desplaza y punto, así que medir el alto solo produciría ruido. En
 * ESCRITORIO sí lo es, y por una razón concreta: una ventana se puede dejar en
 * 600px de alto —era el mínimo que declaraba el shell de Wails, retirado el
 * 2026-09-23— y ahí nunca se había medido nada. Un
 * diálogo de 640px con «Guardar» abajo, fijo y sin desplazamiento propio, deja
 * ese botón fuera de la ventana sin manera de llegar a él: ni rueda, ni barra.
 * La app no está lenta ni fea, está INUTILIZABLE, y no hay atajo que lo salve.
 *
 * SOLO LO PULSABLE, y no todo el contenido. Un párrafo que se corta se lee
 * desplazando o agrandando la ventana; un botón al que no se llega no tiene
 * vuelta. Acotarlo a lo interactivo es lo que hace que el informe se pueda leer
 * y que cada línea sea algo que arreglar.
 */
export const DETECTOR_VERTICAL = () => {
  const ruta = (el) => {
    const partes = [];
    for (
      let n = el;
      n && n.nodeType === 1 && partes.length < 4;
      n = n.parentElement
    ) {
      let s = n.tagName.toLowerCase();
      if (n.id) s += `#${n.id}`;
      else if (typeof n.className === "string" && n.className.trim())
        s += `.${n.className.trim().split(/\s+/).slice(0, 3).join(".")}`;
      partes.unshift(s);
    }
    return partes.join(" > ");
  };

  /**
   * ¿Se puede traer a la vista desplazando algo?
   *
   * Se sube por los ancestros buscando un desplazador vertical con recorrido de
   * verdad (`scrollHeight > clientHeight`): declarar `overflow-y:auto` sobre una
   * caja que ya cabe no salva nada. `hidden` corta la búsqueda —recorta antes de
   * que el scroll del documento pueda ayudar— y `fixed` también: lo fijo no se
   * mueve con la página, que es justo lo que hace peligrosos a los diálogos.
   */
  const alcanzable = (el) => {
    const caja = el.getBoundingClientRect();
    for (let n = el; n; n = n.parentElement) {
      const cs = getComputedStyle(n);
      if (n !== el) {
        if (
          /(auto|scroll)/.test(cs.overflowY) &&
          n.scrollHeight - n.clientHeight > 1
        )
          return true;
        // `hidden` SOLO estorba si de verdad recorta a ESTE elemento. Preguntar
        // por el estilo y no por la geometría convertía en «inalcanzable» todo lo
        // que quedara bajo el pliegue dentro de una `<Card class="overflow-hidden">`
        // —que en esta suite se pone para que el contenido no pise las esquinas
        // redondeadas, y no recorta nada—. Medido contra producción: la portada del
        // portal salía con diez pulsables perdidos y no le pasaba absolutamente nada.
        if (cs.overflowY === "hidden") {
          const c = n.getBoundingClientRect();
          if (caja.bottom > c.bottom + 1 || caja.top < c.top - 1) return false;
        }
      }
      if (cs.position === "fixed") return false;
    }
    const raiz = document.scrollingElement || document.documentElement;
    return raiz.scrollHeight - raiz.clientHeight > 1;
  };

  const vh = document.documentElement.clientHeight;
  const fuera = [];
  const PULSABLE =
    "button,a[href],input:not([type=hidden]),select,textarea,[role=button]," +
    '[role=menuitem],[role=option],[role=tab],[role=switch],[tabindex]:not([tabindex="-1"])';
  for (const el of document.querySelectorAll(PULSABLE)) {
    const r = el.getBoundingClientRect();
    // Nada que perder: adornos de 2px, medidores y controles apilados fuera de
    // pantalla a propósito (`sr-only`, menús cerrados, pestañas inactivas).
    if (r.width < 8 || r.height < 8) continue;
    const cs = getComputedStyle(el);
    if (cs.visibility === "hidden" || +cs.opacity === 0) continue;
    if (el.closest("[aria-hidden=true],[inert],[hidden]")) continue;
    if (el.disabled) continue;
    // Se cuenta cuando se sale de VERDAD, no por un píxel de redondeo ni por
    // asomar medio borde: 4px es menos que cualquier control real.
    const por = r.top < 0 ? Math.round(-r.top) : Math.round(r.bottom - vh);
    if (por <= 4) continue;
    if (alcanzable(el)) continue;
    if (fuera.some((f) => f.el.contains(el))) continue;
    fuera.push({
      el,
      sel: ruta(el),
      por,
      borde: r.top < 0 ? "ARRIBA" : "ABAJO",
      txt: (el.getAttribute("aria-label") || el.textContent || "")
        .trim()
        .slice(0, 40),
    });
  }
  return {
    vh,
    fuera: fuera.map(({ el: _el, ...resto }) => resto).slice(0, 10),
    titulo: document.title,
  };
};

/**
 * Corre EN LA PÁGINA. ¿Se desplaza el DOCUMENTO por debajo del armazón?
 *
 * En una pantalla de armazón de alto fijo (`AppShell`, `AdminShell`, Rooms, el ERP: la
 * raíz lleva `sytue-frame`) lo que desplaza es su `main`; el documento mide la ventana y
 * no se mueve. Si se mueve, algo se ha escapado de su panel: lo típico, un `absolute`
 * sin ancestro posicionado —un `sr-only`— que se ancla al documento en el sitio que
 * ocuparía sin desplazar y lo alarga. En la v3.11.0 eran 315 px en Sytue Admin y 674 en
 * Mi cuenta: en la forma «Marco» asomaba el fondo de la página bajo el marco, con las
 * tarjetas cortadas por el borde de la hoja. El test del armazón
 * (`armazon-no-desborda.test.tsx`) vigila la condición en dos componentes; esto mide el
 * resultado, en cada pantalla que el barrido ya visita y con datos de verdad.
 *
 * En una pantalla de PÁGINA (el Hub, Docs: `sytue-frame-page`) el documento sí desplaza,
 * y lo que se exige es que el marco llegue hasta su final: nada del fondo de la página
 * por debajo. Solo en la forma «Marco»; en «Carril» ese envoltorio no es una caja.
 *
 * Una pantalla sin armazón (una página pública, el acceso) no se juzga.
 */
export const DETECTOR_DOCUMENTO = () => {
  const d = document.documentElement;
  const sobra = d.scrollHeight - d.clientHeight;
  const marco = document.querySelector(".sytue-frame");
  if (!marco) return { tipo: "libre", sobra, falla: false };
  if (marco.classList.contains("sytue-frame-page")) {
    if (getComputedStyle(marco).display === "contents")
      return { tipo: "pagina", sobra, falla: false };
    const falta = Math.round(
      d.scrollHeight - (marco.getBoundingClientRect().bottom + scrollY),
    );
    return { tipo: "pagina", sobra, falta, falla: falta > 4 };
  }
  // 4px: ni un redondeo ni medio borde alargan el documento.
  if (sobra <= 4) return { tipo: "fijo", sobra, falla: false };

  // El culpable: lo que queda más abajo de la ventana SIN que ningún panel lo recorte.
  // Un `absolute` salta los ancestros no posicionados: su bloque contenedor está más
  // arriba, así que un `overflow` por el camino no lo recorta.
  const recortado = (el) => {
    const pos = getComputedStyle(el).position;
    if (pos === "fixed") return true;
    let escapa = pos === "absolute";
    for (
      let c = el.parentElement;
      c && c !== document.body;
      c = c.parentElement
    ) {
      const cs = getComputedStyle(c);
      if (escapa && cs.position === "static") continue;
      escapa = false;
      if (/auto|scroll|hidden|clip/.test(cs.overflowY)) return true;
      if (cs.position === "fixed") return true;
      if (cs.position === "absolute") escapa = true;
    }
    return false;
  };
  let peor = null;
  for (const el of document.querySelectorAll("body *")) {
    const r = el.getBoundingClientRect();
    if (r.height <= 0 || r.bottom <= d.clientHeight + 4) continue;
    if (peor && r.bottom <= peor.fin) continue;
    if (recortado(el)) continue;
    peor = { el, fin: r.bottom };
  }
  let culpable = "?";
  if (peor) {
    const el = peor.el;
    const clases =
      typeof el.className === "string" && el.className.trim()
        ? `.${el.className.trim().split(/\s+/).slice(0, 3).join(".")}`
        : "";
    culpable = `${el.tagName.toLowerCase()}${el.id ? `#${el.id}` : clases} (${getComputedStyle(el).position})`;
  }
  return { tipo: "fijo", sobra, culpable, falla: true };
};

/**
 * Corre EN LA PÁGINA. La CONDICIÓN de la que sale el desbordamiento de arriba, haya o no
 * datos para que se note: un `absolute` sin anclar en vertical —un `sr-only`— dentro de
 * un panel que desplaza y con el bloque contenedor FUERA de ese panel.
 *
 * Ese elemento se queda donde le tocaría en el flujo, pero medido desde un ancestro que
 * no desplaza: no sigue al contenido y, en cuanto la lista es más larga que la ventana,
 * alarga lo que tenga encima. `DETECTOR_DOCUMENTO` solo lo ve cuando llega al documento
 * y hay filas bastantes: en la v3.11.1 el buzón de la cuenta de auditoría alargaba Mail
 * 1113 px, y un buzón de diez correos habría salido en verde con el mismo defecto. Y
 * posicionar el `main` del armazón (#492) no lo arregla si el que desplaza es un panel
 * de dentro: el sobrante deja el documento y pasa al `main`, que gana un segundo
 * desplazamiento sin nada que enseñar.
 *
 * Lo anclado a propósito (`top`/`bottom` puestos: un botón flotante sobre la lista) no
 * cuenta: ese está donde se le dijo. `computedStyleMap` y no `getComputedStyle`, que de
 * un posicionado devuelve el valor USADO en píxeles aunque la hoja diga `auto`.
 *
 * ACUMULA en la página: el barrido la llama también con cada desplegable, pestaña y
 * diálogo abiertos —un diálogo con una lista larga de casillas es el mismo caso—, y lo
 * que devuelve es todo lo visto desde que se cargó. Una navegación lo pone a cero.
 *
 * Se arregla en el panel: `relative` en el que desplaza.
 */
export const DETECTOR_FUGAS = () => {
  const desplaza = (cs) =>
    /auto|scroll/.test(`${cs.overflowY} ${cs.overflowX}`);
  // Lo que hace de bloque contenedor de un `absolute` sin estar posicionado.
  const contiene = (cs) =>
    cs.position !== "static" ||
    cs.transform !== "none" ||
    cs.filter !== "none" ||
    cs.containerType !== "normal" ||
    /layout|paint|strict|content/.test(cs.contain);
  const sinAnclar = (el) => {
    const m = el.computedStyleMap();
    return m.get("top").value === "auto" && m.get("bottom").value === "auto";
  };
  const nombre = (el) => {
    const clases =
      typeof el.className === "string" && el.className.trim()
        ? `.${el.className.trim().split(/\s+/).slice(0, 3).join(".")}`
        : "";
    return `${el.tagName.toLowerCase()}${el.id ? `#${el.id}` : clases}`;
  };
  const vistas = (globalThis.__sytueFugas ??= new Map());
  const fugas = new Map();
  for (const el of document.querySelectorAll("body *")) {
    if (getComputedStyle(el).position !== "absolute" || !sinAnclar(el))
      continue;
    for (
      let c = el.parentElement;
      c && c !== document.body;
      c = c.parentElement
    ) {
      const cs = getComputedStyle(c);
      if (contiene(cs)) break;
      if (!desplaza(cs)) continue;
      const f = fugas.get(c) ?? { panel: nombre(c), fugado: nombre(el), n: 0 };
      f.n++;
      fugas.set(c, f);
      break;
    }
  }
  // Por nombre y no por nodo: el panel de un diálogo se desmonta al cerrarlo.
  for (const f of fugas.values()) {
    const clave = `${f.fugado} ${f.panel}`;
    if ((vistas.get(clave)?.n ?? 0) < f.n) vistas.set(clave, f);
  }
  return [...vistas.values()];
};

/** Una línea legible por combinación medida: lo que se escapa de su panel. */
export const lineaFugas = (fugas) =>
  fugas?.length
    ? ` FUGA: ${fugas.map((f) => `${f.n}× ${f.fugado} fuera de ${f.panel}`).join("; ")}`
    : "";

/** Una línea legible por combinación medida: el documento y su armazón. */
export const lineaDocumento = (r) => {
  if (!r?.falla) return "";
  return r.tipo === "pagina"
    ? ` MARCO a ${r.falta}px del final del documento`
    : ` DOCUMENTO +${r.sobra}px bajo el armazón ← ${r.culpable}`;
};

/** Una línea legible por combinación medida, para el barrido de escritorio. */
export const lineaVertical = (r) => {
  const f = r.fuera[0];
  if (!f) return "";
  return (
    `−${f.por}px POR ${f.borde} (inalcanzable) en ${f.sel.split(" > ").pop()} «${f.txt}»` +
    `${r.fuera.length > 1 ? ` +${r.fuera.length - 1} más` : ""}`
  );
};

/** Añade a `dest` lo que no esté ya (mismo selector). Un mismo desbordamiento
 *  aparece en la vista en reposo y otra vez con cada menú abierto.
 *
 *  Y si el de reposo vino SIN culpable («?») y el de menú abierto lo nombra, se
 *  queda el nombre: en reposo el culpable puede ser invisible para el escaneo y
 *  el mismo contenedor, ya con el desplegable caído, sí señala quién es. Sin
 *  esto el informe guardaba el primero y tiraba el dato que identificaba al
 *  culpable — mail llevaba dos días diciendo «← ? «»» por exactamente esto. */
export const unir = (dest, extra) => {
  for (const e of extra) {
    const d = dest.find((x) => x.sel === e.sel);
    if (!d) dest.push(e);
    else if (!d.culpable && e.culpable) d.culpable = e.culpable;
  }
};

/** Una línea legible por combinación medida, igual en las dos puertas. */
export const linea = (r) => {
  const h = r.hallazgos[0];
  const i = r.izq[0];
  return (
    `${h ? `+${h.exceso}px ${h.modo} en ${h.sel.split(" > ").pop()} ← ${h.culpable?.sel.split(" > ").pop() ?? "?"}${h.culpable?.oculto ? " (oculto)" : ""} «${h.culpable?.txt ?? ""}»` : ""}` +
    `${i ? ` −${i.fuera}px POR LA IZQUIERDA (inalcanzable) en ${i.sel.split(" > ").pop()} «${i.txt}»${r.izq.length > 1 ? ` +${r.izq.length - 1} más` : ""}` : ""}`
  );
};

/* ---------------------------------------------------------------------------
 * Comprobación del DETECTOR, no de la suite: `bun run detector-desbordamiento.mjs`.
 *
 * Página sintética, sin servidor, sin sesión y sin producción. Existe porque el
 * detector es lo único que sostiene las tres puertas de layout, y una excusa mal
 * puesta aquí no rompe nada: pone todo en verde. Así se encontró que una tabla
 * cualquiera en otro rincón del panel borraba del informe un desbordamiento real.
 *
 * No es un `.spec.ts` de `tests/`: aquellos exigen la pila local en pie, y esto
 * no necesita más que el navegador.
 */
if (import.meta.main) {
  const { chromium } = await import("playwright");
  let fallosV = 0;
  const navegador = await chromium.launch();
  const page = await navegador.newPage({
    viewport: { width: 320, height: 600 },
  });
  // `<!doctype html>` NO es adorno. Sin él, `setContent` monta el documento en
  // QUIRKS MODE, y ahí `documentElement.clientWidth/Height` devuelven el tamaño
  // del DOCUMENTO en vez del de la ventana. Medido: con una página de 1421px de
  // alto en una ventana de 800, `clientHeight` daba 1421, así que nada caía
  // nunca fuera y los casos que esperaban «excusado» pasaban sin comprobar nada.
  // Una página real siempre lo lleva; el banco de pruebas era el que mentía.
  const DOCTYPE = "<!doctype html>";
  const CSS =
    DOCTYPE +
    "<style>body{margin:0}#panel{width:320px;overflow-x:auto}#ancho{width:900px;height:40px}" +
    "table{width:900px}</style>";
  const casos = [
    [
      "desbordamiento real",
      "<main id=panel><div id=ancho>ancho</div></main>",
      true,
    ],
    [
      "desbordamiento real CON una tabla ajena al lado",
      "<main id=panel><div id=ancho>ancho</div><table><tr><td>ajena</td></tr></table></main>",
      true,
    ],
    [
      "la tabla ES quien desborda (legítimo)",
      "<main id=panel><table><tr><td>hoja de cálculo</td></tr></table></main>",
      false,
    ],
    [
      "desplazamiento declarado con data-scroll-x (legítimo)",
      "<main id=panel data-scroll-x><div id=ancho>cinta</div></main>",
      false,
    ],
    [
      // El editor de hojas: entre el scroller y el `role="grid"` hay un div que
      // aplica el zoom, y era ÉL quien salía como culpable.
      "rejilla envuelta en un div de zoom (legítimo)",
      "<main id=panel><div style='width:900px'><div role=grid style='width:900px'>hoja</div></div></main>",
      false,
    ],
    [
      // Y el contrario, que es el que se rompió una vez: algo ancho por su cuenta
      // con una rejilla pequeña dentro NO se excusa.
      "envoltorio ancho con una rejilla pequeña dentro (defecto real)",
      "<main id=panel><div id=ancho><div role=grid style='width:80px'>mini</div></div></main>",
      true,
    ],
    [
      // EL CULPABLE FANTASMA de la lista de Mail (+13/+53px, runs 2120 y 2138):
      // una caja INVISIBLE —visibility/opacity 0, como las barras de acciones al
      // hover— infla el scrollWidth sin que ningún descendiente visible rebasa.
      // Antes el informe decía «← ? «»» y no había forma de saber a qué fichero
      // ir; ahora se nombra y se marca «(oculto)».
      "culpable invisible: se reporta Y se nombra",
      "<main id=panel><div id=fantasma style='visibility:hidden;width:900px;height:40px'>oculto</div></main>",
      true,
    ],
  ];
  let fallos = 0;
  for (const [nombre, cuerpo, esperado] of casos) {
    await page.setContent(CSS + cuerpo);
    const r = await page.evaluate(DETECTOR);
    const reportado = r.hallazgos.some((h) => h.sel.includes("#panel"));
    const ok = reportado === esperado;
    if (!ok) fallos++;
    console.log(
      `${ok ? "  ok  " : "FALLO "} ${nombre}: ${reportado ? "REPORTADO" : "excusado"}` +
        ` (esperado ${esperado ? "REPORTADO" : "excusado"}) ${linea(r)}`,
    );
  }

  /* ---- DETECTOR_VERTICAL: la ventana mínima de escritorio, 880×600 ----
   *
   * Las dos direcciones cuestan lo mismo. De más, se pone en rojo una app sana y
   * la puerta se desactiva; de menos, se aprueba un «Guardar» al que no se llega
   * con el ratón, que es el defecto que esto viene a cazar.
   */
  const V = await navegador.newPage({ viewport: { width: 880, height: 600 } });
  const casosV = [
    [
      "diálogo fijo más alto que la ventana: el botón de abajo no se alcanza",
      "<div style='position:fixed;top:0;left:0;width:400px;height:900px;background:#eee'>" +
        "<button style='margin-top:820px'>Guardar</button></div>",
      true,
    ],
    [
      "el mismo diálogo CON desplazamiento propio: se llega",
      "<div style='position:fixed;top:0;left:0;width:400px;height:560px;overflow-y:auto'>" +
        "<div style='height:900px'></div><button>Guardar</button></div>",
      false,
    ],
    [
      "contenido largo en la página: se llega desplazando, no es defecto",
      "<div style='height:1400px'></div><button>Guardar</button>",
      false,
    ],
    [
      // EL CASO QUE DESTAPÓ LA PORTADA DEL PORTAL: una tarjeta con
      // `overflow-hidden` puesto para las esquinas redondeadas, que no recorta
      // nada porque su contenido cabe dentro. Si eso cuenta como barrera, todo
      // lo que haya debajo del pliegue se reporta inalcanzable y la puerta se
      // llena de ruido justo en las páginas mejor construidas.
      "tarjeta con overflow-hidden que NO recorta: se llega desplazando",
      "<div style='height:1400px'></div>" +
        "<div style='overflow:hidden;border-radius:8px'><button>Guardar</button></div>",
      false,
    ],
    [
      "recortado por overflow-y:hidden: no hay scroll que valga",
      "<div style='height:560px;overflow-y:hidden'><div style='height:900px'></div>" +
        "<button>Guardar</button></div>",
      true,
    ],
  ];
  for (const [nombre, cuerpo, esperado] of casosV) {
    await V.setContent(DOCTYPE + "<style>body{margin:0}</style>" + cuerpo);
    const r = await V.evaluate(DETECTOR_VERTICAL);
    const reportado = r.fuera.some((f) => /button/.test(f.sel));
    const ok = reportado === esperado;
    if (!ok) fallosV++;
    console.log(
      `${ok ? "  ok  " : "FALLO "} ${nombre}: ${reportado ? "REPORTADO" : "excusado"}` +
        ` (esperado ${esperado ? "REPORTADO" : "excusado"}) ${lineaVertical(r)}`,
    );
  }

  /* ---- DETECTOR_FUGAS: un `absolute` sin anclar dentro de un panel que desplaza ----
   *
   * De menos, vuelve el documento alargado bajo el armazón en cuanto un buzón tiene
   * correos bastantes. De más, se manda posicionar un panel cuyo botón flotante está
   * anclado a propósito a su marco de fuera, y al hacerlo se va con el contenido.
   */
  let fallosF = 0;
  const SR = "position:absolute;width:1px;height:1px;overflow:hidden";
  const panel = (estilo, dentro) =>
    `<div id=panel style='height:200px;overflow-y:auto;${estilo}'>` +
    `<div style='height:900px'></div>${dentro}</div>`;
  const casosF = [
    [
      "sr-only en un panel que desplaza sin posicionar: se escapa",
      panel("", `<span style='${SR}'>no leído</span>`),
      true,
    ],
    [
      "el mismo panel con position:relative: se queda dentro",
      panel("position:relative", `<span style='${SR}'>no leído</span>`),
      false,
    ],
    [
      "con un envoltorio posicionado entre medias: se queda en él",
      panel(
        "",
        `<div style='position:relative'><span style='${SR}'>no leído</span></div>`,
      ),
      false,
    ],
    [
      "anclado a propósito (top puesto) al marco de fuera: no cuenta",
      panel("", `<button style='position:absolute;top:8px'>Bajar</button>`),
      false,
    ],
    [
      "panel que es contenedor de consultas (container-type): ya contiene",
      panel(
        "container-type:inline-size",
        `<span style='${SR}'>no leído</span>`,
      ),
      false,
    ],
  ];
  for (const [nombre, cuerpo, esperado] of casosF) {
    // El detector ACUMULA en la página, y `setContent` conserva el global: se
    // navega antes para que cada caso empiece sin lo que vio el anterior.
    await V.goto("about:blank");
    await V.setContent(DOCTYPE + "<style>body{margin:0}</style>" + cuerpo);
    const fugas = await V.evaluate(DETECTOR_FUGAS);
    const reportado = fugas.some((f) => f.panel === "div#panel");
    const ok = reportado === esperado;
    if (!ok) fallosF++;
    console.log(
      `${ok ? "  ok  " : "FALLO "} ${nombre}: ${reportado ? "REPORTADO" : "excusado"}` +
        ` (esperado ${esperado ? "REPORTADO" : "excusado"})${lineaFugas(fugas)}`,
    );
  }

  await navegador.close();
  process.exit(fallos + fallosV + fallosF ? 1 : 0);
}
