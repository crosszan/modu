package market_sentiment

import (
	"math"
	"sort"
)

const (
	componentVolatility = "volatility_sentiment"
	componentVolume     = "volume_sentiment"
	componentPrice      = "price_strength_sentiment"
	componentRisk       = "risk_appetite_sentiment"
	componentBreadth    = "breadth_sentiment"
	componentLimit      = "limit_sentiment"
	componentProfit     = "profitability_sentiment"
	componentSector     = "sector_sentiment"
	componentStyle      = "style_risk_appetite"
)

var componentDefinitions = []struct {
	key    string
	name   string
	weight float64
}{
	{componentVolatility, "波动率情绪", 0.15},
	{componentVolume, "成交情绪", 0.15},
	{componentPrice, "股价强度情绪", 0.10},
	{componentRisk, "风险偏好情绪", 0.10},
	{componentBreadth, "市场广度情绪", 0.15},
	{componentLimit, "涨跌停情绪", 0.15},
	{componentProfit, "赚钱效应", 0.10},
	{componentSector, "板块扩散情绪", 0.05},
	{componentStyle, "风格风险偏好", 0.05},
}

func CalculateSnapshot(raw RawSnapshot) Snapshot {
	calculated := map[string]Component{
		componentVolatility: volatilityComponent(raw),
		componentVolume:     volumeComponent(raw),
		componentPrice:      priceComponent(raw),
		componentRisk:       riskComponent(raw),
		componentBreadth:    breadthComponent(raw),
		componentLimit:      limitComponent(raw),
		componentProfit:     profitabilityComponent(raw),
		componentSector:     sectorComponent(raw),
		componentStyle:      styleComponent(raw),
	}

	components := make([]Component, 0, len(componentDefinitions))
	total := 0.0
	for _, definition := range componentDefinitions {
		component := calculated[definition.key]
		component.Key = definition.key
		component.Name = definition.name
		component.Weight = definition.weight
		component.Score = round2(clamp(component.Score))
		components = append(components, component)
		total += component.Score * component.Weight
	}

	tradeDate := ""
	if !raw.TradeDate.IsZero() {
		tradeDate = raw.TradeDate.Format("2006-01-02")
	}
	industries := append([]Industry(nil), raw.Industries...)
	sort.SliceStable(industries, func(i, j int) bool { return industries[i].ChangePct > industries[j].ChangePct })
	hot := append([]HotStock(nil), raw.HotStocks...)
	sort.SliceStable(hot, func(i, j int) bool { return hot[i].ChangePct > hot[j].ChangePct })

	var northbound *NorthboundPoint
	if len(raw.Northbound) > 0 {
		point := raw.Northbound[len(raw.Northbound)-1]
		northbound = &point
	}
	errors := cloneErrors(raw.Errors)
	return Snapshot{
		TradeDate:        tradeDate,
		Score:            round2(clamp(total)),
		State:            ClassifyState(total),
		Components:       components,
		Quotes:           append([]Quote(nil), raw.Quotes...),
		Industries:       industries,
		IndustryDataDate: raw.IndustryDataDate,
		HotStocks:        hot,
		Northbound:       northbound,
		Dragon:           append([]DragonTigerStock(nil), raw.Dragon...),
		News:             append([]NewsItem(nil), raw.News...),
		Errors:           errors,
		DataNotices:      BuildDataNotices(errors),
	}
}

func ClassifyState(score float64) string {
	switch {
	case score < 20:
		return "极度恐惧"
	case score < 40:
		return "恐惧"
	case score < 60:
		return "中性"
	case score < 80:
		return "贪婪"
	default:
		return "极度贪婪"
	}
}

func missingComponent(source, message string) Component {
	return Component{Score: 50, Status: StatusMissing, Source: source, Message: message}
}

func volatilityComponent(raw RawSnapshot) Component {
	bars := raw.Histories[IndexHS300]
	if len(bars) < 21 {
		bars = raw.Histories[ETF50]
	}
	volatility, ok := annualizedVolatility(bars, 20)
	if !ok {
		return missingComponent("腾讯历史行情", "至少需要 21 根日线")
	}
	return Component{
		Score:    50 + (0.20-volatility)*250,
		RawValue: round4(volatility),
		Unit:     "年化",
		Status:   StatusProxy,
		Source:   "腾讯沪深300/上证50ETF 20日历史波动率",
		Message:  "无 iVIX 时采用参考项目的历史波动率兜底口径",
	}
}

