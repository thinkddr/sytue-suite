// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

import (
	"context"
	"log/slog"
	"path/filepath"
	"runtime"

	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// NewNativeFactory supplies the suite's maintained work witness, registration,
// journal and bound HTTP adapter. Unsupported native shared storage fails closed
// during Capture, before any device setup or historical source is frozen.
func NewNativeFactory(suite *sesion.Suite, product string) (Factory[*mx.Client], error) {
	if suite == nil || suite.Sesion == nil || !storage.Product(product) {
		return Factory[*mx.Client]{}, ErrPrivateResources
	}
	consumer, err := suite.ConsumidorAlmacenamiento(product)
	if err != nil {
		return Factory[*mx.Client]{}, err
	}
	factory := Factory[*mx.Client]{
		NativeConsumer: consumer,
		Product:        product, CaptureWork: suite.TestigoTrabajo, RegisterWork: suite.RegistrarTrabajoPrivado, BindHTTP: suite.SesionVinculadaPrivada,
		RegisterTransition: suite.RegistrarPreparadorTransicionPrivada,
		CurrentIdentity:    func() (auth.IdentitySnapshot, bool) { return suite.Sesion.SnapshotIdentity(), suite.Estado().Dentro() },
	}
	factory.OpenJournal = func(ctx context.Context) (*offline.Store, error) {
		application, _ := ctx.Value(applicationSessionKey{}).(*ApplicationSession)
		if application == nil || application.scope.Product != product {
			return nil, ErrPrivateResources
		}
		if err := application.Check(ctx); err != nil {
			return nil, err
		}
		return suite.AlmacenPrivadoAplicacion(ctx, product, application.witness)
	}
	factory.configurePanel = func(panel *Panel) error {
		return configureNativePanel(suite, product, consumer, runtime.GOOS == "ios" || runtime.GOOS == "android", panel)
	}

	return factory, nil
}

func configureNativePanel(suite *sesion.Suite, product, consumer string, mobile bool, panel *Panel) error {
	if panel == nil || storage.NativeReturnApp(product, consumer) == "" || (panel.Base.NativeConsumer != "" && panel.Base.NativeConsumer != consumer) {
		return ErrPrivateResources
	}
	panel.nativeRecovery = product == "drive"
	// Todos los productos de almacenamiento guardan su clave envuelta en Rooms, por
	// la cuenta de Matrix de cada panel, con la clave de desbloqueo de la sesión (ADR
	// 0084). Un solo sitio: nadie lo copia.
	if panel.Custodio == nil && suite != nil {
		panel.Custodio = custodiaDeSesion{roomsURL: suite.URL}
	}
	if panel.Desbloqueo == nil {
		panel.ConSesion(suite)
	}
	if consumer != "" {
		base, err := ajustes.Carpeta()
		if err != nil {
			return err
		}
		panel.Base.NativeConsumer = consumer
		panel.Base.StoreDir = filepath.Join(base, "matrix-native", consumer)
	}
	// All mobile products use the native broker. Consumers retain that immutable
	// app/data binding on desktop too; own desktop products keep loopback OAuth.
	if mobile || consumer != "" {
		panel.Login = func(ctx context.Context, cfg mx.Config) (*mx.Client, error) {
			bound, err := SessionHTTP(ctx)
			if err != nil {
				return nil, err
			}
			return mx.LoginNative(ctx, cfg, api.NewRooms(suite.URL("rooms"), bound, nil), suite.MatrixBrowserReturn)
		}
	}
	return nil
}

// ConSesion conecta el panel con la sesión de la suite (ADR 0084): la clave de
// desbloqueo que trajo el inicio de sesión en Sytue ID, guardada con la sesión que las
// apps nativas del dispositivo comparten (un inicio de sesión desbloquea todas), y
// volver a entrar en Sytue ID cuando falta.
func (c *Panel) ConSesion(suite *sesion.Suite) {
	if c == nil || suite == nil || suite.Sesion == nil {
		return
	}
	c.Desbloqueo = func() Material {
		clave, legado := suite.Sesion.Desbloqueo()
		return Material{Clave: clave, Legado: legado}
	}
	c.OlvidarLegado = func(ctx context.Context) {
		if err := suite.Sesion.OlvidarLegado(ctx); err != nil {
			slog.Warn("cifrado: no se pudo retirar lo que abría los envoltorios de la ADR 0082", "error", err)
		}
	}
	c.IniciarSesion = suite.EntrarParaDesbloquear
}

type nativeRecoverySecrets struct{}

func (nativeRecoverySecrets) Read(key string) ([]byte, error) { return auth.LeerSecreto(key) }
func (nativeRecoverySecrets) Write(key string, value []byte) error {
	return auth.GuardarSecreto(key, value)
}
func (nativeRecoverySecrets) Delete(key string) error { return auth.BorrarSecreto(key) }
