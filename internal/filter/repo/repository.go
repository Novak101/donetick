package repo

import (
	"context"
	"errors"

	config "donetick.com/core/config"
	fModel "donetick.com/core/internal/filter/model"
	"donetick.com/core/internal/utils"
	"donetick.com/core/logging"
	"gorm.io/gorm"
)

type FilterRepository struct {
	db *gorm.DB
}

func NewFilterRepository(db *gorm.DB, cfg *config.Config) *FilterRepository {
	return &FilterRepository{db: db}
}

// GetCircleFilters gets all filters for a circle
func (r *FilterRepository) GetCircleFilters(ctx context.Context, circleID int) ([]*fModel.Filter, error) {
	var filters []*fModel.Filter
	if err := r.db.WithContext(ctx).Where("circle_id = ?", circleID).Order("name ASC").Find(&filters).Error; err != nil {
		return nil, err
	}
	return filters, nil
}

// GetFilterByID gets a specific filter by ID
func (r *FilterRepository) GetFilterByID(ctx context.Context, filterID int, circleID int) (*fModel.Filter, error) {
	var filter fModel.Filter
	if err := r.db.WithContext(ctx).Where("id = ? AND circle_id = ?", filterID, circleID).First(&filter).Error; err != nil {
		return nil, err
	}
	return &filter, nil
}

// CreateFilter creates a new filter
func (r *FilterRepository) CreateFilter(ctx context.Context, filter *fModel.Filter) error {
	if err := r.db.WithContext(ctx).Create(filter).Error; err != nil {
		return err
	}
	return nil
}

// UpdateFilter updates an existing filter
func (r *FilterRepository) UpdateFilter(ctx context.Context, filter *fModel.Filter, userID int, circleID int) error {
	log := logging.FromContext(ctx)

	// Check if user has permission to update this filter
	var existingFilter fModel.Filter
	if err := r.db.WithContext(ctx).Where("id = ? AND circle_id = ?", filter.ID, circleID).First(&existingFilter).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("filter not found")
		}
		log.Error("Error finding filter", "error", err)
		return err
	}

	// Only creator can update filter
	if existingFilter.CreatedBy != userID {
		return errors.New("user does not have permission to update this filter")
	}

	updates := map[string]interface{}{
		"name":        filter.Name,
		"description": filter.Description,
		"color":       filter.Color,
		"icon":        filter.Icon,
		"conditions":  filter.Conditions,
		"operator":    filter.Operator,
		"is_pinned":   filter.IsPinned,
	}

	if err := r.db.WithContext(ctx).Model(&fModel.Filter{}).Where("id = ? AND circle_id = ?", filter.ID, circleID).Updates(updates).Error; err != nil {
		return err
	}
	return nil
}

// DeleteFilter deletes a filter
func (r *FilterRepository) DeleteFilter(ctx context.Context, filterID int, userID int, circleID int) error {
	log := logging.FromContext(ctx)

	// Check if filter exists and user has permission
	var filter fModel.Filter
	if err := r.db.WithContext(ctx).Where("id = ? AND circle_id = ?", filterID, circleID).First(&filter).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("filter not found")
		}
		return err
	}

	// Check if user has permission to delete this filter
	if filter.CreatedBy != userID {
		return errors.New("user does not have permission to delete this filter")
	}

	if err := r.db.WithContext(ctx).Where("id = ? AND circle_id = ?", filterID, circleID).Delete(&fModel.Filter{}).Error; err != nil {
		log.Error("Error deleting filter", "error", err)
		return err
	}

	return nil
}

// ToggleFilterPin toggles the pin status of a filter
func (r *FilterRepository) ToggleFilterPin(ctx context.Context, filterID int, userID int, circleID int) (bool, error) {
	log := logging.FromContext(ctx)

	// Check if filter exists and user has permission
	var filter fModel.Filter
	if err := r.db.WithContext(ctx).Where("id = ? AND circle_id = ?", filterID, circleID).First(&filter).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, errors.New("filter not found")
		}
		return false, err
	}

	// Only creator can toggle pin
	if filter.CreatedBy != userID {
		return false, errors.New("user does not have permission to update this filter")
	}

	newPinStatus := !filter.IsPinned
	if err := r.db.WithContext(ctx).Model(&fModel.Filter{}).Where("id = ? AND circle_id = ?", filterID, circleID).Update("is_pinned", newPinStatus).Error; err != nil {
		log.Error("Error toggling filter pin", "error", err)
		return false, err
	}

	return newPinStatus, nil
}

