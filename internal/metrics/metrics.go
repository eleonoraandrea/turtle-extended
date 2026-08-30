package metrics

import (
	"math"
	"sort"
	"time"

	"github.com/atps/atps/internal/backtest"
)

// Stats comprehensive metrics ~32, mirrors backtesting.py parity attempt
type Stats struct {
	Symbol string `json:"symbol"`
	Variant string `json:"variant"`
	Start  time.Time `json:"start"`
	End    time.Time `json:"end"`
	DurationDays float64 `json:"duration_days"`
	InitialCapital float64 `json:"initial_capital"`
	FinalEquity    float64 `json:"final_equity"`
	ReturnPct      float64 `json:"return_pct"`
	ReturnAnnual   float64 `json:"return_ann"` // CAGR
	VolatilityAnn  float64 `json:"vol_ann"`
	Sharpe         float64 `json:"sharpe"`
	Sortino        float64 `json:"sortino"`
	Calmar         float64 `json:"calmar"`
	MaxDD          float64 `json:"max_dd"` // negative %
	MaxDDDurationBars int `json:"max_dd_duration"`
	ProfitFactor   float64 `json:"profit_factor"`
	PayoffRatio    float64 `json:"payoff_ratio"`
	WinRate        float64 `json:"win_rate"`
	Expectancy     float64 `json:"expectancy"`
	Trades         int     `json:"trades"`
	Winners        int     `json:"winners"`
	Losers         int     `json:"losers"`
	AvgTrade       float64 `json:"avg_trade"`
	AvgWin         float64 `json:"avg_win"`
	AvgLoss        float64 `json:"avg_loss"`
	BestTrade      float64 `json:"best_trade"`
	WorstTrade     float64 `json:"worst_trade"`
	AvgBarsHeld    float64 `json:"avg_bars_held"`
	ExposurePct    float64 `json:"exposure_pct"` // % time in market
	SQN            float64 `json:"sqn"`
	KellyPct       float64 `json:"kelly_pct"`
	UlcerIndex     float64 `json:"ulcer_index"`
	SerenityIndex  float64 `json:"serenity_index"` // custom
	GrossPnL       float64 `json:"gross_pnl"`
	NetPnL         float64 `json:"net_pnl"`
	TotalFee       float64 `json:"total_fee"`
	TotalFunding   float64 `json:"total_funding"`
	FeeDragPct     float64 `json:"fee_drag_pct"`
	FundingDragPct float64 `json:"funding_drag_pct"`
	BuyHoldReturn  float64 `json:"buy_hold_return"`
	Alpha          float64 `json:"alpha"`
	Beta           float64 `json:"beta"`
	AnnualTrades   float64 `json:"annual_trades"`
	ProfitPerBar   float64 `json:"profit_per_bar"`
	// ── NEW: expectancy × skew × compounding ──
	Skew           float64 `json:"skew"`            // skew of trade PnL
	SkewR          float64 `json:"skew_r"`          // skew of R-multiples (positive skew target)
	ExpectancyR    float64 `json:"expectancy_r"`    // expectancy in R
	MedianR        float64 `json:"median_r"`
	TailRatio      float64 `json:"tail_ratio"`      // best/worst in R
	PosSkewScore   float64 `json:"pos_skew_score"`  // expectancyR * skewR * (1+winRate) proxy for compounding
	// Monthly
	MonthlyReturns map[string]float64 `json:"monthly_returns,omitempty"`
	YearlyReturns  map[string]float64 `json:"yearly_returns,omitempty"`
}

