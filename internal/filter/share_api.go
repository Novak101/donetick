package filter

import (
	"donetick.com/core/config"
	"donetick.com/core/internal/auth"
	"donetick.com/core/internal/chore"
	chRepo "donetick.com/core/internal/chore/repo"
	cRepo "donetick.com/core/internal/circle/repo"
	fRepo "donetick.com/core/internal/filter/repo"
	uRepo "donetick.com/core/internal/user/repo"
	"donetick.com/core/internal/utils"
	"github.com/gin-gonic/gin"
	limiter "github.com/ulule/limiter/v3"
)

// shareHandler holds the dependencies needed to list tasks for a shared filter.
// It's deliberately separate from the authenticated filter.Handler rather than adding
// a chore-repo dependency there, since it serves a different (unauthenticated) route
// group with its own concerns.
type shareHandler struct {
	choreRepo  *chRepo.ChoreRepository
	circleRepo *cRepo.CircleRepository
}

// getSharedFilterTasks returns the raw filter definition plus every chore visible to
// the filter's circle (respecting the same privacy rules as the authenticated app),
// and the circle's members. No server-side filter-condition matching happens here -
// the frontend's existing FilterEngine.js already does that for the authenticated
// app, so the share page reuses it instead of duplicating condition-matching logic
// in Go.
func (h *shareHandler) getSharedFilterTasks(c *gin.Context) {
	shareUser := auth.MustCurrentUser(c)
	filter, ok := auth.ShareFilter(c)
	if !ok {
		c.JSON(404, gin.H{"error": "Share link not found"})
		return
	}

	chores, err := h.choreRepo.GetChores(c, filter.CircleID, shareUser.ID, false, nil, false)
	if err != nil {
		c.JSON(500, gin.H{"error": "Error getting tasks"})
		return
	}

	members, err := h.circleRepo.GetCircleUsers(c, filter.CircleID)
	if err != nil {
		c.JSON(500, gin.H{"error": "Error getting circle members"})
		return
	}

	c.JSON(200, gin.H{
		"filter":  filter,
		"chores":  chores,
		"members": members,
	})
}

// ShareAPIs registers the unauthenticated, share-token-gated routes used to embed a
// saved filter's task list (e.g. in a Home Assistant iframe card). This mirrors the
// existing parallel-unauthenticated-group pattern in chore.APIs (the "eapi/v1/chore"
// group gated by APITokenMiddleware instead of JWT), but deliberately does NOT
// include RequirePlusMemberMiddleware - that gate only applies to the existing
// API-token-authenticated complete/update routes and has no bearing on this
// separate, purpose-built share feature.
func ShareAPIs(cfg *config.Config, choreAPI *chore.API, r *gin.Engine, rateLimiter *limiter.Limiter,
	filterRepo *fRepo.FilterRepository, circleRepo *cRepo.CircleRepository, userRepo *uRepo.UserRepository, choreRepo *chRepo.ChoreRepository) {

	h := &shareHandler{choreRepo: choreRepo, circleRepo: circleRepo}

	sharedAPI := r.Group("eapi/v1/shared/:token")
	sharedAPI.Use(
		utils.TimeoutMiddleware(cfg.Server.WriteTimeout),
		utils.RateLimitMiddleware(rateLimiter),
		auth.ShareTokenMiddleware(filterRepo, circleRepo, userRepo),
	)
	{
		sharedAPI.GET("/tasks", h.getSharedFilterTasks)
		sharedAPI.POST("/tasks", choreAPI.CreateChore)
		sharedAPI.POST("/tasks/:id/complete", choreAPI.CompleteChore)
	}
}
