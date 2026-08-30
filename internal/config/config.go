package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Root config mirroring default.yaml
type Config struct {
	General    General    `yaml:"general"`
	Costs      Costs      `yaml:"costs"`
	Risk       Risk       `yaml:"risk"`
	Portfolio  Portfolio  `yaml:"portfolio"`
	LeverageCfg LeverageCfg `yaml:"leverage"`
	Trend      TrendCfg    `yaml:"trend"`
	ATRConf    ATRCfg      `yaml:"atr"`
	Pyramiding PyramidingCfg `yaml:"pyramiding"`
	Profit     ProfitCfg     `yaml:"profit"`
	Regime     RegimeCfg     `yaml:"regime"`
	Volatility VolatilityCfg `yaml:"volatility"`
	Drawdown   DrawdownCfg   `yaml:"drawdown"`
	Funding    FundingCfg    `yaml:"funding"`
	OpenInterest OpenInterestCfg `yaml:"open_interest"`
	Backtest   Backtest   `yaml:"backtest"`
	VariantA   VariantA   `yaml:"variant_a"`
	VariantB   VariantB   `yaml:"variant_b"`
	VariantC   VariantC   `yaml:"variant_c"`
	VariantD   VariantD   `yaml:"variant_d"`
	Compare    Compare    `yaml:"compare"`
	WalkForward WalkForward `yaml:"walk_forward"`
	MonteCarlo MonteCarlo `yaml:"monte_carlo"`
	Report     Report     `yaml:"report"`
	Data       Data       `yaml:"data"`
	Orderly    Orderly    `yaml:"orderly"`
}

type General struct {
	InitialCapital float64  `yaml:"initial_capital"`
	Interval       string   `yaml:"interval"`
	Symbols        []string `yaml:"symbols"`
	OrderlySymbols []string `yaml:"orderly_symbols"`
	StartTime      string   `yaml:"start_time"`
	EndTime        string   `yaml:"end_time"`
}
type Costs struct {
	FeeBps               float64 `yaml:"fee_bps"`
	SlippageBps          float64 `yaml:"slippage_bps"`
	FundingIntervalHours int     `yaml:"funding_interval_hours"`
	Leverage             float64 `yaml:"leverage"`
	MaxNotionalPerTrade  float64 `yaml:"max_notional_per_trade"`
}

// Risk — global risk limits. Leverage is DYNAMIC, never fixed:
// it derives from notional/equity after risk-based sizing, capped
// by market riskiness (vol regime, ADX, funding z, drawdown).
// Supporta sia spec legacy (max_risk_per_trade_pct) che nuovo spec (base/min/max 0.01/0.0025/0.02).
type Risk struct {
	MaxRiskPerTradePct  float64 `yaml:"max_risk_per_trade_pct"`   // legacy: 2.0 = max 2%
	Base                float64 `yaml:"base"`                     // new spec: 0.01 = 1% base
	Min                 float64 `yaml:"min"`                      // new spec: 0.0025 = 0.25% min
	Max                 float64 `yaml:"max"`                      // new spec: 0.02 = 2% max
	MaxHeatPct          float64 `yaml:"max_portfolio_heat_pct"`   // legacy heat
	MaxLeverage         float64 `yaml:"max_leverage"`             // HARD cap (e.g. 5)
	MinLeverageCap      float64 `yaml:"min_leverage_cap"`         // floor for dynamic cap
	MaxNotional         float64 `yaml:"max_notional"`             // 0 = disabled
	VolTargetPct        float64 `yaml:"vol_target_pct"`           // annualized vol target
	KellyCapPct         float64 `yaml:"kelly_cap_pct"`            // 0 = disabled
	DDDeleverageStart   float64 `yaml:"dd_deleverage_start_pct"`  // start scaling risk at this drawdown
	DDFlatPct           float64 `yaml:"dd_flat_pct"`              // risk → 0 at this drawdown
	ADXSoftThreshold    float64 `yaml:"adx_soft_threshold"`       // weak trend → lower lev cap
}

