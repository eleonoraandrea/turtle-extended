package tui

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/atps/atps/internal/bot"
	"github.com/atps/atps/internal/config"
)

type liveTickMsg time.Time
type liveBotMsg struct{ err error }

type LiveModel struct {
	cfg      *config.Config
	bot      *bot.Bot
	symbol   string
	variant  string
	interval string
	paper    bool

	width, height int
	spinner       spinner.Model
	viewport      viewport.Model
	ready         bool
	status        string
	autoRefresh   bool
	showHelp      bool
}

func NewLive(cfg *config.Config, b *bot.Bot, symbol, variant, interval string, paper bool) LiveModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = spinnerStyle
	vp := viewport.New(80, 12)
	vp.Style = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cardBd)
	return LiveModel{
		cfg:         cfg,
		bot:         b,
		symbol:      symbol,
		variant:     variant,
		interval:    interval,
		paper:       paper,
		spinner:     s,
		viewport:    vp,
		autoRefresh: true,
		status:      "Connesso — polling Binance klines + Orderly",
	}
}

func (m LiveModel) Init() tea.Cmd {
	// Il polling del bot è di competenza ESCLUSIVA di b.Start (rispetta --poll):
	// un secondo scheduler qui causava tick/ordini duplicati. 'r' resta manuale.
	return tea.Batch(m.spinner.Tick, m.tickCmd(), tea.EnterAltScreen)
}

func (m LiveModel) tickCmd() tea.Cmd {
	return tea.Tick(1500*time.Millisecond, func(t time.Time) tea.Msg {
		return liveTickMsg(t)
	})
}

func (m LiveModel) botTickCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		err := m.bot.Tick(ctx)
		return liveBotMsg{err: err}
	}
}

func (m LiveModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = max(80, m.width-4)
		m.viewport.Height = max(10, m.height-24)
		if !m.ready {
			m.viewport = viewport.New(m.viewport.Width, m.viewport.Height)
			m.ready = true
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "esc":
			if m.showHelp {
				m.showHelp = false
				return m, nil
			}
			return m, tea.Quit
		case "?", "h":
			m.showHelp = !m.showHelp
			return m, nil
		case "r", "R":
			m.status = "Refresh manuale — Tick..."
			return m, m.botTickCmd()
		case "p", "P":
			m.autoRefresh = !m.autoRefresh
			if m.autoRefresh {
				m.status = "Auto-refresh ON (1.5s)"
			} else {
				m.status = "Auto-refresh OFF (manuale con 'r')"
			}
			return m, nil
		case "c", "C":
			m.bot.GetLogs() // just to keep
			return m, nil
		case "d", "D":
			newDry := !m.bot.IsDryRun()
			m.bot.SetDryRun(newDry)
			if newDry {
				m.status = "✓ dry-run → PAPER (nessun ordine reale) — toggle 'd' per LIVE"
			} else {
				m.status = "⚠️ dry-run → LIVE (ordini reali!) — toggle 'd' per tornare PAPER"
			}
			return m, nil
		}
		// viewport scroll
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case liveTickMsg:
		cmds := []tea.Cmd{m.tickCmd()}
		if m.autoRefresh {
			// refresh viewport content from bot logs
			logs := m.bot.GetLogs()
			// build log view
			content := strings.Join(logs, "\n")
			m.viewport.SetContent(content)
			m.viewport.GotoBottom()
		}
		// also trigger bot tick every interval? For demo we tick every 12s via separate
		return m, tea.Batch(cmds...)
	case liveBotMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("Bot error: %v", msg.err)
		}
		// niente rescheduling: il loop di polling è di b.Start (--poll)
		return m, nil
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

type liveBotMsgWrapper struct{}

func (m LiveModel) View() string {
	if m.width == 0 {
		return "Caricamento LIVE TUI…"
	}
	if m.showHelp {
		return m.viewLiveHelp()
	}
	header := m.viewLiveHeader()
	market := m.viewLiveMarket()
	positions := m.viewLivePositions()
	signals := m.viewLiveSignals()
	logs := m.viewLiveLogs()

	// layout: header full width, then market | positions, then signals | logs
	topRow := lipgloss.JoinHorizontal(lipgloss.Top, market, positions)
	bottomRow := lipgloss.JoinHorizontal(lipgloss.Top, signals, logs)

	helpBar := helpStyle.Render("q: esci  r: refresh  p: auto ON/OFF  ?: help  •  Paper trading se live non configurato  •  Orderly: https://api.orderly.org")
	statusBar := mutedStyle.Render(m.status) + "  " + spinnerStyle.Render(m.spinner.View())

	content := lipgloss.JoinVertical(lipgloss.Left,
		header,
		topRow,
		bottomRow,
		helpBar,
		statusBar,
	)
	return lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, content)
}

