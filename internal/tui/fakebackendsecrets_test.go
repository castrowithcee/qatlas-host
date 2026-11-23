package tui

import (
	"context"
	"slices"

	"github.com/castrowithcee/qatlas-host/internal/control"
)

func (f *fakeBackend) Secrets(ctx context.Context, stack string) (control.SecretListResult, error) {
	f.count("secrets")
	f.noteEngine(ctx, "secrets", stack)
	if err := f.fail("secrets", stack); err != nil {
		return control.SecretListResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return control.SecretListResult{Stack: stack, Keys: slices.Clone(f.secrets[stack])}, nil
}

func (f *fakeBackend) GrantList(ctx context.Context, stack string) (control.GrantListResult, error) {
	f.count("grantlist")
	f.noteEngine(ctx, "grantlist", stack)
	if err := f.fail("grantlist", stack); err != nil {
		return control.GrantListResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return control.GrantListResult{Grants: slices.Clone(f.grants[stack]), Problem: f.grantProblem}, nil
}

func (f *fakeBackend) SecretSet(ctx context.Context, stack, key, value string) (control.SecretResult, error) {
	if err := f.actCtx(ctx, "secretset", stack+"/"+key); err != nil {
		return control.SecretResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.secretSets = append(f.secretSets, control.SecretSetArgs{Stack: stack, Key: key, Value: value})
	replaced := slices.Contains(f.secrets[stack], key)
	if !replaced {
		if f.secrets == nil {
			f.secrets = map[string][]string{}
		}
		f.secrets[stack] = append(f.secrets[stack], key)
		slices.Sort(f.secrets[stack])
	}
	return control.SecretResult{Stack: stack, Key: key, Replaced: replaced}, nil
}

func (f *fakeBackend) SecretDelete(ctx context.Context, stack, key string) (control.SecretResult, error) {
	if err := f.actCtx(ctx, "secretdelete", stack+"/"+key); err != nil {
		return control.SecretResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.secrets[stack] = slices.DeleteFunc(f.secrets[stack], func(k string) bool { return k == key })
	return control.SecretResult{Stack: stack, Key: key}, nil
}
