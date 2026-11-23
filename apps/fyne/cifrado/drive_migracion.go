// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

// Pasar a la bóveda directa el Drive de una cuenta que vive en Matrix (ADR 0086,
// «Drive nativo sobre la bóveda»). Lo pide la persona, con confirmación, desde
// el Drive de Matrix ya abierto:
//
//   - Se copia todo el catalogo con drive.Repository.MigrateToVault: cada
//     revisión, cada fichero recifrado y comprobado, y el recibo de la época del
//     servidor al final. Hasta que la copia no se relee entera, la bóveda no
//     tiene recibo y nadie la abre.
//   - Matrix no se toca: sus datos siguen ahí, se termine o no la copia.
//   - Mientras copia, la bóveda lleva en el servidor la marca «copia en curso»:
//     nadie más publica en ella ni la vacía, y la web enseña que otro dispositivo
//     está pasando el Drive a la bóveda.
//   - Una copia a medias se reanuda; si Matrix cambió entre medias, o el diario se
//     perdió y la marca dice que lo que hay es una copia, la bóveda (sin recibo,
//     solo con la copia) se vacía y se empieza otra vez. El vaciado va con lo que
//     la copia vio: si alguien publicó algo después, el servidor no vacía nada.
//
// Y la salida de quien no puede abrir Matrix: volver a la copia que el servidor
// guardó al congelar (AdoptarCopiaDrive), importada en la bóveda como lo haría
// la web. Lo escrito después en Matrix se queda allí, sin leer.

import (
	"context"
	"errors"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	privateDrive "git.sytue.eu/sytue/one/apps/nucleo/drive"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvault"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/vaultfiles"
)

// ErrMatrixPendiente: el Drive de Matrix tiene trabajo sin terminar en este
// dispositivo. Se termina en Matrix antes de copiar nada.
var ErrMatrixPendiente = errors.New("cifrado: Matrix Drive has unfinished work on this device")

// CopiarFicheroBoveda recifra en la bóveda (`destino`, con su pasarela y su
// diario) el contenido de un fichero de Matrix.
type CopiarFicheroBoveda func(ctx context.Context, destino *vaultfiles.Store, gateway *api.Drive, journal *offline.Store, job string, publication storage.PublicationTarget, ref storage.Reference, object, revision string) (storage.Reference, error)

// CopiarDesdeMatrix es CopiarFicheroBoveda con el cliente de Matrix de la cuenta.
func CopiarDesdeMatrix(client *mx.Client) CopiarFicheroBoveda {
	return func(ctx context.Context, destino *vaultfiles.Store, gateway *api.Drive, journal *offline.Store, job string, publication storage.PublicationTarget, ref storage.Reference, object, revision string) (storage.Reference, error) {
		if client == nil || destino == nil {
			return storage.Reference{}, ErrPrivateResources
		}
		return client.MigrateStorageFileContentInto(ctx, destino.StorageFiles, gateway, journal, job, publication, ref, object, revision)
	}
}

// MigracionDrive es lo que hace falta para copiar el Drive de Matrix de una
// cuenta en su bóveda.
type MigracionDrive struct {
	// Matrix es el repositorio de Drive de Matrix, ya abierto.
	Matrix *privateDrive.Repository
	// Copiar recifra cada fichero (CopiarDesdeMatrix).
	Copiar CopiarFicheroBoveda
	// AbrirBoveda abre la bóveda de la cuenta o, con `vaciar`, la vacía y la
	// estrena si sigue tal como se vio. Solo se vacía una bóveda sin recibo: la que
	// tiene una copia a medias que ya no vale.
	AbrirBoveda func(ctx context.Context, vaciar *api.PQVaultExpect) (*pqvault.Vault, error)
	// Fabrica abre los recursos de la bóveda (su diario y su sesión).
	Fabrica FabricaBoveda
	// DriveURL es el servicio de Drive.
	DriveURL string
	// Arrendamiento es el de Matrix: la copia trabaja con los dos repositorios a
	// la vez bajo el mismo arrendamiento de trabajo (nil: el de la bóveda).
	Arrendamiento offline.WorkLease
	// PendientesMatrix cuenta el trabajo sin terminar del Drive de Matrix.
	PendientesMatrix func(context.Context) (int, error)
}

