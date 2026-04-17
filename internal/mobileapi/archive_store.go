package mobileapi

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	flatbuffers "github.com/google/flatbuffers/go"
)

type archiveEventKind uint8

const (
	archiveEventUnknown archiveEventKind = iota
	archiveEventStart
	archiveEventPrint
	archiveEventStop
)

const (
	archiveEventVTKind       flatbuffers.VOffsetT = 4
	archiveEventVTSessionID  flatbuffers.VOffsetT = 6
	archiveEventVTItemCode   flatbuffers.VOffsetT = 8
	archiveEventVTItemName   flatbuffers.VOffsetT = 10
	archiveEventVTWarehouse  flatbuffers.VOffsetT = 12
	archiveEventVTQty        flatbuffers.VOffsetT = 14
	archiveEventVTUnit       flatbuffers.VOffsetT = 16
	archiveEventVTDraftName  flatbuffers.VOffsetT = 18
	archiveEventVTEPC        flatbuffers.VOffsetT = 20
	archiveEventVTOccurredAt flatbuffers.VOffsetT = 22
	archiveEventVTTotalQty   flatbuffers.VOffsetT = 24
	archiveEventVTPrintCount flatbuffers.VOffsetT = 26
)

type archiveEventRecord struct {
	Kind       archiveEventKind
	SessionID  string
	ItemCode   string
	ItemName   string
	Warehouse  string
	Qty        float64
	Unit       string
	DraftName  string
	EPC        string
	OccurredAt string
	TotalQty   float64
	PrintCount uint32
}

type ArchivePrintEntry struct {
	ItemCode  string  `json:"item_code"`
	ItemName  string  `json:"item_name"`
	Qty       float64 `json:"qty"`
	Unit      string  `json:"unit,omitempty"`
	DraftName string  `json:"draft_name,omitempty"`
	EPC       string  `json:"epc,omitempty"`
	PrintedAt string  `json:"printed_at"`
}

type ArchiveSession struct {
	SessionID  string              `json:"session_id"`
	Active     bool                `json:"active"`
	ItemCode   string              `json:"item_code"`
	ItemName   string              `json:"item_name"`
	Warehouse  string              `json:"warehouse"`
	StartedAt  string              `json:"started_at"`
	EndedAt    string              `json:"ended_at,omitempty"`
	TotalQty   float64             `json:"total_qty"`
	Unit       string              `json:"unit,omitempty"`
	PrintCount int                 `json:"print_count"`
	Prints     []ArchivePrintEntry `json:"prints,omitempty"`
}

type archiveSessionState struct {
	SessionID  string
	ItemCode   string
	ItemName   string
	Warehouse  string
	StartedAt  string
	EndedAt    string
	Active     bool
	TotalQty   float64
	Unit       string
	PrintCount int
	Prints     []ArchivePrintEntry
	LastAt     string
}

type ArchiveStore struct {
	path string

	mu sync.Mutex

	activeSessionID  string
	activeSelection  archiveSelection
	activeStartedAt  string
	activeTotalQty   float64
	activePrintCount int
	activeUnit       string
}

type archiveSelection struct {
	ItemCode  string
	ItemName  string
	Warehouse string
}

func NewArchiveStore(path string) *ArchiveStore {
	path = strings.TrimSpace(path)
	if path == "" {
		return &ArchiveStore{}
	}
	return &ArchiveStore{path: path}
}

func (s *ArchiveStore) Path() string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(s.path)
}

func (s *ArchiveStore) OpenSession(itemCode, itemName, warehouse string, startedAt time.Time) (string, error) {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return "", nil
	}

	itemCode = strings.TrimSpace(itemCode)
	itemName = strings.TrimSpace(itemName)
	warehouse = strings.TrimSpace(warehouse)
	if itemName == "" {
		itemName = itemCode
	}
	if itemCode == "" || warehouse == "" {
		return "", fmt.Errorf("archive selection bo'sh")
	}
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.restoreActiveSessionLocked(); err != nil {
		return "", err
	}
	if s.activeSessionID != "" {
		return "", fmt.Errorf("archive session allaqachon faol")
	}

	sessionID, err := newArchiveSessionID()
	if err != nil {
		return "", err
	}

	record := archiveEventRecord{
		Kind:       archiveEventStart,
		SessionID:  sessionID,
		ItemCode:   itemCode,
		ItemName:   itemName,
		Warehouse:  warehouse,
		OccurredAt: startedAt.UTC().Format(time.RFC3339Nano),
	}
	if err := s.appendRecordLocked(record); err != nil {
		return "", err
	}

	s.activeSessionID = sessionID
	s.activeSelection = archiveSelection{
		ItemCode:  itemCode,
		ItemName:  itemName,
		Warehouse: warehouse,
	}
	s.activeStartedAt = record.OccurredAt
	s.activeTotalQty = 0
	s.activePrintCount = 0
	s.activeUnit = ""
	return sessionID, nil
}

