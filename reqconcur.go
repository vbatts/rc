/*
Make a bunch of requests against a domain.

This can handle providing client side x.509/rsa
*/
package main

import (
	"crypto/tls"
	"flag"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime"
  "time"
)

var (
	config_url, config_cert, config_key  string
	config_requests, config_workers      int64
	config_cpus                          int
	config_head_method, config_fail_quit bool
	config_quiet                         bool
	routines                             int                 = 15
	fail_now                             chan *http.Response = make(chan *http.Response)
	//codes                                map[int]uint        = make(map[int]uint)
	cert tls.Certificate
	err  error
)

func init() {
	flag.StringVar(&config_url, "url", "", "url to fetch (like https://www.mydomain.com/content/)")
	flag.StringVar(&config_cert, "cert", "", "x509 certificate file to use")
	flag.StringVar(&config_key, "key", "", "RSA key file to use")
	flag.Int64Var(&config_requests, "requests", 10, "Number of requests to perform")
	flag.Int64Var(&config_workers, "workers", 5, "Number of workers to use")
	flag.IntVar(&config_cpus, "cpus", runtime.NumCPU(), "Number of CPUs to use (defaults to all)")
	flag.BoolVar(&config_head_method, "head", false, "Whether to use HTTP HEAD (default is GET)")
	flag.BoolVar(&config_fail_quit, "fail", false, "Whether to exit on a non-OK response")
	flag.BoolVar(&config_quiet, "quiet", false, "do not print all responses")
}

type Requests struct {
	Method string
	Url    *url.URL
	Total  int64
}

func (r *Requests) Next() (*http.Request, error) {
	defer func() { r.Total-- }()
	return http.NewRequest(r.Method, r.Url.String(), nil)
}

func main() {
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		signal.Notify(c, os.Kill)
		for sig := range c {
			// sig is a ^C, handle it
			if sig == os.Interrupt {
				log.Println("interrupted ...")
				panic("showing stack")
			} else if sig == os.Kill {
				log.Println("killing ...")
				panic("showing stack")
			}
		}
	}()

	flag.Parse()
	runtime.GOMAXPROCS(config_cpus)

	t_config := tls.Config{
		InsecureSkipVerify: true,
	}

	if len(config_url) == 0 {
		log.Fatal("Please actually provide a -url to run against")
	}
	u, err := url.Parse(config_url)
	if err != nil {
		log.Fatal("ERROR: %s", err)
	}

	// load the cert if provided
	if len(config_cert) != 0 && len(config_key) != 0 {
		cert, err = tls.LoadX509KeyPair(config_cert, config_key)
		if err != nil {
			log.Fatal(err)
		}
		t_config.Certificates = append(t_config.Certificates, cert)
	}

	log.Printf("Please wait, calling against [%s] ...", config_url)
	worker_in_queue := make(chan *http.Request, config_workers)
	worker_out_queue := make(chan *http.Response, config_workers)

	// rev up these workers
	for i := int64(0); i < config_workers; i++ {
		go Worker(&http.Client{Transport: &http.Transport{TLSClientConfig: &t_config}},
			worker_in_queue,
			worker_out_queue, config_fail_quit)
	}

	reqs := &Requests{
		Url:   u,
		Total: config_requests,
	}
	if config_head_method {
		reqs.Method = "HEAD"
	} else {
		reqs.Method = "GET"
	}

  t_start := time.Now()
	// holy moly. this thing was blocking!
	go func() {
		for reqs.Total != 0 {
			r, err := reqs.Next()
			if err != nil {
				log.Printf("ERROR: setting up request %s", err)
				if config_fail_quit {
					os.Exit(2)
				}
			}
			worker_in_queue <- r
		}
	}()

	go func() {
		req := <-fail_now
		log.Printf("made %d requests before failure", (reqs.Total - config_requests))
		log.Printf("ERROR: %#v", req)
		os.Exit(2)
	}()

	count := int64(0)
	for resp := range worker_out_queue {
		if !config_quiet {
			log.Printf("%#v", resp)
		}
		count++
		if count == config_requests {
			break
		}
	}

	log.Printf("completed %d requests in %s", count, time.Since(t_start))

}

func Worker(client *http.Client,
	in chan *http.Request,
	out chan *http.Response,
	fail_early bool) {
	for {
		select {
		case req := <-in:
			resp, err := client.Do(req)
			if err != nil {
				log.Printf("ERROR: setting up request %s", err)
				if fail_early {
					return
				}
			}
			if fail_early && resp.StatusCode != 200 {
				fail_now <- resp
				return
			}
			resp.Body.Close()
			out <- resp
		}
	}
}
