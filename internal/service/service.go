package service

import (
	httpPort "github.com/contentforward/bolt-ui/ports/http"
)

type Service struct {
	HTTPServer *httpPort.Server
}

func NewService(httpServer *httpPort.Server) *Service {
	return &Service{
		HTTPServer: httpServer,
	}
}
