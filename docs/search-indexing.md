# Search indexing

Search engines decide whether and when to index a page. Correct technical signals
make pages crawlable; they do not guarantee rankings, impressions or recovery.

## Installation checks

1. Set the exact final HTTPS `TECHPEDIA_PUBLIC_URL`.
2. Confirm article GET and HEAD return the expected status and canonical URL.
3. Inspect returned HTML without JavaScript: the title and article body must exist.
4. Check `/robots.txt` and `/sitemap.xml` through the real public proxy.
5. Confirm active articles are listed, not drafts, admin pages or missing articles.
6. Check canonical/hreflang links use real Techdata relationships.
7. Submit the sitemap in Search Console; inspect representative URLs and request
   indexing for corrected public articles where appropriate.

History, editor, account, search and administrator pages are intentionally
excluded. A noindex report for those paths is not an article indexing defect.
Missing articles return 404 even when a creation UI is displayed.

## Old URLs

Legacy MediaWiki paths, underscore/title variants and deleted pages need separate
analysis. Redirect only when there is a real equivalent target. Do not redirect
every 404 to the homepage or remove all noindex rules to improve a coverage count.
Maintain real moved-name and alias redirects.

The sitemap uses stored update times; pages with dynamic boxes/system components
may omit lastmod rather than invent a date. HTML validators use full-response
ETags because edition UI, boxes and links can change independently of revisions.

Compare Search Console by page/query and date, not only a single aggregate chart.
Check actual HTTP responses, deploy history, server availability and editorial
changes before attributing a decline to one code change.

References: [Google sitemap guidance](https://developers.google.com/search/docs/crawling-indexing/sitemaps/build-sitemap),
[canonical URLs](https://developers.google.com/search/docs/crawling-indexing/consolidate-duplicate-urls),
[JavaScript SEO](https://developers.google.com/search/docs/crawling-indexing/javascript/javascript-seo-basics).
