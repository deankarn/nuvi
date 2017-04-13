package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"time"

	"github.com/go-redis/redis"
)

const (
	url          = "http://bitly.com/nuvi-plz"
	xmlList      = "NEWS_XML"
	xmlLatestKey = "NEWS_XML_LATEST"
)

var (
	timeout   = time.Second * 5
	hrefRegex = regexp.MustCompile(`href="(.+\.zip)"`)
	latest    = ""
)

type fileDocuments struct {
	filename  string
	documents []string
}

type fileDownload struct {
	filename string
	url      string
}

func main() {

	start := time.Now()

	client := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	cmd := client.Get(xmlLatestKey)
	if cmd.Err() == nil && cmd.Err() != redis.Nil {
		latest = cmd.Val()
	}

	fmt.Println("LATEST:", latest)

	hrefs, err := getPosts()
	if err != nil {
		log.Fatal(err)
	}

	if len(hrefs) == 0 {
		log.Println("No new posts found")
		os.Exit(0)
	}

	posts := downloadFiles(hrefs)

	save(client, posts)

	fmt.Println(time.Now().Sub(start))
}

func save(client *redis.Client, posts chan *fileDocuments) {

	for post := range posts {

		values := make([]interface{}, len(post.documents))

		for i := 0; i < len(post.documents); i++ {
			values[i] = post.documents[i]
		}

		err := client.Watch(func(tx *redis.Tx) error {

			result := tx.LPush(xmlList, values...)
			if result.Err() != nil && result.Err() != redis.Nil {
				return result.Err()
			}

			result2 := tx.Set(xmlLatestKey, post.filename, 0)

			return result2.Err()

		}, xmlList, xmlLatestKey)

		if err == redis.TxFailedErr {
			log.Fatal(err)
		}
	}
}

func downloadFiles(hrefs []fileDownload) (posts chan *fileDocuments) {

	fmt.Println("Begin downloading files...")

	posts = make(chan *fileDocuments)

	go func() {

		defer close(posts)

		for i := 0; i < len(hrefs); i++ {

			xmlContents, err := download(hrefs[i])
			if err != nil {
				// save to DB for retry later
				log.Println(err)
			}

			posts <- xmlContents
		}

	}()

	return
}

func download(df fileDownload) (*fileDocuments, error) {

	fmt.Println("Downloading file ", df.url)

	res, err := http.Get(df.url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	// wish I could just feed res.Body into unzip
	// but can't the way zip works. If we don't have
	// this much memory available write zip to disk
	allXML, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	zipReader, err := zip.NewReader(bytes.NewReader(allXML), int64(len(allXML)))
	if err != nil {
		return nil, err
	}

	var files []string

	for _, file := range zipReader.File {

		if file.FileInfo().IsDir() {
			continue
		}

		f, err := file.Open()
		if err != nil {
			// save to DB for retry later
			log.Println(err)
			continue
		}
		defer f.Close()

		b, err := ioutil.ReadAll(f)
		if err != nil {
			// save to DB for retry later
			log.Println(err)
			continue
		}

		files = append(files, string(b))
	}

	return &fileDocuments{filename: df.filename, documents: files}, nil
}

func getPosts() ([]fileDownload, error) {

	fmt.Print("Retrieving Posts...")

	client := http.Client{
		Timeout: timeout,
	}

	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// if response body become too large, create a lexer to
	// parse out href's to avoid memory overhead
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	page := string(b)

	// find all href's in HTML
	hrefs := hrefRegex.FindAllStringSubmatch(page, -1)
	if len(hrefs) == 0 {
		return nil, nil
	}

	// just for testing
	hrefs = hrefs[:1]

	results := make([]fileDownload, 0, len(hrefs))
	finalURL := resp.Request.URL.String() // grabbing final URL just in case there were redirects during Get

	for _, matches := range hrefs {

		// skip already processed files.
		if matches[1] <= latest {
			continue
		}

		results = append(results, fileDownload{filename: matches[1], url: finalURL + matches[1]})
	}

	// don't assume they are sorted in the order you want
	// retrieving from an external source after all.
	sort.Slice(results, func(i, j int) bool {
		return results[i].filename < results[j].filename
	})

	fmt.Println("Complete")

	return results, nil
}
