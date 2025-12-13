package graph

import (
	"github.com/palemoky/chinese-poetry-api/internal/database"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require here.

type Resolver struct {
	DB   *database.DB
	Repo *database.Repository
}

func NewResolver(db *database.DB, repo *database.Repository) *Resolver {
	return &Resolver{
		DB:   db,
		Repo: repo,
	}
}
