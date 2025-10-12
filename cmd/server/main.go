package main

import (
	"customspotify/internal/server"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	// basic example with audio in the assets/audio/ folder
	s := server.New("assets/audio")
	s.Run(":" + port)
}
