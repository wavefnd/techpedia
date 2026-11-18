import type {
  AdminPageResponse,
  AdminDashboard,
	AdminAnalytics,
  AdminDocumentSummary,
  AdminDocumentRedirect,
  DiscussionReply,
  DiscussionThread,
  DiscussionTopic,
  AdminUser,
  AdminUserManagement,
  ArticleResponse,
  Locale,
  EditionNavigationSection,
  CountryCatalogEntry,
  LanguageCatalogEntry,
  LanguageEdition,
  PreviewResponse,
  PublicConfig,
  Permission,
  PublicUserProfile,
  RegularPermissionUpdate,
  RevisionListResponse,
  RevisionResponse,
  ResolvedWikiLink,
  SearchResponse,
  TechdataItemResponse,
  TechdataDuplicateResponse,
  WikiMessageKey,
  NamingSuggestion,
  NamingSuggestionStatus,
  NamingScanResult,
  BacklinkResponse,
  AdminLinkReport,
  AdminTechdataAudit,
  AdminServerInfo,
  SupportSettings,
  EditorDraft,
  RevisionDiff,
  HistoryResetResponse,
  WikiChange,
  ChangeAction,
  TrashItem,
  AuditEntry,
  GraphResponse,
	MediaFile,
	MediaSearchResponse,
} from '../types/api'

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    message: string,
    public readonly details?: unknown,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

export function techdataDuplicateResponse(reason: unknown): TechdataDuplicateResponse | null {
  if (!(reason instanceof ApiError) || reason.status !== 409) return null
  const details = reason.details
  if (!details || typeof details !== 'object' || !('code' in details) || !('candidates' in details)) return null
  const candidate = details as Partial<TechdataDuplicateResponse>
  return candidate.code === 'techdata_duplicate_candidate' && Array.isArray(candidate.candidates)
    ? candidate as TechdataDuplicateResponse
    : null
}

export async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: {
      Accept: 'application/json',
      ...init.headers,
    },
  })
  if (!response.ok) {
    const raw = (await response.text()).trim()
    let details: unknown
    try { details = raw ? JSON.parse(raw) : undefined } catch { details = undefined }
    const message = typeof details === 'object' && details && 'error' in details
      ? String((details as { error: unknown }).error)
      : raw
    throw new ApiError(response.status, message || `Request failed with status ${response.status}`, details)
  }
  if (response.status === 204) return undefined as T
  return response.json() as Promise<T>
}

export function getArticle(locale: Locale, name: string, signal?: AbortSignal): Promise<ArticleResponse> {
  return request<ArticleResponse>(`/api/pages/${locale}/${encodeURIComponent(name)}`, { signal })
}

export function recordArticleView(locale: Locale, name: string): Promise<void> {
  return request<void>(`/api/pages/${locale}/${encodeURIComponent(name)}/views`, { method: 'POST' })
}

export function getBacklinks(locale: Locale, name: string, signal?: AbortSignal): Promise<BacklinkResponse> {
  return request<BacklinkResponse>(`/api/pages/${locale}/${encodeURIComponent(name)}/backlinks`, { signal })
}

export function getPublicConfig(signal?: AbortSignal): Promise<PublicConfig> {
  return request<PublicConfig>('/api/config', { signal })
}

export function getUserProfile(username: string, signal?: AbortSignal): Promise<PublicUserProfile> {
  return request<PublicUserProfile>(`/api/users/${encodeURIComponent(username)}`, { signal })
}

export function getGraph(locale?: Locale, signal?: AbortSignal): Promise<GraphResponse> {
  const params = new URLSearchParams()
  if (locale) params.set('locale', locale)
  const query = params.size ? `?${params}` : ''
  return request<GraphResponse>(`/api/graph${query}`, { signal })
}

export function searchArticles(locale: Locale, query: string, signal?: AbortSignal, limit = 8): Promise<SearchResponse> {
  const params = new URLSearchParams({ locale, q: query, limit: String(limit) })
  return request<SearchResponse>(`/api/search?${params}`, { signal })
}

export function searchMedia(query: string, signal?: AbortSignal): Promise<MediaSearchResponse> {
	const params = new URLSearchParams({ q: query, limit: '30' })
	return request<MediaSearchResponse>(`/api/editor/media?${params}`, { signal })
}

export function uploadMedia(data: FormData): Promise<MediaFile> {
	return request<MediaFile>('/api/editor/media', { method: 'POST', body: data })
}