func volumeComponent(raw RawSnapshot) Component {
	ratio, ok := latestVolumeRatio(raw.Histories[IndexShanghai], 20)
	if !ok {
		return missingComponent("腾讯上证指数历史行情", "至少需要 6 根含成交量日线")
	}
	return Component{
		Score:    50 + (ratio-1)*60,
		RawValue: round4(ratio),
		Unit:     "倍",
		Status:   StatusProxy,
		Source:   "腾讯上证指数成交量/前20日均量",
		Message:  "腾讯 K 线不含全 A 成交额，使用上证指数成交量代理",
	}
}

func priceComponent(raw RawSnapshot) Component {
	keys := []string{IndexShanghai, IndexHS300, IndexCSI1000, IndexChiNext}
	valid, highs := 0, 0
	returns := make([]float64, 0, len(keys))
	for _, key := range keys {
		bars := raw.Histories[key]
		if len(bars) < 20 {
			continue
		}
		valid++
		latest := bars[len(bars)-1].Close
		previousHigh := bars[len(bars)-20].Close
		for _, bar := range bars[len(bars)-20 : len(bars)-1] {
			previousHigh = math.Max(previousHigh, bar.Close)
		}
		if latest >= previousHigh {
			highs++
		}
		if bars[len(bars)-2].Close > 0 {
			returns = append(returns, latest/bars[len(bars)-2].Close-1)
		}
	}
	if valid == 0 {
		return missingComponent("腾讯主要指数历史行情", "主要指数历史样本不足")
	}
	highRatio := float64(highs) / float64(valid)
	dailyScore := 50 + mean(returns)*2500
	return Component{
		Score:    highRatio*60 + clamp(dailyScore)*0.40,
		RawValue: round4(highRatio),
		Unit:     "占比",
		Status:   StatusProxy,
		Source:   "腾讯四大指数20日新高占比",
		Message:  "无全 A 252 日截面，使用四大指数 20 日新高代理",
	}
}

func riskComponent(raw RawSnapshot) Component {
	hsReturn, hsOK := periodReturn(raw.Histories[IndexHS300], 20)
	bondReturn, bondOK := periodReturn(raw.Histories[ETFBond], 20)
	if !hsOK || !bondOK {
		return missingComponent("腾讯沪深300与国债ETF历史行情", "至少需要 21 根日线")
	}
	spread := hsReturn - bondReturn
	return Component{
		Score:    50 + spread*250,
		RawValue: round4(spread),
		Unit:     "20日相对收益",
		Status:   StatusOK,
		Source:   "腾讯沪深300与国债ETF 20日相对收益",
	}
}

func breadthComponent(raw RawSnapshot) Component {
	up, down := 0, 0
	for _, industry := range raw.Industries {
		up += industry.UpCount
		down += industry.DownCount
	}
	if up+down == 0 {
		return missingComponent("东方财富行业板块", "缺少行业上涨/下跌家数")
	}
	ratio := float64(up) / float64(up+down)
	status, message := industryDataStatus(raw)
	return Component{Score: ratio * 100, RawValue: round4(ratio), Unit: "占比", Status: status, Source: "东方财富行业成分股上涨占比", Message: message}
}

func limitComponent(raw RawSnapshot) Component {
	if len(raw.HotStocks) == 0 {
		return missingComponent("同花顺强势股", "当日强势股数据为空")
	}
	count := float64(len(raw.HotStocks))
	return Component{
		Score:    50 + (count-80)*0.625,
		RawValue: count,
		Unit:     "只",
		Status:   StatusProxy,
		Source:   "同花顺当日强势股数量",
		Message:  "无全市场涨跌停截面，使用强势股数量代理",
	}
}

