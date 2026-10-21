// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pageshttp

import (
	"git.sytue.eu/sytue/one/platform/pkg/altcha"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/services/pages/internal/analitica"
	"git.sytue.eu/sytue/one/services/pages/internal/catalogo"
	"git.sytue.eu/sytue/one/services/pages/internal/config"
	"git.sytue.eu/sytue/one/services/pages/internal/dominios"
	"git.sytue.eu/sytue/one/services/pages/internal/ia"
	"git.sytue.eu/sytue/one/services/pages/internal/mapa"
	"git.sytue.eu/sytue/one/services/pages/internal/publicar"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
	"git.sytue.eu/sytue/one/services/pages/internal/tienda"
)

// Deps son las dependencias del router. Los servicios de cada área son tipos
// CONCRETOS de paquetes que su dueño sustituye entero (PAGES-ENCARGO §18.2): el
// esqueleto los deja sin comportamiento, y cablearlos es un bloque marcado de
// cmd/pages/main.go. Quien añada una dependencia añade SOLO su línea aquí.
type Deps struct {
	Cfg           config.Config
	Store         *store.Store
	Verifier      *authn.Verifier
	Objetos       *objectstore.Client
	ReadyCheckers []httpx.Checker
	Altcha        *altcha.Verifier

	Catalogo  *catalogo.Catalogo
	Publicar  *publicar.Servicio
	Mapa      *mapa.Escritor
	Lector    *mapa.Lector
	Dominios  *dominios.Servicio
	Analitica *analitica.Cliente
	Tienda    *tienda.Servicio
	IA        *ia.Servicio
}

// Handlers es el receptor de los manejadores de todas las áreas: cada una los
// escribe en su fichero de montaje (handlers.go, tienda.go, ia.go…).
type Handlers struct {
	Deps
}
