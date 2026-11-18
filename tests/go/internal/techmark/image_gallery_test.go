package techmark

import (
	"net/url"
	"strings"
	"testing"
)

func TestWikiImageFileListPreservesOrderDuplicatesAndCaption(t *testing.T) {
	output := NewParser().RenderWithLocale(`<wiki-image file=" LinuxMint.webp, Linux Mint MATE.png, LinuxMint.webp " layout="side" alt="Linux Mint desktops">**Linux Mint** [[Cinnamon]]</wiki-image>`, "en")
	assertContains(t, output, `<wiki-image class="wiki-image wiki-image-side"><figure><div class="wiki-image-gallery" dir="ltr">`)
	assertContains(t, output, `class="wiki-image-previous" aria-label="Previous image"`)
	assertContains(t, output, `class="wiki-image-next" aria-label="Next image"`)
	assertContains(t, output, `aria-atomic="true">1 / 3</output>`)
	assertContains(t, output, `<strong>Linux Mint</strong> <a class="wiki-link" href="/en/wiki/Cinnamon">Cinnamon</a>`)
	for _, pair := range []struct {
		fragment string
		count    int
	}{
		{`<img `, 3}, {`<figure>`, 1}, {`<figcaption>`, 1},
		{`class="wiki-image-stage wiki-image-open"`, 3},
		{`alt="Linux Mint desktops" loading="lazy" decoding="async"`, 3},
		{`src="/media/files/LinuxMint.webp/article.webp"`, 2},
		{`sizes="(max-width: 720px) 100vw, 440px"`, 3},
	} {
		if got := strings.Count(output, pair.fragment); got != pair.count {
			t.Fatalf("%q appeared %d times, want %d:\n%s", pair.fragment, got, pair.count, output)
		}
	}
	a := strings.Index(output, `src="/media/files/LinuxMint.webp/article.webp"`)
	b := strings.Index(output, `src="/media/files/Linux_Mint_MATE.webp/article.webp"`)
	c := strings.LastIndex(output, `src="/media/files/LinuxMint.webp/article.webp"`)
	if !(a >= 0 && a < b && b < c) {
		t.Fatalf("file order changed:\n%s", output)
	}
}

func TestWikiImageSingleFileMarkupUnchanged(t *testing.T) {
	output := NewParser().Render(`<wiki-image file="LinuxMint.webp" layout="side" alt="Mint">Desktop</wiki-image>`)
	want := `<wiki-image class="wiki-image wiki-image-side"><figure><button type="button" class="wiki-image-stage wiki-image-open" aria-haspopup="dialog"><img alt="Mint" loading="lazy" decoding="async" data-large-src="/media/files/LinuxMint.webp/large.webp" src="/media/files/LinuxMint.webp/article.webp" srcset="/media/files/LinuxMint.webp/thumb.webp 320w, /media/files/LinuxMint.webp/article.webp 960w, /media/files/LinuxMint.webp/large.webp 1920w" sizes="(max-width: 720px) 100vw, 440px"></button><figcaption><span class="wiki-image-caption">Desktop</span></figcaption></figure></wiki-image>` + "\n"
	if output != want {
		t.Fatalf("single image markup changed:\ngot %s\nwant %s", output, want)
	}
}

func TestWikiImageFileListSupportsInfoboxAndOtherLayouts(t *testing.T) {
	for _, locale := range []string{"ko", "en", "ar"} {
		for _, layout := range []string{"frame", "wide", "side"} {
			t.Run(locale+"/"+layout, func(t *testing.T) {
				image := `<wiki-image file="사진.webp, ثانية.webp" layout="` + layout + `">[[Page]]</wiki-image>`
				output := NewParser().RenderWithLocale(image, locale)
				assertContains(t, output, `class="wiki-image wiki-image-`+layout+`"`)
				assertContains(t, output, `href="/`+locale+`/wiki/Page"`)
				output = NewParser().RenderWithLocale("<wiki-infobox>\n"+image+"\n| Name = Value\n</wiki-infobox>", locale)
				assertContains(t, output, `class="wiki-image wiki-image-infobox"`)
				assertContains(t, output, `class="wiki-image-gallery"`)
				assertContains(t, output, `<th scope="row">Name</th><td>Value</td>`)
				if strings.Count(output, `sizes="(max-width: 720px) calc(100vw - 56px), 274px"`) != 2 {
					t.Fatalf("infobox responsive sizing changed:\n%s", output)
				}
			})
		}
	}
}

func TestWikiImageFileListRejectsEntireInvalidList(t *testing.T) {
	for _, files := range []string{",a.webp", "a.webp,", "a.webp,,b.webp", "a.webp,  ,b.webp", "a.webp,../private.webp", `a.webp,b\c.webp`, "a.webp,b\x00.webp", `a.webp,https://example.com/b.webp`} {
		t.Run(files, func(t *testing.T) {
			output := NewParser().Render(`<wiki-image file="` + files + `">caption</wiki-image>`)
			assertContains(t, output, `Invalid media filename`)
			assertNotContains(t, output, `<img`)
			assertNotContains(t, output, `<figure`)
		})
	}
	output := NewParser().Render(`<wiki-image file="a.webp,b.webp" src="https://example.com/image.webp">caption</wiki-image>`)
	assertContains(t, output, `Invalid image source`)
	assertNotContains(t, output, `<img`)
}

func TestWikiImageLiteralCommaAndExternalURLComma(t *testing.T) {
	output := NewParser().Render(`<wiki-image file="Linux\,Mint.webp,Other\,Photo.webp"></wiki-image>`)
	for _, name := range []string{"Linux,Mint.webp", "Other,Photo.webp"} {
		assertContains(t, output, `src="/media/files/`+url.PathEscape(name)+`/article.webp"`)
	}
	output = NewParser().Render(`<wiki-image file="Linux\,Mint.webp"></wiki-image>`)
	assertNotContains(t, output, `wiki-image-gallery`)
	output = NewParser().Render(`<wiki-image src="https://example.com/a,b.webp?q=x,y">source</wiki-image>`)
	assertNotContains(t, output, `wiki-image-gallery`)
	assertContains(t, output, `src="https://example.com/a,b.webp?q=x,y"`)
	assertContains(t, output, `>example.com</a>`)
}

func TestWikiImageFileListExamplesAndUnsafeAttributes(t *testing.T) {
	markup := `<wiki-image file="a.webp,b.webp">example</wiki-image>`
	for _, source := range []string{"`" + markup + "`", "```techmark\n" + markup + "\n```"} {
		output := NewParser().Render(source)
		assertNotContains(t, output, `<img`)
		assertContains(t, output, `&lt;wiki-image`)
	}
	output := NewParser().Render(`<wiki-image file="a.webp,b.webp" alt="A & B" onclick="evil()" style="display:none"><script>evil()</script></wiki-image>`)
	assertNotContains(t, output, `onclick=`)
	assertNotContains(t, output, `style=`)
	assertNotContains(t, output, `<script>`)
	assertContains(t, output, `&lt;script&gt;evil()&lt;/script&gt;`)
	if strings.Count(output, `alt="A &amp; B"`) != 2 {
		t.Fatal("each photo must use the safely escaped shared alt text")
	}
}