func profitabilityComponent(raw RawSnapshot) Component {
	if len(raw.HotStocks) == 0 {
		return missingComponent("同花顺强势股", "当日强势股数据为空")
	}
	changes := make([]float64, 0, len(raw.HotStocks))
	for _, stock := range raw.HotStocks {
		changes = append(changes, stock.ChangePct)
	}
	avg := mean(changes)
	return Component{
		Score:    (avg - 5) * 10,
		RawValue: round2(avg),
		Unit:     "%",
		Status:   StatusProxy,
		Source:   "同花顺强势股平均涨幅",
		Message:  "无全 A 5 日收益截面，使用强势股平均涨幅代理",
	}
}

func sectorComponent(raw RawSnapshot) Component {
	if len(raw.Industries) == 0 {
		return missingComponent("东方财富行业板块", "行业行情为空")
	}
	up := 0
	for _, industry := range raw.Industries {
		if industry.ChangePct > 0 {
			up++
		}
	}
	ratio := float64(up) / float64(len(raw.Industries))
	status, message := industryDataStatus(raw)
	return Component{Score: ratio * 100, RawValue: round4(ratio), Unit: "占比", Status: status, Source: "东方财富行业上涨占比", Message: message}
}

func industryDataStatus(raw RawSnapshot) (Status, string) {
	if !raw.IndustryCached {
		return StatusOK, ""
	}
	if raw.IndustryDataDate == "" {
		return StatusProxy, "实时行业接口失败，使用最近一次本地缓存"
	}
	return StatusProxy, "实时行业接口失败，使用 " + raw.IndustryDataDate + " 的本地缓存"
}

func styleComponent(raw RawSnapshot) Component {
	small, smallOK := periodReturn(raw.Histories[IndexCSI1000], 5)
	large, largeOK := periodReturn(raw.Histories[IndexHS300], 5)
	if !smallOK || !largeOK {
		return missingComponent("腾讯中证1000与沪深300历史行情", "至少需要 6 根日线")
	}
	spread := small - large
	return Component{Score: 50 + spread*500, RawValue: round4(spread), Unit: "5日相对收益", Status: StatusOK, Source: "腾讯中证1000相对沪深300强弱"}
}

func annualizedVolatility(bars []Bar, window int) (float64, bool) {
	if len(bars) < window+1 {
		return 0, false
	}
	bars = bars[len(bars)-window-1:]
	returns := make([]float64, 0, window)
	for i := 1; i < len(bars); i++ {
		if bars[i-1].Close <= 0 || bars[i].Close <= 0 {
			continue
		}
		returns = append(returns, math.Log(bars[i].Close/bars[i-1].Close))
	}
	if len(returns) < window {
		return 0, false
	}
	avg := mean(returns)
	variance := 0.0
	for _, value := range returns {
		variance += (value - avg) * (value - avg)
	}
	variance /= float64(len(returns) - 1)
	return math.Sqrt(variance) * math.Sqrt(252), true
}

func latestVolumeRatio(bars []Bar, window int) (float64, bool) {
	if len(bars) < 6 || bars[len(bars)-1].Volume <= 0 {
		return 0, false
	}
	start := len(bars) - window - 1
	if start < 0 {
		start = 0
	}
	previous := bars[start : len(bars)-1]
	volumes := make([]float64, 0, len(previous))
	for _, bar := range previous {
		if bar.Volume > 0 {
			volumes = append(volumes, bar.Volume)
		}
	}
	if len(volumes) < 5 {
		return 0, false
	}
	return bars[len(bars)-1].Volume / mean(volumes), true
}

func periodReturn(bars []Bar, days int) (float64, bool) {
	if len(bars) < days+1 {
		return 0, false
	}
	start := bars[len(bars)-days-1].Close
	end := bars[len(bars)-1].Close
	if start <= 0 || end <= 0 {
		return 0, false
	}
	return end/start - 1, true
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	total := 0.0
	for _, value := range values {
		total += value
	}
	return total / float64(len(values))
}

func clamp(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 50
	}
	return math.Max(0, math.Min(100, value))
}

func round2(value float64) float64 { return math.Round(value*100) / 100 }
func round4(value float64) float64 { return math.Round(value*10000) / 10000 }

func cloneErrors(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
