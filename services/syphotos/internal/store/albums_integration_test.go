//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/syphotos/db"
	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
	"git.sytue.eu/sytue/one/services/syphotos/internal/store"
)

func TestAlbum_crearAnyadirOrdenarYQuitar(t *testing.T) {
	st, ctx := nuevoStore(t)
	a1, _ := st.Ingerir(ctx, foto("a.jpg", "d1", nil))
	a2, _ := st.Ingerir(ctx, foto("b.jpg", "d2", nil))
	a3, _ := st.Ingerir(ctx, foto("c.jpg", "d3", nil))

	al, err := st.CrearAlbum(ctx, "u1", "  Verano  ")
	if err != nil {
		t.Fatal(err)
	}
	if al.Titulo != "Verano" || al.Fotos != 0 || al.PortadaID != nil {
		t.Fatalf("álbum recién creado: %+v", al)
	}

	// Añadir en orden b, a; repetir b no duplica; una foto borrada no entra.
	_ = st.ALaPapelera(ctx, a3.ID, "u1")
	n, err := st.AnyadirAlAlbum(ctx, al.ID, "u1", []uuid.UUID{a2.ID, a1.ID, a2.ID, a3.ID})
	if err != nil || n != 2 {
		t.Fatalf("añadir: n=%d err=%v (quería 2)", n, err)
	}
	fotos, err := st.FotosDelAlbum(ctx, al.ID, "u1")
	if err != nil || len(fotos) != 2 || fotos[0].ID != a2.ID || fotos[1].ID != a1.ID {
		t.Fatalf("orden del álbum: %v %v", ids(fotos), err)
	}

	// Sin portada explícita, la primera foto; con ella, la elegida.
	al, _ = st.Album(ctx, al.ID, "u1")
	if al.Fotos != 2 || al.PortadaID == nil || *al.PortadaID != a2.ID {
		t.Fatalf("portada implícita: %+v", al)
	}
	al, err = st.RenombrarAlbum(ctx, al.ID, "u1", "Verano 2026", &a1.ID)
	if err != nil || al.Titulo != "Verano 2026" || *al.PortadaID != a1.ID {
		t.Fatalf("renombrar con portada: %+v %v", al, err)
	}
	if _, err := st.RenombrarAlbum(ctx, al.ID, "u1", "x", &a3.ID); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("una portada que no está en el álbum debía rechazarse: %v", err)
	}

	// Quitar y borrar.
	if err := st.QuitarDelAlbum(ctx, al.ID, "u1", a2.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.QuitarDelAlbum(ctx, al.ID, "u1", a2.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("quitar dos veces: %v", err)
	}
	if err := st.BorrarAlbum(ctx, al.ID, "u1"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Album(ctx, al.ID, "u1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("tras borrar: %v", err)
	}
	if _, err := st.Activo(ctx, a1.ID, "u1"); err != nil {
		t.Fatalf("borrar el álbum no toca las fotos: %v", err)
	}
}

func TestAlbum_esDeSuDuenyoYDeSuTenant(t *testing.T) {
	st, ctx := nuevoStore(t)
	al, _ := st.CrearAlbum(ctx, "u1", "Mío")
	if _, err := st.Album(ctx, al.ID, "u2"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("otra persona no lo ve: %v", err)
	}
	if _, err := st.AnyadirAlAlbum(ctx, al.ID, "u2", []uuid.UUID{uuid.New()}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("otra persona no añade: %v", err)
	}
	if lista, _ := st.Albumes(ctxDe("t2"), "u1"); len(lista) != 0 {
		t.Fatalf("otro tenant no lo lista: %v", lista)
	}
	if _, err := st.CrearAlbum(ctx, "u1", "   "); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("título vacío: %v", err)
	}
}

func ids(fotos []domain.Activo) []uuid.UUID {
	out := make([]uuid.UUID, len(fotos))
	for i, f := range fotos {
		out[i] = f.ID
	}
	return out
}

