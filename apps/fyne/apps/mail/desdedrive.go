// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"slices"
	"sync"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// # Llegar al redactor desde otra app
//
// «Adjuntar a un correo» de Sytue Drive —y el ERP, que deja escrito el correo de una
// factura— abren Sytue Mail por su esquema con `action=compose` y lo que haya que poner:
// destinatario, asunto, cuerpo y ficheros de Drive (`api.LeerRedactar`). Es el
// `useEffect` de MailApp.tsx que atiende ese enlace. Los ficheros llegan como los de
// «Adjuntar desde Drive» (elegirdrive.go): por REFERENCIA, en fichas sin tamaño.

// Resolutor es lo que hace falta para pintar un fichero que llega solo con su id. Lo
// cumple *api.Drive, que es el mismo de `ConDrive`.
type Resolutor interface {
	File(ctx context.Context, id string) (api.File, error)
}

var _ Resolutor = (*api.Drive)(nil)

// topeDrive es el de `/api/drive-files`: lo que tiene sentido adjuntar de una vez, y sin
// tope un enlace fabricado a mano encargaría cientos de consultas con un clic.
const topeDrive = 20

// RedactarDesde abre el redactor con lo que trae el enlace. Los ids de Drive se resuelven
// antes a nombre y enlace con el token de quien abre, en paralelo y dejando fuera los que
// ya no existen o no puede ver. Si no se resuelve NINGUNO, no se abre un redactor vacío
// fingiendo que funcionó: se dice que no se pudo (`compose.driveIntentFailed`).
func (b *Buzon) RedactarDesde(r api.Redactar) {
	br := Borrador{Para: r.Para, Asunto: r.Asunto, Cuerpo: ConFirma(r.Cuerpo, b.firma, false)}
	if len(r.Drive) == 0 {
		b.abrirRedactor(br)
		return
	}
	res, ok := b.Explorador.(Resolutor)
	if !ok || b.URLDrive == "" {
		b.avisar(ui.AvisoPeligro, "compose.driveIntentFailed", nil)
		return
	}
	ids := r.Drive[:min(len(r.Drive), topeDrive)]
	pedir(b, func(ctx context.Context) ([]EnlaceDrive, error) {
		out := make([]EnlaceDrive, len(ids))
		var wg sync.WaitGroup
		for i, id := range ids {
			wg.Go(func() {
				if f, err := res.File(ctx, id); err == nil {
					out[i] = EnlaceDrive{Nombre: f.Name, URL: EnlaceDeFichero(b.URLDrive, id)}
				}
			})
		}
		wg.Wait()
		return slices.DeleteFunc(out, func(e EnlaceDrive) bool { return e.URL == "" }), nil
	}, func(ds []EnlaceDrive, _ error) {
		if len(ds) == 0 {
			b.avisar(ui.AvisoPeligro, "compose.driveIntentFailed", nil)
			return
		}
		br.Drive = ds
		b.abrirRedactor(br)
	})
}
