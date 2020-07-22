package main

import (
	"live/util"
)

var logger = util.GetLogger()

func main() {
	defer logger.Sync()
	r := NewServer()
	err := r.Run()
	if err != nil {
		logger.Error(err)
		return
	}
}
