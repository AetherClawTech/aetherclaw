package multiagent

import "context"

type blackboardContextKey struct{}

// WithBlackboard attaches a blackboard to the context for tool execution.
func WithBlackboard(ctx context.Context, board *Blackboard) context.Context {
	if ctx == nil || board == nil {
		return ctx
	}
	return context.WithValue(ctx, blackboardContextKey{}, board)
}

// BlackboardFromContext retrieves a blackboard attached to the context.
func BlackboardFromContext(ctx context.Context) *Blackboard {
	if ctx == nil {
		return nil
	}
	if v := ctx.Value(blackboardContextKey{}); v != nil {
		if board, ok := v.(*Blackboard); ok {
			return board
		}
	}
	return nil
}
