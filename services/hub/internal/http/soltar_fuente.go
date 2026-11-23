// SPDX-License-Identifier: Apache-2.0
package hubhttp

import (
	"context"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/services/hub/internal/domain"
	"git.sytue.eu/sytue/one/services/hub/internal/notify"
)

// avisoPropio deja el aviso de «soltar la fuente» de Sytue One en la campana
// sin salir del proceso: el Hub es quien guarda los avisos.
func avisoPropio(svc *notify.Service) func(context.Context, hubnotify.Notification) error {
	if svc == nil {
		return nil
	}
	return func(ctx context.Context, n hubnotify.Notification) error {
		_, err := svc.Create(ctx, domain.NewNotification{
			Subject: n.Subject, Source: domain.Source(n.Source), Type: n.Type,
			Level: domain.NotificationLevel(n.Level), Title: n.Title, Params: n.Params,
		})
		return err
	}
}