func (m LiveModel) viewLiveHeader() string {
	isDry := m.bot.IsDryRun()
	mode := "LIVE"
	modeColor := lipgloss.NewStyle().Background(redCol).Foreground(lipgloss.Color("#FFFFFF")).Bold(true).Padding(0, 1)
	if isDry {
		mode = "DRY-RUN"
		modeColor = lipgloss.NewStyle().Background(greenCol).Foreground(lipgloss.Color("#052E16")).Bold(true).Padding(0, 1)
	} else if m.paper {
		mode = "PAPER"
		modeColor = lipgloss.NewStyle().Background(greenCol).Foreground(lipgloss.Color("#052E16")).Bold(true).Padding(0, 1)
	}
	bal := m.bot.GetBalance()
	eq := m.bot.GetEquity()
	// derive PnL vs initial
	initCap := m.cfg.General.InitialCapital
	pnl := eq - initCap
	pnlPct := 0.0
	if initCap > 0 {
		pnlPct = pnl / initCap * 100
	}
	pnlStyle := lipgloss.NewStyle().Foreground(greenCol).Bold(true)
	if pnl < 0 {
		pnlStyle = lipgloss.NewStyle().Foreground(redCol).Bold(true)
	}
	title := titleStyle.Render(fmt.Sprintf("◉ ATPS LIVE BOT — %s %s (%s)", m.symbol, m.variant, m.interval)) + "  " + modeColor.Render(mode)
	sub := mutedStyle.Render(fmt.Sprintf("Equity $%.2f  (%s)  •  Balance $%.2f  •  Strat: %s  •  Orderly: %s",
		eq, pnlStyle.Render(fmt.Sprintf("%+.2f (%.2f%%)", pnl, pnlPct)), bal.TotalEquity, m.bot.GetStratName(), m.bot.OrderlySymbol()))
	// right side: last update
	last := m.bot.GetLastUpdate()
	updStr := "mai"
	if !last.IsZero() {
		updStr = last.Format("15:04:05")
	}
	right := mutedStyle.Render(fmt.Sprintf("Last tick: %s  •  %s", updStr, m.cfg.Orderly.Mainnet))
	if m.paper {
		right = mutedStyle.Render(fmt.Sprintf("Last tick: %s  •  PAPER — nessun ordine reale", updStr))
	}
	headerContent := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.JoinVertical(lipgloss.Left, title, sub),
		lipgloss.NewStyle().MarginLeft(4).Render(right),
	)
	return headerStyle.Width(m.width - 4).Render(headerContent)
}

func (m LiveModel) viewLiveMarket() string {
	bars := m.bot.GetBars()
	w := 52
	content := ""
	if len(bars) == 0 {
		content = mutedStyle.Render("Warmup — caricamento barre Binance...") + "\n" +
			mutedStyle.Render("Attesa 500 barre per warmup 200 SMA")
	} else {
		last := bars[len(bars)-1]
		prev := bars[len(bars)-1]
		if len(bars) > 1 {
			prev = bars[len(bars)-2]
		}
		chg := 0.0
		if prev.Close > 0 {
			chg = (last.Close - prev.Close) / prev.Close * 100
		}
		chgStr := fmt.Sprintf("%+.2f%%", chg)
		chgStyle := lipgloss.NewStyle().Foreground(greenCol).Bold(true)
		if chg < 0 {
			chgStyle = lipgloss.NewStyle().Foreground(redCol).Bold(true)
		}
		priceStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#F8FAFC")).Bold(true).Background(lipgloss.Color("#1E293B")).Padding(0, 1).Margin(0, 1)
		// Big price
		priceBox := priceStyle.Render(fmt.Sprintf("$ %.2f  %s", last.Close, chgStyle.Render(chgStr)))
		// Market details in two columns
		leftCol := fmt.Sprintf(
			"%s %s\n%s %.2f\n%s %.2f\n%s %.2f\n%s %.0f",
			mutedStyle.Render("Symbol:"), last.Time.Format("15:04:05"),
			mutedStyle.Render("Open:"), last.Open,
			mutedStyle.Render("High:"), last.High,
			mutedStyle.Render("Low:"), last.Low,
			mutedStyle.Render("Volume:"), last.Volume,
		)
		rightCol := fmt.Sprintf(
			"%s %d\n%s %.2f\n%s %s\n%s %.4f\n%s %.0f",
			mutedStyle.Render("Bars:"), len(bars),
			mutedStyle.Render("Trades:"), float64(last.Trades),
			mutedStyle.Render("Funding:"), fmt.Sprintf("%.4f%%", last.FundingRate*100),
			mutedStyle.Render("Mark:"), last.MarkPrice,
			mutedStyle.Render("QuoteVol:"), last.QuoteVolume,
		)
		// Funding — DISABILITATO come veto (solo costo, su richiesta utente)
		fundingNote := mutedStyle.Render(fmt.Sprintf("Funding %.4f%% (solo costo, non blocca)", last.FundingRate*100))
		content = priceBox + "\n\n" +
			lipgloss.JoinHorizontal(lipgloss.Top, leftCol, "   ", rightCol) + "\n\n" +
			fundingNote + "\n" +
			mutedStyle.Render(fmt.Sprintf("Range: %s → %s  •  Interval: %s", bars[0].Time.Format("06-01-02"), last.Time.Format("06-01-02 15:04"), m.interval))
	}
	return cardStyle.Width(w + 4).Height(14).Render(titleStyle.Render("▣ MERCATO — Prezzo real-time (no grafico)") + "\n" + content)
}

