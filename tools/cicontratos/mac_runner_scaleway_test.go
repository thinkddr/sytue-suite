// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cicontratos

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// Contratos del guion real deploy/sytue-git/bin/mac-runner-scaleway.sh con un
// proveedor y un SSH de mentira, desechables: sin credenciales de la nube. El
// doble (scw, gh, ssh y scp a la vez) anota cada llamada en calls.jsonl y
// responde según FAKE_SCENARIO; el ssh ejecuta el guion remoto con el bootstrap
// sustituido por una sonda que devuelve el token y la etiqueta que le llegan.

const macRunnerScalewayDoble = `#!/usr/bin/env python3
import json,os,pathlib,subprocess,sys
root=pathlib.Path(os.environ['FAKE_ROOT'])
name=pathlib.Path(sys.argv[0]).name
args=sys.argv[1:]
with (root/'calls.jsonl').open('a') as f:f.write(json.dumps([name,*args])+'\n')
scenario=os.environ['FAKE_SCENARIO']
server={'id':'11111111-1111-1111-1111-111111111111','name':os.environ['SYTUE_GIT_MAC_NAME'],'type':'M2-M','ip':'192.0.2.4','deletable_at':'2050-01-01T00:00:00Z','deletion_scheduled':True}
if name=='scw':
 if args[:3]==['apple-silicon','server','list']:
  if scenario=='resume_wrong_type':server['type']='M4-M'
  out=[server] if scenario in ('existing','resume_existing','resume_wrong_type') else []
 elif args[:3]==['apple-silicon','server-type','list']:out=[{'name':'M2-M','stock':'no_stock' if scenario=='no_stock' else 'low_stock'}]
 elif args[:3]==['apple-silicon','os','list']:out=[{'id':'os-stable','name':'macos-stable','label':'macOS Stable','family':'Tahoe','version':'26','compatible_server_types':['M2-M']},{'id':'os-beta','name':'macos-beta','label':'macOS beta','family':'Next','version':'99','compatible_server_types':['M2-M']},{'id':'os-linux','name':'asahi-linux','label':'Linux','family':'Asahi','version':'999','compatible_server_types':['M2-M']}]
 elif args[:3]==['apple-silicon','server','create']:
  server['deletion_scheduled']=False
  if scenario=='foreign_create':server['name']='unrelated-existing-server'
  out=server
 elif args[:3]==['apple-silicon','server','update']:
  count=sum(1 for line in (root/'calls.jsonl').read_text().splitlines() if json.loads(line)[:4]==['scw','apple-silicon','server','update'])
  if scenario=='ordering_forever' or scenario=='ordering_once' and count==1 or scenario=='unexpected_error':
   print(json.dumps({'error':{'resource':'option','current_state':'ordering' if scenario!='unexpected_error' else 'failed'}}),file=sys.stderr)
   sys.exit(1)
  if scenario=='unarmed':server['deletion_scheduled']=False
  out=server
 elif args[:3]==['apple-silicon','server','wait']:
  if scenario=='lost_guard':server['deletion_scheduled']=False
  out=server
 else:sys.exit('unexpected provider call')
 print(json.dumps(out))
elif name=='gh':print(os.environ['FAKE_EXPECT_TOKEN'])
elif name=='scp':pass
elif name=='ssh':
 data=sys.stdin.read()
 if data:
  if os.environ['FAKE_EXPECT_TOKEN'] in '\0'.join(args):sys.exit('token exposed in ssh argv')
  (root/'remote-script').write_text(data)
  marker='bash /tmp/sytue-git/bin/bootstrap-runner-macos.sh'
  if data.count(marker)!=1:sys.exit('missing bootstrap')
  probe="python3 -c 'import os,json;print(json.dumps({k:os.environ[k] for k in [\"SYTUE_GIT_MACOS_RUNNER_TOKEN\",\"SYTUE_GIT_MACOS_LABEL\"]}))'"
  result=subprocess.run(['bash','-s'],input=data.replace(marker,probe),capture_output=True,text=True)
  if result.returncode:sys.exit('invalid remote shell')
  values=json.loads(result.stdout)
  if values['SYTUE_GIT_MACOS_RUNNER_TOKEN']!=os.environ['FAKE_EXPECT_TOKEN'] or values['SYTUE_GIT_MACOS_LABEL']!=os.environ['SYTUE_GIT_MACOS_LABEL']:sys.exit('remote values changed')
`

