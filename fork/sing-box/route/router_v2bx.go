package route

import "context"

func (r *Router) GetCtx() context.Context {
	return r.ctx
}
