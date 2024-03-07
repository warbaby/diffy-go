package main

import (
	"flag"
	"fmt"
	"github.com/nsf/jsondiff"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sync"
)

var (
	primary   string
	candidate string
	numMatch  int
	numDiff   int
	numTotal  int
)

func env(key string, defaultValue string) string {
	if v := os.Getenv(key); v == "" {
		return defaultValue
	} else {
		return v
	}
}

func main() {
	var help bool
	flag.BoolVar(&help, "h", false, "show these messages")
	flag.StringVar(&primary, "primary", env("diffy.primary", ""), "primary server for old logic")
	flag.StringVar(&candidate, "candidate", env("diffy.candidate", ""), "candidate server for developing logic")
	var port string
	flag.StringVar(&port, "port", env("diffy.port", "8080"), "diffy-go listen port")

	flag.Usage = func() {
		fmt.Println("diffy-go 1.0")
		fmt.Println("Usage: diffy-go -primary http://abc.com -candidate http://candidate:80 -port 8080")
		fmt.Println("Parameters can also be defined in env diffy.primary, diffy.candidate, diffy.port")
		flag.PrintDefaults()
	}

	flag.Parse()

	if help || primary == "" || candidate == "" {
		flag.Usage()
		os.Exit(0)
	}

	_ = os.Mkdir("/logs", 0666)
	file, err := os.OpenFile("/logs/diffy-go.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal("Failed to open log file:", err)
	}
	defer file.Close()
	log.SetOutput(file)

	fmt.Println("server started, use gor to forward requests...")

	http.HandleFunc("/result", showResult)
	http.HandleFunc("/", handleRequest)
	_ = http.ListenAndServe(":"+port, nil)

}

func showResult(w http.ResponseWriter, r *http.Request) {
	// Write result to client
	w.Header().Set("Content-Type", "text/plain")
	_, _ = fmt.Fprintf(w, "numTotal: %d\nnumDiff: %d\nnumMatch: %d\nnumIgnore: %d\n", numTotal, numDiff, numMatch, numTotal-numDiff-numMatch)
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	if r.RequestURI == "/" || r.RequestURI == "" {
		showResult(w, r)
		return
	}

	// Perform requests to backend servers concurrently
	var wg sync.WaitGroup
	var respA, respB []byte
	var errA, errB error

	wg.Add(2)
	go func() {
		defer wg.Done()
		respA, errA = doRequest(primary, r)
	}()
	go func() {
		defer wg.Done()
		respB, errB = doRequest(candidate, r)
	}()
	wg.Wait()

	numTotal++

	// Compare responses
	if errA != nil {
		if errB != nil {
			log.Println("[result]", "[ignore]", "ALL FAIL", r.URL.Path, r.URL.RawQuery)
		} else {
			log.Println("[result]", "[ignore]", "PRIMARY FAIL", r.URL.Path, r.URL.RawQuery)
		}
		return
	}
	if errB != nil {
		log.Println("[result]", "[diff]", "CANDIDATE FAIL", errB.Error(), r.URL.Path, r.URL.RawQuery)
		numDiff++
		return
	}

	options := &jsondiff.Options{
		Added:            jsondiff.Tag{Begin: "++ "}, //jsondiff.Tag{Begin: "<++>", End: "</++>"},
		Removed:          jsondiff.Tag{Begin: "-- "}, //jsondiff.Tag{Begin: "<-->", End: "</-->"},
		Changed:          jsondiff.Tag{Begin: "!! "}, //jsondiff.Tag{Begin: "<!!>", End: "</!!>"},
		ChangedSeparator: ", ",
		Indent:           "    ",
		SkipMatches:      true,
	}

	match, diffstr := jsondiff.Compare(respA, respB, options)

	if match == jsondiff.FullMatch {
		log.Println("[result]", "[match]", r.URL.Path, r.URL.RawQuery)
		numMatch++
	} else {
		log.Println("[result]", "[diff]", "MISMATCH", r.URL.Path, r.URL.RawQuery)
		log.Println("[detail]", diffstr)
		if match == jsondiff.BothArgsAreInvalidJson || match == jsondiff.FirstArgIsInvalidJson {
			log.Println("[detail]", "[primary]", string(respA))
		}
		if match == jsondiff.BothArgsAreInvalidJson || match == jsondiff.SecondArgIsInvalidJson {
			log.Println("[detail]", "[candidate]", string(respB))
		}
		numDiff++
	}

	showResult(w, r)
}

func doRequest(backend string, r *http.Request) ([]byte, error) {
	// Create a new request to the backend
	backendURL, _ := url.Parse(backend)
	proxyURL := fmt.Sprintf("%s://%s%s", backendURL.Scheme, backendURL.Host, r.RequestURI)
	req, err := http.NewRequest(r.Method, proxyURL, r.Body)
	if err != nil {
		return nil, err
	}
	req.Header = r.Header

	// Perform the request to the backend
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status: %d", resp.StatusCode)
	}

	// Read the response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return respBody, nil
}