func (m LiveModel) viewLivePositions() string {
	pos := m.bot.GetPositions()
	w := m.width - 60 - 8
	if w < 40 {
		w = 40
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render("◆ POSIZIONI — Orderly") + "\n")
	if len(pos) == 0 {
		b.WriteString(mutedStyle.Render("Nessuna posizione aperta — flat\n"))
		b.WriteString(mutedStyle.Render("Attesa breakout Donchian 55/20 + filtri"))
	} else {
		// header
		b.WriteString(tableHeaderStyle.Render(fmt.Sprintf("%-14s %-6s %8s %10s %10s", "Simbolo", "Side", "Qty", "Entry", "uPnL")) + "\n")
		for _, p := range pos {
			sideStyle := badgeLongStyle
			if p.Side == "SHORT" {
				sideStyle = badgeShortStyle
			}
			pnlStr := fmt.Sprintf("%+.2f", p.UnrealizedPnL)
			if p.UnrealizedPnL > 0 {
				pnlStr = successStyle.Render(pnlStr)
			} else if p.UnrealizedPnL < 0 {
				pnlStr = errorStyle.Render(pnlStr)
			}
			line := fmt.Sprintf("%-14s %-6s %8.4f %10.2f %10s",
				p.Symbol, sideStyle.Render(p.Side), p.Qty, p.EntryPrice, pnlStr)
			b.WriteString(tableCellStyle.Render(line) + "\n")
		}
	}
	b.WriteString("\n" + mutedStyle.Render(fmt.Sprintf("Adapter: %v", m.bot.IsPaper())))
	b.WriteString(mutedStyle.Render(fmt.Sprintf(" (%s)", map[bool]string{true: "paper", false: "orderly"}[m.bot.IsPaper()])))
	if !m.bot.IsPaper() {
		b.WriteString(" " + warnStyle.Render("LIVE — ordini reali"))
	}
	return cardStyle.Width(w).Height(14).Render(b.String())
}

