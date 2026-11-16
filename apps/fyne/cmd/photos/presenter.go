// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"context"
	"errors"
	"runtime"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/apps/photos"
	"git.sytue.eu/sytue/one/apps/fyne/cifrado"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/carrete"
	privateDrive "git.sytue.eu/sytue/one/apps/nucleo/drive"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	privatePhotos "git.sytue.eu/sytue/one/apps/nucleo/photos"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

func newPhotosPresenter(application fyne.App, window fyne.Window, suite *sesion.Suite, translate *i18n.Translator, resources *cifrado.Resources[*mx.Client], repository *privatePhotos.Repository, resolveDrive func(context.Context) (*privateDrive.Repository, error)) (*photosPresenter, error) {
	client, err := api.NewPhotos(suite.URL("photos"), resources.HTTP(), nil).WithPrivateData(repository)
	if err != nil {
		return nil, err
	}
	thumbnails, large := photos.NuevaFototeca(), photos.NuevaFototeca()
	large.Variante = photos.VarianteGrande
	view := photos.Vista{T: translate, Teca: thumbnails, TecaGrande: large, Ventana: anchoInicial, Variante: tema.De(application.Settings().ThemeVariant())}
	presenter := photos.Nueva(client, view, window)
	driveScope := resources.PrivateStorageScope()
	driveScope.Product = "drive"
	presenter.Drive, err = api.NewDrive(suite.URL("drive"), resources.HTTP(), nil).WithPrivateData(galeriaDriveData{resolve: resolveDrive, scope: driveScope})
	if err != nil {
		presenter.Cerrar()
		return nil, err
	}
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
	presenter.CambiarSesion(resources.Identity.Issuer + "\x00" + resources.Identity.Subject + "\x00" + resources.Identity.Organization)
	presenter.Arrancar()
	return &photosPresenter{app: presenter, repository: repository, resources: resources}, nil
}

func (h *photosStorage) OpenDriveFile(ctx context.Context, opening api.Apertura) error {
	current := h.presenter.Load()
	if current == nil {
		return privatePhotos.ErrNotReady
	}
	if err := current.resources.Check(ctx); err != nil {
		return err
	}
	photo, err := current.repository.PhotoForDriveFile(ctx, opening.Ref.ID)
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
