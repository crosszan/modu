package market_sentiment

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

//go:embed web/index.html
var dashboardHTML []byte

type HTTPService interface {
	Refresh(context.Context) (Snapshot, error)
	Current() (Snapshot, bool, error)
	History() ([]Snapshot, error)
}

func NewHTTPHandler(service HTTPService) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(dashboardHTML)
	})
	mux.HandleFunc("GET /api/current", func(w http.ResponseWriter, r *http.Request) {
		snapshot, ok, err := service.Current()
		if err != nil {
			writeAPIError(w, err, http.StatusInternalServerError)
			return
		}
		if !ok {
			writeAPIError(w, errors.New("本地暂无市场情绪数据"), http.StatusNotFound)
			return
		}
		writeJSON(w, snapshot, http.StatusOK)
	})
	mux.HandleFunc("GET /api/history", func(w http.ResponseWriter, r *http.Request) {
		history, err := service.History()
		if err != nil {
			writeAPIError(w, err, http.StatusInternalServerError)
			return
		}
		writeJSON(w, history, http.StatusOK)
	})
	mux.HandleFunc("POST /api/refresh", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
		defer cancel()
		snapshot, err := service.Refresh(ctx)
		if err != nil {
			writeAPIError(w, err, http.StatusBadGateway)
			return
		}
		writeJSON(w, snapshot, http.StatusOK)
	})
	return securityHeaders(mux)
}

func writeJSON(w http.ResponseWriter, value any, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeAPIError(w http.ResponseWriter, err error, status int) {
	writeJSON(w, map[string]string{"error": err.Error()}, status)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
