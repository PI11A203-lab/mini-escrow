package main

import (
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"mini-escrow/internal/db"
	"mini-escrow/internal/order"
)

func main() {
	database, err := db.NewDB()
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	orderService := order.NewService(database)

	// POST /orders/:id/fund
	r.POST("/orders/:id/fund", func(c *gin.Context) {
		idStr := c.Param("id")
		orderID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid order id"})
			return
		}

		// TODO: 실제 플랫폼 ID는 인증/컨텍스트에서 가져오도록 변경
		const platformID int64 = 1

		err = orderService.FundOrder(orderID, platformID)
		if err != nil {
			// 도메인/비즈니스 에러는 400/409로 매핑 (단순 구현)
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}

		c.Status(http.StatusOK)
	})

	log.Println("Server running on :8080")
	r.Run(":8080")
}