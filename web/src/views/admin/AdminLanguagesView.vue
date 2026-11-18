<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { Plus, Trash2 } from '@lucide/vue'
import { activateLanguageEdition, approveLanguageEdition, getCountryCatalog, proposeLanguageEdition, rejectLanguageEdition, translateLanguageInterface, updateLanguageInterface, updateLanguageNavigation, updateLanguagePortal } from '../../services/api'
import { languageEditions, loadLanguageEditions } from '../../services/editions'
import { allWikiMessageKeys, wikiMessageKeys } from '../../i18n'
import type { CountryCatalogEntry, EditionNavigationSection, LanguageEdition, WikiMessageKey } from '../../types/api'

type Form = {
  mainPageName: string
  source: string
  messages: Record<WikiMessageKey, string>
  navigation: EditionNavigationSection[]
  countryCodes: string[]
  countryQuery: string
  busy: boolean
  saved: string
  error: string
}

const forms = reactive<Record<string, Form>>({})
const error = ref('')
const notice = ref('')
const proposalTag = ref('')
const proposalBusy = ref(false)
const reviewBusy = ref('')
const tab = ref<'active' | 'proposals'>('active')
const countries = ref<CountryCatalogEntry[]>([])
const activeEditions = computed(() => languageEditions.value.filter((value) => value.status === 'active'))
const pendingEditions = computed(() => languageEditions.value.filter((value) => value.status === 'pending'))
const approvedEditions = computed(() => languageEditions.value.filter((value) => value.status === 'approved'))
const proposalCount = computed(() => pendingEditions.value.length + approvedEditions.value.length)
const primaryMessageGroups: Array<{ title: string; keys: WikiMessageKey[] }> = [
  { title: '브랜드와 검색', keys: ['tagline', 'searchLabel', 'searchPlaceholder', 'search', 'noResults'] },
  { title: '계정 메뉴', keys: ['menu', 'close', 'userActions', 'account', 'adminPanel', 'logout', 'login', 'signup', 'theme'] },
  { title: '공통 레이아웃', keys: ['navigation', 'languages', 'addLanguage', 'sidebarNote'] },
  { title: '문서 화면', keys: ['toc', 'section', 'document', 'read', 'discussion', 'edit', 'history', 'contentLicense', 'updated', 'lines', 'characters', 'scrollTop', 'scrollBottom', 'backlinks', 'noBacklinks'] },
  { title: '그래프 뷰', keys: ['graph', 'graphAll', 'graphLanguage', 'graphSearch', 'graphLoading', 'graphEmpty', 'graphReset', 'graphDocuments', 'graphLinks', 'graphTruncated', 'graphLayoutStart', 'graphLayoutPause', 'graphLayoutRunning', 'graphLayoutStable', 'graphLayoutPaused', 'graphLayoutFailed'] },
]
const primaryMessageKeys = new Set(primaryMessageGroups.flatMap((group) => group.keys))
const messageGroups: Array<{ title: string; keys: WikiMessageKey[] }> = [
  ...primaryMessageGroups,
  { title: '역사와 편집기', keys: allWikiMessageKeys.filter((key) => !primaryMessageKeys.has(key)) },
]

const renderedMessageKeys = messageGroups.flatMap((group) => group.keys)
if (renderedMessageKeys.length !== allWikiMessageKeys.length || new Set(renderedMessageKeys).size !== allWikiMessageKeys.length) {
  throw new Error('관리자 레이아웃 번역 필드와 필수 메시지 키가 일치하지 않습니다.')
}

