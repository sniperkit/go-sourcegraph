package sourcegraph

import (
	"errors"
	"fmt"
	"text/template"

	"sourcegraph.com/sourcegraph/go-vcs/vcs"
	"sourcegraph.com/sourcegraph/vcsstore/vcsclient"

	"strconv"
	"strings"

	"sourcegraph.com/sourcegraph/go-sourcegraph/router"
)

// ReposService communicates with the repository-related endpoints in the
// Sourcegraph API.
type ReposService interface {
	// Get fetches a repository.
	Get(repo RepoSpec, opt *RepoGetOptions) (*Repo, Response, error)

	// GetStats gets statistics about a repository at a specific
	// commit. Some statistics are per-commit and some are global to
	// the repository. If you only care about global repository
	// statistics, pass an empty Rev to the RepoRevSpec (which will be
	// resolved to the repository's default branch).
	GetStats(repo RepoRevSpec) (RepoStats, Response, error)

	// CreateStatus creates a repository status for the given commit.
	CreateStatus(spec RepoRevSpec, st RepoStatus) (*RepoStatus, Response, error)

	// GetCombinedStatus fetches the combined repository status for
	// the given commit.
	GetCombinedStatus(spec RepoRevSpec) (*CombinedStatus, Response, error)

	// GetOrCreate fetches a repository using Get. If no such repository exists
	// with the URI, and the URI refers to a recognized repository host (such as
	// github.com), the repository's information is fetched from the external
	// host and the repository is created.
	GetOrCreate(repo RepoSpec, opt *RepoGetOptions) (*Repo, Response, error)

	// GetSettings fetches a repository's configuration settings.
	GetSettings(repo RepoSpec) (*RepoSettings, Response, error)

	// UpdateSettings updates a repository's configuration settings.
	UpdateSettings(repo RepoSpec, settings RepoSettings) (Response, error)

	// RefreshProfile updates the repository metadata for a repository, fetching
	// it from an external host if the host is recognized (such as GitHub).
	//
	// This operation is performed asynchronously on the server side (after
	// receiving the request) and the API currently has no way of notifying
	// callers when the operation completes.
	RefreshProfile(repo RepoSpec) (Response, error)

	// RefreshVCSData updates the repository VCS (git/hg) data, fetching all new
	// commits, branches, tags, and blobs.
	//
	// This operation is performed asynchronously on the server side (after
	// receiving the request) and the API currently has no way of notifying
	// callers when the operation completes.
	RefreshVCSData(repo RepoSpec) (Response, error)

	// ComputeStats updates the statistics about a repository.
	//
	// This operation is performed asynchronously on the server side (after
	// receiving the request) and the API currently has no way of notifying
	// callers when the operation completes.
	ComputeStats(repo RepoRevSpec) (Response, error)

	// GetBuild gets the build for a specific revspec. It returns
	// additional information about the build, such as whether it is
	// exactly up-to-date with the revspec or a few commits behind the
	// revspec. The opt param controls what is returned in this case.
	GetBuild(repo RepoRevSpec, opt *RepoGetBuildOptions) (*RepoBuildInfo, Response, error)

	// Create adds the repository at cloneURL, filling in all information about
	// the repository that can be inferred from the URL (or, for GitHub
	// repositories, fetched from the GitHub API). If a repository with the
	// specified clone URL, or the same URI, already exists, it is returned.
	Create(newRepoSpec NewRepoSpec) (*Repo, Response, error)

	// GetReadme fetches the formatted README file for a repository.
	GetReadme(repo RepoRevSpec) (*vcsclient.TreeEntry, Response, error)

	// List repositories.
	List(opt *RepoListOptions) ([]*Repo, Response, error)

	// List commits.
	ListCommits(repo RepoSpec, opt *RepoListCommitsOptions) ([]*Commit, Response, error)

	// GetCommit gets a commit.
	GetCommit(rev RepoRevSpec, opt *RepoGetCommitOptions) (*Commit, Response, error)

	// ListBranches lists a repository's branches.
	ListBranches(repo RepoSpec, opt *RepoListBranchesOptions) ([]*vcs.Branch, Response, error)

	// ListTags lists a repository's tags.
	ListTags(repo RepoSpec, opt *RepoListTagsOptions) ([]*vcs.Tag, Response, error)

	// ListBadges lists the available badges for repo.
	ListBadges(repo RepoSpec) ([]*Badge, Response, error)

	// ListCounters lists the available counters for repo.
	ListCounters(repo RepoSpec) ([]*Counter, Response, error)
}

