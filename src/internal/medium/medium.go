package medium

import (
	"html/template"
	"net/http"
)

type Medium interface {
	Name() string
	ConfigPanel(r *http.Request, message string) (template.HTML, error)
	HandleAction(r *http.Request) (string, error)
}
