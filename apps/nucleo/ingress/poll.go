// SPDX-License-Identifier: Apache-2.0
package ingress

import (
	"context"
	"errors"
	"slices"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	contract "git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
)

type pendingSource interface {
	Pending(context.Context, string, int) (contract.PendingPage, error)
}

// PollResult contains counts only. Domain contents and private metadata remain
// inside the executor; a successful task is counted only after its verified ACK.
type PollResult struct{ Attempted, Completed int }

// Poll processes one bounded authenticated page. A blocked original assignment
// does not starve later pages, and its original encrypted intent stays retained.
func (w *Worker) Poll(parent context.Context) (PollResult, error) {
	var result PollResult
	ctx, done, err := w.gate.Start(parent)
	if err != nil {
		return result, err
	}
	defer done()
	w.pollMu.Lock()
	defer w.pollMu.Unlock()
	if err := ctx.Err(); err != nil {
		return result, err
	}
	source, ok := w.cfg.Source.(pendingSource)
	if !ok {
		return result, contract.ErrContract
	}
	retained, next, err := w.retainedPage()
	if err != nil {
		return result, err
	}
	attempted := map[string]bool{}
	var failures []error
	for _, id := range retained {
		err := w.pollTask(ctx, id, &result, attempted)
		if err != nil {
			failures = append(failures, err)
			if errors.Is(err, contract.ErrRevoked) || ctx.Err() != nil {
				return result, errors.Join(failures...)
			}
		}
	}
	w.retryAfter = next
	page, err := source.Pending(ctx, w.pollAfter, contract.MaxPendingTasks)
	if err != nil {
		return result, errors.Join(append(failures, err)...)
	}
	if page.Validate(w.cfg.Scope, w.cfg.Epoch, w.pollAfter) != nil {
		return result, errors.Join(append(failures, contract.ErrContract)...)
	}
	for _, task := range page.Tasks {
		if attempted[task.ID] {
			continue
		}
		err := w.pollTask(ctx, task.ID, &result, attempted)
		if err != nil {
			failures = append(failures, err)
			if errors.Is(err, contract.ErrRevoked) || ctx.Err() != nil {
				return result, errors.Join(failures...)
			}
		}
	}
	w.pollAfter = page.Next
	return result, errors.Join(failures...)
}

func (w *Worker) pollTask(ctx context.Context, id string, result *PollResult, attempted map[string]bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	attempted[id] = true
	result.Attempted++
	err := w.processAdmitted(ctx, id)
	if err == nil {
		result.Completed++
	}
	return err
}

// An accepted ACK can disappear from remote Pending before its reply reaches
// this device. Retry the retained original WAL independently of discovery.
func (w *Worker) retainedPage() ([]string, string, error) {
	ids, err := w.cfg.Journal.RecordIDs(w.owner(), storage.PrivateIngressNamespace)
	if err != nil {
		return nil, "", err
	}
	slices.Sort(ids)
	start, _ := slices.BinarySearch(ids, w.retryAfter)
	for start < len(ids) && ids[start] <= w.retryAfter {
		start++
	}
	if start == len(ids) {
		return nil, "", nil
	}
	ids = ids[start:]
	if len(ids) <= contract.MaxPendingTasks {
		return ids, "", nil
	}
	page := ids[:contract.MaxPendingTasks]
	return page, page[len(page)-1], nil
}

// Run resumes private receipt work while the owning application is active.
// The caller owns this lifetime and cancels it when backgrounded or detached.
// Every page reacquires and validates its persistent session witness via Poll.
func (w *Worker) Run(ctx context.Context, changed func(PollResult, error)) error {
	for {
		result, err := w.Poll(ctx)
		if failure := ctx.Err(); failure != nil {
			return failure
		}
		if changed != nil && (result.Attempted > 0 || err != nil) {
			changed(result, err)
		}
		if errors.Is(err, offline.ErrWorkClosed) || errors.Is(err, offline.ErrWorkPaused) || errors.Is(err, contract.ErrRevoked) {
			return err
		}
		timer := time.NewTimer(30 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