// MigrarDriveABoveda copia el Drive de Matrix en la bóveda (ctx con la intención
// de autorización de la cuenta). Al volver sin error, la bóveda tiene el Drive
// entero y su recibo: la app vuelve a elegir y la abre. Nunca escribe en Matrix.
func MigrarDriveABoveda(ctx context.Context, m MigracionDrive) error {
	if m.Matrix == nil || m.Copiar == nil || m.AbrirBoveda == nil || m.PendientesMatrix == nil || m.DriveURL == "" {
		return ErrPrivateResources
	}
	pendientes, err := m.PendientesMatrix(ctx)
	if err != nil {
		return err
	}
	if pendientes > 0 {
		return ErrMatrixPendiente
	}
	err = m.intentar(ctx, nil)
	var lleno *privateDrive.VaultNotEmptyError
	if errors.As(err, &lleno) && lleno.Copy {
		// Una copia a medias que no se puede reanudar, en una bóveda sin recibo:
		// solo hay copia. Se vacía con la marca de la copia y tal como se vio, y se
		// empieza de nuevo; Matrix sigue entero. Si alguien publicó algo después,
		// el servidor no vacía (pqvault.ErrChanged) y el siguiente intento lo ve.
		err = m.intentar(ctx, lleno)
	}
	if errors.Is(err, privateDrive.ErrVaultInUse) {
		// La bóveda ya tiene su Drive completo: es la que vale.
		return nil
	}
	return err
}

func (m MigracionDrive) intentar(ctx context.Context, lleno *privateDrive.VaultNotEmptyError) error {
	var (
		vault *pqvault.Vault
		err   error
	)
	if lleno != nil {
		vault, err = m.AbrirBoveda(api.WithPQVaultCopy(ctx, lleno.CopyID), &lleno.Expect)
	} else {
		vault, err = m.AbrirBoveda(ctx, nil)
	}
	if err != nil {
		return err
	}
	recursos, err := m.Fabrica.Abrir(ctx, vault)
	if err != nil {
		return err
	}
	defer recursos.Close()
	scope := recursos.PrivateStorageScope()
	if scope != m.Matrix.PrivateStorageScope() || scope.Product != "drive" {
		return ErrPrivateResources
	}
	if lleno != nil {
		if err := privateDrive.DiscardVaultMigration(recursos.Journal, scope); err != nil {
			return err
		}
	}
	lease := m.Arrendamiento
	if lease == nil {
		lease = recursos.WorkLease()
	}
	gateway := api.NewDrive(m.DriveURL, recursos.HTTP(), nil)
	//nolint:contextcheck // El repositorio vive lo que los recursos de la bóveda (recursos.Context), no esta petición; se cierra al salir.
	repo, err := privateDrive.New(privateDrive.Config{
		Context: recursos.Context(), Scope: scope, Crypto: recursos.Store,
		Gateway: gateway, Source: gateway, Journal: recursos.Journal, WorkLease: lease,
	})
	if err != nil {
		return err
	}
	defer repo.Close() //nolint:contextcheck // Close cancela y espera su propia vida; no recibe contexto.
	return repo.MigrateToVault(ctx, m.Matrix, func(ctx context.Context, publication storage.PublicationTarget, job string, ref storage.Reference, object, revision string) (storage.Reference, error) {
		return m.Copiar(ctx, recursos.Store, gateway, recursos.Journal, job, publication, ref, object, revision)
	})
}

