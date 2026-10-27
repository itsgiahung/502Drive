package app

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/itsgiahung/502Drive/internal/drive"
	"github.com/itsgiahung/502Drive/internal/state"
)

const (
	RoleOwner    = "owner"
	RoleOperator = "operator"
	planTTL      = 10 * time.Minute
)

// Principal is who is acting. Adapters derive it from authenticated transport
// data only; it is never read from request bodies.
type Principal struct {
	ID   string
	Role string
}

var Owner = Principal{ID: "owner", Role: RoleOwner}

type Service struct {
	Store    *state.Store
	Auth     *drive.Auth
	Provider func(ctx context.Context) (drive.Provider, error)
	Wake     func()
	Version  string
	BotState func() string
	// Lifecycle reports the power/network state; nil means "running, online".
	Lifecycle func() Lifecycle
	// FullReconcileEvery is the periodic full reconcile interval for realtime
	// subscriptions. nil keeps the 7-day default; 0 disables it.
	FullReconcileEvery *time.Duration
	// Throttle reports the account-wide concurrency/pacing state; nil means
	// "not wired" (tests) and reports the default worker count.
	Throttle func() ThrottleStatus
	Now      func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) wake() {
	if s.Wake != nil {
		s.Wake()
	}
}

func (s *Service) provider(ctx context.Context) (drive.Provider, error) {
	if s.Provider == nil {
		return nil, drive.ErrNotConnected
	}
	return s.Provider(ctx)
}

func requireOwner(p Principal) error {
	if p.Role != RoleOwner {
		return Fail(http.StatusForbidden, "forbidden", "Only the owner can do this.")
	}
	return nil
}

// scope is the principal filter for reads: owners may ask for everything.
func scope(p Principal, all bool) string {
	if p.Role == RoleOwner && all {
		return ""
	}
	return p.ID
}

// ---- plans and jobs ----

type PlanRequest struct {
	Source        string `json:"source"`
	DestinationID string `json:"destination_id"`
	RootName      string `json:"root_name"`
}

type Preview struct {
	ObservedFiles   int
	ObservedFolders int
	ScanComplete    bool
}

type PlanView struct {
	state.Plan
	Preview  Preview
	Warnings []string
}

