package util

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

var client = http.Client{
	Timeout: time.Second * 10,
}

func Request(method, url string) string {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		fmt.Println(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
	}
	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
	}
	return string(b)
}
