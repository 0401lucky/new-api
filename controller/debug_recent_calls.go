package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func GetRecentCalls(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	limit := 100
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 100 {
		limit = 100
	}

	var beforeID uint64
	if v := c.Query("before_id"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			beforeID = n
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  service.RecentCallsCache().List(limit, beforeID),
		"limit": limit,
	})
}

func GetRecentCallByID(c *gin.Context) {
	c.Header("Cache-Control", "no-store")

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid id",
		})
		return
	}

	rec, ok := service.RecentCallsCache().Get(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": rec,
	})
}
