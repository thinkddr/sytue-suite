// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain_test

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

const registro = "rg.fr-par.scw.cloud/sytue"

func app() domain.CatalogApp {
	return domain.CatalogApp{
		ID: "valkey", Name: "Valkey", Description: "Caché en memoria.",
		Category: domain.AppCategoryCache, License: "BSD-3-Clause",
		Homepage:       "https://valkey.io/",
		Upstream:       "docker.io/valkey/valkey:9.1.1-alpine",
		UpstreamDigest: "sha256:" + strings.Repeat("a", 64),
		MirroredDigest: "sha256:" + strings.Repeat("a", 64),
		Version:        "9.1.1",
		Ports:          []domain.AppPort{{Name: "valkey", Port: 6379}},
		Exports:        []domain.AppEnvExport{{Key: "VALKEY_URL", Value: "redis://{host}:{port}"}},
		MemoryMiB:      512, CPUMilli: 500, RunAsUser: 1000,
	}
}

func TestUnaEntradaBienFormadaPasa(t *testing.T) {
	if err := domain.ValidateCatalogApp(app(), registro); err != nil {
		t.Fatalf("la entrada de referencia no valida: %v", err)
	}
}

// TestElDigestDeOrigenTieneQueEstarBienFormado: una etiqueta apunta hoy a un
// contenido y mañana a otro, así que lo que se revisó no sería lo que corre.
func TestElDigestDeOrigenTieneQueEstarBienFormado(t *testing.T) {
	for _, malo := range []string{
		"8.1",                               // una etiqueta
		"sha256:abc",                        // longitud
		"sha512:" + strings.Repeat("a", 64), // otro algoritmo
		"sha256:" + strings.Repeat("A", 64), // mayúsculas
	} {
		a := app()
		a.UpstreamDigest = malo
		a.MirroredDigest = malo
		if domain.ValidateCatalogApp(a, registro) == nil {
			t.Errorf("se aceptó %q", malo)
		}
	}
}

// TestLaReferenciaSaleDelREGISTRODelDespliegue: el namespace del PaaS se llama
// distinto en dev y en producción, así que escribirlo en el catálogo lo dejaba
// apuntando a un sitio que en producción NO existe — y no fallaba hasta que un
// cliente intentaba crear el servicio.
func TestLaReferenciaSaleDelREGISTRODelDespliegue(t *testing.T) {
	a := app()
	got := a.ImageRef("rg.fr-par.scw.cloud/sytue-prod-paas")
	want := "rg.fr-par.scw.cloud/sytue-prod-paas/valkey@" + a.MirroredDigest
	if got != want {
		t.Errorf("referencia = %q; se esperaba %q", got, want)
	}
	if a.ImageRef("rg.fr-par.scw.cloud/otro/") == want {
		t.Error("el registro tiene que mandar; no puede ignorarse")
	}
}

// TestElDigestReplicadoTieneQueSerELDeOrigen es la prueba que protege el espejo:
// `crane copy` conserva el digest, así que si el de nuestra copia no es el del
// origen es que NO se copió lo mismo —casi siempre, un `docker pull` + `push`
// que reconstruyó el manifiesto y se dejó una arquitectura por el camino—.
func TestElDigestReplicadoTieneQueSerELDeOrigen(t *testing.T) {
	a := app()
	a.MirroredDigest = "sha256:" + strings.Repeat("b", 64)
	err := domain.ValidateCatalogApp(a, registro)
	if err == nil {
		t.Fatal("se aceptó una imagen con un digest distinto al de origen")
	}
	if !strings.Contains(err.Error(), "copia fiel") {
		t.Errorf("el error debería explicar por qué importa: %v", err)
	}
}

// TestElEspejoTieneQueSerTRAZABLE: sin saber de dónde viene y con qué digest, la
// entrada no se puede auditar ni volver a replicar.
func TestElEspejoTieneQueSerTRAZABLE(t *testing.T) {
	for _, tc := range []struct {
		nombre string
		mut    func(*domain.CatalogApp)
	}{
		{"sin upstream", func(a *domain.CatalogApp) { a.Upstream = "" }},
		{"upstream sin etiqueta", func(a *domain.CatalogApp) { a.Upstream = "docker.io/valkey/valkey" }},
		{"upstream que somos nosotros", func(a *domain.CatalogApp) { a.Upstream = registro + "/valkey:9" }},
		{"sin digest de origen", func(a *domain.CatalogApp) { a.UpstreamDigest = "" }},
		{"digest de origen mal formado", func(a *domain.CatalogApp) { a.UpstreamDigest = "sha256:abc" }},
	} {
		a := app()
		tc.mut(&a)
		if domain.ValidateCatalogApp(a, registro) == nil {
			t.Errorf("%s: se aceptó", tc.nombre)
		}
	}
}