func TestCompartirAlbum_elTokenEsLaCredencial(t *testing.T) {
	st, ctx := nuevoStore(t)
	a1, _ := st.Ingerir(ctx, foto("a.jpg", "d-a", nil))
	a2, _ := st.Ingerir(ctx, foto("b.jpg", "d-b", nil))
	fuera, _ := st.Ingerir(ctx, foto("fuera.jpg", "d-fuera", nil))
	al, err := st.CrearAlbum(ctx, "u1", "Verano")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AnyadirAlAlbum(ctx, al.ID, "u1", []uuid.UUID{a1.ID, a2.ID}); err != nil {
		t.Fatal(err)
	}

	// Sin compartir, no hay token y ningún enlace resuelve.
	if al.Compartido {
		t.Fatal("un álbum recién creado no está compartido")
	}
	if _, err := st.AlbumPorToken(context.Background(), "loquesea"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("un token inventado no resuelve: %v", err)
	}

	// De otra persona, tampoco se comparte.
	if _, err := st.CompartirAlbum(ctx, al.ID, "u2"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("otra persona no comparte: %v", err)
	}

	token, err := st.CompartirAlbum(ctx, al.ID, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(token) < 32 {
		t.Fatalf("el token es la credencial: %q es corto", token)
	}
	// FILE-10 (decisión del dueño): compartir otra vez da un enlace NUEVO y el
	// anterior deja de valer.
	viejo := token
	if token, err = st.CompartirAlbum(ctx, al.ID, "u1"); err != nil || token == viejo {
		t.Fatalf("compartir otra vez da un enlace nuevo: %q (%v)", token, err)
	}
	if _, err := st.AlbumPorToken(context.Background(), viejo); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("el enlace anterior ya no resuelve: %v", err)
	}

	// Y se resuelve SIN organización en el contexto, que es como llega quien
	// abre el enlace.
	pub, err := st.AlbumPorToken(context.Background(), token)
	if err != nil {
		t.Fatalf("el enlace no resuelve: %v", err)
	}
	if pub.Titulo != "Verano" {
		t.Fatalf("álbum del enlace: %+v", pub)
	}
	if pub.Duenyo != "" {
		t.Fatalf("el dueño no se publica: %q", pub.Duenyo)
	}

	fotos, err := st.FotosDelAlbumPublico(context.Background(), pub)
	if err != nil || len(fotos) != 2 {
		t.Fatalf("fotos del enlace: %d %v", len(fotos), err)
	}
	if fotos[0].Activo.ID != a1.ID || fotos[1].Activo.ID != a2.ID {
		t.Fatal("las fotos salen en el orden del álbum")
	}

	// Una foto que NO está en el álbum no se sirve por su enlace: si no, un
	// enlace válido sería licencia para pedir cualquier identificador.
	if _, err := st.FotoDelAlbumPublico(context.Background(), pub, fuera.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("lo que no está en el álbum no se sirve: %v", err)
	}
	if _, err := st.FotoDelAlbumPublico(context.Background(), pub, a1.ID); err != nil {
		t.Fatalf("lo que sí está: %v", err)
	}

	// Lo que se tira deja de verse por el enlace, sin tocar el enlace.
	if err := st.ALaPapelera(ctx, a2.ID, "u1"); err != nil {
		t.Fatal(err)
	}
	if fotos, _ := st.FotosDelAlbumPublico(context.Background(), pub); len(fotos) != 1 {
		t.Fatalf("una foto en la papelera no sale por el enlace: %d", len(fotos))
	}

	// Dejar de compartir apaga el enlace de verdad.
	if err := st.DejarDeCompartirAlbum(ctx, al.ID, "u2"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("otra persona no lo apaga: %v", err)
	}
	if err := st.DejarDeCompartirAlbum(ctx, al.ID, "u1"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AlbumPorToken(context.Background(), token); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("el enlace apagado no resuelve: %v", err)
	}
}

