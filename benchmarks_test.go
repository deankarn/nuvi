package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-redis/redis"
)

func BenchmarkDownload(b *testing.B) {

	fs := http.StripPrefix("/testdata/", http.FileServer(http.Dir("./testdata")))

	mux := http.NewServeMux()
	mux.HandleFunc("/testdata", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html;charset=UTF-8")
		w.Write([]byte("<a href=\"1491828942693.zip\">1491828942693.zip</a>"))
	})
	mux.Handle("/testdata/", fs)

	server := httptest.NewServer(mux)
	defer server.Close()

	saveFn := func(client *redis.Client, posts chan *fileDocuments) {
		for range posts {
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for n := 0; n < b.N; n++ {

		hrefs, err := getPosts(server.URL + "/testdata")
		if err != nil {
			b.Fatal(err)
		}

		posts := downloadParallel(ctx, 1, hrefs)

		saveFn(nil, posts)
	}
}

func BenchmarkDownloadParallel(b *testing.B) {

	fs := http.StripPrefix("/testdata/", http.FileServer(http.Dir("./testdata")))

	mux := http.NewServeMux()
	mux.HandleFunc("/testdata", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html;charset=UTF-8")
		w.Write([]byte("<a href=\"1491828942693.zip\">1491828942693.zip</a>"))
	})
	mux.Handle("/testdata/", fs)

	server := httptest.NewServer(mux)
	defer server.Close()

	saveFn := func(client *redis.Client, posts chan *fileDocuments) {
		for range posts {
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			hrefs, err := getPosts(server.URL + "/testdata")
			if err != nil {
				b.Fatal(err)
			}

			posts := downloadParallel(ctx, 1, hrefs)

			saveFn(nil, posts)
		}
	})
}
