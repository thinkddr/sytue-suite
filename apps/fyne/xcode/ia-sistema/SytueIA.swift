// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// El puente de Sytue Office con Foundation Models, el modelo de Apple Intelligence en el
// dispositivo (iOS y iPadOS 26 en adelante). Foundation Models solo se llama desde Swift:
// esto es un framework pequeño, SytueIA.framework, con tres funciones de C (`@_cdecl`)
// que Go busca con dlopen/dlsym (apps/nucleo/local/asistente/sistema_ios.go). Si la app no
// lleva el framework, o el sistema es anterior a iOS 26, Go lo sabe y lo dice en Ajustes.
//
// Lo construye construir.sh (swiftc, sin proyecto de Xcode) y lo mete en
// `<App>.app/Frameworks/` empaquetar.sh, solo para Sytue Office (SYTUE_ARBOL=cmd-local).
//
// Go llama desde un hilo suyo, que no es de la concurrencia de Swift: esperar aquí con un
// semáforo a la tarea asíncrona no bloquea a nadie más.

import Foundation
#if canImport(FoundationModels)
import FoundationModels
#endif

/// Las tareas en curso, por el identificador que da Go, para poder cancelarlas.
private final class Tareas: @unchecked Sendable {
    private let cerrojo = NSLock()
    private var vivas: [Int64: Task<Void, Never>] = [:]

    func poner(_ id: Int64, _ t: Task<Void, Never>) {
        cerrojo.lock(); vivas[id] = t; cerrojo.unlock()
    }

    func quitar(_ id: Int64) {
        cerrojo.lock(); vivas[id] = nil; cerrojo.unlock()
    }

    func cancelar(_ id: Int64) {
        cerrojo.lock(); let t = vivas[id]; cerrojo.unlock()
        t?.cancel()
    }
}

/// Lo que devuelve la tarea, escrito una vez antes de la señal del semáforo.
private final class Caja: @unchecked Sendable {
    var texto = ""
    var codigo: Int32 = 0
}

private let tareas = Tareas()

/// Si hay modelo: 0 listo, 1 el dispositivo no lo tiene, 2 Apple Intelligence apagada,
/// 3 el modelo se está preparando, 4 otro motivo, 5 sistema anterior a iOS 26.
@_cdecl("sytue_ia_estado")
public func sytue_ia_estado() -> Int32 {
    #if canImport(FoundationModels)
    if #available(iOS 26.0, macOS 26.0, *) {
        let disponibilidad = SystemLanguageModel.default.availability
        if case .available = disponibilidad {
            return 0
        }
        if case .unavailable(let motivo) = disponibilidad {
            switch motivo {
            case .deviceNotEligible: return 1
            case .appleIntelligenceNotEnabled: return 2
            case .modelNotReady: return 3
            @unknown default: return 4
            }
        }
        return 4
    }
    #endif
    return 5
}

/// Pide una respuesta con `instrucciones` como instrucciones de la sesión (el prompt de
/// sistema de la suite) y `texto` como mensaje. Devuelve un texto reservado con malloc,
/// que Go libera con free: la respuesta, o el error si `fallo` vale distinto de 0
/// (1 error del modelo, 2 cancelada, 3 sin Foundation Models).
@_cdecl("sytue_ia_responder")
public func sytue_ia_responder(_ instrucciones: UnsafePointer<CChar>, _ texto: UnsafePointer<CChar>,
                               _ id: Int64, _ fallo: UnsafeMutablePointer<Int32>) -> UnsafeMutablePointer<CChar>? {
    let i = String(cString: instrucciones)
    let t = String(cString: texto)
    #if canImport(FoundationModels)
    if #available(iOS 26.0, macOS 26.0, *) {
        let caja = Caja()
        let listo = DispatchSemaphore(value: 0)
        let tarea = Task.detached(priority: .userInitiated) {
            do {
                let sesion = LanguageModelSession(instructions: i)
                let r = try await sesion.respond(to: t)
                caja.texto = r.content
            } catch is CancellationError {
                caja.codigo = 2
                caja.texto = "cancelada"
            } catch {
                caja.codigo = 1
                caja.texto = String(describing: error)
            }
            listo.signal()
        }
        tareas.poner(id, tarea)
        listo.wait()
        tareas.quitar(id)
        fallo.pointee = caja.codigo
        return strdup(caja.texto)
    }
    #endif
    fallo.pointee = 3
    return strdup("Foundation Models no está en este sistema")
}

/// Cancela la respuesta `id` si aún no ha llegado (Go dejó de esperar).
@_cdecl("sytue_ia_cancelar")
public func sytue_ia_cancelar(_ id: Int64) {
    tareas.cancelar(id)
}
