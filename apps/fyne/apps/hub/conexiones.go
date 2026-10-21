// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"context"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// AlmacenConectable is the authoritative availability check shared with Drive.
type AlmacenConectable interface {
	ExternalStorage(context.Context) (api.AlmacenExterno, error)
}

// ConAlmacenamiento adds the direct Drive wizard shortcut for permitted accounts.
func (v *Vista) ConAlmacenamiento(s AlmacenConectable) *Vista { v.almacen = s; return v }

// ConConexiones installs the native OAuth catalogue. The session owns browser and
// callback validation; this screen only chooses the provider and public fields.
func (v *Vista) ConConexiones(w fyne.Window, gateway *api.Integraciones, autorizar func(context.Context, string, string) (string, error)) *Vista {
	v.ventanaConexiones, v.cuentasApps, v.autorizarApp = w, gateway, autorizar
	return v
}

func puedeConectar(r auth.Reclamos) bool {
	return r.Sub != "" && r.Tenant != "" && (r.EsCuentaPersonal() || slices.Contains(r.Roles, "owner") || slices.Contains(r.Roles, "admin"))
}

func (v *Vista) cargarAccesosConexiones(ctx context.Context, r auth.Reclamos) {
	permitido := puedeConectar(r)
	externo := false
	if permitido && v.almacen != nil {
		estado, err := v.almacen.ExternalStorage(ctx)
		externo = err == nil && estado.Available
	}
	v.enUI(func() {
		v.lanzador.PuedeConectarAlmacen = externo
		v.lanzador.PuedeConectarApps = permitido && v.cuentasApps != nil && v.autorizarApp != nil
	})
}

func (v *Vista) conectarAlmacen() {
	if !v.lanzador.PuedeConectarAlmacen || v.abrir == nil {
		return
	}
	// Configuration must reach Drive even if an organisation substitutes that product.
	_ = api.AbrirEn(v.abrir, v.apex, "drive", v.org(), "/?connect-storage=1")
}

func (v *Vista) conectarApp() {
	if !v.lanzador.PuedeConectarApps || v.ventanaConexiones == nil {
		return
	}
	c := &conexionApp{v: v, cuerpo: container.NewStack(), pie: container.New(ui.FilaAuto{Hueco: ui.HuecoPieDialogo, Partir: true, SaltoFila: ui.HuecoPieDialogo, AlFinal: true}), abierto: true, cargando: true, campos: map[string]*ui.Entrada{}}
	c.d = ui.NuevoDialogo(v.t.T("apps.connect", nil), c.cuerpo).Con(ui.DialogoMD, c.pie)
	c.ancho = ui.AnchoCuerpoDialogo(c.d, v.ventanaConexiones.Canvas().Size())
	c.d.RotuloCerrar = v.t.T("ui.close", nil)
	c.d.AlCerrar = c.cerrar
	c.pintar()
	c.d.Mostrar(v.ventanaConexiones.Canvas())
	c.cargar()
}

type conexionApp struct {
	v                                     *Vista
	d                                     *ui.Dialogo
	cuerpo, pie                           *fyne.Container
	ancho                                 float32
	proveedores                           []api.OAuthTemplate
	cuentas                               []api.OAuthInstance
	proveedor                             api.OAuthTemplate
	instancia                             string
	campos                                map[string]*ui.Entrada
	paso                                  int
	abierto, cargando, ocupado, confirmar bool
	aviso                                 string
	cancelar                              context.CancelFunc
}

func (c *conexionApp) t(key string, vars ...i18n.Vars) string {
	var v i18n.Vars
	if len(vars) > 0 {
		v = vars[0]
	}
	return c.v.t.T(key, v)
}

func (c *conexionApp) cerrar() {
	c.abierto = false
	if c.cancelar != nil {
		c.cancelar()
	}
	c.d.Cerrar()
}

func (c *conexionApp) cargar() {
	c.cargando = true
	c.pintar()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		proveedores, err := c.v.cuentasApps.OAuthTemplates(ctx)
		cuentas, errCuentas := c.v.cuentasApps.OAuthInstances(ctx)
		fyne.Do(func() {
			if !c.abierto {
				return
			}
			c.cargando = false
			if err != nil || errCuentas != nil {
				c.aviso = c.t("apps.toast.gatewayDown")
			} else {
				c.proveedores, c.cuentas = proveedores, cuentas
				c.aviso = ""
			}
			c.pintar()
		})
	}()
}

func (c *conexionApp) elegir(p api.OAuthTemplate) {
	c.proveedor, c.instancia, c.campos, c.aviso = p, "", map[string]*ui.Entrada{}, ""
	if cuenta, ok := api.PreferredOAuthInstance(c.cuentas, p.Key, ""); ok && (cuenta.Active() || cuenta.OAuthAvailable) {
		c.instancia = cuenta.ID
	}
	c.paso = 2
	if c.instancia == "" && len(p.Fields) > 0 {
		c.paso = 1
		for _, f := range p.Fields {
			campo := ui.NuevaEntrada(f.Placeholder)
			campo.Ancho = c.ancho
			campo.AlCambiar(func(string) { c.pintarPie() })
			c.campos[f.Key] = campo
		}
	}
	c.pintar()
}

