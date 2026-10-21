// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// ctx: un contexto raíz; el timeout real lo pone el http.Client. Signal-based
// cancel lo añadimos si aparece un comando de larga duración (no hay streaming).
func ctx() context.Context { return context.Background() }

// setup crea el FlagSet con globales, parsea (flags intercalados con
// posicionales) y construye el cliente.
func setup(name string, args []string) (*Client, *Config, []string, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	cfg := bindGlobals(fs)
	pos, err := parseFlags(fs, args)
	if err != nil {
		return nil, nil, nil, err
	}
	cl, err := newClient(cfg)
	if err != nil {
		return nil, nil, nil, err
	}
	return cl, cfg, pos, nil
}

// kvFlags recoge --flag k=v repetible en un mapa.
type kvFlags map[string]string

func (k kvFlags) String() string { return "" }
func (k kvFlags) Set(s string) error {
	p := strings.SplitN(s, "=", 2)
	if len(p) != 2 || p[0] == "" {
		return fmt.Errorf("esperado k=v, recibido %q", s)
	}
	k[p[0]] = p[1]
	return nil
}

func need(pos []string, what string) (string, error) {
	if len(pos) == 0 {
		return "", fmt.Errorf("falta %s", what)
	}
	return pos[0], nil
}

// ---- projects ----

func projectsCmd(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("uso: projects list|get <id>|create --slug S|update <id> [flags]|delete <id>|restart <id>|clone <id> --slug S|db-url <id>")
	}
	switch args[0] {
	case "list":
		return projectsList(args[1:])
	case "get":
		return projectsGet(args[1:])
	case "create":
		return projectsCreate(args[1:])
	case "update":
		return projectsUpdate(args[1:])
	case "delete":
		return projectsDelete(args[1:])
	case "restart":
		return projectsRestart(args[1:])
	case "db-url":
		return projectsDbURL(args[1:])
	case "clone":
		return projectsClone(args[1:])
	default:
		return fmt.Errorf("projects: subcomando desconocido %q", args[0])
	}
}

