package market_sentiment

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

const defaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36"

type TencentClient struct {
	HTTP      *http.Client
	QuoteURL  string
	KlineURL  string
	UserAgent string
}

func NewTencentClient(client *http.Client) *TencentClient {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &TencentClient{
		HTTP:      client,
		QuoteURL:  "https://qt.gtimg.cn/q=%s",
		KlineURL:  "https://web.ifzq.gtimg.cn/appstock/app/fqkline/get?param=%s,day,,,%d,qfq",
		UserAgent: defaultUserAgent,
	}
}

func (c *TencentClient) Quotes(ctx context.Context, securities []Security) ([]Quote, error) {
	if len(securities) == 0 {
		return nil, nil
	}
	symbols := make([]string, 0, len(securities))
	bySymbol := make(map[string]Security, len(securities))
	for _, security := range securities {
		symbols = append(symbols, security.Symbol)
		bySymbol[security.Symbol] = security
	}
	endpoint := fmt.Sprintf(c.QuoteURL, strings.Join(symbols, ","))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build Tencent quote request: %w", err)
	}
	req.Header.Set("User-Agent", c.UserAgent)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request Tencent quotes: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request Tencent quotes: HTTP %d", resp.StatusCode)
	}
	reader := transform.NewReader(io.LimitReader(resp.Body, 2<<20), simplifiedchinese.GBK.NewDecoder())
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("decode Tencent quotes: %w", err)
	}

	quotes := make([]Quote, 0, len(securities))
	for _, line := range strings.Split(string(body), ";") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		equals := strings.IndexByte(line, '=')
		firstQuote := strings.IndexByte(line, '"')
		lastQuote := strings.LastIndexByte(line, '"')
		if equals < 0 || firstQuote < 0 || lastQuote <= firstQuote {
			continue
		}
		variable := strings.TrimSpace(line[:equals])
		symbol := strings.TrimPrefix(variable, "v_")
		security, ok := bySymbol[symbol]
		if !ok {
			continue
		}
		fields := strings.Split(line[firstQuote+1:lastQuote], "~")
		if len(fields) < 35 {
			continue
		}
		quote := Quote{
			Key:          security.Key,
			Symbol:       security.Symbol,
			Name:         stringAt(fields, 1),
			Price:        floatAt(fields, 3),
			Previous:     floatAt(fields, 4),
			Open:         floatAt(fields, 5),
			TradeTime:    stringAt(fields, 30),
			ChangePct:    floatAt(fields, 32),
			High:         floatAt(fields, 33),
			Low:          floatAt(fields, 34),
			AmountWan:    floatAt(fields, 37),
			TurnoverPct:  floatAt(fields, 38),
			AmplitudePct: floatAt(fields, 43),
			VolumeRatio:  floatAt(fields, 49),
		}
		if quote.Name == "" {
			quote.Name = security.Name
		}
		quotes = append(quotes, quote)
	}
	if len(quotes) == 0 {
		return nil, errors.New("Tencent quotes returned no usable rows")
	}
	return quotes, nil
}