const messageLabels: Record<WikiMessageKey, string> = {
  menu: '메뉴', close: '닫기', tagline: '로고 설명', searchLabel: '검색창 접근성 이름', searchPlaceholder: '검색창 안내 문구', search: '검색 버튼', noResults: '검색 결과 없음',
  userActions: '사용자 메뉴 접근성 이름', account: '내 계정', adminPanel: '관리자 패널', logout: '로그아웃', login: '로그인', signup: '회원가입', theme: '테마 전환', navigation: '둘러보기 접근성 이름',
  languages: '언어', addLanguage: '언어 추가', sidebarNote: '사이드바 설명', toc: '목차', section: '문단', document: '문서', read: '읽기', discussion: '토론', edit: '편집', history: '역사', contentLicense: '콘텐츠 라이선스', updated: '최근 수정', lines: '줄 수 단위', characters: '글자 수 단위', scrollTop: '페이지 맨 위로', scrollBottom: '페이지 맨 아래로', backlinks: '이 문서를 가리키는 문서', noBacklinks: '역링크 없음 안내',
  graph: '그래프 뷰', graphAll: '전체 언어 그래프', graphLanguage: '현재 언어 그래프', graphSearch: '그래프 문서 검색', graphLoading: '그래프 불러오는 중', graphEmpty: '그래프 문서 없음', graphReset: '그래프 화면 초기화', graphDocuments: '문서 수 단위', graphLinks: '연결 수 단위', graphTruncated: '일부 문서만 표시 안내',
  graphLayoutStart: '움직임 이어가기', graphLayoutPause: '움직임 일시정지', graphLayoutRunning: '진행 안내', graphLayoutStable: '움직임 안정화 안내', graphLayoutPaused: '정지 안내', graphLayoutFailed: '계산 불가 안내',
  historyTitle: '문서 역사 제목', currentArticle: '현재 문서', noSummary: '편집 요약 없음', noRevisions: '리비전 없음', selectRevision: '리비전 선택 안내', loadingHistory: '역사 불러오는 중', historyError: '역사 불러오기 오류', revisionError: '리비전 불러오기 오류',
  editor: '편집기 제목', cancel: '취소', save: '문서 저장', saving: '저장 중', loadingSource: '원문 불러오는 중', documentName: '문서 이름', editSummary: '편집 요약', summaryPlaceholder: '편집 요약 안내 문구', source: '원문', autocomplete: '자동 완성 안내', livePreview: '실시간 미리보기', serverRendering: '서버 렌더링 안내', saved: '저장 완료', sourceError: '원문 불러오기 오류', previewError: '미리보기 오류', saveError: '저장 오류',
}

function fallbackMessages(edition: LanguageEdition): Record<WikiMessageKey, string> {
  return Object.fromEntries(allWikiMessageKeys.map((key) => [key, edition.interface_messages[key] || ''])) as Record<WikiMessageKey, string>
}

function configuredRequiredMessages(form: Form): number {
  return wikiMessageKeys.filter((key) => form.messages[key].trim()).length
}

function cloneNavigation(value: EditionNavigationSection[]): EditionNavigationSection[] {
  return value.map((section) => ({ title: section.title, items: section.items.map((item) => ({ ...item })) }))
}

function ensureForm(edition: LanguageEdition) {
  if (forms[edition.tag]) return
  forms[edition.tag] = {
    mainPageName: edition.main_page_name || '',
    source: '',
    messages: fallbackMessages(edition),
    navigation: cloneNavigation(edition.navigation || []),
    countryCodes: [...(edition.country_codes || [])],
    countryQuery: '',
    busy: false,
    saved: '',
    error: '',
  }
}

function countryLabel(country: CountryCatalogEntry): string {
  try { return new Intl.DisplayNames(['ko'], { type: 'region' }).of(country.code) || country.name } catch { return country.name }
}

function matchingCountries(form: Form): CountryCatalogEntry[] {
  const query = form.countryQuery.trim().toLocaleLowerCase('ko')
  if (!query) return countries.value
  return countries.value.filter((country) => [country.code, country.name, countryLabel(country)].some((value) => value.toLocaleLowerCase('ko').includes(query)))
}

function toggleCountry(form: Form, code: string) {
  const index = form.countryCodes.indexOf(code)
  if (index >= 0) form.countryCodes.splice(index, 1)
  else form.countryCodes.push(code)
}

async function savePortal(edition: LanguageEdition) {
  const form = forms[edition.tag]
  form.busy = true; form.error = ''; form.saved = ''
  try {
    await updateLanguagePortal(edition.tag, form.countryCodes, edition.portal_order)
    form.saved = '언어 포털 국가 연결을 저장했습니다.'
    await loadLanguageEditions(true)
  } catch (reason) { form.error = reason instanceof Error ? reason.message : '언어 포털 설정을 저장하지 못했습니다.' } finally { form.busy = false }
}

watch(languageEditions, (values) => values.forEach(ensureForm), { immediate: true })

