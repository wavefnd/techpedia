import { defineConfig } from 'vitepress'

const repository = process.env.GITHUB_REPOSITORY || ''
const name = repository.split('/')[1] || ''
const base = process.env.DOCS_BASE || (name && !name.endsWith('.github.io') ? `/${name}/` : '/')

if (!base.startsWith('/') || !base.endsWith('/')) throw new Error('DOCS_BASE must begin and end with /')

export default defineConfig({
  lang: 'en-US',
  title: 'TechPedia',
  description: 'Install, write, and operate a multilingual Go wiki.',
  base,
  lastUpdated: false,
  cleanUrls: false,
  themeConfig: {
    nav: [{ text: 'Guide', link: '/getting-started' }, { text: 'Server administration', link: '/deployment' }, { text: 'TechMark', link: '/techmark' }],
    sidebar: [
      { text: 'Use TechPedia', items: [{ text: 'Overview', link: '/' }, { text: 'Quick start', link: '/getting-started' }, { text: 'Language editions', link: '/language-editions' }, { text: 'Write and edit', link: '/editing' }, { text: 'TechMark reference', link: '/techmark' }, { text: 'Techdata and language links', link: '/techdata' }, { text: 'Reusable boxes and media', link: '/boxes-and-media' }] },
      { text: 'Run a server', items: [{ text: 'Deployment', link: '/deployment' }, { text: 'Configuration', link: '/configuration' }, { text: 'Operations and recovery', link: '/operations' }, { text: 'GitHub migration', link: '/source-migration' }, { text: 'Security', link: '/security' }, { text: 'Search indexing', link: '/search-indexing' }, { text: 'ChatGPT and MCP', link: '/integrations' }] },
      { text: 'Develop and distribute', items: [{ text: 'Architecture', link: '/arch' }, { text: 'API boundaries', link: '/api' }, { text: 'Performance', link: '/performance' }, { text: 'Fonts', link: '/font' }, { text: 'Contributing', link: '/contributing' }, { text: 'Documentation site', link: '/documentation' }, { text: 'License', link: '/license' }] },
    ],
    search: { provider: 'local' },
    outline: { level: [2, 3] },
    footer: { message: 'Original engine source: Blue Oak Model License 1.0.0. Third-party assets retain their own terms.', copyright: 'Copyright 2026 Wave Foundation' },
    socialLinks: repository ? [{ icon: 'github', link: `https://github.com/${repository}` }] : [],
  },
})
