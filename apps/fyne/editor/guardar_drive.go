// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"context"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/office/guardardrive"
)

// PlazoGuardarEnDrive acota «Guardar en Drive»: exportar, cifrar y subir un documento
// grande a la bóveda tarda más que una petición corriente.
const PlazoGuardarEnDrive = 3 * time.Minute

// GuardarEnDrive es lo que «Guardar en Drive» necesita del documento abierto (el
// `saveToDrive` de `useDocument.ts`). Las reglas —dónde va, cifrado o no, cuándo se
// ancla— son las de `guardardrive`, las mismas que la web.
type GuardarEnDrive struct {
	// Destinos son los dos Drive de la cuenta (el del servidor y el de la bóveda). nil:
	// la entrada no sale (sin cuenta del servicio, o con los datos privados en Matrix,
	// donde no hay servicio al que anclar). Anclar lo pone esto, con el id.
	Destinos *guardardrive.Destinos
	ID       string
	Doc      guardardrive.Documento
	// Antes prepara EN EL HILO DE LA INTERFAZ el guardado de lo pendiente y devuelve lo
	// que lo hace (o nil si no hay nada): el servicio exporta desde SU estado.
	Antes func() func() error
	// Recomponer repinta el marco cuando la entrada aparece.
	Recomponer func()
	// Avisar enseña un aviso con su texto en la pila de la ventana.
	Avisar func(tono ui.TonoAviso, titulo, texto string)
}

// EngancharGuardarEnDrive pone [Contexto.GuardarEnDrive] si toca: con un ancla en
// seguida; sin ella, solo si el Drive de quien edita está en la bóveda, lo que se
// pregunta fuera del hilo y sin claves. Pulsarla no abre la bóveda por su cuenta si no
// la ha abierto ya la persona: quien da `Destinos.Boveda` ofrece desbloquearla.
func (s Servicio) EngancharGuardarEnDrive(ctx *Contexto, g GuardarEnDrive) {
	ctx.GuardarEnDrive = nil
	if g.Destinos == nil || s.Svc == nil || g.ID == "" {
		return
	}
	destinos := *g.Destinos
	destinos.Anclar = func(c context.Context, nueva, antes string) error {
		return s.Svc.AnclarEnBoveda(c, g.ID, nueva, antes)
	}
	ancla, guardando := g.Doc.Ancla, false
	pulsar := func() {
		if guardando {
			return
		}
		guardando = true
		var antes func() error
		if g.Antes != nil {
			antes = g.Antes()
		}
		doc := g.Doc
		doc.Ancla = ancla
		if ctx.Titulo != "" {
			doc.Titulo = ctx.Titulo // el de ahora: se ha podido renombrar
		}
		s.lanzar(func() {
			c, cancelar := context.WithTimeout(context.Background(), max(s.Plazo, PlazoGuardarEnDrive))
			defer cancelar()
			var err error
			if antes != nil {
				err = antes()
			}
			var r guardardrive.Resultado
			if err == nil {
				r, err = guardardrive.GuardarDocumento(c, destinos, doc, func(c context.Context, formato string) ([]byte, error) {
					return s.Svc.Export(c, g.ID, formato)
				})
			}
			fyne.Do(func() {
				guardando = false
				if err == nil {
					ancla = r.AnclaDelDocumento
				}
				s.avisarDrive(g.Avisar, r, err)
			})
		})
	}
	if guardardrive.Ofrecer(ancla, false) {
		ctx.GuardarEnDrive = pulsar
		return
	}
	if destinos.DriveEnBoveda == nil {
		return
	}
	s.lanzar(func() {
		c, cancelar := s.ctx()
		defer cancelar()
		enBoveda, err := destinos.DriveEnBoveda(c)
		if err != nil || !enBoveda {
			return // ante la duda, como la web: el editor sigue como siempre
		}
		fyne.Do(func() {
			ctx.GuardarEnDrive = pulsar
			if g.Recomponer != nil {
				g.Recomponer()
			}
		})
	})
}

// avisarDrive dice cómo fue, con las frases de la web; las de la bóveda dicen que se
// cifró en ESTE dispositivo.
func (s Servicio) avisarDrive(avisar func(ui.TonoAviso, string, string), r guardardrive.Resultado, err error) {
	if avisar == nil {
		return
	}
	t := func(clave string, v i18n.Vars) string {
		if s.T == nil {
			return clave
		}
		return s.T(clave, v)
	}
	nombre := i18n.Vars{"name": r.Nombre}
	switch clave := guardardrive.Motivo(err); {
	case err == nil && r.Cifrado:
		avisar(ui.AvisoExito, t("doc.msg.savedToDriveEncrypted", nil), t("doc.msg.savedToDriveEncryptedOnDevice", nombre))
	case err == nil:
		avisar(ui.AvisoExito, t("doc.msg.savedToDrive", nil), t("doc.msg.savedToDriveIn", nombre))
	case clave == "doc.msg.saveToDriveVault":
		avisar(ui.AvisoAdvertencia, t(clave, nil), t("doc.msg.saveToDriveVault.detail", nil))
	case clave == "doc.msg.saveToDriveFailed":
		avisar(ui.AvisoPeligro, t(clave, nil), "")
	default:
		avisar(ui.AvisoAdvertencia, t(clave, nil), "")
	}
}

// DocumentoDrive es el guardardrive.Documento de una ficha del servicio.
func DocumentoDrive(doc api.Document, rol string, tipo Tipo) guardardrive.Documento {
	return guardardrive.Documento{Ancla: doc.SourceRef, EsDueno: rol == "owner", Titulo: doc.Title, Kind: tipo.Kind()}
}
