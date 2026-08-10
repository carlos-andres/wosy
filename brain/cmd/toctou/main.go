package main

import (
	"fmt"
	"path/filepath"
	"sync"
	"wosy.local/brain/internal/store"
)

func main() {
	s, err := store.Open(filepath.Join("/tmp", "toctou_test.db"))
	if err != nil {
		panic(err)
	}
	defer s.Close()

	var wg sync.WaitGroup
	errors := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		errors[0] = s.Put(map[string]any{"id": "x", "type": "umbrella", "status": "active", "updated": "2026-05-25"})
	}()
	go func() {
		defer wg.Done()
		errors[1] = s.Put(map[string]any{"id": "x", "type": "project", "status": "active", "updated": "2026-05-25"})
	}()
	wg.Wait()
	fmt.Printf("goroutine 0 error: %v\n", errors[0])
	fmt.Printf("goroutine 1 error: %v\n", errors[1])
	got, _ := s.Get("x")
	fmt.Printf("final type in db: %v\n", got["type"])
}
