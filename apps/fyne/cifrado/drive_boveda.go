// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

// Drive nativo sobre la bóveda directa (ADR 0086, «Drive nativo sobre la
// bóveda»). La web ya guarda Drive en la bóveda (`private-drive.ts`); aquí, cuando
// la web ya llevó ahí los datos de una persona, Drive, Photos y PDF nativos leen y
// escriben en esa misma bóveda, y si no, siguen en Matrix como hasta ahora.
//
// Tres reglas que no se rompen:
//
//   - Se elige por cuenta y sin pedir ninguna clave, solo con lo que dicen el
//     servidor y los diarios del dispositivo (ElegirAlmacenDrive).
//   - Nunca se mezclan los diarios: la bóveda tiene su diario de dispositivo
//     aparte (sesion.DiarioBoveda), que el cierre de sesión cuenta y borra como el
//     de Matrix. Una subida pendiente en el diario de Matrix se termina en Matrix:
//     mientras la haya, esa cuenta sigue en Matrix.
//   - Se desbloquea con la clave de Sytue ID del dispositivo (PuertaBoveda): ni
//     una contraseña nueva ni un bucle de inicio de sesión.

import (
	"context"
	"sync"

	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	privateDrive "git.sytue.eu/sytue/one/apps/nucleo/drive"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvault"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/vaultfiles"
)

// AlmacenDrive es dónde vive el Drive cifrado de una cuenta en este dispositivo.
type AlmacenDrive uint8

const (
	// DriveEnMatrix: el de siempre (matrix.StorageFiles sobre la cuenta de Matrix).
	DriveEnMatrix AlmacenDrive = iota + 1
	// DriveEnBoveda: la bóveda directa, la misma que usa la web.
	DriveEnBoveda
)

// ConsultaDrive es lo que hace falta saber para elegir. Ninguna pide clave.
type ConsultaDrive struct {
	// BovedaConDatos: ¿la bóveda de Drive de la cuenta guarda ya registros?
	BovedaConDatos func(context.Context) (bool, error)
	// PendientesMatrix y PendientesBoveda cuentan el trabajo sin terminar de Drive
	// de la cuenta en los diarios de Matrix y en los de la bóveda.
	PendientesMatrix func(context.Context) (int, error)
	PendientesBoveda func(context.Context) (int, error)
	// CopiaPendiente: ¿hay en algún diario de la bóveda una copia de Matrix a la
	// bóveda sin terminar (drive.VaultMigrationNamespace)?
	CopiaPendiente func(context.Context) (bool, error)
	// CopiaEnServidor: ¿la bóveda lleva la marca de una copia de Matrix (de este
	// dispositivo con el diario perdido, o de otro), en vigor o caducada?
	CopiaEnServidor func(context.Context) (bool, error)
	// FuenteLegacy: ¿la fuente de Drive del servidor sigue en `legacy` (nadie la
	// ha congelado todavía)?
	FuenteLegacy func(context.Context) (bool, error)
}

