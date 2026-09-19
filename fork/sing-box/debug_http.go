package box

import (
	"github.com/sagernet/sing-box/option"
)

// applyDebugListenOption is stubbed out in the agent build: the debug HTTP
// server (pprof/chi) is never used and only bloats the binary.
func applyDebugListenOption(options option.DebugOptions) {}