func (s *ArchiveStore) RecordPrint(qty float64, unit, draftName, epc string, printedAt time.Time) error {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return nil
	}
	if qty <= 0 {
		return nil
	}
	if printedAt.IsZero() {
		printedAt = time.Now().UTC()
	}
	unit = normalizeArchiveUnit(unit)

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.restoreActiveSessionLocked(); err != nil {
		return err
	}
	if s.activeSessionID == "" {
		return fmt.Errorf("archive session faol emas")
	}

	record := archiveEventRecord{
		Kind:       archiveEventPrint,
		SessionID:  s.activeSessionID,
		ItemCode:   s.activeSelection.ItemCode,
		ItemName:   s.activeSelection.ItemName,
		Warehouse:  s.activeSelection.Warehouse,
		Qty:        qty,
		Unit:       unit,
		DraftName:  strings.TrimSpace(draftName),
		EPC:        strings.ToUpper(strings.TrimSpace(epc)),
		OccurredAt: printedAt.UTC().Format(time.RFC3339Nano),
	}
	if err := s.appendRecordLocked(record); err != nil {
		return err
	}

	s.activeTotalQty += qty
	s.activePrintCount++
	if unit != "" {
		s.activeUnit = unit
	}
	return nil
}

func (s *ArchiveStore) CloseSession(stoppedAt time.Time) error {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return nil
	}
	if stoppedAt.IsZero() {
		stoppedAt = time.Now().UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.restoreActiveSessionLocked(); err != nil {
		return err
	}
	if s.activeSessionID == "" {
		return nil
	}

	record := archiveEventRecord{
		Kind:       archiveEventStop,
		SessionID:  s.activeSessionID,
		ItemCode:   s.activeSelection.ItemCode,
		ItemName:   s.activeSelection.ItemName,
		Warehouse:  s.activeSelection.Warehouse,
		OccurredAt: stoppedAt.UTC().Format(time.RFC3339Nano),
		TotalQty:   s.activeTotalQty,
		PrintCount: uint32(s.activePrintCount),
		Unit:       s.activeUnit,
	}
	if err := s.appendRecordLocked(record); err != nil {
		return err
	}

	s.activeSessionID = ""
	s.activeSelection = archiveSelection{}
	s.activeStartedAt = ""
	s.activeTotalQty = 0
	s.activePrintCount = 0
	s.activeUnit = ""
	return nil
}

func (s *ArchiveStore) ListSessions(limit int) ([]ArchiveSession, error) {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return nil, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sessions, _, err := s.readSessionsLocked()
	if err != nil {
		return nil, err
	}

	sort.Slice(sessions, func(i, j int) bool {
		return parseArchiveTime(sessions[i].StartedAt).After(parseArchiveTime(sessions[j].StartedAt))
	})

	if limit > 0 && len(sessions) > limit {
		sessions = sessions[:limit]
	}
	return sessions, nil
}

func (s *ArchiveStore) restoreActiveSessionLocked() error {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return nil
	}
	return nil
}

