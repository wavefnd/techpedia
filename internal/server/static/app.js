class TechPediaApp extends HTMLElement {
  constructor() {
    super();
    const route = this.resolveRoute();
    this.locale = route.locale;
    this.slug = route.slug;
    this.group = new URLSearchParams(window.location.search).get('group') || '';
    this.page = null;
    this.mode = 'read';
    this.searchTimer = null;
    this.searchAbortController = null;
    this.supportedLocales = [];
  }

  async connectedCallback() {
    document.documentElement.lang = this.locale;
    this.renderShell();
    this.bindEvents();
    await this.loadSupportedLocales();
    this.loadPage();
  }

  resolveRoute() {
    const path = window.location.pathname;
    const localized = path.match(/^\/([^/]+)\/wiki\/?([^/]*)$/);
    if (localized) {
      return {
        locale: localized[1],
        slug: decodeURIComponent(localized[2] || 'main') || 'main',
      };
    }
    return { locale: document.documentElement.lang || 'ko', slug: 'main' };
  }

  makeWikiPath(slug = this.slug, locale = this.locale, params = {}) {
    const search = new URLSearchParams();
    Object.entries(params).forEach(([key, value]) => {
      if (value !== undefined && value !== null && String(value).trim() !== '') {
        search.set(key, String(value));
      }
    });

    const query = search.toString();
    return `/${locale}/wiki/${encodeURIComponent(slug || 'main')}${query ? `?${query}` : ''}`;
  }

  apiPagePath(locale = this.locale, slug = this.slug) {
    return `/api/pages/${encodeURIComponent(locale)}/${encodeURIComponent(slug)}`;
  }

  localeName(locale) {
    try {
      return new Intl.DisplayNames([this.locale], { type: 'language' }).of(locale) || locale;
    } catch {
      return locale;
    }
  }

  async loadSupportedLocales() {
    try {
      const response = await fetch('/api/languages', { headers: { Accept: 'application/json' } });
      if (!response.ok) return;
      const payload = await response.json();
      this.supportedLocales = (payload.editions || [])
        .filter((edition) => edition.status === 'active')
        .map((edition) => edition.tag);
    } catch {
      this.supportedLocales = [];
    }
  }

  escapeHTML(value) {
    return String(value ?? '')
      .replaceAll('&', '&amp;')
      .replaceAll('<', '&lt;')
      .replaceAll('>', '&gt;')
      .replaceAll('"', '&quot;')
      .replaceAll("'", '&#039;');
  }

  renderShell() {
    const safeSlug = this.escapeHTML(this.slug);
    const mainHref = this.makeWikiPath('main');
    const recentHref = this.makeWikiPath('recent-changes');
    const randomHref = this.makeWikiPath('random');
    const helpHref = this.makeWikiPath('help');
    const copyrightHref = this.makeWikiPath('copyright');
    const programmingHref = this.makeWikiPath('programming');
    const systemHref = this.makeWikiPath('system');
    const networkHref = this.makeWikiPath('network');
    const securityHref = this.makeWikiPath('security');
    const databaseHref = this.makeWikiPath('database');

    this.innerHTML = `
      <div class="tp-wiki">
        <aside class="tp-sidebar" aria-label="사이트 메뉴">
          <a class="tp-brand" href="${mainHref}" aria-label="TechPedia 대문">
            <span class="tp-brand-mark">T</span>
            <span>
              <strong>TechPedia</strong>
              <small>기술을 설명하는 위키</small>
            </span>
          </a>

          <nav class="tp-side-block" aria-label="기본 탐색">
            <h2>탐색</h2>
            <a href="${mainHref}">대문</a>
            <a href="${recentHref}">최근 변경</a>
            <a href="${randomHref}">임의 문서</a>
            <a href="${helpHref}">도움말</a>
            <a href="${copyrightHref}">저작권</a>
          </nav>

          <nav class="tp-side-block" aria-label="기술 분류">
            <h2>기술 분류</h2>
            <a href="${programmingHref}">프로그래밍</a>
            <a href="${systemHref}">시스템</a>
            <a href="${networkHref}">네트워크</a>
            <a href="${securityHref}">보안</a>
            <a href="${databaseHref}">데이터베이스</a>
          </nav>

          <nav class="tp-side-block" aria-label="문서 도구">
            <h2>문서 도구</h2>
            <button type="button" data-action="open-editor">문서 편집</button>
            <button type="button" data-action="copy-link">링크 복사</button>
            <button type="button" data-action="scroll-top">맨 위로</button>
          </nav>
        </aside>

        <div class="tp-main">
          <header class="tp-top" aria-label="상단 검색">
            <a class="tp-mobile-brand" href="${mainHref}">TechPedia</a>
            <div class="tp-search-area">
              <form class="tp-search" id="search-form" role="search">
                <label for="search-input">TechPedia 검색</label>
                <input id="search-input" type="search" placeholder="TechPedia 검색" autocomplete="off" aria-controls="search-results" aria-expanded="false">
                <button type="submit">검색</button>
              </form>
              <div class="tp-search-results" id="search-results" hidden></div>
            </div>
          </header>

          <main class="tp-page">
            <section class="tp-page-main">
              <div class="tp-crumb">
                <a href="${mainHref}">TechPedia</a>
                <span>/</span>
                <span id="crumb-locale">${this.escapeHTML(this.localeName(this.locale))}</span>
                <span>/</span>
                <span id="crumb-slug">${safeSlug}</span>
              </div>

              <header class="tp-title-head">
                <div class="tp-title-copy">
                  <p class="tp-label">기술 문서</p>
                  <h1 id="page-title">${safeSlug}</h1>
                  <p id="page-subtitle">문서를 불러오는 중입니다.</p>
                </div>
                <nav class="tp-language-switch" id="language-switch" aria-label="문서 언어">
                  <span>언어</span>
                </nav>
              </header>

              <div class="tp-tabs" aria-label="문서 탭">
                <div class="tp-tab-group">
                  <button class="tp-tab" type="button" aria-selected="true" data-action="read-mode">문서</button>
                  <button class="tp-tab" type="button" aria-selected="false" data-action="discussion-placeholder">토론</button>
                </div>
                <div class="tp-tab-group tp-tab-actions">
                  <button class="tp-tab" type="button" aria-selected="false" data-action="open-editor">편집</button>
                  <button class="tp-tab" type="button" aria-selected="false" data-action="open-history">역사</button>
                </div>
              </div>

              <div class="tp-message" id="status-banner" role="status" aria-live="polite" hidden></div>

              <article class="tp-read-view" id="read-view" aria-label="문서 본문">
                <div class="tp-read-meta" id="read-meta">최근 수정 정보를 준비하는 중입니다.</div>
                <div id="article" class="tp-article-body">
                  <div class="tp-loading">
                    <span></span>
                    <span></span>
                    <span></span>
                  </div>
                </div>
                <footer class="tp-license">
                  이 문서는 CC BY-SA 4.0 조건으로 제공됩니다.
                </footer>
              </article>

              <section class="tp-edit-view" id="edit-view" aria-label="문서 편집기" hidden>
                <div class="tp-edit-head">
                  <div>
                    <h2>문서 편집</h2>
                    <p id="edit-summary">문서 원문을 수정합니다.</p>
                  </div>
                  <div class="tp-edit-head-actions">
                    <button class="tp-button primary" type="button" id="save">저장</button>
                    <button class="tp-button" type="button" data-action="read-mode">읽기로 돌아가기</button>
                  </div>
                </div>

                <div class="tp-edit-layout">
                  <div class="tp-editor-pane">
                    <label class="tp-field">
                      <span>문서 제목</span>
                      <input id="title" placeholder="문서 제목">
                    </label>

                    <label class="tp-field">
                      <span>URL 이름</span>
                      <input id="url-slug" placeholder="예: operating-systems" spellcheck="false">
                    </label>
                    <p class="tp-field-hint">주소에 들어가는 이름입니다. 예: operating-systems → /ko/wiki/operating-systems</p>

                    <div class="tp-field-grid">
                      <label class="tp-field">
                        <span>문서 언어</span>
                        <input id="locale-field" readonly>
                      </label>

                      <label class="tp-field">
                        <span>번역 그룹</span>
                        <input id="group" placeholder="예: operating-systems" spellcheck="false">
                      </label>
                    </div>
                    <p class="tp-field-hint">같은 개념의 한국어/영어 문서는 같은 번역 그룹을 사용합니다. 예: 한국어 운영체제와 영어 Operating systems는 둘 다 operating-systems</p>

                    <div class="tp-editor-toolbar" aria-label="TechMark 삽입">
                      <button type="button" data-insert="heading">문단</button>
                      <button type="button" data-insert="bold">굵게</button>
                      <button type="button" data-insert="link">문서 링크</button>
                      <button type="button" data-insert="toc">목차</button>
                      <button type="button" data-insert="alert">알림</button>
                      <button type="button" data-insert="infobox">정보상자</button>
                      <button type="button" data-insert="table">표</button>
                      <button type="button" data-insert="code">코드</button>
                    </div>

                    <label class="tp-field">
                      <span>본문</span>
                      <textarea id="source" spellcheck="false" placeholder="# 문서 제목&#10;&#10;문서 내용을 작성하세요."></textarea>
                    </label>

                    <div class="tp-save-row">
                      <button class="tp-button primary" type="button" data-action="save-page">저장</button>
                      <button class="tp-button" type="button" data-action="read-mode">취소</button>
                      <span class="tp-save-status" id="save-status" role="status" aria-live="polite"></span>
                    </div>
                  </div>

                  <aside class="tp-edit-guide" aria-label="편집 도움말">
                    <h3>TechMark 빠른 규칙</h3>
                    <dl>
                      <div><dt># 제목</dt><dd>문서 제목 또는 큰 문단</dd></div>
                      <div><dt>[[문서]]</dt><dd>다른 문서로 연결</dd></div>
                      <div><dt>**강조**</dt><dd>중요한 용어 강조</dd></div>
                      <div><dt>&lt;wiki-toc&gt;</dt><dd>문서 목차 삽입</dd></div>
                    </dl>
                    <h3>문서 템플릿</h3>
                    <div class="tp-template-actions">
                      <button type="button" data-template="language">언어</button>
                      <button type="button" data-template="person">인물</button>
                      <button type="button" data-template="company">기업</button>
                      <button type="button" data-template="software">소프트웨어</button>
                    </div>
                  </aside>
                </div>
              </section>

              <section class="tp-history-view" id="history-view" aria-label="문서 역사" hidden>
                <div class="tp-history-head">
                  <div>
                    <h2>문서 역사</h2>
                    <p>이 언어판의 저장된 판 목록입니다.</p>
                  </div>
                  <button class="tp-button" type="button" data-action="read-mode">문서로 돌아가기</button>
                </div>
                <div class="tp-history-list" id="history-list">
                  <p>역사를 불러오는 중입니다.</p>
                </div>
              </section>
            </section>

            <aside class="tp-aside" aria-label="문서 보조">
              <section class="tp-aside-box" id="language-box">
                <h2>언어판</h2>
                <nav class="tp-language-list" id="language-list" aria-label="언어판 목록"></nav>
              </section>

              <section class="tp-aside-box tp-toc-box">
                <h2>목차</h2>
                <nav class="tp-toc" id="toc-list" aria-label="문서 목차">
                  <p>목차를 준비하는 중입니다.</p>
                </nav>
              </section>

              <section class="tp-aside-box" id="related-box" hidden>
                <h2>관련 문서</h2>
                <nav class="tp-related" id="related-list" aria-label="관련 문서"></nav>
              </section>

              <section class="tp-aside-box tp-page-tools">
                <h2>문서 작업</h2>
                <button type="button" data-action="open-editor">편집</button>
                <button type="button" data-action="open-history">역사</button>
                <button type="button" data-action="copy-link">링크 복사</button>
              </section>
            </aside>
          </main>
        </div>
      </div>
    `;
  }

  bindEvents() {
    this.querySelector('#save').addEventListener('click', () => this.savePage());

    const searchForm = this.querySelector('#search-form');
    const searchInput = this.querySelector('#search-input');

    searchForm.addEventListener('submit', (event) => {
      event.preventDefault();
      const value = searchInput.value.trim();
      if (value) {
        this.performSearch(value, true);
      }
    });

    searchInput.addEventListener('input', () => {
      const value = searchInput.value.trim();
      window.clearTimeout(this.searchTimer);
      if (!value) {
        this.clearSearchResults();
        return;
      }
      this.searchTimer = window.setTimeout(() => this.performSearch(value, false), 180);
    });

    searchInput.addEventListener('focus', () => {
      const value = searchInput.value.trim();
      if (value) {
        this.performSearch(value, false);
      }
    });

    document.addEventListener('click', (event) => {
      if (!this.contains(event.target)) {
        this.clearSearchResults();
      }
    });

    this.addEventListener('click', (event) => {
      const templateTarget = event.target.closest('[data-template]');
      if (templateTarget) {
        this.applyTemplate(templateTarget.dataset.template);
        return;
      }

      const insertTarget = event.target.closest('[data-insert]');
      if (insertTarget) {
        this.insertSnippet(insertTarget.dataset.insert);
        return;
      }

      const actionTarget = event.target.closest('[data-action]');
      if (!actionTarget) {
        return;
      }

      const action = actionTarget.dataset.action;
      if (action === 'open-editor') this.openEditor();
      if (action === 'read-mode') this.closeEditor();
      if (action === 'save-page') this.savePage();
      if (action === 'copy-link') this.copyLink();
      if (action === 'scroll-top') window.scrollTo({ top: 0, behavior: 'smooth' });
      if (action === 'discussion-placeholder') this.selectPlaceholderTab(action, '토론 기능은 문서 엔진 이후 연결됩니다.');
      if (action === 'open-history') this.openHistory();
      if (action === 'view-revision') this.viewRevision(actionTarget.dataset.revision);
    });
  }

  async loadPage() {
    const article = this.querySelector('#article');
    const titleInput = this.querySelector('#title');
    const sourceInput = this.querySelector('#source');
    const urlSlugInput = this.querySelector('#url-slug');
    const groupInput = this.querySelector('#group');
    const localeInput = this.querySelector('#locale-field');

    this.renderLoading();

    let res;
    try {
      res = await fetch(this.apiPagePath());
    } catch (error) {
      this.page = null;
      this.setTitle(this.slug, '문서 로드 실패');
      this.setReadMeta(null);
      article.innerHTML = `<div class="tp-empty"><strong>서버에 연결할 수 없습니다.</strong><p>TechPedia 서버 상태를 확인하세요.</p></div>`;
      this.updateToc([]);
      this.updateRelated(article);
      this.renderLanguageSwitch(null);
      this.setStatus('문서를 불러오지 못했습니다.', 'error');
      return;
    }

    if (res.status === 404) {
      const title = this.createTitleFromURL() || this.slug;
      this.page = null;
      this.group = this.group || this.slug;
      document.documentElement.lang = this.locale;
      const translations = await this.fetchTranslations(this.group);
      this.setTitle(title, '아직 생성되지 않은 문서입니다.');
      this.setReadMeta(null);
      article.innerHTML = `<div class="tp-empty"><strong>아직 없는 문서입니다.</strong><p>이 제목으로 새 기술 문서를 작성할 수 있습니다.</p></div>`;
      this.updateToc([]);
      this.updateRelated(article);
      this.renderLanguageSwitch({
        locale: this.locale,
        group: this.group,
        slug: this.slug,
        title,
        translations,
      });
      titleInput.value = title;
      urlSlugInput.value = this.slug;
      sourceInput.value = this.defaultSource(title);
      groupInput.value = this.group;
      localeInput.value = this.localeName(this.locale);
      this.openEditor(false);
      return;
    }

    if (!res.ok) {
      this.page = null;
      this.setTitle(this.slug, '문서 로드 실패');
      this.setReadMeta(null);
      article.innerHTML = `<div class="tp-empty"><strong>문서를 불러오지 못했습니다.</strong><p>응답 코드: ${this.escapeHTML(res.status)}</p></div>`;
      this.updateToc([]);
      this.updateRelated(article);
      this.renderLanguageSwitch(null);
      this.setStatus('문서를 불러오지 못했습니다.', 'error');
      return;
    }

    let page;
    try {
      page = await res.json();
    } catch (error) {
      this.page = null;
      this.setTitle(this.slug, '문서 로드 실패');
      this.setReadMeta(null);
      article.innerHTML = `<div class="tp-empty"><strong>문서 응답을 해석할 수 없습니다.</strong></div>`;
      this.updateToc([]);
      this.updateRelated(article);
      this.renderLanguageSwitch(null);
      this.setStatus('문서 응답을 해석할 수 없습니다.', 'error');
      return;
    }

    this.page = page;
    this.locale = page.locale || this.locale;
    this.group = page.group || this.group || this.slug;
    document.documentElement.lang = this.locale;
    this.setTitle(page.title || this.slug, this.buildSubtitle(page));
    this.setReadMeta(page);
    this.renderLanguageSwitch(page);

    article.innerHTML = page.html || '<p class="tp-empty-paragraph">본문이 비어 있습니다.</p>';
    this.enhanceArticle(article);
    this.updateRelated(article);

    titleInput.value = page.title || this.slug;
    urlSlugInput.value = this.slug;
    sourceInput.value = page.source || '';
    groupInput.value = this.group;
    localeInput.value = this.localeName(this.locale);
    this.closeEditor(false);
    this.setStatus('', '');
  }

  async fetchTranslations(group) {
    const normalizedGroup = String(group || '').trim();
    if (!normalizedGroup) {
      return {};
    }

    try {
      const res = await fetch(`/api/translations/${encodeURIComponent(normalizedGroup)}`);
      if (!res.ok) {
        return {};
      }
      const payload = await res.json();
      this.group = payload.group || normalizedGroup;
      return payload.translations || {};
    } catch (error) {
      return {};
    }
  }

  renderLoading() {
    this.querySelector('#article').innerHTML = `
      <div class="tp-loading">
        <span></span>
        <span></span>
        <span></span>
      </div>
    `;
    this.querySelector('#toc-list').innerHTML = '<p>목차를 준비하는 중입니다.</p>';
    this.querySelector('#related-box').hidden = true;
  }

  defaultSource(title) {
    const translationNote = this.translationSourceNote();
    if (this.locale === 'en') {
      return `# ${title}

${translationNote ? `${translationNote}\n` : ''}
Start with the core explanation of ${title}.

<wiki-toc></wiki-toc>

## Overview

Explain what it is and why it matters.

## Characteristics

- Key characteristic
- Common use cases

## Related pages

- [[main|Main page]]
`;
    }

    return `# ${title}

${translationNote ? `${translationNote}\n` : ''}
${title}에 대한 핵심 설명을 먼저 작성하세요.

<wiki-toc></wiki-toc>

## 개요

무엇인지, 왜 중요한지 설명합니다.

## 특징

- 핵심 특징
- 사용되는 분야

## 관련 문서

- [[main|대문]]
`;
  }

  translationSourceNote() {
    const params = new URLSearchParams(window.location.search);
    const fromLocale = params.get('from_locale');
    const fromSlug = params.get('from_slug');
    const fromTitle = params.get('from_title') || fromSlug;
    if (!fromLocale || !fromSlug) {
      return '';
    }

    if (this.locale === 'en') {
      return `Translation draft connected to the ${this.localeName(fromLocale)} page: /${fromLocale}/wiki/${fromSlug} (${fromTitle})`;
    }

    return `${this.localeName(fromLocale)} 문서와 연결된 번역 초안입니다: /${fromLocale}/wiki/${fromSlug} (${fromTitle})`;
  }

  createTitleFromURL() {
    const title = new URLSearchParams(window.location.search).get('title');
    return title?.trim() || '';
  }

  buildSubtitle(page) {
    const updated = this.formatDate(page?.updated_at);
    if (!page?.revision) {
      return `최근 수정 ${updated}`;
    }
    return `최근 수정 ${updated} · ${page.revision}번째 판`;
  }

  setTitle(title, subtitle) {
    this.querySelector('#page-title').textContent = title;
    this.querySelector('#page-subtitle').textContent = subtitle;
    this.querySelector('#crumb-locale').textContent = this.localeName(this.locale);
    this.querySelector('#crumb-slug').textContent = this.slug;
    document.title = `${title} - TechPedia`;
  }

  renderLanguageSwitch(page) {
    const nav = this.querySelector('#language-switch');
    const list = this.querySelector('#language-list');
    if (!nav) return;

    nav.replaceChildren();
    if (list) list.replaceChildren();

    const label = document.createElement('span');
    label.className = 'tp-language-label';
    nav.append(label);

    const entries = this.languageEntries(page);
    const existingCount = entries.filter((entry) => entry.exists).length;
    label.textContent = `언어 ${existingCount}/${entries.length}`;

    for (const entry of entries) {
      nav.append(this.createLanguagePill(entry));
      if (list) {
        list.append(this.createLanguageRow(entry));
      }
    }
  }

  languageEntries(page) {
    const group = page?.group || this.group || this.slug;
    const title = page?.title || this.createTitleFromURL() || this.slug;
    const translations = page?.translations || {};

    return this.supportedLocales.map((locale) => {
      const translation = translations[locale];
      const exists = Boolean(translation?.exists);
      const slug = exists ? translation.slug : this.translationDraftSlug(group, this.slug);
      const entryTitle = exists ? translation.title : this.translationDraftTitle(locale, group, title);
      const params = exists
        ? {}
        : {
            group,
            title: entryTitle,
            from_locale: this.locale,
            from_slug: page?.slug || this.slug,
            from_title: title,
          };

      return {
        locale,
        slug,
        title: entryTitle,
        href: this.makeWikiPath(slug, locale, params),
        exists,
        active: locale === this.locale,
      };
    });
  }

  createLanguagePill(entry) {
    const link = document.createElement('a');
    link.href = entry.href;
    link.lang = entry.locale;
    link.className = [
      entry.active ? 'active' : '',
      entry.exists ? 'exists' : 'missing',
    ].filter(Boolean).join(' ');
    link.title = entry.exists
      ? `${this.localeName(entry.locale)}: ${entry.title}`
      : `${this.localeName(entry.locale)} 번역 만들기`;

    const code = document.createElement('span');
    code.className = 'tp-language-code';
    code.textContent = entry.locale.toUpperCase();

    const name = document.createElement('span');
    name.className = 'tp-language-name';
    name.textContent = this.localeName(entry.locale);

    link.append(code, name);
    return link;
  }

  createLanguageRow(entry) {
    const link = document.createElement('a');
    link.href = entry.href;
    link.lang = entry.locale;
    link.className = [
      'tp-language-row',
      entry.active ? 'active' : '',
      entry.exists ? 'exists' : 'missing',
    ].filter(Boolean).join(' ');

    const code = document.createElement('span');
    code.className = 'tp-language-code';
    code.textContent = entry.locale.toUpperCase();

    const body = document.createElement('span');
    body.className = 'tp-language-row-body';

    const name = document.createElement('strong');
    name.textContent = this.localeName(entry.locale);

    const detail = document.createElement('small');
    detail.textContent = entry.exists
      ? `${entry.title || entry.slug} · /${entry.locale}/wiki/${entry.slug}`
      : `/${entry.locale}/wiki/${entry.slug} 생성`;

    body.append(name, detail);

    const state = document.createElement('span');
    state.className = 'tp-language-state';
    state.textContent = entry.active ? '현재' : entry.exists ? '이동' : '생성';

    link.append(code, body, state);
    return link;
  }

  translationDraftSlug(group, fallback) {
    const candidate = this.slugifyDocument(group || fallback || 'main');
    return candidate || 'main';
  }

  translationDraftTitle(locale, group, fallbackTitle) {
    if (locale === this.locale) {
      return fallbackTitle;
    }

    const groupTitle = this.titleFromGroup(group);
    return groupTitle || fallbackTitle;
  }

  titleFromGroup(group) {
    const value = String(group || '').trim();
    if (!/^[a-z0-9][a-z0-9-]*$/i.test(value)) {
      return '';
    }

    return value
      .split('-')
      .filter(Boolean)
      .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
      .join(' ');
  }

  slugifyDocument(value) {
    const text = String(value || '').trim().toLowerCase();
    let result = '';
    let lastDash = false;

    for (const char of text) {
      if (/[\p{Letter}\p{Number}]/u.test(char)) {
        result += char;
        lastDash = false;
        continue;
      }
      if (char === '+') {
        if (result && !lastDash) result += '-';
        result += 'plus';
        lastDash = false;
        continue;
      }
      if (char === '#') {
        if (result && !lastDash) result += '-';
        result += 'sharp';
        lastDash = false;
        continue;
      }
      if ((char === '-' || char === '_' || char === '.' || /\s/u.test(char)) && result && !lastDash) {
        result += '-';
        lastDash = true;
      }
    }

    return result.replace(/^-+|-+$/g, '') || 'untitled';
  }

  setReadMeta(page) {
    const meta = this.querySelector('#read-meta');
    if (!page) {
      meta.textContent = '새 문서';
      return;
    }

    const updated = this.formatDate(page.updated_at);
    const revision = page.revision ? `${page.revision}번째 판` : '판 정보 없음';
    meta.textContent = `최근 수정: ${updated} · ${revision}`;
  }

  formatDate(value) {
    if (!value) return '-';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) {
      return String(value);
    }
    return date.toLocaleString('ko-KR', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    });
  }

  enhanceArticle(article) {
    const headings = Array.from(article.querySelectorAll('h1, h2, h3'));
    const usedIds = new Set();

    headings.forEach((heading) => {
      const baseId = this.slugifyHeading(heading.textContent) || 'section';
      let id = baseId;
      let index = 2;

      while (usedIds.has(id) || article.querySelector(`#${CSS.escape(id)}`)) {
        id = `${baseId}-${index}`;
        index += 1;
      }

      usedIds.add(id);
      heading.id = id;
    });

    article.querySelectorAll('wiki-toc').forEach((toc) => {
      if (typeof toc.refresh === 'function') {
        toc.refresh(headings);
      }
    });

    this.updateToc(headings);
    this.resolveArticleLinks(article);
  }

  async resolveArticleLinks(article) {
    const links = Array.from(article.querySelectorAll('a.wiki-link'));
    if (links.length === 0) {
      return;
    }

    const slugs = [];
    const bySlug = new Map();
    for (const link of links) {
      const slug = this.slugFromWikiHref(link.getAttribute('href'));
      if (!slug) continue;
      link.dataset.wikiSlug = slug;
      if (!bySlug.has(slug)) {
        bySlug.set(slug, []);
        slugs.push(slug);
      }
      bySlug.get(slug).push(link);
    }

    if (slugs.length === 0) {
      return;
    }

    let payload;
    try {
      const params = new URLSearchParams({ locale: this.locale });
      slugs.forEach((slug) => params.append('target', slug));
      const res = await fetch(`/api/links/resolve?${params.toString()}`);
      if (!res.ok) return;
      payload = await res.json();
    } catch (error) {
      return;
    }

    const resolved = new Map((payload.links || []).map((link) => [link.slug, link]));
    for (const [slug, linkElements] of bySlug.entries()) {
      const info = resolved.get(slug);
      for (const link of linkElements) {
        if (!info || !info.exists) {
          link.classList.add('missing');
          link.href = this.makeWikiPath(slug, this.locale, { title: link.textContent.trim() || slug });
          link.title = '아직 없는 문서입니다. 클릭하면 새 문서를 만들 수 있습니다.';
          continue;
        }
        link.classList.add('exists');
        link.href = info.url || this.makeWikiPath(slug, this.locale);
        link.title = info.title || link.textContent.trim();
      }
    }
  }

  slugFromWikiHref(href) {
    if (!href) return '';
    try {
      const url = new URL(href, window.location.origin);
      const match = url.pathname.match(/^\/[^/]+\/wiki\/(.+)$/);
      if (!match) return '';
      return decodeURIComponent(match[1]);
    } catch (error) {
      return '';
    }
  }

  updateToc(headings) {
    const toc = this.querySelector('#toc-list');
    toc.replaceChildren();

    if (!headings.length) {
      const empty = document.createElement('p');
      empty.textContent = '목차가 없습니다.';
      toc.append(empty);
      return;
    }

    const counters = { 1: 0, 2: 0, 3: 0 };
    headings.forEach((heading) => {
      const level = Number(heading.tagName.slice(1));
      counters[level] += 1;
      for (let next = level + 1; next <= 3; next += 1) {
        counters[next] = 0;
      }

      const number = [];
      for (let current = 1; current <= level; current += 1) {
        if (counters[current] > 0) number.push(counters[current]);
      }

      const link = document.createElement('a');
      link.href = `#${heading.id}`;
      link.className = `level-${level}`;

      const index = document.createElement('span');
      index.textContent = number.join('.');

      const label = document.createElement('strong');
      label.textContent = heading.textContent.trim();

      link.append(index, label);
      toc.append(link);
    });
  }

  updateRelated(article) {
    const box = this.querySelector('#related-box');
    const list = this.querySelector('#related-list');
    const links = Array.from(article.querySelectorAll('a.wiki-link'));
    const seen = new Set();

    list.replaceChildren();
    links.slice(0, 8).forEach((sourceLink) => {
      const href = sourceLink.getAttribute('href');
      const text = sourceLink.textContent.trim();
      if (!href || !text || seen.has(href)) return;
      seen.add(href);

      const link = document.createElement('a');
      link.href = href;
      link.textContent = text;
      list.append(link);
    });

    box.hidden = seen.size === 0;
  }

  slugifyHeading(value) {
    return String(value ?? '')
      .trim()
      .toLowerCase()
      .replace(/[^\p{Letter}\p{Number}]+/gu, '-')
      .replace(/^-+|-+$/g, '');
  }

  openEditor(scroll = true) {
    this.mode = 'edit';
    this.querySelector('#read-view').hidden = true;
    this.querySelector('#edit-view').hidden = false;
    this.querySelector('#history-view').hidden = true;
    this.setSelectedTab('open-editor');
    const group = this.querySelector('#group')?.value.trim() || this.group || this.slug;
    const urlSlug = this.querySelector('#url-slug')?.value.trim() || this.slug;
    this.querySelector('#edit-summary').textContent = this.page
      ? `${this.page.title || this.slug} 문서를 편집합니다. URL: /${this.locale}/wiki/${urlSlug} · 번역 그룹: ${group}`
      : `새 문서를 작성합니다. URL: /${this.locale}/wiki/${urlSlug} · 번역 그룹: ${group}`;

    if (scroll) {
      this.querySelector('#edit-view').scrollIntoView({ behavior: 'smooth', block: 'start' });
    }
  }

  closeEditor(clearStatus = true) {
    this.mode = 'read';
    this.querySelector('#read-view').hidden = false;
    this.querySelector('#edit-view').hidden = true;
    this.querySelector('#history-view').hidden = true;
    this.setSelectedTab('read-mode');
    if (clearStatus) this.setStatus('', '');
  }

  async openHistory() {
    this.mode = 'history';
    this.querySelector('#read-view').hidden = true;
    this.querySelector('#edit-view').hidden = true;
    this.querySelector('#history-view').hidden = false;
    this.setSelectedTab('open-history');

    const list = this.querySelector('#history-list');
    list.innerHTML = '<p>역사를 불러오는 중입니다.</p>';

    let res;
    try {
      res = await fetch(`${this.apiPagePath()}/revisions?limit=30`);
    } catch (error) {
      list.innerHTML = '<p class="tp-history-empty">문서 역사를 불러오지 못했습니다.</p>';
      return;
    }

    if (!res.ok) {
      list.innerHTML = '<p class="tp-history-empty">문서 역사를 불러오지 못했습니다.</p>';
      return;
    }

    const payload = await res.json();
    const revisions = Array.isArray(payload.revisions) ? payload.revisions : [];
    list.replaceChildren();

    if (revisions.length === 0) {
      const empty = document.createElement('p');
      empty.className = 'tp-history-empty';
      empty.textContent = '저장된 판이 없습니다.';
      list.append(empty);
      return;
    }

    for (const revision of revisions) {
      const item = document.createElement('article');
      item.className = 'tp-history-item';

      const title = document.createElement('strong');
      title.textContent = `${revision.title || this.slug} · ${revision.revision || '판 정보 없음'}`;

      const meta = document.createElement('span');
      const author = revision.author ? ` · ${revision.author}` : '';
      meta.textContent = `${this.formatDate(revision.updated_at)}${author}`;

      const actions = document.createElement('div');
      actions.className = 'tp-history-actions';

      const view = document.createElement('button');
      view.type = 'button';
      view.dataset.action = 'view-revision';
      view.dataset.revision = revision.revision || '';
      view.textContent = '이 판 보기';
      actions.append(view);

      item.append(title, meta, actions);
      list.append(item);
    }
  }

  async viewRevision(revision) {
    if (!revision) return;

    let res;
    try {
      res = await fetch(`${this.apiPagePath()}/revisions/${encodeURIComponent(revision)}`);
    } catch (error) {
      this.setStatus('해당 판을 불러오지 못했습니다.', 'error');
      return;
    }

    if (!res.ok) {
      this.setStatus('해당 판을 불러오지 못했습니다.', 'error');
      return;
    }

    const page = await res.json();
    const article = this.querySelector('#article');
    this.querySelector('#read-view').hidden = false;
    this.querySelector('#edit-view').hidden = true;
    this.querySelector('#history-view').hidden = true;
    this.setSelectedTab('read-mode');
    this.setTitle(page.title || this.slug, `이전 판 · ${this.formatDate(page.updated_at)} · ${page.revision}`);
    this.setReadMeta(page);
    article.innerHTML = page.html || '<p class="tp-empty-paragraph">본문이 비어 있습니다.</p>';
    this.enhanceArticle(article);
    this.updateRelated(article);
    this.setStatus('현재 최신 문서가 아니라 선택한 과거 판을 보고 있습니다.', 'info');
  }

  setSelectedTab(action) {
    this.querySelectorAll('.tp-tab').forEach((tab) => {
      tab.setAttribute('aria-selected', tab.dataset.action === action ? 'true' : 'false');
    });
  }

  selectPlaceholderTab(action, message) {
    this.querySelector('#read-view').hidden = false;
    this.querySelector('#edit-view').hidden = true;
    this.querySelector('#history-view').hidden = true;
    this.setSelectedTab(action);
    this.setStatus(message, 'info');
  }

  setStatus(message, type = '') {
    const banner = this.querySelector('#status-banner');
    const saveStatus = this.querySelector('#save-status');

    banner.textContent = message;
    banner.className = `tp-message ${type}`.trim();
    banner.hidden = !message;

    saveStatus.textContent = message;
    saveStatus.className = `tp-save-status ${type}`.trim();
  }

  insertSnippet(kind) {
    const source = this.querySelector('#source');
    const selection = source.value.slice(source.selectionStart, source.selectionEnd);
    const selected = selection || '내용';
    const snippets = {
      heading: `\n## ${selected}\n\n`,
      bold: `**${selected}**`,
      link: `[[${selection || '문서명'}|${selection || '표시 이름'}]]`,
      toc: '\n<wiki-toc></wiki-toc>\n',
      alert: '\n<wiki-alert type="info">\n  안내 내용을 작성하세요.\n</wiki-alert>\n',
      infobox: '\n<wiki-infobox title="정보">\n| 종류 = 값\n| 개발자 = 값\n| 최초 공개 = 값\n</wiki-infobox>\n',
      table: '\n| 항목 | 내용 |\n|------|------|\n| 이름 | 값 |\n',
      code: '\n```text\n코드를 입력하세요.\n```\n',
    };

    const snippet = snippets[kind];
    if (!snippet) return;

    source.setRangeText(snippet, source.selectionStart, source.selectionEnd, 'end');
    source.focus();
  }

  applyTemplate(kind) {
    const source = this.querySelector('#source');
    const title = this.querySelector('#title').value.trim() || this.createTitleFromURL() || this.slug;
    if (source.value.trim() && !window.confirm('현재 편집 중인 본문을 템플릿으로 교체할까요?')) {
      return;
    }

    const templates = {
      language: this.languageTemplate(title),
      person: this.personTemplate(title),
      company: this.companyTemplate(title),
      software: this.softwareTemplate(title),
    };
    if (!templates[kind]) return;
    source.value = templates[kind];
    source.focus();
  }

  languageTemplate(title) {
    return `# ${title}

**${title}**은 프로그래밍 언어이다.

<wiki-infobox title="${title}" type="language">
| 종류 = 프로그래밍 언어
| 설계자 =
| 최초 공개 =
| 패러다임 =
</wiki-infobox>

<wiki-toc></wiki-toc>

## 개요

${title}의 목적과 특징을 설명한다.

## 역사

개발 배경과 주요 버전을 정리한다.

## 특징

- 핵심 특징

## 관련 문서

- [[programming|프로그래밍]]
`;
  }

  personTemplate(title) {
    return `# ${title}

**${title}**은 기술 분야의 인물이다.

<wiki-infobox title="${title}" type="person">
| 분류 = 인물
| 직업 =
| 주요 활동 =
| 관련 기술 =
</wiki-infobox>

<wiki-toc></wiki-toc>

## 개요

인물의 주요 활동과 기술적 기여를 설명한다.

## 생애

## 기여

## 관련 문서

- [[programming|프로그래밍]]
`;
  }

  companyTemplate(title) {
    return `# ${title}

**${title}**은 기술 기업 또는 조직이다.

<wiki-infobox title="${title}" type="company">
| 분류 = 기업
| 설립 =
| 창립자 =
| 주요 제품 =
</wiki-infobox>

<wiki-toc></wiki-toc>

## 개요

기업의 기술적 역할과 주요 제품을 설명한다.

## 역사

## 제품 및 서비스

## 관련 문서

- [[programming|프로그래밍]]
`;
  }

  softwareTemplate(title) {
    return `# ${title}

**${title}**은 소프트웨어이다.

<wiki-infobox title="${title}" type="software">
| 분류 = 소프트웨어
| 개발자 =
| 최초 공개 =
| 라이선스 =
</wiki-infobox>

<wiki-toc></wiki-toc>

## 개요

소프트웨어의 목적과 사용 분야를 설명한다.

## 특징

## 구조

## 관련 문서

- [[programming|프로그래밍]]
`;
  }

  async savePage(retryAuth = true) {
    const title = this.querySelector('#title').value.trim() || this.slug;
    const source = this.querySelector('#source').value;
    const group = this.querySelector('#group').value.trim() || this.group || this.slug;
    const slugInput = this.querySelector('#url-slug');
    const saveSlug = this.slugifyDocument(slugInput.value || title || this.slug);
    slugInput.value = saveSlug;
    this.group = group;

    this.setStatus('저장 중...', 'info');

    let res;
    try {
      const headers = { 'Content-Type': 'application/json' };
      const token = window.localStorage.getItem('techpedia-admin-token');
      if (token) {
        headers['X-TechPedia-Admin-Token'] = token;
      }

      res = await fetch(this.apiPagePath(this.locale, saveSlug), {
        method: 'PUT',
        headers,
        body: JSON.stringify({ title, source, author: '', group }),
      });
    } catch (error) {
      this.setStatus('저장 실패: 서버에 연결할 수 없습니다.', 'error');
      return;
    }

    if (res.status === 401 && retryAuth) {
      const token = window.prompt('문서를 저장하려면 TechPedia 관리자 토큰이 필요합니다.');
      if (token) {
        window.localStorage.setItem('techpedia-admin-token', token.trim());
        return this.savePage(false);
      }
      this.setStatus('저장 취소: 관리자 토큰이 필요합니다.', 'error');
      return;
    }

    if (!res.ok) {
      this.setStatus('저장 실패', 'error');
      return;
    }

    if (saveSlug !== this.slug) {
      this.slug = saveSlug;
      window.history.pushState({}, '', this.makeWikiPath(saveSlug, this.locale));
    }

    await this.loadPage();
    this.setStatus('저장 완료', 'ok');
  }

  async performSearch(query, forceOpen) {
    if (this.searchAbortController) {
      this.searchAbortController.abort();
    }

    const controller = new AbortController();
    this.searchAbortController = controller;

    try {
      const res = await fetch(`/api/search?locale=${encodeURIComponent(this.locale)}&q=${encodeURIComponent(query)}&limit=8`, {
        signal: controller.signal,
      });
      if (!res.ok) {
        this.renderSearchError(query);
        return;
      }

      const payload = await res.json();
      this.renderSearchResults(payload, forceOpen);
    } catch (error) {
      if (error.name === 'AbortError') {
        return;
      }
      this.renderSearchError(query);
    }
  }

  renderSearchResults(payload, forceOpen) {
    const box = this.querySelector('#search-results');
    const input = this.querySelector('#search-input');
    const results = Array.isArray(payload.results) ? payload.results : [];
    const query = payload.query || input.value.trim();

    box.replaceChildren();

    const list = document.createElement('div');
    list.className = 'tp-search-list';

    if (results.length === 0) {
      const empty = document.createElement('div');
      empty.className = 'tp-search-empty';
      empty.textContent = '검색 결과가 없습니다.';
      list.append(empty);
    }

    for (const result of results) {
      const link = document.createElement('a');
      link.className = 'tp-search-result';
      link.href = this.makeWikiPath(result.slug, result.locale || this.locale);

      const title = document.createElement('strong');
      title.textContent = result.title || result.slug;

      const snippet = document.createElement('span');
      snippet.textContent = result.snippet || result.slug;

      link.append(title, snippet);
      list.append(link);
    }

    if (query && payload.create && !payload.exact_match) {
      const create = document.createElement('a');
      create.className = 'tp-search-create';
      create.href = this.makeWikiPath(payload.create.slug, payload.create.locale || this.locale, { title: payload.create.title });
      create.textContent = `"${payload.create.title}" 문서 만들기`;
      list.append(create);
    }

    box.append(list);
    box.hidden = !forceOpen && results.length === 0 && !query;
    input.setAttribute('aria-expanded', box.hidden ? 'false' : 'true');
  }

  renderSearchError(query) {
    const box = this.querySelector('#search-results');
    const input = this.querySelector('#search-input');
    box.innerHTML = '';

    const list = document.createElement('div');
    list.className = 'tp-search-list';

    const error = document.createElement('div');
    error.className = 'tp-search-empty';
    error.textContent = '검색을 실행할 수 없습니다.';
    list.append(error);

    const slug = this.slugifyHeading(query);
    if (slug) {
      const create = document.createElement('a');
      create.className = 'tp-search-create';
      create.href = this.makeWikiPath(slug, this.locale, { title: query });
      create.textContent = `"${query}" 문서 만들기`;
      list.append(create);
    }

    box.append(list);
    box.hidden = false;
    input.setAttribute('aria-expanded', 'true');
  }

  clearSearchResults() {
    const box = this.querySelector('#search-results');
    const input = this.querySelector('#search-input');
    if (!box || !input) {
      return;
    }
    box.hidden = true;
    input.setAttribute('aria-expanded', 'false');
  }

  async copyLink() {
    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(window.location.href);
      } else {
        const textarea = document.createElement('textarea');
        textarea.value = window.location.href;
        textarea.setAttribute('readonly', '');
        textarea.style.position = 'fixed';
        textarea.style.top = '-999px';
        document.body.append(textarea);
        textarea.select();
        document.execCommand('copy');
        textarea.remove();
      }
      this.setStatus('문서 링크를 복사했습니다.', 'ok');
    } catch (error) {
      this.setStatus('복사 권한을 얻을 수 없습니다.', 'error');
    }
  }
}

if (!customElements.get('techpedia-app')) {
  customElements.define('techpedia-app', TechPediaApp);
}
