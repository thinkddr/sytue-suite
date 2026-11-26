// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package agenda

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/local/biblioteca"
	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
)

// La cola de las invitaciones por correo que no salieron (A-11). Un correo iMIP que
// el SMTP no aceptó (sin red, el servidor caído) o que no tenía por dónde salir (ninguna
// cuenta de Mail con la dirección del organizador) no se pierde: se guarda TAL CUAL se
// compuso, con su ficha, en la carpeta de datos de Calendar
// (`<Datos>/calendar/agenda/imip/<id>.eml` y `<id>.json`, legibles), y se reintenta con
// espera creciente (de [EsperaMinima] a [EsperaMaxima], como la subida de los eventos)
// al arrancar, en cada vuelta de la sincronización y, sin esperar, cuando la red
// vuelve. Lo que espera una cuenta de Mail sale en cuanto la hay.
//
// Lo que un envío posterior deja viejo se quita: si a las mismas personas (o a más) les
// sale después otro mensaje del mismo evento, el anterior ya no hace falta. Y lo que
// lleva más de [MaxEdadIMIP] en la cola se tira: una invitación de hace un mes ya no
// sirve a nadie.

// MaxEdadIMIP es cuánto, como mucho, espera un correo en la cola.
const MaxEdadIMIP = 30 * 24 * time.Hour

// PendienteIMIP es la ficha de un correo de la cola.
type PendienteIMIP struct {
	ID string `json:"id"`
	// Direccion es la de la cuenta de Mail por la que tiene que salir: la del
	// organizador o la de quien contesta.
	Direccion string   `json:"direccion"`
	Para      []string `json:"para"`
	// UID, Metodo (REQUEST, CANCEL, REPLY) y Resumen son del evento.
	UID      string    `json:"uid"`
	Metodo   string    `json:"metodo"`
	Resumen  string    `json:"resumen"`
	Creado   time.Time `json:"creado"`
	Intentos int       `json:"intentos,omitempty"`
	Proximo  time.Time `json:"proximo"`
	Error    string    `json:"error,omitempty"`
}

// ColaIMIP es la cola de una carpeta.
type ColaIMIP struct {
	Dir string
	// Ahora es el reloj (las pruebas lo cambian).
	Ahora func() time.Time

	mu       sync.Mutex // los ficheros
	vaciando sync.Mutex // un vaciado cada vez
}

// InformeCola es lo que hizo un vaciado.
type InformeCola struct {
	Enviados, Caducados int
	// Quedan son los que siguen en la cola al acabar.
	Quedan int
	// Errores son los de los envíos que volvieron a fallar (FalloEnvio).
	Errores []error
}

func (q *ColaIMIP) ahora() time.Time {
	if q.Ahora != nil {
		return q.Ahora()
	}
	return time.Now()
}

// espera es la de un reintento tras n intentos fallidos (la de la subida de eventos).
func espera(n int) time.Duration {
	if n <= 0 {
		return 0
	}
	e := EsperaMinima << min(n-1, 12)
	if e > EsperaMaxima || e <= 0 {
		e = EsperaMaxima
	}
	return e
}

func (q *ColaIMIP) ruta(id, ext string) string { return filepath.Join(q.Dir, id+ext) }

// Guardar pone un correo en la cola. p.Intentos son los que ya fallaron (0 si no se
// pudo ni intentar: sin cuenta de Mail); el siguiente intento espera lo que toca. Antes
// quita lo que este deja viejo.
func (q *ColaIMIP) Guardar(p PendienteIMIP, crudo []byte) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := os.MkdirAll(q.Dir, 0o700); err != nil {
		return err
	}
	q.superar(p.UID, p.Para)
	ahora := q.ahora()
	var r [4]byte
	_, _ = rand.Read(r[:])
	p.ID = ahora.UTC().Format("20060102T150405.000000000") + "-" + hex.EncodeToString(r[:])
	p.Creado, p.Proximo = ahora, ahora.Add(espera(p.Intentos))
	p.Direccion = strings.ToLower(strings.TrimSpace(p.Direccion))
	// Primero el mensaje: una ficha sin su mensaje no se podría mandar.
	if err := biblioteca.EscribirAtomico(q.ruta(p.ID, ".eml"), crudo); err != nil {
		return err
	}
	return espacio.EscribirJSON(q.ruta(p.ID, ".json"), p)
}

// Superar quita de la cola los correos de ese evento a personas que están todas en
// para: el que acaba de salir (o de entrar en la cola) los deja viejos.
func (q *ColaIMIP) Superar(uid string, para []string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.superar(uid, para)
}

