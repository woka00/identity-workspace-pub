package application

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"avatar-id/internal/domain"
)

type catalogRepoStub struct {
	Repository
	saved          domain.FoodCatalogItem
	searchByQuery  map[string][]domain.FoodCatalogItem
	searchMore     bool
	searchQueries  []string
	recentMeal     string
	candidateInput domain.FoodCatalogCandidateInput
	linkedBarcode  string
	linkedFoodID   string
	deletedFoodID  string
	promotedID     int64
	rejectedID     int64
	updatedID      int64
	updatedReview  domain.FoodCatalogReviewInput
}

func (r *catalogRepoStub) SearchFoodCatalog(_ context.Context, query string, _ int) ([]domain.FoodCatalogItem, bool, error) {
	r.searchQueries = append(r.searchQueries, query)
	return append([]domain.FoodCatalogItem{}, r.searchByQuery[query]...), r.searchMore, nil
}

func (r *catalogRepoStub) DeleteFoodCatalog(_ context.Context, externalID string) error {
	r.deletedFoodID = externalID
	return nil
}

func (r *catalogRepoStub) FoodCatalogCandidates(_ context.Context, input domain.FoodCatalogCandidateInput) ([]domain.FoodCatalogItem, error) {
	r.candidateInput = input
	return []domain.FoodCatalogItem{{Name: "Match"}}, nil
}

func (r *catalogRepoStub) LinkFoodCatalogBarcode(_ context.Context, barcode, foodID string) (domain.FoodCatalogItem, error) {
	r.linkedBarcode = barcode
	r.linkedFoodID = foodID
	return domain.FoodCatalogItem{ID: foodID, Name: "Match"}, nil
}

func (r *catalogRepoStub) AdminFoodCatalog(_ context.Context, page int, pendingOnly bool) (domain.FoodCatalogReviewPage, error) {
	return domain.FoodCatalogReviewPage{Page: page, HasMore: pendingOnly}, nil
}

func (r *catalogRepoStub) PromoteFoodCatalogItem(_ context.Context, id int64) (domain.FoodCatalogItem, error) {
	r.promotedID = id
	return domain.FoodCatalogItem{Name: "Published"}, nil
}

func (r *catalogRepoStub) RejectFoodCatalogItem(_ context.Context, id int64) error {
	r.rejectedID = id
	return nil
}

func (r *catalogRepoStub) UpdateAdminFoodCatalog(_ context.Context, id int64, input domain.FoodCatalogReviewInput) (domain.FoodCatalogReviewItem, error) {
	r.updatedID = id
	r.updatedReview = input
	return domain.FoodCatalogReviewItem{ReviewID: id, FoodCatalogItem: domain.FoodCatalogItem{Name: input.Name}}, nil
}

func (r *catalogRepoStub) UpsertFoodCatalog(_ context.Context, item domain.FoodCatalogItem) (domain.FoodCatalogItem, error) {
	r.saved = item
	return item, nil
}

func (r *catalogRepoStub) RecentFoodCatalog(_ context.Context, meal string) ([]domain.FoodCatalogItem, error) {
	r.recentMeal = meal
	return []domain.FoodCatalogItem{}, nil
}

type catalogGatewayStub struct {
	items       []domain.FoodCatalogItem
	more        bool
	err         error
	queries     []string
	hasDeadline bool
	deadline    time.Time
}

func (g *catalogGatewayStub) Search(ctx context.Context, query string, _ int) ([]domain.FoodCatalogItem, bool, error) {
	g.queries = append(g.queries, query)
	g.deadline, g.hasDeadline = ctx.Deadline()
	return append([]domain.FoodCatalogItem{}, g.items...), g.more, g.err
}

func (g *catalogGatewayStub) Barcode(_ context.Context, _ string) (domain.FoodCatalogItem, error) {
	return domain.FoodCatalogItem{}, domain.ErrNotFound
}

func TestSearchCatalogUsesDatabaseWithoutExternalCallWhenPageIsFull(t *testing.T) {
	local := make([]domain.FoodCatalogItem, foodCatalogPageSize)
	for index := range local {
		local[index] = domain.FoodCatalogItem{Provider: domain.FoodProviderCommunity, ExternalID: strconv.Itoa(index + 1), Name: "Product"}
	}
	repo := &catalogRepoStub{searchByQuery: map[string][]domain.FoodCatalogItem{"молоко": local}, searchMore: true}
	gateway := &catalogGatewayStub{}
	service := New(repo, nil, time.Now).WithFoodCatalog(gateway)

	result, err := service.SearchCatalog(context.Background(), "Молоко", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Foods) != foodCatalogPageSize || !result.HasMore || len(gateway.queries) != 0 {
		t.Fatalf("unexpected full-page result: %+v, external calls=%v", result, gateway.queries)
	}
}

