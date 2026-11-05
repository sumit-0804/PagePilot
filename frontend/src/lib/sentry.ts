import * as Sentry from '@sentry/react'
import type { Breadcrumb, ErrorEvent } from '@sentry/react'

/** Whether a DSN was configured; without one every Sentry call is a no-op. */
export const sentryEnabled = Boolean(import.meta.env.VITE_SENTRY_DSN)

export function initSentry() {
  if (!sentryEnabled) return
  const tracesSampleRate = Number(import.meta.env.VITE_SENTRY_TRACES_SAMPLE_RATE ?? 0) || 0
  Sentry.init({
    dsn: import.meta.env.VITE_SENTRY_DSN,
    environment: import.meta.env.VITE_SENTRY_ENVIRONMENT || __APP_ENVIRONMENT__ || import.meta.env.MODE,
    release: __APP_RELEASE__ || undefined,
    // SDK 11 collects most of this by default; PagePilot sends none of it (scrubEvent is the backstop).
    dataCollection: {
      userInfo: false,
      cookies: false,
      httpHeaders: false,
      httpBodies: [],
      urlQueryParams: false,
      genAI: { inputs: false, outputs: false },
      stackFrameVariables: false,
    },
    // Page-load performance only, and off unless asked for. No trace headers go to the API,
    // which would make every request a CORS preflight; API traces live in Langfuse.
    integrations: tracesSampleRate > 0 ? [Sentry.browserTracingIntegration()] : [],
    tracesSampleRate,
    tracePropagationTargets: [],
    beforeSend: scrubEvent,
    beforeBreadcrumb: scrubBreadcrumb,
  })
}

/**
 * Drops the query string and fragment from a URL. Supabase puts access tokens in the fragment
 * after a sign-in redirect, and query strings can hold search text.
 */
export function stripUrl(url: string): string {
  const cut = url.search(/[?#]/)
  return cut === -1 ? url : url.slice(0, cut)
}

/** An API path without ids or query, e.g. /documents/:id/status, so reports group by endpoint. */
export function pathOnly(path: string): string {
  return stripUrl(path).replace(/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/gi, ':id')
}

export function scrubEvent(event: ErrorEvent): ErrorEvent {
  if (event.request) {
    if (event.request.url) event.request.url = stripUrl(event.request.url)
    delete event.request.query_string
    delete event.request.cookies
    delete event.request.data
    if (event.request.headers) {
      for (const name of Object.keys(event.request.headers)) {
        if (/^(authorization|cookie|apikey)$/i.test(name)) delete event.request.headers[name]
      }
    }
  }
  delete event.user
  return event
}

export function scrubBreadcrumb(crumb: Breadcrumb): Breadcrumb | null {
  // Console output can include document text or answers, so it is not kept.
  if (crumb.category === 'console') return null
  if (crumb.data) {
    for (const key of ['url', 'from', 'to']) {
      if (typeof crumb.data[key] === 'string') crumb.data[key] = stripUrl(crumb.data[key])
    }
  }
  return crumb
}

/** Reports a failed API call. Only server faults (5xx) and broken streams are worth an alert. */
export function reportApiFailure(err: Error, details: { path: string; status: number; traceId?: string }) {
  if (!sentryEnabled) return
  Sentry.withScope((scope) => {
    scope.setTag('category', 'api')
    scope.setTag('api_path', details.path)
    scope.setTag('status', String(details.status))
    // The same id as the backend's trace in Langfuse and its Sentry event.
    if (details.traceId) scope.setTag('trace_id', details.traceId)
    Sentry.captureException(err)
  })
}

/** Leaves a trail entry for failures that are not app faults (offline, server unreachable). */
export function noteApiProblem(message: string, path: string) {
  if (!sentryEnabled) return
  Sentry.addBreadcrumb({ category: 'api', level: 'warning', message, data: { path } })
}
