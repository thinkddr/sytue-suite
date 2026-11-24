// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

// Sytue Mail nativo sobre la bóveda directa (ADR 0087, entrega 5: clientes
// nativos), como la web (`almacenCorreo.ts`, `openVaultMailPrivate`):
//
//   - Se elige por la autoridad del correo que sirve Rooms, la misma lectura y la
//     misma regla de la duda que la web (ElegirAlmacenCorreo).
//   - En la bóveda, el módulo de correo de siempre (mail/application) abre en modo
//     de bóveda: su almacén es la bóveda (con su capa de ficheros), sus secretos
//     de entrada son registros de la bóveda y sus acciones de entrada las firma la
//     clave de la bóveda (storage/vaultingress). Sin nada de Matrix.
//   - El traspaso desde Matrix (vaciar y pasar a la bóveda) lo lleva el anfitrión
//     (aplicacion_correo.go).

import (
	"context"
	"errors"
	"net/http"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/mail/application"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/vaultproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvault"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/vaultingress"
)

// AlmacenCorreo es dónde vive el correo privado de una cuenta, según la
// autoridad del correo que sirve Rooms (una por ámbito).
type AlmacenCorreo uint8

const (
	// CorreoEnBoveda: la autoridad es de la bóveda (`…:sytue-vault`), o no hay
	// ninguna todavía (cuenta nueva).
	CorreoEnBoveda AlmacenCorreo = iota + 1
	// CorreoEnMatrix: autoridad de Matrix confirmada. Su correo está cifrado con el
	// sistema anterior: se le ofrece pasarlo a la bóveda (vaciándolo) o seguir.
	CorreoEnMatrix
	// CorreoDudoso: cualquier otra respuesta (red, 5xx, respuesta rara, sin
	// organización). Se va por Matrix, el camino de siempre, sin proponer vaciar.
	CorreoDudoso
)

// codeRootEnrollmentRequired es el 409 de Rooms cuando el correo aún no tiene
// autoridad: una cuenta nueva.
const codeRootEnrollmentRequired = "storage_root_enrollment_required"

// LectorAutoridad es lo que se lee de Rooms para elegir (*api.Rooms).
type LectorAutoridad interface {
	MatrixStorageAuthority(context.Context, storage.Scope) (matrixproof.Authority, error)
}

// ElegirAlmacenCorreo es `elegirAlmacenCorreo` de la web. Elegir la bóveda, o
// proponer vaciar, sin saber qué hay podría dejar a alguien sin el correo que ya
// tiene: por eso cualquier duda es CorreoDudoso y nunca un error.
func ElegirAlmacenCorreo(ctx context.Context, rooms LectorAutoridad, scope storage.Scope) AlmacenCorreo {
	if rooms == nil || scope.Product != "mail" || scope.Tenant == "" || !scope.Valid() {
		return CorreoDudoso
	}
	authority, err := rooms.MatrixStorageAuthority(ctx, scope)
	if err != nil {
		var failure *api.Error
		if errors.As(err, &failure) && failure.Status == http.StatusConflict && failure.Code() == codeRootEnrollmentRequired {
			return CorreoEnBoveda
		}
		return CorreoDudoso
	}
	if vaultproof.IsAuthority(authority) {
		return CorreoEnBoveda
	}
	return CorreoEnMatrix
}

// destinoCorreo es lo que el anfitrión hace con una cuenta (`DestinoCorreo` de la
// web).
type destinoCorreo uint8

const (
	correoSinDecidir destinoCorreo = iota
	// correoBoveda: abrir la bóveda que hay.
	correoBoveda
	// correoMatrix: el correo de Matrix de siempre.
	correoMatrix
	// correoTraspaso: abrir una bóveda NUEVA y registrar su clave con la marca de
	// traspaso (vaciar y pasar a la bóveda).
	correoTraspaso
	// correoElegir: la pantalla que ofrece pasar el correo a la bóveda o seguir.
	correoElegir
)

// elegirDestinoCorreo: la bóveda gana SIEMPRE (con el traspaso hecho, o sin
// Matrix, se abre la que hay y nunca se vacía otra vez); un traspaso pedido va
// después; con autoridad de Matrix confirmada se ofrece elegir, salvo que en esta
// ejecución se eligiera seguir; ante la duda, Matrix sin proponer nada.
func elegirDestinoCorreo(almacen AlmacenCorreo, traspasoPedido, seguirConMatrix bool) destinoCorreo {
	switch {
	case almacen == CorreoEnBoveda:
		return correoBoveda
	case traspasoPedido:
		return correoTraspaso
	case almacen == CorreoEnMatrix && !seguirConMatrix:
		return correoElegir
	default:
		return correoMatrix
	}
}

// CorreoBovedaConfig son los servicios del correo de una cuenta (Suite.URL).
type CorreoBovedaConfig struct {
	MailURL, DriveURL, RoomsURL string
}

