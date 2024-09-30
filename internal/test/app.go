package test

import (
	"context"
	"sync"
)

type App interface {
	Start() error
	Scan(wg *sync.WaitGroup, ctx context.Context)
}
