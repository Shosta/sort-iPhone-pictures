package remote

import "context"

// Activity is a sign of life from a slow step, so the UI can show it is
// working (for instance how many files of the NAS library were read so far).
type Activity struct {
	Files int
}

type activityKey struct{}

// WithActivity returns a context whose slow steps report their activity.
func WithActivity(ctx context.Context, fn func(Activity)) context.Context {
	return context.WithValue(ctx, activityKey{}, fn)
}

func notify(ctx context.Context, a Activity) {
	if fn, ok := ctx.Value(activityKey{}).(func(Activity)); ok {
		fn(a)
	}
}

// ToolInfo is implemented by transports that can say which rsync they use.
type ToolInfo interface {
	Tool(ctx context.Context) (Tool, error)
}

// Legacy is true for rsync builds that lack modern options (openrsync, < 3.1).
func (t Tool) Legacy() bool { return t.OpenRsync || !t.Info2 }
