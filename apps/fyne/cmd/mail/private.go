// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"errors"
	"log/slog"

	"fyne.io/fyne/v2"

	mailUI "git.sytue.eu/sytue/one/apps/fyne/apps/mail"
	"git.sytue.eu/sytue/one/apps/fyne/cifrado"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/ingress"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
)

// mailStorage es el correo privado de la app: en la bóveda directa o en Matrix,
// según la autoridad del correo de cada cuenta (cifrado.AplicacionCorreo, ADR
// 0087). La vista y el receptor son los mismos en los dos.
type mailStorage struct {
	application *cifrado.AplicacionCorreo
	buzon       *mailUI.Buzon
	identity    string
	onMount     func(*mailUI.Buzon)
}

func newMailStorage(s *sesion.Suite, application fyne.App, window fyne.Window, translator *i18n.Translator) (*mailStorage, error) {
	host := &mailStorage{}
	consent := cifrado.NewIngressConsent(s.URL("rooms"))
	translate := func(key string) string { return translator.T(key, nil) }
	mount := func(ctx context.Context, resources cifrado.Cuenta, repository *cifrado.MailApplicationRepository) (cifrado.ApplicationView, error) {
		return host.mount(ctx, s, application, translator, resources, repository)
	}
	privateApplication, err := cifrado.NuevaAplicacionCorreo(cifrado.AplicacionCorreoConfig{
		Matrix: cifrado.PrivateApplicationConfig[*cifrado.MailApplicationRepository]{
			Suite: s, Application: application, Window: window, Product: "mail", Translate: translate,
			BeforeOpen: func(ctx context.Context, resources *cifrado.Resources[*mx.Client], approve cifrado.StorageApproval) error {
				return consent.BeforeOpen(ctx, resources, approve)
			},
			Build: func(ctx context.Context, resources *cifrado.Resources[*mx.Client]) (cifrado.ApplicationPlan[*cifrado.MailApplicationRepository], error) {
				authority, err := consent.Authority(ctx, resources)
				if err != nil {
					return cifrado.ApplicationPlan[*cifrado.MailApplicationRepository]{}, err
				}
				return cifrado.NewMailApplicationPlan(ctx, resources, cifrado.MailApplicationConfig{
					Gateway:   api.NewDrive(s.URL("drive"), resources.HTTP(), nil),
					Source:    api.NewMail(s.URL("mail"), resources.HTTP(), nil),
					Rooms:     api.NewRooms(s.URL("rooms"), resources.HTTP(), nil),
					Authority: authority,
				})
			},
			Mount: func(ctx context.Context, resources *cifrado.Resources[*mx.Client], repository *cifrado.MailApplicationRepository) (cifrado.ApplicationView, error) {
				return mount(ctx, resources, repository)
			},
		},
		MontarBoveda: mount,
		PasarABoveda: func(confirmar, seguir func()) fyne.CanvasObject {
			return mailUI.NuevaPasarABoveda(translator, confirmar, seguir).Objeto()
		},
		OfertaTraspaso: func(confirmar func()) fyne.CanvasObject {
			return mailUI.NuevaOfertaTraspaso(translator, confirmar).Objeto()
		},
		SinComprobar: func(reintentar func()) fyne.CanvasObject {
			return mailUI.NuevaCorreoSinComprobar(translator, reintentar).Objeto()
		},
	})
	if err != nil {
		return nil, err
	}
	host.application = privateApplication
	return host, nil
}

// mount monta el buzón sobre el correo privado ya preparado de una cuenta, en la
// bóveda o en Matrix, y arranca su receptor.
func (h *mailStorage) mount(ctx context.Context, s *sesion.Suite, application fyne.App, translator *i18n.Translator, resources cifrado.Cuenta, repository *cifrado.MailApplicationRepository) (cifrado.ApplicationView, error) {
	data, err := repository.ReadyRepository(ctx)
	if err != nil {
		return cifrado.ApplicationView{}, err
	}
	service, err := api.NewMail(s.URL("mail"), resources.HTTP(), nil).WithPrivateData(data)
	if err != nil {
		return cifrado.ApplicationView{}, err
	}
	//nolint:contextcheck // Construction installs UI callbacks; account IO is bound by CambiarSesion below before mounting.
	screen := mailUI.NuevaPantalla(translator, s.Idioma())
	//nolint:contextcheck // Constructors install deferred UI events; CambiarSesion owns their authenticated lifetime.
	box := mailUI.NuevoBuzon(screen, service)
	box.AlCambiarDatos = s.AvisarWidget
	box.Abridor = application
	box.Entrar = s.Entrar
	// Incoming messages are processed by the authenticated private receiver,
	// not by the legacy plaintext SSE endpoint.
	box.Fondo = func(func()) {}
	scope := resources.PrivateStorageScope()
	identity := s.URL("id") + "\x00" + scope.Subject + "\x00" + scope.Tenant
	box.CambiarSesion(ctx, identity)
	h.buzon, h.identity = box, identity
	if h.onMount != nil {
		h.onMount(box)
	}

	receiverCtx, stopReceiver := context.WithCancel(ctx)
	receiverDone := make(chan struct{})
	go func() {
		defer close(receiverDone)
		err := repository.RunReceiver(receiverCtx, func(result ingress.PollResult, _ error) {
			if result.Completed == 0 {
				return
			}
			fyne.Do(func() {
				if receiverCtx.Err() == nil && h.buzon == box {
					//nolint:contextcheck // Refrescar uses the current account context retained by CambiarSesion, after receiverCtx is checked.
					box.Refrescar()
				}
			})
		})
		if err != nil && !errors.Is(err, context.Canceled) && receiverCtx.Err() == nil {
			slog.Warn("mail: private ingress receiver stopped")
		}
	}()
	return cifrado.ApplicationView{
		Object: screen.Objeto(),
		Detach: func() {
			stopReceiver()
			box.Parar()
			if h.buzon == box {
				h.buzon, h.identity = nil, ""
			}
		},
		Join: func(ctx context.Context) error {
			select {
			case <-receiverDone:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}, nil
}

func (h *mailStorage) Current(identity string) *mailUI.Buzon {
	if h == nil || h.identity != identity {
		return nil
	}
	return h.buzon
}
