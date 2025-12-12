//go:build tools
// +build tools

// Package tools pins tool dependencies for the project.
// This ensures `go mod tidy` keeps these dependencies in go.mod.
// See: https://github.com/golang/go/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module
package tools

import (
	_ "github.com/99designs/gqlgen"
)
