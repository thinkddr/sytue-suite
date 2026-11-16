// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"strings"
	"sync"
	"time"
)

// La revisión de seguridad de una cuenta y sus «Primeros pasos», como funciones puras.
//
// Es el espejo de `revisionDeCuenta` y `primerosPasos` de la web
// (`apps/frontend/packages/shell/src/revisionCuenta.ts`): la portada del portal y el Hub
// nativo enseñan la misma tarjeta, y dos criterios escritos a mano se separarían en el
// primer cambio. Los dos lados recorren los MISMOS vectores
// (`revisionCuenta.vectores.json`, junto a la función web), así que un cambio en uno sin
// el otro se ve en rojo.
//
// La regla de la web se conserva tal cual: un dato que no ha llegado no es un aviso. En
// Go «no ha llegado» es el puntero o la lista a nil; una lista vacía SÍ ha llegado y dice
// que no hay ninguna.

// DiasSinUso es `DIAS_SIN_USO`: pasado este tiempo sin uso, una credencial se da por
// olvidada.
const DiasSinUso = 90

// ClaveRevision es uno de los cinco puntos de la revisión.
type ClaveRevision string

// Los cinco puntos, en el orden en que se enseñan (de más a menos grave).
const (
	RevisionDosPasos       ClaveRevision = "dos_pasos"
	RevisionRecuperacion   ClaveRevision = "recuperacion"
	RevisionSesiones       ClaveRevision = "sesiones"
	RevisionContrasenasApp ClaveRevision = "contrasenas_app"
	RevisionDispositivos   ClaveRevision = "dispositivos"
)

// AnclasRevision es `ANCLAS`: la sección de Mi cuenta que resuelve cada punto.
var AnclasRevision = map[ClaveRevision]string{
	RevisionDosPasos:       "#seguridad",
	RevisionRecuperacion:   "#recuperacion",
	RevisionSesiones:       "#sesiones",
	RevisionContrasenasApp: "#contrasenas-app",
	RevisionDispositivos:   "#dispositivos",
}

// PuntoRevision es `PuntoRevision`.
type PuntoRevision struct {
	Clave ClaveRevision `json:"clave"`
	// Estado es "ok" o "aviso".
	Estado string `json:"estado"`
	// Ancla es la sección de Mi cuenta que lo resuelve, con su `#`.
	Ancla string `json:"ancla"`
	// Cuantos son las cosas que lo provocan; 0 si está bien.
	Cuantos int `json:"cuantos"`
}

// PerfilRevision es lo que importa de `/me`. HasAvatar es un puntero porque la web
// distingue «sin foto» de «no se sabe»: un `/me` sin el campo no pide la foto.
type PerfilRevision struct {
	MFAEnabled    bool   `json:"mfa_enabled"`
	RecoveryEmail string `json:"recovery_email"`
	Phone         string `json:"phone"`
	HasAvatar     *bool  `json:"has_avatar"`
}

// PerfilDe saca de la ficha lo que mira la revisión. `has_avatar` lo pone siempre el
// servicio (`setAvatarFields`), así que aquí sí se sabe.
func PerfilDe(p Profile) *PerfilRevision {
	foto := p.HasAvatar
	return &PerfilRevision{MFAEnabled: p.MFAEnabled, RecoveryEmail: p.RecoveryEmail, Phone: p.Phone, HasAvatar: &foto}
}

// DatosRevision es `DatosRevision`: lo que ya han traído `/me`, las passkeys, las
// sesiones, los dispositivos y las contraseñas de aplicación. Nil es «no ha llegado».
type DatosRevision struct {
	Me             *PerfilRevision  `json:"me"`
	Passkeys       []Passkey        `json:"passkeys"`
	Sesiones       []AccountSession `json:"sesiones"`
	Dispositivos   []AccountDevice  `json:"dispositivos"`
	ContrasenasApp []AppPassword    `json:"contrasenasApp"` //nolint:tagliatelle // el nombre lo fija la web
	// Ahora se recibe para que el resultado no dependa del reloj (y se pueda probar).
	Ahora time.Time `json:"ahora"`
}