// repositoriesService implements ReposService.
type repositoriesService struct {
	client *Client
}

var _ ReposService = &repositoriesService{}

// RepoSpec specifies a repository.
type RepoSpec struct {
	URI string
	RID int
}

// PathComponent returns the URL path component that specifies the
// repository.
func (s RepoSpec) PathComponent() string {
	if s.RID > 0 {
		return "R$" + strconv.Itoa(s.RID)
	}
	if s.URI != "" {
		if strings.HasPrefix("sourcegraph.com/", s.URI) {
			return s.URI[len("sourcegraph.com/"):]
		} else {
			return s.URI
		}
	}
	panic("empty RepoSpec")
}

// RouteVars returns route variables for constructing repository
// routes.
func (s RepoSpec) RouteVars() map[string]string {
	return map[string]string{"RepoSpec": s.PathComponent()}
}

// ParseRepoSpec parses a string generated by
// (*RepoSpec).PathComponent() and returns the equivalent
// RepoSpec struct.
func ParseRepoSpec(pathComponent string) (RepoSpec, error) {
	if pathComponent == "" {
		return RepoSpec{}, errors.New("empty repository spec")
	}
	if strings.HasPrefix(pathComponent, "R$") {
		rid, err := strconv.Atoi(pathComponent[2:])
		return RepoSpec{RID: rid}, err
	}

	var uri string
	if strings.HasPrefix(pathComponent, "sourcegraph/") {
		uri = "sourcegraph.com/" + pathComponent
	} else {
		uri = pathComponent
	}

	return RepoSpec{URI: uri}, nil
}

// UnmarshalRepoSpec marshals a map containing route variables
// generated by (*RepoSpec).RouteVars() and returns the
// equivalent RepoSpec struct.
func UnmarshalRepoSpec(routeVars map[string]string) (RepoSpec, error) {
	return ParseRepoSpec(routeVars["RepoSpec"])
}

// RepoRevSpec specifies a repository at a specific commit (or
// revision specifier, such as a branch, which is resolved on the
// server side to a specific commit).
//
// Filling in CommitID is an optional optimization. It avoids the need
// for another resolution of Rev. If CommitID is filled in, the "Rev"
// route variable becomes "Rev===CommitID" (e.g.,
// "master===af4cd6"). Handlers can parse this string to retrieve the
// pre-resolved commit ID (e.g., "af4cd6") and still return data that
// constructs URLs using the unresolved revspec (e.g., "master").
//
// Why is it important/useful to pass the resolved commit ID instead
// of just using a revspec everywhere? Consider this case. Your
// application wants to make a bunch of requests for resources
// relating to "master"; for example, it wants to retrieve a source
// file foo.go at master and all of the definitions and references
// contained in the file. This may consist of dozens of API calls. If
// each API call specified just "master", there would be 2 problems:
// (1) each API call would have to re-resolve "master" to its actual
// commit ID, which takes a lot of extra work; and (2) if the "master"
// ref changed during the API calls (if someone pushed in the middle
// of the API call, for example), then your application would receive
// data from 2 different commits. The solution is for your application
// to resolve the revspec once and pass both the original revspec and
// the resolved commit ID in all API calls it makes.
//
// And why do we want to preserve the unresolved revspec? In this
// case, your app wants to let the user continue browsing "master". If
// the API data all referred to a specific commit ID, then the user
// would cease browsing master the next time she clicked a link on
// your app. Preserving the revspec gives the user a choice whether to
// use the absolute commit ID or the revspec (similar to how GitHub
// lets you canonicalize a URL with 'y' but does not default to using
// the canonical URL).
type RepoRevSpec struct {
	RepoSpec        // repository URI or RID
	Rev      string // the abstract/unresolved revspec, such as a branch name or abbreviated commit ID
	CommitID string // the full commit ID that Rev resolves to
}

