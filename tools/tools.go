//go:build tools
// +build tools

package tools

// This file is used to track tool dependencies for the project.
// It ensures that 'go mod tidy' doesn't remove the tools from go.mod.
// See: https://github.com/golang/go/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module

import (
	_ "github.com/99designs/gqlgen"
)
