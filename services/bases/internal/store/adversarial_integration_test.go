// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

// Revisión adversarial (pieza F del ADR 0068). Cada aserción de este fichero se ha visto
// en ROJO quitando la comprobación que vigila (docs/security.md#bases-adversarial): un
// test que no se ha visto fallar no protege nada.

// IDOR ENTRE BASES de la MISMA organización: bea es propietaria de SU base y no tiene
// nada en la de ana. La RLS no separa a las dos —comparten tenant—, así que aquí solo
// protege la ACL, y cada operación que recibe un id tiene que resolver el rol sobre la
// base REAL de ese id, no sobre la que bea sí controla.
func TestIDOREntreBasesDeLaMismaOrganizacion(t *testing.T) {
	e := nuevo(t)
	reg := e.registro(ana, e.tabl.ID, dato(e.txt.ID, `"secreto"`))
	vista, err := e.s.CrearVista(e.ctx, ana, e.tabl.ID, "F", domain.VistaFormulario, domain.ConfigVista{})
	e.ok(err)
	enlace, _, err := e.s.CrearEnlaceFormulario(e.ctx, ana, vista.ID, 5, nil)
	e.ok(err)
	con, _, err := e.s.CrearConexion(e.ctx, ana, e.tabl.ID, "", "Calcus", "", 1)
	e.ok(err)
	perm, err := e.s.ConcederPermiso(e.ctx, ana, e.base.ID, domain.Permiso{PrincipalKind: "user", PrincipalID: "cleo", Role: domain.RolLector})
	e.ok(err)

	suya, err := e.s.CrearBase(e.ctx, bea, "De bea", "")
	e.ok(err)
	suTabla, err := e.s.CrearTabla(e.ctx, bea, suya.ID, "T")
	e.ok(err)
	suTexto, err := e.s.CrearCampo(e.ctx, bea, suTabla.ID, domain.EntradaCampo{Name: "N", Kind: domain.TipoTexto})
	e.ok(err)

	// Con el id ajeno a secas: todo es «no existe», nunca «prohibido» (no se revela).
	ajeno := map[string]func() error{
		"obtener base":   func() error { _, err := e.s.ObtenerBase(e.ctx, bea, e.base.ID); return err },
		"renombrar base": func() error { return e.s.ActualizarBase(e.ctx, bea, e.base.ID, "x", "") },
		"borrar base":    func() error { return e.s.BorrarBase(e.ctx, bea, e.base.ID) },
		"listar tablas":  func() error { _, err := e.s.ListarTablas(e.ctx, bea, e.base.ID); return err },
		"crear tabla":    func() error { _, err := e.s.CrearTabla(e.ctx, bea, e.base.ID, "x"); return err },
		"importar": func() error {
			_, _, err := e.s.ImportarTabla(e.ctx, bea, e.base.ID, "x", domain.TablaCSV{})
			return err
		},
		"renombrar tabla": func() error { return e.s.RenombrarTabla(e.ctx, bea, e.tabl.ID, "x") },
		"borrar tabla":    func() error { return e.s.BorrarTabla(e.ctx, bea, e.tabl.ID) },
		"crear campo": func() error {
			_, err := e.s.CrearCampo(e.ctx, bea, e.tabl.ID, domain.EntradaCampo{Name: "x", Kind: domain.TipoTexto})
			return err
		},
		"renombrar campo": func() error { return e.s.RenombrarCampo(e.ctx, bea, e.txt.ID, "x") },
		"borrar campo":    func() error { return e.s.BorrarCampo(e.ctx, bea, e.txt.ID) },
		"leer registros":  func() error { _, err := e.s.ListarRegistros(e.ctx, bea, e.tabl.ID, "", 0, ""); return err },
		"crear registro": func() error {
			_, err := e.s.CrearRegistro(e.ctx, bea, e.tabl.ID, dato(e.txt.ID, `"x"`), nil)
			return err
		},
		"editar registro": func() error {
			_, err := e.s.ActualizarRegistro(e.ctx, bea, reg.ID, 1, dato(e.txt.ID, `"x"`), nil)
			return err
		},
		"borrar registro":    func() error { return e.s.BorrarRegistro(e.ctx, bea, reg.ID, false) },
		"borrar a la fuerza": func() error { return e.s.BorrarRegistro(e.ctx, bea, reg.ID, true) },
		"exportar":           func() error { return e.s.ExportarTabla(e.ctx, bea, e.tabl.ID, func(Exportacion) error { return nil }) },
		"listar vistas":      func() error { _, err := e.s.ListarVistas(e.ctx, bea, e.tabl.ID); return err },
		"crear vista": func() error {
			_, err := e.s.CrearVista(e.ctx, bea, e.tabl.ID, "x", domain.VistaRejilla, domain.ConfigVista{})
			return err
		},
		"borrar vista":      func() error { return e.s.BorrarVista(e.ctx, bea, vista.ID) },
		"crear enlace":      func() error { _, _, err := e.s.CrearEnlaceFormulario(e.ctx, bea, vista.ID, 5, nil); return err },
		"listar enlaces":    func() error { _, err := e.s.ListarEnlacesFormulario(e.ctx, bea, vista.ID); return err },
		"revocar enlace":    func() error { return e.s.RevocarEnlaceFormulario(e.ctx, bea, enlace.ID) },
		"crear conexión":    func() error { _, _, err := e.s.CrearConexion(e.ctx, bea, e.tabl.ID, "", "x", "", 1); return err },
		"listar conexiones": func() error { _, err := e.s.ListarConexiones(e.ctx, bea, e.base.ID); return err },
		"revocar conexión":  func() error { return e.s.RevocarConexion(e.ctx, bea, con.ID) },
		"rotar conexión":    func() error { _, _, err := e.s.RotarConexion(e.ctx, bea, con.ID); return err },
		"listar permisos":   func() error { _, err := e.s.ListarPermisos(e.ctx, bea, e.base.ID); return err },
		"conceder permiso": func() error {
			_, err := e.s.ConcederPermiso(e.ctx, bea, e.base.ID, domain.Permiso{PrincipalKind: "user", PrincipalID: "bea", Role: domain.RolEditor})
			return err
		},
		"revocar permiso": func() error { return e.s.RevocarPermiso(e.ctx, bea, e.base.ID, perm.ID) },
		"auditoría":       func() error { _, err := e.s.ListarAuditoria(e.ctx, bea, e.base.ID, 0, 10); return err },
	}
	for qué, f := range ajeno {
		esperar(t, f(), domain.ErrNoExiste, qué+" de una base ajena")
	}

	// MEZCLANDO ids: bea pasa SU base (o su tabla), donde sí manda, junto a un id de ana.
	_, err = e.s.ConcederPermiso(e.ctx, bea, suya.ID, domain.Permiso{TableID: e.tabl.ID, PrincipalKind: "user", PrincipalID: "bea", Role: domain.RolEditor})
	esperar(t, err, domain.ErrNoExiste, "permiso de su base sobre una tabla ajena")
	esperar(t, e.s.RevocarPermiso(e.ctx, bea, suya.ID, perm.ID), domain.ErrNoExiste, "revocar un permiso ajeno desde su base")
	_, err = e.s.ListarRegistros(e.ctx, bea, suTabla.ID, vista.ID, 0, "")
	esperar(t, err, domain.ErrNoExiste, "leer su tabla con una vista ajena")
	_, _, err = e.s.CrearConexion(e.ctx, bea, suTabla.ID, vista.ID, "x", "", 1)
	esperar(t, err, domain.ErrNoExiste, "conexión de su tabla con una vista ajena")
	_, err = e.s.CrearCampo(e.ctx, bea, suTabla.ID, domain.EntradaCampo{Name: "L", Kind: domain.TipoRelacion, LinkTableID: e.tabl.ID})
	esperar(t, err, domain.ErrNoExiste, "campo de enlace hacia una tabla de otra base")
	_, err = e.s.CrearVista(e.ctx, bea, suTabla.ID, "V", domain.VistaRejilla, domain.ConfigVista{Sort: []domain.Orden{{Field: e.txt.ID, Dir: "asc"}}})
	esperar(t, err, domain.ErrInvalido, "vista de su tabla que ordena por un campo ajeno")
	_, err = e.s.CrearRegistro(e.ctx, bea, suTabla.ID, dato(e.txt.ID, `"x"`), nil)
	esperar(t, err, domain.ErrInvalido, "registro en su tabla con un campo ajeno")

	// Un campo de enlace de bea hacia SU otra tabla no acepta registros de la de ana.
	suOtra, err := e.s.CrearTabla(e.ctx, bea, suya.ID, "T2")
	e.ok(err)
	lnk, err := e.s.CrearCampo(e.ctx, bea, suTabla.ID, domain.EntradaCampo{Name: "L", Kind: domain.TipoRelacion, LinkTableID: suOtra.ID})
	e.ok(err)
	_, err = e.s.CrearRegistro(e.ctx, bea, suTabla.ID, dato(suTexto.ID, `"x"`), map[string][]string{lnk.ID: {reg.ID}})
	esperar(t, err, domain.ErrInvalido, "enlace a un registro de otra base")

	// Y nada cambió en la base de ana.
	pag, err := e.s.ListarRegistros(e.ctx, ana, e.tabl.ID, "", 0, "")
	e.ok(err)
	if len(pag.Records) != 1 || string(pag.Records[0].Data[e.txt.ID]) != `"secreto"` || len(pag.Fields) != 2 {
		t.Errorf("la base de ana cambió: %+v", pag)
	}
}

