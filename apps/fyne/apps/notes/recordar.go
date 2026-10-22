// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"context"
	"log/slog"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// # Abrir un cuaderno cifrado con Touch ID
//
// Decisión del dueño (2026-09-24): como OPCIÓN, al abrir un cuaderno cifrado con su
// contraseña la app ofrece recordarlo en este equipo. Lo que se recuerda es la clave ya
// derivada (la de `api.AbrirCifradoNotas`), en el almacén seguro del sistema —el llavero
// en escritorio, el Keychain en iOS, el fichero cifrado con el Keystore en Android—, y
// leerla pide antes al dueño que diga quién es con lo que tenga el sistema, la misma
// verificación que el bloqueo de la sesión (`escritorio.Verificar`). Aquí no hay
// criptografía nueva: la del cuaderno es la de la web y no cambia.
//
// La contraseña sigue valiendo siempre: cancelar Touch ID, o que falle, lleva a pedirla.
// Si la clave guardada ya no abre el cuaderno —la contraseña se cambió desde la web—, se
// olvida sola. Y sin la casilla marcada todo queda como en la web.
//
// Protege de lo mismo que el bloqueo de la sesión: de quien se sienta delante de un
// equipo desbloqueado. No de un programa del propio usuario, que ya puede leer el llavero.

// Recordador es lo que hace falta del sistema: cómo se llama su verificación («Touch
// ID», «Windows Hello»), la verificación misma y el almacén. Lo rellena `cmd/notes` con
// `escritorio.Verificar` y `auth.*Secreto`; el guardián, con dobles.
type Recordador struct {
	Metodo    string
	Motivo    string // lo que dice el diálogo del sistema, ya traducido
	Verificar func(ctx context.Context, motivo string) error
	Leer      func(cuenta string) ([]byte, error)
	Guardar   func(cuenta string, datos []byte) error
	Borrar    func(cuenta string) error
}

// cuentaCuaderno es la entrada del almacén seguro de la clave de un cuaderno.
func cuentaCuaderno(id string) string { return "notes-" + id }

// conRecordada devuelve la clave recordada del cuaderno si la hay, el dueño lo verifica
// y todavía abre el cuaderno. `recordada` dice si había una guardada, para enseñar la
// casilla marcada si al final hay que pedir la contraseña. Bloquea: va en su rutina.
func (p *Pantalla) conRecordada(c api.NotesCrypto, id string) (clave []byte, recordada bool) {
	r := p.Recordar
	guardada, err := r.Leer(cuentaCuaderno(id))
	if err != nil || len(guardada) == 0 {
		return nil, false
	}
	if err := r.Verificar(context.Background(), r.Motivo); err != nil {
		return nil, true // canceló o no se reconoció: la contraseña, con la casilla marcada
	}
	if api.ComprobarClaveNotas(c, guardada) != nil {
		// La contraseña se cambió: la guardada ya no vale y no se ofrece más.
		p.olvidarClave(id)
		return nil, false
	}
	return guardada, true
}

// recordarClave guarda la clave (con la casilla marcada) u olvida la que hubiera (sin
// ella). Guardarla pide antes verificar al dueño: así se sabe que el sistema lo reconoce
// y no la ata a Touch ID quien solo conoce la contraseña. Si no se verifica, el cuaderno
// se abre igual; solo no se recuerda.
func (p *Pantalla) recordarClave(id string, clave []byte, recordar bool) {
	r := p.Recordar
	if !recordar {
		p.olvidarClave(id)
		return
	}
	if err := r.Verificar(context.Background(), r.Motivo); err != nil {
		return
	}
	if err := r.Guardar(cuentaCuaderno(id), clave); err != nil {
		slog.Warn("notes: no se ha podido recordar la clave del cuaderno", "error", err)
	}
}

// olvidarClave quita la clave recordada de un cuaderno: al desmarcar la casilla, cuando ya
// no abre, o al borrar el cuaderno.
func (p *Pantalla) olvidarClave(id string) {
	if p.Recordar == nil {
		return
	}
	if err := p.Recordar.Borrar(cuentaCuaderno(id)); err != nil {
		slog.Warn("notes: no se ha podido olvidar la clave del cuaderno", "error", err)
	}
}