func projectsList(args []string) error {
	cl, cfg, _, err := setup("projects list", args)
	if err != nil {
		return err
	}
	raw, err := cl.request(ctx(), http.MethodGet, "/api/v1/projects", nil)
	if err != nil {
		return err
	}
	var out struct {
		Projects []project `json:"projects"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return err
	}
	rows := make([][]string, 0, len(out.Projects))
	for _, p := range out.Projects {
		rows = append(rows, []string{p.ID, dash(p.Slug), dash(p.Framework), dash(p.GitProvider), dash(p.RepoURL)})
	}
	return emit(cfg, raw, []string{"ID", "SLUG", "FRAMEWORK", "PROVIDER", "REPO"}, rows)
}

func projectsGet(args []string) error {
	cl, cfg, pos, err := setup("projects get", args)
	if err != nil {
		return err
	}
	id, err := need(pos, "<id>")
	if err != nil {
		return err
	}
	raw, err := cl.request(ctx(), http.MethodGet, "/api/v1/projects/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}
	var p project
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	return emit(cfg, raw, []string{"FIELD", "VALUE"}, projectRows(p))
}

// projectRows es la ficha del proyecto en modo tabla, compartida por `get` y por
// `update` para que tras un cambio se vea exactamente el mismo formato.
func projectRows(p project) [][]string {
	rows := [][]string{
		{"id", p.ID},
		{"slug", p.Slug},
		{"name", dash(p.Name)},
		{"framework", dash(p.Framework)},
		{"build_strategy", dash(p.BuildStrategy)},
		{"provider", dash(p.GitProvider)},
		{"repo_url", dash(p.RepoURL)},
		{"production_ref", dash(p.ProductionRef)},
		// Escala y contrato de la app: lo que hay que ver ANTES de tocar nada con
		// `update`, y lo que se factura. Se muestran siempre, también en su defecto.
		{"instance_size", dash(p.InstanceSize)},
		{"instance_count", fmt.Sprint(p.InstanceCount)},
		{"autoscale_max", zeroAs(p.AutoscaleMaxCount, "auto")},
		{"autoscale_cpu_target_pct", zeroAs(p.AutoscaleCPUTarget, "-")},
		{"app_port", fmt.Sprint(p.AppPort)},
		{"health_path", dash(p.HealthPath)},
		{"release_command", dash(p.ReleaseCommand)},
		{"paused", fmt.Sprint(p.Paused)},
		{"created_at", p.CreatedAt},
	}
	if p.WebhookURL != "" {
		rows = append(rows, []string{"webhook_url", p.WebhookURL})
	}
	if p.WebhookSecret != "" {
		rows = append(rows, []string{"webhook_secret", p.WebhookSecret})
	}
	return rows
}

func projectsCreate(args []string) error {
	fs := flag.NewFlagSet("projects create", flag.ContinueOnError)
	cfg := bindGlobals(fs)
	body := map[string]any{}
	slug := fs.String("slug", "", "slug del proyecto (requerido)")
	name := fs.String("name", "", "nombre visible")
	repo := fs.String("repo", "", "URL del repositorio git")
	provider := fs.String("provider", "", "git|github|gitlab")
	prodRef := fs.String("production-ref", "", "rama de producción")
	framework := fs.String("framework", "", "auto|astro|static|node|go|docker")
	buildStrategy := fs.String("build-strategy", "", "dockerfile|buildpacks (vacío = auto)")
	rootDir := fs.String("root-dir", "", "directorio raíz del build")
	buildCmd := fs.String("build-command", "", "comando de build")
	outputDir := fs.String("output-dir", "", "directorio de salida")
	installCmd := fs.String("install-command", "", "comando de install")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	cl, err := newClient(cfg)
	if err != nil {
		return err
	}
	if *slug == "" {
		return fmt.Errorf("projects create requiere --slug")
	}
	body["slug"] = *slug
	setIf(body, "name", *name)
	setIf(body, "repo_url", *repo)
	setIf(body, "git_provider", *provider)
	setIf(body, "production_ref", *prodRef)
	setIf(body, "framework", *framework)
	setIf(body, "build_strategy", *buildStrategy)
	setIf(body, "root_dir", *rootDir)
	setIf(body, "build_command", *buildCmd)
	setIf(body, "output_dir", *outputDir)
	setIf(body, "install_command", *installCmd)

	raw, err := cl.request(ctx(), http.MethodPost, "/api/v1/projects", body)
	if err != nil {
		return err
	}
	var p project
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	// webhook_secret/webhook_url llegan UNA vez aquí; se muestran a propósito.
	rows := [][]string{{"id", p.ID}, {"slug", p.Slug}}
	if p.WebhookURL != "" {
		rows = append(rows, []string{"webhook_url", p.WebhookURL})
	}
	if p.WebhookSecret != "" {
		rows = append(rows, []string{"webhook_secret", p.WebhookSecret})
	}
	return emit(cfg, raw, []string{"FIELD", "VALUE"}, rows)
}

// projectsUpdate cambia la configuración de un proyecto ya creado. Hasta ahora el
// CLI podía crear y borrar pero no MODIFICAR, así que escalar una app o corregir el
// puerto solo se podía hacer con curl contra la API.
//
// Solo viajan los flags TECLEADOS (`fs.Visit`, no el valor por defecto): un PATCH
// que enviara todos los campos mandaría el cero de los que no se han tocado y
// machacaría configuración ajena al cambio. Es también lo que permite poner un
// valor vacío a propósito (p. ej. `--build-command ""` para quitar el comando).
func projectsUpdate(args []string) error {
	fs := flag.NewFlagSet("projects update", flag.ContinueOnError)
	cfg := bindGlobals(fs)
	name := fs.String("name", "", "nombre visible")
	repo := fs.String("repo", "", "URL del repositorio git")
	prodRef := fs.String("production-ref", "", "rama de producción")
	framework := fs.String("framework", "", "auto|astro|static|node|go|docker")
	buildStrategy := fs.String("build-strategy", "", "dockerfile|buildpacks (vacío = auto)")
	rootDir := fs.String("root-dir", "", "directorio raíz del build")
	buildCmd := fs.String("build-command", "", "comando de build")
	outputDir := fs.String("output-dir", "", "directorio de salida")
	installCmd := fs.String("install-command", "", "comando de install")
	size := fs.String("instance-size", "", "basic|standard|pro|pro-xl")
	instances := fs.Int("instances", 0, "número de instancias (1-50); es el MÍNIMO del autoescalado")
	maxInstances := fs.Int("max-instances", 0, "techo del autoescalado (1-50); 0 = automático")
	cpuTarget := fs.Int("cpu-target", 0, "% de CPU al que escala el autoescalado (10-100); 0 = 75")
	port := fs.Int("port", 0, "puerto en el que escucha la app")
	healthPath := fs.String("health-path", "", "ruta de la sonda de salud")
	paused := fs.Bool("paused", false,
		"apagar el proyecto sin borrarlo (cero réplicas y cero factura de cómputo)")
	releaseCmd := fs.String("release-command", "",
		"comando previo al arranque de la versión nueva (migraciones); \"\" lo quita")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	id, err := need(pos, "<id>")
	if err != nil {
		return err
	}
	cl, err := newClient(cfg)
	if err != nil {
		return err
	}

	// flag tecleado -> campo del PATCH. El orden no importa: es un mapa de body.
	campos := map[string]struct {
		field string
		val   func() any
	}{
		"name":            {"name", func() any { return *name }},
		"repo":            {"repo_url", func() any { return *repo }},
		"production-ref":  {"production_ref", func() any { return *prodRef }},
		"framework":       {"framework", func() any { return *framework }},
		"build-strategy":  {"build_strategy", func() any { return *buildStrategy }},
		"root-dir":        {"root_dir", func() any { return *rootDir }},
		"build-command":   {"build_command", func() any { return *buildCmd }},
		"output-dir":      {"output_dir", func() any { return *outputDir }},
		"install-command": {"install_command", func() any { return *installCmd }},
		"instance-size":   {"instance_size", func() any { return *size }},
		"instances":       {"instance_count", func() any { return *instances }},
		"max-instances":   {"autoscale_max_count", func() any { return *maxInstances }},
		"cpu-target":      {"autoscale_cpu_target", func() any { return *cpuTarget }},
		"port":            {"app_port", func() any { return *port }},
		"health-path":     {"health_path", func() any { return *healthPath }},
		// `fs.Visit` solo recorre los flags TECLEADOS, así que pasar
		// --release-command="" es la forma de quitarlo: la cadena vacía viaja y el
		// control-plane la interpreta como «sin comando».
		"release-command": {"release_command", func() any { return *releaseCmd }},
		"paused":          {"paused", func() any { return *paused }},
	}
	body := map[string]any{}
	fs.Visit(func(f *flag.Flag) {
		if c, ok := campos[f.Name]; ok {
			body[c.field] = c.val()
		}
	})
	if len(body) == 0 {
		return fmt.Errorf("projects update: no se ha indicado ningún cambio (ver --help)")
	}

	raw, err := cl.request(ctx(), http.MethodPatch, "/api/v1/projects/"+url.PathEscape(id), body)
	if err != nil {
		return err
	}
	var p project
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	return emit(cfg, raw, []string{"FIELD", "VALUE"}, projectRows(p))
}

func projectsDelete(args []string) error {
	cl, _, pos, err := setup("projects delete", args)
	if err != nil {
		return err
	}
	id, err := need(pos, "<id>")
	if err != nil {
		return err
	}
	if _, err := cl.request(ctx(), http.MethodDelete, "/api/v1/projects/"+url.PathEscape(id), nil); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "proyecto %s borrado\n", id)
	return nil
}

// projectsRestart reinicia las instancias de la producción activa.
//
// Es el verbo que faltaba para arreglar un proceso colgado desde un script: sin
// él, la única forma automatizable era volver a desplegar, que recompila y cobra
// minutos para acabar corriendo el mismo commit.
func projectsRestart(args []string) error {
	cl, cfg, pos, err := setup("projects restart", args)
	if err != nil {
		return err
	}
	id, err := need(pos, "<id>")
	if err != nil {
		return err
	}
	raw, err := cl.request(ctx(), http.MethodPost, "/api/v1/projects/"+url.PathEscape(id)+"/restart", map[string]any{})
	if err != nil {
		return err
	}
	var r struct {
		Restarted int `json:"restarted"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return err
	}
	// Cero no es un fallo: es que no había nada corriendo. Se dice tal cual en vez
	// de callar y dejar a quien lo lanzó esperando un efecto que no va a llegar.
	return emit(cfg, raw, []string{"REINICIADAS"}, [][]string{{strconv.Itoa(r.Restarted)}})
}

