export type Locale = string
export type Permission = 'edit' | 'techdata' | 'admin' | 'owner'
export type DocumentEditPolicy = 'editors' | 'admins'

export interface Account {
  username: string
  display_name: string
  bio: string
  permissions: Permission[]
}

export interface PublicUserProfile {
  username: string
  display_name: string
  bio: string
  created_at: string
  contributions: WikiChange[]
}

export interface AdminUser extends Account {
  id: string
  created_at: string
  updated_at: string
}

export interface AdminUserManagement {
  users: AdminUser[]
  regular_permissions: Permission[]
}

export interface RegularPermissionUpdate {
  permissions: Permission[]
  updated_users: number
}

export interface LanguageLink {
  locale: Locale
  name: string
  url: string
  exists: boolean
}

export interface LanguageEdition {
  tag: Locale
  name: string
  native_name: string
  writing_system: string
  script_name: string
  direction: 'ltr' | 'rtl'
  status: 'pending' | 'approved' | 'active'
  main_page_name?: string
  main_page_url?: string
  interface_messages: Partial<Record<WikiMessageKey, string>>
  navigation: EditionNavigationSection[]
  portal_order: number
  country_codes: string[]
  document_count: number
  interface_translation_status?: 'complete' | 'failed'
  interface_translation_filled?: number
}

export interface EditionNavigationSection {
  title: string
  items: Array<{ label: string; target: string }>
}

export type WikiMessageKey =
  | 'menu' | 'close' | 'tagline' | 'searchLabel' | 'searchPlaceholder' | 'search' | 'noResults'
  | 'userActions' | 'account' | 'adminPanel' | 'logout' | 'login' | 'signup' | 'theme' | 'navigation' | 'languages'
  | 'addLanguage' | 'sidebarNote' | 'toc' | 'section' | 'document' | 'read' | 'discussion' | 'edit'
  | 'history' | 'contentLicense' | 'updated' | 'historyTitle' | 'currentArticle' | 'noSummary'
  | 'noRevisions' | 'selectRevision' | 'loadingHistory' | 'historyError' | 'revisionError' | 'editor'
  | 'cancel' | 'save' | 'saving' | 'loadingSource' | 'documentName' | 'editSummary' | 'summaryPlaceholder'
  | 'source' | 'autocomplete' | 'livePreview' | 'serverRendering' | 'saved' | 'sourceError'
  | 'previewError' | 'saveError' | 'lines' | 'characters' | 'scrollTop' | 'scrollBottom'
  | 'backlinks' | 'noBacklinks' | 'graph' | 'graphAll' | 'graphLanguage' | 'graphSearch'
  | 'graphLoading' | 'graphEmpty' | 'graphReset' | 'graphDocuments' | 'graphLinks' | 'graphTruncated'
  | 'graphLayoutStart' | 'graphLayoutPause' | 'graphLayoutRunning' | 'graphLayoutStable' | 'graphLayoutPaused' | 'graphLayoutFailed'

export interface GraphNode {
  id: string
  locale: Locale
  name: string
  url: string
  degree: number
}

export interface GraphEdge {
  source: string
  target: string
  kind: 'link' | 'translation'
}

export interface GraphResponse {
  scope: 'all' | 'language'
  locale?: Locale
  nodes: GraphNode[]
  edges: GraphEdge[]
  stats: {
    documents: number
    visible: number
    links: number
    missing: number
    truncated: boolean
  }
}

export interface LanguageCatalogEntry {
  tag: Locale
  name: string
  native_name: string
  writing_system: string
  script_name: string
}

export interface CountryCatalogEntry {
  code: string
  name: string
}

export interface PublicConfig {
  engine: {
    version: string
  }
  discussion: {
    enabled: boolean
    mode: 'board'
  }
}

export interface MediaFile {
	name: string
	original_name?: string
	width: number
	height: number
	size_bytes: number
	author?: string
	license?: string
	source?: string
	uploaded_by: string
	created_at: string
	thumbnail_url: string
	preview_url: string
	original_url: string
	reused?: boolean
}

