package reconciler_test

import (
	"context"
	"errors"
	"maps"
	"strconv"
	"sync"
	"testing"

	reconciler "github.com/yornik/go-practice/exercises/22-reconciler"
)

func TestObjectDeepCopy(t *testing.T) {
	orig := reconciler.Object{Name: "web", Spec: spec{"replicas": "3"}, ResourceVersion: 7}
	cp := orig.DeepCopy()
	if cp.Name != orig.Name || cp.ResourceVersion != orig.ResourceVersion || !maps.Equal(cp.Spec, orig.Spec) {
		t.Fatalf("DeepCopy() = %+v, want %+v", cp, orig)
	}
	cp.Spec["replicas"] = "5"
	if orig.Spec["replicas"] != "3" {
		t.Fatalf("changing the Spec of the copy changed the original to %v: the copy must not share the map", orig.Spec)
	}
}

func TestMemoryStoreCreateAndGet(t *testing.T) {
	hangGuard(t)
	ctx := context.Background()
	s := reconciler.NewMemoryStore()

	created, err := s.Create(ctx, reconciler.Object{Name: "web", Spec: spec{"replicas": "3"}})
	if err != nil {
		t.Fatalf("Create(web) error = %v, want nil", err)
	}
	if created.Name != "web" || !maps.Equal(created.Spec, spec{"replicas": "3"}) || created.ResourceVersion <= 0 {
		t.Fatalf("Create(web) = %+v, want name web, spec map[replicas:3] and a ResourceVersion > 0", created)
	}

	got := mustGet(t, s, "web")
	if got.Name != "web" || !maps.Equal(got.Spec, created.Spec) || got.ResourceVersion != created.ResourceVersion {
		t.Fatalf("Get(web) = %+v, want %+v", got, created)
	}

	if _, err := s.Get(ctx, "nope"); !errors.Is(err, reconciler.ErrNotFound) {
		t.Fatalf("Get(nope) error = %v, want an error that wraps ErrNotFound", err)
	}

	_, err = s.Create(ctx, reconciler.Object{Name: "web", Spec: spec{"replicas": "9"}})
	if !errors.Is(err, reconciler.ErrAlreadyExists) {
		t.Fatalf("Create(web) a second time: error = %v, want an error that wraps ErrAlreadyExists", err)
	}
	if got := mustGet(t, s, "web"); !maps.Equal(got.Spec, spec{"replicas": "3"}) {
		t.Fatalf("a failed Create changed the stored object to %+v", got)
	}
}

func TestMemoryStoreResourceVersionIncreases(t *testing.T) {
	hangGuard(t)
	ctx := context.Background()
	s := reconciler.NewMemoryStore()
	var last int64
	check := func(step string, obj reconciler.Object, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: error = %v", step, err)
		}
		if obj.ResourceVersion <= last {
			t.Fatalf("%s: ResourceVersion = %d, want > %d (greater than every version handed out before)", step, obj.ResourceVersion, last)
		}
		if got := mustGet(t, s, obj.Name); got.ResourceVersion != obj.ResourceVersion {
			t.Fatalf("%s: Get(%q).ResourceVersion = %d, want %d (the version the write returned)", step, obj.Name, got.ResourceVersion, obj.ResourceVersion)
		}
		last = obj.ResourceVersion
	}

	web, err := s.Create(ctx, reconciler.Object{Name: "web", Spec: spec{"replicas": "1"}})
	check("Create(web)", web, err)
	api, err := s.Create(ctx, reconciler.Object{Name: "api", Spec: spec{"replicas": "1"}})
	check("Create(api)", api, err)
	web.Spec["replicas"] = "2"
	web, err = s.Update(ctx, web)
	check("Update(web)", web, err)
	if err := s.Delete(ctx, "api"); err != nil {
		t.Fatalf("Delete(api) error = %v", err)
	}
	// Create it again from the old copy, which still carries its old version
	// (like re-applying a saved manifest): Create ignores that version.
	api, err = s.Create(ctx, api)
	check("Create(api) after Delete(api), from the old copy of api", api, err)
}

