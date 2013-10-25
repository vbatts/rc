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
)

var (
	config_url, config_cert, config_key  string
	config_requests, config_workers      int
	config_head_method, config_fail_quit bool
	config_quiet                         bool
	routines                             int = 15
	results                              chan stat
	quit_now                             chan *http.Response = make(chan *http.Response)
	codes                                map[int]uint        = make(map[int]uint)
	count                                int
	cert                                 tls.Certificate
	err                                  error
)

func init() {
	flag.StringVar(&config_url, "url", "", "url to fetch (like https://www.mydomain.com/content/)")
	flag.StringVar(&config_cert, "cert", "", "x509 certificate file to use")
	flag.StringVar(&config_key, "key", "", "RSA key file to use")
	flag.IntVar(&config_requests, "requests", 10, "Number of requests to perform")
	flag.IntVar(&config_workers, "workers", 5, "Number of workers to use")
	flag.BoolVar(&config_head_method, "head", false, "Whether to use HTTP HEAD (default is GET)")
	flag.BoolVar(&config_fail_quit, "fail", false, "Whether to exit on a non-OK response")
	flag.BoolVar(&config_quiet, "quiet", false, "do not print all responses")
}

func main() {
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

	if config_workers > routines {
		routines = config_workers
	}

  log.Printf("Please wait, calling against [%s] ...", config_url)
	results = make(chan stat, routines)
	for i := 0; i < config_workers; i++ {
		go func() {
			for j := 0; j < config_requests; j++ {
				var (
					req *http.Request
					err error
				)
				if config_head_method {
					req, err = http.NewRequest("HEAD", config_url, nil)
				} else {
					req, err = http.NewRequest("GET", config_url, nil)
				}
				if err != nil {
					log.Printf("ERROR: setting up request %s", err)
					return
				}
				resp, err := client.Do(req)
				if err != nil {
					log.Printf("ERROR: %s", err)
				} else {
					codes[resp.StatusCode]++
          // just don't process this response if we need to be quieter
					if !config_quiet {
						go func() {
							results <- respStat(resp)
						}()
					}
				}
				if resp.StatusCode != 200 {
					quit_now <- resp
				}
				count++
			}
		}()
	}
	for {
		select {
		case r := <-results:
			log.Println(r)
		case req := <-quit_now:
			log.Printf("made %d requests before failure", count)
			log.Printf("ERROR: %#v", req)
			os.Exit(2)
		}
		if count == (config_requests * config_workers) {
			break
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
