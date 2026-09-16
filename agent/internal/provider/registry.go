package provider

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

var ErrUnknownUnit = errors.New("unknown unit")

type Registry struct {
	list   []Provider
	byName map[string]Provider
}

func NewRegistry() *Registry {
	return &Registry{byName: map[string]Provider{}}
}

func (r *Registry) Add(p Provider) error {
	name := p.Name()
	if name == "" || strings.Contains(name, ":") {
		return fmt.Errorf("invalid provider name %q", name)
	}
	if _, dup := r.byName[name]; dup {
		return fmt.Errorf("provider %q registered twice", name)
	}
	r.list = append(r.list, p)
	r.byName[name] = p
	return nil
}

func (r *Registry) Providers() []Provider { return r.list }

// Resolve splits a full unit id into its provider and local id.
func (r *Registry) Resolve(id string) (Provider, string, error) {
	name, local, ok := strings.Cut(id, ":")
	if !ok || local == "" {
		return nil, "", ErrUnknownUnit
	}
	p, ok := r.byName[name]
	if !ok {
		return nil, "", ErrUnknownUnit
	}
	return p, local, nil
}

// ListAll queries every provider concurrently. A failing provider doesn't
// hide the others; its error is returned keyed by provider name.
func (r *Registry) ListAll(ctx context.Context) ([]Unit, map[string]string) {
	type result struct {
		idx   int
		units []Unit
		err   error
	}
	results := make([]result, len(r.list))
	var wg sync.WaitGroup
	for i, p := range r.list {
		wg.Add(1)
		go func() {
			defer wg.Done()
			units, err := p.List(ctx)
			results[i] = result{i, units, err}
		}()
	}
	wg.Wait()

	var all []Unit
	errs := map[string]string{}
	for _, res := range results {
		p := r.list[res.idx]
		if res.err != nil {
			errs[p.Name()] = res.err.Error()
			continue
		}
		sort.SliceStable(res.units, func(a, b int) bool {
			return strings.ToLower(res.units[a].Name) < strings.ToLower(res.units[b].Name)
		})
		for _, u := range res.units {
			u.ID = p.Name() + ":" + u.ID
			u.Provider = p.Name()
			if u.Health == "" {
				u.Health = HealthNone
			}
			if u.Actions == nil {
				u.Actions = []string{}
			}
			all = append(all, u)
		}
	}
	if all == nil {
		all = []Unit{}
	}
	return all, errs
}

func (r *Registry) Do(ctx context.Context, id, action string) error {
	if !ValidAction(action) {
		return ErrBadAction
	}
	p, local, err := r.Resolve(id)
	if err != nil {
		return err
	}
	return p.Do(ctx, local, action)
}

func (r *Registry) Logs(ctx context.Context, id string, tail int) (<-chan LogLine, error) {
	p, local, err := r.Resolve(id)
	if err != nil {
		return nil, err
	}
	return p.Logs(ctx, local, tail)
}
