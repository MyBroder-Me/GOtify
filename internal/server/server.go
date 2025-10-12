package server

import (
	"customspotify/internal/handlers"

	"github.com/gin-gonic/gin"
)

type Server struct {
	engine *gin.Engine
	root   string
}

func New(root string) *Server {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	h := handlers.NewFileHandler(root)
	r.GET("/stream/:folder/*file", h.Serve)

	return &Server{engine: r, root: root}
}

func (s *Server) Run(addr string) {
	if err := s.engine.Run(addr); err != nil {
		panic(err)
	}
}
