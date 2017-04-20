package main

import (
	"archive/zip"
	"bytes"
	"context"
	"io/ioutil"
	stdlog "log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"sync"
	"time"

	"strings"

	"net/url"

	"github.com/go-redis/redis"
	download "github.com/joeybloggs/go-download"
	"github.com/namsral/flag" // checks flags + ENV variables
)

const (
	xmlList      = "NEWS_XML"
	xmlLatestKey = "NEWS_XML_LATEST"
)

var (
	downloadSite string
	maxDownloads int
	timeout      = time.Second * 5
	hrefRegex    = regexp.MustCompile(`href="(.+\.zip)"`)
	latest       = ""
	log          = stdlog.New(os.Stdout, "", stdlog.Ldate|stdlog.Ltime|stdlog.Lshortfile)
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

	flag.StringVar(&downloadSite, "url", "http://bitly.com/nuvi-plz", "URL to dowload files from")
	flag.IntVar(&maxDownloads, "maxdownloads", 2, "Maximum parallel downloads")
	flag.Parse()

	if maxDownloads < 1 {
		log.Fatal("Must specify at least one download.")
	}

	_, err := url.Parse(downloadSite)
	if err != nil {
		log.Fatalf("Invalid url specified '%s'", downloadSite)
	}

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

	log.Println("LATEST:", latest)

	hrefs, err := getPosts(downloadSite)
	if err != nil {
		log.Fatal(err)
	}

	if len(hrefs) == 0 {
		log.Println("No new posts found")
		os.Exit(0)
	}

	// ctx, cancel := context.WithCancel(context.Background())
	// defer cancel()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*2)
	defer cancel()

	posts := downloadParallel(ctx, uint(maxDownloads), hrefs)

	save(client, posts)

	log.Println(time.Now().Sub(start))
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

		log.Println("Saved:", post.filename)
	}
}

func downloadFile(df fileDownload) (*fileDocuments, error) {

	log.Println("Downloading file ", df.url)

	options := &download.Options{
		Concurrency: func(size int64) int {
			return 20
		},
	}

	f, err := download.Open(df.url, options)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// wish I could just feed res.Body into unzip
	// but can't the way zip works. If we don't have
	// this much memory available write zip to disk
	allXML, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}

	// fi, _ := f.Stat()
	// fmt.Println("LEN:", df.url, len(allXML), fi.Size())
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

	log.Println("Download Complete for ", df.filename)

	return &fileDocuments{filename: df.filename, documents: files}, nil
}

func getPosts(url string) ([]fileDownload, error) {

	log.Print("Retrieving Posts...")

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

	results := make([]fileDownload, 0, len(hrefs))
	finalURL := strings.TrimRight(resp.Request.URL.String(), "/") + "/" // grabbing final URL just in case there were redirects during Get

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

	log.Println("Complete", len(results))

	return results, nil
}

// downloadPrallel downloads files in parallel to a max of the 'maxDownload' property at a time
// additionally a backlog for FIFO logic has been implemented so that the files will still be saved
// to the database in cronological order regardless of the order the downloads complete.
func downloadParallel(ctx context.Context, maxDownloads uint, files []fileDownload) (ch chan *fileDocuments) {

	ch = make(chan *fileDocuments)
	dlChan := make(chan fileDownload)

	go func() {

		defer close(dlChan)

		for _, file := range files {
			select {
			case <-ctx.Done():
				return
			case dlChan <- file:
			}
		}
	}()

	idx := 0
	backlog := make(map[int]*fileDocuments)
	m := sync.Mutex{}
	wg := sync.WaitGroup{}
	wg.Add(int(maxDownloads))

	go func() {
		wg.Wait()
		close(ch)
	}()

	for i := 0; i < int(maxDownloads); i++ {
		go func() {

			defer wg.Done()

			for {

				m.Lock()

				for {
					if file, ok := backlog[idx]; ok {

						select {
						case <-ctx.Done():
							return
						case ch <- file:
							delete(backlog, idx)
							idx++
							continue
						}
					}

					break
				}

				m.Unlock()

				select {
				case <-ctx.Done():
					return
				case dl := <-dlChan:

					// when closing dlChan an
					// empty entry can be consumed
					if len(dl.url) == 0 {
						break
					}

					var file *fileDocuments
					var err error

					// try and download 3 times, just in case of connections interuption
					j := 0
					for {

						if j == 3 {
							log.Fatal(err)
						}

						file, err = downloadFile(dl)
						if err != nil {
							j++
							continue
						}

						break
					}

					m.Lock()

					if file.filename != files[idx].filename {

						// find idx of file
						for i := 0; i < len(files); i++ {
							if files[i].filename == file.filename {
								backlog[i] = file
								break
							}
						}

						m.Unlock()
						continue
					}

					idx++

					m.Unlock()

					ch <- file
					continue
				}

				break
			}
		}()
	}

	return ch
}
