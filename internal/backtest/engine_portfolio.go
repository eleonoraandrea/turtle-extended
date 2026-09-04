package backtest

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
	"github.com/atps/atps/internal/indicators"
	"github.com/atps/atps/internal/risk"
	"github.com/atps/atps/internal/strategy"
)

// symState — stato per-simbolo del portfolio engine
type symState struct {
	symbol     string
	bars       data.Bars
	strat      strategy.Strategy
	ctx        *strategy.Context
	donExitH   []float64
	donExitL   []float64
	donExitH55 []float64
	donExitL55 []float64
	positions  []*Position
	cursor     int
	brakeUntil int
	lastClose  float64 // close dell'ultima barra processata (0 = nessuna)
	lastStop   struct {
		valid      bool
		side       int
		exitBarIdx int
	}
}

// RunPortfolio adatta il loop di engine.Run (engine.go) al caso multi-simbolo.
// REGOLA DI SINCRONIZZAZIONE: ogni modifica alle semantiche di Run (uscite, fill,
// sizing, funding) va replicata qui. I test invariante (TestRunPortfolioSingleSymbol
// Invariant*) su dati reali BTC/ETH sono il tripwire: falliscono se i due motori
// divergono. NON rifattorizzare Run mentre questo file esiste senza aggiornare
// anche qui + i test.
func RunPortfolio(barsMap map[string]data.Bars, strats map[string]strategy.Strategy, cfg *config.Config, eng EngineConfig) *Result {
	sym := eng.Symbol
	if sym == "" {
		sym = "PORTFOLIO"
	}
	res := &Result{Symbol: sym, Variant: eng.Variant, InitialCapital: eng.InitialCapital, FinalEquity: eng.InitialCapital}
	symbols := make([]string, 0, len(barsMap))
	for s := range barsMap {
		symbols = append(symbols, s)
	}
	sort.Strings(symbols) // ordine deterministico

	// risk limits + guardrail scaling — identico a Run
	lim := risk.LimitsFromConfig(cfg, eng.Variant)
	if lim.MaxLeverage == 0 {
		lim.MaxLeverage = eng.Leverage
		if lim.MaxLeverage == 0 {
			lim.MaxLeverage = 3
		}
	}
	if lim.MaxNotional == 0 && cfg != nil && cfg.Costs.MaxNotionalPerTrade > 0 {
		lim.MaxNotional = cfg.Costs.MaxNotionalPerTrade
	}
	// separate ⟹ satellite forced OFF (difesa in profondità: eng può divergere
	// da cfg.Pyramiding.Mode in test/costruzione diretta; risk.go copre cfg, qui copriamo eng).
	if eng.PyramidingMode == "separate" {
		lim.SatelliteEnabled = false
		lim.SatelliteAlloc = 0
	}
	res.RiskLimitsUsed = lim
	res.ScalingCeilingPct, res.ScalingBinding = risk.ScalingCeiling(lim)
	if res.ScalingCeilingPct < lim.MaxRiskPct {
		res.Warnings = append(res.Warnings, fmt.Sprintf("scaling: risk richiesto %.2f%% → tetto effettivo %.2f%% (%s lega)",
			lim.MaxRiskPct, res.ScalingCeilingPct, res.ScalingBinding))
	}
	if eng.PyramidingMode == "separate" && lim.PyramidingRiskNeutral {
		res.Warnings = append(res.Warnings, "pyramiding.mode=separate ignora risk_neutral (vale solo per merged)")
	}
	if eng.PyramidingMode == "separate" && cfg != nil && cfg.Profit.Satellite.Enabled {
		res.Warnings = append(res.Warnings, "pyramiding.mode=separate disabilita satellite (incompatibile: le gambe usano già exit wide)")
	}

	intervalH := intervalHours(cfg.General.Interval)
	if intervalH == 0 {
		intervalH = 4
	}

	exitLen := eng.DonExit
	if exitLen == 0 {
		exitLen = 20
	}

	// stato per-simbolo (Prepare + canali donchian)
	states := make([]*symState, 0, len(symbols))
	for _, s := range symbols {
		bars := barsMap[s]
		high := make([]float64, len(bars))
		low := make([]float64, len(bars))
		for i, b := range bars {
			high[i] = b.High
			low[i] = b.Low
		}
		st := &symState{
			symbol: s, bars: bars, strat: strats[s],
			ctx: strats[s].Prepare(bars), brakeUntil: -1,
			donExitH: indicators.DonchianHigh(high, exitLen), donExitL: indicators.DonchianLow(low, exitLen),
			donExitH55: indicators.DonchianHigh(high, 55), donExitL55: indicators.DonchianLow(low, 55),
		}
		states = append(states, st)
	}

	// barra di riferimento per metriche/report (buy&hold del primo simbolo in ordine alfabetico)
	res.Bars = barsMap[symbols[0]]

	// timeline: union ordinata dei timestamp
	seen := map[time.Time]bool{}
	var timeline []time.Time
	for _, st := range states {
		for _, b := range st.bars {
			if !seen[b.Time] {
				seen[b.Time] = true
				timeline = append(timeline, b.Time)
			}
		}
	}
	sort.Slice(timeline, func(a, b int) bool { return timeline[a].Before(timeline[b]) })

	// stato condiviso
	equity := eng.InitialCapital
	peak := equity
	var trades []Trade
	var equityCurve []EquityPoint
	var totalFee, totalFundingNet, totalSlippage float64

	openHeatAll := func() float64 {
		sum := 0.0
		for _, st := range states {
			for _, p := range st.positions {
				sum += p.RiskPct
			}
		}
		return sum
	}
	unrealizedAll := func() float64 {
		sum := 0.0
		for _, st := range states {
			for _, p := range st.positions {
				markPx := st.lastClose
				// come Run: posizione fillata next-open nella barra appena processata
				// → marcata a prezzo di entry (unrealized 0 sulla barra di segnale)
				if eng.UseNextOpen && p.EntryBarIdx == st.cursor-1 {
					markPx = p.EntryPrice
				}
				if p.Side == 1 {
					sum += (markPx - p.EntryPrice) * p.Qty
				} else {
					sum += (p.EntryPrice - markPx) * p.Qty
				}
			}
		}
		return sum
	}
	openNotionalAll := func() float64 {
		sum := 0.0
		for _, st := range states {
			for _, p := range st.positions {
				sum += p.Qty * st.lastClose
			}
		}
		return sum
	}

	// recordExit portfolio — identico a Run ma con equity/trades condivisi
	recordExitP := func(st *symState, pos *Position, exitPrice float64, reason string, barIdx int) {
		var pnl float64
		if pos.Side == 1 {
			pnl = (exitPrice - pos.EntryPrice) * pos.Qty
		} else {
			pnl = (pos.EntryPrice - exitPrice) * pos.Qty
		}
		exitFee := exitPrice * pos.Qty * eng.FeeBps / 10000.0
		fee := pos.EntryFee + exitFee
		pnlNet := pnl - fee - pos.FundingAccum
		equity += pnl - exitFee
		totalFee += exitFee
		rMult := 0.0
		if pos.RiskAmount > 0 {
			rMult = pnlNet / pos.RiskAmount
		}
		trades = append(trades, Trade{
			Symbol: st.symbol, Side: pos.Side,
			EntryTime: pos.EntryTime, ExitTime: st.bars[barIdx].Time,
			EntryPrice: pos.EntryPrice, ExitPrice: exitPrice,
			Qty: pos.Qty, EntryATR: pos.EntryATR, StopPrice: pos.StopPrice,
			EntryReason: pos.EntryReason, ExitReason: reason, DonExitLen: pos.DonExitLen,
			PnL: pnl, PnLNet: pnlNet, Fee: fee, FundingCost: pos.FundingAccum,
			BarsHeld: barIdx - pos.EntryBarIdx, MAE: pos.MAE, MFE: pos.MFE,
			ReturnPct: pnlNet / (pos.EntryPrice * pos.Qty) * 100,
			RiskPct:   pos.RiskPct, Leverage: pos.Leverage, Notional: pos.Notional,
			StopDist: math.Abs(pos.EntryPrice - pos.StopPrice), RMultiple: rMult,
			SizingLog: pos.SizingLog, IsSatellite: pos.IsSatellite,
		})
	}

	// contratto dati: barre per simbolo ordinate e con timestamp UNICI (i duplicati verrebbero skippati)
	for _, ts := range timeline {
		for _, st := range states {
			if st.cursor >= len(st.bars) || !st.bars[st.cursor].Time.Equal(ts) {
				continue
			}
			i := st.cursor
			bar := st.bars[i]
			st.cursor++
			n := len(st.bars)

			// ── funding — identico a Run ──
			for _, pos := range st.positions {
				if bar.FundingRate != 0 {
					scale := intervalH / 8.0
					notional := pos.Qty * bar.Close
					pay := notional * bar.FundingRate * scale
					if pos.Side == 1 {
						equity -= pay
						pos.FundingAccum += pay
						totalFundingNet += pay
					} else {
						equity += pay
						pos.FundingAccum -= pay
						totalFundingNet -= pay
					}
				}
			}

			// ── exits — identico a Run (per-simbolo, canali propri) ──
			var remaining []*Position
			for _, pos := range st.positions {
				exit := false
				exitReason := ""
				exitPrice := bar.Close
				var donL, donH float64
				if i >= 1 {
					if pos.DonExitLen == 55 {
						donL = st.donExitL55[i-1]
						donH = st.donExitH55[i-1]
					} else {
						donL = st.donExitL[i-1]
						donH = st.donExitH[i-1]
					}
				}
				if pos.Side == 1 {
					if bar.Low <= pos.StopPrice {
						exit = true
						exitReason = "stop"
						exitPrice = pos.StopPrice
						if bar.Open < exitPrice {
							exitPrice = bar.Open
						}
						if eng.SlippageBps > 0 {
							slip := exitPrice * eng.SlippageBps / 10000.0
							exitPrice -= slip
							totalSlippage += slip * pos.Qty
						}
					} else if !math.IsNaN(donL) && bar.Close < donL {
						exit = true
						if pos.IsSatellite {
							exitReason = "satellite_donchian55"
						} else {
							exitReason = "donchian_exit"
						}
						exitPrice = bar.Close
					} else {
						var newStop float64
						if eng.TrailMode == "chandelier" {
							mult := eng.TrailATRMult
							if mult <= 0 {
								mult = 3.0
							}
							if pos.IsSatellite {
								mult += 1.0
							}
							newStop = strategy.TrailStop(st.ctx, i, pos.Side, mult, "chandelier")
						} else {
							newStop = donL
						}
						if !math.IsNaN(newStop) {
							pos.StopPrice = risk.TrailStopPosition(pos.StopPrice, newStop, pos.Side)
						}
					}
				} else if pos.Side == -1 {
					if bar.High >= pos.StopPrice {
						exit = true
						exitReason = "stop"
						exitPrice = pos.StopPrice
						if bar.Open > exitPrice {
							exitPrice = bar.Open
						}
						if eng.SlippageBps > 0 {
							slip := exitPrice * eng.SlippageBps / 10000.0
							exitPrice += slip
							totalSlippage += slip * pos.Qty
						}
					} else if !math.IsNaN(donH) && bar.Close > donH {
						exit = true
						if pos.IsSatellite {
							exitReason = "satellite_donchian55"
						} else {
							exitReason = "donchian_exit"
						}
						exitPrice = bar.Close
					} else {
						var newStop float64
						if eng.TrailMode == "chandelier" {
							mult := eng.TrailATRMult
							if mult <= 0 {
								mult = 3.0
							}
							if pos.IsSatellite {
								mult += 1.0
							}
							newStop = strategy.TrailStop(st.ctx, i, pos.Side, mult, "chandelier")
						} else {
							newStop = donH
						}
						if !math.IsNaN(newStop) {
							pos.StopPrice = risk.TrailStopPosition(pos.StopPrice, newStop, pos.Side)
						}
					}
				}
				// MAE/MFE — identico a Run
				if pos.Side == 1 {
					if mae := (bar.Low - pos.EntryPrice) / pos.EntryPrice * 100; mae < pos.MAE {
						pos.MAE = mae
					}
					if mfe := (bar.High - pos.EntryPrice) / pos.EntryPrice * 100; mfe > pos.MFE {
						pos.MFE = mfe
					}
				} else {
					if mae := (pos.EntryPrice - bar.High) / pos.EntryPrice * 100; mae < pos.MAE {
						pos.MAE = mae
					}
					if mfe := (pos.EntryPrice - bar.Low) / pos.EntryPrice * 100; mfe > pos.MFE {
						pos.MFE = mfe
					}
				}
				if exit {
					recordExitP(st, pos, exitPrice, exitReason, i)
					if exitReason == "stop" {
						st.lastStop.valid = true
						st.lastStop.side = pos.Side
						st.lastStop.exitBarIdx = i
					}
				} else {
					remaining = append(remaining, pos)
				}
			}
			st.positions = remaining

			// ── crash brake per-simbolo — identico a Run (chiude SOLO questo simbolo) ──
			if cfg.Portfolio.CrashBrakeDropPct > 0 && i > 0 {
				retPct := (bar.Close - st.bars[i-1].Close) / st.bars[i-1].Close * 100
				if math.Abs(retPct) >= cfg.Portfolio.CrashBrakeDropPct {
					for _, pos := range st.positions {
						recordExitP(st, pos, bar.Close, "crash_brake", i)
					}
					st.positions = nil
					st.brakeUntil = i + 6
				}
			}

			st.lastClose = bar.Close

			// mark-to-market condiviso + peak/dd (come Run pre-signal)
			unrealized := 0.0
			for _, st2 := range states {
				for _, p := range st2.positions {
					if p.Side == 1 {
						unrealized += (st2.lastClose - p.EntryPrice) * p.Qty
					} else {
						unrealized += (p.EntryPrice - st2.lastClose) * p.Qty
					}
				}
			}
			curEq := equity + unrealized
			if curEq > peak {
				peak = curEq
			}
			ddPct := 0.0
			if peak > 0 {
				ddPct = (curEq - peak) / peak * 100
			}

			if i < st.brakeUntil || curEq <= 0 {
				continue // niente segnali per questo simbolo (curve point a fine timestamp)
			}

			// ── signal: intrabar → Next → re-entry — identico a Run, per-simbolo ──
			var sig strategy.Signal
			intrabarFill, intrabarSlip := 0.0, 0.0
			isIntrabar := false
			if eng.EntryMode == "intrabar" && len(st.positions) == 0 && i >= 1 && i+1 < n {
				if lv, ok := st.strat.(strategy.IntrabarLevels); ok {
					levels := lv.IntrabarEntry(st.ctx, i)
					atrPrev := st.ctx.ATR[i-1]
					if levels.Enabled && !math.IsNaN(atrPrev) && atrPrev > 0 {
						longHit := !math.IsNaN(levels.LongLevel) && bar.High >= levels.LongLevel
						shortHit := !math.IsNaN(levels.ShortLevel) && bar.Low <= levels.ShortLevel
						side := 0
						var level, stopATR float64
						if longHit && !shortHit {
							side, level, stopATR = 1, levels.LongLevel, levels.LongStopATR
						} else if shortHit && !longHit {
							side, level, stopATR = -1, levels.ShortLevel, levels.ShortStopATR
						}
						if side != 0 && stopATR > 0 {
							fill := level
							if (side == 1 && bar.Open > level) || (side == -1 && bar.Open < level) {
								fill = bar.Open
							}
							if eng.SlippageBps > 0 {
								intrabarSlip = fill * eng.SlippageBps / 10000.0
								if side == 1 {
									fill += intrabarSlip
								} else {
									fill -= intrabarSlip
								}
							}
							stop := fill - float64(side)*stopATR*atrPrev
							sig = strategy.Signal{Side: side, Strength: 1, StopPrice: stop, Reason: "intrabar breakout"}
							intrabarFill = fill
							isIntrabar = true
						}
					}
				}
			}
			if sig.Side == 0 {
				sig = st.strat.Next(st.ctx, i)
			}
			if sig.Side == 0 && st.lastStop.valid {
				if rc, ok := st.strat.(strategy.ReEntryChecker); ok {
					sig = rc.ReEntry(st.ctx, i, strategy.StopOutInfo{Side: st.lastStop.side, ExitBarIdx: st.lastStop.exitBarIdx})
				}
			}

			if sig.Side != 0 && !(eng.UseNextOpen && !isIntrabar && i+1 >= n) {
				atr := st.ctx.ATR[i]
				if math.IsNaN(atr) {
					atr = 0
				}
				fillPrice := bar.Close
				fillTime := bar.Time
				slipAmt := 0.0
				if isIntrabar {
					fillPrice = intrabarFill
					slipAmt = intrabarSlip
				} else if eng.UseNextOpen && i+1 < n {
					fillPrice = st.bars[i+1].Open
					fillTime = st.bars[i+1].Time
					if eng.SlippageBps > 0 {
						slipAmt = fillPrice * eng.SlippageBps / 10000.0
						if sig.Side == 1 {
							fillPrice += slipAmt
						} else {
							fillPrice -= slipAmt
						}
					}
				} else if eng.SlippageBps > 0 {
					slipAmt = fillPrice * eng.SlippageBps / 10000.0
					if sig.Side == 1 {
						fillPrice += slipAmt
					} else {
						fillPrice -= slipAmt
					}
				}
				stopPx := sig.StopPrice
				if math.IsNaN(stopPx) || stopPx <= 0 {
					stopPx = fillPrice - float64(sig.Side)*2*atr
					if math.IsNaN(stopPx) {
						stopPx = 0
					}
				}
				stopValid := (sig.Side == 1 && stopPx < fillPrice) || (sig.Side == -1 && stopPx > fillPrice)
				if !stopValid {
					sig.Side = 0
				}
				if sig.Side != 0 {
					// heat condiviso: totale = tutte le posizioni; correlato = same-side TUTTI simboli
					corrHeat := 0.0
					for _, st2 := range states {
						for _, p := range st2.positions {
							if p.Side == sig.Side {
								corrHeat += p.RiskPct
							}
						}
					}
					ms := risk.MarketState{
						Equity:                 curEq,
						Price:                  fillPrice,
						ATR:                    atr,
						StopPrice:              stopPx,
						Side:                   sig.Side,
						VolRegime:              st.ctx.VolRegime[i],
						ADX:                    st.ctx.ADX[i],
						FundingZ:               st.ctx.FundingZ[i],
						VolAnnualizedPct:       risk.AnnualizedVolPct(atr, fillPrice, intervalH),
						PortfolioHeatPct:       openHeatAll(),
						PortfolioCorrelatedPct: corrHeat,
						EquityDDPct:            -ddPct,
					}

					sameSideHeat := 0.0
					var earliest *Position
					for _, p := range st.positions {
						if p.Side == sig.Side {
							sameSideHeat += p.RiskPct
							if earliest == nil {
								earliest = p
							}
						}
					}
					sameSideUnits := 0
					if earliest != nil {
						sameSideUnits = earliest.Units
					}
					if eng.PyramidingMode == "separate" {
						sameSideUnits = 0
						for _, p := range st.positions {
							if p.Side == sig.Side && !p.IsSatellite {
								sameSideUnits++
							}
						}
					}
					hasSameSide := earliest != nil
					if hasSameSide {
						if risk.CanPyramid(earliest.EntryPrice, bar.Close, atr, sig.Side, sameSideUnits, eng.PyramidingMax, eng.PyramidStepATR) {
							dec := risk.Size(ms, lim)
							if dec.CappedByNotional {
								res.NotionalCapHits++
							}
							if lim.PyramidingRiskNeutral && eng.PyramidingMode != "separate" {
								dec.RiskPct = dec.RiskPct * 0.5
								dec.RiskAmount = dec.RiskPct / 100 * ms.Equity
								dec.Qty = dec.RiskAmount / dec.StopDist
								dec.Notional = dec.Qty * fillPrice
								dec.Leverage = dec.Notional / ms.Equity
								dec.Factors = append(dec.Factors, "pyramid risk_neutral ×0.5")
							} else if eng.PyramidingMode != "separate" {
								dec.Notional = dec.Qty * fillPrice
								dec.RiskAmount = dec.Qty * dec.StopDist
								if ms.Equity > 0 {
									dec.RiskPct = dec.RiskAmount / ms.Equity * 100
									totalNotional := 0.0
									for _, p := range st.positions {
										if p.Side == sig.Side {
											totalNotional += p.Notional
										}
									}
									dec.Leverage = (totalNotional + dec.Notional) / ms.Equity
								}
							}
							if dec.Accept && dec.Qty > 0 {
								fee := fillPrice * dec.Qty * eng.FeeBps / 10000.0
								slipCost := slipAmt * dec.Qty
								equity -= fee
								totalFee += fee
								totalSlippage += slipCost
								if eng.PyramidingMode == "separate" {
									leg := &Position{
										Symbol: st.symbol, Side: sig.Side, Qty: dec.Qty,
										EntryPrice: fillPrice, EntryTime: fillTime, EntryATR: atr,
										StopPrice: stopPx, Units: 1, EntryBarIdx: i,
										RiskPct: dec.RiskPct, Leverage: dec.Leverage,
										Notional: dec.Notional, RiskAmount: dec.RiskAmount,
										SizingLog: logFactors(dec) + " | pyramid separate (wide Don55)",
										EntryFee:  fee, EntryReason: sig.Reason + " | pyramid separate",
										IsSatellite: false, DonExitLen: 55,
									}
									st.positions = append(st.positions, leg)
								} else if lim.PyramidingRiskNeutral {
									earliest.EntryPrice = (earliest.EntryPrice*earliest.Qty + fillPrice*dec.Qty) / (earliest.Qty + dec.Qty)
									earliest.Qty += dec.Qty
									earliest.Units++
									earliest.Notional += dec.Notional
									earliest.EntryFee += fee
									earliest.Leverage = earliest.Notional / ms.Equity
									if !math.IsNaN(stopPx) {
										earliest.StopPrice = risk.TrailStopPosition(earliest.StopPrice, stopPx, sig.Side)
									}
									earliest.SizingLog += " | pyramid(risk_neutral): " + logFactors(dec)
								} else {
									totalQty := earliest.Qty + dec.Qty
									earliest.EntryPrice = (earliest.EntryPrice*earliest.Qty + fillPrice*dec.Qty) / totalQty
									earliest.Qty = totalQty
									earliest.Units++
									earliest.RiskPct += dec.RiskPct
									earliest.RiskAmount += dec.RiskAmount
									earliest.Notional += dec.Notional
									earliest.EntryFee += fee
									earliest.Leverage = earliest.Notional / ms.Equity
									if !math.IsNaN(stopPx) {
										earliest.StopPrice = risk.TrailStopPosition(earliest.StopPrice, stopPx, sig.Side)
									}
									earliest.SizingLog += " | pyramid: " + logFactors(dec)
								}
							}
						}
					} else if len(st.positions) == 0 {
						dec := risk.Size(ms, lim)
						if dec.CappedByNotional {
							res.NotionalCapHits++
						}
						if dec.Accept && dec.Qty > 0 {
							fee := fillPrice * dec.Qty * eng.FeeBps / 10000.0
							slipCost := slipAmt * dec.Qty
							equity -= fee
							totalFee += fee
							totalSlippage += slipCost
							if lim.SatelliteEnabled && lim.SatelliteAlloc > 0 && lim.SatelliteAlloc < 1 {
								coreQty := dec.Qty * (1 - lim.SatelliteAlloc)
								satQty := dec.Qty * lim.SatelliteAlloc
								coreRisk := dec.RiskPct * (1 - lim.SatelliteAlloc)
								satRisk := dec.RiskPct * lim.SatelliteAlloc
								coreNotional := coreQty * fillPrice
								satNotional := satQty * fillPrice
								corePos := &Position{
									Symbol: st.symbol, Side: sig.Side, Qty: coreQty,
									EntryPrice: fillPrice, EntryTime: fillTime, EntryATR: atr,
									StopPrice: stopPx, Units: 1, EntryBarIdx: i,
									RiskPct: coreRisk, Leverage: coreNotional / ms.Equity,
									Notional: coreNotional, RiskAmount: coreRisk / 100 * ms.Equity,
									SizingLog:   logFactors(dec) + " | core 70%",
									EntryFee:    fee * (1 - lim.SatelliteAlloc),
									EntryReason: sig.Reason, IsSatellite: false, DonExitLen: 20,
								}
								satPos := &Position{
									Symbol: st.symbol, Side: sig.Side, Qty: satQty,
									EntryPrice: fillPrice, EntryTime: fillTime, EntryATR: atr,
									StopPrice: stopPx, Units: 1, EntryBarIdx: i,
									RiskPct: satRisk, Leverage: satNotional / ms.Equity,
									Notional: satNotional, RiskAmount: satRisk / 100 * ms.Equity,
									SizingLog:   logFactors(dec) + " | satellite 30% (wide Don55)",
									EntryFee:    fee * lim.SatelliteAlloc,
									EntryReason: sig.Reason, IsSatellite: true, DonExitLen: 55,
								}
								st.positions = append(st.positions, corePos, satPos)
							} else {
								pos := &Position{
									Symbol: st.symbol, Side: sig.Side, Qty: dec.Qty,
									EntryPrice: fillPrice, EntryTime: fillTime, EntryATR: atr,
									StopPrice: stopPx, Units: 1, EntryBarIdx: i,
									RiskPct: dec.RiskPct, Leverage: dec.Leverage,
									Notional: dec.Notional, RiskAmount: dec.RiskAmount,
									SizingLog: logFactors(dec), EntryFee: fee,
									EntryReason: sig.Reason, DonExitLen: 20,
								}
								st.positions = append(st.positions, pos)
							}
						}
					}

					// ── intrabar same-bar stop — identico a Run ──
					if isIntrabar {
						var survived []*Position
						for _, p := range st.positions {
							if p.EntryBarIdx != i {
								survived = append(survived, p)
								continue
							}
							stopHit := (p.Side == 1 && bar.Low <= p.StopPrice) || (p.Side == -1 && bar.High >= p.StopPrice)
							if !stopHit {
								survived = append(survived, p)
								continue
							}
							exitPrice := p.StopPrice
							if eng.SlippageBps > 0 {
								slip := exitPrice * eng.SlippageBps / 10000.0
								if p.Side == 1 {
									exitPrice -= slip
								} else {
									exitPrice += slip
								}
								totalSlippage += slip * p.Qty
							}
							if p.Side == 1 {
								if mae := (bar.Low - p.EntryPrice) / p.EntryPrice * 100; mae < p.MAE {
									p.MAE = mae
								}
								if mfe := (bar.High - p.EntryPrice) / p.EntryPrice * 100; mfe > p.MFE {
									p.MFE = mfe
								}
							} else {
								if mae := (p.EntryPrice - bar.High) / p.EntryPrice * 100; mae < p.MAE {
									p.MAE = mae
								}
								if mfe := (p.EntryPrice - bar.Low) / p.EntryPrice * 100; mfe > p.MFE {
									p.MFE = mfe
								}
							}
							st.lastStop.valid = true
							st.lastStop.side = p.Side
							st.lastStop.exitBarIdx = i
							recordExitP(st, p, exitPrice, "stop_same_bar", i)
						}
						st.positions = survived
					}
				}
			}
		}

		// ── equity point per timestamp (fine barra, tutti i simboli processati) ──
		curEq := equity + unrealizedAll()
		if curEq > peak {
			peak = curEq
		}
		dd := 0.0
		if peak > 0 {
			dd = (curEq - peak) / peak * 100
		}
		price := 0.0
		for _, st := range states {
			if st.cursor > 0 && st.bars[st.cursor-1].Time.Equal(ts) {
				price = st.bars[st.cursor-1].Close
				break
			}
		}
		equityCurve = append(equityCurve, EquityPoint{
			Time: ts, Equity: curEq, Drawdown: dd,
			// close del primo simbolo (ordine alfabetico) che ha processato questo timestamp — riferimento grafico, non prezzo di portafoglio
			Price: price,
			Heat:  openHeatAll(), Leverage: openNotionalAll() / math.Max(curEq, 1),
		})
	}

	// ── chiusure EOD per-simbolo — identico a Run ──
	for _, st := range states {
		if len(st.bars) == 0 {
			continue
		}
		n := len(st.bars)
		for _, pos := range st.positions {
			recordExitP(st, pos, st.bars[n-1].Close, "eod", n-1)
		}
		st.positions = nil
	}

	final := equity
	if len(equityCurve) > 0 {
		equityCurve[len(equityCurve)-1].Equity = final
	}
	gross, net := 0.0, 0.0
	maxLev, sumLev, maxRisk, sumRisk, maxHeat := 0.0, 0.0, 0.0, 0.0, 0.0
	for _, t := range trades {
		gross += t.PnL
		net += t.PnLNet
		if t.Leverage > maxLev {
			maxLev = t.Leverage
		}
		sumLev += t.Leverage
		if t.RiskPct > maxRisk {
			maxRisk = t.RiskPct
		}
		sumRisk += t.RiskPct
	}
	for _, e := range equityCurve {
		if e.Heat > maxHeat {
			maxHeat = e.Heat
		}
	}
	tn := float64(len(trades))
	if tn > 0 {
		res.AvgLeverage = sumLev / tn
		res.AvgRiskPct = sumRisk / tn
	}
	res.MaxLeverageUsed = maxLev
	res.MaxRiskPctUsed = maxRisk
	res.MaxHeatSeen = maxHeat
	res.Trades = trades
	res.Equity = equityCurve
	res.FinalEquity = final
	res.GrossPnL = gross
	res.NetPnL = net
	res.TotalFee = totalFee
	res.TotalFunding = totalFundingNet
	res.TotalSlippage = totalSlippage
	return res
}