func Compute(res *backtest.Result) Stats {
	s:=Stats{Symbol:res.Symbol, Variant:res.Variant, InitialCapital: res.InitialCapital, FinalEquity: res.FinalEquity, Trades: len(res.Trades), GrossPnL: res.GrossPnL, NetPnL: res.NetPnL, TotalFee: res.TotalFee, TotalFunding: res.TotalFunding}
	if len(res.Equity)==0 {return s}
	s.Start = res.Equity[0].Time
	s.End = res.Equity[len(res.Equity)-1].Time
	s.DurationDays = s.End.Sub(s.Start).Hours()/24
	if s.DurationDays<=0 {s.DurationDays=1}
	if s.InitialCapital>0{
		s.ReturnPct = (s.FinalEquity - s.InitialCapital)/s.InitialCapital*100
		years:= s.DurationDays/365.25
		if years>0{
			s.ReturnAnnual = (math.Pow(s.FinalEquity/s.InitialCapital, 1/years)-1)*100
		}
	}
	// Equity returns for vol/sharpe
	equityVals:= make([]float64,len(res.Equity))
	for i,e:=range res.Equity{equityVals[i]=e.Equity}
	rets:= logReturns(equityVals)
	// Volatility annualized (assume 4h bars => 2190 per year? but equity per bar)
	// Use daily resampled vol: approximate using bar returns * sqrt(252 * barsPerDay?) Simpler: use 252 trading days assumes daily equity; for 4h we have 6 per day -> annual factor = sqrt(252*6)
	barsPerDay:= estimateBarsPerDay(res)
	annFactor:= math.Sqrt(252 * barsPerDay)
	if barsPerDay==0{annFactor= math.Sqrt(252)}
	mean:= mean(rets)
	std:= stddev(rets, mean)
	s.VolatilityAnn = std*annFactor*100
	if std!=0{
		rf:=0.0 // assume 0
		s.Sharpe = (mean - rf)/std * annFactor
		downStd:= downsideDev(rets, 0)
		if downStd!=0{
			s.Sortino = (mean - rf)/downStd * annFactor
		} else { s.Sortino = math.NaN()}
	} else {s.Sharpe=math.NaN(); s.Sortino=math.NaN()}
	// MaxDD
	maxDD, ddDur := maxDrawdown(equityVals)
	s.MaxDD = maxDD // negative already? we store negative
	s.MaxDDDurationBars = ddDur
	if s.MaxDD!=0 {
		s.Calmar = s.ReturnAnnual / math.Abs(s.MaxDD)
	} else {s.Calmar=math.NaN()}
	// Trade stats
	var grossWin, grossLoss float64
	var wins, losses int
	var best, worst float64
	var sumTrade, sumWin, sumLoss float64
	var sumBars int
	if len(res.Trades)>0{
		best = res.Trades[0].PnLNet
		worst = res.Trades[0].PnLNet
		for _,t:=range res.Trades{
			sumTrade+=t.PnLNet
			sumBars+=t.BarsHeld
			if t.PnLNet>0{grossWin+=t.PnLNet; wins++; sumWin+=t.PnLNet; if t.PnLNet>best{best=t.PnLNet}} else {grossLoss+= -t.PnLNet; losses++; sumLoss+=t.PnLNet; if t.PnLNet<worst{worst=t.PnLNet}}
		}
		s.Winners=wins; s.Losers=losses
		s.WinRate= float64(wins)/float64(len(res.Trades))*100
		s.AvgTrade= sumTrade/float64(len(res.Trades))
		if wins>0{s.AvgWin=sumWin/float64(wins)} else {s.AvgWin=0}
		if losses>0{s.AvgLoss=sumLoss/float64(losses)} else {s.AvgLoss=0}
		if grossLoss!=0{s.ProfitFactor=grossWin/grossLoss}else if grossWin>0{s.ProfitFactor=math.Inf(1)} else {s.ProfitFactor=math.NaN()}
		if s.AvgLoss!=0{ s.PayoffRatio= s.AvgWin / math.Abs(s.AvgLoss)} else {s.PayoffRatio=math.NaN()}
		s.Expectancy = s.WinRate/100*s.AvgWin + (1-s.WinRate/100)*s.AvgLoss
		s.AvgBarsHeld = float64(sumBars)/float64(len(res.Trades))
		s.BestTrade=best; s.WorstTrade=worst
		// SQN
		if len(res.Trades)>=2{
			meanTrade:=meanOfTrades(res.Trades)
			sdTrade:=stdOfTrades(res.Trades, meanTrade)
			if sdTrade!=0{
				s.SQN = math.Sqrt(float64(len(res.Trades))) * meanTrade / sdTrade
			}
		}
		// Kelly (simplified edge/odds)
		if s.WinRate>0 && s.PayoffRatio>0 && !math.IsNaN(s.PayoffRatio){
			p:=s.WinRate/100
			b:= s.PayoffRatio
			s.KellyPct = (p*b - (1-p))/b *100
		}
		// ── NEW: skew / expectancyR / tail ──
		var rs []float64
		for _,t:=range res.Trades{ rs = append(rs, t.RMultiple)}
		if len(rs)>=3{
			s.Skew = calcSkew(rs)
			s.SkewR = s.Skew // alias
			// expectancy in R
			sumR:=0.0
			for _,r:=range rs{ sumR+=r }
			s.ExpectancyR = sumR/float64(len(rs))
			// median R
			tmp:=append([]float64(nil), rs...)
			sort.Float64s(tmp)
			s.MedianR = tmp[len(tmp)/2]
			// tail ratio: 95th percentile R / 5th percentile |R| magnitude
			if len(tmp)>=10{
				p95:= tmp[int(float64(len(tmp))*0.95)]
				p05:= tmp[int(float64(len(tmp))*0.05)]
				if p05!=0{ s.TailRatio = p95/math.Abs(p05) }
			}
			// positive skew score = expectancyR * max(0, skewR) * (1 + winRate/100)
			// This is the metric user wants to maximize
			if s.SkewR>0{
				s.PosSkewScore = s.ExpectancyR * s.SkewR * (1+s.WinRate/100)
			} else {
				s.PosSkewScore = s.ExpectancyR * 0.1 // penalize negative skew
			}
		}
	}
	// Exposure: time in market approximated as barsHeld / totalBars
	if len(res.Equity)>0 && len(res.Trades)>0{
		totalHeld:=0
		for _,t:=range res.Trades{totalHeld+=t.BarsHeld}
		s.ExposurePct = float64(totalHeld)/float64(len(res.Equity))*100
		if s.ExposurePct>100{s.ExposurePct=100}
	}
	// Ulcer Index
	s.UlcerIndex = ulcerIndex(equityVals)
	// Annual trades
	if s.DurationDays>0{
		s.AnnualTrades = float64(len(res.Trades))/s.DurationDays*365.25
	}
	// Fee drag
	if s.GrossPnL!=0{
		s.FeeDragPct = s.TotalFee / math.Abs(s.GrossPnL) *100
		s.FundingDragPct = math.Abs(s.TotalFunding) / math.Abs(s.GrossPnL) *100
	}
	// BuyHold
	if len(res.Bars)>=2{
		bhStart:=res.Bars[0].Close
		bhEnd:=res.Bars[len(res.Bars)-1].Close
		if bhStart>0{
			s.BuyHoldReturn = (bhEnd-bhStart)/bhStart*100
		}
	}
	// Monthly/Yearly
	s.MonthlyReturns = monthlyReturns(res.Equity)
	s.YearlyReturns = yearlyReturns(res.Equity)
	// Alpha/Beta vs buyHold (approx)
	// beta = covariance(ret, bhRet)/var(bhRet)
	if len(res.Bars)==len(res.Equity){
		bhEquity:= make([]float64,len(res.Bars))
		// synthetic BH equity: initial * price/price0
		p0:=res.Bars[0].Close
		for i,b:=range res.Bars{bhEquity[i]=s.InitialCapital * b.Close / p0}
		bhRets:=logReturns(bhEquity)
		if len(rets)==len(bhRets) && len(rets)>2{
			cov:= covariance(rets, bhRets)
			varBh:= variance(bhRets)
			if varBh!=0{s.Beta=cov/varBh}
			s.Alpha= s.ReturnAnnual - s.Beta* s.BuyHoldReturn // rough
		}
	}
	return s
}

