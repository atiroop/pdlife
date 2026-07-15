package main

import "testing"

func TestParseSourceFlag(t *testing.T) {
	cases := []struct {
		input      string
		wantINMU   bool
		wantAnamai bool
		wantErr    bool
	}{
		{"all", true, true, false},
		{"inmu", true, false, false},
		{"anamai", false, true, false},
		{"", false, false, true},
		{"INMU", false, false, true}, // case-sensitive on purpose — no silent typo tolerance for a flag that controls which network calls fire
		{"both", false, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			gotINMU, gotAnamai, err := parseSourceFlag(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseSourceFlag(%q) expected an error, got none", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSourceFlag(%q) unexpected error: %v", tc.input, err)
			}
			if gotINMU != tc.wantINMU || gotAnamai != tc.wantAnamai {
				t.Fatalf("parseSourceFlag(%q) = (%v, %v), want (%v, %v)", tc.input, gotINMU, gotAnamai, tc.wantINMU, tc.wantAnamai)
			}
		})
	}
}

func TestComputeHash(t *testing.T) {
	a := computeHash([]string{"1", "2", "3"})
	b := computeHash([]string{"1", "2", "3"})
	if a != b {
		t.Fatalf("computeHash not deterministic: %q vs %q", a, b)
	}
	if len(a) != 64 {
		t.Fatalf("expected a 64-char hex SHA-256 digest, got %d chars: %q", len(a), a)
	}

	c := computeHash([]string{"1", "2", "4"}) // one id changed
	if a == c {
		t.Fatal("expected hash to change when the id set changes")
	}

	// Order matters unless the caller sorts first (main() always sorts
	// before calling this) — assert that expectation explicitly so a
	// future refactor that forgets to sort fails loudly here instead of
	// producing spurious diffs in production.
	unsorted := computeHash([]string{"3", "1", "2"})
	sorted := computeHash([]string{"1", "2", "3"})
	if unsorted == sorted {
		t.Fatal("expected computeHash to be order-sensitive (caller is responsible for sorting first)")
	}
}

func TestINMUIDPattern(t *testing.T) {
	// A real fragment captured from https://inmu.mahidol.ac.th/thaifcd/foodsearch/food_group_result
	// (food_group_id=70, page_no=1) on 2026-07-08.
	html := `<a href="food_name/?dbcode=STD&amp;food_group_id=70&amp;id=129&amp;name=Rice, brown, germinated, raw">Rice, brown, germinated, raw</a>
<a href="food_name/?dbcode=STD&amp;food_group_id=70&amp;id=130&amp;name=Rice, brown, germinated, steamed">Rice, brown, germinated, steamed</a>`

	matches := inmuIDPattern.FindAllStringSubmatch(html, -1)
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d: %v", len(matches), matches)
	}
	got := []string{matches[0][1], matches[1][1]}
	want := []string{"129", "130"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("match %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestAnamaiFidPattern(t *testing.T) {
	html := `<a href="view.php?fID=05021">Banana</a> <a href="view_branded.php?fID=R010068">Salted peanuts</a>`

	matches := anamaiFidPattern.FindAllStringSubmatch(html, -1)
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d: %v", len(matches), matches)
	}
	if matches[0][1] != "05021" || matches[1][1] != "R010068" {
		t.Fatalf("unexpected matches: %v", matches)
	}
}

func TestParseRobotsDisallow(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []string
	}{
		{"empty body means no restrictions", "", nil},
		{
			"wildcard group disallow rules collected",
			"User-agent: *\nDisallow: /admin\nDisallow: /private\n",
			[]string{"/admin", "/private"},
		},
		{
			"rules under a non-wildcard UA are ignored",
			"User-agent: Googlebot\nDisallow: /admin\nUser-agent: *\nDisallow: /nss/private\n",
			[]string{"/nss/private"},
		},
		{
			"comments and blank lines ignored",
			"# comment\n\nUser-agent: *\n# another comment\nDisallow: /x\n",
			[]string{"/x"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseRobotsDisallow(tc.body)
			if len(got) != len(tc.want) {
				t.Fatalf("parseRobotsDisallow(%q) = %v, want %v", tc.body, got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("index %d: got %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}
