// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package erp es el ERP de Sytue Office: un almacén SQLite de un solo inquilino y un
// TRANSPORTE EN PROCESO que atiende las rutas `/api/v1/…` de services/syerp sobre él
// (apps/fyne/PLAN-EDICION-LOCAL.md §6.13, D-10). `api.ERP` y la pantalla de
// `apps/fyne/apps/erp` no cambian: hablan HTTP como siempre, pero la petición no sale
// del proceso (no se abre ningún socket, R-RED) y los datos quedan en claro en
// `<datos>/Datos/erp/erp.sqlite` (R-CLARO), con el JSON de cada registro legible.
//
// # Primera entrega (E-02, E-03 y parte de E-04)
//
// Lo que la pantalla pide AL ARRANCAR, y qué contesta aquí:
//
//	GET  /catalogo                 el catálogo real (apps/nucleo/erp/catalogo) y los módulos encendidos
//	GET  /mis-permisos             todo en administrador; verbos crear, asentar, pagar, conciliar,
//	                               administrar (no aprobar: es de varias personas, E-09)
//	GET  /organizacion             el perfil, o 404 hasta que se hace el asistente de arranque
//	GET  /organizacion/requisitos  los del país del perfil (apps/nucleo/erp/obligaciones)
//	GET  /resumen                  facturación de clientes y lo asentado en el libro
//	GET  /{modelo}/estadisticas    recuento por estado y suma del primer importe de la lista
//	GET  /tableros, /actividades     los de la tercera entrega; /aprobaciones, 501 (la pantalla calla)
//
// Los flujos núcleo:
//
//	GET  /organizacion/paises, PUT /organizacion, POST /modulos/{m}   el asistente de arranque
//	GET  /obligaciones                                               el calendario (sin preparación)
//	GET|POST /{modelo}/  GET|PUT|DELETE /{modelo}/{id}               el CRUD genérico de TODOS los modelos
//	POST /{modelo}/{id}/estado|restaurar|duplicar|convertir          flujo, papelera, copia, presupuesto → pedido → factura
//	POST /{modelo}/lote                                              estado, borrar y restaurar en bloque
//	GET|POST /{modelo}/{id}/historial                                creado, cambios, borrado, restaurado y notas
//	GET  /search                                                     la paleta (⌘K)
//
// Clientes y proveedores (`contacto`), productos, presupuestos, pedidos, facturas de
// cliente y de proveedor y sus líneas son modelos del catálogo, así que salen con el
// CRUD genérico; los cobros y pagos son, por ahora, el estado «pagado» de la factura.
// La numeración (`FV-2026-00001`) sigue la del servidor: serie por modelo y año de la
// fecha del documento.
//
// # Sin VERI*FACTU (D-29)
//
// Las facturas se emiten SIN registro de VERI*FACTU hasta la entrega 4 y su revisión
// legal: `/verifactu` y `/{modelo}/{id}/verifactu` contestan 501 y la app lo dice en una
// franja fija (AvisoVerifactu).
//
// # Segunda entrega: contabilidad (E-05)
//
//	POST /{modelo}/{id}/asentar  GET /{modelo}/{id}/asientos     libro.go (dominio.AsientoDe, numeración por diario)
//	GET  /libro  GET /balance  POST /libro/{id}/anular            libro, sumas y saldos, anular con motivo
//	GET|POST /contabilidad/cierre  POST /contabilidad/reapertura  período cerrado: no se asienta ni se anula en él
//	GET  /banco  POST /banco/{id}/conciliar|explicar|reabrir       banco.go: el banco como tareas
//	POST /linea_extracto/importar (Norma 43)                       casa lo seguro y explica lo evidente (apps/nucleo/erp/banca)
//	GET  /resumen                                                  con lo asentado
//
// Una factura de venta que se EMITE (pasa a confirmada o pagada) tiene que cuadrar con
// sus líneas (dominio.CuadreFactura), y convertir presupuesto → pedido → factura copia
// las líneas como el servidor (dominio.ComprobarImportesVenta).
//
// # Tercera entrega (E-06) e IA (E-08)
//
//	GET|POST /actividades, /actividades/{id}/hecha|cancelar, /equipo   seguimiento.go (una persona: E-09)
//	GET|POST /adjuntos, GET|DELETE /adjuntos/{id}                       FICHEROS en Datos/erp/adjuntos/
//	GET  /{modelo}/export                                              CSV con las celdas del servidor (apps/nucleo/erp/exportar)
//	POST /{modelo}/importar[?simular=1], GET /importaciones,
//	POST /importaciones/{id}/revertir                                  importacion.go: lote con vista previa y marcha atrás
//	POST /informes/datos|excel, GET|POST|DELETE /tableros               informes.go (apps/nucleo/erp/ia)
//	POST /ia/informe, POST /{modelo}/ia/rellenar (texto o PDF)          con la IA de la persona (PonerIA); sin ella, 503
//	POST /ia/acciones, /ia/acciones/crear, /banco/pistas                ia_acciones.go: plan de registros y pistas del banco
//	GET  /obligaciones/calculo|ics, GET|POST /obligaciones/proceso,
//	GET  /obligaciones/proceso/pendientes                              obligaciones.go (apps/nucleo/erp/obligaciones)
//	POST /tpv/cobrar, /tpv/presupuesto                                 tpv.go (apps/nucleo/erp/tpv)
//
// # Facturas (apps/nucleo/erp/efactura)
//
//	GET  /{modelo}/{id}/facturae|ubl|imprimir|pdf                       facturas.go: la misma generación que el servidor
//	GET|POST /factura_venta/{id}/enviar                                 enviar.go: por correo, con el PDF adjunto
//	POST /factura_compra/asistencia/leer, /{modelo}/importar (XML)      la factura recibida en UBL, CII o Facturae
//
// El PDF de la factura se escribe con apps/nucleo/pdfdoc y el texto de un PDF se lee
// con apps/nucleo/pdf; ninguno sale del proceso. Enviarla por correo sí sale: por el
// SMTP de una cuenta de Sytue Mail de este equipo que elige la persona
// (`nucleo/local/correo.Remitente`, PonerCorreo), por el portero de red; sin cuentas,
// contesta 503 `correo_no_configurado` como un despliegue sin servidor de correo.
//
// # Comprobar un NIF-IVA en VIES (vies.go)
//
// No es una ruta: la pantalla lo pide directamente (`Pantalla.ComprobarVIES`, que pone
// cmd-local/erp) y solo cuando la persona pulsa «Comprobar en VIES». Sale el prefijo y
// el número al servicio de la Comisión Europea por el permiso puntual del portero
// (propósito `vies`); lo contestado se enseña junto a lo propuesto y no cambia nada.
//
// # Vectores dorados
//
// apps/nucleo/erp/vectores describe escenarios (numeración, asientos, saldos) que
// recorren este almacén (vectores_test.go) y el de Postgres
// (services/syerp/internal/store/vectores_integration_test.go, etiqueta integration).
//
// # Lo que contesta 501
//
// VERI*FACTU (D-29); y lo que es de varias personas o de un servidor (E-09):
// aprobaciones, peticiones de edición y conectores.
// Contestan 501 con el sobre de error de siempre y el código `no_disponible_local`, que
// la pantalla dice con su texto (apps/fyne/apps/erp.ErroresLocales).
package erp
