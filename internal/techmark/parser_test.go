package techmark

import (
	"strings"
	"testing"
)

func TestRenderExtendedBlocks(t *testing.T) {
	parser := NewParser()
	html := parser.Render(`# Linux

**Linux**는 [[Operating System|운영체제]] 커널입니다.

- 오픈소스
* 다양한 하드웨어 지원

#### 세부 항목

| 항목 | 내용 |
|------|------|
| 이름 | TechPedia |
| 문법 | TechMark |

` + "```go" + `
package main
` + "```" + `

<wiki-toc></wiki-toc>`)

	assertContains(t, html, "<h1>Linux</h1>")
	assertContains(t, html, `<strong>Linux</strong>`)
	assertContains(t, html, `<a class="wiki-link" href="/wiki/Operating_System">운영체제</a>`)
	assertContains(t, html, "<ul>")
	assertContains(t, html, "<li>오픈소스</li>")
	assertContains(t, html, "<li>다양한 하드웨어 지원</li>")
	assertContains(t, html, `<h4 id="section-1">세부 항목</h4>`)
	assertContains(t, html, "<table>")
	assertContains(t, html, "<th>항목</th>")
	assertContains(t, html, "<td>TechMark</td>")
	assertContains(t, html, `<pre class="code-block"><code class="syntax-chroma language-go">`)
	assertContains(t, html, `<span class="syntax-kn">package</span>`)
	assertNotContains(t, html, "wiki-toc")
}

func TestRenderHorizontalRuleWithoutChangingNavboxOrCodeSyntax(t *testing.T) {
	output := NewParser().Render(`첫 문단

---

둘째 문단

` + "```text" + `
---
` + "```" + `

<wiki-navbox title="계열">
- 상위:
-- 하위:
--- 세부: [[C]]
</wiki-navbox>`)

	assertContains(t, output, `<p>첫 문단</p>`)
	assertContains(t, output, `<hr class="techmark-divider">`)
	assertContains(t, output, `<p>둘째 문단</p>`)
	assertContains(t, output, `<pre class="code-block"><code class="syntax-chroma language-text">---</code></pre>`)
	assertContains(t, output, `<span class="wiki-navbox-label">세부</span>`)
	if strings.Count(output, `<hr class="techmark-divider">`) != 1 {
		t.Fatalf("only the standalone body separator must render as a horizontal rule\noutput:\n%s", output)
	}
}

func TestRenderAddsDocumentOrderHeadingAnchors(t *testing.T) {
	html := NewParser().Render("## First\n\n### Child\n\n## Second\n\n#### Detail")
	for _, expected := range []string{
		`<h2 id="section-1">First</h2>`,
		`<h3 id="section-2">Child</h3>`,
		`<h2 id="section-3">Second</h2>`,
		`<h4 id="section-4">Detail</h4>`,
	} {
		assertContains(t, html, expected)
	}
}

func TestSecondLevelSectionsAreCollapsible(t *testing.T) {
	html := NewParser().Render(`도입부

## 펼친 문단

첫 번째 내용

### 하위 문단

하위 내용

!## 접힌 문단

두 번째 내용

## 다시 펼친 문단

세 번째 내용`)

	assertContains(t, html, `<p>도입부</p>`)
	if strings.Count(html, `<div class="techmark-section-viewport">`) != 3 || strings.Count(html, `</div></details></div>`) != 3 {
		t.Fatalf("each collapsible section needs one independent viewport wrapper\noutput:\n%s", html)
	}
	assertContains(t, html, `<details class="techmark-section" open><summary><h2 id="section-1">펼친 문단</h2></summary><div class="techmark-section-content">`)
	assertContains(t, html, `<h3 id="section-2">하위 문단</h3>`)
	assertContains(t, html, `<details class="techmark-section"><summary><h2 id="section-3">접힌 문단</h2></summary><div class="techmark-section-content">`)
	assertContains(t, html, `<p>두 번째 내용</p>`)
	assertContains(t, html, `<details class="techmark-section" open><summary><h2 id="section-4">다시 펼친 문단</h2></summary><div class="techmark-section-content">`)
	assertContains(t, html, `<p>세 번째 내용</p>`)
	assertNotContains(t, html, `!##`)
}