// El token lleva comillas y una sustitución: si el guion lo interpolara en shell,
// crearía escaped-token-guard.
const macRunnerScalewayToken = "artificial token ' $(touch escaped-token-guard)"

type macRunnerScalewayCaso struct {
	res   guiontest.Resultado
	calls [][]string
}

// macRunnerScalewayCorrer ejecuta `mac-runner-scaleway.sh crear` con el doble
// delante en el PATH y devuelve el resultado y las llamadas, en orden.
func macRunnerScalewayCorrer(t *testing.T, escenario string, provisto, reanudar bool) macRunnerScalewayCaso {
	t.Helper()
	fixture := t.TempDir()
	for _, orden := range []string{"scw", "gh", "ssh", "scp"} {
		ruta := filepath.Join(fixture, orden)
		guiontest.Escribir(t, ruta, macRunnerScalewayDoble)
		if err := os.Chmod(ruta, 0o700); err != nil { //nolint:gosec // G302: el doble tiene que ser ejecutable.
			t.Fatal(err)
		}
	}
	booleano := func(b bool) string {
		if b {
			return "1"
		}
		return "0"
	}
	token := ""
	if provisto {
		token = macRunnerScalewayToken
	}
	res := guiontest.Guion(t, guiontest.Opciones{Dir: fixture, Plazo: 30 * time.Second, Env: []string{
		guiontest.ConPATH(fixture), "FAKE_ROOT=" + fixture,
		"FAKE_SCENARIO=" + escenario, "FAKE_EXPECT_TOKEN=" + macRunnerScalewayToken,
		"SYTUE_GIT_ENV_FILE=" + filepath.Join(fixture, "missing-env"), "SYTUE_GIT_URL=https://github.com",
		"SYTUE_GIT_MAC_NAME=sytue-qa-fixture", "SYTUE_GIT_MAC_TYPE=M2-M",
		"SYTUE_GIT_MACOS_LABEL=sytue-apple-qa-remote", "SYTUE_GIT_SSH_KEY=",
		"SYTUE_GIT_MAC_RESUME_EXISTING=" + booleano(reanudar),
		"SYTUE_GIT_MAC_DELETE_ATTEMPTS=3", "SYTUE_GIT_MAC_DELETE_RETRY_SECONDS=0",
		"SYTUE_GIT_MACOS_RUNNER_TOKEN=" + token,
	}}, "deploy/sytue-git/bin/mac-runner-scaleway.sh", "crear")
	var calls [][]string
	for linea := range strings.SplitSeq(strings.TrimSpace(guiontest.Leer(t, filepath.Join(fixture, "calls.jsonl"))), "\n") {
		var c []string
		if err := json.Unmarshal([]byte(linea), &c); err != nil {
			t.Fatalf("llamada ilegible %q: %v", linea, err)
		}
		calls = append(calls, c)
	}
	if strings.Contains(res.Todo(), macRunnerScalewayToken) {
		t.Fatalf("el token salió por stdout o stderr\n%s", res.Todo())
	}
	if _, err := os.Stat(filepath.Join(fixture, "escaped-token-guard")); err == nil {
		t.Fatal("el token se evaluó como shell: existe escaped-token-guard")
	}
	return macRunnerScalewayCaso{res: res, calls: calls}
}

func macRunnerScalewayEs(c []string, prefijo ...string) bool {
	return len(c) >= len(prefijo) && reflect.DeepEqual(c[:len(prefijo)], prefijo)
}

// indice es la primera llamada que cumple, o -1.
func (c macRunnerScalewayCaso) indice(cumple func([]string) bool) int {
	return slices.IndexFunc(c.calls, cumple)
}

func (c macRunnerScalewayCaso) hay(prefijo ...string) bool {
	return c.indice(func(x []string) bool { return macRunnerScalewayEs(x, prefijo...) }) >= 0
}

