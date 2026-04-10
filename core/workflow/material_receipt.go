package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const maxDuplicateBarcodeRetries = 5

func (r *MaterialReceiptRunner) Run(ctx context.Context, selection Selection, hooks Hooks) error {
	if r == nil {
		return fmt.Errorf("material receipt runner bo'sh")
	}
	if r.qtyReader == nil {
		return fmt.Errorf("material receipt qty reader bo'sh")
	}
	if r.erp == nil {
		return fmt.Errorf("material receipt erp client bo'sh")
	}
	if r.printRequests == nil {
		return fmt.Errorf("material receipt print request writer bo'sh")
	}
	if r.epcGenerator == nil {
		return fmt.Errorf("material receipt epc generator bo'sh")
	}

	selection = selection.Normalize()
	if selection.ItemCode == "" {
		return fmt.Errorf("material receipt item code bo'sh")
	}
	if selection.Warehouse == "" {
		return fmt.Errorf("material receipt warehouse bo'sh")
	}

	draftCount := 0
	lastSuccess := LastSuccess{}

	for {
		reading, err := r.qtyReader.WaitStablePositiveReading(ctx, r.options.StableReadTimeout, r.options.StableReadPollInterval)
		if isContextError(err) {
			return nil
		}
		if err != nil {
			if isTimeoutError(err) {
				continue
			}
			r.reportProgress(hooks, Progress{
				Selection:   selection,
				DraftCount:  draftCount,
				LastSuccess: lastSuccess,
				Note:        "Scale xato: " + err.Error(),
			})
			continue
		}

		r.logf(
			"batch stable qty: item=%s warehouse=%s qty=%.3f unit=%s scale_at=%s",
			selection.ItemCode,
			selection.Warehouse,
			reading.Qty,
			strings.TrimSpace(reading.Unit),
			reading.UpdatedAt.Format(timeRFC3339Nano),
		)

		epc, draft, err := createDraftWithFreshEPC(
			func() string {
				return r.epcGenerator.Next(reading.UpdatedAt)
			},
			func(epc string) (Draft, error) {
				return r.erp.CreateMaterialReceiptDraft(ctx, CreateMaterialReceiptDraftInput{
					ItemCode:  selection.ItemCode,
					Warehouse: selection.Warehouse,
					Qty:       reading.Qty,
					Barcode:   epc,
				})
			},
			r.isDuplicateBarcodeError,
		)
		if err != nil {
			r.logf("batch draft create error: qty=%.3f epc=%s err=%v", reading.Qty, epc, err)
			r.reportProgress(hooks, Progress{
				Selection:   selection,
				DraftCount:  draftCount,
				LastSuccess: lastSuccess,
				Note:        "ERP xato: " + err.Error(),
			})
			continue
		}

		r.logf("batch draft created: draft=%s qty=%.3f epc=%s", strings.TrimSpace(draft.Name), draft.Qty, epc)
		r.printRequests.SetPrintRequest(epc, draft.Qty, reading.Unit, selection)
		r.reportProgress(hooks, Progress{
			Selection:   selection,
			DraftCount:  draftCount,
			LastSuccess: lastSuccess,
			Note:        "Batch davom etmoqda | Print navbatga qo'yildi",
		})

		printResult, err := r.qtyReader.WaitPrintRequestResult(ctx, r.options.PrintResultTimeout, r.options.PrintResultPollInterval, epc)
		r.printRequests.ClearPrintRequest()
		if isContextError(err) {
			return nil
		}
		if err != nil {
			r.logf("batch print result error: draft=%s epc=%s err=%v", strings.TrimSpace(draft.Name), epc, err)
			deleteErr := r.erp.DeleteStockEntryDraft(ctx, draft.Name)
			note := "Print xato: " + err.Error() + " | Draft delete qilindi"
			if deleteErr != nil {
				note = "Print xato: " + err.Error() + " | Draft delete xato: " + deleteErr.Error()
			}
			r.reportProgress(hooks, Progress{
				Selection:   selection,
				DraftCount:  draftCount,
				LastSuccess: lastSuccess,
				Note:        note,
			})
			if err := r.qtyReader.WaitForNextCycle(ctx, r.options.NextCycleTimeout, r.options.NextCyclePollInterval, reading.Qty); isContextError(err) {
				return nil
			}
			continue
		}

		if strings.ToLower(strings.TrimSpace(printResult.Status)) != "done" {
			r.logf(
				"batch print failed: draft=%s epc=%s status=%s err=%s",
				strings.TrimSpace(draft.Name),
				epc,
				strings.TrimSpace(printResult.Status),
				strings.TrimSpace(printResult.Error),
			)
			deleteErr := r.erp.DeleteStockEntryDraft(ctx, draft.Name)
			note := "Print xato"
			if strings.TrimSpace(printResult.Error) != "" {
				note += ": " + strings.TrimSpace(printResult.Error)
			}
			note += " | Draft delete qilindi"
			if deleteErr != nil {
				note += " | Delete xato: " + deleteErr.Error()
			}
			r.reportProgress(hooks, Progress{
				Selection:   selection,
				DraftCount:  draftCount,
				LastSuccess: lastSuccess,
				Note:        note,
			})
			if err := r.qtyReader.WaitForNextCycle(ctx, r.options.NextCycleTimeout, r.options.NextCyclePollInterval, reading.Qty); isContextError(err) {
				return nil
			}
			continue
		}

		if err := r.erp.SubmitStockEntryDraft(ctx, draft.Name); isContextError(err) {
			return nil
		} else if err != nil {
			r.logf("batch draft submit error: draft=%s epc=%s err=%v", strings.TrimSpace(draft.Name), epc, err)
			r.reportProgress(hooks, Progress{
				Selection:   selection,
				DraftCount:  draftCount,
				LastSuccess: lastSuccess,
				Note:        "Submit xato: " + err.Error(),
			})
			if err := r.qtyReader.WaitForNextCycle(ctx, r.options.NextCycleTimeout, r.options.NextCyclePollInterval, reading.Qty); isContextError(err) {
				return nil
			}
			continue
		}

		if r.history != nil {
			r.history.Add(epc)
		}
		draftCount++
		lastSuccess = LastSuccess{
			DraftName: strings.TrimSpace(draft.Name),
			Qty:       draft.Qty,
			Unit:      reading.Unit,
			EPC:       epc,
			Verify:    "OK",
		}

		for {
			err := r.qtyReader.WaitForNextCycle(ctx, r.options.NextCycleTimeout, r.options.NextCyclePollInterval, draft.Qty)
			if err == nil {
				break
			}
			if isContextError(err) {
				return nil
			}
			r.reportProgress(hooks, Progress{
				Selection:   selection,
				DraftCount:  draftCount,
				LastSuccess: lastSuccess,
				Note:        "Keyingi mahsulotni qo'ying (yoki 0 kg)",
			})
		}
	}
}

