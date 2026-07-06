package handlers

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSlugify_NormalName covers the common case: a normal org name
// slugifies to a predictable, lowercase, hyphenated slug.
func TestSlugify_NormalName(t *testing.T) {
	require.Equal(t, "acme-corp", slugify("Acme Corp"))
	require.Equal(t, "acme-corp", slugify("  Acme   Corp!! "))
	require.Equal(t, "foo-bar-42", slugify("Foo_Bar 42"))
}

// TestSlugify_PunctuationOnlyFallsBack covers the code-review finding: an
// org name with no alphanumeric characters at all (punctuation or emoji
// only) must not slugify to "", since "" would collide with every other
// such name against the tenants.slug UNIQUE constraint.
func TestSlugify_PunctuationOnlyFallsBack(t *testing.T) {
	for _, name := range []string{"!!!", "😀😀", "---", "   "} {
		got := slugify(name)
		require.NotEmpty(t, got, "slugify(%q) must not be empty", name)
		require.True(t, len(got) > len("tenant-"), "slugify(%q) = %q should include a random suffix", name, got)
	}
}

// TestSlugify_PunctuationOnlyNoCollision ensures two separate calls with
// punctuation-only names produce DIFFERENT fallback slugs, so a second
// such org can still be created without hitting the UNIQUE constraint.
func TestSlugify_PunctuationOnlyNoCollision(t *testing.T) {
	a := slugify("!!!")
	b := slugify("!!!")
	require.NotEqual(t, a, b, "two punctuation-only org names must not produce the same fallback slug")

	c := slugify("😀😀")
	require.NotEqual(t, a, c)
	require.NotEqual(t, b, c)
}
