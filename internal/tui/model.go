package tui

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/guptarohit/asciigraph"

	"github.com/atps/atps/internal/backtest"
	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
	"github.com/atps/atps/internal/metrics"
	"github.com/atps/atps/internal/report"
	"github.com/atps/atps/internal/strategy"
)

// State
type state int

const (
	stateMenu state = iota
	stateRunning
	stateResult
	stateCompare
	stateHelp
)

// messages
type backtestDoneMsg struct {
	symbol  string
	variant string
	result  *backtest.Result
	stats   metrics.Stats
	path    string
	err     error
}

type compareDoneMsg struct {
	rows []report.ComparisonRow
	path string
	err  error
}

type Model struct {
	cfg     *config.Config
	cfgPath string

	width, height int

	state state
	// selection
	symbols   []string
	variants  []struct{ Key, Name, Desc string }
	intervals []string
	symIdx    int
	varIdx    int
	intIdx    int
	focus     int // 0 symbols,1 variants,2 intervals,3 actions
	actionIdx int // 0 Run,1 Compare,2 WalkForward,3 MonteCarlo

	// spinner
	spinner spinner.Model

	// result
	result     *backtest.Result
	stats      metrics.Stats
	reportPath string
	errMsg     string
	statusMsg  string

	// compare
	compareRows []report.ComparisonRow
	comparePath string

	// viewport for trades
	viewport viewport.Model
	ready    bool

	// last logs
	logs []string
}