// Lo que el test de aislamiento entre organizaciones no cubría: las piezas C y D y la
// estructura (vistas, campos, permisos, enlaces de formulario y conexiones).
func TestIDOREntreOrganizacionesEnVistasFormulariosYConector(t *testing.T) {
	e := nuevo(t)
	vista, err := e.s.CrearVista(e.ctx, ana, e.tabl.ID, "F", domain.VistaFormulario, domain.ConfigVista{})
	e.ok(err)
	enlace, _, err := e.s.CrearEnlaceFormulario(e.ctx, ana, vista.ID, 5, nil)
	e.ok(err)
	con, _, err := e.s.CrearConexion(e.ctx, ana, e.tabl.ID, "", "Calcus", "", 1)
	e.ok(err)
	perm, err := e.s.ConcederPermiso(e.ctx, ana, e.base.ID, domain.Permiso{PrincipalKind: "tenant", PrincipalID: "acme", Role: domain.RolEditor})
	e.ok(err)

	for qué, f := range map[string]func() error{
		"listar vistas": func() error { _, err := e.s.ListarVistas(e.ctx, gus, e.tabl.ID); return err },
		"crear vista": func() error {
			_, err := e.s.CrearVista(e.ctx, gus, e.tabl.ID, "x", domain.VistaRejilla, domain.ConfigVista{})
			return err
		},
		"borrar vista":      func() error { return e.s.BorrarVista(e.ctx, gus, vista.ID) },
		"renombrar campo":   func() error { return e.s.RenombrarCampo(e.ctx, gus, e.txt.ID, "x") },
		"borrar campo":      func() error { return e.s.BorrarCampo(e.ctx, gus, e.txt.ID) },
		"crear enlace":      func() error { _, _, err := e.s.CrearEnlaceFormulario(e.ctx, gus, vista.ID, 5, nil); return err },
		"listar enlaces":    func() error { _, err := e.s.ListarEnlacesFormulario(e.ctx, gus, vista.ID); return err },
		"revocar enlace":    func() error { return e.s.RevocarEnlaceFormulario(e.ctx, gus, enlace.ID) },
		"listar conexiones": func() error { _, err := e.s.ListarConexiones(e.ctx, gus, e.base.ID); return err },
		"rotar conexión":    func() error { _, _, err := e.s.RotarConexion(e.ctx, gus, con.ID); return err },
		"listar permisos":   func() error { _, err := e.s.ListarPermisos(e.ctx, gus, e.base.ID); return err },
		"revocar permiso":   func() error { return e.s.RevocarPermiso(e.ctx, gus, e.base.ID, perm.ID) },
		"importar": func() error {
			_, _, err := e.s.ImportarTabla(e.ctx, gus, e.base.ID, "x", domain.TablaCSV{})
			return err
		},
	} {
		esperar(t, f(), domain.ErrNoExiste, qué+" desde otra organización")
	}
	// «Toda la organización» es la de la base: un sub que se llame igual en otra no entra,
	// ni un actor cuyo tenant sea el id del permiso de otra.
	_, err = e.s.ObtenerBase(e.ctx, domain.Actor{Sub: "ana", Tenant: "globex"}, e.base.ID)
	esperar(t, err, domain.ErrNoExiste, "el mismo sub en otra organización")
}

