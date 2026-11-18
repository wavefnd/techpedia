class WikiAlert extends HTMLElement {
  connectedCallback() {
    if (this.shadowRoot) return;

    const labels = {
      info: '안내',
      warning: '주의',
      danger: '경고',
      success: '확인',
      note: '참고',
    };
    const type = labels[this.getAttribute('type')] ? this.getAttribute('type') : 'info';

    const shadow = this.attachShadow({ mode: 'open' });
    shadow.innerHTML = `
      <style>
        :host {
          display: block;
          clear: both;
          margin: 16px 0;
          color: var(--tp-ink, #202122);
          font: inherit;
        }

        .box {
          display: grid;
          grid-template-columns: 86px minmax(0, 1fr);
          border: 1px solid var(--tp-border, #a2a9b1);
          border-left: 4px solid var(--accent, #3366cc);
          background: var(--surface, #f8fbff);
        }

        .label {
          display: grid;
          place-items: center;
          min-height: 44px;
          padding: 10px;
          border-right: 1px solid var(--tp-border-soft, #d8dee4);
          background: rgba(255, 255, 255, 0.62);
          color: var(--accent, #3366cc);
          font-size: 13px;
          font-weight: 800;
        }

        .content {
          min-width: 0;
          padding: 10px 13px;
        }

        .content ::slotted(:first-child) {
          margin-top: 0;
        }

        .content ::slotted(:last-child) {
          margin-bottom: 0;
        }

        .box.info {
          --accent: var(--tp-blue, #3366cc);
          --surface: var(--tp-soft-blue, #eef5ff);
        }

        .box.note {
          --accent: #59636e;
          --surface: #f6f8fa;
        }

        .box.warning {
          --accent: #a45c00;
          --surface: var(--tp-yellow, #fff8df);
        }

        .box.danger {
          --accent: var(--tp-red, #b32424);
          --surface: #fff1f1;
        }

        .box.success {
          --accent: var(--tp-green, #008275);
          --surface: #edf8f6;
        }

        @media (max-width: 560px) {
          .box {
            grid-template-columns: 1fr;
          }

          .label {
            place-items: start;
            border-right: 0;
            border-bottom: 1px solid var(--tp-border-soft, #d8dee4);
          }
        }
      </style>
      <aside class="box ${type}">
        <div class="label"></div>
        <div class="content"><slot></slot></div>
      </aside>
    `;

    shadow.querySelector('.label').textContent = labels[type];
  }
}

class WikiInfobox extends HTMLElement {
  connectedCallback() {
    if (this.shadowRoot) return;

    const title = this.getAttribute('title') || '정보';
    const shadow = this.attachShadow({ mode: 'open' });
    shadow.innerHTML = `
      <style>
        :host {
          display: block;
          float: right;
          width: min(330px, 42%);
          margin: 0 0 18px 24px;
          color: var(--tp-ink, #202122);
          font: inherit;
          font-size: 14px;
          line-height: 1.58;
        }

        .box {
          border: 1px solid var(--tp-border, #a2a9b1);
          background: var(--tp-paper, #ffffff);
          box-shadow: var(--tp-shadow, 0 1px 2px rgba(31, 35, 40, 0.06));
        }

        .title {
          padding: 9px 10px;
          border-bottom: 1px solid var(--tp-border, #a2a9b1);
          background: linear-gradient(180deg, #eef5ff 0, #dce9fb 100%);
          color: #263f68;
          font-weight: 800;
          text-align: center;
        }

        .body {
          min-width: 0;
          padding: 10px 12px;
        }

        .body ::slotted(:first-child) {
          margin-top: 0;
        }

        .body ::slotted(:last-child) {
          margin-bottom: 0;
        }

        ::slotted(img) {
          display: block;
          max-width: 100%;
          height: auto;
          margin: 0 auto 10px;
        }

        ::slotted(table) {
          width: 100% !important;
          margin: 0 !important;
          font-size: 13px !important;
        }

        @media (max-width: 720px) {
          :host {
            float: none;
            width: auto;
            margin: 16px 0;
          }
        }
      </style>
      <aside class="box">
        <div class="title"></div>
        <div class="body"><slot></slot></div>
      </aside>
    `;

    shadow.querySelector('.title').textContent = title;
  }
}

class WikiToc extends HTMLElement {
  connectedCallback() {
    if (this.shadowRoot) return;

    const shadow = this.attachShadow({ mode: 'open' });
    shadow.innerHTML = `
      <style>
        :host {
          display: block;
          clear: both;
          max-width: 360px;
          margin: 16px 0;
          border: 1px solid var(--tp-border, #a2a9b1);
          background: var(--tp-soft, #f8f9fa);
          color: var(--tp-ink, #202122);
          font: inherit;
          font-size: 14px;
        }

        .title {
          padding: 7px 10px;
          border-bottom: 1px solid var(--tp-border-soft, #d8dee4);
          background: #ffffff;
          color: #263f68;
          font-weight: 800;
        }

        .body {
          padding: 8px 12px;
        }

        ol {
          margin: 0;
          padding-left: 20px;
        }

        li {
          margin: 3px 0;
        }

        a {
          color: var(--tp-link, #0645ad);
          text-decoration: none;
        }

        a:hover {
          text-decoration: underline;
        }

        .level-2 {
          margin-left: 12px;
        }

        .level-3 {
          margin-left: 24px;
          font-size: 13px;
        }

        .empty {
          margin: 0;
          color: var(--tp-ink-faint, #72777d);
        }
      </style>
      <nav aria-label="문서 목차">
        <div class="title">목차</div>
        <div class="body"><p class="empty">표시할 제목이 없습니다.</p></div>
      </nav>
    `;
  }

  refresh(headings) {
    const body = this.shadowRoot?.querySelector('.body');
    if (!body) return;

    const items = headings
      .filter((heading) => heading.id && !this.contains(heading))
      .map((heading) => ({
        id: heading.id,
        level: Number(heading.tagName.slice(1)),
        text: heading.textContent.trim(),
      }))
      .filter((item) => item.text);

    body.replaceChildren();

    if (items.length === 0) {
      const empty = document.createElement('p');
      empty.className = 'empty';
      empty.textContent = '표시할 제목이 없습니다.';
      body.append(empty);
      return;
    }

    const list = document.createElement('ol');
    for (const item of items) {
      const entry = document.createElement('li');
      entry.className = `level-${Math.min(item.level, 3)}`;

      const link = document.createElement('a');
      link.href = `#${item.id}`;
      link.textContent = item.text;

      entry.append(link);
      list.append(entry);
    }

    body.append(list);
  }
}

if (!customElements.get('wiki-alert')) {
  customElements.define('wiki-alert', WikiAlert);
}

if (!customElements.get('wiki-infobox')) {
  customElements.define('wiki-infobox', WikiInfobox);
}

if (!customElements.get('wiki-toc')) {
  customElements.define('wiki-toc', WikiToc);
}
