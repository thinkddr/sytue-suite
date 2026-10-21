// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// Detector de CONTROLES PEGADOS AL BORDE.
//
// Hermano del de desbordamiento y con la misma forma —se serializa y corre EN LA
// PÁGINA, así que no puede referirse a nada de fuera— para poder enchufarlo en
// el mismo barrido sin duplicar las novecientas líneas que lo mueven.
//
// QUÉ MIDE Y POR QUÉ ASÍ. La distancia de cada control al borde VISIBLE de la
// caja que lo contiene, no a su caja de contenido. Un botón a lo ancho dentro de
// una tarjeta con `p-4` toca su caja de contenido —está bien, el aire lo pone la
// tarjeta— pero queda a 16px del borde que se ve. Midiendo contra la caja de
// contenido, ese botón correcto saldría como hallazgo y el informe sería ruido.
//
// ponytail: un consumidor (mobile-audit). Si aparece un segundo, ya está listo.

/**
 * Aire mínimo, en píxeles CSS.
 *
 * 10 CONTRA LA VENTANA, Y EL NÚMERO SALE DE MEDIR, no de redondear. El anillo de
 * foco de la casa son 2px dibujados FUERA de la caja y sin desplazamiento
 * (`.sytue-focus-ring`: `box-shadow: 0 0 0 2px`), así que por debajo de ~10px
 * empieza a leerse pegado al cristal.
 *
 * Estuvo en 12 y era demasiado: sacaba 27 veces la hamburguesa de Rooms y el
 * selector de idioma de www, los dos a 10,5px por un `-ml-1` DELIBERADO —el
 * truco de siempre para que el icono, y no su caja, caiga en la línea del texto
 * de al lado—. Un detector que acusa a la alineación óptica no se usa dos veces.
 *
 * Contra una tarjeta bastan 8: el marco ya separa del resto de la página.
 */
export const AIRE_VENTANA = 10;
export const AIRE_MARCO = 8;

