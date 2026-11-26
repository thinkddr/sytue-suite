// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"crypto/x509"
	"net/url"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
)

// anclasDeSello son las raíces contra las que se AUTENTICA el token de la autoridad de
// sellado (su firma, su cadena y el uso timeStamping), no solo que selle la huella y el
// nonce pedidos: sobre HTTP plano, sin esto un intermediario podría fabricar un token y
// el PDF saldría con un «sello» que el primer validador rechaza. Son las del sistema
// —las autoridades cualificadas europeas encadenan a raíces que llevan Mozilla, Apple y
// Microsoft, Izenpe incluida— más las de la carpeta de confianza de la persona, para una
// autoridad que el sistema no conozca. Si no cuadra con ninguna, no se sella.
func anclasDeSello(carpeta string) *x509.CertPool {
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	for _, c := range certificadosAncla(carpeta) {
		pool.AddCert(c)
	}
	return pool
}

// destinoSello es la autoridad de sellado elegida como destino del portero (se le
// pregunta en cada conexión: cambiarla en Ajustes vale en el acto). Sin autoridad, nada.
func (a *ajustesFirma) destinoSello() []red.Destino {
	actual, _ := a.sello()
	if d, ok := destinoDeAutoridad(actual); ok {
		return []red.Destino{d}
	}
	return nil
}

// destinoDeAutoridad es el destino de la dirección de una autoridad de sellado. RFC 3161
// va casi siempre por HTTP plano (la de Izenpe, también), y eso se admite aquí: el token
// lleva el nonce de la petición y se AUTENTICA contra su raíz (anclasDeSello), así que
// un intermediario no puede colar uno que valga.
func destinoDeAutoridad(direccion string) (red.Destino, bool) {
	u, err := url.Parse(strings.TrimSpace(direccion))
	if err != nil || u.Hostname() == "" {
		return red.Destino{}, false
	}
	d := red.Destino{Proposito: red.SelloDeTiempo, Host: strings.ToLower(u.Hostname()), Motivo: "CertiPDF — " + u.Hostname()}
	if p := u.Port(); p != "" {
		d.Puerto, _ = strconv.Atoi(p)
	}
	switch u.Scheme {
	case "https":
	case "http":
		d.Claro = true
	default:
		return red.Destino{}, false
	}
	return d, true
}
