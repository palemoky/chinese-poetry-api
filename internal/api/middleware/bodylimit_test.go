package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestBodyLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(BodyLimit(8))
	router.POST("/", func(c *gin.Context) {
		if _, err := io.ReadAll(c.Request.Body); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		c.Status(http.StatusOK)
	})

	send := func(body string, chunked bool) int {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		if chunked {
			// 未声明长度，只能边读边判断
			req.ContentLength = -1
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Code
	}

	assert.Equal(t, http.StatusOK, send("12345678", false), "exactly at the limit")
	assert.Equal(t, http.StatusRequestEntityTooLarge, send("123456789", false), "declared length over the limit")
	assert.Equal(t, http.StatusOK, send("1234", true))
	assert.Equal(t, http.StatusBadRequest, send("123456789", true), "undeclared length must still be cut off")
}
