package sourcegraph

import (
	"encoding/base64"
	"strings"

	"sourcegraph.com/sourcegraph/go-diff/diff"
	"sourcegraph.com/sourcegraph/go-sourcegraph/router"
)

// DeltasService interacts with the delta-related endpoints of the
// Sourcegraph API. A delta is all of the changes between two commits,
// possibly from two different repositories. It includes the usual
// file diffs as well as definition-level diffs, affected author/repo
// impact information, etc.
type DeltasService interface {
	// Get fetches a summary of a delta.
	Get(ds DeltaSpec, opt *DeltaGetOptions) (*Delta, Response, error)

	// ListDefs lists definitions added/changed/deleted in a delta.
	ListDefs(ds DeltaSpec, opt *DeltaListDefsOptions) (*DeltaDefs, Response, error)

	// ListDependencies lists dependencies added/changed/deleted in a
	// delta.
	ListDependencies(ds DeltaSpec, opt *DeltaListDependenciesOptions) (*DeltaDependencies, Response, error)

	// ListFiles fetches the file diff for a delta.
	ListFiles(ds DeltaSpec, opt *DeltaListFilesOptions) (*DeltaFiles, Response, error)

	// ListAffectedAuthors lists authors whose code is added/deleted/changed
	// in a delta.
	ListAffectedAuthors(ds DeltaSpec, opt *DeltaListAffectedAuthorsOptions) ([]*DeltaAffectedPerson, Response, error)

	// ListAffectedClients lists clients whose code is affected by a delta.
	ListAffectedClients(ds DeltaSpec, opt *DeltaListAffectedClientsOptions) ([]*DeltaAffectedPerson, Response, error)

	// ListAffectedDependents lists dependent repositories that are affected
	// by a delta.
	ListAffectedDependents(ds DeltaSpec, opt *DeltaListAffectedDependentsOptions) ([]*DeltaAffectedRepo, Response, error)

	// ListReviewers lists people who are reviewing or are suggested
	// reviewers for this delta.
	ListReviewers(ds DeltaSpec, opt *DeltaListReviewersOptions) ([]*DeltaReviewer, Response, error)

	// ListIncoming lists deltas that affect the given repo.
	ListIncoming(rr RepoRevSpec, opt *DeltaListIncomingOptions) ([]*Delta, Response, error)
}

// deltasService implements DeltasService.
type deltasService struct {
	client *Client
}

var _ DeltasService = &deltasService{}

// A DeltaSpec specifies a delta.
type DeltaSpec struct {
	Base RepoRevSpec
	Head RepoRevSpec
}

// RouteVars returns the route variables for generating URLs to the
// delta specified by this DeltaSpec.
func (s DeltaSpec) RouteVars() map[string]string {
	m := s.Base.RouteVars()

	if s.Base.RepoSpec == s.Head.RepoSpec {
		m["DeltaHeadRev"] = s.Head.RevPathComponent()
	} else {
		m["DeltaHeadRev"] = encodeCrossRepoRevSpecForDeltaHeadRev(s.Head)
	}
	return m
}

func encodeCrossRepoRevSpecForDeltaHeadRev(rr RepoRevSpec) string {
	return base64.URLEncoding.EncodeToString([]byte(rr.RepoSpec.PathComponent())) + ":" + rr.RevPathComponent()
}

// UnmarshalDeltaSpec marshals a map containing route variables
// generated by (*DeltaSpec).RouteVars() and returns the
// equivalent DeltaSpec struct.
func UnmarshalDeltaSpec(routeVars map[string]string) (DeltaSpec, error) {
	s := DeltaSpec{}

	rr, err := UnmarshalRepoRevSpec(routeVars)
	if err != nil {
		return DeltaSpec{}, err
	}
	s.Base = rr

	dhr := routeVars["DeltaHeadRev"]
	if i := strings.Index(dhr, ":"); i != -1 {
		// base repo != head repo
		repoPCB64, revPC := dhr[:i], dhr[i+1:]

		repoPC, err := base64.URLEncoding.DecodeString(repoPCB64)
		if err != nil {
			return DeltaSpec{}, err
		}

		rr, err := UnmarshalRepoRevSpec(map[string]string{"RepoSpec": string(repoPC), "Rev": revPC})
		if err != nil {
			return DeltaSpec{}, err
		}

		s.Head = rr
	} else {
		rr, err := UnmarshalRepoRevSpec(map[string]string{"RepoSpec": routeVars["RepoSpec"], "Rev": dhr})
		if err != nil {
			return DeltaSpec{}, err
		}

		s.Head = rr
	}
	return s, nil
}