const repoRevSpecCommitSep = "==="

// RouteVars returns route variables for constructing routes to a
// repository commit.
func (s RepoRevSpec) RouteVars() map[string]string {
	m := s.RepoSpec.RouteVars()
	m["Rev"] = s.RevPathComponent()
	return m
}

// RevPathComponent encodes the revision and commit ID for use in a
// URL path. If CommitID is set, the path component is
// "Rev===CommitID"; otherwise, it is just "Rev". See the docstring
// for RepoRevSpec for an explanation why.
func (s RepoRevSpec) RevPathComponent() string {
	if s.Rev == "" && s.CommitID != "" {
		panic("invalid empty Rev but non-empty CommitID (" + s.CommitID + ")")
	}
	if s.CommitID != "" {
		return s.Rev + repoRevSpecCommitSep + s.CommitID
	}
	return s.Rev
}

// UnmarshalRepoRevSpec marshals a map containing route variables
// generated by (*RepoRevSpec).RouteVars() and returns the equivalent
// RepoRevSpec struct.
func UnmarshalRepoRevSpec(routeVars map[string]string) (RepoRevSpec, error) {
	repoSpec, err := UnmarshalRepoSpec(routeVars)
	if err != nil {
		return RepoRevSpec{}, err
	}

	repoRevSpec := RepoRevSpec{RepoSpec: repoSpec}
	revStr := routeVars["Rev"]
	if i := strings.Index(revStr, repoRevSpecCommitSep); i == -1 {
		repoRevSpec.Rev = revStr
	} else {
		repoRevSpec.Rev = revStr[:i]
		repoRevSpec.CommitID = revStr[i+len(repoRevSpecCommitSep):]
	}

	if repoRevSpec.Rev == "" && repoRevSpec.CommitID != "" {
		return RepoRevSpec{}, fmt.Errorf("invalid empty Rev but non-empty CommitID (%q)", repoRevSpec.CommitID)
	}

	return repoRevSpec, nil
}

// RepoGetOptions specifies options for getting a repository.
type RepoGetOptions struct {
	Stats bool `url:",omitempty" json:",omitempty"` // whether to fetch and include stats in the returned repository
}

func (s *repositoriesService) Get(repo RepoSpec, opt *RepoGetOptions) (*Repo, Response, error) {
	url, err := s.client.URL(router.Repo, repo.RouteVars(), opt)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, nil, err
	}

	var repo_ *Repo
	resp, err := s.client.Do(req, &repo_)
	if err != nil {
		return nil, resp, err
	}

	return repo_, resp, nil
}

func (s *repositoriesService) GetStats(repoRev RepoRevSpec) (RepoStats, Response, error) {
	url, err := s.client.URL(router.RepoStats, repoRev.RouteVars(), nil)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, nil, err
	}

	var stats RepoStats
	resp, err := s.client.Do(req, &stats)
	if err != nil {
		return nil, resp, err
	}

	return stats, resp, nil
}

func (s *repositoriesService) GetOrCreate(repo_ RepoSpec, opt *RepoGetOptions) (*Repo, Response, error) {
	url, err := s.client.URL(router.ReposGetOrCreate, repo_.RouteVars(), opt)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("PUT", url.String(), nil)
	if err != nil {
		return nil, nil, err
	}

	var repo__ *Repo
	resp, err := s.client.Do(req, &repo__)
	if err != nil {
		return nil, resp, err
	}

	return repo__, resp, nil
}

