// Los ORIGINALES de los forks y parches de apps/fyne/go.mod, en la versión de la que parte
// cada uno. No se compila: es un lockfile para osv-scanner (security.yml). govulncheck y
// osv-scanner consultan la base de avisos por la ruta de REEMPLAZO
// (github.com/thinkddr/…), que no tiene avisos nunca; aquí van las rutas de verdad, y
// un aviso contra Fyne, GLFW o typesetting pone `security` en rojo (FYP-04).
// empaquetar-test.sh comprueba que cada `replace … => github.com/thinkddr/` tenga aquí
// su original y en la misma versión que pide apps/fyne/go.mod.
// No ejecutar go mod tidy: al no haber fuentes, borraría este inventario deliberado.
module git.sytue.eu/sytue/one/apps/fyne/upstream

go 1.27.3

require (
	fyne.io/fyne/v2 v2.8.1
	github.com/go-gl/glfw/v3.4/glfw v0.1.0-pre.1.0.20260707082822-2a407d02d01a
	github.com/go-text/typesetting v0.3.4
	github.com/livekit/server-sdk-go/v2 v2.19.0
)
