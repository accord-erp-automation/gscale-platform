package app

func (a *App) setPrintRequest(epc string, qty float64, unit string, sel SelectedContext) {
	if a == nil || a.batchState == nil {
		return
	}
	if err := a.batchState.SetPrintRequest(epc, qty, unit, sel.ItemCode, sel.ItemName); err != nil {
		a.logBatch.Printf("print request write error: %v", err)
	}
}

func (a *App) clearPrintRequest() {
	if a == nil || a.batchState == nil {
		return
	}
	if err := a.batchState.ClearPrintRequest(); err != nil {
		a.logBatch.Printf("print request clear error: %v", err)
	}
}
