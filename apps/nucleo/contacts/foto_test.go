// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contacts_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

// # La instantánea guardada (foto.go)
//
// Estas pruebas fallan con una instantánea guardada mal invalidada: que no vea lo que
// escribe otro repositorio sobre el mismo almacén, que tome por buena la candidata de
// una escritura propia que otro cruzó, que una alta no vea la anterior, o que un
// almacén cambiado por fuera deje de dar error.

// abrirCon abre Contacts sobre el almacén de rec con otra configuración: otro Crypto
// sobre el mismo puerto u otro diario transitorio (otro proceso, otro dispositivo).
func abrirCon(t *testing.T, cfg records.Config) *contacts.Repository {
	t.Helper()
	repo, err := contacts.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(repo.Close)
	if err := almacen.Preparar(t.Context(), repo); err != nil {
		t.Fatal("preparar:", err)
	}
	return repo
}

// otroDiario es un diario transitorio del motor aparte, como el de otro proceso.
func otroDiario(t *testing.T) *offline.Store {
	t.Helper()
	dir := t.TempDir()
	diario, err := offline.OpenWithSecretsContext(t.Context(), filepath.Join(dir, "diario"), almacen.ClaveEnFichero{Dir: filepath.Join(dir, "clave")})
	if err != nil {
		t.Fatal(err)
	}
	return diario
}

