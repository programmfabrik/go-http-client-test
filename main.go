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
	resultFile := flag.String("result-file", "result.csv", "Path to the result CSV file")
	method := flag.String("method", "HEAD", "HTTP method: GET or HEAD")
	parallel := flag.Int("parallel", 1, "Number of parallel requests")
	flag.Parse()

	if *csvFile == "" {
		log.Fatal("Missing --csv-file")
	}
	if *resultFile == "" {
		log.Fatal("Missing --result-file")
	}
	if *method != "GET" && *method != "HEAD" {
		log.Fatal("Invalid --method; must be GET or HEAD")
	}
	if *parallel <= 0 {
		log.Fatal("--parallel must be positive")
	}

	srcFile, err := os.Open(*csvFile)
	if err != nil {
		log.Fatalf("Failed to open CSV: %v", err)
	}
	defer srcFile.Close()

	targetFile, err := os.Create(*resultFile)
	if err != nil {
		log.Fatalf("Failed to create target CSV: %v", err)
	}
	defer targetFile.Close()

	reader := csv.NewReader(srcFile)
	reader.Comma = ';'

	writer := csv.NewWriter(targetFile)
	writer.Comma = ';'
	defer writer.Flush()

	// Read header, remove BOM if if exists
	header, err := reader.Read()
	if err != nil {
		log.Fatalf("Failed to read header: %v", err)
	}
	header[0], _ = strings.CutPrefix(header[0], "\ufeff")

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
		if len(row) != len(header) {
			log.Printf("Error reading row: row length (%d) != expected header length (%d)", len(row), len(header))
			continue
		}

		line := map[string]string{}
		for i := range row {
			line[header[i]] = row[i]
		}

		bild := strings.TrimSpace(line["bildnummer"])
		urlsStr := strings.TrimSpace(line["weitere_dateien"])
		urlsStr = strings.Trim(urlsStr, `"`)
		urlLines := strings.SplitSeq(urlsStr, "\n")
		for u := range urlLines {
			url := strings.TrimSpace(u)
			if url != "" {
				tasks = append(tasks, task{bild: bild, url: url})
			}
		}
	}

	// Output header
	resultHeader := []string{"id", "url", "method", "content_type", "content_length", "actual_size", "error"}
	fmt.Println(strings.Join(resultHeader, "\t"))
	writer.Write(resultHeader)

	resultLines := [][]string{}

	urlChan := make(chan task)
	var wg sync.WaitGroup

	for i := 0; i < *parallel; i++ {
		wg.Go(func() {
			for t := range urlChan {
				processURL(client, &resultLines, t, "HEAD")
				processURL(client, &resultLines, t, "GET")
			}
		})
	}

	go func() {
		for _, t := range tasks {
			urlChan <- t
		}
		close(urlChan)
	}()

	wg.Wait()

	for _, line := range resultLines {
		writer.Write(line)
	}
}

func processURL(client *http.Client, resultLines *[][]string, t task, method string) {
	resultRow := []string{
		t.bild,
		t.url,
		method,
		"", // Content-Type
		"", // Content-Length
		"", // actual size
		"", // error (if any)
	}

	req, err := http.NewRequest(method, t.url, nil)
	//req.Close = true
	if err != nil {
		resultRow[len(resultRow)-1] = err.Error()
		fmt.Println(strings.Join(resultRow, "\t"))
		*resultLines = append(*resultLines, resultRow)
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		resultRow[len(resultRow)-1] = err.Error()
		fmt.Println(strings.Join(resultRow, "\t"))
		*resultLines = append(*resultLines, resultRow)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		resultRow[len(resultRow)-1] = fmt.Sprintf("statuscode=%d", resp.StatusCode)
		fmt.Println(strings.Join(resultRow, "\t"))
		*resultLines = append(*resultLines, resultRow)
		return
	}

	contentType := resp.Header.Get("Content-Type")
	contentLengthStr := resp.Header.Get("Content-Length")
	contentLength, _ := strconv.ParseInt(contentLengthStr, 10, 64)
	if contentLength < 0 {
		contentLength = 0
	}

	resultRow[len(resultRow)-4] = contentType
	resultRow[len(resultRow)-3] = fmt.Sprintf("%d", contentLength)

	actualSize := int64(-1)
	if method == "GET" {
		n, err := io.Copy(io.Discard, resp.Body)
		if err != nil {
			resultRow[len(resultRow)-1] = err.Error()
			fmt.Println(strings.Join(resultRow, "\t"))
			*resultLines = append(*resultLines, resultRow)
			return
		}
		actualSize = n
	}

	resultRow[len(resultRow)-2] = fmt.Sprintf("%d", actualSize)
	fmt.Println(strings.Join(resultRow, "\t"))
	*resultLines = append(*resultLines, resultRow)
}
