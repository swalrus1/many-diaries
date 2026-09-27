package medium

import "net/http"

type Medium interface {
	Name() string
	Handler() http.Handler
}
