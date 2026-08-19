package market_sentiment

import "time"

const (
	IndexShanghai = "sh"
	IndexHS300    = "hs300"
	IndexCSI1000  = "csi1000"
	IndexChiNext  = "chinext"
	ETF50         = "etf50"
	ETFBond       = "bond"
)

// Status describes whether a score uses the reference input, an explicitly
// documented public-data proxy, or a neutral fallback.
type Status string

const (
	StatusOK      Status = "ok"
	StatusProxy   Status = "proxy"
	StatusMissing Status = "missing"
)

type Security struct {
	Key    string `json:"key"`
	Symbol string `json:"symbol"`
	Name   string `json:"name"`
}

var DefaultSecurities = []Security{
	{Key: IndexShanghai, Symbol: "sh000001", Name: "上证指数"},
	{Key: IndexHS300, Symbol: "sh000300", Name: "沪深300"},
	{Key: IndexCSI1000, Symbol: "sh000852", Name: "中证1000"},
	{Key: IndexChiNext, Symbol: "sz399006", Name: "创业板指"},
	{Key: ETF50, Symbol: "sh510050", Name: "上证50ETF"},
	{Key: ETFBond, Symbol: "sh511010", Name: "国债ETF"},
}

type Quote struct {
	Key          string  `json:"key"`
	Symbol       string  `json:"symbol"`
	Name         string  `json:"name"`
	TradeTime    string  `json:"trade_time,omitempty"`
	Price        float64 `json:"price"`
	Previous     float64 `json:"previous"`
	Open         float64 `json:"open"`
	High         float64 `json:"high"`
	Low          float64 `json:"low"`
	ChangePct    float64 `json:"change_pct"`
	AmountWan    float64 `json:"amount_wan"`
	TurnoverPct  float64 `json:"turnover_pct"`
	AmplitudePct float64 `json:"amplitude_pct"`
	VolumeRatio  float64 `json:"volume_ratio"`
}

type Bar struct {
	Date   string  `json:"date"`
	Open   float64 `json:"open"`
	Close  float64 `json:"close"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Volume float64 `json:"volume"`
}

type Industry struct {
	Code         string  `json:"code"`
	Name         string  `json:"name"`
	ChangePct    float64 `json:"change_pct"`
	UpCount      int     `json:"up_count"`
	DownCount    int     `json:"down_count"`
	Leader       string  `json:"leader,omitempty"`
	LeaderChange float64 `json:"leader_change,omitempty"`
}

type HotStock struct {
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Reason      string  `json:"reason"`
	ChangePct   float64 `json:"change_pct"`
	TurnoverPct float64 `json:"turnover_pct"`
	Amount      float64 `json:"amount"`
}

type NorthboundPoint struct {
	Time string  `json:"time"`
	HGT  float64 `json:"hgt_yi"`
	SGT  float64 `json:"sgt_yi"`
}

type DragonTigerStock struct {
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Reason      string  `json:"reason"`
	ChangePct   float64 `json:"change_pct"`
	NetBuyWan   float64 `json:"net_buy_wan"`
	TurnoverPct float64 `json:"turnover_pct"`
}

type NewsItem struct {
	Title   string `json:"title"`
	Summary string `json:"summary,omitempty"`
	Time    string `json:"time,omitempty"`
}

type DataNotice struct {
	Key        string `json:"key"`
	Title      string `json:"title"`
	Impact     string `json:"impact"`
	Fallback   string `json:"fallback"`
	Suggestion string `json:"suggestion"`
	Detail     string `json:"detail"`
}

// RawSnapshot contains only source facts. The scoring layer does not perform
// network access and can therefore be unit tested independently.
type RawSnapshot struct {
	TradeDate        time.Time          `json:"trade_date"`
	Quotes           []Quote            `json:"quotes"`
	Histories        map[string][]Bar   `json:"histories"`
	Industries       []Industry         `json:"industries"`
	IndustryDataDate string             `json:"industry_data_date,omitempty"`
	IndustryCached   bool               `json:"industry_cached,omitempty"`
	HotStocks        []HotStock         `json:"hot_stocks"`
	Northbound       []NorthboundPoint  `json:"northbound"`
	Dragon           []DragonTigerStock `json:"dragon_tiger"`
	News             []NewsItem         `json:"news"`
	Errors           map[string]string  `json:"errors,omitempty"`
}

type Component struct {
	Key      string  `json:"key"`
	Name     string  `json:"name"`
	Weight   float64 `json:"weight"`
	Score    float64 `json:"score"`
	RawValue float64 `json:"raw_value"`
	Unit     string  `json:"unit,omitempty"`
	Status   Status  `json:"status"`
	Source   string  `json:"source"`
	Message  string  `json:"message,omitempty"`
}

type Snapshot struct {
	TradeDate        string             `json:"trade_date"`
	UpdatedAt        time.Time          `json:"updated_at"`
	Score            float64            `json:"score"`
	State            string             `json:"state"`
	Change           float64            `json:"change"`
	Components       []Component        `json:"components"`
	Quotes           []Quote            `json:"quotes"`
	Industries       []Industry         `json:"industries"`
	IndustryDataDate string             `json:"industry_data_date,omitempty"`
	HotStocks        []HotStock         `json:"hot_stocks"`
	Northbound       *NorthboundPoint   `json:"northbound,omitempty"`
	Dragon           []DragonTigerStock `json:"dragon_tiger"`
	News             []NewsItem         `json:"news"`
	Analysis         string             `json:"analysis"`
	Errors           map[string]string  `json:"errors,omitempty"`
	DataNotices      []DataNotice       `json:"data_notices,omitempty"`
}
