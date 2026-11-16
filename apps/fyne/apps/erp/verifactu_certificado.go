// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"bytes"
	"path/filepath"
	"regexp"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Subir el certificado de VERI*FACTU (`AsistenteCertificadoVerifactu` de CertificadoVerifactu.tsx)
//
// La FNMT, las cámaras y los colegios entregan el certificado como `.p12`/`.pfx` con
// contraseña: eso es lo que se pide, tal cual, y el servicio lo abre, comprueba el NIF y
// sella la clave. Tres pasos del `Wizard` en un `Modal md`: el fichero (y la clave, si
// es un PEM sin ella), la contraseña con la casilla de sello y «Listo».
//
// La contraseña vive en su campo mientras el diálogo está abierto y viaja en la misma
// petición (por TLS) que el fichero; al guardar se borra y al cerrar se suelta todo.

// ExtensionesCertificado son el `accept` del fichero; ExtensionesClave, el de la clave.
var (
	ExtensionesCertificado = []string{".p12", ".pfx", ".pem", ".crt", ".cer"}
	ExtensionesClave       = []string{".pem", ".key"}
)

// Pasos del asistente.
const (
	PasoCertFichero = iota
	PasoCertConfirmar
	PasoCertListo
)

// HuecoAsistenteCert es el `gap-6` del Wizard; DescripcionCert el `mt-1.5`.
const (
	HuecoAsistenteCert float32 = 6 * 0.25 * tema.Rem
	DescripcionCert    float32 = 1.5 * 0.25 * tema.Rem
)

var clavePrivadaPEM = regexp.MustCompile(`-{5}BEGIN [A-Z ]*PRIVATE KEY-{5}`)

// ExaminarCertificado mira si el fichero es PEM (y si trae la clave) sin fiarse de la
// extensión: lo que no es PEM se trata como PKCS#12 y pide contraseña.
func ExaminarCertificado(datos []byte) (pem, conClave bool) {
	pem = bytes.Contains(datos, []byte("-----BEGIN "))
	return pem, pem && clavePrivadaPEM.Match(datos)
}

// CertificadoVF es el estado del asistente.
type CertificadoVF struct {
	tr *i18n.Translator

	Paso int
	// Fichero es el certificado elegido, en bruto; Nombre, cómo se llama en el disco.
	Fichero, Clave           []byte
	Nombre, NombreClave      string
	PEM, ConClave            bool
	Enviando                 bool
	Fallo, FalloContrasena   string
	Guardado                 *api.ERPEstadoVerifactu
	Ventana                  float32
	AlElegirFichero          func()
	AlElegirClave, AlGuardar func()
	AlCambio                 func()

	contrasena *ui.Entrada
	sello      *ui.Eleccion
}

// NuevoCertificadoVF abre el asistente vacío.
func NuevoCertificadoVF(tr *i18n.Translator) *CertificadoVF {
	c := &CertificadoVF{tr: tr}
	c.contrasena = ui.NuevaEntrada("").ConPropia(CampoFicha)
	c.contrasena.Contrasena(true)
	c.contrasena.AlCambiar(func(string) {
		// Equivocarse y volver a teclear: el error se va con lo que se escribe.
		c.FalloContrasena, c.contrasena.Invalida = "", false
		c.cambio()
	})
	c.sello = ui.NuevaCasilla(c.t("vf.cert.seal"), func(bool) {})
	return c
}

func (c *CertificadoVF) t(clave string, vars ...i18n.Vars) string {
	if len(vars) > 0 {
		return c.tr.T(clave, vars[0])
	}
	return c.tr.T(clave, nil)
}

func (c *CertificadoVF) cambio() {
	if c.AlCambio != nil {
		c.AlCambio()
	}
}

// PonerFichero es lo elegido en el selector; otro fichero olvida la clave de antes.
func (c *CertificadoVF) PonerFichero(nombre string, datos []byte) {
	c.Fichero, c.Nombre = datos, filepath.Base(nombre)
	c.PEM, c.ConClave = ExaminarCertificado(datos)
	c.Clave, c.NombreClave, c.Fallo = nil, "", ""
	c.cambio()
}

// PonerClave es la clave privada PEM de un certificado que no la trae.
func (c *CertificadoVF) PonerClave(nombre string, datos []byte) {
	c.Clave, c.NombreClave = datos, filepath.Base(nombre)
	c.cambio()
}

// FaltaClave: un PEM con el certificado solo.
func (c *CertificadoVF) FaltaClave() bool { return c.PEM && !c.ConClave }

// ListoFichero es el `canContinue` del primer paso.
func (c *CertificadoVF) ListoFichero() bool {
	return len(c.Fichero) > 0 && (!c.FaltaClave() || len(c.Clave) > 0)
}

// PuedeGuardar es el del segundo: un `.p12` no se abre sin su contraseña.
func (c *CertificadoVF) PuedeGuardar() bool {
	return c.ListoFichero() && (c.PEM || c.contrasena.Texto() != "")
}

