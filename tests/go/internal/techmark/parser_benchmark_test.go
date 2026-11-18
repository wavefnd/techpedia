package techmark

import (
	"strings"
	"testing"
)

func BenchmarkRenderLargeMixedDocument(b *testing.B) {
	section := `## Parser section

TechPedia renders [[C++]] and [external references](https://example.com/reference_(parser)).<wiki-footnote>[source](https://example.com/source)</wiki-footnote>

| Syntax | Result |
| --- | --- |
| ` + "`#include`" + ` | code |

` + "```objective-c++" + `
#import <Foundation/Foundation.h>
` + "```" + `

`
	source := strings.Repeat(section, 250)
	parser := NewParser()
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		_ = parser.render(source, "/ko/wiki/")
	}
}
