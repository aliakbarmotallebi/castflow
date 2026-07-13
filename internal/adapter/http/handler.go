package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"castflow/internal/application"
	"castflow/internal/domain"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

// Handler exposes REST API for video management.
type Handler struct {
	uploadUC      *application.UploadVideo
	getVideoUC    *application.GetVideo
	listUC        *application.ListVideos
	linksUC       *application.GetVideoLinks
	deleteUC      *application.DeleteVideo
	retranscodeUC *application.RetranscodeVideo
	apiKey        string
}

type Deps struct {
	Upload      *application.UploadVideo
	GetVideo    *application.GetVideo
	List        *application.ListVideos
	Links       *application.GetVideoLinks
	Delete      *application.DeleteVideo
	Retranscode *application.RetranscodeVideo
	APIKey      string
}

func NewHandler(d Deps) *Handler {
	return &Handler{
		uploadUC:      d.Upload,
		getVideoUC:    d.GetVideo,
		listUC:        d.List,
		linksUC:       d.Links,
		deleteUC:      d.Delete,
		retranscodeUC: d.Retranscode,
		apiKey:        d.APIKey,
	}
}

func (h *Handler) Router() chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(120 * time.Second))

	r.Get("/health", h.health)
	r.Get("/ready", h.ready)

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(h.apiKeyAuth)
		r.Route("/videos", func(r chi.Router) {
			r.Get("/", h.listVideos)
			r.Post("/upload", h.uploadVideo)
			r.Get("/{id}", h.getVideo)
			r.Get("/{id}/links", h.getLinks)
			r.Get("/{id}/status", h.getStatus)
			r.Post("/{id}/retranscode", h.retranscodeVideo)
			r.Delete("/{id}", h.deleteVideo)
		})
	})

	return r
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) ready(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (h *Handler) apiKeyAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-API-Key")
		if key == "" {
			key = r.URL.Query().Get("api_key")
		}
		if h.apiKey != "" && key != h.apiKey {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) uploadVideo(w http.ResponseWriter, r *http.Request) {
	const maxMem = 32 << 20
	if err := r.ParseMultipartForm(maxMem); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file field required")
		return
	}
	defer file.Close()

	title := r.FormValue("title")
	if title == "" {
		title = header.Filename
	}

	out, err := h.uploadUC.Execute(r.Context(), application.UploadInput{
		Title:       title,
		Description: r.FormValue("description"),
		ContentType: header.Header.Get("Content-Type"),
		FileSize:    header.Size,
		Body:        file,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"id":      out.Video.ID.String(),
		"title":   out.Video.Title,
		"status":  out.Video.Status,
		"message": out.Message,
	})
}

func (h *Handler) listVideos(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	videos, total, err := h.listUC.Execute(r.Context(), limit, offset)
	if err != nil {
		writeAppError(w, err)
		return
	}
	items := make([]map[string]any, len(videos))
	for i, v := range videos {
		items[i] = videoDTO(v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (h *Handler) getVideo(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	v, err := h.getVideoUC.Execute(r.Context(), id)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, videoDTO(v))
}

func (h *Handler) getLinks(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	out, err := h.linksUC.Execute(r.Context(), id)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"videoId":    out.Video.ID.String(),
		"title":      out.Video.Title,
		"status":     out.Status,
		"primary":    out.Primary,
		"renditions": out.Renditions,
		"links":      out.Links,
	})
}

func (h *Handler) retranscodeVideo(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var body struct {
		Profiles []string `json:"profiles"`
		Force    bool     `json:"force"`
	}
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
	}

	if err := h.retranscodeUC.Execute(r.Context(), id, application.RetranscodeInput{
		Profiles: body.Profiles,
		Force:    body.Force,
	}); err != nil {
		writeAppError(w, err)
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"id":      id.String(),
		"status":  domain.StatusProcessing,
		"message": "retranscode scheduled",
	})
}

func (h *Handler) getStatus(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	v, err := h.getVideoUC.Execute(r.Context(), id)
	if err != nil {
		writeAppError(w, err)
		return
	}
	resp := map[string]any{
		"id":     v.ID.String(),
		"status": v.Status,
	}
	if v.ErrorMessage != "" {
		resp["error"] = v.ErrorMessage
	}
	if v.Status == domain.StatusReady {
		resp["durationSec"] = v.DurationSec
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) deleteVideo(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.deleteUC.Execute(r.Context(), id); err != nil {
		writeAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func videoDTO(v *domain.Video) map[string]any {
	return map[string]any{
		"id":          v.ID.String(),
		"title":       v.Title,
		"description": v.Description,
		"status":      v.Status,
		"durationSec": v.DurationSec,
		"fileSize":    v.FileSize,
		"contentType": v.ContentType,
		"createdAt":   v.CreatedAt.UTC().Format(time.RFC3339),
		"updatedAt":   v.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeAppError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, domain.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrNotReady):
		writeError(w, http.StatusConflict, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}
