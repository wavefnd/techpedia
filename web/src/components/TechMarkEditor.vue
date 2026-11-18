<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { basicSetup } from 'codemirror'
import { autocompletion, snippetCompletion, type CompletionContext, type CompletionResult } from '@codemirror/autocomplete'
import { markdown } from '@codemirror/lang-markdown'
import { Compartment, EditorState, RangeSetBuilder } from '@codemirror/state'
import { Decoration, EditorView, ViewPlugin, type DecorationSet, type ViewUpdate } from '@codemirror/view'
import { ensureCJKCodeFont } from '../services/fonts'

const props = defineProps<{ modelValue: string; readonly?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const host = ref<HTMLElement | null>(null)
let editor: EditorView | null = null
const editable = new Compartment()
const editability = () => [EditorState.readOnly.of(Boolean(props.readonly)), EditorView.editable.of(!props.readonly)]

const techmarkOptions = [
  snippetCompletion('<wiki-box name="${박스 이름}" />', { label: '<wiki-box>', detail: '등록한 공용 박스 · 문서 언어로 표시', type: 'keyword' }),
  snippetCompletion('[[${문서명}|${표시 이름}]]', { label: '[[문서명|표시 이름]]', detail: '내부 문서 링크', type: 'keyword' }),
  snippetCompletion('<wiki-infobox title="${제목}">\n  <p><strong>${항목}:</strong> ${값}</p>\n</wiki-infobox>', { label: '<wiki-infobox>', detail: '정보상자', type: 'keyword' }),
  snippetCompletion('<wiki-navbox title="${제목}" state="expanded" accent="#F5C32C">\n- ${상위 분류} {color="#AFD2DB" note="${보조 표기}"}: [[${문서명}]], [[${문서명}]]\n-- ${하위 분류} {color="#E95420"}: [[${문서명}]], [[${문서명}]]\n--- ${세부 분류} {color="#F4A261"}: [[${문서명}]], [[${문서명}]]\n</wiki-navbox>', { label: '<wiki-navbox>', detail: '둘러보기 상자 · 하이픈 수로 4단계 이상 확장', type: 'keyword' }),
  snippetCompletion('<wiki-tree title="${제목}" state="expanded">\n${루트}\n├── ${하위 항목}\n│   └── ${세부 항목}\n└── ${하위 항목}\n</wiki-tree>', { label: '<wiki-tree>', detail: '계층 구조 시각화', type: 'keyword' }),
  snippetCompletion('<wiki-flow title="${제목}" state="expanded">\n${첫 단계}\n↓\n${다음 단계}\n↓\n${마지막 단계}\n</wiki-flow>', { label: '<wiki-flow>', detail: '실행 흐름 시각화', type: 'keyword' }),
  snippetCompletion('<wiki-image file="${파일.webp}" layout="frame" alt="${대체 텍스트}">${캡션}</wiki-image>', { label: '<wiki-image>', detail: '이미지', type: 'keyword' }),
  snippetCompletion('<wiki-alert type="${note}">${내용}</wiki-alert>', { label: '<wiki-alert>', detail: '알림 상자', type: 'keyword' }),
  snippetCompletion('```${언어}\n${코드}\n```', { label: '``` 코드 블록', detail: '코드 블록', type: 'keyword' }),
  snippetCompletion('```${언어} id="${식별자}"\n${코드}\n```', { label: '``` 코드 블록 + id', detail: '앵커가 있는 코드 블록', type: 'keyword' }),
  snippetCompletion('`${코드}`', { label: '`인라인 코드`', detail: '인라인 코드', type: 'keyword' }),
  snippetCompletion('[${표시 이름}](https://${주소})', { label: '[이름](https://주소)', detail: '외부 링크', type: 'keyword' }),
  snippetCompletion('<wiki-footnote>[${출처}](https://${주소})</wiki-footnote>', { label: '<wiki-footnote>', detail: '문서 각주', type: 'keyword' }),
  snippetCompletion('| ${항목} | ${내용} |\n| --- | --- |\n| ${값} | ${설명} |', { label: '| 표 |', detail: '표', type: 'keyword' }),
  snippetCompletion('---', { label: '---', detail: '문서 구분선', type: 'keyword' }),
  snippetCompletion('## ${문단 제목}', { label: '## 문단 제목', detail: '2단계 제목', type: 'keyword' }),
  snippetCompletion('!## ${접힌 문단 제목}', { label: '!## 접힌 문단', detail: '기본으로 접힌 2단계 문단', type: 'keyword' }),
  snippetCompletion('**${강조할 내용}**', { label: '**굵게**', detail: '강조', type: 'keyword' }),
]

function techmarkCompletion(context: CompletionContext): CompletionResult | null {
  const token = context.matchBefore(/(?:\[\[[^\]]*|<wiki-[^>\s]*|`{1,3}[^\s]*|!#{2}|#{1,6})$/)
  if (!context.explicit && !token) return null
  return {
    from: token?.from ?? context.pos,
    options: techmarkOptions,
  }
}

function buildDecorations(view: EditorView): DecorationSet {
  const builder = new RangeSetBuilder<Decoration>()
  const pattern = /\[\[[^\]]+\]\]|!##\s+[^\n]+|<\/?wiki-(?:box|infobox|image|navbox|tree|flow|alert|footnote)(?:\s[^>]*)?>|<\/?wiki-(?:recent-changes|random-page|home-grid|featured-pages|popular-pages)(?:\s[^>]*)?>/g
  for (const range of view.visibleRanges) {
    const text = view.state.doc.sliceString(range.from, range.to)
    pattern.lastIndex = 0
    for (let match = pattern.exec(text); match; match = pattern.exec(text)) {
      const from = range.from + match.index
      const to = from + match[0].length
      const className = match[0].startsWith('[[')
        ? 'cm-techmark-link'
        : match[0].startsWith('!##')
          ? 'cm-techmark-section'
        : /wiki-(?:recent-changes|random-page|home-grid|featured-pages|popular-pages)/.test(match[0])
          ? 'cm-techmark-private'
          : 'cm-techmark-tag'
      builder.add(from, to, Decoration.mark({ class: className }))
    }
  }
  return builder.finish()
}

const techmarkHighlight = ViewPlugin.fromClass(class {
  decorations: DecorationSet

  constructor(view: EditorView) {
    this.decorations = buildDecorations(view)
  }

  update(update: ViewUpdate) {
    if (update.docChanged || update.viewportChanged) {
      this.decorations = buildDecorations(update.view)
    }
  }
}, {
  decorations: (value) => value.decorations,
})

onMounted(() => {
  if (!host.value) return
  ensureCJKCodeFont(props.modelValue)
  editor = new EditorView({
    parent: host.value,
    doc: props.modelValue,
    extensions: [
      basicSetup,
      editable.of(editability()),
      markdown(),
      techmarkHighlight,
      autocompletion({ override: [techmarkCompletion], activateOnTyping: true }),
      EditorView.lineWrapping,
      EditorView.updateListener.of((update) => {
        if (update.docChanged) emit('update:modelValue', update.state.doc.toString())
      }),
      EditorView.theme({
        '&': { height: '100%', fontSize: '14px' },
        '.cm-scroller': { fontFamily: 'var(--font-code)', lineHeight: '1.65' },
        '.cm-content': { padding: '16px 0' },
        '.cm-techmark-section': { color: 'var(--accent)', fontWeight: '650' },
        '.cm-techmark-private, .cm-techmark-private *': { color: 'inherit !important', fontWeight: 'inherit !important' },
        '&.cm-focused': { outline: 'none' },
      }),
    ],
  })
})

watch(() => props.modelValue, (value) => {
  ensureCJKCodeFont(value)
  if (!editor || editor.state.doc.toString() === value) return
  editor.dispatch({
    changes: { from: 0, to: editor.state.doc.length, insert: value },
  })
})

watch(() => props.readonly, () => editor?.dispatch({ effects: editable.reconfigure(editability()) }))
onBeforeUnmount(() => editor?.destroy())

function insert(value: string) {
  if (!editor || props.readonly) return
  const range = editor.state.selection.main
  const prefix = range.from > 0 && editor.state.doc.sliceString(range.from - 1, range.from) !== '\n' ? '\n' : ''
  const suffix = range.to < editor.state.doc.length && editor.state.doc.sliceString(range.to, range.to + 1) !== '\n' ? '\n' : ''
  const text = prefix + value + suffix
  editor.dispatch({
    changes: { from: range.from, to: range.to, insert: text },
    selection: { anchor: range.from + text.length },
    scrollIntoView: true,
  })
  editor.focus()
}

defineExpose({ insert })
</script>

<template>
  <div ref="host" class="techmark-editor" aria-label="TechMark 편집기"></div>
</template>
