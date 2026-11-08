// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build (darwin || ios) && cgo

package widgets

/*
#include <dlfcn.h>
#include <TargetConditionals.h>

// SYTUE_PUENTE_DLOPEN: on iOS the host comes from the official `fyne package --target
// ios`, which links no extra framework, so the signed bridge embedded in the bundle is
// opened here. macOS, the Simulator and the Xcode build phase still link it, and then
// the first dlsym already finds it. Development binaries without it keep file snapshots.
static void *sytueWidgetSymbol(void) {
    void *symbol = dlsym(RTLD_DEFAULT, "sytue_widgets_reload");
#if TARGET_OS_IPHONE
    if (symbol == NULL &&
        dlopen("@executable_path/Frameworks/SytueWidgetBridge.framework/SytueWidgetBridge", RTLD_NOW | RTLD_GLOBAL) != NULL) {
        symbol = dlsym(RTLD_DEFAULT, "sytue_widgets_reload");
    }
#endif
    return symbol;
}

static void sytueWidgetReload(void) {
    void (*reload)(void) = (void (*)(void))sytueWidgetSymbol();
    if (reload != NULL) reload();
}

// The bridge is only embedded in hosts that ship the WidgetKit extension.
static int sytueWidgetBridge(void) {
    return sytueWidgetSymbol() != NULL;
}

// How many of this app's widgets are placed, or -1 when WidgetKit does not say (or the
// bridge predates the function). Found after the reload symbol, which opens the bridge.
static int sytueWidgetsInstalled(void) {
    if (sytueWidgetSymbol() == NULL) return -1;
    int (*installed)(void) = (int (*)(void))dlsym(RTLD_DEFAULT, "sytue_widgets_instalados");
    return installed != NULL ? installed() : -1;
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
	instaladosPlataforma = func() (int, bool) {
		n := int(C.sytueWidgetsInstalled())
		return max(n, 0), n >= 0
	}
}