func New(cfg *config.Config, cfgPath string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = spinnerStyle

	symbols := cfg.General.Symbols
	if len(symbols) == 0 {
		symbols = []string{"BTCUSDT", "ETHUSDT", "SOLUSDT"}
	}
	variants := []struct{ Key, Name, Desc string }{
		{"A", "Classic Turtle", "S1 20/55, 2ATR stop, SMA200"},
		{"B", "Regime Filter", "+ ADX, EMA 50/200, vol regime"},
		{"C", "Derivatives", "+ funding Z, OI Δ, volume"},
		{"D", "Full Adaptive", "20/55/100 adapt, vol-target, chandelier"},
	}
	intervals := []string{"1h", "4h", "1d"}
	// find current interval index
	intIdx := 1 // default 4h
	for i, v := range intervals {
		if v == cfg.General.Interval {
			intIdx = i
			break
		}
	}

	vp := viewport.New(80, 20)
	vp.Style = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cardBd)

	return Model{
		cfg: cfg, cfgPath: cfgPath,
		symbols: symbols, variants: variants, intervals: intervals,
		symIdx: 0, varIdx: 3, intIdx: intIdx, focus: 0, actionIdx: 0,
		spinner: s, viewport: vp,
		statusMsg: "Pronto — seleziona simbolo/variante e lancia backtest",
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, tea.EnterAltScreen)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// update viewport size
		m.viewport.Width = max(60, m.width-6)
		m.viewport.Height = max(10, m.height-22)
		if !m.ready {
			m.viewport = viewport.New(m.viewport.Width, m.viewport.Height)
			m.ready = true
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.state == stateResult || m.state == stateCompare {
				m.state = stateMenu
				m.errMsg = ""
				return m, nil
			}
			return m, tea.Quit
		case "tab", "right", "l":
			if m.state == stateMenu {
				m.focus = (m.focus + 1) % 4
				return m, nil
			}
		case "shift+tab", "left", "h":
			if m.state == stateMenu {
				m.focus = (m.focus - 1 + 4) % 4
				if m.focus < 0 {
					m.focus = 3
				}
				return m, nil
			}
		case "up", "k":
			if m.state == stateMenu {
				switch m.focus {
				case 0:
					m.symIdx = (m.symIdx - 1 + len(m.symbols)) % len(m.symbols)
				case 1:
					m.varIdx = (m.varIdx - 1 + len(m.variants)) % len(m.variants)
				case 2:
					m.intIdx = (m.intIdx - 1 + len(m.intervals)) % len(m.intervals)
				case 3:
					m.actionIdx = (m.actionIdx - 1 + 4) % 4
					if m.actionIdx < 0 {
						m.actionIdx = 3
					}
				}
			} else if m.state == stateResult {
				m.viewport.LineUp(1)
			}
			return m, nil
		case "down", "j":
			if m.state == stateMenu {
				switch m.focus {
				case 0:
					m.symIdx = (m.symIdx + 1) % len(m.symbols)
				case 1:
					m.varIdx = (m.varIdx + 1) % len(m.variants)
				case 2:
					m.intIdx = (m.intIdx + 1) % len(m.intervals)
				case 3:
					m.actionIdx = (m.actionIdx + 1) % 4
				}
			} else if m.state == stateResult {
				m.viewport.LineDown(1)
			}
			return m, nil
		case "enter", " ":
			if m.state == stateMenu && m.focus == 3 {
				switch m.actionIdx {
				case 0: // Run backtest
					m.state = stateRunning
					m.statusMsg = "Esecuzione backtest…"
					m.errMsg = ""
					return m, tea.Batch(m.spinner.Tick, m.runBacktestCmd(m.symbols[m.symIdx], m.variants[m.varIdx].Key, m.intervals[m.intIdx]))
				case 1: // Compare
					m.state = stateRunning
					m.statusMsg = "Confronto A/B/C/D…"
					return m, tea.Batch(m.spinner.Tick, m.runCompareCmd())
				case 2: // WalkForward
					m.state = stateRunning
					m.statusMsg = "Walk-forward…"
					return m, tea.Batch(m.spinner.Tick, m.runBacktestCmd(m.symbols[m.symIdx], m.variants[m.varIdx].Key, m.intervals[m.intIdx])) // fallback to backtest for now, later could expand
				case 3: // Help
					m.state = stateHelp
					return m, nil
				}
			} else if m.state == stateMenu && m.focus != 3 {
				// quick run on enter on selector
				m.state = stateRunning
				return m, tea.Batch(m.spinner.Tick, m.runBacktestCmd(m.symbols[m.symIdx], m.variants[m.varIdx].Key, m.intervals[m.intIdx]))
			} else if m.state == stateHelp {
				m.state = stateMenu
				return m, nil
			} else if m.state == stateResult {
				// open report?
				if m.reportPath != "" {
					m.statusMsg = fmt.Sprintf("Report: %s", m.reportPath)
				}
				return m, nil
			}
		case "r":
			if m.state == stateMenu {
				m.state = stateRunning
				return m, tea.Batch(m.spinner.Tick, m.runBacktestCmd(m.symbols[m.symIdx], m.variants[m.varIdx].Key, m.intervals[m.intIdx]))
			}
			if m.state == stateResult {
				// re-run
				m.state = stateRunning
				return m, tea.Batch(m.spinner.Tick, m.runBacktestCmd(m.symbols[m.symIdx], m.variants[m.varIdx].Key, m.intervals[m.intIdx]))
			}
		case "c":
			if m.state == stateMenu {
				m.state = stateRunning
				return m, tea.Batch(m.spinner.Tick, m.runCompareCmd())
			}
		case "o":
			if m.state == stateResult && m.reportPath != "" {
				// try to open with xdg-open
				// non-blocking, just set status
				m.statusMsg = fmt.Sprintf("Apri: xdg-open %s  (o open in browser)", m.reportPath)
				return m, nil
			}
		case "esc":
			if m.state == stateResult || m.state == stateCompare || m.state == stateHelp {
				m.state = stateMenu
				return m, nil
			}
		case "?":
			if m.state == stateMenu {
				m.state = stateHelp
				return m, nil
			}
			if m.state == stateHelp {
				m.state = stateMenu
				return m, nil
			}
		}
		// passthrough to viewport when result
		if m.state == stateResult {
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.state == stateRunning {
			return m, cmd
		}
		return m, nil

	case backtestDoneMsg:
		m.state = stateResult
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			m.statusMsg = "Errore backtest"
		} else {
			m.result = msg.result
			m.stats = msg.stats
			m.reportPath = msg.path
			m.statusMsg = fmt.Sprintf("✓ %s %s → %s  (%d trades)", msg.symbol, msg.variant, msg.path, msg.stats.Trades)
			// build viewport content
			content := m.buildResultViewport()
			m.viewport.SetContent(content)
			m.viewport.GotoTop()
			m.logs = append(m.logs, fmt.Sprintf("%s %s Return %.2f%% Sharpe %.2f", msg.symbol, msg.variant, msg.stats.ReturnPct, msg.stats.Sharpe))
		}
		return m, nil

	case compareDoneMsg:
		m.state = stateCompare
		if msg.err != nil {
			m.errMsg = msg.err.Error()
		} else {
			m.compareRows = msg.rows
			m.comparePath = msg.path
			m.statusMsg = fmt.Sprintf("✓ Comparison → %s", msg.path)
		}
		return m, nil
	}

	// propagate viewport update
	if m.state == stateResult {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) View() string {
	if m.width == 0 {
		return "Caricamento TUI…"
	}
	switch m.state {
	case stateMenu:
		return m.viewMenu()
	case stateRunning:
		return m.viewRunning()
	case stateResult:
		return m.viewResult()
	case stateCompare:
		return m.viewCompare()
	case stateHelp:
		return m.viewHelp()
	}
	return m.viewMenu()
}