func TestSearchCatalogUsesReadOnlyExternalFallback(t *testing.T) {
	repo := &catalogRepoStub{searchByQuery: map[string][]domain.FoodCatalogItem{
		"рис": {{Provider: domain.FoodProviderCommunity, ExternalID: "local-result", Name: "Рис"}},
	}}
	gateway := &catalogGatewayStub{items: []domain.FoodCatalogItem{{
		Provider: domain.FoodProviderOpenFoodFacts, ExternalID: "4601", Barcode: "4601", Name: "Рис длиннозерный",
	}}, more: true}
	service := New(repo, nil, time.Now).WithFoodCatalog(gateway)

	result, err := service.SearchCatalog(context.Background(), "рис", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Foods) != 2 || !result.HasMore || len(gateway.queries) != 1 {
		t.Fatalf("unexpected fallback result: %+v, queries=%v", result, gateway.queries)
	}
	if !gateway.hasDeadline || time.Until(gateway.deadline) > externalFoodSearchBudget {
		t.Fatalf("external search has no bounded deadline: %v", gateway.deadline)
	}
	if repo.saved.Name != "" {
		t.Fatalf("search persisted an external card: %+v", repo.saved)
	}
	if result.Foods[1].ID != "open_food_facts:4601" {
		t.Fatalf("external result has no public id: %+v", result.Foods[1])
	}
}

func TestSearchCatalogReportsUnavailableExternalSource(t *testing.T) {
	repo := &catalogRepoStub{searchByQuery: map[string][]domain.FoodCatalogItem{}}
	gateway := &catalogGatewayStub{err: context.DeadlineExceeded}
	service := New(repo, nil, time.Now).WithFoodCatalog(gateway)

	result, err := service.SearchCatalog(context.Background(), "редкий продукт", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !result.ExternalUnavailable {
		t.Fatalf("external outage was hidden: %+v", result)
	}
}

func TestSearchCatalogCorrectionCanBeBypassed(t *testing.T) {
	corrected := domain.FoodCatalogItem{Provider: domain.FoodProviderCommunity, ExternalID: "1", Name: "Овсяная каша"}
	repo := &catalogRepoStub{searchByQuery: map[string][]domain.FoodCatalogItem{"овсяная каша": {corrected}}}
	service := New(repo, nil, time.Now)

	result, err := service.SearchCatalog(context.Background(), "овсянная каша", false, 0)
	if err != nil || result.CorrectedQuery != "овсяная каша" {
		t.Fatalf("correction result=%+v error=%v", result, err)
	}
	if _, err := service.SearchCatalog(context.Background(), "овсянная каша", true, 0); err != nil {
		t.Fatal(err)
	}
	if got := repo.searchQueries[len(repo.searchQueries)-1]; got != "овсянная каша" {
		t.Fatalf("exact search query=%q", got)
	}
}

func TestSearchCatalogRejectsUnboundedPage(t *testing.T) {
	service := New(&catalogRepoStub{}, nil, time.Now)
	if _, err := service.SearchCatalog(context.Background(), "рис", false, foodCatalogMaximumPage+1); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected invalid page, got %v", err)
	}
}

