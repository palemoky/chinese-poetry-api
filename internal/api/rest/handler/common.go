package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// parseID extracts and validates an int64 ID from a URL parameter.
// Returns the ID and true if successful, or sends an error response and returns false.
func parseID(c *gin.Context, param, entityName string) (int64, bool) {
	idStr := c.Param(param)
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid " + entityName + " ID"})
		return 0, false
	}
	return id, true
}

// respondError sends a JSON error response with the given status code and message.
func respondError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"error": message})
}

// respondOK sends a JSON success response with the given data.
func respondOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"data": data})
}
