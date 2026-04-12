package server

import (
	"net/http"
	"strings"
)

const connectExposeHeaders = "Grpc-Status,Grpc-Message,Grpc-Status-Details-Bin"

const connectAllowedHeaders = "Content-Type,Connect-Protocol-Version,Connect-Timeout-Ms,Connect-Accept-Encoding,Connect-Content-Encoding,Grpc-Timeout,Grpc-Encoding,Grpc-Accept-Encoding,Grpc-Message,X-Grpc-Web,X-User-Agent,Authorization"

const connectAllowedMethods = "GET,POST,OPTIONS"

func WithCORS(next http.Handler, allowedOrigin string) http.Handler {
	allowedOrigin = strings.TrimSpace(allowedOrigin)
	allowedOrigins := splitAllowedOrigins(allowedOrigin)
	allowAnyOrigin := len(allowedOrigins) == 0

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := normalizeOrigin(r.Header.Get("Origin"))
		if origin != "" {
			if !allowAnyOrigin && !originAllowed(origin, allowedOrigins) {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}

			if allowAnyOrigin {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			w.Header().Add("Vary", "Origin")
			w.Header().Set("Access-Control-Expose-Headers", connectExposeHeaders)
		}

		if r.Method == http.MethodOptions {
			if r.Header.Get("Access-Control-Request-Method") != "" {
				w.Header().Add("Vary", "Access-Control-Request-Method")
				w.Header().Add("Vary", "Access-Control-Request-Headers")
				w.Header().Set("Access-Control-Allow-Methods", connectAllowedMethods)
				w.Header().Set("Access-Control-Allow-Headers", connectAllowedHeaders)
				w.Header().Set("Access-Control-Max-Age", "7200")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func splitAllowedOrigins(raw string) []string {
	if raw == "" || raw == "*" {
		return nil
	}

	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' '
	})
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		part = normalizeOrigin(part)
		if part != "" {
			origins = append(origins, part)
		}
	}
	return origins
}

func originAllowed(origin string, allowedOrigins []string) bool {
	for _, allowedOrigin := range allowedOrigins {
		if allowedOrigin == origin {
			return true
		}
	}
	return false
}

func normalizeOrigin(origin string) string {
	origin = strings.TrimSpace(origin)
	origin = strings.Trim(origin, "\"")
	origin = strings.TrimSuffix(origin, "/")
	return strings.ToLower(origin)
}