// projectsDbURL imprime la cadena de conexión completa del Postgres del proyecto.
//
// Lo que hace utilizable la base desde fuera del panel: `psql "$(sytue-cloud
// projects db-url $PID)"`. Va a stdout SOLA —sin tabla ni etiqueta— justamente
// para poder encadenarla; el aviso, a stderr, para que no se cuele dentro de una
// redirección. El control-plane exige admin/owner y lo deja en la auditoría.
func projectsDbURL(args []string) error {
	cl, _, pos, err := setup("projects db-url", args)
	if err != nil {
		return err
	}
	id, err := need(pos, "<id|slug>")
	if err != nil {
		return err
	}
	raw, err := cl.request(ctx(), http.MethodGet, "/api/v1/projects/"+url.PathEscape(id)+"/db/credentials", nil)
	if err != nil {
		return err
	}
	var r struct {
		ConnectionString string `json:"connection_string"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "contraseña en claro: no la dejes en el history ni en un log")
	fmt.Println(r.ConnectionString)
	return nil
}

// projectsClone crea otro proyecto con la misma configuración y variables.
func projectsClone(args []string) error {
	fs := flag.NewFlagSet("projects clone", flag.ContinueOnError)
	cfg := bindGlobals(fs)
	slug := fs.String("slug", "", "slug del proyecto nuevo (obligatorio)")
	name := fs.String("name", "", "nombre; por defecto, el del origen")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	id, err := need(pos, "<id>")
	if err != nil {
		return err
	}
	if *slug == "" {
		return fmt.Errorf("--slug es obligatorio")
	}
	cl, err := newClient(cfg)
	if err != nil {
		return err
	}
	raw, err := cl.request(ctx(), http.MethodPost, "/api/v1/projects/"+url.PathEscape(id)+"/clone",
		map[string]any{"slug": *slug, "name": *name})
	if err != nil {
		return err
	}
	var r struct {
		Project project `json:"project"`
		Secret  string  `json:"webhook_secret"`
		Copied  int     `json:"env_vars_copied"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return err
	}
	// El secreto del webhook va a stderr y no a la tabla: es una credencial que
	// solo se puede leer esta vez, y en stdout se la llevaría un `| jq` a un
	// fichero de log.
	fmt.Fprintf(os.Stderr, "secreto de webhook (solo se muestra una vez): %s\n", r.Secret)
	return emit(cfg, raw, []string{"ID", "SLUG", "VARIABLES"},
		[][]string{{r.Project.ID, dash(r.Project.Slug), strconv.Itoa(r.Copied)}})
}

// ---- env ----

func envCmd(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("uso: env list|set|unset|pull --project <id>")
	}
	switch args[0] {
	case "list":
		return envList(args[1:])
	case "set":
		return envSet(args[1:])
	case "unset":
		return envUnset(args[1:])
	case "pull":
		return envPull(args[1:])
	default:
		return fmt.Errorf("env: subcomando desconocido %q", args[0])
	}
}

// envPull escribe por la salida estándar el `.env` del entorno pedido.
//
// Se llama `pull` porque es lo que hace y lo que ya espera quien viene de otras
// plataformas. A stdout y no a un fichero: así `sytue-cloud env pull --project X
// > .env` es una decisión de quien lo lanza y no una sorpresa que machaca lo que
// hubiera. Revela los secretos en claro; el control-plane exige admin/owner y lo
// deja escrito en la auditoría.
func envPull(args []string) error {
	fs := flag.NewFlagSet("env pull", flag.ContinueOnError)
	cfg := bindGlobals(fs)
	project := fs.String("project", "", "id del proyecto (obligatorio)")
	target := fs.String("target", "production", "entorno: production|preview")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	if *project == "" {
		return fmt.Errorf("--project es obligatorio")
	}
	cl, err := newClient(cfg)
	if err != nil {
		return err
	}
	raw, err := cl.request(ctx(), http.MethodGet,
		"/api/v1/projects/"+url.PathEscape(*project)+"/env/export?target="+url.QueryEscape(*target), nil)
	if err != nil {
		return err
	}
	// El cuerpo YA es el fichero: no se decodifica ni se reformatea, porque
	// cualquier paso de más aquí es una ocasión de romper el escapado que el
	// servidor hizo bien.
	_, err = os.Stdout.Write(raw)
	return err
}

