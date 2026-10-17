// Command secret-refs parses and resolves "scheme:ref" secret references
// (package secret), the values a profile's `secrets:` map holds instead of
// the secrets themselves. It uses the env scheme with a fake lookup and a
// custom Resolver, so no gopass and no real environment is touched.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"

	"github.com/khanakia/agx/secret"
)

// fakeEnv is the environment the System resolver sees.
var fakeEnv = map[string]string{"KIMI_TOKEN": "fake-kimi-token"}

// vault is a stand-in store for the custom resolver below.
var vault = map[string]string{"team/claude": "fake-vault-token"}

// vaultResolver shows that any backend fits: implement Resolve and pass it
// to ResolveAll. Here it answers gopass references from a map.
type vaultResolver struct{}

// Resolve implements secret.Resolver.
func (vaultResolver) Resolve(_ context.Context, ref secret.Ref) (string, error) {
	if ref.Scheme != secret.SchemeGopass {
		return "", fmt.Errorf("vault: cannot resolve %s references", ref.Scheme)
	}
	v, ok := vault[ref.Path]
	if !ok {
		return "", fmt.Errorf("vault: %s not found", ref.Path)
	}
	return v, nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	for _, raw := range []string{"gopass:team/claude", "env:KIMI_TOKEN", "KIMI_TOKEN", "vault:x"} {
		ref, err := secret.Parse(raw)
		if err != nil {
			fmt.Printf("parse %-20q error: %v\n", raw, err)
			continue
		}
		fmt.Printf("parse %-20q scheme=%s path=%s\n", raw, ref.Scheme, ref.Path)
	}

	ctx := context.Background()
	sys := secret.System{LookupEnv: func(k string) (string, bool) { v, ok := fakeEnv[k]; return v, ok }}
	vals, err := secret.ResolveAll(ctx, sys, map[string]string{"ANTHROPIC_AUTH_TOKEN": "env:KIMI_TOKEN"})
	if err != nil {
		return err
	}
	printLengths("system", vals)

	vals, err = secret.ResolveAll(ctx, vaultResolver{}, map[string]string{"ANTHROPIC_API_KEY": "gopass:team/claude"})
	if err != nil {
		return err
	}
	printLengths("custom", vals)

	// Errors name the variable and the reference, never a value.
	_, err = secret.ResolveAll(ctx, sys, map[string]string{"ANTHROPIC_AUTH_TOKEN": "env:MISSING_TOKEN"})
	fmt.Printf("unset: %v\n", err)
	empty := secret.System{LookupEnv: func(string) (string, bool) { return "", true }}
	_, err = secret.ResolveAll(ctx, empty, map[string]string{"ANTHROPIC_AUTH_TOKEN": "env:KIMI_TOKEN"})
	fmt.Printf("empty: %v (empty=%v)\n", err, errors.Is(err, secret.ErrEmpty))
	return nil
}

// printLengths prints each resolved variable with its value's length only.
func printLengths(label string, vals map[string]string) {
	names := make([]string, 0, len(vals))
	for k := range vals {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		fmt.Printf("%s: %s resolved (%d chars)\n", label, k, len(vals[k]))
	}
}