// TestUnDirectorioNoPuedeSerDatosYEfimeroALaVez: montar los dos encima del mismo
// sitio haría que los datos se perdieran al reiniciar, en silencio.
func TestUnDirectorioNoPuedeSerDatosYEfimeroALaVez(t *testing.T) {
	a := app()
	a.DiskGiB, a.DataPath = 5, "/var/lib/rabbitmq"
	a.WritablePaths = []string{"/var/lib/rabbitmq"}
	if domain.ValidateCatalogApp(a, registro) == nil {
		t.Error("se aceptó el mismo directorio como disco y como efímero")
	}
	a.WritablePaths = []string{"etc/rabbitmq"} // relativa
	if domain.ValidateCatalogApp(a, registro) == nil {
		t.Error("se aceptó una ruta relativa en writable_paths")
	}
}

// TestElOrigenNoPuedeSerNuestroPropioRegistro: si el «upstream» fuera nuestro,
// no habría espejo que auditar — solo una referencia circular.
func TestElOrigenNoPuedeSerNuestroPropioRegistro(t *testing.T) {
	a := app()
	a.Upstream = registro + "/valkey:9.1.1-alpine"
	if domain.ValidateCatalogApp(a, registro) == nil {
		t.Fatal("se aceptó un upstream que somos nosotros mismos")
	}
}

// TestLasLicenciasNoPermitidasSeRECHAZAN. Las «source available» son el caso
// interesante: se anuncian como open source y prohíben exactamente esto.
func TestLasLicenciasNoPermitidasSeRECHAZAN(t *testing.T) {
	for _, lic := range []string{
		"AGPL-3.0",           // ADR 0022
		"SSPL-1.0",           // MongoDB, Redis ≥7.4
		"BUSL-1.1",           // Directus, Sentry
		"Elastic-2.0",        // Elasticsearch
		"LicenseRef-n8n-SUL", // Sustainable Use License
		"",                   // sin declarar
		"mit",                // SPDX es sensible a mayúsculas
	} {
		a := app()
		a.License = lic
		if domain.ValidateCatalogApp(a, registro) == nil {
			t.Errorf("se aceptó la licencia %q", lic)
		}
	}
	// Y las permitidas sí pasan. Las de copyleft fuerte, solo con su fuente: es la
	// condición con la que entraron (ADR 0032).
	for _, lic := range domain.AllowedAppLicenses {
		a := app()
		a.License = lic
		if domain.LicenseIsCopyleft(lic) {
			a.Source = "https://github.com/ejemplo/ejemplo"
		}
		if err := domain.ValidateCatalogApp(a, registro); err != nil {
			t.Errorf("se rechazó la licencia permitida %q: %v", lic, err)
		}
	}
}

// TestElCopyleftExigeElFuente. Una imagen AGPL se puede ofrecer como servicio
// —corre sin modificar, y el digest idéntico al del origen lo demuestra— pero su
// §13 obliga a que quien la usa por red pueda obtener el fuente. Si eso dependiera
// de que alguien se acuerde al escribir el JSON, no se cumpliría.
func TestElCopyleftExigeElFuente(t *testing.T) {
	for _, lic := range domain.CopyleftAppLicenses {
		a := app()
		a.License = lic
		if err := domain.ValidateCatalogApp(a, registro); err == nil {
			t.Errorf("se aceptó %s sin `source`", lic)
		}
	}
	// Y un `source` que no es una URL descargable no cumple nada.
	a := app()
	a.License = "AGPL-3.0-only"
	a.Source = "el repositorio oficial"
	if domain.ValidateCatalogApp(a, registro) == nil {
		t.Error("se aceptó un `source` que no es una URL")
	}
	// A las permisivas no se les exige: su licencia no lo obliga.
	b := app()
	b.License = "MIT"
	if err := domain.ValidateCatalogApp(b, registro); err != nil {
		t.Errorf("se exigió `source` a una licencia permisiva: %v", err)
	}
}