func TestRecentCatalogFoodsScopesResultsToMeal(t *testing.T) {
	repo := &catalogRepoStub{}
	service := New(repo, nil, time.Now)

	if _, err := service.RecentCatalogFoods(context.Background(), " Breakfast "); err != nil {
		t.Fatal(err)
	}
	if repo.recentMeal != "breakfast" {
		t.Fatalf("recent meal was not normalized: got %q", repo.recentMeal)
	}

	if _, err := service.RecentCatalogFoods(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if repo.recentMeal != "" {
		t.Fatalf("all-time recent foods must not be scoped to a meal: got %q", repo.recentMeal)
	}

	if _, err := service.RecentCatalogFoods(context.Background(), "brunch"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected invalid meal error, got %v", err)
	}
}

func TestCreateCatalogFoodDoesNotTrustClientIdentity(t *testing.T) {
	repo := &catalogRepoStub{}
	now := time.Date(2026, time.August, 24, 10, 30, 0, 123, time.UTC)
	service := New(repo, nil, func() time.Time { return now })

	_, err := service.CreateCatalogFood(context.Background(), domain.FoodCatalogItem{
		Provider:   domain.FoodProviderOpenFoodFacts,
		ExternalID: "another-users-product",
		Name:       "Product",
		Barcode:    "4600000000000",
	})
	if err != nil {
		t.Fatal(err)
	}
	if repo.saved.Provider != domain.FoodProviderLocal {
		t.Fatalf("provider was not forced to local: %q", repo.saved.Provider)
	}
	wantID := "1787567400000000123"
	if repo.saved.ExternalID != wantID {
		t.Fatalf("client identity was trusted: got %q, want %q", repo.saved.ExternalID, wantID)
	}
	if repo.saved.NutritionUnit != "g" || len(repo.saved.Servings) != 1 || repo.saved.Servings[0].ID != "g" {
		t.Fatalf("unexpected default nutrition format: %+v", repo.saved)
	}
}

func TestDeleteCatalogFoodAllowsOnlyLocalProvider(t *testing.T) {
	repo := &catalogRepoStub{}
	service := New(repo, nil, time.Now)

	if err := service.DeleteCatalogFood(context.Background(), "local:personal-42"); err != nil {
		t.Fatal(err)
	}
	if repo.deletedFoodID != "personal-42" {
		t.Fatalf("deleted food id = %q", repo.deletedFoodID)
	}

	for _, id := range []string{"community:shared-1", "open_food_facts:4600000000000", "invalid"} {
		repo.deletedFoodID = ""
		if err := service.DeleteCatalogFood(context.Background(), id); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("delete %q error = %v, want invalid input", id, err)
		}
		if repo.deletedFoodID != "" {
			t.Fatalf("repository was called for non-local food %q", id)
		}
	}
}

func TestCreateCatalogFoodBuildsVolumeAndPortionServings(t *testing.T) {
	repo := &catalogRepoStub{}
	service := New(repo, nil, time.Now)

	_, err := service.CreateCatalogFood(context.Background(), domain.FoodCatalogItem{
		Name: "Молоко", NutritionUnit: "ml", PortionAmount: 250,
		CaloriesPer100G: 60, ProteinPer100G: 3, FatPer100G: 3.2, CarbohydratePer100G: 4.7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.saved.Servings) != 2 {
		t.Fatalf("expected base unit and portion, got %+v", repo.saved.Servings)
	}
	base, portion := repo.saved.Servings[0], repo.saved.Servings[1]
	if base.ID != "ml" || base.Description != "мл" || base.NumberOfUnits != 100 {
		t.Fatalf("unexpected volume serving: %+v", base)
	}
	if portion.ID != "portion" || portion.MetricAmount != 250 || portion.NumberOfUnits != 1 || portion.Calories != 150 || portion.Protein != 7.5 {
		t.Fatalf("unexpected portion serving: %+v", portion)
	}
}

func TestCreateCatalogFoodRejectsInvalidFormatAndPortion(t *testing.T) {
	service := New(&catalogRepoStub{}, nil, time.Now)
	for _, item := range []domain.FoodCatalogItem{
		{Name: "Product", NutritionUnit: "oz"},
		{Name: "Product", NutritionUnit: "g", PortionAmount: -1},
		{Name: "Product", NutritionUnit: "ml", PortionAmount: 10001},
	} {
		if _, err := service.CreateCatalogFood(context.Background(), item); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("expected invalid food format or portion for %+v, got %v", item, err)
		}
	}
}

func TestDeduplicateCatalogPrefersLocalBarcodeCorrection(t *testing.T) {
	provider := domain.FoodCatalogItem{
		Provider: domain.FoodProviderOpenFoodFacts, ExternalID: "4600000000000", Barcode: "4600000000000", Name: "Wrong", DataQuality: 1,
	}
	local := domain.FoodCatalogItem{
		Provider: domain.FoodProviderLocal, ExternalID: "correction", Barcode: "4600000000000", Name: "Correct", DataQuality: 1,
	}

	for _, input := range [][]domain.FoodCatalogItem{{provider, local}, {local, provider}} {
		items := deduplicateCatalog(input)
		if len(items) != 1 || items[0].Name != "Correct" || items[0].ID != "local:correction" {
			t.Fatalf("unexpected catalog result: %+v", items)
		}
	}
}

