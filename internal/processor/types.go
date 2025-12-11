package processor

import (
	"github.com/palemoky/chinese-poetry-api/internal/loader"
)

type PoemWork struct {
	loader.PoemWithMeta
	ID int64
}
