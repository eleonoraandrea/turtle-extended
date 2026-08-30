# Graph Report - turtle-extended  (2026-08-30)

## Corpus Check
- cluster-only mode — file stats not available

## Summary
- 313 nodes · 740 edges · 13 communities (12 shown, 1 thin omitted)
- Extraction: 98% EXTRACTED · 2% INFERRED · 0% AMBIGUOUS · INFERRED: 17 edges (avg confidence: 0.85)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `048d074e`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- Bars
- Config
- context.Context
- Compute
- Model
- Run
- GenerateSynthetic
- PrepareCommon
- binance.go
- MonteCarlo
- WalkForward
- Perturb
- github.com/atps/atps

## God Nodes (most connected - your core abstractions)
1. `Config` - 60 edges
2. `Run()` - 29 edges
3. `Compute()` - 27 edges
4. `Bars` - 25 edges
5. `Model` - 24 edges
6. `New()` - 22 edges
7. `GenerateSynthetic()` - 20 edges
8. `PrepareCommon()` - 18 edges
9. `loadCfg()` - 14 edges
10. `Context` - 13 edges

## Surprising Connections (you probably didn't know these)
- `loadCfg()` --calls--> `DefaultPath()`  [EXTRACTED]
  cmd/atps/main.go → internal/config/config.go
- `engineFromCfg()` --references--> `Config`  [EXTRACTED]
  cmd/atps/main.go → internal/config/config.go
- `loadCfg()` --references--> `Config`  [EXTRACTED]
  cmd/atps/main.go → internal/config/config.go
- `cmdWalkForward()` --calls--> `WalkForward()`  [EXTRACTED]
  cmd/atps/main.go → internal/analysis/walkforward.go
- `main()` --calls--> `WalkForward()`  [EXTRACTED]
  scripts/walk_forward/main.go → internal/analysis/walkforward.go

## Import Cycles
- None detected.

## Communities (13 total, 1 thin omitted)

### Community 0 - "Bars"
Cohesion: 0.07
Nodes (15): Bar, Bars, Strategy, volConfirm(), NewA(), NewB(), NewC(), NewD() (+7 more)

### Community 1 - "Config"
Cohesion: 0.07
Nodes (32): ATRCfg, Backtest, Compare, Costs, Data, DrawdownCfg, FundingCfg, General (+24 more)

### Community 2 - "context.Context"
Cohesion: 0.11
Nodes (14): Adapter, errNotLive, PaperAdapter, context.Context, Balance, OrderRequest, OrderResponse, Position (+6 more)

### Community 3 - "Compute"
Cohesion: 0.12
Nodes (33): PortfolioResult, Position, time.Time, RunPortfolio(), EngineConfig, EquityPoint, Result, Trade (+25 more)

### Community 4 - "Model"
Cohesion: 0.09
Nodes (17): github.com/charmbracelet/bubbles/spinner.Model, github.com/charmbracelet/bubbles/viewport.Model, github.com/charmbracelet/bubbletea.Cmd, github.com/charmbracelet/bubbletea.Model, github.com/charmbracelet/bubbletea.Msg, github.com/charmbracelet/lipgloss.Style, time.Duration, ComparisonRow (+9 more)

### Community 5 - "Run"
Cohesion: 0.14
Nodes (27): testing.T, logFactors(), Run(), Load(), AnnualizedVolPct(), CanPyramid(), DefaultLimits(), RiskLimits (+19 more)

### Community 6 - "GenerateSynthetic"
Cohesion: 0.21
Nodes (22): cmdBacktest(), cmdCompare(), cmdDownload(), cmdGenerateDemo(), cmdMonteCarlo(), cmdPerturb(), cmdPortfolio(), cmdReportDemo() (+14 more)

### Community 7 - "PrepareCommon"
Cohesion: 0.26
Nodes (15): ADX(), ATR(), ChandelierLong(), ChandelierShort(), DonchianHigh(), DonchianLow(), EMA(), PercentileRank() (+7 more)

### Community 8 - "binance.go"
Cohesion: 0.22
Nodes (10): BinanceClient, FundingRate, OIHist, net/http.Client, AlignDerivatives(), intervalToDuration(), NewBinanceClient(), toFloat() (+2 more)

### Community 9 - "MonteCarlo"
Cohesion: 0.52
Nodes (6): MCResult, meanRets(), MonteCarlo(), percentile(), stdRets(), tradeSharpe()

### Community 10 - "WalkForward"
Cohesion: 0.48
Nodes (5): WFFold, WFResult, mean(), SortedFoldsByReturn(), WalkForward()

### Community 11 - "Perturb"
Cohesion: 0.83
Nodes (3): PerturbResult, Perturb(), PerturbSummary()

## Knowledge Gaps
- **6 isolated node(s):** `VariantA`, `VariantB`, `VariantC`, `VariantD`, `github.com/atps/atps` (+1 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **1 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `Config` connect `Config` to `Bars`, `Compute`, `Model`, `Run`, `GenerateSynthetic`, `PrepareCommon`, `MonteCarlo`, `WalkForward`, `Perturb`?**
  _High betweenness centrality (0.406) - this node is a cross-community bridge._
- **Why does `Position` connect `context.Context` to `Compute`?**
  _High betweenness centrality (0.182) - this node is a cross-community bridge._
- **Why does `Model` connect `Model` to `Config`, `Compute`, `GenerateSynthetic`?**
  _High betweenness centrality (0.152) - this node is a cross-community bridge._
- **What connects `VariantA`, `VariantB`, `VariantC` to the rest of the system?**
  _6 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `Bars` be split into smaller, more focused modules?**
  _Cohesion score 0.06868686868686869 - nodes in this community are weakly interconnected._
- **Should `Config` be split into smaller, more focused modules?**
  _Cohesion score 0.06871035940803383 - nodes in this community are weakly interconnected._
- **Should `context.Context` be split into smaller, more focused modules?**
  _Cohesion score 0.10668563300142248 - nodes in this community are weakly interconnected._