// Delta represents the difference between two commits (possibly in 2
// separate repositories).
type Delta struct {
	Base, Head             RepoRevSpec // base/head repo and revspec
	BaseCommit, HeadCommit *Commit     // base/head commits
	BaseRepo, HeadRepo     *Repository // base/head repositories
	BaseBuild, HeadBuild   *Build      // base/head builds (or nil)

	// add summary fields
}

func (d *Delta) DeltaSpec() DeltaSpec {
	return DeltaSpec{
		Base: d.Base,
		Head: d.Head,
	}
}

// DeltaGetOptions specifies options for getting a delta.
type DeltaGetOptions struct{}

func (s *deltasService) Get(ds DeltaSpec, opt *DeltaGetOptions) (*Delta, Response, error) {
	url, err := s.client.url(router.Delta, ds.RouteVars(), opt)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, nil, err
	}

	var delta *Delta
	resp, err := s.client.Do(req, &delta)
	if err != nil {
		return nil, resp, err
	}

	return delta, resp, nil
}

// DeltaListDefsOptions specifies options for ListDefs.
type DeltaListDefsOptions struct {
	ListOptions
}

// DeltaDefs describes definitions added/changed/deleted in a delta.
type DeltaDefs struct {
	Defs []*DefDelta // added/changed/deleted defs

	DiffStat diff.Stat // overall diffstat (not subject to pagination)
}

// A DefDelta represents a single definition that was changed. It has
// fields for the before (Base) and after (Head) versions. If both
// Base and Head are non-nil, then the def was changed from base to
// head. Otherwise, one of the fields being nil means that the def did
// not exist in that revision (e.g., it was added or deleted from base
// to head).
type DefDelta struct {
	Base *Def // the def in the base commit (if nil, this def was added in the head)
	Head *Def // the def in the head commit (if nil, this def was deleted in the head)
}

// Added is whether this represents an added def (not present in base,
// present in head).
func (dd DefDelta) Added() bool { return dd.Base == nil && dd.Head != nil }

// Changed is whether this represents a changed def (present in base,
// present in head).
func (dd DefDelta) Changed() bool { return dd.Base != nil && dd.Head != nil }

// Deleted is whether this represents a deleted def (present in base,
// not present in head).
func (dd DefDelta) Deleted() bool { return dd.Base != nil && dd.Head == nil }

func (s *deltasService) ListDefs(ds DeltaSpec, opt *DeltaListDefsOptions) (*DeltaDefs, Response, error) {
	url, err := s.client.url(router.DeltaDefs, ds.RouteVars(), opt)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, nil, err
	}

	var defs *DeltaDefs
	resp, err := s.client.Do(req, &defs)
	if err != nil {
		return nil, resp, err
	}

	return defs, resp, nil
}

// DeltaListDependenciesOptions specifies options for
// ListDependencies.
type DeltaListDependenciesOptions struct {
	ListOptions
}

// DeltaDependencies describes dependencies added/changed/deleted in a
// delta.
type DeltaDependencies struct {
	// TODO(sqs): define this struct

	// Added   []*Dependency
	// Changed []*Dependency
	// Deleted []*Dependency
}

func (s *deltasService) ListDependencies(ds DeltaSpec, opt *DeltaListDependenciesOptions) (*DeltaDependencies, Response, error) {
	url, err := s.client.url(router.DeltaDependencies, ds.RouteVars(), opt)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, nil, err
	}

	var dependencies *DeltaDependencies
	resp, err := s.client.Do(req, &dependencies)
	if err != nil {
		return nil, resp, err
	}

	return dependencies, resp, nil
}

// DeltaListFilesOptions specifies options for
// ListFiles.
type DeltaListFilesOptions struct{}

// DeltaFiles describes files added/changed/deleted in a delta.
type DeltaFiles struct {
	FileDiffs []*diff.FileDiff
}

// DiffStat returns a diffstat that is the sum of all of the files'
// diffstats.
func (d *DeltaFiles) DiffStat() diff.Stat {
	ds := diff.Stat{}
	for _, fd := range d.FileDiffs {
		st := fd.Stat()
		ds.Added += st.Added
		ds.Changed += st.Changed
		ds.Deleted += st.Deleted
	}
	return ds
}

