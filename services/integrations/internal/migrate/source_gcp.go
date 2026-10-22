// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migrate

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
)

// Raíces de las APIs de Google Cloud. Son configurables por la misma razón que
// las de Drive o Gmail: los tests las apuntan a un servidor simulado y una nube
// soberana puede servirlas en otro dominio.
const (
	ConfigGCPRunBase       = "gcp_run_base"
	ConfigGCPStorageBase   = "gcp_storage_base"
	ConfigGCPSecretBase    = "gcp_secret_base"    //nolint:gosec // G101: nombre de una clave de configuración, no una credencial.
	ConfigGCPSchedulerBase = "gcp_scheduler_base" //nolint:gosec // G101: idem.
	ConfigGCPSQLBase       = "gcp_sql_base"
	ConfigGCPFirestoreBase = "gcp_firestore_base"
	ConfigGCPHostingBase   = "gcp_hosting_base"
	// ConfigGCPProject es el proyecto de Google Cloud del que se migra. Sin él no
	// hay nada que listar: en Google TODO cuelga de un proyecto.
	ConfigGCPProject = "gcp_project"
	// ConfigGCPRegions acota las regiones a recorrer (separadas por comas). Cloud
	// Run NO admite comodín de región, así que sin esto hay que barrer la lista de
	// regiones conocidas; con proyectos que solo usan una o dos, esto ahorra
	// decenas de peticiones inútiles.
	ConfigGCPRegions = "gcp_regions"
)

const (
	defaultRunBase     = "https://run.googleapis.com"
	defaultStorageBase = "https://storage.googleapis.com"
	//nolint:gosec // G101: la raíz pública de una API de Google, no una credencial.
	defaultSecretBase    = "https://secretmanager.googleapis.com"
	defaultSchedulerBase = "https://cloudscheduler.googleapis.com"
	defaultSQLBase       = "https://sqladmin.googleapis.com"
	defaultFirestoreBase = "https://firestore.googleapis.com"
	defaultHostingBase   = "https://firebasehosting.googleapis.com"
	// scopeCloudRead es el scope de LECTURA de toda la plataforma. Google no
	// ofrece uno más fino que cubra Cloud Run, Storage, Secret Manager y
	// Scheduler a la vez, y pedir cinco scopes distintos multiplicaría por cinco
	// las pantallas de consentimiento sin reducir un ápice lo que se puede leer.
	scopeCloudRead = "https://www.googleapis.com/auth/cloud-platform.read-only"
	// gcpPageSize es el tamaño de página de los listados.
	gcpPageSize = 100
)

// gcpRegions son las regiones que se barren cuando la conexión no declara
// ninguna. Cloud Run rechaza el comodín "-" en el listado de servicios (lo dice
// su propio contrato), así que la única forma de encontrar una aplicación es
// preguntar región por región. La lista arranca por Europa: es donde está lo que
// se migra a una plataforma europea, y encontrarlo antes acorta el barrido.
var gcpRegions = []string{
	"europe-west1", "europe-west2", "europe-west3", "europe-west4", "europe-west6",
	"europe-west8", "europe-west9", "europe-north1", "europe-southwest1", "europe-central2",
	"us-central1", "us-east1", "us-east4", "us-west1", "us-west2",
	"northamerica-northeast1", "southamerica-east1",
	"asia-east1", "asia-northeast1", "asia-south1", "asia-southeast1",
	"australia-southeast1",
}

// gcp lee la infraestructura de aplicaciones de un proyecto de Google Cloud (y
// de su Firebase, que es el mismo proyecto con otra cara).
//
// Lo que se lee es la DEFINICIÓN de cada aplicación —imagen, variables, escala,
// dominios—, no su estado interno: eso es lo que hace falta para volver a
// levantarla en Sytue Cloud. Los datos que viven en una base de datos NO se
// leen: se enumeran, con lo que hay que hacer con ellos.
type gcp struct {
	*google
}

