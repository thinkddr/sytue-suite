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
