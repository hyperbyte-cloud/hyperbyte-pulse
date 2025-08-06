package main

import (
	"log"

	"hyperbyte-proc-monitor/internal/app"
)

func main() {
	application, err := app.NewApp()
	if err != nil {
		log.Fatalf("Failed to create application: %v", err)
	}

	if err := application.Run(); err != nil {
		log.Fatalf("Application error: %v", err)
	}
}
