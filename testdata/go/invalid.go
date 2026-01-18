package main

// This file contains intentional syntax errors for testing.

func broken( {
	return
}

type Incomplete struct {
	Name string
	// Missing closing brace
