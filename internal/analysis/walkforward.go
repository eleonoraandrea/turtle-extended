package analysis

import (
	"sort"

	"github.com/atps/atps/internal/backtest"
	"github.com/atps/atps/internal/config"
	"github.com/atps/atps/internal/data"
	"github.com/atps/atps/internal/metrics"
	"github.com/atps/atps/internal/strategy"
)

type WFFold struct {
	Index int     `json:"index"`
	TrainStart int `json:"train_start"`
	TrainEnd   int `json:"train_end"`
	TestStart  int `json:"test_start"`
	TestEnd    int `json:"test_end"`
	TrainStats metrics.Stats `json:"train_stats"`
	TestStats  metrics.Stats `json:"test_stats"`
}

type WFResult struct {
	Symbol string `json:"symbol"`
	Variant string `json:"variant"`
	Folds []WFFold `json:"folds"`
	AvgTrainSharpe float64 `json:"avg_train_sharpe"`
	AvgTestSharpe  float64 `json:"avg_test_sharpe"`
	Decay          float64 `json:"decay"` // test/train
	OOSReturn      float64 `json:"oos_return"` // compounded
}

func WalkForward(bars data.Bars, strat strategy.Strategy, cfg *config.Config, eng backtest.EngineConfig, folds int, trainRatio float64) WFResult {
	n:=len(bars)
	if folds<=1{folds=4}
	if trainRatio<=0||trainRatio>=1{trainRatio=0.7}
	chunk:= n / folds
	var res WFResult
	res.Symbol=eng.Symbol
	res.Variant=eng.Variant
	var oosEquity=1.0
	var trainSharpes, testSharpes []float64
	for i:=0;i<folds;i++{
		start:= i*chunk
		end:= start+chunk
		if i==folds-1{end=n}
		trainEnd:= start + int(float64(end-start)*trainRatio)
		if trainEnd>=end{trainEnd=end-1}
		testStart:= trainEnd
		// need warmup: ensure train has enough bars for indicators
		trainBars:= bars[start:trainEnd]
		testBars:= bars[testStart:end]
		if len(trainBars)<210 || len(testBars)<30{continue}
		trainRes:= backtest.Run(trainBars, strat, cfg, eng)
		testRes:= backtest.Run(testBars, strat, cfg, eng)
		trainStats:= metrics.Compute(trainRes)
		testStats:= metrics.Compute(testRes)
		oosEquity *= (1+ testStats.ReturnPct/100)
		trainSharpes=append(trainSharpes, trainStats.Sharpe)
		testSharpes=append(testSharpes, testStats.Sharpe)
		res.Folds=append(res.Folds, WFFold{Index:i, TrainStart:start, TrainEnd:trainEnd, TestStart:testStart, TestEnd:end, TrainStats:trainStats, TestStats:testStats})
	}
	if len(trainSharpes)>0{
		res.AvgTrainSharpe = mean(trainSharpes)
		res.AvgTestSharpe = mean(testSharpes)
		if res.AvgTrainSharpe!=0{res.Decay = res.AvgTestSharpe/res.AvgTrainSharpe}
		res.OOSReturn = (oosEquity-1)*100
	}
	return res
}
func mean(a []float64) float64 { if len(a)==0{return 0}; s:=0.0; for _,v:=range a{if !isNaN(v){s+=v}}; return s/float64(len(a)) }
func isNaN(f float64) bool { return f!=f }
func SortedFoldsByReturn(folds []WFFold) []WFFold {
	cp:=append([]WFFold(nil), folds...)
	sort.Slice(cp, func(i,j int) bool{return cp[i].TestStats.ReturnPct>cp[j].TestStats.ReturnPct})
	return cp
}