// Helpers
func (m Model) viewMenu() string {
	// header
	logo := `
 █████╗ ████████╗██████╗ ███████╗
██╔══██╗╚══██╔══╝██╔══██╗██╔════╝
███████║   ██║   ██████╔╝███████╗
██╔══██║   ██║   ██╔═══╝ ╚════██║
██║  ██║   ██║   ██║     ███████║
╚═╝  ╚═╝   ╚═╝   ╚═╝     ╚══════╝
`
	logoBox := logoStyle.Render(logo)
	header := headerStyle.Render(lipgloss.JoinHorizontal(lipgloss.Top,
		logoBox,
		"  "+titleStyle.Render("Adaptive Turtle Perpetual System")+"\n"+
			"  "+subtitleStyle.Render("Binance dati  •  Orderly esecuzione  •  Go 1.24  •  Report MT5++")+"\n"+
			"  "+mutedStyle.Render(fmt.Sprintf("Config: %s  •  %dx%d", m.cfgPath, m.width, m.height)),
	))
	// panels
	symPanel := m.renderSymbols()
	varPanel := m.renderVariants()
	intervalPanel := m.renderIntervals()
	actionPanel := m.renderActions()

	topRow := lipgloss.JoinHorizontal(lipgloss.Top, symPanel, varPanel, intervalPanel)
	bottomRow := actionPanel
	statusBar := helpStyle.Render("Tab/Shift-Tab: focus  ↑/↓: seleziona  Enter: esegui  r: Run  c: Compare  ?: Help  q: Esci") +
		"   " + mutedStyle.Render(m.statusMsg)
	if m.errMsg != "" {
		statusBar += "  " + errorStyle.Render(m.errMsg)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, header, "", topRow, "", bottomRow, "", statusBar)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceBackground(bg),
	)
}

func (m Model) renderSymbols() string {
	style := focusCard(m.focus == 0)
	var b strings.Builder
	b.WriteString(titleStyle.Render("▣ SIMBOLO") + "\n")
	b.WriteString(mutedStyle.Render("Binance → Orderly") + "\n\n")
	for i, s := range m.symbols {
		cursor := "  "
		itemStyle := normalItemStyle
		if i == m.symIdx {
			cursor = "▶ "
			if m.focus == 0 {
				itemStyle = selectedStyle
			} else {
				itemStyle = lipgloss.NewStyle().Foreground(accentCol).Bold(true).Padding(0, 1)
			}
		}
		icon := "●"
		switch s {
		case "BTCUSDT":
			icon = "₿"
		case "ETHUSDT":
			icon = "♦"
		case "SOLUSDT":
			icon = "◎"
		}
		line := fmt.Sprintf("%s%s %s", cursor, icon, s)
		b.WriteString(itemStyle.Render(line) + "\n")
		// orderly mapping
		if i == m.symIdx {
			ord := m.cfg.Orderly.SymbolsMap[s]
			if ord == "" {
				ord = strings.Replace(s, "USDT", "_USDC", 1)
				ord = "PERP_" + ord
			}
			b.WriteString(mutedStyle.Render("  ↳ "+ord) + "\n")
		}
	}
	b.WriteString("\n" + mutedStyle.Render(fmt.Sprintf("%d simboli", len(m.symbols))))
	return style.Width(28).Height(14).Render(b.String())
}

func (m Model) renderVariants() string {
	style := focusCard(m.focus == 1)
	var b strings.Builder
	b.WriteString(titleStyle.Render("◆ VARIANTE") + "\n")
	b.WriteString(mutedStyle.Render("Turtle A → D full") + "\n\n")
	for i, v := range m.variants {
		cursor := "  "
		itemStyle := normalItemStyle
		if i == m.varIdx {
			cursor = "▶ "
			if m.focus == 1 {
				itemStyle = selectedStyle
			} else {
				itemStyle = lipgloss.NewStyle().Foreground(purpleCol).Bold(true).Padding(0, 1)
			}
		}
		b.WriteString(itemStyle.Render(fmt.Sprintf("%s[%s] %s", cursor, v.Key, v.Name)) + "\n")
		if i == m.varIdx {
			b.WriteString(mutedStyle.Render("  "+v.Desc) + "\n")
		}
	}
	b.WriteString("\n" + mutedStyle.Render("4 varianti • D = adaptive 20/55/100"))
	return style.Width(38).Height(14).Render(b.String())
}

func (m Model) renderIntervals() string {
	style := focusCard(m.focus == 2)
	var b strings.Builder
	b.WriteString(titleStyle.Render("◇ TIMEFRAME") + "\n")
	b.WriteString(mutedStyle.Render("OHLCV interval") + "\n\n")
	for i, iv := range m.intervals {
		cursor := "  "
		itemStyle := normalItemStyle
		if i == m.intIdx {
			cursor = "▶ "
			if m.focus == 2 {
				itemStyle = selectedStyle
			} else {
				itemStyle = lipgloss.NewStyle().Foreground(accent2).Bold(true).Padding(0, 1)
			}
		}
		b.WriteString(itemStyle.Render(fmt.Sprintf("%s%s", cursor, iv)) + "\n")
	}
	b.WriteString("\n")
	// show costs
	b.WriteString(mutedStyle.Render(fmt.Sprintf("Fee %.1f bps", m.cfg.Costs.FeeBps)) + "\n")
	b.WriteString(mutedStyle.Render(fmt.Sprintf("Slipp %.1f bps", m.cfg.Costs.SlippageBps)) + "\n")
	b.WriteString(mutedStyle.Render(fmt.Sprintf("Cap $%.0f", m.cfg.General.InitialCapital)) + "\n")
	return style.Width(22).Height(14).Render(b.String())
}

