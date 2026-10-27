package app

import "context"

// Setup reuses Doctor; the core checks are ordered by what a new user must do.
type Setup struct {
	Ready  bool    `json:"ready"`
	Checks []Check `json:"checks"`
}

func (s *Service) Setup(ctx context.Context, p Principal) (Setup, error) {
	checks, err := s.Doctor(ctx, p, false)
	if err != nil {
		return Setup{}, err
	}
	out := Setup{Ready: true, Checks: []Check{}}
	for _, c := range checks {
		switch c.Name {
		case "database", "oauth_client", "account", "destination":
			out.Checks = append(out.Checks, c)
			out.Ready = out.Ready && c.Status == "passed"
		}
	}
	return out, nil
}