func punto(clave ClaveRevision, cuantos int) PuntoRevision {
	estado := "ok"
	if cuantos > 0 {
		estado = "aviso"
	}
	return PuntoRevision{Clave: clave, Estado: estado, Ancla: AnclasRevision[clave], Cuantos: cuantos}
}

// falta es el `cuantos` de un punto de sí o no: 1 si no se cumple.
func falta(cumplido bool) int {
	if cumplido {
		return 0
	}
	return 1
}

// olvidada dice si la fecha es de hace más de DiasSinUso. Sin fecha, no: no se inventan
// avisos.
func olvidada(t, ahora time.Time) bool {
	return !t.IsZero() && ahora.Sub(t) > DiasSinUso*24*time.Hour
}

// RevisionDeCuenta es `revisionDeCuenta`: los puntos de la revisión, en orden.
func RevisionDeCuenta(d DatosRevision) []PuntoRevision {
	out := []PuntoRevision{}
	if d.Me != nil {
		// Una passkey ya es un segundo factor. Si la verificación está apagada y las
		// passkeys no han llegado, no se sabe si hay segundo factor: no se avisa.
		if d.Me.MFAEnabled || d.Passkeys != nil {
			protegida := d.Me.MFAEnabled || len(d.Passkeys) > 0
			out = append(out, punto(RevisionDosPasos, falta(protegida)))
		}
		recuperable := strings.TrimSpace(d.Me.RecoveryEmail) != "" || strings.TrimSpace(d.Me.Phone) != ""
		out = append(out, punto(RevisionRecuperacion, falta(recuperable)))
	}
	if d.Sesiones != nil {
		ajenas := 0
		for _, s := range d.Sesiones {
			if !s.Current {
				ajenas++
			}
		}
		out = append(out, punto(RevisionSesiones, ajenas))
	}
	if d.ContrasenasApp != nil {
		// Nunca usada cuenta desde el alta: la que no ha servido nunca es justo la que
		// conviene revocar.
		viejas := 0
		for _, p := range d.ContrasenasApp {
			uso := p.CreatedAt
			if p.LastUsedAt != nil && !p.LastUsedAt.IsZero() {
				uso = *p.LastUsedAt
			}
			if olvidada(uso, d.Ahora) {
				viejas++
			}
		}
		out = append(out, punto(RevisionContrasenasApp, viejas))
	}
	if d.Dispositivos != nil {
		// De confianza y sin verse en 90 días; el actual nunca cuenta.
		dormidos := 0
		for _, e := range d.Dispositivos {
			if e.Trusted && !e.Current && olvidada(e.LastSeen, d.Ahora) {
				dormidos++
			}
		}
		out = append(out, punto(RevisionDispositivos, dormidos))
	}
	return out
}

// ClavePaso es uno de los «Primeros pasos».
type ClavePaso string

// Los cinco, en el orden en que se enseñan (primero lo que protege la cuenta).
const (
	PasoDosPasos     ClavePaso = "dos_pasos"
	PasoRecuperacion ClavePaso = "recuperacion"
	PasoFoto         ClavePaso = "foto"
	PasoDispositivos ClavePaso = "dispositivos"
	PasoInstalar     ClavePaso = "instalar"
)

// PasoPendiente es `PasoPendiente`: qué queda y dónde se arregla.
type PasoPendiente struct {
	Clave ClavePaso `json:"clave"`
	// Destino es "cuenta" (una sección de Mi cuenta, Ruta con su `#`) o "portal" (una
	// página del portal, Ruta sin prefijo).
	Destino string `json:"destino"`
	Ruta    string `json:"ruta"`
}

