//go:build !old

package main

import (
	"log"
	"math/rand"
	"net/http"
	"time"
)

func demoBuildLabel() string {
	return "binary profile=new: delay 1–5s, body=ok"
}

func newDemoHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		delay := time.Duration(1+rand.Intn(5)) * time.Second
		log.Printf("%s %s sleep %v (new)", r.Method, r.URL.Path, delay)
		time.Sleep(delay)
		_, _ = w.Write([]byte("ok\n"))
	})
}