func (c *conexionApp) conectada() bool {
	for _, cuenta := range c.cuentas {
		if cuenta.ID == c.instancia {
			return cuenta.Active()
		}
	}
	return false
}

func (c *conexionApp) completo() bool {
	for _, f := range c.proveedor.Fields {
		if f.Required && (c.campos[f.Key] == nil || strings.TrimSpace(c.campos[f.Key].Texto()) == "") {
			return false
		}
	}
	return true
}

func (c *conexionApp) disponible() bool {
	if c.proveedor.Available {
		return true
	}
	for _, cuenta := range c.cuentas {
		if cuenta.TemplateKey == c.proveedor.Key && (c.instancia == "" || cuenta.ID == c.instancia) && cuenta.OAuthAvailable {
			return true
		}
	}
	return false
}

//nolint:contextcheck // autorizar launches background authorization flow with its own timeout context.
func (c *conexionApp) autorizar() {
	if c.ocupado || !c.disponible() {
		return
	}
	c.ocupado, c.aviso = true, ""
	c.pintar()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	c.cancelar = cancel
	proveedor, instancia := c.proveedor, c.instancia
	config := map[string]string{}
	for key, entrada := range c.campos {
		config[key] = strings.TrimSpace(entrada.Texto())
	}
	go func() {
		defer cancel()
		var err error
		if instancia == "" && len(config) > 0 {
			instancia, err = c.v.cuentasApps.OAuthCreateInstance(ctx, proveedor.Key, proveedor.Name, config)
		}
		id := ""
		if err == nil {
			id, err = c.v.autorizarApp(ctx, proveedor.Key, instancia)
		}
		fyne.Do(func() {
			if !c.abierto {
				return
			}
			c.ocupado = false
			if err != nil {
				c.aviso = c.t("apps.toast.actionFailed")
				if instancia != "" {
					c.instancia = instancia
				}
				c.pintar()
				return
			}
			c.instancia, c.paso = id, 3
			if i := slices.IndexFunc(c.cuentas, func(cuenta api.OAuthInstance) bool { return cuenta.ID == id }); i >= 0 {
				c.cuentas[i].Status, c.cuentas[i].HasSecrets, c.cuentas[i].OAuthAvailable = "active", true, true
			} else {
				c.cuentas = append(c.cuentas, api.OAuthInstance{ID: id, TemplateKey: proveedor.Key, Name: proveedor.Name, Status: "active", HasSecrets: true, OAuthAvailable: true})
			}
			c.pintar()
			go c.v.cargarIntegraciones()
		})
	}()
}

func (c *conexionApp) desconectar() {
	if c.ocupado {
		return
	}
	c.ocupado, c.aviso = true, ""
	c.pintar()
	id := c.instancia
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := c.v.cuentasApps.OAuthDisconnect(ctx, id)
		fyne.Do(func() {
			if !c.abierto {
				return
			}
			c.ocupado = false
			if err != nil {
				c.aviso = c.t("apps.toast.actionFailed")
				c.pintar()
				return
			}
			c.confirmar, c.paso, c.instancia = false, 0, ""
			c.cargar()
			go c.v.cargarIntegraciones()
		})
	}()
}

