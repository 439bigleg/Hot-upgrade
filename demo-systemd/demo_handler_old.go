//go:build old

package main

import (
	"log"
	"math/rand"
	"net/http"
	"time"
)

func demoBuildLabel() string {
	return "binary profile=old: delay 1–10s, body=hello (systemd demo)"
}

func newDemoHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		delay := time.Duration(1+rand.Intn(10)) * time.Second
		log.Printf("%s %s sleep %v (old)", r.Method, r.URL.Path, delay)
		time.Sleep(delay)
		_, _ = w.Write([]byte("hello\n"))
	})
}
