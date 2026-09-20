package convert

import "testing"

func TestRewriteLinks(t *testing.T) {
	// The corpus: intro.md at root, tables/orders.md in a subdir.
	srcToOut := map[string]string{
		"intro.md":         "intro.md",
		"tables/orders.md": "tables/orders.md",
	}

	t.Run("relative link rewritten to bundle-absolute", func(t *testing.T) {
		body := "See the [orders](tables/orders.md) table.\n"
		out, links := rewriteLinks(body, "intro.md", srcToOut)
		if out != "See the [orders](/tables/orders.md) table.\n" {
			t.Fatalf("unexpected rewrite:\n%s", out)
		}
		if len(links) != 1 || !links[0].Resolved || links[0].TargetID != "tables/orders" {
			t.Fatalf("unexpected link: %+v", links)
		}
	})

	t.Run("parent-relative link resolved", func(t *testing.T) {
		body := "Back to [intro](../intro.md).\n"
		out, links := rewriteLinks(body, "tables/orders.md", srcToOut)
		if out != "Back to [intro](/intro.md).\n" {
			t.Fatalf("unexpected rewrite:\n%s", out)
		}
		if !links[0].Resolved {
			t.Fatalf("link should resolve: %+v", links[0])
		}
	})

	t.Run("anchor preserved on rewrite", func(t *testing.T) {
		body := "[schema](tables/orders.md#schema)\n"
		out, _ := rewriteLinks(body, "intro.md", srcToOut)
		if out != "[schema](/tables/orders.md#schema)\n" {
			t.Fatalf("anchor not preserved:\n%s", out)
		}
	})

	t.Run("broken link left untouched but reported", func(t *testing.T) {
		body := "[missing](nope.md)\n"
		out, links := rewriteLinks(body, "intro.md", srcToOut)
		if out != body {
			t.Fatalf("broken link must be left untouched, got:\n%s", out)
		}
		if len(links) != 1 || links[0].Resolved {
			t.Fatalf("broken link should be reported unresolved: %+v", links)
		}
	})

	t.Run("external and anchor-only links untouched and not edges", func(t *testing.T) {
		body := "[ext](https://example.com) and [top](#section) and [img](pic.png)\n"
		out, links := rewriteLinks(body, "intro.md", srcToOut)
		if out != body {
			t.Fatalf("non-md/external links must be untouched:\n%s", out)
		}
		if len(links) != 0 {
			t.Fatalf("external/anchor/non-md must not be edges: %+v", links)
		}
	})

	t.Run("bundle-absolute link kept and resolved", func(t *testing.T) {
		body := "[o](/tables/orders.md)\n"
		out, links := rewriteLinks(body, "guides/setup.md", srcToOut)
		if out != "[o](/tables/orders.md)\n" {
			t.Fatalf("absolute link should stay absolute:\n%s", out)
		}
		if !links[0].Resolved || links[0].TargetID != "tables/orders" {
			t.Fatalf("absolute link should resolve: %+v", links[0])
		}
	})
}
