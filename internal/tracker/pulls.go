package tracker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/Tencent/LoopForge/internal/domain"
)

const (
	mrDeliverySyncInterval = 5 * time.Minute
	mrDeliveryStaleAfter   = 5 * time.Minute
	prStateTTL             = 2 * time.Minute
)

// EnsureAndMergeBranchMR finds or creates a pull request source→target and
// merges it with a merge commit; the source branch is kept. An empty target
// only merges an already-open pull request for the source (no create). Same
// source and target is a no-op.
func (s *Service) EnsureAndMergeBranchMR(repoPath, sourceBranch, targetBranch, title, description string) (string, error) {
	repoPath = strings.TrimSpace(repoPath)
	src := strings.TrimSpace(sourceBranch)
	tgt := strings.TrimSpace(targetBranch)
	if src == "" {
		return "", errors.New("source branch is required")
	}
	if tgt != "" && src == tgt {
		return "", nil
	}
	if repoPath == "" {
		return "", errors.New("repository, source branch and target branch are required")
	}
	provider, err := s.providerForRepoPath(repoPath)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pr, found, err := findOpenPull(ctx, provider, repoPath, src, tgt)
	if err != nil {
		return "", err
	}
	if !found {
		if tgt == "" {
			return "", ErrNoOpenMR
		}
		createTitle := strings.TrimSpace(title)
		if createTitle == "" {
			createTitle = fmt.Sprintf("LoopForge: %s → %s", src, tgt)
		}
		createBody := strings.TrimSpace(description)
		if createBody == "" {
			createBody = fmt.Sprintf("Scheduled SOP merges `%s` into `%s`.", src, tgt)
		}
		_, createErr := provider.CreatePullRequest(ctx, repoPath, PullRequestInput{Title: createTitle, Body: createBody, SourceBranch: src, TargetBranch: tgt})
		// Search again even when create failed: another instance may have won
		// the create race and left a reusable pull request open.
		pr, found, err = findOpenPull(ctx, provider, repoPath, src, tgt)
		if err != nil {
			return "", err
		}
		if !found {
			if createErr != nil {
				return "", fmt.Errorf("create pull request %s → %s: %w", src, tgt, createErr)
			}
			return "", fmt.Errorf("create pull request %s → %s returned no pull request", src, tgt)
		}
	}
	if err := provider.MergePullRequest(ctx, repoPath, pr.Number, title); err != nil {
		return "", fmt.Errorf("merge pull request #%d: %w", pr.Number, err)
	}
	return "#" + strconv.Itoa(pr.Number), nil
}

func findOpenPull(ctx context.Context, provider Provider, repoPath, src, tgt string) (PullRequest, bool, error) {
	items, err := provider.ListPullRequests(ctx, repoPath, PullRequestQuery{SourceBranch: src, TargetBranch: tgt, State: "opened", Limit: 5})
	if err != nil {
		return PullRequest{}, false, fmt.Errorf("search pull requests for %s: %w", src, err)
	}
	for _, it := range items {
		if it.State == "opened" && (tgt == "" || domain.SameGitBranch(it.TargetBranch, tgt)) {
			return it, true, nil
		}
	}
	return PullRequest{}, false, nil
}

// FindBranchMR returns the delivery pull request whose source is
// sourceBranch: the newest non-closed one, else the newest closed one. It is
// the authoritative PR of a run repository; links in artifacts are only a
// fallback.
func (s *Service) FindBranchMR(repoPath, sourceBranch string) (domain.DeliveryMR, bool, error) {
	repoPath = strings.TrimSpace(repoPath)
	src := strings.TrimSpace(sourceBranch)
	if repoPath == "" || src == "" {
		return domain.DeliveryMR{}, false, nil
	}
	provider, err := s.providerForRepoPath(repoPath)
	if err != nil {
		return domain.DeliveryMR{}, false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	items, err := provider.ListPullRequests(ctx, repoPath, PullRequestQuery{SourceBranch: src, Limit: 20})
	if err != nil {
		return domain.DeliveryMR{}, false, fmt.Errorf("search pull requests for %s: %w", src, err)
	}
	item, ok := pickBranchPull(items, src)
	if !ok {
		return domain.DeliveryMR{}, false, nil
	}
	url := strings.TrimSpace(item.URL)
	if _, _, parsed, ok := domain.ParseForgeMR(url); ok {
		url = parsed
	} else {
		url = domain.ForgeMRURL(repoPath, item.Number)
	}
	return domain.DeliveryMR{
		Project:      repoPath,
		IID:          item.Number,
		URL:          url,
		TargetBranch: item.TargetBranch,
		ForgeState:   item.State,
	}, true, nil
}

func pickBranchPull(items []PullRequest, sourceBranch string) (PullRequest, bool) {
	var best PullRequest
	found, bestClosed := false, false
	for _, item := range items {
		if s := strings.TrimSpace(item.SourceBranch); s != "" && !domain.SameGitBranch(s, sourceBranch) {
			continue
		}
		closed := item.State == "closed"
		switch {
		case !found:
		case bestClosed && !closed:
		case bestClosed == closed && item.Number > best.Number:
		default:
			continue
		}
		best, found, bestClosed = item, true, closed
	}
	return best, found
}

// MRDeliveryStore is the persistence surface used by the pull request state
// background sync.
type MRDeliveryStore interface {
	DiscoverMRDeliveryStatesFromArtifacts() error
	MRDeliveryStatesNeedingRefresh(staleBefore time.Time) ([]domain.MRDeliveryState, error)
	UpdateMRDeliveryState(project string, iid int, state string, targetBranch string, checkedAt time.Time) error
	ReconcileMergedMRDeliveries() error
	ReconcileCompletedDeliveries() error
	ReconcileDeliveryMRStates() error
	Repositories() []domain.Repository
	EnsureMRDeliveryState(project string, iid int, url, issueID string) error
}

// StartMRStateSync launches a background loop that discovers pull request
// links in run artifacts and refreshes their merge state from the forge.
func (s *Service) StartMRStateSync(store MRDeliveryStore) {
	if s == nil || store == nil {
		return
	}
	s.autoMu.Lock()
	if s.mrTicker != nil {
		s.autoMu.Unlock()
		return
	}
	s.mrTicker = time.NewTicker(mrDeliverySyncInterval)
	s.mrStopCh = make(chan struct{})
	ticker, stopCh := s.mrTicker, s.mrStopCh
	s.autoMu.Unlock()

	go func() {
		s.syncMRDeliveryStates(store)
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				s.syncMRDeliveryStates(store)
			}
		}
	}()
}