func TestFootnotesRemainOutsideCollapsedSections(t *testing.T) {
	html := NewParser().Render(`!## 접힌 문단

본문<wiki-footnote>출처</wiki-footnote>`)
	sectionEnd := strings.Index(html, `</div></details>`)
	footnotes := strings.Index(html, `class="techmark-footnotes"`)
	if sectionEnd < 0 || footnotes < sectionEnd {
		t.Fatalf("footnotes must remain after the collapsible section\noutput:\n%s", html)
	}
}

func TestRenderEscapesUnsafeHTML(t *testing.T) {
	parser := NewParser()
	html := parser.Render(`<script>alert("x")</script>

<wiki-infobox title="Linux">
  <p onclick="evil()"><strong>분류:</strong> 운영체제 커널</p>
</wiki-infobox>

<wiki-alert type="success">저장되었습니다.</wiki-alert>`)

	assertContains(t, html, `&lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt;`)
	assertContains(t, html, `<wiki-infobox title="Linux">`)
	assertContains(t, html, `<p><strong>분류:</strong> 운영체제 커널</p>`)
	assertContains(t, html, `<wiki-alert type="success">`)
	assertContains(t, html, `</wiki-alert>`)

	if strings.Contains(html, "onclick") {
		t.Fatalf("expected unsafe HTML attributes to be stripped\noutput:\n%s", html)
	}
}

func TestWikiTagsUseExplicitTagAndAttributeAllowlist(t *testing.T) {
	output := NewParser().Render(`<wiki-alert type="warning" onclick="evil()" style="display:none">주의</wiki-alert>

<wiki-infobox title="C" type="language" onmouseover="evil()">
| 종류 = 언어
</wiki-infobox>

<wiki-unknown data-value="kept?" onclick="evil()">미지원</wiki-unknown>`)

	assertContains(t, output, `<wiki-alert type="warning"><p>주의</p>`)
	assertContains(t, output, `<wiki-infobox title="C" type="language">`)
	assertContains(t, output, `&lt;wiki-unknown data-value=&#34;kept?&#34; onclick=&#34;evil()&#34;&gt;`)
	assertContains(t, output, `&lt;/wiki-unknown&gt;`)
	assertNotContains(t, output, `<wiki-alert type="warning" onclick=`)
	assertNotContains(t, output, `<wiki-infobox title="C" type="language" onmouseover=`)
	assertNotContains(t, output, `<wiki-alert type="warning" style=`)
}

func TestRenderInfoboxRows(t *testing.T) {
	parser := NewParser()
	html := parser.Render(`<wiki-infobox title="C" type="language">
| 종류 = 프로그래밍 언어
| 설계자 = [[Dennis Ritchie]]
| 최초 공개 = 1972년
</wiki-infobox>`)

	assertContains(t, html, `<wiki-infobox title="C" type="language">`)
	assertContains(t, html, `<table class="wiki-infobox-rows">`)
	assertContains(t, html, `<tr><th scope="row">종류</th><td>프로그래밍 언어</td></tr>`)
	assertContains(t, html, `<tr><th scope="row">설계자</th><td><a class="wiki-link" href="/wiki/Dennis_Ritchie">Dennis Ritchie</a></td></tr>`)
	assertContains(t, html, `<tr><th scope="row">최초 공개</th><td>1972년</td></tr>`)
	assertContains(t, html, `</wiki-infobox>`)
}

