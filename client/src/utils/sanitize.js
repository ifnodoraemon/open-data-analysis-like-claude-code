const SAFE_URL_PROTOCOLS = new Set(['http:', 'https:', 'mailto:'])
const MARKDOWN_ALLOWED_TAGS = new Set([
  'A', 'BLOCKQUOTE', 'BR', 'CODE', 'DIV', 'EM', 'H1', 'H2', 'H3', 'H4', 'H5', 'H6',
  'LI', 'OL', 'P', 'PRE', 'SPAN', 'STRONG', 'TABLE', 'TBODY', 'TD', 'TH', 'THEAD', 'TR', 'UL',
])

function sanitizeClassList(value) {
  return String(value || '')
    .split(/\s+/)
    .map(item => item.trim())
    .filter(item => item && /^[A-Za-z0-9:_-]+$/.test(item))
    .join(' ')
}

function sanitizeURL(value) {
  const raw = String(value || '').trim()
  if (!raw) return ''
  if (raw.startsWith('#') || raw.startsWith('/')) return raw
  try {
    const parsed = new URL(raw, window.location.origin)
    return SAFE_URL_PROTOCOLS.has(parsed.protocol) ? raw : ''
  } catch {
    return ''
  }
}

function cleanAttributes(node, { allowMarkdownClasses = false, allowChartStyle = false } = {}) {
  const attrs = Array.from(node.attributes || [])
  for (const attr of attrs) {
    const name = attr.name.toLowerCase()
    const value = attr.value
    if (name.startsWith('on')) {
      node.removeAttribute(attr.name)
      continue
    }
    if (name === 'href' || name === 'src') {
      const safe = sanitizeURL(value)
      if (!safe) {
        node.removeAttribute(attr.name)
      } else {
        node.setAttribute(attr.name, safe)
      }
      if (name === 'href' && node.tagName === 'A') {
        node.setAttribute('rel', 'noopener noreferrer')
        node.setAttribute('target', '_blank')
      }
      continue
    }
    if (name === 'class') {
      const safe = sanitizeClassList(value)
      if (!safe || (!allowMarkdownClasses && node.tagName !== 'BODY')) {
        node.removeAttribute('class')
      } else {
        node.setAttribute('class', safe)
      }
      continue
    }
    if (name === 'style') {
      if (!allowChartStyle || !node.hasAttribute('data-chart-id')) {
        node.removeAttribute('style')
        continue
      }
      const match = String(value || '').match(/height\s*:\s*([0-9.]+(?:px|%|vh|rem))/i)
      if (!match) {
        node.removeAttribute('style')
        continue
      }
      node.setAttribute('style', `height:${match[1]};`)
      continue
    }
    if (name === 'target' || name === 'rel') {
      continue
    }
    if (name.startsWith('data-') || name === 'id' || name === 'title' || name === 'colspan' || name === 'rowspan') {
      continue
    }
    node.removeAttribute(attr.name)
  }
}

function sanitizeTree(root, options = {}) {
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_ELEMENT)
  const toRemove = []
  while (walker.nextNode()) {
    const node = walker.currentNode
    if (options.allowedTags && !options.allowedTags.has(node.tagName)) {
      toRemove.push(node)
      continue
    }
    cleanAttributes(node, options)
  }
  for (const node of toRemove.reverse()) {
    const parent = node.parentNode
    if (!parent) continue
    while (node.firstChild) {
      parent.insertBefore(node.firstChild, node)
    }
    parent.removeChild(node)
  }
}

export function sanitizeMarkdownHTML(html) {
  const parser = new DOMParser()
  const doc = parser.parseFromString(String(html || ''), 'text/html')
  sanitizeTree(doc.body, { allowedTags: MARKDOWN_ALLOWED_TAGS, allowMarkdownClasses: true })
  return doc.body.innerHTML
}

export function sanitizeReportHTML(html) {
  const parser = new DOMParser()
  const doc = parser.parseFromString(String(html || ''), 'text/html')

  doc.querySelectorAll('iframe, object, embed, base, meta[http-equiv]').forEach(node => node.remove())
  doc.querySelectorAll('link').forEach(node => node.remove())
  doc.querySelectorAll('script').forEach((node) => {
    const id = node.getAttribute('id') || ''
    const src = node.getAttribute('src') || ''
    const isSafeLoader = id === 'oda-echarts-loader' && src === 'https://cdn.jsdelivr.net/npm/echarts@5/dist/echarts.min.js'
    const isSafeRuntime = id === 'oda-chart-runtime' && !src
    if (!isSafeLoader && !isSafeRuntime) {
      node.remove()
    }
  })

  const bodyClass = sanitizeClassList(doc.body.getAttribute('class'))
  if (bodyClass) {
    doc.body.setAttribute('class', bodyClass)
  } else {
    doc.body.removeAttribute('class')
  }

  sanitizeTree(doc.documentElement, { allowChartStyle: true })
  return `<!DOCTYPE html>\n${doc.documentElement.outerHTML}`
}
