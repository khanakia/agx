// Command sessions-gc creates session folders the way `agx new` names them,
// lists them, plans garbage collection (empty, no history, old enough),
// removes what the plan allows, and promotes one folder into a project.
//
// Whether a folder has conversation history is a callback, so this program
// answers it from a fixed set; agx asks every provider's HasHistory.
package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/khanakia/agx/sessions"
)

// minAge is the gc safety margin: a folder created within it is kept even
// when empty, because its agent may not have written anything yet.
const minAge = 24 * time.Hour

// filePerm is the mode of the one file written into a folder.
const filePerm os.FileMode = 0o644

// day is the spacing between the fake folders' creation stamps.
const day = 24 * time.Hour

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	tmp, err := os.MkdirTemp("", "agx-example-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	root := filepath.Join(tmp, "sessions")

	// Creation time is part of the name, so it is fixed here; gc reads it
	// back from the name, not from the folder's mtime.
	base := time.Date(2026, 10, 1, 9, 0, 0, 0, time.Local)
	now := base.Add(5 * day)

	if _, err := sessions.Create(root, sessions.Slug("Try", "the new API!"), base); err != nil {
		return err
	}
	withFile, err := sessions.Create(root, "notes", base.Add(day))
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(withFile, "plan.md"), []byte("# plan\n"), filePerm); err != nil {
		return err
	}
	withHistory, err := sessions.Create(root, "chat", base.Add(2*day))
	if err != nil {
		return err
	}
	if _, err := sessions.Create(root, "", now.Add(-time.Hour)); err != nil { // too new
		return err
	}
	// Two creates in the same second get a numeric suffix, never a shared folder.
	twin, err := sessions.Create(root, "chat", base.Add(2*day))
	if err != nil {
		return err
	}
	fmt.Printf("same-second twin: %s\n", filepath.Base(twin))

	hasHistory := func(dir string) bool { return dir == withHistory }
	folders, err := sessions.List(root, hasHistory)
	if err != nil {
		return err
	}
	plan := sessions.PlanGC(folders, now, minAge)
	for _, it := range plan {
		fmt.Printf("  %-32s files=%d remove=%-5v %s\n", it.Folder.Name, it.Folder.Files, it.Remove, it.Reason)
	}
	for _, it := range plan {
		if !it.Remove {
			continue
		}
		if err := sessions.RemoveEmpty(it.Folder.Path); err != nil {
			return err
		}
		fmt.Printf("removed %s\n", it.Folder.Name)
	}

	// RemoveEmpty re-checks at delete time: a folder that gained a file
	// after planning is refused, not deleted.
	err = sessions.RemoveEmpty(withFile)
	fmt.Printf("remove non-empty: refused=%v\n", err != nil)

	// Promote is one rename on the same disk and never overwrites.
	dst := filepath.Join(tmp, "projects", "notes-app")
	if err := sessions.Promote(withFile, dst); err != nil {
		return err
	}
	fmt.Printf("promoted %s → projects/%s\n", filepath.Base(withFile), filepath.Base(dst))
	again, err := sessions.Create(root, "notes", now)
	if err != nil {
		return err
	}
	err = sessions.Promote(again, dst)
	fmt.Printf("promote onto existing: exists=%v\n", errors.Is(err, sessions.ErrExists))
	return nil
}
