package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

type requestContextKey string

const (
	requestMetadataKey requestContextKey = "request_metadata"
	headerRequestID    string            = "X-Request-ID"
	headerChainID      string            = "X-Chain-ID"
	headerCorrelation  string            = "X-Correlation-ID"
	serviceName        string            = "templates-registry"
	componentHTTP      string            = "httpapi"
)

type requestMetadata struct {
	RequestID string
	ChainID   string
	Action    string
	Method    string
	Path      string
	User      string
}

// withRequestMetadata обогащает контекст запроса стабильными полями для логирования.
func (s *Server) withRequestMetadata(r *http.Request) (*http.Request, requestMetadata) {
	requestID := strings.TrimSpace(r.Header.Get(headerRequestID))
	if requestID == "" {
		requestID = uuid.NewString()
	}

	chainID := strings.TrimSpace(r.Header.Get(headerChainID))
	if chainID == "" {
		chainID = strings.TrimSpace(r.Header.Get(headerCorrelation))
	}
	if chainID == "" {
		chainID = requestID
	}

	meta := requestMetadata{
		RequestID: requestID,
		ChainID:   chainID,
		Action:    inferAction(r),
		Method:    r.Method,
		Path:      r.URL.Path,
		User:      s.actorFromRequest(r),
	}

	ctx := context.WithValue(r.Context(), requestMetadataKey, meta)
	r = r.WithContext(ctx)
	r.Header.Set(headerRequestID, requestID)
	r.Header.Set(headerChainID, chainID)

	return r, meta
}

// requestMetaFromContext возвращает метаданные запроса из контекста, если они есть.
func requestMetaFromContext(ctx context.Context) (requestMetadata, bool) {
	meta, ok := ctx.Value(requestMetadataKey).(requestMetadata)
	return meta, ok
}

// logRequestStarted пишет структурированный лог о начале обработки запроса.
func logRequestStarted(meta requestMetadata, remoteAddr string) {
	slog.Info(
		"Начата обработка HTTP-запроса",
		slog.String("service", serviceName),
		slog.String("component", componentHTTP),
		slog.String("event", "request_started"),
		slog.String("request_id", meta.RequestID),
		slog.String("chain_id", meta.ChainID),
		slog.String("action", meta.Action),
		slog.String("method", meta.Method),
		slog.String("path", meta.Path),
		slog.String("user", meta.User),
		slog.String("remote_addr", remoteAddr),
	)
}

// logRequestCompleted пишет структурированный лог об окончании обработки запроса.
func logRequestCompleted(meta requestMetadata, statusCode int, bytesWritten int, duration time.Duration) {
	slog.Info(
		"HTTP-запрос обработан",
		slog.String("service", serviceName),
		slog.String("component", componentHTTP),
		slog.String("event", "request_completed"),
		slog.String("request_id", meta.RequestID),
		slog.String("chain_id", meta.ChainID),
		slog.String("action", meta.Action),
		slog.String("method", meta.Method),
		slog.String("path", meta.Path),
		slog.String("user", meta.User),
		slog.Int("status_code", statusCode),
		slog.Int("bytes_written", bytesWritten),
		slog.Int64("duration_ms", duration.Milliseconds()),
	)
}

// inferAction возвращает стабильное имя действия для анализа в логах.
func inferAction(r *http.Request) string {
	path := strings.TrimSpace(r.URL.Path)
	switch {
	case r.Method == http.MethodPost && path == "/register":
		return "user.register"
	case r.Method == http.MethodPost && path == "/token":
		return "user.login"
	case r.Method == http.MethodGet && path == "/db_version":
		return "db.version.read"
	case r.Method == http.MethodGet && path == "/entities":
		return "entity.list"
	case r.Method == http.MethodPost && path == "/entities":
		return "entity.create"
	case r.Method == http.MethodPost && path == "/entities/templates/batch":
		return "template.batch.read"
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/templates"):
		return "template.history.read"
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/revisions"):
		return "revision.list"
	case r.Method == http.MethodGet && strings.Contains(path, "/revisions/"):
		return "revision.read"
	case r.Method == http.MethodPut && strings.HasPrefix(path, "/entities/"):
		return "entity.update"
	case r.Method == http.MethodDelete && strings.HasPrefix(path, "/entities/"):
		return "entity.delete"
	case r.Method == http.MethodGet && path == "/users":
		return "admin.user.list"
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/ban"):
		return "admin.user.ban"
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/unban"):
		return "admin.user.unban"
	case r.Method == http.MethodPut && strings.HasSuffix(path, "/role"):
		return "admin.user.role.update"
	case r.Method == http.MethodGet && path == "/healthz":
		return "system.health"
	default:
		return strings.ToLower(r.Method) + "." + strings.Trim(strings.ReplaceAll(path, "/", "."), ".")
	}
}
