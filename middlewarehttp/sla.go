package middleware

import (
	"log"
	"net/http"
	"time"
)

func TrackServiceMethodSla(inner http.Handler, sla int64) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		start := time.Now()
		inner.ServeHTTP(w, r)
		difference := time.Since(start).Milliseconds()

		if difference > sla {
			log.Printf("Sla of %v ms was exceeded. Actual execution time: %v ms", sla, difference)
		} else {
			log.Printf("Sla of %v ms was met successfully. Actual execution time: %v ms", sla, difference)
		}

	})
}
