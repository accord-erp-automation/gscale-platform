package workflow

import (
	"context"
	"strings"
	"time"
)

type Selection struct {
	ItemCode       string
	ItemName       string
	Warehouse      string
	PrintMode      string
	Printer        string
	QuantitySource string
	ManualQtyKG    float64
	TareEnabled    bool
	TareKG         float64
}

func (s Selection) Normalize() Selection {
	s.ItemCode = strings.TrimSpace(s.ItemCode)
	s.ItemName = strings.TrimSpace(s.ItemName)
	s.Warehouse = strings.TrimSpace(s.Warehouse)
	s.PrintMode = normalizePrintMode(s.PrintMode)
	s.Printer = strings.ToLower(strings.TrimSpace(s.Printer))
	s.QuantitySource = normalizeQuantitySource(s.QuantitySource)
	if s.QuantitySource != QuantitySourceManual || s.ManualQtyKG <= 0 {
		s.ManualQtyKG = 0
	}
	if !s.TareEnabled || s.TareKG <= 0 {
		s.TareEnabled = false
		s.TareKG = 0
	}
	if s.ItemName == "" {
		s.ItemName = s.ItemCode
	}
	return s
}

func (s Selection) UsesManualQty() bool {
	return s.Normalize().QuantitySource == QuantitySourceManual
}

func (s Selection) NetQty(grossQty float64) float64 {
	s = s.Normalize()
	if !s.TareEnabled {
		return grossQty
	}
	net := grossQty - s.TareKG
	if net < 0 {
		return 0
	}
	return net
}

const (
	PrintModeRFID      = "rfid"
	PrintModeLabelOnly = "label"
)

const (
	QuantitySourceScale  = "scale"
	QuantitySourceManual = "manual"
)

func normalizePrintMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", PrintModeRFID, "rfid-label", "rfid_label", "rfidprint":
		return PrintModeRFID
	case PrintModeLabelOnly, "label-only", "label_only", "plain", "plain-label", "plain_label", "simple":
		return PrintModeLabelOnly
	default:
		return PrintModeRFID
	}
}

func normalizeQuantitySource(source string) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case QuantitySourceManual, "manual_kg", "manual-kg", "kg":
		return QuantitySourceManual
	default:
		return QuantitySourceScale
	}
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
	TotalQty    float64
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
	SetPrintRequest(epc string, qty float64, grossQty float64, unit string, selection Selection)
	ClearPrintRequest(epc string)
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
