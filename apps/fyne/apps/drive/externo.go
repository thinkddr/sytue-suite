// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"errors"
	"net/http"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # «Añadir almacenamiento» (ADR 0038)
//
// `ExternalStorage.tsx`: UNA sola puerta para el almacenamiento de terceros, en la
// sección «Tus nubes» del lateral.
//
//   - Google Drive, OneDrive o Dropbox: un toque y el consentimiento en el navegador.
//     Por defecto es una nube PERSONAL (nubes.go). Owner/admin puede marcar «Guardar
//     aquí los archivos nuevos de la organización»: a la vuelta queda una sola pantalla
//     con la cuenta, el aviso de responsabilidad y Guardar.
//   - WebDAV, S3 y NAS (solo administración y con KEK): un formulario, el aviso y
//     Guardar. El servicio prueba antes de sellar, así que no hay «comprobar» aparte: su
//     motivo sale en el mismo diálogo. El NAS conserva su comando y su sondeo.
//
// No hace falta ninguna credencial nueva en la app: todo va con el token de la persona.

// nubeExterna es una nube por consentimiento con sus tres nombres: el `kind` de Drive,
// el proveedor de la nube personal y la plantilla de la pasarela.
type nubeExterna struct{ kind, nube, plantilla, descripcion string }

var (
	nubesExternas = []nubeExterna{
		{"gdrive", "google-drive", "google-drive", "googleDescription"},
		{"onedrive", "onedrive", "onedrive", "onedriveDescription"},
		{"dropbox", "dropbox", "dropbox-storage", "dropboxDescription"},
	}
	servidoresExternos = []string{"webdav", "s3", "nas"}
	// socioExterno es el nombre del otro lado en el aviso de datos: marca o protocolo,
	// sin traducir. El mismo `PARTNER` de la web.
	socioExterno = map[string]string{"s3": "S3", "webdav": "WebDAV", "gdrive": "Google Drive", "onedrive": "OneDrive", "nas": "NAS", "dropbox": "Dropbox"}
)

