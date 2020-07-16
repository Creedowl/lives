package util

import "go.uber.org/zap"

var logger *zap.SugaredLogger

func NewLogger() *zap.SugaredLogger {
	if logger == nil {
		_logger, _ := zap.NewDevelopment()
		logger = _logger.Sugar()
	}
	return logger
}