func envList(args []string) error {
	fs := flag.NewFlagSet("env list", flag.ContinueOnError)
	cfg := bindGlobals(fs)
	proj := fs.String("project", "", "UUID del proyecto (requerido)")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	cl, err := newClient(cfg)
	if err != nil {
		return err
	}
	if *proj == "" {
		return fmt.Errorf("env list requiere --project")
	}
	raw, err := cl.request(ctx(), http.MethodGet, "/api/v1/projects/"+url.PathEscape(*proj)+"/env", nil)
	if err != nil {
		return err
	}
	var out struct {
		Env []envVar `json:"env"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return err
	}
	rows := make([][]string, 0, len(out.Env))
	for _, e := range out.Env {
		val := e.Value
		if e.IsSecret {
			val = "(secret)"
		}
		rows = append(rows, []string{e.Key, dash(val), boolStr(e.IsSecret), dash(e.Scope)})
	}
	return emit(cfg, raw, []string{"KEY", "VALUE", "SECRET", "SCOPE"}, rows)
}

func envSet(args []string) error {
	fs := flag.NewFlagSet("env set", flag.ContinueOnError)
	cfg := bindGlobals(fs)
	proj := fs.String("project", "", "UUID del proyecto (requerido)")
	secret := fs.Bool("secret", false, "marcar como secreto")
	scope := fs.String("scope", "", "all|production|preview")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	cl, err := newClient(cfg)
	if err != nil {
		return err
	}
	if *proj == "" {
		return fmt.Errorf("env set requiere --project")
	}
	kv, err := need(pos, "KEY=VALUE")
	if err != nil {
		return err
	}
	parts := strings.SplitN(kv, "=", 2)
	if len(parts) != 2 || parts[0] == "" {
		return fmt.Errorf("esperado KEY=VALUE, recibido %q", kv)
	}
	body := map[string]any{"key": parts[0], "value": parts[1], "is_secret": *secret}
	setIf(body, "scope", *scope)
	raw, err := cl.request(ctx(), http.MethodPut, "/api/v1/projects/"+url.PathEscape(*proj)+"/env", body)
	if err != nil {
		return err
	}
	var e envVar
	_ = json.Unmarshal(raw, &e)
	return emit(cfg, raw, []string{"KEY", "SCOPE"}, [][]string{{parts[0], dash(e.Scope)}})
}

func envUnset(args []string) error {
	fs := flag.NewFlagSet("env unset", flag.ContinueOnError)
	cfg := bindGlobals(fs)
	proj := fs.String("project", "", "UUID del proyecto (requerido)")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	cl, err := newClient(cfg)
	if err != nil {
		return err
	}
	if *proj == "" {
		return fmt.Errorf("env unset requiere --project")
	}
	key, err := need(pos, "KEY")
	if err != nil {
		return err
	}
	// La API borra por id (/env/{id}); resolvemos KEY -> id listando el env.
	raw, err := cl.request(ctx(), http.MethodGet, "/api/v1/projects/"+url.PathEscape(*proj)+"/env", nil)
	if err != nil {
		return err
	}
	var out struct {
		Env []envVar `json:"env"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return err
	}
	id := ""
	for _, e := range out.Env {
		if e.Key == key {
			id = e.ID
			break
		}
	}
	if id == "" {
		return fmt.Errorf("no existe la variable %q en el proyecto", key)
	}
	if _, err := cl.request(ctx(), http.MethodDelete, "/api/v1/env/"+url.PathEscape(id), nil); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "variable %s borrada\n", key)
	return nil
}

// ---- domains ----

func domainsCmd(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("uso: domains list --project <id>|add --project <id> --hostname H|remove <id>")
	}
	switch args[0] {
	case "list":
		return domainsList(args[1:])
	case "add":
		return domainsAdd(args[1:])
	case "remove":
		return domainsRemove(args[1:])
	default:
		return fmt.Errorf("domains: subcomando desconocido %q", args[0])
	}
}

func domainsList(args []string) error {
	fs := flag.NewFlagSet("domains list", flag.ContinueOnError)
	cfg := bindGlobals(fs)
	proj := fs.String("project", "", "UUID del proyecto (requerido)")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	cl, err := newClient(cfg)
	if err != nil {
		return err
	}
	if *proj == "" {
		return fmt.Errorf("domains list requiere --project")
	}
	raw, err := cl.request(ctx(), http.MethodGet, "/api/v1/projects/"+url.PathEscape(*proj)+"/domains", nil)
	if err != nil {
		return err
	}
	var out struct {
		Domains []domain `json:"domains"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return err
	}
	rows := make([][]string, 0, len(out.Domains))
	for _, d := range out.Domains {
		rows = append(rows, []string{d.ID, d.Hostname, dash(d.Status), boolStr(d.IsPrimary)})
	}
	return emit(cfg, raw, []string{"ID", "HOSTNAME", "STATUS", "PRIMARY"}, rows)
}

func domainsAdd(args []string) error {
	fs := flag.NewFlagSet("domains add", flag.ContinueOnError)
	cfg := bindGlobals(fs)
	proj := fs.String("project", "", "UUID del proyecto (requerido)")
	hostname := fs.String("hostname", "", "hostname a añadir (requerido)")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	cl, err := newClient(cfg)
	if err != nil {
		return err
	}
	if *proj == "" || *hostname == "" {
		return fmt.Errorf("domains add requiere --project y --hostname")
	}
	raw, err := cl.request(ctx(), http.MethodPost, "/api/v1/projects/"+url.PathEscape(*proj)+"/domains", map[string]any{"hostname": *hostname})
	if err != nil {
		return err
	}
	var out struct {
		Domain domain `json:"domain"`
		DNS    struct {
			CNAMETarget string `json:"cname_target"`
			TXTRecord   string `json:"txt_record"`
			TXTValue    string `json:"txt_value"`
		} `json:"dns"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return err
	}
	rows := [][]string{
		{"id", out.Domain.ID},
		{"hostname", out.Domain.Hostname},
		{"status", dash(out.Domain.Status)},
		{"dns:cname", dash(out.DNS.CNAMETarget)},
		{"dns:txt_record", dash(out.DNS.TXTRecord)},
		{"dns:txt_value", dash(out.DNS.TXTValue)},
	}
	return emit(cfg, raw, []string{"FIELD", "VALUE"}, rows)
}

func domainsRemove(args []string) error {
	cl, _, pos, err := setup("domains remove", args)
	if err != nil {
		return err
	}
	id, err := need(pos, "<id>")
	if err != nil {
		return err
	}
	if _, err := cl.request(ctx(), http.MethodDelete, "/api/v1/domains/"+url.PathEscape(id), nil); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "dominio %s eliminado\n", id)
	return nil
}

// ---- services ----

func servicesCmd(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("uso: services list --project <id>|create --project <id> --kind K|delete <id>|backups <id>|restore <id> --backup NAME|run <id>|logs <id>|download <id> --backup NAME")
	}
	switch args[0] {
	case "list":
		return servicesList(args[1:])
	case "create":
		return servicesCreate(args[1:])
	case "delete":
		return servicesDelete(args[1:])
	case "backups":
		return servicesBackups(args[1:])
	case "restore":
		return servicesRestore(args[1:])
	case "run":
		return servicesRun(args[1:])
	case "logs":
		return servicesLogs(args[1:])
	case "download":
		return servicesDownload(args[1:])
	default:
		return fmt.Errorf("services: subcomando desconocido %q", args[0])
	}
}

