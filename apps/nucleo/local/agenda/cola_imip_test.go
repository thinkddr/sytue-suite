// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package agenda_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/local/agenda"
)

// correoFalso son unas cuentas de Mail de mentira cuyo SMTP se puede «cortar».
type correoFalso struct {
	mu       sync.Mutex
	buzones  []agenda.Buzon
	falla    bool
	enviados []struct {
		buzon string
		para  []string
		crudo string
	}
}

func (c *correoFalso) Buzones() ([]agenda.Buzon, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.buzones), nil
}

func (c *correoFalso) Enviar(_ context.Context, b string, para []string, crudo []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.falla {
		return errors.New("dial tcp: connect: network is unreachable")
	}
	c.enviados = append(c.enviados, struct {
		buzon string
		para  []string
		crudo string
	}{b, slices.Clone(para), string(crudo)})
	return nil
}

func (c *correoFalso) cortar(si bool) { c.mu.Lock(); c.falla = si; c.mu.Unlock() }

func (c *correoFalso) tomar() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.enviados))
	for _, e := range c.enviados {
		out = append(out, e.buzon+"→"+strings.Join(e.para, ",")+"|"+e.crudo)
	}
	c.enviados = nil
	return out
}

type reloj struct {
	mu sync.Mutex
	t  time.Time
}

func (r *reloj) ahora() time.Time      { r.mu.Lock(); defer r.mu.Unlock(); return r.t }
func (r *reloj) pasar(d time.Duration) { r.mu.Lock(); r.t = r.t.Add(d); r.mu.Unlock() }

