// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package guardardrive

// «Guardar en Drive» contra un Drive de la bóveda de verdad (drive.Repository sobre
// vaultfiles y pqvault, con el servidor de bloques de pqvaulttest): lo que se
// comprueba es lo que queda allí, cifrado, y lo que llega al servicio de Docs.

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	privateDrive "git.sytue.eu/sytue/one/apps/nucleo/drive"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvault"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvaulttest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/vaultfiles"
)

var scope = storage.Scope{Tenant: "acme", Subject: "persona-1", Product: "drive"}

const raizID = "0b3c6f9e-5d1a-4c2b-9e7f-1a2b3c4d5e60"

// fuenteLegacy es la fuente del Drive del servidor con solo «Mi unidad»: la bóveda la
// importa al abrir (DirectImport, como la web) y queda lista para escribir.
type fuenteLegacy struct {
	mu    sync.Mutex
	state storage.LegacyMigrationState
}

func (f *fuenteLegacy) PrivateStorageState(context.Context, storage.Scope) (storage.LegacyMigrationState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state, nil
}

func (f *fuenteLegacy) BeginPrivateStorageMigration(_ context.Context, s storage.Scope, epoch string) (storage.LegacyMigrationState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.state.Mode == "legacy" {
		now := time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC)
		f.state = storage.LegacyMigrationState{Scope: s, Mode: "frozen", Epoch: epoch, StartedAt: &now, Rows: 1}
	}
	return f.state, nil
}

func (f *fuenteLegacy) PrivateStorageSourcePage(_ context.Context, s storage.Scope, epoch string, _ storage.LegacySourceCursor) (storage.LegacySourcePage, error) {
	stamp := `"created_at":"2026-01-01T10:00:00Z","updated_at":"2026-01-02T10:00:00Z"`
	return storage.LegacySourcePage{Scope: s, Epoch: epoch, Rows: []storage.LegacySourceRow{
		{Kind: "folder", ID: raizID, Payload: []byte(`{"id":"` + raizID + `","parent_id":null,"name":"Mi unidad","is_root":true,` + stamp + `}`)},
	}}, nil
}

func (*fuenteLegacy) OpenPrivateStorageSource(context.Context, storage.Scope, string, string, string, int64) (io.ReadCloser, error) {
	return nil, privateDrive.ErrSource
}

type banco struct {
	fake *pqvaulttest.Fake
	repo *privateDrive.Repository
}

