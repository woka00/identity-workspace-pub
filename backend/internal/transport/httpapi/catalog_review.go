package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"avatar-id/internal/domain"
)

func parseFoodMacroQuery(r *http.Request) (domain.FoodCatalogCandidateInput, error) {
	parse := func(key string) (float64, error) {
		return strconv.ParseFloat(strings.TrimSpace(r.URL.Query().Get(key)), 64)
	}
	calories, err := parse("calories")
	if err != nil {
		return domain.FoodCatalogCandidateInput{}, domain.InvalidInputError{Message: "неверное значение калорий"}
	}
	protein, err := parse("protein")
	if err != nil {
		return domain.FoodCatalogCandidateInput{}, domain.InvalidInputError{Message: "неверное значение белков"}
	}
	fat, err := parse("fat")
	if err != nil {
		return domain.FoodCatalogCandidateInput{}, domain.InvalidInputError{Message: "неверное значение жиров"}
	}
	carbohydrate, err := parse("carbohydrate")
	if err != nil {
		return domain.FoodCatalogCandidateInput{}, domain.InvalidInputError{Message: "неверное значение углеводов"}
	}
	return domain.FoodCatalogCandidateInput{
		Name: r.URL.Query().Get("name"), BrandName: r.URL.Query().Get("brand"),
		NutritionUnit:   r.URL.Query().Get("unit"),
		CaloriesPer100G: calories, ProteinPer100G: protein,
		FatPer100G: fat, CarbohydratePer100G: carbohydrate,
	}, nil
}

func (s *Server) foodCandidates(w http.ResponseWriter, r *http.Request) {
	input, err := parseFoodMacroQuery(r)
	if err != nil {
		writeError(w, err)
		return
	}
	items, err := s.service.CatalogCandidates(r.Context(), input)
	respond(w, items, err, http.StatusOK)
}

func (s *Server) linkFoodBarcode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Barcode string `json:"barcode"`
		FoodID  string `json:"foodId"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		http.Error(w, "bad barcode link json", http.StatusBadRequest)
		return
	}
	item, err := s.service.LinkCatalogBarcode(r.Context(), body.Barcode, body.FoodID)
	respond(w, item, err, http.StatusCreated)
}

func (s *Server) adminFoodCatalog(w http.ResponseWriter, r *http.Request) {
	page := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("page")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, domain.InvalidInputError{Message: "неверная страница"})
			return
		}
		page = parsed
	}
	pendingOnly := r.URL.Query().Get("status") != "all"
	result, err := s.service.ReviewUserFoods(r.Context(), page, pendingOnly)
	respond(w, result, err, http.StatusOK)
}

func (s *Server) promoteUserFood(w http.ResponseWriter, r *http.Request) {
	reviewID, err := strconv.ParseInt(strings.TrimSpace(r.PathValue("id")), 10, 64)
	if err != nil {
		writeError(w, domain.InvalidInputError{Message: "неверный идентификатор продукта"})
		return
	}
	item, err := s.service.PromoteUserFood(r.Context(), reviewID)
	respond(w, item, err, http.StatusOK)
}

func (s *Server) rejectUserFood(w http.ResponseWriter, r *http.Request) {
	reviewID, err := strconv.ParseInt(strings.TrimSpace(r.PathValue("id")), 10, 64)
	if err != nil {
		writeError(w, domain.InvalidInputError{Message: "неверный идентификатор продукта"})
		return
	}
	if err := s.service.RejectUserFood(r.Context(), reviewID); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) updateUserFoodReview(w http.ResponseWriter, r *http.Request) {
	reviewID, err := strconv.ParseInt(strings.TrimSpace(r.PathValue("id")), 10, 64)
	if err != nil {
		writeError(w, domain.InvalidInputError{Message: "неверный идентификатор продукта"})
		return
	}
	var input domain.FoodCatalogReviewInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&input); err != nil {
		http.Error(w, "bad food review json", http.StatusBadRequest)
		return
	}
	item, err := s.service.UpdateUserFoodReview(r.Context(), reviewID, input)
	respond(w, item, err, http.StatusOK)
}