// cargarExterno pregunta si hay almacén conectado. Un 403 (no es administración) o
// cualquier fallo lo deja en nil, que es lo que hace la web.
func (p *Pantalla) cargarExterno() {
	if p.svc == nil || !p.hayRed() {
		return
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	e, err := p.svc.ExternalStorage(ctx)
	enHilo(func() {
		p.lateral.Externo = nil
		if err == nil {
			p.lateral.Externo = &e
		}
		p.Repintar()
		if p.abrirExternoPendiente {
			p.abrirExternoPendiente = false
			p.pedirExterno()
		}
	})
}

// AbrirAlmacenamiento abre el selector desde el acceso directo de Hub
// (`?connect-storage=1`). Lo que se ofrece lo decide Drive al cargar, nunca un claim local.
func (p *Pantalla) AbrirAlmacenamiento() {
	p.abrirExternoPendiente = true
	p.enSegundoPlano(p.cargarExterno)
}

// RotuloExterno es el nombre del almacén de la organización en el lateral.
func RotuloExterno(e api.AlmacenExterno) string {
	switch {
	case e.Label != "":
		return e.Label
	case e.Bucket != "":
		return e.Bucket
	}
	return socioExterno[e.Kind]
}

// dialogoExterno es el estado del diálogo.
type dialogoExterno struct {
	p *Pantalla
	// estado es el almacén de la organización; nil si no se administra.
	estado *api.AlmacenExterno
	d      *ui.Dialogo
	cuerpo *fyne.Container
	pie    *fyne.Container
	ancho  float32

	// vista es `elegir`, `confirmar`, `formulario`, `nas` o `gestionar`.
	vista   string
	tipo    string
	paraOrg bool
	// cuenta es la cuenta de organización que se va a guardar; pendiente, el borrador
	// que dejó un consentimiento denegado, para reintentarlo sin crear otro.
	cuenta                                          *api.InstanciaExterna
	pendiente                                       string
	cuentas                                         []api.InstanciaExterna
	proveedores                                     map[string]bool
	pidiendo                                        bool
	endpoint, base, region, bucket, acceso, secreta *ui.Entrada
	nombre                                          *ui.Entrada
	// marca es la clave de la marca de agua con la que se creó el campo del endpoint.
	marca    string
	acepta   bool
	cancelar context.CancelFunc
	// fase es `idle`, `authorizing`, `saving` o `deleting`.
	fase      string
	aviso     string
	avisoOK   bool
	nas       *api.EstadoNAS
	abierto   bool
	sondeando bool
}

// pedirExterno abre el selector «Añadir almacenamiento».
func (p *Pantalla) pedirExterno() *dialogoExterno { return p.abrirExterno("elegir") }

// gestionarExterno abre el almacén de la organización ya conectado: desconectar, o el
// comando del NAS si el túnel todavía no responde.
func (p *Pantalla) gestionarExterno() *dialogoExterno { return p.abrirExterno("gestionar") }

func (p *Pantalla) abrirExterno(vista string) *dialogoExterno {
	if p.win == nil || p.svc == nil {
		return nil
	}
	de := &dialogoExterno{p: p, vista: vista, fase: "idle", cuerpo: container.NewStack(), abierto: true}
	if p.lateral.Externo != nil {
		e := *p.lateral.Externo
		de.estado = &e
	}
	de.ancho = AnchoCuerpoDialogo(ui.DialogoMD, p.explor.Ventana)
	campo := func() *ui.Entrada { return de.campo("", "") }
	de.endpoint, de.base, de.region, de.bucket = campo(), campo(), campo(), campo()
	de.acceso, de.secreta, de.nombre = campo(), campo(), campo()
	de.secreta.Contrasena(true)

	// `flex flex-wrap justify-end gap-2` en escritorio y, en el teléfono, de dos en dos a
	// todo el ancho: el pie de Modal.tsx (`ui.PieAcciones`).
	de.pie = container.New(ui.PieAcciones{})
	de.d = ui.NuevoDialogo(p.tr.T("storage.add", nil), de.cuerpo).Con(ui.DialogoMD, de.pie)
	de.d.RotuloCerrar = p.tr.T("ui.close", nil)
	de.d.AlCerrar = de.cerrar
	de.pintar()
	de.d.Mostrar(p.win.Canvas())
	if de.estado != nil {
		de.cargarCuentas(nil)
	}
	if vista == "gestionar" && de.estado != nil && de.estado.Kind == "nas" {
		de.sondearNas()
	}
	return de
}

// campo es el `<input>` escrito a mano de la web: `border-border bg-surface rounded-md
// border px-3 py-2 text-sm`. Cada pulsación rehace el pie, que se enciende cuando el
// formulario está completo.
func (de *dialogoExterno) campo(valor, marca string) *ui.Entrada {
	x := ui.NuevaEntrada(marca).ConPropia(ui.CampoPropio{
		MargenX: tema.Espacio(3), MargenY: tema.Espacio(2), Texto: tema.TextoSM,
		Borde: "border", Radio: tema.RadioMD, Relleno: "surface",
	})
	x.PonerTexto(valor)
	x.AlCambiar(func(string) { de.aviso = ""; de.pintarPie() })
	return x
}

func (de *dialogoExterno) t(k string, v ...i18n.Vars) string {
	var x i18n.Vars
	if len(v) > 0 {
		x = v[0]
	}
	return de.p.tr.T(k, x)
}

// config es lo escrito, listo para mandar.
func (de *dialogoExterno) config() api.ConfigAlmacen {
	c := api.ConfigAlmacen{Kind: de.tipo, AcceptTerms: de.acepta}
	if de.cuenta != nil {
		c.InstanceID = de.cuenta.ID
		return c
	}
	c.Endpoint, c.BasePath, c.Region = de.endpoint.Texto(), de.base.Texto(), de.region.Texto()
	c.Bucket, c.AccessKey, c.SecretKey, c.Label = de.bucket.Texto(), de.acceso.Texto(), de.secreta.Texto(), de.nombre.Texto()
	return c
}

// Completo es lo que enciende Guardar: el aviso aceptado y la cuenta, o los campos.
func (de *dialogoExterno) Completo() bool {
	if !de.acepta {
		return false
	}
	if de.vista == "confirmar" {
		return de.cuenta != nil
	}
	c := de.config()
	return c.Endpoint != "" && c.AccessKey != "" && c.SecretKey != "" && (c.Kind != "s3" || c.Bucket != "")
}

// nubePersonalDisponible dice si la organización deja conectar esa nube personal.
func (de *dialogoExterno) nubePersonalDisponible(proveedor string) bool {
	if de.p.lateral.Nubes == nil {
		return false
	}
	for _, x := range de.p.lateral.Nubes.Providers {
		if x.Provider == proveedor {
			return x.Available
		}
	}
	return false
}

func (de *dialogoExterno) nubeDisponible(n nubeExterna) bool {
	if de.paraOrg {
		return de.proveedores[n.kind]
	}
	return de.nubePersonalDisponible(n.nube)
}

// cargarCuentas trae las cuentas de organización ya autorizadas; `despues` corre en el
// hilo de la interfaz con el resultado ya adoptado.
func (de *dialogoExterno) cargarCuentas(despues func(error)) {
	de.pidiendo = true
	de.p.enSegundoPlano(func() {
		ctx, cancelar := de.p.ctx()
		defer cancelar()
		cuentas, err := de.p.svc.ExternalStorageAccounts(ctx)
		enHilo(func() {
			if !de.abierto {
				return
			}
			de.cuentas, de.proveedores, de.pidiendo = cuentas.Items, cuentas.Providers, false
			if despues != nil {
				despues(err)
			}
			de.pintar()
		})
	})
}

// cuentaLista es la cuenta de organización utilizable con ese id (o la primera de la
// plantilla, sin id).
func (de *dialogoExterno) cuentaLista(plantilla, id string) *api.InstanciaExterna {
	for i, c := range de.cuentas {
		if c.TemplateKey == plantilla && c.Connected && c.OAuthAvailable && (id == "" || c.ID == id) {
			return &de.cuentas[i]
		}
	}
	return nil
}

// elegirNube es el toque en Google Drive, OneDrive o Dropbox.
func (de *dialogoExterno) elegirNube(n nubeExterna) {
	if de.fase != "idle" || !de.nubeDisponible(n) {
		return
	}
	if !de.paraOrg {
		// Nube personal: si ya está conectada se abre; si no, su consentimiento.
		if de.p.lateral.Nubes != nil {
			for _, c := range de.p.lateral.Nubes.Conectadas() {
				if c.Provider == n.nube && de.p.lateral.AlElegirNube != nil {
					de.cerrar()
					de.p.lateral.AlElegirNube(c)
					return
				}
			}
		}
		de.cerrar()
		de.p.enSegundoPlano(func() { de.p.conectarNube(n.nube) })
		return
	}
	de.tipo = n.kind
	if c := de.cuentaLista(n.plantilla, ""); c != nil {
		de.confirmar(c)
		return
	}
	de.autorizar()
}

func (de *dialogoExterno) confirmar(c *api.InstanciaExterna) {
	de.cuenta, de.acepta, de.aviso, de.vista = c, false, "", "confirmar"
	de.pintar()
}

func (de *dialogoExterno) plantilla() string {
	for _, n := range nubesExternas {
		if n.kind == de.tipo {
			return n.plantilla
		}
	}
	return ""
}

func (de *dialogoExterno) puedeAutorizar() bool {
	if de.pendiente != "" {
		return de.p.AutorizarApp != nil
	}
	return de.p.AutorizarAppNueva != nil || de.p.AutorizarApp != nil
}

// autorizar abre el consentimiento de organización de `de.tipo` en el navegador y, a la
// vuelta, pasa directo a confirmar con la cuenta que el servicio confirme.
//
//nolint:contextcheck // autorizar creates its own long-lived timeout context for OAuth.
func (de *dialogoExterno) autorizar() {
	if !de.puedeAutorizar() || de.fase != "idle" {
		return
	}
	de.fase, de.aviso = "authorizing", ""
	de.pintar()
	ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Minute)
	de.cancelar = cancelar
	plantilla, pendiente := de.plantilla(), de.pendiente
	de.p.enSegundoPlano(func() {
		defer cancelar()
		var id string
		var err error
		switch {
		case pendiente != "":
			id, err = de.p.AutorizarApp(ctx, plantilla, pendiente)
		case de.p.AutorizarAppNueva != nil:
			id, err = de.p.AutorizarAppNueva(ctx, plantilla)
		default:
			id, err = de.p.AutorizarApp(ctx, plantilla, "")
		}
		enHilo(func() {
			if !de.abierto {
				return
			}
			de.fase = "idle"
			if err != nil || id == "" {
				// Un borrador denegado se reintenta tal cual: no se crea otra cuenta.
				de.pendiente = id
				de.aviso = de.t("storage.external.failed")
				if errors.Is(err, api.ErrOAuthUnavailable) {
					de.aviso = de.t("storage.external.wizard.providerUnavailable", i18n.Vars{"provider": socioExterno[de.tipo]})
				}
				de.pintar()
				return
			}
			de.pendiente = ""
			// Solo vale la cuenta que el servicio confirma como autorizada.
			de.cargarCuentas(func(error) {
				if c := de.cuentaLista(plantilla, id); c != nil {
					de.confirmar(c)
					return
				}
				de.aviso = de.t("storage.external.wizard.accountsFailed")
			})
		})
	})
}

