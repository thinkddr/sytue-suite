// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/zalando/go-keyring"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

var tokenAvisos = strings.Repeat("ab01", 8)

// bancoDespertar es una suite de iOS compilada con SYTUE_DESPERTAR_APPLE, con escena,
// con sesión, un widget que publicar y un Hub que apunta lo que se le pide.
type bancoDespertar struct {
	s        *Suite
	escena   *EscenaDePrueba
	mu       sync.Mutex
	pedidos  []string // método, ruta y cuerpo
	lecturas atomic.Int32
	// sinRed hace que el Hub conteste 503 a todo lo de aparatos, como sin cobertura.
	sinRed atomic.Bool
	// registroLento hace que el Hub tarde en contestar al registro: hasta que quien pide
	// corta, o un segundo.
	registroLento atomic.Bool
}

func nuevoBancoDespertar(t *testing.T, instalados int) *bancoDespertar {
	t.Helper()
	aparte(t)
	t.Cleanup(widgets.SimularLector(true))
	t.Cleanup(widgets.SimularInstalados(instalados))
	app := test.NewApp()
	t.Cleanup(app.Quit)
	b := &bancoDespertar{}
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		b.mu.Lock()
		b.pedidos = append(b.pedidos, r.Method+" "+strings.TrimPrefix(r.URL.Path, "/api/v1")+" "+string(cuerpo))
		b.mu.Unlock()
		if strings.HasPrefix(r.URL.Path, "/api/v1/me/push/") {
			if b.sinRed.Load() {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			if r.URL.Path == "/api/v1/me/push/apns" && b.registroLento.Load() {
				select {
				case <-r.Context().Done():
				case <-time.After(time.Second):
				}
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(hub.Close)
	b.s = suiteConSesion(t, widgetApp{app}, "ana")
	b.escena = b.s.ProbarSinEscena()
	b.escena.ProbarAvisos(tokenAvisos)
	b.escena.Conectar()
	b.s.Hub = api.NewHub(hub.URL, tokenFijo("tok"), nil)
	b.s.ProbarWidgetsCon(widgets.NuevoRelojDePrueba(time.Now()).Opciones(widgets.OpcionesPublicador{}))
	b.s.PublicadorWidget(func(context.Context) (widgets.Detail, error) {
		b.lecturas.Add(1)
		return conDocumentos(4), nil
	})
	return b
}

// aparatos son las peticiones al Hub de registrar u olvidar aparatos.
func (b *bancoDespertar) aparatos() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []string
	for _, p := range b.pedidos {
		if strings.Contains(p, "/me/push/apns") {
			out = append(out, p)
		}
	}
	return out
}

func (b *bancoDespertar) esperar(t *testing.T) {
	t.Helper()
	ctx, parar := context.WithTimeout(context.Background(), 5*time.Second)
	defer parar()
	fyne.DoAndWait(func() {})
	b.s.esperarDespertar(ctx)
}

// encenderPara deja el interruptor encendido como lo deja `sujeto` al tocarlo.
func encenderPara(t *testing.T, sujeto string) {
	t.Helper()
	if err := ajustes.Cambiar(func(a *ajustes.Ajustes) {
		a.DespertarWidgets, a.DespertarDe = true, huellaDespertar(sujeto)
	}); err != nil {
		t.Fatal(err)
	}
}

const (
	registrar = `POST /me/push/apns {"bundle":"eu.sytue.docus","token":"`
	olvidar   = `POST /me/push/apns/forget {"token":"`
)

// TestElDespertarSoloSeRegistraConElInterruptor: con el interruptor apagado (por
// defecto) no se pide ni el token; encendido, la app se registra con su bundle y su
// token y lo guarda en el almacén para olvidarlo al salir; volver a mirar no repite.
//
// VISTO EN ROJO sin mirar ajustes.DespertarWidgets: se registraba sin que la persona
// lo hubiera encendido.
func TestElDespertarSoloSeRegistraConElInterruptor(t *testing.T) {
	b := nuevoBancoDespertar(t, 1)
	b.s.Arrancar(t.Context())
	b.esperar(t)
	if got := b.aparatos(); len(got) != 0 || b.escena.PedidosDeToken() != 0 {
		t.Fatalf("apagado por defecto: %v y %d tokens pedidos", got, b.escena.PedidosDeToken())
	}
	if !b.s.DespertarDisponible() {
		t.Fatal("con el modo de fondo y un widget, el interruptor tiene que salir")
	}
	if err := b.s.PonerDespertar(true); err != nil {
		t.Fatal(err)
	}
	b.esperar(t)
	if got := b.aparatos(); len(got) != 1 || got[0] != registrar+tokenAvisos+`"}` {
		t.Fatalf("aparatos = %q", got)
	}
	if tok, _ := auth.LeerSecreto(prefijoDespertar + "eu.sytue.docus"); string(tok) != tokenAvisos {
		t.Fatalf("token guardado = %q", tok)
	}
	b.s.sincronizarDespertar()
	b.esperar(t)
	if got := b.aparatos(); len(got) != 1 {
		t.Fatalf("volver a mirar registra otra vez: %q", got)
	}
}

// TestQuitarElUltimoWidgetOlvidaElAparato: sin widget puesto no hay nada que despertar
// y la app se olvida, con su token y solo el suyo. Visto en rojo sin mirar Instalados.
func TestQuitarElUltimoWidgetOlvidaElAparato(t *testing.T) {
	b := nuevoBancoDespertar(t, 1)
	encenderPara(t, "ana")
	b.s.Arrancar(t.Context())
	b.esperar(t)
	t.Cleanup(widgets.SimularInstalados(0))
	b.s.sincronizarDespertar() // lo que hace volver a primer plano
	b.esperar(t)
	got := b.aparatos()
	if len(got) != 2 || got[1] != olvidar+tokenAvisos+`"}` {
		t.Fatalf("aparatos = %q, quiero registrar y olvidar", got)
	}
	if tok, _ := auth.LeerSecreto(prefijoDespertar + "eu.sytue.docus"); tok != nil {
		t.Fatal("olvidado en el Hub y sigue en el almacén")
	}
}

// guardarDespertares guarda los tokens del despertar de otras apps como en un iPhone:
// cada una en su cuenta del grupo de la suite, pero el índice de cuentas del almacén es
// de cada app, así que la que sale no ve en él los de las demás.
func guardarDespertares(t *testing.T, tokens map[string]string) {
	t.Helper()
	for bundle, tok := range tokens {
		if err := auth.GuardarSecreto(prefijoDespertar+bundle, []byte(tok)); err != nil {
			t.Fatal(err)
		}
	}
	if err := keyring.Delete("eu.sytue.one", "indice-secretos"); err != nil {
		t.Fatal(err)
	}
}

// TestSalirOlvidaLosAparatosDeLasQuince: al salir, la app olvida en el Hub los tokens
// de TODAS las apps de este aparato —las demás pueden no estar corriendo, y luego no
// tendrían sesión—, con la sesión que aún vale, antes de revocarla. Apagar el
// interruptor hace lo mismo. Salir, además, lo apaga: quien entre después no queda
// registrada sin pedirlo.
//
// VISTO EN ROJO sin olvidarDespertares en SalirConError: el Hub seguía despertando a
// las apps de una persona que ya no estaba; buscando las cuentas en el índice del
// almacén (que en iOS es de cada app): solo se olvidaba la de la app que salía; y sin
// apagar DespertarWidgets en SalirConError.
func TestSalirOlvidaLosAparatosDeLasQuince(t *testing.T) {
	for _, caso := range []string{"salir", "apagar"} {
		t.Run(caso, func(t *testing.T) {
			b := nuevoBancoDespertar(t, 1)
			encenderPara(t, "ana")
			otro := strings.Repeat("cd02", 8)
			guardarDespertares(t, map[string]string{"eu.sytue.docus": tokenAvisos, "eu.sytue.mail": otro})
			if caso == "salir" {
				_ = b.s.SalirConError()
			} else if err := b.s.PonerDespertar(false); err != nil {
				t.Fatal(err)
			}
			b.esperar(t)
			got := b.aparatos()
			slices.Sort(got)
			quiere := []string{olvidar + tokenAvisos + `"}`, olvidar + otro + `"}`}
			slices.Sort(quiere)
			if !slices.Equal(got, quiere) {
				t.Fatalf("aparatos = %q, quiero %q", got, quiere)
			}
			for _, bundle := range []string{"eu.sytue.docus", "eu.sytue.mail"} {
				if tok, _ := auth.LeerSecreto(prefijoDespertar + bundle); tok != nil {
					t.Fatalf("queda en el almacén el de %s", bundle)
				}
			}
			if ajustes.Leer().DespertarWidgets {
				t.Fatal("el interruptor sigue encendido")
			}
		})
	}
}

// TestApagarSinRedReintentaElOlvido: apagar el interruptor sin red deja la casilla
// apagada, pero el token se queda en el almacén y la vuelta siguiente, ya con red, lo
// olvida en el Hub. Al salir no hay sesión con que reintentarlo: se borra igual.
//
// VISTO EN ROJO borrando del almacén todos los tokens aunque el Hub no los olvidara:
// la fila se quedaba en el Hub hasta la purga del mes, despertando la app.
func TestApagarSinRedReintentaElOlvido(t *testing.T) {
	b := nuevoBancoDespertar(t, 1)
	if err := b.s.PonerDespertar(true); err != nil {
		t.Fatal(err)
	}
	b.s.Arrancar(t.Context())
	b.esperar(t)
	b.sinRed.Store(true)
	if err := b.s.PonerDespertar(false); err != nil {
		t.Fatal(err)
	}
	b.esperar(t)
	if tok, _ := auth.LeerSecreto(prefijoDespertar + "eu.sytue.docus"); string(tok) != tokenAvisos {
		t.Fatal("sin red, el token tiene que quedarse para reintentar el olvido")
	}
	b.sinRed.Store(false)
	b.s.sincronizarDespertar() // lo que hace volver a primer plano
	b.esperar(t)
	got := b.aparatos()
	if len(got) < 3 || got[len(got)-1] != olvidar+tokenAvisos+`"}` {
		t.Fatalf("aparatos = %q, quiero el olvido reintentado al final", got)
	}
	if tok, _ := auth.LeerSecreto(prefijoDespertar + "eu.sytue.docus"); tok != nil {
		t.Fatal("olvidado en el Hub y sigue en el almacén")
	}

	guardarDespertares(t, map[string]string{"eu.sytue.mail": tokenAvisos})
	b.sinRed.Store(true)
	_ = b.s.SalirConError()
	if tok, _ := auth.LeerSecreto(prefijoDespertar + "eu.sytue.mail"); tok != nil {
		t.Fatal("al salir no hay con qué reintentar: se borra")
	}
}

// TestUnaVueltaNoRegistraTrasElOlvido: lo que olvida los de las quince (salir, el
// bloqueo) va antes de que el estado cambie; una vuelta que llegue en medio, con el
// mismo estado, no vuelve a registrar. Encender el interruptor levanta el veto.
// VISTO EN ROJO sin despertarSuite.vetado: la vuelta registraba otra vez.
func TestUnaVueltaNoRegistraTrasElOlvido(t *testing.T) {
	b := nuevoBancoDespertar(t, 1)
	if err := b.s.PonerDespertar(true); err != nil {
		t.Fatal(err)
	}
	b.s.Arrancar(t.Context())
	b.esperar(t)
	b.s.olvidarDespertares(t.Context(), false)
	b.s.sincronizarDespertar()
	b.esperar(t)
	got := b.aparatos()
	if len(got) != 2 || got[1] != olvidar+tokenAvisos+`"}` {
		t.Fatalf("aparatos = %q, quiero registrar y olvidar, sin volver a registrar", got)
	}
	if err := b.s.PonerDespertar(true); err != nil {
		t.Fatal(err)
	}
	b.esperar(t)
	if got := b.aparatos(); len(got) != 3 || got[2] != registrar+tokenAvisos+`"}` {
		t.Fatalf("aparatos = %q, quiero que encender vuelva a registrar", got)
	}
}

// TestLosBundlesDelDespertarSonLosDeLasApps ata bundlesDespertar a las apps de la suite
// (el `ID` de cada FyneApp.toml, sin la demo): una app nueva sin entrar aquí quedaría
// registrada en el Hub tras salir.
func TestLosBundlesDelDespertarSonLosDeLasApps(t *testing.T) {
	tomls, err := filepath.Glob(filepath.Join("..", "cmd", "*", "FyneApp.toml"))
	if err != nil || len(tomls) < 10 {
		t.Fatalf("no encuentro las apps: %v (%d)", err, len(tomls))
	}
	id := regexp.MustCompile(`(?m)^\s*ID\s*=\s*"([^"]+)"`)
	esquema := regexp.MustCompile(`(?m)^\s*esquema\s*=\s*"sytue-([^"]+)"`)
	var deLasApps []string
	porProducto := map[string]string{}
	for _, p := range tomls {
		crudo, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		m := id.FindSubmatch(crudo)
		if m == nil {
			t.Fatalf("%s sin ID", p)
		}
		if b := string(m[1]); b != "eu.sytue.fyne.demo" {
			deLasApps = append(deLasApps, b)
			if e := esquema.FindSubmatch(crudo); e != nil {
				porProducto[string(e[1])] = b
			}
		}
	}
	slices.Sort(deLasApps)
	if !slices.Equal(deLasApps, bundlesDespertar) {
		t.Fatalf("bundlesDespertar = %v\nlas apps son     %v", bundlesDespertar, deLasApps)
	}
	// Y cada producto, con el paquete de SU app: el relevo de Android avisa por él.
	if !maps.Equal(porProducto, bundlePorProducto) {
		t.Fatalf("bundlePorProducto = %v\nlas apps dicen    %v", bundlePorProducto, porProducto)
	}
}

// TestElAvisoSilenciosoRefrescaElWidget: el aviso del Hub entra por la misma puerta que
// el refresco periódico: lee con la Fuente, publica y contesta «hay datos nuevos»; con
// el teléfono bloqueado no lee nada. Visto en rojo con avisoSilencioso sin refrescar.
func TestElAvisoSilenciosoRefrescaElWidget(t *testing.T) {
	b := nuevoBancoDespertar(t, 1)
	b.s.Arrancar(t.Context())
	b.esperar(t)
	nuevos, ok := b.escena.AvisoSilencioso(t.Context())
	if !nuevos || !ok || b.lecturas.Load() != 1 || publicado(t, "eu.sytue.docus") != 4 {
		t.Fatalf("aviso = (%v, %v), %d lecturas, %d en el widget", nuevos, ok, b.lecturas.Load(), publicado(t, "eu.sytue.docus"))
	}
	b.escena.Bloquear(true)
	nuevos, ok = b.escena.AvisoSilencioso(t.Context())
	if nuevos || !ok || b.lecturas.Load() != 1 {
		t.Fatalf("bloqueado: (%v, %v) con %d lecturas; quiero no leer", nuevos, ok, b.lecturas.Load())
	}
}

// TestSinElModoDeFondoNoHayDespertar: una compilación sin SYTUE_DESPERTAR_APPLE no
// enseña el interruptor ni habla con el Hub de aparatos, aunque esté encendido.
func TestSinElModoDeFondoNoHayDespertar(t *testing.T) {
	b := nuevoBancoDespertar(t, 1)
	b.escena.ProbarAvisos("")
	encenderPara(t, "ana")
	b.s.Arrancar(t.Context())
	b.esperar(t)
	if b.s.DespertarDisponible() || len(b.aparatos()) != 0 {
		t.Fatalf("sin el modo de fondo: disponible %v, %q", b.s.DespertarDisponible(), b.aparatos())
	}
	// Ni al salir: sin registro posible no se lee el almacén seguro (en escritorio, quince
	// lecturas del llavero) ni se habla con el Hub. Visto en rojo sin la salida temprana
	// de olvidarDespertares.
	guardarDespertares(t, map[string]string{"eu.sytue.mail": tokenAvisos})
	_ = b.s.SalirConError()
	if got := b.aparatos(); len(got) != 0 {
		t.Fatalf("sin el modo de fondo, salir ha hablado con el Hub: %q", got)
	}
}

// TestElInterruptorEsDeQuienLoEncendio: encendido por otra persona cuya sesión se acabó
// sin salir (caducó, se revocó), quien entra después lo encuentra apagado y no se la
// registra en el Hub sin pedirlo.
//
// VISTO EN ROJO mirando solo ajustes.DespertarWidgets: se registraba a «ana» con el
// interruptor que encendió «berta».
func TestElInterruptorEsDeQuienLoEncendio(t *testing.T) {
	b := nuevoBancoDespertar(t, 1)
	encenderPara(t, "berta")
	b.s.Arrancar(t.Context())
	b.esperar(t)
	if got := b.aparatos(); len(got) != 0 || b.s.DespertarEncendido() {
		t.Fatalf("con el interruptor de otra persona: %q, encendido %v", got, b.s.DespertarEncendido())
	}
	if err := b.s.PonerDespertar(true); err != nil {
		t.Fatal(err)
	}
	b.esperar(t)
	if got := b.aparatos(); len(got) != 1 || got[0] != registrar+tokenAvisos+`"}` || !b.s.DespertarEncendido() {
		t.Fatalf("encendido por ella: %q, encendido %v", got, b.s.DespertarEncendido())
	}
}

// orgsAlHub es Sytue ID apuntando el cambio de organización en la misma lista que las
// peticiones al Hub, para ver el orden.
type orgsAlHub struct{ b *bancoDespertar }

func (o orgsAlHub) Organizaciones(context.Context) ([]api.Organizacion, error) { return nil, nil }

func (o orgsAlHub) CambiarOrganizacion(_ context.Context, id string) error {
	o.b.mu.Lock()
	o.b.pedidos = append(o.b.pedidos, "cambiar "+id)
	o.b.mu.Unlock()
	return nil
}

// TestCambiarDeOrganizacionOlvidaLosAparatos: las filas de aparatos son de cada
// organización y `forget` borra en la del token. Cambiar de organización olvida los de
// las quince con la sesión de la de antes, ANTES del cambio, y la vuelta siguiente
// registra en la nueva.
//
// VISTO EN ROJO sin olvidarDespertares en CambiarOrganizacion: solo se registraba en la
// nueva y la fila de la de antes seguía despertando el iPhone tras salir.
func TestCambiarDeOrganizacionOlvidaLosAparatos(t *testing.T) {
	b := nuevoBancoDespertar(t, 1)
	b.s.Arrancar(t.Context())
	if err := b.s.PonerDespertar(true); err != nil {
		t.Fatal(err)
	}
	b.esperar(t)
	b.s.orgs = orgsAlHub{b}
	b.s.reentrar = func(context.Context) error {
		soltar, err := ajustes.CerrojoSesion(context.Background())
		if err != nil {
			return err
		}
		err = auth.Save(auth.Tokens{Access: "a-r2", Refresh: "r2", Subject: "ana", Organizacion: "o2", Expiry: time.Now().Add(time.Hour)})
		soltar()
		if err != nil {
			return err
		}
		b.s.aplicar(ajustes.Leer()) // lo que hace el vigilante al ver la sesión nueva
		return nil
	}
	if err := b.s.CambiarOrganizacion(t.Context(), "o2"); err != nil {
		t.Fatal(err)
	}
	b.esperar(t)
	b.mu.Lock()
	var got []string
	for _, p := range b.pedidos {
		if strings.Contains(p, "/me/push/apns") || strings.HasPrefix(p, "cambiar ") {
			got = append(got, p)
		}
	}
	b.mu.Unlock()
	quiere := []string{registrar + tokenAvisos + `"}`, olvidar + tokenAvisos + `"}`, "cambiar o2", registrar + tokenAvisos + `"}`}
	if !slices.Equal(got, quiere) {
		t.Fatalf("pedidos = %q\nquiero     %q", got, quiere)
	}
}

// TestElOlvidoCortaElRegistroEnCamino: con el Hub lento, el olvido no consigue el turno
// de la vuelta que está registrando; lo corta y olvida también su token, y la vuelta no
// guarda lo que no llegó a registrar.
//
// VISTO EN ROJO sin cortar la vuelta ni olvidar su token (el olvido volvía sin haber
// olvidado nada, y salir revocaba la sesión antes de que la vuelta pudiera), y sin que
// la vuelta mire olvidos tras el registro (se guardaba y el aparato quedaba en el Hub).
func TestElOlvidoCortaElRegistroEnCamino(t *testing.T) {
	antes := plazoTurnoOlvido
	plazoTurnoOlvido = 100 * time.Millisecond
	t.Cleanup(func() { plazoTurnoOlvido = antes })
	b := nuevoBancoDespertar(t, 1)
	b.s.Arrancar(t.Context())
	b.esperar(t)
	b.registroLento.Store(true)
	if err := b.s.PonerDespertar(true); err != nil {
		t.Fatal(err)
	}
	for limite := time.Now().Add(3 * time.Second); len(b.aparatos()) == 0; {
		if time.Now().After(limite) {
			t.Fatal("la vuelta no ha llegado a registrar")
		}
		time.Sleep(10 * time.Millisecond)
	}
	b.s.olvidarDespertares(t.Context(), false)
	// Al volver del olvido ya está olvidado: salir revoca la sesión justo después.
	quiere := []string{registrar + tokenAvisos + `"}`, olvidar + tokenAvisos + `"}`}
	if got := b.aparatos(); !slices.Equal(got, quiere) {
		t.Fatalf("al volver del olvido, aparatos = %q; quiero el registro y su olvido", got)
	}
	time.Sleep(1200 * time.Millisecond) // lo que tardaría el Hub en contestar sin corte
	b.esperar(t)
	if got := b.aparatos(); !slices.Equal(got, quiere) {
		t.Fatalf("aparatos = %q, quiero solo el registro y su olvido", got)
	}
	if tok, _ := auth.LeerSecreto(prefijoDespertar + "eu.sytue.docus"); tok != nil {
		t.Fatal("se ha guardado un registro que el olvido había cortado")
	}
}
