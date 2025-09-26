package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
)

type task struct {
	bild string
	url  string
}

func main() {
	csvFile := flag.String("csv-file", "", "Path to the CSV file")
	method := flag.String("method", "HEAD", "HTTP method: GET or HEAD")
	parallel := flag.Int("parallel", 1, "Number of parallel requests")
	flag.Parse()

	if *csvFile == "" {
		log.Fatal("Missing --csv-file")
	}
	if *method != "GET" && *method != "HEAD" {
		log.Fatal("Invalid --method; must be GET or HEAD")
	}
	if *parallel <= 0 {
		log.Fatal("--parallel must be positive")
	}

	file, err := os.Open(*csvFile)
	if err != nil {
		log.Fatalf("Failed to open CSV: %v", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comma = ';'

	// Read and skip header
	_, err = reader.Read()
	if err != nil {
		log.Fatalf("Failed to read header: %v", err)
	}

	client := http.DefaultClient

	var tasks []task
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Error reading row: %v", err)
			continue
		}
		if len(row) < 2 {
			continue
		}

		bild := strings.TrimSpace(row[0])
		urlsStr := strings.TrimSpace(row[1])
		urlsStr = strings.Trim(urlsStr, `"`)
		urlLines := strings.Split(urlsStr, "\n")
		for _, u := range urlLines {
			url := strings.TrimSpace(u)
			if url != "" {
				tasks = append(tasks, task{bild: bild, url: url})
			}
		}
	}

	// Output header
	fmt.Println("id\turl\tmethod\tcontent_type\tcontent_length\tactual_size")

	urlChan := make(chan task)
	var wg sync.WaitGroup

	for i := 0; i < *parallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range urlChan {
				processURL(client, t, *method)
			}
		}()
	}

	go func() {
		for _, t := range tasks {
			urlChan <- t
		}
		close(urlChan)
	}()

	wg.Wait()
}

func processURL(client *http.Client, t task, method string) {
	req, err := http.NewRequest(method, t.url, nil)
	if err != nil {
		fmt.Printf("%s,%s,%s,,0,0 Error: %v\n", t.bild, t.url, method, err)
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("%s,%s,%s,,0,0 Error: %v\n", t.bild, t.url, method, err)
		return
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	contentLengthStr := resp.Header.Get("Content-Length")
	contentLength, _ := strconv.ParseInt(contentLengthStr, 10, 64)
	if contentLength < 0 {
		contentLength = 0
	}

	actualSize := int64(-1)
	if method == "GET" {
		n, err := io.Copy(io.Discard, resp.Body)
		if err != nil {
			fmt.Printf("%s,%s,%s,%s,%d,0 Error reading body: %v\n", t.bild, t.url, method, contentType, contentLength, err)
			return
		}
		actualSize = n
	}

	fmt.Printf("%s\t%s\t%s\t%s\t%d\t%d\n", t.bild, t.url, method, contentType, contentLength, actualSize)
}