// TestSoloSePublicaUnPuertoQueExiste: un web_port que no está entre los puertos
// declarados crea el Ingress, emite el certificado y devuelve 502.
func TestSoloSePublicaUnPuertoQueExiste(t *testing.T) {
	a := app()
	a.WebPort = 9999
	if domain.ValidateCatalogApp(a, registro) == nil {
		t.Error("se aceptó publicar un puerto que la app no declara")
	}
	a.WebPort = a.Ports[0].Port
	if err := domain.ValidateCatalogApp(a, registro); err != nil {
		t.Errorf("se rechazó publicar un puerto declarado: %v", err)
	}
}

// TestLaPlantillaDeExportsNoEsUnLenguaje: el valor acaba dentro de la app del
// cliente, así que cada capacidad extra es una vía nueva de filtrar algo.
func TestLaPlantillaDeExportsNoEsUnLenguaje(t *testing.T) {
	for _, mala := range []string{
		"redis://{secret}@{host}:{port}",
		"{env.DATABASE_URL}",
		"http://{host}:{port}/{tenant}",
		"",
	} {
		a := app()
		a.Exports = []domain.AppEnvExport{{Key: "X_URL", Value: mala}}
		if domain.ValidateCatalogApp(a, registro) == nil {
			t.Errorf("se aceptó la plantilla %q", mala)
		}
	}
	// Y un nombre de variable que no lo es.
	for _, clave := range []string{"valkey_url", "1URL", "X", "URL-CACHE"} {
		a := app()
		a.Exports = []domain.AppEnvExport{{Key: clave, Value: "redis://{host}:{port}"}}
		if domain.ValidateCatalogApp(a, registro) == nil {
			t.Errorf("se aceptó la clave %q", clave)
		}
	}
}

func TestRenderExportsSustituyeHostYPuerto(t *testing.T) {
	got := domain.RenderExports(app(), "svc-valkey.tenant-acme.svc.cluster.local")
	want := "redis://svc-valkey.tenant-acme.svc.cluster.local:6379"
	if got["VALKEY_URL"] != want {
		t.Errorf("VALKEY_URL = %q; se esperaba %q", got["VALKEY_URL"], want)
	}
}

// TestLosLimitesDeRecursosSonOBLIGATORIOS: un servicio sin techo se come la
// cuota del proyecto y tumba la app a la que venía a ayudar.
func TestLosLimitesDeRecursosSonOBLIGATORIOS(t *testing.T) {
	for _, tc := range []struct {
		nombre string
		mut    func(*domain.CatalogApp)
	}{
		{"sin memoria", func(a *domain.CatalogApp) { a.MemoryMiB = 0 }},
		{"sin cpu", func(a *domain.CatalogApp) { a.CPUMilli = 0 }},
		{"memoria desorbitada", func(a *domain.CatalogApp) { a.MemoryMiB = domain.MaxAppMemoryMiB + 1 }},
		{"cpu desorbitada", func(a *domain.CatalogApp) { a.CPUMilli = domain.MaxAppCPUMilli + 1 }},
		{"disco desorbitado", func(a *domain.CatalogApp) { a.DiskGiB = domain.MaxAppDiskGiB + 1 }},
		{"disco negativo", func(a *domain.CatalogApp) { a.DiskGiB = -1 }},
	} {
		a := app()
		tc.mut(&a)
		if domain.ValidateCatalogApp(a, registro) == nil {
			t.Errorf("%s: se aceptó", tc.nombre)
		}
	}
}

func TestSinPuertoNoHayServicio(t *testing.T) {
	a := app()
	a.Ports = nil
	if domain.ValidateCatalogApp(a, registro) == nil {
		t.Error("se aceptó una entrada sin puertos")
	}
	a = app()
	a.Ports = []domain.AppPort{{Name: "a", Port: 6379}, {Name: "b", Port: 6379}}
	if domain.ValidateCatalogApp(a, registro) == nil {
		t.Error("se aceptó un puerto repetido")
	}
}

// --- El catálogo embebido ---------------------------------------------------

// TestElCatalogoEmbebidoCumpleTodosLosInvariantes es la prueba que de verdad
// protege: las de arriba comprueban la función, esta comprueba lo que SE VA A
// SERVIR.
func TestElCatalogoEmbebidoCumpleTodosLosInvariantes(t *testing.T) {
	cat, err := domain.LoadAppCatalog(registro)
	if err != nil {
		t.Fatalf("el catálogo embebido no carga: %v", err)
	}
	total := len(cat.Apps) + len(cat.PendingMirror)
	if total == 0 {
		t.Fatal("el catálogo está vacío")
	}
	for _, a := range cat.Apps {
		if err := domain.ValidateCatalogApp(a, registro); err != nil {
			t.Errorf("%s: %v", a.ID, err)
		}
	}
	t.Logf("catálogo: %d ofrecibles, %d pendientes de espejo (%v)",
		len(cat.Apps), len(cat.PendingMirror), cat.PendingMirror)
}