// Peticion es lo que se manda al servicio.
func (c *CertificadoVF) Peticion() api.ERPFicheroCertificado {
	f := api.ERPFicheroCertificado{Fichero: c.Fichero, Sello: c.sello.Marcada}
	if c.FaltaClave() {
		f.Clave = c.Clave
	}
	if !c.PEM {
		f.Contrasena = c.contrasena.Texto()
	}
	return f
}

// Hecho pasa al paso final y olvida la contraseña.
func (c *CertificadoVF) Hecho(e api.ERPEstadoVerifactu) {
	c.contrasena.PonerTexto("")
	c.Guardado, c.Enviando, c.Paso = &e, false, PasoCertListo
	c.cambio()
}

// Fallido pone el error donde toca: la contraseña equivocada, en su campo y sin perder
// el paso; lo demás, arriba.
func (c *CertificadoVF) Fallido(contrasena bool, mensaje string) {
	c.Enviando = false
	if contrasena {
		c.FalloContrasena, c.contrasena.Invalida = mensaje, true
	} else {
		c.Fallo = mensaje
	}
	c.cambio()
}

// Ir cambia de paso (Atrás, Continuar o el progreso); hecho, ya no se vuelve.
func (c *CertificadoVF) Ir(paso int) {
	if c.Guardado != nil || c.Enviando {
		return
	}
	c.Paso, c.Fallo = paso, ""
	c.cambio()
}

// Cuerpo es el Wizard sin su pie: el progreso, el encabezado y el contenido (`gap-6`),
// con el error arriba (`mb-4`) como lo pinta el Modal.
func (c *CertificadoVF) Cuerpo(v tema.Variante, ancho float32) fyne.CanvasObject {
	p := &pila{}
	if c.Fallo != "" {
		o, h := cajaPeligro(c.Fallo, tema.TextoSM, v, ancho)
		p.add(o, h, 0)
	}
	titulos := []string{c.t("vf.cert.stepFile"), c.t("vf.cert.stepConfirm"), c.t("vf.cert.stepDone")}
	pasos := ui.NuevosPasos(titulos...)
	pasos.Actual, pasos.Ventana = c.Paso, c.Ventana
	pasos.RotuloIr = func(n int) string { return c.t("ui.wizard.goToStep", i18n.Vars{"n": n}) }
	if c.Guardado == nil && !c.Enviando {
		pasos.AlElegir = c.Ir
	}
	arriba := float32(0)
	if c.Fallo != "" {
		arriba = tema.Espacio(4)
	}
	p.add(pasos, ui.AltoPasos, arriba)
	enc := &pila{}
	cabeza := []string{c.t("vf.cert.fileHeading"), c.t("vf.cert.confirmHeading"), c.t("vf.cert.saved")}[c.Paso]
	addParrafo(enc, cabeza, tema.TextoXL, ui.SemiNegrita, "text", v, ancho, 0)
	if c.Paso == PasoCertFichero {
		addParrafo(enc, c.t("vf.cert.fileHint"), tema.TextoBase, ui.Normal, "text-muted", v, min(ancho, tema.Espacio(168)), DescripcionCert)
	}
	o, h := enc.objeto()
	p.add(o, h, HuecoAsistenteCert)
	o, h = c.contenido(v, ancho)
	p.add(o, h, HuecoAsistenteCert)
	o, _ = p.objeto()
	return o
}

func (c *CertificadoVF) contenido(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	p := &pila{}
	hueco := float32(0)
	add := func(o fyne.CanvasObject, h float32) {
		p.add(o, h, hueco)
		hueco = HuecoCampos
	}
	switch c.Paso {
	case PasoCertFichero:
		o, h := c.campoFichero(v, c.t("vf.cert.fileLabel"), c.Nombre, "", c.AlElegirFichero, ancho)
		add(o, h)
		if c.FaltaClave() {
			o, h := c.campoFichero(v, c.t("vf.cert.keyLabel"), c.NombreClave, c.t("vf.cert.keyHint"), c.AlElegirClave, ancho)
			add(o, h)
		}
	case PasoCertConfirmar:
		if !c.PEM {
			c.contrasena.Ancho = ancho
			c.contrasena.Inerte(c.Enviando)
			if c.FalloContrasena != "" {
				g := ui.GrupoCampo(c.t("vf.cert.passwordLabel"), false, c.contrasena, "", c.FalloContrasena)
				add(g, g.MinSize().Height)
			} else {
				o, h := campoConPista(v, c.t("vf.cert.passwordLabel"), c.contrasena, c.t("vf.cert.passwordHint"), ancho)
				add(o, h)
			}
		}
		if c.Enviando {
			c.sello.Disable()
		} else {
			c.sello.Enable()
		}
		c.sello.AnchoEtiqueta = ancho
		add(c.sello, c.sello.AltoPara(ancho))
		pr := ui.NuevoParrafo(c.t("vf.cert.privacy"), relajadoXS, ui.Normal, "text-muted", v)
		add(pr, ui.AltoParrafo(pr, ancho))
	default:
		nif := ""
		if c.Guardado != nil {
			nif = c.Guardado.CertificadoNIF
		}
		pr := ui.NuevoParrafo(c.t("vf.cert.savedBody", i18n.Vars{"nif": nif}), tema.TextoSM, ui.Normal, "text-muted", v)
		add(pr, ui.AltoParrafo(pr, ancho))
	}
	return p.objeto()
}

