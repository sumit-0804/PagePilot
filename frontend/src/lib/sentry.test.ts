import { describe, expect, it } from 'vitest'
import type { ErrorEvent } from '@sentry/react'
import { pathOnly, scrubBreadcrumb, scrubEvent, stripUrl } from './sentry'

describe('stripUrl', () => {
  it('drops query strings and fragments, which can hold tokens', () => {
    expect(stripUrl('https://app.example/chat?q=my+notes')).toBe('https://app.example/chat')
    expect(stripUrl('https://app.example/#access_token=secret&type=signup')).toBe('https://app.example/')
    expect(stripUrl('/api/documents')).toBe('/api/documents')
  })
})

describe('pathOnly', () => {
  it('replaces ids so reports group by endpoint', () => {
    expect(pathOnly('/documents/6f1c1b8e-6a0e-4b8e-9b8e-1c2d3e4f5a6b/status')).toBe('/documents/:id/status')
    expect(pathOnly('/progress?document_id=6f1c1b8e-6a0e-4b8e-9b8e-1c2d3e4f5a6b')).toBe('/progress')
  })
})

describe('scrubEvent', () => {
  it('removes credentials, request data and the user', () => {
    const event = {
      type: undefined,
      request: {
        url: 'https://app.example/chat/1#access_token=secret',
        query_string: 'q=private',
        cookies: { sb: 'session' },
        data: '{"message":"my private notes"}',
        headers: { Authorization: 'Bearer jwt', 'User-Agent': 'test' },
      },
      user: { id: 'u1', email: 'student@example.com', ip_address: '1.2.3.4' },
    } as ErrorEvent

    const json = JSON.stringify(scrubEvent(event))
    for (const secret of ['secret', 'private', 'session', 'Bearer', 'student@example.com', '1.2.3.4']) {
      expect(json).not.toContain(secret)
    }
    expect(event.request?.url).toBe('https://app.example/chat/1')
    expect(event.request?.headers).toEqual({ 'User-Agent': 'test' })
  })
})

describe('scrubBreadcrumb', () => {
  it('drops console output, which can contain document text', () => {
    expect(scrubBreadcrumb({ category: 'console', message: 'answer text' })).toBeNull()
  })

  it('strips query strings and fragments from fetch and navigation URLs', () => {
    const fetchCrumb = scrubBreadcrumb({ category: 'fetch', data: { url: '/api/progress?document_id=x', status_code: 500 } })
    expect(fetchCrumb?.data).toEqual({ url: '/api/progress', status_code: 500 })
    const nav = scrubBreadcrumb({ category: 'navigation', data: { from: '/login#access_token=t', to: '/' } })
    expect(nav?.data).toEqual({ from: '/login', to: '/' })
  })
})