func (m Model) renderActions() string {
	style := focusCard(m.focus == 3)
	var b strings.Builder
	b.WriteString(titleStyle.Render("▶ AZIONI") + "  " + mutedStyle.Render("Enter per eseguire") + "\n\n")
	actions := []struct{ Key, Label, Hint string }{
		{"r", "▶ Run Backtest", "HTML dettagliato"},
		{"c", "▣ Compare A/B/C/D", "ranking Sharpe"},
		{"w", "◈ Walk-Forward", "8 folds (todo)"},
		{"?", "？ Help", "guida rapida"},
	}
	for i, a := range actions {
		isSel := i == m.actionIdx
		styleBtn := buttonInactiveStyle
		if isSel && m.focus == 3 {
			styleBtn = buttonFocusStyle
		} else if isSel {
			styleBtn = buttonStyle
		}
		// highlight key
		label := fmt.Sprintf("[%s] %s — %s", a.Key, a.Label, a.Hint)
		b.WriteString(styleBtn.Render(label) + "\n")
	}
	b.WriteString("\n" + mutedStyle.Render("Focus 3/4 → Tab per azioni."))
	return style.Width(92).Render(b.String())
}

func (m Model) viewRunning() string {
	header := headerStyle.Render(titleStyle.Render("⏳ ESECUZIONE IN CORSO") + "  " + m.spinner.View() + "  " + mutedStyle.Render(m.statusMsg))
	body := cardStyle.Width(m.width - 6).Height(10).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			spinnerStyle.Render(m.spinner.View()+"  Elaborazione backtest…"),
			"",
			mutedStyle.Render(fmt.Sprintf("Simbolo: %s  Variante: %s  Interval: %s", m.symbols[m.symIdx], m.variants[m.varIdx].Key, m.intervals[m.intIdx])),
			mutedStyle.Render("Engine: next-open fill, fee/slippage/funding, pyramiding, chandelier trail"),
			"",
			warnStyle.Render("Non chiudere — il report HTML si sta generando…"),
		),
	)
	footer := helpStyle.Render("q: annulla  •  esc: menu")
	content := lipgloss.JoinVertical(lipgloss.Left, header, "", body, "", footer)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