func logReturns(equity []float64) []float64 {
	var out []float64
	for i:=1;i<len(equity);i++{
		if equity[i-1]<=0||equity[i]<=0{continue}
		out=append(out, math.Log(equity[i]/equity[i-1]))
	}
	return out
}
func mean(a []float64) float64 {
	if len(a)==0{return math.NaN()}
	sum:=0.0; for _,v:=range a{sum+=v}
	return sum/float64(len(a))
}
func stddev(a []float64, m float64) float64 {
	if len(a)<2{return 0}
	sum:=0.0; for _,v:=range a{ d:=v-m; sum+=d*d }
	return math.Sqrt(sum/float64(len(a)-1))
}
func downsideDev(a []float64, target float64) float64 {
	var sum float64
	n:=0
	for _,v:=range a{if v<target{ sum+= (v-target)*(v-target); n++}}
	if n==0{return 0}
	return math.Sqrt(sum/float64(n))
}
func maxDrawdown(equity []float64) (float64,int){
	peak:=equity[0]
	maxDD:=0.0
	maxDur:=0
	curDur:=0
	peakIdx:=0
	for i,eq:=range equity{
		if eq>peak{peak=eq; peakIdx=i; curDur=0}
		dd:=0.0
		if peak>0{dd=(eq-peak)/peak*100}
		if dd<maxDD{maxDD=dd; curDur=i-peakIdx; if curDur>maxDur{maxDur=curDur}}
	}
	return maxDD, maxDur
}
func meanOfTrades(trades []backtest.Trade) float64 {
	sum:=0.0; for _,t:=range trades{sum+=t.PnLNet}
	return sum/float64(len(trades))
}
func stdOfTrades(trades []backtest.Trade, m float64) float64 {
	sum:=0.0; for _,t:=range trades{ d:=t.PnLNet-m; sum+=d*d}
	return math.Sqrt(sum/float64(len(trades)-1))
}
func ulcerIndex(equity []float64) float64 {
	n:=len(equity)
	if n<2{return 0}
	peak:=equity[0]
	sumSq:=0.0
	for _,eq:=range equity{
		if eq>peak{peak=eq}
		dd:=0.0
		if peak>0{dd=(eq-peak)/peak*100}
		sumSq+= dd*dd
	}
	return math.Sqrt(sumSq/float64(n))
}
func estimateBarsPerDay(res *backtest.Result) float64 {
	if len(res.Equity)<2{return 6}
	// assume interval from config or infer from time delta
	dt:= res.Equity[1].Time.Sub(res.Equity[0].Time).Hours()
	if dt<=0{return 6}
	return 24/dt
}
func monthlyReturns(equity []backtest.EquityPoint) map[string]float64 {
	if len(equity)<2{return nil}
	// group by YYYY-MM, compute return within month
	m:=map[string][]backtest.EquityPoint{}
	for _,e:=range equity{
		key:=e.Time.Format("2006-01")
		m[key]=append(m[key], e)
	}
	out:=map[string]float64{}
	for k, pts:=range m{
		if len(pts)<2{continue}
		start:=pts[0].Equity
		end:=pts[len(pts)-1].Equity
		if start>0{out[k]=(end-start)/start*100}
	}
	return out
}
func yearlyReturns(equity []backtest.EquityPoint) map[string]float64 {
	if len(equity)<2{return nil}
	m:=map[string][]backtest.EquityPoint{}
	for _,e:=range equity{
		key:=e.Time.Format("2006")
		m[key]=append(m[key], e)
	}
	out:=map[string]float64{}
	for k, pts:=range m{
		if len(pts)<2{continue}
		start:=pts[0].Equity
		end:=pts[len(pts)-1].Equity
		if start>0{out[k]=(end-start)/start*100}
	}
	return out
}
func covariance(a,b []float64) float64 {
	if len(a)!=len(b)||len(a)==0{return 0}
	ma:=mean(a); mb:=mean(b)
	sum:=0.0; for i:=range a{ sum+=(a[i]-ma)*(b[i]-mb)}
	return sum/float64(len(a)-1)
}
func variance(a []float64) float64 {
	m:=mean(a)
	sum:=0.0; for _,v:=range a{ d:=v-m; sum+=d*d}
	return sum/float64(len(a)-1)
}

// Pretty helper for report sorting
func SortedMonthly(keys map[string]float64) []string {
	var ks []string
	for k:=range keys{ks=append(ks,k)}
	sort.Strings(ks)
	return ks
}

func calcSkew(vals []float64) float64 {
	n:=float64(len(vals))
	if n<3{return 0}
	m:=mean(vals)
	sd:=stddev(vals, m)
	if sd==0{return 0}
	sum:=0.0
	for _,v:=range vals{ sum+= math.Pow((v-m)/sd, 3)}
	return sum / n
}
