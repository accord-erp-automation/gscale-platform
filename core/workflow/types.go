package workflow

import (
	"context"
	"strings"
	"time"
)

type Selection struct {
	ItemCode  string
	ItemName  string
	Warehouse string
}

func (s Selection) Normalize() Selection {
	s.ItemCode = strings.TrimSpace(s.ItemCode)
	s.ItemName = strings.TrimSpace(s.ItemName)
	s.Warehouse = strings.TrimSpace(s.Warehouse)
	if s.ItemName == "" {
		s.ItemName = s.ItemCode
	}
	return s
}

type StableReading struct {
	Qty       float64
	Unit      string
	UpdatedAt time.Time
}

type PrintRequestResult struct {
	EPC       string
	Status    string
	Error     string
	UpdatedAt time.Time
}

type CreateMaterialReceiptDraftInput struct {
	ItemCode  string
	Warehouse string
	Qty       float64
	Barcode   string
}

type Draft struct {
	Name      string
	ItemCode  string
	Warehouse string
	Qty       float64
	UOM       string
	Barcode   string
}

type LastSuccess struct {
	DraftName string
	Qty       float64
	Unit      string
	EPC       string
	Verify    string
}

type Progress struct {
	Selection   Selection
	DraftCount  int
	LastSuccess LastSuccess
	Note        string
}

type QtyReader interface {
	WaitStablePositiveReading(ctx context.Context, timeout, pollInterval time.Duration) (StableReading, error)
	WaitPrintRequestResult(ctx context.Context, timeout, pollInterval time.Duration, epc string) (PrintRequestResult, error)
	WaitForNextCycle(ctx context.Context, timeout, pollInterval time.Duration, lastQty float64) error
}

type ERP interface {
	CreateMaterialReceiptDraft(ctx context.Context, in CreateMaterialReceiptDraftInput) (Draft, error)
	SubmitStockEntryDraft(ctx context.Context, name string) error
	DeleteStockEntryDraft(ctx context.Context, name string) error
}

type PrintRequestWriter interface {
	SetPrintRequest(epc string, qty float64, unit string, selection Selection)
	ClearPrintRequest()
}

type EPCGenerator interface {
	Next(t time.Time) string
}

type History interface {
	Add(epc string)
}

type Logger interface {
	Printf(format string, args ...any)
}

type Hooks struct {
	Progress func(Progress)
}

type MaterialReceiptOptions struct {
	StableReadTimeout       time.Duration
	StableReadPollInterval  time.Duration
	PrintResultTimeout      time.Duration
	PrintResultPollInterval time.Duration
	NextCycleTimeout        time.Duration
	NextCyclePollInterval   time.Duration
}

func DefaultMaterialReceiptOptions() MaterialReceiptOptions {
	return MaterialReceiptOptions{
		StableReadTimeout:       35 * time.Second,
		StableReadPollInterval:  220 * time.Millisecond,
		PrintResultTimeout:      12 * time.Second,
		PrintResultPollInterval: 120 * time.Millisecond,
		NextCycleTimeout:        10 * time.Minute,
		NextCyclePollInterval:   220 * time.Millisecond,
	}
}

type MaterialReceiptDependencies struct {
	QtyReader               QtyReader
	ERP                     ERP
	PrintRequests           PrintRequestWriter
	EPCGenerator            EPCGenerator
	History                 History
	Logger                  Logger
	IsDuplicateBarcodeError func(error) bool
}

type MaterialReceiptRunner struct {
	qtyReader               QtyReader
	erp                     ERP
	printRequests           PrintRequestWriter
	epcGenerator            EPCGenerator
	history                 History
	logger                  Logger
	isDuplicateBarcodeError func(error) bool
	options                 MaterialReceiptOptions
}

func NewMaterialReceiptRunner(deps MaterialReceiptDependencies) *MaterialReceiptRunner {
	return &MaterialReceiptRunner{
		qtyReader:               deps.QtyReader,
		erp:                     deps.ERP,
		printRequests:           deps.PrintRequests,
		epcGenerator:            deps.EPCGenerator,
		history:                 deps.History,
		logger:                  deps.Logger,
		isDuplicateBarcodeError: deps.IsDuplicateBarcodeError,
		options:                 DefaultMaterialReceiptOptions(),
	}
}

func (r *MaterialReceiptRunner) WithOptions(options MaterialReceiptOptions) *MaterialReceiptRunner {
	if r == nil {
		return nil
	}
	if options.StableReadTimeout > 0 {
		r.options.StableReadTimeout = options.StableReadTimeout
	}
	if options.StableReadPollInterval > 0 {
		r.options.StableReadPollInterval = options.StableReadPollInterval
	}
	if options.PrintResultTimeout > 0 {
		r.options.PrintResultTimeout = options.PrintResultTimeout
	}
	if options.PrintResultPollInterval > 0 {
		r.options.PrintResultPollInterval = options.PrintResultPollInterval
	}
	if options.NextCycleTimeout > 0 {
		r.options.NextCycleTimeout = options.NextCycleTimeout
	}
	if options.NextCyclePollInterval > 0 {
		r.options.NextCyclePollInterval = options.NextCyclePollInterval
	}
	return r
}