// New spec top-level sections — per massimizzare Expectancy × Skew × Compounding
type LeverageCfg struct {
	Max float64 `yaml:"max"`
}
type TrendCfg struct {
	DonchianEntry int `yaml:"donchian_entry"`
	DonchianExit  int `yaml:"donchian_exit"`
}
type ATRCfg struct {
	Period      int     `yaml:"period"`
	InitialStop float64 `yaml:"initial_stop"`
}
type PyramidingCfg struct {
	Enabled      bool `yaml:"enabled"`
	MaxAdditions int  `yaml:"max_additions"`
	RiskNeutral  bool `yaml:"risk_neutral"`
}
type SatelliteCfg struct {
	Enabled    bool    `yaml:"enabled"`
	Allocation float64 `yaml:"allocation"` // 0.30 = 30% satellite
}
type ProfitCfg struct {
	Trailing  bool         `yaml:"trailing"`
	FixedTP   bool         `yaml:"fixed_tp"`
	Satellite SatelliteCfg `yaml:"satellite"`
}
type RegimeCfg struct {
	BtcFilter bool    `yaml:"btc_filter"`
	AdxMin    float64 `yaml:"adx_min"`
}
type VolatilityCfg struct {
	AdaptiveRisk bool `yaml:"adaptive_risk"`
}
type DrawdownCfg struct {
	AdaptiveRisk bool `yaml:"adaptive_risk"`
}
type FundingCfg struct {
	Filter bool `yaml:"filter"`
}
type OpenInterestCfg struct {
	Filter bool `yaml:"filter"`
}
type Portfolio struct {
	MaxPortfolioHeatPct float64 `yaml:"max_portfolio_heat_pct"` // legacy
	MaxOpenRisk         float64 `yaml:"max_open_risk"`         // new spec: 0.03 = 3%
	MaxCorrelatedRisk   float64 `yaml:"max_correlated_risk"`   // new spec: 0.02 = 2%
	MaxCorrExposure     int     `yaml:"max_corr_exposure"`
	CrashBrakeDropPct   float64 `yaml:"crash_brake_drop_pct"`
}
type Backtest struct {
	PyramidingMaxUnits int     `yaml:"pyramiding_max_units"`
	PyramidStepATR     float64 `yaml:"pyramid_step_atr"`
	TrailATRMult       float64 `yaml:"trail_atr_mult"`
	UseNextOpenFill    bool    `yaml:"use_next_open_fill"`
	CommissionModel    string  `yaml:"commission_model"`
}
type VariantA struct {
	Name         string  `yaml:"name"`
	DonchianEntry int    `yaml:"donchian_entry"`
	DonchianExit  int    `yaml:"donchian_exit"`
	DonchianAlt   int    `yaml:"donchian_alt"`
	ATRPeriod    int     `yaml:"atr_period"`
	ATRStopMult  float64 `yaml:"atr_stop_mult"`
	RiskPct      float64 `yaml:"risk_pct"`
	SMAFilter    int     `yaml:"sma_filter"`
	UseEMAFilter bool    `yaml:"use_ema_filter"`
}
type VariantB struct {
	Name            string  `yaml:"name"`
	DonchianEntry   int     `yaml:"donchian_entry"`
	DonchianExit    int     `yaml:"donchian_exit"`
	DonchianAlt     int     `yaml:"donchian_alt"`
	ATRPeriod       int     `yaml:"atr_period"`
	ATRStopMult     float64 `yaml:"atr_stop_mult"`
	RiskPct         float64 `yaml:"risk_pct"`
	ADXPeriod       int     `yaml:"adx_period"`
	ADXThreshold    float64 `yaml:"adx_threshold"`
	EMAFast         int     `yaml:"ema_fast"`
	EMASlow         int     `yaml:"ema_slow"`
	VolLookback     int     `yaml:"vol_regime_lookback"`
	VolLowPct       float64 `yaml:"vol_regime_low_pct"`
	VolHighPct      float64 `yaml:"vol_regime_high_pct"`
}
type VariantC struct {
	Name                string  `yaml:"name"`
	DonchianEntry       int     `yaml:"donchian_entry"`
	DonchianExit        int     `yaml:"donchian_exit"`
	ATRPeriod           int     `yaml:"atr_period"`
	ATRStopMult         float64 `yaml:"atr_stop_mult"`
	RiskPct             float64 `yaml:"risk_pct"`
	ADXPeriod           int     `yaml:"adx_period"`
	ADXThreshold        float64 `yaml:"adx_threshold"`
	EMAFast             int     `yaml:"ema_fast"`
	EMASlow             int     `yaml:"ema_slow"`
	FundingZThreshold   float64 `yaml:"funding_z_threshold"`
	FundingZLookback    int     `yaml:"funding_z_lookback"`
	OIDeltaThreshold    float64 `yaml:"oi_delta_threshold"`
	VolumeMult          float64 `yaml:"volume_mult"`
	VolumeSMA           int     `yaml:"volume_sma"`
}
type VariantD struct {
	Name              string  `yaml:"name"`
	DonchianFast      int     `yaml:"donchian_fast"`
	DonchianMid       int     `yaml:"donchian_mid"`
	DonchianSlow      int     `yaml:"donchian_slow"`
	ATRPeriod         int     `yaml:"atr_period"`
	ATRStopMult       float64 `yaml:"atr_stop_mult"`
	RiskPct           float64 `yaml:"risk_pct"`
	VolTargetPct      float64 `yaml:"vol_target_pct"`
	KellyCapPct       float64 `yaml:"kelly_cap_pct"`
	ADXPeriod         int     `yaml:"adx_period"`
	ADXThreshold      float64 `yaml:"adx_threshold"`
	EMAFast           int     `yaml:"ema_fast"`
	EMASlow           int     `yaml:"ema_slow"`
	VolLookback       int     `yaml:"vol_regime_lookback"`
	FundingZThreshold float64 `yaml:"funding_z_threshold"`
	FundingZLookback  int     `yaml:"funding_z_lookback"`
	OIDeltaThreshold  float64 `yaml:"oi_delta_threshold"`
	VolumeMult        float64 `yaml:"volume_mult"`
	VolumeSMA         int     `yaml:"volume_sma"`
	TrailATRMult      float64 `yaml:"trail_atr_mult"`
	TrailMode         string  `yaml:"trail_mode"`
	UseCrashBrake     bool    `yaml:"use_crash_brake"`
	AdaptiveChannel   bool    `yaml:"adaptive_channel"`
}
type Compare struct {
	Variants []string `yaml:"variants"`
	Symbols  []string `yaml:"symbols"`
}
type WalkForward struct {
	Folds      int     `yaml:"folds"`
	TrainRatio float64 `yaml:"train_ratio"`
	Metric     string  `yaml:"metric"`
	Anchored   bool    `yaml:"anchored"`
}
type MonteCarlo struct {
	Runs             int     `yaml:"runs"`
	PerturbationPct  float64 `yaml:"perturbation_pct"`
	BlockBootstrap   bool    `yaml:"block_bootstrap"`
	BlockSize        int     `yaml:"block_size"`
	Seed             int64   `yaml:"seed"`
}
type Report struct {
	TitlePrefix        string   `yaml:"title_prefix"`
	IncludeTrades      bool     `yaml:"include_trades"`
	MaxTradesInTable   int      `yaml:"max_trades_in_table"`
	Theme              string   `yaml:"theme"`
	EmbedLightweightCharts bool `yaml:"embed_lightweight_charts"`
	ComparisonMetrics  []string `yaml:"comparison_metrics"`
}
type Data struct {
	BinanceBase       string `yaml:"binance_base"`
	BinanceVisionBase string `yaml:"binance_vision_base"`
	CacheTTLSeconds   int    `yaml:"cache_ttl_seconds"`
	AlignFunding      bool   `yaml:"align_funding"`
	AlignOI           bool   `yaml:"align_oi"`
	OIPeriod          string `yaml:"oi_period"`
}
type Orderly struct {
	Mainnet    string            `yaml:"mainnet"`
	Testnet    string            `yaml:"testnet"`
	WSMainnet  string            `yaml:"ws_mainnet"`
	DefaultChain string          `yaml:"default_chain"`
	SymbolsMap map[string]string `yaml:"symbols_map"`
	Leverage   int               `yaml:"leverage"`
	SlippageBps int              `yaml:"slippage_bps"`
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("yaml unmarshal: %w", err)
	}
	if c.General.InitialCapital == 0 {
		c.General.InitialCapital = 10000
	}
	return &c, nil
}