func (q *ColaIMIP) superar(uid string, para []string) {
	if uid == "" {
		return
	}
	for _, p := range q.leer() {
		if p.UID != uid {
			continue
		}
		todos := true
		for _, x := range p.Para {
			if !slices.ContainsFunc(para, func(y string) bool { return strings.EqualFold(x, y) }) {
				todos = false
			}
		}
		if todos {
			q.quitar(p.ID)
		}
	}
}

func (q *ColaIMIP) quitar(id string) {
	_ = os.Remove(q.ruta(id, ".json"))
	_ = os.Remove(q.ruta(id, ".eml"))
}

// leer son las fichas de la cola, de la más vieja a la más nueva. Una ficha sin su
// mensaje no sirve y se quita.
func (q *ColaIMIP) leer() []PendienteIMIP {
	ms, _ := filepath.Glob(filepath.Join(q.Dir, "*.json"))
	slices.Sort(ms)
	out := make([]PendienteIMIP, 0, len(ms))
	for _, m := range ms {
		var p PendienteIMIP
		if err := espacio.LeerJSON(m, &p); err != nil || p.ID == "" || p.ID+".json" != filepath.Base(m) {
			continue
		}
		if _, err := os.Stat(q.ruta(p.ID, ".eml")); err != nil {
			_ = os.Remove(m)
			continue
		}
		out = append(out, p)
	}
	return out
}

// Pendientes son los correos de la cola, del más viejo al más nuevo.
func (q *ColaIMIP) Pendientes() []PendienteIMIP {
	if q == nil {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.leer()
}

// Cuantos son los correos que esperan.
func (q *ColaIMIP) Cuantos() int { return len(q.Pendientes()) }

// Proximo es cuándo toca el primer reintento (cero si la cola está vacía). Lo que
// espera una cuenta de Mail no cuenta: no sale por esperar.
func (q *ColaIMIP) Proximo(buzones []Buzon) time.Time {
	var t time.Time
	for _, p := range q.Pendientes() {
		if _, ok := buzonDe(buzones, p.Direccion); !ok {
			continue
		}
		if t.IsZero() || p.Proximo.Before(t) {
			t = p.Proximo
		}
	}
	return t
}

// Vaciar manda lo que toca de la cola por c: lo que ya pasó su espera o, con todo, todo
// (al arrancar, cuando la red vuelve). Lo que sale se quita; lo que vuelve a fallar
// espera más; lo que no tiene cuenta de Mail por la que salir se queda como estaba.
func (q *ColaIMIP) Vaciar(ctx context.Context, c Correo, todo bool) InformeCola {
	var inf InformeCola
	if q == nil {
		return inf
	}
	q.vaciando.Lock()
	defer q.vaciando.Unlock()
	pend := q.Pendientes()
	if len(pend) == 0 {
		return inf
	}
	buzones, err := c.Buzones()
	if err != nil {
		inf.Quedan = len(pend)
		inf.Errores = append(inf.Errores, err)
		return inf
	}
	for _, p := range pend {
		if ctx.Err() != nil {
			break
		}
		ahora := q.ahora()
		if ahora.Sub(p.Creado) > MaxEdadIMIP {
			q.mu.Lock()
			q.quitar(p.ID)
			q.mu.Unlock()
			inf.Caducados++
			continue
		}
		b, ok := buzonDe(buzones, p.Direccion)
		if !ok || (!todo && ahora.Before(p.Proximo)) {
			continue
		}
		crudo, err := os.ReadFile(q.ruta(p.ID, ".eml"))
		if err != nil {
			continue // otro vaciado o un envío más nuevo ya la quitó
		}
		err = c.Enviar(ctx, b.ID, p.Para, crudo)
		q.mu.Lock()
		_, sigue := os.Stat(q.ruta(p.ID, ".json"))
		switch {
		case err == nil:
			q.quitar(p.ID)
			inf.Enviados++
		case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
		case sigue == nil:
			p.Intentos++
			p.Proximo, p.Error = q.ahora().Add(espera(p.Intentos)), err.Error()
			_ = espacio.EscribirJSON(q.ruta(p.ID, ".json"), p)
			inf.Errores = append(inf.Errores, FalloEnvio{Direccion: b.Direccion, Resumen: p.Resumen, Err: err})
		}
		q.mu.Unlock()
	}
	inf.Quedan = q.Cuantos()
	return inf
}