func servicesList(args []string) error {
	fs := flag.NewFlagSet("services list", flag.ContinueOnError)
	cfg := bindGlobals(fs)
	proj := fs.String("project", "", "UUID del proyecto (requerido)")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	cl, err := newClient(cfg)
	if err != nil {
		return err
	}
	if *proj == "" {
		return fmt.Errorf("services list requiere --project")
	}
	raw, err := cl.request(ctx(), http.MethodGet, "/api/v1/projects/"+url.PathEscape(*proj)+"/services", nil)
	if err != nil {
		return err
	}
	var out struct {
		Services []managedService `json:"services"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return err
	}
	rows := make([][]string, 0, len(out.Services))
	for _, s := range out.Services {
		rows = append(rows, []string{s.ID, dash(s.Kind), dash(s.Name), dash(s.Status)})
	}
	return emit(cfg, raw, []string{"ID", "KIND", "NAME", "STATUS"}, rows)
}

func servicesCreate(args []string) error {
	fs := flag.NewFlagSet("services create", flag.ContinueOnError)
	cfg := bindGlobals(fs)
	proj := fs.String("project", "", "UUID del proyecto (requerido)")
	kind := fs.String("kind", "", "postgres|storage|functions|cron|job|auth (requerido)")
	name := fs.String("name", "", "nombre del servicio")
	config := kvFlags{}
	fs.Var(config, "config", "config k=v (repetible; p. ej. --config schedule=@daily)")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	cl, err := newClient(cfg)
	if err != nil {
		return err
	}
	if *proj == "" || *kind == "" {
		return fmt.Errorf("services create requiere --project y --kind")
	}
	body := map[string]any{"kind": *kind}
	setIf(body, "name", *name)
	if len(config) > 0 {
		body["config"] = map[string]string(config)
	}
	raw, err := cl.request(ctx(), http.MethodPost, "/api/v1/projects/"+url.PathEscape(*proj)+"/services", body)
	if err != nil {
		return err
	}
	var s managedService
	if err := json.Unmarshal(raw, &s); err != nil {
		return err
	}
	return emit(cfg, raw, []string{"ID", "KIND", "NAME", "STATUS"}, [][]string{{s.ID, dash(s.Kind), dash(s.Name), dash(s.Status)}})
}

func servicesDelete(args []string) error {
	cl, _, pos, err := setup("services delete", args)
	if err != nil {
		return err
	}
	id, err := need(pos, "<id>")
	if err != nil {
		return err
	}
	if _, err := cl.request(ctx(), http.MethodDelete, "/api/v1/services/"+url.PathEscape(id), nil); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "servicio %s marcado para eliminación\n", id)
	return nil
}

// servicesBackups lista los backups (dumps) de un Sytue Postgres. El servidor solo
// devuelve nombres de fichero bajo el prefijo propio del servicio.
func servicesBackups(args []string) error {
	cl, cfg, pos, err := setup("services backups", args)
	if err != nil {
		return err
	}
	id, err := need(pos, "<id>")
	if err != nil {
		return err
	}
	raw, err := cl.request(ctx(), http.MethodGet, "/api/v1/services/"+url.PathEscape(id)+"/backups", nil)
	if err != nil {
		return err
	}
	var out struct {
		Backups []struct {
			Key          string `json:"key"`
			Size         int64  `json:"size"`
			LastModified string `json:"last_modified"`
		} `json:"backups"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return err
	}
	rows := make([][]string, 0, len(out.Backups))
	for _, b := range out.Backups {
		rows = append(rows, []string{b.Key, strconv.FormatInt(b.Size, 10), dash(b.LastModified)})
	}
	return emit(cfg, raw, []string{"BACKUP", "SIZE_BYTES", "MODIFIED"}, rows)
}

// servicesRestore dispara la restauración de un backup elegido sobre el Postgres del
// servicio (destructivo: sobrescribe la BD). El cliente pasa SOLO el nombre; el
// servidor compone el objeto S3 bajo el prefijo del servicio (anti cross-tenant).
func servicesRestore(args []string) error {
	fs := flag.NewFlagSet("services restore", flag.ContinueOnError)
	cfg := bindGlobals(fs)
	backup := fs.String("backup", "", "nombre del backup a restaurar (requerido; ver 'services backups <id>')")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	cl, err := newClient(cfg)
	if err != nil {
		return err
	}
	id, err := need(pos, "<id>")
	if err != nil {
		return err
	}
	if *backup == "" {
		return fmt.Errorf("services restore requiere --backup (usa 'services backups %s' para listarlos)", id)
	}
	if _, err := cl.request(ctx(), http.MethodPost, "/api/v1/services/"+url.PathEscape(id)+"/restore",
		map[string]any{"backup": *backup}); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "restore de %s solicitado desde el backup %q (destructivo: sobrescribe la base de datos)\n", id, *backup)
	return nil
}

// servicesRun ejecuta ahora una tarea programada o puntual.
//
// Es lo que convierte un cron en algo probable desde un script: antes había que
// esperar a su hora o retocarle el horario y acordarse de devolverlo.
func servicesRun(args []string) error {
	cl, _, pos, err := setup("services run", args)
	if err != nil {
		return err
	}
	id, err := need(pos, "<service-id>")
	if err != nil {
		return err
	}
	if _, err := cl.request(ctx(), http.MethodPost, "/api/v1/services/"+url.PathEscape(id)+"/run", map[string]any{}); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "tarea %s lanzada; su registro se lee con `services logs %s`\n", id, id)
	return nil
}

