/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_SUPABASE_URL: string
  readonly VITE_SUPABASE_PUBLISHABLE_KEY: string
  readonly VITE_API_URL?: string
  readonly VITE_HCAPTCHA_SITE_KEY?: string
  readonly VITE_SENTRY_DSN?: string
  readonly VITE_SENTRY_ENVIRONMENT?: string
  readonly VITE_SENTRY_TRACES_SAMPLE_RATE?: string
}

/** Set at build time from Vercel's VERCEL_GIT_COMMIT_SHA and VERCEL_ENV (empty elsewhere). */
declare const __APP_RELEASE__: string
declare const __APP_ENVIRONMENT__: string

interface ImportMeta {
  readonly env: ImportMetaEnv
}