func TestRenderNavboxGroupsAndSafeColors(t *testing.T) {
	output := NewParser().RenderWithLocale(`<wiki-navbox title="Linux 배포판" state="expanded" accent="#F5C32C">
- Debian {color="#AFD2DB" note="APT"}: [[Debian]], [[Knoppix]], [[Kali Linux]]
-- Ubuntu {color="#E95420"}: [[Ubuntu]], [[Linux Mint]], [[Kubuntu]]
- Fedora {color="#55A5D8" note="RPM"}: [[Fedora]], [[Amazon Linux]]
-- CentOS Stream {color="#A14F91"}: [[AlmaLinux]], [[Rocky Linux]]
- 기타: [[OpenWrt]], [[NixOS]]
</wiki-navbox>`, "ko")

	assertContains(t, output, `<wiki-navbox class="wiki-navbox" style="--navbox-accent:#F5C32C;--navbox-accent-text:#111111"><details class="wiki-navbox-details" open>`)
	assertContains(t, output, `<summary class="wiki-navbox-title"><span>Linux 배포판</span></summary>`)
	assertContains(t, output, `<th class="wiki-navbox-parent" scope="rowgroup" rowspan="2" style="--navbox-label-color:#AFD2DB;--navbox-label-text:#111111"><span class="wiki-navbox-label">Debian<small>APT</small></span></th>`)
	assertContains(t, output, `<th class="wiki-navbox-child" scope="row" style="--navbox-label-color:#E95420;--navbox-label-text:#111111"><span class="wiki-navbox-label">Ubuntu</span></th>`)
	assertContains(t, output, `<a class="wiki-link" href="/ko/wiki/Linux_Mint">Linux Mint</a>`)
	assertContains(t, output, `<th class="wiki-navbox-parent" scope="row"><span class="wiki-navbox-label">기타</span></th>`)
	assertNotContains(t, output, `<p>- Debian`)
}

func TestRenderNavboxDefaultsOpenAndSupportsEmptyParentRows(t *testing.T) {
	output := NewParser().Render(`<wiki-navbox title="계열">
- Debian:
-- Ubuntu: [[Ubuntu]]
-- 기타: [[Linux Mint]]
</wiki-navbox>`)

	assertContains(t, output, `<details class="wiki-navbox-details" open>`)
	assertContains(t, output, `<tr class="wiki-navbox-row wiki-navbox-child-row"><th class="wiki-navbox-parent" scope="rowgroup" rowspan="2">`)
	assertContains(t, output, `<th class="wiki-navbox-child" scope="row"><span class="wiki-navbox-label">Ubuntu</span></th>`)
}

func TestRenderNavboxSupportsThreeLevels(t *testing.T) {
	output := NewParser().RenderWithLocale(`<wiki-navbox title="Linux 배포판">
- Debian:
-- Ubuntu {color="#E95420"}:
--- Linux Mint {color="#87CF3E"}: [[Linux Mint]], [[LMDE]]
--- Kubuntu: [[Kubuntu]]
-- Knoppix: [[Knoppix]]
</wiki-navbox>`, "ko")

	assertContains(t, output, `<table class="wiki-navbox-table wiki-navbox-depth-3">`)
	assertContains(t, output, `<tr class="wiki-navbox-row wiki-navbox-grandchild-row"><th class="wiki-navbox-parent" scope="rowgroup" rowspan="3"><span class="wiki-navbox-label">Debian</span></th><th class="wiki-navbox-child" scope="rowgroup" rowspan="2" style="--navbox-label-color:#E95420;--navbox-label-text:#111111"><span class="wiki-navbox-label">Ubuntu</span></th><th class="wiki-navbox-grandchild" scope="row" style="--navbox-label-color:#87CF3E;--navbox-label-text:#111111"><span class="wiki-navbox-label">Linux Mint</span></th>`)
	assertContains(t, output, `<th class="wiki-navbox-grandchild" scope="row"><span class="wiki-navbox-label">Kubuntu</span></th><td class="wiki-navbox-content"><a class="wiki-link" href="/ko/wiki/Kubuntu">Kubuntu</a></td>`)
	assertContains(t, output, `<tr class="wiki-navbox-row wiki-navbox-child-row"><th class="wiki-navbox-child" scope="row"><span class="wiki-navbox-label">Knoppix</span></th><td class="wiki-navbox-content" colspan="2"><a class="wiki-link" href="/ko/wiki/Knoppix">Knoppix</a></td>`)
	assertNotContains(t, output, `<p>--- Linux Mint`)
}

func TestRenderNavboxPromotesOrphanThirdLevelToChild(t *testing.T) {
	output := NewParser().Render(`<wiki-navbox title="계열">
- Parent:
--- Orphan detail: [[C]]
</wiki-navbox>`)

	assertContains(t, output, `<th class="wiki-navbox-child" scope="row"><span class="wiki-navbox-label">Orphan detail</span></th>`)
	assertNotContains(t, output, `<th class="wiki-navbox-grandchild"`)
}