func (de *dialogoExterno) elegirServidor(tipo string) {
	de.tipo, de.cuenta, de.acepta, de.aviso, de.vista = tipo, nil, false, "", "formulario"
	for _, x := range []*ui.Entrada{de.endpoint, de.base, de.region, de.bucket, de.acceso, de.secreta, de.nombre} {
		x.PonerTexto("")
	}
	de.pintar()
}

func (de *dialogoExterno) volver() {
	de.vista, de.cuenta, de.aviso = "elegir", nil, ""
	de.pintar()
}

// sondearNas consulta el túnel cada 5 s mientras el diálogo siga abierto en un NAS:
// cuando el WebDAV responde, se avisa y se cierra.
func (de *dialogoExterno) sondearNas() {
	if de.sondeando {
		return
	}
	de.sondeando = true
	var consultar func()
	consultar = func() {
		ctx, cancelar := de.p.ctx()
		defer cancelar()
		st, err := de.p.svc.NASStatus(ctx)
		enHilo(func() {
			if !de.abierto {
				de.sondeando = false
				return
			}
			if err == nil {
				de.nas = &st
				switch {
				case st.Reachable && de.vista == "nas":
					de.sondeando = false
					de.terminado("NAS")
					return
				case st.Reachable:
					// Gestionar un NAS que ya responde: nada que esperar.
					de.sondeando = false
					return
				case st.Command != "":
					de.vista = "nas"
				}
				de.pintar()
			}
			time.AfterFunc(5*time.Second, func() { de.p.enSegundoPlano(consultar) })
		})
	}
	de.p.enSegundoPlano(consultar)
}

