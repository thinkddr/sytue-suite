// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

import Foundation
import WidgetKit

// This is our own C entry point, called from apps/nucleo/widgets. The framework
// uses WidgetKit's public Swift API and has no access to credentials or snapshots.
@_cdecl("sytue_widgets_reload")
public func sytueWidgetsReload() {
    DispatchQueue.main.async {
        WidgetCenter.shared.reloadAllTimelines()
    }
}

// How many of this app's widgets are placed (WidgetKit's current configurations), or -1
// when WidgetKit does not answer within five seconds. Go calls it off the main thread:
// the despertar of apps/fyne/sesion only registers with the Hub while one is placed.
private final class Cuenta: @unchecked Sendable {
    var valor: Int32 = -1
}

@_cdecl("sytue_widgets_instalados")
public func sytueWidgetsInstalados() -> Int32 {
    let cuenta = Cuenta()
    let listo = DispatchSemaphore(value: 0)
    WidgetCenter.shared.getCurrentConfigurations { resultado in
        if case .success(let widgets) = resultado {
            cuenta.valor = Int32(widgets.count)
        }
        listo.signal()
    }
    guard listo.wait(timeout: .now() + 5) == .success else { return -1 }
    return cuenta.valor
}