//nolint:gocognit,gocyclo,gocritic // pintar renders progressive wizard step content using if-else cascade.
func (c *conexionApp) pintar() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	var bloques []ui.Bloque
	anadir := func(o fyne.CanvasObject) { bloques = append(bloques, ui.Bloque{Obj: o, Margen: tema.Espacio(3)}) }
	texto := func(s string, peso ui.Peso, tinta string) {
		anadir(ui.NuevoParrafo(s, tema.TextoSM, peso, tinta, v).A(c.ancho))
	}
	if c.confirmar {
		texto(c.t("apps.deactivate.title"), ui.SemiNegrita, "text")
		texto(c.proveedor.Name, ui.Normal, "text-muted")
	} else if c.cargando {
		texto(c.t("ui.loading"), ui.Normal, "text-muted")
	} else if c.paso == 3 {
		texto(c.t("wizard.app.done.heading", i18n.Vars{"app": c.proveedor.Name}), ui.SemiNegrita, "text")
	} else {
		titulos := []string{c.t("wizard.app.step.choose")}
		conCampos := len(c.proveedor.Fields) > 0 && !c.conectada()
		if conCampos {
			titulos = append(titulos, c.t("wizard.app.step.configure"))
		}
		titulos = append(titulos, c.t("wizard.app.step.review"))
		pasos := ui.NuevosPasos(titulos...)
		pasos.Ventana = c.v.ventanaConexiones.Canvas().Size().Width
		pasos.Actual = c.paso
		if !conCampos && c.paso == 2 {
			pasos.Actual = 1
		}
		if !c.ocupado {
			pasos.AlElegir = func(i int) {
				if i == 0 {
					c.paso = 0
				} else {
					c.paso = 1
				}
				c.aviso = ""
				c.pintar()
			}
		}
		pasos.RotuloIr = func(n int) string { return c.t("ui.wizard.goToStep", i18n.Vars{"n": n}) }
		anadir(pasos)
		switch c.paso {
		case 0:
			texto(c.t("wizard.app.choose.heading"), ui.SemiNegrita, "text")
			if len(c.proveedores) == 0 {
				texto(c.t("apps.catalog.emptyDescription"), ui.Normal, "text-muted")
			}
			for _, p := range c.proveedores {
				tarjeta := ui.NuevaTarjetaEleccion(p.Name, p.Description, func() { c.elegir(p) }).A(c.ancho)
				tarjeta.Densa = true
				for _, cuenta := range c.cuentas {
					if cuenta.TemplateKey == p.Key && cuenta.Active() {
						tarjeta.Insignia = c.t("apps.status.active")
						break
					}
				}
				anadir(tarjeta)
			}
		case 1:
			texto(c.t("wizard.app.configure.heading", i18n.Vars{"app": c.proveedor.Name}), ui.SemiNegrita, "text")
			for _, campo := range c.proveedor.Fields {
				anadir(ui.GrupoCampo(campo.Label, campo.Required, c.campos[campo.Key], "", ""))
				if campo.Help != "" {
					texto(campo.Help, ui.Normal, "text-muted")
				}
			}
		case 2:
			texto(c.t("wizard.app.oauth.title", i18n.Vars{"app": c.proveedor.Name}), ui.SemiNegrita, "text")
			if c.conectada() {
				texto(c.t("apps.status.active"), ui.Medio, "text")
			} else {
				if c.disponible() {
					texto(c.t("wizard.app.oauth.body", i18n.Vars{"app": c.proveedor.Name}), ui.Normal, "text-muted")
				} else {
					texto(c.t("wizard.auto.error.noProviderApp", i18n.Vars{"source": c.proveedor.Name}), ui.Normal, "text-muted")
				}
				if len(c.proveedor.Scopes) > 0 {
					texto(c.t("wizard.app.oauth.willAuthorize"), ui.Medio, "text")
					for _, scope := range c.proveedor.Scopes {
						texto(scope, ui.Normal, "text-muted")
					}
				}
			}
			texto(c.t("wizard.app.oauth.revocable", i18n.Vars{"app": c.proveedor.Name}), ui.Normal, "text-muted")
		}
	}
	if c.aviso != "" {
		texto(c.aviso, ui.Normal, "danger")
	}
	c.cuerpo.Objects = []fyne.CanvasObject{ui.PilaBloques(bloques...)}
	c.cuerpo.Refresh()
	c.pintarPie()
	c.d.Recolocar()
}

func (c *conexionApp) pintarPie() {
	var botones []fyne.CanvasObject
	boton := func(texto string, variante ui.Variante, hacer func(), inerte bool) {
		b := ui.NuevoBoton(texto, hacer).Con(variante, ui.TallaMD)
		b.Inerte = inerte
		botones = append(botones, b)
	}
	if c.confirmar {
		boton(c.t("ui.cancel"), ui.Secundario, func() { c.confirmar = false; c.pintar() }, c.ocupado)
		boton(c.t("apps.deactivate"), ui.Primario, c.desconectar, c.ocupado)
	} else {
		boton(c.t("ui.close"), ui.Secundario, c.cerrar, false)
		if c.aviso != "" && c.paso == 0 {
			boton(c.t("action.retry"), ui.Primario, c.cargar, c.cargando)
		}
		if c.paso > 0 && c.paso < 3 {
			boton(c.t("ui.wizard.back"), ui.Secundario, func() { c.paso = 0; c.aviso = ""; c.pintar() }, c.ocupado)
		}
		if c.paso == 1 {
			boton(c.t("ui.wizard.next"), ui.Primario, func() { c.paso = 2; c.pintar() }, c.ocupado || !c.completo())
		}
		if c.paso == 2 || c.paso == 3 {
			if c.conectada() {
				boton(c.t("apps.deactivate"), ui.Secundario, func() { c.confirmar = true; c.pintar() }, c.ocupado)
			} else {
				boton(c.t("wizard.app.oauth.title", i18n.Vars{"app": c.proveedor.Name}), ui.Primario, c.autorizar, c.ocupado || !c.disponible())
			}
		}
	}
	c.pie.Objects = botones
	c.pie.Refresh()
}