export function deleteMedia(name: string, confirmation: string): Promise<{ cleanup_pending: boolean }> {
  return request(`/api/admin/media/${encodeURIComponent(name)}`, {
    method: 'DELETE', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ confirmation }),
  })
}

export async function resolveWikiLinks(locale: Locale, targets: string[], signal?: AbortSignal): Promise<ResolvedWikiLink[]> {
  const params = new URLSearchParams({ locale })
  targets.forEach((target) => params.append('target', target))
  const response = await request<{ links: ResolvedWikiLink[] }>(`/api/links/resolve?${params}`, { signal })
  return response.links
}

export function getAdminPage(locale: Locale, name: string, signal?: AbortSignal): Promise<AdminPageResponse> {
  return request<AdminPageResponse>(`/api/editor/pages/${locale}/${encodeURIComponent(name)}`, { signal })
}

export function saveAdminPage(
  locale: Locale,
  routeName: string,
  page: Pick<AdminPageResponse, 'name' | 'source' | 'edit_policy'> & { base_revision: string },
  summary: string,
  translationSource?: { locale: Locale; name: string },
): Promise<AdminPageResponse> {
  return request<AdminPageResponse>(`/api/editor/pages/${locale}/${encodeURIComponent(routeName)}`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ ...page, summary, translation_source: translationSource }),
  })
}

export async function getEditorDraft(locale: Locale, name: string, signal?: AbortSignal): Promise<EditorDraft | null> {
  const response = await request<{ draft: EditorDraft | null }>(`/api/editor/pages/${locale}/${encodeURIComponent(name)}/draft`, { signal })
  return response.draft
}

export function saveEditorDraft(locale: Locale, name: string, draft: Pick<EditorDraft, 'name' | 'source' | 'summary' | 'base_revision'>): Promise<EditorDraft> {
  return request<EditorDraft>(`/api/editor/pages/${locale}/${encodeURIComponent(name)}/draft`, {
    method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(draft),
  })
}

export function deleteEditorDraft(locale: Locale, name: string): Promise<void> {
  return request<void>(`/api/editor/pages/${locale}/${encodeURIComponent(name)}/draft`, { method: 'DELETE' })
}

export function moveAdminPage(locale: Locale, routeName: string, name: string): Promise<AdminPageResponse> {
  return request<AdminPageResponse>(`/api/admin/pages/${locale}/${encodeURIComponent(routeName)}/move`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  })
}

export function previewTechMark(locale: Locale, name: string, source: string, signal?: AbortSignal): Promise<PreviewResponse> {
  return request<PreviewResponse>('/api/editor/preview', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ locale, name, source }),
    signal,
  })
}

export function getTechdataItem(tid: string, signal?: AbortSignal): Promise<TechdataItemResponse> {
  return request<TechdataItemResponse>(`/api/admin/techdata/items/${encodeURIComponent(tid)}`, {
    signal,
  })
}

export function deleteTechdataItem(tid: string, confirmation: string): Promise<void> {
  return request<void>(`/api/admin/techdata/items/${encodeURIComponent(tid)}`, {
    method: 'DELETE',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ confirmation }),
  })
}

export async function getTechdataItems(query = '', signal?: AbortSignal): Promise<TechdataItemResponse[]> {
  const params = new URLSearchParams({ limit: '100' })
  if (query.trim()) params.set('q', query.trim())
  const response = await request<{ items: TechdataItemResponse[] }>(`/api/admin/techdata/items?${params}`, { signal })
  return response.items
}

export function linkPageTechdata(locale: Locale, name: string, tid = '', confirmDistinct = false): Promise<TechdataItemResponse> {
  return request<TechdataItemResponse>(`/api/admin/pages/${encodeURIComponent(locale)}/${encodeURIComponent(name)}/techdata`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ tid, confirm_distinct: confirmDistinct }),
  })
}

export function unlinkPageTechdata(locale: Locale, name: string): Promise<void> {
  return request<void>(`/api/admin/pages/${encodeURIComponent(locale)}/${encodeURIComponent(name)}/techdata`, {
    method: 'DELETE',
  })
}

export function getRevisions(locale: Locale, name: string, signal?: AbortSignal): Promise<RevisionListResponse> {
  return request<RevisionListResponse>(`/api/pages/${locale}/${encodeURIComponent(name)}/revisions?limit=100`, { signal })
}

export function getRevision(locale: Locale, name: string, revision: string, signal?: AbortSignal): Promise<RevisionResponse> {
  return request<RevisionResponse>(`/api/pages/${locale}/${encodeURIComponent(name)}/revisions/${encodeURIComponent(revision)}`, { signal })
}