// Acceso a UNA tabla no es acceso a las demás de la base, ni a la base.
func TestAccesoPorTablaNoSeExtiende(t *testing.T) {
	e := nuevo(t)
	otra, err := e.s.CrearTabla(e.ctx, ana, e.base.ID, "Nóminas")
	e.ok(err)
	sueldo, err := e.s.CrearCampo(e.ctx, ana, otra.ID, domain.EntradaCampo{Name: "Sueldo", Kind: domain.TipoNumero})
	e.ok(err)
	nomina := e.registro(ana, otra.ID, dato(sueldo.ID, `1000`))
	_, err = e.s.ConcederPermiso(e.ctx, ana, e.base.ID, domain.Permiso{TableID: e.tabl.ID, PrincipalKind: "user", PrincipalID: "bea", Role: domain.RolEditor})
	e.ok(err)

	if _, err := e.s.CrearRegistro(e.ctx, bea, e.tabl.ID, dato(e.txt.ID, `"x"`), nil); err != nil {
		t.Fatalf("editora de la tabla no escribe en ella: %v", err)
	}
	_, err = e.s.ListarRegistros(e.ctx, bea, otra.ID, "", 0, "")
	esperar(t, err, domain.ErrNoExiste, "leer otra tabla")
	err = e.s.ExportarTabla(e.ctx, bea, otra.ID, func(Exportacion) error { return nil })
	esperar(t, err, domain.ErrNoExiste, "exportar otra tabla")
	_, err = e.s.ActualizarRegistro(e.ctx, bea, nomina.ID, 1, dato(sueldo.ID, `9`), nil)
	esperar(t, err, domain.ErrNoExiste, "editar un registro de otra tabla")
	esperar(t, e.s.BorrarRegistro(e.ctx, bea, nomina.ID, false), domain.ErrNoExiste, "borrar un registro de otra tabla")
	_, err = e.s.ListarVistas(e.ctx, bea, otra.ID)
	esperar(t, err, domain.ErrNoExiste, "vistas de otra tabla")
	ts, err := e.s.ListarTablas(e.ctx, bea, e.base.ID)
	e.ok(err)
	if len(ts) != 1 || ts[0].ID != e.tabl.ID {
		t.Errorf("ve tablas que no son la suya: %+v", ts)
	}
	_, err = e.s.CrearTabla(e.ctx, bea, e.base.ID, "x")
	esperar(t, err, domain.ErrNoExiste, "crear tabla con acceso solo a una")
}

