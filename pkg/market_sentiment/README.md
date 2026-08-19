# market_sentiment

`market_sentiment` 用免费公开数据计算 0–100 的 A 股市场情绪指数，并通过 `pkg/agent` 生成可选的中文解读。数值计算不依赖 LLM、Tushare 或 Python；模型不可用时，指数和网页仍能工作。

## 运行

```bash
go run ./cmd/market_sentiment
```

浏览器访问 `http://127.0.0.1:8088`，点击“获取最新市场数据”。启动过程只读取 `data/market_sentiment/history.json`，点击按钮才会联网。当天重复刷新会覆盖同一交易日记录，不会产生重复历史点。

需要 Agent 解读时配置一个 OpenAI Chat Completions 兼容服务：

```bash
export MARKET_SENTIMENT_MODEL=qwen3.5
export MARKET_SENTIMENT_BASE_URL=http://localhost:11434/v1
export MARKET_SENTIMENT_API_KEY=
go run ./cmd/market_sentiment
```

可用环境变量：

| 变量 | 默认值 | 用途 |
|---|---|---|
| `MARKET_SENTIMENT_ADDR` | `127.0.0.1:8088` | HTTP 监听地址 |
| `MARKET_SENTIMENT_CACHE` | `data/market_sentiment/history.json` | 历史缓存路径 |
| `MARKET_SENTIMENT_MODEL` | 空 | 非空时启用 `pkg/agent` 解读 |
| `MARKET_SENTIMENT_BASE_URL` | `http://localhost:11434/v1` | 模型服务地址 |
| `MARKET_SENTIMENT_API_KEY` | 空 | 模型服务 API Key |

## 九分项口径

权重与参考项目一致。没有 Tushare 全 A 历史截面后，无法等价计算的分项明确标为 `proxy`；请求失败或样本不足标为 `missing`，固定使用 50 分。

| 分项 | 权重 | 当前输入 | 状态 |
|---|---:|---|---|
| 波动率情绪 | 15% | 腾讯沪深300；缺失时用上证50ETF，20日年化历史波动率 | `proxy` |
| 成交情绪 | 15% | 腾讯上证指数当日成交量 / 前20日均量 | `proxy` |
| 股价强度情绪 | 10% | 上证、沪深300、中证1000、创业板的20日新高占比与当日收益 | `proxy` |
| 风险偏好情绪 | 10% | 沪深300与国债ETF的20日相对收益 | `ok` |
| 市场广度情绪 | 15% | 东财行业成分股上涨家数 / 上涨与下跌家数 | `ok`；缓存回退时为 `proxy` |
| 涨跌停情绪 | 15% | 同花顺当日强势股数量，80只映射为50分 | `proxy` |
| 赚钱效应 | 10% | 同花顺强势股平均涨幅，10%映射为50分 | `proxy` |
| 板块扩散情绪 | 5% | 东财上涨行业占比 | `ok`；缓存回退时为 `proxy` |
| 风格风险偏好 | 5% | 中证1000相对沪深300的5日收益差 | `ok` |

总分是九个分项的固定加权和。状态区间为 `[0,20)` 极度恐惧、`[20,40)` 恐惧、`[40,60)` 中性、`[60,80)` 贪婪、`[80,100]` 极度贪婪。

## 数据源与失败行为

- 腾讯提供实时行情和复权日 K 线，不限流并优先用于行情数据。
- 同花顺提供当日强势股和北向分钟数据。
- 东方财富只提供行业、龙虎榜和全球资讯。所有东财请求共用一个客户端，严格串行；请求间隔至少 1 秒并增加 100–500ms 抖动，瞬时错误只重试一次。行业主域名失败后，客户端按相同限流规则改用 `push2delay.eastmoney.com`。
- 两个东财行业域名都失败时，服务复用历史文件中最近一次非空行业快照。市场广度和板块扩散继续计算，但状态改为 `proxy`，分项说明和 `industry_data_date` 会暴露数据日期；原始接口错误仍保存在 `Snapshot.Errors`。
- 如果行业接口失败且本地从未缓存过行业数据，相关分项才回落到 50 分并标记 `missing`。其他单个数据源失败也不会让整次刷新失败。
- `Snapshot.DataNotices` 把底层错误转换成数据源名称、影响范围、系统降级和处理建议，`Detail` 保留原始错误。网页默认展示可执行提示，技术详情折叠显示；旧历史文件只有 `Errors` 时，读取过程会自动补全提示。
- 只有腾讯、东财行业和同花顺强势股等所有计分来源都不可用时，刷新才返回错误。

北向、龙虎榜和资讯用于页面与 Agent 解读，不直接进入当前九分项数值，避免临时新闻改变确定性分数。

## 包入口

```go
collector := market_sentiment.NewDefaultCollector(&http.Client{Timeout: 20 * time.Second})
store := market_sentiment.NewFileStore("data/market_sentiment/history.json")
service := market_sentiment.NewService(collector, store, market_sentiment.RuleExplainer{})

snapshot, err := service.Refresh(context.Background())
```

需要接入模型时，把 `RuleExplainer` 换成 `NewAgentExplainer(model, streamFn)`。`streamFn` 传 `nil` 时使用 Modu 已注册 Provider。

## 验证

```bash
go test ./pkg/market_sentiment ./cmd/market_sentiment
go run ./cmd/market_sentiment
curl -X POST http://127.0.0.1:8088/api/refresh
```

## 不做的范围

- 不回填首次运行之前的历史指数。免费接口没有参考项目所需的全 A 252 日截面，伪造历史没有验收价值。
- 不声称 `proxy` 与原项目分项等价。页面和 JSON 都保留每个分项的状态、来源和说明。
- 不让 LLM 修改分数。Agent 只接收已经计算完成的快照并生成解释。