func (c macRunnerScalewayCaso) cuantas(prefijo ...string) int {
	n := 0
	for _, x := range c.calls {
		if macRunnerScalewayEs(x, prefijo...) {
			n++
		}
	}
	return n
}

// tocoElMac dice si hubo ssh, scp o gh: lo que solo puede pasar con la guarda armada.
func (c macRunnerScalewayCaso) tocoElMac() bool {
	return c.hay("ssh") || c.hay("scp") || c.hay("gh")
}

var (
	macRunnerScalewayUpdate = []string{"scw", "apple-silicon", "server", "update"}
	macRunnerScalewayWait   = []string{"scw", "apple-silicon", "server", "wait"}
	macRunnerScalewayCreate = []string{"scw", "apple-silicon", "server", "create"}
)

func TestMacRunnerScalewayPocoStockArmaElBorradoAntesDeEsperarYPideElTokenTrasLaEntrega(t *testing.T) {
	c := macRunnerScalewayCorrer(t, "success", false, false)
	if c.res.Codigo != 0 {
		t.Fatalf("código %d\n%s", c.res.Codigo, c.res.Error)
	}
	update := c.indice(func(x []string) bool { return macRunnerScalewayEs(x, macRunnerScalewayUpdate...) })
	wait := c.indice(func(x []string) bool { return macRunnerScalewayEs(x, macRunnerScalewayWait...) })
	github := c.indice(func(x []string) bool { return x[0] == "gh" })
	ssh := c.indice(func(x []string) bool { return x[0] == "ssh" && x[len(x)-1] == "bash -s" })
	if update < 0 || wait < 0 || github < 0 || ssh < 0 {
		t.Fatalf("faltan llamadas (update %d, wait %d, gh %d, ssh %d): %v", update, wait, github, ssh, c.calls)
	}
	if update >= wait || wait >= github || github >= ssh {
		t.Fatalf("orden update %d < wait %d < gh %d < ssh %d roto", update, wait, github, ssh)
	}
	if !slices.Contains(c.calls[update], "schedule-deletion=true") {
		t.Fatalf("el update no arma el borrado: %v", c.calls[update])
	}
	create := c.calls[c.indice(func(x []string) bool { return macRunnerScalewayEs(x, macRunnerScalewayCreate...) })]
	if slices.Contains(create, "--wait") || !slices.Contains(create, "commitment-type=none") || !slices.Contains(create, "os-id=os-stable") {
		t.Fatalf("create %v", create)
	}
}

func TestMacRunnerScalewayTokenTemporalProvistoNoNecesitaSesionDeGitHub(t *testing.T) {
	c := macRunnerScalewayCorrer(t, "success", true, false)
	if c.res.Codigo != 0 {
		t.Fatalf("código %d\n%s", c.res.Codigo, c.res.Error)
	}
	if c.hay("gh") {
		t.Fatal("pidió el token a gh teniendo uno provisto")
	}
}

func TestMacRunnerScalewayPedidoTransitorioReintentaLaGuardaAntesDeCualquierBootstrap(t *testing.T) {
	c := macRunnerScalewayCorrer(t, "ordering_once", false, false)
	if c.res.Codigo != 0 {
		t.Fatalf("código %d\n%s", c.res.Codigo, c.res.Error)
	}
	var updates []int
	for i, x := range c.calls {
		if macRunnerScalewayEs(x, macRunnerScalewayUpdate...) {
			updates = append(updates, i)
		}
	}
	if len(updates) != 2 {
		t.Fatalf("%d updates, no 2", len(updates))
	}
	if wait := c.indice(func(x []string) bool { return macRunnerScalewayEs(x, macRunnerScalewayWait...) }); wait < 0 || updates[1] >= wait {
		t.Fatalf("el último update (%d) no va antes del wait (%d)", updates[1], wait)
	}
}