func (s *ArchiveStore) readSessionsLocked() ([]ArchiveSession, *archiveSessionState, error) {
	records, err := s.readRecordsLocked()
	if err != nil {
		return nil, nil, err
	}

	sessionByID := make(map[string]*archiveSessionState, 8)
	order := make([]string, 0, 8)
	var active *archiveSessionState

	for _, rec := range records {
		sessionID := strings.TrimSpace(rec.SessionID)
		if sessionID == "" {
			continue
		}

		state, ok := sessionByID[sessionID]
		if !ok {
			state = &archiveSessionState{SessionID: sessionID}
			sessionByID[sessionID] = state
			order = append(order, sessionID)
		}

		switch rec.Kind {
		case archiveEventStart:
			state.ItemCode = firstNonEmpty(state.ItemCode, rec.ItemCode)
			state.ItemName = firstNonEmpty(state.ItemName, rec.ItemName, rec.ItemCode)
			state.Warehouse = firstNonEmpty(state.Warehouse, rec.Warehouse)
			state.StartedAt = firstNonEmpty(state.StartedAt, rec.OccurredAt)
			state.Active = true
			if state.Unit == "" {
				state.Unit = normalizeArchiveUnit(rec.Unit)
			}
			if state.StartedAt != "" {
				state.LastAt = state.StartedAt
			}
		case archiveEventPrint:
			state.ItemCode = firstNonEmpty(state.ItemCode, rec.ItemCode)
			state.ItemName = firstNonEmpty(state.ItemName, rec.ItemName, rec.ItemCode)
			state.Warehouse = firstNonEmpty(state.Warehouse, rec.Warehouse)
			state.PrintCount++
			state.TotalQty += rec.Qty
			if unit := normalizeArchiveUnit(rec.Unit); unit != "" {
				state.Unit = unit
			}
			printedAt := firstNonEmpty(rec.OccurredAt, state.LastAt)
			if printedAt == "" {
				printedAt = time.Now().UTC().Format(time.RFC3339Nano)
			}
			state.Prints = append(state.Prints, ArchivePrintEntry{
				ItemCode:  firstNonEmpty(rec.ItemCode, state.ItemCode),
				ItemName:  firstNonEmpty(rec.ItemName, state.ItemName, rec.ItemCode),
				Qty:       rec.Qty,
				Unit:      normalizeArchiveUnit(rec.Unit),
				DraftName: strings.TrimSpace(rec.DraftName),
				EPC:       strings.ToUpper(strings.TrimSpace(rec.EPC)),
				PrintedAt: printedAt,
			})
			state.LastAt = printedAt
		case archiveEventStop:
			state.ItemCode = firstNonEmpty(state.ItemCode, rec.ItemCode)
			state.ItemName = firstNonEmpty(state.ItemName, rec.ItemName, rec.ItemCode)
			state.Warehouse = firstNonEmpty(state.Warehouse, rec.Warehouse)
			state.EndedAt = firstNonEmpty(state.EndedAt, rec.OccurredAt)
			state.Active = false
			if rec.TotalQty > 0 {
				state.TotalQty = rec.TotalQty
			}
			if rec.PrintCount > 0 {
				state.PrintCount = int(rec.PrintCount)
			}
			if unit := normalizeArchiveUnit(rec.Unit); unit != "" {
				state.Unit = unit
			}
			if state.EndedAt != "" {
				state.LastAt = state.EndedAt
			}
		}
	}

	sessions := make([]ArchiveSession, 0, len(order))
	for _, sessionID := range order {
		state := sessionByID[sessionID]
		if state == nil {
			continue
		}
		session := ArchiveSession{
			SessionID:  state.SessionID,
			Active:     state.Active,
			ItemCode:   strings.TrimSpace(state.ItemCode),
			ItemName:   strings.TrimSpace(state.ItemName),
			Warehouse:  strings.TrimSpace(state.Warehouse),
			StartedAt:  strings.TrimSpace(state.StartedAt),
			EndedAt:    strings.TrimSpace(state.EndedAt),
			TotalQty:   state.TotalQty,
			Unit:       normalizeArchiveUnit(state.Unit),
			PrintCount: state.PrintCount,
			Prints:     append([]ArchivePrintEntry(nil), state.Prints...),
		}
		if session.ItemName == "" {
			session.ItemName = session.ItemCode
		}
		if session.TotalQty == 0 && len(session.Prints) > 0 {
			for _, entry := range session.Prints {
				session.TotalQty += entry.Qty
			}
		}
		if session.PrintCount == 0 {
			session.PrintCount = len(session.Prints)
		}
		sessions = append(sessions, session)
		if session.Active {
			copyState := state
			active = copyState
		}
	}

	return sessions, active, nil
}

func (s *ArchiveStore) readRecordsLocked() ([]archiveEventRecord, error) {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return nil, nil
	}

	b, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	records := make([]archiveEventRecord, 0, 32)
	for offset := 0; offset < len(b); {
		if len(b)-offset < 4 {
			break
		}
		size := binary.LittleEndian.Uint32(b[offset : offset+4])
		offset += 4
		if size == 0 || int(size) > len(b)-offset {
			break
		}

		record, err := decodeArchiveEvent(b[offset : offset+int(size)])
		if err != nil {
			break
		}
		records = append(records, record)
		offset += int(size)
	}
	return records, nil
}

