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
	var liveFlag, iUnderstandLive bool
	var testnet bool
	var pollSec int

	cmd := &cobra.Command{
		Use:   "live",
		Short: "Avvia bot live Orderly con TUI — paper di default, live con --live",
		Long: `Live bot ATPS su Orderly (Binance klines per segnali, Orderly per esecuzione).

Paper (sicuro, default):
  ./atps live --symbol BTCUSDT --variant D --interval 4h

Live reale (richiede env + flag):
  ORDERLY_ACCOUNT_ID=... ORDERLY_KEY=... ORDERLY_SECRET=... \
  ./atps live --symbol BTCUSDT --variant D --live --i-understand-live

Testnet:
  ./atps live --testnet --symbol BTCUSDT --variant D --live --i-understand-live

Safety: paper default, kill-switch /tmp/atps.halt, heat 3%/2%, leva hard 5×, crash brake 8% → flat 24h.
Spec: docs/LIVE_EXECUTION_SPEC.md
`,
		Run: func(cmd *cobra.Command, args []string) {
			cfg := loadCfg()
			if symbol == "" {
				symbol = cfg.General.Symbols[0]
			}
			if variant == "" {
				variant = "D"
			}
			if interval == "" {
				interval = cfg.General.Interval
			}
			// safety: live requires flag
			isLive := liveFlag && iUnderstandLive
			if liveFlag && !iUnderstandLive {
				fmt.Fprintln(os.Stderr, "⚠️  --live richiede --i-understand-live (conferma ordini reali)")
				fmt.Fprintln(os.Stderr, "   Senza flag il bot resta in PAPER (sicuro).")
				isLive = false
				paper = true
			}
			if isLive {
				paper = false
				if testnet {
					cfg.Orderly.Mainnet = cfg.Orderly.Testnet
					if cfg.Orderly.Mainnet == "" {
						cfg.Orderly.Mainnet = "https://testnet-api.orderly.org"
					}
				}
				// also check env
				if os.Getenv("ORDERLY_SECRET") == "" && os.Getenv("ORDERLY_KEY") == "" {
					fmt.Fprintln(os.Stderr, "⚠️  Env ORDERLY_* mancanti → fallback PAPER")
					paper = true
					isLive = false
				}
				// kill switch check
				if _, err := os.Stat("/tmp/atps.halt"); err == nil {
					fmt.Fprintln(os.Stderr, "🛑 Kill-switch attivo (/tmp/atps.halt) → PAPER forzato")
					paper = true
					isLive = false
				}
			} else {
				paper = true
			}

			modeStr := "PAPER"
			if !paper {
				modeStr = "LIVE"
				if testnet {
					modeStr = "LIVE-TESTNET"
				}
			}
			fmt.Printf("Avvio bot %s %s %s [%s] paper=%v interval=%s poll=%ds\n", symbol, variant, interval, modeStr, paper, interval, pollSec)

			b, err := bot.New(cfg, symbol, interval, variant, paper)
			if err != nil {
				fmt.Fprintf(os.Stderr, "bot init error: %v\n", err)
				os.Exit(1)
			}

			// If not a TTY (e.g., headless, CI), run headless loop without TUI
			if !isTTY() {
				fmt.Println("No TTY rilevato — avvio modalità headless (Ctrl-C per stop)")
				fmt.Println("Paper:", paper, "— nessun ordine reale se paper=true. Logs ogni tick:")
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
						// also show equity/positions snapshot every 5 ticks
					}
				}
			}

			// TUI mode
			m := tui.NewLive(cfg, b, symbol, variant, interval, paper)
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
	cmd.Flags().BoolVar(&paper, "paper", true, "paper trading (simulato, default true)")
	cmd.Flags().BoolVar(&liveFlag, "live", false, "abilita ordini reali Orderly (richiede --i-understand-live)")
	cmd.Flags().BoolVar(&iUnderstandLive, "i-understand-live", false, "conferma invio ordini reali")
	cmd.Flags().BoolVar(&testnet, "testnet", false, "usa Orderly testnet")
	cmd.Flags().IntVar(&pollSec, "poll", 30, "poll interval secondi (live)")

	return cmd
}

func isTTY() bool {
	return isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsTerminal(os.Stderr.Fd())
}