func createDraftWithFreshEPC(
	nextEPC func() string,
	create func(string) (Draft, error),
	isDuplicateBarcodeError func(error) bool,
) (string, Draft, error) {
	if nextEPC == nil || create == nil {
		return "", Draft{}, fmt.Errorf("draft create helper dependency bo'sh")
	}

	var lastErr error
	var lastEPC string
	for attempt := 0; attempt < maxDuplicateBarcodeRetries; attempt++ {
		epc := strings.ToUpper(strings.TrimSpace(nextEPC()))
		if epc == "" {
			return "", Draft{}, fmt.Errorf("epc generator bo'sh qiymat qaytardi")
		}
		lastEPC = epc

		draft, err := create(epc)
		if err == nil {
			return epc, draft, nil
		}
		lastErr = err
		if isDuplicateBarcodeError == nil || !isDuplicateBarcodeError(err) {
			return epc, Draft{}, err
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("duplicate retry exhausted")
	}
	return lastEPC, Draft{}, fmt.Errorf(
		"duplicate barcode retry exhausted after %d attempts: %w",
		maxDuplicateBarcodeRetries,
		lastErr,
	)
}

func (r *MaterialReceiptRunner) reportProgress(hooks Hooks, progress Progress) {
	if hooks.Progress == nil {
		return
	}
	progress.Selection = progress.Selection.Normalize()
	progress.Note = strings.TrimSpace(progress.Note)
	hooks.Progress(progress)
}

func (r *MaterialReceiptRunner) logf(format string, args ...any) {
	if r == nil || r.logger == nil {
		return
	}
	r.logger.Printf(format, args...)
}

func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "timeout")
}

const timeRFC3339Nano = "2006-01-02T15:04:05.999999999Z07:00"