export interface MediaSearchResponse {
	files: MediaFile[]
	usage_bytes: number
	limit_bytes: number
}

export interface SupportWallet {
  asset: string
  network: string
  address: string
  memo?: string
}

export interface SupportSettings {
  enabled: boolean
  title: string
  description: string
  wallets: SupportWallet[]
  updated_at?: string
}

export interface LinkPage {
  locale: Locale
  name: string
  url: string
  edit_url?: string
}

export interface BacklinkResponse {
  count: number
  pages: LinkPage[]
}

export interface AdminDocumentSummary {
  wid?: string
  tid?: string
  locale: Locale
  name: string
  url: string
  edit_url: string
  revision: string
  author: string
  updated_at: string
  system: boolean
  editable: boolean
  edit_policy: DocumentEditPolicy
}

export interface AdminDocumentRedirect {
  locale: Locale
  name: string
  url: string
  target_name?: string
  target_url?: string
  target_exists: boolean
  created_by: string
  created_at: string
}

export interface AdminMissingLink {
  source: LinkPage
  target: LinkPage
}

export interface AdminLinkReport {
  documents: number
  links: number
  missing_links: AdminMissingLink[]
  orphan_documents: LinkPage[]
}

export interface AdminTechdataMetadataCandidate {
  page: AdminDocumentSummary
  tid: string
  label: string
  linked_locales: Locale[]
}

export interface AdminTechdataNameCandidate {
  name: string
  pages: AdminDocumentSummary[]
}

export interface AdminTechdataAudit {
  documents: number
  linked_documents: number
  unlinked_documents: AdminDocumentSummary[]
  metadata_candidates: AdminTechdataMetadataCandidate[]
  name_candidates: AdminTechdataNameCandidate[]
}

export interface AdminServerInfo {
  location: string
  runtime: { engine_version: string; go_version: string; started_at: string; uptime_seconds: number; goroutines: number }
  host: { hostname: string; name: string; os: string; platform: string; platform_version: string; kernel_version: string; architecture: string; virtualization: string; virtualization_role: string }
  container: { name: string; os: string; platform: string; platform_version: string; architecture: string; virtualization: string; virtualization_role: string }
  cpu: { model: string; physical_cores: number; logical_cores: number }
  memory: { total: number; available: number; used: number; used_percent: number }
  storage: { path: string; total: number; free: number; used: number; used_percent: number }
}

export interface DiscussionTopic {
  id: string
  locale: Locale
  page_title: string
  page_url: string
  subject: string
  body: string
  author: string
  reply_count: number
  created_at: string
  updated_at: string
  archived: boolean
  archived_at?: string
  can_archive: boolean
  can_delete: boolean
}

export interface DiscussionReply {
  id: string
  body: string
  author: string
  created_at: string
  can_delete: boolean
}

export interface DiscussionThread {
  topic: DiscussionTopic
  replies: DiscussionReply[]
}

export interface AdminDashboard {
  documents: number
  users: number
  active_editions: number
  pending_editions: number
  by_locale: Array<{ locale: Locale; documents: number }>
  recent_documents: AdminDocumentSummary[]
}

export interface AdminAnalyticsDaily {
  date: string
  views: number
  average_rtt_ms: number
}

export interface AdminAnalyticsBreakdown {
  key: string
  views: number
}

export interface AdminAnalytics {
  days: number
  from: string
  through: string
  total_views: number
  average_rtt_ms: number
  rtt_samples: number
  daily: AdminAnalyticsDaily[]
  countries: AdminAnalyticsBreakdown[]
  locales: AdminAnalyticsBreakdown[]
  routes: AdminAnalyticsBreakdown[]
}

export interface ArticleResponse {
  locale: Locale
  name: string
  url: string
  description: string
  meta_description?: string
  html: string
  updated_at: string
  line_count: number
  character_count: number
  edit_policy: DocumentEditPolicy
  languages: LanguageLink[]
}

export interface SearchResult {
  locale: Locale
  name: string
  url: string
  description: string
}

