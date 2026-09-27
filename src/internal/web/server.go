package web

import (
	"embed"
	"encoding/json"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"time"

	"github.com/swalrus1/many-diaries/internal/index"
	"github.com/swalrus1/many-diaries/internal/medium"
)

//go:embed templates static
var content embed.FS

type Server struct {
	idx     index.MetadataIndex
	mediums map[string]medium.Medium
	tmpl    *template.Template
}

func New(idx index.MetadataIndex) (*Server, error) {
	tmpl, err := template.ParseFS(content, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &Server{idx: idx, tmpl: tmpl, mediums: map[string]medium.Medium{}}, nil
}

func (s *Server) AddMedium(m medium.Medium) {
	s.mediums[m.Name()] = m
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.home)
	mux.HandleFunc("GET /stats", s.stats)
	mux.HandleFunc("GET /api/stats", s.apiStats)
	mux.HandleFunc("GET /medium/{name}/", s.mediumPage)
	mux.HandleFunc("POST /medium/{name}/", s.mediumPage)
	static, _ := fs.Sub(content, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	return mux
}

func (s *Server) RenderPage(w http.ResponseWriter, title string, body template.HTML) {
	err := s.tmpl.ExecuteTemplate(w, "layout.html", struct {
		Title string
		Body  template.HTML
	}{title, body})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	var body template.HTML
	body = "<h2>Mediums</h2><ul>"
	for _, m := range s.mediums {
		body += `<li><a href="/medium/` + template.HTML(m.Name()) + `/">` + template.HTML(m.Name()) + `</a></li>`
	}
	body += "</ul>"
	s.RenderPage(w, "Home", body)
}

func (s *Server) mediumPage(w http.ResponseWriter, r *http.Request) {
	m, ok := s.mediums[r.PathValue("name")]
	if !ok {
		http.NotFound(w, r)
		return
	}
	message := ""
	if r.Method == http.MethodPost {
		msg, err := m.HandleAction(r)
		if err != nil {
			message = "Error: " + err.Error()
		} else {
			message = msg
		}
	}
	frag, err := m.ConfigPanel(r, message)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.RenderPage(w, m.Name(), frag+activityPanel(m.Name()))
}

// activityPanel is the single shared heatmap snippet; medium "" means all mediums.
func activityPanel(mediumName string) template.HTML {
	src := "/api/stats"
	if mediumName != "" {
		src += "?medium=" + url.QueryEscape(mediumName)
	}
	return template.HTML(`<h3>Activity</h3>
<div id="heatmap"></div>
<script src="/static/frappe-charts.min.umd.js"></script>
<script>
fetch('` + src + `').then(r => r.json()).then(dataPoints => {
	const start = new Date();
	start.setFullYear(start.getFullYear() - 1);
	new frappe.Chart('#heatmap', {
		type: 'heatmap',
		data: { dataPoints, start },
		countLabel: 'records',
		discreteDomains: 0,
		colors: ['#ebedf0', '#9be9a8', '#40c463', '#30a14e', '#216e39'],
	});
});
</script>`)
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	s.RenderPage(w, "Stats", "<h2>Stats</h2>"+activityPanel(""))
}

// apiStats returns record counts keyed by unix timestamp of the day (local time).
func (s *Server) apiStats(w http.ResponseWriter, r *http.Request) {
	recs, err := s.idx.ListRecords(r.Context(), r.URL.Query().Get("medium"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	counts := map[int64]int{}
	for _, rec := range recs {
		y, m, d := rec.CreatedAt.Local().Date()
		counts[time.Date(y, m, d, 0, 0, 0, 0, time.Local).Unix()]++
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(counts)
}