function addSection(edition: LanguageEdition) {
  forms[edition.tag].navigation.push({ title: '', items: [{ label: '', target: '' }] })
}

function addItem(edition: LanguageEdition, sectionIndex: number) {
  forms[edition.tag].navigation[sectionIndex].items.push({ label: '', target: '' })
}

function removeSection(edition: LanguageEdition, sectionIndex: number) {
  forms[edition.tag].navigation.splice(sectionIndex, 1)
}

function removeItem(edition: LanguageEdition, sectionIndex: number, itemIndex: number) {
  const section = forms[edition.tag].navigation[sectionIndex]
  if (section.items.length > 1) section.items.splice(itemIndex, 1)
}

async function saveInterface(edition: LanguageEdition) {
  const form = forms[edition.tag]
  form.busy = true; form.error = ''; form.saved = ''
  try {
    await updateLanguageInterface(edition.tag, form.messages)
    form.saved = '레이아웃 번역을 저장했습니다.'
    await loadLanguageEditions(true)
  } catch (reason) { form.error = reason instanceof Error ? reason.message : '인터페이스 번역을 저장하지 못했습니다.' } finally { form.busy = false }
}

async function autoTranslateInterface(edition: LanguageEdition) {
  const form = forms[edition.tag]
  form.busy = true; form.error = ''; form.saved = ''
  try {
    const translated = await translateLanguageInterface(edition.tag)
    for (const key of allWikiMessageKeys) {
      if (!form.messages[key].trim()) form.messages[key] = translated.interface_messages[key] || ''
    }
    const filled = translated.interface_translation_filled || 0
    form.saved = filled > 0
      ? `빈 레이아웃 문구 ${filled}개를 자동 번역했습니다. 필요하면 수정한 뒤 저장하세요.`
      : '자동 번역할 빈 레이아웃 문구가 없습니다.'
    await loadLanguageEditions(true)
  } catch { form.error = '자동 번역 서비스를 사용할 수 없습니다. 잠시 후 다시 시도하거나 문구를 직접 입력하세요.' } finally { form.busy = false }
}

async function saveNavigation(edition: LanguageEdition) {
  const form = forms[edition.tag]
  form.busy = true; form.error = ''; form.saved = ''
  try {
    await updateLanguageNavigation(edition.tag, form.navigation)
    form.saved = '둘러보기 구성을 저장했습니다.'
    await loadLanguageEditions(true)
  } catch (reason) { form.error = reason instanceof Error ? reason.message : '둘러보기 구성을 저장하지 못했습니다.' } finally { form.busy = false }
}

async function activate(edition: LanguageEdition) {
  const form = forms[edition.tag]
  form.busy = true; form.error = ''; form.saved = ''
  try {
    await activateLanguageEdition(edition.tag, form.mainPageName, form.source, form.messages, form.navigation)
    tab.value = 'active'
    await loadLanguageEditions(true)
  } catch (reason) { form.error = reason instanceof Error ? reason.message : '언어판을 활성화하지 못했습니다.' } finally { form.busy = false }
}

function activationReady(edition: LanguageEdition) {
  const form = forms[edition.tag]
  return Boolean(form?.mainPageName.trim() && form?.source.trim() && form?.navigation.length && wikiMessageKeys.every((key) => form.messages[key].trim()))
}

async function proposeEdition() {
  proposalBusy.value = true
  error.value = ''
  notice.value = ''
  try {
    await proposeLanguageEdition(proposalTag.value.trim())
    notice.value = '언어판을 제안했습니다. 관리자가 수락하면 메인 메뉴 번역 초안을 자동 생성합니다.'
    proposalTag.value = ''
    await loadLanguageEditions(true)
  } catch (reason) { error.value = reason instanceof Error ? reason.message : '언어판을 제안하지 못했습니다.' } finally { proposalBusy.value = false }
}

