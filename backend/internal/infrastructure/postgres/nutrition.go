package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"avatar-id/internal/domain"
)

const createNutritionEntryQuery = `INSERT INTO user_nutrition_entries (user_id,entry_date,meal,food_id,name,description,number_of_units,calories,carbohydrate,protein,fat,serving_id) SELECT $1,$2,$3,id,$4,$5,$6,$7,$8,$9,$10,$11 FROM food_catalog WHERE provider=$12 AND external_id=$13 AND (provider<>'local' OR owner_user_id=$1)`

func createNutritionEntryArgs(userID int64, input domain.LocalNutritionEntryInput, item domain.FoodCatalogItem, serving domain.FoodServing, calories, carbs, protein, fat float64) [13]any {
	return [13]any{userID, input.Date, input.Meal, item.Name, serving.Description, input.NumberOfUnits, calories, carbs, protein, fat, serving.ID, item.Provider, item.ExternalID}
}

func catalogPublicID(provider domain.FoodProvider, externalID string) string {
	return string(provider) + ":" + externalID
}

func splitCatalogID(value string) (domain.FoodProvider, string, error) {
	parts := strings.SplitN(strings.TrimSpace(value), ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid food id: %w", domain.ErrInvalidInput)
	}
	return domain.FoodProvider(parts[0]), parts[1], nil
}

func scanCatalogItem(scanner interface{ Scan(...any) error }) (domain.FoodCatalogItem, error) {
	var item domain.FoodCatalogItem
	var servings []byte
	var provider string
	if err := scanner.Scan(&provider, &item.ExternalID, &item.Barcode, &item.Name, &item.BrandName, &item.Description,
		&item.CaloriesPer100G, &item.CarbohydratePer100G, &item.ProteinPer100G, &item.FatPer100G, &servings, &item.DataQuality); err != nil {
		return item, err
	}
	item.Provider = domain.FoodProvider(provider)
	item.ID = catalogPublicID(item.Provider, item.ExternalID)
	if len(servings) > 0 {
		if err := json.Unmarshal(servings, &item.Servings); err != nil {
			return item, err
		}
	}
	// Older cached products exposed the nutrition reference amount (100 g) as
	// the unit label. The editable amount is grams; 100 is only the reference
	// quantity used by the nutrition values.
	for index := range item.Servings {
		if item.Servings[index].ID == "100g" && item.Servings[index].MetricUnit == "g" {
			item.Servings[index].ID = "g"
			item.Servings[index].Description = "г"
			item.Servings[index].MetricAmount = 1
			item.Servings[index].Measurement = "г"
		}
	}
	if item.Servings == nil {
		item.Servings = []domain.FoodServing{}
	}
	return item, nil
}