func (c *TencentClient) History(ctx context.Context, security Security, limit int) ([]Bar, error) {
	if limit <= 0 {
		limit = 30
	}
	endpoint := fmt.Sprintf(c.KlineURL, security.Symbol, limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build Tencent history request: %w", err)
	}
	req.Header.Set("User-Agent", c.UserAgent)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request Tencent history for %s: %w", security.Symbol, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request Tencent history for %s: HTTP %d", security.Symbol, resp.StatusCode)
	}
	var payload struct {
		Code int `json:"code"`
		Data map[string]struct {
			Day    [][]any `json:"day"`
			QFQDay [][]any `json:"qfqday"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode Tencent history for %s: %w", security.Symbol, err)
	}
	if payload.Code != 0 {
		return nil, fmt.Errorf("Tencent history for %s returned code %d", security.Symbol, payload.Code)
	}
	series, ok := payload.Data[security.Symbol]
	if !ok {
		return nil, fmt.Errorf("Tencent history for %s is missing", security.Symbol)
	}
	rows := series.Day
	if len(rows) == 0 {
		rows = series.QFQDay
	}
	bars := make([]Bar, 0, len(rows))
	for _, row := range rows {
		if len(row) < 6 {
			continue
		}
		bars = append(bars, Bar{
			Date:   valueString(row[0]),
			Open:   valueFloat(row[1]),
			Close:  valueFloat(row[2]),
			High:   valueFloat(row[3]),
			Low:    valueFloat(row[4]),
			Volume: valueFloat(row[5]),
		})
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("Tencent history for %s returned no usable rows", security.Symbol)
	}
	return bars, nil
}

type EastMoneyClient struct {
	HTTP                *http.Client
	IndustryURL         string
	IndustryFallbackURL string
	DataCenterURL       string
	NewsURL             string
	MinInterval         time.Duration
	Jitter              func() time.Duration

	mu       sync.Mutex
	lastCall time.Time
}

func NewEastMoneyClient(client *http.Client) *EastMoneyClient {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &EastMoneyClient{
		HTTP:                client,
		IndustryURL:         "https://push2.eastmoney.com/api/qt/clist/get",
		IndustryFallbackURL: "https://push2delay.eastmoney.com/api/qt/clist/get",
		DataCenterURL:       "https://datacenter-web.eastmoney.com/api/data/v1/get",
		NewsURL:             "https://np-weblist.eastmoney.com/comm/web/getFastNewsList",
		MinInterval:         time.Second,
		Jitter:              eastMoneyJitter,
	}
}

func (c *EastMoneyClient) Industries(ctx context.Context) ([]Industry, error) {
	industries, primaryErr := c.industriesFromURL(ctx, c.IndustryURL)
	if primaryErr == nil {
		return industries, nil
	}
	if c.IndustryFallbackURL == "" || c.IndustryFallbackURL == c.IndustryURL {
		return nil, primaryErr
	}
	industries, fallbackErr := c.industriesFromURL(ctx, c.IndustryFallbackURL)
	if fallbackErr == nil {
		return industries, nil
	}
	return nil, fmt.Errorf("Eastmoney industries: primary: %v; fallback: %w", primaryErr, fallbackErr)
}

func (c *EastMoneyClient) industriesFromURL(ctx context.Context, endpoint string) ([]Industry, error) {
	params := url.Values{
		"pn": {"1"}, "pz": {"100"}, "po": {"1"}, "np": {"1"},
		"fltt": {"2"}, "invt": {"2"}, "fs": {"m:90+t:2"},
		"fields": {"f2,f3,f4,f12,f13,f14,f104,f105,f128,f136,f140,f141,f207"},
	}
	body, err := c.get(ctx, endpoint, params, "https://quote.eastmoney.com/")
	if err != nil {
		return nil, err
	}
	var payload struct {
		Data struct {
			Diff json.RawMessage `json:"diff"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode Eastmoney industries: %w", err)
	}
	rows, err := rawObjectRows(payload.Data.Diff)
	if err != nil {
		return nil, fmt.Errorf("decode Eastmoney industry rows: %w", err)
	}
	industries := make([]Industry, 0, len(rows))
	for _, row := range rows {
		industries = append(industries, Industry{
			Code:         valueString(row["f12"]),
			Name:         valueString(row["f14"]),
			ChangePct:    valueFloat(row["f3"]),
			UpCount:      valueInt(row["f104"]),
			DownCount:    valueInt(row["f105"]),
			Leader:       valueString(row["f140"]),
			LeaderChange: valueFloat(row["f136"]),
		})
	}
	if len(industries) == 0 {
		return nil, errors.New("Eastmoney industries returned no usable rows")
	}
	return industries, nil
}

