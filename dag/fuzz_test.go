package dag

import (
	"strconv"
	"testing"
)

// ParseTaskURI decides whether a cross-repository dependency is well formed.
// A dependency reference reaches it from task metadata, which any participant
// with filesystem access can write, so it parses input the process did not
// produce itself.
//
// The properties fuzzed here are the ones the rest of the system relies on: it
// must not panic, and a successful parse must round-trip to the same URI. A
// parse that accepted a URI but returned a different alias or ID than the text
// carried would resolve a dependency against the wrong task.
func FuzzParseTaskURI(f *testing.F) {
	for _, seed := range []string{
		"todo://local/1",
		"todo://upstream/42",
		"todo://a.b_c-d/7",
		"",
		"todo://",
		"todo:///1",
		"todo://local/",
		"todo://local/0",
		"todo://local/-1",
		"todo://local/1/2",
		"todo://loc al/1",
		"todo://local/99999999999999999999999",
		"todo://local/1\n",
		"TODO://local/1",
		"todo://local/+1",
		"todo://local/0x1",
		"todo://ünïcode/1",
		"local/1",
		"1",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, uri string) {
		alias, id, err := ParseTaskURI(uri)
		if err != nil {
			// A rejected URI must not report a usable reference.
			if alias != "" || id != 0 {
				t.Fatalf("ParseTaskURI(%q) failed but returned %q/%d", uri, alias, id)
			}
			return
		}

		if alias == "" {
			t.Fatalf("ParseTaskURI(%q) accepted an empty alias", uri)
		}
		if id == 0 {
			t.Fatalf("ParseTaskURI(%q) accepted task ID zero", uri)
		}
		for _, r := range alias {
			ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
				(r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-'
			if !ok {
				t.Fatalf("ParseTaskURI(%q) accepted alias %q containing %q", uri, alias, r)
			}
		}

		// The parsed parts must describe the same task the text carried.
		// Re-parsing the canonical form must land on the same reference.
		//
		// This is deliberately not an equality check against the input: a
		// leading zero such as todo://local/01 is accepted and normalizes to
		// todo://local/1. Callers that store or compare references must
		// canonicalize first, because two spellings of one edge compared as
		// strings become two edges.
		rebuilt := "todo://" + alias + "/" + strconv.FormatUint(id, 10)
		rebuiltAlias, rebuiltID, err := ParseTaskURI(rebuilt)
		if err != nil {
			t.Fatalf("ParseTaskURI(%q) produced unparseable canonical form %q: %v", uri, rebuilt, err)
		}
		if rebuiltAlias != alias || rebuiltID != id {
			t.Fatalf("ParseTaskURI(%q) is not idempotent: %q/%d then %q/%d",
				uri, alias, id, rebuiltAlias, rebuiltID)
		}
	})
}

// ParseLocalID chooses between a local numeric dependency and a URI. Getting
// that choice wrong routes a dependency to the wrong resolver, so it must
// never panic and must never report a local ID of zero.
func FuzzParseLocalID(f *testing.F) {
	for _, seed := range []string{
		"1", "0", "-1", "", "todo://local/1", "todo://remote/1",
		"18446744073709551615", "18446744073709551616",
		" 1", "1 ", "+1", "1.0", "0b1", "٣",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, value string) {
		id, local := ParseLocalID(value)
		if !local {
			return
		}
		if id == 0 {
			t.Fatalf("ParseLocalID(%q) reported local task ID zero", value)
		}
		// A local reference must name the same task whether it arrived as a
		// bare number or as a todo://local URI, so re-parsing the canonical
		// spelling must land on the same task.
		canonical := "todo://local/" + strconv.FormatUint(id, 10)
		canonicalID, canonicalLocal := ParseLocalID(canonical)
		if !canonicalLocal || canonicalID != id {
			t.Fatalf("ParseLocalID(%q) is not idempotent through %q", value, canonical)
		}
	})
}
