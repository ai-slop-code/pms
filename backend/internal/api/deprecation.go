package api

import "net/http"

func addDeprecatedAPIHeaders(w http.ResponseWriter, message string) {
	w.Header().Set("Deprecation", "true")
	w.Header().Set("Warning", `299 PMS "`+message+`"`)
}

func (s *Server) deprecatedHandler(message string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		addDeprecatedAPIHeaders(w, message)
		next(w, r)
	}
}