func (c *EastMoneyClient) DragonTiger(ctx context.Context, tradeDate string) ([]DragonTigerStock, error) {
	params := url.Values{
		"reportName": {"RPT_DAILYBILLBOARD_DETAILSNEW"},
		"columns":    {"ALL"},
		"filter":     {fmt.Sprintf("(TRADE_DATE>='%s')(TRADE_DATE<='%s')", tradeDate, tradeDate)},
		"pageNumber": {"1"}, "pageSize": {"500"},
		"sortColumns": {"BILLBOARD_NET_AMT"}, "sortTypes": {"-1"},
		"source": {"WEB"}, "client": {"WEB"},
	}
	body, err := c.get(ctx, c.DataCenterURL, params, "https://data.eastmoney.com/")
	if err != nil {
		return nil, err
	}
	var payload struct {
		Result struct {
			Data []map[string]any `json:"data"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode Eastmoney dragon tiger: %w", err)
	}
	rows := make([]DragonTigerStock, 0, len(payload.Result.Data))
	for _, row := range payload.Result.Data {
		rows = append(rows, DragonTigerStock{
			Code:        valueString(row["SECURITY_CODE"]),
			Name:        valueString(row["SECURITY_NAME_ABBR"]),
			Reason:      valueString(row["EXPLANATION"]),
			ChangePct:   valueFloat(row["CHANGE_RATE"]),
			NetBuyWan:   valueFloat(row["BILLBOARD_NET_AMT"]) / 10000,
			TurnoverPct: valueFloat(row["TURNOVERRATE"]),
		})
	}
	return rows, nil
}

func (c *EastMoneyClient) GlobalNews(ctx context.Context, pageSize int) ([]NewsItem, error) {
	if pageSize <= 0 {
		pageSize = 20
	}
	params := url.Values{
		"client": {"web"}, "biz": {"web_724"}, "fastColumn": {"102"},
		"sortEnd": {""}, "pageSize": {strconv.Itoa(pageSize)}, "req_trace": {uuid.NewString()},
	}
	body, err := c.get(ctx, c.NewsURL, params, "https://kuaixun.eastmoney.com/")
	if err != nil {
		return nil, err
	}
	var payload struct {
		Data struct {
			Items []map[string]any `json:"fastNewsList"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode Eastmoney news: %w", err)
	}
	items := make([]NewsItem, 0, len(payload.Data.Items))
	for _, item := range payload.Data.Items {
		items = append(items, NewsItem{Title: valueString(item["title"]), Summary: valueString(item["summary"]), Time: valueString(item["showTime"])})
	}
	return items, nil
}

func (c *EastMoneyClient) get(ctx context.Context, endpoint string, params url.Values, referer string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse Eastmoney URL: %w", err)
	}
	u.RawQuery = params.Encode()
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if err := c.wait(ctx); err != nil {
			return nil, err
		}
		body, err := c.getOnce(ctx, u.String(), referer)
		c.lastCall = time.Now()
		if err == nil {
			return body, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func (c *EastMoneyClient) wait(ctx context.Context) error {
	if c.lastCall.IsZero() {
		return nil
	}
	wait := c.MinInterval - time.Since(c.lastCall)
	if wait <= 0 {
		return nil
	}
	if c.Jitter != nil {
		wait += c.Jitter()
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *EastMoneyClient) getOnce(ctx context.Context, endpoint, referer string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build Eastmoney request: %w", err)
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request Eastmoney: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request Eastmoney: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("read Eastmoney response: %w", err)
	}
	return body, nil
}

type THSClient struct {
	HTTP      *http.Client
	HotURL    string
	UserAgent string
}

func NewTHSClient(client *http.Client) *THSClient {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &THSClient{
		HTTP:      client,
		HotURL:    "http://zx.10jqka.com.cn/event/api/getharden/date/%s/orderby/date/orderway/desc/charset/GBK/",
		UserAgent: defaultUserAgent,
	}
}

func (c *THSClient) HotStocks(ctx context.Context, tradeDate string) ([]HotStock, error) {
	endpoint := fmt.Sprintf(c.HotURL, tradeDate)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build THS hot-stock request: %w", err)
	}
	req.Header.Set("User-Agent", c.UserAgent)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request THS hot stocks: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request THS hot stocks: HTTP %d", resp.StatusCode)
	}
	reader := transform.NewReader(io.LimitReader(resp.Body, 8<<20), simplifiedchinese.GBK.NewDecoder())
	var payload struct {
		ErrorCode any              `json:"errocode"`
		ErrorMsg  string           `json:"errormsg"`
		Data      []map[string]any `json:"data"`
	}
	if err := json.NewDecoder(reader).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode THS hot stocks: %w", err)
	}
	if valueInt(payload.ErrorCode) != 0 {
		return nil, fmt.Errorf("THS hot stocks: %s", payload.ErrorMsg)
	}
	stocks := make([]HotStock, 0, len(payload.Data))
	for _, row := range payload.Data {
		stocks = append(stocks, HotStock{
			Code: valueString(row["code"]), Name: valueString(row["name"]),
			Reason: valueString(row["reason"]), ChangePct: valueFloat(row["zhangfu"]),
			TurnoverPct: valueFloat(row["huanshou"]), Amount: valueFloat(row["chengjiaoe"]),
		})
	}
	return stocks, nil
}

type NorthboundClient struct {
	HTTP *http.Client
	URL  string
}

func NewNorthboundClient(client *http.Client) *NorthboundClient {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &NorthboundClient{HTTP: client, URL: "https://data.hexin.cn/market/hsgtApi/method/dayChart/"}
}

func (c *NorthboundClient) Realtime(ctx context.Context) ([]NorthboundPoint, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("build northbound request: %w", err)
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("Referer", "https://data.hexin.cn/")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request northbound flow: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request northbound flow: HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Times []any `json:"time"`
		HGT   []any `json:"hgt"`
		SGT   []any `json:"sgt"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode northbound flow: %w", err)
	}
	points := make([]NorthboundPoint, 0, len(payload.Times))
	for i, item := range payload.Times {
		point := NorthboundPoint{Time: valueString(item)}
		if i < len(payload.HGT) {
			point.HGT = valueFloat(payload.HGT[i])
		}
		if i < len(payload.SGT) {
			point.SGT = valueFloat(payload.SGT[i])
		}
		points = append(points, point)
	}
	return points, nil
}

func rawObjectRows(raw json.RawMessage) ([]map[string]any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var list []map[string]any
	if err := json.Unmarshal(raw, &list); err == nil {
		return list, nil
	}
	var object map[string]map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, err
	}
	list = make([]map[string]any, 0, len(object))
	for _, row := range object {
		list = append(list, row)
	}
	return list, nil
}

func stringAt(values []string, index int) string {
	if index < 0 || index >= len(values) {
		return ""
	}
	return strings.TrimSpace(values[index])
}

func floatAt(values []string, index int) float64 {
	return valueFloat(stringAt(values, index))
}

func valueString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func valueFloat(value any) float64 {
	text := strings.TrimSpace(valueString(value))
	if text == "" || text == "-" {
		return 0
	}
	number, _ := strconv.ParseFloat(strings.ReplaceAll(text, ",", ""), 64)
	return number
}

func valueInt(value any) int { return int(valueFloat(value)) }

func eastMoneyJitter() time.Duration {
	var value [1]byte
	if _, err := cryptorand.Read(value[:]); err != nil {
		return 250 * time.Millisecond
	}
	return time.Duration(100+int(value[0])%401) * time.Millisecond
}
