package batchcontrol

import (
	"context"
	"fmt"
	"sync"

	"core/workflow"
)

type Item struct {
	Name     string `json:"name,omitempty"`
	ItemCode string `json:"item_code"`
	ItemName string `json:"item_name"`
}

type WarehouseStock struct {
	Warehouse string  `json:"warehouse"`
	ActualQty float64 `json:"actual_qty"`
}

type Catalog interface {
	CheckConnection(ctx context.Context) (string, error)
	SearchItems(ctx context.Context, query string, limit int, warehouse string) ([]Item, error)
	SearchItemWarehouses(ctx context.Context, itemCode, query string, limit int) ([]WarehouseStock, error)
}

type BatchStateWriter interface {
	Set(active bool, ownerID int64, selection workflow.Selection) error
}

type Runner interface {
	Run(ctx context.Context, selection workflow.Selection, hooks workflow.Hooks) error
	PrintOnce(ctx context.Context, selection workflow.Selection, grossQty float64) (workflow.Draft, string, error)
}

type Logger interface {
	Printf(format string, args ...any)
}

type Dependencies struct {
	Catalog    Catalog
	BatchState BatchStateWriter
	Runner     Runner
	Logger     Logger
}

type Service struct {
	catalog    Catalog
	batchState BatchStateWriter
	runner     Runner
	logger     Logger

	mu        sync.Mutex
	nextID    int64
	batchByID map[int64]batchSession
}

type batchSession struct {
	id        int64
	cancel    context.CancelFunc
	selection workflow.Selection
}

type ActiveBatch struct {
	Active    bool
	OwnerID   int64
	Selection workflow.Selection
}

func New(deps Dependencies) *Service {
	return &Service{
		catalog:    deps.Catalog,
		batchState: deps.BatchState,
		runner:     deps.Runner,
		logger:     deps.Logger,
		batchByID:  make(map[int64]batchSession),
	}
}

func (s *Service) CheckConnection(ctx context.Context) (string, error) {
	if s == nil || s.catalog == nil {
		return "", nil
	}
	return s.catalog.CheckConnection(ctx)
}

func (s *Service) SearchItems(ctx context.Context, query string, limit int, warehouse string) ([]Item, error) {
	if s == nil || s.catalog == nil {
		return nil, nil
	}
	return s.catalog.SearchItems(ctx, query, limit, warehouse)
}

func (s *Service) SearchItemWarehouses(ctx context.Context, itemCode, query string, limit int) ([]WarehouseStock, error) {
	if s == nil || s.catalog == nil {
		return nil, nil
	}
	return s.catalog.SearchItemWarehouses(ctx, itemCode, query, limit)
}

func (s *Service) Start(parent context.Context, ownerID int64, selection workflow.Selection, hooks workflow.Hooks) bool {
	if s == nil || s.runner == nil {
		return false
	}
	selection = selection.Normalize()
	if selection.ItemCode == "" || selection.Warehouse == "" {
		return false
	}

	ctx, cancel := context.WithCancel(parent)

	s.mu.Lock()
	if _, ok := s.batchByID[ownerID]; ok {
		s.mu.Unlock()
		cancel()
		return false
	}
	for activeOwner := range s.batchByID {
		if activeOwner != ownerID {
			s.mu.Unlock()
			cancel()
			return false
		}
	}
	s.nextID++
	session := batchSession{
		id:        s.nextID,
		cancel:    cancel,
		selection: selection,
	}
	s.batchByID[ownerID] = session
	s.mu.Unlock()
	s.syncBatchState(ownerID)

	go func(ownerID int64, sessionID int64, selection workflow.Selection) {
		if err := s.runner.Run(ctx, selection, hooks); err != nil {
			s.logf("batch runner error: owner=%d err=%v", ownerID, err)
		}

		s.mu.Lock()
		cur, ok := s.batchByID[ownerID]
		if ok && cur.id == sessionID {
			delete(s.batchByID, ownerID)
		}
		s.mu.Unlock()
		s.syncBatchState(ownerID)
	}(ownerID, session.id, selection)

	return true
}

func (s *Service) Stop(ownerID int64) bool {
	if s == nil {
		return false
	}

	s.mu.Lock()
	session, ok := s.batchByID[ownerID]
	if ok {
		delete(s.batchByID, ownerID)
	}
	s.mu.Unlock()
	s.syncBatchState(ownerID)

	if ok && session.cancel != nil {
		session.cancel()
		return true
	}
	return false
}

func (s *Service) ManualPrint(ctx context.Context, ownerID int64, grossQty float64) (workflow.Draft, string, error) {
	if s == nil || s.runner == nil {
		return workflow.Draft{}, "", fmt.Errorf("manual print runner bo'sh")
	}
	if grossQty <= 0 {
		return workflow.Draft{}, "", fmt.Errorf("manual qty required")
	}

	s.mu.Lock()
	session, ok := s.batchByID[ownerID]
	s.mu.Unlock()
	if !ok {
		return workflow.Draft{}, "", fmt.Errorf("manual batch active emas")
	}

	selection := session.selection.Normalize()
	if !selection.UsesManualQty() {
		return workflow.Draft{}, "", fmt.Errorf("manual print faqat manual batch uchun")
	}
	selection.ManualQtyKG = grossQty
	return s.runner.PrintOnce(ctx, selection, grossQty)
}

func (s *Service) StopAll() {
	if s == nil {
		return
	}

	s.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(s.batchByID))
	for _, session := range s.batchByID {
		if session.cancel != nil {
			cancels = append(cancels, session.cancel)
		}
	}
	s.batchByID = make(map[int64]batchSession)
	s.mu.Unlock()
	s.syncBatchState(0)

	for _, cancel := range cancels {
		cancel()
	}
}

func (s *Service) HasActiveBatch(ownerID int64) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	_, ok := s.batchByID[ownerID]
	s.mu.Unlock()
	return ok
}

func (s *Service) OtherActiveBatchOwner(ownerID int64) (int64, bool) {
	if s == nil {
		return 0, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for activeOwner := range s.batchByID {
		if activeOwner != ownerID {
			return activeOwner, true
		}
	}
	return 0, false
}

func (s *Service) ActiveBatch() ActiveBatch {
	if s == nil {
		return ActiveBatch{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for ownerID, session := range s.batchByID {
		return ActiveBatch{
			Active:    true,
			OwnerID:   ownerID,
			Selection: session.selection.Normalize(),
		}
	}
	return ActiveBatch{}
}

func (s *Service) syncBatchState(ownerHint int64) {
	if s == nil || s.batchState == nil {
		return
	}

	active := s.ActiveBatch()
	if ownerHint != 0 && active.Active && active.OwnerID != ownerHint {
		active.OwnerID = ownerHint
	}
	if err := s.batchState.Set(active.Active, active.OwnerID, active.Selection); err != nil {
		s.logf("batch state write error: %v", err)
	}
}

func (s *Service) logf(format string, args ...any) {
	if s == nil || s.logger == nil {
		return
	}
	s.logger.Printf(format, args...)
}
