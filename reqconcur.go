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
	"os"
  "os/signal"
)

var (
	config_url, config_cert, config_key  string
	config_requests, config_workers      int64
	config_head_method, config_fail_quit bool
	config_quiet                         bool
	routines                             int = 15
	results                              chan stat
	fail_now                             chan *http.Response = make(chan *http.Response)
	codes                                map[int]uint        = make(map[int]uint)
	cert                                 tls.Certificate
	err                                  error
)

func init() {
	flag.StringVar(&config_url, "url", "", "url to fetch (like https://www.mydomain.com/content/)")
	flag.StringVar(&config_cert, "cert", "", "x509 certificate file to use")
	flag.StringVar(&config_key, "key", "", "RSA key file to use")
	flag.Int64Var(&config_requests, "requests", 10, "Number of requests to perform")
	flag.Int64Var(&config_workers, "workers", 5, "Number of workers to use")
	flag.BoolVar(&config_head_method, "head", false, "Whether to use HTTP HEAD (default is GET)")
	flag.BoolVar(&config_fail_quit, "fail", false, "Whether to exit on a non-OK response")
	flag.BoolVar(&config_quiet, "quiet", false, "do not print all responses")
}

type Requests struct {
	Method string
	Url    string
	Total  int64
}

func (r *Requests) Next() (*http.Request, error) {
	defer func() { r.Total = r.Total - 1 }()
	return http.NewRequest(r.Method, r.Url, nil)
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

	t_config := tls.Config{
		InsecureSkipVerify: true,
	}

	if len(config_url) == 0 {
		log.Fatal("Please actually provide a -url to run against")
	}

	// load the cert if provided
	if len(config_cert) != 0 && len(config_key) != 0 {
		cert, err = tls.LoadX509KeyPair(config_cert, config_key)
		if err != nil {
			log.Fatal(err)
		}
		t_config.Certificates = append(t_config.Certificates, cert)
	}

	tr := &http.Transport{
		TLSClientConfig: &t_config,
	}
	client := &http.Client{
		Transport: tr,
	}

	log.Printf("Please wait, calling against [%s] ...", config_url)
	results = make(chan stat, config_workers)
	workers := make(chan *http.Request, config_workers)

	// rev up these workers
	for i := int64(0); i < config_workers; i++ {
		go func() {
			// this goroutine needs a channel to receive and cleanup
			for {
				resp, err := client.Do(<-workers)
				if err != nil {
					log.Printf("ERROR: setting up request %s", err)
					if config_fail_quit {
						os.Exit(2)
					}
				}
				go func() {
					// this goroutine will spin off and get harvested
					codes[resp.StatusCode]++
					results <- respStat(resp)
					if config_fail_quit && resp.StatusCode != 200 {
						fail_now <- resp
					}
				}()
        resp.Body.Close()
			}
		}()
	}

	reqs := &Requests{
		Url:   config_url,
		Total: config_requests,
	}
	if config_head_method {
		reqs.Method = "HEAD"
	} else {
		reqs.Method = "GET"
	}

	for reqs.Total != 0 {
		r, err := reqs.Next()
		if err != nil {
			log.Printf("ERROR: setting up request %s", err)
			if config_fail_quit {
				os.Exit(2)
			}
		}
		workers <- r
	}

	for {
		select {
		case r := <-results:
			// just don't process this response if we need to be quieter
			if !config_quiet {
				log.Println(r)
			}
		case req := <-fail_now:
			log.Printf("made %d requests before failure", (reqs.Total - config_requests))
			log.Printf("ERROR: %#v", req)
			os.Exit(2)
		}
	}
	log.Println("HTTP Codes:")
	for k, v := range codes {
		log.Printf("  %d: %d", k, v)
	}
}

type stat struct {
	size int64
	code int
	pass bool
}

func respStat(resp *http.Response) (r_stat stat) {
	r_stat.size = resp.ContentLength
	r_stat.code = resp.StatusCode
	if r_stat.code > 499 && r_stat.size > 340 {
		log.Println(resp)
	}
	if r_stat.code == 200 {
		r_stat.pass = true
	} else {
		r_stat.pass = false
	}
	return
}