// RepoSettings describes a repository's configuration settings.
type RepoSettings struct {
	// Enabled is whether this repository has been enabled for use on
	// Sourcegraph by a repository owner or a site admin.
	Enabled *bool `db:"enabled" json:",omitempty"`

	// BuildPushes is whether head commits on newly pushed branches
	// should be automatically built.
	BuildPushes *bool `db:"build_pushes" json:",omitempty"`

	// ExternalCommitStatuses is whether the build status
	// (pending/failure/success) of each commit should be published to
	// GitHub using the repo status API
	// (https://developer.github.com/v3/repos/statuses/).
	//
	// This behavior is also subject to the
	// UnsuccessfulExternalCommitStatuses setting value.
	ExternalCommitStatuses *bool `db:"external_commit_statuses" json:",omitempty"`

	// UnsuccessfulExternalCommitStatuses, if true, indicates that
	// pending/failure commit statuses should be published to
	// GitHub. If false (default), only successful commit status are
	// published. The default of false avoids bothersome warning
	// messages and UI pollution in case the Sourcegraph build
	// fails. Until our builds are highly reliable, a Sourcegraph
	// build failure is not necessarily an indication of a problem
	// with the repository.
	//
	// This setting is only meaningful if ExternalCommitStatuses is
	// true.
	UnsuccessfulExternalCommitStatuses *bool `db:"unsuccessful_external_commit_statuses" json:",omitempty"`

	// UseSSHPrivateKey is whether Sourcegraph should clone and update
	// the repository using an SSH key, and whether it should copy the
	// corresponding public key to the repository's origin host as an
	// authorized key. It is only necessary for private repositories
	// and for write operations on public repositories.
	UseSSHPrivateKey *bool `db:"use_ssh_private_key" json:",omitempty"`
}

func (s *repositoriesService) GetSettings(repo RepoSpec) (*RepoSettings, Response, error) {
	url, err := s.client.URL(router.RepoSettings, repo.RouteVars(), nil)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, nil, err
	}

	var settings *RepoSettings
	resp, err := s.client.Do(req, &settings)
	if err != nil {
		return nil, resp, err
	}

	return settings, resp, nil
}

func (s *repositoriesService) UpdateSettings(repo RepoSpec, settings RepoSettings) (Response, error) {
	url, err := s.client.URL(router.RepoSettingsUpdate, repo.RouteVars(), nil)
	if err != nil {
		return nil, err
	}

	req, err := s.client.NewRequest("PUT", url.String(), settings)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req, nil)
	if err != nil {
		return resp, err
	}

	return resp, nil
}

func (s *repositoriesService) RefreshProfile(repo RepoSpec) (Response, error) {
	url, err := s.client.URL(router.RepoRefreshProfile, repo.RouteVars(), nil)
	if err != nil {
		return nil, err
	}

	req, err := s.client.NewRequest("PUT", url.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req, nil)
	if err != nil {
		return resp, err
	}

	return resp, nil
}

func (s *repositoriesService) RefreshVCSData(repo RepoSpec) (Response, error) {
	url, err := s.client.URL(router.RepoRefreshVCSData, repo.RouteVars(), nil)
	if err != nil {
		return nil, err
	}

	req, err := s.client.NewRequest("PUT", url.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req, nil)
	if err != nil {
		return resp, err
	}

	return resp, nil
}

func (s *repositoriesService) ComputeStats(repo RepoRevSpec) (Response, error) {
	url, err := s.client.URL(router.RepoComputeStats, repo.RouteVars(), nil)
	if err != nil {
		return nil, err
	}

	req, err := s.client.NewRequest("PUT", url.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req, nil)
	if err != nil {
		return resp, err
	}

	return resp, nil
}