func TestRenderNavboxCollapsedAndRejectsUnsafeMetadata(t *testing.T) {
	output := NewParser().Render(`<wiki-navbox title="&lt;unsafe&gt;" state="collapsed" accent="red" style="background:url(x)" onclick="evil()">
- Group {color="var(--danger)" note="&lt;script&gt;" style="display:none"}: [[C]]
</wiki-navbox>`)

	assertContains(t, output, `<wiki-navbox class="wiki-navbox"><details class="wiki-navbox-details">`)
	assertContains(t, output, `<span>&amp;lt;unsafe&amp;gt;</span>`)
	assertContains(t, output, `<span class="wiki-navbox-label">Group<small>&amp;lt;script&amp;gt;</small></span>`)
	assertNotContains(t, output, `background:url`)
	assertNotContains(t, output, `onclick`)
	assertNotContains(t, output, `display:none`)
	assertNotContains(t, output, `--navbox-label-color`)
}

func TestRenderWikiTreeBuildsNestedVisualStructure(t *testing.T) {
	output := NewParser().RenderWithLocale(`<wiki-tree title="UNIX 역사 트리" state="expanded" accent="#DDEEFF">
[[Research UNIX]]
├── AT&T 상용 계통
│   ├── [[System III]]
│   └── [[System V]]
│       └── [[SVR4]]
│
├── BSD 계통
│   └── [[FreeBSD]]

독립적으로 구현된 유닉스 계열
├── [[Linux]]
└── [[MINIX]]
</wiki-tree>`, "ko")

	assertContains(t, output, `<wiki-tree class="wiki-visual wiki-tree" style="--wiki-visual-accent:#DDEEFF;--wiki-visual-accent-text:#111111"><details class="wiki-visual-details" open>`)
	assertContains(t, output, `<summary class="wiki-visual-title"><span>UNIX 역사 트리</span></summary>`)
	assertContains(t, output, `<li class="wiki-tree-item"><div class="wiki-tree-node"><a class="wiki-link" href="/ko/wiki/Research_UNIX">Research UNIX</a></div><ul>`)
	assertContains(t, output, `<div class="wiki-tree-node"><a class="wiki-link" href="/ko/wiki/SVR4">SVR4</a></div>`)
	assertContains(t, output, `<div class="wiki-tree-node">독립적으로 구현된 유닉스 계열</div><ul>`)
	assertNotContains(t, output, `<div class="wiki-tree-node">│</div>`)
	assertNotContains(t, output, `├──`)
}

func TestRenderWikiFlowTurnsArrowsIntoConnectors(t *testing.T) {
	output := NewParser().Render(`<wiki-flow title="자원 처리 흐름" state="collapsed">
자원 획득
↓
오류 확인
⇣
자원 사용
▼
모든 실행 경로에서 [[자원 반환|반환]]
</wiki-flow>`)

	assertContains(t, output, `<wiki-flow class="wiki-visual wiki-flow"><details class="wiki-visual-details"><summary class="wiki-visual-title"><span>자원 처리 흐름</span></summary>`)
	assertContains(t, output, `<li class="wiki-flow-step"><div class="wiki-flow-node">자원 획득</div><span class="wiki-flow-connector" aria-hidden="true"><span>↓</span></span></li>`)
	assertContains(t, output, `<div class="wiki-flow-node">모든 실행 경로에서 <a class="wiki-link" href="/wiki/자원_반환">반환</a></div></li>`)
	assertNotContains(t, output, `<div class="wiki-flow-node">↓</div>`)
}

func TestVisualRenderersEscapeContentAndDropUnsafeAttributes(t *testing.T) {
	output := NewParser().Render(`<wiki-tree title="Tree" accent="red" onclick="evil()">
Root
└── <script>alert(1)</script>
</wiki-tree>

<wiki-flow>
Start
↓
Finish
</wiki-flow>`)

	assertContains(t, output, `<wiki-tree class="wiki-visual wiki-tree">`)
	assertContains(t, output, `&lt;script&gt;alert(1)&lt;/script&gt;`)
	assertContains(t, output, `<wiki-flow class="wiki-visual wiki-flow"><div class="wiki-visual-body wiki-visual-untitled">`)
	assertNotContains(t, output, `onclick`)
	assertNotContains(t, output, `--wiki-visual-accent`)
}

