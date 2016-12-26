package httpd

import (
	"sync"
)

type Handler struct {
	mu sync.RWMutex
}

func (h *Handler) SetHTTPClient() {

}

func (h *Handler) StartAnnouncement() {

}