// servicesLogs enseña el registro reciente de un servicio gestionado.
func servicesLogs(args []string) error {
	fs := flag.NewFlagSet("services logs", flag.ContinueOnError)
	cfg := bindGlobals(fs)
	lines := fs.Int("lines", 200, "número de líneas")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	id, err := need(pos, "<service-id>")
	if err != nil {
		return err
	}
	cl, err := newClient(cfg)
	if err != nil {
		return err
	}
	raw, err := cl.request(ctx(), http.MethodGet,
		fmt.Sprintf("/api/v1/services/%s/logs?lines=%d", url.PathEscape(id), *lines), nil)
	if err != nil {
		return err
	}
	var r struct {
		Lines []struct {
			TS   string `json:"ts"`
			Text string `json:"text"`
		} `json:"lines"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return err
	}
	// Un registro no es una tabla: se imprime tal cual para poder pasarlo por
	// `grep` sin que las columnas estorben. El modo -o json sigue dando el crudo.
	if cfg.Output == "json" {
		return emit(cfg, raw, nil, nil)
	}
	for _, l := range r.Lines {
		ts := l.TS
		if len(ts) >= 19 {
			ts = ts[11:19] // solo la hora: el nanosegundo de Kubernetes no aporta
		}
		fmt.Println(ts, l.Text)
	}
	return nil
}

// servicesDownload imprime la URL temporal para descargar una copia de seguridad.
//
// Imprime la URL en vez de bajar el fichero: así se encadena con `curl -o` sin
// que este binario tenga que gestionar ficheros grandes, reanudaciones ni
// permisos de escritura.
func servicesDownload(args []string) error {
	fs := flag.NewFlagSet("services download", flag.ContinueOnError)
	cfg := bindGlobals(fs)
	backup := fs.String("backup", "", "nombre del fichero de copia (obligatorio)")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	id, err := need(pos, "<service-id>")
	if err != nil {
		return err
	}
	if *backup == "" {
		return fmt.Errorf("--backup es obligatorio (ver `services backups <id>`)")
	}
	cl, err := newClient(cfg)
	if err != nil {
		return err
	}
	raw, err := cl.request(ctx(), http.MethodGet,
		"/api/v1/services/"+url.PathEscape(id)+"/backups/download?backup="+url.QueryEscape(*backup), nil)
	if err != nil {
		return err
	}
	var r struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return err
	}
	// La URL caduca en 15 minutos y es una credencial: va a stdout para poder
	// encadenarla, y el aviso a stderr para que no acabe dentro del fichero.
	fmt.Fprintln(os.Stderr, "URL firmada válida 15 minutos; no la compartas")
	fmt.Println(r.URL)
	return nil
}

// ---- deployments ----

func deploymentsCmd(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("uso: deployments list|create|get <id>|rollback <id>|promote <id>|cancel <id>")
	}
	switch args[0] {
	case "list":
		return deploymentsList(args[1:])
	case "create":
		return deploymentsCreate(args[1:])
	case "get":
		return deploymentsGet(args[1:])
	case "rollback":
		return deploymentsRollback(args[1:])
	case "promote":
		return deploymentsPromote(args[1:])
	case "cancel":
		return deploymentsCancel(args[1:])
	default:
		return fmt.Errorf("deployments: subcomando desconocido %q", args[0])
	}
}

func deploymentsList(args []string) error {
	fs := flag.NewFlagSet("deployments list", flag.ContinueOnError)
	cfg := bindGlobals(fs)
	proj := fs.String("project", "", "UUID del proyecto (requerido)")
	limit := fs.Int("limit", 0, "máximo de resultados (1-200)")
	cursor := fs.String("cursor", "", "cursor de paginación keyset")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	cl, err := newClient(cfg)
	if err != nil {
		return err
	}
	if *proj == "" {
		return fmt.Errorf("deployments list requiere --project")
	}
	q := url.Values{}
	if *limit > 0 {
		q.Set("limit", fmt.Sprint(*limit))
	}
	if *cursor != "" {
		q.Set("cursor", *cursor)
	}
	raw, err := cl.request(ctx(), http.MethodGet, "/api/v1/projects/"+url.PathEscape(*proj)+"/deployments"+queryString(q), nil)
	if err != nil {
		return err
	}
	var out struct {
		Deployments []deployment `json:"deployments"`
		NextCursor  string       `json:"next_cursor"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return err
	}
	rows := make([][]string, 0, len(out.Deployments))
	for _, d := range out.Deployments {
		rows = append(rows, []string{d.ID, dash(d.Target), dash(d.Status), dash(shortSHA(d.CommitSHA)), dash(d.URL), d.CreatedAt})
	}
	if err := emit(cfg, raw, []string{"ID", "TARGET", "STATUS", "COMMIT", "URL", "CREATED"}, rows); err != nil {
		return err
	}
	if cfg.Output == "table" && out.NextCursor != "" {
		fmt.Fprintf(os.Stderr, "next_cursor: %s\n", out.NextCursor)
	}
	return nil
}