func TestMemoryStoreUpdate(t *testing.T) {
	hangGuard(t)
	ctx := context.Background()

	t.Run("with the version it read", func(t *testing.T) {
		s := reconciler.NewMemoryStore()
		seed(t, s, map[string]spec{"web": {"replicas": "1"}})
		obj := mustGet(t, s, "web")
		old := obj.ResourceVersion
		obj.Spec["replicas"] = "5"
		updated, err := s.Update(ctx, obj)
		if err != nil {
			t.Fatalf("Update with the current ResourceVersion: error = %v, want nil", err)
		}
		if !maps.Equal(updated.Spec, spec{"replicas": "5"}) || updated.ResourceVersion <= old {
			t.Fatalf("Update() = %+v, want spec map[replicas:5] and a ResourceVersion > %d", updated, old)
		}
		if got := mustGet(t, s, "web"); !maps.Equal(got.Spec, spec{"replicas": "5"}) || got.ResourceVersion != updated.ResourceVersion {
			t.Fatalf("Get(web) after Update = %+v, want %+v", got, updated)
		}
	})

	t.Run("two writers read the same version: the second one gets ErrConflict", func(t *testing.T) {
		s := reconciler.NewMemoryStore()
		seed(t, s, map[string]spec{"web": {"replicas": "1", "image": "v1"}})
		a := mustGet(t, s, "web") // an operator runs kubectl scale
		b := mustGet(t, s, "web") // a controller rolls out a new image
		a.Spec["replicas"] = "5"
		if _, err := s.Update(ctx, a); err != nil {
			t.Fatalf("first Update error = %v, want nil", err)
		}
		b.Spec["image"] = "v2"
		_, err := s.Update(ctx, b)
		if !errors.Is(err, reconciler.ErrConflict) {
			t.Fatalf("second Update with the version read before the first Update: error = %v, want an error that wraps ErrConflict", err)
		}
		want := spec{"replicas": "5", "image": "v1"}
		if got := mustGet(t, s, "web"); !maps.Equal(got.Spec, want) {
			t.Fatalf("Get(web) = %v, want %v: the rejected Update must not change the object", got.Spec, want)
		}
	})

	t.Run("version 0 is never current", func(t *testing.T) {
		s := reconciler.NewMemoryStore()
		seed(t, s, map[string]spec{"web": {"replicas": "1"}})
		// A blind write: the caller never read the object.
		_, err := s.Update(ctx, reconciler.Object{Name: "web", Spec: spec{"replicas": "5"}})
		if !errors.Is(err, reconciler.ErrConflict) {
			t.Fatalf("Update with ResourceVersion 0: error = %v, want an error that wraps ErrConflict", err)
		}
		if got := mustGet(t, s, "web"); !maps.Equal(got.Spec, spec{"replicas": "1"}) {
			t.Fatalf("Get(web) = %v, want map[replicas:1]: the rejected Update must not change the object", got.Spec)
		}
	})

	t.Run("missing object", func(t *testing.T) {
		s := reconciler.NewMemoryStore()
		_, err := s.Update(ctx, reconciler.Object{Name: "web", Spec: spec{"replicas": "5"}, ResourceVersion: 1})
		if !errors.Is(err, reconciler.ErrNotFound) {
			t.Fatalf("Update of a missing object: error = %v, want an error that wraps ErrNotFound", err)
		}
		expectGone(t, s, "web")
	})
}

func TestMemoryStoreDelete(t *testing.T) {
	hangGuard(t)
	ctx := context.Background()
	s := reconciler.NewMemoryStore()
	seed(t, s, map[string]spec{"web": {"replicas": "1"}, "api": {"replicas": "1"}})
	if err := s.Delete(ctx, "web"); err != nil {
		t.Fatalf("Delete(web) error = %v, want nil", err)
	}
	expectGone(t, s, "web")
	mustGet(t, s, "api") // the other object is still there
	if err := s.Delete(ctx, "web"); !errors.Is(err, reconciler.ErrNotFound) {
		t.Fatalf("Delete(web) a second time: error = %v, want an error that wraps ErrNotFound", err)
	}
	if _, err := s.Create(ctx, reconciler.Object{Name: "web"}); err != nil {
		t.Fatalf("Create(web) after Delete(web): error = %v, want nil (the name is free again)", err)
	}
}

