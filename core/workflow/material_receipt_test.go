package workflow

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestCreateDraftWithFreshEPCRetriesDuplicateAndSucceeds(t *testing.T) {
	t.Parallel()

	epcs := []string{"EPC-1", "EPC-2"}
	nextIdx := 0

	epc, draft, err := createDraftWithFreshEPC(
		func() string {
			v := epcs[nextIdx]
			nextIdx++
			return v
		},
		func(epc string) (Draft, error) {
			if epc == "EPC-1" {
				return Draft{}, fmt.Errorf("erp stock entry http 417: barcode already exists")
			}
			return Draft{Name: "MAT-STE-1", Barcode: epc}, nil
		},
		func(err error) bool {
			return strings.Contains(strings.ToLower(err.Error()), "barcode already exists")
		},
	)
	if err != nil {
		t.Fatalf("createDraftWithFreshEPC error: %v", err)
	}
	if epc != "EPC-2" {
		t.Fatalf("epc mismatch: %q", epc)
	}
	if draft.Barcode != "EPC-2" {
		t.Fatalf("draft barcode mismatch: %q", draft.Barcode)
	}
}

func TestCreateDraftWithFreshEPCReturnsNonDuplicateImmediately(t *testing.T) {
	t.Parallel()

	_, _, err := createDraftWithFreshEPC(
		func() string { return "EPC-1" },
		func(epc string) (Draft, error) {
			return Draft{}, fmt.Errorf("erp stock entry http 500: unexpected failure")
		},
		func(err error) bool {
			return strings.Contains(strings.ToLower(err.Error()), "duplicate")
		},
	)
	if err == nil || err.Error() != "erp stock entry http 500: unexpected failure" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMaterialReceiptRunnerRunSuccess(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reader := &stubQtyReader{
		stableReadings: []stableReadingResult{
			{
				reading: StableReading{
					Qty:       5.25,
					Unit:      "kg",
					UpdatedAt: time.Date(2026, 4, 10, 9, 0, 0, 0, time.UTC),
				},
			},
		},
		printResults: []printRequestResultResult{
			{
				result: PrintRequestResult{
					EPC:    "EPC-2",
					Status: "done",
				},
			},
		},
		nextCycleErrs: []error{
			context.Canceled,
		},
		onNextCycle: func() {
			cancel()
		},
	}
	erpClient := &stubERP{
		createDraftErrs: []error{
			fmt.Errorf("erp stock entry http 417: barcode already exists"),
			nil,
		},
		drafts: []Draft{
			{Name: "ignored", Qty: 5.25, Barcode: "EPC-1"},
			{Name: "MAT-STE-0001", Qty: 5.25, Barcode: "EPC-2"},
		},
	}
	printWriter := &stubPrintRequestWriter{}
	history := &stubHistory{}
	progresses := make([]Progress, 0, 2)

	runner := NewMaterialReceiptRunner(MaterialReceiptDependencies{
		QtyReader:     reader,
		ERP:           erpClient,
		PrintRequests: printWriter,
		EPCGenerator:  &stubGenerator{epcs: []string{"EPC-1", "EPC-2"}},
		History:       history,
		IsDuplicateBarcodeError: func(err error) bool {
			return strings.Contains(strings.ToLower(err.Error()), "barcode already exists")
		},
	})

	err := runner.Run(ctx, Selection{ItemCode: "ITEM-1", ItemName: "Tea", Warehouse: "Stores - A"}, Hooks{
		Progress: func(progress Progress) {
			progresses = append(progresses, progress)
		},
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	if len(erpClient.createInputs) != 2 {
		t.Fatalf("create attempts = %d, want 2", len(erpClient.createInputs))
	}
	if got := erpClient.createInputs[1].Barcode; got != "EPC-2" {
		t.Fatalf("final barcode = %q", got)
	}
	if !slices.Equal(erpClient.submitted, []string{"MAT-STE-0001"}) {
		t.Fatalf("submitted = %#v", erpClient.submitted)
	}
	if len(erpClient.deleted) != 0 {
		t.Fatalf("deleted drafts = %#v", erpClient.deleted)
	}
	if len(printWriter.setCalls) != 1 {
		t.Fatalf("setCalls = %d", len(printWriter.setCalls))
	}
	if printWriter.setCalls[0].epc != "EPC-2" {
		t.Fatalf("setCall epc = %q", printWriter.setCalls[0].epc)
	}
	if printWriter.clearCalls != 1 {
		t.Fatalf("clearCalls = %d", printWriter.clearCalls)
	}
	if !slices.Equal(history.items, []string{"EPC-2"}) {
		t.Fatalf("history = %#v", history.items)
	}
	if len(progresses) != 2 {
		t.Fatalf("progresses = %#v", progresses)
	}
	if progresses[0].TotalQty != 0 {
		t.Fatalf("first progress total = %.3f", progresses[0].TotalQty)
	}
	if progresses[1].TotalQty != 5.25 {
		t.Fatalf("final progress total = %.3f", progresses[1].TotalQty)
	}
	if !strings.Contains(progresses[1].Note, "Jami 5.250 kg") {
		t.Fatalf("final progress note = %q", progresses[1].Note)
	}
}

func TestMaterialReceiptRunnerRunWithTareUsesNetQtyAndPrintGrossQty(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reader := &stubQtyReader{
		stableReadings: []stableReadingResult{
			{
				reading: StableReading{
					Qty:       5,
					Unit:      "kg",
					UpdatedAt: time.Date(2026, 4, 10, 9, 0, 0, 0, time.UTC),
				},
			},
		},
		printResults: []printRequestResultResult{
			{
				result: PrintRequestResult{
					EPC:    "EPC-TARE",
					Status: "done",
				},
			},
		},
		nextCycleErrs: []error{context.Canceled},
		onNextCycle: func() {
			cancel()
		},
	}
	erpClient := &stubERP{}
	printWriter := &stubPrintRequestWriter{}

	runner := NewMaterialReceiptRunner(MaterialReceiptDependencies{
		QtyReader:     reader,
		ERP:           erpClient,
		PrintRequests: printWriter,
		EPCGenerator:  &stubGenerator{epcs: []string{"EPC-TARE"}},
		History:       &stubHistory{},
	})

	err := runner.Run(ctx, Selection{
		ItemCode:    "ITEM-1",
		ItemName:    "Tea",
		Warehouse:   "Stores - A",
		TareEnabled: true,
		TareKG:      0.78,
	}, Hooks{})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(erpClient.createInputs) != 1 {
		t.Fatalf("create inputs = %d", len(erpClient.createInputs))
	}
	if got := erpClient.createInputs[0].Qty; fmt.Sprintf("%.2f", got) != "4.22" {
		t.Fatalf("erp qty = %.3f, want 4.220", got)
	}
	if len(printWriter.setCalls) != 1 {
		t.Fatalf("setCalls = %d", len(printWriter.setCalls))
	}
	if got := printWriter.setCalls[0].qty; fmt.Sprintf("%.2f", got) != "4.22" {
		t.Fatalf("print net qty = %.3f, want 4.220", got)
	}
	if got := printWriter.setCalls[0].grossQty; got != 5 {
		t.Fatalf("print gross qty = %.3f, want 5.000", got)
	}
	if !printWriter.setCalls[0].selection.TareEnabled || printWriter.setCalls[0].selection.TareKG != 0.78 {
		t.Fatalf("print selection tare = %+v", printWriter.setCalls[0].selection)
	}
}

func TestMaterialReceiptRunnerManualPrintOnceUsesProvidedQty(t *testing.T) {
	t.Parallel()

	reader := &stubQtyReader{
		printResults: []printRequestResultResult{
			{
				result: PrintRequestResult{
					EPC:    "EPC-MANUAL",
					Status: "done",
				},
			},
		},
	}
	erpClient := &stubERP{}
	printWriter := &stubPrintRequestWriter{}
	history := &stubHistory{}

	runner := NewMaterialReceiptRunner(MaterialReceiptDependencies{
		QtyReader:     reader,
		ERP:           erpClient,
		PrintRequests: printWriter,
		EPCGenerator:  &stubGenerator{epcs: []string{"EPC-MANUAL"}},
		History:       history,
	})

	draft, epc, err := runner.PrintOnce(context.Background(), Selection{
		ItemCode:       "ITEM-1",
		ItemName:       "Tea",
		Warehouse:      "Stores - A",
		QuantitySource: QuantitySourceManual,
		TareEnabled:    true,
		TareKG:         0.78,
	}, 5)
	if err != nil {
		t.Fatalf("PrintOnce error: %v", err)
	}
	if epc != "EPC-MANUAL" {
		t.Fatalf("epc = %q", epc)
	}
	if draft.Qty == 0 {
		t.Fatal("draft qty is zero")
	}
	if reader.stableIdx != 0 {
		t.Fatalf("PrintOnce should not read scale, stable reads = %d", reader.stableIdx)
	}
	if len(erpClient.createInputs) != 1 {
		t.Fatalf("create inputs = %d", len(erpClient.createInputs))
	}
	if got := erpClient.createInputs[0].Qty; fmt.Sprintf("%.2f", got) != "4.22" {
		t.Fatalf("erp qty = %.3f, want 4.220", got)
	}
	if len(printWriter.setCalls) != 1 {
		t.Fatalf("setCalls = %d", len(printWriter.setCalls))
	}
	if got := printWriter.setCalls[0].grossQty; got != 5 {
		t.Fatalf("print gross qty = %.3f, want 5.000", got)
	}
	if !slices.Equal(history.items, []string{"EPC-MANUAL"}) {
		t.Fatalf("history = %#v", history.items)
	}
}

func TestMaterialReceiptRunnerManualRunWaitsForStop(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reader := &stubQtyReader{}
	erpClient := &stubERP{}
	printWriter := &stubPrintRequestWriter{}
	runner := NewMaterialReceiptRunner(MaterialReceiptDependencies{
		QtyReader:     reader,
		ERP:           erpClient,
		PrintRequests: printWriter,
		EPCGenerator:  &stubGenerator{epcs: []string{"EPC-MANUAL"}},
		History:       &stubHistory{},
	})

	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx, Selection{
			ItemCode:       "ITEM-1",
			ItemName:       "Tea",
			Warehouse:      "Stores - A",
			QuantitySource: QuantitySourceManual,
		}, Hooks{})
	}()

	time.Sleep(50 * time.Millisecond)
	if reader.stableIdx != 0 {
		t.Fatalf("manual run should not read scale, stable reads = %d", reader.stableIdx)
	}
	if len(erpClient.createInputs) != 0 {
		t.Fatalf("manual run should not create draft, got %d", len(erpClient.createInputs))
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("manual run did not stop after cancel")
	}
}

func TestMaterialReceiptRunnerDeletesDraftOnPrintFailure(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reader := &stubQtyReader{
		stableReadings: []stableReadingResult{
			{
				reading: StableReading{
					Qty:       2.1,
					Unit:      "kg",
					UpdatedAt: time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC),
				},
			},
		},
		printResults: []printRequestResultResult{
			{
				result: PrintRequestResult{
					EPC:    "EPC-1",
					Status: "error",
					Error:  "zebra disabled",
				},
			},
		},
		nextCycleErrs: []error{
			context.Canceled,
		},
		onNextCycle: func() {
			cancel()
		},
	}
	erpClient := &stubERP{
		drafts: []Draft{
			{Name: "MAT-STE-0002", Qty: 2.1, Barcode: "EPC-1"},
		},
	}
	printWriter := &stubPrintRequestWriter{}
	progresses := make([]Progress, 0, 3)

	runner := NewMaterialReceiptRunner(MaterialReceiptDependencies{
		QtyReader:     reader,
		ERP:           erpClient,
		PrintRequests: printWriter,
		EPCGenerator:  &stubGenerator{epcs: []string{"EPC-1"}},
	})

	err := runner.Run(ctx, Selection{ItemCode: "ITEM-2", Warehouse: "Stores - B"}, Hooks{
		Progress: func(progress Progress) {
			progresses = append(progresses, progress)
		},
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	if !slices.Equal(erpClient.deleted, []string{"MAT-STE-0002"}) {
		t.Fatalf("deleted = %#v", erpClient.deleted)
	}
	if len(erpClient.submitted) != 0 {
		t.Fatalf("submitted = %#v", erpClient.submitted)
	}
	if printWriter.clearCalls != 1 {
		t.Fatalf("clearCalls = %d", printWriter.clearCalls)
	}
	if len(progresses) != 2 {
		t.Fatalf("progresses = %#v", progresses)
	}
	if progresses[1].Note != "Print xato: zebra disabled | Draft delete qilindi" {
		t.Fatalf("failure note = %q", progresses[1].Note)
	}
}

type stubQtyReader struct {
	stableReadings []stableReadingResult
	printResults   []printRequestResultResult
	nextCycleErrs  []error

	stableIdx    int
	printIdx     int
	nextCycleIdx int
	onNextCycle  func()
}

type stableReadingResult struct {
	reading StableReading
	err     error
}

type printRequestResultResult struct {
	result PrintRequestResult
	err    error
}

func (s *stubQtyReader) WaitStablePositiveReading(context.Context, time.Duration, time.Duration) (StableReading, error) {
	if s.stableIdx >= len(s.stableReadings) {
		return StableReading{}, context.Canceled
	}
	got := s.stableReadings[s.stableIdx]
	s.stableIdx++
	return got.reading, got.err
}

func (s *stubQtyReader) WaitPrintRequestResult(context.Context, time.Duration, time.Duration, string) (PrintRequestResult, error) {
	if s.printIdx >= len(s.printResults) {
		return PrintRequestResult{}, context.Canceled
	}
	got := s.printResults[s.printIdx]
	s.printIdx++
	return got.result, got.err
}

func (s *stubQtyReader) WaitForNextCycle(context.Context, time.Duration, time.Duration, float64) error {
	if s.onNextCycle != nil {
		s.onNextCycle()
		s.onNextCycle = nil
	}
	if s.nextCycleIdx >= len(s.nextCycleErrs) {
		return nil
	}
	err := s.nextCycleErrs[s.nextCycleIdx]
	s.nextCycleIdx++
	return err
}

type stubERP struct {
	createInputs    []CreateMaterialReceiptDraftInput
	createDraftErrs []error
	drafts          []Draft
	submitted       []string
	deleted         []string
}

func (s *stubERP) CreateMaterialReceiptDraft(_ context.Context, in CreateMaterialReceiptDraftInput) (Draft, error) {
	s.createInputs = append(s.createInputs, in)
	idx := len(s.createInputs) - 1
	if idx < len(s.createDraftErrs) && s.createDraftErrs[idx] != nil {
		return Draft{}, s.createDraftErrs[idx]
	}
	if idx < len(s.drafts) {
		return s.drafts[idx], nil
	}
	return Draft{
		Name:      fmt.Sprintf("MAT-STE-%d", idx+1),
		ItemCode:  in.ItemCode,
		Warehouse: in.Warehouse,
		Qty:       in.Qty,
		Barcode:   in.Barcode,
	}, nil
}

func (s *stubERP) SubmitStockEntryDraft(_ context.Context, name string) error {
	s.submitted = append(s.submitted, name)
	return nil
}

func (s *stubERP) DeleteStockEntryDraft(_ context.Context, name string) error {
	s.deleted = append(s.deleted, name)
	return nil
}

type printSetCall struct {
	epc       string
	qty       float64
	grossQty  float64
	unit      string
	selection Selection
}

type stubPrintRequestWriter struct {
	setCalls   []printSetCall
	clearCalls int
	clearEPCs  []string
}

func (s *stubPrintRequestWriter) SetPrintRequest(epc string, qty float64, grossQty float64, unit string, selection Selection) {
	s.setCalls = append(s.setCalls, printSetCall{
		epc:       epc,
		qty:       qty,
		grossQty:  grossQty,
		unit:      unit,
		selection: selection,
	})
}

func (s *stubPrintRequestWriter) ClearPrintRequest(epc string) {
	s.clearCalls++
	s.clearEPCs = append(s.clearEPCs, epc)
}

type stubGenerator struct {
	epcs []string
	idx  int
}

func (s *stubGenerator) Next(time.Time) string {
	if s.idx >= len(s.epcs) {
		return ""
	}
	v := s.epcs[s.idx]
	s.idx++
	return v
}

type stubHistory struct {
	items []string
}

func (s *stubHistory) Add(epc string) {
	s.items = append(s.items, epc)
}

func TestIsContextError(t *testing.T) {
	t.Parallel()

	if !isContextError(context.Canceled) {
		t.Fatal("context.Canceled should be treated as context error")
	}
	if !isContextError(context.DeadlineExceeded) {
		t.Fatal("context.DeadlineExceeded should be treated as context error")
	}
	if isContextError(errors.New("boom")) {
		t.Fatal("plain error should not be treated as context error")
	}
}

func TestRunSkipsTooSmallStableQty(t *testing.T) {
	t.Parallel()

	qtyReader := &stubQtyReader{
		stableReadings: []stableReadingResult{
			{reading: StableReading{Qty: 0.050, Unit: "kg"}},
			{err: context.Canceled},
		},
		nextCycleErrs: []error{nil},
	}
	erp := &stubERP{}
	printWriter := &stubPrintRequestWriter{}
	generator := &stubGenerator{epcs: []string{"EPC-001"}}
	history := &stubHistory{}

	runner := NewMaterialReceiptRunner(MaterialReceiptDependencies{
		QtyReader:     qtyReader,
		ERP:           erp,
		PrintRequests: printWriter,
		EPCGenerator:  generator,
		History:       history,
	})

	err := runner.Run(context.Background(), Selection{
		ItemCode:  "ITEM-001",
		ItemName:  "Green Tea",
		Warehouse: "Stores - A",
	}, Hooks{})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(erp.createInputs) != 0 {
		t.Fatalf("draft should not be created for tiny qty: %#v", erp.createInputs)
	}
	if len(printWriter.setCalls) != 0 {
		t.Fatalf("print request should not be created for tiny qty: %#v", printWriter.setCalls)
	}
	if len(history.items) != 0 {
		t.Fatalf("history should stay empty: %#v", history.items)
	}
}

func TestRunSkipsTooSmallNetQtyWithTare(t *testing.T) {
	t.Parallel()

	qtyReader := &stubQtyReader{
		stableReadings: []stableReadingResult{
			{reading: StableReading{Qty: 0.650, Unit: "kg"}},
			{err: context.Canceled},
		},
		nextCycleErrs: []error{nil},
	}
	erp := &stubERP{}
	printWriter := &stubPrintRequestWriter{}
	progresses := make([]Progress, 0, 1)

	runner := NewMaterialReceiptRunner(MaterialReceiptDependencies{
		QtyReader:     qtyReader,
		ERP:           erp,
		PrintRequests: printWriter,
		EPCGenerator:  &stubGenerator{epcs: []string{"EPC-001"}},
		History:       &stubHistory{},
	})

	err := runner.Run(context.Background(), Selection{
		ItemCode:    "ITEM-001",
		ItemName:    "Green Tea",
		Warehouse:   "Stores - A",
		TareEnabled: true,
		TareKG:      0.78,
	}, Hooks{
		Progress: func(progress Progress) {
			progresses = append(progresses, progress)
		},
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(erp.createInputs) != 0 {
		t.Fatalf("draft should not be created for too-small net qty: %#v", erp.createInputs)
	}
	if len(printWriter.setCalls) != 0 {
		t.Fatalf("print request should not be created for too-small net qty: %#v", printWriter.setCalls)
	}
	if len(progresses) == 0 || !strings.Contains(progresses[0].Note, "NETTO juda kichik") {
		t.Fatalf("expected net qty progress note, got %#v", progresses)
	}
}