// project devuelve el proyecto de Google Cloud del que se migra.
func (g *gcp) project() (string, error) {
	p := strings.TrimSpace(g.gcpProject)
	if p == "" {
		return "", fmt.Errorf(
			"migrate: esta conexión con Google no dice de qué proyecto de Google Cloud migrar; " +
				"indícalo en la conexión (es el identificador que aparece en la consola de Google Cloud, no el nombre visible)",
		)
	}
	return p, nil
}

// regions son las regiones a recorrer.
func (g *gcp) regions() []string {
	raw := strings.TrimSpace(g.gcpRegions)
	if raw == "" {
		return gcpRegions
	}
	out := make([]string, 0, 4)
	for _, r := range strings.Split(raw, ",") {
		if r = strings.TrimSpace(r); r != "" {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		return gcpRegions
	}
	return out
}

// ListApps devuelve las aplicaciones del proyecto: los servicios de Cloud Run y
// los sitios de Firebase Hosting.
func (g *gcp) ListApps(ctx context.Context) ([]CloudApp, error) {
	project, err := g.project()
	if err != nil {
		return nil, err
	}
	apps, err := g.runServices(ctx, project)
	if err != nil {
		return nil, err
	}
	sites, err := g.hostingSites(ctx, project)
	if err != nil {
		// Firebase Hosting puede no estar habilitado, y eso no es un fallo: es un
		// proyecto sin sitios. Lo que NO se puede hacer es tumbar la migración de
		// los servicios de Cloud Run por ello.
		return apps, nil //nolint:nilerr // ausencia de Firebase, no error de migración.
	}
	return append(apps, sites...), nil
}

// runServices lee los servicios de Cloud Run de todas las regiones.
func (g *gcp) runServices(ctx context.Context, project string) ([]CloudApp, error) {
	base, err := baseURLOr(g.gcpRunBase, defaultRunBase)
	if err != nil {
		return nil, err
	}
	out := make([]CloudApp, 0, 8)
	var lastErr error
	found := false
	for _, region := range g.regions() {
		u := fmt.Sprintf("%s/v2/projects/%s/locations/%s/services?pageSize=%d",
			base, url.PathEscape(project), url.PathEscape(region), gcpPageSize)
		var resp runServicesResponse
		if err := getJSON(ctx, g.rt, u, g.token, &resp); err != nil {
			// Una región sin la API habilitada responde 403: es lo normal en 20 de
			// las 22 regiones de un proyecto cualquiera, así que NO puede abortar el
			// barrido. Solo si fallan TODAS hay un problema real que contar.
			lastErr = err
			continue
		}
		found = true
		for _, s := range resp.Services {
			out = append(out, s.toApp(region, project))
		}
	}
	if !found && lastErr != nil {
		return nil, fmt.Errorf(
			"migrate: no se pudo leer Cloud Run en ninguna región del proyecto %q "+
				"(¿tiene la cuenta permiso de lectura y está habilitada la API?): %w", project, lastErr,
		)
	}
	return out, nil
}

// runServicesResponse es el contrato de Cloud Run Admin v2.
//
//nolint:tagliatelle // los tags son el CONTRATO de la API de Google (camelCase).
type runServicesResponse struct {
	Services []runService `json:"services"`
}

//nolint:tagliatelle // contrato de la API de Cloud Run v2.
type runService struct {
	Name        string            `json:"name"` // projects/P/locations/L/services/S
	Description string            `json:"description"`
	URI         string            `json:"uri"`
	URLs        []string          `json:"urls"`
	Labels      map[string]string `json:"labels"`
	Scaling     struct {
		MinInstanceCount int `json:"minInstanceCount"`
	} `json:"scaling"`
	Template struct {
		Scaling struct {
			MinInstanceCount int `json:"minInstanceCount"`
			MaxInstanceCount int `json:"maxInstanceCount"`
		} `json:"scaling"`
		Containers []struct {
			Image string `json:"image"`
			Env   []struct {
				Name      string `json:"name"`
				Value     string `json:"value"`
				ValueFrom struct {
					SecretKeyRef struct {
						Secret  string `json:"secret"`
						Version string `json:"version"`
					} `json:"secretKeyRef"`
				} `json:"valueSource"`
			} `json:"env"`
			Resources struct {
				Limits map[string]string `json:"limits"`
			} `json:"resources"`
		} `json:"containers"`
	} `json:"template"`
}

// toApp normaliza un servicio de Cloud Run.
func (s runService) toApp(region, project string) CloudApp {
	name := s.Name
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	app := CloudApp{
		ID:           "run/" + region + "/" + name,
		Name:         name,
		Kind:         "run",
		Region:       region,
		PublicURL:    s.URI,
		Description:  s.Description,
		MinInstances: s.Template.Scaling.MinInstanceCount,
		MaxInstances: s.Template.Scaling.MaxInstanceCount,
	}
	if len(s.Template.Containers) == 0 {
		app.Skip = "el servicio no declara ningún contenedor, así que no hay nada que desplegar"
		return app
	}
	c := s.Template.Containers[0]
	app.Image = c.Image
	app.CPU = parseCPU(c.Resources.Limits["cpu"])
	app.MemoryMB = parseMemoryMB(c.Resources.Limits["memory"])
	for _, e := range c.Env {
		if ref := e.ValueFrom.SecretKeyRef.Secret; ref != "" {
			// El VALOR vive en Secret Manager y sale por su propia API: aquí solo se
			// sabe a qué secreto apunta. Se traslada el nombre y se dice de dónde
			// hay que sacar el valor, en vez de dejar la variable fuera.
			app.Vars = append(app.Vars, CloudVar{
				Name: e.Name, Secret: true,
				Skip: "el valor está en el secreto " + secretShortName(ref) +
					" de Google Secret Manager; se ha creado la variable vacía para que la rellenes en Sytue Cloud",
			})
			continue
		}
		app.Vars = append(app.Vars, CloudVar{Name: e.Name, Value: e.Value})
	}
	// La imagen vive en el registro de Google (Artifact Registry o gcr.io): la
	// migración NO la copia —son gigabytes por los que se paga salida— y Sytue
	// Cloud construye desde el repositorio del proyecto, que es su modelo.
	if app.Image != "" && !strings.Contains(app.Image, "sytue") {
		app.Description = strings.TrimSpace(app.Description + "\n\nImagen en el origen: " + app.Image +
			". Sytue Cloud construye desde tu repositorio: conéctalo al proyecto y el primer despliegue la reemplaza.")
	}
	_ = project
	return app
}

// hostingSites lee los sitios de Firebase Hosting del proyecto.
func (g *gcp) hostingSites(ctx context.Context, project string) ([]CloudApp, error) {
	base, err := baseURLOr(g.gcpHostingBase, defaultHostingBase)
	if err != nil {
		return nil, err
	}
	//nolint:tagliatelle // contrato de la API de Firebase Hosting.
	var resp struct {
		Sites []struct {
			Name        string `json:"name"` // projects/P/sites/S
			DefaultURL  string `json:"defaultUrl"`
			Type        string `json:"type"`
			AppID       string `json:"appId"`
			SiteID      string `json:"siteId"`
			Description string `json:"labels"`
		} `json:"sites"`
	}
	u := fmt.Sprintf("%s/v1beta1/projects/%s/sites?pageSize=%d", base, url.PathEscape(project), gcpPageSize)
	if err := getJSON(ctx, g.rt, u, g.token, &resp); err != nil {
		return nil, err
	}
	out := make([]CloudApp, 0, len(resp.Sites))
	for _, s := range resp.Sites {
		id := s.SiteID
		if id == "" {
			id = s.Name
			if i := strings.LastIndex(id, "/"); i >= 0 {
				id = id[i+1:]
			}
		}
		out = append(out, CloudApp{
			ID: "hosting/" + id, Name: id, Kind: "hosting", PublicURL: s.DefaultURL,
			Description: "Sitio de Firebase Hosting. Sytue Cloud sirve el sitio construyéndolo " +
				"desde tu repositorio: conéctalo al proyecto y publica.",
		})
	}
	return out, nil
}

// ListBuckets devuelve los buckets de Cloud Storage del proyecto (los de
// Firebase Storage son los mismos: Firebase usa Cloud Storage por debajo).
func (g *gcp) ListBuckets(ctx context.Context) ([]CloudBucket, error) {
	project, err := g.project()
	if err != nil {
		return nil, err
	}
	base, err := baseURLOr(g.gcpStorageBase, defaultStorageBase)
	if err != nil {
		return nil, err
	}
	out := make([]CloudBucket, 0, 8)
	pageToken := ""
	for {
		u := fmt.Sprintf("%s/storage/v1/b?project=%s&maxResults=%d",
			base, url.QueryEscape(project), gcpPageSize)
		if pageToken != "" {
			u += "&pageToken=" + url.QueryEscape(pageToken)
		}
		//nolint:tagliatelle // contrato de la API JSON de Cloud Storage.
		var resp struct {
			Items []struct {
				Name         string `json:"name"`
				Location     string `json:"location"`
				StorageClass string `json:"storageClass"`
			} `json:"items"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := getJSON(ctx, g.rt, u, g.token, &resp); err != nil {
			return nil, err
		}
		for _, b := range resp.Items {
			bucket := CloudBucket{Name: b.Name, Location: b.Location}
			// Un bucket en clase de archivo tarda horas en devolver cada objeto y se
			// paga por rescatarlo: copiarlo sin avisar puede costar una factura seria.
			if b.StorageClass == "ARCHIVE" {
				bucket.Skip = "está en clase ARCHIVE: rescatar cada objeto tarda horas y se factura aparte, " +
					"así que se migra a mano cuando decidas asumir ese coste"
			}
			out = append(out, bucket)
		}
		if resp.NextPageToken == "" {
			return out, nil
		}
		pageToken = resp.NextPageToken
	}
}

// ListObjects devuelve una página de objetos de un bucket.
func (g *gcp) ListObjects(ctx context.Context, bucket, cursor string) ([]CloudObject, string, error) {
	base, err := baseURLOr(g.gcpStorageBase, defaultStorageBase)
	if err != nil {
		return nil, "", err
	}
	u := fmt.Sprintf("%s/storage/v1/b/%s/o?maxResults=%d", base, url.PathEscape(bucket), gcpPageSize)
	if cursor != "" {
		u += "&pageToken=" + url.QueryEscape(cursor)
	}
	//nolint:tagliatelle // contrato de la API JSON de Cloud Storage.
	var resp struct {
		Items []struct {
			Name        string `json:"name"`
			ContentType string `json:"contentType"`
			Size        string `json:"size"` // string: puede superar int32 y JSON no distingue enteros grandes.
		} `json:"items"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := getJSON(ctx, g.rt, u, g.token, &resp); err != nil {
		return nil, "", err
	}
	out := make([]CloudObject, 0, len(resp.Items))
	for _, o := range resp.Items {
		size, _ := strconv.ParseInt(o.Size, 10, 64)
		obj := CloudObject{Bucket: bucket, Key: o.Name, ContentType: o.ContentType, Size: size}
		// Una "carpeta" de Cloud Storage es un objeto de cero bytes acabado en "/".
		// Copiarla crearía un fichero fantasma: el prefijo de los objetos ya la
		// reconstruye en destino.
		if strings.HasSuffix(o.Name, "/") && size == 0 {
			continue
		}
		out = append(out, obj)
	}
	return out, resp.NextPageToken, nil
}

// OpenObject abre el contenido de un objeto (streaming).
func (g *gcp) OpenObject(ctx context.Context, o CloudObject) (io.ReadCloser, error) {
	base, err := baseURLOr(g.gcpStorageBase, defaultStorageBase)
	if err != nil {
		return nil, err
	}
	u := fmt.Sprintf("%s/storage/v1/b/%s/o/%s?alt=media",
		base, url.PathEscape(o.Bucket), url.PathEscape(o.Key))
	return openStream(ctx, g.rt, u, g.token)
}

// ListJobs devuelve las tareas programadas de Cloud Scheduler.
func (g *gcp) ListJobs(ctx context.Context) ([]CloudJob, error) {
	project, err := g.project()
	if err != nil {
		return nil, err
	}
	base, err := baseURLOr(g.gcpSchedulerBase, defaultSchedulerBase)
	if err != nil {
		return nil, err
	}
	out := make([]CloudJob, 0, 4)
	var lastErr error
	found := false
	for _, region := range g.regions() {
		u := fmt.Sprintf("%s/v1/projects/%s/locations/%s/jobs?pageSize=%d",
			base, url.PathEscape(project), url.PathEscape(region), gcpPageSize)
		//nolint:tagliatelle // contrato de la API de Cloud Scheduler.
		var resp struct {
			Jobs []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Schedule    string `json:"schedule"`
				TimeZone    string `json:"timeZone"`
				State       string `json:"state"`
				HTTPTarget  struct {
					URI        string `json:"uri"`
					HTTPMethod string `json:"httpMethod"`
					Body       string `json:"body"` // base64
				} `json:"httpTarget"`
				PubsubTarget struct {
					TopicName string `json:"topicName"`
				} `json:"pubsubTarget"`
			} `json:"jobs"`
		}
		if err := getJSON(ctx, g.rt, u, g.token, &resp); err != nil {
			lastErr = err
			continue
		}
		found = true
		for _, j := range resp.Jobs {
			name := j.Name
			if i := strings.LastIndex(name, "/"); i >= 0 {
				name = name[i+1:]
			}
			job := CloudJob{
				Name: name, Schedule: j.Schedule, TimeZone: j.TimeZone,
				TargetURL: j.HTTPTarget.URI, Method: j.HTTPTarget.HTTPMethod,
			}
			if body, err := decodeBase64(j.HTTPTarget.Body); err == nil {
				job.Body = body
			}
			switch {
			case j.PubsubTarget.TopicName != "":
				job.Skip = "publica en un tema de Pub/Sub, que no tiene equivalente en Sytue Cloud: " +
					"reescríbela como una llamada HTTP a tu aplicación"
			case job.TargetURL == "":
				job.Skip = "no llama a ninguna URL (usa un destino de App Engine), así que no hay nada que programar aquí"
			case j.State == "PAUSED":
				job.Skip = "estaba en pausa en el origen: no se recrea para no encender sola una tarea que alguien paró"
			}
			out = append(out, job)
		}
	}
	if !found && lastErr != nil {
		return nil, fmt.Errorf("migrate: no se pudo leer Cloud Scheduler en el proyecto %q: %w", project, lastErr)
	}
	return out, nil
}

// ListDatabases enumera las bases de datos del proyecto. NO se migran: cada una
// llega al informe con lo que hay que hacer con ella.
func (g *gcp) ListDatabases(ctx context.Context) ([]CloudDatabase, error) {
	project, err := g.project()
	if err != nil {
		return nil, err
	}
	out := make([]CloudDatabase, 0, 4)
	out = append(out, g.sqlInstances(ctx, project)...)
	out = append(out, g.firestoreDatabases(ctx, project)...)
	return out, nil
}

// sqlInstances enumera las instancias de Cloud SQL.
func (g *gcp) sqlInstances(ctx context.Context, project string) []CloudDatabase {
	base, err := baseURLOr(g.gcpSQLBase, defaultSQLBase)
	if err != nil {
		return nil
	}
	//nolint:tagliatelle // contrato de la API de Cloud SQL Admin.
	var resp struct {
		Items []struct {
			Name            string `json:"name"`
			DatabaseVersion string `json:"databaseVersion"`
		} `json:"items"`
	}
	u := fmt.Sprintf("%s/v1/projects/%s/instances", base, url.PathEscape(project))
	if err := getJSON(ctx, g.rt, u, g.token, &resp); err != nil {
		return nil
	}
	out := make([]CloudDatabase, 0, len(resp.Items))
	for _, it := range resp.Items {
		db := CloudDatabase{Name: it.Name, Engine: it.DatabaseVersion}
		if strings.HasPrefix(strings.ToUpper(it.DatabaseVersion), "POSTGRES") {
			db.Skip = "es una base PostgreSQL: aprovisiona Sytue Postgres en el proyecto y restaura tu copia " +
				"con pg_dump/pg_restore. No se copia sola porque hay que decidir cuándo se para la escritura"
		} else {
			db.Skip = "es una base " + it.DatabaseVersion + ": Sytue Cloud ofrece PostgreSQL, " +
				"así que hay que exportarla y adaptarla; no se copia sola"
		}
		out = append(out, db)
	}
	return out
}

// firestoreDatabases enumera las bases de Firestore.
func (g *gcp) firestoreDatabases(ctx context.Context, project string) []CloudDatabase {
	base, err := baseURLOr(g.gcpFirestoreBase, defaultFirestoreBase)
	if err != nil {
		return nil
	}
	//nolint:tagliatelle // contrato de la API de Firestore Admin.
	var resp struct {
		Databases []struct {
			Name string `json:"name"` // projects/P/databases/D
			Type string `json:"type"`
		} `json:"databases"`
	}
	u := fmt.Sprintf("%s/v1/projects/%s/databases", base, url.PathEscape(project))
	if err := getJSON(ctx, g.rt, u, g.token, &resp); err != nil {
		return nil
	}
	out := make([]CloudDatabase, 0, len(resp.Databases))
	for _, d := range resp.Databases {
		name := d.Name
		if i := strings.LastIndex(name, "/"); i >= 0 {
			name = name[i+1:]
		}
		out = append(out, CloudDatabase{
			Name: name, Engine: "firestore",
			Skip: "Firestore guarda documentos y Sytue Cloud da PostgreSQL: no hay conversión automática que " +
				"no invente un esquema. Expórtala desde Google (a Cloud Storage) y decide cómo la modelas",
		})
	}
	return out
}

// secretShortName deja el nombre legible de un secreto de Secret Manager
// (projects/P/secrets/NOMBRE → NOMBRE).
func secretShortName(ref string) string {
	if i := strings.LastIndex(ref, "/"); i >= 0 && i+1 < len(ref) {
		return ref[i+1:]
	}
	return ref
}

// decodeBase64 decodifica el cuerpo de una tarea de Scheduler. Google usa el
// alfabeto estándar, pero acepta el de URL en algunos campos: probar los dos
// cuesta una línea y evita perder el cuerpo de una tarea por un guion bajo.
func decodeBase64(s string) ([]byte, error) {
	if s == "" {
		return nil, nil
	}
	if out, err := base64.StdEncoding.DecodeString(s); err == nil {
		return out, nil
	}
	return base64.URLEncoding.DecodeString(s)
}

// parseCPU traduce el límite de CPU de Cloud Run ("1", "500m") a núcleos.
func parseCPU(v string) float64 {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if strings.HasSuffix(v, "m") {
		milli, err := strconv.ParseFloat(strings.TrimSuffix(v, "m"), 64)
		if err != nil {
			return 0
		}
		return milli / 1000
	}
	cpu, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0
	}
	return cpu
}

// parseMemoryMB traduce el límite de memoria ("512Mi", "1Gi") a megabytes.
func parseMemoryMB(v string) int {
	v = strings.TrimSpace(v)
	switch {
	case v == "":
		return 0
	case strings.HasSuffix(v, "Gi"):
		n, err := strconv.ParseFloat(strings.TrimSuffix(v, "Gi"), 64)
		if err != nil {
			return 0
		}
		return int(n * 1024)
	case strings.HasSuffix(v, "Mi"):
		n, err := strconv.ParseFloat(strings.TrimSuffix(v, "Mi"), 64)
		if err != nil {
			return 0
		}
		return int(n)
	case strings.HasSuffix(v, "G"):
		n, err := strconv.ParseFloat(strings.TrimSuffix(v, "G"), 64)
		if err != nil {
			return 0
		}
		return int(n * 1000)
	case strings.HasSuffix(v, "M"):
		n, err := strconv.ParseFloat(strings.TrimSuffix(v, "M"), 64)
		if err != nil {
			return 0
		}
		return int(n)
	default:
		return 0
	}
}

var _ CloudSource = (*gcp)(nil)
