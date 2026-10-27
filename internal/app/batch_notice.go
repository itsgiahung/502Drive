package app

import "context"

func (s *Service) SealBatch(ctx context.Context, p Principal, id string) error {
	if _, err := s.Batch(ctx, p, id); err != nil {
		return err
	}
	return s.Store.SealBatchNotice(ctx, p.ID, id, s.now())
}