func (m LiveModel) viewLiveSignals() string {
	sig := m.bot.GetLastSignal()
	bars := m.bot.GetBars()
	w := 52
	var b strings.Builder
	b.WriteString(titleStyle.Render("◇ PARAMETRI — tutti i filtri real-time") + "\n")
	sideStr := "HOLD"
	sideColor := mutedStyle
	if sig.Side == 1 {
		sideStr = "LONG"
		sideColor = badgeLongStyle
	} else if sig.Side == -1 {
		sideStr = "SHORT"
		sideColor = badgeShortStyle
	}
	b.WriteString(sideColor.Render(fmt.Sprintf("▶ %s", sideStr)) + "  ")
	b.WriteString(mutedStyle.Render(fmt.Sprintf("Strength %.2f", sig.Strength)) + "  ")
	b.WriteString(mutedStyle.Render("Reason: ") + sig.Reason + "\n")
	if sig.StopPrice != 0 {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("Stop: %.2f  Risk: %.2f%%", sig.StopPrice, m.bot.GetLastRiskPct())) + "\n")
	}
	// Tutti i parametri real-time (da bot.GetLastParams)
	if len(bars) > 0 {
		// Usa bot helper per ultimi indicatori
		params := m.bot.GetLastParams()
		// Mostra in griglia 2 colonne
		col1 := fmt.Sprintf(
			"%s %.2f (%.2f%%)\n%s %.2f\n%s %.2f\n%s %.0f\n%s %.2f",
			mutedStyle.Render("ATR 20:"), params.ATR, params.ATR/bars[len(bars)-1].Close*100,
			mutedStyle.Render("ADX 14:"), params.ADX,
			mutedStyle.Render("EMA50:"), params.EMA50,
			mutedStyle.Render("VolRegime:"), params.VolRegime,
			mutedStyle.Render("FundingZ:"), params.FundingZ,
		)
		// FundingZ ora solo info (veto disabilitato)
		fundingZStr := fmt.Sprintf("%.2f", params.FundingZ)
		if !math.IsNaN(params.FundingZ) {
			fundingZStr = mutedStyle.Render(fundingZStr + " (solo costo)")
		}
		// override FundingZ line with color
		// Donchian
		donH55 := params.Don55H
		donL55 := params.Don55L
		donH20 := params.Don20H
		donL20 := params.Don20L
		col2 := fmt.Sprintf(
			"%s %.2f\n%s %.2f\n%s %.2f\n%s %.2f\n%s %s",
			mutedStyle.Render("SMA200:"), params.SMA200,
			mutedStyle.Render("EMA200:"), params.EMA200,
			mutedStyle.Render("Don55 H/L:"), donH55, // will be formatted below
			mutedStyle.Render("Don20 H/L:"), donH20,
			mutedStyle.Render("OI Δ:"), fmt.Sprintf("%.2f%%", params.OIDelta*100),
		)
		// More precise Donchian + volume
		donStr := fmt.Sprintf("%s %.0f/%.0f\n%s %.0f/%.0f\n%s %.1fx\n%s %.0f",
			mutedStyle.Render("Don55:"), donH55, donL55,
			mutedStyle.Render("Don20:"), donH20, donL20,
			mutedStyle.Render("Vol mult:"), bars[len(bars)-1].Volume/(params.VolumeSMA+1),
			mutedStyle.Render("FundingZ:"), params.FundingZ,
		)
		_ = donStr
		// Instead, show compact table
		b.WriteString("\n")
		// Use lipgloss grid via strings
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Width(26).Render(col1),
			lipgloss.NewStyle().Width(26).Render(col2),
		) + "\n")
		b.WriteString(mutedStyle.Render(fmt.Sprintf("Don55: %.0f / %.0f  Don20: %.0f / %.0f  OI Δ %.2f%%  Vol %.1fx",
			donH55, donL55, donH20, donL20, params.OIDelta*100, bars[len(bars)-1].Volume/(params.VolumeSMA+1))) + "\n")
		b.WriteString(mutedStyle.Render(fmt.Sprintf("FundingZ %s  ADX %.1f  SMA200 %.0f",
			fundingZStr, params.ADX, params.SMA200)) + "\n")
		b.WriteString(mutedStyle.Render("Funding = solo costo (veto DISABILITATO su richiesta)"))
	}
	b.WriteString("\n" + mutedStyle.Render(fmt.Sprintf("Variant: %s • %s", m.variant, m.bot.GetStratName())))
	return cardStyle.Width(w + 4).Height(14).Render(b.String())
}

func (m LiveModel) viewLiveLogs() string {
	w := m.width - 60 - 8
	if w < 40 {
		w = 40
	}
	content := m.viewport.View()
	// ensure viewport has logs
	if content == "" {
		logs := m.bot.GetLogs()
		if len(logs) == 0 {
			content = mutedStyle.Render("Nessun log — bot in attesa tick...")
		} else {
			content = strings.Join(logs, "\n")
		}
	}
	return cardStyle.Width(w).Height(10).Render(titleStyle.Render("▤ LOGS — bot") + "\n" + content)
}

func (m LiveModel) viewLiveHelp() string {
	txt := `
 LIVE BOT — Guida

 Avvio:
   ./atps live --symbol BTCUSDT --variant D --interval 4h          # paper (default, sicuro)
   ./atps live --symbol BTCUSDT --variant D --live --i-understand-live  # Orderly reale (richiede env)
   Env per live: ORDERLY_ACCOUNT_ID, ORDERLY_KEY, ORDERLY_SECRET (base64 ed25519)

 Comandi TUI:
   q / esc  esci (bot continua in background se headless)
   r        tick manuale (forza fetch Binance + Orderly)
   p        toggle auto-refresh logs (1.5s)
   ?        help
   ↑/↓      scroll logs

 Flusso live:
   Binance klines (poll 30s) → AlignDerivatives (funding) → strategy.Next (Donchian 55/20, ATR 20, ADX, EMA, funding/OI)
   → risk.Size (qty = equity×2% / |entry-stop|, lev dinamica 5×, heat 3%/2%, satellite 30%)
   → execution.PlaceOrder (Orderly PERP_*_USDC, MARKET, ed25519) → WS/REST poll posizioni

 Safety (LIVE_EXECUTION_SPEC.md):
   • Paper default — senza --live non invia ordini
   • Kill-switch: touch /tmp/atps.halt → blocca ordini
   • Crash brake: DD 8% → flat 24h
   • Max notional 75k, heat 3%, leva hard 5×
   • Testnet first: --testnet

 Logica TUI: polling Binance, risk engine isolato da execution, report giornaliero in data/live/
`
	card := cardStyle.Width(min(90, m.width-6)).Render(txt)
	header := headerStyle.Render(titleStyle.Render("？ LIVE BOT — GUIDA"))
	footer := helpStyle.Render("esc: chiudi  •  q: esci")
	content := lipgloss.JoinVertical(lipgloss.Left, header, "", card, "", footer)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}
