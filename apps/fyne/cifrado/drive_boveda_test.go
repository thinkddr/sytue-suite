// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/internal/nativestoragetest"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	privateDrive "git.sytue.eu/sytue/one/apps/nucleo/drive"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvault"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvaulttest"
)

// La regla de elección, caso por caso. Ninguna rama pide una clave.
func TestElegirAlmacenDrive(t *testing.T) {
	fallo := errors.New("sin red")
	type estado struct {
		datos, copia, legacy bool
		marcada              bool
		matrix, boveda       int
		err                  error
	}
	consulta := func(e estado) ConsultaDrive {
		return ConsultaDrive{
			BovedaConDatos:   func(context.Context) (bool, error) { return e.datos, e.err },
			PendientesMatrix: func(context.Context) (int, error) { return e.matrix, nil },
			PendientesBoveda: func(context.Context) (int, error) { return e.boveda, nil },
			CopiaPendiente:   func(context.Context) (bool, error) { return e.copia, nil },
			CopiaEnServidor:  func(context.Context) (bool, error) { return e.marcada, nil },
			FuenteLegacy:     func(context.Context) (bool, error) { return e.legacy, nil },
		}
	}
	casos := []struct {
		nombre string
		c      ConsultaDrive
		quiero AlmacenDrive
		falla  bool
	}{
		{"fuente congelada y bóveda vacía: el Drive está en Matrix", consulta(estado{}), DriveEnMatrix, false},
		{"fuente en legacy y bóveda vacía: la bóveda, que importa como la web", consulta(estado{legacy: true}), DriveEnBoveda, false},
		{"la web ya migró: la bóveda", consulta(estado{datos: true}), DriveEnBoveda, false},
		{"subidas pendientes en Matrix: se terminan en Matrix", consulta(estado{datos: true, matrix: 2}), DriveEnMatrix, false},
		{"Matrix pendiente gana aunque la bóveda también tenga", consulta(estado{datos: true, matrix: 1, boveda: 1}), DriveEnMatrix, false},
		{"Matrix pendiente gana también a una fuente en legacy", consulta(estado{legacy: true, matrix: 1}), DriveEnMatrix, false},
		{"pendientes en la bóveda: se terminan en la bóveda", consulta(estado{boveda: 1}), DriveEnBoveda, false},
		{"copia de Matrix a medias: Matrix, aunque la bóveda tenga datos", consulta(estado{datos: true, copia: true, boveda: 3}), DriveEnMatrix, false},
		{"marca de copia en la bóveda (otro dispositivo, o un diario perdido): Matrix", consulta(estado{datos: true, marcada: true, legacy: true}), DriveEnMatrix, false},
		{"pendientes en la bóveda ganan a la marca", consulta(estado{boveda: 1, marcada: true}), DriveEnBoveda, false},
		{"sin saber si hay datos: ni una ni otra", consulta(estado{err: fallo}), 0, true},
		{"sin consulta completa: ni una ni otra", ConsultaDrive{}, 0, true},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			got, err := ElegirAlmacenDrive(t.Context(), caso.c)
			if (err != nil) != caso.falla || got != caso.quiero {
				t.Fatalf("eligió %d (%v), quería %d", got, err, caso.quiero)
			}
		})
	}
	contar := consulta(estado{datos: true})
	contar.PendientesMatrix = func(context.Context) (int, error) { return 0, fallo }
	if _, err := ElegirAlmacenDrive(t.Context(), contar); !errors.Is(err, fallo) {
		t.Fatal("sin poder contar lo pendiente de Matrix no se cambia de almacén", err)
	}
	marca := consulta(estado{datos: true})
	marca.CopiaEnServidor = func(context.Context) (bool, error) { return false, fallo }
	if _, err := ElegirAlmacenDrive(t.Context(), marca); !errors.Is(err, fallo) {
		t.Fatal("sin saber si hay una copia en curso no se elige", err)
	}
	fuente := consulta(estado{})
	fuente.FuenteLegacy = func(context.Context) (bool, error) { return false, fallo }
	if _, err := ElegirAlmacenDrive(t.Context(), fuente); !errors.Is(err, fallo) {
		t.Fatal("sin saber si la fuente sigue en legacy no se elige", err)
	}
}