// ElegirAlmacenDrive decide, en este orden:
//
//  1. Trabajo pendiente en el diario de Matrix → Matrix. Se termina donde se
//     empezó; la cuenta pasa a la bóveda cuando ese diario quede vacío.
//  2. Una copia de Matrix a la bóveda sin terminar → Matrix: sus datos siguen
//     ahí, enteros, y desde ahí se reanuda o se repite la copia.
//  3. Trabajo pendiente en el diario de la bóveda → la bóveda, por lo mismo que
//     el 1 (también una importación a medias o la vuelta a la copia del servidor).
//  4. La bóveda lleva la marca de una copia de Matrix → Matrix: otro dispositivo
//     está copiando, o una copia se quedó a medias y su diario se perdió. Lo que
//     haya en la bóveda sin recibo es copia; los datos siguen enteros en Matrix,
//     y desde ahí se ofrece repetir la copia (vaciando la bóveda con seguridad).
//  5. La bóveda guarda registros (la web ya migró a esa persona) → la bóveda.
//  6. La fuente del servidor sigue en `legacy` → la bóveda, que la importa como
//     la web (drive.Config.DirectImport). Matrix ya no congela la fuente.
//  7. Si no → Matrix: el Drive de esa persona está en Matrix. Desde ahí se ofrece
//     pasarlo a la bóveda (MigrarDriveABoveda).
//
// Cualquier error es un error: ni una cosa ni la otra («Reintentar»).
func ElegirAlmacenDrive(ctx context.Context, c ConsultaDrive) (AlmacenDrive, error) {
	if c.BovedaConDatos == nil || c.PendientesMatrix == nil || c.PendientesBoveda == nil || c.CopiaPendiente == nil || c.CopiaEnServidor == nil || c.FuenteLegacy == nil {
		return 0, ErrPrivateResources
	}
	matrix, err := c.PendientesMatrix(ctx)
	if err != nil {
		return 0, err
	}
	if matrix > 0 {
		return DriveEnMatrix, nil
	}
	copia, err := c.CopiaPendiente(ctx)
	if err != nil {
		return 0, err
	}
	if copia {
		return DriveEnMatrix, nil
	}
	boveda, err := c.PendientesBoveda(ctx)
	if err != nil {
		return 0, err
	}
	if boveda > 0 {
		return DriveEnBoveda, nil
	}
	marcada, err := c.CopiaEnServidor(ctx)
	if err != nil {
		return 0, err
	}
	if marcada {
		return DriveEnMatrix, nil
	}
	datos, err := c.BovedaConDatos(ctx)
	if err != nil {
		return 0, err
	}
	if datos {
		return DriveEnBoveda, nil
	}
	legacy, err := c.FuenteLegacy(ctx)
	if err != nil {
		return 0, err
	}
	if legacy {
		return DriveEnBoveda, nil
	}
	return DriveEnMatrix, ctx.Err()
}

// ConsultaDriveDeSesion pregunta por `owner` con la sesión de la suite atada a
// esa cuenta (una petición tras cambiar de cuenta falla, nunca lleva el token de
// otra persona).
func ConsultaDriveDeSesion(suite *sesion.Suite, owner auth.IdentitySnapshot) ConsultaDrive {
	scope := storage.Scope{Tenant: owner.Organization, Subject: owner.Subject, Product: "drive"}
	servidor := func() *api.Drive {
		return api.NewDrive(suite.URL("drive"), nuevaSesionDeCuenta(suite.Sesion, owner), nil)
	}
	return ConsultaDrive{
		BovedaConDatos: func(ctx context.Context) (bool, error) {
			return pqvault.HasRecords(ctx, servidor(), scope)
		},
		PendientesMatrix: func(ctx context.Context) (int, error) {
			return suite.PendientesPrivadosDe(ctx, owner, "drive", false)
		},
		PendientesBoveda: func(ctx context.Context) (int, error) {
			return suite.PendientesPrivadosDe(ctx, owner, "drive", true)
		},
		CopiaPendiente: func(ctx context.Context) (bool, error) {
			n, err := suite.PendientesPrivadosEn(ctx, owner, "drive", true, privateDrive.VaultMigrationNamespace)
			return n > 0, err
		},
		CopiaEnServidor: func(ctx context.Context) (bool, error) {
			state, err := servidor().PQVaultKeys(ctx, scope)
			if err != nil {
				return false, err
			}
			return state.Copy != nil && state.Copy.Kind == api.PQVaultCopyKind, nil
		},
		FuenteLegacy: func(ctx context.Context) (bool, error) {
			state, err := servidor().PrivateStorageState(ctx, scope)
			if err != nil {
				return false, err
			}
			if state.Validate(scope) != nil {
				return false, privateDrive.ErrSource
			}
			return state.Mode == "legacy", nil
		},
	}
}

// ElegirDriveDeSesion elige para la cuenta actual de la sesión.
func ElegirDriveDeSesion(ctx context.Context, suite *sesion.Suite) (AlmacenDrive, auth.IdentitySnapshot, error) {
	if suite == nil || suite.Sesion == nil {
		return 0, auth.IdentitySnapshot{}, ErrPrivateResources
	}
	owner := suite.Sesion.SnapshotIdentity()
	if owner.Subject == "" || owner.Organization == "" {
		return 0, owner, auth.ErrAuthorizationChanged
	}
	almacen, err := ElegirAlmacenDrive(ctx, ConsultaDriveDeSesion(suite, owner))
	return almacen, owner, err
}