export interface SearchResponse {
  query: string
  results: SearchResult[]
  create?: {
    locale: Locale
    name: string
    url: string
  }
}

export interface ResolvedWikiLink {
  locale: Locale
  target: string
  name: string
  url: string
  exists: boolean
}

export interface AdminPageResponse {
  locale: Locale
  name: string
  source: string
  edit_policy: DocumentEditPolicy
  revision: string
  updated_at: string
}

export interface EditorDraft {
  locale: Locale
  slug: string
  name: string
  source: string
  summary: string
  base_revision: string
  updated_at: string
}

export interface RevisionDiffLine {
  kind: 'equal' | 'added' | 'removed'
  old_line?: number
  new_line?: number
  text: string
}

export interface RevisionDiff {
  from: string
  to: string
  lines: RevisionDiffLine[]
}

export type ChangeAction = 'create' | 'edit' | 'move' | 'restore' | 'delete'

export interface WikiChange {
  locale: Locale
  name: string
  url: string
  revision?: string
  author: string
  summary?: string
  action: ChangeAction
  created_at: string
}

export interface TrashItem {
  wid: string
  locale: Locale
  name: string
  revision: string
  deleted_by: string
  reason?: string
  deleted_at: string
}

export interface AuditEntry {
  id: string
  actor: string
  action: string
  target_type: string
  target: string
  detail?: string
  result: 'success' | 'failed'
  created_at: string
}

export interface PreviewResponse {
  html: string
}

export interface RevisionSummary {
  revision: string
  author: string
  summary: string
  updated_at: string
}

export interface RevisionListResponse {
  locale: Locale
  name: string
  url: string
  can_compare: boolean
  can_reset_history: boolean
  revisions: RevisionSummary[]
}

export interface HistoryResetResponse {
  locale: Locale
  name: string
  url: string
  revision: string
  updated_at: string
  removed_revisions: number
  removed_changes: number
  removed_drafts: number
  removed_suggestions: number
}

export interface RevisionResponse extends RevisionSummary {
  locale: Locale
  name: string
  url: string
  html: string
}

export type TechdataDataType = 'item' | 'string' | 'quantity' | 'time' | 'url'

export interface TechdataValue {
  type: TechdataDataType
  item?: string
  text?: string
  amount?: string
  unit?: string
}

export interface TechdataStatement {
  property: string
  value: TechdataValue
  rank: 'preferred' | 'normal' | 'deprecated'
  qualifiers: Array<{ property: string; value: TechdataValue }>
  references: Array<{ snaks: Array<{ property: string; value: TechdataValue }> }>
}

export interface TechdataItemResponse {
  id: string
  labels: Partial<Record<Locale, string>>
  descriptions: Partial<Record<Locale, string>>
  aliases: Partial<Record<Locale, string[]>>
  wiki_pages: Partial<Record<Locale, string>>
  statements: TechdataStatement[]
  version: number
  updated_at: string
}

export interface TechdataDuplicateResponse {
  error: string
  code: 'techdata_duplicate_candidate'
  candidates: TechdataItemResponse[]
}

export type NamingSuggestionStatus = 'open' | 'accepted' | 'rejected'

export interface NamingFinding {
  checker_id: string
  area: 'link'
  before: string
  after: string
  match_kind: 'exact' | 'fuzzy'
  source: 'title' | 'techdata_label' | 'techdata_alias' | 'generated'
}

export interface NamingSuggestion {
  schema_version: number
  id: string
  locale: Locale
  page_name: string
  base_revision: string
  original_source: string
  proposed_name: string
  proposed_source: string
  findings: NamingFinding[]
  status: NamingSuggestionStatus
  created_at: string
  updated_at: string
  reviewed_by?: string
  reviewed_at?: string
}

export interface NamingScanResult {
  generator_version: string
  indexed_documents: number
  indexed_terms: number
  index_collisions: number
  gram_keys: number
  gram_postings: number
  documents_scanned: number
  links_discovered: number
  missing_links: number
  orphan_documents: number
  created: number
  existing: number
}
