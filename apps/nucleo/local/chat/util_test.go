// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chat

import "git.sytue.eu/sytue/one/apps/nucleo/api"

func apiSend(ids ...string) api.RoomsSend { return api.RoomsSend{FileIDs: ids} }