export function getRevisionDiff(locale: Locale, name: string, from: string, to: string, signal?: AbortSignal): Promise<RevisionDiff> {
  const params = new URLSearchParams({ from, to })
  return request<RevisionDiff>(`/api/pages/${locale}/${encodeURIComponent(name)}/diff?${params}`, { signal })
}

export function restoreRevision(locale: Locale, name: string, revision: string, baseRevision: string, reason: string): Promise<AdminPageResponse> {
  return request<AdminPageResponse>(`/api/editor/pages/${locale}/${encodeURIComponent(name)}/restore`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ revision, base_revision: baseRevision, reason }),
  })
}

export function resetDocumentHistory(locale: Locale, name: string, baseRevision: string, confirmation: string): Promise<HistoryResetResponse> {
  return request<HistoryResetResponse>(`/api/admin/pages/${locale}/${encodeURIComponent(name)}/history/reset`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ base_revision: baseRevision, confirmation }),
  })
}

export interface ChangeFilters {
  locale?: string
  user?: string
  q?: string
  action?: ChangeAction | ''
  since?: string
  until?: string
  limit?: number
}

function changeParams(filters: ChangeFilters): URLSearchParams {
  const params = new URLSearchParams()
  Object.entries(filters).forEach(([key, value]) => {
    if (value !== undefined && value !== '') params.set(key, String(value))
  })
  return params
}

export async function getRecentChanges(filters: ChangeFilters = {}, signal?: AbortSignal): Promise<WikiChange[]> {
  const response = await request<{ changes: WikiChange[] }>(`/api/recent-changes?${changeParams(filters)}`, { signal })
  return response.changes
}

export async function getMyContributions(filters: ChangeFilters = {}, signal?: AbortSignal): Promise<WikiChange[]> {
  const response = await request<{ changes: WikiChange[] }>(`/api/account/contributions?${changeParams(filters)}`, { signal })
  return response.changes
}

export function getUserManagement(signal?: AbortSignal): Promise<AdminUserManagement> {
  return request<AdminUserManagement>('/api/admin/users', { signal })
}

export function getAdminDashboard(signal?: AbortSignal): Promise<AdminDashboard> {
  return request<AdminDashboard>('/api/admin/dashboard', { signal })
}

export function getAdminAnalytics(days = 30, signal?: AbortSignal): Promise<AdminAnalytics> {
  const params = new URLSearchParams({ days: String(days) })
  return request<AdminAnalytics>(`/api/admin/analytics?${params}`, { signal })
}

export function getAdminServerInfo(signal?: AbortSignal): Promise<AdminServerInfo> {
  return request<AdminServerInfo>('/api/admin/server', { signal })
}

export async function getAdminPages(locale = '', query = '', signal?: AbortSignal): Promise<AdminDocumentSummary[]> {
  const params = new URLSearchParams({ limit: '300' })
  if (locale) params.set('locale', locale)
  if (query) params.set('q', query)
  const response = await request<{ pages: AdminDocumentSummary[] }>(`/api/admin/pages?${params}`, { signal })
  return response.pages
}

export async function getAdminRedirects(locale = '', signal?: AbortSignal): Promise<AdminDocumentRedirect[]> {
  const params = new URLSearchParams({ limit: '300' })
  if (locale) params.set('locale', locale)
  const query = params.size ? `?${params}` : ''
  const response = await request<{ redirects: AdminDocumentRedirect[] }>(`/api/admin/redirects${query}`, { signal })
  return response.redirects
}

export function createAdminRedirect(locale: Locale, name: string, target: string): Promise<AdminDocumentRedirect> {
  return request<AdminDocumentRedirect>('/api/admin/redirects', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ locale, name, target }),
  })
}

export function deleteAdminRedirect(locale: Locale, name: string): Promise<void> {
  return request<void>(`/api/admin/redirects/${encodeURIComponent(locale)}/${encodeURIComponent(name)}`, { method: 'DELETE' })
}

export function getAdminDocumentSummary(locale: Locale, name: string, signal?: AbortSignal): Promise<AdminDocumentSummary> {
  return request<AdminDocumentSummary>(`/api/admin/pages/${encodeURIComponent(locale)}/${encodeURIComponent(name)}`, { signal })
}