// TestNadaSeOFRECEAntesDeReplicarSuImagen: una entrada cuya imagen aún no está
// en nuestro registro no se puede aprovisionar —fallaría al tirar de la imagen,
// después de haberle prometido el servicio al cliente—, así que no sale en la
// lista de ofrecibles. Sale en la de pendientes, que es distinto de esconderla.
func TestNadaSeOFRECEAntesDeReplicarSuImagen(t *testing.T) {
	cat, err := domain.LoadAppCatalog(registro)
	if err != nil {
		t.Fatalf("cargar: %v", err)
	}
	for _, a := range cat.Apps {
		if a.MirroredDigest == "" {
			t.Errorf("%s se ofrece sin copia replicada", a.ID)
		}
	}
	// Hoy, con las cinco entradas sin replicar, la lista ofrecible está vacía.
	// Cuando alguien replique una imagen y ponga su digest real, este test se lo
	// dirá: es el recordatorio de que el trabajo de ops existe.
	if len(cat.Apps) == 0 && len(cat.PendingMirror) == 0 {
		t.Fatal("ni ofrecibles ni pendientes: el catálogo se ha quedado sin entradas")
	}
}

// TestQuienSeEnteraDeQueApareceUnaApp. El catálogo declara de quién sabe consumir
// cada app, no quién la consume a ella; sin recorrerlo al revés, levantar una
// caché no le llegaría nunca al CMS que ya estaba puesto, y no fallaría nada: la
// integración simplemente no existiría.
func TestQuienSeEnteraDeQueApareceUnaApp(t *testing.T) {
	cms := app()
	cms.ID = "cms"
	cms.Integrations = []domain.AppIntegration{{
		App: "cache", Docs: "https://ejemplo.org/d",
		Env: []domain.AppEnvExport{{Key: "CACHE_URL", Value: "redis://{host}:{port}"}},
	}}
	buscador := app()
	buscador.ID = "buscador"
	cache := app()
	cache.ID = "cache"
	cat := domain.AppCatalog{Apps: []domain.CatalogApp{cms, buscador, cache}}

	if got := domain.IntegratorsOf(cat, "cache"); len(got) != 1 || got[0] != "cms" {
		t.Errorf("IntegratorsOf(cache) = %v, quiero [cms]", got)
	}
	if got := domain.IntegratorsOf(cat, "buscador"); len(got) != 0 {
		t.Errorf("nadie integra con el buscador, y salió %v", got)
	}
	if got := domain.IntegratorsOf(cat, ""); len(got) != 0 {
		t.Errorf("un id vacío no integra con nada, y salió %v", got)
	}
}

// TestUnaIntegracionSinDocumentacionOficialNoEntra: sin el ancla a la
// documentación del fabricante, «Sytue las conecta solas» sería Sytue inventando
// la configuración de un proyecto ajeno, y el fallo aparecería en el arranque de
// un contenedor de un cliente.
func TestUnaIntegracionSinDocumentacionOficialNoEntra(t *testing.T) {
	buena := domain.AppIntegration{
		App: "otra", Docs: "https://ejemplo.org/docs",
		Env: []domain.AppEnvExport{{Key: "OTRA_URL", Value: "http://{host}:{port}"}},
	}
	malas := map[string]domain.AppIntegration{
		"sin docs":            {App: "otra", Env: buena.Env},
		"docs que no son URL": {App: "otra", Docs: "el manual", Env: buena.Env},
		"sin variables":       {App: "otra", Docs: buena.Docs},
		"consigo misma":       {App: "valkey", Docs: buena.Docs, Env: buena.Env},
		"plantilla libre": {
			App: "otra", Docs: buena.Docs,
			Env: []domain.AppEnvExport{{Key: "X", Value: "{secret}"}},
		},
	}
	for nombre, mala := range malas {
		a := app()
		a.Integrations = []domain.AppIntegration{mala}
		if domain.ValidateCatalogApp(a, registro) == nil {
			t.Errorf("se aceptó una integración %s", nombre)
		}
	}
	a := app()
	a.Integrations = []domain.AppIntegration{buena}
	if err := domain.ValidateCatalogApp(a, registro); err != nil {
		t.Errorf("se rechazó una integración bien formada: %v", err)
	}
}

