package main

import (
	"log"

	"github.com/gin-gonic/gin"
	"mini-escrow/internal/db"
)

func main() {
	database, err := db.NewDB()
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	log.Println("Server running on :8080")
	r.Run(":8080")
}