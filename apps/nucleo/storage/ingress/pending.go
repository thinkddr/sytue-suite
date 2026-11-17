// SPDX-License-Identifier: Apache-2.0
package ingress

import "git.sytue.eu/sytue/one/apps/nucleo/storage"

const MaxPendingTasks = 200

type PendingPage struct {
	Version int           `json:"v"`
	Scope   storage.Scope `json:"scope"`
	Epoch   string        `json:"epoch"`
	Tasks   []Task        `json:"tasks"`
	Next    string        `json:"next,omitempty"`
}

func (p PendingPage) Validate(scope storage.Scope, epoch, after string) error {
	if p.Version != 1 || p.Scope != scope || !scope.Valid() || p.Epoch != epoch || !storage.MigrationEpoch(epoch) || len(p.Tasks) > MaxPendingTasks || (after != "" && !storage.Identifier(after)) {
		return ErrContract
	}
	previous := after
	for _, task := range p.Tasks {
		if task.Validate(scope, epoch) != nil || task.ID <= previous {
			return ErrContract
		}
		previous = task.ID
	}
	if p.Next != "" && (len(p.Tasks) == 0 || p.Next != previous) {
		return ErrContract
	}
	return nil
}
