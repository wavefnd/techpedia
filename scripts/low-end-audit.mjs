#!/usr/bin/env node

import { spawn } from 'node:child_process'
import { existsSync, mkdtempSync, readFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const args = process.argv.slice(2)
const desktop = args.includes('--desktop')
const target = args.find((argument) => !argument.startsWith('--'))
if (!target) {
  console.error('Usage: node scripts/low-end-audit.mjs [--desktop] http://127.0.0.1:8080/ko/wiki/Page')
  process.exit(2)
}

const browserPath = process.env.CHROMIUM_PATH || [
  '/usr/bin/chromium-browser',
  '/usr/bin/chromium',
  '/usr/bin/google-chrome',
].find(existsSync)

if (!browserPath) {
  console.error('Chromium was not found. Set CHROMIUM_PATH to a Chromium-compatible executable.')
  process.exit(2)
}

const profile = mkdtempSync(join(tmpdir(), 'techpedia-low-end-'))
const browser = spawn(browserPath, [
  '--headless=new',
  '--no-sandbox',
  '--disable-background-networking',
  '--disable-component-update',
  '--disable-default-apps',
  '--disable-extensions',
  '--disable-sync',
  '--metrics-recording-only',
  '--no-first-run',
  '--remote-debugging-port=0',
  `--user-data-dir=${profile}`,
  'about:blank',
], { stdio: 'ignore' })

const sleep = (milliseconds) => new Promise((resolve) => setTimeout(resolve, milliseconds))

async function devtoolsPort() {
  const path = join(profile, 'DevToolsActivePort')
  for (let attempt = 0; attempt < 100; attempt++) {
    if (existsSync(path)) return Number(readFileSync(path, 'utf8').split('\n')[0])
    if (browser.exitCode !== null) throw new Error(`Chromium exited with status ${browser.exitCode}`)
    await sleep(100)
  }
  throw new Error('Timed out waiting for the Chromium debugging port')
}

async function pageSocket(port) {
  for (let attempt = 0; attempt < 100; attempt++) {
    try {
      const pages = await fetch(`http://127.0.0.1:${port}/json/list`).then((response) => response.json())
      const page = pages.find((entry) => entry.type === 'page')
      if (page?.webSocketDebuggerUrl) return page.webSocketDebuggerUrl
    } catch {}
    await sleep(100)
  }
  throw new Error('Timed out waiting for a Chromium page target')
}

class CDP {
  constructor(url) {
    this.socket = new WebSocket(url)
    this.sequence = 0
    this.pending = new Map()
    this.listeners = new Map()
  }

  async connect() {
    await new Promise((resolve, reject) => {
      this.socket.addEventListener('open', resolve, { once: true })
      this.socket.addEventListener('error', reject, { once: true })
    })
    this.socket.addEventListener('message', (event) => {
      const message = JSON.parse(event.data)
      if (message.id) {
        const pending = this.pending.get(message.id)
        if (!pending) return
        this.pending.delete(message.id)
        if (message.error) pending.reject(new Error(message.error.message))
        else pending.resolve(message.result)
        return
      }
      for (const listener of this.listeners.get(message.method) || []) listener(message.params)
    })
  }

  send(method, params = {}) {
    const id = ++this.sequence
    this.socket.send(JSON.stringify({ id, method, params }))
    return new Promise((resolve, reject) => this.pending.set(id, { resolve, reject }))
  }

  on(method, listener) {
    const listeners = this.listeners.get(method) || []
    listeners.push(listener)
    this.listeners.set(method, listeners)
  }

  once(method, timeout = 30000) {
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error(`Timed out waiting for ${method}`)), timeout)
      const listener = (params) => {
        clearTimeout(timer)
        this.listeners.set(method, (this.listeners.get(method) || []).filter((value) => value !== listener))
        resolve(params)
      }
      this.on(method, listener)
    })
  }

  close() {
    this.socket.close()
  }
}

