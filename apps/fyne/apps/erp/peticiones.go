// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"context"
	"slices"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/peticion"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// La franja de un módulo en lectura, con su «Pedir acceso de edición» a la
// administración, y la de la administración que llega desde el aviso: las
// `FranjaPeticion` de `ErpApp.tsx`. La pieza es la común (`peticion`).

// modoPeticiones es el estado de las dos franjas.
type modoPeticiones struct {
	modulo      string // el módulo cuya franja está enganchada
	estado      peticion.Estado
	delAviso    string // el módulo del aviso que atiende la administración
	estadoDueno peticion.Estado
}

// franjasPeticion son las franjas que tocan ahora encima de la vista.
func (p *Pantalla) franjasPeticion() []fyne.CanvasObject {
	var out []fyne.CanvasObject
	t := func(k string, v i18n.Vars) string { return p.tr.T(k, v) }
	if f := peticion.Franja(&p.peticiones.estadoDueno, false, "", t); f != nil && p.peticiones.delAviso != "" {
		out = append(out, f)
	}
	modulo := p.lista.Modelo.Modulo
	if p.svc == nil || !p.SabePermisos || modulo == "" || p.Vista != "" || p.Permisos.Modulos[modulo] != api.ERPLectura {
		return out
	}
	if p.peticiones.modulo != modulo {
		p.peticiones.modulo = modulo
		pedida := slices.Contains(p.Permisos.Pedidos, modulo)
		peticion.Enganchar(&p.peticiones.estado, p.svc.Peticiones(modulo), peticion.Opciones{
			SinPermiso: true,
			YaPedida:   func(context.Context) (bool, error) { return pedida, nil },
			Recomponer: p.Repintar,
			T:          t,
			Concedido:  "erp.access.granted", Recurso: modulo,
			AlConceder: func() { p.peticiones.modulo = ""; p.fondo(p.cargarCatalogo) },
		})
	}
	nombre := modulo
	for _, m := range p.Catalogo.Modulos {
		if m.Clave == modulo {
			nombre = m.Etiqueta
		}
	}
	return append(out, peticion.Franja(&p.peticiones.estado, true, p.t("readOnly.module", i18n.Vars{"name": nombre}), t))
}

// AtenderPeticion enseña a la administración quién pide editar `modulo`, con «Dar
// acceso» y «Rechazar»: es a lo que lleva el aviso de una petición de edición.
func (p *Pantalla) AtenderPeticion(modulo string) {
	if p.svc == nil {
		return
	}
	p.peticiones.delAviso = modulo
	peticion.Enganchar(&p.peticiones.estadoDueno, p.svc.Peticiones(modulo), peticion.Opciones{
		PuedeConceder: true, Recomponer: p.Repintar, T: func(k string, v i18n.Vars) string { return p.tr.T(k, v) },
	})
}
