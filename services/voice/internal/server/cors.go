package server

import (
	"net/http"
	"strings"
)

const connectExposeHeaders = "Grpc-Status,Grpc-Message,Grpc-Status-Details-Bin"

const connectAllowedHeaders = "Content-Type,Connect-Protocol-Version,Connect-Timeout-Ms,Grpc-Timeout,X-Grpc-Web,X-User-Agent,Authorization"

const connectAllowedMethods = "GET,POST,OPTIONS"

func WithCORS(next http.Handler, allowedOrigin string) http.Handler {
	allowedOrigin = strings.TrimSpace(allowedOrigin)
	if allowedOrigin == "" {
		allowedOrigin = "*"
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if allowedOrigin != "*" && origin != allowedOrigin {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}

			if allowedOrigin == "*" {
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