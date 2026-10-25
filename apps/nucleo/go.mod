// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// El NÚCLEO de los clientes: lo que las apps nativas necesitan y NO depende del
// toolkit de interfaz. Cliente HTTP/WS de los servicios, catálogo de idiomas, sesión
// OIDC + llavero, capacidades del sistema operativo, diálogos de fichero, motor de
// fórmulas y HTML a texto plano.
//
// Vive en su propio módulo, y no dentro del `internal/` de la suite en Gio (retirada el
// 2026-09-23), porque desde allí **no se podía importar**: `internal/` lo cierra a su
// subárbol, y lo que hay aquí son ~19.000 líneas que usaron dos toolkits distintos. El
// día que se copien dejan de ser las mismas (docs/CONVENTIONS.md §13.2).
//
// El nombre NO lleva el toolkit a propósito: en un mes se ha cambiado dos veces (Fyne →
// Gio → Fyne) y nombrar la plomería por quien la pinta es parte de por qué duele.
//
// Módulo SEPARADO del árbol raíz por lo mismo que apps/fyne: el llavero y
// D-Bus son dependencias de cliente de escritorio y no deben entrar en el go.mod de la
// plataforma ni en su gate de osv/trivy.
module git.sytue.eu/sytue/one/apps/nucleo

go 1.27.3

require (
	git.wow.st/gmp/jni v0.0.0-20210610011705-34026c7e22d0
	github.com/coder/websocket v1.8.15
	github.com/godbus/dbus/v5 v5.2.4
	github.com/mattn/go-sqlite3 v1.14.52
	github.com/rs/zerolog v1.35.1
	github.com/zalando/go-keyring v0.2.8
	golang.org/x/image v0.47.0
	golang.org/x/sys v0.49.0
	golang.org/x/text v0.43.0
	maunium.net/go/mautrix v0.32.0
)

require (
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/petermattis/goid v0.0.0-20260918085751-abfca077860b // indirect
	github.com/tidwall/gjson v1.20.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	go.mau.fi/util v0.11.0 // indirect
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/exp v0.0.0-20261014143641-f0251eb4aa3e // indirect
	golang.org/x/net v0.61.0 // indirect
)
