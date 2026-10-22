// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración del store de Sytue Photos contra un PostgreSQL real.
//
// POR QUÉ AQUÍ Y NO EN UNIDAD: lo que falla en un store es el SQL, y un almacén
// de mentira no tiene SQL. En Sytue Notes, crear y actualizar un cuaderno
// fallaban SIEMPRE contra una base real por un alias de más en un `RETURNING`, y
// los tests de unidad pasaban. Este fichero ejercita las consultas de verdad.
package store_test

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/syphotos/db"
	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
	"git.sytue.eu/sytue/one/services/syphotos/internal/store"
)

func nuevoStore(t *testing.T) (*store.Store, context.Context) {
	t.Helper()
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	return store.New(h.Pool), ctxDe("t1")
}

func foto(nombre, driveID string, tomada *time.Time) domain.Activo {
	return domain.Activo{
		Duenyo: "u1", DriveID: driveID, Nombre: nombre, Tipo: "image/jpeg",
		Bytes: 2048, Ancho: 4000, Alto: 3000, Orientacion: 1, TomadaEn: tomada,
	}
}

func en(anyo int, mes time.Month, dia int) *time.Time {
	t := time.Date(anyo, mes, dia, 12, 0, 0, 0, time.UTC)
	return &t
}

// El `INSERT … RETURNING` de verdad: es donde vive el fallo del alias.
func TestIngerir_devuelveElActivoCreado(t *testing.T) {
	st, ctx := nuevoStore(t)

	a, err := st.Ingerir(ctx, foto("playa.jpg", "drive-1", en(2026, 7, 1)))
	if err != nil {
		t.Fatalf("ingerir: %v", err)
	}
	if a.ID == uuid.Nil {
		t.Error("el activo volvió sin id")
	}
	if a.Nombre != "playa.jpg" || a.Ancho != 4000 || a.Alto != 3000 {
		t.Errorf("volvió %+v", a)
	}
	if a.Tenant != "t1" {
		t.Errorf("tenant %q, se esperaba t1", a.Tenant)
	}
	if a.SubidaEn.IsZero() {
		t.Error("uploaded_at no llegó de vuelta")
	}
}

