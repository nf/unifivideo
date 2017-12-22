package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"time"
)

func main() {
	var (
		interval = flag.Duration("interval", 5*time.Second, "snapshot interval")
		user     = flag.String("user", "", "UniFi camera `user name`")
		pass     = flag.String("pass", "", "UniFi camera `password`")
		basePath = flag.String("path", "", "base `path` in which to store snapshots")
		baseURL  = flag.String("url", "", "UniFi camera admin interface `url`")
	)
	flag.Parse()
	required("user", *user)
	required("pass", *pass)
	required("path", *basePath)
	required("url", *baseURL)

	jar, _ := cookiejar.New(nil)
	poller := &poller{
		baseURL:  *baseURL,
		basePath: *basePath,
		user:     *user,
		pass:     *pass,
		interval: *interval,
		client: &http.Client{
			Jar: jar,
		},
	}
	poller.poll()
}

func required(name, value string) {
	if value == "" {
		fmt.Fprintf(os.Stderr, "flag -%s must be provided\n", name)
		flag.Usage()
		os.Exit(2)
	}
}

type poller struct {
	baseURL, basePath, user, pass string
	interval                      time.Duration
	client                        *http.Client
}

func (p *poller) poll() {
	t := time.NewTicker(p.interval)
	for now := range t.C {
		b, err := p.snap()
		if err != nil {
			log.Println("snap:", err)
			continue
		}
		go p.write(now, b)
	}
}

func (p *poller) write(t time.Time, b []byte) {
	s := t.Format("2006/01/02/15/2006-01-02-15-04-05.jpg")
	file := filepath.Join(p.basePath, filepath.FromSlash(s))

	err := os.MkdirAll(filepath.Dir(file), 0700)
	if err != nil {
		log.Println("write:", err)
	}
	err = ioutil.WriteFile(file, b, 0600)
	if err != nil {
		log.Println("write:", err)
	}
}

func (p *poller) snap() ([]byte, error) {
	for attempt := 0; attempt < 2; attempt++ {
		u := fmt.Sprintf("%ssnap.jpeg?cb=%d", p.baseURL, time.Now().Unix())
		resp, err := p.client.Get(u)
		if err != nil {
			return nil, err
		}
		b, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized {
			if err := p.login(); err != nil {
				return nil, err
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, errors.New(resp.Status)
		}
		return b, err
	}
	return nil, errors.New("authentication failure")
}

func (p *poller) login() error {
	b, err := json.Marshal(map[string]string{
		"username": p.user,
		"password": p.pass,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", p.baseURL+"api/1.0/login", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New(resp.Status)
	}
	return nil
}
