// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store_test

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/sqlorg"
	"git.sytue.eu/sytue/one/services/cast/db"
)

// TestElSQLLlevaLaOrganizacion: toda sentencia que lee o modifica una tabla con
// `tenant` lo nombra en su predicado. La política RLS no basta —un rol que se la
// salta lee la fila de cualquiera con un `WHERE id = $1`—, así que el filtro tiene
// que estar escrito en la consulta. Ver platform/pkg/sqlorg.
func TestElSQLLlevaLaOrganizacion(t *testing.T) {
	sqlorg.Exigir(t, sqlorg.Opciones{
		Migraciones: db.Migrations,
		Dir:         db.MigrationsDir,
		Paquetes:    []string{"."},
		Excepciones: map[string]string{
			"barrido.go:BorradosCaducados":            "el barrido recorre los vídeos borrados de todas las organizaciones, en segundo plano (app.maintenance)",
			"barrido.go:Eliminar":                     "el barrido elimina por id lo que acaba de listar BorradosCaducados (app.maintenance)",
			"barrido.go:CaducarSubidas":               "el barrido caduca las subidas abandonadas de todas las organizaciones (app.maintenance)",
			"calidades.go:EncolarCalidadesPendientes": "el trabajador encola las calidades pendientes de todas las organizaciones (app.maintenance)",
			"calidades.go:CalidadesEnCola":            "cuenta la cola de transcodificación, que es una para todas las organizaciones",
			"jobs.go:Reclamar":                        "la cola de transcodificación es una para todas las organizaciones: se reclama el siguiente trabajo, sea de quien sea",
			"jobs.go:Terminar":                        "el trabajador cierra por id el trabajo que reclamó; el vídeo se toca con el tenant del trabajo",
			"jobs.go:Fallar":                          "el trabajador falla por id el trabajo que reclamó",
			"videos.go:Publico":                       "un vídeo public/unlisted se ve desde cualquier organización (política videos_publico, app.cast_publico)",
			"videos.go:Explore":                       "la portada pública lista los vídeos public de todas las organizaciones (app.cast_publico)",
			"videos.go:EsInvitado":                    "una persona de otra organización puede estar invitada a un vídeo people (política de persona, app.cast_persona)",
			"videos.go:PorID":                         "/media sirve por id con un token firmado que prueba que se pasó por play (app.maintenance)",
		},
	})
}
