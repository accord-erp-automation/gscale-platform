package main

import (
	bridgestate "bridge/state"
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type readingMsg struct {
	reading Reading
}

type zebraMsg struct {
	status ZebraStatus
}

type quitMsg struct{}

type clockMsg time.Time

type tuiModel struct {
	ctx                   context.Context
	updates               <-chan Reading
	zebraUpdates          <-chan ZebraStatus
	sourceLine            string
	zebraPreferred        string
	bridgeStore           *bridgestate.Store
	batchState            *batchStateReader
	printRequest          *printRequestReader
	batchActive           bool
	message               string
	info                  string
	last                  Reading
	zebra                 ZebraStatus
	width                 int
	height                int
	now                   time.Time
	activePrintRequestEPC string
}

func runTUI(ctx context.Context, updates <-chan Reading, zebraUpdates <-chan ZebraStatus, sourceLine string, zebraPreferred string, bridgeStateFile string, autoWhenNoBatch bool, serialErr error) error {
	m := newRuntimeModel(ctx, updates, zebraUpdates, sourceLine, zebraPreferred, bridgeStateFile, autoWhenNoBatch, serialErr)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func newRuntimeModel(ctx context.Context, updates <-chan Reading, zebraUpdates <-chan ZebraStatus, sourceLine string, zebraPreferred string, bridgeStateFile string, autoWhenNoBatch bool, serialErr error) tuiModel {
	m := tuiModel{
		ctx:            ctx,
		updates:        updates,
		zebraUpdates:   zebraUpdates,
		sourceLine:     sourceLine,
		zebraPreferred: zebraPreferred,
		bridgeStore:    bridgestate.New(bridgeStateFile),
		batchState:     newBatchStateReader(bridgeStateFile, autoWhenNoBatch),
		printRequest:   newPrintRequestReader(bridgeStateFile),
		batchActive:    true,
		last:           Reading{Unit: "kg"},
		message:        "scale oqimi kutilmoqda",
		info:           "ready",
		now:            time.Now(),
		zebra: ZebraStatus{
			Connected: false,
			Verify:    "-",
			ReadLine1: "-",
			ReadLine2: "-",
			UpdatedAt: time.Now(),
		},
	}
	if m.batchState != nil {
		m.batchActive = m.batchState.Active(time.Now())
	}
	if serialErr != nil {
		m.message = serialErr.Error()
	}
	if zebraUpdates == nil {
		m.zebra.Error = "disabled"
	}
	return m
}

func (m tuiModel) Init() tea.Cmd {
	cmds := []tea.Cmd{waitForReadingCmd(m.ctx, m.updates), clockTickCmd()}
	if m.zebraUpdates != nil {
		cmds = append(cmds, waitForZebraCmd(m.ctx, m.zebraUpdates))
	}
	return tea.Batch(cmds...)
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		s := strings.ToLower(strings.TrimSpace(msg.String()))
		switch s {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "e":
			if !m.batchActive {
				m.info = "batch inactive: botda Material Receipt ni bosing"
				return m, nil
			}
			if m.zebraUpdates == nil {
				m.info = "zebra monitor o'chirilgan (--no-zebra)"
				return m, nil
			}
			m.info = "encode+print yuborildi"
			itemName := ""
			if m.batchState != nil {
				itemName = m.batchState.ItemLabel(time.Now())
			}
			return m, runEncodeEPCCmd(m.zebraPreferred, m.last.Weight, m.last.Unit, itemName)
		case "r":
			if !m.batchActive {
				m.info = "batch inactive: botda Material Receipt ni bosing"
				return m, nil
			}
			if m.zebraUpdates == nil {
				m.info = "zebra monitor o'chirilgan (--no-zebra)"
				return m, nil
			}
			m.info = "rfid read yuborildi"
			return m, runRFIDReadCmd(m.zebraPreferred)
		default:
			return m, nil
		}
	case readingMsg:
		upd := msg.reading
		if upd.Unit == "" && m.last.Unit != "" {
			upd.Unit = m.last.Unit
		}

		prevBatchActive := m.batchActive
		if m.batchState != nil {
			m.batchActive = m.batchState.Active(time.Now())
		}
		if prevBatchActive != m.batchActive {
			if m.batchActive {
				m.info = "batch active: ERP workflow yoqildi"
			} else {
				m.info = "batch inactive: ERP workflow to'xtadi"
			}
		}

		m.last = upd
		if err := writeBridgeStateSnapshot(m.bridgeStore, upd, m.zebra); err != nil {
			m.info = "bridge snapshot xato: " + err.Error()
		}
		if upd.Error != "" {
			m.message = upd.Error
		} else {
			m.message = "ok"
		}

		cmd := waitForReadingCmd(m.ctx, m.updates)
		if !m.batchActive {
			return m, cmd
		}
		return m, cmd
	case zebraMsg:
		st := mergeZebraStatus(m.zebra, msg.status)
		m.zebra = st
		if err := writeBridgeStateSnapshot(m.bridgeStore, m.last, m.zebra); err != nil {
			m.info = "bridge snapshot xato: " + err.Error()
		}
		if m.activePrintRequestEPC != "" && strings.EqualFold(strings.TrimSpace(st.Action), "encode") {
			status := "done"
			errText := ""
			if strings.TrimSpace(st.Error) != "" {
				status = "error"
				errText = st.Error
			}
			if err := writePrintRequestStatus(m.bridgeStore, m.activePrintRequestEPC, status, errText); err != nil {
				m.info = "print request status xato: " + err.Error()
			}
			m.activePrintRequestEPC = ""
		}
		if st.Action != "" {
			m.info = zebraActionSummary(st)
		}
		if st.Error != "" && st.Action != "" {
			m.info = zebraActionSummary(st)
		}
		if m.zebraUpdates != nil {
			return m, waitForZebraCmd(m.ctx, m.zebraUpdates)
		}
		return m, nil
	case quitMsg:
		return m, tea.Quit
	case clockMsg:
		m.now = time.Time(msg)
		cmd := clockTickCmd()
		if reqCmd := m.syncPendingPrintRequest(m.now); reqCmd != nil {
			cmd = tea.Batch(cmd, reqCmd)
		}
		return m, cmd
	default:
		return m, nil
	}
}

func mergeZebraStatus(prev ZebraStatus, incoming ZebraStatus) ZebraStatus {
	st := incoming
	if strings.TrimSpace(st.LastEPC) == "" && strings.TrimSpace(prev.LastEPC) != "" {
		st.LastEPC = prev.LastEPC
		if strings.TrimSpace(st.Verify) == "" || strings.TrimSpace(st.Verify) == "-" {
			st.Verify = prev.Verify
		}
		// Monitor heartbeat eski EPC ni qayta vaqtlab yubormasin.
		if !prev.UpdatedAt.IsZero() {
			st.UpdatedAt = prev.UpdatedAt
		}
	}
	if st.UpdatedAt.IsZero() {
		st.UpdatedAt = time.Now()
	}
	return st
}

func (m tuiModel) View() string {
	w, _ := viewSize(m.width, m.height)
	now := m.now
	if now.IsZero() {
		now = time.Now()
	}

	unit := strings.TrimSpace(m.last.Unit)
	if unit == "" {
		unit = "kg"
	}
	qty := "-- " + unit
	if m.last.Weight != nil {
		qty = fmt.Sprintf("%.3f %s", *m.last.Weight, unit)
	}

	status := strings.TrimSpace(m.message)
	if status == "" {
		status = "ok"
	}

	scaleConnected := isConnected(status, m.last, now)
	scaleState := stateText(scaleConnected)

	port := strings.TrimSpace(m.last.Port)
	if port == "" {
		port = "-"
	}

	updated := "-"
	lag := "-"
	if !m.last.UpdatedAt.IsZero() {
		updated = m.last.UpdatedAt.Format("15:04:05.000")
		d := now.Sub(m.last.UpdatedAt)
		if d < 0 {
			d = 0
		}
		lag = fmt.Sprintf("%d ms", d.Milliseconds())
	}

	panelW := w

	zebraDisabled := strings.EqualFold(strings.TrimSpace(m.zebra.Error), "disabled")
	zebraConnected := m.zebra.Connected && strings.TrimSpace(m.zebra.Error) == "" && !zebraDisabled
	zebraState := "DOWN"
	if zebraDisabled {
		zebraState = "DISABLED"
	} else if zebraConnected {
		zebraState = "UP"
	}

	zebraName := strings.TrimSpace(m.zebra.Name)
	if zebraName == "" {
		zebraName = "-"
	}
	zebraDevice := strings.TrimSpace(m.zebra.DevicePath)
	if zebraDevice == "" {
		zebraDevice = "-"
	}
	deviceState := strings.ToUpper(safeText("-", m.zebra.DeviceState))
	mediaState := strings.ToUpper(safeText("-", m.zebra.MediaState))
	read1 := safeText("-", m.zebra.ReadLine1)
	read2 := safeText("-", m.zebra.ReadLine2)
	verify := strings.ToUpper(safeText("-", m.zebra.Verify))
	lastEPC := safeText("-", m.zebra.LastEPC)
	zebraUpdated := "-"
	if !m.zebra.UpdatedAt.IsZero() {
		zebraUpdated = m.zebra.UpdatedAt.Format("15:04:05.000")
	}
	zebraErr := safeText("-", m.zebra.Error)

	scaleLines := []string{
		kv("STATUS", scaleState),
		kv("BATCH", batchGateText(m.batchActive)),
		kv("QTY", qty),
		kv("STABLE", strings.ToUpper(stableText(m.last.Stable))),
		kv("UPDATED", updated),
		kv("LAG", lag),
		kv("SOURCE", elideMiddle(m.sourceLine, maxInt(20, panelW-16))),
		kv("PORT", elideMiddle(port, maxInt(20, panelW-16))),
		kv("MESSAGE", elideMiddle(status, maxInt(20, panelW-16))),
	}

	zebraLines := []string{
		kv("STATUS", zebraState),
		kv("PRINTER", elideMiddle(zebraName, maxInt(18, panelW-16))),
		kv("DEVICE", elideMiddle(zebraDevice, maxInt(18, panelW-16))),
		kv("DEVICE ST", deviceState),
		kv("MEDIA ST", mediaState),
		kv("VERIFY", verify),
		kv("LAST EPC", elideMiddle(lastEPC, maxInt(18, panelW-16))),
		kv("READ 1", elideMiddle(read1, maxInt(18, panelW-16))),
		kv("READ 2", elideMiddle(read2, maxInt(18, panelW-16))),
		kv("UPDATED", zebraUpdated),
		kv("ERROR", elideMiddle(zebraErr, maxInt(18, panelW-16))),
	}

	header := renderTopBar(w, now, scaleState, zebraState, m.batchActive)
	footer := renderBottomBar(w, m.info)

	panel := renderUnifiedStatusCard("GSCALE-ZEBRA MONITOR", scaleState, zebraState, scaleLines, zebraLines, panelW)

	return header + "\n" + panel + "\n" + footer
}

func renderTopBar(width int, now time.Time, scaleState, zebraState string, batchActive bool) string {
	line := fmt.Sprintf(
		"GSCALE-ZEBRA MONITOR | %s | SCALE=%s | ZEBRA=%s | BATCH=%s",
		now.Format("2006-01-02 15:04:05"),
		scaleState,
		zebraState,
		batchGateText(batchActive),
	)
	line = fitLineRaw(line, width)
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#E2E8F0")).
		Render(line)
}

func renderBottomBar(width int, info string) string {
	left := "keys: [q] quit [e] encode+print [r] read"
	info = strings.TrimSpace(info)
	line := left
	if info != "" {
		line = left + " | " + info
	}
	line = fitLineRaw(line, width)

	color := lipgloss.Color("#94A3B8")
	l := strings.ToLower(info)
	if strings.Contains(l, "xato") || strings.Contains(l, "error") || strings.Contains(l, "timeout") {
		color = lipgloss.Color("#F59E0B")
	} else if info != "" {
		color = lipgloss.Color("#A7F3D0")
	}

	return lipgloss.NewStyle().Foreground(color).Render(line)
}

func renderStatusCard(title, state string, lines []string, width int) string {
	if width < 34 {
		width = 34
	}
	inner := width - 4
	if inner < 10 {
		inner = 10
	}

	header := fitPanelLine(strings.ToUpper(strings.TrimSpace(title))+" | "+strings.ToUpper(strings.TrimSpace(state)), inner)
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E2E8F0"))
	lineStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#CBD5E1"))

	rows := make([]string, 0, len(lines)+1)
	rows = append(rows, titleStyle.Render(header))
	for _, line := range lines {
		rows = append(rows, lineStyle.Render(fitPanelLine(line, inner)))
	}

	cardStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(panelBorderColor(state)).
		Padding(0, 1)
	return cardStyle.Render(strings.Join(rows, "\n"))
}

func panelBorderColor(state string) lipgloss.Color {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "UP", "ACTIVE":
		return lipgloss.Color("#16A34A")
	case "DOWN":
		return lipgloss.Color("#DC2626")
	case "DISABLED", "STOPPED":
		return lipgloss.Color("#64748B")
	default:
		return lipgloss.Color("#0EA5E9")
	}
}