// StopMRStateSync halts the pull request background sync.
func (s *Service) StopMRStateSync() {
	s.autoMu.Lock()
	defer s.autoMu.Unlock()
	if s.mrTicker != nil {
		s.mrTicker.Stop()
		s.mrTicker = nil
	}
	if s.mrStopCh != nil {
		close(s.mrStopCh)
		s.mrStopCh = nil
	}
}

// SyncMRDeliveryStatesOnce runs discovery and refresh immediately.
func (s *Service) SyncMRDeliveryStatesOnce(store MRDeliveryStore) {
	if s == nil || store == nil {
		return
	}
	s.syncMRDeliveryStates(store)
}

func (s *Service) syncMRDeliveryStates(store MRDeliveryStore) {
	s.deliveryReconcileOnce.Do(func() {
		if err := store.ReconcileMergedMRDeliveries(); err != nil {
			log.Printf("pr-delivery reconcile: %v", err)
		}
		if err := store.ReconcileCompletedDeliveries(); err != nil {
			log.Printf("delivery reconcile: %v", err)
		}
	})
	if err := store.ReconcileDeliveryMRStates(); err != nil {
		log.Printf("delivery pr-state reconcile: %v", err)
	}
	if err := store.DiscoverMRDeliveryStatesFromArtifacts(); err != nil {
		log.Printf("pr-delivery discover: %v", err)
		return
	}
	s.discoverPromotePulls(store)
	pending, err := store.MRDeliveryStatesNeedingRefresh(time.Now().UTC().Add(-mrDeliveryStaleAfter))
	if err != nil {
		log.Printf("pr-delivery list: %v", err)
		return
	}
	now := time.Now().UTC()
	updated := 0
	for _, row := range pending {
		state, target := s.lookupPullState(row.MRProject, row.MRIID)
		if err := store.UpdateMRDeliveryState(row.MRProject, row.MRIID, state, target, now); err != nil {
			log.Printf("pr-delivery update %s#%d: %v", row.MRProject, row.MRIID, err)
			continue
		}
		updated++
	}
	if updated > 0 {
		log.Printf("pr-delivery sync: refreshed %d/%d pull request states", updated, len(pending))
	}
}

type prCacheEntry struct {
	state, targetBranch string
	at                  time.Time
}

func (s *Service) lookupPullState(repoPath string, number int) (state, targetBranch string) {
	key := repoPath + "#" + strconv.Itoa(number)
	if raw, ok := s.prCache.Load(key); ok {
		if e, ok := raw.(prCacheEntry); ok && time.Since(e.at) < prStateTTL {
			return e.state, e.targetBranch
		}
	}
	state = "unknown"
	if provider, err := s.providerForRepoPath(repoPath); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if pr, err := provider.GetPullRequest(ctx, repoPath, number); err == nil && pr.State != "" {
			state, targetBranch = pr.State, pr.TargetBranch
		}
		cancel()
	}
	s.prCache.Store(key, prCacheEntry{state: state, targetBranch: targetBranch, at: time.Now()})
	return state, targetBranch
}

// discoverPromotePulls records integrate→default promotion pull requests so
// baseline promotions show up in delivery tracking.
func (s *Service) discoverPromotePulls(store MRDeliveryStore) {
	now := time.Now().UTC()
	for _, repo := range store.Repositories() {
		repoPath := strings.TrimSpace(repo.RepoPath)
		def := strings.TrimSpace(repo.DefaultBranch)
		integ := domain.NormalizeIntegrateBranch(repo.IntegrateBranch)
		if repoPath == "" || def == "" || integ == "" || domain.SameGitBranch(integ, def) {
			continue
		}
		provider, err := s.providerForProject(repo.ProjectID)
		if err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		items, err := provider.ListPullRequests(ctx, repoPath, PullRequestQuery{SourceBranch: integ, TargetBranch: def, Limit: 8})
		cancel()
		if err != nil {
			log.Printf("pr-delivery discover promote %s: %v", repoPath, err)
			continue
		}
		for _, pr := range items {
			if pr.Number <= 0 || pr.State == "closed" {
				continue
			}
			if err := store.EnsureMRDeliveryState(repoPath, pr.Number, domain.ForgeMRURL(repoPath, pr.Number), domain.BaselinePromoteIssueID); err != nil {
				log.Printf("pr-delivery ensure promote %s#%d: %v", repoPath, pr.Number, err)
				continue
			}
			target := strings.TrimSpace(pr.TargetBranch)
			if target == "" {
				target = def
			}
			checked := now
			if pr.State == "merged" && !pr.MergedAt.IsZero() {
				checked = pr.MergedAt
			}
			if err := store.UpdateMRDeliveryState(repoPath, pr.Number, pr.State, target, checked); err != nil {
				log.Printf("pr-delivery update promote %s#%d: %v", repoPath, pr.Number, err)
			}
		}
	}
}
