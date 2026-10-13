package gateway

import (
	"sync/atomic"
)

var staleMidReadRuns atomic.Int32
