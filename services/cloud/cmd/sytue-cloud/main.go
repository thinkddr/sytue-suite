// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command sytue-cloud es un cliente delgado de la API REST del control plane
// (Sytue Cloud). Cubre el árbol operacional: proyectos, env, dominios,
// servicios gestionados, deployments/rollback, logs de build, uso y billing.
//
// Config por entorno: $SYTUE_CLOUD_API, $SYTUE_CLOUD_TOKEN, $SYTUE_CLOUD_OUTPUT.
// Salida por defecto en tabla; con -o json el JSON crudo para scripting.
// Errores a stderr con código de salida != 0.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

// version la fija el build con -ldflags "-X main.version=..."; "dev" en local.
var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		// `-h`/`--help` en un subcomando llega como flag.ErrHelp: el FlagSet ya
		// imprimió su uso, así que es un camino de ÉXITO (exit 0, sin línea "error:").
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage(os.Stderr)
		return fmt.Errorf("falta subcomando")
	}
	// Las globales (--api/--token/-o/--output) las parsea el FlagSet hoja, que
	// solo se alcanza tras la ruta completa del subcomando. Los switch de
	// despacho ven un token "-flag" como subcomando y fallan. hoistGlobals las
	// mueve al final para que aparezcan en cualquier posición y el leaf las
	// parsee igual, sin duplicar el binding.
	args = hoistGlobals(args)
	switch args[0] {
	case "projects":
		return projectsCmd(args[1:])
	case "env":
		return envCmd(args[1:])
	case "domains":
		return domainsCmd(args[1:])
	case "services":
		return servicesCmd(args[1:])
	case "deployments":
		return deploymentsCmd(args[1:])
	case "deploy": // alias histórico de `deployments create`
		return deploymentsCreate(args[1:])
	case "logs":
		return logsCmd(args[1:])
	case "usage":
		return usageCmd(args[1:])
	case "billing":
		return billingCmd(args[1:])
	case "version", "--version", "-v":
		fmt.Println(version)
		return nil
	case "help", "-h", "--help":
		usage(os.Stdout)
		return nil
	default:
		usage(os.Stderr)
		return fmt.Errorf("subcomando desconocido: %s", args[0])
	}
}

// globalNames son las flags globales que bindGlobals registra en cada hoja.
var globalNames = map[string]bool{"api": true, "token": true, "output": true, "o": true}

// hoistGlobals mueve las flags globales (y su valor) al final de args, dejando
// la ruta de subcomandos al frente para el switch de despacho. Reconoce las
// formas "--flag valor", "--flag=valor" y "-flag" (dash simple, como flag). No
// vuelca ni registra el valor del token; solo lo reordena.
func hoistGlobals(args []string) []string {
	rest := make([]string, 0, len(args))
	var globals []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" { // el resto son posicionales literales
			rest = append(rest, args[i:]...)
			break
		}
		name, hasValue := flagName(a)
		if name != "" && globalNames[name] {
			globals = append(globals, a)
			if !hasValue && i+1 < len(args) { // "--token X": arrastra el valor
				i++
				globals = append(globals, args[i])
			}
			continue
		}
		rest = append(rest, a)
	}
	return append(rest, globals...)
}

// flagName devuelve el nombre de una flag ("-x"/"--x"/"--x=v") y si el token ya
// incluye su valor con "="; "" si a no es una flag.
func flagName(a string) (name string, hasValue bool) {
	if len(a) < 2 || a[0] != '-' {
		return "", false
	}
	a = strings.TrimLeft(a, "-")
	if n, _, ok := strings.Cut(a, "="); ok {
		return n, true
	}
	return a, false
}

func usage(w *os.File) {
	fmt.Fprint(w, `sytue-cloud — CLI del control plane de Sytue Cloud

Uso:
  sytue-cloud <comando> <subcomando> [flags]

Comandos:
  projects     list | get <id> | create --slug S [...] | update <id> [...] | delete <id>
               update cambia solo lo que se teclea: --instance-size, --instances,
               --port, --health-path, --release-command, --name, --repo,
               --production-ref, --framework,
               --build-strategy, --root-dir, --build-command, --output-dir, --install-command
               | restart <id>            reinicia las instancias sin recompilar
               | clone <id> --slug S     copia configuración y variables a un proyecto nuevo
               | db-url <id>             cadena de conexión del Postgres (con contraseña)
  env          list --project <id> | set --project <id> KEY=VALUE [--secret] [--scope S] | unset --project <id> KEY
               | pull --project <id> [--target production|preview]   escribe el .env por stdout
  domains      list --project <id> | add --project <id> --hostname H | remove <id>
  services     list --project <id> | create --project <id> --kind K [--name N] [--config k=v] | delete <id>
               | backups <id> | restore <id> --backup NAME
               | run <id>                        ejecuta ahora un cron o un job
               | logs <id> [--lines N]           registro reciente del servicio
               | download <id> --backup NAME     URL temporal para bajar la copia
  deployments  list --project <id> [--limit N] [--cursor C] | create --project <id> [--image REF] | get <id> | rollback <id> | promote <id> | cancel <id>
  deploy       alias de 'deployments create'
  logs         --build <id> | --deployment <id>  [--q S] [--level L] [--limit N]
               | --project <id> [--q S] [--since T] [--until T] [--limit N] [--cursor C]  (runtime)
  usage        [--from RFC3339] [--to RFC3339]
  billing      account | invoices [--limit N] [--cursor C] | invoice <id>
  version | help

Flags globales (en cualquier posición):
  --api URL      Base de la API ($SYTUE_CLOUD_API, default http://127.0.0.1:8090)
  --token TOKEN  Bearer token; mejor via $SYTUE_CLOUD_TOKEN (--token queda en el history del shell)
  -o, --output   table|json ($SYTUE_CLOUD_OUTPUT, default table)

Ejemplos:
  export SYTUE_CLOUD_TOKEN=scl_...
  sytue-cloud projects list
  sytue-cloud projects create --slug web --repo https://git.gay/acme/web.git --framework astro
  sytue-cloud projects update $PID --instance-size pro --instances 3
  sytue-cloud env set --project $PID API_URL=https://api.acme.com
  sytue-cloud deployments create --project $PID
  sytue-cloud logs --deployment $DID -o json
  sytue-cloud logs --project $PID --q timeout --since 2026-07-01
  sytue-cloud projects restart $PID                     # proceso colgado, sin recompilar
  sytue-cloud deployments promote $DID                  # publica la imagen de una preview
  sytue-cloud services run $SID                         # lanza el cron ahora, para probarlo
  sytue-cloud env pull --project $PID > .env            # el .env del entorno de producción
  psql "$(sytue-cloud projects db-url $PID)"            # tu base, con tu cliente de siempre
  curl -o copia.sql.gz "$(sytue-cloud services download $SID --backup 20260817T030000Z.sql.gz)"
`)
}
