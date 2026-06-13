package main

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func (s *server) agentAuth(c *gin.Context) {
	token := c.GetHeader("X-Agent-Token")
	if token == "" {
		auth := c.GetHeader("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			token = strings.TrimPrefix(auth, "Bearer ")
		}
	}
	if token == "" || token != s.cfg.AgentToken {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid agent token"})
		return
	}
	c.Next()
}

func createdBy(c *gin.Context) string {
	if user := strings.TrimSpace(c.GetHeader("X-User")); user != "" {
		return user
	}
	return "api"
}