// RepoGetBuildOptions sets options for the Repos.GetBuild call.
type RepoGetBuildOptions struct {
	// Exact is whether only a build whose commit ID exactly matches
	// the revspec should be returned. (For non-full-commit ID
	// revspecs, such as branches, tags, and partial commit IDs, this
	// means that the build's commit ID matches the resolved revspec's
	// commit ID.)
	//
	// If Exact is false, then builds for older commits that are
	// reachable from the revspec may also be returned. For example,
	// if there's a build for master~1 but no build for master, and
	// your revspec is master, using Exact=false will return the build
	// for master~1.
	//
	// Using Exact=true is faster as the commit and build history
	// never needs to be searched. If the exact build is not
	// found, or the exact build was found but it failed,
	// LastSuccessful and LastSuccessfulCommit for RepoBuildInfo
	// will be nil.
	Exact bool `url:",omitempty" json:",omitempty"`
}

// RepoBuildInfo holds a repository build (if one exists for the
// originally specified revspec) and additional information. It is returned by
// Repos.GetBuild.
type RepoBuildInfo struct {
	Exact *Build // the newest build, if any, that exactly matches the revspec (can be same as LastSuccessful)

	LastSuccessful *Build // the last successful build of a commit ID reachable from the revspec (can be same as Exact)

	CommitsBehind        int     // the number of commits between the revspec and the commit of the LastSuccessful build
	LastSuccessfulCommit *Commit // the commit of the LastSuccessful build
}

func (s *repositoriesService) GetBuild(repo RepoRevSpec, opt *RepoGetBuildOptions) (*RepoBuildInfo, Response, error) {
	url, err := s.client.URL(router.RepoBuild, repo.RouteVars(), opt)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, nil, err
	}

	var info *RepoBuildInfo
	resp, err := s.client.Do(req, &info)
	if err != nil {
		return nil, resp, err
	}

	return info, resp, nil
}

type NewRepoSpec struct {
	Type        string
	CloneURLStr string `json:"CloneURL"`
}

func (s *repositoriesService) Create(newRepoSpec NewRepoSpec) (*Repo, Response, error) {
	url, err := s.client.URL(router.ReposCreate, nil, nil)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("POST", url.String(), newRepoSpec)
	if err != nil {
		return nil, nil, err
	}

	var repo_ *Repo
	resp, err := s.client.Do(req, &repo_)
	if err != nil {
		return nil, resp, err
	}

	return repo_, resp, nil
}

func (s *repositoriesService) GetReadme(repo RepoRevSpec) (*vcsclient.TreeEntry, Response, error) {
	url, err := s.client.URL(router.RepoReadme, repo.RouteVars(), nil)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, nil, err
	}

	var readme *vcsclient.TreeEntry
	resp, err := s.client.Do(req, &readme)
	if err != nil {
		return nil, resp, err
	}

	return readme, resp, nil
}

type RepoListOptions struct {
	Name string `url:",omitempty" json:",omitempty"`

	// Specifies a search query for repositories. If specified, then the Sort and Direction options are ignored
	Query string `url:",omitempty" json:",omitempty"`

	URIs []string `url:",comma,omitempty" json:",omitempty"`

	BuiltOnly bool `url:",omitempty" json:",omitempty"`

	Sort      string `url:",omitempty" json:",omitempty"`
	Direction string `url:",omitempty" json:",omitempty"`

	NoFork bool `url:",omitempty" json:",omitempty"`

	Type string `url:",omitempty" json:",omitempty"` // "public" or "private" (empty default means "all")

	State string `url:",omitempty" json:",omitempty"` // "enabled" or "disabled" (empty default means return "all")

	Owner string `url:",omitempty" json:",omitempty"`

	Stats bool `url:",omitempty" json:",omitempty"` // whether to fetch and include stats in the returned repositories

	ListOptions
}

