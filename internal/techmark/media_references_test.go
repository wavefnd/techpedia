package techmark

import "testing"

func TestUsesMediaFileFollowsRenderedImagesNotCodeExamples(t *testing.T) {
	p := NewParser()
	for _, test := range []struct {
		source, name string
		want         bool
	}{
		{`<wiki-image file="A.png">caption</wiki-image>`, "A.webp", true},
		{`<wiki-image file="A.webp,B.webp"></wiki-image>`, "b.webp", true},
		{`<wiki-infobox><wiki-image file="사진\,one.webp"></wiki-image></wiki-infobox>`, "사진,one.webp", true},
		{`<wiki-image src="https://wiki.example/media/files/A.webp/large.webp"></wiki-image>`, "A.webp", true},
		{"`<wiki-image file=\"A.webp\"></wiki-image>`", "A.webp", false},
		{"```techmark\n<wiki-image file=\"A.webp\"></wiki-image>\n```", "A.webp", false},
		{"A.webp is only a filename in text", "A.webp", false},
		{`<wiki-image file="Other.webp">A.webp</wiki-image>`, "A.webp", false},
	} {
		if got := p.UsesMediaFile(test.source, test.name); got != test.want {
			t.Fatalf("%q: got %v, want %v", test.source, got, test.want)
		}
	}
}