func TestRenderWithLocaleUsesLocalizedWikiLinks(t *testing.T) {
	parser := NewParser()
	html := parser.RenderWithLocale(`See [[Operating System|operating systems]] and [[programming language]].`, "en")

	assertContains(t, html, `<a class="wiki-link" href="/en/wiki/Operating_System">operating systems</a>`)
	assertContains(t, html, `<a class="wiki-link" href="/en/wiki/Programming_language">programming language</a>`)
	assertContains(t, parser.RenderWithLocale(`[[Accueil]]`, "fr"), `<a class="wiki-link" href="/fr/wiki/Accueil">Accueil</a>`)
}

func TestRenderPreservesSafeLocalTimeElement(t *testing.T) {
	parser := NewParser()
	html := parser.Render(`- [[C++]] — <techpedia-time datetime="2026-07-14T01:02:03Z">2026-07-14 01:02 UTC</techpedia-time>`)

	assertContains(t, html, `<li><a class="wiki-link" href="/wiki/C++">C++</a> — <techpedia-time datetime="2026-07-14T01:02:03Z">2026-07-14 01:02 UTC</techpedia-time></li>`)
}

func TestRenderFourthLevelHeadingStartsANewBlock(t *testing.T) {
	parser := NewParser()
	html := parser.Render("본문 문장\n#### 세부 제목")

	assertContains(t, html, `<p>본문 문장</p>`+"\n"+`<h4 id="section-1">세부 제목</h4>`)
}

func TestRenderInlineCodeKeepsMarkupLiteral(t *testing.T) {
	parser := NewParser()
	html := parser.Render("`[[C|언어]] **강조** <script>`와 **설명**")

	assertContains(t, html, `<code>[[C|언어]] **강조** &lt;script&gt;</code>`)
	assertContains(t, html, `<strong>설명</strong>`)
	assertNotContains(t, html, `<a class="wiki-link"`)
}

func TestUnclosedInlineCodeDoesNotActivateMarkup(t *testing.T) {
	output := NewParser().Render("설명 `[[Hidden]] **강조** <wiki-alert type=\"danger\">")

	assertContains(t, output, "`[[Hidden]] **강조** &lt;wiki-alert type=&#34;danger&#34;&gt;")
	assertNotContains(t, output, `<a class="wiki-link"`)
	assertNotContains(t, output, `<strong>`)
	assertNotContains(t, output, `<wiki-alert`)
}

func TestInternalLinkEscapesAmpersandsExactlyOnce(t *testing.T) {
	output := NewParser().Render(`[[R&D|Research & Development]]`)

	assertContains(t, output, `<a class="wiki-link" href="/wiki/R&amp;D">Research &amp; Development</a>`)
	assertNotContains(t, output, `&amp;amp;`)
}

func TestRenderExternalLinkAndFootnotes(t *testing.T) {
	parser := NewParser()
	output := parser.RenderWithLocale("read(2)는 파일 디스크립터에서 데이터를 읽는다.<wiki-footnote>[man7.org](https://man7.org/linux/man-pages/man2/read.2.html)의 `read(2)` 설명</wiki-footnote>\n\n"+`같은 문서의 오류 규약도 참고한다.<wiki-footnote>[[C]]의 **오류 처리** 설명</wiki-footnote>`, "ko")

	assertContains(t, output, `<sup class="techmark-footnote-ref" id="footnote-ref-inline-1-1"><a href="#footnote-inline-1" role="doc-noteref">[1]</a></sup>`)
	assertContains(t, output, `<p>read(2)는 파일 디스크립터에서 데이터를 읽는다.<sup class="techmark-footnote-ref"`)
	assertContains(t, output, `<section class="techmark-footnotes" role="doc-endnotes"><ol>`)
	assertContains(t, output, `<li id="footnote-inline-1"><a class="external-link" href="https://man7.org/linux/man-pages/man2/read.2.html" target="_blank" rel="nofollow noopener noreferrer">man7.org</a>의 <code>read(2)</code> 설명`)
	assertContains(t, output, `<li id="footnote-inline-2"><a class="wiki-link" href="/ko/wiki/C">C</a>의 <strong>오류 처리</strong> 설명`)
	assertContains(t, output, `href="#footnote-ref-inline-2-1"`)
	assertNotContains(t, output, `wiki-footnote`)
}