// REINGERIR NO DUPLICA. Se reindexa una carpeta, o dos clientes suben a la vez:
// si eso crea una segunda fila, la línea de tiempo sale doble y la persona no
// tiene forma de saber cuál borrar.
func TestIngerir_esIdempotentePorFicheroDeDrive(t *testing.T) {
	st, ctx := nuevoStore(t)

	uno, err := st.Ingerir(ctx, foto("playa.jpg", "drive-1", en(2026, 7, 1)))
	if err != nil {
		t.Fatal(err)
	}
	otro, err := st.Ingerir(ctx, foto("playa.jpg", "drive-1", en(2026, 7, 1)))
	if err != nil {
		t.Fatalf("la segunda ingesta falló en vez de devolver la que había: %v", err)
	}
	if uno.ID != otro.ID {
		t.Errorf("dos ids para el mismo fichero: %s y %s", uno.ID, otro.ID)
	}
	linea, _, err := st.LineaDeTiempo(ctx, store.FiltroLinea{Duenyo: "u1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(linea) != 1 {
		t.Errorf("la galería tiene %d fotos tras reingerir la misma", len(linea))
	}
}

// La fecha de captura solo MEJORA: si la primera pasada no supo leer el EXIF y
// la segunda sí, se gana; y al revés no se pierde.
func TestIngerir_laFechaDeCapturaNoSePierde(t *testing.T) {
	st, ctx := nuevoStore(t)

	if _, err := st.Ingerir(ctx, foto("x.jpg", "drive-1", nil)); err != nil {
		t.Fatal(err)
	}
	conFecha, err := st.Ingerir(ctx, foto("x.jpg", "drive-1", en(2019, 5, 4)))
	if err != nil {
		t.Fatal(err)
	}
	if conFecha.TomadaEn == nil {
		t.Fatal("la segunda pasada traía fecha y no se guardó")
	}
	sinFecha, err := st.Ingerir(ctx, foto("x.jpg", "drive-1", nil))
	if err != nil {
		t.Fatal(err)
	}
	if sinFecha.TomadaEn == nil {
		t.Error("una pasada sin EXIF borró la fecha que ya se había leído")
	}
}

// Sin dimensiones no hay dónde colocar el hueco en la cuadrícula.
func TestIngerir_exigeDimensiones(t *testing.T) {
	st, ctx := nuevoStore(t)
	mala := foto("x.jpg", "drive-1", nil)
	mala.Ancho = 0
	if _, err := st.Ingerir(ctx, mala); err == nil {
		t.Error("se ingirió un activo sin ancho")
	}
}

// LA PAGINACIÓN ES POR CURSOR y no por OFFSET: con OFFSET, subir una foto
// mientras alguien se desplaza le repite o le salta una fila.
func TestLineaDeTiempo_paginaSinRepetirNiSaltar(t *testing.T) {
	st, ctx := nuevoStore(t)
	for i := 1; i <= 5; i++ {
		if _, err := st.Ingerir(ctx, foto("f.jpg", string(rune('a'+i)), en(2026, time.Month(i), 1))); err != nil {
			t.Fatal(err)
		}
	}

	pag1, cur, err := st.LineaDeTiempo(ctx, store.FiltroLinea{Duenyo: "u1", Limite: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(pag1) != 2 || cur == nil {
		t.Fatalf("primera página: %d fotos, cursor %v", len(pag1), cur)
	}
	// Más reciente primero: mayo antes que abril.
	if !pag1[0].TomadaEn.After(*pag1[1].TomadaEn) {
		t.Error("la línea de tiempo no va de más reciente a más antigua")
	}

	pag2, _, err := st.LineaDeTiempo(ctx, store.FiltroLinea{Duenyo: "u1", Limite: 2, Desde: cur})
	if err != nil {
		t.Fatal(err)
	}
	vistos := map[uuid.UUID]bool{}
	for _, a := range append(append([]domain.Activo{}, pag1...), pag2...) {
		if vistos[a.ID] {
			t.Errorf("la foto %s salió en las dos páginas", a.ID)
		}
		vistos[a.ID] = true
	}
	if len(vistos) != 4 {
		t.Errorf("%d fotos distintas en dos páginas de 2", len(vistos))
	}
}

// Una foto en la papelera no aparece, y quitarla NO toca Drive: eso es otra
// operación y tiene que pedirse aparte.
func TestALaPapelera_desapareceDeLaLinea(t *testing.T) {
	st, ctx := nuevoStore(t)
	a, err := st.Ingerir(ctx, foto("x.jpg", "drive-1", en(2026, 7, 1)))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ALaPapelera(ctx, a.ID, "u1"); err != nil {
		t.Fatal(err)
	}
	linea, _, err := st.LineaDeTiempo(ctx, store.FiltroLinea{Duenyo: "u1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(linea) != 0 {
		t.Errorf("la foto en la papelera sigue en la línea de tiempo")
	}
	if _, err := st.Activo(ctx, a.ID, "u1"); err == nil {
		t.Error("una foto en la papelera se devuelve por id")
	}
	// Y borrar dos veces no es un éxito silencioso.
	if err := st.ALaPapelera(ctx, a.ID, "u1"); err == nil {
		t.Error("borrar dos veces la misma foto no dio ErrNotFound")
	}
}

// UNA RECETA POR FOTO Y PERSONA: guardar otra sustituye, no acumula. Y la
// huella que devuelve es la que nombra las derivadas.
func TestGuardarReceta_sustituyeYDevuelveLaHuella(t *testing.T) {
	st, ctx := nuevoStore(t)
	a, err := st.Ingerir(ctx, foto("x.jpg", "drive-1", nil))
	if err != nil {
		t.Fatal(err)
	}

	r1 := domain.Receta{Version: domain.VersionReceta, Pasos: []domain.Paso{
		{Tipo: domain.PasoExposicion, Params: map[string]float64{"valor": 0.5}},
	}}
	h1, err := st.GuardarReceta(ctx, a.ID, "u1", r1)
	if err != nil {
		t.Fatal(err)
	}
	esperada, _ := r1.Huella()
	if h1 != esperada {
		t.Errorf("huella %q, se esperaba %q", h1, esperada)
	}

	r2 := domain.Receta{Version: domain.VersionReceta, Pasos: []domain.Paso{
		{Tipo: domain.PasoContraste, Params: map[string]float64{"valor": -0.2}},
	}}
	if _, err := st.GuardarReceta(ctx, a.ID, "u1", r2); err != nil {
		t.Fatal(err)
	}
	vuelta, err := st.Receta(ctx, a.ID, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(vuelta.Pasos) != 1 || vuelta.Pasos[0].Tipo != domain.PasoContraste {
		t.Errorf("la segunda receta no sustituyó a la primera: %+v", vuelta)
	}
}

// Una foto sin editar es el estado NORMAL, no una excepción: devolver la receta
// vacía evita que quien llama tenga que distinguir dos nadas.
func TestReceta_sinEditarDevuelveLaVacia(t *testing.T) {
	st, ctx := nuevoStore(t)
	a, err := st.Ingerir(ctx, foto("x.jpg", "drive-1", nil))
	if err != nil {
		t.Fatal(err)
	}
	r, err := st.Receta(ctx, a.ID, "u1")
	if err != nil {
		t.Fatalf("una foto sin editar dio error: %v", err)
	}
	if err := r.Valida(); err != nil || len(r.Pasos) != 0 {
		t.Errorf("se esperaba la receta vacía, salió %+v (%v)", r, err)
	}
}

// Una receta inválida no llega a la base de datos: se guardaría y se
// reproduciría en cada render.
func TestGuardarReceta_rechazaLaInvalida(t *testing.T) {
	st, ctx := nuevoStore(t)
	a, err := st.Ingerir(ctx, foto("x.jpg", "drive-1", nil))
	if err != nil {
		t.Fatal(err)
	}
	mala := domain.Receta{Version: domain.VersionReceta, Pasos: []domain.Paso{
		{Tipo: domain.PasoExposicion, Params: map[string]float64{"valor": 99}},
	}}
	if _, err := st.GuardarReceta(ctx, a.ID, "u1", mala); err == nil {
		t.Error("se guardó una receta con la exposición fuera de rango")
	}
}

func TestLineaDeTiempo_filtraPorNombreSinComodines(t *testing.T) {
	st, ctx := nuevoStore(t)
	for _, n := range []string{"Playa 2026.jpg", "playa_noche.jpg", "100%.jpg", "montaña.jpg"} {
		if _, err := st.Ingerir(ctx, foto(n, "d-"+n, nil)); err != nil {
			t.Fatal(err)
		}
	}
	nombres := func(texto string) []string {
		activos, _, err := st.LineaDeTiempo(ctx, store.FiltroLinea{Duenyo: "u1", Texto: texto})
		if err != nil {
			t.Fatal(err)
		}
		out := make([]string, len(activos))
		for i, a := range activos {
			out[i] = a.Nombre
		}
		return out
	}
	if got := nombres("PLAYA"); len(got) != 2 {
		t.Fatalf("sin distinguir mayúsculas: %v", got)
	}
	if got := nombres("100%"); len(got) != 1 || got[0] != "100%.jpg" {
		t.Fatalf("el %% del texto es literal, no comodín: %v", got)
	}
	if got := nombres("playa_"); len(got) != 1 || got[0] != "playa_noche.jpg" {
		t.Fatalf("el _ del texto es literal: %v", got)
	}
	if got := nombres("   "); len(got) != 4 {
		t.Fatalf("texto en blanco es «todas»: %v", got)
	}
}

func TestPapelera_restaurarDevuelveLaFotoASuSitio(t *testing.T) {
	st, ctx := nuevoStore(t)
	a, _ := st.Ingerir(ctx, foto("x.jpg", "dx", nil))
	al, _ := st.CrearAlbum(ctx, "u1", "Con x")
	_, _ = st.AnyadirAlAlbum(ctx, al.ID, "u1", []uuid.UUID{a.ID})
	if err := st.ALaPapelera(ctx, a.ID, "u1"); err != nil {
		t.Fatal(err)
	}
	if lista, _ := st.Papelera(ctx, "u1"); len(lista) != 1 || lista[0].ID != a.ID {
		t.Fatalf("papelera: %v", lista)
	}
	if fotos, _ := st.FotosDelAlbum(ctx, al.ID, "u1"); len(fotos) != 0 {
		t.Fatalf("en la papelera no se ve en el álbum: %v", ids(fotos))
	}
	if err := st.Restaurar(ctx, a.ID, "u1"); err != nil {
		t.Fatal(err)
	}
	if err := st.Restaurar(ctx, a.ID, "u1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("restaurar dos veces: %v", err)
	}
	if fotos, _ := st.FotosDelAlbum(ctx, al.ID, "u1"); len(fotos) != 1 {
		t.Fatalf("restaurada vuelve al álbum: %v", ids(fotos))
	}
	if lista, _ := st.Papelera(ctx, "u1"); len(lista) != 0 {
		t.Fatalf("papelera tras restaurar: %v", lista)
	}
}

// Dentro de una organización, las fotos de una persona no son de las demás.
//
// La RLS aísla por ORGANIZACIÓN y nada más: dentro de una, lo único que separa
// a dos personas es el `owner_sub` de cada consulta. Tres métodos lo tomaban
// solo por id —ver una foto, tirarla y restaurarla—, así que quien acertara un
// identificador de la organización podía mirar y borrar lo de otra persona.
func TestActivo_esDeSuDuenyo(t *testing.T) {
	st, ctx := nuevoStore(t)
	mia, err := st.Ingerir(ctx, foto("mia.jpg", "d-mia", nil))
	if err != nil {
		t.Fatal(err)
	}

	// Leer.
	if _, err := st.Activo(ctx, mia.ID, "u2"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("otra persona no la ve: %v", err)
	}
	if _, err := st.Activo(ctx, mia.ID, "u1"); err != nil {
		t.Fatalf("su dueño sí la ve: %v", err)
	}

	// Tirar. Y que siga estando: un fallo silencioso aquí es una foto que
	// desaparece de la galería de otra persona sin que ella haya tocado nada.
	if err := st.ALaPapelera(ctx, mia.ID, "u2"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("otra persona no la tira: %v", err)
	}
	if _, err := st.Activo(ctx, mia.ID, "u1"); err != nil {
		t.Fatalf("sigue viva tras el intento ajeno: %v", err)
	}

	// Restaurar lo que otra persona tiró tampoco.
	if err := st.ALaPapelera(ctx, mia.ID, "u1"); err != nil {
		t.Fatal(err)
	}
	if err := st.Restaurar(ctx, mia.ID, "u2"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("otra persona no la restaura: %v", err)
	}
	if lista, _ := st.Papelera(ctx, "u1"); len(lista) != 1 {
		t.Fatalf("sigue en la papelera de su dueño: %v", lista)
	}
	if err := st.Restaurar(ctx, mia.ID, "u1"); err != nil {
		t.Fatalf("su dueño sí la restaura: %v", err)
	}

	// Y otra ORGANIZACIÓN tampoco, con el mismo identificador de persona.
	if _, err := st.Activo(ctxDe("t2"), mia.ID, "u1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("otra organización no la ve: %v", err)
	}
}

func TestFavoritas_filtranLaLineaYSonDeSuDuenya(t *testing.T) {
	st, ctx := nuevoStore(t)
	a, _ := st.Ingerir(ctx, foto("a.jpg", "d-a", en(2026, 3, 1)))
	b, _ := st.Ingerir(ctx, foto("b.jpg", "d-b", en(2026, 2, 1)))
	_, _ = st.Ingerir(ctx, foto("c.jpg", "d-c", en(2026, 1, 1)))

	// Recién ingerida no es favorita de nadie.
	if a.Favorita {
		t.Fatal("una foto recién ingerida no es favorita")
	}
	favoritas := func() []string {
		lista, _, err := st.LineaDeTiempo(ctx, store.FiltroLinea{Duenyo: "u1", SoloFavoritas: true})
		if err != nil {
			t.Fatal(err)
		}
		out := make([]string, len(lista))
		for i, f := range lista {
			out[i] = f.Nombre
		}
		return out
	}
	if got := favoritas(); len(got) != 0 {
		t.Fatalf("sin marcar ninguna: %v", got)
	}

	if err := st.MarcarFavorita(ctx, a.ID, "u1", true); err != nil {
		t.Fatal(err)
	}
	if err := st.MarcarFavorita(ctx, b.ID, "u1", true); err != nil {
		t.Fatal(err)
	}
	// El orden es el de la línea: la más reciente primero, no el de marcado.
	if got := favoritas(); len(got) != 2 || got[0] != "a.jpg" || got[1] != "b.jpg" {
		t.Fatalf("favoritas en orden de la línea: %v", got)
	}
	// Y la línea entera sigue trayéndolas todas.
	if todas, _, _ := st.LineaDeTiempo(ctx, store.FiltroLinea{Duenyo: "u1"}); len(todas) != 3 {
		t.Fatalf("la línea completa sigue con las tres: %d", len(todas))
	}

	// Marcar es idempotente: repetir no alterna.
	if err := st.MarcarFavorita(ctx, a.ID, "u1", true); err != nil {
		t.Fatal(err)
	}
	if got := favoritas(); len(got) != 2 {
		t.Fatalf("repetir no alterna: %v", got)
	}
	if err := st.MarcarFavorita(ctx, a.ID, "u1", false); err != nil {
		t.Fatal(err)
	}
	if got := favoritas(); len(got) != 1 || got[0] != "b.jpg" {
		t.Fatalf("desmarcar: %v", got)
	}

	// De otra persona, no. Y de la papelera tampoco: no habría dónde verlo.
	if err := st.MarcarFavorita(ctx, b.ID, "u2", true); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("otra persona no marca: %v", err)
	}
	if err := st.ALaPapelera(ctx, b.ID, "u1"); err != nil {
		t.Fatal(err)
	}
	if err := st.MarcarFavorita(ctx, b.ID, "u1", true); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("en la papelera no se marca: %v", err)
	}
	if got := favoritas(); len(got) != 0 {
		t.Fatalf("una favorita en la papelera no sale en la vista: %v", got)
	}
}

func TestPurgar_borraDeVerdadYSoloLoDeLaPapelera(t *testing.T) {
	st, ctx := nuevoStore(t)
	viva, _ := st.Ingerir(ctx, foto("viva.jpg", "d-viva", nil))
	tirada, _ := st.Ingerir(ctx, foto("tirada.jpg", "d-tirada", nil))
	editada, _ := st.Ingerir(ctx, foto("editada.jpg", "d-editada", nil))

	// Una con receta: al purgarla hay que saber su huella para poder nombrar
	// las derivadas que se quedan sin dueño.
	receta := domain.Receta{Version: domain.VersionReceta, Pasos: []domain.Paso{
		{Tipo: domain.PasoGiro, Params: map[string]float64{"grados": 90}},
	}}
	huella, err := st.GuardarReceta(ctx, editada.ID, "u1", receta)
	if err != nil {
		t.Fatal(err)
	}

	// Lo que NO está en la papelera no se purga, ni por id ni al vaciar.
	if _, err := st.Purgar(ctx, "u1", &viva.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("una foto viva no se purga: %v", err)
	}
	if n, err := st.Purgar(ctx, "u1", nil); err != nil || len(n) != 0 {
		t.Fatalf("papelera vacía: %d %v", len(n), err)
	}

	// Una sola, con su huella.
	if err := st.ALaPapelera(ctx, editada.ID, "u1"); err != nil {
		t.Fatal(err)
	}
	purgadas, err := st.Purgar(ctx, "u1", &editada.ID)
	if err != nil || len(purgadas) != 1 {
		t.Fatalf("purgar una: %v %v", purgadas, err)
	}
	if purgadas[0].ID != editada.ID || purgadas[0].Huella != huella {
		t.Fatalf("la huella viaja con la purgada: %+v (quería %s)", purgadas[0], huella)
	}
	// Y se fue de verdad: ya no está ni en la papelera.
	if lista, _ := st.Papelera(ctx, "u1"); len(lista) != 0 {
		t.Fatalf("tras purgar sigue en la papelera: %v", lista)
	}

	// Vaciar entera, y sin tocar lo que está vivo.
	if err := st.ALaPapelera(ctx, tirada.ID, "u1"); err != nil {
		t.Fatal(err)
	}
	purgadas, err = st.Purgar(ctx, "u1", nil)
	if err != nil || len(purgadas) != 1 || purgadas[0].ID != tirada.ID {
		t.Fatalf("vaciar: %v %v", purgadas, err)
	}
	if purgadas[0].Huella != "" {
		t.Fatalf("sin editar, la huella va vacía: %q", purgadas[0].Huella)
	}
	if _, err := st.Activo(ctx, viva.ID, "u1"); err != nil {
		t.Fatalf("vaciar la papelera no toca lo vivo: %v", err)
	}
}

func TestPurgar_esDeSuDuenyoYSeLlevaLoQueCuelga(t *testing.T) {
	st, ctx := nuevoStore(t)
	a, _ := st.Ingerir(ctx, foto("a.jpg", "d-a", nil))
	al, _ := st.CrearAlbum(ctx, "u1", "Con a")
	if _, err := st.AnyadirAlAlbum(ctx, al.ID, "u1", []uuid.UUID{a.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.RenombrarAlbum(ctx, al.ID, "u1", "Con a", &a.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.ALaPapelera(ctx, a.ID, "u1"); err != nil {
		t.Fatal(err)
	}

	// De otra persona, no.
	if _, err := st.Purgar(ctx, "u2", &a.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("otra persona no purga: %v", err)
	}
	if lista, _ := st.Papelera(ctx, "u1"); len(lista) != 1 {
		t.Fatalf("sigue en su papelera: %v", lista)
	}

	// Al purgarla se van sus recetas y su pertenencia al álbum, y el álbum se
	// queda sin portada en vez de bloquear el borrado.
	if _, err := st.Purgar(ctx, "u1", &a.ID); err != nil {
		t.Fatal(err)
	}
	vuelto, err := st.Album(ctx, al.ID, "u1")
	if err != nil {
		t.Fatalf("el álbum sobrevive a que se borren sus fotos: %v", err)
	}
	if vuelto.Fotos != 0 || vuelto.PortadaID != nil {
		t.Fatalf("el álbum queda vacío y sin portada: %+v", vuelto)
	}
}

func TestLugar_seGuardaYSoloMejora(t *testing.T) {
	st, ctx := nuevoStore(t)
	lat, lon := 41.403629, 2.174356 // la Sagrada Familia
	con := foto("con-gps.jpg", "d-gps", nil)
	con.Lat, con.Lon = &lat, &lon
	a, err := st.Ingerir(ctx, con)
	if err != nil {
		t.Fatal(err)
	}
	if a.Lat == nil || a.Lon == nil {
		t.Fatal("el lugar no volvió del INSERT")
	}
	if *a.Lat != lat || *a.Lon != lon {
		t.Fatalf("lugar = %v,%v; quería %v,%v", *a.Lat, *a.Lon, lat, lon)
	}

	// Reingerir SIN GPS no lo borra: una segunda pasada que no supo leerlo no
	// puede perder lo que sí se leyó.
	sin := foto("con-gps.jpg", "d-gps", nil)
	vuelto, err := st.Ingerir(ctx, sin)
	if err != nil {
		t.Fatal(err)
	}
	if vuelto.Lat == nil || *vuelto.Lat != lat {
		t.Fatalf("el lugar solo mejora, no se borra: %v", vuelto.Lat)
	}

	// Y una foto sin GPS se queda sin lugar, no en el meridiano cero.
	b, err := st.Ingerir(ctx, foto("sin-gps.jpg", "d-sin", nil))
	if err != nil {
		t.Fatal(err)
	}
	if b.Lat != nil || b.Lon != nil {
		t.Fatalf("sin GPS no hay lugar: %v %v", b.Lat, b.Lon)
	}
	// Y se lee igual desde la línea de tiempo, que es por donde entra a la app.
	lista, _, err := st.LineaDeTiempo(ctx, store.FiltroLinea{Duenyo: "u1"})
	if err != nil || len(lista) != 2 {
		t.Fatalf("línea: %d %v", len(lista), err)
	}
	conLugar := 0
	for _, f := range lista {
		if f.Lat != nil {
			conLugar++
		}
	}
	if conLugar != 1 {
		t.Fatalf("una de las dos tiene lugar, no %d", conLugar)
	}
}

// La auto-purga recorre TODAS las organizaciones —no hay `app.tenant` que fijar
// en un worker— y solo se lleva lo que ya cumplió la retención. Sin la política
// de mantenimiento no vería ni una fila y diría «purgadas 0» para siempre.
func TestPurgeExpiredTrash_soloLoCaducadoYDeTodaLaInstalacion(t *testing.T) {
	st, ctx := nuevoStore(t)
	otra := ctxDe("t2")

	viva, _ := st.Ingerir(ctx, foto("viva.jpg", "d-viva", nil))
	vieja, _ := st.Ingerir(ctx, foto("vieja.jpg", "d-vieja", nil))
	reciente, _ := st.Ingerir(ctx, foto("reciente.jpg", "d-reciente", nil))
	ajena, _ := st.Ingerir(otra, foto("ajena.jpg", "d-ajena", nil))

	huella, err := st.GuardarReceta(ctx, vieja.ID, "u1", domain.Receta{
		Version: domain.VersionReceta,
		Pasos:   []domain.Paso{{Tipo: domain.PasoGiro, Params: map[string]float64{"grados": 90}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ALaPapelera(ctx, vieja.ID, "u1"); err != nil {
		t.Fatal(err)
	}
	if err := st.ALaPapelera(otra, ajena.ID, "u1"); err != nil {
		t.Fatal(err)
	}
	if err := st.ALaPapelera(ctx, reciente.ID, "u1"); err != nil {
		t.Fatal(err)
	}
	// `vieja` y `ajena` caducan; `reciente` acaba de entrar.
	envejecer(t, st, vieja.ID, 40*24*time.Hour)
	envejecer(t, st, ajena.ID, 40*24*time.Hour)

	// Con la retención apagada no se purga nada, que es lo que promete el cero.
	if p, err := st.PurgeExpiredTrash(context.Background(), 0); err != nil || len(p) != 0 {
		t.Fatalf("retención a cero no purga: %d %v", len(p), err)
	}

	purgadas, err := st.PurgeExpiredTrash(context.Background(), 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(purgadas) != 2 {
		t.Fatalf("caducadas de TODAS las organizaciones: %d", len(purgadas))
	}
	porID := map[uuid.UUID]store.Purgada{}
	for _, p := range purgadas {
		porID[p.ID] = p
	}
	// La organización y la huella viajan con cada purgada: sin ellas no se
	// pueden nombrar las derivadas que quedan sin dueño.
	if p, ok := porID[vieja.ID]; !ok || p.Tenant != "t1" || p.Huella != huella {
		t.Fatalf("la purgada trae organización y huella: %+v", p)
	}
	if p, ok := porID[ajena.ID]; !ok || p.Tenant != "t2" {
		t.Fatalf("la de la otra organización también: %+v", p)
	}

	// Lo reciente sigue en su papelera y lo vivo, en la galería.
	if lista, _ := st.Papelera(ctx, "u1"); len(lista) != 1 || lista[0].ID != reciente.ID {
		t.Fatalf("lo reciente no se purga: %v", lista)
	}
	if _, err := st.Activo(ctx, viva.ID, "u1"); err != nil {
		t.Fatalf("lo vivo no se toca: %v", err)
	}
}

// envejecer mueve hacia atrás el `deleted_at` de una foto, que es la única
// forma de probar una retención de treinta días sin esperarlos.
//
// VA EN UNA TRANSACCIÓN DE MANTENIMIENTO, no a pelo contra el pool: con la RLS
// forzada y sin `app.tenant` ni `app.maintenance`, el UPDATE no encuentra
// ninguna fila y no falla —afecta a cero—. El primer intento lo hizo así y el
// test decía «purgadas 0» por una razón que no era la que medía.
func envejecer(t *testing.T, st *store.Store, id uuid.UUID, cuanto time.Duration) {
	t.Helper()
	err := st.DB().InMaintenanceTx(context.Background(), func(tx pgx.Tx) error {
		tag, err := tx.Exec(context.Background(),
			`UPDATE assets SET deleted_at = now() - $2::interval WHERE id = $1`,
			id, cuanto.String())
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			t.Fatalf("envejecer no tocó la foto %s: %d filas", id, tag.RowsAffected())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDescripcion_seGuardaYSeBuscaPorLoQueSaleEnLaFoto(t *testing.T) {
	st, ctx := nuevoStore(t)
	a, _ := st.Ingerir(ctx, foto("IMG_20260714_193045.jpg", "d-a", nil))
	b, _ := st.Ingerir(ctx, foto("playa.jpg", "d-b", nil))

	if a.Descripcion != "" || len(a.Etiquetas) != 0 {
		t.Fatalf("recién ingerida no tiene descripción: %+v", a)
	}
	if err := st.GuardarDescripcion(ctx, a.ID, "u1",
		"Un perro corriendo por la orilla al atardecer",
		[]string{"perro", "playa", "atardecer", "perro", "  ", "PERRO"}); err != nil {
		t.Fatal(err)
	}

	vuelta, err := st.Activo(ctx, a.ID, "u1")
	if err != nil {
		t.Fatal(err)
	}
	// Las repetidas y las vacías se caen; se conserva el orden de llegada.
	if len(vuelta.Etiquetas) != 3 || vuelta.Etiquetas[0] != "perro" {
		t.Fatalf("etiquetas limpias: %v", vuelta.Etiquetas)
	}

	buscar := func(q string) []string {
		lista, _, err := st.LineaDeTiempo(ctx, store.FiltroLinea{Duenyo: "u1", Texto: q})
		if err != nil {
			t.Fatal(err)
		}
		out := make([]string, len(lista))
		for i, f := range lista {
			out[i] = f.Nombre
		}
		return out
	}
	// Por el NOMBRE, como siempre.
	if got := buscar("playa.jpg"); len(got) != 1 || got[0] != "playa.jpg" {
		t.Fatalf("por nombre: %v", got)
	}
	// Por lo que SALE en la foto, que es lo nuevo: «perro» no está en ningún
	// nombre de fichero.
	if got := buscar("perro"); len(got) != 1 || got[0] != "IMG_20260714_193045.jpg" {
		t.Fatalf("por etiqueta: %v", got)
	}
	if got := buscar("orilla"); len(got) != 1 {
		t.Fatalf("por descripción: %v", got)
	}
	// Y «playa» encuentra las dos: la que se llama así y la que lo tiene dentro.
	if got := buscar("playa"); len(got) != 2 {
		t.Fatalf("nombre y contenido a la vez: %v", got)
	}

	// De otra persona, no se describe.
	if err := st.GuardarDescripcion(ctx, b.ID, "u2", "lo que sea", nil); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("otra persona no describe: %v", err)
	}

	// El texto viene de un modelo, o sea de fuera: se recorta en vez de guardarse
	// entero, y quien pulsó «describir» no se queda sin descripción por eso.
	largo := strings.Repeat("a", store.MaxDescripcion+200)
	muchas := make([]string, 40)
	for i := range muchas {
		muchas[i] = "etiqueta" + itoa(i)
	}
	if err := st.GuardarDescripcion(ctx, a.ID, "u1", largo, muchas); err != nil {
		t.Fatal(err)
	}
	vuelta, _ = st.Activo(ctx, a.ID, "u1")
	if len(vuelta.Descripcion) != store.MaxDescripcion {
		t.Fatalf("descripción recortada: %d", len(vuelta.Descripcion))
	}
	if len(vuelta.Etiquetas) != store.MaxEtiquetas {
		t.Fatalf("etiquetas recortadas: %d", len(vuelta.Etiquetas))
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func vectorDe(semilla float32, dim int) []float32 {
	v := make([]float32, dim)
	for i := range v {
		v[i] = semilla + float32(i%7)/100
	}
	return v
}

func TestVectores_pendientesGuardarYParecidas(t *testing.T) {
	st, ctx := nuevoStore(t)
	const modelo = "siglip2-so400m"
	a, _ := st.Ingerir(ctx, foto("a.jpg", "d-a", nil))
	b, _ := st.Ingerir(ctx, foto("b.jpg", "d-b", nil))
	c, _ := st.Ingerir(ctx, foto("c.jpg", "d-c", nil))

	// Todo está pendiente mientras no tenga vector DE ESE MODELO.
	pend, err := st.PendientesDeVector(context.Background(), modelo, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(pend) != 3 {
		t.Fatalf("las tres están pendientes: %d", len(pend))
	}

	// `a` y `b` se parecen; `c` apunta a otro sitio.
	if err := st.GuardarVector(ctx, a.ID, modelo, vectorDe(1.0, 16)); err != nil {
		t.Fatal(err)
	}
	if err := st.GuardarVector(ctx, b.ID, modelo, vectorDe(0.98, 16)); err != nil {
		t.Fatal(err)
	}
	cv := vectorDe(1.0, 16)
	for i := range cv {
		cv[i] = -cv[i]
	}
	if err := st.GuardarVector(ctx, c.ID, modelo, cv); err != nil {
		t.Fatal(err)
	}

	if pend, _ := st.PendientesDeVector(context.Background(), modelo, 100); len(pend) != 0 {
		t.Fatalf("ya no queda nada pendiente: %d", len(pend))
	}
	// Cambiar de modelo lo devuelve TODO a la cola: dos vectores de modelos
	// distintos viven en espacios distintos y compararlos da parecidos inventados.
	if pend, _ := st.PendientesDeVector(context.Background(), "otro-modelo", 100); len(pend) != 3 {
		t.Fatalf("otro modelo lo deja todo pendiente: %d", len(pend))
	}

	// Lo que no es un vector no se guarda.
	if err := st.GuardarVector(ctx, a.ID, modelo, make([]float32, 16)); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("un vector de ceros no apunta a nada: %v", err)
	}
	if err := st.GuardarVector(ctx, a.ID, "", vectorDe(1, 16)); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("sin modelo no se guarda: %v", err)
	}

	// Parecerse: `b` antes que `c`, y la propia foto no sale.
	similares, err := st.Parecidas(ctx, a.ID, "u1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(similares) != 2 {
		t.Fatalf("dos candidatas: %v", ids(similares))
	}
	if similares[0].ID != b.ID {
		t.Fatalf("la más parecida a «a» es «b»: %v", ids(similares))
	}
	for _, s := range similares {
		if s.ID == a.ID {
			t.Fatal("una foto no se parece a sí misma en su propia lista")
		}
	}

	// De otra persona, no.
	if _, err := st.Parecidas(ctx, a.ID, "u2", 10); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("otra persona no consulta: %v", err)
	}
	// Y una foto sin vector devuelve lista vacía, no un error: el lote todavía
	// no la ha mirado.
	d, _ := st.Ingerir(ctx, foto("d.jpg", "d-d", nil))
	if lista, err := st.Parecidas(ctx, d.ID, "u1", 10); err != nil || len(lista) != 0 {
		t.Fatalf("sin vector, lista vacía: %v %v", lista, err)
	}
}

func TestDuplicadas_agrupaLasRafagasYRespetaElTiempo(t *testing.T) {
	st, ctx := nuevoStore(t)
	const modelo = "siglip2-so400m"
	en := func(min int) *time.Time {
		x := time.Date(2026, 7, 1, 12, min, 0, 0, time.UTC)
		return &x
	}
	// Tres disparos del mismo momento (casi el mismo vector) y una foto
	// distinta entre medias.
	a, _ := st.Ingerir(ctx, foto("rafaga-1.jpg", "d1", en(0)))
	b, _ := st.Ingerir(ctx, foto("rafaga-2.jpg", "d2", en(1)))
	c, _ := st.Ingerir(ctx, foto("rafaga-3.jpg", "d3", en(2)))
	otra, _ := st.Ingerir(ctx, foto("otra.jpg", "d4", en(3)))
	// Y una casi idéntica a la primera pero DOS HORAS después: no es la misma
	// ráfaga, es la misma vista en otro momento, y eso no sobra.
	lejos, _ := st.Ingerir(ctx, foto("otro-dia.jpg", "d5", en(180)))

	base := vectorDe(1.0, 32)
	casi := append([]float32(nil), base...)
	casi[0] += 0.002
	casi2 := append([]float32(nil), base...)
	casi2[1] += 0.002
	distinto := vectorDe(1.0, 32)
	for i := range distinto {
		distinto[i] = -distinto[i]
	}
	for id, v := range map[uuid.UUID][]float32{
		a.ID: base, b.ID: casi, c.ID: casi2, otra.ID: distinto, lejos.ID: base,
	} {
		if err := st.GuardarVector(ctx, id, modelo, v); err != nil {
			t.Fatal(err)
		}
	}

	grupos, err := st.Duplicadas(ctx, "u1", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(grupos) != 1 {
		t.Fatalf("un solo grupo, el de la ráfaga: %d", len(grupos))
	}
	if len(grupos[0].Fotos) != 3 {
		t.Fatalf("los tres disparos: %v", ids(grupos[0].Fotos))
	}
	dentro := map[uuid.UUID]bool{}
	for _, f := range grupos[0].Fotos {
		dentro[f.ID] = true
	}
	for _, id := range []uuid.UUID{a.ID, b.ID, c.ID} {
		if !dentro[id] {
			t.Fatalf("falta un disparo de la ráfaga: %v", ids(grupos[0].Fotos))
		}
	}
	if dentro[otra.ID] {
		t.Fatal("una foto distinta no entra en el grupo")
	}
	if dentro[lejos.ID] {
		t.Fatal("la misma vista dos horas después NO es una repetida")
	}

	// Sin vector no hay grupos: el lote todavía no ha mirado nada.
	st2, ctx2 := nuevoStore(t)
	if _, err := st2.Ingerir(ctx2, foto("x.jpg", "dx", en(0))); err != nil {
		t.Fatal(err)
	}
	if g, err := st2.Duplicadas(ctx2, "u1", 50); err != nil || len(g) != 0 {
		t.Fatalf("sin vectores no hay repetidas: %d %v", len(g), err)
	}
}

func TestBusqueda_terminosAlternativos(t *testing.T) {
	st, ctx := nuevoStore(t)
	a, _ := st.Ingerir(ctx, foto("IMG_001.jpg", "d1", nil))
	b, _ := st.Ingerir(ctx, foto("IMG_002.jpg", "d2", nil))
	_, _ = st.Ingerir(ctx, foto("IMG_003.jpg", "d3", nil))
	if err := st.GuardarDescripcion(ctx, a.ID, "u1", "Un perro en la orilla del mar", []string{"perro"}); err != nil {
		t.Fatal(err)
	}
	if err := st.GuardarDescripcion(ctx, b.ID, "u1", "Un gato en un tejado", []string{"gato"}); err != nil {
		t.Fatal(err)
	}

	buscar := func(q string, tambien ...string) []string {
		lista, _, err := st.LineaDeTiempo(ctx, store.FiltroLinea{Duenyo: "u1", Texto: q, Tambien: tambien})
		if err != nil {
			t.Fatal(err)
		}
		out := make([]string, len(lista))
		for i, f := range lista {
			out[i] = f.Nombre
		}
		return out
	}

	// «playa» literal no está en ninguna descripción.
	if got := buscar("playa"); len(got) != 0 {
		t.Fatalf("literal: %v", got)
	}
	// Con la alternativa que propondría el modelo, sí aparece.
	if got := buscar("playa", "orilla"); len(got) != 1 || got[0] != "IMG_001.jpg" {
		t.Fatalf("con alternativa: %v", got)
	}
	// CASAR CON CUALQUIERA, no con todas: si fuera «y», añadir términos
	// estrecharía la búsqueda en vez de ampliarla, que es lo contrario.
	if got := buscar("perro", "gato"); len(got) != 2 {
		t.Fatalf("cualquiera de los dos: %v", got)
	}
	// Las vacías se ignoran y no arrastran a media galería.
	if got := buscar("perro", "", "   "); len(got) != 1 {
		t.Fatalf("las vacías no cuentan: %v", got)
	}
	// Y los comodines del término alternativo siguen siendo literales.
	if got := buscar("nada-de-esto", "%"); len(got) != 0 {
		t.Fatalf("el %% de una alternativa es literal: %v", got)
	}
}

func TestRecuerdos_mismoDiaDeAnyosAnteriores(t *testing.T) {
	st, ctx := nuevoStore(t)
	hoy := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	el := func(anyo int, mes time.Month, dia int) *time.Time {
		x := time.Date(anyo, mes, dia, 10, 0, 0, 0, time.UTC)
		return &x
	}
	dosMil24a, _ := st.Ingerir(ctx, foto("2024-a.jpg", "d1", el(2024, 7, 14)))
	dosMil24b, _ := st.Ingerir(ctx, foto("2024-b.jpg", "d2", el(2024, 7, 14)))
	dosMil20, _ := st.Ingerir(ctx, foto("2020.jpg", "d3", el(2020, 7, 14)))
	_, _ = st.Ingerir(ctx, foto("otro-dia.jpg", "d4", el(2024, 7, 15)))
	_, _ = st.Ingerir(ctx, foto("hoy-mismo.jpg", "d5", el(2026, 7, 14)))
	// Sin fecha de captura: se coloca por la de subida, o sea HOY, y no se hizo
	// ese día de ningún año anterior.
	_, _ = st.Ingerir(ctx, foto("sin-exif.jpg", "d6", nil))

	recuerdos, err := st.Recuerdos(ctx, "u1", hoy)
	if err != nil {
		t.Fatal(err)
	}
	if len(recuerdos) != 2 {
		t.Fatalf("dos años con recuerdo: %d", len(recuerdos))
	}
	if recuerdos[0].Anyo != 2024 || recuerdos[1].Anyo != 2020 {
		t.Fatalf("del más reciente al más antiguo: %d, %d", recuerdos[0].Anyo, recuerdos[1].Anyo)
	}
	if len(recuerdos[0].Fotos) != 2 {
		t.Fatalf("las dos de 2024: %v", ids(recuerdos[0].Fotos))
	}
	dentro := map[uuid.UUID]bool{}
	for _, r := range recuerdos {
		for _, f := range r.Fotos {
			dentro[f.ID] = true
		}
	}
	for _, id := range []uuid.UUID{dosMil24a.ID, dosMil24b.ID, dosMil20.ID} {
		if !dentro[id] {
			t.Fatal("falta una foto del mismo día de un año anterior")
		}
	}
	if len(dentro) != 3 {
		t.Fatalf("ni el día de al lado, ni las de hoy, ni las sin fecha de captura: %d", len(dentro))
	}

	// Un día sin nada devuelve lista vacía, no un error.
	if r, err := st.Recuerdos(ctx, "u1", time.Date(2026, 3, 3, 12, 0, 0, 0, time.UTC)); err != nil || len(r) != 0 {
		t.Fatalf("un día cualquiera: %d %v", len(r), err)
	}
	// Y los recuerdos son de cada cual.
	if r, _ := st.Recuerdos(ctx, "u2", hoy); len(r) != 0 {
		t.Fatalf("otra persona no ve los recuerdos ajenos: %d", len(r))
	}
}

func TestParecidasAVarias_elCentroideYElUmbral(t *testing.T) {
	st, ctx := nuevoStore(t)
	const modelo = "siglip2-so400m"
	// Direcciones a mano: `vectorDe` da siempre la misma dirección al
	// normalizar, y aquí lo que se prueba son justamente los ángulos.
	eje := func(xs ...float32) []float32 {
		v := make([]float32, 8)
		copy(v, xs)
		return v
	}
	guarda := func(nombre string, v []float32) uuid.UUID {
		t.Helper()
		a, err := st.Ingerir(ctx, foto(nombre, "d-"+nombre, nil))
		if err != nil {
			t.Fatal(err)
		}
		if err := st.GuardarVector(ctx, a.ID, modelo, v); err != nil {
			t.Fatal(err)
		}
		return a.ID
	}

	// Dos semillas en ángulo recto: su centroide apunta justo entre las dos.
	s1 := guarda("semilla-1.jpg", eje(1, 0))
	s2 := guarda("semilla-2.jpg", eje(0, 1))
	// Con el centroide en (0,707 · 0,707), el coseno de cada una sale de cuánto
	// pesa la tercera componente: 1,00 · 0,84 · 0,75 · 0,00.
	tema := guarda("del-tema.jpg", eje(1, 1))
	flojo := guarda("mas-flojo.jpg", eje(1, 1, 0.9))
	rozando := guarda("no-llega.jpg", eje(1, 1, 1.24))
	nada := guarda("otra-cosa.jpg", eje(0, 0, 1))

	lista, err := st.ParecidasAVarias(ctx, "u1", []uuid.UUID{s1, s2}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(lista) != 2 {
		t.Fatalf("solo las que pasan de %.2f: %v", store.UmbralDeAmpliacion, ids(lista))
	}
	if lista[0].ID != tema || lista[1].ID != flojo {
		t.Fatalf("de más a menos parecida al conjunto: %v", ids(lista))
	}
	for _, f := range lista {
		if f.ID == s1 || f.ID == s2 {
			t.Fatal("las de partida no se devuelven como resultado")
		}
		if f.ID == rozando || f.ID == nada {
			t.Fatalf("por debajo del umbral no entra: %v", ids(lista))
		}
	}

	// UNA lista siempre llena sería una lista en la que no se puede confiar:
	// partiendo de algo que no se parece a nada, la respuesta es «nada».
	if solo, err := st.ParecidasAVarias(ctx, "u1", []uuid.UUID{nada}, 10); err != nil || len(solo) != 0 {
		t.Fatalf("sin nada parecido, lista vacía: %v %v", ids(solo), err)
	}

	// De otra persona no se parte: sus fotos no existen para quien pregunta, y
	// colarlas en el centroide diría algo de su contenido sin enseñarlas.
	if ajena, err := st.ParecidasAVarias(ctx, "u2", []uuid.UUID{s1, s2}, 10); err != nil || len(ajena) != 0 {
		t.Fatalf("otra persona no ve nada: %v %v", ids(ajena), err)
	}

	// Una semilla que el lote todavía no ha mirado no es un error: es una lista
	// vacía, igual que en «parecidas a esta».
	sinVector, err := st.Ingerir(ctx, foto("sin-vector.jpg", "d-sv", nil))
	if err != nil {
		t.Fatal(err)
	}
	if l, err := st.ParecidasAVarias(ctx, "u1", []uuid.UUID{sinVector.ID}, 10); err != nil || len(l) != 0 {
		t.Fatalf("sin vector, lista vacía: %v %v", ids(l), err)
	}
	// Y sin fotos de partida no hay pregunta que hacer.
	if _, err := st.ParecidasAVarias(ctx, "u1", nil, 10); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("sin semillas, entrada inválida: %v", err)
	}
}

func TestTemas_agrupaPorLoQueSeVeYNoPorLaHora(t *testing.T) {
	st, ctx := nuevoStore(t)
	const modelo = "siglip2-so400m"
	eje := func(xs ...float32) []float32 {
		v := make([]float32, 8)
		copy(v, xs)
		return v
	}
	guarda := func(nombre string, cuando *time.Time, v []float32) uuid.UUID {
		t.Helper()
		a, err := st.Ingerir(ctx, foto(nombre, "d-"+nombre, cuando))
		if err != nil {
			t.Fatal(err)
		}
		if err := st.GuardarVector(ctx, a.ID, modelo, v); err != nil {
			t.Fatal(err)
		}
		return a.ID
	}
	dia := func(d int) *time.Time {
		x := time.Date(2026, 7, d, 12, 0, 0, 0, time.UTC)
		return &x
	}

	// Cinco de un tema, repartidas por el verano entero: lo que las junta es lo
	// que se ve, no la hora. Un par de ellas ligeramente distintas para que el
	// centro no sea el mismo vector cinco veces.
	playa := []uuid.UUID{
		guarda("playa-1.jpg", dia(1), eje(1, 0.05)),
		guarda("playa-2.jpg", dia(9), eje(1, 0)),
		guarda("playa-3.jpg", dia(17), eje(1, 0.1)),
		guarda("playa-4.jpg", dia(25), eje(0.98, 0.05)),
		guarda("playa-5.jpg", dia(30), eje(1, 0.02)),
	}
	// Y cuatro de otro, en ángulo recto con el primero.
	monte := []uuid.UUID{
		guarda("monte-1.jpg", dia(2), eje(0, 1)),
		guarda("monte-2.jpg", dia(10), eje(0.05, 1)),
		guarda("monte-3.jpg", dia(18), eje(0, 0.99)),
		guarda("monte-4.jpg", dia(26), eje(0.02, 1)),
	}
	// Cuatro A MEDIO CAMINO de los dos anteriores: 0,71 de coseno con cada uno.
	// Son las que prueban que el umbral hace algo. Con 0,85 son su propio
	// montón; bajándolo, se funden con uno de los otros dos y los tres temas se
	// convierten en dos, que es exactamente el fallo que se quiere evitar.
	medias := []uuid.UUID{
		guarda("media-1.jpg", dia(5), eje(1, 1)),
		guarda("media-2.jpg", dia(13), eje(0.98, 1)),
		guarda("media-3.jpg", dia(21), eje(1, 0.97)),
		guarda("media-4.jpg", dia(29), eje(1, 1.02)),
	}
	// Tres sueltas que no se parecen a nada: no son un tema, son tres fotos.
	guarda("suelta-1.jpg", dia(3), eje(0, 0, 1))
	guarda("suelta-2.jpg", dia(11), eje(0, 0, 0, 1))
	guarda("suelta-3.jpg", dia(19), eje(0, 0, 0, 0, 1))
	// Y DOS que sí se parecen entre sí y solo entre sí. Dos fotos parecidas son
	// dos fotos parecidas: si esto saliera como tema, la pantalla se llenaría de
	// parejas y escondería los montones que valen.
	guarda("pareja-1.jpg", dia(4), eje(0, 0, 0, 0, 0, 1))
	guarda("pareja-2.jpg", dia(12), eje(0, 0, 0, 0, 0, 0.99))

	temas, err := st.Temas(ctx, "u1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(temas) != 3 {
		t.Fatalf("tres montones: playa, monte y las de en medio: %d", len(temas))
	}
	// El más grande primero: es el que se reconoce de un vistazo.
	if len(temas[0].Fotos) != 5 {
		t.Fatalf("de más fotos a menos, y el grande son cinco: %d", len(temas[0].Fotos))
	}
	// Por conjuntos y no por posición: entre los dos montones de cuatro el orden
	// lo decide la foto más reciente, y eso no es lo que se está probando.
	donde := map[uuid.UUID]int{}
	for i, g := range temas {
		for _, f := range g.Fotos {
			donde[f.ID] = i
		}
	}
	familias := map[string][]uuid.UUID{"playa": playa, "monte": monte, "de en medio": medias}
	suyo := map[string]int{}
	for nombre, familia := range familias {
		suyo[nombre] = donde[familia[0]]
		for _, id := range familia {
			if donde[id] != suyo[nombre] {
				t.Fatalf("las de %s tienen que caer todas en el mismo montón: %v", nombre, donde)
			}
		}
	}
	if suyo["playa"] == suyo["monte"] || suyo["playa"] == suyo["de en medio"] || suyo["monte"] == suyo["de en medio"] {
		t.Fatalf("cada familia en un montón distinto: %v", suyo)
	}

	// El mismo montón dos veces seguidas: una pantalla que propone no puede
	// proponer algo distinto en cada recarga.
	otra, err := st.Temas(ctx, "u1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(otra) != len(temas) || !slices.Equal(ids(otra[0].Fotos), ids(temas[0].Fotos)) {
		t.Fatalf("el resultado tiene que ser el mismo: %v vs %v", ids(otra[0].Fotos), ids(temas[0].Fotos))
	}

	// De otra persona, nada.
	if ajenos, err := st.Temas(ctx, "u2", 10); err != nil || len(ajenos) != 0 {
		t.Fatalf("otra persona no ve montones: %d %v", len(ajenos), err)
	}
	// Y sin vectores tampoco: el lote todavía no ha mirado nada.
	st2, ctx2 := nuevoStore(t)
	if _, err := st2.Ingerir(ctx2, foto("x.jpg", "dx", dia(1))); err != nil {
		t.Fatal(err)
	}
	if g, err := st2.Temas(ctx2, "u1", 10); err != nil || len(g) != 0 {
		t.Fatalf("sin vectores no hay montones: %d %v", len(g), err)
	}
}

func TestCercaDe_elMismoSitioYNoLaMismaCiudad(t *testing.T) {
	st, ctx := nuevoStore(t)
	// La Malvarrosa, Valencia. Los puntos están puestos con la calculadora:
	// un grado de latitud son 111,32 km SIEMPRE, y uno de longitud son esos
	// mismos kilómetros por el coseno de la latitud (aquí, 0,76).
	const lat, lon = 39.4740, -0.3230
	conLugar := func(nombre string, la, lo *float64) uuid.UUID {
		t.Helper()
		a := foto(nombre, "d-"+nombre, nil)
		a.Lat, a.Lon = la, lo
		puesta, err := st.Ingerir(ctx, a)
		if err != nil {
			t.Fatal(err)
		}
		return puesta.ID
	}
	punto := func(la, lo float64) (*float64, *float64) { return &la, &lo }

	la, lo := punto(lat, lon)
	origen := conLugar("origen.jpg", la, lo)
	// A 111 metros al norte: el mismo sitio.
	la1, lo1 := punto(lat+0.001, lon)
	alLado := conLugar("al-lado.jpg", la1, lo1)
	// A ~1,1 km al este: sigue siendo el mismo sitio.
	la2, lo2 := punto(lat, lon+0.013)
	mismaPlaya := conLugar("misma-playa.jpg", la2, lo2)
	// A ~1,7 km al este: dentro, y ADEMÁS es la que prueba el coseno. Son 0,020
	// grados de longitud, más que los 0,018 que mide media caja en latitud: si
	// los dos ejes usaran el mismo delta, esta se quedaría fuera del filtro
	// grueso sin llegar a medirse nunca.
	la5, lo5 := punto(lat, lon+0.020)
	mismoPaseo := conLugar("mismo-paseo.jpg", la5, lo5)
	// A ~5,5 km al sur: la misma ciudad, que NO es la misma pregunta.
	la3, lo3 := punto(lat-0.05, lon)
	otraParte := conLugar("otra-parte.jpg", la3, lo3)
	// En la ESQUINA de la caja: dentro del cuadrado de dos kilómetros de lado y
	// fuera del círculo de dos kilómetros de radio. Es la que prueba que la
	// distancia se calcula de verdad y no se despacha con un BETWEEN.
	la4, lo4 := punto(lat+0.0170, lon+0.0230)
	esquina := conLugar("esquina.jpg", la4, lo4)
	// Y una sin coordenadas, que no está en ninguna parte.
	sinLugar := conLugar("sin-lugar.jpg", nil, nil)

	cerca, err := st.CercaDe(ctx, origen, "u1", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(cerca) != 3 {
		t.Fatalf("solo las de este sitio: %v", ids(cerca))
	}
	// De la más próxima a la más lejana.
	if cerca[0].ID != alLado || cerca[1].ID != mismaPlaya || cerca[2].ID != mismoPaseo {
		t.Fatalf("ordenadas por distancia: %v", ids(cerca))
	}
	for _, f := range cerca {
		if f.ID == origen || f.ID == otraParte || f.ID == esquina || f.ID == sinLugar {
			t.Fatalf("esa no debería estar: %v", ids(cerca))
		}
	}

	// Una foto sin coordenadas no da error: da lista vacía.
	if l, err := st.CercaDe(ctx, sinLugar, "u1", 50); err != nil || len(l) != 0 {
		t.Fatalf("sin coordenadas, lista vacía: %v %v", ids(l), err)
	}
	// Y de otra persona, ni existe.
	if _, err := st.CercaDe(ctx, origen, "u2", 50); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("otra persona no consulta: %v", err)
	}
}