func (s *Repository) SearchFoodCatalog(ctx context.Context, query string, page int) ([]domain.FoodCatalogItem, bool, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, false, err
	}
	if page < 0 {
		return nil, false, fmt.Errorf("invalid page: %w", domain.ErrInvalidInput)
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return []domain.FoodCatalogItem{}, false, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		WITH user_usage AS (
			SELECT food_id, count(*) AS usage_count, max(updated_at) AS last_used_at
			FROM user_nutrition_entries
			WHERE user_id=$1 AND food_id IS NOT NULL
			GROUP BY food_id
		), searchable AS (
			SELECT food.*,
			       replace(lower(food.name || ' ' || food.brand_name), 'ё', 'е') AS search_text,
			       replace(lower(food.name), 'ё', 'е') AS normalized_name,
			       setweight(to_tsvector('simple', replace(lower(food.name), 'ё', 'е')), 'A') ||
			         setweight(to_tsvector('simple', replace(lower(food.brand_name), 'ё', 'е')), 'B') AS simple_vector,
			       setweight(to_tsvector('russian', replace(lower(food.name), 'ё', 'е')), 'A') ||
			         setweight(to_tsvector('russian', replace(lower(food.brand_name), 'ё', 'е')), 'B') AS russian_vector,
			       usage.usage_count, usage.last_used_at
			FROM food_catalog AS food
			LEFT JOIN user_usage AS usage ON usage.food_id=food.id
			WHERE food.provider <> 'local' OR food.owner_user_id=$1
		), ranked AS (
			SELECT searchable.*,
			       plainto_tsquery('simple', $2) AS simple_query,
			       plainto_tsquery('russian', $2) AS russian_query,
			       COALESCE((
			         SELECT string_agg(quote_literal(lexeme) || ':*', ' & ')::tsquery
			         FROM unnest(tsvector_to_array(to_tsvector('simple', $2))) AS terms(lexeme)
			       ), ''::tsquery) AS simple_prefix_query,
			       COALESCE((
			         SELECT string_agg(quote_literal(lexeme) || ':*', ' & ')::tsquery
			         FROM unnest(tsvector_to_array(to_tsvector('russian', $2))) AS terms(lexeme)
			       ), ''::tsquery) AS russian_prefix_query
			FROM searchable
			WHERE barcode=$2
			   OR to_tsvector('simple', search_text) @@ plainto_tsquery('simple', $2)
			   OR to_tsvector('russian', search_text) @@ plainto_tsquery('russian', $2)
			   OR to_tsvector('simple', search_text) @@ COALESCE((
			        SELECT string_agg(quote_literal(lexeme) || ':*', ' & ')::tsquery
			        FROM unnest(tsvector_to_array(to_tsvector('simple', $2))) AS terms(lexeme)
			      ), ''::tsquery)
			   OR to_tsvector('russian', search_text) @@ COALESCE((
			        SELECT string_agg(quote_literal(lexeme) || ':*', ' & ')::tsquery
			        FROM unnest(tsvector_to_array(to_tsvector('russian', $2))) AS terms(lexeme)
			      ), ''::tsquery)
			   OR search_text LIKE '%' || $2 || '%'
			   OR $2 <% search_text
		)
		SELECT provider, external_id, barcode, name, brand_name, description,
		       calories_per_100g, carbohydrate_per_100g, protein_per_100g, fat_per_100g,
		       servings, data_quality
		FROM ranked
		ORDER BY
		  (barcode=$2) DESC,
		  (normalized_name=$2) DESC,
		  (strpos(normalized_name, $2)=1) DESC,
		  greatest(
			ts_rank_cd(simple_vector, simple_query),
			ts_rank_cd(russian_vector, russian_query),
			ts_rank_cd(simple_vector, simple_prefix_query),
			ts_rank_cd(russian_vector, russian_prefix_query)
		  ) DESC,
		  COALESCE(last_used_at >= now() - interval '30 days', false) DESC,
		  (provider = 'local') DESC,
		  COALESCE(usage_count, 0) DESC,
		  word_similarity($2, search_text) DESC,
		  data_quality DESC, name, provider, external_id
		LIMIT 21 OFFSET $3`, userID, query, page*20)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	items := make([]domain.FoodCatalogItem, 0, 20)
	hasMore := false
	for rows.Next() {
		if len(items) == 20 {
			hasMore = true
			break
		}
		item, err := scanCatalogItem(rows)
		if err != nil {
			return nil, false, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	return items, hasMore, nil
}

func (s *Repository) RecentFoodCatalog(ctx context.Context, meal string) ([]domain.FoodCatalogItem, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		WITH recent AS (
			SELECT DISTINCT ON (food_id)
			       food_id, serving_id, number_of_units, updated_at
			FROM user_nutrition_entries
			WHERE user_id=$1 AND food_id IS NOT NULL AND ($2='' OR meal=$2)
			ORDER BY food_id, updated_at DESC, id DESC
		)
		SELECT food.provider, food.external_id, food.barcode, food.name, food.brand_name, food.description,
		       food.calories_per_100g, food.carbohydrate_per_100g, food.protein_per_100g, food.fat_per_100g,
		       food.servings, food.data_quality, recent.serving_id, recent.number_of_units
		FROM recent
		JOIN food_catalog AS food ON food.id=recent.food_id
		WHERE food.provider <> 'local' OR food.owner_user_id=$1
		ORDER BY recent.updated_at DESC
		LIMIT 20`, userID, meal)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.FoodCatalogItem, 0, 20)
	for rows.Next() {
		var item domain.FoodCatalogItem
		var servings []byte
		var provider string
		if err := rows.Scan(&provider, &item.ExternalID, &item.Barcode, &item.Name, &item.BrandName, &item.Description,
			&item.CaloriesPer100G, &item.CarbohydratePer100G, &item.ProteinPer100G, &item.FatPer100G,
			&servings, &item.DataQuality, &item.ServingID, &item.Units); err != nil {
			return nil, err
		}
		item.Provider = domain.FoodProvider(provider)
		item.ID = catalogPublicID(item.Provider, item.ExternalID)
		if len(servings) > 0 {
			if err := json.Unmarshal(servings, &item.Servings); err != nil {
				return nil, err
			}
		}
		if item.Servings == nil {
			item.Servings = []domain.FoodServing{}
		}
		for index := range item.Servings {
			if item.Servings[index].ID == "100g" && item.Servings[index].MetricUnit == "g" {
				item.Servings[index].ID = "g"
				item.Servings[index].Description = "г"
				item.Servings[index].MetricAmount = 1
				item.Servings[index].Measurement = "г"
				if item.ServingID == "100g" {
					item.ServingID = "g"
				}
			}
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Repository) FoodCatalogItem(ctx context.Context, publicID string) (domain.FoodCatalogItem, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return domain.FoodCatalogItem{}, err
	}
	provider, externalID, err := splitCatalogID(publicID)
	if err != nil {
		return domain.FoodCatalogItem{}, err
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT provider, external_id, barcode, name, brand_name, description,
		       calories_per_100g, carbohydrate_per_100g, protein_per_100g, fat_per_100g,
		       servings, data_quality
		FROM food_catalog
		WHERE provider=$1 AND external_id=$2
		  AND (provider <> 'local' OR owner_user_id=$3)`, provider, externalID, userID)
	item, err := scanCatalogItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.FoodCatalogItem{}, domain.ErrNotFound
	}
	return item, err
}

func (s *Repository) DeleteFoodCatalog(ctx context.Context, externalID string) error {
	userID, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM food_catalog
		WHERE provider='local' AND external_id=$1 AND owner_user_id=$2`, externalID, userID)
	if err != nil {
		return err
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if deleted == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Repository) FoodCatalogByBarcode(ctx context.Context, barcode string) (domain.FoodCatalogItem, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return domain.FoodCatalogItem{}, err
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT provider, external_id, barcode, name, brand_name, description,
		       calories_per_100g, carbohydrate_per_100g, protein_per_100g, fat_per_100g,
		       servings, data_quality
		FROM (
			SELECT food.*, 0 AS match_priority
			FROM user_food_barcode_links AS link
			JOIN food_catalog AS food ON food.id=link.food_id
			WHERE link.user_id=$2 AND link.barcode=$1
			  AND (food.provider <> 'local' OR food.owner_user_id=$2)
			UNION ALL
			SELECT food.*, 1 AS match_priority
			FROM food_catalog AS food
			WHERE food.barcode=$1 AND (food.provider <> 'local' OR food.owner_user_id=$2)
		) AS matched
		ORDER BY match_priority, (provider = 'local') DESC, data_quality DESC
		LIMIT 1`, barcode, userID)
	item, err := scanCatalogItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.FoodCatalogItem{}, domain.ErrNotFound
	}
	return item, err
}

func (s *Repository) UpsertFoodCatalog(ctx context.Context, item domain.FoodCatalogItem) (domain.FoodCatalogItem, error) {
	if item.Provider == "" || strings.TrimSpace(item.ExternalID) == "" || strings.TrimSpace(item.Name) == "" {
		return domain.FoodCatalogItem{}, fmt.Errorf("food catalog item is incomplete: %w", domain.ErrInvalidInput)
	}
	servings, err := json.Marshal(item.Servings)
	if err != nil {
		return domain.FoodCatalogItem{}, err
	}
	if item.Provider == domain.FoodProviderLocal {
		userID, userErr := currentUserID(ctx)
		if userErr != nil {
			return domain.FoodCatalogItem{}, userErr
		}
		if item.Barcode == "" {
			err = s.db.QueryRowContext(ctx, `
				INSERT INTO food_catalog (provider, external_id, barcode, name, brand_name, description,
					calories_per_100g, carbohydrate_per_100g, protein_per_100g, fat_per_100g, servings, data_quality, owner_user_id)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
				ON CONFLICT (provider, external_id) DO UPDATE SET
					name=EXCLUDED.name, brand_name=EXCLUDED.brand_name, description=EXCLUDED.description,
					calories_per_100g=EXCLUDED.calories_per_100g,
					carbohydrate_per_100g=EXCLUDED.carbohydrate_per_100g, protein_per_100g=EXCLUDED.protein_per_100g,
					fat_per_100g=EXCLUDED.fat_per_100g, servings=EXCLUDED.servings,
					data_quality=EXCLUDED.data_quality, cached_at=now()
				WHERE food_catalog.owner_user_id=EXCLUDED.owner_user_id
				RETURNING external_id`, item.Provider, item.ExternalID, item.Barcode,
				item.Name, item.BrandName, item.Description, item.CaloriesPer100G, item.CarbohydratePer100G,
				item.ProteinPer100G, item.FatPer100G, servings, item.DataQuality, userID).Scan(&item.ExternalID)
		} else {
			err = s.db.QueryRowContext(ctx, `
				INSERT INTO food_catalog (provider, external_id, barcode, name, brand_name, description,
					calories_per_100g, carbohydrate_per_100g, protein_per_100g, fat_per_100g, servings, data_quality, owner_user_id)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
				ON CONFLICT (owner_user_id, barcode)
				WHERE provider='local' AND owner_user_id IS NOT NULL AND barcode <> '' DO UPDATE SET
				name=EXCLUDED.name, brand_name=EXCLUDED.brand_name, description=EXCLUDED.description,
				calories_per_100g=EXCLUDED.calories_per_100g,
				carbohydrate_per_100g=EXCLUDED.carbohydrate_per_100g, protein_per_100g=EXCLUDED.protein_per_100g,
				fat_per_100g=EXCLUDED.fat_per_100g, servings=EXCLUDED.servings,
				data_quality=EXCLUDED.data_quality, cached_at=now()
				RETURNING external_id`, item.Provider, item.ExternalID, item.Barcode,
				item.Name, item.BrandName, item.Description, item.CaloriesPer100G, item.CarbohydratePer100G,
				item.ProteinPer100G, item.FatPer100G, servings, item.DataQuality, userID).Scan(&item.ExternalID)
		}
	} else {
		_, err = s.db.ExecContext(ctx, `
		INSERT INTO food_catalog (provider, external_id, barcode, name, brand_name, description,
			calories_per_100g, carbohydrate_per_100g, protein_per_100g, fat_per_100g, servings, data_quality)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (provider, external_id) DO UPDATE SET
			barcode=EXCLUDED.barcode, name=EXCLUDED.name, brand_name=EXCLUDED.brand_name,
			description=EXCLUDED.description, calories_per_100g=EXCLUDED.calories_per_100g,
			carbohydrate_per_100g=EXCLUDED.carbohydrate_per_100g, protein_per_100g=EXCLUDED.protein_per_100g,
			fat_per_100g=EXCLUDED.fat_per_100g, servings=EXCLUDED.servings,
			data_quality=EXCLUDED.data_quality, cached_at=now()`, item.Provider, item.ExternalID, item.Barcode,
			item.Name, item.BrandName, item.Description, item.CaloriesPer100G, item.CarbohydratePer100G,
			item.ProteinPer100G, item.FatPer100G, servings, item.DataQuality)
	}
	if err != nil {
		return domain.FoodCatalogItem{}, err
	}
	item.ID = catalogPublicID(item.Provider, item.ExternalID)
	return item, nil
}

func (s *Repository) Nutrition(ctx context.Context, date string) (domain.Nutrition, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return domain.Nutrition{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT e.id, COALESCE(c.provider,''), COALESCE(c.external_id,''), e.name, e.description, e.meal, e.number_of_units, e.calories, e.carbohydrate, e.protein, e.fat, e.serving_id FROM user_nutrition_entries e LEFT JOIN food_catalog c ON c.id=e.food_id WHERE e.user_id=$1 AND e.entry_date=$2 ORDER BY e.id`, userID, date)
	if err != nil {
		return domain.Nutrition{}, err
	}
	defer rows.Close()
	result := domain.Nutrition{Date: date, Meals: []domain.MealNutrition{}, FetchedAt: "local"}
	byMeal := map[string]*domain.MealNutrition{}
	order := []string{"Breakfast", "Lunch", "Dinner", "Other"}
	for _, meal := range order {
		byMeal[meal] = &domain.MealNutrition{Meal: meal, Entries: []domain.NutritionEntry{}}
	}
	for rows.Next() {
		var entry domain.NutritionEntry
		var servingID, provider, externalID string
		if err := rows.Scan(&entry.ID, &provider, &externalID, &entry.Name, &entry.Description, &entry.Meal, &entry.NumberOfUnits, &entry.Calories, &entry.Carbohydrate, &entry.Protein, &entry.Fat, &servingID); err != nil {
			return domain.Nutrition{}, err
		}
		entry.ServingID = servingID
		entry.ID = "local-entry:" + entry.ID
		switch entry.Meal {
		case "breakfast":
			entry.Meal = "Breakfast"
		case "lunch":
			entry.Meal = "Lunch"
		case "dinner":
			entry.Meal = "Dinner"
		default:
			entry.Meal = "Other"
		}
		if provider != "" && externalID != "" {
			entry.FoodID = catalogPublicID(domain.FoodProvider(provider), externalID)
		}
		meal := byMeal[entry.Meal]
		if meal == nil {
			meal = &domain.MealNutrition{Meal: entry.Meal, Entries: []domain.NutritionEntry{}}
			byMeal[entry.Meal] = meal
			order = append(order, entry.Meal)
		}
		meal.Entries = append(meal.Entries, entry)
		meal.EntryCount++
		meal.Calories += entry.Calories
		meal.Carbohydrate += entry.Carbohydrate
		meal.Protein += entry.Protein
		meal.Fat += entry.Fat
		result.EntryCount++
		result.Calories += entry.Calories
		result.Carbohydrate += entry.Carbohydrate
		result.Protein += entry.Protein
		result.Fat += entry.Fat
	}
	for _, mealName := range order {
		if meal := byMeal[mealName]; meal != nil && meal.EntryCount > 0 {
			result.Meals = append(result.Meals, *meal)
		}
	}
	return result, rows.Err()
}