func TestExternalLinksSupportUppercaseProtocolAndBalancedParentheses(t *testing.T) {
	output := NewParser().Render(`[C++ reference](HTTPS://en.wikipedia.org/wiki/C%2B%2B_(programming_language))`)

	assertContains(t, output, `<a class="external-link" href="HTTPS://en.wikipedia.org/wiki/C%2B%2B_(programming_language)" target="_blank" rel="nofollow noopener noreferrer">C++ reference</a>`)
	assertNotContains(t, output, `</a>)`)
	if got, want := Excerpt(`[C++ reference](HTTPS://en.wikipedia.org/wiki/C%2B%2B_(programming_language))`, "", 80), "C++ reference"; got != want {
		t.Fatalf("Excerpt() = %q, want %q", got, want)
	}
}

func TestInlineFootnoteMaySpanPhysicalLines(t *testing.T) {
	output := NewParser().RenderWithLocale(`스트롭스트룹은 Java와 C#의 제네릭이 C++ 템플릿과 정적 타입 컨테이너의 영향을 받았다고 설명했다.<wiki-footnote>
[Bjarne Stroustrup, Evolving a Language in and for the Real World: C++ 1991-2006](https://www.stroustrup.com/hopl2.pdf)
</wiki-footnote>`, "ko")

	assertContains(t, output, `<p>스트롭스트룹은 Java와 C#의 제네릭이 C++ 템플릿과 정적 타입 컨테이너의 영향을 받았다고 설명했다.<sup class="techmark-footnote-ref"`)
	assertContains(t, output, `<li id="footnote-inline-1"><a class="external-link" href="https://www.stroustrup.com/hopl2.pdf"`)
	assertContains(t, output, `Bjarne Stroustrup, Evolving a Language in and for the Real World: C++ 1991-2006</a>`)
	assertNotContains(t, output, `wiki-footnote`)
}

func TestInlineFootnoteDoesNotCrossCodeFence(t *testing.T) {
	output := NewParser().Render("본문<wiki-footnote>닫히지 않은 각주\n\n```techmark\n</wiki-footnote>\n```\n\n뒤 문장")

	assertNotContains(t, output, `class="techmark-footnote-ref"`)
	assertNotContains(t, output, `class="techmark-footnotes"`)
	assertContains(t, output, `&lt;/wiki-footnote&gt;`)
}

func TestFootnoteNumbersFollowFirstReferenceOrder(t *testing.T) {
	output := NewParser().Render(`두 번째 정의를 먼저 인용한다.[^second] 첫 번째 정의는 나중에 인용한다.[^first]

[^first]: 첫 번째 정의
[^second]: 두 번째 정의`)

	assertContains(t, output, `href="#footnote-second" role="doc-noteref">[1]</a>`)
	assertContains(t, output, `href="#footnote-first" role="doc-noteref">[2]</a>`)
	if strings.Index(output, `id="footnote-second"`) > strings.Index(output, `id="footnote-first"`) {
		t.Fatal("footnote list did not follow first-reference order")
	}
}

func TestFootnoteSyntaxIsLiteralInsideCode(t *testing.T) {
	output := NewParser().Render("`<wiki-footnote>inline</wiki-footnote>`\n\n```techmark\n<wiki-footnote>block</wiki-footnote>\n```\n\n`[^read]`\n\n[^read]: source")
	assertContains(t, output, `<code>&lt;wiki-footnote&gt;inline&lt;/wiki-footnote&gt;</code>`)
	assertContains(t, output, `&lt;wiki-footnote&gt;block&lt;/wiki-footnote&gt;`)
	assertContains(t, output, `<code>[^read]</code>`)
	assertNotContains(t, output, `class="techmark-footnote-ref"`)
}

func TestEmptyAndUnclosedFootnotesAreNotNumbered(t *testing.T) {
	output := NewParser().Render(`빈 각주<wiki-footnote></wiki-footnote>와 닫히지 않은 각주<wiki-footnote>출처`)
	assertNotContains(t, output, `class="techmark-footnote-ref"`)
	assertNotContains(t, output, `class="techmark-footnotes"`)
}

