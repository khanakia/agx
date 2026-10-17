// Command autopick runs agx's auto-pick policy (package pick) over a fixed
// set of candidates: the profile whose busiest window is the least used
// wins, api-billed profiles and failed fetches are skipped with a reason,
// and an empty field is an error that lists every reason.
//
// pick is pure, so the usage figures are written inline; in agx they come
// from each provider's Usage call.
package main

import (
	"errors"
	"fmt"
	"log"

	"github.com/khanakia/agx/pick"
	"github.com/khanakia/agx/provider"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cands := []pick.Candidate{
		// personal: the 5-hour window is nearly free, but the weekly one is
		// at 77%, and any window at its cap blocks work — so it scores 77.
		{Profile: profile("personal", provider.BillingPlan), Usage: usage(4, 77)},
		{Profile: profile("work", provider.BillingPlan), Usage: usage(5, 12)},
		// kimi is billed per token: never chosen, whatever its usage says.
		{Profile: profile("kimi", provider.BillingAPI)},
		// A failed fetch makes a candidate ineligible; the error is kept.
		{Profile: profile("spare", provider.BillingPlan), Err: provider.ErrNotLoggedIn},
	}

	res, err := pick.Best(cands)
	if err != nil {
		return err
	}
	fmt.Println(res.Explain())
	fmt.Printf("chosen=%s score=%.0f\n", res.Chosen.Name, res.Score)
	for _, c := range cands {
		if reason, ok := res.Skipped[c.Profile.Name]; ok {
			fmt.Printf("skipped %s: %v\n", c.Profile.Name, reason)
		}
	}

	// Ties keep candidate (config) order: "a" and "b" both score 40.
	tie, err := pick.Best([]pick.Candidate{
		{Profile: profile("a", provider.BillingPlan), Usage: usage(40, 10)},
		{Profile: profile("b", provider.BillingPlan), Usage: usage(10, 40)},
	})
	if err != nil {
		return err
	}
	fmt.Println(tie.Explain())

	// Nobody eligible: ErrNoCandidate, with every reason in the message.
	_, err = pick.Best(cands[2:])
	fmt.Printf("none eligible: %v (no-candidate=%v)\n", err, errors.Is(err, pick.ErrNoCandidate))
	return nil
}

// profile builds a minimal Claude profile; pick reads only Name and Billing.
func profile(name string, billing provider.Billing) provider.Profile {
	return provider.Profile{Name: name, Provider: provider.Claude, Billing: billing}
}

// usage builds a snapshot with a session and a weekly window.
func usage(session, weekly float64) provider.Usage {
	return provider.Usage{Windows: []provider.Window{
		{Kind: provider.WindowSession, Group: provider.GroupSession, Label: "Current session", Percent: session},
		{Kind: provider.WindowWeekly, Group: provider.GroupWeekly, Label: "All models", Percent: weekly},
	}}
}
