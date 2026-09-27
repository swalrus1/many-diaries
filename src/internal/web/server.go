package web

import (
	"embed"
	"encoding/json"
	"html/template"
	"io/fs"
	"net/http"
	"time"

	"github.com/swalrus1/many-diaries/internal/index"
	"github.com/swalrus1/many-diaries/internal/medium"
)

//go:embed templates static
var content embed.FS

type Server struct {
	idx     index.MetadataIndex
	mediums []medium.Medium
	tmpl    *template.Template
}

func New(idx index.MetadataIndex) (*Server, error) {
	tmpl, err := template.ParseFS(content, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &Server{idx: idx, tmpl: tmpl}, nil
}

func (s *Server) AddMedium(m medium.Medium) {
	s.mediums = append(s.mediums, m)
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.home)
	mux.HandleFunc("GET /stats", s.stats)
	mux.HandleFunc("GET /api/stats", s.apiStats)
	static, _ := fs.Sub(content, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	for _, m := range s.mediums {
		mux.Handle("/medium/"+m.Name()+"/", http.StripPrefix("/medium/"+m.Name(), m.Handler()))
	}
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

// apiStats returns record counts keyed by unix timestamp of the day (local time).
func (s *Server) apiStats(w http.ResponseWriter, r *http.Request) {
	recs, err := s.idx.ListRecords(r.Context())
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

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	s.RenderPage(w, "Stats", statsBody)
}

const statsBody = `<h2>Activity</h2>
<div id="heatmap"></div>
<script src="/static/frappe-charts.min.umd.js"></script>
<script>
fetch('/api/stats').then(r => r.json()).then(dataPoints => {
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
</script>`