func (s *Repository) localFoodForEntry(ctx context.Context, id string) (domain.FoodCatalogItem, error) {
	return s.FoodCatalogItem(ctx, id)
}

func servingForEntry(item domain.FoodCatalogItem, servingID string) (domain.FoodServing, error) {
	for _, serving := range item.Servings {
		if serving.ID == servingID {
			return serving, nil
		}
	}
	return domain.FoodServing{}, fmt.Errorf("serving not found: %w", domain.ErrInvalidInput)
}

func (s *Repository) CreateNutritionEntry(ctx context.Context, input domain.LocalNutritionEntryInput) (domain.Nutrition, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return domain.Nutrition{}, err
	}
	item, err := s.localFoodForEntry(ctx, input.FoodID)
	if err != nil {
		return domain.Nutrition{}, err
	}
	serving, err := servingForEntry(item, input.ServingID)
	if err != nil {
		return domain.Nutrition{}, err
	}
	calories := serving.Calories * input.NumberOfUnits / serving.NumberOfUnits
	carbs := serving.Carbohydrate * input.NumberOfUnits / serving.NumberOfUnits
	protein := serving.Protein * input.NumberOfUnits / serving.NumberOfUnits
	fat := serving.Fat * input.NumberOfUnits / serving.NumberOfUnits
	args := createNutritionEntryArgs(userID, input, item, serving, calories, carbs, protein, fat)
	_, err = s.db.ExecContext(ctx, createNutritionEntryQuery, args[:]...)
	if err != nil {
		return domain.Nutrition{}, err
	}
	return s.Nutrition(ctx, input.Date)
}