// A-11: una invitación por correo que no sale (el SMTP no contesta) no se pierde: queda
// en la cola de la carpeta de datos, tal cual se compuso, y sale al reintentar pasada su
// espera, que crece; «la red volvió» (todo) no espera. Un cambio posterior a las mismas
// personas deja viejo el anterior. Lo que no tenía cuenta de Mail sale cuando la hay, y
// lo de hace más de un mes se tira.
func TestColaIMIPReintentaLoQueNoSalio(t *testing.T) {
	rl := &reloj{t: time.Now()}
	mail := &correoFalso{buzones: []agenda.Buzon{{ID: "m1", Direccion: "ana@example.org", Nombre: "Ana"}}, falla: true}
	cola := &agenda.ColaIMIP{Dir: filepath.Join(t.TempDir(), "agenda", "imip"), Ahora: rl.ahora}
	m := montarCalendario(t, false)
	m.s.Ahora = rl.ahora
	m.s.Invitar = (&agenda.Cartero{Correo: mail, Propia: "ana@example.org", Cola: cola, Ahora: rl.ahora}).Invitar
	inicio := time.Now().AddDate(0, 0, 7).Truncate(time.Hour)
	datos := strings.NewReplacer("%INICIO%", inicio.Format("20060102T150405"), "%FIN%", inicio.Add(time.Hour).Format("20060102T150405")).Replace(reunionICS)
	m.srv.Poner(m.col+"reunion-1.ics", []byte(datos))
	m.sincronizar()
	cambiar := func(uid, de, a string) agenda.Resultado {
		t.Helper()
		o := m.locales()[uid]
		if _, err := m.repo.PonerObjetoCrudo(t.Context(), m.cal, o.ID, "", strings.ReplaceAll(o.Datos, de, a)); err != nil {
			t.Fatal(err)
		}
		return m.sincronizar()
	}

	// Sin red: el evento sube, el correo no sale y espera en la cola, legible.
	res := cambiar("reunion-1", "Sala 2", "Sala 3")
	var fallo agenda.FalloEnvio
	if res.Subidos != 1 || len(res.Invitaciones) != 1 || !errors.As(res.Invitaciones[0], &fallo) || cola.Cuantos() != 1 {
		t.Fatalf("sin red: %+v cola=%d", res, cola.Cuantos())
	}
	p := cola.Pendientes()[0]
	if p.UID != "reunion-1" || p.Metodo != "REQUEST" || !slices.Equal(p.Para, []string{"luis@example.org", "marta@example.org"}) || p.Intentos != 1 {
		t.Fatalf("ficha: %+v", p)
	}
	if eml, err := os.ReadFile(filepath.Join(cola.Dir, p.ID+".eml")); err != nil || !strings.Contains(string(eml), "From: \"Ana\" <ana@example.org>") {
		t.Fatalf("mensaje: %v %q", err, eml)
	}

	// Antes de su espera no se reintenta; pasada, sí, y sale.
	mail.cortar(false)
	if inf := cola.Vaciar(t.Context(), mail, false); inf.Enviados != 0 || inf.Quedan != 1 {
		t.Fatalf("antes de la espera: %+v", inf)
	}
	rl.pasar(agenda.EsperaMinima + time.Second)
	if inf := cola.Vaciar(t.Context(), mail, false); inf.Enviados != 1 || inf.Quedan != 0 {
		t.Fatalf("pasada la espera: %+v", inf)
	}
	if env := mail.tomar(); len(env) != 1 || !strings.HasPrefix(env[0], "m1→luis@example.org,marta@example.org|") || !strings.Contains(env[0], "method=REQUEST") {
		t.Fatalf("enviado: %q", env)
	}

	// Dos cambios seguidos sin red: solo queda el último. La espera crece; «la red
	// volvió» no espera.
	mail.cortar(true)
	cambiar("reunion-1", "Sala 3", "Sala 4")
	cambiar("reunion-1", "Sala 4", "Sala 5")
	if ps := cola.Pendientes(); len(ps) != 1 {
		t.Fatalf("el viejo no se quitó: %+v", ps)
	}
	rl.pasar(agenda.EsperaMinima + time.Second)
	if inf := cola.Vaciar(t.Context(), mail, false); len(inf.Errores) != 1 || inf.Quedan != 1 {
		t.Fatalf("vuelve a fallar: %+v", inf)
	}
	if p := cola.Pendientes()[0]; p.Intentos != 2 || p.Proximo.Sub(rl.ahora()) != 2*agenda.EsperaMinima {
		t.Fatalf("espera: %+v", p)
	}
	mail.cortar(false)
	if inf := cola.Vaciar(t.Context(), mail, true); inf.Enviados != 1 || inf.Quedan != 0 {
		t.Fatalf("la red volvió: %+v", inf)
	}
	if env := mail.tomar(); len(env) != 1 || !strings.Contains(env[0], "Where: Sala 5") || strings.Contains(env[0], "Sala 4") {
		t.Fatalf("el último cambio: %q", env)
	}

	// Organiza una dirección sin cuenta de Mail: se avisa y espera a que la haya.
	sinMail := strings.NewReplacer("UID:reunion-1", "UID:reunion-3", "mailto:ana@example.org", "mailto:ana@icloud.example").Replace(datos)
	m.s.Invitar = (&agenda.Cartero{Correo: mail, Propia: "ana@icloud.example", Cola: cola, Ahora: rl.ahora}).Invitar
	m.srv.Poner(m.col+"reunion-3.ics", []byte(sinMail))
	m.sincronizar()
	res = cambiar("reunion-3", "Sala 2", "Sala 9")
	var sin agenda.SinRemitente
	if len(res.Invitaciones) != 1 || !errors.As(res.Invitaciones[0], &sin) || cola.Cuantos() != 1 || len(mail.tomar()) != 0 {
		t.Fatalf("sin remitente: %+v", res)
	}
	if inf := cola.Vaciar(t.Context(), mail, true); inf.Enviados != 0 || inf.Quedan != 1 || len(inf.Errores) != 0 {
		t.Fatalf("sin cuenta aún: %+v", inf)
	}
	if !cola.Proximo(mail.buzones).IsZero() {
		t.Fatal("lo que espera una cuenta de Mail no tiene reintento")
	}
	mail.mu.Lock()
	mail.buzones = append(mail.buzones, agenda.Buzon{ID: "m2", Direccion: "Ana@iCloud.example"})
	mail.mu.Unlock()
	if inf := cola.Vaciar(t.Context(), mail, false); inf.Enviados != 1 {
		t.Fatalf("con la cuenta ya: %+v", inf)
	}
	if env := mail.tomar(); len(env) != 1 || !strings.HasPrefix(env[0], "m2→") || !strings.Contains(env[0], "Where: Sala 9") {
		t.Fatalf("enviado: %q", env)
	}

	// Lo que lleva más de un mes en la cola se tira.
	mail.cortar(true)
	cambiar("reunion-1", "Sala 5", "Sala 6")
	rl.pasar(agenda.MaxEdadIMIP + time.Hour)
	if inf := cola.Vaciar(t.Context(), mail, true); inf.Caducados != 1 || inf.Quedan != 0 {
		t.Fatalf("caducado: %+v", inf)
	}
	if fs, _ := os.ReadDir(cola.Dir); len(fs) != 0 {
		t.Fatalf("quedaron ficheros: %v", fs)
	}
}