func DefaultPath() string {
	if _, err := os.Stat("configs/default.yaml"); err == nil {
		return "configs/default.yaml"
	}
	// try relative to binary
	return "/mnt/1e428cbf-3065-4949-bd32-5716ca8eb8f3/turtle-extended/configs/default.yaml"
}

// ── Helpers per nuovo spec risk (base/min/max) + portfolio heat + skew ──

func (c *Config) RiskBasePct() float64 {
	if c.Risk.Base != 0 {
		return c.Risk.Base * 100
	}
	if c.Risk.MaxRiskPerTradePct != 0 {
		// legacy: usa ~ metà del max come base (conservative)
		return c.Risk.MaxRiskPerTradePct * 0.5
	}
	return 1.0
}
func (c *Config) RiskMinPct() float64 {
	if c.Risk.Min != 0 {
		return c.Risk.Min * 100
	}
	return 0.25
}
func (c *Config) RiskMaxPct() float64 {
	if c.Risk.Max != 0 {
		return c.Risk.Max * 100
	}
	if c.Risk.MaxRiskPerTradePct != 0 {
		return c.Risk.MaxRiskPerTradePct
	}
	return 2.0
}
func (c *Config) PortfolioMaxOpenRiskPct() float64 {
	if c.Portfolio.MaxOpenRisk != 0 {
		return c.Portfolio.MaxOpenRisk * 100
	}
	if c.Portfolio.MaxPortfolioHeatPct != 0 {
		return c.Portfolio.MaxPortfolioHeatPct
	}
	if c.Risk.MaxHeatPct != 0 {
		return c.Risk.MaxHeatPct
	}
	return 3.0
}
func (c *Config) PortfolioMaxCorrelatedRiskPct() float64 {
	if c.Portfolio.MaxCorrelatedRisk != 0 {
		return c.Portfolio.MaxCorrelatedRisk * 100
	}
	return 2.0
}
func (c *Config) LeverageMax() float64 {
	if c.LeverageCfg.Max != 0 {
		return c.LeverageCfg.Max
	}
	if c.Risk.MaxLeverage != 0 {
		return c.Risk.MaxLeverage
	}
	if c.Costs.Leverage != 0 {
		return c.Costs.Leverage
	}
	return 5.0
}
func (c *Config) IsVolatilityAdaptive() bool {
	if c.Volatility.AdaptiveRisk {
		return true
	}
	// legacy fallback: if vol_target_pct defined
	return c.Risk.VolTargetPct != 0
}
func (c *Config) IsDrawdownAdaptive() bool {
	if c.Drawdown.AdaptiveRisk {
		return true
	}
	return c.Risk.DDDeleverageStart != 0
}
func (c *Config) IsFundingFilter() bool {
	if c.Funding.Filter {
		return true
	}
	// legacy
	return c.VariantD.FundingZThreshold != 0
}
func (c *Config) IsOpenInterestFilter() bool {
	if c.OpenInterest.Filter {
		return true
	}
	return c.VariantD.OIDeltaThreshold != 0
}
func (c *Config) IsRegimeFilter() bool {
	if c.Regime.BtcFilter {
		return true
	}
	return c.VariantB.ADXThreshold != 0
}
