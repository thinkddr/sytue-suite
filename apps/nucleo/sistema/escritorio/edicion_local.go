// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build edicionlocal

package escritorio

// EdicionLocal dice que este binario es de Sytue Office, la edición local: se compila con
// la etiqueta `edicionlocal` (`empaquetar.sh` con `SYTUE_ARBOL=cmd-local`, y el CI). La
// cáscara de la edición (`apps/fyne/local.Abrir`) no arranca sin ella (D-02 del plan,
// apps/fyne/PLAN-EDICION-LOCAL.md §5.3): sin la etiqueta, `ui` enlazaría la sesión de la
// suite y su login, y las llamadas sus medios.
const EdicionLocal = true