// RecursosBoveda son los de una app sobre la bóveda: la sesión de la app (testigo
// de trabajo, arrendamiento y HTTP atados a una cuenta, los mismos que Matrix),
// la bóveda abierta con su capa de ficheros y el diario propio de la bóveda.
// Cumple ResourceGuard y Cuenta. La bóveda es suya: Close la cierra.
type RecursosBoveda struct {
	session *ApplicationSession
	life    context.Context
	Vault   *pqvault.Vault
	Store   *vaultfiles.Store
	Journal *offline.Store
	once    sync.Once
}

var (
	_ ResourceGuard = (*RecursosBoveda)(nil)
	_ Cuenta        = (*RecursosBoveda)(nil)
)

// FabricaBoveda abre los recursos de la bóveda de una app.
type FabricaBoveda struct {
	// Capturar ata la app a la sesión actual (Factory.Capture).
	Capturar func(context.Context) (*ApplicationSession, error)
	// Diario abre el diario de la bóveda con el testigo de esa sesión.
	Diario func(context.Context, auth.TestigoTrabajo) (*offline.Store, error)
}

// NuevaFabricaBoveda es la de una app instalada de la suite para `product`.
func NuevaFabricaBoveda(suite *sesion.Suite, product string) (FabricaBoveda, error) {
	factory, err := NewNativeFactory(suite, product)
	if err != nil {
		return FabricaBoveda{}, err
	}
	return FabricaBoveda{
		Capturar: factory.Capture,
		Diario: func(ctx context.Context, witness auth.TestigoTrabajo) (*offline.Store, error) {
			return suite.DiarioBoveda(ctx, product, witness)
		},
	}, nil
}

// Abrir toma posesión de `vault` (recién entregada por la puerta, con la
// intención de autorización de la cuenta en ctx). Si falla, la cierra.
func (f FabricaBoveda) Abrir(ctx context.Context, vault *pqvault.Vault) (*RecursosBoveda, error) {
	if vault == nil {
		return nil, ErrPrivateResources
	}
	if f.Capturar == nil || f.Diario == nil {
		vault.Close()
		return nil, ErrPrivateResources
	}
	session, err := f.Capturar(ctx)
	if err != nil {
		vault.Close()
		return nil, err
	}
	// Desde aquí, la vida es la de la app (testigo persistente), no la intención
	// corta de la entrega: lo mismo que Factory.Open.
	return f.abrir(session.Context(), session, vault) //nolint:contextcheck // ver arriba.
}

func (f FabricaBoveda) abrir(life context.Context, session *ApplicationSession, vault *pqvault.Vault) (*RecursosBoveda, error) {
	if session.PrivateStorageScope() != vault.Scope() {
		vault.Close()
		return nil, ErrPrivateResources
	}
	store, err := vaultfiles.New(vault)
	if err != nil {
		vault.Close()
		return nil, err
	}
	release, err := session.WorkLease()(life)
	if err != nil {
		vault.Close()
		return nil, err
	}
	journal, err := f.Diario(life, session.witness)
	release()
	if err != nil {
		vault.Close()
		return nil, err
	}
	if journal == nil || !journal.Encrypted() {
		vault.Close()
		return nil, offline.ErrEncryptedStore
	}
	r := &RecursosBoveda{session: session, life: life, Vault: vault, Store: store, Journal: journal}
	if err := r.Revalidate(life); err != nil {
		r.Close()
		return nil, err
	}
	return r, nil
}

func (r *RecursosBoveda) Context() context.Context           { return r.life }
func (r *RecursosBoveda) PrivateStorageScope() storage.Scope { return r.session.PrivateStorageScope() }
func (r *RecursosBoveda) HTTP() api.Session                  { return r.session.HTTP() }
func (r *RecursosBoveda) WorkLease() offline.WorkLease       { return r.session.WorkLease() }
func (r *RecursosBoveda) Register(ctx context.Context, work offline.PrivateWork) (func(), error) {
	return r.session.Register(ctx, work)
}

