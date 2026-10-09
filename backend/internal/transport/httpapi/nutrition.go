package httpapi

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"avatar-id/internal/domain"
)

func (s *Server) foodSearch(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	page := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("page")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			http.Error(w, "invalid page", http.StatusBadRequest)
			return
		}
		page = parsed
	}
	exact := r.URL.Query().Get("exact") == "1"
	result, err := s.service.SearchCatalog(r.Context(), r.URL.Query().Get("q"), exact, page)
	if err != nil {
		s.writeFoodError(w, r, err)
		return
	}
	log.Printf("food search request_id=%s page=%d count=%d external_unavailable=%t duration_ms=%d",
		requestID(r), page, len(result.Foods), result.ExternalUnavailable, time.Since(started).Milliseconds())
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) recentFoods(w http.ResponseWriter, r *http.Request) {
	items, err := s.service.RecentCatalogFoods(r.Context(), r.URL.Query().Get("meal"))
	if err != nil {
		s.writeFoodError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) foodCatalogItem(w http.ResponseWriter, r *http.Request) {
	item, err := s.service.CatalogFood(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeFoodError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}
func (s *Server) deleteFoodCatalogItem(w http.ResponseWriter, r *http.Request) {
	if err := s.service.DeleteCatalogFood(r.Context(), r.PathValue("id")); err != nil {
		s.writeFoodError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) foodBarcode(w http.ResponseWriter, r *http.Request) {
	barcode := r.URL.Query().Get("value")
	if barcode == "" {
		barcode = r.PathValue("barcode")
	}
	item, err := s.service.CatalogBarcode(r.Context(), barcode)
	if err != nil {
		s.writeFoodError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}
func (s *Server) createFoodCatalogItem(w http.ResponseWriter, r *http.Request) {
	var item domain.FoodCatalogItem
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&item); err != nil {
		http.Error(w, "bad food json", http.StatusBadRequest)
		return
	}
	saved, err := s.service.CreateCatalogFood(r.Context(), item)
	if err != nil {
		s.writeFoodError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, saved)
}
func (s *Server) localNutrition(w http.ResponseWriter, r *http.Request) {
	item, err := s.service.LocalNutrition(r.Context(), r.URL.Query().Get("date"))
	if err != nil {
		s.writeFoodError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}
func (s *Server) createLocalNutritionEntry(w http.ResponseWriter, r *http.Request) {
	var input domain.LocalNutritionEntryInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&input); err != nil {
		http.Error(w, "bad nutrition json", http.StatusBadRequest)
		return
	}
	result, err := s.service.CreateLocalNutritionEntry(r.Context(), input)
	if err != nil {
		s.writeFoodError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}
func (s *Server) updateLocalNutritionEntry(w http.ResponseWriter, r *http.Request) {
	var input domain.LocalNutritionEntryInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&input); err != nil {
		http.Error(w, "bad nutrition json", http.StatusBadRequest)
		return
	}
	result, err := s.service.UpdateLocalNutritionEntry(r.Context(), r.PathValue("id"), input)
	if err != nil {
		s.writeFoodError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *Server) deleteLocalNutritionEntry(w http.ResponseWriter, r *http.Request) {
	result, err := s.service.DeleteLocalNutritionEntry(r.Context(), r.PathValue("id"), r.URL.Query().Get("date"))
	if err != nil {
		s.writeFoodError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *Server) writeFoodError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, domain.ErrNotFound) {
		http.Error(w, "продукт не найден", http.StatusNotFound)
		return
	}
	if errors.Is(err, domain.ErrInvalidInput) || strings.Contains(err.Error(), "invalid input") {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Printf("food operation request_id=%s: %s", requestID(r), sanitizeLogText(err.Error(), 2_000))
	http.Error(w, "не удалось обработать продукт", http.StatusBadGateway)
}