func (s *ArchiveStore) appendRecordLocked(record archiveEventRecord) error {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("archive mkdir: %w", err)
	}

	unlock, err := lockArchiveFile(s.path + ".lock")
	if err != nil {
		return fmt.Errorf("archive lock: %w", err)
	}
	defer unlock()

	payload, err := encodeArchiveEvent(record)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("archive open: %w", err)
	}
	defer f.Close()

	var sizeBuf [4]byte
	binary.LittleEndian.PutUint32(sizeBuf[:], uint32(len(payload)))
	if _, err := f.Write(sizeBuf[:]); err != nil {
		return fmt.Errorf("archive write size: %w", err)
	}
	if _, err := f.Write(payload); err != nil {
		return fmt.Errorf("archive write payload: %w", err)
	}
	return nil
}

func encodeArchiveEvent(record archiveEventRecord) ([]byte, error) {
	builder := flatbuffers.NewBuilder(256)

	sessionID := builder.CreateString(strings.TrimSpace(record.SessionID))
	itemCode := builder.CreateString(strings.TrimSpace(record.ItemCode))
	itemName := builder.CreateString(strings.TrimSpace(record.ItemName))
	warehouse := builder.CreateString(strings.TrimSpace(record.Warehouse))
	unit := builder.CreateString(normalizeArchiveUnit(record.Unit))
	draftName := builder.CreateString(strings.TrimSpace(record.DraftName))
	epc := builder.CreateString(strings.ToUpper(strings.TrimSpace(record.EPC)))
	occurredAt := builder.CreateString(strings.TrimSpace(record.OccurredAt))

	builder.StartObject(12)
	builder.PrependUint8Slot(0, uint8(record.Kind), 0)
	builder.PrependUOffsetTSlot(1, sessionID, 0)
	builder.PrependUOffsetTSlot(2, itemCode, 0)
	builder.PrependUOffsetTSlot(3, itemName, 0)
	builder.PrependUOffsetTSlot(4, warehouse, 0)
	builder.PrependFloat64Slot(5, record.Qty, 0)
	builder.PrependUOffsetTSlot(6, unit, 0)
	builder.PrependUOffsetTSlot(7, draftName, 0)
	builder.PrependUOffsetTSlot(8, epc, 0)
	builder.PrependUOffsetTSlot(9, occurredAt, 0)
	builder.PrependFloat64Slot(10, record.TotalQty, 0)
	// Print count is encoded into the same record to keep the file append-only
	// and easy to project into the archive list.
	builder.PrependUint32Slot(11, record.PrintCount, 0)
	root := builder.EndObject()
	builder.Finish(root)
	return append([]byte(nil), builder.FinishedBytes()...), nil
}

func decodeArchiveEvent(buf []byte) (archiveEventRecord, error) {
	if len(buf) == 0 {
		return archiveEventRecord{}, fmt.Errorf("archive record bo'sh")
	}

	var tab flatbuffers.Table
	tab.Bytes = buf
	tab.Pos = flatbuffers.GetUOffsetT(buf)

	record := archiveEventRecord{
		Kind:       archiveEventKind(tab.GetUint8Slot(archiveEventVTKind, 0)),
		SessionID:  archiveTableString(&tab, archiveEventVTSessionID),
		ItemCode:   archiveTableString(&tab, archiveEventVTItemCode),
		ItemName:   archiveTableString(&tab, archiveEventVTItemName),
		Warehouse:  archiveTableString(&tab, archiveEventVTWarehouse),
		Qty:        tab.GetFloat64Slot(archiveEventVTQty, 0),
		Unit:       normalizeArchiveUnit(archiveTableString(&tab, archiveEventVTUnit)),
		DraftName:  archiveTableString(&tab, archiveEventVTDraftName),
		EPC:        archiveTableString(&tab, archiveEventVTEPC),
		OccurredAt: archiveTableString(&tab, archiveEventVTOccurredAt),
		TotalQty:   tab.GetFloat64Slot(archiveEventVTTotalQty, 0),
		PrintCount: tab.GetUint32Slot(archiveEventVTPrintCount, 0),
	}
	return record, nil
}

func lockArchiveFile(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}

func newArchiveSessionID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func normalizeArchiveUnit(unit string) string {
	unit = strings.TrimSpace(unit)
	if unit == "" {
		return "kg"
	}
	return strings.ToLower(unit)
}

func parseArchiveTime(v string) time.Time {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return time.Time{}
	}
	return t
}

func archiveTableString(tab *flatbuffers.Table, slot flatbuffers.VOffsetT) string {
	if tab == nil {
		return ""
	}
	off := tab.Offset(slot)
	if off == 0 {
		return ""
	}
	return strings.TrimSpace(tab.String(flatbuffers.UOffsetT(off) + tab.Pos))
}
