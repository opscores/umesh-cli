package nodeconfig

import "testing"

func TestMergePeerID(t *testing.T) {
	cases := []struct {
		name    string
		current string
		newID   string
		want    string
	}{
		{"empty current", "", "abc", "abc"},
		{"append", "abc,def", "ghi", "abc,def,ghi"},
		{"dedupe exact", "abc,def", "abc", "abc,def"},
		{"dedupe with spaces", "abc, def", "abc", "abc,def"},
		{"trim and sort", "z,y,a", "b", "a,b,y,z"},
		{"ignore empty", "abc,,def", "ghi", "abc,def,ghi"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MergePeerID(tc.current, tc.newID)
			if got != tc.want {
				t.Fatalf("MergePeerID(%q,%q) = %q, want %q", tc.current, tc.newID, got, tc.want)
			}
		})
	}
}

func TestSetAndGetPath(t *testing.T) {
	tree := NewTree("", nil)
	if err := tree.Parse([]byte("[p2p]\nseeds = \"\"\n")); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := setPath(tree.root, splitPath("p2p.seeds"), "\"s1,s2\""); err != nil {
		t.Fatalf("set: %v", err)
	}
	if got := tree.GetString("p2p.seeds", ""); got != `"s1,s2"` {
		t.Fatalf("GetString p2p.seeds = %q", got)
	}

	// A path through an existing string scalar must fail.
	if err := setPath(tree.root, splitPath("p2p.seeds.extra"), "x"); err == nil {
		t.Fatal("expected PathError traversing through scalar, got nil")
	}
}

func TestNormalizeExternalAddress(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"whitespace only", "  ", ""},
		{"bare ipv4 gets default port", "1.2.3.4", "1.2.3.4:26656"},
		{"hostname gets default port", "node.example.com", "node.example.com:26656"},
		{"ipv4 with port", "1.2.3.4:26656", "1.2.3.4:26656"},
		{"non-default port preserved", "1.2.3.4:26657", "1.2.3.4:26657"},
		{"tcp:// scheme stripped", "tcp://1.2.3.4:26656", "1.2.3.4:26656"},
		{"tcp:// scheme stripped, default port", "tcp://1.2.3.4", "1.2.3.4:26656"},
		{"ipv6 gets bracketed", "2001:db8::1", "[2001:db8::1]:26656"},
		{"bracketed ipv6 preserved", "[2001:db8::1]:26656", "[2001:db8::1]:26656"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeExternalAddress(tc.in); got != tc.want {
				t.Fatalf("NormalizeExternalAddress(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