// La receta de la dueña viaja con la foto pública: es lo que nombra la derivada
// que hay que servir. Sin ella se serviría el original de una foto editada.
func TestAlbumPublico_traeLaHuellaDeLaReceta(t *testing.T) {
	st, ctx := nuevoStore(t)
	a, _ := st.Ingerir(ctx, foto("editada.jpg", "d-ed", nil))
	al, _ := st.CrearAlbum(ctx, "u1", "Con edición")
	if _, err := st.AnyadirAlAlbum(ctx, al.ID, "u1", []uuid.UUID{a.ID}); err != nil {
		t.Fatal(err)
	}
	token, err := st.CompartirAlbum(ctx, al.ID, "u1")
	if err != nil {
		t.Fatal(err)
	}
	pub, _ := st.AlbumPorToken(context.Background(), token)

	// Sin editar, la huella va vacía y quien sirva usará la del original.
	fotos, _ := st.FotosDelAlbumPublico(context.Background(), pub)
	if len(fotos) != 1 || fotos[0].Huella != "" {
		t.Fatalf("sin editar la huella va vacía: %+v", fotos)
	}

	huella, err := st.GuardarReceta(ctx, a.ID, "u1", domain.Receta{
		Version: domain.VersionReceta,
		Pasos:   []domain.Paso{{Tipo: domain.PasoGiro, Params: map[string]float64{"grados": 90}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	fotos, _ = st.FotosDelAlbumPublico(context.Background(), pub)
	if len(fotos) != 1 || fotos[0].Huella != huella {
		t.Fatalf("la huella de la dueña viaja con la foto: %+v (quería %s)", fotos, huella)
	}
	una, err := st.FotoDelAlbumPublico(context.Background(), pub, a.ID)
	if err != nil || una.Huella != huella {
		t.Fatalf("y también al pedir una sola: %+v %v", una, err)
	}
}

// Las posiciones del álbum son CONTIGUAS, y eso hay que vigilarlo desde que
// añadir una selección es una sola sentencia: al numerar de una vez con
// `row_number()`, cualquier fila que se descarte después —la que ya estaba, la
// borrada, la repetida— gastaría un número y dejaría un hueco.
//
// Un hueco no se ve en la pantalla, porque el orden sigue saliendo bien. Se ve
// mucho más tarde, cuando alguien cuente con que la posición es un índice.
func TestAlbum_lasPosicionesNoDejanHuecos(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := ctxDe("t1")

	a1, _ := st.Ingerir(ctx, foto("1.jpg", "d1", nil))
	a2, _ := st.Ingerir(ctx, foto("2.jpg", "d2", nil))
	a3, _ := st.Ingerir(ctx, foto("3.jpg", "d3", nil))
	a4, _ := st.Ingerir(ctx, foto("4.jpg", "d4", nil))
	borrada, _ := st.Ingerir(ctx, foto("5.jpg", "d5", nil))
	_ = st.ALaPapelera(ctx, borrada.ID, "u1")

	al, err := st.CrearAlbum(ctx, "u1", "Huecos")
	if err != nil {
		t.Fatal(err)
	}
	if n, err := st.AnyadirAlAlbum(ctx, al.ID, "u1", []uuid.UUID{a1.ID, a2.ID}); err != nil || n != 2 {
		t.Fatalf("primer añadido: n=%d err=%v", n, err)
	}
	// Segunda tanda con de todo: una que ya estaba, una repetida dentro de la
	// propia selección, una borrada y dos nuevas.
	n, err := st.AnyadirAlAlbum(ctx, al.ID, "u1",
		[]uuid.UUID{a1.ID, a3.ID, a3.ID, borrada.ID, a4.ID})
	if err != nil || n != 2 {
		t.Fatalf("segundo añadido: n=%d err=%v (quería 2: a3 y a4)", n, err)
	}

	var posiciones []int
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		filas, err := tx.Query(ctx,
			`SELECT position FROM album_assets WHERE album_id = $1 ORDER BY position`, al.ID)
		if err != nil {
			return err
		}
		defer filas.Close()
		for filas.Next() {
			var p int
			if err := filas.Scan(&p); err != nil {
				return err
			}
			posiciones = append(posiciones, p)
		}
		return filas.Err()
	}); err != nil {
		t.Fatalf("leer posiciones: %v", err)
	}
	for i, p := range posiciones {
		if p != i {
			t.Fatalf("posiciones con hueco: %v (la %d debería ser %d)", posiciones, i, i)
		}
	}
	if len(posiciones) != 4 {
		t.Fatalf("esperaba 4 fotos en el álbum, hay %d: %v", len(posiciones), posiciones)
	}
	// Y el orden es el de llegada, con las nuevas al final.
	fotos, _ := st.FotosDelAlbum(ctx, al.ID, "u1")
	quiero := []uuid.UUID{a1.ID, a2.ID, a3.ID, a4.ID}
	for i, f := range fotos {
		if f.ID != quiero[i] {
			t.Fatalf("orden del álbum: %v", ids(fotos))
		}
	}
}

// TestElEnlaceSeGuardaComoHuella (FILE-10): el token no está en la base, y un
// enlace anterior a la migración 0012 —guardado entonces en claro y pasado a
// huella con su SQL— sigue resolviendo.
func TestElEnlaceSeGuardaComoHuella(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st, ctx := store.New(h.Pool), ctxDe("t1")
	al, err := st.CrearAlbum(ctx, "u1", "Verano")
	if err != nil {
		t.Fatal(err)
	}
	token, err := st.CompartirAlbum(ctx, al.ID, "u1")
	if err != nil {
		t.Fatal(err)
	}
	var enClaro, huellas int
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE public_token = $1 OR public_token_hash = $1), count(*) FILTER (WHERE public_token_hash <> '') FROM albums`, token).Scan(&enClaro, &huellas)
	}); err != nil || enClaro != 0 || huellas != 1 {
		t.Fatalf("el token está en la base (%d) o falta la huella (%d): %v", enClaro, huellas, err)
	}

	// Un enlace de antes: la huella con la MISMA expresión que la migración.
	const deAntes = "enlace-mandado-antes-de-la-0012"
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE albums SET public_token_hash = encode(sha256(convert_to($1, 'UTF8')), 'hex') WHERE tenant = 't1' AND id = $2`, deAntes, al.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if pub, err := st.AlbumPorToken(context.Background(), deAntes); err != nil || pub.ID != al.ID {
		t.Fatalf("el enlace migrado sigue valiendo: %v", err)
	}
	if lista, err := st.Albumes(ctx, "u1"); err != nil || len(lista) != 1 || !lista[0].Compartido {
		t.Fatalf("la dueña ve que está compartido: %+v %v", lista, err)
	}
}