async function approveEdition(edition: LanguageEdition) {
  reviewBusy.value = edition.tag
  error.value = ''; notice.value = ''
  try {
    const approved = await approveLanguageEdition(edition.tag)
    if (approved.interface_translation_status === 'complete') {
      notice.value = `${edition.native_name} 언어판을 수락하고 메인 메뉴 문구 ${approved.interface_translation_filled || 0}개의 번역 초안을 만들었습니다.`
    } else if (approved.interface_translation_status === 'failed') {
      notice.value = `${edition.native_name} 언어판을 수락했습니다. 자동 번역에 실패해 다시 시도하거나 직접 입력할 수 있습니다.`
    } else {
      notice.value = `${edition.native_name} 언어판을 수락했습니다.`
    }
    delete forms[edition.tag]
    await loadLanguageEditions(true)
  } catch (reason) { error.value = reason instanceof Error ? reason.message : '언어판을 수락하지 못했습니다.' } finally { reviewBusy.value = '' }
}

async function rejectEdition(edition: LanguageEdition) {
  if (!window.confirm(`${edition.native_name} (${edition.tag}) 언어판 제안을 거절하시겠습니까? 제안 설정은 제거되지만 같은 언어를 다시 제안할 수 있습니다.`)) return
  reviewBusy.value = edition.tag
  error.value = ''; notice.value = ''
  try {
    await rejectLanguageEdition(edition.tag)
    notice.value = `${edition.native_name} 언어판 제안을 거절했습니다.`
    delete forms[edition.tag]
    await loadLanguageEditions(true)
  } catch (reason) { error.value = reason instanceof Error ? reason.message : '언어판 제안을 거절하지 못했습니다.' } finally { reviewBusy.value = '' }
}

onMounted(async () => {
  try {
    const [, catalog] = await Promise.all([loadLanguageEditions(true), getCountryCatalog()])
    countries.value = catalog
    if (activeEditions.value.length === 0) tab.value = 'proposals'
  } catch (reason) { error.value = reason instanceof Error ? reason.message : '언어판을 불러오지 못했습니다.' }
})
</script>

