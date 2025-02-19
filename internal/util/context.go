package util

import "context"

type contextKey string

const (
	clientContextKey contextKey = "client"
)

func NewContext(ctx context.Context, client string) context.Context {
	return context.WithValue(ctx, clientContextKey, client)
}

// FromContext returns the User value stored in ctx, if any.
func GetClientFromContext(ctx context.Context) string {
	if client := ctx.Value(clientContextKey); client != nil {
		//nolint:errcheck // ignore error
		return client.(string)
	}

	return ""
}