func TestMacRunnerScalewayPedidoFallidoOAgotadoNuncaHaceBootstrap(t *testing.T) {
	for escenario, intentos := range map[string]int{"ordering_forever": 3, "unexpected_error": 1} {
		t.Run(escenario, func(t *testing.T) {
			c := macRunnerScalewayCorrer(t, escenario, false, false)
			if c.res.Codigo == 0 {
				t.Fatal("salió con 0")
			}
			if n := c.cuantas(macRunnerScalewayUpdate...); n != intentos {
				t.Fatalf("%d updates, no %d", n, intentos)
			}
			if c.tocoElMac() {
				t.Fatalf("llegó al Mac: %v", c.calls)
			}
		})
	}
}

func TestMacRunnerScalewayReanudarComprobaElTipoPropioYNuncaCreaOtroMac(t *testing.T) {
	c := macRunnerScalewayCorrer(t, "resume_existing", false, true)
	if c.res.Codigo != 0 {
		t.Fatalf("código %d\n%s", c.res.Codigo, c.res.Error)
	}
	if c.hay(macRunnerScalewayCreate...) || !c.hay("ssh") {
		t.Fatalf("reanudar creó un Mac o no llegó a él: %v", c.calls)
	}
	c = macRunnerScalewayCorrer(t, "resume_wrong_type", false, true)
	if c.res.Codigo == 0 {
		t.Fatal("reanudó un Mac de otro tipo")
	}
	if c.tocoElMac() || c.hay(macRunnerScalewayUpdate...) {
		t.Fatalf("con otro tipo tocó el Mac o su guarda: %v", c.calls)
	}
}

func TestMacRunnerScalewaySinStockOConServidorExistenteNoCrea(t *testing.T) {
	for _, escenario := range []string{"no_stock", "existing"} {
		t.Run(escenario, func(t *testing.T) {
			c := macRunnerScalewayCorrer(t, escenario, false, false)
			if (c.res.Codigo == 0) != (escenario == "existing") {
				t.Fatalf("código %d", c.res.Codigo)
			}
			if c.hay(macRunnerScalewayCreate...) || c.tocoElMac() {
				t.Fatalf("creó o llegó al Mac: %v", c.calls)
			}
		})
	}
}

func TestMacRunnerScalewayIdentidadAjenaOGuardaDeBorradoFallidaNuncaLleganASSH(t *testing.T) {
	for _, escenario := range []string{"foreign_create", "unarmed", "lost_guard"} {
		t.Run(escenario, func(t *testing.T) {
			c := macRunnerScalewayCorrer(t, escenario, false, false)
			if c.res.Codigo == 0 {
				t.Fatal("salió con 0")
			}
			if c.tocoElMac() {
				t.Fatalf("llegó al Mac: %v", c.calls)
			}
			if escenario == "foreign_create" && c.hay(macRunnerScalewayUpdate...) {
				t.Fatal("tocó la guarda de un servidor que no es el suyo")
			}
		})
	}
}

func TestMacRunnerScalewayElBootstrapConsumeElTokenDeRegistroDeUnUsoSinGh(t *testing.T) {
	fuente := guiontest.Leer(t, "deploy/sytue-git/bin/bootstrap-runner-macos.sh")
	inicio := strings.Index(fuente, "token_de() {")
	if inicio < 0 {
		t.Fatal("bootstrap-runner-macos.sh ya no define token_de")
	}
	fin := strings.Index(fuente[inicio:], "\n}\n")
	if fin < 0 {
		t.Fatal("token_de sin cierre")
	}
	funcion := fuente[inicio : inicio+fin+3]
	r := guiontest.Ejecutar(t, guiontest.Opciones{
		Entrada: funcion + "\ngh() { return 71; }\ntoken_de registration-token\n",
		Env:     []string{"SYTUE_GIT_MACOS_RUNNER_TOKEN=artificial-one-use-registration"},
	}, "bash", "-s")
	if r.Codigo != 0 || strings.TrimSpace(r.Salida) != "artificial-one-use-registration" {
		t.Fatalf("token_de devolvió %q (código %d)\n%s", r.Salida, r.Codigo, r.Error)
	}
	if strings.Contains(fuente, "for c in gh") {
		t.Fatal("el bootstrap vuelve a probar gh para el token")
	}
}
