package sourcegraph

import (
	"net/http"
	"reflect"
	"testing"

	"sourcegraph.com/sourcegraph/go-sourcegraph/router"
	"sourcegraph.com/sourcegraph/srclib/graph"
)

func TestDefsService_Get(t *testing.T) {
	setup()
	defer teardown()

	want := &Def{Def: graph.Def{Name: "n"}}

	var called bool
	mux.HandleFunc(urlPath(t, router.Def, map[string]string{"RepoSpec": "r.com/x", "UnitType": "t", "Unit": "u", "Path": "p"}), func(w http.ResponseWriter, r *http.Request) {
		called = true
		testMethod(t, r, "GET")
		testFormValues(t, r, values{"Doc": "true"})

		writeJSON(w, want)
	})

	repo_, _, err := client.Defs.Get(DefSpec{Repo: "r.com/x", UnitType: "t", Unit: "u", Path: "p"}, &DefGetOptions{Doc: true})
	if err != nil {
		t.Errorf("Defs.Get returned error: %v", err)
	}

	if !called {
		t.Fatal("!called")
	}

	if !reflect.DeepEqual(repo_, want) {
		t.Errorf("Defs.Get returned %+v, want %+v", repo_, want)
	}
}

func TestDefsService_List(t *testing.T) {
	setup()
	defer teardown()

	want := []*Def{{Def: graph.Def{Name: "n"}}}

	var called bool
	mux.HandleFunc(urlPath(t, router.Defs, nil), func(w http.ResponseWriter, r *http.Request) {
		called = true
		testMethod(t, r, "GET")
		testFormValues(t, r, values{
			"RepoRevs":  "r1,r2@x",
			"Sort":      "name",
			"Direction": "asc",
			"Kinds":     "a,b",
			"Exported":  "true",
			"Doc":       "true",
			"PerPage":   "1",
			"Page":      "2",
			"ByteStart": "0",
			"ByteEnd":   "0",
		})

		writeJSON(w, want)
	})

	defs, _, err := client.Defs.List(&DefListOptions{
		RepoRevs:    []string{"r1", "r2@x"},
		Sort:        "name",
		Direction:   "asc",
		Kinds:       []string{"a", "b"},
		Exported:    true,
		Doc:         true,
		ListOptions: ListOptions{PerPage: 1, Page: 2},
	})
	if err != nil {
		t.Errorf("Defs.List returned error: %v", err)
	}

	if !called {
		t.Fatal("!called")
	}

	if !reflect.DeepEqual(defs, want) {
		t.Errorf("Defs.List returned %+v, want %+v", defs, want)
	}
}

func TestDefsService_ListRefs(t *testing.T) {
	setup()
	defer teardown()

	want := []*Ref{{Ref: graph.Ref{File: "f"}}}

	var called bool
	mux.HandleFunc(urlPath(t, router.DefRefs, map[string]string{"RepoSpec": "r.com/x", "UnitType": "t", "Unit": "u", "Path": "p"}), func(w http.ResponseWriter, r *http.Request) {
		called = true
		testMethod(t, r, "GET")
		testFormValues(t, r, values{"Authorship": "true"})

		writeJSON(w, want)
	})

	refs, _, err := client.Defs.ListRefs(DefSpec{Repo: "r.com/x", UnitType: "t", Unit: "u", Path: "p"}, &DefListRefsOptions{Authorship: true})
	if err != nil {
		t.Errorf("Defs.ListRefs returned error: %v", err)
	}

	if !called {
		t.Fatal("!called")
	}

	if !reflect.DeepEqual(refs, want) {
		t.Errorf("Defs.ListRefs returned %+v, want %+v", refs, want)
	}
}

func TestDefsService_ListExamples(t *testing.T) {
	setup()
	defer teardown()

	want := []*Example{{Ref: graph.Ref{File: "f"}}}

	var called bool
	mux.HandleFunc(urlPath(t, router.DefExamples, map[string]string{"RepoSpec": "r.com/x", "UnitType": "t", "Unit": "u", "Path": "p"}), func(w http.ResponseWriter, r *http.Request) {
		called = true
		testMethod(t, r, "GET")

		writeJSON(w, want)
	})

	refs, _, err := client.Defs.ListExamples(DefSpec{Repo: "r.com/x", UnitType: "t", Unit: "u", Path: "p"}, nil)
	if err != nil {
		t.Errorf("Defs.ListExamples returned error: %v", err)
	}

	if !called {
		t.Fatal("!called")
	}

	if !reflect.DeepEqual(refs, want) {
		t.Errorf("Defs.ListExamples returned %+v, want %+v", refs, want)
	}
}

func TestDefsService_ListAuthors(t *testing.T) {
	setup()
	defer teardown()

	want := []*AugmentedDefAuthor{{Person: &Person{FullName: "b"}}}

	var called bool
	mux.HandleFunc(urlPath(t, router.DefAuthors, map[string]string{"RepoSpec": "r.com/x", "UnitType": "t", "Unit": "u", "Path": "p"}), func(w http.ResponseWriter, r *http.Request) {
		called = true
		testMethod(t, r, "GET")

		writeJSON(w, want)
	})

	authors, _, err := client.Defs.ListAuthors(DefSpec{Repo: "r.com/x", UnitType: "t", Unit: "u", Path: "p"}, nil)
	if err != nil {
		t.Errorf("Defs.ListAuthors returned error: %v", err)
	}

	if !called {
		t.Fatal("!called")
	}

	if !reflect.DeepEqual(authors, want) {
		t.Errorf("Defs.ListAuthors returned %+v, want %+v", authors, want)
	}
}
