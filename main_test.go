package main

import (
	"context"
	"io/ioutil"
	stdlog "log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-redis/redis"
)

// NOTES:
// - Run "go test" to run tests
// - Run "gocov test | gocov report" to report on test converage by file
// - Run "gocov test | gocov annotate -" to report on all code and functions, those ,marked with "MISS" were never called
//
// or
//
// -- may be a good idea to change to output path to somewherelike /tmp
// go test -coverprofile cover.out && go tool cover -html=cover.out -o cover.html

func TestMain(m *testing.M) {

	log = stdlog.New(ioutil.Discard, "", stdlog.Ldate|stdlog.Ltime|stdlog.Lshortfile)

	os.Exit(m.Run())
}

func TestDownload(t *testing.T) {

	fs := http.StripPrefix("/testdata/", http.FileServer(http.Dir("./testdata")))

	mux := http.NewServeMux()
	mux.Handle("/testdata/", fs)

	server := httptest.NewServer(mux)
	defer server.Close()

	req, _ := http.NewRequest(http.MethodGet, server.URL+"/testdata/1491828942693.zip", nil)
	client := &http.Client{}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected '%d' Got '%d'", http.StatusOK, resp.StatusCode)
	}
}

func TestDownloadParallel(t *testing.T) {

	fs := http.StripPrefix("/testdata/", http.FileServer(http.Dir("./testdata")))

	mux := http.NewServeMux()
	mux.HandleFunc("/testdata", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html;charset=UTF-8")
		w.Write([]byte("<a href=\"1491828942693.zip\">1491828942693.zip</a>"))
	})
	mux.Handle("/testdata/", fs)

	server := httptest.NewServer(mux)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hrefs, err := getPosts(server.URL + "/testdata")
	if err != nil {
		t.Fatal(err)
	}

	if len(hrefs) != 1 {
		t.Fatalf("Expected '%d' Got '%d'", 1, len(hrefs))
	}

	var resultFilename string
	var resultLen int

	posts := downloadParallel(ctx, 1, hrefs)

	saveFn := func(client *redis.Client, posts chan *fileDocuments) {

		for post := range posts {
			resultFilename = post.filename
			resultLen = len(post.documents)
		}
	}

	saveFn(nil, posts)

	expected := "1491828942693.zip"

	if resultFilename != expected {
		t.Fatalf("Expected '%s' Got '%s'", expected, resultFilename)
	}

	expectedLen := 300

	if resultLen != expectedLen {
		t.Fatalf("Expected '%d' Got '%d'", expectedLen, resultLen)
	}
}