func (m Model) viewResult() string {
	if m.result == nil {
		return m.viewMenu()
	}
	// KPI row
	retCol := greenCol
	if m.stats.ReturnPct < 0 {
		retCol = redCol
	}
	retStyle := lipgloss.NewStyle().Foreground(retCol).Bold(true).Align(lipgloss.Center).Width(18)

	kpi1 := kpiCard("RETURN", fmt.Sprintf("%.2f%%", m.stats.ReturnPct), fmt.Sprintf("CAGR %.2f%% BH %.1f%%", m.stats.ReturnAnnual, m.stats.BuyHoldReturn), m.stats.ReturnPct > 0)
	kpi2 := kpiCard("SHARPE", fmt.Sprintf("%.2f", m.stats.Sharpe), fmt.Sprintf("Sort %.2f Cal %.2f", m.stats.Sortino, m.stats.Calmar), m.stats.Sharpe > 1)
	kpi3 := kpiCard("MAX DD", fmt.Sprintf("%.2f%%", m.stats.MaxDD), fmt.Sprintf("%d bars  Ulcer %.1f", m.stats.MaxDDDurationBars, m.stats.UlcerIndex), false)
	kpi4 := kpiCard("PF / WIN", fmt.Sprintf("%.2f / %.1f%%", m.stats.ProfitFactor, m.stats.WinRate), fmt.Sprintf("%d trades  Exp %.0f%%", m.stats.Trades, m.stats.ExposurePct), m.stats.ProfitFactor > 1.5)

	kpiRow := lipgloss.JoinHorizontal(lipgloss.Top,
		cardStyle.Render(kpi1),
		cardStyle.Render(kpi2),
		cardStyle.Render(kpi3),
		cardStyle.Render(kpi4),
	)
	// also second row small KPIs
	kpi5 := kpiCard("FEES", fmt.Sprintf("$%.0f", m.stats.TotalFee), fmt.Sprintf("%.1f%% drag", m.stats.FeeDragPct), false)
	kpi6 := kpiCard("FUNDING", fmt.Sprintf("$%.2f", m.stats.TotalFunding), fmt.Sprintf("%.1f%% drag", m.stats.FundingDragPct), m.stats.TotalFunding < 5)
	kpi7 := kpiCard("SQN / KELLY", fmt.Sprintf("%.2f / %.1f%%", m.stats.SQN, m.stats.KellyPct), fmt.Sprintf("Payoff %.2f", m.stats.PayoffRatio), m.stats.SQN > 1.6)
	kpi8 := kpiCard("FINAL", fmt.Sprintf("$%.0f", m.stats.FinalEquity), fmt.Sprintf("da $%.0f", m.stats.InitialCapital), m.stats.FinalEquity > m.stats.InitialCapital)

	kpiRow2 := lipgloss.JoinHorizontal(lipgloss.Top,
		cardStyle.Render(kpi5),
		cardStyle.Render(kpi6),
		cardStyle.Render(kpi7),
		cardStyle.Render(retStyle.Render(fmt.Sprintf("$%.0f", m.stats.FinalEquity))+"\n"+mutedStyle.Align(lipgloss.Center).Width(18).Render(fmt.Sprintf("da $%.0f", m.stats.InitialCapital))),
	)
	_ = kpi8

	// Equity sparkline
	eqVals := make([]float64, len(m.result.Equity))
	for i, e := range m.result.Equity {
		eqVals[i] = e.Equity
	}
	spark := ""
	if len(eqVals) > 2 {
		spark = asciigraph.Plot(eqVals,
			asciigraph.Height(8),
			asciigraph.Width(60),
			asciigraph.Caption("Equity net (fee+funding incl)"),
			asciigraph.Precision(0),
		)
		spark = mutedStyle.Render(spark)
	}
	// Drawdown sparkline small
	ddVals := make([]float64, len(m.result.Equity))
	for i, e := range m.result.Equity {
		ddVals[i] = e.Drawdown
		if ddVals[i] > 0 {
			ddVals[i] = 0
		}
	}
	ddSpark := ""
	if len(ddVals) > 2 {
		// invert for visual: show as positive drawdown depth
		for i, v := range ddVals {
			ddVals[i] = -v
		}
		ddSpark = asciigraph.Plot(ddVals, asciigraph.Height(4), asciigraph.Width(60), asciigraph.Caption("Drawdown % (depth)"), asciigraph.Precision(1))
	}

	charts := lipgloss.JoinHorizontal(lipgloss.Top,
		cardStyle.Width(62).Render(spark),
		cardStyle.Width(38).Render(
			ddSpark+"\n\n"+
				mutedStyle.Render(fmt.Sprintf("Trades: %d  Winners %d  Losers %d\nBest $%.0f  Worst $%.0f\nAvg $%.0f  Bars %.1f\nExposure %.1f%%  Vol %.1f%%",
					m.stats.Trades, m.stats.Winners, m.stats.Losers, m.stats.BestTrade, m.stats.WorstTrade, m.stats.AvgTrade, m.stats.AvgBarsHeld, m.stats.ExposurePct, m.stats.VolatilityAnn))+"\n"+
				lipgloss.NewStyle().Foreground(yellowCol).Render(fmt.Sprintf("Lev avg %.2fx max %.2fx (cap %.1fx)\nRisk/trade avg %.2f%% max %.2f%%\nHeat max %.2f%% (lim %.1f%%)",
					m.result.AvgLeverage, m.result.MaxLeverageUsed, m.result.RiskLimitsUsed.MaxLeverage,
					m.result.AvgRiskPct, m.result.MaxRiskPctUsed, m.result.MaxHeatSeen, m.result.RiskLimitsUsed.MaxHeatPct))),
	)

	// Report path + actions
	reportBox := cardStyle.Render(
		successStyle.Render("✓ Report HTML generato") + "\n" +
			mutedStyle.Render(m.reportPath) + "\n\n" +
			buttonStyle.Render("[o] Apri in browser") + "  " +
			buttonInactiveStyle.Render("[r] Re-run") + "  " +
			buttonInactiveStyle.Render("[esc] Menu"),
	)

	// Trades table via viewport
	// viewport already contains trades
	tradesCard := cardFocusStyle.Width(m.viewport.Width + 2).Render(
		titleStyle.Render("▤ TRADES DETTAGLIATI (scroll ↑/↓,  MT5 style)") + "\n" +
			m.viewport.View() +
			"\n" + mutedStyle.Render(fmt.Sprintf("Mostrati %d/%d  •  Fee $%.0f  Funding $%.2f  •  Gross $%.0f → Net $%.0f", len(m.result.Trades), m.stats.Trades, m.stats.TotalFee, m.stats.TotalFunding, m.stats.GrossPnL, m.stats.NetPnL)),
	)

	header := headerStyle.Render(
		lipgloss.JoinHorizontal(lipgloss.Top,
			titleStyle.Render(fmt.Sprintf("◉ RISULTATO %s %s", m.symbols[m.symIdx], m.variants[m.varIdx].Key))+"  ",
			badgeForVariant(m.variants[m.varIdx].Key)+"  ",
			mutedStyle.Render(fmt.Sprintf("%s → %s  •  %d barre  •  %s", m.stats.Start.Format("2006-01-02"), m.stats.End.Format("2006-01-02"), len(m.result.Bars), m.reportPath)),
		),
	)

	helpBar := helpStyle.Render("↑/↓: scroll trades  r: re-run  o: apri HTML  esc: menu  q: esci  •  Report già salvato in reports/")

	content := lipgloss.JoinVertical(lipgloss.Left,
		header,
		"",
		kpiRow,
		kpiRow2,
		"",
		charts,
		"",
		reportBox,
		"",
		tradesCard,
		"",
		helpBar,
	)

	// place with scrolling if needed
	return lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, content)
}