export function deleteAdminPage(locale: Locale, name: string, reason = ''): Promise<void> {
  return request<void>(`/api/admin/pages/${encodeURIComponent(locale)}/${encodeURIComponent(name)}`, {
    method: 'DELETE', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ reason }),
  })
}

export async function getAdminTrash(signal?: AbortSignal): Promise<TrashItem[]> {
  const response = await request<{ items: TrashItem[] }>('/api/admin/trash', { signal })
  return response.items
}

export function restoreAdminTrash(wid: string, reason = ''): Promise<{ locale: Locale; name: string; url: string }> {
  return request(`/api/admin/trash/${encodeURIComponent(wid)}/restore`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ reason }),
  })
}

export async function getAdminAudit(filters: { actor?: string; action?: string; target?: string } = {}, signal?: AbortSignal): Promise<AuditEntry[]> {
  const params = new URLSearchParams({ limit: '300' })
  Object.entries(filters).forEach(([key, value]) => { if (value) params.set(key, value) })
  const response = await request<{ entries: AuditEntry[] }>(`/api/admin/audit?${params}`, { signal })
  return response.entries
}

export function getAdminLinkReport(locale = '', signal?: AbortSignal): Promise<AdminLinkReport> {
  const params = new URLSearchParams()
  if (locale) params.set('locale', locale)
  const query = params.size ? `?${params}` : ''
  return request<AdminLinkReport>(`/api/admin/links/report${query}`, { signal })
}

export function getAdminTechdataAudit(signal?: AbortSignal): Promise<AdminTechdataAudit> {
  return request<AdminTechdataAudit>('/api/admin/techdata/audit', { signal })
}

export function getSupportSettings(signal?: AbortSignal): Promise<SupportSettings> {
  return request<SupportSettings>('/api/support', { signal })
}

export function getAdminSupportSettings(signal?: AbortSignal): Promise<SupportSettings> {
  return request<SupportSettings>('/api/admin/support', { signal })
}

export function saveAdminSupportSettings(settings: Omit<SupportSettings, 'updated_at'>): Promise<SupportSettings> {
  return request<SupportSettings>('/api/admin/support', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(settings),
  })
}

export async function getAdminDiscussionTopics(query = '', signal?: AbortSignal): Promise<{ count: number; topics: DiscussionTopic[] }> {
  const params = new URLSearchParams()
  if (query) params.set('q', query)
  return request<{ count: number; topics: DiscussionTopic[] }>(`/api/admin/discussions/topics?${params}`, { signal })
}

export function deleteAdminDiscussionTopic(topicID: string): Promise<void> {
  return request<void>(`/api/admin/discussions/topics/${encodeURIComponent(topicID)}`, { method: 'DELETE' })
}

export function scanNamingIndex(locale = ''): Promise<NamingScanResult> {
  const params = new URLSearchParams()
  if (locale) params.set('locale', locale)
  const query = params.size ? `?${params}` : ''
  return request<NamingScanResult>(`/api/admin/bots/naming/scan${query}`, { method: 'POST' })
}

export async function getNamingSuggestions(status: NamingSuggestionStatus | '' = 'open', signal?: AbortSignal): Promise<NamingSuggestion[]> {
  const params = new URLSearchParams({ limit: '200' })
  if (status) params.set('status', status)
  const response = await request<{ suggestions: NamingSuggestion[] }>(`/api/admin/bots/naming/suggestions?${params}`, { signal })
  return response.suggestions
}

export function acceptNamingSuggestion(id: string, proposedName: string, proposedSource: string): Promise<NamingSuggestion> {
  return request<NamingSuggestion>(`/api/admin/bots/naming/suggestions/${encodeURIComponent(id)}/accept`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ proposed_name: proposedName, proposed_source: proposedSource }),
  })
}

export function rejectNamingSuggestion(id: string): Promise<NamingSuggestion> {
  return request<NamingSuggestion>(`/api/admin/bots/naming/suggestions/${encodeURIComponent(id)}/reject`, { method: 'POST' })
}

export async function getDiscussionTopics(locale: Locale, name: string, signal?: AbortSignal): Promise<DiscussionTopic[]> {
  const response = await request<{ topics: DiscussionTopic[] }>(`/api/discussions/${locale}/${encodeURIComponent(name)}/topics`, { signal })
  return response.topics
}

export function createDiscussionTopic(locale: Locale, name: string, subject: string, body: string): Promise<DiscussionTopic> {
  return request<DiscussionTopic>(`/api/discussions/${locale}/${encodeURIComponent(name)}/topics`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ subject, body }),
  })
}