let client
try {
  const port = await devtoolsPort()
  client = new CDP(await pageSocket(port))
  await client.connect()

  const resources = new Map()
  const browserErrors = []
  const expectedBrowserErrors = []
  client.on('Network.responseReceived', ({ requestId, response, type }) => {
    resources.set(requestId, { url: response.url, type, bytes: 0, status: response.status })
  })
  client.on('Network.loadingFinished', ({ requestId, encodedDataLength }) => {
    const resource = resources.get(requestId)
    if (resource) resource.bytes = encodedDataLength
  })
  client.on('Log.entryAdded', ({ entry }) => {
    if (entry.level === 'error') {
      const location = entry.url ? ` (${entry.url}${entry.lineNumber ? `:${entry.lineNumber}` : ''})` : ''
      const message = `${entry.source}: ${entry.text}${location}`
      if (entry.url?.endsWith('/api/auth/session') && entry.text.includes('401')) expectedBrowserErrors.push(message)
      else browserErrors.push(message)
    }
  })
  client.on('Runtime.exceptionThrown', ({ exceptionDetails }) => {
    const frame = exceptionDetails.stackTrace?.callFrames?.[0]
    const location = frame ? ` (${frame.url}:${frame.lineNumber + 1}:${frame.columnNumber + 1})` : ''
    browserErrors.push(`javascript: ${exceptionDetails.text}${location}`)
  })

  await Promise.all([
    client.send('Page.enable'),
    client.send('Network.enable'),
    client.send('Performance.enable'),
    client.send('Log.enable'),
    client.send('Runtime.enable'),
  ])
  await client.send('Network.setCacheDisabled', { cacheDisabled: true })
  await client.send('Emulation.setDeviceMetricsOverride', {
    width: desktop ? 1440 : 360,
    height: desktop ? 900 : 640,
    deviceScaleFactor: desktop ? 1 : 2,
    mobile: !desktop,
  })
  await client.send('Emulation.setCPUThrottlingRate', { rate: desktop ? 1 : 6 })
  await client.send('Emulation.setEmulatedMedia', {
    media: 'screen',
    features: [{ name: 'prefers-reduced-motion', value: 'reduce' }],
  })
  await client.send('Network.emulateNetworkConditions', {
    offline: false,
    latency: desktop ? 40 : 400,
    downloadThroughput: (desktop ? 10 * 1024 : 400) * 1024 / 8,
    uploadThroughput: (desktop ? 5 * 1024 : 400) * 1024 / 8,
    connectionType: desktop ? 'wifi' : 'cellular3g',
  })
  await client.send('Page.addScriptToEvaluateOnNewDocument', {
    source: `
      window.__techpediaAudit = { cls: 0, longTasks: [], contentTransitions: [] };
      try { new PerformanceObserver((list) => { for (const entry of list.getEntries()) window.__techpediaAudit.longTasks.push(entry.duration); }).observe({ type: 'longtask', buffered: true }); } catch {}
      try { new PerformanceObserver((list) => { for (const entry of list.getEntries()) if (!entry.hadRecentInput) window.__techpediaAudit.cls += entry.value; }).observe({ type: 'layout-shift', buffered: true }); } catch {}
      (() => {
        let previousVisible;
        let scheduled = false;
        const sample = () => {
          scheduled = false;
          const content = document.querySelector('.mobile-article-reader .techmark-content, .article-view .techmark-content, .language-portal');
          const characters = content?.textContent?.trim().length || 0;
          const visible = characters >= 20 && !!content && getComputedStyle(content).display !== 'none' && getComputedStyle(content).visibility !== 'hidden';
          if (visible !== previousVisible) {
            window.__techpediaAudit.contentTransitions.push({ at: performance.now(), visible, characters });
            previousVisible = visible;
          }
        };
        const schedule = () => {
          if (scheduled) return;
          scheduled = true;
          requestAnimationFrame(sample);
        };
        new MutationObserver(schedule).observe(document, { childList: true, subtree: true, characterData: true });
        document.addEventListener('DOMContentLoaded', schedule, { once: true });
        schedule();
      })();
    `,
  })

  const loaded = client.once('Page.loadEventFired', 45000)
  const navigationStarted = Date.now()
  await client.send('Page.navigate', { url: new URL(target).href })
  await loaded
  const settleStarted = Date.now()
  let stableSince = 0
  for (;;) {
    const sample = await client.send('Runtime.evaluate', {
      returnByValue: true,
      expression: `(() => {
        const content = document.querySelector('.mobile-article-reader .techmark-content, .article-view .techmark-content, .language-portal');
        return { characters: content?.textContent?.trim().length || 0,
          painted: performance.getEntriesByType('paint').some((entry) => entry.name === 'first-contentful-paint') };
      })()`,
    })
    if (sample.result.value?.characters >= 20 && sample.result.value?.painted) {
      if (!stableSince) stableSince = Date.now()
      if (Date.now() - stableSince >= 750) break
    } else {
      stableSince = 0
    }
    if (Date.now() - settleStarted >= 20000) break
    await sleep(250)
  }
  const settleWait = Date.now() - settleStarted

  const runtime = await client.send('Runtime.evaluate', {
    returnByValue: true,
    expression: `JSON.stringify((() => {
      const navigation = performance.getEntriesByType('navigation')[0];
      const paints = Object.fromEntries(performance.getEntriesByType('paint').map((entry) => [entry.name, entry.startTime]));
      const content = document.querySelector('.mobile-article-reader .techmark-content, .article-view .techmark-content, .language-portal');
      return {
        url: location.href,
        title: document.title,
        fcp: paints['first-contentful-paint'] || 0,
        domContentLoaded: navigation?.domContentLoadedEventEnd || 0,
        load: navigation?.loadEventEnd || 0,
        contentCharacters: content?.textContent?.trim().length || 0,
        domNodes: document.querySelectorAll('*').length,
        horizontalOverflow: Math.max(document.documentElement.scrollWidth, document.body.scrollWidth) - document.documentElement.clientWidth,
        cls: window.__techpediaAudit?.cls || 0,
        longTasks: window.__techpediaAudit?.longTasks || [],
        contentTransitions: window.__techpediaAudit?.contentTransitions || [],
      };
    })())`,
  })
  const performanceMetrics = await client.send('Performance.getMetrics')
  const metrics = Object.fromEntries(performanceMetrics.metrics.map((metric) => [metric.name, metric.value]))
  const result = JSON.parse(runtime.result.value)
  const transferred = [...resources.values()].filter((resource) => resource.url.startsWith(new URL(target).origin))
  const bytesByType = transferred.reduce((totals, resource) => {
    totals[resource.type] = (totals[resource.type] || 0) + resource.bytes
    return totals
  }, {})
  const maxLongTask = Math.max(0, ...result.longTasks)
  const firstVisible = result.contentTransitions.find((transition) => transition.visible)
  const blankAfterFirstVisible = result.contentTransitions.some((transition, index) =>
    !transition.visible && result.contentTransitions.slice(0, index).some((earlier) => earlier.visible),
  )
  const report = {
    profile: desktop
      ? '1440x900, 10 Mbps/40 ms, unthrottled CPU, reduced motion'
      : '360x640, Slow 3G 400 kbps/400 ms, CPU 6x, reduced motion',
    target: result.url,
    fcp_ms: Math.round(result.fcp),
    dom_content_loaded_ms: Math.round(result.domContentLoaded),
    load_ms: Math.round(result.load),
    content_ready_ms: firstVisible ? Math.round(firstVisible.at) : 0,
    content_blank_after_first_paint: blankAfterFirstVisible,
    settle_wait_ms: settleWait,
    audit_wall_ms: Date.now() - navigationStarted,
    transferred_kib: Math.round(transferred.reduce((sum, resource) => sum + resource.bytes, 0) / 1024),
    javascript_kib: Math.round((bytesByType.Script || 0) / 1024),
    css_kib: Math.round((bytesByType.Stylesheet || 0) / 1024),
    requests: transferred.length,
    dom_nodes: result.domNodes,
    content_characters: result.contentCharacters,
    horizontal_overflow_px: Math.max(0, Math.round(result.horizontalOverflow)),
    cls: Number(result.cls.toFixed(3)),
    max_long_task_ms: Math.round(maxLongTask),
    layout_count: metrics.LayoutCount || 0,
    layout_duration_ms: Math.round((metrics.LayoutDuration || 0) * 1000),
    style_recalc_count: metrics.RecalcStyleCount || 0,
    style_recalc_duration_ms: Math.round((metrics.RecalcStyleDuration || 0) * 1000),
    script_duration_ms: Math.round((metrics.ScriptDuration || 0) * 1000),
    task_duration_ms: Math.round((metrics.TaskDuration || 0) * 1000),
    api_requests: transferred
      .map((resource) => new URL(resource.url).pathname)
      .filter((path) => path.startsWith('/api/')),
    browser_errors: browserErrors,
    expected_browser_errors: expectedBrowserErrors,
    content_transitions: result.contentTransitions.map((transition) => ({
      at_ms: Math.round(transition.at),
      visible: transition.visible,
      characters: transition.characters,
    })),
  }
  const checks = {
    content_visible: report.content_characters >= 20,
    content_not_removed_after_first_paint: !report.content_blank_after_first_paint,
    no_horizontal_overflow: report.horizontal_overflow_px <= 1,
    fcp_under_8_seconds: report.fcp_ms > 0 && report.fcp_ms <= 8000,
    javascript_under_240_kib_uncompressed_origin: report.javascript_kib <= 240,
    cls_under_0_1: report.cls <= 0.1,
    max_long_task_within_profile_budget: report.max_long_task_ms <= (desktop ? 500 : 1000),
    no_browser_errors: report.browser_errors.length === 0,
  }

  console.log(JSON.stringify({ report, checks }, null, 2))
  if (Object.values(checks).some((passed) => !passed)) process.exitCode = 1
} catch (error) {
  console.error(error instanceof Error ? error.stack : error)
  process.exitCode = 1
} finally {
  client?.close()
  const closed = browser.exitCode !== null || browser.signalCode !== null
    ? Promise.resolve()
    : new Promise((resolve) => browser.once('exit', resolve))
  browser.kill('SIGTERM')
  await Promise.race([closed, sleep(1000)])
  if (browser.exitCode === null && browser.signalCode === null) {
    browser.kill('SIGKILL')
    await Promise.race([closed, sleep(1000)])
  }
  rmSync(profile, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 })
}
