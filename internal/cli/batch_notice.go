package cli

import (
	"context"
	"errors"
	"fmt"
	"net/url"
)

func (a *App) sealBatch(ctx context.Context, id string) {
	_, err := a.call(ctx, "POST", "/v1/batches/"+url.PathEscape(id)+"/seal", nil, nil, "")
	var ae *APIError
	if err != nil && !(errors.As(err, &ae) && ae.Status == 404) {
		fmt.Fprintln(a.errw(), "batch notification:", err)
	}
}