func renderUnifiedStatusCard(title, scaleState, zebraState string, scaleLines, zebraLines []string, width int) string {
	if width < 68 {
		width = 68
	}
	inner := width - 4
	if inner < 10 {
		inner = 10
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E2E8F0"))
	lineStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#CBD5E1"))
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#93C5FD"))

	rows := make([]string, 0, len(scaleLines)+len(zebraLines)+6)
	rows = append(rows, titleStyle.Render(fitPanelLine(strings.ToUpper(strings.TrimSpace(title)), inner)))
	rows = append(rows, sectionStyle.Render(fitPanelLine("[SCALE] "+strings.ToUpper(strings.TrimSpace(scaleState)), inner)))
	for _, line := range scaleLines {
		rows = append(rows, lineStyle.Render(fitPanelLine(line, inner)))
	}
	rows = append(rows, lineStyle.Render(strings.Repeat("─", inner)))
	rows = append(rows, sectionStyle.Render(fitPanelLine("[ZEBRA] "+strings.ToUpper(strings.TrimSpace(zebraState)), inner)))
	for _, line := range zebraLines {
		rows = append(rows, lineStyle.Render(fitPanelLine(line, inner)))
	}

	// Bitta monitor panel: programma "bir butun" ko'rinsin.
	cardStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#0EA5E9")).
		Padding(0, 1)
	return cardStyle.Render(strings.Join(rows, "\n"))
}

func waitForReadingCmd(ctx context.Context, updates <-chan Reading) tea.Cmd {
	return func() tea.Msg {
		select {
		case <-ctx.Done():
			return quitMsg{}
		case upd, ok := <-updates:
			if !ok {
				return quitMsg{}
			}
			return readingMsg{reading: upd}
		}
	}
}

func waitForZebraCmd(ctx context.Context, updates <-chan ZebraStatus) tea.Cmd {
	if updates == nil {
		return nil
	}
	return func() tea.Msg {
		select {
		case <-ctx.Done():
			return quitMsg{}
		case upd, ok := <-updates:
			if !ok {
				return quitMsg{}
			}
			return zebraMsg{status: upd}
		}
	}
}

func runEncodeEPCCmd(preferredDevice string, weight *float64, unit, itemName string) tea.Cmd {
	epc := generateTestEPC(time.Now())
	return runEncodeEPCCmdWithEPC(preferredDevice, epc, weight, unit, itemName)
}

func runEncodeEPCCmdWithEPC(preferredDevice, epc string, weight *float64, unit, itemName string) tea.Cmd {
	qtyText := formatLabelQty(weight, unit)
	itemName = strings.TrimSpace(itemName)
	return func() tea.Msg {
		st := runZebraEncodeAndRead(preferredDevice, epc, qtyText, itemName, 1400*time.Millisecond)
		st.UpdatedAt = time.Now()
		return zebraMsg{status: st}
	}
}

func formatLabelQty(weight *float64, unit string) string {
	u := strings.TrimSpace(unit)
	if u == "" {
		u = "kg"
	}
	if weight == nil {
		return "- " + u
	}
	return fmt.Sprintf("%.3f %s", *weight, u)
}

func runRFIDReadCmd(preferredDevice string) tea.Cmd {
	return func() tea.Msg {
		st := runZebraRead(preferredDevice, 1400*time.Millisecond)
		st.UpdatedAt = time.Now()
		return zebraMsg{status: st}
	}
}

func zebraActionSummary(st ZebraStatus) string {
	a := strings.ToUpper(strings.TrimSpace(st.Action))
	if a == "" {
		a = "MONITOR"
	}
	if strings.TrimSpace(st.Error) != "" {
		return fmt.Sprintf("zebra %s xato: %s", strings.ToLower(a), st.Error)
	}
	if a == "ENCODE" {
		return fmt.Sprintf("zebra encode: epc=%s verify=%s line1=%s", safeText("-", st.LastEPC), safeText("UNKNOWN", st.Verify), safeText("-", st.ReadLine1))
	}
	if a == "READ" {
		return fmt.Sprintf("zebra read: verify=%s line1=%s", safeText("UNKNOWN", st.Verify), safeText("-", st.ReadLine1))
	}
	return fmt.Sprintf("zebra %s: ok", strings.ToLower(a))
}

func clockTickCmd() tea.Cmd {
	return tea.Tick(350*time.Millisecond, func(t time.Time) tea.Msg {
		return clockMsg(t)
	})
}

func isConnected(status string, last Reading, now time.Time) bool {
	if strings.TrimSpace(last.Error) != "" {
		return false
	}
	if strings.ToLower(strings.TrimSpace(status)) != "ok" {
		return false
	}
	if last.UpdatedAt.IsZero() {
		return false
	}
	if now.Sub(last.UpdatedAt) > 3*time.Second {
		return false
	}
	if strings.TrimSpace(last.Port) == "" {
		return false
	}
	return true
}

func viewSize(w, h int) (int, int) {
	if w <= 0 {
		w = 110
	}
	if h <= 0 {
		h = 30
	}

	width := w - 2
	if width > 136 {
		width = 136
	}
	if width < 68 {
		width = 68
	}
	return width, h
}

func panelWidths(total int) (int, int) {
	left := total
	right := total
	if total >= 110 {
		left = (total - 1) / 2
		right = total - left - 1
	}
	return left, right
}

func renderHeader(width int, now time.Time, scaleState, zebraState string) string {
	text := fmt.Sprintf("GSCALE-ZEBRA CONSOLE | %s | SCALE=%s | ZEBRA=%s", now.Format("2006-01-02 15:04:05"), scaleState, zebraState)
	return fitLineRaw(text, width)
}

func renderFooter(width int, info string) string {
	left := "keys: [q] quit [e] encode+print [r] read"
	text := left + " | " + strings.TrimSpace(info)
	if strings.TrimSpace(info) == "" {
		text = left
	}
	return fitLineRaw(text, width)
}

func renderUnifiedPanel(title, scaleTitle string, scaleLines []string, zebraTitle string, zebraLines []string, width int) string {
	if width < 68 {
		width = 68
	}
	inner := width - 2
	rows := make([]string, 0, len(scaleLines)+len(zebraLines)+6)
	rows = append(rows, "┌"+centerTitle(title, inner)+"┐")
	rows = append(rows, "│"+fitPanelLine("["+strings.ToUpper(strings.TrimSpace(scaleTitle))+"]", inner)+"│")
	for _, line := range scaleLines {
		rows = append(rows, "│"+fitPanelLine(line, inner)+"│")
	}
	rows = append(rows, "├"+strings.Repeat("─", inner)+"┤")
	rows = append(rows, "│"+fitPanelLine("["+strings.ToUpper(strings.TrimSpace(zebraTitle))+"]", inner)+"│")
	for _, line := range zebraLines {
		rows = append(rows, "│"+fitPanelLine(line, inner)+"│")
	}
	rows = append(rows, "└"+strings.Repeat("─", inner)+"┘")
	return strings.Join(rows, "\n")
}

func renderUnixPanel(title string, lines []string, width int) string {
	if width < 32 {
		width = 32
	}
	inner := width - 2
	rows := make([]string, 0, len(lines)+2)
	rows = append(rows, "┌"+centerTitle(title, inner)+"┐")
	for _, line := range lines {
		rows = append(rows, "│"+fitPanelLine(line, inner)+"│")
	}
	rows = append(rows, "└"+strings.Repeat("─", inner)+"┘")
	return strings.Join(rows, "\n")
}

func joinHorizontalPanels(left, right string, leftW, rightW int) string {
	lrows := strings.Split(left, "\n")
	rrows := strings.Split(right, "\n")
	n := len(lrows)
	if len(rrows) > n {
		n = len(rrows)
	}
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		l := strings.Repeat(" ", leftW)
		if i < len(lrows) {
			l = fitLineRaw(lrows[i], leftW)
		}
		r := strings.Repeat(" ", rightW)
		if i < len(rrows) {
			r = fitLineRaw(rrows[i], rightW)
		}
		out = append(out, l+" "+r)
	}
	return strings.Join(out, "\n")
}