// terminado avisa de lo conectado, recarga el lateral y la cuota, y cierra.
func (de *dialogoExterno) terminado(socio string) {
	if n := de.nombre.Texto(); n != "" {
		socio = n
	}
	de.p.avisos.Notificar(ui.NuevoAviso(ui.AvisoExito,
		de.t("storage.external.wizard.doneHeading", i18n.Vars{"provider": socio}),
		de.t("storage.external.wizard.doneDescription")))
	de.recargar()
	de.cerrar()
}

// recargar vuelve a pedir el estado del almacén y la cuota, que dice si lo nuevo va fuera.
func (de *dialogoExterno) recargar() {
	de.p.enSegundoPlano(de.p.cargarExterno)
	de.p.enSegundoPlano(de.p.cargarCuota)
}

// fallo es el aviso de un guardado que no salió: el motivo del servicio, que ya probó
// la conexión (o el código, si no dio ninguno).
func (de *dialogoExterno) fallo(err error) string {
	if codigo(err) == http.StatusServiceUnavailable && api.MotivoDe(err) == "" {
		return de.t("storage.external.unavailable")
	}
	m := api.MotivoDe(err)
	if m == "" && codigo(err) != 0 {
		m = http.StatusText(codigo(err))
	}
	if m == "" {
		return de.t("storage.external.failed")
	}
	return de.t("storage.external.checkFail", i18n.Vars{"error": m})
}

func (de *dialogoExterno) guardar() {
	if de.fase != "idle" || !de.Completo() {
		return
	}
	de.fase, de.aviso = "saving", ""
	de.pintar()
	c := de.config()
	socio := socioExterno[de.tipo]
	if de.cuenta != nil {
		socio = de.cuenta.Name
	}
	de.p.enSegundoPlano(func() {
		ctx, cancelar := de.p.ctx()
		defer cancelar()
		var st api.EstadoNAS
		var err error
		if c.Kind == "nas" {
			st, err = de.p.svc.ConnectNAS(ctx, c)
		} else {
			err = de.p.svc.SaveExternalStorage(ctx, c)
		}
		enHilo(func() {
			de.fase = "idle"
			if err != nil {
				de.aviso, de.avisoOK = de.fallo(err), false
				de.pintar()
				return
			}
			de.secreta.PonerTexto("")
			if c.Kind != "nas" {
				de.terminado(socio)
				return
			}
			st.Online, st.Reachable = false, false
			de.nas, de.vista = &st, "nas"
			de.p.enSegundoPlano(de.p.cargarExterno)
			de.pintar()
			de.sondearNas()
		})
	})
}

