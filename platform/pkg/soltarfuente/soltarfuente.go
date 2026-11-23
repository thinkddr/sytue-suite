// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package soltarfuente monta «soltar la fuente» (ADR 0086) en un servicio de
// registros: POST /private-storage/migration/release devuelve a `legacy` una
// fuente `frozen` o `private`, con los datos del servidor tal como estaban al
// congelarse.
//
// Es la salida de quien no puede abrir lo que se copió a Matrix desde un
// dispositivo: sin ella, la web no escribe en una fuente congelada y la app
// nativa solo sabe ir a Matrix. Se pierde lo escrito después de la congelación
// en los dispositivos, y por eso exige tres cosas antes de tocar nada:
//
//   - que la pida la propia persona (sin delegación) con el ámbito de su app;
//   - la confirmación explícita en el cuerpo (`discard_private_writes`), no
//     solo la casilla de la interfaz;
//   - una identificación de hace menos de cinco minutos, con segundo factor si
//     la cuenta lo tiene (authn.IdentificacionReciente). Si falta, contesta el
//     401 del RFC 9470 con `max_age` para que el cliente mande a identificarse
//     y repita.
//
// Queda auditado en la fila de estado (`releases`, en la misma transacción), en
// el registro del servicio, y se avisa a la persona en la campana del Hub.
package soltarfuente

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// Ruta es donde se monta, junto a GET/POST /private-storage/migration.
const Ruta = "/private-storage/migration/release"

// TipoAviso es el tipo del aviso de la campana (`notifications.kind.<tipo>`).
const TipoAviso = "security.storage.released"

// Soltar corre la transacción del dominio (la de su Begin, con el GUC del dueño)
// alrededor de pg.ReleasePrivateStorageState.
// El ámbito ya está comprobado: es el del token y la cabecera de la app.
type Soltar func(ctx context.Context, scope storage.Scope, release pg.PrivateStorageRelease) error

// Config de un producto.
type Config struct {
	Product string
	Soltar  Soltar
	// Avisar deja el aviso en la campana de la persona (hubnotify.Client.Send, o
	// el propio Hub). Nil no avisa; un fallo no deshace nada, queda en el registro.
	Avisar func(context.Context, hubnotify.Notification) error
	Logger *slog.Logger
}

// Montar registra la ruta en el router autenticado del servicio.
func Montar(api chi.Router, cfg Config) {
	handler, err := Handler(cfg)
	if err != nil {
		panic(err)
	}
	api.Post(Ruta, handler)
}

// Handler es la ruta sin montar, para servicios que montan a mano.
func Handler(cfg Config) (http.HandlerFunc, error) {
	if cfg.Soltar == nil {
		return nil, errors.New("soltarfuente: falta Soltar")
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	soltar := cfg.Soltar
	return httpx.NewPrivateStorageRelease(httpx.PrivateStorageReleaseConfig{
		Product:         cfg.Product,
		Identity:        authn.PrivateStorageIdentity,
		Reauthenticated: authn.PrivateStorageReauthentication,
		Release: func(ctx context.Context, scope storage.Scope, epoch uuid.UUID, proof httpx.Reauthentication) error {
			return soltar(ctx, scope, pg.PrivateStorageRelease{Epoch: epoch, AuthTime: proof.AuthTime, AMR: proof.AMR})
		},
		MapError: Error,
		Notify: func(ctx context.Context, scope storage.Scope) {
			if cfg.Avisar == nil {
				return
			}
			if err := cfg.Avisar(ctx, Aviso(scope)); err != nil {
				logger.WarnContext(ctx, "soltar la fuente: el aviso no se envió", "product", scope.Product, "error", err)
			}
		},
		Logger: logger,
	})
}

// Error traduce los fallos del dominio sin dejar salir causas ni contenido.
func Error(err error) *httpx.Error {
	var failure *pgconn.PgError
	switch {
	case errors.Is(err, pg.ErrPrivateStorageReleaseStale):
		return httpx.ErrConflict.WithCode("private_storage_release_stale")
	case errors.Is(err, pg.ErrPrivateStorageScope):
		return httpx.ErrConflict.WithCode("storage_identity_changed")
	case errors.As(err, &failure) && failure.Code == "PST01":
		return httpx.ErrConflict.WithCode("private_storage_frozen")
	default:
		return httpx.ErrInternal
	}
}

// nombres son las marcas de los productos de registros, sin traducir.
var nombres = map[string]string{
	"calendar": "Sytue Calendar", "contacts": "Sytue Contacts", "notes": "Sytue Notepads",
	"erp": "Sytue ERP", "bases": "Sytue Bases", "cast": "Sytue Cast", "hub": "Sytue One",
	"docs": "Sytue Docus", "sheets": "Sytue Calcus", "slides": "Sytue Diapos",
}

// Aviso es el aviso a la persona: su app ha vuelto a los datos del servidor.
func Aviso(scope storage.Scope) hubnotify.Notification {
	nombre := nombres[scope.Product]
	if nombre == "" {
		nombre = scope.Product
	}
	source := scope.Product
	if source == "hub" {
		source = "one" // el origen de los avisos de Sytue One en el Hub
	}
	return hubnotify.Notification{
		Tenant: scope.Tenant, Subject: scope.Subject, Source: source,
		Type: TipoAviso, Level: "warning",
		Title:  fmt.Sprintf("%s ha vuelto a los datos del servidor", nombre),
		Params: map[string]string{"name": nombre},
	}
}