func kv(label, value string) string {
	label = strings.ToUpper(strings.TrimSpace(label))
	value = strings.TrimSpace(value)
	if value == "" {
		value = "-"
	}
	return fmt.Sprintf("%-10s : %s", label, value)
}

func stateText(connected bool) string {
	if connected {
		return "UP"
	}
	return "DOWN"
}

func batchGateText(active bool) string {
	if active {
		return "ACTIVE"
	}
	return "STOPPED"
}

func centerTitle(title string, width int) string {
	t := " " + strings.ToUpper(strings.TrimSpace(title)) + " "
	if width <= 0 {
		return ""
	}
	if runeLen(t) > width {
		return truncateRunes(t, width)
	}
	left := (width - runeLen(t)) / 2
	right := width - runeLen(t) - left
	return strings.Repeat("─", left) + t + strings.Repeat("─", right)
}

func fitPanelLine(text string, width int) string {
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\r", " ")
	text = strings.TrimSpace(text)
	if width <= 0 {
		return ""
	}
	if runeLen(text) > width {
		text = elideMiddle(text, width)
	}
	return padRight(text, width)
}

func fitLineRaw(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if runeLen(text) > width {
		text = truncateRunes(text, width)
	}
	return padRightRaw(text, width)
}

func padRight(text string, width int) string {
	if runeLen(text) >= width {
		return text
	}
	return text + strings.Repeat(" ", width-runeLen(text))
}

func padRightRaw(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if runeLen(text) > width {
		return truncateRunes(text, width)
	}
	if runeLen(text) < width {
		text += strings.Repeat(" ", width-runeLen(text))
	}
	return text
}

func runeLen(text string) int {
	return len([]rune(text))
}

func truncateRunes(text string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(text)
	if len(r) <= max {
		return text
	}
	return string(r[:max])
}

func elideMiddle(text string, max int) string {
	runes := []rune(strings.TrimSpace(text))
	if max <= 0 {
		return ""
	}
	if len(runes) <= max {
		return string(runes)
	}
	if max <= 5 {
		return string(runes[:max])
	}
	keep := (max - 3) / 2
	left := string(runes[:keep])
	right := string(runes[len(runes)-keep:])
	return left + "..." + right
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
