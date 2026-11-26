// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package com llama a interfaces COM de Windows en Go puro, sin cgo: por la tabla de
// métodos del objeto y `syscall.SyscallN`, como hacen ya los `_windows.go` de
// `nucleo/sistema/escritorio` con las funciones sueltas de las DLL.
//
// Lo usan los descodificadores y miniaturas del sistema de Sytue Office (Media
// Foundation en `apps/fyne/video/sistema/mediafoundation`, la miniatura del
// Explorador en `local/videoteca`). Se compila en cruzado sin el SDK de Windows
// (`GOOS=windows CGO_ENABLED=0`) y es defensivo: lo que falta (una DLL, una función,
// una interfaz) es un error, nunca un pánico que tumbe la app.
//
// Fuera de Windows el paquete está vacío.
package com
