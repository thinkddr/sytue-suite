// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package certipdf es Sytue CertiPDF en Fyne: el visor y anotador de PDF que en la web
// es `apps/frontend/packages/office/src/pdf/` montado en `pages/pdf.astro`.
//
// # Por qué NO es un editor.Lienzo
//
// Docus, Calcus y Diapos comparten el cromo de [git.sytue.eu/sytue/one/apps/fyne/editor]
// porque en la web son literalmente `EditorApp` con otro `kind`. CertiPDF no lo es:
// `pdf.astro` monta `PdfApp`, que trae SU PROPIA cabecera-cinta (`Barra.tsx`, seis
// pestañas) y NO tiene ninguna de las cuatro zonas del editor —ni cabecera con título
// editable, ni carril de cinco paneles, ni panel de ajustes, ni barra de estado—.
//
// De todo el cromo del editor, `Barra.tsx` importa EXACTAMENTE una cosa: `RibbonGroup`
// de `../editor/ribbon/primitives`. Aquí se hace lo mismo y por el mismo motivo: se
// importan las primitivas de la cinta ([editor.Grupo], [editor.PestanaCinta],
// [editor.BotonCinta], [editor.FilaGrupos]) y nada más. Meter CertiPDF dentro de
// `editor.Lienzo` le haría crecer una cabecera, un carril y una barra de estado que la
// web no tiene, que es inventar paridad en vez de copiarla.
//
// El catálogo de idioma SÍ es el mismo ([catalogos.Docs]): en la web es una sola app.
//
// # Este paquete NO FIRMA y NO REESCRIBE
//
// Tres límites que no son de esta ola sino del producto, y están aquí arriba para que
// nadie los cruce por accidente —el mismo sitio donde el ERP declara su solo-lectura—:
//
//  1. **No hay criptografía en este paquete.** Ni una línea. Firmar un PDF lo hace el
//     motor del servicio (`services/docs/internal/domain/pdfsign.go`) o la aplicación
//     OFICIAL del país de quien firma ([HerramientasNacionales]). Esta vista enseña lo
//     que la firma DECLARA y si cubre el fichero entero; eso no es verificarla, y la
//     pantalla lo dice con esas palabras (`pdf.signatures.disclaimer`).
//     La norma se levanta SOLO para una llamada (ADR 0051): pedir al almacén del
//     sistema la firma de un resumen que preparó el servicio (`firmasistema.go`, vía
//     `apps/nucleo/certificados`). Ni ahí se escribe un byte de formato criptográfico.
//
//  2. **Un documento firmado no se reescribe jamás.** Este paquete no escribe PDF: no
//     tiene serializador. Lo que cambia el documento son las operaciones del servicio
//     (`op: anotar`, `redactar`, `crear-campos`…), que se piden y se vuelven a leer
//     enteras. Un viaje de ida y vuelta sin pérdida no hay que demostrarlo porque no
//     hay vuelta: el cliente no produce bytes de PDF.
//
//  3. **Una página que no se sabe dibujar se dice, no se aproxima.** Ver [Hoja] y el
//     comentario de `pagina.go`, que es donde está medido por qué.
package certipdf