func (de *dialogoExterno) desconectar() {
	if de.fase != "idle" {
		return
	}
	de.fase, de.aviso = "deleting", ""
	de.pintar()
	de.p.enSegundoPlano(func() {
		ctx, cancelar := de.p.ctx()
		defer cancelar()
		err := de.p.svc.DeleteExternalStorage(ctx)
		enHilo(func() {
			de.fase = "idle"
			switch {
			case err == nil:
				de.recargar()
				de.cerrar()
				return
			case codigo(err) == http.StatusConflict:
				de.aviso = de.t("storage.external.conflict")
			default:
				de.aviso = de.t("storage.external.failed")
			}
			de.avisoOK = false
			de.pintar()
		})
	})
}

// cerrar cancela el consentimiento en vuelo al descartar la misma hoja.
func (de *dialogoExterno) cerrar() {
	de.abierto = false
	if de.cancelar != nil {
		de.cancelar()
	}
	de.d.Cerrar()
}

// pintarPie usa el pie del diálogo, con una sola acción principal por vista.
func (de *dialogoExterno) pintarPie() {
	ocupado := de.fase != "idle"
	var bs []fyne.CanvasObject
	boton := func(texto string, variante ui.Variante, hacer func(), inerte bool) *ui.Boton {
		b := ui.NuevoBoton(texto, hacer).Con(variante, ui.TallaMD)
		b.Inerte = inerte
		bs = append(bs, b)
		return b
	}
	switch de.vista {
	case "gestionar":
		boton(de.t("storage.external.disconnect"), ui.Secundario, de.desconectar, ocupado)
	case "nas":
		boton(de.t("storage.external.close"), ui.Primario, de.cerrar, false)
	case "confirmar", "formulario":
		boton(de.t("ui.wizard.back"), ui.Secundario, de.volver, ocupado)
		if de.vista == "confirmar" {
			// Otra cuenta del mismo proveedor: un consentimiento nuevo, no se reutiliza.
			boton(de.t("storage.external.wizard.addAccount"), ui.Secundario, func() { de.pendiente = ""; de.autorizar() }, ocupado || !de.puedeAutorizar())
		}
		clave := "storage.external.save"
		if de.tipo == "nas" {
			clave = "storage.external.nas.create"
		}
		if de.fase == "saving" {
			clave = "storage.external.saving"
		}
		boton(de.t(clave), ui.Primario, de.guardar, ocupado || !de.Completo()).Cargando = de.fase == "saving"
	}
	de.pie.Objects = bs
	de.pie.Refresh()
}

func (de *dialogoExterno) pintar() {
	v, ancho := de.p.variante(), de.ancho
	var bloques []fyne.CanvasObject
	var separas []float32
	anadir := func(o fyne.CanvasObject) { bloques = append(bloques, o); separas = append(separas, tema.Espacio(3)) }
	texto := func(s string, peso ui.Peso, tinta string) {
		anadir(ui.NuevoParrafo(s, tema.TextoSM, peso, tinta, v).A(ancho))
	}
	switch de.vista {
	case "gestionar":
		if de.estado != nil {
			texto(de.t("storage.external.wizard.provider")+": "+socioExterno[de.estado.Kind], ui.Medio, "text")
			texto(de.t("storage.external.wizard.destination")+": "+RotuloExterno(*de.estado), ui.Normal, "text-muted")
		}
		texto(de.t("storage.external.wizard.reviewDescription"), ui.Normal, "text-muted")
	case "nas":
		if de.nas != nil {
			anadir(de.comandoNas(v, ancho))
		}
	case "confirmar":
		texto(de.t("storage.external.wizard.accountHeading", i18n.Vars{"provider": socioExterno[de.tipo]}), ui.SemiNegrita, "text")
		if de.cuenta != nil {
			texto(de.t("storage.external.account")+": "+de.cuenta.Name, ui.Medio, "text")
		}
		anadir(ui.FlujoDatos(de.p.tr, socioExterno[de.tipo], []string{"files"}, []string{"files"}, false, de.p.abrirPolitica, ancho))
		anadir(de.avisoResponsabilidad(v, ancho))
	case "formulario":
		texto(de.t("storage.external.wizard.accountHeading", i18n.Vars{"provider": socioExterno[de.tipo]}), ui.SemiNegrita, "text")
		anadir(de.formulario(v))
		anadir(ui.FlujoDatos(de.p.tr, socioExterno[de.tipo], []string{"files"}, []string{"files"}, false, de.p.abrirPolitica, ancho))
		anadir(de.avisoResponsabilidad(v, ancho))
	default:
		de.pintarEleccion(anadir, texto, ancho)
	}
	if de.aviso != "" {
		tinta := "danger"
		if de.avisoOK {
			tinta = "text"
		}
		texto(de.aviso, ui.Normal, tinta)
	}
	if len(separas) > 0 {
		separas[0] = 0
	}
	de.cuerpo.Objects = []fyne.CanvasObject{container.New(&pilaSeparada{separas: separas}, bloques...)}
	de.cuerpo.Refresh()
	de.pintarPie()
	de.d.Recolocar()
}