// campoFichero es el `<input type="file">`: el rótulo, «Elegir…» con el nombre de lo
// elegido al lado y, si la hay, la pista. Sin selector del sistema no hay botón.
func (c *CertificadoVF) campoFichero(v tema.Variante, rotulo, nombre, pista string, alElegir func(), ancho float32) (fyne.CanvasObject, float32) {
	l := &lienzo{}
	l.texto(ui.Texto(rotulo, tema.TextoSM, ui.Medio, tema.Color("text", v)), 0, 0)
	y := tema.TextoSM.Linea + HuecoUno
	b := ui.NuevoBoton(c.t("relation.choose"), alElegir).Con(ui.Secundario, ui.TallaSM)
	b.Icono, b.TamIcono = iconos.Subir, 16
	b.Inerte = alElegir == nil || c.Enviando
	mb := b.MinSize()
	l.poner(b, 0, y, mb.Width, mb.Height)
	if nombre != "" {
		n := ui.Texto(nombre, tema.TextoSM, ui.Normal, tema.Color("text-muted", v))
		ui.Truncar(n, nombre, tema.TextoSM, ui.Normal, max(ancho-mb.Width-HuecoDos, 0))
		l.texto(n, mb.Width+HuecoDos, y+(mb.Height-tema.TextoSM.Linea)/2)
	}
	y += mb.Height
	if pista != "" {
		y += HuecoUno
		hp := ui.NuevoParrafo(pista, tema.TextoXS, ui.Normal, "text-subtle", v)
		h := ui.AltoParrafo(hp, ancho)
		l.poner(hp, 0, y, ancho, h)
		y += h
	}
	return l.objeto(y)
}

// Pie son las acciones del Wizard en el pie del Modal: «Cancelar», «Atrás» y
// «Continuar» (o «Guardar certificado»), juntas a la derecha como en el resto de
// diálogos de la app. En el paso final la web
// pone «Cerrar» en el contenido y deja el pie vacío; aquí va en el pie, que un diálogo
// de la app sin acciones abajo deja un filete suelto.
func (c *CertificadoVF) Pie(alCerrar func()) fyne.CanvasObject {
	if c.Paso == PasoCertListo {
		cerrar := ui.NuevoBoton(c.t("action.close"), alCerrar).Con(ui.Secundario, ui.TallaMD)
		return container.New(ui.Fila{Hueco: HuecoDos}, cerrar)
	}
	cancelar := ui.NuevoBoton(c.t("ui.wizard.cancel"), alCerrar).Con(ui.Fantasma, ui.TallaMD)
	cancelar.Inerte = c.Enviando
	objs := []fyne.CanvasObject{cancelar}
	if c.Paso > PasoCertFichero {
		atras := ui.NuevoBoton(c.t("ui.wizard.back"), func() { c.Ir(c.Paso - 1) }).Con(ui.Secundario, ui.TallaMD)
		atras.Icono, atras.TamIcono, atras.Inerte = iconos.ChevronIzquierda, 16, c.Enviando
		objs = append(objs, atras)
	}
	seguir := ui.NuevoBoton(c.t("ui.wizard.next"), func() { c.Ir(c.Paso + 1) }).Con(ui.Primario, ui.TallaMD)
	seguir.Inerte = !c.ListoFichero()
	if c.Paso == PasoCertConfirmar {
		seguir.Texto, seguir.AlPulsar = c.t("vf.cert.save"), c.AlGuardar
		seguir.Inerte, seguir.Cargando = !c.PuedeGuardar() || c.Enviando, c.Enviando
	}
	objs = append(objs, seguir)
	return container.New(ui.Fila{Hueco: HuecoDos}, objs...)
}

// cajaPeligro es el `<p role="alert" class="bg-danger-bg text-danger-text rounded-md
// p-3">` de arriba de un asistente, con el texto a esa escala.
func cajaPeligro(texto string, escala tema.Escalon, v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	dentro := max(ancho-2*tema.Espacio(3), 0)
	a := ui.NuevoParrafo(texto, escala, ui.Normal, "danger-text", v)
	h := ui.AltoParrafo(a, dentro) + 2*tema.Espacio(3)
	l := &lienzo{}
	l.caja(ui.NuevaCaja(tema.Color("danger-bg", v), sinColor, tema.RadioMD), ancho, h)
	l.poner(a, tema.Espacio(3), tema.Espacio(3), dentro, h-2*tema.Espacio(3))
	return l.objeto(h)
}
