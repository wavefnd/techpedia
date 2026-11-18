package techmark

import (
	"strings"
	"testing"
)

func TestInfoboxImagesPreserveRowsCaptionsAndMediaVariants(t *testing.T) {
	output := NewParser().RenderWithLocale(`<wiki-infobox title="Linux">
| 종류 = [[운영체제 커널]]
<wiki-image file="Linux logo.png" layout="side" alt="Tux">
**마스코트** [[Linux|리눅스]]
| 캡션 = 정보 행이 아님
</wiki-image>
| 공개 = 1991년
<wiki-image file="Linux screenshot.webp" layout="wide" alt="화면"></wiki-image>
설명 문단
</wiki-infobox>`, "ko")
	assertContains(t, output, `<tr><th scope="row">종류</th><td><a class="wiki-link" href="/ko/wiki/운영체제_커널">운영체제 커널</a></td></tr>`)
	assertContains(t, output, `<tr><th scope="row">공개</th><td>1991년</td></tr>`)
	assertContains(t, output, `<strong>마스코트</strong> <a class="wiki-link" href="/ko/wiki/Linux">리눅스</a>`)
	assertContains(t, output, `| 캡션 = 정보 행이 아님`)
	assertNotContains(t, output, `<th scope="row">캡션</th>`)
	assertContains(t, output, `src="/media/files/Linux_logo.webp/article.webp"`)
	assertContains(t, output, `/media/files/Linux_logo.webp/thumb.webp 320w`)
	assertContains(t, output, `data-large-src="/media/files/Linux_logo.webp/large.webp"`)
	assertContains(t, output, `sizes="(max-width: 720px) calc(100vw - 56px), 274px"`)
	assertContains(t, output, `loading="lazy" decoding="async"`)
	assertContains(t, output, `<p>설명 문단</p>`)
	if strings.Count(output, `<wiki-image class="wiki-image wiki-image-infobox">`) != 2 {
		t.Fatalf("expected two infobox images:\n%s", output)
	}
	if !(strings.Index(output, `>종류</th>`) < strings.Index(output, `<wiki-image`) && strings.Index(output, `<wiki-image`) < strings.Index(output, `>공개</th>`)) {
		t.Fatalf("image and information row order changed:\n%s", output)
	}
	assertNotContains(t, output, `wiki-image-side`)
	assertNotContains(t, output, `wiki-image-wide`)
}

func TestInfoboxExternalImageReusesSafetyAndAttribution(t *testing.T) {
	output := NewParser().Render(`<wiki-infobox title="Photo"><wiki-image src="https://images.example.com/a.png?x=1&y=2" alt="A & B" onclick="evil()" style="width:9999px"><script>evil()</script> caption</wiki-image></wiki-infobox>`)
	assertContains(t, output, `<img alt="A &amp; B" loading="lazy" decoding="async"`)
	assertContains(t, output, `src="https://images.example.com/a.png?x=1&amp;y=2" referrerpolicy="no-referrer"`)
	assertContains(t, output, `>images.example.com</a>`)
	assertContains(t, output, `&lt;script&gt;evil()&lt;/script&gt; caption`)
	assertNotContains(t, output, `onclick=`)
	assertNotContains(t, output, `style=`)
	assertNotContains(t, output, `<script>`)
}

func TestInfoboxImagesRejectInvalidSources(t *testing.T) {
	for _, attributes := range []string{
		`src="http://example.com/a.png"`, `src="javascript:alert(1)"`,
		`src="data:image/png;base64,a"`, `src="https://user:password@example.com/a.png"`,
		`src="//example.com/a.png"`, `file="../../secret.png"`,
		`file="a.webp" src="https://example.com/a.png"`, `alt="empty"`,
	} {
		t.Run(attributes, func(t *testing.T) {
			output := NewParser().Render(`<wiki-infobox><wiki-image ` + attributes + `>caption</wiki-image>` + "\n| Name = Safe\n</wiki-infobox>")
			assertContains(t, output, `wiki-image-error`)
			assertNotContains(t, output, `<img`)
			assertContains(t, output, `<th scope="row">Name</th><td>Safe</td>`)
		})
	}
}

func TestInfoboxImageCodeExamplesAndMissingCloseRemainInert(t *testing.T) {
	for _, body := range []string{
		"`<wiki-image file=\"a.webp\">literal</wiki-image>`",
		"```techmark\n<wiki-image file=\"a.webp\">literal</wiki-image>\n```",
		`<wiki-image file="a.webp">unclosed`,
	} {
		output := NewParser().Render("<wiki-infobox>\n" + body + "\n| Name = Safe\n</wiki-infobox>")
		assertNotContains(t, output, `<img`)
		assertContains(t, output, `&lt;wiki-image`)
		assertContains(t, output, `<th scope="row">Name</th><td>Safe</td>`)
	}
}

func TestInfoboxDoesNotChangeOrdinaryImages(t *testing.T) {
	for _, locale := range []string{"ko", "en", "ar"} {
		t.Run(locale, func(t *testing.T) {
			output := NewParser().RenderWithLocale(`<wiki-infobox><wiki-image file="a.webp" alt="داخل"></wiki-image></wiki-infobox>
<wiki-image file="b.webp" layout="side">[[Page]]</wiki-image>`, locale)
			assertContains(t, output, `wiki-image-infobox`)
			assertContains(t, output, `wiki-image-side`)
			assertContains(t, output, `sizes="(max-width: 720px) 100vw, 440px"`)
			assertContains(t, output, `href="/`+locale+`/wiki/Page"`)
		})
	}
}