func TestMemoryStoreList(t *testing.T) {
	hangGuard(t)
	ctx := context.Background()
	s := reconciler.NewMemoryStore()
	objs, err := s.List(ctx)
	if err != nil || len(objs) != 0 {
		t.Fatalf("List() of an empty store = %v, %v, want an empty list and nil", objs, err)
	}

	for _, name := range []string{"web", "api", "db"} {
		if _, err := s.Create(ctx, reconciler.Object{Name: name, Spec: spec{"name": name}}); err != nil {
			t.Fatalf("Create(%s) error = %v", name, err)
		}
	}
	objs, err = s.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	var got []string
	for _, obj := range objs {
		got = append(got, obj.Name)
		want := mustGet(t, s, obj.Name)
		if !maps.Equal(obj.Spec, want.Spec) || obj.ResourceVersion != want.ResourceVersion {
			t.Errorf("List() has %+v, but Get(%q) = %+v", obj, obj.Name, want)
		}
	}
	if len(got) != 3 || got[0] != "api" || got[1] != "db" || got[2] != "web" {
		t.Fatalf("List() names = %v, want [api db web] (sorted by name)", got)
	}
}

// TestMemoryStoreReturnsCopies checks that no map is shared between the
// store and its callers. A controller that changes an object it got from a
// shared cache corrupts the cache for everybody: client-go's docs warn about
// exactly this.
func TestMemoryStoreReturnsCopies(t *testing.T) {
	hangGuard(t)
	ctx := context.Background()
	want := spec{"replicas": "3"}
	expectStored := func(t *testing.T, s *reconciler.MemoryStore, after string) {
		t.Helper()
		if got := mustGet(t, s, "web"); !maps.Equal(got.Spec, want) {
			t.Fatalf("stored spec = %v after %s, want %v: the store must keep its own copy", got.Spec, after, want)
		}
	}

	t.Run("Create", func(t *testing.T) {
		s := reconciler.NewMemoryStore()
		in := spec{"replicas": "3"}
		out, err := s.Create(ctx, reconciler.Object{Name: "web", Spec: in})
		if err != nil {
			t.Fatalf("Create error = %v", err)
		}
		in["replicas"] = "100"
		expectStored(t, s, "changing the map passed to Create")
		out.Spec["replicas"] = "200"
		expectStored(t, s, "changing the Spec returned by Create")
	})

	t.Run("Get and List", func(t *testing.T) {
		s := reconciler.NewMemoryStore()
		seed(t, s, map[string]spec{"web": {"replicas": "3"}})
		got := mustGet(t, s, "web")
		got.Spec["replicas"] = "100"
		expectStored(t, s, "changing the Spec returned by Get")
		objs, err := s.List(ctx)
		if err != nil || len(objs) != 1 {
			t.Fatalf("List() = %v, %v, want 1 object", objs, err)
		}
		objs[0].Spec["replicas"] = "200"
		expectStored(t, s, "changing the Spec returned by List")
	})

	t.Run("Update", func(t *testing.T) {
		s := reconciler.NewMemoryStore()
		seed(t, s, map[string]spec{"web": {"replicas": "1"}})
		obj := mustGet(t, s, "web")
		obj.Spec["replicas"] = "3"
		out, err := s.Update(ctx, obj)
		if err != nil {
			t.Fatalf("Update error = %v", err)
		}
		obj.Spec["replicas"] = "100"
		expectStored(t, s, "changing the map passed to Update")
		out.Spec["replicas"] = "200"
		expectStored(t, s, "changing the Spec returned by Update")
	})
}

// TestMemoryStoreConcurrentUpdates runs 8 writers that each increment a
// counter 25 times with read-modify-write, retrying on ErrConflict. That is
// the optimistic concurrency loop, and it must lose no increment. Run it with
// -race.
func TestMemoryStoreConcurrentUpdates(t *testing.T) {
	hangGuard(t)
	ctx := context.Background()
	s := reconciler.NewMemoryStore()
	seed(t, s, map[string]spec{"counter": {"n": "0"}})
	const writers, increments = 8, 25

	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range increments {
				for {
					obj, err := s.Get(ctx, "counter")
					if err != nil {
						errs <- err
						return
					}
					n, _ := strconv.Atoi(obj.Spec["n"])
					obj.Spec["n"] = strconv.Itoa(n + 1)
					_, err = s.Update(ctx, obj)
					if err == nil {
						break
					}
					if !errors.Is(err, reconciler.ErrConflict) {
						errs <- err
						return
					}
					// Someone else was faster: read again and retry.
				}
			}
		}()
	}
	within(t, "the writers", wg.Wait)
	close(errs)
	for err := range errs {
		t.Fatalf("a writer got error %v, want only nil or ErrConflict", err)
	}
	if got := mustGet(t, s, "counter").Spec["n"]; got != strconv.Itoa(writers*increments) {
		t.Fatalf("counter = %s after %d increments, want %d: updates were lost", got, writers*increments, writers*increments)
	}
}