func (s *repositoriesService) List(opt *RepoListOptions) ([]*Repo, Response, error) {
	url, err := s.client.URL(router.Repos, nil, opt)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, nil, err
	}

	var repos []*Repo
	resp, err := s.client.Do(req, &repos)
	if err != nil {
		return nil, resp, err
	}

	return repos, resp, nil
}

type Commit struct {
	*vcs.Commit
}

type RepoListCommitsOptions struct {
	Head string `url:",omitempty" json:",omitempty"`
	Base string `url:",omitempty" json:",omitempty"`
	ListOptions
}

func (s *repositoriesService) ListCommits(repo RepoSpec, opt *RepoListCommitsOptions) ([]*Commit, Response, error) {
	url, err := s.client.URL(router.RepoCommits, repo.RouteVars(), opt)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, nil, err
	}

	var commits []*Commit
	resp, err := s.client.Do(req, &commits)
	if err != nil {
		return nil, resp, err
	}

	return commits, resp, nil
}

type RepoGetCommitOptions struct {
}

func (s *repositoriesService) GetCommit(rev RepoRevSpec, opt *RepoGetCommitOptions) (*Commit, Response, error) {
	url, err := s.client.URL(router.RepoCommit, rev.RouteVars(), opt)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, nil, err
	}

	var commit *Commit
	resp, err := s.client.Do(req, &commit)
	if err != nil {
		return nil, resp, err
	}

	return commit, resp, nil
}

type RepoListBranchesOptions struct {
	ListOptions
}

func (s *repositoriesService) ListBranches(repo RepoSpec, opt *RepoListBranchesOptions) ([]*vcs.Branch, Response, error) {
	url, err := s.client.URL(router.RepoBranches, repo.RouteVars(), opt)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, nil, err
	}

	var branches []*vcs.Branch
	resp, err := s.client.Do(req, &branches)
	if err != nil {
		return nil, resp, err
	}

	return branches, resp, nil
}

type RepoListTagsOptions struct {
	ListOptions
}

func (s *repositoriesService) ListTags(repo RepoSpec, opt *RepoListTagsOptions) ([]*vcs.Tag, Response, error) {
	url, err := s.client.URL(router.RepoTags, repo.RouteVars(), opt)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, nil, err
	}

	var tags []*vcs.Tag
	resp, err := s.client.Do(req, &tags)
	if err != nil {
		return nil, resp, err
	}

	return tags, resp, nil
}

type Badge struct {
	Name              string
	Description       string
	ImageURL          string
	UncountedImageURL string
	Markdown          string
}

func (b *Badge) HTML() string {
	return fmt.Sprintf(`<img src="%s" alt="%s">`, template.HTMLEscapeString(b.ImageURL), template.HTMLEscapeString(b.Name))
}

func (s *repositoriesService) ListBadges(repo RepoSpec) ([]*Badge, Response, error) {
	url, err := s.client.URL(router.RepoBadges, repo.RouteVars(), nil)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, nil, err
	}

	var badges []*Badge
	resp, err := s.client.Do(req, &badges)
	if err != nil {
		return nil, resp, err
	}

	return badges, resp, nil
}

type Counter struct {
	Name              string
	Description       string
	ImageURL          string
	UncountedImageURL string
	Markdown          string
}

func (c *Counter) HTML() string {
	return fmt.Sprintf(`<img src="%s" alt="%s">`, template.HTMLEscapeString(c.ImageURL), template.HTMLEscapeString(c.Name))
}

func (s *repositoriesService) ListCounters(repo RepoSpec) ([]*Counter, Response, error) {
	url, err := s.client.URL(router.RepoCounters, repo.RouteVars(), nil)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, nil, err
	}

	var counters []*Counter
	resp, err := s.client.Do(req, &counters)
	if err != nil {
		return nil, resp, err
	}

	return counters, resp, nil
}

var _ ReposService = &MockReposService{}
