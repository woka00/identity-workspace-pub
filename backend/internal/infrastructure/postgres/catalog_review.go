package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"avatar-id/internal/application"
	"avatar-id/internal/domain"
	"github.com/lib/pq"
)

func foodReviewWriteError(operation string, err error) error {
	var postgresError *pq.Error
	if errors.As(err, &postgresError) && postgresError.Code == "23505" {
		return fmt.Errorf("штрихкод уже принадлежит другому глобальному продукту: %w", domain.ErrConflict)
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func (s *Repository) FoodCatalogCandidates(ctx context.Context, input domain.FoodCatalogCandidateInput) ([]domain.FoodCatalogItem, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT provider, external_id, barcode, name, brand_name, description,
		       calories_per_100g, carbohydrate_per_100g, protein_per_100g, fat_per_100g,
		       servings, data_quality
		FROM food_catalog
		WHERE (provider <> 'local' OR owner_user_id=$1)
		  AND COALESCE(NULLIF(servings->0->>'metricUnit', ''), 'g')=$8
		  AND calories_per_100g BETWEEN $2-10.0 AND $2+10.0
		  AND protein_per_100g BETWEEN $3-1.5 AND $3+1.5
		  AND fat_per_100g BETWEEN $4-1.5 AND $4+1.5
		  AND carbohydrate_per_100g BETWEEN $5-1.5 AND $5+1.5
		ORDER BY
		  CASE
		    WHEN $6<>'' AND lower(name)=lower($6) THEN 0
		    WHEN $6<>'' AND lower(name) LIKE '%' || lower($6) || '%' THEN 1
		    ELSE 2
		  END,
		  CASE WHEN $7<>'' AND lower(brand_name)=lower($7) THEN 0 ELSE 1 END,
		  abs(calories_per_100g-$2)/10.0 + abs(protein_per_100g-$3)/1.5 +
		  abs(fat_per_100g-$4)/1.5 + abs(carbohydrate_per_100g-$5)/1.5,
		  data_quality DESC
		LIMIT 5`, userID, input.CaloriesPer100G, input.ProteinPer100G, input.FatPer100G,
		input.CarbohydratePer100G, input.Name, input.BrandName, input.NutritionUnit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.FoodCatalogItem, 0, 5)
	for rows.Next() {
		item, err := scanCatalogItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Repository) LinkFoodCatalogBarcode(ctx context.Context, barcode, publicFoodID string) (domain.FoodCatalogItem, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return domain.FoodCatalogItem{}, err
	}
	provider, externalID, err := splitCatalogID(publicFoodID)
	if err != nil {
		return domain.FoodCatalogItem{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.FoodCatalogItem{}, err
	}
	defer tx.Rollback()
	row := tx.QueryRowContext(ctx, `
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
	if err != nil {
		return domain.FoodCatalogItem{}, err
	}
	// A barcode proposed by a user must be immediately useful to that user but
	// must not mutate a shared catalogue row before moderation. Materialise a
	// private reviewable copy of the selected candidate and point the alias at
	// that copy. This also makes the proposal visible in the admin catalogue.
	var linkedFoodID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO food_catalog (
			provider, external_id, barcode, name, brand_name, description,
			calories_per_100g, carbohydrate_per_100g, protein_per_100g, fat_per_100g,
			servings, data_quality, cached_at, owner_user_id)
		SELECT 'local',
		       'barcode-link-' || ($1::bigint)::text || '-' || source.id::text || '-' || md5($2),
		       $2, source.name, source.brand_name, source.description,
		       source.calories_per_100g, source.carbohydrate_per_100g,
		       source.protein_per_100g, source.fat_per_100g,
		       source.servings, 1, now(), $1::bigint
		FROM food_catalog AS source
		WHERE source.provider=$3 AND source.external_id=$4
		  AND (source.provider <> 'local' OR source.owner_user_id=$1::bigint)
		ON CONFLICT (owner_user_id, barcode)
		WHERE provider='local' AND owner_user_id IS NOT NULL AND barcode <> '' DO UPDATE SET
			name=EXCLUDED.name, brand_name=EXCLUDED.brand_name, description=EXCLUDED.description,
			calories_per_100g=EXCLUDED.calories_per_100g,
			carbohydrate_per_100g=EXCLUDED.carbohydrate_per_100g,
			protein_per_100g=EXCLUDED.protein_per_100g, fat_per_100g=EXCLUDED.fat_per_100g,
			servings=EXCLUDED.servings, data_quality=1, cached_at=now()
		RETURNING id`, userID, barcode, provider, externalID).Scan(&linkedFoodID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.FoodCatalogItem{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.FoodCatalogItem{}, err
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO user_food_barcode_links (user_id, barcode, food_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, barcode) DO UPDATE SET food_id=EXCLUDED.food_id, created_at=now()`,
		userID, barcode, linkedFoodID)
	if err != nil {
		return domain.FoodCatalogItem{}, err
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil || rows == 0 {
		if rowsErr != nil {
			return domain.FoodCatalogItem{}, rowsErr
		}
		return domain.FoodCatalogItem{}, domain.ErrNotFound
	}
	item, err = scanCatalogItem(tx.QueryRowContext(ctx, `
		SELECT provider, external_id, barcode, name, brand_name, description,
		       calories_per_100g, carbohydrate_per_100g, protein_per_100g, fat_per_100g,
		       servings, data_quality
		FROM food_catalog
		WHERE id=$1 AND provider='local' AND owner_user_id=$2`, linkedFoodID, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.FoodCatalogItem{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.FoodCatalogItem{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.FoodCatalogItem{}, err
	}
	return item, nil
}

func scanFoodReview(scanner interface{ Scan(...any) error }) (domain.FoodCatalogReviewItem, error) {
	var item domain.FoodCatalogReviewItem
	var provider string
	var servings []byte
	if err := scanner.Scan(
		&item.ReviewID, &item.OwnerLogin, &provider, &item.ExternalID, &item.Barcode,
		&item.Name, &item.BrandName, &item.Description, &item.CaloriesPer100G,
		&item.CarbohydratePer100G, &item.ProteinPer100G, &item.FatPer100G,
		&servings, &item.DataQuality, &item.SubmittedAt, &item.Promoted, &item.PromotedAt,
	); err != nil {
		return item, err
	}
	item.Provider = domain.FoodProvider(provider)
	item.ID = catalogPublicID(item.Provider, item.ExternalID)
	if len(servings) > 0 {
		if err := json.Unmarshal(servings, &item.Servings); err != nil {
			return item, err
		}
	}
	if item.Servings == nil {
		item.Servings = []domain.FoodServing{}
	}
	return item, nil
}

const adminFoodReviewSelect = `
	SELECT food.id, account.login, food.provider, food.external_id,
	       COALESCE(edit.barcode, food.barcode),
	       COALESCE(edit.name, food.name),
	       COALESCE(edit.brand_name, food.brand_name),
	       COALESCE(edit.description, food.description),
	       COALESCE(edit.calories_per_100g, food.calories_per_100g),
	       COALESCE(edit.carbohydrate_per_100g, food.carbohydrate_per_100g),
	       COALESCE(edit.protein_per_100g, food.protein_per_100g),
	       COALESCE(edit.fat_per_100g, food.fat_per_100g),
	       food.servings, food.data_quality,
	       to_char(food.cached_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'),
	       promotion.source_food_id IS NOT NULL,
	       COALESCE(to_char(promotion.promoted_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'), '')
	FROM food_catalog AS food
	JOIN users AS account ON account.id=food.owner_user_id
	LEFT JOIN food_catalog_review_edits AS edit ON edit.source_food_id=food.id
	LEFT JOIN food_catalog_promotions AS promotion ON promotion.source_food_id=food.id`

func reviewServings(item domain.FoodCatalogItem) []domain.FoodServing {
	unit, label := "g", "г"
	portionAmount := 0.0
	for _, serving := range item.Servings {
		if serving.ID == "ml" || serving.MetricUnit == "ml" {
			unit, label = "ml", "мл"
		}
		if serving.ID == "portion" && serving.MetricAmount > 0 {
			portionAmount = serving.MetricAmount
		}
	}
	servings := []domain.FoodServing{{
		ID: unit, Description: label, MetricAmount: 1, MetricUnit: unit,
		NumberOfUnits: 100, Measurement: "г", Calories: item.CaloriesPer100G,
		Carbohydrate: item.CarbohydratePer100G, Protein: item.ProteinPer100G, Fat: item.FatPer100G,
	}}
	servings[0].Measurement = label
	if portionAmount > 0 {
		multiplier := portionAmount / 100
		servings = append(servings, domain.FoodServing{
			ID: "portion", Description: fmt.Sprintf("порция (%s %s)", strconv.FormatFloat(portionAmount, 'f', -1, 64), label),
			MetricAmount: portionAmount, MetricUnit: unit, NumberOfUnits: 1, Measurement: "порция",
			Calories: item.CaloriesPer100G * multiplier, Carbohydrate: item.CarbohydratePer100G * multiplier,
			Protein: item.ProteinPer100G * multiplier, Fat: item.FatPer100G * multiplier,
		})
	}
	return servings
}

func (s *Repository) adminFoodCatalogItem(ctx context.Context, reviewID int64) (domain.FoodCatalogReviewItem, error) {
	item, err := scanFoodReview(s.db.QueryRowContext(ctx, adminFoodReviewSelect+`
		WHERE food.id=$1 AND food.provider='local'`, reviewID))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.FoodCatalogReviewItem{}, domain.ErrNotFound
	}
	if err == nil {
		item.Servings = reviewServings(item.FoodCatalogItem)
	}
	return item, err
}

func (s *Repository) AdminFoodCatalog(ctx context.Context, page int, pendingOnly bool) (domain.FoodCatalogReviewPage, error) {
	if err := application.RequireAdmin(ctx); err != nil {
		return domain.FoodCatalogReviewPage{}, err
	}
	rows, err := s.db.QueryContext(ctx, adminFoodReviewSelect+`
		WHERE food.provider='local'
		  AND NOT EXISTS (SELECT 1 FROM food_catalog_rejections AS rejection WHERE rejection.source_food_id=food.id)
		  AND ($1=FALSE OR promotion.source_food_id IS NULL)
		ORDER BY (promotion.source_food_id IS NULL) DESC, food.cached_at DESC, food.id DESC
		LIMIT 51 OFFSET $2`, pendingOnly, page*50)
	if err != nil {
		return domain.FoodCatalogReviewPage{}, err
	}
	defer rows.Close()
	items := make([]domain.FoodCatalogReviewItem, 0, 51)
	for rows.Next() {
		item, err := scanFoodReview(rows)
		if err != nil {
			return domain.FoodCatalogReviewPage{}, err
		}
		item.Servings = reviewServings(item.FoodCatalogItem)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return domain.FoodCatalogReviewPage{}, err
	}
	hasMore := len(items) > 50
	if hasMore {
		items = items[:50]
	}
	return domain.FoodCatalogReviewPage{Foods: items, Page: page, HasMore: hasMore}, nil
}

func (s *Repository) RejectFoodCatalogItem(ctx context.Context, reviewID int64) error {
	if err := application.RequireAdmin(ctx); err != nil {
		return err
	}
	adminID, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var promoted bool
	err = tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM food_catalog_promotions AS promotion
			WHERE promotion.source_food_id=food.id
		)
		FROM food_catalog AS food
		WHERE food.id=$1 AND food.provider='local' AND food.owner_user_id IS NOT NULL
		FOR UPDATE OF food`, reviewID).Scan(&promoted)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}
	if promoted {
		return fmt.Errorf("товар уже добавлен в общую базу: %w", domain.ErrConflict)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO food_catalog_rejections (source_food_id, rejected_by_user_id)
		VALUES ($1, $2)
		ON CONFLICT (source_food_id) DO UPDATE SET
			rejected_by_user_id=EXCLUDED.rejected_by_user_id,
			rejected_at=now()`, reviewID, adminID); err != nil {
		return fmt.Errorf("reject food review: %w", err)
	}
	return tx.Commit()
}

func (s *Repository) UpdateAdminFoodCatalog(ctx context.Context, reviewID int64, input domain.FoodCatalogReviewInput) (domain.FoodCatalogReviewItem, error) {
	if err := application.RequireAdmin(ctx); err != nil {
		return domain.FoodCatalogReviewItem{}, err
	}
	adminID, err := currentUserID(ctx)
	if err != nil {
		return domain.FoodCatalogReviewItem{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.FoodCatalogReviewItem{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		INSERT INTO food_catalog_review_edits (
			source_food_id, name, brand_name, description, barcode,
			calories_per_100g, carbohydrate_per_100g, protein_per_100g, fat_per_100g,
			updated_by_user_id, updated_at)
		SELECT food.id, $2, $3, $4, $5, $6, $7, $8, $9, $10, now()
		FROM food_catalog AS food
		WHERE food.id=$1 AND food.provider='local' AND food.owner_user_id IS NOT NULL
		ON CONFLICT (source_food_id) DO UPDATE SET
			name=EXCLUDED.name, brand_name=EXCLUDED.brand_name, description=EXCLUDED.description,
			barcode=EXCLUDED.barcode, calories_per_100g=EXCLUDED.calories_per_100g,
			carbohydrate_per_100g=EXCLUDED.carbohydrate_per_100g,
			protein_per_100g=EXCLUDED.protein_per_100g, fat_per_100g=EXCLUDED.fat_per_100g,
			updated_by_user_id=EXCLUDED.updated_by_user_id, updated_at=now()`,
		reviewID, input.Name, input.BrandName, input.Description, input.Barcode,
		input.CaloriesPer100G, input.CarbohydratePer100G, input.ProteinPer100G,
		input.FatPer100G, adminID)
	if err != nil {
		return domain.FoodCatalogReviewItem{}, foodReviewWriteError("save food review", err)
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil {
		return domain.FoodCatalogReviewItem{}, rowsErr
	} else if rows == 0 {
		return domain.FoodCatalogReviewItem{}, domain.ErrNotFound
	}
	effective := domain.FoodCatalogItem{
		Barcode: input.Barcode, Name: input.Name, BrandName: input.BrandName,
		Description: input.Description, CaloriesPer100G: input.CaloriesPer100G,
		CarbohydratePer100G: input.CarbohydratePer100G,
		ProteinPer100G:      input.ProteinPer100G, FatPer100G: input.FatPer100G,
	}
	servings, err := json.Marshal(reviewServings(effective))
	if err != nil {
		return domain.FoodCatalogReviewItem{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE food_catalog AS global SET
			barcode=$2, name=$3, brand_name=$4, description=$5,
			calories_per_100g=$6, carbohydrate_per_100g=$7,
			protein_per_100g=$8, fat_per_100g=$9, servings=$10,
			data_quality=1, cached_at=now()
		FROM food_catalog_promotions AS promotion
		WHERE promotion.source_food_id=$1 AND global.id=promotion.global_food_id`,
		reviewID, input.Barcode, input.Name, input.BrandName, input.Description,
		input.CaloriesPer100G, input.CarbohydratePer100G, input.ProteinPer100G,
		input.FatPer100G, servings); err != nil {
		return domain.FoodCatalogReviewItem{}, foodReviewWriteError("update promoted food", err)
	}
	if err := tx.Commit(); err != nil {
		return domain.FoodCatalogReviewItem{}, err
	}
	return s.adminFoodCatalogItem(ctx, reviewID)
}

func (s *Repository) PromoteFoodCatalogItem(ctx context.Context, reviewID int64) (domain.FoodCatalogItem, error) {
	if err := application.RequireAdmin(ctx); err != nil {
		return domain.FoodCatalogItem{}, err
	}
	adminID, err := currentUserID(ctx)
	if err != nil {
		return domain.FoodCatalogItem{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.FoodCatalogItem{}, err
	}
	defer tx.Rollback()
	row := tx.QueryRowContext(ctx, `
		SELECT food.provider, food.external_id,
		       COALESCE(edit.barcode, food.barcode), COALESCE(edit.name, food.name),
		       COALESCE(edit.brand_name, food.brand_name), COALESCE(edit.description, food.description),
		       COALESCE(edit.calories_per_100g, food.calories_per_100g),
		       COALESCE(edit.carbohydrate_per_100g, food.carbohydrate_per_100g),
		       COALESCE(edit.protein_per_100g, food.protein_per_100g),
		       COALESCE(edit.fat_per_100g, food.fat_per_100g),
		       servings, data_quality
		FROM food_catalog AS food
		LEFT JOIN food_catalog_review_edits AS edit ON edit.source_food_id=food.id
		WHERE food.id=$1 AND food.provider='local' AND food.owner_user_id IS NOT NULL
		  AND NOT EXISTS (
			SELECT 1 FROM food_catalog_rejections AS rejection
			WHERE rejection.source_food_id=food.id
		  )
		FOR UPDATE OF food`, reviewID)
	source, err := scanCatalogItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.FoodCatalogItem{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.FoodCatalogItem{}, err
	}
	source.Servings = reviewServings(source)
	globalExternalID := "approved-" + strconv.FormatInt(reviewID, 10)
	servings, err := json.Marshal(source.Servings)
	if err != nil {
		return domain.FoodCatalogItem{}, err
	}
	var globalDBID int64
	var savedExternalID string
	if strings.TrimSpace(source.Barcode) == "" {
		err = tx.QueryRowContext(ctx, `
			INSERT INTO food_catalog (provider, external_id, barcode, name, brand_name, description,
				calories_per_100g, carbohydrate_per_100g, protein_per_100g, fat_per_100g, servings, data_quality)
			VALUES ('community',$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,1)
			ON CONFLICT (provider, external_id) DO UPDATE SET
				name=EXCLUDED.name, brand_name=EXCLUDED.brand_name, description=EXCLUDED.description,
				calories_per_100g=EXCLUDED.calories_per_100g, carbohydrate_per_100g=EXCLUDED.carbohydrate_per_100g,
				protein_per_100g=EXCLUDED.protein_per_100g, fat_per_100g=EXCLUDED.fat_per_100g,
				servings=EXCLUDED.servings, data_quality=1, cached_at=now()
			RETURNING id, external_id`, globalExternalID, source.Barcode, source.Name, source.BrandName,
			source.Description, source.CaloriesPer100G, source.CarbohydratePer100G, source.ProteinPer100G,
			source.FatPer100G, servings).Scan(&globalDBID, &savedExternalID)
	} else {
		err = tx.QueryRowContext(ctx, `
			INSERT INTO food_catalog (provider, external_id, barcode, name, brand_name, description,
				calories_per_100g, carbohydrate_per_100g, protein_per_100g, fat_per_100g, servings, data_quality)
			VALUES ('community',$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,1)
			ON CONFLICT (provider, barcode)
			WHERE provider <> 'local' AND barcode <> '' DO UPDATE SET
				name=EXCLUDED.name, brand_name=EXCLUDED.brand_name, description=EXCLUDED.description,
				calories_per_100g=EXCLUDED.calories_per_100g, carbohydrate_per_100g=EXCLUDED.carbohydrate_per_100g,
				protein_per_100g=EXCLUDED.protein_per_100g, fat_per_100g=EXCLUDED.fat_per_100g,
				servings=EXCLUDED.servings, data_quality=1, cached_at=now()
			RETURNING id, external_id`, globalExternalID, source.Barcode, source.Name, source.BrandName,
			source.Description, source.CaloriesPer100G, source.CarbohydratePer100G, source.ProteinPer100G,
			source.FatPer100G, servings).Scan(&globalDBID, &savedExternalID)
	}
	if err != nil {
		return domain.FoodCatalogItem{}, foodReviewWriteError("promote food", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO food_catalog_promotions (source_food_id, global_food_id, reviewed_by_user_id)
		VALUES ($1,$2,$3)
		ON CONFLICT (source_food_id) DO UPDATE SET
			global_food_id=EXCLUDED.global_food_id,
			reviewed_by_user_id=EXCLUDED.reviewed_by_user_id,
			promoted_at=now()`, reviewID, globalDBID, adminID); err != nil {
		return domain.FoodCatalogItem{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.FoodCatalogItem{}, err
	}
	source.Provider = domain.FoodProviderCommunity
	source.ExternalID = savedExternalID
	source.ID = catalogPublicID(source.Provider, source.ExternalID)
	source.DataQuality = 1
	return source, nil
}