// pintarEleccion es el selector: las tres nubes, la casilla de organización (solo
// administración) y, con KEK, los tres servidores. Un toque hace la acción.
func (de *dialogoExterno) pintarEleccion(anadir func(fyne.CanvasObject), texto func(string, ui.Peso, string), ancho float32) {
	ocupado := de.fase != "idle"
	texto(de.t("storage.external.wizard.chooseDescription"), ui.Normal, "text-muted")
	tarjeta := func(titulo, descripcion string, inerte bool, hacer func()) {
		x := ui.NuevaTarjetaEleccion(titulo, descripcion, hacer).A(ancho)
		x.Densa, x.Inerte = true, inerte
		anadir(x)
	}
	for _, n := range nubesExternas {
		nube := n
		disponible := de.nubeDisponible(n)
		cargando := de.paraOrg && de.pidiendo
		descripcion := de.t("storage.external.wizard." + n.descripcion)
		if !disponible && !cargando {
			descripcion = de.t("storage.external.wizard.providerUnavailable", i18n.Vars{"provider": socioExterno[n.kind]})
		}
		tarjeta(socioExterno[n.kind], descripcion, ocupado || cargando || !disponible, func() { de.elegirNube(nube) })
	}
	if de.estado == nil {
		return
	}
	anadir(de.casilla(de.t("storage.external.forOrg"), de.paraOrg, func(si bool) { de.paraOrg = si; de.pintar() }, ancho))
	if !de.estado.Available {
		return
	}
	for _, s := range servidoresExternos {
		tipo := s
		tarjeta(socioExterno[s], de.t("storage.external.wizard."+s+"Description"), ocupado, func() { de.elegirServidor(tipo) })
	}
}

