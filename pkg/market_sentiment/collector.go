package market_sentiment

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

type TencentSource interface {
	Quotes(context.Context, []Security) ([]Quote, error)
	History(context.Context, Security, int) ([]Bar, error)
}

type EastMoneySource interface {
	Industries(context.Context) ([]Industry, error)
	DragonTiger(context.Context, string) ([]DragonTigerStock, error)
	GlobalNews(context.Context, int) ([]NewsItem, error)
}

type THSSource interface {
	HotStocks(context.Context, string) ([]HotStock, error)
}

type NorthboundSource interface {
	Realtime(context.Context) ([]NorthboundPoint, error)
}

type Collector struct {
	Tencent     TencentSource
	EastMoney   EastMoneySource
	THS         THSSource
	Northbound  NorthboundSource
	Securities  []Security
	HistoryDays int
	Now         func() time.Time
}

func NewCollector(tencent TencentSource, eastMoney EastMoneySource, ths THSSource, northbound NorthboundSource) *Collector {
	return &Collector{
		Tencent: tencent, EastMoney: eastMoney, THS: ths, Northbound: northbound,
		Securities: append([]Security(nil), DefaultSecurities...), HistoryDays: 40,
		Now: time.Now,
	}
}

func NewDefaultCollector(client *http.Client) *Collector {
	return NewCollector(NewTencentClient(client), NewEastMoneyClient(client), NewTHSClient(client), NewNorthboundClient(client))
}

func (c *Collector) Collect(ctx context.Context) (RawSnapshot, error) {
	raw := RawSnapshot{Histories: map[string][]Bar{}, Errors: map[string]string{}}
	if c.Now == nil {
		c.Now = time.Now
	}
	raw.TradeDate = shanghaiDate(c.Now())

	if c.Tencent != nil {
		quotes, err := c.Tencent.Quotes(ctx, c.Securities)
		if err != nil {
			raw.Errors["tencent_quotes"] = err.Error()
		} else {
			raw.Quotes = quotes
			if date, ok := latestQuoteDate(quotes); ok {
				raw.TradeDate = date
			}
		}
		for _, security := range c.Securities {
			bars, err := c.Tencent.History(ctx, security, c.HistoryDays)
			if err != nil {
				raw.Errors["tencent_history_"+security.Key] = err.Error()
				continue
			}
			raw.Histories[security.Key] = bars
			if raw.TradeDate.IsZero() && len(bars) > 0 {
				if date, err := time.ParseInLocation("2006-01-02", bars[len(bars)-1].Date, shanghaiLocation()); err == nil {
					raw.TradeDate = date
				}
			}
		}
	}

	date := raw.TradeDate.Format("2006-01-02")
	if c.EastMoney != nil {
		industries, err := c.EastMoney.Industries(ctx)
		if err != nil {
			raw.Errors["eastmoney_industries"] = err.Error()
		} else {
			raw.Industries = industries
		}
	}
	if c.THS != nil {
		stocks, err := c.THS.HotStocks(ctx, date)
		if err != nil {
			raw.Errors["ths_hot_stocks"] = err.Error()
		} else {
			raw.HotStocks = stocks
		}
	}
	if c.Northbound != nil {
		points, err := c.Northbound.Realtime(ctx)
		if err != nil {
			raw.Errors["northbound"] = err.Error()
		} else {
			raw.Northbound = points
		}
	}
	if c.EastMoney != nil {
		dragon, err := c.EastMoney.DragonTiger(ctx, date)
		if err != nil {
			raw.Errors["eastmoney_dragon_tiger"] = err.Error()
		} else {
			raw.Dragon = dragon
		}
		news, err := c.EastMoney.GlobalNews(ctx, 20)
		if err != nil {
			raw.Errors["eastmoney_news"] = err.Error()
		} else {
			raw.News = news
		}
	}

	if len(raw.Quotes) == 0 && len(raw.Histories) == 0 && len(raw.Industries) == 0 && len(raw.HotStocks) == 0 {
		return raw, fmt.Errorf("market sentiment: all scoring sources failed: %w", errors.New(joinSourceErrors(raw.Errors)))
	}
	return raw, nil
}

func latestQuoteDate(quotes []Quote) (time.Time, bool) {
	var latest time.Time
	for _, quote := range quotes {
		if len(quote.TradeTime) < 8 {
			continue
		}
		date, err := time.ParseInLocation("20060102", quote.TradeTime[:8], shanghaiLocation())
		if err == nil && date.After(latest) {
			latest = date
		}
	}
	return latest, !latest.IsZero()
}

func shanghaiDate(value time.Time) time.Time {
	local := value.In(shanghaiLocation())
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, shanghaiLocation())
}

func shanghaiLocation() *time.Location {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	return location
}

func joinSourceErrors(sourceErrors map[string]string) string {
	if len(sourceErrors) == 0 {
		return "no source returned data"
	}
	message := ""
	for source, err := range sourceErrors {
		if message != "" {
			message += "; "
		}
		message += source + ": " + err
	}
	return message
}