func deploymentsCreate(args []string) error {
	fs := flag.NewFlagSet("deployments create", flag.ContinueOnError)
	cfg := bindGlobals(fs)
	proj := fs.String("project", "", "UUID del proyecto (requerido)")
	image := fs.String("image", "", "imagen OCI BYO (despliega sin build desde git)")
	commit := fs.String("commit", "", "commit SHA a desplegar")
	gitRef := fs.String("ref", "", "git ref/rama a desplegar")
	target := fs.String("target", "", "production|preview")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	cl, err := newClient(cfg)
	if err != nil {
		return err
	}
	if *proj == "" {
		return fmt.Errorf("deployments create requiere --project")
	}
	body := map[string]any{}
	setIf(body, "image_ref", *image)
	setIf(body, "commit_sha", *commit)
	setIf(body, "git_ref", *gitRef)
	setIf(body, "target", *target)
	raw, err := cl.request(ctx(), http.MethodPost, "/api/v1/projects/"+url.PathEscape(*proj)+"/deployments", body)
	if err != nil {
		return err
	}
	var out struct {
		Deployment deployment `json:"deployment"`
		Build      struct {
			ID string `json:"id"`
		} `json:"build"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return err
	}
	rows := [][]string{
		{"deployment_id", out.Deployment.ID},
		{"status", dash(out.Deployment.Status)},
		{"target", dash(out.Deployment.Target)},
		{"build_id", dash(out.Build.ID)},
	}
	return emit(cfg, raw, []string{"FIELD", "VALUE"}, rows)
}

func deploymentsGet(args []string) error {
	cl, cfg, pos, err := setup("deployments get", args)
	if err != nil {
		return err
	}
	id, err := need(pos, "<id>")
	if err != nil {
		return err
	}
	raw, err := cl.request(ctx(), http.MethodGet, "/api/v1/deployments/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}
	var d deployment
	if err := json.Unmarshal(raw, &d); err != nil {
		return err
	}
	rows := [][]string{
		{"id", d.ID},
		{"project_id", d.ProjectID},
		{"build_id", dash(d.BuildID)},
		{"target", dash(d.Target)},
		{"status", dash(d.Status)},
		{"commit", dash(d.CommitSHA)},
		{"git_ref", dash(d.GitRef)},
		{"url", dash(d.URL)},
		{"created_at", d.CreatedAt},
		{"ready_at", dash(d.ReadyAt)},
		{"error", dash(d.Error)},
	}
	return emit(cfg, raw, []string{"FIELD", "VALUE"}, rows)
}

func deploymentsRollback(args []string) error {
	cl, cfg, pos, err := setup("deployments rollback", args)
	if err != nil {
		return err
	}
	id, err := need(pos, "<deployment-id>")
	if err != nil {
		return err
	}
	raw, err := cl.request(ctx(), http.MethodPost, "/api/v1/deployments/"+url.PathEscape(id)+"/rollback", map[string]any{})
	if err != nil {
		return err
	}
	var d deployment
	if err := json.Unmarshal(raw, &d); err != nil {
		return err
	}
	return emit(cfg, raw, []string{"ID", "TARGET", "STATUS", "URL"}, [][]string{{d.ID, dash(d.Target), dash(d.Status), dash(d.URL)}})
}

// deploymentsPromote publica en producción la imagen de una preview.
//
// Es otro verbo que `rollback`, aunque los dos acaben en producción: aquel exige
// que el despliegue YA sea de producción y devuelve 409 con una preview. Aquí se
// crea una producción nueva reusando la imagen que se revisó, sin recompilar.
func deploymentsPromote(args []string) error {
	cl, cfg, pos, err := setup("deployments promote", args)
	if err != nil {
		return err
	}
	id, err := need(pos, "<deployment-id>")
	if err != nil {
		return err
	}
	raw, err := cl.request(ctx(), http.MethodPost, "/api/v1/deployments/"+url.PathEscape(id)+"/promote", map[string]any{})
	if err != nil {
		return err
	}
	var d deployment
	if err := json.Unmarshal(raw, &d); err != nil {
		return err
	}
	return emit(cfg, raw, []string{"ID", "TARGET", "STATUS", "URL"}, [][]string{{d.ID, dash(d.Target), dash(d.Status), dash(d.URL)}})
}

func deploymentsCancel(args []string) error {
	cl, _, pos, err := setup("deployments cancel", args)
	if err != nil {
		return err
	}
	id, err := need(pos, "<id>")
	if err != nil {
		return err
	}
	if _, err := cl.request(ctx(), http.MethodDelete, "/api/v1/deployments/"+url.PathEscape(id), nil); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "deployment %s cancelado\n", id)
	return nil
}

// ---- logs ----
//
// Dos fuentes distintas y una sola orden:
//   - --build/--deployment: log de BUILD, texto plano (/builds/{id}/logs), con
//     q/level/limit; --deployment resuelve su build_id.
//   - --project: logs de RUNTIME ya guardados (/projects/{id}/logs), buscables
//     por texto y rango con paginación keyset.
//
// --follow sigue sin existir: el tail en vivo es SSE y esta CLI no lo habla.
func logsCmd(args []string) error {
	fs := flag.NewFlagSet("logs", flag.ContinueOnError)
	cfg := bindGlobals(fs)
	build := fs.String("build", "", "UUID de la build")
	deploy := fs.String("deployment", "", "UUID del deployment (resuelve su build)")
	proj := fs.String("project", "", "UUID del proyecto (logs de runtime, buscables)")
	follow := fs.Bool("follow", false, "no soportado: la API sirve logs estáticos, sin streaming")
	q := fs.String("q", "", "filtro subcadena case-insensitive")
	level := fs.String("level", "", "filtro por nivel (INFO/WARN/ERROR); solo logs de build")
	since := fs.String("since", "", "logs de runtime: desde (RFC3339 o AAAA-MM-DD)")
	until := fs.String("until", "", "logs de runtime: hasta (RFC3339 o AAAA-MM-DD)")
	cursor := fs.String("cursor", "", "logs de runtime: cursor de paginación keyset")
	limit := fs.Int("limit", 0, "máximo de líneas")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	if *follow {
		return fmt.Errorf("--follow no disponible: la API sirve logs estáticos, sin streaming")
	}
	cl, err := newClient(cfg)
	if err != nil {
		return err
	}
	if *proj != "" {
		return runtimeLogs(cl, cfg, *proj, *q, *since, *until, *cursor, *limit)
	}
	buildID := *build
	if buildID == "" && *deploy != "" {
		raw, err := cl.request(ctx(), http.MethodGet, "/api/v1/deployments/"+url.PathEscape(*deploy), nil)
		if err != nil {
			return err
		}
		var d deployment
		if err := json.Unmarshal(raw, &d); err != nil {
			return err
		}
		if d.BuildID == "" {
			return fmt.Errorf("el deployment %s no tiene build asociada (¿BYO-image?); no hay logs", *deploy)
		}
		buildID = d.BuildID
	}
	if buildID == "" {
		return fmt.Errorf("logs requiere --project <id> (runtime), --build <id> o --deployment <id>")
	}
	qs := url.Values{}
	if *q != "" {
		qs.Set("q", *q)
	}
	if *level != "" {
		qs.Set("level", *level)
	}
	if *limit > 0 {
		qs.Set("limit", fmt.Sprint(*limit))
	}
	raw, err := cl.request(ctx(), http.MethodGet, "/api/v1/builds/"+url.PathEscape(buildID)+"/logs"+queryString(qs), nil)
	if err != nil {
		return err
	}
	// Logs = texto plano; se vuelca tal cual en ambos modos.
	_, err = os.Stdout.Write(raw)
	return err
}

// runtimeLogs busca en los logs de runtime del proyecto. En modo tabla se imprime
// una línea por línea de log (instante + texto), que es como se lee un log; el
// cursor de la siguiente página va por stderr para no ensuciar un `| grep`.
func runtimeLogs(cl *Client, cfg *Config, projectID, q, since, until, cursor string, limit int) error {
	qs := url.Values{}
	setIfQuery(qs, "q", q)
	setIfQuery(qs, "since", since)
	setIfQuery(qs, "until", until)
	setIfQuery(qs, "cursor", cursor)
	if limit > 0 {
		qs.Set("limit", fmt.Sprint(limit))
	}
	raw, err := cl.request(ctx(), http.MethodGet,
		"/api/v1/projects/"+url.PathEscape(projectID)+"/logs"+queryString(qs), nil)
	if err != nil {
		return err
	}
	var out struct {
		Lines []struct {
			TS   string `json:"ts"`
			Text string `json:"text"`
		} `json:"lines"`
		NextCursor string `json:"next_cursor"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return err
	}
	if cfg.Output == "json" {
		return renderJSON(os.Stdout, raw)
	}
	for _, l := range out.Lines {
		fmt.Fprintf(os.Stdout, "%s  %s\n", l.TS, l.Text)
	}
	if out.NextCursor != "" {
		fmt.Fprintf(os.Stderr, "next_cursor: %s\n", out.NextCursor)
	}
	return nil
}

