package landing

import (
	"embed"
	"html/template"
	"net/http"

	"github.com/mpepping/discovery-service/internal/state"
	"go.uber.org/zap"
)

//go:embed html/*
var htmlFiles embed.FS

//go:embed templates/*
var templateFiles embed.FS

// Handler provides HTTP handlers for the landing page
type Handler struct {
	state    *state.State
	logger   *zap.Logger
	template *template.Template
}

// NewHandler creates a new landing page handler
func NewHandler(st *state.State, logger *zap.Logger) (*Handler, error) {
	tmpl, err := template.ParseFS(templateFiles, "templates/*.tmpl")
	if err != nil {
		return nil, err
	}

	return &Handler{
		state:    st,
		logger:   logger,
		template: tmpl,
	}, nil
}

// ServeHTTP serves the landing page
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		h.serveIndex(w, r)
		return
	}

	if r.URL.Path == "/inspect" {
		h.serveInspect(w, r)
		return
	}

	http.NotFound(w, r)
}

func (h *Handler) serveIndex(w http.ResponseWriter, r *http.Request) {
	data, err := htmlFiles.ReadFile("html/index.html")
	if err != nil {
		h.logger.Error("failed to read index.html", zap.Error(err))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

func (h *Handler) serveInspect(w http.ResponseWriter, r *http.Request) {
	clusterID := r.URL.Query().Get("clusterID")
	if clusterID == "" {
		http.Error(w, "clusterID parameter is required", http.StatusBadRequest)
		return
	}

	cluster := h.state.GetCluster(clusterID)
	affiliates := cluster.ListAffiliates()

	data := struct {
		ClusterID  string
		Affiliates []*state.Affiliate
	}{
		ClusterID:  clusterID,
		Affiliates: affiliates,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.template.ExecuteTemplate(w, "inspect.html.tmpl", data); err != nil {
		h.logger.Error("failed to render template", zap.Error(err))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}
