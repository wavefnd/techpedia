export interface BoxDefinition {
  name: string
  locales: string[]
  fallback_locale: string
  version: number
  updated_at: string
}

export interface BoxContent {
  locale: string
  source: string
  author: string
  version: number
  updated_at: string
}

export interface BoxEntry extends BoxDefinition { content: BoxContent }
export interface BoxPage { boxes: BoxDefinition[]; next: string }