// RepositorioCorreoBoveda es el correo privado de una cuenta sobre la bóveda: el
// mismo MailApplicationRepository que en Matrix, en modo de bóveda, con el ciclo
// de preparación compartido.
type RepositorioCorreoBoveda struct {
	app   *MailApplicationRepository
	ciclo *RepositoryCycle[*MailApplicationRepository]
}

// NuevoCorreoBoveda monta el correo sobre los recursos de la bóveda del correo.
// La autoridad del correo de Rooms tiene que ser la de ESTA bóveda (su identidad,
// su raíz y activa); si es la de Matrix, este no es el camino. No prepara nada:
// eso es Initialize, fuera del hilo de la interfaz.
func NuevoCorreoBoveda(ctx context.Context, recursos *RecursosBoveda, cfg CorreoBovedaConfig) (*RepositorioCorreoBoveda, error) {
	if recursos == nil || recursos.PrivateStorageScope().Product != "mail" || cfg.MailURL == "" || cfg.DriveURL == "" || cfg.RoomsURL == "" {
		return nil, ErrPrivateResources
	}
	if err := recursos.Revalidate(ctx); err != nil {
		return nil, err
	}
	device, err := vaultingress.New(recursos.Store)
	if err != nil {
		return nil, err
	}
	scope := recursos.PrivateStorageScope()
	rooms := api.NewRooms(cfg.RoomsURL, recursos.HTTP(), nil)
	authority, err := rooms.MatrixStorageAuthority(ctx, scope)
	if err != nil {
		return nil, err
	}
	if err := device.RequireStorageAuthority(authority); err != nil {
		return nil, err
	}
	gateway := api.NewDrive(cfg.DriveURL, recursos.HTTP(), nil)
	source := api.NewMail(cfg.MailURL, recursos.HTTP(), nil)
	life := recursos.Context()
	//nolint:contextcheck // El correo vive lo que la cuenta (recursos.Context), no esta petición.
	app, err := application.New(life, recursos, application.Config{
		Gateway: gateway, Source: source, Control: source.PrivateIngress(),
		Metadata: source, Usage: gateway, Rooms: rooms, Authority: authority,
		Journal: recursos.Journal, Isolated: device, Vault: true, Invalid: ErrPrivateResources,
	})
	if err != nil {
		return nil, err
	}
	cycle, err := NewRepositoryCycle(recursos, RepositoryConfig[*MailApplicationRepository]{ //nolint:contextcheck // El ciclo retiene la vida de la cuenta, no esta petición.
		Create:  func(context.Context) (*MailApplicationRepository, error) { return app, nil },
		Prepare: func(ctx context.Context, repository *MailApplicationRepository) error { return repository.Prepare(ctx) },
	})
	if err != nil {
		app.Close() //nolint:contextcheck // La construcción no ha empezado ninguna E/S: solo cancela su vida.
		return nil, err
	}
	return &RepositorioCorreoBoveda{app: app, ciclo: cycle}, nil
}

// Initialize prepara el correo (época del servidor, alta de la clave de entrada,
// importación directa y receptor) y lo inicializa.
func (r *RepositorioCorreoBoveda) Initialize(ctx context.Context) error { return r.ciclo.Open(ctx) }

// Application es el correo ya preparado, el que monta la vista.
func (r *RepositorioCorreoBoveda) Application() *MailApplicationRepository { return r.app }

// Close retira el correo conservando su diario: lo pendiente se reanuda en la
// siguiente apertura. No cierra la bóveda: eso es RecursosBoveda.Close.
//
//nolint:contextcheck // Limpieza desligada: espera al trabajo cancelado y nunca descarta el diario.
func (r *RepositorioCorreoBoveda) Close() {
	ctx := context.Background()
	if _, err := r.ciclo.RetirePreservingWork(ctx); err != nil {
		return
	}
	_ = r.app.Retire(ctx)
}

// AbrirCorreoBoveda junta FabricaBoveda.Abrir y NuevoCorreoBoveda: lo que hace la
// app con la bóveda del correo que le entrega la puerta, ya con su clave de firma
// activa en Sytue ID. Si falla, nada queda abierto.
func AbrirCorreoBoveda(ctx context.Context, fabrica FabricaBoveda, cfg CorreoBovedaConfig, vault *pqvault.Vault) (*RecursosBoveda, *RepositorioCorreoBoveda, error) {
	if vault != nil && vault.SigningStatus() != pqvault.SigningRegistered {
		vault.Close()
		return nil, nil, pqvault.ErrSigningKey
	}
	recursos, err := fabrica.Abrir(ctx, vault)
	if err != nil {
		return nil, nil, err
	}
	repository, err := NuevoCorreoBoveda(ctx, recursos, cfg)
	if err != nil {
		recursos.Close()
		return nil, nil, err
	}
	return recursos, repository, nil
}
