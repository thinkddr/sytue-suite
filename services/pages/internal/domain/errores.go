// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package domain son los tipos y errores de Sytue Web que comparten el store, los
// servicios y la capa HTTP.
package domain

import "errors"

// ErrNoExiste es «no existe» y también «no puedes verlo»: la API contesta un solo
// not_found para los dos (PAGES-ENCARGO §6).
var ErrNoExiste = errors.New("pages: no existe")

// ErrSoloAdministracion es lo que un miembro sin rol de administración no puede hacer en
// un sitio: cambiar su dirección o sus datos legales (WEB-02). La API contesta 403.
var ErrSoloAdministracion = errors.New("pages: solo quien administra la organización")

// ErrLimiteSitios: la organización ya tiene sus 20 webs, o ya creó 5 hoy (WEB-08).
var ErrLimiteSitios = errors.New("pages: límite de webs de la organización")

// Errores de dominios propios (§8.3).
var (
	ErrDominioInvalido  = errors.New("pages: dominio no válido")
	ErrDominioReservado = errors.New("pages: dominio reservado de la plataforma")
	ErrDominioEnUso     = errors.New("pages: dominio ya en uso")
	ErrLimiteDominios   = errors.New("pages: límite de dominios alcanzado")
	ErrSitioSuspendido  = errors.New("pages: sitio suspendido")
)

// Errores de tienda, checkout y pedidos (§10.4-10.5, §9.5).
var (
	ErrTiendaOcupada           = errors.New("pages: store_busy")
	ErrExistenciasAgotadas     = errors.New("pages: existencias agotadas")
	ErrPagoNoDisponible        = errors.New("pages: pasarela de pago no disponible")
	ErrFacturaRequerida        = errors.New("pages: factura obligatoria")
	ErrNIFInvalido             = errors.New("pages: NIF/CIF no válido")
	ErrDemasiadosIntentos      = errors.New("pages: demasiados intentos de pago")
	ErrOrigenInvalido          = errors.New("pages: origen no permitido")
	ErrPedidoNoExiste          = errors.New("pages: pedido no encontrado")
	ErrTokenPedidoInvalido     = errors.New("pages: token de pedido no válido")
	ErrFormularioNoExiste      = errors.New("pages: formulario no encontrado")
	ErrConsentimientoRequerido = errors.New("pages: consentimiento obligatorio")
	// ErrAvisadosInvalidos: la lista de correos a los que avisa un formulario tiene una
	// dirección que no lo es, o más de las que se admiten.
	ErrAvisadosInvalidos  = errors.New("pages: invalid_notify")
	ErrTerminosRequeridos = errors.New("pages: aceptación de términos obligatoria")
	ErrReembolsoInvalido  = errors.New("pages: reembolso no válido")
	ErrTiendaNoDisponible = errors.New("pages: tienda no disponible")
	// ErrPedidoYaPagado: el pedido ya no espera pago; no se abre otro cobro (WEB-03).
	ErrPedidoYaPagado = errors.New("pages: el pedido ya no espera pago")
	// ErrPagoYaTratado: la notificación de un pago que ya consta cobrado. No es un
	// error para la pasarela: no se repite nada (avisos, evento al ERP, reembolso).
	ErrPagoYaTratado = errors.New("pages: pago ya tratado")
	// ErrDemasiadosPendientes: ese comprador ya tiene sus pedidos sin pagar; uno solo
	// no llena los huecos de la tienda (WEB-06).
	ErrDemasiadosPendientes = errors.New("pages: too_many_pending_orders")
)
