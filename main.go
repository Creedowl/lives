package main

import (
	"live/util"
)

func main() {
	logger := util.NewLogger()
	defer logger.Sync()
	r := NewServer()
	err := r.Run()
	if err != nil {
		logger.Error(err)
		return
	}
}