// formulario es el `grid gap-3 sm:grid-cols-2` de WebDAV, S3 o NAS: lo que ocupa las
// dos columnas va solo en su fila; región y bucket, y usuario y contraseña, van de dos
// en dos desde `sm`. Lo opcional (nombre y carpeta o región) va al final.
func (de *dialogoExterno) formulario(v tema.Variante) fyne.CanvasObject {
	ancho := de.ancho
	dos := de.p.explor.Ventana == 0 || de.p.explor.Ventana >= ui.CorteSM
	media := ancho
	if dos {
		media = (ancho - tema.Espacio(3)) / 2
	}
	etiquetado := func(rotulo string, control *ui.Entrada, pista fyne.CanvasObject, w float32) fyne.CanvasObject {
		// `text-text-muted mb-1 block text-xs font-medium`, el control y la pista
		// `mt-1 block text-xs text-muted`.
		control.Ancho = w
		hijos := []fyne.CanvasObject{lineaTexto(ui.Texto(rotulo, tema.TextoXS, ui.Medio, tema.Color("text-muted", v)), tema.TextoXS), control}
		separas := []float32{0, tema.Espacio(1)}
		if pista != nil {
			hijos = append(hijos, pista)
			separas = append(separas, tema.Espacio(1))
		}
		return container.New(&pilaSeparada{separas: separas, ancho: &w}, hijos...)
	}
	var filas [][]fyne.CanvasObject
	if de.tipo == "nas" {
		filas = append(filas, []fyne.CanvasObject{ui.NuevoParrafo(de.t("storage.external.nas.intro"), tema.Escalon{Tam: tema.TextoXS.Tam, Linea: 1.625 * tema.TextoXS.Tam}, ui.Normal, "text-muted", v).A(ancho)})
	}
	rotulo, pista := "storage.external.endpoint", "storage.external.endpointHint"
	switch de.tipo {
	case "nas":
		rotulo, pista = "storage.external.nas.local", "storage.external.nas.localHint"
	case "webdav":
		pista = "storage.external.endpointHintWebdav"
	}
	if de.marca != pista {
		// La marca de agua depende de la clase y un campo no la cambia: se rehace con lo
		// que ya llevaba escrito.
		de.endpoint, de.marca = de.campo(de.endpoint.Texto(), de.t(pista)), pista
	}
	filas = append(filas, []fyne.CanvasObject{etiquetado(de.t(rotulo), de.endpoint, nil, ancho)})
	usuario, secreta := "storage.external.user", "storage.external.password"
	if de.tipo == "s3" {
		usuario, secreta = "storage.external.accessKey", "storage.external.secretKey"
		filas = append(filas, []fyne.CanvasObject{etiquetado(de.t("storage.external.bucket"), de.bucket, nil, ancho)})
	}
	ayuda := ui.NuevoParrafo(de.t("storage.external.secretHint"), tema.TextoXS, ui.Normal, "text-muted", v).A(media)
	filas = append(filas, []fyne.CanvasObject{
		etiquetado(de.t(usuario), de.acceso, nil, media),
		etiquetado(de.t(secreta), de.secreta, ayuda, media),
	})
	opcional := etiquetado(de.t("storage.external.basePath"), de.base, nil, media)
	if de.tipo == "s3" {
		opcional = etiquetado(de.t("storage.external.region"), de.region, nil, media)
	}
	filas = append(filas, []fyne.CanvasObject{etiquetado(de.t("storage.external.label"), de.nombre, nil, media), opcional})

	var bloques []fyne.CanvasObject
	var separas []float32
	for i, f := range filas {
		antes := float32(0)
		if i > 0 {
			antes = tema.Espacio(3)
		}
		if len(f) == 2 && dos {
			bloques = append(bloques, container.New(&dosAlLado{hueco: tema.Espacio(3)}, f[0], f[1]))
			separas = append(separas, antes)
			continue
		}
		for j, o := range f {
			if j > 0 {
				antes = tema.Espacio(3)
			}
			bloques = append(bloques, o)
			separas = append(separas, antes)
		}
	}
	return container.New(&pilaSeparada{separas: separas}, bloques...)
}

// comandoNas es la caja con el `docker run` que se ejecuta en el NAS: `border
// bg-surface-sunken rounded-md p-3`, el título, la ayuda, el comando en un `<pre>`
// seleccionable y el estado del túnel.
func (de *dialogoExterno) comandoNas(v tema.Variante, ancho float32) fyne.CanvasObject {
	dentro := ancho - 2*(tema.Borde+tema.Espacio(3))
	// El `<pre class="bg-surface overflow-x-auto rounded p-2 text-xs select-all">`: una
	// sola línea que se desplaza a lo ancho y se puede seleccionar para copiarla. Es un
	// campo de solo lectura, que es lo que en nativo se deja seleccionar; sin borde, porque
	// el `<pre>` no lo tiene. En JetBrains Mono, como el `font-mono` del `<pre>`.
	cmd := ui.NuevaEntrada("").ConPropia(ui.CampoPropio{
		MargenX: tema.Espacio(2), MargenY: tema.Espacio(2), Texto: tema.TextoXS,
		Borde: "surface", Radio: tema.RadioSM, Relleno: "surface", Mono: true,
	})
	cmd.PonerTexto(de.nas.Command)
	cmd.SoloLectura(true)
	cmd.Ancho = dentro
	estado := de.t("storage.external.nas.waiting")
	if de.nas.Online {
		estado = de.t("storage.external.nas.online")
	}
	if de.nas.Error != "" {
		estado += " · " + de.nas.Error
	}
	hijos := []fyne.CanvasObject{
		ui.NuevoParrafo(de.t("storage.external.nas.command"), tema.TextoSM, ui.Medio, "text", v).A(dentro),
		ui.NuevoParrafo(de.t("storage.external.nas.commandHelp"), tema.TextoXS, ui.Normal, "text-muted", v).A(dentro),
		cmd,
		ui.NuevoBoton(de.t("storage.external.wizard.copy"), func() {
			fyne.CurrentApp().Clipboard().SetContent(de.nas.Command)
			de.aviso, de.avisoOK = de.t("storage.external.wizard.copied"), true
			de.pintar()
		}).Con(ui.Secundario, ui.TallaSM),
		ui.NuevoParrafo(estado, tema.TextoSM, ui.Normal, "text", v).A(dentro),
	}
	pila := container.New(&pilaSeparada{separas: []float32{0, tema.Espacio(1), tema.Espacio(2), tema.Espacio(2), tema.Espacio(2)}}, hijos...)
	caja := ui.NuevaCaja(tema.Color("surface-sunken", v), tema.Color("border", v), tema.RadioMD)
	return container.New(&conMargen{m: tema.Borde + tema.Espacio(3)}, append(caja.Objetos(), pila)...)
}