func TestRenderTableSupportsOptionalOuterPipesAndLiteralCellPipes(t *testing.T) {
	parser := NewParser()
	html := parser.Render(`항목 | 내용
--- | ---
언어 | [[C|C 언어]]
표현 | ` + "`left | right`" + `
`)

	assertContains(t, html, `<div class="wiki-table-scroll"><table>`)
	assertContains(t, html, `<td><a class="wiki-link" href="/wiki/C">C 언어</a></td>`)
	assertContains(t, html, `<td><code>left | right</code></td>`)
}

func TestRenderUnknownCodeLanguageFallsBackToEscapedCode(t *testing.T) {
	parser := NewParser()
	html := parser.Render("```not-a-language\n<script>alert(1)</script>\n```")

	assertContains(t, html, `<code class="syntax-chroma language-not-a-language">&lt;script&gt;alert(1)&lt;/script&gt;</code>`)
}

func TestRenderCodeFenceWithIDKeepsLanguageHighlighting(t *testing.T) {
	parser := NewParser()
	output := parser.Render("```c id=\"hfvq7t\"\nint main(void) { return 0; }\n```")

	assertContains(t, output, `<pre id="hfvq7t" class="code-block"><code class="syntax-chroma language-c">`)
	assertContains(t, output, `<span class="syntax-kt">int</span>`)
}

func TestObjectiveCPPCodeFenceKeepsPreprocessorLinesInsideCode(t *testing.T) {
	output := NewParser().Render("```objective-c++ id=\"cpp-language-relation-objcpp\"\n#import <Foundation/Foundation.h>\nstd::vector<int> values;\n```")

	assertContains(t, output, `<pre id="cpp-language-relation-objcpp" class="code-block"><code class="syntax-chroma language-objective-c++">`)
	assertContains(t, output, `<span class="syntax-cp">#import`)
	assertContains(t, output, `Foundation/Foundation.h`)
	assertNotContains(t, output, `<h1`)
	assertNotContains(t, output, "<p>```objective-c++")
}

func TestCodeFenceLanguageAllowsChromaPunctuationAliases(t *testing.T) {
	for _, language := range []string{"c++", "c#"} {
		output := NewParser().Render("```" + language + "\n#define VALUE 1\n```")
		assertContains(t, output, `class="syntax-chroma language-`+language+`"`)
		assertNotContains(t, output, `<h1`)
	}
}

func TestCodeFenceIDDoesNotExposeCodeAsTechMark(t *testing.T) {
	parser := NewParser()
	output := parser.Render("```techmark id=\"example.block-1\"\n[[Hidden Link]]\n<wiki-footnote>hidden</wiki-footnote>\n```\n\nVisible<wiki-footnote>shown</wiki-footnote>")

	assertContains(t, output, `<pre id="example.block-1" class="code-block">`)
	assertContains(t, output, `[[Hidden Link]]`)
	assertContains(t, output, `&lt;wiki-footnote&gt;hidden&lt;/wiki-footnote&gt;`)
	assertContains(t, output, `role="doc-noteref">[1]</a>`)
	assertNotContains(t, output, `href="/wiki/Hidden_Link"`)
}

func TestSpecialSyntaxDetectionIsLimitedToReservedTags(t *testing.T) {
	if !ContainsSpecialSyntax(`<wiki-recent-changes limit="20"></wiki-recent-changes>`) {
		t.Fatal("expected recent changes syntax to be reserved")
	}
	if !ContainsSpecialSyntax(`<wiki-random-page label="Open"></wiki-random-page>`) {
		t.Fatal("expected random page syntax to be reserved")
	}
	if !ContainsSpecialSyntax(`<wiki-featured-pages title="Today"></wiki-featured-pages>`) {
		t.Fatal("expected featured page syntax to be reserved")
	}
	if !ContainsHomePageSyntax(`<wiki-home-grid></wiki-home-grid>`) {
		t.Fatal("expected home grid syntax to be homepage-only")
	}
	if ContainsSpecialSyntax(`<wiki-infobox title="C"></wiki-infobox>`) {
		t.Fatal("public TechMark tags must not be treated as reserved syntax")
	}
	if ContainsSpecialSyntax(`<wiki-footnote>source</wiki-footnote>`) {
		t.Fatal("footnotes must remain available to regular editors")
	}
	if ContainsSpecialSyntax(`<wiki-navbox title="Languages"></wiki-navbox>`) {
		t.Fatal("navboxes must remain available to regular editors")
	}
	if ContainsSpecialSyntax(`<wiki-tree title="UNIX"></wiki-tree>`) || ContainsSpecialSyntax(`<wiki-flow title="Lifecycle"></wiki-flow>`) {
		t.Fatal("visual renderers must remain available to regular editors")
	}
	if stripped := StripSpecialSyntax(`before<wiki-random-page></wiki-random-page>after`); stripped != "beforeafter" {
		t.Fatalf("unexpected stripped source %q", stripped)
	}
}

