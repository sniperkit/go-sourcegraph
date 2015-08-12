package sourcegraph

import (
	"reflect"
	"testing"
)

func TestRepoSpec(t *testing.T) {
	tests := []struct {
		str  string
		spec RepoSpec
	}{
		{"src:///a.b/x", RepoSpec{URI: "src:///a.b/x"}},
		{"src:///x", RepoSpec{URI: "src:///x"}},
	}

	for _, test := range tests {
		spec, err := ParseRepoSpec(test.str)
		if err != nil {
			t.Errorf("%q: ParseRepoSpec failed: %s", test.str, err)
			continue
		}
		if spec != test.spec {
			t.Errorf("%q: got spec %+v, want %+v", test.str, spec, test.spec)
			continue
		}

		str := test.spec.SpecString()
		if str != test.str {
			t.Errorf("%+v: got str %q, want %q", test.spec, str, test.str)
			continue
		}

		spec2, err := UnmarshalRepoSpec(test.spec.RouteVars())
		if err != nil {
			t.Errorf("%+v: UnmarshalRepoSpec: %s", test.spec, err)
			continue
		}
		if spec2 != test.spec {
			t.Errorf("%q: got spec %+v, want %+v", test.str, spec, test.spec)
			continue
		}
	}
}

func TestRepoRevSpec(t *testing.T) {
	tests := []struct {
		spec      RepoRevSpec
		routeVars map[string]string
	}{
		{RepoRevSpec{RepoSpec: RepoSpec{URI: "src:///a/x"}, Rev: "r"}, map[string]string{"Repo": "src:///a/x", "Rev": "r"}},
		{RepoRevSpec{RepoSpec: RepoSpec{URI: "src:///x"}, Rev: "r"}, map[string]string{"Repo": "src:///x", "Rev": "r"}},
		{RepoRevSpec{RepoSpec: RepoSpec{URI: "src:///a/x"}, Rev: "r", CommitID: commitID}, map[string]string{"Repo": "src:///a/x", "Rev": "r", "CommitID": commitID}},
	}

	for _, test := range tests {
		routeVars := test.spec.RouteVars()
		if !reflect.DeepEqual(routeVars, test.routeVars) {
			t.Errorf("got route vars %+v, want %+v", routeVars, test.routeVars)
		}
		spec, err := UnmarshalRepoRevSpec(routeVars)
		if err != nil {
			t.Errorf("UnmarshalRepoRevSpec(%+v): %s", routeVars, err)
			continue
		}
		if spec != test.spec {
			t.Errorf("got spec %+v, want %+v", spec, test.spec)
		}
	}
}
