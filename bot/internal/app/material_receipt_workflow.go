package app

import (
	"context"
	"time"

	"bot/internal/bridgeclient"
	"bot/internal/erp"
	"core/workflow"
)

func (a *App) newMaterialReceiptRunner() *workflow.MaterialReceiptRunner {
	return workflow.NewMaterialReceiptRunner(workflow.MaterialReceiptDependencies{
		QtyReader:               workflowQtyReader{client: a.qtyReader},
		ERP:                     workflowERPClient{client: a.erp},
		PrintRequests:           workflowPrintRequestWriter{app: a},
		EPCGenerator:            a.epcGenerator,
		History:                 a.epcHistory,
		Logger:                  a.logBatch,
		IsDuplicateBarcodeError: erp.IsDuplicateBarcodeError,
	})
}

func toWorkflowSelection(sel SelectedContext) workflow.Selection {
	return workflow.Selection{
		ItemCode:  sel.ItemCode,
		ItemName:  sel.ItemName,
		Warehouse: sel.Warehouse,
	}
}

func fromWorkflowSelection(sel workflow.Selection) SelectedContext {
	return SelectedContext{
		ItemCode:  sel.ItemCode,
		ItemName:  sel.ItemName,
		Warehouse: sel.Warehouse,
	}
}

func formatBatchWorkflowProgress(progress workflow.Progress) string {
	selection := fromWorkflowSelection(progress.Selection)
	return formatBatchStatusText(
		selection,
		progress.DraftCount,
		progress.LastSuccess.DraftName,
		progress.LastSuccess.Qty,
		progress.LastSuccess.Unit,
		progress.LastSuccess.EPC,
		progress.LastSuccess.Verify,
		progress.Note,
	)
}

type workflowQtyReader struct {
	client *bridgeclient.Client
}

func (r workflowQtyReader) WaitStablePositiveReading(ctx context.Context, timeout, pollInterval time.Duration) (workflow.StableReading, error) {
	reading, err := r.client.WaitStablePositiveReading(ctx, timeout, pollInterval)
	if err != nil {
		return workflow.StableReading{}, err
	}
	return workflow.StableReading{
		Qty:       reading.Qty,
		Unit:      reading.Unit,
		UpdatedAt: reading.UpdatedAt,
	}, nil
}

func (r workflowQtyReader) WaitPrintRequestResult(ctx context.Context, timeout, pollInterval time.Duration, epc string) (workflow.PrintRequestResult, error) {
	result, err := r.client.WaitPrintRequestResult(ctx, timeout, pollInterval, epc)
	if err != nil {
		return workflow.PrintRequestResult{}, err
	}
	return workflow.PrintRequestResult{
		EPC:       result.EPC,
		Status:    result.Status,
		Error:     result.Error,
		UpdatedAt: result.UpdatedAt,
	}, nil
}

func (r workflowQtyReader) WaitForNextCycle(ctx context.Context, timeout, pollInterval time.Duration, lastQty float64) error {
	return r.client.WaitForNextCycle(ctx, timeout, pollInterval, lastQty)
}

type workflowERPClient struct {
	client *erp.Client
}

func (c workflowERPClient) CreateMaterialReceiptDraft(ctx context.Context, in workflow.CreateMaterialReceiptDraftInput) (workflow.Draft, error) {
	draft, err := c.client.CreateMaterialReceiptDraft(ctx, erp.MaterialReceiptDraftInput{
		ItemCode:  in.ItemCode,
		Warehouse: in.Warehouse,
		Qty:       in.Qty,
		Barcode:   in.Barcode,
	})
	if err != nil {
		return workflow.Draft{}, err
	}
	return workflow.Draft{
		Name:      draft.Name,
		ItemCode:  draft.ItemCode,
		Warehouse: draft.Warehouse,
		Qty:       draft.Qty,
		UOM:       draft.UOM,
		Barcode:   draft.Barcode,
	}, nil
}

func (c workflowERPClient) SubmitStockEntryDraft(ctx context.Context, name string) error {
	return c.client.SubmitStockEntryDraft(ctx, name)
}

func (c workflowERPClient) DeleteStockEntryDraft(ctx context.Context, name string) error {
	return c.client.DeleteStockEntryDraft(ctx, name)
}

type workflowPrintRequestWriter struct {
	app *App
}

func (w workflowPrintRequestWriter) SetPrintRequest(epc string, qty float64, unit string, selection workflow.Selection) {
	if w.app == nil {
		return
	}
	w.app.setPrintRequest(epc, qty, unit, fromWorkflowSelection(selection))
}

func (w workflowPrintRequestWriter) ClearPrintRequest() {
	if w.app == nil {
		return
	}
	w.app.clearPrintRequest()
}