// TestUnaIntegracionNoPisaUnSecretoGenerado. Pisarlo con un literal del catálogo
// dejaría al servicio arrancando con la misma contraseña para todos los clientes,
// y nada fallaría.
func TestUnaIntegracionNoPisaUnSecretoGenerado(t *testing.T) {
	a := app()
	a.Env = append(a.Env, domain.AppEnvVar{Key: "CLAVE_MAESTRA", Generate: true})
	a.Integrations = []domain.AppIntegration{{
		App: "otra", Docs: "https://ejemplo.org/d",
		Env: []domain.AppEnvExport{{Key: "CLAVE_MAESTRA", Value: "http://{host}:{port}"}},
	}}
	if domain.ValidateCatalogApp(a, registro) == nil {
		t.Error("una integración pisó un secreto generado por instalación")
	}

	// Dos integraciones sobre la misma variable: el valor dependería de cuál se
	// levantara antes, así que el mismo proyecto arrancaría distinto dos veces.
	b := app()
	b.Integrations = []domain.AppIntegration{
		{
			App: "una", Docs: "https://ejemplo.org/d",
			Env: []domain.AppEnvExport{{Key: "URL", Value: "http://{host}:{port}"}},
		},
		{
			App: "otra", Docs: "https://ejemplo.org/d",
			Env: []domain.AppEnvExport{{Key: "URL", Value: "http://{host}:{port}"}},
		},
	}
	if domain.ValidateCatalogApp(b, registro) == nil {
		t.Error("dos integraciones se pisaron la misma variable")
	}
}

// Un `{db_host}` sin `needs: postgres` es el fallo que no se ve: la app arranca
// —el contenedor no falla— y se conecta a un servidor llamado literalmente
// «{db_host}». El error aparece dentro del log de otro, horas después.
func TestCatalogoRechazaMarcadoresDeBDSinDeclararla(t *testing.T) {
	t.Parallel()
	base := domain.CatalogApp{
		ID: "prueba-db", Name: "Prueba", Description: "x", Category: domain.AppCategoryProductivity,
		License: "MIT", Homepage: "https://ejemplo.com/",
		Upstream: "docker.io/x/y:1", UpstreamDigest: "sha256:" + strings.Repeat("a", 64),
		Version: "1", Ports: []domain.AppPort{{Name: "http", Port: 8080}},
		MemoryMiB: 256, CPUMilli: 100, RunAsUser: 1000,
		Env: []domain.AppEnvVar{{Key: "DB_HOST", Value: "{db_host}"}},
	}
	if err := domain.ValidateCatalogApp(base, ""); err == nil {
		t.Fatal("un marcador {db_*} sin `needs: postgres` tiene que rechazarse")
	}
	// Con el requisito declarado, la misma entrada vale.
	base.Needs = []string{domain.AppNeedPostgres}
	if err := domain.ValidateCatalogApp(base, ""); err != nil {
		t.Fatalf("con `needs: postgres` debería valer: %v", err)
	}
	// Y un requisito que nadie sabe resolver no entra: dejaría la app esperando
	// para siempre a algo que no va a llegar.
	base.Needs = []string{"mongodb"}
	if err := domain.ValidateCatalogApp(base, ""); err == nil {
		t.Fatal("un requisito desconocido tiene que rechazarse")
	}
}

// Los marcadores se sustituyen TODOS o el valor llega a medias: una contraseña
// resuelta con el host sin resolver es una conexión que falla sin decir por qué.
func TestRenderEnvValue(t *testing.T) {
	t.Parallel()
	vars := map[string]string{"{db_host}": "pg.svc", "{db_password}": "s3cr3t", "{web_url}": "https://x.eu"}
	got := domain.RenderEnvValue("postgres://u:{db_password}@{db_host}/d?app={web_url}", vars)
	if got != "postgres://u:s3cr3t@pg.svc/d?app=https://x.eu" {
		t.Fatalf("sustitución incompleta: %s", got)
	}
	// Un marcador que no está en el mapa se queda tal cual: es visible en el log
	// del contenedor, que es mejor que silenciarlo con una cadena vacía.
	if domain.RenderEnvValue("{db_name}", vars) != "{db_name}" {
		t.Error("un marcador sin valor no debería desaparecer")
	}
}

