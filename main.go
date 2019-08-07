package main

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"github.com/jawher/mow.cli"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	Clear  = "\033[0m"
	Green  = "\033[1;32m"
	Red    = "\033[1;31m"
	Yellow = "\033[1;33m"
	Blue   = "\033[1;34m"
)

var (
	OutputData = Data{
		Errors: make(map[string]*ErrorData),
	}
	colorMutex = sync.Mutex{}
	errorMutex = sync.Mutex{}
)

type Options struct {
	Command           string
	Data              string
	Duration          int
	Headers           []string
	IgnoreCertificate bool
	RequestCount      int
	Delay             float64
	Threads           int
	Timeout           int
	URL               string
}

type Data struct {
	End               time.Time
	ErrorCount        int64
	Errors            map[string]*ErrorData
	Interrupt         bool
	OutColor          string
	RequestCount      int64
	Start             time.Time
}

type ErrorData struct {
	Count int64
	Color string
}

func setColor(color string) {
	colorMutex.Lock()
	defer colorMutex.Unlock()
	fmt.Printf(color)
	OutputData.OutColor = color
}

func addError(error string, color string) {
	go func() {
		errorMutex.Lock()
		defer errorMutex.Unlock()
		if data, found := OutputData.Errors[error]; found {
			data.Count++
		} else {
			data = &ErrorData{
				Color: color,
				Count: 1,
			}
			OutputData.Errors[error] = data
		}
	}()
}

func sendRequest(client *http.Client, options *Options) error {
	var request *http.Request
	var err     error
	var buffer  *bytes.Buffer
	dataBytes := []byte(options.Data)
	buffer = bytes.NewBuffer(dataBytes)
	request, err = http.NewRequest(options.Command, options.URL, buffer)
	if err != nil {
		fmt.Printf("Error creating the request: %s\n", err.Error())
		return err
	}
	defer request.Body.Close()
	for _, h := range options.Headers {
		header := strings.SplitN(h, ":", 2)
		request.Header.Set(strings.TrimSpace(header[0]), strings.TrimSpace(header[1]))
	}
	resp, err := client.Do(request)
	if err != nil {
		atomic.AddInt64(&OutputData.ErrorCount, 1)
		setColor(Blue)
		addError(err.Error(), Blue)
		fmt.Print(".")
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		setColor(Green)
		fmt.Print(".")
		addError("200", Green)
	} else {
		atomic.AddInt64(&OutputData.ErrorCount, 1)
		setColor(Red)
		fmt.Print(".")
		addError(strconv.Itoa(resp.StatusCode), Red)
	}
	return nil
}

func performTest(options *Options) {
	tr := &http.Transport{}
	if strings.HasPrefix(options.URL, "https") {
		if options.IgnoreCertificate {
			tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		}
	}
	client := &http.Client{
		Transport: tr,
		Timeout: time.Second * time.Duration(options.Timeout),
	}
	for i := int64(0); OutputData.Interrupt == false && (options.RequestCount == 0 || OutputData.RequestCount < int64(options.RequestCount)); i++ {
		atomic.AddInt64(&OutputData.RequestCount, 1)
		if err := sendRequest(client, options); err != nil {
			return
		}
		if options.Duration > 0 && OutputData.Start.Add(time.Second * time.Duration(options.Duration)).Before(time.Now()) {
			OutputData.Interrupt = true
		}
		time.Sleep(time.Millisecond * time.Duration(options.Delay * 1000))
	}
	return
}

func countString(base string, count int64) string {
	base = fmt.Sprintf("%d %s", count, base)
	if count == 1 {
		return base
	}
	if strings.HasSuffix(base, "s") {
		base += "e"
	}
	return base + "s"
}

func loadtest(options *Options) {
	requests := "unlimited requests"
	if options.RequestCount > 0 {
		requests = countString("total request", int64(options.RequestCount))
	}
	duration := ""
	if options.Duration > 0 {
		duration = fmt.Sprintf(", maximum duration of %s", countString("second", int64(options.Duration)))
	}
	fmt.Printf("Running %s against: %s (%s)%s\n", countString("thread", int64(options.Threads)), options.URL, requests, duration)

	if strings.HasPrefix(options.URL, "https") {
		if options.IgnoreCertificate {
			fmt.Printf("Skipping certificate verification.\n")
		}
	}
	wg := sync.WaitGroup{}
	wg.Add(options.Threads)
	for i := 0; i < options.Threads; i++ {
		go func(thread int) {
			performTest(options)
			defer wg.Done()
		}(i)
	}
	wg.Wait()
}

// Resolves a path; attempts to resolve ~ and ~<user>
func resolvePath(datafile string) (string, error) {
	var file string
	if strings.Contains(datafile, "://") {
		f, err := url.Parse(datafile)
		if err != nil {
			return "", err
		}
		file = f.Path
	} else {
		if start := strings.Index(datafile, "~"); start >= 0 {
			var err error
			username := ""
			end := start + 1
			for end < len(datafile) && datafile[end] != '/' {
				username += string(datafile[end])
				end++
			}
			var userAccount *user.User
			if username == "" {
				userAccount, err = user.Current()
			} else {
				userAccount, err = user.Lookup(username)
			}
			if err != nil {
				return "", err
			}
			homeDir := userAccount.HomeDir
			datafile = strings.Replace(datafile, "~" + username, homeDir, -1)
			file, err = filepath.Abs(datafile)
			if err != nil {
				return "", err
			}
		} else {
			file = datafile
		}
	}
	return file, nil
}