// casilla es una casilla con su texto al lado (`flex items-start gap-3`), sin caja.
func (de *dialogoExterno) casilla(texto string, marcada bool, alCambiar func(bool), ancho float32) fyne.CanvasObject {
	c := ui.NuevaCasilla(texto, alCambiar)
	c.SinEtiqueta, c.Marcada = true, marcada
	m := ui.MedidaEleccion("")
	x := ui.NuevoParrafo(texto, tema.TextoSM, ui.Normal, "text", de.p.variante()).A(ancho - m.Width - tema.Espacio(3))
	return container.New(&dosAlLado{hueco: tema.Espacio(3), izq: m.Width}, c, x)
}

// avisoResponsabilidad es el aviso de responsabilidad: `border bg-surface-sunken flex items-start gap-2
// rounded-md p-3`, la casilla y el texto ENTERO a la vista, no detrás de un enlace.
func (de *dialogoExterno) avisoResponsabilidad(v tema.Variante, ancho float32) fyne.CanvasObject {
	texto := de.t("storage.external.terms")
	casilla := ui.NuevaCasilla(texto, func(si bool) { de.acepta = si; de.pintarPie() })
	casilla.SinEtiqueta, casilla.Marcada = true, de.acepta
	m := ui.MedidaEleccion("")
	dentro := ancho - 2*(tema.Borde+tema.Espacio(3)) - m.Width - tema.Espacio(2)
	x := ui.NuevoParrafo(texto, tema.Escalon{Tam: tema.TextoXS.Tam, Linea: 1.625 * tema.TextoXS.Tam}, ui.Normal, "text", v).A(dentro)
	fila := container.New(&dosAlLado{hueco: tema.Espacio(2), izq: m.Width}, casilla, x)
	caja := ui.NuevaCaja(tema.Color("surface-sunken", v), tema.Color("border", v), tema.RadioMD)
	return container.New(&conMargen{m: tema.Borde + tema.Espacio(3)}, append(caja.Objetos(), fila)...)
}

// dosAlLado pone dos objetos en una fila, cada uno a su alto y arriba (`items-start`): con
// `izq` el primero mide eso y el segundo el resto; sin él, mitad y mitad.
type dosAlLado struct{ hueco, izq float32 }

func (d *dosAlLado) anchos(total float32) (a, b float32) {
	if d.izq > 0 {
		return d.izq, max(total-d.izq-d.hueco, 0)
	}
	a = (total - d.hueco) / 2
	return a, a
}

func (d *dosAlLado) MinSize(objs []fyne.CanvasObject) fyne.Size {
	a, b := objs[0].MinSize(), objs[1].MinSize()
	return fyne.NewSize(a.Width+d.hueco+b.Width, max(a.Height, b.Height))
}

func (d *dosAlLado) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	a, b := d.anchos(t.Width)
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(a, objs[0].MinSize().Height))
	objs[1].Move(fyne.NewPos(a+d.hueco, 0))
	objs[1].Resize(fyne.NewSize(b, objs[1].MinSize().Height))
}

// abrirPolitica abre la política de privacidad del `DataFlow`, en el idioma de la sesión.
func (p *Pantalla) abrirPolitica() {
	_ = api.AbrirFuera(p.abridor(), ui.PoliticaDePrivacidad(p.apex(), p.tr.Locale()))
}