func almacenDe(t *testing.T, dir string) *almacen.Recursos {
	t.Helper()
	rec, err := almacen.Abrir(context.Background(), dir, "contacts", "instalacion-1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rec.Cerrar)
	return rec
}

func nombres(t *testing.T, repo *contacts.Repository) []string {
	t.Helper()
	todos, _, err := repo.All(t.Context(), 500, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(todos))
	for _, c := range todos {
		out = append(out, c.FormattedName)
	}
	slices.Sort(out)
	return out
}

// Otro repositorio sobre el mismo almacén escribe: el primero lo ve en cuanto su motor
// lo ve (un refresco o una escritura suya), no la libreta que tenía guardada.
func TestLaLibretaGuardadaVeLoQueEscribeOtroRepositorio(t *testing.T) {
	ctx := t.Context()
	rec := almacenDe(t, t.TempDir())
	a := abrirCon(t, rec.Config)
	libro, err := a.CreateAddressBook(ctx, "Personal")
	if err != nil {
		t.Fatal(err)
	}
	if got := nombres(t, a); len(got) != 0 {
		t.Fatal(got)
	}
	otro := rec.Config
	otro.Journal = otroDiario(t)
	b := abrirCon(t, otro)
	xenia, err := b.Create(ctx, libro.ID, api.ContactInput{FormattedName: "Xenia"})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.RefreshPrivateData(ctx); err != nil {
		t.Fatal(err)
	}
	if got := nombres(t, a); !slices.Equal(got, []string{"Xenia"}) {
		t.Fatal("A no ve el alta de B tras refrescar:", got)
	}
	cambio := xenia.Input()
	cambio.FormattedName = "Xenia Bravo"
	if _, err := b.Update(ctx, xenia.ID, cambio, xenia.ETag); err != nil {
		t.Fatal(err)
	}
	// La escritura de A refresca su catálogo antes de preparar: ve el cambio de B.
	if _, err := a.Create(ctx, libro.ID, api.ContactInput{FormattedName: "Yago"}); err != nil {
		t.Fatal(err)
	}
	if got := nombres(t, a); !slices.Equal(got, []string{"Xenia Bravo", "Yago"}) {
		t.Fatal("A no ve el cambio de B tras escribir:", got)
	}
	visto, err := a.Contact(ctx, xenia.ID)
	if err != nil || visto.FormattedName != "Xenia Bravo" {
		t.Fatal(visto, err)
	}
}

// cruce es el puerto del almacén con un gancho que corre justo antes de publicar el
// primer índice: lo que otro escribe mientras este repositorio publica el suyo.
type cruce struct {
	*almacen.Puerto
	mu    sync.Mutex
	antes func()
}

func (c *cruce) PutStorageObjectAt(ctx context.Context, t storage.PublicationTarget, txn, objeto, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	c.mu.Lock()
	antes := c.antes
	c.antes = nil
	c.mu.Unlock()
	if antes != nil {
		antes()
	}
	return c.Puerto.PutStorageObjectAt(ctx, t, txn, objeto, revision, commit)
}

// Si otro escribe mientras A publica su alta, la candidata de A (sin lo del otro) no es
// la libreta de después: A la relee y ve las dos altas.
func TestUnaEscrituraCruzadaNoDejaGuardadaLaCandidata(t *testing.T) {
	ctx := t.Context()
	rec := almacenDe(t, t.TempDir())
	puerto := &cruce{Puerto: rec.Puerto}
	conCruce := rec.Config
	conCruce.Crypto = puerto
	a := abrirCon(t, conCruce)
	otro := rec.Config
	otro.Journal = otroDiario(t)
	b := abrirCon(t, otro)
	libro, err := a.CreateAddressBook(ctx, "Personal")
	if err != nil {
		t.Fatal(err)
	}
	if got := nombres(t, a); len(got) != 0 {
		t.Fatal(got)
	}
	var errB error
	puerto.mu.Lock()
	puerto.antes = func() { _, errB = b.Create(ctx, libro.ID, api.ContactInput{FormattedName: "Bruno"}) }
	puerto.mu.Unlock()
	if _, err := a.Create(ctx, libro.ID, api.ContactInput{FormattedName: "Ana"}); err != nil {
		t.Fatal(err)
	}
	if errB != nil {
		t.Fatal("B no escribió en medio:", errB)
	}
	if got := nombres(t, a); !slices.Equal(got, []string{"Ana", "Bruno"}) {
		t.Fatal("A se quedó con su candidata y no ve lo que B escribió en medio:", got)
	}
}

// Dos altas seguidas: la segunda ve la primera (el mismo UID choca), y la libreta las
// tiene las dos.
func TestDosAltasSeguidasVenLaAnterior(t *testing.T) {
	ctx := t.Context()
	repo, cerrar := abrirLocal(t, t.TempDir())
	defer cerrar()
	libro, err := repo.CreateAddressBook(ctx, "Personal")
	if err != nil {
		t.Fatal(err)
	}
	// Con el UID que trae (la de una cuenta sincronizada): el mismo UID dos veces choca.
	const tarjeta = "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:urn:uuid:5f1c3c2e-8d3b-4f0a-9c55-0b3c1d2e4f60\r\nFN:Carmen Ruiz\r\nEND:VCARD\r\n"
	cruda, err := repo.PonerTarjetaCruda(ctx, libro.ID, "", "", tarjeta)
	if err != nil {
		t.Fatal(err)
	}
	var apiErr *api.Error
	if _, err := repo.PonerTarjetaCruda(ctx, libro.ID, "", "", tarjeta); !errors.As(err, &apiErr) || apiErr.Status != http.StatusConflict {
		t.Fatal("la segunda alta con el mismo UID no vio la primera:", err)
	}
	primera, err := repo.Contact(ctx, cruda.ID)
	if err != nil {
		t.Fatal(err)
	}
	segunda, err := repo.Create(ctx, libro.ID, api.ContactInput{FormattedName: "Diego"})
	if err != nil {
		t.Fatal(err)
	}
	if got := nombres(t, repo); !slices.Equal(got, []string{"Carmen Ruiz", "Diego"}) {
		t.Fatal(got)
	}
	// Lo que devuelve cada alta es lo guardado.
	for _, c := range []api.Contact{primera, segunda} {
		visto, err := repo.Contact(ctx, c.ID)
		if err != nil || visto.ETag != c.ETag || visto.FormattedName != c.FormattedName {
			t.Fatal(visto, err)
		}
	}
	// Destacar y editar: el favorito sigue en la libreta de después de editar.
	if err := repo.SetFavorite(ctx, segunda.UID, true); err != nil {
		t.Fatal(err)
	}
	cambio := segunda.Input()
	cambio.Note = "Llamar el lunes"
	if _, err := repo.Update(ctx, segunda.ID, cambio, segunda.ETag); err != nil {
		t.Fatal(err)
	}
	favoritos, err := repo.Favorites(ctx)
	if err != nil || !slices.Equal(favoritos, []string{segunda.UID}) {
		t.Fatal(favoritos, err)
	}
	visto, err := repo.Contact(ctx, segunda.ID)
	if err != nil || visto.Note != "Llamar el lunes" {
		t.Fatal(visto, err)
	}
}

// Lo que devuelve la libreta guardada es de quien lo pide: cambiarlo no cambia la
// siguiente consulta.
func TestLoQueDevuelveLaLibretaGuardadaEsUnaCopia(t *testing.T) {
	ctx := t.Context()
	repo, cerrar := abrirLocal(t, t.TempDir())
	defer cerrar()
	libro, err := repo.CreateAddressBook(ctx, "Personal")
	if err != nil {
		t.Fatal(err)
	}
	c, err := repo.Create(ctx, libro.ID, api.ContactInput{FormattedName: "Elena", Emails: []api.ContactEmail{{Email: "elena@ejemplo.org"}}})
	if err != nil {
		t.Fatal(err)
	}
	todos, _, err := repo.All(ctx, 10, 0)
	if err != nil || len(todos) != 1 {
		t.Fatal(todos, err)
	}
	todos[0].Emails[0].Email = "cambiado@ejemplo.org"
	todos[0].FormattedName = "Otra"
	foto, err := repo.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	gente, err := foto.Contacts()
	if err != nil {
		t.Fatal(err)
	}
	gente[0].Emails[0].Email = "otro@ejemplo.org"
	visto, err := repo.Contact(ctx, c.ID)
	if err != nil || visto.FormattedName != "Elena" || visto.Emails[0].Email != "elena@ejemplo.org" {
		t.Fatal("la libreta guardada cambió por lo que hizo quien la leyó:", visto, err)
	}
}

// Un almacén cambiado por fuera sigue dando error al abrirlo, como siempre: lo leído se
// valida una vez por fila nueva, no se deja de validar.
func TestUnAlmacenManipuladoSigueDandoError(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	repo, cerrar := abrirLocal(t, dir)
	libro, err := repo.CreateAddressBook(ctx, "Personal")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, libro.ID, api.ContactInput{FormattedName: "Fátima"}); err != nil {
		t.Fatal(err)
	}
	cerrar()
	// La fila del contacto pasa a ser de otro inquilino, con su huella al día: solo la
	// validación de Contacts la rechaza.
	manipular(t, dir, func(fila map[string]any) bool {
		if fila["kind"] != "contact_state" {
			return false
		}
		for _, r := range fila["rows"].([]any) {
			raw := r.(map[string]any)
			var datos map[string]any
			b, _ := base64Decode(raw["json"].(string))
			if json.Unmarshal(b, &datos) != nil {
				t.Fatal("fila ilegible")
			}
			datos["tenant"] = "otro"
			nuevo, _ := json.Marshal(datos)
			raw["json"] = base64Encode(nuevo)
		}
		return true
	})
	rec, err := almacen.Abrir(ctx, dir, "contacts", "instalacion-1")
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Cerrar()
	otro, err := contacts.New(rec.Config)
	if err != nil {
		t.Fatal(err)
	}
	defer otro.Close()
	if err := almacen.Preparar(ctx, otro); !errors.Is(err, contacts.ErrSnapshot) {
		t.Fatal("un almacén manipulado se abrió:", err)
	}
}