export function getDiscussionThread(topicID: string, signal?: AbortSignal): Promise<DiscussionThread> {
  return request<DiscussionThread>(`/api/discussions/topics/${encodeURIComponent(topicID)}`, { signal })
}

export function createDiscussionReply(topicID: string, body: string): Promise<DiscussionReply> {
  return request<DiscussionReply>(`/api/discussions/topics/${encodeURIComponent(topicID)}/replies`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ body }),
  })
}

export function archiveDiscussionTopic(topicID: string): Promise<DiscussionTopic> {
  return request<DiscussionTopic>(`/api/discussions/topics/${encodeURIComponent(topicID)}/archive`, { method: 'POST' })
}

export function deleteDiscussionTopic(topicID: string): Promise<void> {
  return request<void>(`/api/discussions/topics/${encodeURIComponent(topicID)}`, { method: 'DELETE' })
}

export function deleteDiscussionReply(topicID: string, replyID: string): Promise<void> {
  return request<void>(`/api/discussions/topics/${encodeURIComponent(topicID)}/replies/${encodeURIComponent(replyID)}`, { method: 'DELETE' })
}

export function setUserPermissions(userID: string, permissions: Permission[]): Promise<AdminUser> {
  return request<AdminUser>(`/api/admin/users/${encodeURIComponent(userID)}/permissions`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ permissions }),
  })
}

export function setRegularUserPermissions(permissions: Permission[]): Promise<RegularPermissionUpdate> {
  return request<RegularPermissionUpdate>('/api/admin/users/regular-permissions', {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ permissions }),
  })
}

export async function getLanguageEditions(signal?: AbortSignal): Promise<LanguageEdition[]> {
  const response = await request<{ editions: LanguageEdition[] }>('/api/languages', { signal })
  return response.editions
}

export async function searchLanguageCatalog(query: string, signal?: AbortSignal): Promise<LanguageCatalogEntry[]> {
  const params = new URLSearchParams({ q: query, limit: '40' })
  const response = await request<{ languages: LanguageCatalogEntry[] }>(`/api/language-catalog?${params}`, { signal })
  return response.languages
}

export async function getCountryCatalog(signal?: AbortSignal): Promise<CountryCatalogEntry[]> {
  const response = await request<{ countries: CountryCatalogEntry[] }>('/api/country-catalog', { signal })
  return response.countries
}

export function proposeLanguageEdition(tag: string): Promise<LanguageEdition> {
  return request<LanguageEdition>('/api/editor/languages', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ tag }),
  })
}

export function approveLanguageEdition(tag: string): Promise<LanguageEdition> {
  return request<LanguageEdition>(`/api/admin/languages/${encodeURIComponent(tag)}/approve`, {
    method: 'POST',
  })
}

export function rejectLanguageEdition(tag: string): Promise<void> {
  return request<void>(`/api/admin/languages/${encodeURIComponent(tag)}`, {
    method: 'DELETE',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ confirmation: tag }),
  })
}

export function activateLanguageEdition(tag: string, mainPageName: string, source: string, interfaceMessages: Partial<Record<WikiMessageKey, string>>, navigation: EditionNavigationSection[]): Promise<LanguageEdition> {
  return request<LanguageEdition>(`/api/admin/languages/${encodeURIComponent(tag)}/activate`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ main_page_name: mainPageName, source, interface_messages: interfaceMessages, navigation }),
  })
}

export function updateLanguageInterface(tag: string, interfaceMessages: Partial<Record<WikiMessageKey, string>>): Promise<LanguageEdition> {
  return request<LanguageEdition>(`/api/admin/languages/${encodeURIComponent(tag)}/interface`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ interface_messages: interfaceMessages }),
  })
}

export function translateLanguageInterface(tag: string): Promise<LanguageEdition> {
  return request<LanguageEdition>(`/api/admin/languages/${encodeURIComponent(tag)}/interface/translate`, {
    method: 'POST',
  })
}

export function updateLanguageNavigation(tag: string, navigation: EditionNavigationSection[]): Promise<LanguageEdition> {
  return request<LanguageEdition>(`/api/admin/languages/${encodeURIComponent(tag)}/navigation`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ navigation }),
  })
}

export function updateLanguagePortal(tag: string, countryCodes: string[], portalOrder = 0): Promise<LanguageEdition> {
  return request<LanguageEdition>(`/api/admin/languages/${encodeURIComponent(tag)}/portal`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ portal_order: portalOrder, country_codes: countryCodes }),
  })
}