func TestValidateCatalogFoodRejectsImpossibleNutrition(t *testing.T) {
	item := domain.FoodCatalogItem{Name: "Product", ProteinPer100G: 101}
	if err := validateCatalogFood(item); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
}

func TestValidateCatalogFoodAcceptsPrintableBarcode(t *testing.T) {
	for _, barcode := range []string{"4600000000000", "SKU-ABC/2026 01", strings.Repeat("A", 80)} {
		item := domain.FoodCatalogItem{Name: "Product", Barcode: barcode}
		if err := validateCatalogFood(item); err != nil {
			t.Fatalf("barcode value %q was rejected: %v", barcode, err)
		}
	}
}

func TestValidateCatalogFoodRejectsUnsafeBarcodeValues(t *testing.T) {
	for _, barcode := range []string{"ABC\n123", "ABC\x1d123", strings.Repeat("A", 513)} {
		item := domain.FoodCatalogItem{Name: "Product", Barcode: barcode}
		if err := validateCatalogFood(item); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("expected invalid barcode %q, got %v", barcode, err)
		}
	}
}

func TestCatalogCandidatesAndBarcodeLink(t *testing.T) {
	repo := &catalogRepoStub{}
	service := New(repo, nil, time.Now)
	items, err := service.CatalogCandidates(context.Background(), domain.FoodCatalogCandidateInput{
		Name: " Milk ", CaloriesPer100G: 60, ProteinPer100G: 3.2, FatPer100G: 3.5, CarbohydratePer100G: 4.7,
	})
	if err != nil || len(items) != 1 || repo.candidateInput.Name != "Milk" {
		t.Fatalf("candidate search = %+v, input=%+v, error=%v", items, repo.candidateInput, err)
	}
	if _, err := service.LinkCatalogBarcode(context.Background(), " SKU-1   LOT-4 ", "community:food-1"); err != nil {
		t.Fatal(err)
	}
	if repo.linkedBarcode != "SKU-1 LOT-4" || repo.linkedFoodID != "community:food-1" {
		t.Fatalf("barcode link = %q -> %q", repo.linkedBarcode, repo.linkedFoodID)
	}
	if repo.candidateInput.NutritionUnit != "g" {
		t.Fatalf("candidate nutrition unit was not normalized: %q", repo.candidateInput.NutritionUnit)
	}
}

func TestFoodReviewRequiresAdmin(t *testing.T) {
	repo := &catalogRepoStub{}
	service := New(repo, nil, time.Now)
	userContext := WithUser(context.Background(), domain.User{ID: 1})
	if _, err := service.ReviewUserFoods(userContext, 0, true); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("non-admin review error = %v", err)
	}
	adminContext := WithUser(context.Background(), domain.User{ID: 1, IsAdmin: true})
	page, err := service.ReviewUserFoods(adminContext, 2, true)
	if err != nil || page.Page != 2 || !page.HasMore {
		t.Fatalf("admin review page = %+v, error=%v", page, err)
	}
	if _, err := service.PromoteUserFood(adminContext, 42); err != nil || repo.promotedID != 42 {
		t.Fatalf("promotion id=%d, error=%v", repo.promotedID, err)
	}
	if err := service.RejectUserFood(adminContext, 43); err != nil || repo.rejectedID != 43 {
		t.Fatalf("rejection id=%d, error=%v", repo.rejectedID, err)
	}
	updated, err := service.UpdateUserFoodReview(adminContext, 42, domain.FoodCatalogReviewInput{
		Name: " Исправленный продукт ", BrandName: " Марка ", Barcode: " CODE-39 ",
		CaloriesPer100G: 120, ProteinPer100G: 5, FatPer100G: 4, CarbohydratePer100G: 16,
	})
	if err != nil || updated.ReviewID != 42 || repo.updatedID != 42 {
		t.Fatalf("review update = %+v, id=%d, error=%v", updated, repo.updatedID, err)
	}
	if repo.updatedReview.Name != "Исправленный продукт" || repo.updatedReview.Barcode != "CODE-39" {
		t.Fatalf("review update was not normalized: %+v", repo.updatedReview)
	}
	if _, err := service.UpdateUserFoodReview(userContext, 42, domain.FoodCatalogReviewInput{Name: "Product"}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("non-admin update error = %v", err)
	}
	if err := service.RejectUserFood(userContext, 43); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("non-admin rejection error = %v", err)
	}
	if err := service.RejectUserFood(adminContext, 0); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("invalid rejection id error = %v", err)
	}
}