// setIfQuery añade el parámetro solo si trae valor (queryString ya omite los
// vacíos, pero así no se escriben claves que luego hay que filtrar).
func setIfQuery(q url.Values, key, val string) {
	if val != "" {
		q.Set(key, val)
	}
}

// ---- usage & billing ----

func usageCmd(args []string) error {
	fs := flag.NewFlagSet("usage", flag.ContinueOnError)
	cfg := bindGlobals(fs)
	from := fs.String("from", "", "inicio del periodo (RFC3339)")
	to := fs.String("to", "", "fin del periodo (RFC3339)")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	cl, err := newClient(cfg)
	if err != nil {
		return err
	}
	q := url.Values{}
	if *from != "" {
		q.Set("from", *from)
	}
	if *to != "" {
		q.Set("to", *to)
	}
	raw, err := cl.request(ctx(), http.MethodGet, "/api/v1/usage"+queryString(q), nil)
	if err != nil {
		return err
	}
	var out struct {
		Usage []struct {
			Metric string  `json:"metric"`
			Total  float64 `json:"total"`
			Unit   string  `json:"unit"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return err
	}
	rows := make([][]string, 0, len(out.Usage))
	for _, u := range out.Usage {
		rows = append(rows, []string{u.Metric, fmt.Sprintf("%g", u.Total), dash(u.Unit)})
	}
	return emit(cfg, raw, []string{"METRIC", "TOTAL", "UNIT"}, rows)
}

func billingCmd(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("uso: billing account|invoices|invoice <id>")
	}
	switch args[0] {
	case "account":
		return billingAccount(args[1:])
	case "invoices":
		return billingInvoices(args[1:])
	case "invoice":
		return billingInvoice(args[1:])
	default:
		return fmt.Errorf("billing: subcomando desconocido %q", args[0])
	}
}

func billingAccount(args []string) error {
	cl, cfg, _, err := setup("billing account", args)
	if err != nil {
		return err
	}
	raw, err := cl.request(ctx(), http.MethodGet, "/api/v1/billing/account", nil)
	if err != nil {
		return err
	}
	var out struct {
		Account struct {
			Tenant        string   `json:"tenant"`
			Plan          string   `json:"plan"`
			SpendLimitEUR *float64 `json:"spend_limit_eur"`
		} `json:"account"`
		CurrentSpendEUR float64 `json:"current_spend_eur"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return err
	}
	limit := "sin tope"
	if out.Account.SpendLimitEUR != nil {
		limit = fmt.Sprintf("%.2f", *out.Account.SpendLimitEUR)
	}
	rows := [][]string{
		{"tenant", out.Account.Tenant},
		{"plan", out.Account.Plan},
		{"spend_limit_eur", limit},
		{"current_spend_eur", fmt.Sprintf("%.2f", out.CurrentSpendEUR)},
	}
	return emit(cfg, raw, []string{"FIELD", "VALUE"}, rows)
}

func billingInvoices(args []string) error {
	fs := flag.NewFlagSet("billing invoices", flag.ContinueOnError)
	cfg := bindGlobals(fs)
	limit := fs.Int("limit", 0, "máximo de resultados (1-200)")
	cursor := fs.String("cursor", "", "cursor de paginación keyset")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	cl, err := newClient(cfg)
	if err != nil {
		return err
	}
	q := url.Values{}
	if *limit > 0 {
		q.Set("limit", fmt.Sprint(*limit))
	}
	if *cursor != "" {
		q.Set("cursor", *cursor)
	}
	raw, err := cl.request(ctx(), http.MethodGet, "/api/v1/billing/invoices"+queryString(q), nil)
	if err != nil {
		return err
	}
	var out struct {
		Invoices   []invoice `json:"invoices"`
		NextCursor string    `json:"next_cursor"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return err
	}
	rows := make([][]string, 0, len(out.Invoices))
	for _, inv := range out.Invoices {
		rows = append(rows, []string{inv.ID, dash(inv.Status), inv.PeriodFrom, inv.PeriodTo, fmt.Sprintf("%.2f", inv.TotalEUR)})
	}
	if err := emit(cfg, raw, []string{"ID", "STATUS", "FROM", "TO", "TOTAL_EUR"}, rows); err != nil {
		return err
	}
	if cfg.Output == "table" && out.NextCursor != "" {
		fmt.Fprintf(os.Stderr, "next_cursor: %s\n", out.NextCursor)
	}
	return nil
}

func billingInvoice(args []string) error {
	cl, cfg, pos, err := setup("billing invoice", args)
	if err != nil {
		return err
	}
	id, err := need(pos, "<id>")
	if err != nil {
		return err
	}
	raw, err := cl.request(ctx(), http.MethodGet, "/api/v1/billing/invoices/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}
	var inv invoice
	if err := json.Unmarshal(raw, &inv); err != nil {
		return err
	}
	rows := [][]string{
		{"id", inv.ID},
		{"status", dash(inv.Status)},
		{"period_from", inv.PeriodFrom},
		{"period_to", inv.PeriodTo},
		{"subtotal_eur", fmt.Sprintf("%.2f", inv.SubtotalEUR)},
		{"total_eur", fmt.Sprintf("%.2f", inv.TotalEUR)},
	}
	return emit(cfg, raw, []string{"FIELD", "VALUE"}, rows)
}

// ---- helpers ----

func setIf(m map[string]any, key, val string) {
	if val != "" {
		m[key] = val
	}
}

func boolStr(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func shortSHA(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

func queryString(q url.Values) string {
	if len(q) == 0 {
		return ""
	}
	return "?" + q.Encode()
}
