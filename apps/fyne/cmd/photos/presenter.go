// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"runtime"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/apps/photos"
	"git.sytue.eu/sytue/one/apps/fyne/cifrado"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/carrete"
	privateDrive "git.sytue.eu/sytue/one/apps/nucleo/drive"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	privatePhotos "git.sytue.eu/sytue/one/apps/nucleo/photos"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

// almacenPhotos es dónde lee y escribe la galería de una cuenta: el repositorio
// cifrado de Matrix con su Drive oculto, o —sin repositorio— la API del servidor,
// como la web (ADR 0086, «Photos como las apps de registros»).
type almacenPhotos struct {
	cuenta     cifrado.Cuenta
	identity   auth.IdentitySnapshot
	repository *privatePhotos.Repository
	// resolveDrive es el Drive oculto de los originales; solo con repositorio.
	resolveDrive func(context.Context) (*privateDrive.Repository, error)
	// driveDeSubida, solo por la API: dónde suben las fotos (photos.App.DriveDeSubida).
	driveDeSubida func(context.Context) (*api.Drive, error)
	// galeriaCifrada, solo por la API: el Drive de la bóveda para la galería cifrada
	// (photos.App.GaleriaCifrada).
	galeriaCifrada func(context.Context, bool) (*api.Drive, error)
}

func newPhotosPresenter(application fyne.App, window fyne.Window, suite *sesion.Suite, translate *i18n.Translator, almacen almacenPhotos) (*photosPresenter, error) {
	// Las escrituras de Photos por la API avisan a su widget (Suite.ClienteConAviso);
	// las del Drive de la galería no: el servidor avisa al indexar (señal `datos`).
	client, drive, err := clientesPhotos(suite.URL("photos"), suite.URL("drive"), suite.ClienteConAviso(nil), almacen)
	if err != nil {
		return nil, err
	}
	if almacen.repository != nil {
		// Con repositorio, el widget lee esos registros mientras la cuenta siga montada: el
		// servidor de esta cuenta está congelado desde que pasó a Matrix (ADR 0078).
		suite.WidgetDesdeRegistrosPrivados(almacen.cuenta.Context(), almacen.cuenta.HTTP(), client)
	}
	thumbnails, large := photos.NuevaFototeca(), photos.NuevaFototeca()
	large.Variante = photos.VarianteGrande
	view := photos.Vista{T: translate, Teca: thumbnails, TecaGrande: large, Ventana: anchoInicial, Variante: tema.De(application.Settings().ThemeVariant())}
	presenter := photos.Nueva(client, view, window)
	presenter.Drive = drive
	presenter.DriveDeSubida = almacen.driveDeSubida
	presenter.GaleriaCifrada = almacen.galeriaCifrada
	presenter.ClaveEnDispositivo = func() bool { return claveEnDispositivo(suite.Sesion) }
	resources := almacen.cuenta
	presenter.Directorio = api.NewDirectorio(suite.URL("id"), resources.HTTP(), nil)
	presenter.AlEntrar = func() { go func() { _ = suite.Entrar(context.Background()) }() }
	presenter.AbrirWeb = func(target string) { _ = api.AbrirFuera(application, target) }
	presenter.WebDe = func(path string) string { return api.AppURL(suite.Apex(), "photos", "", path) }
	organization := resources.PrivateStorageScope().Tenant
	presenter.AbrirApp = func(product, path string) { _ = api.AbrirEn(application, suite.Apex(), product, organization, path) }
	presenter.Elegir = func() ([]string, error) { return ficheros.ElegirImagenes(translate.T("gallery.add", nil)) }
	presenter.Guardar = func(name string, raw []byte) (string, error) {
		if runtime.GOOS == "ios" || runtime.GOOS == "android" {
			if err := guardarEnGaleria(presenter, name, raw); !errors.Is(err, carrete.ErrNoDisponible) {
				return "", err
			}
		}
		directory, err := ficheros.Carpeta()
		if err != nil {
			return "", err
		}
		return ficheros.Guardar(directory, name, bytes.NewReader(raw))
	}
	thumbnails.Pedir = func(ctx context.Context, id, variant string) ([]byte, error) {
		return leer(client.Variant(ctx, id, variant))
	}
	large.Pedir = func(ctx context.Context, id, variant string) ([]byte, error) {
		return leer(client.VariantDe(ctx, id, variant, presenter.Huella(id)))
	}
	presenter.NoEncontrado = suite.NoEncontrado
	presenter.Galeria = photos.CarreteDelSistema{}
	if ruta, err := photos.RutaRegistroCopia(); err == nil {
		presenter.Copia = photos.CopiaDe(presenter.Galeria, ruta)
	}
	presenter.FondoDisponible = runtime.GOOS == "android" || runtime.GOOS == "ios"
	switch runtime.GOOS {
	case "darwin":
		presenter.DestinoGaleria = photos.GaleriaFotos
	case "windows", "linux":
		presenter.DestinoGaleria = photos.GaleriaImagenes
	}
	presenter.AlCambiarCopia = func(aj photos.AjustesCopia) { suite.ProgramarTareaEnFondo(aj.Activa, aj.ConDatos) }
	presenter.SeguirEnFondo = sistema.SeguirEnFondo
	presenter.CambiarSesion(almacen.identity.Issuer + "\x00" + almacen.identity.Subject + "\x00" + almacen.identity.Organization)
	presenter.Arrancar()
	return &photosPresenter{app: presenter, client: client, repository: almacen.repository, resources: resources}, nil
}