// GetPinnedFilters gets all pinned filters for a circle
func (r *FilterRepository) GetPinnedFilters(ctx context.Context, circleID int) ([]*fModel.Filter, error) {
	var filters []*fModel.Filter
	if err := r.db.WithContext(ctx).Where("circle_id = ? AND is_pinned = ?", circleID, true).Order("name ASC").Find(&filters).Error; err != nil {
		return nil, err
	}
	return filters, nil
}

// GetFiltersByUsage gets filters sorted by usage count
func (r *FilterRepository) GetFiltersByUsage(ctx context.Context, circleID int) ([]*fModel.Filter, error) {
	var filters []*fModel.Filter
	if err := r.db.WithContext(ctx).Where("circle_id = ?", circleID).Order("usage_count DESC, name ASC").Find(&filters).Error; err != nil {
		return nil, err
	}
	return filters, nil
}

// GetFilterByShareToken gets a filter by its share token, only if sharing is enabled
func (r *FilterRepository) GetFilterByShareToken(ctx context.Context, token string) (*fModel.Filter, error) {
	var filter fModel.Filter
	if err := r.db.WithContext(ctx).Where("share_token = ? AND share_enabled = ?", token, true).First(&filter).Error; err != nil {
		return nil, err
	}
	return &filter, nil
}

// EnableShare turns on sharing for a filter, generating a token if one doesn't exist yet
func (r *FilterRepository) EnableShare(ctx context.Context, filterID int, userID int, circleID int) (*fModel.Filter, error) {
	filter, err := r.checkFilterOwnership(ctx, filterID, userID, circleID)
	if err != nil {
		return nil, err
	}

	updates := map[string]interface{}{"share_enabled": true}
	if filter.ShareToken == nil {
		updates["share_token"] = utils.GenerateShareToken(ctx)
	}

	if err := r.db.WithContext(ctx).Model(&fModel.Filter{}).Where("id = ? AND circle_id = ?", filterID, circleID).Updates(updates).Error; err != nil {
		return nil, err
	}

	return r.GetFilterByID(ctx, filterID, circleID)
}

// DisableShare turns off sharing for a filter, keeping the existing token so re-enabling reuses the same URL
func (r *FilterRepository) DisableShare(ctx context.Context, filterID int, userID int, circleID int) (*fModel.Filter, error) {
	if _, err := r.checkFilterOwnership(ctx, filterID, userID, circleID); err != nil {
		return nil, err
	}

	if err := r.db.WithContext(ctx).Model(&fModel.Filter{}).Where("id = ? AND circle_id = ?", filterID, circleID).Update("share_enabled", false).Error; err != nil {
		return nil, err
	}
	return r.GetFilterByID(ctx, filterID, circleID)
}

// RegenerateShareToken issues a fresh share token for a filter, invalidating the old URL
func (r *FilterRepository) RegenerateShareToken(ctx context.Context, filterID int, userID int, circleID int) (*fModel.Filter, error) {
	if _, err := r.checkFilterOwnership(ctx, filterID, userID, circleID); err != nil {
		return nil, err
	}

	token := utils.GenerateShareToken(ctx)
	if err := r.db.WithContext(ctx).Model(&fModel.Filter{}).Where("id = ? AND circle_id = ?", filterID, circleID).Update("share_token", token).Error; err != nil {
		return nil, err
	}
	return r.GetFilterByID(ctx, filterID, circleID)
}

// checkFilterOwnership loads a filter and confirms userID is its creator, matching the
// permission convention used by UpdateFilter/DeleteFilter/ToggleFilterPin.
func (r *FilterRepository) checkFilterOwnership(ctx context.Context, filterID int, userID int, circleID int) (*fModel.Filter, error) {
	var filter fModel.Filter
	if err := r.db.WithContext(ctx).Where("id = ? AND circle_id = ?", filterID, circleID).First(&filter).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("filter not found")
		}
		return nil, err
	}
	if filter.CreatedBy != userID {
		return nil, errors.New("user does not have permission to update this filter")
	}
	return &filter, nil
}

// FilterNameExists checks if a filter name already exists (case-insensitive)
func (r *FilterRepository) FilterNameExists(ctx context.Context, name string, circleID int, excludeFilterID *int) (bool, error) {
	var count int64
	query := r.db.WithContext(ctx).Model(&fModel.Filter{}).Where("LOWER(name) = LOWER(?) AND circle_id = ?", name, circleID)

	if excludeFilterID != nil {
		query = query.Where("id != ?", *excludeFilterID)
	}

	if err := query.Count(&count).Error; err != nil {
		return false, err
	}

	return count > 0, nil
}
