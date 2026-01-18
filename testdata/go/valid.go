package main

import "fmt"

// Server represents the main application server.
type Server struct {
	Name string
	Port int
}

// NewServer creates a new Server instance.
func NewServer(name string, port int) *Server {
	return &Server{
		Name: name,
		Port: port,
	}
}

// Start starts the server.
func (s *Server) Start() error {
	fmt.Printf("Starting server %s on port %d\n", s.Name, s.Port)
	return nil
}

// Config holds application configuration.
type Config struct {
	Debug   bool
	Timeout int
}

const (
	// DefaultPort is the default server port.
	DefaultPort = 8080
)

var (
	// Version is the application version.
	Version = "1.0.0"
)

func main() {
	srv := NewServer("main", DefaultPort)
	srv.Start()
}