func (s *deltasService) ListFiles(ds DeltaSpec, opt *DeltaListFilesOptions) (*DeltaFiles, Response, error) {
	url, err := s.client.url(router.DeltaFiles, ds.RouteVars(), opt)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, nil, err
	}

	var files *DeltaFiles
	resp, err := s.client.Do(req, &files)
	if err != nil {
		return nil, resp, err
	}

	return files, resp, nil
}

// DeltaAffectedPerson describes a person (registered user or
// committer email address) that is affected by a delta. It includes
// fields for the person affected as well as the defs that are the
// reason why we consider them to be affected.
//
// The person's relationship to the Defs depends on what method
// returned this DeltaAffectedPerson. If it was returned by a method
// that lists authors, then the Defs are definitions that the Person
// committed. If it was returned by a method that lists clients (a.k.a
// users), then the Defs are definitions that the Person uses.
type DeltaAffectedPerson struct {
	Person // the affected person

	Defs []*Def // the defs they authored or use (the reason why they're affected)
}

// DeltaListAffectedAuthorsOptions specifies options for
// ListAffectedAuthors.
type DeltaListAffectedAuthorsOptions struct {
	ListOptions
}

func (s *deltasService) ListAffectedAuthors(ds DeltaSpec, opt *DeltaListAffectedAuthorsOptions) ([]*DeltaAffectedPerson, Response, error) {
	url, err := s.client.url(router.DeltaAffectedAuthors, ds.RouteVars(), opt)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, nil, err
	}

	var authors []*DeltaAffectedPerson
	resp, err := s.client.Do(req, &authors)
	if err != nil {
		return nil, resp, err
	}

	return authors, resp, nil
}

// DeltaListAffectedClientsOptions specifies options for
// ListAffectedClients.
type DeltaListAffectedClientsOptions struct {
	ListOptions
}

func (s *deltasService) ListAffectedClients(ds DeltaSpec, opt *DeltaListAffectedClientsOptions) ([]*DeltaAffectedPerson, Response, error) {
	url, err := s.client.url(router.DeltaAffectedClients, ds.RouteVars(), opt)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, nil, err
	}

	var clients []*DeltaAffectedPerson
	resp, err := s.client.Do(req, &clients)
	if err != nil {
		return nil, resp, err
	}

	return clients, resp, nil
}

// DeltaAffectedRepo describes a repository that is affected by a
// delta.
type DeltaAffectedRepo struct {
	Repository // the affected repository

	DefRefs []*DeltaDefRefs // refs to defs that were changed/deleted
}

// DeltaDefRefs is used in DeltaAffectedRepo to store a single
// changed/deleted def and all of the repository's refs to that def.
type DeltaDefRefs struct {
	Def  *Def       // the changed/deleted def
	Refs []*Example // all of the parent DeltaAffectedRepo.Repository's refs to Def
}

// DeltaListAffectedDependentsOptions specifies options for
// ListAffectedDependents.
type DeltaListAffectedDependentsOptions struct {
	ListOptions
}

func (s *deltasService) ListAffectedDependents(ds DeltaSpec, opt *DeltaListAffectedDependentsOptions) ([]*DeltaAffectedRepo, Response, error) {
	url, err := s.client.url(router.DeltaAffectedDependents, ds.RouteVars(), opt)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, nil, err
	}

	var dependents []*DeltaAffectedRepo
	resp, err := s.client.Do(req, &dependents)
	if err != nil {
		return nil, resp, err
	}

	return dependents, resp, nil
}

// A DeltaReviewer is a person who is reviewing, or is suggested as a
// reviewer for, a delta.
type DeltaReviewer struct {
	Person

	Suggested       bool   `json:",omitempty"` // whether this reviewer is just suggested as a possible reviewer (and not actually assigned)
	ReasonSuggested string `json:",omitempty"` // if Suggested, this is why (e.g., because the person wrote code this delta touches)

	Defs []*Def `json:",omitempty"` // defs that this reviewer committed to and that were changed in or affected by the delta
}

type DeltaListReviewersOptions struct {
	ListOptions
}

func (s *deltasService) ListReviewers(ds DeltaSpec, opt *DeltaListReviewersOptions) ([]*DeltaReviewer, Response, error) {
	url, err := s.client.url(router.DeltaReviewers, ds.RouteVars(), opt)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, nil, err
	}

	var reviewers []*DeltaReviewer
	resp, err := s.client.Do(req, &reviewers)
	if err != nil {
		return nil, resp, err
	}

	return reviewers, resp, nil
}