// Una conexión acotada a una vista con lista de campos enseña SOLO esas columnas.
func TestConectorRespetaLosCamposDeLaVista(t *testing.T) {
	e := nuevo(t)
	e.registro(ana, e.tabl.ID, dato(e.txt.ID, `"Ana"`, e.num.ID, `1200`))
	vista, err := e.s.CrearVista(e.ctx, ana, e.tabl.ID, "Solo nombres", domain.VistaFormulario, domain.ConfigVista{Fields: []string{e.txt.ID}})
	e.ok(err)
	_, token, err := e.s.CrearConexion(e.ctx, ana, e.tabl.ID, vista.ID, "Calcus", "", 1)
	e.ok(err)
	l, err := e.s.LeerPorConector(e.ctx, token, "", "", "test")
	e.ok(err)
	if len(l.Columns) != 1 || l.Columns[0].Name != "Nombre" || len(l.Rows) != 1 || len(l.Rows[0]) != 1 || l.Rows[0][0] != "Ana" {
		t.Errorf("la conexión enseña columnas fuera de la vista: %+v", l)
	}
}

// Una lectura que no cabe en memoria se rechaza en vez de cargarse entera.
func TestLecturaAcotadaEnBytes(t *testing.T) {
	e := nuevo(t)
	gordo := `"` + strings.Repeat("x", 4000) + `"`
	for range 4 {
		e.registro(ana, e.tabl.ID, dato(e.txt.ID, gordo))
	}
	_, token, err := e.s.CrearConexion(e.ctx, ana, e.tabl.ID, "", "Calcus", "", 1)
	e.ok(err)
	previo := maxBytesLectura
	maxBytesLectura = 10_000
	t.Cleanup(func() { maxBytesLectura = previo })

	err = e.s.ExportarTabla(e.ctx, ana, e.tabl.ID, func(Exportacion) error { return nil })
	esperar(t, err, domain.ErrLimite, "exportar por encima del techo")
	_, err = e.s.LeerPorConector(e.ctx, token, "", "", "")
	esperar(t, err, domain.ErrLimite, "conector por encima del techo")
	_, err = e.s.ListarRegistros(e.ctx, ana, e.tabl.ID, "", 0, "")
	esperar(t, err, domain.ErrLimite, "página por encima del techo")
	if _, err := e.s.ListarRegistros(e.ctx, ana, e.tabl.ID, "", 2, ""); err != nil {
		t.Errorf("una página que sí cabe: %v", err)
	}
}