<template>
  <div class="admin-view">
    <header class="admin-page-header"><div><p>다국어 운영</p><h1>언어판 관리</h1><span>언어 제안을 검토하고, 공개 메뉴와 대문을 준비한 뒤 언어판을 활성화합니다.</span></div><span>활성 {{ activeEditions.length }} · 처리 중 {{ proposalCount }}</span></header>
    <VaAlert v-if="error" color="danger">{{ error }}</VaAlert>
    <VaAlert v-if="notice" color="info">{{ notice }}</VaAlert>
    <section class="admin-language-flow" aria-label="언어판 개설 절차">
      <span><b>1</b><span><strong>언어 제안</strong><small>기본 언어 코드 등록</small></span></span>
      <span><b>2</b><span><strong>관리자 수락</strong><small>메인 메뉴 초안 자동 번역</small></span></span>
      <span><b>3</b><span><strong>개설 준비</strong><small>대문과 둘러보기 검토</small></span></span>
      <span><b>4</b><span><strong>활성화</strong><small>공개 문서 작성 허용</small></span></span>
    </section>
    <div class="admin-language-tabs" role="tablist" aria-label="언어판 분류">
      <button type="button" :class="{ active: tab === 'active' }" @click="tab = 'active'">활성 언어판 <b>{{ activeEditions.length }}</b></button>
      <button type="button" :class="{ active: tab === 'proposals' }" @click="tab = 'proposals'">언어 제안 <b>{{ proposalCount }}</b></button>
    </div>

    <section v-if="tab === 'proposals'" class="admin-section">
      <form class="admin-edition-proposal" @submit.prevent="proposeEdition">
        <VaInput v-model="proposalTag" label="새 언어 코드" placeholder="fr, ar, zh, sh" required />
        <VaButton type="submit" :loading="proposalBusy">언어판 추가</VaButton>
      </form>
      <p class="admin-hint">문자·지역 하위 태그 없이 하나의 언어 코드만 제안합니다. 언어 관계와 언어판 운영 여부는 엔진이 정하지 않고 관리자가 검토합니다.</p>
      <p v-if="proposalCount === 0" class="admin-empty">대기 중이거나 수락된 언어 제안이 없습니다.</p>
      <VaCard v-for="edition in pendingEditions" :key="edition.tag" class="admin-language-card is-review">
        <VaCardTitle><span :lang="edition.tag" :dir="edition.direction">{{ edition.native_name }}</span><VaBadge :text="`${edition.tag} · 검토 대기`" color="warning" /></VaCardTitle>
        <VaCardContent>
          <p>언어판을 수락하면 메인 메뉴 번역 초안을 자동 생성하고 대문·둘러보기를 구성할 수 있습니다. 거절하면 이 제안만 제거됩니다.</p>
          <div class="interface-translation-actions">
            <VaButton :loading="reviewBusy === edition.tag" @click="approveEdition(edition)">수락 및 자동 번역</VaButton>
            <VaButton preset="secondary" color="danger" :disabled="Boolean(reviewBusy)" @click="rejectEdition(edition)">거절</VaButton>
          </div>
        </VaCardContent>
      </VaCard>
      <VaCard v-for="edition in approvedEditions" :key="edition.tag" class="admin-language-card is-setup">
        <VaCardTitle><span :lang="edition.tag" :dir="edition.direction">{{ edition.native_name }}</span><VaBadge :text="`${edition.tag} · 수락됨 · ${edition.script_name}`" color="info" /></VaCardTitle>
        <VaCardContent v-if="forms[edition.tag]">
          <div class="language-main-form">
            <VaInput v-model="forms[edition.tag].mainPageName" label="메인 페이지 이름" placeholder="이 언어판의 메인 페이지 이름" />
            <label>초기 본문<textarea v-model="forms[edition.tag].source" rows="8" placeholder="# 메인 페이지"></textarea></label>
          </div>
          <details class="navigation-editor" open>
            <summary>둘러보기 구성 <span>{{ forms[edition.tag].navigation.length }}개 섹션</span></summary>
            <div class="navigation-sections">
              <fieldset v-for="(section, sectionIndex) in forms[edition.tag].navigation" :key="sectionIndex">
                <div class="navigation-section-head"><label>섹션 제목<input v-model="section.title" maxlength="80" required></label><button type="button" title="섹션 삭제" @click="removeSection(edition, sectionIndex)"><Trash2 :size="17" /></button></div>
                <div v-for="(item, itemIndex) in section.items" :key="itemIndex" class="navigation-item-row"><label>표시 이름<input v-model="item.label" maxlength="120" required></label><label>문서 이름<input v-model="item.target" maxlength="120" required></label><button type="button" title="항목 삭제" :disabled="section.items.length === 1" @click="removeItem(edition, sectionIndex, itemIndex)"><Trash2 :size="16" /></button></div>
                <button class="navigation-add" type="button" @click="addItem(edition, sectionIndex)"><Plus :size="16" /> 항목 추가</button>
              </fieldset>
              <button class="navigation-add" type="button" @click="addSection(edition)"><Plus :size="16" /> 섹션 추가</button>
            </div>
          </details>
          <details class="interface-editor">
            <summary>레이아웃 번역 <span>필수 {{ configuredRequiredMessages(forms[edition.tag]) }}/{{ wikiMessageKeys.length }}</span></summary>
            <div class="interface-translation-actions"><p>언어판을 수락할 때 생성된 메인 메뉴 번역 초안입니다. 빈 문구만 다시 자동 번역하며 직접 입력한 내용은 덮어쓰지 않습니다.</p><VaButton size="small" preset="secondary" :loading="forms[edition.tag].busy" @click="autoTranslateInterface(edition)">빈 문구 자동 번역</VaButton></div>
            <div v-for="group in messageGroups" :key="group.title" class="interface-group"><h3>{{ group.title }}</h3><div class="interface-fields"><VaInput v-for="key in group.keys" :key="key" v-model="forms[edition.tag].messages[key]" :label="messageLabels[key]" placeholder="표시 문구 입력" required /></div></div>
          </details>
          <VaAlert v-if="forms[edition.tag].error" color="danger">{{ forms[edition.tag].error }}</VaAlert>
          <p v-if="forms[edition.tag].saved" class="admin-saved">{{ forms[edition.tag].saved }}</p>
          <VaButton :loading="forms[edition.tag].busy" :disabled="!activationReady(edition)" @click="activate(edition)">메인 페이지 생성 및 활성화</VaButton>
        </VaCardContent>
      </VaCard>
    </section>

    <section v-else class="admin-section">
      <VaCard v-for="edition in activeEditions" :key="edition.tag" class="admin-language-card is-live">
        <VaCardTitle><span :lang="edition.tag" :dir="edition.direction">{{ edition.native_name }}</span><VaBadge :text="`${edition.tag} · ${edition.script_name}`" color="success" /></VaCardTitle>
        <VaCardContent v-if="forms[edition.tag]">
          <p class="language-main-link">메인 페이지 <RouterLink :to="edition.main_page_url || '/'">{{ edition.main_page_name }}</RouterLink></p>
          <details class="portal-metadata-editor">
            <summary>언어 포털 국가 연결 <span>선택 {{ forms[edition.tag].countryCodes.length }}개</span></summary>
            <div class="portal-metadata-fields">
              <label>국가/지역 검색<input v-model="forms[edition.tag].countryQuery" type="search" placeholder="대한민국, Korea, KR"></label>
            </div>
            <div v-if="forms[edition.tag].countryCodes.length" class="selected-country-list">
              <button v-for="code in forms[edition.tag].countryCodes" :key="code" type="button" @click="toggleCountry(forms[edition.tag], code)">{{ countryLabel(countries.find((country) => country.code === code) || { code, name: code }) }} <span>{{ code }}</span> ×</button>
            </div>
            <div class="country-option-list">
              <label v-for="country in matchingCountries(forms[edition.tag])" :key="country.code"><input type="checkbox" :checked="forms[edition.tag].countryCodes.includes(country.code)" @change="toggleCountry(forms[edition.tag], country.code)"><span>{{ countryLabel(country) }}</span><code>{{ country.code }}</code></label>
            </div>
            <VaButton size="small" :loading="forms[edition.tag].busy" @click="savePortal(edition)">포털 설정 저장</VaButton>
          </details>
          <details class="navigation-editor">
            <summary>둘러보기 구성 <span>{{ forms[edition.tag].navigation.length }}개 섹션</span></summary>
            <div class="navigation-sections">
              <fieldset v-for="(section, sectionIndex) in forms[edition.tag].navigation" :key="sectionIndex">
                <div class="navigation-section-head"><label>섹션 제목<input v-model="section.title" maxlength="80"></label><button type="button" title="섹션 삭제" @click="removeSection(edition, sectionIndex)"><Trash2 :size="17" /></button></div>
                <div v-for="(item, itemIndex) in section.items" :key="itemIndex" class="navigation-item-row">
                  <label>표시 이름<input v-model="item.label" maxlength="120"></label><label>문서 이름<input v-model="item.target" maxlength="120"></label>
                  <button type="button" title="항목 삭제" :disabled="section.items.length === 1" @click="removeItem(edition, sectionIndex, itemIndex)"><Trash2 :size="16" /></button>
                </div>
                <button class="navigation-add" type="button" @click="addItem(edition, sectionIndex)"><Plus :size="16" /> 항목 추가</button>
              </fieldset>
              <button class="navigation-add" type="button" @click="addSection(edition)"><Plus :size="16" /> 섹션 추가</button>
              <VaButton size="small" :loading="forms[edition.tag].busy" @click="saveNavigation(edition)">둘러보기 저장</VaButton>
            </div>
          </details>
          <details class="interface-editor">
            <summary>레이아웃 번역 편집 <span>필수 {{ configuredRequiredMessages(forms[edition.tag]) }}/{{ wikiMessageKeys.length }}</span></summary>
            <div class="interface-translation-actions"><p>비어 있는 문구만 자동 번역합니다. 이미 저장했거나 현재 수정 중인 문구는 유지됩니다.</p><VaButton size="small" preset="secondary" :loading="forms[edition.tag].busy" @click="autoTranslateInterface(edition)">빈 문구 자동 번역</VaButton></div>
            <div v-for="group in messageGroups" :key="group.title" class="interface-group"><h3>{{ group.title }}</h3><div class="interface-fields"><VaInput v-for="key in group.keys" :key="key" v-model="forms[edition.tag].messages[key]" :label="messageLabels[key]" placeholder="표시 문구 입력" required /></div></div>
          </details>
          <VaAlert v-if="forms[edition.tag].error" color="danger">{{ forms[edition.tag].error }}</VaAlert>
          <p v-if="forms[edition.tag].saved" class="admin-saved">{{ forms[edition.tag].saved }}</p>
          <VaButton :loading="forms[edition.tag].busy" @click="saveInterface(edition)">레이아웃 번역 저장</VaButton>
        </VaCardContent>
      </VaCard>
    </section>
  </div>
</template>
