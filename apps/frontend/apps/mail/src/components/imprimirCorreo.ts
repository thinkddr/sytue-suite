/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Imprimir una conversación (D-MAIL-09).
 *
 * NO SE IMPRIME LA PÁGINA. Ctrl+P sobre Mail saca el menú, la lista de
 * conversaciones y el lector en una caja con altura de pantalla, y del cuerpo de
 * cada correo solo lo que cabe en su iframe: un marco no se parte entre folios,
 * así que un boletín de tres páginas se cortaba en la primera. Gmail y Outlook
 * resuelven lo mismo con una vista de impresión aparte, y aquí se hace igual: se
 * compone un documento con el asunto y, por cada mensaje del hilo, remitente,
 * fecha, destinatarios, cuerpo y nombres de los adjuntos, y se imprime ESE
 * documento desde un marco oculto. Al ser un documento propio, el navegador lo
 * pagina entero.
 *
 * El marco va con `sandbox` sin `allow-scripts`, igual que el del lector: el HTML
 * del remitente no ejecuta nada. `allow-same-origin` hace falta para llamar a
 * `print()` desde fuera (es de los métodos que no cruzan orígenes) y para que las
 * imágenes en línea, que sirve nuestro BFF con la sesión, se carguen;
 * `allow-modals` es lo que deja abrir el diálogo de impresión dentro de un marco
 * aislado. La misma CSP del lector bloquea cualquier carga remota.
 */
import { brand } from '@sytue/config';

/** Un mensaje del hilo, ya resuelto y con los rótulos traducidos por quien llama. */
export interface MensajeParaImprimir {
  /** «De: Nombre <dirección>». */
  remitente: string;
  /** Fecha larga ya formateada en el idioma de la persona. */
  fecha: string;
  /** «Para: …», «Cc: …»: solo las que tengan destinatarios. */
  destinatarios: string[];
  /** Cuerpo HTML con los `cid:` ya resueltos; manda sobre `texto`. */
  html?: string | null | undefined;
  /** Cuerpo de texto (o el extracto, si no llegó el cuerpo). */
  texto?: string | null | undefined;
  /** Imágenes del propio mensaje que el texto no cita (fotos del iPhone). */
  imagenes?: readonly { src: string; alt: string }[] | undefined;
  /** Título de la lista de adjuntos («Adjuntos · 2») y sus nombres. */
  adjuntos?: { titulo: string; nombres: readonly string[] } | null | undefined;
}

export interface ConversacionParaImprimir {
  /** `lang` del documento: corta palabras y elige comillas como el resto de la suite. */
  idioma: string;
  asunto: string;
  /** Antiguo → reciente, como se lee en el lector. */
  mensajes: readonly MensajeParaImprimir[];
}

/**
 * CSP de todo documento con el HTML de un correo (el marco del lector, `HtmlBody`,
 * y el de impresión): nada remoto, solo lo nuestro y lo incrustado.
 */
export const CSP_CUERPO_CORREO =
  "default-src 'none'; img-src data: 'self'; style-src 'unsafe-inline'; font-src data:";