// Un fichero de configuración montado DENTRO del disco de datos lo taparía en
// cada arranque: el usuario vería que sus cambios desaparecen y nada fallaría.
// Es el modo de fallo que este corte existe para impedir.
func TestConfigFilesNoPuedeTaparElDisco(t *testing.T) {
	t.Parallel()
	base := domain.CatalogApp{
		ID: "prueba-cfg", Name: "Prueba", Description: "x", Category: domain.AppCategoryTools,
		License: "MIT", Homepage: "https://ejemplo.com/",
		Upstream: "docker.io/x/y:1", UpstreamDigest: "sha256:" + strings.Repeat("b", 64),
		Version: "1", Ports: []domain.AppPort{{Name: "http", Port: 8080}},
		MemoryMiB: 256, CPUMilli: 100, RunAsUser: 1000,
		// `/var/cache` y no `/tmp`: declarar `/tmp` en writable_paths pasó a ser un
		// error de validación (lo monta siempre el aprovisionador), así que este
		// `base` había dejado de ser válido y el test fallaba en el caso de abajo
		// que sí debía pasar.
		DiskGiB: 5, DataPath: "/data", WritablePaths: []string{"/var/cache"},
	}
	base.ConfigFiles = []domain.AppConfigFile{{Path: "/data/config.yml", Content: "a: 1"}}
	if err := domain.ValidateCatalogApp(base, ""); err == nil {
		t.Error("un fichero dentro del disco de datos tiene que rechazarse")
	}
	base.ConfigFiles = []domain.AppConfigFile{{Path: "/var/cache/config.yml", Content: "a: 1"}}
	if err := domain.ValidateCatalogApp(base, ""); err == nil {
		t.Error("un fichero dentro de un directorio efímero tiene que rechazarse")
	}
	// Y `/tmp` sigue rechazándose aunque ya NO se pueda declarar: se monta
	// siempre, así que un config_file ahí lo taparía igual en cada arranque.
	base.ConfigFiles = []domain.AppConfigFile{{Path: "/tmp/config.yml", Content: "a: 1"}}
	if err := domain.ValidateCatalogApp(base, ""); err == nil {
		t.Error("un fichero dentro de /tmp tiene que rechazarse aunque /tmp no esté declarado")
	}
	// Fuera de los dos, y con contenido, vale.
	base.ConfigFiles = []domain.AppConfigFile{{Path: "/etc/app/config.yml", Content: "a: 1"}}
	if err := domain.ValidateCatalogApp(base, ""); err != nil {
		t.Errorf("una ruta limpia debería valer: %v", err)
	}
	// Sin contenido no: montaría un fichero vacío encima del que trae la imagen,
	// que es la forma más silenciosa de romper una app.
	base.ConfigFiles = []domain.AppConfigFile{{Path: "/etc/app/config.yml", Content: "  "}}
	if err := domain.ValidateCatalogApp(base, ""); err == nil {
		t.Error("un fichero de configuración vacío tiene que rechazarse")
	}
}

// `/tmp` lo monta siempre el aprovisionador. Declararlo en el manifiesto produce
// dos volumeMounts en la misma ruta, el API server rechaza el Deployment y el
// servicio se queda en «provisioning» para siempre sin un error que lo explique.
// Trece entradas del catálogo nacieron así.
func TestWritablePathsNoPuedeRepetirTmp(t *testing.T) {
	t.Parallel()
	base := domain.CatalogApp{
		ID: "prueba-tmp", Name: "Prueba", Description: "x", Category: domain.AppCategoryTools,
		License: "MIT", Homepage: "https://ejemplo.com/",
		Upstream: "docker.io/x/y:1", UpstreamDigest: "sha256:" + strings.Repeat("c", 64),
		Version: "1", Ports: []domain.AppPort{{Name: "http", Port: 8080}},
		MemoryMiB: 256, CPUMilli: 100, RunAsUser: 1000,
	}
	for _, ruta := range []string{"/tmp", "/tmp/cache"} {
		base.WritablePaths = []string{ruta}
		if err := domain.ValidateCatalogApp(base, ""); err == nil {
			t.Errorf("%q tiene que rechazarse: /tmp ya se monta siempre", ruta)
		}
	}
	base.WritablePaths = []string{"/etc/app"}
	if err := domain.ValidateCatalogApp(base, ""); err != nil {
		t.Errorf("una ruta fuera de /tmp debería valer: %v", err)
	}
}
