package servers

import (
	"context"
	"sync"
)

type App interface {
	Start(wg *sync.WaitGroup, ctx context.Context) error
}