func sortedKeys(errorMap map[string]*ErrorData) []string {
	keys := make([]string, len(errorMap))
	i := 0
	for key := range errorMap {
		keys[i] = key
		i++
	}
	sort.Strings(keys)
	return keys
}

// Returns a CLI boolean option. There's a bug in the output that displays "default: true" for any bool. This
// just hides the default value.  The boolean option behaves as expected; this is just a help display issue.
func boolOpt(app *cli.Cli, name string, value bool, desc string) *bool {
	return app.Bool(cli.BoolOpt{
		Name:      name,
		Value:     value,
		Desc:      desc,
		HideValue: true,
	})
}

func main() {
	app := cli.App("loadtest", "Http(s) load test utility")

	app.Spec = "URL [ -h ] [ -k ] [ -X=<command> ] [ -c=<count> ] [ --delay=<delay> ] [ --connect-timeout=<timeout> ] [ -t=<threads> ] ([ -d=<data> ] | [ --data-file=<data-file> ]) [ -H=<key:value> ]... [ --duration=<duration> ]"

	timeout := app.IntOpt("connect-timeout",3,"Maximum time allowed for connection in seconds")
	count := app.IntOpt("c count",0,"The total number of requests to make. If count < 1 there is no limit")
	data := app.StringOpt("d data", "", "HTTP POST data")
	datafile := app.StringOpt("data-file", "", "HTTP POST data from file")
	delayString := app.String(cli.StringOpt{
		Name:      "delay",
		Value:     "0",
		Desc:      "Wait time between requests in seconds, per thread (default 0)",
		HideValue: true,
	})
	duration := app.IntOpt("duration", 0, "The maximum duration for the test in seconds")
	headers := app.StringsOpt("H header", []string{}, "Pass custom header(s) to server")
	help := boolOpt(app,"h help", false, "Display this help")
	ignoreCert := boolOpt(app,"k insecure", false, "Allow insecure server connections when using SSL")
	threads := app.IntOpt("t threads",1,"How many concurrent threads to use in the load test")
	URL := app.StringArg("URL", "", "URL to work with")
	app.Version("v version", "loadtest 0.0.1")
	command := app.StringOpt("X command", "GET", "Specify request command to use")

	app.Action = func() {
		if *help {
			app.PrintHelp()
			os.Exit(0)
		}
		if *count < 1 {
			*count = 0
		}
		if *threads < 1 {
			fmt.Printf("Invalid thread value (%d). Must be a positive integer.", *count)
			app.PrintHelp()
			os.Exit(1)
		}
		if *timeout < 1 {
			fmt.Printf("Invalid timeout value (%d). Must be a positive integer.", *timeout)
			app.PrintHelp()
			os.Exit(1)
		}
		delay, err := strconv.ParseFloat(*delayString, 32)
		if err != nil {
			fmt.Printf("Invalid delay value (%s). Must be a positive float.", *delayString)
			app.PrintHelp()
			os.Exit(1)
		}
		if delay < 0 {
			delay = 0
		}

		for _, h := range *headers {
			data := strings.SplitN(h, ":", 2)
			if len(data) != 2 {
				fmt.Printf("Invalid header format (%s). Expected: key:value\n", h)
				os.Exit(1)
			}
		}

		if *datafile != "" {
			file, err := resolvePath(*datafile)
			if err != nil {
				fmt.Println(err.Error())
				os.Exit(1)
			}
			b, err := ioutil.ReadFile(file)
			if err != nil {
				fmt.Println(err.Error())
				os.Exit(1)
			}
			*data = string(b)
			fmt.Printf("Read data from file: %s\n", *data)
		}

		OutputData.Start = time.Now()
		options := &Options{
			Command:           strings.ToUpper(strings.TrimSpace(*command)),
			Data:              *data,
			Duration:          *duration,
			Headers:           *headers,
			IgnoreCertificate: *ignoreCert,
			RequestCount:      *count,
			Delay:             delay,
			Threads:           *threads,
			Timeout:           *timeout,
			URL:               strings.TrimSpace(*URL),
		}

		loadtest(options)
	}

	// trap ^C to stop our threads
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		select {
		case <-c:
			OutputData.Interrupt = true
		}
	}()

	// Start the app.
	app.Run(os.Args)
	d := time.Now().Sub(OutputData.Start)

	// Show some stats when we're done.
	setColor(Clear)
	fmt.Printf("\n%s of %s (%d%% error rate) made in %f seconds.\n",
		countString("error", OutputData.ErrorCount),
		countString("request", int64(OutputData.RequestCount)),
		OutputData.ErrorCount * 100.0 / OutputData.RequestCount, d.Seconds())
	fmt.Printf("%.2f requests/second.\n", float64(OutputData.RequestCount) / d.Seconds())
	fmt.Printf("Responses:\n")
	for _, key := range sortedKeys(OutputData.Errors) {
		value := OutputData.Errors[key]
		fmt.Printf("  %s: %s%d%s %.2f per second\n", key, value.Color, value.Count, Clear, float64(value.Count) / d.Seconds())
	}
}
