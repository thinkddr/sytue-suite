// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build (darwin || ios) && cgo

package widgets

/*
#include <dlfcn.h>

// The packaged host loads our signed Swift framework. Development binaries that
// do not embed it can still use file snapshots without loading WidgetKit.
static void sytueWidgetReload(void) {
    void (*reload)(void) = (void (*)(void))dlsym(RTLD_DEFAULT, "sytue_widgets_reload");
    if (reload != NULL) reload();
}

// The bridge is linked only into hosts that embed the WidgetKit extension.
static int sytueWidgetBridge(void) {
    return dlsym(RTLD_DEFAULT, "sytue_widgets_reload") != NULL;
}
*/
import "C"

import "sync"

func init() {
	// Con el puente cargado hay una extensión WidgetKit dentro del paquete que lee los
	// snapshots; sin él (widgets de Apple apagados, o un binario de desarrollo) no se
	// escribe nada. Se mira una vez: el ejecutable no cambia de bibliotecas en marcha.
	lector = sync.OnceValue(func() bool { return C.sytueWidgetBridge() != 0 })
	notifyPlatform = func(string) { C.sytueWidgetReload() }
}
