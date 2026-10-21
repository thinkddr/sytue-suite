// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package compartir es «Compartir con una persona» para las apps nativas que lo tienen
// (Photos y Notepads): el espejo de `CompartirConPersonas.tsx` de @sytue/shell, talla
// lg. Se escribe el correo, se elige el nivel —si el recurso tiene más de uno— y debajo
// está con quién ya está compartido, con «Revocar».
//
// La persona se ESCRIBE por su correo y se GUARDA por su `sub`: la traducción la hace
// Sytue ID con el Directorio (`directory:resolve`), como Drive, y el servicio lo vuelve a
// comprobar antes de guardarlo. El contrato con el servicio es `api.Personas`.
//
// Un correo que no es de nadie de la organización es información, no un fallo: se queda
// escrito junto al campo, en vez de irse en un aviso.
package compartir

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Medidas de CompartirConPersonas.tsx (las mismas que la parte de personas del
// `ShareDialog` de Drive).
const (
	// AnchoNivel es el `w-44` del selector de nivel; MinCorreo, el `min-w-56` del correo.
	AnchoNivel float32 = 44 * 0.25 * tema.Rem
	MinCorreo  float32 = 56 * 0.25 * tema.Rem
	// Plazo acota cada llamada al servicio.
	Plazo = 15 * time.Second
)

// Opciones es lo que necesita el diálogo.
type Opciones struct {
	T *i18n.Translator
	V tema.Variante
	// Nombre de lo que se comparte, para el título.
	Nombre string
	// Niveles que admite el recurso, en orden (`api.NivelVer`, `api.NivelEditar`).
	// Con uno solo no se pregunta.
	Niveles    []string
	Personas   api.Personas
	Directorio *api.Directorio
	// Avisar enseña un aviso de la app; exito false es de peligro. nil: sin avisos.
	Avisar func(exito bool, texto string)
	// Lanzar corre una llamada fuera del hilo de la interfaz; nil: una rutina nueva.
	// EnHilo vuelve a él; nil: fyne.Do. Los guardianes ponen los dos en línea.
	Lanzar, EnHilo func(func())
}

// Dialogo es el diálogo abierto.
type Dialogo struct {
	o Opciones
	d *ui.Dialogo

	personas    []api.PersonaCompartida
	caras       map[string]api.Persona
	falloLista  bool
	errorCorreo string
	quitando    string
	enviando    bool

	correo *ui.Entrada
	nivel  *ui.Selector
	anadir *ui.Boton
	cuerpo *fyne.Container
}

// Abrir enseña el diálogo sobre `c` y empieza a cargar la lista.
func Abrir(c fyne.Canvas, o Opciones) *Dialogo {
	if o.Lanzar == nil {
		o.Lanzar = func(f func()) { go f() }
	}
	if o.EnHilo == nil {
		o.EnHilo = fyne.Do
	}
	if len(o.Niveles) == 0 {
		o.Niveles = []string{api.NivelVer}
	}
	d := &Dialogo{o: o, cuerpo: container.NewStack(), caras: map[string]api.Persona{}}
	d.correo = ui.NuevaEntrada(d.t("sharePeople.placeholder"))
	d.correo.AlCambiar(func(string) {
		if d.errorCorreo != "" {
			d.errorCorreo = ""
			d.pintar()
		}
		d.pintarBoton()
	})
	d.correo.AlTeclear = func(k *fyne.KeyEvent) bool {
		if k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter {
			d.Compartir()
			return true
		}
		return false
	}
	rotulos := make([]string, len(o.Niveles))
	for i, n := range o.Niveles {
		rotulos[i] = d.rotuloNivel(n)
	}
	d.nivel = ui.NuevoSelector(rotulos, nil)
	d.anadir = ui.NuevoBoton(d.t("sharePeople.add"), d.Compartir)
	listo := ui.NuevoBoton(d.t("sharePeople.done"), func() { d.d.Cerrar() }).Con(ui.Primario, ui.TallaMD)
	d.d = ui.NuevoDialogo(o.T.T("sharePeople.title", i18n.Vars{"name": o.Nombre}), d.cuerpo).Con(ui.DialogoLG, listo)
	d.d.RotuloCerrar = d.t("ui.close")
	d.pintar()
	if c != nil {
		d.d.Mostrar(c)
	}
	o.Lanzar(d.cargar)
	return d
}

func (d *Dialogo) t(k string) string { return d.o.T.T(k, nil) }

func (d *Dialogo) rotuloNivel(n string) string {
	if n == api.NivelEditar {
		return d.t("sharePeople.write")
	}
	return d.t("sharePeople.read")
}

func (d *Dialogo) avisar(exito bool, texto string) {
	if d.o.Avisar != nil {
		d.o.Avisar(exito, texto)
	}
}

// Personas es la lista que se enseña (para los guardianes).
func (d *Dialogo) Personas() []api.PersonaCompartida { return d.personas }

// Cerrar cierra el diálogo.
func (d *Dialogo) Cerrar() { d.d.Cerrar() }

// cargar trae la lista y la cara de cada persona.
func (d *Dialogo) cargar() {
	ctx, cancelar := context.WithTimeout(context.Background(), Plazo)
	defer cancelar()
	lista, err := d.o.Personas.Lista(ctx)
	caras := map[string]api.Persona{}
	if err == nil && d.o.Directorio != nil {
		for _, p := range lista {
			if per, e := d.o.Directorio.PorID(ctx, p.Sub); e == nil {
				caras[p.Sub] = per
			}
		}
	}
	d.o.EnHilo(func() {
		d.personas, d.falloLista = lista, err != nil
		if d.personas == nil && err == nil {
			d.personas = []api.PersonaCompartida{}
		}
		for k, v := range caras {
			d.caras[k] = v
		}
		d.pintar()
	})
}

// Compartir lo da a la persona del campo, con el nivel elegido.
func (d *Dialogo) Compartir() {
	correo := strings.TrimSpace(d.correo.Texto())
	if correo == "" || d.enviando || d.o.Directorio == nil {
		return
	}
	nivel := d.o.Niveles[min(max(d.nivel.Elegida, 0), len(d.o.Niveles)-1)]
	d.enviando, d.errorCorreo = true, ""
	d.pintarBoton()
	d.o.Lanzar(func() {
		ctx, cancelar := context.WithTimeout(context.Background(), Plazo)
		defer cancelar()
		per, err := d.o.Directorio.PorCorreo(ctx, correo)
		if err == nil {
			_, err = d.o.Personas.Compartir(ctx, per.ID, nivel)
		}
		d.o.EnHilo(func() {
			d.enviando = false
			switch {
			case err == nil:
				d.correo.PonerTexto("")
				d.caras[per.ID] = per
				quedan := d.personas[:0:0]
				for _, x := range d.personas {
					if x.Sub != per.ID {
						quedan = append(quedan, x)
					}
				}
				quedan = append(quedan, api.PersonaCompartida{Sub: per.ID, Permiso: nivel})
				d.personas = quedan
				d.avisar(true, d.o.T.T("sharePeople.added", i18n.Vars{"email": cmpOr(per.Email, correo)}))
			case codigo(err) == http.StatusNotFound:
				d.errorCorreo = d.t("sharePeople.unknown")
			default:
				d.avisar(false, d.t("sharePeople.failed"))
			}
			d.pintar()
		})
	})
}

// Quitar se lo retira a esa persona.
func (d *Dialogo) Quitar(p api.PersonaCompartida) {
	if d.quitando != "" {
		return
	}
	d.quitando = p.Sub
	d.pintar()
	d.o.Lanzar(func() {
		ctx, cancelar := context.WithTimeout(context.Background(), Plazo)
		defer cancelar()
		err := d.o.Personas.Quitar(ctx, p.Sub)
		// 404: ya no estaba (otra ventana). En los dos casos, fuera de la lista.
		if codigo(err) == http.StatusNotFound {
			err = nil
		}
		d.o.EnHilo(func() {
			d.quitando = ""
			if err != nil {
				d.avisar(false, d.t("sharePeople.removeFailed"))
			} else {
				quedan := d.personas[:0:0]
				for _, x := range d.personas {
					if x.Sub != p.Sub {
						quedan = append(quedan, x)
					}
				}
				d.personas = quedan
				d.avisar(true, d.o.T.T("sharePeople.removed", i18n.Vars{"name": d.nombre(p)}))
			}
			d.pintar()
		})
	})
}

// nombre es cómo se dice quién es: su nombre, su correo o «alguien de tu organización».
func (d *Dialogo) nombre(p api.PersonaCompartida) string {
	c := d.caras[p.Sub]
	return cmpOr(c.Nombre, c.Email, d.t("sharePeople.someone"))
}

func (d *Dialogo) pintarBoton() {
	d.anadir.Inerte = strings.TrimSpace(d.correo.Texto()) == ""
	d.anadir.Cargando = d.enviando
	d.anadir.Refresh()
}

// pintar rehace el cuerpo. Los campos son los MISMOS de una vez a otra: no pierden ni lo
// escrito ni el foco.
func (d *Dialogo) pintar() {
	v := d.o.V
	d.pintarBoton()
	pista := ui.NuevaPistaCampo(d.t("sharePeople.hint"))
	if d.errorCorreo != "" {
		pista = ui.NuevaPistaCampo(d.errorCorreo).ConTono(ui.AyudaPeligro)
	}
	campos := []fyne.CanvasObject{grupo(d.t("sharePeople.email"), d.correo, pista)}
	anchos := []float32{0}
	if len(d.o.Niveles) > 1 {
		campos = append(campos, grupo(d.t("sharePeople.access"), d.nivel))
		anchos = append(anchos, AnchoNivel)
	}
	leyenda := enLinea(ui.Texto(d.t("sharePeople.legend"), tema.TextoBase, ui.SemiNegrita, tema.Color("text", v)), tema.TextoBase)
	versal := ui.NuevoVersal(d.t("sharePeople.heading"), tema.TextoXS, ui.SemiNegrita, tema.Color("text-subtle", v))
	fila := container.New(&filaForm{campos: campos, anchos: anchos, boton: d.anadir}, append(campos, d.anadir)...)
	// `mb-2` de la leyenda, `mt-5 mb-2` del encabezado de la lista.
	bloques := []fyne.CanvasObject{leyenda, fila, enVersal(versal), d.lista(v)}
	separas := []float32{0, tema.Espacio(2), tema.Espacio(5), tema.Espacio(2)}
	d.cuerpo.Objects = []fyne.CanvasObject{container.New(&pila{separas: separas}, bloques...)}
	d.cuerpo.Refresh()
	d.d.Recolocar()
}

// lista es con quién está compartido, o lo que haya en su lugar.
func (d *Dialogo) lista(v tema.Variante) fyne.CanvasObject {
	switch {
	case d.personas == nil && !d.falloLista:
		borde := ui.NuevaCaja(transparente, tema.Color("border", v), tema.RadioMD)
		return container.New(&altoFijo{alto: tema.Espacio(16)}, append(borde.Objetos(), container.NewCenter(ui.NuevaRueda(18, "text", v)))...)
	case d.falloLista:
		fondo := ui.NuevaCaja(tema.Color("bg-subtle", v), tema.Color("border", v), tema.RadioMD)
		texto := enLinea(ui.Texto(d.t("sharePeople.loadFailed"), tema.TextoSM, ui.Normal, tema.Color("text-muted", v)), tema.TextoSM)
		reintentar := ui.NuevoBoton(d.t("action.retry"), func() {
			d.personas, d.falloLista = nil, false
			d.pintar()
			d.o.Lanzar(d.cargar)
		}).Con(ui.Secundario, ui.TallaSM)
		return container.New(&margen{x: tema.Espacio(3), y: tema.Espacio(2)},
			append(fondo.Objetos(), container.NewBorder(nil, nil, texto, reintentar))...)
	case len(d.personas) == 0:
		return enLinea(ui.Texto(d.t("sharePeople.empty"), tema.TextoSM, ui.Normal, tema.Color("text-muted", v)), tema.TextoSM)
	}
	borde := ui.NuevaCaja(transparente, tema.Color("border", v), tema.RadioMD)
	filas := make([]fyne.CanvasObject, 0, len(d.personas))
	for i, p := range d.personas {
		filas = append(filas, d.fila(p, i > 0, v))
	}
	return container.New(&margen{x: tema.Borde, y: tema.Borde}, append(borde.Objetos(), container.New(ui.Columna{}, filas...))...)
}

// fila es una persona: `px-3 py-2 gap-3`, el glifo, quién, el nivel (y su correo si se
// enseña su nombre) y «Revocar».
func (d *Dialogo) fila(p api.PersonaCompartida, filete bool, v tema.Variante) fyne.CanvasObject {
	c := d.caras[p.Sub]
	meta := d.rotuloNivel(p.Permiso)
	if c.Nombre != "" && c.Email != "" {
		meta += " · " + c.Email
	}
	quitar := ui.NuevoBoton(d.t("sharePeople.remove"), func() { d.Quitar(p) }).Con(ui.Fantasma, ui.TallaSM)
	quitar.Cargando = d.quitando == p.Sub
	quitar.Inerte = d.quitando != "" && d.quitando != p.Sub
	f := &filaPersona{
		glifo:  iconos.Trazo(iconos.Usuario, 16, "text-muted", v),
		nombre: ui.Texto(d.nombre(p), tema.TextoBase, ui.Medio, tema.Color("text", v)),
		meta:   ui.Texto(meta, tema.TextoXS, ui.Normal, tema.Color("text-subtle", v)),
		quitar: quitar,
		linea:  canvas.NewRectangle(tokens.ConAlfa(tema.Color("border", v), 0.6)),
	}
	f.linea.Hidden = !filete
	return container.New(f, f.linea, f.glifo.Objeto(), f.nombre, f.meta, quitar)
}

// grupo es el FieldGroup con su pista debajo, en la misma columna `gap-1.5`.
func grupo(etiqueta string, control fyne.CanvasObject, debajo ...fyne.CanvasObject) fyne.CanvasObject {
	g := ui.GrupoCampo(etiqueta, false, control, "", "")
	if len(debajo) == 0 {
		return g
	}
	return container.New(ui.Columna{Hueco: ui.HuecoGrupoCampo}, append([]fyne.CanvasObject{g}, debajo...)...)
}

func codigo(err error) int {
	var e *api.Error
	if errors.As(err, &e) {
		return e.Status
	}
	return 0
}

func cmpOr(s ...string) string {
	for _, x := range s {
		if x != "" {
			return x
		}
	}
	return ""
}