func validName(s string) bool {
	if s == "" || utf8.RuneCountInString(s) > 255 || strings.TrimSpace(s) != s {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// Inspect reads metadata only (no Drive write) and stores a short-lived plan.
func (s *Service) Inspect(ctx context.Context, p Principal, key string, req PlanRequest) (PlanView, bool, error) {
	if err := state.ValidateKey(key); err != nil {
		return PlanView{}, false, Fail(http.StatusBadRequest, "invalid_request", "Idempotency-Key is required (1-128 printable characters).")
	}
	if req.RootName != "" && !validName(req.RootName) {
		return PlanView{}, false, Fail(http.StatusBadRequest, "invalid_request", "root_name must be 1-255 characters without control characters.")
	}
	ref, err := drive.Parse(req.Source)
	if err != nil {
		return PlanView{}, false, Fail(http.StatusBadRequest, "ambiguous_source", err.Error())
	}
	var dst state.Destination
	if req.DestinationID == "" {
		dst, err = s.defaultOrRoot(ctx)
	} else {
		dst, err = s.Store.GetDestination(ctx, req.DestinationID)
	}
	if err != nil {
		return PlanView{}, false, err
	}
	prov, err := s.provider(ctx)
	if err != nil {
		return PlanView{}, false, err
	}
	src, err := prov.GetFile(ctx, ref.ID, ref.Key)
	if err != nil {
		return PlanView{}, false, err
	}
	if src.Trashed {
		return PlanView{}, false, Fail(http.StatusNotFound, "source_not_found", "That item is in the trash.")
	}
	if src.IsShortcut() {
		return PlanView{}, false, Fail(http.StatusBadRequest, "unsupported_source", "Shortcuts cannot be copied directly; open the target and use its link.")
	}
	if !src.IsFolder() && !src.CanCopy {
		return PlanView{}, false, Fail(http.StatusForbidden, "copy_not_permitted", "The owner does not allow this file to be copied.")
	}
	if err = s.checkDestination(ctx, prov, dst, src.ID); err != nil {
		return PlanView{}, false, err
	}
	kind := state.KindFile
	if src.IsFolder() {
		kind = state.KindFolder
	}
	root := req.RootName
	if root == "" {
		root = src.Name
	}
	plan := state.Plan{ID: state.ID("plan"), Principal: p.ID, SourceID: src.ID, SourceName: src.Name, SourceKind: kind, SourceMime: src.MimeType, SourceSize: src.Size,
		SourceVersion: src.Fingerprint(), ResourceKey: ref.Key, DestinationID: dst.ID, DestinationFolder: dst.FolderID, DestinationRevision: dst.Revision,
		RootName: root, ExpiresAt: s.now().Add(planTTL).UTC(), Revision: 1}
	view := PlanView{Plan: plan, Warnings: []string{}}
	if kind == state.KindFile {
		view.Preview = Preview{ObservedFiles: 1, ScanComplete: true}
	} else {
		kids, next, e := prov.ListChildren(ctx, src.ID, ref.Key, "")
		if e != nil {
			return PlanView{}, false, e
		}
		for _, k := range kids {
			if k.IsFolder() {
				view.Preview.ObservedFolders++
			} else {
				view.Preview.ObservedFiles++
			}
		}
		view.Preview.ScanComplete = next == ""
		if next != "" {
			view.Warnings = append(view.Warnings, "preview_incomplete")
		}
	}
	stored, replayed, err := s.Store.PutPlanIdempotent(ctx, p.ID, key, req, plan)
	if err != nil {
		return PlanView{}, false, err
	}
	if replayed {
		view.Plan = stored
	}
	return view, replayed, nil
}

// checkDestination refuses writing into the source itself or any descendant
// and fails closed when ancestry cannot be verified.
func (s *Service) checkDestination(ctx context.Context, prov drive.Provider, dst state.Destination, sourceID string) error {
	id := dst.FolderID
	seen := map[string]bool{}
	for depth := 0; depth < 200; depth++ {
		if id == sourceID {
			return Fail(http.StatusBadRequest, "destination_inside_source", "The destination cannot be the source or inside it.")
		}
		if seen[id] {
			break
		}
		seen[id] = true
		f, err := prov.GetFile(ctx, id, dst.ResourceKey)
		if err != nil {
			if drive.KindOf(err) == drive.KindNotFound || drive.KindOf(err) == drive.KindPermission {
				return Fail(http.StatusConflict, "destination_unverifiable", "The destination folder cannot be verified; choose it again.")
			}
			return err
		}
		if depth == 0 && (!f.IsFolder() || !f.CanAddChildren) {
			return Fail(http.StatusConflict, "destination_not_writable", "The destination folder is not writable by this account.")
		}
		if len(f.Parents) == 0 {
			return nil
		}
		id = f.Parents[0]
	}
	return Fail(http.StatusConflict, "destination_unverifiable", "The destination folder ancestry could not be verified.")
}

// Submit durably admits a job from a plan. It returns only after commit.
func (s *Service) Submit(ctx context.Context, p Principal, key, planID string, revision int) (state.Result, error) {
	return s.SubmitMode(ctx, p, key, planID, revision, "new")
}

// SubmitMode admits a copy with an explicit answer for an existing copy of the
// same source to the same destination (L2):
//   - "new": always create another copy (the historical behaviour of Submit);
//   - "update": refresh the existing copy with one delta run, or copy when none exists;
//   - "": create only when no copy exists; otherwise 409 already_copied.
func (s *Service) SubmitMode(ctx context.Context, p Principal, key, planID string, revision int, mode string) (state.Result, error) {
	if err := state.ValidateKey(key); err != nil {
		return state.Result{}, Fail(http.StatusBadRequest, "invalid_request", "Idempotency-Key is required (1-128 printable characters).")
	}
	switch mode {
	case "", "update", "new":
	default:
		return state.Result{}, Fail(http.StatusBadRequest, "invalid_request", "mode must be update, new or omitted.")
	}
	guard := func(ctx context.Context, q state.Querier, plan state.Plan) error {
		var rev int
		err := q.QueryRowContext(ctx, "SELECT revision FROM destinations WHERE id=?", plan.DestinationID).Scan(&rev)
		if err != nil || rev != plan.DestinationRevision {
			return state.ErrPlanChanged
		}
		if s.Auth != nil && !s.Auth.Connected() {
			return drive.ErrReconnectRequired
		}
		if mode != "new" {
			if c, found, err := state.ExistingCopy(ctx, q, p.ID, plan); err != nil {
				return err
			} else if found {
				return state.ErrExistingCopy{Copy: c}
			}
		}
		return nil
	}
	res, err := s.Store.AdmitGuarded(ctx, p.ID, key, planID, revision, guard)
	var existing state.ErrExistingCopy
	if errors.As(err, &existing) {
		return s.answerExistingCopy(ctx, p, key, existing.Copy, mode)
	}
	if err != nil {
		return state.Result{}, err
	}
	s.wake()
	return res, nil
}

func (s *Service) Job(ctx context.Context, p Principal, id string) (state.Job, error) {
	return s.Store.GetJob(ctx, scope(p, true), id)
}

func (s *Service) Jobs(ctx context.Context, p Principal, all bool, st string, limit int, cursor int64) ([]state.Job, int64, error) {
	return s.Store.ListJobs(ctx, scope(p, all), st, limit, cursor)
}

func (s *Service) Events(ctx context.Context, p Principal, id string, after int64, limit int) ([]state.Event, error) {
	return s.Store.Events(ctx, scope(p, true), id, after, limit)
}

func (s *Service) JobAction(ctx context.Context, p Principal, key, id, action string, revision int) (state.Result, error) {
	if err := state.ValidateKey(key); err != nil {
		return state.Result{}, Fail(http.StatusBadRequest, "invalid_request", "Idempotency-Key is required (1-128 printable characters).")
	}
	owner := p.ID
	if p.Role == RoleOwner { // owners control any job, acting through its principal
		j, err := s.Store.GetJob(ctx, "", id)
		if err != nil {
			return state.Result{}, err
		}
		owner = j.Principal
	}
	res, err := s.Store.ActionAs(ctx, p.ID, owner, key, id, action, revision)
	if err != nil {
		return state.Result{}, err
	}
	s.wake()
	return res, nil
}

// answerExistingCopy applies the L2 choice to a copy that already exists.
func (s *Service) answerExistingCopy(ctx context.Context, p Principal, key string, existing state.Collection, mode string) (state.Result, error) {
	if mode == "" {
		return state.Result{}, Fail(http.StatusConflict, "already_copied",
			"This source is already copied to this destination (collection "+existing.ID+", "+existing.State+"). Use update to refresh it, or new for another copy.")
	}
	// update
	if existing.State == state.CollectionRetired {
		return state.Result{}, Fail(http.StatusConflict, "collection_retired",
			"The existing copy ("+existing.ID+") is retired. Resume it first, or use new for another copy.")
	}
	out, replayed, err := s.RunNow(ctx, p, key, existing.ID, existing.Revision)
	if err != nil {
		return state.Result{}, err
	}
	return state.Result{Job: out.Job, Replayed: replayed}, nil
}
