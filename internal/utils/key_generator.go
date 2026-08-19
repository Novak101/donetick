package utils

import (
	"context"
	"encoding/base64"

	crand "crypto/rand"

	"donetick.com/core/logging"
	"github.com/gin-gonic/gin"
)

func GenerateInviteCode(c *gin.Context) string {
	logger := logging.FromContext(c)
	// Define the length of the token (in bytes). For example, 32 bytes will result in a 44-character base64-encoded token.
	tokenLength := 12

	// Generate a random byte slice.
	tokenBytes := make([]byte, tokenLength)
	_, err := crand.Read(tokenBytes)
	if err != nil {
		logger.Errorw("utility.GenerateEmailResetToken failed to generate random bytes", "err", err)
	}

	// Encode the byte slice to a base64 string.
	token := base64.URLEncoding.EncodeToString(tokenBytes)

	return token
}

// GenerateShareToken generates a random token for filter share links. Uses more
// entropy than GenerateInviteCode since this token grants write access (task
// completion and creation) to anyone holding it, not just read access to an invite.
func GenerateShareToken(ctx context.Context) string {
	logger := logging.FromContext(ctx)
	tokenLength := 24

	tokenBytes := make([]byte, tokenLength)
	_, err := crand.Read(tokenBytes)
	if err != nil {
		logger.Errorw("utility.GenerateShareToken failed to generate random bytes", "err", err)
	}

	return base64.URLEncoding.EncodeToString(tokenBytes)
}
