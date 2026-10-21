// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin && !ios

package escritorio

/*
#cgo LDFLAGS: -framework Carbon
#include <Carbon/Carbon.h>

extern void sytueAtajoPulsado(void);

static OSStatus sytueManejadorAtajo(EventHandlerCallRef siguiente, EventRef evento, void *datos) {
	sytueAtajoPulsado();
	return noErr;
}

// RegisterEventHotKey es de Carbon y sigue siendo LA forma de un atajo global sin pedir
// permiso de accesibilidad. Va en el hilo principal, que es donde vive el bucle de
// eventos de la aplicación.
static void sytueRegistrarAtajo(UInt32 modificadores) {
	dispatch_async(dispatch_get_main_queue(), ^{
		EventTypeSpec tipo = {kEventClassKeyboard, kEventHotKeyPressed};
		InstallApplicationEventHandler(&sytueManejadorAtajo, 1, &tipo, NULL, NULL);
		EventHotKeyID id = {'sytu', 1};
		EventHotKeyRef ref;
		RegisterEventHotKey(kVK_Space, modificadores, id, GetApplicationEventTarget(), 0, &ref);
	});
}

static UInt32 sytueModificadores(int ctrl, int alt, int cmd) {
	return (ctrl ? controlKey : 0) | (alt ? optionKey : 0) | (cmd ? cmdKey : 0);
}
*/
import "C"

import "sync"

var (
	atajoMu       sync.Mutex
	alPulsarAtajo func()
)

// ponytail: un solo atajo por proceso y sin soltarlo —la función de soltar solo olvida
// qué hacer—, que es lo que la suite necesita: lo registra el Hub una vez al arrancar.
// Si el registro falla (otra app ya tiene la combinación) no se sabe desde aquí:
// RegisterEventHotKey corre después, en el hilo principal.
func atajo(_, _ string, c Combinacion, alPulsar func()) (func(), error) {
	atajoMu.Lock()
	primero := alPulsarAtajo == nil
	alPulsarAtajo = alPulsar
	atajoMu.Unlock()
	if primero {
		C.sytueRegistrarAtajo(C.sytueModificadores(bool2int(c.Ctrl), bool2int(c.Alt), bool2int(c.Cmd)))
	}
	return func() {
		atajoMu.Lock()
		alPulsarAtajo = func() {}
		atajoMu.Unlock()
	}, nil
}

func bool2int(b bool) C.int {
	if b {
		return 1
	}
	return 0
}

func pulsarAtajo() {
	atajoMu.Lock()
	f := alPulsarAtajo
	atajoMu.Unlock()
	if f != nil {
		go f()
	}
}
