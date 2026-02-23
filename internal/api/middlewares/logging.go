package middlewares

import (
	"net/http"
	"time"

	"github.com/rs/zerolog"
)

func Logging(log zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, r)
			log.Info().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Dur("duration", time.Since(start)).
				Str("correlation_id", GetCorrelationID(r.Context())).
				Msg("http_request")
		})
	}
}
