package techmark

import "strings"

// InternalLink describes a wiki link and the exact byte range of its target.
// Byte offsets let analyzers update a target without reparsing or changing the
// display label.
type InternalLink struct {
	Start       int
	End         int
	TargetStart int
	TargetEnd   int
	Target      string
	Label       string
	HasLabel    bool
}

// InternalLinks extracts TechMark links without treating fenced or inline code
// examples as document relationships. This is the shared structural boundary
// for graph analysis and bot suggestions.
func InternalLinks(source string) []InternalLink {
	links := make([]InternalLink, 0)
	inFence := false
	lineStart := 0
	for lineStart <= len(source) {
		lineEnd := strings.IndexByte(source[lineStart:], '\n')
		if lineEnd < 0 {
			lineEnd = len(source)
		} else {
			lineEnd += lineStart
		}
		line := source[lineStart:lineEnd]
		if codeFencePattern.MatchString(strings.TrimSpace(line)) {
			inFence = !inFence
		} else if !inFence {
			links = append(links, internalLinksInLine(line, lineStart)...)
		}
		if lineEnd == len(source) {
			break
		}
		lineStart = lineEnd + 1
	}
	return links
}

// InternalLinkTargets returns normalized targets for graph analysis.
func InternalLinkTargets(source string) []string {
	links := InternalLinks(source)
	targets := make([]string, 0, len(links))
	for _, link := range links {
		targets = append(targets, link.Target)
	}
	return targets
}

func internalLinksInLine(line string, base int) []InternalLink {
	links := make([]InternalLink, 0)
	for position := 0; position < len(line); {
		if line[position] == '`' {
			run := countByteRun(line, position, '`')
			closing := findByteRun(line, position+run, '`', run)
			if closing < 0 {
				break
			}
			position = closing + run
			continue
		}
		if position+1 >= len(line) || line[position] != '[' || line[position+1] != '[' {
			position++
			continue
		}
		closing := strings.Index(line[position+2:], "]]")
		if closing < 0 {
			position += 2
			continue
		}
		end := position + 2 + closing + 2
		innerStart := position + 2
		innerEnd := end - 2
		inner := line[innerStart:innerEnd]
		separator := strings.IndexByte(inner, '|')
		targetRaw := inner
		label := ""
		hasLabel := separator >= 0
		if hasLabel {
			targetRaw = inner[:separator]
			label = strings.TrimSpace(inner[separator+1:])
			if label == "" {
				position = end
				continue
			}
		}
		trimmedTarget := strings.TrimSpace(targetRaw)
		target := WikiLinkTargetName(trimmedTarget)
		if target != "" {
			leading := strings.Index(targetRaw, trimmedTarget)
			if leading < 0 {
				position = end
				continue
			}
			links = append(links, InternalLink{
				Start:       base + position,
				End:         base + end,
				TargetStart: base + innerStart + leading,
				TargetEnd:   base + innerStart + leading + len(trimmedTarget),
				Target:      target,
				Label:       label,
				HasLabel:    hasLabel,
			})
		}
		position = end
	}
	return links
}

func countByteRun(value string, start int, target byte) int {
	end := start
	for end < len(value) && value[end] == target {
		end++
	}
	return end - start
}

func findByteRun(value string, start int, target byte, length int) int {
	for position := start; position < len(value); position++ {
		if value[position] == target && countByteRun(value, position, target) == length {
			return position
		}
	}
	return -1
}

func nonCodeSegments(line string) []string {
	segments := make([]string, 0, 2)
	position := 0
	for position < len(line) {
		opening := strings.IndexByte(line[position:], '`')
		if opening < 0 {
			segments = append(segments, line[position:])
			break
		}
		opening += position
		if opening > position {
			segments = append(segments, line[position:opening])
		}
		run := countByteRun(line, opening, '`')
		closing := findByteRun(line, opening+run, '`', run)
		if closing < 0 {
			break
		}
		position = closing + run
	}
	return segments
}