// Check falla en cuanto la sesión deja de ser la de la app o la bóveda se cierra.
func (r *RecursosBoveda) Check(ctx context.Context) error {
	if err := r.session.Check(ctx); err != nil {
		return err
	}
	if r.Vault.Closed() {
		return pqvault.ErrClosed
	}
	return nil
}

// Revalidate comprueba, con el arrendamiento compartido, que la bóveda sigue
// siendo la de esta cuenta.
func (r *RecursosBoveda) Revalidate(ctx context.Context) error {
	release, err := r.WorkLease()(ctx)
	if err != nil {
		return err
	}
	defer release()
	if err := r.Check(ctx); err != nil {
		return err
	}
	binding, err := r.Vault.StorageBindings(ctx)
	if err != nil {
		return err
	}
	if binding.Scope != r.PrivateStorageScope() {
		return ErrPrivateResources
	}
	return r.Check(ctx)
}

// Close cierra la bóveda (borra sus claves de memoria). El diario sigue en disco:
// lo pendiente se reanuda en la siguiente apertura o se descarta al salir.
func (r *RecursosBoveda) Close() {
	if r == nil {
		return
	}
	r.once.Do(r.Vault.Close)
}

// NuevoRepositorioDriveBoveda es el repositorio de Drive de siempre
// (`apps/nucleo/drive`) sobre la bóveda y su diario: el mismo contrato que el de
// Matrix para la pantalla, Photos y PDF. Abre con el recibo de migración de la
// época del servidor (el que deja la web, la importación nativa o la copia desde
// Matrix). Con la fuente en `legacy`, la importa como la web (DirectImport); una
// época que congeló otro solo la reanuda si la empezó este dispositivo o la
// persona eligió volver a la copia del servidor, y si no, ErrNotReady.
func NuevoRepositorioDriveBoveda(recursos *RecursosBoveda, driveURL string) (*PreparedDriveRepository, error) {
	if recursos == nil || recursos.PrivateStorageScope().Product != "drive" {
		return nil, ErrPrivateResources
	}
	gateway := api.NewDrive(driveURL, recursos.HTTP(), nil)
	repository, err := privateDrive.New(privateDrive.Config{
		Context: recursos.Context(), Scope: recursos.PrivateStorageScope(), Crypto: recursos.Store,
		Gateway: gateway, Source: gateway, Journal: recursos.Journal, WorkLease: recursos.WorkLease(),
		DirectImport: true,
	})
	if err != nil {
		return nil, err
	}
	cycle, err := NewRepositoryCycle(recursos, RepositoryConfig[*privateDrive.Repository]{
		Create: func(context.Context) (*privateDrive.Repository, error) { return repository, nil },
	})
	if err != nil {
		repository.Close()
		return nil, err
	}
	return &PreparedDriveRepository{Repository: repository, ciclo: cycle, retireSource: func(context.Context) error {
		recursos.Close()
		return nil
	}}, nil
}

// AbrirDriveBoveda junta FabricaBoveda.Abrir y NuevoRepositorioDriveBoveda: lo
// que una app hace con la bóveda que le entrega la puerta. No inicializa: eso
// es Initialize, fuera del hilo de la interfaz.
func AbrirDriveBoveda(ctx context.Context, fabrica FabricaBoveda, driveURL string, vault *pqvault.Vault) (*RecursosBoveda, *PreparedDriveRepository, error) {
	recursos, err := fabrica.Abrir(ctx, vault)
	if err != nil {
		return nil, nil, err
	}
	repository, err := NuevoRepositorioDriveBoveda(recursos, driveURL) //nolint:contextcheck // El repositorio vive lo que la cuenta (recursos.Context), no esta petición.
	if err != nil {
		recursos.Close()
		return nil, nil, err
	}
	return recursos, repository, nil
}

// PuertaDriveDeSesion es la puerta de la bóveda de Drive de la cuenta actual.
func PuertaDriveDeSesion(suite *sesion.Suite, translate func(string) string, lista func(context.Context, *pqvault.Vault)) (*PuertaBoveda, error) {
	return NuevaPuertaBovedaDeSesion(suite, "drive", translate, lista)
}