func TestRenderWikiImageUsesGlobalMediaVariants(t *testing.T) {
	output := NewParser().RenderWithLocale(`<wiki-image file="Linux tree.png" layout="side" alt="Linux distribution tree">[[Linux]] 계통</wiki-image>`, "ko")

	assertContains(t, output, `<wiki-image class="wiki-image wiki-image-side">`)
	assertContains(t, output, `<button type="button" class="wiki-image-stage wiki-image-open" aria-haspopup="dialog">`)
	assertContains(t, output, `data-large-src="/media/files/Linux_tree.webp/large.webp"`)
	assertContains(t, output, `src="/media/files/Linux_tree.webp/article.webp"`)
	assertContains(t, output, `/media/files/Linux_tree.webp/thumb.webp 320w`)
	assertContains(t, output, `<span class="wiki-image-caption"><a class="wiki-link" href="/ko/wiki/Linux">Linux</a> 계통</span>`)
	assertNotContains(t, output, `from `)
}

func TestRenderWikiImageValidatesExternalHTTPSAndShowsOrigin(t *testing.T) {
	output := NewParser().Render(`<wiki-image src="https://images.example.com/linux.png" layout="wide" alt="Linux">caption</wiki-image>`)
	assertContains(t, output, `src="https://images.example.com/linux.png" referrerpolicy="no-referrer"`)
	assertContains(t, output, `data-large-src="https://images.example.com/linux.png"`)
	assertContains(t, output, `from <a href="https://images.example.com/linux.png"`)
	assertContains(t, output, `>images.example.com</a>`)

	unsafe := NewParser().Render(`<wiki-image src="http://example.com/image.png">caption</wiki-image>`)
	assertContains(t, unsafe, `Invalid external image URL`)
	assertNotContains(t, unsafe, `<img`)
}

func TestSpecialSyntaxIgnoresCodeExamples(t *testing.T) {
	codeOnly := "`<wiki-recent-changes></wiki-recent-changes>`\n\n```techmark\n<wiki-random-page></wiki-random-page>\n<wiki-home-grid></wiki-home-grid>\n```"
	if ContainsSpecialSyntax(codeOnly) {
		t.Fatal("reserved syntax inside code must not require administrator permissions")
	}
	if ContainsHomePageSyntax(codeOnly) {
		t.Fatal("home syntax inside code must not be treated as executable")
	}
	if stripped := StripSpecialSyntax(codeOnly); stripped != codeOnly {
		t.Fatalf("code example changed while stripping reserved syntax:\n%s", stripped)
	}

	source := codeOnly + "\n\n<wiki-recent-changes></wiki-recent-changes>\n<wiki-home-grid></wiki-home-grid>"
	if !ContainsSpecialSyntax(source) || !ContainsHomePageSyntax(source) {
		t.Fatal("reserved syntax outside code was not detected")
	}
	stripped := StripSpecialSyntax(source)
	assertContains(t, stripped, "```techmark\n<wiki-random-page></wiki-random-page>\n<wiki-home-grid></wiki-home-grid>\n```")
	assertNotContains(t, stripped, "\n\n<wiki-recent-changes></wiki-recent-changes>\n<wiki-home-grid></wiki-home-grid>")
}

func assertContains(t *testing.T, value string, expected string) {
	t.Helper()
	if !strings.Contains(value, expected) {
		t.Fatalf("expected rendered output to contain %q\noutput:\n%s", expected, value)
	}
}

func assertNotContains(t *testing.T, value string, unexpected string) {
	t.Helper()
	if strings.Contains(value, unexpected) {
		t.Fatalf("expected rendered output not to contain %q\noutput:\n%s", unexpected, value)
	}
}