func nuevoBanco(t *testing.T) *banco {
	t.Helper()
	fake, gateway, account := pqvaulttest.New(t, scope)
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	vault, err := pqvault.Open(t.Context(), pqvault.Config{Scope: scope, Server: gateway, Registry: account, UnlockKey: bytes.Repeat([]byte{3}, recoverywrap.UnlockKeyLen)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(vault.Close)
	store, err := vaultfiles.New(vault)
	if err != nil {
		t.Fatal(err)
	}
	fuente := &fuenteLegacy{state: storage.LegacyMigrationState{Scope: scope, Mode: "legacy"}}
	repo, err := privateDrive.New(privateDrive.Config{Context: t.Context(), Scope: scope, Crypto: store, Gateway: gateway, Source: fuente, Journal: journal, DirectImport: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(repo.Close)
	if err := repo.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	return &banco{fake: fake, repo: repo}
}

// carpeta crea `nombre` en la raíz.
func (b *banco) carpeta(t *testing.T, nombre string) string {
	t.Helper()
	c, err := b.repo.CreateFolder(t.Context(), raizID, nombre)
	if err != nil {
		t.Fatal(err)
	}
	return c.ID
}

func (b *banco) subir(t *testing.T, carpeta, nombre, contenido string) api.File {
	t.Helper()
	f, err := b.repo.Upload(t.Context(), carpeta, nombre, "application/octet-stream", strings.NewReader(contenido), int64(len(contenido)))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (b *banco) leer(t *testing.T, id string) string {
	t.Helper()
	cuerpo, err := b.repo.Download(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cuerpo.Close() }()
	datos, err := io.ReadAll(cuerpo)
	if err != nil {
		t.Fatal(err)
	}
	return string(datos)
}

// ficheros son los de la carpeta: nombre → id.
func (b *banco) ficheros(t *testing.T, carpeta string) map[string]string {
	t.Helper()
	hijos, err := b.repo.Children(t.Context(), carpeta, "")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range hijos.Files {
		out[f.Name] = f.ID
	}
	return out
}

func (b *banco) carpetaDocs(t *testing.T) string {
	t.Helper()
	hijos, err := b.repo.Children(t.Context(), raizID, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range hijos.Folders {
		if c.Name == CarpetaDocumentos {
			return c.ID
		}
	}
	return ""
}

// sinClaro: el servidor de bloques nunca ve el documento.
func (b *banco) sinClaro(t *testing.T, texto string) {
	t.Helper()
	for _, blob := range b.fake.Blobs.Data {
		if bytes.Contains(blob, []byte(texto)) {
			t.Fatal("el servidor de bloques ve el documento en claro")
		}
	}
}

// docs es el servicio de Docs: exporta (contando los formatos) y ancla.
type docs struct {
	mu        sync.Mutex
	contenido string
	formatos  []string
	fallar    error
	anclas    [][2]string
	anclar    error
}

func (d *docs) exportar(_ context.Context, formato string) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.formatos = append(d.formatos, formato)
	if d.fallar != nil {
		return nil, d.fallar
	}
	return []byte(d.contenido + " [" + formato + "]"), nil
}

func (d *docs) anclarEn(_ context.Context, nueva, antes string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.anclas = append(d.anclas, [2]string{nueva, antes})
	return d.anclar
}

// servidorEspia es el Drive del servidor: falla la prueba si se le sube algo.
type servidorEspia struct {
	Repositorio
	t      *testing.T
	subido int
}

func (s *servidorEspia) Upload(context.Context, string, string, string, io.Reader, int64) (api.File, error) {
	s.subido++
	s.t.Error("se ha subido al Drive del servidor")
	return api.File{}, errors.New("no")
}

func (s *servidorEspia) ReplaceFile(context.Context, api.File, string, io.Reader, int64) (api.File, error) {
	s.subido++
	s.t.Error("se ha reescrito un fichero del Drive del servidor")
	return api.File{}, errors.New("no")
}

func (s *servidorEspia) File(context.Context, string) (api.File, error) {
	s.t.Error("se ha leído el Drive del servidor con el Drive en la bóveda")
	return api.File{}, errors.New("no")
}

func destinosBoveda(t *testing.T, repo Repositorio, d *docs) Destinos {
	return Destinos{
		DriveEnBoveda: func(context.Context) (bool, error) { return true, nil },
		Servidor:      &servidorEspia{t: t},
		Boveda:        func(context.Context) (Repositorio, error) { return repo, nil },
		Anclar:        d.anclarEn,
	}
}

// Sin ancla y con el Drive en la bóveda: un fichero NUEVO en «Sytue Docs» (que se
// crea), en ODF, cifrado; el dueño ancla el documento a él con «vault:<id>».
func TestNuevoFicheroYAncla(t *testing.T) {
	b := nuevoBanco(t)
	d := &docs{contenido: "acta secreta del consejo"}
	r, err := GuardarDocumento(t.Context(), destinosBoveda(t, b.repo, d), Documento{EsDueno: true, Titulo: "Acta", Kind: api.DocText}, d.exportar)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Nuevo || !r.Cifrado || r.Nombre != "Acta.odt" || !EsAnclaDeBoveda(r.Ancla) || r.AnclaDelDocumento != r.Ancla {
		t.Fatalf("resultado = %+v", r)
	}
	carpeta := b.carpetaDocs(t)
	if carpeta == "" {
		t.Fatal("no se creó «Sytue Docs»")
	}
	id := b.ficheros(t, carpeta)["Acta.odt"]
	if "vault:"+id != r.Ancla {
		t.Fatalf("el ancla %q no es la del fichero %q", r.Ancla, id)
	}
	if got := b.leer(t, id); got != "acta secreta del consejo [odf]" {
		t.Fatalf("contenido = %q", got)
	}
	if len(d.anclas) != 1 || d.anclas[0] != [2]string{r.Ancla, ""} {
		t.Fatalf("anclas = %v", d.anclas)
	}
	b.sinClaro(t, "acta secreta")

	// Quien no es el dueño guarda una copia en SU bóveda, sin anclar el documento.
	d2 := &docs{contenido: "otra"}
	r2, err := GuardarDocumento(t.Context(), destinosBoveda(t, b.repo, d2), Documento{Titulo: "Acta", Kind: api.DocText}, d2.exportar)
	if err != nil {
		t.Fatal(err)
	}
	if r2.AnclaDelDocumento != "" || len(d2.anclas) != 0 || r2.Nombre != "Acta (2).odt" {
		t.Fatalf("sin ser el dueño = %+v, anclas %v", r2, d2.anclas)
	}
}

// Con el documento anclado a un fichero vivo de la bóveda: versión nueva de ESE
// fichero, con su nombre, en su carpeta y en el formato de su extensión; sin anclar.
func TestVersionNuevaDelAnclado(t *testing.T) {
	b := nuevoBanco(t)
	carpeta := b.carpeta(t, "Proyectos")
	f := b.subir(t, carpeta, "Plan.docx", "version uno")
	for _, ancla := range []string{"vault:" + f.ID, "drive:" + f.ID} {
		d := &docs{contenido: "version nueva " + ancla[:5]}
		r, err := GuardarDocumento(t.Context(), destinosBoveda(t, b.repo, d), Documento{Ancla: ancla, EsDueno: true, Titulo: "Otro título", Kind: api.DocText}, d.exportar)
		if err != nil {
			t.Fatal(ancla, err)
		}
		if r.Nuevo || r.Nombre != "Plan.docx" || r.Ancla != ancla || r.AnclaDelDocumento != ancla || len(d.anclas) != 0 {
			t.Fatalf("%s: resultado = %+v, anclas %v", ancla, r, d.anclas)
		}
		if len(d.formatos) != 1 || d.formatos[0] != "ooxml" {
			t.Fatalf("%s: formatos = %v", ancla, d.formatos)
		}
		if got := b.leer(t, f.ID); got != d.contenido+" [ooxml]" {
			t.Fatalf("%s: contenido = %q", ancla, got)
		}
	}
	if fs := b.ficheros(t, carpeta); len(fs) != 1 || b.carpetaDocs(t) != "" {
		t.Fatalf("se creó otro fichero: %v", fs)
	}
	got, err := b.repo.File(t.Context(), f.ID)
	if err != nil || got.Version != 3 {
		t.Fatalf("versión = %+v, %v", got, err)
	}
}

// Un ancla cuyo fichero ya no está (o está en la papelera) va a uno nuevo; uno
// «drive:» no se sustituye por el de la bóveda.
func TestAncladoQueYaNoEsta(t *testing.T) {
	b := nuevoBanco(t)
	carpeta := b.carpeta(t, "Viejos")
	f := b.subir(t, carpeta, "Viejo.odt", "x")
	if err := b.repo.Delete(t.Context(), api.DriveFile, f.ID); err != nil {
		t.Fatal(err)
	}
	d := &docs{contenido: "rescate"}
	ancla := "drive:" + f.ID
	r, err := GuardarDocumento(t.Context(), destinosBoveda(t, b.repo, d), Documento{Ancla: ancla, EsDueno: true, Titulo: "Viejo", Kind: api.DocText}, d.exportar)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Nuevo || r.AnclaDelDocumento != ancla || len(d.anclas) != 0 {
		t.Fatalf("resultado = %+v, anclas %v", r, d.anclas)
	}
	if _, ok := b.ficheros(t, b.carpetaDocs(t))["Viejo.odt"]; !ok {
		t.Fatal("no está el fichero nuevo")
	}
}

// Los nombres que ya hay en «Sytue Docs» —ficheros y carpetas— no se pisan nunca: el
// nuevo lleva « (n)», y lo que había sigue igual.
func TestNombresQueChocan(t *testing.T) {
	b := nuevoBanco(t)
	carpeta := b.carpeta(t, CarpetaDocumentos)
	viejo := b.subir(t, carpeta, "Cuentas.ods", "las de antes")
	if _, err := b.repo.CreateFolder(t.Context(), carpeta, "Cuentas (2).ods"); err != nil {
		t.Fatal(err)
	}
	d := &docs{contenido: "las de ahora"}
	r, err := GuardarDocumento(t.Context(), destinosBoveda(t, b.repo, d), Documento{EsDueno: true, Titulo: "Cuentas", Kind: api.DocSheet}, d.exportar)
	if err != nil {
		t.Fatal(err)
	}
	if r.Nombre != "Cuentas (3).ods" {
		t.Fatalf("nombre = %q", r.Nombre)
	}
	if got := b.leer(t, viejo.ID); got != "las de antes" {
		t.Fatalf("se pisó el que había: %q", got)
	}
	if NombreLibre("a", map[string]bool{"a": true}) != "a (2)" || NombreLibre(".odt", map[string]bool{".odt": true}) != ".odt (2)" {
		t.Fatal("NombreLibre no es el de la web")
	}
}

// Un fichero anclado sin ida y vuelta (.pdf) no se reescribe, ni se exporta nada.
func TestFormatoSinVuelta(t *testing.T) {
	b := nuevoBanco(t)
	f := b.subir(t, b.carpeta(t, "PDF"), "Firmado.pdf", "%PDF")
	d := &docs{contenido: "no"}
	_, err := GuardarDocumento(t.Context(), destinosBoveda(t, b.repo, d), Documento{Ancla: "vault:" + f.ID, EsDueno: true, Titulo: "Firmado", Kind: api.DocText}, d.exportar)
	if !errors.Is(err, ErrFormatoSinVuelta) || Motivo(err) != "doc.msg.saveToDriveUnsupported" {
		t.Fatalf("err = %v", err)
	}
	if len(d.formatos) != 0 || b.leer(t, f.ID) != "%PDF" || b.carpetaDocs(t) != "" {
		t.Fatal("se exportó o se escribió algo")
	}
}

// repoQueFalla es la bóveda con la subida rota (sin red a mitad).
type repoQueFalla struct {
	Repositorio
}

var errCorte = errors.New("corte de red artificial")

func (repoQueFalla) Upload(context.Context, string, string, string, io.Reader, int64) (api.File, error) {
	return api.File{}, errCorte
}

func (repoQueFalla) ReplaceFile(context.Context, api.File, string, io.Reader, int64) (api.File, error) {
	return api.File{}, errCorte
}

// Un fallo no deja nada a medias: si exportar falla, no se escribe nada en la bóveda;
// si la subida falla, no queda fichero nuevo ni versión, y no se ancla nada.
func TestUnFalloNoDejaNadaAMedias(t *testing.T) {
	b := nuevoBanco(t)
	registros := b.fake.RecordCount()
	d := &docs{fallar: errors.New("export 502")}
	if _, err := GuardarDocumento(t.Context(), destinosBoveda(t, b.repo, d), Documento{EsDueno: true, Titulo: "A", Kind: api.DocText}, d.exportar); err == nil || Motivo(err) != "doc.msg.saveToDriveFailed" {
		t.Fatalf("err = %v", err)
	}
	if b.fake.RecordCount() != registros || len(d.anclas) != 0 || b.carpetaDocs(t) != "" {
		t.Fatal("una exportación fallida escribió en la bóveda o ancló")
	}

	f := b.subir(t, b.carpeta(t, "Hojas"), "Hoja.xlsx", "la buena")
	rota := repoQueFalla{Repositorio: b.repo}
	d = &docs{contenido: "a medias"}
	for _, ancla := range []string{"", "vault:" + f.ID} {
		_, err := GuardarDocumento(t.Context(), destinosBoveda(t, rota, d), Documento{Ancla: ancla, EsDueno: true, Titulo: "B", Kind: api.DocSheet}, d.exportar)
		if !errors.Is(err, errCorte) {
			t.Fatalf("%q: err = %v", ancla, err)
		}
	}
	if len(d.anclas) != 0 || b.leer(t, f.ID) != "la buena" {
		t.Fatal("una subida fallida ancló o cambió el fichero")
	}
	if c := b.carpetaDocs(t); c != "" && len(b.ficheros(t, c)) != 0 {
		t.Fatalf("una subida fallida dejó ficheros: %v", b.ficheros(t, c))
	}

	// Que el servicio no deje anclar no deshace lo guardado.
	d = &docs{contenido: "guardado", anclar: errors.New("409 vault_anchor_changed")}
	r, err := GuardarDocumento(t.Context(), destinosBoveda(t, b.repo, d), Documento{EsDueno: true, Titulo: "C", Kind: api.DocText}, d.exportar)
	if err != nil || r.AnclaDelDocumento != "" || len(d.anclas) != 1 || b.leer(t, strings.TrimPrefix(r.Ancla, PrefijoBoveda)) != "guardado [odf]" {
		t.Fatalf("anclar fallido = %+v, %v", r, err)
	}
}

// La bóveda cerrada o aún sin terminar de pasar: no se escribe ni se exporta nada y
// cada motivo tiene su frase.
func TestBovedaNoAbierta(t *testing.T) {
	for _, motivo := range []error{ErrBovedaCerrada, ErrBovedaPendiente, ErrBovedaNoDisponible} {
		d := &docs{contenido: "x"}
		dest := Destinos{
			DriveEnBoveda: func(context.Context) (bool, error) { return true, nil },
			Servidor:      &servidorEspia{t: t},
			Boveda:        func(context.Context) (Repositorio, error) { return nil, motivo },
			Anclar:        d.anclarEn,
		}
		_, err := GuardarDocumento(t.Context(), dest, Documento{Ancla: "drive:0b3c6f9e-5d1a-4c2b-9e7f-1a2b3c4d5e99", EsDueno: true, Kind: api.DocText}, d.exportar)
		if !errors.Is(err, motivo) || len(d.formatos) != 0 {
			t.Fatalf("%v: err = %v, formatos %v", motivo, err, d.formatos)
		}
	}
	quiero := map[error]string{
		ErrBovedaCerrada:      "doc.msg.saveToDriveVaultLocked",
		ErrBovedaPendiente:    "doc.msg.saveToDriveVaultNotReady",
		ErrBovedaNoDisponible: "doc.msg.saveToDriveVault",
		&api.Error{Status: http.StatusConflict, Body: `{"error":{"code":"private_storage_migration_required"}}`}: "doc.msg.saveToDriveVault",
	}
	for err, clave := range quiero {
		if Motivo(err) != clave {
			t.Errorf("Motivo(%v) = %q", err, Motivo(err))
		}
	}
}

// servidorFalso es el Drive del servidor (fuente `legacy`): un fichero «drive:».
type servidorFalso struct {
	Repositorio
	f        api.File
	escrito  []byte
	unidadCi bool
}

func (s *servidorFalso) File(_ context.Context, id string) (api.File, error) {
	if id != s.f.ID {
		return api.File{}, &api.Error{Status: http.StatusNotFound}
	}
	return s.f, nil
}

func (s *servidorFalso) ReplaceFile(_ context.Context, f api.File, _ string, cuerpo io.Reader, _ int64) (api.File, error) {
	if s.unidadCi {
		return api.File{}, &api.Error{Status: http.StatusConflict, Body: `{"error":{"code":"private_storage_migration_required"}}`}
	}
	s.escrito, _ = io.ReadAll(cuerpo)
	return f, nil
}

// Con el Drive en el servidor, un «drive:» se guarda allí (lo que hace el BFF de la
// web); sin ancla no hay dónde. Si el Drive pasó a la bóveda entretanto (409), a la
// bóveda; y con el Drive en la bóveda, al servidor no se le manda nada.
func TestDriveEnElServidor(t *testing.T) {
	const id = "0b3c6f9e-5d1a-4c2b-9e7f-1a2b3c4d5e70"
	srv := &servidorFalso{f: api.File{ID: id, FolderID: raizID, Name: "Informe.docx", Version: 4}}
	d := &docs{contenido: "al servidor"}
	boveda := func(context.Context) (Repositorio, error) {
		t.Error("se ha abierto la bóveda con el Drive en el servidor")
		return nil, ErrBovedaCerrada
	}
	dest := Destinos{DriveEnBoveda: func(context.Context) (bool, error) { return false, nil }, Servidor: srv, Boveda: boveda, Anclar: d.anclarEn}
	r, err := GuardarDocumento(t.Context(), dest, Documento{Ancla: "drive:" + id, EsDueno: true, Kind: api.DocText}, d.exportar)
	if err != nil || r.Cifrado || r.Nombre != "Informe.docx" || string(srv.escrito) != "al servidor [ooxml]" {
		t.Fatalf("resultado = %+v, %v, escrito %q", r, err, srv.escrito)
	}
	if _, err := GuardarDocumento(t.Context(), dest, Documento{EsDueno: true, Kind: api.DocText}, d.exportar); !errors.Is(err, ErrSinFichero) {
		t.Fatalf("sin ancla = %v", err)
	}

	b := nuevoBanco(t)
	srv.unidadCi = true
	dest.Boveda = func(context.Context) (Repositorio, error) { return b.repo, nil }
	r, err = GuardarDocumento(t.Context(), dest, Documento{Ancla: "drive:" + id, EsDueno: true, Titulo: "Informe", Kind: api.DocText}, d.exportar)
	if err != nil || !r.Cifrado || !r.Nuevo || r.AnclaDelDocumento != "drive:"+id {
		t.Fatalf("tras el 409 = %+v, %v", r, err)
	}
	if !Ofrecer("", true) || Ofrecer("", false) || !Ofrecer("drive:"+id, false) {
		t.Fatal("Ofrecer no es el de la web")
	}
}

func TestAnclas(t *testing.T) {
	const id = "0b3c6f9e-5d1a-4c2b-9e7f-1a2b3c4d5e62"
	if a, ok := LeerAncla("drive:" + id); !ok || a.Donde != EnServidor || a.ID != id {
		t.Fatal("drive:")
	}
	if a, ok := LeerAncla("vault:" + id); !ok || a.Donde != EnBoveda {
		t.Fatal("vault:")
	}
	for _, mal := range []string{"", "drive:x", "vault:" + strings.ToUpper(id), "otro:" + id} {
		if _, ok := LeerAncla(mal); ok {
			t.Errorf("LeerAncla(%q) vale", mal)
		}
	}
	if _, err := AnclaDeBoveda("no"); !errors.Is(err, ErrAnclaNoValida) {
		t.Fatal("AnclaDeBoveda admite un id que no es uuid")
	}
	if !SePuedeAnclarEnBoveda("") || !SePuedeAnclarEnBoveda("vault:"+id) || SePuedeAnclarEnBoveda("drive:"+id) {
		t.Fatal("SePuedeAnclarEnBoveda no es el del servicio")
	}
}