var scopeDriveBoveda = storage.Scope{Tenant: "acme", Subject: "alice", Product: "drive"}

const duenoDriveBoveda = "acme\x00alice\x00drive"

// bancoDriveBoveda: la sesión de la app de verdad (testigo, arrendamiento y HTTP),
// el servidor falso de la bóveda y dos diarios en directorios distintos, el de
// Matrix y el de la bóveda.
type bancoDriveBoveda struct {
	fake           *pqvaulttest.Fake
	gateway        *api.Drive
	account        *api.Account
	fabrica        FabricaBoveda
	ctx            context.Context
	matrix, boveda *offline.Store
}

func nuevoBancoDriveBoveda(t *testing.T) *bancoDriveBoveda {
	t.Helper()
	session := auth.NewSession(factoryTokens("alice"), nil, nil)
	ctx := session.WithAuthorizationIntent(t.Context())
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("APPDATA", configDir)
	t.Setenv("HOME", configDir)
	ports, err := nativestoragetest.NewSessionPorts(ctx, session, "sesion-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", func(offline.PrivateWork, auth.TestigoTrabajo) (func(), error) { return func() {}, nil })
	if err != nil {
		t.Fatal(err)
	}
	factory := Factory[*factoryDevice]{Product: "drive", CaptureWork: ports.CaptureWork, RegisterWork: ports.RegisterWork, CurrentIdentity: ports.CurrentIdentity, BindHTTP: ports.BindHTTP}
	b := &bancoDriveBoveda{ctx: ctx}
	b.matrix, err = offline.OpenWithSecrets(t.TempDir(), new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	b.boveda, err = offline.OpenWithSecrets(t.TempDir(), new(authtest.Secrets))
	if err != nil {
		t.Fatal(err)
	}
	b.fake, b.gateway, b.account = pqvaulttest.New(t, scopeDriveBoveda)
	b.fabrica = FabricaBoveda{
		Capturar: factory.Capture,
		Diario: func(ctx context.Context, witness auth.TestigoTrabajo) (*offline.Store, error) {
			if err := witness.Validar(); err != nil {
				return nil, err
			}
			return b.boveda, ctx.Err()
		},
	}
	return b
}

func (b *bancoDriveBoveda) abrirBoveda(t *testing.T) *pqvault.Vault {
	t.Helper()
	vault, err := pqvault.Open(t.Context(), pqvault.Config{Scope: scopeDriveBoveda, Server: b.gateway, Registry: b.account, UnlockKey: claveBoveda(9)})
	if err != nil {
		t.Fatal(err)
	}
	return vault
}

// consulta cuenta en los diarios del banco, como lo hace la sesión en disco.
func (b *bancoDriveBoveda) consulta() ConsultaDrive {
	espacios := append(storage.PrivateJournalNamespaces("drive"), storage.PendingUploadsNamespace)
	return ConsultaDrive{
		BovedaConDatos: func(ctx context.Context) (bool, error) {
			return pqvault.HasRecords(ctx, b.gateway, scopeDriveBoveda)
		},
		PendientesMatrix: func(context.Context) (int, error) { return b.matrix.CountOwner(duenoDriveBoveda, espacios...) },
		PendientesBoveda: func(context.Context) (int, error) { return b.boveda.CountOwner(duenoDriveBoveda, espacios...) },
		CopiaPendiente: func(context.Context) (bool, error) {
			n, err := b.boveda.CountOwner(duenoDriveBoveda, privateDrive.VaultMigrationNamespace)
			return n > 0, err
		},
		CopiaEnServidor: func(ctx context.Context) (bool, error) {
			state, err := b.gateway.PQVaultKeys(ctx, scopeDriveBoveda)
			return state.Copy != nil, err
		},
		FuenteLegacy: func(context.Context) (bool, error) { return false, nil },
	}
}

// Un fichero va y vuelve por la bóveda con los recursos de la app: cifrado en el
// servidor, su intención en el diario de la bóveda y nunca en el de Matrix.
func TestVaultResourcesRoundTripFileWithTheirOwnJournal(t *testing.T) {
	b := nuevoBancoDriveBoveda(t)
	recursos, err := b.fabrica.Abrir(b.ctx, b.abrirBoveda(t))
	if err != nil {
		t.Fatal(err)
	}
	defer recursos.Close()
	if recursos.Journal != b.boveda || recursos.PrivateStorageScope() != scopeDriveBoveda {
		t.Fatal("los recursos no llevan el diario de la bóveda o su cuenta")
	}
	ctx := recursos.Context()
	data := bytes.Repeat([]byte("informe cifrado "), 512)
	request := storage.FileRequest{ObjectID: "file_1", RevisionID: "rev_1", SizeBytes: int64(len(data)), Metadata: []byte(`{"name":"informe.txt"}`)}
	ref, err := recursos.Store.UploadStorageFileAt(ctx, b.gateway, recursos.Journal, "job_1", recursos.Store.PublicationTarget(), request, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	for _, blob := range b.fake.Blobs.Data {
		if bytes.Contains(blob, []byte("informe cifrado")) {
			t.Fatal("el servidor ve el fichero en claro")
		}
	}
	var out bytes.Buffer
	if _, err := recursos.Store.DownloadStorageFile(ctx, b.gateway, ref, "file_1", "rev_1", &out); err != nil || !bytes.Equal(out.Bytes(), data) {
		t.Fatal("el fichero no vuelve igual", err)
	}
	if n, _ := b.boveda.CountOwner(duenoDriveBoveda, storage.PendingUploadsNamespace); n != 1 {
		t.Fatal("la subida sin confirmar no quedó en el diario de la bóveda", n)
	}
	if n, _ := b.matrix.CountOwner(duenoDriveBoveda, storage.PendingUploadsNamespace); n != 0 {
		t.Fatal("la subida de la bóveda tocó el diario de Matrix", n)
	}
	if err := recursos.Revalidate(ctx); err != nil {
		t.Fatal(err)
	}
	recursos.Close()
	if err := recursos.Check(ctx); !errors.Is(err, pqvault.ErrClosed) {
		t.Fatal("una bóveda cerrada sigue dando servicio", err)
	}
}

// Una subida pendiente en el diario de Matrix no se pierde al pasar a la bóveda:
// mientras esté, la cuenta sigue en Matrix y nadie toca su intención; cuando
// Matrix la termina, la cuenta pasa a la bóveda, que escribe en su propio diario.
func TestPendingMatrixUploadIsNotLostOnSwitch(t *testing.T) {
	b := nuevoBancoDriveBoveda(t)
	// La web ya migró: la bóveda tiene datos.
	seed := b.abrirBoveda(t)
	if _, err := seed.Put(t.Context(), "txn_web", storage.Record{Version: 1, Scope: scopeDriveBoveda, ObjectID: "obj_web", RevisionID: "rev_web", Role: "content", Payload: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	seed.Close()
	intencion := map[string]string{"target": "!matrix:rooms.example.test", "object": "file_m"}
	if err := b.matrix.Write(duenoDriveBoveda, storage.PendingUploadsNamespace, "job_matrix", intencion); err != nil {
		t.Fatal(err)
	}

	got, err := ElegirAlmacenDrive(t.Context(), b.consulta())
	if err != nil || got != DriveEnMatrix {
		t.Fatal("con una subida de Matrix pendiente la cuenta tiene que seguir en Matrix", got, err)
	}
	var leida map[string]string
	if found, err := b.matrix.Read(duenoDriveBoveda, storage.PendingUploadsNamespace, "job_matrix", &leida); err != nil || !found || leida["object"] != "file_m" {
		t.Fatal("la intención pendiente de Matrix se perdió al elegir", found, err)
	}

	// Matrix la termina (ResumeUploads la confirma y la retira).
	if err := b.matrix.Delete(duenoDriveBoveda, storage.PendingUploadsNamespace, "job_matrix"); err != nil {
		t.Fatal(err)
	}
	got, err = ElegirAlmacenDrive(t.Context(), b.consulta())
	if err != nil || got != DriveEnBoveda {
		t.Fatal("con Matrix al día y la bóveda con datos, la cuenta pasa a la bóveda", got, err)
	}
	recursos, err := b.fabrica.Abrir(b.ctx, b.abrirBoveda(t))
	if err != nil {
		t.Fatal(err)
	}
	defer recursos.Close()
	if _, err := recursos.Store.UploadStorageFileAt(recursos.Context(), b.gateway, recursos.Journal, "job_boveda", recursos.Store.PublicationTarget(), storage.FileRequest{ObjectID: "file_v", RevisionID: "rev_v", SizeBytes: 4, Metadata: []byte(`{}`)}, bytes.NewReader([]byte("hola"))); err != nil {
		t.Fatal(err)
	}
	if n, _ := b.matrix.CountOwner(duenoDriveBoveda, storage.PendingUploadsNamespace); n != 0 {
		t.Fatal("la bóveda escribió en el diario de Matrix", n)
	}
	// Y con la subida de la bóveda a medias, la cuenta se queda en la bóveda
	// aunque la bóveda pareciera vacía.
	b.fake.Records = nil
	if got, err := ElegirAlmacenDrive(t.Context(), b.consulta()); err != nil || got != DriveEnBoveda {
		t.Fatal("lo pendiente de la bóveda se termina en la bóveda", got, err)
	}
}

// La fábrica no se queda una bóveda de otra cuenta ni la deja abierta si falla.
func TestVaultFactoryRejectsForeignVaultAndClosesIt(t *testing.T) {
	b := nuevoBancoDriveBoveda(t)
	otra := storage.Scope{Tenant: "acme", Subject: "bob", Product: "drive"}
	_, gateway, account := pqvaulttest.New(t, otra)
	ajena, err := pqvault.Open(t.Context(), pqvault.Config{Scope: otra, Server: gateway, Registry: account, UnlockKey: claveBoveda(4)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.fabrica.Abrir(b.ctx, ajena); !errors.Is(err, ErrPrivateResources) {
		t.Fatal("se aceptó la bóveda de otra cuenta", err)
	}
	if !ajena.Closed() {
		t.Fatal("la bóveda rechazada quedó abierta")
	}
	b.fabrica.Diario = func(context.Context, auth.TestigoTrabajo) (*offline.Store, error) {
		return nil, offline.ErrEncryptedStore
	}
	propia := b.abrirBoveda(t)
	if _, err := b.fabrica.Abrir(b.ctx, propia); err == nil || !propia.Closed() {
		t.Fatal("sin diario de la bóveda no se abre nada, y la bóveda se cierra", err)
	}
}

// Antes de vaciar la bóveda de Drive se suelta la fuente del servidor: con una
// reautenticación (misma cuenta) o una época cambiada se reintenta una vez; con otra
// cuenta tras entrar, no se suelta nada.
func TestSoltarFuenteAntesDeVaciar(t *testing.T) {
	falla := errors.New("sin red")
	casos := []struct {
		nombre   string
		errores  []error
		misma    bool
		entrar   error
		llamadas int
		entradas int
		quiere   error
	}{
		{nombre: "suelta", errores: []error{nil}, llamadas: 1},
		{nombre: "reautentica", errores: []error{api.ErrReautenticacion, nil}, misma: true, llamadas: 2, entradas: 1},
		{nombre: "otra cuenta", errores: []error{api.ErrReautenticacion}, llamadas: 1, entradas: 1, quiere: auth.ErrAuthorizationChanged},
		{nombre: "no entra", errores: []error{api.ErrReautenticacion}, entrar: falla, llamadas: 1, entradas: 1, quiere: falla},
		{nombre: "época cambiada", errores: []error{api.ErrFuenteCambiada, nil}, llamadas: 2},
		{nombre: "falla", errores: []error{falla}, llamadas: 1, quiere: falla},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			llamadas, entradas := 0, 0
			err := soltarFuenteAntesDeVaciar(t.Context(), func(context.Context) error {
				llamadas++
				return c.errores[llamadas-1]
			}, func(context.Context) error {
				entradas++
				return c.entrar
			}, func() bool { return c.misma })
			if !errors.Is(err, c.quiere) && (err != nil || c.quiere != nil) {
				t.Fatalf("error %v, quería %v", err, c.quiere)
			}
			if llamadas != c.llamadas || entradas != c.entradas {
				t.Fatalf("soltó %d veces y entró %d", llamadas, entradas)
			}
		})
	}
}
