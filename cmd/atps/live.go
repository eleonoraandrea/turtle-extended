package main

import (
	"context"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/atps/atps/internal/bot"
	"github.com/atps/atps/internal/tui"
)

func cmdLive() *cobra.Command {
	var symbol, variant, interval string
	var paper bool
	var dryRun bool
	var liveFlag, iUnderstandLive bool
	var testnet bool
	var pollSec int
	var orderlyAccount, orderlyKey, orderlySecret string
	var capital float64

	cmd := &cobra.Command{
		Use:   "live",
		Short: "Avvia bot live Orderly con TUI — paper di default, live con --live",
		Long: `Live bot ATPS su Orderly (Binance klines per segnali, Orderly per esecuzione).

Paper (sicuro, default):
  ./atps live --symbol BTCUSDT --variant D --interval 4h                    # dry-run=true (paper)
  ./atps live --dry-run=false --live --i-understand-live                    # ordini reali

Dry-run toggle (NUOVO):
  --dry-run=true   (default) → paper, nessun ordine reale, logga soltanto
  --dry-run=false  → ordini reali Orderly (richiede --live --i-understand-live + chiavi)

Live reale (richiede chiavi + flag):
  ORDERLY_ACCOUNT_ID=... ORDERLY_KEY=... ORDERLY_SECRET=... \
  ./atps live --symbol BTCUSDT --variant D --dry-run=false --live --i-understand-live

  # oppure via flags (hanno precedenza su env):
  ./atps live --orderly-account 0x... --orderly-key ed25519:... --orderly-secret base64... --dry-run=false --live --i-understand-live

Testnet:
  ./atps live --testnet --symbol BTCUSDT --variant D --dry-run=false --live --i-understand-live

Safety: paper default, kill-switch /tmp/atps.halt, heat 3%/2%, leva hard 5×, crash brake 8% → flat 24h.
Spec: docs/LIVE_EXECUTION_SPEC.md — TUI: 'd' toggle dry-run live.
`,
		Run: func(cmd *cobra.Command, args []string) {
			cfg := loadCfg(cmd.Flags().Changed("config"))
			if symbol == "" {
				symbol = cfg.General.Symbols[0]
			}
			if variant == "" {
				variant = "D"
			}
			if interval == "" {
				interval = cfg.General.Interval
			}
			// dry-run capital override (es. 10000 USD)
			if capital > 0 {
				cfg.General.InitialCapital = capital
			}
			// also allow --capital via env/config dry-run
			if cfg.General.InitialCapital == 0 {
				cfg.General.InitialCapital = 10000
			}
			// --- dry-run logic (NUOVO, esplicito) ---
			// --dry-run=true (default) → paper sempre, anche se --live presente senza conferma
			// --dry-run=false + --live + --i-understand-live + chiavi → LIVE reale
			// Compat: --paper=false → dryRun=false
			if cmd.Flags().Changed("paper") && !cmd.Flags().Changed("dry-run") {
				dryRun = paper // --paper true → dryRun true
			}
			if cmd.Flags().Changed("dry-run") {
				// dryRun esplicito ha precedenza
			} else {
				// default dryRun=true, ma se --live con conferma e --paper non toccato, rispetta dryRun default true (sicuro)
				// utente deve mettere --dry-run=false per live reale
				if paper == false && dryRun {
					// --paper=false senza --dry-run → interpreto come dryRun=false per compat
					dryRun = false
				}
			}
			// flag orderly da CLI hanno precedenza su env
			if orderlyAccount != "" {
				_ = os.Setenv("ORDERLY_ACCOUNT_ID", orderlyAccount)
			}
			if orderlyKey != "" {
				_ = os.Setenv("ORDERLY_KEY", orderlyKey)
			}
			if orderlySecret != "" {
				_ = os.Setenv("ORDERLY_SECRET", orderlySecret)
			}
			// safety: live richiede flag
			isLive := liveFlag && iUnderstandLive && !dryRun
			if liveFlag && !iUnderstandLive {
				fmt.Fprintln(os.Stderr, "⚠️  --live richiede --i-understand-live (conferma ordini reali)")
				fmt.Fprintln(os.Stderr, "   Senza flag il bot resta in PAPER (dry-run=true).")
				isLive = false
				dryRun = true
			}
			if dryRun {
				isLive = false
			}
			if isLive {
				if testnet {
					cfg.Orderly.Mainnet = cfg.Orderly.Testnet
					if cfg.Orderly.Mainnet == "" {
						cfg.Orderly.Mainnet = "https://testnet-api.orderly.org"
					}
				}
				// tutte e tre le credenziali Orderly sono obbligatorie per il live
				acc := os.Getenv("ORDERLY_ACCOUNT_ID")
				if acc == "" {
					acc = os.Getenv("ORDERLY_ACCOUNT")
				}
				if acc == "" || os.Getenv("ORDERLY_KEY") == "" || os.Getenv("ORDERLY_SECRET") == "" {
					fmt.Fprintln(os.Stderr, "⚠️  Env ORDERLY_* incomplete (ACCOUNT_ID/KEY/SECRET) → fallback PAPER (dry-run=true)")
					dryRun = true
					isLive = false
				}
				// kill switch check
				if _, err := os.Stat("/tmp/atps.halt"); err == nil {
					fmt.Fprintln(os.Stderr, "🛑 Kill-switch attivo (/tmp/atps.halt) → PAPER forzato")
					dryRun = true
					isLive = false
				}
			}
			// ENFORCEMENT FINALE del double opt-in: senza --live --i-understand-live
			// (+ --dry-run=false + chiavi + no kill-switch) il bot resta PAPER.
			// isLive calcolato ma non applicato era un bypass critico.
			if !isLive {
				dryRun = true
			}
			paper = dryRun // alias per compat con bot.New

			modeStr := "PAPER (dry-run)"
			if !dryRun {
				modeStr = "LIVE"
				if testnet {
					modeStr = "LIVE-TESTNET"
				}
			}
			fmt.Printf("Avvio bot %s %s %s [%s] dry-run=%v live=%v interval=%s poll=%ds\n", symbol, variant, interval, modeStr, dryRun, isLive, interval, pollSec)
			if dryRun {
				fmt.Println("✓ DRY-RUN attivo — nessun ordine reale verrà inviato (paper). Metti --dry-run=false --live --i-understand-live per LIVE.")
			} else {
				fmt.Println("⚠️  LIVE ATTIVO — ordini reali Orderly verranno inviati!")
			}

			b, err := bot.New(cfg, symbol, interval, variant, dryRun)
			if err != nil {
				fmt.Fprintf(os.Stderr, "bot init error: %v\n", err)
				os.Exit(1)
			}

			// If not a TTY (e.g., headless, CI), run headless loop without TUI
			if !isTTY() {
				fmt.Println("No TTY rilevato — avvio modalità headless (Ctrl-C per stop)")
				if dryRun {
					fmt.Println("DRY-RUN=true — nessun ordine reale (paper). Logs ogni tick:")
				} else {
					fmt.Println("DRY-RUN=false — LIVE ordini reali Orderly!")
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				go b.Start(ctx, time.Duration(pollSec)*time.Second)
				// stream logs to stdout
				last := 0
				ticker := time.NewTicker(1500 * time.Millisecond)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						logs := b.GetLogs()
						for i := last; i < len(logs); i++ {
							fmt.Println(logs[i])
						}
						last = len(logs)
					}
				}
			}

			// TUI mode — passa dryRun per header e toggle 'd'
			m := tui.NewLive(cfg, b, symbol, variant, interval, dryRun)
			// start bot in background
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go b.Start(ctx, time.Duration(pollSec)*time.Second)

			p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
			if _, err := p.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
				os.Exit(1)
			}
			b.Stop()
			fmt.Println("Bot fermato. Report giornaliero in data/live/ (se LIVE). Paper: nessun ordine reale inviato.")
		},
	}
	cmd.Flags().StringVar(&symbol, "symbol", "", "BTCUSDT, ETHUSDT, SOLUSDT")
	cmd.Flags().StringVar(&variant, "variant", "", "A/B/C/D (default D)")
	cmd.Flags().StringVar(&interval, "interval", "", "1h, 4h, 1d")
	cmd.Flags().BoolVar(&paper, "paper", true, "DEPRECATO: usa --dry-run (paper=true → dry-run=true)")
	_ = cmd.Flags().MarkHidden("paper")
	cmd.Flags().BoolVar(&dryRun, "dry-run", true, "true=paper (nessun ordine reale), false=ordini reali Orderly (richiede --live --i-understand-live)")
	cmd.Flags().BoolVar(&liveFlag, "live", false, "abilita ordini reali Orderly (richiede --i-understand-live + --dry-run=false)")
	cmd.Flags().BoolVar(&iUnderstandLive, "i-understand-live", false, "conferma invio ordini reali")
	cmd.Flags().BoolVar(&testnet, "testnet", false, "usa Orderly testnet")
	cmd.Flags().IntVar(&pollSec, "poll", 30, "poll interval secondi (live)")
	cmd.Flags().StringVar(&orderlyAccount, "orderly-account", "", "Orderly ACCOUNT_ID (alternativa a env ORDERLY_ACCOUNT_ID)")
	cmd.Flags().StringVar(&orderlyKey, "orderly-key", "", "Orderly KEY (ed25519 pub, alternativa a env)")
	cmd.Flags().StringVar(&orderlySecret, "orderly-secret", "", "Orderly SECRET base64 seed/priv (alternativa a env ORDERLY_SECRET)")
	cmd.Flags().Float64Var(&capital, "capital", 0, "capitale iniziale dry-run in USD (es. 10000) — default da configs/default.yaml (10000)")

	return cmd
}

func isTTY() bool {
	return isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsTerminal(os.Stderr.Fd())
}
