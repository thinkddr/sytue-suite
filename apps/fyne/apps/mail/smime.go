// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// # La firma del correo: bajar el ancla y subir el certificado propio
//
// «Certificado de firma del correo» baja el ancla S/MIME de la organización (en la web,
// un enlace de descarga); «Instalar tu propio certificado» es `SmimeIdentityModal`: el
// PEM de una autoridad pública —elegido de un fichero o pegado— para que la firma se
// valide sola fuera de Sytue.

// Firma es lo que estas dos entradas le piden al servicio. Lo cumple *api.Mail.
type Firma interface {
	SmimeAnchor(ctx context.Context) ([]byte, error)
	SetSmimeIdentity(ctx context.Context, pem string) error
}

var _ Firma = (*api.Mail)(nil)

// NombreAnclaSmime es el `filename` con el que la web la descarga.
const NombreAnclaSmime = "sytue-smime-ca.pem"

// bajarAncla es `bajarAncla` de la web: el PEM a Descargas, y la carpeta abierta. Una
// organización sin autoridad todavía (404: se crea con el primer correo firmado) y un
// fallo de red se dicen con un aviso, cada uno con el suyo.
func (b *Buzon) bajarAncla() {
	b.Lanzar(func() {
		pem, err := b.S.SmimeAnchor(b.ctx)
		if err != nil {
			fyne.Do(func() {
				t := b.P.T
				if api.EsNoEncontrado(err) {
					b.P.Avisar(ui.AvisoAdvertencia, t.T("smime.anchorMissing", nil), t.T("smime.anchorMissingDescription", nil))
					return
				}
				b.P.Avisar(ui.AvisoPeligro, t.T("smime.anchorFailed", nil), "")
			})
			return
		}
		b.descargar(NombreAnclaSmime, bytes.NewReader(pem))
	})
}

type dialogoSmime struct {
	b        *Buzon
	d        *ui.Dialogo
	pem      *ui.AreaTexto
	instalar *ui.Boton
	cancelar *ui.Boton
	caja     *fyne.Container
	ancho    float32
	error    string
}

// verSmime abre «Instalar tu propio certificado».
func (b *Buzon) verSmime() *dialogoSmime {
	lienzo := b.P.Lienzo()
	if lienzo == nil {
		return nil
	}
	t := b.P.T
	g := &dialogoSmime{b: b, ancho: AnchoCuerpoDialogo(ui.DialogoMD, lienzo.Size().Width)}
	// `rows={8}` en `text-xs font-mono`: 8 × 14 + `py-2` + borde = 128, medido en la web.
	g.pem = ui.NuevaAreaTexto(t.T("smime.pastePlaceholder", nil))
	g.pem.Filas, g.pem.Ancho = 8, g.ancho
	g.pem.ConPropia(ui.CampoPropio{MargenX: tema.Espacio(3), MargenY: tema.Espacio(2), Texto: tema.TextoXS, Mono: true})
	g.pem.AlCambiar(func(string) { g.error = ""; g.pintar() })
	elegir := ui.NuevoBoton(t.T("smime.chooseFile", nil), g.elegirFichero).Con(ui.Secundario, ui.TallaMD)
	g.caja = container.New(ui.Columna{Hueco: tema.Espacio(2)}, container.New(anchoLibre{}, elegir), g.pem)
	g.cancelar = ui.NuevoBoton(t.T("common.cancel", nil), nil).Con(ui.Secundario, ui.TallaMD)
	g.instalar = ui.NuevoBoton(t.T("smime.install", nil), g.enviar).Con(ui.Primario, ui.TallaMD)
	cuerpo := container.New(ui.Columna{Hueco: tema.Espacio(4)},
		ui.NuevaAyudaEnLinea(t.T("smime.installIntroBody", nil)).ConTitulo(t.T("smime.installIntroTitle", nil)),
		g.caja)
	g.d = ui.NuevoDialogo(t.T("smime.installTitle", nil), cuerpo).
		Con(ui.DialogoMD, container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, g.cancelar, g.instalar))
	g.cancelar.AlPulsar = g.d.Cerrar
	g.d.RotuloCerrar = t.T("ui.close", nil)
	g.d.Mostrar(lienzo)
	// Después de mostrarlo: la ayuda solo sabe cuántas líneas ocupa cuando ya tiene
	// ancho, y el diálogo se mide con ella.
	g.pintar()
	return g
}

// pintar pone la pista (o el error) y el estado de «Instalar».
func (g *dialogoSmime) pintar() {
	t := g.b.P.T
	pista := ui.NuevaPistaCampo(t.T("smime.hint", nil))
	if g.error != "" {
		pista = ui.NuevaPistaCampo(g.error).ConTono(ui.AyudaPeligro)
	}
	pista.Ancho = g.ancho
	g.caja.Objects = append(g.caja.Objects[:2], pista)
	g.caja.Refresh()
	g.instalar.Inerte = strings.TrimSpace(g.pem.Texto()) == ""
	g.instalar.Refresh()
	if g.d != nil {
		g.d.Recolocar()
	}
}

// elegirFichero es el `<input type="file">`: el primero elegido, leído como texto.
func (g *dialogoSmime) elegirFichero() {
	if g.b.ElegirFicheros == nil {
		return
	}
	pedir(g.b, func(context.Context) (string, error) {
		rutas, err := g.b.ElegirFicheros()
		if err != nil || len(rutas) == 0 {
			return "", err
		}
		datos, err := os.ReadFile(rutas[0])
		return string(datos), err
	}, func(pem string, err error) {
		if err != nil || pem == "" {
			return
		}
		g.pem.PonerTexto(pem)
		g.error = ""
		g.pintar()
	})
}

// enviar es `instalar`: el motivo del servicio, si lo da; si no, el texto de la web.
func (g *dialogoSmime) enviar() {
	pem := g.pem.Texto()
	if g.instalar.Cargando || strings.TrimSpace(pem) == "" {
		return
	}
	g.instalar.Cargando, g.cancelar.Inerte, g.error = true, true, ""
	g.instalar.Refresh()
	g.cancelar.Refresh()
	pedir(g.b, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, g.b.S.SetSmimeIdentity(ctx, pem)
	}, func(_ struct{}, err error) {
		g.instalar.Cargando, g.cancelar.Inerte = false, false
		g.instalar.Refresh()
		g.cancelar.Refresh()
		if err != nil {
			g.error = g.b.P.T.T("smime.installFailed", nil)
			var e *api.Error
			if errors.As(err, &e) && e.Message() != "" {
				g.error = e.Message()
			}
			g.pintar()
			return
		}
		g.b.avisar(ui.AvisoExito, "smime.installed", nil)
		g.d.Cerrar()
	})
}
