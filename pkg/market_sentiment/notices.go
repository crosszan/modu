package market_sentiment

import (
	"sort"
	"strings"
)

// BuildDataNotices turns source-level errors into user-facing impact and
// recovery guidance. Detail keeps the original error for diagnostics.
func BuildDataNotices(sourceErrors map[string]string) []DataNotice {
	if len(sourceErrors) == 0 {
		return nil
	}
	notices := make([]DataNotice, 0, len(sourceErrors))
	handled := map[string]bool{}
	if cacheMessage, ok := sourceErrors["eastmoney_industries_cache"]; ok {
		detail := sourceErrors["eastmoney_industries"]
		if detail == "" {
			detail = cacheMessage
		}
		notices = append(notices, DataNotice{
			Key:        "eastmoney_industries",
			Title:      "行业实时数据失败，已使用缓存",
			Impact:     "市场广度和板块扩散不是本次刷新的实时行业数据。",
			Fallback:   cacheMessage + "；两个分项继续计算并标记为 proxy。",
			Suggestion: "稍后再次刷新；接口恢复后会自动改回实时数据，无需删除缓存。",
			Detail:     detail,
		})
		handled["eastmoney_industries"] = true
		handled["eastmoney_industries_cache"] = true
	}

	keys := make([]string, 0, len(sourceErrors))
	for key := range sourceErrors {
		if !handled[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		notices = append(notices, dataNoticeForError(key, sourceErrors[key]))
	}
	return notices
}

func dataNoticeForError(key, detail string) DataNotice {
	notice := DataNotice{
		Key:        key,
		Title:      "数据源请求失败",
		Impact:     "对应的辅助数据不会展示，其余可用数据继续计算。",
		Fallback:   "系统保留其他来源的结果，不让单个错误终止刷新。",
		Suggestion: "稍后再次刷新；连续失败时检查网络或查看技术详情。",
		Detail:     detail,
	}
	switch {
	case key == "eastmoney_industries":
		notice.Title = "行业数据获取失败"
		notice.Impact = "市场广度和板块扩散缺少行业涨跌与成分股涨跌家数。"
		notice.Fallback = "主域名和备用域名均已尝试；没有历史缓存时，两个分项使用 50 分并标记为 missing。"
		notice.Suggestion = "稍后再次刷新；连续出现 EOF 或超时时可更换网络。"
	case key == "ths_hot_stocks":
		notice.Title = "同花顺强势股数据获取失败"
		notice.Impact = "涨跌停情绪和赚钱效应缺少当日强势股样本。"
		notice.Fallback = "两个分项使用 50 分并标记为 missing。"
		notice.Suggestion = "确认当前是交易日或稍后再次刷新。"
	case key == "tencent_quotes":
		notice.Title = "腾讯实时行情获取失败"
		notice.Impact = "主要指数表可能为空，交易日识别会退回本机日期。"
		notice.Fallback = "九分项仍会尝试使用已取得的历史 K 线和其他数据源。"
		notice.Suggestion = "稍后再次刷新；连续失败时检查是否能访问 qt.gtimg.cn。"
	case strings.HasPrefix(key, "tencent_history_"):
		name := tencentHistoryName(strings.TrimPrefix(key, "tencent_history_"))
		notice.Title = name + "获取失败"
		notice.Impact = "依赖该历史序列的波动、成交、价格强度或风险偏好分项可能缺少样本。"
		notice.Fallback = "样本不足的分项使用 50 分并标记为 missing；其他分项继续计算。"
		notice.Suggestion = "稍后再次刷新；连续失败时检查腾讯 K 线接口连通性。"
	case key == "northbound":
		notice.Title = "北向资金分钟数据获取失败"
		notice.Impact = "页面和 Agent 解读不展示本次北向资金数据。"
		notice.Fallback = "北向资金不参与九分项总分，指数数值不受影响。"
		notice.Suggestion = "交易时段外返回空可能是正常现象；可在交易时段再次刷新。"
	case key == "eastmoney_dragon_tiger":
		notice.Title = "龙虎榜数据获取失败"
		notice.Impact = "页面和 Agent 解读缺少本次龙虎榜记录。"
		notice.Fallback = "龙虎榜不参与九分项总分，指数数值不受影响。"
		notice.Suggestion = "盘后数据可能延迟；稍后再次刷新。"
	case key == "eastmoney_news":
		notice.Title = "财经资讯获取失败"
		notice.Impact = "Agent 解读缺少最新资讯上下文。"
		notice.Fallback = "资讯不参与九分项总分，指数数值不受影响。"
		notice.Suggestion = "稍后再次刷新；连续失败时可更换网络。"
	case key == "agent_explanation":
		notice.Title = "Agent 解读生成失败"
		notice.Impact = "本次没有使用模型生成市场解读。"
		notice.Fallback = "页面已自动使用确定性规则解读，指数数值不受影响。"
		notice.Suggestion = "检查模型地址、模型名和 API Key 后再次刷新。"
	}
	return notice
}

func tencentHistoryName(key string) string {
	names := map[string]string{
		IndexShanghai: "上证指数历史行情",
		IndexHS300:    "沪深300历史行情",
		IndexCSI1000:  "中证1000历史行情",
		IndexChiNext:  "创业板指历史行情",
		ETF50:         "上证50ETF历史行情",
		ETFBond:       "国债ETF历史行情",
	}
	if name := names[key]; name != "" {
		return name
	}
	return "腾讯历史行情"
}