// clientesPhotos son los clientes de Photos y de Drive de la galería. Con
// repositorio, los datos privados: los registros cifrados y el Drive oculto de los
// originales (bóveda o Matrix, cifrado.AplicacionDrive). Sin él, la API del
// servidor: los originales van a la carpeta «Fotos» del Drive del servidor y el
// servicio de Photos los indexa, igual que client/subida.ts en la web. `escrituras`
// es el cliente HTTP de Photos por la API (nil: el de siempre); con repositorio no se
// usa, porque nada sale por HTTP.
func clientesPhotos(photosURL, driveURL string, escrituras *http.Client, almacen almacenPhotos) (*api.Photos, *api.Drive, error) {
	if almacen.cuenta == nil {
		return nil, nil, cifrado.ErrPrivateResources
	}
	session := almacen.cuenta.HTTP()
	client := api.NewPhotos(photosURL, session, escrituras)
	drive := api.NewDrive(driveURL, session, nil)
	if almacen.repository == nil {
		return client, drive, nil
	}
	if almacen.resolveDrive == nil {
		return nil, nil, cifrado.ErrPrivateResources
	}
	client, err := client.WithPrivateData(almacen.repository)
	if err != nil {
		return nil, nil, err
	}
	driveScope := almacen.cuenta.PrivateStorageScope()
	driveScope.Product = "drive"
	drive, err = drive.WithPrivateData(galeriaDriveData{resolve: almacen.resolveDrive, scope: driveScope})
	if err != nil {
		return nil, nil, err
	}
	return client, drive, nil
}

func (h *photosStorage) OpenDriveFile(ctx context.Context, opening api.Apertura) error {
	current := h.presenter.Load()
	if current == nil {
		return privatePhotos.ErrNotReady
	}
	if err := current.resources.Check(ctx); err != nil {
		return err
	}
	var (
		photo api.Photo
		err   error
	)
	if current.repository != nil {
		photo, err = current.repository.PhotoForDriveFile(ctx, opening.Ref.ID)
	} else {
		// Por la API, como la web: indexar es idempotente y devuelve la foto que ya
		// hubiera de ese fichero.
		photo, err = current.client.Indexar(ctx, opening.Ref.ID)
	}
	if err != nil {
		return err
	}
	//nolint:contextcheck // The editor retains this exact presenter lifetime, never the short file-opening request.
	fyne.Do(func() {
		if h.presenter.Load() == current && current.resources.Check(current.resources.Context()) == nil {
			current.app.AbrirEditor(photo, opening.Volver)
		}
	})
	return nil
}

// claveEnDispositivo dice si la sesión tiene la clave de desbloqueo de Sytue ID en este
// dispositivo (photos.App.ClaveEnDispositivo). La copia que devuelve la sesión se borra
// enseguida: aquí solo se pregunta, y abrir la bóveda sigue siendo cosa de su puerta.
func claveEnDispositivo(s *auth.Session) bool {
	if s == nil {
		return false
	}
	clave, legado := s.Desbloqueo()
	defer func() {
		clear(clave)
		for _, l := range legado {
			clear(l.Salt)
			clear(l.IKM)
		}
	}()
	return len(clave) > 0
}