// DatosPrimerosPasos es `DatosPrimerosPasos`. Nil es «no ha llegado».
type DatosPrimerosPasos struct {
	Me       *PerfilRevision `json:"me"`
	Passkeys []Passkey       `json:"passkeys"`
	// ContrasenasApp es lo que crea «Conectar tus dispositivos».
	ContrasenasApp []AppPassword `json:"contrasenasApp"` //nolint:tagliatelle // el nombre lo fija la web
	// Instalada dice si Sytue One ya está en este equipo. En las apps nativas, siempre.
	Instalada bool `json:"instalada"`
}

// PrimerosPasos es `primerosPasos`: los pasos que QUEDAN. Vacío ⇒ no hay tarjeta.
func PrimerosPasos(d DatosPrimerosPasos) []PasoPendiente {
	out := []PasoPendiente{}
	// Los dos primeros SON puntos de la revisión: no se vuelve a decidir aquí qué
	// cuenta como «protegida» o «recuperable». El reloj no interviene en ellos.
	for _, p := range RevisionDeCuenta(DatosRevision{Me: d.Me, Passkeys: d.Passkeys}) {
		if p.Estado == "aviso" && (p.Clave == RevisionDosPasos || p.Clave == RevisionRecuperacion) {
			out = append(out, PasoPendiente{Clave: ClavePaso(p.Clave), Destino: "cuenta", Ruta: p.Ancla})
		}
	}
	if d.Me != nil && d.Me.HasAvatar != nil && !*d.Me.HasAvatar {
		out = append(out, PasoPendiente{Clave: PasoFoto, Destino: "cuenta", Ruta: "#perfil"})
	}
	if d.ContrasenasApp != nil && len(d.ContrasenasApp) == 0 {
		out = append(out, PasoPendiente{Clave: PasoDispositivos, Destino: "portal", Ruta: "/dispositivos"})
	}
	if !d.Instalada {
		out = append(out, PasoPendiente{Clave: PasoInstalar, Destino: "portal", Ruta: "/descargas"})
	}
	// Conectar e instalar acompañan, no sostienen la tarjeta: sin nada de la cuenta
	// pendiente, una tarjeta con solo «instala» o «conecta» no se iría nunca.
	for _, p := range out {
		if p.Destino == "cuenta" {
			return out
		}
	}
	return []PasoPendiente{}
}

// PrimerosPasos es `cargarPrimerosPasos` del portal: junta `/me`, las passkeys y las
// contraseñas de aplicación (a la vez y cada una por su lado) y dice qué queda. `hay` es
// false si Sytue ID no ha contestado a NADA: sin datos no hay tarjeta.
//
// Quien llama desde una app nativa pasa `instalada` a true: está instalada.
func (a *Account) PrimerosPasos(ctx context.Context, instalada bool) (pasos []PasoPendiente, hay bool) {
	var (
		wg             sync.WaitGroup
		me             *PerfilRevision
		passkeys       []Passkey
		contrasenasApp []AppPassword
	)
	wg.Add(3)
	go func() {
		defer wg.Done()
		if p, err := a.Me(ctx); err == nil {
			me = PerfilDe(p)
		}
	}()
	go func() {
		defer wg.Done()
		if l, err := a.Passkeys(ctx); err == nil {
			passkeys = llegada(l)
		}
	}()
	go func() {
		defer wg.Done()
		if l, err := a.AppPasswords(ctx); err == nil {
			contrasenasApp = llegada(l)
		}
	}()
	wg.Wait()
	if me == nil && passkeys == nil && contrasenasApp == nil {
		return nil, false
	}
	return PrimerosPasos(DatosPrimerosPasos{Me: me, Passkeys: passkeys, ContrasenasApp: contrasenasApp, Instalada: instalada}), true
}

// llegada convierte la lista de una respuesta buena en «ha llegado»: un `[]` del
// servicio, o el campo ausente, es ninguna, no «no se sabe».
func llegada[T any](l []T) []T {
	if l == nil {
		return []T{}
	}
	return l
}