// Si el almacén se vuelve a leer del disco (al compactar) y lo leído cambió por fuera,
// la libreta guardada no lo tapa: la siguiente consulta relee y da el error de siempre.
func TestReleerElAlmacenNoTapaUnCambioDeFuera(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	rec := almacenDe(t, dir)
	repo := abrirCon(t, rec.Config)
	libro, err := repo.CreateAddressBook(ctx, "Personal")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, libro.ID, api.ContactInput{FormattedName: "Gonzalo"}); err != nil {
		t.Fatal(err)
	}
	if got := nombres(t, repo); !slices.Equal(got, []string{"Gonzalo"}) {
		t.Fatal(got)
	}
	// Otro cambia el contenido del contacto en el disco, sin tocar su huella, y el
	// almacén lo relee al compactar.
	manipularCon(t, dir, false, func(fila map[string]any) bool {
		if fila["kind"] != "contact_state" {
			return false
		}
		fila["purged"] = false
		return true
	})
	if _, err := rec.Puerto.Compactar(ctx); err != nil {
		t.Fatal("compactar:", err)
	}
	if _, _, err := repo.All(ctx, 10, 0); !errors.Is(err, records.ErrSource) {
		t.Fatal("la libreta guardada tapó un almacén cambiado por fuera:", err)
	}
	// Refrescar tampoco lo da por bueno, como siempre.
	if err := repo.RefreshPrivateData(ctx); !errors.Is(err, records.ErrSource) {
		t.Fatal(err)
	}
}

