package auth

import (
	"fmt"
	"net/http"
	"time"

	cModel "donetick.com/core/internal/circle/model"
	cRepo "donetick.com/core/internal/circle/repo"
	fModel "donetick.com/core/internal/filter/model"
	fRepo "donetick.com/core/internal/filter/repo"
	uModel "donetick.com/core/internal/user/model"
	uRepo "donetick.com/core/internal/user/repo"
	"donetick.com/core/logging"
	"github.com/gin-gonic/gin"
)

// shareFilterKey is the gin context key the resolved shared Filter is stored under,
// so the list-tasks handler doesn't need a second DB round-trip.
const shareFilterKey = "shareFilter"

// ShareTokenMiddleware authenticates anonymous requests to a filter's share link.
// It resolves the URL's :token to a Filter, then to a per-circle placeholder "share"
// user, and sets that user as the effective identity for the request - letting the
// normal (authenticated-shaped) chore handlers run completely unmodified.
func ShareTokenMiddleware(filterRepo *fRepo.FilterRepository, circleRepo *cRepo.CircleRepository, userRepo *uRepo.UserRepository) gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		log := logging.FromContext(c)

		token := c.Param("token")
		if token == "" {
			c.JSON(http.StatusNotFound, gin.H{"error": "Share link not found"})
			c.Abort()
			return
		}

		filter, err := filterRepo.GetFilterByShareToken(c, token)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Share link not found or no longer active"})
			c.Abort()
			return
		}

		circle, err := circleRepo.GetCircleByID(c, filter.CircleID)
		if err != nil {
			log.Errorw("auth.ShareTokenMiddleware failed to load circle", "error", err, "circleId", filter.CircleID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error resolving share link"})
			c.Abort()
			return
		}

		shareUser, err := ensureShareUser(c, circleRepo, userRepo, circle)
		if err != nil {
			log.Errorw("auth.ShareTokenMiddleware failed to provision share user", "error", err, "circleId", circle.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error resolving share link"})
			c.Abort()
			return
		}

		c.Set(identityKey, shareUser)
		c.Set(shareFilterKey, filter)
		c.Next()
	})
}

// ShareFilter reads the Filter resolved by ShareTokenMiddleware out of the gin context.
func ShareFilter(c *gin.Context) (*fModel.Filter, bool) {
	filter, exists := c.Get(shareFilterKey)
	if !exists {
		return nil, false
	}
	f, ok := filter.(*fModel.Filter)
	return f, ok
}

// ensureShareUser returns the circle's placeholder share user, provisioning one on
// first use. The user is added to the circle with Manager role (not plain Member) -
// Chore.CanComplete only lets a plain member complete an unassigned chore, whereas a
// Manager/Admin can complete any non-private chore in the circle regardless of who
// it's assigned to, which is what "check off Mom's chore from the shared screen" needs.
func ensureShareUser(c *gin.Context, circleRepo *cRepo.CircleRepository, userRepo *uRepo.UserRepository, circle *cModel.Circle) (*uModel.UserDetails, error) {
	if circle.ShareUserID != nil {
		user, err := userRepo.GetUserByID(c, *circle.ShareUserID)
		if err != nil {
			return nil, err
		}
		return &uModel.UserDetails{User: *user}, nil
	}

	username := fmt.Sprintf("share-circle-%d", circle.ID)
	now := time.Now().UTC()
	created, err := userRepo.CreateUser(c, &uModel.User{
		Username:    username,
		DisplayName: "Shared Link",
		CircleID:    circle.ID,
		UserType:    uModel.UserTypeParent,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		// Most likely a concurrent first-request race on the deterministic username -
		// fall back to whichever concurrent request won and created it first.
		existing, getErr := userRepo.GetUserByUsername(c, username)
		if getErr != nil {
			return nil, err
		}
		created = &existing.User
	}

	if err := circleRepo.AddUserToCircle(c, &cModel.UserCircle{
		UserID:   created.ID,
		CircleID: circle.ID,
		Role:     cModel.UserRoleManager,
		IsActive: true,
	}); err != nil {
		return nil, err
	}

	if err := circleRepo.SetShareUserID(c, circle.ID, created.ID); err != nil {
		return nil, err
	}

	return &uModel.UserDetails{User: *created}, nil
}
