package util

import (
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

var client = http.Client{
	Timeout: time.Second * 10,
}

func Request(method, url, params string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequest(method, url, strings.NewReader(params))
	if err != nil {
		return nil, err
	}
	if headers != nil {
		for k, v := range headers {
			req.Header.Add(k, v)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return b, nil
}