func (m Model) viewCompare() string {
	header := headerStyle.Render(titleStyle.Render("▣ COMPARISON A/B/C/D — Rank per Sharpe") + "  " + mutedStyle.Render(m.comparePath))
	var b strings.Builder
	// table header
	hdr := lipgloss.JoinHorizontal(lipgloss.Top,
		tableHeaderStyle.Width(6).Render("Rank"),
		tableHeaderStyle.Width(8).Render("Var"),
		tableHeaderStyle.Width(10).Render("Symbol"),
		tableHeaderStyle.Width(10).Render("Return"),
		tableHeaderStyle.Width(10).Render("CAGR"),
		tableHeaderStyle.Width(8).Render("Sharpe"),
		tableHeaderStyle.Width(8).Render("Sort"),
		tableHeaderStyle.Width(10).Render("MaxDD"),
		tableHeaderStyle.Width(8).Render("Win%"),
		tableHeaderStyle.Width(7).Render("PF"),
		tableHeaderStyle.Width(7).Render("Trd"),
	)
	b.WriteString(hdr + "\n")
	for i, r := range m.compareRows {
		style := tableCellStyle
		if i == 0 {
			style = style.Background(lipgloss.Color("#052E16")).Foreground(greenCol)
		}
		retCol := style.Foreground(greenCol)
		if r.Stats.ReturnPct < 0 {
			retCol = style.Foreground(redCol)
		}
		row := lipgloss.JoinHorizontal(lipgloss.Top,
			style.Width(6).Render(fmt.Sprintf("%d", i)),
			style.Width(8).Render(r.Variant),
			style.Width(10).Render(r.Symbol),
			retCol.Width(10).Render(fmt.Sprintf("%.1f%%", r.Stats.ReturnPct)),
			style.Width(10).Render(fmt.Sprintf("%.1f%%", r.Stats.ReturnAnnual)),
			style.Width(8).Render(fmt.Sprintf("%.2f", r.Stats.Sharpe)),
			style.Width(8).Render(fmt.Sprintf("%.2f", r.Stats.Sortino)),
			style.Width(10).Render(fmt.Sprintf("%.1f%%", r.Stats.MaxDD)),
			style.Width(8).Render(fmt.Sprintf("%.1f%%", r.Stats.WinRate)),
			style.Width(7).Render(fmt.Sprintf("%.2f", r.Stats.ProfitFactor)),
			style.Width(7).Render(fmt.Sprintf("%d", r.Stats.Trades)),
		)
		b.WriteString(row + "\n")
	}
	table := cardStyle.Width(m.width - 8).Render(b.String())
	footer := helpStyle.Render("esc: menu  •  report: " + m.comparePath + "  •  Sharpe desc")
	content := lipgloss.JoinVertical(lipgloss.Left, header, "", table, "", footer)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Top, content)
}