// AbrirBovedaDeSesion abre (o, con `vaciar`, vacía tal como se vio y estrena) la
// bóveda de `product` de la cuenta actual con la clave de desbloqueo del
// dispositivo, como PuertaBoveda pero sin interfaz: sin clave, pqvault.ErrSignin.
func AbrirBovedaDeSesion(ctx context.Context, suite *sesion.Suite, product string, vaciar *api.PQVaultExpect) (*pqvault.Vault, error) {
	if suite == nil || suite.Sesion == nil || !storage.Product(product) {
		return nil, ErrPrivateResources
	}
	owner := suite.Sesion.SnapshotIdentity()
	ctx = suite.Sesion.WithAuthorizationIntent(ctx)
	if auth.RequestAuthorizationIntent(ctx).Identity() != owner {
		return nil, auth.ErrAuthorizationChanged
	}
	clave, _ := suite.Sesion.Desbloqueo()
	defer clear(clave)
	if len(clave) == 0 {
		return nil, pqvault.ErrSignin
	}
	session := sesionBovedaDeCuenta{session: suite.Sesion, owner: owner}
	cfg := pqvault.Config{
		Scope:  storage.Scope{Tenant: owner.Organization, Subject: owner.Subject, Product: product},
		Server: api.NewDrive(suite.URL("drive"), session, nil), Registry: api.NewAccount(suite.URL("id"), session, nil),
		UnlockKey: clave,
	}
	if vaciar != nil {
		cfg.Expect = vaciar
		return pqvault.Reset(ctx, cfg)
	}
	return pqvault.Open(ctx, cfg)
}

// AdoptarCopiaDrive es «Vaciar y volver a los datos del servidor» para Drive:
// deja en el diario de la bóveda de esta app que la persona quiere la copia que
// el servidor guardó al congelar. La siguiente elección va a la bóveda, que la
// importa (drive.AdoptRetainedSource). No borra nada de Matrix.
func AdoptarCopiaDrive(ctx context.Context, suite *sesion.Suite) error {
	if suite == nil || suite.Sesion == nil {
		return ErrPrivateResources
	}
	fabrica, err := NuevaFabricaBoveda(suite, "drive")
	if err != nil {
		return err
	}
	ctx = suite.Sesion.WithAuthorizationIntent(ctx)
	session, err := fabrica.Capturar(ctx)
	if err != nil {
		return err
	}
	scope := session.PrivateStorageScope()
	state, err := api.NewDrive(suite.URL("drive"), session.HTTP(), nil).PrivateStorageState(ctx, scope)
	if err != nil {
		return err
	}
	if state.Validate(scope) != nil || state.Mode != "frozen" {
		// En `legacy` no hay nada que adoptar (la bóveda ya importa sola); en
		// `private` la copia del servidor ya no está para volver a ella.
		return privateDrive.ErrSource
	}
	release, err := session.WorkLease()(ctx)
	if err != nil {
		return err
	}
	defer release()
	journal, err := fabrica.Diario(ctx, session.witness)
	if err != nil {
		return err
	}
	return privateDrive.AdoptRetainedSource(journal, scope, state.Epoch)
}

// EstadoPaso es lo que enseña PasoBoveda.
type EstadoPaso uint8

const (
	// PasoOferta: explica qué pasa y pide la casilla.
	PasoOferta EstadoPaso = iota
	// PasoCopiando: la copia en marcha.
	PasoCopiando
	// PasoHecho: copiado y comprobado; la app pasa a la bóveda.
	PasoHecho
	// PasoFallo: no se pudo; Matrix sigue intacto. Reintentar.
	PasoFallo
	// PasoPendiente: Matrix tiene trabajo sin terminar; primero eso.
	PasoPendiente
	// PasoEntrar: este dispositivo no tiene la clave de desbloqueo.
	PasoEntrar
	// PasoOcupado: otro dispositivo está pasando ya el Drive a la bóveda.
	// Esperar y reintentar.
	PasoOcupado
)

// PasoBoveda es la confirmación y el progreso de pasar Drive a la bóveda. Sus
// métodos se llaman desde el hilo de la interfaz.
type PasoBoveda struct {
	// Migrar hace la copia (MigrarDriveABoveda), fuera del hilo de la interfaz.
	Migrar func(context.Context) error
	// Hecho se llama en el hilo de la interfaz cuando la copia termina bien.
	Hecho func()
	// Cancelar cierra el paso sin hacer nada.
	Cancelar func()
	// Dispatch lleva al hilo de la interfaz (fyne.Do si nil).
	Dispatch func(func())

	t        func(string) string
	ctx      context.Context
	estado   EstadoPaso
	raiz     *fyne.Container
	mensaje  *widget.Label
	progreso *widget.ProgressBarInfinite
	entiendo *widget.Check
	pasar    *widget.Button
	cancelar *widget.Button
}