export function escaparHtml(s: string): string {
  return s
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

// Papel blanco y tinta oscura SIEMPRE, también con la suite en tema oscuro: un
// documento aislado no hereda los tokens de la página (`srcdoc`), y en papel el
// tema de la pantalla no pinta nada. Por eso salen de la paleta de la suite en
// valor (no de `var(--sytue-…)`, que aquí no resuelve): la tinta del tema claro,
// un gris de la misma familia con contraste AA sobre blanco y el filete neutro.
const PAPEL = brand.neutral[0];
const TINTA = brand.accent[800];
const TENUE = brand.accent[500];
const FILETE = brand.neutral[300];

// Clases con prefijo `sytue-imp-`: el HTML de los correos va en el mismo
// documento y sus estilos (`.header`, `h1`…) no deben pisar la cabecera.
const ESTILO = `@page{margin:16mm}
html{color-scheme:light}
body{margin:0;background:${PAPEL};color:${TINTA};font:11pt/1.5 system-ui,-apple-system,sans-serif;overflow-wrap:anywhere}
.sytue-imp-asunto{font-size:16pt;line-height:1.3;font-weight:600;margin:0 0 4mm}
.sytue-imp-mensaje{border-top:1px solid ${FILETE};padding-top:4mm;margin-top:6mm}
.sytue-imp-cabecera{margin:0 0 4mm;break-inside:avoid;break-after:avoid-page}
.sytue-imp-cabecera p{margin:0}
.sytue-imp-remitente{font-weight:600}
.sytue-imp-meta{color:${TENUE};font-size:10pt}
.sytue-imp-texto{white-space:pre-wrap;margin:0;font:inherit}
.sytue-imp-imagen{display:block;max-width:100%;height:auto;margin-top:3mm}
.sytue-imp-adjuntos{margin-top:4mm;font-size:10pt;color:${TENUE};break-inside:avoid}
.sytue-imp-adjuntos ul{margin:1mm 0 0;padding-left:5mm}
.sytue-imp-cuerpo img{max-width:100%;height:auto}`;

/**
 * El HTML de un mensaje, cerrado sobre sí mismo antes de juntarlo con los demás.
 *
 * En el lector cada cuerpo va en su propio marco; aquí comparten documento, y un
 * correo mal formado (un `<!--` sin cerrar, un `<div>` o un `<textarea>` abiertos
 * hasta el final, su propio `<body>`) se tragaba o escondía los mensajes que
 * venían detrás. Analizarlo aparte en un `<template>` (sin scripts ni cargas) y
 * volver a serializarlo cierra todo lo que dejó abierto y descarta sus
 * `<html>`/`<body>`. `<plaintext>` es lo único que no se cierra al volver a
 * analizarlo: pasa a `<pre>` con el mismo texto.
 */
export function htmlAcotado(html: string, doc: Document = document): string {
  const plantilla = doc.createElement('template');
  plantilla.innerHTML = html;
  for (const p of Array.from(plantilla.content.querySelectorAll('plaintext'))) {
    const pre = doc.createElement('pre');
    pre.textContent = p.textContent;
    p.replaceWith(pre);
  }
  return plantilla.innerHTML;
}

function mensajeHtml(m: MensajeParaImprimir): string {
  const meta = [m.fecha, ...m.destinatarios]
    .filter(Boolean)
    .map((linea) => `<p class="sytue-imp-meta">${escaparHtml(linea)}</p>`)
    .join('');
  // El HTML del remitente es el mismo que el lector pinta en su marco, y aquí lo
  // contiene el mismo sandbox y la misma CSP; solo se acota (ver `htmlAcotado`).
  const cuerpo = m.html?.trim()
    ? htmlAcotado(m.html)
    : `<pre class="sytue-imp-texto">${escaparHtml(m.texto ?? '')}</pre>`;
  const imagenes = (m.imagenes ?? [])
    .map(
      (i) =>
        `<img class="sytue-imp-imagen" src="${escaparHtml(i.src)}" alt="${escaparHtml(i.alt)}">`,
    )
    .join('');
  const adjuntos =
    m.adjuntos && m.adjuntos.nombres.length > 0
      ? `<div class="sytue-imp-adjuntos"><p>${escaparHtml(m.adjuntos.titulo)}</p><ul>${m.adjuntos.nombres
          .map((n) => `<li>${escaparHtml(n)}</li>`)
          .join('')}</ul></div>`
      : '';
  return (
    `<article class="sytue-imp-mensaje">` +
    `<header class="sytue-imp-cabecera"><p class="sytue-imp-remitente">${escaparHtml(m.remitente)}</p>${meta}</header>` +
    `<div class="sytue-imp-cuerpo">${cuerpo}</div>${imagenes}${adjuntos}</article>`
  );
}

/** El documento que se imprime: asunto y, debajo, cada mensaje del hilo. */
export function documentoDeImpresion(c: ConversacionParaImprimir): string {
  const asunto = escaparHtml(c.asunto);
  return `<!doctype html><html lang="${escaparHtml(c.idioma)}"><head><meta charset="utf-8">
<meta http-equiv="Content-Security-Policy" content="${CSP_CUERPO_CORREO}">
<base target="_blank">
<title>${asunto}</title>
<style>${ESTILO}</style>
</head><body><h1 class="sytue-imp-asunto">${asunto}</h1>${c.mensajes.map(mensajeHtml).join('')}</body></html>`;
}

/** Marca del marco de impresión: uno a la vez, el anterior se retira al pedir otro. */
export const ATRIBUTO_MARCO = 'data-impresion-correo';

/** Cuánto se espera a que el documento (y sus imágenes) cargue antes de rendirse. */
const ESPERA_CARGA_MS = 20_000;

/**
 * Imprime `html` desde un marco oculto. Se resuelve cuando se ha abierto el
 * diálogo de impresión; falla si el documento no llega a cargar o el navegador
 * no deja imprimir el marco.
 *
 * El marco se queda montado hasta `afterprint`: en Safari `print()` no bloquea,
 * y quitarlo en seguida dejaba el diálogo con una página en blanco.
 */
export function imprimirEnMarco(html: string, doc: Document = document): Promise<void> {
  doc.querySelectorAll(`iframe[${ATRIBUTO_MARCO}]`).forEach((f) => f.remove());
  const marco = doc.createElement('iframe');
  marco.setAttribute(ATRIBUTO_MARCO, '');
  marco.setAttribute('sandbox', 'allow-same-origin allow-modals');
  marco.setAttribute('referrerpolicy', 'no-referrer');
  marco.setAttribute('aria-hidden', 'true');
  marco.tabIndex = -1;
  // Fuera de la vista pero con tamaño de folio: con `display:none` o 0×0 hay
  // navegadores que no maquetan el documento y sale un folio vacío.
  marco.style.cssText =
    'position:fixed;left:-10000px;top:0;width:210mm;height:297mm;border:0;visibility:hidden';

  return new Promise<void>((resolver, rechazar) => {
    const tope = setTimeout(() => {
      marco.remove();
      rechazar(new Error('el documento de impresión no cargó'));
    }, ESPERA_CARGA_MS);
    marco.addEventListener(
      'load',
      () => {
        clearTimeout(tope);
        const ventana = marco.contentWindow;
        if (!ventana) {
          marco.remove();
          rechazar(new Error('sin ventana que imprimir'));
          return;
        }
        ventana.addEventListener('afterprint', () => marco.remove(), { once: true });
        try {
          ventana.focus();
          ventana.print();
          resolver();
        } catch (e) {
          marco.remove();
          rechazar(e instanceof Error ? e : new Error(String(e)));
        }
      },
      { once: true },
    );
    marco.srcdoc = html;
    doc.body.appendChild(marco);
  });
}