func (m Model) viewHelp() string {
	helpTxt := `
 ATPS — Guida TUI

 Navigazione
   Tab / Shift-Tab / ←/→   cambia focus (Simbolo / Variante / Timeframe / Azioni)
   ↑/↓                    seleziona voce
   Enter / Space          esegui azione o lancia backtest rapido
   r                      Run backtest (menu o result)
   c                      Compare A/B/C/D
   o                      Apri report HTML (quando in result)
   esc                    torna al menu
   q / Ctrl-C             esci (da menu esce, da result torna a menu)

 Simboli
   BTCUSDT → PERP_BTC_USDC  (Binance klines + funding/OI)
   ETHUSDT → PERP_ETH_USDC
   SOLUSDT → PERP_SOL_USDC
   Dati: GET /fapi/v1/klines, /fapi/v1/fundingRate, /futures/data/openInterestHist
   Se CSV mancante in data/raw/ → demo sintetico (seed diverso per simbolo)

 Varianti
   A Classic   Donchian 20/55, 2×ATR stop, SMA200
   B Regime    + ADX 14>18, EMA 50/200, vol regime 100
   C Derivs    + funding Z 2.0, OI delta, volume 1.2×SMA20
   D Full      20/55/100 adattivo, vol-target 20%, Kelly cap, chandelier trail,

 Report HTML
   Self-contained, offline file:// ok.
   Include: equity vs BH, drawdown, histogram PnL, 32 metriche, monthly/yearly heatmap,
            regime LONG/SHORT, costi (fee+funding), trades table MT5 con MAE/MFE/R.

 Live (Orderly)
   Backtester MAI importa execution. Solo: go build -tags live -o atps-live
   Testnet: https://testnet-api.orderly.org  Mainnet: https://api.orderly.org
   Symbol PERP_*_USDC, ed25519 sign. Paper di default, flag --i-understand-live per reale.

 Suggerimento: dopo il backtest premi 'o' e apri il path reports/...html nel browser.
`
	card := cardStyle.Width(min(90, m.width-6)).Render(helpTxt)
	header := headerStyle.Render(titleStyle.Render("？ GUIDA TUI — ATPS"))
	footer := helpStyle.Render("esc / ?: chiudi  •  q: esci")
	content := lipgloss.JoinVertical(lipgloss.Left, header, "", card, "", footer)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

// builders
func (m Model) buildResultViewport() string {
	// header trades
	var b strings.Builder
	// use monospaced table
	hdr := fmt.Sprintf("%-3s %-6s %-16s %-16s %8s %8s %6s %4s %6s %4s %6s %6s %6s %6s %8s %6s %s\n",
		"#", "Side", "Entry", "Exit", "E Price", "X Price", "Qty", "Lev", "Risk%", "Bars", "MAE", "MFE", "Fee", "Fund", "PnL", "R", "Reason")
	b.WriteString(tableHeaderStyle.Render(hdr) + "\n")
	limit := len(m.result.Trades)
	if limit > 500 {
		limit = 500
	}
	for i := 0; i < limit; i++ {
		t := m.result.Trades[i]
		side := "LONG"
		badge := badgeLongStyle
		if t.Side == -1 {
			side = "SHORT"
			badge = badgeShortStyle
		}
		pnlCol := greenCol
		if t.PnLNet < 0 {
			pnlCol = redCol
		}
		pnlStr := "" // kept for compatibility, not used in viewport plain text
		_ = pnlStr
		_ = badge
		_ = pnlCol
		plain := fmt.Sprintf("%-3d %-6s %-16s %-16s %8.2f %8.2f %6.3f %4.1fx %5.2f%% %4d %6.1f %6.1f %6.0f %6.1f %8.0f %5.2fR %s",
			i, side, t.EntryTime.Format("06-01-02 15:04"), t.ExitTime.Format("06-01-02 15:04"), t.EntryPrice, t.ExitPrice, t.Qty, t.Leverage, t.RiskPct, t.BarsHeld, t.MAE, t.MFE, t.Fee, t.FundingCost, t.PnLNet, t.RMultiple, t.ExitReason)
		b.WriteString(plain + "\n")
	}
	// footer summary
	b.WriteString("\n" + mutedStyle.Render(fmt.Sprintf("Totale %d trades — mostra ultimi %d", len(m.result.Trades), limit)))
	return b.String()
}

// Cmds

func (m Model) runBacktestCmd(symbol, variant, interval string) tea.Cmd {
	return func() tea.Msg {
		// load config fresh
		cfg := m.cfg
		// override interval temporaneamente
		cfgCopy := *cfg
		cfgCopy.General.Interval = interval
		// locate bars
		csvPath := fmt.Sprintf("data/raw/%s_%s.csv", symbol, interval)
		var bars data.Bars
		if _, err := os.Stat(csvPath); err == nil {
			b, err := data.LoadBarsCSV(csvPath)
			if err != nil {
				return backtestDoneMsg{err: fmt.Errorf("load csv %w", err)}
			}
			bars = b
		} else {
			seedMap := map[string]int64{"BTCUSDT": 42, "ETHUSDT": 1337, "SOLUSDT": 9999}
			seed := seedMap[symbol]
			if seed == 0 {
				seed = 42
			}
			bars = data.GenerateSynthetic(3000, intervalToDuration(interval), seed)
		}
		strat := strategy.New(variant, &cfgCopy)
		trailMode := "donchian"
		if variant == "D" {
			trailMode = cfgCopy.VariantD.TrailMode
		}
		eng := backtest.EngineConfig{
			Variant: variant, Symbol: symbol,
			InitialCapital: cfgCopy.General.InitialCapital,
			FeeBps:         cfgCopy.Costs.FeeBps, SlippageBps: cfgCopy.Costs.SlippageBps,
			Leverage:       cfgCopy.Costs.Leverage,
			UseNextOpen:    cfgCopy.Backtest.UseNextOpenFill,
			PyramidingMax:  cfgCopy.Backtest.PyramidingMaxUnits,
			PyramidStepATR: cfgCopy.Backtest.PyramidStepATR,
			TrailATRMult:   cfgCopy.Backtest.TrailATRMult,
			TrailMode:      trailMode,
			DonExit:        20,
		}
		res := backtest.Run(bars, strat, &cfgCopy, eng)
		stats := metrics.Compute(res)
		path := fmt.Sprintf("reports/TUI_%s_%s_%s.html", symbol, variant, time.Now().Format("20060102_150405"))
		// ensure dir
		_ = os.MkdirAll(filepath.Dir(path), 0755)
		in := report.Input{Config: &cfgCopy, Bars: bars, Result: res, Stats: stats, Symbol: symbol, Variant: variant, GeneratedAt: time.Now()}
		if err := report.Generate(path, in); err != nil {
			return backtestDoneMsg{err: err}
		}
		// also save trades json
		return backtestDoneMsg{symbol: symbol, variant: variant, result: res, stats: stats, path: path}
	}
}

func (m Model) runCompareCmd() tea.Cmd {
	return func() tea.Msg {
		cfg := m.cfg
		syms := cfg.General.Symbols
		if len(syms) == 0 {
			syms = []string{"BTCUSDT", "ETHUSDT", "SOLUSDT"}
		}
		var rows []report.ComparisonRow
		for _, sym := range syms {
			csvPath := fmt.Sprintf("data/raw/%s_%s.csv", sym, cfg.General.Interval)
			var bars data.Bars
			if _, err := os.Stat(csvPath); err == nil {
				b, _ := data.LoadBarsCSV(csvPath)
				bars = b
			} else {
				seedMap := map[string]int64{"BTCUSDT": 42, "ETHUSDT": 1337, "SOLUSDT": 9999}
				seed := seedMap[sym]
				if seed == 0 {
					seed = 42
				}
				bars = data.GenerateSynthetic(3000, 4*time.Hour, seed)
			}
			for _, v := range []string{"A", "B", "C", "D"} {
				strat := strategy.New(v, cfg)
				trailMode := "donchian"
				if v == "D" {
					trailMode = cfg.VariantD.TrailMode
				}
				eng := backtest.EngineConfig{Variant: v, Symbol: sym, InitialCapital: cfg.General.InitialCapital, FeeBps: cfg.Costs.FeeBps, SlippageBps: cfg.Costs.SlippageBps, Leverage: cfg.Costs.Leverage, UseNextOpen: true, PyramidingMax: cfg.Backtest.PyramidingMaxUnits, PyramidStepATR: cfg.Backtest.PyramidStepATR, TrailATRMult: cfg.Backtest.TrailATRMult, TrailMode: trailMode, DonExit: 20}
				res := backtest.Run(bars, strat, cfg, eng)
				stats := metrics.Compute(res)
				rows = append(rows, report.ComparisonRow{Symbol: sym, Variant: v, Stats: stats})
				// also generate individual html silently
				path := fmt.Sprintf("reports/%s_%s.html", sym, v)
				_ = report.Generate(path, report.Input{Config: cfg, Bars: bars, Result: res, Stats: stats, Symbol: sym, Variant: v, GeneratedAt: time.Now()})
			}
		}
		out := "reports/TUI_comparison.html"
		if err := report.GenerateComparison(out, rows, cfg); err != nil {
			return compareDoneMsg{err: err}
		}
		return compareDoneMsg{rows: rows, path: out}
	}
}

// helpers
func intervalToDuration(s string) time.Duration {
	switch s {
	case "1m":
		return time.Minute
	case "5m":
		return 5 * time.Minute
	case "15m":
		return 15 * time.Minute
	case "1h":
		return time.Hour
	case "4h":
		return 4 * time.Hour
	case "1d":
		return 24 * time.Hour
	}
	return 4 * time.Hour
}
func badgeForVariant(v string) string {
	switch v {
	case "A":
		return badgeLongStyle.Render("A Classic")
	case "B":
		return lipgloss.NewStyle().Background(lipgloss.Color("#1E3A8A")).Foreground(lipgloss.Color("#93C5FD")).Padding(0, 1).Render("B Regime")
	case "C":
		return lipgloss.NewStyle().Background(lipgloss.Color("#064E3B")).Foreground(lipgloss.Color("#6EE7B7")).Padding(0, 1).Render("C Deriv")
	case "D":
		return badgeShortStyle.Background(lipgloss.Color("#4C1D95")).Foreground(lipgloss.Color("#DDD6FE")).Render("D Full")
	}
	return v
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func isNaN(f float64) bool { return math.IsNaN(f) }