// manipular reescribe registros.jsonl cambiando con cambia el JSON de cada agregado
// guardado en un registro de contenido (si devuelve true) y poniendo al día la huella
// de su índice.
func manipular(t *testing.T, dir string, cambia func(map[string]any) bool) {
	t.Helper()
	manipularCon(t, dir, true, cambia)
}

// manipularCon es manipular, con la huella del índice al día solo si alDia.
func manipularCon(t *testing.T, dir string, alDia bool, cambia func(map[string]any) bool) {
	t.Helper()
	lineas := leerLineas(t, dir)
	huellas := map[string]string{} // objeto → huella nueva
	for _, l := range lineas {
		reg, ok := l["registro"].(map[string]any)
		if !ok || reg["role"] != "content" {
			continue
		}
		carga, contenido := cargaDe(t, reg)
		var agregado map[string]any
		if json.Unmarshal(contenido, &agregado) != nil || !cambia(agregado) {
			continue
		}
		nuevo, _ := json.Marshal(agregado)
		carga["json"] = base64Encode(nuevo)
		reg["payload"] = base64Encode(mustJSON(t, carga))
		suma := sha256.Sum256(nuevo)
		huellas[reg["object_id"].(string)] = hex.EncodeToString(suma[:])
	}
	if len(huellas) == 0 {
		t.Fatal("nada que manipular")
	}
	for _, l := range lineas {
		if !alDia {
			break
		}
		reg, ok := l["registro"].(map[string]any)
		if !ok || reg["role"] != "index" || huellas[reg["object_id"].(string)] == "" {
			continue
		}
		var commit map[string]any
		raw, _ := base64Decode(reg["payload"].(string))
		if json.Unmarshal(raw, &commit) != nil {
			t.Fatal("índice ilegible")
		}
		var meta map[string]any
		mraw, _ := base64Decode(commit["metadata"].(string))
		if json.Unmarshal(mraw, &meta) != nil {
			t.Fatal("metadatos ilegibles")
		}
		meta["sha256"] = huellas[reg["object_id"].(string)]
		commit["metadata"] = base64Encode(mustJSON(t, meta))
		reg["payload"] = base64Encode(mustJSON(t, commit))
	}
	escribirLineas(t, dir, lineas)
}

func base64Decode(s string) ([]byte, error) { return base64.StdEncoding.DecodeString(s) }
func base64Encode(b []byte) string          { return base64.StdEncoding.EncodeToString(b) }

func cargaDe(t *testing.T, reg map[string]any) (map[string]any, []byte) {
	t.Helper()
	raw, err := base64Decode(reg["payload"].(string))
	if err != nil {
		t.Fatal(err)
	}
	var carga map[string]any
	if json.Unmarshal(raw, &carga) != nil {
		t.Fatal("contenido ilegible")
	}
	codificado, ok := carga["json"].(string)
	if !ok {
		return carga, nil // el recibo de la importación, sin JSON de dominio
	}
	contenido, err := base64Decode(codificado)
	if err != nil {
		t.Fatal(err)
	}
	return carga, contenido
}

func leerLineas(t *testing.T, dir string) []map[string]any {
	t.Helper()
	f, err := os.Open(filepath.Join(dir, "registros.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []map[string]any
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 1<<20), 64<<20)
	for s.Scan() {
		var l map[string]any
		if json.Unmarshal(s.Bytes(), &l) != nil {
			t.Fatal("línea ilegible")
		}
		out = append(out, l)
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func escribirLineas(t *testing.T, dir string, lineas []map[string]any) {
	t.Helper()
	var b bytes.Buffer
	for _, l := range lineas {
		b.Write(mustJSON(t, l))
		b.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(dir, "registros.jsonl"), b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
