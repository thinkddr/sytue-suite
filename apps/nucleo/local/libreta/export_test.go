// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package libreta

// FallarAlComprobar hace fallar (f no nil) la comprobación de la migración.
func FallarAlComprobar(f func() error) { fallarAlComprobar = f }

// ConsultasConIndice son las consultas de las lecturas y de lo que leen las escrituras,
// con el índice que tiene que usar cada una (TestLasConsultasVanPorIndice).
var ConsultasConIndice = map[string]string{
	sqlContactoEntero:             "fila_contacto",
	sqlGrupoEntero:                "fila_grupo",
	sqlContactosFilas + "?,?)":    "sqlite_autoindex_fila_1",
	sqlContactosTarjetas + "?,?)": "sqlite_autoindex_fila_1",
	sqlContactosCanales + "?,?)":  "fila_contacto",
	sqlTodos:                      "contacto_vivos",
	sqlContactos:                  "contacto_vivos",
	sqlCumpleanos:                 "contacto_cumple",
	sqlBuscar:                     "contacto_vivos",
	sqlConTarjeta:                 "sqlite_autoindex_fila_1",
	sqlFotoPorUID:                 "contacto_uid_limpio",
	sqlFavoritos:                  "contacto_favoritos",
	sqlTarjetas:                   "contacto_libreta",
	sqlPapelera:                   "contacto_papelera",
	sqlAncla:                      "sqlite_autoindex_contacto_1",
	sqlPagina:                     "contacto_libreta",
	sqlPaginaDesde:                "contacto_libreta",
	// Lo que leen las escrituras para hacer su parte (escrituras.go).
	`SELECT id FROM contacto WHERE libreta = ? AND uid = ? AND ` + vivos:         "contacto_uid",
	`SELECT id FROM contacto WHERE uid_limpio = ? AND ` + vivos:                  "contacto_uid_limpio",
	`SELECT id FROM contacto WHERE uid IN (?,?)`:                                 "contacto_uid",
	`SELECT id FROM contacto WHERE id IN (?,?)`:                                  "sqlite_autoindex_contacto_1",
	`SELECT id FROM grupo WHERE libreta = ? AND uid_limpio = ?`:                  "grupo_uid",
	`SELECT DISTINCT grupo FROM fila WHERE clase = 'member' AND contacto IN (?)`: "fila_contacto",
	`SELECT DISTINCT c.id FROM correo e JOIN contacto c ON c.id = e.contacto
				WHERE e.libreta = ? AND e.direccion IN (?) AND c.borrado IS NULL AND c.oculto = 0`: "correo_direccion",
	`SELECT uid, valor FROM favorito WHERE uid IN (?)`: "sqlite_autoindex_favorito_1",
	// Lo que hace guardar (guardar.go).
	`SELECT libreta FROM contacto WHERE id = ?`:        "sqlite_autoindex_contacto_1",
	`DELETE FROM correo WHERE contacto = ?`:            "correo_contacto",
	`UPDATE contacto SET oculto = ? WHERE libreta = ?`: "contacto_de",
	`DELETE FROM fila WHERE clase = ? AND id = ?`:      "sqlite_autoindex_fila_1",
}