// NuevoPasoBoveda crea el paso para la vida de `ctx` (la de la cuenta).
func NuevoPasoBoveda(ctx context.Context, translate func(string) string) *PasoBoveda {
	p := &PasoBoveda{t: translate, ctx: ctx}
	p.mensaje = widget.NewLabel("")
	p.mensaje.Wrapping = fyne.TextWrapWord
	p.progreso = widget.NewProgressBarInfinite()
	p.pasar = widget.NewButton(translate("vault.moveAction"), p.Confirmar)
	p.pasar.Importance = widget.HighImportance
	p.entiendo = widget.NewCheck(translate("vault.moveUnderstood"), func(bool) { p.pintar() })
	p.cancelar = widget.NewButton(translate("action.cancel"), func() {
		if p.Cancelar != nil && p.estado != PasoCopiando {
			p.Cancelar()
		}
	})
	// El título («vault.moveTitle») lo pone quien lo enseña (el diálogo).
	p.raiz = container.NewVBox(p.mensaje, p.progreso, p.entiendo, container.NewHBox(p.cancelar, p.pasar))
	p.pintar()
	return p
}

// Objeto es la vista del paso.
func (p *PasoBoveda) Objeto() fyne.CanvasObject { return p.raiz }

// Estado es lo que enseña ahora.
func (p *PasoBoveda) Estado() EstadoPaso { return p.estado }

// Confirmar empieza la copia. Solo con la casilla marcada y sin otra en marcha.
func (p *PasoBoveda) Confirmar() {
	if !p.entiendo.Checked || p.estado == PasoCopiando || p.estado == PasoHecho || p.Migrar == nil {
		return
	}
	p.estado = PasoCopiando
	p.pintar()
	dispatch := p.Dispatch
	if dispatch == nil {
		dispatch = fyne.Do
	}
	ctx := p.ctx
	go func() {
		err := p.Migrar(ctx)
		dispatch(func() { p.terminar(err) })
	}()
}

func (p *PasoBoveda) terminar(err error) {
	if p.ctx.Err() != nil {
		return
	}
	switch {
	case err == nil:
		p.estado = PasoHecho
	case errors.Is(err, ErrMatrixPendiente):
		p.estado = PasoPendiente
	case errors.Is(err, pqvault.ErrSignin):
		p.estado = PasoEntrar
	case errors.Is(err, privateDrive.ErrVaultCopyInProgress), errors.Is(err, pqvault.ErrCopyInProgress):
		p.estado = PasoOcupado
	default:
		p.estado = PasoFallo
	}
	p.pintar()
	if p.estado == PasoHecho && p.Hecho != nil {
		p.Hecho()
	}
}

func (p *PasoBoveda) pintar() {
	mostrar := func(o fyne.CanvasObject, si bool) {
		if si {
			o.Show()
		} else {
			o.Hide()
		}
	}
	switch p.estado {
	case PasoCopiando:
		p.mensaje.SetText(p.t("vault.moveCopying"))
	case PasoHecho:
		p.mensaje.SetText(p.t("vault.moveDone"))
	case PasoFallo:
		p.mensaje.SetText(p.t("vault.moveFailed"))
	case PasoPendiente:
		p.mensaje.SetText(p.t("vault.movePending"))
	case PasoEntrar:
		p.mensaje.SetText(p.t("matrix.signInBody"))
	case PasoOcupado:
		p.mensaje.SetText(p.t("vault.copyInProgress"))
	default:
		p.mensaje.SetText(p.t("vault.moveBody"))
	}
	ofrece := p.estado == PasoOferta || p.estado == PasoFallo || p.estado == PasoEntrar || p.estado == PasoOcupado
	mostrar(p.progreso, p.estado == PasoCopiando)
	mostrar(p.entiendo, ofrece)
	mostrar(p.pasar, ofrece)
	mostrar(p.cancelar, p.estado != PasoCopiando && p.estado != PasoHecho)
	if ofrece && p.entiendo.Checked {
		p.pasar.Enable()
	} else {
		p.pasar.Disable()
	}
}
