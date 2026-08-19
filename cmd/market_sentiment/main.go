package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	market_sentiment "github.com/openmodu/modu/pkg/market_sentiment"
	"github.com/openmodu/modu/pkg/providers"
	"github.com/openmodu/modu/pkg/providers/openai"
	"github.com/openmodu/modu/pkg/types"
)

func main() {
	addr := flag.String("addr", envOr("MARKET_SENTIMENT_ADDR", "127.0.0.1:8088"), "HTTP listen address")
	cachePath := flag.String("cache", envOr("MARKET_SENTIMENT_CACHE", "data/market_sentiment/history.json"), "history cache path")
	flag.Parse()

	httpClient := &http.Client{Timeout: 20 * time.Second}
	collector := market_sentiment.NewDefaultCollector(httpClient)
	store := market_sentiment.NewFileStore(*cachePath)
	explainer := buildExplainer()
	service := market_sentiment.NewService(collector, store, explainer)

	server := &http.Server{
		Addr:              *addr,
		Handler:           market_sentiment.NewHTTPHandler(service),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	log.Printf("A股市场情绪指数: http://%s", *addr)
	log.Printf("本地缓存: %s", *cachePath)
	if os.Getenv("MARKET_SENTIMENT_MODEL") == "" {
		log.Printf("Agent 解读未配置，使用确定性规则解读；设置 MARKET_SENTIMENT_MODEL 可启用 pkg/agent")
	}
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func buildExplainer() market_sentiment.Explainer {
	modelID := os.Getenv("MARKET_SENTIMENT_MODEL")
	if modelID == "" {
		return market_sentiment.RuleExplainer{}
	}
	baseURL := envOr("MARKET_SENTIMENT_BASE_URL", "http://localhost:11434/v1")
	apiKey := os.Getenv("MARKET_SENTIMENT_API_KEY")
	const providerID = "market-sentiment-openai"
	providers.Register(openai.New(providerID, openai.WithBaseURL(baseURL), openai.WithAPIKey(apiKey)))
	return market_sentiment.NewAgentExplainer(&types.Model{
		ID: modelID, Name: fmt.Sprintf("%s (market sentiment)", modelID), ProviderID: providerID,
	}, nil)
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