export const DETECTOR_MARGENES = () => {
  const AIRE_VENTANA = 10;
  const AIRE_MARCO = 8;

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
    if (
      cs.visibility === "hidden" ||
      +cs.opacity === 0 ||
      cs.display === "none"
    )
      return false;
    const r = el.getBoundingClientRect();
    return r.width > 8 && r.height > 8 && r.bottom > 0 && r.top < innerHeight;
  };

  /**
   * ¿Esta caja ENMARCA lo que lleva dentro? Solo entonces tiene sentido hablar
   * de «pegado a su borde»: un `div` de maquetación sin fondo ni trazo no tiene
   * borde que ver, y medir contra él inventaría hallazgos que nadie percibe.
   *
   * El sistema de diseño redondea tarjetas y diálogos, así que el radio es un
   * buen indicio; el `role=dialog` entra por su cuenta porque un modal a pantalla
   * completa puede no llevarlo.
   */
  const enmarca = (el) => {
    if (el.matches("dialog,[role=dialog],[role=alertdialog]")) return true;
    const cs = getComputedStyle(el);
    const radio = parseFloat(cs.borderTopLeftRadius) || 0;
    const conTrazo =
      parseFloat(cs.borderTopWidth) > 0 || parseFloat(cs.borderLeftWidth) > 0;
    const conFondo =
      cs.backgroundColor !== "rgba(0, 0, 0, 0)" &&
      cs.backgroundColor !== "transparent";
    return radio >= 4 && (conTrazo || conFondo);
  };

  /**
   * Pegado A PROPÓSITO, y no es un defecto aunque toque el borde.
   *
   * Lo que ocupa la caja entera —una imagen de portada, un mapa, un lienzo, una
   * fila de lista con su propio resalte— está flush por diseño. Se reconoce
   * porque cubre casi todo el ancho Y todo el alto de quien lo enmarca: un botón
   * nunca hace eso.
   */
  /**
   * LA TINTA QUE SE VE, no la caja.
   *
   * Un botón fantasma —sin fondo ni trazo— no se percibe por su caja sino por su
   * icono o su rótulo, que van metidos por su propio relleno. Midiendo la caja,
   * un icono con `p-2` a 7px del cristal salía como hallazgo cuando lo que se ve
   * está a 15px y está bien. Con fondo o trazo, en cambio, la caja ES lo que se
   * ve y se mide tal cual.
   */
  const tinta = (el) => {
    const cs = getComputedStyle(el);
    const conFondo =
      cs.backgroundColor !== "rgba(0, 0, 0, 0)" &&
      cs.backgroundColor !== "transparent";
    const conTrazo =
      parseFloat(cs.borderTopWidth) > 0 || parseFloat(cs.outlineWidth) > 2;
    if (conFondo || conTrazo) return { izq: 0, der: 0, arr: 0, aba: 0 };
    return {
      izq: parseFloat(cs.paddingLeft) || 0,
      der: parseFloat(cs.paddingRight) || 0,
      arr: parseFloat(cs.paddingTop) || 0,
      aba: parseFloat(cs.paddingBottom) || 0,
    };
  };

  const deliberado = (el, marco) => {
    const r = el.getBoundingClientRect();
    const m = marco.getBoundingClientRect();
    if (r.width >= m.width - 1 && r.height >= m.height - 1) return true;
    // RELLENA SU MARCO: entonces el aire lo pone el marco, no el control, y
    // juzgarlo aquí acusa al botón de lo que decide su contenedor. Es el caso de
    // toda opción de menú y de toda pastilla de un segmentado: `w-full` dentro de
    // un panel con `p-1`. El relleno del marco se juzga aparte, más abajo.
    const cm = getComputedStyle(marco);
    const dentro =
      m.width -
      (parseFloat(cm.paddingLeft) || 0) -
      (parseFloat(cm.paddingRight) || 0);
    if (r.width >= dentro - 1) return true;
    if (el.matches("img,canvas,video,svg,iframe,hr,[data-flush]")) return true;
    // Una barra de progreso o un separador pegados al borde son la convención.
    const rol = el.getAttribute("role");
    return rol === "progressbar" || rol === "separator";
  };

  const CONTROLES =
    "button,[role=button],[role=menuitem],[role=tab],a[href],input,select,textarea,[contenteditable=true]";

  /**
   * Quién pone (o no pone) el aire: el primer ancestro con relleno horizontal,
   * o la cadena entera si ninguno lo tiene.
   *
   * Sin esto el informe dice QUÉ está apretado pero no DÓNDE se arregla, y cada
   * hallazgo obliga a abrir el navegador otra vez. Con la cadena, el arreglo se
   * decide leyendo la tabla.
   */
  const culpa = (el) => {
    for (
      let n = el.parentElement, i = 0;
      n && n !== document.body && i < 5;
      n = n.parentElement, i++
    ) {
      const cs = getComputedStyle(n);
      const pl = parseFloat(cs.paddingLeft) || 0;
      const pr = parseFloat(cs.paddingRight) || 0;
      if (pl > 0 || pr > 0) {
        return `${n.tagName.toLowerCase()}${typeof n.className === "string" && n.className.trim() ? "." + n.className.trim().split(/\s+/).slice(0, 3).join(".") : ""} px=${pl}/${pr}`;
      }
    }
    return "(nadie pone relleno)";
  };

  const hallazgos = [];
  const vistos = new Set();

  for (const el of document.querySelectorAll(CONTROLES)) {
    if (!visible(el)) continue;
    const r = el.getBoundingClientRect();

    // 1) Contra el borde de la caja que lo enmarca (tarjeta, diálogo, panel).
    let marco = el.parentElement;
    while (marco && marco !== document.body && !enmarca(marco))
      marco = marco.parentElement;
    if (marco && marco !== document.body && !deliberado(el, marco)) {
      const m = marco.getBoundingClientRect();
      const lados = {
        izquierda: r.left - m.left,
        derecha: m.right - r.right,
        arriba: r.top - m.top,
        abajo: m.bottom - r.bottom,
      };
      // EL CONTROL ESTÁ DONDE SU MARCO LO PONE.
      //
      // Si el hueco coincide con el relleno del marco por ese lado, el control
      // no se ha ido a ningún sitio: está exactamente donde el contenedor lo
      // coloca, y quien decide el aire es el marco. Acusar al botón aquí llena
      // el informe de lo que NO es un defecto: una pastilla de segmentado a 2px
      // de su carril (85 veces en el barrido), una opción de menú a 5px de su
      // panel (54), una reacción a 3px de su fila. Los tres son la convención de
      // su control, y los tres desaparecen con esta línea.
      //
      // Lo que sigue saliendo es el control que se sale de lo que su marco
      // previó —o el que no tiene marco y va contra el cristal—, que es de lo
      // que se quejaba la gente.
      const cm = getComputedStyle(marco);
      const relleno = {
        izquierda: parseFloat(cm.paddingLeft) || 0,
        derecha: parseFloat(cm.paddingRight) || 0,
        arriba: parseFloat(cm.paddingTop) || 0,
        abajo: parseFloat(cm.paddingBottom) || 0,
      };
      for (const lado of Object.keys(lados)) {
        if (Math.abs(lados[lado] - relleno[lado]) < 1.5) lados[lado] = Infinity;
      }
      const t = tinta(el);
      lados.izquierda += t.izq;
      lados.derecha += t.der;
      lados.arriba += t.arr;
      lados.abajo += t.aba;
      for (const [lado, d] of Object.entries(lados)) {
        // Negativo = se sale, y de eso ya informa el otro detector.
        if (d >= 0 && d < AIRE_MARCO) {
          const clave = `${ruta(el)}|${lado}`;
          if (vistos.has(clave)) continue;
          vistos.add(clave);
          hallazgos.push({
            sel: clave,
            tipo: "marco",
            lado,
            aire: Math.round(d),
            control: ruta(el),
            texto: (el.textContent || el.getAttribute("aria-label") || "")
              .trim()
              .slice(0, 40),
            marco: ruta(marco),
            culpa: culpa(el),
          });
        }
      }
    }

    // 2) Contra el borde de la VENTANA. Lo que se pega al cristal se ve aunque
    //    no haya tarjeta alrededor, y es lo que más canta en el teléfono.
    const t = tinta(el);
    const aLosLados = Math.min(r.left + t.izq, innerWidth - r.right + t.der);
    const anchoCasiTotal = r.width >= innerWidth * 0.92;
    // TESELA DE UNA REJILLA A SANGRE: no es un control mal colocado, es una
    // casilla. La rejilla del mes va de borde a borde —como en cualquier
    // calendario— y su primera columna empieza en 0 por definición; el barrido
    // la sacaba nueve veces como «0px por la izquierda: «31»».
    const padre = el.parentElement;
    const enRejillaASangre =
      !!padre &&
      padre.getBoundingClientRect().width >= innerWidth * 0.92 &&
      (getComputedStyle(padre).display.includes("grid") ||
        !!el.closest("table,[role=grid],[role=row],[role=table]"));
    if (
      aLosLados >= 0 &&
      aLosLados < AIRE_VENTANA &&
      !anchoCasiTotal &&
      !enRejillaASangre
    ) {
      const clave = `${ruta(el)}|ventana`;
      if (!vistos.has(clave)) {
        vistos.add(clave);
        hallazgos.push({
          sel: clave,
          tipo: "ventana",
          lado: r.left < innerWidth - r.right ? "izquierda" : "derecha",
          aire: Math.round(aLosLados),
          control: ruta(el),
          texto: (el.textContent || el.getAttribute("aria-label") || "")
            .trim()
            .slice(0, 40),
          marco: "(ventana)",
          culpa: culpa(el),
        });
      }
    }
  }

  hallazgos.sort((a, b) => a.aire - b.aire);
  // `izq` vacío: el barrido comparte la tubería con el detector de
  // desbordamiento, que sí lo usa. Devolverlo evita tocar su lado.
  return { hallazgos: hallazgos.slice(0, 40), izq: [] };
};

/** Misma firma que `linea(r)` del otro detector, para enchufarlo sin tocar el
 *  informe: recibe el resultado entero y resume el hallazgo más apretado. */
export const lineaMargen = (r) => {
  const h = r.hallazgos[0];
  if (!h) return "";
  const donde =
    h.tipo === "ventana" ? "la ventana" : h.marco.split(" > ").pop();
  const qué = h.texto ? `«${h.texto}»` : h.control.split(" > ").pop();
  return (
    `${h.aire}px por la ${h.lado}: ${qué} contra ${donde}` +
    `${r.hallazgos.length > 1 ? ` +${r.hallazgos.length - 1} más` : ""}`
  );
};
