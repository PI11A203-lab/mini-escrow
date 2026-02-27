package main

import (
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"mini-escrow/internal/db"
	"mini-escrow/internal/order"
	userSvc "mini-escrow/internal/user"
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
	userService := userSvc.NewService(database)

	// POST /users
	r.POST("/users", func(c *gin.Context) {
		var req struct {
			Name string `json:"name"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}

		u, err := userService.CreateUser(req.Name)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"id":   u.ID,
			"name": u.Name,
		})
	})

	// POST /users/:id/deposit
	r.POST("/users/:id/deposit", func(c *gin.Context) {
		idStr := c.Param("id")
		userID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
			return
		}

		var req struct {
			Amount int64 `json:"amount"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.Amount <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid amount"})
			return
		}

		if err := userService.Deposit(userID, req.Amount); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.Status(http.StatusOK)
	})

	// GET /users/:id/balance
	r.GET("/users/:id/balance", func(c *gin.Context) {
		idStr := c.Param("id")
		userID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
			return
		}

		balance, err := userService.GetBalance(userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"user_id": userID, "balance": balance})
	})

	// POST /orders
	r.POST("/orders", func(c *gin.Context) {
		var req struct {
			BuyerID  int64 `json:"buyer_id"`
			SellerID int64 `json:"seller_id"`
			Amount   int64 `json:"amount"`
		}

		if err := c.ShouldBindJSON(&req); err != nil || req.BuyerID <= 0 || req.SellerID <= 0 || req.Amount <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
			return
		}

		o, err := orderService.CreateOrder(req.BuyerID, req.SellerID, req.Amount)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"id":        o.ID,
			"buyer_id":  o.BuyerID,
			"seller_id": o.SellerID,
			"amount":    o.Amount,
			"status":    o.Status,
		})
	})

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

	// POST /orders/:id/confirm
	r.POST("/orders/:id/confirm", func(c *gin.Context) {
		idStr := c.Param("id")
		orderID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid order id"})
			return
		}

		const platformID int64 = 1

		if err := orderService.ConfirmOrder(orderID, platformID); err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}

		c.Status(http.StatusOK)
	})

	// POST /orders/:id/cancel
	r.POST("/orders/:id/cancel", func(c *gin.Context) {
		idStr := c.Param("id")
		orderID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid order id"})
			return
		}

		const platformID int64 = 1

		if err := orderService.CancelOrder(orderID, platformID); err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}

		c.Status(http.StatusOK)
	})

	log.Println("Server running on :8080")
	r.Run(":8080")
}