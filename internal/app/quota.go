package app

import (
	"context"
	"github.com/itsgiahung/502Drive/internal/present"
)

func (s *Service) Quota(ctx context.Context, p Principal) (present.QuotaStatus, error) {
	out := present.QuotaStatus{}
	var err error
	out.Used24hBytes, err = s.Store.CopiedBytes24h(ctx, p.ID, s.now())
	if err != nil {
		return out, err
	}
	out.ResumeAfter, err = s.Store.QuotaResume(ctx, p.ID, s.now())
	return out, err
}