// DeltaListIncomingOptions specifies options for
// ListIncoming.
type DeltaListIncomingOptions struct {
	ListOptions
}

func (s *deltasService) ListIncoming(rr RepoRevSpec, opt *DeltaListIncomingOptions) ([]*Delta, Response, error) {
	url, err := s.client.url(router.DeltasIncoming, rr.RouteVars(), opt)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, nil, err
	}

	var deltas []*Delta
	resp, err := s.client.Do(req, &deltas)
	if err != nil {
		return nil, resp, err
	}

	return deltas, resp, nil
}

type MockDeltasService struct {
	Get_                    func(ds DeltaSpec, opt *DeltaGetOptions) (*Delta, Response, error)
	ListDefs_               func(ds DeltaSpec, opt *DeltaListDefsOptions) (*DeltaDefs, Response, error)
	ListDependencies_       func(ds DeltaSpec, opt *DeltaListDependenciesOptions) (*DeltaDependencies, Response, error)
	ListFiles_              func(ds DeltaSpec, opt *DeltaListFilesOptions) (*DeltaFiles, Response, error)
	ListAffectedAuthors_    func(ds DeltaSpec, opt *DeltaListAffectedAuthorsOptions) ([]*DeltaAffectedPerson, Response, error)
	ListAffectedClients_    func(ds DeltaSpec, opt *DeltaListAffectedClientsOptions) ([]*DeltaAffectedPerson, Response, error)
	ListAffectedDependents_ func(ds DeltaSpec, opt *DeltaListAffectedDependentsOptions) ([]*DeltaAffectedRepo, Response, error)
	ListReviewers_          func(ds DeltaSpec, opt *DeltaListReviewersOptions) ([]*DeltaReviewer, Response, error)
	ListIncoming_           func(rr RepoRevSpec, opt *DeltaListIncomingOptions) ([]*Delta, Response, error)
}

func (s MockDeltasService) Get(ds DeltaSpec, opt *DeltaGetOptions) (*Delta, Response, error) {
	if s.Get_ == nil {
		return nil, nil, nil
	}
	return s.Get_(ds, opt)
}

func (s MockDeltasService) ListDefs(ds DeltaSpec, opt *DeltaListDefsOptions) (*DeltaDefs, Response, error) {
	if s.ListDefs_ == nil {
		return nil, nil, nil
	}
	return s.ListDefs_(ds, opt)
}

func (s MockDeltasService) ListDependencies(ds DeltaSpec, opt *DeltaListDependenciesOptions) (*DeltaDependencies, Response, error) {
	if s.ListDependencies_ == nil {
		return nil, nil, nil
	}
	return s.ListDependencies_(ds, opt)
}

func (s MockDeltasService) ListFiles(ds DeltaSpec, opt *DeltaListFilesOptions) (*DeltaFiles, Response, error) {
	if s.ListFiles_ == nil {
		return nil, nil, nil
	}
	return s.ListFiles_(ds, opt)
}

func (s MockDeltasService) ListAffectedAuthors(ds DeltaSpec, opt *DeltaListAffectedAuthorsOptions) ([]*DeltaAffectedPerson, Response, error) {
	if s.ListAffectedAuthors_ == nil {
		return nil, nil, nil
	}
	return s.ListAffectedAuthors_(ds, opt)
}

func (s MockDeltasService) ListAffectedClients(ds DeltaSpec, opt *DeltaListAffectedClientsOptions) ([]*DeltaAffectedPerson, Response, error) {
	if s.ListAffectedClients_ == nil {
		return nil, nil, nil
	}
	return s.ListAffectedClients_(ds, opt)
}

func (s MockDeltasService) ListAffectedDependents(ds DeltaSpec, opt *DeltaListAffectedDependentsOptions) ([]*DeltaAffectedRepo, Response, error) {
	if s.ListAffectedDependents_ == nil {
		return nil, nil, nil
	}
	return s.ListAffectedDependents_(ds, opt)
}

func (s MockDeltasService) ListReviewers(ds DeltaSpec, opt *DeltaListReviewersOptions) ([]*DeltaReviewer, Response, error) {
	if s.ListReviewers_ == nil {
		return nil, nil, nil
	}
	return s.ListReviewers_(ds, opt)
}

func (s MockDeltasService) ListIncoming(rr RepoRevSpec, opt *DeltaListIncomingOptions) ([]*Delta, Response, error) {
	if s.ListIncoming_ == nil {
		return nil, nil, nil
	}
	return s.ListIncoming_(rr, opt)
}