func (s *Repository) UpdateNutritionEntry(ctx context.Context, id int64, input domain.LocalNutritionEntryInput) (domain.Nutrition, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return domain.Nutrition{}, err
	}
	item, err := s.localFoodForEntry(ctx, input.FoodID)
	if err != nil {
		return domain.Nutrition{}, err
	}
	serving, err := servingForEntry(item, input.ServingID)
	if err != nil {
		return domain.Nutrition{}, err
	}
	calories := serving.Calories * input.NumberOfUnits / serving.NumberOfUnits
	carbs := serving.Carbohydrate * input.NumberOfUnits / serving.NumberOfUnits
	protein := serving.Protein * input.NumberOfUnits / serving.NumberOfUnits
	fat := serving.Fat * input.NumberOfUnits / serving.NumberOfUnits
	_, err = s.db.ExecContext(ctx, `UPDATE user_nutrition_entries SET entry_date=$1, meal=$2, food_id=(SELECT id FROM food_catalog WHERE provider=$3 AND external_id=$4 AND (provider<>'local' OR owner_user_id=$14)), name=$5, description=$6, number_of_units=$7, calories=$8, carbohydrate=$9, protein=$10, fat=$11, serving_id=$12, updated_at=now() WHERE id=$13 AND user_id=$14`, input.Date, input.Meal, item.Provider, item.ExternalID, item.Name, serving.Description, input.NumberOfUnits, calories, carbs, protein, fat, input.ServingID, id, userID)
	if err != nil {
		return domain.Nutrition{}, err
	}
	return s.Nutrition(ctx, input.Date)
}
func (s *Repository) DeleteNutritionEntry(ctx context.Context, id int64, date string) (domain.Nutrition, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return domain.Nutrition{}, err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM user_nutrition_entries WHERE id=$1 AND user_id=$2`, id, userID)
	if err != nil {
		return domain.Nutrition{}, err
	}
	return s.Nutrition(ctx, date)
}

func parseLocalEntryID(value string) (int64, error) {
	return strconv.ParseInt(strings.TrimPrefix(value, "local-entry:"), 10, 64)
}
